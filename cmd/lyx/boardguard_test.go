// boardguard_test.go enforces the Weft Git Invariant's board carve-out:
// internal/boardengine's non-test production code never imports internal/gitrepo
// or internal/gitexec directly, and never shells out to `git` itself. boardengine
// routes every weft-repo git operation through internal/fabricengine's
// CommitWeftAt/PushWeftAt instead (see the plan's "no import cycle from
// boardengine into fabricengine" Shared Decision) — this guard is the mechanical
// enforcement that keeps a future change from regressing that routing back into a
// raw git call. See CONSTRAINTS.md's Weft Git Invariant.

package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// boardGuardBannedImports are the import paths internal/boardengine's production
// code may never reference directly: the two packages that own raw git access.
// boardengine must instead route weft-repo git operations through
// internal/fabricengine's CommitWeftAt/PushWeftAt.
var boardGuardBannedImports = map[string]bool{
	"github.com/Knatte18/loomyard/internal/gitrepo": true,
	"github.com/Knatte18/loomyard/internal/gitexec": true,
}

// boardGuardExecSpawnTokens identify an exec.Command or exec.CommandContext call,
// matched on the SAME LINE as the quoted "git" argument (see lineHasBannedGitSpawn)
// -- the same same-line co-occurrence discipline ghguard_test.go's "gh" check uses,
// generalized to "git" here so internal/boardengine/spawn.go's git-free
// exec.Command(exe, "board", "--board-path", abs, "sync") self-relaunch call
// passes cleanly: that line never mentions "git".
var boardGuardExecSpawnTokens = []string{"exec.Command", "exec.CommandContext"}

// boardGuardMinScannedFiles is the vacuous-scan floor for this guard's
// single-directory walk of internal/boardengine. The package has 9 non-test .go
// files today (board.go, config.go, layer.go, render.go, spawn.go, store.go,
// sync.go, task.go, template.go); fewer than 5 found means the directory
// resolution is misconfigured rather than the package having genuinely shrunk.
const boardGuardMinScannedFiles = 5

// TestBoardGuard_NoRawGitImportOrShellOut walks internal/boardengine's non-test
// *.go files (skipping the boardtest subdirectory, a sibling package of
// integration tests that legitimately spawn git via lyxtest.CopyWeft, not
// production code this guard's ban applies to) and fails if any file imports
// internal/gitrepo or internal/gitexec directly, or shells out to `git` via
// exec.Command/exec.CommandContext.
func TestBoardGuard_NoRawGitImportOrShellOut(t *testing.T) {
	// Skip cleanly rather than fail when the go toolchain is not on PATH,
	// mirroring ghguard_test.go and gitrepoboundary_test.go so this gate never
	// blocks a minimal environment.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// Resolve the module root via `go env GOMOD` rather than assuming the
	// test's working directory, exactly as ghguard_test.go and
	// gitrepoboundary_test.go do, so the walk is cwd-independent.
	out, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v\n%s", err, out)
	}
	goMod := strings.TrimSpace(string(out))
	if goMod == "" || goMod == os.DevNull {
		t.Skip("no enclosing Go module (go env GOMOD is empty)")
	}
	dir := filepath.Join(filepath.Dir(goMod), "internal", "boardengine")

	var scanned int
	var failures []string

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// boardtest is a sibling package of integration tests that legitimately
			// spawn git via lyxtest.CopyWeft -- this guard's import/shell-out ban
			// applies to boardengine's own production code, not its test fixtures.
			if d.Name() == "boardtest" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		scanned++

		relPath, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}

		if imp, bad := firstBannedBoardImport(path); bad {
			failures = append(failures, fmt.Sprintf(
				"%s: imports banned package %q -- route weft-repo git operations through internal/fabricengine's CommitWeftAt/PushWeftAt instead (see CONSTRAINTS.md's Weft Git Invariant)",
				relPath, imp,
			))
		}

		if token, bad := firstBannedGitSpawn(string(data)); bad {
			failures = append(failures, fmt.Sprintf(
				"%s: contains banned git shell-out token %q -- route weft-repo git operations through internal/fabricengine's CommitWeftAt/PushWeftAt instead (see CONSTRAINTS.md's Weft Git Invariant)",
				relPath, token,
			))
		}

		return nil
	})
	if walkErr != nil {
		t.Fatalf("failed to walk internal/boardengine: %v", walkErr)
	}

	// Vacuous-scan protection: a mis-resolved root that still finds a handful of
	// files must not pass.
	if scanned < boardGuardMinScannedFiles {
		t.Fatalf("board guard: only scanned %d non-test .go file(s) in %s; expected at least %d -- the directory resolution may be misconfigured", scanned, dir, boardGuardMinScannedFiles)
	}

	if len(failures) > 0 {
		t.Errorf("Weft Git Invariant violated (see CONSTRAINTS.md):\n%s", strings.Join(failures, "\n"))
	}
}

