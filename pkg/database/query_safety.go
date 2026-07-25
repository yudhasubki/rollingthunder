package database

import (
	"fmt"
	"strings"
	"unicode"
)

type QuerySafetyAnalysis struct {
	UnfilteredMutations []string
}

func (analysis QuerySafetyAnalysis) RequiresConfirmation() bool {
	return len(analysis.UnfilteredMutations) > 0
}

var writeAccessKeywords = map[string]struct{}{
	"ALTER":                {},
	"ANALYZE":              {},
	"ATTACH":               {},
	"BACKUP":               {},
	"CALL":                 {},
	"CLUSTER":              {},
	"COMMENT":              {},
	"COPY":                 {},
	"CREATE":               {},
	"DBCC":                 {},
	"DELETE":               {},
	"DETACH":               {},
	"DISCARD":              {},
	"DO":                   {},
	"DROP":                 {},
	"EXEC":                 {},
	"EXECUTE":              {},
	"GET_LOCK":             {},
	"GRANT":                {},
	"INSERT":               {},
	"INSTALL":              {},
	"INTO":                 {},
	"KILL":                 {},
	"LOAD":                 {},
	"LOCK":                 {},
	"MERGE":                {},
	"NEXTVAL":              {},
	"PG_ADVISORY_LOCK":     {},
	"PG_TRY_ADVISORY_LOCK": {},
	"PRAGMA":               {},
	"REFRESH":              {},
	"REINDEX":              {},
	"RELEASE_LOCK":         {},
	"RENAME":               {},
	"REPLACE":              {},
	"RESET":                {},
	"RESTORE":              {},
	"REVOKE":               {},
	"SET":                  {},
	"SETVAL":               {},
	"SHUTDOWN":             {},
	"SP_GETAPPLOCK":        {},
	"TRUNCATE":             {},
	"UNINSTALL":            {},
	"UNLOCK":               {},
	"UPDATE":               {},
	"UPDLOCK":              {},
	"UPSERT":               {},
	"USE":                  {},
	"VACUUM":               {},
	"XLOCK":                {},
}

// FindWriteStatement returns the first SQL keyword that requires write access.
// Read-only profiles deliberately use an allow-by-exclusion policy: DML, DDL,
// administrative statements, file-changing pragmas, locking functions, and
// SELECT INTO are rejected before reaching a driver.
func FindWriteStatement(query string) string {
	for _, token := range tokenizeSQLForSafety(query) {
		if _, requiresWrite := writeAccessKeywords[token.word]; requiresWrite {
			return token.word
		}
	}
	return ""
}

type sqlSafetyToken struct {
	word  string
	depth int
}

func AnalyzeQuerySafety(query string) QuerySafetyAnalysis {
	tokens := tokenizeSQLForSafety(query)
	analysis := QuerySafetyAnalysis{}

	for index, token := range tokens {
		if token.word != "UPDATE" && token.word != "DELETE" {
			continue
		}
		if !isMutationCommandToken(tokens, index) {
			continue
		}

		hasWhere := false
		for next := index + 1; next < len(tokens); next++ {
			candidate := tokens[next]
			if candidate.word == ";" && candidate.depth == token.depth {
				break
			}
			if candidate.depth < token.depth {
				break
			}
			if candidate.depth == token.depth && candidate.word == "WHERE" {
				hasWhere = true
				break
			}
		}
		if !hasWhere {
			analysis.UnfilteredMutations = append(
				analysis.UnfilteredMutations,
				token.word,
			)
		}
	}

	return analysis
}

func isMutationCommandToken(
	tokens []sqlSafetyToken,
	index int,
) bool {
	token := tokens[index]
	previous := previousTokenAtDepth(tokens, index, token.depth)
	if token.word == "UPDATE" && (previous == "DO" || previous == "FOR") {
		return false
	}
	if previous == "" {
		return true
	}

	// A data-changing statement can follow one or more top-level CTEs:
	// WITH archived AS (...) DELETE FROM ...
	// Mutations inside a CTE are already detected as the first token at their
	// own parenthesis depth.
	if token.depth != 0 {
		return false
	}
	for start := index - 1; start >= 0; start-- {
		candidate := tokens[start]
		if candidate.depth != 0 {
			continue
		}
		if candidate.word == ";" {
			break
		}
		if candidate.word == "WITH" || candidate.word == "EXPLAIN" {
			return true
		}
	}
	return false
}

