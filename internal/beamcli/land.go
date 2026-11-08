// land.go holds the `lym beam land` and `lym beam ready` sequences, which judge the named beam.
// land applies the refusals that stop a landing of a beam not flagged ready or with work remaining under it,
// and it writes the land request the beam's pawl run consumes at Beam-Await.
// ready writes the ready record.

package beamcli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/reviewbudget"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// landDeps holds the seam of every land step: one field per read or write a step makes.
// A test substitutes the fields it needs.
// ParentOf names the slug of the pair a slug was forked from, "" for the prime.
// ChildStatus reads the status of the beam pair's own pawl run, found false when it has none.
// RunID returns the seed id of that pawl run, which a land request names.
// Ready reads whether the beam's ready record flags it.
// MarkReady writes that record unless it already flags the beam, commits it and returns its path.
// Refusal is the landing's refusal of a beam with work remaining under it.
// WriteRequest writes the land request into the beam worktree and returns its path.
// Tasks reads the board's entries.
// FileTask creates the task its fields describe and releases it.
// CodeReviewCaps reads the hub's Code-Review budget.
// Only the `merge-parent` verb calls these three.
// DraftDescription writes a starting point for the beam's change description and returns its path.
// It refuses when a description already stands.
// RecordDescription validates the change description the orch wrote and stamps the landing record's DescribedAt with at.
// It commits the landing directory and returns the description's path.
// Only the `describe` verb calls these two.
// PullRequestSide reads the beam pair's origin facts: its branch and its parent branch.
// It uses the pull request API when the origin allows one.
// PushBeam pushes the beam branch and returns the head it pushed.
// BeamHasCommits reports whether the beam holds a commit its parent branch lacks.
// Landing reads the beam's landing record, found false when it has none.
// WriteLanding updates it and commits the landing directory.
// A commit over an unchanged directory is a no-op.
// Description reads the title and body of the written change description.
// BeamHead reads the beam pair's HEAD.
// PullRequestMode reads the pull_request mode of the parent's beam.yaml.
// It also reports whether GitHub's rules for the parent branch require a pull request.
// A hub with no GitHub origin or no usable token reports that no rule requires one.
// Sleep waits d or until ctx ends.
// Now is the clock a read of a pull request's merge state keeps its retry window by.
// ParentUnmerged reports whether the parent branch's tip is not an ancestor of the beam's head.
// It is the local tip, or the upstream tip fetched when the parent has one and the hub's push skip is unset.
// DescriptionFindings lists the findings of the written change description, none when it is valid.
// MergedLacks lists the beam commits past mergedHead that a landing of the merged pull request would lack.
// Each is a short object name and subject; the parent merges and records-only commits are left out.
// Only the pull request verbs and `ready` call them.
type landDeps struct {
	Machine        func() (string, error)
	ReadOwners     func() (boardengine.Owners, error)
	ParentOf       func(slug string) (string, error)
	PairPresent    func(slug string) bool
	OriginRecord   func(slug string) (fabricengine.Origin, bool, error)
	ChildStatus    func(slug string) (status shedengine.Status, found bool, err error)
	RunID          func(slug string) (string, error)
	Ready          func(slug string) (bool, error)
	MarkReady      func(slug string, record pawlengine.ReadyRecord) (string, error)
	Refusal        func(slug string) error
	WriteRequest   func(slug string, req pawlengine.LandRequest) (string, error)
	Now            func() time.Time
	Tasks          func() ([]boardengine.Task, error)
	FileTask       func(fields map[string]any) error
	CodeReviewCaps func() (reviewbudget.Caps, error)

	DraftDescription  func(slug string) (path string, err error)
	RecordDescription func(slug string, at time.Time) (path string, err error)

	PullRequestSide func(slug string) (beamPullRequestSide, error)
	PushBeam        func(slug string) (head string, err error)
	BeamHasCommits  func(slug string) (bool, error)
	Landing         func(slug string) (pawlengine.LandingRecord, bool, error)
	WriteLanding    func(slug string, mutate func(rec *pawlengine.LandingRecord) error) error
	Description     func(slug string) (title, body string, err error)
	BeamHead        func(slug string) (string, error)
	PullRequestMode func(slug string) (mode string, rulesRequired bool, err error)
	Sleep           func(ctx context.Context, d time.Duration) error

	ParentUnmerged      func(slug string) (bool, error)
	DescriptionFindings func(slug string) ([]string, error)
	MergedLacks         func(slug, mergedHead string) ([]string, error)
}

