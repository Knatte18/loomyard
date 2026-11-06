// journal_test.go pins how the journal folds into the pool and how retention carries live state out of old day files.

package creelengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	alpha = "ly:alpha:driver"
	beta  = "ly:beta:driver"
	gamma = "ly:gamma:driver"
	delta = "ly:delta:driver"
	orch  = "ly:orch"
	// remoteHub and remoteOrch are a hub of another shortname and its orch, the far side of mail across hubs.
	remoteHub  = "/hubs/xx"
	remoteOrch = "xx:orch"
)

// testClock is a Clock that stands still until a test moves it.
type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { return c.now }

func (c *testClock) After(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- c.now
	return ch
}

var journalEpoch = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// recordLine renders rec as one journal line.
func recordLine(t *testing.T, rec record) string {
	t.Helper()
	if rec.At.IsZero() {
		rec.At = journalEpoch
	}
	line, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal %+v: %v", rec, err)
	}
	return string(line)
}

// messageShape is the part of a Message a row compares.
type messageShape struct {
	Holder        string
	Path          []string
	Status        Status
	EndedUnreadBy string
}

func shapesOf(p *pool) map[int64]messageShape {
	shapes := map[int64]messageShape{}
	for id, msg := range p.messages {
		shapes[id] = messageShape{msg.Holder, msg.Path, msg.Status, msg.EndedUnreadBy}
	}
	return shapes
}

