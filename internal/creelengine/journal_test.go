// journal_test.go pins how the journal folds into the pool and how retention carries live state out of old day files.

package creelengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
)

const (
	alpha = "ly:alpha:driver"
	beta  = "ly:beta:driver"
	gamma = "ly:gamma:driver"
	delta = "ly:delta:driver"
	orch  = "ly:orch"
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
	line := func(rec record) string { return recordLine(t, rec) }
	posted := func(id int64, from, to, body string) string {
		return line(record{Event: eventPosted, ID: id, From: from, To: to, Body: body})
	}
	injectPosted := func(id int64, to, text string) string {
		return line(record{Event: eventPosted, Kind: kindInject, ID: id, From: alpha, To: to, Body: text, TTLSeconds: 600})
	}
	tests := []struct {
		name string
		// files maps a day file name to its raw content; a nil map leaves the journal directory missing.
		files    map[string]string
		wantErr  []string
		wantWarn int
		check    func(t *testing.T, p *pool)
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
				if got := p.withheld[orch].Notice; got != "creel-notices" {
					t.Errorf("withheld notice for the orch = %q, want creel-notices", got)
				}
			},
		},
		{
			name: "an unknown key and an unknown event name are ignored",
			files: map[string]string{"2026-10-01.jsonl": `{"event":"posted","id":1,"from":"` + alpha + `","to":"` + beta + `","body":"hi","at":"2026-10-01T12:00:00Z","zzz":{"a":1}}` + "\n" +
				`{"event":"from-the-future","id":9,"at":"2026-10-01T12:00:00Z"}` + "\n"},
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
			name:     "a torn last line is skipped with one warning",
			files:    map[string]string{"2026-10-01.jsonl": posted(1, alpha, beta, "kept") + "\n" + `{"event":"posted","id":2,"fr`},
			wantWarn: 1,
			check: func(t *testing.T, p *pool) {
				if len(p.messages) != 1 || p.messages[1] == nil {
					t.Errorf("messages = %+v, want only message 1", shapesOf(p))
				}
			},
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
			tt.check(t, p)
		})
	}
}

func TestJournal_Retention(t *testing.T) {
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

	// Day one: two messages (one read), two injects (one finished), and three holds (one live, one expiring before the carry, one released).
	appendRecord(record{Event: eventPosted, ID: 1, From: alpha, To: beta, Body: "unread"})
	appendRecord(record{Event: eventForwarded, ID: 1, By: beta, To: gamma})
	appendRecord(record{Event: eventPosted, ID: 2, From: alpha, To: beta, Body: "read"})
	appendRecord(record{Event: eventRead, ID: 2, By: beta})
	appendRecord(record{Event: eventPosted, Kind: kindInject, ID: 3, From: alpha, To: beta, Body: "/clear", TTLSeconds: 600})
	appendRecord(record{Event: eventAged, ID: 3, ConsumedSeconds: 30})
	appendRecord(record{Event: eventPosted, Kind: kindInject, ID: 4, From: alpha, To: beta, Body: "/compact", TTLSeconds: 600})
	appendRecord(record{Event: eventSending, ID: 4})
	appendRecord(record{Event: eventInjected, ID: 4})
	appendRecord(record{Event: eventHeld, To: beta, By: alpha, ExpiresAt: livePast})
	appendRecord(record{Event: eventHeld, To: gamma, By: alpha, ExpiresAt: journalEpoch.AddDate(0, 0, 1)})
	appendRecord(record{Event: eventHeld, To: delta, By: alpha, ExpiresAt: livePast})
	appendRecord(record{Event: eventReleased, To: delta, By: alpha})

	// Day five is inside the retention and keeps its file.
	clock.now = journalEpoch.AddDate(0, 0, 4)
	appendRecord(record{Event: eventPosted, ID: 5, From: alpha, To: beta, Body: "mid"})
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
		if !reflect.DeepEqual(holds, []string{beta}) {
			t.Errorf("%s: holds on %v, want only the live hold on %s", label, holds, beta)
		}
		if got := p.messages[1].Posted; !got.Equal(journalEpoch) {
			t.Errorf("%s: message 1 posted at %v, want its original time", label, got)
		}
		if !p.programs["ly:BATTEN"] || p.maxID != 6 {
			t.Errorf("%s: programs = %v and maxID = %d, want ly:BATTEN known and 6", label, p.programs, p.maxID)
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
}
