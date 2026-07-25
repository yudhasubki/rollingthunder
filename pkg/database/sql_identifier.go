package database

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// SQLReferencesIdentifier reports whether SQL text contains an identifier
// token with the requested name. It ignores string literals and comments and
// understands the identifier quoting styles used by the supported engines.
// This is deliberately a token-level check, not a full SQL parser.
func SQLReferencesIdentifier(sqlText string, identifier string) bool {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return false
	}

	for offset := 0; offset < len(sqlText); {
		switch {
		case strings.HasPrefix(sqlText[offset:], "--"):
			offset = skipSQLIdentifierLineComment(sqlText, offset+2)
		case strings.HasPrefix(sqlText[offset:], "/*"):
			offset = skipSQLIdentifierBlockComment(sqlText, offset+2)
		case sqlText[offset] == '#':
			offset = skipSQLIdentifierLineComment(sqlText, offset+1)
		case (sqlText[offset] == 'q' || sqlText[offset] == 'Q') &&
			offset+1 < len(sqlText) &&
			sqlText[offset+1] == '\'':
			offset = skipOracleAlternativeQuote(sqlText, offset)
		case sqlText[offset] == '$':
			delimiter, ok := postgresDollarQuoteDelimiter(sqlText[offset:])
			if !ok {
				offset++
				continue
			}
			offset += len(delimiter)
			if end := strings.Index(sqlText[offset:], delimiter); end >= 0 {
				offset += end + len(delimiter)
			} else {
				offset = len(sqlText)
			}
		case sqlText[offset] == '\'':
			_, offset = readSQLIdentifierDelimited(
				sqlText,
				offset,
				'\'',
				true,
			)
		case sqlText[offset] == '"' || sqlText[offset] == '`':
			token, next := readSQLIdentifierDelimited(
				sqlText,
				offset,
				sqlText[offset],
				sqlText[offset] == '`',
			)
			if strings.EqualFold(token, identifier) {
				return true
			}
			offset = next
		case sqlText[offset] == '[':
			token, next := readSQLIdentifierDelimited(
				sqlText,
				offset,
				']',
				false,
			)
			if strings.EqualFold(token, identifier) {
				return true
			}
			offset = next
		default:
			current, size := utf8.DecodeRuneInString(sqlText[offset:])
			if !sqlIdentifierRune(current) {
				offset += size
				continue
			}
			start := offset
			offset += size
			for offset < len(sqlText) {
				current, size = utf8.DecodeRuneInString(sqlText[offset:])
				if !sqlIdentifierRune(current) {
					break
				}
				offset += size
			}
			if strings.EqualFold(sqlText[start:offset], identifier) {
				return true
			}
		}
	}
	return false
}

func sqlIdentifierRune(value rune) bool {
	return value == '_' ||
		value == '$' ||
		unicode.IsLetter(value) ||
		unicode.IsDigit(value)
}

func readSQLIdentifierDelimited(
	value string,
	start int,
	close byte,
	backslashEscapes bool,
) (string, int) {
	offset := start + 1
	var token strings.Builder
	for offset < len(value) {
		if backslashEscapes &&
			value[offset] == '\\' &&
			offset+1 < len(value) {
			token.WriteByte(value[offset+1])
			offset += 2
			continue
		}
		if value[offset] != close {
			token.WriteByte(value[offset])
			offset++
			continue
		}
		if offset+1 < len(value) && value[offset+1] == close {
			token.WriteByte(close)
			offset += 2
			continue
		}
		return token.String(), offset + 1
	}
	return token.String(), len(value)
}

func skipSQLIdentifierLineComment(value string, offset int) int {
	if newline := strings.IndexByte(value[offset:], '\n'); newline >= 0 {
		return offset + newline + 1
	}
	return len(value)
}

func skipSQLIdentifierBlockComment(value string, offset int) int {
	if end := strings.Index(value[offset:], "*/"); end >= 0 {
		return offset + end + 2
	}
	return len(value)
}
