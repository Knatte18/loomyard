// outbound_test.go covers the outbound read of the relay's mailbox over the in-memory seams of inbound_test.go.

package slackengine_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/slackengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// outboundHarness is an outbound read over the harness's seams.
type outboundHarness struct {
	*harness
	out *slackengine.Outbound
}

func newOutboundHarness(t *testing.T) *outboundHarness {
	t.Helper()
	h := newHarness(t)
	return &outboundHarness{harness: h, out: slackengine.NewOutbound(h.geom, h.mailbox, h.slack, h.clock)}
}

// mailFrom returns a message of id from sender, posted a minute before the harness's clock.
func (h *outboundHarness) mailFrom(id int64, from, body string) creelengine.Message {
	return creelengine.Message{ID: id, From: from, Body: body, Kind: "mail", Posted: h.clock.Now().Add(-time.Minute)}
}

func (h *outboundHarness) read() (time.Duration, error) {
	h.t.Helper()
	return h.out.Read(context.Background())
}

func (h *outboundHarness) mustRead() time.Duration {
	h.t.Helper()
	wait, err := h.read()
	if err != nil {
		h.t.Fatalf("Read = %v", err)
	}
	return wait
}

func (h *outboundHarness) wantAcks(want ...[]int64) {
	h.t.Helper()
	if got := h.mailbox.acknowledged(); !cmp.Equal(got, append([][]int64(nil), want...)) && !(len(got) == 0 && len(want) == 0) {
		h.t.Errorf("acknowledgements mismatch (-want +got):\n%s", cmp.Diff(want, got))
	}
}

