//go:build tmux && linux

// attachgeometry_integration_test.go proves the attach handover works against a real tmux, real pty, and real attaching client.
// It also holds the pty harness the other tmux-tier suites attach through.
//
// Linux-only: the pty harness below is built directly on golang.org/x/sys/unix's /dev/ptmx ioctls
// (TIOCSPTLCK, TIOCGPTN, TIOCSWINSZ), which have no portable equivalent, and psmux's behaviour under
// a real pty is unverified anywhere in this repo — this file must not even attempt to compile off
// Linux.

package reedengine

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

// attachGeometryPTY is a running child process attached to a real pty of a fixed initial size, with
// the master file descriptor kept open so a caller can drive further ioctls or reads against it.
type attachGeometryPTY struct {
	master *os.File
	cmd    *exec.Cmd
}

// startInPTY opens a fresh /dev/ptmx master sized to cols x rows and starts argv[0] with the pty
// slave as its stdin/stdout/stderr and as its controlling terminal (SysProcAttr{Setsid: true,
// Setctty: true} — Setctty's zero-value Ctty names fd 0 in the CHILD's own descriptor table, which is
// the slave since it is also wired to stdin).
//
// The master is drained continuously in a background goroutine so the child never blocks writing to
// a full pty buffer, and both the master and the started process are torn down via t.Cleanup so a
// failing assertion never leaks either.
func startInPTY(t *testing.T, argv []string, cols, rows int) *attachGeometryPTY {
	t.Helper()

	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open /dev/ptmx: %v", err)
	}
	fd := int(master.Fd())

	// Clear the slave's lock before it can be opened at all — /dev/ptmx starts every new pty locked.
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		master.Close()
		t.Fatalf("TIOCSPTLCK (clear pty lock): %v", err)
	}
	slaveNum, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		master.Close()
		t.Fatalf("TIOCGPTN (resolve slave number): %v", err)
	}
	// Size the pty before the child ever reads/writes it: TIOCSWINSZ on either end of a pty pair sets
	// the size both sides observe, and setting it here means the child's very first #{window_width}/
	// #{window_height} are already the size this test drives.
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
	// The child holds its own copy of the slave once started (or forever, on a failed start); the
	// test only ever needs the master.
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

	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := master.Read(buf); err != nil {
				return
			}
		}
	}()

	return &attachGeometryPTY{master: master, cmd: cmd}
}

// setupAttachGeometryFixture boots a fresh integration engine and adds two strands — a
// parent and its child, in that insertion order — so the session carries a Selvage pane and two strand panes.
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

// TestAttachGeometry_DegradedPathStillAttaches asserts AttachArgv(0, 0)'s argv still
// lands a working attach: attach is the operator's escape hatch, and this pins
// that a pre-flight with no client size known is a working attach, not a broken one.
func TestAttachGeometry_DegradedPathStillAttaches(t *testing.T) {
	e := setupAttachGeometryFixture(t)

	argv := e.AttachArgv(0, 0)
	// The strands live in own windows, so an agent window is relevant and the bare prefix carries the chained select-window tail.
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

// TestAttachGeometry_ClientExitStatusOnSessionEndAndServerShutdown pins the tmux exit statuses the run window's close rule rests on:
// a client attached to a session that is killed while the server lives on exits 0, and a client attached to a session whose server is killed exits non-zero.
// A second session is created up front, so killing the first leaves the server running under tmux's exit-empty.
func TestAttachGeometry_ClientExitStatusOnSessionEndAndServerShutdown(t *testing.T) {
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