// land asks the pawl run of the beam slug to land the beam.
// It returns the request's path and the command that follows the landing.
// The request is only a file: Beam-Await consumes it, and the beam's pair is removed at the run's teardown.
// So the verb runs from any worktree, the beam itself included.
func land(deps landDeps, slug string) (map[string]any, error) {
	here, err := deps.Machine()
	if err != nil {
		return nil, fmt.Errorf("lym beam land: read this machine's name: %w", err)
	}
	owners, err := deps.ReadOwners()
	if err != nil {
		return nil, fmt.Errorf("lym beam land: read the board's owners: %w", err)
	}
	if err := judgeLandEntry(deps, owners, "lym beam land", slug); err != nil {
		return nil, err
	}
	if err := judgeLandRun(deps, slug); err != nil {
		return nil, err
	}
	if err := judgeLandOwners(deps, owners, here, "lym beam land", slug); err != nil {
		return nil, err
	}
	ready, err := deps.Ready(slug)
	if err != nil {
		return nil, fmt.Errorf("lym beam land: read the ready record of %s: %w; way forward: fix the cause it names, then run `lym beam land %s` again", slug, err, slug)
	}
	if !ready {
		return nil, fmt.Errorf("lym beam land: the beam %s is not flagged ready to merge; way forward: run `lym beam ready %s`, after which its dobby lands it once nothing works under it", slug, slug)
	}
	if err := deps.Refusal(slug); err != nil {
		return nil, err
	}

	runID, err := deps.RunID(slug)
	if err != nil {
		return nil, fmt.Errorf("lym beam land: read the pawl run id of %s: %w; way forward: fix the cause it names, then run `lym beam land %s` again", slug, err, slug)
	}
	path, err := deps.WriteRequest(slug, pawlengine.LandRequest{RunID: runID, RequestedAt: deps.Now().UTC()})
	if err != nil {
		return nil, fmt.Errorf("lym beam land: write the land request of %s: %w; way forward: fix the cause it names, then run `lym beam land %s` again", slug, err, slug)
	}
	return map[string]any{
		"slug":        slug,
		"request":     path,
		"run_id":      runID,
		"way_forward": fmt.Sprintf("run `lym batten status %s` to follow the landing", slug),
	}, nil
}

// markReady flags the beam slug ready to be merged into its parent.
// It returns the ready record's path and the command that follows the landing.
// A beam already flagged is reported as such and its record is not rewritten,
// but a record left uncommitted by a failed commit is committed.
// A beam not yet flagged is judged by judgeReadyLanding first, since the flag is final.
func markReady(deps landDeps, slug string) (map[string]any, error) {
	owners, err := deps.ReadOwners()
	if err != nil {
		return nil, fmt.Errorf("lym beam ready: read the board's owners: %w", err)
	}
	if err := judgeLandEntry(deps, owners, "lym beam ready", slug); err != nil {
		return nil, err
	}
	ready, err := deps.Ready(slug)
	if err != nil {
		return nil, fmt.Errorf("lym beam ready: read the ready record of %s: %w; way forward: fix the cause it names, then run `lym beam ready %s` again", slug, err, slug)
	}
	if !ready {
		// The one wait here is a read of a merge state GitHub has not computed yet.
		// pullrequest.MergeRetryWindow bounds it.
		if err := judgeReadyLanding(context.Background(), deps, slug); err != nil {
			return nil, err
		}
	}
	result := map[string]any{
		"slug":        slug,
		"already":     ready,
		"way_forward": fmt.Sprintf("the beam's dobby lands %s once nothing works under it; run `lym batten status %s` to follow the landing", slug, slug),
	}
	path, err := deps.MarkReady(slug, pawlengine.ReadyRecord{Slug: slug, SetAt: deps.Now().UTC()})
	if err != nil {
		return nil, fmt.Errorf("lym beam ready: write the ready record of %s: %w; way forward: fix the cause it names, then run `lym beam ready %s` again", slug, err, slug)
	}
	result["record"] = path
	return result, nil
}