// TestJournal_Fold mutates the process-global logger output, so it does not run in parallel.
func TestJournal_Fold(t *testing.T) {
	tiergate.Require(t, 0)
	line := func(rec record) string { return recordLine(t, rec) }
	posted := func(id int64, from, to, body string) string {
		return line(record{Event: eventPosted, ID: id, From: from, To: to, Body: body})
	}
	injectPosted := func(id int64, to, text string) string {
		return line(record{Event: eventPosted, Kind: kindInject, ID: id, From: alpha, To: to, Body: text, TTLSeconds: 600})
	}
	const torn = `{"event":"posted","id":2,"fr`
	kept := posted(1, alpha, beta, "kept") + "\n"
	keptAndSecond := kept + posted(2, alpha, beta, "second") + "\n"
	keptAndBlanks := kept + "\n\n"
	empty, blanks := "", "\n\n"
	onlyMessageOne := func(t *testing.T, p *pool) {
		if len(p.messages) != 1 || p.messages[1] == nil {
			t.Errorf("messages = %+v, want only message 1", shapesOf(p))
		}
	}
	tests := []struct {
		name string
		// files maps a day file name to its raw content; a nil map leaves the journal directory missing.
		files    map[string]string
		wantErr  []string
		wantWarn int
		check    func(t *testing.T, p *pool)
		// wantNewest, when set, is the newest day file's content after the journal opens over the folded pool;
		// wantCutBytes is the byte count the repair's Warn names, zero for a repair that cuts nothing.
		wantNewest   *string
		wantCutBytes int
	}{
		{
			name: "a missing journal directory folds to an empty pool",
			check: func(t *testing.T, p *pool) {
				if len(p.messages)+len(p.injects)+len(p.holds)+len(p.events) != 0 || p.maxID != 0 {
					t.Errorf("pool = %+v, want empty", p)
				}
			},
		},
		{
			name:  "an empty journal directory folds to an empty pool",
			files: map[string]string{},
			check: func(t *testing.T, p *pool) {
				if len(p.messages)+len(p.injects)+len(p.holds)+len(p.events) != 0 || p.maxID != 0 {
					t.Errorf("pool = %+v, want empty", p)
				}
			},
		},
		{
			name: "posted, forwarded, read, returned and withheld fold to holder, path, status and ended-unread receiver",
			files: map[string]string{"2026-10-01.jsonl": strings.Join([]string{
				posted(1, alpha, beta, "first line\nsecond line"),
				line(record{Event: eventForwarded, ID: 1, By: beta, To: gamma}),
				posted(2, alpha, beta, "other"),
				line(record{Event: eventRead, ID: 2, By: beta}),
				posted(3, alpha, gamma, "third"),
				line(record{Event: eventForwarded, ID: 3, By: gamma, To: delta}),
				line(record{Event: eventReturned, ID: 3, To: gamma, Receiver: delta}),
				posted(4, "ly:BATTEN", orch, "from a program"),
				posted(5, "ly:EXTERNAL", beta, "from outside"),
				line(record{Event: eventWithheld, To: orch, Notice: "creel-notices"}),
			}, "\n") + "\n"},
			check: func(t *testing.T, p *pool) {
				want := map[int64]messageShape{
					1: {Holder: gamma, Path: []string{beta}, Status: StatusUnread},
					2: {Holder: beta, Status: StatusRead},
					3: {Holder: gamma, Status: StatusUnread, EndedUnreadBy: delta},
					4: {Holder: orch, Status: StatusUnread},
					5: {Holder: beta, Status: StatusUnread},
				}
				if got := shapesOf(p); !reflect.DeepEqual(got, want) {
					t.Errorf("messages = %+v, want %+v", got, want)
				}
				if got := p.messages[1].Subject; got != "first line" {
					t.Errorf("subject = %q, want the first line of the body", got)
				}
				if !p.messages[3].EndedUnreadAt.Equal(journalEpoch) {
					t.Errorf("ended-unread time = %v, want the returned record's time", p.messages[3].EndedUnreadAt)
				}
				if !reflect.DeepEqual(p.programs, map[string]bool{"ly:BATTEN": true}) {
					t.Errorf("program mailboxes = %v, want only ly:BATTEN (EXTERNAL has none)", p.programs)
				}
			},
		},
		{
			name: "a marked post folds with its mark, and a reply by the holder, also after a forward, sets Replied",
			files: map[string]string{"2026-10-01.jsonl": strings.Join([]string{
				line(record{Event: eventPosted, ID: 1, From: alpha, To: beta, Body: "question", ExpectReply: true}),
				line(record{Event: eventRead, ID: 1, By: beta}),
				line(record{Event: eventPosted, Kind: kindReply, ID: 2, From: beta, To: alpha, Body: "answer", ReplyTo: 1}),
				line(record{Event: eventPosted, ID: 3, From: alpha, To: beta, Body: "second", ExpectReply: true}),
				line(record{Event: eventForwarded, ID: 3, By: beta, To: gamma}),
				line(record{Event: eventPosted, Kind: kindReply, ID: 4, From: gamma, To: alpha, Body: "answer", ReplyTo: 3}),
				posted(5, alpha, beta, "plain"),
				line(record{Event: eventPosted, Kind: kindReply, ID: 6, From: beta, To: alpha, Body: "answer", ReplyTo: 5}),
			}, "\n") + "\n"},
			check: func(t *testing.T, p *pool) {
				type marks struct{ ExpectReply, Replied bool }
				want := map[int64]marks{1: {true, true}, 2: {}, 3: {true, true}, 4: {}, 5: {}, 6: {}}
				got := map[int64]marks{}
				for id, msg := range p.messages {
					got[id] = marks{msg.ExpectReply, msg.Replied}
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("marks = %+v, want %+v", got, want)
				}
			},
		},
		{
			name: "chase records fold into the message's chase count, mark, clock and reason, and a program's post stays out of readers while its read puts it in",
			files: map[string]string{"2026-10-01.jsonl": strings.Join([]string{
				line(record{Event: eventPosted, ID: 1, From: alpha, To: beta, Body: "question", ExpectReply: true}),
				line(record{Event: eventRead, ID: 1, By: beta}),
				line(record{Event: eventChaseAged, ID: 1, ConsumedSeconds: 900}),
				line(record{Event: eventPosted, ID: 2, From: "ly:CREEL", To: beta, Body: "chase", Chase: 1, Chased: 1, ConsumedSeconds: 900}),
				line(record{Event: eventChased, ID: 1, To: beta, Chase: 1}),
				line(record{Event: eventPosted, ID: 3, From: "ly:CREEL", To: "ly:BATTEN", Body: "sender notice"}),
				line(record{Event: eventPosted, ID: 4, From: "ly:BATTEN", To: alpha, Body: "post only"}),
				line(record{Event: eventRead, By: "ly:WEBSTER"}),
				line(record{Event: eventUnanswered, ID: 1, To: beta, Reason: UnansweredNoReply}),
			}, "\n") + "\n"},
			check: func(t *testing.T, p *pool) {
				msg := p.messages[1]
				if msg.Chases != 1 || msg.ChaseMark != 15*time.Minute || msg.ChaseConsumed != 15*time.Minute || msg.Unanswered != UnansweredNoReply || !msg.ReadAt.Equal(journalEpoch) {
					t.Errorf("message 1 = chases %d, mark %v, clock %v, unanswered %q, read at %v; want 1, 15m, 15m, no-reply, the read time", msg.Chases, msg.ChaseMark, msg.ChaseConsumed, msg.Unanswered, msg.ReadAt)
				}
				if p.messages[2].ChaseOf != 1 || p.chased[1] != 1 {
					t.Errorf("chase of = %d and highest chased = %d; want 1 and 1", p.messages[2].ChaseOf, p.chased[1])
				}
				if want := map[string]bool{"ly:BATTEN": true, "ly:WEBSTER": true}; !reflect.DeepEqual(p.programs, want) {
					t.Errorf("program mailboxes = %v; want %v, with no CREEL since a chase and a sender notice make no mailbox", p.programs, want)
				}
				if want := map[string]string{"ly:WEBSTER": "2026-10-01"}; !reflect.DeepEqual(p.readers, want) {
					t.Errorf("readers = %v; want %v, since ly:BATTEN only posted", p.readers, want)
				}
				var chased, unanswered LogEntry
				for _, entry := range p.events {
					switch entry.Event {
					case eventChased:
						chased = entry
					case eventUnanswered:
						unanswered = entry
					}
				}
				if chased.Chase != 1 || chased.ID != 1 || unanswered.Reason != UnansweredNoReply {
					t.Errorf("log entries = chased %+v, unanswered %+v; want chase 1 of message 1 and the no-reply reason", chased, unanswered)
				}
			},
		},
		{
			name: "outbound records fold into the outbound map and no message, delivered and returned settle them, and their log entries carry the parties, subject and receiving hub",
			files: map[string]string{"2026-10-01.jsonl": strings.Join([]string{
				line(record{Event: eventOutbound, ID: 1, From: alpha, To: remoteOrch, ReceivingHub: remoteHub, Body: "first line\nmore", ExpectReply: true}),
				line(record{Event: eventRemoteDelivered, ID: 1, RemoteID: 40}),
				line(record{Event: eventOutbound, ID: 2, From: alpha, To: remoteOrch, ReceivingHub: remoteHub, Body: "second"}),
				line(record{Event: eventOutboundReturned, ID: 2, Reason: "no-hub"}),
			}, "\n") + "\n"},
			check: func(t *testing.T, p *pool) {
				if len(p.messages) != 0 {
					t.Errorf("messages = %+v, want none: an outbound message is held by no local strand", shapesOf(p))
				}
				first, second := p.outbound[1], p.outbound[2]
				if first == nil || first.State != outboundDelivered || first.RemoteID != 40 || second == nil || second.State != outboundReturned || second.Reason != "no-hub" {
					t.Fatalf("outbound = %+v, want 1 delivered as remote 40 and 2 returned for no-hub", p.outbound)
				}
				if got := second.message(); got.Holder != remoteOrch || got.Status != StatusUndelivered || got.Subject != "second" {
					t.Errorf("returned outbound message = %+v, want holder %s, status undelivered and subject second", got, remoteOrch)
				}
				logged := map[string]LogEntry{}
				for _, entry := range p.events {
					logged[entry.Event+fmt.Sprint(entry.ID)] = entry
				}
				delivered, returned := logged[eventRemoteDelivered+"1"], logged[eventOutboundReturned+"2"]
				if delivered.From != alpha || delivered.To != remoteOrch || delivered.Subject != "first line" || delivered.ReceivingHub != remoteHub || delivered.RemoteID != 40 {
					t.Errorf("remote-delivered entry = %+v, want the parties, subject, receiving hub and remote id of message 1", delivered)
				}
				if returned.From != alpha || returned.To != remoteOrch || returned.Subject != "second" || returned.ReceivingHub != remoteHub || returned.Reason != "no-hub" {
					t.Errorf("outbound-returned entry = %+v, want the parties, subject, receiving hub and reason of message 2", returned)
				}
			},
		},
		{
			name: "an outbound reply marks the local message replied, an inbound record folds as unread mail with its hub fields and a forward leaves DeliveredTo, and an inbound reply marks its outbound message replied",
			files: map[string]string{"2026-10-01.jsonl": strings.Join([]string{
				line(record{Event: eventPosted, ID: 1, From: alpha, To: beta, Body: "question", ExpectReply: true}),
				line(record{Event: eventRead, ID: 1, By: beta}),
				line(record{Event: eventOutbound, ID: 2, From: beta, To: remoteOrch, ReceivingHub: remoteHub, Kind: kindReply, Body: "answer", ReplyTo: 9, Answers: 1}),
				line(record{Event: eventInbound, ID: 3, From: remoteOrch, To: beta, Body: "ask", SendingHub: remoteHub, SendingShortname: "xx", SendingID: 31, Trusted: true, ExpectReply: true,
					RemotePosted: journalEpoch.Add(-time.Hour), DeliveredTo: beta}),
				line(record{Event: eventForwarded, ID: 3, By: beta, To: gamma}),
				line(record{Event: eventOutbound, ID: 4, From: gamma, To: remoteOrch, ReceivingHub: remoteHub, Body: "q", ExpectReply: true}),
				line(record{Event: eventInbound, ID: 5, From: remoteOrch, To: gamma, Kind: kindReply, ReplyTo: 4, Body: "re", SendingHub: remoteHub, SendingID: 32}),
			}, "\n") + "\n"},
			check: func(t *testing.T, p *pool) {
				if !p.messages[1].Replied {
					t.Errorf("message 1 Replied = false, want true after the outbound reply that answers it")
				}
				inbound := p.messages[3]
				if inbound.Holder != gamma || inbound.DeliveredTo != beta || inbound.Status != StatusUnread || inbound.SendingHub != remoteHub || inbound.SendingID != 31 ||
					!inbound.Trusted || !inbound.RemotePosted.Equal(journalEpoch.Add(-time.Hour)) || !inbound.ExpectReply || inbound.HubStamp() != StampTrusted {
					t.Errorf("inbound message = %+v, want unread mail held by %s, delivered to %s, trusted, from %s as id 31", inbound, gamma, beta, remoteHub)
				}
				if reply := p.messages[5]; reply.Kind != kindReply || reply.ReplyTo != 4 || reply.HubStamp() != StampUntrusted || reply.DeliveredTo != gamma {
					t.Errorf("inbound reply = %+v, want an untrusted reply to 4 delivered to %s", reply, gamma)
				}
				if !p.outbound[4].Replied || p.outbound[2].Replied {
					t.Errorf("outbound replied marks = %v and %v for 4 and 2, want true and false", p.outbound[4].Replied, p.outbound[2].Replied)
				}
				if len(p.programs) != 0 {
					t.Errorf("program mailboxes = %v, want none: another hub's sender makes no mailbox here", p.programs)
				}
				logged := map[int64]LogEntry{}
				for _, entry := range p.events {
					if entry.Event == eventInbound || entry.Event == eventOutbound {
						logged[entry.ID] = entry
					}
				}
				if entry := logged[3]; entry.Kind != kindMail || !entry.ExpectReply || entry.SendingHub != remoteHub || entry.SendingShortname != "xx" || entry.SendingID != 31 {
					t.Errorf("inbound entry = %+v, want kind mail, the mark and the sending hub's path, shortname and id", entry)
				}
				if entry := logged[5]; entry.Kind != kindReply || entry.ReplyTo != 4 {
					t.Errorf("inbound reply entry = %+v, want kind reply answering 4", entry)
				}
				if entry := logged[2]; entry.Kind != kindReply || entry.ReplyTo != 9 || entry.Answers != 1 || entry.ReceivingHub != remoteHub {
					t.Errorf("outbound reply entry = %+v, want kind reply to 9 answering 1 for %s", entry, remoteHub)
				}
			},
		},
		{
			name:  "a dropped record joins the log with the claimed strand as sender and folds no entry",
			files: map[string]string{"2026-10-01.jsonl": line(record{Event: eventDropped, From: "xx:gamma:driver", To: beta, SendingHub: remoteHub, SendingShortname: "xx", Reason: "unconfirmed"}) + "\n"},
			check: func(t *testing.T, p *pool) {
				if len(p.messages)+len(p.outbound) != 0 || len(p.events) != 1 {
					t.Fatalf("pool = %d messages, %d outbound, %d events; want only the one log entry", len(p.messages), len(p.outbound), len(p.events))
				}
				if entry := p.events[0]; entry.Event != eventDropped || entry.From != "xx:gamma:driver" || entry.To != beta || entry.SendingHub != remoteHub || entry.SendingShortname != "xx" || entry.Reason != "unconfirmed" {
					t.Errorf("dropped entry = %+v, want the claimed strand as sender, the receiver, the sending hub, its shortname and the reason", entry)
				}
			},
		},
		{
			name: "an unknown key and an unknown event name are ignored",
			files: map[string]string{"2026-10-01.jsonl": `{"event":"posted","id":1,"from":"` + alpha + `","to":"` + beta + `","body":"hi","at":"2026-10-01T12:00:00Z","zzz":{"a":1}}` + "\n" +
				`{"event":"from-the-future","id":9,"at":"2026-10-01T12:00:00Z"}` + "\n" +
				`{"event":"carried","of":"from-the-future","id":8,"at":"2026-10-01T12:00:00Z"}` + "\n"},
			check: func(t *testing.T, p *pool) {
				if got := shapesOf(p); !reflect.DeepEqual(got, map[int64]messageShape{1: {Holder: beta, Status: StatusUnread}}) {
					t.Errorf("messages = %+v, want only message 1", got)
				}
				if len(p.events) != 1 || p.maxID != 9 {
					t.Errorf("events = %d and maxID = %d, want 1 event and an id counted past the unknown record", len(p.events), p.maxID)
				}
			},
		},
		{
			name:         "a torn last line with no newline is skipped with one warning and cut",
			files:        map[string]string{"2026-10-01.jsonl": kept + torn},
			wantWarn:     1,
			wantNewest:   &kept,
			wantCutBytes: len(torn),
			check:        onlyMessageOne,
		},
		{
			name:         "a torn last line that ends in a newline is skipped and cut",
			files:        map[string]string{"2026-10-01.jsonl": kept + torn + "\n"},
			wantWarn:     1,
			wantNewest:   &kept,
			wantCutBytes: len(torn) + 1,
			check:        onlyMessageOne,
		},
		{
			name:         "a torn last line followed by blank lines is cut with them",
			files:        map[string]string{"2026-10-01.jsonl": kept + torn + "\n\n  \n"},
			wantWarn:     1,
			wantNewest:   &kept,
			wantCutBytes: len(torn) + len("\n\n  \n"),
			check:        onlyMessageOne,
		},
		{
			name:         "a newest file whose only line is torn is cut to empty",
			files:        map[string]string{"2026-10-01.jsonl": torn},
			wantWarn:     1,
			wantNewest:   &empty,
			wantCutBytes: len(torn),
		},
		{
			name:         "a last line that is valid JSON but no record is skipped and cut",
			files:        map[string]string{"2026-10-01.jsonl": kept + "[1]\n"},
			wantWarn:     1,
			wantNewest:   &kept,
			wantCutBytes: len("[1]\n"),
			check:        onlyMessageOne,
		},
		{
			name:       "a complete last line missing its newline gets one and keeps its record",
			files:      map[string]string{"2026-10-01.jsonl": kept + posted(2, alpha, beta, "second")},
			wantNewest: &keptAndSecond,
			check: func(t *testing.T, p *pool) {
				if len(p.messages) != 2 {
					t.Errorf("messages = %+v, want messages 1 and 2", shapesOf(p))
				}
			},
		},
		{
			name:       "a complete last line followed by blank lines is left as it was",
			files:      map[string]string{"2026-10-01.jsonl": kept + "\n\n"},
			wantNewest: &keptAndBlanks,
			check:      onlyMessageOne,
		},
		{
			name:       "a newest file holding only blank lines is left as it was",
			files:      map[string]string{"2026-10-01.jsonl": "\n\n"},
			wantNewest: &blanks,
		},
		{
			name:    "a malformed middle line fails naming the file and line",
			files:   map[string]string{"2026-10-01.jsonl": posted(1, alpha, beta, "a") + "\nnot json\n" + posted(2, alpha, beta, "b") + "\n"},
			wantErr: []string{"2026-10-01.jsonl", "line 2", "repair or remove that line by hand"},
		},
		{
			name: "a torn last line of an older file fails, since only the newest file's tail can be a crash's",
			files: map[string]string{
				"2026-10-01.jsonl": posted(1, alpha, beta, "a") + "\n" + `{"event":"posted","id":2,"fr`,
				"2026-10-02.jsonl": posted(3, alpha, beta, "c") + "\n",
			},
			wantErr: []string{"2026-10-01.jsonl", "line 2"},
		},
		{
			name: "a sending without an outcome folds as unconfirmed",
			files: map[string]string{"2026-10-01.jsonl": strings.Join([]string{
				injectPosted(1, beta, "/clear"),
				line(record{Event: eventSending, ID: 1}),
				injectPosted(2, beta, "/compact"),
				line(record{Event: eventSending, ID: 2}),
				line(record{Event: eventInjected, ID: 2}),
				injectPosted(3, beta, "/status"),
			}, "\n") + "\n"},
			check: func(t *testing.T, p *pool) {
				got := map[int64]string{}
				for id, entry := range p.injects {
					got[id] = entry.Outcome
				}
				want := map[int64]string{1: injectUnconfirmed, 2: injectInjected, 3: injectQueued}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("inject outcomes = %v, want %v", got, want)
				}
			},
		},
		{
			name: "a command inject folds with its command, a record without the fields as a text inject, and a carried command inject keeps it",
			files: map[string]string{"2026-10-01.jsonl": strings.Join([]string{
				line(record{Event: eventPosted, Kind: kindInject, ID: 1, From: "ly:BATTEN", To: orch, Command: CommandCompact, Argument: "keep the plan", TTLSeconds: 600}),
				injectPosted(2, beta, "/clear"),
				line(record{Event: eventCarried, Of: eventPosted, Kind: kindInject, ID: 3, From: "ly:BATTEN", To: orch, Command: CommandColor, Argument: "blue", TTLSeconds: 600}),
			}, "\n") + "\n"},
			check: func(t *testing.T, p *pool) {
				got := map[int64]Command{}
				for id, entry := range p.injects {
					got[id] = entry.Command
				}
				want := map[int64]Command{
					1: {Name: CommandCompact, Argument: "keep the plan"},
					2: {},
					3: {Name: CommandColor, Argument: "blue"},
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("inject commands = %v, want %v", got, want)
				}
				if p.injects[2].Text != "/clear" {
					t.Errorf("text inject text = %q, want /clear", p.injects[2].Text)
				}
			},
		},
		{
			name: "the last aged event sets the consumed TTL",
			files: map[string]string{"2026-10-01.jsonl": strings.Join([]string{
				injectPosted(1, beta, "/clear"),
				line(record{Event: eventAged, ID: 1, ConsumedSeconds: 30}),
				line(record{Event: eventAged, ID: 1, ConsumedSeconds: 90}),
			}, "\n") + "\n"},
			check: func(t *testing.T, p *pool) {
				if got := p.injects[1].Consumed; got != 90*time.Second {
					t.Errorf("consumed TTL = %v, want 1m30s", got)
				}
			},
		},
		{
			name: "ids continue past a high-water record whose file held the largest id",
			files: map[string]string{
				"2026-10-01.jsonl": posted(5, alpha, beta, "kept") + "\n",
				"2026-10-02.jsonl": line(record{Event: eventHighWater, ID: 99}) + "\n",
			},
			check: func(t *testing.T, p *pool) {
				if first, second := p.newID(), p.newID(); first != 100 || second != 101 {
					t.Errorf("new ids = %d, %d; want 100, 101", first, second)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := logcapture.Capture(t)
			dir := filepath.Join(t.TempDir(), "journal")
			if tt.files != nil {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
			}
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}

			p, err := foldJournal(dir)
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("foldJournal() = nil error, want one naming %q", tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("foldJournal() error = %q, want it to contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("foldJournal() error: %v", err)
			}
			if got := strings.Count(buf.String(), "level=WARN"); got != tt.wantWarn {
				t.Errorf("WARN lines = %d, want %d; log:\n%s", got, tt.wantWarn, buf.String())
			}
			if tt.check != nil {
				tt.check(t, p)
			}
			if tt.wantNewest != nil {
				checkRepairedTail(t, dir, p, buf, tt.files, *tt.wantNewest, tt.wantCutBytes, line)
			}
		})
	}
}

