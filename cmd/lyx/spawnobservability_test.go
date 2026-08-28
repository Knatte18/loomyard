// spawnobservability_test.go enforces CONSTRAINTS.md's Live-Substrate Spawn Observability invariant's
// mechanical half: a production (non-_test.go) .go file under internal/ or cmd/ that contains a real
// exec.Command/exec.CommandContext call must either import internal/logger, or carry a written-reason
// entry in spawnObservabilityAllowedSpawners.
//
// # AST, not substring, and why that diverges from the siblings
//
// Every other guard in this package is a raw-substring scan. This one must not be: three production
// files hit the exec.Command substring with zero real calls — internal/githubclient/doc.go,
// internal/reedengine/doc.go, and internal/reedengine/attach.go — all prose in doc comments. A
// substring guard would demand permanent allowlist entries for files that spawn nothing at all, which
// reads as "these spawn without logging, and we accepted it" — the opposite of the truth. This guard
// parses each candidate file with go/parser and only counts a genuine *ast.CallExpr, following the
// in-repo precedent at internal/githubclient/leaf_enforcement_test.go, which already walks the same
// package with go/parser rather than a substring scan. cmd/lyx/tierpurity_test.go, by contrast,
// deliberately does NOT strip comments, because there a mention genuinely is the scan data it wants —
// its raw-substring approach is correct for its own purpose and is not a precedent to "harmonise" this
// guard toward. Collapsing this guard into a substring scan would silently reintroduce the three
// phantom violations above.
//
// # The known blind spot
//
// Detection here is file-level import presence, which is coarse: a file that imports internal/logger
// for an unrelated line and spawns a process unlogged still passes this guard. It catches the
// regression shape that actually occurs — a brand-new spawn landing in a file with no logging at all —
// and does not claim more than that, in the same spirit as cmd/lyx/checkedcall_test.go's own
// blind-spot section. A per-call-site check (does the specific exec.Command call have a logger.Info/
// Debug immediately around it, not just anywhere in the file) would close this gap but is not what
// this guard does.
//
// # Why the hard-error half of the audit gets no guard
//
// manifest/designs/logger-coverage.md also enumerates every outcome-switch hard-error-return site
// reachable from a lyx command, but that table is document-only, with no guard here or anywhere else.
// A new unlogged exec.Command is nearly always a real miss, so a file-level check on it has high
// signal. A new outcome-switch branch, by contrast, may legitimately return normally for the caller to
// branch on — internal/burlerengine/engine.go's Run does exactly that — so an equivalent file-level
// check on outcome switches would fire on correct code and train the next author to reach for an
// allowlist instead of writing correct code. See manifest/designs/logger-coverage.md's "Enforcement
// asymmetry" section for the full argument.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// spawnObservabilityLoggerImportPath is the full import path fileImportsLogger matches against.
const spawnObservabilityLoggerImportPath = "github.com/Knatte18/loomyard/internal/logger"

// spawnObservabilityScanRoots are the module-relative directories this guard walks.
var spawnObservabilityScanRoots = []string{"internal", "cmd"}

// spawnObservabilityAllowedSpawners is the Live-Substrate Spawn Observability allowlist:
// module-relative, slash-separated file paths that are permitted to contain a real, unlogged
// exec.Command/exec.CommandContext call, each with a one-line reason — mirroring
// cmd/lyx/tierpurity_test.go's allowedSpawners and cmd/lyx/sandbox_coverage_test.go's
// excludedModules style.
//
// The first three entries are exemptions INSIDE the rule: each owes a written reason because it is a
// real spawn site the invariant would otherwise require logged, but is structurally barred from doing
// so. The last two are OUTSIDE the rule: the walk reaches them because they live under internal/ or
// cmd/, but they were never governed by this invariant in the first place — a test-fixture builder and
// a test-timing harness, neither reachable from a lyx command. tools/ sites need no entry at all: the
// walk never visits tools/.
var spawnObservabilityAllowedSpawners = map[string]string{
	"internal/gitexec/gitexec.go": "structurally barred: internal/logger imports internal/lyxcwd, which imports " +
		"internal/gitexec, so importing logger here would close an import cycle; gitexec.Run already returns a " +
		"*GitError carrying args, dir, exit code, and stderr, so the diagnostic is not actually lost",
	"internal/gitkit/gitkit.go": "structurally barred by the gitkit Leaf Invariant's pinned import list " +
		"(stdlib, lyxcwd, weftname, configengine, lyxdirs only)",
	"internal/githubclient/token.go": "structurally barred by the GitHub Auth Invariant's leaf allowlist " +
		"(enforced by internal/githubclient/leaf_enforcement_test.go); the failure is logged at both production " +
		"callers instead, internal/selfreportengine/selfreport.go and internal/landingshed/publish.go",
	"internal/hubforge/hub.go": "not governed: a test-fixture builder, not a code path reachable from a lyx command",
	"cmd/testtiming/main.go":   "not governed: a test-timing harness, not a code path reachable from a lyx command",
}

// spawnObservabilityMinScannedFiles is the vacuous-scan floor for this guard's two-root walk of
// internal/ and cmd/, mirroring checkedCallMinScannedFiles in cmd/lyx/checkedcall_test.go, so a
// misconfigured walk fails loudly rather than silently passing on a near-empty scan.
const spawnObservabilityMinScannedFiles = 200

