//go:build llm

// smoke_turns_test.go provokes the hard turn-end cases against a REAL claude in a REAL tmux pane:
// a long background shell, a background shell finishing mid-turn, a Monitor, a fork, a message sent mid-turn, a skill load and a compact.
// Each test drives shuttleengine.Runner directly and reads the run's events file through claudeengine's ParseEvents while Wait runs,
// so a scenario whose agent has background work running checks that the Stop payload lists exactly what is running.
// A failing scenario keeps its events file and transcript in a directory under the OS temp dir that it logs,
// so promoting it to a claudeengine corpus case is a copy and a trim.

package shuttlecli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/llmkit"
)

// turnScenarioTimeout bounds each scenario's run and every wait on it.
const turnScenarioTimeout = 8 * time.Minute

// turnHub is a fixture hub with reed up and a runner over it, the substrate every turn scenario starts its run on.
type turnHub struct {
	runner *shuttleengine.Runner
	engine shuttleengine.Engine
	cfg    shuttleengine.Config
	reed   *reedengine.Engine
	prime  string
}

// newTurnHub forges a hub, chdirs into its prime worktree, brings reed up and builds a runner, tearing reed down at cleanup.
func newTurnHub(t *testing.T) turnHub {
	t.Helper()
	llmkit.Claude(t, "LYX_REED_CLAUDE")
	h := hubforge.NewHub(t, ".")
	deferHubRelease(t, h.Path)
	t.Chdir(h.PrimeWorktree())
	t.Cleanup(func() {
		var buf bytes.Buffer
		reedcli.RunCLI(&buf, []string{"down"})
	})
	var reedOut bytes.Buffer
	if code := reedcli.RunCLI(&reedOut, []string{"up"}); code != 0 {
		t.Fatalf("reed up = %d; want 0, output: %s", code, reedOut.String())
	}
	runner, engine, reedEngine, cfg := newSmokeRunner(t)
	return turnHub{runner: runner, engine: engine, cfg: cfg, reed: reedEngine, prime: h.PrimeWorktree()}
}

// turnWatch is one live run under observation: its Wait outcome, the last snapshot of its events file and the notices its runner sent.
type turnWatch struct {
	t      *testing.T
	run    *shuttleengine.Run
	waitCh chan waitOutcome
	parser *claudeengine.Claude

	mu      sync.Mutex
	notices int
	events  []byte
}

// startTurnScenario starts spec on hub's runner, runs Wait in a goroutine, and on a failing test keeps the events and transcript.
func startTurnScenario(t *testing.T, hub turnHub, spec shuttleengine.Spec) *turnWatch {
	t.Helper()
	w := &turnWatch{t: t, parser: claudeengine.New(), waitCh: make(chan waitOutcome, 1)}
	hub.runner.SetNotifier(func(string) error {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.notices++
		return nil
	})
	spec.Timeout = turnScenarioTimeout
	run, err := hub.runner.Start(spec)
	if err != nil {
		t.Fatalf("runner.Start: %v", err)
	}
	w.run = run
	t.Cleanup(w.keepOnFailure)
	go func() {
		result, waitErr := run.Wait()
		w.waitCh <- waitOutcome{result, waitErr}
	}()
	return w
}

// poll snapshots the run's events file; the run directory is removed when the run finishes done, so the last snapshot is the record.
func (w *turnWatch) poll() {
	data, err := os.ReadFile(filepath.Join(w.run.RunDir(), "events.jsonl"))
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = data
}

// turnEnds parses the last snapshot and returns its turn ends, waiting ones included, in file order.
func (w *turnWatch) turnEnds() []shuttleengine.Event {
	w.mu.Lock()
	data := w.events
	w.mu.Unlock()
	events, err := w.parser.ParseEvents(data)
	if err != nil {
		w.t.Fatalf("ParseEvents: %v", err)
	}
	var ends []shuttleengine.Event
	for _, ev := range events {
		if ev.Kind == shuttleengine.EventStop || ev.Kind == shuttleengine.EventWaiting {
			ends = append(ends, ev)
		}
	}
	return ends
}

