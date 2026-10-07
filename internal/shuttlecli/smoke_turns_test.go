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
	runner, _, reedEngine, _ := newSmokeRunner(t)
	return turnHub{runner: runner, reed: reedEngine, prime: h.PrimeWorktree()}
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
