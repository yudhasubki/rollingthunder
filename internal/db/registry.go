package db

import (
	"context"
	"fmt"
	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/postgres"
)

func NewDriver(ctx context.Context, driver string, cfg database.Config) (database.Driver, error) {
	switch driver {
	case "postgres":
		return postgres.NewPostgres(ctx, postgres.Config{
			Host:     cfg.Host,
			Port:     cfg.Port,
			User:     cfg.User,
			Password: cfg.Password,
			Db:       cfg.Db,
		}), nil
	// case "mysql": return NewMySQL(cfg), nil
	// case "sqlite": return NewSQLite(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported database type: %s", driver)
	}
}