// awaitTurnEnds polls until the events file holds at least n turn ends and returns them, failing if Wait returns or the scenario times out first.
func (w *turnWatch) awaitTurnEnds(n int) []shuttleengine.Event {
	w.t.Helper()
	deadline := time.Now().Add(turnScenarioTimeout)
	for {
		w.poll()
		if ends := w.turnEnds(); len(ends) >= n {
			return ends
		}
		select {
		case res := <-w.waitCh:
			w.t.Fatalf("run.Wait returned (outcome=%s err=%v) before turn end %d; want it still running", res.result.Outcome, res.err, n)
		default:
		}
		if time.Now().After(deadline) {
			w.t.Fatalf("no turn end %d within %s", n, turnScenarioTimeout)
		}
		time.Sleep(time.Second)
	}
}

// signals parses the last snapshot into session signals, in file order.
func (w *turnWatch) signals() []shuttleengine.SessionSignal {
	w.mu.Lock()
	data := w.events
	w.mu.Unlock()
	signals, _ := w.parser.ParseSessionSignals(data)
	return signals
}

// awaitSignal polls until the events file holds a signal of kind, failing if the scenario times out first.
// A run that ends first fails the scenario unless runMayEnd, since a session that exits ends its run as died;
// then the file is read once more, because its last line can land just before Wait returns.
func (w *turnWatch) awaitSignal(kind shuttleengine.SessionSignalKind, runMayEnd bool) {
	w.t.Helper()
	has := func() bool {
		for _, signal := range w.signals() {
			if signal.Kind == kind {
				return true
			}
		}
		return false
	}
	deadline := time.Now().Add(turnScenarioTimeout)
	for {
		w.poll()
		if has() {
			return
		}
		select {
		case res := <-w.waitCh:
			if runMayEnd {
				w.poll()
				if has() {
					return
				}
			}
			w.t.Fatalf("run.Wait returned (outcome=%s err=%v) before a %s signal; want it still running", res.result.Outcome, res.err, kind)
		default:
		}
		if time.Now().After(deadline) {
			w.t.Fatalf("no %s signal within %s", kind, turnScenarioTimeout)
		}
		time.Sleep(time.Second)
	}
}

// finishDone polls the events file until Wait returns and fails unless the run ended done.
func (w *turnWatch) finishDone() {
	w.t.Helper()
	deadline := time.After(turnScenarioTimeout)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case res := <-w.waitCh:
			if res.err != nil {
				w.t.Fatalf("run.Wait: %v", res.err)
			}
			if res.result.Outcome != shuttleengine.OutcomeDone {
				w.t.Fatalf("run.Wait outcome = %q; want %q", res.result.Outcome, shuttleengine.OutcomeDone)
			}
			return
		case <-tick.C:
			w.poll()
		case <-deadline:
			w.t.Fatalf("run.Wait did not return within %s", turnScenarioTimeout)
		}
	}
}

// noticeCount returns how many hold notices the runner sent.
func (w *turnWatch) noticeCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.notices
}

// keepOnFailure copies the last events snapshot and the transcript its Stop lines name into a new directory under the OS temp dir, and logs it.
// A passing scenario leaves nothing behind.
func (w *turnWatch) keepOnFailure() {
	if !w.t.Failed() {
		return
	}
	w.mu.Lock()
	data := w.events
	w.mu.Unlock()
	dir, err := os.MkdirTemp("", "lyx-turn-scenario-")
	if err != nil {
		w.t.Logf("keep evidence: %v", err)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), data, 0o644); err != nil {
		w.t.Logf("keep events: %v", err)
	}
	if ends := w.turnEnds(); len(ends) > 0 {
		var payload struct {
			TranscriptPath string `json:"transcript_path"`
		}
		if json.Unmarshal(ends[len(ends)-1].Raw, &payload) == nil && payload.TranscriptPath != "" {
			if transcript, err := os.ReadFile(payload.TranscriptPath); err == nil {
				_ = os.WriteFile(filepath.Join(dir, "transcript.jsonl"), transcript, 0o644)
			}
		}
	}
	w.t.Logf("kept the failing scenario's events and transcript in %s", dir)
}

