// dobbydeps_integration_test.go pins the seams hubDobbyDeps fills over a hubforge hub.
// It covers the seams that differ between a beam's dobby and the prime's, and the seams it fills for both.

package beamcli

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hibernatecli"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/pawlengine"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/boardkit"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestHubDobbyDeps_PrimeTidyShuttle(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task1")
	prime := h.Location

	beamGeom, err := hubgeom.DobbyGeometry(prime, "task1")
	if err != nil {
		t.Fatalf("DobbyGeometry(task1) = %v; want nil", err)
	}
	beamDeps, err := hubDobbyDeps(prime, beamGeom)
	if err != nil {
		t.Fatalf("hubDobbyDeps(beam) = %v; want nil", err)
	}
	stopper, ok := beamDeps.PrimeTidyShuttle.(*runnerTidyShuttle)
	if !ok || stopper.worktree != prime {
		t.Errorf("beam PrimeTidyShuttle = %#v; want a *runnerTidyShuttle over the prime's location", beamDeps.PrimeTidyShuttle)
	}

	primeGeom, err := hubgeom.DobbyGeometry(prime, prime.WorktreeName)
	if err != nil {
		t.Fatalf("DobbyGeometry(prime) = %v; want nil", err)
	}
	primeDeps, err := hubDobbyDeps(prime, primeGeom)
	if err != nil {
		t.Fatalf("hubDobbyDeps(prime) = %v; want nil", err)
	}
	if primeDeps.PrimeTidyShuttle != nil {
		t.Errorf("prime PrimeTidyShuttle = %#v; want nil", primeDeps.PrimeTidyShuttle)
	}
	// The watch opens only a directory that exists, and a tick creates it as a side effect.
	// So the check comes before any tick.
	if info, err := os.Stat(filepath.Dir(hibernatecli.RequestPath(prime))); err != nil || !info.IsDir() {
		t.Errorf("the request's directory after the prime's wiring: stat = %v; want an existing directory", err)
	}

	// Only the prime's dobby carries the hibernate request; with no request, its tick takes nothing up.
	if beamDeps.Tick != nil {
		t.Errorf("beam Tick is set; want nil, since no beam's dobby carries the hibernate request")
	}
	if primeDeps.Tick == nil {
		t.Fatal("prime Tick = nil; want the hibernate carrier's tick")
	}
	if inFlight, err := primeDeps.Tick(t.Context(), dobbyengine.TickInput{}); inFlight || err != nil {
		t.Errorf("prime Tick() with no request = %v, %v; want false, nil", inFlight, err)
	}

	// The seam reads the host's real cgroup, so it is checked for being filled and never called.
	if beamDeps.TaskLimit == nil {
		t.Errorf("beam TaskLimit = nil; want the task-limit reader")
	}
	if primeDeps.TaskLimit == nil {
		t.Errorf("prime TaskLimit = nil; want the task-limit reader")
	}

	// Both dobbies send the run notices.
	// They use the batten recipe's quiet window and the marker in the run's records directory.
	for name, notices := range map[string]dobbyengine.RunNoticeDeps{"beam": beamDeps.RunNotices, "prime": primeDeps.RunNotices} {
		if notices.ReadChild == nil || notices.DriverAlive == nil || notices.Agents == nil || notices.NewestAgentEnd == nil || notices.Pause == nil || notices.ReviveAPIError == nil || notices.MarkerPath == nil {
			t.Errorf("%s RunNotices = %+v; want every seam filled", name, notices)
			continue
		}
		if notices.Quiet != 30*time.Minute {
			t.Errorf("%s RunNotices.Quiet = %s; want the Run-Shed row's 30m", name, notices.Quiet)
		}
		if got, want := notices.MarkerPath("task1"), shedrun.NoticeEpisodeFile(prime, "task1"); got != want {
			t.Errorf("%s RunNotices.MarkerPath(task1) = %q; want %q", name, got, want)
		}
	}
}

