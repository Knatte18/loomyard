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
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// helperRoleTestMapGo makes the test binary the stand-in `go` of a `lym gate testmap` call.
// The same role runs as the coverage binary the stand-in builds.
const helperRoleTestMapGo = "testmap-go"

// helperRoleTestMapCLI makes the test binary run `lym gate testmap` through its cobra command, as a child process.
// It is the one way a test reaches the command's flags and a terminating signal without signalling the test process.
const helperRoleTestMapCLI = "testmap-cli"

// The variables the testmap stand-in reads name the module's import path.
// They also name the import path whose coverage build fails, and the one whose coverage build never ends.
// testMapLymBuildsEnv names the file the stand-in appends a line to for each `lym` build it acts as.
// testMapLymSeenEnv names the file a coverage run appends the prebuilt `lym` path it was given to, empty when none.
// testMapCoverBuildsEnv names the file the stand-in appends a line to for each coverage build it acts as.
// Each line holds the build's import path and coverpkg.
// testMapFailLymBuildEnv, when set, makes every `lym` build the stand-in acts as fail.
// testMapLinksEnv lists, comma-separated, `<package>=<linked package>` pairs of fixture package names:
// the stand-in lists the linked package as linked by the package's test binary, and profiles its blocks.
// testMapDirtyEnv names the untracked file a test ending in Dirties writes.
// testMapFailListEnv, when set, makes every `go list` the stand-in acts as fail.
// testMapFailGoEnvEnv, when set, does the same for every `go env`.
// testMapLandEnv names the fixture package whose file a test ending in LandsLinked commits a change to.
// testMapLandFileEnv, when set, names the file of that package, creating it, instead of its `<package>.go`.
// testMapRemoveFileEnv, when set, names the file of that package the commit removes instead.
// testMapHangListEnv names the file the stand-in appends a line to before every `go list` it acts as never ends.
// testMapHangLymBuildEnv, when set, makes every `lym` build the stand-in acts as never end, after it records itself.
const (
	testMapModuleEnv      = "GATECLI_TESTMAP_MODULE"
	testMapFailBuildEnv   = "GATECLI_TESTMAP_FAIL_BUILD"
	testMapHangBuildEnv   = "GATECLI_TESTMAP_HANG_BUILD"
	testMapLymBuildsEnv   = "GATECLI_TESTMAP_LYM_BUILDS"
	testMapLymSeenEnv     = "GATECLI_TESTMAP_LYM_SEEN"
	testMapCoverBuildsEnv = "GATECLI_TESTMAP_COVER_BUILDS"

	testMapFailLymBuildEnv = "GATECLI_TESTMAP_FAIL_LYM_BUILD"
	// testMapHangingEnv names the file a coverage run appends the test name to once it is about to hang.
	testMapHangingEnv = "GATECLI_TESTMAP_HANGING"
	testMapLinksEnv   = "GATECLI_TESTMAP_LINKS"
	testMapDirtyEnv   = "GATECLI_TESTMAP_DIRTY"

	testMapFailListEnv  = "GATECLI_TESTMAP_FAIL_LIST"
	testMapFailGoEnvEnv = "GATECLI_TESTMAP_FAIL_GO_ENV"
	testMapLandEnv      = "GATECLI_TESTMAP_LAND"

	testMapLandFileEnv   = "GATECLI_TESTMAP_LAND_FILE"
	testMapRemoveFileEnv = "GATECLI_TESTMAP_REMOVE_FILE"

	testMapHangListEnv     = "GATECLI_TESTMAP_HANG_LIST"
	testMapHangLymBuildEnv = "GATECLI_TESTMAP_HANG_LYM_BUILD"

	// testMapMeetEnv names the file a test ending in Meets records itself in before it waits for another to do the same.
	testMapMeetEnv = "GATECLI_TESTMAP_MEET"
)

// testMapMeetWait bounds a Meets test's wait for another test to record itself beside it.
const testMapMeetWait = 10 * time.Second

// The stand-in's `go env GOVERSION GOOS GOARCH` answer.
const (
	testMapGoVersion = "go1.26.8"
	testMapGOOS      = "linux"
	testMapGOARCH    = "amd64"
)

func init() {
	helperRoles[helperRoleTestMapGo] = runTestMapStandIn
	helperRoles[helperRoleTestMapCLI] = runTestMapCLIHelper
}

// runTestMapCLIHelper runs `lym gate testmap` with the process's arguments over the directory helperCwdEnv names.
// The command's `go` is this test binary, in the helperRoleTestMapGo role.
func runTestMapCLIHelper() int {
	os.Setenv(helperEnv, helperRoleTestMapGo)
	root := &cobra.Command{Use: "gate", Args: cobra.ArbitraryArgs, RunE: clihelp.GroupRunE}
	root.AddCommand(testMapCmd(os.Args[0], time.Minute, time.Second))
	code := clihelp.ExecuteIn(root, os.Getenv(helperCwdEnv), os.Stdout, os.Args[1:])
	_ = lymtemp.Remove()
	return code
}

// startTestMapCLI starts the test binary in the helperRoleTestMapCLI role over the prime worktree prime, under env.
// The child runs `gate testmap` with args and is killed when the test ends or after two minutes.
// It returns the child and the buffer its output fills until waitTestMapCLI returns.
func startTestMapCLI(t *testing.T, prime string, env []string, args ...string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, os.Args[0], args...)
	cmd.Env = append(slices.Clone(env), helperEnv+"="+helperRoleTestMapCLI, helperCwdEnv+"="+prime)
	cmd.Stdout = &out
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the testmap child: %v", err)
	}
	return cmd, &out
}

// waitTestMapCLI waits for the child cmd to end and returns its exit code.
func waitTestMapCLI(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	err := cmd.Wait()
	if err == nil {
		return 0
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("wait for the testmap child: %v", err)
	}
	return exitErr.ExitCode()
}

// pollUntil polls until done reports true, failing the test after 30 seconds.
// what names the condition for the failure.
func pollUntil(t *testing.T, what string, done func() bool) {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); !done(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
	}
}

// seedTestMapModule writes the gate config gate into the hub h and commits a Go module to its prime worktree.
// The module is named module and holds a package per key of packageTests, declaring the tests that key maps to.
// A package mapped to nil has a source file and no test file.
// It returns the prime worktree.
func seedTestMapModule(t *testing.T, h *hubforge.Hub, module, gate string, packageTests map[string]map[string]int) string {
	t.Helper()
	if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(gate), 0o644); err != nil {
		t.Fatal(err)
	}
	prime := h.PrimeWorktree()
	files := map[string]string{"go.mod": "module " + module + "\n\ngo 1.21\n"}
	for pkg, tiers := range packageTests {
		files[pkg+"/"+pkg+".go"] = testMapSource(pkg)
		if tiers != nil {
			files[pkg+"/"+pkg+"_test.go"] = testMapTestFile(pkg, tiers)
		}
	}
	plankit.WriteTree(t, prime, files)
	gitkit.Git(t, prime, append([]string{"add", "go.mod"}, slices.Sorted(maps.Keys(packageTests))...)...)
	gitkit.Git(t, prime, "commit", "-q", "-m", "seed the "+module+" module")
	return prime
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
// As `go list` it runs runTestMapStandInList.
// As `go env` it prints the testMapGoVersion, testMapGOOS and testMapGOARCH lines.
// Both fail when the variable naming them, testMapFailListEnv or testMapFailGoEnvEnv, is set.
// As a coverage binary run in a package directory, it reports the test `-test.run` names as passing.
// Its profile covers the first function of the package's `<package>.go`.
// A test ending in Two covers the second function too, and one ending in Elsewhere neither.
// The profile of a package testMapLinksEnv links also lists the linked package's two functions,
// and a test ending in Calls or Elsewhere covers the first of them.
// A test ending in Fails reports FAIL and one ending in Skips reports SKIP, each writing no profile.
// A test ending in Hangs never ends, after it records its name in the file testMapHangingEnv names.
// One ending in Moves first commits a change to the package's file.
// One ending in Lands first commits a change to a file outside the package.
// One ending in LandsLinked first commits a change to a file of the package testMapLandEnv names, when it names one:
// the file testMapLandFileEnv names, its removal when testMapRemoveFileEnv names a file, or else the package's own file.
// One ending in Dirties first writes the file testMapDirtyEnv names, when it names one.
// One ending in Meets, when testMapMeetEnv names a file, records itself there.
// It fails unless another test records itself there within testMapMeetWait, so it passes only beside another.
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
	if len(args) > 0 && args[0] == "list" {
		return runTestMapStandInList(args)
	}
	if len(args) > 0 && args[0] == "env" {
		if os.Getenv(testMapFailGoEnvEnv) != "" {
			fmt.Fprintln(os.Stderr, "stand-in: go env failed")
			return 1
		}
		fmt.Printf("%s\n%s\n%s\n", testMapGoVersion, testMapGOOS, testMapGOARCH)
		return 0
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
	case strings.HasSuffix(name, "Fails"):
		fmt.Printf("--- FAIL: %s (0.00s)\n", name)
		return 1
	case strings.HasSuffix(name, "Skips"):
		fmt.Printf("--- SKIP: %s (0.00s)\n", name)
		return 0
	case strings.HasSuffix(name, "Hangs"):
		if hanging := os.Getenv(testMapHangingEnv); hanging != "" {
			if appendLine(hanging, name) != nil {
				return 1
			}
		}
		time.Sleep(time.Hour)
	case strings.HasSuffix(name, "Moves"):
		if appendAndCommit(dir, pkg+".go") != nil {
			return 1
		}
	case strings.HasSuffix(name, "Lands"):
		if appendAndCommit(filepath.Dir(dir), "landed.txt") != nil {
			return 1
		}
	case strings.HasSuffix(name, "LandsLinked"):
		if landed := os.Getenv(testMapLandEnv); landed != "" && landInLinked(filepath.Join(filepath.Dir(dir), landed), landed) != nil {
			return 1
		}
	case strings.HasSuffix(name, "Dirties"):
		if dirty := os.Getenv(testMapDirtyEnv); dirty != "" && os.WriteFile(dirty, []byte("dirty\n"), 0o644) != nil {
			return 1
		}
	case strings.HasSuffix(name, "Meets"):
		if meet := os.Getenv(testMapMeetEnv); meet != "" && !meetsAnother(meet, name) {
			fmt.Printf("--- FAIL: %s (0.00s)\n", name)
			return 1
		}
	}
	first, second := 1, 0
	if strings.HasSuffix(name, "Two") {
		second = 1
	}
	if strings.HasSuffix(name, "Elsewhere") {
		first = 0
	}
	module := os.Getenv(testMapModuleEnv)
	file := module + "/" + pkg + "/" + pkg + ".go"
	lines := fmt.Sprintf("mode: set\n%s:3.16,3.28 1 %d\n%s:5.16,5.28 1 %d\n", file, first, file, second)
	if linked := testMapLinkOf(pkg); linked != "" {
		calls := 0
		if strings.HasSuffix(name, "Calls") || strings.HasSuffix(name, "Elsewhere") {
			calls = 1
		}
		linkedFile := module + "/" + linked + "/" + linked + ".go"
		lines += fmt.Sprintf("%s:3.16,3.28 1 %d\n%s:5.16,5.28 1 0\n", linkedFile, calls, linkedFile)
	}
	if err := os.WriteFile(profile, []byte(lines), 0o644); err != nil {
		return 1
	}
	fmt.Printf("--- PASS: %s (0.01s)\n", name)
	return 0
}

// meetsAnother appends name to the file meet and reports whether the file holds a second line within testMapMeetWait.
func meetsAnother(meet, name string) bool {
	if appendLine(meet, name) != nil {
		return false
	}
	for deadline := time.Now().Add(testMapMeetWait); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if data, err := os.ReadFile(meet); err == nil && bytes.Count(data, []byte("\n")) >= 2 {
			return true
		}
	}
	return false
}

// testMapLinkOf returns the fixture package that testMapLinksEnv names as linked by the fixture package pkg, or "".
func testMapLinkOf(pkg string) string {
	for _, pair := range strings.Split(os.Getenv(testMapLinksEnv), ",") {
		if from, to, found := strings.Cut(pair, "="); found && from == pkg {
			return to
		}
	}
	return ""
}

// runTestMapStandInList acts as `go list -deps -test -f <template> <import path>`.
// It prints, in the template's form, the package itself and the package testMapLinksEnv names as linked by it.
// It fails when testMapFailListEnv is set, and never ends after recording itself when testMapHangListEnv names a file.
func runTestMapStandInList(args []string) int {
	if os.Getenv(testMapFailListEnv) != "" {
		fmt.Fprintln(os.Stderr, "stand-in: go list failed")
		return 1
	}
	if hanging := os.Getenv(testMapHangListEnv); hanging != "" {
		if appendLine(hanging, "list") != nil {
			return 1
		}
		time.Sleep(time.Hour)
	}
	importPath := args[len(args)-1]
	fmt.Printf("%s\t%s\t\n", importPath, path.Base(importPath))
	if linked := testMapLinkOf(path.Base(importPath)); linked != "" {
		fmt.Printf("%s/%s\t%s\t\n", os.Getenv(testMapModuleEnv), linked, linked)
	}
	return 0
}