// requireOutstanding fails unless turnEnd waits on exactly one task of kind, reported by the payload signal.
func requireOutstanding(t *testing.T, turnEnd shuttleengine.Event, kind shuttleengine.BackgroundKind) {
	t.Helper()
	if turnEnd.Kind != shuttleengine.EventWaiting || len(turnEnd.Outstanding) != 1 ||
		turnEnd.Outstanding[0].Kind != kind || turnEnd.Outstanding[0].Signal != shuttleengine.SignalPayload {
		t.Fatalf("turn end = kind %v, outstanding %+v; want waiting on exactly one %s reported by the %s signal", turnEnd.Kind, turnEnd.Outstanding, kind, shuttleengine.SignalPayload)
	}
}

// TestSmokeTurnsLongBackgroundShell pins that a turn end with a backgrounded shell still running reads waiting on that shell, by the payload,
// and that the run ends done once the shell finishes and the agent writes its output.
func TestSmokeTurnsLongBackgroundShell(t *testing.T) {
	hub := newTurnHub(t)
	output := filepath.Join(hub.prime, "turns-long-shell.txt")
	w := startTurnScenario(t, hub, shuttleengine.Spec{
		Prompt: fmt.Sprintf("Run the shell command `sleep 90` with the Bash tool's run_in_background option, then end your turn at once without writing any file. "+
			"When you are told the command finished, write exactly DONE to %s and end your turn.", output),
		OutputFiles: []string{output},
		Model:       smokeClaudeModel,
	})
	requireOutstanding(t, w.awaitTurnEnds(1)[0], shuttleengine.BackgroundShell)
	w.finishDone()
}

// TestSmokeTurnsShellFinishesMidTurn pins the issue 411 shape:
// a background shell that finishes while the turn still runs leaves the turn end with nothing outstanding, so it reads done on the tick that reads it.
func TestSmokeTurnsShellFinishesMidTurn(t *testing.T) {
	hub := newTurnHub(t)
	output := filepath.Join(hub.prime, "turns-shell-mid-turn.txt")
	w := startTurnScenario(t, hub, shuttleengine.Spec{
		Prompt: fmt.Sprintf("Do all of this in one turn, without ending it in between: run `sleep 5` with the Bash tool's run_in_background option, "+
			"then run `sleep 30` in the foreground, then write exactly DONE to %s, then end your turn.", output),
		OutputFiles: []string{output},
		Model:       smokeClaudeModel,
	})
	w.finishDone()
	ends := w.turnEnds()
	if len(ends) == 0 {
		t.Fatal("the events file holds no turn end; want the one that wrote the output")
	}
	if last := ends[len(ends)-1]; last.Kind != shuttleengine.EventStop || len(last.Outstanding) != 0 {
		t.Fatalf("last turn end = kind %v, outstanding %+v; want a Stop with nothing outstanding", last.Kind, last.Outstanding)
	}
}

// TestSmokeTurnsMonitor pins that a running Monitor is listed by the payload as an outstanding shell.
func TestSmokeTurnsMonitor(t *testing.T) {
	hub := newTurnHub(t)
	output := filepath.Join(hub.prime, "turns-monitor.txt")
	w := startTurnScenario(t, hub, shuttleengine.Spec{
		Prompt: fmt.Sprintf("Use the Monitor tool to watch the command `sleep 60; echo finished`, then end your turn at once without writing any file. "+
			"When the monitor reports finished, write exactly DONE to %s and end your turn.", output),
		OutputFiles: []string{output},
		Model:       smokeClaudeModel,
	})
	requireOutstanding(t, w.awaitTurnEnds(1)[0], shuttleengine.BackgroundShell)
	w.finishDone()
}

