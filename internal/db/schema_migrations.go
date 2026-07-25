package db

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"
)

type schemaMigrationPlan struct {
	preview database.SchemaMigrationPreview
	plan    database.ObjectChangePlan
}

func (s *Service) schemaSnapshot(
	connectionID string,
	schema string,
) (database.SchemaSnapshot, error) {
	driver, release, err := s.driverFor(connectionID)
	if err != nil {
		return database.SchemaSnapshot{}, err
	}
	defer release()

	engine := driver.Capabilities().Engine
	names, err := driver.GetCollections(schema)
	if err != nil {
		return database.SchemaSnapshot{}, fmt.Errorf(
			"list %s tables: %w",
			schema,
			err,
		)
	}
	sort.Strings(names)
	snapshot := database.SchemaSnapshot{
		Engine: engine,
		Schema: schema,
		Tables: make([]database.SchemaTableSnapshot, 0, len(names)),
	}
	for _, name := range names {
		table := database.Table{Schema: schema, Name: name}
		columns, err := driver.GetCollectionStructures(table)
		if err != nil {
			return database.SchemaSnapshot{}, fmt.Errorf(
				"inspect %s.%s columns: %w",
				schema,
				name,
				err,
			)
		}
		indexes, err := driver.GetIndices(table)
		if err != nil {
			return database.SchemaSnapshot{}, fmt.Errorf(
				"inspect %s.%s indexes: %w",
				schema,
				name,
				err,
			)
		}
		sort.Slice(indexes, func(i, j int) bool {
			return indexes[i].Name < indexes[j].Name
		})
		definition, err := driver.GetTableDDL(table)
		if err != nil {
			return database.SchemaSnapshot{}, fmt.Errorf(
				"read %s.%s definition: %w",
				schema,
				name,
				err,
			)
		}
		snapshot.Tables = append(snapshot.Tables, database.SchemaTableSnapshot{
			Name:       name,
			Definition: definition,
			Columns:    columns,
			Indexes:    indexes,
		})
	}
	return snapshot, nil
}

func qualifiedMigrationName(
	driver database.Driver,
	schema string,
	name string,
) string {
	quoted := driver.QuoteIdentifier(name)
	if strings.TrimSpace(schema) == "" {
		return quoted
	}
	return driver.QuoteIdentifier(schema) + "." + quoted
}

func retargetTableDefinition(
	driver database.Driver,
	engine string,
	definition string,
	sourceSchema string,
	targetSchema string,
	table string,
) (string, bool) {
	definition = strings.TrimSpace(definition)
	if definition == "" {
		return "", false
	}
	if !strings.HasSuffix(definition, ";") {
		definition += ";"
	}
	quotedTable := driver.QuoteIdentifier(table)
	qualifiedTarget := qualifiedMigrationName(driver, targetSchema, table)

	switch engine {
	case "postgres":
		sourcePrefix := driver.QuoteIdentifier(sourceSchema) + "."
		targetPrefix := driver.QuoteIdentifier(targetSchema) + "."
		if sourcePrefix != targetPrefix {
			definition = strings.ReplaceAll(definition, sourcePrefix, targetPrefix)
		}
		return definition, true
	case "mysql":
		needles := []string{
			"CREATE TABLE " + quotedTable,
			"CREATE TABLE IF NOT EXISTS " + quotedTable,
		}
		for _, needle := range needles {
			if strings.Contains(definition, needle) {
				return strings.Replace(
					definition,
					needle,
					strings.Replace(needle, quotedTable, qualifiedTarget, 1),
					1,
				), true
			}
		}
	case "sqlite":
		needles := []string{
			"CREATE TABLE " + quotedTable,
			"CREATE TABLE IF NOT EXISTS " + quotedTable,
			"CREATE TABLE " + table,
			"CREATE TABLE IF NOT EXISTS " + table,
		}
		for _, needle := range needles {
			if strings.Contains(definition, needle) {
				replacement := strings.TrimSuffix(needle, quotedTable)
				if strings.HasSuffix(needle, table) {
					replacement = strings.TrimSuffix(needle, table)
				}
				return strings.Replace(
					definition,
					needle,
					replacement+qualifiedTarget,
					1,
				), true
			}
		}
	}
	return "", false
}

