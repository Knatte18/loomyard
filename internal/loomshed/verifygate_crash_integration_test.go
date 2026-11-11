// verifygate_crash_integration_test.go covers the Terminal flag NewVerifyGate sets on a failed verify, over a real scratch repo and real shell processes.

package loomshed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestVerifyGate_TerminalOnlyForARunnerCrash(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name         string
		logLines     []string
		exit         string
		wantTerminal bool
		wantFindings string
	}{
		{name: "a runner crash line is terminal", logLines: []string{gateslot.RunnerCrashLine("tier1 (exit 134, SIGABRT)")}, exit: "134", wantTerminal: true, wantFindings: "tier1 (exit 134, SIGABRT)"},
		{name: "a plain failure is not terminal", logLines: []string{"FAIL"}, exit: "3", wantFindings: "exit code 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worktree, verifyDir := t.TempDir(), t.TempDir()
			gitkit.Git(t, worktree, "init", "-b", "main")
			gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
			gitkit.Git(t, worktree, "config", "user.name", "Test")
			gitkit.CommitFile(t, worktree, "a.txt", "a\n", "init")
			logSource := filepath.Join(t.TempDir(), "gate.log")
			if err := os.WriteFile(logSource, []byte(strings.Join(tt.logLines, "\n")+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			command := "cat '" + logSource + "'; exit " + tt.exit

			got, err := NewVerifyGate(worktree, verifyDir, "Webster-Burler gate", func() (string, error) { return command, nil }, nil, nil, nil, VerifyBudget{})()

			if err != nil {
				t.Fatalf("gate() error = %v; want nil", err)
			}
			if got.Passed || got.Terminal != tt.wantTerminal {
				t.Errorf("gate() = passed %v, terminal %v; want a failure with terminal %v", got.Passed, got.Terminal, tt.wantTerminal)
			}
			if !strings.Contains(got.Findings, tt.wantFindings) {
				t.Errorf("gate() findings = %q; want %q in them", got.Findings, tt.wantFindings)
			}
		})
	}
}
