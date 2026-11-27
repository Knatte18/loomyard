// reach_integration_test.go drives the reach report step over a hubforge hub.
// Its task pair holds a small Go module whose functions the pair changes and diffs from the prime's branch.
// The testmap stand-in acts as `go` and as the coverage binaries it builds, so no test builds a real binary.
// It is Tier 1: it builds a real hub and spawns git and the stand-ins.

package gatecli

import (
	"bytes"
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// reachFunctionLine is the part of a ReachFunction the rows compare: the function, its tests and whether it has a caveat.
type reachFunctionLine struct {
	Package, Func string
	Tests         []string
	Caveated      bool
}

// TestGateTestReachStep runs the reach report step in order over one hub.
// Each subtest works on the tree, the maps and the report the last one left.
// It is not parallel: its subtests share that state, and a call row sets environment variables for its child processes.
func TestGateTestReachStep(t *testing.T) {
	tiergate.Require(t, 1)

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	prime, pair := h.PrimeWorktree(), h.PairCodeWorktree(hubforge.TemplatePairSlug)
	const module = "example.com/reached"
	gateYAML := func(reports string) string {
		return "slots: 2\nscoper: go\nredundancy_build_timeout_min: 1\nredundancy_test_timeout_sec: 20\nreach_budget_min: 9\ntiers:\n" +
			"  tier0: {command: 'true', timeout_min: 1, counter: go-json, parallel: 2, env: {LYM_TEST_TIER: \"0\"}}\n" +
			"gates:\n  darn-write: {tiers: [0], scope: all, reports: " + reports + "}\n"
	}
	writeGate := func(t *testing.T, reports string) {
		t.Helper()
		if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(gateYAML(reports)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeGate(t, "[reach]")

	// Each package holds the two one-line functions One and Two and a tier0 test that covers One.
	// nomap's test binary links lib, and tiered holds a tier2 test file beside its tier0 test.
	packages := []string{"alpha", "dirty", "dirtylink", "fresh", "lib", "lib2", "nomap", "stale", "tiered"}
	title := func(pkg string) string { return strings.ToUpper(pkg[:1]) + pkg[1:] }
	seeded := map[string]string{"go.mod": "module " + module + "\n\ngo 1.21\n"}
	for _, pkg := range packages {
		seeded[pkg+"/"+pkg+".go"] = testMapSource(pkg)
		seeded[pkg+"/"+pkg+"_test.go"] = testMapTestFile(pkg, map[string]int{"Test" + title(pkg) + "One": 0})
	}
	seeded["tiered/tiered_slow_test.go"] = testMapTestFile("tiered", map[string]int{"TestTieredSlow": 2})
	plankit.WriteTree(t, prime, seeded)
	gitkit.Git(t, prime, append([]string{"add", "go.mod"}, packages...)...)
	gitkit.Git(t, prime, "commit", "-q", "-m", "seed the reached module")
	primeHead := gitkit.RevParse(t, prime, "HEAD")
	gitkit.Git(t, pair, "merge", "--ff-only", "-q", "main")

	// The pair changes One in every package but alpha and lib, and Two in tiered, and commits.
	edit := func(t *testing.T, root, pkg, from, to string) {
		t.Helper()
		file := filepath.Join(root, pkg, pkg+".go")
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(strings.Replace(string(data), from, to, 1)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, pkg := range []string{"dirty", "dirtylink", "fresh", "nomap", "stale"} {
		edit(t, pair, pkg, "return 1 }", "return 9 }")
	}
	edit(t, pair, "tiered", "return 2 }", "return 8 }")
	gitkit.Git(t, pair, "commit", "-q", "-a", "-m", "change functions of the reached module")
	// The uncommitted files sit in two directories.
	// One is dirty's own; the other is lib2's, which dirtylink's map records as linked.
	plankit.WriteTree(t, pair, map[string]string{"dirty/notes.txt": "dirty\n", "lib2/notes.txt": "dirty\n"})

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
	environ := append(os.Environ(), helperEnv+"="+helperRoleTestMapGo, testMapModuleEnv+"="+module, testMapLinksEnv+"=nomap=lib")
	sharedDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
	anchorDir := testcover.Dir(pairLocation.AnchorPath())

	diskHashes := func(t *testing.T, dir string) map[string]string {
		t.Helper()
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		hashes := map[string]string{}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			hashes[entry.Name()] = testcover.ContentHash(data)
		}
		return hashes
	}
	// mapOf returns a map of pkg at the worktree root whose test of the name covers One, built an hour ago.
	mapOf := func(t *testing.T, root, pkg, test string) testcover.Map {
		t.Helper()
		file := module + "/" + pkg + "/" + pkg + ".go"
		keys, err := testcover.KeyBlocks(filepath.Join(root, pkg), []testcover.ProfileBlock{
			{File: file, StartLine: 3, StartCol: 16, EndLine: 3, EndCol: 28, Statements: 1},
			{File: file, StartLine: 5, StartCol: 16, EndLine: 5, EndCol: 28, Statements: 1},
		})
		if err != nil {
			t.Fatal(err)
		}
		m := testcover.Map{Commit: "c0ffee", BuiltAt: time.Now().Add(-time.Hour), Files: diskHashes(t, filepath.Join(root, pkg)), Blocks: keys}
		if test != "" {
			m.Tests = []testcover.MapTest{{Name: test, FuncHash: "x", Seconds: 0.01, Blocks: []int{0}}}
		}
		return m
	}
	writeMap := func(t *testing.T, dir, pkg string, m testcover.Map) {
		t.Helper()
		if err := testcover.WriteMap(dir, pkg, m); err != nil {
			t.Fatal(err)
		}
	}
	staleMapOf := func(t *testing.T) testcover.Map {
		t.Helper()
		m := mapOf(t, pair, "stale", "TestStaleOne")
		m.Files["stale.go"] = "an earlier stale.go"
		return m
	}
	stepAt := func(t *testing.T, location *lymcwd.Location, env []string, minuteUnit time.Duration) (reachStep, *bytes.Buffer) {
		t.Helper()
		cfg, err := gateslot.LoadConfig(fabricengine.ConfigDirs(location))
		if err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		return reachStep{
			ctx: context.Background(), stderr: &stderr, cfg: cfg.WithDefaultDir(location.AnchorRel), location: location,
			root: location.WorktreePath(), gateKey: "darn-write", environ: env, goBinary: standIn,
			minuteUnit: minuteUnit, secondUnit: 150 * time.Millisecond, grace: time.Second,
		}, &stderr
	}
	readReport := func(t *testing.T, location *lymcwd.Location) ReachReport {
		t.Helper()
		path := reachReportPath(location)
		report, found, err := state.ReadJSON[ReachReport](path, lock.TrackedFileLockPath(path, ".lock"))
		if err != nil || !found {
			t.Fatalf("read %s = found %v, err %v; want a report", path, found, err)
		}
		return report
	}
	// builds returns the packages the stand-in recorded a coverage build of in the file, sorted.
	builds := func(t *testing.T, file string) []string {
		t.Helper()
		data, err := os.ReadFile(file)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			t.Fatal(err)
		}
		var built []string
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			importPath, _, _ := strings.Cut(line, " ")
			built = append(built, strings.TrimPrefix(importPath, module+"/"))
		}
		slices.Sort(built)
		return built
	}
	functionLines := func(report ReachReport) []reachFunctionLine {
		var lines []reachFunctionLine
		for _, fn := range report.Functions {
			line := reachFunctionLine{Package: fn.Package, Func: fn.Func, Caveated: len(fn.Caveats) > 0}
			for _, test := range fn.Tests {
				line.Tests = append(line.Tests, test.Package+"."+test.Test)
			}
			lines = append(lines, line)
		}
		return lines
	}
	buildsEnv := func(t *testing.T) (string, []string) {
		t.Helper()
		file := filepath.Join(t.TempDir(), "builds")
		return file, append(slices.Clone(environ), testMapCoverBuildsEnv+"="+file)
	}

	// The prime changes alpha's One without committing it.
	edit(t, prime, "alpha", "return 1 }", "return 9 }")

	t.Run("APrimeCallInAHubWithNoSharedMapsJudgesNothingAndNamesTheMapRun", func(t *testing.T) {
		step, _ := stepAt(t, primeLocation, environ, time.Minute)
		step.run(false, true)
		report := readReport(t, primeLocation)
		if !report.Prime || report.Base != primeHead || !strings.Contains(report.Note, "lym gate test --map") || len(report.Functions) != 0 {
			t.Errorf("report = prime %v, base %q, note %q, functions %+v; want a prime report on %s judging nothing and naming lym gate test --map",
				report.Prime, report.Base, report.Note, report.Functions, primeHead)
		}
	})

	sharedAlpha := mapOf(t, prime, "alpha", "TestAlphaOne")
	writeMap(t, sharedDir, "alpha", sharedAlpha)
	sharedBytes, err := os.ReadFile(testcover.MapPath(sharedDir, "alpha"))
	if err != nil {
		t.Fatal(err)
	}
	// fresh's own map matches the worktree; stale's does not; dirtylink's does not and records lib2 as linked.
	writeMap(t, anchorDir, "fresh", mapOf(t, pair, "fresh", "TestFreshOne"))
	writeMap(t, anchorDir, "stale", staleMapOf(t))
	dirtyLinked := mapOf(t, pair, "dirtylink", "")
	dirtyLinked.Files["dirtylink.go"] = "an earlier dirtylink.go"
	dirtyLinked.Linked = map[string]map[string]string{module + "/lib2": diskHashes(t, filepath.Join(pair, "lib2"))}
	writeMap(t, anchorDir, "dirtylink", dirtyLinked)

	t.Run("RefreshesTheStaleAndMissingMapsOfTheChangedPackagesAndAnswersFromThem", func(t *testing.T) {
		builtFile, env := buildsEnv(t)
		step, stderr := stepAt(t, pairLocation, env, time.Minute)
		step.run(false, true)
		report := readReport(t, pairLocation)

		// nomap and tiered have no map and stale's is stale, so each is built.
		// fresh's matches, and dirty's and dirtylink's hold an uncommitted file.
		if got, want := builds(t, builtFile), []string{"nomap", "stale", "tiered"}; !slices.Equal(got, want) {
			t.Errorf("coverage builds = %v; want %v", got, want)
		}
		wantUnrefreshed := []string{
			"dirty: not refreshed, the uncommitted file dirty/notes.txt is its own file; way forward: commit it and run the gate again",
			"dirtylink: not refreshed, the uncommitted file lib2/notes.txt is linked package lib2's file; way forward: commit it and run the gate again",
		}
		if diff := cmp.Diff(wantUnrefreshed, report.Unrefreshed); diff != "" {
			t.Errorf("report unrefreshed mismatch (-want +got):\n%s", diff)
		}
		wantFunctions := []reachFunctionLine{
			{Package: "dirty", Func: "One"},
			{Package: "dirtylink", Func: "One"},
			{Package: "fresh", Func: "One", Tests: []string{"fresh.TestFreshOne"}},
			{Package: "nomap", Func: "One", Tests: []string{"nomap.TestNomapOne"}},
			{Package: "stale", Func: "One", Tests: []string{"stale.TestStaleOne"}},
			{Package: "tiered", Func: "Two", Caveated: true},
		}
		if diff := cmp.Diff(wantFunctions, functionLines(report), cmpopts.EquateEmpty()); diff != "" {
			t.Errorf("report functions mismatch (-want +got):\n%s", diff)
		}
		if report.Prime || report.Note != "" || len(report.Errors) != 0 || report.GateKey != "darn-write" || report.ModuleDir != "." {
			t.Errorf("report = prime %v, note %q, errors %q, gate key %q, module dir %q; want a task report with no note and no error",
				report.Prime, report.Note, report.Errors, report.GateKey, report.ModuleDir)
		}
		for _, pkg := range []string{"nomap", "stale", "tiered"} {
			m, found, err := testcover.ReadMap(anchorDir, pkg)
			if err != nil || !found || m.Commit != gitkit.RevParse(t, pair, "HEAD") || !maps.Equal(m.Files, diskHashes(t, filepath.Join(pair, pkg))) {
				t.Errorf("anchor map of %s = found %v, err %v, commit %q, files %v; want one built at the worktree's HEAD over its files", pkg, found, err, m.Commit, m.Files)
			}
		}
		if m := readTestMap(t, anchorDir, "nomap"); len(m.Linked) != 1 || m.Linked[module+"/lib"] == nil {
			t.Errorf("anchor map of nomap records linked %v; want lib, listed by the stand-in's dependency list", m.Linked)
		}
		if !strings.Contains(stderr.String(), "gate test: reach: ") || !strings.Contains(stderr.String(), "dirty: not refreshed") {
			t.Errorf("stderr = %q; want the note's lines", stderr.String())
		}
	})

	t.Run("APassingCallListingReachWritesTheReportAndKeepsItsExitCode", func(t *testing.T) {
		// The call's child processes read the stand-in's role and module from the environment.
		// They read no outer gate's slot or strand.
		t.Setenv(helperEnv, helperRoleTestMapGo)
		t.Setenv(testMapModuleEnv, module)
		t.Setenv(testMapLinksEnv, "nomap=lib")
		t.Setenv(gateslot.InheritEnv, "")
		t.Setenv(enclosingGateEnv, "")
		t.Setenv(agentname.StrandNameEnv, "")
		call := func(t *testing.T) (int, string) {
			t.Helper()
			var out, stderr bytes.Buffer
			code := runTest(lymcwd.WithCwd(context.Background(), pair), &out, testRequest{
				goBinary: standIn, samplePeriod: defaultSamplePeriod, preTierCeiling: defaultPreTierCeiling, timeoutUnit: time.Minute,
				secondUnit: 150 * time.Millisecond, rerunDelay: defaultRerunDelay, pipeWaitDelay: defaultPipeWaitDelay, stderr: &stderr, gate: "darn-write",
			})
			return code, stderr.String()
		}
		writeGate(t, "[]")
		if err := os.Remove(reachReportPath(pairLocation)); err != nil {
			t.Fatal(err)
		}
		without, stderrWithout := call(t)
		if _, err := os.Stat(reachReportPath(pairLocation)); err == nil || strings.Contains(stderrWithout, "gate test: reach") {
			t.Errorf("a call listing no report wrote a reach report (stat error %v) or printed its note", err)
		}
		writeGate(t, "[reach]")
		with, stderrWith := call(t)
		if with != without || with != 0 {
			t.Errorf("exit codes with the step %d and without %d; want both 0", with, without)
		}
		if report := readReport(t, pairLocation); report.GateKey != "darn-write" || len(report.Functions) == 0 || !strings.Contains(stderrWith, "gate test: reach: ") {
			t.Errorf("report = %+v, stderr %q; want a report of the call's gate key and its note on stderr", report, stderrWith)
		}
	})

	t.Run("ReportsThatJudgeNothingMakeNoLookup", func(t *testing.T) {
		tests := []struct {
			name   string
			red    bool
			inHub  bool
			scoper string
			want   string
		}{
			{name: "ARedTier", red: true, inHub: true, want: noteRedGate},
			{name: "ACallOutsideAHub", want: noteNoHub},
			{name: "AScoperOtherThanGo", inHub: true, scoper: "none", want: "the hub's scoper is none, not go, so the step judged nothing"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				builtFile, env := buildsEnv(t)
				step, _ := stepAt(t, pairLocation, env, time.Minute)
				if tt.scoper != "" {
					step.cfg.Scoper = tt.scoper
				}
				step.run(tt.red, tt.inHub)
				report := readReport(t, pairLocation)
				if report.Note != tt.want || report.RedGate != tt.red || len(report.Functions) != 0 || len(report.Maps) != 0 || len(builds(t, builtFile)) != 0 {
					t.Errorf("report = note %q, red gate %v, %d functions, %d maps; want note %q, red gate %v and no lookup",
						report.Note, report.RedGate, len(report.Functions), len(report.Maps), tt.want, tt.red)
				}
			})
		}
	})

	t.Run("APrimeCallRefreshesNothingAndAnswersFromTheSharedMaps", func(t *testing.T) {
		builtFile, env := buildsEnv(t)
		step, _ := stepAt(t, primeLocation, env, time.Minute)
		step.run(false, true)
		report := readReport(t, primeLocation)
		wantUnrefreshed := []string{"alpha: not refreshed, " + reasonPrimeKeepsNoMaps}
		if diff := cmp.Diff(wantUnrefreshed, report.Unrefreshed); diff != "" {
			t.Errorf("report unrefreshed mismatch (-want +got):\n%s", diff)
		}
		wantFunctions := []reachFunctionLine{{Package: "alpha", Func: "One", Tests: []string{"alpha.TestAlphaOne"}}}
		if diff := cmp.Diff(wantFunctions, functionLines(report)); diff != "" {
			t.Errorf("report functions mismatch (-want +got):\n%s", diff)
		}
		if !report.Prime || report.Base != primeHead || len(builds(t, builtFile)) != 0 {
			t.Errorf("report = prime %v, base %q, builds %v; want a prime report on %s with no build", report.Prime, report.Base, builds(t, builtFile), primeHead)
		}
		if _, err := os.Stat(testcover.Dir(primeLocation.AnchorPath())); err == nil {
			t.Error("the prime call wrote worktree maps; want none")
		}
	})

	t.Run("ASpentBudgetRefreshesNothingAndSaysSo", func(t *testing.T) {
		writeMap(t, anchorDir, "stale", staleMapOf(t))
		builtFile, env := buildsEnv(t)
		step, _ := stepAt(t, pairLocation, env, time.Nanosecond)
		step.run(false, true)
		report := readReport(t, pairLocation)
		want := "stale: not refreshed, " + mapBudgetReason
		if !slices.Contains(report.Unrefreshed, want) || len(builds(t, builtFile)) != 0 {
			t.Errorf("report unrefreshed = %q, builds %v; want the line %q and no build", report.Unrefreshed, builds(t, builtFile), want)
		}
		if m := readTestMap(t, anchorDir, "stale"); m.Files["stale.go"] != "an earlier stale.go" {
			t.Errorf("stale's anchor map files = %v; want the map the budget left unrefreshed", m.Files)
		}
	})

	t.Run("NoRefreshWroteTheSharedMaps", func(t *testing.T) {
		got, err := os.ReadFile(testcover.MapPath(sharedDir, "alpha"))
		if err != nil || !bytes.Equal(got, sharedBytes) {
			t.Errorf("shared map of alpha = %d bytes, err %v; want the %d bytes written before the steps ran", len(got), err, len(sharedBytes))
		}
		for _, pkg := range []string{"fresh", "nomap", "stale", "tiered"} {
			if _, found, err := testcover.ReadMap(sharedDir, pkg); err != nil || found {
				t.Errorf("ReadMap(%s) in the shared maps = found %v, err %v; want no map", pkg, found, err)
			}
		}
	})

	t.Run("AReportHoldsForHeadUntilACommitChangesWhatItNamed", func(t *testing.T) {
		_, env := buildsEnv(t)
		step, _ := stepAt(t, pairLocation, env, time.Minute)
		step.run(false, true)
		report := readReport(t, pairLocation)
		head := gitkit.RevParse(t, pair, "HEAD")
		appendTo := func(t *testing.T, rel, text string) {
			t.Helper()
			file := filepath.Join(pair, filepath.FromSlash(rel))
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(file, append(data, text...), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		// Each change is committed on top of the call's HEAD and dropped again once its row ran.
		// lib is no package the report names, so its added function changes the set of changed functions alone.
		tests := []struct {
			name   string
			change func(t *testing.T)
			want   bool
		}{
			{name: "TheCommitTheCallRanOn", want: true},
			{name: "ACommitChangingATestFileOfANamedPackage", change: func(t *testing.T) { appendTo(t, "fresh/fresh_test.go", "\n// changed\n") }},
			{name: "ACommitChangingAChangedFunctionsText", change: func(t *testing.T) { edit(t, pair, "fresh", "return 9 }", "return 7 }") }},
			{name: "ACommitAddingAChangedFunction", change: func(t *testing.T) { appendTo(t, "lib/lib.go", "\nfunc Three() int { return 3 }\n") }},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if tt.change != nil {
					tt.change(t)
					gitkit.Git(t, pair, "commit", "-q", "-a", "-m", tt.name)
					t.Cleanup(func() { gitkit.Git(t, pair, "reset", "-q", "--hard", head) })
				}
				got, err := ReachReportMatches(pairLocation, report, gitkit.RevParse(t, pair, "HEAD"))
				if err != nil || got != tt.want {
					t.Errorf("ReachReportMatches() = %v, %v; want %v, nil", got, err, tt.want)
				}
			})
		}
		primeReport := readReport(t, primeLocation)
		if got, err := ReachReportMatches(primeLocation, primeReport, primeHead); err != nil || got {
			t.Errorf("ReachReportMatches(prime report) = %v, %v; want false, nil", got, err)
		}
	})

	t.Run("ReachPendingOfAnswersTheUnreachedFunctionsOfAReportThatHoldsForHead", func(t *testing.T) {
		_, env := buildsEnv(t)
		step, _ := stepAt(t, pairLocation, env, time.Minute)
		step.run(false, true)
		report := readReport(t, pairLocation)
		reportFile := reachReportPath(pairLocation)
		original, err := os.ReadFile(reportFile)
		if err != nil {
			t.Fatal(err)
		}
		restoreReport := func(t *testing.T) {
			t.Helper()
			if err := os.WriteFile(reportFile, original, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		head := gitkit.RevParse(t, pair, "HEAD")
		hashOf := map[string]string{}
		for _, fn := range report.Functions {
			hashOf[fn.Package+"."+fn.Func] = fn.Hash
		}
		wantFunctions := map[string]string{
			"dirty/dirty.go#One":         hashOf["dirty.One"],
			"dirtylink/dirtylink.go#One": hashOf["dirtylink.One"],
			"tiered/tiered.go#Two":       hashOf["tiered.Two"],
		}

		tests := []struct {
			name    string
			key     string
			arrange func(t *testing.T)
			want    bool
		}{
			{name: "TheCallsOwnKey", key: "darn-write", want: true},
			{name: "AnotherKey", key: "code-burler-fix"},
			{name: "ACommitAfterTheCall", key: "darn-write", arrange: func(t *testing.T) {
				edit(t, pair, "fresh", "return 9 }", "return 7 }")
				gitkit.Git(t, pair, "commit", "-q", "-a", "-m", "change a function after the call")
				t.Cleanup(func() { gitkit.Git(t, pair, "reset", "-q", "--hard", head) })
			}},
			{name: "AReportThatJudgedNothing", key: "darn-write", arrange: func(t *testing.T) {
				judgedNothing := report
				judgedNothing.Note = "the step judged nothing"
				if err := state.WriteJSON(reportFile, lock.TrackedFileLockPath(reportFile, ".lock"), judgedNothing); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { restoreReport(t) })
			}},
			{name: "AReportThatDoesNotParse", key: "darn-write", arrange: func(t *testing.T) {
				if err := os.WriteFile(reportFile, []byte("{not json"), 0o644); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { restoreReport(t) })
			}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if tt.arrange != nil {
					tt.arrange(t)
				}
				got, found, err := ReachPendingOf(pairLocation)(tt.key)
				if err != nil || found != tt.want {
					t.Fatalf("ReachPendingOf()(%q) = found %v, err %v; want found %v and no error", tt.key, found, err, tt.want)
				}
				if !tt.want {
					return
				}
				if diff := cmp.Diff(wantFunctions, got.Functions); diff != "" {
					t.Errorf("ReachPendingOf() functions mismatch (-want +got):\n%s", diff)
				}
				if !strings.Contains(got.Note, "gate test: reach: no mapped test reaches dirty.One") {
					t.Errorf("ReachPendingOf() note = %q; want the report's note lines", got.Note)
				}
			})
		}
	})

	t.Run("ATaskWorktreeWithNoParentBranchJudgesNothing", func(t *testing.T) {
		if err := os.Remove(fabricengine.OriginRecordPath(pairLocation)); err != nil {
			t.Fatal(err)
		}
		builtFile, env := buildsEnv(t)
		step, _ := stepAt(t, pairLocation, env, time.Minute)
		step.run(false, true)
		report := readReport(t, pairLocation)
		if report.Note != noteNoParent || report.Prime || len(report.Functions) != 0 || len(builds(t, builtFile)) != 0 {
			t.Errorf("report = note %q, prime %v, %d functions; want note %q and no lookup", report.Note, report.Prime, len(report.Functions), noteNoParent)
		}
	})
}
