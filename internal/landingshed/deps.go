// deps.go declares this package's told-value surface: Deps, whose every field is told by the caller.
// It also declares the one-method resolver seam that the producers hold.

package landingshed

import (
	"context"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/mergeresolve"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// resolver is the one-method seam through which the producers merge and resolve conflicts.
// It is unexported so that the package's constructors stay the only way to obtain a resolver.
// The seam also lets in-package tests substitute a fake.
type resolver interface {
	// Resolve merges source into the current pair.
	// If the merge produces conflicts, it drives a conflict-resolution session to clear them.
	// A non-empty tip is the commit merged in place of source's own tip.
	Resolve(ctx context.Context, source, tip string) (mergeresolve.Result, error)
}

var _ resolver = (*mergeresolve.Resolver)(nil)

// Deps carries every told value the producers in this package need to construct and run.
// It derives nothing; `landingDeps` in internal/loomcli sets every field, and a drift test asserts that it does.
type Deps struct {
	// WorktreeRoot is the absolute root of the task worktree the producers run from.
	WorktreeRoot string
	// TaskBranch is the name of the task's own branch.
	// It is the pull request's head branch, and the branch merged into the parent pair.
	TaskBranch string
	// ParentBranch is the name of the branch this task's work merges toward.
	// It is the pull request's base branch, and the branch that both the merge-in and the parent-side merge target.
	ParentBranch string
	// DescriptionPath is the absolute path to the change description file itself, not a directory.
	// The caller resolves it, so this package does not need to know which producer wrote the file.
	DescriptionPath string
	// StencilsDir is the absolute directory the conflict-resolution stencil is read from.
	StencilsDir string
	// ParentName is the name of the session the conflict session escalates to.
	// Empty renders the no-parent variant.
	ParentName string
	// LanguageSkills are the hub's language skills the conflict session loads after its own.
	// Empty means none.
	LanguageSkills []string

	// ScratchDir is the told absolute scratch directory, used only as the directory of the conflict-resolution report.
	ScratchDir string

	// PushSkipped is the told decision to skip the parent push.
	PushSkipped bool

	// EscalateJudgedFirst is the told decision whether a merge fixer's escalate halts with `EscalateStopPrefix`,
	// so the arbiter judges it before the orch, or with `LandingStopPrefix` as a hand-back.
	// It is true where the arbiter has a decision verb to settle the escalation.
	// It is false for a pawl run, whose arbiter has none.
	EscalateJudgedFirst bool

	// OpenFabric opens the fabric over the task worktree's own pair.
	// OpenParentFabric opens the fabric over the parent branch's pair.
	// Opening lazily is required, since the constructor each one wraps stat-checks the paired layout.
	// Opening eagerly would fail before the run's own preflight has confirmed anything is wired.
	OpenFabric       func() (*fabricengine.Fabric, error)
	OpenParentFabric func() (*fabricengine.Fabric, error)

	// CommitStatus is the injected loop-owner closure that commits the product's own orchestration status file.
	// It runs immediately before the producers merge.
	// The product rewrites that file on every producer transition but commits it only once, at bootstrap.
	// So by the time the merge runs, the file is a tracked, uncommitted modification.
	// fabricengine's merge guard refuses any tracked modification on either side of the pair.
	// Without this closure, the merge refuses on the run's own bookkeeping every time, and only a human can recover.
	//
	// It is a told closure rather than a path this package commits itself.
	// Naming the status file here would make this package declare a location it may not declare.
	// The pathspec and the commit message belong to the product whose status file it is.
	//
	// Nil means no status file to commit.
	CommitStatus func() error

	// Reconcile runs the origin reconcile over the run's own scope and returns its report.
	// The producers read the report before they act on the parent.
	// It is called either before the parent lock is taken or under the parent lock the run already holds.
	// A reconcile that judges the run ended or stopped is a Stuck.
	// An unreachable origin proceeds.
	// A returned error is returned from Call.
	//
	// Nil means no reconcile.
	Reconcile func() (fabricengine.OriginReport, error)
	// RemoveFollowUp returns the step that settles the run's batten record.
	// A stopped reason whose way forward names `lym fabric remove` ends in that step instead of the resume.
	// The removal deletes the task worktree the resume runs in.
	// An empty answer keeps the resume.
	//
	// Nil means no follow-up.
	RemoveFollowUp func() string

	// Slug is the run's slug, which the owner halts and repair notes name in the adopt commands they give.
	Slug string
	// AdoptSubject returns what `lym adopt` takes to move the run: the beam or `root`.
	// A returned error and an empty answer both read as the run's Slug.
	//
	// Nil means the run's slug.
	AdoptSubject func() (string, error)
	// ParentHoldsTaskWork reports whether the parent branch already holds the task's work.
	// A true answer marks the run as past its parent-side merge.
	// It is read before the owner checks.
	// A parent holding the work skips both owner checks and the unreachable halt.
	// A returned error halts the run.
	//
	// Nil means the run is not past the merge.
	ParentHoldsTaskWork func() (bool, error)
	// ParentOwner reads who owns the parent, from the local board the origin reconcile left.
	// A returned error halts the run.
	//
	// Nil means the parent is owned here.
	ParentOwner func() (ParentOwnership, error)
	// OriginParentHoldsWork reports whether origin's parent branch already holds the task's work.
	// It is read after a failed parent push.
	// A returned error adds no repair note.
	//
	// Nil means no repair note.
	OriginParentHoldsWork func() (bool, error)
	// ParentCommitsOriginLacks lists the SHAs of the parent branch's commits that origin's tracking ref lacks, newest first.
	// It is read for the repair note only when origin's parent branch already holds the task's work.
	//
	// Nil means no repair note.
	ParentCommitsOriginLacks func() ([]string, error)

	// LandBlockers reads what still works under the beam the run lands:
	// its open member entries and the local pairs forked from it.
	// It is read under the parent lock, right after the owner checks, and any remaining blocker halts the landing.
	// A returned error halts the run.
	//
	// Nil means no beam to check, which is the value for every recipe but pawl.
	LandBlockers func() ([]LandBlocker, error)
	// ParentWorktree is the name of the beam's parent worktree, where the land check's ways forward say a cancel runs.
	// Told by the caller, and empty when no land blockers are wired.
	ParentWorktree string

	// CommitParentRecords commits the parent pair's own run records.
	// The finalize step calls it after its merge of the parent branch and before the parent-side merge.
	// Sibling runs leave their status records uncommitted in the parent pair,
	// and the merge guard refuses any tracked modification there.
	// A clean subtree is a no-op, and a returned error is Stuck.
	//
	// Nil means "no parent run records to commit".
	// `internal/loomcli`'s `landingDeps` fills it, and its drift guard keeps it filled.
	CommitParentRecords func() error

	// MergeState probes the pair's merge state.
	// AbortMerge aborts the pair's parked fabric merge.
	// StopConflictSession stops every live conflict-session strand of the run.
	// The producers use them to clear the merge-in their own earlier attempt left parked.
	// That clearing runs before the clean-tree check that precedes a fresh merge-in.
	// StopConflictSession returns an empty guid and a nil error when each strand stopped or none is live.
	// It returns the guid of the strand whose stop failed beside its error.
	// It returns an empty guid beside an error when the strand table could not be read.
	//
	// A nil MergeState skips the step, a nil AbortMerge aborts nothing, and a nil StopConflictSession stops nothing.
	// `internal/loomcli`'s `landingDeps` fills them all, and its drift guard keeps them filled.
	MergeState          func() (fabricengine.MidMergeState, error)
	AbortMerge          func() error
	StopConflictSession func() (guid string, err error)

	// TaskHead returns the task branch's local HEAD SHA.
	// The Parent-Align row refuses a nil TaskHead when it is built.
	// `internal/loomcli`'s `landingDeps` fills it, and its drift guard keeps it filled.
	TaskHead func() (string, error)

	// MarkTaskDone marks the task's board entry done.
	// The finalize step calls it once the parent-side merge has landed, before the parent push.
	// It only logs a failure: the change is on the parent whatever the board says.
	// landingshed names no board path, so the board wiring lives in the closure.
	//
	// Nil means "no board to update", and the step fails safe by leaving the board untouched.
	// `internal/loomcli`'s `landingDeps` fills it, and its drift guard keeps it filled.
	MarkTaskDone func() error

	// RecordLanding records commit as the run's landing commit, so the dobby driving the run can name it.
	// The commit is the parent pair's head right after a parent-side merge that landed.
	// The finalize step calls it after marking the task done and before the push.
	// It only logs a failure, of the head read or of the record; nil records nothing.
	RecordLanding func(commit string) error
	// RecordLandedSize records the size of the diff the run landed.
	// The diff runs from the parent head merged into the task worktree to the landing commit.
	// The finalize step calls it after recording the landing and before the push.
	// It only logs a failure; nil records nothing.
	// A finalize step that read no such parent head in its process, as one resumed past the merge, does not call it.
	RecordLandedSize func(from, to string) error

	// ConfigChanges returns the task's changes to its own per-worktree config files since its fork point.
	// The finalize step calls it before the catch-up merge-in, so before any parent-side mutation.
	// It only reports the result as a notice: landing never carries those files to the parent.
	// A returned error is logged and reported as a notice, never a stop.
	//
	// Nil means "no config changes to report".
	// `internal/loomcli`'s `landingDeps` fills it, and its drift guard keeps it filled.
	ConfigChanges func() (fabricengine.ConfigChanges, error)

	// Notify queues one single-line notice for the hub's orchestrator.
	// The finalize step calls it once, at Done, when the config changes produced a line.
	// It only logs a failure: the landing has happened whatever the notice delivery says.
	// landingshed names no orchestrator path, so the delivery wiring lives in the closure.
	//
	// Nil means "no one to notify".
	// `internal/loomcli`'s `landingDeps` fills it, and its drift guard keeps it filled.
	Notify func(line string) error

	// GateKey is the key of the hub's gate.yaml the row runs after a merge-in:
	// `parent-align` for Parent-Align, `finalize` for Finalize.
	// The row's registry entry sets it on its own copy of Deps.
	// The producer reads it as set, with no default for an empty one.
	GateKey string
	// VerifyCommand returns the call of the named gate key.
	// The Parent-Align and Finalize rows run it in the task worktree after a merge-in, each with its own gate key.
	// A darn run with a tree-scoped diff returns the tree-scoped command in its place.
	// The clean-tree checks around the merge-in run whether or not a command is wired.
	// The caller builds Deps before the command's source exists on a fresh run.
	// So it is read each time the gate runs rather than once at construction.
	// A refusal of the gate config is the returned error, carrying its own way forward.
	//
	// Nil means "no verify wiring":
	// the gate never runs and nothing is logged.
	// An empty returned string means the source carries no command for the key, which skips the gate with a warning.
	VerifyCommand func(gateKey string) (string, error)
	// BaseCommand returns the command whose verified-tree entry every landing pass keeps.
	// That keeps a pass from pruning the entry the next review round diffs from.
	// It is read at each gate call.
	// Nil, or an empty answer, names none.
	BaseCommand func() (string, error)
	// GateTiers returns the tier numbers the named gate key runs, in order.
	// A failing call whose log names no failing tier records the highest tier of its row's key, read through it.
	// Told, and required wherever a failure record is written:
	// a nil lookup would panic on a failing log with no tier summary line.
	GateTiers func(gate string) ([]int, error)
	// VerifyWaitMark sets or clears the pane mark of the run's driver strand around the post-merge verify.
	// It lets the wait show on screen.
	// The gate clears the mark before each verify.
	// It sets the mark with the label `verify <producer>` and the verify's start time immediately before the verify runs.
	// It clears the mark on every path out of the verify; an empty label clears it.
	// The mark is display only: a returned error is logged and changes no Stuck reason, error return or verify outcome.
	//
	// Nil means "no strand to mark", so a run with no driver strand gets the status note only.
	// `internal/loomcli`'s `landingDeps` fills it, and its drift guard keeps it filled.
	VerifyWaitMark func(label string, start time.Time) error
	// FailingTests parses a failed verify's log into the failing tests the failure record names.
	// It is told so this package stays free of the module that owns the log format.
	// Nil records no tests.
	// `internal/loomcli`'s `landingDeps` fills it, and its drift guard keeps it filled.
	FailingTests func(log string) []verifytree.FailedTest
	// TraceFailure traces a failed verify's log, run at head, to the plan cards that hold the failing code.
	// The failure record's cards, later range and untraced locations come from the trace.
	// It is told so this package stays free of the plan and quarry modules.
	// Nil traces nothing, as for a run with no plan.
	// A trace error is logged and leaves the record without the fields.
	// `internal/loomcli`'s `landingDeps` fills it for a tapestry run and leaves it nil for a darn run.
	TraceFailure func(log, head string) (verifytree.FailureTrace, error)
	// VerifyDir is the told directory of `internal/verifytree`'s verified-tree record, running marker and verify log.
	// Every plan-verify site of the worktree shares it, so a pass at one site lets the next skip.
	// Told, never derived, per the Told-Geometry Invariant.
	VerifyDir string
	// VerifyHold is the told pause hold of the post-merge verifies.
	// It holds a verify while a pause covers the run, and stops its command past the grace end.
	// Nil holds nothing.
	VerifyHold *verifytree.PauseHold

	// Shuttle is the told session-runner seam of the conflict resolver.
	// The resolver's constructor rejects a nil value, so without this seam the conflict session could never spawn.
	Shuttle mergeresolve.Shuttle

	// EnginesRun returns the names of the engines the task's sessions ran under.
	// The names pick the credit block's engine lines.
	// The finalize step calls it once per Call, before any parent-side mutation.
	// PR-Review calls it each time it sets the pull request's body.
	// A returned error is returned from Call.
	//
	// Nil means "no engine ran", so the credit block carries no engine line.
	EnginesRun func() ([]string, error)

	// DriftVerdict returns the task's regression check as a markdown section.
	// PR-Review and the finalize step append it to the change description's body in the pull request and the landing commit.
	// The PR-Review session's prompt shows it as well.
	// An empty answer carries no section, and a returned error is returned from Call.
	//
	// Nil means "no regression check".
	// `internal/loomcli`'s `landingDeps` fills it, and its drift guard keeps it filled.
	DriftVerdict func() (string, error)

	// Registry is the resolved model-spec registry that the producers' resolvers read the conflict spec's alias against.
	Registry modelspec.Registry
	// Config is the loaded landing.yaml configuration.
	Config Config

	// Lock is the parent lock's told seams, which the lock-taking rows share through one guard.
	Lock ParentLockDeps
	// ReadParentTips reads the parent branch's local and upstream tips over the task pair, fetching first when fetch is true.
	ReadParentTips func(fetch bool) (fabricengine.ParentTips, error)
	// HoldsWork reports whether merging source into base adds nothing, over the task pair.
	HoldsWork func(base, source string) (bool, error)
	// ChangedPaths returns the sorted paths that differ between from and to, over the task pair.
	ChangedPaths func(from, to string) ([]string, error)
	// MergeBase returns the best common ancestor of the task pair's HEAD and ref.
	MergeBase func(ref string) (string, error)
	// MergeRecordPath is the told merge record file, in the run's scratch directory.
	MergeRecordPath string

	// FixerSeats is the seat runner the merge fixer session runs on, which `*seatengine.Engine` satisfies.
	// The rows that run the merge fixer run it through this seat runner, and the fixer loop refuses to run without it.
	FixerSeats fixerSeats
	// DecisionRecordPath is the run's decision record, which the fixer session reads.
	DecisionRecordPath string
	// PlanDir is the run's plan directory, which the fixer session reads; empty for a darn run, which has none.
	PlanDir string
	// LineChange reads the paths, line count and added lines of the change between two commits of the task pair.
	// The fixer's caps judge that change.
	LineChange func(from, to string) (fabricengine.LineChange, error)
	// IsTestFile reports whether a slash-separated repository-relative path is a test file of the run's language.
	// The fixer's kind cap uses it to judge a change.
	IsTestFile func(path string) bool
	// RowEntry answers the current entry of the named row from the run's status history.
	// That lets a row's attempt record keep one identity across its resumes.
	RowEntry func(row string) (int, error)
	// TaskRemoteTip reads the task branch's remote-tracking tip, `origin/<task branch>`, without a fetch.
	// It answers "" when there is none.
	// A push the fixer session makes from the task pair updates that ref, so a changed tip shows the push.
	TaskRemoteTip func() (string, error)
	// CodeReviewOff reports whether the Code-Perch reads as off:
	// its effective review budget is 0, or the Code-Review rows' entry gate leaves them out.
	// With the perch off no review follows Parent-Align and its fixer's change is judged against the caps instead.
	// A nil CodeReviewOff reads as the budget being 0, and a returned error is an error stop of the row.
	CodeReviewOff func() (bool, error)

	// PullRequest is the PR-Review row's told seams.
	PullRequest PullRequestDeps
}

// PullRequestDeps carries the told seams the PR-Review row decides and opens the run's pull request through.
// The row treats every field but `BeamRecord` and `DropBeamRecord` as required.
// It returns an error naming the first one left unset.
type PullRequestDeps struct {
	// RecordPath is the run's pull-request record file, and RecordLockPath the advisory lock that guards it.
	RecordPath     string
	RecordLockPath string
	// ParentMode reads the receiving branch's beam.yaml `pull_request`.
	// It returns that value with the name of the worktree it was read in.
	// On an error the worktree name is still returned, so the halt names where to fix the file.
	ParentMode func() (mode, worktree string, err error)
	// EntryField reads the landing entry's `pull_request` field, empty when the entry has none.
	EntryField func() (string, error)
	// Origin reads the facts about the task pair's origin that a pull request needs.
	Origin func() (Origin, error)
	// API opens the GitHub calls for the repository the origin URL names.
	API func(originURL string) (pullrequest.API, error)
	// PushTask pushes the task branch to its origin and returns once the push has finished.
	PushTask func() error
	// CommitsMissing lists the task commits not reachable from merged, the head GitHub merged.
	CommitsMissing func(merged, task string) ([]string, error)
	// TaskHasCommits reports whether the task branch holds a commit the local parent branch lacks, read by git.
	TaskHasCommits func() (bool, error)
	// DirtyPaths lists the task worktree's uncommitted paths.
	DirtyPaths func() ([]string, error)
	// MailAgent posts line to the creel strand named strandName.
	MailAgent func(strandName, line string) error
	// PauseCovered reports whether a pause covers the run at the time of the call.
	PauseCovered func() (bool, error)
	// Now reads the clock the wait's poll spacing runs on.
	// Sleep waits d or returns the context's error once ctx ends first.
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration) error
	// RecordChanges opens a watch on the record file and returns a channel that delivers a value after a change to it.
	// It also returns the function that ends the watch.
	// A nil channel, as when no watch can be opened, leaves the wait on its timer.
	RecordChanges func() (changes <-chan struct{}, stop func())
	// Sessions starts and stops the interactive session the operator discusses the pull request with.
	// `*shuttleengine.Runner` satisfies it.
	Sessions prSessions
	// SessionLive reports whether the strand guid names is live.
	SessionLive func(guid string) (bool, error)
	// RegisterSession registers the process of the session's pane under the run, so a teardown ends it.
	RegisterSession func(guid string) error
	// ParentReviewWanted reports whether the hub or the task turns on the parent orch's review of the pull request.
	// It is read at each entry of the row, and nil reads as off.
	ParentReviewWanted func() (bool, error)
	// AskParent posts line to the orch the run's agents reach as their parent, and reports whether it posted.
	// It reports false with no error when no orch is recorded to take the line.
	AskParent func(line string) (bool, error)
	// BeamRecord reads the pull request the beam's landing record holds and reports whether it holds one.
	// A non-nil BeamRecord marks the landing as a beam's, and is nil on every run but a beam's.
	BeamRecord func() (pullRequest BeamPullRequest, recorded bool, err error)
	// DropBeamRecord drops the pull request from the beam's landing record and commits the landing directory.
	// It is nil on every run but a beam's.
	DropBeamRecord func() error
}

// ParentOwnership is who owns the parent a run lands on.
type ParentOwnership struct {
	// Slug is the parent pair's slug, empty when the parent is the prime.
	Slug string
	// Owner is the machine that owns the parent, empty when no other machine does.
	Owner string
	// Here reports whether this machine may land on the parent.
	Here bool
}

// ParentLockDeps carries the told seams the parent lock guard takes the lock through.
type ParentLockDeps struct {
	// Store is the parent lock store.
	// Nil means no lock is wired, and the guard takes none.
	Store *parentlock.Store
	// Owner returns the identity the lock is taken in: the run, and the process that holds the lock while the run is away.
	Owner func() (parentlock.Owner, error)
	// WaitPath is the run's wait record file.
	WaitPath string
	// PauseRequested reports whether the operator asked for the wait to end.
	// The request comes with a `--after` naming the row waiting in the parent lock's queue.
	// Nil reads as never.
	PauseRequested func() (bool, error)
}
