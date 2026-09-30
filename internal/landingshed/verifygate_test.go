// verifygate_test.go covers verifyGate.check branch by branch against a fake runner,
// so no test spawns a shell.

package landingshed

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeVerifyRun records every call the gate makes to its runner seam and returns a scripted result.
type fakeVerifyRun struct {
	calls   int
	command string
	dir     string
	out     io.Writer
	code    int
	err     error
	onRun   func()
}

func (f *fakeVerifyRun) run(ctx context.Context, command, dir string, out io.Writer) (int, error) {
	f.calls++
	f.command = command
	f.dir = dir
	f.out = out
	if f.onRun != nil {
		f.onRun()
	}
	return f.code, f.err
}

// gateFixture builds a gate over t.TempDir() paths with a fake runner and a counting command closure.
type gateFixture struct {
	gate         verifyGate
	fake         *fakeVerifyRun
	closureCalls int
	marker       string
	output       string
}

func newGateFixture(t *testing.T, command string, closureErr error) *gateFixture {
	t.Helper()
	root := t.TempDir()
	f := &gateFixture{
		fake:   &fakeVerifyRun{},
		marker: filepath.Join(root, "scratch", "verify-pending"),
		output: filepath.Join(root, "scratch", "verify-output.log"),
	}
	f.gate = verifyGate{
		command: func() (string, error) {
			f.closureCalls++
			return command, closureErr
		},
		pendingPath: f.marker,
		outputPath:  f.output,
		dir:         root,
		run:         f.fake.run,
	}
	return f
}

func (f *gateFixture) markerExists(t *testing.T) bool {
	t.Helper()
	_, err := os.Stat(f.marker)
	if err == nil {
		return true
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat marker: %v", err)
	}
	return false
}

