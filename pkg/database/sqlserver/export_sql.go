package sqlserver

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/sqladapter"
)

func quoteSQLServerText(value string) string {
	quoted := "N'" + strings.ReplaceAll(value, "'", "''") + "'"
	if utf8.RuneCountInString(value) <= 1800 {
		return quoted
	}
	runes := []rune(value)
	chunks := make([]string, 0, len(runes)/1800+1)
	for start := 0; start < len(runes); start += 1800 {
		end := min(start+1800, len(runes))
		chunk := strings.ReplaceAll(
			string(runes[start:end]),
			"'",
			"''",
		)
		if start == 0 {
			chunks = append(
				chunks,
				"CAST(N'"+chunk+"' AS nvarchar(max))",
			)
		} else {
			chunks = append(chunks, "N'"+chunk+"'")
		}
	}
	return strings.Join(chunks, " + ")
}

func sqlServerSQLLiteral(
	value interface{},
	column database.Structure,
) (string, error) {
	if value == nil {
		return "NULL", nil
	}
	if number, ok, err := sqladapter.SQLNumericLiteral(value); ok {
		return number, err
	}
	switch typed := value.(type) {
	case bool:
		if typed {
			return "1", nil
		}
		return "0", nil
	case string:
		return quoteSQLServerText(typed), nil
	case []byte:
		return "0x" + strings.ToUpper(hex.EncodeToString(typed)), nil
	case time.Time:
		return quoteSQLServerText(
			typed.Format(time.RFC3339Nano),
		), nil
	default:
		return "", fmt.Errorf(
			"unsupported SQL Server SQL literal type %T for %s",
			value,
			column.Name,
		)
	}
}

func sqlServerInsertExportDialect() *sqladapter.InsertExportDialect {
	return &sqladapter.InsertExportDialect{
		EngineLabel:      "SQL Server",
		QuoteIdentifier:  quoteIdentifier,
		QuoteQualified:   quoteQualified,
		Literal:          sqlServerSQLLiteral,
		BeginStatement:   "BEGIN TRANSACTION;",
		CommitStatement:  "COMMIT TRANSACTION;",
		MultiRowValues:   true,
		MaximumBatchSize: 1000,
	}
}
