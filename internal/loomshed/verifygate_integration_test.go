//go:build integration

// verifygate_integration_test.go exercises NewVerifyGate against a real scratch repo and real shell processes, so it carries the integration tag.

package loomshed

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// TestVerifyGate_Scenario drives NewVerifyGate over one one-commit repo, a separate anchor holding the plan and a verify directory outside the worktree.
// The steps run in order and each rewrites the plan's `## verify:` section; the last step dirties the worktree, so it must stay last.
// The scenario calls no t.Parallel in its steps because they share the repo, and the top level calls it because nothing else touches that fixture.
func TestVerifyGate_Scenario(t *testing.T) {
	t.Parallel()

	anchor, worktree, verifyDir := t.TempDir(), t.TempDir(), t.TempDir()
	gitkit.Git(t, worktree, "init", "-b", "main")
	gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, worktree, "config", "user.name", "Test")
	gitkit.CommitFile(t, worktree, "a.txt", "a\n", "init")

	// setVerifyCommand rewrites the plan with command as its `## verify:` section, or none when command is empty.
	setVerifyCommand := func(t *testing.T, command string) {
		t.Helper()
		plan := firstCardPlan(true, "none", "internal/firstcard/new.go", "")
		if command != "" {
			plan.Sections = append(plan.Sections, plankit.Section{Heading: "verify:", Body: command})
		}
		plankit.Write(t, filepath.Join(anchor, lyxdirs.LyxDirName, "plan"), plan)
	}
	runGate := func(t *testing.T) (passed bool, findings string) {
		t.Helper()
		got, err := NewVerifyGate(anchor, worktree, verifyDir, "Webster-Burler gate")()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		return got.Passed, got.Findings
	}

	if !t.Run("passes on a plan without a verify section", func(t *testing.T) {
		setVerifyCommand(t, "")
		if passed, findings := runGate(t); !passed {
			t.Errorf("gate() findings = %q; want a pass without a verify section", findings)
		}
	}) {
		return
	}

	if !t.Run("fails with findings on a failing command", func(t *testing.T) {
		setVerifyCommand(t, "echo boom-marker; exit 3")
		passed, findings := runGate(t)
		if passed {
			t.Fatalf("gate() passed; want a failure")
		}
		for _, want := range []string{"exit code 3", filepath.Join(verifyDir, "verify.log"), "boom-marker"} {
			if !strings.Contains(findings, want) {
				t.Errorf("Findings = %q; want it to contain %q", findings, want)
			}
		}
	}) {
		return
	}

	if !t.Run("passes without running on a verified tree", func(t *testing.T) {
		setVerifyCommand(t, "true")
		paths := verifytree.NewPaths(worktree, verifyDir)
		if res, err := verifytree.Verify(context.Background(), paths, verifytree.Site{Label: "Webster-Burler gate"}, "true"); err != nil || res.Status != verifytree.StatusPassed {
			t.Fatalf("seed Verify = %+v, %v; want a pass", res, err)
		}
		if err := os.Remove(paths.Log); err != nil {
			t.Fatalf("remove the seeded log: %v", err)
		}

		if passed, findings := runGate(t); !passed {
			t.Errorf("gate() findings = %q; want a pass on a tree the record names", findings)
		}
		if _, err := os.Stat(paths.Log); err == nil {
			t.Errorf("verify log %s exists; want the command not to have run", paths.Log)
		}
	}) {
		return
	}

	t.Run("fails naming the paths of a dirty tree", func(t *testing.T) {
		setVerifyCommand(t, "true")
		if err := os.WriteFile(filepath.Join(worktree, "stray.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if passed, findings := runGate(t); passed || !strings.Contains(findings, "stray.txt") {
			t.Errorf("gate() passed = %v, findings = %q; want a failure naming stray.txt", passed, findings)
		}
	})
}
