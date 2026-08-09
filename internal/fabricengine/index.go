// index.go — the fabric layer's git-owning wrapper around the git-free corrindex.go component.
// Per the correspondence index layering decision, corrindex.go's corrIndex type never touches git;
// this file is the only place in fabricengine that resolves the weft worktree's gitdir, computes a
// warp SHA's ordering sequence, or scans weft history for Warp-SHA trailers.
// It is also where the exported Fabric methods (RecordCorrespondence, WeftSHAForWarpSHA,
// RebuildIndex) that Fabric.Commit and Fabric.Diff build on live.

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

// ErrNoCorrespondence is returned by WeftSHAForWarpSHA (and, via classifyCorrespondence, by
// resolveRevertTarget's Fabric.Diff caller) when the correspondence index has no entry — exact or
// nearest-older — for a requested warp SHA at all, as opposed to ErrStaleSHA's "an entry exists but
// no longer resolves" case.
var ErrNoCorrespondence = errors.New("fabricengine: no recorded warp<->weft correspondence")

// ErrStaleSHA is returned when a correspondence index entry names a weft SHA that no longer exists
// in the weft repo — even after one RebuildIndex self-correction attempt — meaning weft's own
// history was rewritten out from under the recorded correspondence (rebase/amend/force-push).
// Per the stale-SHA handling decision, fabric never auto-recovers from this;
// it surfaces the typed error and leaves recovery to the caller.
var ErrStaleSHA = errors.New("fabricengine: stale SHA in correspondence index")

// weftGitDir resolves the git directory backing f.weft's worktree via
// `git rev-parse --git-dir`, absolutized against the weft path when git
// reports a relative one (the common case for a standard checkout). In a
// linked worktree this names the per-worktree gitdir, not the shared common
// dir — deliberately, since the correspondence index is scoped per
// warp<->weft pair, not shared across every worktree of the same weft clone.
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

// RecordCorrespondence upserts an entry mapping warpSHA to weftSHA into the correspondence index,
// computing warpSHA's WarpSeq via warpSeq.
// Callers call this alongside every weft commit that carries a Warp-SHA trailer, so the index stays
// current without a rebuild;
// RebuildIndex remains the self-correcting fallback when a lookup's cached answer turns out stale.
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
// On an index miss it returns wrapped ErrNoCorrespondence.
// On a cache hit whose weft SHA no longer exists in the weft repo, it runs RebuildIndex once — the
// index's self-correction path, since the trailer history the rebuild scans remains the sole source
// of truth — and retries;
// if the rebuilt answer still fails to resolve, it returns wrapped ErrStaleSHA naming both the
// requested warp SHA and the weft SHA that no longer exists.
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
	if f.weft.SHAExists(entry.WeftSHA) {
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
	if entry, ok = ix.exact(warpSHA); ok && f.weft.SHAExists(entry.WeftSHA) {
		return entry.WeftSHA, nil
	}

	return "", fmt.Errorf("%w: warp SHA %s, weft SHA %s", ErrStaleSHA, warpSHA, staleWeftSHA)
}

// warpSHATrailerCommit is one weft commit's SHA paired with the Warp-SHA
// trailer value it carries and every Snapshot trailer value alongside it, as
// scanned by scanWarpSHATrailers. snapshotTags is nil (not empty-non-nil)
// when the commit carries no Snapshot trailer at all — the common case.
type warpSHATrailerCommit struct {
	weftSHA      string
	warpSHA      string
	snapshotTags []string
}

