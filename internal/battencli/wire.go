// wire.go implements wire, the assembly seam that builds the shedrecipe.Env and
// shedbuild.ShedPaths the run and status verbs need, plus the receiver field the abandoned
// session value is recorded into.
//
// Every seam that touches the managed task worktree resolves inside its own closure body on Call,
// never here at wiring time: the task worktree does not exist until WorktreeCreate has already run,
// so resolving any of its paths any earlier would resolve a path that is not there yet. That applies
// to the status-path pair, the spawn directory, and both teardown halves alike -- not only to the one
// whose laziness (ResolveStatus's own signature) makes it visible.

package battencli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrecipe"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// taskWorktreeLocation resolves the managed task worktree's own *lyxcwd.Location, for slug, from
// the prime *lyxcwd.Location. It is the shared body every lazily-resolved seam below calls, so a
// caller reading this file only once still sees every "resolved lazily" claim in one place.
//
// It joins the task worktree's root via fabricengine.WorktreePath -- the topology package's own
// sibling-path helper -- and resolves that root through lyxcwd.ResolveWorktree, which applies no
// cwd gate: the caller here holds a worktree root, not an acting cwd, so the gate would spuriously
// fire.
//
// An absent pair is refused by name rather than left to the resolver's generic "not a git
// repository": batten's status is durable, so a run resumed on another machine reaches every row
// past Worktree-Create with no pair here.
// Recreating it is not attempted, since fabric's Add refuses a pre-existing branch by design -- and
// the refusal below names no fabric command as a substitute: "lyx fabric checkout" switches the
// CALLER's own worktree onto the named branch, so run from prime, as every batten verb must be, it
// would mutate prime itself rather than restore anything.
// Deleting the pair's branches alone rewinds nothing: the resumed run re-enters its persisted row,
// not Worktree-Create, so the abandon path names batten's own run directory as well, and both
// branch copies, since the ones on the remote make a fresh create's push refuse.
func taskWorktreeLocation(prime *lyxcwd.Location, slug string) (*lyxcwd.Location, error) {
	worktreePath := fabricengine.WorktreePath(prime, slug)
	if _, err := os.Stat(worktreePath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"battencli: the task worktree for %q is not present at %s; this run's durable status says it was already created, so it is either on another machine or was removed by hand -- batten does not recreate a pair from its branch, and no \"lyx fabric\" command currently does either (creating one refuses when its branch already exists). Resolve it by hand, one of two ways: restore the worktree pair yourself from its branches, outside lyx's own automation, which keeps the task's work, then resume this run; or abandon this run by deleting its run directory %s (a change on the pair's fabric sibling) and the pair's branches, local and remote, after which \"lyx batten run %s\" starts over from a fresh create -- discarding any of the task's work not already merged",
				slug, worktreePath, shedrun.RunDir(prime, slug), slug,
			)
		}
		return nil, err
	}
	return lyxcwd.ResolveWorktree(worktreePath)
}

