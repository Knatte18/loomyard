// land_test.go drives the land and ready sequences over fake deps: the entries that are not a landable beam, the pawl run, owner and ready refusals, the work-remaining refusal passed through, the land request written for a beam waiting at Beam-Await, and the ready record written once.

package beamcli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/reviewbudget"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/prkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// landNow is the time the fake hub's clock reads.
var landNow = time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)

// landWorld is the fake hub every landDeps seam reads, and the log of the land requests written.
type landWorld struct {
	owners  boardengine.Owners
	parents map[string]string
	pairs   map[string]bool
	origins map[string]fabricengine.Origin
	// children holds each beam's pawl run status; a beam with no entry has no pawl run.
	children map[string]shedengine.Status
	// ready holds each beam whose ready record flags it, marked the ready records written and markCalls the MarkReady calls, which commit a flagged beam's record too.
	// readyErr and markErr are what the Ready and MarkReady seams fail with.
	ready      map[string]bool
	marked     []pawlengine.ReadyRecord
	markCalls  int
	readyErr   error
	markErr    error
	runID      string
	refusal    error
	refusedFor []string
	requests   []pawlengine.LandRequest
	// tasks is the board's entries, filed the fields of each task FileTask filed and caps the hub's Code-Review budget.
	tasks []boardengine.Task
	filed []map[string]any
	caps  reviewbudget.Caps
	// described holds the slug of each Describe call, and describeErr is what the Describe seam fails with.
	described   []string
	describeErr error
	// api is the fake GitHub every pull request side holds, and origin and sideErr the facts and the failure PullRequestSide answers with.
	api     *prkit.API
	origin  landingshed.Origin
	sideErr error
	// pushed counts the PushBeam calls, which answer pushHead or pushErr.
	pushed   int
	pushHead string
	pushErr  error
	// hasCommits and commitsErr are what BeamHasCommits answers.
	hasCommits bool
	commitsErr error
	// beamHead is what BeamHead answers.
	beamHead string
	// mode, rulesRequired and modeErr are what PullRequestMode answers.
	mode          string
	rulesRequired bool
	modeErr       error
	// slept is the time the fake clock has advanced by Sleep, which every read of the merge state waits through.
	slept time.Duration
	// parentUnmerged is what ParentUnmerged answers, findings what DescriptionFindings answers and mergedLacks what MergedLacks answers.
	parentUnmerged bool
	findings       []string
	mergedLacks    []string
	// apiErr, when set, leaves the pull request side with no API and this as the reason.
	apiErr error
	// landing holds each beam's landing record, and writes counts the WriteLanding calls, which fail with writeErr.
	landing  map[string]pawlengine.LandingRecord
	writes   int
	writeErr error
	// title and body are the change description Description reads.
	title string
	body  string
}

// newLandWorld returns a hub owned by here with a beam "b" forked from main whose pair is here and marked and whose pawl run waits at Beam-Await, a task, a grouping project and a done beam.
func newLandWorld() *landWorld {
	return &landWorld{
		owners: boardengine.Owners{Prime: here, Entries: map[string]boardengine.OwnerEntry{
			"b":      {IsProject: true, Beam: true, Owner: here},
			"task1":  {IsTask: true},
			"group":  {IsProject: true},
			"landed": {IsProject: true, Beam: true, Done: true},
			"note1":  {},
		}},
		parents:  map[string]string{},
		pairs:    map[string]bool{"b": true, "landed": true},
		origins:  map[string]fabricengine.Origin{"b": {Beam: true, ParentBranch: "main"}, "landed": {Beam: true}},
		children: map[string]shedengine.Status{"b": {State: shedengine.StateRunning, CurrentProducer: pawlengine.NameBeamAwait}},
		runID:    "2026-02-28T08:00:00Z",
		ready:    map[string]bool{"b": true},
		tasks: []boardengine.Task{
			{Slug: "b", Kind: boardengine.KindProject, Labels: []string{"enhancement", "reed"}},
			{Slug: "task1", Kind: boardengine.KindTask, Project: "b"},
		},
		api:        &prkit.API{},
		origin:     landingshed.Origin{URL: "https://github.com/o/r.git", Upstream: true},
		pushHead:   "head1",
		hasCommits: true,
		beamHead:   "head1",
		mode:       "off",
		landing:    map[string]pawlengine.LandingRecord{"b": {Slug: "b", DescribedAt: landNow}},
		title:      "Ship the beam",
		body:       "Groups the work.",
	}
}

