// server_test.go pins the daemon over a real socket: the protocol, identity from the peer, the single-instance lifecycle and the waiting verbs across a restart.

package creelengine_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lyxtemp"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
)

// rig is one hub's geometry, a fake delivery and a fake daemon clock; a test starts daemons over it.
type rig struct {
	t     *testing.T
	geom  creelengine.Geometry
	del   *creelfake.Delivery
	clock *creelfake.Clock
	// pollClock times the daemon's poll apart from the delivery's waits on clock.
	pollClock *creelfake.Clock
	replaced  atomic.Bool
	started   atomic.Int32
	spawns    atomic.Int32
	// serving is closed when the first daemon of the rig listens.
	serving     chan struct{}
	servingOnce sync.Once
}

// newRig returns a rig over a socket in the process run directory.
// With asStrand the test process is the pane process of alpha, so its calls resolve to that strand; otherwise no strand's pane holds it and its calls are EXTERNAL.
func newRig(t *testing.T, asStrand bool) *rig {
	t.Helper()
	dir, err := lyxtemp.MkdirTemp("c")
	if err != nil {
		t.Fatalf("lyxtemp.MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	state := filepath.Join(dir, "state")
	r := &rig{
		t: t,
		geom: creelengine.Geometry{
			HubPath: filepath.Join(dir, "hub"), Shortname: "ly", StateDir: state,
			JournalDir: filepath.Join(state, "journal"), LockPath: filepath.Join(state, "daemon.lock"),
			SocketPath: filepath.Join(dir, "s.sock"), DaemonLogPath: filepath.Join(state, "daemon.log"),
		},
		del:       creelfake.NewDelivery(),
		clock:     creelfake.NewClock(epoch),
		pollClock: creelfake.NewClock(epoch),
		serving:   make(chan struct{}),
	}
	alphaStrand := creelfake.LiveStrand(alpha)
	if asStrand {
		alphaStrand.PanePID = os.Getpid()
	}
	r.del.SetStrands(alphaStrand, creelfake.LiveStrand(beta), creelfake.LiveStrand(orch), creelfake.LiveStrand(orch2))
	return r
}

func (r *rig) options(ready func()) creelengine.Options {
	return creelengine.Options{
		Geometry: r.geom, Delivery: r.del, Clock: r.clock, PollClock: r.pollClock, ParentPID: proc.ParentPID, GOOS: runtime.GOOS,
		Replaced: r.replaced.Load,
		Ready: func() {
			r.started.Add(1)
			r.servingOnce.Do(func() { close(r.serving) })
			if ready != nil {
				ready()
			}
		},
	}
}

// running is one daemon started by a rig.
type running struct {
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
	err    error
}

// stop ends the daemon and returns what Run returned.
func (d *running) stop() error {
	d.once.Do(func() {
		d.cancel()
		d.err = <-d.done
	})
	return d.err
}

// start runs a daemon over the rig on fresh clocks and returns once it listens.
// Fresh clocks keep the waits of a stopped daemon out of the count of the next.
func (r *rig) start() *running {
	r.t.Helper()
	r.clock, r.pollClock = creelfake.NewClock(epoch), creelfake.NewClock(epoch)
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	daemon := &running{cancel: cancel, done: make(chan error, 1)}
	go func() { daemon.done <- creelengine.Run(ctx, r.options(func() { close(ready) })) }()
	select {
	case <-ready:
	case err := <-daemon.done:
		r.t.Fatalf("the daemon ended before it listened: %v", err)
	case <-time.After(10 * time.Second):
		r.t.Fatal("the daemon did not listen")
	}
	r.t.Cleanup(func() { _ = daemon.stop() })
	return daemon
}

// spawn starts a daemon in the background, as the CLI's detached spawn does, and returns once one daemon of the rig serves.
func (r *rig) spawn() error {
	r.spawns.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- creelengine.Run(ctx, r.options(nil)) }()
	r.t.Cleanup(func() {
		cancel()
		<-done
	})
	select {
	case <-r.serving:
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("no daemon came up")
	}
}

// client returns a client over the rig whose spawn, when set, answers pid 0 with no exit signal.
func (r *rig) client(claim creelengine.Claim, spawn func() error, clock creelengine.Clock) *creelengine.Client {
	var adapted func() (creelengine.Spawn, error)
	if spawn != nil {
		adapted = func() (creelengine.Spawn, error) { return creelengine.Spawn{}, spawn() }
	}
	return creelengine.NewClient(r.geom, claim, adapted, clock)
}

func externalClaim() creelengine.Claim {
	return creelengine.Claim{Terminal: "pts/4", ParentProcess: "bash", WorkingDir: "/home/me/x"}
}

func strandClaim(name string) creelengine.Claim { return creelengine.Claim{StrandName: name} }

func programClaim(name string) creelengine.Claim { return creelengine.Claim{Program: name} }

type callResult struct {
	resp creelengine.Response
	err  error
}

func callAsync(c *creelengine.Client, req creelengine.Request, timeout time.Duration) <-chan callResult {
	results := make(chan callResult, 1)
	go func() {
		resp, err := c.Call(req, timeout, nil)
		results <- callResult{resp, err}
	}()
	return results
}

// result waits for an async call; the real-time deadline only bounds a failing test.
func result(t *testing.T, results <-chan callResult) callResult {
	t.Helper()
	select {
	case got := <-results:
		return got
	case <-time.After(10 * time.Second):
		t.Fatal("the call did not return")
		return callResult{}
	}
}

// nextProgress waits for the next progress response of a watching call; the real-time deadline only bounds a failing test.
func nextProgress(t *testing.T, progress <-chan creelengine.Response) creelengine.Response {
	t.Helper()
	select {
	case note := <-progress:
		return note
	case <-time.After(10 * time.Second):
		t.Fatal("no progress response arrived")
		return creelengine.Response{}
	}
}

// expiredLine returns the journal line of the first expired event.
func expiredLine(t *testing.T, geom creelengine.Geometry) string {
	t.Helper()
	for _, line := range strings.Split(journalText(t, geom), "\n") {
		if strings.Contains(line, `"event":"expired"`) {
			return line
		}
	}
	return ""
}

func mustCall(t *testing.T, c *creelengine.Client, req creelengine.Request) creelengine.Response {
	t.Helper()
	got := result(t, callAsync(c, req, 0))
	if got.err != nil {
		t.Fatalf("%s: %v", req.Verb, got.err)
	}
	return got.resp
}

func journalText(t *testing.T, geom creelengine.Geometry) string {
	t.Helper()
	entries, err := os.ReadDir(geom.JournalDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read the journal directory: %v", err)
	}
	var text strings.Builder
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(geom.JournalDir, entry.Name()))
		if err != nil {
			t.Fatalf("read the journal file: %v", err)
		}
		text.Write(data)
	}
	return text.String()
}

