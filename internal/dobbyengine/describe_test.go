// describe_test.go pins dobby's description rewrite over a fake shuttle, fake seams and a temp state record.
// It covers the landings the rewrite applies to and the spawn under a recorded pull request.
// It covers the record of a passing draft.
// It also covers the line a refused record or a timed-out agent posts, and the agent's prompt and launch spec.

package dobbyengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// describeTimeout is the describer agent's timeout in the harness.
const describeTimeout = 30 * time.Minute

// describeHarness drives a beam dobby's landing count and description rewrite over fakes.
type describeHarness struct {
	t       *testing.T
	d       *daemon
	shuttle *tidyShuttleFake
	notices []string
	// logged holds the dobby log lines written, without their timestamps.
	logged []string
	landed []string
	// pullRequest and pullRequestErr are what the PullRequestRecorded seam answers.
	pullRequest    bool
	pullRequestErr error
	// findings are what the Findings seam answers for a written draft.
	findings []string
	// recorded holds the draft path of each Record call, which fails with recordErr.
	// recordedDespiteErr is what Record reports as recorded beside that error.
	recorded           []string
	recordErr          error
	recordedDespiteErr bool
	// noticeErr is what the Notice seam fails with, posting nothing.
	noticeErr error
	now       time.Time
}

// fakeBeamDescribeDeps returns describe seams over shuttle.
// Its facts read, its draft has no findings and its record succeeds.
func fakeBeamDescribeDeps(shuttle AgentShuttle) BeamDescribeDeps {
	return BeamDescribeDeps{
		Shuttle: shuttle,
		Model:   shuttleengine.ModelChoice{Engine: "b", Model: "sonnet"},
		Timeout: describeTimeout,
		Facts: func() (BeamDescribeFacts, error) {
			return BeamDescribeFacts{
				DescriptionPath: "/hub/b/_lym/landing/summary.md",
				Branch:          "b",
				ParentBranch:    "main",
				Changes:         "- #266 Tool-joint pressure loss, merged 2026-10-30 13:05 UTC",
			}, nil
		},
		Findings: func(string) ([]string, error) { return nil, nil },
		Record:   func(context.Context, string) (bool, error) { return true, nil },
	}
}

// beamWorktreeRoot returns a directory that reads as a beam's live worktree, which holds a git link.
func beamWorktreeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: elsewhere\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() = %v; want nil", err)
	}
	return root
}

// startHookShuttle is a shuttle fake that runs after after each start has been recorded.
type startHookShuttle struct {
	*tidyShuttleFake
	after func()
}

func (s *startHookShuttle) Start(spec shuttleengine.Spec) (string, error) {
	guid, err := s.tidyShuttleFake.Start(spec)
	s.after()
	return guid, err
}

func newDescribeHarness(t *testing.T) *describeHarness {
	t.Helper()
	stencils := filepath.Join(t.TempDir(), "stencils")
	stencilkit.SeedInto(t, stencils)
	root := t.TempDir()
	h := &describeHarness{t: t, shuttle: &tidyShuttleFake{}, pullRequest: true, now: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)}
	describe := fakeBeamDescribeDeps(h.shuttle)
	describe.Findings = func(string) ([]string, error) { return h.findings, nil }
	describe.Record = func(_ context.Context, path string) (bool, error) {
		h.recorded = append(h.recorded, path)
		return h.recordedDespiteErr, h.recordErr
	}
	h.d = &daemon{
		geom: Geometry{WorktreeName: "b", Slug: "b", StatePath: filepath.Join(root, StateFileName), DescribeDir: filepath.Join(root, DescribeDirName)},
		deps: Deps{
			Landings:            func() ([]string, error) { return slices.Clone(h.landed), nil },
			PullRequestRecorded: func() (bool, error) { return h.pullRequest, h.pullRequestErr },
			StencilsDir:         stencils,
			NearestOrch:         func() (string, error) { return "ly:orch", nil },
			Log: func(_, line string, _ bool) {
				_, text, _ := strings.Cut(line, " ")
				h.logged = append(h.logged, text)
			},
			Notice: func(_ context.Context, line string) error {
				if h.noticeErr != nil {
					return h.noticeErr
				}
				h.notices = append(h.notices, line)
				return nil
			},
			BeamDescribe: describe,
		},
	}
	return h
}

