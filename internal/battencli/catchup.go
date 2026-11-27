// catchup.go wires the catch-up from origin over a hub, and the catch-up a pass of one run makes before it resumes.

package battencli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pairteardown"
	"github.com/Knatte18/loomyard/internal/tendercli"
)

// catchUpRefusal is the refusal of a pass whose catch-up failed or stopped before it acted on the run.
// A pass exits with its own code on this refusal, and dobby reads that code as no failing pass.
type catchUpRefusal struct {
	// text is the refusal's message, which ends in its way forward.
	text string
	// err is the catch-up's own error.
	err error
}

// Error returns the refusal's message.
func (r *catchUpRefusal) Error() string { return r.text }

// Unwrap returns the catch-up's own error.
func (r *catchUpRefusal) Unwrap() error { return r.err }

// catchUpRun runs the pass's own catch-up before the pass resumes, through the catchUp seam.
// A nil seam, which only a hand-populated receiver has, catches up nothing.
// A catch-up that failed refuses the pass with nothing acted on, naming the way forward:
// an unreachable origin and a registration refusal each name their own, and any other failure names its error.
// A run the catch-up handed over to the machine that owns it is refused as owned elsewhere.
func (c *battenCLI) catchUpRun() error {
	if c.catchUp == nil {
		return nil
	}
	report, err := c.catchUp()
	if err != nil {
		return c.catchUpFailure(err)
	}
	if slices.Contains(report.HandedOver, c.slug) {
		return c.handedOverRefusal()
	}
	return nil
}

// catchUpFailure words the refusal of a pass whose catch-up returned err.
func (c *battenCLI) catchUpFailure(err error) error {
	rerun := "lym batten run " + c.slug
	var registration *boardengine.RegistrationError
	switch {
	case errors.Is(err, dobbyengine.ErrOriginUnreachable):
		return &catchUpRefusal{
			text: fmt.Sprintf("batten run: origin is unreachable, so the catch-up acted on no run; way forward: run %s again once origin answers", rerun),
			err:  err,
		}
	case errors.As(err, &registration):
		return &catchUpRefusal{text: "batten run: " + err.Error(), err: err}
	default:
		return &catchUpRefusal{
			text: fmt.Sprintf("batten run: the catch-up failed: %v; way forward: fix the cause it names, then run %s again", err, rerun),
			err:  err,
		}
	}
}

// handedOverRefusal is the owned-elsewhere refusal of a run the catch-up handed over.
// It names the owner and the adopt subject the local board reads.
// When the board does not read, it says only what happened.
func (c *battenCLI) handedOverRefusal() error {
	if owner, subject, ok := c.ownerAndSubject(); ok {
		return ownedElsewhereFor(c.slug, owner, subject)
	}
	return &OwnedElsewhereError{
		Slug: c.slug,
		text: fmt.Sprintf("battencli: the catch-up handed %q over to the machine that owns it; way forward: run `lym adopt %s` from this machine's prime to take it over", c.slug, c.slug),
	}
}

// ownerAndSubject reads the slug's effective owner and its adopt subject from the local board.
// It reports whether both read.
func (c *battenCLI) ownerAndSubject() (owner, subject string, ok bool) {
	if c.owner.Read == nil {
		return "", "", false
	}
	owners, err := c.owner.Read()
	if err != nil {
		return "", "", false
	}
	owner, err = owners.Effective(c.slug, c.owner.ParentOf)
	if err != nil {
		return "", "", false
	}
	subject, err = owners.AdoptSubject(c.slug, c.owner.ParentOf)
	if err != nil {
		return "", "", false
	}
	return owner, subject, true
}

// HubCatchUp fills the catch-up's seams over the hub of prime, for the slugs runs lists.
// PullBolt brings Bolt level.
// An unreachable origin maps to dobbyengine.ErrOriginUnreachable.
// A Bolt that cannot be brought level maps to an error naming its detail and way forward.
// Register records this machine in the registry, and a refusal is returned as the registry gave it.
// FetchRun fetches a run's pair, or the prime's records branch for a run with no pair here.
// A pair directory that is no registered linked worktree is a run with no pair here, and no git command runs in it.
// ReadRun reads the run's owner and whether this machine holds commits of it origin lacks.
// PushOwed pushes the prime's records and then the pair, and a push that hands the run over reads as done.
// Launcher names the hub's board directory and recorded shortname.
// The catch-up removes the hub's desktop launcher once this machine owns nothing there.
// A hub with no shortname keeps none.
func HubCatchUp(prime *lymcwd.Location, runs func() ([]string, error)) dobbyengine.CatchUpDeps {
	shortname, _ := fabricengine.ReadShortname(fabriclayout.BoltDir(prime.HubPath))
	return dobbyengine.CatchUpDeps{
		Launcher:          dobbyengine.Launcher{BoardDir: fabriclayout.BoardDir(prime.HubPath), Shortname: shortname},
		PullBolt:          func() error { return pullBoltForCatchUp(prime) },
		Register:          func() error { return registerThisMachine(prime) },
		Runs:              runs,
		FetchRun:          func(slug string) error { return fetchRunForCatchUp(prime, slug) },
		ReadRun:           func(slug string) (dobbyengine.CatchUpRun, error) { return readRunForCatchUp(prime, slug) },
		PushOwed:          func(slug string) error { return pushOwedForCatchUp(prime, slug) },
		HandOver:          func(slug, owner string) error { return pairteardown.HandOverOwnedElsewhere(prime, slug, owner) },
		ApplyPauseIntents: func(slugs []string) error { return tendercli.PauseIntentEntries(prime, slugs) },
	}
}

