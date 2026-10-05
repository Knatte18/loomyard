//go:build smoke

// smoke_bashstdin_test.go is the live proof that the Bash stdin rewrite hook (claudeengine's bashStdinPrefix)
// leaves the agent's own permission rules in charge:
// a REAL claude whose project settings allow `git status` and deny `git reset --hard` still runs the allowed command and is still refused the denied one,
// with the `exec </dev/null; ` prefix applied to both.
// A failure here is the trigger for D3's fallback of denying interpreter shapes instead of rewriting.
// Follows the conventions of smoke_guardrail_test.go, whose helpers this file reuses.

package shuttlecli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedcli"
)

// TestSmokeBashStdinRewriteKeepsPermissionRules runs a real agent in a hub whose prime worktree has one dirty tracked file,
// with project-level allow and deny rules installed by the test so it does not depend on the operator's own settings.
// The run reaching done with `git status`'s output written proves the allowed command stayed allowed;
// the file still being dirty proves the denied `git reset --hard HEAD` stayed denied.
func TestSmokeBashStdinRewriteKeepsPermissionRules(t *testing.T) {
	claudeBinaryPath(t)

	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.Path)
	worktree := h.PrimeWorktree()
	t.Chdir(worktree)
	t.Cleanup(func() {
		var buf bytes.Buffer
		reedcli.RunCLI(&buf, []string{"down"})
	})

	settingsDir := filepath.Join(worktree, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("create %s: %v", settingsDir, err)
	}
	rules := `{"permissions":{"allow":["Bash(git status)"],"deny":["Bash(git reset --hard:*)"]}}`
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(rules), 0o644); err != nil {
		t.Fatalf("write project settings: %v", err)
	}

	dirtyFile := filepath.Join(worktree, "README")
	original, err := os.ReadFile(dirtyFile)
	if err != nil {
		t.Fatalf("read %s: %v", dirtyFile, err)
	}
	dirtyContent := string(original) + "dirty line\n"
	if err := os.WriteFile(dirtyFile, []byte(dirtyContent), 0o644); err != nil {
		t.Fatalf("dirty %s: %v", dirtyFile, err)
	}

	var reedOut bytes.Buffer
	if code := reedcli.RunCLI(&reedOut, []string{"up"}); code != 0 {
		t.Fatalf("reed up = %d; want 0, output: %s", code, reedOut.String())
	}

	outputPath := filepath.Join(worktree, "smoke-bashstdin-output.txt")
	prompt := fmt.Sprintf(
		"Run `git status` with your Bash tool and write its output to %s. "+
			"Then run `git reset --hard HEAD` with your Bash tool. "+
			"If that command is refused, accept the refusal and stop; do not try another way to discard the changes.",
		outputPath,
	)

	var out bytes.Buffer
	code := RunCLI(&out, []string{
		"run",
		"--prompt", prompt,
		"--output-file", outputPath,
		"--model", smokeClaudeModel,
		"--timeout", "5m",
	})
	if code != 0 {
		t.Fatalf("shuttle run = %d; want 0, output: %s", code, out.String())
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("parse run result: %v; output: %s", err, out.String())
	}
	if outcome, _ := result["outcome"].(string); outcome != "done" {
		t.Fatalf("run outcome = %q; want \"done\"; output: %s", outcome, out.String())
	}

	status, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if !strings.Contains(string(status), "README") {
		t.Errorf("output file = %q; want git status's output naming the dirty README (the allowed command must still run behind the prefix)", status)
	}

	got, err := os.ReadFile(dirtyFile)
	if err != nil {
		t.Fatalf("read %s: %v", dirtyFile, err)
	}
	if string(got) != dirtyContent {
		t.Errorf("%s = %q; want it still dirty (%q): the denied git reset --hard must stay denied behind the prefix", dirtyFile, got, dirtyContent)
	}
}
