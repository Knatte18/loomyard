// index.go — the fabric layer's git-owning wrapper around the git-free corrindex.go component.
// The corrIndex type in corrindex.go never touches git.
// This file is the only place in fabricengine that resolves the weft worktree's gitdir.
// Only this file computes a warp SHA's ordering sequence or scans weft history for Warp-SHA trailers.
// It also holds the exported Fabric methods RecordCorrespondence, WeftSHAForWarpSHA and RebuildIndex.
// Fabric.Commit and Fabric.Diff build on them.

package fabricengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/state"
)

// corrIndexFileName is the fixed filename of the correspondence index.
// The file lives inside the weft worktree's gitdir, never inside the working tree.
const corrIndexFileName = "fabric-corrindex.json"

// warpSHATrailerFormatUnitSep and warpSHATrailerFormatRecordSep are control-character separators.
// The git-log format string in scanWarpSHATrailers uses them.
// The unit separator delimits a commit SHA from its trailer value,
// and the record separator delimits one commit record from the next.
// Control characters are never legal in a SHA or a trailer value, so ordinary commit content cannot confuse the split.
const (
	warpSHATrailerFormatUnitSep   = "\x1f"
	warpSHATrailerFormatRecordSep = "\x1e"
)

// ErrNoCorrespondence is returned when the correspondence index has no entry for a warp SHA.
// The index holds neither an exact entry nor a nearest-older one for it.
var ErrNoCorrespondence = errors.New("fabricengine: no recorded warp<->weft correspondence")

// ErrStaleSHA is returned when a correspondence index entry names a weft SHA that no longer exists.
// It is returned even after the index is rebuilt once from the trailer history.
// It means weft's history was rewritten (by rebase, amend or force-push) under the recorded correspondence.
// Fabric never recovers from this on its own; recovery is left to the caller.
var ErrStaleSHA = errors.New("fabricengine: stale SHA in correspondence index")

// weftGitDir resolves the git directory backing f.records's worktree.
// In a linked worktree this names the per-worktree gitdir, not the shared common dir.
// The correspondence index is scoped per warp<->weft pair, not shared across every worktree of the same weft clone.
func (f *Fabric) weftGitDir() (string, error) {
	dir, err := f.records.GitDir()
	if err != nil {
		return "", fmt.Errorf("fabricengine: resolve weft gitdir in %s: %w", f.weftPath, err)
	}
	return dir, nil
}

// corrIndexPath returns the correspondence index's on-disk path:
// the fixed corrIndexFileName inside the weft worktree's gitdir.
func (f *Fabric) corrIndexPath() (string, error) {
	gitDir, err := f.weftGitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(gitDir, corrIndexFileName), nil
}

// warpSeq returns warpSHA's first-parent commit count in the warp repo.
// It runs `git rev-list --count --first-parent <sha>`.
// The count is the ordering key the correspondence index sorts by for its binary-search "nearest older" lookup.
func (f *Fabric) warpSeq(warpSHA string) (int, error) {
	stdout, err := gitexec.Run([]string{"rev-list", "--count", "--first-parent", warpSHA}, f.warpPath)
	if err != nil {
		return 0, fmt.Errorf("fabricengine: count first-parent commits for warp SHA %s in %s: %w", warpSHA, f.warpPath, err)
	}

	seq, err := strconv.Atoi(strings.TrimSpace(stdout))
	if err != nil {
		return 0, fmt.Errorf("fabricengine: parse rev-list --count output %q: %w", stdout, err)
	}
	return seq, nil
}

// RecordCorrespondence upserts an entry mapping warpSHA to weftSHA into the correspondence index.
// It computes warpSHA's WarpSeq for the entry.
// Callers call it for every weft commit that carries a Warp-SHA trailer,
// so the index stays current without a rebuild.
// A lookup whose cached answer turns out stale rebuilds the index from the trailer history.
func (f *Fabric) RecordCorrespondence(warpSHA, weftSHA string) error {
	seq, err := f.warpSeq(warpSHA)
	if err != nil {
		return err
	}

	path, err := f.corrIndexPath()
	if err != nil {
		return err
	}
	ix, err := loadCorrIndex(path)
	if err != nil {
		return err
	}

	return ix.record(corrEntry{WarpSHA: warpSHA, WeftSHA: weftSHA, WarpSeq: seq})
}