// TestHubDobbyDeps_PrimeTickEnsuresRelay pins that the prime's tick starts the Slack relay.
// It does so while the presence is slack and the relay is down.
// It starts nothing in the states `lym slack stop` and a gave-up relay leave.
// It redirects the credential directory with t.Setenv, which is process-global state, so it does not run in parallel.
func TestHubDobbyDeps_PrimeTickEnsuresRelay(t *testing.T) {
	tiergate.Require(t, 1)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task1")
	prime := h.Location
	geom, err := hubgeom.SlackGeometry(prime.HubPath)
	if err != nil {
		t.Fatalf("SlackGeometry = %v; want nil", err)
	}
	if err := slackengine.WriteCredentials(geom.TokenPath, slackengine.Credentials{BotToken: "xoxb-test", AppToken: "xapp-test", AppID: "A1"}); err != nil {
		t.Fatalf("WriteCredentials = %v", err)
	}
	if err := slackengine.WriteSetup(geom.SetupPath, slackengine.Setup{ChannelID: "C1", OperatorID: "U1"}); err != nil {
		t.Fatalf("WriteSetup = %v", err)
	}
	setPresence := func(presence slackengine.Presence) {
		t.Helper()
		if _, err := slackengine.WritePresence(geom.PresencePath, presence, time.Now()); err != nil {
			t.Fatalf("WritePresence(%s) = %v", presence, err)
		}
	}

	primeGeom, err := hubgeom.DobbyGeometry(prime, prime.WorktreeName)
	if err != nil {
		t.Fatalf("DobbyGeometry(prime) = %v; want nil", err)
	}
	primeDeps, err := hubDobbyDeps(prime, primeGeom)
	if err != nil {
		t.Fatalf("hubDobbyDeps(prime) = %v; want nil", err)
	}

	setPresence(slackengine.PresenceSlack)
	if inFlight, err := primeDeps.Tick(t.Context(), dobbyengine.TickInput{}); inFlight || !errors.Is(err, slackengine.ErrSpawnUnderTest) {
		t.Errorf("prime Tick() with presence slack and no relay = %v, %v; want false and an error matching ErrSpawnUnderTest, which shows the tick reached the spawn", inFlight, err)
	}

	// A relay that died young five starts in a row is not started again, and that is no error of the tick.
	if err := os.WriteFile(geom.RelayStatePath, []byte(`{"start_failures":5}`), 0o600); err != nil {
		t.Fatalf("WriteFile(relay state) = %v", err)
	}
	if inFlight, err := primeDeps.Tick(t.Context(), dobbyengine.TickInput{}); inFlight || err != nil {
		t.Errorf("prime Tick() with a relay that gave up = %v, %v; want false, nil", inFlight, err)
	}

	setPresence(slackengine.PresenceTerminal)
	if inFlight, err := primeDeps.Tick(t.Context(), dobbyengine.TickInput{}); inFlight || err != nil {
		t.Errorf("prime Tick() with presence terminal = %v, %v; want false, nil", inFlight, err)
	}

	beamGeom, err := hubgeom.DobbyGeometry(prime, "task1")
	if err != nil {
		t.Fatalf("DobbyGeometry(task1) = %v; want nil", err)
	}
	beamDeps, err := hubDobbyDeps(prime, beamGeom)
	if err != nil {
		t.Fatalf("hubDobbyDeps(beam) = %v; want nil", err)
	}
	if beamDeps.Tick != nil {
		t.Error("beam Tick is set; want nil, since no beam's dobby starts the relay")
	}
}

// TestHubDobbyDeps_WaitReminders pins that every dobby gets the wait reminder seams.
// It also pins what they read over a hub: the reminded rows, the presence, the template's interval,
// and the pause record's latest resume.
func TestHubDobbyDeps_WaitReminders(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task1")
	prime := h.Location
	primeGeom, err := hubgeom.DobbyGeometry(prime, prime.WorktreeName)
	if err != nil {
		t.Fatalf("DobbyGeometry(prime) = %v; want nil", err)
	}
	primeDeps, err := hubDobbyDeps(prime, primeGeom)
	if err != nil {
		t.Fatalf("hubDobbyDeps(prime) = %v; want nil", err)
	}
	beamGeom, err := hubgeom.DobbyGeometry(prime, "task1")
	if err != nil {
		t.Fatalf("DobbyGeometry(task1) = %v; want nil", err)
	}
	beamDeps, err := hubDobbyDeps(prime, beamGeom)
	if err != nil {
		t.Fatalf("hubDobbyDeps(beam) = %v; want nil", err)
	}
	if beamDeps.WaitReminders.Post == nil || primeDeps.WaitReminders.Post == nil {
		t.Error("WaitReminders.Post is nil on a dobby; want it set on the prime's and a beam's")
	}

	reminders := primeDeps.WaitReminders
	if want := []string{loomshed.NamePRReview, loomshed.NameSamplerSession}; !slices.Equal(reminders.Rows, want) {
		t.Errorf("Rows = %q; want %q", reminders.Rows, want)
	}
	if interval, err := reminders.Interval(); interval != 30*time.Minute || err != nil {
		t.Errorf("Interval() = %v, %v; want the template's 30m and nil", interval, err)
	}

	if away, err := reminders.Away(); away || err != nil {
		t.Errorf("Away() with no presence record = %v, %v; want false, nil", away, err)
	}
	slack, err := hubgeom.SlackGeometry(prime.HubPath)
	if err != nil {
		t.Fatalf("SlackGeometry = %v; want nil", err)
	}
	if _, err := slackengine.WritePresence(slack.PresencePath, slackengine.PresenceSlack, time.Now()); err != nil {
		t.Fatalf("WritePresence(slack) = %v", err)
	}
	if away, err := reminders.Away(); !away || err != nil {
		t.Errorf("Away() with presence slack = %v, %v; want true, nil", away, err)
	}

	if resumed, err := reminders.LatestResume("task1"); !resumed.IsZero() || err != nil {
		t.Errorf("LatestResume() with no pause record = %v, %v; want zero, nil", resumed, err)
	}
	pause, err := hubgeom.PauseGeometry(prime)
	if err != nil {
		t.Fatalf("PauseGeometry = %v; want nil", err)
	}
	resumedAt := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	if _, err := pauserecord.Update(pause.RecordPath, resumedAt, func(record *pauserecord.Record) error {
		record.Resumed = append(record.Resumed, pauserecord.Entry{Scope: "task1", PausedAt: resumedAt.Add(-time.Minute), ResumedAt: resumedAt})
		return nil
	}); err != nil {
		t.Fatalf("Update() = %v; want nil", err)
	}
	if resumed, err := reminders.LatestResume("task1"); !resumed.Equal(resumedAt) || err != nil {
		t.Errorf("LatestResume() = %v, %v; want %v, nil", resumed, err, resumedAt)
	}
}