// scanWarpSHATrailers scans the weft repo's current branch history for every
// commit carrying a Warp-SHA trailer, via one `git log` invocation using
// git's own trailers-extracting format placeholder (`%(trailers:...)`) — the
// accepted one-pass implementation, rather than parsing each commit message
// by hand. This is the single generalized scan the trailer-is-truth-no-new-
// cache Shared Decision calls for: it captures the commit's Snapshot trailer
// values alongside its Warp-SHA value in the same pass, so snapshotWarpSHA
// (snapshot.go) and RebuildIndex share one git-log plumbing site, one copy of
// the unit/record separator convention, and one copy of the unborn-HEAD
// tolerance, rather than each spawning its own scan.
//
// Commits are returned in **topological order** (via --topo-order), never in
// git's plain reverse-chronological default: a snapshot commit can arrive on
// this weft branch from another machine through Pull, and that other
// machine's wall-clock commit date is not trustworthy relative to this
// history's own commits — a skewed clock could stamp an older baseline with a
// newer date, and under either RebuildIndex's dedup or
// snapshotWarpSHA's newest-wins lookup, a date-ordered scan would then
// pick the older baseline and under-report staleness, the one failure
// direction that loses data. Topological order guarantees no commit is ever
// listed before one of its own descendants, so "first in the scan" reliably
// means "newest, tie-broken by ancestry" regardless of clock skew. Commits
// without a Warp-SHA trailer are omitted — see parseTrailerScanRecord.
func (f *Fabric) scanWarpSHATrailers() ([]warpSHATrailerCommit, error) {
	format := "%H" + warpSHATrailerFormatUnitSep +
		"%(trailers:key=" + WarpSHATrailerKey + ",valueonly)" + warpSHATrailerFormatUnitSep +
		"%(trailers:key=" + SnapshotTrailerKey + ",valueonly)" +
		warpSHATrailerFormatRecordSep

	stdout, stderr, code, err := gitexec.RunGit([]string{"log", "--topo-order", "--format=" + format}, f.weftPath)
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
		weftSHA, warpSHA, snapshotTags, ok := parseTrailerScanRecord(record)
		if !ok {
			continue
		}
		commits = append(commits, warpSHATrailerCommit{weftSHA: weftSHA, warpSHA: warpSHA, snapshotTags: snapshotTags})
	}
	return commits, nil
}

// parseTrailerScanRecord parses one warpSHATrailerFormatRecordSep-delimited
// record produced by scanWarpSHATrailers's git-log format string into its
// three unit-separated fields — the commit's own SHA, its Warp-SHA trailer
// value, and its Snapshot trailer value(s) — with no I/O and no git spawn, so
// it is unit-testable in Tier 1 the way the git-spawning scan loop around it
// is not.
//
// record is trimmed of surrounding newlines first: git's own per-entry
// formatting inserts one, and the trailers placeholder inserts more whenever
// a trailer value itself spans lines. The trimmed record is then split on
// warpSHATrailerFormatUnitSep into at most three fields via
// strings.SplitN(record, sep, 3); a record shorter than three fields (e.g. a
// commit with no trailers at all, or an empty record) still parses whichever
// fields it does have rather than panicking on a missing slice index.
//
// A commit carrying more than one "Snapshot:" trailer renders as a
// MULTI-LINE value for that one placeholder — one line per trailer
// occurrence — so the snapshot field is itself split on "\n" into individual
// tags, each trimmed of surrounding whitespace, with empty lines dropped.
//
// ok reports false, and weftSHA/warpSHA/snapshotTags are left at their zero
// values, for an empty record or one whose Warp-SHA field is empty: a
// snapshot record with no recorded baseline is not usable by
// snapshotWarpSHA any more than an index record with no warp SHA is
// usable by RebuildIndex, so the same rule skips the record for both
// consumers rather than each re-deriving it.
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
		// No Warp-SHA trailer on this commit — not every weft commit carries
		// one (e.g. history predating fabric, or a manual commit) — so it is
		// skipped rather than treated as an error.
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

// refreshCorrIndexAfterSwitch discards and rebuilds the pair's correspondence
// index after a coordinated branch switch. The index file is per-worktree, so
// it survives Checkout moving the pair onto another branch — and entries
// recorded on the previous branch keep passing SHAExists (the commits still
// exist on the other branch's refs), which means the stale-hit self-correction
// in WeftSHAForWarpSHA/resolveRevertTarget never fires and lookups can serve
// weft SHAs the current branch's trailer history (the sole source of truth)
// would never produce — a Fabric.Diff bridged against such an answer via
// weftAnchorForWarpSHA would graft the current branches onto the other
// branch's history. Deleting the file
// first makes the refresh fail-safe: if the rebuild then errors, lookups miss
// honestly (ErrNoCorrespondence) instead of answering cross-branch.
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

