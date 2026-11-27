// prreview_session_test.go covers the PR-Review row's interactive session over a real session runner.
// It pins what the session starts under, and when the row reuses, resumes and stops it.
// It also holds the session world the other PR-Review tests share.

package landingshed

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// prSessionWorld is what a PR-Review test's sessions run over:
// a real session runner above the shared engine and strand-table fakes.
// A start adds a live strand named after the role, on session sess-1, and a stop removes it.
type prSessionWorld struct {
	engine *shuttlefake.Engine
	reed   *shuttlefake.Reed
	runner *shuttleengine.Runner
	// registered records each strand the row registered, and registerErr is what a registration fails with.
	registered  []string
	registerErr error
}

func newPRSessionWorld(t *testing.T) *prSessionWorld {
	t.Helper()
	engine := &shuttlefake.Engine{PrepareLaunch: &shuttleengine.Launch{Cmd: "fake-launch-cmd", SessionID: "sess-1"}}
	reed := &shuttlefake.Reed{}
	anchor := t.TempDir()
	engines := shuttleengine.NewEngineSet("claude", map[string]shuttleengine.Engine{"claude": &shuttlefake.ResumeEngine{Engine: engine}})
	cfg := shuttleengine.Config{RunDir: t.TempDir(), RunTimeoutMin: 60, StartupTimeoutS: 30, Agents: shuttlefake.Agents(t)}
	return &prSessionWorld{engine: engine, reed: reed, runner: shuttleengine.NewRunner(reed, engines, anchor, anchor, cfg)}
}

// fill points the session seams of deps at the world.
func (w *prSessionWorld) fill(deps *Deps) {
	deps.PullRequest.Sessions = w.runner
	deps.PullRequest.SessionLive = w.isLive
	deps.PullRequest.RegisterSession = func(guid string) error {
		w.registered = append(w.registered, guid)
		return w.registerErr
	}
}

func (w *prSessionWorld) isLive(guid string) (bool, error) {
	status, err := w.reed.Status()
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(status.Strands, func(s reedengine.StrandStatus) bool { return s.GUID == guid && s.Live }), nil
}

// seedLive puts a live strand guid in the table, as a session an earlier call started left it.
func (w *prSessionWorld) seedLive(guid string) {
	w.reed.Strands = append(w.reed.Strands, reedengine.StrandStatus{GUID: guid, Name: "pr-strand", PaneID: "%90", Live: true})
}

// onFirstStop runs fn at the first stop, before the strand is removed.
func (w *prSessionWorld) onFirstStop(fn func()) {
	w.reed.RemoveStrandFn = func(guid string, recursive bool) (reedengine.Removed, error) {
		fn()
		w.reed.RemoveStrandFn = nil
		return w.reed.RemoveStrand(guid, recursive)
	}
}

func (w *prSessionWorld) starts() int { return len(w.reed.AddedSpecs) }

func (w *prSessionWorld) stops() int { return len(w.reed.RemovedGUIDs) }

