// poll_test.go covers the poll verb's classification wiring end to end:
// no-batch-in-flight refusal, a running snapshot at the wait deadline (no
// weft commit, no git diff), a done classification from an on-disk report
// (with a real diff/dirty computation against a scratch git repo), and a
// dead/asking classification derived purely from builderengine.TurnEnded/
// builderengine.StrandLive when no report has landed. Fakes for
// shuttleengine.Engine/MuxOps are package-local doubles mirroring
// builderengine's own poll_test.go fakeEngine/fakeMux.

package buildercli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/muxengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// pollFakeEngine is a minimal shuttleengine.Engine double for
// builderengine.TurnEnded: only ParseEvents is scripted, mirroring
// builderengine's own poll_test.go fakeEngine.
type pollFakeEngine struct {
	events []shuttleengine.Event
}

func (e *pollFakeEngine) Prepare(runDir string, spec shuttleengine.Spec, cfg shuttleengine.Config) (shuttleengine.Launch, error) {
	return shuttleengine.Launch{}, nil
}
func (e *pollFakeEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	return e.events, nil
}
func (e *pollFakeEngine) Startup(capture string) shuttleengine.StartupState {
	return shuttleengine.StartupPending
}
func (e *pollFakeEngine) InterruptSequence() []shuttleengine.PaneInput      { return nil }
func (e *pollFakeEngine) TrustDismissSequence() []shuttleengine.PaneInput   { return nil }
func (e *pollFakeEngine) ComposeSend(text string) []shuttleengine.PaneInput { return nil }

var _ shuttleengine.Engine = (*pollFakeEngine)(nil)

// pollFakeMux is a minimal shuttleengine.MuxOps double for
// builderengine.StrandLive: only Status is scripted, mirroring
// builderengine's own poll_test.go fakeMux.
type pollFakeMux struct {
	status muxengine.StatusResult
}

func (m *pollFakeMux) AddStrand(spec muxengine.AddSpec) (muxengine.Strand, error) {
	return muxengine.Strand{}, nil
}
func (m *pollFakeMux) RemoveStrand(guid string, recursive bool) (muxengine.Removed, error) {
	return muxengine.Removed{}, nil
}
func (m *pollFakeMux) Status() (muxengine.StatusResult, error)       { return m.status, nil }
func (m *pollFakeMux) SendText(guid, text string, submit bool) error { return nil }
func (m *pollFakeMux) SendKey(guid, key string) error                { return nil }
func (m *pollFakeMux) CapturePane(guid string) (string, error)       { return "", nil }

var _ shuttleengine.MuxOps = (*pollFakeMux)(nil)

// pollFixture is a fully-wired *builderCLI plus a scratch git repo standing
// in for the host worktree, with the plan-valid fixture seeded under its
// own _lyx/plan.
type pollFixture struct {
	CLI *builderCLI
	Hub string
}

func newPollFixture(t *testing.T, engine shuttleengine.Engine, mux shuttleengine.MuxOps) *pollFixture {
	t.Helper()

	hub := newScratchRepo(t)
	commitFile(t, hub, "base.txt", "base", "base commit")
	seedPlanFixture(t, hub, builderengineTestdataDir("plan-valid"))

	layout := &hubgeometry.Layout{WorktreeRoot: hub, Cwd: hub, RelPath: "."}

	c := &builderCLI{
		engine:     engine,
		mux:        mux,
		layout:     layout,
		cfg:        builderengine.Config{BatchTimeoutMin: 60, PollWaitS: 5},
		planDir:    hubgeometry.PlanDir(hub),
		builderDir: hubgeometry.BuilderDir(hub),
		reportsDir: hubgeometry.BuilderReportsDir(hub),
	}

	return &pollFixture{CLI: c, Hub: hub}
}

