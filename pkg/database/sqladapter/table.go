package sqladapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"rollingthunder/pkg/database"
)

type Dialect struct {
	QuoteIdentifier            func(string) string
	QuoteQualified             func(string, string) string
	Placeholder                func(int) string
	Pagination                 func(int, int) (string, error)
	RequiresOrderForPagination bool
	PaginationFallbackOrder    string
	SupportsNullOrdering       bool
	TextExpression             func(string) string
	NullOrderExpression        func(string, database.NullsPosition) string
	InsertExport               *InsertExportDialect
	IdentityInsertStatements   func(database.Table) (string, string)
}

func structureNameSet(
	structures database.Structures,
) map[string]string {
	result := make(map[string]string, len(structures))
	for _, structure := range structures {
		result[strings.ToLower(structure.Name)] = structure.Name
	}
	return result
}

func resolveColumn(
	columns map[string]string,
	name string,
) (string, error) {
	resolved, ok := columns[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return "", fmt.Errorf("unknown column %q", name)
	}
	return resolved, nil
}

func BuildFilterClause(
	filters []database.Filter,
	structures database.Structures,
	dialect Dialect,
) (string, []interface{}, error) {
	if len(filters) == 0 {
		return "", nil, nil
	}
	columns := structureNameSet(structures)
	clauses := make([]string, 0, len(filters))
	args := make([]interface{}, 0, len(filters))
	for _, filter := range filters {
		if err := filter.Validate(); err != nil {
			return "", nil, err
		}
		column, err := resolveColumn(columns, filter.Column)
		if err != nil {
			return "", nil, err
		}
		quoted := dialect.QuoteIdentifier(column)
		switch filter.Operator {
		case database.FilterIsNull:
			clauses = append(clauses, quoted+" IS NULL")
		case database.FilterIsNotNull:
			clauses = append(clauses, quoted+" IS NOT NULL")
		default:
			operator := map[database.FilterOperator]string{
				database.FilterEqual:        "=",
				database.FilterNotEqual:     "<>",
				database.FilterGreaterThan:  ">",
				database.FilterLessThan:     "<",
				database.FilterGreaterEqual: ">=",
				database.FilterLessEqual:    "<=",
				database.FilterContains:     "LIKE",
			}[filter.Operator]
			if operator == "" {
				return "", nil, fmt.Errorf(
					"unsupported filter operator %q",
					filter.Operator,
				)
			}
			value := filter.Value
			if filter.Operator == database.FilterContains {
				value = "%" + fmt.Sprint(value) + "%"
				if dialect.TextExpression != nil {
					quoted = dialect.TextExpression(quoted)
				}
			}
			args = append(args, value)
			clauses = append(
				clauses,
				quoted+" "+operator+" "+dialect.Placeholder(len(args)),
			)
		}
	}
	return " WHERE " + strings.Join(clauses, " AND "), args, nil
}

