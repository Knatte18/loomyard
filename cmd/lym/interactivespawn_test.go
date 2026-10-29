//go:build tier0

// interactivespawn_test.go enforces `PATTERN-interactive-spawn`: no production Go file spawns claude headless, with `-p` or `--print`.
// It parses each file, so a comment naming the forbidden form never counts.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedHeadlessSpawn is the `PATTERN-interactive-spawn` allowlist, keyed by module-relative file path.
// An entry admits every finding in its file, and its reason names why the print flag there launches no agent.
var allowedHeadlessSpawn = []scankit.Entry{}

// claudeEngineDir is the engine directory that composes claude's pane-shell launch lines from flag fragments,
// so a print flag in any of its strings is a finding.
const claudeEngineDir = "internal/shuttleengine/claudeengine/"

// isPrintFlag reports whether token is claude's headless print flag in any spelling.
func isPrintFlag(token string) bool {
	return token == "-p" || token == "--print" || strings.HasPrefix(token, "--print=")
}

// namesClaudeBinary reports whether token names the claude executable, by bare name or by path.
func namesClaudeBinary(token string) bool {
	base := token[strings.LastIndexAny(token, `/\`)+1:]
	return base == "claude" || base == "claude.exe"
}

// stringLiteralValue returns the unquoted value of expr when it is a string literal.
func stringLiteralValue(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}

// exprNamesClaude reports whether expr names the claude executable:
// a string literal naming it, or an identifier, selector or call whose name holds "claude" in any case.
func exprNamesClaude(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		value, ok := stringLiteralValue(e)
		return ok && namesClaudeBinary(value)
	case *ast.Ident:
		return strings.Contains(strings.ToLower(e.Name), "claude")
	case *ast.SelectorExpr:
		return strings.Contains(strings.ToLower(e.Sel.Name), "claude") || exprNamesClaude(e.X)
	case *ast.CallExpr:
		return exprNamesClaude(e.Fun)
	}
	return false
}

// callSpawnsHeadlessClaude reports whether call takes a print-flag string literal and another argument naming the claude executable.
func callSpawnsHeadlessClaude(call *ast.CallExpr) bool {
	hasFlag, hasClaude := false, false
	for _, arg := range call.Args {
		if value, ok := stringLiteralValue(arg); ok && isPrintFlag(value) {
			hasFlag = true
			continue
		}
		if exprNamesClaude(arg) {
			hasClaude = true
		}
	}
	return hasFlag && hasClaude
}

// literalTokens splits a string literal's value at whitespace and trims shell quote characters from each token.
func literalTokens(value string) []string {
	tokens := strings.Fields(value)
	for i, tok := range tokens {
		tokens[i] = strings.Trim(tok, "\"'`")
	}
	return tokens
}

// headlessClaudeLine reports whether tokens name the claude executable and carry a print flag after it.
func headlessClaudeLine(tokens []string) bool {
	sawClaude := false
	for _, tok := range tokens {
		if namesClaudeBinary(tok) {
			sawClaude = true
			continue
		}
		if sawClaude && isPrintFlag(tok) {
			return true
		}
	}
	return false
}

// headlessSpawnFailures walks the production Go files under opts and returns one failure line per headless claude spawn shape, with the number of files scanned.
func headlessSpawnFailures(t *testing.T, opts scankit.Options, allow *scankit.Allowlist) (failures []string, scanned int) {
	t.Helper()
	scanned = scankit.Walk(t, opts, func(f *scankit.File) {
		rel := filepath.ToSlash(f.Rel)
		file := f.AST(t, parser.SkipObjectResolution)
		inClaudeEngine := strings.HasPrefix(rel, claudeEngineDir)
		var fileFailures []string
		report := func(pos token.Pos, shape string) {
			fileFailures = append(fileFailures, fmt.Sprintf("%s:%d: %s; launch the agent through shuttle as an interactive tmux session", rel, f.FileSet().Position(pos).Line, shape))
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				if callSpawnsHeadlessClaude(node) {
					report(node.Pos(), "a call passes claude a print flag")
				}
			case *ast.BasicLit:
				value, ok := stringLiteralValue(node)
				if !ok {
					return true
				}
				tokens := literalTokens(value)
				switch {
				case headlessClaudeLine(tokens):
					report(node.Pos(), "a string runs claude with a print flag")
				case inClaudeEngine && slices.ContainsFunc(tokens, isPrintFlag):
					report(node.Pos(), "a claude engine string carries a print flag")
				}
			}
			return true
		})
		if len(fileFailures) > 0 && !allow.Allowed(rel) {
			failures = append(failures, fileFailures...)
		}
	})
	sort.Strings(failures)
	return failures, scanned
}

