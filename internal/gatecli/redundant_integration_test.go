// redundant_integration_test.go drives `lym gate redundant` over a hubforge hub whose prime holds a small Go module.
// It is Tier 1: it builds a real hub and spawns git.

package gatecli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestGateRedundant judges a written coverage map and refuses where no map can be judged.
func TestGateRedundant(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	gate := "slots: 1\ntiers:\n  tier0: {command: x, timeout_min: 1, counter: go-json, env: {LYM_TEST_TIER: \"0\"}}\n"
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{hubforge.TemplatePairSlug}})
	prime := seedTestMapModule(t, h, "example.com/redundant", gate, map[string]map[string]int{
		"cov": {"TestCovSub": 0, "TestCovSuper": 0, "TestCovFails": 0}, "nomap": {"TestNoMap": 0}, "broken": {"TestBroken": 0},
		"badindex": {"TestBadIndex": 0}, "badkeep": {"TestBadKeep": 0},
	})
	plankit.WriteTree(t, prime, map[string]string{
		"cov/kept_test.go":     "package cov\n\nimport \"testing\"\n\n//testtiming:keep pins the message\nfunc TestCovKept(t *testing.T) {}\n",
		"badkeep/keep_test.go": "package badkeep\n\nimport \"testing\"\n\n//testtiming:keep\nfunc TestBadKeepMarked(t *testing.T) {}\n",
	})
	mapsDir := testcover.Dir(fabricengine.BoltDir(h.Location.HubPath))
	key := func(ordinal int) testcover.BlockKey {
		return testcover.BlockKey{File: "cov.go", Func: "One", Hash: "h", Ordinal: ordinal}
	}
	builtAt := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	if err := testcover.WriteMap(mapsDir, "cov", testcover.Map{
		Commit: "c0ffee", BuiltAt: builtAt, Blocks: []testcover.BlockKey{key(0), key(1)},
		Failure: &testcover.MapFailure{Commit: "deadbeef", Error: "later build broke"},
		Tests: []testcover.MapTest{
			{Name: "TestCovFails", Outcome: testcover.OutcomeFailed},
			{Name: "TestCovKept", Seconds: 3, Blocks: []int{0}},
			{Name: "TestCovSub", Tier: 1, Seconds: 2, Blocks: []int{0}},
			{Name: "TestCovSuper", Seconds: 1, Blocks: []int{0, 1}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	// Each map is sound except for the one fault its package's refusal row names.
	for pkg, index := range map[string]int{"badindex": 5, "badkeep": 0} {
		if err := testcover.WriteMap(mapsDir, pkg, testcover.Map{
			Commit: "c0ffee", BuiltAt: builtAt, Blocks: []testcover.BlockKey{key(0)},
			Tests: []testcover.MapTest{{Name: "Test" + pkg, Seconds: 1, Blocks: []int{index}}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := testcover.WriteMap(mapsDir, "broken", testcover.Map{Commit: "c0ffee", BuildFailure: "boom"}); err != nil {
		t.Fatal(err)
	}

	t.Run("ListsTheContainedTestsAndSetsApartTheKeptAndUnjudgedOnes", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		code := RunCLIIn(prime, &out, []string{"redundant", "./cov"})
		type answer struct {
			OK         bool                  `json:"ok"`
			Package    string                `json:"package"`
			MapCommit  string                `json:"map_commit"`
			MapBuiltAt string                `json:"map_built_at"`
			Tests      int                   `json:"tests"`
			Failure    *testcover.MapFailure `json:"map_build_failure"`
			Candidates []redundantCandidate  `json:"candidates"`
			Kept       []redundantSkip       `json:"kept"`
			NotJudged  []redundantSkip       `json:"not_judged"`
		}
		var got answer
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || code != 0 {
			t.Fatalf("redundant ./cov = %d, %s; want exit 0 and a JSON envelope (%v)", code, out.Bytes(), err)
		}
		want := answer{
			OK: true, Package: "cov", MapCommit: "c0ffee", MapBuiltAt: builtAt.Format(time.RFC3339), Tests: 4,
			Failure:    &testcover.MapFailure{Commit: "deadbeef", Error: "later build broke"},
			Candidates: []redundantCandidate{{Name: "TestCovSub", Tier: 1, Seconds: 2, Removable: true, Covering: []string{"TestCovSuper"}}},
			Kept:       []redundantSkip{{Name: "TestCovKept", Reason: "pins the message"}},
			NotJudged:  []redundantSkip{{Name: "TestCovFails", Reason: string(testcover.OutcomeFailed)}},
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
	}{
		{name: "OutsideEveryHub", cwd: t.TempDir(), pkg: "cov", wantError: "is in no hub"},
		{name: "APackageWithNoMap", cwd: prime, pkg: "nomap", wantError: "nomap has no coverage map"},
		{name: "AMapHoldingOnlyAFailedBuild", cwd: prime, pkg: "broken", wantError: "records only a failed build: boom"},
		{name: "AMapNamingABlockOutsideItsTable", cwd: prime, pkg: "badindex", wantError: "is unreadable"},
		{name: "AFailedKeepScan", cwd: prime, pkg: "badkeep", wantError: "the keep scan of"},
	}
	for _, tt := range refusals {
		t.Run("Refuses"+tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			code := RunCLIIn(tt.cwd, &out, []string{"redundant", tt.pkg})
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
		})
	}
}
