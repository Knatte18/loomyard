//go:build linux

// serve_linux_integration_test.go runs a built binary's board page verbs, the detached server included, in a hub.

package boardcli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardserve"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// viewerEnvelope is the JSON answer of `board open` or `board serve`.
type viewerEnvelope struct {
	OK       bool   `json:"ok"`
	Address  string `json:"address"`
	Launcher string `json:"launcher"`
	Started  bool   `json:"started"`
	Running  bool   `json:"running"`
}

func TestBoardServeScenario(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	bin := lymbin.Build(t)
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "task-a")
	prime := h.PrimeWorktree()
	paths := boardserve.Paths(fabricengine.HubScratchDir(h.Path))

	stubs := t.TempDir()
	opened := filepath.Join(stubs, "opened")
	script := "#!/bin/sh\nprintf '%s' \"$1\" > '" + opened + "'\n"
	if err := os.WriteFile(filepath.Join(stubs, "xdg-open"), []byte(script), 0o755); err != nil {
		t.Fatalf("write the opener stub: %v", err)
	}
	env := []string{"BOARD_SKIP_GIT=1", "LYM_STRAND_NAME=", "PATH=" + stubs + string(os.PathListSeparator) + os.Getenv("PATH")}

	run := func(dir, verb string) viewerEnvelope {
		t.Helper()
		out, code, err := lymbin.RunWithEnv(bin, dir, env, time.Minute, "board", verb)
		if err != nil || code != 0 {
			t.Fatalf("board %s = exit %d, %v: %s", verb, code, err, out)
		}
		// Log lines may share the output, and the envelope is the one line that decodes as an answer.
		for _, line := range strings.Split(out, "\n") {
			var envelope viewerEnvelope
			if json.Unmarshal([]byte(line), &envelope) == nil && envelope.OK {
				return envelope
			}
		}
		t.Fatalf("board %s answered no envelope: %s", verb, out)
		return viewerEnvelope{}
	}
	// The server outlives the command that started it, so the test ends it.
	t.Cleanup(func() {
		current, found, err := boardserve.ReadState(paths)
		if err != nil || !found || !proc.IsRunning(current.PID) {
			return
		}
		if err := syscall.Kill(current.PID, syscall.SIGKILL); err != nil {
			t.Errorf("end the server %d: %v", current.PID, err)
			return
		}
		deadline := time.Now().Add(time.Minute)
		for proc.IsRunning(current.PID) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
	})

	first := run(h.PairCodeWorktree("task-a"), "open")
	current, found, err := boardserve.ReadState(paths)
	if err != nil || !found {
		t.Fatalf("ReadState() = found %v, %v; want the server's state", found, err)
	}
	if !first.Started || first.Address != boardserve.PageAddress(current.Port) || strings.Contains(first.Address, current.Token) {
		t.Errorf("first open = %+v; want a started server's address without its token", first)
	}
	if got, err := os.ReadFile(opened); err != nil || string(got) != first.Launcher || first.Launcher != paths.Launcher {
		t.Errorf("the opener got %q, %v; want the launcher %q", got, err, paths.Launcher)
	}
	cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", current.PID))
	if err != nil {
		t.Fatalf("read the server's cwd: %v", err)
	}
	wantCwd, err := filepath.EvalSymlinks(prime)
	if err != nil {
		t.Fatalf("resolve the prime: %v", err)
	}
	if cwd != wantCwd {
		t.Errorf("server cwd = %q; want the prime's anchor %q", cwd, wantCwd)
	}

	if second := run(h.PairCodeWorktree("task-a"), "open"); second.Started {
		t.Errorf("second open = %+v; want the running server reused", second)
	}
	if loser := run(prime, "serve"); !loser.Running {
		t.Errorf("a second serve = %+v; want running", loser)
	}
	if err := boardserve.Probe(current); err != nil {
		t.Errorf("Probe() of the first server = %v; want it still answering", err)
	}
}
