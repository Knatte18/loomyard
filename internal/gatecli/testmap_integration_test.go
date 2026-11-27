// testmap_integration_test.go drives `lym gate test --map` over a hubforge hub whose prime holds a small Go module.
// The test binary, re-executed in the helper role helperRoleTestMapGo, stands in for `go`.
// It also stands in for the coverage binaries `go` builds, so no test builds a real binary.
// It is Tier 1: it builds a real hub and spawns git and the stand-ins.

package gatecli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// helperRoleTestMapGo makes the test binary the stand-in `go` of a `lym gate test --map` call.
// The same role runs as the coverage binary the stand-in builds.
const helperRoleTestMapGo = "testmap-go"

// helperRoleTestMapCLI makes the test binary run `lym gate test --map` through its cobra command, as a child process.
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

	testMapHangListEnv = "GATECLI_TESTMAP_HANG_LIST"

	// testMapMeetEnv names the file a test ending in Meets records itself in before it waits for another to do the same.
	testMapMeetEnv = "GATECLI_TESTMAP_MEET"

	// testMapExcludeEnv names the directory a test ending in Excludes records itself in, as a file of its own name.
	testMapExcludeEnv = "GATECLI_TESTMAP_EXCLUDE"

	// testMapCollideEnv names the directory a test ending in Collides keeps its record in, as a file of its own name.
	testMapCollideEnv = "GATECLI_TESTMAP_COLLIDE"

	// testMapOverlapEnv names the fixture package whose coverage build a test ending in Overlaps waits to see recorded.
	// The build is recorded in the file testMapCoverBuildsEnv names.
	testMapOverlapEnv = "GATECLI_TESTMAP_OVERLAP"
)

// testMapMeetWait bounds a Meets test's wait for another test to record itself beside it.
const testMapMeetWait = 10 * time.Second

// testMapExcludeWait is how long an Excludes test stays recorded while it looks for another test beside it.
const testMapExcludeWait = 300 * time.Millisecond

// testMapIndexFile is the file name of the index the prime's map run writes beside the shared maps.
const testMapIndexFile = "index.json"

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

// runTestMapCLIHelper runs `lym gate test --map` with the process's arguments over the directory helperCwdEnv names.
// The command's `go` is this test binary, in the helperRoleTestMapGo role.
// Its other parameters are the CLI's defaults.
func runTestMapCLIHelper() int {
	os.Setenv(helperEnv, helperRoleTestMapGo)
	root := newCommand(os.Args[0], defaultSamplePeriod, defaultPreTierCeiling, time.Minute, defaultRerunDelay, defaultPipeWaitDelay, defaultDetachSeams())
	code := clihelp.ExecuteIn(root, os.Getenv(helperCwdEnv), os.Stdout, os.Args[1:])
	_ = lymtemp.Remove()
	return code
}

// startTestMapCLI starts the test binary in the helperRoleTestMapCLI role over the prime worktree prime, under env.
// The child runs `gate` with args, such as `test --wait --map`, and is killed when the test ends or after two minutes.
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
// One ending in Excludes, when testMapExcludeEnv names a directory, records itself there for testMapExcludeWait.
// It fails if another test recorded itself there meanwhile, so it passes only apart from another.
// One ending in Overlaps, when testMapOverlapEnv names a package, passes only beside that package's coverage build.
// It fails unless the file testMapCoverBuildsEnv names records that build within testMapMeetWait.
// One ending in Collides fails while another test's record is beside its own, when testMapCollideEnv names a directory.
// It passes at once when its record is already there.
// That is how a rerun alone passes where two side by side both failed.
// One ending in Hoards keeps testMapHoardMegabytes megabytes, writing each one, and then passes.
// One ending in HoldsMemory keeps them too, and sleeps past the memory guard's sampling before it passes.
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
	case strings.HasSuffix(name, "Excludes"):
		if exclude := os.Getenv(testMapExcludeEnv); exclude != "" && !runsApart(exclude, name) {
			fmt.Printf("--- FAIL: %s (0.00s)\n", name)
			return 1
		}
	case strings.HasSuffix(name, "Overlaps"):
		if awaited := os.Getenv(testMapOverlapEnv); awaited != "" && !seesBuildOf(awaited) {
			fmt.Printf("--- FAIL: %s (0.00s)\n", name)
			return 1
		}
	case strings.HasSuffix(name, "Collides"):
		if collide := os.Getenv(testMapCollideEnv); collide != "" && collidesWithAnother(collide, name) {
			fmt.Printf("--- FAIL: %s (0.00s)\n", name)
			return 1
		}
	case strings.HasSuffix(name, "Hoards"):
		runtime.KeepAlive(hoardMegabytes(testMapHoardMegabytes))
	case strings.HasSuffix(name, "HoldsMemory"):
		hoarded := hoardMegabytes(testMapHoardMegabytes)
		time.Sleep(testMapMeetWait)
		runtime.KeepAlive(hoarded)
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

// hoardMegabytes keeps megabytes megabytes, writing each one so the host counts it, and returns them.
func hoardMegabytes(megabytes int) [][]byte {
	kept := make([][]byte, 0, megabytes)
	for range megabytes {
		block := make([]byte, 1<<20)
		for at := range block {
			block[at] = 1
		}
		kept = append(kept, block)
	}
	return kept
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

// seesBuildOf reports whether a coverage build of the fixture package pkg is recorded within testMapMeetWait.
// The builds are recorded in the file testMapCoverBuildsEnv names.
func seesBuildOf(pkg string) bool {
	prefix := os.Getenv(testMapModuleEnv) + "/" + pkg + " "
	for deadline := time.Now().Add(testMapMeetWait); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		data, err := os.ReadFile(os.Getenv(testMapCoverBuildsEnv))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, prefix) {
				return true
			}
		}
	}
	return false
}

// collidesWithAnother keeps name as a file in dir and reports whether another test's file appears beside it.
// It waits for one at most testMapMeetWait, and ends as soon as one appears.
// A file of name already in dir means the test ran before, so it reports false at once.
func collidesWithAnother(dir, name string) bool {
	own := filepath.Join(dir, name)
	if _, err := os.Stat(own); err == nil {
		return false
	}
	if os.WriteFile(own, nil, 0o644) != nil {
		return true
	}
	for deadline := time.Now().Add(testMapMeetWait); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if entries, err := os.ReadDir(dir); err == nil && len(entries) > 1 {
			return true
		}
	}
	return false
}

