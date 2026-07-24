package database

import (
	"fmt"
	"regexp"
	"strings"
)

const MaxQueryStatements = 20

var queryVariableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// SplitSQLStatements returns top-level SQL statements without breaking quoted
// strings, comments, quoted identifiers, or PostgreSQL dollar-quoted bodies.
// Database routine and trigger definitions are intentionally kept intact.
func SplitSQLStatements(query string) ([]string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if isRoutineDefinition(query) {
		return []string{strings.TrimSuffix(query, ";")}, nil
	}

	statements := make([]string, 0, 2)
	start := 0
	for index := 0; index < len(query); {
		current := query[index]
		next := byte(0)
		if index+1 < len(query) {
			next = query[index+1]
		}
		switch {
		case current == '-' && next == '-':
			index = skipLineComment(query, index+2)
		case current == '#':
			index = skipLineComment(query, index+1)
		case current == '/' && next == '*':
			index = skipBlockComment(query, index+2)
		case current == '\'' || current == '"' || current == '`':
			index = skipQuotedSQL(query, index, current)
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
		case current == ';':
			if statement := strings.TrimSpace(query[start:index]); statement != "" {
				statements = append(statements, statement)
				if len(statements) > MaxQueryStatements {
					return nil, fmt.Errorf(
						"query contains more than %d statements",
						MaxQueryStatements,
					)
				}
			}
			index++
			start = index
		default:
			index++
		}
	}
	if statement := strings.TrimSpace(query[start:]); statement != "" {
		statements = append(statements, statement)
	}
	if len(statements) > MaxQueryStatements {
		return nil, fmt.Errorf(
			"query contains more than %d statements",
			MaxQueryStatements,
		)
	}
	return statements, nil
}

func isRoutineDefinition(query string) bool {
	keywords := LeadingSQLKeywords(query, 5)
	if len(keywords) < 2 || keywords[0] != "CREATE" {
		return false
	}
	for _, keyword := range keywords[1:] {
		switch keyword {
		case "FUNCTION", "PROCEDURE", "TRIGGER":
			return true
		}
	}
	return false
}

func skipLineComment(query string, index int) int {
	for index < len(query) && query[index] != '\n' {
		index++
	}
	return index
}

func skipBlockComment(query string, index int) int {
	depth := 1
	for index < len(query) && depth > 0 {
		if index+1 < len(query) && query[index] == '/' && query[index+1] == '*' {
			depth++
			index += 2
			continue
		}
		if index+1 < len(query) && query[index] == '*' && query[index+1] == '/' {
			depth--
			index += 2
			continue
		}
		index++
	}
	return index
}

func skipBracketIdentifier(query string, index int) int {
	for index < len(query) {
		if query[index] != ']' {
			index++
			continue
		}
		if index+1 < len(query) && query[index+1] == ']' {
			index += 2
			continue
		}
		return index + 1
	}
	return len(query)
}

// BindQueryVariables replaces {{name}} tokens outside SQL strings/comments
// with driver placeholders and returns values in placeholder order.
func BindQueryVariables(
	query string,
	driver CapabilityDriver,
	variables []QueryVariable,
) (string, []interface{}, error) {
	values := make(map[string]QueryVariable, len(variables))
	for _, variable := range variables {
		name := strings.TrimSpace(variable.Name)
		if !queryVariableName.MatchString(name) {
			return "", nil, fmt.Errorf("invalid query variable name %q", variable.Name)
		}
		if _, duplicate := values[name]; duplicate {
			return "", nil, fmt.Errorf("query variable %q was provided more than once", name)
		}
		values[name] = variable
	}

	var output strings.Builder
	args := make([]interface{}, 0, len(variables))
	for index := 0; index < len(query); {
		current := query[index]
		next := byte(0)
		if index+1 < len(query) {
			next = query[index+1]
		}
		var end int
		switch {
		case current == '-' && next == '-':
			end = skipLineComment(query, index+2)
		case current == '#':
			end = skipLineComment(query, index+1)
		case current == '/' && next == '*':
			end = skipBlockComment(query, index+2)
		case current == '\'' || current == '"' || current == '`':
			end = skipQuotedSQL(query, index, current)
		case current == '[':
			end = skipBracketIdentifier(query, index+1)
		case current == '$':
			delimiter, ok := postgresDollarQuoteDelimiter(query[index:])
			if ok {
				bodyStart := index + len(delimiter)
				bodyEnd := strings.Index(query[bodyStart:], delimiter)
				if bodyEnd < 0 {
					end = len(query)
				} else {
					end = bodyStart + bodyEnd + len(delimiter)
				}
			}
		}
		if end > index {
			output.WriteString(query[index:end])
			index = end
			continue
		}

		if current != '{' || next != '{' {
			output.WriteByte(current)
			index++
			continue
		}
		closeOffset := strings.Index(query[index+2:], "}}")
		if closeOffset < 0 {
			return "", nil, fmt.Errorf("query variable starting at byte %d is not closed", index+1)
		}
		closeIndex := index + 2 + closeOffset
		name := strings.TrimSpace(query[index+2 : closeIndex])
		if !queryVariableName.MatchString(name) {
			return "", nil, fmt.Errorf("invalid query variable token %q", name)
		}
		variable, exists := values[name]
		if !exists {
			return "", nil, fmt.Errorf("query variable %q requires a value", name)
		}
		args = append(args, normalizeQueryVariableValue(variable))
		output.WriteString(driver.Placeholder(len(args)))
		index = closeIndex + 2
	}
	return output.String(), args, nil
}

func normalizeQueryVariableValue(variable QueryVariable) interface{} {
	switch strings.ToLower(strings.TrimSpace(variable.Type)) {
	case "null":
		return nil
	case "boolean", "bool":
		switch value := variable.Value.(type) {
		case bool:
			return value
		case string:
			return strings.EqualFold(strings.TrimSpace(value), "true")
		}
	}
	return variable.Value
}
