//go:build integration

// lintcomments_integration_test.go drives the lint-comments verb over a real repository.

package loomcli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestLintCommentsCmd_OverARepository walks one repository through a wrapped new comment, a clean tree and a committed range.
// The steps share the repository and run in order.
func TestLintCommentsCmd_OverARepository(t *testing.T) {
	dir := t.TempDir()
	gitkit.Git(t, dir, "init", "-q")
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
	write("clean.go", "package p\n\n// a comment that ends here.\nfunc Clean() {}\n")
	gitkit.Git(t, dir, "add", "-A")
	gitkit.Git(t, dir, "commit", "-q", "-m", "clean")
	base := gitkit.Git(t, dir, "rev-parse", "HEAD")

	c := &loomCLI{env: shedrecipe.Env{WorktreeRoot: dir}}
	lint := func(args ...string) (int, envelope.Envelope) {
		t.Helper()
		var out bytes.Buffer
		exitCode := clihelp.Execute(c.lintCommentsCmd(), &out, args)
		return exitCode, envelope.Decode(t, out.String())
	}

	write("wrapped.go", "package p\n\n// a comment that wraps in the\n// middle of a sentence.\nfunc Wrapped() {}\n")
	t.Run("a wrapped new comment fails with its file, line and text", func(t *testing.T) {
		exitCode, env := lint()
		if exitCode != 1 || env.OK {
			t.Fatalf("exit code = %d, ok = %v; want 1, false", exitCode, env.OK)
		}
		findings, _ := env.Raw["findings"].([]any)
		if want := []any{"wrapped.go:3: // a comment that wraps in the"}; !slices.Equal(findings, want) {
			t.Errorf("findings = %v; want %v", findings, want)
		}
	})

	if err := os.Remove(filepath.Join(dir, "wrapped.go")); err != nil {
		t.Fatalf("Remove(wrapped.go): %v", err)
	}
	t.Run("a clean tree succeeds", func(t *testing.T) {
		if exitCode, env := lint(); exitCode != 0 || !env.OK {
			t.Errorf("exit code = %d, ok = %v; want 0, true", exitCode, env.OK)
		}
	})

	write("wrapped.go", "package p\n\n// a comment that wraps in the\n// middle of a sentence.\nfunc Wrapped() {}\n")
	gitkit.Git(t, dir, "add", "-A")
	gitkit.Git(t, dir, "commit", "-q", "-m", "wrapped")
	t.Run("--base lints the committed range", func(t *testing.T) {
		if exitCode, env := lint(); exitCode != 0 || !env.OK {
			t.Errorf("worktree form: exit code = %d, ok = %v; want 0, true for a committed tree", exitCode, env.OK)
		}
		exitCode, env := lint("--base", base)
		if exitCode != 1 || env.OK {
			t.Fatalf("--base form: exit code = %d, ok = %v; want 1, false", exitCode, env.OK)
		}
		if findings, _ := env.Raw["findings"].([]any); len(findings) != 1 {
			t.Errorf("findings = %v; want the one wrapped comment", findings)
		}
	})
}