// WeftSHAForWarpSHA returns the weft SHA recorded as corresponding to warpSHA.
// On an index miss it returns an error wrapping ErrNoCorrespondence.
// On a cache hit whose weft SHA no longer exists in the weft repo, it rebuilds the index once and retries.
// The rebuild scans the trailer history, which is the sole source of truth.
// If the rebuilt answer still fails to resolve, it returns an error wrapping ErrStaleSHA.
// That error names both the requested warp SHA and the stale weft SHA.
func (f *Fabric) WeftSHAForWarpSHA(warpSHA string) (string, error) {
	path, err := f.corrIndexPath()
	if err != nil {
		return "", err
	}
	ix, err := loadCorrIndex(path)
	if err != nil {
		return "", err
	}

	entry, ok := ix.exact(warpSHA)
	if !ok {
		return "", fmt.Errorf("%w: warp SHA %s", ErrNoCorrespondence, warpSHA)
	}
	if f.records.SHAExists(entry.WeftSHA) {
		return entry.WeftSHA, nil
	}

	// The cached weft SHA no longer resolves.
	// Rebuild from the trailer history, the source of truth, before declaring the entry stale.
	// The cache itself may simply be behind.
	// For example, the detached CLI push path records a pre-push SHA that a rebase-recovered push later rewrote.
	staleWeftSHA := entry.WeftSHA
	if err := f.RebuildIndex(); err != nil {
		return "", err
	}
	ix, err = loadCorrIndex(path)
	if err != nil {
		return "", err
	}
	if entry, ok = ix.exact(warpSHA); ok && f.records.SHAExists(entry.WeftSHA) {
		return entry.WeftSHA, nil
	}

	return "", fmt.Errorf("%w: warp SHA %s, weft SHA %s", ErrStaleSHA, warpSHA, staleWeftSHA)
}

// warpSHATrailerCommit pairs one weft commit's SHA with the Warp-SHA trailer value it carries.
// It also holds every Snapshot trailer value of that commit.
// snapshotTags is nil, not an empty slice, when the commit carries no Snapshot trailer, which is the common case.
type warpSHATrailerCommit struct {
	weftSHA      string
	warpSHA      string
	snapshotTags []string
}

// scanWarpSHATrailers scans the weft repo's current branch history for every commit carrying a Warp-SHA trailer.
// It makes one `git log` call that uses git's trailers-extracting format placeholder (`%(trailers:...)`).
// That one pass replaces parsing each commit message by hand.
// It also captures each commit's Snapshot trailer values in the same pass.
// Every caller therefore shares one git-log call, one separator convention, and one unborn-HEAD tolerance.
//
// Commits are returned in topological order (`--topo-order`), not in git's reverse-chronological default.
// A snapshot commit can arrive on this weft branch from another machine through Pull.
// That machine's wall-clock commit date is not trustworthy relative to this history's own commits.
// A skewed clock could stamp an older baseline with a newer date.
// A date-ordered scan would then pick the older baseline and under-report staleness.
// That is the one failure direction that loses data.
// Topological order guarantees no commit is listed before one of its own descendants.
// So the first commit in the scan reliably means the newest, tie-broken by ancestry, regardless of clock skew.
// Commits without a Warp-SHA trailer are omitted.
func (f *Fabric) scanWarpSHATrailers() ([]warpSHATrailerCommit, error) {
	format := "%H" + warpSHATrailerFormatUnitSep +
		"%(trailers:key=" + WarpSHATrailerKey + ",valueonly)" + warpSHATrailerFormatUnitSep +
		"%(trailers:key=" + SnapshotTrailerKey + ",valueonly)" +
		warpSHATrailerFormatRecordSep

	stdout, err := gitexec.Run([]string{"log", "--topo-order", "--format=" + format}, f.weftPath)
	if err != nil {
		// An unborn HEAD, a fresh weft branch with zero commits, is not a scan failure.
		// It has empty trailer history, so it yields no commits rather than an error.
		var gitErr *gitexec.GitError
		if errors.As(err, &gitErr) && strings.Contains(gitErr.Stderr, "does not have any commits yet") {
			return nil, nil
		}
		return nil, fmt.Errorf("fabricengine: git log --format trailers in %s: %w", f.weftPath, err)
	}

	var commits []warpSHATrailerCommit
	for _, record := range strings.Split(stdout, warpSHATrailerFormatRecordSep) {
		weftSHA, warpSHA, snapshotTags, ok := parseTrailerScanRecord(record)
		if !ok {
			continue
		}
		commits = append(commits, warpSHATrailerCommit{weftSHA: weftSHA, warpSHA: warpSHA, snapshotTags: snapshotTags})
	}
	return commits, nil
}