func migrationColumnType(column database.Structure, engine string) string {
	if column.TypeName != nil && strings.TrimSpace(*column.TypeName) != "" &&
		engine == "postgres" {
		name := strings.TrimSpace(*column.TypeName)
		if column.TypeSchema != nil && strings.TrimSpace(*column.TypeSchema) != "" {
			name = `"` + strings.ReplaceAll(
				strings.TrimSpace(*column.TypeSchema),
				`"`,
				`""`,
			) + `"."` + strings.ReplaceAll(name, `"`, `""`) + `"`
		}
		return name
	}
	dataType := strings.TrimSpace(column.NativeType)
	if dataType == "" {
		dataType = strings.TrimSpace(column.DataType)
	}
	if dataType == "" {
		dataType = "TEXT"
	}
	if column.IsAutoInc {
		switch engine {
		case "postgres":
			switch strings.ToLower(dataType) {
			case "smallint", "int2":
				return "smallserial"
			case "bigint", "int8":
				return "bigserial"
			default:
				return "serial"
			}
		case "mysql":
			if !strings.Contains(strings.ToLower(dataType), "auto_increment") {
				dataType += " AUTO_INCREMENT"
			}
		}
	}
	return dataType
}

func migrationColumnDefinition(
	column database.Structure,
	engine string,
) database.ColumnDefinition {
	defaultValue := ""
	if column.Default != nil && !(engine == "postgres" && column.IsAutoInc) {
		defaultValue = *column.Default
	}
	return database.ColumnDefinition{
		Name:       column.Name,
		Type:       migrationColumnType(column, engine),
		Nullable:   column.Nullable,
		Default:    defaultValue,
		PrimaryKey: column.IsPrimary,
		Unique:     column.IsUnique && !column.IsPrimary,
	}
}

