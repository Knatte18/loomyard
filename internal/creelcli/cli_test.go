//go:build integration

// cli_test.go drives the `lyx creel` verbs through RunCLIIn over a hubforge hub, with the daemon run in process on a fake delivery and fake clocks.
// The rows set the strand name in the environment and replace the package's client seam, which is process-global state, so the test does not run in parallel.

package creelcli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lyxtemp"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
)

var epoch = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// isProbeWait and isSweepWait tell the delivery workers' waits (1 to 8 seconds) from the sweep's (30 seconds and up) on the daemon's clock.
func isProbeWait(d time.Duration) bool { return d <= 8*time.Second }

func isSweepWait(d time.Duration) bool { return d >= 30*time.Second }

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
	seam = clientSeam{clock: r.clientClock, spawn: func(string) error { return nil }}
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
		r := newRig(t)
		r.startDaemon(t)
		run := runCreel(r.cwd, "daemon", "--hub-path", r.hub.Path)
		if run.code != 0 || !strings.Contains(run.raw, "another daemon") {
			t.Errorf("creel daemon beside a live daemon = (%d, %q); want exit 0 saying another daemon serves the hub", run.code, run.raw)
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

		waiting := runAsync(r.cwd, "post", "--wait-reply", beta, "anyone?")
		r.awaitLogged(t, "anyone?")
		r.clock.AwaitWaitsWhere(1, isSweepWait)
		r.delivery.SetStrands(creelfake.LiveStrand(alpha))
		r.sweepStep()
		r.sweepLast()
		if returned := awaitRun(t, waiting); returned.code != ExitReturnedUndelivered {
			t.Errorf("creel post --wait-reply once the receiver left = (%d, %q); want exit %d", returned.code, returned.raw, ExitReturnedUndelivered)
		}
	})

	t.Run("as a strand read prints the message and acknowledges it", func(t *testing.T) {
		r := newRig(t)
		alpha := r.name("alpha:driver")
		t.Setenv(agentname.StrandNameEnv, alpha)
		strand := creelfake.LiveStrand(alpha)
		strand.PanePID = os.Getpid()
		r.delivery.SetStrands(strand)
		r.startDaemon(t)

		if post := runCreel(r.cwd, "post", alpha, "note to self"); post.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
		}
		read := runCreel(r.cwd, "read")
		messages, _ := read.fields["messages"].([]any)
		if read.code != 0 || len(messages) != 1 || !strings.Contains(read.raw, "note to self") {
			t.Fatalf("creel read = (%d, %q); want exit 0 with the one message", read.code, read.raw)
		}
		again := runCreel(r.cwd, "read")
		messages, _ = again.fields["messages"].([]any)
		if again.code != 0 || len(messages) != 0 || !strings.Contains(again.raw, "nothing unread") {
			t.Errorf("creel read after the acknowledged one = (%d, %q); want exit 0 saying nothing unread", again.code, again.raw)
		}
	})
}
