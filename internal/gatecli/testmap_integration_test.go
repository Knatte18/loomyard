// testmap_integration_test.go drives `lym gate testmap` over a hubforge hub whose prime holds a small Go module.
// The test binary, re-executed in the helper role helperRoleTestMapGo, stands in for `go`.
// It also stands in for the coverage binaries `go` builds, so no test builds a real binary.
// It is Tier 1: it builds a real hub and spawns git and the stand-ins.

package gatecli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// helperRoleTestMapGo makes the test binary the stand-in `go` of a `lym gate testmap` call.
// The same role runs as the coverage binary the stand-in builds.
const helperRoleTestMapGo = "testmap-go"

// The variables the testmap stand-in reads name the module's import path.
// They also name the import path whose coverage build fails, and the one whose coverage build never ends.
// testMapLymBuildsEnv names the file the stand-in appends a line to for each `lym` build it acts as.
// testMapLymSeenEnv names the file a coverage run appends the prebuilt `lym` path it was given to, empty when none.
const (
	testMapModuleEnv    = "GATECLI_TESTMAP_MODULE"
	testMapFailBuildEnv = "GATECLI_TESTMAP_FAIL_BUILD"
	testMapHangBuildEnv = "GATECLI_TESTMAP_HANG_BUILD"
	testMapLymBuildsEnv = "GATECLI_TESTMAP_LYM_BUILDS"
	testMapLymSeenEnv   = "GATECLI_TESTMAP_LYM_SEEN"
)

func init() {
	helperRoles[helperRoleTestMapGo] = runTestMapStandIn
}

// testMapSource is the Go file `<pkg>.go` of each fixture package: two one-line functions, each one coverage block.
func testMapSource(pkg string) string {
	return "package " + pkg + "\n\nfunc One() int { return 1 }\n\nfunc Two() int { return 2 }\n"
}

