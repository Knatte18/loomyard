//go:build tier0

package battencli

import (
	"context"
	"errors"
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
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/parentlock"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/envkit"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

// intentionallyNil maps each shedrecipe.Env field path wire leaves nil to the reason.
// envkit.NilSeams already skips the seams whose nil is a documented default, so only wire's own gaps are listed.
// batten wires only its own producers, so every loom seam of shedrecipe.Env stays nil here.
var intentionallyNil = map[string]string{
	"VerifyCommand":               "loom-only seam, batten runs no verify gate",
	"VerifyHold":                  "loom-only seam, batten runs no verify gate",
	"GateFindingsFile":            "loom-only seam, batten has no gated row that keeps findings",
	"Shuttle":                     "loom-only seam, batten drives no agent itself",
	"Burler":                      "loom-only seam, batten runs no review round",
	"BeamDescribe.Entry":          "loom-only seam, batten runs no beam description row",
	"BeamDescribe.CommitSubjects": "loom-only seam, batten runs no beam description row",
	"BeamDescribe.Commit":         "loom-only seam, batten runs no beam description row",
	"Pawl.SetStatus":              "loom-only seam, batten runs no pawl row",
	"Pawl.EntryProducer":          "loom-only seam, batten runs no pawl row",
	"Pawl.OwnedElsewhere":         "loom-only seam, batten runs no pawl row",
	"Pawl.MarkBeam":               "loom-only seam, batten runs no pawl row",
	"Pawl.SpawnPush":              "loom-only seam, batten runs no pawl row",
	"Pawl.RegenerateWorkspace":    "loom-only seam, batten runs no pawl row",
	"Pawl.StopDobby":              "loom-only seam, batten runs no pawl row",
	"Pawl.StartDobby":             "loom-only seam, batten runs no pawl row",
	"Pawl.DobbyLive":              "loom-only seam, batten runs no pawl row",
	"Pawl.Recheck":                "loom-only seam, batten runs no pawl row",
	"Pawl.WriteSnapshot":          "loom-only seam, batten runs no pawl row",
	"Pawl.Notify":                 "loom-only seam, batten runs no pawl row",
	"Pawl.Down":                   "loom-only seam, batten runs no pawl row",
	"Pawl.Now":                    "loom-only seam, batten runs no pawl row",
	"Pawl.Sleep":                  "loom-only seam, batten runs no pawl row",
	"Seats":                       "loom-only seam, batten runs no seat table",
	"Models":                      "loom-only seam, batten resolves no seat model",
	"WebsterRun":                  "loom-only seam, batten runs no webster",
	"WebsterDeps.Starter":         "loom-only seam, batten runs no webster",
	"WebsterDeps.Stopper":         "loom-only seam, batten runs no webster",
	"WebsterDeps.Engines":         "loom-only seam, batten runs no webster",
	"WebsterDeps.RefMatcher":      "loom-only seam, batten runs no webster",
	"WebsterDeps.ReadOnly":        "loom-only seam, batten runs no webster",
	"WebsterDeps.Geom.Index":      "loom-only seam, batten runs no webster",
	"WebsterDeps.Geom.VerifyHold": "loom-only seam, batten runs no webster",
	"PlanIndex":                   "loom-only seam, batten has no plan segment",
	"CommitWebster":               "loom-only seam, batten runs no webster",
	"Landing.BaseCommand":         "loom-only seam, batten lands no branch",
	"Landing.VerifyHold":          "loom-only seam, batten lands no branch",
	"Landing.GateTiers":           "loom-only seam, batten lands no branch",
	"Landing.OpenFabric":          "loom-only seam, batten lands no branch",
	"Landing.OpenParentFabric":    "loom-only seam, batten lands no branch",
	"Landing.TaskHead":            "loom-only seam, batten lands no branch",
	"Landing.Shuttle":             "loom-only seam, batten lands no branch",
	"Landing.EnginesRun":          "loom-only seam, batten lands no branch",
	"Landing.Lock.Store":          "loom-only seam, batten lands no branch",
	"Landing.Lock.Owner":          "loom-only seam, batten lands no branch",
	"Landing.Lock.PauseRequested": "loom-only seam, batten lands no branch",
	"Landing.ReadParentTips":      "loom-only seam, batten lands no branch",
	"Landing.HoldsWork":           "loom-only seam, batten lands no branch",
	"Landing.ChangedPaths":        "loom-only seam, batten lands no branch",
	"Landing.MergeBase":           "loom-only seam, batten lands no branch",
	"Landing.FixerSeats":          "loom-only seam, batten lands no branch",
	"Landing.LineChange":          "loom-only seam, batten lands no branch",
	"Landing.IsTestFile":          "loom-only seam, batten lands no branch",
	"Landing.RowEntry":            "loom-only seam, batten lands no branch",
	"Landing.TaskRemoteTip":       "loom-only seam, batten lands no branch",
	"Landing.CodeReviewOff":       "loom-only seam, batten lands no branch",
	"Landing.RerouteReviewed":     "loom-only seam, batten lands no branch",
	"DiscussionTable":             "loom-only seam, batten has no discussion segment",
	"CommitDiscussion":            "loom-only seam, batten has no discussion segment",
	"DescribeTable":               "loom-only seam, batten has no describe segment",
	"CommitDescription":           "loom-only seam, batten has no describe segment",
	"PlanTable":                   "loom-only seam, batten has no plan segment",
	"CommitPlan":                  "loom-only seam, batten has no plan segment",
	"ApprovePlan":                 "loom-only seam, batten has no plan segment",
	"SkipPlanReview":              "loom-only seam, batten has no plan segment",
	"CarryOver":                   "loom-only seam, batten runs no review segment",
	"ReflectFriction":             "loom-only seam, batten has no reflect segment",
	"Rework.ReadCommitted":        "loom-only seam, batten has no rework segment",
	"Rework.ArchiveWebster":       "loom-only seam, batten has no rework segment",
	"DarnTable":                   "darn-only seam, batten has no darn segment",
	"Darn.Commit":                 "darn-only seam, batten has no darn segment",
	"Darn.LatestOutcome":          "darn-only seam, batten has no darn segment",
	"Drift.Config":                "loom- and darn-only seam, batten has no Drift row",
	"Drift.Status":                "loom- and darn-only seam, batten has no Drift row",
	"Drift.Head":                  "loom- and darn-only seam, batten has no Drift row",
	"Drift.BranchPoint":           "loom- and darn-only seam, batten has no Drift row",
	"Drift.DirtyPaths":            "loom- and darn-only seam, batten has no Drift row",
	"Drift.Commit":                "loom- and darn-only seam, batten has no Drift row",
	"Drift.BaselineTable":         "loom- and darn-only seam, batten has no Drift row",
	"Drift.StartCapture":          "loom- and darn-only seam, batten has no Drift row",
	"Drift.RunCapture":            "loom- and darn-only seam, batten has no Drift row",
	"Drift.Sleep":                 "loom- and darn-only seam, batten has no Drift row",
	"Drift.VerifyTable":           "loom- and darn-only seam, batten has no Drift row",
	"Drift.Compare":               "loom- and darn-only seam, batten has no Drift row",
}

//lym:seam internal/battenshed#InnerRunDeps
//lym:seam internal/battenshed#LandDeps
//lym:seam internal/battenshed#PrimeLock
//lym:seam internal/battenshed#RowNoticeDeps
//lym:seam internal/battenshed#SeedChildDeps
//lym:seam internal/battenshed#TeardownDeps
//lym:seam internal/configengine#Dirs
//lym:seam internal/driftengine#Deps
//lym:seam internal/driftengine#Paths
//lym:seam internal/landingshed#BeamDescribeDeps
//lym:seam internal/landingshed#Config
//lym:seam internal/landingshed#Deps
//lym:seam internal/landingshed#ParentLockDeps
//lym:seam internal/loomshed#DarnDeps
//lym:seam internal/loomshed#PRReworkDeps
//lym:seam internal/pawlengine#Deps
//lym:seam internal/pawlengine#Paths
//lym:seam internal/shedadapters#CommandDeps
//lym:seam internal/shedrecipe#Env
//lym:seam internal/shuttleengine#Config
//lym:seam internal/websterengine#Config
//lym:seam internal/websterengine#Geometry
//lym:seam internal/websterengine#RunDeps
func TestWire_EverySeamFilled(t *testing.T) {
	t.Parallel()

	location := &lymcwd.Location{
		RepoName:     "example",
		HubPath:      t.TempDir(),
		WorktreeName: "hub-repo",
		AnchorRel:    ".",
	}

	c := &battenCLI{}
	if err := c.wire(location, "a-slug-with-no-worktree-anywhere"); err != nil {
		t.Fatalf("wire() error = %v; want nil", err)
	}

	// envkit lists these three as legal nils, so wire's filling them is asserted here.
	if c.env.InnerRun.Sleeps == nil || c.env.InnerRun.Awake == nil || c.env.InnerRun.ReviveAPIError == nil {
		t.Errorf("InnerRun Sleeps, Awake and ReviveAPIError set = %t, %t, %t; want all set after wire()", c.env.InnerRun.Sleeps != nil, c.env.InnerRun.Awake != nil, c.env.InnerRun.ReviveAPIError != nil)
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

	escalated := shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Finalize", Error: landingshed.EscalateStopPrefix + "the fixer escalated"}
	if !c.env.InnerRun.HandBack(escalated) {
		t.Errorf("InnerRun.HandBack(%+v) = false; want true, so the binary re-step leaves a loom or darn escalate halt alone", escalated)
	}
}

// TestRestepSkips pins which child statuses the binary re-step leaves alone: a blocked landing, decision or escalate halt, and nothing else.
func TestRestepSkips(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status shedengine.Status
		want   bool
	}{
		{"LandingHaltAtFinalize", shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Finalize", Error: landingshed.LandingStopPrefix + "over the caps"}, true},
		{"DecisionHalt", shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Discussion-Bouncer", Error: shedadapters.DecisionHaltPrefix + "a held accept"}, true},
		{"EscalateHaltAtFinalize", shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Finalize", Error: landingshed.EscalateStopPrefix + "a conflict"}, true},
		{"EscalateHaltAtParentAlign", shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Parent-Align", Error: landingshed.EscalateStopPrefix + "a conflict"}, true},
		{"EscalateReasonOnAFailedStatus", shedengine.Status{State: shedengine.StateFailed, CurrentProducer: "Finalize", Error: landingshed.EscalateStopPrefix + "a conflict"}, false},
		{"EscalateReasonAtDescribe", shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Describe", Error: landingshed.EscalateStopPrefix + "a conflict"}, false},
		{"PlainHalt", shedengine.Status{State: shedengine.StateBlocked, CurrentProducer: "Finalize", Error: "dirty parent"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := restepSkips(tt.status); got != tt.want {
				t.Errorf("restepSkips(%+v) = %v; want %v", tt.status, got, tt.want)
			}
		})
	}
}

// wiredPrime wires a battenCLI over a prime location rooted in a temp hub with no task worktree.
func wiredPrime(t *testing.T) (*battenCLI, *lymcwd.Location) {
	t.Helper()
	location := &lymcwd.Location{
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

// TestWire_PointsTheNetworkGateAtItsSlug installs a gate with no slug over a record holding a live entry for the wired slug, and asserts a network call made after wire waits.
// It is not parallel because the network gate is process-wide.
func TestWire_PointsTheNetworkGateAtItsSlug(t *testing.T) {
	recordPath := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
	now := time.Now()
	if _, err := pauserecord.Update(recordPath, now, func(record *pauserecord.Record) error {
		record.Live = append(record.Live, pauserecord.Entry{Scope: "a-slug", PausedAt: now})
		return nil
	}); err != nil {
		t.Fatalf("pauserecord.Update: %v", err)
	}
	slept := false
	t.Cleanup(pauserecord.InstallGate(pauserecord.Gate{
		Geometry: pauserecord.Geometry{RecordPath: recordPath},
		Notify:   func(string) {},
		Sleep: func(context.Context, time.Duration) error {
			slept = true
			return errors.New("stop waiting")
		},
	}))

	wiredPrime(t)
	_, done := pauserecord.Network("git fetch")
	done()

	if !slept {
		t.Error("a network call after wire did not wait; want the gate pointed at the wired slug's entry")
	}
}

func TestWire_PairDirRefusesAnAbsentTaskWorktreeByName(t *testing.T) {
	t.Parallel()

	c, _ := wiredPrime(t)

	_, err := c.env.InnerRun.PairDir()
	if err == nil || !strings.Contains(err.Error(), "a-slug") {
		t.Errorf("PairDir() error = %v; want the absent-worktree refusal naming the slug", err)
	}
}

// TestWire_BattenStatusSeamsReadBattensOwnStatus asserts the wired seams that read batten's own status file agree on one fixture:
// the pause seams of Run-Shed and Land report the pause_requested flag and false while the file is absent, and the row notices' history seam reports the history length and 0 while the file is absent.
func TestWire_BattenStatusSeamsReadBattensOwnStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      *shedengine.Status
		want        bool
		wantHistory int
	}{
		{name: "AbsentStatusIsNotPaused"},
		{name: "FlagClear", status: &shedengine.Status{State: shedengine.StateRunning}},
		{name: "FlagSet", status: &shedengine.Status{State: shedengine.StateRunning, PauseRequested: true}, want: true},
		{name: "HistoryLength", status: &shedengine.Status{State: shedengine.StateRunning, History: []shedengine.HistoryEntry{{Producer: "Worktree-Create"}, {Producer: "Seed-Child"}}}, wantHistory: 2},
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
			landPaused, err := c.env.Land.PauseRequested()
			if err != nil || landPaused != tt.want {
				t.Errorf("Land.PauseRequested() = %v, %v; want %v, nil", landPaused, err, tt.want)
			}
			history, err := c.env.Land.Notices.BattenHistoryLen()
			if err != nil || history != tt.wantHistory {
				t.Errorf("BattenHistoryLen() = %d, %v; want %d, nil", history, err, tt.wantHistory)
			}
		})
	}
}

// TestWire_MarkerAndReportSeamsRefuseAnAbsentTaskWorktree asserts the watched-marker seams, the stop-report seam and the terminal window seam return an error, rather than reporting not held or not found, when the task worktree their files live in is absent.
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
	if reading, err := c.env.InnerRun.ArbiterStop(context.Background(), shedengine.Status{}); err == nil || reading.Eligible || !strings.Contains(err.Error(), "a-slug") {
		t.Errorf("ArbiterStop() eligible=%v, error=%v; want the absent-worktree refusal naming the slug", reading.Eligible, err)
	}

	// Every Command row reads its switch from the task worktree when it is reached, so the lookup refuses before any command runs.
	if value, err := c.env.Setting("batten", "open_window"); err == nil || value != nil || !strings.Contains(err.Error(), "a-slug") {
		t.Errorf("Setting(batten, open_window) = %v, %v; want the absent-worktree refusal naming the slug", value, err)
	}
}

// TestReadAgentActivity_ReadsLiveRunsAndTheWaitMarker asserts the Activity seam's reading maps a task worktree's live shuttle run onto battenshed's type, with the session state of its stamped turn end, skips a run whose record reads stopped, and reports a live wait marker, a live parent lock wait record or a child running at Beam-Await, ignoring a lock wait record whose writer is dead.
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

	taskLocation := &lymcwd.Location{RepoName: "example", HubPath: t.TempDir(), WorktreeName: "task", AnchorRel: "."}
	runsRoot := filepath.Join(taskLocation.AnchorPath(), ".lym", "shuttle")
	writeRun := func(name string, pid int, strand, outcome string) string {
		dir := filepath.Join(runsRoot, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		record := fmt.Sprintf(`{"runId":%q,"strandName":%q,"outcome":%q,"pid":%d,"eventsPath":%q,"createdAt":"2026-01-01T09:00:00Z"}`, name, strand, outcome, pid, filepath.Join(dir, "events"))
		if err := os.WriteFile(filepath.Join(dir, "run.json"), []byte(record), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	livePID := os.Getpid()

	liveDir := writeRun("live", livePID, "hub:task:impl", "running")
	activeAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	eventsPath := filepath.Join(liveDir, "events")
	stampedStop := fmt.Sprintf(`{"lym_stamp":"Stop","lym_at":%q}`+"\n"+`{"hook_event_name":"Stop","session_id":"sess-1"}`+"\n", activeAt.Format(time.RFC3339))
	if err := os.WriteFile(eventsPath, []byte(stampedStop), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(eventsPath, activeAt, activeAt); err != nil {
		t.Fatal(err)
	}
	writeRun("ended", 1<<30, "hub:task:review", "stopped")

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

	if err := os.Remove(filepath.Join(liveDir, "wait.yaml")); err != nil {
		t.Fatal(err)
	}
	verifyMarkerPath := verifytree.NewPaths(taskLocation.WorktreePath(), verifytree.Dir(taskLocation.AnchorPath())).Marker
	if err := os.MkdirAll(filepath.Dir(verifyMarkerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		state    string
		wantLive bool
	}{
		{verifytree.MarkerStateRunning, true},
		{verifytree.MarkerStatePaused, false},
	} {
		marker := fmt.Sprintf("site: Publish\nattempt: 0\ncommand: go test ./...\nstarted: 2026-01-01T10:00:00Z\npid: %d\nstate: %s\n", livePID, step.state)
		if err := os.WriteFile(verifyMarkerPath, []byte(marker), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, waitLive, err := readAgentActivity(taskLocation); err != nil || waitLive != step.wantLive {
			t.Errorf("readAgentActivity() waitLive = %v, error = %v with a live %s verify marker; want %v, nil", waitLive, err, step.state, step.wantLive)
		}
	}
	if err := os.Remove(verifyMarkerPath); err != nil {
		t.Fatal(err)
	}
	scratch := loomengine.LoomScratchDir(taskLocation)
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	lockWaitPath := parentlock.WaitRecordPath(scratch)
	deadWriter := parentlock.Wait{Branch: "main", Holder: "other-task", PID: 1 << 30, StartTime: "gone", Started: activeAt}
	if err := parentlock.WriteWait(lockWaitPath, deadWriter); err != nil {
		t.Fatal(err)
	}
	if _, waitLive, err := readAgentActivity(taskLocation); err != nil || waitLive {
		t.Errorf("readAgentActivity() waitLive = %v, error = %v with a lock wait record of a dead writer; want false, nil", waitLive, err)
	}
	if err := parentlock.WriteWait(lockWaitPath, parentlock.NewWait("main", "other-task")); err != nil {
		t.Fatal(err)
	}
	if _, waitLive, err := readAgentActivity(taskLocation); err != nil || !waitLive {
		t.Errorf("readAgentActivity() waitLive = %v, error = %v with a live lock wait record; want true, nil", waitLive, err)
	}

	if err := os.Remove(lockWaitPath); err != nil {
		t.Fatal(err)
	}
	childStatusPath := shedrun.StatusFile(taskLocation, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(childStatusPath), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		name     string
		status   string
		wantLive bool
	}{
		{"RunningAtBeamAwaitWithNoAgentIsAWait", `{"current_producer":"Beam-Await","state":"running"}`, true},
		{"RunningAtParentAlignIsNoWait", `{"current_producer":"Parent-Align","state":"running"}`, false},
		{"BlockedAtBeamAwaitIsNoWait", `{"current_producer":"Beam-Await","state":"blocked"}`, false},
		{"UnreadableStatusIsNoWait", `{"current_producer":`, false},
	} {
		if err := os.WriteFile(childStatusPath, []byte(step.status), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, waitLive, err := readAgentActivity(taskLocation); err != nil || waitLive != step.wantLive {
			t.Errorf("readAgentActivity() waitLive = %v, error = %v for %s; want %v, nil", waitLive, err, step.name, step.wantLive)
		}
	}
}

// TestWire_CreateWorktreeClaimsTheBoardEntry asserts the Worktree-Create closure claims the board entry for this machine before it touches the pair:
// a setter error and an entry another machine owns each fail the row naming their cause, an absent entry passes the claim with nothing written, and the real setter records the claim status and no owner.
// Every row that passes the claim fails the closure on the absent fabric config, so no pair is created.
// It sets BOARD_SKIP_GIT and BOARD_SKIP_PUSH, process-global, so it does not call t.Parallel.
func TestWire_CreateWorktreeClaimsTheBoardEntry(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	t.Setenv("BOARD_SKIP_PUSH", "1")

	const slug = "claimed"
	const machine = "here"
	seededHub := func(t *testing.T) string {
		hub := boardkit.HubWithBoardConfig(t)
		board, err := boardengine.OpenHub(hub)
		if err != nil {
			t.Fatalf("OpenHub: %v", err)
		}
		if _, err := board.UpsertTask(map[string]any{"slug": slug, "kind": "task", "labels": []string{"bug"}}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		return hub
	}
	tests := []struct {
		name string
		// beam is the beam project the task is a member of on the owner seams, which a board write cannot seed; empty reads a root task.
		beam string
		// set replaces the owner setter; nil keeps the real one over the hub's board.
		set func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error)
		// landing replaces the beam landing seams; nil keeps the real ones, which read the seeded task.
		landing *landingSeams
		// wantErr lists substrings the row's error carries; an empty list wants the absent fabric config, the row's own failure.
		wantErr []string
		// wantStatus is the seeded entry's status after the call, whose recorded owner stays empty; empty skips the read.
		wantStatus string
	}{
		{
			name: "SetterErrorFailsTheRow",
			set: func(boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
				return boardengine.OwnerWrite{}, errors.New("push refused")
			},
			wantErr: []string{"push refused", slug},
		},
		{
			name: "EntryOwnedElsewhereFailsTheRow",
			set: func(boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
				return boardengine.OwnerWrite{Lost: map[string]string{slug: "there"}}, nil
			},
			wantErr: []string{`"there"`, "`lym adopt root`"},
		},
		{
			name: "EntryOwnedElsewhereInABeamNamesTheBeam",
			beam: "some-beam",
			set: func(boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
				return boardengine.OwnerWrite{Lost: map[string]string{slug: "there"}}, nil
			},
			wantErr: []string{`"there"`, "`lym adopt some-beam`"},
		},
		{
			name: "AbsentEntryPassesTheClaimWithNothingWritten",
			set: func(req boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
				if req.Machine != machine || len(req.Slugs) != 1 || req.Slugs[0] != slug || req.Claim != "running · Worktree-Create" || req.Takeover {
					return boardengine.OwnerWrite{}, fmt.Errorf("claim request = %+v; want %q's Worktree-Create claim for %q with no takeover", req, machine, slug)
				}
				return boardengine.OwnerWrite{}, nil
			},
		},
		{
			name: "BeamProjectWithNoPairReachesTheCreateAndNotAPrecondition",
			set: func(boardengine.OwnerRequest) (boardengine.OwnerWrite, error) {
				return boardengine.OwnerWrite{}, nil
			},
			landing: &landingSeams{
				Project: func() (bool, error) { return true, nil },
			},
		},
		{
			name:       "RealSetterRecordsTheClaimStatusAlone",
			wantStatus: "running · Worktree-Create",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := seededHub(t)
			location := &lymcwd.Location{RepoName: "example", HubPath: hub, WorktreeName: "hub-repo", AnchorRel: "."}
			c := &battenCLI{}
			if err := c.wire(location, slug); err != nil {
				t.Fatalf("wire() error = %v; want nil", err)
			}
			c.owner = hubOwnerSeams(location)
			c.owner.Machine = func() (string, error) { return machine, nil }
			if tt.beam != "" {
				c.owner.Read = func() (boardengine.Owners, error) {
					return boardengine.Owners{Entries: map[string]boardengine.OwnerEntry{
						tt.beam: {IsProject: true, Beam: true},
						slug:    {IsTask: true},
					}}, nil
				}
				c.owner.ParentOf = func(string) (string, error) { return tt.beam, nil }
			}
			if tt.set != nil {
				c.owner.Set = tt.set
			}
			if tt.landing != nil {
				c.beam = *tt.landing
			}

			err := c.env.CreateWorktree(context.Background())

			if err == nil {
				t.Fatal("CreateWorktree() error = nil; want a failed row")
			}
			wantErr := tt.wantErr
			if len(wantErr) == 0 {
				wantErr = []string{configengine.ConfigFile(fabricengine.BoardDir(hub), "fabric")}
			}
			for _, want := range wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("CreateWorktree() error = %q; want it to carry %q", err.Error(), want)
				}
			}
			if tt.wantStatus == "" {
				return
			}
			board, openErr := boardengine.OpenHub(hub)
			if openErr != nil {
				t.Fatalf("OpenHub: %v", openErr)
			}
			task, found, getErr := board.GetTask(slug)
			if getErr != nil || !found || task.Status == nil || *task.Status != tt.wantStatus || task.OwnerMachine != "" {
				t.Errorf("entry = %+v, found %v, err %v; want status %q and no recorded owner", task, found, getErr, tt.wantStatus)
			}
		})
	}
}