// startCreelDaemon runs creel's daemon in process for the hub over a fake delivery.
// The delivery holds receiver as a live strand.
// A client built over the hub then connects to it and spawns no daemon of its own.
func startCreelDaemon(t *testing.T, h *hubforge.Hub, receiver string) {
	t.Helper()
	root, err := lymtemp.ProcessRoot()
	if err != nil {
		t.Fatalf("ProcessRoot() = %v; want nil", err)
	}
	geometry, err := hubgeom.CreelGeometry(h.Path, root.Path())
	if err != nil {
		t.Fatalf("CreelGeometry() = %v; want nil", err)
	}
	delivery := creelfake.NewDelivery()
	delivery.SetStrands(creelfake.LiveStrand(receiver))
	epoch := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- creelengine.Run(ctx, creelengine.Options{
			Geometry: geometry, Delivery: delivery, Clock: creelfake.NewClock(epoch), PollClock: creelfake.NewClock(epoch),
			ParentPID: proc.ParentPID, Ready: func() { close(ready) },
		})
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("the creel daemon ended before it listened: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the creel daemon did not listen")
	}
}

// TestHubDobbyDeps_WaitReminderPostReachesTheSlackRelaySignedAsTheDobby pins the Slack post of the wait reminder.
// Each dobby's line is posted to the hub's Slack relay, signed DOBBY in the dobby's own scope.
func TestHubDobbyDeps_WaitReminderPostReachesTheSlackRelaySignedAsTheDobby(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "task1")
	prime := h.Location
	slack, err := hubgeom.SlackGeometry(prime.HubPath)
	if err != nil {
		t.Fatalf("SlackGeometry = %v; want nil", err)
	}
	relayName := slack.Shortname + ":" + slackengine.SlackProgram
	startCreelDaemon(t, h, relayName)

	tests := []struct {
		name string
		slug string
		line string
	}{
		{"Prime", "", "prime: waiting at PR-Review for 30 minutes"},
		{"Beam", "task1", "task1: waiting at PR-Review for 30 minutes"},
	}
	for _, tt := range tests {
		geom, err := hubgeom.DobbyGeometry(prime, cmp.Or(tt.slug, prime.WorktreeName))
		if err != nil {
			t.Fatalf("%s: DobbyGeometry = %v; want nil", tt.name, err)
		}
		deps, err := hubDobbyDeps(prime, geom)
		if err != nil {
			t.Fatalf("%s: hubDobbyDeps = %v; want nil", tt.name, err)
		}
		if err := deps.WaitReminders.Post(t.Context(), tt.line); err != nil {
			t.Fatalf("%s: WaitReminders.Post = %v; want nil", tt.name, err)
		}
	}

	reader, err := creelcli.NewProgramClient(prime, "CHECK")
	if err != nil {
		t.Fatalf("NewProgramClient(CHECK) = %v; want nil", err)
	}
	entries, err := reader.Log(creelengine.LogFilter{})
	if err != nil {
		t.Fatalf("Log() = %v; want nil", err)
	}
	type mail struct{ from, to, body string }
	var got []mail
	for _, entry := range entries {
		if entry.Event == "posted" {
			got = append(got, mail{entry.From, entry.To, entry.Subject})
		}
	}
	want := []mail{
		{slack.Shortname + ":DOBBY", relayName, tests[0].line},
		{slack.Shortname + ":task1:DOBBY", relayName, tests[1].line},
	}
	if !slices.Equal(got, want) {
		t.Errorf("the posted mails = %q; want %q", got, want)
	}
}