// seedInFlightBatch1 persists a state.json recording batch 1 as currently
// in flight, spawned startSHA at the given moment, with eventsPath and
// strandGuid pointing at fx's own hub tree so builderengine.TurnEnded/
// builderengine.StrandLive have somewhere real to look.
func (fx *pollFixture) seedInFlightBatch1(t *testing.T, startSHA string, spawnedAt time.Time, eventsPath string) {
	t.Helper()

	st := &builderengine.State{
		CurrentBatch: 1,
		Batches: map[int]*builderengine.BatchState{
			1: {
				Slug:       "json-flag",
				StartSHA:   startSHA,
				Role:       "implementer",
				StrandGUID: "strand-1",
				EventsPath: eventsPath,
				SpawnedAt:  spawnedAt.UTC().Format(time.RFC3339),
			},
		},
	}
	if err := builderengine.SaveState(fx.CLI.builderDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
}

func TestPollCmd_NoBatchInFlight(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newPollFixture(t, &pollFakeEngine{}, &pollFakeMux{})

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.pollCmd(), &out, nil)

	if exitCode != 1 {
		t.Fatalf("poll with no in-flight batch = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), "no batch in flight") {
		t.Errorf("output missing no-batch-in-flight message; got %q", out.String())
	}
}

func TestPollCmd_DeadlineReturnsRunningWithoutWeftCommit(t *testing.T) {
	fx := newPollFixture(t, &pollFakeEngine{}, &pollFakeMux{status: muxengine.StatusResult{
		Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: true}},
	}})
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	fx.seedInFlightBatch1(t, "irrelevant-sha", time.Now(), eventsPath)

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.pollCmd(), &out, []string{"--wait", "10ms"})

	if exitCode != 0 {
		t.Fatalf("poll --wait 10ms = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"status":"running"`) {
		t.Errorf("output missing status:running; got %q", got)
	}
	if !strings.Contains(got, `"batch":"01-json-flag"`) {
		t.Errorf("output missing batch identifier; got %q", got)
	}

	// A running snapshot must never weft-commit: state.json's Terminal
	// field stays false and no batch-boundary commit happens.
	loaded, err := builderengine.LoadState(fx.CLI.builderDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() error = %v, %v", loaded, err)
	}
	if loaded.Batches[1].Terminal {
		t.Errorf("Batches[1].Terminal = true after a running snapshot; want false")
	}
}

func TestPollCmd_ReportPresentClassifiesDoneAndCommits(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newPollFixture(t, &pollFakeEngine{}, &pollFakeMux{})

	startSHA := mustGit(t, fx.Hub, "rev-parse", "HEAD")
	startSHA = strings.TrimSpace(startSHA)
	// Advance HEAD past the recorded start SHA so ChangedFiles/Dirty have a
	// real (empty-diff) range to compute — the report-present branch must
	// run gitquery successfully even when nothing actually changed.
	commitFile(t, fx.Hub, "extra.txt", "extra", "extra commit")

	fx.seedInFlightBatch1(t, startSHA, time.Now(), filepath.Join(t.TempDir(), "events.jsonl"))

	if err := os.MkdirAll(fx.CLI.reportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	reportPath := filepath.Join(fx.CLI.reportsDir, "01-json-flag.yaml")
	if err := os.WriteFile(reportPath, []byte("batch: 01-json-flag\nstatus: done\ntests: green\nstuck_reason: null\n"), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.pollCmd(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("poll (report present) = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"status":"done"`) {
		t.Errorf("output missing status:done; got %q", got)
	}
	if !strings.Contains(got, `"tests":"green"`) {
		t.Errorf("output missing tests:green; got %q", got)
	}

	loaded, err := builderengine.LoadState(fx.CLI.builderDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() error = %v, %v", loaded, err)
	}
	if !loaded.Batches[1].Terminal || loaded.Batches[1].Status != "done" {
		t.Errorf("Batches[1] = %+v; want Terminal=true Status=done", loaded.Batches[1])
	}
}

func TestPollCmd_NoReportTurnEndedClassifiesDeadAsking(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newPollFixture(t, &pollFakeEngine{events: []shuttleengine.Event{{Kind: shuttleengine.EventStop, Message: "final"}}}, &pollFakeMux{
		status: muxengine.StatusResult{Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: true}}},
	})

	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(eventsPath, []byte("irrelevant; pollFakeEngine ignores bytes"), 0o644); err != nil {
		t.Fatalf("write events file: %v", err)
	}
	fx.seedInFlightBatch1(t, "irrelevant-sha", time.Now(), eventsPath)

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.pollCmd(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("poll (turn ended, no report) = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"status":"dead"`) || !strings.Contains(got, `"dead_reason":"asking"`) {
		t.Errorf("output missing dead/asking classification; got %q", got)
	}

	loaded, err := builderengine.LoadState(fx.CLI.builderDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() error = %v, %v", loaded, err)
	}
	if !loaded.Batches[1].Terminal || loaded.Batches[1].Status != "dead" {
		t.Errorf("Batches[1] = %+v; want Terminal=true Status=dead", loaded.Batches[1])
	}
}