func (w *landWorld) deps() landDeps {
	return landDeps{
		Machine:     func() (string, error) { return here, nil },
		ReadOwners:  func() (boardengine.Owners, error) { return w.owners, nil },
		ParentOf:    func(slug string) (string, error) { return w.parents[slug], nil },
		PairPresent: func(slug string) bool { return w.pairs[slug] },
		OriginRecord: func(slug string) (fabricengine.Origin, bool, error) {
			origin, found := w.origins[slug]
			return origin, found, nil
		},
		ChildStatus: func(slug string) (shedengine.Status, bool, error) {
			status, found := w.children[slug]
			return status, found, nil
		},
		RunID: func(string) (string, error) { return w.runID, nil },
		Ready: func(slug string) (bool, error) { return w.ready[slug], w.readyErr },
		MarkReady: func(slug string, record pawlengine.ReadyRecord) (string, error) {
			w.markCalls++
			if w.markErr != nil {
				return "", w.markErr
			}
			if !w.ready[slug] {
				w.marked = append(w.marked, record)
			}
			return "/hub/" + slug + "/_lym/landing/beam-ready.json", nil
		},
		Refusal: func(slug string) error {
			w.refusedFor = append(w.refusedFor, slug)
			return w.refusal
		},
		WriteRequest: func(slug string, req pawlengine.LandRequest) (string, error) {
			w.requests = append(w.requests, req)
			return "/hub/" + slug + "/.lym/beam/land-request.json", nil
		},
		Now:   func() time.Time { return landNow.Add(w.slept) },
		Tasks: func() ([]boardengine.Task, error) { return w.tasks, nil },
		FileTask: func(fields map[string]any) error {
			w.filed = append(w.filed, fields)
			return nil
		},
		CodeReviewCaps: func() (reviewbudget.Caps, error) { return w.caps, nil },
		Describe: func(slug string, _ time.Time) (string, error) {
			w.described = append(w.described, slug)
			if w.describeErr != nil {
				return "", w.describeErr
			}
			return "/hub/" + slug + "/_lym/landing/summary.md", nil
		},
		PullRequestSide: func(string) (beamPullRequestSide, error) {
			side := beamPullRequestSide{API: w.api, Origin: w.origin, Branch: "b", ParentBranch: "main"}
			if w.apiErr != nil {
				side.API, side.APIErr = nil, w.apiErr
			}
			return side, w.sideErr
		},
		PushBeam: func(string) (string, error) {
			w.pushed++
			return w.pushHead, w.pushErr
		},
		BeamHasCommits:  func(string) (bool, error) { return w.hasCommits, w.commitsErr },
		BeamHead:        func(string) (string, error) { return w.beamHead, nil },
		PullRequestMode: func(string) (string, bool, error) { return w.mode, w.rulesRequired, w.modeErr },
		Sleep: func(_ context.Context, d time.Duration) error {
			w.slept += d
			return nil
		},
		ParentUnmerged:      func(string) (bool, error) { return w.parentUnmerged, nil },
		DescriptionFindings: func(string) ([]string, error) { return w.findings, nil },
		MergedLacks:         func(string, string) ([]string, error) { return w.mergedLacks, nil },
		Landing: func(slug string) (pawlengine.LandingRecord, bool, error) {
			record, found := w.landing[slug]
			return record, found, nil
		},
		WriteLanding: func(slug string, mutate func(rec *pawlengine.LandingRecord) error) error {
			w.writes++
			if w.writeErr != nil {
				return w.writeErr
			}
			record, found := w.landing[slug]
			if !found {
				record = pawlengine.LandingRecord{Slug: slug}
			}
			if err := mutate(&record); err != nil {
				return err
			}
			w.landing[slug] = record
			return nil
		},
		Description: func(string) (string, string, error) { return w.title, w.body, nil },
	}
}

