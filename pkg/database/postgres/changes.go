package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"rollingthunder/pkg/database"
)

type postgresMutation struct {
	SQL  string
	Args []interface{}
}

type postgresMutationExecer interface {
	ExecContext(
		context.Context,
		string,
		...interface{},
	) (sql.Result, error)
}

func postgresStructureMap(
	structures database.Structures,
) map[string]database.Structure {
	columns := make(map[string]database.Structure, len(structures))
	for _, structure := range structures {
		columns[structure.Name] = structure
	}
	return columns
}

func postgresPrimaryKeyColumns(
	structures database.Structures,
) []string {
	primaryKeys := make([]string, 0)
	for _, structure := range structures {
		if structure.IsPrimary {
			primaryKeys = append(primaryKeys, structure.Name)
		}
	}
	return primaryKeys
}

func validatePostgresMutationTable(table database.Table) error {
	if strings.TrimSpace(table.Schema) == "" {
		return fmt.Errorf("table schema is required")
	}
	if strings.TrimSpace(table.Name) == "" {
		return fmt.Errorf("table name is required")
	}
	return nil
}

func validatePostgresMutationFields(
	data map[string]interface{},
	available map[string]database.Structure,
) error {
	for column := range data {
		if strings.HasPrefix(column, "_rt") ||
			column == "_isNew" ||
			strings.HasPrefix(column, "temp_") {
			continue
		}
		if _, exists := available[column]; !exists {
			return fmt.Errorf("unknown table column %q", column)
		}
	}
	return nil
}

func buildPostgresInsertMutation(
	table database.Table,
	data map[string]interface{},
	structures database.Structures,
) (postgresMutation, error) {
	if err := validatePostgresMutationTable(table); err != nil {
		return postgresMutation{}, err
	}
	available := postgresStructureMap(structures)
	if err := validatePostgresMutationFields(data, available); err != nil {
		return postgresMutation{}, err
	}

	columns := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	args := make([]interface{}, 0, len(data))
	overrideIdentity := false
	for _, structure := range structures {
		value, exists := data[structure.Name]
		if !exists {
			continue
		}
		if value == nil && (structure.IsAutoInc || structure.Default != nil) {
			continue
		}
		columns = append(columns, quotePostgresIdentifier(structure.Name))
		args = append(args, value)
		if structure.IsAutoInc &&
			strings.EqualFold(
				strings.TrimSpace(structure.Generation),
				"IDENTITY ALWAYS",
			) {
			overrideIdentity = true
		}
		placeholders = append(
			placeholders,
			fmt.Sprintf("$%d", len(args)),
		)
	}

	target := quotePostgresQualifiedIdentifier(table.Schema, table.Name)
	if len(columns) == 0 {
		return postgresMutation{
			SQL: fmt.Sprintf("INSERT INTO %s DEFAULT VALUES", target),
		}, nil
	}
	query := "INSERT INTO " + target +
		" (" + strings.Join(columns, ", ") + ")"
	if overrideIdentity {
		query += " OVERRIDING SYSTEM VALUE"
	}
	query += " VALUES (" + strings.Join(placeholders, ", ") + ")"
	return postgresMutation{SQL: query, Args: args}, nil
}

