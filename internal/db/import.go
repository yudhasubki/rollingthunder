package db

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/response"

	"github.com/google/uuid"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const importBatchSize = 200

var importFileFilters = []wailsruntime.FileFilter{
	{
		DisplayName: "CSV and JSON data (*.csv, *.json, *.jsonl, *.ndjson)",
		Pattern:     "*.csv;*.json;*.jsonl;*.ndjson",
	},
	{
		DisplayName: "CSV files (*.csv)",
		Pattern:     "*.csv",
	},
	{
		DisplayName: "JSON files (*.json, *.jsonl, *.ndjson)",
		Pattern:     "*.json;*.jsonl;*.ndjson",
	},
}

type importFileGrant struct {
	selection database.ImportFileSelection
	path      string
}

func importFormatFromPath(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv":
		return "csv", nil
	case ".json", ".jsonl", ".ndjson":
		return "json", nil
	default:
		return "", fmt.Errorf("only CSV, JSON, JSONL, and NDJSON files are supported")
	}
}

// ChooseImportFile is the only way the frontend receives an import token. The
// selected absolute path stays backend-only so arbitrary files cannot be read
// by sending a crafted Wails request.
func (s *Service) ChooseImportFile() response.BaseResponse[database.ImportFileSelection] {
	if s.ctx == nil {
		return serviceErrorWithCode[database.ImportFileSelection](
			http.StatusServiceUnavailable,
			errorCodeDatabaseOperationFailed,
			"Application is not ready",
			"The native file picker is unavailable before application startup.",
			"Wait for Rolling Thunder to finish starting and try again.",
		)
	}
	path, err := s.importOpenDialog(s.ctx, wailsruntime.OpenDialogOptions{
		Title:                "Import CSV or JSON",
		Filters:              importFileFilters,
		CanCreateDirectories: false,
		ResolvesAliases:      true,
	})
	if err != nil {
		return serviceErrorWithCode[database.ImportFileSelection](
			http.StatusInternalServerError,
			errorCodeDatabaseOperationFailed,
			"Could not choose import file",
			err.Error(),
			"Check file permissions and try the native file picker again.",
		)
	}
	if strings.TrimSpace(path) == "" {
		return response.BaseResponse[database.ImportFileSelection]{}
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return serviceError[database.ImportFileSelection](err.Error())
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return serviceErrorWithCode[database.ImportFileSelection](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Import file is unavailable",
			err.Error(),
			"Choose a readable CSV or JSON file.",
		)
	}
	if !info.Mode().IsRegular() {
		return serviceErrorWithCode[database.ImportFileSelection](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Import source must be a file",
			"The selected path is not a regular file.",
			"Choose a CSV or JSON file.",
		)
	}
	format, err := importFormatFromPath(absolute)
	if err != nil {
		return serviceErrorWithCode[database.ImportFileSelection](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Unsupported import format",
			err.Error(),
			"Choose a .csv, .json, .jsonl, or .ndjson file.",
		)
	}
	selection := database.ImportFileSelection{
		Token:  uuid.NewString(),
		Name:   filepath.Base(absolute),
		Format: format,
		Size:   info.Size(),
	}
	s.importFileMu.Lock()
	s.importFiles[selection.Token] = importFileGrant{
		selection: selection,
		path:      filepath.Clean(absolute),
	}
	s.importFileMu.Unlock()
	return response.BaseResponse[database.ImportFileSelection]{Data: selection}
}

func (s *Service) importFile(token string) (importFileGrant, error) {
	s.importFileMu.RLock()
	grant, ok := s.importFiles[strings.TrimSpace(token)]
	s.importFileMu.RUnlock()
	if !ok {
		return importFileGrant{}, fmt.Errorf(
			"the import file token is invalid or expired; choose the file again",
		)
	}
	return grant, nil
}

type importRecordReader interface {
	Next() (map[string]interface{}, error)
	Columns() []string
}

type csvImportReader struct {
	reader  *csv.Reader
	columns []string
	pending []string
	options database.ImportOptions
}

