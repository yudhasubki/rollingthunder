package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"
)

type dataSyncPlan struct {
	preview database.DataSyncPreview
	changes database.TableChangeSet
}

func dataSyncContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, objectChangeTimeout)
}

func normalizedDataSyncLimit(value int) int {
	if value <= 0 {
		return database.DefaultDataSyncRowLimit
	}
	return min(value, database.MaxDataSyncRowLimit)
}

func normalizeDataSyncNames(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func canonicalDataValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	case *time.Time:
		if typed == nil {
			return nil
		}
		return typed.UTC().Format(time.RFC3339Nano)
	case map[string]interface{}:
		result := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			result[key] = canonicalDataValue(item)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(typed))
		for index, item := range typed {
			result[index] = canonicalDataValue(item)
		}
		return result
	default:
		return value
	}
}

func canonicalDataJSON(value interface{}) string {
	encoded, err := json.Marshal(canonicalDataValue(value))
	if err != nil {
		return fmt.Sprintf("%T:%v", value, value)
	}
	return string(encoded)
}

func dataSyncKey(
	row map[string]interface{},
	keyColumns []string,
) (string, map[string]interface{}, error) {
	values := make([]interface{}, 0, len(keyColumns))
	key := make(map[string]interface{}, len(keyColumns))
	for _, column := range keyColumns {
		value, exists := row[column]
		if !exists {
			return "", nil, fmt.Errorf("key column %q is missing from query results", column)
		}
		value = canonicalDataValue(value)
		key[column] = value
		values = append(values, value)
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", nil, fmt.Errorf("encode row key: %w", err)
	}
	return string(encoded), key, nil
}

func subsetDataRow(
	row map[string]interface{},
	columns []string,
) map[string]interface{} {
	result := make(map[string]interface{}, len(columns))
	for _, column := range columns {
		result[column] = row[column]
	}
	return result
}

func dataSyncChangeID(kind, key string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + key))
	return hex.EncodeToString(sum[:12])
}