// GitHub's mergeable_state values judgeReadyLanding names an action for.
const (
	mergeableDirty   = "dirty"
	mergeableBehind  = "behind"
	mergeableBlocked = "blocked"
)

// judgeReadyLanding refuses a beam whose landing the orch can still fix, each refusal naming its way forward.
// The checks run in this order:
// a run not at Beam-Await; open members and child pairs; a parent holding commits the beam lacks;
// a missing or invalid description; the pull request conditions.
// With no pull request recorded, a required parent or GitHub's rules refuse the landing, naming `lym beam pr open`;
// with one recorded, it must be able to merge now or already be merged.
func judgeReadyLanding(ctx context.Context, deps landDeps, slug string) error {
	const verb = "lym beam ready"
	if err := judgeAwaitingRun(deps, verb, slug); err != nil {
		return err
	}
	if err := deps.Refusal(slug); err != nil {
		return err
	}
	if err := judgeParentMerged(deps, verb, slug); err != nil {
		return err
	}
	record, err := judgeDescribed(deps, verb, slug)
	if err != nil {
		return err
	}
	if record.PullRequest == nil {
		return judgeNoPullRequest(deps, verb, slug)
	}
	return judgeRecordedPullRequest(ctx, deps, verb, slug, *record.PullRequest)
}

// judgeParentMerged refuses a beam whose parent branch holds commits the beam lacks.
// The refusal names the merge-in the orch runs, or the one already filed.
func judgeParentMerged(deps landDeps, verb, slug string) error {
	unmerged, err := deps.ParentUnmerged(slug)
	if err != nil {
		return rerunRefusal(verb, slug, "read whether the beam holds the parent's commits", err)
	}
	if !unmerged {
		return nil
	}
	tasks, err := deps.Tasks()
	if err != nil {
		return rerunRefusal(verb, slug, "read the board", err)
	}
	if task, open := openMergeTask(tasks, slug); open {
		return fmt.Errorf("%s: the parent branch holds commits the beam %s lacks, and the merge task %s is under way; way forward: run `lym batten status %s`, let it land into the beam, then run `%s %s` again", verb, slug, task, task, verb, slug)
	}
	return fmt.Errorf("%s: the parent branch holds commits the beam %s lacks; way forward: run `lym beam merge-parent %s`, let its task land into the beam, then run `%s %s` again", verb, slug, slug, verb, slug)
}

// judgeDescribed refuses a beam whose change description is missing or fails validation.
// It returns the landing record it read.
func judgeDescribed(deps landDeps, verb, slug string) (pawlengine.LandingRecord, error) {
	record, found, err := deps.Landing(slug)
	if err != nil {
		return record, rerunRefusal(verb, slug, "read the landing record", err)
	}
	if !found || record.DescribedAt.IsZero() {
		return record, fmt.Errorf("%s: no change description is written for the beam %s; way forward: run `lym beam describe %s`, then `%s %s` again", verb, slug, slug, verb, slug)
	}
	findings, err := deps.DescriptionFindings(slug)
	if err != nil {
		return record, rerunRefusal(verb, slug, "validate the change description", err)
	}
	if len(findings) > 0 {
		return record, fmt.Errorf("%s: the change description of the beam %s fails validation: %s; way forward: fix summary.md in the beam's _lym/landing directory, run `lym beam describe %s`, then `%s %s` again", verb, slug, strings.Join(findings, "; "), slug, verb, slug)
	}
	return record, nil
}

