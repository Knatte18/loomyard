// websterretiredsteps_test.go pins that no webster way forward names `git reset --keep`, or a `lyx webster reset` followed by `lyx webster run --fresh`.
// `reset --to start` archives the run record and performs the keep form itself, so a plain `lyx webster run` follows every reset.
// The scan reads files only, so it stays in the untagged tier, and reuses the token matcher of norecoverydeny_test.go.
// The scanned surface is `contracts/specs/`, `contracts/stencils/webster/` and the string literals of the non-test Go files under the webster modules.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

var (
	// retiredKeepForm is the git form a way forward never names, since a reset target performs it.
	retiredKeepForm = []string{"git", "reset", "--keep"}
	// retiredResetStep and retiredRunFreshStep are the pair a way forward never offers in that order on one line.
	retiredResetStep    = []string{"lyx", "webster", "reset"}
	retiredRunFreshStep = []string{"lyx", "webster", "run", "--fresh"}
)

// retiredStepsTextSurfaces are the module-root-relative directories whose files are scanned as plain text.
var retiredStepsTextSurfaces = []string{"contracts/specs", "contracts/stencils/webster"}

// retiredStepsGoModulePrefixes are the directory-name prefixes under internal/ whose non-test Go string literals are scanned.
var retiredStepsGoModulePrefixes = []string{"webster"}

// scanRetiredSteps returns one hit per line of text that names `git reset --keep`, or that names `lyx webster reset` before `lyx webster run --fresh`.
// Line numbers are 1-based and offset by firstLine-1.
func scanRetiredSteps(file, text string, firstLine int) []deniedRecoveryHit {
	var hits []deniedRecoveryHit
	for i, line := range strings.Split(text, "\n") {
		spans := deniedRecoveryTokenPattern.FindAllStringIndex(line, -1)
		resetAt := -1
		for j := range spans {
			switch {
			case tokensMatch(line, spans[j:], retiredKeepForm):
				hits = append(hits, deniedRecoveryHit{file: file, line: firstLine + i, form: strings.Join(retiredKeepForm, " ")})
			case resetAt < 0 && tokensMatch(line, spans[j:], retiredResetStep):
				resetAt = j
			case resetAt >= 0 && tokensMatch(line, spans[j:], retiredRunFreshStep):
				hits = append(hits, deniedRecoveryHit{file: file, line: firstLine + i, form: strings.Join(retiredResetStep, " ") + " ... " + strings.Join(retiredRunFreshStep, " ")})
			}
		}
	}
	return hits
}

// scanRetiredStepsGoFile scans the string literals of one Go source file.
func scanRetiredStepsGoFile(t *testing.T, root, path string) []deniedRecoveryHit {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatalf("rel %s: %v", path, err)
	}
	var hits []deniedRecoveryHit
	ast.Inspect(parsed, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		hits = append(hits, scanRetiredSteps(filepath.ToSlash(rel), text, fset.Position(lit.Pos()).Line)...)
		return true
	})
	return hits
}

// TestWebsterRetiredSteps_ScannedSurfaceHasNone fails, naming file and line, for every retired step the scanned surface names.
//
//testtiming:keep pins that no webster refusal, stencil or spec names git reset --keep or a reset followed by run --fresh, the retirement of both routes
//lyx:guard
func TestWebsterRetiredSteps_ScannedSurfaceHasNone(t *testing.T) {
	root := scankit.Root(t)
	var hits []deniedRecoveryHit
	scanned := 0

	for _, dir := range retiredStepsTextSurfaces {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(dir)), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			scanned++
			hits = append(hits, scanRetiredSteps(filepath.ToSlash(rel), string(data), 1)...)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}

	internalDir := filepath.Join(root, "internal")
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		t.Fatalf("read %s: %v", internalDir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !hasAnyPrefix(entry.Name(), retiredStepsGoModulePrefixes) {
			continue
		}
		err := filepath.WalkDir(filepath.Join(internalDir, entry.Name()), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			scanned++
			hits = append(hits, scanRetiredStepsGoFile(t, root, path)...)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", entry.Name(), err)
		}
	}

	scankit.RequireFloor(t, scanned, 20, "webster retired-steps scan")

	if len(hits) > 0 {
		lines := make([]string, len(hits))
		for i, h := range hits {
			lines[i] = fmt.Sprintf("%s:%d: names retired step %q; name a `lyx webster reset` target and then a plain `lyx webster run`", h.file, h.line, h.form)
		}
		t.Errorf("retired webster steps named:\n%s", strings.Join(lines, "\n"))
	}
}

// TestWebsterRetiredSteps_ScanSelfCheck plants each shape in an in-memory file and asserts the scan's verdict on it.
//
//testtiming:keep proves the TestWebsterRetiredSteps_ScannedSurfaceHasNone scan fires on each planted retired shape and stays quiet on a lone run --fresh
func TestWebsterRetiredSteps_ScanSelfCheck(t *testing.T) {
	cases := []struct {
		name string
		text string
		want int
	}{
		{"reset keep", "way forward: run `git reset --keep abc123`", 1},
		{"reset then run fresh", "way forward: 1) lyx webster reset --to start; 2) lyx webster run --fresh", 1},
		{"reset then plain run", "way forward: 1) lyx webster reset --to start; 2) lyx webster run", 0},
		{"lone run fresh", "way forward: rm outcome.yaml, then `lyx webster run --fresh`", 0},
		{"run fresh then reset", "`lyx webster run --fresh`, or `lyx webster reset --to start`", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := scanRetiredSteps("planted.md", tc.text, 1)
			if len(hits) != tc.want {
				t.Fatalf("scan(%q) = %d hit(s) %+v; want %d", tc.text, len(hits), hits, tc.want)
			}
		})
	}
}
