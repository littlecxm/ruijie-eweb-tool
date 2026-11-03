package main

import (
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

func normalizeFilePath(input string) string {
	// Prefer literal filenames so real quotes and backslashes are not lost.
	if _, err := os.Stat(input); err == nil {
		return input
	}
	leftTrimmed := strings.TrimLeftFunc(input, unicode.IsSpace)
	path := strings.TrimRightFunc(leftTrimmed, unicode.IsSpace)
	// An escaped trailing space belongs to the filename, not the paste padding.
	backslashes := 0
	for i := len(path) - 1; i >= 0 && path[i] == '\\'; i-- {
		backslashes++
	}
	if len(path) < len(leftTrimmed) && backslashes%2 == 1 {
		_, size := utf8.DecodeRuneInString(leftTrimmed[len(path):])
		path = leftTrimmed[:len(path)+size]
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	unquoted := path
	if len(path) >= 2 && ((path[0] == '"' && path[len(path)-1] == '"') || (path[0] == '\'' && path[len(path)-1] == '\'')) {
		unquoted = path[1 : len(path)-1]
		if _, err := os.Stat(unquoted); err == nil {
			return unquoted
		}
	}
	if decoded, ok := unquoteFilePath(path); ok {
		return decoded
	}
	return unquoted
}

// unquoteFilePath handles quoting and escaped metacharacters from terminal file
// drops. Spaces are kept, and no shell expansion or command execution occurs.
func unquoteFilePath(path string) (string, bool) {
	var result strings.Builder
	var quote byte
	for i := 0; i < len(path); i++ {
		c := path[i]
		if c == '\\' && quote != '\'' && i+1 < len(path) {
			next := path[i+1]
			// Preserve ordinary backslashes, including Windows path separators.
			escapable := " \t\r\n\\'\"`$&;|<>()[]{}*?!#~^=%"
			if quote == '"' {
				escapable = "$`\"\\\n"
			}
			if strings.ContainsRune(escapable, rune(next)) {
				if next != '\n' {
					result.WriteByte(next)
				}
				i++
				continue
			}
		}
		if quote == 0 && (c == '\'' || c == '"') {
			quote = c
			continue
		}
		if c == quote {
			quote = 0
			continue
		}
		result.WriteByte(c)
	}
	return result.String(), quote == 0
}
