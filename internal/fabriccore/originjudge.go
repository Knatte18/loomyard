// originjudge.go holds the rules of judging local branches against origin.
// They decide the reads and retries, the verdict on each branch and on each entry, and a stopped branch's repair step.

package fabriccore

import (
	"fmt"
	"slices"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
)

const (
	// FieldReconcileTargets is the branches a reconcile judges, with the pairs and repositories they come from.
	FieldReconcileTargets Field = "reconcile_targets"
	// FieldMovedHeads is the fetch of the heads origin moved past their tracking refs.
	FieldMovedHeads Field = "moved_heads"
	// FieldBranchFacts is what is read of each judged branch against origin's head.
	FieldBranchFacts Field = "branch_facts"
)

// ReconcileFacts is what is read for judging this machine's branches against origin.
type ReconcileFacts struct {
	Scope fabriclayout.ReconcileScope
	// Targets are the judged branches.
	Targets []BranchFacts
	// Pairs are the slugs of the pairs the scope covers.
	Pairs      []string
	CodeDir    string
	RecordsDir string
	// PairAbsent reports that the scope's own pair is not there.
	PairAbsent bool
	// Attempts is how many times origin is read at most.
	Attempts      int
	RetryInterval time.Duration
	// Fetched reports that the moved heads were fetched.
	Fetched bool
	// Judged reports that each target's branch facts were read.
	Judged bool
}

// BranchFacts is one judged branch: where it lives and what is read of it.
// The local tip, the rewrite verdict and the ancestry answers are read only as far as the verdict needs them.
type BranchFacts struct {
	// Repo is the repository side: code, records or bolt.
	Repo         string
	Branch       string
	Worktree     string
	CodeWorktree string
	Slug         string
	Prime        bool
	// Tracking is the commit the tracking ref held before origin was read.
	Tracking string
	// OriginHead is the head origin holds for the branch, empty when origin has no such branch.
	OriginHead string
	LocalTip   string
	// Rewound reports that origin reset a code branch past the commits in Dropped, which this machine holds.
	Rewound bool
	Dropped []string
	// Rewritten reports that origin replaced a code branch's history with commits this machine never held.
	Rewritten bool
	// Behind reports that the local tip is an ancestor of origin's head.
	Behind bool
	// Ahead reports that origin's head is an ancestor of the local tip.
	Ahead bool
}

// moved reports whether origin holds a head the tracking ref did not.
func (f BranchFacts) moved() bool {
	return f.OriginHead != "" && f.OriginHead != f.Tracking
}

// OriginJudge is the rule that reads what judging the branches against origin needs.
// It refuses a run lacking a seam it would use before origin is read.
// A read of origin that failed in transit is made again after the retry interval, while attempts remain.
// After the last it is done with origin unread.
// Only heads origin moved are fetched.
func OriginJudge(snapshot Snapshot) Answer {
	facts := snapshot.Reconcile
	needed := []RequiredSeam{SeamRunRecordPaths, SeamStatusCodec}
	// Only a moving hub scope takes the prime's run lock.
	if facts.Scope.Run == "" && !facts.Scope.JudgeOnly {
		needed = append(needed, SeamRunLocks)
	}
	if refusal := RefuseUnsupplied(snapshot, needed...); refusal != nil {
		return Answer{Refusal: refusal}
	}

	if !snapshot.Stores[StoreCode].OriginRead {
		failures := 0
		for _, failure := range snapshot.ReadFailures {
			if failure.Field == FieldOriginRefs {
				failures++
			}
		}
		switch {
		case failures == 0:
			return Answer{Phase: PhaseOrigin, Read: []Field{FieldOriginRefs}}
		case failures < facts.Attempts:
			return Answer{Actions: []Action{waitFor(facts.RetryInterval)}, Reread: []Field{FieldOriginRefs}}
		default:
			return Answer{}
		}
	}
	if !facts.Fetched && slices.ContainsFunc(facts.Targets, BranchFacts.moved) {
		return Answer{Phase: PhaseOrigin, Read: []Field{FieldMovedHeads}}
	}
	if !facts.Judged {
		return Answer{Phase: PhaseOrigin, Read: []Field{FieldBranchFacts}}
	}
	return Answer{}
}

