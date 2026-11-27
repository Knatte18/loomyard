// redundant_integration_test.go drives `lym gate redundant` over a hubforge hub whose prime holds a small Go module.
// It is Tier 1: it builds a real hub, spawns git, and builds and runs the module's tests with the real go command.

package gatecli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// redundantModuleFiles is the fixture module of TestGateRedundant.
// Its package cov holds tests whose coverage overlaps while their checks differ:
// TestClassifyChecked checks what TestAll only runs.
// TestAll and TestDoubleChecked check the same result.
// TestDoubleUnchecked checks nothing.
// TestSourceScan checks what the module's scan kit reads, and TestGuarded carries a //lym:guard marker.
// TestTallyChecked and TestTallyCheckedAgain check the same loop, whose dropped decrement makes it endless.
// TestHoardChecked and TestHoardCheckedAgain check a loop that keeps a megabyte a pass.
// Its negated condition makes it endless.
// Hoard panics at hoardStop megabytes, so a memory cap that does not hold fails the test and never the machine.
// TestAll and TestDoubleChecked also call Timeout, whose constant does not compile behind the mutant helper.
// The module stands in its own tiergate, at the import path a coverage map reads tiers from, so it builds alone.
var redundantModuleFiles = map[string]string{
	"go.mod":                                "module github.com/Knatte18/loomyard\n\ngo 1.21\n",
	"internal/testkit/tiergate/tiergate.go": "package tiergate\n\nimport \"testing\"\n\n// Require stands in for the tier gate: every test runs.\nfunc Require(t testing.TB, tier int) {}\n",
	"internal/testkit/scankit/scankit.go":   "package scankit\n\n// Files stands in for the module's source scan.\nfunc Files() int { return 0 }\n",
	"cov/cov.go": `package cov

import "time"

func Classify(n int) string {
	if n < 0 {
		return "negative"
	}
	return "non-negative"
}

func Double(n int) int {
	return n * 2
}

func Timeout() time.Duration {
	return 5 * time.Second
}

func Tally(n int) int {
	steps := 0
	for n > 0 {
		n -= 1
		steps += 1
	}
	return steps
}

const hoardStop = 384

func Hoard(n int) int {
	var kept [][]byte
	for i := 1; i <= n; i++ {
		if len(kept) == hoardStop {
			panic("Hoard kept hoardStop megabytes: no memory cap ended the run")
		}
		block := make([]byte, 1<<20)
		for at := range block {
			block[at] = 1
		}
		kept = append(kept, block)
	}
	return len(kept)
}
`,
	"cov/cov_test.go": `package cov

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestAll(t *testing.T) {
	tiergate.Require(t, 0)
	Classify(-1)
	Classify(1)
	Timeout()
	if Double(2) != 4 {
		t.Fatal("Double(2) != 4")
	}
}

func TestClassifyChecked(t *testing.T) {
	tiergate.Require(t, 0)
	if Classify(-1) != "negative" || Classify(1) != "non-negative" {
		t.Fatal("Classify names the wrong sign")
	}
}

func TestDoubleChecked(t *testing.T) {
	tiergate.Require(t, 0)
	Timeout()
	if Double(2) != 4 {
		t.Fatal("Double(2) != 4")
	}
}

func TestDoubleUnchecked(t *testing.T) {
	tiergate.Require(t, 0)
	Double(3)
}

//testtiming:keep pins the message
func TestKept(t *testing.T) {
	tiergate.Require(t, 0)
	Double(5)
}

func TestFails(t *testing.T) {
	tiergate.Require(t, 0)
	t.Fatal("fails")
}

func TestSourceScan(t *testing.T) {
	tiergate.Require(t, 0)
	Double(1)
	Tally(3)
	if scankit.Files() != 0 {
		t.Fatal("the scan found a file")
	}
}

//lym:guard
func TestGuarded(t *testing.T) {
	tiergate.Require(t, 0)
	Double(7)
}

func TestTallyChecked(t *testing.T) {
	tiergate.Require(t, 0)
	if Tally(3) != 3 {
		t.Fatal("Tally(3) != 3")
	}
}

func TestTallyCheckedAgain(t *testing.T) {
	tiergate.Require(t, 0)
	if Tally(3) != 3 {
		t.Fatal("Tally(3) != 3")
	}
}

func TestHoardChecked(t *testing.T) {
	tiergate.Require(t, 0)
	if Hoard(0) != 0 {
		t.Fatal("Hoard(0) != 0")
	}
}

func TestHoardCheckedAgain(t *testing.T) {
	tiergate.Require(t, 0)
	if Hoard(0) != 0 {
		t.Fatal("Hoard(0) != 0")
	}
}
`,
	"lacking/lacking.go":   "package lacking\n\nfunc One() int { return 1 }\n",
	"stale/stale.go":       "package stale\n\nfunc One() int { return 1 }\n",
	"badkeep/keep_test.go": "package badkeep\n\nimport \"testing\"\n\n//testtiming:keep\nfunc TestBadKeepMarked(t *testing.T) {}\n",
}

