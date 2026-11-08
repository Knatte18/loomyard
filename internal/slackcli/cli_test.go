// cli_test.go drives the slack verbs over a slackCLI whose seams are fakes.
// Its Slack is an httptest server, so no hub, Slack or creel is reached.

package slackcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// callerPID is the process id of the verb's process in every pane fixture.
const callerPID = 4242

// primeName is the prime worktree's name the relay verb is told.
const primeName = "main"

// fixedClock is a Clock that stands at one instant and never waits.
type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) }

func (fixedClock) After(time.Duration) <-chan time.Time {
	elapsed := make(chan time.Time)
	close(elapsed)
	return elapsed
}

// slackRequest is what the fake Slack saw of one request.
type slackRequest struct {
	path          string
	authorization string
	form          url.Values
}

// requestOptions compare the requests the fake Slack saw, an absent form equal to an empty one.
var requestOptions = []cmp.Option{cmp.AllowUnexported(slackRequest{}), cmpopts.EquateEmpty()}

// fakeLoginSocket is the Socket Mode connection login opens and closes at once; it answers nothing and records its Close.
type fakeLoginSocket struct {
	closed bool
}

func (s *fakeLoginSocket) Read(context.Context) (slackengine.Envelope, error) {
	return slackengine.Envelope{}, nil
}

func (s *fakeLoginSocket) Ack(context.Context, string) error { return nil }

func (s *fakeLoginSocket) Ping(context.Context) error { return nil }

func (s *fakeLoginSocket) Close() error {
	s.closed = true
	return nil
}

// fakeSlack is an httptest server standing in for Slack's Web API.
// It answers each method from a canned body and records the requests.
type fakeSlack struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []slackRequest
}

