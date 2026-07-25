package db

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"rollingthunder/pkg/database"

	"golang.org/x/crypto/ssh"
)

type fakeConnectionTunnel struct {
	host   string
	port   string
	closed atomic.Bool
}

func (tunnel *fakeConnectionTunnel) LocalHost() string {
	return tunnel.host
}

func (tunnel *fakeConnectionTunnel) LocalPort() string {
	return tunnel.port
}

func (tunnel *fakeConnectionTunnel) Close() error {
	tunnel.closed.Store(true)
	return nil
}

func TestSSHHostKeyFingerprintRequiresExactMatch(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := ssh.FingerprintSHA256(signer.PublicKey())
	callback, verification, err := sshHostKeyCallback(database.Config{
		SSHHostKeyFingerprint: strings.TrimPrefix(fingerprint, "SHA256:"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(verification, fingerprint) {
		t.Fatalf("verification description = %q", verification)
	}
	if err := callback(
		"ssh.example:22",
		&net.TCPAddr{},
		signer.PublicKey(),
	); err != nil {
		t.Fatalf("matching host key rejected: %v", err)
	}

	_, otherPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherSigner, err := ssh.NewSignerFromKey(otherPrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := callback(
		"ssh.example:22",
		&net.TCPAddr{},
		otherSigner.PublicKey(),
	); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("mismatched host key error = %v", err)
	}
}

func TestValidateSSHConfigRejectsUnsafeOrIncompleteProfiles(t *testing.T) {
	tests := []struct {
		name   string
		config database.Config
		match  string
	}{
		{
			name: "disabled",
			config: database.Config{
				Driver: "postgres",
			},
			match: "not enabled",
		},
		{
			name: "sqlite",
			config: database.Config{
				Driver:     "sqlite",
				SSHEnabled: true,
			},
			match: "SQLite",
		},
		{
			name: "missing ssh host",
			config: database.Config{
				Driver:     "postgres",
				SSHEnabled: true,
				SSHUser:    "deploy",
				Host:       "database.internal",
				Port:       "5432",
			},
			match: "SSH host",
		},
		{
			name: "missing target",
			config: database.Config{
				Driver:     "postgres",
				SSHEnabled: true,
				SSHHost:    "bastion.example",
				SSHUser:    "deploy",
			},
			match: "database host and port",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSSHConfig(test.config)
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("validateSSHConfig() error = %v", err)
			}
		})
	}
}

func TestConnectRoutesDriverThroughTunnelAndClosesIt(t *testing.T) {
	driver := &connectionTestDriver{}
	service := connectionTestService(driver)
	tunnel := &fakeConnectionTunnel{host: "127.0.0.1", port: "41001"}
	var tunnelConfig database.Config
	service.newTunnel = func(
		_ context.Context,
		config database.Config,
	) (connectionTunnel, error) {
		tunnelConfig = config
		return tunnel, nil
	}
	var driverConfig database.Config
	service.newDriver = func(
		_ context.Context,
		_ string,
		config database.Config,
	) (database.Driver, error) {
		driverConfig = config
		return driver, nil
	}

	result := service.Connect(ConnectRequest{
		AttemptID: "ssh-connect",
		Driver:    "postgres",
		Config: database.Config{
			Name:       "Private database",
			Driver:     "postgres",
			Host:       "database.internal",
			Port:       "5432",
			Db:         "rolling",
			SSHEnabled: true,
			SSHHost:    "bastion.example",
			SSHUser:    "deploy",
		},
	})
	if len(result.Errors) > 0 || !result.Data.Connected {
		t.Fatalf("Connect() = %+v", result)
	}
	if tunnelConfig.Host != "database.internal" ||
		tunnelConfig.SSHHost != "bastion.example" {
		t.Fatalf("tunnel config = %+v", tunnelConfig)
	}
	if driverConfig.Host != tunnel.host || driverConfig.Port != tunnel.port {
		t.Fatalf("driver endpoint = %s:%s", driverConfig.Host, driverConfig.Port)
	}
	if driverConfig.TLSServerName != "database.internal" {
		t.Fatalf("TLS server name = %q", driverConfig.TLSServerName)
	}
	connection := service.connections[result.Data.ConnectionID]
	if connection.Config.Host != "database.internal" ||
		connection.EndpointPort != tunnel.port {
		t.Fatalf("stored connection = %+v", connection)
	}
	if tunnel.closed.Load() {
		t.Fatal("active tunnel was closed")
	}

	disconnected := service.DisconnectConnection(result.Data.ConnectionID)
	if len(disconnected.Errors) > 0 || !disconnected.Data {
		t.Fatalf("DisconnectConnection() = %+v", disconnected)
	}
	if !driver.closed.Load() || !tunnel.closed.Load() {
		t.Fatalf(
			"cleanup driver=%t tunnel=%t",
			driver.closed.Load(),
			tunnel.closed.Load(),
		)
	}
}

func TestConnectClosesTunnelWhenDatabaseConnectFails(t *testing.T) {
	driver := &connectionTestDriver{
		connect: func(context.Context) error {
			return context.DeadlineExceeded
		},
	}
	service := connectionTestService(driver)
	tunnel := &fakeConnectionTunnel{host: "127.0.0.1", port: "41002"}
	service.newTunnel = func(
		context.Context,
		database.Config,
	) (connectionTunnel, error) {
		return tunnel, nil
	}

	result := service.Connect(ConnectRequest{
		AttemptID: "ssh-connect-failure",
		Driver:    "postgres",
		Config: database.Config{
			Driver:     "postgres",
			Host:       "database.internal",
			Port:       "5432",
			SSHEnabled: true,
		},
	})
	if len(result.Errors) == 0 {
		t.Fatal("Connect() unexpectedly succeeded")
	}
	if !driver.closed.Load() || !tunnel.closed.Load() {
		t.Fatalf(
			"cleanup driver=%t tunnel=%t",
			driver.closed.Load(),
			tunnel.closed.Load(),
		)
	}
}

type directTCPIPRequest struct {
	DestinationHost string
	DestinationPort uint32
	OriginHost      string
	OriginPort      uint32
}

func proxySSHChannel(channel ssh.Channel, upstream net.Conn) {
	defer channel.Close()
	defer upstream.Close()
	finished := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(channel, upstream)
		finished <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(upstream, channel)
		finished <- struct{}{}
	}()
	<-finished
}

