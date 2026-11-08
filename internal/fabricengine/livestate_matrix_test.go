// livestate_matrix_test.go is the cross-product driver: the one uniform loop over states, verbs and anchors.
// It runs every cell that livestate_verbs_test.go and livestate_states_test.go describe.
// It contains no per-verb or per-state special case.
// Every restriction the loop honors is data, either on a VerbCase's States field or as a row in the Omissions table.
// A conditional here would let the cross-product property quietly stop holding for the next verb someone appends.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// anchors is the two anchor values that every ordinary and hostile-input cell in the cross product runs against.
var anchors = []string{".", "backend"}

// isCloneHubResetCase reports whether vc is a CloneHub{Reset: true} column entry, by its "CloneHubReset/" name prefix.
// That column is scoped to the ownership axis rather than the States matrix, so a separate test drives it.
// Folding it into the loop would reintroduce the vacuous dirtiness rows it is exempt from.
// The alternative is a per-verb conditional, which the loop must not contain.
func isCloneHubResetCase(vc VerbCase) bool {
	return strings.HasPrefix(vc.Name, "CloneHubReset/")
}

// mainVerbs returns the VerbCase entries TestCrossProduct drives: the ordinary verbs and the hostile-input cases.
// It excludes the CloneHub{Reset: true} column.
func mainVerbs() []VerbCase {
	var out []VerbCase
	for _, vc := range Verbs {
		if !isCloneHubResetCase(vc) {
			out = append(out, vc)
		}
	}
	return out
}

// verbTemplates holds, per anchor, the registered template of each Verbs case that has a Build, keyed by verbTemplateKey.
var verbTemplates = registerVerbTemplates()

// registerVerbTemplates returns one registration per anchor for each Verbs case that has a Build:
// the anchor's bare hub with the case's Build run on it.
func registerVerbTemplates() map[string]*hubforge.Registration {
	templates := map[string]*hubforge.Registration{}
	for _, vc := range Verbs {
		if vc.Build == nil {
			continue
		}
		for _, anchor := range anchors {
			key := verbTemplateKey(vc.Name, anchor)
			templates[key] = &hubforge.Registration{Key: key, Base: hubforge.Shape{Anchor: anchor}, Build: vc.Build}
		}
	}
	return templates
}

// verbTemplateKey returns the template key of the verb case verbName at anchor.
func verbTemplateKey(verbName, anchor string) string {
	return "fabricengine-livestate-" + verbName + "-" + anchor
}

// copyVerbHub returns a hub the test owns for vc at anchor, before vc's Arrange runs.
// It is a copy of vc's registered template when vc has a Build, and of the anchor's bare hub otherwise.
func copyVerbHub(t *testing.T, vc VerbCase, anchor string) *hubforge.Hub {
	t.Helper()

	if vc.Build == nil {
		return hubforge.CopyHub(t, hubforge.Shape{Anchor: anchor})
	}
	return hubforge.CopyRegistered(t, verbTemplates[verbTemplateKey(vc.Name, anchor)])
}

// cloneHubResetVerbs returns the VerbCase entries of the CloneHub{Reset: true} column.
func cloneHubResetVerbs() []VerbCase {
	var out []VerbCase
	for _, vc := range Verbs {
		if isCloneHubResetCase(vc) {
			out = append(out, vc)
		}
	}
	return out
}

// cloneHubResetCellCount returns the cell count of the CloneHub{Reset: true} column.
// It is the column's VerbCase entries times the anchor count.
// TestCrossProduct adds this count to its single total, so TestCloneHubReset's loop is not repeated there.
func cloneHubResetCellCount() int {
	return len(cloneHubResetVerbs()) * len(anchors)
}

// effectiveStates returns the states vc runs against.
// When vc.States names any, it returns those states from the package-level States slice.
// Hostile-input cases and the CloneHub{Reset: true} column name only "clean".
// When vc.States is empty, it returns the full States slice, which is every ordinary verb's default.
// A state the case does not name never becomes a subtest, so the loop generates no cell for it.
func effectiveStates(vc VerbCase) []State {
	if len(vc.States) == 0 {
		return States
	}
	var subset []State
	for _, name := range vc.States {
		for _, s := range States {
			if s.Name == name {
				subset = append(subset, s)
				break
			}
		}
	}
	return subset
}

// isOmitted reports whether the (verbName, stateName) pair is in the Omissions table.
// The loop generates no cell for such a pair.
func isOmitted(verbName, stateName string) bool {
	for _, o := range Omissions {
		if o.Verb == verbName && o.State == stateName {
			return true
		}
	}
	return false
}

