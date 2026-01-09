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
	Host        string
	Port        string
	User        string
	Password    string
	Db          string
	SSLMode     string
	SSLRootCert string
	SSLCert     string
	SSLKey      string
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

	dsn := []string{"dbname=" + p.cfg.Db}

	// SSL Mode
	sslMode := "disable"
	if p.cfg.SSLMode != "" {
		sslMode = p.cfg.SSLMode
	}
	dsn = append(dsn, fmt.Sprintf("sslmode=%s", sslMode))

	// SSL Certificates
	if p.cfg.SSLRootCert != "" {
		dsn = append(dsn, fmt.Sprintf("sslrootcert=%s", p.cfg.SSLRootCert))
	}
	if p.cfg.SSLCert != "" {
		dsn = append(dsn, fmt.Sprintf("sslcert=%s", p.cfg.SSLCert))
	}
	if p.cfg.SSLKey != "" {
		dsn = append(dsn, fmt.Sprintf("sslkey=%s", p.cfg.SSLKey))
	}

	if p.cfg.User != "" {
		dsn = append(dsn, fmt.Sprintf("user=%s", p.cfg.User))
	}

	if p.cfg.Password != "" {
		dsn = append(dsn, fmt.Sprintf("password=%s", p.cfg.Password))
	}

	host := "localhost"
	if p.cfg.Host != "" {
		host = p.cfg.Host
	}
	dsn = append(dsn, fmt.Sprintf("host=%s", host))

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
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", table.Schema, table.Name)
	if table.Filter != "" {
		query += fmt.Sprintf(" WHERE %s", table.Filter)
	}
	err := p.conn.Get(&result, query)
	return result, err
}

func (p *Postgres) GetCollectionData(table database.Table) (database.Structures, []map[string]interface{}, error) {
	query := fmt.Sprintf(`SELECT * FROM %s.%s`, table.Schema, table.Name)
	if table.Filter != "" {
		query += fmt.Sprintf(" WHERE %s", table.Filter)
	}
	query += fmt.Sprintf(" LIMIT %d OFFSET %d", table.Limit, table.Offset)
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

// InsertRow inserts a new row into the table
func (p *Postgres) InsertRow(table database.Table, data map[string]interface{}) error {
	if len(data) == 0 {
		return errors.New("no data to insert")
	}

	columns := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data))
	i := 1

	for col, val := range data {
		// Skip internal fields
		if col == "id" && val == nil {
			continue
		}
		if col == "_isNew" || strings.HasPrefix(col, "temp_") {
			continue
		}
		columns = append(columns, fmt.Sprintf(`"%s"`, col))
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		values = append(values, val)
		i++
	}

	query := fmt.Sprintf(
		`INSERT INTO %s.%s (%s) VALUES (%s)`,
		table.Schema,
		table.Name,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	_, err := p.conn.Exec(query, values...)
	return err
}

// UpdateRow updates an existing row in the table
func (p *Postgres) UpdateRow(table database.Table, data map[string]interface{}, primaryKey string) error {
	if len(data) == 0 {
		return errors.New("no data to update")
	}

	primaryValue, ok := data[primaryKey]
	if !ok {
		return fmt.Errorf("primary key '%s' not found in data", primaryKey)
	}

	setClauses := make([]string, 0, len(data))
	values := make([]interface{}, 0, len(data))
	i := 1

	for col, val := range data {
		if col == primaryKey {
			continue
		}
		// Skip internal fields
		if col == "_isNew" || strings.HasPrefix(col, "temp_") {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf(`"%s" = $%d`, col, i))
		values = append(values, val)
		i++
	}

	values = append(values, primaryValue)
	query := fmt.Sprintf(
		`UPDATE %s.%s SET %s WHERE "%s" = $%d`,
		table.Schema,
		table.Name,
		strings.Join(setClauses, ", "),
		primaryKey,
		i,
	)

	_, err := p.conn.Exec(query, values...)
	return err
}

// DeleteRow deletes a row from the table by primary key
func (p *Postgres) DeleteRow(table database.Table, primaryKey string, primaryValue interface{}) error {
	query := fmt.Sprintf(
		`DELETE FROM %s.%s WHERE "%s" = $1`,
		table.Schema,
		table.Name,
		primaryKey,
	)

	_, err := p.conn.Exec(query, primaryValue)
	return err
}

// ExecuteQuery executes a raw SQL query and returns results
func (p *Postgres) ExecuteQuery(query string) ([]map[string]interface{}, error) {
	rows, err := p.conn.Queryx(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return nil, fmt.Errorf("error scanning row: %v", err)
		}
		results = append(results, row)
	}

	return results, nil
}

