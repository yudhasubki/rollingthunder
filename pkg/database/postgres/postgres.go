package postgres

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"rollingthunder/pkg/database"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

type Config struct {
	Host          string
	Port          string
	User          string
	Password      string
	Db            string
	SSLMode       string
	SSLRootCert   string
	SSLCert       string
	SSLKey        string
	TLSServerName string
}

type Postgres struct {
	cfg    Config
	ctx    context.Context
	conn   *sqlx.DB
	pool   *pgxpool.Pool
	engine string
}

func NewPostgres(ctx context.Context, cfg Config) *Postgres {
	return &Postgres{
		cfg:    cfg,
		ctx:    ctx,
		engine: "PostgreSQL",
	}
}

func applyPostgresTLSServerName(
	config *pgxpool.Config,
	serverName string,
) {
	serverName = strings.TrimSpace(serverName)
	if config == nil || serverName == "" {
		return
	}
	if config.ConnConfig.TLSConfig != nil {
		config.ConnConfig.TLSConfig = config.ConnConfig.TLSConfig.Clone()
		config.ConnConfig.TLSConfig.ServerName = serverName
	}
	for _, fallback := range config.ConnConfig.Fallbacks {
		if fallback.TLSConfig == nil {
			continue
		}
		fallback.TLSConfig = fallback.TLSConfig.Clone()
		fallback.TLSConfig.ServerName = serverName
	}
}

func quotePostgresConfigValue(value string) string {
	escaped := strings.NewReplacer(
		`\`, `\\`,
		`'`, `\'`,
	).Replace(value)
	return "'" + escaped + "'"
}

func buildPostgresPoolConfig(config Config) (*pgxpool.Config, error) {
	sslMode := strings.TrimSpace(config.SSLMode)
	if sslMode == "" {
		sslMode = "disable"
	}
	host := strings.TrimSpace(config.Host)
	if host == "" {
		host = "localhost"
	}
	port := strings.TrimSpace(config.Port)
	if port == "" {
		port = "5432"
	}

	setting := func(name string, value string) string {
		return name + "=" + quotePostgresConfigValue(value)
	}
	dsn := []string{
		setting("dbname", config.Db),
		setting("application_name", "Rolling Thunder"),
		setting("sslmode", sslMode),
		setting("host", host),
		setting("port", port),
	}
	if config.SSLRootCert != "" {
		dsn = append(dsn, setting("sslrootcert", config.SSLRootCert))
	}
	if config.SSLCert != "" {
		dsn = append(dsn, setting("sslcert", config.SSLCert))
	}
	if config.SSLKey != "" {
		dsn = append(dsn, setting("sslkey", config.SSLKey))
	}
	if config.User != "" {
		dsn = append(dsn, setting("user", config.User))
	}
	if config.Password != "" {
		dsn = append(dsn, setting("password", config.Password))
	}

	poolConfig, err := pgxpool.ParseConfig(strings.Join(dsn, " "))
	if err != nil {
		return nil, err
	}
	applyPostgresTLSServerName(poolConfig, config.TLSServerName)
	return poolConfig, nil
}

func (p *Postgres) Connect(ctx context.Context) error {
	if p.cfg.Db == "" {
		return errors.New("database is not exists")
	}
	if ctx == nil {
		ctx = p.ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}

	poolConfig, err := buildPostgresPoolConfig(p.cfg)
	if err != nil {
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return err
	}

	p.pool = pool
	p.conn = sqlx.NewDb(stdlib.OpenDBFromPool(pool), "pgx")
	return nil
}

func (p *Postgres) Close() error {
	var closeErr error
	if p.conn != nil {
		closeErr = p.conn.Close()
		p.conn = nil
	}
	if p.pool != nil {
		p.pool.Close()
		p.pool = nil
	}
	return closeErr
}