// newFakeSlack starts a server that answers the methods in answers, as `/<method>` keys, and ok for the rest.
// An auth.test not in answers names the bot B1, and a bots.info not in answers names the app A1.
func newFakeSlack(t *testing.T, answers map[string]string) *fakeSlack {
	t.Helper()
	fake := &fakeSlack{}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fake.mu.Lock()
		fake.requests = append(fake.requests, slackRequest{path: r.URL.Path, authorization: r.Header.Get("Authorization"), form: r.PostForm})
		fake.mu.Unlock()
		body, found := answers[r.URL.Path]
		switch {
		case !found && r.URL.Path == "/auth.test":
			body = `{"ok":true,"bot_id":"B1"}`
		case !found && r.URL.Path == "/bots.info":
			body = `{"ok":true,"bot":{"app_id":"A1"}}`
		case !found:
			body = `{"ok":true}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(fake.server.Close)
	return fake
}

// seen returns a copy of the requests the fake has answered.
func (fake *fakeSlack) seen() []slackRequest {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return append([]slackRequest(nil), fake.requests...)
}

// recordingMailbox is a creel mailbox that records the posts and reads nothing.
type recordingMailbox struct {
	mu    sync.Mutex
	posts []string
}

func (m *recordingMailbox) Post(receiver, body string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.posts = append(m.posts, receiver+" "+body)
	return int64(len(m.posts)), nil
}

func (m *recordingMailbox) ReadAll(func([]creelengine.Message) error) error { return nil }

func (m *recordingMailbox) ReadWait(time.Duration, func([]creelengine.Message) error) error {
	return nil
}

// harness is one slackCLI over fakes, with the knobs a test turns before it runs a verb.
type harness struct {
	t       *testing.T
	dir     string
	geom    slackengine.Geometry
	slack   *fakeSlack
	mailbox *recordingMailbox
	cli     *slackCLI

	// caller is the process the pane check sees.
	caller paneCaller
	// onTerminal makes standard input read as a terminal.
	// It answers the next of terminalLines, then terminalLine, with terminalErr.
	// prompts records every prompt the hidden read printed.
	onTerminal    bool
	terminalLines []string
	terminalLine  string
	terminalErr   error
	prompts       []string
	// appID is the app the fake dial's hello names, and dialErr is what the dial fails with.
	// dials counts the dials made, and dialToken is the app-level token the last one used.
	// sockets records the connections opened.
	appID     string
	dialErr   error
	dials     int
	dialToken string
	sockets   []*fakeLoginSocket
	// liveHubs and liveHubsErr are what the live-hub seam answers; stoppedHubs lists the hubs the stop seam was told.
	liveHubs    []slackengine.Geometry
	liveHubsErr error
	stoppedHubs []string
	// beams, paused and the errors are what the seams answer.
	beams                                     []string
	paused                                    bool
	ensureErr, stopErr, awaitErr, reparentErr error
	// calls logs the seams the verb reached, in order; stopPID and presenceAtAwait are what the stop and await seams saw.
	calls           []string
	stopPID         int
	presenceAtAwait slackengine.Presence
	// installPreRun runs the next verb under the group's pre-run; resolveCalls and wired count what that pre-run did.
	installPreRun bool
	resolveCalls  int
	wired         []bool
}

// noParent reads every process as having no parent, which ends the ancestry walk.
func noParent(int) (int, bool) { return 0, false }

// paneIn describes a process that sits in the pane of the live strand name.
func paneIn(name string) paneCaller {
	strands := []creelengine.Strand{{Name: name, Live: true, PanePID: callerPID}}
	return paneCaller{PID: callerPID, Strands: strands, ParentPID: noParent, GOOS: "linux"}
}

// newHarness builds a harness whose Slack answers from answers.
// Its process sits outside every pane, and the pane check knows every live pane process.
func newHarness(t *testing.T, answers map[string]string) *harness {
	t.Helper()
	dir := t.TempDir()
	hub := filepath.Join(dir, "hub")
	slackDir := filepath.Join(hub, "slack")
	h := &harness{
		t:       t,
		dir:     dir,
		slack:   newFakeSlack(t, answers),
		mailbox: &recordingMailbox{},
		appID:   "A1",
		geom: slackengine.Geometry{
			HubPath:        hub,
			Shortname:      "hub",
			Dir:            slackDir,
			TokenPath:      filepath.Join(dir, "credentials", slackengine.TokenFileName),
			ChannelMapPath: filepath.Join(slackDir, slackengine.ChannelMapFileName),
			SetupPath:      filepath.Join(slackDir, slackengine.SetupFileName),
			PresencePath:   filepath.Join(slackDir, slackengine.PresenceFileName),
			OutboxPath:     filepath.Join(slackDir, slackengine.OutboxFileName),
			RelayStatePath: filepath.Join(slackDir, slackengine.RelayStateFileName),
			LockPath:       filepath.Join(slackDir, slackengine.RelayLockFileName),
			LogPath:        filepath.Join(slackDir, slackengine.RelayLogFileName),
			RunProcDir:     filepath.Join(slackDir, "procs"),
			LymExecutable:  "lym",
		},
		caller: paneCaller{
			PID: callerPID,
			Strands: []creelengine.Strand{
				{Name: "hub:orch", Live: true, PanePID: 7000},
				{Name: "hub:impl", Live: true, PanePID: 7001},
			},
			ParentPID: noParent,
			GOOS:      "linux",
		},
	}
	h.cli = &slackCLI{
		geom:         h.geom,
		slackBaseURL: h.slack.server.URL + "/",
		clock:        fixedClock{},
		resolveLocation: func(string) (*lymcwd.Location, error) {
			h.resolveCalls++
			return nil, errors.New("slack-test: this directory is no worktree")
		},
		wire: func(_ *slackCLI, location *lymcwd.Location) error {
			h.wired = append(h.wired, location == nil)
			return nil
		},
		pane: func() paneCaller { return h.caller },
		readHiddenLine: func(_ io.Reader, prompt string) (string, bool, error) {
			if !h.onTerminal {
				return "", false, nil
			}
			h.prompts = append(h.prompts, prompt)
			line := h.terminalLine
			if len(h.terminalLines) > 0 {
				line, h.terminalLines = h.terminalLines[0], h.terminalLines[1:]
			}
			return line, true, h.terminalErr
		},
		newSlack: func(baseURL, token string) slackChecks {
			return slackengine.NewAPI(baseURL, token, h.slack.server.Client())
		},
		newDialer: func(_, appToken string) slackengine.Dialer {
			return func(context.Context) (slackengine.Socket, string, error) {
				h.dials++
				h.dialToken = appToken
				if h.dialErr != nil {
					return nil, "", h.dialErr
				}
				socket := &fakeLoginSocket{}
				h.sockets = append(h.sockets, socket)
				return socket, h.appID, nil
			}
		},
		liveHubs:   func() ([]slackengine.Geometry, error) { return h.liveHubs, h.liveHubsErr },
		newMailbox: func() slackengine.Mailbox { return h.mailbox },
		ensure: func(slackengine.Geometry) (slackengine.StartOutcome, error) {
			h.calls = append(h.calls, "ensure")
			return slackengine.OutcomeStarted, h.ensureErr
		},
		stop: func(geom slackengine.Geometry, callerPID int) ([]runproc.Ended, error) {
			h.calls = append(h.calls, "stop")
			h.stoppedHubs = append(h.stoppedHubs, geom.HubPath)
			h.stopPID = callerPID
			return nil, h.stopErr
		},
		awaitFirstRead: func(slackengine.Geometry, slackengine.Clock, time.Duration) error {
			h.calls = append(h.calls, "await")
			presence, err := slackengine.ReadPresence(h.geom.PresencePath)
			if err != nil {
				h.t.Errorf("ReadPresence during the await = %v", err)
			}
			h.presenceAtAwait = presence
			return h.awaitErr
		},
		beams:  func() ([]string, error) { return h.beams, nil },
		paused: func() (bool, error) { return h.paused, nil },
		spawnIntermediate: func(_, prime string) error {
			h.calls = append(h.calls, "spawn:"+prime)
			return nil
		},
		awaitReparent: func(intermediate int) error {
			h.calls = append(h.calls, fmt.Sprintf("reparent:%d", intermediate))
			return h.reparentErr
		},
		runRelay: func(_ context.Context, hubPath, prime string) error {
			h.calls = append(h.calls, "run:"+hubPath+":"+prime)
			return nil
		},
	}
	return h
}

// storeCredentials writes the machine's credential file.
func (h *harness) storeCredentials(credentials slackengine.Credentials) {
	h.t.Helper()
	if err := slackengine.WriteCredentials(h.geom.TokenPath, credentials); err != nil {
		h.t.Fatalf("WriteCredentials = %v", err)
	}
}

// configure stores both tokens and a setup, as login and setup leave them.
func (h *harness) configure() {
	h.t.Helper()
	h.storeCredentials(slackengine.Credentials{BotToken: "xoxb-stored", AppToken: "xapp-stored", AppID: "A1"})
	if err := slackengine.WriteSetup(h.geom.SetupPath, slackengine.Setup{ChannelID: "C1", OperatorID: "U1"}); err != nil {
		h.t.Fatalf("WriteSetup = %v", err)
	}
}

// storePresence writes the presence file.
func (h *harness) storePresence(presence slackengine.Presence) {
	h.t.Helper()
	if _, err := slackengine.WritePresence(h.geom.PresencePath, presence, fixedClock{}.Now()); err != nil {
		h.t.Fatalf("WritePresence = %v", err)
	}
}

// presence reads the presence file.
func (h *harness) presence() slackengine.Presence {
	h.t.Helper()
	presence, err := slackengine.ReadPresence(h.geom.PresencePath)
	if err != nil {
		h.t.Fatalf("ReadPresence = %v", err)
	}
	return presence
}

// run executes the slack command with args and standard input stdin, from the harness's directory.
// It decodes the envelope the command printed.
func (h *harness) run(stdin string, args ...string) (int, envelope.Envelope) {
	h.t.Helper()
	cmd := h.cli.command()
	if h.installPreRun {
		cmd.PersistentPreRunE = h.cli.preRun
	}
	cmd.SetIn(strings.NewReader(stdin))
	var out bytes.Buffer
	code := clihelp.ExecuteIn(cmd, h.dir, &out, args)
	return code, envelope.Decode(h.t, out.String())
}

// wantRefused fails the test unless the run exited 1 with an error envelope naming every one of wants.
func wantRefused(t *testing.T, code int, env envelope.Envelope, wants ...string) {
	t.Helper()
	if code != 1 || env.OK {
		t.Errorf("run = exit %d, ok %v; want exit 1 and a refusal", code, env.OK)
	}
	for _, want := range wants {
		if !strings.Contains(env.Error, want) {
			t.Errorf("error = %q; want it to name %q", env.Error, want)
		}
	}
}

// wantOK fails the test unless the run exited 0 with an ok envelope.
func wantOK(t *testing.T, code int, env envelope.Envelope) {
	t.Helper()
	if code != 0 || !env.OK {
		t.Errorf("run = exit %d, ok %v, error %q; want exit 0 and ok", code, env.OK, env.Error)
	}
}

// wantCalls fails the test unless the seams were called as want says, in order.
func (h *harness) wantCalls(want ...string) {
	h.t.Helper()
	if diff := cmp.Diff(want, h.calls); diff != "" {
		h.t.Errorf("seam calls mismatch (-want +got):\n%s", diff)
	}
}

// fileExists reports whether a file stands at path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// setupArgs is the argument list of a valid `setup` run.
var setupArgs = []string{"setup", "--operator", "U1"}

// paneCallers are the callers the pane check tells apart.
// loginOK says whether login and setup, which no strand may run, let the caller through.
// orchVerbOK says the same for start, stop, away and back, which an orch may run.
var paneCallers = []struct {
	name       string
	caller     paneCaller
	loginOK    bool
	orchVerbOK bool
}{
	{"InOrchPane", paneIn("hub:orch"), false, true},
	{"InNonOrchStrandPane", paneIn("hub:impl"), false, false},
	{
		"PaneProcessUnknown",
		paneCaller{PID: callerPID, Strands: []creelengine.Strand{{Name: "hub:impl", Live: true}}, ParentPID: noParent, GOOS: "linux"},
		false, false,
	},
	{
		"StrandsUnreadable",
		paneCaller{PID: callerPID, StrandsErr: errors.New("reed state unreadable"), ParentPID: noParent, GOOS: "linux"},
		false, false,
	},
	{
		"OutsideEveryPaneAllKnown",
		paneCaller{PID: callerPID, Strands: []creelengine.Strand{{Name: "hub:impl", Live: true, PanePID: 7001}}, ParentPID: noParent, GOOS: "linux"},
		true, true,
	},
}

// TestSlackCLI drives every verb of the slack module over fakes.
func TestSlackCLI(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("Login", func(t *testing.T) {
		t.Parallel()

		t.Run("StoresBothTokensWithTheAppIDOnlyAfterSlackAcceptsThem", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			code, env := h.run("  xoxb-good\nxapp-good\n", "login")
			wantOK(t, code, env)
			wantStored := slackengine.Credentials{BotToken: "xoxb-good", AppToken: "xapp-good", AppID: "A1"}
			if stored, err := slackengine.ReadCredentials(h.geom.TokenPath); err != nil || stored != wantStored {
				t.Errorf("ReadCredentials = %+v, %v; want both tokens and the app id: %+v", stored, err, wantStored)
			}
			if info, err := os.Stat(h.geom.TokenPath); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
				t.Errorf("token file mode = %v, %v; want 0600", info.Mode().Perm(), err)
			}
			if fileExists(h.geom.SetupPath) || fileExists(h.geom.PresencePath) {
				t.Error("login wrote a setup or presence file; want the token file alone")
			}
			wantRequests := []slackRequest{
				{path: "/auth.test", authorization: "Bearer xoxb-good"},
				{path: "/bots.info", authorization: "Bearer xoxb-good", form: url.Values{"bot": {"B1"}}},
			}
			if diff := cmp.Diff(wantRequests, h.slack.seen(), requestOptions...); diff != "" {
				t.Errorf("Slack requests mismatch (-want +got):\n%s", diff)
			}
			if h.dials != 1 || h.dialToken != "xapp-good" || len(h.sockets) != 1 || !h.sockets[0].closed {
				t.Errorf("dials = %d with token %q and %d sockets; want one dial with xapp-good, its connection closed", h.dials, h.dialToken, len(h.sockets))
			}
			if len(h.prompts) != 0 {
				t.Errorf("piped login printed prompts %q; want none", h.prompts)
			}
		})

		t.Run("OnATerminalPromptsPerTokenAndFilesEachByItsPrefix", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.onTerminal = true
			h.terminalLines = []string{" xapp-typed ", "xoxb-typed"}
			code, env := h.run("xoxb-piped xapp-piped\n", "login")
			wantOK(t, code, env)
			if diff := cmp.Diff([]string{tokenPrompt, tokenPrompt}, h.prompts); diff != "" {
				t.Errorf("prompts mismatch (-want +got):\n%s", diff)
			}
			wantStored := slackengine.Credentials{BotToken: "xoxb-typed", AppToken: "xapp-typed", AppID: "A1"}
			if stored, err := slackengine.ReadCredentials(h.geom.TokenPath); err != nil || stored != wantStored {
				t.Errorf("ReadCredentials = %+v, %v; want the trimmed lines read at the prompts: %+v", stored, err, wantStored)
			}
		})

		terminalRefusals := []struct {
			name  string
			lines []string
			err   error
			wants []string
		}{
			{"AnEmptyLineIsRefusedNamingThePrompt", []string{" "}, nil, []string{"no token entered at the prompt", "lym slack login"}},
			{"AnEmptySecondLineIsRefusedNamingThePrompt", []string{"xoxb-typed", " "}, nil, []string{"no token entered at the prompt", "lym slack login"}},
			{"AnInterruptedReadIsRefusedNamingLogin", nil, errors.New("interrupted"), []string{"interrupted", "nothing was stored", "lym slack login"}},
		}
		for _, tt := range terminalRefusals {
			t.Run("OnATerminal"+tt.name, func(t *testing.T) {
				t.Parallel()
				h := newHarness(t, nil)
				h.onTerminal = true
				h.terminalLines, h.terminalErr = tt.lines, tt.err
				code, env := h.run("", "login")
				wantRefused(t, code, env, tt.wants...)
				if len(h.slack.seen()) != 0 || h.dials != 0 || fileExists(h.geom.TokenPath) {
					t.Error("a refused terminal read reached Slack or the token file; want neither")
				}
			})
		}

		kindRefusals := []struct {
			name  string
			stdin string
		}{
			{"AUserTokenIsRefused", "xoxp-user xapp-good\n"},
			{"TwoBotTokensAreRefused", "xoxb-one xoxb-two\n"},
			{"ALoneBotTokenIsRefused", "xoxb-good\n"},
		}
		for _, tt := range kindRefusals {
			t.Run(tt.name+"NamingBothKindsWithNoSlackCallAndNoFile", func(t *testing.T) {
				t.Parallel()
				h := newHarness(t, nil)
				code, env := h.run(tt.stdin, "login")
				wantRefused(t, code, env, "bot token (xoxb-)", "app-level token (xapp-)", "lym slack login")
				if len(h.slack.seen()) != 0 || h.dials != 0 || fileExists(h.geom.TokenPath) {
					t.Error("a refused token set reached Slack or the token file; want neither")
				}
			})
		}

		t.Run("AHelloFromAnotherAppIsRefusedAndStoresNothing", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.appID = "A2"
			code, env := h.run("xoxb-good xapp-good\n", "login")
			wantRefused(t, code, env, "A1", "A2", "lym slack login")
			if fileExists(h.geom.TokenPath) {
				t.Error("login stored tokens of two different apps; want no file")
			}
		})

		rejections := []struct {
			name    string
			answers map[string]string
			dialErr error
			wants   []string
		}{
			{"ARefusedBotTokenNamesTheOAuthPage", map[string]string{"/auth.test": `{"ok":false,"error":"invalid_auth"}`}, nil, []string{"bot token (xoxb-)", "invalid_auth", "OAuth & Permissions", "lym slack login"}},
			{"ARefusedBotsInfoNamesTheBotToken", map[string]string{"/bots.info": `{"ok":false,"error":"invalid_auth"}`}, nil, []string{"bot token (xoxb-)", "invalid_auth"}},
			{"ARefusedConnectionsOpenNamesTheAppLevelToken", nil, &slackengine.APIError{Method: "apps.connections.open", Code: "invalid_auth"}, []string{"app-level token (xapp-)", "invalid_auth", "Basic Information"}},
			{"ATransportErrorOnTheDialNamesTheNetwork", nil, errors.New("dial tcp: connection refused"), []string{"app-level token (xapp-)", "network"}},
		}
		for _, tt := range rejections {
			t.Run(tt.name+"AndStoresNothing", func(t *testing.T) {
				t.Parallel()
				h := newHarness(t, tt.answers)
				h.dialErr = tt.dialErr
				code, env := h.run("xoxb-bad xapp-bad\n", "login")
				wantRefused(t, code, env, tt.wants...)
				if strings.Contains(env.Error, "xoxb-bad") || strings.Contains(env.Error, "xapp-bad") || fileExists(h.geom.TokenPath) {
					t.Errorf("refusal %q or the token file leaks a rejected token", env.Error)
				}
			})
		}

		t.Run("EmptyStandardInputIsRefusedWithoutAskingSlack", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			code, env := h.run(" \n", "login")
			wantRefused(t, code, env, "no token on standard input")
			if len(h.slack.seen()) != 0 || h.dials != 0 || fileExists(h.geom.TokenPath) {
				t.Error("an empty token reached Slack or the token file; want neither")
			}
		})

		liveHubs := []slackengine.Geometry{{HubPath: "/hubs/a"}, {HubPath: "/hubs/b"}}
		replacements := []struct {
			name      string
			stored    slackengine.Credentials
			stdin     string
			wantEnded []string
		}{
			{"AFirstLoginEndsNoRelay", slackengine.Credentials{}, "xoxb-new xapp-new\n", nil},
			{"RepeatedTokensEndNoRelay", slackengine.Credentials{BotToken: "xoxb-same", AppToken: "xapp-same", AppID: "A1"}, "xoxb-same xapp-same\n", nil},
			{"AReplacedBotTokenEndsEveryLiveHubsRelay", slackengine.Credentials{BotToken: "xoxb-old", AppToken: "xapp-same", AppID: "A1"}, "xoxb-new xapp-same\n", []string{"/hubs/a", "/hubs/b"}},
			{"AReplacedAppLevelTokenEndsEveryLiveHubsRelay", slackengine.Credentials{BotToken: "xoxb-same", AppToken: "xapp-old", AppID: "A1"}, "xoxb-same xapp-new\n", []string{"/hubs/a", "/hubs/b"}},
		}
		for _, tt := range replacements {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newHarness(t, nil)
				h.liveHubs = liveHubs
				if tt.stored != (slackengine.Credentials{}) {
					h.storeCredentials(tt.stored)
				}
				code, env := h.run(tt.stdin, "login")
				wantOK(t, code, env)
				if diff := cmp.Diff(tt.wantEnded, h.stoppedHubs, cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("stopped hubs mismatch (-want +got):\n%s", diff)
				}
				var named []string
				items, _ := env.Raw["relays_ended"].([]any)
				for _, item := range items {
					hub, _ := item.(string)
					named = append(named, hub)
				}
				if diff := cmp.Diff(tt.wantEnded, named, cmpopts.EquateEmpty()); diff != "" {
					t.Errorf("relays_ended mismatch (-want +got):\n%s", diff)
				}
				wayForward, _ := env.Raw["way_forward"].(string)
				if hasWay := strings.Contains(wayForward, "lym slack start"); hasWay != (len(tt.wantEnded) > 0) {
					t.Errorf("way_forward = %q; want `lym slack start` named exactly when a relay was ended", wayForward)
				}
				if len(tt.wantEnded) > 0 && h.stopPID != os.Getpid() {
					t.Errorf("stop spared pid %d; want this process %d", h.stopPID, os.Getpid())
				}
			})
		}

		t.Run("AFailedLiveHubReadAfterTheWriteStoresBothTokensAndNamesTheWayForward", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.storeCredentials(slackengine.Credentials{BotToken: "xoxb-old", AppToken: "xapp-same", AppID: "A1"})
			h.liveHubsErr = errors.New("hubs record unreadable")
			code, env := h.run("xoxb-new xapp-same\n", "login")
			wantRefused(t, code, env, "hubs record unreadable", "tokens are stored", "lym slack stop", "lym slack start")
			wantStored := slackengine.Credentials{BotToken: "xoxb-new", AppToken: "xapp-same", AppID: "A1"}
			if stored, err := slackengine.ReadCredentials(h.geom.TokenPath); err != nil || stored != wantStored {
				t.Errorf("ReadCredentials = %+v, %v; want the new tokens stored: %+v", stored, err, wantStored)
			}
			h.wantCalls()
		})
	})

	t.Run("Setup", func(t *testing.T) {
		t.Parallel()

		t.Run("RecordsTheOperatorSlackConfirms", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.storeCredentials(slackengine.Credentials{BotToken: "xoxb-stored"})
			code, env := h.run("", setupArgs...)
			wantOK(t, code, env)
			want := slackengine.Setup{OperatorID: "U1"}
			if got, found, err := slackengine.ReadSetup(h.geom.SetupPath); err != nil || !found || got != want {
				t.Errorf("ReadSetup = %+v, %v, %v; want %+v", got, found, err, want)
			}
			wantRequests := []slackRequest{
				{path: "/users.info", authorization: "Bearer xoxb-stored", form: url.Values{"user": {"U1"}}},
			}
			if diff := cmp.Diff(wantRequests, h.slack.seen(), requestOptions...); diff != "" {
				t.Errorf("Slack requests mismatch (-want +got):\n%s", diff)
			}
		})

		t.Run("KeepsTheChannelAnOlderSetupFileHolds", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.storeCredentials(slackengine.Credentials{BotToken: "xoxb-stored"})
			if err := os.MkdirAll(filepath.Dir(h.geom.SetupPath), 0o700); err != nil {
				t.Fatalf("MkdirAll = %v", err)
			}
			if err := os.WriteFile(h.geom.SetupPath, []byte(`{"channel_id":"C9"}`), 0o600); err != nil {
				t.Fatalf("WriteFile = %v", err)
			}
			code, env := h.run("", setupArgs...)
			wantOK(t, code, env)
			want := slackengine.Setup{ChannelID: "C9", OperatorID: "U1"}
			if got, found, err := slackengine.ReadSetup(h.geom.SetupPath); err != nil || !found || got != want {
				t.Errorf("ReadSetup = %+v, %v, %v; want %+v", got, found, err, want)
			}
		})

		t.Run("TheChannelFlagIsRefusedAsUnknown", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.storeCredentials(slackengine.Credentials{BotToken: "xoxb-stored"})
			code, env := h.run("", "setup", "--channel", "C1", "--operator", "U1")
			wantRefused(t, code, env, "unknown flag")
			if len(h.slack.seen()) != 0 || fileExists(h.geom.SetupPath) {
				t.Error("a setup with an unknown flag reached Slack or wrote the setup file; want neither")
			}
		})

		refusals := []struct {
			name    string
			token   bool
			answers map[string]string
			wants   []string
		}{
			{"MissingScopeNamesTheScopes", true, map[string]string{"/users.info": `{"ok":false,"error":"missing_scope"}`}, []string{"channels:history", "channels:join", "channels:manage", "channels:read", "chat:write", "chat:write.customize", "users:read", "docs/slack-app.md", "reinstall"}},
			{"UserNotFoundNamesTheMemberId", true, map[string]string{"/users.info": `{"ok":false,"error":"user_not_found"}`}, []string{"member id", "user_not_found"}},
			{"NoTokenNamesLogin", false, nil, []string{"lym slack login"}},
		}
		for _, tt := range refusals {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newHarness(t, tt.answers)
				if tt.token {
					h.storeCredentials(slackengine.Credentials{BotToken: "xoxb-stored"})
				}
				code, env := h.run("", setupArgs...)
				wantRefused(t, code, env, tt.wants...)
				if fileExists(h.geom.SetupPath) {
					t.Error("a refused setup wrote the setup file")
				}
			})
		}
	})

	t.Run("PaneCheck", func(t *testing.T) {
		t.Parallel()

		t.Run("LoginAndSetupRefuseEveryStrandAndEveryUnknownPane", func(t *testing.T) {
			t.Parallel()
			for _, tc := range paneCallers {
				if tc.loginOK {
					continue
				}
				for _, args := range [][]string{{"login"}, setupArgs} {
					h := newHarness(t, nil)
					h.caller = tc.caller
					code, env := h.run("xoxb-x\n", args...)
					wantRefused(t, code, env, "way forward")
					if len(h.slack.seen()) != 0 || fileExists(h.geom.TokenPath) || fileExists(h.geom.SetupPath) {
						t.Errorf("%s %v: a refused verb reached Slack or wrote a file", tc.name, args)
					}
				}
			}
		})

		verbs := []struct {
			name         string
			args         []string
			initial      slackengine.Presence
			wantCalls    []string
			wantPresence slackengine.Presence
		}{
			{"Start", []string{"start"}, slackengine.PresenceTerminal, []string{"ensure"}, slackengine.PresenceTerminal},
			{"Stop", []string{"stop"}, slackengine.PresenceSlack, []string{"stop"}, slackengine.PresenceTerminal},
			{"Away", []string{"away"}, slackengine.PresenceTerminal, []string{"ensure", "await"}, slackengine.PresenceSlack},
			{"Back", []string{"back"}, slackengine.PresenceSlack, nil, slackengine.PresenceTerminal},
		}
		for _, verb := range verbs {
			t.Run(verb.name+"RunsOnlyForAnOrchOrAProcessOutsideEveryKnownPane", func(t *testing.T) {
				t.Parallel()
				for _, tc := range paneCallers {
					h := newHarness(t, nil)
					h.configure()
					h.storePresence(verb.initial)
					h.caller = tc.caller
					code, env := h.run("", verb.args...)
					if tc.orchVerbOK {
						wantOK(t, code, env)
						h.wantCalls(verb.wantCalls...)
						if got := h.presence(); got != verb.wantPresence {
							t.Errorf("%s: presence = %q; want %q", tc.name, got, verb.wantPresence)
						}
						continue
					}
					wantRefused(t, code, env, "way forward")
					h.wantCalls()
					if got := h.presence(); got != verb.initial {
						t.Errorf("%s: a refused verb moved the presence to %q; want %q", tc.name, got, verb.initial)
					}
				}
			})
		}

		t.Run("OffLinuxTheStrandNameDecides", func(t *testing.T) {
			t.Parallel()
			strands := []creelengine.Strand{{Name: "hub:orch", Live: true}, {Name: "hub:impl", Live: true}}
			for name, want := range map[string]bool{"hub:impl": false, "hub:orch": true, "": true} {
				h := newHarness(t, nil)
				h.configure()
				h.caller = paneCaller{PID: callerPID, Strands: strands, GOOS: "darwin", StrandName: name}
				code, env := h.run("", "start")
				if want {
					wantOK(t, code, env)
					h.wantCalls("ensure")
					continue
				}
				wantRefused(t, code, env, "hub:impl")
				h.wantCalls()
			}
		})
	})

	t.Run("StartAndAwayNeedLoginAndSetup", func(t *testing.T) {
		t.Parallel()
		for _, verb := range []string{"start", "away"} {
			h := newHarness(t, nil)
			code, env := h.run("", verb)
			wantRefused(t, code, env, "lym slack login")

			h.storeCredentials(slackengine.Credentials{BotToken: "xoxb-stored"})
			code, env = h.run("", verb)
			wantRefused(t, code, env, "app-level token (xapp-)", "lym slack login")

			h.storeCredentials(slackengine.Credentials{BotToken: "xoxb-stored", AppToken: "xapp-stored", AppID: "A1"})
			code, env = h.run("", verb)
			wantRefused(t, code, env, "lym slack setup")
			h.wantCalls()
		}
	})

	t.Run("Away", func(t *testing.T) {
		t.Parallel()

		t.Run("SetsSlackOnlyAfterTheRelayHasReadCreel", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.configure()
			code, env := h.run("", "away")
			wantOK(t, code, env)
			if h.presenceAtAwait != slackengine.PresenceTerminal || h.presence() != slackengine.PresenceSlack {
				t.Errorf("presence during the await = %q and after = %q; want terminal then slack", h.presenceAtAwait, h.presence())
			}
		})

		t.Run("RefusesWithTheAwaitsMessageAndStaysTerminal", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.configure()
			h.awaitErr = errors.New("slack: the relay made no first creel read; run `lym slack away` again")
			code, env := h.run("", "away")
			wantRefused(t, code, env, "no first creel read", "lym slack away")
			if got := h.presence(); got != slackengine.PresenceTerminal {
				t.Errorf("presence = %q; want terminal", got)
			}
		})
	})

	t.Run("Stop", func(t *testing.T) {
		t.Parallel()

		t.Run("SetsTerminalTellsTheOrchAndEndsTheRelayOnceSparingItself", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.configure()
			h.storePresence(slackengine.PresenceSlack)
			code, env := h.run("", "stop")
			wantOK(t, code, env)
			h.wantCalls("stop")
			if requests := h.slack.seen(); len(requests) != 0 {
				t.Errorf("stop made Slack requests %+v; want none, since stop archives nothing", requests)
			}
			if h.stopPID != os.Getpid() {
				t.Errorf("stop spared pid %d; want this process %d", h.stopPID, os.Getpid())
			}
			if diff := cmp.Diff([]string{"hub:orch [slack presence] presence is now terminal"}, h.mailbox.posts); diff != "" {
				t.Errorf("notices mismatch (-want +got):\n%s", diff)
			}
		})

		t.Run("AStopErrorNamesStopAndLeavesTerminal", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.configure()
			h.storePresence(slackengine.PresenceSlack)
			h.stopErr = errors.New("a process would not end")
			code, env := h.run("", "stop")
			wantRefused(t, code, env, "a process would not end", "lym slack stop")
			if got := h.presence(); got != slackengine.PresenceTerminal {
				t.Errorf("presence = %q; want terminal, so a second stop sends no notice", got)
			}
		})
	})

	t.Run("Status", func(t *testing.T) {
		t.Parallel()

		t.Run("AHubWithNoPresenceFilePrintsTerminal", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.caller = paneIn("hub:impl")
			code, env := h.run("", "status")
			wantOK(t, code, env)
			want := map[string]any{"presence": "terminal", "relay_live": false, "paused": false}
			if diff := cmp.Diff(want, env.Raw); diff != "" {
				t.Errorf("status fields mismatch (-want +got):\n%s", diff)
			}
		})

		t.Run("PrintsPresenceRelayPauseOperatorChannelsAndLastHealthy", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.configure()
			h.storePresence(slackengine.PresenceSlack)
			h.paused = true
			if err := os.MkdirAll(filepath.Dir(h.geom.LockPath), 0o700); err != nil {
				t.Fatalf("MkdirAll = %v", err)
			}
			held, acquired, err := lock.TryAcquireWriteLock(h.geom.LockPath)
			if err != nil || !acquired {
				t.Fatalf("TryAcquireWriteLock = %v, %v; want the relay lock", acquired, err)
			}
			t.Cleanup(func() { _ = held.Release() })
			err = slackengine.UpdateChannelMap(h.geom.ChannelMapPath, func(channelMap *slackengine.ChannelMap) error {
				channelMap.Channels["hub:orch"] = slackengine.ChannelEntry{ID: "C1", Name: "ly-neve"}
				channelMap.Channels["hub:impl:orch"] = slackengine.ChannelEntry{ID: "C2", Name: "ly-neve-impl-2026-11-05", Archived: true}
				return nil
			})
			if err != nil {
				t.Fatalf("UpdateChannelMap = %v", err)
			}
			if err := os.WriteFile(h.geom.RelayStatePath, []byte(`{"last_slack_ok":"2026-11-05T11:00:00Z"}`), 0o600); err != nil {
				t.Fatalf("WriteFile = %v", err)
			}
			code, env := h.run("", "status")
			wantOK(t, code, env)
			want := map[string]any{
				"presence": "slack", "relay_live": true, "paused": true,
				"operator": "U1", "last_healthy": "2026-11-05T11:00:00Z",
				"channels": []any{
					map[string]any{"orch": "hub:impl:orch", "name": "ly-neve-impl-2026-11-05", "id": "C2", "archived": true},
					map[string]any{"orch": "hub:orch", "name": "ly-neve", "id": "C1", "archived": false},
				},
			}
			if diff := cmp.Diff(want, env.Raw); diff != "" {
				t.Errorf("status fields mismatch (-want +got):\n%s", diff)
			}
		})

		t.Run("AnUnreadableChannelMapIsRefusedNamingItAndMovingItAside", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			if err := os.MkdirAll(filepath.Dir(h.geom.ChannelMapPath), 0o700); err != nil {
				t.Fatalf("MkdirAll = %v", err)
			}
			if err := os.WriteFile(h.geom.ChannelMapPath, []byte(`{`), 0o600); err != nil {
				t.Fatalf("WriteFile = %v", err)
			}
			code, env := h.run("", "status")
			wantRefused(t, code, env, h.geom.ChannelMapPath, "move the file aside")
		})
	})

	t.Run("Relay", func(t *testing.T) {
		t.Parallel()

		t.Run("ForkSpawnsTheIntermediateWithThePrimeAndNothingElse", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			code, env := h.run("", "relay", "--hub-path", h.geom.HubPath, "--prime", primeName, "--fork")
			wantOK(t, code, env)
			h.wantCalls("spawn:" + primeName)
		})

		t.Run("AfterWaitsForTheReparentBeforeTheRelayRuns", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			code, env := h.run("", "relay", "--hub-path", h.geom.HubPath, "--prime", primeName, "--after", "123")
			wantOK(t, code, env)
			h.wantCalls("reparent:123", "run:"+h.geom.HubPath+":"+primeName)
		})

		t.Run("AReparentErrorExitsWithoutRunningTheRelay", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.reparentErr = errors.New("still a child of its start process")
			code, env := h.run("", "relay", "--hub-path", h.geom.HubPath, "--prime", primeName, "--after", "123")
			wantRefused(t, code, env, "still a child")
			h.wantCalls("reparent:123")
		})

		t.Run("ARelativeHubPathIsRefused", func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			code, env := h.run("", "relay", "--hub-path", "hub", "--prime", primeName)
			wantRefused(t, code, env, "absolute")
			h.wantCalls()
		})
	})

	t.Run("PreRunResolvesNoWorktreeForTheRelayButDoesForEveryOtherVerb", func(t *testing.T) {
		t.Parallel()
		h := newHarness(t, nil)
		h.installPreRun = true

		code, env := h.run("", "relay", "--hub-path", h.geom.HubPath, "--prime", primeName, "--after", "7")
		wantOK(t, code, env)
		h.wantCalls("reparent:7", "run:"+h.geom.HubPath+":"+primeName)
		if h.resolveCalls != 0 {
			t.Errorf("relay resolved a worktree %d times; want none", h.resolveCalls)
		}

		code, env = h.run("", "status")
		wantRefused(t, code, env, "no worktree")
		if h.resolveCalls != 1 || len(h.wired) != 1 || !h.wired[0] {
			t.Errorf("resolver calls = %d, wiring nil-location calls = %v; want one resolve, and only the relay wired", h.resolveCalls, h.wired)
		}
	})
}
