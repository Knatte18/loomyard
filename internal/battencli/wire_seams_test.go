package battencli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

// intentionallyNil maps each shedrecipe.Env field path wire leaves nil to the reason.
// envkit.NilSeams already skips the seams whose nil is a documented default, so only wire's own gaps are listed.
// batten wires only its own producers, so every loom seam of shedrecipe.Env stays nil here.
var intentionallyNil = map[string]string{
	"VerifyCommand":             "loom-only seam, batten runs no verify gate",
	"Shuttle":                   "loom-only seam, batten drives no agent itself",
	"Burler":                    "loom-only seam, batten runs no review round",
	"Seats":                     "loom-only seam, batten runs no seat table",
	"Models":                    "loom-only seam, batten resolves no seat model",
	"WebsterRun":                "loom-only seam, batten runs no webster",
	"WebsterDeps.Starter":       "loom-only seam, batten runs no webster",
	"WebsterDeps.Stopper":       "loom-only seam, batten runs no webster",
	"WebsterDeps.Engine":        "loom-only seam, batten runs no webster",
	"WebsterDeps.RefMatcher":    "loom-only seam, batten runs no webster",
	"WebsterDeps.Geom.Index":    "loom-only seam, batten runs no webster",
	"PlanIndex":                 "loom-only seam, batten has no plan segment",
	"CommitWebster":             "loom-only seam, batten runs no webster",
	"Landing.PushBranch":        "loom-only seam, batten lands no branch",
	"Landing.RemoteOnlyCommits": "loom-only seam, batten lands no branch",
	"Landing.OpenFabric":        "loom-only seam, batten lands no branch",
	"Landing.OpenParentFabric":  "loom-only seam, batten lands no branch",
	"Landing.TaskHead":          "loom-only seam, batten lands no branch",
	"Landing.Shuttle":           "loom-only seam, batten lands no branch",
	"DiscussionSpec":            "loom-only seam, batten has no discussion segment",
	"DiscussionTable":           "loom-only seam, batten has no discussion segment",
	"CommitDiscussion":          "loom-only seam, batten has no discussion segment",
	"DescribeSpec":              "loom-only seam, batten has no describe segment",
	"CommitDescription":         "loom-only seam, batten has no describe segment",
	"PlanSpec":                  "loom-only seam, batten has no plan segment",
	"CommitPlan":                "loom-only seam, batten has no plan segment",
	"ApprovePlan":               "loom-only seam, batten has no plan segment",
	"SkipPlanReview":            "loom-only seam, batten has no plan segment",
	"CarryOver":                 "loom-only seam, batten runs no review segment",
	"ReflectFriction":           "loom-only seam, batten has no reflect segment",
	"ReworkSpec":                "loom-only seam, batten has no rework segment",
	"Rework.ReadCommitted":      "loom-only seam, batten has no rework segment",
	"Rework.ReadRejection":      "loom-only seam, batten has no rework segment",
	"Rework.ClearRejection":     "loom-only seam, batten has no rework segment",
	"Rework.ArchiveWebster":     "loom-only seam, batten has no rework segment",
	"Rework.Commit":             "loom-only seam, batten has no rework segment",
}

func TestWire_EverySeamFilled(t *testing.T) {
	t.Parallel()

	location := &lyxcwd.Location{
		RepoName:     "example",
		HubPath:      t.TempDir(),
		WorktreeName: "hub-repo",
		AnchorRel:    ".",
	}

	c := &battenCLI{}
	if err := c.wire(location, "a-slug-with-no-worktree-anywhere"); err != nil {
		t.Fatalf("wire() error = %v; want nil", err)
	}

	nils := envkit.NilSeams(c.env)
	for _, path := range nils {
		if _, ok := intentionallyNil[path]; !ok {
			t.Errorf("c.env.%s is nil after wire(); fill it or list it in intentionallyNil with a reason", path)
		}
	}
	for path := range intentionallyNil {
		if !slices.Contains(nils, path) {
			t.Errorf("intentionallyNil lists c.env.%s but wire() now fills it; remove the entry", path)
		}
	}
}

// wiredPrime wires a battenCLI over a prime location rooted in a temp hub with no task worktree.
func wiredPrime(t *testing.T) (*battenCLI, *lyxcwd.Location) {
	t.Helper()
	location := &lyxcwd.Location{
		RepoName:     "example",
		HubPath:      t.TempDir(),
		WorktreeName: "hub-repo",
		AnchorRel:    ".",
	}
	c := &battenCLI{}
	if err := c.wire(location, "a-slug"); err != nil {
		t.Fatalf("wire() error = %v; want nil", err)
	}
	return c, location
}

