package oracle

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"rollingthunder/pkg/database"
	"rollingthunder/pkg/database/sqladapter"
)

func quoteOracleText(value string) string {
	quoted := "'" + strings.ReplaceAll(value, "'", "''") + "'"
	if len([]byte(value)) <= 1900 {
		return quoted
	}
	chunks := make([]string, 0, len(value)/1900+1)
	start := 0
	size := 0
	for index, runeValue := range value {
		runeSize := utf8.RuneLen(runeValue)
		if size+runeSize <= 1900 {
			size += runeSize
			continue
		}
		chunks = append(
			chunks,
			"TO_CLOB('"+
				strings.ReplaceAll(value[start:index], "'", "''")+
				"')",
		)
		start = index
		size = runeSize
	}
	if start < len(value) {
		chunks = append(
			chunks,
			"TO_CLOB('"+
				strings.ReplaceAll(value[start:], "'", "''")+
				"')",
		)
	}
	return strings.Join(chunks, " || ")
}

func oracleTimeLiteral(
	value time.Time,
	column database.Structure,
) string {
	dataType := strings.ToUpper(column.DataType)
	switch {
	case strings.Contains(dataType, "WITH TIME ZONE"):
		return "TO_TIMESTAMP_TZ('" +
			value.Format("2006-01-02 15:04:05.000000000 -07:00") +
			"', 'YYYY-MM-DD HH24:MI:SS.FF9 TZH:TZM')"
	case strings.HasPrefix(dataType, "DATE"):
		return "TO_DATE('" +
			value.Format("2006-01-02 15:04:05") +
			"', 'YYYY-MM-DD HH24:MI:SS')"
	default:
		return "TO_TIMESTAMP('" +
			value.Format("2006-01-02 15:04:05.000000000") +
			"', 'YYYY-MM-DD HH24:MI:SS.FF9')"
	}
}

func oracleSQLLiteral(
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
			return "TRUE", nil
		}
		return "FALSE", nil
	case string:
		return quoteOracleText(typed), nil
	case []byte:
		dataType := strings.ToUpper(column.DataType)
		if strings.Contains(dataType, "CHAR") ||
			strings.Contains(dataType, "CLOB") ||
			strings.Contains(dataType, "JSON") {
			return quoteOracleText(string(typed)), nil
		}
		if len(typed) > 2000 {
			return "", fmt.Errorf(
				"binary value is %d bytes; Oracle inline RAW export is limited to 2000 bytes",
				len(typed),
			)
		}
		return "HEXTORAW('" + strings.ToUpper(hex.EncodeToString(typed)) +
			"')", nil
	case time.Time:
		return oracleTimeLiteral(typed, column), nil
	default:
		return "", fmt.Errorf(
			"unsupported Oracle SQL literal type %T",
			value,
		)
	}
}

func oracleInsertExportDialect() *sqladapter.InsertExportDialect {
	return &sqladapter.InsertExportDialect{
		EngineLabel:      "Oracle",
		QuoteIdentifier:  quoteIdentifier,
		QuoteQualified:   quoteQualified,
		Literal:          oracleSQLLiteral,
		BeginStatement:   "SET TRANSACTION READ WRITE;",
		CommitStatement:  "COMMIT;",
		MultiRowValues:   false,
		MaximumBatchSize: 1,
	}
}