// writeJournal writes a day file of raw lines into the rig's journal directory.
func (r *rig) writeJournal(day string, lines ...string) {
	r.t.Helper()
	if err := os.MkdirAll(r.geom.JournalDir, 0o755); err != nil {
		r.t.Fatalf("create the journal directory: %v", err)
	}
	path := filepath.Join(r.geom.JournalDir, day+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		r.t.Fatalf("write %s: %v", path, err)
	}
}

// rawExchange sends one raw line on a fresh connection and returns the response line.
func (r *rig) rawExchange(line string) creelengine.Response {
	r.t.Helper()
	conn, err := net.Dial("unix", r.geom.SocketPath)
	if err != nil {
		r.t.Fatalf("dial %s: %v", r.geom.SocketPath, err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(line + "\n")); err != nil {
		r.t.Fatalf("write: %v", err)
	}
	answer, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		r.t.Fatalf("read the answer: %v", err)
	}
	var resp creelengine.Response
	if err := json.Unmarshal(answer, &resp); err != nil {
		r.t.Fatalf("decode %q: %v", answer, err)
	}
	return resp
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

func TestDaemon_Protocol(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		asStrand bool
		run      func(t *testing.T, r *rig)
	}{
		{"PostIsAcknowledgedWithItsIdAfterItsLineIsInTheJournal", false, func(t *testing.T, r *rig) {
			r.start()
			c := r.client(externalClaim(), nil, creelfake.NewClock(epoch))
			resp := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbPost, Receiver: beta, Text: "hello"})
			if resp.ID == 0 || !strings.Contains(journalText(t, r.geom), `"event":"posted","at":`) {
				t.Errorf("post answered id %d with the journal %q", resp.ID, journalText(t, r.geom))
			}
			if !strings.Contains(journalText(t, r.geom), `"body":"hello"`) {
				t.Errorf("the journal lacks the posted line: %q", journalText(t, r.geom))
			}
		}},
		{"ReadAckMarksReadAndAClientDyingBeforeTheAckLeavesMessagesUnread", true, func(t *testing.T, r *rig) {
			r.start()
			c := r.client(strandClaim(alpha), nil, creelfake.NewClock(epoch))
			mustCall(t, c, creelengine.Request{Verb: creelengine.VerbPost, Receiver: alpha, Text: "note to self"})
			conn, err := net.Dial("unix", r.geom.SocketPath)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			if _, err := conn.Write([]byte(`{"verb":"read","claim":{"strand_name":"` + alpha + `"}}` + "\n")); err != nil {
				t.Fatalf("write: %v", err)
			}
			answer, err := bufio.NewReader(conn).ReadBytes('\n')
			var handed creelengine.Response
			if err != nil || json.Unmarshal(answer, &handed) != nil || len(handed.Messages) != 1 || !handed.AckRequired {
				t.Fatalf("raw read answered %q, %v; want one message awaiting its ack", answer, err)
			}
			_ = conn.Close()
			got := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbRead, Wait: true})
			if len(got.Messages) != 1 {
				t.Fatalf("read --wait after the dying client = %d messages; want the message back", len(got.Messages))
			}
			if again := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbRead}); len(again.Messages) != 0 {
				t.Errorf("a read after the acked one = %d messages; want none", len(again.Messages))
			}
		}},
		{"ReadHandsOutEveryUnreadMessageFromEverySender", false, func(t *testing.T, r *rig) {
			r.start()
			batten := r.client(programClaim("ly:BATTEN"), nil, creelfake.NewClock(epoch))
			mustCall(t, batten, creelengine.Request{Verb: creelengine.VerbRead})
			external := r.client(externalClaim(), nil, creelfake.NewClock(epoch))
			mustCall(t, external, creelengine.Request{Verb: creelengine.VerbPost, Receiver: "ly:BATTEN", Text: "from the external"})
			carter := r.client(programClaim("ly:CARTER"), nil, creelfake.NewClock(epoch))
			mustCall(t, carter, creelengine.Request{Verb: creelengine.VerbPost, Receiver: "ly:BATTEN", Text: "from carter"})
			got := mustCall(t, batten, creelengine.Request{Verb: creelengine.VerbRead})
			var bodies []string
			for _, msg := range got.Messages {
				bodies = append(bodies, msg.Body)
			}
			if want := []string{"from the external", "from carter"}; !slices.Equal(bodies, want) {
				t.Errorf("read = %q; want %q", bodies, want)
			}
		}},
		{"ReadWaitReturnsOnceAPostArrives", true, func(t *testing.T, r *rig) {
			r.start()
			c := r.client(strandClaim(alpha), nil, creelfake.NewClock(epoch))
			waiting := callAsync(c, creelengine.Request{Verb: creelengine.VerbRead, Wait: true}, 0)
			mustCall(t, c, creelengine.Request{Verb: creelengine.VerbPost, Receiver: alpha, Text: "wake up"})
			got := result(t, waiting)
			if got.err != nil || len(got.resp.Messages) != 1 || got.resp.Messages[0].Body != "wake up" {
				t.Errorf("read --wait = %+v, %v; want the posted message", got.resp.Messages, got.err)
			}
		}},
		{"WaitingCallsReturnErrWaitTimeoutOnTheClientClock", true, func(t *testing.T, r *rig) {
			r.start()
			r.del.SetIdle(alpha, false)
			requests := []creelengine.Request{
				{Verb: creelengine.VerbRead, Wait: true},
				{Verb: creelengine.VerbPost, Receiver: beta, Text: "anyone?", Wait: true},
				{Verb: creelengine.VerbInject, Receiver: alpha, Text: "/clear", Wait: true},
			}
			for _, req := range requests {
				clock := creelfake.NewClock(epoch)
				waiting := callAsync(r.client(strandClaim(alpha), nil, clock), req, 5*time.Minute)
				clock.AwaitWaits(1)
				clock.Advance(5 * time.Minute)
				if got := result(t, waiting); !errors.Is(got.err, creelengine.ErrWaitTimeout) {
					t.Errorf("%s --wait = %v; want ErrWaitTimeout", req.Verb, got.err)
				}
			}
		}},
		{"ReplyReachesAWaitingExternalPost", false, func(t *testing.T, r *rig) {
			r.start()
			program := r.client(programClaim("ly:BATTEN"), nil, creelfake.NewClock(epoch))
			mustCall(t, program, creelengine.Request{Verb: creelengine.VerbRead})
			external := r.client(externalClaim(), nil, creelfake.NewClock(epoch))
			waiting := callAsync(external, creelengine.Request{Verb: creelengine.VerbPost, Receiver: "ly:BATTEN", Text: "question", Wait: true}, 0)
			mail := mustCall(t, program, creelengine.Request{Verb: creelengine.VerbRead, Wait: true})
			if len(mail.Messages) != 1 {
				t.Fatalf("the program read %d messages; want the external question", len(mail.Messages))
			}
			mustCall(t, program, creelengine.Request{Verb: creelengine.VerbReply, ID: mail.Messages[0].ID, Text: "answer"})
			got := result(t, waiting)
			if got.err != nil || got.resp.Outcome != creelengine.OutcomeReply || got.resp.Message == nil || got.resp.Message.Body != "answer" {
				t.Errorf("post --wait-reply = %+v, %v; want the reply handed over", got.resp, got.err)
			}
		}},
		{"InjectWaitReportsInjected", false, func(t *testing.T, r *rig) {
			r.start()
			c := r.client(externalClaim(), nil, creelfake.NewClock(epoch))
			resp := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbInject, Receiver: alpha, Text: "/clear", Wait: true})
			if resp.Outcome != creelengine.OutcomeInjected {
				t.Errorf("inject --wait = %v; want injected", resp.Outcome)
			}
		}},
		{"InjectWaitReportsUnconfirmed", false, func(t *testing.T, r *rig) {
			r.start()
			r.del.SetSendError(creelengine.ErrNotLanded)
			c := r.client(externalClaim(), nil, creelfake.NewClock(epoch))
			resp := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbInject, Receiver: alpha, Text: "/clear", Wait: true})
			if resp.Outcome != creelengine.OutcomeUnconfirmed {
				t.Errorf("inject --wait = %v; want unconfirmed", resp.Outcome)
			}
		}},
		{"InjectWaitReportsExpired", false, func(t *testing.T, r *rig) {
			r.start()
			r.del.SetIdle(alpha, false)
			c := r.client(externalClaim(), nil, creelfake.NewClock(epoch))
			waiting := callAsync(c, creelengine.Request{Verb: creelengine.VerbInject, Receiver: alpha, Text: "/clear", TTLSeconds: 60, Wait: true}, 0)
			r.clock.AwaitWaitsWhere(1, isProbeWait)
			r.clock.Advance(61 * time.Second)
			if got := result(t, waiting); got.err != nil || got.resp.Outcome != creelengine.OutcomeExpired || got.resp.Stall != "" {
				t.Errorf("inject --wait = %+v, %v; want expired with no stall", got.resp, got.err)
			}
		}},
		{"InjectWaitHearsEachStallChangeAndExpiresWithTheLastOne", false, func(t *testing.T, r *rig) {
			r.start()
			r.del.SetStall(alpha, creelengine.StallDraft)
			c := r.client(externalClaim(), nil, creelfake.NewClock(epoch))
			progress := make(chan creelengine.Response, 8)
			done := make(chan callResult, 1)
			returned := make(chan struct{})
			go func() {
				resp, err := c.CallWatching(creelengine.Request{Verb: creelengine.VerbInject, Receiver: alpha, Text: "/clear", TTLSeconds: 60, Wait: true}, 0, nil,
					func(note creelengine.Response) { progress <- note })
				done <- callResult{resp, err}
				close(returned)
			}()
			r.clock.AwaitWaitsWhere(1, isProbeWait)
			r.clock.Advance(time.Second)
			if note := nextProgress(t, progress); !note.Progress || note.Stall != creelengine.StallDraft || note.StallAt.IsZero() {
				t.Errorf("first progress = %+v; want the draft stall with its time", note)
			}
			r.del.SetStall(alpha, creelengine.StallTooShort)
			r.clock.AwaitWaitsWhere(2, isProbeWait)
			r.clock.Advance(2 * time.Second)
			if note := nextProgress(t, progress); note.Stall != creelengine.StallTooShort {
				t.Errorf("second progress = %+v; want the too-short stall", note)
			}
			r.clock.AdvanceByWaitsWhere(isProbeWait, returned)
			got := result(t, done)
			if got.err != nil || got.resp.Outcome != creelengine.OutcomeExpired || got.resp.Stall != creelengine.StallTooShort || got.resp.StallAt.IsZero() {
				t.Errorf("inject --wait = %+v, %v; want expired carrying the too-short stall", got.resp, got.err)
			}
			if len(progress) != 0 {
				t.Errorf("%d further progress responses; want exactly the two changes", len(progress))
			}
			if !strings.Contains(expiredLine(t, r.geom), `"reason":"too-short"`) {
				t.Errorf("the expired line = %q; want it to carry reason too-short", expiredLine(t, r.geom))
			}
		}},
		{"ProgramCommandInjectIsAnsweredWithItsIdAndLoggedWithItsCommand", false, func(t *testing.T, r *rig) {
			r.start()
			c := r.client(programClaim("ly:BATTEN"), nil, creelfake.NewClock(epoch))
			queued := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbInject, Receiver: orch, Command: creelengine.CommandCompact, Argument: "keep the plan"})
			if queued.ID == 0 {
				t.Fatalf("command inject answered %+v; want its id", queued)
			}
			logged := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbLog, ID: queued.ID})
			if len(logged.Log) == 0 {
				t.Fatalf("log of inject %d is empty", queued.ID)
			}
			for _, entry := range logged.Log {
				if entry.Command != creelengine.CommandCompact || entry.Argument != "keep the plan" {
					t.Errorf("log entry %+v lacks the command and argument", entry)
				}
			}
		}},
		{"CommandInjectWithWaitOrInterruptIsRefused", false, func(t *testing.T, r *rig) {
			r.start()
			c := r.client(programClaim("ly:BATTEN"), nil, creelfake.NewClock(epoch))
			for _, req := range []creelengine.Request{
				{Verb: creelengine.VerbInject, Receiver: orch, Command: creelengine.CommandClear, Wait: true},
				{Verb: creelengine.VerbInject, Receiver: orch, Command: creelengine.CommandClear, Interrupt: true},
			} {
				got := result(t, callAsync(c, req, 0))
				if got.err == nil || !strings.Contains(got.err.Error(), "neither wait nor interrupt") {
					t.Errorf("%+v answered %v; want the refusal naming wait and interrupt", req, got.err)
				}
			}
		}},
		{"ResumedInjectWaitHearsTheStandingStallAtOnce", false, func(t *testing.T, r *rig) {
			r.start()
			r.del.SetStall(alpha, creelengine.StallDraft)
			c := r.client(externalClaim(), nil, creelfake.NewClock(epoch))
			queued := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbInject, Receiver: alpha, Text: "/clear", TTLSeconds: 60})
			r.clock.AwaitWaitsWhere(1, isProbeWait)
			r.clock.Advance(time.Second)
			r.clock.AwaitWaitsWhere(2, isProbeWait)
			progress := make(chan creelengine.Response, 8)
			done := make(chan callResult, 1)
			returned := make(chan struct{})
			go func() {
				resp, err := c.CallWatching(creelengine.Request{Verb: creelengine.VerbWaitInject, ID: queued.ID, Wait: true}, 0, nil,
					func(note creelengine.Response) { progress <- note })
				done <- callResult{resp, err}
				close(returned)
			}()
			if note := nextProgress(t, progress); note.ID != queued.ID || note.Stall != creelengine.StallDraft {
				t.Errorf("resumed wait progress = %+v; want the standing draft stall for inject %d", note, queued.ID)
			}
			r.clock.AdvanceByWaitsWhere(isProbeWait, returned)
			if got := result(t, done); got.err != nil || got.resp.Outcome != creelengine.OutcomeExpired || got.resp.Stall != creelengine.StallDraft {
				t.Errorf("wait-inject = %+v, %v; want expired carrying the draft stall", got.resp, got.err)
			}
		}},
		{"PostWaitReplyIsAnsweredWhenTheMessageReturnsUndelivered", false, func(t *testing.T, r *rig) {
			r.start()
			c := r.client(externalClaim(), nil, creelfake.NewClock(epoch))
			waiting := callAsync(c, creelengine.Request{Verb: creelengine.VerbPost, Receiver: alpha, Text: "anyone?", Wait: true}, 0)
			r.clock.AwaitWaitsWhere(1, isSweepWait)
			r.del.SetStrands(creelfake.LiveStrand(beta))
			r.sweepStep()
			r.sweepLast()
			got := result(t, waiting)
			if got.err != nil || got.resp.Outcome != creelengine.OutcomeReturned {
				t.Errorf("post --wait-reply = %+v, %v; want returned undelivered", got.resp, got.err)
			}
		}},
		{"ClientRefusesADaemonAnsweringForAnotherHub", false, func(t *testing.T, r *rig) {
			r.start()
			other := r.geom
			other.HubPath += "-other"
			c := creelengine.NewClient(other, externalClaim(), nil, creelfake.NewClock(epoch))
			_, err := c.Call(creelengine.Request{Verb: creelengine.VerbLog}, 0, nil)
			if err == nil || !strings.Contains(err.Error(), "answers for the hub") {
				t.Errorf("Call = %v; want a refusal of the daemon answering for another hub", err)
			}
		}},
		{"MalformedLineAndUnknownVerbAreAnsweredWithAnErrorAndLeaveNoJournalLine", false, func(t *testing.T, r *rig) {
			r.start()
			if resp := r.rawExchange("this is not json"); resp.OK || !strings.Contains(resp.Error, "not valid JSON") {
				t.Errorf("malformed line answered %+v; want an error naming the fault", resp)
			}
			if resp := r.rawExchange(`{"verb":"frobnicate"}`); resp.OK || !strings.Contains(resp.Error, "unknown verb") {
				t.Errorf("unknown verb answered %+v; want an error naming the verb", resp)
			}
			if text := journalText(t, r.geom); text != "" {
				t.Errorf("the journal holds %q after two refused requests", text)
			}
		}},
		{"SenderIsResolvedFromThePeer", true, func(t *testing.T, r *rig) {
			r.start()
			c := r.client(strandClaim(alpha), nil, creelfake.NewClock(epoch))
			resp := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbPost, Receiver: beta, Text: "signed"})
			if resp.Sender != alpha {
				t.Errorf("post resolved the sender %q; want %q", resp.Sender, alpha)
			}
			if runtime.GOOS != "linux" {
				t.Log("the disagreeing-name refusal rests on D4's Linux-only ancestry check")
				return
			}
			liar := r.client(strandClaim(beta), nil, creelfake.NewClock(epoch))
			if _, err := liar.Call(creelengine.Request{Verb: creelengine.VerbPost, Receiver: alpha, Text: "forged"}, 0, nil); err == nil || !strings.Contains(err.Error(), "runs in the pane of") {
				t.Errorf("a claim naming another strand: %v; want it refused", err)
			}
		}},
		{"WithdrawByAProgramClientIsAnsweredWithTheResult", false, func(t *testing.T, r *rig) {
			r.del.SetIdle(alpha, false)
			r.start()
			c := r.client(programClaim(batten), nil, creelfake.NewClock(epoch))
			inject := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbInject, Receiver: alpha, Text: "/clear"})
			mail := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbPost, Receiver: alpha, Text: "note\nmore"})
			resp := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbWithdraw, Receiver: alpha})
			want := &creelengine.WithdrawResult{Injects: []int64{inject.ID}, Messages: []creelengine.WithdrawnMessage{{ID: mail.ID, Subject: "note"}}}
			if !reflect.DeepEqual(resp.Withdrawn, want) {
				t.Errorf("withdraw answered %+v; want %+v", resp.Withdrawn, want)
			}
		}},
		{"StrandClaimFromOutsideEveryPaneCarriesTheRefusalCode", false, func(t *testing.T, r *rig) {
			if runtime.GOOS != "linux" {
				t.Skip("the outside-pane refusal rests on D4's Linux-only ancestry check")
			}
			r.start()
			c := r.client(strandClaim(alpha), nil, creelfake.NewClock(epoch))
			_, err := c.Call(creelengine.Request{Verb: creelengine.VerbLog}, 0, nil)
			if !errors.Is(err, creelengine.ErrStrandClaimOutsidePane) || !strings.Contains(err.Error(), "runs in no strand pane") {
				t.Errorf("a strand claim from outside every pane: %v; want an error matching ErrStrandClaimOutsidePane with the daemon's message", err)
			}
			if resp := r.rawExchange(`{"verb":"log","claim":{"strand_name":"` + alpha + `"}}`); resp.OK || resp.Code != creelengine.CodeStrandClaimOutsidePane {
				t.Errorf("raw refusal = %+v; want the code %q", resp, creelengine.CodeStrandClaimOutsidePane)
			}
			if resp := r.rawExchange(`{"verb":"frobnicate"}`); resp.OK || resp.Code != "" {
				t.Errorf("an unrelated refusal = %+v; want no code", resp)
			}
		}},
		{"NonWaitingCallToAListenerThatNeverAnswersTimesOut", false, func(t *testing.T, r *rig) {
			listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: r.geom.SocketPath, Net: "unix"})
			if err != nil {
				t.Fatalf("listen: %v", err)
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
			clientClock := creelfake.NewClock(epoch)
			calling := callAsync(r.client(externalClaim(), nil, clientClock), creelengine.Request{Verb: creelengine.VerbLog}, 5*time.Second)
			clientClock.AwaitWaits(1)
			clientClock.Advance(5 * time.Second)
			if got := result(t, calling); !errors.Is(got.err, creelengine.ErrWaitTimeout) {
				t.Errorf("non-waiting call = %+v, %v; want ErrWaitTimeout at the timeout", got.resp, got.err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t, newRig(t, tt.asStrand))
		})
	}
}

