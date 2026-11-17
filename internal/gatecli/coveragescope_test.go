// coveragescope_test.go covers how a coverage gate key marks a call and how its scope narrows a diff share.
// It needs no process and no hub: an importing package is a directory of test files under a temporary root.

package gatecli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/testcover"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestWithGateScope(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	cfg := gateslot.Config{Gates: map[string]gateslot.GateKey{
		"darn-write": {Tiers: []int{0}, Scope: gateslot.ScopeCoverage},
		"pr-review":  {Tiers: []int{0}, Scope: "diff"},
	}}
	tests := []struct {
		name    string
		request testRequest
		want    testRequest
	}{
		{
			name:    "CoverageKeyIsTheComputedFormMarkedForCoverage",
			request: testRequest{gate: "darn-write", cards: []string{"1"}},
			want:    testRequest{gate: "darn-write", coverage: true},
		},
		{
			name:    "DiffKeyIsNotMarked",
			request: testRequest{gate: "pr-review", cards: []string{"1"}},
			want:    testRequest{gate: "pr-review"},
		},
		{
			name:    "ExplicitTargetsLeaveACoverageKeyUnnarrowed",
			request: testRequest{gate: "darn-write", targets: []string{"./a"}},
			want:    testRequest{gate: "darn-write", targets: []string{"./a"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := withGateScope(cfg, tt.request); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("withGateScope(%+v) = %+v; want %+v", tt.request, got, tt.want)
			}
		})
	}
}

func TestCoverageTier(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	withTier0 := func(change func(*gateslot.Tier)) gateslot.Config {
		cfg := planConfig()
		t0 := cfg.Tiers["tier0"]
		change(&t0)
		cfg.Tiers["tier0"] = t0
		return cfg
	}
	withoutTier0 := func() gateslot.Config {
		cfg := planConfig()
		delete(cfg.Tiers, "tier0")
		return cfg
	}
	tests := []struct {
		name    string
		cfg     gateslot.Config
		request testRequest
		want    bool
	}{
		{name: "NotATierTheHubDoesNotDeclare", cfg: withoutTier0(), request: testRequest{coverage: true}},
		{name: "NotATierWhoseCommandLacksATargetPlaceholder", cfg: withTier0(func(t0 *gateslot.Tier) { t0.Command = "go test {args}" }), request: testRequest{coverage: true}},
		{name: "NarrowsADeclaredTierOfACoverageCall", cfg: planConfig(), request: testRequest{coverage: true}, want: true},
		{name: "NotWithoutTheCoverageScope", cfg: planConfig(), request: testRequest{}},
		{name: "NotWhenTheCallsWordsCarryAFilter", cfg: planConfig(), request: testRequest{coverage: true, args: []string{"-run", "X"}}},
		{name: "NotATierWithCarryFalse", cfg: withTier0(func(t0 *gateslot.Tier) { t0.NoCarry = true }), request: testRequest{coverage: true}},
		{name: "NotATierWhoseCommandLacksArgs", cfg: withTier0(func(t0 *gateslot.Tier) { t0.Command = "go test {targets}" }), request: testRequest{coverage: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, got := coverageTier(tt.cfg, tt.request, 0); got != tt.want {
				t.Errorf("coverageTier() narrows = %v; want %v", got, tt.want)
			}
		})
	}
}

// testFileHeader opens every fixture test file of package b.
const testFileHeader = "package b\n\nimport (\n\t\"testing\"\n\n\t\"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n)\n"

// testFunc returns the source of a Test function that opens with tiergate.Require for tier.
func testFunc(name string, tier int) string {
	return fmt.Sprintf("\nfunc %s(t *testing.T) { tiergate.Require(t, %d) }\n", name, tier)
}

// guardFunc returns the source of a tier0 Test function marked //lym:guard.
func guardFunc(name string) string {
	return "\n//lym:guard" + testFunc(name, 0)
}

// mapTests returns a selection's recorded tests named names, each with a passing measurement.
func mapTests(names ...string) []testcover.MapTest {
	tests := make([]testcover.MapTest, len(names))
	for i, name := range names {
		tests[i] = testcover.MapTest{Name: name}
	}
	return tests
}