func normalizedMigrationValue(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func normalizedMigrationDefault(value *string) string {
	if value == nil {
		return ""
	}
	return normalizedMigrationValue(*value)
}

func columnSignature(column database.Structure, engine string) string {
	return strings.Join([]string{
		normalizedMigrationValue(migrationColumnType(column, engine)),
		fmt.Sprintf("nullable=%t", column.Nullable),
		"default=" + normalizedMigrationDefault(column.Default),
		fmt.Sprintf("auto=%t", column.IsAutoInc),
		fmt.Sprintf("generated=%t:%s", column.IsGenerated, column.Generation),
	}, "|")
}

func keySignature(column database.Structure) string {
	foreign := ""
	if column.ForeignTable != nil {
		foreign = stringValue(column.ForeignSchema) + "." +
			stringValue(column.ForeignTable) + "(" +
			stringValue(column.ForeignColumn) + ")"
	}
	return fmt.Sprintf(
		"primary=%t|unique=%t|foreign=%s",
		column.IsPrimary,
		column.IsUnique,
		foreign,
	)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func indexSignature(index database.Index) string {
	columns := make([]string, 0, len(index.Columns))
	for _, column := range index.Columns {
		columns = append(columns, normalizedMigrationValue(column))
	}
	return fmt.Sprintf(
		"%t|%s|%s",
		index.IsUnique,
		normalizedMigrationValue(index.Algorithm),
		strings.Join(columns, ","),
	)
}

func migratableIndex(index database.Index) bool {
	if index.IsPrimary || len(index.Columns) == 0 {
		return false
	}
	if strings.HasPrefix(strings.ToLower(index.Name), "sqlite_autoindex_") {
		return false
	}
	for _, column := range index.Columns {
		if column == "<expression>" {
			return false
		}
	}
	return true
}

func tableMap(
	tables []database.SchemaTableSnapshot,
) map[string]database.SchemaTableSnapshot {
	result := make(map[string]database.SchemaTableSnapshot, len(tables))
	for _, table := range tables {
		result[table.Name] = table
	}
	return result
}

func columnMap(columns database.Structures) map[string]database.Structure {
	result := make(map[string]database.Structure, len(columns))
	for _, column := range columns {
		result[column.Name] = column
	}
	return result
}

func indexMap(indexes database.Indices) map[string]database.Index {
	result := make(map[string]database.Index, len(indexes))
	for _, index := range indexes {
		if migratableIndex(index) {
			result[index.Name] = index
		}
	}
	return result
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func appendPlanChange(
	changes *[]database.SchemaMigrationChange,
	action string,
	object string,
	plan database.ObjectChangePlan,
) {
	warnings := plan.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	*changes = append(*changes, database.SchemaMigrationChange{
		ID:          action + ":" + object,
		Action:      action,
		Object:      object,
		Summary:     plan.Summary,
		Statements:  plan.Statements,
		Destructive: plan.Destructive,
		Selected:    true,
		Supported:   true,
		Warnings:    warnings,
	})
}

func appendManualChange(
	changes *[]database.SchemaMigrationChange,
	action string,
	object string,
	summary string,
	reason string,
	destructive bool,
	selected bool,
) {
	*changes = append(*changes, database.SchemaMigrationChange{
		ID:          action + ":" + object,
		Action:      action,
		Object:      object,
		Summary:     summary,
		Statements:  []string{},
		Destructive: destructive,
		Selected:    selected,
		Supported:   false,
		Reason:      reason,
		Warnings:    []string{},
	})
}

func buildTargetObjectPlan(
	ctx context.Context,
	changeDriver database.ObjectChangeDriver,
	request database.ObjectChangeRequest,
) (database.ObjectChangePlan, error) {
	plan, err := changeDriver.BuildObjectChange(ctx, request)
	if err != nil {
		return database.ObjectChangePlan{}, err
	}
	if err := plan.Validate(); err != nil {
		return database.ObjectChangePlan{}, err
	}
	return plan, nil
}

func addIndexPlan(
	ctx context.Context,
	changeDriver database.ObjectChangeDriver,
	schema string,
	table string,
	index database.Index,
) (database.ObjectChangePlan, error) {
	method := index.Algorithm
	if strings.Contains(strings.ToLower(method), "b-tree") {
		method = "BTREE"
	}
	return buildTargetObjectPlan(ctx, changeDriver, database.ObjectChangeRequest{
		Action: database.ObjectChangeCreateIndex,
		Index: &database.IndexChange{
			Table: database.Table{Schema: schema, Name: table},
			Name:  index.Name, Columns: index.Columns,
			Unique: index.IsUnique, Method: method,
		},
	})
}

func dropIndexPlan(
	ctx context.Context,
	changeDriver database.ObjectChangeDriver,
	schema string,
	table string,
	index database.Index,
) (database.ObjectChangePlan, error) {
	return buildTargetObjectPlan(ctx, changeDriver, database.ObjectChangeRequest{
		Action: database.ObjectChangeDrop,
		Reference: database.ObjectReference{
			Kind:         database.ObjectKindIndex,
			Schema:       schema,
			Name:         index.Name,
			ParentSchema: schema,
			ParentName:   table,
		},
	})
}

func (s *Service) buildSchemaMigration(
	ctx context.Context,
	request database.SchemaMigrationRequest,
) (schemaMigrationPlan, error) {
	source, err := s.schemaSnapshot(
		request.SourceConnectionID,
		request.SourceSchema,
	)
	if err != nil {
		return schemaMigrationPlan{}, fmt.Errorf("snapshot source schema: %w", err)
	}
	target, err := s.schemaSnapshot(
		request.TargetConnectionID,
		request.TargetSchema,
	)
	if err != nil {
		return schemaMigrationPlan{}, fmt.Errorf("snapshot target schema: %w", err)
	}
	if source.Engine != target.Engine {
		return schemaMigrationPlan{}, fmt.Errorf(
			"reviewed migration requires matching engines; source is %s and target is %s",
			source.Engine,
			target.Engine,
		)
	}

	targetDriver, release, err := s.driverFor(request.TargetConnectionID)
	if err != nil {
		return schemaMigrationPlan{}, err
	}
	defer release()
	changeDriver, ok := targetDriver.(database.ObjectChangeDriver)
	if !ok {
		return schemaMigrationPlan{}, fmt.Errorf(
			"target driver does not support reviewed structural changes",
		)
	}

	changes := make([]database.SchemaMigrationChange, 0)
	sourceTables := tableMap(source.Tables)
	targetTables := tableMap(target.Tables)

	for _, name := range sortedKeys(sourceTables) {
		sourceTable := sourceTables[name]
		targetTable, exists := targetTables[name]
		object := request.TargetSchema + "." + name
		if !exists {
			definition, supported := retargetTableDefinition(
				targetDriver,
				target.Engine,
				sourceTable.Definition,
				request.SourceSchema,
				request.TargetSchema,
				name,
			)
			if !supported {
				appendManualChange(
					&changes,
					"create_table",
					object,
					"Create table "+object,
					"The source table definition could not be safely retargeted.",
					false,
					false,
				)
				continue
			}
			appendPlanChange(
				&changes,
				"create_table",
				object,
				database.ObjectChangePlan{
					Summary:       "Create table " + object,
					Statements:    []string{definition},
					Transactional: targetDriver.Capabilities().TransactionalDDL,
					Warnings: []string{
						"Review engine-specific defaults, generated columns, and foreign-key dependencies before applying.",
					},
				},
			)

			if target.Engine != "mysql" {
				for _, indexName := range sortedKeys(indexMap(sourceTable.Indexes)) {
					index := indexMap(sourceTable.Indexes)[indexName]
					plan, planErr := addIndexPlan(
						ctx,
						changeDriver,
						request.TargetSchema,
						name,
						index,
					)
					if planErr != nil {
						appendManualChange(
							&changes,
							"create_index",
							object+"."+index.Name,
							"Create index "+index.Name+" on "+object,
							planErr.Error(),
							false,
							false,
						)
						continue
					}
					appendPlanChange(
						&changes,
						"create_index",
						object+"."+index.Name,
						plan,
					)
				}
			}
			continue
		}

		sourceColumns := columnMap(sourceTable.Columns)
		targetColumns := columnMap(targetTable.Columns)
		for _, columnName := range sortedKeys(sourceColumns) {
			sourceColumn := sourceColumns[columnName]
			targetColumn, columnExists := targetColumns[columnName]
			columnObject := object + "." + columnName
			if !columnExists {
				plan, planErr := buildTargetObjectPlan(
					ctx,
					changeDriver,
					database.ObjectChangeRequest{
						Action: database.ObjectChangeAddColumn,
						AddColumn: &database.AddColumnChange{
							Table: database.Table{
								Schema: request.TargetSchema,
								Name:   name,
							},
							Column: migrationColumnDefinition(
								sourceColumn,
								target.Engine,
							),
						},
					},
				)
				if planErr != nil {
					appendManualChange(
						&changes,
						"add_column",
						columnObject,
						"Add column "+columnObject,
						planErr.Error(),
						false,
						false,
					)
					continue
				}
				appendPlanChange(&changes, "add_column", columnObject, plan)
				continue
			}

			if columnSignature(sourceColumn, source.Engine) !=
				columnSignature(targetColumn, target.Engine) {
				change := database.ColumnChange{
					Table: database.Table{
						Schema: request.TargetSchema,
						Name:   name,
					},
					Name: columnName,
				}
				if normalizedMigrationValue(migrationColumnType(sourceColumn, source.Engine)) !=
					normalizedMigrationValue(migrationColumnType(targetColumn, target.Engine)) {
					change.DataType = migrationColumnType(sourceColumn, target.Engine)
				}
				if sourceColumn.Nullable != targetColumn.Nullable {
					nullable := sourceColumn.Nullable
					change.Nullable = &nullable
				}
				sourceDefault := normalizedMigrationDefault(sourceColumn.Default)
				targetDefault := normalizedMigrationDefault(targetColumn.Default)
				if sourceDefault != targetDefault {
					if sourceColumn.Default == nil {
						change.DropDefault = true
					} else {
						value := *sourceColumn.Default
						change.Default = &value
					}
				}
				if sourceColumn.IsAutoInc != targetColumn.IsAutoInc ||
					sourceColumn.IsGenerated != targetColumn.IsGenerated ||
					sourceColumn.Generation != targetColumn.Generation {
					appendManualChange(
						&changes,
						"alter_column",
						columnObject,
						"Align generated or identity definition for "+columnObject,
						"Identity and generated-column changes require an engine-specific table rebuild.",
						true,
						false,
					)
				} else {
					plan, planErr := buildTargetObjectPlan(
						ctx,
						changeDriver,
						database.ObjectChangeRequest{
							Action: database.ObjectChangeAlterColumn,
							Column: &change,
						},
					)
					if planErr != nil {
						appendManualChange(
							&changes,
							"alter_column",
							columnObject,
							"Align column "+columnObject,
							planErr.Error(),
							true,
							false,
						)
					} else {
						plan.Destructive = true
						plan.Warnings = append(
							plan.Warnings,
							"Type or nullability changes can reject or rewrite existing rows.",
						)
						appendPlanChange(&changes, "alter_column", columnObject, plan)
					}
				}
			}

			if keySignature(sourceColumn) != keySignature(targetColumn) {
				appendManualChange(
					&changes,
					"align_constraint",
					columnObject,
					"Align primary, unique, or foreign-key constraint for "+columnObject,
					"Constraint names and multi-column ordering need explicit review in the table designer.",
					true,
					false,
				)
			}
		}

		for _, columnName := range sortedKeys(targetColumns) {
			if _, exists := sourceColumns[columnName]; exists {
				continue
			}
			columnObject := object + "." + columnName
			if !request.IncludeDestructive {
				appendManualChange(
					&changes,
					"drop_column",
					columnObject,
					"Drop extra column "+columnObject,
					"Enable destructive changes to include this removal.",
					true,
					false,
				)
				continue
			}
			plan, planErr := buildTargetObjectPlan(
				ctx,
				changeDriver,
				database.ObjectChangeRequest{
					Action: database.ObjectChangeDropColumn,
					DropColumn: &database.DropColumnChange{
						Table: database.Table{
							Schema: request.TargetSchema,
							Name:   name,
						},
						Name: columnName,
					},
				},
			)
			if planErr != nil {
				appendManualChange(
					&changes,
					"drop_column",
					columnObject,
					"Drop extra column "+columnObject,
					planErr.Error(),
					true,
					false,
				)
				continue
			}
			appendPlanChange(&changes, "drop_column", columnObject, plan)
		}

		sourceIndexes := indexMap(sourceTable.Indexes)
		targetIndexes := indexMap(targetTable.Indexes)
		for _, indexName := range sortedKeys(sourceIndexes) {
			sourceIndex := sourceIndexes[indexName]
			targetIndex, indexExists := targetIndexes[indexName]
			indexObject := object + "." + indexName
			if !indexExists {
				plan, planErr := addIndexPlan(
					ctx,
					changeDriver,
					request.TargetSchema,
					name,
					sourceIndex,
				)
				if planErr != nil {
					appendManualChange(
						&changes,
						"create_index",
						indexObject,
						"Create index "+indexName+" on "+object,
						planErr.Error(),
						false,
						false,
					)
				} else {
					appendPlanChange(&changes, "create_index", indexObject, plan)
				}
				continue
			}
			if indexSignature(sourceIndex) == indexSignature(targetIndex) {
				continue
			}
			if !request.IncludeDestructive {
				appendManualChange(
					&changes,
					"replace_index",
					indexObject,
					"Replace changed index "+indexName+" on "+object,
					"Enable destructive changes to replace this index.",
					true,
					false,
				)
				continue
			}
			dropPlan, dropErr := dropIndexPlan(
				ctx,
				changeDriver,
				request.TargetSchema,
				name,
				targetIndex,
			)
			createPlan, createErr := addIndexPlan(
				ctx,
				changeDriver,
				request.TargetSchema,
				name,
				sourceIndex,
			)
			if dropErr != nil || createErr != nil {
				reason := ""
				if dropErr != nil {
					reason = dropErr.Error()
				} else {
					reason = createErr.Error()
				}
				appendManualChange(
					&changes,
					"replace_index",
					indexObject,
					"Replace changed index "+indexName+" on "+object,
					reason,
					true,
					false,
				)
				continue
			}
			appendPlanChange(
				&changes,
				"replace_index",
				indexObject,
				database.ObjectChangePlan{
					Summary:       "Replace index " + indexName + " on " + object,
					Statements:    append(dropPlan.Statements, createPlan.Statements...),
					Destructive:   true,
					Transactional: targetDriver.Capabilities().TransactionalDDL,
					Warnings: append(
						dropPlan.Warnings,
						createPlan.Warnings...,
					),
				},
			)
		}
		for _, indexName := range sortedKeys(targetIndexes) {
			if _, exists := sourceIndexes[indexName]; exists {
				continue
			}
			index := targetIndexes[indexName]
			indexObject := object + "." + indexName
			if !request.IncludeDestructive {
				appendManualChange(
					&changes,
					"drop_index",
					indexObject,
					"Drop extra index "+indexName+" on "+object,
					"Enable destructive changes to include this removal.",
					true,
					false,
				)
				continue
			}
			plan, planErr := dropIndexPlan(
				ctx,
				changeDriver,
				request.TargetSchema,
				name,
				index,
			)
			if planErr != nil {
				appendManualChange(
					&changes,
					"drop_index",
					indexObject,
					"Drop extra index "+indexName+" on "+object,
					planErr.Error(),
					true,
					false,
				)
			} else {
				appendPlanChange(&changes, "drop_index", indexObject, plan)
			}
		}
	}

	for _, name := range sortedKeys(targetTables) {
		if _, exists := sourceTables[name]; exists {
			continue
		}
		object := request.TargetSchema + "." + name
		if !request.IncludeDestructive {
			appendManualChange(
				&changes,
				"drop_table",
				object,
				"Drop extra table "+object,
				"Enable destructive changes to include this removal.",
				true,
				false,
			)
			continue
		}
		plan, planErr := buildTargetObjectPlan(
			ctx,
			changeDriver,
			database.ObjectChangeRequest{
				Action: database.ObjectChangeDrop,
				Reference: database.ObjectReference{
					Kind:   database.ObjectKindTable,
					Schema: request.TargetSchema,
					Name:   name,
				},
				Cascade: target.Engine == "postgres",
			},
		)
		if planErr != nil {
			appendManualChange(
				&changes,
				"drop_table",
				object,
				"Drop extra table "+object,
				planErr.Error(),
				true,
				false,
			)
		} else {
			appendPlanChange(&changes, "drop_table", object, plan)
		}
	}

	statements := make([]string, 0)
	warnings := make([]string, 0)
	warningSet := make(map[string]struct{})
	selectedChanges := 0
	manualChanges := 0
	destructive := false
	for _, change := range changes {
		if !change.Supported {
			manualChanges++
		}
		if !change.Selected || !change.Supported {
			continue
		}
		selectedChanges++
		statements = append(statements, change.Statements...)
		destructive = destructive || change.Destructive
		for _, warning := range change.Warnings {
			if _, seen := warningSet[warning]; seen || strings.TrimSpace(warning) == "" {
				continue
			}
			warningSet[warning] = struct{}{}
			warnings = append(warnings, warning)
		}
	}
	if !targetDriver.Capabilities().TransactionalDDL && len(statements) > 1 {
		warning := "This engine auto-commits DDL; a multi-step migration may be partially applied if a later statement fails."
		if _, exists := warningSet[warning]; !exists {
			warnings = append(warnings, warning)
		}
	}
	summary := fmt.Sprintf(
		"Migrate %s.%s to %s.%s",
		source.Engine,
		request.SourceSchema,
		target.Engine,
		request.TargetSchema,
	)
	plan := database.ObjectChangePlan{
		Summary:       summary,
		Statements:    statements,
		Destructive:   destructive,
		Transactional: targetDriver.Capabilities().TransactionalDDL,
		Warnings:      warnings,
	}
	fingerprint := database.SchemaMigrationFingerprint(
		request,
		target.Engine,
		changes,
	)
	return schemaMigrationPlan{
		preview: database.SchemaMigrationPreview{
			SourceEngine:    source.Engine,
			TargetEngine:    target.Engine,
			SourceSchema:    request.SourceSchema,
			TargetSchema:    request.TargetSchema,
			Changes:         changes,
			SQL:             strings.Join(statements, "\n\n"),
			StatementCount:  len(statements),
			SelectedChanges: selectedChanges,
			ManualChanges:   manualChanges,
			Destructive:     destructive,
			Transactional:   plan.Transactional,
			Warnings:        warnings,
			Fingerprint:     fingerprint,
		},
		plan: plan,
	}, nil
}

func (s *Service) PreviewSchemaMigration(
	request database.SchemaMigrationRequest,
) response.BaseResponse[database.SchemaMigrationPreview] {
	if err := request.Validate(); err != nil {
		return serviceErrorWithCode[database.SchemaMigrationPreview](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid schema migration",
			err.Error(),
			"Choose different source and target schemas before comparing them.",
		)
	}
	ctx, cancel := s.structuralChangeContext()
	defer cancel()
	plan, err := s.buildSchemaMigration(ctx, request)
	if err != nil {
		return serviceErrorWithCode[database.SchemaMigrationPreview](
			http.StatusBadRequest,
			errorCodeSchemaMigrationFailed,
			"Could not compare schemas",
			err.Error(),
			"Check both connections and retry the schema comparison.",
		)
	}
	return response.BaseResponse[database.SchemaMigrationPreview]{
		Data: plan.preview,
	}
}

func (s *Service) ApplySchemaMigration(
	request database.ApplySchemaMigrationRequest,
) response.BaseResponse[database.SchemaMigrationResult] {
	if err := request.Migration.Validate(); err != nil {
		return serviceErrorWithCode[database.SchemaMigrationResult](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid schema migration",
			err.Error(),
			"Return to the migration editor and generate a fresh preview.",
		)
	}
	if strings.TrimSpace(request.Fingerprint) == "" {
		return serviceErrorWithCode[database.SchemaMigrationResult](
			http.StatusConflict,
			errorCodeSchemaMigrationReview,
			"Migration review required",
			"The migration SQL has not been reviewed.",
			"Compare the schemas and review the SQL before applying it.",
		)
	}

	ctx, cancel := s.structuralChangeContext()
	defer cancel()
	built, err := s.buildSchemaMigration(ctx, request.Migration)
	if err != nil {
		return serviceErrorWithCode[database.SchemaMigrationResult](
			http.StatusBadRequest,
			errorCodeSchemaMigrationFailed,
			"Could not refresh migration plan",
			err.Error(),
			"Compare the schemas again before applying any changes.",
		)
	}
	if !reviewedFingerprintMatches(
		request.Fingerprint,
		built.preview.Fingerprint,
	) {
		return serviceErrorWithCode[database.SchemaMigrationResult](
			http.StatusConflict,
			errorCodeSchemaMigrationReview,
			"Schema migration changed",
			"One of the schemas changed after the SQL preview was generated.",
			"Review the refreshed diff before applying the migration.",
		)
	}
	if len(built.plan.Statements) == 0 {
		return response.BaseResponse[database.SchemaMigrationResult]{
			Data: database.SchemaMigrationResult{
				Applied:     true,
				Fingerprint: built.preview.Fingerprint,
			},
		}
	}

	targetDriver, release, err := s.driverFor(
		request.Migration.TargetConnectionID,
	)
	if err != nil {
		return serviceError[database.SchemaMigrationResult](err.Error())
	}
	defer release()
	changeDriver, ok := targetDriver.(database.ObjectChangeDriver)
	if !ok {
		return unsupportedObjectChange[database.SchemaMigrationResult]()
	}
	if err := changeDriver.ApplyObjectChange(ctx, built.plan); err != nil {
		return serviceErrorWithCode[database.SchemaMigrationResult](
			http.StatusBadRequest,
			errorCodeSchemaMigrationFailed,
			"Schema migration failed",
			err.Error(),
			"If the engine auto-commits DDL, compare schemas again before retrying.",
		)
	}
	return response.BaseResponse[database.SchemaMigrationResult]{
		Data: database.SchemaMigrationResult{
			Applied:        true,
			StatementCount: len(built.plan.Statements),
			Fingerprint:    built.preview.Fingerprint,
		},
	}
}