// landingHub returns a hubforge hub holding the beam b, owned by this machine, whose pawl run waits at Beam-Await.
// It also returns the seam set of b's dobby over that hub.
func landingHub(t *testing.T) (*hubforge.Hub, dobbyengine.Deps) {
	t.Helper()
	boardkit.InstallReplay(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPairWith(t, h, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})

	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
	if err != nil {
		t.Fatalf("load board config: %v", err)
	}
	cfg.Path = h.BoardDir()
	if _, err := boardengine.New(cfg).UpsertTask(map[string]any{"slug": "b", "kind": "project", "title": "b", "labels": []string{"bug"}}); err != nil {
		t.Fatalf("seed the board entry b: %v", err)
	}
	machine, err := machinename.Name()
	if err != nil {
		t.Fatalf("machine name: %v", err)
	}
	if _, err := boardengine.SetOwners(h.BoardDir(), boardengine.OwnerRequest{Machine: machine, Slugs: []string{"b"}, Beam: true}); err != nil {
		t.Fatalf("flag b as a beam: %v", err)
	}

	beam, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(h.Location, "b"))
	if err != nil {
		t.Fatalf("resolve the beam worktree: %v", err)
	}
	if err := shedrun.WriteSeed(beam, shedrun.SelfRunID, shedrun.Seed{Recipe: "pawl", Driver: shedrun.DriverGo}); err != nil {
		t.Fatalf("write the pawl run's seed: %v", err)
	}
	lockPath := shedrun.StatusLock(beam, shedrun.SelfRunID)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("create the status lock directory: %v", err)
	}
	waiting := shedengine.Status{State: shedengine.StateRunning, CurrentProducer: pawlengine.NameBeamAwait}
	if err := state.WriteJSON(shedrun.StatusFile(beam, shedrun.SelfRunID), lockPath, waiting); err != nil {
		t.Fatalf("write the pawl run's status: %v", err)
	}

	geom, err := hubgeom.DobbyGeometry(h.Location, "b")
	if err != nil {
		t.Fatalf("DobbyGeometry(b) = %v; want nil", err)
	}
	deps, err := hubDobbyDeps(h.Location, geom)
	if err != nil {
		t.Fatalf("hubDobbyDeps(b) = %v; want nil", err)
	}
	return h, deps
}

func TestHubDobbyDeps_LandsOnlyAFlaggedBeam(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, deps := landingHub(t)
	beam, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(h.Location, "b"))
	if err != nil {
		t.Fatalf("resolve the beam worktree: %v", err)
	}
	requestPath := hubgeom.PawlPaths(beam).LandRequest

	reading, err := deps.CanLand()
	if err != nil || reading.Passes {
		t.Fatalf("CanLand() = %+v, %v for an unflagged beam; want a reading that does not pass", reading, err)
	}
	err = deps.Land()
	if err == nil || !strings.Contains(err.Error(), "lym beam ready b") {
		t.Fatalf("Land() = %v for an unflagged beam; want a refusal naming `lym beam ready b`", err)
	}
	if _, statErr := os.Stat(requestPath); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("land request after a refused Land: stat = %v; want none written", statErr)
	}

	// The fixture's beam is not at a judged Beam-Await, so the record is written below `markReady`'s judgment.
	if _, err := hubLandDeps(h.Location).MarkReady("b", pawlengine.ReadyRecord{Slug: "b", SetAt: time.Now().UTC()}); err != nil {
		t.Fatalf("MarkReady(b) = %v; want nil", err)
	}
	reading, err = deps.CanLand()
	if err != nil || !reading.Passes || reading.ParentOwner != "" {
		t.Fatalf("CanLand() = %+v, %v for a flagged beam; want a passing reading with no foreign parent owner", reading, err)
	}
	if err := deps.Land(); err != nil {
		t.Fatalf("Land() = %v for a flagged beam; want nil", err)
	}
	if _, statErr := os.Stat(requestPath); statErr != nil {
		t.Errorf("land request after Land: stat = %v; want the request `lym beam land` writes", statErr)
	}
}

