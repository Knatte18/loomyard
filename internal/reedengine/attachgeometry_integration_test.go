//go:build linux

// attachgeometry_integration_test.go proves the attach handover against real tmux, real pty and real client.
// It also holds the pty harness the other tmux-tier suites attach through.
//
// Linux-only: the pty harness below is built directly on golang.org/x/sys/unix's /dev/ptmx ioctls:
// TIOCSPTLCK, TIOCGPTN and TIOCSWINSZ, which have no portable equivalent.
// psmux's behaviour under a real pty is unverified anywhere in this repo.
// This file must not attempt to compile off Linux.

package reedengine

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// attachGeometryPTY is a running child process attached to a real pty of a fixed initial size.
// It keeps the master file descriptor open, so a caller can drive further ioctls or reads against it.
type attachGeometryPTY struct {
	master *os.File
	cmd    *exec.Cmd

	// mu guards output, which the drain goroutine appends to.
	mu sync.Mutex
	// output is what the child wrote to the pty since the last forget.
	output strings.Builder
}

// drew reports whether the child has written text to the pty since the last forget.
func (p *attachGeometryPTY) drew(text string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.Contains(p.output.String(), text)
}

// forget discards what the child has written to the pty so far, so a later drew sees only newer output.
func (p *attachGeometryPTY) forget() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.output.Reset()
}

// startInPTY opens a fresh /dev/ptmx master sized to cols x rows and starts argv[0] with the pty
// slave as its stdin, stdout and stderr.
// It sets Setsid and Setctty, which make the slave the controlling terminal.
// Ctty's zero value names fd 0 in the child's descriptor table, and fd 0 is the slave because stdin is wired to it.
//
// The master is drained continuously in a background goroutine, so the child never blocks writing to
// a full pty buffer.
// Both the master and the started process are torn down via t.Cleanup, so a failing assertion never leaks either.
func startInPTY(t *testing.T, argv []string, cols, rows int) *attachGeometryPTY {
	t.Helper()

	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open /dev/ptmx: %v", err)
	}
	fd := int(master.Fd())

	// Clear the slave's lock before it can be opened: /dev/ptmx starts every new pty locked.
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		master.Close()
		t.Fatalf("TIOCSPTLCK (clear pty lock): %v", err)
	}
	slaveNum, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		master.Close()
		t.Fatalf("TIOCGPTN (resolve slave number): %v", err)
	}
	// Size the pty before the child reads or writes it: TIOCSWINSZ on either end of a pty pair sets
	// the size both sides observe.
	// Setting it here makes the child's very first #{window_width} and #{window_height} match the test's size.
	if err := unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(cols), Row: uint16(rows)}); err != nil {
		master.Close()
		t.Fatalf("TIOCSWINSZ (%dx%d): %v", cols, rows, err)
	}

	slavePath := "/dev/pts/" + strconv.Itoa(slaveNum)
	slave, err := os.OpenFile(slavePath, os.O_RDWR, 0)
	if err != nil {
		master.Close()
		t.Fatalf("open pty slave %s: %v", slavePath, err)
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	cmd.Env = append(os.Environ(), "TERM=xterm")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	startErr := cmd.Start()
	// The child keeps its own copy of the slave once started, or for good after a failed start.
	// The test needs only the master.
	slave.Close()
	if startErr != nil {
		master.Close()
		t.Fatalf("start %v in pty: %v", argv, startErr)
	}

	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = master.Close()
	})

	pty := &attachGeometryPTY{master: master, cmd: cmd}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			if err != nil {
				return
			}
			pty.mu.Lock()
			pty.output.Write(buf[:n])
			pty.mu.Unlock()
		}
	}()

	return pty
}

// setupAttachGeometryFixture boots a fresh integration engine and adds two strands.
// The parent is added first and its child second, so the session carries a Selvage pane and two strand panes.
func setupAttachGeometryFixture(t *testing.T) *Engine {
	t.Helper()

	e := newIntegrationEngine(t, "off")
	if _, err := e.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}

	parent, err := e.AddStrand(AddSpec{
		Cmd:     "sleep 300",
		Display: render.Display{Anchor: render.AnchorOwnWindow},
	})
	if err != nil {
		t.Fatalf("AddStrand(parent): %v", err)
	}
	if _, err := e.AddStrand(AddSpec{
		Cmd:     "sleep 300",
		Parent:  parent.GUID,
		Display: render.Display{Anchor: render.AnchorOwnWindow},
	}); err != nil {
		t.Fatalf("AddStrand(child): %v", err)
	}

	return e
}

