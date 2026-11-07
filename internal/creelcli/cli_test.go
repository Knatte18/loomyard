// cli_test.go drives the `lym creel` verbs through RunCLIIn over a hubforge hub.
// The daemon runs in process on a fake delivery and fake clocks.
// The rows set the strand name in the environment and replace the package's client seam.
// That seam is process-global state, so the test does not run in parallel.

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
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/hubregistry"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymtemp"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/orchroute"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

var epoch = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// isProbeWait and isSweepWait tell the delivery workers' waits from the sweep's, on the daemon's clock.
// Workers wait 1 to 8 seconds; the sweep waits longer than the probe ceiling.
func isProbeWait(d time.Duration) bool { return d <= 8*time.Second }

func isSweepWait(d time.Duration) bool { return d > 8*time.Second && d <= 50*time.Second }

// isChaseWait tells the chase timer's waits, from one minute to eight, from the workers' and the sweep's.
func isChaseWait(d time.Duration) bool { return d >= time.Minute && d <= 8*time.Minute }

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

// runCreelSplit runs the creel command as runCreel does and returns the stdout run.
// Stderr goes to the writer it is handed.
func runCreelSplit(cwd string, stderr io.Writer, args ...string) cliRun {
	var out bytes.Buffer
	cmd := Command()
	cmd.SetOut(&out)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	code := clihelp.RunRootCtx(lymcwd.WithCwd(context.Background(), cwd), cmd, &out)
	run := cliRun{code: code, raw: out.String()}
	_ = json.Unmarshal([]byte(strings.TrimSpace(run.raw)), &run.fields)
	return run
}

// The hook inputs Claude Code passes a creel hook on standard input, as far as the engine's parse reads them.
const (
	postToolUseInput         = `{"hook_event_name":"PostToolUse"}`
	stopInput                = `{"hook_event_name":"Stop"}`
	stopActiveInput          = `{"hook_event_name":"Stop","stop_hook_active":true}`
	subagentPostToolUseInput = `{"hook_event_name":"PostToolUse","agent_id":"agent-1"}`
	subagentStopInput        = `{"hook_event_name":"Stop","agent_id":"agent-1"}`
)

// runCreelHook runs the creel command as runCreelSplit does with stdin as its standard input.
func runCreelHook(cwd, stdin string, stderr io.Writer, args ...string) cliRun {
	var out bytes.Buffer
	cmd := Command()
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&out)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	code := clihelp.RunRootCtx(lymcwd.WithCwd(context.Background(), cwd), cmd, &out)
	run := cliRun{code: code, raw: out.String()}
	_ = json.Unmarshal([]byte(strings.TrimSpace(run.raw)), &run.fields)
	return run
}

// runHookAsync starts runCreelHook in its own goroutine, for a hook verb that waits.
func runHookAsync(cwd, stdin string, stderr io.Writer, args ...string) <-chan cliRun {
	results := make(chan cliRun, 1)
	go func() { results <- runCreelHook(cwd, stdin, stderr, args...) }()
	return results
}

// newHookRig starts a rig whose daemon knows one hook strand, placed in this process's pane.
// It sets that strand's name in the environment and returns the rig with the name.
func newHookRig(t *testing.T) (*rig, string) {
	t.Helper()
	r := newRig(t)
	alpha := r.name("alpha:driver")
	t.Setenv(agentname.StrandNameEnv, alpha)
	strand := creelfake.HookStrand(alpha)
	strand.PanePID = os.Getpid()
	r.delivery.SetStrands(strand)
	r.startDaemon(t)
	return r, alpha
}

// hookLine is the line a claim hands out for message id from sender with the given body.
func hookLine(id int, sender, body string) string {
	return fmt.Sprintf("[creel] #%d from \"%s\": %s", id, sender, body)
}

// hookBlock is what hook-read prints for text on event: the claude engine's hook output and a newline.
func hookBlock(t *testing.T, event shuttleengine.HookEvent, text string) string {
	t.Helper()
	return hookOutputLine(t, event, text, "")
}