func FindTransactionControl(query string) string {
	tokens := tokenizeSQLForSafety(query)
	if len(tokens) > 0 && tokens[0].depth == 0 {
		if tokens[0].word == "DECLARE" {
			return ""
		}
		if tokens[0].word == "BEGIN" &&
			len(tokens) > 1 &&
			tokens[1].depth == 0 &&
			tokens[1].word != ";" &&
			tokens[1].word != "TRANSACTION" &&
			tokens[1].word != "TRAN" &&
			tokens[1].word != "WORK" {
			return ""
		}
	}
	statementStart := true

	for index, token := range tokens {
		if token.depth != 0 {
			continue
		}
		if token.word == ";" {
			statementStart = true
			continue
		}
		if !statementStart {
			continue
		}

		statementStart = false
		switch token.word {
		case "BEGIN":
			if index+1 >= len(tokens) ||
				tokens[index+1].depth != 0 ||
				tokens[index+1].word == ";" ||
				tokens[index+1].word == "TRANSACTION" ||
				tokens[index+1].word == "TRAN" ||
				tokens[index+1].word == "WORK" {
				return token.word
			}
		case "COMMIT",
			"ROLLBACK",
			"SAVEPOINT",
			"RELEASE",
			"ABORT",
			"END":
			return token.word
		case "START":
			if index+1 < len(tokens) &&
				tokens[index+1].depth == 0 &&
				tokens[index+1].word == "TRANSACTION" {
				return "START TRANSACTION"
			}
		}
	}
	return ""
}

// CountSQLStatements counts non-empty top-level SQL statements while ignoring
// semicolons inside strings, comments, parentheses, and PostgreSQL dollar
// quoted bodies.
func CountSQLStatements(query string) int {
	if strings.TrimSpace(query) == "" {
		return 0
	}
	if isRoutineDefinition(query) || isCompoundSQLBlock(query) {
		return 1
	}
	tokens := tokenizeSQLForSafety(query)
	count := 0
	statementHasToken := false
	for _, token := range tokens {
		if token.depth != 0 {
			continue
		}
		if token.word == ";" {
			if statementHasToken {
				count++
				statementHasToken = false
			}
			continue
		}
		statementHasToken = true
	}
	if statementHasToken {
		count++
	}
	return count
}

func HasTopLevelStatementSeparator(query string) bool {
	for _, token := range tokenizeSQLForSafety(query) {
		if token.depth == 0 && token.word == ";" {
			return true
		}
	}
	return false
}

// ValidateDDLFragment accepts a single expression-like SQL fragment while
// rejecting delimiters that could escape a generated column definition. It
// deliberately permits commas and parentheses inside function calls, type
// modifiers, quoted values, and engine-specific quoted literals.
func ValidateDDLFragment(value, label string) error {
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s contains a NUL byte", label)
	}
	depth := 0
	for index := 0; index < len(value); {
		current := value[index]
		next := byte(0)
		if index+1 < len(value) {
			next = value[index+1]
		}
		switch {
		case current == '-' && next == '-':
			index = skipLineComment(value, index+2)
		case current == '/' && next == '*':
			end, closed := skipDDLBlockComment(value, index+2)
			if !closed {
				return fmt.Errorf("%s contains an unterminated comment", label)
			}
			index = end
		case current == '#':
			index = skipLineComment(value, index+1)
		case (current == 'q' || current == 'Q') && next == '\'':
			end, closed := skipDDLOracleAlternativeQuote(value, index)
			if !closed {
				return fmt.Errorf(
					"%s contains an unterminated quoted value",
					label,
				)
			}
			index = end
		case current == '\'' || current == '"' || current == '`':
			end, closed := skipDDLQuoted(value, index, current)
			if !closed {
				return fmt.Errorf(
					"%s contains an unterminated quoted value",
					label,
				)
			}
			index = end
		case current == '[':
			end, closed := skipDDLBracketIdentifier(value, index+1)
			if !closed {
				return fmt.Errorf(
					"%s contains an unterminated quoted identifier",
					label,
				)
			}
			index = end
		case current == '$':
			delimiter, ok := postgresDollarQuoteDelimiter(value[index:])
			if !ok {
				index++
				continue
			}
			start := index + len(delimiter)
			end := strings.Index(value[start:], delimiter)
			if end < 0 {
				return fmt.Errorf(
					"%s contains an unterminated dollar-quoted value",
					label,
				)
			}
			index = start + end + len(delimiter)
		case current == '(':
			depth++
			index++
		case current == ')':
			if depth == 0 {
				return fmt.Errorf(
					"%s escapes its generated column definition",
					label,
				)
			}
			depth--
			index++
		case current == ',' && depth == 0:
			return fmt.Errorf(
				"%s must not add another column or constraint",
				label,
			)
		case current == ';' && depth == 0:
			return fmt.Errorf("%s must not contain another SQL statement", label)
		default:
			index++
		}
	}
	if depth != 0 {
		return fmt.Errorf("%s contains unbalanced parentheses", label)
	}
	return nil
}

func skipDDLQuoted(
	value string,
	start int,
	quote byte,
) (int, bool) {
	for index := start + 1; index < len(value); index++ {
		if value[index] == '\\' && index+1 < len(value) {
			index++
			continue
		}
		if value[index] != quote {
			continue
		}
		if index+1 < len(value) && value[index+1] == quote {
			index++
			continue
		}
		return index + 1, true
	}
	return len(value), false
}

func skipDDLBracketIdentifier(
	value string,
	start int,
) (int, bool) {
	for index := start; index < len(value); index++ {
		if value[index] != ']' {
			continue
		}
		if index+1 < len(value) && value[index+1] == ']' {
			index++
			continue
		}
		return index + 1, true
	}
	return len(value), false
}

