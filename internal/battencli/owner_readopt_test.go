// owner_readopt_test.go covers the owner pre-flight of a run whose task worktree is gone.
// It runs over fake re-adoption seams that record the order of their calls.

package battencli

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeReadopt is a working stand-in for the re-adoption seams.
// It answers what it holds and records each re-adoption step call in order;
// it also records each notice and each failed-stop commit.
type fakeReadopt struct {
	gone         bool
	goneErr      error
	report       fabricengine.OriginReport
	reconcileErr error
	pruneErr     error
	addErr       error

	tags            []fabricengine.RecordsTag
	tagsErr         error
	branchFile      []byte
	branchFileFound bool
	upstream        bool
	taskBranchGone  bool

	// calls lists the re-adoption step calls in order.
	calls []string
	// notices lists the lines posted to the run's parent.
	notices []string
	// commitFailed counts the commits of a failed stop.
	commitFailed int
}

// fakeRecordsRoot is the records repository root the fake names in a way forward.
const fakeRecordsRoot = "/records"

// seams returns the re-adoption seams over the fake.
func (f *fakeReadopt) seams() readoptSeams {
	return readoptSeams{
		Gone: func(string) (bool, error) {
			f.calls = append(f.calls, "gone")
			return f.gone, f.goneErr
		},
		Reconcile: func(string) (fabricengine.OriginReport, error) {
			f.calls = append(f.calls, "reconcile")
			return f.report, f.reconcileErr
		},
		Prune: func() error {
			f.calls = append(f.calls, "prune")
			return f.pruneErr
		},
		Add: func(string) error {
			f.calls = append(f.calls, "add")
			return f.addErr
		},
		SpawnPush: func(string) error {
			f.calls = append(f.calls, "push")
			return nil
		},
		Notice: func(_, line string) error {
			f.notices = append(f.notices, line)
			return nil
		},
		CommitFailed: func(string, string) error {
			f.commitFailed++
			return nil
		},
		RecordsRoot: func() (string, error) { return fakeRecordsRoot, nil },
		Tags:        func(string) ([]fabricengine.RecordsTag, error) { return f.tags, f.tagsErr },
		BranchFile:  func(string, string) ([]byte, bool, error) { return f.branchFile, f.branchFileFound, nil },
		Upstream:    func(string) (bool, error) { return f.upstream, nil },
		TaskBranch:  func(slug string) (string, error) { return slug, nil },
		TaskBranchGone: func(string) (bool, error) {
			return f.taskBranchGone, nil
		},
	}
}