// runTestMapStandInBuild acts as `go test -c ... <import path>`.
// It fails when its working directory holds no go.mod, as the go command does outside a module.
// It records the import path and the coverpkg list in the file testMapCoverBuildsEnv names, when one is named.
// It fails for the import path testMapFailBuildEnv names, and never ends for the one testMapHangBuildEnv names.
// Otherwise it copies the test binary to the `-o` path, so the copy runs as the package's coverage binary.
func runTestMapStandInBuild(args []string) int {
	if _, err := os.Stat("go.mod"); err != nil {
		fmt.Fprintln(os.Stderr, "stand-in: the go command ran outside the module root")
		return 1
	}
	importPath := args[len(args)-1]
	if builds := os.Getenv(testMapCoverBuildsEnv); builds != "" {
		coverpkg := ""
		for _, arg := range args {
			if value, ok := strings.CutPrefix(arg, "-coverpkg="); ok {
				coverpkg = value
			}
		}
		if appendLine(builds, importPath+" "+coverpkg) != nil {
			return 1
		}
	}
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
// It records the build in the file testMapLymBuildsEnv names, then never ends when testMapHangLymBuildEnv is set.
// It fails when testMapFailLymBuildEnv is set, and otherwise copies the test binary to the `-o` path.
func runTestMapStandInLymBuild(args []string) int {
	if builds := os.Getenv(testMapLymBuildsEnv); builds != "" {
		if appendLine(builds, "built") != nil {
			return 1
		}
	}
	if os.Getenv(testMapHangLymBuildEnv) != "" {
		time.Sleep(time.Hour)
	}
	if os.Getenv(testMapFailLymBuildEnv) != "" {
		fmt.Fprintln(os.Stderr, "stand-in: the lym build failed")
		return 1
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

// landInLinked commits a change to the linked fixture package pkg, whose directory is dir.
// When testMapRemoveFileEnv names a file, it removes that file.
// Otherwise it changes the file testMapLandFileEnv names, or `<pkg>.go` when none is named.
func landInLinked(dir, pkg string) error {
	if removed := os.Getenv(testMapRemoveFileEnv); removed != "" {
		return gitSteps(dir, []string{"rm", "-q", "--", removed}, []string{"commit", "-q", "-m", "remove " + removed, "--", removed})
	}
	if named := os.Getenv(testMapLandFileEnv); named != "" {
		return appendAndCommit(dir, named)
	}
	return appendAndCommit(dir, pkg+".go")
}

// gitSteps runs each git command of steps in dir, stopping at the first failure.
func gitSteps(dir string, steps ...[]string) error {
	for _, args := range steps {
		if err := exec.Command("git", append([]string{"-C", dir}, args...)...).Run(); err != nil {
			return err
		}
	}
	return nil
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
	return gitSteps(dir, []string{"add", "--", rel}, []string{"commit", "-q", "-m", "land " + rel, "--", rel})
}

// testMapOutcome is the envelope of a `lym gate testmap` call that walked its packages, bar its plan and timings.
type testMapOutcome struct {
	Built               []string         `json:"built"`
	Skipped             []testMapSkip    `json:"skipped"`
	Failed              []testMapFailure `json:"failed"`
	TimedOut            []string         `json:"timed_out"`
	Queued              int              `json:"queued"`
	SelfCheckMismatches int              `json:"self_check_mismatches"`
	AuditMismatches     int              `json:"audit_mismatches"`
}

// runTestMapIn runs `lym gate testmap` over the prime worktree prime with no audit round.
// A call naming no package runs with --upkeep, so it registers and writes the exit record.
// It returns the call's exit code and output.
// It uses the go command goBinary and the environment env.
// One build minute lasts minuteUnit.
func runTestMapIn(prime, goBinary string, env []string, minuteUnit time.Duration, maxPackages int, packages ...string) (int, []byte) {
	return runTestMapRequest(prime, testMapRequest{
		goBinary: goBinary, maxPackages: maxPackages, packages: packages, environ: env, upkeep: len(packages) == 0,
		secondUnit: 150 * time.Millisecond, minuteUnit: minuteUnit, grace: time.Second,
		waitBase: testMapTestWait, waitCeiling: 5 * testMapTestWait, yieldPoll: testMapTestWait,
	})
}

// testMapTestWait is the first pause of a test call's waits and the pause of its yield checks.
const testMapTestWait = 10 * time.Millisecond

// runTestMapRequest runs `lym gate testmap` over the prime worktree prime as request asks.
// It returns the call's exit code and output.
func runTestMapRequest(prime string, request testMapRequest) (int, []byte) {
	var out bytes.Buffer
	code := runTestMap(lymcwd.WithCwd(context.Background(), prime), &out, request)
	return code, out.Bytes()
}

// callTestMap runs `lym gate testmap` as runTestMapIn does.
// It fails the test unless the call walked its packages, and returns the call's envelope.
func callTestMap(t *testing.T, prime, goBinary string, env []string, minuteUnit time.Duration, maxPackages int, packages ...string) testMapOutcome {
	t.Helper()
	code, out := runTestMapIn(prime, goBinary, env, minuteUnit, maxPackages, packages...)
	return decodeTestMap(t, code, out)
}

// decodeTestMap fails the test unless a `lym gate testmap` call walked its packages.
// code is the call's exit code and out its output; it returns the call's envelope.
func decodeTestMap(t *testing.T, code int, out []byte) testMapOutcome {
	t.Helper()
	if code != 0 {
		t.Fatalf("runTestMap exit = %d; want 0; output %s", code, out)
	}
	var got testMapOutcome
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	return got
}

// readTestMap returns the coverage map of pkg under the maps directory mapsDir, failing the test when there is none.
func readTestMap(t *testing.T, mapsDir, pkg string) testcover.Map {
	t.Helper()
	m, found, err := testcover.ReadMap(mapsDir, pkg)
	if err != nil || !found {
		t.Fatalf("ReadMap(%s) = found %v, err %v; want a map", pkg, found, err)
	}
	return m
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
	// Two slots let a pass take one while the other stays free.
	gateYAML := func(tierFields string) string {
		return "slots: 2\nredundancy_build_timeout_min: 1\nredundancy_test_timeout_sec: 20\ntiers:\n" +
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

	// zmoved's test commits a change to its own file, and its name puts its job last.
	packageTests := map[string]map[string]int{
		"alpha":   {"TestAlphaOne": 0, "TestAlphaTwo": 1, "TestAlphaHangs": 1, "TestAlphaTier2": 2, "TestAlphaUntiered": -1},
		"broken":  {"TestBrokenOne": 0},
		"dirty":   nil,
		"locked":  {"TestLockedOne": 0},
		"zmoved":  {"TestZmovedMoves": 0},
		"notests": nil,
		"steady":  {"TestSteadyOne": 0},
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
		return callTestMap(t, prime, standIn, env, minuteUnit, maxPackages, packages...)
	}
	call := func(t *testing.T, maxPackages int, packages ...string) testMapOutcome {
		t.Helper()
		return callWith(t, environ, time.Minute, maxPackages, packages...)
	}
	readMap := func(t *testing.T, pkg string) testcover.Map {
		t.Helper()
		return readTestMap(t, mapsDir, pkg)
	}
	// wantExitRecord checks the exit record's pid, planned HEAD and queued count.
	wantExitRecord := func(t *testing.T, head string, queued int) {
		t.Helper()
		record, found, err := testcover.ReadExitRecord(mapsDir)
		if err != nil || !found || record.PID != os.Getpid() || record.PlannedHead != head || record.Queued != queued {
			t.Errorf("ReadExitRecord() = %+v, found %v, err %v; want pid %d, planned HEAD %s, %d queued", record, found, err, os.Getpid(), head, queued)
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
	movedSkip := testMapSkip{Package: "zmoved", Cause: "files changed under its tests: zmoved.go"}
	ignoreRunFields := cmpopts.IgnoreFields(testcover.MapTest{}, "FuncHash", "Seconds", "Commit", "MeasuredAt")
	outsideHub := t.TempDir()
	// startLivePass registers a live process as the hub's upkeep pass until the test ends, and returns its pid.
	startLivePass := func(t *testing.T) int {
		t.Helper()
		pass := exec.Command(standIn, "-test.run=^TestLivePassHangs$")
		pass.Dir = t.TempDir()
		pass.Env = append(slices.Clone(environ), tiergate.EnvVar+"=0")
		if err := pass.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			pass.Process.Kill()
			pass.Wait()
		})
		record := runproc.ChildRecord(pass.Process.Pid, runproc.RoleTestMap, pass.Args)
		if err := runproc.Register(fabricengine.RunProcDir(h.Location.HubPath), runproc.HubOwner, record); err != nil {
			t.Fatal(err)
		}
		return pass.Process.Pid
	}

	t.Run("Refusals", func(t *testing.T) {
		// The rows run in order: the live pass a row starts ends with the row, before the next registers.
		tests := []struct {
			name string
			dir  string
			args []string
			want string
			// setup, when set, changes the hub's gate config for the row.
			setup func(t *testing.T)
			// livePass starts a registered upkeep pass for the row; want then holds a %d for its pid.
			livePass bool
			// contains, when set, replaces want: the error holds each part, since an OS error's text differs.
			contains []string
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
				name: "UpkeepWithAPackage", dir: prime, args: []string{"testmap", "--upkeep", "alpha"},
				want: "gate testmap: --upkeep takes no package, since a pass plans every package; way forward: drop the packages to run a pass, or drop --upkeep to rebuild the named packages",
			},
			{
				name: "UpkeepBesideALivePass", dir: prime, args: []string{"testmap", "--upkeep"}, livePass: true,
				want: "gate testmap: the upkeep pass %d is running for the hub; way forward: wait for it to end, then re-run the same command",
			},
			{
				name: "UpkeepRegistrationReadFails", dir: prime, args: []string{"testmap", "--upkeep"},
				contains: []string{"gate testmap: cannot read the hub's registered processes: ", "; way forward: re-run the same command"},
				setup: func(t *testing.T) {
					runprocDir := fabricengine.RunProcDir(h.Location.HubPath)
					if err := os.RemoveAll(runprocDir); err != nil {
						t.Fatal(err)
					}
					if err := os.MkdirAll(filepath.Dir(runprocDir), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(runprocDir, []byte("not a directory\n"), 0o644); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { os.Remove(runprocDir) })
				},
			},
			{
				name: "UpkeepRegistrationWriteFails", dir: prime, args: []string{"testmap", "--upkeep"},
				contains: []string{"gate testmap: cannot register the upkeep pass, so it plans nothing: ", "; way forward: re-run the same command"},
				setup: func(t *testing.T) {
					if runtime.GOOS == "windows" || os.Geteuid() == 0 {
						t.Skip("a read-only directory stops no write on Windows or for root")
					}
					runprocDir := fabricengine.RunProcDir(h.Location.HubPath)
					ownerDir := filepath.Join(runprocDir, runproc.HubOwner)
					if err := os.RemoveAll(runprocDir); err != nil {
						t.Fatal(err)
					}
					if err := os.MkdirAll(ownerDir, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(ownerDir, 0o555); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { os.Chmod(ownerDir, 0o755) })
				},
			},
			{
				name: "NoMappedTier", dir: prime, args: []string{"testmap", "--upkeep"},
				want: "gate testmap: the hub declares neither tier0 nor tier1, so no test of a package can be mapped; way forward: declare them with lym config gate from the prime, then re-run the same command",
				setup: func(t *testing.T) {
					writeGate(t, "slots: 1\ntiers:\n  tier2: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"2\"}}\n")
					t.Cleanup(func() { writeGate(t, gateYAML("")) })
				},
			},
			{
				name: "UnknownPackage", dir: prime, args: []string{"testmap", "nosuch"},
				want: "gate testmap: \"nosuch\" names no package directory of the module at " + prime + "; way forward: name a package by its directory relative to the module root, such as internal/foo",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if tt.setup != nil {
					tt.setup(t)
				}
				want := tt.want
				if tt.livePass {
					want = fmt.Sprintf(tt.want, startLivePass(t))
				}
				var out bytes.Buffer
				code := RunCLIIn(tt.dir, &out, tt.args)
				var envelope struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
					t.Fatalf("unmarshal %q: %v", out.String(), err)
				}
				if tt.contains != nil {
					prefix, suffix := tt.contains[0], tt.contains[len(tt.contains)-1]
					if code != 1 || !strings.HasPrefix(envelope.Error, prefix) || !strings.HasSuffix(envelope.Error, suffix) {
						t.Errorf("lym gate %q = exit %d, error %q; want exit 1, error starting %q and ending %q", tt.args, code, envelope.Error, prefix, suffix)
					}
					return
				}
				if code != 1 || envelope.Error != want {
					t.Errorf("lym gate %q = exit %d, error %q; want exit 1, error %q", tt.args, code, envelope.Error, want)
				}
			})
		}
		if _, found, err := testcover.ReadExitRecord(mapsDir); err != nil || found {
			t.Errorf("ReadExitRecord after the refusals = found %v, err %v; want none, since a refusal before HEAD is read writes no exit record", found, err)
		}
		if _, found, err := testcover.ReadMap(mapsDir, "alpha"); err != nil || found {
			t.Errorf("ReadMap(alpha) after the refusals = found %v, err %v; want no map written", found, err)
		}
	})

	t.Run("CapLeavesTheRestQueued", func(t *testing.T) {
		head := gitkit.RevParse(t, prime, "HEAD")
		got := call(t, 2)
		// notests, steady and zmoved lie past the cap, and locked's lock was held at the plan.
		wantOutcome(t, got, testMapOutcome{
			Built: []string{"alpha"}, Skipped: []testMapSkip{dirtySkip, lockedSkip}, TimedOut: []string{"alpha.TestAlphaHangs"}, Queued: 4,
		})
		wantBrokenFailed(t, got)

		alpha := readMap(t, "alpha")
		wantTests := []testcover.MapTest{
			{Name: "TestAlphaHangs", Tier: 1, Outcome: testcover.OutcomeTimedOut},
			{Name: "TestAlphaOne", Tier: 0, Blocks: []int{0}},
			{Name: "TestAlphaTwo", Tier: 1, Blocks: []int{0, 1}},
		}
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
		broken := readMap(t, "broken")
		if broken.Commit != head || len(broken.Tests) != 0 || !strings.Contains(broken.BuildFailure, "the coverage build failed") {
			t.Errorf("broken map = commit %s, tests %v, failure %q; want commit %s, no test, the build failure", broken.Commit, broken.Tests, broken.BuildFailure, head)
		}
		if broken.Format == nil {
			t.Fatal("broken map has no format marker; want one on a failed build's map")
		}
		if marker := broken.Format; marker.Version != testcover.MapFormatVersion || marker.Commit != broken.Commit || !marker.BuiltAt.Equal(broken.BuiltAt) || broken.Funcs != nil {
			t.Errorf("broken map marker = %+v, funcs %v; want version %d, the map's commit %s and build time %v, no funcs",
				marker, broken.Funcs, testcover.MapFormatVersion, broken.Commit, broken.BuiltAt)
		}
		wantExitRecord(t, head, 4)
	})

	t.Run("SecondCallMapsTheRestAndKeepsAFailureAtOneHead", func(t *testing.T) {
		alphaBefore := readMap(t, "alpha")
		head := gitkit.RevParse(t, prime, "HEAD")
		got := call(t, 10)
		// zmoved's test moves HEAD, so its round is discarded and it counts as queued beside locked.
		wantOutcome(t, got, testMapOutcome{Built: []string{"notests", "steady"}, Skipped: []testMapSkip{dirtySkip, lockedSkip, movedSkip}, Queued: 2})
		if len(got.Failed) != 0 {
			t.Errorf("runTestMap() failed = %+v; want broken not rebuilt at the HEAD of its failure", got.Failed)
		}
		steady := readMap(t, "steady")
		if diff := cmp.Diff([]testcover.MapTest{{Name: "TestSteadyOne", Blocks: []int{0}}}, steady.Tests, ignoreRunFields); diff != "" {
			t.Errorf("steady map tests mismatch (-want +got):\n%s", diff)
		}
		if steady.Commit != head {
			t.Errorf("steady map commit = %s; want %s, the HEAD planned before zmoved's test landed a commit", steady.Commit, head)
		}
		if notests := readMap(t, "notests"); len(notests.Tests) != 0 || notests.BuildFailure != "" {
			t.Errorf("notests map = tests %v, failure %q; want no test and no failure", notests.Tests, notests.BuildFailure)
		}
		// alpha is current at its own commit, and its timed-out test has an outcome entry, so the plan leaves it.
		if alpha := readMap(t, "alpha"); !alpha.BuiltAt.Equal(alphaBefore.BuiltAt) {
			t.Errorf("alpha map built at %v; want it left from %v", alpha.BuiltAt, alphaBefore.BuiltAt)
		}
		if _, found, err := testcover.ReadMap(mapsDir, "zmoved"); err != nil || found {
			t.Errorf("ReadMap(zmoved) = found %v, err %v; want no map for a package whose files changed under its only round", found, err)
		}
		wantExitRecord(t, head, 2)
	})

	t.Run("HeadMoveRetriesTheFailedBuildAndMovesTheRestWhateverTheirAge", func(t *testing.T) {
		aged := readMap(t, "notests")
		aged.BuiltAt = time.Now().Add(-8 * 24 * time.Hour)
		aged.Format.BuiltAt = aged.BuiltAt
		if err := testcover.WriteMap(mapsDir, "notests", aged); err != nil {
			t.Fatal(err)
		}
		head := gitkit.RevParse(t, prime, "HEAD")
		got := call(t, 10)
		wantOutcome(t, got, testMapOutcome{Skipped: []testMapSkip{dirtySkip, lockedSkip, movedSkip}, Queued: 2})
		wantBrokenFailed(t, got)
		for _, pkg := range []string{"alpha", "notests", "steady"} {
			if m := readMap(t, pkg); m.Commit != head || m.Format == nil || m.Format.Commit != head || !m.Format.BuiltAt.Equal(m.BuiltAt) {
				t.Errorf("%s map = commit %s, marker %+v; want it moved to HEAD %s with its marker", pkg, m.Commit, m.Format, head)
			}
		}
		record, _, err := testcover.ReadExitRecord(mapsDir)
		wantMisses := map[testcover.MissReason]int{testcover.MissBuildFailure: 1, testcover.MissNoMap: 1}
		if err != nil || record.Plan.Current != 3 || record.Plan.Affected != 0 || !maps.Equal(record.Plan.Misses, wantMisses) {
			t.Errorf("exit record plan = %+v, err %v; want 3 current maps and the misses %v", record.Plan, err, wantMisses)
		}
	})

	t.Run("NamedPackageIsRebuiltWhenFreshAndLeavesTheExitRecord", func(t *testing.T) {
		before := readMap(t, "alpha")
		recordBefore, _, err := testcover.ReadExitRecord(mapsDir)
		if err != nil {
			t.Fatal(err)
		}
		got := call(t, 1, "./alpha")
		wantOutcome(t, got, testMapOutcome{Built: []string{"alpha"}, TimedOut: []string{"alpha.TestAlphaHangs"}})
		if after := readMap(t, "alpha"); !after.BuiltAt.After(before.BuiltAt) {
			t.Errorf("alpha map built at %v; want after %v", after.BuiltAt, before.BuiltAt)
		}
		recordAfter, _, err := testcover.ReadExitRecord(mapsDir)
		if diff := cmp.Diff(recordBefore, recordAfter); err != nil || diff != "" {
			t.Errorf("exit record after a call without --upkeep, err %v, mismatch (-before +after):\n%s; want it left as it was", err, diff)
		}
	})

	t.Run("AChangedFunctionReMeasuresItsTestsAndAnAddedEmptyFileMovesTheMap", func(t *testing.T) {
		edited := strings.Replace(files["alpha/alpha.go"], "return 1 }", "return 10 }", 1)
		if err := os.WriteFile(filepath.Join(prime, "alpha", "alpha.go"), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(prime, "steady", "extra.go"), []byte("package steady\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitkit.Git(t, prime, "add", "alpha/alpha.go", "steady/extra.go")
		gitkit.Git(t, prime, "commit", "-q", "-m", "edit alpha's One and add a file to steady")

		// TestAlphaOne and TestAlphaTwo covered One, and an own-file change drops TestAlphaHangs' outcome entry.
		got := call(t, 10)
		wantOutcome(t, got, testMapOutcome{
			Built: []string{"alpha"}, Skipped: []testMapSkip{dirtySkip, lockedSkip, movedSkip}, TimedOut: []string{"alpha.TestAlphaHangs"}, Queued: 2,
		})
		wantBrokenFailed(t, got)
		if alpha := readMap(t, "alpha"); alpha.Files["alpha.go"] != testcover.ContentHash([]byte(edited)) || len(alpha.Tests) != 3 {
			t.Errorf("alpha map = files %v, %d tests; want alpha.go hashed from the edited content and its three tests", alpha.Files, len(alpha.Tests))
		}
		if steady := readMap(t, "steady"); steady.Files["extra.go"] != testcover.ContentHash([]byte("package steady\n")) || len(steady.Tests) != 1 {
			t.Errorf("steady map = files %v, %d tests; want the added extra.go and its test kept", steady.Files, len(steady.Tests))
		}
	})

	t.Run("NamedPackageWithNoTrackedGoFileIsSkipped", func(t *testing.T) {
		got := call(t, 1, "ignored")
		wantOutcome(t, got, testMapOutcome{Skipped: []testMapSkip{{Package: "ignored", Cause: "no Go file tracked at HEAD"}}})
	})

	t.Run("BuildPastItsTimeoutKeepsTheMapAndIsNotRetriedAtItsHead", func(t *testing.T) {
		head := gitkit.RevParse(t, prime, "HEAD")
		before := readMap(t, "steady")
		hangEnviron := append(slices.Clone(environ), testMapHangBuildEnv+"="+module+"/steady")
		got := callWith(t, hangEnviron, 2*time.Second, 1, "steady")
		wantOutcome(t, got, testMapOutcome{})
		if len(got.Failed) != 1 || got.Failed[0].Package != "steady" || !strings.Contains(got.Failed[0].Failure, "timed out after 2s") {
			t.Fatalf("runTestMap() failed = %+v; want steady alone, naming the build timeout", got.Failed)
		}
		after := readMap(t, "steady")
		wantFailure := &testcover.MapFailure{Commit: head, Error: got.Failed[0].Failure}
		if diff := cmp.Diff(wantFailure, after.Failure); diff != "" {
			t.Errorf("steady map failure mismatch (-want +got):\n%s", diff)
		}
		if after.Commit != before.Commit || !after.BuiltAt.Equal(before.BuiltAt) || len(after.Tests) != len(before.Tests) || after.BuildFailure != "" {
			t.Errorf("steady map = commit %s, built %v, %d tests, build failure %q; want its commit %s, build time %v and %d tests kept",
				after.Commit, after.BuiltAt, len(after.Tests), after.BuildFailure, before.Commit, before.BuiltAt, len(before.Tests))
		}

		// The next pass plans at the same HEAD, so it queues no build of steady.
		again := call(t, 10)
		if slices.Contains(again.Built, "steady") || slices.ContainsFunc(again.Failed, func(failed testMapFailure) bool { return failed.Package == "steady" }) {
			t.Errorf("next pass = built %v, failed %+v; want steady neither built nor failed at the HEAD of its failure", again.Built, again.Failed)
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

// TestGateTestMapLinkedPackages runs `lym gate testmap` calls in order over one hub.
// The hub's packages link the package linked.
// Each call builds on the maps and HEAD the last one left.
// Its subtests share that state and run in sequence, so none is parallel.
func TestGateTestMapLinkedPackages(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	prime := h.PrimeWorktree()
	const module = "example.com/linking"
	// A tier's command holds the {parallel} placeholder, which the tiers digest reads before it is substituted.
	writeGate := func(t *testing.T, command string, parallel int) {
		t.Helper()
		gate := fmt.Sprintf("slots: 2\nredundancy_build_timeout_min: 1\nredundancy_test_timeout_sec: 20\ntiers:\n"+
			"  tier0: {command: %q, timeout_min: 1, counter: go-json, parallel: %d, env: {LYM_TEST_TIER: \"0\"}}\n"+
			"  tier1: {command: %q, timeout_min: 1, counter: go-json, parallel: %d, env: {LYM_TEST_TIER: \"1\"}}\n",
			command, parallel, command, parallel)
		if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(gate), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const defaultCommand = "x {parallel}"
	writeGate(t, defaultCommand, 1)

	// TestCallerSpawns lies in a tier1 file and runs os.Executable, so the spawn scan marks it out of process.
	// TestCallerUnresolved lies there too and calls a local function value, which the scan cannot resolve.
	// linked holds a file in a subdirectory, as a go:embed target does, and a nested package its map leaves out.
	// It holds a nested module, which owns its go.mod.
	// It also holds files under testdata, a dot directory and an underscore directory.
	// No package owns those files, so they stay linked's own.
	files := map[string]string{
		"go.mod":                "module " + module + "\n\ngo 1.21\n",
		"go.sum":                "example.com/dependency v1.0.0 h1:placeholder=\n",
		"caller/caller.go":      testMapSource("caller"),
		"caller/caller_test.go": testMapTestFile("caller", map[string]int{"TestCallerCalls": 0, "TestCallerElsewhere": 0}),
		"caller/spawn_tier1_test.go": "//go:build tier1\n\npackage caller\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n\n" +
			"func TestCallerSpawns(t *testing.T) {\n\ttiergate.Require(t, 1)\n\t_, _ = os.Executable()\n}\n\n" +
			"func TestCallerUnresolved(t *testing.T) {\n\ttiergate.Require(t, 1)\n\tcall := func() {}\n\tcall()\n}\n",
		"caller/testdata/fixture.txt": "fixture\n",
		"dirtier/dirtier.go":          testMapSource("dirtier"),
		"dirtier/dirtier_test.go":     testMapTestFile("dirtier", map[string]int{"TestDirtierDirties": 0, "TestDirtierLandsLinked": 0}),
		"follower/follower.go":        testMapSource("follower"),
		"follower/follower_test.go":   testMapTestFile("follower", map[string]int{"TestFollowerCalls": 0}),
		"linked/linked.go":            testMapSource("linked"),
		"linked/assets/data.txt":      "embedded\n",
		"linked/inner/inner.go":       "package inner\n",
		"linked/modnest/go.mod":       "module example.com/modnest\n\ngo 1.21\n",
		"linked/testdata/only.go":     "package only\n",
		"linked/_hidden/hidden.go":    "package hidden\n",
		"linked/.dotdir/dot.go":       "package dot\n",
	}
	plankit.WriteTree(t, prime, files)
	gitkit.Git(t, prime, "add", "go.mod", "go.sum", "caller", "dirtier", "follower", "linked")
	gitkit.Git(t, prime, "commit", "-q", "-m", "seed the linking module")

	mapsDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
	standIn, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	builds := filepath.Join(t.TempDir(), "builds")
	environ := append(os.Environ(), helperEnv+"="+helperRoleTestMapGo, testMapModuleEnv+"="+module,
		testMapLinksEnv+"=caller=linked,dirtier=linked,follower=linked", testMapCoverBuildsEnv+"="+builds)
	callMax := func(t *testing.T, env []string, maxPackages int, packages ...string) testMapOutcome {
		t.Helper()
		return callTestMap(t, prime, standIn, env, time.Minute, maxPackages, packages...)
	}
	call := func(t *testing.T, env []string, packages ...string) testMapOutcome {
		t.Helper()
		return callMax(t, env, 10, packages...)
	}
	// withEnv returns the call environment with the variable name set to value.
	withEnv := func(name, value string) []string {
		return append(slices.Clone(environ), name+"="+value)
	}
	wantOutcome := func(t *testing.T, got, want testMapOutcome) {
		t.Helper()
		if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("runTestMap() mismatch (-want +got):\n%s", diff)
		}
	}
	wantLeft := func(t *testing.T, pkg string, before testcover.Map) {
		t.Helper()
		if after := readTestMap(t, mapsDir, pkg); !after.BuiltAt.Equal(before.BuiltAt) {
			t.Errorf("%s map built at %v; want it left from %v", pkg, after.BuiltAt, before.BuiltAt)
		}
	}
	hash := func(content string) string { return testcover.ContentHash([]byte(content)) }
	// rebuiltInnerStamp rebuilds the map of linked/inner, which has no test and so needs no build, and returns its stamp.
	rebuiltInnerStamp := func(t *testing.T) testcover.EnvironmentStamp {
		t.Helper()
		wantOutcome(t, call(t, environ, "linked/inner"), testMapOutcome{Built: []string{"linked/inner"}})
		marker := readTestMap(t, mapsDir, "linked/inner").Format
		if marker == nil {
			t.Fatal("linked/inner map has no format marker")
		}
		return marker.Environment
	}

	t.Run("TheMapRecordsLinkedFunctionsAndFilesTheSpawnVerdictAndAMarker", func(t *testing.T) {
		head := gitkit.RevParse(t, prime, "HEAD")
		wantOutcome(t, call(t, environ), testMapOutcome{Built: []string{"caller", "dirtier", "follower", "linked", "linked/inner"}})

		caller := readTestMap(t, mapsDir, "caller")
		wantFuncs := []testcover.FuncKey{{Package: module + "/linked", File: "linked.go", Func: "One"}}
		if diff := cmp.Diff(wantFuncs, caller.Funcs); diff != "" {
			t.Errorf("caller map funcs mismatch (-want +got):\n%s", diff)
		}
		// The spawn-marked tests are recorded with their verdict and never run.
		wantTests := []testcover.MapTest{
			{Name: "TestCallerCalls", Blocks: []int{0}, Funcs: []int{0}, Commit: head},
			{Name: "TestCallerElsewhere", Funcs: []int{0}, Commit: head},
			{Name: "TestCallerSpawns", Tier: 1, Spawn: testcover.SpawnVerdict{OutOfProcess: true}, Commit: head},
			{Name: "TestCallerUnresolved", Tier: 1, Spawn: testcover.SpawnVerdict{Unresolved: true}, Commit: head},
		}
		if diff := cmp.Diff(wantTests, caller.Tests, cmpopts.EquateEmpty(), cmpopts.IgnoreFields(testcover.MapTest{}, "FuncHash", "Seconds", "MeasuredAt")); diff != "" {
			t.Errorf("caller map tests mismatch (-want +got):\n%s", diff)
		}
		wantLinked := map[string]map[string]string{module + "/linked": {
			"linked.go":         hash(files["linked/linked.go"]),
			"assets/data.txt":   hash("embedded\n"),
			"testdata/only.go":  hash(files["linked/testdata/only.go"]),
			"_hidden/hidden.go": hash(files["linked/_hidden/hidden.go"]),
			".dotdir/dot.go":    hash(files["linked/.dotdir/dot.go"]),
		}}
		if diff := cmp.Diff(wantLinked, caller.Linked); diff != "" {
			t.Errorf("caller map linked files mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(map[string]string{"testdata/fixture.txt": hash("fixture\n")}, caller.OtherFiles); diff != "" {
			t.Errorf("caller map other files mismatch (-want +got):\n%s", diff)
		}
		if caller.Format == nil {
			t.Fatal("caller map has no format marker")
		}
		wantFormat := &testcover.MapFormat{
			Version: testcover.MapFormatVersion, Commit: head, BuiltAt: caller.BuiltAt,
			Environment: testcover.EnvironmentStamp{
				GoVersion: testMapGoVersion, GOOS: testMapGOOS, GOARCH: testMapGOARCH,
				GoModHash: hash(files["go.mod"]), GoSumHash: hash(files["go.sum"]),
			},
		}
		if diff := cmp.Diff(wantFormat, caller.Format, cmpopts.IgnoreFields(testcover.EnvironmentStamp{}, "TiersDigest", "HubEnvDigest")); diff != "" || caller.Commit != head {
			t.Errorf("caller map commit = %s, format mismatch (-want +got):\n%s; want commit %s", caller.Commit, diff, head)
		}
		if stamp := caller.Format.Environment; stamp.TiersDigest == "" || stamp.HubEnvDigest == "" {
			t.Errorf("caller map digests = tiers %q, hub env %q; want both set", stamp.TiersDigest, stamp.HubEnvDigest)
		}
		recorded, err := os.ReadFile(builds)
		if err != nil {
			t.Fatal(err)
		}
		if want := module + "/caller " + module + "/caller," + module + "/linked"; !slices.Contains(strings.Split(string(recorded), "\n"), want) {
			t.Errorf("coverage builds = %q; want the line %q", recorded, want)
		}
	})

	t.Run("ALinkedCommentChangeMovesTheMapToHeadWithEveryEntry", func(t *testing.T) {
		before := readTestMap(t, mapsDir, "caller")
		commented := testMapSource("linked") + "// edited\n"
		plankit.WriteTree(t, prime, map[string]string{"linked/linked.go": commented})
		gitkit.Git(t, prime, "commit", "-q", "-am", "edit the linked package's comments")
		head := gitkit.RevParse(t, prime, "HEAD")
		wantOutcome(t, call(t, environ), testMapOutcome{})
		after := readTestMap(t, mapsDir, "caller")
		if after.Commit != head || after.Format == nil || after.Format.Commit != head || !after.Format.BuiltAt.Equal(after.BuiltAt) {
			t.Errorf("caller map = commit %s, marker %+v; want it and its marker at HEAD %s", after.Commit, after.Format, head)
		}
		if diff := cmp.Diff(before.Tests, after.Tests); diff != "" {
			t.Errorf("caller map tests mismatch (-before +after):\n%s; want every entry kept", diff)
		}
		if got := after.Linked[module+"/linked"]["linked.go"]; got != hash(commented) {
			t.Errorf("caller map linked.go hash = %s; want the hash at HEAD, %s", got, hash(commented))
		}
		record, _, err := testcover.ReadExitRecord(mapsDir)
		if err != nil || record.Plan.Current != 5 || record.Plan.Affected != 0 || len(record.Plan.Misses) != 0 {
			t.Errorf("exit record plan = %+v, err %v; want all 5 maps current", record.Plan, err)
		}
	})

	t.Run("ALinkedBodyChangeReMeasuresOnlyTheTestsThatCoveredIt", func(t *testing.T) {
		before := readTestMap(t, mapsDir, "caller")
		plankit.WriteTree(t, prime, map[string]string{"linked/linked.go": strings.Replace(testMapSource("linked"), "return 1 }", "return 100 }", 1)})
		gitkit.Git(t, prime, "commit", "-q", "-am", "change the body of the linked package's One")
		head := gitkit.RevParse(t, prime, "HEAD")
		runs := filepath.Join(t.TempDir(), "runs")
		wantOutcome(t, call(t, withEnv(testMapLymSeenEnv, runs)), testMapOutcome{Built: []string{"caller", "follower"}})
		// Each coverage run writes one line: caller's two tests that covered One and follower's one.
		if data, err := os.ReadFile(runs); err != nil || bytes.Count(data, []byte("\n")) != 3 {
			t.Errorf("coverage runs = %q, %v; want 3", data, err)
		}
		caller := readTestMap(t, mapsDir, "caller")
		if len(caller.Tests) != len(before.Tests) {
			t.Fatalf("caller map tests = %+v; want the %d entries it held", caller.Tests, len(before.Tests))
		}
		for _, test := range caller.Tests {
			if remeasured, coveredOne := test.Commit == head, test.Name == "TestCallerCalls" || test.Name == "TestCallerElsewhere"; remeasured != coveredOne {
				t.Errorf("caller entry %s measured at %s; want it re-measured at HEAD %s exactly when it covered One", test.Name, test.Commit, head)
			}
		}
		record, _, err := testcover.ReadExitRecord(mapsDir)
		if err != nil || record.Plan.Affected != 2 || record.Plan.Current != 3 {
			t.Errorf("exit record plan = %+v, err %v; want caller and follower affected and the rest current", record.Plan, err)
		}
	})

	t.Run("ANewTestIsMeasuredAloneAndAKeptEntryTheBuildCannotPlaceIsReMeasured", func(t *testing.T) {
		stale := readTestMap(t, mapsDir, "caller")
		const staleHash = "a block text the package no longer holds"
		stale.Blocks[0].Hash = staleHash
		if err := testcover.WriteMap(mapsDir, "caller", stale); err != nil {
			t.Fatal(err)
		}
		plankit.WriteTree(t, prime, map[string]string{
			"caller/caller_test.go": testMapTestFile("caller", map[string]int{"TestCallerAdded": 0, "TestCallerCalls": 0, "TestCallerElsewhere": 0}),
		})
		gitkit.Git(t, prime, "commit", "-q", "-am", "add a test to caller")
		runs := filepath.Join(t.TempDir(), "runs")
		// TestCallerCalls covers the block whose key the fresh build lacks, so the self-check re-runs it beside the new test.
		wantOutcome(t, call(t, withEnv(testMapLymSeenEnv, runs)), testMapOutcome{Built: []string{"caller"}, SelfCheckMismatches: 1})
		if data, err := os.ReadFile(runs); err != nil || bytes.Count(data, []byte("\n")) != 2 {
			t.Errorf("coverage runs = %q, %v; want 2: TestCallerAdded and TestCallerCalls", data, err)
		}
		caller := readTestMap(t, mapsDir, "caller")
		if slices.ContainsFunc(caller.Blocks, func(key testcover.BlockKey) bool { return key.Hash == staleHash }) {
			t.Errorf("caller map blocks = %+v; want the stale key replaced by the fresh build's", caller.Blocks)
		}
		added := slices.IndexFunc(caller.Tests, func(test testcover.MapTest) bool { return test.Name == "TestCallerAdded" })
		if added < 0 || !slices.Equal(caller.Tests[added].Blocks, []int{0}) {
			t.Errorf("caller map tests = %+v; want TestCallerAdded covering block 0", caller.Tests)
		}
	})

	t.Run("TheAuditReMeasuresTheOldestEntryAndCountsADifferentResult", func(t *testing.T) {
		old := readTestMap(t, mapsDir, "follower")
		measuredLong := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		old.Tests[0].MeasuredAt = measuredLong
		old.Tests[0].Blocks = nil
		if err := testcover.WriteMap(mapsDir, "follower", old); err != nil {
			t.Fatal(err)
		}
		code, out := runTestMapRequest(prime, testMapRequest{
			goBinary: standIn, maxPackages: 10, environ: environ, auditTests: 1, upkeep: true,
			secondUnit: 150 * time.Millisecond, minuteUnit: time.Minute, grace: time.Second,
			waitBase: testMapTestWait, waitCeiling: 5 * testMapTestWait, yieldPoll: testMapTestWait,
		})
		// Every map stands at HEAD with no gap, so the queue is empty and the round re-measures the oldest entry alone.
		wantOutcome(t, decodeTestMap(t, code, out), testMapOutcome{Built: []string{"follower"}, AuditMismatches: 1})
		follower := readTestMap(t, mapsDir, "follower")
		if len(follower.Tests) != 1 || !slices.Equal(follower.Tests[0].Blocks, []int{0}) || !follower.Tests[0].MeasuredAt.After(measuredLong) {
			t.Errorf("follower map tests = %+v; want TestFollowerCalls re-measured, covering block 0", follower.Tests)
		}
	})

	t.Run("ADirtyLinkedPackageLeavesTheMapAsItWasAndCountsNothingAgainstTheCap", func(t *testing.T) {
		callerBefore := readTestMap(t, mapsDir, "caller")
		followerBefore := readTestMap(t, mapsDir, "follower")
		notes := filepath.Join(prime, "linked", "notes.txt")
		if err := os.WriteFile(notes, []byte("notes\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		// With a cap of one, the two skipped packages must leave the slot to linked/inner.
		// No dirty file keeps linked/inner from building.
		got := callMax(t, environ, 1, "caller", "follower", "linked/inner")
		if err := os.Remove(notes); err != nil {
			t.Fatal(err)
		}
		const cause = "modified or untracked file linked/notes.txt"
		wantOutcome(t, got, testMapOutcome{
			Built:   []string{"linked/inner"},
			Skipped: []testMapSkip{{Package: "caller", Cause: cause}, {Package: "follower", Cause: cause}},
		})
		wantLeft(t, "caller", callerBefore)
		wantLeft(t, "follower", followerBefore)
	})

	t.Run("ALinkedFileDirtiedUnderTheTestsWritesNoMapAndIsSeenAfreshByALaterPackage", func(t *testing.T) {
		dirtierBefore := readTestMap(t, mapsDir, "dirtier")
		followerBefore := readTestMap(t, mapsDir, "follower")
		scratch := filepath.Join(prime, "linked", "scratch.txt")
		got := call(t, withEnv(testMapDirtyEnv, scratch), "dirtier", "follower")
		if err := os.Remove(scratch); err != nil {
			t.Fatal(err)
		}
		// The file does not exist when the call starts.
		// So follower, walked after dirtier, skips only on a status read taken before its build.
		wantOutcome(t, got, testMapOutcome{Skipped: []testMapSkip{
			{Package: "dirtier", Cause: "modified or untracked file linked/scratch.txt after its tests"},
			{Package: "follower", Cause: "modified or untracked file linked/scratch.txt"},
		}})
		wantLeft(t, "dirtier", dirtierBefore)
		wantLeft(t, "follower", followerBefore)
	})

	t.Run("ASkipAfterTheTestsCountsAgainstTheCap", func(t *testing.T) {
		dirtierBefore := readTestMap(t, mapsDir, "dirtier")
		followerBefore := readTestMap(t, mapsDir, "follower")
		scratch := filepath.Join(prime, "linked", "scratch.txt")
		code, out := runTestMapIn(prime, standIn, withEnv(testMapDirtyEnv, scratch), time.Minute, 1, "dirtier", "follower")
		got := decodeTestMap(t, code, out)
		var counts testMapVerbCounts
		if err := json.Unmarshal(out, &counts); err != nil {
			t.Fatalf("unmarshal %q: %v", out, err)
		}
		if err := os.Remove(scratch); err != nil {
			t.Fatal(err)
		}
		if len(counts.Packages) != 0 {
			t.Errorf("packages = %+v; want none, since the skipped package reports no build or run seconds", counts.Packages)
		}
		// With a cap of one, dirtier's skip after its tests uses the slot, so follower is never tried.
		wantOutcome(t, got, testMapOutcome{
			Skipped: []testMapSkip{{Package: "dirtier", Cause: "modified or untracked file linked/scratch.txt after its tests"}},
			Queued:  1,
		})
		wantLeft(t, "dirtier", dirtierBefore)
		wantLeft(t, "follower", followerBefore)
	})

	// dirtierEntries returns the names of the entries of dirtier's map.
	dirtierEntries := func(t *testing.T) []string {
		t.Helper()
		var names []string
		for _, test := range readTestMap(t, mapsDir, "dirtier").Tests {
			names = append(names, test.Name)
		}
		return names
	}

	t.Run("ALinkedFileChangedUnderARoundDiscardsItKeepsTheRoundsBeforeAndEndsTheCall", func(t *testing.T) {
		followerBefore := readTestMap(t, mapsDir, "follower")
		head := gitkit.RevParse(t, prime, "HEAD")
		got := call(t, withEnv(testMapLandEnv, "linked"), "dirtier", "follower")
		// dirtier's rounds run in name order, so TestDirtierDirties is written before TestDirtierLandsLinked moves HEAD.
		// follower's job comes after the move, so the call ends there and counts it queued beside dirtier.
		wantOutcome(t, got, testMapOutcome{
			Built:   []string{"dirtier"},
			Skipped: []testMapSkip{{Package: "dirtier", Cause: "files changed under its tests: linked/linked.go"}},
			Queued:  2,
		})
		dirtier := readTestMap(t, mapsDir, "dirtier")
		wantTests := []testcover.MapTest{{Name: "TestDirtierDirties", Blocks: []int{0}, Commit: head}}
		if diff := cmp.Diff(wantTests, dirtier.Tests, cmpopts.EquateEmpty(), cmpopts.IgnoreFields(testcover.MapTest{}, "FuncHash", "Seconds", "MeasuredAt")); diff != "" {
			t.Errorf("dirtier map tests mismatch (-want +got):\n%s", diff)
		}
		wantLeft(t, "follower", followerBefore)
	})

	t.Run("ALinkedFileAddedOrRemovedUnderARoundDiscardsIt", func(t *testing.T) {
		const added = "landed.txt"
		// The rows run in order: the second removes the file the first adds.
		tests := []struct {
			name string
			env  []string
		}{
			{"Added", append(withEnv(testMapLandEnv, "linked"), testMapLandFileEnv+"="+added)},
			{"Removed", append(withEnv(testMapLandEnv, "linked"), testMapRemoveFileEnv+"="+added)},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := call(t, tt.env, "dirtier")
				wantOutcome(t, got, testMapOutcome{
					Built:   []string{"dirtier"},
					Skipped: []testMapSkip{{Package: "dirtier", Cause: "files changed under its tests: linked/" + added}},
					Queued:  1,
				})
				if names := dirtierEntries(t); !slices.Equal(names, []string{"TestDirtierDirties"}) {
					t.Errorf("dirtier map entries = %v; want TestDirtierDirties alone, the round before the change", names)
				}
			})
		}
	})

	t.Run("TheTiersDigestIgnoresTheParallelCountAndFollowsTheCommand", func(t *testing.T) {
		t.Cleanup(func() { writeGate(t, defaultCommand, 1) })
		digestUnder := func(command string, parallel int) string {
			t.Helper()
			writeGate(t, command, parallel)
			return rebuiltInnerStamp(t).TiersDigest
		}
		first := digestUnder(defaultCommand, 1)
		if again := digestUnder(defaultCommand, 2); first == "" || again != first {
			t.Errorf("tiers digest at parallel 2 = %q; want %q, non-empty and unchanged by the parallel count", again, first)
		}
		if changed := digestUnder("y {parallel}", 1); changed == first {
			t.Errorf("tiers digest after a command change = %q; want it to differ from %q", changed, first)
		}
	})

	t.Run("TheHubEnvDigestFollowsTheHubsVariables", func(t *testing.T) {
		before := rebuiltInnerStamp(t).HubEnvDigest
		envFile := configengine.ConfigFile(h.BoardDir(), "env")
		if err := os.WriteFile(envFile, []byte("vars:\n  TESTMAP_GREETING: hello\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Remove(envFile) })
		if after := rebuiltInnerStamp(t).HubEnvDigest; before == "" || after == before {
			t.Errorf("hub env digest after setting a variable = %q; want it non-empty and different from %q", after, before)
		}
	})

	t.Run("AGoListFailureKeepsTheMapsEntriesAndRecordsTheFailureWithHead", func(t *testing.T) {
		before := readTestMap(t, mapsDir, "caller")
		head := gitkit.RevParse(t, prime, "HEAD")
		got := call(t, withEnv(testMapFailListEnv, "1"), "caller")
		if len(got.Built) != 0 || len(got.Failed) != 1 || got.Failed[0].Package != "caller" || !strings.Contains(got.Failed[0].Failure, "list linked packages") {
			t.Fatalf("runTestMap() = built %v, failed %+v; want caller alone failed, naming the list failure", got.Built, got.Failed)
		}
		failed := readTestMap(t, mapsDir, "caller")
		if diff := cmp.Diff(&testcover.MapFailure{Commit: head, Error: got.Failed[0].Failure}, failed.Failure); diff != "" {
			t.Errorf("caller map failure mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(before.Tests, failed.Tests); diff != "" || failed.Commit != before.Commit || failed.BuildFailure != "" || !cmp.Equal(failed.Format, before.Format) {
			t.Errorf("caller map = commit %s, build failure %q, format %+v, tests mismatch (-before +after):\n%s; want the map kept as it was",
				failed.Commit, failed.BuildFailure, failed.Format, diff)
		}
	})

	t.Run("ASpawnScanFailureWritesAFailedMapWithTheMarker", func(t *testing.T) {
		plankit.WriteTree(t, prime, map[string]string{
			"garbled/garbled.go":      "package garbled\n\nfunc (\n",
			"garbled/garbled_test.go": testMapTestFile("garbled", map[string]int{"TestGarbledOne": 0}),
		})
		gitkit.Git(t, prime, "add", "garbled")
		gitkit.Git(t, prime, "commit", "-q", "-m", "add a package the spawn scan cannot parse")
		got := call(t, environ, "garbled")
		if len(got.Built) != 0 || len(got.Failed) != 1 || got.Failed[0].Package != "garbled" || !strings.Contains(got.Failed[0].Failure, "spawn scan") {
			t.Fatalf("runTestMap() = built %v, failed %+v; want garbled alone failed, naming the spawn scan", got.Built, got.Failed)
		}
		if failed := readTestMap(t, mapsDir, "garbled"); failed.BuildFailure != got.Failed[0].Failure || failed.Format == nil {
			t.Errorf("garbled map = failure %q, format %+v; want the spawn scan failure and a marker", failed.BuildFailure, failed.Format)
		}
	})

	t.Run("AGoEnvFailureEndsTheWalk", func(t *testing.T) {
		linkedBefore := readTestMap(t, mapsDir, "linked")
		innerBefore := readTestMap(t, mapsDir, "linked/inner")
		code, out := runTestMapIn(prime, standIn, withEnv(testMapFailGoEnvEnv, "1"), time.Minute, 10, "linked", "linked/inner")
		var envelope struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(out, &envelope); err != nil {
			t.Fatalf("unmarshal %q: %v", out, err)
		}
		if code != 1 || !strings.Contains(envelope.Error, "read the Go toolchain's environment") {
			t.Errorf("runTestMap() = exit %d, error %q; want exit 1, naming the toolchain environment read", code, envelope.Error)
		}
		wantLeft(t, "linked", linkedBefore)
		wantLeft(t, "linked/inner", innerBefore)
	})
}

// testMapVerbCounts holds the plan and packages keys of a `lym gate testmap` envelope, which testMapOutcome leaves out.
type testMapVerbCounts struct {
	Plan     testcover.PlanCounts       `json:"plan"`
	Packages []testcover.PackageMeasure `json:"packages"`
}

// TestGateTestMapUpkeep runs `lym gate testmap` calls in order over one hub whose module holds a package per upkeep case.
// Each call builds on the maps and HEAD the last one left.
// Its subtests share that state and run in sequence, so none is parallel.
func TestGateTestMapUpkeep(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	prime := h.PrimeWorktree()
	location, err := lymcwd.ResolveWorktree(prime)
	if err != nil {
		t.Fatal(err)
	}
	const module = "example.com/upkeep"
	// tier0's parallel of two lets two tests that declare Parallel run side by side.
	gate := "slots: 2\nredundancy_build_timeout_min: 1\nredundancy_test_timeout_sec: 20\ntiers:\n" +
		"  tier0: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"0\"}, parallel: 2}\n" +
		"  tier1: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"1\"}}\n"
	if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(gate), 0o644); err != nil {
		t.Fatal(err)
	}

	packageTests := map[string]map[string]int{
		"caller":   {"TestCallerCalls": 0},
		"headmove": {"TestHeadmoveOne": 0},
		"linked":   nil,
		"outcomes": {"TestOutcomesPass": 0, "TestOutcomesFails": 0, "TestOutcomesSkips": 0, "TestOutcomesElsewhere": 0},
	}
	files := map[string]string{"go.mod": "module " + module + "\n\ngo 1.21\n"}
	for pkg, tiers := range packageTests {
		files[pkg+"/"+pkg+".go"] = testMapSource(pkg)
		if tiers != nil {
			files[pkg+"/"+pkg+"_test.go"] = testMapTestFile(pkg, tiers)
		}
	}
	plankit.WriteTree(t, prime, files)
	gitkit.Git(t, prime, append([]string{"add", "go.mod"}, slices.Sorted(maps.Keys(packageTests))...)...)
	gitkit.Git(t, prime, "commit", "-q", "-m", "seed the upkeep module")

	mapsDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
	standIn, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environ := append(os.Environ(), helperEnv+"="+helperRoleTestMapGo, testMapModuleEnv+"="+module, testMapLinksEnv+"=caller=linked")
	withEnv := func(name, value string) []string {
		return append(slices.Clone(environ), name+"="+value)
	}
	request := func(env []string, maxPackages, auditTests int, packages ...string) testMapRequest {
		return testMapRequest{
			goBinary: standIn, maxPackages: maxPackages, packages: packages, environ: env, auditTests: auditTests, upkeep: len(packages) == 0,
			secondUnit: 150 * time.Millisecond, minuteUnit: time.Minute, grace: time.Second,
			waitBase: testMapTestWait, waitCeiling: 5 * testMapTestWait, yieldPoll: testMapTestWait,
		}
	}
	// call runs the verb under a minute's bound, so a call that waits for a slot it must not take fails the test.
	call := func(t *testing.T, env []string, maxPackages, auditTests int, packages ...string) (testMapOutcome, testMapVerbCounts) {
		t.Helper()
		ctx, cancel := context.WithTimeout(lymcwd.WithCwd(context.Background(), prime), time.Minute)
		defer cancel()
		var out bytes.Buffer
		code := runTestMap(ctx, &out, request(env, maxPackages, auditTests, packages...))
		var counts testMapVerbCounts
		if err := json.Unmarshal(out.Bytes(), &counts); err != nil {
			t.Fatalf("unmarshal %q: %v", out.Bytes(), err)
		}
		return decodeTestMap(t, code, out.Bytes()), counts
	}
	readMap := func(t *testing.T, pkg string) testcover.Map {
		t.Helper()
		return readTestMap(t, mapsDir, pkg)
	}
	writeMap := func(t *testing.T, pkg string, m testcover.Map) {
		t.Helper()
		if err := testcover.WriteMap(mapsDir, pkg, m); err != nil {
			t.Fatal(err)
		}
	}
	wantOutcome := func(t *testing.T, got, want testMapOutcome) {
		t.Helper()
		if diff := cmp.Diff(want, got, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("runTestMap() mismatch (-want +got):\n%s", diff)
		}
	}
	entryIndex := func(m testcover.Map, name string) int {
		return slices.IndexFunc(m.Tests, func(test testcover.MapTest) bool { return test.Name == name })
	}
	ignoreRunFields := cmpopts.IgnoreFields(testcover.MapTest{}, "FuncHash", "Seconds", "Commit", "MeasuredAt")
	const extraVariable = "\nvar Extra = 1\n"

	t.Run("EveryOutcomeGetsItsEntryWithNoCoverageAndTheVerbPrintsItsPlanAndPackages", func(t *testing.T) {
		got, counts := call(t, environ, 10, 0)
		wantOutcome(t, got, testMapOutcome{Built: []string{"caller", "headmove", "linked", "outcomes"}})
		wantTests := []testcover.MapTest{
			{Name: "TestOutcomesElsewhere", Outcome: testcover.OutcomeNoCoverage},
			{Name: "TestOutcomesFails", Outcome: testcover.OutcomeFailed},
			{Name: "TestOutcomesPass", Blocks: []int{0}},
			{Name: "TestOutcomesSkips", Outcome: testcover.OutcomeSkipped},
		}
		if diff := cmp.Diff(wantTests, readMap(t, "outcomes").Tests, cmpopts.EquateEmpty(), ignoreRunFields); diff != "" {
			t.Errorf("outcomes map tests mismatch (-want +got):\n%s", diff)
		}
		var measured []string
		for _, measure := range counts.Packages {
			measured = append(measured, measure.Package)
		}
		if want := []string{"caller", "headmove", "outcomes"}; !slices.Equal(measured, want) {
			t.Errorf("packages measured = %v; want %v, the packages with a mapped test", measured, want)
		}
		if counts.Plan.Misses[testcover.MissNoMap] != len(got.Built) || counts.Plan.Current != 0 || counts.Plan.Affected != 0 {
			t.Errorf("plan = %+v; want every package missed for having no map", counts.Plan)
		}
	})

	t.Run("AMissOnAMapWithEntriesRebuildsItFromNothing", func(t *testing.T) {
		before := readMap(t, "headmove")
		ghost := testcover.MapTest{Name: "TestHeadmoveGhost", Blocks: []int{0}, Commit: before.Commit, MeasuredAt: before.BuiltAt}
		before.Tests = slices.Insert(before.Tests, 0, ghost)
		writeMap(t, "headmove", before)
		plankit.WriteTree(t, prime, map[string]string{"headmove/headmove.go": testMapSource("headmove") + extraVariable})
		gitkit.Git(t, prime, "commit", "-q", "-am", "add a package variable to headmove")
		head := gitkit.RevParse(t, prime, "HEAD")

		got, counts := call(t, environ, 10, 0)
		wantOutcome(t, got, testMapOutcome{Built: []string{"headmove"}})
		if counts.Plan.Misses[testcover.MissNonBody] != 1 || counts.Plan.Current != len(packageTests)-1 {
			t.Errorf("plan = %+v; want headmove missed for a non-body change and the other maps current", counts.Plan)
		}
		after := readMap(t, "headmove")
		var names, older []string
		for _, test := range after.Tests {
			names = append(names, test.Name)
			if test.Commit != head {
				older = append(older, test.Name)
			}
		}
		if after.Commit != head || !slices.Equal(names, []string{"TestHeadmoveOne"}) || len(older) != 0 {
			t.Errorf("headmove map = commit %s, entries %v, entries not measured at HEAD %v; want HEAD %s and TestHeadmoveOne alone, measured there",
				after.Commit, names, older, head)
		}
	})

	t.Run("AKeptEntryNamingALinkedFunctionTheBuildLacksIsReMeasured", func(t *testing.T) {
		stale := readMap(t, "caller")
		stale.Funcs = append(stale.Funcs, testcover.FuncKey{Package: module + "/linked", File: "linked.go", Func: "Zzz"})
		stale.Tests[0].Funcs = append(stale.Tests[0].Funcs, len(stale.Funcs)-1)
		writeMap(t, "caller", stale)
		plankit.WriteTree(t, prime, map[string]string{
			"caller/caller_test.go": testMapTestFile("caller", map[string]int{"TestCallerAdded": 0, "TestCallerCalls": 0}),
		})
		gitkit.Git(t, prime, "commit", "-q", "-am", "add a test to caller")
		head := gitkit.RevParse(t, prime, "HEAD")

		runs := filepath.Join(t.TempDir(), "runs")
		got, _ := call(t, withEnv(testMapLymSeenEnv, runs), 10, 0)
		wantOutcome(t, got, testMapOutcome{Built: []string{"caller"}, SelfCheckMismatches: 1})
		if data, err := os.ReadFile(runs); err != nil || bytes.Count(data, []byte("\n")) != 2 {
			t.Errorf("coverage runs = %q, %v; want 2: TestCallerAdded and TestCallerCalls", data, err)
		}
		caller := readMap(t, "caller")
		if slices.ContainsFunc(caller.Funcs, func(key testcover.FuncKey) bool { return key.Func == "Zzz" }) {
			t.Errorf("caller map funcs = %+v; want the function the build lacks gone", caller.Funcs)
		}
		if i := entryIndex(caller, "TestCallerCalls"); i < 0 || caller.Tests[i].Commit != head {
			t.Errorf("caller map tests = %+v; want TestCallerCalls re-measured at HEAD %s", caller.Tests, head)
		}
	})

	t.Run("APassWhoseQueuedTestsLeaveNoProfileStillChecksTheKeptEntries", func(t *testing.T) {
		stale := readMap(t, "caller")
		i := entryIndex(stale, "TestCallerCalls")
		stale.Funcs = append(stale.Funcs, testcover.FuncKey{Package: module + "/linked", File: "linked.go", Func: "Zzz"})
		stale.Tests[i].Funcs = append(stale.Tests[i].Funcs, len(stale.Funcs)-1)
		writeMap(t, "caller", stale)
		plankit.WriteTree(t, prime, map[string]string{
			"caller/caller_test.go": testMapTestFile("caller", map[string]int{"TestCallerAdded": 0, "TestCallerCalls": 0, "TestCallerFails": 0}),
		})
		gitkit.Git(t, prime, "commit", "-q", "-am", "add a failing test to caller")

		// The one queued test fails and writes no profile, so the self-check reads one from a kept entry it re-runs.
		got, _ := call(t, environ, 10, 0)
		wantOutcome(t, got, testMapOutcome{Built: []string{"caller"}, SelfCheckMismatches: 1})
		caller := readMap(t, "caller")
		if slices.ContainsFunc(caller.Funcs, func(key testcover.FuncKey) bool { return key.Func == "Zzz" }) {
			t.Errorf("caller map funcs = %+v; want the function the build lacks gone", caller.Funcs)
		}
		if i := entryIndex(caller, "TestCallerFails"); i < 0 || caller.Tests[i].Outcome != testcover.OutcomeFailed {
			t.Errorf("caller map tests = %+v; want TestCallerFails recorded as failed", caller.Tests)
		}
	})

	t.Run("TheAuditRunsOnlyWithNothingQueuedAndASlotFreeAndCountsOnlyADifferentResult", func(t *testing.T) {
		measuredLong := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
		old := readMap(t, "caller")
		i := entryIndex(old, "TestCallerCalls")
		old.Tests[i].MeasuredAt = measuredLong
		old.Tests[i].Blocks = nil
		writeMap(t, "caller", old)
		wantUntouched := func(t *testing.T, what string) {
			t.Helper()
			caller := readMap(t, "caller")
			if i := entryIndex(caller, "TestCallerCalls"); i < 0 || !caller.Tests[i].MeasuredAt.Equal(measuredLong) || len(caller.Tests[i].Blocks) != 0 {
				t.Errorf("caller map tests = %+v; want the entry the audit would count left as it was %s", caller.Tests, what)
			}
		}

		// The plan moves every map to the new HEAD and holds no slot.
		// The test holds one of the two slots, so a pass may take none, and the call returns with no audit.
		plankit.WriteTree(t, prime, map[string]string{"landed.txt": "landed\n"})
		gitkit.Git(t, prime, "add", "landed.txt")
		gitkit.Git(t, prime, "commit", "-q", "-m", "land a file outside every package")
		head := gitkit.RevParse(t, prime, "HEAD")
		lease, err := hubgeom.GateSlots(location).Acquire(context.Background(), gateslot.Holder{Worktree: location.WorktreePath(), Site: "an enclosing gate run"})
		if err != nil {
			t.Fatal(err)
		}
		got, counts := call(t, environ, 10, 1)
		if err := lease.Release(); err != nil {
			t.Fatal(err)
		}
		wantOutcome(t, got, testMapOutcome{})
		if counts.Plan.Current != len(packageTests) {
			t.Errorf("plan = %+v; want every map current", counts.Plan)
		}
		for pkg := range packageTests {
			if m := readMap(t, pkg); m.Commit != head {
				t.Errorf("%s map commit = %s; want the plan to move it to HEAD %s with no slot", pkg, m.Commit, head)
			}
		}
		wantUntouched(t, "while no slot was free")

		// Two packages need a rebuild and the cap is one, so work stays queued and the audit does not run.
		plankit.WriteTree(t, prime, map[string]string{
			"headmove/headmove.go": testMapSource("headmove") + extraVariable + "var Again = 2\n",
			"outcomes/outcomes.go": testMapSource("outcomes") + extraVariable,
		})
		gitkit.Git(t, prime, "commit", "-q", "-am", "add a package variable to headmove and outcomes")
		got, _ = call(t, environ, 1, 1)
		wantOutcome(t, got, testMapOutcome{Built: []string{"headmove"}, Queued: 1})
		wantUntouched(t, "while work stayed queued")

		got, _ = call(t, environ, 10, 1)
		wantOutcome(t, got, testMapOutcome{Built: []string{"outcomes", "caller"}, AuditMismatches: 1})
		caller := readMap(t, "caller")
		if i := entryIndex(caller, "TestCallerCalls"); i < 0 || !slices.Equal(caller.Tests[i].Blocks, []int{0}) || !caller.Tests[i].MeasuredAt.After(measuredLong) {
			t.Errorf("caller map tests = %+v; want TestCallerCalls re-measured, covering block 0", caller.Tests)
		}

		// An entry with no measurement time sorts before the dated one, so the audit takes it first.
		undated := readMap(t, "caller")
		dated, added := entryIndex(undated, "TestCallerCalls"), entryIndex(undated, "TestCallerAdded")
		undated.Tests[dated].MeasuredAt = measuredLong
		undated.Tests[added].MeasuredAt = time.Time{}
		undated.Tests[added].Blocks = nil
		writeMap(t, "caller", undated)
		got, _ = call(t, environ, 10, 1)
		wantOutcome(t, got, testMapOutcome{Built: []string{"caller"}, AuditMismatches: 1})
		caller = readMap(t, "caller")
		if i := entryIndex(caller, "TestCallerAdded"); i < 0 || !slices.Equal(caller.Tests[i].Blocks, []int{0}) || caller.Tests[i].MeasuredAt.IsZero() {
			t.Errorf("caller map tests = %+v; want TestCallerAdded, which had no measurement time, re-measured first, covering block 0", caller.Tests)
		}

		// Every entry now agrees with a fresh run, so the audit re-measures the oldest and counts nothing.
		got, _ = call(t, environ, 10, 1)
		if len(got.Built) != 1 || got.AuditMismatches != 0 || got.SelfCheckMismatches != 0 || len(got.Failed) != 0 || got.Queued != 0 {
			t.Errorf("runTestMap() = %+v; want one package audited with no mismatch", got)
		}
	})

	t.Run("ACallEndedDuringABuildOrATestRecordsNoEntryAndNoFailure", func(t *testing.T) {
		plankit.WriteTree(t, prime, map[string]string{
			"cancelled/cancelled.go":      testMapSource("cancelled"),
			"cancelled/cancelled_test.go": testMapTestFile("cancelled", map[string]int{"TestCancelledHangs": 0}),
			"cmd/lym/main.go":             "package main\n\nfunc main() {}\n",
		})
		gitkit.Git(t, prime, "add", "cancelled", "cmd")
		gitkit.Git(t, prime, "commit", "-q", "-m", "add a package whose test never ends and the lym command")
		// The tier of the test prebuilds `lym`, so a call builds it before it runs the test.
		gateFile := configengine.ConfigFile(h.BoardDir(), "gate")
		prebuilt := strings.ReplaceAll(gate, "counter: go-json, env", "counter: go-json, prebuilt_lym: true, env")
		if err := os.WriteFile(gateFile, []byte(prebuilt), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.WriteFile(gateFile, []byte(gate), 0o644) })
		// Each row's stand-in appends a line to the started file once it hangs, which is when the call is cancelled.
		// The rows run in the order the call reaches them, and leave no map, so each starts from the same state.
		tests := []struct {
			name string
			env  func(started string) []string
		}{
			{"WhileTheGoListHangs", func(started string) []string { return withEnv(testMapHangListEnv, started) }},
			{"WhileTheCoverageBuildHangs", func(started string) []string {
				return append(withEnv(testMapCoverBuildsEnv, started), testMapHangBuildEnv+"="+module+"/cancelled")
			}},
			{"WhileTheLymBuildHangs", func(started string) []string {
				return append(withEnv(testMapLymBuildsEnv, started), testMapHangLymBuildEnv+"=1")
			}},
			{"WhileATestHangs", func(started string) []string { return withEnv(testMapHangingEnv, started) }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				started := filepath.Join(t.TempDir(), "started")
				// The test would time out after 20 seconds, and the build after a minute, so the cancel comes first.
				req := request(tt.env(started), 1, 0, "cancelled")
				req.secondUnit = time.Second
				ctx, cancel := context.WithCancel(lymcwd.WithCwd(context.Background(), prime))
				t.Cleanup(cancel)
				var out bytes.Buffer
				done := make(chan int, 1)
				go func() { done <- runTestMap(ctx, &out, req) }()
				for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(10 * time.Millisecond) {
					if _, err := os.Stat(started); err == nil {
						break
					}
					if time.Now().After(deadline) {
						cancel()
						<-done
						t.Fatal("the stand-in did not start hanging")
					}
				}
				cancel()
				select {
				case code := <-done:
					if code != 1 || !strings.Contains(out.String(), "the call was cancelled") {
						t.Errorf("runTestMap() = exit %d, output %q; want exit 1, naming the cancelled call", code, out.String())
					}
				case <-time.After(30 * time.Second):
					t.Fatal("the call did not end after it was cancelled")
				}
				if _, found, err := testcover.ReadMap(mapsDir, "cancelled"); err != nil || found {
					t.Errorf("ReadMap(cancelled) = found %v, err %v; want no map, so no entry and no failure, for a call that was ended", found, err)
				}
			})
		}
	})

	t.Run("TestsDeclaringParallelRunSideBySideAndMapAsASerialRunDoes", func(t *testing.T) {
		parallelTest := func(name string) string {
			return "\nfunc " + name + "(t *testing.T) {\n\ttiergate.Require(t, 0)\n\tt.Parallel()\n}\n"
		}
		plankit.WriteTree(t, prime, map[string]string{
			"sidebyside/sidebyside.go": testMapSource("sidebyside"),
			"sidebyside/sidebyside_test.go": "package sidebyside\n\nimport (\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n" +
				parallelTest("TestSidebysideFirstMeets") + parallelTest("TestSidebysideSecondMeets"),
		})
		gitkit.Git(t, prime, "add", "sidebyside")
		gitkit.Git(t, prime, "commit", "-q", "-m", "add a package whose tests declare Parallel")
		// Each test passes only once the other records itself beside it, so a serial run would fail both.
		got, _ := call(t, withEnv(testMapMeetEnv, filepath.Join(t.TempDir(), "meet")), 10, 0, "sidebyside")
		wantOutcome(t, got, testMapOutcome{Built: []string{"sidebyside"}})
		wantTests := []testcover.MapTest{
			{Name: "TestSidebysideFirstMeets", Blocks: []int{0}},
			{Name: "TestSidebysideSecondMeets", Blocks: []int{0}},
		}
		if diff := cmp.Diff(wantTests, readMap(t, "sidebyside").Tests, cmpopts.EquateEmpty(), ignoreRunFields); diff != "" {
			t.Errorf("sidebyside map tests mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("ANamedCallWaitsForABuildLockAnotherCallHoldsAndThenRebuildsThePackageWhole", func(t *testing.T) {
		before := readMap(t, "headmove")
		buildLock, err := lock.AcquireWriteLock(testMapBuildLockPath(mapsDir, "headmove"))
		if err != nil {
			t.Fatal(err)
		}
		release := sync.OnceFunc(func() { buildLock.Release() })
		t.Cleanup(release)
		ctx, cancel := context.WithTimeout(lymcwd.WithCwd(context.Background(), prime), time.Minute)
		t.Cleanup(cancel)
		var out bytes.Buffer
		done := make(chan int, 1)
		go func() { done <- runTestMap(ctx, &out, request(environ, 10, 0, "headmove")) }()

		// A call that skipped the locked package would end at once, so one still running after many waits is waiting.
		select {
		case code := <-done:
			t.Fatalf("runTestMap() = exit %d, output %s while another call held the build lock; want it waiting", code, out.Bytes())
		case <-time.After(20 * testMapTestWait):
		}
		release()
		select {
		case code := <-done:
			wantOutcome(t, decodeTestMap(t, code, out.Bytes()), testMapOutcome{Built: []string{"headmove"}})
		case <-time.After(30 * time.Second):
			t.Fatal("the named call did not end after the build lock was released")
		}
		if after := readMap(t, "headmove"); !after.BuiltAt.After(before.BuiltAt) {
			t.Errorf("headmove map built at %v; want after %v, rebuilt whole once the lock was free", after.BuiltAt, before.BuiltAt)
		}
	})
}

// TestGateTestMapPassYields runs a pass over a hub of two slots, and takes the other slot as a gate run would.
// The pass gives its slot back, keeping the round that finished, and a hand call naming the package builds it meanwhile.
// Once both slots are free again the pass takes one and plans the package afresh.
// It skips the package as queued when another call holds its build lock then.
// With no hand call it resumes the package, which it reports once.
func TestGateTestMapPassYields(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// handCall makes a call naming the package build it while the pass waits for a slot.
		handCall bool
		// holdLock makes a call hold the package's build lock from before the pass takes its slot again.
		holdLock bool
	}{
		{"RetakeReplansAndLeavesTheMapAHandCallWrote", true, false},
		{"RetakeSkipsAPackageAnotherCallLocksAndCountsItQueued", true, true},
		{"ResumedPackageIsBuiltAndMeasuredOnce", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runPassYields(t, tt.handCall, tt.holdLock)
		})
	}
}

// runPassYields runs one scenario of TestGateTestMapPassYields.
// handCall makes the test build the package with a hand call while the pass waits.
// holdLock makes the test hold the package's build lock when the pass takes its slot again.
func runPassYields(t *testing.T, handCall, holdLock bool) {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	prime := h.PrimeWorktree()
	location, err := lymcwd.ResolveWorktree(prime)
	if err != nil {
		t.Fatal(err)
	}
	const module = "example.com/yielding"
	gate := "slots: 2\nredundancy_build_timeout_min: 1\nredundancy_test_timeout_sec: 20\ntiers:\n" +
		"  tier0: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"0\"}}\n"
	if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(gate), 0o644); err != nil {
		t.Fatal(err)
	}
	// The rounds run in name order: TestYieldingAOne finishes before TestYieldingHangs starts.
	plankit.WriteTree(t, prime, map[string]string{
		"go.mod":                    "module " + module + "\n\ngo 1.21\n",
		"yielding/yielding.go":      testMapSource("yielding"),
		"yielding/yielding_test.go": testMapTestFile("yielding", map[string]int{"TestYieldingAOne": 0, "TestYieldingHangs": 0}),
	})
	gitkit.Git(t, prime, "add", "go.mod", "yielding")
	gitkit.Git(t, prime, "commit", "-q", "-m", "seed the yielding module")

	mapsDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
	standIn, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environ := append(os.Environ(), helperEnv+"="+helperRoleTestMapGo, testMapModuleEnv+"="+module)
	request := func(env []string, secondUnit time.Duration, packages ...string) testMapRequest {
		return testMapRequest{
			goBinary: standIn, maxPackages: 10, packages: packages, environ: env, upkeep: len(packages) == 0,
			secondUnit: secondUnit, minuteUnit: time.Minute, grace: time.Second,
			waitBase: testMapTestWait, waitCeiling: 5 * testMapTestWait, yieldPoll: testMapTestWait,
		}
	}
	// await polls until done reports true, failing the test after 30 seconds.
	await := func(t *testing.T, what string, done func() bool) {
		t.Helper()
		for deadline := time.Now().Add(30 * time.Second); !done(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting until %s", what)
			}
		}
	}

	// The hanging test would time out after 10 seconds, so the gate run's take comes first.
	started := filepath.Join(t.TempDir(), "started")
	ctx, cancel := context.WithTimeout(lymcwd.WithCwd(context.Background(), prime), 2*time.Minute)
	t.Cleanup(cancel)
	var passOut bytes.Buffer
	passDone := make(chan int, 1)
	go func() {
		passDone <- runTestMap(ctx, &passOut, request(append(slices.Clone(environ), testMapHangingEnv+"="+started), 500*time.Millisecond))
	}()
	await(t, "the pass runs its hanging test", func() bool {
		_, err := os.Stat(started)
		return err == nil
	})
	pool := hubgeom.GateSlots(location)
	gateLease, err := pool.Acquire(ctx, gateslot.Holder{Worktree: location.WorktreePath(), Site: "a gate run"})
	if err != nil {
		t.Fatal(err)
	}
	await(t, "the pass gives its slot back", func() bool {
		holders, err := pool.Holders()
		return err == nil && len(holders) == 1
	})
	var names []string
	for _, test := range readTestMap(t, mapsDir, "yielding").Tests {
		names = append(names, test.Name)
	}
	if want := []string{"TestYieldingAOne"}; !slices.Equal(names, want) {
		t.Errorf("yielding map entries after the yield = %v; want %v: the finished round kept, the killed test with no entry", names, want)
	}

	// The waiting pass holds no build lock, so a hand call naming the package takes it at once and rebuilds the package.
	if handCall {
		handCtx, handCancel := context.WithTimeout(lymcwd.WithCwd(context.Background(), prime), time.Minute)
		defer handCancel()
		var handOut bytes.Buffer
		code := runTestMap(handCtx, &handOut, request(environ, 150*time.Millisecond, "yielding"))
		if diff := cmp.Diff(testMapOutcome{Built: []string{"yielding"}, TimedOut: []string{"yielding.TestYieldingHangs"}}, decodeTestMap(t, code, handOut.Bytes()), cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("hand call mismatch (-want +got):\n%s", diff)
		}
	}

	afterHandCall := readTestMap(t, mapsDir, "yielding")
	if holdLock {
		retakeLock, err := lock.AcquireWriteLock(testMapBuildLockPath(mapsDir, "yielding"))
		if err != nil {
			t.Fatal(err)
		}
		release := sync.OnceFunc(func() { retakeLock.Release() })
		t.Cleanup(release)
	}

	if err := gateLease.Release(); err != nil {
		t.Fatal(err)
	}
	var passCode int
	select {
	case passCode = <-passDone:
	case <-time.After(30 * time.Second):
		t.Fatal("the pass did not end after both slots were free")
	}
	pass := decodeTestMap(t, passCode, passOut.Bytes())
	wantQueued := 0
	var wantSkipped []testMapSkip
	if holdLock {
		wantQueued = 1
		wantSkipped = []testMapSkip{{Package: "yielding", Cause: "another call holds its map build lock"}}
	}
	if diff := cmp.Diff(wantSkipped, pass.Skipped, cmpopts.EquateEmpty()); diff != "" || pass.Queued != wantQueued {
		t.Errorf("pass skipped %+v, queued %d; want queued %d and skipped mismatch (-want +got):\n%s", pass.Skipped, pass.Queued, wantQueued, diff)
	}
	if !handCall {
		// The pass resumed the package after its yield: its second job reruns the hanging test and times out,
		// and the package still shows one built entry and one measure, summed over both of its jobs.
		var counts testMapVerbCounts
		if err := json.Unmarshal(passOut.Bytes(), &counts); err != nil {
			t.Fatalf("unmarshal %q: %v", passOut.Bytes(), err)
		}
		if !slices.Equal(pass.Built, []string{"yielding"}) || !slices.Equal(pass.TimedOut, []string{"yielding.TestYieldingHangs"}) ||
			len(counts.Packages) != 1 || counts.Packages[0].Package != "yielding" || counts.Packages[0].Tests < 2 {
			t.Errorf("resumed pass built %v, timed out %v, measured %+v; want yielding once in built and measured, its second job having run",
				pass.Built, pass.TimedOut, counts.Packages)
		}
	} else if afterPass := readTestMap(t, mapsDir, "yielding"); !afterPass.BuiltAt.Equal(afterHandCall.BuiltAt) || len(afterPass.Tests) != len(afterHandCall.Tests) {
		t.Errorf("yielding map after the pass = built %v, %d tests; want the hand call's map, built %v with %d tests, left as it was",
			afterPass.BuiltAt, len(afterPass.Tests), afterHandCall.BuiltAt, len(afterHandCall.Tests))
	}
	if record, found, err := testcover.ReadExitRecord(mapsDir); err != nil || !found || record.PID != os.Getpid() || record.Queued != wantQueued {
		t.Errorf("ReadExitRecord() = %+v, found %v, err %v; want the pass's record with %d queued", record, found, err, wantQueued)
	}
}

// TestGateTestMapPassSlots runs `lym gate testmap` passes in order over one hub with a package.
// The hub's slot count changes between them.
// Each call builds on the maps and HEAD the last one left.
// Its subtests share that state and run in sequence, so none is parallel.
func TestGateTestMapPassSlots(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	const module = "example.com/slots"
	gateOf := func(slots int) string {
		return fmt.Sprintf("slots: %d\nredundancy_build_timeout_min: 1\nredundancy_test_timeout_sec: 20\ntiers:\n"+
			"  tier0: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"0\"}}\n", slots)
	}
	prime := seedTestMapModule(t, h, module, gateOf(2), map[string]map[string]int{"solo": {"TestSoloOne": 0}})
	location, err := lymcwd.ResolveWorktree(prime)
	if err != nil {
		t.Fatal(err)
	}
	mapsDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
	standIn, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environ := append(os.Environ(), helperEnv+"="+helperRoleTestMapGo, testMapModuleEnv+"="+module)
	request := func(env []string) testMapRequest {
		return testMapRequest{
			goBinary: standIn, maxPackages: 10, environ: env, upkeep: true,
			secondUnit: 150 * time.Millisecond, minuteUnit: time.Minute, grace: time.Second,
			waitBase: testMapTestWait, waitCeiling: 5 * testMapTestWait, yieldPoll: testMapTestWait,
		}
	}
	// call runs a pass under a minute's bound, so a call that waits for a slot it must not take fails the test.
	call := func(t *testing.T, env []string) testMapOutcome {
		t.Helper()
		ctx, cancel := context.WithTimeout(lymcwd.WithCwd(context.Background(), prime), time.Minute)
		defer cancel()
		var out bytes.Buffer
		code := runTestMap(ctx, &out, request(env))
		return decodeTestMap(t, code, out.Bytes())
	}
	writeGate := func(t *testing.T, slots int) {
		t.Helper()
		if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(gateOf(slots)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// declareTests commits a test file declaring tests to solo, and returns the HEAD it makes.
	declareTests := func(t *testing.T, tests map[string]int) string {
		t.Helper()
		plankit.WriteTree(t, prime, map[string]string{"solo/solo_test.go": testMapTestFile("solo", tests)})
		gitkit.Git(t, prime, "commit", "-q", "-am", "declare tests of solo")
		return gitkit.RevParse(t, prime, "HEAD")
	}
	soloTests := func(t *testing.T) []string {
		t.Helper()
		var names []string
		for _, test := range readTestMap(t, mapsDir, "solo").Tests {
			names = append(names, test.Name)
		}
		return names
	}
	wantSoloTests := func(t *testing.T, want ...string) {
		t.Helper()
		if got := soloTests(t); !slices.Equal(got, want) {
			t.Errorf("solo map tests = %v; want %v", got, want)
		}
	}

	t.Run("APassMapsThePackageWhenASlotIsFree", func(t *testing.T) {
		got := call(t, environ)
		if !slices.Equal(got.Built, []string{"solo"}) || got.Queued != 0 {
			t.Errorf("runTestMap() = built %v, queued %d; want solo built with nothing queued", got.Built, got.Queued)
		}
		wantSoloTests(t, "TestSoloOne")
	})

	t.Run("APassOnAHubOfOneSlotPlansTakesNoSlotAndCountsEveryJobQueued", func(t *testing.T) {
		writeGate(t, 1)
		head := declareTests(t, map[string]int{"TestSoloOne": 0, "TestSoloTwo": 0})
		builds := filepath.Join(t.TempDir(), "builds")
		got := call(t, append(slices.Clone(environ), testMapCoverBuildsEnv+"="+builds))
		if len(got.Built) != 0 || got.Queued != 1 {
			t.Errorf("runTestMap() = built %v, queued %d; want nothing built and the one job queued", got.Built, got.Queued)
		}
		if _, err := os.Stat(builds); !os.IsNotExist(err) {
			t.Errorf("coverage builds file, err %v; want none: the pass takes no slot, so it builds nothing", err)
		}
		if m := readTestMap(t, mapsDir, "solo"); m.Commit != head {
			t.Errorf("solo map commit = %s; want the plan to move it to HEAD %s", m.Commit, head)
		}
		wantSoloTests(t, "TestSoloOne")
		if record, found, err := testcover.ReadExitRecord(mapsDir); err != nil || !found || record.PlannedHead != head || record.Queued != 1 {
			t.Errorf("ReadExitRecord() = %+v, found %v, err %v; want planned HEAD %s and 1 queued", record, found, err, head)
		}
	})

	t.Run("ACallInsideAnEnclosingGateRunsSlotTakesNoSlotAndNeverYields", func(t *testing.T) {
		// The hub has one slot and the test holds it, so a second take would wait for ever.
		lease, err := hubgeom.GateSlots(location).Acquire(context.Background(), gateslot.Holder{Worktree: location.WorktreePath(), Site: "an enclosing gate run"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { lease.Release() })
		got := call(t, lease.InheritEnv(environ))
		if !slices.Equal(got.Built, []string{"solo"}) || got.Queued != 0 {
			t.Errorf("runTestMap() = built %v, queued %d; want solo measured to the end inside the inherited slot", got.Built, got.Queued)
		}
		wantSoloTests(t, "TestSoloOne", "TestSoloTwo")
	})

	t.Run("AHeadMoveFoundWhenTheSlotWaitWakesEndsThePassWithTheJobQueued", func(t *testing.T) {
		writeGate(t, 2)
		head := declareTests(t, map[string]int{"TestSoloOne": 0, "TestSoloThree": 0, "TestSoloTwo": 0})
		// The test holds one of the two slots, so the pass may take none and waits for one.
		pool := hubgeom.GateSlots(location)
		ctx, cancel := context.WithTimeout(lymcwd.WithCwd(context.Background(), prime), 2*time.Minute)
		t.Cleanup(cancel)
		gateLease, err := pool.Acquire(ctx, gateslot.Holder{Worktree: location.WorktreePath(), Site: "a gate run"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { gateLease.Release() })
		var passOut bytes.Buffer
		passDone := make(chan int, 1)
		go func() { passDone <- runTestMap(ctx, &passOut, request(environ)) }()
		// The plan moves the map to HEAD before the pass reaches its slot wait.
		pollUntil(t, "the pass planned the package at HEAD", func() bool {
			m, found, err := testcover.ReadMap(mapsDir, "solo")
			return err == nil && found && m.Commit == head
		})
		plankit.WriteTree(t, prime, map[string]string{"landed.txt": "landed\n"})
		gitkit.Git(t, prime, "add", "landed.txt")
		gitkit.Git(t, prime, "commit", "-q", "-m", "land a file outside every package")
		if err := gateLease.Release(); err != nil {
			t.Fatal(err)
		}
		var code int
		select {
		case code = <-passDone:
		case <-time.After(30 * time.Second):
			t.Fatal("the pass did not end after the slot was free")
		}
		got := decodeTestMap(t, code, passOut.Bytes())
		if len(got.Built) != 0 || got.Queued != 1 {
			t.Errorf("runTestMap() = built %v, queued %d; want nothing built and the package queued", got.Built, got.Queued)
		}
		if m := readTestMap(t, mapsDir, "solo"); m.Commit != head {
			t.Errorf("solo map commit = %s; want it left at the planned HEAD %s", m.Commit, head)
		}
		wantSoloTests(t, "TestSoloOne", "TestSoloTwo")
	})
}

// TestGateTestMapCLI runs `lym gate testmap` through its cobra command in a child process over a hub of two slots.
func TestGateTestMapCLI(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	gate := "slots: 2\nredundancy_build_timeout_min: 1\nredundancy_test_timeout_sec: 20\ntiers:\n" +
		"  tier0: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"0\"}}\n"

	t.Run("AnUpkeepPassHasNoCapUnlessMaxIsGiven", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
		const module = "example.com/capped"
		// One package more than the default cap leaves queued beside the one the first call measures.
		packageTests := map[string]map[string]int{}
		for i := 0; i <= defaultTestMapMax; i++ {
			packageTests[fmt.Sprintf("p%d", i)] = map[string]int{"TestPackageOne": 0}
		}
		prime := seedTestMapModule(t, h, module, gate, packageTests)
		mapsDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
		environ := append(os.Environ(), testMapModuleEnv+"="+module)

		cmd, out := startTestMapCLI(t, prime, environ, "testmap", "--upkeep", "--max", "1")
		capped := decodeTestMap(t, waitTestMapCLI(t, cmd), out.Bytes())
		if len(capped.Built) != 1 || capped.Queued != len(packageTests)-1 {
			t.Errorf("testmap --upkeep --max 1 = built %v, queued %d; want one package built and %d queued", capped.Built, capped.Queued, len(packageTests)-1)
		}

		cmd, out = startTestMapCLI(t, prime, environ, "testmap", "--upkeep")
		uncapped := decodeTestMap(t, waitTestMapCLI(t, cmd), out.Bytes())
		if uncapped.Queued != 0 {
			t.Errorf("testmap --upkeep = queued %d; want every queued package measured, with no cap", uncapped.Queued)
		}
		for pkg := range packageTests {
			if m := readTestMap(t, mapsDir, pkg); len(m.Tests) != 1 {
				t.Errorf("%s map tests = %+v; want its one test measured", pkg, m.Tests)
			}
		}
	})

	t.Run("ATerminatingSignalKeepsTheRoundsThatFinishedAndExitsWithItsNumber", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows has no catchable terminating signal to send")
		}
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
		const module = "example.com/signalled"
		// The rounds run in name order: TestYieldingAOne finishes before TestYieldingHangs starts.
		prime := seedTestMapModule(t, h, module, gate, map[string]map[string]int{"yielding": {"TestYieldingAOne": 0, "TestYieldingHangs": 0}})
		head := gitkit.RevParse(t, prime, "HEAD")
		mapsDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
		hanging := filepath.Join(t.TempDir(), "hanging")
		environ := append(os.Environ(), testMapModuleEnv+"="+module, testMapHangingEnv+"="+hanging)

		cmd, out := startTestMapCLI(t, prime, environ, "testmap", "--upkeep")
		pollUntil(t, "the pass runs its hanging test", func() bool {
			_, err := os.Stat(hanging)
			return err == nil
		})
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		if code, want := waitTestMapCLI(t, cmd), 128+int(syscall.SIGTERM); code != want {
			t.Errorf("testmap --upkeep exit = %d, output %s; want %d, 128 plus the signal number", code, out.Bytes(), want)
		}
		var names []string
		for _, test := range readTestMap(t, mapsDir, "yielding").Tests {
			names = append(names, test.Name)
		}
		if want := []string{"TestYieldingAOne"}; !slices.Equal(names, want) {
			t.Errorf("yielding map tests = %v; want %v: the finished round kept, the killed test with no entry", names, want)
		}
		if record, found, err := testcover.ReadExitRecord(mapsDir); err != nil || !found || record.PlannedHead != head || record.Queued != 1 {
			t.Errorf("ReadExitRecord() = %+v, found %v, err %v; want planned HEAD %s and the unmeasured package queued", record, found, err, head)
		}
	})
}