// taskWorktreePresent reports whether the task worktree for slug is already on disk under prime's
// hub and resolvable as a worktree of its own.
//
// Worktree-Teardown's two halves use this for their own post-condition, "the task worktree is gone"
// (see the Teardown field below): a bare directory check is the right question there, since fabric
// removes the task worktree before its sibling and the mirror-image state (task worktree gone,
// sibling remaining) is PairSiblingRemnant's own question to answer, not this one's.
//
// Worktree-Create uses the stronger taskWorktreeComplete instead, not this function: a bare
// directory check cannot tell a pair Add finished from one a SIGKILL interrupted partway through
// (see taskWorktreeComplete's own doc comment).
//
// It answers false rather than an error when the worktree is absent, so a genuinely absent one
// still reaches Topology.Add or Topology.Remove and every real failure keeps its Stuck disposition.
// A stat error that is not "absent", and a path that is there but does not resolve as a worktree,
// are both returned: neither is an answer to the question asked.
func taskWorktreePresent(prime *lyxcwd.Location, slug string) (bool, error) {
	worktreePath := fabricengine.WorktreePath(prime, slug)
	if _, err := os.Stat(worktreePath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if _, err := lyxcwd.ResolveWorktree(worktreePath); err != nil {
		return false, err
	}
	return true, nil
}

// taskWorktreeComplete reports whether the task worktree for slug is not just present but fully
// materialised: Topology.Add's own full post-condition (fabricengine.PairComplete), which
// taskWorktreePresent's bare directory check cannot tell from a pair Add left half-built.
//
// Worktree-Create's post-condition is "the task worktree for this slug exists", so a row re-entered
// against an already-satisfied post-condition must report success rather than asking fabric to
// create the same worktree twice. The window is wider than "Add returned but the transition was not
// yet persisted": Add is a multi-step transaction whose own in-process rollback runs only when Add
// itself observes an error, never when the process running it is killed instead -- a bare "the
// worktree directory resolves" check is satisfied by that transaction's very first step alone.
//
// present is false only when the worktree does not exist at all -- the genuinely-absent case, which
// still reaches Topology.Add. incompleteReason is non-empty only when the worktree exists but is not
// yet a complete pair, worded for this package's own refusal text rather than repeating
// PairComplete's fabric-vocabulary reason verbatim.
func taskWorktreeComplete(prime *lyxcwd.Location, slug string) (present, complete bool, incompleteReason string, err error) {
	present, err = taskWorktreePresent(prime, slug)
	if err != nil || !present {
		return present, false, "", err
	}
	taskLocation, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))
	if err != nil {
		return present, false, "", err
	}
	complete, _, err = fabricengine.PairComplete(taskLocation)
	if err != nil {
		return present, false, "", err
	}
	if !complete {
		return present, false, "its pair is not fully created -- most likely a create interrupted partway through, since the process that ran it was killed rather than erroring, so its own rollback never ran", nil
	}
	return present, true, "", nil
}

// incompletePairRemedy composes the manual-cleanup remedy taskWorktreeComplete's own incomplete
// case names: the task worktree itself, its pair's other-side leftover when SIGKILL landed late
// enough in Add for one to exist, and the branch Add created -- in that order, since removing the
// worktrees first is what lets the branch deletion below ever succeed (git refuses to delete a
// branch still checked out).
func incompletePairRemedy(location *lyxcwd.Location, slug string) (string, error) {
	target := fabricengine.WorktreePath(location, slug)
	remedy := fmt.Sprintf("\"git worktree remove --force %s\"", target)
	remnant, remnantPresent, err := fabricengine.PairSiblingRemnant(location, slug)
	if err != nil {
		return "", err
	}
	if remnantPresent {
		remedy += fmt.Sprintf(" and \"git worktree remove --force %s\"", remnant)
	}
	return fmt.Sprintf("remove it by hand (%s) and its branch (\"git branch -D %s\")", remedy, slug), nil
}

// createRefusal rewords the one create refusal whose fabric remedy is wrong from prime, and passes
// every other create error through verbatim (fabric's own refusals otherwise name remedies that hold
// from here, and rewording them would only drop information).
//
// fabric words its leftover-branch refusal for a caller inside a pair: "switch a pair onto it with
// lyx fabric checkout". Every batten verb runs from prime, where that command switches prime's own
// pair onto the task's branches. The branch is a leftover in the everyday flow -- a torn-down pair
// keeps its branch and both remote copies, and a rolled-back create keeps the branch it made -- so
// the remedy named here is the abandon path the absent-worktree refusal already names: delete the
// leftover locally and on the remote, both sides of the pair, then resume.
func createRefusal(err error) error {
	var branchExists *fabricengine.ErrBranchExists
	if !errors.As(err, &branchExists) {
		return err
	}
	return fmt.Errorf(
		"branch %q already exists, left behind by an earlier pair for this slug (a torn-down pair keeps its branch and both remote copies, and a rolled-back create keeps the branch it made); delete it locally (\"git branch -D %s\") and on the remote (\"git push origin --delete %s\"), remove the pair's leftover sibling branch the same way (\"lyx fabric cleanup --apply --remote\" removes an orphaned one), then resume this run -- never \"lyx fabric checkout\" from here, which would switch this worktree itself onto that branch",
		branchExists.Branch, branchExists.Branch, branchExists.Branch,
	)
}

