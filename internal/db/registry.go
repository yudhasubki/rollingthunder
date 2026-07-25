package db

import (
	"context"
	"fmt"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/mysql"
	"rollingthunder/pkg/database/postgres"
	sqlitedriver "rollingthunder/pkg/database/sqlite"
)

type driverFactory func(
	context.Context,
	string,
	database.Config,
) (database.Driver, error)

func NewDriver(ctx context.Context, driver string, cfg database.Config) (database.Driver, error) {
	switch driver {
	case "postgres":
		return postgres.NewPostgres(ctx, postgres.Config{
			Host:          cfg.Host,
			Port:          cfg.Port,
			User:          cfg.User,
			Password:      cfg.Password,
			Db:            cfg.Db,
			SSLMode:       cfg.SSLMode,
			SSLRootCert:   cfg.SSLRootCert,
			SSLCert:       cfg.SSLCert,
			SSLKey:        cfg.SSLKey,
			TLSServerName: cfg.TLSServerName,
		}), nil
	case "mysql", "mariadb":
		return mysql.NewMySQL(ctx, mysql.Config{
			Host:          cfg.Host,
			Port:          cfg.Port,
			User:          cfg.User,
			Password:      cfg.Password,
			Db:            cfg.Db,
			SSLMode:       cfg.SSLMode,
			SSLRootCert:   cfg.SSLRootCert,
			SSLCert:       cfg.SSLCert,
			SSLKey:        cfg.SSLKey,
			TLSServerName: cfg.TLSServerName,
		}), nil
	case "sqlite":
		return sqlitedriver.NewSQLite(ctx, sqlitedriver.Config{
			Db: cfg.Db,
		}), nil
	default:
		return nil, fmt.Errorf("unsupported database type: %s", driver)
	}
}
