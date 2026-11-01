//go:build tier2 && linux

// buttonclick_integration_test.go drives runButtonVia against a real tmux server and an attached client, with the real process runner and a recorder script standing in for the lym binary.

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
)

// TestRunButton_RealTmux runs each kind of spec entry through a real server: a plain one in the run directory, one that exits non-zero, one behind a confirm prompt answered in the client's terminal, and one naming {strand}.
// It reads the logger's output through logcapture, which is process-global, so it does not call t.Parallel.
func TestRunButton_RealTmux(t *testing.T) {
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
	}}
	specJSON, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	if err := e.tmux.run("set-option", "-t", exactSessionWindowTarget(e.SessionName()), buttonSpecOption, string(specJSON)); err != nil {
		t.Fatalf("set the spec option: %v", err)
	}
	deps := buttonDeps{run: runButtonScript, executablePath: func() (string, error) { return recorder, nil }, sh: shell.Posix()}
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
}
