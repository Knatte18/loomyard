//go:build tier1

// verifygate_integration_test.go exercises NewVerifyGate against a real scratch repo and real shell processes, so it carries the integration tag.

package loomshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/patternlint"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
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
		got, err := NewVerifyGate(worktree, verifyDir, "Webster-Burler gate", func() (string, error) { return told, nil }, nil, nil, nil)()
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
		if res, err := verifytree.Verify(context.Background(), paths, verifytree.Site{Label: "Webster-Burler gate"}, "true"); err != nil || res.Status != verifytree.StatusPassed {
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

// roundGateConfig is the gate.yaml the round scenario's worktree holds: tier0 untagged, tier1 integration, tier2 tmux.
// Each tier runs a real `go test -json` over its targets.
const roundGateConfig = `slots: 1
scoper: go
tier_parallel:
  tier0: 2
  tier1: 2
  tier2: 2
tiers:
  tier0:
    about: untagged tests
    command: go test -json {targets} {args}
    all_targets: ./...
    counter: go-json
    timeout_min: 5
  tier1:
    about: integration tests
    command: go test -json -tags integration {targets} {args}
    all_targets: ./...
    counter: go-json
    timeout_min: 5
    covers: [0]
  tier2:
    about: tmux tests
    command: go test -json -tags tmux {targets} {args}
    all_targets: ./...
    counter: go-json
    timeout_min: 5
gates:
  card: {tiers: [0], scope: card}
  batch: {tiers: [0, 1], scope: card}
  webster: {tiers: [0], scope: all}
  parent-align: {tiers: [0, 1], scope: all}
  finalize: {tiers: [0, 1], scope: all}
  webster-fix: {tiers: [0, 1], scope: diff}
  darn-write: {tiers: [0, 1], scope: diff}
  code-burler-fix: {tiers: [0, 1], scope: diff}
`

// roundCall spells the site's own call, the `code-burler-fix` key's, which the fix rows tell the gate as its verify command.
func roundCall() string {
	return "lym gate test --wait --gate code-burler-fix"
}

// roundGateFixture is a repo holding a two-package Go module, its own gate.yaml and a verify directory outside the worktree.
// The told verify command is roundCall, which runs the built `lym` over the worktree's gate.yaml.
type roundGateFixture struct {
	worktree, verifyDir string
	command             string
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

// runGate runs the round form of the gate once, told the fixture's command and the given merge base reader, which may be nil.
func (f roundGateFixture) runGate(t *testing.T, mergeBase func() (string, error)) (passed bool, findings string) {
	t.Helper()
	got, err := NewVerifyGate(f.worktree, f.verifyDir, "Code-Burler-Fix gate", func() (string, error) { return f.command, nil }, mergeBase, nil, nil)()
	if err != nil {
		t.Fatalf("gate() error = %v; want nil", err)
	}
	return got.Passed, got.Findings
}

// diffGate runs the darn form of the gate once, told the given command source, merge base reader and tree scope reader.
func (f roundGateFixture) diffGate(command, mergeBase func() (string, error), treeScope func() (string, bool, error)) (shuttleengine.GateResult, error) {
	return NewVerifyGate(f.worktree, f.verifyDir, "Darn-Write gate", command, mergeBase, treeScope, nil)()
}

// TestVerifyGate_RoundScenario drives NewVerifyGate over a repo holding a small Go module and its gate.yaml, through the told command, the lint and the refusals.
// The steps run in order against the commits the steps before left, and the last one dirties the worktree.
// The told command runs the built `lym`, so the test puts a copy of the built binary first on `PATH`; it is not parallel because `t.Setenv` sets that process-global state.
func TestVerifyGate_RoundScenario(t *testing.T) {
	lymbin.PutFirstOnPath(t)

	f := roundGateFixture{worktree: t.TempDir(), verifyDir: t.TempDir(), command: roundCall()}
	gitkit.Git(t, f.worktree, "init", "-b", "main")
	gitkit.Git(t, f.worktree, "config", "user.email", "test@test.com")
	gitkit.Git(t, f.worktree, "config", "user.name", "Test")
	// The runs the round call records under the worktree's .lym directory would otherwise dirty the tree the next call verifies.
	exclude := filepath.Join(f.worktree, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exclude, []byte(lymdirs.DotLymDirName+"/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	configFile := configengine.ConfigFile(f.worktree, "gate")
	if err := os.MkdirAll(filepath.Dir(configFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, []byte(roundGateConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	initial := f.commitFiles(t, map[string]string{
		"go.mod":      "module example.com/m\n\ngo 1.21\n",
		"a/a.go":      "package a\n",
		"a/a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n",
		"b/b.go":      "package b\n",
		"b/b_test.go": "package b\n\nimport \"testing\"\n\nfunc TestB(t *testing.T) {}\n",
	})
	paths := verifytree.NewPaths(f.worktree, f.verifyDir)
	baseOf := func(sha string) func() (string, error) { return func() (string, error) { return sha, nil } }

	if !t.Run("a first round lints from the merge base and fails on a fixed-column comment the round added", func(t *testing.T) {
		f.commitFiles(t, map[string]string{"a/wrapped.go": "package a\n\n// Wrapped comment that breaks in the\n// middle of a sentence.\nfunc FirstWrapped() {}\n"})
		logsBefore := verifyLogs(t, f.verifyDir)
		passed, findings := f.runGate(t, baseOf(initial))
		if passed || !strings.Contains(findings, "a/wrapped.go:3") {
			t.Errorf("gate() passed = %v, findings = %q; want a failure naming a/wrapped.go:3", passed, findings)
		}
		if logs := verifyLogs(t, f.verifyDir); len(logs) != len(logsBefore) {
			t.Errorf("verify logs = %q; want %q, the lint to fail before any command ran", logs, logsBefore)
		}
	}) {
		return
	}

	if !t.Run("runs the told command as it is over the tree and lints nothing with neither a pass nor a merge base", func(t *testing.T) {
		if passed, findings := f.runGate(t, nil); !passed {
			t.Fatalf("gate() findings = %q; want a pass", findings)
		}
		if _, ok := verifytree.LatestPass(paths, f.command); !ok {
			t.Errorf("no recorded pass of %q; want one, the `code-burler-fix` key", f.command)
		}
	}) {
		return
	}

	firstPass, _ := verifytree.LatestPass(paths, f.command)
	if !t.Run("a later round lints from its own recorded pass and ignores a wrapped comment older than it", func(t *testing.T) {
		head := f.commitFiles(t, map[string]string{"b/b.go": "package b\n\nconst Changed = 1\n"})
		if passed, findings := f.runGate(t, baseOf(initial)); !passed {
			t.Fatalf("gate() findings = %q; want a pass, the merge base's wrapped comment lying before the recorded pass", findings)
		}
		if pass, ok := verifytree.LatestPass(paths, f.command); !ok || pass.Commit != head {
			t.Errorf("LatestPass(told command) = %+v, %v; want a pass at %s", pass, ok, head)
		}
	}) {
		return
	}

	if !t.Run("skips on a second arrival at the same tree", func(t *testing.T) {
		logsBefore := verifyLogs(t, f.verifyDir)
		if passed, findings := f.runGate(t, nil); !passed {
			t.Fatalf("gate() findings = %q; want a pass", findings)
		}
		if logs := verifyLogs(t, f.verifyDir); len(logs) != len(logsBefore) {
			t.Errorf("verify logs = %q; want %q, the command not to have run", logs, logsBefore)
		}
	}) {
		return
	}

	toldCommand := func() (string, error) { return f.command, nil }
	toldPass, _ := verifytree.LatestPass(paths, f.command)

	head := func(t *testing.T) string { return gitkit.Git(t, f.worktree, "rev-parse", "HEAD") }
	notScoped := func() (string, bool, error) { return "", false, nil }
	scopedLog := filepath.ToSlash(filepath.Join(t.TempDir(), "scoped-ran.log"))
	scopedCommand := "echo scoped >> " + scopedLog
	scoped := func() (string, bool, error) { return scopedCommand, true, nil }
	scopedRanCount := func(t *testing.T) int {
		t.Helper()
		data, err := os.ReadFile(scopedLog)
		if os.IsNotExist(err) {
			return 0
		}
		if err != nil {
			t.Fatal(err)
		}
		return strings.Count(string(data), "scoped\n")
	}

	if !t.Run("darn form told a tree-scoped answer runs only the scoped command and keeps the told command's record entry", func(t *testing.T) {
		head := f.commitFiles(t, map[string]string{"b/b.go": "package b\n\nconst Changed = 10\n"})
		got, err := f.diffGate(toldCommand, baseOf(firstPass.Commit), scoped)
		if err != nil || !got.Passed {
			t.Fatalf("gate() = %+v, %v; want a pass", got, err)
		}
		if ran := scopedRanCount(t); ran != 1 {
			t.Errorf("the scoped command ran %d times; want once", ran)
		}
		if pass, ok := verifytree.LatestPass(paths, scopedCommand); !ok || pass.Commit != head {
			t.Errorf("LatestPass(scoped command) = %+v, %v; want a pass at %s", pass, ok, head)
		}
		if kept, ok := verifytree.LatestPass(paths, f.command); !ok || kept != toldPass {
			t.Errorf("LatestPass(told command) = %+v, %v; want the entry %+v kept, the told command not to have run", kept, ok, toldPass)
		}
	}) {
		return
	}

	if !t.Run("darn form told an empty tree-scoped command runs nothing and passes", func(t *testing.T) {
		logsBefore, scopedBefore := verifyLogs(t, f.verifyDir), scopedRanCount(t)
		got, err := f.diffGate(toldCommand, baseOf(firstPass.Commit), func() (string, bool, error) { return "", true, nil })
		if err != nil || !got.Passed {
			t.Fatalf("gate() = %+v, %v; want a pass", got, err)
		}
		if logs := verifyLogs(t, f.verifyDir); len(logs) != len(logsBefore) || scopedRanCount(t) != scopedBefore {
			t.Errorf("verify logs = %q, scoped ran %d times; want no command to have run", logs, scopedRanCount(t))
		}
	}) {
		return
	}

	if !t.Run("darn form told a not-scoped answer runs the told command as it is, whatever the merge base", func(t *testing.T) {
		f.commitFiles(t, map[string]string{"a/a.go": "package a\n\nconst Changed = 1\n"})
		base := head(t)
		tip := f.commitFiles(t, map[string]string{"b/b.go": "package b\n\nconst Changed = 12\n"})

		got, err := f.diffGate(toldCommand, baseOf(base), notScoped)
		if err != nil || !got.Passed {
			t.Fatalf("gate() = %+v, %v; want a pass", got, err)
		}
		if pass, ok := verifytree.LatestPass(paths, f.command); !ok || pass.Commit != tip {
			t.Errorf("LatestPass(told command) = %+v, %v; want a pass at %s", pass, ok, tip)
		}
	}) {
		return
	}

	t.Run("darn form returns the read errors and an empty command as errors and runs nothing", func(t *testing.T) {
		logsBefore := verifyLogs(t, f.verifyDir)
		tests := []struct {
			name      string
			command   func() (string, error)
			mergeBase func() (string, error)
			treeScope func() (string, bool, error)
			want      string
		}{
			{"CommandError", func() (string, error) { return "", errors.New("command source down") }, baseOf(firstPass.Commit), notScoped, "command source down"},
			{"EmptyCommand", func() (string, error) { return "", nil }, baseOf(firstPass.Commit), notScoped, "empty command"},
			{"MergeBaseError", toldCommand, func() (string, error) { return "", errors.New("merge base unreadable") }, notScoped, "merge base unreadable"},
			{"NilReader", toldCommand, nil, notScoped, "no merge base reader"},
			{"TreeScopeError", toldCommand, baseOf(firstPass.Commit), func() (string, bool, error) { return "", false, errors.New("tree scope unreadable") }, "tree scope unreadable"},
		}
		for _, tt := range tests {
			got, err := NewVerifyGate(f.worktree, f.verifyDir, "Darn-Write gate", tt.command, tt.mergeBase, tt.treeScope, nil)()
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
	})

	if !t.Run("fails with the file and line of a misplaced marker", func(t *testing.T) {
		// The key's call carries no --base, so the guard scan of a narrowed run is out of reach until the key's diff base is read from the green record.
		t.Skip("the key's call carries no --base, so no guard scan runs over the diff")
		f.commitFiles(t, map[string]string{"a/a_test.go": "package a\n\nimport \"testing\"\n\n//lym:guard\nvar misplaced = 1\n\nfunc TestA(t *testing.T) {}\n"})
		passed, findings := f.runGate(t, nil)
		if passed || !strings.Contains(findings, "a/a_test.go:5") || !strings.Contains(findings, "directly above") {
			t.Errorf("gate() passed = %v, findings = %q; want a failure naming a/a_test.go:5 and the way to place the marker", passed, findings)
		}

		got, err := f.diffGate(toldCommand, baseOf(firstPass.Commit), notScoped)
		if err != nil || got.Passed || !strings.Contains(got.Findings, "a/a_test.go:5") || !strings.Contains(got.Findings, "directly above") {
			t.Errorf("darn gate() = %+v, %v; want a failure naming a/a_test.go:5 and the way to place the marker", got, err)
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
		passed, findings := f.runGate(t, nil)
		if passed || !strings.Contains(findings, "a/a.go:3") {
			t.Errorf("gate() passed = %v, findings = %q; want a failure naming a/a.go:3", passed, findings)
		}
		if logs := verifyLogs(t, f.verifyDir); len(logs) != len(logsBefore) {
			t.Errorf("verify logs = %q; want %q, the lint to fail before any command ran", logs, logsBefore)
		}

		got, err := f.diffGate(toldCommand, baseOf(firstPass.Commit), notScoped)
		if err != nil || got.Passed || !strings.Contains(got.Findings, "a/a.go:3") {
			t.Errorf("darn gate() = %+v, %v; want a failure naming a/a.go:3 from the merge base", got, err)
		}
	}) {
		return
	}

	f.commitFiles(t, map[string]string{"a/a.go": "package a\n\n// Wrapped comment now breaks at the end of a sentence.\n// Each sentence has its own line.\nfunc Wrapped() {}\n"})
	overview := func(size int) string { return strings.Repeat("o", size-1) + "\n" }

	if !t.Run("round form told the merge base fails a branch that grew the overview past the line before any command runs", func(t *testing.T) {
		parent := head(t)
		f.commitFiles(t, map[string]string{"PATTERN.md": overview(pattern.HeadroomBytes + 100), "b/b.go": "package b\n\nconst Changed = 4\n"})
		logsBefore := verifyLogs(t, f.verifyDir)

		passed, findings := f.runGate(t, baseOf(parent))
		if passed {
			t.Fatal("gate() passed; want a failure for the grown overview")
		}
		for _, want := range []string{"cut 100 bytes", patternlint.WayForward, "re-step the row"} {
			if !strings.Contains(findings, want) {
				t.Errorf("Findings = %q; want it to contain %q", findings, want)
			}
		}
		if logs := verifyLogs(t, f.verifyDir); len(logs) != len(logsBefore) {
			t.Errorf("verify logs = %q; want %q, no command to have run", logs, logsBefore)
		}
		darn, err := f.diffGate(toldCommand, baseOf(parent), notScoped)
		if err != nil || darn.Passed || !strings.Contains(darn.Findings, "cut 100 bytes") {
			t.Errorf("darn gate() = %+v, %v; want a failure naming the grown overview", darn, err)
		}
		if logs := verifyLogs(t, f.verifyDir); len(logs) != len(logsBefore) {
			t.Errorf("verify logs = %q; want %q, the darn form to run no command either", logs, logsBefore)
		}

		f.commitFiles(t, map[string]string{"PATTERN.md": overview(pattern.HeadroomBytes)})
		if passed, findings := f.runGate(t, baseOf(parent)); !passed {
			t.Fatalf("gate() findings = %q; want a pass once the growth is cut", findings)
		}
		if pass, ok := verifytree.LatestPass(paths, f.command); !ok || pass.Commit != head(t) {
			t.Errorf("LatestPass(told command) = %+v, %v; want a pass at %s", pass, ok, head(t))
		}
	}) {
		return
	}

	if !t.Run("round form passes a branch that did not grow the overview, and one that never grew a merged-in overview above the line", func(t *testing.T) {
		f.commitFiles(t, map[string]string{"PATTERN.md": overview(pattern.HeadroomBytes + 500)})
		parent := head(t)
		f.commitFiles(t, map[string]string{"b/b.go": "package b\n\nconst Changed = 5\n"})
		if passed, findings := f.runGate(t, baseOf(parent)); !passed {
			t.Fatalf("gate() findings = %q; want a pass for a branch that left the overview alone", findings)
		}

		f.commitFiles(t, map[string]string{"PATTERN.md": overview(pattern.HeadroomBytes + 530)})
		passed, findings := f.runGate(t, baseOf(parent))
		if passed || !strings.Contains(findings, "cut 30 bytes") {
			t.Errorf("gate() passed = %v, findings = %q; want a failure naming only the branch's own 30 bytes", passed, findings)
		}
		if passed, findings := f.runGate(t, nil); !passed {
			t.Errorf("gate() told no reader: findings = %q; want a pass, the headroom lint skipped", findings)
		}
	}) {
		return
	}

	t.Run("refuses a dirty tree", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(f.worktree, "stray.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if passed, findings := f.runGate(t, nil); passed || !strings.Contains(findings, "stray.txt") {
			t.Errorf("gate() passed = %v, findings = %q; want a failure naming stray.txt", passed, findings)
		}
	})
}
