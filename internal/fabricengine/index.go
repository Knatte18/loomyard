// index.go — the fabric layer's git-owning wrapper around the git-free
// corrindex.go component. Per the correspondence index layering decision,
// corrindex.go's corrIndex type never touches git; this file is the only
// place in fabricengine that resolves the weft worktree's gitdir, computes a
// warp SHA's ordering sequence, or scans weft history for Warp-SHA trailers.
// It is also where the exported Fabric methods (RecordCorrespondence,
// WeftSHAForWarpSHA, RebuildIndex) that SyncWeft and RevertWithWeft (a later
// batch) build on live.

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

// corrIndexFileName is the correspondence index's fixed filename inside the
// weft worktree's gitdir (never inside the working tree — see weftGitDir).
const corrIndexFileName = "fabric-corrindex.json"

// warpSHATrailerFormatUnitSep and warpSHATrailerFormatRecordSep are the
// control-character separators RebuildIndex's git-log format string uses to
// delimit, respectively, a commit SHA from its trailer value and one commit
// record from the next. Control characters (never legal in a SHA or a
// trailer value) guarantee the split can never be confused by ordinary
// commit content.
const (
	warpSHATrailerFormatUnitSep   = "\x1f"
	warpSHATrailerFormatRecordSep = "\x1e"
)

// ErrNoCorrespondence is returned by WeftSHAForWarpSHA (and, via
// classifyCorrespondence in a later batch, by RevertWithWeft) when the
// correspondence index has no entry — exact or nearest-older — for a
// requested warp SHA at all, as opposed to ErrStaleSHA's "an entry exists but
// no longer resolves" case.
var ErrNoCorrespondence = errors.New("fabricengine: no recorded warp<->weft correspondence")

// ErrStaleSHA is returned when a correspondence index entry names a weft SHA
// that no longer exists in the weft repo — even after one RebuildIndex
// self-correction attempt — meaning weft's own history was rewritten out
// from under the recorded correspondence (rebase/amend/force-push). Per the
// stale-SHA handling decision, fabric never auto-recovers from this; it
// surfaces the typed error and leaves recovery to the caller.
var ErrStaleSHA = errors.New("fabricengine: stale SHA in correspondence index")

// weftGitDir resolves the git directory backing f.Weft's worktree via
// `git rev-parse --git-dir`, absolutized against the weft path when git
// reports a relative one (the common case for a standard checkout). In a
// linked worktree this names the per-worktree gitdir, not the shared common
// dir — deliberately, since the correspondence index is scoped per
// host<->weft pair, not shared across every worktree of the same weft clone.
func (f *Fabric) weftGitDir() (string, error) {
	stdout, stderr, code, err := gitexec.RunGit([]string{"rev-parse", "--git-dir"}, f.weftPath)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("fabricengine: git rev-parse --git-dir in %s: %s", f.weftPath, stderr)
	}

	dir := strings.TrimSpace(stdout)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(f.weftPath, dir)
	}
	return dir, nil
}

// corrIndexPath returns the correspondence index's on-disk path: the fixed
// corrIndexFileName inside the weft worktree's gitdir.
func (f *Fabric) corrIndexPath() (string, error) {
	gitDir, err := f.weftGitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(gitDir, corrIndexFileName), nil
}

// warpSeq returns warpSHA's first-parent commit count in the warp repo, via
// `git rev-list --count --first-parent <sha>` — the ordering key the
// correspondence index sorts by for its binary-search "nearest older"
// lookup.
func (f *Fabric) warpSeq(warpSHA string) (int, error) {
	stdout, stderr, code, err := gitexec.RunGit([]string{"rev-list", "--count", "--first-parent", warpSHA}, f.warpPath)
	if err != nil {
		return 0, err
	}
	if code != 0 {
		return 0, fmt.Errorf("fabricengine: git rev-list --count --first-parent %s in %s: %s", warpSHA, f.warpPath, stderr)
	}

	seq, err := strconv.Atoi(strings.TrimSpace(stdout))
	if err != nil {
		return 0, fmt.Errorf("fabricengine: parse rev-list --count output %q: %w", stdout, err)
	}
	return seq, nil
}