// hookOutputLine is what hook-read prints for mail and echo on event: the claude engine's hook output and a newline.
func hookOutputLine(t *testing.T, event shuttleengine.HookEvent, mail, echo string) string {
	t.Helper()
	output, err := claudeengine.New().HookOutput(event, mail, echo)
	if err != nil {
		t.Fatalf("HookOutput: %v", err)
	}
	return string(output) + "\n"
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

// runCreelInto runs the creel command in cwd, prints to out, and writes stderr to the writer it is handed.
// It returns the exit code.
func runCreelInto(cwd string, out, stderr io.Writer, args ...string) int {
	cmd := Command()
	cmd.SetOut(out)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	return clihelp.RunRootCtx(lymcwd.WithCwd(context.Background(), cwd), cmd, out)
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

// advanceUntilReturned steps clock by each newly recorded wait that keep accepts.
// It stops once the async verb returns and returns its run.
// Each new wait is stepped on as it is recorded,
// so a wait an earlier verb's worker recorded late is never taken for one this verb's own work caused.
func advanceUntilReturned(t *testing.T, clock *creelfake.Clock, keep func(time.Duration) bool, results <-chan cliRun) cliRun {
	t.Helper()
	returned := make(chan struct{})
	forwarded := make(chan cliRun, 1)
	go func() {
		forwarded <- <-results
		close(returned)
	}()
	clock.AdvanceByWaitsWhere(keep, returned)
	return awaitRun(t, forwarded)
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
	root, err := lymtemp.ProcessRoot()
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
	registryDir := t.TempDir()
	seam = clientSeam{
		clock: r.clientClock, spawn: func(string, string) (creelengine.Spawn, error) { return creelengine.Spawn{}, nil }, live: previous.live,
		registryDir: func() (string, error) { return registryDir, nil },
	}
	t.Cleanup(func() { seam = previous })
	return r
}

// addHub gives a row its second hub: a copy of the fixture hub with that hub's geometry, delivery and clock.
// The copy's shortname record is rewritten, since hubforge hubs share one shortname.
// The new rig shares the rig's one seam and client clock, so it starts no daemon until the row starts one.
func (r *rig) addHub(t *testing.T, shortname string) *rig {
	t.Helper()
	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	if err := fabricengine.WriteShortname(fabricengine.BoltDir(hub.Path), shortname); err != nil {
		t.Fatalf("WriteShortname(%q) = %v", shortname, err)
	}
	root, err := lymtemp.ProcessRoot()
	if err != nil {
		t.Fatalf("ProcessRoot() = %v", err)
	}
	geometry, err := hubgeom.CreelGeometry(hub.Path, root.Path())
	if err != nil {
		t.Fatalf("CreelGeometry() = %v", err)
	}
	return &rig{
		hub: hub, cwd: hub.PrimeWorktree(), geometry: geometry, short: shortname,
		delivery: creelfake.NewDelivery(), clock: creelfake.NewClock(epoch), clientClock: r.clientClock,
	}
}

// startDaemon runs the daemon in process on the rig's geometry and returns once it listens.
func (r *rig) startDaemon(t *testing.T) {
	t.Helper()
	root, err := lymtemp.ProcessRoot()
	if err != nil {
		t.Fatalf("ProcessRoot() = %v", err)
	}
	hubs := daemonHubs(r.hub.Path, r.short, root.Path())
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- creelengine.Run(ctx, creelengine.Options{
			Geometry: r.geometry, Delivery: r.delivery, Clock: r.clock, PollClock: creelfake.NewClock(epoch),
			ParentPID: proc.ParentPID, GOOS: runtime.GOOS, Hubs: hubs, Ready: func() { close(ready) },
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
	tiergate.Require(t, 1)
	// LYM_STRAND_NAME and the client seam are process-global, so no row runs in parallel.
	t.Setenv(agentname.StrandNameEnv, "")

	t.Run("every verb refuses a standalone repository naming that creel needs a hub", func(t *testing.T) {
		repo := t.TempDir()
		gitkit.Git(t, repo, "init", "-b", "main")
		verbs := [][]string{
			{"post", "r", "text"}, {"inject", "r", "text"}, {"interrupt", "r"}, {"forward", "1", "r"}, {"read"},
			{"inbox"}, {"log"}, {"hold", "r", "--for", "1m"}, {"release", "r"}, {"hubs"},
		}
		for _, args := range verbs {
			run := runCreel(repo, args...)
			if run.code != 1 || !strings.Contains(run.errorText(), "creel needs a hub") {
				t.Errorf("creel %v in a plain repository = (%d, %q); want exit 1 naming that creel needs a hub", args, run.code, run.raw)
			}
		}
	})

	t.Run("hubs lists the recorded hubs that pass the lookup check and starts no daemon", func(t *testing.T) {
		const xx, yy = "xx", "yy"
		// listed returns the entries of a hubs run by shortname.
		listed := func(t *testing.T, run cliRun) map[string]map[string]any {
			t.Helper()
			if run.code != 0 {
				t.Fatalf("creel hubs = (%d, %q); want exit 0", run.code, run.raw)
			}
			entries, _ := run.fields["hubs"].([]any)
			byName := map[string]map[string]any{}
			for _, entry := range entries {
				fields, _ := entry.(map[string]any)
				byName[fields["shortname"].(string)] = fields
			}
			return byName
		}
		recordHubs := func(t *testing.T, hubs ...*rig) {
			t.Helper()
			registry, err := seam.registryDir()
			if err != nil {
				t.Fatalf("registryDir() = %v", err)
			}
			for _, other := range hubs {
				if err := hubregistry.Record(registry, other.hub.Path, other.short, epoch); err != nil {
					t.Fatalf("Record(%s) = %v", other.short, err)
				}
			}
		}

		t.Run("an empty record prints an empty list", func(t *testing.T) {
			r := newRig(t)
			run := runCreel(r.cwd, "hubs")
			if entries, ok := run.fields["hubs"].([]any); run.code != 0 || !ok || len(entries) != 0 {
				t.Errorf("creel hubs over an empty record = (%d, %q); want exit 0 and an empty list", run.code, run.raw)
			}
		})

		t.Run("a daemon that answers shows true, one that does not shows false and none is started", func(t *testing.T) {
			r := newRig(t)
			up, down := r.addHub(t, xx), r.addHub(t, yy)
			up.startDaemon(t)
			recordHubs(t, up, down)
			byName := listed(t, runCreel(r.cwd, "hubs"))
			if byName[xx]["daemon_answers"] != true || byName[yy]["daemon_answers"] != false || byName[xx]["hub_path"] != up.hub.Path {
				t.Errorf("creel hubs = %v; want %s answering at %s and %s not answering", byName, xx, up.hub.Path, yy)
			}
			if _, err := os.Stat(down.geometry.SocketPath); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the socket of the hub with no daemon after creel hubs: %v; want none, since the verb starts no daemon", err)
			}
		})

		t.Run("a hub whose repository trusted_hubs lists shows it trusted, and a listed shortname or a hub with no repository does not", func(t *testing.T) {
			r := newRig(t)
			trusted, local, bare := r.addHub(t, xx), r.addHub(t, yy), r.addHub(t, "zz")
			gitkit.Git(t, trusted.hub.PrimeWorktree(), "remote", "set-url", "origin", "git@github.com:Owner/xx-repo.git")
			gitkit.Git(t, bare.hub.PrimeWorktree(), "remote", "remove", "origin")
			recordHubs(t, trusted, local, bare)
			writeBoardCreelConfig(t, r.hub.Path, "trusted_hubs:\n  - owner/XX-repo\n  - "+yy+"\n  - zz\n")
			byName := listed(t, runCreel(r.cwd, "hubs"))
			if byName[xx]["trusted"] != true || byName[xx]["repository"] != "Owner/xx-repo" {
				t.Errorf("creel hubs entry of %s = %v; want trusted with the repository Owner/xx-repo", xx, byName[xx])
			}
			wantWhy := map[string]string{yy: "names no GitHub repository", "zz": "no readable origin remote"}
			for shortname, why := range wantWhy {
				entry := byName[shortname]
				reason, _ := entry["no_repository"].(string)
				if entry["trusted"] != false || entry["repository"] != nil || !strings.Contains(reason, why) {
					t.Errorf("creel hubs entry of %s = %v; want it untrusted with no repository and a reason containing %q", shortname, entry, why)
				}
			}
		})

		t.Run("a hub whose shortname record is gone is not listed", func(t *testing.T) {
			r := newRig(t)
			gone, kept := r.addHub(t, xx), r.addHub(t, yy)
			recordHubs(t, gone, kept)
			if err := os.Remove(filepath.Join(fabricengine.BoltDir(gone.hub.Path), fabricengine.ShortnameFileName)); err != nil {
				t.Fatalf("Remove(shortname record) = %v", err)
			}
			byName := listed(t, runCreel(r.cwd, "hubs"))
			if _, found := byName[xx]; found || byName[yy] == nil {
				t.Errorf("creel hubs = %v; want only %s", byName, yy)
			}
		})

		refusals := []struct {
			name  string
			setup func(t *testing.T, r *rig)
			want  []string
		}{
			{"a corrupt record is refused naming its path", func(t *testing.T, _ *rig) {
				registry, _ := seam.registryDir()
				if err := os.WriteFile(filepath.Join(registry, hubregistry.RecordFileName), []byte("{not json"), 0o644); err != nil {
					t.Fatalf("WriteFile(record) = %v", err)
				}
			}, []string{hubregistry.RecordFileName, "way forward"}},
			{"an invalid creel.yaml is refused naming lym config creel", func(t *testing.T, r *rig) {
				writeBoardCreelConfig(t, r.hub.Path, "trusted_hubs: [aa\n")
			}, []string{"lym config creel"}},
			{"a failing registryDir is refused naming its cause", func(t *testing.T, _ *rig) {
				seam.registryDir = func() (string, error) {
					return "", errors.New("no state directory: set XDG_STATE_HOME or HOME")
				}
			}, []string{"XDG_STATE_HOME", "HOME"}},
		}
		for _, tt := range refusals {
			t.Run(tt.name, func(t *testing.T) {
				r := newRig(t)
				tt.setup(t, r)
				run := runCreel(r.cwd, "hubs")
				if run.code != 1 {
					t.Errorf("creel hubs = (%d, %q); want exit 1", run.code, run.raw)
				}
				for _, want := range tt.want {
					if !strings.Contains(run.errorText(), want) {
						t.Errorf("creel hubs error = %q; want it to contain %q", run.errorText(), want)
					}
				}
			})
		}
	})

	t.Run("a post from a strand reaches a strand of another hub, whose read shows the stamp and whose reply ends the sender's wait", func(t *testing.T) {
		r := newRig(t)
		other := r.addHub(t, "xx")
		sender, receiver := r.name("alpha:driver"), other.name("beta:driver")
		for hub, name := range map[*rig]string{r: sender, other: receiver} {
			inPane := creelfake.LiveStrand(name)
			inPane.PanePID = os.Getpid()
			hub.delivery.SetStrands(inPane)
		}
		r.startDaemon(t)
		other.startDaemon(t)

		t.Setenv(agentname.StrandNameEnv, sender)
		waiting := runAsync(r.cwd, "post", "--wait-reply", receiver, "which schema?")
		// The post has read the environment once it is journaled, so the row switches strands after it.
		r.awaitLogged(t, "which schema?")
		t.Setenv(agentname.StrandNameEnv, receiver)
		other.awaitLogged(t, "which schema?")
		read := runCreel(other.cwd, "read")
		if read.code != 0 || !strings.Contains(read.raw, `"`+sender+`" `+creelengine.StampUntrusted) || !strings.Contains(read.raw, "which schema?") {
			t.Fatalf("creel read in the receiving hub = (%d, %q); want the message with %s and the stamp after it", read.code, read.raw, sender)
		}
		id := regexp.MustCompile(`#(\d+)`).FindStringSubmatch(read.raw)
		if id == nil {
			t.Fatalf("creel read = %q; want a message id", read.raw)
		}
		if reply := runCreel(other.cwd, "post", "--reply", id[1], "the schema is v2"); reply.code != 0 {
			t.Fatalf("creel post --reply across hubs = (%d, %q); want exit 0", reply.code, reply.raw)
		}

		answered := awaitRun(t, waiting)
		reply, _ := answered.fields["reply"].(map[string]any)
		if answered.code != 0 || answered.fields["outcome"] != "reply" || reply["body"] != "the schema is v2" ||
			reply["from"] != receiver || reply["trust"] != "untrusted" || reply["sending_hub"] != other.hub.Path {
			t.Errorf("creel post --wait-reply = (%d, %q); want the reply from the bare name %s beside the trust and sending hub fields", answered.code, answered.raw, receiver)
		}
		t.Setenv(agentname.StrandNameEnv, sender)
		mail := runCreel(r.cwd, "read", "--envelope")
		if messages, _ := mail.fields["messages"].([]any); mail.code != 0 || len(messages) != 1 || messages[0].(map[string]any)["body"] != "the schema is v2" {
			t.Errorf("creel read in the sending hub = (%d, %q); want the reply still delivered as mail", mail.code, mail.raw)
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

	t.Run("post refuses a wrong form before any call, naming the three forms or the way forward", func(t *testing.T) {
		r := newRig(t)
		tests := []struct {
			args []string
			want string
		}{
			{[]string{"post", "text"}, "post --parent <text>"},
			{[]string{"post", "--reply", "3", "a", "b"}, "post --parent <text>"},
			{[]string{"post", "--parent", "receiver", "text"}, "post --parent takes the text alone; the forms are: post <receiver> <text>, post --parent <text> and post --reply <id> <text>"},
			{[]string{"post", "--parent", "--reply", "3", "text"}, "post --reply <id> <text> without --parent"},
		}
		for _, tt := range tests {
			run := runCreel(r.cwd, tt.args...)
			if run.code != 1 || !strings.Contains(run.errorText(), tt.want) {
				t.Errorf("creel %v = (%d, %q); want exit 1 naming %q", tt.args, run.code, run.raw, tt.want)
			}
		}
		r.startDaemon(t)
		if log := runCreel(r.cwd, "log"); logHasEvent(log, "posted") {
			t.Errorf("creel log after the refused forms = %q; want no posted event", log.raw)
		}
	})

	t.Run("post --parent posts to the parent orch of the caller's worktree and refuses when none is recorded", func(t *testing.T) {
		r := newRig(t)
		hubforge.AddPairWith(t, r.hub, "b", fabricengine.AddOptions{SkipPush: true, Beam: true})
		beam, err := lymcwd.ResolveWorktree(r.hub.PairCodeWorktree("b"))
		if err != nil {
			t.Fatalf("ResolveWorktree(b) = %v", err)
		}
		beamOrch := r.name("b:orch")
		r.delivery.SetStrands(creelfake.LiveStrand(beamOrch))
		r.startDaemon(t)
		previous := seam.live
		t.Cleanup(func() { seam.live = previous })

		unrecorded := runCreel(r.cwd, "post", "--parent", "nobody home")
		wantRefusal := "creel: post --parent found no orch strand recorded for this worktree's parent; way forward: post to a strand by name, as lym reed list lists them"
		if unrecorded.code != 1 || unrecorded.errorText() != wantRefusal {
			t.Errorf("creel post --parent with no orch recorded = (%d, %q); want exit 1 with %q", unrecorded.code, unrecorded.raw, wantRefusal)
		}
		if log := runCreel(r.cwd, "log"); logHasEvent(log, "posted") {
			t.Errorf("creel log after the unrecorded refusal = %q; want no posted event", log.raw)
		}

		if err := orchengine.SaveState(orchroute.Paths(beam), orchengine.State{Phase: orchengine.PhaseIdle, Strand: "beam-guid"}); err != nil {
			t.Fatalf("SaveState(b) = %v", err)
		}
		seam.live = func(worktree *lymcwd.Location) (bool, error) { return worktree.WorktreeName == "b", nil }
		post := runCreel(r.hub.PairCodeWorktree("b"), "post", "--parent", "which schema?")
		if id, _ := post.fields["id"].(float64); post.code != 0 || id < 1 || post.fields["receiver"] != beamOrch {
			t.Fatalf("creel post --parent from a beam with its orch live = (%d, %q); want exit 0 with receiver %q", post.code, post.raw, beamOrch)
		}
		entries, _ := runCreel(r.cwd, "log").fields["log"].([]any)
		var holders []any
		for _, entry := range entries {
			if fields, _ := entry.(map[string]any); fields["event"] == "posted" {
				holders = append(holders, fields["to"])
			}
		}
		if !reflect.DeepEqual(holders, []any{beamOrch}) {
			t.Errorf("creel log posted receivers = %v; want only %q", holders, beamOrch)
		}
	})

	t.Run("post --interrupt is refused with --reply, and to a marked strand answers its id and interrupted", func(t *testing.T) {
		r := newRig(t)
		alpha := r.name("alpha:driver")
		r.delivery.SetStrands(creelfake.HookStrand(alpha))
		r.startDaemon(t)

		refused := runCreel(r.cwd, "post", "--interrupt", "--reply", "3", "text")
		if refused.code != 1 || !strings.Contains(refused.errorText(), "post --interrupt to the original sender by name") || !strings.Contains(refused.errorText(), "post --reply <id> without --interrupt") {
			t.Errorf("creel post --interrupt --reply = (%d, %q); want exit 1 with its way forward", refused.code, refused.raw)
		}
		post := runCreel(r.cwd, "post", "--interrupt", alpha, "stop, scope changed")
		if id, _ := post.fields["id"].(float64); post.code != 0 || id < 1 || post.fields["interrupted"] != true {
			t.Errorf("creel post --interrupt = (%d, %q); want exit 0 with the message id and interrupted true", post.code, post.raw)
		}
		if got := r.delivery.Interrupts(); !slices.Equal(got, []string{alpha}) {
			t.Errorf("interrupts = %v; want one key into %s", got, alpha)
		}
	})

	t.Run("post --expect-reply marks the message for its receiver, a reply's log entry names the id it answers, and --reply refuses the flag", func(t *testing.T) {
		r := newRig(t)
		alpha := r.name("alpha:driver")
		t.Setenv(agentname.StrandNameEnv, alpha)
		strand := creelfake.LiveStrand(alpha)
		strand.PanePID = os.Getpid()
		r.delivery.SetStrands(strand)
		r.startDaemon(t)

		refused := runCreel(r.cwd, "post", "--expect-reply", "--reply", "3", "text")
		if refused.code != 1 || !strings.Contains(refused.errorText(), "post --reply <id> <text> without --expect-reply") {
			t.Errorf("creel post --expect-reply --reply = (%d, %q); want exit 1 with its way forward", refused.code, refused.raw)
		}
		if log := runCreel(r.cwd, "log"); logHasEvent(log, "posted") {
			t.Errorf("creel log after the refused form = %q; want no posted event", log.raw)
		}

		post := runCreel(r.cwd, "post", "--expect-reply", alpha, "question")
		if id, _ := post.fields["id"].(float64); post.code != 0 || id != 1 {
			t.Fatalf("creel post --expect-reply = (%d, %q); want exit 0 with message 1", post.code, post.raw)
		}
		posted := runCreel(r.cwd, "log", "--id", "1").fields["log"].([]any)[0].(map[string]any)
		if posted["event"] != "posted" || posted["expect_reply"] != true {
			t.Errorf("creel log --id 1 first entry = %v; want the posted event with expect_reply true", posted)
		}
		read := runCreel(r.cwd, "read", "--envelope")
		messages, _ := read.fields["messages"].([]any)
		if read.code != 0 || len(messages) != 1 || messages[0].(map[string]any)["expect_reply"] != true {
			t.Fatalf("creel read --envelope = (%d, %q); want the one message with expect_reply true", read.code, read.raw)
		}

		if reply := runCreel(r.cwd, "post", "--reply", "1", "answer"); reply.code != 0 {
			t.Fatalf("creel post --reply = (%d, %q); want exit 0", reply.code, reply.raw)
		}
		answer := runCreel(r.cwd, "log", "--id", "2").fields["log"].([]any)[0].(map[string]any)
		if answer["event"] != "posted" || answer["reply_to"] != float64(1) || answer["expect_reply"] != nil {
			t.Errorf("creel log --id 2 first entry = %v; want the posted event with reply_to 1 and no mark", answer)
		}
	})

	t.Run("post --wait-reply on a message its holder reads and never answers exits 1 unanswered after two chases, and the log shows their numbers", func(t *testing.T) {
		r, alpha := newHookRig(t)
		waiting := runAsync(r.cwd, "post", "--wait-reply", alpha, "question")
		r.awaitLogged(t, "question")
		if read := runCreel(r.cwd, "read"); read.code != 0 {
			t.Fatalf("creel read = (%d, %q); want exit 0", read.code, read.raw)
		}
		// 15 minutes of running time is a step: two chases, then the step that settles the message.
		for range 44 {
			waits := r.clock.AwaitWaitsWhere(1, isChaseWait)
			r.clock.Advance(waits[len(waits)-1])
			r.clock.AwaitWaitsWhere(len(waits)+1, isChaseWait)
		}
		waits := r.clock.AwaitWaitsWhere(1, isChaseWait)
		r.clock.Advance(waits[len(waits)-1])

		unanswered := awaitRun(t, waiting)
		if unanswered.code != 1 || unanswered.fields["outcome"] != "unanswered" || unanswered.fields["unanswered"] == nil {
			t.Fatalf("creel post --wait-reply = (%d, %q); want exit 1 with outcome unanswered and the message under unanswered", unanswered.code, unanswered.raw)
		}
		for _, want := range []string{"the message got no reply", "recorded it unanswered", "lym creel log --id 1"} {
			if !strings.Contains(unanswered.errorText(), want) {
				t.Errorf("creel post --wait-reply error %q lacks %q", unanswered.errorText(), want)
			}
		}

		var path []string
		var chases []float64
		for _, entry := range runCreel(r.cwd, "log", "--id", "1").fields["log"].([]any) {
			fields := entry.(map[string]any)
			event, _ := fields["event"].(string)
			switch event {
			case "posted":
				if fields["expect_reply"] != true {
					t.Errorf("posted entry = %v; want expect_reply true", fields)
				}
			case "unanswered":
				if fields["reason"] != creelengine.UnansweredNoReply {
					t.Errorf("unanswered entry = %v; want reason %q", fields, creelengine.UnansweredNoReply)
				}
			case "chased":
				chase, _ := fields["chase"].(float64)
				chases = append(chases, chase)
			case "read":
			default:
				continue
			}
			path = append(path, event)
		}
		if want := []string{"posted", "read", "chased", "chased", "unanswered"}; !slices.Equal(path, want) {
			t.Errorf("creel log --id 1 events = %v; want %v", path, want)
		}
		if want := []float64{1, 2}; !slices.Equal(chases, want) {
			t.Errorf("chased entries carry chase numbers %v; want %v", chases, want)
		}
	})

	t.Run("the group help and the post help both carry the reply rule", func(t *testing.T) {
		r := newRig(t)
		for _, args := range [][]string{{"--help"}, {"post", "--help"}} {
			run := runCreel(r.cwd, args...)
			for _, want := range []string{"lym creel post --reply <id>", "EXTERNAL"} {
				if !strings.Contains(run.raw, want) {
					t.Errorf("creel %v printed %q; want it to hold %q", args, run.raw, want)
				}
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
		expiring := runAsync(r.cwd, "inject", "--wait", "--ttl", "1m", alpha, "/clear")
		if expired := advanceUntilReturned(t, r.clock, isProbeWait, expiring); expired.code != ExitInjectExpired {
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
		r.delivery.SetStrands(creelfake.LiveStrand(alpha))
		if returned := advanceUntilReturned(t, r.clock, isSweepWait, waiting); returned.code != ExitReturnedUndelivered {
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
		if reason, _ := timedOut.fields["stall_reason"].(string); !strings.Contains(reason, "lym reed attach") {
			t.Errorf("stall_reason = %q; want the way forward naming lym reed attach", reason)
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
		want := fmt.Sprintf("#1 from \"%[1]s\", posted %[2]s — reply with: lym creel post --reply 1 <text>\nnote to self\n\n#2 from \"%[1]s\", posted %[2]s — reply with: lym creel post --reply 2 <text>\nand another\n", alpha, posted)
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

	t.Run("mail from another hub reads with trust and sending fields beside a bare from, and the log shows the sending hub and its id", func(t *testing.T) {
		r := newRig(t)
		alpha := r.name("alpha:driver")
		t.Setenv(agentname.StrandNameEnv, alpha)
		strand := creelfake.LiveStrand(alpha)
		strand.PanePID = os.Getpid()
		r.delivery.SetStrands(strand)
		if err := os.MkdirAll(r.geometry.JournalDir, 0o755); err != nil {
			t.Fatalf("MkdirAll() = %v", err)
		}
		inbound := fmt.Sprintf(`{"event":"inbound","at":"2026-10-01T11:00:00Z","id":1,"from":"xx:orch","to":%q,"body":"hello","sending_hub":"/hubs/xx","sending_id":7,"trusted":true,"delivered_to":%q}`, alpha, alpha)
		if err := os.WriteFile(filepath.Join(r.geometry.JournalDir, "2026-10-01.jsonl"), []byte(inbound+"\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() = %v", err)
		}
		r.startDaemon(t)

		logged := runCreel(r.cwd, "log", "--id", "1").fields["log"].([]any)[0].(map[string]any)
		if logged["event"] != "inbound" || logged["sending_hub"] != "/hubs/xx" || logged["sending_id"] != float64(7) {
			t.Errorf("creel log --id 1 first entry = %v; want the inbound event with the sending hub's path and id 7", logged)
		}
		read := runCreel(r.cwd, "read", "--envelope")
		messages, _ := read.fields["messages"].([]any)
		if read.code != 0 || len(messages) != 1 {
			t.Fatalf("creel read --envelope = (%d, %q); want exit 0 with one message", read.code, read.raw)
		}
		message, _ := messages[0].(map[string]any)
		if message["from"] != "xx:orch" || message["trust"] != "trusted-orch" || message["sending_hub"] != "/hubs/xx" || message["sending_id"] != float64(7) {
			t.Errorf("creel read --envelope message = %v; want from xx:orch bare beside trust trusted-orch, sending_hub /hubs/xx and sending_id 7", message)
		}
	})

	t.Run("as a strand hook-read prints its mail as the engine's hook output, nothing when none is left, and refuses to stderr", func(t *testing.T) {
		r, alpha := newHookRig(t)

		for _, body := range []string{"first\nline", "second"} {
			if post := runCreel(r.cwd, "post", alpha, body); post.code != 0 {
				t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
			}
		}
		wantBlock := hookBlock(t, shuttleengine.HookEventToolBoundary, fmt.Sprintf("[creel] #1 from \"%[1]s\": first line\n[creel] #2 from \"%[1]s\": second", alpha))
		var stderr bytes.Buffer
		if got := runCreelHook(r.cwd, postToolUseInput, &stderr, "hook-read", "claude"); got.code != 0 || got.raw != wantBlock || stderr.Len() != 0 {
			t.Fatalf("creel hook-read claude = (%d, %q, stderr %q); want exit 0 with %s", got.code, got.raw, stderr.String(), wantBlock)
		}
		if again := runCreelHook(r.cwd, postToolUseInput, &stderr, "hook-read", "claude"); again.code != 0 || again.raw != "" {
			t.Errorf("creel hook-read after the claimed mail = (%d, %q); want exit 0 printing nothing", again.code, again.raw)
		}
		unknown := runCreelHook(r.cwd, postToolUseInput, &stderr, "hook-read", "nope")
		if unknown.code != 1 || unknown.raw != "" || !strings.HasPrefix(stderr.String(), "creel: ") || !strings.Contains(stderr.String(), `"nope"`) {
			t.Errorf("creel hook-read nope = (%d, %q, stderr %q); want exit 1, no stdout and the refusal on stderr", unknown.code, unknown.raw, stderr.String())
		}
	})

	t.Run("hook-read on a Stop hands out one budgeted block per claim, and exits 1 printing nothing with no mail", func(t *testing.T) {
		r, alpha := newHookRig(t)
		bodies := []string{strings.Repeat("a", 5000), strings.Repeat("b", 5000)}
		for _, body := range bodies {
			if post := runCreel(r.cwd, "post", alpha, body); post.code != 0 {
				t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
			}
		}
		var stderr bytes.Buffer
		first := runCreelHook(r.cwd, stopInput, &stderr, "hook-read", "claude")
		if want := hookBlock(t, shuttleengine.HookEventStop, hookLine(1, alpha, bodies[0])); first.code != 0 || first.raw != want {
			t.Fatalf("first Stop = (%d, %q); want exit 0 with the block of the first message", first.code, first.raw)
		}
		second := runCreelHook(r.cwd, stopActiveInput, &stderr, "hook-read", "claude")
		if want := hookBlock(t, shuttleengine.HookEventStop, hookLine(2, alpha, bodies[1])); second.code != 0 || second.raw != want {
			t.Fatalf("Stop with stop_hook_active = (%d, %q); want exit 0 with the block of the second message", second.code, second.raw)
		}
		if empty := runCreelHook(r.cwd, stopInput, &stderr, "hook-read", "claude"); empty.code != 1 || empty.raw != "" || stderr.Len() != 0 {
			t.Errorf("Stop with no mail = (%d, %q, stderr %q); want exit 1 printing nothing on either stream", empty.code, empty.raw, stderr.String())
		}
	})

	t.Run("hook-read after a tool call hands out whole lines up to the claim budget and the next call the one left", func(t *testing.T) {
		r, alpha := newHookRig(t)
		firstPrefix, secondPrefix := hookLine(1, alpha, ""), hookLine(2, alpha, "")
		firstBody := strings.Repeat("a", 4000)
		// The two lines and the newline between them fill the 8 KiB budget exactly.
		secondBody := strings.Repeat("b", 8<<10-1-len(firstPrefix)-len(firstBody)-len(secondPrefix))
		bodies := []string{firstBody, secondBody, "beyond the budget"}
		for _, body := range bodies {
			if post := runCreel(r.cwd, "post", alpha, body); post.code != 0 {
				t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
			}
		}
		var stderr bytes.Buffer
		want := hookBlock(t, shuttleengine.HookEventToolBoundary, hookLine(1, alpha, bodies[0])+"\n"+hookLine(2, alpha, bodies[1]))
		if got := runCreelHook(r.cwd, postToolUseInput, &stderr, "hook-read", "claude"); got.code != 0 || got.raw != want {
			t.Fatalf("first call = (%d, %d bytes); want exit 0 with the block of the two lines that fill the budget", got.code, len(got.raw))
		}
		if got, want := runCreelHook(r.cwd, postToolUseInput, &stderr, "hook-read", "claude"), hookBlock(t, shuttleengine.HookEventToolBoundary, hookLine(3, alpha, bodies[2])); got.code != 0 || got.raw != want {
			t.Errorf("next call = (%d, %q); want exit 0 with the block of the one left", got.code, got.raw)
		}
	})

	t.Run("hook-read from a subagent claims nothing and leaves the mail for the main thread, and refuses another event", func(t *testing.T) {
		r, alpha := newHookRig(t)
		if post := runCreel(r.cwd, "post", alpha, "mail"); post.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
		}
		var stderr bytes.Buffer
		if got := runCreelHook(r.cwd, subagentPostToolUseInput, &stderr, "hook-read", "claude"); got.code != 0 || got.raw != "" || stderr.Len() != 0 {
			t.Errorf("subagent PostToolUse = (%d, %q, stderr %q); want exit 0 printing nothing", got.code, got.raw, stderr.String())
		}
		if got := runCreelHook(r.cwd, subagentStopInput, &stderr, "hook-read", "claude"); got.code != 1 || got.raw != "" || stderr.Len() != 0 {
			t.Errorf("subagent Stop = (%d, %q, stderr %q); want exit 1 printing nothing", got.code, got.raw, stderr.String())
		}
		if got, want := runCreelHook(r.cwd, postToolUseInput, &stderr, "hook-read", "claude"), hookBlock(t, shuttleengine.HookEventToolBoundary, hookLine(1, alpha, "mail")); got.code != 0 || got.raw != want {
			t.Errorf("main-thread call after the subagent calls = (%d, %q); want exit 0 with the mail the subagent left", got.code, got.raw)
		}
		other := runCreelHook(r.cwd, `{"hook_event_name":"SessionStart"}`, &stderr, "hook-read", "claude")
		if other.code != 1 || other.raw != "" || !strings.HasPrefix(stderr.String(), "creel: ") {
			t.Errorf("another event = (%d, %q, stderr %q); want exit 1 with the refusal on stderr", other.code, other.raw, stderr.String())
		}
	})

	t.Run("after a ring hook-read shows the echo to the operator alone, ahead of any mail", func(t *testing.T) {
		r, alpha := newHookRig(t)
		if post := runCreel(r.cwd, "post", alpha, "mail"); post.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
		}
		var rung lockedBuffer
		if got := awaitRun(t, runHookAsync(r.cwd, postToolUseInput, &rung, "hook-wait", "claude")); got.code != 2 {
			t.Fatalf("hook-wait = (%d, %q, stderr %q); want exit 2", got.code, got.raw, rung.String())
		}
		var stderr bytes.Buffer
		echoOnly := hookOutputLine(t, shuttleengine.HookEventStop, "", creelengine.EchoHeading+"\n"+hookLine(1, alpha, "mail"))
		if got := runCreelHook(r.cwd, stopInput, &stderr, "hook-read", "claude"); got.code != 1 || got.raw != echoOnly || stderr.Len() != 0 {
			t.Fatalf("first Stop = (%d, %q, stderr %q); want exit 1 with the echo alone as %q", got.code, got.raw, stderr.String(), echoOnly)
		}
		if got := runCreelHook(r.cwd, stopInput, &stderr, "hook-read", "claude"); got.code != 1 || got.raw != "" || stderr.Len() != 0 {
			t.Fatalf("second Stop = (%d, %q, stderr %q); want exit 1 printing nothing, since the echo was shown", got.code, got.raw, stderr.String())
		}

		// The first ring's woken turn recorded no prompt submit, so a turn end after the ring stands in for the next turn's end.
		r.delivery.SetTurn(alpha, false, r.clock.Now().Add(time.Second))
		if post := runCreel(r.cwd, "post", alpha, "second"); post.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
		}
		var secondRing lockedBuffer
		if got := awaitRun(t, runHookAsync(r.cwd, postToolUseInput, &secondRing, "hook-wait", "claude")); got.code != 2 {
			t.Fatalf("second hook-wait = (%d, %q, stderr %q); want exit 2", got.code, got.raw, secondRing.String())
		}
		if post := runCreel(r.cwd, "post", alpha, "third"); post.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
		}
		both := hookOutputLine(t, shuttleengine.HookEventToolBoundary, hookLine(3, alpha, "third"), creelengine.EchoHeading+"\n"+hookLine(2, alpha, "second"))
		if got := runCreelHook(r.cwd, postToolUseInput, &stderr, "hook-read", "claude"); got.code != 0 || got.raw != both {
			t.Errorf("tool-boundary hook-read = (%d, %q); want exit 0 with the echo of the second post and the third as mail, %q", got.code, got.raw, both)
		}
	})

	t.Run("hook-wait rings a strand as exit 2 with its mail, and a subagent or a refused call exits 0 printing nothing", func(t *testing.T) {
		r, alpha := newHookRig(t)
		if post := runCreel(r.cwd, "post", alpha, "mail"); post.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
		}
		var rung lockedBuffer
		got := awaitRun(t, runHookAsync(r.cwd, postToolUseInput, &rung, "hook-wait", "claude"))
		if want := hookLine(1, alpha, "mail") + "\n"; got.code != 2 || got.raw != "" || rung.String() != want {
			t.Errorf("hook-wait = (%d, %q, stderr %q); want exit 2 with %q on stderr", got.code, got.raw, rung.String(), want)
		}
		if !logHasEvent(runCreel(r.cwd, "log", "--id", "1"), "read") {
			t.Errorf("creel log --id 1 after the ring holds no read event; want the ring's mail journaled read")
		}
		silent := []struct {
			name  string
			input string
			args  []string
		}{
			{"Subagent", subagentPostToolUseInput, []string{"hook-wait", "claude"}},
			{"UnknownEngine", postToolUseInput, []string{"hook-wait", "nope"}},
			{"InputThatIsNotJSON", "not json", []string{"hook-wait", "claude"}},
		}
		for _, tt := range silent {
			var stderr lockedBuffer
			if got := awaitRun(t, runHookAsync(r.cwd, tt.input, &stderr, tt.args...)); got.code != 0 || got.raw != "" || stderr.String() != "" {
				t.Errorf("hook-wait %s = (%d, %q, stderr %q); want exit 0 printing nothing", tt.name, got.code, got.raw, stderr.String())
			}
		}
		// With the strand's pane elsewhere and no strand name, the daemon resolves the caller as EXTERNAL.
		t.Setenv(agentname.StrandNameEnv, "")
		r.delivery.SetStrands(creelfake.HookStrand(alpha))
		var stderr lockedBuffer
		if got := awaitRun(t, runHookAsync(r.cwd, postToolUseInput, &stderr, "hook-wait", "claude")); got.code != 0 || got.raw != "" || stderr.String() != "" {
			t.Errorf("hook-wait from a caller that is no strand = (%d, %q, stderr %q); want exit 0 printing nothing", got.code, got.raw, stderr.String())
		}
	})

	t.Run("hook-wait with no daemon to reach exits 0 printing nothing", func(t *testing.T) {
		r := newRig(t)
		done := make(chan struct{})
		defer close(done)
		go r.clientClock.AutoAdvance(done)
		t.Setenv(agentname.StrandNameEnv, r.name("alpha:driver"))
		var stderr lockedBuffer
		if got := awaitRun(t, runHookAsync(r.cwd, postToolUseInput, &stderr, "hook-wait", "claude")); got.code != 0 || got.raw != "" || stderr.String() != "" {
			t.Errorf("hook-wait with no daemon = (%d, %q, stderr %q); want exit 0 printing nothing", got.code, got.raw, stderr.String())
		}
	})

	t.Run("a hook-wait superseded by a newer one of the strand exits 0 printing nothing", func(t *testing.T) {
		r, alpha := newHookRig(t)
		// A running turn keeps the daemon from ringing, and each registration wakes the worker, whose probe reads the turn.
		r.delivery.SetTurn(alpha, true, time.Time{})
		if post := runCreel(r.cwd, "post", alpha, "mail"); post.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
		}
		r.clock.AwaitWaitsWhere(1, isProbeWait)
		var older, newer lockedBuffer
		start := func(stderr *lockedBuffer) <-chan cliRun {
			probes := r.delivery.IdleCalls(alpha)
			results := runHookAsync(r.cwd, postToolUseInput, stderr, "hook-wait", "claude")
			deadline := time.Now().Add(10 * time.Second)
			for r.delivery.IdleCalls(alpha) == probes {
				if time.Now().After(deadline) {
					t.Fatal("the daemon did not probe after the waiter registered")
				}
				time.Sleep(time.Millisecond)
			}
			return results
		}
		olderResults := start(&older)
		newerResults := start(&newer)
		if got := awaitRun(t, olderResults); got.code != 0 || got.raw != "" || older.String() != "" {
			t.Errorf("superseded hook-wait = (%d, %q, stderr %q); want exit 0 printing nothing", got.code, got.raw, older.String())
		}
		r.delivery.SetTurn(alpha, false, time.Time{})
		if post := runCreel(r.cwd, "post", alpha, "more mail"); post.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
		}
		want := hookLine(1, alpha, "mail") + "\n" + hookLine(2, alpha, "more mail") + "\n"
		if got := awaitRun(t, newerResults); got.code != 2 || newer.String() != want {
			t.Errorf("the newer hook-wait = (%d, %q, stderr %q); want it still waiting, then rung with exit 2 and %q", got.code, got.raw, newer.String(), want)
		}
	})

	t.Run("hook-wait whose stderr refuses the lines exits 0 and leaves the mail unread", func(t *testing.T) {
		r, alpha := newHookRig(t)
		if post := runCreel(r.cwd, "post", alpha, "mail"); post.code != 0 {
			t.Fatalf("creel post = (%d, %q); want exit 0", post.code, post.raw)
		}
		if got := awaitRun(t, runHookAsync(r.cwd, postToolUseInput, failingWriter{}, "hook-wait", "claude")); got.code != 0 || got.raw != "" {
			t.Fatalf("hook-wait over a refusing stderr = (%d, %q); want exit 0 printing nothing", got.code, got.raw)
		}
		// The daemon releases the ring's claim on its own goroutine once it reads the closed connection, so a hook-read before that prints nothing and claims nothing.
		want := hookBlock(t, shuttleengine.HookEventToolBoundary, hookLine(1, alpha, "mail"))
		deadline := time.Now().Add(10 * time.Second)
		for {
			var stderr bytes.Buffer
			got := runCreelHook(r.cwd, postToolUseInput, &stderr, "hook-read", "claude")
			if got.raw == want {
				break
			}
			if got.raw != "" || got.code != 0 {
				t.Fatalf("hook-read after the refused wake = (%d, %q, stderr %q); want nothing until the mail is released, then %q", got.code, got.raw, stderr.String(), want)
			}
			if time.Now().After(deadline) {
				t.Fatal("the mail was never released after the refused wake")
			}
			time.Sleep(time.Millisecond)
		}
		if !logHasEvent(runCreel(r.cwd, "log", "--id", "1"), "read") {
			t.Errorf("creel log --id 1 holds no read event; want the mail journaled read by the hook-read that showed it")
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
			// The daemon may still hold alpha's draft stall from the expired inject as a fresh reading when this wait registers.
			// No probe of alpha has read the cleared stall yet, so a stall note may precede the answer.
			text := timeoutStderr.String()
			lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
			for _, note := range lines[:len(lines)-1] {
				if !strings.Contains(note, "is stalled: ") {
					t.Errorf("stderr line %q before the answer; want only stall notes", note)
				}
			}
			if last := lines[len(lines)-1]; code != ExitWaitTimeout || last != "creel: no outcome arrived before the timeout of 5s" {
				t.Errorf("creel inject --wait --timeout 5s into a failing writer = (%d, %q); want exit %d and a creel: line naming the timeout and no id", code, text, ExitWaitTimeout)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the verb did not return")
		}

		// Busy, so the EXTERNAL mail is not carried into the pane and marked read before beta goes.
		r.delivery.SetIdle(beta, false)
		var returnedStderr lockedBuffer
		returnedCodes := make(chan int, 1)
		postReturned := make(chan struct{})
		go func() {
			returnedCodes <- runCreelInto(r.cwd, failingWriter{}, &returnedStderr, "post", "--wait-reply", beta, "anyone?")
			close(postReturned)
		}()
		r.awaitLogged(t, "anyone?")
		r.delivery.SetStrands(creelfake.LiveStrand(alpha))
		// The probe advances above may have fired the sweep already,
		// so a counted step could land while it runs and drop a sweep;
		// stepping on every new sweep wait until the post returns cannot.
		r.clock.AdvanceByWaitsWhere(isSweepWait, postReturned)
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

	// The Go client rows make the test process the pane of the hub's orch strand.
	// Only that pane may also claim a program name.
	orchRig := func(t *testing.T) (r *rig, orch string, location *lymcwd.Location) {
		t.Helper()
		r = newRig(t)
		orch = r.name("orch")
		t.Setenv(agentname.StrandNameEnv, orch)
		strand := creelfake.LiveStrand(orch)
		strand.PanePID = os.Getpid()
		r.delivery.SetStrands(strand)
		r.startDaemon(t)
		location, err := lymcwd.Resolve(r.cwd)
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
		// The daemon releases the reservation once it sees the connection close, and no signal reaches the test.
		// The test therefore retries on a short tick under a real-time deadline.
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

	t.Run("a program client's ReadWait blocks until a post arrives, hands it to handle and acknowledges it", func(t *testing.T) {
		r, orch, location := orchRig(t)
		program, err := NewProgramClient(location, "BATTEN")
		if err != nil {
			t.Fatalf("NewProgramClient() = %v", err)
		}
		// A program's mailbox is known once it has read.
		if err := program.ReadAll(func([]creelengine.Message) error { return nil }); err != nil {
			t.Fatalf("first ReadAll() = %v", err)
		}
		var read []creelengine.Message
		done := make(chan error, 1)
		go func() {
			done <- program.ReadWait(time.Minute, func(messages []creelengine.Message) error {
				read = messages
				return nil
			})
		}()
		strand, err := NewStrandClient(location, orch)
		if err != nil {
			t.Fatalf("NewStrandClient() = %v", err)
		}
		if _, err := strand.Post(r.short+":BATTEN", "wake up"); err != nil {
			t.Fatalf("Post() = %v", err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("ReadWait() = %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("ReadWait did not return after a post")
		}
		if len(read) != 1 || read[0].Body != "wake up" {
			t.Errorf("ReadWait handed %+v; want the one posted message", read)
		}
		if err := program.ReadAll(func(messages []creelengine.Message) error {
			if len(messages) > 0 {
				t.Errorf("ReadAll after ReadWait handed %+v; want nothing unread", messages)
			}
			return nil
		}); err != nil {
			t.Fatalf("second ReadAll() = %v", err)
		}
	})

	// The refusal rests on the daemon's Linux peer walk, so the row exists on Linux only.
	if runtime.GOOS == "linux" {
		t.Run("a program client in a non-orch strand's pane is refused with an error matching ErrProgramClaimInStrandPane", func(t *testing.T) {
			r := newRig(t)
			strand := creelfake.LiveStrand(r.name("alpha:driver"))
			strand.PanePID = os.Getpid()
			r.delivery.SetStrands(strand)
			r.startDaemon(t)
			location, err := lymcwd.Resolve(r.cwd)
			if err != nil {
				t.Fatalf("Resolve(%q) = %v", r.cwd, err)
			}
			program, err := NewProgramClient(location, "BATTEN")
			if err != nil {
				t.Fatalf("NewProgramClient() = %v", err)
			}
			err = program.ReadAll(func([]creelengine.Message) error { return nil })
			if !errors.Is(err, creelengine.ErrProgramClaimInStrandPane) {
				t.Errorf("ReadAll() from a non-orch strand's pane = %v; want an error matching ErrProgramClaimInStrandPane", err)
			}
		})
	}

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
			location, err := lymcwd.Resolve(r.cwd)
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
		for name, build := range map[string]func(*lymcwd.Location) (*GoClient, error){
			"program": func(l *lymcwd.Location) (*GoClient, error) { return NewProgramClient(l, "BATTEN") },
			"strand":  func(l *lymcwd.Location) (*GoClient, error) { return NewStrandClient(l, "ly:orch") },
		} {
			for _, location := range []*lymcwd.Location{nil, {}} {
				if _, err := build(location); err == nil || !strings.Contains(err.Error(), "way forward") {
					t.Errorf("%s client on %+v = %v; want a refusal naming the way forward", name, location, err)
				}
			}
		}
	})
}
