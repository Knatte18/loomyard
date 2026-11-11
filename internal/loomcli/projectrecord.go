// projectrecord.go places a run's decision record by its recipe.
// A recipe with a Beam-Await row gets the beam's project record; any other recipe gets the discussion record.
// It reads the record's history back to its newest replace.
// It finds the project records that bind a member's run, read live from each enclosing beam's worktree.
// It fills a member prompt's project directive from those records.
// It also finds the member pairs a project decision reaches.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

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

// recordReplacedTrailerKey is the git trailer key a `lym loom decision replace` commit carries.
// Its value is three fields separated by spaces.
// They are the call's --by label, the record's anchor-relative slash-separated path and the sha256 of the bytes written.
const recordReplacedTrailerKey = "Lym-Record-Replaced"

// recordReplacedValue returns the Lym-Record-Replaced trailer value of a replace by by that wrote written at rel.
func recordReplacedValue(by, rel string, written []byte) string {
	return fmt.Sprintf("%s %s %s", by, filepath.ToSlash(rel), sha256Hex(written))
}

// parseRecordReplaced returns the record path and sha256 a Lym-Record-Replaced value names.
// ok is false for a value that is not three space-separated fields.
func parseRecordReplaced(value string) (path, sum string, ok bool) {
	fields := strings.Fields(value)
	if len(fields) != 3 {
		return "", "", false
	}
	return fields[1], fields[2], true
}

// recordHistory returns the seam that reads the committed versions of record in location's worktree, newest first.
// They are the versions the dropped-entry check holds the record to.
// With no replace trailer naming the record's path, those are every committed version, read along the path walk.
// Otherwise they run from the cut of the newest such trailer onward, as cutAtReplace finds it,
// so an entry a replace dropped is no longer held to the record.
// A trailer whose sha256 matches no version at or before its commit cuts nothing, and the history is read whole.
// Building it opens nothing: the fabric is read on each call.
func recordHistory(location *lymcwd.Location, record runRecord) func() ([][]byte, error) {
	return func() ([][]byte, error) {
		trailers, err := fabricengine.TrailerCommits(location, recordReplacedTrailerKey)
		if err != nil {
			return nil, err
		}
		return recordHistoryFrom(filepath.ToSlash(record.rel), trailers,
			func() ([]fabricengine.AnchoredFileVersion, error) {
				return fabricengine.AnchoredFileFirstParentHistory(location, record.rel)
			},
			func() ([][]byte, error) { return fabricengine.CommittedAnchoredFileHistory(location, record.rel) })
	}
}

// recordHistoryFrom returns the versions of the record at rel that the dropped-entry check holds it to, newest first.
// trailers are the replace trailer commits, newest first.
// When one names rel, readVersions supplies the versions to cut.
// If the cut is found, the result is the versions from the cut onward.
// Otherwise they are the whole history readWhole returns; readVersions is read only when a trailer names rel.
func recordHistoryFrom(
	rel string,
	trailers []fabricengine.TrailerCommit,
	readVersions func() ([]fabricengine.AnchoredFileVersion, error),
	readWhole func() ([][]byte, error),
) ([][]byte, error) {
	if trailer, found := newestReplaceOf(rel, trailers); found {
		versions, err := readVersions()
		if err != nil {
			return nil, err
		}
		if cut, found := cutAtReplace(trailer, versions); found {
			return cut, nil
		}
	}
	return readWhole()
}

// newestReplaceOf returns the newest of trailers whose Lym-Record-Replaced value names the record at rel.
// trailers come newest first.
// A pair's fabric history inherits its parent's commits, so a trailer may name another record and is skipped.
func newestReplaceOf(rel string, trailers []fabricengine.TrailerCommit) (fabricengine.TrailerCommit, bool) {
	for _, trailer := range trailers {
		if path, _, ok := parseRecordReplaced(trailer.Value); ok && path == rel {
			return trailer, true
		}
	}
	return fabricengine.TrailerCommit{}, false
}

// cutAtReplace returns the record versions from the replace trailer's cut onward, newest first.
// versions are the record's first-parent history, newest first.
// The cut is the newest version whose bytes hash to the trailer's sha256.
// Its first-parent position must be at or before the trailer commit's.
// So whichever commit landed the replaced bytes first is the cut,
// and a later commit of the same bytes never is, which keeps a call recorded after the replace held to the record.
// found is false when no version qualifies.
func cutAtReplace(trailer fabricengine.TrailerCommit, versions []fabricengine.AnchoredFileVersion) ([][]byte, bool) {
	_, sum, _ := parseRecordReplaced(trailer.Value)
	for i, version := range versions {
		if version.Position > trailer.Position || sha256Hex(version.Data) != sum {
			continue
		}
		kept := make([][]byte, 0, i+1)
		for _, newer := range versions[:i+1] {
			kept = append(kept, newer.Data)
		}
		return kept, true
	}
	return nil, false
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
