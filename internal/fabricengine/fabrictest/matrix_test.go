//go:build integration

// matrix_test.go is the cross-product driver: the one uniform loop over states, verbs and anchors that
// runs every cell verbs.go and states.go describe, and the sole deliverable of batch 7.
// It contains no per-verb or per-state special case — every restriction the loop honors is expressed as
// data, either on a VerbCase's own States field or as a row in verbs.go's Omissions table — because a
// conditional here would be the exact point at which the cross-product property quietly stops holding
// for the next verb someone appends.

package fabrictest

import (
	"path/filepath"
	"strings"
	"testing"
)

// anchors is the two anchor values every ordinary and hostile-input cell in the cross product runs
// against.
var anchors = []string{".", "backend"}

// isCloneHubResetCase reports whether vc is one of the two CloneHub{Reset: true} column entries in
// Verbs, by its "CloneHubReset/" name prefix.
// The column is re-scoped to the ownership axis rather than the ten-member States matrix (see verbs.go's
// doc comment), so it is driven by TestCloneHubReset instead of TestCrossProduct's loop; folding it into
// the loop would either reintroduce the vacuous dirtiness rows it is exempt from, or require a per-verb
// conditional the plan rejects.
func isCloneHubResetCase(vc VerbCase) bool {
	return strings.HasPrefix(vc.Name, "CloneHubReset/")
}

// mainVerbs returns every VerbCase TestCrossProduct's loop drives: the ordinary verbs and the
// hostile-input cases, excluding the CloneHub{Reset: true} column.
func mainVerbs() []VerbCase {
	var out []VerbCase
	for _, vc := range Verbs {
		if !isCloneHubResetCase(vc) {
			out = append(out, vc)
		}
	}
	return out
}

// cloneHubResetVerbs returns the CloneHub{Reset: true} column's own VerbCase entries.
func cloneHubResetVerbs() []VerbCase {
	var out []VerbCase
	for _, vc := range Verbs {
		if isCloneHubResetCase(vc) {
			out = append(out, vc)
		}
	}
	return out
}

// cloneHubResetCellCount is the CloneHub{Reset: true} column's own cell count: its VerbCase entries
// times the anchor count, the same arithmetic TestCloneHubReset's own loop drives.
// TestCrossProduct's tally folds this count into the plan's single cross-product total without
// duplicating TestCloneHubReset's loop.
func cloneHubResetCellCount() int {
	return len(cloneHubResetVerbs()) * len(anchors)
}

// effectiveStates returns the states vc actually runs against: vc.States when it names one, which is
// what restricts a hostile-input case or the CloneHub{Reset} column to "clean" alone, or the full
// package-level States slice when vc.States is empty, which is every ordinary verb's default.
// A state a case does not name is never turned into a subtest at all -- there is no verb-shaped reason
// to generate five hundred mostly-skipped cells when the case's own States field already says which
// ones apply.
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

// omittedReason reports the recorded reason and true if the (verbName, stateName) pair appears in
// verbs.go's Omissions table, so a matching cell is skipped rather than silently absent.
func omittedReason(verbName, stateName string) (string, bool) {
	for _, o := range Omissions {
		if o.Verb == verbName && o.State == stateName {
			return o.Reason, true
		}
	}
	return "", false
}

