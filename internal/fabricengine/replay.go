// replay.go holds the records replay: bringing a records branch level with a moved origin by replaying the local commits origin lacks, and the ended-set rule that decides which of those commits a run's end on origin has made obsolete.
// A weft worktree is replayed commit by commit; the board is replayed by entry through the installed board replay.

package fabricengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// The board's file names a replay recognises: the entry store, the rendered overview and the per-entry design files.
const (
	boardFileName     = "board.json"
	boardOverviewFile = "README.md"
	boardDesignPrefix = "design-"
	boardDesignSuffix = ".md"
	// boardEntryDone is the status an entry carries once its run has landed.
	boardEntryDone = "done"
)

// The two ways an entry ends on origin.
const (
	endedDone    = "done"
	endedRemoved = "removed"
)

// setAsideTagPrefix is the short-name prefix of the local tag that keeps a commit a replay set aside.
// No archive reader matches it.
const setAsideTagPrefix = "set-aside/"

// The loom and batten rows whose status marks how far a run's landing got on this machine.
const (
	finalizeRow         = "Finalize"
	landRow             = "Land"
	worktreeTeardownRow = "Worktree-Teardown"
)

// ErrReplayConflict is returned, wrapped with the conflicting paths, by a replay that stopped on a commit it could not apply, with the starting tip restored.
var ErrReplayConflict error = errors.New("records replay stopped on a conflict")

// ErrReplayDirty is returned, wrapped with the paths, by a replay that refused over an uncommitted tracked change on a path it would touch, with the branch unmoved.
var ErrReplayDirty error = errors.New("records replay refused over an uncommitted change")

// replayConflict is the error cherryPickOnto returns for a conflicting pick: it is ErrReplayConflict and carries the conflicting paths.
type replayConflict struct {
	commit string
	paths  []string
	cause  error
}

func (e *replayConflict) Error() string {
	return fmt.Sprintf("%s: commit %s conflicts on %s", ErrReplayConflict, e.commit, joinOrNone(e.paths))
}

func (e *replayConflict) Is(target error) bool { return target == ErrReplayConflict }

func (e *replayConflict) Unwrap() error { return e.cause }

// EndedSet maps the slug of each run origin has ended to how: "done" or "removed".
type EndedSet map[string]string

// SetAsideCommit is a local commit a replay did not re-apply, kept under a local tag.
type SetAsideCommit struct {
	Commit string
	Slug   string
	Tag    string
}

// ReplayOutcome reports what a records replay did with the local commits origin lacked.
// Replayed holds the commits re-applied, Dropped those that became empty or were seed commits, and SetAside those kept only under a tag.
// Conflicts names the paths a stop conflicted on, and Lost and Refused the board changes a by-entry replay did not apply.
type ReplayOutcome struct {
	Replayed  []string
	Dropped   []string
	SetAside  []SetAsideCommit
	Conflicts []string
	Lost      []BoardReplayFinding
	Refused   []BoardReplayFinding
}