func TestOutboundRead(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("MailFromOrchProgramAndReplyIsPostedOnceAndAcknowledged", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		reply := h.mailFrom(3, "ly:orch", "answer")
		reply.Kind, reply.ReplyTo = "reply", 9
		h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{
			h.mailFrom(1, "ly:orch", "hello"), h.mailFrom(2, "ly:BATTEN", "done"), reply,
		}})
		if wait := h.mustRead(); wait != slackengine.Floor {
			t.Errorf("Read wait = %s; want the floor %s", wait, slackengine.Floor)
		}
		h.wantAnswers("[ly:orch] hello", "[ly:BATTEN] done", "[ly:orch] answer")
		h.wantAcks([]int64{1, 2, 3})
	})

	t.Run("TextIsEscapedAndBeamMailCarriesTheAnswerHint", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{
			h.mailFrom(1, "ly:orch", "a & <b>"),
			h.mailFrom(2, "ly:payments:orch", "x"),
			h.mailFrom(3, "ly:payments:DOBBY", "y"),
		}})
		h.mustRead()
		h.wantAnswers(
			"[ly:orch] a &amp; &lt;b&gt;",
			"[ly:payments:orch] x (answer: @payments …)",
			"[ly:payments:DOBBY] y (answer: @payments …)",
		)
	})

	t.Run("NonOrchStrandAndExternalMailIsDroppedAndAcknowledged", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{
			h.mailFrom(1, "ly:payments:worker", "from a strand"),
			h.mailFrom(2, "ly:EXTERNAL", "from outside"),
		}})
		h.mustRead()
		h.wantAnswers()
		h.wantAcks([]int64{1, 2})
	})

	t.Run("SlackFailureMidReadPostsEachMessageExactlyOnce", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		batch := []creelengine.Message{h.mailFrom(1, "ly:orch", "one"), h.mailFrom(2, "ly:orch", "two"), h.mailFrom(3, "ly:orch", "three")}
		h.mailbox.scriptReadWait(readStep{messages: batch}, readStep{messages: batch})
		h.slack.failPosts(nil, errors.New("slack down"))
		if wait, err := h.read(); err == nil || wait != slackengine.Floor {
			t.Fatalf("Read = %s, %v; want the post error and the floor", wait, err)
		}
		h.wantAcks()
		h.mustRead()
		h.wantAnswers("[ly:orch] one", "[ly:orch] two", "[ly:orch] three")
		h.wantAcks([]int64{1, 2, 3})
	})

	t.Run("MessageSlackRefusesFiveTimesIsDroppedWhetherOrNotTheReplacementLinePosts", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			// replacementFailure is what the post of the replacement line itself answers; nil posts it.
			replacementFailure error
			wantLine           bool
		}{
			{"ReplacementLinePosts", nil, true},
			{"ReplacementLinePostFails", errors.New("slack down"), false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOutboundHarness(t)
				refusal := &slackengine.APIError{Method: "chat.postMessage", Code: "msg_too_long"}
				h.slack.failPosts(refusal, refusal, refusal, refusal, refusal, tt.replacementFailure)
				for range 5 {
					h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{h.mailFrom(7, "ly:orch", "huge")}})
				}
				for i := range 4 {
					if _, err := h.read(); !errors.Is(err, refusal) {
						t.Fatalf("Read #%d = %v; want the refusal", i+1, err)
					}
				}
				h.wantAcks()
				h.mustRead()
				answers := h.slack.texts()
				switch {
				case tt.wantLine && (len(answers) != 1 || !strings.Contains(answers[0], "message 7") || !strings.Contains(answers[0], "ly:orch") || !strings.Contains(answers[0], "lym creel log --to ly:SLACK")):
					t.Errorf("answers = %q; want one line naming message 7, its sender and the log command", answers)
				case !tt.wantLine && len(answers) != 0:
					t.Errorf("answers = %q; want none, the replacement line failed", answers)
				}
				h.wantAcks([]int64{7})
				if calls := h.slack.callCount(); calls != 6 {
					t.Errorf("Slack calls = %d; want 6, five refusals and one try of the replacement line, so the message is not retried", calls)
				}
				record, err := os.ReadFile(h.geom.OutboxPath)
				if err != nil || strings.Contains(string(record), "refusals") {
					t.Errorf("outbox record = %q, %v; want no refusal count after the acknowledgement", record, err)
				}
			})
		}
	})

	t.Run("TransportFailureAndServerErrorNeverCapOutAMessage", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			err  error
		}{
			{"TransportFailure", errors.New("slack: call chat.postMessage: connection reset")},
			{"ServerError", errors.New("slack: chat.postMessage answered HTTP 500")},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOutboundHarness(t)
				for range 7 {
					h.slack.failPosts(tt.err)
					h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{h.mailFrom(1, "ly:orch", "keep me")}})
					if _, err := h.read(); !errors.Is(err, tt.err) {
						t.Fatalf("Read = %v; want the failure", err)
					}
				}
				h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{h.mailFrom(1, "ly:orch", "keep me")}})
				h.mustRead()
				h.wantAnswers("[ly:orch] keep me")
				h.wantAcks([]int64{1})
			})
		}
	})

	t.Run("BacklogPastThePostingBudgetIsReleasedAndPostedOnceAcrossReads", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		h.slack.onPost = func() { h.clock.Advance(15 * time.Second) }
		batch := []creelengine.Message{h.mailFrom(1, "ly:orch", "one"), h.mailFrom(2, "ly:orch", "two"), h.mailFrom(3, "ly:orch", "three")}
		h.mailbox.scriptReadWait(readStep{messages: batch}, readStep{messages: batch})
		h.mustRead()
		h.wantAnswers("[ly:orch] one", "[ly:orch] two")
		h.wantAcks()
		h.mustRead()
		h.wantAnswers("[ly:orch] one", "[ly:orch] two", "[ly:orch] three")
		h.wantAcks([]int64{1, 2, 3})
	})

	t.Run("MailOlderThanADayIsAcknowledgedUnpostedWithOneCountLine", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		old1, old2 := h.mailFrom(1, "ly:orch", "old"), h.mailFrom(2, "ly:orch", "older")
		old1.Posted, old2.Posted = h.clock.Now().Add(-25*time.Hour), h.clock.Now().Add(-48*time.Hour)
		h.mailbox.scriptReadWait(readStep{messages: []creelengine.Message{old1, old2, h.mailFrom(3, "ly:orch", "fresh")}})
		h.mustRead()
		answers := h.slack.texts()
		if len(answers) != 2 || answers[0] != "[ly:orch] fresh" || !strings.Contains(answers[1], "2 messages") || !strings.Contains(answers[1], "lym creel log --to ly:SLACK") {
			t.Errorf("answers = %q; want the fresh mail and one line naming the count and the log command", answers)
		}
		h.wantAcks([]int64{1, 2, 3})
	})

	t.Run("StaleMailCountLineSurvivesAFailedReadAndPostsOnce", func(t *testing.T) {
		t.Parallel()
		slackDown := errors.New("slack down")
		tests := []struct {
			name string
			// failures are the answers of the first posts: the fresh message's, then the count line's.
			failures []error
			// wantLineIndex is where the count line stands among the texts Slack accepted.
			wantLineIndex int
		}{
			{"LinePostsBeforeTheFailedReadReleases", []error{slackDown}, 0},
			{"LineThatFailsToPostWaitsForTheNextRead", []error{slackDown, slackDown}, 1},
			{"LineWaitsOutARateLimitInsteadOfPostingIntoIt", []error{&slackengine.RateLimitError{Method: "chat.postMessage", RetryAfter: time.Second}}, 1},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOutboundHarness(t)
				old1, old2 := h.mailFrom(1, "ly:orch", "old"), h.mailFrom(2, "ly:orch", "older")
				old1.Posted, old2.Posted = h.clock.Now().Add(-25*time.Hour), h.clock.Now().Add(-48*time.Hour)
				batch := []creelengine.Message{old1, old2, h.mailFrom(3, "ly:orch", "fresh")}
				h.mailbox.scriptReadWait(readStep{messages: batch}, readStep{messages: batch})
				h.slack.failPosts(tt.failures...)
				if _, err := h.read(); !errors.Is(err, tt.failures[0]) {
					t.Fatalf("Read = %v; want the failed post", err)
				}
				h.wantAcks()
				h.mustRead()
				answers := h.slack.texts()
				if len(answers) != 2 || answers[1-tt.wantLineIndex] != "[ly:orch] fresh" ||
					!strings.Contains(answers[tt.wantLineIndex], "2 messages") || !strings.Contains(answers[tt.wantLineIndex], "lym creel log --to ly:SLACK") {
					t.Errorf("answers = %q; want the fresh mail once and one count line naming 2 messages at index %d", answers, tt.wantLineIndex)
				}
				h.wantAcks([]int64{1, 2, 3})
			})
		}
	})

	t.Run("OutboxWriteFailingAfterAnAcceptedPostAcknowledgesTheMailAndNeverRepostsIt", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name string
			// ackRefused makes creel refuse the first read's acknowledgement, so the batch is delivered again.
			ackRefused bool
		}{
			{"AcknowledgementConfirmed", false},
			{"AcknowledgementRefusedAndTheBatchRedelivered", true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOutboundHarness(t)
				// A directory in the outbox file's place makes every write of the record fail once Slack accepted the post.
				h.slack.onPost = func() {
					if err := os.MkdirAll(h.geom.OutboxPath, 0o700); err != nil {
						t.Errorf("MkdirAll = %v", err)
					}
				}
				batch := []creelengine.Message{h.mailFrom(1, "ly:orch", "once")}
				steps := []readStep{{messages: batch}}
				if tt.ackRefused {
					steps = []readStep{{messages: batch, ackErr: errors.New("daemon refused")}, {messages: batch}}
				}
				h.mailbox.scriptReadWait(steps...)
				for i := range steps {
					if _, err := h.read(); err == nil || !strings.Contains(err.Error(), "outbox file") {
						t.Errorf("Read #%d = %v; want the failed write of the outbox file", i+1, err)
					}
				}
				h.wantAnswers("[ly:orch] once")
				h.wantAcks([]int64{1})
			})
		}
	})

	t.Run("StaleMailLineIsPostedOnceWhenTheOutboxWriteFails", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		// The directory appears at the first post, before any write, so every write of the record fails.
		h.slack.onPost = func() {
			if err := os.MkdirAll(h.geom.OutboxPath, 0o700); err != nil {
				t.Errorf("MkdirAll = %v", err)
			}
		}
		old1, old2 := h.mailFrom(2, "ly:orch", "old"), h.mailFrom(3, "ly:orch", "older")
		old1.Posted, old2.Posted = h.clock.Now().Add(-25*time.Hour), h.clock.Now().Add(-48*time.Hour)
		batch := []creelengine.Message{h.mailFrom(1, "ly:orch", "fresh"), old1, old2}
		h.mailbox.scriptReadWait(readStep{messages: batch, ackErr: errors.New("daemon refused")}, readStep{messages: batch})
		for i := range 2 {
			if _, err := h.read(); err == nil || !strings.Contains(err.Error(), "outbox file") {
				t.Errorf("Read #%d = %v; want the failed write of the outbox file", i+1, err)
			}
		}
		lines := 0
		for _, text := range h.slack.texts() {
			if strings.Contains(text, "2 messages") && strings.Contains(text, "lym creel log --to ly:SLACK") {
				lines++
			}
		}
		if lines != 1 {
			t.Errorf("count lines = %d in %q; want 1", lines, h.slack.texts())
		}
		h.wantAcks([]int64{1, 2, 3})
	})

	t.Run("OutboxWriteFailureRidesAlongWithAFailedPostAndWithTheEndedBudget", func(t *testing.T) {
		t.Parallel()
		slackDown := errors.New("slack down")
		tests := []struct {
			name string
			// advance is how far each post moves the clock.
			advance time.Duration
			// failures are the answers of the first posts.
			failures []error
			// wantPostFailure is whether the read's error must also carry the failed post.
			wantPostFailure bool
		}{
			{"FailedPost", 0, []error{nil, slackDown}, true},
			{"PostingBudgetEnded", 15 * time.Second, nil, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				h := newOutboundHarness(t)
				h.slack.onPost = func() {
					h.clock.Advance(tt.advance)
					if err := os.MkdirAll(h.geom.OutboxPath, 0o700); err != nil {
						t.Errorf("MkdirAll = %v", err)
					}
				}
				h.slack.failPosts(tt.failures...)
				batch := []creelengine.Message{h.mailFrom(1, "ly:orch", "one"), h.mailFrom(2, "ly:orch", "two"), h.mailFrom(3, "ly:orch", "three")}
				h.mailbox.scriptReadWait(readStep{messages: batch})
				_, err := h.read()
				if err == nil || !strings.Contains(err.Error(), "outbox file") {
					t.Fatalf("Read = %v; want the failed write of the outbox file", err)
				}
				if tt.wantPostFailure && !errors.Is(err, slackDown) {
					t.Errorf("Read = %v; want it to carry the failed post too", err)
				}
			})
		}
	})

	t.Run("OutboxFileWritableAgainStandsInPlaceOfTheRecordHeldInMemory", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		h.slack.onPost = func() {
			if err := os.MkdirAll(h.geom.OutboxPath, 0o700); err != nil {
				t.Errorf("MkdirAll = %v", err)
			}
		}
		rest := []creelengine.Message{h.mailFrom(2, "ly:orch", "two"), h.mailFrom(3, "ly:orch", "three"), h.mailFrom(4, "ly:orch", "four")}
		h.mailbox.scriptReadWait(
			readStep{messages: []creelengine.Message{h.mailFrom(1, "ly:orch", "one")}},
			readStep{messages: rest},
			readStep{messages: rest},
		)
		if _, err := h.read(); err == nil {
			t.Fatalf("Read = nil; want the failed write of the outbox file")
		}
		if err := os.Remove(h.geom.OutboxPath); err != nil {
			t.Fatalf("Remove = %v", err)
		}
		// The second read ends on the posting budget, so it leaves the record in the file instead of clearing it.
		h.slack.onPost = func() { h.clock.Advance(15 * time.Second) }
		if _, err := h.read(); err != nil {
			t.Fatalf("Read = %v; want nil once the file is writable again", err)
		}
		record, err := os.ReadFile(h.geom.OutboxPath)
		if err != nil || !strings.Contains(string(record), "handled") {
			t.Errorf("outbox record = %q, %v; want the file to hold the handled messages", record, err)
		}
		// With the file standing in for memory again, the third read skips the messages the second one handled.
		h.mustRead()
		h.wantAnswers("[ly:orch] one", "[ly:orch] two", "[ly:orch] three", "[ly:orch] four")
	})

	t.Run("RefusedAcknowledgementKeepsTheRecordSoNothingIsRepostedTwice", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		batch := []creelengine.Message{h.mailFrom(1, "ly:orch", "once")}
		h.mailbox.scriptReadWait(readStep{messages: batch, ackErr: errors.New("daemon refused")}, readStep{messages: batch})
		if _, err := h.read(); err == nil {
			t.Fatalf("Read = nil; want the refused acknowledgement")
		}
		h.mustRead()
		h.wantAnswers("[ly:orch] once")
		h.wantAcks([]int64{1})
	})

	t.Run("RateLimitReturnsItsRetryAfterAndRaisesTheFloor", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		batch := []creelengine.Message{h.mailFrom(1, "ly:orch", "limited")}
		h.mailbox.scriptReadWait(readStep{messages: batch}, readStep{messages: batch})
		h.slack.failPosts(&slackengine.RateLimitError{Method: "chat.postMessage", RetryAfter: 7 * time.Second})
		var limit *slackengine.RateLimitError
		if wait, err := h.read(); !errors.As(err, &limit) || wait != 7*time.Second {
			t.Fatalf("Read = %s, %v; want the 7-second retry wait", wait, err)
		}
		if wait := h.mustRead(); wait != 2*slackengine.Floor {
			t.Errorf("Read wait after the limit = %s; want the doubled floor %s", wait, 2*slackengine.Floor)
		}
	})

	t.Run("EmptyWaitBacksOffAndKeepsTheRecord", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		batch := []creelengine.Message{h.mailFrom(1, "ly:orch", "once")}
		h.mailbox.scriptReadWait(readStep{messages: batch, ackErr: errors.New("daemon refused")})
		if _, err := h.read(); err == nil {
			t.Fatalf("Read = nil; want the refused acknowledgement")
		}
		var waits []time.Duration
		for range 6 {
			waits = append(waits, h.mustRead())
		}
		if want := []time.Duration{2 * slackengine.Floor, 4 * slackengine.Floor, 8 * slackengine.Floor, slackengine.Ceiling, slackengine.Ceiling, slackengine.Ceiling}; !cmp.Equal(waits, want) {
			t.Errorf("idle waits = %v; want %v", waits, want)
		}
		h.mailbox.scriptReadWait(readStep{messages: batch})
		h.mustRead()
		h.wantAnswers("[ly:orch] once")
	})

	t.Run("TimeoutAfterTheHandlerRanKeepsTheRecordAndReturnsTheFloor", func(t *testing.T) {
		t.Parallel()
		h := newOutboundHarness(t)
		batch := []creelengine.Message{h.mailFrom(1, "ly:orch", "once")}
		h.mailbox.scriptReadWait(
			readStep{messages: batch, timeoutAfterHandler: true},
			readStep{messages: batch, timeoutAfterHandler: true},
		)
		for i := range 2 {
			if wait, err := h.read(); wait != slackengine.Floor || !errors.Is(err, creelengine.ErrWaitTimeout) {
				t.Fatalf("Read #%d = %s, %v; want the floor with the lost confirmation", i+1, wait, err)
			}
		}
		h.wantAnswers("[ly:orch] once")
	})
}
