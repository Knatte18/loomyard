// runner_test.go drives the runner against the test binary re-exec'd as a stand-in `go` and a stand-in coverage binary,
// so no test builds a real test binary or runs `go test`.

package testcover_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// standIn returns a toolchain whose go command is the test binary acting as a stand-in.
// It also returns the file the stand-in records its arguments into.
// At cleanup it kills any child the stand-in left running, so no child outlives the package's run.
func standIn(t *testing.T) (testcover.Toolchain, string) {
	t.Helper()
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(t.TempDir(), "args")
	t.Cleanup(func() {
		recorded, err := os.ReadFile(record + standInChildPIDSuffix)
		if err != nil {
			return
		}
		pid, err := strconv.Atoi(string(recorded))
		if err != nil {
			t.Errorf("stand-in child pid file holds %q: %v", recorded, err)
			return
		}
		_ = proc.KillPID(pid)
	})
	return testcover.Toolchain{Go: testBinary, Env: append(os.Environ(), standInRecordEnv+"="+record)}, record
}

// steppingClock returns a clock that advances by step on each call.
func steppingClock(step time.Duration) func() time.Time {
	start := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	var calls atomic.Int64
	return func() time.Time { return start.Add(time.Duration(calls.Add(1)) * step) }
}

func recordedArgs(t *testing.T, record string) []string {
	t.Helper()
	recorded, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(recorded), "\n")
}

func TestBuildBinaryAndListTests(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tc, record := standIn(t)
	out := filepath.Join(t.TempDir(), "pkg.test")
	if err := tc.BuildBinary("m/p", "m/p,m/q", "tier0,tier1", out); err != nil {
		t.Fatal(err)
	}
	wantBuild := []string{"test", "-c", "-o", out, "-cover", "-covermode=set", "-coverpkg=m/p,m/q", "-tags", "tier0,tier1", "m/p"}
	if got := recordedArgs(t, record); !reflect.DeepEqual(got, wantBuild) {
		t.Errorf("BuildBinary ran go %q; want %q", got, wantBuild)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("BuildBinary left no binary at %s: %v", out, err)
	}

	names, err := tc.ListTests("m/p", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"TestAlpha", "ExampleBeta"}; !reflect.DeepEqual(names, want) {
		t.Errorf("ListTests = %q; want %q, without the package's ok line", names, want)
	}
	if got, want := recordedArgs(t, record), []string{"test", "-list", ".*", "m/p"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListTests ran go %q; want %q, with no -tags for empty tags", got, want)
	}
}