// maxChildOutputInError caps the child output folded into a spawn error, so a misbehaving child
// cannot push an unbounded string into a status file committed onto prime's own pair.
const maxChildOutputInError = 2000

// childSpawnError folds a failed child bootstrap's own output into runErr, so the returned error
// carries the child's diagnosis rather than only its exit status.
// It returns nil for a nil runErr, runErr unchanged when the child said nothing, and truncates
// output past maxChildOutputInError with an explicit marker.
//
// The byte slice lands on a rune boundary only by chance, so strings.ToValidUTF8 drops any partial
// rune it leaves dangling at the cut point rather than embedding invalid UTF-8 into the error text.
func childSpawnError(runErr error, childOutput string) error {
	if runErr == nil {
		return nil
	}
	trimmed := strings.TrimSpace(childOutput)
	if trimmed == "" {
		return runErr
	}
	if len(trimmed) > maxChildOutputInError {
		trimmed = strings.ToValidUTF8(trimmed[:maxChildOutputInError], "") + " ... (truncated)"
	}
	return fmt.Errorf("%w: %s", runErr, trimmed)
}

// childSeedParams returns the seed params recipe's own bootstrap verb will itself write, read from
// childLocation.
//
// shedrun.WriteSeed is idempotent only against a seed agreeing on recipe, driver and params, so a
// child seeded without a param its own bootstrap writes makes that bootstrap refuse.
// Params are per-recipe: only loom declares one, params.parent, taken from the pair's recorded
// origin.
// An absent or empty recorded parent yields no param, leaving loom's own "pass --parent once"
// refusal as the one an operator sees.
func childSeedParams(recipe string, childLocation *lyxcwd.Location) (map[string]string, error) {
	if recipe != shedrun.RecipeLoom {
		return nil, nil
	}
	origin, found, err := fabricengine.ReadOrigin(childLocation)
	if err != nil {
		return nil, err
	}
	if !found || origin.ParentBranch == "" {
		return nil, nil
	}
	return map[string]string{"parent": origin.ParentBranch}, nil
}

