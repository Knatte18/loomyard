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
	"github.com/Knatte18/loomyard/internal/impactset"
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

// roundGateFixture is a repo holding a two-package Go module, a separate anchor holding the plan and a verify directory outside the worktree.
// The plan's `## verify:` command appends to planRanLog, so a line there means the plan's own command ran.
type roundGateFixture struct {
	anchor, worktree, verifyDir string
	planCommand, planRanLog     string
}

// commitFiles writes each file under the worktree, commits them all and returns the new HEAD.
func (f roundGateFixture) commitFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(f.worktree, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitkit.Git(t, f.worktree, "add", "-A")
	gitkit.Git(t, f.worktree, "commit", "-q", "-m", "change")
	return gitkit.Git(t, f.worktree, "rev-parse", "HEAD")
}

func (f roundGateFixture) runGate(t *testing.T) (passed bool, findings string) {
	t.Helper()
	got, err := NewVerifyGate(f.anchor, f.worktree, f.verifyDir, "Webster-Burler gate")()
	if err != nil {
		t.Fatalf("gate() error = %v; want nil", err)
	}
	return got.Passed, got.Findings
}

func (f roundGateFixture) planRanCount(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(f.planRanLog)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "plan\n")
}

// TestVerifyGate_RoundScenario drives NewVerifyGate over a repo holding a small Go module, through the narrowed command, the lint and the refusals.
// The steps run in order against the commits the steps before left, and the last one dirties the worktree.
func TestVerifyGate_RoundScenario(t *testing.T) {
	t.Parallel()

	f := roundGateFixture{anchor: t.TempDir(), worktree: t.TempDir(), verifyDir: t.TempDir()}
	f.planRanLog = filepath.ToSlash(filepath.Join(t.TempDir(), "plan-ran.log"))
	f.planCommand = "echo plan >> " + f.planRanLog
	gitkit.Git(t, f.worktree, "init", "-b", "main")
	gitkit.Git(t, f.worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, f.worktree, "config", "user.name", "Test")
	f.commitFiles(t, map[string]string{
		"go.mod":      "module example.com/m\n\ngo 1.21\n",
		"a/a.go":      "package a\n",
		"a/a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n",
		"b/b.go":      "package b\n",
		"b/b_test.go": "package b\n\nimport \"testing\"\n\nfunc TestB(t *testing.T) {}\n",
	})
	plan := firstCardPlan(true, "none", "internal/firstcard/new.go", "")
	plan.Sections = append(plan.Sections, plankit.Section{Heading: "verify:", Body: f.planCommand})
	plankit.Write(t, filepath.Join(f.anchor, lyxdirs.LyxDirName, "plan"), plan)
	paths := verifytree.NewPaths(f.worktree, f.verifyDir)

	if !t.Run("runs the plan's command and passes the lint with no recorded pass", func(t *testing.T) {
		if passed, findings := f.runGate(t); !passed {
			t.Fatalf("gate() findings = %q; want a pass", findings)
		}
		if got := f.planRanCount(t); got != 1 {
			t.Errorf("the plan's command ran %d times; want once", got)
		}
		if _, ok := verifytree.LatestPass(paths, f.planCommand); !ok {
			t.Errorf("no recorded pass of the plan's command; want one")
		}
	}) {
		return
	}

	planPass, _ := verifytree.LatestPass(paths, f.planCommand)
	if !t.Run("runs the narrowed command for one package and keeps the plan's record entry", func(t *testing.T) {
		head := f.commitFiles(t, map[string]string{"b/b.go": "package b\n\nconst Changed = 1\n"})
		derivation, err := impactset.Derive(f.worktree, planPass.Commit)
		if err != nil || derivation.Command == "" {
			t.Fatalf("Derive() = %+v, %v; want a derived command", derivation, err)
		}
		if passed, findings := f.runGate(t); !passed {
			t.Fatalf("gate() findings = %q; want a pass", findings)
		}
		if got := f.planRanCount(t); got != 1 {
			t.Errorf("the plan's command ran %d times; want it left to the narrowed command", got)
		}
		roundPass, ok := verifytree.LatestPass(paths, derivation.Command)
		if !ok || roundPass.Commit != head {
			t.Errorf("LatestPass(derived command) = %+v, %v; want a pass at %s", roundPass, ok, head)
		}
		if kept, ok := verifytree.LatestPass(paths, f.planCommand); !ok || kept != planPass {
			t.Errorf("LatestPass(plan command) = %+v, %v; want the entry %+v kept", kept, ok, planPass)
		}
	}) {
		return
	}

	if !t.Run("skips on a second arrival at the same tree", func(t *testing.T) {
		if err := os.Remove(paths.Log); err != nil {
			t.Fatalf("remove the log: %v", err)
		}
		if passed, findings := f.runGate(t); !passed {
			t.Fatalf("gate() findings = %q; want a pass", findings)
		}
		if _, err := os.Stat(paths.Log); err == nil {
			t.Errorf("verify log %s exists; want the command not to have run", paths.Log)
		}
	}) {
		return
	}

	if !t.Run("fails with the file and line of a misplaced marker", func(t *testing.T) {
		f.commitFiles(t, map[string]string{"a/a_test.go": "package a\n\nimport \"testing\"\n\n//lyx:guard\nvar misplaced = 1\n\nfunc TestA(t *testing.T) {}\n"})
		passed, findings := f.runGate(t)
		if passed || !strings.Contains(findings, "a/a_test.go:5") {
			t.Errorf("gate() passed = %v, findings = %q; want a failure naming a/a_test.go:5", passed, findings)
		}
	}) {
		return
	}

	if !t.Run("fails on a wrapped new comment before any test runs", func(t *testing.T) {
		f.commitFiles(t, map[string]string{
			"a/a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n",
			"a/a.go":      "package a\n\n// Wrapped comment that breaks in the\n// middle of a sentence.\nfunc Wrapped() {}\n",
		})
		if err := os.Remove(paths.Log); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		passed, findings := f.runGate(t)
		if passed || !strings.Contains(findings, "a/a.go:3") {
			t.Errorf("gate() passed = %v, findings = %q; want a failure naming a/a.go:3", passed, findings)
		}
		if _, err := os.Stat(paths.Log); err == nil {
			t.Errorf("verify log %s exists; want the lint to fail before any command ran", paths.Log)
		}
	}) {
		return
	}

	t.Run("refuses a dirty tree", func(t *testing.T) {
		f.commitFiles(t, map[string]string{"a/a.go": "package a\n\n// Wrapped comment now breaks at the end of a sentence.\n// Each sentence has its own line.\nfunc Wrapped() {}\n"})
		if err := os.WriteFile(filepath.Join(f.worktree, "stray.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if passed, findings := f.runGate(t); passed || !strings.Contains(findings, "stray.txt") {
			t.Errorf("gate() passed = %v, findings = %q; want a failure naming stray.txt", passed, findings)
		}
	})
}
