//go:build integration

// cli_test.go drives the `lyx creel` verbs through RunCLIIn over a hubforge hub, with the daemon run in process on a fake delivery and fake clocks.
// The rows set the strand name in the environment and replace the package's client seam, which is process-global state, so the test does not run in parallel.

package creelcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxtemp"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

var epoch = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// isProbeWait and isSweepWait tell the delivery workers' waits (1 to 8 seconds) from the sweep's (longer than the probe ceiling) on the daemon's clock.
func isProbeWait(d time.Duration) bool { return d <= 8*time.Second }

func isSweepWait(d time.Duration) bool { return d > 8*time.Second }

// cliRun is one verb's exit code and the JSON envelope it printed.
type cliRun struct {
	code   int
	fields map[string]any
	raw    string
}

// errorText is the envelope's error message, empty when it carries none.
func (r cliRun) errorText() string {
	text, _ := r.fields["error"].(string)
	return text
}

func runCreel(cwd string, args ...string) cliRun {
	var out bytes.Buffer
	code := RunCLIIn(cwd, &out, args)
	run := cliRun{code: code, raw: out.String()}
	_ = json.Unmarshal([]byte(strings.TrimSpace(run.raw)), &run.fields)
	return run
}

// lockedBuffer is a buffer a test reads while a verb in another goroutine still writes it.
type lockedBuffer struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String()
}

// runCreelSplit runs the creel command as runCreel does but writes stderr to the writer it is handed, and returns the stdout run.
func runCreelSplit(cwd string, stderr io.Writer, args ...string) cliRun {
	var out bytes.Buffer
	cmd := Command()
	cmd.SetOut(&out)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	code := clihelp.RunRootCtx(lyxcwd.WithCwd(context.Background(), cwd), cmd, &out)
	run := cliRun{code: code, raw: out.String()}
	_ = json.Unmarshal([]byte(strings.TrimSpace(run.raw)), &run.fields)
	return run
}

// failingWriter is an output that refuses every write.
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) { return 0, errors.New("write refused") }

// partialWriter is an output that takes the first half of its first write and then refuses it, and accepts and records every later write.
type partialWriter struct {
	mu      sync.Mutex
	data    bytes.Buffer
	written bool
}

func (w *partialWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.written {
		w.written = true
		n, _ := w.data.Write(p[:len(p)/2])
		return n, errors.New("write refused")
	}
	return w.data.Write(p)
}

func (w *partialWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.data.String()
}

// logHasEvent reports whether the log run lists an entry of the given event.
func logHasEvent(run cliRun, event string) bool {
	entries, _ := run.fields["log"].([]any)
	for _, entry := range entries {
		if fields, _ := entry.(map[string]any); fields["event"] == event {
			return true
		}
	}
	return false
}

// runCreelInto runs the creel command as runCreel does but prints to out and writes stderr to the writer it is handed, and returns the exit code.
func runCreelInto(cwd string, out, stderr io.Writer, args ...string) int {
	cmd := Command()
	cmd.SetOut(out)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	return clihelp.RunRootCtx(lyxcwd.WithCwd(context.Background(), cwd), cmd, out)
}

// awaitStderr polls the buffer until it holds want, under a real-time deadline that only bounds a failing test.
func awaitStderr(t *testing.T, stderr *lockedBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(stderr.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("stderr = %q; want it to hold %q", stderr.String(), want)
}

func runAsync(cwd string, args ...string) <-chan cliRun {
	results := make(chan cliRun, 1)
	go func() { results <- runCreel(cwd, args...) }()
	return results
}

// awaitRun waits for an async verb; the real-time deadline only bounds a failing test.
func awaitRun(t *testing.T, results <-chan cliRun) cliRun {
	t.Helper()
	select {
	case run := <-results:
		return run
	case <-time.After(10 * time.Second):
		t.Fatal("the verb did not return")
		return cliRun{}
	}
}

// rig is one hub with a daemon running in process over a fake delivery.
type rig struct {
	hub      *hubforge.Hub
	cwd      string
	geometry creelengine.Geometry
	delivery *creelfake.Delivery
	// clock is the daemon's clock, and clientClock the one the verbs' clients are built with.
	clock       *creelfake.Clock
	clientClock *creelfake.Clock
	// name returns a strand's full name in the hub.
	short string
}

func (r *rig) name(role string) string { return r.short + ":" + role }

// newRig builds a hub and replaces the client seam with a fake clock and a spawn that starts nothing.
func newRig(t *testing.T) *rig {
	t.Helper()
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	root, err := lyxtemp.ProcessRoot()
	if err != nil {
		t.Fatalf("ProcessRoot() = %v", err)
	}
	geometry, err := hubgeom.CreelGeometry(hub.Path, root.Path())
	if err != nil {
		t.Fatalf("CreelGeometry() = %v", err)
	}
	r := &rig{
		hub: hub, cwd: hub.PrimeWorktree(), geometry: geometry, short: geometry.Shortname,
		delivery: creelfake.NewDelivery(), clock: creelfake.NewClock(epoch), clientClock: creelfake.NewClock(epoch),
	}
	previous := seam
	seam = clientSeam{clock: r.clientClock, spawn: func(string, string) (creelengine.Spawn, error) { return creelengine.Spawn{}, nil }}
	t.Cleanup(func() { seam = previous })
	return r
}

// startDaemon runs the daemon in process on the rig's geometry and returns once it listens.
func (r *rig) startDaemon(t *testing.T) {
	t.Helper()
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- creelengine.Run(ctx, creelengine.Options{
			Geometry: r.geometry, Delivery: r.delivery, Clock: r.clock, PollClock: creelfake.NewClock(epoch),
			ParentPID: proc.ParentPID, GOOS: runtime.GOOS, Ready: func() { close(ready) },
		})
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("the daemon ended before it listened: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the daemon did not listen")
	}
}

// awaitLogged polls `creel log` until an event carries subject, which says the daemon journaled the message;
// no signal reaches the test, so it polls on a short tick under a real-time deadline.
func (r *rig) awaitLogged(t *testing.T, subject string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(runCreel(r.cwd, "log").raw, subject) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no log event carried %q", subject)
}

// sweepStep moves the daemon clock by the sweep's current wait and waits until it rearmed.
func (r *rig) sweepStep() {
	waits := r.clock.AwaitWaitsWhere(1, isSweepWait)
	r.clock.Advance(waits[len(waits)-1])
	r.clock.AwaitWaitsWhere(len(waits)+1, isSweepWait)
}

// sweepLast moves the daemon clock by the sweep's current wait for the sweep that ends the work.
func (r *rig) sweepLast() {
	waits := r.clock.AwaitWaitsWhere(1, isSweepWait)
	r.clock.Advance(waits[len(waits)-1])
}

