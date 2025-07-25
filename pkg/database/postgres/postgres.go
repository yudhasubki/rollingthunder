package postgres

import (
	"context"
	"errors"
	"fmt"
	"rollingthunder/pkg/database"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	Db       string
}

type Postgres struct {
	cfg    Config
	ctx    context.Context
	conn   *sqlx.DB
	engine string
}

func NewPostgres(ctx context.Context, cfg Config) *Postgres {
	return &Postgres{
		cfg:    cfg,
		ctx:    ctx,
		engine: "PostgreSQL",
	}
}

func (p *Postgres) Connect() error {
	if p.cfg.Db == "" {
		return errors.New("database is not exists")
	}

	dsn := []string{"dbname=" + p.cfg.Db, "sslmode=disable"}
	if p.cfg.User != "" {
		dsn = append(dsn, fmt.Sprintf("user=%s", p.cfg.User))
	}

	if p.cfg.Password != "" {
		dsn = append(dsn, fmt.Sprintf("password=%s", p.cfg.Password))
	}

	port := "5432"
	if p.cfg.Port != "5432" {
		port = p.cfg.Port
	}
	dsn = append(dsn, fmt.Sprintf("port=%s", port))

	pool, err := pgxpool.New(p.ctx, strings.Join(dsn, " "))
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
		SELECT 
			c.relname AS table_name
		FROM 
			pg_class c
		JOIN 
			pg_namespace n ON c.relnamespace = n.oid
		WHERE 
			n.nspname = $1
			AND c.relkind = 'r' 
		ORDER BY 
			c.oid`
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

func (p *Postgres) GetIndices(table database.Table) (database.Indices, error) {
	const query = `
	SELECT
		i.relname AS index_name,
		a.attname AS column_name,
		ix.indisunique AS is_unique,
		am.amname AS algorithm
	FROM
		pg_class t
		JOIN pg_index ix ON t.oid = ix.indrelid
		JOIN pg_class i ON i.oid = ix.indexrelid
		JOIN pg_am am ON i.relam = am.oid
		JOIN unnest(ix.indkey) WITH ORDINALITY AS cols(attnum, ord) ON TRUE
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = cols.attnum
	WHERE
		t.oid = $1::regclass
	ORDER BY
		i.relname, cols.ord;
	`

	ref := fmt.Sprintf("%s.%s", table.Schema, table.Name)

	var indices Indices
	err := p.conn.Select(&indices, query, ref)
	if err != nil {
		return nil, err
	}

	indexMap := map[string]*database.Index{}
	for _, index := range indices {
		idx, ok := indexMap[index.IndexName]
		if !ok {
			idx = &database.Index{
				Name:      index.IndexName,
				IsUnique:  index.IsUnique,
				Algorithm: index.Algorithm,
			}
			indexMap[index.IndexName] = idx
		}
		idx.Columns = append(idx.Columns, index.ColumnName)
	}

	var result database.Indices
	for _, idx := range indexMap {
		result = append(result, *idx)
	}

	return result, nil
}

func (p *Postgres) GetForeignKey(table database.Table) (Constraints, error) {
	constraintQuery := `
		SELECT
			a.attname AS column,
			c.contype AS type,
			f.relname AS foreign_table,
			fa.attname AS foreign_column
		FROM pg_constraint c
		JOIN pg_class f ON f.oid = c.confrelid
		JOIN unnest(c.conkey) WITH ORDINALITY AS ck(attnum, ord) ON TRUE
		JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ck.attnum
		JOIN unnest(c.confkey) WITH ORDINALITY AS fk(attnum, ord) ON fk.ord = ck.ord
		JOIN pg_attribute fa ON fa.attrelid = c.confrelid AND fa.attnum = fk.attnum
		WHERE c.conrelid = $1::regclass AND c.contype = 'f'
	`

	var constraints []Constraint
	err := p.conn.Select(&constraints, constraintQuery, fmt.Sprintf("%s.%s", table.Schema, table.Name))
	if err != nil {
		return nil, err
	}

	return constraints, nil
}

func (p *Postgres) GetConstraints(table database.Table) (Constraints, error) {
	const query = `
		SELECT a.attname AS column, c.contype AS type,
		       confrelid::regclass::text AS foreign_table
		FROM pg_attribute a
		JOIN pg_constraint c ON c.conrelid = a.attrelid AND a.attnum = ANY(c.conkey)
		WHERE c.conrelid = $1::regclass AND c.contype IN ('p', 'u', 'f')`

	var out []Constraint
	ref := fmt.Sprintf("%s.%s", table.Schema, table.Name)
	err := p.conn.Select(&out, query, ref)

	return out, err
}

func (p *Postgres) getCollectionStructures(table database.Table) (Columns, error) {
	var (
		query = `SELECT
			column_name,
			data_type,
			is_nullable,
			character_maximum_length,
			column_default
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`
	)

	var rows Columns
	err := p.conn.Select(&rows, query, table.Schema, table.Name)
	if err != nil {
		return nil, err
	}

	return rows, nil
}

func (p *Postgres) GetCollectionStructures(table database.Table) (database.Structures, error) {
	foreignKeys, err := p.GetForeignKey(table)
	if err != nil {
		return nil, err
	}

	constraints, err := p.GetConstraints(table)
	if err != nil {
		return nil, err
	}
	constraints = append(constraints, foreignKeys...)

	cMap := map[string]Constraint{}
	for _, c := range constraints {
		cMap[c.Column] = c
	}

	rows, err := p.getCollectionStructures(table)
	if err != nil {
		return nil, err
	}

	out := make(database.Structures, 0, len(rows))
	for _, r := range rows {
		dataType := r.DataType
		if v, exist := Types[dataType]; exist {
			dataType = v
		}

		info := database.Structure{
			Name:      r.ColumnName,
			DataType:  dataType,
			Length:    r.MaxLength,
			Nullable:  r.IsNullable == "YES",
			Default:   r.ColumnDefault,
			IsAutoInc: r.ColumnDefault != nil && strings.HasPrefix(*r.ColumnDefault, "nextval("),
		}

		if constraint, exist := cMap[r.ColumnName]; exist {
			switch constraint.Type {
			case "p":
				info.IsPrimary = true
				info.IsPrimaryLabel = "PRI"
			case "u":
				info.IsUnique = true
			case "f":
				if constraint.IsForeign() {
					key := fmt.Sprintf("%s(%s)", *constraint.ForeignTable, *constraint.ForeignCol)
					info.ForeignKey = &key
				}
			}
		}

		out = append(out, info)
	}

	return out, nil
}

func (p *Postgres) GetDatabaseInfo() (database.Info, error) {
	var version, db string
	err := p.conn.Get(&version, "SELECT current_setting('server_version')")
	if err != nil {
		return database.Info{}, err
	}
	err = p.conn.Get(&db, "SELECT current_database()")

	return database.Info{
		Engine:   p.engine,
		Version:  version,
		Database: db,
	}, err
}

func (p *Postgres) CountCollectionData(table database.Table) (int, error) {
	var result int
	err := p.conn.Get(&result, fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", table.Schema, table.Name))
	return result, err
}

func (p *Postgres) GetCollectionData(table database.Table) (database.Structures, []map[string]interface{}, error) {
	query := fmt.Sprintf(`
		SELECT * FROM %s.%s 
		LIMIT %d OFFSET %d`,
		table.Schema, table.Name, table.Limit, table.Offset)
	rows, err := p.conn.Queryx(query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	columns, err := p.getCollectionStructures(table)
	if err != nil {
		return nil, nil, err
	}

	structures := make(database.Structures, 0, len(columns))
	for _, column := range columns {
		dataType := column.DataType
		if v, exist := Types[dataType]; exist {
			dataType = v
		}

		structure := database.Structure{
			Name:      column.ColumnName,
			DataType:  dataType,
			Length:    column.MaxLength,
			Nullable:  column.IsNullable == "YES",
			Default:   column.ColumnDefault,
			IsAutoInc: column.ColumnDefault != nil && strings.HasPrefix(*column.ColumnDefault, "nextval("),
		}
		structures = append(structures, structure)
	}

	var results []map[string]interface{}
	for rows.Next() {
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return nil, nil, fmt.Errorf("error scanning row: %v", err)
		}
		results = append(results, row)
	}

	return structures, results, err
}
