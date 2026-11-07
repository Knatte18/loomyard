// spawnobservability_test.go enforces the mechanical half of `PATTERN-child-processes`.
// A production .go file under internal/ or cmd/ that makes a real spawn must import internal/logger.
// A spawn is an exec.Command or exec.CommandContext call.
// Otherwise the file needs a written-reason entry in spawnObservabilityAllowedSpawners.
// The guard parses each file with go/parser and counts only real *ast.CallExpr calls.
// A raw-substring scan would count doc prose that mentions exec.Command as a spawn.
// Detection is file-level import presence.
// A file that imports the logger for an unrelated line and spawns unlogged still passes.
// Only the spawn half is guarded.
// A new unlogged exec.Command is nearly always a real miss.
// A new outcome-switch branch may legitimately return normally for the caller to branch on,
// so a file-level check on it would fire on correct code.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// spawnObservabilityLoggerImportPath is the full import path of internal/logger.
// A spawning file must import it to count as logged.
const spawnObservabilityLoggerImportPath = "github.com/Knatte18/loomyard/internal/logger"

// spawnObservabilityScanRoots are the module-relative directories this guard walks.
var spawnObservabilityScanRoots = []string{"internal", "cmd"}

// spawnObservabilityAllowedSpawners is the Live-Substrate Spawn Observability allowlist.
// It holds the module-relative, slash-separated paths of files allowed to make a real, unlogged spawn:
// exec.Command or exec.CommandContext calls. Each entry carries a written reason.
//
// Structurally barred entries are exemptions inside the rule.
// Each is a real spawn site that the invariant would otherwise require to be logged,
// but it is structurally barred from logging.
// Not-governed entries are outside the rule.
// The walk reaches them because they live under internal/ or cmd/, but this invariant never governed them.
// Each is test tooling, not a code path reachable from a lym command.
// tools/ sites need no entry at all: the walk never visits tools/.
var spawnObservabilityAllowedSpawners = []scankit.Entry{
	{
		Key: "internal/gitkit/gitkit.go",
		Why: "structurally barred by the gitkit Leaf Invariant's pinned import list " +
			"(stdlib, lymcwd, the fabric name package, configengine, lymdirs only)",
	},
	{
		Key: "internal/githubclient/token.go",
		Why: "structurally barred by the GitHub Auth Invariant's leaf allowlist " +
			"(enforced by internal/githubclient/leaf_enforcement_test.go); the failure is logged at both production " +
			"callers instead, internal/selfreportengine/selfreport.go and internal/landingshed/publish.go",
	},
	{
		Key: "internal/proc/tied.go",
		Why: "structurally barred: internal/logger imports internal/proc, so importing logger here would " +
			"close an import cycle; the callers of StartTied log the spawn and the teardown",
	},
	{
		Key: "internal/hubforge/hub.go",
		Why: "not governed: a test-fixture builder, not a code path reachable from a lym command",
	},
	{
		Key: "internal/testkit/lymbin/lymbin.go",
		Why: "not governed: a test-fixture builder, not a code path reachable from a lym command",
	},
	{
		Key: "internal/testkit/tmuxkit/tmuxkit.go",
		Why: "not governed: a test-fixture isolation kit, not a code path reachable from a lym command",
	},
	{
		Key: "cmd/testtiming/main.go",
		Why: "not governed: a test-timing harness, not a code path reachable from a lym command",
	},
	{
		Key: "cmd/testtiming/redundancy.go",
		Why: "not governed: the redundancy mode of the test-timing harness, not a code path reachable from a lym command",
	},
	{
		Key: "cmd/testtiming/resources.go",
		Why: "not governed: the resource mode of the test-timing harness, not a code path reachable from a lym command",
	},
}

// spawnObservabilityMinScannedFiles is the minimum number of files the walk over internal/ and cmd/ must scan.
// A misconfigured walk fails loudly rather than silently passing on a near-empty scan.
const spawnObservabilityMinScannedFiles = 200

// TestSpawnObservability_ProductionSpawnsAreLogged walks every non-test .go file under internal/ and cmd/.
// It fails on each file with a real exec.Command or exec.CommandContext call that does not import internal/logger.
// A file listed in spawnObservabilityAllowedSpawners is exempt.
//
//lym:guard
func TestSpawnObservability_ProductionSpawnsAreLogged(t *testing.T) {
	tiergate.Require(t, 0)
	allow := scankit.NewAllowlist(spawnObservabilityAllowedSpawners)
	var failures []string

	scanned := scankit.Walk(t, scankit.Options{Roots: spawnObservabilityScanRoots}, func(f *scankit.File) {
		if !fileHasUnloggedSpawn(f.AST(t, 0)) {
			return
		}
		if allow.Allowed(f.Rel) {
			return
		}
		failures = append(failures, fmt.Sprintf(
			"%s: contains a real exec.Command/exec.CommandContext call but does not import internal/logger "+
				"— import it and log the spawn, or add a spawnObservabilityAllowedSpawners entry in "+
				"cmd/lym/spawnobservability_test.go with a reason",
			f.Rel,
		))
	})

	scankit.RequireFloor(t, scanned, spawnObservabilityMinScannedFiles, "spawn-observability guard")
	allow.RequireNoStale(t)

	sort.Strings(failures)
	if len(failures) > 0 {
		t.Errorf("`PATTERN-child-processes` violated:\n%s", strings.Join(failures, "\n"))
	}
}

// fileHasUnloggedSpawn reports whether file contains a real spawn call and does not import internal/logger.
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

// fileHasSpawnCall reports whether file contains a real *ast.CallExpr to exec.Command or exec.CommandContext.
// The exec package is matched under the file's own import name, so an import alias is respected.
// It also counts a call to `(*quarry.Repo).DeltaGit`.
// That method spawns git through internal/gitsrc, so a call to it is a real spawn.
// The DeltaGit match uses the selector's method name alone, so a doc-comment mention is not a call.
// It does not check the receiver's static type.
func fileHasSpawnCall(file *ast.File) bool {
	execName, hasExec := execImportName(file)

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
		if sel.Sel.Name == "DeltaGit" {
			found = true
			return false
		}
		if !hasExec {
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

// execImportName returns the identifier file uses for the os/exec package:
// the import's alias if it has one, "exec" otherwise.
// It reports whether file imports os/exec at all.
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

// TestSpawnObservability_DetectionHelper exercises fileHasUnloggedSpawn directly against small in-memory Go sources.
// It runs independently of the live tree.
// It pins the exact matching behaviour.
// Most importantly, it pins the doc-comment case, which a substring match would flag as a spawn.
//
//testtiming:keep proves the TestSpawnObservability_ProductionSpawnsAreLogged guard's AST matching fires on unlogged spawns and ignores doc-comment mentions
func TestSpawnObservability_DetectionHelper(t *testing.T) {
	tiergate.Require(t, 0)
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
		{
			name: "DeltaGit call site with no logger import is reported",
			src: `package p

import "github.com/Knatte18/quarry/quarry"

func run(r *quarry.Repo) {
	r.DeltaGit("from", "to", ".")
}
`,
			want: true,
		},
		{
			name: "DeltaGit call site with a logger import is not reported",
			src: `package p

import (
	"github.com/Knatte18/quarry/quarry"

	"github.com/Knatte18/loomyard/internal/logger"
)

func run(r *quarry.Repo) {
	logger.Info("p: spawn")
	r.DeltaGit("from", "to", ".")
}
`,
			want: false,
		},
		{
			name: "DeltaGit mention only in a doc comment is not a call",
			src: `// Package p spawns via DeltaGit internally.
package p
`,
			want: false,
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
