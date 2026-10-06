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

// dirtyAt scripts the dirty paths the i-th (zero-based) clean-tree check reports.
// Every other check reports a clean tree.
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

// TestVerifyGate_CheckMapsVerifyResult pins how each verifytree status turns into the gate's verdict, and that the verify seam receives the configured command, the gate's paths and the calling producer's site label.
func TestVerifyGate_CheckMapsVerifyResult(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		command string
		site    string
		result  verifytree.Result
		// wantReasonExact, when set, is the whole reason for the log path the fixture gave the gate.
		wantReasonExact func(logPath string) string
		wantReasonHas   []string
		wantErrHas      string
	}{
		{name: "pass", command: "go test ./...", site: "Publish", result: verifytree.Result{Status: verifytree.StatusPassed}},
		{name: "site label follows the producer", command: "true", site: "Finalize", result: verifytree.Result{Status: verifytree.StatusPassed}},
		{name: "skipped proceeds", command: "true", site: "Finalize", result: verifytree.Result{Status: verifytree.StatusSkipped, Tree: "abc"}},
		{
			name: "failed", command: "false", site: "Publish",
			result: verifytree.Result{Status: verifytree.StatusFailed, ExitCode: 3},
			wantReasonExact: func(logPath string) string {
				return `verify failed after merging parent branch "main" (exit code 3); output: ` + logPath +
					`; fix forward on the task branch, then resume`
			},
		},
		{
			name: "could not start", command: "true", site: "Publish",
			result:        verifytree.Result{Status: verifytree.StatusFailed, ExitCode: -1, Detail: "exec: sh not found"},
			wantReasonHas: []string{"could not start", "exec: sh not found", `"main"`},
		},
		{
			name: "dirty result names the paths", command: "true", site: "Publish",
			result:        verifytree.Result{Status: verifytree.StatusDirty, Dirty: []string{"a.txt", "b.txt"}},
			wantReasonHas: []string{"a.txt", "b.txt"},
		},
		{name: "unknown status is an error", command: "true", site: "Publish", result: verifytree.Result{Status: "bogus"}, wantErrHas: "bogus"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newGateFixture(t, tc.command, nil)
			f.fake.result = tc.result
			reason, err := f.gate.check(context.Background(), tc.site, "main")

			if f.fake.verifyCalls != 1 || f.fake.command != tc.command || f.fake.paths != f.paths || f.fake.site != (verifytree.Site{Label: tc.site}) {
				t.Errorf("verify got calls=%d command=%q paths=%+v site=%+v", f.fake.verifyCalls, f.fake.command, f.fake.paths, f.fake.site)
			}
			if tc.wantErrHas != "" {
				if reason != "" || err == nil || !strings.Contains(err.Error(), tc.wantErrHas) {
					t.Fatalf("got (%q, %v), want an error naming %q", reason, err, tc.wantErrHas)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantReasonExact != nil {
				if want := tc.wantReasonExact(f.paths.Log); reason != want {
					t.Fatalf("reason = %q, want %q", reason, want)
				}
				return
			}
			if len(tc.wantReasonHas) == 0 && reason != "" {
				t.Fatalf("reason = %q, want none", reason)
			}
			for _, want := range tc.wantReasonHas {
				if !strings.Contains(reason, want) {
					t.Errorf("reason %q lacks %q", reason, want)
				}
			}
			if tc.result.Status == verifytree.StatusFailed && tc.wantReasonHas != nil && !strings.Contains(reason, f.paths.Log) {
				t.Errorf("reason %q lacks the log path %q", reason, f.paths.Log)
			}
		})
	}
}

