// redundancy_integration_test.go drives the redundancy report step over a hubforge hub.
// Its task pair holds a small Go module and diffs from the prime's branch.
// The step judges the tests the pair adds against coverage maps the test writes.
// The testmap stand-in acts as `go` and as the coverage binaries it builds, so no test builds a real binary.
// It is Tier 1: it builds a real hub and spawns git and the stand-ins.

package gatecli

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
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
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// judgedTestFile returns a test file of pkg holding funcs, each a whole Test function's source.
func judgedTestFile(pkg string, funcs ...string) string {
	return "package " + pkg + "\n\nimport (\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n\n" + strings.Join(funcs, "\n\n") + "\n"
}

// tieredTest returns the source of a Test function name.
// Its body opens with tiergate.Require for tier, then holds extra.
func tieredTest(name string, tier int, extra string) string {
	return fmt.Sprintf("func %s(t *testing.T) {\n\ttiergate.Require(t, %d)\n%s}", name, tier, extra)
}

// TestRedundancyStep runs the redundancy report step in order over one hub.
// Each subtest works on the tree and report the last one left.
// Its subtests share that state and run in sequence, so none is parallel.
func TestRedundancyStep(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	prime, pair := h.PrimeWorktree(), h.PairCodeWorktree(hubforge.TemplatePairSlug)
	const module = "example.com/judged"
	// A test past 20 seconds of 150 milliseconds each times out: 3 seconds.
	// The hub declares tier0 and tier2 but not tier1.
	// tier1 is the line of a tier1 the hub declares beside tier0 and tier2, or empty for a hub with no tier1.
	writeGate := func(t *testing.T, budgetMin int, tier1 string) {
		t.Helper()
		content := "slots: 1\nscoper: go\nredundancy_build_timeout_min: 1\nredundancy_test_timeout_sec: 20\nredundancy_max_tests: 9\n" +
			fmt.Sprintf("redundancy_budget_min: %d\ntiers:\n", budgetMin) +
			"  tier0: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"0\"}}\n" +
			"  tier2: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"2\"}}\n" + tier1
		if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeGate(t, 9, "")

	packages := []string{"alpha", "beta", "broken", "drops", "gone", "moved", "nomap", "stale", "zcapped"}
	seeded := map[string]string{"go.mod": "module " + module + "\n\ngo 1.21\n"}
	for _, pkg := range packages {
		seeded[pkg+"/"+pkg+".go"] = testMapSource(pkg)
	}
	seeded["alpha/alpha_test.go"] = judgedTestFile("alpha", tieredTest("TestAlphaBase", 0, ""))
	seeded["beta/beta_test.go"] = judgedTestFile("beta", tieredTest("TestBetaOne", 0, ""))
	seeded["broken/broken_test.go"] = judgedTestFile("broken", tieredTest("TestBrokenOne", 0, ""))
	seeded["drops/drops_test.go"] = judgedTestFile("drops", tieredTest("TestDropsOne", 0, ""), tieredTest("TestDropsPlain", 0, ""))
	seeded["gone/gone_test.go"] = judgedTestFile("gone", tieredTest("TestGoneOne", 0, ""))
	seeded["moved/a_test.go"] = judgedTestFile("moved", tieredTest("TestMovedOne", 0, ""))
	seeded["nomap/nomap_test.go"] = judgedTestFile("nomap", tieredTest("TestNomapOne", 0, ""))
	seeded["stale/stale_test.go"] = judgedTestFile("stale", tieredTest("TestStaleOld", 0, ""))
	seeded["zcapped/zcapped_test.go"] = judgedTestFile("zcapped", tieredTest("TestZcappedOne", 0, ""))
	plankit.WriteTree(t, prime, seeded)
	gitkit.Git(t, prime, append([]string{"add", "go.mod"}, packages...)...)
	gitkit.Git(t, prime, "commit", "-q", "-m", "seed the judged module")
	gitkit.Git(t, pair, "merge", "--ff-only", "-q", "main")

	// The pair adds tests to most packages, edits TestBetaOne, moves moved's test file and deletes gone's.
	staleTestFile := judgedTestFile("stale", tieredTest("TestStaleOld", 0, ""), tieredTest("TestStaleNew", 0, ""))
	plankit.WriteTree(t, pair, map[string]string{
		"alpha/alpha_test.go":   judgedTestFile("alpha", tieredTest("TestAlphaBase", 0, ""), tieredTest("TestAlphaNewOne", 0, ""), tieredTest("TestAlphaNewTwo", 0, "")),
		"beta/beta_test.go":     judgedTestFile("beta", tieredTest("TestBetaNewOne", 0, ""), tieredTest("TestBetaOne", 0, "\tt.Log(\"edited\")\n")),
		"broken/broken_test.go": judgedTestFile("broken", tieredTest("TestBrokenOne", 0, ""), tieredTest("TestBrokenNew", 0, "")),
		"drops/drops_test.go": judgedTestFile("drops",
			tieredTest("TestDropsOne", 0, ""), tieredTest("TestDropsPlain", 0, ""), tieredTest("TestDropsHangs", 0, ""),
			"//testtiming:keep pins the slow path on purpose\n"+tieredTest("TestDropsKept", 0, ""),
			tieredTest("TestDropsLate", 0, ""), tieredTest("TestDropsTier1", 1, ""), tieredTest("TestDropsTier1Again", 1, ""),
			tieredTest("TestDropsTier2", 2, "")),
		// A test in a file built under a tier tag that runs the go command is marked out of process.
		"drops/spawn_tier1_test.go": "//go:build tier1\n\npackage drops\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestDropsSpawns(t *testing.T) {\n\t_, _ = os.Executable()\n}\n",
		"moved/b_test.go":           seeded["moved/a_test.go"],
		"nomap/nomap_test.go":       judgedTestFile("nomap", tieredTest("TestNomapOne", 0, ""), tieredTest("TestNomapNew", 0, "")),
		"stale/stale_test.go":       staleTestFile,
		"zcapped/zcapped_test.go":   judgedTestFile("zcapped", tieredTest("TestZcappedOne", 0, ""), tieredTest("TestZcappedNew", 0, "")),
	})
	for _, rel := range []string{"moved/a_test.go", "gone/gone_test.go"} {
		if err := os.Remove(filepath.Join(pair, filepath.FromSlash(rel))); err != nil {
			t.Fatal(err)
		}
	}

	// The maps record the seeded files; each keys the two one-line functions every fixture package holds.
	mapsDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
	seededFiles := func(t *testing.T, pkg string) map[string]string {
		t.Helper()
		files := map[string]string{}
		for rel, content := range seeded {
			if dir, name := filepath.Split(rel); dir == pkg+"/" {
				files[name] = testcover.ContentHash([]byte(content))
			}
		}
		return files
	}
	blockKeys := func(t *testing.T, pkg string) []testcover.BlockKey {
		t.Helper()
		file := module + "/" + pkg + "/" + pkg + ".go"
		keys, err := testcover.KeyBlocks(filepath.Join(prime, pkg), []testcover.ProfileBlock{
			{File: file, StartLine: 3, StartCol: 16, EndLine: 3, EndCol: 28, Statements: 1},
			{File: file, StartLine: 5, StartCol: 16, EndLine: 5, EndCol: 28, Statements: 1},
		})
		if err != nil {
			t.Fatal(err)
		}
		return keys
	}
	// mapTest returns the map entry of the seeded test name of pkg, covering the first function alone.
	mapTest := func(t *testing.T, pkg, name string) testcover.MapTest {
		t.Helper()
		rel := pkg + "/" + pkg + "_test.go"
		funcs, err := testcover.ParseTestFuncs(rel, []byte(seeded[rel]))
		if err != nil || len(funcs) != 1 || funcs[0].Name != name {
			t.Fatalf("ParseTestFuncs(%s) = %+v, %v; want %s alone", rel, funcs, err, name)
		}
		return testcover.MapTest{Name: name, FuncHash: funcs[0].Hash, Seconds: 0.01, Blocks: []int{0}}
	}
	writeMap := func(t *testing.T, pkg string, m testcover.Map) {
		t.Helper()
		if err := testcover.WriteMap(mapsDir, pkg, m); err != nil {
			t.Fatal(err)
		}
	}
	builtAt := time.Now().Add(-2 * time.Hour)
	mapOf := func(t *testing.T, pkg string, tests ...testcover.MapTest) testcover.Map {
		t.Helper()
		return testcover.Map{Commit: "c0ffee", BuiltAt: builtAt, Files: seededFiles(t, pkg), Blocks: blockKeys(t, pkg), Tests: tests}
	}
	alphaMap := mapOf(t, "alpha", mapTest(t, "alpha", "TestAlphaBase"))
	alphaMap.Files["alpha_test.go"] = "stale"
	writeMap(t, "alpha", alphaMap)
	// beta's map also records a linked package whose file differs at the merge base, and a function of it TestBetaOne covers.
	// Neither changes beta's verdicts, and its drift names no file of the linked package.
	// It also records a failed build at a later commit beside its entry, which still judges beta's tests.
	betaMap := mapOf(t, "beta", mapTest(t, "beta", "TestBetaOne"))
	betaMap.Linked = map[string]map[string]string{module + "/alpha": {"alpha.go": "stale"}}
	betaMap.Funcs = []testcover.FuncKey{{Package: module + "/alpha", File: "alpha.go", Func: "One"}}
	betaMap.Tests[0].Funcs = []int{0}
	betaMap.Failure = &testcover.MapFailure{Commit: "c0ffee2", Error: "the coverage build failed later"}
	writeMap(t, "beta", betaMap)
	// drops' map records a failed run of TestDropsOne.
	// It also records a spawn-marked entry of TestDropsPlain.
	// Each covers the block TestDropsLate covers, and neither is a cover, so TestDropsLate stays unique.
	dropsFuncs, err := testcover.ParseTestFuncs("drops/drops_test.go", []byte(seeded["drops/drops_test.go"]))
	if err != nil {
		t.Fatal(err)
	}
	dropsEntry := func(name string) testcover.MapTest {
		t.Helper()
		for _, fn := range dropsFuncs {
			if fn.Name == name {
				return testcover.MapTest{Name: name, FuncHash: fn.Hash, Seconds: 0.01, Blocks: []int{0}}
			}
		}
		t.Fatalf("ParseTestFuncs(drops) = %+v; want %s", dropsFuncs, name)
		return testcover.MapTest{}
	}
	failedRun := dropsEntry("TestDropsOne")
	failedRun.Outcome = testcover.OutcomeFailed
	spawned := dropsEntry("TestDropsPlain")
	spawned.Spawn = testcover.SpawnVerdict{OutOfProcess: true}
	writeMap(t, "drops", mapOf(t, "drops", failedRun, spawned))
	broken := mapOf(t, "broken")
	broken.Blocks, broken.BuildFailure = nil, "the coverage build failed"
	writeMap(t, "broken", broken)
	// The map's entry for TestStaleOld records a function text the package no longer holds, so the map test is not counted.
	staleOld := mapTest(t, "stale", "TestStaleOld")
	staleOld.FuncHash = "the text of an earlier TestStaleOld"
	// The map also holds TestStaleNew, a judged test, with the text it has now.
	// Only the exclusion of a judged test keeps it from covering itself.
	staleFuncs, err := testcover.ParseTestFuncs("stale/stale_test.go", []byte(staleTestFile))
	if err != nil {
		t.Fatal(err)
	}
	staleNew := testcover.MapTest{Name: "TestStaleNew", Seconds: 0.01, Blocks: []int{0}}
	for _, fn := range staleFuncs {
		if fn.Name == staleNew.Name {
			staleNew.FuncHash = fn.Hash
		}
	}
	if staleNew.FuncHash == "" {
		t.Fatalf("ParseTestFuncs(stale) = %+v; want TestStaleNew", staleFuncs)
	}
	writeMap(t, "stale", mapOf(t, "stale", staleOld, staleNew))

	pairLocation, err := lymcwd.ResolveWorktree(pair)
	if err != nil {
		t.Fatal(err)
	}
	primeLocation, err := lymcwd.ResolveWorktree(prime)
	if err != nil {
		t.Fatal(err)
	}
	standIn, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	environ := append(os.Environ(), helperEnv+"="+helperRoleTestMapGo, testMapModuleEnv+"="+module)
	// stepAt returns the step of a darn-write call in the worktree at location, with one budget minute lasting minuteUnit.
	stepAt := func(t *testing.T, location *lymcwd.Location, env []string, minuteUnit time.Duration) (redundancyStep, *bytes.Buffer) {
		t.Helper()
		cfg, err := gateslot.LoadConfig(fabricengine.ConfigDirs(location))
		if err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		return redundancyStep{
			ctx: context.Background(), stderr: &stderr, cfg: cfg.WithDefaultDir(location.AnchorRel), location: location,
			root: location.WorktreePath(), gateKey: "darn-write", pool: hubgeom.GateSlots(location), env: env, goBinary: standIn,
			minuteUnit: minuteUnit, secondUnit: 150 * time.Millisecond, grace: time.Second,
		}, &stderr
	}
	readReport := func(t *testing.T, location *lymcwd.Location) RedundancyReport {
		t.Helper()
		report, found, err := ReadRedundancyReport(location)
		if err != nil || !found {
			t.Fatalf("ReadRedundancyReport() = found %v, err %v; want a report", found, err)
		}
		return report
	}
	runStep := func(t *testing.T) RedundancyReport {
		t.Helper()
		step, _ := stepAt(t, pairLocation, environ, time.Minute)
		step.run(false, true)
		return readReport(t, pairLocation)
	}
	notJudged := func(pkg, name, reason string) RedundancyTest {
		return RedundancyTest{Package: pkg, Name: name, File: pkg + "/" + pkg + "_test.go", Verdict: RedundancyNotJudged, Reason: reason}
	}

	var first RedundancyReport
	t.Run("JudgesTheDiffsNewAndChangedTestsAgainstTheMaps", func(t *testing.T) {
		first = runStep(t)
		wantTests := []RedundancyTest{
			{Package: "alpha", Name: "TestAlphaNewOne", File: "alpha/alpha_test.go", Verdict: RedundancyContained, Covering: []string{"TestAlphaBase"}},
			{Package: "alpha", Name: "TestAlphaNewTwo", File: "alpha/alpha_test.go", Verdict: RedundancyUnique},
			{Package: "beta", Name: "TestBetaNewOne", File: "beta/beta_test.go", Verdict: RedundancyContained, Covering: []string{"TestBetaOne"}},
			{Package: "beta", Name: "TestBetaOne", File: "beta/beta_test.go", Verdict: RedundancyUnique},
			notJudged("broken", "TestBrokenNew", reasonMapFailure),
			notJudged("drops", "TestDropsHangs", "timed out after 3s"),
			notJudged("drops", "TestDropsKept", "kept: pins the slow path on purpose"),
			{Package: "drops", Name: "TestDropsLate", File: "drops/drops_test.go", Verdict: RedundancyUnique},
			notJudged("drops", "TestDropsTier1", "tier1, which the hub does not declare"),
			notJudged("drops", "TestDropsTier1Again", "tier1, which the hub does not declare"),
			notJudged("drops", "TestDropsTier2", "a tier2 test, which no coverage map holds"),
			{Package: "drops", Name: "TestDropsSpawns", File: "drops/spawn_tier1_test.go", Verdict: RedundancyNotJudged, Reason: testcover.ReasonOutOfProcess},
			notJudged("nomap", "TestNomapNew", reasonNoMap),
			{Package: "stale", Name: "TestStaleNew", File: "stale/stale_test.go", Verdict: RedundancyUnique},
			notJudged("zcapped", "TestZcappedNew", "past redundancy_max_tests (9)"),
		}
		if diff := cmp.Diff(wantTests, first.Tests, cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("report tests mismatch (-want +got):\n%s", diff)
		}
		wantPackages := []RedundancyPackage{
			{Package: "alpha", Judged: true, MapCommit: "c0ffee", Drift: []string{"alpha_test.go"}},
			{Package: "beta", Judged: true, MapCommit: "c0ffee"},
			{Package: "broken", Reason: "its coverage map records a build failure: the coverage build failed"},
			{Package: "drops", Judged: true, MapCommit: "c0ffee"},
			{Package: "nomap", Reason: "it has no coverage map"},
			{Package: "stale", Judged: true, MapCommit: "c0ffee"},
		}
		if diff := cmp.Diff(wantPackages, first.Packages, cmpopts.EquateEmpty(), cmpopts.IgnoreFields(RedundancyPackage{}, "MapBuiltAt", "MapAge")); diff != "" {
			t.Errorf("report packages mismatch (-want +got):\n%s", diff)
		}
		if age := first.Packages[0].MapAge; age != "2h0m0s" {
			t.Errorf("alpha map age = %q; want 2h0m0s", age)
		}
		wantTaken := map[string][]string{
			"alpha": {"TestAlphaNewOne", "TestAlphaNewTwo"}, "beta": {"TestBetaNewOne", "TestBetaOne"}, "broken": {"TestBrokenNew"},
			"drops": {"TestDropsHangs", "TestDropsKept", "TestDropsLate", "TestDropsSpawns", "TestDropsTier1", "TestDropsTier1Again", "TestDropsTier2"},
			"nomap": {"TestNomapNew"}, "stale": {"TestStaleNew"}, "zcapped": {"TestZcappedNew"},
		}
		gotTaken := map[string][]string{}
		for pkg, hashes := range first.TestHashes {
			gotTaken[pkg] = slices.Sorted(maps.Keys(hashes))
		}
		if diff := cmp.Diff(wantTaken, gotTaken); diff != "" {
			t.Errorf("report test hashes mismatch (-want +got):\n%s; want no test of the moved file or the deleted one", diff)
		}
		if got := slices.Sorted(maps.Keys(first.FileHashes)); !slices.Equal(got, []string{"alpha", "beta", "drops", "stale"}) {
			t.Errorf("report file hashes name %v; want the judged packages alpha, beta, drops and stale", got)
		}
		if len(first.Errors) != 0 || first.Note != "" || first.GateKey != "darn-write" {
			t.Errorf("report = errors %q, note %q, gate key %q; want no error, no note and darn-write", first.Errors, first.Note, first.GateKey)
		}
	})

	t.Run("ARepeatRunsAgainWhileAPackageHasNoUsableMap", func(t *testing.T) {
		if again := runStep(t); !again.Written.After(first.Written) {
			t.Errorf("report written %v; want a report written after %v, since nomap has no map and broken's records a failure", again.Written, first.Written)
		}
	})

	var kept RedundancyReport
	t.Run("ARepeatOnAMatchingTreeRunsNoBuildAndKeepsTheReport", func(t *testing.T) {
		writeMap(t, "nomap", mapOf(t, "nomap"))
		writeMap(t, "broken", mapOf(t, "broken"))
		kept = runStep(t)
		if kept.retries() {
			t.Fatalf("report = %+v; want one that asks for no retry", kept)
		}
		builds := filepath.Join(t.TempDir(), "builds")
		step, stderr := stepAt(t, pairLocation, append(slices.Clone(environ), testMapCoverBuildsEnv+"="+builds), time.Minute)
		step.run(false, true)
		if again := readReport(t, pairLocation); !again.Written.Equal(kept.Written) {
			t.Errorf("report written %v; want the report of %v kept", again.Written, kept.Written)
		}
		if _, err := os.Stat(builds); err == nil {
			t.Error("the repeat started a coverage build; want none on a matching tree")
		}
		if !strings.Contains(stderr.String(), "matches the report already written") {
			t.Errorf("stderr = %q; want the line saying the report was kept", stderr.String())
		}
	})

	t.Run("TheReportMatchesACommitOfItsTreeUntilATakenTestChanges", func(t *testing.T) {
		gitkit.Git(t, pair, append([]string{"add", "-A", "--"}, packages...)...)
		gitkit.Git(t, pair, "commit", "-q", "-m", "commit the judged tree")
		wantMatch := func(t *testing.T, want bool) {
			t.Helper()
			head := gitkit.RevParse(t, pair, "HEAD")
			if got, err := RedundancyReportMatches(pairLocation, kept, head); err != nil || got != want {
				t.Errorf("RedundancyReportMatches(%s) = %v, %v; want %v", head, got, err, want)
			}
		}
		wantMatch(t, true)
		// The pair's _lym is a link that git cannot add through, so the record sits beside the packages.
		plankit.WriteTree(t, pair, map[string]string{"records/notes.md": "note\n"})
		gitkit.Git(t, pair, "add", "records/notes.md")
		gitkit.Git(t, pair, "commit", "-q", "-m", "touch only run records")
		wantMatch(t, true)
		plankit.WriteTree(t, pair, map[string]string{"alpha/alpha.go": testMapSource("alpha") + "// edited\n"})
		gitkit.Git(t, pair, "commit", "-q", "-am", "edit a Go file of a judged package")
		wantMatch(t, false)
		plankit.WriteTree(t, pair, map[string]string{"alpha/alpha.go": testMapSource("alpha")})
		gitkit.Git(t, pair, "commit", "-q", "-am", "restore the Go file of the judged package")
		wantMatch(t, true)
		plankit.WriteTree(t, pair, map[string]string{
			"alpha/alpha_test.go": judgedTestFile("alpha", tieredTest("TestAlphaBase", 0, ""), tieredTest("TestAlphaNewOne", 0, ""), tieredTest("TestAlphaNewTwo", 0, "\tt.Log(\"edited\")\n")),
		})
		gitkit.Git(t, pair, "commit", "-q", "-am", "edit an alpha test")
		wantMatch(t, false)
	})

	t.Run("ASpentBudgetStartsNoBuild", func(t *testing.T) {
		writeGate(t, 1, "")
		builds := filepath.Join(t.TempDir(), "builds")
		step, _ := stepAt(t, pairLocation, append(slices.Clone(environ), testMapCoverBuildsEnv+"="+builds), time.Nanosecond)
		step.run(false, true)
		report := readReport(t, pairLocation)
		for _, test := range report.Tests {
			if test.Package == "alpha" || test.Package == "beta" || test.Package == "broken" {
				if test.Verdict != RedundancyNotJudged || test.Reason != reasonBudgetSpent {
					t.Errorf("test %s.%s = %s, %q; want not judged for the spent budget", test.Package, test.Name, test.Verdict, test.Reason)
				}
			}
		}
		if _, err := os.Stat(builds); err == nil {
			t.Error("the step started a coverage build; want none once the budget is spent")
		}
		if !report.retries() {
			t.Error("report retries() = false; want a report with a test the budget left unrun to be retried")
		}
	})

	t.Run("ABudgetSpentMidPackageListsTheRestOfThePackageAsNotJudged", func(t *testing.T) {
		// TestDropsHangs runs to its 5 second timeout, past the 3 second budget that counts from the step's slot hold.
		writeGate(t, 1, "")
		step, _ := stepAt(t, pairLocation, environ, 3*time.Second)
		step.secondUnit = 250 * time.Millisecond
		step.run(false, true)
		report := readReport(t, pairLocation)
		i := slices.IndexFunc(report.Tests, func(test RedundancyTest) bool { return test.Name == "TestDropsLate" })
		if i < 0 {
			t.Fatalf("report tests = %+v; want a line for TestDropsLate", report.Tests)
		}
		if late := report.Tests[i]; late.Verdict != RedundancyNotJudged || late.Reason != reasonBudgetSpent {
			t.Errorf("TestDropsLate = %s, %q; want not judged for the spent budget", late.Verdict, late.Reason)
		}
	})

	t.Run("ACallCancelledMidRunHoldsASlotUntilThenAndListsTheTestInFlightAndTheRestAsCancelled", func(t *testing.T) {
		writeGate(t, 9, "")
		hanging := filepath.Join(t.TempDir(), "hanging")
		step, _ := stepAt(t, pairLocation, append(slices.Clone(environ), testMapHangingEnv+"="+hanging), time.Minute)
		// TestDropsHangs would time out after 20 seconds; the cancel comes first.
		step.secondUnit = time.Second
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		step.ctx = ctx
		done := make(chan struct{})
		go func() {
			defer close(done)
			step.run(false, true)
		}()
		waitDone := func() {
			t.Helper()
			select {
			case <-done:
			case <-time.After(30 * time.Second):
				t.Fatal("the step did not end after its call was cancelled")
			}
		}
		slotHeld := func() bool {
			t.Helper()
			holders, err := step.pool.Holders()
			if err != nil {
				t.Fatal(err)
			}
			return slices.ContainsFunc(holders, func(h gateslot.Holder) bool {
				return h.Site == redundancySite && h.Worktree == pairLocation.WorktreePath()
			})
		}
		for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			if _, err := os.Stat(hanging); err == nil {
				break
			}
			if time.Now().After(deadline) {
				cancel()
				waitDone()
				t.Fatal("TestDropsHangs did not start")
			}
		}
		if !slotHeld() {
			t.Error("no gate slot held by the redundancy step while its test runs; want one")
		}
		cancel()
		waitDone()
		if slotHeld() {
			t.Error("the redundancy step still holds a gate slot after it ended; want it released")
		}
		report := readReport(t, pairLocation)
		reasons := map[string]string{}
		for _, test := range report.Tests {
			if test.Verdict == RedundancyNotJudged {
				reasons[test.Name] = test.Reason
			}
		}
		for _, name := range []string{"TestDropsHangs", "TestDropsLate"} {
			if reasons[name] != reasonCancelled {
				t.Errorf("%s = not judged for %q; want %q", name, reasons[name], reasonCancelled)
			}
		}
		if !report.retries() {
			t.Error("report retries() = false; want a report with cancelled tests to be retried")
		}
	})

	t.Run("ABrokenLymBuildAndACorruptMapAreErrorLinesThatLeaveTheOtherTestsJudged", func(t *testing.T) {
		writeGate(t, 9, "  tier1: {command: x, timeout_min: 1, counter: go-json, prebuilt_lym: true, env: {LYM_TEST_TIER: \"1\"}}\n")
		t.Cleanup(func() { writeGate(t, 9, "") })
		// The map names block 9 of a table of 2.
		writeMap(t, "alpha", mapOf(t, "alpha", testcover.MapTest{Name: "TestAlphaBase", FuncHash: "x", Seconds: 0.01, Blocks: []int{9}}))
		t.Cleanup(func() { writeMap(t, "alpha", alphaMap) })
		// The prebuild builds `lym` only for a worktree holding cmd/lym.
		plankit.WriteTree(t, pair, map[string]string{"cmd/lym/main.go": "package main\n"})
		t.Cleanup(func() {
			_ = os.RemoveAll(filepath.Join(pair, "cmd", "lym"))
			_ = os.Remove(filepath.Join(pair, "cmd"))
		})

		builds := filepath.Join(t.TempDir(), "lym-builds")
		// The default 150 millisecond second unit gives each test 3 seconds, so a loaded machine does not time out TestBetaNewOne.
		step, _ := stepAt(t, pairLocation, append(slices.Clone(environ), testMapLymBuildsEnv+"="+builds, testMapFailLymBuildEnv+"=1"), time.Minute)
		step.run(false, true)
		report := readReport(t, pairLocation)

		var lymErrors, mapErrors int
		for _, line := range report.Errors {
			switch {
			case strings.HasPrefix(line, "lym build: "):
				lymErrors++
			case strings.HasPrefix(line, "coverage map of alpha: "):
				mapErrors++
			}
		}
		if lymErrors != 1 || mapErrors != 1 || len(report.Errors) != 2 {
			t.Errorf("report errors = %q; want one lym build line and one alpha map line", report.Errors)
		}
		if data, err := os.ReadFile(builds); err != nil || bytes.Count(data, []byte("\n")) != 1 {
			t.Errorf("lym builds file = %q, %v; want one build for both tier1 tests", data, err)
		}
		wantReasons := map[string]string{
			"TestDropsTier1": reasonLymFailed, "TestDropsTier1Again": reasonLymFailed,
			"TestAlphaNewOne": reasonMapUnreadable, "TestAlphaNewTwo": reasonMapUnreadable,
		}
		for _, test := range report.Tests {
			if want, listed := wantReasons[test.Name]; listed && (test.Verdict != RedundancyNotJudged || test.Reason != want) {
				t.Errorf("test %s = %s, %q; want not judged for %q", test.Name, test.Verdict, test.Reason, want)
			}
			if test.Name == "TestBetaNewOne" && test.Verdict != RedundancyContained {
				t.Errorf("TestBetaNewOne = %s, %q; want contained, since the failures left beta's package judged", test.Verdict, test.Reason)
			}
		}
		if alpha := report.Packages[0]; alpha.Package != "alpha" || alpha.Judged || alpha.Reason != "its coverage map is unreadable" {
			t.Errorf("first package line = %+v; want alpha with an unreadable map", alpha)
		}
	})

	t.Run("AnInheritedSlotIsHeldByTheCallerSoTheStepTakesNone", func(t *testing.T) {
		step, _ := stepAt(t, pairLocation, environ, time.Minute)
		// The one slot is held, so a step that took its own would wait out the context and list an error.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		step.ctx, step.inherited = ctx, true
		step.cfg.RedundancyMaxTests = 1
		lease, err := step.pool.Acquire(context.Background(), gateslot.Holder{Worktree: pairLocation.WorktreePath(), Site: "an enclosing gate run"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = lease.Release() })
		step.run(false, true)
		report := readReport(t, pairLocation)
		i := slices.IndexFunc(report.Tests, func(test RedundancyTest) bool { return test.Name == "TestAlphaNewOne" })
		if len(report.Errors) != 0 || i < 0 || report.Tests[i].Verdict != RedundancyContained {
			t.Errorf("report = errors %q, tests %+v; want TestAlphaNewOne contained and no error", report.Errors, report.Tests)
		}
	})

	t.Run("ReportsThatJudgeNothing", func(t *testing.T) {
		tests := []struct {
			name     string
			location *lymcwd.Location
			red      bool
			scoper   string
			want     string
		}{
			{name: "ARedGate", location: pairLocation, red: true, want: noteRedGate},
			{name: "NoParentBranch", location: primeLocation, want: noteNoParent},
			{name: "AScoperOtherThanGo", location: pairLocation, scoper: "none", want: "the hub's scoper is none, not go, so the step judged nothing"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				step, _ := stepAt(t, tt.location, environ, time.Minute)
				if tt.scoper != "" {
					step.cfg.Scoper = tt.scoper
				}
				step.run(tt.red, true)
				report := readReport(t, tt.location)
				if report.Note != tt.want || report.RedGate != tt.red || len(report.Tests) != 0 || len(report.Packages) != 0 {
					t.Errorf("report = note %q, red gate %v, %d tests, %d packages; want note %q, red gate %v, nothing judged",
						report.Note, report.RedGate, len(report.Tests), len(report.Packages), tt.want, tt.red)
				}
			})
		}
	})

	t.Run("AReportThatCannotBeWrittenLeavesNoReportAndPrintsNoSummary", func(t *testing.T) {
		// A directory at the report's path fails the write.
		path := redundancyReportPath(pairLocation)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(path) })
		step, stderr := stepAt(t, pairLocation, environ, time.Minute)
		step.run(true, true)
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Errorf("report path = %v, %v; want the directory the test made, since the write failed", info, err)
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr = %q; want no summary line for a report that was not written", stderr.String())
		}
	})
}
