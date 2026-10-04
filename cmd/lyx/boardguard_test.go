// boardguard_test.go enforces the Fabric Git Invariant's board carve-out: internal/boardengine's
// non-test production code never imports internal/gitrepo or internal/gitexec directly,
// and never shells out to `git` itself.
// boardengine routes every fabric-repo git operation through internal/fabricengine's
// commit and push entry points for the records side instead (see the plan's "no import cycle from boardengine into
// fabricengine" Shared Decision) — this guard is the mechanical enforcement that keeps a future
// change from regressing that routing back into a raw git call.
// See `PATTERN-fabric-git`.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// boardGuardBannedImports are the import paths internal/boardengine's production
// code may never reference directly: the two packages that own raw git access.
// boardengine must instead route fabric-repo git operations through
// internal/fabricengine's commit and push entry points for the records side.
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

// TestBoardGuard_NoRawGitImportOrShellOut walks internal/boardengine's non-test *.go files
// (skipping the boardtest subdirectory, a sibling package of integration tests that legitimately
// spawn git via hubforge.NewHub, not production code this guard's ban applies to) and fails if any
// file imports internal/gitrepo or internal/gitexec directly, or shells out to `git` via
// exec.Command/exec.CommandContext.
func TestBoardGuard_NoRawGitImportOrShellOut(t *testing.T) {
	const dir = "internal/boardengine"

	var scanned int
	var failures []string

	scankit.Walk(t, scankit.Options{Roots: []string{dir}}, func(f *scankit.File) {
		// boardtest is a sibling package of integration tests — skip it.
		if strings.HasPrefix(f.Rel, dir+"/boardtest/") {
			return
		}
		scanned++

		if imp, bad := firstBannedBoardImport(f.AST(t, parser.ImportsOnly)); bad {
			failures = append(failures, fmt.Sprintf(
				"%s: imports banned package %q -- route fabric-repo git operations through internal/fabricengine's commit and push entry points for the records side instead (see `PATTERN-fabric-git`)",
				f.Rel, imp,
			))
		}

		if token, bad := firstBannedGitSpawn(string(f.Data)); bad {
			failures = append(failures, fmt.Sprintf(
				"%s: contains banned git shell-out token %q -- route fabric-repo git operations through internal/fabricengine's commit and push entry points for the records side instead (see `PATTERN-fabric-git`)",
				f.Rel, token,
			))
		}
	})

	scankit.RequireFloor(t, scanned, boardGuardMinScannedFiles, "board guard")

	if len(failures) > 0 {
		t.Errorf("`PATTERN-fabric-git` violated:\n%s", strings.Join(failures, "\n"))
	}
}

// firstBannedBoardImport reports the first banned import in astFile's import declarations.
func firstBannedBoardImport(astFile *ast.File) (string, bool) {
	for _, imp := range astFile.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		if boardGuardBannedImports[importPath] {
			return importPath, true
		}
	}
	return "", false
}

// firstBannedGitSpawn reports the first banned git shell-out token found in content, using two-pass detection.
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

// lineHasBannedGitSpawn reports whether line contains both a spawn token and "git" on the same line.
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

// TestBoardGuard_ShellOutDetection verifies firstBannedGitSpawn's detection against crafted
// snippets.
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