func parseImportDelimiter(value string) (rune, error) {
	if value == "" {
		return ',', nil
	}
	if value == `\t` {
		return '\t', nil
	}
	delimiter, size := utf8.DecodeRuneInString(value)
	if delimiter == utf8.RuneError || size != len(value) || delimiter == '\r' ||
		delimiter == '\n' || (delimiter != '\t' && unicode.IsSpace(delimiter)) {
		return 0, fmt.Errorf("CSV delimiter must be one visible character or \\\\t")
	}
	return delimiter, nil
}

func uniqueImportColumns(values []string) []string {
	columns := make([]string, len(values))
	used := make(map[string]int, len(values))
	for index, value := range values {
		name := strings.TrimSpace(strings.TrimPrefix(value, "\ufeff"))
		if name == "" {
			name = fmt.Sprintf("column_%d", index+1)
		}
		base := name
		suffix := 2
		for used[name] > 0 {
			name = fmt.Sprintf("%s_%d", base, suffix)
			suffix++
		}
		used[name]++
		columns[index] = name
	}
	return columns
}

func newCSVImportReader(
	source io.Reader,
	options database.ImportOptions,
) (*csvImportReader, error) {
	delimiter, err := parseImportDelimiter(options.Delimiter)
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(source)
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false
	first, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("the CSV file is empty")
		}
		return nil, fmt.Errorf("read CSV header: %w", err)
	}
	result := &csvImportReader{reader: reader, options: options}
	if options.Header {
		result.columns = uniqueImportColumns(first)
	} else {
		names := make([]string, len(first))
		for index := range first {
			names[index] = fmt.Sprintf("column_%d", index+1)
		}
		result.columns = names
		result.pending = first
	}
	return result, nil
}

func (reader *csvImportReader) Columns() []string {
	return append([]string(nil), reader.columns...)
}

func (reader *csvImportReader) Next() (map[string]interface{}, error) {
	record := reader.pending
	reader.pending = nil
	if record == nil {
		var err error
		record, err = reader.reader.Read()
		if err != nil {
			return nil, err
		}
	}
	row := make(map[string]interface{}, len(reader.columns))
	for index, column := range reader.columns {
		value := ""
		if index < len(record) {
			value = record[index]
		}
		if reader.options.EmptyAsNull && value == "" {
			row[column] = nil
		} else {
			row[column] = value
		}
	}
	return row, nil
}

type jsonImportReader struct {
	decoder *json.Decoder
	array   bool
	closed  bool
	columns map[string]struct{}
}

func newJSONImportReader(source io.Reader) (*jsonImportReader, error) {
	buffered := bufio.NewReader(source)
	for {
		peek, err := buffered.Peek(1)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("the JSON file is empty")
			}
			return nil, err
		}
		if !unicode.IsSpace(rune(peek[0])) {
			break
		}
		_, _ = buffered.ReadByte()
	}
	decoder := json.NewDecoder(buffered)
	decoder.UseNumber()
	reader := &jsonImportReader{
		decoder: decoder,
		columns: make(map[string]struct{}),
	}
	first, _ := buffered.Peek(1)
	if len(first) == 1 && first[0] == '[' {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("read JSON array: %w", err)
		}
		if delimiter, ok := token.(json.Delim); !ok || delimiter != '[' {
			return nil, fmt.Errorf("expected a JSON array")
		}
		reader.array = true
	}
	return reader, nil
}

func normalizeJSONImportValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case json.Number:
		if integer, err := typed.Int64(); err == nil {
			return integer
		}
		if decimal, err := typed.Float64(); err == nil {
			return decimal
		}
		return typed.String()
	case map[string]interface{}, []interface{}:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(encoded)
	default:
		return typed
	}
}