func TestCreelCLI(t *testing.T) {
	// LYX_STRAND_NAME and the client seam are process-global, so no row runs in parallel.
	t.Setenv(agentname.StrandNameEnv, "")

	t.Run("every verb refuses a standalone repository naming that creel needs a hub", func(t *testing.T) {
		repo := t.TempDir()
		gitkit.Git(t, repo, "init", "-b", "main")
		verbs := [][]string{
			{"post", "r", "text"}, {"inject", "r", "text"}, {"interrupt", "r"}, {"forward", "1", "r"}, {"read"},
			{"inbox"}, {"log"}, {"hold", "r", "--for", "1m"}, {"release", "r"},
		}
		for _, args := range verbs {
			run := runCreel(repo, args...)
			if run.code != 1 || !strings.Contains(run.errorText(), "creel needs a hub") {
				t.Errorf("creel %v in a plain repository = (%d, %q); want exit 1 naming that creel needs a hub", args, run.code, run.raw)
			}
		}
	})

	t.Run("a subdirectory of a hub worktree passes the mode resolver's refusal through", func(t *testing.T) {
		r := newRig(t)
		sub := filepath.Join(r.cwd, "sub")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v", sub, err)
		}
		_, _, want := preflight.ResolveMode(sub)
		if want == nil {
			t.Fatal("ResolveMode of a hub subdirectory = nil error; the fixture does not exercise the refusal")
		}
		run := runCreel(sub, "inbox")
		if run.code != 1 || run.errorText() != want.Error() {
			t.Errorf("creel inbox in a hub subdirectory = (%d, %q); want exit 1 with %q", run.code, run.errorText(), want.Error())
		}
	})

	t.Run("daemon refuses an absent and a relative hub path", func(t *testing.T) {
		r := newRig(t)
		for _, args := range [][]string{{"daemon"}, {"daemon", "--hub-path", "relative/hub"}} {
			run := runCreel(r.cwd, args...)
			if run.code == 0 || !strings.Contains(run.raw, "hub-path") {
				t.Errorf("creel %v = (%d, %q); want a refusal naming --hub-path", args, run.code, run.raw)
			}
		}
	})

	t.Run("daemon beside a live daemon exits 0 with its message", func(t *testing.T) {
		// runDaemon redirects the process-global logger, so the capture restores it.
		logcapture.Capture(t)
		r := newRig(t)
		r.startDaemon(t)
		var stderr lockedBuffer
		run := runCreelSplit(r.cwd, &stderr, "daemon", "--hub-path", r.hub.Path)
		if run.code != 0 || !strings.Contains(run.raw, "another daemon") {
			t.Errorf("creel daemon beside a live daemon = (%d, %q); want exit 0 saying another daemon serves the hub", run.code, run.raw)
		}
		if !strings.HasPrefix(stderr.String(), fmt.Sprintf("creel daemon start pid=%d at=", os.Getpid())) {
			t.Errorf("daemon stderr = %q; want it to open with the start header of pid %d", stderr.String(), os.Getpid())
		}
	})

	t.Run("a daemon whose journal has a malformed middle line exits non-zero with its start header and a pid-tagged error line on stderr", func(t *testing.T) {
		logcapture.Capture(t)
		r := newRig(t)
		const posted = `{"event":"posted","at":"2026-10-01T11:00:00Z","id":%d,"from":"ly:BATTEN","to":"ly:alpha:driver","kind":"inject","body":"/clear","ttl_s":600}`
		if err := os.MkdirAll(r.geometry.JournalDir, 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v", err)
		}
		journal := fmt.Sprintf(posted, 1) + "\nnot json\n" + fmt.Sprintf(posted, 2) + "\n"
		if err := os.WriteFile(filepath.Join(r.geometry.JournalDir, "2026-10-01.jsonl"), []byte(journal), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v", err)
		}
		var stderr lockedBuffer
		run := runCreelSplit(r.cwd, &stderr, "daemon", "--hub-path", r.hub.Path)
		if run.code == 0 {
			t.Errorf("creel daemon over a malformed journal = (%d, %q); want a non-zero exit", run.code, run.raw)
		}
		lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
		if len(lines) < 2 || !strings.HasPrefix(lines[0], fmt.Sprintf("creel daemon start pid=%d at=", os.Getpid())) {
			t.Fatalf("daemon stderr = %q; want the start header of pid %d first", stderr.String(), os.Getpid())
		}
		tagged := lines[1]
		if !strings.HasPrefix(tagged, creelengine.DaemonLogPrefix(os.Getpid())) || !strings.Contains(tagged, "2026-10-01.jsonl") || !strings.Contains(tagged, "line 2") {
			t.Errorf("second stderr line = %q; want a pid-tagged line naming the journal file and line 2", tagged)
		}
	})

	t.Run("a pid line writer prefixes every line of a multi-line write and a line split across writes, each once", func(t *testing.T) {
		var out bytes.Buffer
		w := &pidLineWriter{dst: &out}
		for _, chunk := range []string{"one\ntwo\n", "thr", "ee\n"} {
			if n, err := w.Write([]byte(chunk)); err != nil || n != len(chunk) {
				t.Fatalf("Write(%q) = (%d, %v); want the whole chunk", chunk, n, err)
			}
		}
		prefix := creelengine.DaemonLogPrefix(os.Getpid())
		if want := prefix + "one\n" + prefix + "two\n" + prefix + "three\n"; out.String() != want {
			t.Errorf("pid line writer output = %q; want %q", out.String(), want)
		}
	})

	t.Run("a verb with no daemon fails after the capped retries naming the socket", func(t *testing.T) {
		r := newRig(t)
		done := make(chan struct{})
		defer close(done)
		go r.clientClock.AutoAdvance(done)
		run := runCreel(r.cwd, "inbox")
		if run.code != 1 || !strings.Contains(run.errorText(), r.geometry.SocketPath) {
			t.Errorf("creel inbox with no daemon = (%d, %q); want exit 1 naming %s", run.code, run.raw, r.geometry.SocketPath)
		}
	})

	t.Run("post refuses one argument without --reply and two with it", func(t *testing.T) {
		r := newRig(t)
		for _, args := range [][]string{{"post", "text"}, {"post", "--reply", "3", "a", "b"}} {
			run := runCreel(r.cwd, args...)
			if run.code != 1 || !strings.Contains(run.errorText(), "post <receiver> <text>") {
				t.Errorf("creel %v = (%d, %q); want exit 1 naming both forms", args, run.code, run.raw)
			}
		}
	})

	t.Run("as EXTERNAL the posting and waiting verbs report their outcomes", func(t *testing.T) {
		r := newRig(t)
		alpha, beta := r.name("alpha:driver"), r.name("beta:driver")
		r.delivery.SetStrands(creelfake.LiveStrand(alpha), creelfake.LiveStrand(beta))
		r.startDaemon(t)

		post := runCreel(r.cwd, "post", alpha, "hello")
		if id, _ := post.fields["id"].(float64); post.code != 0 || id < 1 {
			t.Errorf("creel post = (%d, %q); want exit 0 with the message id", post.code, post.raw)
		}

		injected := runCreel(r.cwd, "inject", "--wait", alpha, "/clear")
		if injected.code != 0 || injected.fields["outcome"] != string(creelengine.OutcomeInjected) {
			t.Errorf("creel inject --wait on a send that lands = (%d, %q); want exit 0 injected", injected.code, injected.raw)
		}
		external, err := agentname.FormatProgram(r.short, agentname.External)
		if err != nil {
			t.Fatalf("FormatProgram: %v", err)
		}
		if injected.fields["sender"] != external {
			t.Errorf("creel inject --wait printed sender %v; want %q", injected.fields["sender"], external)
		}

		r.delivery.SetSendError(creelengine.ErrNotLanded)
		unconfirmed := runCreel(r.cwd, "inject", "--wait", alpha, "/clear")
		if unconfirmed.code != ExitInjectUnconfirmed {
			t.Errorf("creel inject --wait on a send answering ErrNotLanded = (%d, %q); want exit %d", unconfirmed.code, unconfirmed.raw, ExitInjectUnconfirmed)
		}
		r.delivery.SetSendError(nil)

		r.delivery.SetIdle(alpha, false)
		probesBefore := len(r.clock.AwaitWaitsWhere(0, isProbeWait))
		expiring := runAsync(r.cwd, "inject", "--wait", "--ttl", "1m", alpha, "/clear")
		r.clock.AwaitWaitsWhere(probesBefore+1, isProbeWait)
		r.clock.Advance(61 * time.Second)
		if expired := awaitRun(t, expiring); expired.code != ExitInjectExpired {
			t.Errorf("creel inject --wait past its TTL = (%d, %q); want exit %d", expired.code, expired.raw, ExitInjectExpired)
		}

		timingOut := runAsync(r.cwd, "inject", "--wait", "--timeout", "5s", alpha, "/clear")
		r.clientClock.AwaitWaits(1)
		r.clientClock.Advance(5 * time.Second)
		if timedOut := awaitRun(t, timingOut); timedOut.code != ExitWaitTimeout || !strings.Contains(timedOut.errorText(), "no outcome arrived") {
			t.Errorf("creel inject --wait --timeout 5s = (%d, %q); want exit %d saying no outcome arrived", timedOut.code, timedOut.raw, ExitWaitTimeout)
		}

		// Busy, so the EXTERNAL mail is not carried into the pane and marked read before beta goes.
		r.delivery.SetIdle(beta, false)
		waiting := runAsync(r.cwd, "post", "--wait-reply", beta, "anyone?")
		r.awaitLogged(t, "anyone?")
		r.clock.AwaitWaitsWhere(1, isSweepWait)
		r.delivery.SetStrands(creelfake.LiveStrand(alpha))
		r.sweepStep()
		r.sweepLast()
		if returned := awaitRun(t, waiting); returned.code != ExitReturnedUndelivered {
			t.Errorf("creel post --wait-reply once the receiver left = (%d, %q); want exit %d", returned.code, returned.raw, ExitReturnedUndelivered)
		} else if returned.fields["sender"] != external {
			t.Errorf("creel post --wait-reply printed sender %v; want %q", returned.fields["sender"], external)
		}
	})

	t.Run("a waiting inject hears the stall on stderr and an expired answer carries it", func(t *testing.T) {
		r := newRig(t)
		alpha := r.name("alpha:driver")
		r.delivery.SetStrands(creelfake.LiveStrand(alpha))
		r.delivery.SetStall(alpha, creelengine.StallDraft)
		r.startDaemon(t)

		probesBefore := len(r.clock.AwaitWaitsWhere(0, isProbeWait))
		var stderr lockedBuffer
		done := make(chan cliRun, 1)
		returned := make(chan struct{})
		go func() {
			done <- runCreelSplit(r.cwd, &stderr, "inject", "--wait", "--ttl", "1m", alpha, "/clear")
			close(returned)
		}()
		r.clock.AwaitWaitsWhere(probesBefore+1, isProbeWait)
		r.clock.Advance(time.Second)
		awaitStderr(t, &stderr, "is stalled: "+creelengine.StallDraft.Reason())
		r.clock.AdvanceByWaitsWhere(isProbeWait, returned)
		expired := awaitRun(t, done)
		if expired.code != ExitInjectExpired {
			t.Fatalf("creel inject --wait past its TTL = (%d, %q); want exit %d", expired.code, expired.raw, ExitInjectExpired)
		}
		if expired.fields["stall"] != string(creelengine.StallDraft) || expired.fields["stall_reason"] != creelengine.StallDraft.Reason() || expired.fields["stall_at"] == nil {
			t.Errorf("expired envelope = %q; want stall draft with its reason and time", expired.raw)
		}
		if strings.Contains(expired.raw, "is stalled") {
			t.Errorf("stdout = %q; the progress line belongs on stderr", expired.raw)
		}
	})

	t.Run("a timed-out waiting inject carries the last stall beside the timeout", func(t *testing.T) {
		r := newRig(t)
		alpha := r.name("alpha:driver")
		r.delivery.SetStrands(creelfake.LiveStrand(alpha))
		r.delivery.SetStall(alpha, creelengine.StallTooShort)
		r.startDaemon(t)

		probesBefore := len(r.clock.AwaitWaitsWhere(0, isProbeWait))
		var stderr lockedBuffer
		done := make(chan cliRun, 1)
		go func() { done <- runCreelSplit(r.cwd, &stderr, "inject", "--wait", "--timeout", "5s", alpha, "/clear") }()
		r.clientClock.AwaitWaits(1)
		r.clock.AwaitWaitsWhere(probesBefore+1, isProbeWait)
		r.clock.Advance(time.Second)
		awaitStderr(t, &stderr, "is stalled: "+creelengine.StallTooShort.Reason())
		r.clientClock.Advance(5 * time.Second)
		timedOut := awaitRun(t, done)
		if timedOut.code != ExitWaitTimeout || timedOut.fields["stall"] != string(creelengine.StallTooShort) || timedOut.fields["stall_at"] == nil {
			t.Fatalf("creel inject --wait --timeout 5s = (%d, %q); want exit %d with the too-short stall", timedOut.code, timedOut.raw, ExitWaitTimeout)
		}
		if reason, _ := timedOut.fields["stall_reason"].(string); !strings.Contains(reason, "lyx reed attach") {
			t.Errorf("stall_reason = %q; want the way forward naming lyx reed attach", reason)
		}
	})

	t.Run("log shows a stalled event with its receiver and reason", func(t *testing.T) {
		r := newRig(t)
		alpha := r.name("alpha:driver")
		r.delivery.SetStrands(creelfake.LiveStrand(alpha))
		r.delivery.SetStall(alpha, creelengine.StallTooShort)
		r.startDaemon(t)

		if inject := runCreel(r.cwd, "inject", alpha, "/clear"); inject.code != 0 {
			t.Fatalf("creel inject = (%d, %q); want exit 0", inject.code, inject.raw)
		}
		r.awaitLogged(t, "stalled")
		var stalled []map[string]any
		entries, _ := runCreel(r.cwd, "log").fields["log"].([]any)
		for _, entry := range entries {
			if fields, _ := entry.(map[string]any); fields["event"] == "stalled" {
				stalled = append(stalled, fields)
			}
		}
		if len(stalled) != 1 || stalled[0]["to"] != alpha || stalled[0]["reason"] != string(creelengine.StallTooShort) {
			t.Errorf("creel log stalled events = %v; want one to %s with reason %q", stalled, alpha, creelengine.StallTooShort)
		}
	})

	t.Run("log --id prints a carried entry's kind", func(t *testing.T) {
		r := newRig(t)
		alpha, beta := r.name("alpha:driver"), r.name("beta:driver")
		r.delivery.SetStrands(creelfake.LiveStrand(alpha), creelfake.DormantStrand(beta))
		if err := os.MkdirAll(r.geometry.JournalDir, 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v", err)
		}
		carried := fmt.Sprintf(`{"event":"carried","at":"2026-10-01T11:00:00Z","of":"posted","id":1,"from":%q,"to":%q,"body":"old mail","orig_at":"2026-09-20T11:00:00Z"}`, alpha, beta)
		if err := os.WriteFile(filepath.Join(r.geometry.JournalDir, "2026-10-01.jsonl"), []byte(carried+"\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v", err)
		}
		r.startDaemon(t)

		entries, _ := runCreel(r.cwd, "log", "--id", "1").fields["log"].([]any)
		if len(entries) != 1 {
			t.Fatalf("creel log --id 1 listed %d entries; want the carried one", len(entries))
		}
		if fields, _ := entries[0].(map[string]any); fields["event"] != "carried" || fields["kind"] != "mail" {
			t.Errorf("creel log --id 1 entry = %v; want event carried with kind mail", entries[0])
		}
	})

	t.Run("as a strand read prints the messages as text, or as JSON with --envelope, and acknowledges them", func(t *testing.T) {
		r := newRig(t)
		alpha := r.name("alpha:driver")
		t.Setenv(agentname.StrandNameEnv, alpha)
		strand := creelfake.LiveStrand(alpha)
		strand.PanePID = os.Getpid()
		r.delivery.SetStrands(strand)
		r.startDaemon(t)

		for _, body := range []string{"note to self", "and another"} {
			if post := runCreel(r.cwd, "post", alpha, body); post.code != 0 {
				t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
			}
		}
		r.clientClock.Advance(6 * time.Minute)
		read := runCreel(r.cwd, "read")
		posted := epoch.Format(time.RFC3339)
		want := fmt.Sprintf("#1 from %[1]s, posted %[2]s — reply with: lyx creel post --reply 1 <text>\nnote to self\n\n#2 from %[1]s, posted %[2]s — reply with: lyx creel post --reply 2 <text>\nand another\n", alpha, posted)
		if read.code != 0 || read.raw != want {
			t.Fatalf("creel read = (%d, %q); want exit 0 with %q", read.code, read.raw, want)
		}
		if post := runCreel(r.cwd, "post", alpha, "as json"); post.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
		}
		asJSON := runCreel(r.cwd, "read", "--envelope")
		messages, _ := asJSON.fields["messages"].([]any)
		if asJSON.code != 0 || len(messages) != 1 {
			t.Fatalf("creel read --envelope = (%d, %q); want exit 0 with one message", asJSON.code, asJSON.raw)
		}
		if message, _ := messages[0].(map[string]any); message["body"] != "as json" || message["from"] != alpha {
			t.Fatalf("creel read --envelope = (%d, %q); want exit 0 with the one message's fields", asJSON.code, asJSON.raw)
		}
		if again := runCreel(r.cwd, "read"); again.code != 0 || again.raw != "nothing unread\n" {
			t.Errorf("creel read after the acknowledged ones = (%d, %q); want exit 0 printing nothing unread", again.code, again.raw)
		}
	})

	t.Run("log shows a reminded event with its unread ids and step and no id", func(t *testing.T) {
		r := newRig(t)
		alpha := r.name("alpha:driver")
		// Posted from alpha's own pane, since only an agent strand's mail is reminded rather than carried.
		t.Setenv(agentname.StrandNameEnv, alpha)
		strand := creelfake.LiveStrand(alpha)
		strand.PanePID = os.Getpid()
		r.delivery.SetStrands(strand)
		r.startDaemon(t)

		post := runCreel(r.cwd, "post", alpha, "hello")
		if post.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
		}
		r.awaitLogged(t, "reminded")
		r.clock.AwaitWaitsWhere(1, isProbeWait)
		r.clock.Advance(5 * time.Minute)
		r.awaitLogged(t, `"step":1`)
		var reminded map[string]any
		entries, _ := runCreel(r.cwd, "log").fields["log"].([]any)
		for _, entry := range entries {
			if fields, _ := entry.(map[string]any); fields["event"] == "reminded" && fields["step"] != nil {
				reminded = fields
			}
		}
		if reminded == nil {
			t.Fatalf("creel log holds no reminded event with a step: %v", entries)
		}
		if _, hasID := reminded["id"]; hasID {
			t.Errorf("reminded event = %v; want no id", reminded)
		}
		if unread, _ := reminded["unread"].([]any); len(unread) != 1 || unread[0] != post.fields["id"] || reminded["step"] != float64(1) {
			t.Errorf("reminded event = %v; want unread [%v] at step 1", reminded, post.fields["id"])
		}
	})

	t.Run("read into an output that cannot be written exits non-zero and acknowledges nothing", func(t *testing.T) {
		r := newRig(t)
		alpha := r.name("alpha:driver")
		t.Setenv(agentname.StrandNameEnv, alpha)
		strand := creelfake.LiveStrand(alpha)
		strand.PanePID = os.Getpid()
		r.delivery.SetStrands(strand)
		r.startDaemon(t)

		var emptyStderr lockedBuffer
		if code := runCreelInto(r.cwd, failingWriter{}, &emptyStderr, "read"); code != 1 || !strings.HasPrefix(emptyStderr.String(), "creel: ") {
			t.Errorf("creel read of nothing unread into a failing writer = (%d, %q); want exit 1 and a creel: line on stderr", code, emptyStderr.String())
		}
		if post := runCreel(r.cwd, "post", alpha, "note to self"); post.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
		}
		var messageStderr lockedBuffer
		code := runCreelInto(r.cwd, failingWriter{}, &messageStderr, "read")
		if text := messageStderr.String(); code != 1 || !strings.HasPrefix(text, "creel: ") || !strings.Contains(text, "write refused") || !strings.Contains(text, "unread") {
			t.Errorf("creel read of a message into a failing writer = (%d, %q); want exit 1 and a creel: line holding the write's error and unread", code, text)
		}
		// The daemon releases the dropped read's reservation only once it sees the connection close, so the retry runs under a real-time deadline.
		deadline := time.Now().Add(10 * time.Second)
		for {
			read := runCreel(r.cwd, "read", "--envelope")
			if messages, _ := read.fields["messages"].([]any); read.code == 0 && len(messages) == 1 && strings.Contains(read.raw, "note to self") {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("creel read after the failed write = (%d, %q); want the message still unread", read.code, read.raw)
			}
			time.Sleep(10 * time.Millisecond)
		}
	})

	t.Run("a waiting verb's envelope that cannot be written keeps its exit code and goes to stderr", func(t *testing.T) {
		r := newRig(t)
		alpha := r.name("alpha:driver")
		beta := r.name("beta:driver")
		r.delivery.SetStrands(creelfake.LiveStrand(alpha), creelfake.LiveStrand(beta))
		r.startDaemon(t)

		r.delivery.SetSendError(creelengine.ErrNotLanded)
		var writtenStderr lockedBuffer
		written := runCreelSplit(r.cwd, &writtenStderr, "inject", "--wait", alpha, "/clear")
		if _, hasDone := written.fields["done"]; written.code != ExitInjectUnconfirmed || writtenStderr.String() != "" || written.fields["error"] != "the text was typed but its landing was not confirmed" || hasDone {
			t.Errorf("creel inject --wait on a send answering ErrNotLanded = (%d, %q, stderr %q); want exit %d, the unconfirmed error, no done key and an empty stderr", written.code, written.raw, writtenStderr.String(), ExitInjectUnconfirmed)
		}
		var unconfirmedStderr lockedBuffer
		code := runCreelInto(r.cwd, failingWriter{}, &unconfirmedStderr, "inject", "--wait", alpha, "/clear")
		line := strings.TrimSuffix(unconfirmedStderr.String(), "\n")
		unconfirmed := regexp.MustCompile(`^creel: inject #(\d+) was queued; the text was typed but its landing was not confirmed$`).FindStringSubmatch(line)
		if code != ExitInjectUnconfirmed || unconfirmed == nil {
			t.Fatalf("creel inject --wait on a send answering ErrNotLanded into a failing writer = (%d, %q); want exit %d and a creel: line naming the queued inject and the unconfirmed landing", code, line, ExitInjectUnconfirmed)
		}
		if !logHasEvent(runCreel(r.cwd, "log", "--id", unconfirmed[1]), "posted") {
			t.Errorf("creel log --id %s lists no posted event; want the unconfirmed inject stored", unconfirmed[1])
		}
		r.delivery.SetSendError(nil)

		r.delivery.SetStall(alpha, creelengine.StallDraft)
		probesBefore := len(r.clock.AwaitWaitsWhere(0, isProbeWait))
		var expiredStderr lockedBuffer
		expiredCodes := make(chan int, 1)
		returned := make(chan struct{})
		go func() {
			expiredCodes <- runCreelInto(r.cwd, failingWriter{}, &expiredStderr, "inject", "--wait", "--ttl", "1m", alpha, "/clear")
			close(returned)
		}()
		r.clock.AwaitWaitsWhere(probesBefore+1, isProbeWait)
		r.clock.Advance(time.Second)
		awaitStderr(t, &expiredStderr, "is stalled: "+creelengine.StallDraft.Reason())
		r.clock.AdvanceByWaitsWhere(isProbeWait, returned)
		select {
		case code := <-expiredCodes:
			lines := strings.Split(strings.TrimSuffix(expiredStderr.String(), "\n"), "\n")
			for _, note := range lines[:len(lines)-1] {
				if !strings.Contains(note, "is stalled: ") {
					t.Errorf("stderr line %q before the answer; want only stall notes", note)
				}
			}
			if last := lines[len(lines)-1]; code != ExitInjectExpired || !regexp.MustCompile(`^creel: inject #(\d+) was queued; the inject expired before it could be typed$`).MatchString(last) {
				t.Errorf("creel inject --wait past its TTL into a failing writer = (%d, last line %q); want exit %d and a creel: line naming the queued inject and the expiry", code, last, ExitInjectExpired)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the expired inject did not return")
		}
		r.delivery.SetStall(alpha, "")

		r.delivery.SetIdle(alpha, false)
		var timeoutStderr lockedBuffer
		codes := make(chan int, 1)
		go func() {
			codes <- runCreelInto(r.cwd, failingWriter{}, &timeoutStderr, "inject", "--wait", "--timeout", "5s", alpha, "/clear")
		}()
		r.clientClock.AwaitWaits(1)
		r.clientClock.Advance(5 * time.Second)
		select {
		case code := <-codes:
			if text := timeoutStderr.String(); code != ExitWaitTimeout || text != "creel: no outcome arrived before the timeout of 5s\n" {
				t.Errorf("creel inject --wait --timeout 5s into a failing writer = (%d, %q); want exit %d and a creel: line naming the timeout and no id", code, text, ExitWaitTimeout)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the verb did not return")
		}

		// Busy, so the EXTERNAL mail is not carried into the pane and marked read before beta goes.
		r.delivery.SetIdle(beta, false)
		var returnedStderr lockedBuffer
		returnedCodes := make(chan int, 1)
		go func() {
			returnedCodes <- runCreelInto(r.cwd, failingWriter{}, &returnedStderr, "post", "--wait-reply", beta, "anyone?")
		}()
		r.awaitLogged(t, "anyone?")
		r.clock.AwaitWaitsWhere(1, isSweepWait)
		r.delivery.SetStrands(creelfake.LiveStrand(alpha))
		r.sweepStep()
		r.sweepLast()
		select {
		case code := <-returnedCodes:
			line := strings.TrimSuffix(returnedStderr.String(), "\n")
			if code != ExitReturnedUndelivered || !regexp.MustCompile(`^creel: message #(\d+) was posted; the message returned undelivered$`).MatchString(line) {
				t.Errorf("creel post --wait-reply returned undelivered into a failing writer = (%d, %q); want exit %d and a creel: line naming the posted message", code, line, ExitReturnedUndelivered)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the returned post did not return")
		}
	})

	t.Run("a verb whose answer cannot be written exits 1 with one creel: line naming what it did", func(t *testing.T) {
		r := newRig(t)
		alpha, beta := r.name("alpha:driver"), r.name("beta:driver")
		r.delivery.SetStrands(creelfake.LiveStrand(alpha), creelfake.LiveStrand(beta))
		r.startDaemon(t)

		lost := func(args ...string) (int, string) {
			var stderr lockedBuffer
			code := runCreelInto(r.cwd, failingWriter{}, &stderr, args...)
			return code, strings.TrimSuffix(stderr.String(), "\n")
		}
		wantLine := func(args []string, want string) {
			t.Helper()
			if code, line := lost(args...); code != 1 || line != want {
				t.Errorf("creel %v into a failing writer = (%d, %q); want exit 1 and %q", args, code, line, want)
			}
		}
		wantIDLine := func(args []string, pattern string) string {
			t.Helper()
			code, line := lost(args...)
			match := regexp.MustCompile(pattern).FindStringSubmatch(line)
			if code != 1 || match == nil {
				t.Errorf("creel %v into a failing writer = (%d, %q); want exit 1 and a line matching %s", args, code, line, pattern)
				return ""
			}
			return match[1]
		}
		wantStored := func(id string) {
			t.Helper()
			if !logHasEvent(runCreel(r.cwd, "log", "--id", id), "posted") {
				t.Errorf("creel log --id %s lists no posted event; want the stored message or inject", id)
			}
		}

		var writtenStderr lockedBuffer
		if run := runCreelSplit(r.cwd, &writtenStderr, "post", alpha, "seen"); run.code != 0 || strings.Contains(writtenStderr.String(), "creel: ") {
			t.Errorf("creel post = (%d, %q, stderr %q); want exit 0 and no creel: line", run.code, run.raw, writtenStderr.String())
		}
		wantStored(wantIDLine([]string{"post", alpha, "hello"}, `^creel: message #(\d+) was posted, but its answer could not be written: write refused$`))
		wantStored(wantIDLine([]string{"inject", alpha, "/clear"}, `^creel: inject #(\d+) was queued, but its answer could not be written: write refused$`))
		wantLine([]string{"interrupt", alpha}, "creel: the interrupt took effect, but its answer could not be written: write refused")
		r.delivery.SetInterruptError(creelengine.ErrNoRunningTurn)
		wantLine([]string{"interrupt", alpha}, "creel: the answer could not be written: write refused")
		r.delivery.SetInterruptError(nil)
		wantLine([]string{"hold", alpha, "--for", "1m"}, "creel: the hold took effect, but its answer could not be written: write refused")
		if !logHasEvent(runCreel(r.cwd, "log", "--to", alpha), "held") {
			t.Errorf("creel log --to %s lists no held event; want the hold to stand", alpha)
		}
		wantLine([]string{"release", alpha}, "creel: the release took effect, but its answer could not be written: write refused")
		wantLine([]string{"log"}, "creel: the answer could not be written: write refused")

		if code := runCreelInto(r.cwd, failingWriter{}, failingWriter{}, "log"); code != 1 {
			t.Errorf("creel log into failing stdout and stderr = exit %d; want exit 1", code)
		}

		var partial partialWriter
		var partialStderr lockedBuffer
		code := runCreelInto(r.cwd, &partial, &partialStderr, "log")
		if line := partialStderr.String(); code != 1 || line != "creel: the answer could not be written: write refused\n" {
			t.Errorf("creel log into a partial writer = (%d, %q); want exit 1 and the lost-answer line with the write's error", code, line)
		}
		if got := partial.String(); got == "" || strings.Contains(got, `"ok":false`) || strings.Contains(got, "\n") {
			t.Errorf("partial writer holds %q; want only the half line it took, with no error envelope after it", got)
		}

		t.Setenv(agentname.StrandNameEnv, alpha)
		self := creelfake.LiveStrand(alpha)
		self.PanePID = os.Getpid()
		r.delivery.SetStrands(self, creelfake.LiveStrand(beta))
		wantLine([]string{"inbox"}, "creel: the answer could not be written: write refused")
		note := runCreel(r.cwd, "post", alpha, "note to self")
		if note.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", note.code, note.raw)
		}
		if read := runCreel(r.cwd, "read"); read.code != 0 {
			t.Fatalf("creel read = (%d, %q); want exit 0", read.code, read.raw)
		}
		noteID := fmt.Sprint(note.fields["id"])
		replyID := wantIDLine([]string{"post", "--reply", noteID, "answer"}, `^creel: message #(\d+) was posted, but its answer could not be written: write refused$`)
		if replyID == noteID {
			t.Errorf("the reply is message #%s, the replied one; want a new message", replyID)
		}
		wantStored(replyID)
		pass := runCreel(r.cwd, "post", alpha, "pass on")
		if pass.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", pass.code, pass.raw)
		}
		wantLine([]string{"forward", fmt.Sprint(pass.fields["id"]), beta}, "creel: the forward took effect, but its answer could not be written: write refused")
	})

	t.Run("a refusal into an output that cannot be written prints the creel: prefix once", func(t *testing.T) {
		r := newRig(t)
		alpha := r.name("alpha:driver")
		r.delivery.SetStrands(creelfake.LiveStrand(alpha))
		r.startDaemon(t)
		plainRepo := t.TempDir()
		gitkit.Git(t, plainRepo, "init", "-b", "main")

		tests := []struct {
			name string
			cwd  string
			args []string
		}{
			{"UnknownReceiver", r.cwd, []string{"post", r.name("nobody:driver"), "text"}},
			{"ExternalInbox", r.cwd, []string{"inbox"}},
			{"NeedsAHub", plainRepo, []string{"inbox"}},
			{"PostArguments", r.cwd, []string{"post", "text"}},
		}
		for _, tt := range tests {
			var stderr lockedBuffer
			code := runCreelInto(tt.cwd, failingWriter{}, &stderr, tt.args...)
			line := strings.TrimSuffix(stderr.String(), "\n")
			if code != 1 || strings.Contains(line, "\n") || !strings.HasPrefix(line, "creel: ") || strings.Count(line, "creel: ") != 1 {
				t.Errorf("%s: creel %v into a failing writer = (%d, %q); want exit 1 and one line opening with creel: once", tt.name, tt.args, code, stderr.String())
			}
		}
	})

	t.Run("read shows the receiver a message was returned from", func(t *testing.T) {
		r := newRig(t)
		alpha, beta := r.name("alpha:driver"), r.name("beta:driver")
		t.Setenv(agentname.StrandNameEnv, alpha)
		strand := creelfake.LiveStrand(alpha)
		strand.PanePID = os.Getpid()
		r.delivery.SetStrands(strand, creelfake.LiveStrand(beta))
		r.startDaemon(t)

		if post := runCreel(r.cwd, "post", beta, "anyone?"); post.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
		}
		r.awaitLogged(t, "anyone?")
		r.clock.AwaitWaitsWhere(1, isSweepWait)
		r.delivery.SetStrands(strand)
		r.sweepStep()
		r.sweepLast()
		deadline := time.Now().Add(10 * time.Second)
		for {
			read := runCreel(r.cwd, "read", "--envelope")
			messages, _ := read.fields["messages"].([]any)
			if len(messages) == 1 {
				message, _ := messages[0].(map[string]any)
				if message["ended_unread_by"] != beta || message["ended_unread_at"] == nil {
					t.Errorf("returned message = %v; want ended_unread_by %s and an ended_unread_at", message, beta)
				}
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("creel read after the sweeps = (%d, %q); want the returned message", read.code, read.raw)
			}
			time.Sleep(10 * time.Millisecond)
		}
	})

	t.Run("the daemon log is appended to, truncated over its cap, and a log that cannot be opened is a nil file with a Warn", func(t *testing.T) {
		captured := logcapture.Capture(t)
		dir := t.TempDir()

		small := filepath.Join(dir, "state", "daemon.log")
		if err := os.MkdirAll(filepath.Dir(small), 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v", err)
		}
		if err := os.WriteFile(small, []byte("earlier line\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v", err)
		}
		file, err := openDaemonLog(small)
		if err != nil || file == nil {
			t.Fatalf("openDaemonLog(small) = (%v, %v); want a file", file, err)
		}
		_, _ = file.WriteString("new line\n")
		_ = file.Close()
		if got, _ := os.ReadFile(small); string(got) != "earlier line\nnew line\n" {
			t.Errorf("small log = %q; want the new line appended", got)
		}

		big := filepath.Join(dir, "big.log")
		if err := os.WriteFile(big, bytes.Repeat([]byte("x"), daemonLogCap+1), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v", err)
		}
		file, err = openDaemonLog(big)
		if err != nil || file == nil {
			t.Fatalf("openDaemonLog(big) = (%v, %v); want a file", file, err)
		}
		_, _ = file.WriteString("fresh\n")
		_ = file.Close()
		if got, _ := os.ReadFile(big); string(got) != "fresh\n" {
			t.Errorf("big log = %d bytes; want it truncated before the append", len(got))
		}

		blocked := filepath.Join(small, "inner", "daemon.log")
		file, err = openDaemonLog(blocked)
		if file != nil || err == nil {
			t.Errorf("openDaemonLog(parent is a file) = (%v, %v); want a nil file and an error", file, err)
		}
		if !strings.Contains(captured.String(), "the daemon log cannot be opened") {
			t.Errorf("logger output = %q; want a Warn naming the daemon log", captured.String())
		}
	})

	t.Run("the spawned daemon's reap logs its exit and reads exit 0 as a lost start race", func(t *testing.T) {
		tests := []struct {
			name       string
			script     string
			wantLost   bool
			wantStatus string
		}{
			{"ExitZeroIsALostRace", "exit 0", true, "status=0"},
			{"ExitOneIsAFailedStart", "exit 1", false, "status=1"},
			{"KilledBySignalIsNoLostRace", "kill -9 $$", false, "status=-1"},
		}
		for _, tt := range tests {
			captured := logcapture.CaptureVerbose(t)
			cmd := exec.Command("sh", "-c", tt.script)
			if err := cmd.Start(); err != nil {
				t.Fatalf("%s: start sh: %v", tt.name, err)
			}
			pid := cmd.Process.Pid
			select {
			case exit := <-reapDaemon(cmd):
				if exit.LostRace != tt.wantLost {
					t.Errorf("%s: LostRace = %v; want %v", tt.name, exit.LostRace, tt.wantLost)
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("%s: the reap sent no exit", tt.name)
			}
			for _, want := range []string{fmt.Sprintf("pid=%d", pid), tt.wantStatus} {
				if !strings.Contains(captured.String(), want) {
					t.Errorf("%s: logger output = %q; want it to hold %q", tt.name, captured.String(), want)
				}
			}
		}
	})

	// The Go client rows make the test process the pane of the hub's orch strand, the one pane that may also claim a program name.
	orchRig := func(t *testing.T) (r *rig, orch string, location *lyxcwd.Location) {
		t.Helper()
		r = newRig(t)
		orch = r.name("orch")
		t.Setenv(agentname.StrandNameEnv, orch)
		strand := creelfake.LiveStrand(orch)
		strand.PanePID = os.Getpid()
		r.delivery.SetStrands(strand)
		r.startDaemon(t)
		location, err := lyxcwd.Resolve(r.cwd)
		if err != nil {
			t.Fatalf("Resolve(%q) = %v", r.cwd, err)
		}
		return r, orch, location
	}

	t.Run("a program client signs as the program whatever the environment holds and its mail reaches the strand", func(t *testing.T) {
		r, orch, location := orchRig(t)
		program, err := NewProgramClient(location, "BATTEN")
		if err != nil {
			t.Fatalf("NewProgramClient() = %v", err)
		}
		if program.Shortname() != r.short {
			t.Errorf("Shortname() = %q; want %q", program.Shortname(), r.short)
		}
		// Busy, so the delivery does not carry the program's mail into the pane and mark it read before the strand reads it.
		r.delivery.SetIdle(orch, false)
		if _, err := program.Post(orch, "from the program\nsecond line"); err != nil {
			t.Fatalf("Post() = %v", err)
		}
		strand, err := NewStrandClient(location, orch)
		if err != nil {
			t.Fatalf("NewStrandClient() = %v", err)
		}
		var read []creelengine.Message
		if err := strand.ReadAll(func(messages []creelengine.Message) error {
			read = messages
			return nil
		}); err != nil {
			t.Fatalf("ReadAll() = %v", err)
		}
		if len(read) != 1 || read[0].Body != "from the program\nsecond line" || read[0].From != r.short+":BATTEN" {
			t.Errorf("the strand read %+v; want one message from %s:BATTEN", read, r.short)
		}
	})

	t.Run("a program client's command inject is typed through the delivery and the log shows its command and argument", func(t *testing.T) {
		r, orch, location := orchRig(t)
		program, err := NewProgramClient(location, "TENDER")
		if err != nil {
			t.Fatalf("NewProgramClient() = %v", err)
		}
		command := creelengine.Command{Name: creelengine.CommandCompact, Argument: "keep the plan"}
		id, err := program.InjectCommand(orch, command, 0)
		if err != nil {
			t.Fatalf("InjectCommand() = %v", err)
		}
		r.clock.AwaitWaitsWhere(1, isProbeWait)
		if got, want := r.delivery.Commands(), []creelfake.CommandCall{{Strand: orch, Command: command}}; !reflect.DeepEqual(got, want) {
			t.Errorf("Commands = %+v; want %+v", got, want)
		}
		run := runCreel(r.cwd, "log", "--id", strconv.FormatInt(id, 10))
		if !strings.Contains(run.raw, `"command":"compact"`) || !strings.Contains(run.raw, `"argument":"keep the plan"`) {
			t.Errorf("creel log --id %d = %q; want the command and argument", id, run.raw)
		}
		entries, err := program.Log(creelengine.LogFilter{ID: id})
		if err != nil || len(entries) == 0 || entries[0].Command != creelengine.CommandCompact {
			t.Errorf("Log() = %+v, %v; want the inject's entries carrying its command", entries, err)
		}
	})

	t.Run("a program client's ReadAll with a failing handler leaves the reply unread for the next ReadAll", func(t *testing.T) {
		r, orch, location := orchRig(t)
		program, err := NewProgramClient(location, "BATTEN")
		if err != nil {
			t.Fatalf("NewProgramClient() = %v", err)
		}
		// A program's mailbox is known once it has read.
		if err := program.ReadAll(func([]creelengine.Message) error { return nil }); err != nil {
			t.Fatalf("first ReadAll() = %v", err)
		}
		strand, err := NewStrandClient(location, orch)
		if err != nil {
			t.Fatalf("NewStrandClient() = %v", err)
		}
		if _, err := strand.Post(r.short+":BATTEN", "the reply"); err != nil {
			t.Fatalf("Post() = %v", err)
		}
		refused := errors.New("handler refused")
		if err := program.ReadAll(func([]creelengine.Message) error { return refused }); !errors.Is(err, refused) {
			t.Fatalf("ReadAll() with a failing handler = %v; want %v", err, refused)
		}
		// The daemon releases the reservation once it sees the connection close, which no signal reaches the test, so it retries on a short tick under a real-time deadline.
		var read []creelengine.Message
		deadline := time.Now().Add(10 * time.Second)
		for len(read) == 0 && time.Now().Before(deadline) {
			if err := program.ReadAll(func(messages []creelengine.Message) error {
				read = messages
				return nil
			}); err != nil {
				t.Fatalf("second ReadAll() = %v", err)
			}
			time.Sleep(10 * time.Millisecond)
		}
		if len(read) != 1 || read[0].Body != "the reply" {
			t.Errorf("second ReadAll handed %+v; want the unread reply", read)
		}
	})

	t.Run("a program client's withdraw removes its queued inject and the log names the reason", func(t *testing.T) {
		r, orch, location := orchRig(t)
		r.delivery.SetIdle(orch, false)
		program, err := NewProgramClient(location, "BATTEN")
		if err != nil {
			t.Fatalf("NewProgramClient() = %v", err)
		}
		id, err := program.Inject(orch, "/clear", 0)
		if err != nil {
			t.Fatalf("Inject() = %v", err)
		}
		got, err := program.Withdraw(orch)
		if err != nil || !slices.Equal(got.Injects, []int64{id}) {
			t.Fatalf("Withdraw() = %+v, %v; want the inject %d", got, err, id)
		}
		if run := runCreel(r.cwd, "log", "--id", strconv.FormatInt(id, 10)); !strings.Contains(run.raw, `"reason":"withdrawn"`) {
			t.Errorf("creel log --id %d = %q; want the withdrawn reason", id, run.raw)
		}
	})

	t.Run("every Go client call against a daemon that never answers times out at goCallTimeout", func(t *testing.T) {
		calls := map[string]func(*GoClient) error{
			"Post":    func(c *GoClient) error { _, err := c.Post("tst:orch", "hello"); return err },
			"ReadAll": func(c *GoClient) error { return c.ReadAll(func([]creelengine.Message) error { return nil }) },
			"Inject":  func(c *GoClient) error { _, err := c.Inject("tst:orch", "text", 0); return err },
			"InjectCommand": func(c *GoClient) error {
				_, err := c.InjectCommand("tst:orch", creelengine.Command{Name: creelengine.CommandClear}, 0)
				return err
			},
			"Log":      func(c *GoClient) error { _, err := c.Log(creelengine.LogFilter{}); return err },
			"Hold":     func(c *GoClient) error { return c.Hold("tst:orch", time.Minute) },
			"Release":  func(c *GoClient) error { _, err := c.Release("tst:orch"); return err },
			"Withdraw": func(c *GoClient) error { _, err := c.Withdraw("tst:orch"); return err },
		}
		for name, call := range calls {
			r := newRig(t)
			listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: r.geometry.SocketPath, Net: "unix"})
			if err != nil {
				t.Fatalf("%s: listen: %v", name, err)
			}
			var held []net.Conn
			var heldMu sync.Mutex
			go func() {
				for {
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					heldMu.Lock()
					held = append(held, conn)
					heldMu.Unlock()
				}
			}()
			t.Cleanup(func() {
				_ = listener.Close()
				heldMu.Lock()
				defer heldMu.Unlock()
				for _, conn := range held {
					_ = conn.Close()
				}
			})
			location, err := lyxcwd.Resolve(r.cwd)
			if err != nil {
				t.Fatalf("Resolve(%q) = %v", r.cwd, err)
			}
			program, err := NewProgramClient(location, "BATTEN")
			if err != nil {
				t.Fatalf("NewProgramClient() = %v", err)
			}
			done := make(chan error, 1)
			go func() { done <- call(program) }()
			r.clientClock.AwaitWaits(1)
			r.clientClock.Advance(goCallTimeout)
			select {
			case got := <-done:
				if !errors.Is(got, creelengine.ErrWaitTimeout) {
					t.Errorf("%s = %v; want ErrWaitTimeout once goCallTimeout elapsed", name, got)
				}
			case <-time.After(10 * time.Second):
				t.Errorf("%s still waiting after goCallTimeout elapsed on the client's clock", name)
			}
		}
	})

	t.Run("a Go client on a location outside a hub is refused naming the way forward", func(t *testing.T) {
		for name, build := range map[string]func(*lyxcwd.Location) (*GoClient, error){
			"program": func(l *lyxcwd.Location) (*GoClient, error) { return NewProgramClient(l, "BATTEN") },
			"strand":  func(l *lyxcwd.Location) (*GoClient, error) { return NewStrandClient(l, "ly:orch") },
		} {
			for _, location := range []*lyxcwd.Location{nil, {}} {
				if _, err := build(location); err == nil || !strings.Contains(err.Error(), "way forward") {
					t.Errorf("%s client on %+v = %v; want a refusal naming the way forward", name, location, err)
				}
			}
		}
	})
}