func TestWire_NotifyQueuesOnThePrimesOrch(t *testing.T) {
	t.Parallel()

	c, location := wiredPrime(t)
	paths := orchcli.PrimePaths(location)
	if err := orchengine.SaveState(paths, orchengine.State{Strand: "orch-strand", Phase: orchengine.PhaseIdle}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	if queued, err := c.env.InnerRun.Notify(context.Background(), "batten a-slug: child left running"); err != nil || !queued {
		t.Fatalf("Notify() = %v, %v; want the notice queued", queued, err)
	}

	wantDir := filepath.Join(location.AnchorPath(), ".lyx", "orch", "notices")
	if paths.NoticesDir != wantDir {
		t.Fatalf("NoticesDir = %q; want %q", paths.NoticesDir, wantDir)
	}
	notices, err := orchengine.ListNotices(paths)
	if err != nil {
		t.Fatalf("ListNotices: %v", err)
	}
	if len(notices) != 1 || notices[0].Line != "batten a-slug: child left running" {
		t.Errorf("queued notices = %+v; want exactly the one notice line", notices)
	}
}

func TestWire_NotifyWritesNothingWithoutAnOrchStrand(t *testing.T) {
	t.Parallel()

	c, location := wiredPrime(t)
	paths := orchcli.PrimePaths(location)

	if queued, err := c.env.InnerRun.Notify(context.Background(), "batten a-slug: child left running"); err != nil || queued {
		t.Fatalf("Notify() = %v, %v; want the notice reported not queued, without an error", queued, err)
	}

	if _, err := os.Stat(paths.NoticesDir); !os.IsNotExist(err) {
		t.Errorf("stat %s error = %v; want the queue directory absent", paths.NoticesDir, err)
	}
}

func TestWire_AttachDirRefusesAnAbsentTaskWorktreeByName(t *testing.T) {
	t.Parallel()

	c, _ := wiredPrime(t)

	_, err := c.env.InnerRun.AttachDir()
	if err == nil || !strings.Contains(err.Error(), "a-slug") {
		t.Errorf("AttachDir() error = %v; want the absent-worktree refusal naming the slug", err)
	}
}

// TestWire_PauseRequestedReadsBattensOwnStatus asserts the pause seam reports the pause_requested flag of batten's own status file, and false while that file is absent.
func TestWire_PauseRequestedReadsBattensOwnStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status *shedengine.Status
		want   bool
	}{
		{name: "AbsentStatusIsNotPaused"},
		{name: "FlagClear", status: &shedengine.Status{State: shedengine.StateRunning}},
		{name: "FlagSet", status: &shedengine.Status{State: shedengine.StateRunning, PauseRequested: true}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c, location := wiredPrime(t)
			statusPath, lockPath := StatusFile(location, "a-slug"), StatusLock(location, "a-slug")
			// The batten pre-run creates both directories before any verb runs.
			for _, dir := range []string{filepath.Dir(statusPath), filepath.Dir(lockPath)} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if tt.status != nil {
				if err := state.WriteJSON(statusPath, lockPath, *tt.status); err != nil {
					t.Fatal(err)
				}
			}

			got, err := c.env.InnerRun.PauseRequested()
			if err != nil || got != tt.want {
				t.Errorf("PauseRequested() = %v, %v; want %v, nil", got, err, tt.want)
			}
		})
	}
}

// TestWire_MarkerAndReportSeamsRefuseAnAbsentTaskWorktree asserts the watched-marker seams and the stop-report seam return an error, rather than reporting not held or not found, when the task worktree their files live in is absent.
func TestWire_MarkerAndReportSeamsRefuseAnAbsentTaskWorktree(t *testing.T) {
	t.Parallel()

	c, _ := wiredPrime(t)

	for name, mark := range map[string]func(context.Context) (bool, error){"RunShed": c.env.InnerRun.MarkWatched, "SeedChild": c.env.SeedChild.MarkWatched} {
		held, err := mark(context.Background())
		if err == nil || held || !strings.Contains(err.Error(), "a-slug") {
			t.Errorf("%s MarkWatched() = %v, %v; want not held and the absent-worktree refusal naming the slug", name, held, err)
		}
	}
	if _, _, found, err := c.env.InnerRun.StopReport(); err == nil || found || !strings.Contains(err.Error(), "a-slug") {
		t.Errorf("StopReport() found=%v, error=%v; want the absent-worktree refusal naming the slug", found, err)
	}
	if runs, _, err := c.env.InnerRun.Activity(context.Background()); err == nil || runs != nil || !strings.Contains(err.Error(), "a-slug") {
		t.Errorf("Activity() runs=%v, error=%v; want the absent-worktree refusal naming the slug", runs, err)
	}
}

