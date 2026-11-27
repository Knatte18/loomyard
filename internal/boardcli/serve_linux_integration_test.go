//go:build linux

// serve_linux_integration_test.go runs a built lym in a hub, so `board open` starts a real detached server.

package boardcli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
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

// viewerAnswer is the union of the envelopes `board open` and `board serve` print.
type viewerAnswer struct {
	OK       bool   `json:"ok"`
	Address  string `json:"address"`
	Launcher string `json:"launcher"`
	Started  bool   `json:"started"`
	Running  bool   `json:"running"`
}

func TestBoardServeScenario(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	lym := lymbin.Build(t)
	hub := hubforge.NewHub(t, ".")
	pair := hubforge.AddPair(t, hub, "viewer-task")
	paths := boardserve.Paths(fabricengine.HubScratchDir(hub.Path))

	stubDir := t.TempDir()
	opened := filepath.Join(stubDir, "opened")
	stub := "#!/bin/sh\nprintf %s \"$1\" > '" + opened + "'\n"
	if err := os.WriteFile(filepath.Join(stubDir, "xdg-open"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"BOARD_SKIP_GIT=1", "LYM_STRAND_NAME=", "PATH=" + stubDir + string(os.PathListSeparator) + os.Getenv("PATH")}

	t.Cleanup(func() {
		server, found, err := boardserve.ReadState(paths)
		if err != nil || !found {
			return
		}
		_ = syscall.Kill(server.PID, syscall.SIGKILL)
		for deadline := time.Now().Add(time.Minute); proc.IsRunning(server.PID) && time.Now().Before(deadline); {
			time.Sleep(10 * time.Millisecond)
		}
	})
	run := func(dir string, args ...string) viewerAnswer {
		t.Helper()
		out, code, err := lymbin.RunWithEnv(lym, dir, env, time.Minute, args...)
		if err != nil || code != 0 {
			t.Fatalf("lym %v = %d, %v: %s", args, code, err, out)
		}
		var answer viewerAnswer
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &answer); err != nil || !answer.OK {
			t.Fatalf("lym %v printed %q: %v", args, out, err)
		}
		return answer
	}

	first := run(pair.Path, "board", "open")
	server, found, err := boardserve.ReadState(paths)
	if err != nil || !found {
		t.Fatalf("ReadState = %v, %v; want the started server's state", found, err)
	}
	if !first.Started || first.Address != boardserve.PageAddress(server.Port) || strings.Contains(first.Address, server.Token) {
		t.Errorf("first open = %+v; want a started server at %s", first, boardserve.PageAddress(server.Port))
	}
	if argument, err := os.ReadFile(opened); err != nil || string(argument) != first.Launcher {
		t.Errorf("the opener got %q, %v; want %q", argument, err, first.Launcher)
	}
	cwd, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(server.PID), "cwd"))
	if err != nil {
		t.Fatal(err)
	}
	prime, err := filepath.EvalSymlinks(hub.Location.AnchorPath())
	if err != nil {
		t.Fatal(err)
	}
	if cwd != prime {
		t.Errorf("the server runs in %s; want the prime's anchored directory %s", cwd, prime)
	}

	if second := run(pair.Path, "board", "open"); second.Started {
		t.Errorf("second open = %+v; want the running server taken up", second)
	}
	if rival := run(hub.Location.AnchorPath(), "board", "serve"); !rival.Running {
		t.Errorf("second serve = %+v; want running", rival)
	}
	if err := boardserve.Probe(server); err != nil {
		t.Errorf("Probe of the first server = %v; want it still answering", err)
	}
}
