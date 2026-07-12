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
	"errors"
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
// builderengine.StrandLive and poll's terminal cleanup: Status is scripted,
// and RemoveStrand records every call so a test can assert whether the
// terminal branch released the strand.
type pollFakeMux struct {
	status         muxengine.StatusResult
	removedStrands []string
}

func (m *pollFakeMux) AddStrand(spec muxengine.AddSpec) (muxengine.Strand, error) {
	return muxengine.Strand{}, nil
}
func (m *pollFakeMux) RemoveStrand(guid string, recursive bool) (muxengine.Removed, error) {
	m.removedStrands = append(m.removedStrands, guid)
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
	fx.seedInFlightBatch1WithRunDir(t, startSHA, spawnedAt, eventsPath, filepath.Join(t.TempDir(), "run-1"))
}

// seedInFlightBatch1WithRunDir is seedInFlightBatch1 with a caller-chosen
// ShuttleRunDir, for the cleanup tests that assert whether poll's terminal
// branch removed or kept the run directory.
func (fx *pollFixture) seedInFlightBatch1WithRunDir(t *testing.T, startSHA string, spawnedAt time.Time, eventsPath, runDir string) {
	t.Helper()

	st := &builderengine.State{
		CurrentBatch: 1,
		Batches: map[int]*builderengine.BatchState{
			1: {
				Slug:          "json-flag",
				StartSHA:      startSHA,
				Role:          "implementer",
				StrandGUID:    "strand-1",
				ShuttleRunDir: runDir,
				EventsPath:    eventsPath,
				SpawnedAt:     spawnedAt.UTC().Format(time.RFC3339),
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
	// The digest contract pins a running snapshot to exactly batch, status,
	// and elapsed_s: files_changed/dirty are terminal, report-backed fields
	// nothing measured on this tick, and elapsed_s must be present even at
	// its zero first second.
	if strings.Contains(got, `"files_changed"`) || strings.Contains(got, `"dirty"`) {
		t.Errorf("running snapshot carries report-backed fields; got %q", got)
	}
	if !strings.Contains(got, `"elapsed_s"`) {
		t.Errorf("running snapshot missing elapsed_s; got %q", got)
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
	if loaded.CurrentBatch != 0 {
		t.Errorf("CurrentBatch = %d after a terminal classification; want 0 (state.go: 0 means none in flight)", loaded.CurrentBatch)
	}
}

// TestPollCmd_TerminalCleanupMatrix proves poll's terminal branch releases
// the batch's substrate exactly per the doc's discipline: done removes the
// strand AND the run dir (shuttle-finalize parity); stuck removes the
// strand but keeps the run dir (the raw session output is the stuck trail a
// human may still inspect); dead keeps both for diagnosis. Without the
// done/stuck cleanup every finished batch leaks a live pane hosting an idle
// agent process forever, since nobody else ever holds the shuttle Run
// handle (found live in round fable-r2: four leaked panes after two runs).
func TestPollCmd_TerminalCleanupMatrix(t *testing.T) {
	tests := []struct {
		name          string
		reportContent string
		events        []shuttleengine.Event
		wantRemoved   bool
		wantRunDir    bool
	}{
		{
			name:          "done removes strand and run dir",
			reportContent: "batch: 01-json-flag\nstatus: done\ntests: green\nstuck_reason: null\n",
			wantRemoved:   true,
			wantRunDir:    false,
		},
		{
			name:          "stuck removes strand but keeps run dir",
			reportContent: "batch: 01-json-flag\nstatus: stuck\ntests: red\nstuck_reason: \"blocked\"\n",
			wantRemoved:   true,
			wantRunDir:    true,
		},
		{
			name:        "dead asking keeps strand and run dir",
			events:      []shuttleengine.Event{{Kind: shuttleengine.EventStop, Message: "final"}},
			wantRemoved: false,
			wantRunDir:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("WEFT_SKIP_GIT", "1")
			mux := &pollFakeMux{status: muxengine.StatusResult{
				Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: true}},
			}}
			fx := newPollFixture(t, &pollFakeEngine{events: tt.events}, mux)

			startSHA := strings.TrimSpace(mustGit(t, fx.Hub, "rev-parse", "HEAD"))
			eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
			if err := os.WriteFile(eventsPath, []byte("irrelevant; pollFakeEngine ignores bytes"), 0o644); err != nil {
				t.Fatalf("write events file: %v", err)
			}
			runDir := filepath.Join(t.TempDir(), "run-1")
			if err := os.MkdirAll(runDir, 0o755); err != nil {
				t.Fatalf("mkdir run dir: %v", err)
			}
			fx.seedInFlightBatch1WithRunDir(t, startSHA, time.Now(), eventsPath, runDir)

			if tt.reportContent != "" {
				if err := os.MkdirAll(fx.CLI.reportsDir, 0o755); err != nil {
					t.Fatalf("mkdir reports dir: %v", err)
				}
				reportPath := filepath.Join(fx.CLI.reportsDir, "01-json-flag.yaml")
				if err := os.WriteFile(reportPath, []byte(tt.reportContent), 0o644); err != nil {
					t.Fatalf("write report: %v", err)
				}
			}

			var out bytes.Buffer
			if exitCode := clihelp.Execute(fx.CLI.pollCmd(), &out, nil); exitCode != 0 {
				t.Fatalf("poll = %d; want 0, output: %s", exitCode, out.String())
			}

			removed := len(mux.removedStrands) > 0
			if removed != tt.wantRemoved {
				t.Errorf("RemoveStrand calls = %v; wantRemoved = %v", mux.removedStrands, tt.wantRemoved)
			}
			if removed && mux.removedStrands[0] != "strand-1" {
				t.Errorf("RemoveStrand guid = %q; want strand-1", mux.removedStrands[0])
			}
			_, statErr := os.Stat(runDir)
			if tt.wantRunDir && statErr != nil {
				t.Errorf("run dir %s missing after poll (stat: %v); want it kept", runDir, statErr)
			}
			if !tt.wantRunDir && !os.IsNotExist(statErr) {
				t.Errorf("run dir %s still present after poll (stat: %v); want it removed", runDir, statErr)
			}
		})
	}
}

// pollRaceEngine forces the report-vs-Stop interleave: its ParseEvents
// writes the batch report to disk BEFORE returning a Stop event, modeling a
// report that lands between gather's first report stat and the (slower)
// events read. onParse runs once; later calls only return the Stop event.
type pollRaceEngine struct {
	pollFakeEngine
	onParse func()
	fired   bool
}

func (e *pollRaceEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	if !e.fired {
		e.fired = true
		e.onParse()
	}
	return []shuttleengine.Event{{Kind: shuttleengine.EventStop, Message: "final"}}, nil
}

// TestPollCmd_ReportLandingDuringGatherBeatsStopEvent proves the
// report-present branch wins FOR REAL, not just in decision order: a report
// written after gather's first stat but before its Stop-event read must
// still classify done — never dead/asking, which would wedge the
// orchestrator's next respawn on "batch report already exists" (found in
// round fable-r2; the implementer always writes its report before its turn
// ends, so this interleave is reachable on every stuck/done batch).
func TestPollCmd_ReportLandingDuringGatherBeatsStopEvent(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")

	var fx *pollFixture
	var reportPath string
	engine := &pollRaceEngine{onParse: func() {
		if err := os.MkdirAll(fx.CLI.reportsDir, 0o755); err != nil {
			t.Errorf("mkdir reports dir: %v", err)
		}
		if err := os.WriteFile(reportPath, []byte("batch: 01-json-flag\nstatus: done\ntests: green\nstuck_reason: null\n"), 0o644); err != nil {
			t.Errorf("write report mid-gather: %v", err)
		}
	}}
	fx = newPollFixture(t, engine, &pollFakeMux{
		status: muxengine.StatusResult{Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: true}}},
	})
	reportPath = filepath.Join(fx.CLI.reportsDir, "01-json-flag.yaml")

	startSHA := strings.TrimSpace(mustGit(t, fx.Hub, "rev-parse", "HEAD"))
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(eventsPath, []byte("irrelevant; pollRaceEngine ignores bytes"), 0o644); err != nil {
		t.Fatalf("write events file: %v", err)
	}
	fx.seedInFlightBatch1(t, startSHA, time.Now(), eventsPath)

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.pollCmd(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("poll (report landed mid-gather) = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"status":"done"`) {
		t.Errorf("output = %q; want status:done — the mid-gather report must win over the Stop event", got)
	}
	if strings.Contains(got, `"dead_reason"`) {
		t.Errorf("output = %q; want no dead_reason at all", got)
	}
}

