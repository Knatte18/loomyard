// origintypes.go holds plain result types and the pure methods that read them.
// They carry what the origin reconcile, the parent tips, the teardown, the hand-over and the records replay report.

package fabriclayout

import (
	"fmt"
	"strings"
	"time"
)

// cleanupRerun is the re-entry a printed stop of a dirty, gone or busy branch appends to its way forward.
const cleanupRerun = "then re-run lym cleanup --apply from the prime"

// ReconcileScope names what one reconcile judges.
// An empty Run is the hub scope: Bolt, the prime's code and records branches, and those of every local pair.
// A slug is the run scope: Bolt, the run's parent branch on both sides, read from the pair's origin record;
// the run's own pair is judged too.
// JudgeOnly judges and fetches objects but moves no ref, for a dry run.
// PairMayBeAbsent is meaningful only with Run set.
// It lets the run scope judge a run whose task worktree is not a registered worktree of the code repository.
// It also covers a task worktree that is registered with its directory gone.
// Such a run's entry is judged from origin's board, with no branch of its own and no parent branch.
// Otherwise the reconcile refuses the run.
type ReconcileScope struct {
	Run             string
	JudgeOnly       bool
	PairMayBeAbsent bool
}

// EndedTeardown asks the teardown seam to remove the local pair of an entry origin has ended.
// Force marks the case where the pair's uncommitted work is snapshotted and removed instead of refusing the removal.
// EarlierSnapshot is set when the weft repository already holds a snapshot tag of the slug.
// An earlier force-case run leaves such a tag.
type EndedTeardown struct {
	Slug            string
	Force           bool
	EarlierSnapshot bool
}

// The values a busy teardown waits on.
const (
	// BusyOnDriver means the pair's driver is still running.
	BusyOnDriver = "driver"
	// BusyOnLock means the teardown lock, the weft write lock or a detached push holds the pair.
	BusyOnLock = "lock"
)

// TeardownReport is what the teardown seam reports about one removal.
// It holds the tags the removal kept work under and the reasons it kept anything.
// It also holds whether the removal was busy and the error it stopped on.
// BusyOn names what a busy removal waits on, BusyOnDriver or BusyOnLock.
// It embeds MutationRecord, filled from the removal's own record.
type TeardownReport struct {
	MutationRecord
	Tags   []string
	Kept   []string
	Busy   bool
	BusyOn string
	Error  error
}

// BranchState is the verdict on one local branch against origin's head for it.
type BranchState string

// The verdicts, the first group from judging alone and the rest from the moves that follow it.
const (
	// BranchLevel means the branch and origin's head are the same commit.
	BranchLevel BranchState = "level"
	// BranchFastForward means the local tip is an ancestor of origin's head and nothing local is lost by moving to it.
	BranchFastForward BranchState = "fast_forward"
	// BranchAhead means origin's head is an ancestor of the local tip.
	BranchAhead BranchState = "ahead"
	// BranchRewound means origin reset the branch back past commits this machine holds.
	BranchRewound BranchState = "rewound"
	// BranchRewritten means origin replaced the branch's history with commits this machine never held.
	BranchRewritten BranchState = "rewritten"
	// BranchDiverged means a code branch both sides moved, or a records branch whose replay stopped on a conflict.
	BranchDiverged BranchState = "diverged"
	// BranchNeedsReplay means a records branch both sides moved, which a moving reconcile replays.
	BranchNeedsReplay BranchState = "needs_replay"
	// BranchGone means origin no longer has the branch.
	BranchGone BranchState = "gone"
	// BranchDirty means an uncommitted change blocks the move.
	BranchDirty BranchState = "dirty"
	// BranchBusy means a lock on the branch is held.
	BranchBusy BranchState = "busy"
	// BranchLive means the branch belongs to a pair that is live, which a reconcile never moves.
	BranchLive BranchState = "live"
	// BranchAnchorStopped means a weft branch was left unmoved.
	// Taking origin's tip in would cross a change of its anchor marker, or its own marker is empty.
	BranchAnchorStopped BranchState = "anchor_stopped"
)

