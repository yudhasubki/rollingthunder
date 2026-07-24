package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

type mysqlMutation struct {
	SQL  string
	Args []interface{}
}

type mysqlMutationExecer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

func mysqlPrimaryKeys(structures database.Structures) []string {
	primaryKeys := make([]string, 0)
	for _, structure := range structures {
		if structure.IsPrimary {
			primaryKeys = append(primaryKeys, structure.Name)
		}
	}
	return primaryKeys
}

func mysqlStructureMap(
	structures database.Structures,
) map[string]database.Structure {
	available := make(map[string]database.Structure, len(structures))
	for _, structure := range structures {
		available[structure.Name] = structure
	}
	return available
}

func validateMySQLMutationFields(
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

func buildMySQLInsertMutation(
	table database.Table,
	values map[string]interface{},
	structures database.Structures,
) (mysqlMutation, error) {
	if strings.TrimSpace(table.Name) == "" {
		return mysqlMutation{}, fmt.Errorf("table name is required")
	}
	available := mysqlStructureMap(structures)
	if err := validateMySQLMutationFields(values, available); err != nil {
		return mysqlMutation{}, err
	}

	columns := make([]string, 0, len(values))
	args := make([]interface{}, 0, len(values))
	for _, structure := range structures {
		value, exists := values[structure.Name]
		if !exists {
			continue
		}
		if value == nil && (structure.IsAutoInc || structure.Default != nil) {
			continue
		}
		columns = append(columns, quoteMySQLIdentifier(structure.Name))
		args = append(args, value)
	}
	target := quoteMySQLQualifiedIdentifier(table.Schema, table.Name)
	if len(columns) == 0 {
		return mysqlMutation{
			SQL: "INSERT INTO " + target + " () VALUES ()",
		}, nil
	}
	placeholders := make([]string, len(columns))
	for index := range placeholders {
		placeholders[index] = "?"
	}
	return mysqlMutation{
		SQL: fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES (%s)",
			target,
			strings.Join(columns, ", "),
			strings.Join(placeholders, ", "),
		),
		Args: args,
	}, nil
}

func buildMySQLUpdateMutation(
	table database.Table,
	update database.RowUpdate,
	structures database.Structures,
	primaryKeys []string,
) (mysqlMutation, error) {
	if strings.TrimSpace(table.Name) == "" {
		return mysqlMutation{}, fmt.Errorf("table name is required")
	}
	if len(primaryKeys) == 0 {
		return mysqlMutation{}, fmt.Errorf(
			"table has no primary key; existing rows cannot be updated safely",
		)
	}
	available := mysqlStructureMap(structures)
	if err := validateMySQLMutationFields(update.Values, available); err != nil {
		return mysqlMutation{}, err
	}
	if err := validateMySQLMutationFields(update.Original, available); err != nil {
		return mysqlMutation{}, err
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
		if _, exists := available[column]; !exists {
			return mysqlMutation{}, fmt.Errorf(
				"cannot update unknown column %q",
				column,
			)
		}
		value, exists := update.Values[column]
		if !exists {
			return mysqlMutation{}, fmt.Errorf(
				"updated value for column %q is missing",
				column,
			)
		}
		seen[column] = struct{}{}
		assignments = append(
			assignments,
			quoteMySQLIdentifier(column)+" = ?",
		)
		args = append(args, value)
	}
	if len(assignments) == 0 {
		return mysqlMutation{}, fmt.Errorf("row update has no changed columns")
	}

	where := make([]string, 0, len(primaryKeys))
	for _, primaryKey := range primaryKeys {
		value, exists := update.Original[primaryKey]
		if !exists || value == nil {
			return mysqlMutation{}, fmt.Errorf(
				"original primary-key value for %q is missing",
				primaryKey,
			)
		}
		where = append(where, quoteMySQLIdentifier(primaryKey)+" = ?")
		args = append(args, value)
	}
	return mysqlMutation{
		SQL: fmt.Sprintf(
			"UPDATE %s SET %s WHERE %s",
			quoteMySQLQualifiedIdentifier(table.Schema, table.Name),
			strings.Join(assignments, ", "),
			strings.Join(where, " AND "),
		),
		Args: args,
	}, nil
}

func buildMySQLDeleteMutation(
	table database.Table,
	row map[string]interface{},
	primaryKeys []string,
) (mysqlMutation, error) {
	if strings.TrimSpace(table.Name) == "" {
		return mysqlMutation{}, fmt.Errorf("table name is required")
	}
	if len(primaryKeys) == 0 {
		return mysqlMutation{}, fmt.Errorf(
			"table has no primary key; existing rows cannot be deleted safely",
		)
	}
	where := make([]string, 0, len(primaryKeys))
	args := make([]interface{}, 0, len(primaryKeys))
	for _, primaryKey := range primaryKeys {
		value, exists := row[primaryKey]
		if !exists || value == nil {
			return mysqlMutation{}, fmt.Errorf(
				"primary-key value for %q is missing",
				primaryKey,
			)
		}
		where = append(where, quoteMySQLIdentifier(primaryKey)+" = ?")
		args = append(args, value)
	}
	return mysqlMutation{
		SQL: fmt.Sprintf(
			"DELETE FROM %s WHERE %s",
			quoteMySQLQualifiedIdentifier(table.Schema, table.Name),
			strings.Join(where, " AND "),
		),
		Args: args,
	}, nil
}

func executeMySQLMutation(
	ctx context.Context,
	execer mysqlMutationExecer,
	mutation mysqlMutation,
	action string,
) error {
	result, err := execer.ExecContext(ctx, mutation.SQL, mutation.Args...)
	if err != nil {
		return err
	}
	return requireOneMySQLRow(result, action)
}

func (m *MySQL) ApplyTableChanges(
	ctx context.Context,
	changes database.TableChangeSet,
) (database.TableChangeResult, error) {
	if changes.Count() == 0 {
		return database.TableChangeResult{}, fmt.Errorf(
			"there are no row changes to apply",
		)
	}
	if ctx == nil {
		ctx = m.ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	changes.Table.Schema = m.defaultDatabase(changes.Table.Schema)
	structures, err := m.GetCollectionStructures(changes.Table)
	if err != nil {
		return database.TableChangeResult{}, err
	}
	primaryKeys := mysqlPrimaryKeys(structures)

	transaction, err := m.conn.BeginTxx(ctx, nil)
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
		mutation, buildErr := buildMySQLInsertMutation(
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
		if err := executeMySQLMutation(
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
		mutation, buildErr := buildMySQLUpdateMutation(
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
		if err := executeMySQLMutation(
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
		mutation, buildErr := buildMySQLDeleteMutation(
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
		if err := executeMySQLMutation(
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

var _ database.TableChangeDriver = (*MySQL)(nil)
