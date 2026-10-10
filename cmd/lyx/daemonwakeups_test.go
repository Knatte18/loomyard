// daemonwakeups_test.go enforces `PATTERN-daemon-wakeups`: the files that hold an unbounded wait carry no sub-second duration literal and no sub-second `MS` or `Ms` constant unless the line announces itself with `//lyx:one-shot <reason>`, and no production file outside that list starts a `time.NewTicker` or `time.Tick`.
// The scan sees literal shapes only; a computed period, or a sleep loop in a file the list does not name, is left to review.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// unboundedWaitFiles are the module-relative, slash-separated files that hold an unbounded wait or the loop that drives one.
var unboundedWaitFiles = []string{
	"internal/reedengine/watchloop.go",
	"internal/reedengine/namerepair.go",
	"internal/reedcli/watchdog.go",
	"internal/orchengine/watchloop.go",
	"internal/battenshed/innerrun.go",
	"internal/battenshed/primelockwait.go",
	"internal/shuttleengine/wait.go",
	"internal/websterengine/poll.go",
	"internal/shedverbs/status.go",
	"internal/shedverbs/loopwait.go",
	"internal/shedverbs/loop.go",
	"internal/shedverbs/loopwatchdog.go",
}

// oneShotMarker opens the comment that lets a sub-second literal through on its own line, followed by the reason.
const oneShotMarker = "//lyx:one-shot"

// subSecondWakeFindings parses data as Go and returns one message per sub-second duration literal or sub-second `MS`/`Ms` integer constant that its own line does not excuse with a reasoned oneShotMarker.
func subSecondWakeFindings(filename string, data []byte) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, data, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	reasons := map[int]string{}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if _, reason, ok := strings.Cut(comment.Text, oneShotMarker); ok {
				reasons[fset.Position(comment.Slash).Line] = strings.TrimSpace(reason)
			}
		}
	}

	var findings []string
	report := func(pos token.Pos, what string) {
		position := fset.Position(pos)
		reason, annotated := reasons[position.Line]
		switch {
		case annotated && reason != "":
		case annotated:
			findings = append(findings, fmt.Sprintf("%s: %s carries %s with no reason; a reason is required", position, what, oneShotMarker))
		default:
			findings = append(findings, fmt.Sprintf("%s: sub-second %s; raise it to a second or more, or annotate the line with `%s <reason>`", position, what, oneShotMarker))
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BinaryExpr:
			if node.Op != token.MUL {
				return true
			}
			literal, scale, ok := splitLiteralAndScale(node.X, node.Y)
			selector, isSelector := scale.(*ast.SelectorExpr)
			if !ok || !isSelector {
				return true
			}
			pkg, isIdent := selector.X.(*ast.Ident)
			nanos := scaleInNanoseconds(selector.Sel.Name)
			count, parseErr := strconv.ParseFloat(literal.Value, 64)
			if isIdent && pkg.Name == "time" && nanos > 0 && parseErr == nil && count*nanos < 1e9 {
				report(node.Pos(), fmt.Sprintf("duration %s * time.%s", literal.Value, selector.Sel.Name))
			}
		case *ast.GenDecl:
			if node.Tok != token.CONST {
				return true
			}
			for _, spec := range node.Specs {
				valueSpec := spec.(*ast.ValueSpec)
				for i, name := range valueSpec.Names {
					if !strings.HasSuffix(name.Name, "MS") && !strings.HasSuffix(name.Name, "Ms") {
						continue
					}
					if i >= len(valueSpec.Values) {
						continue
					}
					literal, isLiteral := valueSpec.Values[i].(*ast.BasicLit)
					if !isLiteral || literal.Kind != token.INT {
						continue
					}
					if value, parseErr := strconv.Atoi(literal.Value); parseErr == nil && value < 1000 {
						report(name.Pos(), fmt.Sprintf("millisecond constant %s = %s", name.Name, literal.Value))
					}
				}
			}
		}
		return true
	})
	return findings, nil
}

