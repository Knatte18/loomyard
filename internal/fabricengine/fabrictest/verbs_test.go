//go:build integration

// verbs_test.go drives each entry in Verbs exactly once in the "clean" state, before the cross product
// batch 7 builds multiplies any wiring mistake by nine.
// Each subtest runs the five phases in order (build, arrange, state, capture-before, run-then-capture-
// after) exactly as the driver will, so a phase-ordering mistake surfaces here rather than as a
// mysterious unpermitted-change failure across dozens of cells: without a passing clean-state cell, a
// gate that refused every request would satisfy every refusal cell in the matrix, and slice 12 just
// rewired roughly 29 call sites into that gate.

package fabrictest

import (
	"testing"
)

// cleanStateEntry returns the "clean" State from the package-level States slice, failing tb if it is
// somehow absent — every verb case's clean-state cell needs it to run the five-phase order faithfully.
func cleanStateEntry(tb testing.TB) State {
	tb.Helper()

	for _, st := range States {
		if st.Name == "clean" {
			return st
		}
	}
	tb.Fatalf("States carries no %q entry", "clean")
	return State{}
}

// TestVerbCases_CleanState drives every entry in Verbs exactly once in the clean state, at both
// anchors, asserting the three properties a clean-state cell must hold: Run's error shape matches
// Expect("clean"), the clean-state Effect closure passes, and the manifest diff reports nothing
// outside the cell's permitted roots.
func TestVerbCases_CleanState(t *testing.T) {
	for _, vc := range Verbs {
		vc := vc

		t.Run(vc.Name, func(t *testing.T) {
			for _, anchor := range []string{".", "backend"} {
				anchor := anchor

				t.Run(anchor, func(t *testing.T) {
					t.Parallel()

					// Phase 1: build.
					h := NewHub(t, anchor)

					// Phase 2: arrange.
					fixture := vc.Arrange(t, h)

					// Three targeted assertions the cross product cannot make for itself: each is a
					// precondition whose silent failure would make a whole column vacuous while still
					// passing.
					switch vc.Name {
					case "Add":
						if got := mustGitRemoteURL(t, h.PrimeWorktree()); got != fixture.BrokenOriginURL {
							t.Errorf("Add's Arrange: prime warp origin URL = %q; want the broken URL %q", got, fixture.BrokenOriginURL)
						}
					case "Checkout":
						if fixture.CheckoutBranch == fixture.OriginalBranch {
							t.Errorf("Checkout's Arrange: CheckoutBranch %q is not distinct from the branch already checked out", fixture.CheckoutBranch)
						}
					case "Pull":
						if fixture.AdvancedFromSHA == fixture.AdvancedToSHA {
							t.Errorf("Pull's Arrange: warp bare was not advanced (from == to == %s)", fixture.AdvancedFromSHA)
						}
					}

					// Phase 3: state — "clean" plants nothing, but it still asserts its own
					// establishment, matching every other state's five-phase contract.
					cleanStateEntry(t).Apply(t, h, fixture.Target)

					// Phase 4: capture (before) — after both arrange and state.
					before := CaptureManifest(t, h.Path)

					// Phase 5: run, then capture (after).
					err := vc.Run(t, h, fixture)
					exp := vc.Expect("clean")

					switch exp.Kind {
					case KindProceeds:
						if err != nil {
							t.Fatalf("%s.Run() clean state = %v; want nil (Expect(\"clean\").Kind = Proceeds)", vc.Name, err)
						}
					case KindRefusedByGate:
						if err == nil {
							t.Fatalf("%s.Run() clean state = nil; want a %s gate refusal", vc.Name, exp.Check)
						}
						if !RefusedByGate(err, exp.Check) {
							t.Errorf("%s.Run() clean state = %v; want RefusedByGate(%s)", vc.Name, err, exp.Check)
						}
					case KindRefusedBefore:
						if err == nil {
							t.Fatalf("%s.Run() clean state = nil; want a pre-flight refusal containing %q", vc.Name, exp.Substring)
						}
						if !RefusedBefore(err, exp.Substring) {
							t.Errorf("%s.Run() clean state = %v; want RefusedBefore(%q)", vc.Name, err, exp.Substring)
						}
					default:
						t.Fatalf("%s.Expect(\"clean\").Kind = %v: unrecognised expectation kind", vc.Name, exp.Kind)
					}

					if exp.Effect != nil {
						exp.Effect(t, h, fixture)
					}

					after := CaptureManifest(t, h.Path)
					AssertNoUnpermittedChange(t, before, after, exp.PermittedRoots)
				})
			}
		})
	}
}

// TestVerbCases_StatesRestrictionIsWellFormed proves every hostile-input and CloneHub{Reset} case
// restricts States to exactly ["clean"], and every ordinary verb leaves States empty (inheriting the
// full ten-state matrix) — a case that got this backwards would either vanish from the cross product
// entirely or explode it with cells that were never derived.
func TestVerbCases_StatesRestrictionIsWellFormed(t *testing.T) {
	ordinary := map[string]bool{
		"Add": true, "Remove": true, "Prune": true, "Cleanup": true,
		"Checkout": true, "Reconcile": true, "UnwireJunctions": true, "Pull": true,
	}

	for _, vc := range Verbs {
		if ordinary[vc.Name] {
			if len(vc.States) != 0 {
				t.Errorf("%s: States = %v; want empty (ordinary verbs inherit the full matrix)", vc.Name, vc.States)
			}
			continue
		}

		if len(vc.States) != 1 || vc.States[0] != "clean" {
			t.Errorf("%s: States = %v; want exactly [\"clean\"]", vc.Name, vc.States)
		}
	}
}
