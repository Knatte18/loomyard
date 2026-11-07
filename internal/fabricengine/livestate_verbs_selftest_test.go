// livestate_verbs_selftest_test.go drives each entry in Verbs exactly once in the "clean" state.
// Each subtest runs the five phases in order, exactly as the driver does.
// A phase-ordering mistake therefore surfaces here, not as an unpermitted-change failure in another cell.
// Without a passing clean-state cell, a gate that refused every request would satisfy every refusal cell.

package fabricengine_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// cleanStateEntry returns the "clean" State from the package-level States slice.
// It fails tb if that entry is absent, since every verb case's clean-state cell needs it.
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

// TestVerbCases_CleanState drives every entry in Verbs exactly once in the clean state, at each anchor.
// It asserts the three properties a clean-state cell must hold:
// Run's error shape matches Expect("clean"), the clean-state Effect closure passes,
// and the manifest diff reports nothing outside the cell's permitted roots.
func TestVerbCases_CleanState(t *testing.T) {
	tiergate.Require(t, 1)
	for _, vc := range Verbs {
		vc := vc

		t.Run(vc.Name, func(t *testing.T) {
			for _, anchor := range []string{".", "backend"} {
				anchor := anchor

				t.Run(anchor, func(t *testing.T) {
					t.Parallel()

					// Phase 1: build.
					h := hubforge.CopyHub(t, hubforge.Shape{Anchor: anchor})

					// Phase 2: arrange.
					fixture := vc.Arrange(t, h)

					// These targeted assertions check preconditions the cross product cannot check for itself.
					// A silent failure of any one would make a whole column vacuous.
					// The test would still pass.
					switch vc.Name {
					case "Add":
						if info, err := os.Lstat(fixture.LauncherBlocker); err != nil || !info.IsDir() {
							t.Errorf("Add's Arrange: launcher blocker %s is not a directory (err %v)", fixture.LauncherBlocker, err)
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
					// establishment, as every state does.
					cleanStateEntry(t).Apply(t, h, fixture.Target)

					// Phase 4: capture (before) — after both arrange and state.
					before := CaptureManifest(t, h.Path)

					// This test asserts each case's error shape, not its record, so the returned Mutations is discarded.
					// The matrix cells assert the record.
					_, err := vc.Run(t, h, fixture)
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

// TestVerbCases_StatesRestrictionIsWellFormed proves that every hostile-input and CloneHub{Reset} case
// restricts States to exactly ["clean"], and that every ordinary verb leaves States empty.
// An empty States inherits the full state matrix.
// A case that got this backwards would vanish from the cross product, or explode it with cells that were never derived.
//
//testtiming:keep hostile-input and CloneHub{Reset} cases restricting States to clean and ordinary verbs inheriting the full matrix; coverage of its blocks by other tests does not show an assertion of this
func TestVerbCases_StatesRestrictionIsWellFormed(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

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