// RecordCorrespondence upserts an entry mapping warpSHA to weftSHA into the
// correspondence index, computing warpSHA's WarpSeq via warpSeq. Callers
// call this alongside every weft commit that carries a Warp-SHA trailer, so
// the index stays current without a rebuild; RebuildIndex remains the
// self-correcting fallback when a lookup's cached answer turns out stale.
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

// WeftSHAForWarpSHA returns the weft SHA recorded as corresponding to
// warpSHA. On an index miss it returns wrapped ErrNoCorrespondence. On a
// cache hit whose weft SHA no longer exists in the weft repo, it runs
// RebuildIndex once — the index's self-correction path, since the trailer
// history the rebuild scans remains the sole source of truth — and retries;
// if the rebuilt answer still fails to resolve, it returns wrapped
// ErrStaleSHA naming both the requested warp SHA and the weft SHA that no
// longer exists.
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
	if f.Weft.SHAExists(entry.WeftSHA) {
		return entry.WeftSHA, nil
	}

	// The cached weft SHA no longer resolves. Rebuild once from the trailer
	// history (the source of truth) before declaring the entry stale — the
	// cache itself may simply be behind, e.g. the detached CLI push path
	// records a pre-push SHA that a rebase-recovered push later rewrote.
	staleWeftSHA := entry.WeftSHA
	if err := f.RebuildIndex(); err != nil {
		return "", err
	}
	ix, err = loadCorrIndex(path)
	if err != nil {
		return "", err
	}
	if entry, ok = ix.exact(warpSHA); ok && f.Weft.SHAExists(entry.WeftSHA) {
		return entry.WeftSHA, nil
	}

	return "", fmt.Errorf("%w: warp SHA %s, weft SHA %s", ErrStaleSHA, warpSHA, staleWeftSHA)
}

// warpSHATrailerCommit is one weft commit's SHA paired with the Warp-SHA
// trailer value it carries, as scanned by scanWarpSHATrailers.
type warpSHATrailerCommit struct {
	weftSHA string
	warpSHA string
}

// scanWarpSHATrailers scans the weft repo's current branch history for every
// commit carrying a Warp-SHA trailer, via one `git log` invocation using
// git's own trailers-extracting format placeholder (`%(trailers:...)`) —
// the accepted one-pass implementation, rather than parsing each commit
// message by hand. Commits are returned newest-first, matching `git log`'s
// natural order; commits without a Warp-SHA trailer are omitted.
func (f *Fabric) scanWarpSHATrailers() ([]warpSHATrailerCommit, error) {
	format := "%H" + warpSHATrailerFormatUnitSep +
		"%(trailers:key=" + WarpSHATrailerKey + ",valueonly)" +
		warpSHATrailerFormatRecordSep

	stdout, stderr, code, err := gitexec.RunGit([]string{"log", "--format=" + format}, f.weftPath)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		// An unborn HEAD (a fresh weft branch with zero commits) is not a
		// genuine scan failure — it just has empty trailer history — so it
		// yields no commits rather than an error.
		if strings.Contains(stderr, "does not have any commits yet") {
			return nil, nil
		}
		return nil, fmt.Errorf("fabricengine: git log --format trailers in %s: %s", f.weftPath, stderr)
	}

	var commits []warpSHATrailerCommit
	for _, record := range strings.Split(stdout, warpSHATrailerFormatRecordSep) {
		// Each record is bracketed by newlines the trailers placeholder and
		// git log's own per-entry formatting insert; trim them before
		// splitting the fields.
		record = strings.Trim(record, "\n")
		if record == "" {
			continue
		}

		parts := strings.SplitN(record, warpSHATrailerFormatUnitSep, 2)
		weftSHA := strings.TrimSpace(parts[0])
		if weftSHA == "" {
			continue
		}
		var warpSHA string
		if len(parts) > 1 {
			warpSHA = strings.TrimSpace(parts[1])
		}
		if warpSHA == "" {
			// No Warp-SHA trailer on this commit — not every weft commit
			// carries one (e.g. history predating fabric, or a manual
			// commit), so it is simply omitted rather than treated as an
			// error.
			continue
		}

		commits = append(commits, warpSHATrailerCommit{weftSHA: weftSHA, warpSHA: warpSHA})
	}
	return commits, nil
}

