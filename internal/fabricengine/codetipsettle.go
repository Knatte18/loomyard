// codetipsettle.go holds the owner's side of a code tip origin lacks:
// fetching a pair's tips and returning a run to its previous transition when the tip is given up.

package fabricengine

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// FetchPairTips fetches the code branch and the records branch of the pair at pair from origin.
// A reader of origin's fetched refs, such as OriginCodeTip, then sees what origin holds now.
func FetchPairTips(pair *lymcwd.Location) error {
	if err := gitrepo.New(RecordsWorktree(pair)).Fetch(); err != nil {
		return fmt.Errorf("fetch the records branch of the pair at %s: %w", pair.WorktreePath(), err)
	}
	if err := gitrepo.New(pair.WorktreePath()).Fetch(); err != nil {
		return fmt.Errorf("fetch the code branch of the pair at %s: %w", pair.WorktreePath(), err)
	}
	return nil
}

// ReturnToPreviousTransition gives up the code tip the last transition of slug names.
// It fetches the pair's records branch.
// On origin's ref it finds the newest commit whose Run-Transition names slug, and the previous such commit.
// It skips the records commits that mark no transition.
// It commits, in the pair's records worktree, the run's status as that previous transition left it.
// The commit's Warp-SHA is the previous transition's, never the pair's code HEAD.
// The commit carries a Run-Transition trailer.
// It reports false, committing nothing, when origin's records branch holds fewer than two transitions of slug:
// the first transition names the tip the pair was made at, so nothing earlier can stand in.
// It pushes nothing.
// A one-repository pair's code tip is never owed apart from its records, so it reports false and commits nothing.
func ReturnToPreviousTransition(pair *lymcwd.Location, slug string) (returned bool, err error) {
	if oneRepositoryAt(pair.WorktreePath()) {
		return false, nil
	}
	recordsDir := RecordsWorktree(pair)
	repo := gitrepo.New(recordsDir)
	if err := repo.Fetch(); err != nil {
		return false, fmt.Errorf("fetch the records branch of %q: %w", slug, err)
	}
	upstream, hasUpstream, err := upstreamSHAAt(recordsDir)
	if err != nil || !hasUpstream {
		return false, err
	}
	commits, err := repo.Log(upstream, gitrepo.LogQuery{FirstParent: true})
	if err != nil {
		return false, fmt.Errorf("read the records history of %q at %s: %w", slug, upstream, err)
	}
	var transitions []gitrepo.LogCommit
	for _, commit := range commits {
		if trailerValue(commit.Message, TransitionTrailerKey) != slug {
			continue
		}
		transitions = append(transitions, commit)
		if len(transitions) == 2 {
			break
		}
	}
	if len(transitions) < 2 {
		return false, nil
	}
	previous := transitions[1]
	previousTip := trailerValue(previous.Message, WarpSHATrailerKey)
	if previousTip == "" {
		return false, fmt.Errorf("the previous transition %s of %q names no code tip", previous.SHA, slug)
	}

	runID, status, found, err := statusAtCommit(pair, recordsDir, previous.SHA, slug)
	if err != nil {
		return false, err
	}
	if !found {
		return false, fmt.Errorf("the previous transition %s of %q holds no status record", previous.SHA, slug)
	}
	statusPath := shedrun.StatusFile(pair, runID)
	if err := state.WriteJSON(statusPath, shedrun.StatusLock(pair, runID), status); err != nil {
		return false, fmt.Errorf("write the status of %q as the previous transition left it: %w", slug, err)
	}
	statusRel, err := filepath.Rel(pair.AnchorPath(), statusPath)
	if err != nil {
		return false, fmt.Errorf("place the status path of %q under its anchor: %w", slug, err)
	}

	message := appendTrailerLine(appendWarpSHATrailer(slug+": return to the previous transition", previousTip), TransitionTrailerKey, slug)
	_, committed, err := CommitAnchoredPaths(NewMutations(pair.HubPath), pair, []string{statusRel}, message, SyncOptions{})
	if err != nil {
		return false, fmt.Errorf("commit the return of %q to its previous transition: %w", slug, err)
	}
	if !committed {
		return false, fmt.Errorf("the status of %q already equals the previous transition's, so the return committed nothing", slug)
	}
	return true, nil
}

// statusAtCommit reads the run record of slug at the records commit sha of the repository at recordsDir.
// It tries the run directory named for the slug, then the legacy `self` one, and returns the run id that held it.
func statusAtCommit(pair *lymcwd.Location, recordsDir, sha, slug string) (runID string, status shedengine.Status, found bool, err error) {
	names, err := RepoWiredNames(pair)
	if err != nil {
		return "", shedengine.Status{}, false, fmt.Errorf("load the fabric config: %w", err)
	}
	anchor, err := treeAnchor(recordsDir, sha, names, pair.AnchorRel)
	if err != nil {
		return "", shedengine.Status{}, false, err
	}
	for _, candidate := range []string{slug, shedrun.SelfRunID} {
		_, statusRel, err := shedrun.TreeRunFileRels(candidate)
		if err != nil {
			return "", shedengine.Status{}, false, err
		}
		data, err := showFile(recordsDir, sha, path.Join(filepath.ToSlash(anchor), statusRel))
		if err != nil {
			return "", shedengine.Status{}, false, fmt.Errorf("read the status of %q at %s: %w", slug, sha, err)
		}
		if data == nil {
			continue
		}
		status, err := shedengine.ParseStatusTolerant(data)
		if err != nil {
			return "", shedengine.Status{}, false, fmt.Errorf("parse the status of %q at %s: %w", slug, sha, err)
		}
		return candidate, status, true, nil
	}
	return "", shedengine.Status{}, false, nil
}
