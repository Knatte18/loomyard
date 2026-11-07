// livestate_refusal_selftest_test.go checks that RefusedByGate and RefusedBefore pin the correct layer.
// They are checked against real errors from real verbs on real hubs.
// It covers one gate refusal per reachable Check member.
// It also covers two negative properties that keep the matching from being merely permissive.
// It covers the pre-flight cases of RefusedBefore, including the pair the "check failed" exclusion exists for.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// driveContainmentGateRefusal drives a real containment refusal through UnwireJunctions.
// A junction name of "../<escapeName>" cancels the slug segment and lands the link at the hub itself,
// and that puts the link outside the worktree UnwireJunctions was asked to operate on.
func driveContainmentGateRefusal(t testing.TB, h *hubforge.Hub, slug, escapeName string) error {
	t.Helper()

	escapeLink := filepath.Join(h.Path, escapeName)
	elsewhere := t.TempDir()
	if err := fslink.CreateDirLink(escapeLink, elsewhere); err != nil {
		t.Fatalf("create escape link %s: %v", escapeLink, err)
	}

	_, err := fabricengine.UnwireJunctions(h.Location, slug, []string{"../" + escapeName})
	return err
}

// driveOwnershipGateRefusal drives a real ownership refusal through UnwireJunctions.
// The gate's ownership check can diverge from the pre-flight check.
// The pre-flight resolves the link with fslink.PointsTo, which walks every hop;
// the gate reads fslink.RawTarget, the one hop a wiring call recorded.
// Chaining the junction through an intermediate symlink makes PointsTo pass, so removal is attempted.
// RawTarget then reads the intermediate, which does not match the gate's expected target.
// This is a gate-only refusal, not a duplicate of a pre-flight check.
func driveOwnershipGateRefusal(t testing.TB, h *hubforge.Hub, slug string) error {
	t.Helper()

	hubforge.AddPair(t, h, slug)

	names, err := fabricengine.WiredNames(configengine.Dirs{Board: h.BoardDir()})
	if err != nil {
		t.Fatalf("fabricengine.WiredNames(%s): %v", h.BoardDir(), err)
	}
	if len(names) == 0 {
		t.Fatalf("fabricengine.WiredNames(%s): want at least one wired name, got none", h.BoardDir())
	}

	junction := fabricengine.WarpJunctions(h.Location, slug, names)[0]

	intermediate := filepath.Join(t.TempDir(), "intermediate")
	if err := fslink.CreateDirLink(intermediate, junction.Target); err != nil {
		t.Fatalf("create intermediate link %s: %v", intermediate, err)
	}
	if err := fslink.Remove(junction.Link); err != nil {
		t.Fatalf("remove original junction link %s: %v", junction.Link, err)
	}
	if err := fslink.CreateDirLink(junction.Link, intermediate); err != nil {
		t.Fatalf("chain junction link %s through %s: %v", junction.Link, intermediate, err)
	}

	_, err = fabricengine.UnwireJunctions(h.Location, slug, names)
	return err
}

// driveDirtinessGateRefusal drives a real dirtiness refusal through Fabric.ResetHard.
// The gate's pathRequest there hardcodes force: false, so the refusal cannot be answered by a flag,
// and no caller's pre-flight stands in front of it to claim the planted dirt first.
// ResetHard's force flag is a constant in production source.
// That makes this the honest way to reach a gate dirtiness refusal.
func driveDirtinessGateRefusal(t testing.TB, h *hubforge.Hub, slug string) error {
	t.Helper()

	hubforge.AddPair(t, h, slug)

	pairLocation := &lymcwd.Location{HubPath: h.Path, WorktreeName: slug, AnchorRel: h.Location.AnchorRel}
	warpTarget := h.PairCodeWorktree(slug)

	trackedFile := filepath.Join(warpTarget, "dirt.txt")
	head := gitkit.CommitFile(t, warpTarget, "dirt.txt", "v1\n", "livestate: seed tracked file for dirtiness refusal")
	if err := os.WriteFile(trackedFile, []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("modify %s: %v", trackedFile, err)
	}

	f, err := fabricengine.Open(pairLocation)
	if err != nil {
		t.Fatalf("fabricengine.Open(%s): %v", warpTarget, err)
	}
	// The gate refuses before repo.ResetHard is called, so this SHA is never reset to;
	// it is the pair's own HEAD so the request is a realistic one rather than a sentinel.
	return f.ResetHard(fabricengine.NewMutations(h.Path), head)
}