func buildPostgresUpdateMutation(
	table database.Table,
	update database.RowUpdate,
	structures database.Structures,
	primaryKeys []string,
) (postgresMutation, error) {
	if err := validatePostgresMutationTable(table); err != nil {
		return postgresMutation{}, err
	}
	if len(primaryKeys) == 0 {
		return postgresMutation{}, fmt.Errorf(
			"table has no primary key; existing rows cannot be updated safely",
		)
	}

	available := postgresStructureMap(structures)
	if err := validatePostgresMutationFields(update.Values, available); err != nil {
		return postgresMutation{}, err
	}
	if err := validatePostgresMutationFields(update.Original, available); err != nil {
		return postgresMutation{}, err
	}

	setClauses := make([]string, 0, len(update.ChangedColumns))
	args := make([]interface{}, 0, len(update.ChangedColumns)+len(primaryKeys))
	seen := make(map[string]struct{}, len(update.ChangedColumns))
	for _, requestedColumn := range update.ChangedColumns {
		column := strings.TrimSpace(requestedColumn)
		if column == "" {
			continue
		}
		if _, duplicate := seen[column]; duplicate {
			continue
		}
		if _, exists := available[column]; !exists {
			return postgresMutation{}, fmt.Errorf(
				"cannot update unknown column %q",
				column,
			)
		}
		value, exists := update.Values[column]
		if !exists {
			return postgresMutation{}, fmt.Errorf(
				"updated value for column %q is missing",
				column,
			)
		}
		seen[column] = struct{}{}
		args = append(args, value)
		setClauses = append(
			setClauses,
			fmt.Sprintf(
				"%s = $%d",
				quotePostgresIdentifier(column),
				len(args),
			),
		)
	}
	if len(setClauses) == 0 {
		return postgresMutation{}, fmt.Errorf(
			"row update has no changed columns",
		)
	}

	whereClauses := make([]string, 0, len(primaryKeys))
	for _, primaryKey := range primaryKeys {
		value, exists := update.Original[primaryKey]
		if !exists || value == nil {
			return postgresMutation{}, fmt.Errorf(
				"original primary-key value for %q is missing",
				primaryKey,
			)
		}
		args = append(args, value)
		whereClauses = append(
			whereClauses,
			fmt.Sprintf(
				"%s = $%d",
				quotePostgresIdentifier(primaryKey),
				len(args),
			),
		)
	}

	return postgresMutation{
		SQL: fmt.Sprintf(
			"UPDATE %s SET %s WHERE %s",
			quotePostgresQualifiedIdentifier(table.Schema, table.Name),
			strings.Join(setClauses, ", "),
			strings.Join(whereClauses, " AND "),
		),
		Args: args,
	}, nil
}

func buildPostgresDeleteMutation(
	table database.Table,
	row map[string]interface{},
	primaryKeys []string,
) (postgresMutation, error) {
	if err := validatePostgresMutationTable(table); err != nil {
		return postgresMutation{}, err
	}
	if len(primaryKeys) == 0 {
		return postgresMutation{}, fmt.Errorf(
			"table has no primary key; existing rows cannot be deleted safely",
		)
	}

	whereClauses := make([]string, 0, len(primaryKeys))
	args := make([]interface{}, 0, len(primaryKeys))
	for _, primaryKey := range primaryKeys {
		value, exists := row[primaryKey]
		if !exists || value == nil {
			return postgresMutation{}, fmt.Errorf(
				"primary-key value for %q is missing",
				primaryKey,
			)
		}
		args = append(args, value)
		whereClauses = append(
			whereClauses,
			fmt.Sprintf(
				"%s = $%d",
				quotePostgresIdentifier(primaryKey),
				len(args),
			),
		)
	}

	return postgresMutation{
		SQL: fmt.Sprintf(
			"DELETE FROM %s WHERE %s",
			quotePostgresQualifiedIdentifier(table.Schema, table.Name),
			strings.Join(whereClauses, " AND "),
		),
		Args: args,
	}, nil
}

func executePostgresMutation(
	ctx context.Context,
	execer postgresMutationExecer,
	mutation postgresMutation,
	action string,
) error {
	result, err := execer.ExecContext(
		ctx,
		mutation.SQL,
		mutation.Args...,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf(
			"%s affected %d rows instead of exactly one",
			action,
			affected,
		)
	}
	return nil
}

func (p *Postgres) ApplyTableChanges(
	ctx context.Context,
	changes database.TableChangeSet,
) (database.TableChangeResult, error) {
	if changes.Count() == 0 {
		return database.TableChangeResult{}, fmt.Errorf(
			"there are no row changes to apply",
		)
	}
	if ctx == nil {
		ctx = p.ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}

	structures, err := p.getCollectionStructures(changes.Table)
	if err != nil {
		return database.TableChangeResult{}, err
	}
	primaryKeys := postgresPrimaryKeyColumns(
		structuresFromColumns(structures),
	)
	normalizedStructures := structuresFromColumns(structures)

	transaction, err := p.conn.BeginTxx(ctx, nil)
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
		mutation, buildErr := buildPostgresInsertMutation(
			changes.Table,
			row,
			normalizedStructures,
		)
		if buildErr != nil {
			return database.TableChangeResult{}, fmt.Errorf(
				"insert %d: %w",
				index+1,
				buildErr,
			)
		}
		if err := executePostgresMutation(
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
		mutation, buildErr := buildPostgresUpdateMutation(
			changes.Table,
			update,
			normalizedStructures,
			primaryKeys,
		)
		if buildErr != nil {
			return database.TableChangeResult{}, fmt.Errorf(
				"update %d: %w",
				index+1,
				buildErr,
			)
		}
		if err := executePostgresMutation(
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
		mutation, buildErr := buildPostgresDeleteMutation(
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
		if err := executePostgresMutation(
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
