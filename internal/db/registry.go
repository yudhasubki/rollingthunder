package db

import (
	"context"
	"fmt"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/mysql"
	"rollingthunder/pkg/database/oracle"
	"rollingthunder/pkg/database/postgres"
	sqlitedriver "rollingthunder/pkg/database/sqlite"
	"rollingthunder/pkg/database/sqlserver"
)

type driverFactory func(
	context.Context,
	string,
	database.Config,
) (database.Driver, error)

func NewDriver(ctx context.Context, driver string, cfg database.Config) (database.Driver, error) {
	switch driver {
	case database.DriverPostgres:
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
	case database.DriverMySQL, database.DriverMariaDB:
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
	case database.DriverSQLite:
		return sqlitedriver.NewSQLite(ctx, sqlitedriver.Config{
			Db: cfg.Db,
		}), nil
	case database.DriverOracle:
		return oracle.NewOracle(ctx, oracle.Config{
			Host:           cfg.Host,
			Port:           cfg.Port,
			User:           cfg.User,
			Password:       cfg.Password,
			Db:             cfg.Db,
			SSLMode:        cfg.SSLMode,
			SSLRootCert:    cfg.SSLRootCert,
			SSLCert:        cfg.SSLCert,
			SSLKey:         cfg.SSLKey,
			TLSServerName:  cfg.TLSServerName,
			ConnectionMode: cfg.OracleConnectionMode,
			TNSConfigPath:  cfg.OracleTNSConfigPath,
			TNSAlias:       cfg.OracleTNSAlias,
			WalletPath:     cfg.OracleWalletPath,
			WalletPassword: cfg.OracleWalletPassword,
		}), nil
	case database.DriverSQLServer:
		return sqlserver.NewSQLServer(ctx, sqlserver.Config{
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
	default:
		return nil, fmt.Errorf("unsupported database type: %s", driver)
	}
}
