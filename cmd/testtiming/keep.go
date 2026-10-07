package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	keepDirective = "//testtiming:keep"

	// lyxDirectivePrefix opens a `//lyx:` directive line, which may stack between a keep and its func line.
	lyxDirectivePrefix = "//lyx:"
)

var topLevelTestDecl = regexp.MustCompile(`^func (Test\w*)\(`)

// scanKeeps returns the tests of the package directory marked with a `//testtiming:keep <reason>` line, by name with their reasons.
// The directive sits on the line directly above the test's `func Test…` line, or above only `//lyx:` directive lines that precede it.
// A directive with an empty reason, or one not directly above a top-level test declaration, is an error naming the file and line.
func scanKeeps(dir string) (map[string]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		return nil, err
	}
	keeps := map[string]string{}
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			reason, ok := keepReason(line)
			if !ok {
				continue
			}
			where := fmt.Sprintf("%s:%d", file, i+1)
			next := i + 1
			for next < len(lines) && strings.HasPrefix(lines[next], lyxDirectivePrefix) {
				next++
			}
			var test []string
			if next < len(lines) {
				test = topLevelTestDecl.FindStringSubmatch(lines[next])
			}
			if test == nil {
				return nil, fmt.Errorf("%s: %s is not directly above a top-level test function", where, keepDirective)
			}
			if reason == "" {
				return nil, fmt.Errorf("%s: %s on %s has no reason", where, keepDirective, test[1])
			}
			keeps[test[1]] = reason
		}
	}
	return keeps, nil
}

// keepReason returns the trimmed reason of a keep directive line and whether the line is one.
func keepReason(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, keepDirective)
	if !ok || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
		return "", false
	}
	return strings.TrimSpace(rest), true
}