func (reader *jsonImportReader) Next() (map[string]interface{}, error) {
	if reader.closed {
		return nil, io.EOF
	}
	if reader.array && !reader.decoder.More() {
		if _, err := reader.decoder.Token(); err != nil {
			return nil, fmt.Errorf("close JSON array: %w", err)
		}
		reader.closed = true
		return nil, io.EOF
	}
	var raw map[string]interface{}
	if err := reader.decoder.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			reader.closed = true
			return nil, io.EOF
		}
		return nil, fmt.Errorf("decode JSON object: %w", err)
	}
	if raw == nil {
		return nil, fmt.Errorf("each JSON record must be an object")
	}
	row := make(map[string]interface{}, len(raw))
	for key, value := range raw {
		name := strings.TrimSpace(key)
		if name == "" {
			return nil, fmt.Errorf("JSON object keys cannot be empty")
		}
		reader.columns[name] = struct{}{}
		row[name] = normalizeJSONImportValue(value)
	}
	return row, nil
}

func (reader *jsonImportReader) Columns() []string {
	columns := make([]string, 0, len(reader.columns))
	for column := range reader.columns {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return columns
}

func openImportRecordReader(
	path string,
	options database.ImportOptions,
) (importRecordReader, io.Closer, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	format := strings.ToLower(strings.TrimSpace(options.Format))
	if format == "" {
		format, err = importFormatFromPath(path)
		if err != nil {
			_ = file.Close()
			return nil, nil, err
		}
	}
	switch format {
	case "csv":
		reader, readerErr := newCSVImportReader(file, options)
		if readerErr != nil {
			_ = file.Close()
			return nil, nil, readerErr
		}
		return reader, file, nil
	case "json":
		reader, readerErr := newJSONImportReader(file)
		if readerErr != nil {
			_ = file.Close()
			return nil, nil, readerErr
		}
		return reader, file, nil
	default:
		_ = file.Close()
		return nil, nil, fmt.Errorf("unsupported import format %q", format)
	}
}

func inferredImportType(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "integer"
	case float32, float64:
		return "number"
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return "text"
		}
		if text == "true" || text == "false" {
			return "boolean"
		}
		if !(len(text) > 1 && text[0] == '0') {
			if _, err := strconv.ParseInt(text, 10, 64); err == nil {
				return "integer"
			}
		}
		if _, err := strconv.ParseFloat(text, 64); err == nil {
			return "number"
		}
		if _, err := time.Parse(time.RFC3339, text); err == nil {
			return "datetime"
		}
		return "text"
	default:
		return "text"
	}
}

func mergeImportTypes(current, next string) string {
	if next == "null" {
		return current
	}
	if current == "" || current == "null" {
		return next
	}
	if current == next {
		return current
	}
	if (current == "integer" && next == "number") ||
		(current == "number" && next == "integer") {
		return "number"
	}
	return "text"
}

func (s *Service) InspectImportFile(
	request database.ImportPreviewRequest,
) response.BaseResponse[database.ImportPreview] {
	grant, err := s.importFile(request.Token)
	if err != nil {
		return serviceErrorWithCode[database.ImportPreview](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Import file is unavailable",
			err.Error(),
			"Choose the source file again.",
		)
	}
	limit := request.Limit
	if limit <= 0 {
		limit = database.DefaultImportPreviewRows
	}
	if limit > 200 {
		limit = 200
	}
	reader, closer, err := openImportRecordReader(grant.path, request.Options)
	if err != nil {
		return serviceError[database.ImportPreview](err.Error())
	}
	defer closer.Close()

	rows := make([]map[string]interface{}, 0, limit)
	types := make(map[string]string)
	seen := make(map[string]int)
	for len(rows) < limit {
		row, readErr := reader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return serviceErrorWithCode[database.ImportPreview](
				http.StatusBadRequest,
				errorCodeInvalidRequest,
				"Could not parse import file",
				readErr.Error(),
				"Check the file format, delimiter, and header settings.",
			)
		}
		rows = append(rows, row)
		for column, value := range row {
			seen[column]++
			types[column] = mergeImportTypes(types[column], inferredImportType(value))
		}
	}
	columns := reader.Columns()
	if len(columns) == 0 {
		return serviceErrorWithCode[database.ImportPreview](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Import file has no columns",
			"No object keys or CSV columns could be detected.",
			"Choose another source or enable the correct CSV header setting.",
		)
	}
	previewColumns := make([]database.ImportColumn, 0, len(columns))
	for _, column := range columns {
		inferred := types[column]
		if inferred == "" || inferred == "null" {
			inferred = "text"
		}
		nullable := seen[column] < len(rows)
		if !nullable {
			for _, row := range rows {
				if row[column] == nil {
					nullable = true
					break
				}
			}
		}
		previewColumns = append(previewColumns, database.ImportColumn{
			SourceName:   column,
			TargetName:   column,
			InferredType: inferred,
			Nullable:     nullable,
			Included:     true,
		})
	}
	return response.BaseResponse[database.ImportPreview]{
		Data: database.ImportPreview{
			File:    grant.selection,
			Columns: previewColumns,
			Rows:    rows,
			Sampled: len(rows),
		},
	}
}