// Stops reports whether a branch in this state stops its entry or the scope.
func (s BranchState) Stops() bool {
	switch s {
	case BranchRewound, BranchRewritten, BranchDiverged, BranchGone, BranchDirty, BranchBusy, BranchLive, BranchAnchorStopped:
		return true
	}
	return false
}

// BranchOutcome is the verdict on one judged branch, with what a reader of a stop needs.
// Repo is the repository side: "code", "records" or "bolt".
// Worktree is where the branch is checked out;
// CodeWorktree is the pair's code worktree, where every fabric verb of a records branch runs.
// Paths are the conflicting or dirty paths, and Commits those a rewind dropped or a replay stopped on.
// BusyLock names the held lock of a busy branch;
// PushLockHeld is set when it was the push lock, which a parked push holds.
// WayForward is the one line every reader of a stop takes, the repair step alone, with no re-entry verb.
// Anchor is the anchor an anchor_stopped branch's way forward names;
// it is empty when the stop is the weft's own empty marker.
// MoveWayForward is the way forward of an anchor_stopped branch whose weft carries a move record;
// it replaces the one the anchor would name.
// Moved is set by a moving reconcile once it brought the branch up to origin's head.
// State then reads level, or ahead when replayed commits sit on it.
// Warning is the text of a failed worktree event fired for a code branch the reconcile moved, empty otherwise.
type BranchOutcome struct {
	Branch         string
	Repo           string
	Slug           string
	Prime          bool
	Worktree       string
	CodeWorktree   string
	State          BranchState
	OriginHead     string
	Paths          []string
	Commits        []string
	BusyLock       string
	PushLockHeld   bool
	WayForward     string
	Anchor         string
	MoveWayForward string
	Moved          bool
	Warning        string
}

// rerunsCleanup reports whether the way forward of this stop is followed by a re-run of `lym cleanup --apply`.
func (b BranchOutcome) rerunsCleanup() bool {
	switch b.State {
	case BranchDirty, BranchGone:
		return true
	case BranchBusy:
		return !b.PushLockHeld
	}
	return false
}

// EntryState is the verdict on one entry, the run a pair carries.
type EntryState string

// The verdicts of an entry.
const (
	// EntryActive means nothing about the entry stops it.
	EntryActive EntryState = "active"
	// EntryEnded means origin's board shows the entry done or removed.
	EntryEnded EntryState = "ended"
	// EntryStopped means a branch of the entry or a records stop keeps it from carrying on.
	EntryStopped EntryState = "stopped"
	// EntryOwnedElsewhere means origin's board records another machine as the entry's owner.
	EntryOwnedElsewhere EntryState = "owned_elsewhere"
)

// EntryOutcome is the verdict a reconcile reaches on one judged entry.
// Busy marks an ended entry whose driver or branch is busy, so its pair is not yet removed;
// WayForward then names what lets the next run remove it.
// An ended entry also carries How origin ended it, "done" or "removed";
// Local is set when this machine holds a part of its pair.
// Removed is set once the teardown removed that pair, with the Tags it kept work under and the reasons it Kept anything.
// Error is the message of a teardown that failed for any reason other than being busy.
// The pair is left for a later `lym cleanup --apply`.
// Owner names the machine that owns an owned_elsewhere entry.
// AdoptSubject is the beam or `root` that `lym adopt` takes to move it; an empty AdoptSubject reads as the slug.
// PairAbsent marks the run scope's own entry when its task worktree was not a registered worktree,
// so the entry was judged with no branch of its own.
type EntryOutcome struct {
	Slug         string
	State        EntryState
	Owner        string
	AdoptSubject string
	Busy         bool
	WayForward   string
	How          string
	Local        bool
	Removed      bool
	Tags         []string
	Kept         []string
	Error        string
	PairAbsent   bool `json:"pair_absent,omitempty"`
}

// AdoptTarget returns what `lym adopt` takes to move the entry: AdoptSubject, or the slug when it is empty.
func (e EntryOutcome) AdoptTarget() string {
	if e.AdoptSubject == "" {
		return e.Slug
	}
	return e.AdoptSubject
}