// runsApart records name as a file in dir for testMapExcludeWait, removes it, and reports whether no other file appeared.
func runsApart(dir, name string) bool {
	own := filepath.Join(dir, name)
	if os.WriteFile(own, nil, 0o644) != nil {
		return false
	}
	defer os.Remove(own)
	for deadline := time.Now().Add(testMapExcludeWait); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if entries, err := os.ReadDir(dir); err != nil || len(entries) > 1 {
			return false
		}
	}
	return true
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
// It records the build in the file testMapLymBuildsEnv names.
// It fails when testMapFailLymBuildEnv is set, and otherwise copies the test binary to the `-o` path.
func runTestMapStandInLymBuild(args []string) int {
	if builds := os.Getenv(testMapLymBuildsEnv); builds != "" {
		if appendLine(builds, "built") != nil {
			return 1
		}
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

// testMapOutcome is the envelope of a `lym gate test --map` call that walked its packages, bar its head and timings.
type testMapOutcome struct {
	Built       []string         `json:"built"`
	Unmeasured  []testMapSkip    `json:"unmeasured"`
	Failed      []testMapFailure `json:"failed"`
	TimedOut    []string         `json:"timed_out"`
	FailedTests []string         `json:"failed_tests"`
	PassedAlone []string         `json:"passed_alone"`
	LeftOut     []testMapSkip    `json:"left_out"`
	// Exit is the call's exit code, and Stderr what it wrote beside its envelope; neither is part of the envelope.
	Exit   int    `json:"-"`
	Stderr string `json:"-"`
}

// headMovedReason is the reason a package is unmeasured when HEAD moved from the commit from to the commit to.
func headMovedReason(from, to string) string {
	return "HEAD moved from " + from + " to " + to + "; way forward: re-run the same command once HEAD stays put"
}

// dirtyFileReason is the reason a package is unmeasured when a file is modified or untracked.
// The file is a worktree-relative slash path.
func dirtyFileReason(file string) string {
	return "modified or untracked file " + file + "; way forward: commit or revert it, then re-run the same command"
}

// mapScope returns the scope of a call over targets: the targets themselves, or every package when none are given.
func mapScope(targets []string) testRequest {
	if len(targets) == 0 {
		return testRequest{all: true}
	}
	return testRequest{targets: targets}
}

// runTestMapIn runs `lym gate test --map` over the prime worktree prime.
// It maps targets, or every package when none are given.
// It returns the call's exit code, its output and what it wrote to stderr.
// It uses the go command goBinary and the environment env.
// One build minute lasts minuteUnit.
func runTestMapIn(prime, goBinary string, env []string, minuteUnit time.Duration, targets ...string) (int, []byte, string) {
	var stderr bytes.Buffer
	code, out := runTestMapRequest(prime, testMapRequest{
		scope: mapScope(targets), goBinary: goBinary, environ: env,
		secondUnit: 150 * time.Millisecond, minuteUnit: minuteUnit, grace: time.Second, cappedParallel: unclampedParallel,
		stderr: &stderr,
	})
	return code, out, stderr.String()
}

// unclampedParallel keeps every test run of a call side by side, whatever memory the machine has left.
func unclampedParallel(parallel int, _ int64) int { return parallel }

// testMapHoardMegabytes is the memory a test ending in Hoards keeps before it passes.
// It stops there, so a memory cap that does not hold fails the test and never the machine.
const testMapHoardMegabytes = 384

// runTestMapRequest runs `lym gate test --map` over the prime worktree prime as request asks.
// It returns the call's exit code and output.
func runTestMapRequest(prime string, request testMapRequest) (int, []byte) {
	var out bytes.Buffer
	code := runTestMap(lymcwd.WithCwd(context.Background(), prime), &out, request)
	return code, out.Bytes()
}

// callTestMap runs `lym gate test --map` as runTestMapIn does.
// It fails the test unless the call walked its packages, and returns the call's envelope.
func callTestMap(t *testing.T, prime, goBinary string, env []string, minuteUnit time.Duration, targets ...string) testMapOutcome {
	t.Helper()
	outcome, _ := callTestMapCounts(t, prime, goBinary, env, minuteUnit, targets...)
	return outcome
}

// callTestMapCounts runs `lym gate test --map` as callTestMap does.
// It also returns the package times of its envelope.
func callTestMapCounts(t *testing.T, prime, goBinary string, env []string, minuteUnit time.Duration, targets ...string) (testMapOutcome, testMapVerbCounts) {
	t.Helper()
	code, out, stderr := runTestMapIn(prime, goBinary, env, minuteUnit, targets...)
	outcome := decodeTestMap(t, code, out)
	outcome.Stderr = stderr
	var counts testMapVerbCounts
	if err := json.Unmarshal(out, &counts); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	return outcome, counts
}

// decodeTestMap fails the test unless a `lym gate test --map` call walked its packages and printed its report.
// code is the call's exit code and out its output; it returns the call's envelope with the exit code set.
// A call that ended on a failed test or build still prints its report, so a failing exit decodes.
func decodeTestMap(t *testing.T, code int, out []byte) testMapOutcome {
	t.Helper()
	var refusal struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(out, &refusal); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	if refusal.Error != "" {
		t.Fatalf("runTestMap refused with exit %d: %s", code, refusal.Error)
	}
	var got testMapOutcome
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	got.Exit = code
	return got
}

// runTestMapOutcome runs `lym gate test --map` over the prime worktree prime as request asks.
// It returns the call's envelope with its exit code and the stderr it wrote.
func runTestMapOutcome(t *testing.T, prime string, request testMapRequest) testMapOutcome {
	t.Helper()
	var stderr bytes.Buffer
	request.stderr = &stderr
	code, out := runTestMapRequest(prime, request)
	got := decodeTestMap(t, code, out)
	got.Stderr = stderr.String()
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

// TestGateTestMapInATaskWorktreeKeepsItsOwnMaps builds a map from a task worktree at that worktree's HEAD.
// The call writes the map under the worktree's own `.lym` and leaves the hub's shared maps alone,
// and `lym gate redundant` from the same worktree reads the map it wrote.
// A card scope there maps the card's packages inside the module alone.
func TestGateTestMapInATaskWorktreeKeepsItsOwnMaps(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "backend", Pairs: []string{hubforge.TemplatePairSlug}})
	pair := h.PairCodeWorktree(hubforge.TemplatePairSlug)
	anchor := filepath.Join(pair, "backend")
	const module = "example.com/tasked"
	gate := "slots: 1\nscoper: go\nredundancy_build_timeout_min: 1\nredundancy_test_timeout_sec: 20\ntiers:\n" +
		"  tier0: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"0\"}}\n"
	if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(gate), 0o644); err != nil {
		t.Fatal(err)
	}
	plankit.WriteTree(t, anchor, map[string]string{
		"go.mod":       "module " + module + "\n\ngo 1.21\n",
		"solo/solo.go": testMapSource("solo"),
		// One test leaves no contained test, so `lym gate redundant` reads the map and builds nothing.
		"solo/solo_test.go": testMapTestFile("solo", map[string]int{"TestSoloOne": 0}),
	})
	gitkit.Git(t, pair, "add", "backend")
	gitkit.Git(t, pair, "commit", "-q", "-m", "seed the tasked module")
	head := gitkit.RevParse(t, pair, "HEAD")
	standIn, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), helperEnv+"="+helperRoleTestMapGo, testMapModuleEnv+"="+module)

	got := callTestMap(t, pair, standIn, env, time.Minute)
	if diff := cmp.Diff(testMapOutcome{Built: []string{"solo"}}, got, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("runTestMap() from the task worktree mismatch (-want +got):\n%s", diff)
	}
	// The worktree's own maps lie under its anchor's `.lym`, not under the worktree root's.
	if m := readTestMap(t, testcover.Dir(anchor), "solo"); m.Commit != head {
		t.Errorf("the task worktree's map of solo has commit %s; want the worktree's HEAD %s", m.Commit, head)
	}
	if _, found, err := testcover.ReadMap(testcover.Dir(fabricengine.BoltDir(h.Location.HubPath)), "solo"); err != nil || found {
		t.Errorf("ReadMap(solo) in the hub's shared maps = found %v, err %v; want no map", found, err)
	}
	// A task worktree's maps are few and read whole, so its call indexes neither its own maps nor the shared ones.
	for _, dir := range []string{testcover.Dir(anchor), testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))} {
		if _, err := os.Stat(filepath.Join(dir, testMapIndexFile)); !os.IsNotExist(err) {
			t.Errorf("stat of the index under %s after the task worktree's call = %v; want no index file", dir, err)
		}
	}

	var out bytes.Buffer
	code := clihelp.ExecuteIn(Command(), pair, &out, []string{"redundant", "solo"})
	var answer struct {
		OK        bool   `json:"ok"`
		MapCommit string `json:"map_commit"`
	}
	if err := json.Unmarshal(out.Bytes(), &answer); err != nil || code != 0 || !answer.OK || answer.MapCommit != head {
		t.Errorf("redundant solo from the task worktree = %d, %s; want exit 0 and map_commit %s", code, out.Bytes(), head)
	}

	// The card names a package of the backend module and a file outside it.
	plankit.WriteTree(t, anchor, map[string]string{
		"other/other.go":      testMapSource("other"),
		"other/other_test.go": testMapTestFile("other", map[string]int{"TestOtherOne": 0}),
	})
	gitkit.Git(t, pair, "add", "backend")
	gitkit.Git(t, pair, "commit", "-q", "-m", "add the other package")
	plankit.Write(t, planparser.PlanDir(anchor), plankit.Plan{Approved: true, Cards: []plankit.Card{{
		Number: 1, Slug: "span", Groups: []plankit.Group{{Label: "Edit", Targets: []string{"backend/solo/solo.go", "tools/gen.go"}}},
	}}})
	code, out2 := runTestMapRequest(pair, testMapRequest{
		scope: testRequest{cards: []string{"1"}}, goBinary: standIn, environ: env,
		secondUnit: 150 * time.Millisecond, minuteUnit: time.Minute, grace: time.Second, cappedParallel: unclampedParallel,
	})
	if diff := cmp.Diff(testMapOutcome{Built: []string{"solo"}}, decodeTestMap(t, code, out2), cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("runTestMap() --card 1 from the task worktree mismatch (-want +got):\n%s", diff)
	}
	if _, found, err := testcover.ReadMap(testcover.Dir(anchor), "other"); err != nil || found {
		t.Errorf("ReadMap(other) = found %v, err %v; want no map, since the card names solo alone", found, err)
	}
}

