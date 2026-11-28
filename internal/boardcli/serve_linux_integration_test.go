// serve_linux_integration_test.go opens the board of a real hub with a built lym, and a stub in place of the opener.

package boardcli_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/boardserve"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestBoardServeScenario(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	bin := lymbin.Build(t)
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "viewer")
	prime := h.Location.AnchorPath()
	paths := boardserve.Paths(fabricengine.HubScratchDir(h.Path))

	stubs := t.TempDir()
	record := filepath.Join(stubs, "opened")
	if err := os.WriteFile(filepath.Join(stubs, "xdg-open"), []byte("#!/bin/sh\nprintf '%s' \"$1\" > '"+record+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"BOARD_SKIP_GIT=1", "LYM_STRAND_NAME=", "PATH=" + stubs + string(os.PathListSeparator) + os.Getenv("PATH")}

	// The server outlives the verbs that started it, so the test ends it by its recorded pid.
	t.Cleanup(func() {
		st, found, err := boardserve.ReadState(paths)
		if err != nil || !found {
			return
		}
		process, err := os.FindProcess(st.PID)
		if err != nil || !proc.IsRunning(st.PID) {
			return
		}
		_ = process.Kill()
		deadline := time.Now().Add(time.Minute)
		for proc.IsRunning(st.PID) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
	})

	// run runs a board verb in dir and returns the envelope it printed among any log lines.
	run := func(dir, verb string) envelope.Envelope {
		t.Helper()
		out, code, err := lymbin.RunWithEnv(bin, dir, env, 2*time.Minute, "board", verb)
		if err != nil || code != 0 {
			t.Fatalf("lym board %s = %d, %v: %s", verb, code, err, out)
		}
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "{") && strings.Contains(line, `"ok"`) {
				return envelope.RequireOK(t, line)
			}
		}
		t.Fatalf("lym board %s printed no envelope: %s", verb, out)
		return envelope.Envelope{}
	}

	first := run(h.PairCodeWorktree("viewer"), "open")
	st, found, err := boardserve.ReadState(paths)
	if err != nil || !found {
		t.Fatalf("ReadState() = %v, %v; want the server's state", found, err)
	}
	if first.Raw["started"] != true || first.Raw["address"] != boardserve.PageAddress(st.Port) || strings.Contains(first.Raw["address"].(string), st.Token) {
		t.Errorf("first open = %v; want a started server at its address, without the token", first.Raw)
	}
	opened, err := os.ReadFile(record)
	if err != nil || string(opened) != paths.Launcher {
		t.Errorf("the opener got %q, %v; want %q", opened, err, paths.Launcher)
	}
	cwd, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(st.PID), "cwd"))
	if err != nil {
		t.Fatal(err)
	}
	wantCwd, err := filepath.EvalSymlinks(prime)
	if err != nil {
		t.Fatal(err)
	}
	if cwd != wantCwd {
		t.Errorf("the server runs in %q; want the prime's anchored directory %q", cwd, wantCwd)
	}

	if second := run(h.PairCodeWorktree("viewer"), "open"); second.Raw["started"] != false {
		t.Errorf("second open = %v; want started false", second.Raw)
	}
	if served := run(prime, "serve"); served.Raw["running"] != true {
		t.Errorf("a second serve = %v; want running", served.Raw)
	}
	if err := boardserve.Probe(st); err != nil {
		t.Errorf("Probe() of the first server = %v", err)
	}
}
