// corpus_test.go replays the corpus of real turn ends under testdata/corpus through ParseEvents, shuttle's wait loop and the session-state fold.
// Each case directory holds a trimmed transcript, the run's events file and a case.yaml naming the reading each turn end must get and the session state each signal-bearing line reaches.

package claudeengine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

// corpusDir holds one directory per corpus case.
var corpusDir = filepath.Join("testdata", "corpus")

// Readings a corpus turn end can expect.
const (
	readingDone    = "done"
	readingHeld    = "held"
	readingWaiting = "waiting"
)

// corpusCase is the part of a case.yaml the replay reads: whether the run is gated, the skills Start loads, each later turn end's expected reading,
// and the session state after each events line the signal parser reads.
type corpusCase struct {
	Gated    bool            `yaml:"gated"`
	Skills   []string        `yaml:"skills"`
	TurnEnds []corpusTurnEnd `yaml:"turn_ends"`
	States   []corpusState   `yaml:"states"`
}

// corpusState is the session state and cause the fold reads after one events line, with an API error's text as detail.
type corpusState struct {
	State  string `yaml:"state"`
	Cause  string `yaml:"cause"`
	Detail string `yaml:"detail"`
}

// corpusTurnEnd is one turn end's expected reading, whether every output file exists when it is written, and, for a waiting one, its outstanding tasks.
type corpusTurnEnd struct {
	Reading      string       `yaml:"reading"`
	OutputsExist bool         `yaml:"outputs_exist"`
	Tasks        []corpusTask `yaml:"tasks"`
}

// corpusTask is one outstanding task a waiting turn end must report.
type corpusTask struct {
	Kind   shuttleengine.BackgroundKind `yaml:"kind"`
	ID     string                       `yaml:"id"`
	Signal string                       `yaml:"signal"`
}

// corpusReplay feeds a case's events to a running Run, one line per poll tick, and checks each read turn end's effect on the next tick's sleep.
// Wait calls Sleep from its own goroutine, the test's, so the replay needs no locking.
type corpusReplay struct {
	t        *testing.T
	name     string
	turnEnds []corpusTurnEnd
	lines    []string
	// turnEndOf is, per line, the index of the turn end the line carries, or -1 for a line ParseEvents reads no event from.
	turnEndOf  []int
	outputFile string
	eventsPath string
	now        time.Time
	runTimeout time.Duration
	appended   int
	notices    int
	noticesAt  int
	finished   bool
}

func (r *corpusReplay) Now() time.Time { return r.now }

// Sleep advances the clock, checks the turn end the run read since the last sleep when the line it read carries one, and appends the next line.
// Past the last line it moves the clock beyond the run deadline, so Wait ends.
func (r *corpusReplay) Sleep(d time.Duration) {
	r.now = r.now.Add(d)
	if r.finished {
		return
	}
	if r.appended > 0 && r.turnEndOf[r.appended-1] >= 0 {
		r.checkRead(r.turnEndOf[r.appended-1])
	}
	if r.appended == len(r.lines) {
		r.finished = true
		r.now = r.now.Add(2 * r.runTimeout)
		return
	}
	if index := r.turnEndOf[r.appended]; index >= 0 && r.turnEnds[index].OutputsExist {
		if err := os.WriteFile(r.outputFile, []byte("output\n"), 0o644); err != nil {
			r.t.Fatalf("%s: write output file: %v", r.name, err)
		}
	}
	f, err := os.OpenFile(r.eventsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		r.t.Fatalf("%s: open events file: %v", r.name, err)
	}
	defer f.Close()
	if _, err := f.WriteString(r.lines[r.appended] + "\n"); err != nil {
		r.t.Fatalf("%s: append turn end: %v", r.name, err)
	}
	r.appended++
	r.noticesAt = r.notices
}

// checkRead checks the effect of turn end index, which the run read on the tick before this sleep, so the run did not finalize on it.
func (r *corpusReplay) checkRead(index int) {
	switch notified := r.notices - r.noticesAt; r.turnEnds[index].Reading {
	case readingDone:
		r.t.Errorf("%s: turn end %d reads done, but the run did not finalize on the tick that read it", r.name, index+1)
	case readingHeld:
		if notified != 1 {
			r.t.Errorf("%s: turn end %d reads held: the notifier was called %d times, want 1", r.name, index+1, notified)
		}
	case readingWaiting:
		if notified != 0 {
			r.t.Errorf("%s: turn end %d reads waiting: the notifier was called %d times, want 0", r.name, index+1, notified)
		}
	}
}

func TestCorpus_ReplaysEachTurnEnd(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	cases := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cases++
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			replayCorpusCase(t, name)
		})
	}
	if cases == 0 {
		t.Fatalf("%s holds no case directory", corpusDir)
	}
}