// checkRepairedTail opens the journal over the folded pool and pins the newest day file's content and the repair's warning.
// It then appends one record, which only an empty file gets behind a high-water record, and pins that the directory folds again cleanly.
func checkRepairedTail(t *testing.T, dir string, p *pool, buf *logcapture.Buffer, files map[string]string, wantNewest string, wantCutBytes int, line func(record) string) {
	t.Helper()
	newest := ""
	for name := range files {
		newest = max(newest, name)
	}
	logBefore := len(buf.String())
	writer, err := openJournal(dir, &testClock{now: journalEpoch}, p)
	if err != nil {
		t.Fatalf("openJournal() error: %v", err)
	}
	repairLog := buf.String()[logBefore:]
	if wantCutBytes == 0 && strings.Contains(repairLog, "cutting a torn last line") {
		t.Errorf("repair log = %q, want no cut", repairLog)
	}
	if wantCutBytes > 0 && !strings.Contains(repairLog, fmt.Sprintf("bytes=%d", wantCutBytes)) {
		t.Errorf("repair log = %q, want a cut of %d bytes", repairLog, wantCutBytes)
	}
	read := func() string {
		data, err := os.ReadFile(filepath.Join(dir, newest))
		if err != nil {
			t.Fatalf("read %s: %v", newest, err)
		}
		return string(data)
	}
	if got := read(); got != wantNewest {
		t.Errorf("newest file after open = %q, want %q", got, wantNewest)
	}

	wantAfter := wantNewest
	if wantNewest == "" {
		wantAfter += line(record{Event: eventHighWater, ID: p.maxID}) + "\n"
	}
	next := record{Event: eventPosted, ID: 100, From: alpha, To: beta, Body: "after the repair"}
	if err := writer.append(next); err != nil {
		t.Fatalf("append() error: %v", err)
	}
	if err := writer.close(); err != nil {
		t.Fatalf("close() error: %v", err)
	}
	wantAfter += line(next) + "\n"
	if got := read(); got != wantAfter {
		t.Errorf("newest file after the append = %q, want %q", got, wantAfter)
	}
	refolded, err := foldJournal(dir)
	if err != nil {
		t.Fatalf("foldJournal() after the append error: %v", err)
	}
	if refolded.messages[100] == nil {
		t.Errorf("refolded messages = %+v, want the appended message 100", shapesOf(refolded))
	}
}