// OriginReport is the outcome of one reconcile: its branch and entry verdicts, and whether origin was reachable.
// Unreachable means a transport failure outlasted the retry bound, so nothing was judged against origin or moved.
// It embeds MutationRecord, filled by the moves.
type OriginReport struct {
	MutationRecord
	Unreachable bool
	Branches    []BranchOutcome
	Entries     []EntryOutcome
}

// Summary renders the report in a few lines for an operator.
// It names what moved, what stopped with its way forward, and which entries another machine owns.
// It also names which entries ended and whether origin was unreachable.
// A stop of a dirty, gone or busy-on-a-write-lock branch is followed by the re-entry that retries it.
// An empty report renders as the empty string.
func (r OriginReport) Summary() string {
	var lines []string
	if r.Unreachable {
		lines = append(lines, "unreachable: origin could not be reached, so nothing was judged against it or moved")
	}

	var moved []string
	for _, branch := range r.Branches {
		if branch.Moved {
			moved = append(moved, branch.Branch)
		}
	}
	if len(moved) > 0 {
		lines = append(lines, "moved: "+strings.Join(moved, ", "))
	}

	for _, branch := range r.Branches {
		if branch.Warning != "" {
			lines = append(lines, fmt.Sprintf("warning: %s: %s", branch.Branch, branch.Warning))
		}
	}

	for _, branch := range r.Branches {
		if !branch.State.Stops() {
			continue
		}
		line := fmt.Sprintf("stopped: %s (%s): %s", branch.Branch, branch.State, branch.WayForward)
		if branch.rerunsCleanup() {
			line += "; " + cleanupRerun
		}
		lines = append(lines, line)
	}

	for _, entry := range r.Entries {
		if entry.State == EntryOwnedElsewhere {
			lines = append(lines, fmt.Sprintf("owned elsewhere: %s by %s; `lym adopt %s` takes it over", entry.Slug, entry.Owner, entry.AdoptTarget()))
		}
	}

	for _, entry := range r.Entries {
		if entry.PairAbsent {
			lines = append(lines, fmt.Sprintf("pair absent: %s (%s); its task worktree is gone, so it was judged from origin's board alone", entry.Slug, entry.State))
		}
	}

	var ended []string
	for _, entry := range r.Entries {
		switch {
		case entry.State != EntryEnded || !entry.Local:
		case entry.Busy:
			lines = append(lines, fmt.Sprintf("ended, not yet removed: %s: %s", entry.Slug, entry.WayForward))
		case entry.Error != "":
			lines = append(lines, fmt.Sprintf("ended, not yet removed: %s: %s; a later `lym cleanup --apply` retries the removal", entry.Slug, entry.Error))
		default:
			ended = append(ended, entry.Slug)
		}
	}
	if len(ended) > 0 {
		lines = append(lines, "ended: "+strings.Join(ended, ", "))
	}
	return strings.Join(lines, "\n")
}

// ParentRelation names how a branch's local tip stands to its upstream tip.
type ParentRelation string

const (
	// ParentEqual means the local and upstream tips are the same commit.
	ParentEqual ParentRelation = "equal"
	// ParentBehind means local is an ancestor of upstream, so a fast-forward reaches upstream.
	ParentBehind ParentRelation = "behind"
	// ParentAhead means upstream is an ancestor of local.
	ParentAhead ParentRelation = "ahead"
	// ParentDiverged means each tip holds a commit the other lacks.
	ParentDiverged ParentRelation = "diverged"
	// ParentNoUpstream means the branch tracks no remote branch, or the tracked one was never fetched.
	ParentNoUpstream ParentRelation = "no-upstream"
)

// ParentTips is a branch's local and upstream tips and how they relate.
type ParentTips struct {
	// Local is the SHA of refs/heads/<branch>.
	Local string
	// Upstream is the SHA of the remote-tracking ref the branch tracks, empty when Relation is ParentNoUpstream.
	Upstream string
	// Relation is how Local stands to Upstream.
	Relation ParentRelation
}

// FastForwardRefusal names why a fast-forward left the pair unmoved without returning an error.
type FastForwardRefusal string