func TestLand(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name  string
		slug  string
		setup func(w *landWorld)
		// wantErr is empty for a land that writes its request, else every substring the refusal names.
		wantErr []string
		// wantRefusalRead says the work-remaining refusal was applied.
		wantRefusalRead bool
	}{
		{name: "AbsentEntryRefused", slug: "ghost", wantErr: []string{"not a beam", "lym board list"}},
		{name: "NoteRefused", slug: "note1", wantErr: []string{"not a beam"}},
		{name: "TaskRefusedNamingItsOwnLanding", slug: "task1", wantErr: []string{"lym batten run task1", "lym board upsert", "lym beam create <that slug>"}},
		{name: "GroupingProjectRefusedNamingCreate", slug: "group", wantErr: []string{"grouping project", "lym beam create group"}},
		{name: "DoneBeamRefused", slug: "landed", wantErr: []string{"already landed"}},
		{
			name: "BeamWithNoPairHereRefusedNamingCreateAndAdopt",
			slug: "b",
			setup: func(w *landWorld) {
				w.pairs["b"] = false
			},
			wantErr: []string{"no pair on this machine", "lym beam create b", "lym adopt b"},
		},
		{
			name: "PairWithNoOriginRecordRefusedNamingReconcile",
			slug: "b",
			setup: func(w *landWorld) {
				delete(w.origins, "b")
			},
			wantErr: []string{"lym fabric reconcile"},
		},
		{
			name: "UnmarkedPairRefusedNamingCreate",
			slug: "b",
			setup: func(w *landWorld) {
				w.origins["b"] = fabricengine.Origin{}
			},
			wantErr: []string{"beam mark", "lym beam create b"},
		},
		{
			name: "BeamWithNoPawlRunRefusedNamingCreate",
			slug: "b",
			setup: func(w *landWorld) {
				delete(w.children, "b")
			},
			wantErr: []string{"no pawl run", "lym beam create b"},
		},
		{
			name: "RunAwayFromBeamAwaitRefusedNamingStatus",
			slug: "b",
			setup: func(w *landWorld) {
				w.children["b"] = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Parent-Align"}
			},
			wantErr: []string{"Parent-Align", pawlengine.NameBeamAwait, "lym batten status b"},
		},
		{
			name: "PausedRunAtBeamAwaitRefusedNamingStatus",
			slug: "b",
			setup: func(w *landWorld) {
				w.children["b"] = shedengine.Status{State: shedengine.StatePaused, CurrentProducer: pawlengine.NameBeamAwait}
			},
			wantErr: []string{"lym batten status b"},
		},
		{
			name: "BeamOwnedElsewhereRefusedNamingAdopt",
			slug: "b",
			setup: func(w *landWorld) {
				w.owners.Entries["b"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there}
			},
			wantErr: []string{"owned by machine there", "lym adopt b"},
		},
		{
			name: "ParentOwnedElsewhereRefusedNamingRootAdopt",
			slug: "b",
			setup: func(w *landWorld) {
				w.owners.Prime = there
				w.owners.Entries["b"] = boardengine.OwnerEntry{IsProject: true, Beam: true}
			},
			wantErr: []string{"parent of b is owned by machine there", "lym adopt root"},
		},
		{
			name: "NestedBeamsParentOwnedElsewhereRefusedNamingItsBeam",
			slug: "inner",
			setup: func(w *landWorld) {
				w.owners.Entries["outer"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there}
				w.owners.Entries["inner"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: here}
				w.parents["inner"] = "outer"
				w.pairs["inner"] = true
				w.origins["inner"] = fabricengine.Origin{Beam: true}
				w.children["inner"] = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: pawlengine.NameBeamAwait}
			},
			wantErr: []string{"parent of inner is owned by machine there", "lym adopt outer"},
		},
		{
			name: "UnflaggedBeamRefusedNamingReady",
			slug: "b",
			setup: func(w *landWorld) {
				delete(w.ready, "b")
			},
			wantErr: []string{"not flagged ready", "lym beam ready b"},
		},
		{
			name: "OwnerRefusalComesBeforeTheReadyRefusal",
			slug: "b",
			setup: func(w *landWorld) {
				w.owners.Entries["b"] = boardengine.OwnerEntry{IsProject: true, Beam: true, Owner: there}
				delete(w.ready, "b")
			},
			wantErr: []string{"owned by machine there", "lym adopt b"},
		},
		{
			name: "ReadyRefusalComesBeforeTheWorkRemainingRefusal",
			slug: "b",
			setup: func(w *landWorld) {
				w.refusal = errors.New("the open entry m is a member of the beam")
				delete(w.ready, "b")
			},
			wantErr: []string{"not flagged ready", "lym beam ready b"},
		},
		{
			name: "UnreadableReadyRecordRefusedNamingLand",
			slug: "b",
			setup: func(w *landWorld) {
				w.readyErr = errors.New("the record is corrupt")
			},
			wantErr: []string{"the record is corrupt", "lym beam land b"},
		},
		{
			name: "WorkRemainingRefusalPassesThroughAndWritesNothing",
			slug: "b",
			setup: func(w *landWorld) {
				w.refusal = errors.New("the open entry m is a member of the beam")
			},
			wantErr:         []string{"the open entry m is a member of the beam"},
			wantRefusalRead: true,
		},
		{name: "CleanBeamWritesOneRequest", slug: "b", wantRefusalRead: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newLandWorld()
			if tt.setup != nil {
				tt.setup(w)
			}

			got, err := land(w.deps(), tt.slug)

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("land(%q) = nil; want a refusal naming %v", tt.slug, tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("land(%q) error = %q; want it to name %q", tt.slug, err.Error(), want)
					}
				}
				if strings.Contains(err.Error(), "lym beam promote") {
					t.Errorf("land(%q) error = %q; want no lym beam promote", tt.slug, err.Error())
				}
				if len(w.requests) != 0 {
					t.Errorf("land(%q) wrote %d requests on a refusal; want none", tt.slug, len(w.requests))
				}
			} else {
				if err != nil {
					t.Fatalf("land(%q) = %v; want the request written", tt.slug, err)
				}
				if got["request"] != "/hub/b/.lym/beam/land-request.json" || got["run_id"] != w.runID || !strings.Contains(got["way_forward"].(string), "lym batten status b") {
					t.Errorf("land(%q) = %v; want the request's path, the run id and `lym batten status b`", tt.slug, got)
				}
				want := []pawlengine.LandRequest{{RunID: w.runID, RequestedAt: landNow}}
				if len(w.requests) != 1 || w.requests[0] != want[0] {
					t.Errorf("land(%q) wrote requests %v; want exactly %v", tt.slug, w.requests, want)
				}
			}
			if read := len(w.refusedFor) > 0; read != tt.wantRefusalRead {
				t.Errorf("land(%q) applied the work-remaining refusal = %v; want %v", tt.slug, read, tt.wantRefusalRead)
			}
		})
	}
}

