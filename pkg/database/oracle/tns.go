package oracle

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const maxTNSConfigBytes = 4 * 1024 * 1024

type tnsAliasDefinition struct {
	name       string
	descriptor string
}

func stripTNSComments(value string) string {
	var result strings.Builder
	result.Grow(len(value))
	inSingle := false
	inDouble := false
	for index := 0; index < len(value); index++ {
		character := value[index]
		switch character {
		case '\'':
			if !inDouble {
				if inSingle && index+1 < len(value) && value[index+1] == '\'' {
					result.WriteByte(character)
					index++
					result.WriteByte(value[index])
					continue
				}
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				if inDouble && index+1 < len(value) && value[index+1] == '"' {
					result.WriteByte(character)
					index++
					result.WriteByte(value[index])
					continue
				}
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				for index < len(value) && value[index] != '\n' {
					index++
				}
				if index < len(value) {
					result.WriteByte('\n')
				}
				continue
			}
		}
		result.WriteByte(character)
	}
	return result.String()
}

func skipTNSSpace(value string, index int) int {
	for index < len(value) && unicode.IsSpace(rune(value[index])) {
		index++
	}
	return index
}

func findTNSEquals(value string, start int) (int, error) {
	inSingle := false
	inDouble := false
	for index := start; index < len(value); index++ {
		switch value[index] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '=':
			if !inSingle && !inDouble {
				return index, nil
			}
		case '(':
			if !inSingle && !inDouble {
				return 0, fmt.Errorf(
					"invalid tnsnames.ora entry near byte %d: alias is missing '='",
					start,
				)
			}
		}
		if index-start > 4096 {
			return 0, fmt.Errorf("Oracle TNS alias declaration is too long")
		}
	}
	return 0, fmt.Errorf("Oracle TNS alias is missing a descriptor")
}

func scanTNSDescriptor(value string, start int) (int, error) {
	if start >= len(value) || value[start] != '(' {
		return 0, fmt.Errorf(
			"Oracle TNS aliases must use a parenthesized connect descriptor",
		)
	}
	depth := 0
	inSingle := false
	inDouble := false
	for index := start; index < len(value); index++ {
		switch value[index] {
		case '\'':
			if !inDouble {
				if inSingle && index+1 < len(value) && value[index+1] == '\'' {
					index++
					continue
				}
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				if inDouble && index+1 < len(value) && value[index+1] == '"' {
					index++
					continue
				}
				inDouble = !inDouble
			}
		case '(':
			if !inSingle && !inDouble {
				depth++
			}
		case ')':
			if !inSingle && !inDouble {
				depth--
				if depth == 0 {
					return index + 1, nil
				}
				if depth < 0 {
					return 0, fmt.Errorf(
						"Oracle TNS descriptor contains an unexpected ')'",
					)
				}
			}
		}
	}
	return 0, fmt.Errorf("Oracle TNS descriptor has unbalanced parentheses")
}

func validateTNSAlias(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 {
		return "", fmt.Errorf("Oracle TNS alias is empty or exceeds 256 bytes")
	}
	if strings.IndexFunc(value, func(character rune) bool {
		return character < 0x20 ||
			character == 0x7f ||
			strings.ContainsRune("=()", character)
	}) >= 0 {
		return "", fmt.Errorf("Oracle TNS alias %q contains invalid characters", value)
	}
	return value, nil
}

func parseTNSAliases(value string) (map[string]tnsAliasDefinition, error) {
	value = strings.TrimPrefix(value, "\ufeff")
	value = stripTNSComments(value)
	result := make(map[string]tnsAliasDefinition)
	for index := 0; ; {
		index = skipTNSSpace(value, index)
		if index >= len(value) {
			break
		}
		equals, err := findTNSEquals(value, index)
		if err != nil {
			return nil, err
		}
		left := strings.TrimSpace(value[index:equals])
		right := skipTNSSpace(value, equals+1)
		if strings.EqualFold(left, "IFILE") {
			for right < len(value) && value[right] != '\n' {
				right++
			}
			index = right
			continue
		}
		end, err := scanTNSDescriptor(value, right)
		if err != nil {
			return nil, fmt.Errorf("parse Oracle TNS alias %q: %w", left, err)
		}
		descriptor := strings.TrimSpace(value[right:end])
		for _, rawAlias := range strings.Split(left, ",") {
			alias, err := validateTNSAlias(rawAlias)
			if err != nil {
				return nil, err
			}
			key := strings.ToUpper(alias)
			if previous, exists := result[key]; exists &&
				previous.descriptor != descriptor {
				return nil, fmt.Errorf(
					"Oracle TNS alias %q is defined more than once",
					alias,
				)
			}
			result[key] = tnsAliasDefinition{
				name:       alias,
				descriptor: descriptor,
			}
		}
		index = end
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("tnsnames.ora contains no connect aliases")
	}
	return result, nil
}

func readTNSConfig(path string) (map[string]tnsAliasDefinition, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("tnsnames.ora path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve tnsnames.ora path: %w", err)
	}
	file, err := os.Open(filepath.Clean(absolute))
	if err != nil {
		return nil, fmt.Errorf("open tnsnames.ora: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect tnsnames.ora: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("tnsnames.ora must be a regular file")
	}
	if info.Size() > maxTNSConfigBytes {
		return nil, fmt.Errorf(
			"tnsnames.ora exceeds the %d-byte safety limit",
			maxTNSConfigBytes,
		)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxTNSConfigBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read tnsnames.ora: %w", err)
	}
	if len(data) > maxTNSConfigBytes {
		return nil, fmt.Errorf(
			"tnsnames.ora exceeds the %d-byte safety limit",
			maxTNSConfigBytes,
		)
	}
	aliases, err := parseTNSAliases(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse tnsnames.ora: %w", err)
	}
	return aliases, nil
}

// ReadTNSAliases returns the aliases that can be selected by the connection UI.
// Include directives are deliberately not followed: the user must explicitly
// choose the file that contains the reviewed descriptor.
func ReadTNSAliases(path string) ([]string, error) {
	aliases, err := readTNSConfig(path)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		result = append(result, alias.name)
	}
	sort.Slice(result, func(left, right int) bool {
		return strings.ToLower(result[left]) < strings.ToLower(result[right])
	})
	return result, nil
}

func resolveTNSAlias(path string, name string) (string, error) {
	aliases, err := readTNSConfig(path)
	if err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	alias, exists := aliases[strings.ToUpper(name)]
	if !exists {
		return "", fmt.Errorf(
			"Oracle TNS alias %q was not found in %s",
			name,
			filepath.Base(path),
		)
	}
	return alias.descriptor, nil
}