func TestCoverageSelections(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// A request outside a hub names no maps directory, so the selector never opens and each package misses.
	selectionOf := coverageSelections(context.Background(), gateslot.Config{}, nil, t.TempDir(), testRequest{})
	for _, pkg := range []string{"internal/b", "internal/b", "internal/c"} {
		got := selectionOf(pkg)
		if got.Miss == nil || got.Miss.Reason != testcover.MissNoMap {
			t.Errorf("selection of %s = %+v; want a miss for %q", pkg, got, testcover.MissNoMap)
		}
	}
}

func TestNarrowCoverage(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	otherGOOS := "plan9"
	if runtime.GOOS == otherGOOS {
		otherGOOS = "linux"
	}
	mixed := testFileHeader + testFunc("TestSel", 0) + testFunc("TestOther", 0) + guardFunc("TestGuard") +
		testFunc("TestSpawn", 0) + testFunc("TestNew", 0) + testFunc("TestLater", 1)
	// TestSpawn and TestNew are declared and unrecorded, as a spawn-marked or new test is.
	mixedRecord := mapTests("TestSel", "TestOther", "TestGuard", "TestLater")
	var longNames strings.Builder
	longNames.WriteString(testFileHeader)
	for i := range 200 {
		longNames.WriteString(testFunc(fmt.Sprintf("TestAVeryLongNameThatFillsTheCommandBudget%03d", i), 0))
	}

	line := func(number int, target, label string) string {
		return "gate test: " + tierName(number) + " coverage: " + target + " " + label
	}
	root := func(targets ...string) []moduleTargets { return []moduleTargets{{dir: ".", targets: targets}} }
	tests := []struct {
		name      string
		files     map[string]string
		selection testcover.Selection
		unwatched []string
		want      tierScope
		wantLabel string
		// number is the tier narrowed; tier0 when unset.
		number int
		// labelPrefix makes wantLabel the start of the importer's label, which goes on with a path or a parser's text.
		labelPrefix bool
	}{
		{
			name:      "an importer runs its selected, unrecorded and guard tests of the tier",
			files:     map[string]string{"b_test.go": mixed},
			selection: testcover.Selection{Tests: []string{"TestLater", "TestSel"}, Recorded: mixedRecord},
			want: tierScope{
				modules:       root("./internal/a", "./internal/e"),
				selected:      root("./internal/b"),
				selectedTests: []string{"TestGuard", "TestNew", "TestSel", "TestSpawn"},
			},
			wantLabel: "selected 4 of 5",
		},
		{
			name:      "an importer runs its guard and unrecorded tests when the map selects none",
			files:     map[string]string{"b_test.go": mixed},
			selection: testcover.Selection{Recorded: mixedRecord},
			want: tierScope{
				modules:       root("./internal/a", "./internal/e"),
				selected:      root("./internal/b"),
				selectedTests: []string{"TestGuard", "TestNew", "TestSpawn"},
			},
			wantLabel: "selected 3 of 5",
		},
		{
			name:      "an importer with no test in the union runs nothing",
			files:     map[string]string{"b_test.go": testFileHeader + testFunc("TestOther", 0)},
			selection: testcover.Selection{Recorded: mapTests("TestOther")},
			want:      tierScope{modules: root("./internal/a", "./internal/e")},
			wantLabel: "none selected",
		},
		{
			name:      "an importer whose only union member is an unwatched guard test runs nothing",
			files:     map[string]string{"b_test.go": testFileHeader + guardFunc("TestGuard") + testFunc("TestOther", 0)},
			selection: testcover.Selection{Recorded: mapTests("TestGuard", "TestOther")},
			unwatched: []string{"TestGuard"},
			want:      tierScope{modules: root("./internal/a", "./internal/e"), skip: []string{"TestGuard"}},
			wantLabel: "none selected",
		},
		{
			name: "an unrecorded test in a file constrained to another GOOS stays out of the union",
			files: map[string]string{
				"b_test.go":       testFileHeader + testFunc("TestOther", 0),
				"b_other_test.go": "//go:build " + otherGOOS + "\n\n" + testFileHeader + testFunc("TestElsewhere", 0),
			},
			selection: testcover.Selection{Recorded: mapTests("TestOther")},
			want:      tierScope{modules: root("./internal/a", "./internal/e")},
			wantLabel: "none selected",
		},
		{
			name:      "a selection miss runs the importer whole",
			files:     map[string]string{"b_test.go": mixed},
			selection: testcover.Selection{Miss: &testcover.Miss{Reason: testcover.MissNoMap, Detail: "no map was built for the package"}},
			want:      tierScope{modules: root("./internal/a", "./internal/b", "./internal/e")},
			wantLabel: "miss: no coverage map: no map was built for the package",
		},
		{
			name:      "a union whose pattern passes the command budget runs the selected packages whole",
			files:     map[string]string{"b_test.go": longNames.String()},
			want:      tierScope{modules: root("./internal/a", "./internal/b", "./internal/e")},
			wantLabel: "whole: over budget",
		},
		{
			name:      "an importer runs a test that opens with no tiergate.Require",
			files:     map[string]string{"b_test.go": testFileHeader + "\nfunc TestPlain(t *testing.T) {}\n" + testFunc("TestOther", 0)},
			selection: testcover.Selection{Recorded: mapTests("TestOther")},
			want: tierScope{
				modules:       root("./internal/a", "./internal/e"),
				selected:      root("./internal/b"),
				selectedTests: []string{"TestPlain"},
			},
			wantLabel: "selected 1 of 2",
		},
		{
			name:   "a tier that covers another counts the covered tier's tests but not a third tier's",
			number: 1,
			files: map[string]string{"b_test.go": testFileHeader + testFunc("TestZero", 0) + testFunc("TestOne", 1) +
				testFunc("TestTwo", 2)},
			want: tierScope{
				modules:       root("./internal/a", "./internal/e"),
				selected:      root("./internal/b"),
				selectedTests: []string{"TestOne", "TestZero"},
			},
			wantLabel: "selected 2 of 2",
		},
		{
			name:        "an importer whose test file does not parse runs whole with a parse miss",
			files:       map[string]string{"b_test.go": "package b\n\nfunc (\n"},
			want:        tierScope{modules: root("./internal/a", "./internal/b", "./internal/e")},
			wantLabel:   "miss: Go file does not parse: ",
			labelPrefix: true,
		},
		{
			name:        "an importer whose guard marker is misplaced runs whole with the scan error as its label",
			files:       map[string]string{"b_test.go": testFileHeader + "\n//lym:guard\nfunc helper() {}\n"},
			want:        tierScope{modules: root("./internal/a", "./internal/b", "./internal/e")},
			wantLabel:   "miss: impactset: ",
			labelPrefix: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			worktree := t.TempDir()
			dir := filepath.Join(worktree, "internal", "b")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			for name, source := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			targets := impactset.Targets{Modules: []impactset.ModuleTargets{{
				Dir:            ".",
				Packages:       []string{"./internal/a", "./internal/b", "./internal/e"},
				Importers:      []string{"./internal/b"},
				BinaryBuilders: []string{"./internal/e"},
			}}}
			share := tierScope{modules: root(targets.Modules[0].Packages...)}
			if tt.unwatched != nil {
				targets.Unwatched = map[string][]string{"internal/b": tt.unwatched}
				share.skip = tt.unwatched
			}
			var asked []string
			selectionOf := func(pkg string) testcover.Selection {
				asked = append(asked, pkg)
				return tt.selection
			}

			got, lines := narrowCoverage(share, targets, worktree, tt.number, planConfig().Tiers[tierName(tt.number)], selectionOf)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("narrowCoverage() = %+v; want %+v", got, tt.want)
			}
			wantLines := []string{
				line(tt.number, "./internal/a", "changed"),
				line(tt.number, "./internal/b", tt.wantLabel),
				line(tt.number, "./internal/e", "whole: lym binary"),
			}
			if tt.labelPrefix && len(lines) == len(wantLines) && strings.HasPrefix(lines[1], wantLines[1]) {
				lines[1] = wantLines[1]
			}
			if !slices.Equal(lines, wantLines) {
				t.Errorf("narrowCoverage() lines = %q; want %q", lines, wantLines)
			}
			if !slices.Equal(asked, []string{"internal/b"}) {
				t.Errorf("narrowCoverage() asked the selections of %q; want the importer internal/b alone", asked)
			}
		})
	}
}