func skipDDLBlockComment(
	value string,
	start int,
) (int, bool) {
	depth := 1
	for index := start; index < len(value); {
		if index+1 < len(value) &&
			value[index] == '/' && value[index+1] == '*' {
			depth++
			index += 2
			continue
		}
		if index+1 < len(value) &&
			value[index] == '*' && value[index+1] == '/' {
			depth--
			index += 2
			if depth == 0 {
				return index, true
			}
			continue
		}
		index++
	}
	return len(value), false
}

func skipDDLOracleAlternativeQuote(
	value string,
	start int,
) (int, bool) {
	if start+2 >= len(value) {
		return len(value), false
	}
	open := value[start+2]
	close := open
	switch open {
	case '[':
		close = ']'
	case '{':
		close = '}'
	case '(':
		close = ')'
	case '<':
		close = '>'
	}
	for index := start + 3; index+1 < len(value); index++ {
		if value[index] == close && value[index+1] == '\'' {
			return index + 2, true
		}
	}
	return len(value), false
}

// LeadingSQLKeywords returns the first top-level words in a statement. It is
// intended for validating reviewed DDL templates, not for parsing arbitrary
// SQL grammar.
func LeadingSQLKeywords(query string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	keywords := make([]string, 0, limit)
	for _, token := range tokenizeSQLForSafety(query) {
		if token.depth != 0 {
			continue
		}
		if token.word == ";" {
			break
		}
		keywords = append(keywords, token.word)
		if len(keywords) == limit {
			break
		}
	}
	return keywords
}

func previousTokenAtDepth(
	tokens []sqlSafetyToken,
	index int,
	depth int,
) string {
	for previous := index - 1; previous >= 0; previous-- {
		if tokens[previous].depth < depth || tokens[previous].word == ";" {
			return ""
		}
		if tokens[previous].depth == depth {
			return tokens[previous].word
		}
	}
	return ""
}

func tokenizeSQLForSafety(query string) []sqlSafetyToken {
	tokens := make([]sqlSafetyToken, 0)
	depth := 0

	for index := 0; index < len(query); {
		current := query[index]
		next := byte(0)
		if index+1 < len(query) {
			next = query[index+1]
		}

		switch {
		case current == '-' && next == '-':
			index += 2
			for index < len(query) && query[index] != '\n' {
				index++
			}
		case current == '/' && next == '*':
			index += 2
			for index+1 < len(query) &&
				!(query[index] == '*' && query[index+1] == '/') {
				index++
			}
			if index+1 < len(query) {
				index += 2
			}
		case current == '#':
			index = skipLineComment(query, index+1)
		case (current == 'q' || current == 'Q') &&
			next == '\'':
			index = skipOracleAlternativeQuote(query, index)
		case current == '\'':
			index = skipQuotedSQL(query, index, '\'')
		case current == '"':
			index = skipQuotedSQL(query, index, '"')
		case current == '`':
			index = skipQuotedSQL(query, index, '`')
		case current == '[':
			index = skipBracketIdentifier(query, index+1)
		case current == '$':
			delimiter, ok := postgresDollarQuoteDelimiter(query[index:])
			if !ok {
				index++
				continue
			}
			index += len(delimiter)
			end := strings.Index(query[index:], delimiter)
			if end < 0 {
				index = len(query)
			} else {
				index += end + len(delimiter)
			}
		case current == '(':
			depth++
			index++
		case current == ')':
			if depth > 0 {
				depth--
			}
			index++
		case current == ';':
			tokens = append(tokens, sqlSafetyToken{word: ";", depth: depth})
			index++
		case isSQLWordStart(rune(current)):
			start := index
			index++
			for index < len(query) &&
				isSQLWordPart(rune(query[index])) {
				index++
			}
			tokens = append(tokens, sqlSafetyToken{
				word:  strings.ToUpper(query[start:index]),
				depth: depth,
			})
		default:
			index++
		}
	}

	return tokens
}

func skipQuotedSQL(query string, start int, quote byte) int {
	for index := start + 1; index < len(query); index++ {
		if query[index] != quote {
			continue
		}
		if index+1 < len(query) && query[index+1] == quote {
			index++
			continue
		}
		return index + 1
	}
	return len(query)
}

func postgresDollarQuoteDelimiter(value string) (string, bool) {
	if len(value) < 2 || value[0] != '$' {
		return "", false
	}
	for index := 1; index < len(value); index++ {
		switch {
		case value[index] == '$':
			return value[:index+1], true
		case value[index] == '_' ||
			value[index] >= 'a' && value[index] <= 'z' ||
			value[index] >= 'A' && value[index] <= 'Z' ||
			index > 1 && value[index] >= '0' && value[index] <= '9':
			continue
		default:
			return "", false
		}
	}
	return "", false
}

func isSQLWordStart(value rune) bool {
	return value == '_' || unicode.IsLetter(value)
}

func isSQLWordPart(value rune) bool {
	return isSQLWordStart(value) || unicode.IsDigit(value)
}
