// darnclassify_test.go covers the darn failure classifier: which failed verifies the task owns and the route a not-owned one takes.

package loomcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

func TestDarnFailureRoute(t *testing.T) {
	t.Parallel()

	const (
		rootModule  = "example.com/m"
		toolsModule = "example.com/tools"
		outsideZ    = rootModule + "/internal/z"
		outsideY    = rootModule + "/internal/y"
		guardPkg    = rootModule + "/internal/g"
		driverPkg   = rootModule + "/internal/lyxdriver"
	)
	// failedTests renders go test output for pkg whose named tests failed.
	failedTests := func(pkg string, names ...string) string {
		var out strings.Builder
		for _, name := range names {
			out.WriteString("--- FAIL: " + name + " (0.00s)\n")
		}
		return out.String() + "FAIL\nFAIL\t" + pkg + "\t0.01s\n"
	}
	buildFailed := func(pkg string) string { return "FAIL\t" + pkg + " [build failed]\n" }
	zFailure := failedTests(outsideZ, "TestZ")
	zTest := verifytree.FailedTest{Package: outsideZ, Test: "TestZ"}

	toldTargets := impactset.Targets{
		Modules: []impactset.ModuleTargets{
			{Dir: ".", Packages: []string{"./internal/a", "./internal/bin"}},
			{Dir: "tools", Packages: []string{"./x"}},
		},
		Guards:      map[string][]string{"internal/g": {"TestGuard"}},
		BinaryTests: []string{"internal/lyxdriver"},
	}
	toldDerive := func(context.Context, string, string) (impactset.Targets, error) { return toldTargets, nil }
	owned := landingshed.FailureRoute{Target: "Darn-Write", WayForward: darnFailedWayForward}
	notOwned := func(lead, wayForward string) landingshed.FailureRoute {
		return landingshed.FailureRoute{Target: "Merge-In", Lead: lead, WayForward: wayForward}
	}

	tests := []struct {
		name string
		// log is the verify log's content.
		log   string
		tests []verifytree.FailedTest
		// derive replaces the told derivation.
		derive func(context.Context, string, string) (impactset.Targets, error)
		// mergeBaseErr makes the merge base reader fail.
		mergeBaseErr bool
		// noLog points the record at a log file that does not exist.
		noLog bool
		// noToolsModule removes the nested module's go.mod.
		noToolsModule bool
		want          landingshed.FailureRoute
	}{
		{
			name:  "a failing test in an impacted package, given by import path against a module-relative argument, is owned",
			log:   failedTests(rootModule+"/internal/a", "TestA") + zFailure,
			tests: []verifytree.FailedTest{{Package: rootModule + "/internal/a", Test: "TestA"}, zTest},
			want:  owned,
		},
		{
			name:  "a failing test in a nested module's package is owned",
			log:   failedTests(toolsModule+"/x", "TestX") + zFailure,
			tests: []verifytree.FailedTest{{Package: toolsModule + "/x", Test: "TestX"}, zTest},
			want:  owned,
		},
		{
			name:  "a failing guard test outside the set is owned",
			log:   failedTests(guardPkg, "TestGuard", "TestGuard/sub") + zFailure,
			tests: []verifytree.FailedTest{{Package: guardPkg, Test: "TestGuard/sub"}, zTest},
			want:  owned,
		},
		{
			name:  "a failing test of a guard package that is not the listed guard is not owned",
			log:   failedTests(guardPkg, "TestOther"),
			tests: []verifytree.FailedTest{{Package: guardPkg, Test: "TestOther"}},
			want:  notOwned(" outside the task's impacted set, in "+guardPkg+",", notOwnedWayForward),
		},
		{
			name:  "a Whole answer is owned",
			log:   zFailure,
			tests: []verifytree.FailedTest{zTest},
			derive: func(context.Context, string, string) (impactset.Targets, error) {
				return impactset.Targets{Whole: true}, nil
			},
			want: owned,
		},
		{
			name:  "a build failure in an impacted package is owned",
			log:   buildFailed(rootModule+"/internal/a") + zFailure,
			tests: []verifytree.FailedTest{zTest},
			want:  owned,
		},
		{
			name:  "an opaque identity is owned",
			log:   "go vet: something failed\n",
			tests: []verifytree.FailedTest{zTest},
			want:  owned,
		},
		{
			name: "a record with no failing tests is owned",
			log:  zFailure,
			want: owned,
		},
		{
			name:  "a derive error is owned",
			log:   zFailure,
			tests: []verifytree.FailedTest{zTest},
			derive: func(context.Context, string, string) (impactset.Targets, error) {
				return impactset.Targets{}, errors.New("go list failed")
			},
			want: owned,
		},
		{
			name:         "a merge base error is owned",
			log:          zFailure,
			tests:        []verifytree.FailedTest{zTest},
			mergeBaseErr: true,
			want:         owned,
		},
		{
			name:  "a log path that names no file is owned",
			tests: []verifytree.FailedTest{zTest},
			noLog: true,
			want:  owned,
		},
		{
			name:          "a nested module without a go.mod is owned",
			log:           zFailure,
			tests:         []verifytree.FailedTest{zTest},
			noToolsModule: true,
			want:          owned,
		},
		{
			name:  "failures only outside the set route to Merge-In, naming the packages sorted, with only the parent fix conditional",
			log:   buildFailed(outsideY) + zFailure,
			tests: []verifytree.FailedTest{zTest},
			want:  notOwned(" outside the task's impacted set, in "+outsideY+", "+outsideZ+",", notOwnedWayForward),
		},
		{
			name:  "a failing test in a package that drives the lyx binary names the Darn-Write goto after the binary clause",
			log:   failedTests(driverPkg, "TestDrive"),
			tests: []verifytree.FailedTest{{Package: driverPkg, Test: "TestDrive"}},
			want: notOwned(
				" outside the task's impacted set, in "+driverPkg+",",
				notOwnedWayForward+"; "+driverPkg+" drive the lyx binary in their tests and may reach the task's change, so when the test does not fail on the parent branch, the Darn-Write goto is the way forward",
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			worktree := t.TempDir()
			writeGoMod := func(dir, module string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(worktree, dir), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(worktree, dir, "go.mod"), []byte("module "+module+"\n\ngo 1.22\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			writeGoMod(".", rootModule)
			if !tt.noToolsModule {
				writeGoMod("tools", toolsModule)
			}
			logPath := filepath.Join(worktree, "verify.log")
			if !tt.noLog {
				if err := os.WriteFile(logPath, []byte(tt.log), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			derive := toldDerive
			if tt.derive != nil {
				derive = tt.derive
			}
			mergeBase := func() (string, error) {
				if tt.mergeBaseErr {
					return "", errors.New("no merge base")
				}
				return "base-sha", nil
			}
			failure := verifytree.PublishFailure{Kind: verifytree.FailureKindLanding, Row: "Merge-In", LogPath: logPath, Tests: tt.tests}

			got := darnFailureRoute(context.Background(), failure, worktree, mergeBase, derive)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("darnFailureRoute() = %+v; want %+v", got, tt.want)
			}
			if tt.want.Target == "Merge-In" && !strings.HasPrefix(got.WayForward, `fix the failure on the parent branch if it fails there too, then run "lyx loom goto --to Merge-In"`) {
				t.Errorf("darnFailureRoute().WayForward = %q; want it to open with the conditional parent fix and the Merge-In goto", got.WayForward)
			}
		})
	}
}
