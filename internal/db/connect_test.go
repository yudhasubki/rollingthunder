package db

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"rollingthunder/pkg/database"
)

type connectionTestDriver struct {
	database.Driver
	connect func(context.Context) error
	closed  atomic.Bool
}

func (driver *connectionTestDriver) Connect(ctx context.Context) error {
	if driver.connect == nil {
		return nil
	}
	return driver.connect(ctx)
}

func (driver *connectionTestDriver) Close() error {
	driver.closed.Store(true)
	return nil
}

func connectionTestService(driver database.Driver) *Service {
	service := NewService()
	service.Start(context.Background())
	service.newDriver = func(
		context.Context,
		string,
		database.Config,
	) (database.Driver, error) {
		return driver, nil
	}
	return service
}

func TestConnectRegistersSuccessfulConnection(t *testing.T) {
	driver := &connectionTestDriver{}
	service := connectionTestService(driver)

	result := service.Connect(ConnectRequest{
		AttemptID: "successful-attempt",
		Driver:    "postgres",
		Config: database.Config{
			Name: "Local",
			Db:   "rolling_thunder",
		},
	})

	if len(result.Errors) != 0 {
		t.Fatalf("Connect returned errors: %+v", result.Errors)
	}
	if !result.Data.Connected || result.Data.ConnectionID == "" {
		t.Fatalf("Connect returned an incomplete response: %+v", result.Data)
	}
	if driver.closed.Load() {
		t.Fatal("successful driver was closed")
	}
	if len(service.connections) != 1 {
		t.Fatalf("registered connections = %d, want 1", len(service.connections))
	}
	if len(service.connectionAttempts) != 0 {
		t.Fatalf("running connection attempts = %d, want 0", len(service.connectionAttempts))
	}
}

func TestConnectTimesOutAndClosesTemporaryDriver(t *testing.T) {
	driver := &connectionTestDriver{
		connect: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	service := connectionTestService(driver)
	service.connectionTimeout = 20 * time.Millisecond

	result := service.Connect(ConnectRequest{
		AttemptID: "timeout-attempt",
		Driver:    "postgres",
		Config:    database.Config{Db: "rolling_thunder"},
	})

	if len(result.Errors) != 1 ||
		!strings.Contains(result.Errors[0].Detail, "timed out after 20ms") {
		t.Fatalf("Connect timeout error = %+v", result.Errors)
	}
	if result.Data.Connected {
		t.Fatal("timed out connection was reported as connected")
	}
	if !driver.closed.Load() {
		t.Fatal("timed out driver was not closed")
	}
	if len(service.connections) != 0 {
		t.Fatalf("registered connections = %d, want 0", len(service.connections))
	}
}

func TestCancelConnectionAttemptStopsConnect(t *testing.T) {
	started := make(chan struct{})
	driver := &connectionTestDriver{
		connect: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	service := connectionTestService(driver)
	resultChannel := make(chan ConnectResponse, 1)
	errorChannel := make(chan string, 1)

	go func() {
		result := service.Connect(ConnectRequest{
			AttemptID: "cancel-attempt",
			Driver:    "postgres",
			Config:    database.Config{Db: "rolling_thunder"},
		})
		resultChannel <- result.Data
		if len(result.Errors) > 0 {
			errorChannel <- result.Errors[0].Detail
		}
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("connection attempt did not start")
	}

	cancelled := service.CancelConnectionAttempt("cancel-attempt")
	if len(cancelled.Errors) != 0 || !cancelled.Data {
		t.Fatalf("CancelConnectionAttempt = %+v", cancelled)
	}

	select {
	case result := <-resultChannel:
		if result.Connected {
			t.Fatal("cancelled connection was reported as connected")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled connection attempt did not stop")
	}

	select {
	case detail := <-errorChannel:
		if detail != "Connection attempt cancelled." {
			t.Fatalf("cancel error = %q", detail)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled connection returned no error")
	}

	if !driver.closed.Load() {
		t.Fatal("cancelled driver was not closed")
	}
	if len(service.connections) != 0 {
		t.Fatalf("registered connections = %d, want 0", len(service.connections))
	}
}

func TestCancelWinsAgainstDriverReturningSuccess(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	driver := &connectionTestDriver{
		connect: func(context.Context) error {
			close(started)
			<-release
			return nil
		},
	}
	service := connectionTestService(driver)
	done := make(chan responseSnapshot, 1)

	go func() {
		result := service.Connect(ConnectRequest{
			AttemptID: "cancel-success-race",
			Driver:    "postgres",
			Config:    database.Config{Db: "rolling_thunder"},
		})
		snapshot := responseSnapshot{connected: result.Data.Connected}
		if len(result.Errors) > 0 {
			snapshot.error = result.Errors[0].Detail
		}
		done <- snapshot
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("connection attempt did not start")
	}
	cancelled := service.CancelConnectionAttempt("cancel-success-race")
	if len(cancelled.Errors) != 0 {
		t.Fatalf("CancelConnectionAttempt returned errors: %+v", cancelled.Errors)
	}
	close(release)

	select {
	case result := <-done:
		if result.connected || result.error != "Connection attempt cancelled." {
			t.Fatalf("Connect after cancellation = %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("connection attempt did not return")
	}
	if !driver.closed.Load() {
		t.Fatal("driver returning success after cancellation was not closed")
	}
	if len(service.connections) != 0 {
		t.Fatalf("registered connections = %d, want 0", len(service.connections))
	}
}

func TestCancelConnectionAttemptRejectsUnknownAttempt(t *testing.T) {
	service := NewService()
	service.Start(context.Background())

	result := service.CancelConnectionAttempt("missing")

	if len(result.Errors) != 1 ||
		result.Errors[0].Detail != "connection attempt is not running" {
		t.Fatalf("CancelConnectionAttempt errors = %+v", result.Errors)
	}
}

type responseSnapshot struct {
	connected bool
	error     string
}
