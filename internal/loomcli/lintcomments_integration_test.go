//go:build integration

// lintcomments_integration_test.go drives the lint-comments and lint-pattern verbs over a real repository.

package loomcli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestLintCommentsCmd_OverARepository walks one repository through a wrapped new comment, a clean tree and a committed range, asserting lint-comments and lint-pattern over the same tree at each step.
// The steps share the repository and run in order, so the test is parallel as a whole and no step is.
func TestLintCommentsCmd_OverARepository(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	gitkit.Git(t, dir, "init", "-q")
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
	overviewOfSize := func(size int, lineEnd string) string {
		return strings.Repeat("x", size-len(lineEnd)) + lineEnd
	}
	committedOverview := overviewOfSize(pattern.HeadroomBytes-100, "\n")
	grownOverview := overviewOfSize(pattern.HeadroomBytes+50, "\n")
	write("clean.go", "package p\n\n// a comment that ends here.\nfunc Clean() {}\n")
	write("PATTERN.md", committedOverview)
	gitkit.Git(t, dir, "add", "-A")
	gitkit.Git(t, dir, "commit", "-q", "-m", "clean")
	base := gitkit.Git(t, dir, "rev-parse", "HEAD")

	c := &loomCLI{env: shedrecipe.Env{WorktreeRoot: dir}}
	lint := func(verb string, args ...string) (int, envelope.Envelope) {
		t.Helper()
		cmd := c.lintCommentsCmd()
		if verb == "lint-pattern" {
			cmd = c.lintPatternCmd()
		}
		var out bytes.Buffer
		exitCode := clihelp.Execute(cmd, &out, args)
		return exitCode, envelope.Decode(t, out.String())
	}

	write("wrapped.go", "package p\n\n// a comment that wraps in the\n// middle of a sentence.\nfunc Wrapped() {}\n")
	write("PATTERN.md", grownOverview)
	t.Run("a wrapped new comment fails with its file, line and text", func(t *testing.T) {
		exitCode, env := lint("lint-comments")
		if exitCode != 1 || env.OK {
			t.Fatalf("exit code = %d, ok = %v; want 1, false", exitCode, env.OK)
		}
		findings, _ := env.Raw["findings"].([]any)
		if want := []any{"wrapped.go:3: // a comment that wraps in the"}; !slices.Equal(findings, want) {
			t.Errorf("findings = %v; want %v", findings, want)
		}
	})
	t.Run("an overview grown past the line fails with the bytes to cut", func(t *testing.T) {
		exitCode, env := lint("lint-pattern")
		if exitCode != 1 || env.OK {
			t.Fatalf("exit code = %d, ok = %v; want 1, false", exitCode, env.OK)
		}
		findings, _ := env.Raw["findings"].([]any)
		if len(findings) != 1 || !strings.Contains(findings[0].(string), "cut 50 bytes") {
			t.Errorf("findings = %v; want one message naming 50 bytes to cut", findings)
		}
	})

	if err := os.Remove(filepath.Join(dir, "wrapped.go")); err != nil {
		t.Fatalf("Remove(wrapped.go): %v", err)
	}
	write("PATTERN.md", committedOverview)
	t.Run("a clean tree succeeds", func(t *testing.T) {
		for _, verb := range []string{"lint-comments", "lint-pattern"} {
			if exitCode, env := lint(verb); exitCode != 0 || !env.OK {
				t.Errorf("%s: exit code = %d, ok = %v; want 0, true", verb, exitCode, env.OK)
			}
		}
	})
	write("PATTERN.md", strings.ReplaceAll(committedOverview, "\n", "\r\n"))
	t.Run("an untouched overview with CRLF line endings succeeds", func(t *testing.T) {
		if exitCode, env := lint("lint-pattern"); exitCode != 0 || !env.OK {
			t.Errorf("exit code = %d, ok = %v; want 0, true", exitCode, env.OK)
		}
	})

	write("wrapped.go", "package p\n\n// a comment that wraps in the\n// middle of a sentence.\nfunc Wrapped() {}\n")
	write("PATTERN.md", grownOverview)
	gitkit.Git(t, dir, "add", "-A")
	gitkit.Git(t, dir, "commit", "-q", "-m", "wrapped")
	t.Run("--base lints the committed range", func(t *testing.T) {
		for _, verb := range []string{"lint-comments", "lint-pattern"} {
			if exitCode, env := lint(verb); exitCode != 0 || !env.OK {
				t.Errorf("%s worktree form: exit code = %d, ok = %v; want 0, true for a committed tree", verb, exitCode, env.OK)
			}
			exitCode, env := lint(verb, "--base", base)
			if exitCode != 1 || env.OK {
				t.Fatalf("%s --base form: exit code = %d, ok = %v; want 1, false", verb, exitCode, env.OK)
			}
			if findings, _ := env.Raw["findings"].([]any); len(findings) != 1 {
				t.Errorf("%s findings = %v; want the one finding", verb, findings)
			}
		}
	})
}
