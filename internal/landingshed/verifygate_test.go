// verifygate_test.go covers verifyGate.clean and verifyGate.check branch by branch against fake verifytree seams,
// so no test spawns git or a shell.

package landingshed

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// fakeVerifyTree scripts both verifytree seams and records every call the gate makes through them.
type fakeVerifyTree struct {
	dirtyCalls  int
	dirtyScript [][]string
	dirtyErr    error

	verifyCalls int
	paths       verifytree.Paths
	site        verifytree.Site
	command     string
	result      verifytree.Result
	verifyErr   error
	onVerify    func()
}

// dirtyAt scripts the dirty paths the i-th (zero-based) clean-tree check reports; every other check reports a clean tree.
func (f *fakeVerifyTree) dirtyAt(i int, paths ...string) {
	for len(f.dirtyScript) <= i {
		f.dirtyScript = append(f.dirtyScript, nil)
	}
	f.dirtyScript[i] = paths
}

func (f *fakeVerifyTree) dirty(string) ([]string, error) {
	i := f.dirtyCalls
	f.dirtyCalls++
	if f.dirtyErr != nil {
		return nil, f.dirtyErr
	}
	if i < len(f.dirtyScript) {
		return f.dirtyScript[i], nil
	}
	return nil, nil
}

func (f *fakeVerifyTree) verify(ctx context.Context, p verifytree.Paths, site verifytree.Site, command string) (verifytree.Result, error) {
	f.verifyCalls++
	f.paths = p
	f.site = site
	f.command = command
	if f.onVerify != nil {
		f.onVerify()
	}
	return f.result, f.verifyErr
}

// gateFixture builds a gate over t.TempDir() paths with fake verifytree seams and a counting command closure.
type gateFixture struct {
	gate         verifyGate
	fake         *fakeVerifyTree
	closureCalls int
	paths        verifytree.Paths
}

func newGateFixture(t *testing.T, command string, closureErr error) *gateFixture {
	t.Helper()
	root := t.TempDir()
	f := &gateFixture{
		fake:  &fakeVerifyTree{result: verifytree.Result{Status: verifytree.StatusPassed}},
		paths: verifytree.NewPaths(root, filepath.Join(root, "verify")),
	}
	f.gate = verifyGate{
		command: func() (string, error) {
			f.closureCalls++
			return command, closureErr
		},
		paths:  f.paths,
		dirty:  f.fake.dirty,
		verify: f.fake.verify,
	}
	return f
}

func TestVerifyGate_CheckPass(t *testing.T) {
	f := newGateFixture(t, "go test ./...", nil)
	reason, err := f.gate.check(context.Background(), "Publish", "main")
	if reason != "" || err != nil {
		t.Fatalf("got (%q, %v), want (\"\", nil)", reason, err)
	}
	if f.fake.verifyCalls != 1 || f.fake.command != "go test ./..." || f.fake.paths != f.paths || f.fake.site != (verifytree.Site{Label: "Publish"}) {
		t.Fatalf("verify got calls=%d command=%q paths=%+v site=%+v", f.fake.verifyCalls, f.fake.command, f.fake.paths, f.fake.site)
	}
}

func TestVerifyGate_CheckSiteLabelFollowsProducer(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	if _, err := f.gate.check(context.Background(), "Finalize", "main"); err != nil {
		t.Fatal(err)
	}
	if f.fake.site.Label != "Finalize" {
		t.Fatalf("site label = %q, want Finalize", f.fake.site.Label)
	}
}

func TestVerifyGate_CheckSkippedIsProceed(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	f.fake.result = verifytree.Result{Status: verifytree.StatusSkipped, Tree: "abc"}
	reason, err := f.gate.check(context.Background(), "Finalize", "main")
	if reason != "" || err != nil {
		t.Fatalf("got (%q, %v), want (\"\", nil)", reason, err)
	}
}

func TestVerifyGate_CheckFailed(t *testing.T) {
	f := newGateFixture(t, "false", nil)
	f.fake.result = verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 3}
	reason, err := f.gate.check(context.Background(), "Publish", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `verify failed after merging parent branch "main" (exit code 3); output: ` + f.paths.Log +
		`; fix forward on the task branch, then resume`
	if reason != want {
		t.Fatalf("reason = %q, want %q", reason, want)
	}
}

func TestVerifyGate_CheckCouldNotStart(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	f.fake.result = verifytree.Result{Status: verifytree.StatusFailed, ExitCode: -1, Detail: "exec: sh not found"}
	reason, err := f.gate.check(context.Background(), "Publish", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reason, "could not start") || !strings.Contains(reason, "exec: sh not found") || !strings.Contains(reason, `"main"`) || !strings.Contains(reason, f.paths.Log) {
		t.Fatalf("reason = %q", reason)
	}
}