// assertExpectation asserts err against exp's declared Kind.
// KindRefusedByGate asserts RefusedByGate for exp.Check.
// It also asserts that err matches neither of the other two gate checks.
// KindRefusedBefore asserts RefusedBefore for exp.Substring.
// That also asserts that err does not carry "check failed".
// KindProceeds asserts err == nil.
func assertExpectation(t *testing.T, err error, exp Expectation) {
	t.Helper()

	switch exp.Kind {
	case KindRefusedByGate:
		if !RefusedByGate(err, exp.Check) {
			t.Errorf("expected refusal by gate check %q; got err = %v", exp.Check, err)
		}
		for _, other := range []fabricengine.Check{fabricengine.CheckContainment, fabricengine.CheckOwnership, fabricengine.CheckDirtiness} {
			if other == exp.Check {
				continue
			}
			if RefusedByGate(err, other) {
				t.Errorf("err unexpectedly also matches gate check %q: %v", other, err)
			}
		}
	case KindRefusedBefore:
		if !RefusedBefore(err, exp.Substring) {
			t.Errorf("expected pre-flight refusal containing %q; got err = %v", exp.Substring, err)
		}
	case KindProceeds:
		if err != nil {
			t.Errorf("expected the verb to proceed; got err = %v", err)
		}
	case KindEitherProceedsOrRefusedBefore:
		if err != nil && !RefusedBefore(err, exp.Substring) {
			t.Errorf("expected either success or a pre-flight refusal containing %q; got err = %v", exp.Substring, err)
		}
	default:
		t.Fatalf("unknown ExpectationKind %v", exp.Kind)
	}
}

// assertPlantedContentSurvives asserts that stateName's planted content survives outside the permitted roots.
// A path inside a permitted root may legitimately vanish as part of the verb's own intended effect.
// Prune, for one, tears down the stale weft sibling a dirtiness state planted into.
// Asserting survival there would fail against correct behaviour.
// Outside a permitted root, this check can fail only where AssertNoUnpermittedChange also fails on the same path.
// So it adds no new failure mode.
// It makes survival an explicit cell-level assertion, not an implication a reader derives from the manifest diff.
func assertPlantedContentSurvives(t *testing.T, h *hubforge.Hub, stateName string, target StateTarget, permitted []string) {
	t.Helper()

	switch stateName {
	case "clean":
		return
	case "dirtyWarpTracked":
		assertSurvivesIfUnpermitted(t, h, target.WarpCheckout, permitted, func() {
			assertDirtyContains(t, target.WarpCheckout, sentinelFileName(stateName), " M")
		})
	case "dirtyWarpUntracked":
		assertSurvivesIfUnpermitted(t, h, target.WarpCheckout, permitted, func() {
			assertDirtyContains(t, target.WarpCheckout, sentinelFileName(stateName), "??")
		})
	case "dirtyWeftTracked":
		assertSurvivesIfUnpermitted(t, h, target.WeftCheckout, permitted, func() {
			assertDirtyContains(t, target.WeftCheckout, sentinelFileName(stateName), " M")
		})
	case "dirtyWeftUntracked":
		assertSurvivesIfUnpermitted(t, h, target.WeftCheckout, permitted, func() {
			assertDirtyContains(t, target.WeftCheckout, sentinelFileName(stateName), "??")
		})
	case "bothDirty":
		assertSurvivesIfUnpermitted(t, h, target.WarpCheckout, permitted, func() {
			assertDirtyContains(t, target.WarpCheckout, "bothDirty-warp-tracked", " M")
			assertDirtyContains(t, target.WarpCheckout, "bothDirty-warp-untracked", "??")
		})
		assertSurvivesIfUnpermitted(t, h, target.WeftCheckout, permitted, func() {
			assertDirtyContains(t, target.WeftCheckout, "bothDirty-weft-tracked", " M")
			assertDirtyContains(t, target.WeftCheckout, "bothDirty-weft-untracked", "??")
		})
	case "trackedSymlinkAtWiredPath", "staleWiredJunction":
		assertSurvivesIfUnpermitted(t, h, target.StructuralPath, permitted, func() {
			assertExists(t, target.StructuralPath)
		})
	case "foreignDirAtFabricOwnedPath", "unrelatedGitCloneAtWeftNamedPath":
		assertSurvivesIfUnpermitted(t, h, target.StructuralPath, permitted, func() {
			assertExists(t, filepath.Join(target.StructuralPath, sentinelFileName(stateName)))
		})
	}
}

