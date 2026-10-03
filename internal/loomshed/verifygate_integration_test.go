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

// verifyGateScratch is a one-commit repo, a separate anchor holding the plan, and a verify directory outside the worktree.
type verifyGateScratch struct {
	anchor    string
	worktree  string
	verifyDir string
}

// newVerifyGateScratch writes a plan whose `## verify:` section is command, or has none when command is empty.
func newVerifyGateScratch(t *testing.T, command string) verifyGateScratch {
	t.Helper()
	s := verifyGateScratch{anchor: t.TempDir(), worktree: t.TempDir(), verifyDir: t.TempDir()}
	gitkit.Git(t, s.worktree, "init", "-b", "main")
	gitkit.Git(t, s.worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, s.worktree, "config", "user.name", "Test")
	gitkit.CommitFile(t, s.worktree, "a.txt", "a\n", "init")

	plan := firstCardPlan(true, "none", "internal/firstcard/new.go", "")
	if command != "" {
		plan.Sections = append(plan.Sections, plankit.Section{Heading: "verify:", Body: command})
	}
	plankit.Write(t, filepath.Join(s.anchor, lyxdirs.LyxDirName, "plan"), plan)
	return s
}

func TestVerifyGate_PassesWithoutRunningOnVerifiedTree(t *testing.T) {
	s := newVerifyGateScratch(t, "true")
	paths := verifytree.NewPaths(s.worktree, s.verifyDir)
	if res, err := verifytree.Verify(context.Background(), paths, verifytree.Site{Label: "Webster-Burler gate"}, "true"); err != nil || res.Status != verifytree.StatusPassed {
		t.Fatalf("seed Verify = %+v, %v; want a pass", res, err)
	}
	if err := os.Remove(paths.Log); err != nil {
		t.Fatalf("remove the seeded log: %v", err)
	}

	res, err := NewVerifyGate(s.anchor, s.worktree, s.verifyDir, "Webster-Burler gate")()
	if err != nil {
		t.Fatalf("gate() error = %v; want nil", err)
	}
	if !res.Passed {
		t.Errorf("gate() = %+v; want a pass on a tree the record names", res)
	}
	if _, err := os.Stat(paths.Log); err == nil {
		t.Errorf("verify log %s exists; want the command not to have run", paths.Log)
	}
}

func TestVerifyGate_FailsWithFindingsOnFailingCommand(t *testing.T) {
	s := newVerifyGateScratch(t, "echo boom-marker; exit 3")
	res, err := NewVerifyGate(s.anchor, s.worktree, s.verifyDir, "Webster-Burler gate")()
	if err != nil {
		t.Fatalf("gate() error = %v; want nil", err)
	}
	if res.Passed {
		t.Fatalf("gate() passed; want a failure")
	}
	for _, want := range []string{"exit code 3", filepath.Join(s.verifyDir, "verify.log"), "boom-marker"} {
		if !strings.Contains(res.Findings, want) {
			t.Errorf("Findings = %q; want it to contain %q", res.Findings, want)
		}
	}
}

func TestVerifyGate_FailsNamingPathsOnDirtyTree(t *testing.T) {
	s := newVerifyGateScratch(t, "true")
	if err := os.WriteFile(filepath.Join(s.worktree, "stray.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := NewVerifyGate(s.anchor, s.worktree, s.verifyDir, "Webster-Burler gate")()
	if err != nil {
		t.Fatalf("gate() error = %v; want nil", err)
	}
	if res.Passed || !strings.Contains(res.Findings, "stray.txt") {
		t.Errorf("gate() = %+v; want a failure naming stray.txt", res)
	}
}

func TestVerifyGate_PassesOnPlanWithoutVerifySection(t *testing.T) {
	s := newVerifyGateScratch(t, "")
	res, err := NewVerifyGate(s.anchor, s.worktree, s.verifyDir, "Webster-Burler gate")()
	if err != nil {
		t.Fatalf("gate() error = %v; want nil", err)
	}
	if !res.Passed {
		t.Errorf("gate() = %+v; want a pass without a verify section", res)
	}
}