// TestSmokeTurnsFork pins that a working fork is listed by the payload as an outstanding fork, so the turn end reads waiting until it returns.
func TestSmokeTurnsFork(t *testing.T) {
	hub := newTurnHub(t)
	output := filepath.Join(hub.prime, "turns-fork.txt")
	w := startTurnScenario(t, hub, shuttleengine.Spec{
		Prompt: fmt.Sprintf("Launch one forked subagent with the Agent tool, subagent_type fork, in the background, whose whole task is to run `sleep 45` and reply OK. "+
			"Then end your turn at once without writing any file. When the fork returns, write exactly DONE to %s and end your turn.", output),
		OutputFiles:   []string{output},
		Model:         smokeClaudeModel,
		ForkSubagents: true,
	})
	requireOutstanding(t, w.awaitTurnEnds(1)[0], shuttleengine.BackgroundFork)
	w.finishDone()
}

// TestSmokeTurnsMessageMidTurn pins that a message sent with Run.Send while the agent works does not end the run: it still ends done.
func TestSmokeTurnsMessageMidTurn(t *testing.T) {
	hub := newTurnHub(t)
	output := filepath.Join(hub.prime, "turns-message.txt")
	w := startTurnScenario(t, hub, shuttleengine.Spec{
		Prompt:      fmt.Sprintf("Run `sleep 40` in the foreground, then write exactly DONE to %s, then end your turn.", output),
		OutputFiles: []string{output},
		Model:       smokeClaudeModel,
	})
	time.Sleep(15 * time.Second)
	if err := w.run.Send(shuttleengine.WithMessageTail("Say in one line what you are doing.")); err != nil {
		t.Fatalf("run.Send: %v", err)
	}
	w.finishDone()
}

// TestSmokeTurnsSkillLoad pins that the skill-load turn end is not read as the run's own: the run never holds on it and ends done on the prompt's turn.
func TestSmokeTurnsSkillLoad(t *testing.T) {
	hub := newTurnHub(t)
	output := filepath.Join(hub.prime, "turns-skill.txt")
	w := startTurnScenario(t, hub, shuttleengine.Spec{
		Prompt:      fmt.Sprintf("Write exactly DONE to %s, then end your turn.", output),
		OutputFiles: []string{output},
		Model:       smokeClaudeModel,
		Skills:      []string{"scribe:prose"},
	})
	w.finishDone()
	if got := w.noticeCount(); got != 0 {
		t.Fatalf("notifier called %d times; want 0, since the skill-load turn end belongs to Start", got)
	}
}

// TestSmokeTurnsCompact pins that compacting the live session while its turn end waits on a background shell does not end the run: it still ends done.
func TestSmokeTurnsCompact(t *testing.T) {
	hub := newTurnHub(t)
	output := filepath.Join(hub.prime, "turns-compact.txt")
	w := startTurnScenario(t, hub, shuttleengine.Spec{
		Prompt: fmt.Sprintf("Run the shell command `sleep 60` with the Bash tool's run_in_background option, then end your turn at once without writing any file. "+
			"When you are told the command finished, write exactly DONE to %s and end your turn.", output),
		OutputFiles: []string{output},
		Model:       smokeClaudeModel,
	})
	requireOutstanding(t, w.awaitTurnEnds(1)[0], shuttleengine.BackgroundShell)
	if err := hub.runner.CompactSession(w.run.StrandGUID(), ""); err != nil {
		t.Fatalf("CompactSession: %v", err)
	}
	w.finishDone()
}