// judgeNoPullRequest refuses a beam with no pull request recorded when its landing needs one.
// A required parent or GitHub's rules demand a pull request;
// an origin that lacks a fact the demand needs refuses the beam too.
// A merge parent passes, since its landing opens its own pull request.
// A beam with no commits beyond its parent passes too.
func judgeNoPullRequest(deps landDeps, verb, slug string) error {
	mode, rulesRequired, err := deps.PullRequestMode(slug)
	if err != nil {
		return rerunRefusal(verb, slug, "read the parent's pull request mode", err)
	}
	hasCommits, err := deps.BeamHasCommits(slug)
	if err != nil {
		return rerunRefusal(verb, slug, "read the beam's commits", err)
	}
	side, err := deps.PullRequestSide(slug)
	if err != nil {
		return rerunRefusal(verb, slug, "read the beam's origin", err)
	}
	decision := landingshed.ResolvePullRequest(slug, mode, "", landingshed.BeamWithoutPullRequest, rulesRequired, side.Origin, hasCommits)
	if decision.Halt != "" {
		demand := "GitHub's rules for the parent branch require a pull request"
		if mode == parentModeRequired || mode == parentModeMerge {
			demand = fmt.Sprintf("the parent's beam.yaml pull_request is %s", mode)
		}
		return fmt.Errorf("%s: a pull request is required for the beam %s (%s) but %s; way forward: give the hub a GitHub origin, an upstream for the parent branch and no push skip, then run `%s %s` again", verb, slug, demand, landingshed.OriginMissing(side.Origin), verb, slug)
	}
	if decision.Open && mode != parentModeMerge {
		return fmt.Errorf("%s: a pull request is required for the beam %s and none is recorded; way forward: run `lym beam pr open %s`, then `%s %s` again", verb, slug, slug, verb, slug)
	}
	return nil
}

// judgeRecordedPullRequest refuses a beam whose recorded pull request GitHub would not merge now.
// That is a closed pull request, or an open one at a head other than the beam's or in a state that cannot merge;
// a merged pull request that lacks beam commits is refused too.
func judgeRecordedPullRequest(ctx context.Context, deps landDeps, verb, slug string, recorded pawlengine.LandingPullRequest) error {
	side, err := deps.PullRequestSide(slug)
	if err != nil {
		return rerunRefusal(verb, slug, "read the beam's origin", err)
	}
	if err := requireOrigin(verb, slug, side); err != nil {
		return err
	}
	head, err := deps.BeamHead(slug)
	if err != nil {
		return rerunRefusal(verb, slug, "read the beam's head", err)
	}
	merge, err := pullrequest.ReadMergeState(ctx, side.API, recorded.Number, head, side.ParentBranch, deps.Now, deps.Sleep)
	if err != nil {
		return rerunRefusal(verb, slug, fmt.Sprintf("read pull request #%d", recorded.Number), err)
	}
	switch merge.State {
	case pullrequest.StateClosed:
		return fmt.Errorf("%s: pull request #%d of the beam %s is closed; way forward: run `lym beam pr open %s`, which reopens it, or `lym beam pr close %s` to land without one, then `%s %s` again", verb, recorded.Number, slug, slug, slug, verb, slug)
	case pullrequest.StateMerged:
		if merge.Head == head {
			return nil
		}
		lacking, err := deps.MergedLacks(slug, merge.Head)
		if err != nil {
			return rerunRefusal(verb, slug, "read the beam commits past the merged pull request", err)
		}
		if len(lacking) > 0 {
			return fmt.Errorf("%s: pull request #%d is merged without these beam commits: %s; way forward: run `lym beam pr close %s`, then land without a pull request or open a new one with `lym beam pr open %s`", verb, recorded.Number, strings.Join(lacking, "; "), slug, slug)
		}
		return nil
	}
	if merge.Head != head {
		return fmt.Errorf("%s: pull request #%d is at %s, not the beam's head %s; way forward: run `lym beam pr update %s`, then `%s %s` again", verb, recorded.Number, merge.Head, head, slug, verb, slug)
	}
	if merge.MergeableNow {
		return nil
	}
	var actions []string
	if merge.Draft {
		actions = append(actions, fmt.Sprintf("run `lym beam pr update %s`, which marks the draft ready for review", slug))
	}
	switch merge.MergeableState {
	case mergeableDirty, mergeableBehind:
		actions = append(actions, fmt.Sprintf("run `lym beam merge-parent %s`, which brings the parent in", slug))
	case mergeableBlocked:
		actions = append(actions, "get the review or the required check that GitHub still owes")
	}
	if len(actions) == 0 {
		actions = append(actions, fmt.Sprintf("look at %s and fix what stops the merge", recorded.URL))
	}
	return fmt.Errorf("%s: pull request #%d cannot merge now: %s; way forward: %s, then run `%s %s` again; `lym beam pr status %s` reads the same state", verb, recorded.Number, strings.Join(merge.Causes, "; "), strings.Join(actions, "; "), verb, slug, slug)
}

