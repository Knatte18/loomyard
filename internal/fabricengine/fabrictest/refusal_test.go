//go:build integration

// refusal_test.go proves RefusedByGate and RefusedBefore pin the correct layer against real errors
// driven from real verbs against real hubs — one gate refusal per reachable Check member, the two
// negative properties that make the matching trustworthy rather than merely permissive, and
// RefusedBefore's own real pre-flight cases, including the discriminating pair the "check failed"
// exclusion exists for.

package fabrictest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fslink"
)

// driveContainmentGateRefusal drives a real containment refusal through UnwireJunctions: a junction
// name of "../<escapeName>" cancels the slug segment and lands the "link" at the hub itself, escaping
// the worktree UnwireJunctions was asked to operate on. This is the same reach point
// TestUnwireJunctions_RefusesLinkOutsideItsWorktree (destructivegaps_integration_test.go) proves live:
// junction.go:368's UnwireJunctions, through unseedLyxJunction, into unseedJunctionRecords's
// pathRequest and removeLink call at junction.go:474-483.
func driveContainmentGateRefusal(t testing.TB, h *Hub, slug, escapeName string) error {
	t.Helper()

	escapeLink := filepath.Join(h.Path, escapeName)
	elsewhere := t.TempDir()
	if err := fslink.CreateDirLink(escapeLink, elsewhere); err != nil {
		t.Fatalf("create escape link %s: %v", escapeLink, err)
	}

	_, err := fabricengine.UnwireJunctions(h.Location, slug, []string{"../" + escapeName})
	return err
}

