// pull.go — the unified Fabric.Pull entry point: weft ff-pull followed by
// warp fetch/inspect/reconcile, detecting a warp history rewrite (rebase or
// force-push) and, when it is safe to do so, re-anchoring weft's own
// correspondence to the new upstream tip via the existing empty-commit
// machinery. This file defines PullResult and *PartialPullError — the result
// and partial-failure contract batches 3-4 (the CLI and docs layers) consume
// — mirroring PartialCommitError's shape (commit.go), with the two sides'
// roles swapped to match Pull's weft-first ordering.

package fabricengine

import (
	"errors"
	"fmt"
)

// PullResult reports what Fabric.Pull actually did, on both sides
// independently, and — when a warp history rewrite forced a reconcile — the
// re-anchor baseline and the weft content a caller should treat as
// PATTERN-residue (potentially replayed against the wrong warp baseline).
type PullResult struct {
	// WeftPulled reports whether the weft ff-pull (PullWeft) ran and
	// succeeded. Every field below is only ever populated once this is true —
	// see Fabric.Pull's weft-first ordering.
	WeftPulled bool
	// WarpFetched reports whether the warp fetch (f.Warp.Fetch) ran and
	// succeeded.
	WarpFetched bool
	// WarpAdvanced reports whether the warp branch pointer actually moved,
	// either via a clean fast-forward reset or a reconcile reset.
	WarpAdvanced bool
	// NewWarpHEAD is the fetched upstream tip SHA warp advanced to. It is
	// empty when warp did not advance (already up to date, or an abort path).
	NewWarpHEAD string
	// RewriteDetected reports whether the warp pull was a non-fast-forward —
	// i.e. local warp HEAD is not an ancestor of the fetched upstream tip,
	// meaning upstream's history was rewritten (rebase/force-push), not
	// merely advanced.
	RewriteDetected bool
	// Reconciled reports whether a re-anchor weft commit was written to
	// re-bind weft's correspondence to the new warp HEAD after a rewrite.
	Reconciled bool
	// AnchorWarpSHA is the surviving correspondence entry's warp SHA — the
	// confirmed re-anchor baseline reachableAnchor resolved. Populated only
	// when Reconciled is true.
	AnchorWarpSHA string
	// AnchorWeftSHA is that same entry's weft SHA — the baseline the
	// PATTERN-residue range (PatternResidue) starts from. Populated only when
	// Reconciled is true.
	AnchorWeftSHA string
	// ReanchorWeftSHA is the new empty weft anchor commit's own SHA, bound to
	// NewWarpHEAD via its Warp-SHA trailer. Populated only when Reconciled is
	// true.
	ReanchorWeftSHA string
	// PatternResidue lists the post-anchor weft commits (between
	// AnchorWeftSHA and the weft HEAD at reconcile time) that touched
	// _pattern/... paths — content a caller should treat as potentially
	// stale against the new warp baseline. Populated only when Reconciled is
	// true.
	PatternResidue []PatternResidueEntry
}

// PatternResidueEntry names one post-anchor weft commit and the _pattern/...
// paths it touched, as enumerated by Fabric.Pull's reconcile branch (see
// patternResidueCommits).
type PatternResidueEntry struct {
	WeftSHA string
	Paths   []string
}

// PartialPullError reports a Fabric.Pull call whose weft side completed
// cleanly but whose warp-side work did not — mirroring PartialCommitError's
// shape (commit.go) with the two sides' roles swapped, per the
// weft-first-ordering / report-not-rollback Shared Decision. WeftPulled is
// always true for this type: a weft-side failure never produces a
// *PartialPullError at all, since Fabric.Pull returns immediately on that
// path (see Fabric.Pull's doc comment). Stage names which warp-side step
// failed (e.g. "fetch", "reset", "reanchor"), so a caller (or an operator
// reading the error) knows exactly where the call stopped without
// re-deriving it from Err's message.
type PartialPullError struct {
	WeftPulled bool
	Stage      string
	Err        error
}

// Error implements the error interface, stating that weft succeeded and
// naming the warp-side stage that failed.
func (e *PartialPullError) Error() string {
	return fmt.Sprintf("fabricengine: weft pull succeeded, warp %s failed: %v", e.Stage, e.Err)
}

// Unwrap returns the wrapped error, so errors.Is/errors.As reach it.
func (e *PartialPullError) Unwrap() error {
	return e.Err
}

// ErrWarpDivergedUnpushed is returned by Fabric.Pull when the warp remote's
// history has been rewritten AND local warp already carries unpushed
// commits of its own — the double-conflict case Fabric.Pull refuses to
// reconcile automatically, since resolving it would require deciding what
// happens to the caller's own unpushed work. Fabric.Pull makes no change to
// either repo when this is returned.
var ErrWarpDivergedUnpushed = errors.New("fabricengine: warp remote diverged and local warp has unpushed commits; aborting, no changes")

// ErrNoSurvivingAnchor is returned by Fabric.Pull when the warp remote's
// history has been rewritten so thoroughly that no entry in the
// correspondence index survives — reachableAnchor found nothing reachable
// from the new upstream tip — leaving no safe baseline to re-anchor weft
// against. Fabric.Pull makes no change to either repo when this is returned.
var ErrNoSurvivingAnchor = errors.New("fabricengine: warp history rewritten and no recorded correspondence survives; aborting, no changes")
