// outbound_test.go pins the sending half's delivery of mail across hubs.
// It covers the outbound worker's retries, its start cap, and the hour's return.
// It also covers the quoting of the other hub's text and the notice a return sends.

package creelengine_test

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/testkit/creelfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	// sendingChaser is the program the sending hub signs its return notices with.
	sendingChaser = "ly:CREEL"
	// callTimeout is the timeout of one deliver call on the daemon's clock.
	callTimeout = 30 * time.Second
)

// eventually polls cond until it holds;
// the real-time deadline only bounds a failing test, since no signal marks the journal write it waits for.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// notCallTimeout keeps the waits of a retrying worker and leaves out the timer of a deliver call in flight.
func notCallTimeout(wait time.Duration) bool { return wait != callTimeout }

func anyWait(time.Duration) bool { return true }

// driveClock advances the rig's daemon clock by every wait that keep accepts and that the daemon arms.
// That runs a retrying worker through its hour; the returned function ends it.
func driveClock(t *testing.T, r *rig, keep func(time.Duration) bool) (stop func()) {
	t.Helper()
	done, finished := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		r.clock.AdvanceByWaitsWhere(keep, done)
	}()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			close(done)
			<-finished
		})
	}
	t.Cleanup(stop)
	return stop
}

// hold lets a hubFake's answer wait until the row releases it.
type hold struct {
	ch   chan struct{}
	once sync.Once
}

func newHold() *hold { return &hold{ch: make(chan struct{})} }

func (h *hold) release() { h.once.Do(func() { close(h.ch) }) }

// events returns the log entries of the rig's daemon that carry event, oldest first.
func (p *hubPair) events(r *rig, event string) []creelengine.LogEntry {
	p.t.Helper()
	var matching []creelengine.LogEntry
	for _, entry := range p.logOf(r, "") {
		if entry.Event == event {
			matching = append(matching, entry)
		}
	}
	return matching
}

// awaitEvents waits until the rig's log holds n entries of event and returns them.
func (p *hubPair) awaitEvents(r *rig, event string, n int) []creelengine.LogEntry {
	p.t.Helper()
	var matching []creelengine.LogEntry
	eventually(p.t, fmt.Sprintf("%d %s entries in the log", n, event), func() bool {
		matching = p.events(r, event)
		return len(matching) >= n
	})
	return matching
}

// pendingLine is the journal line of an outbound message from alpha to rcvAlpha, posted at the given RFC 3339 time.
func (p *hubPair) pendingLine(id int, at string) string {
	return fmt.Sprintf(`{"event":"outbound","at":%q,"id":%d,"from":%q,"to":%q,"receiving_hub":%q,"kind":"mail","body":"b"}`, at, id, alpha, rcvAlpha, p.receiving.geom.HubPath)
}

// noticeLines waits until n return notices were typed into alpha's pane and returns the typed lines.
func (p *hubPair) noticeLines(n int) []string {
	p.t.Helper()
	var lines []string
	eventually(p.t, "the return notice typed into alpha", func() bool {
		lines = nil
		for _, send := range p.sending.del.Sends() {
			if send.Strand == alpha && strings.Contains(send.Text, `from "`+sendingChaser+`"`) {
				lines = append(lines, send.Text)
			}
		}
		return len(lines) >= n
	})
	return lines
}

// hubAnswer is the script of a hubFake: the response, or a raw line written in its place.
type hubAnswer = func(creelengine.Request) (creelengine.Response, string)

func accepted(id int64) hubAnswer {
	return func(creelengine.Request) (creelengine.Response, string) {
		return creelengine.Response{OK: true, Delivery: "accepted", ID: id}, ""
	}
}

// exitsAtOnce is a spawn whose daemon exits as soon as it started, which the client reads as a start that failed.
func exitsAtOnce(string) (creelengine.Spawn, error) {
	exited := make(chan creelengine.SpawnExit, 1)
	exited <- creelengine.SpawnExit{}
	return creelengine.Spawn{Exited: exited}, nil
}

