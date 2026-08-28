// landingdeps.go declares landingDeps, the assembly seam that builds a landingshed.Deps struct from
// every value already resolved by drive.go. landingDeps performs no I/O of any kind -- it exists so
// the drift-guard test in landingdeps_test.go stays Tier 1 with no hubforge fixture, mirroring
// wiring.go's own header-comment convention for why wire is extracted.

package loomcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// landingDeps assembles a landingshed.Deps from values already resolved by the caller (drive.go),
// per the assembly-seam-takes-plain-values decision: every argument arrives already resolved, and
// this function does no I/O and returns no error.
//
// runner assigns directly into the Shuttle mergeresolve.Shuttle field: *shuttleengine.Runner already
// satisfies that interface, per the existing compile-time assertion at
// internal/mergeresolve/deps.go:46.
func landingDeps(
	l *lyxcwd.Location,
	geom websterengine.Geometry,
	taskBranch, originURL, parentBranch string,
	pushSkipped bool,
	pushBranch func() error,
	registry modelspec.Registry,
	runner *shuttleengine.Runner,
	cfg landingshed.Config,
) landingshed.Deps {
	return landingshed.Deps{
		WorktreeRoot:     l.WorktreePath(),
		TaskBranch:       taskBranch,
		ParentBranch:     parentBranch,
		FinalSummaryPath: summaryparser.Path(geom.WebsterDir),
		StencilsDir:      geom.StencilsDir,
		ScratchDir:       loomengine.LoomScratchDir(l),
		OriginURL:        originURL,
		PushSkipped:      pushSkipped,
		PushBranch:       pushBranch,
		OpenFabric: func() (*fabricengine.Fabric, error) {
			return fabricengine.Open(l)
		},
		OpenParentFabric: func() (*fabricengine.Fabric, error) {
			return fabricengine.OpenParent(l, parentBranch)
		},
		// CommitStatus commits loom's own phase-machine status file, which Shed rewrites on every
		// producer transition. The per-transition Shed.CommitStatus seam (wiring.go) already keeps
		// the status file current on the ordinary path, so both landing producers calling this
		// immediately before they merge is retained only as the sole protection if a product wires
		// Shed.CommitStatus as nil -- not because the merge guard still inspects the local-only side
		// of the pair, which this task removed from the merge participants entirely, so the guard no
		// longer looks at it.
		//
		// It mirrors wiring.go's CommitDiscussion/CommitPlan closures in every respect: the same
		// CommitAnchoredPaths call, the same throwaway mutation recorder, the same EnvSyncOptions,
		// and the same discard of the (sha, committed) pair in favour of the error alone, which is
		// what makes a second call over an already-committed, already-clean path a no-op rather
		// than a failure. The pathspec is loomengine.LoomStatusRel(), never a hand-built join
		// naming the _lyx literal, which the Lyxdirs Single-Declarer Invariant forbids.
		CommitStatus: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), l, []string{loomengine.LoomStatusRel()}, fmt.Sprintf("loom: status checkpoint for %s", seedSlug(l.WorktreeName)), fabricengine.EnvSyncOptions())
			return err
		},
		Shuttle:  runner,
		Registry: registry,
		Config:   cfg,
	}
}
