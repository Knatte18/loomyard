// prreview_session_test.go covers the PR-Review row's interactive session over a real session runner and reed.
// It pins what the session starts under, and when the row reuses, resumes and stops it.
// It also holds the session world the other PR-Review tests share.

package landingshed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/permkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxrec"
)

// Recordings of the tmux the session worlds replay.
const (
	// recordingStartStop starts a session, stops it, starts it again and stops it again.
	recordingStartStop = "session-start-stop"
	// recordingEnds starts a session whose pane's process then ends.
	recordingEnds = "session-ends"
	// recordingNotStarted starts a session whose provider never comes up.
	recordingNotStarted = "session-not-started"
)

// firstGUID is the guid of the first strand a session world's reed starts, and secondGUID of the second.
const (
	firstGUID  = tmuxrec.GUIDPrefix + "1"
	secondGUID = tmuxrec.GUIDPrefix + "2"
)

// startedPanePID is the pid a replay answers for the pane of the first strand a session world's reed starts:
// the {pid:4} pane of the session-start-stop recording, above the replay's pid base of 1<<30.
const startedPanePID = 1<<30 + 4

// prSessionWorld is what a PR-Review test's sessions run over:
// the real session runner and reed over a recorded tmux, with the provider engine the one stand-in above reed.
// The row's starts and stops pass through a counting wrapper to the runner.
// Each pane registration passes through a recording wrapper to runproc.
type prSessionWorld struct {
	t       testing.TB
	session *tmuxrec.Session
	counted *countedSessions
	// registerDir is the directory the registrations write into, and registered holds each pane pid registered.
	registerDir string
	registered  []int
}

// countedSessions passes each start and stop to the real runner and counts them.
// beforeStop runs once at the first stop before the strand is stopped, and afterStop once after it.
type countedSessions struct {
	*shuttleengine.Runner
	starts     int
	stopped    []string
	beforeStop func()
	afterStop  func()
}

func (c *countedSessions) StartSession(spec shuttleengine.Spec) (shuttleengine.OpenResult, error) {
	c.starts++
	return c.Runner.StartSession(spec)
}

func (c *countedSessions) StopStrand(guid string) error {
	if fn := c.beforeStop; fn != nil {
		c.beforeStop = nil
		fn()
	}
	c.stopped = append(c.stopped, guid)
	err := c.Runner.StopStrand(guid)
	if fn := c.afterStop; fn != nil {
		c.afterStop = nil
		fn()
	}
	return err
}

// newPRSessionWorld builds a session world whose reed replays the recording named recording.
func newPRSessionWorld(t testing.TB, recording string) *prSessionWorld {
	t.Helper()
	session, replay := tmuxrec.PlaySession(t, recording)
	t.Cleanup(func() {
		if unanswered := replay.Unanswered(); len(unanswered) > 0 {
			t.Errorf("recording %s holds no answer to %q", recording, unanswered)
		}
	})
	return &prSessionWorld{
		t:           t,
		session:     session,
		counted:     &countedSessions{Runner: session.Runner},
		registerDir: filepath.Join(t.TempDir(), "runproc"),
	}
}

// fill points the session seams of deps at the world.
func (w *prSessionWorld) fill(deps *Deps) {
	status := w.session.Reed.Status
	deps.PullRequest.Sessions = w.counted
	deps.PullRequest.SessionLive = reedengine.SessionLiveness(status)
	deps.PullRequest.RegisterSession = reedengine.SessionRegistrar(status, func(pid int) error {
		w.registered = append(w.registered, pid)
		return runproc.Register(w.registerDir, "the-slug", runproc.ChildRecord(pid, runproc.RoleSession, nil))
	})
}

func (w *prSessionWorld) isLive(guid string) (bool, error) {
	return reedengine.SessionLiveness(w.session.Reed.Status)(guid)
}

// seedLive starts a session as an earlier call of the row deps builds would have, and returns its strand's guid.
func (w *prSessionWorld) seedLive(deps Deps) string {
	w.t.Helper()
	// The prompt needs a pull request URL, whichever one the row under test records.
	spec, err := NewPRReview("PR-Review", deps).sessionSpec(pullrequest.Record{URL: "https://github.com/acme/widgets/pull/7"})
	if err != nil {
		w.t.Fatalf("sessionSpec() error = %v", err)
	}
	started, err := w.session.Runner.StartSession(spec)
	if err != nil {
		w.t.Fatalf("seed the session: StartSession() error = %v", err)
	}
	return started.StrandGUID
}

// onFirstStop runs fn at the first stop, before the strand is stopped.
func (w *prSessionWorld) onFirstStop(fn func()) { w.counted.beforeStop = fn }

// afterFirstStop runs fn once the first stop has stopped the strand.
func (w *prSessionWorld) afterFirstStop(fn func()) { w.counted.afterStop = fn }

