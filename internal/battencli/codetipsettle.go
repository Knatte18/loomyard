// codetipsettle.go settles a missing code tip for a run this machine owns, once per dobby read of the run.
// It pushes a tip the local code branch holds.
// It gives up a tip no machine holds after the grace.
// It brings a code branch that origin moved past level.

package battencli

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/runfsm"
)

// codeTipSettling keeps one failure episode per run whose missing code tip cannot be settled on this machine yet.
// It is keyed by slug, so a persistent failure logs one Warn, not one line per dobby tick.
var codeTipSettling logger.EpisodeSet

// codeTipDeps holds what settleCodeTip reads and does, so a tier0 test drives every branch with fakes.
type codeTipDeps struct {
	// Read reads the code tip the run's last transition on origin names, from the refs the last fetch left.
	Read func(slug string) (fabricengine.CodeTipReading, error)
	// Fetch fetches the run's code and records branches from origin.
	Fetch func(slug string) error
	// HoldsTip reports whether the run's local code branch holds tip.
	HoldsTip func(slug, tip string) (bool, error)
	// PushTip pushes the run's pair, records first and then its code branch, which lands a tip the branch holds.
	PushTip func(slug string) error
	// GiveUp returns the run to its previous transition and pushes that, reporting whether it returned.
	// It returns true with an error when the return was committed and its push failed.
	GiveUp func(slug string) (returned bool, err error)
	// FastForward brings the run's local code branch level with origin's.
	FastForward func(slug string) error
}

// hubCodeTipDeps returns the deps over the pairs of the hub of prime.
func hubCodeTipDeps(prime *lymcwd.Location) codeTipDeps {
	pairOf := func(slug string) (*lymcwd.Location, error) {
		return lymcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))
	}
	push := pairteardown.RunPush{LockWait: fabricengine.StatusPushLockWait, TeardownWait: fabricengine.StatusPushLockWait}
	return codeTipDeps{
		Read: func(slug string) (fabricengine.CodeTipReading, error) { return fabricengine.OriginCodeTip(prime, slug) },
		Fetch: func(slug string) error {
			pair, err := pairOf(slug)
			if err != nil {
				return err
			}
			return fabricengine.FetchPairTips(pair)
		},
		HoldsTip: func(slug, tip string) (bool, error) {
			pair, err := pairOf(slug)
			if err != nil {
				return false, err
			}
			return fabricengine.HeadContains(pair.WorktreePath(), tip)
		},
		PushTip: func(slug string) error {
			pair, err := pairOf(slug)
			if err != nil {
				return err
			}
			_, err = pairteardown.PushRun(prime, pair, slug, push)
			return err
		},
		GiveUp: func(slug string) (bool, error) {
			pair, err := pairOf(slug)
			if err != nil {
				return false, err
			}
			returned, err := fabricengine.ReturnToPreviousTransition(pair, slug)
			if err != nil || !returned {
				return returned, err
			}
			_, err = pairteardown.PushRun(prime, pair, slug, push)
			return true, err
		},
		FastForward: func(slug string) error {
			pair, err := pairOf(slug)
			if err != nil {
				return err
			}
			fabric, err := fabricengine.Open(pair)
			if err != nil {
				return err
			}
			_, err = fabric.FastForwardPair(fabricengine.SyncOptions{})
			return err
		},
	}
}

// settleCodeTip acts on the code tip of the run slug, which this machine owns, once its verdict is read.
// It acts only for a code_tip verdict, or for a run whose code branch origin's fetched branch moved past.
//
// A code_tip verdict fetches the code and records branches first and reads the tip again.
// A tip origin now holds is not missing: the local code branch is brought level and nothing is given up.
// A tip still missing that the local code branch holds is pushed, inside the grace or past it, and never given up.
// A tip still missing that the local code branch lacks waits while the transition is younger than runfsm.CodeTipGrace.
// Past the grace the run returns to its previous transition.
// A run with one transition logs the machine that owes the tip.
// A failed push or give-up is logged and leaves the run for the next tick.
//
// Any other verdict is acted on only when origin's code branch holds the tip and the local branch lacks it.
// The local branch is then brought level before the run's next pass.
//
// It returns the error of a failed read, fetch or fast-forward.
func settleCodeTip(deps codeTipDeps, slug string, verdict runfsm.Verdict, now time.Time) error {
	reading, err := deps.Read(slug)
	if err != nil {
		return fmt.Errorf("read the code tip of %s: %w", slug, err)
	}
	waiting := verdict.Reason == runfsm.ReasonCodeTip
	if waiting {
		if err := deps.Fetch(slug); err != nil {
			return fmt.Errorf("fetch the tips of %s: %w", slug, err)
		}
		if reading, err = deps.Read(slug); err != nil {
			return fmt.Errorf("read the code tip of %s after the fetch: %w", slug, err)
		}
	}
	if reading.Tip == "" {
		return nil
	}

	held, err := deps.HoldsTip(slug, reading.Tip)
	if err != nil {
		return fmt.Errorf("read whether the code branch of %s holds %s: %w", slug, reading.Tip, err)
	}
	if !reading.Missing {
		codeTipSettling.Recover(slug, "battencli: the code tip of a run is on origin again", "slug", slug)
		if held {
			return nil
		}
		if err := deps.FastForward(slug); err != nil {
			return fmt.Errorf("bring the code branch of %s level with origin: %w", slug, err)
		}
		return nil
	}
	if !waiting {
		return nil
	}

	pastGrace := now.Sub(reading.At) >= runfsm.CodeTipGrace
	if held {
		if err := deps.PushTip(slug); err != nil {
			if pastGrace {
				codeTipSettling.Fail(slug, "battencli: a code tip this machine holds is past its grace and still not on origin; run `lym fabric push` in this machine's task worktree",
					"slug", slug, "tip", reading.Tip, "error", err)
			} else {
				logger.Debug("battencli: the push of a held code tip failed and is tried again next tick", "slug", slug, "tip", reading.Tip, "error", err)
			}
		}
		return nil
	}
	if !pastGrace {
		return nil
	}
	returned, err := deps.GiveUp(slug)
	switch {
	case err != nil:
		codeTipSettling.Fail(slug, "battencli: the give-up of a missing code tip failed and is tried again next tick", "slug", slug, "tip", reading.Tip, "error", err)
	case !returned:
		codeTipSettling.Fail(slug, "battencli: a missing code tip past its grace cannot be given up, since its run has one transition; run `lym fabric push` in the task worktree on machine "+reading.Machine,
			"slug", slug, "tip", reading.Tip, "machine", reading.Machine)
	}
	return nil
}