func BuildOrderClause(
	sorts []database.Sort,
	structures database.Structures,
	dialect Dialect,
) (string, error) {
	columns := structureNameSet(structures)
	parts := make([]string, 0, len(sorts))
	for _, item := range sorts {
		column, err := resolveColumn(columns, item.Column)
		if err != nil {
			return "", err
		}
		direction := strings.ToUpper(string(item.Direction))
		if direction == "" {
			direction = "ASC"
		}
		if direction != "ASC" && direction != "DESC" {
			return "", fmt.Errorf("unsupported sort direction %q", item.Direction)
		}
		quoted := dialect.QuoteIdentifier(column)
		part := quoted + " " + direction
		if item.Nulls != "" {
			switch item.Nulls {
			case database.NullsFirst:
			case database.NullsLast:
			default:
				return "", fmt.Errorf("unsupported NULL position %q", item.Nulls)
			}
			if dialect.SupportsNullOrdering {
				if item.Nulls == database.NullsFirst {
					part += " NULLS FIRST"
				} else {
					part += " NULLS LAST"
				}
			} else if dialect.NullOrderExpression != nil {
				parts = append(
					parts,
					dialect.NullOrderExpression(quoted, item.Nulls),
				)
			} else {
				return "", fmt.Errorf("this engine does not support explicit NULL ordering")
			}
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " ORDER BY " + strings.Join(parts, ", "), nil
}

func BuildTableSelect(
	table database.Table,
	structures database.Structures,
	projection string,
	dialect Dialect,
	includePagination bool,
) (string, []interface{}, error) {
	if strings.TrimSpace(table.Name) == "" {
		return "", nil, errors.New("table name is required")
	}
	filters, args, err := BuildFilterClause(table.Filters, structures, dialect)
	if err != nil {
		return "", nil, err
	}
	order, err := BuildOrderClause(table.Sorts, structures, dialect)
	if err != nil {
		return "", nil, err
	}
	if includePagination && table.Limit > 0 &&
		order == "" && dialect.RequiresOrderForPagination {
		primaryKeys := make([]string, 0)
		for _, structure := range structures {
			if structure.IsPrimary {
				primaryKeys = append(
					primaryKeys,
					dialect.QuoteIdentifier(structure.Name)+" ASC",
				)
			}
		}
		if len(primaryKeys) > 0 {
			order = " ORDER BY " + strings.Join(primaryKeys, ", ")
		} else {
			order = dialect.PaginationFallbackOrder
		}
		if order == "" {
			return "", nil, errors.New(
				"this engine requires an ORDER BY clause for pagination",
			)
		}
	}
	query := "SELECT " + projection + " FROM " +
		dialect.QuoteQualified(table.Schema, table.Name) +
		filters + order
	if includePagination && table.Limit > 0 {
		pagination, paginationErr := dialect.Pagination(
			table.Limit,
			max(0, table.Offset),
		)
		if paginationErr != nil {
			return "", nil, paginationErr
		}
		query += " " + pagination
	}
	return query, args, nil
}

func CountTable(
	db *sql.DB,
	table database.Table,
	structures database.Structures,
	dialect Dialect,
) (int, error) {
	table.Sorts = nil
	query, args, err := BuildTableSelect(
		table,
		structures,
		"COUNT(*)",
		dialect,
		false,
	)
	if err != nil {
		return 0, err
	}
	var count int
	err = db.QueryRow(query, args...).Scan(&count)
	return count, err
}

func GetTableData(
	ctx context.Context,
	db *sql.DB,
	table database.Table,
	structures database.Structures,
	dialect Dialect,
) ([]map[string]interface{}, error) {
	query, args, err := BuildTableSelect(
		table,
		structures,
		"*",
		dialect,
		true,
	)
	if err != nil {
		return nil, err
	}
	result, err := ExecuteQuery(ctx, db, query, database.QueryOptions{
		Args: args,
	})
	return result.Rows, err
}

func mutableDataKeys(data map[string]interface{}) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		if key == "_isNew" || strings.HasPrefix(key, "temp_") ||
			strings.HasPrefix(key, "__rolling_thunder_") {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func dataValue(
	data map[string]interface{},
	column string,
) (interface{}, bool, error) {
	var (
		value interface{}
		found bool
	)
	for key, candidate := range data {
		if !strings.EqualFold(key, column) {
			continue
		}
		if found {
			return nil, false, fmt.Errorf(
				"row contains ambiguous values for column %q",
				column,
			)
		}
		value = candidate
		found = true
	}
	return value, found, nil
}

func InsertRow(
	executor interface {
		Exec(string, ...interface{}) (sql.Result, error)
	},
	table database.Table,
	data map[string]interface{},
	dialect Dialect,
) error {
	return InsertRowWithStructures(
		executor,
		table,
		data,
		nil,
		dialect,
	)
}

func InsertRowWithStructures(
	executor interface {
		Exec(string, ...interface{}) (sql.Result, error)
	},
	table database.Table,
	data map[string]interface{},
	structures database.Structures,
	dialect Dialect,
) error {
	keys := mutableDataKeys(data)
	if len(keys) == 0 {
		return errors.New("no data to insert")
	}
	known := make(map[string]database.Structure, len(structures))
	for _, structure := range structures {
		known[strings.ToLower(structure.Name)] = structure
	}
	columns := make([]string, 0, len(keys))
	placeholders := make([]string, 0, len(keys))
	values := make([]interface{}, 0, len(keys))
	insertedColumns := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		column := key
		if len(known) > 0 {
			structure, exists := known[strings.ToLower(key)]
			if !exists {
				return fmt.Errorf("unknown insert column %q", key)
			}
			identity := structure.IsAutoInc ||
				strings.HasPrefix(
					strings.ToUpper(strings.TrimSpace(structure.Generation)),
					"IDENTITY",
				)
			if structure.IsGenerated && !identity {
				continue
			}
			if identity && data[key] == nil {
				continue
			}
			column = structure.Name
		}
		if len(known) == 0 &&
			strings.EqualFold(key, "id") &&
			data[key] == nil {
			continue
		}
		columnKey := strings.ToLower(column)
		if _, duplicate := insertedColumns[columnKey]; duplicate {
			return fmt.Errorf(
				"insert column %q is provided more than once",
				column,
			)
		}
		insertedColumns[columnKey] = struct{}{}
		columns = append(columns, dialect.QuoteIdentifier(column))
		values = append(values, data[key])
		placeholders = append(placeholders, dialect.Placeholder(len(values)))
	}
	if len(columns) == 0 {
		return errors.New("no insertable data")
	}
	query := "INSERT INTO " +
		dialect.QuoteQualified(table.Schema, table.Name) +
		" (" + strings.Join(columns, ", ") + ") VALUES (" +
		strings.Join(placeholders, ", ") + ")"
	_, err := executor.Exec(query, values...)
	return err
}

func UpdateRow(
	executor interface {
		Exec(string, ...interface{}) (sql.Result, error)
	},
	table database.Table,
	data map[string]interface{},
	primaryKey string,
	dialect Dialect,
) error {
	return UpdateRowWithStructures(
		executor,
		table,
		data,
		primaryKey,
		nil,
		dialect,
	)
}

func UpdateRowWithStructures(
	executor interface {
		Exec(string, ...interface{}) (sql.Result, error)
	},
	table database.Table,
	data map[string]interface{},
	primaryKey string,
	structures database.Structures,
	dialect Dialect,
) error {
	primaryKey = strings.TrimSpace(primaryKey)
	if primaryKey == "" {
		return errors.New("a primary key is required for row updates")
	}
	primaryValue, ok, err := dataValue(data, primaryKey)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("primary key %q is missing", primaryKey)
	}
	keys := mutableDataKeys(data)
	known := make(map[string]database.Structure, len(structures))
	for _, structure := range structures {
		known[strings.ToLower(structure.Name)] = structure
	}
	if len(known) > 0 {
		structure, exists := known[strings.ToLower(primaryKey)]
		if !exists {
			return fmt.Errorf("unknown primary key column %q", primaryKey)
		}
		primaryKey = structure.Name
	}
	clauses := make([]string, 0, len(keys))
	values := make([]interface{}, 0, len(keys))
	updatedColumns := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		column := key
		if len(known) > 0 {
			structure, exists := known[strings.ToLower(key)]
			if !exists {
				return fmt.Errorf("unknown update column %q", key)
			}
			if structure.IsGenerated || structure.IsAutoInc {
				continue
			}
			column = structure.Name
		}
		if strings.EqualFold(column, primaryKey) {
			continue
		}
		columnKey := strings.ToLower(column)
		if _, duplicate := updatedColumns[columnKey]; duplicate {
			return fmt.Errorf(
				"update column %q is provided more than once",
				column,
			)
		}
		updatedColumns[columnKey] = struct{}{}
		values = append(values, data[key])
		clauses = append(
			clauses,
			dialect.QuoteIdentifier(column)+" = "+dialect.Placeholder(len(values)),
		)
	}
	if len(clauses) == 0 {
		return errors.New("no mutable columns to update")
	}
	values = append(values, primaryValue)
	query := "UPDATE " +
		dialect.QuoteQualified(table.Schema, table.Name) +
		" SET " + strings.Join(clauses, ", ") +
		" WHERE " + dialect.QuoteIdentifier(primaryKey) +
		" = " + dialect.Placeholder(len(values))
	result, err := executor.Exec(query, values...)
	if err != nil {
		return err
	}
	return requireSingleAffectedRow(result, "update")
}

