//go:build linux

// serve_linux_integration_test.go drives the built `lym board open` and `lym board serve` in a hubforge hub.
// A stub xdg-open records the launcher path it is handed, so no browser starts.

package boardcli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardserve"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// scenarioVerbWait bounds each lym call of the scenario.
const scenarioVerbWait = 2 * time.Minute

// TestBoardServeScenario runs open twice and serve once against one hub, from the pair's worktree and the prime's anchor.
func TestBoardServeScenario(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	bin := lymbin.Build(t)
	hub := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, hub, "serve-pair")
	pair := hub.PairCodeWorktree("serve-pair")
	primeAnchor := filepath.Join(hub.PrimeWorktree(), hub.Anchor)
	paths := boardserve.Paths(fabricengine.HubScratchDir(hub.Path))

	stubDir := t.TempDir()
	recorded := filepath.Join(stubDir, "opened.txt")
	stub := "#!/bin/sh\nprintf '%s' \"$1\" > '" + recorded + "'\n"
	if err := os.WriteFile(filepath.Join(stubDir, "xdg-open"), []byte(stub), 0o755); err != nil {
		t.Fatalf("write the xdg-open stub: %v", err)
	}
	// The empty strand name runs each call as the operator's unnamed shell, whatever session runs this test.
	env := []string{
		"BOARD_SKIP_GIT=1",
		agentname.StrandNameEnv + "=",
		"PATH=" + stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}

	t.Cleanup(func() {
		st, found, _ := boardserve.ReadState(paths)
		if !found {
			return
		}
		_ = syscall.Kill(st.PID, syscall.SIGTERM)
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if live, err := boardserve.Live(paths); err == nil && !live {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	})

	run := func(dir string, args ...string) envelope.Envelope {
		t.Helper()
		out, code, err := lymbin.RunWithEnv(bin, dir, env, scenarioVerbWait, args...)
		if err != nil || code != 0 {
			t.Fatalf("lym %v = exit %d, %v:\n%s", args, code, err, out)
		}
		return envelope.Decode(t, out)
	}

	first := run(pair, "board", "open")
	st, found, err := boardserve.ReadState(paths)
	if err != nil || !found {
		t.Fatalf("ReadState after open = %+v, %v, %v; want the server's state", st, found, err)
	}
	address, _ := first.Raw["address"].(string)
	if started, _ := first.Raw["started"].(bool); !started {
		t.Errorf("first open: started = %v; want true", first.Raw["started"])
	}
	if want := "http://127.0.0.1:" + strconv.Itoa(st.Port) + "/"; address != want || !regexp.MustCompile(`^http://127\.0\.0\.1:\d+/$`).MatchString(address) {
		t.Errorf("first open: address = %q; want %q with no token", address, want)
	}
	if opened, _ := os.ReadFile(recorded); string(opened) != paths.Launcher {
		t.Errorf("the opener was handed %q; want the launcher %s", opened, paths.Launcher)
	}
	wantCwd, _ := filepath.EvalSymlinks(primeAnchor)
	if gotCwd, err := filepath.EvalSymlinks("/proc/" + strconv.Itoa(st.PID) + "/cwd"); err != nil || gotCwd != wantCwd {
		t.Errorf("server cwd = %q, %v; want the prime's anchored directory %q", gotCwd, err, wantCwd)
	}

	second := run(pair, "board", "open")
	if started, _ := second.Raw["started"].(bool); started {
		t.Errorf("second open: started = true; want false for the server that runs")
	}

	served := run(primeAnchor, "board", "serve")
	if running, _ := served.Raw["running"].(bool); !served.OK || !running {
		t.Errorf("serve beside a running server = %+v; want ok with running true", served)
	}
	if err := boardserve.Probe(st, 10*time.Second); err != nil {
		t.Errorf("the first server stopped answering after the second serve: %v", err)
	}
}