func (p *Postgres) Ping(ctx context.Context) error {
	if p.conn == nil {
		return fmt.Errorf("PostgreSQL connection is not open")
	}
	return p.conn.PingContext(ctx)
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
		ix.indisprimary AS is_primary,
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

	ref := quotePostgresQualifiedIdentifier(table.Schema, table.Name)

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
				IsPrimary: index.IsPrimary,
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

func (p *Postgres) GetConstraints(table database.Table) (Constraints, error) {
	const query = `
		SELECT
			a.attname AS column,
			c.contype::text AS type,
			foreign_namespace.nspname AS foreign_schema,
			foreign_table.relname AS foreign_table,
			CASE
				WHEN c.contype = 'f' THEN foreign_attribute.attname
			END AS foreign_column
		FROM pg_constraint c
		JOIN unnest(c.conkey) WITH ORDINALITY AS local_key(attnum, ord) ON TRUE
		JOIN pg_attribute a
			ON a.attrelid = c.conrelid
			AND a.attnum = local_key.attnum
		LEFT JOIN pg_class foreign_table
			ON c.contype = 'f'
			AND foreign_table.oid = c.confrelid
		LEFT JOIN pg_namespace foreign_namespace
			ON foreign_namespace.oid = foreign_table.relnamespace
		LEFT JOIN unnest(c.confkey) WITH ORDINALITY AS foreign_key(attnum, ord)
			ON foreign_key.ord = local_key.ord
		LEFT JOIN pg_attribute foreign_attribute
			ON foreign_attribute.attrelid = c.confrelid
			AND foreign_attribute.attnum = foreign_key.attnum
		WHERE c.conrelid = $1::regclass
			AND c.contype IN ('p', 'u', 'f')
		ORDER BY c.oid, local_key.ord`

	var out []Constraint
	ref := quotePostgresQualifiedIdentifier(table.Schema, table.Name)
	err := p.conn.Select(&out, query, ref)

	return out, err
}

func (p *Postgres) getCollectionStructures(table database.Table) (Columns, error) {
	var (
		query = `SELECT
			c.column_name,
			c.data_type,
			c.udt_schema,
			c.udt_name,
			EXISTS (
				SELECT 1
				FROM pg_type enum_type
				JOIN pg_namespace enum_namespace
					ON enum_namespace.oid = enum_type.typnamespace
				WHERE enum_namespace.nspname = c.udt_schema
					AND enum_type.typname = c.udt_name
					AND enum_type.typtype = 'e'
			) AS is_enum,
			c.is_nullable,
			c.character_maximum_length,
			c.column_default,
			c.is_identity,
			c.identity_generation,
			c.is_generated,
			EXISTS (
				SELECT 1
				FROM information_schema.table_constraints tc
				JOIN information_schema.key_column_usage kcu
					ON tc.constraint_catalog = kcu.constraint_catalog
					AND tc.constraint_schema = kcu.constraint_schema
					AND tc.constraint_name = kcu.constraint_name
				WHERE tc.constraint_type = 'PRIMARY KEY'
					AND tc.table_schema = c.table_schema
					AND tc.table_name = c.table_name
					AND kcu.column_name = c.column_name
			) AS is_primary
		FROM information_schema.columns c
		WHERE c.table_schema = $1 AND c.table_name = $2
		ORDER BY c.ordinal_position`
	)

	var rows Columns
	err := p.conn.Select(&rows, query, table.Schema, table.Name)
	if err != nil {
		return nil, err
	}

	return rows, nil
}

func (p *Postgres) GetCollectionStructures(table database.Table) (database.Structures, error) {
	constraints, err := p.GetConstraints(table)
	if err != nil {
		return nil, err
	}

	rows, err := p.getCollectionStructures(table)
	if err != nil {
		return nil, err
	}

	out := make(database.Structures, 0, len(rows))
	for _, r := range rows {
		info := database.Structure{
			Name:      r.ColumnName,
			Length:    r.MaxLength,
			Nullable:  r.IsNullable == "YES",
			Default:   r.ColumnDefault,
			IsAutoInc: isAutoIncrementColumn(r),
		}

		applyColumnType(&info, r)
		applyColumnConstraints(&info, r.IsPrimary, constraints)
		out = append(out, info)
	}

	return out, nil
}

func applyColumnType(info *database.Structure, column Column) {
	dataType := column.DataType
	if value, exists := Types[dataType]; exists {
		dataType = value
	}
	info.DataType = dataType

	if column.IsEnum {
		info.DataType = "enum"
		info.IsEnum = true
	}

	if column.IsEnum || strings.EqualFold(column.DataType, "USER-DEFINED") {
		if column.UDTSchema != "" {
			typeSchema := column.UDTSchema
			info.TypeSchema = &typeSchema
		}
		if column.UDTName != "" {
			typeName := column.UDTName
			info.TypeName = &typeName
		}
	}
}

func applyColumnConstraints(info *database.Structure, isPrimary bool, constraints Constraints) {
	if isPrimary {
		info.IsPrimary = true
		info.IsPrimaryLabel = "PRI"
	}

	for _, constraint := range constraints {
		if constraint.Column != info.Name {
			continue
		}

		switch constraint.Type {
		case "p":
			info.IsPrimary = true
			info.IsPrimaryLabel = "PRI"
		case "u":
			info.IsUnique = true
		case "f":
			if constraint.IsForeign() {
				info.ForeignSchema = constraint.ForeignSchema
				info.ForeignTable = constraint.ForeignTable
				info.ForeignColumn = constraint.ForeignCol

				key := fmt.Sprintf("%s(%s)", *constraint.ForeignTable, *constraint.ForeignCol)
				if constraint.ForeignSchema != nil && *constraint.ForeignSchema != "" {
					key = fmt.Sprintf(
						"%s.%s(%s)",
						*constraint.ForeignSchema,
						*constraint.ForeignTable,
						*constraint.ForeignCol,
					)
				}
				info.ForeignKey = &key
			}
		}
	}
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
	columns, err := p.getCollectionStructures(table)
	if err != nil {
		return 0, err
	}
	filterClause, args, err := buildPostgresFilterClause(
		table.Filters,
		structuresFromColumns(columns),
		1,
	)
	if err != nil {
		return 0, err
	}
	query := fmt.Sprintf(
		"SELECT COUNT(*) FROM %s",
		quotePostgresQualifiedIdentifier(table.Schema, table.Name),
	)
	query += filterClause
	err = p.conn.Get(&result, query, args...)
	return result, err
}

func structuresFromColumns(columns Columns) database.Structures {
	structures := make(database.Structures, 0, len(columns))
	for _, column := range columns {
		primaryLabel := ""
		if column.IsPrimary {
			primaryLabel = "PRI"
		}

		structure := database.Structure{
			Name:           column.ColumnName,
			Length:         column.MaxLength,
			Nullable:       column.IsNullable == "YES",
			Default:        column.ColumnDefault,
			IsPrimary:      column.IsPrimary,
			IsPrimaryLabel: primaryLabel,
			IsAutoInc:      isAutoIncrementColumn(column),
		}
		applyColumnType(&structure, column)
		structures = append(structures, structure)
	}
	return structures
}

func isAutoIncrementColumn(column Column) bool {
	return strings.EqualFold(column.IsIdentity, "YES") ||
		(column.ColumnDefault != nil &&
			strings.HasPrefix(*column.ColumnDefault, "nextval("))
}

func (p *Postgres) GetCollectionData(table database.Table) (database.Structures, []map[string]interface{}, error) {
	if table.Limit < 0 {
		return nil, nil, fmt.Errorf("table limit cannot be negative")
	}
	if table.Offset < 0 {
		return nil, nil, fmt.Errorf("table offset cannot be negative")
	}

	columns, err := p.getCollectionStructures(table)
	if err != nil {
		return nil, nil, err
	}
	structures := structuresFromColumns(columns)

	orderClause, err := buildPostgresOrderClause(table.Sorts, structures)
	if err != nil {
		return nil, nil, err
	}

	filterClause, args, err := buildPostgresFilterClause(
		table.Filters,
		structures,
		1,
	)
	if err != nil {
		return nil, nil, err
	}

	query := fmt.Sprintf(
		`SELECT * FROM %s`,
		quotePostgresQualifiedIdentifier(table.Schema, table.Name),
	)
	query += filterClause
	query += orderClause
	query += fmt.Sprintf(" LIMIT %d OFFSET %d", table.Limit, table.Offset)
	rows, err := p.conn.Queryx(query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return nil, nil, fmt.Errorf("error scanning row: %v", err)
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("error reading rows: %w", err)
	}

	return structures, results, nil
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
		columns = append(columns, quotePostgresIdentifier(col))
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		values = append(values, val)
		i++
	}

	query := fmt.Sprintf(
		`INSERT INTO %s (%s) VALUES (%s)`,
		quotePostgresQualifiedIdentifier(table.Schema, table.Name),
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
	primaryKey = strings.TrimSpace(primaryKey)
	if primaryKey == "" {
		return errors.New("a primary key is required for row updates")
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
		setClauses = append(
			setClauses,
			fmt.Sprintf("%s = $%d", quotePostgresIdentifier(col), i),
		)
		values = append(values, val)
		i++
	}
	if len(setClauses) == 0 {
		return errors.New("no mutable columns to update")
	}

	values = append(values, primaryValue)
	query := fmt.Sprintf(
		`UPDATE %s SET %s WHERE %s = $%d`,
		quotePostgresQualifiedIdentifier(table.Schema, table.Name),
		strings.Join(setClauses, ", "),
		quotePostgresIdentifier(primaryKey),
		i,
	)

	result, err := p.conn.Exec(query, values...)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf(
			"row update affected %d rows instead of exactly one",
			affected,
		)
	}
	return nil
}

