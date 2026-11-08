// prreview_session_test.go covers the PR-Review row's interactive session over a fake session runner.
// It pins what the session starts under, and when it reuses, resumes, stops and restarts it.
// It also holds the fake and helpers the other PR-Review tests share.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakePRSessions is a fake session runner that records every start and stop and tracks which strands are live.
// A start marks the strand its result names live, and a stop clears it.
type fakePRSessions struct {
	mu sync.Mutex
	// result is what every start answers.
	result      shuttleengine.OpenResult
	startErr    error
	liveErr     error
	registerErr error
	// stopErr is what every stop fails with; a failed stop leaves the strand live.
	stopErr    error
	live       map[string]bool
	starts     []shuttleengine.Spec
	stops      []string
	registered []string
}

var _ prSessions = (*fakePRSessions)(nil)

// newFakePRSessions returns a fake whose starts answer guid-1, named pr-strand, on session sess-1 of the claude engine.
func newFakePRSessions() *fakePRSessions {
	return &fakePRSessions{
		result: shuttleengine.OpenResult{StrandGUID: "guid-1", StrandName: "pr-strand", SessionID: "sess-1", Engine: "claude"},
		live:   map[string]bool{},
	}
}

func (f *fakePRSessions) StartSession(spec shuttleengine.Spec) (shuttleengine.OpenResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts = append(f.starts, spec)
	if f.startErr != nil {
		return shuttleengine.OpenResult{}, f.startErr
	}
	f.live[f.result.StrandGUID] = true
	return f.result, nil
}

func (f *fakePRSessions) StopStrand(guid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops = append(f.stops, guid)
	if f.stopErr != nil {
		return f.stopErr
	}
	f.live[guid] = false
	return nil
}

func (f *fakePRSessions) isLive(guid string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.live[guid], f.liveErr
}

func (f *fakePRSessions) register(guid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.registered = append(f.registered, guid)
	return f.registerErr
}

// sessionsOf returns the fake session runner prReviewTestDeps put in deps.
func sessionsOf(deps Deps) *fakePRSessions {
	return deps.PullRequest.Sessions.(*fakePRSessions)
}

// withSessionOf returns got with the session fields of want.
// It lets a test pin the rest of a record without pinning the session's identity.
func withSessionOf(got, want pullrequest.Record) pullrequest.Record {
	got.StrandGUID, got.StrandName, got.SessionID, got.Engine = want.StrandGUID, want.StrandName, want.SessionID, want.Engine
	return got
}

// assertSessionCounts fails the test unless the fake session runner of deps saw the given number of starts and stops.
func assertSessionCounts(t *testing.T, deps Deps, starts, stops int) {
	t.Helper()
	sessions := sessionsOf(deps)
	if len(sessions.starts) != starts || len(sessions.stops) != stops {
		t.Errorf("session starts = %d, stops = %d; want %d and %d", len(sessions.starts), len(sessions.stops), starts, stops)
	}
}