// firstBannedBoardImport parses path's import declarations (using go/parser with
// ImportsOnly, mirroring internal/pattern/leaf_enforcement_test.go's AST walk, so
// only real import statements are inspected, never string literals in doc
// comments) and reports the first import path found in boardGuardBannedImports.
func firstBannedBoardImport(path string) (string, bool) {
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return "", false
	}
	for _, imp := range astFile.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if boardGuardBannedImports[importPath] {
			return importPath, true
		}
	}
	return "", false
}

// firstBannedGitSpawn reports the first banned git shell-out token found in
// content. It runs two passes. The first is line-scoped (lineHasBannedGitSpawn):
// the precise, common `exec.Command(..., "git", ...)` form, reported with its
// same-line token pairing. The second is a file-level backstop for a git
// shell-out whose "git" literal and exec.Command call land on DIFFERENT lines —
// variable indirection (`g := "git"; exec.Command(g, ...)`) or a gofmt-split
// multi-line call (`exec.Command(\n\t"git",\n)`) — both of which slip past the
// same-line check yet are exactly the natural refactors that would reintroduce a
// raw git shell-out into internal/boardengine. The backstop flags a file that
// contains BOTH an exec spawn token and a standalone quoted "git" literal
// anywhere in it. It cannot false-positive on internal/boardengine/spawn.go's
// legitimate self-relaunch, which uses exec.Command but contains no "git"
// literal at all; and matching the standalone quoted "git" token (not a bare
// substring) keeps it from tripping on comments (`// git`) or non-standalone
// mentions (`"run git"`). Dataflow-level evasion (building "git" from
// non-literal parts, e.g. "gi"+"t") remains out of scope, as for every
// grep-style guard in this repo — the AST import ban is the airtight primary
// defense.
func firstBannedGitSpawn(content string) (token string, bad bool) {
	for _, line := range strings.Split(content, "\n") {
		if spawnToken, bad := lineHasBannedGitSpawn(line); bad {
			return spawnToken, true
		}
	}
	if strings.Contains(content, `"git"`) {
		for _, spawnToken := range boardGuardExecSpawnTokens {
			if strings.Contains(content, spawnToken) {
				return spawnToken + ` ... "git" (different lines)`, true
			}
		}
	}
	return "", false
}

// lineHasBannedGitSpawn reports whether line contains both an exec.Command/
// exec.CommandContext spawn token and the quoted "git" argument -- the pairing
// that identifies a git shell-out regardless of how many arguments (e.g. a
// leading context.Context) separate the two tokens on the line. This same-line
// co-occurrence requirement (not a bare substring ban) is what lets
// internal/boardengine/spawn.go's legitimate, git-free
// exec.Command(exe, "board", "--board-path", abs, "sync") self-relaunch call
// pass cleanly, since that line never mentions "git".
func lineHasBannedGitSpawn(line string) (token string, bad bool) {
	if !strings.Contains(line, `"git"`) {
		return "", false
	}
	for _, spawnToken := range boardGuardExecSpawnTokens {
		if strings.Contains(line, spawnToken) {
			return spawnToken + ` ... "git"`, true
		}
	}
	return "", false
}

// TestBoardGuard_ShellOutDetection pins firstBannedGitSpawn's detection against
// crafted source snippets rather than the live tree, proving the guard is sound
// (not merely that it currently passes vacuously). It covers the naive same-line
// shell-out AND the two natural evasions the earlier same-line-only check missed
// — variable indirection and a gofmt-split multi-line call — while confirming
// the two shapes that must stay unflagged: spawn.go's git-free self-relaunch
// (exec.Command with no "git" literal) and a bare "git" literal with no spawn.
func TestBoardGuard_ShellOutDetection(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "same-line git shell-out is flagged",
			content: `exec.Command("git", "status")`,
			want:    true,
		},
		{
			name:    "context-first same-line git shell-out is flagged",
			content: `exec.CommandContext(ctx, "git", "status")`,
			want:    true,
		},
		{
			name:    "variable-indirected git command name is flagged",
			content: "g := \"git\"\nexec.Command(g, \"status\")",
			want:    true,
		},
		{
			name:    "gofmt-split multi-line git shell-out is flagged",
			content: "exec.Command(\n\t\"git\",\n\t\"status\",\n)",
			want:    true,
		},
		{
			name:    "spawn.go self-relaunch (exec.Command, no git literal) is not flagged",
			content: `exec.Command(exe, "board", "--board-path", abs, "sync")`,
			want:    false,
		},
		{
			name:    "bare git literal with no spawn is not flagged",
			content: `msg := "git"`,
			want:    false,
		},
		{
			name:    "non-standalone git mention with a spawn is not flagged",
			content: "// runs git under the hood\nexec.Command(exe, \"sync\")",
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := firstBannedGitSpawn(tt.content)
			if got != tt.want {
				t.Errorf("firstBannedGitSpawn(%q) flagged = %v; want %v", tt.content, got, tt.want)
			}
		})
	}
}