// JudgeBranch returns the verdict on one branch, with no way forward.
// A code branch origin reset past commits this machine holds reads rewound, which takes precedence over ahead.
// A records branch neither side contains reads needs_replay.
func JudgeBranch(facts BranchFacts) fabriclayout.BranchOutcome {
	outcome := fabriclayout.BranchOutcome{
		Repo:         facts.Repo,
		Branch:       facts.Branch,
		Worktree:     facts.Worktree,
		CodeWorktree: facts.CodeWorktree,
		Slug:         facts.Slug,
		Prime:        facts.Prime,
		OriginHead:   facts.OriginHead,
	}
	switch {
	case facts.OriginHead == "":
		outcome.State = fabriclayout.BranchGone
	case facts.LocalTip == facts.OriginHead:
		outcome.State = fabriclayout.BranchLevel
	case facts.Rewound:
		outcome.State = fabriclayout.BranchRewound
		outcome.Commits = facts.Dropped
	case facts.Behind:
		outcome.State = fabriclayout.BranchFastForward
	case facts.Ahead:
		outcome.State = fabriclayout.BranchAhead
	case facts.Repo != fabriclayout.RepoCode:
		outcome.State = fabriclayout.BranchNeedsReplay
	case facts.Rewritten:
		outcome.State = fabriclayout.BranchRewritten
	default:
		outcome.State = fabriclayout.BranchDiverged
	}
	return outcome
}

// EntryFacts are the facts an entry is judged from.
type EntryFacts struct {
	// Ended reports that origin's board shows the entry done or removed.
	Ended bool
	// OwnedElsewhere reports that origin's board records another machine as the entry's owner.
	OwnedElsewhere bool
	// Branches are the verdicts on the entry's judged branches.
	Branches []fabriclayout.BranchState
	// D3Stop reports that the records replay stopped on the prime's records branch or Bolt for this entry's sake.
	D3Stop bool
}

// ClassifyEntry turns an entry's facts into its verdict.
// An ended entry is ended whatever its branches say.
// Otherwise an entry owned elsewhere is owned_elsewhere whatever its branches say.
// Otherwise a D3 stop, or a branch that stops, makes it stopped; every other entry is active.
func ClassifyEntry(f EntryFacts) fabriclayout.EntryState {
	if f.Ended {
		return fabriclayout.EntryEnded
	}
	if f.OwnedElsewhere {
		return fabriclayout.EntryOwnedElsewhere
	}
	if f.D3Stop || slices.ContainsFunc(f.Branches, fabriclayout.BranchState.Stops) {
		return fabriclayout.EntryStopped
	}
	return fabriclayout.EntryActive
}

// ClassifyEntries returns the verdict on each of the recorded pairs, from the verdicts on its branches.
// A stop on Bolt or on the prime's records branch stops every entry, a busy one excepted.
// A busy records branch of the run scope's own pair stops nothing, since the run's own driver holds it.
// A stopped entry carries the way forward of the branch that stopped it.
// owners maps the slug of each entry another machine owns to that owner.
// subjects maps the same slug to what `lym adopt` takes to move it.
// Such an entry reads owned_elsewhere and carries the way forward that takes it over.
// That way forward names its subject, or its slug when there is none.
func ClassifyEntries(scope fabriclayout.ReconcileScope, recorded []string, branches []fabriclayout.BranchOutcome, owners, subjects map[string]string) []fabriclayout.EntryOutcome {
	d3Stop := false
	d3WayForward := ""
	for _, branch := range branches {
		shared := branch.Repo == fabriclayout.RepoBolt || (branch.Repo == fabriclayout.RepoRecords && branch.Prime)
		if shared && branch.State.Stops() && branch.State != fabriclayout.BranchBusy && !d3Stop {
			d3Stop = true
			d3WayForward = branch.WayForward
		}
	}

	var entries []fabriclayout.EntryOutcome
	for _, slug := range recorded {
		owner, ownedElsewhere := owners[slug]
		facts := EntryFacts{D3Stop: d3Stop, OwnedElsewhere: ownedElsewhere}
		wayForward := ""
		for _, branch := range branches {
			if branch.Slug != slug || (scope.Run == slug && branch.Repo == fabriclayout.RepoRecords && branch.State == fabriclayout.BranchBusy) {
				continue
			}
			facts.Branches = append(facts.Branches, branch.State)
			if wayForward == "" && branch.State.Stops() {
				wayForward = branch.WayForward
			}
		}
		state := ClassifyEntry(facts)
		entry := fabriclayout.EntryOutcome{Slug: slug, State: state}
		switch {
		case state == fabriclayout.EntryOwnedElsewhere:
			entry.Owner = owner
			entry.AdoptSubject = subjects[slug]
			wayForward = fmt.Sprintf("`lym adopt %s` from this machine's prime takes it over", entry.AdoptTarget())
		case state != fabriclayout.EntryStopped:
			wayForward = ""
		case wayForward == "":
			wayForward = d3WayForward
		}
		entry.WayForward = wayForward
		entries = append(entries, entry)
	}
	return entries
}