// driveOwnershipGateRefusal drives a real ownership refusal through UnwireJunctions, using the one
// reach point in this batch's Context files where the gate's own ownership predicate genuinely
// diverges from a caller's own pre-flight rather than merely re-running it.
//
// Topology.Prune(apply=true) against a hub child ending in the weft suffix that fabric never created
// was tried first, matching this batch's own instructions, and confirmed unreachable by running it:
// Prune's applyStalePairOwnership pre-check calls the exact same isRegisteredLinkedWorktreeIn
// predicate, on the exact same (repoDir, target) pair, that the gate's own
// ownedRegisteredLinkedWorktree check resolves inside removeStalePair — deliberately so, per
// isRegisteredLinkedWorktreeIn's own doc comment ("shared rather than duplicated because both sides
// of the pair need the same rule"). The two calls can never disagree, so the gate's own pipeline is
// never reached for an unregistered child; the resulting PruneEntry.Error is entirely
// applyStalePairOwnership's own hand-written message, which never contains "check failed". The
// identical duplication blocks Remove's removeWarpWorktreeDir for the same reason on its primary
// gate call, and its fallback call is only reached once the same registration check has already
// passed, so ownership cannot fail there either.
//
// A real divergence does exist for ownedWiredJunction (destroy.go:376-397), though.
// unseedJunctionRecords's own pre-check resolves the link with fslink.PointsTo — fully resolved,
// walking every hop — while the gate's own resolvePathOwnership case for pathOwnershipWiredJunction
// reads fslink.RawTarget: the ONE HOP a wiring call actually recorded (see RawTarget's own doc
// comment for why). Chaining the junction through an intermediate symlink (link -> intermediate ->
// target) makes PointsTo resolve correctly — so the pre-check passes and removeLink is attempted —
// while RawTarget reads "intermediate", which does not match the gate's expected one-hop target: a
// genuine gate-only refusal, not a proxy for a pre-flight duplicate.
func driveOwnershipGateRefusal(t testing.TB, h *Hub, slug string) error {
	t.Helper()

	AddPair(t, h, slug)

	names, err := fabricengine.WiredNames(h.BoardDir())
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

// driveDirtinessGateRefusal drives a real dirtiness refusal through the gate's own dirtyScopeAll
// request inside removeWarpWorktreeDir (remove.go:196 the primary request, remove.go:230 the
// fallback), reached via Remove.
//
// force is passed as true so Remove's own earlier pre-flight (remove.go:68-76 — the identical
// worktreeDirty(scopeAll, target) check on the identical target) is skipped entirely rather than
// claiming the planted dirt first. force=true also makes the primary gate call's own dirtiness check
// pass immediately (force satisfies dirtiness, never containment or ownership) and run `git worktree
// remove --force`, which git itself still refuses on a LOCKED worktree even with --force. That
// failure sends removeWarpWorktreeDir to its fallback request, which carries no force field at all
// (defaulting to false) — so the fallback's own dirtiness check finally sees the tracked dirt that
// survived because the locked git call never actually removed anything.
func driveDirtinessGateRefusal(t testing.TB, h *Hub, slug string) error {
	t.Helper()

	AddPair(t, h, slug)

	warpTarget := h.PairWarpWorktree(slug)
	trackedFile := filepath.Join(warpTarget, "dirt.txt")
	if err := os.WriteFile(trackedFile, []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", trackedFile, err)
	}
	mustGit(warpTarget, "add", "dirt.txt")
	mustGit(warpTarget, "commit", "-m", "fabrictest: seed tracked file for dirtiness refusal")
	if err := os.WriteFile(trackedFile, []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("modify %s: %v", trackedFile, err)
	}
	mustGit(warpTarget, "worktree", "lock", warpTarget)

	_, err := h.Topology.Remove(h.Location, slug, true)
	return err
}

// TestRefusedByGate proves RefusedByGate matches a real gate refusal for each of the three reachable
// Check members, one driven scenario per member.
func TestRefusedByGate(t *testing.T) {
	t.Run("Containment", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")
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

		h := NewHub(t, ".")
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

		h := NewHub(t, ".")
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
// ordinary, non-refusal error matches no Check at all, and a refusal genuinely attributable to one
// check does not also match either of the other two.
// A helper proved only on its positive cases could still pass with an overly loose substring match —
// these are the cases that would catch that.
func TestRefusedByGate_Negatives(t *testing.T) {
	t.Run("OrdinaryErrorMatchesNoCheck", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")
		const slug = "ordinary-owner"
		AddPair(t, h, slug)
		// A second Add against the same slug fails for an entirely ordinary, non-refusal reason
		// (add.go:71's plain fmt.Errorf: the worktree directory already exists) — a real error from
		// a real verb, not a hand-written string standing in for one.
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

		h := NewHub(t, ".")
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

// TestRefusedBefore proves RefusedBefore matches real pre-flight refusals — verb-level guards that
// run before a request ever reaches the destructive gate — and, in its discriminating case, proves
// the mandatory "check failed" exclusion actually earns its keep against a gate refusal sharing the
// identical reason text.
func TestRefusedBefore(t *testing.T) {
	t.Run("RemovesOwnDirtyMessage", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")
		const slug = "before-dirty-owner"
		AddPair(t, h, slug)

		warpTarget := h.PairWarpWorktree(slug)
		scratch := filepath.Join(warpTarget, "scratch-dirty.txt")
		if err := os.WriteFile(scratch, []byte("dirty"), 0o644); err != nil {
			t.Fatalf("write %s: %v", scratch, err)
		}

		_, err := h.Topology.Remove(h.Location, slug, false)
		if err == nil {
			t.Fatalf("Remove(%s, force=false) against a dirty warp worktree: want an error, got nil", slug)
		}
		if !RefusedBefore(err, "worktree has uncommitted changes; use --force") {
			t.Errorf("RefusedBefore(%v, ...) = false; want true for Remove's own pre-flight dirty message", err)
		}
	})

	// DiscriminatesFromGateDirtinessOnIdenticalReasonText is the whole point of the "check failed"
	// exclusion documented on RefusedBefore itself: the gate's own dirtiness reason (destroy.go:564)
	// is byte-identical to Remove's own pre-flight reason (remove.go:74), so a naive substring match
	// on the reason text alone cannot tell the two apart. Without the exclusion this sub-test's first
	// assertion would be true instead of false, and sabotage row 3 in batch 8 would stay green on its
	// refusal half for the wrong reason.
	t.Run("DiscriminatesFromGateDirtinessOnIdenticalReasonText", func(t *testing.T) {
		t.Parallel()

		h := NewHub(t, ".")
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

		h := NewHub(t, ".")
		_, err := h.Topology.Remove(h.Location, "..", false)
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

		// A plain directory at the derived hub path has no _board entry and no weft sibling, so
		// resetHub's own looksLikeHub pre-flight (clone.go:573-577) refuses before CloneHub ever
		// touches the network.
		_, err := fabricengine.CloneHub(cwd, fabricengine.CloneOptions{
			WeftURL: "https://example.invalid/reset-hub-preflight-weft.git",
			WarpURL: warpURL,
			Reset:   true,
		})
		if err == nil {
			t.Fatalf("CloneHub(Reset: true) against a non-hub directory: want an error, got nil")
		}
		if !RefusedBefore(err, "is not a fabric hub") {
			t.Errorf("RefusedBefore(%v, ...) = false; want true for resetHub's own pre-flight refusal", err)
		}
	})
}
