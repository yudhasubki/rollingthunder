package db

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"rollingthunder/pkg/database"
)

type healthTestDriver struct {
	*routingTestDriver
	healthMu   sync.Mutex
	pingErr    error
	pingCalls  int
	connectErr error
}

func newHealthTestDriver(name string) *healthTestDriver {
	return &healthTestDriver{
		routingTestDriver: &routingTestDriver{name: name},
	}
}

func (driver *healthTestDriver) Ping(context.Context) error {
	driver.healthMu.Lock()
	defer driver.healthMu.Unlock()
	driver.pingCalls++
	return driver.pingErr
}

func (driver *healthTestDriver) Connect(context.Context) error {
	driver.healthMu.Lock()
	defer driver.healthMu.Unlock()
	return driver.connectErr
}

func (driver *healthTestDriver) setPingError(err error) {
	driver.healthMu.Lock()
	driver.pingErr = err
	driver.healthMu.Unlock()
}

func (driver *healthTestDriver) pingCount() int {
	driver.healthMu.Lock()
	defer driver.healthMu.Unlock()
	return driver.pingCalls
}

func TestConnectionHealthTracksFailureAndRecovery(t *testing.T) {
	service := NewService()
	service.Start(context.Background())
	driver := newHealthTestDriver("primary")
	service.connections["connection"] = &Connection{
		ID:     "connection",
		Driver: driver,
		Config: database.Config{Driver: "test"},
	}

	driver.setPingError(errors.New("network unavailable"))
	failed := service.CheckConnection("connection")
	if len(failed.Errors) != 1 ||
		failed.Data.State != database.ConnectionHealthDegraded ||
		failed.Data.FailureCount != 1 {
		t.Fatalf("failed health = %+v", failed)
	}

	driver.setPingError(nil)
	recovered := service.CheckConnection("connection")
	if len(recovered.Errors) > 0 ||
		recovered.Data.State != database.ConnectionHealthHealthy ||
		recovered.Data.FailureCount != 0 ||
		recovered.Data.LastHealthy == "" {
		t.Fatalf("recovered health = %+v", recovered)
	}
}

func TestReconnectKeepsOldDriverUntilReplacementConnects(t *testing.T) {
	service := NewService()
	service.Start(context.Background())
	oldDriver := newHealthTestDriver("old")
	service.connections["connection"] = &Connection{
		ID:     "connection",
		Driver: oldDriver,
		Config: database.Config{Driver: "test"},
	}
	failing := newHealthTestDriver("replacement")
	failing.connectErr = errors.New("dial failed")
	service.newDriver = func(
		context.Context,
		string,
		database.Config,
	) (database.Driver, error) {
		return failing, nil
	}

	result := service.ReconnectConnection("connection", "failed-reconnect")
	if len(result.Errors) != 1 ||
		result.Data.State != database.ConnectionHealthDegraded {
		t.Fatalf("ReconnectConnection() = %+v", result)
	}
	if service.connections["connection"].Driver != oldDriver {
		t.Fatal("failed reconnect replaced the working driver")
	}
	oldDriver.mu.Lock()
	oldClosed := oldDriver.closed
	oldDriver.mu.Unlock()
	if oldClosed {
		t.Fatal("failed reconnect closed the old driver")
	}
}

func TestReconnectAtomicallySwapsVerifiedDriver(t *testing.T) {
	service := NewService()
	service.Start(context.Background())
	oldDriver := newHealthTestDriver("old")
	replacement := newHealthTestDriver("replacement")
	service.connections["connection"] = &Connection{
		ID:     "connection",
		Driver: oldDriver,
		Config: database.Config{Driver: "test"},
	}
	service.newDriver = func(
		context.Context,
		string,
		database.Config,
	) (database.Driver, error) {
		return replacement, nil
	}

	result := service.ReconnectConnection("connection", "successful-reconnect")
	if len(result.Errors) > 0 ||
		result.Data.State != database.ConnectionHealthHealthy {
		t.Fatalf("ReconnectConnection() = %+v", result)
	}
	if service.connections["connection"].Driver != replacement {
		t.Fatal("successful reconnect did not install the replacement")
	}
	oldDriver.mu.Lock()
	oldClosed := oldDriver.closed
	oldDriver.mu.Unlock()
	if !oldClosed {
		t.Fatal("successful reconnect did not close the old driver")
	}
}