// CreateTable creates a new table in the database
func (p *Postgres) CreateTable(table database.Table, columns []database.ColumnDefinition) error {
	// Validate table name
	if strings.TrimSpace(table.Name) == "" {
		return errors.New("table name is required")
	}
	// Validate schema
	schema := table.Schema
	if strings.TrimSpace(schema) == "" {
		schema = "public"
	}

	if len(columns) == 0 {
		return errors.New("at least one column is required")
	}

	var colDefs []string
	var primaryKeys []string

	for _, col := range columns {
		// Skip columns with empty names
		if strings.TrimSpace(col.Name) == "" {
			continue
		}

		def := fmt.Sprintf(`"%s" %s`, strings.TrimSpace(col.Name), col.Type)

		if !col.Nullable {
			def += " NOT NULL"
		}

		if col.Default != "" {
			def += fmt.Sprintf(" DEFAULT %s", col.Default)
		}

		if col.Unique {
			def += " UNIQUE"
		}

		if col.PrimaryKey {
			primaryKeys = append(primaryKeys, fmt.Sprintf(`"%s"`, strings.TrimSpace(col.Name)))
		}

		colDefs = append(colDefs, def)
	}

	// Validate we have at least one valid column
	if len(colDefs) == 0 {
		return errors.New("at least one column with a name is required")
	}

	// Add primary key constraint if any
	if len(primaryKeys) > 0 {
		colDefs = append(colDefs, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(primaryKeys, ", ")))
	}

	query := fmt.Sprintf(`CREATE TABLE "%s"."%s" (%s)`, schema, strings.TrimSpace(table.Name), strings.Join(colDefs, ", "))

	_, err := p.conn.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create table: %v", err)
	}

	return nil
}

// DropTable drops a table from the database
func (p *Postgres) DropTable(table database.Table) error {
	if strings.TrimSpace(table.Name) == "" {
		return errors.New("table name is required")
	}

	schema := table.Schema
	if strings.TrimSpace(schema) == "" {
		schema = "public"
	}

	query := fmt.Sprintf(`DROP TABLE IF EXISTS "%s"."%s"`, schema, strings.TrimSpace(table.Name))
	_, err := p.conn.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to drop table: %v", err)
	}

	return nil
}

// TruncateTable removes all rows from a table
func (p *Postgres) TruncateTable(table database.Table) error {
	if strings.TrimSpace(table.Name) == "" {
		return errors.New("table name is required")
	}

	schema := table.Schema
	if strings.TrimSpace(schema) == "" {
		schema = "public"
	}

	query := fmt.Sprintf(`TRUNCATE TABLE "%s"."%s" CASCADE`, schema, strings.TrimSpace(table.Name))
	_, err := p.conn.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to truncate table: %v", err)
	}

	return nil
}

// GetDataTypes returns available PostgreSQL data types
func (p *Postgres) GetDataTypes() []database.DataType {
	return []database.DataType{
		// Numeric Types
		{Name: "smallint", Category: "Numeric", Description: "2 bytes, -32768 to 32767"},
		{Name: "integer", Category: "Numeric", Description: "4 bytes, -2147483648 to 2147483647"},
		{Name: "bigint", Category: "Numeric", Description: "8 bytes, large range"},
		{Name: "decimal", Category: "Numeric", Description: "Variable precision"},
		{Name: "numeric", Category: "Numeric", Description: "Variable precision"},
		{Name: "real", Category: "Numeric", Description: "4 bytes floating-point"},
		{Name: "double precision", Category: "Numeric", Description: "8 bytes floating-point"},
		{Name: "smallserial", Category: "Numeric", Description: "Auto-increment 2 bytes"},
		{Name: "serial", Category: "Numeric", Description: "Auto-increment 4 bytes"},
		{Name: "bigserial", Category: "Numeric", Description: "Auto-increment 8 bytes"},

		// Character Types
		{Name: "varchar", Category: "Character", Description: "Variable length with limit"},
		{Name: "char", Category: "Character", Description: "Fixed length, blank padded"},
		{Name: "text", Category: "Character", Description: "Variable unlimited length"},

		// Binary Types
		{Name: "bytea", Category: "Binary", Description: "Binary data"},

		// Date/Time Types
		{Name: "date", Category: "Date/Time", Description: "Date only"},
		{Name: "time", Category: "Date/Time", Description: "Time of day"},
		{Name: "time with time zone", Category: "Date/Time", Description: "Time with timezone"},
		{Name: "timestamp", Category: "Date/Time", Description: "Date and time"},
		{Name: "timestamp with time zone", Category: "Date/Time", Description: "Date and time with timezone"},
		{Name: "interval", Category: "Date/Time", Description: "Time interval"},

		// Boolean
		{Name: "boolean", Category: "Boolean", Description: "true/false"},

		// UUID
		{Name: "uuid", Category: "UUID", Description: "Universally unique identifier"},

		// JSON Types
		{Name: "json", Category: "JSON", Description: "JSON data"},
		{Name: "jsonb", Category: "JSON", Description: "Binary JSON (faster)"},

		// Array Types
		{Name: "integer[]", Category: "Array", Description: "Array of integers"},
		{Name: "text[]", Category: "Array", Description: "Array of text"},

		// Network Types
		{Name: "inet", Category: "Network", Description: "IPv4/IPv6 host address"},
		{Name: "cidr", Category: "Network", Description: "IPv4/IPv6 network"},
		{Name: "macaddr", Category: "Network", Description: "MAC address"},

		// Geometric Types
		{Name: "point", Category: "Geometric", Description: "Point on plane"},
		{Name: "line", Category: "Geometric", Description: "Infinite line"},
		{Name: "box", Category: "Geometric", Description: "Rectangular box"},
		{Name: "circle", Category: "Geometric", Description: "Circle"},

		// Other
		{Name: "money", Category: "Monetary", Description: "Currency amount"},
		{Name: "xml", Category: "XML", Description: "XML data"},
		{Name: "tsquery", Category: "Full Text", Description: "Text search query"},
		{Name: "tsvector", Category: "Full Text", Description: "Text search document"},
	}
}