func validateImportColumns(columns []database.ImportColumn) ([]database.ImportColumn, error) {
	included := make([]database.ImportColumn, 0, len(columns))
	targets := make(map[string]struct{}, len(columns))
	sources := make(map[string]struct{}, len(columns))
	for _, column := range columns {
		if !column.Included {
			continue
		}
		column.SourceName = strings.TrimSpace(column.SourceName)
		column.TargetName = strings.TrimSpace(column.TargetName)
		column.InferredType = strings.ToLower(strings.TrimSpace(column.InferredType))
		if column.SourceName == "" || column.TargetName == "" {
			return nil, fmt.Errorf("source and target column names are required")
		}
		if _, duplicate := targets[column.TargetName]; duplicate {
			return nil, fmt.Errorf("target column %q is mapped more than once", column.TargetName)
		}
		if _, duplicate := sources[column.SourceName]; duplicate {
			return nil, fmt.Errorf("source column %q is mapped more than once", column.SourceName)
		}
		switch column.InferredType {
		case "text", "integer", "number", "boolean", "datetime":
		default:
			return nil, fmt.Errorf(
				"unsupported inferred type %q for %s",
				column.InferredType,
				column.SourceName,
			)
		}
		targets[column.TargetName] = struct{}{}
		sources[column.SourceName] = struct{}{}
		included = append(included, column)
	}
	if len(included) == 0 {
		return nil, fmt.Errorf("include at least one column")
	}
	return included, nil
}

func importColumnType(engine string, inferred string) string {
	switch strings.ToLower(engine) {
	case "postgres", "postgresql":
		switch inferred {
		case "integer":
			return "BIGINT"
		case "number":
			return "DOUBLE PRECISION"
		case "boolean":
			return "BOOLEAN"
		case "datetime":
			return "TIMESTAMPTZ"
		default:
			return "TEXT"
		}
	case "mysql", "mariadb":
		switch inferred {
		case "integer":
			return "BIGINT"
		case "number":
			return "DOUBLE"
		case "boolean":
			return "BOOLEAN"
		case "datetime":
			return "DATETIME"
		default:
			return "TEXT"
		}
	default:
		switch inferred {
		case "integer", "boolean":
			return "INTEGER"
		case "number":
			return "REAL"
		default:
			return "TEXT"
		}
	}
}

func qualifiedImportTable(
	driver database.CapabilityDriver,
	schema string,
	table string,
) string {
	if strings.TrimSpace(schema) == "" {
		return driver.QuoteIdentifier(table)
	}
	return driver.QuoteIdentifier(schema) + "." + driver.QuoteIdentifier(table)
}

func buildImportCreateStatement(
	driver database.CapabilityDriver,
	schema string,
	table string,
	columns []database.ImportColumn,
) string {
	engine := driver.Capabilities().Engine
	definitions := make([]string, 0, len(columns))
	for _, column := range columns {
		definition := driver.QuoteIdentifier(column.TargetName) +
			" " + importColumnType(engine, column.InferredType)
		if !column.Nullable {
			definition += " NOT NULL"
		}
		definitions = append(definitions, definition)
	}
	return "CREATE TABLE " + qualifiedImportTable(driver, schema, table) +
		" (" + strings.Join(definitions, ", ") + ")"
}

