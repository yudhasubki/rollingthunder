package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

type sqliteMutation struct {
	SQL  string
	Args []interface{}
}

type sqliteMutationExecer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func sqlitePrimaryKeys(structures database.Structures) []string {
	primaryKeys := make([]string, 0)
	for _, structure := range structures {
		if structure.IsPrimary {
			primaryKeys = append(primaryKeys, structure.Name)
		}
	}
	return primaryKeys
}

func sqliteStructureMap(
	structures database.Structures,
) map[string]database.Structure {
	available := make(map[string]database.Structure, len(structures))
	for _, structure := range structures {
		available[structure.Name] = structure
	}
	return available
}

func validateSQLiteMutationFields(
	values map[string]interface{},
	available map[string]database.Structure,
) error {
	for column := range values {
		if column == "_isNew" || strings.HasPrefix(column, "temp_") ||
			strings.HasPrefix(column, "_rt") {
			continue
		}
		if _, exists := available[column]; !exists {
			return fmt.Errorf("unknown table column %q", column)
		}
	}
	return nil
}

func buildSQLiteInsertMutation(
	table database.Table,
	values map[string]interface{},
	structures database.Structures,
) (sqliteMutation, error) {
	if strings.TrimSpace(table.Name) == "" {
		return sqliteMutation{}, fmt.Errorf("table name is required")
	}
	available := sqliteStructureMap(structures)
	if err := validateSQLiteMutationFields(values, available); err != nil {
		return sqliteMutation{}, err
	}
	columns := make([]string, 0, len(values))
	args := make([]interface{}, 0, len(values))
	for _, structure := range structures {
		value, exists := values[structure.Name]
		if !exists || structure.IsGenerated {
			continue
		}
		if value == nil && (structure.IsAutoInc || structure.Default != nil) {
			continue
		}
		columns = append(columns, quoteSQLiteIdentifier(structure.Name))
		args = append(args, value)
	}
	target := quoteSQLiteQualifiedIdentifier(table.Schema, table.Name)
	if len(columns) == 0 {
		return sqliteMutation{SQL: "INSERT INTO " + target + " DEFAULT VALUES"}, nil
	}
	placeholders := make([]string, len(columns))
	for index := range placeholders {
		placeholders[index] = "?"
	}
	return sqliteMutation{
		SQL: fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES (%s)",
			target,
			strings.Join(columns, ", "),
			strings.Join(placeholders, ", "),
		),
		Args: args,
	}, nil
}

func buildSQLiteUpdateMutation(
	table database.Table,
	update database.RowUpdate,
	structures database.Structures,
	primaryKeys []string,
) (sqliteMutation, error) {
	if strings.TrimSpace(table.Name) == "" {
		return sqliteMutation{}, fmt.Errorf("table name is required")
	}
	if len(primaryKeys) == 0 {
		return sqliteMutation{}, fmt.Errorf(
			"table has no primary key; existing rows cannot be updated safely",
		)
	}
	available := sqliteStructureMap(structures)
	if err := validateSQLiteMutationFields(update.Values, available); err != nil {
		return sqliteMutation{}, err
	}
	if err := validateSQLiteMutationFields(update.Original, available); err != nil {
		return sqliteMutation{}, err
	}

	assignments := make([]string, 0, len(update.ChangedColumns))
	args := make([]interface{}, 0, len(update.ChangedColumns)+len(primaryKeys))
	seen := make(map[string]struct{}, len(update.ChangedColumns))
	for _, requested := range update.ChangedColumns {
		column := strings.TrimSpace(requested)
		if column == "" {
			continue
		}
		if _, duplicate := seen[column]; duplicate {
			continue
		}
		structure, exists := available[column]
		if !exists {
			return sqliteMutation{}, fmt.Errorf(
				"cannot update unknown column %q",
				column,
			)
		}
		if structure.IsGenerated {
			return sqliteMutation{}, fmt.Errorf(
				"generated column %q cannot be updated",
				column,
			)
		}
		value, exists := update.Values[column]
		if !exists {
			return sqliteMutation{}, fmt.Errorf(
				"updated value for column %q is missing",
				column,
			)
		}
		seen[column] = struct{}{}
		assignments = append(
			assignments,
			quoteSQLiteIdentifier(column)+" = ?",
		)
		args = append(args, value)
	}
	if len(assignments) == 0 {
		return sqliteMutation{}, fmt.Errorf("row update has no changed columns")
	}
	where := make([]string, 0, len(primaryKeys))
	for _, primaryKey := range primaryKeys {
		value, exists := update.Original[primaryKey]
		if !exists || value == nil {
			return sqliteMutation{}, fmt.Errorf(
				"original primary-key value for %q is missing",
				primaryKey,
			)
		}
		where = append(where, quoteSQLiteIdentifier(primaryKey)+" IS ?")
		args = append(args, value)
	}
	return sqliteMutation{
		SQL: fmt.Sprintf(
			"UPDATE %s SET %s WHERE %s",
			quoteSQLiteQualifiedIdentifier(table.Schema, table.Name),
			strings.Join(assignments, ", "),
			strings.Join(where, " AND "),
		),
		Args: args,
	}, nil
}