// boltStands reports whether the hub of prime has a Bolt checkout, which a hub with no board does not.
func boltStands(prime *lymcwd.Location) (bool, error) {
	if _, err := os.Stat(fabriclayout.BoltDir(prime.HubPath)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// pullBoltForCatchUp brings the hub's Bolt level with origin.
// BOARD_SKIP_GIT and BOARD_SKIP_PUSH skip the pull, as they skip every board write.
func pullBoltForCatchUp(prime *lymcwd.Location) error {
	stands, err := boltStands(prime)
	if err != nil || !stands {
		return err
	}
	if skip := boardengine.ApplySkipEnv(boardengine.Config{}); skip.SkipGit || skip.SkipPush {
		return nil
	}
	skipped, detail, err := fabricengine.NewBolt(fabriclayout.BoltDir(prime.HubPath)).Pull(fabricengine.SyncOptions{}, fabricengine.NewMutations(prime.HubPath))
	if err != nil {
		return err
	}
	switch skipped {
	case "":
		return nil
	case fabriclayout.BoltSkipFetchFailed:
		return fmt.Errorf("%w: %s", dobbyengine.ErrOriginUnreachable, detail)
	default:
		return fmt.Errorf("the board could not be brought up to date (%s): %s", skipped, detail)
	}
}

// registerThisMachine registers this machine in the hub's registry.
// It writes the machine's id into the machine file first when the file has none.
func registerThisMachine(prime *lymcwd.Location) error {
	stands, err := boltStands(prime)
	if err != nil || !stands {
		return err
	}
	identity, err := boardengine.EnsureIdentity()
	if err != nil {
		return err
	}
	if _, err := boardengine.RegisterMachine(fabriclayout.BoardDir(prime.HubPath), boardengine.Registration{Identity: identity}); err != nil {
		if errors.Is(err, boardengine.ErrOwnerOriginUnreachable) {
			return fmt.Errorf("%w: %v", dobbyengine.ErrOriginUnreachable, err)
		}
		return err
	}
	return nil
}

// catchUpPair returns the location of the pair of slug and whether it stands here.
// A pair stands when its task worktree is a registered linked worktree of the prime's repository.
func catchUpPair(prime *lymcwd.Location, slug string) (*lymcwd.Location, bool, error) {
	teardown, err := pairteardown.New(prime)
	if err != nil {
		return nil, false, err
	}
	gone, err := teardown.TaskWorktreeGone(slug)
	if err != nil || gone {
		return nil, false, err
	}
	pair, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))
	if err != nil {
		return nil, false, err
	}
	return pair, true, nil
}

// fetchRunForCatchUp fetches the branches of the run of slug from origin.
// A fetch that fails reads as an unreachable origin.
// A records layout the fetch cannot read is returned as is, with the refspecs that restore it.
// A fetch folds git's exit into its message and names no transport failure.
func fetchRunForCatchUp(prime *lymcwd.Location, slug string) error {
	pair, stands, err := catchUpPair(prime, slug)
	if err != nil {
		return err
	}
	if stands {
		err = fabricengine.FetchPairTips(pair)
	} else {
		err = gitrepo.New(fabricengine.RecordsWorktree(prime)).Fetch()
	}
	if errors.Is(err, fabricengine.ErrRecordsLayout) {
		return err
	}
	if err != nil {
		return fmt.Errorf("%w: %v", dobbyengine.ErrOriginUnreachable, err)
	}
	return nil
}

// readRunForCatchUp reads the owner of the run of slug and whether this machine holds commits of it origin lacks.
func readRunForCatchUp(prime *lymcwd.Location, slug string) (dobbyengine.CatchUpRun, error) {
	_, inputs, err := pairteardown.ReadRun(prime, slug)
	if err != nil {
		return dobbyengine.CatchUpRun{}, err
	}
	pair, stands, err := catchUpPair(prime, slug)
	if err != nil {
		return dobbyengine.CatchUpRun{}, err
	}
	sides := []string{fabricengine.RecordsWorktree(prime)}
	if stands {
		sides = append(sides, fabricengine.RecordsWorktree(pair), pair.WorktreePath())
	}
	for _, side := range sides {
		ahead, err := gitrepo.New(side).HasUnpushed()
		if err != nil {
			return dobbyengine.CatchUpRun{}, fmt.Errorf("read whether %s holds commits origin lacks: %w", side, err)
		}
		if ahead {
			return dobbyengine.CatchUpRun{Owner: inputs.Owner, OwnedElsewhere: inputs.OwnedElsewhere, Ahead: true}, nil
		}
	}
	return dobbyengine.CatchUpRun{Owner: inputs.Owner, OwnedElsewhere: inputs.OwnedElsewhere}, nil
}