const (
	// FastForwardDirty means the code worktree carries uncommitted tracked changes.
	FastForwardDirty FastForwardRefusal = "dirty"
	// FastForwardMergeInProgress means a fabric merge record exists on the pair.
	FastForwardMergeInProgress FastForwardRefusal = "merge-in-progress"
	// FastForwardNotFastForward means the local head is not an ancestor of the freshly fetched upstream.
	FastForwardNotFastForward FastForwardRefusal = "not-fast-forward"
)

// FastForwardResult reports what a fast-forward did to the pair.
// It embeds MutationRecord, which is empty on a refusal.
type FastForwardResult struct {
	MutationRecord
	// Refusal is set when the pair was left unmoved, and is empty otherwise.
	Refusal FastForwardRefusal
	// NewHead is the upstream SHA the code side moved to, empty when it did not move.
	NewHead string
}

// CodeTipReading is what a read of a run's code tip on origin found.
type CodeTipReading struct {
	// Tip is the code tip the run's last transition on origin names, empty when origin holds no such transition.
	Tip string
	// Missing reports whether origin's fetched code branch lacks Tip.
	Missing bool
	// Machine is the machine of the last history entry of the run's child record at that transition.
	Machine string
	// At is the commit time of that transition.
	At time.Time
}

// PendingTeardown is the record of a remote teardown handed to a detached child.
// The command writes it before the handover, and the child removes it once the job has run with no failed step.
// A record whose slug's teardown lock no process holds therefore names a teardown that did not finish:
// its child ended before the job did, killed outright for instance, or a remote step of the job failed.
// Either can leave the pair's branches on origin.
// A record whose pair is gone everywhere stays until it is settled.
type PendingTeardown struct {
	Slug     string    `json:"slug"`
	Kind     string    `json:"kind"`
	HandedAt time.Time `json:"handed_at"`
	// Worktree is the code worktree the teardown ran from.
	Worktree string `json:"worktree,omitempty"`
	// WeftTip is the local records tip the teardown tagged before it deleted the branch, empty when it found none.
	WeftTip string `json:"weft_tip,omitempty"`
	// TaskTip is the local task tip a discard tagged before it deleted the branch, empty when it found none.
	TaskTip string `json:"task_tip,omitempty"`
	// Rerun is the command whose re-run retries a discard or a removal, empty when its caller named none.
	Rerun string `json:"rerun,omitempty"`
	// WayForward is what finishes the teardown.
	// A record on disk holds one only once a remote step failed; a listed record always holds one.
	WayForward string `json:"way_forward,omitempty"`
}

// HandOverMark records that this machine handed a run over to the machine of a takeover entry.
type HandOverMark struct {
	// Machine is the machine the takeover entry names.
	Machine string `json:"machine"`
	// At is the time the takeover entry was recorded.
	At string `json:"at"`
	// Written is the time this machine wrote the mark.
	Written time.Time `json:"written"`
}

// LedgerRun is one run as the run ledger records it.
// Created is the zero time when the run's creation is unknown.
type LedgerRun struct {
	Slug    string
	Ended   bool
	Created time.Time
}

// ReplayFile is one Bolt file a local commit touches, with its bytes at the merge base, before the commit and after it.
// Path is relative to the Bolt root and slash-separated,
// and a nil slice means the file is absent at that point.
type ReplayFile struct {
	Path   string
	Base   []byte
	Before []byte
	After  []byte
}

// ReplayRequest is one local commit's changes to a replayer's files, to re-apply onto origin's tip.
// Dir is the Bolt checkout at origin's tip, and Files holds the commit's files that replayer claims.
type ReplayRequest struct {
	Dir   string
	Files []ReplayFile
}

// ReplayFinding names one entry-level or file-level change a replay did not apply and why.
// Field is empty when the whole entry is concerned.
// Local is the local side's value of the field when it was lost to origin's.
type ReplayFinding struct {
	Entry  string
	Field  string
	Reason string
	Local  string
}

// ReplayResult reports what a replayer applied.
// Empty is true when no change survived, so nothing was written.
// Lost holds the local values origin's side won, and Refused the changes the replayer's own checks refused.
type ReplayResult struct {
	Empty   bool
	Lost    []ReplayFinding
	Refused []ReplayFinding
}