// TestSpawnObservability_ProductionSpawnsAreLogged walks every non-test .go file under internal/ and
// cmd/ and fails if any of them, other than a spawnObservabilityAllowedSpawners entry, contains a real
// exec.Command/exec.CommandContext call and does not import internal/logger.
func TestSpawnObservability_ProductionSpawnsAreLogged(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH, mirroring every sibling
	// guard in this package so this gate never blocks a minimal environment.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// Resolve the module root via `go env GOMOD` rather than assuming the test's working directory.
	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v\n%s", err, out)
	}
	goMod := strings.TrimSpace(string(out))
	if goMod == "" || goMod == os.DevNull {
		t.Skip("no enclosing Go module (go env GOMOD is empty)")
	}
	moduleRoot := filepath.Dir(goMod)

	var scanned int
	var failures []string

	for _, rootRel := range spawnObservabilityScanRoots {
		rootDir := filepath.Join(moduleRoot, rootRel)

		walkErr := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}

			relPath, relErr := filepath.Rel(moduleRoot, path)
			if relErr != nil {
				return relErr
			}
			// Normalize to slash-separated form before any comparison: filepath.WalkDir yields
			// backslash paths on Windows (the primary dev OS).
			relPath = filepath.ToSlash(relPath)
			scanned++

			fset := token.NewFileSet()
			astFile, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return fmt.Errorf("failed to parse %s: %w", relPath, parseErr)
			}

			if !fileHasUnloggedSpawn(astFile) {
				return nil
			}
			if _, allowlisted := spawnObservabilityAllowedSpawners[relPath]; allowlisted {
				return nil
			}

			failures = append(failures, fmt.Sprintf(
				"%s: contains a real exec.Command/exec.CommandContext call but does not import internal/logger "+
					"— import it and log the spawn, or add a spawnObservabilityAllowedSpawners entry in "+
					"cmd/lyx/spawnobservability_test.go with a reason",
				relPath,
			))
			return nil
		})
		if walkErr != nil {
			t.Fatalf("failed to walk %s: %v", rootDir, walkErr)
		}
	}

	// Vacuous-scan protection: fewer than minimum found means misconfiguration.
	if scanned < spawnObservabilityMinScannedFiles {
		t.Fatalf("spawn-observability guard: only scanned %d non-test .go file(s) under %v; expected at least %d — the walk may be misconfigured", scanned, spawnObservabilityScanRoots, spawnObservabilityMinScannedFiles)
	}

	sort.Strings(failures)
	if len(failures) > 0 {
		t.Errorf("Live-Substrate Spawn Observability invariant violated (see CONSTRAINTS.md):\n%s", strings.Join(failures, "\n"))
	}
}

// fileHasUnloggedSpawn reports whether astFile contains a real exec.Command/exec.CommandContext call
// and does not import internal/logger. It is the detection helper both the tree-wide walk above and
// TestSpawnObservability_DetectionHelper below exercise, so the walk's exact matching behaviour is
// pinned against small in-memory sources rather than only against the live tree.
func fileHasUnloggedSpawn(file *ast.File) bool {
	if fileImportsLogger(file) {
		return false
	}
	return fileHasSpawnCall(file)
}

// fileImportsLogger reports whether file imports internal/logger by its full import path.
func fileImportsLogger(file *ast.File) bool {
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) == spawnObservabilityLoggerImportPath {
			return true
		}
	}
	return false
}

// fileHasSpawnCall reports whether file contains a real *ast.CallExpr whose Fun is an
// *ast.SelectorExpr selecting Command or CommandContext off the file's own os/exec import name
// (respecting an import alias). A file with no os/exec import has no spawn site by construction, and a
// doc-comment mention of exec.Command is never a *ast.CallExpr in the first place.
func fileHasSpawnCall(file *ast.File) bool {
	execName, ok := execImportName(file)
	if !ok {
		return false
	}

	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if ident.Name != execName {
			return true
		}
		if sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext" {
			found = true
			return false
		}
		return true
	})
	return found
}

// execImportName returns the local identifier file's source uses to refer to the os/exec package —
// the import's alias if it has one, "exec" otherwise — and reports whether file imports os/exec at
// all.
func execImportName(file *ast.File) (name string, ok bool) {
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != "os/exec" {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name, true
		}
		return "exec", true
	}
	return "", false
}

// TestSpawnObservability_DetectionHelper exercises fileHasUnloggedSpawn directly against small
// in-memory Go sources, independent of the live tree, pinning the exact matching behaviour the header
// comment's "AST, not substring" section argues for — most importantly the doc-comment case, which is
// the phantom-violation shape a substring guard would get wrong.
func TestSpawnObservability_DetectionHelper(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "unlogged real spawn is reported",
			src: `package p

import "os/exec"

func run() {
	exec.Command("git", "status")
}
`,
			want: true,
		},
		{
			name: "spawn with logger import is not reported",
			src: `package p

import (
	"os/exec"

	"github.com/Knatte18/loomyard/internal/logger"
)

func run() {
	logger.Info("p: spawn")
	exec.Command("git", "status")
}
`,
			want: false,
		},
		{
			name: "exec.Command mention only in a doc comment is not a call",
			src: `// Package p spawns exec.Command internally for its git operations.
package p
`,
			want: false,
		},
		{
			name: "aliased os/exec import is reported",
			src: `package p

import execpkg "os/exec"

func run() {
	execpkg.Command("git", "status")
}
`,
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "test.go", tt.src, 0)
			if err != nil {
				t.Fatalf("failed to parse test source: %v", err)
			}
			if got := fileHasUnloggedSpawn(file); got != tt.want {
				t.Errorf("fileHasUnloggedSpawn() = %v; want %v", got, tt.want)
			}
		})
	}
}