// parseTrailerScanRecord parses one record of the git-log output that scanWarpSHATrailers reads.
// The record's fields are separated by warpSHATrailerFormatUnitSep.
// They are the commit's own SHA, its Warp-SHA trailer value, and its Snapshot trailer value(s).
// It does no I/O and spawns no git, so a unit test can cover it without a repository.
//
// The record is trimmed of surrounding newlines first.
// git's own per-entry formatting inserts one.
// The trailers placeholder inserts more whenever a trailer value itself spans lines.
// The trimmed record is then split into at most three fields on warpSHATrailerFormatUnitSep:
// strings.SplitN(record, sep, 3).
// A record with fewer than three fields parses whichever fields it has.
// It does not panic on a missing slice index.
//
// A commit carrying more than one "Snapshot:" trailer renders as a multi-line value for that placeholder.
// The snapshot field is split on "\n" into individual tags.
// Each tag is trimmed of surrounding whitespace, and empty lines are dropped.
//
// ok reports false for an empty record or one whose Warp-SHA field is empty.
// In that case weftSHA, warpSHA and snapshotTags are left at their zero values.
// A record with no warp SHA is unusable to every consumer of the scan, so this check lives here, once.
func parseTrailerScanRecord(record string) (weftSHA, warpSHA string, snapshotTags []string, ok bool) {
	record = strings.Trim(record, "\n")
	if record == "" {
		return "", "", nil, false
	}

	parts := strings.SplitN(record, warpSHATrailerFormatUnitSep, 3)
	weftSHA = strings.TrimSpace(parts[0])
	if weftSHA == "" {
		return "", "", nil, false
	}

	if len(parts) > 1 {
		warpSHA = strings.TrimSpace(parts[1])
	}
	if warpSHA == "" {
		// Not every weft commit carries a Warp-SHA trailer: history predating fabric and manual commits have none.
		// Such a commit is skipped, not treated as an error.
		return "", "", nil, false
	}

	if len(parts) > 2 {
		for _, line := range strings.Split(parts[2], "\n") {
			tag := strings.TrimSpace(line)
			if tag != "" {
				snapshotTags = append(snapshotTags, tag)
			}
		}
	}

	return weftSHA, warpSHA, snapshotTags, true
}

// refreshCorrIndexAfterSwitch discards and rebuilds the pair's correspondence index after a coordinated branch switch.
// The index file is per-worktree, so it survives a checkout that moves the pair onto another branch.
// Entries recorded on the previous branch still pass the existence check, because their commits still exist.
// Lookups would then answer with weft SHAs that the current branch's trailer history never produces,
// and a diff bridged against such an answer would graft the current branch onto the other branch's history.
// The file is deleted before the rebuild, so a failed rebuild makes lookups report no correspondence.
func refreshCorrIndexAfterSwitch(worktreeRoot, weftWorktree string) error {
	f, err := newPaired(worktreeRoot, weftWorktree)
	if err != nil {
		return err
	}
	path, err := f.corrIndexPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("fabricengine: discard correspondence index %s: %w", path, err)
	}
	return f.RebuildIndex()
}

// RebuildIndex reconstructs the correspondence index from scratch by scanning the weft branch's Warp-SHA trailers.
// That history is the sole source of truth the index is a derived cache over.
// It atomically replaces the on-disk index file with the result.
// A trailer value naming a warp commit that no longer exists is still recorded.
// Staleness surfaces at lookup time, never at rebuild time.
func (f *Fabric) RebuildIndex() error {
	path, err := f.corrIndexPath()
	if err != nil {
		return err
	}

	commits, err := f.scanWarpSHATrailers()
	if err != nil {
		return err
	}

	// scanWarpSHATrailers returns commits in topological order, newest first.
	// Walking them oldest to newest leaves a warp SHA recorded by more than one weft commit on the newest one.
	// That is the same last-recorded-wins rule as corrIndex.record's upsert.
	//
	// A warp SHA is often recorded by more than one weft commit.
	// The warp SHA does not change for a later tags-only or unchanged-content commit at the same warp HEAD.
	// The empty commit's record therefore routinely overwrites the entry of the content commit before it.
	// That is accepted: an empty commit's tree equals its parent's,
	// so a diff anchored at the empty commit reaches the same weft tree as the content commit.
	// The index must agree with the trailer scan, so the empty commit is recorded like any other.
	//
	// The dedup keeps the last assignment in the reversed walk:
	// the newest commit in topological order wins for a warp SHA.
	// sort.SliceStable keeps insertion order among entries with equal WarpSeq:
	// the seq 0 sentinel entries and side-branch commits at equal first-parent depth.
	// Both rules keep the index in agreement with its readers over one trailer history.
	byWarpSHA := make(map[string]corrEntry, len(commits))
	var order []string
	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]
		if _, seen := byWarpSHA[c.warpSHA]; !seen {
			order = append(order, c.warpSHA)
		}

		seq, err := f.warpSeq(c.warpSHA)
		if err != nil {
			// The trailer names a warp SHA that no longer exists, after a history rewrite.
			// The entry is recorded anyway, with an unknown-position sentinel (seq 0), rather than dropped.
			// Lookups check it with f.code.SHAExists before trusting it, so the imprecise position only orders the broken entry.
			seq = 0
		}
		byWarpSHA[c.warpSHA] = corrEntry{WarpSHA: c.warpSHA, WeftSHA: c.weftSHA, WarpSeq: seq}
	}

	entries := make([]corrEntry, 0, len(order))
	for _, warpSHA := range order {
		entries = append(entries, byWarpSHA[warpSHA])
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].WarpSeq < entries[j].WarpSeq })

	// Write the freshly scanned entry set directly, not by replaying it through corrIndex.record one entry at a time:
	// the rebuild is then one atomic file replace, so a concurrent reader never sees a partially rebuilt index.
	return state.WriteJSON(path, path+".lock", entries)
}
