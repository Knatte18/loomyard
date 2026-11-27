// scenarios_test.go holds the scenarios the package's recordings were made from, and what drives them.

package tmuxrec_test

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxrec"
)

// paneEndPolls is how many status reads a scenario makes at most while a pane's process ends.
const paneEndPolls = 50

// world is what a scenario drives: the session, and how a pane's process ends in it.
type world struct {
	session *tmuxrec.Session
	// endProcess ends the process pid, the process of a strand's pane.
	// Over a replay the recording holds the end, so it does nothing.
	endProcess func(pid int)
	// wait spaces two reads while a pane's process ends.
	wait func()
}

// sessionSpec is the spec the recorded sessions start under: an interactive, unawaited PR reviewer.
func sessionSpec(resume string) shuttleengine.Spec {
	return shuttleengine.Spec{
		Prompt:          "Review the pull request.",
		PromptEngine:    tmuxrec.EngineName,
		ModelChoice:     shuttleengine.ModelChoice{Engine: tmuxrec.EngineName, Model: "fixture-model"},
		Interactive:     true,
		Unawaited:       true,
		DenyPublish:     true,
		Role:            string(agentconfig.RolePRReviewer),
		Segment:         segmentcolor.Landing,
		ConfigRole:      agentconfig.RolePRReviewer,
		ResumeSessionID: resume,
	}
}

// reading reads reed's strand table as the scenario's step.
func reading(w *world, step string) tmuxrec.Reading {
	status, err := w.session.Reed.Status()
	got := tmuxrec.Reading{Step: step}
	if err != nil {
		got.Err = err.Error()
		return got
	}
	for _, strand := range status.Strands {
		got.Strands = append(got.Strands, tmuxrec.StrandReading{GUID: strand.GUID, Name: strand.Name, Live: strand.Live, Retiring: strand.Retiring})
	}
	return got
}

// notStarted is the error a reading records for a start whose provider never came up.
const notStarted = "the provider did not start"

// scenario is one recorded scenario: the runner's startup timeout it runs under, and its steps.
// The steps return what reed read at each.
type scenario struct {
	startupTimeoutS int
	run             func(t *testing.T, w *world) []tmuxrec.Reading
}

// sessionStartupTimeoutS gives a session's provider time to come up.
const sessionStartupTimeoutS = 30

// scenarios are the package's recorded scenarios, by recording name.
// Each reads the strand table before any session starts.
var scenarios = map[string]scenario{
	// A session starts, is stopped, starts again resuming its provider session, and is stopped again.
	"session-start-stop": {startupTimeoutS: sessionStartupTimeoutS, run: func(t *testing.T, w *world) []tmuxrec.Reading {
		runner := w.session.Runner
		readings := []tmuxrec.Reading{reading(w, "before")}
		first, err := runner.StartSession(sessionSpec(""))
		if err != nil {
			t.Fatalf("StartSession() error = %v", err)
		}
		readings = append(readings, reading(w, "started"))
		if err := runner.StopStrand(first.StrandGUID); err != nil {
			t.Fatalf("StopStrand(%q) error = %v", first.StrandGUID, err)
		}
		readings = append(readings, reading(w, "stopped"))
		second, err := runner.StartSession(sessionSpec(first.SessionID))
		if err != nil {
			t.Fatalf("StartSession() of the resume error = %v", err)
		}
		readings = append(readings, reading(w, "resumed"))
		if err := runner.StopStrand(second.StrandGUID); err != nil {
			t.Fatalf("StopStrand(%q) error = %v", second.StrandGUID, err)
		}
		return append(readings, reading(w, "stopped again"))
	}},
	// A session starts, its pane's process ends, and the strand is stopped.
	"session-ends": {startupTimeoutS: sessionStartupTimeoutS, run: func(t *testing.T, w *world) []tmuxrec.Reading {
		runner := w.session.Runner
		readings := []tmuxrec.Reading{reading(w, "before")}
		started, err := runner.StartSession(sessionSpec(""))
		if err != nil {
			t.Fatalf("StartSession() error = %v", err)
		}
		readings = append(readings, reading(w, "started"))
		status, err := w.session.Reed.Status()
		if err != nil || len(status.Strands) != 1 {
			t.Fatalf("Status() = %+v, %v; want the one started strand", status, err)
		}
		w.endProcess(status.Strands[0].PanePID)
		var last tmuxrec.Reading
		ended := false
		for range paneEndPolls {
			last = reading(w, "ended")
			if len(last.Strands) == 1 && !last.Strands[0].Live {
				ended = true
				break
			}
			w.wait()
		}
		if !ended {
			t.Fatalf("no reading found the strand dead in paneEndPolls = %d reads; last = %+v", paneEndPolls, last)
		}
		readings = append(readings, last)
		if err := runner.StopStrand(started.StrandGUID); err != nil {
			t.Fatalf("StopStrand(%q) error = %v", started.StrandGUID, err)
		}
		return append(readings, reading(w, "stopped"))
	}},
	// A session starts under a runner that gives its provider no time, and the provider is never up.
	"session-not-started": {startupTimeoutS: 0, run: func(t *testing.T, w *world) []tmuxrec.Reading {
		w.session.Engine.StartupFn = func(string) shuttleengine.StartupState { return shuttleengine.StartupPending }
		readings := []tmuxrec.Reading{reading(w, "before")}
		_, err := w.session.Runner.StartSession(sessionSpec(""))
		if !errors.Is(err, shuttleengine.ErrNotStarted) {
			t.Fatalf("StartSession() error = %v; want one wrapping ErrNotStarted", err)
		}
		readings = append(readings, tmuxrec.Reading{Step: "start", Err: notStarted})
		return append(readings, reading(w, "after"))
	}},
}
