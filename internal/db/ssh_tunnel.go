package db

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"rollingthunder/pkg/database"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

const defaultSSHPort = "22"

type connectionTunnel interface {
	LocalHost() string
	LocalPort() string
	Close() error
}

type tunnelFactory func(context.Context, database.Config) (connectionTunnel, error)

type tunnelConnection struct {
	local  net.Conn
	remote net.Conn
}

type sshDatabaseTunnel struct {
	client     *ssh.Client
	listener   net.Listener
	target     string
	done       chan struct{}
	closeOnce  sync.Once
	closeError error
	mu         sync.Mutex
	active     map[*tunnelConnection]struct{}
	wg         sync.WaitGroup
}

func (tunnel *sshDatabaseTunnel) LocalHost() string {
	host, _, err := net.SplitHostPort(tunnel.listener.Addr().String())
	if err != nil {
		return "127.0.0.1"
	}
	return host
}

func (tunnel *sshDatabaseTunnel) LocalPort() string {
	_, port, err := net.SplitHostPort(tunnel.listener.Addr().String())
	if err != nil {
		return ""
	}
	return port
}

func (tunnel *sshDatabaseTunnel) Close() error {
	tunnel.closeOnce.Do(func() {
		close(tunnel.done)
		if err := tunnel.listener.Close(); err != nil &&
			!errors.Is(err, net.ErrClosed) {
			tunnel.closeError = err
		}

		tunnel.mu.Lock()
		active := make([]*tunnelConnection, 0, len(tunnel.active))
		for connection := range tunnel.active {
			active = append(active, connection)
		}
		tunnel.mu.Unlock()
		for _, connection := range active {
			_ = connection.local.Close()
			_ = connection.remote.Close()
		}
		if err := tunnel.client.Close(); err != nil &&
			!errors.Is(err, net.ErrClosed) &&
			tunnel.closeError == nil {
			tunnel.closeError = err
		}
		tunnel.wg.Wait()
	})
	return tunnel.closeError
}

func (tunnel *sshDatabaseTunnel) serve() {
	for {
		local, err := tunnel.listener.Accept()
		if err != nil {
			select {
			case <-tunnel.done:
				return
			default:
				continue
			}
		}
		tunnel.wg.Add(1)
		go tunnel.forward(local)
	}
}

func (tunnel *sshDatabaseTunnel) forward(local net.Conn) {
	defer tunnel.wg.Done()
	remote, err := tunnel.client.Dial("tcp", tunnel.target)
	if err != nil {
		_ = local.Close()
		return
	}
	connection := &tunnelConnection{local: local, remote: remote}
	tunnel.mu.Lock()
	tunnel.active[connection] = struct{}{}
	tunnel.mu.Unlock()
	defer func() {
		_ = local.Close()
		_ = remote.Close()
		tunnel.mu.Lock()
		delete(tunnel.active, connection)
		tunnel.mu.Unlock()
	}()

	finished := make(chan struct{}, 2)
	copyStream := func(destination io.Writer, source io.Reader) {
		_, _ = io.Copy(destination, source)
		finished <- struct{}{}
	}
	go copyStream(remote, local)
	go copyStream(local, remote)
	<-finished
}

func normalizeSSHFingerprint(value string) string {
	value = strings.TrimSpace(value)
	if value != "" && !strings.HasPrefix(value, "SHA256:") {
		value = "SHA256:" + value
	}
	return value
}

func sshHostKeyCallback(
	config database.Config,
) (ssh.HostKeyCallback, string, error) {
	if expected := normalizeSSHFingerprint(config.SSHHostKeyFingerprint); expected != "" {
		callback := func(_ string, _ net.Addr, key ssh.PublicKey) error {
			actual := ssh.FingerprintSHA256(key)
			if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
				return fmt.Errorf(
					"SSH host key fingerprint mismatch: received %s",
					actual,
				)
			}
			return nil
		}
		return callback, "pinned fingerprint " + expected, nil
	}

	path := strings.TrimSpace(config.SSHKnownHostsPath)
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "", fmt.Errorf(
				"locate the default SSH known_hosts file: %w",
				err,
			)
		}
		path = filepath.Join(home, ".ssh", "known_hosts")
	} else {
		expanded, err := expandSSHPath(path)
		if err != nil {
			return nil, "", err
		}
		path = expanded
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf(
				"SSH known_hosts file %q does not exist; add the host key or provide its SHA256 fingerprint",
				path,
			)
		}
		return nil, "", fmt.Errorf("read SSH known_hosts file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("SSH known_hosts path %q is not a regular file", path)
	}
	callback, err := knownhosts.New(path)
	if err != nil {
		return nil, "", fmt.Errorf("parse SSH known_hosts file: %w", err)
	}
	return callback, path, nil
}

func expandSSHPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") ||
		strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand SSH path %q: %w", path, err)
		}
		if path == "~" {
			return home, nil
		}
		path = filepath.Join(home, path[2:])
	}
	return filepath.Clean(path), nil
}