func buildSQLiteDeleteMutation(
	table database.Table,
	row map[string]interface{},
	primaryKeys []string,
) (sqliteMutation, error) {
	if strings.TrimSpace(table.Name) == "" {
		return sqliteMutation{}, fmt.Errorf("table name is required")
	}
	if len(primaryKeys) == 0 {
		return sqliteMutation{}, fmt.Errorf(
			"table has no primary key; existing rows cannot be deleted safely",
		)
	}
	where := make([]string, 0, len(primaryKeys))
	args := make([]interface{}, 0, len(primaryKeys))
	for _, primaryKey := range primaryKeys {
		value, exists := row[primaryKey]
		if !exists || value == nil {
			return sqliteMutation{}, fmt.Errorf(
				"primary-key value for %q is missing",
				primaryKey,
			)
		}
		where = append(where, quoteSQLiteIdentifier(primaryKey)+" IS ?")
		args = append(args, value)
	}
	return sqliteMutation{
		SQL: fmt.Sprintf(
			"DELETE FROM %s WHERE %s",
			quoteSQLiteQualifiedIdentifier(table.Schema, table.Name),
			strings.Join(where, " AND "),
		),
		Args: args,
	}, nil
}

func executeSQLiteMutation(
	ctx context.Context,
	execer sqliteMutationExecer,
	mutation sqliteMutation,
	action string,
) error {
	result, err := execer.ExecContext(ctx, mutation.SQL, mutation.Args...)
	if err != nil {
		return err
	}
	return requireOneSQLiteRow(result, action)
}

func (s *SQLite) ApplyTableChanges(
	ctx context.Context,
	changes database.TableChangeSet,
) (database.TableChangeResult, error) {
	if changes.Count() == 0 {
		return database.TableChangeResult{}, fmt.Errorf(
			"there are no row changes to apply",
		)
	}
	if ctx == nil {
		ctx = s.ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	changes.Table.Schema = normalizeSQLiteSchema(changes.Table.Schema)
	structures, err := s.GetCollectionStructures(changes.Table)
	if err != nil {
		return database.TableChangeResult{}, err
	}
	primaryKeys := sqlitePrimaryKeys(structures)

	transaction, err := s.conn.BeginTxx(ctx, nil)
	if err != nil {
		return database.TableChangeResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = transaction.Rollback()
		}
	}()

	result := database.TableChangeResult{}
	for index, row := range changes.Added {
		mutation, buildErr := buildSQLiteInsertMutation(
			changes.Table,
			row,
			structures,
		)
		if buildErr != nil {
			return database.TableChangeResult{}, fmt.Errorf(
				"insert %d: %w",
				index+1,
				buildErr,
			)
		}
		if err := executeSQLiteMutation(
			ctx,
			transaction,
			mutation,
			"row insert",
		); err != nil {
			return database.TableChangeResult{}, fmt.Errorf(
				"insert %d: %w",
				index+1,
				err,
			)
		}
		result.Inserted++
	}
	for index, update := range changes.Updated {
		mutation, buildErr := buildSQLiteUpdateMutation(
			changes.Table,
			update,
			structures,
			primaryKeys,
		)
		if buildErr != nil {
			return database.TableChangeResult{}, fmt.Errorf(
				"update %d: %w",
				index+1,
				buildErr,
			)
		}
		if err := executeSQLiteMutation(
			ctx,
			transaction,
			mutation,
			"row update",
		); err != nil {
			return database.TableChangeResult{}, fmt.Errorf(
				"update %d: %w",
				index+1,
				err,
			)
		}
		result.Updated++
	}
	for index, row := range changes.Deleted {
		mutation, buildErr := buildSQLiteDeleteMutation(
			changes.Table,
			row,
			primaryKeys,
		)
		if buildErr != nil {
			return database.TableChangeResult{}, fmt.Errorf(
				"delete %d: %w",
				index+1,
				buildErr,
			)
		}
		if err := executeSQLiteMutation(
			ctx,
			transaction,
			mutation,
			"row deletion",
		); err != nil {
			return database.TableChangeResult{}, fmt.Errorf(
				"delete %d: %w",
				index+1,
				err,
			)
		}
		result.Deleted++
	}
	if err := transaction.Commit(); err != nil {
		return database.TableChangeResult{}, err
	}
	committed = true
	return result, nil
}

var _ database.TableChangeDriver = (*SQLite)(nil)