func TestReconnectAtomicallySwapsSSHTunnel(t *testing.T) {
	service := NewService()
	service.Start(context.Background())
	oldDriver := newHealthTestDriver("old")
	oldTunnel := &fakeConnectionTunnel{host: "127.0.0.1", port: "42001"}
	replacement := newHealthTestDriver("replacement")
	replacementTunnel := &fakeConnectionTunnel{
		host: "127.0.0.1",
		port: "42002",
	}
	service.connections["connection"] = &Connection{
		ID:           "connection",
		Driver:       oldDriver,
		Tunnel:       oldTunnel,
		EndpointHost: oldTunnel.host,
		EndpointPort: oldTunnel.port,
		Config: database.Config{
			Driver:     "postgres",
			Host:       "database.internal",
			Port:       "5432",
			SSHEnabled: true,
			SSHHost:    "bastion.example",
			SSHUser:    "deploy",
		},
	}
	service.newTunnel = func(
		context.Context,
		database.Config,
	) (connectionTunnel, error) {
		return replacementTunnel, nil
	}
	var received database.Config
	service.newDriver = func(
		_ context.Context,
		_ string,
		config database.Config,
	) (database.Driver, error) {
		received = config
		return replacement, nil
	}

	result := service.ReconnectConnection("connection", "ssh-reconnect")
	if len(result.Errors) > 0 ||
		result.Data.State != database.ConnectionHealthHealthy {
		t.Fatalf("ReconnectConnection() = %+v", result)
	}
	connection := service.connections["connection"]
	if connection.Driver != replacement ||
		connection.Tunnel != replacementTunnel ||
		connection.EndpointPort != replacementTunnel.port {
		t.Fatalf("replacement connection = %+v", connection)
	}
	if received.Host != replacementTunnel.host ||
		received.Port != replacementTunnel.port {
		t.Fatalf("replacement driver endpoint = %s:%s", received.Host, received.Port)
	}
	if !oldTunnel.closed.Load() || replacementTunnel.closed.Load() {
		t.Fatalf(
			"tunnels old=%t replacement=%t",
			oldTunnel.closed.Load(),
			replacementTunnel.closed.Load(),
		)
	}
}

func TestReconnectResolvesLatestSavedCredential(t *testing.T) {
	service, credentials := credentialTestService(t)
	saved := service.SaveConnection(database.Config{
		Name:     "Saved",
		Driver:   "postgres",
		Password: "old-password",
		Db:       "rolling",
	})
	if len(saved.Errors) > 0 {
		t.Fatalf("SaveConnection() = %+v", saved)
	}
	if err := credentials.Set(saved.Data.ID, "new-password"); err != nil {
		t.Fatal(err)
	}

	oldDriver := newHealthTestDriver("old")
	service.connections["connection"] = &Connection{
		ID:        "connection",
		ProfileID: saved.Data.ID,
		Driver:    oldDriver,
		Config: database.Config{
			Driver:   "postgres",
			Password: "old-password",
		},
	}
	replacement := newHealthTestDriver("replacement")
	var received database.Config
	service.newDriver = func(
		_ context.Context,
		_ string,
		config database.Config,
	) (database.Driver, error) {
		received = config
		return replacement, nil
	}

	result := service.ReconnectConnection("connection", "credential-reconnect")
	if len(result.Errors) > 0 {
		t.Fatalf("ReconnectConnection() = %+v", result)
	}
	if received.Password != "new-password" {
		t.Fatalf("reconnect password = %q", received.Password)
	}
}

func TestHealthMonitorPingsUntilContextStops(t *testing.T) {
	service := NewService()
	service.healthInterval = 5 * time.Millisecond
	service.healthTimeout = 50 * time.Millisecond
	driver := newHealthTestDriver("monitored")
	service.connections["connection"] = &Connection{
		ID:     "connection",
		Driver: driver,
		Config: database.Config{Driver: "test"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	service.Start(ctx)
	t.Cleanup(func() {
		cancel()
		service.Shutdown(context.Background())
	})

	deadline := time.Now().Add(250 * time.Millisecond)
	for driver.pingCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if driver.pingCount() == 0 {
		t.Fatal("health monitor did not ping the active connection")
	}
	cancel()
	before := driver.pingCount()
	time.Sleep(20 * time.Millisecond)
	if after := driver.pingCount(); after != before {
		t.Fatalf("health monitor kept running after cancellation: %d -> %d", before, after)
	}
}