// DeleteRow deletes a row from the table by primary key
func (p *Postgres) DeleteRow(table database.Table, primaryKey string, primaryValue interface{}) error {
	primaryKey = strings.TrimSpace(primaryKey)
	if primaryKey == "" {
		return errors.New("a primary key is required for row deletion")
	}
	query := fmt.Sprintf(
		`DELETE FROM %s WHERE %s = $1`,
		quotePostgresQualifiedIdentifier(table.Schema, table.Name),
		quotePostgresIdentifier(primaryKey),
	)

	result, err := p.conn.Exec(query, primaryValue)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf(
			"row deletion affected %d rows instead of exactly one",
			affected,
		)
	}
	return nil
}

type mappedRows interface {
	Next() bool
	Columns() ([]string, error)
	MapScan(dest map[string]interface{}) error
	Err() error
}

func collectQueryResults(rows mappedRows, maxRows int) (database.QueryResult, error) {
	columns, err := rows.Columns()
	if err != nil {
		return database.QueryResult{}, fmt.Errorf("read query result columns: %w", err)
	}
	result := database.QueryResult{
		Rows:     make([]map[string]interface{}, 0),
		RowLimit: maxRows,
		Columns:  columns,
	}

	for rows.Next() {
		if maxRows > 0 && len(result.Rows) >= maxRows {
			result.Truncated = true
			break
		}

		row := make(map[string]interface{})
		if err := rows.MapScan(row); err != nil {
			return database.QueryResult{}, fmt.Errorf("error scanning row: %w", err)
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return database.QueryResult{}, fmt.Errorf("error reading query results: %w", err)
	}

	return result, nil
}

type queryContextRunner interface {
	QueryxContext(
		context.Context,
		string,
		...interface{},
	) (*sqlx.Rows, error)
}

func executePostgresQuery(
	ctx context.Context,
	runner queryContextRunner,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	rows, err := runner.QueryxContext(ctx, query, options.Args...)
	if err != nil {
		return database.QueryResult{}, err
	}
	defer rows.Close()

	return collectQueryResults(rows, options.MaxRows)
}

// ExecuteQuery executes a raw SQL query and returns a bounded result set.
func (p *Postgres) ExecuteQuery(
	ctx context.Context,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	return executePostgresQuery(ctx, p.conn, query, options)
}

type postgresTransaction struct {
	tx *sqlx.Tx
}

func (transaction *postgresTransaction) ExecuteQuery(
	ctx context.Context,
	query string,
	options database.QueryOptions,
) (database.QueryResult, error) {
	return executePostgresQuery(ctx, transaction.tx, query, options)
}

func (transaction *postgresTransaction) Commit() error {
	return transaction.tx.Commit()
}

func (transaction *postgresTransaction) Rollback() error {
	return transaction.tx.Rollback()
}

func (p *Postgres) BeginTransaction(
	ctx context.Context,
) (database.Transaction, error) {
	transaction, err := p.conn.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &postgresTransaction{tx: transaction}, nil
}

type sqlxExportRows struct {
	rows *sqlx.Rows
}

func (r *sqlxExportRows) Columns() ([]string, error) {
	return r.rows.Columns()
}

func (r *sqlxExportRows) Next() bool {
	return r.rows.Next()
}

func (r *sqlxExportRows) Values() ([]interface{}, error) {
	return r.rows.SliceScan()
}

func (r *sqlxExportRows) Err() error {
	return r.rows.Err()
}

func buildPostgresExportQuery(
	table database.Table,
	scope database.ExportScope,
	structures database.Structures,
) (postgresQuery, error) {
	return buildPostgresExportQueryWithProjection(table, scope, structures, "*")
}

func buildPostgresExportQueryWithProjection(
	table database.Table,
	scope database.ExportScope,
	structures database.Structures,
	projection string,
) (postgresQuery, error) {
	if strings.TrimSpace(table.Schema) == "" || strings.TrimSpace(table.Name) == "" {
		return postgresQuery{}, fmt.Errorf("schema and table are required")
	}
	if strings.TrimSpace(projection) == "" {
		return postgresQuery{}, fmt.Errorf("export projection is required")
	}
	if table.Offset < 0 {
		return postgresQuery{}, fmt.Errorf("table offset cannot be negative")
	}

	orderClause, err := buildPostgresOrderClause(table.Sorts, structures)
	if err != nil {
		return postgresQuery{}, err
	}
	filterClause, args, err := buildPostgresFilterClause(
		table.Filters,
		structures,
		1,
	)
	if err != nil {
		return postgresQuery{}, err
	}

	query := fmt.Sprintf(
		"SELECT %s FROM %s",
		projection,
		quotePostgresQualifiedIdentifier(table.Schema, table.Name),
	)
	query += filterClause
	query += orderClause

	switch scope {
	case database.ExportScopePage:
		if table.Limit <= 0 {
			return postgresQuery{}, fmt.Errorf("current-page export requires a positive table limit")
		}
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", table.Limit, table.Offset)
	case database.ExportScopeSelected:
		if table.Limit <= 0 {
			return postgresQuery{}, fmt.Errorf("selected-row export requires a positive table limit")
		}
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", table.Limit, table.Offset)
	case database.ExportScopeAll:
	default:
		return postgresQuery{}, fmt.Errorf("unsupported table export scope %q", scope)
	}

	return postgresQuery{SQL: query, Args: args}, nil
}

func (p *Postgres) ExportTable(
	ctx context.Context,
	request database.TableExportRequest,
	writer io.Writer,
) (database.ExportStats, error) {
	if err := database.ValidateExportOptions(request.Options); err != nil {
		return database.ExportStats{}, err
	}

	columns, err := p.getCollectionStructures(request.Table)
	if err != nil {
		return database.ExportStats{}, err
	}
	projection := "*"
	insertColumns := columns
	if request.Options.Format == database.ExportFormatSQL {
		insertColumns = postgresInsertableColumns(columns)
		projection, err = postgresSQLLiteralProjection(insertColumns)
		if err != nil {
			return database.ExportStats{}, err
		}
	}

	query, err := buildPostgresExportQueryWithProjection(
		request.Table,
		request.Scope,
		structuresFromColumns(columns),
		projection,
	)
	if err != nil {
		return database.ExportStats{}, err
	}

	rows, err := p.conn.QueryxContext(ctx, query.SQL, query.Args...)
	if err != nil {
		return database.ExportStats{}, err
	}
	defer rows.Close()

	var exportRows database.RowStream = &sqlxExportRows{rows: rows}
	if request.Scope == database.ExportScopeSelected {
		exportRows, err = newSelectedRowStream(
			exportRows,
			request.SelectedRowIndexes,
			request.Table.Limit,
		)
		if err != nil {
			return database.ExportStats{}, err
		}
	}

	if request.Options.Format == database.ExportFormatSQL {
		return writePostgresInsertStreamContext(
			ctx,
			writer,
			exportRows,
			request.Table,
			insertColumns,
			request.Options.SQL,
		)
	}

	return database.WriteExportStreamContext(ctx, writer, exportRows, request.Options)
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

// GetTableDDL generates CREATE TABLE DDL statement for a table
func (p *Postgres) GetTableDDL(table database.Table) (string, error) {
	if strings.TrimSpace(table.Name) == "" {
		return "", errors.New("table name is required")
	}

	schema := table.Schema
	if strings.TrimSpace(schema) == "" {
		schema = "public"
	}

	// Get columns
	columns, err := p.GetCollectionStructures(table)
	if err != nil {
		return "", fmt.Errorf("failed to get table structure: %v", err)
	}

	if len(columns) == 0 {
		return "", errors.New("table has no columns")
	}

	var ddl strings.Builder
	ddl.WriteString(fmt.Sprintf("CREATE TABLE \"%s\".\"%s\" (\n", schema, table.Name))

	var primaryKeys []string
	for i, col := range columns {
		// Column name and type
		ddl.WriteString(fmt.Sprintf("    \"%s\" %s", col.Name, col.DataType))

		// NOT NULL constraint
		if !col.Nullable {
			ddl.WriteString(" NOT NULL")
		}

		// DEFAULT value (skip auto-increment defaults)
		if col.Default != nil && *col.Default != "" && !col.IsAutoInc {
			ddl.WriteString(fmt.Sprintf(" DEFAULT %s", *col.Default))
		}

		// Track primary keys
		if col.IsPrimary {
			primaryKeys = append(primaryKeys, fmt.Sprintf("\"%s\"", col.Name))
		}

		// Add comma if not last column and no primary key following
		if i < len(columns)-1 || len(primaryKeys) > 0 {
			ddl.WriteString(",")
		}
		ddl.WriteString("\n")
	}

	// Add PRIMARY KEY constraint
	if len(primaryKeys) > 0 {
		ddl.WriteString(fmt.Sprintf("    PRIMARY KEY (%s)\n", strings.Join(primaryKeys, ", ")))
	}

	ddl.WriteString(");")

	return ddl.String(), nil
}
