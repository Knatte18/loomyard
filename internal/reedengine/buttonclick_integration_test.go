//go:build linux

// buttonclick_integration_test.go drives runButtonVia against a real tmux server and an attached client.
// The real process runner runs the click's commands, with a recorder script standing in for the lym binary.

package reedengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestRunButton_RealTmux runs each kind of spec entry through a real server.
// The kinds are a plain entry in the run directory, one that exits non-zero, and one naming {strand}.
// Two more run behind prompts answered in the client's terminal: a confirm prompt, and an ask prompt.
// It reads the logger's output through logcapture, which is process-global, so it does not call t.Parallel.
func TestRunButton_RealTmux(t *testing.T) {
	tiergate.Require(t, 2)
	const strandName = "tst:a-task:agent"
	runDir := filepath.Join(t.TempDir(), "run dir")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	recordPath := filepath.Join(t.TempDir(), "recorded-args")
	recorder := filepath.Join(t.TempDir(), "lym-recorder")
	if err := os.WriteFile(recorder, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > "+recordPath+"\n"), 0o755); err != nil {
		t.Fatalf("write recorder script: %v", err)
	}

	e := newIntegrationEngine(t, "on")
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	pty := startInPTY(t, append([]string{e.cfg.Tmux}, e.AttachArgv(bindingsClientCols, bindingsClientRows)...), bindingsClientCols, bindingsClientRows)
	waitForClientAttached(t, e, 15*time.Second)
	client := strings.TrimSpace(mustTmuxOutput(t, e, "list-clients", "-F", "#{client_name}"))

	spec := buttonSpec{Shell: "/bin/sh", Flag: "-c", Dir: runDir, Entries: []buttonSpecEntry{
		{Index: 0, Label: "where", Run: "pwd > out"},
		{Index: 1, Label: "fail", Run: "echo first; exit 3"},
		{Index: 2, Label: "ask", Run: "touch confirmed", Confirm: "go?"},
		{Index: 3, Label: "name", Run: "printf %s {strand} > strand", Strand: true},
		{Index: 4, Label: "say", Run: "printf %s {answer} > answer", Ask: "Say what:", AskDefault: "dflt"},
	}}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	if err := e.tmux.run("set-option", "-t", exactSessionWindowTarget(e.SessionName()), buttonSpecOption, string(specJSON)); err != nil {
		t.Fatalf("set the spec option: %v", err)
	}
	deps := buttonDeps{
		run:            runButtonScript,
		executablePath: func() (string, error) { return recorder, nil },
		anchorOf:       func(dir string) (string, error) { return dir, nil },
		sh:             shell.Posix(),
	}
	request := func(rangeName string) ButtonRequest {
		return ButtonRequest{TmuxPath: e.cfg.Tmux, SocketPath: "", Session: e.SessionName(), Range: rangeName, Client: client}
	}
	logs := logcapture.CaptureVerbose(t)

	runButtonVia(e.tmux, request("lymbtn0"), deps)
	where, err := os.ReadFile(filepath.Join(runDir, "out"))
	if err != nil {
		t.Fatalf("read out: %v", err)
	}
	gotDir, _ := filepath.EvalSymlinks(strings.TrimSpace(string(where)))
	wantDir, _ := filepath.EvalSymlinks(runDir)
	if gotDir != wantDir {
		t.Errorf("the plain entry ran in %q, want the run directory %q", gotDir, wantDir)
	}

	runButtonVia(e.tmux, request("lymbtn1"), deps)
	if got := logs.String(); !strings.Contains(got, "exit=3") || !strings.Contains(got, "first") {
		t.Errorf("log = %q, want the failing entry's exit code 3 and output", got)
	}

	// confirm-before returns only once the prompt is answered, so the click runs beside the answer.
	prompted := make(chan struct{})
	go func() {
		defer close(prompted)
		runButtonVia(e.tmux, request("lymbtn2"), deps)
	}()
	// A "y" typed before the prompt opens lands in the pane harmlessly, so the answer is repeated until the click returns.
	waitUntil(t, 10*time.Second, "the click never returned after a y was typed", func() bool {
		select {
		case <-prompted:
			return true
		default:
		}
		if _, err := pty.master.WriteString("y"); err != nil {
			t.Fatalf("answer the prompt: %v", err)
		}
		return false
	})
	var recorded []string
	waitUntil(t, 10*time.Second, "the confirmed line never ran the recorder", func() bool {
		raw, err := os.ReadFile(recordPath)
		recorded = strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
		return err == nil && len(recorded) > 1
	})
	hash := buttonRunHash("touch confirmed")
	for _, want := range []string{"--confirmed", hash, e.SessionName()} {
		if !containsArg(recorded, want) {
			t.Errorf("recorded arguments = %q, want them to carry %q", recorded, want)
		}
	}
	confirmed := request("lymbtn2")
	confirmed.Confirmed, confirmed.Hash = true, hash
	runButtonVia(e.tmux, confirmed, deps)
	if _, err := os.Stat(filepath.Join(runDir, "confirmed")); err != nil {
		t.Errorf("the confirmed call left no file: %v", err)
	}

	pane := strings.TrimSpace(mustTmuxOutput(t, e, "display-message", "-p", "-c", client, "#{pane_id}"))
	if err := e.tmux.run("set-option", "-p", "-t", pane, strandNamePaneOption, strandName); err != nil {
		t.Fatalf("set the strand name option: %v", err)
	}
	runButtonVia(e.tmux, request("lymbtn3"), deps)
	gotStrand, err := os.ReadFile(filepath.Join(runDir, "strand"))
	if err != nil {
		t.Fatalf("read strand: %v", err)
	}
	if string(gotStrand) != strandName {
		t.Errorf("the strand entry wrote %q, want the clicking pane's name %q", gotStrand, strandName)
	}

	// The rows share one client and one session option, so they run in order.
	// The Escape row comes before the empty row, which proves the empty one reads a fresh answer and not the first row's.
	const askRun = "printf %s {answer} > answer"
	answerPath := filepath.Join(runDir, "answer")
	asks := []struct {
		name  string
		typed string
		// want is the file's content; wantNoFile says the click must leave no file and run no re-entry line.
		want       string
		wantNoFile bool
	}{
		{name: "AnswerWithEveryMetacharacterReachesTheFileIntact", typed: "it's \"q\" \\ $HOME ; ~ #{session_name} a  b\r", want: "it's \"q\" \\ $HOME ; ~ #{session_name} a  b"},
		{name: "EscapeRunsNothing", typed: "\x1b", wantNoFile: true},
		{name: "EmptyEnterTakesTheAskDefault", typed: "\r", want: "dflt"},
	}
	for _, tt := range asks {
		t.Run(tt.name, func(t *testing.T) {
			for _, path := range []string{answerPath, recordPath} {
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					t.Fatalf("clear %s: %v", path, err)
				}
			}
			answerTarget := exactSessionWindowTarget(e.SessionName())
			if tt.wantNoFile {
				// A template wrongly run by Escape would overwrite this value before its run-shell starts.
				if err := e.tmux.run("set-option", "-t", answerTarget, buttonAnswerOption, "stale"); err != nil {
					t.Fatalf("seed the answer option: %v", err)
				}
			}
			pty.forget()
			returned := make(chan struct{})
			go func() {
				defer close(returned)
				runButtonVia(e.tmux, request("lymbtn4"), deps)
			}()
			waitUntil(t, 10*time.Second, "the ask prompt never drew", func() bool { return pty.drew("Say what:") })
			if _, err := pty.master.WriteString(tt.typed); err != nil {
				t.Fatalf("type the answer: %v", err)
			}
			waitUntil(t, 10*time.Second, "the click never returned after the answer was typed", func() bool {
				select {
				case <-returned:
					return true
				default:
					return false
				}
			})

			if tt.wantNoFile {
				if _, err := os.Stat(recordPath); err == nil {
					t.Errorf("an answered line ran after Escape, want no re-entry")
				}
				if got := strings.TrimSpace(mustTmuxOutput(t, e, "show-options", "-qv", "-t", answerTarget, buttonAnswerOption)); got != "stale" {
					t.Errorf("the answer option = %q after Escape, want it left at %q", got, "stale")
				}
				return
			}
			waitUntil(t, 10*time.Second, "the answered line never ran the recorder", func() bool {
				_, err := os.Stat(recordPath)
				return err == nil
			})
			answered := request("lymbtn4")
			answered.Answered, answered.Hash = true, buttonRunHash(askRun)
			runButtonVia(e.tmux, answered, deps)
			got, err := os.ReadFile(answerPath)
			if err != nil {
				t.Fatalf("read answer: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("the ask entry wrote %q, want %q", got, tt.want)
			}
		})
	}
}
