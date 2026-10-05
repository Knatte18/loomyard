// norecoverydeny_test.go enforces `PATTERN-no-denied-recovery`: no refusal, stencil, spec or skill names a command the agents' settings deny as a way forward.
// The scan reads files only, so it stays in the untagged tier.
// The scanned surface is `contracts/specs/`, `contracts/stencils/`, `plugins/ly/skills/` and the string literals of the non-test Go files under the webster, shed and loom modules.
// `pattern/` background files and test files are outside it.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// deniedRecoveryForms is the closed list of commands the agents' settings deny today, each as its token sequence.
// `agent-permission-setup` owns the real permission set and updates this list when the set changes.
var deniedRecoveryForms = [][]string{
	{"git", "reset", "--hard"},
	{"git", "push", "--force"},
	{"git", "push", "-f"},
	{"rm", "-rf"},
}

// deniedRecoveryTokenPattern splits a line into command tokens.
// Whitespace, backticks, quotes, brackets, commas and semicolons separate tokens, so a form matches only as a whole token sequence and `git push --force-with-lease` is not `git push --force`.
var deniedRecoveryTokenPattern = regexp.MustCompile("[^\\s`\"'(),;]+")

// deniedRecoveryMarker is the one pinned prohibition marker: a denied form directly after it is forbidden, not offered.
const deniedRecoveryMarker = "never "

// deniedRecoveryTextSurfaces are the module-root-relative directories whose files are scanned as plain text.
var deniedRecoveryTextSurfaces = []string{"contracts/specs", "contracts/stencils", "plugins/ly/skills"}

// deniedRecoveryGoModulePrefixes are the directory-name prefixes under internal/ whose non-test Go string literals are scanned: the modules with a refusal-spec section.
var deniedRecoveryGoModulePrefixes = []string{"webster", "shed", "loom"}

// deniedRecoveryHit is one denied form found at a line of a scanned file.
type deniedRecoveryHit struct {
	file string
	line int
	form string
}

// scanDeniedRecovery returns the denied forms in text that no "never " marker forbids, with 1-based line numbers offset by firstLine-1.
// A form is exempt only when "never " precedes it directly, with at most one backtick between;
// a denied form later in the same sentence, or preceded by anything else, still counts.
func scanDeniedRecovery(file, text string, firstLine int) []deniedRecoveryHit {
	var hits []deniedRecoveryHit
	for i, line := range strings.Split(text, "\n") {
		spans := deniedRecoveryTokenPattern.FindAllStringIndex(line, -1)
		for j := range spans {
			for _, form := range deniedRecoveryForms {
				if !tokensMatch(line, spans[j:], form) {
					continue
				}
				if forbiddenByMarker(line[:spans[j][0]]) {
					continue
				}
				hits = append(hits, deniedRecoveryHit{file: file, line: firstLine + i, form: strings.Join(form, " ")})
			}
		}
	}
	return hits
}

// tokensMatch reports whether the tokens at spans, read from line, are exactly form.
func tokensMatch(line string, spans [][]int, form []string) bool {
	if len(spans) < len(form) {
		return false
	}
	for k, want := range form {
		if line[spans[k][0]:spans[k][1]] != want {
			return false
		}
	}
	return true
}

// forbiddenByMarker reports whether before, the line text ahead of a denied form, ends in the prohibition marker.
func forbiddenByMarker(before string) bool {
	before = strings.TrimSuffix(before, "`")
	return strings.HasSuffix(strings.ToLower(before), deniedRecoveryMarker)
}

// scanDeniedRecoveryGoFile scans the string literals of one Go source file.
func scanDeniedRecoveryGoFile(t *testing.T, root, path string) []deniedRecoveryHit {
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
	rel = filepath.ToSlash(rel)
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
		hits = append(hits, scanDeniedRecovery(rel, text, fset.Position(lit.Pos()).Line)...)
		return true
	})
	return hits
}

// TestNoDeniedRecovery_ScannedSurfaceHasNone fails, naming file and line, for every denied form the scanned surface offers.
func TestNoDeniedRecovery_ScannedSurfaceHasNone(t *testing.T) {
	root := scankit.Root(t)
	var hits []deniedRecoveryHit
	scanned := 0

	for _, dir := range deniedRecoveryTextSurfaces {
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
			hits = append(hits, scanDeniedRecovery(filepath.ToSlash(rel), string(data), 1)...)
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
		if !entry.IsDir() || !hasAnyPrefix(entry.Name(), deniedRecoveryGoModulePrefixes) {
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
			hits = append(hits, scanDeniedRecoveryGoFile(t, root, path)...)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", entry.Name(), err)
		}
	}

	scankit.RequireFloor(t, scanned, 20, "no-denied-recovery scan")

	if len(hits) > 0 {
		lines := make([]string, len(hits))
		for i, h := range hits {
			lines[i] = fmt.Sprintf("%s:%d: offers denied command %q; lyx performs that step itself, name its verb or a non-denied command", h.file, h.line, h.form)
		}
		t.Errorf("`PATTERN-no-denied-recovery` violated:\n%s", strings.Join(lines, "\n"))
	}
}

// TestNoDeniedRecovery_ScanSelfCheck plants each shape in an in-memory file and asserts the scan's verdict on it.
func TestNoDeniedRecovery_ScanSelfCheck(t *testing.T) {
	cases := []struct {
		name string
		text string
		want int
	}{
		{"offered as a way forward", "way forward: run `git reset --hard` in the worktree", 1},
		{"forbidden by never", "an agent must never `git reset --hard`", 0},
		{"forbidden by Never at a sentence start", "Never git push --force.", 0},
		{"never then offered again in the same sentence", "never `git reset --hard`, or run `git reset --hard`", 1},
		{"reset keep is not denied", "way forward: run `git reset --keep abc123`", 0},
		{"checkout is not denied", "way forward: git checkout abc123 -- path/to/file", 0},
		{"force with lease is not force", "way forward: git push --force-with-lease", 0},
		{"short force flag", "way forward: git push -f origin main", 1},
		{"rm -rf", "way forward: rm -rf .lyx/webster", 1},
		{"plain rm", "way forward: rm outcome.yaml", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := scanDeniedRecovery("planted.md", tc.text, 1)
			if len(hits) != tc.want {
				t.Fatalf("scan(%q) = %d hit(s) %+v; want %d", tc.text, len(hits), hits, tc.want)
			}
			for _, h := range hits {
				if h.file != "planted.md" || h.line != 1 {
					t.Errorf("hit = %+v; want file planted.md line 1", h)
				}
			}
		})
	}
}

// hasAnyPrefix reports whether name starts with one of prefixes.
func hasAnyPrefix(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
