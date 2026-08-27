// mergestate.go implements the on-disk per-pair merge-state record — the checkpoint every merge
// verb writes before it mutates anything and MergeAbort/MergeContinue read back to recover a
// crashed or resolving-in-progress merge — plus the foreign-state probes the sibling-verb guards
// (card 13) and the four mutating merge verbs use to detect git-level merge state fabric did not
// itself start, plus the hub-wide scan Remove uses to refuse tearing down a pair some other
// pair's merge is currently consuming.
// The record is never exported and never serialized into a public result type: warp/weft field
// naming is legal here, since fabricengine is in the Fabric Vocabulary Invariant's owner set.

package fabricengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/state"
)

// mergeStateFileName is the merge-state record's fixed filename inside the weft worktree's gitdir
// (never inside the working tree — see mergeStatePath), mirroring corrIndexFileName's placement
// precedent in index.go.
const mergeStateFileName = "fabric-merge.json"

// mergeState is the on-disk record a merge verb writes before it mutates anything and
// MergeAbort/MergeContinue/MergeInProgress read back — the checkpoint recovering a crashed or
// resolving-in-progress merge.
//
// WarpSource/WeftSource are the resolved per-side SHAs the attempt merges, recorded alongside the
// caller's branch name in Source because only the SHAs are usable as evidence later: a branch name
// can be re-pointed between the crash and the resume, while the SHA fabric handed `git merge` is
// exactly what git wrote into MERGE_HEAD and therefore exactly what a genuine conclude-commit
// carries as its second parent. sideConcludeAlreadyLanded is what consumes them.
// A record written by an older binary carries them empty; every consumer treats empty as "no
// evidence available" and refuses to make a positive claim, never as "evidence satisfied".
//
// WeftStart, WeftSource, WeftOutcome and WeftCommitted now record the weft as unmoved rather than as
// a merge participant. WeftStart is the weft HEAD at record time; WeftSource is the best-effort
// resolved weft counterpart SHA, legitimately empty when no counterpart resolves; WeftOutcome is
// written as mergeOutcomeAlreadyUpToDate before any MergeStart call runs; WeftCommitted stays empty
// on every path this binary produces. The fields are kept rather than dropped for two reasons:
// mergeAttemptIncompleteReason refuses a resume on an empty WeftOutcome, and filling them leaves the
// persisted JSON schema byte-compatible in both directions across the binary change.
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
// The weft conjunct is now always satisfied, since WeftOutcome is written as
// mergeOutcomeAlreadyUpToDate before any weft MergeStart call runs — so the derived
// MergeResult.AlreadyUpToDate answers about the warp side alone.
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

// mergeSourceInFlight reports whether any pair in l's hub holds a merge record whose Source is
// warpBranch — that is, whether some pair elsewhere in the hub is mid-merge ON this branch.
// mergeBlocksMutation answers the other question, "is THIS pair mid-merge", and the two are
// genuinely different subjects: tearing a pair down while a merge somewhere else is consuming its
// branches deletes the weft branch that merge is resolving against, with no warning to the operator
// that they just removed the subject of a merge in progress.
// Records live at <weft gitdir>/fabric-merge.json, which is the weft repo's own .git for the prime
// pair and .git/worktrees/<name>/ for every linked pair, so both shapes are globbed. The linked shape
// is the common one — a merge normally runs in a task pair, and the prime worktree is the exception —
// so both shapes carry their own test (TestMergeCrucible_RemoveRefuses…, prime and linked), each
// asserting the record's on-disk location before asserting the refusal. Only the prime shape was
// covered until round 7: disabling the linked glob left the whole suite green. Each candidate
// is read through internal/state's locked reader, exactly as loadMergeState does — never a raw
// os.ReadFile, which could observe a torn record another process is mid-write on.
// A hub whose weft repo root cannot be resolved reports false rather than blocking, matching
// mergeBlocksMutation's own already-half-broken-pair rule: refusing there would make a broken hub
// impossible to finish tearing down.
func mergeSourceInFlight(l *lyxcwd.Location, warpBranch string) (bool, error) {
	weftRepoRoot, err := WeftRepoRoot(l)
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

// foreignMergeStatePresent reports whether git-level merge state exists on either side that fabric
// did not itself start: a live MERGE_HEAD or unmerged index entries, checked on both warp and weft.
// All four probes are evaluated unconditionally before combining, rather than short-circuiting on
// the first true result, so no evaluation-order timing difference ever leaks which side (if either)
// carries the state.
//
// The two probe KINDS are not redundant spellings of one condition, and neither can be dropped as
// implied by the other. An ordinary conflicted `git merge` sets both at once, which is what makes
// that shape useless for proving either one; the two states that separate them are real and reachable:
//   - MERGE_HEAD live with an EMPTY unmerged set — a foreign merge resolved but not concluded, seen
//     only by the MERGE_HEAD probe, and the most dangerous shape because nothing about the worktree
//     looks wrong.
//   - Unmerged entries with NO MERGE_HEAD — a conflicted `git merge --squash`, which writes none, and
//     equally a conflicted cherry-pick or `checkout -m`. Seen only by the conflicted-index probe.
//
// Each of the four probes is pinned by its own row of
// TestMergeVerbs_ForeignMergeState_EverySideAndShapeRefuses (three shapes x two sides). Before that
// matrix existed, deleting either weft probe, or either warp probe, left the whole suite green.
//
// Both weft probes are deliberately kept even though the weft is no longer a merge participant: this
// is the one weft-reading guard the change leaves in place, and it still refuses a mutating merge
// verb on weft-side foreign merge state.
func (f *Fabric) foreignMergeStatePresent() (bool, error) {
	warpMergeHead, err := f.warp.MergeHeadPresent()
	if err != nil {
		return false, fmt.Errorf("fabricengine: check merge head: %w", err)
	}
	warpConflicted, err := f.warp.ConflictedFiles()
	if err != nil {
		return false, fmt.Errorf("fabricengine: check conflicted files: %w", err)
	}
	weftMergeHead, err := f.weft.MergeHeadPresent()
	if err != nil {
		return false, fmt.Errorf("fabricengine: check merge head: %w", err)
	}
	weftConflicted, err := f.weft.ConflictedFiles()
	if err != nil {
		return false, fmt.Errorf("fabricengine: check conflicted files: %w", err)
	}

	return warpMergeHead || len(warpConflicted) > 0 || weftMergeHead || len(weftConflicted) > 0, nil
}