func (f *gateFixture) seedMarker(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.marker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyGate_TreeChangedPass(t *testing.T) {
	f := newGateFixture(t, "go test ./...", nil)
	reason, err := f.gate.check(context.Background(), "Publish", "main", true)
	if reason != "" || err != nil {
		t.Fatalf("got (%q, %v), want (\"\", nil)", reason, err)
	}
	if f.fake.calls != 1 || f.fake.command != "go test ./..." || f.fake.dir != f.gate.dir {
		t.Fatalf("runner got calls=%d command=%q dir=%q", f.fake.calls, f.fake.command, f.fake.dir)
	}
	if file, ok := f.fake.out.(*os.File); !ok || file.Name() != f.output {
		t.Fatalf("runner got writer %#v; want the output file %q", f.fake.out, f.output)
	}
	if f.markerExists(t) {
		t.Fatal("marker still present after a pass")
	}
}

func TestVerifyGate_TreeChangedFail(t *testing.T) {
	f := newGateFixture(t, "false", nil)
	f.fake.code = 3
	reason, err := f.gate.check(context.Background(), "Publish", "main", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `verify failed after merging parent branch "main" (exit code 3); output: ` + f.output +
		`; fix forward on the task branch, then resume`
	if reason != want {
		t.Fatalf("reason = %q, want %q", reason, want)
	}
	if !f.markerExists(t) {
		t.Fatal("marker missing after a failure")
	}
}

func TestVerifyGate_NoChangeNoMarker(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	reason, err := f.gate.check(context.Background(), "Finalize", "main", false)
	if reason != "" || err != nil {
		t.Fatalf("got (%q, %v), want (\"\", nil)", reason, err)
	}
	if f.fake.calls != 0 || f.closureCalls != 0 {
		t.Fatalf("runner calls=%d closure calls=%d, want 0 and 0", f.fake.calls, f.closureCalls)
	}
}

func TestVerifyGate_NoChangeMarkerPresentPass(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	f.seedMarker(t)
	reason, err := f.gate.check(context.Background(), "Finalize", "main", false)
	if reason != "" || err != nil {
		t.Fatalf("got (%q, %v), want (\"\", nil)", reason, err)
	}
	if f.fake.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", f.fake.calls)
	}
	if f.markerExists(t) {
		t.Fatal("marker still present after a pass")
	}
}

func TestVerifyGate_NoChangeMarkerPresentFail(t *testing.T) {
	f := newGateFixture(t, "false", nil)
	f.seedMarker(t)
	f.fake.code = 1
	reason, err := f.gate.check(context.Background(), "Finalize", "main", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reason, "exit code 1") {
		t.Fatalf("reason = %q, want it to name exit code 1", reason)
	}
	if f.fake.calls != 1 || !f.markerExists(t) {
		t.Fatalf("runner calls=%d marker=%v, want 1 and true", f.fake.calls, f.markerExists(t))
	}
}

func TestVerifyGate_EmptyCommand(t *testing.T) {
	for _, withMarker := range []bool{false, true} {
		buf := captureLogOutput(t)
		f := newGateFixture(t, "", nil)
		if withMarker {
			f.seedMarker(t)
		}
		reason, err := f.gate.check(context.Background(), "Publish", "main", true)
		if reason != "" || err != nil {
			t.Fatalf("withMarker=%v: got (%q, %v), want (\"\", nil)", withMarker, reason, err)
		}
		if f.fake.calls != 0 {
			t.Fatalf("withMarker=%v: runner called", withMarker)
		}
		logged := buf.String()
		if !strings.Contains(logged, "WARN") || !strings.Contains(logged, "no verify command is configured") ||
			!strings.Contains(logged, "Publish") || !strings.Contains(logged, "main") {
			t.Fatalf("withMarker=%v: log = %q", withMarker, logged)
		}
		if got := strings.Contains(logged, f.marker); got != withMarker {
			t.Fatalf("withMarker=%v: marker path in log = %v", withMarker, got)
		}
		if f.markerExists(t) {
			t.Fatalf("withMarker=%v: marker still present after an empty-command skip", withMarker)
		}
	}
}

func TestVerifyGate_EmptyCommandNoChangeMarkerPresent(t *testing.T) {
	captureLogOutput(t)
	f := newGateFixture(t, "", nil)
	f.seedMarker(t)
	reason, err := f.gate.check(context.Background(), "Finalize", "main", false)
	if reason != "" || err != nil {
		t.Fatalf("got (%q, %v), want (\"\", nil)", reason, err)
	}
	if f.fake.calls != 0 {
		t.Fatal("runner called")
	}
	if f.markerExists(t) {
		t.Fatal("marker still present after an empty-command skip")
	}
}

func TestVerifyGate_NilCommand(t *testing.T) {
	buf := captureLogOutput(t)
	f := newGateFixture(t, "true", nil)
	f.gate.command = nil
	reason, err := f.gate.check(context.Background(), "Publish", "main", true)
	if reason != "" || err != nil {
		t.Fatalf("got (%q, %v), want (\"\", nil)", reason, err)
	}
	if f.fake.calls != 0 {
		t.Fatal("runner called")
	}
	if buf.Len() != 0 {
		t.Fatalf("unexpected log output: %q", buf.String())
	}
}

func TestVerifyGate_ClosureError(t *testing.T) {
	f := newGateFixture(t, "", errors.New("plan unreadable"))
	reason, err := f.gate.check(context.Background(), "Publish", "main", true)
	if reason != "" || err == nil || !strings.Contains(err.Error(), "plan unreadable") || !strings.Contains(err.Error(), "Publish") {
		t.Fatalf("got (%q, %v)", reason, err)
	}
	if f.fake.calls != 0 {
		t.Fatal("runner called")
	}
	if !f.markerExists(t) {
		t.Fatal("marker missing after a command read error on a changed tree")
	}
}

func TestVerifyGate_ClosureErrorThenResumeRunsVerify(t *testing.T) {
	f := newGateFixture(t, "", errors.New("plan unreadable"))
	if _, err := f.gate.check(context.Background(), "Publish", "main", true); err == nil {
		t.Fatal("first check: want a command read error")
	}
	f.gate.command = func() (string, error) { return "go test ./...", nil }
	reason, err := f.gate.check(context.Background(), "Publish", "main", false)
	if reason != "" || err != nil {
		t.Fatalf("resume check: got (%q, %v), want (\"\", nil)", reason, err)
	}
	if f.fake.calls != 1 || f.fake.command != "go test ./..." {
		t.Fatalf("resume check: runner calls=%d command=%q, want 1 and %q", f.fake.calls, f.fake.command, "go test ./...")
	}
	if f.markerExists(t) {
		t.Fatal("marker still present after the resumed verify passed")
	}
}

func TestVerifyGate_MarkerWriteFailure(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f.gate.pendingPath = filepath.Join(blocker, "sub", "verify-pending")
	reason, err := f.gate.check(context.Background(), "Publish", "main", true)
	if reason != "" || err == nil {
		t.Fatalf("got (%q, %v), want an error", reason, err)
	}
	if f.fake.calls != 0 || f.closureCalls != 0 {
		t.Fatalf("runner calls=%d closure calls=%d, want 0 and 0", f.fake.calls, f.closureCalls)
	}
}

func TestVerifyGate_OutputCreateFailure(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f.gate.outputPath = filepath.Join(blocker, "sub", "out.log")
	reason, err := f.gate.check(context.Background(), "Publish", "main", true)
	if reason != "" || err == nil {
		t.Fatalf("got (%q, %v), want an error", reason, err)
	}
	if f.fake.calls != 0 {
		t.Fatal("runner called")
	}
}

func TestVerifyGate_SpawnFailure(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	f.fake.code = -1
	f.fake.err = errors.New("exec: sh not found")
	reason, err := f.gate.check(context.Background(), "Publish", "main", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reason, "exec: sh not found") || !strings.Contains(reason, `"main"`) || !strings.Contains(reason, f.output) {
		t.Fatalf("reason = %q", reason)
	}
	if !f.markerExists(t) {
		t.Fatal("marker missing after a spawn failure")
	}
}

func TestVerifyGate_Cancellation(t *testing.T) {
	f := newGateFixture(t, "true", nil)
	ctx, cancel := context.WithCancel(context.Background())
	f.fake.onRun = cancel
	f.fake.code = -1
	f.fake.err = errors.New("killed")
	reason, err := f.gate.check(ctx, "Publish", "main", true)
	if reason != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("got (%q, %v), want a context.Canceled error", reason, err)
	}
	if !f.markerExists(t) {
		t.Fatal("marker missing after a cancellation")
	}
}