// tickerCallPositions returns the position of every time.NewTicker and time.Tick call in file.
func tickerCallPositions(fset *token.FileSet, file *ast.File) []string {
	var positions []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, isIdent := selector.X.(*ast.Ident); isIdent && pkg.Name == "time" && (selector.Sel.Name == "NewTicker" || selector.Sel.Name == "Tick") {
			positions = append(positions, fset.Position(call.Pos()).String())
		}
		return true
	})
	return positions
}

// TestDaemonWakeups_UnboundedWaitsStayAboveTheFloor enforces `PATTERN-daemon-wakeups`.
//
//lyx:guard
func TestDaemonWakeups_UnboundedWaitsStayAboveTheFloor(t *testing.T) {
	t.Run("detector", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			src  string
			// want is a substring of the one expected finding; empty means none.
			want string
		}{
			{"millisecond literal below a second", "const d = 200 * time.Millisecond\n", "sub-second duration 200 * time.Millisecond"},
			{"millisecond literal reversed", "const d = time.Millisecond * 200\n", "sub-second duration"},
			{"microsecond multiple", "const d = 5 * time.Microsecond\n", "sub-second duration 5 * time.Microsecond"},
			{"nanosecond multiple", "const d = 5 * time.Nanosecond\n", "sub-second duration 5 * time.Nanosecond"},
			{"a thousand milliseconds", "const d = 1000 * time.Millisecond\n", ""},
			{"seconds", "const d = 2 * time.Second\n", ""},
			{"computed milliseconds", "func f(n int) time.Duration { return time.Duration(n) * time.Millisecond }\n", ""},
			{"MS constant below a thousand", "const pollMS = 250\n", "sub-second millisecond constant pollMS = 250"},
			{"Ms constant below a thousand", "const pollMs = 999\n", "sub-second millisecond constant pollMs = 999"},
			{"MS constant at a thousand", "const pollMS = 1000\n", ""},
			{"name not ending in MS", "const forms = 5\n", ""},
			{"annotated with a reason", "const d = 200 * time.Millisecond //lyx:one-shot bounded retry\n", ""},
			{"bare annotation", "const d = 200 * time.Millisecond //lyx:one-shot\n", "a reason is required"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				findings, err := subSecondWakeFindings(tt.name+".go", []byte("package p\n\nimport \"time\"\n\n"+tt.src))
				if err != nil {
					t.Fatal(err)
				}
				if tt.want == "" {
					if len(findings) != 0 {
						t.Errorf("findings = %q; want none", findings)
					}
					return
				}
				if len(findings) != 1 || !strings.Contains(findings[0], tt.want) {
					t.Errorf("findings = %q; want one containing %q", findings, tt.want)
				}
			})
		}
	})

	t.Run("declared files hold no unannounced sub-second wake", func(t *testing.T) {
		t.Parallel()
		root := scankit.Root(t)
		for _, rel := range unboundedWaitFiles {
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				t.Errorf("declared unbounded-wait file %s is unreadable: %v; fix the list in cmd/lyx/daemonwakeups_test.go", rel, err)
				continue
			}
			findings, err := subSecondWakeFindings(rel, data)
			if err != nil {
				t.Errorf("%s: %v", rel, err)
			}
			for _, finding := range findings {
				t.Errorf("`PATTERN-daemon-wakeups` violated: %s", finding)
			}
		}
	})

	t.Run("tickers stay in declared files", func(t *testing.T) {
		t.Parallel()
		var failures []string
		scanned := scankit.Walk(t, scankit.Options{Roots: []string{"internal", "cmd"}}, func(f *scankit.File) {
			if slices.Contains(unboundedWaitFiles, f.Rel) {
				return
			}
			file := f.AST(t, 0)
			for _, position := range tickerCallPositions(f.FileSet(), file) {
				failures = append(failures, fmt.Sprintf("%s: time.NewTicker or time.Tick outside the declared unbounded-wait files; list the loop in unboundedWaitFiles in cmd/lyx/daemonwakeups_test.go, or explain it there, in the same commit", position))
			}
		})
		scankit.RequireFloor(t, scanned, 200, "daemon wakeups ticker scan")
		if len(failures) > 0 {
			t.Errorf("`PATTERN-daemon-wakeups` violated:\n%s", strings.Join(failures, "\n"))
		}
	})
}