func TestRunAlone(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		test string
		// timeout is the run's TestTimeout; zero leaves it unbounded.
		timeout time.Duration
		// within, when above zero, is the longest the run may take.
		within time.Duration
		// memory is the run's MemoryLimit; zero leaves it uncapped.
		memory int64
		// noProfile runs the test with no profile flag, as a binary built without coverage.
		noProfile bool
		// now is the run's Now; nil sets none.
		now     func() time.Time
		want    testcover.RunResult
		wantErr string
	}{
		{
			// The stand-in prints both lines in one write and reports 0.00s, so only stamping each line separates them.
			name: "ElapsedRunsFromTheRunLineToTheResultLine", test: standInTimed, now: steppingClock(1500 * time.Microsecond),
			want: testcover.RunResult{Name: standInTimed, Elapsed: 0.0015, Action: testcover.ActionPass, Profile: []testcover.ProfileBlock{
				{File: "m/p/x.go", StartLine: 3, StartCol: 21, EndLine: 5, EndCol: 2, Statements: 1, Count: 1},
			}},
		},
		{
			// The stand-in splits its result line across two writes and prints an indented subtest result line before it.
			// Only the RUN line and the completed top-level result line are stamped, so the clock steps twice.
			name: "ElapsedStampsOnlyTheRunLineAndTheCompletedTopLevelResultLine", test: standInSplit, now: steppingClock(1500 * time.Microsecond),
			want: testcover.RunResult{Name: standInSplit, Elapsed: 0.0015, Action: testcover.ActionPass, Profile: []testcover.ProfileBlock{
				{File: "m/p/x.go", StartLine: 3, StartCol: 21, EndLine: 5, EndCol: 2, Statements: 1, Count: 1},
			}},
		},
		{
			name: "PassWithNoProfileFlag", test: standInPass, noProfile: true,
			want: testcover.RunResult{Name: standInPass, Elapsed: 0.25, Action: testcover.ActionPass},
		},
		{
			name: "Pass",
			test: standInPass,
			want: testcover.RunResult{Name: standInPass, Elapsed: 0.25, Action: testcover.ActionPass, Profile: []testcover.ProfileBlock{
				{File: "m/p/x.go", StartLine: 3, StartCol: 21, EndLine: 5, EndCol: 2, Statements: 1, Count: 1},
			}},
		},
		{name: "Fail", test: standInFail, want: testcover.RunResult{Name: standInFail, Elapsed: 0.1, Action: testcover.ActionFail}},
		{name: "SkipWithoutProfile", test: standInSkip, want: testcover.RunResult{Name: standInSkip, Action: testcover.ActionSkip}},
		{
			name: "SkipWithProfile",
			test: standInSkipProfile,
			want: testcover.RunResult{Name: standInSkipProfile, Action: testcover.ActionSkip, Profile: []testcover.ProfileBlock{
				{File: "m/p/x.go", StartLine: 3, StartCol: 21, EndLine: 5, EndCol: 2, Statements: 1, Count: 1},
			}},
		},
		{name: "PassWithoutProfile", test: standInNoProfile, wantErr: "wrote no coverage profile"},
		{
			name: "PassWithinTheTimeout", test: standInPass, timeout: time.Minute,
			want: testcover.RunResult{Name: standInPass, Elapsed: 0.25, Action: testcover.ActionPass, Profile: []testcover.ProfileBlock{
				{File: "m/p/x.go", StartLine: 3, StartCol: 21, EndLine: 5, EndCol: 2, Statements: 1, Count: 1},
			}},
		},
		{
			// The timeout leaves the stand-in time to start and record its arguments before the kill, even on a loaded machine.
			name: "TimedOut", test: standInHang, timeout: 3 * time.Second,
			want: testcover.RunResult{Name: standInHang, Action: testcover.ActionFail, TimedOut: true},
		},
		{
			name: "PassUnderTheMemoryLimit", test: standInPass, memory: 128 << 20,
			want: testcover.RunResult{Name: standInPass, Elapsed: 0.25, Action: testcover.ActionPass, Profile: []testcover.ProfileBlock{
				{File: "m/p/x.go", StartLine: 3, StartCol: 21, EndLine: 5, EndCol: 2, Statements: 1, Count: 1},
			}},
		},
		{
			// The stand-in passes by itself once it holds three times the limit, so a cap that does not hold fails the row.
			name: "MemoryCapped", test: standInHoards, memory: 128 << 20,
			want: testcover.RunResult{Name: standInHoards, Action: testcover.ActionFail, MemoryCapped: true},
		},
		{
			// The stand-in fails as a test does whose allocation the cap refused.
			// So the row pins the runtime-error branch on any host.
			name: "MemoryCappedByTheRuntimesOutOfMemoryLine", test: standInOutOfMemory, memory: 128 << 20,
			want: testcover.RunResult{Name: standInOutOfMemory, Action: testcover.ActionFail, MemoryCapped: true},
		},
		{
			// The test passes and exits, but a child it left holds the output pipe past the grace.
			// The child outlasts the minute the row allows, so a run that waits on it fails however loaded the machine.
			name: "ReturnsWithinTheGraceWhileADescendantHoldsTheOutput", test: standInHoldsPipe, timeout: time.Minute, within: time.Minute,
			want: testcover.RunResult{Name: standInHoldsPipe, Elapsed: 0.25, Action: testcover.ActionPass, Profile: []testcover.ProfileBlock{
				{File: "m/p/x.go", StartLine: 3, StartCol: 21, EndLine: 5, EndCol: 2, Statements: 1, Count: 1},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tc, record := standIn(t)
			tc.TestTimeout, tc.MemoryLimit, tc.Grace, tc.Now = tt.timeout, tt.memory, time.Second, tt.now
			profile := filepath.Join(t.TempDir(), "cover.out")
			if tt.noProfile {
				profile = ""
			}
			started := time.Now()
			got, err := tc.RunAlone(tc.Go, t.TempDir(), profile, tt.test)
			if elapsed := time.Since(started); tt.within > 0 && elapsed > tt.within {
				t.Errorf("RunAlone(%s) took %s; want at most %s, the grace bounding the wait for the output", tt.test, elapsed, tt.within)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("RunAlone(%s) error = %v; want one holding %q", tt.test, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RunAlone(%s) = %+v; want %+v", tt.test, got, tt.want)
			}
			wantArgs := []string{"-test.run=^" + tt.test + "$", "-test.coverprofile=" + profile, "-test.v=true"}
			if tt.noProfile {
				wantArgs = slices.Delete(wantArgs, 1, 2)
			}
			if args := recordedArgs(t, record); !reflect.DeepEqual(args, wantArgs) {
				t.Errorf("RunAlone ran the binary with %q; want %q, each flag one -flag=value argument", args, wantArgs)
			}
		})
	}
}
