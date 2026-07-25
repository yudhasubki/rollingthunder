package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConnectHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	driver := NewPostgres(ctx, Config{
		Db:   "rolling_thunder",
		Host: "127.0.0.1",
		Port: "1",
	})
	startedAt := time.Now()

	err := driver.Connect(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Connect error = %v, want context cancellation", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("cancelled Connect took %s", elapsed)
	}
	if driver.conn != nil {
		t.Fatal("cancelled Connect retained a database handle")
	}
}

func TestPostgresTLSServerNameSurvivesLocalTunnelEndpoint(t *testing.T) {
	config, err := pgxpool.ParseConfig(
		"host=127.0.0.1 port=41001 dbname=rolling sslmode=verify-full",
	)
	if err != nil {
		t.Fatal(err)
	}
	applyPostgresTLSServerName(config, "database.internal")
	if config.ConnConfig.TLSConfig == nil ||
		config.ConnConfig.TLSConfig.ServerName != "database.internal" {
		t.Fatalf("TLS config = %#v", config.ConnConfig.TLSConfig)
	}
	for _, fallback := range config.ConnConfig.Fallbacks {
		if fallback.TLSConfig != nil &&
			fallback.TLSConfig.ServerName != "database.internal" {
			t.Fatalf(
				"fallback TLS server name = %q",
				fallback.TLSConfig.ServerName,
			)
		}
	}
}

func TestPostgresPoolConfigPreservesQuotedValues(t *testing.T) {
	config, err := buildPostgresPoolConfig(Config{
		Host:     "127.0.0.1",
		Port:     "5432",
		User:     "local user",
		Password: `pa'ss\word`,
		Db:       "my app db",
		SSLMode:  "disable",
	})
	if err != nil {
		t.Fatal(err)
	}

	if config.ConnConfig.Database != "my app db" {
		t.Fatalf("database = %q", config.ConnConfig.Database)
	}
	if config.ConnConfig.User != "local user" {
		t.Fatalf("user = %q", config.ConnConfig.User)
	}
	if config.ConnConfig.Password != `pa'ss\word` {
		t.Fatalf("password = %q", config.ConnConfig.Password)
	}
	if got := config.ConnConfig.RuntimeParams["application_name"]; got != "Rolling Thunder" {
		t.Fatalf("application_name = %q", got)
	}
}
