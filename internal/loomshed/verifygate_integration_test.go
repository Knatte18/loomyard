//go:build integration

// verifygate_integration_test.go exercises NewVerifyGate against a real scratch repo and real shell processes, so it carries the integration tag.

package loomshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// verifyLogs returns the per-run verify logs in verifyDir, so a step can tell whether a command ran.
func verifyLogs(t *testing.T, verifyDir string) []string {
	t.Helper()
	logs, err := filepath.Glob(filepath.Join(verifyDir, "verify-*.log"))
	if err != nil {
		t.Fatal(err)
	}
	return logs
}

// TestVerifyGate_Scenario drives NewVerifyGate in its round form over one one-commit repo and a verify directory outside the worktree.
// The steps run in order and each sets the told verify command; the last step dirties the worktree, so it must stay last.
// The scenario calls no t.Parallel in its steps because they share the repo, and the top level calls it because nothing else touches that fixture.
func TestVerifyGate_Scenario(t *testing.T) {
	t.Parallel()

	worktree, verifyDir := t.TempDir(), t.TempDir()
	gitkit.Git(t, worktree, "init", "-b", "main")
	gitkit.Git(t, worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, worktree, "config", "user.name", "Test")
	gitkit.CommitFile(t, worktree, "a.txt", "a\n", "init")

	var told string
	setVerifyCommand := func(t *testing.T, command string) {
		t.Helper()
		told = command
	}
	runGate := func(t *testing.T) (passed bool, findings string) {
		t.Helper()
		got, err := NewVerifyGate(worktree, verifyDir, "Webster-Burler gate", func() (string, error) { return told, nil }, nil, nil)()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		return got.Passed, got.Findings
	}

	if !t.Run("passes on an empty told command", func(t *testing.T) {
		setVerifyCommand(t, "")
		if passed, findings := runGate(t); !passed {
			t.Errorf("gate() findings = %q; want a pass without a told command", findings)
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
		for _, want := range []string{"exit code 3", filepath.Join(verifyDir, "verify-1.log"), "boom-marker"} {
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
		if res, err := verifytree.Verify(context.Background(), paths, verifytree.Site{Label: "Webster-Burler gate"}, "true", verifytree.Timeout, nil); err != nil || res.Status != verifytree.StatusPassed {
			t.Fatalf("seed Verify = %+v, %v; want a pass", res, err)
		}
		logsBefore := verifyLogs(t, verifyDir)

		if passed, findings := runGate(t); !passed {
			t.Errorf("gate() findings = %q; want a pass on a tree the record names", findings)
		}
		if logs := verifyLogs(t, verifyDir); len(logs) != len(logsBefore) {
			t.Errorf("verify logs = %q; want %q, the command not to have run", logs, logsBefore)
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

// roundGateFixture is a repo holding a two-package Go module and a verify directory outside the worktree.
// The told verify command appends to planRanLog, so a line there means the told command ran in full.
type roundGateFixture struct {
	worktree, verifyDir     string
	planCommand, planRanLog string
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
	got, err := NewVerifyGate(f.worktree, f.verifyDir, "Webster-Burler gate", func() (string, error) { return f.planCommand, nil }, nil, nil)()
	if err != nil {
		t.Fatalf("gate() error = %v; want nil", err)
	}
	return got.Passed, got.Findings
}

// wholeDiffGate runs the whole-diff form of the gate once, told the fixture's command and the given merge base reader.
func (f roundGateFixture) wholeDiffGate(command, mergeBase func() (string, error)) (shuttleengine.GateResult, error) {
	return NewVerifyGate(f.worktree, f.verifyDir, "Darn gate", command, mergeBase, nil)()
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

	f := roundGateFixture{worktree: t.TempDir(), verifyDir: t.TempDir()}
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
		logsBefore := verifyLogs(t, f.verifyDir)
		if passed, findings := f.runGate(t); !passed {
			t.Fatalf("gate() findings = %q; want a pass", findings)
		}
		if logs := verifyLogs(t, f.verifyDir); len(logs) != len(logsBefore) {
			t.Errorf("verify logs = %q; want %q, the command not to have run", logs, logsBefore)
		}
	}) {
		return
	}

	if !t.Run("appends the failing tests and the impacted-set tmux pass while a publish failure record is present", func(t *testing.T) {
		head := f.commitFiles(t, map[string]string{"b/b.go": "package b\n\nconst Changed = 2\n"})
		derivation, err := impactset.Derive(f.worktree, planPass.Commit)
		if err != nil || derivation.Command == "" || len(derivation.Packages) == 0 {
			t.Fatalf("Derive() = %+v, %v; want a derived command with packages", derivation, err)
		}
		failure := verifytree.PublishFailure{Kind: verifytree.FailureKindPublishVerify, Head: head, Tests: []verifytree.FailedTest{{Package: "example.com/m/b", Test: "TestB"}}}
		if err := verifytree.WritePublishFailure(paths, failure); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := verifytree.RemovePublishFailure(paths); err != nil {
				t.Error(err)
			}
		})

		if passed, findings := f.runGate(t); !passed {
			t.Fatalf("gate() findings = %q; want a pass", findings)
		}
		want := derivation.Command + " && go test -tags tmux -run '^TestB$' example.com/m/b && go test -tags tmux " + strings.Join(derivation.Packages, " ")
		if pass, ok := verifytree.LatestPass(paths, want); !ok || pass.Commit != head {
			t.Errorf("LatestPass(extended command) = %+v, %v; want a pass at %s of %q", pass, ok, head, want)
		}
		if _, ok := verifytree.LatestPass(paths, derivation.Command); ok {
			t.Errorf("a pass of the plain derived command is recorded; want the extended command to have run instead")
		}
	}) {
		return
	}

	baseOf := func(sha string) func() (string, error) { return func() (string, error) { return sha, nil } }
	toldCommand := func() (string, error) { return f.planCommand, nil }

	if !t.Run("whole-diff form runs the told command in full and records its pass under it", func(t *testing.T) {
		head := gitkit.Git(t, f.worktree, "rev-parse", "HEAD")
		ranBefore := f.planRanCount(t)
		got, err := f.wholeDiffGate(toldCommand, baseOf(planPass.Commit))
		if err != nil || !got.Passed {
			t.Fatalf("gate() = %+v, %v; want a pass", got, err)
		}
		if ran := f.planRanCount(t); ran != ranBefore+1 {
			t.Errorf("the told command ran %d times; want %d, one more in full", ran, ranBefore+1)
		}
		if pass, ok := verifytree.LatestPass(paths, f.planCommand); !ok || pass.Commit != head {
			t.Errorf("LatestPass(told command) = %+v, %v; want a pass at %s", pass, ok, head)
		}
	}) {
		return
	}

	if !t.Run("whole-diff form appends the failing tests and the tmux pass over all packages while a publish failure record is present", func(t *testing.T) {
		head := f.commitFiles(t, map[string]string{"b/b.go": "package b\n\nconst Changed = 3\n"})
		failure := verifytree.PublishFailure{Kind: verifytree.FailureKindPublishVerify, Head: head, Tests: []verifytree.FailedTest{{Package: "example.com/m/b", Test: "TestB"}}}
		if err := verifytree.WritePublishFailure(paths, failure); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := verifytree.RemovePublishFailure(paths); err != nil {
				t.Error(err)
			}
		})

		got, err := f.wholeDiffGate(toldCommand, baseOf(planPass.Commit))
		if err != nil || !got.Passed {
			t.Fatalf("gate() = %+v, %v; want a pass", got, err)
		}
		want := f.planCommand + " && go test -tags tmux -run '^TestB$' example.com/m/b && go test -tags tmux ./..."
		if pass, ok := verifytree.LatestPass(paths, want); !ok || pass.Commit != head {
			t.Errorf("LatestPass(extended command) = %+v, %v; want a pass at %s of %q", pass, ok, head, want)
		}
	}) {
		return
	}

	t.Run("whole-diff form returns the read errors and an empty command as errors and runs nothing", func(t *testing.T) {
		logsBefore := verifyLogs(t, f.verifyDir)
		ranBefore := f.planRanCount(t)
		tests := []struct {
			name      string
			command   func() (string, error)
			mergeBase func() (string, error)
			want      string
		}{
			{"CommandError", func() (string, error) { return "", errors.New("command source down") }, baseOf(planPass.Commit), "command source down"},
			{"EmptyCommand", func() (string, error) { return "", nil }, baseOf(planPass.Commit), "empty command"},
			{"MergeBaseError", toldCommand, func() (string, error) { return "", errors.New("merge base unreadable") }, "merge base unreadable"},
		}
		for _, tt := range tests {
			got, err := f.wholeDiffGate(tt.command, tt.mergeBase)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("%s: gate() error = %v; want it to contain %q", tt.name, err, tt.want)
			}
			if got.Passed {
				t.Errorf("%s: gate() passed; want no pass alongside the error", tt.name)
			}
		}
		if logs := verifyLogs(t, f.verifyDir); len(logs) != len(logsBefore) {
			t.Errorf("verify logs = %q; want %q, no command to have run", logs, logsBefore)
		}
		if ran := f.planRanCount(t); ran != ranBefore {
			t.Errorf("the told command ran %d times; want %d, none", ran, ranBefore)
		}
	})

	if !t.Run("fails with the file and line of a misplaced marker", func(t *testing.T) {
		f.commitFiles(t, map[string]string{"a/a_test.go": "package a\n\nimport \"testing\"\n\n//lyx:guard\nvar misplaced = 1\n\nfunc TestA(t *testing.T) {}\n"})
		passed, findings := f.runGate(t)
		if passed || !strings.Contains(findings, "a/a_test.go:5") || !strings.Contains(findings, "directly above") {
			t.Errorf("gate() passed = %v, findings = %q; want a failure naming a/a_test.go:5 and the way to place the marker", passed, findings)
		}
	}) {
		return
	}

	if !t.Run("fails on a wrapped new comment before any test runs", func(t *testing.T) {
		f.commitFiles(t, map[string]string{
			"a/a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n",
			"a/a.go":      "package a\n\n// Wrapped comment that breaks in the\n// middle of a sentence.\nfunc Wrapped() {}\n",
		})
		logsBefore := verifyLogs(t, f.verifyDir)
		passed, findings := f.runGate(t)
		if passed || !strings.Contains(findings, "a/a.go:3") {
			t.Errorf("gate() passed = %v, findings = %q; want a failure naming a/a.go:3", passed, findings)
		}
		if logs := verifyLogs(t, f.verifyDir); len(logs) != len(logsBefore) {
			t.Errorf("verify logs = %q; want %q, the lint to fail before any command ran", logs, logsBefore)
		}

		got, err := f.wholeDiffGate(toldCommand, baseOf(planPass.Commit))
		if err != nil || got.Passed || !strings.Contains(got.Findings, "a/a.go:3") {
			t.Errorf("whole-diff gate() = %+v, %v; want a failure naming a/a.go:3 from the merge base", got, err)
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