// BranchStopWayForward returns the repair step for a stopped branch, and "" for a state that does not stop.
// A way forward a move already set stands.
// emptyMarkerRefusal is the refusal of a blank anchor marker, empty when the marker is not blank;
// it is the step of an anchor stop that has no anchor.
func BranchStopWayForward(b fabriclayout.BranchOutcome, dirs VerbDirs, emptyMarkerRefusal string) string {
	if b.MoveWayForward != "" {
		return b.MoveWayForward
	}
	record := fabriclayout.FailedPush{Repo: b.Repo, RepoPath: b.Worktree, CodeWorktree: b.CodeWorktree, Branch: b.Branch, Prime: b.Prime}
	switch b.State {
	case fabriclayout.BranchRewound:
		return RewoundWayForward(b.Worktree, b.Branch, b.OriginHead, b.Commits, dirs)
	case fabriclayout.BranchRewritten:
		return fmt.Sprintf("`lym fabric pull` in %s", dirs.of(b.CodeWorktree))
	case fabriclayout.BranchDiverged:
		if b.Repo == fabriclayout.RepoCode {
			return CodeDivergedWayForward(b.Worktree, b.Branch, b.Prime, dirs)
		}
		return RecordsConflictWayForward(b.Worktree, b.Branch, b.Paths, b.Commits, PushStep(record, dirs))
	case fabriclayout.BranchGone:
		if b.Slug != "" {
			record.Class = fabriclayout.PushFailureGone
			return FailedPushWayForward(record, dirs)
		}
		return fmt.Sprintf("origin no longer has branch %s; run %s to publish it again", b.Branch, PushStep(record, dirs))
	case fabriclayout.BranchDirty:
		return branchDirtyWayForward(b, dirs)
	case fabriclayout.BranchBusy:
		if b.PushLockHeld && b.Slug != "" {
			return fmt.Sprintf("`lym tender resume %s`", b.Slug)
		}
		return fmt.Sprintf("wait for the holder of the %s lock to end", b.BusyLock)
	case fabriclayout.BranchLive:
		return "no step now: the run's own Parent-Align judges and moves its pair, and `lym cleanup --apply` re-judges it once the run has ended"
	case fabriclayout.BranchAnchorStopped:
		return branchAnchorStopWayForward(b, dirs, emptyMarkerRefusal)
	}
	return ""
}

// branchAnchorStopWayForward names the step that settles an anchor_stopped weft.
// A blank own marker names the marker file and its repair.
// A pair's weft names the `--set` to run in its code worktree.
// The prime's weft names the steps in order.
// First, end every reed session of the hub and wait out every live batten run, since the `--set` refuses both.
// Then run the `--set`, then start the orch again.
func branchAnchorStopWayForward(b fabriclayout.BranchOutcome, dirs VerbDirs, emptyMarkerRefusal string) string {
	if b.Anchor == "" {
		if emptyMarkerRefusal != "" {
			return emptyMarkerRefusal
		}
		return fmt.Sprintf("restore the anchor marker of %s", b.Worktree)
	}
	if b.Slug != "" {
		return fmt.Sprintf("run `lym fabric anchor %s --set %s` in %s", b.Slug, b.Anchor, dirs.of(b.CodeWorktree))
	}
	return fmt.Sprintf("end every reed session of the hub, the orch's included, with `lym reed down` in each, and wait until no batten run of the hub is live; then run `lym fabric anchor --set %s` in %s; then run `lym beam start` again", b.Anchor, dirs.of(b.CodeWorktree))
}

// branchDirtyWayForward names where each uncommitted path of a dirty branch is committed or restored.
// A records path under the record pathspec is committed by `lym fabric commit` in the pair's code worktree.
// The records sibling itself refuses every fabric verb.
// Any path, including one outside that pathspec, is committed or restored by git in the branch's worktree.
func branchDirtyWayForward(b fabriclayout.BranchOutcome, dirs VerbDirs) string {
	paths := joinOrNone(b.Paths)
	git := fmt.Sprintf("`git commit` or `git checkout -- <path>` in %s", b.Worktree)
	if b.Repo == fabriclayout.RepoRecords {
		return fmt.Sprintf("commit the uncommitted paths (%s) with `lym fabric commit` in %s, or any of them with %s", paths, dirs.of(b.CodeWorktree), git)
	}
	return fmt.Sprintf("commit or restore the uncommitted paths (%s) with %s", paths, git)
}