// TestPollCmd_DeadRecheckStatErrorPropagates proves the dead-classification
// re-check's report-existence stat gets the same fail-loud treatment as
// gather's primary stat (round opus-r3's R2): the primary stat already
// propagated a non-ENOENT error as a poll-tick failure, but the re-check
// silently ignored one and let a dead classification stand -- exactly the
// false positive this re-check exists to prevent. A real filesystem race
// between the two stat calls cannot be scripted deterministically, so this
// test drives it via statReportPath, the package seam both calls go through.
func TestPollCmd_DeadRecheckStatErrorPropagates(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newPollFixture(t, &pollFakeEngine{events: []shuttleengine.Event{{Kind: shuttleengine.EventStop, Message: "final"}}}, &pollFakeMux{
		status: muxengine.StatusResult{Strands: []muxengine.StrandStatus{{GUID: "strand-1", Live: true}}},
	})

	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(eventsPath, []byte("irrelevant; pollFakeEngine ignores bytes"), 0o644); err != nil {
		t.Fatalf("write events file: %v", err)
	}
	fx.seedInFlightBatch1(t, "irrelevant-sha", time.Now(), eventsPath)

	// Call 1 (gather's primary stat) reports the report genuinely absent,
	// driving the TurnEnded/StrandLive path to a dead classification; call 2
	// (the pre-dead-classification re-check) then hits a distinct,
	// non-ENOENT error -- e.g. a permission error a real stat call could
	// surface, which the primary stat already treats as a hard failure.
	wantErr := errors.New("boom: stat failed transiently")
	calls := 0
	origStat := statReportPath
	statReportPath = func(name string) (os.FileInfo, error) {
		calls++
		if calls == 1 {
			return nil, os.ErrNotExist
		}
		return nil, wantErr
	}
	t.Cleanup(func() { statReportPath = origStat })

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.pollCmd(), &out, nil)

	if exitCode == 0 {
		t.Fatalf("poll (re-check stat error) = 0; want a non-zero exit surfacing the stat error, output: %s", out.String())
	}
	got := out.String()
	if !strings.Contains(got, wantErr.Error()) {
		t.Errorf("output = %q; want it to surface the re-check's stat error %q", got, wantErr.Error())
	}
	if strings.Contains(got, `"status":"dead"`) {
		t.Errorf("output = %q; want no dead classification -- the re-check's stat error must propagate, never be silently swallowed", got)
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
	// A dead digest is terminal but not report-backed: files_changed/dirty
	// were never measured, so the envelope must not assert their zeros.
	if strings.Contains(got, `"files_changed"`) || strings.Contains(got, `"dirty"`) {
		t.Errorf("dead digest carries report-backed fields; got %q", got)
	}

	loaded, err := builderengine.LoadState(fx.CLI.builderDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() error = %v, %v", loaded, err)
	}
	if !loaded.Batches[1].Terminal || loaded.Batches[1].Status != "dead" {
		t.Errorf("Batches[1] = %+v; want Terminal=true Status=dead", loaded.Batches[1])
	}
	if loaded.CurrentBatch != 0 {
		t.Errorf("CurrentBatch = %d after a terminal classification; want 0 (state.go: 0 means none in flight)", loaded.CurrentBatch)
	}
}

