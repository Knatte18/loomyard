// projectrecord.go places a run's decision record by its recipe.
// A recipe with a Beam-Await row gets the beam's project record; any other recipe gets the discussion record.
// It finds the project records that bind a member's run, read live from each enclosing beam's worktree.
// It fills a member prompt's project directive from those records.
// It also finds the member pairs a project decision reaches.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/seatengine"
)

// beamAwaitEngine is the engine of a beam run's build row, the row that marks the recipe as a beam's life.
const beamAwaitEngine = "BeamAwait"

// runRecord locates a run's decision record: the record, its support log, and the directory holding both.
// rel and dirRel are the record and the directory in anchor-relative form.
type runRecord struct {
	path, supportLog, dir string
	rel, dirRel           string
	// project is true for a beam's project record.
	project bool
}

// runRecordOf returns the decision record of the run at location whose recipe is recipe.
// A recipe with a Beam-Await row is a beam's life, and its record is the beam's project record.
// The record is kept under the beam's slug, so no member's own record and no enclosing beam's record shares its path.
// Every other recipe's record is the discussion record.
func runRecordOf(location *lymcwd.Location, recipe string) runRecord {
	if recipeHasEngine(recipe, beamAwaitEngine) {
		slug := seedSlug(location.WorktreeName)
		return runRecord{
			path:       loomengine.ProjectDecisionRecord(location, slug),
			supportLog: loomengine.ProjectSupportLog(location, slug),
			dir:        loomengine.ProjectDir(location, slug),
			rel:        loomengine.ProjectDecisionRecordRel(slug),
			dirRel:     loomengine.ProjectDirRel(slug),
			project:    true,
		}
	}
	return runRecord{
		path:       loomengine.DiscussionDecisionRecord(location),
		supportLog: loomengine.DiscussionSupportLog(location),
		dir:        loomengine.DiscussionDir(location),
		rel:        loomengine.DiscussionDecisionRecordRel(),
		dirRel:     loomengine.DiscussionDirRel(),
	}
}

// commitRunRecord commits record's directory in location's worktree through the fabric.
// The discussion record commits with the plan amendment record.
// A repeat commit over an unchanged directory is a no-op.
func commitRunRecord(location *lymcwd.Location, record runRecord) error {
	if !record.project {
		return commitDiscussion(location)
	}
	_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{record.dirRel}, fmt.Sprintf("loom: project record for %s", seedSlug(location.WorktreeName)), fabricengine.EnvSyncOptions())
	return err
}

// recordHistory returns the seam that reads every committed version of record in location's worktree, newest first.
// Building it opens nothing: the fabric is read on each call.
func recordHistory(location *lymcwd.Location, record runRecord) func() ([][]byte, error) {
	return func() ([][]byte, error) {
		return fabricengine.CommittedAnchoredFileHistory(location, record.rel)
	}
}

// projectRecordPaths returns the project records that bind the run of the pair at location, nearest beam first.
// It reads the record of each beam above the pair in that beam's own worktree.
// The beams come from the board, read through ParentBeam, so a member's project links name them.
// A beam whose worktree or record is absent here is left out, and a pair under no beam has none.
// The paths name each beam's live record, never the copy a pair inherited when it forked,
// so a decision recorded on the beam after the fork is read too.
func projectRecordPaths(location *lymcwd.Location) ([]string, error) {
	tasks, err := boardengine.ReadTasks(fabricengine.BoardDir(location.HubPath))
	if err != nil {
		return nil, err
	}
	var paths []string
	seen := map[string]bool{}
	for slug := seedSlug(location.WorktreeName); ; {
		beam := boardengine.ParentBeam(tasks, slug)
		if beam == "" || seen[beam] {
			return paths, nil
		}
		seen[beam] = true
		path, found, err := beamProjectRecord(location, beam)
		if err != nil {
			return nil, err
		}
		if found {
			paths = append(paths, path)
		}
		slug = beam
	}
}

// beamProjectRecord returns the path of the project record in the worktree of the beam slug.
// found is false when the worktree or its record is absent.
func beamProjectRecord(location *lymcwd.Location, slug string) (string, bool, error) {
	beam, err := lymcwd.NamedWorktree(location.HubPath, slug)
	if err != nil {
		return "", false, err
	}
	path := loomengine.ProjectDecisionRecord(beam, slug)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	return path, true, nil
}

// memberPairs returns the local worktrees of the runs working under the beam slug.
// Those are each open member's pair and each pair forked from the beam, at any depth of grouping projects.
func memberPairs(location *lymcwd.Location, slug string) ([]*lymcwd.Location, error) {
	prime, err := primeOfHub(location)
	if err != nil {
		return nil, err
	}
	blockers, err := battencli.ReadBeamLandBlockers(prime, slug)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(blockers.Members)+len(blockers.ChildPairs))
	for _, member := range blockers.Members {
		if member.HasPair {
			names = append(names, member.Slug)
		}
	}
	names = append(names, blockers.ChildPairs...)

	var pairs []*lymcwd.Location
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		pair, err := lymcwd.NamedWorktree(location.HubPath, name)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(pair.WorktreePath()); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		pairs = append(pairs, pair)
	}
	return pairs, nil
}

// withProjectDirective fills table's project directive from the project records that bind the pair's run.
// It reads the records at call time, so a decision recorded on a beam reaches the next spawn.
// A pair under no beam keeps the marker empty.
func withProjectDirective(table seatengine.Table, location *lymcwd.Location, stencilsDir string) (seatengine.Table, error) {
	records, err := projectRecordPaths(location)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("loom: read the project records above %s: %w", location.WorktreeName, err)
	}
	directive, err := loomengine.ProjectRecordDirective(stencilsDir, records)
	if err != nil {
		return seatengine.Table{}, err
	}
	if table.Values == nil {
		table.Values = map[string]string{}
	}
	table.Values[loomengine.ProjectDirectiveMarker] = directive
	return table, nil
}