// judgeLandEntry refuses the entry slug unless it is a beam project that is not done.
// Its pair must be on this machine and carry the beam mark.
// verb prefixes each refusal message.
func judgeLandEntry(deps landDeps, owners boardengine.Owners, verb, slug string) error {
	entry, held := owners.Entries[slug]
	switch {
	case !held || (!entry.IsTask && !entry.IsProject):
		return fmt.Errorf("%s: %q is not a beam, as an absent entry or a note is not; way forward: `lym board list` shows the entries", verb, slug)
	case entry.IsTask:
		return fmt.Errorf("%s: %q is a task, not a beam; way forward: land a task with `lym batten run %s`, or make a project entry with `lym board upsert` under a slug of its own, then run `lym beam create <that slug>`", verb, slug, slug)
	case entry.Done:
		return fmt.Errorf("%s: the beam %q is done and already landed", verb, slug)
	case !entry.Beam:
		return fmt.Errorf("%s: %q is a grouping project, not a beam; way forward: run `lym beam create %s` to make it a beam", verb, slug, slug)
	case !deps.PairPresent(slug):
		return fmt.Errorf("%s: the beam %s has no pair on this machine; way forward: run `lym beam create %s` to finish the create, or `lym adopt %s` when the beam's branch is on origin", verb, slug, slug, slug)
	}
	origin, found, err := deps.OriginRecord(slug)
	if err != nil {
		return fmt.Errorf("%s: read the origin record of %s: %w", verb, slug, err)
	}
	if !found {
		return fmt.Errorf("%s: the pair of the beam %s has no readable origin record; way forward: run `lym fabric reconcile` to restore its records worktree, then `%s %s`", verb, slug, verb, slug)
	}
	if !origin.Beam {
		return fmt.Errorf("%s: the pair of %s does not carry the beam mark; way forward: run `lym beam create %s`, which marks it", verb, slug, slug)
	}
	return nil
}

// judgeLandRun refuses a beam whose pawl run is absent.
// It also refuses a beam whose run is not running at Beam-Await, the one row that reads a land request.
// A beam made before the pawl recipe has no run.
func judgeLandRun(deps landDeps, slug string) error {
	status, found, err := deps.ChildStatus(slug)
	if err != nil {
		return fmt.Errorf("lym beam land: read the pawl run of %s: %w; way forward: fix the cause it names, then run `lym beam land %s` again", slug, err, slug)
	}
	if !found {
		return fmt.Errorf("lym beam land: the beam %s has no pawl run on this machine; way forward: run `lym beam create %s`, which starts it", slug, slug)
	}
	if status.State != shedengine.StateRunning || status.CurrentProducer != pawlengine.NameBeamAwait {
		return fmt.Errorf("lym beam land: the pawl run of %s is %s at %s, and only a run waiting at %s reads a land request; way forward: run `lym batten status %s`, then `lym beam land %s` once the run waits", slug, status.State, status.CurrentProducer, pawlengine.NameBeamAwait, slug, slug)
	}
	return nil
}

