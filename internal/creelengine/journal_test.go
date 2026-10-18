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
	appendRecord(record{Event: eventRenewed, To: beta, By: alpha, ExpiresAt: renewedPast})
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
		dayEleven := journalEpoch.AddDate(0, 0, 10)
		wantMail := []LogEntry{{Day: "2026-10-11", Event: eventCarried, At: dayEleven, ID: 1, From: alpha, To: gamma, Kind: kindMail, Subject: "unread"}}
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
	wantLater := []LogEntry{{Day: "2026-10-19", Event: eventCarried, At: dayNineteen, ID: 1, From: alpha, To: gamma, Kind: kindMail, Subject: "unread"}}
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