// TestReadAgentActivity_ReadsLiveRunsAndTheWaitMarker asserts the Activity seam's reading maps a task worktree's live shuttle run onto battenshed's type, with the session state of its stamped turn end, skips a run whose process has ended, and reports a live wait marker.
// It points HOME at a session registry through t.Setenv, so it does not run in parallel.
func TestReadAgentActivity_ReadsLiveRunsAndTheWaitMarker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	registry := filepath.Join(home, ".claude", "sessions")
	if err := os.MkdirAll(registry, 0o755); err != nil {
		t.Fatal(err)
	}
	// A registry entry for the live run's session, proven alive by this test process, where the platform reads a process start time.
	wantState, wantCause := "unknown", "liveness-unproven"
	if procStart, ok := proc.StartTime(os.Getpid()); ok {
		entry := fmt.Sprintf(`{"pid":%d,"sessionId":"sess-1","procStart":%q}`, os.Getpid(), procStart)
		if err := os.WriteFile(filepath.Join(registry, "live.json"), []byte(entry), 0o644); err != nil {
			t.Fatal(err)
		}
		wantState, wantCause = "idle-stalled", "no-output"
	}

	taskLocation := &lyxcwd.Location{RepoName: "example", HubPath: t.TempDir(), WorktreeName: "task", AnchorRel: "."}
	runsRoot := filepath.Join(taskLocation.AnchorPath(), ".lyx", "shuttle")
	writeRun := func(name string, pid int, strand string) string {
		dir := filepath.Join(runsRoot, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		record := fmt.Sprintf(`{"runId":%q,"strandName":%q,"outcome":"running","pid":%d,"eventsPath":%q,"createdAt":"2026-01-01T09:00:00Z"}`, name, strand, pid, filepath.Join(dir, "events"))
		if err := os.WriteFile(filepath.Join(dir, "run.json"), []byte(record), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	livePID := os.Getpid()

	liveDir := writeRun("live", livePID, "hub:task:impl")
	activeAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	eventsPath := filepath.Join(liveDir, "events")
	stampedStop := fmt.Sprintf(`{"lyx_stamp":"Stop","lyx_at":%q}`+"\n"+`{"hook_event_name":"Stop","session_id":"sess-1"}`+"\n", activeAt.Format(time.RFC3339))
	if err := os.WriteFile(eventsPath, []byte(stampedStop), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(eventsPath, activeAt, activeAt); err != nil {
		t.Fatal(err)
	}
	writeRun("ended", 1<<30, "hub:task:review")

	runs, waitLive, err := readAgentActivity(taskLocation)
	if err != nil || waitLive || len(runs) != 1 || runs[0].Producer != "hub:task:impl" || !runs[0].LastActivity.Equal(activeAt) || runs[0].APIError {
		t.Fatalf("readAgentActivity() = %+v, %v, %v; want the live run hub:task:impl last active at %s, no wait", runs, waitLive, err, activeAt)
	}
	if runs[0].SessionState != wantState || runs[0].SessionCause != wantCause {
		t.Errorf("session state = %q/%q, want %q/%q", runs[0].SessionState, runs[0].SessionCause, wantState, wantCause)
	}

	writeMarker := func(dir, kind string) {
		marker := fmt.Sprintf("kind: %s\nstarted: 2026-01-01T10:00:00Z\npid: %d\n", kind, livePID)
		if err := os.WriteFile(filepath.Join(dir, "wait.yaml"), []byte(marker), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeMarker(liveDir, "held")
	if _, waitLive, err := readAgentActivity(taskLocation); err != nil || waitLive {
		t.Errorf("readAgentActivity() waitLive = %v, error = %v with only a held marker; want false, nil", waitLive, err)
	}
	writeMarker(filepath.Join(runsRoot, "ended"), "background shells")
	if _, waitLive, err := readAgentActivity(taskLocation); err != nil || !waitLive {
		t.Errorf("readAgentActivity() waitLive = %v, error = %v with a held marker beside a live background-shell marker; want true, nil", waitLive, err)
	}
	writeMarker(liveDir, "background shells")
	if err := os.Remove(filepath.Join(runsRoot, "ended", "wait.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, waitLive, err := readAgentActivity(taskLocation); err != nil || !waitLive {
		t.Errorf("readAgentActivity() waitLive = %v, error = %v with a live wait marker; want true, nil", waitLive, err)
	}
}

// TestWire_OrchStrandRecordedReadsThePrimesOrchState asserts the seam reports whether the prime orch's state records a strand, the fact a notice needs a destination for.
func TestWire_OrchStrandRecordedReadsThePrimesOrchState(t *testing.T) {
	t.Parallel()

	c, location := wiredPrime(t)
	if recorded, err := c.env.InnerRun.OrchStrandRecorded(); err != nil || recorded {
		t.Fatalf("OrchStrandRecorded() = %v, %v with no orch state; want false", recorded, err)
	}

	paths := orchcli.PrimePaths(location)
	if err := orchengine.SaveState(paths, orchengine.State{Strand: "orch-strand", Phase: orchengine.PhaseIdle}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if recorded, err := c.env.InnerRun.OrchStrandRecorded(); err != nil || !recorded {
		t.Errorf("OrchStrandRecorded() = %v, %v with a strand recorded; want true", recorded, err)
	}
}

// TestWire_CreateWorktreeClaimsTheBoardEntry asserts the Worktree-Create closure writes the run-status claim before it touches the pair, and that a failed claim only warns.
// Both rows fail the closure on the absent fabric config, so no pair is created.
// It sets BOARD_SKIP_GIT and BOARD_SKIP_PUSH and redirects the logger's output, all process-global, so it does not call t.Parallel.
func TestWire_CreateWorktreeClaimsTheBoardEntry(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	t.Setenv("BOARD_SKIP_PUSH", "1")
	logs := logcapture.Capture(t)

	const slug = "claimed"
	tests := []struct {
		name string
		// hub builds the hub and returns its path.
		hub func(t *testing.T) string
		// wantStatus is the entry's status after the call; empty when the row has no board config to read it from.
		wantStatus string
		wantWarn   bool
	}{
		{
			name: "ClaimSurvivesTheFailedRow",
			hub: func(t *testing.T) string {
				hub := boardkit.HubWithBoardConfig(t)
				board, err := boardengine.OpenHub(hub)
				if err != nil {
					t.Fatalf("OpenHub: %v", err)
				}
				if _, err := board.UpsertTask(map[string]any{"slug": slug, "kind": "task", "labels": []string{"bug"}}); err != nil {
					t.Fatalf("seed: %v", err)
				}
				return hub
			},
			wantStatus: "running · Worktree-Create",
		},
		{
			name: "FailedClaimWarnsAndTheRowContinues",
			hub: func(t *testing.T) string {
				hub := t.TempDir()
				if err := os.MkdirAll(configengine.ConfigDir(fabricengine.BoardDir(hub)), 0o755); err != nil {
					t.Fatalf("mkdir config dir: %v", err)
				}
				return hub
			},
			wantWarn: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs.Reset()
			hub := tt.hub(t)
			location := &lyxcwd.Location{RepoName: "example", HubPath: hub, WorktreeName: "hub-repo", AnchorRel: "."}
			c := &battenCLI{}
			if err := c.wire(location, slug); err != nil {
				t.Fatalf("wire() error = %v; want nil", err)
			}

			err := c.env.CreateWorktree(context.Background())

			if err == nil {
				t.Fatal("CreateWorktree() error = nil; want the absent fabric config to fail the row")
			}
			if tt.wantWarn {
				fabricConfig := configengine.ConfigFile(fabricengine.BoardDir(hub), "fabric")
				if !strings.Contains(err.Error(), fabricConfig) {
					t.Errorf("CreateWorktree() error = %q; want it to name %q, the row's own failure", err.Error(), fabricConfig)
				}
				if got := logs.String(); !strings.Contains(got, slug) || !strings.Contains(got, "claim board entry failed") {
					t.Errorf("log = %q; want a warning naming %q", got, slug)
				}
				return
			}
			board, openErr := boardengine.OpenHub(hub)
			if openErr != nil {
				t.Fatalf("OpenHub: %v", openErr)
			}
			task, found, getErr := board.GetTask(slug)
			if getErr != nil || !found || task.Status == nil || *task.Status != tt.wantStatus {
				t.Errorf("entry = %+v, found %v, err %v; want status %q", task, found, getErr, tt.wantStatus)
			}
		})
	}
}