// assertExpectation asserts err against exp's declared Kind: KindRefusedByGate asserts RefusedByGate for
// exp.Check and that err does not also match either of the other two gate checks; KindRefusedBefore
// asserts RefusedBefore for exp.Substring, which by construction also asserts err does not carry "check
// failed"; KindProceeds asserts err == nil.
func assertExpectation(t *testing.T, err error, exp Expectation) {
	t.Helper()

	switch exp.Kind {
	case KindRefusedByGate:
		if !RefusedByGate(err, exp.Check) {
			t.Errorf("expected refusal by gate check %q; got err = %v", exp.Check, err)
		}
		for _, other := range []Check{CheckContainment, CheckOwnership, CheckDirtiness} {
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
	default:
		t.Fatalf("unknown ExpectationKind %v", exp.Kind)
	}
}

// assertPlantedContentSurvives asserts that stateName's own planted content is still present, but only
// at whichever of the checkouts or structural path it dirtied that fall OUTSIDE exp's permitted roots --
// a path inside a permitted root may legitimately vanish as part of the verb's own intended effect (Prune
// tearing down the very stale weft sibling a dirtiness state planted into, for one), so asserting survival
// there would fail against correct behaviour.
// Outside a permitted root this check can never fail without AssertNoUnpermittedChange also failing on
// the same path, so it adds no new failure mode -- it exists to make survival an explicit, named
// assertion at the cell level, per the five-phase-cell-order Shared Decision's KindProceeds requirement,
// rather than an implication a reader has to derive from the manifest diff alone.
func assertPlantedContentSurvives(t *testing.T, h *Hub, stateName string, target StateTarget, permitted []string) {
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

// assertSurvivesIfUnpermitted runs check unless abs is empty or falls at or below one of permitted's
// roots, in which case abs may legitimately have changed as part of the verb's own intended effect.
func assertSurvivesIfUnpermitted(t *testing.T, h *Hub, abs string, permitted []string, check func()) {
	t.Helper()

	if abs == "" {
		return
	}
	if pathPermitted(hubRelative(t, h, abs), permitted) {
		return
	}
	check()
}

// runCell drives one non-skipped cell's five phases in the fixed order the five-phase-cell-order Shared
// Decision requires -- build, arrange, state, capture-before, run-then-capture-after -- then asserts
// against the state-derived Expectation: the declared Kind, the cell's Effect if it names one (populated
// for both refusal and Proceeds kinds throughout verbs.go, so it is always run when present rather than
// gated on Kind), planted-content survival for a Proceeds kind, and finally that nothing outside the
// cell's permitted roots changed.
func runCell(t *testing.T, anchor string, state State, vc VerbCase) {
	t.Helper()

	// Build: the factory clones a fresh hub at the cell's anchor. Every cell owns its hub, so pushes
	// across parallel cells never race.
	h := NewHub(t, anchor)

	// Arrange: the verb's own fixture.
	fixture := vc.Arrange(t, h)

	// State: the hostile or dirty condition, planted against the fixture's own resolved target.
	state.Apply(t, h, fixture.Target)

	// Capture (before): after both arrange and state, so each is baseline rather than diff noise.
	before := CaptureManifest(t, h.Path)

	// Run, then capture (after).
	err := vc.Run(t, h, fixture)
	after := CaptureManifest(t, h.Path)

	exp := vc.Expect(state.Name)
	assertExpectation(t, err, exp)
	if exp.Effect != nil {
		exp.Effect(t, h, fixture)
	}
	if exp.Kind == KindProceeds {
		assertPlantedContentSurvives(t, h, state.Name, fixture.Target, exp.PermittedRoots)
	}
	AssertNoUnpermittedChange(t, before, after, exp.PermittedRoots)
}

// TestCrossProduct drives the ordinary verbs and the hostile-input cases against every state each
// declares, at both anchors, as independent parallel subtests, one per cell.
// The CloneHub{Reset: true} column is deliberately absent from this loop; see TestCloneHubReset.
func TestCrossProduct(t *testing.T) {
	verbs := mainVerbs()
	ran := 0
	skipped := 0

	for _, anchor := range anchors {
		for _, verbCase := range verbs {
			for _, state := range effectiveStates(verbCase) {
				anchor, verbCase, state := anchor, verbCase, state
				name := anchor + "/" + state.Name + "/" + verbCase.Name

				reason, omitted := omittedReason(verbCase.Name, state.Name)
				if omitted {
					skipped++
					t.Run(name, func(t *testing.T) {
						// A skipped cell states out loud what it did not run, and why, rather than
						// silently omitting it -- so a green matrix is auditable against doc.go's
						// omission table without a reader having to reconstruct the exclusion by hand.
						t.Skip(reason)
					})
					continue
				}

				ran++
				t.Run(name, func(t *testing.T) {
					// Every cell owns its own hub (see runCell's Build phase), so pushes across
					// parallel cells never race -- t.Parallel() is free here, and Go's own -parallel
					// flag already bounds concurrency, so no hand-rolled worker cap is needed.
					t.Parallel()
					runCell(t, anchor, state, verbCase)
				})
			}
		}
	}

	assertCellTally(t, ran, skipped)
}

// assertCellTally fails t unless ran and skipped match the enumeration the Verbs, States and Omissions
// tables in this package describe, so a silently capped loop or a silently swallowed skip cannot hide
// behind a green run.
// Every want* value below is derived from the tables' own lengths and contents, never a hardcoded
// literal, so appending a verb, a state, or an omission updates the expected count automatically instead
// of failing the next run for an unrelated reason.
func assertCellTally(t *testing.T, ran, skipped int) {
	t.Helper()

	wantSkipped := len(Omissions) * len(anchors)
	if skipped != wantSkipped {
		t.Errorf("TestCrossProduct skipped %d cells; want %d (len(Omissions) * len(anchors))", skipped, wantSkipped)
	}

	wantRanPerAnchor := 0
	for _, verbCase := range mainVerbs() {
		for _, state := range effectiveStates(verbCase) {
			if _, omitted := omittedReason(verbCase.Name, state.Name); !omitted {
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
	t.Logf("cross-product cell tally: %d ran, %d skipped (omission table), %d CloneHub{Reset} column cells, %d total", ran, skipped, cloneHubResetCellCount(), total)
}

// TestCloneHubReset drives the CloneHub{Reset: true} column: two targets (a non-hub directory refused at
// resetHub's own pre-flight, and a real hub torn down and rebuilt) at both anchors.
// It is a separate test function rather than a row in TestCrossProduct's loop because the column is
// scoped to the ownership axis, not to the ten-member States matrix -- see verbs.go's doc comment on
// cloneHubResetNonHubCase and cloneHubResetRealHubCase for why folding it into the loop would either
// reintroduce vacuous dirtiness rows or require a per-verb conditional the plan rejects.
// It manifests fixture.ResetHubPath rather than the built hub's own h.Path, because
// cloneHubResetNonHubCase's target is a directory entirely independent of the hub NewHub built for this
// cell -- h exists only to satisfy VerbCase.Arrange's signature for that case, and is the real reset
// target only for cloneHubResetRealHubCase, where fixture.ResetHubPath equals h.Path.
func TestCloneHubReset(t *testing.T) {
	for _, anchor := range anchors {
		for _, verbCase := range cloneHubResetVerbs() {
			anchor, verbCase := anchor, verbCase
			name := anchor + "/" + verbCase.Name

			t.Run(name, func(t *testing.T) {
				t.Parallel()

				h := NewHub(t, anchor)
				fixture := verbCase.Arrange(t, h)

				before := CaptureManifest(t, fixture.ResetHubPath)
				err := verbCase.Run(t, h, fixture)
				after := CaptureManifest(t, fixture.ResetHubPath)

				exp := verbCase.Expect("clean")
				assertExpectation(t, err, exp)
				if exp.Effect != nil {
					exp.Effect(t, h, fixture)
				}
				AssertNoUnpermittedChange(t, before, after, exp.PermittedRoots)
			})
		}
	}
}