func TestJournal_Retention(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "journal")
	clock := &testClock{now: journalEpoch}
	state := newPool()
	writer, err := openJournal(dir, clock, state)
	if err != nil {
		t.Fatalf("openJournal: %v", err)
	}
	t.Cleanup(func() { _ = writer.close() })
	appendRecord := func(rec record) {
		t.Helper()
		if err := writer.append(rec); err != nil {
			t.Fatalf("append %s: %v", rec.Event, err)
		}
	}
	livePast := journalEpoch.AddDate(0, 0, 30)
	renewedPast := livePast.AddDate(0, 0, 1)

	// Day one: two messages (one read), two injects (one finished), and three holds (one live and renewed to a later expiry, one expiring before the carry, one released).
	appendRecord(record{Event: eventPosted, ID: 1, From: alpha, To: beta, Body: "unread", ExpectReply: true})
	appendRecord(record{Event: eventForwarded, ID: 1, By: beta, To: gamma})
	appendRecord(record{Event: eventPosted, ID: 2, From: alpha, To: beta, Body: "read"})
	appendRecord(record{Event: eventRead, ID: 2, By: beta})
	appendRecord(record{Event: eventPosted, Kind: kindInject, ID: 3, From: alpha, To: beta, Body: "/clear", TTLSeconds: 600})
	appendRecord(record{Event: eventAged, ID: 3, ConsumedSeconds: 30})
	appendRecord(record{Event: eventPosted, Kind: kindInject, ID: 4, From: alpha, To: beta, Body: "/compact", TTLSeconds: 600})
	appendRecord(record{Event: eventSending, ID: 4})
	appendRecord(record{Event: eventInjected, ID: 4})
	appendRecord(record{Event: eventHeld, To: beta, By: alpha, ExpiresAt: livePast})
	appendRecord(record{Event: eventRenewed, To: beta, By: alpha, ExpiresAt: renewedPast})
	appendRecord(record{Event: eventHeld, To: gamma, By: alpha, ExpiresAt: journalEpoch.AddDate(0, 0, 1)})
	appendRecord(record{Event: eventHeld, To: delta, By: alpha, ExpiresAt: livePast})
	appendRecord(record{Event: eventReleased, To: delta, By: alpha})
	appendRecord(record{Event: eventHeld, To: alpha, By: beta, ExpiresAt: livePast})

	// Day five is inside the retention and keeps its file.
	clock.now = journalEpoch.AddDate(0, 0, 4)
	appendRecord(record{Event: eventPosted, ID: 5, From: alpha, To: beta, Body: "mid"})
	appendRecord(record{Event: eventRenewed, To: alpha, By: beta, ExpiresAt: renewedPast})
	dayOne, err := os.ReadFile(filepath.Join(dir, "2026-10-01.jsonl"))
	if err != nil {
		t.Fatalf("read day one: %v", err)
	}

	// Day eleven puts day one past seven days: day five stays.
	clock.now = journalEpoch.AddDate(0, 0, 10)
	appendRecord(record{Event: eventPosted, ID: 6, From: "ly:BATTEN", To: beta, Body: "new"})

	names, err := listJournalFiles(dir)
	if err != nil {
		t.Fatalf("listJournalFiles: %v", err)
	}
	if want := []string{"2026-10-05.jsonl", "2026-10-11.jsonl"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("journal files = %v, want %v", names, want)
	}

	check := func(label string, p *pool) {
		t.Helper()
		wantShapes := map[int64]messageShape{
			1: {Holder: gamma, Path: []string{beta}, Status: StatusUnread},
			5: {Holder: beta, Status: StatusUnread},
			6: {Holder: beta, Status: StatusUnread},
		}
		if got := shapesOf(p); !reflect.DeepEqual(got, wantShapes) {
			t.Errorf("%s: messages = %+v, want %+v", label, got, wantShapes)
		}
		if len(p.injects) != 1 || p.injects[3] == nil || p.injects[3].Consumed != 30*time.Second || p.injects[3].Outcome != injectQueued || p.injects[3].TTL != 10*time.Minute {
			t.Errorf("%s: injects = %+v, want only inject 3 queued with its 30s consumed TTL kept", label, p.injects)
		}
		var holds []string
		for receiver := range p.holds {
			holds = append(holds, receiver)
		}
		slices.Sort(holds)
		if !reflect.DeepEqual(holds, []string{alpha, beta}) {
			t.Errorf("%s: holds on %v, want only the live holds on %s and %s", label, holds, alpha, beta)
		}
		// The renewal on day five sits inside the retention, so that hold stays in its file and is not carried.
		if hold := p.holds[alpha]; hold == nil || hold.Owner != beta || !hold.Expires.Equal(renewedPast) {
			t.Errorf("%s: hold on %s = %+v, want owner %s with the renewed expiry %v", label, alpha, hold, beta, renewedPast)
		}
		if got := p.messages[1].Posted; !got.Equal(journalEpoch) {
			t.Errorf("%s: message 1 posted at %v, want its original time", label, got)
		}
		if !p.messages[1].ExpectReply || p.messages[5].ExpectReply {
			t.Errorf("%s: ExpectReply of messages 1 and 5 = %v and %v, want the carried unread message to keep its mark and the unmarked one none", label, p.messages[1].ExpectReply, p.messages[5].ExpectReply)
		}
		if !p.programs["ly:BATTEN"] || p.maxID != 6 {
			t.Errorf("%s: programs = %v and maxID = %d, want ly:BATTEN known and 6", label, p.programs, p.maxID)
		}
		dayEleven := journalEpoch.AddDate(0, 0, 10)
		wantMail := []LogEntry{{Day: "2026-10-11", Event: eventCarried, At: dayEleven, ID: 1, From: alpha, To: gamma, Kind: kindMail, Subject: "unread", ExpectReply: true}}
		if got := carriedEntries(p, 1); !reflect.DeepEqual(got, wantMail) {
			t.Errorf("%s: carried entries of message 1 = %+v, want %+v", label, got, wantMail)
		}
		wantInject := []LogEntry{{Day: "2026-10-11", Event: eventCarried, At: dayEleven, ID: 3, From: alpha, To: beta, Kind: kindInject, Subject: "/clear"}}
		if got := carriedEntries(p, 3); !reflect.DeepEqual(got, wantInject) {
			t.Errorf("%s: carried entries of inject 3 = %+v, want %+v", label, got, wantInject)
		}
		if hold := p.holds[beta]; hold == nil || hold.Owner != alpha || !hold.Expires.Equal(renewedPast) {
			t.Errorf("%s: hold on %s = %+v, want owner %s with the renewed expiry %v", label, beta, hold, alpha, renewedPast)
		}
		wantHold := []LogEntry{{Day: "2026-10-11", Event: eventCarried, At: dayEleven, To: beta, By: alpha}}
		if got := carriedEntries(p, 0); !reflect.DeepEqual(got, wantHold) {
			t.Errorf("%s: carried entries of the live hold = %+v, want %+v", label, got, wantHold)
		}
	}
	check("the writer's pool", state)
	refolded, err := foldJournal(dir)
	if err != nil {
		t.Fatalf("foldJournal after retention: %v", err)
	}
	check("the refolded journal", refolded)

	// A crash between the carry and the deletion leaves the old file beside the carried records: the entries fold once.
	if err := os.WriteFile(filepath.Join(dir, "2026-10-01.jsonl"), dayOne, 0o644); err != nil {
		t.Fatalf("restore day one: %v", err)
	}
	crashed, err := foldJournal(dir)
	if err != nil {
		t.Fatalf("foldJournal after a crash between carry and delete: %v", err)
	}
	wantCrashed := map[int64]messageShape{
		1: {Holder: gamma, Path: []string{beta}, Status: StatusUnread},
		2: {Holder: beta, Status: StatusRead},
		5: {Holder: beta, Status: StatusUnread},
		6: {Holder: beta, Status: StatusUnread},
	}
	if got := shapesOf(crashed); !reflect.DeepEqual(got, wantCrashed) {
		t.Errorf("after a crash between carry and delete: messages = %+v, want %+v", got, wantCrashed)
	}
	if crashed.injects[3] == nil || crashed.injects[3].Consumed != 30*time.Second {
		t.Errorf("after a crash between carry and delete: inject 3 = %+v, want one entry with its consumed TTL", crashed.injects[3])
	}

	// Message 1, still unread and carried in day eleven's file, is carried again once that file is past retention.
	// The earlier carry's entry goes with its deleted file, so the carry of the day just opened is the one the log shows.
	dayNineteen := journalEpoch.AddDate(0, 0, 18)
	clock.now = dayNineteen
	appendRecord(record{Event: eventPosted, ID: 7, From: alpha, To: beta, Body: "later"})
	wantLater := []LogEntry{{Day: "2026-10-19", Event: eventCarried, At: dayNineteen, ID: 1, From: alpha, To: gamma, Kind: kindMail, Subject: "unread", ExpectReply: true}}
	if got := carriedEntries(state, 1); !reflect.DeepEqual(got, wantLater) {
		t.Errorf("the writer's pool after a second carry: carried entries of message 1 = %+v, want %+v", got, wantLater)
	}
	refoldedLater, err := foldJournal(dir)
	if err != nil {
		t.Fatalf("foldJournal after the second retention: %v", err)
	}
	if got := carriedEntries(refoldedLater, 1); !reflect.DeepEqual(got, wantLater) {
		t.Errorf("the refolded journal after a second carry: carried entries of message 1 = %+v, want %+v", got, wantLater)
	}

	t.Run("a chaseable message, an unread chase and a program's read are carried, and no carry makes CREEL a mailbox", func(t *testing.T) {
		chaseDir := filepath.Join(t.TempDir(), "journal")
		chaseClock := &testClock{now: journalEpoch}
		chasePool := newPool()
		chaseWriter, err := openJournal(chaseDir, chaseClock, chasePool)
		if err != nil {
			t.Fatalf("openJournal: %v", err)
		}
		t.Cleanup(func() { _ = chaseWriter.close() })
		for _, rec := range []record{
			{Event: eventPosted, ID: 1, From: alpha, To: beta, Body: "question", ExpectReply: true},
			{Event: eventRead, ID: 1, By: beta},
			{Event: eventChaseAged, ID: 1, ConsumedSeconds: 600},
			{Event: eventPosted, ID: 2, From: "ly:CREEL", To: beta, Body: "chase", Chase: 1, Chased: 1, ConsumedSeconds: 600},
			{Event: eventChased, ID: 1, To: beta, Chase: 1},
			{Event: eventPosted, ID: 3, From: "ly:CREEL", To: "ly:BATTEN", Body: "sender notice"},
			{Event: eventRead, By: "ly:WEBSTER"},
		} {
			if err := chaseWriter.append(rec); err != nil {
				t.Fatalf("append %s: %v", rec.Event, err)
			}
		}
		// Day eleven puts day one past seven days.
		chaseClock.now = journalEpoch.AddDate(0, 0, 10)
		if err := chaseWriter.append(record{Event: eventPosted, ID: 4, From: alpha, To: beta, Body: "new"}); err != nil {
			t.Fatalf("append: %v", err)
		}
		refolded, err := foldJournal(chaseDir)
		if err != nil {
			t.Fatalf("foldJournal after retention: %v", err)
		}
		for label, p := range map[string]*pool{"the writer's pool": chasePool, "the refolded journal": refolded} {
			msg := p.messages[1]
			if msg == nil || msg.Holder != beta || msg.Status != StatusRead || !msg.ReadAt.Equal(journalEpoch) || !isChaseable(msg) {
				t.Fatalf("%s: message 1 = %+v; want it carried read and still chaseable with beta", label, msg)
			}
			if msg.ChaseConsumed != 10*time.Minute || msg.ChaseMark != 10*time.Minute || msg.Chases != 1 || p.chased[1] != 1 {
				t.Errorf("%s: message 1 clock %v, mark %v, chases %d, highest chased %d; want 10m, 10m, 1, 1", label, msg.ChaseConsumed, msg.ChaseMark, msg.Chases, p.chased[1])
			}
			if chase := p.messages[2]; chase == nil || chase.Holder != beta || chase.Status != StatusUnread || chase.ChaseOf != 1 {
				t.Errorf("%s: chase = %+v; want it carried unread with beta, chasing message 1", label, chase)
			}
			if notice := p.messages[3]; notice == nil || notice.Holder != "ly:BATTEN" || notice.Status != StatusUnread {
				t.Errorf("%s: sender notice = %+v; want it carried unread with ly:BATTEN", label, notice)
			}
			if _, read := p.readers["ly:WEBSTER"]; !read || !p.programs["ly:WEBSTER"] {
				t.Errorf("%s: readers = %v and programs = %v; want the program whose only read is past retention still in both", label, p.readers, p.programs)
			}
			if p.programs["ly:CREEL"] {
				t.Errorf("%s: programs = %v; want no ly:CREEL, the condition a post to it is refused as an unknown receiver on", label, p.programs)
			}
		}
	})

	// Retention carries a pending outbound message and an inbound message, and no settled outbound message, and a carried line beside its original folds to one entry.
	t.Run("mail across hubs", func(t *testing.T) {
		t.Parallel()
		dir := filepath.Join(t.TempDir(), "journal")
		clock := &testClock{now: journalEpoch}
		state := newPool()
		writer, err := openJournal(dir, clock, state)
		if err != nil {
			t.Fatalf("openJournal: %v", err)
		}
		t.Cleanup(func() { _ = writer.close() })
		postedAt := journalEpoch.Add(123456789 * time.Nanosecond)
		remotePosted := journalEpoch.Add(-time.Hour)
		for _, rec := range []record{
			{Event: eventOutbound, At: postedAt, ID: 1, From: alpha, To: remoteOrch, ReceivingHub: remoteHub, Kind: kindReply, Body: "pending", ReplyTo: 9, Answers: 8, ExpectReply: true},
			{Event: eventOutbound, ID: 2, From: alpha, To: remoteOrch, ReceivingHub: remoteHub, Body: "delivered"},
			{Event: eventRemoteDelivered, ID: 2, RemoteID: 40},
			{Event: eventOutbound, ID: 3, From: alpha, To: remoteOrch, ReceivingHub: remoteHub, Body: "returned"},
			{Event: eventOutboundReturned, ID: 3, Reason: "no-hub"},
			{Event: eventInbound, ID: 4, From: remoteOrch, To: beta, Body: "unread", SendingHub: remoteHub, SendingID: 12, Trusted: true, RemotePosted: remotePosted, DeliveredTo: beta},
			{Event: eventInbound, ID: 5, From: remoteOrch, To: beta, Body: "read", SendingHub: remoteHub, SendingID: 13, RemotePosted: remotePosted, DeliveredTo: beta, ExpectReply: true},
			{Event: eventRead, ID: 5, By: beta},
			{Event: eventChaseAged, ID: 5, ConsumedSeconds: 600},
		} {
			if err := writer.append(rec); err != nil {
				t.Fatalf("append %s: %v", rec.Event, err)
			}
		}
		dayOne, err := os.ReadFile(filepath.Join(dir, "2026-10-01.jsonl"))
		if err != nil {
			t.Fatalf("read day one: %v", err)
		}
		// Day eleven puts day one past seven days.
		clock.now = journalEpoch.AddDate(0, 0, 10)
		if err := writer.append(record{Event: eventPosted, ID: 6, From: alpha, To: beta, Body: "new"}); err != nil {
			t.Fatalf("append: %v", err)
		}

		check := func(label string, p *pool, wantOutbound int) {
			t.Helper()
			pending := p.outbound[1]
			if len(p.outbound) != wantOutbound || pending == nil || pending.State != outboundPending || !pending.Posted.Equal(postedAt) || pending.ReceivingHub != remoteHub ||
				pending.ReplyTo != 9 || pending.Answers != 8 || pending.Kind != kindReply || !pending.ExpectReply || pending.To != remoteOrch {
				t.Errorf("%s: outbound = %+v, want %d entries with 1 pending, posted at %v, for %s, replying to 9 and answering 8", label, p.outbound, wantOutbound, postedAt, remoteHub)
			}
			unread, read := p.messages[4], p.messages[5]
			if unread == nil || unread.Status != StatusUnread || unread.SendingHub != remoteHub || unread.SendingID != 12 || !unread.Trusted || !unread.RemotePosted.Equal(remotePosted) || unread.DeliveredTo != beta || !unread.Posted.Equal(journalEpoch) {
				t.Errorf("%s: unread inbound message = %+v, want it back with its hub fields and its original time", label, unread)
			}
			if read == nil || read.Status != StatusRead || !read.ReadAt.Equal(journalEpoch) || read.ChaseConsumed != 10*time.Minute || read.SendingID != 13 || read.Trusted || !isChaseable(read) {
				t.Errorf("%s: read inbound message = %+v, want it back read, chaseable, with its read time, chase clock and hub fields", label, read)
			}
			if len(p.messages) != 3 {
				t.Errorf("%s: messages = %+v, want only 4, 5 and 6", label, shapesOf(p))
			}
		}
		check("the writer's pool", state, 1)
		if got := carriedEntries(state, 4); len(got) != 1 || got[0].SendingHub != remoteHub || got[0].SendingID != 12 || got[0].Kind != kindMail {
			t.Errorf("carried entries of inbound message 4 = %+v, want one showing its sending hub and id", got)
		}
		if got := carriedEntries(state, 1); len(got) != 1 || got[0].ReceivingHub != remoteHub || got[0].Kind != kindReply || got[0].Answers != 8 {
			t.Errorf("carried entries of outbound message 1 = %+v, want one showing its receiving hub and the id it answers", got)
		}
		refolded, err := foldJournal(dir)
		if err != nil {
			t.Fatalf("foldJournal after retention: %v", err)
		}
		check("the refolded journal", refolded, 1)

		// A crash between the carry and the deletion leaves the old file beside the carried lines: each entry folds once, and the settled outbound messages return with it.
		if err := os.WriteFile(filepath.Join(dir, "2026-10-01.jsonl"), dayOne, 0o644); err != nil {
			t.Fatalf("restore day one: %v", err)
		}
		crashed, err := foldJournal(dir)
		if err != nil {
			t.Fatalf("foldJournal after a crash between carry and delete: %v", err)
		}
		check("after a crash between carry and delete", crashed, 3)
	})
}

// carriedEntries returns the carried entries of the log for id, oldest first; id 0 selects the entries of holds.
func carriedEntries(p *pool, id int64) []LogEntry {
	var carried []LogEntry
	for _, entry := range p.events {
		if entry.Event == eventCarried && entry.ID == id {
			carried = append(carried, entry)
		}
	}
	return carried
}
