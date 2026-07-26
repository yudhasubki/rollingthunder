package postgres

import (
	"context"
	"os"
	"testing"

	"rollingthunder/pkg/database/drivertest"
)

func TestPostgresSharedCapabilityContract(t *testing.T) {
	driver := NewPostgres(context.Background(), Config{})
	drivertest.RunCapabilityContract(t, driver, "postgres")
}

func TestPostgresLiveConformance(t *testing.T) {
	host := os.Getenv("ROLLINGTHUNDER_POSTGRES_TEST_HOST")
	if host == "" {
		t.Skip("set ROLLINGTHUNDER_POSTGRES_TEST_HOST to run live PostgreSQL conformance")
	}
	config := Config{
		Host:        host,
		Port:        os.Getenv("ROLLINGTHUNDER_POSTGRES_TEST_PORT"),
		User:        os.Getenv("ROLLINGTHUNDER_POSTGRES_TEST_USER"),
		Password:    os.Getenv("ROLLINGTHUNDER_POSTGRES_TEST_PASSWORD"),
		Db:          os.Getenv("ROLLINGTHUNDER_POSTGRES_TEST_DATABASE"),
		SSLMode:     os.Getenv("ROLLINGTHUNDER_POSTGRES_TEST_SSL_MODE"),
		SSLRootCert: os.Getenv("ROLLINGTHUNDER_POSTGRES_TEST_SSL_ROOT_CERT"),
		SSLCert:     os.Getenv("ROLLINGTHUNDER_POSTGRES_TEST_SSL_CERT"),
		SSLKey:      os.Getenv("ROLLINGTHUNDER_POSTGRES_TEST_SSL_KEY"),
		TLSServerName: os.Getenv(
			"ROLLINGTHUNDER_POSTGRES_TEST_TLS_SERVER_NAME",
		),
	}
	driver := NewPostgres(context.Background(), config)
	if err := driver.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		if err := driver.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	drivertest.RunLiveContract(t, drivertest.LiveConfig{
		Driver:             driver,
		Schema:             "public",
		IntegerType:        "integer",
		TextType:           "text",
		ExercisePrivileged: os.Getenv("ROLLINGTHUNDER_TEST_PRIVILEGED") == "1",
	})
}