func coerceImportedValue(value interface{}, inferred string) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	text, isText := value.(string)
	if !isText {
		return value, nil
	}
	if text == "" {
		return text, nil
	}
	switch inferred {
	case "integer":
		number, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("expected integer, got %q", text)
		}
		return number, nil
	case "number":
		number, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil {
			return nil, fmt.Errorf("expected number, got %q", text)
		}
		return number, nil
	case "boolean":
		value, err := strconv.ParseBool(strings.TrimSpace(text))
		if err != nil {
			return nil, fmt.Errorf("expected boolean, got %q", text)
		}
		return value, nil
	default:
		return text, nil
	}
}

func flushImportRows(
	ctx context.Context,
	transaction database.Transaction,
	driver database.CapabilityDriver,
	schema string,
	table string,
	columns []database.ImportColumn,
	rows []map[string]interface{},
) error {
	if len(rows) == 0 {
		return nil
	}
	columnSQL := make([]string, len(columns))
	for index, column := range columns {
		columnSQL[index] = driver.QuoteIdentifier(column.TargetName)
	}
	args := make([]interface{}, 0, len(rows)*len(columns))
	valueGroups := make([]string, 0, len(rows))
	position := 1
	for rowIndex, row := range rows {
		placeholders := make([]string, len(columns))
		for columnIndex, column := range columns {
			value, err := coerceImportedValue(row[column.SourceName], column.InferredType)
			if err != nil {
				return fmt.Errorf(
					"row %d column %s: %w",
					rowIndex+1,
					column.SourceName,
					err,
				)
			}
			args = append(args, value)
			placeholders[columnIndex] = driver.Placeholder(position)
			position++
		}
		valueGroups = append(valueGroups, "("+strings.Join(placeholders, ", ")+")")
	}
	query := "INSERT INTO " + qualifiedImportTable(driver, schema, table) +
		" (" + strings.Join(columnSQL, ", ") + ") VALUES " +
		strings.Join(valueGroups, ", ")
	_, err := transaction.ExecuteQuery(ctx, query, database.QueryOptions{
		Args: args,
	})
	return err
}