// TestSmokeTurnsSessionHooks pins ParseSessionSignals over the hook payloads Claude Code really writes for a prompt, a turn end, an idle notice and a session end:
// an interactive run that replies without writing its output file holds, reads asking once the idle notice lands, and reads dead with the session end's reason after /exit.
// The fold is read directly for the last step, because the run's record stops reading running once its pane is gone.
func TestSmokeTurnsSessionHooks(t *testing.T) {
	hub := newTurnHub(t)
	output := filepath.Join(hub.prime, "turns-hooks.txt")
	w := startTurnScenario(t, hub, shuttleengine.Spec{
		Prompt:      "Reply with one short line saying READY, and write no file.",
		OutputFiles: []string{output},
		Model:       smokeClaudeModel,
		Interactive: true,
	})

	w.awaitSignal(shuttleengine.SessionSignalIdleNotice, false)
	readings, err := shuttleengine.ReadSessionStates(hub.cfg, hub.prime, hub.engine, time.Now())
	if err != nil {
		t.Fatalf("ReadSessionStates: %v", err)
	}
	var asking bool
	for _, reading := range readings {
		if reading.StrandGUID == w.run.StrandGUID() {
			asking = reading.State.Name == shuttleengine.SessionAsking
		}
	}
	if !asking {
		t.Fatalf("ReadSessionStates = %+v; want the run listed as %s", readings, shuttleengine.SessionAsking)
	}

	if err := hub.reed.SendText(w.run.StrandGUID(), "/exit", true); err != nil {
		t.Fatalf("send /exit: %v", err)
	}
	w.awaitSignal(shuttleengine.SessionSignalSessionEnd, true)

	w.mu.Lock()
	events := w.events
	w.mu.Unlock()
	stamped := map[string]bool{}
	for _, line := range bytes.Split(events, []byte("\n")) {
		var stamp struct {
			Hook string `json:"lyx_stamp"`
			At   string `json:"lyx_at"`
		}
		if json.Unmarshal(line, &stamp) != nil || stamp.Hook == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339, stamp.At); err != nil {
			t.Errorf("stamp for %s carries time %q; want a parseable RFC 3339 time", stamp.Hook, stamp.At)
			continue
		}
		stamped[stamp.Hook] = true
	}
	for _, hook := range []string{"UserPromptSubmit", "Stop", "Notification", "SessionEnd"} {
		if !stamped[hook] {
			t.Errorf("the events file holds no stamp for %s; stamped hooks: %v", hook, stamped)
		}
	}

	var turnStart, turnEnd, idleNotice, sessionEnd *shuttleengine.SessionSignal
	signals := w.signals()
	for i := range signals {
		signal := &signals[i]
		switch {
		case signal.Kind == shuttleengine.SessionSignalTurnStart && turnStart == nil:
			turnStart = signal
		case signal.Kind == shuttleengine.SessionSignalTurnEnd && turnEnd == nil:
			turnEnd = signal
		case signal.Kind == shuttleengine.SessionSignalIdleNotice && idleNotice == nil:
			idleNotice = signal
		case signal.Kind == shuttleengine.SessionSignalSessionEnd:
			sessionEnd = signal
		}
	}
	if turnStart == nil || turnEnd == nil || idleNotice == nil || sessionEnd == nil {
		t.Fatalf("signals = %+v; want a turn start, a turn end, an idle notice and a session end", signals)
	}
	if !sessionEnd.EndsProcess || sessionEnd.Reason == "" {
		t.Fatalf("session end = %+v; want a process-ending reason", sessionEnd)
	}

	var fold shuttleengine.SessionFold
	fold.Fold([]shuttleengine.SessionSignal{*turnStart, *turnEnd, *idleNotice, *sessionEnd}, shuttleengine.SessionFacts{
		Interactive: true,
		Liveness:    shuttleengine.LivenessAlive,
		ReadAt:      time.Now(),
	})
	history := fold.History()
	names := make([]shuttleengine.SessionStateName, len(history))
	for i, state := range history {
		names[i] = state.Name
	}
	want := []shuttleengine.SessionStateName{shuttleengine.SessionBusy, shuttleengine.SessionAsking, shuttleengine.SessionDead}
	if !slices.Equal(names, want) || history[0].Cause != shuttleengine.SessionCauseTurn || history[2].Cause != sessionEnd.Reason {
		t.Fatalf("history = %+v; want %v with the turn cause first and the session end's reason %q last", history, want, sessionEnd.Reason)
	}
}