// assertSurvivesIfUnpermitted runs check unless abs is empty or falls at or below one of permitted's roots.
// Such an abs may legitimately have changed as part of the verb's own intended effect.
func assertSurvivesIfUnpermitted(t *testing.T, h *hubforge.Hub, abs string, permitted []string, check func()) {
	t.Helper()

	if abs == "" {
		return
	}
	if pathPermitted(hubRelative(t, h, abs), permitted) {
		return
	}
	check()
}

// assertArrangePreconditions fails t when verb's Arrange left a fixture that would make the verb's column vacuous.
// Add, Checkout and Pull each need one fixture field set, and an empty one fails as surely as a wrong one:
// A launcher blocker that is not a directory lets Add proceed.
// A checkout branch equal to the original makes Checkout a no-op.
// An unadvanced warp bare makes Pull return before its dirty check.
func assertArrangePreconditions(t *testing.T, verb string, fixture VerbFixture) {
	t.Helper()

	switch verb {
	case "Add":
		if fixture.LauncherBlocker == "" {
			t.Fatalf("Arrange: Add left LauncherBlocker empty")
		}
		if info, err := os.Lstat(fixture.LauncherBlocker); err != nil || !info.IsDir() {
			t.Errorf("Arrange: launcher blocker %s is not a directory (err %v)", fixture.LauncherBlocker, err)
		}
	case "Checkout":
		if fixture.CheckoutBranch == "" {
			t.Fatalf("Arrange: Checkout left CheckoutBranch empty")
		}
		if fixture.CheckoutBranch == fixture.OriginalBranch {
			t.Errorf("Arrange: CheckoutBranch %q is not distinct from the branch already checked out", fixture.CheckoutBranch)
		}
	case "Pull":
		if fixture.AdvancedToSHA == "" {
			t.Fatalf("Arrange: Pull left AdvancedToSHA empty")
		}
		if fixture.AdvancedFromSHA == fixture.AdvancedToSHA {
			t.Errorf("Arrange: warp bare was not advanced (from == to == %s)", fixture.AdvancedFromSHA)
		}
	}
}

// runCell drives one non-omitted cell through five phases in a fixed order:
// build, arrange, state, capture before, then run and capture after.
// Right after arrange it checks the fixture's preconditions, before the state can disturb them.
// It then asserts against the state-derived Expectation: the declared Kind, and the cell's Effect when it names one.
// exp.Effect runs whenever it is present, for refusal and Proceeds kinds alike, not only for one Kind.
// For a Proceeds kind it also checks planted-content survival.
// Finally it checks that nothing outside the cell's permitted roots changed.
func runCell(t *testing.T, anchor string, state State, vc VerbCase) {
	t.Helper()

	// Build: a copy of the verb's template at the cell's anchor.
	// Every cell owns its hub, so pushes across parallel cells never race.
	h := copyVerbHub(t, vc, anchor)

	// Arrange: the rest of the verb's own fixture, derived from the copy.
	fixture := vc.Arrange(t, h)
	assertArrangePreconditions(t, vc.Name, fixture)

	// State: the hostile or dirty condition, planted against the fixture's own resolved target.
	state.Apply(t, h, fixture.Target)

	// Capture (before): after both arrange and state, so each is baseline rather than diff noise.
	before := CaptureManifest(t, h.Path)

	// Run, then capture (after).
	rec, err := vc.Run(t, h, fixture)
	after := CaptureManifest(t, h.Path)

	exp := vc.Expect(state.Name)
	assertExpectation(t, err, exp)
	if exp.Effect != nil {
		exp.Effect(t, h, fixture)
	}
	if exp.Kind == KindProceeds {
		assertPlantedContentSurvives(t, h, state.Name, fixture.Target, exp.PermittedRoots)
	}
	// Permitted removal roots suppress diff noise only, never the honesty assertion.
	// exp.PermittedRoots narrows what AssertNoUnpermittedChange treats as an unpermitted change.
	// The truthfulness oracle below always runs against the unfiltered, nil-permitted diff.
	// A cell that destroys content inside a permitted root still has its record checked against it.
	AssertNoUnpermittedChange(t, before, after, exp.PermittedRoots)
	unfiltered := DiffManifest(before, after, nil)
	AssertRecordMatchesDiff(t, rec, unfiltered)
}