// step runs the landing count and the description step at the harness's clock.
func (h *describeHarness) step() {
	h.t.Helper()
	if err := h.d.countLandings(h.now); err != nil {
		h.t.Fatalf("countLandings() = %v; want nil", err)
	}
	if err := h.d.describeStep(context.Background(), h.now); err != nil {
		h.t.Fatalf("describeStep() = %v; want nil", err)
	}
}

func (h *describeHarness) state() State {
	h.t.Helper()
	current, _, err := ReadState(h.d.geom.StatePath)
	if err != nil {
		h.t.Fatalf("ReadState() = %v; want nil", err)
	}
	return current
}

// writeDraft writes the in-flight agent's draft.
func (h *describeHarness) writeDraft() {
	h.t.Helper()
	path := h.state().Describe.DraftPath
	if err := os.WriteFile(path, []byte("# Ship the beam\n"), 0o644); err != nil {
		h.t.Fatalf("WriteFile(%s) = %v; want nil", path, err)
	}
}

func TestDescribeStep(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("a member landing under a recorded pull request starts the describer over the description and the beam's changes", func(t *testing.T) {
		t.Parallel()
		h := newDescribeHarness(t)
		h.step()
		h.landed = []string{"m1"}
		h.step()
		if len(h.shuttle.started) != 1 {
			t.Fatalf("agents started = %d; want 1", len(h.shuttle.started))
		}
		wantDraft := filepath.Join(h.d.geom.DescribeDir, h.now.UTC().Format("20060102T150405Z")+".md")
		for _, want := range []string{wantDraft, "/hub/b/_lym/landing/summary.md", "#266 Tool-joint pressure loss", "`b`", "main"} {
			if !strings.Contains(h.shuttle.started[0].Prompt, want) {
				t.Errorf("prompt does not name %q", want)
			}
		}
		describe := h.state().Describe
		if describe == nil || describe.StrandGUID != "strand-1" || describe.DraftPath != wantDraft ||
			!describe.Deadline.Equal(h.now.Add(describeTimeout)) || !slices.Equal(describe.Landings, []string{"m1"}) {
			t.Fatalf("state Describe = %+v; want strand-1 over %s, a deadline 30 minutes out and the landing m1", describe, wantDraft)
		}
		if want := "description rewrite started after m1: " + wantDraft; !slices.Equal(h.logged, []string{want}) {
			t.Errorf("log lines = %q; want the one %q", h.logged, want)
		}
		h.d.busy = false
		h.step()
		if !h.d.busy {
			t.Errorf("daemon busy = false with the agent in flight; want true, so the loop keeps out of its idle backoff")
		}
	})

	t.Run("a spawn that fails records no agent, drops the landings and tells the orch why", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			breakIt func(h *describeHarness)
			wantErr string
		}{
			{
				name: "FactsUnreadable",
				breakIt: func(h *describeHarness) {
					h.d.deps.BeamDescribe.Facts = func() (BeamDescribeFacts, error) { return BeamDescribeFacts{}, errors.New("no origin record") }
				},
				wantErr: "read the beam's facts for the describer agent: no origin record",
			},
			{
				name: "NearestOrchUnreadable",
				breakIt: func(h *describeHarness) {
					h.d.deps.NearestOrch = func() (string, error) { return "", errors.New("creel is down") }
				},
				wantErr: "read the nearest live orch: creel is down",
			},
			{
				name:    "EngineUnresolved",
				breakIt: func(h *describeHarness) { h.shuttle.engineErr = errors.New("the runner did not build") },
				wantErr: "resolve the describer agent's engine: the runner did not build",
			},
			{
				name:    "StencilMissing",
				breakIt: func(h *describeHarness) { stencilkit.Remove(h.t, h.d.deps.StencilsDir, beamDescribeStencilName) },
				wantErr: beamDescribeStencilName,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newDescribeHarness(t)
				h.step()
				h.landed = []string{"m1"}
				tt.breakIt(h)
				if err := h.d.countLandings(h.now); err != nil {
					t.Fatalf("countLandings() = %v; want nil", err)
				}
				if err := h.d.describeStep(context.Background(), h.now); err != nil {
					t.Errorf("describeStep() = %v; want nil, the failure posted to the orch", err)
				}
				if len(h.notices) != 1 || !strings.Contains(h.notices[0], "b: the description rewrite after m1 did not start: ") ||
					!strings.Contains(h.notices[0], tt.wantErr) || !strings.HasSuffix(h.notices[0], describeStandsWayForward) {
					t.Errorf("notices = %q; want the one line naming the failed start and %q", h.notices, tt.wantErr)
				}
				if got := h.state(); len(h.shuttle.started) != 0 || got.Describe != nil || len(got.LandedSinceDescribe) != 0 {
					t.Errorf("agents started, state = %d, Describe %+v, landings %v; want none started, none recorded and no landings left", len(h.shuttle.started), got.Describe, got.LandedSinceDescribe)
				}
			})
		}
	})

	t.Run("a spawn failure whose post fails keeps the landings and posts on the next tick", func(t *testing.T) {
		t.Parallel()
		h := newDescribeHarness(t)
		h.step()
		h.landed = []string{"m1"}
		h.d.deps.NearestOrch = func() (string, error) { return "", errors.New("creel is down") }
		h.noticeErr = errors.New("creel is down")
		if err := h.d.countLandings(h.now); err != nil {
			t.Fatalf("countLandings() = %v; want nil", err)
		}
		err := h.d.describeStep(context.Background(), h.now)
		if err == nil || !strings.Contains(err.Error(), "post the description rewrite's line: creel is down") {
			t.Errorf("describeStep() = %v; want the post's error", err)
		}
		if got := h.state().LandedSinceDescribe; !slices.Equal(got, []string{"m1"}) {
			t.Fatalf("landings to describe = %v; want m1 kept", got)
		}
		h.noticeErr = nil
		if err := h.d.describeStep(context.Background(), h.now); err != nil {
			t.Errorf("describeStep() = %v; want nil once the post goes out", err)
		}
		if got := h.state().LandedSinceDescribe; len(h.notices) != 1 || len(got) != 0 {
			t.Errorf("notices, landings to describe = %q, %v; want the one line and none left", h.notices, got)
		}
	})

	t.Run("a state record that cannot be written after the start ends the started agent again", func(t *testing.T) {
		t.Parallel()
		h := newDescribeHarness(t)
		h.step()
		h.landed = []string{"m1"}
		h.d.deps.BeamDescribe.Shuttle = &startHookShuttle{tidyShuttleFake: h.shuttle, after: func() {
			if err := os.Remove(h.d.geom.StatePath); err != nil {
				t.Errorf("Remove(%s) = %v; want nil", h.d.geom.StatePath, err)
			}
			if err := os.Mkdir(h.d.geom.StatePath, 0o755); err != nil {
				t.Errorf("Mkdir(%s) = %v; want nil", h.d.geom.StatePath, err)
			}
		}}
		if err := h.d.countLandings(h.now); err != nil {
			t.Fatalf("countLandings() = %v; want nil", err)
		}
		if err := h.d.describeStep(context.Background(), h.now); err == nil {
			t.Errorf("describeStep() = nil; want the state write's error")
		}
		if !slices.Equal(h.shuttle.stopped, []string{"strand-1"}) {
			t.Errorf("strands stopped = %v; want [strand-1], the agent that was started", h.shuttle.stopped)
		}
	})

	t.Run("a draft whose findings do not read fails the step while the deadline stands, and ends the agent after it", func(t *testing.T) {
		t.Parallel()
		h := newDescribeHarness(t)
		h.step()
		h.landed = []string{"m1"}
		h.step()
		h.writeDraft()
		h.d.deps.BeamDescribe.Findings = func(string) ([]string, error) { return nil, errors.New("the draft is unreadable") }
		err := h.d.describeStep(context.Background(), h.now)
		if err == nil || !strings.Contains(err.Error(), "check the describer agent's draft: the draft is unreadable") {
			t.Errorf("describeStep() = %v; want the findings error", err)
		}
		if h.state().Describe == nil || len(h.recorded) != 0 {
			t.Errorf("state Describe, drafts recorded = %+v, %v; want the agent kept and nothing recorded", h.state().Describe, h.recorded)
		}
		h.now = h.now.Add(describeTimeout + time.Minute)
		if err := h.d.describeStep(context.Background(), h.now); err != nil {
			t.Fatalf("describeStep() past the deadline = %v; want nil", err)
		}
		want := "b: the describer agent gave no valid description after m1 by its deadline (draft unreadable: the draft is unreadable)" + describeStandsWayForward
		if !slices.Equal(h.notices, []string{want}) || !slices.Equal(h.shuttle.stopped, []string{"strand-1"}) {
			t.Errorf("notices, strands stopped = %q, %v; want the one %q and [strand-1]", h.notices, h.shuttle.stopped, want)
		}
		if got := h.state(); got.Describe != nil || len(got.LandedSinceDescribe) != 0 {
			t.Fatalf("state = Describe %+v, landings %v; want both cleared", got.Describe, got.LandedSinceDescribe)
		}
		h.landed = []string{"m1", "m2"}
		h.step()
		if len(h.shuttle.started) != 2 {
			t.Errorf("agents started = %d; want a second agent for the next landing", len(h.shuttle.started))
		}
	})

	t.Run("a landing with no pull request recorded starts none and is dropped", func(t *testing.T) {
		t.Parallel()
		h := newDescribeHarness(t)
		h.pullRequest = false
		h.step()
		h.landed = []string{"m1"}
		h.step()
		h.pullRequest = true
		h.step()
		if len(h.shuttle.started) != 0 || len(h.state().LandedSinceDescribe) != 0 {
			t.Errorf("agents started, landings to describe = %d, %v; want none of either", len(h.shuttle.started), h.state().LandedSinceDescribe)
		}
	})

	t.Run("an unreadable landing record keeps the landing until it reads", func(t *testing.T) {
		t.Parallel()
		h := newDescribeHarness(t)
		h.step()
		h.pullRequestErr = errors.New("the landing record is unreadable")
		h.landed = []string{"m1"}
		h.step()
		if got := h.state().LandedSinceDescribe; len(h.shuttle.started) != 0 || !slices.Equal(got, []string{"m1"}) {
			t.Fatalf("agents started, landings to describe = %d, %v; want none started and m1 kept", len(h.shuttle.started), got)
		}
		h.pullRequestErr = nil
		h.step()
		if len(h.shuttle.started) != 1 {
			t.Errorf("agents started = %d; want 1 once the record reads", len(h.shuttle.started))
		}
	})

	t.Run("a passing draft is recorded once and ends the agent, and a landing meanwhile gets a rewrite of its own", func(t *testing.T) {
		t.Parallel()
		h := newDescribeHarness(t)
		h.step()
		h.landed = []string{"m1"}
		h.step()
		draft := h.state().Describe.DraftPath
		h.landed = []string{"m1", "m2"}
		h.writeDraft()
		// A strand that does not stop keeps nothing from the record: the agent ends at its own timeout.
		h.shuttle.stopErr = errors.New("the strand is gone")
		h.d.busy = true
		h.step()
		if !h.d.busy {
			t.Errorf("daemon busy = false after the agent settled; want the true an earlier step set kept")
		}
		if !slices.Equal(h.recorded, []string{draft}) {
			t.Fatalf("drafts recorded = %v; want [%s]", h.recorded, draft)
		}
		if !slices.Equal(h.shuttle.stopped, []string{"strand-1"}) {
			t.Errorf("strands stopped = %v; want [strand-1]", h.shuttle.stopped)
		}
		if !slices.Contains(h.logged, "description rewritten after m1") || len(h.notices) != 0 {
			t.Errorf("log lines, notices = %q, %q; want the rewrite line and no notice", h.logged, h.notices)
		}
		if got := h.state(); got.Describe != nil || !slices.Equal(got.LandedSinceDescribe, []string{"m2"}) {
			t.Fatalf("state = Describe %+v, landings %v; want no agent and m2 left to describe", got.Describe, got.LandedSinceDescribe)
		}
		h.step()
		if len(h.shuttle.started) != 2 || len(h.recorded) != 1 {
			t.Fatalf("agents started, drafts recorded = %d, %d; want a second agent and no second record", len(h.shuttle.started), len(h.recorded))
		}
		if got := h.state().Describe.Landings; !slices.Equal(got, []string{"m2"}) {
			t.Errorf("second agent's landings = %v; want [m2]", got)
		}
	})

	t.Run("a record that fails posts why, and the next landing rewrites again", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name               string
			recordedDespiteErr bool
			recordErr          string
			wantNotice         string
			wantLog            string
		}{
			{
				name:       "Refused",
				recordErr:  "lym beam describe: the beam b is flagged ready",
				wantNotice: "b: the description rewrite after m1 was not recorded: lym beam describe: the beam b is flagged ready; the recorded description stands, and the next member landing rewrites it again",
				wantLog:    "description rewrite not recorded: lym beam describe: the beam b is flagged ready",
			},
			{
				name:               "RecordedButPullRequestNotUpdated",
				recordedDespiteErr: true,
				recordErr:          "lym beam describe: the description of b is recorded, but its pull request is not updated: push refused",
				wantNotice:         "b: the description rewrite after m1: lym beam describe: the description of b is recorded, but its pull request is not updated: push refused; way forward: run `lym beam pr update b`",
				wantLog:            "description recorded, its pull request not updated: lym beam describe: the description of b is recorded, but its pull request is not updated: push refused",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newDescribeHarness(t)
				h.recordErr = errors.New(tt.recordErr)
				h.recordedDespiteErr = tt.recordedDespiteErr
				h.step()
				h.landed = []string{"m1"}
				h.step()
				h.writeDraft()
				h.step()
				if !slices.Equal(h.notices, []string{tt.wantNotice}) {
					t.Errorf("notices = %q; want the one %q", h.notices, tt.wantNotice)
				}
				if !slices.Contains(h.logged, tt.wantLog) {
					t.Errorf("log lines = %q; want %q", h.logged, tt.wantLog)
				}
				if got := h.state(); got.Describe != nil || len(got.LandedSinceDescribe) != 0 {
					t.Fatalf("state = Describe %+v, landings %v; want both cleared", got.Describe, got.LandedSinceDescribe)
				}
				h.landed = []string{"m1", "m2"}
				h.step()
				if len(h.shuttle.started) != 2 {
					t.Errorf("agents started = %d; want a second agent for the next landing", len(h.shuttle.started))
				}
			})
		}
	})

	t.Run("a line whose post fails is posted on the next tick, and the draft is not recorded again", func(t *testing.T) {
		t.Parallel()
		h := newDescribeHarness(t)
		h.recordErr = errors.New("the beam b is flagged ready")
		h.step()
		h.landed = []string{"m1"}
		h.step()
		h.writeDraft()
		h.noticeErr = errors.New("creel is down")
		err := h.d.describeStep(context.Background(), h.now)
		if err == nil || !strings.Contains(err.Error(), "post the description rewrite's line: creel is down") {
			t.Errorf("describeStep() = %v; want the post's error", err)
		}
		if got := h.state(); got.Describe == nil || got.Describe.OwedLine == "" || len(h.recorded) != 1 {
			t.Fatalf("state Describe, drafts recorded = %+v, %d; want the agent kept with its line owed and one record", got.Describe, len(h.recorded))
		}
		h.noticeErr = nil
		h.step()
		want := "b: the description rewrite after m1 was not recorded: the beam b is flagged ready" + describeStandsWayForward
		if !slices.Equal(h.notices, []string{want}) || len(h.recorded) != 1 {
			t.Errorf("notices, drafts recorded = %q, %d; want the one %q and no second record", h.notices, len(h.recorded), want)
		}
		if got := h.state(); got.Describe != nil || len(got.LandedSinceDescribe) != 0 {
			t.Errorf("state = Describe %+v, landings %v; want both cleared", got.Describe, got.LandedSinceDescribe)
		}
	})

	t.Run("an agent with no passing draft by its deadline is ended with a line naming why", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name     string
			draft    bool
			findings []string
			cause    string
		}{
			{name: "NoDraft", cause: "no draft written"},
			{name: "DraftWithFindings", draft: true, findings: []string{"empty-body: body is empty"}, cause: "draft findings: empty-body: body is empty"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newDescribeHarness(t)
				h.findings = tt.findings
				h.step()
				h.landed = []string{"m1"}
				h.step()
				if tt.draft {
					h.writeDraft()
					h.step()
					if h.state().Describe == nil || len(h.recorded) != 0 {
						t.Fatalf("state Describe, drafts recorded = %+v, %v; want the agent still waited on and nothing recorded", h.state().Describe, h.recorded)
					}
				}
				h.now = h.now.Add(describeTimeout + time.Minute)
				h.step()
				want := "b: the describer agent gave no valid description after m1 by its deadline (" + tt.cause + "); the recorded description stands, and the next member landing rewrites it again"
				if !slices.Equal(h.notices, []string{want}) {
					t.Errorf("notices = %q; want the one %q", h.notices, want)
				}
				if !slices.Equal(h.shuttle.stopped, []string{"strand-1"}) || len(h.recorded) != 0 {
					t.Errorf("strands stopped, drafts recorded = %v, %v; want [strand-1] and none", h.shuttle.stopped, h.recorded)
				}
				if got := h.state(); got.Describe != nil || len(got.LandedSinceDescribe) != 0 {
					t.Errorf("state = Describe %+v, landings %v; want both cleared", got.Describe, got.LandedSinceDescribe)
				}
			})
		}
	})

	t.Run("the prime's dobby rewrites nothing", func(t *testing.T) {
		t.Parallel()
		h := newDescribeHarness(t)
		h.d.geom.Slug = ""
		h.step()
		h.landed = []string{"m1"}
		h.step()
		if len(h.shuttle.started) != 0 || len(h.state().LandedSinceDescribe) != 0 {
			t.Errorf("agents started, landings to describe = %d, %v; want none of either", len(h.shuttle.started), h.state().LandedSinceDescribe)
		}
	})
}