func TestHubLandDeps_MarkReadyCommitsAnUncommittedFlaggedRecord(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, _ := landingHub(t)
	beam, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(h.Location, "b"))
	if err != nil {
		t.Fatalf("resolve the beam worktree: %v", err)
	}
	path, lockPath := loomengine.BeamReadyPath(beam), loomengine.BeamReadyLockPath(beam)
	flaggedAt := time.Date(2026, 3, 1, 9, 30, 0, 0, time.UTC)
	if err := pawlengine.WriteReady(path, lockPath, pawlengine.ReadyRecord{Slug: "b", SetAt: flaggedAt}); err != nil {
		t.Fatalf("write the ready record: %v", err)
	}

	result, err := markReady(hubLandDeps(h.Location), "b")
	if err != nil || result["already"] != true {
		t.Fatalf("markReady(b) = %v, %v for a flagged beam; want already true", result, err)
	}

	dirty, err := exec.Command("git", "-C", filepath.Dir(path), "status", "--porcelain", "--", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("git status of the ready record: %v: %s", err, dirty)
	}
	if len(strings.TrimSpace(string(dirty))) != 0 {
		t.Errorf("git status of the ready record = %q; want it committed", dirty)
	}
	record, found, err := state.ReadJSON[pawlengine.ReadyRecord](path, lockPath)
	if err != nil || !found || !record.SetAt.Equal(flaggedAt) {
		t.Errorf("ready record = %+v, %v, %v; want the original, not rewritten", record, found, err)
	}
}

func TestHubLandDeps_FileTaskReleasesTheTaskAndCodeReviewCapsReadTheHubConfig(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, _ := landingHub(t)
	deps := hubLandDeps(h.Location)
	// Distinct values for the code perch, so a read of another perch or of another config source fails the caps assertion.
	loomPath := filepath.Join(h.Location.AnchorPath(), lymdirs.LymDirName, "config", "loom.yaml")
	if err := os.MkdirAll(filepath.Dir(loomPath), 0o755); err != nil {
		t.Fatalf("create the anchor config directory: %v", err)
	}
	if err := os.WriteFile(loomPath, []byte("code_max_bounces: 7\ncode_hard_max_bounces: 9\n"), 0o644); err != nil {
		t.Fatalf("write the anchor loom.yaml: %v", err)
	}

	err := deps.FileTask(map[string]any{"slug": "b-merge-in", "title": "Merge main into the beam b", "kind": boardengine.KindTask, "project": "b", "recipe": "darn", "labels": []string{"bug"}})
	if err != nil {
		t.Fatalf("FileTask(b-merge-in) = %v; want nil", err)
	}

	tasks, err := deps.Tasks()
	if err != nil {
		t.Fatalf("Tasks() = %v; want nil", err)
	}
	var filed *boardengine.Task
	for i := range tasks {
		if tasks[i].Slug == "b-merge-in" {
			filed = &tasks[i]
		}
	}
	if filed == nil {
		t.Fatalf("Tasks() = %v; want the filed task b-merge-in", tasks)
	}
	if filed.Status != nil {
		t.Errorf("b-merge-in status = %q; want it released (nil), not draft", *filed.Status)
	}

	caps, err := deps.CodeReviewCaps()
	if err != nil {
		t.Fatalf("CodeReviewCaps() = %v; want nil", err)
	}
	if caps.Soft != 7 || caps.Hard != 9 {
		t.Errorf("CodeReviewCaps() = %+v; want soft 7 and hard 9, the code perch's caps", caps)
	}
}

func TestHubDobbyDeps_ClearNotifyOnLandClearsTheHubBoardEntry(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h, deps := landingHub(t)
	cfg, err := boardengine.LoadConfig(configengine.Dirs{Board: h.BoardDir()}, "board")
	if err != nil {
		t.Fatalf("load board config: %v", err)
	}
	cfg.Path = h.BoardDir()
	board := boardengine.New(cfg)
	if _, err := board.UpsertTask(map[string]any{"slug": "b", "notify_on_land": true}); err != nil {
		t.Fatalf("set notify_on_land on b: %v", err)
	}

	if err := deps.ClearNotifyOnLand("b"); err != nil {
		t.Fatalf("ClearNotifyOnLand(b) = %v; want nil", err)
	}

	task, found, err := board.GetTask("b")
	if err != nil || !found {
		t.Fatalf("GetTask(b) = found %v, err %v; want found", found, err)
	}
	if task.NotifyOnLand {
		t.Errorf("b.NotifyOnLand = true; want it cleared")
	}
}