// replayCorpusCase starts a run over the case's events, with the real Claude parser behind a fake engine and a fake reed,
// checks each turn end's parse and each signal-bearing line's session state, and checks that Wait ends Done on a closing done turn end and times out otherwise.
func replayCorpusCase(t *testing.T, name string) {
	dir := filepath.Join(corpusDir, name)
	c := loadCorpusCase(t, dir)
	lines := loadCorpusEvents(t, dir)
	consumed := 0
	if len(c.Skills) > 0 {
		consumed = 1
	}
	for i, te := range c.TurnEnds {
		if te.Reading == readingDone && i != len(c.TurnEnds)-1 {
			t.Fatalf("%s: turn end %d reads done, so the run ends there; case.yaml names turn ends after it", name, i+1)
		}
	}

	claude := claudeengine.New()
	turnEndOf := make([]int, len(lines))
	turnEnds := 0
	for i := range lines {
		turnEndOf[i] = -1
		if i < consumed {
			continue
		}
		if events, err := claude.ParseEvents([]byte(lines[i])); err != nil || len(events) > 1 {
			t.Fatalf("%s: line %d: ParseEvents = %v, %v; want at most one event", name, i+1, events, err)
		} else if len(events) == 1 {
			if turnEnds == len(c.TurnEnds) {
				t.Fatalf("%s: events.jsonl holds more turn ends than the %d case.yaml names", name, len(c.TurnEnds))
			}
			checkCorpusParse(t, claude, name, turnEnds+1, lines[i], c.TurnEnds[turnEnds])
			turnEndOf[i] = turnEnds
			turnEnds++
		}
	}
	if turnEnds != len(c.TurnEnds) {
		t.Fatalf("%s: events.jsonl holds %d turn ends after the %d line(s) Start consumes; case.yaml names %d", name, turnEnds, consumed, len(c.TurnEnds))
	}
	checkCorpusStates(t, claude, name, c, lines, turnEndOf, consumed)

	worktree := t.TempDir()
	replay := &corpusReplay{
		t:          t,
		name:       name,
		turnEnds:   c.TurnEnds,
		lines:      lines,
		turnEndOf:  turnEndOf,
		outputFile: filepath.Join(worktree, "output.md"),
		now:        time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC),
		runTimeout: time.Hour,
	}

	var pane strings.Builder
	reed := &shuttlefake.Reed{
		SendTextFn: func(_, text string, _ bool) error {
			pane.WriteString(text + "\n")
			return nil
		},
		CapturePaneFn: func(string) (string, error) { return pane.String(), nil },
	}
	launch := shuttleengine.Launch{Cmd: "replay", SessionID: "replay"}
	if len(c.Skills) > 0 {
		launch.PromptLine = "Read the prompt."
	}
	engine := &shuttlefake.Engine{
		PrepareFn: func(runDir string, _ shuttleengine.Spec, _ shuttleengine.Config) (shuttleengine.Launch, error) {
			replay.eventsPath = filepath.Join(runDir, "events.jsonl")
			return launch, nil
		},
		ParseEventsFn: claude.ParseEvents,
		ComposeSendFn: func(text string) []shuttleengine.PaneInput {
			return []shuttleengine.PaneInput{{Text: text, Submit: true}}
		},
		SkillLoadMessageFn:  claude.SkillLoadMessage,
		ClassifySkillLoadFn: claude.ClassifySkillLoad,
	}
	cfg := shuttleengine.Config{PollIntervalMS: 1000, LivenessEveryNPolls: 1, RunTimeoutMin: 60, StartupTimeoutS: 30, BackgroundShellWaitMin: 10}
	runner := shuttleengine.NewRunner(reed, engine, worktree, worktree, cfg)
	runner.SetClock(replay)
	runner.SetNotifier(func(string) error {
		replay.notices++
		return nil
	})

	var gate shuttleengine.GateSpec
	if c.Gated {
		gate = shuttleengine.GateSpec{{
			Name:     "replay",
			Gate:     func() (shuttleengine.GateResult, error) { return shuttleengine.GateResult{Passed: true}, nil },
			Attempts: 1,
		}}
	}
	spec := shuttleengine.Spec{Prompt: "replay", OutputFiles: []string{replay.outputFile}, Role: "replay", Timeout: replay.runTimeout, Skills: c.Skills}
	run, err := runner.StartGated(spec, gate)
	if err != nil {
		t.Fatalf("%s: start: %v", name, err)
	}
	result, err := run.Wait()
	if err != nil {
		t.Fatalf("%s: wait: %v", name, err)
	}

	want := shuttleengine.OutcomeTimeout
	if c.TurnEnds[len(c.TurnEnds)-1].Reading == readingDone {
		want = shuttleengine.OutcomeDone
	}
	if result.Outcome != want || replay.appended != len(lines) {
		t.Errorf("%s: the run ended %q after line %d of %d; want %q after the last", name, result.Outcome, replay.appended, len(lines), want)
	}
}