func dataSyncFingerprint(
	request database.DataSyncRequest,
	preview database.DataSyncPreview,
) string {
	payload := struct {
		Request        database.DataSyncRequest
		KeyColumns     []string
		CompareColumns []string
		Changes        []database.DataSyncChange
	}{
		Request:        request,
		KeyColumns:     preview.KeyColumns,
		CompareColumns: preview.CompareColumns,
		Changes:        preview.Changes,
	}
	encoded, _ := json.Marshal(canonicalDataValue(payload))
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func structureNames(
	structures database.Structures,
) (map[string]database.Structure, []string) {
	byName := make(map[string]database.Structure, len(structures))
	order := make([]string, 0, len(structures))
	for _, structure := range structures {
		byName[strings.ToLower(structure.Name)] = structure
		order = append(order, structure.Name)
	}
	return byName, order
}

func resolveDataSyncColumns(
	request database.DataSyncRequest,
	source database.Structures,
	target database.Structures,
) ([]string, []string, []string, error) {
	sourceByName, sourceOrder := structureNames(source)
	targetByName, _ := structureNames(target)

	keys := normalizeDataSyncNames(request.KeyColumns)
	if len(keys) == 0 {
		for _, structure := range source {
			if structure.IsPrimary {
				keys = append(keys, structure.Name)
			}
		}
	}
	if len(keys) == 0 {
		return nil, nil, nil, fmt.Errorf(
			"source table has no primary key; choose one or more stable key columns",
		)
	}
	for index, key := range keys {
		sourceColumn, sourceExists := sourceByName[strings.ToLower(key)]
		targetColumn, targetExists := targetByName[strings.ToLower(key)]
		if !sourceExists || !targetExists {
			return nil, nil, nil, fmt.Errorf(
				"key column %q must exist in both tables",
				key,
			)
		}
		keys[index] = sourceColumn.Name
		if sourceColumn.Name != targetColumn.Name {
			return nil, nil, nil, fmt.Errorf(
				"cross-engine key column casing must match exactly for %q",
				key,
			)
		}
	}

	compare := normalizeDataSyncNames(request.CompareColumns)
	if len(compare) == 0 {
		for _, name := range sourceOrder {
			targetColumn, exists := targetByName[strings.ToLower(name)]
			if !exists ||
				targetColumn.IsGenerated ||
				targetColumn.IsAutoInc {
				continue
			}
			compare = append(compare, name)
		}
	}
	if len(compare) == 0 {
		return nil, nil, nil, fmt.Errorf("tables have no writable columns in common")
	}
	for index, name := range compare {
		sourceColumn, sourceExists := sourceByName[strings.ToLower(name)]
		targetColumn, targetExists := targetByName[strings.ToLower(name)]
		if !sourceExists || !targetExists {
			return nil, nil, nil, fmt.Errorf(
				"compare column %q must exist in both tables",
				name,
			)
		}
		if targetColumn.IsGenerated || targetColumn.IsAutoInc {
			return nil, nil, nil, fmt.Errorf(
				"generated or identity target column %q cannot be synchronized",
				targetColumn.Name,
			)
		}
		if sourceColumn.Name != targetColumn.Name {
			return nil, nil, nil, fmt.Errorf(
				"source and target column names must match exactly for %q",
				name,
			)
		}
		compare[index] = sourceColumn.Name
	}
	writeColumns := append([]string(nil), compare...)
	for _, key := range keys {
		if !slicesContainsFold(writeColumns, key) {
			writeColumns = append(writeColumns, key)
		}
	}
	return keys, compare, writeColumns, nil
}

func slicesContainsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func readDataSyncRows(
	ctx context.Context,
	driver database.Driver,
	table database.Table,
	keyColumns []string,
	limit int,
) ([]map[string]interface{}, bool, error) {
	query := "SELECT * FROM " + qualifiedImportTable(
		driver,
		table.Schema,
		table.Name,
	)
	if len(keyColumns) > 0 {
		order := make([]string, 0, len(keyColumns))
		for _, column := range keyColumns {
			order = append(order, driver.QuoteIdentifier(column))
		}
		query += " ORDER BY " + strings.Join(order, ", ")
	}
	result, err := driver.ExecuteQuery(ctx, query, database.QueryOptions{
		MaxRows: limit + 1,
	})
	if err != nil {
		return nil, false, err
	}
	truncated := result.Truncated || len(result.Rows) > limit
	if len(result.Rows) > limit {
		result.Rows = result.Rows[:limit]
	}
	return result.Rows, truncated, nil
}

func indexDataSyncRows(
	rows []map[string]interface{},
	keyColumns []string,
) (map[string]map[string]interface{}, map[string]map[string]interface{}, error) {
	index := make(map[string]map[string]interface{}, len(rows))
	keys := make(map[string]map[string]interface{}, len(rows))
	for _, row := range rows {
		encoded, key, err := dataSyncKey(row, keyColumns)
		if err != nil {
			return nil, nil, err
		}
		if _, duplicate := index[encoded]; duplicate {
			return nil, nil, fmt.Errorf(
				"duplicate key %s prevents deterministic synchronization",
				encoded,
			)
		}
		index[encoded] = row
		keys[encoded] = key
	}
	return index, keys, nil
}

func changedDataSyncColumns(
	source map[string]interface{},
	target map[string]interface{},
	columns []string,
) []string {
	changed := make([]string, 0)
	for _, column := range columns {
		if canonicalDataJSON(source[column]) != canonicalDataJSON(target[column]) {
			changed = append(changed, column)
		}
	}
	return changed
}

func (s *Service) buildDataSync(
	ctx context.Context,
	request database.DataSyncRequest,
) (dataSyncPlan, error) {
	sourceDriver, sourceRelease, err := s.driverFor(request.SourceConnectionID)
	if err != nil {
		return dataSyncPlan{}, fmt.Errorf("source connection: %w", err)
	}
	defer sourceRelease()
	targetDriver, targetRelease, err := s.driverFor(request.TargetConnectionID)
	if err != nil {
		return dataSyncPlan{}, fmt.Errorf("target connection: %w", err)
	}
	defer targetRelease()

	sourceTable := database.Table{
		Schema: strings.TrimSpace(request.SourceSchema),
		Name:   strings.TrimSpace(request.SourceTable),
	}
	targetTable := database.Table{
		Schema: strings.TrimSpace(request.TargetSchema),
		Name:   strings.TrimSpace(request.TargetTable),
	}
	sourceStructures, err := sourceDriver.GetCollectionStructures(sourceTable)
	if err != nil {
		return dataSyncPlan{}, fmt.Errorf("source columns: %w", err)
	}
	targetStructures, err := targetDriver.GetCollectionStructures(targetTable)
	if err != nil {
		return dataSyncPlan{}, fmt.Errorf("target columns: %w", err)
	}
	keys, compareColumns, writeColumns, err := resolveDataSyncColumns(
		request,
		sourceStructures,
		targetStructures,
	)
	if err != nil {
		return dataSyncPlan{}, err
	}

	limit := normalizedDataSyncLimit(request.MaxRows)
	sourceRows, sourceTruncated, err := readDataSyncRows(
		ctx,
		sourceDriver,
		sourceTable,
		keys,
		limit,
	)
	if err != nil {
		return dataSyncPlan{}, fmt.Errorf("source rows: %w", err)
	}
	targetRows, targetTruncated, err := readDataSyncRows(
		ctx,
		targetDriver,
		targetTable,
		keys,
		limit,
	)
	if err != nil {
		return dataSyncPlan{}, fmt.Errorf("target rows: %w", err)
	}
	sourceIndex, sourceKeys, err := indexDataSyncRows(sourceRows, keys)
	if err != nil {
		return dataSyncPlan{}, fmt.Errorf("source rows: %w", err)
	}
	targetIndex, targetKeys, err := indexDataSyncRows(targetRows, keys)
	if err != nil {
		return dataSyncPlan{}, fmt.Errorf("target rows: %w", err)
	}

	encodedKeys := make([]string, 0, len(sourceIndex)+len(targetIndex))
	seen := make(map[string]struct{}, len(sourceIndex)+len(targetIndex))
	for key := range sourceIndex {
		seen[key] = struct{}{}
		encodedKeys = append(encodedKeys, key)
	}
	for key := range targetIndex {
		if _, exists := seen[key]; !exists {
			encodedKeys = append(encodedKeys, key)
		}
	}
	sort.Strings(encodedKeys)

	preview := database.DataSyncPreview{
		SourceEngine:   sourceDriver.Capabilities().Engine,
		TargetEngine:   targetDriver.Capabilities().Engine,
		KeyColumns:     append([]string(nil), keys...),
		CompareColumns: append([]string(nil), compareColumns...),
		Changes:        make([]database.DataSyncChange, 0),
		SourceRows:     len(sourceRows),
		TargetRows:     len(targetRows),
		Truncated:      sourceTruncated || targetTruncated,
		SafeToApply:    !sourceTruncated && !targetTruncated,
		Warnings:       make([]string, 0),
	}
	changeSet := database.TableChangeSet{
		Table:   targetTable,
		Added:   make([]map[string]interface{}, 0),
		Updated: make([]database.RowUpdate, 0),
		Deleted: make([]map[string]interface{}, 0),
	}
	for _, encodedKey := range encodedKeys {
		sourceRow, sourceExists := sourceIndex[encodedKey]
		targetRow, targetExists := targetIndex[encodedKey]
		switch {
		case sourceExists && !targetExists:
			source := subsetDataRow(sourceRow, writeColumns)
			preview.Changes = append(preview.Changes, database.DataSyncChange{
				ID:     dataSyncChangeID("insert", encodedKey),
				Kind:   "insert",
				Key:    sourceKeys[encodedKey],
				Source: source,
			})
			changeSet.Added = append(changeSet.Added, source)
			preview.Added++
		case !sourceExists && targetExists:
			target := subsetDataRow(targetRow, writeColumns)
			preview.Changes = append(preview.Changes, database.DataSyncChange{
				ID:     dataSyncChangeID("delete", encodedKey),
				Kind:   "delete",
				Key:    targetKeys[encodedKey],
				Target: target,
			})
			changeSet.Deleted = append(changeSet.Deleted, target)
			preview.Deleted++
		case sourceExists && targetExists:
			changed := changedDataSyncColumns(sourceRow, targetRow, compareColumns)
			if len(changed) == 0 {
				continue
			}
			source := subsetDataRow(sourceRow, writeColumns)
			target := subsetDataRow(targetRow, writeColumns)
			preview.Changes = append(preview.Changes, database.DataSyncChange{
				ID:             dataSyncChangeID("update", encodedKey),
				Kind:           "update",
				Key:            sourceKeys[encodedKey],
				Source:         source,
				Target:         target,
				ChangedColumns: changed,
			})
			changeSet.Updated = append(changeSet.Updated, database.RowUpdate{
				Original:       target,
				Values:         source,
				ChangedColumns: changed,
			})
			preview.Updated++
		}
	}
	if preview.Truncated {
		preview.Warnings = append(
			preview.Warnings,
			fmt.Sprintf(
				"At least one table exceeded %d rows. Increase the reviewed limit or narrow the tables before applying.",
				limit,
			),
		)
	}
	if preview.SourceEngine != preview.TargetEngine {
		preview.Warnings = append(
			preview.Warnings,
			"Source and target engines differ. Review type conversions carefully before applying.",
		)
	}
	request.KeyColumns = append([]string(nil), keys...)
	request.CompareColumns = append([]string(nil), compareColumns...)
	request.MaxRows = limit
	preview.Fingerprint = dataSyncFingerprint(request, preview)
	return dataSyncPlan{preview: preview, changes: changeSet}, nil
}

func (s *Service) PreviewDataSync(
	request database.DataSyncRequest,
) response.BaseResponse[database.DataSyncPreview] {
	if err := request.Validate(); err != nil {
		return serviceErrorWithCode[database.DataSyncPreview](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid data comparison",
			err.Error(),
			"Choose source and target tables, then review the row limit.",
		)
	}
	ctx, cancel := dataSyncContext(s.ctx)
	defer cancel()
	plan, err := s.buildDataSync(ctx, request)
	if err != nil {
		return serviceErrorWithCode[database.DataSyncPreview](
			http.StatusBadRequest,
			errorCodeDataSyncFailed,
			"Could not compare table data",
			err.Error(),
			"Verify table columns, stable keys, permissions, and connection health.",
		)
	}
	return response.BaseResponse[database.DataSyncPreview]{Data: plan.preview}
}

func selectedDataSyncChanges(
	plan dataSyncPlan,
	selected []string,
) (database.TableChangeSet, error) {
	if len(selected) == 0 {
		return database.TableChangeSet{}, fmt.Errorf(
			"select at least one reviewed change before applying data sync",
		)
	}
	requested := make(map[string]struct{}, len(selected))
	for _, id := range selected {
		requested[strings.TrimSpace(id)] = struct{}{}
	}
	result := database.TableChangeSet{
		Table:   plan.changes.Table,
		Added:   make([]map[string]interface{}, 0),
		Updated: make([]database.RowUpdate, 0),
		Deleted: make([]map[string]interface{}, 0),
	}
	insertIndex := 0
	updateIndex := 0
	deleteIndex := 0
	for _, change := range plan.preview.Changes {
		var value interface{}
		switch change.Kind {
		case "insert":
			value = plan.changes.Added[insertIndex]
			insertIndex++
		case "update":
			value = plan.changes.Updated[updateIndex]
			updateIndex++
		case "delete":
			value = plan.changes.Deleted[deleteIndex]
			deleteIndex++
		}
		if _, include := requested[change.ID]; !include {
			continue
		}
		delete(requested, change.ID)
		switch typed := value.(type) {
		case map[string]interface{}:
			if change.Kind == "insert" {
				result.Added = append(result.Added, typed)
			} else {
				result.Deleted = append(result.Deleted, typed)
			}
		case database.RowUpdate:
			result.Updated = append(result.Updated, typed)
		}
	}
	if len(requested) > 0 {
		return database.TableChangeSet{}, fmt.Errorf(
			"one or more selected changes are no longer present",
		)
	}
	return result, nil
}

func (s *Service) ApplyDataSync(
	request database.ApplyDataSyncRequest,
) response.BaseResponse[database.DataSyncResult] {
	if err := request.Sync.Validate(); err != nil {
		return serviceErrorWithCode[database.DataSyncResult](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid data synchronization",
			err.Error(),
			"Return to the comparison and generate a fresh preview.",
		)
	}
	if strings.TrimSpace(request.Fingerprint) == "" {
		return serviceErrorWithCode[database.DataSyncResult](
			http.StatusConflict,
			errorCodeDataSyncReview,
			"Data sync review required",
			"The row changes have not been reviewed.",
			"Compare the tables and review the selected inserts, updates, and deletes.",
		)
	}
	ctx, cancel := dataSyncContext(s.ctx)
	defer cancel()
	plan, err := s.buildDataSync(ctx, request.Sync)
	if err != nil {
		return serviceErrorWithCode[database.DataSyncResult](
			http.StatusBadRequest,
			errorCodeDataSyncFailed,
			"Could not refresh data comparison",
			err.Error(),
			"Compare the tables again before applying any changes.",
		)
	}
	if !reviewedFingerprintMatches(request.Fingerprint, plan.preview.Fingerprint) {
		return serviceErrorWithCode[database.DataSyncResult](
			http.StatusConflict,
			errorCodeDataSyncReview,
			"Table data changed after review",
			"The current row diff no longer matches the reviewed preview.",
			"Review the refreshed comparison before synchronizing.",
		)
	}
	if !plan.preview.SafeToApply {
		return serviceErrorWithCode[database.DataSyncResult](
			http.StatusConflict,
			errorCodeDataSyncReview,
			"Truncated comparison cannot be applied",
			"Rolling Thunder did not compare every row in both tables.",
			"Raise the row limit or narrow the data set, then review a complete comparison.",
		)
	}
	changes, err := selectedDataSyncChanges(plan, request.SelectedChangeIDs)
	if err != nil {
		return serviceErrorWithCode[database.DataSyncResult](
			http.StatusConflict,
			errorCodeDataSyncReview,
			"Selected changes are stale",
			err.Error(),
			"Review the refreshed comparison before synchronizing.",
		)
	}
	if changes.Count() == 0 {
		return response.BaseResponse[database.DataSyncResult]{
			Data: database.DataSyncResult{
				Applied:     true,
				Fingerprint: plan.preview.Fingerprint,
			},
		}
	}
	targetDriver, release, err := s.writeDriverFor(
		request.Sync.TargetConnectionID,
	)
	if err != nil {
		if err == errConnectionReadOnly {
			return readOnlyConnectionError[database.DataSyncResult]()
		}
		return serviceError[database.DataSyncResult](err.Error())
	}
	defer release()
	changeDriver, ok := targetDriver.(database.TableChangeDriver)
	if !ok {
		return serviceErrorWithCode[database.DataSyncResult](
			http.StatusNotImplemented,
			errorCodeDataSyncUnsupported,
			"Atomic data sync is unavailable",
			"The target driver cannot apply reviewed row changes atomically.",
			"Use a connected target whose driver advertises atomic table changes.",
		)
	}
	result, err := changeDriver.ApplyTableChanges(ctx, changes)
	if err != nil {
		return serviceErrorWithCode[database.DataSyncResult](
			http.StatusConflict,
			errorCodeDataSyncFailed,
			"Data synchronization failed",
			err.Error(),
			"The complete change set was rolled back. Refresh both tables before retrying.",
		)
	}
	return response.BaseResponse[database.DataSyncResult]{
		Data: database.DataSyncResult{
			Applied:     true,
			Inserted:    result.Inserted,
			Updated:     result.Updated,
			Deleted:     result.Deleted,
			Fingerprint: plan.preview.Fingerprint,
		},
	}
}