func TestScheduler_Outbound(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	hourOfRetries := []struct {
		name string
		// parked leaves the receiving hub's geometry unreadable.
		parked bool
		answer hubAnswer
		// hangs makes the receiving daemon hold every call unanswered; driveAll also advances the clock by the call timeout.
		hangs      bool
		driveAll   bool
		wantReason string
		// absent is text of the other hub that must not reach the notice.
		absent string
	}{
		{"AnAnswerOfNeitherWordIsRetriedAndAtTheHourGivesBadAnswer", false,
			func(creelengine.Request) (creelengine.Response, string) { return creelengine.Response{OK: true}, "" },
			false, false, "bad-answer", ""},
		{"AnUnknownVerbRefusalIsRetriedAndAtTheHourIsQuoted", false,
			func(creelengine.Request) (creelengine.Response, string) {
				return creelengine.Response{Error: `creel: unknown verb "deliver"`}, ""
			},
			false, false, `"creel: unknown verb \"deliver\""`, ""},
		{"ADaemonThatNeverAnswersGivesTheTimeoutCodeAfterThirtySecondsOfTheFakeClock", false,
			nil, true, true, "timeout", ""},
		{"AnUnreadableGeometryIsRetriedAndGivesHubUnreadable", true, nil, false, false, "hub-unreadable", ""},
		{"AFailingSpawnIsRetriedAndGivesConnectFailed", false, nil, false, false, "connect-failed", ""},
		{"AnAnswerForAForeignHubPathIsRetriedAndGivesBadAnswerWithNoneOfItsText", false,
			func(creelengine.Request) (creelengine.Response, string) {
				return creelengine.Response{OK: true, Delivery: "accepted", HubPath: "/elsewhere \"quoted\"\nFOREIGN"}, ""
			},
			false, false, "bad-answer", "FOREIGN"},
		{"ALineThatIsNotJSONIsRetriedAndGivesBadAnswerWithNoneOfIt", false,
			func(creelengine.Request) (creelengine.Response, string) {
				return creelengine.Response{}, "NONJSON not json"
			},
			false, false, "bad-answer", "NONJSON"},
		{"ACodedRefusalIsRetriedAndAtTheHourItsTextIsQuotedWithTheLineBreakMadeASpace", false,
			func(creelengine.Request) (creelengine.Response, string) {
				return creelengine.Response{Code: creelengine.CodeStrandClaimOutsidePane, Error: "first \"quoted\"\nsecond"}, ""
			},
			false, false, `"first \"quoted\" second"`, ""},
	}
	tests := []struct {
		name string
		run  func(t *testing.T, p *hubPair)
	}{
		{"APostReachesTheReceiversPoolAndTheLogShowsItsRemoteDelivery", func(t *testing.T, p *hubPair) {
			p.unpark()
			p.startSending()
			p.startReceiving()
			mail := p.pending("hello")
			resp := mustCall(t, p.client(p.receiving), creelengine.Request{Verb: creelengine.VerbRead, Wait: true})
			if len(resp.Messages) != 1 || resp.Messages[0].Body != "hello" || resp.Messages[0].From != alpha || resp.Messages[0].SendingHub != p.sending.geom.HubPath {
				t.Fatalf("the receiver read %+v; want the message from alpha stamped with the sending hub", resp.Messages)
			}
			delivered := p.awaitEvents(p.sending, "remote-delivered", 1)
			if delivered[0].ID != mail.SendingID || delivered[0].RemoteID != resp.Messages[0].ID {
				t.Errorf("remote-delivered entry = %+v; want id %d and remote id %d", delivered[0], mail.SendingID, resp.Messages[0].ID)
			}
		}},
		{"APostWhileTheReceivingDaemonIsDownStartsItThroughTheSpawnAndIsDelivered", func(t *testing.T, p *hubPair) {
			p.unpark()
			p.startSending()
			p.sending.hubs.SetSpawn(func(string) (creelengine.Spawn, error) { return creelengine.Spawn{}, p.receiving.spawn() })
			p.pending("hello")
			p.awaitEvents(p.sending, "remote-delivered", 1)
			if spawns := p.sending.hubs.Spawns(p.receiving.geom.HubPath); spawns != 1 {
				t.Errorf("the receiving daemon was started %d times; want once", spawns)
			}
			resp := mustCall(t, p.client(p.receiving), creelengine.Request{Verb: creelengine.VerbRead})
			if len(resp.Messages) != 1 || resp.Messages[0].Body != "hello" {
				t.Errorf("the receiver read %+v; want the delivered message", resp.Messages)
			}
		}},
		{"ARefusalReturnsAtOnceSendsOneNoticeAndEndsAWaitReplyAsReturned", func(t *testing.T, p *hubPair) {
			p.unpark()
			p.startSending()
			p.startReceiving()
			got := result(t, callAsync(p.client(p.sending), creelengine.Request{Verb: creelengine.VerbPost, Receiver: "lyt:nobody:driver", Text: "anyone", Wait: true}, 0))
			if got.err != nil || got.resp.Outcome != creelengine.OutcomeReturned || got.resp.Message == nil || got.resp.Message.Status != creelengine.StatusUndelivered {
				t.Fatalf("the waiting post ended with %+v, %v; want returned with the undelivered message", got.resp, got.err)
			}
			returned := p.events(p.sending, "outbound-returned")
			if len(returned) != 1 || !strings.Contains(returned[0].Reason, "unknown receiver") {
				t.Fatalf("outbound-returned entries = %+v; want one with the receiver's refusal as its reason", returned)
			}
			lines := p.noticeLines(1)
			if len(lines) != 1 || !strings.Contains(lines[0], "lyt:nobody:driver") || !strings.Contains(lines[0], "lym creel log --id") || !strings.Contains(lines[0], "unknown receiver") {
				t.Errorf("notice lines = %q; want one naming the receiver, the reason and the log command", lines)
			}
			posted := 0
			for _, entry := range p.logOf(p.sending, sendingChaser) {
				if entry.Event == "posted" {
					posted++
				}
			}
			if posted != 1 {
				t.Errorf("%d notices posted; want one", posted)
			}
		}},
		{"ARefusalToASenderNoLongerListedSendsNoNotice", func(t *testing.T, p *hubPair) {
			p.startSending()
			p.startReceiving()
			mustCall(t, p.client(p.sending), creelengine.Request{Verb: creelengine.VerbPost, Receiver: "lyt:nobody:driver", Text: "anyone"})
			p.sending.del.SetStrands(creelfake.LiveStrand(beta), creelfake.LiveStrand(orch))
			p.sending.clock.AwaitWaitsWhere(1, anyWait)
			p.unpark()
			p.sending.clock.Advance(time.Second)
			p.awaitEvents(p.sending, "outbound-returned", 1)
			listedAlpha := creelfake.LiveStrand(alpha)
			listedAlpha.PanePID = os.Getpid()
			p.sending.del.SetStrands(listedAlpha, creelfake.LiveStrand(beta), creelfake.LiveStrand(orch))
			if got := p.logOf(p.sending, sendingChaser); len(got) != 0 {
				t.Errorf("the notices posted = %+v; want none for a sender that is no longer listed", got)
			}
		}},
		{"AReceiverDownForTheHourReturnsAtTheHourWithTheLastTransientReason", func(t *testing.T, p *hubPair) {
			p.unpark()
			p.startSending()
			driveClock(t, p.sending, notCallTimeout)
			p.pending("nobody home")
			returned := p.awaitEvents(p.sending, "outbound-returned", 1)[0]
			if returned.At.Before(epoch.Add(time.Hour)) || returned.Reason != "connect-failed" {
				t.Errorf("outbound-returned entry = %+v; want it at or after the hour with the reason connect-failed", returned)
			}
			if lines := p.noticeLines(1); !strings.Contains(lines[0], "connect-failed") {
				t.Errorf("notice lines = %q; want the reason in the notice", lines)
			}
		}},
		{"APendingLineWrittenFortyMinutesBeforeTheStartReturnsTwentyMinutesAfterIt", func(t *testing.T, p *hubPair) {
			p.sending.writeJournal("2026-10-01", p.pendingLine(5, "2026-10-01T11:20:00Z"))
			p.startSending()
			waits := p.sending.clock.AwaitWaitsWhere(1, anyWait)
			if waits[0] != time.Second {
				t.Fatalf("first retry wait = %v; want 1s", waits[0])
			}
			p.sending.clock.Advance(19 * time.Minute)
			waits = p.sending.clock.AwaitWaitsWhere(2, anyWait)
			if waits[1] != 2*time.Second || p.count(p.sending, "outbound-returned") != 0 {
				t.Fatalf("after 19 minutes the retry waits are %v and %d returns stand; want 1s, 2s and none", waits, p.count(p.sending, "outbound-returned"))
			}
			p.sending.clock.Advance(time.Minute)
			returned := p.awaitEvents(p.sending, "outbound-returned", 1)[0]
			if returned.ID != 5 || !returned.At.Equal(epoch.Add(20*time.Minute)) {
				t.Errorf("outbound-returned entry = %+v; want message 5 at 20 minutes after the start", returned)
			}
		}},
		{"TheRetryWaitsDoubleToAMinuteAreCappedAtTheHourAndANewPostMovesNeitherTheWaitNorTheCalls", func(t *testing.T, p *hubPair) {
			fake := listenHubFake(t, p.receiving, func(creelengine.Request) (creelengine.Response, string) {
				return creelengine.Response{Error: "not yet"}, ""
			})
			p.unpark()
			p.startSending()
			p.pending("first")
			fake.awaitArrivals(t, 1)
			retryWaits := func(n int) []time.Duration { return p.sending.clock.AwaitWaitsWhere(n, notCallTimeout) }
			retryWaits(1)
			p.pending("second")
			if got, waits := fake.requests("deliver"), retryWaits(0); got != 1 || len(waits) != 1 {
				t.Fatalf("a post during the wait left %d calls and waits %v; want one call and the one wait", got, waits)
			}
			want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, time.Minute, time.Minute}
			for i := 1; i < len(want); i++ {
				p.sending.clock.Advance(want[i-1])
				fake.awaitArrivals(t, 1)
				retryWaits(i + 1)
			}
			p.sending.clock.Advance(57 * time.Minute)
			fake.awaitArrivals(t, 1)
			waits := retryWaits(len(want) + 1)
			if want = append(want, 57*time.Second); !equalWaits(waits, want) {
				t.Errorf("retry waits = %v; want %v", waits, want)
			}
			if got := fake.requests("deliver"); got != len(want) {
				t.Errorf("%d calls made; want %d, one per wait and the first", got, len(want))
			}
		}},
		{"TwoPostsToAHeldReceiverArriveOldestFirst", func(t *testing.T, p *hubPair) {
			released := newHold()
			var (
				mu     sync.Mutex
				bodies []string
				nextID atomic.Int64
			)
			fake := listenHubFake(t, p.receiving, func(req creelengine.Request) (creelengine.Response, string) {
				mu.Lock()
				bodies = append(bodies, req.Mail.Body)
				first := len(bodies) == 1
				mu.Unlock()
				if first {
					<-released.ch
				}
				return creelengine.Response{OK: true, Delivery: "accepted", ID: nextID.Add(1)}, ""
			})
			p.unpark()
			p.startSending()
			t.Cleanup(released.release)
			p.pending("one")
			fake.awaitArrivals(t, 1)
			p.pending("two")
			released.release()
			fake.awaitArrivals(t, 1)
			delivered := p.awaitEvents(p.sending, "remote-delivered", 2)
			mu.Lock()
			defer mu.Unlock()
			if strings.Join(bodies, ",") != "one,two" || delivered[0].RemoteID != 1 || delivered[1].RemoteID != 2 {
				t.Errorf("bodies %q delivered %+v; want one then two, journaled with the remote ids 1 and 2", bodies, delivered)
			}
		}},
		{"ADeliverHeldAcrossTheHourEndsBeforeAReturnIsJournaled", func(t *testing.T, p *hubPair) {
			released := newHold()
			fake := listenHubFake(t, p.receiving, func(creelengine.Request) (creelengine.Response, string) {
				<-released.ch
				return creelengine.Response{OK: true, Delivery: "accepted", ID: 9}, ""
			})
			p.sending.writeJournal("2026-10-01", p.pendingLine(5, "2026-10-01T11:00:10Z"))
			p.unpark()
			p.startSending()
			t.Cleanup(released.release)
			fake.awaitArrivals(t, 1)
			p.sending.clock.Advance(11 * time.Second)
			if got := p.count(p.sending, "outbound-returned"); got != 0 {
				t.Fatalf("%d returns stand while the call is in flight past the hour; want none", got)
			}
			released.release()
			delivered := p.awaitEvents(p.sending, "remote-delivered", 1)
			if delivered[0].RemoteID != 9 || p.count(p.sending, "outbound-returned") != 0 {
				t.Errorf("delivered %+v with %d returns; want the accepted answer journaled and no return", delivered, p.count(p.sending, "outbound-returned"))
			}
		}},
		{"TheFinalCallAtTheHourAnsweringRefusedReturnsWithThatReason", func(t *testing.T, p *hubPair) {
			var refuse atomic.Bool
			fake := listenHubFake(t, p.receiving, func(creelengine.Request) (creelengine.Response, string) {
				if refuse.Load() {
					return creelengine.Response{OK: true, Delivery: "refused", Reason: "the receiver is gone"}, ""
				}
				return creelengine.Response{Error: "later"}, ""
			})
			p.unpark()
			p.startSending()
			p.pending("x")
			fake.awaitArrivals(t, 1)
			p.sending.clock.AwaitWaitsWhere(1, notCallTimeout)
			refuse.Store(true)
			p.sending.clock.Advance(time.Hour)
			returned := p.awaitEvents(p.sending, "outbound-returned", 1)[0]
			if returned.Reason != `"the receiver is gone"` || returned.At.Before(epoch.Add(time.Hour)) {
				t.Errorf("outbound-returned entry = %+v; want the quoted refusal at the hour", returned)
			}
		}},
		{"APendingLinePastTheHourBesideItsInboundRecordIsJournaledDeliveredWithTheHeldId", func(t *testing.T, p *hubPair) {
			p.sending.writeJournal("2026-10-01", p.pendingLine(5, "2026-10-01T10:00:00Z"))
			p.receiving.writeJournal("2026-10-01", fmt.Sprintf(
				`{"event":"inbound","at":"2026-10-01T10:30:00Z","id":3,"from":%q,"to":%q,"kind":"mail","body":"b","sending_hub":%q,"sending_id":5,"remote_posted":"2026-10-01T10:00:00Z","delivered_to":%q}`,
				alpha, rcvAlpha, p.sending.geom.HubPath, rcvAlpha))
			p.unpark()
			p.startReceiving()
			p.startSending()
			delivered := p.awaitEvents(p.sending, "remote-delivered", 1)[0]
			if delivered.ID != 5 || delivered.RemoteID != 3 || p.count(p.sending, "outbound-returned") != 0 {
				t.Errorf("delivered %+v with %d returns; want message 5 delivered with the held id 3 and no return", delivered, p.count(p.sending, "outbound-returned"))
			}
		}},
		{"APendingMessageSurvivesASendingDaemonRestartAndIsDeliveredAfterIt", func(t *testing.T, p *hubPair) {
			p.startSending()
			p.pending("survives")
			if err := p.sendingRun.stop(); err != nil {
				t.Fatalf("stop the sending daemon: %v", err)
			}
			p.unpark()
			p.startReceiving()
			p.startSending()
			p.awaitEvents(p.sending, "remote-delivered", 1)
			resp := mustCall(t, p.client(p.receiving), creelengine.Request{Verb: creelengine.VerbRead})
			if len(resp.Messages) != 1 || resp.Messages[0].Body != "survives" {
				t.Errorf("the receiver read %+v; want the message posted before the restart", resp.Messages)
			}
		}},
		{"AReceivingDaemonThatExitsAtEachStartIsStartedThreeTimesAndTheMessageReturnsWithStartCapped", func(t *testing.T, p *hubPair) {
			p.unpark()
			p.startSending()
			p.sending.hubs.SetSpawn(exitsAtOnce)
			driveClock(t, p.sending, notCallTimeout)
			hubPath := p.receiving.geom.HubPath
			first := p.pending("first")
			eventually(t, "the first start", func() bool { return p.sending.hubs.Spawns(hubPath) >= 1 })
			p.pending("second")
			eventually(t, "the second start", func() bool { return p.sending.hubs.Spawns(hubPath) >= 2 })
			p.pending("third")
			returned := p.awaitEvents(p.sending, "outbound-returned", 1)[0]
			if returned.ID != first.SendingID || returned.Reason != "start-capped" || returned.At.Before(epoch.Add(time.Hour)) {
				t.Errorf("outbound-returned entry = %+v; want message %d at the hour with start-capped", returned, first.SendingID)
			}
			if spawns := p.sending.hubs.Spawns(hubPath); spawns != 3 {
				t.Errorf("the receiving daemon was started %d times across the hour; want 3", spawns)
			}
		}},
		{"ADaemonThatAnswersResetsTheStartCountAndASendingDaemonRestartEmptiesIt", func(t *testing.T, p *hubPair) {
			p.unpark()
			p.startSending()
			p.sending.hubs.SetSpawn(exitsAtOnce)
			hubPath := p.receiving.geom.HubPath
			// Each step lets the worker's next retry wait pass, so the row controls how much of the hour the clock spends.
			seen := 0
			nextWait := func() time.Duration {
				seen++
				waits := p.sending.clock.AwaitWaitsWhere(seen, notCallTimeout)
				if len(waits) < seen {
					t.Fatalf("no retry wait number %d was armed", seen)
				}
				return waits[seen-1]
			}
			step := func() { p.sending.clock.Advance(nextWait()) }
			p.pending("first")
			for p.sending.hubs.Spawns(hubPath) < 3 {
				step()
			}
			// The capped worker's next wait is armed before the receiver starts, so its attempt reaches a daemon that answers.
			capped := nextWait()
			p.startReceiving()
			p.sending.clock.Advance(capped)
			p.awaitEvents(p.sending, "remote-delivered", 1)
			if spawns := p.sending.hubs.Spawns(hubPath); spawns != 3 {
				t.Fatalf("the capped worker started the daemon %d times in all; want it to connect with no start", spawns)
			}
			if err := p.receivingRun.stop(); err != nil {
				t.Fatalf("stop the receiving daemon: %v", err)
			}
			p.pending("second")
			for p.sending.hubs.Spawns(hubPath) < 6 {
				step()
			}
			if err := p.sendingRun.stop(); err != nil {
				t.Fatalf("stop the sending daemon: %v", err)
			}
			p.startSending()
			eventually(t, "a start after the sending daemon restarted", func() bool { return p.sending.hubs.Spawns(hubPath) >= 7 })
		}},
		{"ARefusedReasonOverTheCapIsQuotedCutOnARuneBoundaryWithTheBreakAndEscapesApplied", func(t *testing.T, p *hubPair) {
			reason := strings.Repeat("a", 190) + "\"\\\n" + "bbbbb" + "世tail"
			fake := listenHubFake(t, p.receiving, func(creelengine.Request) (creelengine.Response, string) {
				return creelengine.Response{OK: true, Delivery: "refused", Reason: reason}, ""
			})
			p.unpark()
			p.startSending()
			p.pending("x")
			returned := p.awaitEvents(p.sending, "outbound-returned", 1)[0]
			want := `"` + strings.Repeat("a", 190) + `\"` + `\\` + ` ` + "bbbbb" + `"`
			if returned.Reason != want || fake.requests("deliver") != 1 {
				t.Errorf("outbound-returned reason = %q after %d calls; want %q after one", returned.Reason, fake.requests("deliver"), want)
			}
			if lines := p.noticeLines(1); !strings.Contains(lines[0], want) || strings.Contains(lines[0], "世") {
				t.Errorf("notice lines = %q; want the quoted cut reason", lines)
			}
		}},
		{"AnIdClashAnsweredByARealReceiverReachesTheNoticeWhole", func(t *testing.T, p *hubPair) {
			p.receiving.writeJournal("2026-10-01", fmt.Sprintf(
				`{"event":"inbound","at":"2026-10-01T10:30:00Z","id":3,"from":%q,"to":%q,"kind":"mail","body":"other","sending_hub":%q,"sending_id":1,"remote_posted":"2026-10-01T10:00:00Z","delivered_to":%q}`,
				alpha, rcvAlpha, p.sending.geom.HubPath, rcvAlpha))
			p.unpark()
			p.startReceiving()
			p.startSending()
			p.pending("mine")
			p.awaitEvents(p.sending, "outbound-returned", 1)
			lines := p.noticeLines(1)
			for _, part := range []string{"id clash", "ids 1-1", "seven-day retention"} {
				if !strings.Contains(lines[0], part) {
					t.Errorf("notice %q lacks %q", lines[0], part)
				}
			}
		}},
		{"ADeliveredMessageThatExpectsAReplyIsNotSettledByASweepOnTheSendingSide", func(t *testing.T, p *hubPair) {
			p.unpark()
			p.startSending()
			p.startReceiving()
			mustCall(t, p.client(p.sending), creelengine.Request{Verb: creelengine.VerbPost, Receiver: rcvAlpha, Text: "question", ExpectReply: true})
			p.awaitEvents(p.sending, "remote-delivered", 1)
			p.sending.clock.Advance(101 * time.Second)
			for _, event := range []string{"unanswered", "undelivered", "outbound-returned"} {
				if got := p.count(p.sending, event); got != 0 {
					t.Errorf("%d %s entries after two sweep ceilings; want none", got, event)
				}
			}
		}},
		{"AReplyFromTheHolderReachesTheOriginalSenderAsMailEndsItsWaitAndIsJournaledAsAnsweringTheOriginal", func(t *testing.T, p *hubPair) {
			p.unpark()
			p.startSending()
			p.startReceiving()
			waiting := callAsync(p.client(p.sending), creelengine.Request{Verb: creelengine.VerbPost, Receiver: rcvAlpha, Text: "question", Wait: true}, 0)
			asked := mustCall(t, p.client(p.receiving), creelengine.Request{Verb: creelengine.VerbRead, Wait: true}).Messages
			if len(asked) != 1 {
				t.Fatalf("the receiver read %+v; want the question", asked)
			}
			reply := mustCall(t, p.client(p.receiving), creelengine.Request{Verb: creelengine.VerbReply, ID: asked[0].ID, Text: "answer"})
			got := result(t, waiting)
			if got.err != nil || got.resp.Outcome != creelengine.OutcomeReply || got.resp.Message == nil || got.resp.Message.Body != "answer" ||
				got.resp.Message.From != rcvAlpha || got.resp.Message.SendingHub != p.receiving.geom.HubPath {
				t.Fatalf("the waiting post ended with %+v, %v; want the reply from %s stamped with the replying hub", got.resp, got.err, rcvAlpha)
			}
			outbound := p.events(p.receiving, "outbound")
			if len(outbound) != 1 || outbound[0].ID != reply.ID || outbound[0].To != alpha || outbound[0].Answers != asked[0].ID {
				t.Errorf("outbound entries of the replying hub = %+v; want one to %s answering #%d", outbound, alpha, asked[0].ID)
			}
			mail := mustCall(t, p.client(p.sending), creelengine.Request{Verb: creelengine.VerbRead}).Messages
			if len(mail) != 1 || mail[0].Body != "answer" {
				t.Errorf("the original sender read %+v; want the reply still delivered as mail", mail)
			}
		}},
		{"ADeliveredReplyNamingAnIdThatWasNeverSentToItsSenderOrIsNoLongerKeptIsRefusedWithNoDrop", func(t *testing.T, p *hubPair) {
			outbound := func(id int, from, hub string) string {
				return fmt.Sprintf(`{"event":"outbound","at":"2026-10-01T11:00:00Z","id":%d,"from":%q,"to":%q,"receiving_hub":%q,"kind":"mail","body":"q"}`, id, from, alpha, hub)
			}
			delivered := func(id int) string {
				return fmt.Sprintf(`{"event":"remote-delivered","at":"2026-10-01T11:00:01Z","id":%d,"remote_id":9}`, id)
			}
			p.receiving.writeJournal("2026-10-01",
				outbound(5, rcvAlpha, "/hubs/elsewhere"), delivered(5),
				outbound(6, rcvBeta, p.sending.geom.HubPath), delivered(6))
			p.startReceiving()
			for _, tt := range []struct {
				name    string
				replyTo int64
			}{{"SentToAnotherHub", 5}, {"SentBySomeoneElse", 6}, {"DroppedByRetention", 99}} {
				mail := creelengine.HubMail{SendingID: 20, Sender: alpha, Receiver: rcvAlpha, ReplyTo: tt.replyTo, Posted: epoch, Body: "reply"}
				wantRefused(t, p.deliver(mail), fmt.Sprintf("#%d", tt.replyTo))
			}
			if got := p.count(p.receiving, "dropped") + p.count(p.receiving, "inbound"); got != 0 {
				t.Errorf("%d entries stored or dropped; want none, since a refused reply is an answer", got)
			}
		}},
		{"AFailingJournalWriteLeavesTheEntryPendingMakesNoSecondCallBeforeTheWaitAndTheRetrySettlesIt", func(t *testing.T, p *hubPair) {
			e := newEnv(t)
			receiving := newRig(t, false)
			receiving.geom.Shortname = "lyt"
			hubs := creelfake.NewHubs()
			hubs.Add("lyt", receiving.geom)
			e.store.SetHubs(hubs, "linux")
			fake := listenHubFake(t, receiving, accepted(4))
			id, _, _, err := e.store.PostWith(strand(alpha), "lyt:orch", "hi", creelengine.PostOptions{})
			if err != nil {
				t.Fatalf("PostWith: %v", err)
			}
			e.store.FailJournalWrites(0)
			e.start(t)
			fake.awaitArrivals(t, 1)
			e.clock.AwaitWaitsWhere(1, func(wait time.Duration) bool { return wait == time.Second })
			e.store.FailJournalWrites(-1)
			if got := fake.requests("deliver"); got != 1 || len(e.store.Log(creelengine.LogFilter{ID: id})) != 1 {
				t.Fatalf("after the failed write: %d calls and log %v; want one call and only the post", got, eventsOf(e.store.Log(creelengine.LogFilter{ID: id})))
			}
			e.clock.Advance(time.Second)
			eventually(t, "the retry's remote delivery", func() bool {
				for _, entry := range e.store.Log(creelengine.LogFilter{ID: id}) {
					if entry.Event == "remote-delivered" && entry.RemoteID == 4 {
						return true
					}
				}
				return false
			})
			if got := fake.requests("deliver"); got != 2 {
				t.Errorf("%d calls made; want the failed one and the retry", got)
			}
		}},
	}
	for _, tt := range hourOfRetries {
		tests = append(tests, struct {
			name string
			run  func(t *testing.T, p *hubPair)
		}{tt.name, func(t *testing.T, p *hubPair) {
			released := newHold()
			var fake *hubFake
			switch {
			case tt.hangs:
				fake = listenHubFake(t, p.receiving, func(creelengine.Request) (creelengine.Response, string) {
					<-released.ch
					return creelengine.Response{}, ""
				})
			case tt.answer != nil:
				fake = listenHubFake(t, p.receiving, tt.answer)
			}
			if !tt.parked {
				p.unpark()
			}
			p.startSending()
			t.Cleanup(released.release)
			p.pending("x")
			if fake != nil {
				fake.awaitArrivals(t, 1)
			}
			keep := notCallTimeout
			if tt.driveAll {
				keep = anyWait
			}
			driveClock(t, p.sending, keep)
			returned := p.awaitEvents(p.sending, "outbound-returned", 1)[0]
			if returned.Reason != tt.wantReason || returned.At.Before(epoch.Add(time.Hour)) {
				t.Errorf("outbound-returned entry = %+v; want the reason %q at or after the hour", returned, tt.wantReason)
			}
			lines := p.noticeLines(1)
			if !strings.Contains(lines[0], tt.wantReason) || (tt.absent != "" && strings.Contains(lines[0], tt.absent)) {
				t.Errorf("notice %q; want it to hold %q and none of %q", lines[0], tt.wantReason, tt.absent)
			}
		}})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t, newHubPair(t))
		})
	}
}

func equalWaits(got, want []time.Duration) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