// RebuildIndex reconstructs the correspondence index from scratch by scanning the current weft
// branch's Warp-SHA trailer history — the sole source of truth the index is a derived cache over —
// and atomically replacing the on-disk index file with the result.
// A trailer value that fails f.warp.SHAExists (the warp commit it names no longer exists) is still
// recorded, per the stale-SHA handling decision: staleness surfaces at use (WeftSHAForWarpSHA,
// resolveRevertTarget), never here at rebuild time.
func (f *Fabric) RebuildIndex() error {
	path, err := f.corrIndexPath()
	if err != nil {
		return err
	}

	commits, err := f.scanWarpSHATrailers()
	if err != nil {
		return err
	}

	// scanWarpSHATrailers returns commits in TOPOLOGICAL order (newest-first,
	// no commit ever listed before one of its own descendants — see its doc
	// comment for why --topo-order matters over git's date-ordered default);
	// walk oldest-to-newest so a warp SHA recorded by more than one weft
	// commit converges on the same "last recorded wins" result an incremental
	// RecordCorrespondence build would have produced, matching
	// corrIndex.record's own upsert semantics.
	//
	// A warp SHA recorded by more than one weft commit is no longer a rare
	// edge case once the empty-commit rule (tags-force-a-weft-commit) exists:
	// the warp SHA does not move between a content commit and a subsequent
	// tags-only or unchanged-content call at the same warp HEAD, so an empty
	// commit's RecordCorrespondence(warpSHA, emptyWeftSHA) routinely upserts
	// over the entry a preceding content commit wrote for that same warp SHA.
	// WeftSHAForWarpSHA(warpSHA) and resolveRevertTarget(warpSHA) then resolve
	// to the empty commit — accepted, not worked around, because an empty
	// commit's tree is identical to its parent's by construction, so
	// bridging a diff anchor to it reaches the same weft tree the content
	// commit produced; resolveRevertTarget uses the resolved weft SHA only
	// as a validation/bridge target (f.weft.SHAExists) and does nothing else
	// with it, so the overwrite changes no other observable behaviour. Two
	// alternatives were rejected rather than merely unconsidered. Skipping
	// RecordCorrespondence for empty
	// commits would make the incremental and rebuilt indexes diverge, since
	// this very rebuild reads trailers (not this call site's choices) and
	// would record the commit anyway. Special-casing the index to keep the
	// content commit as the winner would make the index disagree with a
	// plain trailer scan, breaking the trailer-is-truth/index-is-a-
	// rebuildable-cache layering the whole design rests on — "last recorded
	// wins" is already corrIndex.record's own documented upsert rule, so this
	// is existing semantics meeting a newly-common input, not new semantics.
	//
	// Two order-sensitivities fall out of this walk and are worth stating
	// rather than leaving implicit. First, the dedup below is
	// last-assignment-wins
	// over this reversed (oldest-to-newest) walk of a topologically-ordered
	// scan, so for a warp SHA recorded by more than one weft commit, the
	// winner is whichever commit the (newest-first) scan listed FIRST — i.e.
	// the topologically newest one. Second, sort.SliceStable below preserves
	// insertion order among entries sharing the same WarpSeq, which covers
	// both the seq = 0 dangling sentinel entries assigned in the loop below
	// and genuine side-branch commits sitting at equal first-parent depth.
	// In both cases the intended outcome is the same: the newest commit in
	// topological order wins — the identical rule snapshotWarpSHA
	// applies to its own scan, which is what keeps the index and the reader
	// in agreement over the same trailer history.
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
			// this entry validates it with f.warp.SHAExists before ever
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