func sshAuthentication(
	ctx context.Context,
	config database.Config,
) (ssh.AuthMethod, func(), string, error) {
	mode := resolvedSSHAuthMode(config)
	switch mode {
	case "agent":
		socket := strings.TrimSpace(os.Getenv("SSH_AUTH_SOCK"))
		if socket == "" {
			return nil, nil, "", fmt.Errorf(
				"SSH agent authentication selected, but SSH_AUTH_SOCK is not set",
			)
		}
		agentConnection, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
		if err != nil {
			return nil, nil, "", fmt.Errorf("connect to SSH agent: %w", err)
		}
		cleanup := func() { _ = agentConnection.Close() }
		return ssh.PublicKeysCallback(
			agent.NewClient(agentConnection).Signers,
		), cleanup, "SSH agent", nil

	case "private-key", "key":
		path, err := expandSSHPath(config.SSHPrivateKeyPath)
		if err != nil {
			return nil, nil, "", err
		}
		if strings.TrimSpace(path) == "" || path == "." {
			return nil, nil, "", fmt.Errorf(
				"SSH private key path is required for private-key authentication",
			)
		}
		privateKey, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, "", fmt.Errorf("read SSH private key %q: %w", path, err)
		}
		var signer ssh.Signer
		if config.SSHKeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(
				privateKey,
				[]byte(config.SSHKeyPassphrase),
			)
		} else {
			signer, err = ssh.ParsePrivateKey(privateKey)
		}
		var passphraseMissing *ssh.PassphraseMissingError
		if errors.As(err, &passphraseMissing) {
			return nil, nil, "", fmt.Errorf(
				"SSH private key %q is encrypted; enter its passphrase",
				path,
			)
		}
		if err != nil {
			return nil, nil, "", fmt.Errorf("parse SSH private key %q: %w", path, err)
		}
		return ssh.PublicKeys(signer), func() {}, "private key " + path, nil

	case "password":
		if config.SSHPassword == "" {
			return nil, nil, "", fmt.Errorf(
				"SSH password is required for password authentication",
			)
		}
		return ssh.Password(config.SSHPassword), func() {}, "password", nil

	default:
		return nil, nil, "", fmt.Errorf(
			"unsupported SSH authentication mode %q",
			config.SSHAuthMode,
		)
	}
}

func resolvedSSHAuthMode(config database.Config) string {
	mode := strings.ToLower(strings.TrimSpace(config.SSHAuthMode))
	if mode == "" {
		switch {
		case strings.TrimSpace(config.SSHPrivateKeyPath) != "":
			mode = "private-key"
		case config.SSHPassword != "":
			mode = "password"
		default:
			mode = "agent"
		}
	}
	return mode
}

func validateSSHConfig(config database.Config) error {
	if !config.SSHEnabled {
		return fmt.Errorf("SSH tunnel is not enabled")
	}
	if config.Driver == "sqlite" {
		return fmt.Errorf("SQLite connections cannot use an SSH tunnel")
	}
	if strings.TrimSpace(config.SSHHost) == "" {
		return fmt.Errorf("SSH host is required")
	}
	if strings.TrimSpace(config.SSHUser) == "" {
		return fmt.Errorf("SSH username is required")
	}
	if strings.TrimSpace(config.Host) == "" ||
		strings.TrimSpace(config.Port) == "" {
		return fmt.Errorf("database host and port are required for SSH forwarding")
	}
	return nil
}

func newSSHTunnel(
	ctx context.Context,
	config database.Config,
) (connectionTunnel, error) {
	if err := validateSSHConfig(config); err != nil {
		return nil, err
	}
	sshPort := strings.TrimSpace(config.SSHPort)
	if sshPort == "" {
		sshPort = defaultSSHPort
	}
	sshAddress := net.JoinHostPort(strings.TrimSpace(config.SSHHost), sshPort)
	target := net.JoinHostPort(
		strings.TrimSpace(config.Host),
		strings.TrimSpace(config.Port),
	)
	hostKeyCallback, verification, err := sshHostKeyCallback(config)
	if err != nil {
		return nil, err
	}
	authMethod, cleanupAuth, authentication, err := sshAuthentication(ctx, config)
	if err != nil {
		return nil, err
	}
	defer cleanupAuth()

	clientConfig := &ssh.ClientConfig{
		User:            strings.TrimSpace(config.SSHUser),
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: hostKeyCallback,
		Timeout:         defaultConnectionTimeout,
	}
	rawConnection, err := (&net.Dialer{}).DialContext(ctx, "tcp", sshAddress)
	if err != nil {
		return nil, fmt.Errorf("connect to SSH server %s: %w", sshAddress, err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = rawConnection.SetDeadline(deadline)
	}
	stopCancellationWatch := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = rawConnection.Close()
		case <-stopCancellationWatch:
		}
	}()
	clientConnection, channels, requests, err := ssh.NewClientConn(
		rawConnection,
		sshAddress,
		clientConfig,
	)
	close(stopCancellationWatch)
	if err != nil {
		_ = rawConnection.Close()
		return nil, fmt.Errorf(
			"authenticate SSH connection with %s and verify host using %s: %w",
			authentication,
			verification,
			err,
		)
	}
	if err := ctx.Err(); err != nil {
		_ = clientConnection.Close()
		return nil, err
	}
	_ = rawConnection.SetDeadline(time.Time{})
	client := ssh.NewClient(clientConnection, channels, requests)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("open local SSH forwarding socket: %w", err)
	}
	tunnel := &sshDatabaseTunnel{
		client:   client,
		listener: listener,
		target:   target,
		done:     make(chan struct{}),
		active:   make(map[*tunnelConnection]struct{}),
	}
	go tunnel.serve()
	return tunnel, nil
}
