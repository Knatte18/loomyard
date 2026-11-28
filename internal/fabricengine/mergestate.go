// mergestate.go holds the on-disk per-pair merge-state record.
// Merge verbs checkpoint it before they mutate anything.
// It also holds the probes that detect git-level merge state fabric did not start.
// It also holds the hub-wide scan that Remove consults before tearing down a pair.
// The record is never exported or serialized into a result type, so warp and weft field names are allowed here.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/state"
)

// mergeStateFileName is the merge-state record's fixed filename inside the weft worktree's gitdir.
// The record never lives in the working tree; see mergeStatePath.
// The correspondence index uses the same gitdir placement.
const mergeStateFileName = "fabric-merge.json"

// mergeState is the on-disk record a merge verb writes before it mutates anything.
// MergeAbort, MergeContinue and MergeInProgress read it back to recover a crashed or
// resolving-in-progress merge.
//
// WarpSource/WeftSource are the resolved per-side SHAs the attempt merges, recorded alongside the
// caller's branch name in Source because only the SHAs are usable as evidence later.
// A branch name can be re-pointed between the crash and the resume.
// The SHA fabric handed to `git merge` is what git wrote into MERGE_HEAD,
// so it is exactly what a genuine conclude-commit carries as its second parent.
// sideConcludeAlreadyLanded consumes them.
// A record without them, written by an older binary, reads them as empty.
// Every consumer treats empty as "no evidence available" and refuses to make a positive claim;
// none treats it as "evidence satisfied".
//
// The weft is not a merge participant; its four fields record it as unmoved.
// WeftStart is the weft HEAD at record time.
// WeftSource is the best-effort resolved weft counterpart SHA, legitimately empty when no counterpart resolves.
// WeftOutcome is written as mergeOutcomeAlreadyUpToDate before any MergeStart call runs.
// WeftCommitted stays empty on every path this binary produces.
// The fields are kept for two reasons.
// mergeAttemptIncompleteReason refuses a resume on an empty WeftOutcome.
// Keeping them also leaves the persisted JSON schema byte-compatible in both directions across the binary change.
type mergeState struct {
	Verb          string    `json:"verb"`   // "merge-in" | "merge"
	Source        string    `json:"source"` // caller-supplied branch
	Squash        bool      `json:"squash"`
	Message       string    `json:"message"`
	WarpStart     string    `json:"warp_start"` // pre-merge HEAD SHAs
	WeftStart     string    `json:"weft_start"`
	WarpSource    string    `json:"warp_source"` // the resolved SHA each side actually merges
	WeftSource    string    `json:"weft_source"`
	WarpOutcome   string    `json:"warp_outcome"` // staged|conflicted|fast_forwarded|up_to_date
	WeftOutcome   string    `json:"weft_outcome"`
	WarpCommitted string    `json:"warp_committed"` // conclude SHAs, set as each lands
	WeftCommitted string    `json:"weft_committed"`
	StartedAt     time.Time `json:"started_at"`
}

// The mergeState outcome strings a gitrepo.MergeOutcome maps onto, recorded in WarpOutcome and
// WeftOutcome.
const (
	mergeOutcomeStaged          = "staged"
	mergeOutcomeConflicted      = "conflicted"
	mergeOutcomeFastForwarded   = "fast_forwarded"
	mergeOutcomeAlreadyUpToDate = "up_to_date"
)

// mergeOutcomeString maps a gitrepo.MergeOutcome to the fixed on-disk outcome string mergeState
// records.
func mergeOutcomeString(outcome gitrepo.MergeOutcome) string {
	switch outcome {
	case gitrepo.MergeStaged:
		return mergeOutcomeStaged
	case gitrepo.MergeConflicted:
		return mergeOutcomeConflicted
	case gitrepo.MergeFastForwarded:
		return mergeOutcomeFastForwarded
	case gitrepo.MergeAlreadyUpToDate:
		return mergeOutcomeAlreadyUpToDate
	default:
		return ""
	}
}

// landedConcludeCommit reports whether the conclude phase has put a commit on either side of this merge.
// It reads the record's own conclude-SHA fields, which concludeMergeSides writes as each side lands.
// The flag answers whether the pair carries this merge's conclude-commit.
// It does not answer whether control reached the end of the happy path.
// A merge that fast-forwarded both sides fabricates no commit and reports false.
// A resumed MergeContinue whose sides both landed on an earlier call reports true, since the pair does carry them.
func (st *mergeState) landedConcludeCommit() bool {
	return st.WarpCommitted != "" || st.WeftCommitted != ""
}