func TestMarkReady(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// unflagged runs setup on a beam whose ready record does not stand.
	unflagged := func(setup func(w *landWorld)) func(w *landWorld) {
		return func(w *landWorld) {
			delete(w.ready, "b")
			setup(w)
		}
	}
	tests := []struct {
		name  string
		slug  string
		setup func(w *landWorld)
		// wantErr is empty for a ready that succeeds, else every substring the refusal names, and wantNoErr every one it must not name.
		wantErr   []string
		wantNoErr []string
		// wantAlready says the beam read as flagged before the call, so nothing is written.
		wantAlready bool
		// wantMarkCalls is how many times the MarkReady seam, which writes an absent record and commits it, ran.
		wantMarkCalls int
		// stripDeps removes seams from the deps, so a call to one panics.
		stripDeps func(d *landDeps)
	}{
		{name: "TaskRefusedNamingItsOwnLanding", slug: "task1", wantErr: []string{"lym beam ready:", "lym batten run task1"}},
		{name: "GroupingProjectRefusedNamingCreate", slug: "group", wantErr: []string{"lym beam ready:", "lym beam create group"}},
		{
			name: "BeamWithNoPairHereRefusedNamingCreateAndAdopt",
			slug: "b",
			setup: func(w *landWorld) {
				w.pairs["b"] = false
			},
			wantErr: []string{"lym beam ready:", "lym beam create b", "lym adopt b"},
		},
		{
			name: "UnflaggedBeamWritesOneRecordNamingIt",
			slug: "b",
			setup: func(w *landWorld) {
				delete(w.ready, "b")
			},
			wantMarkCalls: 1,
		},
		{
			name:          "FlaggedBeamWritesNothingAndStillCommitsWithNoJudgingSeam",
			slug:          "b",
			wantAlready:   true,
			wantMarkCalls: 1,
			stripDeps: func(d *landDeps) {
				d.ChildStatus, d.Refusal, d.ParentUnmerged, d.Landing, d.DescriptionFindings = nil, nil, nil, nil, nil
				d.PullRequestMode, d.BeamHasCommits, d.PullRequestSide, d.BeamHead, d.MergedLacks, d.Tasks = nil, nil, nil, nil, nil, nil
			},
		},
		{
			name: "RunAtFinalizeRefusedNamingStatus",
			slug: "b",
			setup: unflagged(func(w *landWorld) {
				w.children["b"] = shedengine.Status{State: shedengine.StateRunning, CurrentProducer: "Finalize"}
			}),
			wantErr: []string{"Finalize", "lym batten status b"},
		},
		{
			name:    "OpenMemberRefused",
			slug:    "b",
			setup:   unflagged(func(w *landWorld) { w.refusal = errors.New("the open entry m is a member of the beam") }),
			wantErr: []string{"the open entry m is a member of the beam"},
		},
		{
			name:    "ParentTipNotAnAncestorRefusedNamingMergeParent",
			slug:    "b",
			setup:   unflagged(func(w *landWorld) { w.parentUnmerged = true }),
			wantErr: []string{"lym beam merge-parent b"},
		},
		{
			name: "ParentTipNotAnAncestorWithAnOpenMergeTaskNamesTheTask",
			slug: "b",
			setup: unflagged(func(w *landWorld) {
				w.parentUnmerged = true
				w.tasks = append(w.tasks, boardengine.Task{Slug: "b-merge-in", Kind: boardengine.KindTask, Project: "b"})
			}),
			wantErr:   []string{"lym batten status b-merge-in"},
			wantNoErr: []string{"lym beam merge-parent"},
		},
		{
			name:    "NoDescriptionStampedRefusedNamingDescribe",
			slug:    "b",
			setup:   unflagged(func(w *landWorld) { w.landing["b"] = pawlengine.LandingRecord{Slug: "b"} }),
			wantErr: []string{"lym beam describe b"},
		},
		{
			name:    "DescriptionFindingRefusedNamingItAndDescribe",
			slug:    "b",
			setup:   unflagged(func(w *landWorld) { w.findings = []string{"empty-body: body is empty"} }),
			wantErr: []string{"empty-body: body is empty", "lym beam describe b"},
		},
		{
			name:    "RequiredParentWithNoneRecordedRefusedNamingOpen",
			slug:    "b",
			setup:   unflagged(func(w *landWorld) { w.mode = "required" }),
			wantErr: []string{"lym beam pr open b"},
		},
		{
			name: "RulesRequiredOnParentWithNoneRecordedRefusedNamingOpen",
			slug: "b",
			setup: unflagged(func(w *landWorld) {
				w.mode = "on"
				w.rulesRequired = true
			}),
			wantErr: []string{"lym beam pr open b"},
		},
		{
			name: "RequiredParentWithNoUpstreamRefusedNamingTheFactAndReady",
			slug: "b",
			setup: unflagged(func(w *landWorld) {
				w.mode = "required"
				w.origin = landingshed.Origin{URL: "https://github.com/o/r.git"}
			}),
			wantErr:   []string{"no upstream", "lym beam ready b"},
			wantNoErr: []string{"lym loom resume"},
		},
		{
			name: "MergeParentWithTheSameMissingFactRefused",
			slug: "b",
			setup: unflagged(func(w *landWorld) {
				w.mode = "merge"
				w.origin = landingshed.Origin{URL: "https://github.com/o/r.git"}
			}),
			wantErr:   []string{"no upstream", "lym beam ready b"},
			wantNoErr: []string{"lym loom resume"},
		},
		{
			name:    "FailingPullRequestModeRefusedNamingTheCauseAndReady",
			slug:    "b",
			setup:   unflagged(func(w *landWorld) { w.modeErr = errors.New("beam.yaml is unreadable") }),
			wantErr: []string{"beam.yaml is unreadable", "lym beam ready b"},
		},
		{
			name:    "FailingBeamHasCommitsRefusedNamingTheCauseAndReady",
			slug:    "b",
			setup:   unflagged(func(w *landWorld) { w.commitsErr = errors.New("the pair is unreadable") }),
			wantErr: []string{"the pair is unreadable", "lym beam ready b"},
		},
		{
			name: "RecordedPullRequestWhoseStatusFailsRefusedNamingTheCauseAndReady",
			slug: "b",
			setup: unflagged(func(w *landWorld) {
				recordPullRequest(w)
				w.api.Script(prkit.MethodStatus, prkit.Answer{Err: errors.New("GitHub is down")})
			}),
			wantErr: []string{"GitHub is down", "lym beam ready b"},
		},
		{
			name: "RecordedHeadOtherThanTheBeamsRefusedNamingUpdate",
			slug: "b",
			setup: unflagged(func(w *landWorld) {
				recordPullRequest(w)
				other := openPullRequestAtBeamHead
				other.HeadSHA = "other"
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: other})
			}),
			wantErr: []string{"lym beam pr update b"},
		},
		{
			name: "BehindRefusedNamingMergeParentAndStatus",
			slug: "b",
			setup: unflagged(func(w *landWorld) {
				recordPullRequest(w)
				behind := openPullRequestAtBeamHead
				behind.MergeableState = "behind"
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: behind})
			}),
			wantErr: []string{"lym beam merge-parent b", "lym beam pr status b"},
		},
		{
			name: "BlockedRefusedNamingTheOwedCheck",
			slug: "b",
			setup: unflagged(func(w *landWorld) {
				recordPullRequest(w)
				blocked := openPullRequestAtBeamHead
				blocked.MergeableState = "blocked"
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: blocked})
				w.api.Script(prkit.MethodRequiredChecks, prkit.Answer{Checks: []pullrequest.Check{{Name: "ci"}}})
			}),
			wantErr: []string{"required check ci", "lym beam pr status b"},
		},
		{
			name: "ClosedRefusedNamingOpenAndClose",
			slug: "b",
			setup: unflagged(func(w *landWorld) {
				recordPullRequest(w)
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateClosed, HeadSHA: "head1"}})
			}),
			wantErr: []string{"lym beam pr open b", "lym beam pr close b"},
		},
		{
			name: "MergedLackingABeamCommitRefusedNamingItAndClose",
			slug: "b",
			setup: unflagged(func(w *landWorld) {
				recordPullRequest(w)
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateMerged, HeadSHA: "old"}})
				w.mergedLacks = []string{"abc1234 add widget"}
			}),
			wantErr: []string{"abc1234 add widget", "lym beam pr close b"},
		},
		{
			name: "MergedAtTheBeamsHeadWritesTheRecord",
			slug: "b",
			setup: unflagged(func(w *landWorld) {
				recordPullRequest(w)
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: pullrequest.Status{State: pullrequest.StateMerged, HeadSHA: "head1"}})
			}),
			wantMarkCalls: 1,
		},
		{
			name: "OpenCleanPullRequestWritesTheRecord",
			slug: "b",
			setup: unflagged(func(w *landWorld) {
				recordPullRequest(w)
				w.api.Script(prkit.MethodStatus, prkit.Answer{Status: openPullRequestAtBeamHead})
			}),
			wantMarkCalls: 1,
		},
		{
			name:          "MergeParentWithNoneRecordedWritesTheRecord",
			slug:          "b",
			setup:         unflagged(func(w *landWorld) { w.mode = "merge" }),
			wantMarkCalls: 1,
		},
		{
			name: "RequiredParentWithNoCommitsBeyondItsParentWritesTheRecord",
			slug: "b",
			setup: unflagged(func(w *landWorld) {
				w.mode = "required"
				w.hasCommits = false
			}),
			wantMarkCalls: 1,
		},
		{
			name:          "UnusableTokenWithNoneRecordedWritesTheRecord",
			slug:          "b",
			setup:         unflagged(func(w *landWorld) { w.apiErr = githubclient.ErrTokenUnresolvable }),
			wantMarkCalls: 1,
		},
		{
			name: "UnreadableReadyRecordRefusedNamingReady",
			slug: "b",
			setup: func(w *landWorld) {
				w.readyErr = errors.New("the record is corrupt")
			},
			wantErr: []string{"the record is corrupt", "lym beam ready b"},
		},
		{
			name: "FailedRecordWriteOrCommitRefusedNamingReady",
			slug: "b",
			setup: func(w *landWorld) {
				delete(w.ready, "b")
				w.markErr = errors.New("the commit failed")
			},
			wantErr:       []string{"the commit failed", "lym beam ready b"},
			wantMarkCalls: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := newLandWorld()
			if tt.setup != nil {
				tt.setup(w)
			}

			deps := w.deps()
			if tt.stripDeps != nil {
				tt.stripDeps(&deps)
			}
			got, err := markReady(deps, tt.slug)

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("markReady(%q) = nil; want a refusal naming %v", tt.slug, tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("markReady(%q) error = %q; want it to name %q", tt.slug, err.Error(), want)
					}
				}
				for _, unwanted := range tt.wantNoErr {
					if strings.Contains(err.Error(), unwanted) {
						t.Errorf("markReady(%q) error = %q; want it not to name %q", tt.slug, err.Error(), unwanted)
					}
				}
				if len(w.marked) != 0 {
					t.Errorf("markReady(%q) wrote %d records on a refusal; want none", tt.slug, len(w.marked))
				}
				if w.markCalls != tt.wantMarkCalls {
					t.Errorf("markReady(%q) ran MarkReady %d times; want %d", tt.slug, w.markCalls, tt.wantMarkCalls)
				}
				return
			}
			if w.markCalls != tt.wantMarkCalls {
				t.Errorf("markReady(%q) ran MarkReady %d times; want %d", tt.slug, w.markCalls, tt.wantMarkCalls)
			}
			if err != nil {
				t.Fatalf("markReady(%q) = %v; want nil", tt.slug, err)
			}
			if got["already"] != tt.wantAlready || !strings.Contains(got["way_forward"].(string), "lym batten status b") {
				t.Errorf("markReady(%q) = %v; want already %t and `lym batten status b`", tt.slug, got, tt.wantAlready)
			}
			var want []pawlengine.ReadyRecord
			if !tt.wantAlready {
				want = []pawlengine.ReadyRecord{{Slug: "b", SetAt: landNow}}
			}
			if !reflect.DeepEqual(w.marked, want) {
				t.Errorf("markReady(%q) wrote records %v; want %v", tt.slug, w.marked, want)
			}
		})
	}
}