// TestPollCmd_TerminalPersistMergesConcurrentSpawn proves the terminal
// persist writes onto a FRESH state loaded under the state-mutation lease,
// never the copy loaded at poll entry: a spawn-batch landing inside the
// long-poll's window (here scripted via the statReportPath seam, firing
// between poll's entry-time LoadState and its terminal write) records a new
// batch and moves CurrentBatch, and saving poll's stale entry-time copy
// would erase both — a live implementer with no state record. The
// classified batch must still be marked terminal, and the concurrently
// spawned batch's record and cursor must survive.
func TestPollCmd_TerminalPersistMergesConcurrentSpawn(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	fx := newPollFixture(t, &pollFakeEngine{}, &pollFakeMux{})

	startSHA := strings.TrimSpace(mustGit(t, fx.Hub, "rev-parse", "HEAD"))
	commitFile(t, fx.Hub, "extra.txt", "extra", "extra commit")

	fx.seedInFlightBatch1(t, startSHA, time.Now(), filepath.Join(t.TempDir(), "events.jsonl"))

	if err := os.MkdirAll(fx.CLI.reportsDir, 0o755); err != nil {
		t.Fatalf("mkdir reports dir: %v", err)
	}
	reportPath := filepath.Join(fx.CLI.reportsDir, "01-json-flag.yaml")
	if err := os.WriteFile(reportPath, []byte("batch: 01-json-flag\nstatus: done\ntests: green\nstuck_reason: null\n"), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	// Script the concurrent spawn: on gather's FIRST report stat — strictly
	// after poll's entry-time LoadState, strictly before its terminal write —
	// record batch 2 as freshly in flight, exactly as a concurrent
	// spawn-batch's own SaveState would.
	injected := false
	origStat := statReportPath
	statReportPath = func(name string) (os.FileInfo, error) {
		if !injected {
			injected = true
			st, err := builderengine.LoadState(fx.CLI.builderDir)
			if err != nil || st == nil {
				t.Errorf("LoadState() inside gather = %v, %v; want the seeded state", st, err)
			} else {
				st.Batches[2] = &builderengine.BatchState{
					Slug:       "list-tests",
					StrandGUID: "concurrent-strand-2",
					SpawnedAt:  time.Now().UTC().Format(time.RFC3339),
				}
				st.CurrentBatch = 2
				if err := builderengine.SaveState(fx.CLI.builderDir, st); err != nil {
					t.Errorf("SaveState() inside gather = %v", err)
				}
			}
		}
		return origStat(name)
	}
	t.Cleanup(func() { statReportPath = origStat })

	var out bytes.Buffer
	exitCode := clihelp.Execute(fx.CLI.pollCmd(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("poll (report present) = %d; want 0, output: %s", exitCode, out.String())
	}

	loaded, err := builderengine.LoadState(fx.CLI.builderDir)
	if err != nil || loaded == nil {
		t.Fatalf("LoadState() error = %v, %v", loaded, err)
	}
	if !loaded.Batches[1].Terminal || loaded.Batches[1].Status != "done" {
		t.Errorf("Batches[1] = %+v; want Terminal=true Status=done", loaded.Batches[1])
	}
	if loaded.Batches[2] == nil || loaded.Batches[2].StrandGUID != "concurrent-strand-2" {
		t.Errorf("Batches[2] = %+v; want the concurrently spawned record to survive poll's terminal persist", loaded.Batches[2])
	}
	if loaded.CurrentBatch != 2 {
		t.Errorf("CurrentBatch = %d; want 2 (the concurrently spawned batch's cursor, not this poll's to reset)", loaded.CurrentBatch)
	}
}