// bothSidesAlreadyUpToDate reports whether the attempt found both sides already carrying the resolved source.
// This is the degenerate no-op, observed after the write lock is taken rather than by the pre-lock probe.
// The two answers differ only when another process lands the same merge in between.
// The lock does not close that race, since the pre-lock probe runs unlocked by design.
// Deriving the result from here makes the loser of that race report what a sequential run would report.
// The weft conjunct always holds.
// WeftOutcome is written as mergeOutcomeAlreadyUpToDate before any weft MergeStart call runs.
// So the result answers about the warp side alone.
func (st *mergeState) bothSidesAlreadyUpToDate() bool {
	return st.WarpOutcome == mergeOutcomeAlreadyUpToDate && st.WeftOutcome == mergeOutcomeAlreadyUpToDate
}

// mergeStatePath returns the merge-state record's on-disk path: the fixed mergeStateFileName
// inside the weft worktree's gitdir, the same placement the correspondence index uses.
func (f *Fabric) mergeStatePath() (string, error) {
	gitDir, err := f.weftGitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(gitDir, mergeStateFileName), nil
}

// loadMergeState reads the merge-state record, returning (nil, nil) when no record exists and a
// decoded record otherwise.
// A corrupt file is a hard error, never silently adopted as "no merge in progress".
// Adopting it that way would mask a real recovery need behind a guard answer that looks clean.
// Reads and writes go through internal/state's locked, atomic-replace helpers under <path>.lock.
// The correspondence index does the same in this weft gitdir.
// Never use raw json.Marshal or os.WriteFile.
// Their truncate-then-write leaves a window where a concurrent reader observes a torn or empty file.
// Such a reader is the sibling verbs' guards, which read this record from other processes without a shared lock.
// It then hits the corrupt-file error spuriously.
func (f *Fabric) loadMergeState() (*mergeState, error) {
	path, err := f.mergeStatePath()
	if err != nil {
		return nil, err
	}

	st, found, err := state.ReadJSON[mergeState](path, path+".lock")
	if err != nil {
		return nil, fmt.Errorf("fabricengine: read merge state %s: %w", path, err)
	}
	if !found {
		return nil, nil
	}
	return &st, nil
}

// saveMergeState writes st as the merge-state record, atomically and under lock, via
// state.WriteJSON.
func (f *Fabric) saveMergeState(st *mergeState) error {
	path, err := f.mergeStatePath()
	if err != nil {
		return err
	}
	if err := state.WriteJSON(path, path+".lock", *st); err != nil {
		return fmt.Errorf("fabricengine: write merge state %s: %w", path, err)
	}
	return nil
}

// deleteMergeState removes the merge-state record, tolerating its absence.
// internal/state has no delete primitive, so this call uses os.Remove directly.
// The correspondence index's own removal does the same.
func (f *Fabric) deleteMergeState() error {
	path, err := f.mergeStatePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("fabricengine: delete merge state %s: %w", path, err)
	}
	return nil
}

// mergeRecordExists reports whether a fabric-written merge-state record exists — a thin wrapper
// over loadMergeState for callers that only need the boolean.
func (f *Fabric) mergeRecordExists() (bool, error) {
	st, err := f.loadMergeState()
	if err != nil {
		return false, err
	}
	return st != nil, nil
}

// mergeBlocksMutation reports whether a fabric merge-state record exists for the pair at warpPath and weftPath.
// Mutating topology verbs use it to decide whether a write would collide with a live merge.
// It builds the pair's state probe from the explicit paths via newPaired, not from an open *Fabric.
// Topology verbs act on a *lymcwd.Location and never open a Fabric handle.
// A pair newPaired cannot open (missing warp or weft directory) reports false rather than blocking.
// So does a pair whose weft gitdir cannot be resolved.
// No recorded merge can exist without a readable weft gitdir.
// Refusing here would make a half-broken pair impossible to finish tearing down.
// It would also shadow a caller's own missing-path refusal behind an unrelated ErrMissingPath.
func mergeBlocksMutation(warpPath, weftPath string) (bool, error) {
	f, err := newPaired(warpPath, weftPath)
	if err != nil {
		return false, nil
	}
	if _, gitDirErr := f.weftGitDir(); gitDirErr != nil {
		return false, nil
	}
	return f.mergeRecordExists()
}

