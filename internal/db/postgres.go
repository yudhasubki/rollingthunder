package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

type Postgres struct {
	cfg  Config
	ctx  context.Context
	conn *sqlx.DB
}

func NewPostgres(ctx context.Context, cfg Config) *Postgres {
	return &Postgres{cfg: cfg, ctx: ctx}
}

func (p *Postgres) Connect() error {
	dsn := fmt.Sprintf(
		"user=%s password=%s dbname=%s host=%s port=%s sslmode=disable",
		p.cfg.User, p.cfg.Password, p.cfg.DBName, p.cfg.Host, p.cfg.Port,
	)

	pool, err := pgxpool.New(p.ctx, dsn)
	if err != nil {
		return err
	}

	db := sqlx.NewDb(stdlib.OpenDBFromPool(pool), "pgx")
	p.conn = db
	return db.Ping()
}

func (p *Postgres) Close() error {
	return p.conn.Close()
}

func (p *Postgres) GetCollections(schema ...string) ([]string, error) {
	var targetSchema string
	if len(schema) > 0 {
		targetSchema = schema[0]
	}

	var tables []string
	query := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = $1
		AND table_type = 'BASE TABLE'
	`
	err := p.conn.Select(&tables, query, targetSchema)
	return tables, err
}

func (p *Postgres) GetSchemas() ([]string, error) {
	var schemas []string
	query := `
		SELECT schema_name
		FROM information_schema.schemata
		WHERE schema_name NOT IN ('pg_catalog', 'information_schema')
		ORDER BY schema_name
	`
	err := p.conn.Select(&schemas, query)

	return schemas, err
}