// TestRefusedByGate proves RefusedByGate matches a real gate refusal for each reachable Check member.
// Each member is driven by its own scenario.
func TestRefusedByGate(t *testing.T) {
	tiergate.Require(t, 1)
	t.Run("Containment", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		err := driveContainmentGateRefusal(t, h, "escape-owner", "escape-link")
		if err == nil {
			t.Fatalf("driveContainmentGateRefusal: want a refusal, got nil")
		}
		if !RefusedByGate(err, fabricengine.CheckContainment) {
			t.Errorf("RefusedByGate(%v, CheckContainment) = false; want true", err)
		}
	})

	t.Run("Ownership", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		err := driveOwnershipGateRefusal(t, h, "own-owner")
		if err == nil {
			t.Fatalf("driveOwnershipGateRefusal: want a refusal, got nil")
		}
		if !RefusedByGate(err, fabricengine.CheckOwnership) {
			t.Errorf("RefusedByGate(%v, CheckOwnership) = false; want true", err)
		}
	})

	t.Run("Dirtiness", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		err := driveDirtinessGateRefusal(t, h, "dirty-owner")
		if err == nil {
			t.Fatalf("driveDirtinessGateRefusal: want a refusal, got nil")
		}
		if !RefusedByGate(err, fabricengine.CheckDirtiness) {
			t.Errorf("RefusedByGate(%v, CheckDirtiness) = false; want true", err)
		}
	})
}

// TestRefusedByGate_Negatives proves the two negative properties RefusedByGate must hold: an
// ordinary, non-refusal error matches no Check at all,
// and a refusal attributable to one check does not also match either of the others.
// A helper proved only on its positive cases could still pass with an overly loose substring match,
// and these cases catch that.
func TestRefusedByGate_Negatives(t *testing.T) {
	tiergate.Require(t, 1)
	t.Run("OrdinaryErrorMatchesNoCheck", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		const slug = "ordinary-owner"
		hubforge.AddPair(t, h, slug)
		// A second Add against the same slug fails for an ordinary, non-refusal reason:
		// the worktree directory already exists.
		// It is a real error from a real verb, not a hand-written string standing in for one.
		_, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{})
		if err == nil {
			t.Fatalf("Add(%s) a second time: want an error, got nil", slug)
		}

		for _, check := range []fabricengine.Check{fabricengine.CheckContainment, fabricengine.CheckOwnership, fabricengine.CheckDirtiness} {
			if RefusedByGate(err, check) {
				t.Errorf("RefusedByGate(%v, %s) = true; want false for an ordinary non-refusal error", err, check)
			}
		}
	})

	t.Run("EachRefusalMatchesOnlyItsOwnCheck", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		containmentErr := driveContainmentGateRefusal(t, h, "cross-owner", "cross-escape-link")
		ownershipErr := driveOwnershipGateRefusal(t, h, "cross-own-owner")
		dirtinessErr := driveDirtinessGateRefusal(t, h, "cross-dirty-owner")

		cases := []struct {
			name string
			err  error
			want fabricengine.Check
		}{
			{"Containment", containmentErr, fabricengine.CheckContainment},
			{"Ownership", ownershipErr, fabricengine.CheckOwnership},
			{"Dirtiness", dirtinessErr, fabricengine.CheckDirtiness},
		}
		for _, tt := range cases {
			if tt.err == nil {
				t.Fatalf("%s: want a refusal, got nil", tt.name)
			}
			for _, check := range []fabricengine.Check{fabricengine.CheckContainment, fabricengine.CheckOwnership, fabricengine.CheckDirtiness} {
				got := RefusedByGate(tt.err, check)
				want := check == tt.want
				if got != want {
					t.Errorf("RefusedByGate(%s refusal, %s) = %v; want %v", tt.name, check, got, want)
				}
			}
		}
	})
}