func DeleteRow(
	executor interface {
		Exec(string, ...interface{}) (sql.Result, error)
	},
	table database.Table,
	primaryKey string,
	primaryValue interface{},
	dialect Dialect,
) error {
	primaryKey = strings.TrimSpace(primaryKey)
	if primaryKey == "" {
		return errors.New("a primary key is required for row deletion")
	}
	query := "DELETE FROM " +
		dialect.QuoteQualified(table.Schema, table.Name) +
		" WHERE " + dialect.QuoteIdentifier(primaryKey) +
		" = " + dialect.Placeholder(1)
	result, err := executor.Exec(query, primaryValue)
	if err != nil {
		return err
	}
	return requireSingleAffectedRow(result, "delete")
}

func requireSingleAffectedRow(result sql.Result, action string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf(
			"row %s affected %d rows instead of exactly one",
			action,
			affected,
		)
	}
	return nil
}

func primaryKeyColumns(
	structures database.Structures,
) []string {
	keys := make([]string, 0)
	for _, structure := range structures {
		if structure.IsPrimary {
			keys = append(keys, structure.Name)
		}
	}
	return keys
}

func whereByKeys(
	original map[string]interface{},
	keys []string,
	start int,
	dialect Dialect,
) (string, []interface{}, error) {
	if len(keys) == 0 {
		return "", nil, errors.New("table changes require a primary key")
	}
	clauses := make([]string, 0, len(keys))
	values := make([]interface{}, 0, len(keys))
	for _, key := range keys {
		value, exists, err := dataValue(original, key)
		if err != nil {
			return "", nil, err
		}
		if !exists {
			return "", nil, fmt.Errorf("primary key %q is missing", key)
		}
		values = append(values, value)
		clauses = append(
			clauses,
			dialect.QuoteIdentifier(key)+" = "+
				dialect.Placeholder(start+len(values)-1),
		)
	}
	return strings.Join(clauses, " AND "), values, nil
}

