// crosshub_test.go pins mail across hubs over real sockets.
// It covers a receiving and a sending daemon confirming each other's mail.
// It covers the drops and notices of mail the sender does not confirm, and the trust stamp.

package creelengine_test

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
	rcvAlpha = "lyt:alpha:driver"
	rcvBeta  = "lyt:beta:driver"
	rcvOrch  = "lyt:orch"
	// chaser is the program the receiving hub signs its drop notices with.
	chaser = "lyt:CREEL"
)

// hubPair is two hubs with their daemons not yet started.
// The sending hub is ly, in whose strand alpha the test process sits.
// The receiving hub is lyt, in whose strand rcvAlpha the test process sits.
// Each rig's Hubs knows both hubs.
type hubPair struct {
	t                        *testing.T
	sending, receiving       *rig
	sendingRun, receivingRun *running
}

// newHubPair builds the two rigs and starts nothing.
// The sending rig's Hubs fails the receiving hub's geometry, which parks an outbound worker on its retry wait.
// A row's hand-sent deliver then never races that worker, and unpark clears the failure.
func newHubPair(t *testing.T) *hubPair {
	t.Helper()
	sending, receiving := newRig(t, true), newRig(t, false)
	receiving.geom.Shortname = "lyt"
	receivingAlpha := creelfake.LiveStrand(rcvAlpha)
	receivingAlpha.PanePID = os.Getpid()
	receiving.del.SetStrands(receivingAlpha, creelfake.LiveStrand(rcvBeta), creelfake.LiveStrand(rcvOrch), creelfake.LiveStrand("lyt:orch-2"))
	for _, r := range []*rig{sending, receiving} {
		hubs := creelfake.NewHubs()
		hubs.Add("ly", sending.geom)
		hubs.Add("lyt", receiving.geom)
		r.hubs, r.goos = hubs, "linux"
	}
	sending.hubs.SetGeometryError(receiving.geom.HubPath, errors.New("the receiving hub's daemon is parked"))
	return &hubPair{t: t, sending: sending, receiving: receiving}
}

func (p *hubPair) startSending()   { p.sendingRun = p.sending.start() }
func (p *hubPair) startReceiving() { p.receivingRun = p.receiving.start() }

// unpark lets the sending side reach the receiving hub's geometry again.
func (p *hubPair) unpark() { p.sending.hubs.SetGeometryError(p.receiving.geom.HubPath, nil) }

// client returns a client of the rig's daemon as the strand the test process sits in.
func (p *hubPair) client(r *rig) *creelengine.Client {
	claim := strandClaim(alpha)
	if r == p.receiving {
		claim = strandClaim(rcvAlpha)
	}
	return r.client(claim, nil, creelfake.NewClock(epoch))
}

// logOf returns the log of the rig's daemon, narrowed to the sender `from` when it is not empty.
func (p *hubPair) logOf(r *rig, from string) []creelengine.LogEntry {
	p.t.Helper()
	return mustCall(p.t, p.client(r), creelengine.Request{Verb: creelengine.VerbLog, From: from}).Log
}

// count returns how many log entries of the rig's daemon carry event.
func (p *hubPair) count(r *rig, event string) int {
	p.t.Helper()
	count := 0
	for _, entry := range p.logOf(r, "") {
		if entry.Event == event {
			count++
		}
	}
	return count
}

// notices returns the drop notices the receiving hub's CREEL program posted to its orch, oldest first.
func (p *hubPair) notices() []creelengine.LogEntry {
	p.t.Helper()
	var notices []creelengine.LogEntry
	for _, entry := range p.logOf(p.receiving, chaser) {
		if entry.Event == "posted" && entry.To == rcvOrch {
			notices = append(notices, entry)
		}
	}
	return notices
}

// deliver sends a deliver for mail by hand to the receiving daemon.
func (p *hubPair) deliver(mail creelengine.HubMail) creelengine.Response {
	p.t.Helper()
	return deliverTo(p.t, p.receiving, mail)
}

func deliverTo(t *testing.T, r *rig, mail creelengine.HubMail) creelengine.Response {
	t.Helper()
	line, err := json.Marshal(creelengine.Request{Verb: creelengine.VerbDeliver, Mail: &mail})
	if err != nil {
		t.Fatalf("marshal the deliver: %v", err)
	}
	return r.rawExchange(string(line))
}

// pending posts body from alpha to rcvAlpha on the running sending daemon.
// It returns the tuple its outbound message makes.
func (p *hubPair) pending(body string) creelengine.HubMail {
	p.t.Helper()
	resp := mustCall(p.t, p.client(p.sending), creelengine.Request{Verb: creelengine.VerbPost, Receiver: rcvAlpha, Text: body})
	return creelengine.HubMail{SendingID: resp.ID, Sender: alpha, Receiver: rcvAlpha, Posted: epoch, Body: body}
}

