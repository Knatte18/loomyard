//go:build integration

// refs_integration_test.go exercises References against a real, held-open
// gopls subprocess — the one test in this package that actually launches a
// language server. It is //go:build integration-tagged and therefore
// excluded from the plain `go test` verify (the Test Tier Purity
// Invariant); it is run separately with `-tags integration` on a machine
// with gopls installed. Only the gopls-spawning subtest is guarded on
// exec.LookPath("gopls") (via t.Skip); the ErrServerNotFound subtest never
// launches gopls and always runs, even on a machine without it. This test
// only spawns gopls, never git, so no TestMain/lyxtest.HermeticGitEnv is
// required per the Hermetic Git Test Environment Invariant.

package codeintelengine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// funcDeclPattern matches a top-level function declaration line
// ("func Name(" or "func (recv Type) Name(") and captures the byte offset
// of the function name within the line via the submatch index, so a
// symbol's position can be located directly from source text without
// loading a package graph — matching the engine's decoupling from
// go/token (position.go's batch-local decision) and #008's approach of
// resolving a known high-fan-in hubgeometry symbol.
var funcDeclPattern = regexp.MustCompile(`^func (?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\(`)

// findFuncPosition scans file for a top-level declaration of funcName and
// returns its Position (1-based line, 1-based byte column of the function
// name itself, ready to hand to References via Query.Pos).
func findFuncPosition(t *testing.T, file, funcName string) Position {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("findFuncPosition: read %s: %v", file, err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		m := funcDeclPattern.FindStringSubmatchIndex(line)
		if m == nil {
			continue
		}
		name := line[m[2]:m[3]]
		if name != funcName {
			continue
		}
		return Position{File: file, Line: i + 1, Character: m[2] + 1}
	}
	t.Fatalf("findFuncPosition: no top-level declaration of %q found in %s", funcName, file)
	return Position{}
}

// repoRoot returns this worktree's module root: internal/codeintelengine is
// always two directories below it.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("repoRoot: could not determine codeintelengine source directory location")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

func TestReferences_Integration(t *testing.T) {
	t.Run("live gopls references for a known high-fan-in symbol", func(t *testing.T) {
		if _, err := exec.LookPath("gopls"); err != nil {
			t.Skip(builtins()["go"].InstallHint)
		}

		root := repoRoot(t)
		hubgeometryFile := filepath.Join(root, "internal", "hubgeometry", "hubgeometry.go")
		pos := findFuncPosition(t, hubgeometryFile, "Resolve")

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		refs, err := References(ctx, Options{
			Registry:  builtins(),
			TargetDir: root,
			Lang:      "go",
			Query:     Query{Pos: &pos},
			Timeout:   30 * time.Second,
		})
		if err != nil {
			t.Fatalf("References(hubgeometry.Resolve) returned unexpected error: %v", err)
		}
		if len(refs) == 0 {
			t.Fatal("References(hubgeometry.Resolve) returned zero references; want the declaration site plus its call sites")
		}

		foundDeclSite := false
		for _, ref := range refs {
			if filepath.Clean(ref.File) == filepath.Clean(hubgeometryFile) && ref.Line == pos.Line {
				foundDeclSite = true
				break
			}
		}
		if !foundDeclSite {
			t.Errorf("References(hubgeometry.Resolve) = %+v; want it to include the declaration site %s:%d", refs, hubgeometryFile, pos.Line)
		}
	})

	t.Run("non-existent server binary yields ErrServerNotFound", func(t *testing.T) {
		root := repoRoot(t)
		reg := Registry{
			"go": {
				Markers:     []string{"go.mod"},
				Match:       "any",
				Command:     []string{"lyx-codeintel-nonexistent-binary-xyz"},
				InstallHint: "this binary is intentionally fake for the test",
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := References(ctx, Options{
			Registry:  reg,
			TargetDir: root,
			Lang:      "go",
			Query:     Query{Symbol: "Resolve"},
			Timeout:   5 * time.Second,
		})
		if !errors.Is(err, ErrServerNotFoundSentinel) {
			t.Errorf("References() with a non-existent server binary err = %v; want errors.Is(err, ErrServerNotFoundSentinel)", err)
		}
	})
}