func ApplyTableChanges(
	ctx context.Context,
	db *sql.DB,
	changes database.TableChangeSet,
	structures database.Structures,
	dialect Dialect,
) (database.TableChangeResult, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return database.TableChangeResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	result := database.TableChangeResult{}
	identityDisable := ""
	identityEnabled := false
	if dialect.IdentityInsertStatements != nil &&
		hasExplicitIdentityValues(changes.Added, structures) {
		identityEnable, disable := dialect.IdentityInsertStatements(
			changes.Table,
		)
		if strings.TrimSpace(identityEnable) == "" ||
			strings.TrimSpace(disable) == "" {
			return database.TableChangeResult{}, errors.New(
				"driver returned incomplete identity-insert statements",
			)
		}
		if _, err := tx.ExecContext(ctx, identityEnable); err != nil {
			return database.TableChangeResult{}, fmt.Errorf(
				"enable explicit identity inserts: %w",
				err,
			)
		}
		identityDisable = disable
		identityEnabled = true
		defer func() {
			if identityEnabled {
				_, _ = tx.ExecContext(context.Background(), identityDisable)
			}
		}()
	}
	for _, row := range changes.Added {
		if err := InsertRowWithStructures(
			tx,
			changes.Table,
			row,
			structures,
			dialect,
		); err != nil {
			return database.TableChangeResult{}, err
		}
		result.Inserted++
	}
	if identityEnabled {
		if _, err := tx.ExecContext(ctx, identityDisable); err != nil {
			return database.TableChangeResult{}, fmt.Errorf(
				"disable explicit identity inserts: %w",
				err,
			)
		}
		identityEnabled = false
	}
	keys := primaryKeyColumns(structures)
	knownColumns := structureNameSet(structures)
	structureByName := make(
		map[string]database.Structure,
		len(structures),
	)
	for _, structure := range structures {
		structureByName[strings.ToLower(structure.Name)] = structure
	}
	for _, update := range changes.Updated {
		columns := append([]string(nil), update.ChangedColumns...)
		sort.Strings(columns)
		set := make([]string, 0, len(columns))
		values := make([]interface{}, 0, len(columns)+len(keys))
		updatedColumns := make(map[string]struct{}, len(columns))
		for _, column := range columns {
			resolved, resolveErr := resolveColumn(
				knownColumns,
				column,
			)
			if resolveErr != nil {
				return database.TableChangeResult{}, resolveErr
			}
			structure := structureByName[strings.ToLower(resolved)]
			if structure.IsGenerated || structure.IsAutoInc {
				return database.TableChangeResult{}, fmt.Errorf(
					"generated or identity column %q cannot be updated",
					resolved,
				)
			}
			resolvedKey := strings.ToLower(resolved)
			if _, duplicate := updatedColumns[resolvedKey]; duplicate {
				return database.TableChangeResult{}, fmt.Errorf(
					"updated column %q is listed more than once",
					resolved,
				)
			}
			updatedColumns[resolvedKey] = struct{}{}
			value, exists, valueErr := dataValue(update.Values, resolved)
			if valueErr != nil {
				return database.TableChangeResult{}, valueErr
			}
			if !exists {
				return database.TableChangeResult{}, fmt.Errorf(
					"updated value for column %q is missing",
					resolved,
				)
			}
			values = append(values, value)
			set = append(
				set,
				dialect.QuoteIdentifier(resolved)+" = "+
					dialect.Placeholder(len(values)),
			)
		}
		if len(set) == 0 {
			continue
		}
		where, keyValues, whereErr := whereByKeys(
			update.Original,
			keys,
			len(values)+1,
			dialect,
		)
		if whereErr != nil {
			return database.TableChangeResult{}, whereErr
		}
		values = append(values, keyValues...)
		query := "UPDATE " +
			dialect.QuoteQualified(changes.Table.Schema, changes.Table.Name) +
			" SET " + strings.Join(set, ", ") + " WHERE " + where
		execResult, execErr := tx.ExecContext(ctx, query, values...)
		if execErr != nil {
			return database.TableChangeResult{}, execErr
		}
		if err := requireSingleAffectedRow(execResult, "update"); err != nil {
			return database.TableChangeResult{}, err
		}
		result.Updated++
	}
	for _, row := range changes.Deleted {
		where, values, whereErr := whereByKeys(row, keys, 1, dialect)
		if whereErr != nil {
			return database.TableChangeResult{}, whereErr
		}
		query := "DELETE FROM " +
			dialect.QuoteQualified(changes.Table.Schema, changes.Table.Name) +
			" WHERE " + where
		execResult, execErr := tx.ExecContext(ctx, query, values...)
		if execErr != nil {
			return database.TableChangeResult{}, execErr
		}
		if err := requireSingleAffectedRow(execResult, "delete"); err != nil {
			return database.TableChangeResult{}, err
		}
		result.Deleted++
	}
	if err := tx.Commit(); err != nil {
		return database.TableChangeResult{}, err
	}
	committed = true
	return result, nil
}

