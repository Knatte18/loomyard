//go:build linux

// serve_linux_integration_test.go starts a real board server through a built lym in a hub, with a stub opener.

package boardcli_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
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
	hub := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, hub, "viewer-pair")
	paths := boardserve.Paths(fabricengine.HubScratchDir(hub.Path))

	stubDir := t.TempDir()
	record := filepath.Join(stubDir, "opened")
	stub := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> '" + record + "'\n"
	if err := os.WriteFile(filepath.Join(stubDir, "xdg-open"), []byte(stub), 0o755); err != nil {
		t.Fatalf("write the stub opener: %v", err)
	}
	env := []string{
		"BOARD_SKIP_GIT=1",
		agentname.StrandNameEnv + "=",
		"PATH=" + stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	run := func(dir string, args ...string) envelope.Envelope {
		t.Helper()
		out, code, err := lymbin.RunWithEnv(bin, dir, env, time.Minute, args...)
		if err != nil || code != 0 {
			t.Fatalf("lym %v = exit %d, %v: %s", args, code, err, out)
		}
		return envelope.RequireOK(t, out)
	}

	t.Cleanup(func() {
		server, found, err := boardserve.ReadState(paths)
		if err != nil || !found {
			return
		}
		if err := syscall.Kill(server.PID, syscall.SIGTERM); err != nil {
			t.Errorf("end the board server %d: %v", server.PID, err)
			return
		}
		deadline := time.Now().Add(time.Minute)
		for proc.IsRunning(server.PID) {
			if time.Now().After(deadline) {
				t.Errorf("the board server %d did not end", server.PID)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	})

	first := run(hub.PairCodeWorktree("viewer-pair"), "board", "open")
	server, found, err := boardserve.ReadState(paths)
	if err != nil || !found {
		t.Fatalf("ReadState = %v, %v", found, err)
	}
	address, _ := first.Raw["address"].(string)
	if first.Raw["started"] != true || address != boardserve.PageAddress(server.Port) || strings.Contains(address, server.Token) {
		t.Errorf("open = %v; want a started server's address without the token", first.Raw)
	}
	if opened, err := os.ReadFile(record); err != nil || string(opened) != paths.Launcher+"\n" || first.Raw["launcher"] != paths.Launcher {
		t.Errorf("the opener was handed %q, %v; want %q", opened, err, paths.Launcher)
	}
	prime, err := filepath.EvalSymlinks(hub.PrimeWorktree())
	if err != nil {
		t.Fatalf("resolve the prime: %v", err)
	}
	if cwd, err := os.Readlink(fmt.Sprintf("/proc/%d/cwd", server.PID)); err != nil || cwd != prime {
		t.Errorf("the server's directory = %q, %v; want %q", cwd, err, prime)
	}

	if second := run(hub.PairCodeWorktree("viewer-pair"), "board", "open"); second.Raw["started"] != false {
		t.Errorf("a second open = %v; want no start", second.Raw)
	}

	if other := run(hub.PrimeWorktree(), "board", "serve"); other.Raw["running"] != true {
		t.Errorf("a second serve = %v; want running", other.Raw)
	}
	if err := boardserve.Probe(server); err != nil {
		t.Errorf("Probe after a second serve = %v", err)
	}
}
