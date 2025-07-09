package db

import (
	"context"
	"fmt"
)

func NewDriver(ctx context.Context, driver string, cfg Config) (Driver, error) {
	switch driver {
	case "postgres":
		return NewPostgres(ctx, cfg), nil
	// case "mysql": return NewMySQL(cfg), nil
	// case "sqlite": return NewSQLite(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported database type: %s", driver)
	}
}