func hasExplicitIdentityValues(
	rows []map[string]interface{},
	structures database.Structures,
) bool {
	identities := make([]string, 0)
	for _, structure := range structures {
		if structure.IsAutoInc ||
			strings.HasPrefix(
				strings.ToUpper(strings.TrimSpace(structure.Generation)),
				"IDENTITY",
			) {
			identities = append(identities, structure.Name)
		}
	}
	for _, row := range rows {
		for _, column := range identities {
			value, exists, err := dataValue(row, column)
			if err == nil && exists && value != nil {
				return true
			}
		}
	}
	return false
}

func ExportTable(
	ctx context.Context,
	db *sql.DB,
	request database.TableExportRequest,
	structures database.Structures,
	dialect Dialect,
	writer io.Writer,
) (database.ExportStats, error) {
	if err := database.ValidateExportOptions(request.Options); err != nil {
		return database.ExportStats{}, err
	}
	if request.Options.Format == database.ExportFormatSQL &&
		dialect.InsertExport == nil {
		return database.ExportStats{}, errors.New(
			"SQL INSERT export is not supported by this driver yet",
		)
	}
	includePagination := request.Scope == database.ExportScopePage ||
		request.Scope == database.ExportScopeSelected
	if request.Scope != database.ExportScopeAll && !includePagination {
		return database.ExportStats{}, fmt.Errorf(
			"unsupported table export scope %q",
			request.Scope,
		)
	}
	projection := "*"
	insertColumns := make(database.Structures, 0, len(structures))
	if request.Options.Format == database.ExportFormatSQL {
		parts := make([]string, 0, len(structures))
		for _, column := range structures {
			if column.IsGenerated || column.IsAutoInc {
				continue
			}
			insertColumns = append(insertColumns, column)
			parts = append(
				parts,
				dialect.QuoteIdentifier(column.Name),
			)
		}
		if len(parts) == 0 {
			return database.ExportStats{}, errors.New(
				"table has no columns that can be exported as INSERT statements",
			)
		}
		projection = strings.Join(parts, ", ")
	}
	query, args, err := BuildTableSelect(
		request.Table,
		structures,
		projection,
		dialect,
		includePagination,
	)
	if err != nil {
		return database.ExportStats{}, err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return database.ExportStats{}, err
	}
	defer rows.Close()
	var stream database.RowStream = NewRows(rows)
	if request.Scope == database.ExportScopeSelected {
		stream, err = NewSelectedRows(
			stream,
			request.SelectedRowIndexes,
			request.Table.Limit,
		)
		if err != nil {
			return database.ExportStats{}, err
		}
	}
	if request.Options.Format == database.ExportFormatSQL {
		return WriteInsertExportContext(
			ctx,
			writer,
			stream,
			request.Table,
			insertColumns,
			request.Options.SQL,
			*dialect.InsertExport,
		)
	}
	return database.WriteExportStreamContext(
		ctx,
		writer,
		stream,
		request.Options,
	)
}