// refreshCorrIndexAfterSwitch discards and rebuilds the pair's correspondence
// index after a coordinated branch switch. The index file is per-worktree, so
// it survives Checkout moving the pair onto another branch — and entries
// recorded on the previous branch keep passing SHAExists (the commits still
// exist on the other branch's refs), which means the stale-hit self-correction
// in WeftSHAForWarpSHA/resolveRevertTarget never fires and lookups can serve
// weft SHAs the current branch's trailer history (the sole source of truth)
// would never produce — a RevertWithWeft against such an answer would graft
// the current branches onto the other branch's history. Deleting the file
// first makes the refresh fail-safe: if the rebuild then errors, lookups miss
// honestly (ErrNoCorrespondence) instead of answering cross-branch.
func refreshCorrIndexAfterSwitch(worktreeRoot, weftWorktree string) error {
	f, err := New(worktreeRoot, weftWorktree)
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

// RebuildIndex reconstructs the correspondence index from scratch by
// scanning the current weft branch's Warp-SHA trailer history — the sole
// source of truth the index is a derived cache over — and atomically
// replacing the on-disk index file with the result. A trailer value that
// fails f.Warp.SHAExists (the warp commit it names no longer exists) is
// still recorded, per the stale-SHA handling decision: staleness surfaces at
// use (WeftSHAForWarpSHA, RevertWithWeft), never here at rebuild time.
func (f *Fabric) RebuildIndex() error {
	path, err := f.corrIndexPath()
	if err != nil {
		return err
	}

	commits, err := f.scanWarpSHATrailers()
	if err != nil {
		return err
	}

	// scanWarpSHATrailers returns newest-first; walk oldest-to-newest so a
	// warp SHA recorded by more than one weft commit (a rare but legal
	// history shape) converges on the same "last recorded wins" result an
	// incremental RecordCorrespondence build would have produced, matching
	// corrIndex.record's own upsert semantics.
	byWarpSHA := make(map[string]corrEntry, len(commits))
	var order []string
	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]
		if _, seen := byWarpSHA[c.warpSHA]; !seen {
			order = append(order, c.warpSHA)
		}

		seq, err := f.warpSeq(c.warpSHA)
		if err != nil {
			// The trailer names a warp SHA that no longer exists (history
			// rewrite). Record the entry anyway with an unknown-position
			// sentinel rather than dropping it — a caller resolving against
			// this entry validates it with f.Warp.SHAExists before ever
			// trusting it, so an imprecise sort position here cannot mask a
			// real staleness bug; it only affects where an already-broken
			// entry sits in the "nearest older" ordering.
			seq = 0
		}
		byWarpSHA[c.warpSHA] = corrEntry{WarpSHA: c.warpSHA, WeftSHA: c.weftSHA, WarpSeq: seq}
	}

	entries := make([]corrEntry, 0, len(order))
	for _, warpSHA := range order {
		entries = append(entries, byWarpSHA[warpSHA])
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].WarpSeq < entries[j].WarpSeq })

	// Write the freshly scanned entry set directly, rather than replaying it
	// through corrIndex.record one entry at a time, so the rebuild is one
	// atomic file replace instead of a sequence of writes that would
	// transiently expose a partially-rebuilt index to a concurrent reader.
	return state.WriteJSON(path, path+".lock", entries)
}