// recordComparison returns the options a record comparison against want runs under.
// A wanted record that names no session leaves the identity of the session the row started out of the comparison.
func recordComparison(want pullrequest.Record) []cmp.Option {
	if want.StrandGUID != "" {
		return nil
	}
	return []cmp.Option{cmpopts.IgnoreFields(pullrequest.Record{}, "StrandGUID", "StrandName", "SessionID", "Engine")}
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
		return func(r *pullrequest.Record) { r.StrandGUID, r.StrandName, r.SessionID = guid, "pr-strand", sessionID }
	}
	const cancelAtFirstSleep = 1
	approved := scriptedCall{status: pullrequest.Status{State: pullrequest.StateOpen, HeadSHA: "head-new", Approved: true}}
	withContinue := func(r *pullrequest.Record) { r.Continue = true }
	withReviewEnded := func(r *pullrequest.Record) { r.ReviewEnded = true }
	seedLiveStrand := func(_ *testing.T, _ *prReviewFixture, s *prSessionWorld) { s.seedLive("g-live") }

	tests := []struct {
		name    string
		record  *pullrequest.Record
		scripts map[string][]scriptedCall
		setup   func(t *testing.T, fx *prReviewFixture, s *prSessionWorld)
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
		check        func(t *testing.T, fx *prReviewFixture, s *prSessionWorld)
	}{
		{
			name: "AnOpenedPullRequestStartsOneInteractiveSessionAndAMergedVerdictStopsIt", scripts: map[string][]scriptedCall{"Create": {created}, "Status": {mergedAtHead}},
			setup: func(_ *testing.T, fx *prReviewFixture, _ *prSessionWorld) {
				fx.deps.LanguageSkills = []string{"scribe:prose", "scribe:golang-testing"}
			},
			wantOutcome: shedengine.Done, wantStarts: 1, wantStops: 1,
			check: func(t *testing.T, fx *prReviewFixture, s *prSessionWorld) {
				spec := s.engine.LastSpec
				if !spec.Interactive || !spec.Unawaited || !spec.DenyPublish {
					t.Errorf("spec Interactive = %v, Unawaited = %v, DenyPublish = %v; want all true", spec.Interactive, spec.Unawaited, spec.DenyPublish)
				}
				if spec.Role != "pr-reviewer" || spec.ConfigRole != agentconfig.RolePRReviewer {
					t.Errorf("spec Role = %q, ConfigRole = %q; want %q and %q", spec.Role, spec.ConfigRole, "pr-reviewer", agentconfig.RolePRReviewer)
				}
				// The runner adds the role's own skills beside the language skills the row passes.
				for _, skill := range fx.deps.LanguageSkills {
					if !slices.Contains(spec.Skills, skill) {
						t.Errorf("spec skills %v lack the language skill %q", spec.Skills, skill)
					}
				}
				wantChoice := shuttleengine.ModelChoice{Engine: "claude", Model: "fixture-model"}
				if diff := cmp.Diff(wantChoice, spec.ModelChoice); diff != "" {
					t.Errorf("spec model choice mismatch (-want +got):\n%s", diff)
				}
				if !strings.Contains(spec.Prompt, recordedURL) {
					t.Errorf("prompt lacks the pull request URL %q:\n%s", recordedURL, spec.Prompt)
				}
				if diff := cmp.Diff([]string{"fake-guid-1"}, s.registered); diff != "" {
					t.Errorf("registered strands mismatch (-want +got):\n%s", diff)
				}
				if live, err := s.isLive("fake-guid-1"); live || err != nil {
					t.Errorf("the session is live = %v (%v) after the call ended with a verdict; want it stopped", live, err)
				}
			},
		},
		{
			name: "TheStartedSessionsIdentityIsRecorded", scripts: map[string][]scriptedCall{"Create": {created}, "Status": {open}},
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true, wantStarts: 1,
			check: func(t *testing.T, fx *prReviewFixture, _ *prSessionWorld) {
				got, found, err := pullrequest.ReadRecord(fx.deps.PullRequest.RecordPath, fx.deps.PullRequest.RecordLockPath)
				if err != nil || !found {
					t.Fatalf("ReadRecord() = %+v, %v, %v; want the record", got, found, err)
				}
				want := pullrequest.Record{
					Number: 7, URL: recordedURL, Base: "main", Head: "task", GatedHead: "head-new",
					StrandGUID: "fake-guid-1", StrandName: "pr-reviewer", SessionID: "sess-1", Engine: "claude",
				}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("record mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "ARecordedLiveStrandIsReusedWithNoStart", record: recordedAtHead(withStrand("g-live", "sess-old")), scripts: map[string][]scriptedCall{"Status": {open}},
			setup:         seedLiveStrand,
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true,
		},
		{
			name: "ARecordedDeadStrandIsResumedBySessionID", record: recordedAtHead(withStrand("g-dead", "sess-old")), scripts: map[string][]scriptedCall{"Status": {open}},
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true, wantStarts: 1,
			check: func(t *testing.T, _ *prReviewFixture, s *prSessionWorld) {
				if got := s.engine.LastSpec.ResumeSessionID; got != "sess-old" {
					t.Errorf("start ResumeSessionID = %q; want the recorded session id", got)
				}
			},
		},
		{
			name: "AStuckVerdictStopsTheSession", record: recordedAtHead(withStrand("g-live", "sess-old")),
			scripts: map[string][]scriptedCall{"Status": {{status: pullrequest.Status{State: pullrequest.StateOpen, HeadSHA: "x9"}}}},
			setup: func(t *testing.T, fx *prReviewFixture, s *prSessionWorld) {
				seedLiveStrand(t, fx, s)
				fx.deps.PullRequest.CommitsMissing = func(string, string) ([]string, error) { return []string{"abc1234 hand fix"}, nil }
			},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"abc1234 hand fix"}, wantStops: 1,
		},
		{
			name: "AClosedPullRequestWithNoContinueFlagHaltsWithoutStartingASession", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {closed}},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"closed on GitHub without being merged"},
		},
		{
			name: "AMergedReEntryStartsNoSession", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {mergedAtHead}},
			wantOutcome: shedengine.Done,
		},
		{
			name: "AnEndedReEntryStartsNoSession", record: recordedAtHead(withReviewEnded), scripts: map[string][]scriptedCall{"Status": {closed}},
			wantOutcome: shedengine.Done,
		},
		{
			name: "ATransientEntryReadFailureLeavesTheSessionRunning", record: recordedAtHead(withStrand("g-live", "sess-old")),
			scripts: map[string][]scriptedCall{"Status": {{err: context.DeadlineExceeded}}},
			setup:   seedLiveStrand, wantErr: true,
		},
		{
			name: "AFailedWriteOfTheStartedSessionsRecordStopsIt", scripts: map[string][]scriptedCall{"Create": {created}},
			setup: func(_ *testing.T, fx *prReviewFixture, _ *prSessionWorld) {
				// The first commit is the pull request's, and the second the session's.
				// A transient failure leaves the end of the row stopping nothing, so the stop seen is the failed write's.
				commits := 0
				fx.deps.PullRequest.CommitRecord = func() error {
					commits++
					if commits == 2 {
						return context.DeadlineExceeded
					}
					return nil
				}
			},
			wantErr: true, wantStarts: 1, wantStops: 1,
		},
		{
			name: "AFailedRegistrationKeepsWaiting", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {open}},
			setup: func(_ *testing.T, _ *prReviewFixture, s *prSessionWorld) {
				s.registerErr = errors.New("no pane process")
			},
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true, wantStarts: 1,
		},
		{
			name: "AContinuedReviewStopsTheSessionBeforeItRecordsTheEnding", record: recordedAtHead(withStrand("g-live", "sess-old"), withContinue),
			scripts: map[string][]scriptedCall{"Status": {open}},
			setup: func(t *testing.T, fx *prReviewFixture, s *prSessionWorld) {
				s.seedLive("g-live")
				pr := fx.deps.PullRequest
				s.onFirstStop(func() {
					rec, _, err := pullrequest.ReadRecord(pr.RecordPath, pr.RecordLockPath)
					if err != nil || rec.ReviewEnded {
						t.Errorf("record at the stop = %+v, %v; want the ending not yet recorded", rec, err)
					}
				})
			},
			wantOutcome: shedengine.Done, wantStops: 1,
		},
		{
			name: "AnApprovalOnADirtyTreeLeavesTheSessionAtWork", record: recordedAtHead(withStrand("g-live", "sess-old")),
			scripts: map[string][]scriptedCall{"Status": {approved}},
			setup: func(_ *testing.T, fx *prReviewFixture, s *prSessionWorld) {
				s.seedLive("g-live")
				fx.dirty = []string{"wip.go"}
			},
			cancelAtSleep: 2, wantCanceled: true,
		},
		{
			name: "ATreeDirtiedAtTheStopIsJudgedOnTheSecondReadAndTheSessionResumed", record: recordedAtHead(withStrand("g-live", "sess-old")),
			scripts: map[string][]scriptedCall{"Status": {approved}},
			setup: func(_ *testing.T, fx *prReviewFixture, s *prSessionWorld) {
				s.seedLive("g-live")
				s.onFirstStop(func() { fx.dirty = []string{"wip.go"} })
			},
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true, wantStarts: 1, wantStops: 1,
		},
		{
			name: "ACancelledContextLeavesTheSessionRunning", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {open}},
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true, wantStarts: 1,
			check: func(t *testing.T, _ *prReviewFixture, s *prSessionWorld) {
				if live, err := s.isLive("fake-guid-1"); !live || err != nil {
					t.Errorf("the session is live = %v (%v) after the context was cancelled; want it left running", live, err)
				}
			},
		},
		{
			name: "ASessionNoLongerLiveWhileTheReviewIsOpenHaltsNamingLoomResume", record: recordedAtHead(withStrand("g-live", "sess-old")), scripts: map[string][]scriptedCall{"Status": {open}},
			setup: func(_ *testing.T, fx *prReviewFixture, _ *prSessionWorld) {
				// The first read is the entry's, which finds the strand live; every later read finds it gone.
				reads := 0
				fx.deps.PullRequest.SessionLive = func(string) (bool, error) {
					reads++
					return reads == 1, nil
				}
			},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"ended while the review was open", "lym loom resume"},
		},
		{
			name: "ANonTransientStartFailureHaltsNamingItAndLoomResume", scripts: map[string][]scriptedCall{"Create": {created}},
			setup:       func(_ *testing.T, _ *prReviewFixture, s *prSessionWorld) { s.reed.AddErr = errors.New("pane died") },
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"the PR-Review session could not be started", "pane died", "lym loom resume"}, wantStarts: 1,
		},
		{
			name: "ATransientStartFailureIsAnError", scripts: map[string][]scriptedCall{"Create": {created}},
			setup: func(_ *testing.T, _ *prReviewFixture, s *prSessionWorld) {
				s.reed.AddErr = fmt.Errorf("start: %w", shuttleengine.ErrNotStarted)
			},
			wantErr: true, wantStarts: 1,
		},
		{
			name: "APauseStopsTheLiveSessionAndTheEndOfThePauseResumesIt", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {open}},
			// Read 1 is the entry's and read 2 the first wake's.
			coveredAt:     func(n int) bool { return n == 2 },
			cancelAtSleep: 2, wantCanceled: true, wantStarts: 2, wantStops: 1,
			check: func(t *testing.T, _ *prReviewFixture, s *prSessionWorld) {
				if got := s.engine.LastSpec.ResumeSessionID; got != "sess-1" {
					t.Errorf("resume ResumeSessionID = %q; want the session the first start recorded", got)
				}
			},
		},
		{
			name: "AFailedResumeAfterAPauseHaltsNamingItAndLoomResume", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {open}},
			coveredAt: func(n int) bool { return n == 2 },
			setup: func(_ *testing.T, _ *prReviewFixture, s *prSessionWorld) {
				s.onFirstStop(func() { s.reed.AddErr = errors.New("pane died") })
			},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"the PR-Review session could not be started", "pane died", "lym loom resume"},
			wantStarts: 2, wantStops: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newPRReviewFixture(t, "review", nil, githubOrigin, tt.record, tt.scripts, nil)
			sessions := newPRSessionWorld(t)
			sessions.fill(&fx.deps)
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
			if sessions.starts() != tt.wantStarts || sessions.stops() != tt.wantStops {
				t.Errorf("session starts = %d, stops = %d; want %d and %d", sessions.starts(), sessions.stops(), tt.wantStarts, tt.wantStops)
			}
			if tt.check != nil {
				tt.check(t, fx, sessions)
			}
		})
	}

	t.Run("NoPullRequestStopsTheRecordedSessionBeforeDroppingTheRecord", func(t *testing.T) {
		t.Parallel()
		record := &pullrequest.Record{Number: 7, StrandGUID: "g-live", SessionID: "sess-old"}
		merged := scriptedCall{status: pullrequest.Status{State: pullrequest.StateMerged}}
		fx := newPRReviewFixture(t, "off", nil, githubOrigin, record, map[string][]scriptedCall{"Status": {merged}}, nil)
		sessions := newPRSessionWorld(t)
		sessions.fill(&fx.deps)
		sessions.seedLive("g-live")
		recordPresentAtStop := false
		sessions.onFirstStop(func() {
			_, found, err := pullrequest.ReadRecord(fx.deps.PullRequest.RecordPath, fx.deps.PullRequest.RecordLockPath)
			recordPresentAtStop = found && err == nil
		})

		outcome, pointer, err := NewPRReview("PR-Review", fx.deps).Call(context.Background())

		if err != nil || outcome != shedengine.Done || pointer.Halt {
			t.Fatalf("Call() = %q, halt %v, error %v; want Done without halt", outcome, pointer.Halt, err)
		}
		if diff := cmp.Diff([]string{"g-live"}, sessions.reed.RemovedGUIDs); diff != "" {
			t.Errorf("stopped strands mismatch (-want +got):\n%s", diff)
		}
		if !recordPresentAtStop {
			t.Errorf("the record was already gone when the session was stopped; want the stop first")
		}
		if _, found, rerr := pullrequest.ReadRecord(fx.deps.PullRequest.RecordPath, fx.deps.PullRequest.RecordLockPath); rerr != nil || found {
			t.Errorf("ReadRecord() found = %v, error = %v; want the record dropped", found, rerr)
		}
		if sessions.starts() != 0 {
			t.Errorf("session starts = %d; want none", sessions.starts())
		}
	})
}