// waitForClientAttached polls list-clients until this session reports at least one attached client.
func waitForClientAttached(t *testing.T, e *Engine, timeout time.Duration) {
	t.Helper()
	waitUntil(t, timeout, "no tmux client ever attached to the session", func() bool {
		out, err := e.tmux.output("list-clients", "-t", exactSessionTarget(e.SessionName()))
		return err == nil && strings.TrimSpace(out) != ""
	})
}

// TestAttachGeometry_DegradedPathStillAttaches asserts AttachArgv(0, 0)'s argv still lands a working attach.
// Attach is the operator's escape hatch.
// This pins that a pre-flight with no client size known is a working attach, not a broken one.
func TestAttachGeometry_DegradedPathStillAttaches(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	e := setupAttachGeometryFixture(t)

	argv := e.AttachArgv(0, 0)
	// The strands live in their own windows, so an agent window is relevant,
	// and the bare prefix carries the chained select-window tail.
	bare := bareAttachArgv(e.Socket(), e.SessionName())
	if len(argv) != len(bare)+4 {
		t.Fatalf("AttachArgv(0, 0) = %v (%d argv elements), want the bare form plus a 4-element select-window tail", argv, len(argv))
	}
	for i, want := range bare {
		if argv[i] != want {
			t.Fatalf("AttachArgv(0, 0)[%d] = %q, want %q; argv=%v", i, argv[i], want, argv)
		}
	}
	if tail := argv[len(bare):]; tail[0] != ";" || tail[1] != "select-window" || tail[2] != "-t" || tail[3] == "" {
		t.Fatalf("AttachArgv(0, 0) tail = %v, want [; select-window -t <window>]", tail)
	}

	pty := startInPTY(t, append([]string{e.cfg.Tmux}, argv...), 80, 24)
	waitForClientAttached(t, e, 15*time.Second)

	if !proc.IsAlive(pty.cmd.Process.Pid) {
		t.Fatalf("attach-session process (pid %d) is not alive after attach was confirmed via list-clients", pty.cmd.Process.Pid)
	}
	if pty.cmd.ProcessState != nil && pty.cmd.ProcessState.ExitCode() != 0 {
		t.Errorf("attach-session process exited with code %d, want it still running (or a clean 0)", pty.cmd.ProcessState.ExitCode())
	}
}

// TestAttachGeometry_ClientExitStatusOnSessionEndAndServerShutdown pins tmux's client exit statuses.
// The run window's close rule rests on these statuses.
// A client attached to a session that is killed while the server lives on exits 0.
// A client attached to a session whose server is killed exits non-zero.
// A second session is created up front, so killing the first leaves the server running under tmux's exit-empty.
func TestAttachGeometry_ClientExitStatusOnSessionEndAndServerShutdown(t *testing.T) {
	tiergate.Require(t, 2)
	t.Parallel()
	e := setupAttachGeometryFixture(t)
	const otherSession = "attach-exit-status-other"
	if err := e.tmux.run("new-session", "-d", "-s", otherSession, "sleep 300"); err != nil {
		t.Fatalf("new-session %s: %v", otherSession, err)
	}

	const cols, rows = 100, 30
	endedSession := startInPTY(t, append([]string{e.cfg.Tmux}, e.AttachArgv(cols, rows)...), cols, rows)
	waitForClientAttached(t, e, 15*time.Second)
	otherClient := startInPTY(t, append([]string{e.cfg.Tmux}, bareAttachArgv(e.Socket(), otherSession)...), cols, rows)
	waitUntil(t, 15*time.Second, "no tmux client ever attached to the second session", func() bool {
		out, err := e.tmux.output("list-clients", "-t", exactSessionTarget(otherSession))
		return err == nil && strings.TrimSpace(out) != ""
	})

	exitCodeOf := func(client *attachGeometryPTY) int {
		t.Helper()
		done := make(chan error, 1)
		go func() { done <- client.cmd.Wait() }()
		select {
		case err := <-done:
			var exitErr *exec.ExitError
			switch {
			case err == nil:
				return 0
			case errors.As(err, &exitErr):
				return exitErr.ExitCode()
			default:
				t.Fatalf("wait for the attach client: %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Fatal("the attach client did not exit within 15s")
		}
		return -1
	}

	if err := e.tmux.run("kill-session", "-t", exactSessionTarget(e.SessionName())); err != nil {
		t.Fatalf("kill-session: %v", err)
	}
	if code := exitCodeOf(endedSession); code != 0 {
		t.Errorf("client exit status after its session was killed = %d; want 0", code)
	}

	if err := e.tmux.run("kill-server"); err != nil {
		t.Logf("kill-server: %v", err)
	}
	if code := exitCodeOf(otherClient); code == 0 {
		t.Errorf("client exit status after the server was killed = 0; want non-zero")
	}
}