// TestRefusedBefore proves RefusedBefore matches real pre-flight refusals.
// Those are verb-level guards that run before a request reaches the destructive gate.
// Its discriminating case proves the "check failed" exclusion.
// It does so against a gate refusal that shares the identical reason text.
func TestRefusedBefore(t *testing.T) {
	tiergate.Require(t, 1)
	t.Run("RemovesOwnDirtyMessage", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		const slug = "before-dirty-owner"
		hubforge.AddPair(t, h, slug)

		warpTarget := h.PairCodeWorktree(slug)
		scratch := filepath.Join(warpTarget, "scratch-dirty.txt")
		if err := os.WriteFile(scratch, []byte("dirty"), 0o644); err != nil {
			t.Fatalf("write %s: %v", scratch, err)
		}

		_, err := h.Topology.Remove(h.Location, slug, false, false)
		if err == nil {
			t.Fatalf("Remove(%s, force=false) against a dirty warp worktree: want an error, got nil", slug)
		}
		if !RefusedBefore(err, "worktree has uncommitted changes; use --force") {
			t.Errorf("RefusedBefore(%v, ...) = false; want true for Remove's own pre-flight dirty message", err)
		}
	})

	// The "check failed" exclusion documented on RefusedBefore is what separates the two:
	// the gate's dirtiness reason is byte-identical to Remove's pre-flight reason.
	// A substring match on the reason text alone cannot tell them apart.
	// Without the exclusion, the first assertion below would be true instead of false,
	// and the refusal half would pass for the wrong reason.
	t.Run("DiscriminatesFromGateDirtinessOnIdenticalReasonText", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		err := driveDirtinessGateRefusal(t, h, "before-gate-owner")
		if err == nil {
			t.Fatalf("driveDirtinessGateRefusal: want a refusal, got nil")
		}
		if RefusedBefore(err, "worktree has uncommitted changes; use --force") {
			t.Errorf("RefusedBefore(%v, ...) = true; want false for a GATE dirtiness refusal carrying the identical reason text", err)
		}
		if !RefusedByGate(err, fabricengine.CheckDirtiness) {
			t.Errorf("RefusedByGate(%v, CheckDirtiness) = false; want true for the same error", err)
		}
	})

	t.Run("RemovesSlugValidationRefusal", func(t *testing.T) {
		t.Parallel()

		h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
		_, err := h.Topology.Remove(h.Location, "..", false, false)
		if err == nil {
			t.Fatalf(`Remove(h.Location, "..", false): want an error, got nil`)
		}
		if !RefusedBefore(err, `invalid slug ".."`) {
			t.Errorf("RefusedBefore(%v, ...) = false; want true for Remove's own slug-validation refusal", err)
		}
	})

	t.Run("ResetHubPreflightRefusal", func(t *testing.T) {
		t.Parallel()

		cwd := t.TempDir()
		const warpURL = "https://example.invalid/reset-hub-preflight.git"
		name := fabricengine.DeriveWarpName(warpURL)
		hubPath := fabricengine.HubPath(cwd, name)
		if err := os.MkdirAll(hubPath, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", hubPath, err)
		}

		// A plain directory at the derived hub path has no Bolt checkout entry and no weft sibling.
		// checkResetTarget's looksLikeHub pre-flight refuses before CloneHub touches the network.
		_, err := fabricengine.CloneHub(cwd, fabricengine.CloneOptions{
			WeftURL: "https://example.invalid/reset-hub-preflight-weft.git",
			WarpURL: warpURL,
			Reset:   true,
		})
		if err == nil {
			t.Fatalf("CloneHub(Reset: true) against a non-hub directory: want an error, got nil")
		}
		if !RefusedBefore(err, "is not a fabric hub") {
			t.Errorf("RefusedBefore(%v, ...) = false; want true for the offline pre-flight refusal", err)
		}
	})
}