// readEndedSet reads which of slugs origin has ended, from the records repository at recordsRoot and its ref boltRef, origin's Bolt tip as last fetched.
// A slug is ended when its entry is `done` on that tip, or is absent from the tip and present in a commit of the tip's history: a pruned entry.
// Where localCreated holds a created stamp for the slug, the entry on the tip or in the history must carry the same stamp,
// so a pruned slug reused by an entry only the local board holds is never ended.
// A slug only the local board ever held matches nothing.
// It runs no network call, and reads each `board.json` through the installed board replay.
func readEndedSet(recordsRoot, boltRef string, slugs []string, localCreated map[string]string) (EndedSet, error) {
	replay, err := InstalledBoardReplay()
	if err != nil {
		return nil, err
	}
	repo := gitrepo.New(recordsRoot)
	tip, err := repo.ResolveSHA(boltRef)
	if err != nil {
		return nil, fmt.Errorf("read the ended set: %w", err)
	}
	entriesAt := func(sha string) ([]BoardEntry, error) {
		data, err := repo.FileAtRevision(sha, boardFileName)
		if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return replay.Entries(data)
	}
	findEntry := func(entries []BoardEntry, slug string) (BoardEntry, bool) {
		for _, entry := range entries {
			if entry.Slug == slug {
				return entry, true
			}
		}
		return BoardEntry{}, false
	}

	tipEntries, err := entriesAt(tip)
	if err != nil {
		return nil, fmt.Errorf("read the ended set: %s at %s: %w", boardFileName, tip, err)
	}
	ended := EndedSet{}
	var absent []string
	for _, slug := range slugs {
		entry, held := findEntry(tipEntries, slug)
		switch {
		case !held:
			absent = append(absent, slug)
		case entry.Status == boardEntryDone && createdStampMatches(entry, localCreated[slug]):
			ended[slug] = endedDone
		}
	}
	if len(absent) == 0 {
		return ended, nil
	}

	history, err := repo.Log(tip, gitrepo.LogQuery{PathPrefix: boardFileName})
	if err != nil {
		return nil, fmt.Errorf("read the ended set: history of %s at %s: %w", boardFileName, tip, err)
	}
	for _, commit := range history {
		if len(absent) == 0 {
			break
		}
		entries, err := entriesAt(commit.SHA)
		if err != nil {
			return nil, fmt.Errorf("read the ended set: %s at %s: %w", boardFileName, commit.SHA, err)
		}
		absent = slices.DeleteFunc(absent, func(slug string) bool {
			entry, held := findEntry(entries, slug)
			if !held || !createdStampMatches(entry, localCreated[slug]) {
				return false
			}
			ended[slug] = endedRemoved
			return true
		})
	}
	return ended, nil
}

// createdStampMatches reports whether entry carries the created stamp, an RFC 3339 time as board.json stores it.
// An empty stamp matches every entry.
func createdStampMatches(entry BoardEntry, stamp string) bool {
	if stamp == "" {
		return true
	}
	parsed, err := time.Parse(time.RFC3339Nano, stamp)
	return err == nil && !entry.Created.IsZero() && parsed.Equal(entry.Created)
}

// runStatusRows is the part of a run's status.json the finalize test reads.
// Only the producer names are decoded, and an unknown key is tolerated.
type runStatusRows struct {
	CurrentProducer string `json:"current_producer"`
	State           string `json:"state"`
	History         []struct {
		Producer string `json:"producer"`
	} `json:"history"`
}

// names reports whether the status's current producer or a history row is one of rows.
func (s runStatusRows) names(rows ...string) bool {
	if slices.Contains(rows, s.CurrentProducer) {
		return true
	}
	for _, entry := range s.History {
		if slices.Contains(rows, entry.Producer) {
			return true
		}
	}
	return false
}

// historyNames reports whether a history row, never the current producer, is row.
func (s runStatusRows) historyNames(row string) bool {
	for _, entry := range s.History {
		if entry.Producer == row {
			return true
		}
	}
	return false
}

// readRunStatusRows reads the status.json at path, and reports false for an absent file.
func readRunStatusRows(path string) (runStatusRows, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return runStatusRows{}, false, nil
	}
	if err != nil {
		return runStatusRows{}, false, err
	}
	var status runStatusRows
	if err := json.Unmarshal(data, &status); err != nil {
		return runStatusRows{}, false, fmt.Errorf("decode %s: %w", path, err)
	}
	return status, true, nil
}

// finalizeEntered reports whether this machine's local state shows slug's run past the parent-side merge of its Finalize.
// After that merge Finalize writes the done mark, and its landing reaches origin.
// It reads the task pair's loom status, the pair's local parent branch and the prime's batten status of the slug:
// a Finalize history row, a current producer Finalize while the local parent branch already holds the task's work, or a batten status at Land or a later row or done.
// A current producer Finalize alone never counts, since it reads Finalize at every Finalize entry, before the merge.
// A read failure counts as entered, logged at Warn, so a doubt never sets aside this machine's own landing.
func finalizeEntered(l *lyxcwd.Location, slug string) bool {
	entered, err := finalizeEnteredRead(l, slug)
	if err != nil {
		logger.Warn("fabricengine: the run's Finalize state could not be read, so its landing counts as entered", "slug", slug, "error", err)
		return true
	}
	return entered
}