// redundantTestSecond is the length of one `redundancy_test_timeout_sec` second in TestGateRedundant.
const redundantTestSecond = 2 * time.Second

// TestGateRedundant confirms a built map's candidates by mutation and refuses where no map can be judged.
func TestGateRedundant(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	// A second lasts redundantTestSecond here.
	// The mutant timeout floor is mutantTimeoutFloor × redundantTestSecond.
	// It is far above a healthy fixture test's whole process run.
	// The test bound, redundancy_test_timeout_sec × redundantTestSecond, lies above that floor.
	// So the mutant making a loop endless ends at the floor and not at the test bound.
	// The memory cap lies far above a healthy fixture test's memory and far below hoardStop megabytes.
	// So the mutant making a loop keep memory for ever ends at the cap, long before the floor.
	gate := "slots: 1\nredundancy_test_timeout_sec: 20\nredundancy_test_memory_mb: 128\ntiers:\n  tier0: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"0\"}}\n"
	if err := os.WriteFile(configengine.ConfigFile(h.BoardDir(), "gate"), []byte(gate), 0o644); err != nil {
		t.Fatal(err)
	}
	prime := h.PrimeWorktree()
	plankit.WriteTree(t, prime, redundantModuleFiles)
	gitkit.Git(t, prime, "add", "go.mod", "internal", "cov", "stale", "lacking", "badkeep")
	gitkit.Git(t, prime, "commit", "-q", "-m", "seed the fixture module")

	mapsDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
	key := testcover.BlockKey{File: "stale.go", Func: "One", Hash: "h"}
	builtAt := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	fakeMaps := map[string]testcover.Map{
		"stale": {
			Commit: "c0ffee", BuiltAt: builtAt, Files: map[string]string{"stale.go": "another file's hash"}, Blocks: []testcover.BlockKey{key},
			Tests: []testcover.MapTest{{Name: "TestSub", Seconds: 1, Blocks: []int{0}}, {Name: "TestSuper", Seconds: 1, Blocks: []int{0}}},
		},
		// The map names gone.go, which the worktree lacks, and not lacking.go, which it holds.
		"lacking": {
			Commit: "c0ffee", BuiltAt: builtAt, Files: map[string]string{"gone.go": "h"},
			Blocks: []testcover.BlockKey{{File: "lacking.go", Func: "One", Hash: "h"}},
			Tests:  []testcover.MapTest{{Name: "TestSub", Seconds: 1, Blocks: []int{0}}, {Name: "TestSuper", Seconds: 1, Blocks: []int{0}}},
		},
		"badindex": {Commit: "c0ffee", BuiltAt: builtAt, Blocks: []testcover.BlockKey{key}, Tests: []testcover.MapTest{{Name: "TestBadIndex", Blocks: []int{5}}}},
		"badkeep":  {Commit: "c0ffee", BuiltAt: builtAt, Blocks: []testcover.BlockKey{key}, Tests: []testcover.MapTest{{Name: "TestBadKeep", Blocks: []int{0}}}},
		"broken":   {Commit: "c0ffee", BuildFailure: "boom"},
	}
	for pkg, m := range fakeMaps {
		if err := testcover.WriteMap(mapsDir, pkg, m); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("ListsTheCandidatesMutationConfirmsAndSetsApartTheRefutedKeptAndUnjudgedOnes", func(t *testing.T) {
		t.Parallel()
		var built bytes.Buffer
		// cov's TestFails fails, so the map run writes the map and exits 1.
		if code := clihelp.ExecuteIn(Command(), prime, &built, []string{"test", "--wait", "--map", "cov"}); code != 1 {
			t.Fatalf("test --wait --map cov = %d, %s; want exit 1", code, built.Bytes())
		}
		m, found, err := testcover.ReadMap(mapsDir, "cov")
		if err != nil || !found {
			t.Fatalf("ReadMap(cov) = found %v, %v; want the map the map run built", found, err)
		}
		m.Failure = &testcover.MapFailure{Commit: "deadbeef", Error: "later build broke"}
		if err := testcover.WriteMap(mapsDir, "cov", m); err != nil {
			t.Fatal(err)
		}

		var out bytes.Buffer
		code := runRedundant(lymcwd.WithCwd(context.Background(), prime), &out, redundantRequest{
			pkg: "./cov", goBinary: defaultGoBinary, environ: os.Environ(),
			secondUnit: redundantTestSecond, minuteUnit: time.Minute, grace: defaultPipeWaitDelay,
			// Two runs go side by side whatever memory the machine has left, so a covering run that fails runs again alone.
			cappedParallel: func(int, int64) int { return 2 },
		})
		type answer struct {
			OK         bool                  `json:"ok"`
			Package    string                `json:"package"`
			Tests      int                   `json:"tests"`
			Failure    *testcover.MapFailure `json:"map_build_failure"`
			Candidates []redundantCandidate  `json:"candidates"`
			Refuted    []redundantRefuted    `json:"refuted"`
			Kept       []redundantSkip       `json:"kept"`
			NotJudged  []redundantSkip       `json:"not_judged"`
			Mutation   mutationStats         `json:"mutation"`
		}
		var got answer
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || code != 0 {
			t.Fatalf("redundant ./cov = %d, %s; want exit 0 and a JSON envelope (%v)", code, out.Bytes(), err)
		}
		removable := 0
		for i := range got.Candidates {
			if got.Candidates[i].Removable {
				removable++
			}
			// The seconds, and with them the greedy passes' order, vary by run.
			got.Candidates[i].Seconds, got.Candidates[i].Removable = 0, false
		}
		for i := range got.Refuted {
			got.Refuted[i].Seconds = 0
		}
		// Each pair of candidates kills the same mutants, so the greedy pass never drops both of a pair.
		if removable > 3 {
			t.Errorf("redundant ./cov lists %d removable candidates; want at most 3, one of each pair", removable)
		}
		// Timeout's constant is the one mutant that does not compile.
		if got.Mutation.Mutants != 14 || got.Mutation.Unbuildable != 1 {
			t.Errorf("redundant ./cov mutation = %+v; want 14 buildable mutants and 1 unbuildable", got.Mutation)
		}
		// Each involved test runs once with no mutant on, and each contained test once with each mutant of its blocks.
		// Each covering test then runs with each mutant a contained test killed and it covers, unless it left that mutant alive.
		// Every one of those covering runs kills, and one that kills beside another run runs once more alone.
		// The two the memory cap ended do not.
		const unmutatedRuns, containedRuns, coveringRuns, cappedCoveringRuns = 8, 29, 18, 2
		if want := unmutatedRuns + containedRuns + 2*coveringRuns - cappedCoveringRuns; got.Mutation.Runs != want {
			t.Errorf("redundant ./cov started %d runs; want %d, with no second run of one the memory cap ended", got.Mutation.Runs, want)
		}
		hoardLoop := mutantStop{File: "cov.go", Line: 33, Operator: testcover.OperatorNegateCondition}
		tallyStep := mutantStop{File: "cov.go", Line: 23, Operator: testcover.OperatorDropStatement}
		stops := func(site mutantStop, tests ...string) []mutantStop {
			var listed []mutantStop
			for _, test := range tests {
				site.Test = test
				listed = append(listed, site)
			}
			return listed
		}
		if diff := cmp.Diff(stops(hoardLoop, "TestHoardChecked", "TestHoardCheckedAgain"), got.Mutation.MemoryCapped); diff != "" {
			t.Errorf("redundant ./cov memory_capped mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(stops(tallyStep, "TestTallyChecked", "TestTallyCheckedAgain"), got.Mutation.TimedOut); diff != "" {
			t.Errorf("redundant ./cov timed_out mismatch (-want +got):\n%s", diff)
		}
		got.Mutation = mutationStats{}
		// The covering lists are the pick among every judged test but the source checks.
		// TestSourceScan sorts before the Tally tests and overlaps them fully, yet neither it nor TestGuarded is named.
		want := answer{
			OK: true, Package: "cov", Tests: 12,
			Failure: &testcover.MapFailure{Commit: "deadbeef", Error: "later build broke"},
			Candidates: []redundantCandidate{
				{Name: "TestAll", Covering: []string{"TestClassifyChecked", "TestDoubleChecked"}, MutantsKilled: 1},
				{Name: "TestDoubleChecked", Covering: []string{"TestAll"}, MutantsKilled: 1},
				{Name: "TestHoardChecked", Covering: []string{"TestHoardCheckedAgain"}, MutantsKilled: 1},
				{Name: "TestHoardCheckedAgain", Covering: []string{"TestHoardChecked"}, MutantsKilled: 1},
				{Name: "TestTallyChecked", Covering: []string{"TestTallyCheckedAgain"}, MutantsKilled: 7},
				{Name: "TestTallyCheckedAgain", Covering: []string{"TestTallyChecked"}, MutantsKilled: 7},
			},
			Refuted: []redundantRefuted{{
				Name: "TestClassifyChecked", Covering: []string{"TestAll"},
				Unique: []redundantMutant{
					{File: "cov.go", Line: 6, Operator: testcover.OperatorNegateCondition},
					{File: "cov.go", Line: 7, Operator: testcover.OperatorChangeConstant},
					{File: "cov.go", Line: 9, Operator: testcover.OperatorChangeConstant},
				},
			}},
			Kept: []redundantSkip{{Name: "TestKept", Reason: "pins the message"}},
			NotJudged: []redundantSkip{
				{Name: "TestFails", Reason: string(testcover.OutcomeFailed)},
				{Name: "TestGuarded", Reason: testcover.ReasonSourceCheck},
				{Name: "TestSourceScan", Reason: testcover.ReasonSourceCheck},
				{Name: "TestDoubleUnchecked", Reason: testcover.ReasonKillsNone},
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("redundant ./cov mismatch (-want +got):\n%s", diff)
		}
	})

	refusals := []struct {
		name      string
		cwd       string
		pkg       string
		wantError string
		// wantWayForward are the parts of the way forward the refusal names.
		wantWayForward []string
	}{
		{name: "OutsideEveryHub", cwd: t.TempDir(), pkg: "cov", wantError: "is in no hub", wantWayForward: []string{"run lym gate redundant from a worktree of the hub"}},
		{name: "APackageWithNoMap", cwd: prime, pkg: "nomap", wantError: "nomap has no coverage map", wantWayForward: []string{"lym gate test --map nomap"}},
		{name: "AMapHoldingOnlyAFailedBuild", cwd: prime, pkg: "broken", wantError: "records only a failed build: boom", wantWayForward: []string{"lym gate test --map broken"}},
		{name: "AMapNamingABlockOutsideItsTable", cwd: prime, pkg: "badindex", wantError: "is unreadable", wantWayForward: []string{"lym gate test --map badindex"}},
		{name: "AFailedKeepScan", cwd: prime, pkg: "badkeep", wantError: "the keep scan of", wantWayForward: []string{"fix the //testtiming:keep line it names"}},
		{name: "AMapBuiltFromOtherFiles", cwd: prime, pkg: "stale", wantError: "was built from other Go files than this worktree holds: stale.go", wantWayForward: []string{"commit or revert", "lym gate test --map stale"}},
		{name: "AMapWithAFileOnOneSideOnly", cwd: prime, pkg: "lacking", wantError: "was built from other Go files than this worktree holds: gone.go, lacking.go", wantWayForward: []string{"commit or revert", "lym gate test --map lacking"}},
	}
	for _, tt := range refusals {
		t.Run("Refuses"+tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			code := clihelp.ExecuteIn(Command(), tt.cwd, &out, []string{"redundant", tt.pkg})
			var envelope struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("output %q is not a JSON envelope: %v", out.String(), err)
			}
			if code != 1 || envelope.OK || !strings.Contains(envelope.Error, tt.wantError) || !strings.Contains(envelope.Error, "way forward") {
				t.Errorf("redundant %s = %d, %+v; want exit 1, ok false and an error holding %q and a way forward", tt.pkg, code, envelope, tt.wantError)
			}
			for _, want := range tt.wantWayForward {
				if _, wayForward, _ := strings.Cut(envelope.Error, "way forward: "); !strings.Contains(wayForward, want) {
					t.Errorf("redundant %s error = %q; want its way forward to hold %q", tt.pkg, envelope.Error, want)
				}
			}
		})
	}
}