func hashOf(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// confirmYes is a hubFake answer that confirms every request.
func confirmYes(creelengine.Request) (creelengine.Response, string) {
	return creelengine.Response{OK: true, Confirmed: "yes"}, ""
}

// hubFake stands in for a sending hub's daemon on a rig's socket;
// it counts the requests by verb and answers each as its answer function says.
type hubFake struct {
	listener net.Listener
	arrived  chan struct{}
	mu       sync.Mutex
	counts   map[string]int
}

// listenHubFake serves the rig's socket with answer, which may block until the row releases it.
// answer returns a Response and a raw line.
// A non-empty raw line is written verbatim with a line end in place of the Response,
// so a row can reach the client's JSON decoder;
// otherwise the Response is written with the HubPath the answer set, or the rig's hub path if the answer left it empty.
func listenHubFake(t *testing.T, r *rig, answer func(req creelengine.Request) (creelengine.Response, string)) *hubFake {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(r.geom.SocketPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	listener, err := net.Listen("unix", r.geom.SocketPath)
	if err != nil {
		t.Fatalf("listen on %s: %v", r.geom.SocketPath, err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	fake := &hubFake{listener: listener, arrived: make(chan struct{}, 64), counts: map[string]int{}}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go fake.handle(conn, r.geom.HubPath, answer)
		}
	}()
	return fake
}

func (f *hubFake) handle(conn net.Conn, hubPath string, answer func(creelengine.Request) (creelengine.Response, string)) {
	defer conn.Close()
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return
	}
	var req creelengine.Request
	if json.Unmarshal(line, &req) != nil {
		return
	}
	f.mu.Lock()
	f.counts[req.Verb]++
	f.mu.Unlock()
	select {
	case f.arrived <- struct{}{}:
	default:
	}
	resp, raw := answer(req)
	if raw != "" {
		_, _ = conn.Write([]byte(raw + "\n"))
		return
	}
	if resp.HubPath == "" {
		resp.HubPath = hubPath
	}
	data, _ := json.Marshal(resp)
	_, _ = conn.Write(append(data, '\n'))
}

// requests returns how many requests of verb the fake has received.
func (f *hubFake) requests(verb string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts[verb]
}

// awaitArrivals waits until n more requests reached the fake; the real-time deadline only bounds a failing test.
func (f *hubFake) awaitArrivals(t *testing.T, n int) {
	t.Helper()
	for range n {
		select {
		case <-f.arrived:
		case <-time.After(10 * time.Second):
			t.Fatal("no request reached the fake sending hub")
		}
	}
}

// deliveredAsync runs deliver in its own goroutine and returns the channel its response arrives on.
func (p *hubPair) deliveredAsync(mail creelengine.HubMail) <-chan creelengine.Response {
	results := make(chan creelengine.Response, 1)
	go func() { results <- p.deliver(mail) }()
	return results
}

// awaitResponse waits for an async deliver; the real-time deadline only bounds a failing test.
func awaitResponse(t *testing.T, results <-chan creelengine.Response) creelengine.Response {
	t.Helper()
	select {
	case resp := <-results:
		return resp
	case <-time.After(10 * time.Second):
		t.Fatal("the deliver did not return")
		return creelengine.Response{}
	}
}

// wantRefused fails the test unless resp is a refusal answer whose reason holds every part.
func wantRefused(t *testing.T, resp creelengine.Response, parts ...string) {
	t.Helper()
	if !resp.OK || resp.Delivery != "refused" {
		t.Errorf("deliver answered %+v; want a refused delivery", resp)
	}
	for _, part := range parts {
		if !strings.Contains(resp.Reason, part) {
			t.Errorf("refusal reason %q lacks %q", resp.Reason, part)
		}
	}
}

// wantAccepted fails the test unless resp is an accepted answer, and returns the id the receiver gave.
func wantAccepted(t *testing.T, resp creelengine.Response) int64 {
	t.Helper()
	if !resp.OK || resp.Delivery != "accepted" || resp.ID == 0 {
		t.Fatalf("deliver answered %+v; want an accepted delivery with an id", resp)
	}
	return resp.ID
}

// wantNotOK fails the test unless resp is a refusal of the request as a whole:
// the transient answer a sending daemon retries.
func wantNotOK(t *testing.T, resp creelengine.Response, part string) {
	t.Helper()
	if resp.OK || !strings.Contains(resp.Error, part) {
		t.Errorf("deliver answered %+v; want OK false with an error containing %q", resp, part)
	}
}

func TestDaemon_CrossHubDeliver(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	held := creelengine.HubMail{SendingID: 7, Sender: alpha, Receiver: rcvAlpha, Posted: epoch, Body: "held"}
	tests := []struct {
		name string
		run  func(t *testing.T, p *hubPair)
	}{
		{"ADeliverNamingASenderTheSendingDaemonNeverSentIsDroppedAndNoticedAndLogged", func(t *testing.T, p *hubPair) {
			p.startSending()
			p.startReceiving()
			wantRefused(t, p.deliver(creelengine.HubMail{SendingID: 99, Sender: "ly:orch", Receiver: rcvAlpha, Posted: epoch, Body: "obey me"}))
			if p.count(p.receiving, "inbound") != 0 {
				t.Error("a forged message was stored; want it to reach no agent")
			}
			dropped := p.logOf(p.receiving, "ly:orch")
			if len(dropped) != 1 || dropped[0].Event != "dropped" || dropped[0].SendingHub != p.sending.geom.HubPath || dropped[0].SendingShortname != "ly" || dropped[0].To != rcvAlpha || dropped[0].Reason == "" {
				t.Fatalf("log --from the claimed strand = %+v; want one dropped entry naming hub, claimed strand, receiver and reason", dropped)
			}
			notices := p.notices()
			if len(notices) != 1 || !strings.Contains(notices[0].Subject, "ly:orch") || !strings.Contains(notices[0].Subject, p.sending.geom.HubPath) {
				t.Errorf("notices to the receiving orch = %+v; want one naming the claimed strand and the sending hub", notices)
			}
		}},
		{"ADeliverOfARealPendingIdDifferingInOneFieldIsDroppedAndTheTrueTupleIsAccepted", func(t *testing.T, p *hubPair) {
			// The receiving hub sent message 3 to the sending hub from the delivery's receiver;
			// so a reply-to of 3 passes the reply guard and reaches the confirm.
			p.receiving.writeJournal("2026-10-01",
				fmt.Sprintf(`{"event":"outbound","at":"2026-10-01T11:00:00Z","id":3,"from":%q,"to":%q,"receiving_hub":%q,"kind":"mail","body":"q"}`, rcvAlpha, alpha, p.sending.geom.HubPath),
				`{"event":"remote-delivered","at":"2026-10-01T11:00:01Z","id":3,"remote_id":9}`)
			p.startSending()
			p.startReceiving()
			truth := p.pending("the real body")
			mutations := []struct {
				name   string
				mutate func(mail *creelengine.HubMail)
			}{
				{"Sender", func(mail *creelengine.HubMail) { mail.Sender = "ly:beta:driver" }},
				{"Receiver", func(mail *creelengine.HubMail) { mail.Receiver = rcvBeta }},
				{"ReplyTo", func(mail *creelengine.HubMail) { mail.ReplyTo = 3 }},
				{"Mark", func(mail *creelengine.HubMail) { mail.ExpectReply = true }},
				{"PostTime", func(mail *creelengine.HubMail) { mail.Posted = epoch.Add(time.Second) }},
				{"BodyWithTheTrueHashSupplied", func(mail *creelengine.HubMail) {
					mail.Body, mail.BodyHash = "an altered body", hashOf(truth.Body)
				}},
			}
			for i, mutation := range mutations {
				mail := truth
				mutation.mutate(&mail)
				wantRefused(t, p.deliver(mail))
				if got := p.count(p.receiving, "inbound"); got != 0 {
					t.Fatalf("%s: %d messages stored; want none", mutation.name, got)
				}
				if got := p.count(p.receiving, "dropped"); got != i+1 {
					t.Fatalf("%s: %d drops journaled; want %d, one per mutation", mutation.name, got, i+1)
				}
			}
			// A hub's first five drops of an hour post a notice, so the later mutations add none.
			if got, want := len(p.notices()), min(len(mutations), 5); got != want {
				t.Errorf("%d notices after %d drops; want %d", got, len(mutations), want)
			}
			wantAccepted(t, p.deliver(truth))
			if got := p.count(p.receiving, "inbound"); got != 1 {
				t.Errorf("the true tuple stored %d messages; want 1", got)
			}
		}},
		{"AFloodFromOneHubPostsFiveNoticesAndTheFirstDropPastTheHourCountsTheRest", func(t *testing.T, p *hubPair) {
			p.startSending()
			p.startReceiving()
			claimed := []string{"ly:orch", "ly:orch", "ly:orch", "ly:a:orch", "ly:b:orch", "ly:c:orch", "ly:d:orch"}
			for i, sender := range claimed {
				wantRefused(t, p.deliver(creelengine.HubMail{SendingID: int64(10 + i), Sender: sender, Receiver: rcvAlpha, Posted: epoch, Body: "forged"}))
			}
			notices := p.notices()
			if len(notices) != 5 {
				t.Fatalf("%d notices after seven drops; want five", len(notices))
			}
			for i, notice := range notices {
				if has := strings.Contains(notice.Subject, "lym creel log --since 1h"); has != (i == 4) {
					t.Errorf("notice %d names lym creel log --since 1h = %v; want it only on the fifth: %q", i+1, has, notice.Subject)
				}
			}
			if got := p.count(p.receiving, "dropped"); got != 7 {
				t.Errorf("%d drops journaled; want all seven", got)
			}
			p.receiving.clock.Advance(61 * time.Minute)
			wantRefused(t, p.deliver(creelengine.HubMail{SendingID: 20, Sender: "ly:orch", Receiver: rcvAlpha, Posted: epoch, Body: "forged"}))
			notices = p.notices()
			if len(notices) != 6 || !strings.Contains(notices[5].Subject, "3 drops from that hub were journaled since the last notice") {
				t.Errorf("notices after the hour = %d, last %q; want a sixth counting the three drops since the last notice", len(notices), notices[len(notices)-1].Subject)
			}
		}},
		{"ADropWhoseNoticeFailsToPostSpendsNoSlotAndTheNextNoticeCountsIt", func(t *testing.T, p *hubPair) {
			e := newEnv(t)
			confirming := newRig(t, false)
			confirming.geom.Shortname = "xx"
			hubs := creelfake.NewHubs()
			hubs.Add("xx", confirming.geom)
			e.store.SetHubs(hubs, "linux")
			listenHubFake(t, confirming, func(creelengine.Request) (creelengine.Response, string) {
				return creelengine.Response{OK: true, Confirmed: "no"}, ""
			})
			forged := creelengine.HubMail{SendingID: 1, Sender: "xx:orch", Receiver: beta, Posted: epoch, Body: "forged"}
			// The first write of a day file is its high-water record;
			// the drop's own write passes after it, and the notice's write fails.
			e.store.FailJournalWrites(2)
			if answer, err := e.store.Deliver(forged); err != nil || answer.Delivery != "refused" {
				t.Fatalf("a forged deliver = %+v, %v; want the refusal", answer, err)
			}
			e.store.FailJournalWrites(-1)
			if entries := e.store.Log(creelengine.LogFilter{From: chaserOf(e)}); len(entries) != 0 {
				t.Fatalf("a notice whose post failed journaled %v; want nothing", eventsOf(entries))
			}
			forged.SendingID = 2
			if answer, err := e.store.Deliver(forged); err != nil || answer.Delivery != "refused" {
				t.Fatalf("a second forged deliver = %+v, %v; want the refusal", answer, err)
			}
			notices := e.store.Log(creelengine.LogFilter{From: chaserOf(e)})
			if len(notices) != 1 || !strings.Contains(notices[0].Subject, "2 drops from that hub were journaled since the last notice") {
				t.Errorf("notices after the next drop = %+v; want one counting both drops", notices)
			}
		}},
		{"ASecondDeliverOfAStoredMessageAnswersTheSameIdAlsoAfterAForwardAndAfterTheReceiverLeft", func(t *testing.T, p *hubPair) {
			p.startSending()
			p.startReceiving()
			mail := p.pending("once")
			id := wantAccepted(t, p.deliver(mail))
			steps := []struct {
				name   string
				before func()
			}{
				{"Replayed", func() {}},
				{"AfterTheHolderForwardedIt", func() {
					mustCall(t, p.client(p.receiving), creelengine.Request{Verb: creelengine.VerbForward, ID: id, Receiver: rcvBeta})
				}},
				{"AfterTheReceiversStrandLeftTheListing", func() { p.receiving.del.SetStrands(creelfake.LiveStrand(rcvOrch)) }},
			}
			for _, step := range steps {
				step.before()
				if got := wantAccepted(t, p.deliver(mail)); got != id {
					t.Errorf("%s: deliver answered id %d; want the stored id %d", step.name, got, id)
				}
			}
			if got := p.count(p.receiving, "inbound"); got != 1 {
				t.Errorf("%d messages stored; want 1", got)
			}
		}},
		{"AHeldIdDifferingInOneFieldIsRefusedNamingTheClashAndTheRangeAndStoresNothing", func(t *testing.T, p *hubPair) {
			p.startReceiving()
			fake := listenHubFake(t, p.sending, confirmYes)
			wantAccepted(t, p.deliver(held))
			mutations := []struct {
				name   string
				mutate func(mail *creelengine.HubMail)
			}{
				{"Sender", func(mail *creelengine.HubMail) { mail.Sender = "ly:beta:driver" }},
				{"Receiver", func(mail *creelengine.HubMail) { mail.Receiver = rcvBeta }},
				{"ReplyTo", func(mail *creelengine.HubMail) { mail.ReplyTo = 2 }},
				{"Mark", func(mail *creelengine.HubMail) { mail.ExpectReply = true }},
				{"PostTimeWithAllElseRepeated", func(mail *creelengine.HubMail) { mail.Posted = epoch.Add(time.Second) }},
				{"BodyHash", func(mail *creelengine.HubMail) { mail.Body = "another body" }},
			}
			for _, mutation := range mutations {
				mail := held
				mutation.mutate(&mail)
				wantRefused(t, p.deliver(mail), "id clash", "7-7")
			}
			if got := p.count(p.receiving, "inbound"); got != 1 {
				t.Errorf("%d messages stored; want only the first", got)
			}
			if got := fake.requests("confirm"); got != 1 {
				t.Errorf("the sending hub was asked %d times; want once, since a clash is refused before any confirm", got)
			}
		}},
		{"AConfirmAfterADeliveryRecordAfterAReturnRecordAndWithADifferingBodyHashAnswersNo", func(t *testing.T, p *hubPair) {
			outbound := func(id int) string {
				return fmt.Sprintf(`{"event":"outbound","at":"2026-10-01T12:00:00Z","id":%d,"from":%q,"to":%q,"receiving_hub":%q,"kind":"mail","body":"b"}`, id, alpha, rcvAlpha, p.receiving.geom.HubPath)
			}
			p.sending.writeJournal("2026-10-01",
				outbound(5), outbound(6), outbound(7),
				`{"event":"remote-delivered","at":"2026-10-01T12:00:01Z","id":6,"remote_id":9}`,
				`{"event":"outbound-returned","at":"2026-10-01T12:00:01Z","id":7,"reason":"gone"}`)
			p.startSending()
			confirm := func(id int64, hash string) string {
				mail := creelengine.HubMail{SendingID: id, Sender: alpha, Receiver: rcvAlpha, Posted: epoch, BodyHash: hash}
				line, err := json.Marshal(creelengine.Request{Verb: creelengine.VerbConfirm, Mail: &mail})
				if err != nil {
					t.Fatalf("marshal the confirm: %v", err)
				}
				resp := p.sending.rawExchange(string(line))
				if !resp.OK {
					t.Fatalf("confirm answered %+v; want OK", resp)
				}
				return resp.Confirmed
			}
			for _, tt := range []struct {
				name string
				id   int64
				hash string
				want string
			}{
				{"PendingWithTheTrueTuple", 5, hashOf("b"), "yes"},
				{"AfterTheDeliveryRecord", 6, hashOf("b"), "no"},
				{"AfterTheReturnRecord", 7, hashOf("b"), "no"},
				{"WithADifferingBodyHash", 5, hashOf("not b"), "no"},
				{"ForAnIdItNeverPosted", 8, hashOf("b"), "no"},
			} {
				if got := confirm(tt.id, tt.hash); got != tt.want {
					t.Errorf("%s: confirm answered %q; want %q", tt.name, got, tt.want)
				}
			}
		}},
		{"TheSendingDaemonDownAnswersNotOKStartsNoDaemonAndStoresAndDropsNothing", func(t *testing.T, p *hubPair) {
			p.startReceiving()
			wantNotOK(t, p.deliver(held), "did not answer the confirm")
			if spawns := p.receiving.hubs.Spawns(p.sending.geom.HubPath); spawns != 0 {
				t.Errorf("the confirm started the sending daemon %d times; want never", spawns)
			}
			if p.count(p.receiving, "inbound")+p.count(p.receiving, "dropped") != 0 {
				t.Error("a confirm that could not be asked stored or dropped the message; want neither")
			}
		}},
		{"AConfirmHeldPastTheTimeoutAndOneAnsweringNeitherWordEachAnswerNotOKWithNothingStoredOrDropped", func(t *testing.T, p *hubPair) {
			p.startReceiving()
			release := make(chan struct{})
			var calls atomic.Int32
			listenHubFake(t, p.sending, func(creelengine.Request) (creelengine.Response, string) {
				if calls.Add(1) == 1 {
					<-release
					return creelengine.Response{OK: true, Confirmed: "yes"}, ""
				}
				return creelengine.Response{OK: true, Confirmed: "maybe"}, ""
			})
			t.Cleanup(func() { close(release) })
			timedOut := p.deliveredAsync(held)
			p.receiving.clock.AwaitWaitsWhere(1, func(d time.Duration) bool { return d == 10*time.Second })
			p.receiving.clock.Advance(10 * time.Second)
			wantNotOK(t, awaitResponse(t, timedOut), "did not answer the confirm")
			wantNotOK(t, p.deliver(creelengine.HubMail{SendingID: 8, Sender: alpha, Receiver: rcvAlpha, Posted: epoch, Body: "odd"}), "neither yes nor no")
			if p.count(p.receiving, "inbound")+p.count(p.receiving, "dropped") != 0 {
				t.Error("a failed confirm stored or dropped the message; want neither")
			}
		}},
		{"WhileAConfirmIsHeldAnInboxOnTheReceivingDaemonStillAnswers", func(t *testing.T, p *hubPair) {
			p.startReceiving()
			release := make(chan struct{})
			fake := listenHubFake(t, p.sending, func(creelengine.Request) (creelengine.Response, string) {
				<-release
				return creelengine.Response{OK: true, Confirmed: "yes"}, ""
			})
			results := p.deliveredAsync(held)
			fake.awaitArrivals(t, 1)
			mustCall(t, p.client(p.receiving), creelengine.Request{Verb: creelengine.VerbInbox})
			close(release)
			wantAccepted(t, awaitResponse(t, results))
		}},
		{"TwoOverlappingDeliveriesOfOneMessageStoreItOnceAndAnswerOneId", func(t *testing.T, p *hubPair) {
			p.startReceiving()
			both := make(chan struct{})
			var arrived atomic.Int32
			listenHubFake(t, p.sending, func(creelengine.Request) (creelengine.Response, string) {
				if arrived.Add(1) == 2 {
					close(both)
				}
				<-both
				return creelengine.Response{OK: true, Confirmed: "yes"}, ""
			})
			first, second := p.deliveredAsync(held), p.deliveredAsync(held)
			if a, b := wantAccepted(t, awaitResponse(t, first)), wantAccepted(t, awaitResponse(t, second)); a != b {
				t.Errorf("the two deliveries got ids %d and %d; want one id", a, b)
			}
			if got := p.count(p.receiving, "inbound"); got != 1 {
				t.Errorf("%d messages stored; want 1", got)
			}
		}},
		{"TwoOverlappingDeliveriesOfOneIdWithDifferingBodiesStoreOneAndRefuseTheOtherAsAClash", func(t *testing.T, p *hubPair) {
			p.startReceiving()
			both := make(chan struct{})
			var arrived atomic.Int32
			listenHubFake(t, p.sending, func(creelengine.Request) (creelengine.Response, string) {
				if arrived.Add(1) == 2 {
					close(both)
				}
				<-both
				return creelengine.Response{OK: true, Confirmed: "yes"}, ""
			})
			other := held
			other.Body = "a different body"
			firstAnswer, secondAnswer := p.deliveredAsync(held), p.deliveredAsync(other)
			answers := []creelengine.Response{awaitResponse(t, firstAnswer), awaitResponse(t, secondAnswer)}
			accepted, refused := 0, 0
			for _, resp := range answers {
				switch resp.Delivery {
				case "accepted":
					accepted++
				case "refused":
					refused++
					if !strings.Contains(resp.Reason, "id clash") {
						t.Errorf("the refused delivery's reason = %q; want an id clash", resp.Reason)
					}
				}
			}
			if accepted != 1 || refused != 1 || p.count(p.receiving, "inbound") != 1 {
				t.Errorf("answers %+v stored %d; want one accepted, one refused and one stored", answers, p.count(p.receiving, "inbound"))
			}
		}},
		{"ARetryThatStoredWhileAnotherAttemptsConfirmWasHeldAnswersAcceptedWhenThatConfirmAnswersNo", func(t *testing.T, p *hubPair) {
			p.startReceiving()
			release := make(chan struct{})
			var calls atomic.Int32
			fake := listenHubFake(t, p.sending, func(creelengine.Request) (creelengine.Response, string) {
				if calls.Add(1) == 1 {
					<-release
					return creelengine.Response{OK: true, Confirmed: "no"}, ""
				}
				return creelengine.Response{OK: true, Confirmed: "yes"}, ""
			})
			slow := p.deliveredAsync(held)
			fake.awaitArrivals(t, 1)
			id := wantAccepted(t, p.deliver(held))
			close(release)
			if got := wantAccepted(t, awaitResponse(t, slow)); got != id {
				t.Errorf("the held attempt answered id %d; want the stored id %d", got, id)
			}
			if got := p.count(p.receiving, "dropped"); got != 0 {
				t.Errorf("%d drops journaled; want none for a message that was stored", got)
			}
		}},
		{"TwoHubsDeliveringToEachOtherAtOnceBothComplete", func(t *testing.T, p *hubPair) {
			p.unpark()
			p.startSending()
			p.startReceiving()
			forward := p.pending("to lyt")
			resp := mustCall(t, p.client(p.receiving), creelengine.Request{Verb: creelengine.VerbPost, Receiver: alpha, Text: "to ly"})
			back := creelengine.HubMail{SendingID: resp.ID, Sender: rcvAlpha, Receiver: alpha, Posted: epoch, Body: "to ly"}
			forwardAnswer, backAnswer := p.deliveredAsync(forward), make(chan creelengine.Response, 1)
			go func() { backAnswer <- deliverTo(t, p.sending, back) }()
			wantAccepted(t, awaitResponse(t, forwardAnswer))
			wantAccepted(t, awaitResponse(t, backAnswer))
		}},
		{"ADeliverWhileALiveStrandHasNoKnownPaneProcessIsAccepted", func(t *testing.T, p *hubPair) {
			unknown := creelfake.LiveStrand(rcvAlpha)
			unknown.PanePID = -1
			p.receiving.del.SetStrands(unknown, creelfake.LiveStrand(rcvOrch))
			p.startSending()
			p.startReceiving()
			wantAccepted(t, p.deliver(p.pending("hello")))
		}},
		{"TheStoredTrustFollowsTheOrchOfAListedHubOnLinuxAndTheListAtEachArrival", func(t *testing.T, p *hubPair) {
			read := func(r *rig) creelengine.Message {
				t.Helper()
				resp := mustCall(t, p.client(r), creelengine.Request{Verb: creelengine.VerbRead})
				if len(resp.Messages) != 1 {
					t.Fatalf("read %d messages; want 1", len(resp.Messages))
				}
				return resp.Messages[0]
			}
			p.receiving.hubs.SetTrusted([]string{"ly"}, nil)
			p.startReceiving()
			listenHubFake(t, p.sending, confirmYes)
			wantAccepted(t, p.deliver(creelengine.HubMail{SendingID: 1, Sender: "ly:orch", Receiver: rcvAlpha, Posted: epoch, Body: "first"}))
			if first := read(p.receiving); !first.Trusted || first.HubStamp() != creelengine.StampTrusted {
				t.Errorf("the orch of a listed hub arrived as %+v; want trusted", first)
			}
			p.receiving.hubs.SetTrusted(nil, nil)
			wantAccepted(t, p.deliver(creelengine.HubMail{SendingID: 2, Sender: "ly:orch", Receiver: rcvAlpha, Posted: epoch, Body: "second"}))
			if second := read(p.receiving); second.Trusted || second.HubStamp() != creelengine.StampUntrusted {
				t.Errorf("the same orch after the list changed arrived as %+v; want untrusted", second)
			}
		}},
		{"TheStoredTrustIsUntrustedForEveryOtherSenderAndForAFailedListAndAnotherPlatform", func(t *testing.T, p *hubPair) {
			cases := []struct {
				name       string
				sender     string
				trusted    []string
				trustedErr error
				goos       string
				want       bool
			}{
				{"TaskOrchOfAListedHub", "ly:build:orch", []string{"ly"}, nil, "", true},
				{"NonOrchStrand", "ly:beta:driver", []string{"ly"}, nil, "", false},
				{"OrchThread", "ly:orch-2", []string{"ly"}, nil, "", false},
				{"OrchOfAnUnlistedHub", "ly:orch", []string{"qq"}, nil, "", false},
				{"AFailedListRead", "ly:orch", []string{"ly"}, errors.New("creel.yaml is unreadable"), "", false},
				{"AnotherPlatform", "ly:orch", []string{"ly"}, nil, "darwin", false},
			}
			for _, tt := range cases {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					p := newHubPair(t)
					p.receiving.hubs.SetTrusted(tt.trusted, tt.trustedErr)
					if tt.goos != "" {
						p.receiving.goos = tt.goos
					}
					p.startReceiving()
					listenHubFake(t, p.sending, confirmYes)
					wantAccepted(t, p.deliver(creelengine.HubMail{SendingID: 1, Sender: tt.sender, Receiver: rcvAlpha, Posted: epoch, Body: "hello"}))
					resp := mustCall(t, p.client(p.receiving), creelengine.Request{Verb: creelengine.VerbRead})
					if len(resp.Messages) != 1 || resp.Messages[0].Trusted != tt.want {
						t.Errorf("the arrived message = %+v; want trusted %v", resp.Messages, tt.want)
					}
				})
			}
		}},
		{"StepOneAndTwoRefusalsAreAnswersNotDrops", func(t *testing.T, p *hubPair) {
			p.startReceiving()
			p.receiving.hubs.Add("dup", creelengine.Geometry{HubPath: "/hubs/d1", Shortname: "dup"})
			p.receiving.hubs.Add("dup", creelengine.Geometry{HubPath: "/hubs/d2", Shortname: "dup"})
			tests := []struct {
				name string
				mail creelengine.HubMail
				want string
			}{
				{"AProgramSender", creelengine.HubMail{SendingID: 1, Sender: "ly:BATTEN", Receiver: rcvAlpha, Posted: epoch, Body: "hi"}, "no strand name"},
				{"ASenderOfTheReceiversOwnShortname", creelengine.HubMail{SendingID: 1, Sender: rcvBeta, Receiver: rcvAlpha, Posted: epoch, Body: "hi"}, "strand of this hub"},
				{"AProgramReceiver", creelengine.HubMail{SendingID: 1, Sender: alpha, Receiver: "lyt:BATTEN", Posted: epoch, Body: "hi"}, "no strand name"},
				{"ALineOverTheCap", creelengine.HubMail{SendingID: 1, Sender: alpha, Receiver: rcvAlpha, Posted: epoch, Body: strings.Repeat("x", 9<<10)}, "write it to a file"},
				{"AnEmptyBody", creelengine.HubMail{SendingID: 1, Sender: alpha, Receiver: rcvAlpha, Posted: epoch}, "empty"},
				{"ASendingShortnameUnknownToTheDirectory", creelengine.HubMail{SendingID: 1, Sender: "qq:orch", Receiver: rcvAlpha, Posted: epoch, Body: "hi"}, "no hub with the shortname"},
				{"ASendingShortnameAddedTwice", creelengine.HubMail{SendingID: 1, Sender: "dup:orch", Receiver: rcvAlpha, Posted: epoch, Body: "hi"}, "distinct shortnames"},
				{"AnUnknownReceiverAfterTheDuplicateCheck", creelengine.HubMail{SendingID: 1, Sender: alpha, Receiver: "lyt:nobody:driver", Posted: epoch, Body: "hi"}, "unknown receiver"},
			}
			for _, tt := range tests {
				if resp := p.deliver(tt.mail); !resp.OK || resp.Delivery != "refused" || !strings.Contains(resp.Reason, tt.want) {
					t.Errorf("%s: deliver answered %+v; want a refusal containing %q", tt.name, resp, tt.want)
				}
			}
			if got := p.count(p.receiving, "dropped"); got != 0 {
				t.Errorf("%d drops journaled; want none, since a refusal is an answer", got)
			}
		}},
		{"ALookupFaultAFailedStrandListingAndARequestWithNoMailAnswerNotOKAndLeaveNoDrop", func(t *testing.T, p *hubPair) {
			p.startReceiving()
			p.receiving.hubs.SetLookupError("ly", errors.New("the machine record is unreadable"))
			wantNotOK(t, p.deliver(held), "the machine record is unreadable")
			p.receiving.hubs.SetLookupError("ly", nil)
			p.receiving.del.SetStrandsError(errors.New("reed state is unreadable"))
			wantNotOK(t, p.deliver(held), "reed state is unreadable")
			p.receiving.del.SetStrandsError(nil)
			wantNotOK(t, p.receiving.rawExchange(`{"verb":"deliver"}`), "carries no mail")
			if p.count(p.receiving, "dropped")+p.count(p.receiving, "inbound") != 0 {
				t.Error("a transient failure stored or dropped the message; want neither")
			}
		}},
		{"ADaemonToldNoHubDirectoryRefusesBothVerbs", func(t *testing.T, p *hubPair) {
			bare := newRig(t, false)
			bare.start()
			for _, verb := range []string{creelengine.VerbDeliver, creelengine.VerbConfirm} {
				mail := held
				line, _ := json.Marshal(creelengine.Request{Verb: verb, Mail: &mail})
				if resp := bare.rawExchange(string(line)); resp.OK || !strings.Contains(resp.Error, "serves no other hub") {
					t.Errorf("%s on a daemon with no hub directory answered %+v; want a refusal", verb, resp)
				}
			}
		}},
		{"AJournalWriteThatFailsAtTheStoreOrTheDropGivesTheTransientAnswerAndTheRetryCompletes", func(t *testing.T, p *hubPair) {
			e := newEnv(t)
			confirming := newRig(t, false)
			confirming.geom.Shortname = "xx"
			hubs := creelfake.NewHubs()
			hubs.Add("xx", confirming.geom)
			e.store.SetHubs(hubs, "linux")
			var answer atomic.Value
			answer.Store("yes")
			listenHubFake(t, confirming, func(creelengine.Request) (creelengine.Response, string) {
				return creelengine.Response{OK: true, Confirmed: answer.Load().(string)}, ""
			})
			mail := creelengine.HubMail{SendingID: 1, Sender: "xx:orch", Receiver: beta, Posted: epoch, Body: "hi"}
			e.store.FailJournalWrites(0)
			if _, err := e.store.Deliver(mail); err == nil {
				t.Fatal("Deliver with a failing journal returned nil; want the transient error")
			}
			if entries := e.store.Log(creelengine.LogFilter{}); len(entries) != 0 {
				t.Errorf("a failed store journaled %v; want nothing", eventsOf(entries))
			}
			e.store.FailJournalWrites(-1)
			if got, err := e.store.Deliver(mail); err != nil || got.Delivery != "accepted" {
				t.Errorf("the retry = %+v, %v; want accepted", got, err)
			}
			answer.Store("no")
			forged := creelengine.HubMail{SendingID: 2, Sender: "xx:orch", Receiver: beta, Posted: epoch, Body: "forged"}
			e.store.FailJournalWrites(0)
			if _, err := e.store.Deliver(forged); err == nil {
				t.Fatal("a drop whose journal write fails returned nil; want the transient error")
			}
			e.store.FailJournalWrites(-1)
			if entries := e.store.Log(creelengine.LogFilter{From: chaserOf(e)}); len(entries) != 0 {
				t.Errorf("a drop that was not journaled sent notices %v; want none", eventsOf(entries))
			}
		}},
	}
	for _, tt := range tests {
		if tt.run == nil {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.run(t, newHubPair(t))
		})
	}
}

// chaserOf returns the name the env's hub signs its notices with.
func chaserOf(e *env) string { return e.geom.Shortname + ":CREEL" }
