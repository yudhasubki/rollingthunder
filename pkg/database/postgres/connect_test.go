package postgres

import (
	"context"
	"errors"
	"testing"
	"time"
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