const (
	oldInject = `{"event":"posted","at":"2026-10-01T11:00:00Z","id":1,"from":"ly:BATTEN","to":"ly:alpha:driver","kind":"inject","body":"/clear","ttl_s":600}`
)

func TestDaemon_Lifecycle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(t *testing.T, r *rig)
	}{
		{"TwoDaemonsStartedAtOnceOneServesAndTheOtherReturnsErrDaemonRunning", func(t *testing.T, r *rig) {
			results := make(chan error, 2)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			for range 2 {
				go func() { results <- creelengine.Run(ctx, r.options(nil)) }()
			}
			select {
			case err := <-results:
				if !errors.Is(err, creelengine.ErrDaemonRunning) {
					t.Fatalf("the first daemon to end returned %v; want ErrDaemonRunning", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("neither daemon ended")
			}
			<-r.serving
			mustCall(t, r.client(externalClaim(), nil, creelfake.NewClock(epoch)), creelengine.Request{Verb: creelengine.VerbLog})
			if got := r.started.Load(); got != 1 {
				t.Errorf("%d daemons listened; want one", got)
			}
			cancel()
			if err := <-results; err != nil {
				t.Errorf("the serving daemon returned %v on cancel", err)
			}
		}},
		{"TwoConcurrentFirstCallsEndWithOneDaemonAndBothAnswered", func(t *testing.T, r *rig) {
			var wg sync.WaitGroup
			errs := make([]error, 2)
			for i := range errs {
				wg.Add(1)
				go func() {
					defer wg.Done()
					c := r.client(externalClaim(), r.spawn, creelfake.NewClock(epoch))
					_, errs[i] = c.Call(creelengine.Request{Verb: creelengine.VerbLog}, 0, nil)
				}()
			}
			wg.Wait()
			if errs[0] != nil || errs[1] != nil {
				t.Fatalf("calls returned %v and %v; want both answered", errs[0], errs[1])
			}
			if got := r.started.Load(); got != 1 {
				t.Errorf("%d daemons listened; want one", got)
			}
		}},
		{"ClientWithNoListenerSpawnsOnceAndGivesUpNamingTheSocketAndQuotingTheDaemonLog", func(t *testing.T, _ *rig) {
			const header4242 = "creel daemon start pid=4242 at=2026-10-01T12:00:00Z"
			logTests := []struct {
				name    string
				content string
				// absent leaves the file uncreated.
				absent bool
				mode   os.FileMode
				// lostRace has the spawn fire a lost-race exit at once.
				lostRace bool
				// skipRoot skips the row on Windows and as root, where the mode denies nothing.
				skipRoot bool
				want     []string
				notWant  []string
			}{
				{
					name: "OwnLinesAndAPanicLineAfterItsHeaderAreQuoted",
					content: "creel daemon start pid=77 at=2026-10-01T11:00:00Z\npid=77 seventy-seven warning\nstray before the header\n" +
						header4242 + "\npid=4242 own warning\npanic: boom\n",
					mode:    0o644,
					want:    []string{"pid=4242 own warning", "panic: boom", header4242},
					notWant: []string{"pid=77", "stray before the header", "creel daemon start pid=77"},
				},
				{
					name:    "AnotherHeaderAfterItQuotesOnlyTaggedLinesAndSaysSo",
					content: header4242 + "\npid=4242 own warning\nan unattributed line\ncreel daemon start pid=88 at=2026-10-01T12:01:00Z\npid=88 other\n",
					mode:    0o644,
					want:    []string{"pid=4242 own warning", "cannot attribute"},
					notWant: []string{"an unattributed line", "pid=88"},
				},
				{
					name:     "ALostRaceExitKeepsTheBackoffAndTheSameError",
					content:  header4242 + "\npid=4242 own warning\n",
					mode:     0o644,
					lostRace: true,
					want:     []string{"pid=4242 own warning", header4242},
				},
				{
					name:    "TaggedLinesWithNoHeaderSurviveATruncation",
					content: "pid=4242 late warning\n",
					mode:    0o644,
					want:    []string{"pid=4242 late warning"},
				},
				{
					name:    "OnlyAnotherPidsLinesSayNoLineFromThatStart",
					content: "creel daemon start pid=77 at=2026-10-01T11:00:00Z\npid=77 seventy-seven warning\n",
					mode:    0o644,
					want:    []string{"holds no line from that start"},
					notWant: []string{"seventy-seven"},
				},
				{
					name:     "ALogThatCannotBeAppendedToCouldNotBeOpened",
					content:  "pid=4242 hidden\n",
					mode:     0o444,
					skipRoot: true,
					want:     []string{"could not be opened"},
					notWant:  []string{"hidden"},
				},
				{
					name:     "ALogThatCannotBeReadCouldNotBeOpened",
					content:  "pid=4242 hidden\n",
					mode:     0o200,
					skipRoot: true,
					want:     []string{"could not be opened"},
					notWant:  []string{"hidden"},
				},
				{
					name:   "AnAbsentLogCouldNotBeOpened",
					absent: true,
					want:   []string{"could not be opened"},
				},
			}
			for _, tt := range logTests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					if tt.skipRoot && (runtime.GOOS == "windows" || os.Geteuid() == 0) {
						t.Skip("the file mode denies nothing on Windows or to root")
					}
					r := newRig(t, false)
					if !tt.absent {
						if err := os.MkdirAll(r.geom.StateDir, 0o755); err != nil {
							t.Fatalf("mkdir: %v", err)
						}
						if err := os.WriteFile(r.geom.DaemonLogPath, []byte(tt.content), 0o644); err != nil {
							t.Fatalf("write the daemon log: %v", err)
						}
						if err := os.Chmod(r.geom.DaemonLogPath, tt.mode); err != nil {
							t.Fatalf("chmod the daemon log: %v", err)
						}
					}
					clock := creelfake.NewClock(epoch)
					done := make(chan struct{})
					go clock.AutoAdvance(done)
					defer close(done)
					var spawns int
					c := creelengine.NewClient(r.geom, externalClaim(), func() (creelengine.Spawn, error) {
						spawns++
						spawned := creelengine.Spawn{PID: 4242}
						if tt.lostRace {
							exited := make(chan creelengine.SpawnExit, 1)
							exited <- creelengine.SpawnExit{LostRace: true}
							spawned.Exited = exited
						}
						return spawned, nil
					}, clock)
					_, err := c.Call(creelengine.Request{Verb: creelengine.VerbLog}, 0, nil)
					if err == nil || !strings.Contains(err.Error(), r.geom.SocketPath) || !strings.Contains(err.Error(), r.geom.DaemonLogPath) {
						t.Fatalf("Call = %v; want an error naming the socket and the daemon log", err)
					}
					for _, want := range tt.want {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("Call error = %q; want it to contain %q", err, want)
						}
					}
					for _, notWant := range tt.notWant {
						if strings.Contains(err.Error(), notWant) {
							t.Errorf("Call error = %q; want it to omit %q", err, notWant)
						}
					}
					if spawns != 1 {
						t.Errorf("spawn called %d times; want once", spawns)
					}
					if waits := clock.Waits(); len(waits) != creelengine.ConnectAttempts-1 || waits[0] != time.Second {
						t.Errorf("sleeps = %v; want %d, from one second", waits, creelengine.ConnectAttempts-1)
					}
				})
			}
		}},
		{"ClientWhoseSpawnedDaemonExitsAtOnceReturnsTheLogErrorWithoutTheBackoff", func(t *testing.T, r *rig) {
			exited := make(chan creelengine.SpawnExit, 1)
			exited <- creelengine.SpawnExit{}
			clock := creelfake.NewClock(epoch)
			c := creelengine.NewClient(r.geom, externalClaim(), func() (creelengine.Spawn, error) {
				return creelengine.Spawn{PID: 4242, Exited: exited}, nil
			}, clock)
			got := result(t, callAsync(c, creelengine.Request{Verb: creelengine.VerbLog}, 0))
			if got.err == nil || !strings.Contains(got.err.Error(), r.geom.SocketPath) || !strings.Contains(got.err.Error(), r.geom.DaemonLogPath) {
				t.Errorf("Call = %v; want an error naming the socket and the daemon log", got.err)
			}
			if !clock.Now().Equal(epoch) {
				t.Errorf("the client clock reads %v; want it never advanced from %v", clock.Now(), epoch)
			}
		}},
		{"ClientWhoseSpawnedDaemonExitsMidWaitConnectsOnceMoreToADaemonThatServes", func(t *testing.T, r *rig) {
			exited := make(chan creelengine.SpawnExit, 1)
			clock := creelfake.NewClock(epoch)
			c := creelengine.NewClient(r.geom, externalClaim(), func() (creelengine.Spawn, error) {
				return creelengine.Spawn{PID: 4242, Exited: exited}, nil
			}, clock)
			waiting := callAsync(c, creelengine.Request{Verb: creelengine.VerbLog}, 0)
			clock.AwaitWaits(1)
			r.start()
			exited <- creelengine.SpawnExit{}
			if got := result(t, waiting); got.err != nil {
				t.Errorf("Call = %v; want it to connect to the daemon that serves once the spawn exited", got.err)
			}
		}},
		{"ClientWhoseSpawnLostTheStartRaceKeepsTheBackoffAndConnects", func(t *testing.T, r *rig) {
			exited := make(chan creelengine.SpawnExit, 1)
			exited <- creelengine.SpawnExit{LostRace: true}
			clock := creelfake.NewClock(epoch)
			c := creelengine.NewClient(r.geom, externalClaim(), func() (creelengine.Spawn, error) {
				return creelengine.Spawn{PID: 4242, Exited: exited}, nil
			}, clock)
			waiting := callAsync(c, creelengine.Request{Verb: creelengine.VerbLog}, 0)
			clock.AwaitWaits(1)
			select {
			case got := <-waiting:
				t.Fatalf("Call returned %+v, %v before the backoff passed; want it to keep waiting after a lost race", got.resp, got.err)
			default:
			}
			r.start()
			clock.Advance(time.Second)
			if got := result(t, waiting); got.err != nil {
				t.Errorf("Call = %v; want it to connect after the backoff", got.err)
			}
		}},
		{"WaitingCallThatReconnectsWaitsOnlyOnItsOwnSpawnsSignal", func(t *testing.T, r *rig) {
			r.del.SetIdle(alpha, false)
			firstExited := make(chan creelengine.SpawnExit, 1)
			secondExited := make(chan creelengine.SpawnExit, 1)
			var stopFirst func()
			var spawns atomic.Int32
			clientClock := creelfake.NewClock(epoch)
			c := creelengine.NewClient(r.geom, externalClaim(), func() (creelengine.Spawn, error) {
				if spawns.Add(1) > 1 {
					return creelengine.Spawn{PID: 4243, Exited: secondExited}, nil
				}
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan error, 1)
				go func() { done <- creelengine.Run(ctx, r.options(nil)) }()
				stopFirst = sync.OnceFunc(func() {
					cancel()
					<-done
				})
				t.Cleanup(stopFirst)
				<-r.serving
				return creelengine.Spawn{PID: 4242, Exited: firstExited}, nil
			}, clientClock)
			waiting := callAsync(c, creelengine.Request{Verb: creelengine.VerbInject, Receiver: alpha, Text: "/clear", Wait: true}, 0)
			r.clock.AwaitWaitsWhere(1, isProbeWait)
			stopFirst()
			firstExited <- creelengine.SpawnExit{}
			clientClock.AwaitWaits(1)
			clientClock.Advance(time.Second)
			clientClock.AwaitWaits(2)
			select {
			case got := <-waiting:
				t.Fatalf("Call returned %+v, %v on the first spawn's exit; want it to wait on its own spawn's signal", got.resp, got.err)
			default:
			}
			r.del.SetIdle(alpha, true)
			r.start()
			r.clock.AwaitWaitsWhere(1, isProbeWait)
			clientClock.Advance(time.Second)
			if got := result(t, waiting); got.err != nil || got.resp.Outcome != creelengine.OutcomeInjected {
				t.Errorf("inject --wait = %+v, %v; want injected", got.resp, got.err)
			}
		}},
		{"StaleSocketFileIsRemovedAndRebound", func(t *testing.T, r *rig) {
			stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: r.geom.SocketPath, Net: "unix"})
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			stale.SetUnlinkOnClose(false)
			_ = stale.Close()
			if _, err := os.Lstat(r.geom.SocketPath); err != nil {
				t.Fatalf("the stale socket file is missing: %v", err)
			}
			r.start()
			mustCall(t, r.client(externalClaim(), nil, creelfake.NewClock(epoch)), creelengine.Request{Verb: creelengine.VerbLog})
		}},
		{"JournalOfAKilledDaemonRebuildsTheQueuedInjectWithItsConsumedTTL", func(t *testing.T, r *rig) {
			r.writeJournal("2026-10-01", oldInject, `{"event":"aged","at":"2026-10-01T11:59:00Z","id":1,"consumed_s":540}`)
			r.del.SetIdle(alpha, false)
			r.start()
			c := r.client(programClaim("ly:BATTEN"), nil, creelfake.NewClock(epoch))
			waiting := callAsync(c, creelengine.Request{Verb: creelengine.VerbWaitInject, ID: 1}, 0)
			r.clock.AwaitWaitsWhere(1, isProbeWait)
			r.clock.Advance(time.Second)
			r.clock.AwaitWaitsWhere(2, isProbeWait)
			r.clock.Advance(30 * time.Second)
			r.clock.AwaitWaitsWhere(3, isProbeWait)
			select {
			case got := <-waiting:
				t.Fatalf("the inject ended %+v, %v after 31 seconds; it had 60 left", got.resp, got.err)
			default:
			}
			r.clock.Advance(30 * time.Second)
			if got := result(t, waiting); got.err != nil || got.resp.Outcome != creelengine.OutcomeExpired {
				t.Errorf("wait-inject = %+v, %v; want expired once the consumed TTL ran out", got.resp, got.err)
			}
		}},
		{"StageOneOrchMailAndItsWithheldRecordFoldAndTheStartedDaemonReminds", func(t *testing.T, r *rig) {
			r.writeJournal("2026-10-01",
				`{"event":"posted","at":"2026-10-01T11:00:00Z","id":1,"from":"`+alpha+`","to":"`+orch+`","kind":"mail","body":"hello"}`,
				`{"event":"withheld","at":"2026-10-01T11:00:00Z","id":1,"to":"`+orch+`","notice":"creel-notices"}`)
			r.start()
			r.clock.AwaitWaitsWhere(1, isProbeWait)
			want := []creelfake.Send{{Strand: orch, Text: "[creel] 1 unread: #1 from " + alpha + " — run lyx creel read"}}
			if got := r.del.Sends(); !slices.Equal(got, want) {
				t.Errorf("typed %v; want the reminder of the stage 1 mail %v", got, want)
			}
		}},
		{"SendingWithoutAnOutcomeIsUnconfirmedAndNeverTypedAgain", func(t *testing.T, r *rig) {
			r.writeJournal("2026-10-01", oldInject, `{"event":"sending","at":"2026-10-01T11:59:00Z","id":1}`)
			r.start()
			c := r.client(programClaim("ly:BATTEN"), nil, creelfake.NewClock(epoch))
			resp := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbWaitInject, ID: 1})
			if resp.Outcome != creelengine.OutcomeUnconfirmed {
				t.Errorf("wait-inject = %v; want unconfirmed", resp.Outcome)
			}
			r.clock.Advance(time.Hour)
			if sends := r.del.Sends(); len(sends) != 0 {
				t.Errorf("typed %v after a restart over a send with no outcome", sends)
			}
		}},
		{"WaiterReconnectsAcrossARestartAndReceivesTheInjectedOutcome", func(t *testing.T, r *rig) {
			r.del.SetIdle(alpha, false)
			first := r.start()
			clientClock := creelfake.NewClock(epoch)
			waiting := callAsync(r.client(externalClaim(), r.spawn, clientClock), creelengine.Request{Verb: creelengine.VerbInject, Receiver: alpha, Text: "/clear", Wait: true}, 0)
			r.clock.AwaitWaitsWhere(1, isProbeWait)
			if err := first.stop(); err != nil {
				t.Fatalf("stop: %v", err)
			}
			clientClock.AwaitWaits(1)
			r.del.SetIdle(alpha, true)
			r.start()
			r.clock.AwaitWaitsWhere(1, isProbeWait)
			clientClock.Advance(time.Second)
			if got := result(t, waiting); got.err != nil || got.resp.Outcome != creelengine.OutcomeInjected {
				t.Errorf("inject --wait across a restart = %+v, %v; want injected, which happened while it was away", got.resp, got.err)
			}
		}},
		{"WaiterReconnectsAcrossARestartAndReceivesAMessageReturnedUndelivered", func(t *testing.T, r *rig) {
			// Busy, so the EXTERNAL mail is not carried into the pane and marked read before alpha goes.
			r.del.SetIdle(alpha, false)
			first := r.start()
			clientClock := creelfake.NewClock(epoch)
			waiting := callAsync(r.client(externalClaim(), r.spawn, clientClock), creelengine.Request{Verb: creelengine.VerbPost, Receiver: alpha, Text: "anyone?", Wait: true}, 0)
			r.clock.AwaitWaitsWhere(1, isProbeWait)
			if err := first.stop(); err != nil {
				t.Fatalf("stop: %v", err)
			}
			clientClock.AwaitWaits(1)
			r.del.SetStrands(creelfake.LiveStrand(beta))
			r.start()
			r.sweepStep()
			r.sweepLast()
			clientClock.Advance(time.Second)
			if got := result(t, waiting); got.err != nil || got.resp.Outcome != creelengine.OutcomeReturned {
				t.Errorf("post --wait-reply across a restart = %+v, %v; want returned undelivered, which happened while it was away", got.resp, got.err)
			}
		}},
		{"ReplyJournaledWhileTheWaiterWasAwayReachesItAtOnceAndIsJournaledDelivered", func(t *testing.T, r *rig) {
			first := r.start()
			program := r.client(programClaim("ly:BATTEN"), nil, creelfake.NewClock(epoch))
			mustCall(t, program, creelengine.Request{Verb: creelengine.VerbRead})
			clientClock := creelfake.NewClock(epoch)
			waiting := callAsync(r.client(externalClaim(), r.spawn, clientClock), creelengine.Request{Verb: creelengine.VerbPost, Receiver: "ly:BATTEN", Text: "question", Wait: true}, 0)
			mail := mustCall(t, program, creelengine.Request{Verb: creelengine.VerbRead, Wait: true})
			if len(mail.Messages) != 1 {
				t.Fatalf("the program read %d messages; want the question", len(mail.Messages))
			}
			if err := first.stop(); err != nil {
				t.Fatalf("stop: %v", err)
			}
			clientClock.AwaitWaits(1)
			r.start()
			reply := mustCall(t, program, creelengine.Request{Verb: creelengine.VerbReply, ID: mail.Messages[0].ID, Text: "answer"})
			clientClock.Advance(time.Second)
			got := result(t, waiting)
			if got.err != nil || got.resp.Outcome != creelengine.OutcomeReply || got.resp.Message == nil || got.resp.Message.Body != "answer" {
				t.Fatalf("post --wait-reply after the restart = %+v, %v; want the reply at once", got.resp, got.err)
			}
			events := mustCall(t, program, creelengine.Request{Verb: creelengine.VerbLog, ID: reply.ID}).Log
			var names []string
			for _, entry := range events {
				names = append(names, entry.Event)
			}
			if !slices.Equal(names, []string{"posted", "undelivered", "delivered"}) {
				t.Errorf("events of the reply = %v; want posted, undelivered, delivered", names)
			}
		}},
		{"WaiterWhoseDaemonNeverReturnsGivesUpNamingTheSocket", func(t *testing.T, r *rig) {
			first := r.start()
			clientClock := creelfake.NewClock(epoch)
			var spawns atomic.Int32
			waiting := callAsync(r.client(externalClaim(), func() error { spawns.Add(1); return nil }, clientClock), creelengine.Request{Verb: creelengine.VerbPost, Receiver: alpha, Text: "anyone?", Wait: true}, 0)
			r.clock.AwaitWaitsWhere(1, isProbeWait)
			if err := first.stop(); err != nil {
				t.Fatalf("stop: %v", err)
			}
			done := make(chan struct{})
			go clientClock.AutoAdvance(done)
			defer close(done)
			got := result(t, waiting)
			if got.err == nil || !strings.Contains(got.err.Error(), r.geom.SocketPath) || !strings.Contains(got.err.Error(), "gave up after") {
				t.Errorf("Call = %v; want it to give up naming the socket", got.err)
			}
			if spawns.Load() == 0 {
				t.Error("the client never tried to start the daemon")
			}
		}},
		{"DayChangeRunsRetentionFromThePoll", func(t *testing.T, r *rig) {
			r.writeJournal("2026-09-21", `{"event":"posted","at":"2026-09-21T12:00:00Z","id":1,"from":"ly:alpha:driver","to":"ly:BATTEN","body":"old mail"}`)
			r.start()
			r.pollClock.AwaitWaits(1)
			r.clock.Advance(25 * time.Hour)
			r.pollClock.Advance(time.Second)
			r.pollClock.AwaitWaits(2)
			entries, err := os.ReadDir(r.geom.JournalDir)
			if err != nil {
				t.Fatalf("read the journal directory: %v", err)
			}
			var names []string
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			if !slices.Equal(names, []string{"2026-10-02.jsonl"}) {
				t.Fatalf("journal files = %v; want only the new day's", names)
			}
			if text := journalText(t, r.geom); !strings.Contains(text, `"event":"carried"`) || !strings.Contains(text, "old mail") {
				t.Errorf("the new day file lacks the carried unread message: %q", text)
			}
			var carried int
			for _, entry := range mustCall(t, r.client(externalClaim(), nil, creelfake.NewClock(epoch)), creelengine.Request{Verb: creelengine.VerbLog, ID: 1}).Log {
				if entry.Event == "carried" && entry.Kind == "mail" {
					carried++
				}
			}
			if carried != 1 {
				t.Errorf("log --id 1 lists %d carried mail entries after the poll; want 1", carried)
			}
		}},
		{"ReplacedBinaryLetsTheInFlightSendFinishThenRunReturnsAndTheLockIsFree", func(t *testing.T, r *rig) {
			entered, release := make(chan struct{}), make(chan struct{})
			var once, releaseOnce sync.Once
			r.del.SetSendFunc(func(string, string) error {
				once.Do(func() { close(entered) })
				<-release
				return nil
			})
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			daemon := r.start()
			c := r.client(programClaim("ly:BATTEN"), nil, creelfake.NewClock(epoch))
			injected := mustCall(t, c, creelengine.Request{Verb: creelengine.VerbInject, Receiver: alpha, Text: "/clear"})
			<-entered
			r.replaced.Store(true)
			r.pollClock.Advance(time.Second)
			releaseOnce.Do(func() { close(release) })
			select {
			case err := <-daemon.done:
				daemon.err = err
				daemon.once.Do(func() {})
				if err != nil {
					t.Fatalf("Run returned %v; want nil on a replaced binary", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("Run did not return after the binary was replaced")
			}
			held, acquired, err := lock.TryAcquireWriteLock(r.geom.LockPath)
			if err != nil || !acquired {
				t.Fatalf("the lock is not free after Run returned: %v, %v", acquired, err)
			}
			_ = held.Release()
			rebuilt, err := creelengine.OpenStore(r.geom, r.del, r.clock)
			if err != nil {
				t.Fatalf("OpenStore: %v", err)
			}
			defer func() { _ = rebuilt.Close() }()
			var events []string
			for _, entry := range rebuilt.Log(creelengine.LogFilter{ID: injected.ID}) {
				events = append(events, entry.Event)
			}
			if !slices.Equal(events, []string{"posted", "sending", "injected"}) {
				t.Errorf("events of the in-flight inject = %v; want its outcome journaled", events)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t, newRig(t, false))
		})
	}
}