func TestVerifyGate_CheckDirtyResult(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	f.fake.result = verifytree.Result{Status: verifytree.StatusDirty, Dirty: []string{"a.txt", "b.txt"}}
	reason, err := f.gate.check(context.Background(), "Publish", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reason, "a.txt") || !strings.Contains(reason, "b.txt") {
		t.Fatalf("reason = %q, want it to name the dirty paths", reason)
	}
}

func TestVerifyGate_CheckUnknownStatus(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	f.fake.result = verifytree.Result{Status: "bogus"}
	reason, err := f.gate.check(context.Background(), "Publish", "main")
	if reason != "" || err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("got (%q, %v), want an error naming the status", reason, err)
	}
}

func TestVerifyGate_EmptyCommand(t *testing.T) {
	buf := logcapture.Capture(t)
	f := newGateFixture(t, "", nil)
	reason, err := f.gate.check(context.Background(), "Publish", "main")
	if reason != "" || err != nil {
		t.Fatalf("got (%q, %v), want (\"\", nil)", reason, err)
	}
	if f.fake.verifyCalls != 0 {
		t.Fatal("verify called")
	}
	logged := buf.String()
	if !strings.Contains(logged, "WARN") || !strings.Contains(logged, "no verify command is configured") ||
		!strings.Contains(logged, "Publish") || !strings.Contains(logged, "main") {
		t.Fatalf("log = %q", logged)
	}
}

func TestVerifyGate_NilCommand(t *testing.T) {
	buf := logcapture.Capture(t)
	f := newGateFixture(t, "true", nil)
	f.gate.command = nil
	reason, err := f.gate.check(context.Background(), "Publish", "main")
	if reason != "" || err != nil {
		t.Fatalf("got (%q, %v), want (\"\", nil)", reason, err)
	}
	if f.fake.verifyCalls != 0 {
		t.Fatal("verify called")
	}
	if buf.String() != "" {
		t.Fatalf("unexpected log output: %q", buf.String())
	}
}

func TestVerifyGate_ClosureError(t *testing.T) {
	f := newGateFixture(t, "", errors.New("plan unreadable"))
	reason, err := f.gate.check(context.Background(), "Publish", "main")
	if reason != "" || err == nil || !strings.Contains(err.Error(), "plan unreadable") || !strings.Contains(err.Error(), "Publish") {
		t.Fatalf("got (%q, %v)", reason, err)
	}
	if f.fake.verifyCalls != 0 {
		t.Fatal("verify called")
	}
}

func TestVerifyGate_Cancellation(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	ctx, cancel := context.WithCancel(context.Background())
	f.fake.onVerify = cancel
	f.fake.verifyErr = context.Canceled
	reason, err := f.gate.check(ctx, "Publish", "main")
	if reason != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("got (%q, %v), want a context.Canceled error", reason, err)
	}
}

func TestVerifyGate_CleanTree(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	reason, err := f.gate.clean("Publish", "before the merge-in")
	if reason != "" || err != nil {
		t.Fatalf("got (%q, %v), want (\"\", nil)", reason, err)
	}
}

func TestVerifyGate_CleanDirtyNamesPointAndPaths(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	f.fake.dirtyAt(0, "x.go", "y.go")
	reason, err := f.gate.clean("Publish", "after the merge-in")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"after the merge-in", "x.go", "y.go", "commit or remove"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason %q lacks %q", reason, want)
		}
	}
}

func TestVerifyGate_CleanCapsTheNamedPaths(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	var paths []string
	for i := 0; i < dirtyPathsShown+3; i++ {
		paths = append(paths, filepath.Join("dir", string(rune('a'+i))+".txt"))
	}
	f.fake.dirtyAt(0, paths...)
	reason, err := f.gate.clean("Finalize", "after the verify")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reason, "and 3 more") || strings.Contains(reason, paths[len(paths)-1]) {
		t.Fatalf("reason = %q, want the list capped with \"and 3 more\"", reason)
	}
}

func TestVerifyGate_CleanStatusError(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	f.fake.dirtyErr = errors.New("git exploded")
	reason, err := f.gate.clean("Publish", "before the merge-in")
	if reason != "" || err == nil || !strings.Contains(err.Error(), "git exploded") || !strings.Contains(err.Error(), "Publish") {
		t.Fatalf("got (%q, %v), want an error", reason, err)
	}
}

func TestVerifyGate_CleanRunsWithoutCommand(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	f.gate.command = nil
	f.fake.dirtyAt(0, "x.go")
	reason, err := f.gate.clean("Publish", "before the merge-in")
	if err != nil || reason == "" {
		t.Fatalf("got (%q, %v), want a dirty reason", reason, err)
	}
}

func TestVerifyGate_ZeroValueSkipsCleanCheck(t *testing.T) {
	reason, err := verifyGate{}.clean("Publish", "before the merge-in")
	if reason != "" || err != nil {
		t.Fatalf("got (%q, %v), want (\"\", nil)", reason, err)
	}
}
