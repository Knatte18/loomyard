// keep.go reads the `//testtiming:keep <reason>` directives of a package's test files.

package testcover

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	keepDirective = "//testtiming:keep"

	// lymDirectivePrefix opens a `//lym:` directive line, which may stack between a keep and its func line.
	lymDirectivePrefix = "//lym:"
)

var topLevelTestDecl = regexp.MustCompile(`^func (Test\w*)\(`)

// ScanKeeps maps each test of the package directory marked with a `//testtiming:keep <reason>` line to its reason.
// The directive sits above the test's `func Test…` line, with only `//lym:` directive lines allowed between them.
// A directive with an empty reason, or one not placed above a top-level test declaration, is an error.
// The error names the file and line.
func ScanKeeps(dir string) (map[string]string, error) {
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
			for next < len(lines) && strings.HasPrefix(lines[next], lymDirectivePrefix) {
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

// keepReason returns the trimmed reason of a keep directive line, and reports whether the line is one.
func keepReason(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, keepDirective)
	if !ok || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
		return "", false
	}
	return strings.TrimSpace(rest), true
}
