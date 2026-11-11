// runner_test.go drives the runner against the test binary re-exec'd as a stand-in `go` and a stand-in coverage binary,
// so no test builds a real test binary or runs `go test`.

package testcover_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
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

func TestLinkedPackages(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tc, record := standIn(t)
	got, err := tc.LinkedPackages("m/p")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"m/p", "m/q"}; !reflect.DeepEqual(got, want) {
		t.Errorf("LinkedPackages(m/p) = %q; want %q, each once, without the external test package or the test main", got, want)
	}
	wantArgs := []string{"list", "-deps", "-test", "-f", `{{if and .Module .Module.Main}}{{.ImportPath}}{{"\t"}}{{.Name}}{{"\t"}}{{.ForTest}}{{end}}`, "m/p"}
	if args := recordedArgs(t, record); !reflect.DeepEqual(args, wantArgs) {
		t.Errorf("LinkedPackages ran go %q; want %q", args, wantArgs)
	}
}

func TestGoEnvironment(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tc, record := standIn(t)
	version, goos, goarch, err := tc.GoEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if version != "go1.26.8" || goos != "linux" || goarch != "amd64" {
		t.Errorf("GoEnvironment() = %q, %q, %q; want go1.26.8, linux, amd64", version, goos, goarch)
	}
	if args, want := recordedArgs(t, record), []string{"env", "GOVERSION", "GOOS", "GOARCH"}; !reflect.DeepEqual(args, want) {
		t.Errorf("GoEnvironment ran go %q; want %q", args, want)
	}
}

func TestBuildBinaryTimesOut(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tc, _ := standIn(t)
	tc.Env = append(tc.Env, standInHangBuildEnv+"=1")
	tc.BuildTimeout, tc.Grace = 2*time.Second, time.Second
	err := tc.BuildBinary("m/p", "m/p", "", filepath.Join(t.TempDir(), "pkg.test"))
	if err == nil || !strings.Contains(err.Error(), "timed out after 2s") {
		t.Errorf("BuildBinary error = %v; want one saying the build timed out after 2s", err)
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
		within  time.Duration
		want    testcover.RunResult
		wantErr string
	}{
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
			tc.TestTimeout, tc.Grace = tt.timeout, time.Second
			profile := filepath.Join(t.TempDir(), "cover.out")
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
			if args := recordedArgs(t, record); !reflect.DeepEqual(args, wantArgs) {
				t.Errorf("RunAlone ran the binary with %q; want %q, each flag one -flag=value argument", args, wantArgs)
			}
		})
	}
}