// judgeAwaitingRun refuses a beam with no pawl run or whose run is not at Beam-Await.
// That is the row the landing verbs serve.
// verb prefixes each refusal message.
// A run at Beam-Await passes in every state, a halted one included;
// Beam-Await halts by an error stop the status records as failed.
func judgeAwaitingRun(deps landDeps, verb, slug string) error {
	status, found, err := deps.ChildStatus(slug)
	if err != nil {
		return fmt.Errorf("%s: read the pawl run of %s: %w; way forward: fix the cause it names, then run `%s %s` again", verb, slug, err, verb, slug)
	}
	if !found {
		return fmt.Errorf("%s: the beam %s has no pawl run on this machine; way forward: run `lym beam create %s`, which starts it", verb, slug, slug)
	}
	if status.CurrentProducer != pawlengine.NameBeamAwait {
		return fmt.Errorf("%s: the pawl run of %s is %s at %s, and the landing work belongs to the run at %s; way forward: run `lym batten status %s`, then `%s %s` once the run waits there", verb, slug, status.State, status.CurrentProducer, pawlengine.NameBeamAwait, slug, verb, slug)
	}
	return nil
}

// judgeNotReady refuses a beam whose ready record names it, since `lym beam ready` is final; verb opens each refusal.
func judgeNotReady(deps landDeps, verb, slug string) error {
	ready, err := deps.Ready(slug)
	if err != nil {
		return fmt.Errorf("%s: read the ready record of %s: %w; way forward: fix the cause it names, then run `%s %s` again", verb, slug, err, verb, slug)
	}
	if ready {
		return fmt.Errorf("%s: the beam %s is flagged ready, and `lym beam ready` is final, so its landing is under way; way forward: run `lym batten status %s` to follow it", verb, slug, slug)
	}
	return nil
}

// judgeLandOwners refuses a beam another machine owns and a beam whose parent another machine owns.
// Each refusal opens with verb, names the run to repeat, and names the adopt that takes the beam over.
// A beam with no owner recorded reads its parent's, so only the parent check judges it.
func judgeLandOwners(deps landDeps, owners boardengine.Owners, here, verb, slug string) error {
	if owner := owners.Entries[slug].Owner; boardengine.OwnedElsewhere(owner, here) {
		return fmt.Errorf("%s: the beam %s is owned by machine %s; way forward: run `%s %s` on %s, or run `lym adopt %s` here to take the beam over", verb, slug, owner, verb, slug, owner, slug)
	}
	parent, err := deps.ParentOf(slug)
	if err != nil {
		return fmt.Errorf("%s: read the parent of %s: %w", verb, slug, err)
	}
	owner, subject := owners.Prime, boardengine.RootSubject
	if parent != "" {
		if owner, err = owners.Effective(parent, deps.ParentOf); err != nil {
			return fmt.Errorf("%s: read the owner of %s: %w", verb, parent, err)
		}
		if subject, err = owners.AdoptSubject(parent, deps.ParentOf); err != nil {
			return fmt.Errorf("%s: read what adopting %s takes: %w", verb, parent, err)
		}
	}
	if boardengine.OwnedElsewhere(owner, here) {
		return fmt.Errorf("%s: the parent of %s is owned by machine %s, and a landing merges into it; way forward: run `lym adopt %s` here to take it over, then `%s %s`", verb, slug, owner, subject, verb, slug)
	}
	return nil
}