func finalizeEnteredRead(l *lyxcwd.Location, slug string) (bool, error) {
	pairPath := filepath.Join(l.HubPath, slug)
	if _, err := os.Stat(pairPath); err == nil {
		pairLocation, err := lyxcwd.ResolveWorktree(pairPath)
		if err != nil {
			return false, err
		}
		status, found, err := readRunStatusRows(shedrun.StatusFile(pairLocation, shedrun.SelfRunID))
		if err != nil {
			return false, err
		}
		if found && status.historyNames(finalizeRow) {
			return true, nil
		}
		if found && status.CurrentProducer == finalizeRow {
			holds, err := parentHoldsTaskWork(l, pairLocation, slug)
			if err != nil {
				return false, err
			}
			if holds {
				return true, nil
			}
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}

	primeName, err := PrimeName(l)
	if err != nil {
		return false, err
	}
	primeLocation, err := lyxcwd.ResolveWorktree(filepath.Join(l.HubPath, primeName))
	if err != nil {
		return false, err
	}
	status, found, err := readRunStatusRows(shedrun.StatusFile(primeLocation, slug))
	if err != nil {
		return false, err
	}
	return found && (status.State == "done" || status.names(landRow, worktreeTeardownRow)), nil
}

// parentHoldsTaskWork reports whether the pair's local parent branch already holds the work of the pair's task branch.
func parentHoldsTaskWork(l, pairLocation *lyxcwd.Location, slug string) (bool, error) {
	origin, found, err := ReadOriginFor(l, slug)
	if err != nil {
		return false, err
	}
	if !found {
		return false, fmt.Errorf("the pair %s has no origin record naming its parent branch", slug)
	}
	pair, err := Open(pairLocation)
	if err != nil {
		return false, err
	}
	taskHead, err := pair.code.CurrentSHA()
	if err != nil {
		return false, err
	}
	return pair.HoldsWork("refs/heads/"+origin.ParentBranch, taskHead)
}

// ParentHoldsTaskWork reports whether the pair's local parent branch already holds the work of the pair's task branch.
// That marks a Finalize already past its parent-side merge.
// It returns an error when the pair has no origin record naming its parent branch or a branch cannot be read.
func ParentHoldsTaskWork(l, pairLocation *lyxcwd.Location, slug string) (bool, error) {
	return parentHoldsTaskWork(l, pairLocation, slug)
}

// withoutFinalizeEntered returns the part of ended whose slugs this machine's own landing has not reached,
// so a replay never sets aside the commits of a run this machine landed.
func withoutFinalizeEntered(l *lyxcwd.Location, ended EndedSet) EndedSet {
	if len(ended) == 0 {
		return ended
	}
	live := EndedSet{}
	for slug, how := range ended {
		if finalizeEntered(l, slug) {
			logger.Info("fabricengine: a run origin has ended is left to this machine's own landing", "slug", slug)
			continue
		}
		live[slug] = how
	}
	return live
}

// sortedSlugs returns the slugs of ended in order.
func sortedSlugs(ended EndedSet) []string {
	slugs := make([]string, 0, len(ended))
	for slug := range ended {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	return slugs
}

// runDirectorySlug returns the slug of the ended run whose record directory holds path, a records-repository-relative slash path.
func runDirectorySlug(l *lyxcwd.Location, ended EndedSet, path string) (string, bool) {
	for _, slug := range sortedSlugs(ended) {
		dir := pathpkg.Clean(pathpkg.Join(filepath.ToSlash(l.AnchorRel), filepath.ToSlash(shedrun.RunDirRel(l, slug))))
		if strings.HasPrefix(path, dir+"/") {
			return slug, true
		}
	}
	return "", false
}

// replayAhead returns the commits repo holds over upstream, oldest first, and repo's current tip.
// It returns no commit when upstream holds nothing repo lacks, since there is nothing to replay then.
func replayAhead(repo *gitrepo.Repo, upstream string) (tip string, ahead []string, err error) {
	tip, err = repo.CurrentSHA()
	if err != nil {
		return "", nil, err
	}
	behind, err := repo.CommitsNotIn(upstream, tip)
	if err != nil {
		return "", nil, err
	}
	if len(behind) == 0 {
		return tip, nil, nil
	}
	ahead, err = repo.CommitsNotIn(tip, upstream)
	if err != nil {
		return "", nil, err
	}
	slices.Reverse(ahead)
	return tip, ahead, nil
}

// replayRequest builds the destructive gate's request for a records replay of target, refusing a tracked change on any of touched.
func replayRequest(target string, ownership pathOwnership, touched []string) pathRequest {
	slices.Sort(touched)
	return pathRequest{
		what:      "replay local commits onto origin",
		container: filepath.Dir(target),
		target:    target,
		ownership: ownership,
		dirtiness: dirtyTrackedOn(slices.Compact(touched)),
		force:     false,
	}
}

// replayStop fills outcome from a stopped replay's error, and returns the error unchanged.
func replayStop(outcome ReplayOutcome, err error) (ReplayOutcome, error) {
	var conflict *replayConflict
	if errors.As(err, &conflict) {
		outcome.Conflicts = conflict.paths
	}
	return outcome, err
}

// replayWeft brings a weft worktree's branch level with upstream when both moved, under its caller's locks.
// It refuses with ErrReplayDirty, naming the paths, a tracked change on a path the local commits touch, leaving the branch unmoved.
// A local commit origin lacks whose every path lies under an ended run's record directory is kept under a local set-aside tag instead of replayed.
// The tags are made before the branch moves, so a stop leaves a tag on a commit still on the branch.
// The other commits are replayed onto upstream oldest first, a commit that becomes empty being dropped, and a conflict returns ErrReplayConflict with the starting tip restored.
// The ended set is first narrowed to the runs this machine's own landing has not reached.
func replayWeft(rec *Mutations, l *lyxcwd.Location, worktree, upstream string, ended EndedSet) (ReplayOutcome, error) {
	repo := gitrepo.New(worktree)
	upstreamSHA, err := repo.ResolveSHA(upstream)
	if err != nil {
		return ReplayOutcome{}, err
	}
	_, ahead, err := replayAhead(repo, upstreamSHA)
	if err != nil || len(ahead) == 0 {
		return ReplayOutcome{}, err
	}

	live := withoutFinalizeEntered(l, ended)
	var outcome ReplayOutcome
	var kept, touched []string
	for _, sha := range ahead {
		detail, err := repo.CommitDetail(sha)
		if err != nil {
			return ReplayOutcome{}, err
		}
		// A set-aside commit's paths count too: the reset onto upstream drops that commit, so git refuses over a dirty file under it as well.
		touched = append(touched, detail.Paths...)
		if slug, aside := endedRunSlug(l, live, detail.Paths); aside {
			outcome.SetAside = append(outcome.SetAside, SetAsideCommit{Commit: sha, Slug: slug})
			continue
		}
		kept = append(kept, sha)
	}

	// The set-aside commits are tagged before the branch moves, so a later stop never leaves one reachable only through the reflog.
	if err := tagSetAside(rec, repo, outcome.SetAside); err != nil {
		return outcome, err
	}
	req := replayRequest(worktree, ownedWarpCheckout(worktree), touched)
	dropped, err := cherryPickOnto(rec, req, repo, upstreamSHA, kept)
	if err != nil {
		return replayStop(outcome, err)
	}
	outcome.Dropped = dropped
	for _, sha := range kept {
		if !slices.Contains(dropped, sha) {
			outcome.Replayed = append(outcome.Replayed, sha)
		}
	}
	return outcome, nil
}

// endedRunSlug returns the slug of the first ended run a commit of paths belongs to, and whether every path lies under an ended run's record directory.
func endedRunSlug(l *lyxcwd.Location, ended EndedSet, paths []string) (string, bool) {
	if len(paths) == 0 || len(ended) == 0 {
		return "", false
	}
	first := ""
	for _, path := range paths {
		slug, under := runDirectorySlug(l, ended, path)
		if !under {
			return "", false
		}
		if first == "" {
			first = slug
		}
	}
	return first, true
}

// tagSetAside puts a local set-aside tag on each commit of asides and fills in the tag names.
func tagSetAside(rec *Mutations, repo *gitrepo.Repo, asides []SetAsideCommit) error {
	for i := range asides {
		tag, err := setAsideTag(rec, repo, asides[i].Slug, asides[i].Commit)
		if err != nil {
			return err
		}
		asides[i].Tag = tag
	}
	return nil
}

// setAsideTag puts the local tag `set-aside/<slug>/<short sha>` on sha, and returns its name.
// The tag prefix is one no archive reader matches, so a set-aside commit shows as no run.
// A tag already on sha is kept.
// It appends KindTagCreated, with the repository root as the target and the tag name as the detail, only after the tag exists.
func setAsideTag(rec *Mutations, repo *gitrepo.Repo, slug, sha string) (string, error) {
	if slug == "" {
		slug = "board"
	}
	tag := setAsideTagPrefix + slug + "/" + sha[:archiveTagSHALen]
	commonDir, err := repo.CommonDir()
	if err != nil {
		return "", fmt.Errorf("tag set-aside commit %s: %w", sha, err)
	}
	existing, err := gitexec.Run([]string{"for-each-ref", "--format=%(objectname)", "refs/tags/" + tag}, commonDir)
	if err != nil {
		return "", fmt.Errorf("look up set-aside tag %q: %w", tag, err)
	}
	switch existing = strings.TrimSpace(existing); {
	case existing == "":
		if _, err := gitexec.Run([]string{"tag", tag, sha}, commonDir); err != nil {
			return "", fmt.Errorf("create set-aside tag %q at %s: %w", tag, sha, err)
		}
	case existing != sha:
		return "", fmt.Errorf("set-aside tag %q already exists at %s, not %s", tag, existing, sha)
	}
	rec.Append(KindTagCreated, filepath.Dir(commonDir), tag)
	return tag, nil
}

// boardDesignSlug returns the slug of the entry a design file belongs to, and reports whether path is a design file:
// `design-<slug>.md` at the board's root.
func boardDesignSlug(path string) (string, bool) {
	name, found := strings.CutPrefix(path, boardDesignPrefix)
	if !found || strings.Contains(path, "/") {
		return "", false
	}
	slug, found := strings.CutSuffix(name, boardDesignSuffix)
	return slug, found && slug != ""
}

// isBoardPath reports whether path is one a board commit changes: the entry store, the rendered overview or a design file.
func isBoardPath(path string) bool {
	if path == boardFileName || path == boardOverviewFile {
		return true
	}
	_, design := boardDesignSlug(path)
	return design
}

// boardEntriesBySlug decodes the raw entries of a `board.json`, keyed by slug and compacted so two writes of the same entry compare equal.
// An absent or empty file holds none.
// It reads only the entries' slugs, never their meaning, which belongs to the board.
func boardEntriesBySlug(data []byte) (map[string]string, error) {
	entries := map[string]string{}
	if len(bytes.TrimSpace(data)) == 0 {
		return entries, nil
	}
	var file struct {
		Entries []json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	for _, raw := range file.Entries {
		var key struct {
			Slug string `json:"slug"`
		}
		if err := json.Unmarshal(raw, &key); err != nil {
			return nil, err
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err != nil {
			return nil, err
		}
		entries[key.Slug] = compact.String()
	}
	return entries, nil
}

// blobAt returns the bytes of rel at rev, nil when the file is absent there, and nil for an empty rev.
func blobAt(repo *gitrepo.Repo, rev, rel string) ([]byte, error) {
	if rev == "" {
		return nil, nil
	}
	data, err := repo.FileAtRevision(rev, rel)
	if errors.Is(err, gitrepo.ErrPathNotAtRevision) {
		return nil, nil
	}
	return data, err
}

// boardCommitSlugs returns the slugs of the entries a board-only commit changes: each entry of `board.json` that differs from the commit's parent, and each design file's entry.
func boardCommitSlugs(repo *gitrepo.Repo, sha, parent string, paths []string) ([]string, error) {
	slugs := map[string]bool{}
	for _, path := range paths {
		if slug, design := boardDesignSlug(path); design {
			slugs[slug] = true
		}
	}
	if slices.Contains(paths, boardFileName) {
		before, err := blobAt(repo, parent, boardFileName)
		if err != nil {
			return nil, err
		}
		after, err := blobAt(repo, sha, boardFileName)
		if err != nil {
			return nil, err
		}
		beforeEntries, err := boardEntriesBySlug(before)
		if err != nil {
			return nil, fmt.Errorf("parent %s of %s: %w", boardFileName, sha, err)
		}
		afterEntries, err := boardEntriesBySlug(after)
		if err != nil {
			return nil, fmt.Errorf("%s of %s: %w", boardFileName, sha, err)
		}
		for slug, entry := range afterEntries {
			if beforeEntries[slug] != entry {
				slugs[slug] = true
			}
		}
		for slug := range beforeEntries {
			if _, kept := afterEntries[slug]; !kept {
				slugs[slug] = true
			}
		}
	}
	result := make([]string, 0, len(slugs))
	for slug := range slugs {
		result = append(result, slug)
	}
	sort.Strings(result)
	return result, nil
}

// localBoardEntries returns the slugs of the entries the local commits ahead change, sorted, and the created stamp each carries at head.
// A commit's overview file is not an entry, and a slug absent at head carries no stamp.
func localBoardEntries(repo *gitrepo.Repo, head string, ahead []string) (slugs []string, created map[string]string, err error) {
	seen := map[string]bool{}
	for _, sha := range ahead {
		detail, err := repo.CommitDetail(sha)
		if err != nil {
			return nil, nil, err
		}
		entryPaths := slices.DeleteFunc(slices.Clone(detail.Paths), func(path string) bool { return !isBoardPath(path) || path == boardOverviewFile })
		if len(entryPaths) == 0 {
			continue
		}
		parents, err := repo.CommitParents(sha)
		if err != nil {
			return nil, nil, err
		}
		parent := ""
		if len(parents) > 0 {
			parent = parents[0]
		}
		changed, err := boardCommitSlugs(repo, sha, parent, entryPaths)
		if err != nil {
			return nil, nil, err
		}
		for _, slug := range changed {
			seen[slug] = true
		}
	}
	for slug := range seen {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	if len(slugs) == 0 {
		return nil, nil, nil
	}

	replay, err := InstalledBoardReplay()
	if err != nil {
		return nil, nil, err
	}
	data, err := blobAt(repo, head, boardFileName)
	if err != nil {
		return nil, nil, err
	}
	entries, err := replay.Entries(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%s at %s: %w", boardFileName, head, err)
	}
	created = map[string]string{}
	for _, entry := range entries {
		if seen[entry.Slug] && !entry.Created.IsZero() {
			created[entry.Slug] = entry.Created.UTC().Format(time.RFC3339)
		}
	}
	return slugs, created, nil
}

// hubLocationOfBoard resolves the location of the hub prime worktree whose board checkout is boardDir.
// The prime is the warp slug of the records repository's main worktree.
func hubLocationOfBoard(boardDir string) (*lyxcwd.Location, error) {
	entries, err := List(boardDir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.Main {
			continue
		}
		slug, ok := WeftWarpSlug(filepath.Base(filepath.Clean(filepath.FromSlash(entry.Path))))
		if !ok {
			return nil, fmt.Errorf("the main worktree %s of the records repository is no weft sibling", entry.Path)
		}
		return lyxcwd.ResolveWorktree(filepath.Join(filepath.Dir(boardDir), slug))
	}
	return nil, fmt.Errorf("the records repository of %s lists no main worktree", boardDir)
}

// boardStep is one local board commit a replay re-applies, by entry or by cherry-pick.
type boardStep struct {
	sha     string
	message string
	paths   []string
	parent  string
	byEntry bool
}

// replayBolt brings the board checkout level with upstream when both moved, as replayWeft does for a weft, under its caller's locks.
// A local commit is set aside under a tag when every entry and design file it changes belongs to an ended entry, the overview file not counted.
// A seed commit is dropped, as the seed-commit drop does.
// A commit that touches only the board's own files is re-applied by entry through the installed board replay onto upstream,
// so the overview and an entry only one side changed never conflict, and no entry change stops the replay.
// A commit that lost a value or met a refusal is committed for what was kept, and its original kept under a set-aside tag.
// Any other commit is replayed by cherry-pick, and its conflict returns ErrReplayConflict with the starting tip restored.
// Every set-aside commit is tagged before the branch moves or as its replay reports it, and a later failure restores the starting tip;
// a restore git refuses, as over a file the failed replay had already written, is joined into the returned error.
// The ended set is first narrowed to the runs this machine's own landing has not reached.
func replayBolt(rec *Mutations, l *lyxcwd.Location, boardDir, upstream string, ended EndedSet) (ReplayOutcome, error) {
	repo := gitrepo.New(boardDir)
	upstreamSHA, err := repo.ResolveSHA(upstream)
	if err != nil {
		return ReplayOutcome{}, err
	}
	start, ahead, err := replayAhead(repo, upstreamSHA)
	if err != nil || len(ahead) == 0 {
		return ReplayOutcome{}, err
	}
	mergeBase, err := repo.MergeBase(start, upstreamSHA)
	if err != nil {
		return ReplayOutcome{}, err
	}

	live := withoutFinalizeEntered(l, ended)
	var outcome ReplayOutcome
	var picks []string
	var steps []boardStep
	var seed, touched []string
	for _, sha := range ahead {
		detail, err := repo.CommitDetail(sha)
		if err != nil {
			return ReplayOutcome{}, err
		}
		touched = append(touched, detail.Paths...)
		if IsSeedCommit(detail.Message, detail.Paths) {
			seed = append(seed, sha)
			continue
		}
		if !slices.ContainsFunc(detail.Paths, func(path string) bool { return !isBoardPath(path) }) && len(detail.Paths) > 0 {
			parents, err := repo.CommitParents(sha)
			if err != nil {
				return ReplayOutcome{}, err
			}
			parent := ""
			if len(parents) > 0 {
				parent = parents[0]
			}
			slugs, err := boardCommitSlugs(repo, sha, parent, detail.Paths)
			if err != nil {
				return ReplayOutcome{}, err
			}
			if len(slugs) > 0 && !slices.ContainsFunc(slugs, func(slug string) bool { _, ended := live[slug]; return !ended }) {
				outcome.SetAside = append(outcome.SetAside, SetAsideCommit{Commit: sha, Slug: slugs[0]})
				continue
			}
			steps = append(steps, boardStep{sha: sha, message: detail.Message, paths: detail.Paths, parent: parent, byEntry: true})
			continue
		}
		picks = append(picks, sha)
		steps = append(steps, boardStep{sha: sha})
	}

	// The commits classification set aside are tagged before the branch moves, so a later stop never leaves one reachable only through the reflog.
	if err := tagSetAside(rec, repo, outcome.SetAside); err != nil {
		return outcome, err
	}
	req := replayRequest(boardDir, ownedBoardWorktree(), touched)
	dropped, err := cherryPickOnto(rec, req, repo, upstreamSHA, picks)
	if err != nil {
		return replayStop(outcome, err)
	}
	// Past the pick the branch sits on upstream, so every failure from here puts the starting tip back.
	restoreOnFailure := func(cause error) (ReplayOutcome, error) {
		if err := restoreReplayTip(rec, repo, start); err != nil {
			return outcome, errors.Join(cause, err)
		}
		outcome.Replayed, outcome.Dropped = nil, nil
		return outcome, cause
	}
	outcome.Dropped = append(outcome.Dropped, dropped...)
	outcome.Dropped = append(outcome.Dropped, seed...)
	afterPicks, err := repo.CurrentSHA()
	if err != nil {
		return restoreOnFailure(err)
	}
	baseline, err := repo.WorktreeChangedFiles()
	if err != nil {
		return restoreOnFailure(err)
	}
	for _, step := range steps {
		if !step.byEntry {
			if !slices.Contains(dropped, step.sha) {
				outcome.Replayed = append(outcome.Replayed, step.sha)
			}
			continue
		}
		replay, err := InstalledBoardReplay()
		if err != nil {
			return restoreOnFailure(err)
		}
		if err := replayBoardCommit(rec, replay, repo, boardDir, mergeBase, baseline, step, &outcome); err != nil {
			return restoreOnFailure(err)
		}
	}

	if len(seed) > 0 {
		rec.Append(KindCommitsDropped, boardDir, strings.Join(seed, " "))
	}
	endTip, err := repo.CurrentSHA()
	if err != nil {
		return restoreOnFailure(err)
	}
	if endTip != afterPicks {
		rec.Append(KindRepoAdvanced, boardDir, endTip)
	}
	return outcome, nil
}

// replayBoardCommit re-applies one board-only commit by entry onto the board checked out at upstream, and records what it did in outcome.
// A commit the replay reports empty writes nothing, and is dropped unless it lost a value or met a refusal.
// A commit that lost a value or met a refusal is committed for what was kept, and its original is set aside.
// The original is tagged as soon as the replay reports it, before anything is committed.
// baseline is the board's uncommitted files before the replay, which are never staged.
func replayBoardCommit(rec *Mutations, replay BoardReplay, repo *gitrepo.Repo, boardDir, mergeBase string, baseline []string, step boardStep, outcome *ReplayOutcome) error {
	request := BoardReplayRequest{BoardDir: boardDir}
	var err error
	if request.BaseBoardJSON, err = blobAt(repo, mergeBase, boardFileName); err != nil {
		return err
	}
	if request.ParentBoardJSON, err = blobAt(repo, step.parent, boardFileName); err != nil {
		return err
	}
	if request.CommitBoardJSON, err = blobAt(repo, step.sha, boardFileName); err != nil {
		return err
	}
	for _, path := range step.paths {
		if _, design := boardDesignSlug(path); !design {
			continue
		}
		file := BoardDesignFile{Name: path}
		if file.Base, err = blobAt(repo, mergeBase, path); err != nil {
			return err
		}
		if file.Before, err = blobAt(repo, step.parent, path); err != nil {
			return err
		}
		if file.After, err = blobAt(repo, step.sha, path); err != nil {
			return err
		}
		request.DesignFiles = append(request.DesignFiles, file)
	}

	result, err := replay.Apply(request)
	if err != nil {
		return fmt.Errorf("replay board commit %s by entry: %w", step.sha, err)
	}
	outcome.Lost = append(outcome.Lost, result.Lost...)
	outcome.Refused = append(outcome.Refused, result.Refused...)
	var aside *SetAsideCommit
	switch {
	case len(result.Lost) > 0:
		aside = &SetAsideCommit{Commit: step.sha, Slug: result.Lost[0].Entry}
	case len(result.Refused) > 0:
		aside = &SetAsideCommit{Commit: step.sha, Slug: result.Refused[0].Entry}
	}
	if aside != nil {
		tag, err := setAsideTag(rec, repo, aside.Slug, aside.Commit)
		if err != nil {
			return err
		}
		aside.Tag = tag
	}
	if !result.Empty {
		changed, err := repo.WorktreeChangedFiles()
		if err != nil {
			return err
		}
		changed = slices.DeleteFunc(changed, func(path string) bool { return slices.Contains(baseline, path) })
		if _, _, err := repo.StageAndCommit(step.message, changed); err != nil {
			return fmt.Errorf("commit the replay of board commit %s: %w", step.sha, err)
		}
		outcome.Replayed = append(outcome.Replayed, step.sha)
	}
	switch {
	case aside != nil:
		outcome.SetAside = append(outcome.SetAside, *aside)
	case result.Empty:
		outcome.Dropped = append(outcome.Dropped, step.sha)
	}
	return nil
}