// TestInteractiveSpawn_NoHeadlessClaude fails for every production call or string that runs claude with `-p` or `--print`, unless allowlisted.
//
//lym:guard
func TestInteractiveSpawn_NoHeadlessClaude(t *testing.T) {
	t.Parallel()
	allow := scankit.NewAllowlist(allowedHeadlessSpawn)
	failures, scanned := headlessSpawnFailures(t, scankit.Options{}, allow)
	scankit.RequireFloor(t, scanned, 20, "interactive spawn scan")
	allow.RequireNoStale(t)
	if len(failures) > 0 {
		t.Errorf("`PATTERN-interactive-spawn` violated:\n%s", strings.Join(failures, "\n"))
	}
}

func TestInteractiveSpawn_FixtureTrees(t *testing.T) {
	t.Parallel()
	const header = "package p\n\nimport \"os/exec\"\n\n"
	tests := []struct {
		name     string
		files    map[string]string
		wantFail int
	}{
		{"Command with a claude literal and -p", map[string]string{"p/a.go": header + "func f() { exec.Command(\"claude\", \"-p\", \"hi\") }\n"}, 1},
		{"CommandContext with a claude path and --print", map[string]string{"p/a.go": header + "func f() { exec.CommandContext(nil, \"/usr/bin/claude\", \"--print\") }\n"}, 1},
		{"claude variable with --print=", map[string]string{"p/a.go": header + "func f(claudePath string) { exec.Command(claudePath, \"--print=json\") }\n"}, 1},
		{"claude config selector", map[string]string{"p/a.go": header + "type c struct{ Binary string }\n\nfunc f(claude c) { exec.Command(claude.Binary, \"-p\") }\n"}, 1},
		{"claude binary call", map[string]string{"p/a.go": header + "func claudeBinary() string { return \"claude\" }\n\nfunc f() { exec.Command(claudeBinary(), \"-p\") }\n"}, 1},
		{"pane line string", map[string]string{"p/a.go": "package p\n\nvar line = \"claude --model opus -p 'hi'\"\n"}, 1},
		{"quoted claude.exe in a pane line", map[string]string{"p/a.go": "package p\n\nvar line = `& 'C:\\bin\\claude.exe' --print`\n"}, 1},
		{"print flag fragment in the claude engine", map[string]string{"internal/shuttleengine/claudeengine/a.go": "package claudeengine\n\nvar flag = \" -p\"\n"}, 1},
		{"interactive launch", map[string]string{"p/a.go": header + "func f() { exec.Command(\"claude\", \"--dangerously-skip-permissions\", \"hi\") }\n"}, 0},
		{"tmux -p", map[string]string{"p/a.go": header + "func f() { exec.Command(\"tmux\", \"display-message\", \"-p\", \"#{pid}\") }\n"}, 0},
		{"print flag before claude", map[string]string{"p/a.go": "package p\n\nvar line = \"tmux display-message -p claude\"\n"}, 0},
		{"mention in a comment", map[string]string{"p/a.go": "package p\n\n// Never run claude -p.\nvar x = 1\n"}, 0},
		{"print flag fragment outside the claude engine", map[string]string{"p/a.go": "package p\n\nvar flag = \" -p\"\n"}, 0},
		{"test file is not scanned", map[string]string{"p/a_test.go": header + "func f() { exec.Command(\"claude\", \"-p\") }\n"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			for rel, body := range tt.files {
				path := filepath.Join(base, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			failures, _ := headlessSpawnFailures(t, scankit.Options{Base: base}, scankit.NewAllowlist(nil))
			if len(failures) != tt.wantFail {
				t.Errorf("headlessSpawnFailures() = %v; want %d failures", failures, tt.wantFail)
			}
		})
	}
}

func TestInteractiveSpawn_Allowlist(t *testing.T) {
	t.Parallel()
	const header = "package p\n\nimport \"os/exec\"\n\n"
	tests := []struct {
		name      string
		body      string
		wantFail  int
		wantStale []string
	}{
		{"entry admits the findings of its file", header + "func f() { exec.Command(\"claude\", \"-p\") }\n", 0, nil},
		{"entry for a file with no finding is stale", header + "func f() { exec.Command(\"claude\") }\n", 0, []string{"p/a.go"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			if err := os.MkdirAll(filepath.Join(base, "p"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(base, "p", "a.go"), []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}
			allow := scankit.NewAllowlist([]scankit.Entry{{Key: "p/a.go", Why: "fixture"}})
			failures, _ := headlessSpawnFailures(t, scankit.Options{Base: base}, allow)
			if len(failures) != tt.wantFail {
				t.Errorf("headlessSpawnFailures() = %v; want %d failures", failures, tt.wantFail)
			}
			if stale := allow.Stale(); !slices.Equal(stale, tt.wantStale) {
				t.Errorf("Stale() = %v; want %v", stale, tt.wantStale)
			}
		})
	}
}