// testMapTestFile returns a fixture test file of pkg declaring each test of tiers.
// Each test opens with tiergate.Require for its tier.
// A negative tier declares the test with no Require.
func testMapTestFile(pkg string, tiers map[string]int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n\nimport (\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n", pkg)
	for _, name := range slices.Sorted(maps.Keys(tiers)) {
		if tiers[name] < 0 {
			fmt.Fprintf(&b, "\nfunc %s(t *testing.T) {}\n", name)
			continue
		}
		fmt.Fprintf(&b, "\nfunc %s(t *testing.T) {\n\ttiergate.Require(t, %d)\n}\n", name, tiers[name])
	}
	return b.String()
}

// runTestMapStandIn acts on its arguments and returns the process exit code.
// As `go test -c` it runs runTestMapStandInBuild, and as `go build` of `lym` it runs runTestMapStandInLymBuild.
// As a coverage binary run in a package directory, it reports the test `-test.run` names as passing.
// Its profile covers the first function of the package's `<package>.go`.
// A test ending in Two covers the second function too.
// A test ending in Hangs never ends.
// One ending in Moves first commits a change to the package's file.
// One ending in Lands first commits a change to a file outside the package.
// A test run without LYM_TEST_TIER fails, so a map entry proves its tier's env reached it.
// A run records the prebuilt `lym` path it was given in the file testMapLymSeenEnv names.
func runTestMapStandIn() int {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "test" {
		return runTestMapStandInBuild(args)
	}
	if len(args) > 0 && args[0] == "build" {
		return runTestMapStandInLymBuild(args)
	}
	var name, profile string
	for _, arg := range args {
		if pattern, ok := strings.CutPrefix(arg, "-test.run="); ok {
			name = strings.TrimSuffix(strings.TrimPrefix(pattern, "^"), "$")
		}
		if path, ok := strings.CutPrefix(arg, "-test.coverprofile="); ok {
			profile = path
		}
	}
	if seen := os.Getenv(testMapLymSeenEnv); seen != "" {
		if appendLine(seen, os.Getenv(gateslot.PrebuiltLymEnv)) != nil {
			return 1
		}
	}
	dir, err := os.Getwd()
	if err != nil {
		return 1
	}
	pkg := filepath.Base(dir)
	switch {
	case os.Getenv(tiergate.EnvVar) == "":
		fmt.Printf("--- FAIL: %s (0.00s)\n", name)
		return 1
	case strings.HasSuffix(name, "Hangs"):
		time.Sleep(time.Hour)
	case strings.HasSuffix(name, "Moves"):
		if appendAndCommit(dir, pkg+".go") != nil {
			return 1
		}
	case strings.HasSuffix(name, "Lands"):
		if appendAndCommit(filepath.Dir(dir), "landed.txt") != nil {
			return 1
		}
	}
	second := 0
	if strings.HasSuffix(name, "Two") {
		second = 1
	}
	file := os.Getenv(testMapModuleEnv) + "/" + pkg + "/" + pkg + ".go"
	lines := fmt.Sprintf("mode: set\n%s:3.16,3.28 1 1\n%s:5.16,5.28 1 %d\n", file, file, second)
	if err := os.WriteFile(profile, []byte(lines), 0o644); err != nil {
		return 1
	}
	fmt.Printf("--- PASS: %s (0.01s)\n", name)
	return 0
}

// runTestMapStandInBuild acts as `go test -c ... <import path>`.
// It fails when its working directory holds no go.mod, as the go command does outside a module.
// It fails for the import path testMapFailBuildEnv names, and never ends for the one testMapHangBuildEnv names.
// Otherwise it copies the test binary to the `-o` path, so the copy runs as the package's coverage binary.
func runTestMapStandInBuild(args []string) int {
	if _, err := os.Stat("go.mod"); err != nil {
		fmt.Fprintln(os.Stderr, "stand-in: the go command ran outside the module root")
		return 1
	}
	importPath := args[len(args)-1]
	if importPath == os.Getenv(testMapFailBuildEnv) {
		fmt.Fprintln(os.Stderr, "stand-in: the coverage build failed")
		return 1
	}
	if importPath == os.Getenv(testMapHangBuildEnv) {
		time.Sleep(time.Hour)
	}
	return copyTestBinary(args[slices.Index(args, "-o")+1])
}

// runTestMapStandInLymBuild acts as `go build ... -o <path> ./cmd/lym`.
// It records the build in the file testMapLymBuildsEnv names, then copies the test binary to the `-o` path.
func runTestMapStandInLymBuild(args []string) int {
	if builds := os.Getenv(testMapLymBuildsEnv); builds != "" {
		if appendLine(builds, "built") != nil {
			return 1
		}
	}
	return copyTestBinary(args[slices.Index(args, "-o")+1])
}

// copyTestBinary copies the running test binary to the executable file dst and returns the process exit code.
func copyTestBinary(dst string) int {
	self, err := os.Executable()
	if err != nil {
		return 1
	}
	data, err := os.ReadFile(self)
	if err != nil {
		return 1
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		return 1
	}
	return 0
}

// appendLine appends line and a newline to the file, creating it.
func appendLine(file, line string) error {
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// appendAndCommit appends a line to the file rel under dir and commits it alone, moving HEAD as a landing does.
func appendAndCommit(dir, rel string) error {
	file, err := os.OpenFile(filepath.Join(dir, rel), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.WriteString("// landed\n"); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	for _, args := range [][]string{{"add", "--", rel}, {"commit", "-q", "-m", "land " + rel, "--", rel}} {
		if err := exec.Command("git", append([]string{"-C", dir}, args...)...).Run(); err != nil {
			return err
		}
	}
	return nil
}

// testMapOutcome is the envelope of a `lym gate testmap` call that walked its packages.
type testMapOutcome struct {
	Built    []string         `json:"built"`
	Skipped  []testMapSkip    `json:"skipped"`
	Failed   []testMapFailure `json:"failed"`
	TimedOut []string         `json:"timed_out"`
	Unbuilt  int              `json:"unbuilt"`
}

// TestGateTestMap runs `lym gate testmap` calls in order over one hub.
// Each call builds on the maps and HEAD the last one left.
// Its subtests share that state and run in sequence, so none is parallel.
func TestGateTestMap(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	prime, pair := h.PrimeWorktree(), h.PairCodeWorktree(hubforge.TemplatePairSlug)
	const module = "example.com/mapped"
	// A test past 20 seconds of 150 milliseconds each times out: 3 seconds, long enough for a stand-in to start under load.
	// tierFields adds fields to the flow mapping of tier0 and tier1.
	gateYAML := func(tierFields string) string {
		return "slots: 1\nredundancy_build_timeout_min: 1\nredundancy_test_timeout_sec: 20\ntiers:\n" +
			"  tier0: {command: x, timeout_min: 1, counter: go-json, " + tierFields + "env: {LYM_TEST_TIER: \"0\"}}\n" +
			"  tier1: {command: x, timeout_min: 1, counter: go-json, " + tierFields + "env: {LYM_TEST_TIER: \"1\"}}\n" +
			"  tier2: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"2\"}}\n"
	}
	writeGate := func(t *testing.T, content string) {
		t.Helper()
		if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeGate(t, gateYAML(""))

	packageTests := map[string]map[string]int{
		"alpha":   {"TestAlphaOne": 0, "TestAlphaTwo": 1, "TestAlphaHangs": 1, "TestAlphaTier2": 2, "TestAlphaUntiered": -1},
		"broken":  {"TestBrokenOne": 0},
		"dirty":   nil,
		"locked":  {"TestLockedOne": 0},
		"moved":   {"TestMovedMoves": 0},
		"notests": nil,
		"steady":  {"TestSteadyLands": 0},
	}
	files := map[string]string{"go.mod": "module " + module + "\n\ngo 1.21\n"}
	for pkg, tiers := range packageTests {
		files[pkg+"/"+pkg+".go"] = testMapSource(pkg)
		if tiers != nil {
			files[pkg+"/"+pkg+"_test.go"] = testMapTestFile(pkg, tiers)
		}
	}
	// The module also holds directories no package walk may enter: testdata, a dot directory and a nested module.
	// The "ignored" package is on disk but untracked and excluded, so HEAD tracks no Go file of it.
	files["testdata/testdata.go"] = "package testdata\n"
	files[".dot/dot.go"] = "package dot\n"
	files["nested/go.mod"] = "module example.com/nested\n\ngo 1.21\n"
	files["nested/nested.go"] = "package nested\n"
	files["ignored/ignored.go"] = testMapSource("ignored")
	plankit.WriteTree(t, prime, files)
	excludeOut, err := exec.Command("git", "-C", prime, "rev-parse", "--git-path", "info/exclude").Output()
	if err != nil {
		t.Fatal(err)
	}
	excludePath := strings.TrimSpace(string(excludeOut))
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(prime, excludePath)
	}
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(excludePath, []byte("ignored/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitkit.Git(t, prime, append([]string{"add", "go.mod", "testdata", ".dot", "nested"}, slices.Sorted(maps.Keys(packageTests))...)...)
	gitkit.Git(t, prime, "commit", "-q", "-m", "seed the mapped module")
	if err := os.WriteFile(filepath.Join(prime, "dirty", "extra.go"), []byte("package dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mapsDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
	lockPath := testMapBuildLockPath(mapsDir, "locked")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	held, err := lock.AcquireWriteLock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { held.Release() })

	standIn, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environ := append(os.Environ(), helperEnv+"="+helperRoleTestMapGo, testMapModuleEnv+"="+module, testMapFailBuildEnv+"="+module+"/broken")
	// callWith runs the verb over the prime under the environment env, with one build minute lasting minuteUnit.
	callWith := func(t *testing.T, env []string, minuteUnit time.Duration, maxPackages int, packages ...string) testMapOutcome {
		t.Helper()
		var out bytes.Buffer
		code := runTestMap(lymcwd.WithCwd(context.Background(), prime), &out, testMapRequest{
			goBinary: standIn, maxPackages: maxPackages, packages: packages, environ: env,
			secondUnit: 150 * time.Millisecond, minuteUnit: minuteUnit, grace: time.Second,
		})
		if code != 0 {
			t.Fatalf("runTestMap exit = %d; want 0; output %s", code, out.String())
		}
		var got testMapOutcome
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal %q: %v", out.String(), err)
		}
		return got
	}
	call := func(t *testing.T, maxPackages int, packages ...string) testMapOutcome {
		t.Helper()
		return callWith(t, environ, time.Minute, maxPackages, packages...)
	}
	readMap := func(t *testing.T, pkg string) testcover.Map {
		t.Helper()
		m, found, err := testcover.ReadMap(mapsDir, pkg)
		if err != nil || !found {
			t.Fatalf("ReadMap(%s) = found %v, err %v; want a map", pkg, found, err)
		}
		return m
	}
	wantExitRecord := func(t *testing.T, unbuilt int) {
		t.Helper()
		record, found, err := testcover.ReadExitRecord(mapsDir)
		if want := (testcover.ExitRecord{PID: os.Getpid(), Unbuilt: unbuilt}); err != nil || !found || record != want {
			t.Errorf("ReadExitRecord() = %+v, found %v, err %v; want %+v", record, found, err, want)
		}
	}
	// wantOutcome compares an outcome but its failures, which wantBrokenFailed checks.
	wantOutcome := func(t *testing.T, got, want testMapOutcome) {
		t.Helper()
		if diff := cmp.Diff(want, got, cmpopts.EquateEmpty(), cmpopts.IgnoreFields(testMapOutcome{}, "Failed")); diff != "" {
			t.Errorf("runTestMap() mismatch (-want +got):\n%s", diff)
		}
	}
	wantBrokenFailed := func(t *testing.T, got testMapOutcome) {
		t.Helper()
		if len(got.Failed) != 1 || got.Failed[0].Package != "broken" || !strings.Contains(got.Failed[0].Failure, "the coverage build failed") {
			t.Errorf("runTestMap() failed = %+v; want broken alone, naming its build failure", got.Failed)
		}
	}
	dirtySkip := testMapSkip{Package: "dirty", Cause: "modified or untracked file dirty/extra.go"}
	lockedSkip := testMapSkip{Package: "locked", Cause: "another call holds its map build lock"}
	movedSkip := testMapSkip{Package: "moved", Cause: "files changed under its tests: moved.go"}
	ignoreRunFields := cmpopts.IgnoreFields(testcover.MapTest{}, "FuncHash", "Seconds")
	outsideHub := t.TempDir()

	t.Run("Refusals", func(t *testing.T) {
		tests := []struct {
			name string
			dir  string
			args []string
			want string
			// setup, when set, changes the hub's gate config for the row.
			setup func(t *testing.T)
			// writesRecord marks a refusal after the call's state exists, which still writes the exit record.
			writesRecord bool
		}{
			{
				name: "OutsideThePrime", dir: pair, args: []string{"testmap"},
				want: "gate testmap: " + pair + " is not the hub's prime worktree, and a coverage map records the prime branch; way forward: run lym gate testmap from " + prime,
			},
			{
				name: "OutsideEveryHub", dir: outsideHub, args: []string{"testmap"},
				want: "gate testmap: " + outsideHub + " is in no hub, and the coverage maps live in a hub; way forward: run lym gate testmap from a hub's prime worktree",
			},
			{name: "MaxBelowOne", dir: prime, args: []string{"testmap", "--max", "0"}, want: "gate testmap: --max 0; way forward: pass --max 1 or more"},
			{
				name: "NoMappedTier", dir: prime, args: []string{"testmap"}, writesRecord: true,
				want: "gate testmap: the hub declares neither tier0 nor tier1, so no test of a package can be mapped; way forward: declare them with lym config gate from the prime, then re-run the same command",
				setup: func(t *testing.T) {
					writeGate(t, "slots: 1\ntiers:\n  tier2: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"2\"}}\n")
					t.Cleanup(func() { writeGate(t, gateYAML("")) })
				},
			},
			{
				name: "UnknownPackage", dir: prime, args: []string{"testmap", "nosuch"}, writesRecord: true,
				want: "gate testmap: \"nosuch\" names no package directory of the module at " + prime + "; way forward: name a package by its directory relative to the module root, such as internal/foo",
			},
		}
		wantRecord := false
		for _, tt := range tests {
			wantRecord = wantRecord || tt.writesRecord
			t.Run(tt.name, func(t *testing.T) {
				if tt.setup != nil {
					tt.setup(t)
				}
				var out bytes.Buffer
				code := RunCLIIn(tt.dir, &out, tt.args)
				var envelope struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
					t.Fatalf("unmarshal %q: %v", out.String(), err)
				}
				if code != 1 || envelope.Error != tt.want {
					t.Errorf("lym gate %q = exit %d, error %q; want exit 1, error %q", tt.args, code, envelope.Error, tt.want)
				}
			})
		}
		if _, found, err := testcover.ReadExitRecord(mapsDir); err != nil || found != wantRecord {
			t.Errorf("ReadExitRecord after the refusals = found %v, err %v; want found %v, written only by a refusal after the call's state exists", found, err, wantRecord)
		}
	})

	t.Run("CapLeavesStalePackagesUnbuilt", func(t *testing.T) {
		head := gitkit.RevParse(t, prime, "HEAD")
		got := call(t, 2)
		wantOutcome(t, got, testMapOutcome{
			Built: []string{"alpha"}, Skipped: []testMapSkip{dirtySkip, lockedSkip}, TimedOut: []string{"alpha.TestAlphaHangs"}, Unbuilt: 3,
		})
		wantBrokenFailed(t, got)

		alpha := readMap(t, "alpha")
		wantTests := []testcover.MapTest{{Name: "TestAlphaOne", Tier: 0, Blocks: []int{0}}, {Name: "TestAlphaTwo", Tier: 1, Blocks: []int{0, 1}}}
		if diff := cmp.Diff(wantTests, alpha.Tests, ignoreRunFields); diff != "" {
			t.Errorf("alpha map tests mismatch (-want +got):\n%s", diff)
		}
		wantFiles := map[string]string{
			"alpha.go":      testcover.ContentHash([]byte(files["alpha/alpha.go"])),
			"alpha_test.go": testcover.ContentHash([]byte(files["alpha/alpha_test.go"])),
		}
		if alpha.Commit != head || len(alpha.Blocks) != 2 || !maps.Equal(alpha.Files, wantFiles) {
			t.Errorf("alpha map = commit %s, %d blocks, files %v; want commit %s, 2 blocks, files %v", alpha.Commit, len(alpha.Blocks), alpha.Files, head, wantFiles)
		}
		if broken := readMap(t, "broken"); broken.Commit != head || len(broken.Tests) != 0 || !strings.Contains(broken.BuildFailure, "the coverage build failed") {
			t.Errorf("broken map = commit %s, tests %v, failure %q; want commit %s, no test, the build failure", broken.Commit, broken.Tests, broken.BuildFailure, head)
		}
		wantExitRecord(t, 3)
	})

	t.Run("SecondCallMapsTheRestAndKeepsAFailureAtOneHead", func(t *testing.T) {
		alphaBefore := readMap(t, "alpha")
		got := call(t, 10)
		wantOutcome(t, got, testMapOutcome{Built: []string{"notests", "steady"}, Skipped: []testMapSkip{dirtySkip, lockedSkip, movedSkip}})
		if len(got.Failed) != 0 {
			t.Errorf("runTestMap() failed = %+v; want broken not rebuilt at the HEAD of its failure", got.Failed)
		}
		steady := readMap(t, "steady")
		if diff := cmp.Diff([]testcover.MapTest{{Name: "TestSteadyLands", Blocks: []int{0}}}, steady.Tests, ignoreRunFields); diff != "" {
			t.Errorf("steady map tests mismatch (-want +got):\n%s", diff)
		}
		if before := gitkit.RevParse(t, prime, "HEAD~1"); steady.Commit != before {
			t.Errorf("steady map commit = %s; want %s, the HEAD read before its tests landed a commit outside it", steady.Commit, before)
		}
		if notests := readMap(t, "notests"); len(notests.Tests) != 0 || notests.BuildFailure != "" {
			t.Errorf("notests map = tests %v, failure %q; want no test and no failure", notests.Tests, notests.BuildFailure)
		}
		if alpha := readMap(t, "alpha"); !alpha.BuiltAt.Equal(alphaBefore.BuiltAt) {
			t.Errorf("alpha map built at %v; want it left from %v", alpha.BuiltAt, alphaBefore.BuiltAt)
		}
		if _, found, err := testcover.ReadMap(mapsDir, "moved"); err != nil || found {
			t.Errorf("ReadMap(moved) = found %v, err %v; want no map for a package whose files changed under its tests", found, err)
		}
		wantExitRecord(t, 0)
	})

	t.Run("HeadMoveRetriesTheFailedBuildAndAgeRebuilds", func(t *testing.T) {
		aged := readMap(t, "notests")
		aged.BuiltAt = time.Now().Add(-8 * 24 * time.Hour)
		if err := testcover.WriteMap(mapsDir, "notests", aged); err != nil {
			t.Fatal(err)
		}
		got := call(t, 10)
		wantOutcome(t, got, testMapOutcome{Built: []string{"notests"}, Skipped: []testMapSkip{dirtySkip, lockedSkip, movedSkip}})
		wantBrokenFailed(t, got)
	})

	t.Run("NamedPackageIsRebuiltWhenFresh", func(t *testing.T) {
		before := readMap(t, "alpha")
		got := call(t, 1, "./alpha")
		wantOutcome(t, got, testMapOutcome{Built: []string{"alpha"}, TimedOut: []string{"alpha.TestAlphaHangs"}})
		if after := readMap(t, "alpha"); !after.BuiltAt.After(before.BuiltAt) {
			t.Errorf("alpha map built at %v; want after %v", after.BuiltAt, before.BuiltAt)
		}
	})

	t.Run("ChangedFileContentAndFileSetRebuild", func(t *testing.T) {
		edited := files["alpha/alpha.go"] + "// edited\n"
		if err := os.WriteFile(filepath.Join(prime, "alpha", "alpha.go"), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(prime, "steady", "extra.go"), []byte("package steady\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitkit.Git(t, prime, "add", "alpha/alpha.go", "steady/extra.go")
		gitkit.Git(t, prime, "commit", "-q", "-m", "edit alpha and add a file to steady")

		got := call(t, 10)
		wantOutcome(t, got, testMapOutcome{
			Built: []string{"alpha", "steady"}, Skipped: []testMapSkip{dirtySkip, lockedSkip, movedSkip}, TimedOut: []string{"alpha.TestAlphaHangs"},
		})
		wantBrokenFailed(t, got)
		if alpha := readMap(t, "alpha"); alpha.Files["alpha.go"] != testcover.ContentHash([]byte(edited)) {
			t.Errorf("alpha map files = %v; want alpha.go hashed from the edited content", alpha.Files)
		}
		if steady := readMap(t, "steady"); steady.Files["extra.go"] != testcover.ContentHash([]byte("package steady\n")) {
			t.Errorf("steady map files = %v; want the added extra.go", steady.Files)
		}
	})

	t.Run("SkippedPackageCountsAgainstMax", func(t *testing.T) {
		aged := readMap(t, "notests")
		aged.BuiltAt = time.Now().Add(-8 * 24 * time.Hour)
		if err := testcover.WriteMap(mapsDir, "notests", aged); err != nil {
			t.Fatal(err)
		}
		got := call(t, 2)
		wantOutcome(t, got, testMapOutcome{Skipped: []testMapSkip{dirtySkip, lockedSkip, movedSkip}, Unbuilt: 1})
		wantBrokenFailed(t, got)
		wantExitRecord(t, 1)
	})

	t.Run("NamedPackageWithNoTrackedGoFileIsSkipped", func(t *testing.T) {
		got := call(t, 1, "ignored")
		wantOutcome(t, got, testMapOutcome{Skipped: []testMapSkip{{Package: "ignored", Cause: "no Go file tracked at HEAD"}}})
	})

	t.Run("BuildPastItsTimeoutFailsThePackage", func(t *testing.T) {
		hangEnviron := append(slices.Clone(environ), testMapHangBuildEnv+"="+module+"/steady")
		got := callWith(t, hangEnviron, 2*time.Second, 1, "steady")
		wantOutcome(t, got, testMapOutcome{})
		if len(got.Failed) != 1 || got.Failed[0].Package != "steady" || !strings.Contains(got.Failed[0].Failure, "timed out after 2s") {
			t.Errorf("runTestMap() failed = %+v; want steady alone, naming the build timeout", got.Failed)
		}
	})

	t.Run("PrebuiltLymIsBuiltOncePerCall", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(prime, "cmd", "lym"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(prime, "cmd", "lym", "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitkit.Git(t, prime, "add", "cmd")
		gitkit.Git(t, prime, "commit", "-q", "-m", "add the lym command")
		writeGate(t, gateYAML("prebuilt_lym: true, "))

		records := t.TempDir()
		builds, seen := filepath.Join(records, "builds"), filepath.Join(records, "seen")
		lymEnviron := append(slices.Clone(environ), testMapLymBuildsEnv+"="+builds, testMapLymSeenEnv+"="+seen)
		got := callWith(t, lymEnviron, time.Minute, 10, "alpha", "steady")
		wantOutcome(t, got, testMapOutcome{Built: []string{"alpha", "steady"}, TimedOut: []string{"alpha.TestAlphaHangs"}})
		if len(got.Failed) != 0 {
			t.Errorf("runTestMap() failed = %+v; want none", got.Failed)
		}

		built, err := os.ReadFile(builds)
		if err != nil {
			t.Fatal(err)
		}
		if lines := strings.Fields(string(built)); len(lines) != 1 {
			t.Errorf("lym builds = %d; want 1 for the two packages", len(lines))
		}
		seenData, err := os.ReadFile(seen)
		if err != nil {
			t.Fatal(err)
		}
		paths := strings.Split(strings.TrimSpace(string(seenData)), "\n")
		if distinct := slices.Compact(slices.Sorted(slices.Values(paths))); len(paths) < 2 || len(distinct) != 1 || distinct[0] == "" {
			t.Errorf("prebuilt lym paths the tests saw = %q; want one non-empty path for every run of both packages", paths)
		}
	})
}