func TestPRReview_Session(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	githubOrigin := Origin{URL: "https://github.com/acme/widgets.git", Upstream: true}
	const recordedURL = "https://github.com/acme/widgets/pull/7"
	open := scriptedCall{status: pullrequest.Status{State: pullrequest.StateOpen}}
	closed := scriptedCall{status: pullrequest.Status{State: pullrequest.StateClosed}}
	mergedAtHead := scriptedCall{status: pullrequest.Status{State: pullrequest.StateMerged, HeadSHA: "head-new"}}
	created := scriptedCall{ref: pullrequest.Ref{Number: 7, URL: recordedURL}}
	recordedAtHead := func(mutate ...func(*pullrequest.Record)) *pullrequest.Record {
		rec := &pullrequest.Record{Number: 7, URL: recordedURL, GatedHead: "head-new"}
		for _, m := range mutate {
			m(rec)
		}
		return rec
	}
	withStrand := func(guid, sessionID string) func(*pullrequest.Record) {
		return func(r *pullrequest.Record) { r.StrandGUID, r.SessionID = guid, sessionID }
	}
	cancelAtFirstSleep := 1
	parentReviewOn := func(_ *testing.T, fx *prReviewFixture, _ *fakePRSessions) {
		fx.deps.PullRequest.ParentReviewWanted = func() (bool, error) { return true, nil }
	}
	// assertAsks fails the test unless the row handed the parent-ask seam wantAsks lines and the record says wantAsked.
	assertAsks := func(t *testing.T, fx *prReviewFixture, wantAsks int, wantAsked bool) {
		t.Helper()
		if len(fx.asks) != wantAsks {
			t.Errorf("parent asks = %q; want %d", fx.asks, wantAsks)
		}
		got, found, err := pullrequest.ReadRecord(fx.deps.PullRequest.RecordPath, fx.deps.PullRequest.RecordLockPath)
		if err != nil || !found || got.ParentReviewAsked != wantAsked {
			t.Errorf("ReadRecord() = %+v, %v, %v; want the record with ParentReviewAsked %v", got, found, err, wantAsked)
		}
	}

	tests := []struct {
		name    string
		record  *pullrequest.Record
		scripts map[string][]scriptedCall
		setup   func(t *testing.T, fx *prReviewFixture, s *fakePRSessions)
		// cancelAtSleep cancels the context at that 1-based sleep, ending a wait that would otherwise go on.
		cancelAtSleep int
		// coveredAt answers whether a pause covers the run at the n-th read of the pause record.
		coveredAt func(n int) bool

		wantOutcome  shedengine.Outcome
		wantHalt     bool
		wantReason   []string
		wantErr      bool
		wantCanceled bool
		wantStarts   int
		wantStops    int
		check        func(t *testing.T, fx *prReviewFixture, s *fakePRSessions)
	}{
		{
			name: "FirstEntryStartsOneSessionWithPathsOnlyRegistersItAndRecordsIt", scripts: map[string][]scriptedCall{"Create": {created}, "Status": {mergedAtHead}},
			wantOutcome: shedengine.Done, wantStarts: 1, wantStops: 1,
			check: func(t *testing.T, fx *prReviewFixture, s *fakePRSessions) {
				spec := s.starts[0]
				if !spec.Interactive || !spec.Unawaited || !spec.DenyPublish {
					t.Errorf("spec Interactive = %v, Unawaited = %v, DenyPublish = %v; want all true", spec.Interactive, spec.Unawaited, spec.DenyPublish)
				}
				if spec.Role != prReviewerRole || spec.Segment != segmentcolor.Landing || spec.Engine != "claude" || spec.Model != "fixture-model" || spec.ResumeSessionID != "" {
					t.Errorf("spec role %q, segment %q, engine %q, model %q, resume %q; want the pr-reviewer role, the landing segment, the pr_reviewer model and no resume", spec.Role, spec.Segment, spec.Engine, spec.Model, spec.ResumeSessionID)
				}
				if spec.ConfigRole != agentconfig.RolePRReviewer {
					t.Errorf("spec ConfigRole = %q; want %q", spec.ConfigRole, agentconfig.RolePRReviewer)
				}
				if len(spec.Skills) != 0 {
					t.Errorf("spec skills = %q; want none beyond the role's", spec.Skills)
				}
				for _, want := range []string{"the-slug", recordedURL, "/wt/task", "`task`", "`main`", fx.deps.DescriptionPath, "parent-session", noDriftVerdict} {
					if !strings.Contains(spec.Prompt, want) {
						t.Errorf("prompt lacks %q:\n%s", want, spec.Prompt)
					}
				}
				if strings.Contains(spec.Prompt, "Change body") {
					t.Errorf("prompt holds the change description's content; want its path only:\n%s", spec.Prompt)
				}
				if diff := cmp.Diff([]string{"guid-1"}, s.registered); diff != "" {
					t.Errorf("registered strands mismatch (-want +got):\n%s", diff)
				}
				got, found, err := pullrequest.ReadRecord(fx.deps.PullRequest.RecordPath, fx.deps.PullRequest.RecordLockPath)
				if err != nil || !found {
					t.Fatalf("ReadRecord() = %+v, %v, %v; want the record", got, found, err)
				}
				if got.StrandGUID != "guid-1" || got.StrandName != "pr-strand" || got.SessionID != "sess-1" || got.Engine != "claude" {
					t.Errorf("record session = %q, %q, %q, %q; want the started session's identity", got.StrandGUID, got.StrandName, got.SessionID, got.Engine)
				}
			},
		},
		{
			name: "ParentReviewOnAsksTheParentOnceNamingTheSlugPullRequestBranchesAndSession", scripts: map[string][]scriptedCall{"Create": {created}, "Status": {mergedAtHead}},
			setup:       parentReviewOn,
			wantOutcome: shedengine.Done, wantStarts: 1, wantStops: 1,
			check: func(t *testing.T, fx *prReviewFixture, _ *fakePRSessions) {
				assertAsks(t, fx, 1, true)
				if len(fx.asks) != 1 {
					return
				}
				for _, want := range []string{parentReviewRequestPrefix + " the-slug:", recordedURL, "branch task into main", "lym creel post pr-strand"} {
					if !strings.Contains(fx.asks[0], want) {
						t.Errorf("parent ask %q lacks %q", fx.asks[0], want)
					}
				}
			},
		},
		{
			name: "ParentReviewOnWithTheRecordAlreadyAskedAsksNone", record: recordedAtHead(func(r *pullrequest.Record) { r.ParentReviewAsked = true }), scripts: map[string][]scriptedCall{"Status": {open}},
			setup:         parentReviewOn,
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true, wantStarts: 1,
			check: func(t *testing.T, fx *prReviewFixture, _ *fakePRSessions) { assertAsks(t, fx, 0, true) },
		},
		{
			name: "ParentReviewOffAsksNone", scripts: map[string][]scriptedCall{"Create": {created}, "Status": {mergedAtHead}},
			wantOutcome: shedengine.Done, wantStarts: 1, wantStops: 1,
			check: func(t *testing.T, fx *prReviewFixture, _ *fakePRSessions) { assertAsks(t, fx, 0, false) },
		},
		{
			name: "AnUnreadableParentReviewSettingAsksNone", scripts: map[string][]scriptedCall{"Create": {created}, "Status": {mergedAtHead}},
			setup: func(_ *testing.T, fx *prReviewFixture, _ *fakePRSessions) {
				fx.deps.PullRequest.ParentReviewWanted = func() (bool, error) { return false, errors.New("loom.yaml refused") }
			},
			wantOutcome: shedengine.Done, wantStarts: 1, wantStops: 1,
			check: func(t *testing.T, fx *prReviewFixture, _ *fakePRSessions) { assertAsks(t, fx, 0, false) },
		},
		{
			name: "AFailedParentAskLeavesTheRecordUnaskedForTheNextEntry", scripts: map[string][]scriptedCall{"Create": {created}, "Status": {mergedAtHead}},
			setup: func(t *testing.T, fx *prReviewFixture, s *fakePRSessions) {
				parentReviewOn(t, fx, s)
				fx.askErr = errors.New("creel down")
			},
			wantOutcome: shedengine.Done, wantStarts: 1, wantStops: 1,
			check: func(t *testing.T, fx *prReviewFixture, _ *fakePRSessions) { assertAsks(t, fx, 1, false) },
		},
		{
			name: "AParentAskNoOrchTakesLeavesTheRecordUnasked", scripts: map[string][]scriptedCall{"Create": {created}, "Status": {mergedAtHead}},
			setup: func(t *testing.T, fx *prReviewFixture, s *fakePRSessions) {
				parentReviewOn(t, fx, s)
				fx.askUnrecorded = true
			},
			wantOutcome: shedengine.Done, wantStarts: 1, wantStops: 1,
			check: func(t *testing.T, fx *prReviewFixture, _ *fakePRSessions) { assertAsks(t, fx, 1, false) },
		},
		{
			name: "PromptCarriesTheRegressionCheck", scripts: map[string][]scriptedCall{"Create": {created}, "Status": {mergedAtHead}},
			setup: func(_ *testing.T, fx *prReviewFixture, _ *fakePRSessions) {
				fx.deps.DriftVerdict = func() (string, error) { return testVerdict, nil }
			},
			wantOutcome: shedengine.Done, wantStarts: 1, wantStops: 1,
			check: func(t *testing.T, _ *prReviewFixture, s *fakePRSessions) {
				prompt := s.starts[0].Prompt
				if !strings.Contains(prompt, testVerdict) || strings.Contains(prompt, noDriftVerdict) {
					t.Errorf("prompt lacks the regression check %q, or says none ran:\n%s", testVerdict, prompt)
				}
			},
		},
		{
			name: "AFailingRegressionCheckReadWhileBuildingTheSpecIsAnErrorWithNoSessionStart", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {open}},
			setup: func(_ *testing.T, fx *prReviewFixture, _ *fakePRSessions) {
				// The first read is the pull request body's, which passes, and the second is the session spec's.
				reads := 0
				fx.deps.DriftVerdict = func() (string, error) {
					reads++
					if reads > 1 {
						return "", errors.New("scope record unreadable")
					}
					return "", nil
				}
			},
			wantErr: true,
		},
		{
			name: "RunMaterialListsTheDecisionRecordThatExistsAndLeavesOutAMissingPlanDirectory", scripts: map[string][]scriptedCall{"Create": {created}, "Status": {mergedAtHead}},
			setup: func(t *testing.T, fx *prReviewFixture, _ *fakePRSessions) {
				dir := t.TempDir()
				decision := filepath.Join(dir, "decision-record.md")
				if err := os.WriteFile(decision, []byte("x"), 0o644); err != nil {
					t.Fatalf("write %s: %v", decision, err)
				}
				fx.deps.DecisionRecordPath = decision
				fx.deps.PlanDir = filepath.Join(dir, "plan")
				fx.deps.LanguageSkills = []string{"scribe:prose", "scribe:golang-testing"}
			},
			wantOutcome: shedengine.Done, wantStarts: 1, wantStops: 1,
			check: func(t *testing.T, fx *prReviewFixture, s *fakePRSessions) {
				prompt := s.starts[0].Prompt
				for _, want := range []string{"- " + fx.deps.DecisionRecordPath} {
					if !strings.Contains(prompt, want) {
						t.Errorf("prompt lacks the run material line %q:\n%s", want, prompt)
					}
				}
				if strings.Contains(prompt, fx.deps.PlanDir) {
					t.Errorf("prompt lists the missing plan directory %q:\n%s", fx.deps.PlanDir, prompt)
				}
				if want := []string{"scribe:prose", "scribe:golang-testing"}; !slices.Equal(s.starts[0].Skills, want) {
					t.Errorf("spec skills = %q; want the language skills alone, %q", s.starts[0].Skills, want)
				}
			},
		},
		{
			name: "ReentryWithALiveRecordedStrandStartsNone", record: recordedAtHead(withStrand("g-live", "sess-old")), scripts: map[string][]scriptedCall{"Status": {open}},
			setup:         func(_ *testing.T, _ *prReviewFixture, s *fakePRSessions) { s.live["g-live"] = true },
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true,
		},
		{
			name: "ReentryWithAKeptReviewEndedRecordStartsNone", record: recordedAtHead(withStrand("g-old", "sess-old"), func(r *pullrequest.Record) { r.ReviewEnded = true }),
			scripts:     map[string][]scriptedCall{"Status": {closed}},
			wantOutcome: shedengine.Done,
		},
		{
			name: "ReentryWithARecordedDeadStrandResumesItBySessionID", record: recordedAtHead(withStrand("g-dead", "sess-old")), scripts: map[string][]scriptedCall{"Status": {open}},
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true, wantStarts: 1,
			check: func(t *testing.T, _ *prReviewFixture, s *fakePRSessions) {
				if s.starts[0].ResumeSessionID != "sess-old" {
					t.Errorf("start ResumeSessionID = %q; want the recorded session id", s.starts[0].ResumeSessionID)
				}
			},
		},
		{
			name: "AStartFailingWithNotStartedIsAnError", scripts: map[string][]scriptedCall{"Create": {created}},
			setup: func(_ *testing.T, _ *prReviewFixture, s *fakePRSessions) {
				s.startErr = fmt.Errorf("start: %w", shuttleengine.ErrNotStarted)
			},
			wantErr: true, wantStarts: 1,
		},
		{
			name: "AnyOtherStartFailureHaltsNamingTheErrorAndLoomResume", scripts: map[string][]scriptedCall{"Create": {created}},
			setup:       func(_ *testing.T, _ *prReviewFixture, s *fakePRSessions) { s.startErr = errors.New("pane died") },
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"pane died", "lym loom resume"}, wantStarts: 1,
		},
		{
			name: "ASessionLiveErrorIsAnError", record: recordedAtHead(withStrand("g", "sess-old")), scripts: map[string][]scriptedCall{"Status": {open}},
			setup:   func(_ *testing.T, _ *prReviewFixture, s *fakePRSessions) { s.liveErr = errors.New("reed down") },
			wantErr: true,
		},
		{
			name: "AFailingRegistrationIsLoggedAndTheRecordStillWritten", scripts: map[string][]scriptedCall{"Create": {created}, "Status": {mergedAtHead}},
			setup:       func(_ *testing.T, _ *prReviewFixture, s *fakePRSessions) { s.registerErr = errors.New("no registry") },
			wantOutcome: shedengine.Done, wantStarts: 1, wantStops: 1,
			check: func(t *testing.T, fx *prReviewFixture, _ *fakePRSessions) {
				got, _, err := pullrequest.ReadRecord(fx.deps.PullRequest.RecordPath, fx.deps.PullRequest.RecordLockPath)
				if err != nil || got.StrandGUID != "guid-1" {
					t.Errorf("ReadRecord() = %+v, %v; want the record naming the started strand", got, err)
				}
			},
		},
		{
			name: "AFailedRecordWriteAfterAStartStopsTheNewSession", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {open}},
			setup: func(_ *testing.T, fx *prReviewFixture, _ *fakePRSessions) {
				pr := &fx.deps.PullRequest
				pr.RegisterSession = func(string) error { return pullrequest.RemoveRecord(pr.RecordPath, pr.RecordLockPath) }
			},
			wantErr: true, wantStarts: 1, wantStops: 1,
		},
		{
			name: "ACancelledContextLeavesTheSessionRunning", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {open}},
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true, wantStarts: 1,
		},
		{
			name: "AStuckHaltStopsTheSession", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {closed}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"closed on GitHub without being merged"}, wantStarts: 1, wantStops: 1,
		},
		{
			name: "AnErrorStopsTheSession", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {open}},
			setup: func(_ *testing.T, fx *prReviewFixture, _ *fakePRSessions) {
				fx.deps.PullRequest.DirtyPaths = func() ([]string, error) { return nil, errors.New("git status failed") }
			},
			wantErr: true, wantStarts: 1, wantStops: 1,
		},
		{
			name: "ACoveredWakeStopsTheSessionAndTheNextUncoveredWakeResumesIt", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {open}},
			// Read 1 is the entry's and read 2 the first wake's.
			coveredAt:     func(n int) bool { return n == 2 },
			cancelAtSleep: 2, wantCanceled: true, wantStarts: 2, wantStops: 1,
			check: func(t *testing.T, _ *prReviewFixture, s *fakePRSessions) {
				if s.starts[1].ResumeSessionID != "sess-1" {
					t.Errorf("resume ResumeSessionID = %q; want the session the first start recorded", s.starts[1].ResumeSessionID)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newPRReviewFixture(t, "on", nil, "", githubOrigin, tt.record, tt.scripts, nil)
			sessions := sessionsOf(fx.deps)
			if tt.setup != nil {
				tt.setup(t, fx, sessions)
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			now := prReviewFixtureNow
			sleeps, pauseReads := 0, 0
			pr := &fx.deps.PullRequest
			pr.Now = func() time.Time { return now }
			pr.Sleep = func(ctx context.Context, d time.Duration) error {
				sleeps++
				now = now.Add(d)
				if tt.cancelAtSleep > 0 && sleeps >= tt.cancelAtSleep {
					cancel()
				}
				return ctx.Err()
			}
			pr.PauseCovered = func() (bool, error) {
				pauseReads++
				return tt.coveredAt != nil && tt.coveredAt(pauseReads), nil
			}

			outcome, pointer, err := NewPRReview("PR-Review", fx.deps).Call(ctx)

			switch {
			case tt.wantCanceled:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("Call() error = %v; want one wrapping context.Canceled", err)
				}
			case tt.wantErr:
				if err == nil {
					t.Fatalf("Call() = %q, %+v; want an error", outcome, pointer)
				}
			default:
				if err != nil {
					t.Fatalf("Call() error = %v; want none", err)
				}
				if outcome != tt.wantOutcome || pointer.Halt != tt.wantHalt {
					t.Errorf("Call() = %q, halt %v; want %q, halt %v (reason %q)", outcome, pointer.Halt, tt.wantOutcome, tt.wantHalt, pointer.Reason)
				}
				for _, want := range tt.wantReason {
					if !strings.Contains(pointer.Reason, want) {
						t.Errorf("Call() reason %q lacks %q", pointer.Reason, want)
					}
				}
			}
			assertSessionCounts(t, fx.deps, tt.wantStarts, tt.wantStops)
			if tt.check != nil {
				tt.check(t, fx, sessions)
			}
		})
	}
}

func TestPRReview_NoStopsTheRecordedSessionBeforeDroppingTheRecord(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	record := &pullrequest.Record{Number: 7, StrandGUID: "g-live", SessionID: "sess-old"}
	merged := scriptedCall{status: pullrequest.Status{State: pullrequest.StateMerged}}
	fx := newPRReviewFixture(t, "off", nil, "", Origin{URL: "https://github.com/acme/widgets.git", Upstream: true}, record, map[string][]scriptedCall{"Status": {merged}}, nil)
	sessionsOf(fx.deps).live["g-live"] = true

	outcome, pointer, err := NewPRReview("PR-Review", fx.deps).Call(context.Background())

	if err != nil || outcome != shedengine.Done || pointer.Halt {
		t.Fatalf("Call() = %q, halt %v, error %v; want Done without halt", outcome, pointer.Halt, err)
	}
	assertSessionCounts(t, fx.deps, 0, 1)
	if diff := cmp.Diff([]string{"g-live"}, sessionsOf(fx.deps).stops); diff != "" {
		t.Errorf("stopped strands mismatch (-want +got):\n%s", diff)
	}
}