// checkCorpusStates folds the case's lines after the ones Start consumes, a line at a time, through the real ParseSessionSignals into one SessionFold,
// and checks the state after each line that yields a signal against the case's states in order.
// The facts are the replay's own: liveness alive, an autonomous run, outputs present exactly when the newest turn end at or before the line says so, a fixed reading time and no transcript marker.
// The marker is left out because the case's transcript is the whole session's final file and would mark every turn end with its end state.
func checkCorpusStates(t *testing.T, claude *claudeengine.Claude, name string, c corpusCase, lines []string, turnEndOf []int, consumed int) {
	t.Helper()
	var fold shuttleengine.SessionFold
	facts := shuttleengine.SessionFacts{Liveness: shuttleengine.LivenessAlive, ReadAt: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)}
	read := 0
	for i, line := range lines {
		if i < consumed {
			continue
		}
		if turnEndOf[i] >= 0 {
			facts.OutputsExist = c.TurnEnds[turnEndOf[i]].OutputsExist
		}
		signals, _ := claude.ParseSessionSignals([]byte(line + "\n"))
		if len(signals) == 0 {
			continue
		}
		fold.Fold(signals, facts)
		got := fold.State()
		if read == len(c.States) {
			t.Fatalf("%s: line %d yields a signal, but case.yaml names only %d states", name, i+1, len(c.States))
		}
		want := c.States[read]
		if string(got.Name) != want.State || got.Cause != want.Cause || got.Detail != want.Detail {
			t.Errorf("%s: state %d after line %d = %s/%s %q; want %s/%s %q", name, read+1, i+1, got.Name, got.Cause, got.Detail, want.State, want.Cause, want.Detail)
		}
		read++
	}
	if read != len(c.States) {
		t.Errorf("%s: %d lines yield a signal; case.yaml names %d states", name, read, len(c.States))
	}
}

// checkCorpusParse checks that ParseEvents reads turn end number from line as one event:
// a waiting one with exactly the case's tasks, any other one as a Stop with nothing outstanding.
func checkCorpusParse(t *testing.T, claude *claudeengine.Claude, name string, number int, line string, te corpusTurnEnd) {
	t.Helper()
	events, err := claude.ParseEvents([]byte(line))
	if err != nil || len(events) != 1 {
		t.Fatalf("%s: turn end %d: ParseEvents = %v, %v; want one event", name, number, events, err)
	}
	wantKind := shuttleengine.EventStop
	if te.Reading == readingWaiting {
		wantKind = shuttleengine.EventWaiting
	}
	var got []corpusTask
	for _, task := range events[0].Outstanding {
		got = append(got, corpusTask{Kind: task.Kind, ID: task.ID, Signal: task.Signal})
	}
	if events[0].Kind != wantKind || !reflect.DeepEqual(got, te.Tasks) {
		t.Errorf("%s: turn end %d: ParseEvents reads kind %v with tasks %+v; want kind %v with tasks %+v", name, number, events[0].Kind, got, wantKind, te.Tasks)
	}
}

// loadCorpusCase reads dir's case.yaml.
func loadCorpusCase(t *testing.T, dir string) corpusCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "case.yaml"))
	if err != nil {
		t.Fatalf("read case.yaml: %v", err)
	}
	var c corpusCase
	if err := yaml.Unmarshal(data, &c); err != nil {
		t.Fatalf("parse %s: %v", filepath.Join(dir, "case.yaml"), err)
	}
	if len(c.TurnEnds) == 0 {
		t.Fatalf("%s names no turn end", filepath.Join(dir, "case.yaml"))
	}
	if len(c.States) == 0 {
		t.Fatalf("%s names no state", filepath.Join(dir, "case.yaml"))
	}
	return c
}

// loadCorpusEvents returns dir's events.jsonl lines, each with its transcript_path rewritten to dir's transcript.jsonl.
func loadCorpusEvents(t *testing.T, dir string) []string {
	t.Helper()
	transcript, err := filepath.Abs(filepath.Join(dir, "transcript.jsonl"))
	if err != nil {
		t.Fatalf("resolve transcript: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatalf("read events.jsonl: %v", err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var fields map[string]any
		if err := json.Unmarshal([]byte(line), &fields); err != nil {
			t.Fatalf("parse %s: %v", filepath.Join(dir, "events.jsonl"), err)
		}
		fields["transcript_path"] = transcript
		rewritten, err := json.Marshal(fields)
		if err != nil {
			t.Fatalf("rewrite transcript_path: %v", err)
		}
		lines = append(lines, string(rewritten))
	}
	return lines
}
