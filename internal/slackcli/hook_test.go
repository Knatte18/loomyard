// hook_test.go covers the hidden hook verb over the real claude engine's hook payloads and a fake strand client,
// and the cut of a reply to creel's line limit.

package slackcli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// hookJSON renders fields as the JSON object Claude Code passes a hook on standard input.
func hookJSON(fields map[string]any) string {
	payload, err := json.Marshal(fields)
	if err != nil {
		panic(err)
	}
	return string(payload)
}

// promptInput is the payload of a UserPromptSubmit hook.
func promptInput(prompt string) string {
	return hookJSON(map[string]any{"hook_event_name": "UserPromptSubmit", "prompt": prompt})
}

// stopInput is the payload of a Stop hook.
func stopInput(reply string) string {
	return hookJSON(map[string]any{"hook_event_name": "Stop", "last_assistant_message": reply})
}

func TestHook(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	injected := func(event, subject string) []creelengine.LogEntry {
		return []creelengine.LogEntry{{Event: event, To: "hub:orch", Subject: subject}}
	}
	tests := []struct {
		name         string
		engine       string
		stdin        string
		strand       string
		presence     slackengine.Presence
		entries      []creelengine.LogEntry
		logErr       error
		postErr      error
		clientErr    error
		buttons      []string
		wantAction   string
		wantRefusal  []string
		wantPresence slackengine.Presence
		wantLogged   bool
		wantPosts    []sentPost
	}{
		{name: "PromptSlashCommandIsLymsLine", stdin: promptInput("/compact"), presence: slackengine.PresenceSlack,
			wantAction: "lym-line", wantPresence: slackengine.PresenceSlack},
		{name: "PromptOpeningCreelPrefixIsLymsLine", stdin: promptInput("[creel] mail"), presence: slackengine.PresenceSlack,
			wantAction: "lym-line", wantPresence: slackengine.PresenceSlack},
		{name: "PromptOpeningLymPrefixIsLymsLine", stdin: promptInput("[lym] Your last turn ended on an API error. Carry on."),
			presence: slackengine.PresenceSlack, wantAction: "lym-line", wantPresence: slackengine.PresenceSlack},
		{name: "PromptTaskNotificationIsLymsLine", stdin: promptInput("<task-notification>done</task-notification>"),
			presence: slackengine.PresenceSlack, wantAction: "lym-line", wantPresence: slackengine.PresenceSlack},
		{name: "PromptWhilePresenceIsTerminalDoesNothing", stdin: promptInput("hello"), presence: slackengine.PresenceTerminal,
			wantAction: "none", wantPresence: slackengine.PresenceTerminal},
		{name: "PromptOutsideAnOrchIsRefused", stdin: promptInput("hello"), strand: "hub:impl", presence: slackengine.PresenceSlack,
			wantRefusal: []string{"no orch's pane", "hub:impl"}, wantPresence: slackengine.PresenceSlack},
		{name: "PromptWhoseStrandClientCannotBeBuiltIsRefused", stdin: promptInput("hello"), presence: slackengine.PresenceSlack,
			clientErr: errors.New("creel is down"), wantRefusal: []string{"creel is down"}, wantPresence: slackengine.PresenceSlack},
		{name: "PromptMatchingAnInjectedLineIsNotTheOperators", stdin: promptInput("hello"), presence: slackengine.PresenceSlack,
			entries: injected("injected", "  hello "), wantAction: "inject", wantPresence: slackengine.PresenceSlack, wantLogged: true},
		{name: "PromptMatchingASendingInjectIsNotTheOperators", stdin: promptInput("hello"), presence: slackengine.PresenceSlack,
			entries: injected("sending", "hello"), wantAction: "inject", wantPresence: slackengine.PresenceSlack, wantLogged: true},
		{name: "PromptMatchingAnUnconfirmedInjectIsNotTheOperators", stdin: promptInput("hello"), presence: slackengine.PresenceSlack,
			entries: injected("unconfirmed", "hello"), wantAction: "inject", wantPresence: slackengine.PresenceSlack, wantLogged: true},
		{name: "PromptMatchingAnEntryThatTypedNothingSetsTerminal", stdin: promptInput("hello"), presence: slackengine.PresenceSlack,
			entries: injected("posted", "hello"), wantAction: "presence-terminal", wantPresence: slackengine.PresenceTerminal, wantLogged: true},
		{name: "PromptOfAnotherSubjectSetsTerminal", stdin: promptInput("hello"), presence: slackengine.PresenceSlack,
			entries: injected("injected", "goodbye"), wantAction: "presence-terminal", wantPresence: slackengine.PresenceTerminal, wantLogged: true},
		{name: "PromptWithNoInjectSetsTerminal", stdin: promptInput("hello"), presence: slackengine.PresenceSlack,
			wantAction: "presence-terminal", wantPresence: slackengine.PresenceTerminal, wantLogged: true},
		{name: "PromptWhoseLogCannotBeReadIsRefusedAndKeepsSlack", stdin: promptInput("hello"), presence: slackengine.PresenceSlack,
			logErr: errors.New("journal unreadable"), wantRefusal: []string{"read creel's injects into hub:orch", "journal unreadable"},
			wantPresence: slackengine.PresenceSlack, wantLogged: true},
		{name: "StopForwardsTheTrimmedReplyToTheSlackProgram", stdin: stopInput(" done "), presence: slackengine.PresenceSlack,
			wantAction: "forwarded", wantPresence: slackengine.PresenceSlack, wantPosts: []sentPost{{"hub:SLACK", "done"}}},
		{name: "StopWhileTheOrchsButtonIsOffSendsNothingEvenWithAnotherOrchsButtonOn", stdin: stopInput("done"),
			presence: slackengine.PresenceSlack, buttons: []string{"hub:b:orch"},
			wantAction: "button-off", wantPresence: slackengine.PresenceSlack},
		{name: "StopOfABeamOrchWhoseOwnButtonIsOnForwardsToTheSlackProgram", stdin: stopInput("done"), strand: "hub:b:orch",
			presence: slackengine.PresenceSlack, buttons: []string{"hub:b:orch"},
			wantAction: "forwarded", wantPresence: slackengine.PresenceSlack, wantPosts: []sentPost{{"hub:SLACK", "done"}}},
		{name: "StopWithNoReplyDoesNothing", stdin: stopInput("  "), presence: slackengine.PresenceSlack,
			wantAction: "none", wantPresence: slackengine.PresenceSlack},
		{name: "StopWhilePresenceIsTerminalDoesNothing", stdin: stopInput("done"), presence: slackengine.PresenceTerminal,
			wantAction: "none", wantPresence: slackengine.PresenceTerminal},
		{name: "StopOutsideAnOrchIsRefused", stdin: stopInput("done"), strand: "hub:impl", presence: slackengine.PresenceSlack,
			wantRefusal: []string{"no orch's pane", "hub:impl"}, wantPresence: slackengine.PresenceSlack},
		{name: "StopWhosePostFailsIsRefusedNamingTheReceiver", stdin: stopInput("done"), presence: slackengine.PresenceSlack,
			postErr: errors.New("creel refused"), wantRefusal: []string{"hub:SLACK", "creel refused"},
			wantPresence: slackengine.PresenceSlack, wantPosts: []sentPost{{"hub:SLACK", "done"}}},
		{name: "SubagentPromptDoesNothingButNameItself",
			stdin:    hookJSON(map[string]any{"hook_event_name": "UserPromptSubmit", "prompt": "hello", "agent_id": "a1"}),
			presence: slackengine.PresenceSlack, wantAction: "subagent", wantPresence: slackengine.PresenceSlack},
		{name: "SubagentStopForwardsNothing",
			stdin:    hookJSON(map[string]any{"hook_event_name": "Stop", "last_assistant_message": "done", "agent_id": "a1"}),
			presence: slackengine.PresenceSlack, wantAction: "subagent", wantPresence: slackengine.PresenceSlack},
		{name: "OtherEventDoesNothing", stdin: hookJSON(map[string]any{"hook_event_name": "PostToolUse"}), presence: slackengine.PresenceSlack,
			wantAction: "none", wantPresence: slackengine.PresenceSlack},
		{name: "PayloadThatIsNoJSONObjectIsRefused", stdin: "not json", presence: slackengine.PresenceSlack,
			wantRefusal: []string{"parse hook input"}, wantPresence: slackengine.PresenceSlack},
		{name: "UnknownEngineIsRefusedNamingTheRegisteredOnes", engine: "nosuch", stdin: promptInput("hello"), presence: slackengine.PresenceSlack,
			wantRefusal: []string{`"nosuch"`, "claude"}, wantPresence: slackengine.PresenceSlack},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, nil)
			h.storePresence(tt.presence)
			buttons := tt.buttons
			if buttons == nil {
				buttons = []string{"hub:orch"}
			}
			h.storeForwarding(buttons...)
			if tt.strand != "" {
				h.strand = tt.strand
			}
			h.client.entries, h.client.logErr, h.client.postErr, h.clientErr = tt.entries, tt.logErr, tt.postErr, tt.clientErr
			engine := tt.engine
			if engine == "" {
				engine = "claude"
			}

			code, env := h.run(tt.stdin, "hook", engine)

			if tt.wantRefusal != nil {
				wantRefused(t, code, env, tt.wantRefusal...)
			} else {
				wantOK(t, code, env)
				if diff := cmp.Diff(map[string]any{"action": tt.wantAction}, env.Raw); diff != "" {
					t.Errorf("hook fields mismatch (-want +got):\n%s", diff)
				}
			}
			if got := h.presence(); got != tt.wantPresence {
				t.Errorf("presence = %q; want %q", got, tt.wantPresence)
			}
			var wantFilters []creelengine.LogFilter
			if tt.wantLogged {
				wantFilters = []creelengine.LogFilter{{To: h.strand, Since: fixedClock{}.Now().Add(-2 * time.Minute)}}
			}
			if diff := cmp.Diff(wantFilters, h.client.filters, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("creel log filters mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantPosts, h.client.posts, cmp.AllowUnexported(sentPost{}), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("posts mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFitReply(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	cut := replyLimit - len(replyCutNote)
	// The two-byte rune starts at cut-1, so the byte at cut is its continuation and the cut falls back to cut-1.
	multiByte := strings.Repeat("a", cut-1) + "é" + strings.Repeat("a", replyLimit)
	tests := []struct {
		name  string
		reply string
		want  string
	}{
		{"ShortReplyIsReturnedWhole", "hello", "hello"},
		{"ReplyAtTheLimitIsReturnedWhole", strings.Repeat("a", replyLimit), strings.Repeat("a", replyLimit)},
		{"LongASCIIReplyIsCutToTheLimitWithTheNote", strings.Repeat("a", replyLimit+1), strings.Repeat("a", cut) + replyCutNote},
		{"CutInsideARuneFallsBackToItsStart", multiByte, strings.Repeat("a", cut-1) + replyCutNote},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := fitReply(tt.reply)
			if got != tt.want {
				t.Errorf("fitReply(%d bytes) = %d bytes ending %q; want %d bytes", len(tt.reply), len(got), got[max(0, len(got)-20):], len(tt.want))
			}
			if !utf8.ValidString(got) || len(got) > replyLimit {
				t.Errorf("fitReply(%d bytes) = %d bytes, valid UTF-8 %v; want valid UTF-8 within %d bytes", len(tt.reply), len(got), utf8.ValidString(got), replyLimit)
			}
		})
	}
}