// pushOwedForCatchUp pushes the prime's records and then the pair of slug when it stands here, records side first.
// A push that meets another machine's takeover entry hands the run over, and ends the call with nil.
func pushOwedForCatchUp(prime *lymcwd.Location, slug string) error {
	push := pairteardown.RunPush{LockWait: fabriclayout.StatusPushLockWait}
	if _, err := pairteardown.PushRun(prime, prime, slug, push); err != nil {
		return handedOverIsDone(err)
	}
	pair, stands, err := catchUpPair(prime, slug)
	if err != nil || !stands {
		return err
	}
	_, err = pairteardown.PushRun(prime, pair, slug, push)
	return handedOverIsDone(err)
}

// handedOverIsDone reads an error wrapping fabricengine.ErrHandedOver as nil.
// The run left this machine, so nothing is owed.
func handedOverIsDone(err error) error {
	if errors.Is(err, fabricengine.ErrHandedOver) {
		return nil
	}
	return err
}

// DobbyPairSlugs lists the slugs of the runs whose parent is the dobby's worktree and whose pair stands here.
// A pair stands when its worktree path exists, as RunView.PairPresent reads it.
// The prime's dobby also lists every local pair whose parent beam reads owned elsewhere.
// No dobby starts for such a beam, so each local pair is caught up by some dobby.
func DobbyPairSlugs(prime *lymcwd.Location, geom dobbyengine.Geometry, parents *DobbyParents) ([]string, error) {
	runs, err := dobbyParentRuns(prime, geom, parents)
	if err != nil {
		return nil, err
	}
	var slugs []string
	for _, run := range runs {
		stands, err := pairStands(prime, run.slug)
		if err != nil {
			return nil, err
		}
		if stands {
			slugs = append(slugs, run.slug)
		}
	}
	if geom.Slug != "" {
		return slugs, nil
	}
	foreign, err := pairsUnderForeignBeams(prime, geom, parents)
	if err != nil {
		return nil, err
	}
	for _, slug := range foreign {
		if !slices.Contains(slugs, slug) {
			slugs = append(slugs, slug)
		}
	}
	return slugs, nil
}

// HubRunSlugs lists the slugs of every run of the hub whose pair stands here, sorted and without duplicates.
// It covers the runs under the prime and under each local beam.
// A geometry or run list that does not read is returned as its error.
func HubRunSlugs(prime *lymcwd.Location) ([]string, error) {
	beams, err := hubgeom.DobbyStart(prime).Beams()
	if err != nil {
		return nil, err
	}
	worktrees := []string{prime.WorktreeName}
	for _, beam := range beams {
		worktrees = append(worktrees, beam.Slug)
	}
	parents := NewDobbyParents(prime)
	var slugs []string
	for _, worktree := range worktrees {
		geom, err := hubgeom.DobbyGeometry(prime, worktree)
		if err != nil {
			return nil, err
		}
		listed, err := DobbyPairSlugs(prime, geom, parents)
		if err != nil {
			return nil, err
		}
		slugs = append(slugs, listed...)
	}
	slices.Sort(slugs)
	return slices.Compact(slugs), nil
}

// pairStands reports whether the pair worktree of slug exists on disk.
func pairStands(prime *lymcwd.Location, slug string) (bool, error) {
	pair, err := lymcwd.NamedWorktree(prime.HubPath, slug)
	if err != nil {
		return false, fmt.Errorf("locate the pair %s: %w", slug, err)
	}
	return markerExists(pair.WorktreePath())
}

// pairsUnderForeignBeams lists the slugs of the local pairs whose parent is a beam another machine owns.
// A pair whose parent does not read is left out with one Warn, so one broken pair never stops the rest.
func pairsUnderForeignBeams(prime *lymcwd.Location, geom dobbyengine.Geometry, parents *DobbyParents) ([]string, error) {
	worktrees, err := fabricengine.List(prime.WorktreePath())
	if err != nil {
		return nil, err
	}
	owners, err := boardengine.ReadOwners(geom.BoardDir)
	if err != nil {
		return nil, err
	}
	here, err := machinename.ReadIdentity()
	if err != nil {
		return nil, err
	}
	var slugs []string
	for _, worktree := range worktrees {
		if worktree.Main || worktree.Prunable {
			continue
		}
		slug := filepath.Base(worktree.Path)
		parent, err := parents.Of(slug)
		if err != nil {
			logger.Warn("battencli: a local pair's parent is unreadable, so it is left out of the catch-up scope", "slug", slug, "error", err)
			continue
		}
		if parent == "" {
			continue
		}
		_, elsewhere, err := readTaskPlacement(prime, parents, owners, here, parent)
		if err != nil {
			logger.Warn("battencli: the owner of a local pair's parent beam is unreadable, so the pair is left out of the catch-up scope", "slug", slug, "error", err)
			continue
		}
		if elsewhere {
			slugs = append(slugs, slug)
		}
	}
	return slugs, nil
}