// TestGateTestMap runs `lym gate test --map` calls in order over one hub.
// Each call builds on the maps and HEAD the last one left.
// Its subtests share that state and run in sequence, so none is parallel.
func TestGateTestMap(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	prime := h.PrimeWorktree()
	const module = "example.com/mapped"
	// A test past 20 seconds of 150 milliseconds each times out: 3 seconds, long enough for a stand-in to start under load.
	// tierFields adds fields to the flow mapping of tier0 and tier1.
	// Two slots let a call hold one while a test takes the other, and tier0's parallel of two runs two tests side by side.
	gateYAML := func(tierFields string) string {
		return "slots: 2\nscoper: go\nredundancy_build_timeout_min: 1\nredundancy_test_timeout_sec: 20\ntiers:\n" +
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
	writeGate(t, gateYAML("parallel: 2, "))

	// zmoved's test commits a change to its own file, and its name puts its job last.
	// outcomes holds a test for each outcome but a timeout: a pass, a failure, a skip and a pass covering nothing.
	packageTests := map[string]map[string]int{
		"alpha":    {"TestAlphaOne": 0, "TestAlphaTwo": 1, "TestAlphaHangs": 1, "TestAlphaTier2": 2, "TestAlphaUntiered": -1},
		"broken":   {"TestBrokenOne": 0},
		"outcomes": {"TestOutcomesPass": 0, "TestOutcomesFails": 0, "TestOutcomesSkips": 0, "TestOutcomesElsewhere": 0},
		"zmoved":   {"TestZmovedMoves": 0},
		"notests":  nil,
		"steady":   {"TestSteadyOne": 0},
	}
	files := map[string]string{"go.mod": "module " + module + "\n\ngo 1.21\n"}
	for pkg, tiers := range packageTests {
		files[pkg+"/"+pkg+".go"] = testMapSource(pkg)
		if tiers != nil {
			files[pkg+"/"+pkg+"_test.go"] = testMapTestFile(pkg, tiers)
		}
	}
	// The module also holds directories no package walk may enter: testdata, a dot directory and a nested module.
	// The "ignored" package is on disk with a test but untracked and excluded, so HEAD tracks no Go file of it.
	files["testdata/testdata.go"] = "package testdata\n"
	files[".dot/dot.go"] = "package dot\n"
	files["nested/go.mod"] = "module example.com/nested\n\ngo 1.21\n"
	files["nested/nested.go"] = "package nested\n"
	files["ignored/ignored.go"] = testMapSource("ignored")
	files["ignored/ignored_test.go"] = testMapTestFile("ignored", map[string]int{"TestIgnoredOne": 0})
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
	// The prime's state and scratch links are untracked too.
	// They would make every --base scope fall back to the whole module.
	if err := os.WriteFile(excludePath, []byte("ignored/\n.lym\n_lym\n.scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitkit.Git(t, prime, append([]string{"add", "go.mod", "testdata", ".dot", "nested"}, slices.Sorted(maps.Keys(packageTests))...)...)
	gitkit.Git(t, prime, "commit", "-q", "-m", "seed the mapped module")

	mapsDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
	standIn, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environ := append(os.Environ(), helperEnv+"="+helperRoleTestMapGo, testMapModuleEnv+"="+module, testMapFailBuildEnv+"="+module+"/broken")
	// callWith runs the verb over the prime under the environment env, with one build minute lasting minuteUnit.
	// It maps the targets, or every package when none are given.
	callWith := func(t *testing.T, env []string, minuteUnit time.Duration, targets ...string) testMapOutcome {
		t.Helper()
		return callTestMap(t, prime, standIn, env, minuteUnit, targets...)
	}
	call := func(t *testing.T, targets ...string) testMapOutcome {
		t.Helper()
		return callWith(t, environ, time.Minute, targets...)
	}
	// callScope runs the verb under the standard environment over the scope the request holds.
	callScope := func(t *testing.T, scope testRequest) testMapOutcome {
		t.Helper()
		code, out := runTestMapRequest(prime, testMapRequest{
			scope: scope, goBinary: standIn, environ: environ,
			secondUnit: 150 * time.Millisecond, minuteUnit: time.Minute, grace: time.Second, cappedParallel: unclampedParallel,
		})
		return decodeTestMap(t, code, out)
	}
	readMap := func(t *testing.T, pkg string) testcover.Map {
		t.Helper()
		return readTestMap(t, mapsDir, pkg)
	}
	// callCounts runs the verb under the standard environment, and returns the package times of its envelope too.
	callCounts := func(t *testing.T, targets ...string) (testMapOutcome, testMapVerbCounts) {
		t.Helper()
		return callTestMapCounts(t, prime, standIn, environ, time.Minute, targets...)
	}
	// withEnv returns the standard environment with the variable name set to value.
	withEnv := func(name, value string) []string {
		return append(slices.Clone(environ), name+"="+value)
	}
	// request returns a call's request under env, with one test second lasting secondUnit.
	request := func(env []string, secondUnit time.Duration, targets ...string) testMapRequest {
		return testMapRequest{
			scope: mapScope(targets), goBinary: standIn, environ: env,
			secondUnit: secondUnit, minuteUnit: time.Minute, grace: time.Second, cappedParallel: unclampedParallel,
		}
	}
	// wantOutcome compares an outcome, its exit code included, but not its stderr.
	// Its failures are checked by wantBrokenFailed.
	wantOutcome := func(t *testing.T, got, want testMapOutcome) {
		t.Helper()
		if diff := cmp.Diff(want, got, cmpopts.EquateEmpty(), cmpopts.IgnoreFields(testMapOutcome{}, "Failed", "Stderr")); diff != "" {
			t.Errorf("runTestMap() mismatch (-want +got):\n%s", diff)
		}
	}
	wantBrokenFailed := func(t *testing.T, got testMapOutcome) {
		t.Helper()
		if len(got.Failed) != 1 || got.Failed[0].Package != "broken" || !strings.Contains(got.Failed[0].Failure, "the coverage build failed") {
			t.Errorf("runTestMap() failed = %+v; want broken alone, naming its build failure", got.Failed)
		}
	}
	ignoreRunFields := cmpopts.IgnoreFields(testcover.MapTest{}, "FuncHash", "Seconds", "Commit", "MeasuredAt")
	outsideHub := t.TempDir()

	t.Run("Refusals", func(t *testing.T) {
		tests := []struct {
			name string
			dir  string
			args []string
			want string
			// wantPart, when set, replaces want: the error need only hold it.
			wantPart string
			// setup, when set, changes the hub's gate config for the row.
			setup func(t *testing.T)
		}{
			{
				name: "FromThePrimesRecordsSiblingActsInThePrime", dir: h.PrimeRecords(), args: []string{"test", "--wait", "--map", "nosuch"},
				want: "gate test --map: \"nosuch\" names no package directory of the module at " + prime + "; way forward: name a package by its directory relative to the module root, such as internal/foo",
			},
			{
				name: "OutsideEveryHub", dir: outsideHub, args: []string{"test", "--wait", "--map"},
				want: "gate test --map: " + outsideHub + " is in no hub, and the coverage maps live in a hub; way forward: run lym gate test --map from a worktree of a hub",
			},
			{
				name: "NoMappedTier", dir: prime, args: []string{"test", "--wait", "--map"},
				want: "gate test --map: the hub declares neither tier0 nor tier1, so no test of a package can be mapped; way forward: declare them with lym config gate from the prime, then re-run the same command",
				setup: func(t *testing.T) {
					writeGate(t, "slots: 1\ntiers:\n  tier2: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"2\"}}\n")
					t.Cleanup(func() { writeGate(t, gateYAML("parallel: 2, ")) })
				},
			},
			{
				name: "NotGoJSONTier0", dir: prime, args: []string{"test", "--wait", "--map"},
				want: "gate test --map: tier0 counts its tests with vstest, and a coverage map exists only for a go-json tier; way forward: when tier0 runs Go tests, set its counter to go-json with lym config gate from the prime and re-run the same command; a hub whose tier0 runs no Go tests has no coverage map to build",
				setup: func(t *testing.T) {
					writeGate(t, strings.Replace(gateYAML("parallel: 2, "), "tier0: {command: x, timeout_min: 1, counter: go-json", "tier0: {command: x, timeout_min: 1, counter: vstest", 1))
					t.Cleanup(func() { writeGate(t, gateYAML("parallel: 2, ")) })
				},
			},
			{
				name: "NotGoJSONTier1", dir: prime, args: []string{"test", "--wait", "--map"},
				want: "gate test --map: tier1 counts its tests with vstest, and a coverage map exists only for a go-json tier; way forward: when tier1 runs Go tests, set its counter to go-json with lym config gate from the prime and re-run the same command; a hub whose tier1 runs no Go tests has no coverage map to build",
				setup: func(t *testing.T) {
					writeGate(t, strings.Replace(gateYAML("parallel: 2, "), "tier1: {command: x, timeout_min: 1, counter: go-json", "tier1: {command: x, timeout_min: 1, counter: vstest", 1))
					t.Cleanup(func() { writeGate(t, gateYAML("parallel: 2, ")) })
				},
			},
			{
				name: "MemoryBelowTheTestCapTier0", dir: prime, args: []string{"test", "--wait", "--map"},
				want: "gate test --map: tier0 sets memory_mb 64, below redundancy_test_memory_mb 128, so no test process fits its memory; way forward: raise tier0's memory_mb or lower redundancy_test_memory_mb with lym config gate from the prime, then re-run the same command",
				setup: func(t *testing.T) {
					writeGate(t, "redundancy_test_memory_mb: 128\n"+strings.Replace(gateYAML("parallel: 2, "), "tier0: {command: x,", "tier0: {memory_mb: 64, command: x,", 1))
					t.Cleanup(func() { writeGate(t, gateYAML("parallel: 2, ")) })
				},
			},
			{
				name: "MemoryBelowTheTestCapTier1", dir: prime, args: []string{"test", "--wait", "--map"},
				want: "gate test --map: tier1 sets memory_mb 64, below redundancy_test_memory_mb 128, so no test process fits its memory; way forward: raise tier1's memory_mb or lower redundancy_test_memory_mb with lym config gate from the prime, then re-run the same command",
				setup: func(t *testing.T) {
					writeGate(t, "redundancy_test_memory_mb: 128\n"+strings.Replace(gateYAML("parallel: 2, "), "tier1: {command: x,", "tier1: {memory_mb: 64, command: x,", 1))
					t.Cleanup(func() { writeGate(t, gateYAML("parallel: 2, ")) })
				},
			},
			{
				name: "UnknownPackage", dir: prime, args: []string{"test", "--wait", "--map", "nosuch"},
				want: "gate test --map: \"nosuch\" names no package directory of the module at " + prime + "; way forward: name a package by its directory relative to the module root, such as internal/foo",
			},
			{
				name: "ACardWithNoPlan", dir: prime, args: []string{"test", "--wait", "--map", "--card", "1"},
				want: "gate test: --card needs the plan at " + planparser.PlanDir(prime) + ", which this worktree does not hold; way forward: run the call from the task worktree that holds the plan, or use --all or explicit targets, then re-run the same command",
			},
			{
				name: "TheRetiredVerb", dir: prime, args: []string{"testmap"}, wantPart: "unknown subcommand \"testmap\"",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if tt.setup != nil {
					tt.setup(t)
				}
				var out bytes.Buffer
				code := clihelp.ExecuteIn(Command(), tt.dir, &out, tt.args)
				var envelope struct {
					Error string `json:"error"`
				}
				if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
					t.Fatalf("unmarshal %q: %v", out.String(), err)
				}
				matches := envelope.Error == tt.want
				if tt.wantPart != "" {
					matches = strings.Contains(envelope.Error, tt.wantPart)
				}
				if code != 1 || !matches {
					t.Errorf("lym gate %q = exit %d, error %q; want exit 1, error %q%s", tt.args, code, envelope.Error, tt.want, tt.wantPart)
				}
			})
		}
		if _, found, err := testcover.ReadMap(mapsDir, "alpha"); err != nil || found {
			t.Errorf("ReadMap(alpha) after the refusals = found %v, err %v; want no map written", found, err)
		}
	})

	t.Run("AnAllCallMapsEveryPackageWithATestAndRecordsAFailedBuild", func(t *testing.T) {
		head := gitkit.RevParse(t, prime, "HEAD")
		got, counts := callCounts(t)
		// notests holds no test and no map, and ignored tracks no Go file, so neither is in the set.
		// zmoved goes last, and its test moves HEAD, so its job is discarded and its package is left unmeasured.
		// The call exits 1: a test timed out, another failed, broken's build failed, and zmoved is unmeasured.
		wantOutcome(t, got, testMapOutcome{
			Exit: 1, Built: []string{"alpha", "outcomes", "steady"}, TimedOut: []string{"alpha.TestAlphaHangs"},
			Unmeasured:  []testMapSkip{{Package: "zmoved", Reason: headMovedReason(head, gitkit.RevParse(t, prime, "HEAD"))}},
			FailedTests: []string{"alpha.TestAlphaHangs", "outcomes.TestOutcomesFails"},
		})
		wantBrokenFailed(t, got)
		// zmoved's test moved HEAD, which ended the walk, so the call wrote no index.
		if _, err := os.Stat(filepath.Join(mapsDir, testMapIndexFile)); !os.IsNotExist(err) {
			t.Errorf("stat of the index after a call whose HEAD moved = %v; want no index file", err)
		}

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
		steady := readMap(t, "steady")
		if diff := cmp.Diff([]testcover.MapTest{{Name: "TestSteadyOne", Blocks: []int{0}}}, steady.Tests, ignoreRunFields); diff != "" {
			t.Errorf("steady map tests mismatch (-want +got):\n%s", diff)
		}
		if steady.Commit != head {
			t.Errorf("steady map commit = %s; want %s, the HEAD read before zmoved's test landed a commit", steady.Commit, head)
		}
		for _, pkg := range []string{"notests", "ignored"} {
			if _, found, err := testcover.ReadMap(mapsDir, pkg); err != nil || found {
				t.Errorf("ReadMap(%s) = found %v, err %v; want no map for a package outside the set", pkg, found, err)
			}
		}
		if _, found, err := testcover.ReadMap(mapsDir, "zmoved"); err != nil || found {
			t.Errorf("ReadMap(zmoved) = found %v, err %v; want no map for a package whose files changed under its only round", found, err)
		}
		wantOutcomes := []testcover.MapTest{
			{Name: "TestOutcomesElsewhere", Outcome: testcover.OutcomeNoCoverage},
			{Name: "TestOutcomesFails", Outcome: testcover.OutcomeFailed},
			{Name: "TestOutcomesPass", Blocks: []int{0}},
			{Name: "TestOutcomesSkips", Outcome: testcover.OutcomeSkipped},
		}
		if diff := cmp.Diff(wantOutcomes, readMap(t, "outcomes").Tests, cmpopts.EquateEmpty(), ignoreRunFields); diff != "" {
			t.Errorf("outcomes map tests mismatch (-want +got):\n%s", diff)
		}
		wantMeasures := []testMapMeasure{{Package: "alpha", Tests: 3}, {Package: "outcomes", Tests: len(wantOutcomes)}, {Package: "steady", Tests: 1}}
		if diff := cmp.Diff(wantMeasures, counts.Packages, cmpopts.IgnoreFields(testMapMeasure{}, "BuildSeconds", "RunSeconds")); diff != "" {
			t.Errorf("packages mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("NamedPackageIsRebuiltWhenFresh", func(t *testing.T) {
		before := readMap(t, "alpha")
		got := call(t, "./alpha")
		// The timeout alone causes the exit.
		wantOutcome(t, got, testMapOutcome{Exit: 1, Built: []string{"alpha"}, TimedOut: []string{"alpha.TestAlphaHangs"}, FailedTests: []string{"alpha.TestAlphaHangs"}})
		if after := readMap(t, "alpha"); !after.BuiltAt.After(before.BuiltAt) {
			t.Errorf("alpha map built at %v; want after %v", after.BuiltAt, before.BuiltAt)
		}
		// The call ran in the prime and measured its package with HEAD in place, so it wrote the index beside the maps.
		if info, err := os.Stat(filepath.Join(mapsDir, testMapIndexFile)); err != nil || info.IsDir() {
			t.Errorf("stat of the index after a measuring prime call = %v, %v; want an index file", info, err)
		}
	})

	t.Run("AFailedIndexWriteChangesNeitherTheOutputNorTheExitCode", func(t *testing.T) {
		indexPath := filepath.Join(mapsDir, testMapIndexFile)
		if err := os.RemoveAll(indexPath); err != nil {
			t.Fatal(err)
		}
		// A directory at the index's path fails its write.
		if err := os.Mkdir(indexPath, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(indexPath) })
		wantOutcome(t, call(t, "steady"), testMapOutcome{Built: []string{"steady"}})
		if info, err := os.Stat(indexPath); err != nil || !info.IsDir() {
			t.Errorf("stat of the index path = %v, %v; want the directory the test made, since no index was written", info, err)
		}
	})

	t.Run("AMapRunReplacesAnExistingMapWhole", func(t *testing.T) {
		before := readMap(t, "outcomes")
		ghost := testcover.MapTest{Name: "TestOutcomesGhost", Blocks: []int{0}, Commit: before.Commit, MeasuredAt: before.BuiltAt}
		before.Tests = slices.Insert(before.Tests, 0, ghost)
		if err := testcover.WriteMap(mapsDir, "outcomes", before); err != nil {
			t.Fatal(err)
		}
		// TestOutcomesFails fails, so the call exits 1.
		wantOutcome(t, call(t, "outcomes"), testMapOutcome{Exit: 1, Built: []string{"outcomes"}, FailedTests: []string{"outcomes.TestOutcomesFails"}})
		var names []string
		for _, test := range readMap(t, "outcomes").Tests {
			names = append(names, test.Name)
		}
		wantNames := []string{"TestOutcomesElsewhere", "TestOutcomesFails", "TestOutcomesPass", "TestOutcomesSkips"}
		if !slices.Equal(names, wantNames) {
			t.Errorf("outcomes map entries = %v; want %v, with the hand-written ghost gone", names, wantNames)
		}
	})

	t.Run("APackageWhoseLastTestWasDeletedGetsAMapWithNoTests", func(t *testing.T) {
		plankit.WriteTree(t, prime, map[string]string{
			"emptied/emptied.go":      testMapSource("emptied"),
			"emptied/emptied_test.go": testMapTestFile("emptied", map[string]int{"TestEmptiedOne": 0}),
		})
		gitkit.Git(t, prime, "add", "emptied")
		gitkit.Git(t, prime, "commit", "-q", "-m", "add a package with one test")
		wantOutcome(t, call(t, "emptied"), testMapOutcome{Built: []string{"emptied"}})
		if m := readMap(t, "emptied"); len(m.Tests) != 1 {
			t.Fatalf("emptied map tests = %+v; want its one test", m.Tests)
		}
		gitkit.Git(t, prime, "rm", "-q", "emptied/emptied_test.go")
		gitkit.Git(t, prime, "commit", "-q", "-m", "delete the package's last test")
		wantOutcome(t, call(t, "emptied"), testMapOutcome{Built: []string{"emptied"}})
		if m := readMap(t, "emptied"); len(m.Tests) != 0 || len(m.Files) != 1 {
			t.Errorf("emptied map = tests %+v, files %v; want no test entry and its one file", m.Tests, m.Files)
		}
	})

	t.Run("APackageWithNoMappedTestIsCheckedBeforeItsWrite", func(t *testing.T) {
		before := readMap(t, "emptied")
		notes := filepath.Join(prime, "emptied", "notes.txt")
		if err := os.WriteFile(notes, []byte("notes\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := call(t, "emptied")
		if err := os.Remove(notes); err != nil {
			t.Fatal(err)
		}
		wantOutcome(t, got, testMapOutcome{Exit: 1, Unmeasured: []testMapSkip{{Package: "emptied", Reason: dirtyFileReason("emptied/notes.txt")}}})
		if after := readMap(t, "emptied"); !after.BuiltAt.Equal(before.BuiltAt) {
			t.Errorf("emptied map built at %v; want it left from %v", after.BuiltAt, before.BuiltAt)
		}
	})

	t.Run("ABaseScopeMapsThePackagesItsDiffTouches", func(t *testing.T) {
		plankit.WriteTree(t, prime, map[string]string{"steady/steady.go": testMapSource("steady") + "// edited\n"})
		gitkit.Git(t, prime, "commit", "-q", "-am", "edit steady's source")
		wantOutcome(t, callScope(t, testRequest{base: "HEAD~1"}), testMapOutcome{Built: []string{"steady"}})
	})

	t.Run("AnEllipsisTargetMapsEveryPackageBelowIt", func(t *testing.T) {
		everything := call(t)
		ellipsis := call(t, "./...")
		if diff := cmp.Diff(everything.Built, ellipsis.Built, cmpopts.EquateEmpty()); diff != "" || len(ellipsis.LeftOut) != 0 {
			t.Errorf("runTestMap() over ./... built mismatch against --all (-all +ellipsis):\n%s\nleft out %+v; want none, since no target named a package by its directory", diff, ellipsis.LeftOut)
		}
		// ./... holds outcomes, alpha's TestAlphaHangs and broken's failed build.
		if everything.Exit != 1 || ellipsis.Exit != 1 {
			t.Errorf("runTestMap() exits = %d over --all and %d over ./...; want 1 for both", everything.Exit, ellipsis.Exit)
		}
	})

	t.Run("ACardScopeMapsItsCardsPackages", func(t *testing.T) {
		plankit.Write(t, planparser.PlanDir(prime), plankit.Plan{Approved: true, Cards: []plankit.Card{{
			Number: 1, Slug: "span", Groups: []plankit.Group{{Label: "Edit", Targets: []string{"steady/steady.go"}}},
		}}})
		t.Cleanup(func() { os.RemoveAll(planparser.PlanDir(prime)) })
		plankit.WriteTree(t, prime, map[string]string{
			"tool/tool.go": "package main\n\nfunc One() int { return 1 }\n\nfunc Two() int { return 2 }\n\nfunc main() {}\n",
			"tool/tool_test.go": "package main\n\nimport (\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n\n" +
				"func TestToolOne(t *testing.T) {\n\ttiergate.Require(t, 0)\n}\n",
		})
		gitkit.Git(t, prime, "add", "tool")
		gitkit.Git(t, prime, "commit", "-q", "-m", "add a package main")
		wantOutcome(t, callScope(t, testRequest{cards: []string{"1"}}), testMapOutcome{Built: []string{"steady"}})
		wantOutcome(t, callScope(t, testRequest{cards: []string{"1"}, mainPackages: true}), testMapOutcome{Built: []string{"steady", "tool"}})
	})

	t.Run("NamedPackageWithNoTrackedGoFileIsSkipped", func(t *testing.T) {
		got := call(t, "ignored", "notests")
		wantOutcome(t, got, testMapOutcome{LeftOut: []testMapSkip{
			{Package: "ignored", Reason: "no Go file tracked at HEAD"},
			{Package: "notests", Reason: "no tier0 or tier1 test and no map"},
		}})
		for _, pkg := range []string{"ignored", "notests"} {
			if _, found, err := testcover.ReadMap(mapsDir, pkg); err != nil || found {
				t.Errorf("ReadMap(%s) = found %v, err %v; want no map written", pkg, found, err)
			}
		}
	})

	t.Run("AComputedScopeTakesNoBaseFromTheGreenRecord", func(t *testing.T) {
		location, err := lymcwd.ResolveWorktree(prime)
		if err != nil {
			t.Fatal(err)
		}
		greenPath := filepath.Join(gateslot.WaitDir(location.AnchorPath()), greenFileName)
		// A green record of tier0 at HEAD would make a computed scope the empty diff from HEAD; a map call reads none.
		writeGate(t, strings.Replace(gateYAML("parallel: 2, "), "tier0: {command: x,", "tier0: {command: 'true',", 1))
		t.Cleanup(func() {
			os.Remove(greenPath)
			writeGate(t, gateYAML("parallel: 2, "))
		})
		var green bytes.Buffer
		runTest(lymcwd.WithCwd(context.Background(), prime), &green, testRequest{
			goBinary: standIn, timeoutUnit: time.Minute, secondUnit: time.Second, preTierCeiling: time.Minute, rerunDelay: time.Second,
			pipeWaitDelay: time.Second, tiers: []int{0}, all: true,
		})
		// The prime has no parent branch, so the computed scope is the whole module, whatever the green record holds.
		// Only the built set is asserted: the exit code follows the failing tests of the set.
		code, out := runTestMapRequest(prime, testMapRequest{
			scope: testRequest{}, goBinary: standIn, environ: environ,
			secondUnit: 150 * time.Millisecond, minuteUnit: time.Minute, grace: time.Second, cappedParallel: unclampedParallel,
		})
		var got testMapOutcome
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("unmarshal %q (exit %d): %v", out, code, err)
		}
		if want := call(t); !slices.Equal(got.Built, want.Built) {
			t.Errorf("test --wait --map with no scope flag built %v; want the %v --all builds", got.Built, want.Built)
		}
	})

	t.Run("BuildPastItsTimeoutKeepsTheMapAndIsNotRetriedAtItsHead", func(t *testing.T) {
		head := gitkit.RevParse(t, prime, "HEAD")
		before := readMap(t, "steady")
		hangEnviron := append(slices.Clone(environ), testMapHangBuildEnv+"="+module+"/steady")
		got := callWith(t, hangEnviron, 2*time.Second, "steady")
		// The failed build alone causes the exit.
		wantOutcome(t, got, testMapOutcome{Exit: 1})
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
	})

	t.Run("AMapRunBuildsNoLym", func(t *testing.T) {
		if err := os.MkdirAll(filepath.Join(prime, "cmd", "lym"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(prime, "cmd", "lym", "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		gitkit.Git(t, prime, "add", "cmd")
		gitkit.Git(t, prime, "commit", "-q", "-m", "add the lym command")
		writeGate(t, gateYAML("parallel: 2, prebuilt_lym: true, "))
		t.Cleanup(func() { writeGate(t, gateYAML("parallel: 2, ")) })

		// The call inherits a forged prebuilt path, which the call's environment drops.
		records := t.TempDir()
		builds, seen := filepath.Join(records, "builds"), filepath.Join(records, "seen")
		lymEnviron := append(withEnv(gateslot.PrebuiltLymEnv, "/forged/lym"), testMapLymBuildsEnv+"="+builds, testMapLymSeenEnv+"="+seen)
		got := callWith(t, lymEnviron, time.Minute, "alpha", "steady")
		wantOutcome(t, got, testMapOutcome{
			Exit: 1, Built: []string{"alpha", "steady"}, TimedOut: []string{"alpha.TestAlphaHangs"}, FailedTests: []string{"alpha.TestAlphaHangs"},
		})
		if len(got.Failed) != 0 {
			t.Errorf("runTestMap() failed = %+v; want none", got.Failed)
		}

		if _, err := os.Stat(builds); !os.IsNotExist(err) {
			t.Errorf("stat of the lym builds file = %v; want it absent, since a map run builds no lym", err)
		}
		seenData, err := os.ReadFile(seen)
		if err != nil {
			t.Fatal(err)
		}
		paths := strings.Split(strings.TrimSuffix(string(seenData), "\n"), "\n")
		if len(paths) < 2 || slices.ContainsFunc(paths, func(path string) bool { return path != "" }) {
			t.Errorf("prebuilt lym paths the tests saw = %q; want an empty one for every run of both packages", paths)
		}
	})

	t.Run("TestsRunSideBySideAndAFailureBesideAnotherEndsPassingWhenRunAlone", func(t *testing.T) {
		writeGate(t, gateYAML("parallel: 2, "))
		plankit.WriteTree(t, prime, map[string]string{
			"excludes/excludes.go":      testMapSource("excludes"),
			"excludes/excludes_test.go": testMapTestFile("excludes", map[string]int{"TestExcludesFirstCollides": 0, "TestExcludesSecondCollides": 0}),
			"lingers/lingers.go":        testMapSource("lingers"),
			"lingers/lingers_test.go":   testMapTestFile("lingers", map[string]int{"TestLingersOverlaps": 0}),
			"meets/meets.go":            testMapSource("meets"),
			"meets/meets_test.go":       testMapTestFile("meets", map[string]int{"TestMeetsFirstMeets": 0, "TestMeetsSecondMeets": 0}),
		})
		gitkit.Git(t, prime, "add", "excludes", "lingers", "meets")
		gitkit.Git(t, prime, "commit", "-q", "-m", "add packages whose tests pass only beside, or only apart from, another")
		// A Meets test passes only while another test runs beside it, so a serial run would fail both.
		// A Collides test fails while another's record is beside its own, so both fail side by side.
		// Each then reruns alone and passes.
		// An Overlaps test passes only while the coverage build of the next package, meets, runs beside it.
		env := append(withEnv(testMapMeetEnv, filepath.Join(t.TempDir(), "meet")), testMapCollideEnv+"="+t.TempDir(),
			testMapCoverBuildsEnv+"="+filepath.Join(t.TempDir(), "builds"), testMapOverlapEnv+"=meets")
		got := callWith(t, env, time.Minute, "excludes", "lingers", "meets")
		slices.Sort(got.PassedAlone)
		wantOutcome(t, got, testMapOutcome{
			Built:       []string{"excludes", "lingers", "meets"},
			PassedAlone: []string{"excludes.TestExcludesFirstCollides", "excludes.TestExcludesSecondCollides"},
		})
		if len(got.Failed) != 0 {
			t.Errorf("runTestMap() failed = %+v; want none", got.Failed)
		}
		wantEntries := map[string][]testcover.MapTest{
			"excludes": {{Name: "TestExcludesFirstCollides", Blocks: []int{0}}, {Name: "TestExcludesSecondCollides", Blocks: []int{0}}},
			"lingers":  {{Name: "TestLingersOverlaps", Blocks: []int{0}}},
			"meets":    {{Name: "TestMeetsFirstMeets", Blocks: []int{0}}, {Name: "TestMeetsSecondMeets", Blocks: []int{0}}},
		}
		for pkg, want := range wantEntries {
			if diff := cmp.Diff(want, readMap(t, pkg).Tests, cmpopts.EquateEmpty(), ignoreRunFields); diff != "" {
				t.Errorf("%s map tests mismatch (-want +got):\n%s", pkg, diff)
			}
		}
	})

	t.Run("ACallInsideAnEnclosingGateRunsSlotTakesNoSlot", func(t *testing.T) {
		location, err := lymcwd.ResolveWorktree(prime)
		if err != nil {
			t.Fatal(err)
		}
		// The test holds both slots, so a call that took a slot of its own would wait until its context ends.
		pool := hubgeom.GateSlots(location)
		holder := gateslot.Holder{Worktree: location.WorktreePath(), Site: "an enclosing gate run"}
		enclosing, err := pool.Acquire(context.Background(), holder)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { enclosing.Release() })
		other, err := pool.Acquire(context.Background(), holder)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { other.Release() })
		ctx, cancel := context.WithTimeout(lymcwd.WithCwd(context.Background(), prime), time.Minute)
		t.Cleanup(cancel)
		var out bytes.Buffer
		code := runTestMap(ctx, &out, request(enclosing.InheritEnv(environ), 150*time.Millisecond, "steady"))
		wantOutcome(t, decodeTestMap(t, code, out.Bytes()), testMapOutcome{Built: []string{"steady"}})
	})

	t.Run("ACallEndedDuringABuildOrATestWritesNothing", func(t *testing.T) {
		// Each row's stand-in appends a line to the started file once it hangs, which is when the call is cancelled.
		// The rows run in the order the call reaches them, and write nothing, so each starts from the same state.
		tests := []struct {
			name string
			env  func(started string) []string
		}{
			{"WhileTheGoListHangs", func(started string) []string { return withEnv(testMapHangListEnv, started) }},
			{"WhileTheCoverageBuildHangs", func(started string) []string {
				return append(withEnv(testMapCoverBuildsEnv, started), testMapHangBuildEnv+"="+module+"/alpha")
			}},
			{"WhileATestHangs", func(started string) []string { return withEnv(testMapHangingEnv, started) }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				before := readMap(t, "alpha")
				started := filepath.Join(t.TempDir(), "started")
				// The test would time out after 20 seconds, and the build after a minute, so the cancel comes first.
				req := request(tt.env(started), time.Second, "alpha")
				ctx, cancel := context.WithCancel(lymcwd.WithCwd(context.Background(), prime))
				t.Cleanup(cancel)
				var out bytes.Buffer
				done := make(chan int, 1)
				go func() { done <- runTestMap(ctx, &out, req) }()
				pollUntil(t, "the stand-in hangs", func() bool {
					_, err := os.Stat(started)
					return err == nil
				})
				cancel()
				select {
				case code := <-done:
					if code != 1 || !strings.Contains(out.String(), "the call was cancelled") {
						t.Errorf("runTestMap() = exit %d, output %q; want exit 1, naming the cancelled call", code, out.String())
					}
				case <-time.After(30 * time.Second):
					t.Fatal("the call did not end after it was cancelled")
				}
				if after := readMap(t, "alpha"); !after.BuiltAt.Equal(before.BuiltAt) {
					t.Errorf("alpha map built at %v; want it left from %v, since a call that was ended writes no entry and no failure", after.BuiltAt, before.BuiltAt)
				}
			})
		}
	})

	t.Run("AnAheadBuildTheWalkNoLongerNeedsIsCancelledRatherThanWaitedFor", func(t *testing.T) {
		// amoves goes first and its test moves HEAD, which ends the walk while the build of bhangs, started ahead, hangs.
		// That build would end by its timeout, a minute off, if the call waited for it.
		plankit.WriteTree(t, prime, map[string]string{
			"amoves/amoves.go":      testMapSource("amoves"),
			"amoves/amoves_test.go": testMapTestFile("amoves", map[string]int{"TestAmovesMoves": 0}),
			"bhangs/bhangs.go":      testMapSource("bhangs"),
			"bhangs/bhangs_test.go": testMapTestFile("bhangs", map[string]int{"TestBhangsOne": 0}),
		})
		gitkit.Git(t, prime, "add", "amoves", "bhangs")
		gitkit.Git(t, prime, "commit", "-q", "-m", "add a package that moves HEAD and one whose build hangs")
		head := gitkit.RevParse(t, prime, "HEAD")
		started := time.Now()
		got := callWith(t, withEnv(testMapHangBuildEnv, module+"/bhangs"), time.Minute, "amoves", "bhangs")
		if elapsed := time.Since(started); elapsed > 30*time.Second {
			t.Errorf("runTestMap() took %v; want it to end soon after the HEAD move, cancelling the hanging build", elapsed)
		}
		reason := headMovedReason(head, gitkit.RevParse(t, prime, "HEAD"))
		wantOutcome(t, got, testMapOutcome{Exit: 1, Unmeasured: []testMapSkip{{Package: "amoves", Reason: reason}, {Package: "bhangs", Reason: reason}}})
	})

	t.Run("ATestPastTheMemoryCapIsRecordedAsFailed", func(t *testing.T) {
		// The fixture test keeps testMapHoardMegabytes and passes unless the cap ends it first; the cap lies far below that.
		writeGate(t, "redundancy_test_memory_mb: 128\n"+gateYAML("parallel: 2, "))
		t.Cleanup(func() { writeGate(t, gateYAML("parallel: 2, ")) })
		plankit.WriteTree(t, prime, map[string]string{
			"capped/capped.go":      testMapSource("capped"),
			"capped/capped_test.go": testMapTestFile("capped", map[string]int{"TestCappedHoards": 0}),
		})
		gitkit.Git(t, prime, "add", "capped")
		gitkit.Git(t, prime, "commit", "-q", "-m", "add a package whose test keeps memory past the cap")
		got := callWith(t, environ, time.Minute, "capped")
		wantOutcome(t, got, testMapOutcome{Exit: 1, Built: []string{"capped"}, FailedTests: []string{"capped.TestCappedHoards"}})
		want := []testcover.MapTest{{Name: "TestCappedHoards", Outcome: testcover.OutcomeFailed}}
		if diff := cmp.Diff(want, readMap(t, "capped").Tests, cmpopts.EquateEmpty(), ignoreRunFields); diff != "" {
			t.Errorf("capped map tests mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("ATiersMemoryMBClampsItsTestsToOneAtATime", func(t *testing.T) {
		// tier0 sets memory_mb equal to the test cap, so one test fits its memory although parallel allows two.
		writeGate(t, "redundancy_test_memory_mb: 128\n"+strings.Replace(gateYAML("parallel: 2, "), "tier0: {command: x,", "tier0: {memory_mb: 128, command: x,", 1))
		t.Cleanup(func() { writeGate(t, gateYAML("parallel: 2, ")) })
		plankit.WriteTree(t, prime, map[string]string{
			"clamped/clamped.go":      testMapSource("clamped"),
			"clamped/clamped_test.go": testMapTestFile("clamped", map[string]int{"TestClampedFirstExcludes": 0, "TestClampedSecondExcludes": 0}),
		})
		gitkit.Git(t, prime, "add", "clamped")
		gitkit.Git(t, prime, "commit", "-q", "-m", "add a package whose two tests exclude each other")
		// An Excludes test run one at a time always passes, and two that overlap fail beside each other and pass alone.
		got := callWith(t, withEnv(testMapExcludeEnv, t.TempDir()), time.Minute, "clamped")
		wantOutcome(t, got, testMapOutcome{Built: []string{"clamped"}})
	})

	t.Run("ATiersTestsRunBeforeTheNextTiers", func(t *testing.T) {
		writeGate(t, gateYAML("parallel: 2, "))
		plankit.WriteTree(t, prime, map[string]string{
			"tiered/tiered.go":      testMapSource("tiered"),
			"tiered/tiered_test.go": testMapTestFile("tiered", map[string]int{"TestTieredFirstExcludes": 0, "TestTieredSecondExcludes": 1}),
		})
		gitkit.Git(t, prime, "add", "tiered")
		gitkit.Git(t, prime, "commit", "-q", "-m", "add a package whose tier0 and tier1 tests exclude each other")
		// Both tiers allow two at once, yet the tier0 test ends before the tier1 test starts, so neither fails beside the other.
		got := callWith(t, withEnv(testMapExcludeEnv, t.TempDir()), time.Minute, "tiered")
		wantOutcome(t, got, testMapOutcome{Built: []string{"tiered"}})
	})

	t.Run("ATestTheMemoryGuardKillsIsNamedAndTheRunGoesOn", func(t *testing.T) {
		// The guard's limit lies below the memory TestGuardedHoldsMemory keeps, and the test cap far above it.
		writeGate(t, "memory_quit_mb: 200\nredundancy_test_memory_mb: 1024\n"+gateYAML("parallel: 2, "))
		t.Cleanup(func() { writeGate(t, gateYAML("parallel: 2, ")) })
		plankit.WriteTree(t, prime, map[string]string{
			"guarded/guarded.go":      testMapSource("guarded"),
			"guarded/guarded_test.go": testMapTestFile("guarded", map[string]int{"TestGuardedHoldsMemory": 0, "TestGuardedOne": 0}),
		})
		gitkit.Git(t, prime, "add", "guarded")
		gitkit.Git(t, prime, "commit", "-q", "-m", "add a package whose test keeps memory past the guard's limit")
		req := request(environ, time.Second, "guarded")
		// A test runs under an enclosing lym gate test, whose ancestry turns the guard of its own runs off.
		// So it starts the guard itself.
		req.guardFor = func(ctx context.Context, limit int64, out io.Writer) (func(), func(int) bool) {
			guard := newMemoryGuard(limit, out)
			// The test process is no runaway test binary: only the test binaries it starts are sampled.
			guard.sample = func(root int) ([]proc.ProcessSample, error) {
				tree, err := proc.TreeSample(root)
				return slices.DeleteFunc(tree, func(sample proc.ProcessSample) bool { return sample.PID == root }), err
			}
			return startMemoryGuard(ctx, guard, os.Getpid(), true)
		}
		got := runTestMapOutcome(t, prime, req)
		wantOutcome(t, got, testMapOutcome{Exit: 1, Built: []string{"guarded"}, FailedTests: []string{"guarded.TestGuardedHoldsMemory"}})
		if !strings.Contains(got.Stderr, "killed the test binary") || !strings.Contains(got.Stderr, "TestGuardedHoldsMemory") {
			t.Errorf("stderr = %q; want the memory guard's line naming the test binary it killed and its test", got.Stderr)
		}
		want := []testcover.MapTest{{Name: "TestGuardedHoldsMemory", Outcome: testcover.OutcomeFailed}, {Name: "TestGuardedOne", Blocks: []int{0}}}
		if diff := cmp.Diff(want, readMap(t, "guarded").Tests, cmpopts.EquateEmpty(), ignoreRunFields); diff != "" {
			t.Errorf("guarded map tests mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("AMapRunHoldsTheLargerSlotAskOfItsTiers", func(t *testing.T) {
		location, err := lymcwd.ResolveWorktree(prime)
		if err != nil {
			t.Fatal(err)
		}
		// The test holds one of the hub's two slots, so a call asking for both waits and a call asking for one goes on.
		held, err := hubgeom.GateSlots(location).Acquire(context.Background(), gateslot.Holder{Worktree: location.WorktreePath(), Site: "a holder of one slot"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { held.Release() })
		t.Cleanup(func() { writeGate(t, gateYAML("parallel: 2, ")) })

		before := readMap(t, "steady")
		writeGate(t, strings.Replace(gateYAML("parallel: 2, "), "tier1: {command: x,", "tier1: {slots: all, command: x,", 1))
		ctx, cancel := context.WithTimeout(lymcwd.WithCwd(context.Background(), prime), 2*time.Second)
		t.Cleanup(cancel)
		var out bytes.Buffer
		code := runTestMap(ctx, &out, request(environ, 150*time.Millisecond, "steady"))
		if code != 1 || !strings.Contains(out.String(), "the call was cancelled") {
			t.Errorf("runTestMap() with tier1 asking every slot = exit %d, output %q; want exit 1, naming the cancelled call", code, out.String())
		}
		if after := readMap(t, "steady"); !after.BuiltAt.Equal(before.BuiltAt) {
			t.Errorf("steady map built at %v; want it left from %v, since the call never held its slots", after.BuiltAt, before.BuiltAt)
		}

		writeGate(t, gateYAML("parallel: 2, "))
		wantOutcome(t, call(t, "steady"), testMapOutcome{Built: []string{"steady"}})
	})

	t.Run("AMapRunWritesNoRunRecordAndAGreenRecordDoesNotStandInForIt", func(t *testing.T) {
		location, err := lymcwd.ResolveWorktree(prime)
		if err != nil {
			t.Fatal(err)
		}
		runLogs := gateslot.RunLogDir(hubgeom.GateSlots(location).Dir)
		greenPath := filepath.Join(gateslot.WaitDir(location.AnchorPath()), greenFileName)
		writeGate(t, strings.Replace(gateYAML("parallel: 2, "), "tier0: {command: x,", "tier0: {command: 'true {targets}', all_targets: ./..., ", 1))
		os.Remove(greenPath)
		t.Cleanup(func() {
			os.Remove(greenPath)
			writeGate(t, gateYAML("parallel: 2, "))
		})
		countRecords := func() int {
			t.Helper()
			records, err := gateslot.ReadRunRecords(runLogs)
			if err != nil {
				t.Fatal(err)
			}
			return len(records)
		}

		before := countRecords()
		wantOutcome(t, call(t, "steady"), testMapOutcome{Built: []string{"steady"}})
		if after := countRecords(); after != before {
			t.Errorf("run records after a map call = %d; want the %d before it, since a map run writes none", after, before)
		}

		var tierOut bytes.Buffer
		code := runTest(lymcwd.WithCwd(context.Background(), prime), &tierOut, testRequest{
			goBinary: standIn, timeoutUnit: time.Minute, secondUnit: time.Second, preTierCeiling: time.Minute, rerunDelay: time.Second,
			pipeWaitDelay: time.Second, tiers: []int{0}, targets: []string{"steady"},
		})
		if code != 0 {
			t.Fatalf("runTest tier 0 over steady = exit %d, %s; want exit 0", code, tierOut.Bytes())
		}
		records, err := gateslot.ReadRunRecords(runLogs)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != before+1 || records[len(records)-1].Carried || records[len(records)-1].Tier != 0 {
			t.Errorf("run records after the tier run = %+v; want one more than the %d before, its tier0 run not carried", records, before)
		}
		if _, err := os.Stat(greenPath); err != nil {
			t.Fatalf("stat the green record = %v; want the tier run to have recorded green", err)
		}

		greenAt := time.Now()
		wantOutcome(t, call(t, "steady"), testMapOutcome{Built: []string{"steady"}})
		if m := readMap(t, "steady"); len(m.Tests) != 1 || !m.Tests[0].MeasuredAt.After(greenAt) {
			t.Errorf("steady map tests = %+v; want its test measured again after the green record at %v", m.Tests, greenAt)
		}
	})
}

// TestGateTestMapLinkedPackages runs `lym gate test --map` calls in order over one hub.
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
		"caller/spawn_tier1_test.go": "package caller\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n\n" +
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
	// linked and linked/inner hold no test, so only a map keeps them in the package set of a call that names none.
	for _, pkg := range []string{"linked", "linked/inner"} {
		if err := testcover.WriteMap(mapsDir, pkg, testcover.Map{Commit: "c0ffee"}); err != nil {
			t.Fatal(err)
		}
	}
	standIn, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	builds := filepath.Join(t.TempDir(), "builds")
	environ := append(os.Environ(), helperEnv+"="+helperRoleTestMapGo, testMapModuleEnv+"="+module,
		testMapLinksEnv+"=caller=linked,dirtier=linked,follower=linked", testMapCoverBuildsEnv+"="+builds)
	call := func(t *testing.T, env []string, targets ...string) testMapOutcome {
		t.Helper()
		return callTestMap(t, prime, standIn, env, time.Minute, targets...)
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

	t.Run("ADirtyLinkedPackageLeavesTheMapAsItWas", func(t *testing.T) {
		callerBefore := readTestMap(t, mapsDir, "caller")
		followerBefore := readTestMap(t, mapsDir, "follower")
		linkedFile := filepath.Join(prime, "linked", "linked.go")
		if err := os.WriteFile(linkedFile, []byte(files["linked/linked.go"]+"// edited\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		// No dirty file keeps linked/inner from building.
		got := call(t, environ, "caller", "follower", "linked/inner")
		if err := os.WriteFile(linkedFile, []byte(files["linked/linked.go"]), 0o644); err != nil {
			t.Fatal(err)
		}
		reason := dirtyFileReason("linked/linked.go")
		wantOutcome(t, got, testMapOutcome{
			Exit:       1,
			Built:      []string{"linked/inner"},
			Unmeasured: []testMapSkip{{Package: "caller", Reason: reason}, {Package: "follower", Reason: reason}},
		})
		wantLeft(t, "caller", callerBefore)
		wantLeft(t, "follower", followerBefore)
	})

	t.Run("ADirtyRecordsFileLeavesAPackageMeasured", func(t *testing.T) {
		// A records-side path that collides with a package path is no file of that package.
		note := filepath.Join(h.PrimeRecords(), "caller", "records-note.txt")
		if err := os.MkdirAll(filepath.Dir(note), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(note, []byte("note\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(note) })
		wantOutcome(t, call(t, environ, "caller"), testMapOutcome{Built: []string{"caller"}})
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
		// So dirtier is checked dirty after its tests.
		// Follower, walked after dirtier, is checked dirty on a status read taken after its build.
		reason := dirtyFileReason("linked/scratch.txt")
		wantOutcome(t, got, testMapOutcome{Exit: 1, Unmeasured: []testMapSkip{{Package: "dirtier", Reason: reason}, {Package: "follower", Reason: reason}}})
		wantLeft(t, "dirtier", dirtierBefore)
		wantLeft(t, "follower", followerBefore)
	})

	t.Run("ALinkedFileChangedUnderAJobDiscardsAllItsRunsAndEndsTheCall", func(t *testing.T) {
		dirtierBefore := readTestMap(t, mapsDir, "dirtier")
		followerBefore := readTestMap(t, mapsDir, "follower")
		head := gitkit.RevParse(t, prime, "HEAD")
		got := call(t, withEnv(testMapLandEnv, "linked"), "dirtier", "follower")
		// TestDirtierLandsLinked moves HEAD.
		// The map is written once after all of dirtier's tests ran, so none of them is kept.
		// follower's job comes after the move, so the call ends there and lists it unmeasured beside dirtier.
		reason := headMovedReason(head, gitkit.RevParse(t, prime, "HEAD"))
		wantOutcome(t, got, testMapOutcome{Exit: 1, Unmeasured: []testMapSkip{{Package: "dirtier", Reason: reason}, {Package: "follower", Reason: reason}}})
		wantLeft(t, "dirtier", dirtierBefore)
		wantLeft(t, "follower", followerBefore)
	})

	t.Run("ALinkedFileAddedOrRemovedUnderAJobDiscardsIt", func(t *testing.T) {
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
				dirtierBefore := readTestMap(t, mapsDir, "dirtier")
				head := gitkit.RevParse(t, prime, "HEAD")
				got := call(t, tt.env, "dirtier")
				reason := headMovedReason(head, gitkit.RevParse(t, prime, "HEAD"))
				wantOutcome(t, got, testMapOutcome{Exit: 1, Unmeasured: []testMapSkip{{Package: "dirtier", Reason: reason}}})
				wantLeft(t, "dirtier", dirtierBefore)
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

	t.Run("AFailedListingThroughADirtyFileWritesNoMap", func(t *testing.T) {
		before := readTestMap(t, mapsDir, "caller")
		notes := filepath.Join(prime, "follower", "notes.txt")
		if err := os.WriteFile(notes, []byte("notes\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := call(t, withEnv(testMapFailListEnv, "1"), "caller")
		if err := os.Remove(notes); err != nil {
			t.Fatal(err)
		}
		wantOutcome(t, got, testMapOutcome{Exit: 1, Unmeasured: []testMapSkip{{Package: "caller", Reason: dirtyFileReason("follower/notes.txt")}}})
		wantLeft(t, "caller", before)
	})

	t.Run("AFailedBuildOverADirtyLinkedFileWritesNoMap", func(t *testing.T) {
		before := readTestMap(t, mapsDir, "caller")
		notes := filepath.Join(prime, "linked", "notes.txt")
		if err := os.WriteFile(notes, []byte("notes\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := call(t, withEnv(testMapFailBuildEnv, module+"/caller"), "caller")
		if err := os.Remove(notes); err != nil {
			t.Fatal(err)
		}
		wantOutcome(t, got, testMapOutcome{Exit: 1, Unmeasured: []testMapSkip{{Package: "caller", Reason: dirtyFileReason("linked/notes.txt")}}})
		wantLeft(t, "caller", before)
	})

	t.Run("APackageWhoseTestsAreAllSpawnMarkedIsCheckedBeforeItsWrite", func(t *testing.T) {
		plankit.WriteTree(t, prime, map[string]string{
			"spawner/spawner.go": testMapSource("spawner"),
			"spawner/spawner_test.go": "package spawner\n\nimport (\n\t\"os\"\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n\n" +
				"func TestSpawnerSpawns(t *testing.T) {\n\ttiergate.Require(t, 1)\n\t_, _ = os.Executable()\n}\n",
		})
		gitkit.Git(t, prime, "add", "spawner")
		gitkit.Git(t, prime, "commit", "-q", "-m", "add a package whose one test spawns")
		wantOutcome(t, call(t, environ, "spawner"), testMapOutcome{Built: []string{"spawner"}})
		before := readTestMap(t, mapsDir, "spawner")
		if len(before.Tests) != 1 || !before.Tests[0].Spawn.OutOfProcess {
			t.Fatalf("spawner map tests = %+v; want its one test marked out of process", before.Tests)
		}
		notes := filepath.Join(prime, "spawner", "notes.txt")
		if err := os.WriteFile(notes, []byte("notes\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := call(t, environ, "spawner")
		if err := os.Remove(notes); err != nil {
			t.Fatal(err)
		}
		wantOutcome(t, got, testMapOutcome{Exit: 1, Unmeasured: []testMapSkip{{Package: "spawner", Reason: dirtyFileReason("spawner/notes.txt")}}})
		wantLeft(t, "spawner", before)
	})

	t.Run("AGoListFailureKeepsTheMapsEntriesAndRecordsTheFailureWithHead", func(t *testing.T) {
		before := readTestMap(t, mapsDir, "caller")
		head := gitkit.RevParse(t, prime, "HEAD")
		got := call(t, withEnv(testMapFailListEnv, "1"), "caller")
		if len(got.Built) != 0 || len(got.Failed) != 1 || got.Failed[0].Package != "caller" || !strings.Contains(got.Failed[0].Failure, "list linked packages") {
			t.Fatalf("runTestMap() = built %v, failed %+v; want caller alone failed, naming the list failure", got.Built, got.Failed)
		}
		if got.Exit != 1 {
			t.Errorf("runTestMap() exit = %d; want 1, since the build failed", got.Exit)
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
		if got.Exit != 1 {
			t.Errorf("runTestMap() exit = %d; want 1, since the spawn scan failed", got.Exit)
		}
		failed := readTestMap(t, mapsDir, "garbled")
		if failed.BuildFailure != got.Failed[0].Failure || failed.Format == nil {
			t.Errorf("garbled map = failure %q, format %+v; want the spawn scan failure and a marker", failed.BuildFailure, failed.Format)
		}
	})

	t.Run("ASpawnScanFailureOverADirtyFileWritesNoMap", func(t *testing.T) {
		before := readTestMap(t, mapsDir, "garbled")
		notes := filepath.Join(prime, "garbled", "notes.txt")
		if err := os.WriteFile(notes, []byte("notes\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := call(t, environ, "garbled")
		if err := os.Remove(notes); err != nil {
			t.Fatal(err)
		}
		wantOutcome(t, got, testMapOutcome{Exit: 1, Unmeasured: []testMapSkip{{Package: "garbled", Reason: dirtyFileReason("garbled/notes.txt")}}})
		wantLeft(t, "garbled", before)
	})

	t.Run("AGoEnvFailureEndsTheWalk", func(t *testing.T) {
		linkedBefore := readTestMap(t, mapsDir, "linked")
		innerBefore := readTestMap(t, mapsDir, "linked/inner")
		code, out, _ := runTestMapIn(prime, standIn, withEnv(testMapFailGoEnvEnv, "1"), time.Minute, "linked", "linked/inner")
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

// testMapVerbCounts holds the packages key of a `lym gate test --map` envelope, which testMapOutcome leaves out.
type testMapVerbCounts struct {
	Packages []testMapMeasure `json:"packages"`
}

// TestGateTestMapCLI runs `lym gate test --map` through its cobra command in a child process over a hub of two slots.
func TestGateTestMapCLI(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	gate := "slots: 2\nredundancy_build_timeout_min: 1\nredundancy_test_timeout_sec: 20\ntiers:\n" +
		"  tier0: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"0\"}}\n"

	t.Run("TheDetachedRunnersReplayWritesTheMap", func(t *testing.T) {
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
		const module = "example.com/replayed"
		prime := seedTestMapModule(t, h, module, gate, map[string]map[string]int{"p0": {"TestPackageOne": 0}})
		mapsDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
		environ := append(os.Environ(), testMapModuleEnv+"="+module)

		// The detached runner replays `test --wait` followed by the call's words.
		args := append([]string{"test", "--wait"}, testRequest{mapMode: true, targets: []string{"p0"}}.words()...)
		cmd, out := startTestMapCLI(t, prime, environ, args...)
		if got := decodeTestMap(t, waitTestMapCLI(t, cmd), out.Bytes()); !slices.Equal(got.Built, []string{"p0"}) {
			t.Errorf("%v built %v; want p0", args, got.Built)
		}
		if m := readTestMap(t, mapsDir, "p0"); len(m.Tests) != 1 {
			t.Errorf("p0 map tests = %+v; want its one test measured", m.Tests)
		}
	})

	t.Run("ATerminatingSignalWritesNoMapAndExitsWithItsNumber", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows has no catchable terminating signal to send")
		}
		t.Parallel()
		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
		const module = "example.com/signalled"
		prime := seedTestMapModule(t, h, module, gate, map[string]map[string]int{"yielding": {"TestYieldingAOne": 0, "TestYieldingHangs": 0}})
		mapsDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
		hanging := filepath.Join(t.TempDir(), "hanging")
		environ := append(os.Environ(), testMapModuleEnv+"="+module, testMapHangingEnv+"="+hanging)

		cmd, out := startTestMapCLI(t, prime, environ, "test", "--wait", "--map")
		pollUntil(t, "the call runs its hanging test", func() bool {
			_, err := os.Stat(hanging)
			return err == nil
		})
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		if code, want := waitTestMapCLI(t, cmd), 128+int(syscall.SIGTERM); code != want {
			t.Errorf("testmap exit = %d, output %s; want %d, 128 plus the signal number", code, out.Bytes(), want)
		}
		// The map is written once after all of a package's tests ran, so TestYieldingAOne's finished run is not kept.
		if _, found, err := testcover.ReadMap(mapsDir, "yielding"); err != nil || found {
			t.Errorf("ReadMap(yielding) = found %v, err %v; want no map for a call that was ended before its package's tests all ran", found, err)
		}
	})
}