func serveSSHForwardConnection(
	raw net.Conn,
	config *ssh.ServerConfig,
) {
	connection, channels, requests, err := ssh.NewServerConn(raw, config)
	if err != nil {
		_ = raw.Close()
		return
	}
	defer connection.Close()
	go ssh.DiscardRequests(requests)
	for request := range channels {
		if request.ChannelType() != "direct-tcpip" {
			_ = request.Reject(ssh.UnknownChannelType, "unsupported channel")
			continue
		}
		var forward directTCPIPRequest
		if err := ssh.Unmarshal(request.ExtraData(), &forward); err != nil {
			_ = request.Reject(ssh.ConnectionFailed, "invalid target")
			continue
		}
		target := net.JoinHostPort(
			forward.DestinationHost,
			fmt.Sprint(forward.DestinationPort),
		)
		upstream, err := net.DialTimeout("tcp", target, time.Second)
		if err != nil {
			_ = request.Reject(ssh.ConnectionFailed, err.Error())
			continue
		}
		channel, channelRequests, err := request.Accept()
		if err != nil {
			_ = upstream.Close()
			continue
		}
		go ssh.DiscardRequests(channelRequests)
		go proxySSHChannel(channel, upstream)
	}
}

func TestSSHTunnelForwardsTrafficWithPinnedHostKey(t *testing.T) {
	echoListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("local TCP listeners are not allowed in this sandbox: %v", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = echoListener.Close() })
	go func() {
		for {
			connection, err := echoListener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				_, _ = io.Copy(connection, connection)
			}()
		}
	}()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := &ssh.ServerConfig{
		PasswordCallback: func(
			_ ssh.ConnMetadata,
			password []byte,
		) (*ssh.Permissions, error) {
			if string(password) != "ssh-secret" {
				return nil, fmt.Errorf("password rejected")
			}
			return nil, nil
		},
	}
	serverConfig.AddHostKey(signer)
	sshListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("local TCP listeners are not allowed in this sandbox: %v", err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sshListener.Close() })
	go func() {
		for {
			connection, err := sshListener.Accept()
			if err != nil {
				return
			}
			go serveSSHForwardConnection(connection, serverConfig)
		}
	}()

	sshHost, sshPort, err := net.SplitHostPort(sshListener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	databaseHost, databasePort, err := net.SplitHostPort(
		echoListener.Addr().String(),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	tunnel, err := newSSHTunnel(ctx, database.Config{
		Driver:                "postgres",
		Host:                  databaseHost,
		Port:                  databasePort,
		SSHEnabled:            true,
		SSHHost:               sshHost,
		SSHPort:               sshPort,
		SSHUser:               "deploy",
		SSHAuthMode:           "password",
		SSHPassword:           "ssh-secret",
		SSHHostKeyFingerprint: ssh.FingerprintSHA256(signer.PublicKey()),
	})
	if err != nil {
		t.Fatalf("newSSHTunnel() = %v", err)
	}
	t.Cleanup(func() { _ = tunnel.Close() })

	connection, err := net.DialTimeout(
		"tcp",
		net.JoinHostPort(tunnel.LocalHost(), tunnel.LocalPort()),
		time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	message := []byte("rolling thunder over ssh")
	if _, err := connection.Write(message); err != nil {
		t.Fatal(err)
	}
	received := make([]byte, len(message))
	if _, err := io.ReadFull(connection, received); err != nil {
		t.Fatal(err)
	}
	if string(received) != string(message) {
		t.Fatalf("forwarded payload = %q", received)
	}
}
