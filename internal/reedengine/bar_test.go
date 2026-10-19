// bar_test.go pins the status bar's format strings against shapes a live tmux of this host cannot catch.

package reedengine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// sessionLoopTwoFormats returns the offset of the first top-level comma in the body of the #{S:} loop starting at loopStart, or -1 when the body holds one format.
// A comma inside a nested #{…} or #[…] group does not count.
func sessionLoopTwoFormats(format string, loopStart int) int {
	closers := []byte{'}'}
	for i := loopStart + len("#{S:"); i < len(format); i++ {
		switch {
		case strings.HasPrefix(format[i:], "#{"):
			closers = append(closers, '}')
			i++
		case strings.HasPrefix(format[i:], "#["):
			closers = append(closers, ']')
			i++
		case format[i] == closers[len(closers)-1]:
			closers = closers[:len(closers)-1]
			if len(closers) == 0 {
				return -1
			}
		case format[i] == ',' && len(closers) == 1:
			return i
		}
	}
	return -1
}

// TestStatusFormats_SessionLoopsTakeOneFormat fails on a #{S:} loop with a second, current-item format, which tmux 3.4 does not take.
// Rendering on a newer tmux cannot catch one, so this untagged check scans the bar's format strings.
// It also scans every Go string literal of the package outside bar.go, so a session loop declared elsewhere cannot escape it.
func TestStatusFormats_SessionLoopsTakeOneFormat(t *testing.T) {
	t.Parallel()
	loops := 0
	for name, format := range map[string]string{"statusFormatButtons": statusFormatButtons, "statusFormatSessions": statusFormatSessions} {
		for from := 0; ; {
			at := strings.Index(format[from:], "#{S:")
			if at < 0 {
				break
			}
			at += from
			loops++
			if comma := sessionLoopTwoFormats(format, at); comma >= 0 {
				t.Errorf("%s: the #{S:} loop at %d has a second format after the comma at %d", name, at, comma)
			}
			from = at + len("#{S:")
		}
	}
	if loops == 0 {
		t.Error("found no #{S:} loop in the bar's formats, want the scan to see statusFormatSessions' loops")
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "bar.go" {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.Contains(lit.Value, "#{S:") {
				t.Errorf("%s declares a #{S:} loop in a string literal; session loops belong in bar.go", fset.Position(lit.Pos()))
			}
			return true
		})
	}
}