// TestOwnerPreflight_GoneTaskWorktree covers a resume whose task worktree is gone.
// An active verdict runs the origin reconcile, the prune, the re-adoption and the pair's push;
// the resume reconcile runs last.
// An ended, stopped, owned-elsewhere or unreachable verdict refuses before the pair is touched,
// and a transient failure of any step refuses the pass with the re-run and leaves the status running.
// A permanent answer of the re-adoption fails a running run once:
// it gets a way forward by row, a status commit and one notice;
// a run not running keeps its state, and a held run lock or a failed tag read refuses the pass as transient.
// A task worktree that is present, and a status at Worktree-Create or Worktree-Teardown, never reach the re-adoption.
func TestOwnerPreflight_GoneTaskWorktree(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const slug = "some-slug"
	rerun := "lym batten run " + slug
	recordsBranch := fabricengine.RecordsBranchName(slug)
	active := fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryActive}}}
	cancel := "lym batten cancel " + slug + " --remove"

	statusRels, err := childStatusRels(slug)
	if err != nil {
		t.Fatalf("childStatusRels: %v", err)
	}
	statusOf := func(state shedengine.State) []byte {
		raw, err := json.Marshal(shedengine.Status{State: state})
		if err != nil {
			t.Fatalf("encode a child status: %v", err)
		}
		return raw
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	archiveTag := func(short string) string { return fabricengine.ArchiveTagPrefix + slug + "/" + short }
	originDone := fabricengine.RecordsTag{Name: archiveTag("aaaaaaaaaaaa"), Commit: strings.Repeat("a", 40), OnOrigin: true, OriginOnly: true, CommitterTime: base.Add(3 * time.Hour), Files: map[string][]byte{statusRels[0]: statusOf(shedengine.StateDone)}}
	olderDone := fabricengine.RecordsTag{Name: archiveTag("bbbbbbbbbbbb"), Commit: strings.Repeat("b", 40), OnOrigin: true, CommitterTime: base.Add(time.Hour), Files: map[string][]byte{statusRels[0]: statusOf(shedengine.StateDone)}}
	newerRunning := fabricengine.RecordsTag{Name: archiveTag("cccccccccccc"), Commit: strings.Repeat("c", 40), CommitterTime: base.Add(4 * time.Hour), Files: map[string][]byte{statusRels[0]: statusOf(shedengine.StateRunning)}}
	newestNoStatus := fabricengine.RecordsTag{Name: archiveTag("dddddddddddd"), Commit: strings.Repeat("d", 40), CommitterTime: base.Add(5 * time.Hour)}
	legacyDone := fabricengine.RecordsTag{Name: archiveTag("eeeeeeeeeeee"), Commit: strings.Repeat("e", 40), CommitterTime: base.Add(2 * time.Hour), Files: map[string][]byte{statusRels[1]: statusOf(shedengine.StateDone)}}
	leftoverTip := strings.Repeat("0123456789", 4)
	leftoverTag := archiveTag(leftoverTip[:fabricengine.ArchiveTagSHALen])
	notLiveRecords := &fabricengine.ErrNotLive{Branch: recordsBranch, Records: true}
	notLiveTask := &fabricengine.ErrNotLive{Branch: slug}
	leftover := &fabricengine.ErrRemoteLeftover{Slug: slug, Branch: recordsBranch, RemoteTip: leftoverTip}
	recordsAt := "git -C " + fakeRecordsRoot + " "

	tests := []struct {
		name     string
		producer string
		// state is the status state before the call, running when empty.
		state shedengine.State
		fake  fakeReadopt
		// lockHeld has the test hold the run lock for the whole call.
		lockHeld         bool
		wantCalls        []string
		wantErrors       []string
		notErrors        []string
		wantOwned        bool
		wantState        shedengine.State
		wantNotices      int
		wantCommitFailed int
	}{
		{
			name:      "ActiveVerdictReadoptsInOrder",
			producer:  battenrecipe.NameRunShed,
			fake:      fakeReadopt{gone: true, report: active},
			wantCalls: []string{"gone", "reconcile", "prune", "add", "push", "resume-reconcile"},
		},
		{
			name:       "OwnedElsewhereRefusesBeforeAdd",
			producer:   battenrecipe.NameRunShed,
			fake:       fakeReadopt{gone: true, report: fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryOwnedElsewhere, Owner: "there", AdoptSubject: "some-beam"}}}},
			wantCalls:  []string{"gone", "reconcile"},
			wantErrors: []string{`"there"`, "`lym adopt some-beam`"},
			wantOwned:  true,
		},
		{
			name:       "EndedRefusesBeforeAdd",
			producer:   battenrecipe.NameRunShed,
			fake:       fakeReadopt{gone: true, report: fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryEnded, How: "done"}}}},
			wantCalls:  []string{"gone", "reconcile"},
			wantErrors: []string{"origin ended", "lym cleanup --apply"},
		},
		{
			name:       "StoppedRefusesBeforeAdd",
			producer:   battenrecipe.NameRunShed,
			fake:       fakeReadopt{gone: true, report: fabricengine.OriginReport{Entries: []fabricengine.EntryOutcome{{Slug: slug, State: fabricengine.EntryStopped, WayForward: "fix the stop"}}}},
			wantCalls:  []string{"gone", "reconcile"},
			wantErrors: []string{"origin reconcile stopped it", "fix the stop", rerun},
		},
		{
			name:       "UnreachableOriginRefusesBeforeAdd",
			producer:   battenrecipe.NameRunShed,
			fake:       fakeReadopt{gone: true, report: fabricengine.OriginReport{Unreachable: true}},
			wantCalls:  []string{"gone", "reconcile"},
			wantErrors: []string{"cannot be judged", rerun, "once origin answers"},
		},
		{
			name:       "ReconcileErrorRefusesNamingItsCause",
			producer:   battenrecipe.NameRunShed,
			fake:       fakeReadopt{gone: true, reconcileErr: errors.New("reconcile blew up")},
			wantCalls:  []string{"gone", "reconcile"},
			wantErrors: []string{"reconcile blew up", rerun},
		},
		{
			name:       "PruneErrorRefusesBeforeAdd",
			producer:   battenrecipe.NameRunShed,
			fake:       fakeReadopt{gone: true, report: active, pruneErr: errors.New("prune blew up")},
			wantCalls:  []string{"gone", "reconcile", "prune"},
			wantErrors: []string{"prune blew up", rerun},
		},
		{
			name:       "TransportErrorFromAddRefuses",
			producer:   battenrecipe.NameRunShed,
			fake:       fakeReadopt{gone: true, report: active, addErr: errors.New("connection reset by peer")},
			wantCalls:  []string{"gone", "reconcile", "prune", "add"},
			wantErrors: []string{"connection reset by peer", rerun},
		},
		{
			name:       "TeardownLockWaitFromAddRefuses",
			producer:   battenrecipe.NameRunShed,
			fake:       fakeReadopt{gone: true, report: active, addErr: fmt.Errorf("take the teardown lock: %w", fabricengine.ErrPairPushRunning)},
			wantCalls:  []string{"gone", "reconcile", "prune", "add"},
			wantErrors: []string{fabricengine.ErrPairPushRunning.Error(), rerun},
		},
		{
			name:       "FailedWorktreeAddFromAddRefuses",
			producer:   battenrecipe.NameRunShed,
			fake:       fakeReadopt{gone: true, report: active, addErr: errors.New("create worktree /hub/some-slug failed: exit status 128")},
			wantCalls:  []string{"gone", "reconcile", "prune", "add"},
			wantErrors: []string{"create worktree", "failed", rerun},
		},
		{
			name:       "GoneErrorRefusesNamingItsCause",
			producer:   battenrecipe.NameRunShed,
			fake:       fakeReadopt{goneErr: errors.New("stat blew up")},
			wantCalls:  []string{"gone"},
			wantErrors: []string{"stat blew up", rerun},
		},
		{
			name:      "PresentTaskWorktreeRunsOnlyTheResumeReconcile",
			producer:  battenrecipe.NameRunShed,
			fake:      fakeReadopt{report: active},
			wantCalls: []string{"gone", "resume-reconcile"},
		},
		{
			name:      "WorktreeCreateNeverReachesTheSeams",
			producer:  battenrecipe.NameWorktreeCreate,
			fake:      fakeReadopt{gone: true, report: active},
			wantCalls: []string{"resume-reconcile"},
		},
		{
			name:     "WorktreeTeardownNeverReachesTheSeams",
			producer: battenrecipe.NameWorktreeTeardown,
			fake:     fakeReadopt{gone: true, report: active},
		},
		{
			name:             "RecordsNotLiveBeforeLandNamesTheCancel",
			producer:         battenrecipe.NameRunShed,
			fake:             fakeReadopt{gone: true, report: active, addErr: notLiveRecords},
			wantCalls:        []string{"gone", "reconcile", "prune", "add"},
			wantErrors:       []string{recordsBranch, "way forward", cancel},
			wantState:        shedengine.StateFailed,
			wantNotices:      1,
			wantCommitFailed: 1,
		},
		{
			name:             "RemoteLeftoverBeforeLandNamesTheCancel",
			producer:         battenrecipe.NameRunShed,
			fake:             fakeReadopt{gone: true, report: active, addErr: leftover},
			wantCalls:        []string{"gone", "reconcile", "prune", "add"},
			wantErrors:       []string{"way forward", cancel},
			wantState:        shedengine.StateFailed,
			wantNotices:      1,
			wantCommitFailed: 1,
		},
		{
			name:     "LandRecordsNotLiveRestoresFromTheNewestFinishedTag",
			producer: battenrecipe.NameLand,
			fake: fakeReadopt{gone: true, report: active, addErr: notLiveRecords,
				tags: []fabricengine.RecordsTag{olderDone, originDone, newerRunning, newestNoStatus}},
			wantCalls:        []string{"gone", "reconcile", "prune", "add"},
			wantErrors:       []string{recordsAt + "fetch origin tag " + originDone.Name, recordsAt + "branch " + recordsBranch + " " + originDone.Name, rerun},
			notErrors:        []string{"lym batten cancel"},
			wantState:        shedengine.StateFailed,
			wantNotices:      1,
			wantCommitFailed: 1,
		},
		{
			name:     "LandRecordsNotLiveReadsAFinishedStatusUnderTheLegacySegment",
			producer: battenrecipe.NameLand,
			fake: fakeReadopt{gone: true, report: active, addErr: notLiveRecords,
				tags: []fabricengine.RecordsTag{legacyDone, newerRunning}},
			wantCalls:        []string{"gone", "reconcile", "prune", "add"},
			wantErrors:       []string{recordsAt + "branch " + recordsBranch + " " + legacyDone.Name, rerun},
			notErrors:        []string{"fetch origin tag"},
			wantState:        shedengine.StateFailed,
			wantNotices:      1,
			wantCommitFailed: 1,
		},
		{
			name:             "LandRecordsNotLiveWithNoFinishedTagNamesFinishingByHand",
			producer:         battenrecipe.NameLand,
			fake:             fakeReadopt{gone: true, report: active, addErr: notLiveRecords, tags: []fabricengine.RecordsTag{newerRunning, newestNoStatus}},
			wantCalls:        []string{"gone", "reconcile", "prune", "add"},
			wantErrors:       []string{"no copy of the run's records", "by hand", "landed-run-left-paused"},
			notErrors:        []string{"task branch", recordsAt + "branch ", cancel},
			wantState:        shedengine.StateFailed,
			wantNotices:      1,
			wantCommitFailed: 1,
		},
		{
			name:       "LandRecordsNotLiveWithATagReadErrorRefusesAsTransient",
			producer:   battenrecipe.NameLand,
			fake:       fakeReadopt{gone: true, report: active, addErr: notLiveRecords, tagsErr: errors.New("tags blew up")},
			wantCalls:  []string{"gone", "reconcile", "prune", "add"},
			wantErrors: []string{"tags blew up", rerun},
			notErrors:  []string{"way forward: "},
		},
		{
			name:     "LandTaskBranchNotLiveRestoresOnlyTheTaskBranch",
			producer: battenrecipe.NameLand,
			fake: fakeReadopt{gone: true, report: active, addErr: notLiveTask, taskBranchGone: true,
				branchFile: []byte(`{"parent_branch":"main"}`), branchFileFound: true},
			wantCalls:        []string{"gone", "reconcile", "prune", "add"},
			wantErrors:       []string{"git -C {prime} branch " + slug + " main", rerun},
			notErrors:        []string{"fetch origin tag", recordsAt + "branch "},
			wantState:        shedengine.StateFailed,
			wantNotices:      1,
			wantCommitFailed: 1,
		},
		{
			name:     "LandRemoteLeftoverWithAnUpstreamUnsetsIt",
			producer: battenrecipe.NameLand,
			fake:     fakeReadopt{gone: true, report: active, addErr: leftover, upstream: true},
			wantErrors: []string{
				recordsAt + "fetch origin " + leftoverTip,
				recordsAt + "tag " + leftoverTag + " " + leftoverTip,
				recordsAt + "push origin " + leftoverTag,
				recordsAt + "push --force-with-lease=refs/heads/" + recordsBranch + ":" + leftoverTip + " origin --delete " + recordsBranch,
				recordsAt + "branch --unset-upstream " + recordsBranch,
				rerun,
			},
			notErrors:        []string{cancel},
			wantCalls:        []string{"gone", "reconcile", "prune", "add"},
			wantState:        shedengine.StateFailed,
			wantNotices:      1,
			wantCommitFailed: 1,
		},
		{
			name:     "LandRemoteLeftoverWithNoUpstreamAndTheTagStandingSkipsBoth",
			producer: battenrecipe.NameLand,
			fake: fakeReadopt{gone: true, report: active, addErr: leftover,
				tags: []fabricengine.RecordsTag{{Name: leftoverTag, Commit: leftoverTip}}},
			wantErrors:       []string{recordsAt + "push origin " + leftoverTag, rerun},
			notErrors:        []string{"--unset-upstream", recordsAt + "tag "},
			wantCalls:        []string{"gone", "reconcile", "prune", "add"},
			wantState:        shedengine.StateFailed,
			wantNotices:      1,
			wantCommitFailed: 1,
		},
		{
			name:             "RecordsWorktreeNotAPairNamesTheDirectoryAndTheRerun",
			producer:         battenrecipe.NameRunShed,
			fake:             fakeReadopt{gone: true, report: active, addErr: &fabricengine.ErrRecordsWorktreeNotPair{Path: "/hub/some-slug-records"}},
			wantCalls:        []string{"gone", "reconcile", "prune", "add"},
			wantErrors:       []string{"/hub/some-slug-records", recordsAt + "worktree remove --force /hub/some-slug-records", rerun},
			notErrors:        []string{cancel},
			wantState:        shedengine.StateFailed,
			wantNotices:      1,
			wantCommitFailed: 1,
		},
		{
			name:             "TaskBranchCheckedOutElsewhereNamesTheCheckout",
			producer:         battenrecipe.NameRunShed,
			fake:             fakeReadopt{gone: true, report: active, addErr: &fabricengine.ErrAdoptLocalCheckedOut{Branch: slug, Path: "/hub/other"}},
			wantCalls:        []string{"gone", "reconcile", "prune", "add"},
			wantErrors:       []string{"lym fabric checkout <other branch>", "/hub/other", rerun},
			notErrors:        []string{cancel, "--adopt-local"},
			wantState:        shedengine.StateFailed,
			wantNotices:      1,
			wantCommitFailed: 1,
		},
		{
			name:             "RecordsBranchCheckedOutElsewhereNamesTheSwitch",
			producer:         battenrecipe.NameRunShed,
			fake:             fakeReadopt{gone: true, report: active, addErr: &fabricengine.ErrAdoptLocalCheckedOut{Branch: recordsBranch, Path: "/hub/records-other", Records: true}},
			wantCalls:        []string{"gone", "reconcile", "prune", "add"},
			wantErrors:       []string{"git -C /hub/records-other switch --detach", rerun},
			notErrors:        []string{cancel, "--adopt-local"},
			wantState:        shedengine.StateFailed,
			wantNotices:      1,
			wantCommitFailed: 1,
		},
		{
			name:             "TaskPathTakenNamesThePathAndTheRerun",
			producer:         battenrecipe.NameRunShed,
			fake:             fakeReadopt{gone: true, report: active, addErr: &fabricengine.ErrTaskPathTaken{Path: "/hub/some-slug"}},
			wantCalls:        []string{"gone", "reconcile", "prune", "add"},
			wantErrors:       []string{"/hub/some-slug", "delete it", rerun},
			notErrors:        []string{cancel},
			wantState:        shedengine.StateFailed,
			wantNotices:      1,
			wantCommitFailed: 1,
		},
		{
			name:       "AFailedRunKeepsItsStateAndTellsNobody",
			producer:   battenrecipe.NameLand,
			state:      shedengine.StateFailed,
			fake:       fakeReadopt{gone: true, report: active, addErr: notLiveRecords, tags: []fabricengine.RecordsTag{originDone}},
			wantCalls:  []string{"gone", "reconcile", "prune", "add"},
			wantErrors: []string{"way forward", recordsAt + "branch " + recordsBranch + " " + originDone.Name},
			wantState:  shedengine.StateFailed,
		},
		{
			name:       "AHeldRunLockRefusesAsTransient",
			producer:   battenrecipe.NameRunShed,
			fake:       fakeReadopt{gone: true, report: active, addErr: notLiveRecords},
			lockHeld:   true,
			wantCalls:  []string{"gone", "reconcile", "prune", "add"},
			wantErrors: []string{"run lock", rerun},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := tt.fake
			c := newFakeReceiver(t, nil)
			c.readopt = fake.seams()
			c.reconcile = func(string) (fabricengine.OriginReport, error) {
				fake.calls = append(fake.calls, "resume-reconcile")
				return active, nil
			}
			before := tt.state
			if before == "" {
				before = shedengine.StateRunning
			}
			wantState := tt.wantState
			if wantState == "" {
				wantState = shedengine.StateRunning
			}
			status := shedengine.Status{CurrentProducer: tt.producer, State: before}
			writeStatus(t, c, status)
			if tt.lockHeld {
				held, acquired, err := lock.TryAcquireWriteLock(c.shedPaths.LockPath)
				if err != nil || !acquired {
					t.Fatalf("hold the run lock: acquired %v, %v", acquired, err)
				}
				defer func() { _ = held.Release() }()
			}

			err := c.ownerPreflight(true, status)

			if len(tt.wantErrors) == 0 && err != nil {
				t.Fatalf("ownerPreflight() = %v; want nil", err)
			}
			if len(tt.wantErrors) > 0 && err == nil {
				t.Fatalf("ownerPreflight() = nil; want a refusal carrying %q", tt.wantErrors)
			}
			expand := func(text string) string { return strings.ReplaceAll(text, "{prime}", c.location.WorktreePath()) }
			for _, want := range tt.wantErrors {
				if !strings.Contains(err.Error(), expand(want)) {
					t.Errorf("ownerPreflight() = %q; want it to carry %q", err.Error(), expand(want))
				}
			}
			for _, unwanted := range tt.notErrors {
				if strings.Contains(err.Error(), expand(unwanted)) {
					t.Errorf("ownerPreflight() = %q; want it not to carry %q", err.Error(), expand(unwanted))
				}
			}
			var owned *OwnedElsewhereError
			if got := errors.As(err, &owned); got != tt.wantOwned {
				t.Errorf("ownerPreflight() = %v; errors.As(*OwnedElsewhereError) = %v, want %v", err, got, tt.wantOwned)
			}
			if !slices.Equal(fake.calls, tt.wantCalls) {
				t.Errorf("seam calls = %v; want %v", fake.calls, tt.wantCalls)
			}
			after, found, readErr := state.ReadJSON[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
			if readErr != nil || !found || after.State != wantState {
				t.Errorf("status after ownerPreflight = %+v, found %v, %v; want state %s", after, found, readErr, wantState)
			}
			if len(fake.notices) != tt.wantNotices || fake.commitFailed != tt.wantCommitFailed {
				t.Errorf("notices = %q and failed-stop commits = %d; want %d and %d", fake.notices, fake.commitFailed, tt.wantNotices, tt.wantCommitFailed)
			}
			if owed, owedErr := OriginOwed(c.location, c.slug); owedErr != nil || owed {
				t.Errorf("OriginOwed() = %v, %v; want false, nil", owed, owedErr)
			}
		})
	}
}