// TestRun_DescribeFailureLeavesTheTidyRunning drives ticks in which the describe step fails every tick.
// The board-tidy step of the same ticks still settles its agent, and no tick is logged as failing.
func TestRun_DescribeFailureLeavesTheTidyRunning(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	h := newLoopHarness(t, 3)
	stencils := filepath.Join(t.TempDir(), "stencils")
	stencilkit.SeedInto(t, stencils)
	stateDir := filepath.Dir(h.geom.StatePath)
	h.geom.WorktreeName, h.geom.Slug, h.geom.WorktreeRoot = "b", "b", beamWorktreeRoot(t)
	h.geom.DescribeDir = filepath.Join(stateDir, DescribeDirName)
	h.geom.TidyDir = filepath.Join(stateDir, TidyDirName)

	landingReads := 0
	h.deps.Landings = func() ([]string, error) {
		landingReads++
		if landingReads == 1 {
			return nil, nil
		}
		return []string{"m1"}, nil
	}
	h.deps.LoadConfig = func() (Config, error) { return Config{BoardTidyEvery: 1, BoardTidyTimeoutMin: 30}, nil }
	h.deps.TidyShuttle = &tidyShuttleFake{}
	h.deps.TidyModel = shuttleengine.ModelChoice{Engine: "b", Model: "sonnet"}
	h.deps.StencilsDir = stencils
	h.deps.NearestOrch = func() (string, error) { return "ly:orch", nil }
	h.deps.PullRequestRecorded = func() (bool, error) { return true, nil }
	h.deps.BeamDescribe = fakeBeamDescribeDeps(&tidyShuttleFake{})
	h.deps.BeamDescribe.Findings = func(string) ([]string, error) { return nil, errors.New("the draft is unreadable") }

	// Tick 2 starts both agents; the draft and the report written after it make tick 3 settle both.
	h.perTick = func(n int) error {
		if n != 2 {
			return nil
		}
		current, _, err := ReadState(h.geom.StatePath)
		if err != nil {
			return err
		}
		if current.Describe == nil || current.Tidy == nil {
			t.Errorf("state after tick 2 = Describe %+v, Tidy %+v; want both agents started", current.Describe, current.Tidy)
			return nil
		}
		for _, path := range []string{current.Describe.DraftPath, current.Tidy.ReportPath} {
			if err := os.WriteFile(path, []byte("# Ship the beam\n"), 0o644); err != nil {
				return err
			}
		}
		return nil
	}

	if err := h.run(t); err != nil {
		t.Fatalf("Run() = %v; want nil", err)
	}
	if !slices.ContainsFunc(h.noticed(), func(line string) bool { return strings.Contains(line, "board tidy after 1 landings") }) {
		t.Errorf("notices = %q; want the board-tidy report line, which the failing describe step must not hold back", h.noticed())
	}
	if _, found := h.loggedLine("tick failing: check the describer agent's draft: the draft is unreadable"); found {
		t.Errorf("the log holds a failing tick; want the describe error logged by its own episode only")
	}
	current, _, err := ReadState(h.geom.StatePath)
	if err != nil || current.Describe == nil || current.Tidy != nil {
		t.Errorf("state = Describe %+v, Tidy %+v, error %v; want the describer kept and the tidy settled", current.Describe, current.Tidy, err)
	}
}

