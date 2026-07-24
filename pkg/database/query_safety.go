package database

import (
	"strings"
	"unicode"
)

type QuerySafetyAnalysis struct {
	UnfilteredMutations []string
}

func (analysis QuerySafetyAnalysis) RequiresConfirmation() bool {
	return len(analysis.UnfilteredMutations) > 0
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
		case "BEGIN",
			"COMMIT",
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
		case current == '\'':
			index = skipQuotedSQL(query, index, '\'')
		case current == '"':
			index = skipQuotedSQL(query, index, '"')
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