// wire builds and stores the shedrecipe.Env and shedbuild.ShedPaths the run and status verbs
// need, over the resolved prime location and slug.
func (c *battenCLI) wire(location *lyxcwd.Location, slug string) error {
	primeRunLockPath := PrimeRunLock(location)
	primeLock := battenshed.PrimeLock{
		Path: primeRunLockPath,
		Acquire: func() (release func() error, ok bool, err error) {
			// The prime lock's own directory first -- it is the one being locked in -- then the
			// per-slug scratch directory, where the producers holding this lock write their
			// stuck-reason files. The two shedrun constructors state no relationship, so neither
			// may be left to the other's MkdirAll.
			if err := os.MkdirAll(filepath.Dir(primeRunLockPath), 0o755); err != nil {
				return nil, false, err
			}
			if err := os.MkdirAll(shedrun.ScratchDir(location, slug), 0o755); err != nil {
				return nil, false, err
			}
			fl, acquired, err := lock.TryAcquireWriteLock(primeRunLockPath)
			if err != nil {
				return nil, false, err
			}
			if !acquired {
				return nil, false, nil
			}
			return fl.Release, true, nil
		},
	}

	env := shedrecipe.Env{
		Slug:       slug,
		ScratchDir: BattenDir(location, slug),
		PrimeLock:  primeLock,
		CreateWorktree: func(ctx context.Context) error {
			// The already-complete probe first, so the row is idempotent against its own
			// post-condition -- see taskWorktreeComplete for the crash window this closes.
			present, complete, incompleteReason, err := taskWorktreeComplete(location, slug)
			if err != nil {
				return err
			}
			if complete {
				logger.Info("battencli: create worktree skipped, the task worktree is already present", "slug", slug)
				return nil
			}
			if present {
				// A worktree exists but Add never finished it -- see taskWorktreeComplete's own doc
				// comment. No fabric verb re-creates a pair in this exact state (Add refuses an
				// existing worktree directory the same way it refuses a leftover branch), so the
				// remedy is the same shape as the leftover-branch one: clean up the incomplete
				// worktree by hand, then resume. The remedy names the pair's other-side leftover too
				// when SIGKILL landed late enough in Add for one to exist -- removing the task
				// worktree alone would otherwise leave Add's own "directory already exists" refusal
				// on that other side as the very next obstacle.
				remedy, remedyErr := incompletePairRemedy(location, slug)
				if remedyErr != nil {
					return remedyErr
				}
				return fmt.Errorf(
					"the task worktree for %q exists but %s; %s, then resume this run to create it fresh",
					slug, incompleteReason, remedy,
				)
			}

			cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(location.HubPath))
			if err != nil {
				return err
			}
			top := fabricengine.NewTopology(cfg)
			res, err := top.Add(location, slug, fabricengine.AddOptions{})
			logger.Info("battencli: create worktree", "slug", slug, "mutations", res.Mutated())
			return createRefusal(err)
		},
		// Both teardown halves are idempotent against their shared post-condition, "the pair is
		// gone", mirroring CreateWorktree's already-present probe: shedengine persists the row's
		// transition only after the producer returns, so a process killed right after Remove
		// succeeded re-enters this row with no pair on disk. Without the probe Shutdown's own
		// location resolution refuses the absence and the run halts at teardown for good.
		// The post-condition is the pair's, not the task worktree's alone: fabric removes the task
		// worktree before its sibling, so a removal interrupted between the two -- or one whose
		// sibling half failed, which fabric reports and this row records as Stuck -- leaves the
		// sibling, the portal and launcher entries, and both branches behind with the task worktree
		// gone. Remove refuses that state by name rather than reporting done over it; Shutdown
		// still skips it, since reed's config is resolved through the task worktree that is gone,
		// and the per-hub watchdog reaps a session whose worktree has vanished.
		Teardown: battenshed.TeardownDeps{
			Shutdown: func(ctx context.Context) (abandonedSession string, err error) {
				present, err := taskWorktreePresent(location, slug)
				if err != nil {
					return "", err
				}
				if !present {
					logger.Info("battencli: session shutdown skipped, the task worktree is already gone", "slug", slug)
					return "", nil
				}
				taskLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return "", err
				}
				reedCfg, err := reedengine.LoadConfig(taskLocation.AnchorPath(), "reed")
				if err != nil {
					return "", err
				}
				reedGeom := hubgeom.ReedGeometry(taskLocation)
				reedEngine := reedengine.New(reedCfg, reedGeom)
				res, err := reedEngine.Down()
				if err != nil {
					return "", err
				}
				// Recorded onto the receiver, not only returned: the run verb reads it back after
				// the Shed's own Run has returned, to surface it on the success envelope.
				c.abandonedSession = res.AbandonedSession
				return res.AbandonedSession, nil
			},
			Remove: func(ctx context.Context) error {
				present, err := taskWorktreePresent(location, slug)
				if err != nil {
					return err
				}
				if !present {
					remnant, remnantPresent, err := fabricengine.PairSiblingRemnant(location, slug)
					if err != nil {
						return err
					}
					if remnantPresent {
						return fmt.Errorf(
							"the task worktree for %q is gone but its pair's fabric sibling is still on disk at %s -- a removal interrupted between its two halves; run \"lyx fabric prune --apply\" from here to remove the sibling with its portal and launcher entries, then resume this run",
							slug, remnant,
						)
					}
					logger.Info("battencli: teardown worktree skipped, the pair is already gone", "slug", slug)
					return nil
				}
				cfg, err := fabricengine.LoadConfig(fabricengine.BoardDir(location.HubPath))
				if err != nil {
					return err
				}
				top := fabricengine.NewTopology(cfg)
				// remote: true -- a batten-driven teardown is the task's own final removal, never a
				// step toward re-adopting the pair, so nothing will ever need the pair's other-side
				// branch again. Topology.Remove always deletes that branch locally regardless of this
				// flag; without it, the remote copy lingers forever, invisible to "lyx fabric cleanup"
				// (its own enumeration is local-branches-only), which is exactly what createRefusal's
				// and doneSlugRefusal's own remedy text both point an operator at.
				res, err := top.Remove(location, slug, false, true)
				logger.Info("battencli: teardown worktree", "slug", slug, "mutations", res.Mutated())
				return err
			},
		},
		InnerRun: battenshed.InnerRunDeps{
			// ResolveStatus also creates the child's ephemeral status-lock directory, since its
			// caller reads through that lock next and nothing else on the Run-Shed path creates
			// it: the child's status file is durable while its lock is not. This mirrors
			// battenPreRun's own MkdirAll for prime's status lock (arm.go).
			ResolveStatus: func() (statusPath, statusLockPath string, err error) {
				taskLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return "", "", err
				}
				statusPath = shedrun.StatusFile(taskLocation, shedrun.SelfRunID)
				statusLockPath = shedrun.StatusLock(taskLocation, shedrun.SelfRunID)
				if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
					return "", "", err
				}
				return statusPath, statusLockPath, nil
			},
			Spawn: func(ctx context.Context) error {
				taskLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return err
				}
				exe, err := os.Executable()
				if err != nil {
					return err
				}
				// loom's bootstrap verb, not a per-recipe dispatch: the WriteSeed seam below admits
				// no other child recipe, so the two stay consistent by construction.
				// Dir is the task worktree's AnchorPath(), never its bare worktree root: the resolver
				// gates a child's working directory to the anchor, and a bare root fails on any
				// subpath-anchored hub.
				cmd := exec.Command(exe, "loom", "start", "--no-attach")
				cmd.Dir = taskLocation.AnchorPath()
				// Captured rather than discarded: the child's own envelope is the only
				// diagnosis this seam can offer for a failed bootstrap.
				var childOutput bytes.Buffer
				cmd.Stdout = &childOutput
				cmd.Stderr = &childOutput
				logger.Info("battencli: spawning loom session", "slug", slug, "dir", cmd.Dir)
				runErr := cmd.Run()
				logger.Info("battencli: loom session wait complete", "slug", slug, "dir", cmd.Dir)
				return childSpawnError(runErr, childOutput.String())
			},
			ReadStatus: func(statusPath, statusLockPath string) (shedengine.Status, bool, error) {
				return state.ReadJSONStrict[shedengine.Status](statusPath, statusLockPath)
			},
		},
		SeedChild: battenshed.SeedChildDeps{
			// ReadBoardType opens the Board fresh on every Call, over fabricengine.BoardDir(
			// location.HubPath), and returns the task's own Type field -- never a value captured at
			// wiring time, so a type corrected after prime was seeded is still honoured.
			ReadBoardType: func(ctx context.Context) (string, error) {
				b := boardengine.New(boardengine.Config{Path: fabricengine.BoardDir(location.HubPath)})
				task, found, err := b.GetTask(slug)
				if err != nil {
					return "", err
				}
				if !found {
					return "", fmt.Errorf("battencli: board task %q not found", slug)
				}
				return task.Type, nil
			},
			// ChildDriver reads the "child_driver" param from prime's own seed -- the seed
			// card 24's auto-seed writes at this run-id -- defaulting to shedrun.DriverGo when the
			// seed is absent or the param is absent or empty. The defaulting itself is childDriverOf
			// (arm.go), shared with refuseAdoptedSeed's own comparison so the two can never read the
			// same seed's child driver differently.
			ChildDriver: func() (string, error) {
				seed, found, err := shedrun.ReadSeed(location, slug)
				if err != nil {
					return "", err
				}
				if !found {
					return shedrun.DriverGo, nil
				}
				return childDriverOf(seed), nil
			},
			// WriteSeed is the only place in the batten path that encodes a seed, per the
			// seed-encoding-stays-behind-a-seam-in-battenshed Shared Decision. It validates recipe
			// itself, wrapping an unknown name in battenshed.ErrUnknownRecipe and a known but
			// unbootstrappable one in battenshed.ErrUnsupportedChildRecipe; a shedrun.WriteSeed
			// failure wrapping shedrun.ErrDisagreeingSeed (a pre-existing child seed that disagrees
			// with the one being written) is re-wrapped in battenshed.ErrDisagreeingChildSeed, this
			// package's own spelling -- so seedChildProducer's own errors.Is checks route all three
			// to Stuck rather than a hard error.
			//
			// Params come from childSeedParams: a seed missing one the child's own bootstrap
			// writes is not a smaller seed, it is a seed that bootstrap refuses.
			WriteSeed: func(ctx context.Context, recipe, driver string) error {
				if err := shedrun.ValidateRecipe(recipe); err != nil {
					return fmt.Errorf("%w: %s", battenshed.ErrUnknownRecipe, err.Error())
				}
				// Refused here, ahead of any write, because Spawn above runs loom's bootstrap verb
				// unconditionally: a child seeded with any other recipe would carry a committed,
				// pushed seed that its own bootstrap then refuses as a disagreement, reported from
				// inside the child as a shedrun fault rather than as this choice.
				if recipe != shedrun.RecipeLoom {
					return fmt.Errorf("%w: only %q has a bootstrap verb Run-Shed can start in the task worktree, so a Board task's type must be %q or empty; got %q", battenshed.ErrUnsupportedChildRecipe, shedrun.RecipeLoom, shedrun.RecipeLoom, recipe)
				}
				childLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return err
				}
				params, err := childSeedParams(recipe, childLocation)
				if err != nil {
					return err
				}
				if err := shedrun.WriteSeed(childLocation, shedrun.SelfRunID, shedrun.Seed{Recipe: recipe, Driver: driver, Params: params}); err != nil {
					if errors.Is(err, shedrun.ErrDisagreeingSeed) {
						return fmt.Errorf("%w: %s", battenshed.ErrDisagreeingChildSeed, err.Error())
					}
					return err
				}
				return nil
			},
			// CommitSeed commits the child's own seed onto the child's own fabric pair -- a
			// one-off write, distinct from CommitStatus below, which commits prime's own batten
			// status onto prime's own pair on every non-no-op transition.
			CommitSeed: func(ctx context.Context) error {
				childLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return err
				}
				_, _, err = fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), childLocation, []string{shedrun.SeedRel(shedrun.SelfRunID)}, fmt.Sprintf("batten: seed child %s", slug), fabricengine.EnvSyncOptions())
				return err
			},
			// PushSeed pushes the child's own fabric pair, the same location CommitSeed just
			// committed onto.
			PushSeed: func(ctx context.Context) error {
				childLocation, err := taskWorktreeLocation(location, slug)
				if err != nil {
					return err
				}
				_, err = fabricengine.PushAnchored(childLocation, fabricengine.EnvSyncOptions())
				return err
			},
		},
	}

	c.env = env
	c.shedPaths = shedbuild.ShedPaths{
		StatusPath:     StatusFile(location, slug),
		LockPath:       RunLock(location, slug),
		StatusLockPath: StatusLock(location, slug),
		// CommitStatus is now batten's own seam, committing prime's own batten status onto prime's
		// own pair on every non-no-op transition: the status file is durable, fabric-synced state
		// now (see paths.go), so nil is no longer right here.
		CommitStatus: battenCommitStatusSeam(location, slug),
	}
	return nil
}
