// mergestate.go implements the on-disk per-pair merge-state record — the checkpoint every merge
// verb writes before it mutates anything and MergeAbort/MergeContinue read back to recover a
// crashed or resolving-in-progress merge — plus the foreign-state probes the sibling-verb guards
// (card 13) and the four mutating merge verbs use to detect git-level merge state fabric did not
// itself start.
// The record is never exported and never serialized into a public result type: warp/weft field
// naming is legal here, since fabricengine is in the Fabric Vocabulary Invariant's owner set.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/state"
)

// mergeStateFileName is the merge-state record's fixed filename inside the weft worktree's gitdir
// (never inside the working tree — see mergeStatePath), mirroring corrIndexFileName's placement
// precedent in index.go.
const mergeStateFileName = "fabric-merge.json"

// mergeState is the on-disk record a merge verb writes before it mutates anything and
// MergeAbort/MergeContinue/MergeInProgress read back — the checkpoint recovering a crashed or
// resolving-in-progress merge.
type mergeState struct {
	Verb          string    `json:"verb"`   // "merge-in" | "merge"
	Source        string    `json:"source"` // caller-supplied branch
	Squash        bool      `json:"squash"`
	Message       string    `json:"message"`
	WarpStart     string    `json:"warp_start"` // pre-merge HEAD SHAs
	WeftStart     string    `json:"weft_start"`
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

// landedConcludeCommit reports whether the conclude phase has actually put a commit on either side
// of this merge — read straight off the record's own conclude-SHA fields, which concludeMergeSides
// writes as each side lands.
// It is what MergeResult.Committed is derived from, so the flag answers "does the pair now carry
// this merge's conclude-commit" rather than "did control reach the end of the happy path". A merge
// that fast-forwarded both sides fabricates no commit and reports false; a resumed MergeContinue
// whose sides both landed on an earlier call reports true, since the pair does carry them.
func (st *mergeState) landedConcludeCommit() bool {
	return st.WarpCommitted != "" || st.WeftCommitted != ""
}

// bothSidesAlreadyUpToDate reports whether the attempt found both sides already carrying the
// resolved source — the degenerate no-op, observed after the write lock was taken rather than by the
// pre-lock probe.
// The two answers differ exactly when another process landed the same merge in between, which is a
// race the lock deliberately does not close (the pre-lock probe is unlocked by design); deriving
// MergeResult.AlreadyUpToDate from here makes the loser of that race report what a strictly
// sequential run of the same two calls reports.
func (st *mergeState) bothSidesAlreadyUpToDate() bool {
	return st.WarpOutcome == mergeOutcomeAlreadyUpToDate && st.WeftOutcome == mergeOutcomeAlreadyUpToDate
}

// mergeStatePath returns the merge-state record's on-disk path: the fixed mergeStateFileName
// inside the weft worktree's gitdir, mirroring the correspondence index's own placement
// (index.go's corrIndexPath).
func (f *Fabric) mergeStatePath() (string, error) {
	gitDir, err := f.weftGitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(gitDir, mergeStateFileName), nil
}

// loadMergeState reads the merge-state record, returning (nil, nil) when no record exists and a
// decoded record otherwise.
// A corrupt file is a hard error, never silently adopted as "no merge in progress" — that would
// mask a real recovery need behind a guard answer that looks clean.
// Reads and writes go through internal/state's locked, atomic-replace helpers under <path>.lock,
// exactly as corrindex.go does for the correspondence index in this same weft gitdir — never raw
// json.Marshal/os.WriteFile, whose truncate-then-write leaves a window where a concurrent reader
// (card 13's sibling-verb guards, which read this record from other processes with no shared lock)
// can observe a torn or empty file and hit this corrupt-file clause spuriously.
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
// internal/state has no delete primitive, so this one call stays raw os.Remove, matching index.go's
// own precedent (refreshCorrIndexAfterSwitch).
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

// mergeBlocksMutation reports whether a fabric merge-state record exists for the pair at warpPath
// and weftPath — the single check every sibling mutating verb (card 13) uses to decide whether its
// own write would corrupt or be corrupted by a live merge.
// It constructs the pair's state probe from the explicit paths via newPaired internally, rather than
// requiring an already-open *Fabric, since Topology verbs (Checkout, Remove) act on a *lyxcwd.Location
// and never open a Fabric handle of their own.
// A pair newPaired cannot even open (missing warp or weft directory) or whose weft gitdir cannot be
// resolved (e.g. a half-torn-down pair) reports false rather than blocking: no recorded merge can
// exist without a readable weft gitdir, and refusing here would make an already half-broken pair
// impossible to finish tearing down, or would shadow a caller's own, more specific "this path does
// not exist" refusal behind an unrelated ErrMissingPath.
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

// foreignMergeStatePresent reports whether git-level merge state exists on either side that fabric
// did not itself start: a live MERGE_HEAD or unmerged index entries, checked on both warp and weft.
// All four probes are evaluated unconditionally before combining, rather than short-circuiting on
// the first true result, so no evaluation-order timing difference ever leaks which side (if either)
// carries the state.
func (f *Fabric) foreignMergeStatePresent() (bool, error) {
	warpMergeHead, err := f.warp.MergeHeadPresent()
	if err != nil {
		return false, fmt.Errorf("fabricengine: check warp merge head: %w", err)
	}
	warpConflicted, err := f.warp.ConflictedFiles()
	if err != nil {
		return false, fmt.Errorf("fabricengine: check warp conflicted files: %w", err)
	}
	weftMergeHead, err := f.weft.MergeHeadPresent()
	if err != nil {
		return false, fmt.Errorf("fabricengine: check weft merge head: %w", err)
	}
	weftConflicted, err := f.weft.ConflictedFiles()
	if err != nil {
		return false, fmt.Errorf("fabricengine: check weft conflicted files: %w", err)
	}

	return warpMergeHead || len(warpConflicted) > 0 || weftMergeHead || len(weftConflicted) > 0, nil
}