func (s *Service) ImportData(
	request database.ImportRequest,
) response.BaseResponse[database.ImportResult] {
	request.Schema = strings.TrimSpace(request.Schema)
	request.Table = strings.TrimSpace(request.Table)
	if request.ConnectionID == "" || request.Table == "" {
		return serviceErrorWithCode[database.ImportResult](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Import target is incomplete",
			"A connection and target table are required.",
			"Choose a namespace and enter or select a table.",
		)
	}
	grant, err := s.importFile(request.Token)
	if err != nil {
		return serviceErrorWithCode[database.ImportResult](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Import file is unavailable",
			err.Error(),
			"Choose the source file again.",
		)
	}
	columns, err := validateImportColumns(request.Columns)
	if err != nil {
		return serviceErrorWithCode[database.ImportResult](
			http.StatusBadRequest,
			errorCodeInvalidRequest,
			"Invalid column mapping",
			err.Error(),
			"Review included columns, target names, and inferred types.",
		)
	}
	connection, release, err := s.pinnedConnection(request.ConnectionID)
	if err != nil {
		return serviceError[database.ImportResult](err.Error())
	}
	defer release()

	transactional, ok := connection.Driver.(database.TransactionalDriver)
	if !ok {
		return serviceErrorWithCode[database.ImportResult](
			http.StatusNotImplemented,
			errorCodeDatabaseOperationFailed,
			"Import is not supported",
			"The active driver does not support transactional imports.",
			"Use a supported PostgreSQL, MySQL, MariaDB, or SQLite connection.",
		)
	}
	if !request.CreateTable {
		structures, structureErr := connection.Driver.GetCollectionStructures(database.Table{
			Schema: request.Schema,
			Name:   request.Table,
		})
		if structureErr != nil {
			return serviceError[database.ImportResult](structureErr.Error())
		}
		available := make(map[string]struct{}, len(structures))
		for _, structure := range structures {
			available[structure.Name] = struct{}{}
		}
		for _, column := range columns {
			if _, exists := available[column.TargetName]; !exists {
				return serviceErrorWithCode[database.ImportResult](
					http.StatusBadRequest,
					errorCodeInvalidRequest,
					"Column mapping does not match the table",
					fmt.Sprintf("target column %q does not exist", column.TargetName),
					"Map every included source column to an existing target column.",
				)
			}
		}
	}

	reader, closer, err := openImportRecordReader(grant.path, request.Options)
	if err != nil {
		return serviceError[database.ImportResult](err.Error())
	}
	defer closer.Close()

	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	warnings := make([]string, 0, 1)
	tableCreated := false
	capabilities := connection.Driver.Capabilities()
	if request.CreateTable && !capabilities.TransactionalDDL {
		definitions := make([]database.ColumnDefinition, 0, len(columns))
		for _, column := range columns {
			definitions = append(definitions, database.ColumnDefinition{
				Name:     column.TargetName,
				Type:     importColumnType(capabilities.Engine, column.InferredType),
				Nullable: column.Nullable,
			})
		}
		if err := connection.Driver.CreateTable(
			database.Table{Schema: request.Schema, Name: request.Table},
			definitions,
		); err != nil {
			return serviceError[database.ImportResult](err.Error())
		}
		tableCreated = true
		warnings = append(
			warnings,
			"The table DDL is not transactional on this engine. If row import fails, the empty table remains.",
		)
	}

	transaction, err := transactional.BeginTransaction(ctx)
	if err != nil {
		return serviceError[database.ImportResult](err.Error())
	}
	committed := false
	defer func() {
		if !committed {
			_ = transaction.Rollback()
		}
	}()

	if request.CreateTable && capabilities.TransactionalDDL {
		if _, err := transaction.ExecuteQuery(
			ctx,
			buildImportCreateStatement(connection.Driver, request.Schema, request.Table, columns),
			database.QueryOptions{},
		); err != nil {
			return serviceError[database.ImportResult](err.Error())
		}
		tableCreated = true
	}

	inserted := 0
	batch := make([]map[string]interface{}, 0, importBatchSize)
	for {
		row, readErr := reader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return serviceErrorWithCode[database.ImportResult](
				http.StatusBadRequest,
				errorCodeInvalidRequest,
				"Could not parse import file",
				readErr.Error(),
				"No rows were committed. Check the source format and try again.",
			)
		}
		batch = append(batch, row)
		if len(batch) < importBatchSize {
			continue
		}
		if err := flushImportRows(
			ctx,
			transaction,
			connection.Driver,
			request.Schema,
			request.Table,
			columns,
			batch,
		); err != nil {
			return serviceErrorWithCode[database.ImportResult](
				http.StatusConflict,
				errorCodeDatabaseOperationFailed,
				"Could not import rows",
				fmt.Sprintf("rows %d–%d: %v", inserted+1, inserted+len(batch), err),
				"No imported rows were committed. Review type mappings and constraints.",
			)
		}
		inserted += len(batch)
		batch = batch[:0]
	}
	if err := flushImportRows(
		ctx,
		transaction,
		connection.Driver,
		request.Schema,
		request.Table,
		columns,
		batch,
	); err != nil {
		return serviceErrorWithCode[database.ImportResult](
			http.StatusConflict,
			errorCodeDatabaseOperationFailed,
			"Could not import rows",
			fmt.Sprintf("rows %d–%d: %v", inserted+1, inserted+len(batch), err),
			"No imported rows were committed. Review type mappings and constraints.",
		)
	}
	inserted += len(batch)
	if err := transaction.Commit(); err != nil {
		return serviceError[database.ImportResult](err.Error())
	}
	committed = true

	s.importFileMu.Lock()
	delete(s.importFiles, request.Token)
	s.importFileMu.Unlock()
	return response.BaseResponse[database.ImportResult]{
		Data: database.ImportResult{
			Schema:       request.Schema,
			Table:        request.Table,
			RowsInserted: inserted,
			TableCreated: tableCreated,
			Warnings:     warnings,
		},
	}
}