// mergeSourceInFlight reports whether a merge record anywhere in the hub names warpBranch as its source.
// A merge elsewhere in the hub can consume a pair's branches.
// Tearing that pair down then deletes the weft branch that merge resolves against.
// Nothing warns the operator that a merge in progress has lost its subject.
// Records live at <weft gitdir>/fabric-merge.json for the prime pair, which is the weft repo's own .git.
// Linked pairs keep theirs under .git/worktrees/<name>/.
// Both shapes are globbed, since a merge normally runs in a linked task pair rather than the prime one.
// Each candidate is read through internal/state's locked reader.
// A raw os.ReadFile could observe a record that another process is mid-write on.
// A hub whose weft repo root cannot be resolved reports false rather than blocking:
// refusing there would make a broken hub impossible to finish tearing down.
func mergeSourceInFlight(l *lymcwd.Location, warpBranch string) (bool, error) {
	weftRepoRoot, err := RecordsRepoRoot(l)
	if err != nil {
		return false, nil
	}

	primeRecord := filepath.Join(weftRepoRoot, ".git", mergeStateFileName)
	linkedRecords, err := filepath.Glob(filepath.Join(weftRepoRoot, ".git", "worktrees", "*", mergeStateFileName))
	if err != nil {
		return false, fmt.Errorf("fabricengine: scan merge records under %s: %w", weftRepoRoot, err)
	}

	for _, path := range append([]string{primeRecord}, linkedRecords...) {
		st, found, err := state.ReadJSON[mergeState](path, path+".lock")
		if err != nil {
			return false, fmt.Errorf("fabricengine: read merge state %s: %w", path, err)
		}
		if found && st.Source == warpBranch {
			return true, nil
		}
	}
	return false, nil
}

// foreignProbeReadings holds the four raw git-level readings:
// whether MERGE_HEAD is live on each side, and each side's conflicted-path list.
type foreignProbeReadings struct {
	warpMergeHead  bool
	warpConflicted []string
	weftMergeHead  bool
	weftConflicted []string
}

// readForeignProbes runs the four git-level merge-state probes: MERGE_HEAD and conflicted files on each side.
// It evaluates all four unconditionally rather than short-circuiting on the first true result,
// so no evaluation-order timing difference leaks which side (if either) carries the state.
//
// The two probe kinds are not redundant, and neither implies the other.
// An ordinary conflicted `git merge` sets both at once, so that shape cannot prove either one.
// Two states separate them, and both are reachable:
//   - MERGE_HEAD live with an empty unmerged set: a foreign merge resolved but not concluded.
//     Only the MERGE_HEAD probe sees it.
//     It is the most dangerous shape, because nothing about the worktree looks wrong.
//   - Unmerged entries with no MERGE_HEAD: a conflicted `git merge --squash`, which writes none.
//     A conflicted cherry-pick or `checkout -m` also writes none.
//     Only the conflicted-index probe sees these.
//
// The weft probes stay even though the weft takes no part in a merge.
// They are the one weft-side guard, and they refuse a mutating merge verb while weft carries foreign merge state.
func (f *Fabric) readForeignProbes() (foreignProbeReadings, error) {
	var r foreignProbeReadings
	var err error
	if r.warpMergeHead, err = f.code.MergeHeadPresent(); err != nil {
		return foreignProbeReadings{}, fmt.Errorf("fabricengine: check merge head: %w", err)
	}
	if r.warpConflicted, err = f.code.ConflictedFiles(); err != nil {
		return foreignProbeReadings{}, fmt.Errorf("fabricengine: check conflicted files: %w", err)
	}
	if r.weftMergeHead, err = f.records.MergeHeadPresent(); err != nil {
		return foreignProbeReadings{}, fmt.Errorf("fabricengine: check merge head: %w", err)
	}
	if r.weftConflicted, err = f.records.ConflictedFiles(); err != nil {
		return foreignProbeReadings{}, fmt.Errorf("fabricengine: check conflicted files: %w", err)
	}
	return r, nil
}

// foreignMergeStatePresent reports whether git-level merge state exists on either side that fabric
// did not itself start: a live MERGE_HEAD or unmerged index entries, checked on both warp and weft.
func (f *Fabric) foreignMergeStatePresent() (bool, error) {
	r, err := f.readForeignProbes()
	if err != nil {
		return false, err
	}
	return r.warpMergeHead || len(r.warpConflicted) > 0 || r.weftMergeHead || len(r.weftConflicted) > 0, nil
}

// MergeBlocked reports whether a fabric merge record exists.
// It also reports true when git-level merge state that fabric did not start sits on either side.
func (f *Fabric) MergeBlocked() (bool, error) {
	recordExists, err := f.mergeRecordExists()
	if err != nil {
		return false, err
	}
	if recordExists {
		return true, nil
	}
	return f.foreignMergeStatePresent()
}