// TestVerifyGate_CheckWithoutCommandSkipsVerify pins that a gate with no command to run proceeds without calling verify, warning once when the command closure answers empty and staying silent when no closure is wired.
//
// It stays serial because logcapture swaps the process-global logger.
func TestVerifyGate_CheckWithoutCommandSkipsVerify(t *testing.T) {
	cases := []struct {
		name    string
		command string
		noGate  bool
		wantLog bool
	}{
		{"empty command warns", "", false, true},
		{"nil command closure is silent", "true", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := logcapture.Capture(t)
			f := newGateFixture(t, tc.command, nil)
			if tc.noGate {
				f.gate.command = nil
			}
			reason, err := f.gate.check(context.Background(), "Publish", "main")
			if reason != "" || err != nil {
				t.Fatalf("got (%q, %v), want (\"\", nil)", reason, err)
			}
			if f.fake.verifyCalls != 0 {
				t.Fatal("verify called")
			}
			logged := buf.String()
			if !tc.wantLog {
				if logged != "" {
					t.Fatalf("unexpected log output: %q", logged)
				}
				return
			}
			if !strings.Contains(logged, "WARN") || !strings.Contains(logged, "no verify command is configured") ||
				!strings.Contains(logged, "Publish") || !strings.Contains(logged, "main") {
				t.Fatalf("log = %q", logged)
			}
		})
	}
}

func TestVerifyGate_ClosureError(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	f := newGateFixture(t, "true", nil)
	ctx, cancel := context.WithCancel(context.Background())
	f.fake.onVerify = cancel
	f.fake.verifyErr = context.Canceled
	reason, err := f.gate.check(ctx, "Publish", "main")
	if reason != "" || !errors.Is(err, context.Canceled) {
		t.Fatalf("got (%q, %v), want a context.Canceled error", reason, err)
	}
}

// TestVerifyGate_Clean pins the clean-tree check: a clean tree gives no reason, a dirty one names the point and the paths (capped), a status failure is an error naming the producer, and the check runs with no verify command and with a zero-value gate.
func TestVerifyGate_Clean(t *testing.T) {
	t.Parallel()

	manyPaths := make([]string, dirtyPathsShown+3)
	for i := range manyPaths {
		manyPaths[i] = filepath.Join("dir", string(rune('a'+i))+".txt")
	}
	cases := []struct {
		name       string
		producer   string
		point      string
		setup      func(f *gateFixture)
		zeroGate   bool
		wantReason []string
		wantNoPath string
		wantErrHas []string
	}{
		{name: "clean tree", producer: "Publish", point: "before the merge-in", setup: func(*gateFixture) {}},
		{
			name: "dirty names the point and paths", producer: "Publish", point: "after the merge-in",
			setup:      func(f *gateFixture) { f.fake.dirtyAt(0, "x.go", "y.go") },
			wantReason: []string{"after the merge-in", "x.go", "y.go", "commit or remove"},
		},
		{
			name: "dirty paths are capped", producer: "Finalize", point: "after the verify",
			setup:      func(f *gateFixture) { f.fake.dirtyAt(0, manyPaths...) },
			wantReason: []string{"and 3 more"}, wantNoPath: manyPaths[len(manyPaths)-1],
		},
		{
			name: "status error names the producer", producer: "Publish", point: "before the merge-in",
			setup:      func(f *gateFixture) { f.fake.dirtyErr = errors.New("git exploded") },
			wantErrHas: []string{"git exploded", "Publish"},
		},
		{
			name: "runs without a verify command", producer: "Publish", point: "before the merge-in",
			setup:      func(f *gateFixture) { f.gate.command = nil; f.fake.dirtyAt(0, "x.go") },
			wantReason: []string{"x.go"},
		},
		{name: "zero-value gate skips the check", producer: "Publish", point: "before the merge-in", zeroGate: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := newGateFixture(t, "true", nil)
			if tc.setup != nil {
				tc.setup(f)
			}
			gate := f.gate
			if tc.zeroGate {
				gate = verifyGate{}
			}
			reason, err := gate.clean(tc.producer, tc.point)

			if len(tc.wantErrHas) > 0 {
				if reason != "" || err == nil {
					t.Fatalf("got (%q, %v), want an error", reason, err)
				}
				for _, want := range tc.wantErrHas {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q lacks %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(tc.wantReason) == 0 && reason != "" {
				t.Fatalf("reason = %q, want none", reason)
			}
			for _, want := range tc.wantReason {
				if !strings.Contains(reason, want) {
					t.Errorf("reason %q lacks %q", reason, want)
				}
			}
			if tc.wantNoPath != "" && strings.Contains(reason, tc.wantNoPath) {
				t.Errorf("reason %q names %q; want it cut by the cap", reason, tc.wantNoPath)
			}
		})
	}
}