func TestBeamDescribePrompt(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	stencils := filepath.Join(t.TempDir(), "stencils")
	stencilkit.SeedInto(t, stencils)

	input := BeamDescribePromptInput{
		Slug:      "hydraulics",
		DraftPath: "/hub/hydraulics/.lym/beam/describe/20261122T120000Z.md",
		Parent:    "ly:orch",
		Facts: BeamDescribeFacts{
			DescriptionPath: "/hub/hydraulics/_lym/landing/summary.md",
			Branch:          "hydraulics",
			ParentBranch:    "main",
			Changes:         "- #266 Tool-joint pressure loss, merged 2026-10-30 13:05 UTC",
		},
	}
	prompt, err := BeamDescribePrompt(stencils, "claude", input)
	if err != nil {
		t.Fatalf("BeamDescribePrompt() = _, %v; want nil", err)
	}
	stencilkit.RequireEditDirectiveOf(t, stencils, prompt, "claude", "codex")
	codexPrompt, err := BeamDescribePrompt(stencils, "codex", input)
	if err != nil {
		t.Fatalf("BeamDescribePrompt(codex) = _, %v; want nil", err)
	}
	stencilkit.RequireEditDirectiveOf(t, stencils, codexPrompt, "codex", "claude")
	for _, want := range []string{"`hydraulics`", input.DraftPath, input.Facts.DescriptionPath, "main...hydraulics", input.Facts.Changes, "ly:orch"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("BeamDescribePrompt() lacks %q", want)
		}
	}
	if strings.Contains(prompt, "{{") {
		t.Errorf("BeamDescribePrompt() holds an unfilled marker")
	}
	input.Facts.Changes = ""
	empty, err := BeamDescribePrompt(stencils, "claude", input)
	if err != nil || !strings.Contains(empty, noBeamChanges) {
		t.Errorf("BeamDescribePrompt(no changes) = %v; want the prompt to name a beam with no commit its parent lacks", err)
	}

	spec := BeamDescribeSpec(prompt, input.DraftPath, "claude", shuttleengine.ModelChoice{Engine: "b", Model: "sonnet"}, describeTimeout)
	if spec.PromptEngine != "claude" || spec.Segment != segmentcolor.Describe || spec.ConfigRole != agentconfig.RoleDescriber {
		t.Errorf("BeamDescribeSpec() = engine %q, segment %v, config role %q; want claude, the describe segment and the describer role", spec.PromptEngine, spec.Segment, spec.ConfigRole)
	}
	if spec.Role != BeamDescribeRole || !slices.Equal(spec.OutputFiles, []string{input.DraftPath}) || spec.Timeout != describeTimeout {
		t.Errorf("BeamDescribeSpec() = role %q, outputs %v, timeout %v; want the role, the draft and the timeout", spec.Role, spec.OutputFiles, spec.Timeout)
	}
}