// blockStarts makes every later start fail, by leaving the runner unable to make a run's directory.
func (w *prSessionWorld) blockStarts() {
	w.t.Helper()
	if err := os.MkdirAll(w.session.RunDir, 0o755); err != nil {
		w.t.Fatalf("create the run directory: %v", err)
	}
	permkit.Chmod(w.t, w.session.RunDir, 0o500)
}

// blockRegistration makes every later registration fail, by putting a file where its directory goes.
func (w *prSessionWorld) blockRegistration() {
	w.t.Helper()
	if err := os.WriteFile(w.registerDir, nil, 0o644); err != nil {
		w.t.Fatalf("block the registration directory: %v", err)
	}
}

// blockStatus makes every later read of reed's strand table fail, by making reed's state directory unreadable.
func (w *prSessionWorld) blockStatus() {
	w.t.Helper()
	permkit.Chmod(w.t, w.session.StateDir, 0)
}

func (w *prSessionWorld) starts() int { return w.counted.starts }

func (w *prSessionWorld) stops() int { return len(w.counted.stopped) }

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
	seedLiveStrand := func(_ *testing.T, fx *prReviewFixture, s *prSessionWorld) { s.seedLive(fx.deps) }
	const startFailure = "permission denied"

	tests := []struct {
		name string
		// recording is the recorded tmux the row's reed replays; empty replays recordingStartStop.
		recording string
		record    *pullrequest.Record
		scripts   map[string][]scriptedCall
		setup     func(t *testing.T, fx *prReviewFixture, s *prSessionWorld)
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
				spec := s.session.Engine.LastSpec
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
				if diff := cmp.Diff([]int{startedPanePID}, s.registered); diff != "" {
					t.Errorf("registered pane pids mismatch (-want +got):\n%s", diff)
				}
				if live, err := s.isLive(firstGUID); live || err != nil {
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
					StrandGUID: firstGUID, StrandName: tmuxrec.Shortname + ":pr-reviewer", SessionID: tmuxrec.SessionID, Engine: "claude",
				}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("record mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name: "ARecordedLiveStrandIsReusedWithNoStart", record: recordedAtHead(withStrand(firstGUID, "sess-old")), scripts: map[string][]scriptedCall{"Status": {open}},
			setup:         seedLiveStrand,
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true,
		},
		{
			name: "ARecordedDeadStrandIsResumedBySessionID", record: recordedAtHead(withStrand("g-dead", "sess-old")), scripts: map[string][]scriptedCall{"Status": {open}},
			// An earlier session came and went, so reed has a session whose strand table lacks the recorded strand.
			setup: func(t *testing.T, fx *prReviewFixture, s *prSessionWorld) {
				if err := s.session.Runner.StopStrand(s.seedLive(fx.deps)); err != nil {
					t.Fatalf("stop the seeded session: %v", err)
				}
			},
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true, wantStarts: 1,
			check: func(t *testing.T, _ *prReviewFixture, s *prSessionWorld) {
				if got := s.session.Engine.LastSpec.ResumeSessionID; got != "sess-old" {
					t.Errorf("start ResumeSessionID = %q; want the recorded session id", got)
				}
			},
		},
		{
			name: "AStuckVerdictStopsTheSession", record: recordedAtHead(withStrand(firstGUID, "sess-old")),
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
			name: "ATransientEntryReadFailureLeavesTheSessionRunning", record: recordedAtHead(withStrand(firstGUID, "sess-old")),
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
			setup:         func(_ *testing.T, _ *prReviewFixture, s *prSessionWorld) { s.blockRegistration() },
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true, wantStarts: 1,
			check: func(t *testing.T, _ *prReviewFixture, s *prSessionWorld) {
				if len(s.registered) != 1 {
					t.Errorf("registration attempts = %v; want the started session's one", s.registered)
				}
			},
		},
		{
			name: "AContinuedReviewStopsTheSessionBeforeItRecordsTheEnding", record: recordedAtHead(withStrand(firstGUID, "sess-old"), withContinue),
			scripts: map[string][]scriptedCall{"Status": {open}},
			setup: func(t *testing.T, fx *prReviewFixture, s *prSessionWorld) {
				seedLiveStrand(t, fx, s)
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
			name: "AnApprovalOnADirtyTreeLeavesTheSessionAtWork", record: recordedAtHead(withStrand(firstGUID, "sess-old")),
			scripts: map[string][]scriptedCall{"Status": {approved}},
			setup: func(t *testing.T, fx *prReviewFixture, s *prSessionWorld) {
				seedLiveStrand(t, fx, s)
				fx.dirty = []string{"wip.go"}
			},
			cancelAtSleep: 2, wantCanceled: true,
		},
		{
			name: "ATreeDirtiedAtTheStopIsJudgedOnTheSecondReadAndTheSessionResumed", record: recordedAtHead(withStrand(firstGUID, "sess-old")),
			scripts: map[string][]scriptedCall{"Status": {approved}},
			setup: func(t *testing.T, fx *prReviewFixture, s *prSessionWorld) {
				seedLiveStrand(t, fx, s)
				s.onFirstStop(func() { fx.dirty = []string{"wip.go"} })
			},
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true, wantStarts: 1, wantStops: 1,
			check: func(t *testing.T, _ *prReviewFixture, s *prSessionWorld) {
				if live, err := s.isLive(secondGUID); !live || err != nil {
					t.Errorf("the resumed session is live = %v (%v); want it running", live, err)
				}
			},
		},
		{
			name: "ACancelledContextLeavesTheSessionRunning", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {open}},
			cancelAtSleep: cancelAtFirstSleep, wantCanceled: true, wantStarts: 1,
			check: func(t *testing.T, _ *prReviewFixture, s *prSessionWorld) {
				if live, err := s.isLive(firstGUID); !live || err != nil {
					t.Errorf("the session is live = %v (%v) after the context was cancelled; want it left running", live, err)
				}
			},
		},
		{
			// The entry reads the strand live, and a later read finds its pane's process ended.
			name: "ASessionNoLongerLiveWhileTheReviewIsOpenHaltsNamingLoomResume", recording: recordingEnds,
			record: recordedAtHead(withStrand(firstGUID, "sess-old")), scripts: map[string][]scriptedCall{"Status": {open}},
			setup:       seedLiveStrand,
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"ended while the review was open", "lym loom resume"},
		},
		{
			name: "ANonTransientStartFailureHaltsNamingItAndLoomResume", scripts: map[string][]scriptedCall{"Create": {created}},
			setup:       func(_ *testing.T, _ *prReviewFixture, s *prSessionWorld) { s.blockStarts() },
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"the PR-Review session could not be started", startFailure, "lym loom resume"}, wantStarts: 1,
		},
		{
			name: "ATransientStartFailureIsAnError", recording: recordingNotStarted, scripts: map[string][]scriptedCall{"Create": {created}},
			setup: func(_ *testing.T, _ *prReviewFixture, s *prSessionWorld) {
				s.session.Engine.StartupFn = func(string) shuttleengine.StartupState { return shuttleengine.StartupPending }
			},
			wantErr: true, wantStarts: 1,
		},
		{
			name: "APauseStopsTheLiveSessionAndTheEndOfThePauseResumesIt", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {open}},
			// Read 1 is the entry's and read 2 the first wake's.
			coveredAt:     func(n int) bool { return n == 2 },
			cancelAtSleep: 2, wantCanceled: true, wantStarts: 2, wantStops: 1,
			check: func(t *testing.T, _ *prReviewFixture, s *prSessionWorld) {
				if got := s.session.Engine.LastSpec.ResumeSessionID; got != tmuxrec.SessionID {
					t.Errorf("resume ResumeSessionID = %q; want the session the first start recorded", got)
				}
			},
		},
		{
			name: "AFailedResumeAfterAPauseHaltsNamingItAndLoomResume", record: recordedAtHead(), scripts: map[string][]scriptedCall{"Status": {open}},
			coveredAt: func(n int) bool { return n == 2 },
			setup: func(_ *testing.T, _ *prReviewFixture, s *prSessionWorld) {
				s.afterFirstStop(s.blockStarts)
			},
			wantOutcome: shedengine.Stuck, wantHalt: true, wantReason: []string{"the PR-Review session could not be started", startFailure, "lym loom resume"},
			wantStarts: 2, wantStops: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newPRReviewFixture(t, "review", nil, githubOrigin, tt.record, tt.scripts, nil)
			recording := tt.recording
			if recording == "" {
				recording = recordingStartStop
			}
			sessions := newPRSessionWorld(t, recording)
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
		record := &pullrequest.Record{Number: 7, StrandGUID: firstGUID, SessionID: "sess-old"}
		merged := scriptedCall{status: pullrequest.Status{State: pullrequest.StateMerged}}
		fx := newPRReviewFixture(t, "off", nil, githubOrigin, record, map[string][]scriptedCall{"Status": {merged}}, nil)
		sessions := newPRSessionWorld(t, recordingStartStop)
		sessions.fill(&fx.deps)
		sessions.seedLive(fx.deps)
		recordPresentAtStop := false
		sessions.onFirstStop(func() {
			_, found, err := pullrequest.ReadRecord(fx.deps.PullRequest.RecordPath, fx.deps.PullRequest.RecordLockPath)
			recordPresentAtStop = found && err == nil
		})

		outcome, pointer, err := NewPRReview("PR-Review", fx.deps).Call(context.Background())

		if err != nil || outcome != shedengine.Done || pointer.Halt {
			t.Fatalf("Call() = %q, halt %v, error %v; want Done without halt", outcome, pointer.Halt, err)
		}
		if diff := cmp.Diff([]string{firstGUID}, sessions.counted.stopped); diff != "" {
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