// TestCrossProduct runs the ordinary and hostile-input verbs against each state they declare, at both anchors.
// Each cell is an independent parallel subtest.
// The CloneHub{Reset: true} column is absent from this loop and a separate test runs it.
func TestCrossProduct(t *testing.T) {
	tiergate.Require(t, 1)
	verbs := mainVerbs()
	ran := 0
	omitted := 0

	for _, anchor := range anchors {
		for _, verbCase := range verbs {
			for _, state := range effectiveStates(verbCase) {
				anchor, verbCase, state := anchor, verbCase, state
				name := anchor + "/" + state.Name + "/" + verbCase.Name

				// The Omissions table records what the matrix does not run and why,
				// so an omitted cell gets no subtest and only the tally below counts it.
				if isOmitted(verbCase.Name, state.Name) {
					omitted++
					continue
				}

				ran++
				t.Run(name, func(t *testing.T) {
					// Every cell owns its own hub, so pushes across parallel cells never race.
					// t.Parallel() is safe here, and the -parallel flag already bounds concurrency,
					// so no hand-rolled worker cap is needed.
					t.Parallel()
					runCell(t, anchor, state, verbCase)
				})
			}
		}
	}

	assertCellTally(t, ran, omitted)
}

// assertCellTally fails t unless ran and omitted match the enumeration the Verbs, States and Omissions tables describe.
// A silently capped loop or an omission row naming no cell cannot hide behind a green run.
// The want* values are derived from the tables' lengths and contents, never hardcoded.
// Appending a verb, a state or an omission updates the expected count,
// so the run does not fail for an unrelated reason.
func assertCellTally(t *testing.T, ran, omitted int) {
	t.Helper()

	wantOmitted := len(Omissions) * len(anchors)
	if omitted != wantOmitted {
		t.Errorf("TestCrossProduct omitted %d cells; want %d (len(Omissions) * len(anchors))", omitted, wantOmitted)
	}

	wantRanPerAnchor := 0
	for _, verbCase := range mainVerbs() {
		for _, state := range effectiveStates(verbCase) {
			if !isOmitted(verbCase.Name, state.Name) {
				wantRanPerAnchor++
			}
		}
	}
	wantRan := wantRanPerAnchor * len(anchors)
	if ran != wantRan {
		t.Errorf("TestCrossProduct ran %d cells; want %d (derived from Verbs, States and Omissions)", ran, wantRan)
	}

	total := ran + cloneHubResetCellCount()
	wantTotal := wantRan + cloneHubResetCellCount()
	if total != wantTotal {
		t.Errorf("cross-product total (TestCrossProduct's ran cells + the CloneHub{Reset} column) = %d; want %d", total, wantTotal)
	}
	t.Logf("cross-product cell tally: %d ran, %d omitted (omission table), %d CloneHub{Reset} column cells, %d total", ran, omitted, cloneHubResetCellCount(), total)
}

// TestCloneHubReset drives the CloneHub{Reset: true} column at both anchors, with two targets:
// a non-hub directory, which CloneHub's offline pre-flight refuses, and a real hub that is torn down and rebuilt.
// It varies ownership, not the States matrix; folding it into the loop would add vacuous dirty-state rows.
// It reads fixture.ResetHubPath rather than h.Path.
// The non-hub target is independent of the cell's hub, so h only satisfies VerbCase.Arrange's signature there.
func TestCloneHubReset(t *testing.T) {
	tiergate.Require(t, 1)
	for _, anchor := range anchors {
		for _, verbCase := range cloneHubResetVerbs() {
			anchor, verbCase := anchor, verbCase
			name := anchor + "/" + verbCase.Name

			t.Run(name, func(t *testing.T) {
				t.Parallel()

				h := hubforge.CopyHub(t, hubforge.Shape{Anchor: anchor})
				fixture := verbCase.Arrange(t, h)

				before := CaptureManifest(t, fixture.ResetHubPath)
				rec, err := verbCase.Run(t, h, fixture)
				after := CaptureManifest(t, fixture.ResetHubPath)

				exp := verbCase.Expect("clean")
				assertExpectation(t, err, exp)
				if exp.Effect != nil {
					exp.Effect(t, h, fixture)
				}
				AssertNoUnpermittedChange(t, before, after, exp.PermittedRoots)
				unfiltered := DiffManifest(before, after, nil)
				AssertRecordMatchesDiff(t, rec, unfiltered)
			})
		}
	}
}
