// sessionusage_test.go covers the session usage writer against a fake transcript reader and a fake pricer.

package shedadapters

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeTranscripts is a TranscriptReader over one located session, with scripted failures.
type fakeTranscripts struct {
	located   []shuttleengine.LocatedSession
	locateErr error
	session   shuttleengine.SessionRead
	readErr   error
}

func (f fakeTranscripts) LocateSessions(string) ([]shuttleengine.LocatedSession, error) {
	return f.located, f.locateErr
}

func (f fakeTranscripts) ReadSession(shuttleengine.LocatedSession) (shuttleengine.SessionRead, error) {
	return f.session, f.readErr
}

// fakePricer prices a message at a per-token rate by model, and has no price for a model not in the map.
type fakePricer map[string]float64

func (p fakePricer) PriceMessage(model string, usage shuttleengine.MessageUsage) (float64, bool) {
	rate, ok := p[model]
	return float64(usage.Input+usage.CacheWrite+usage.CacheRead+usage.Output) * rate, ok
}

// assistantMessage returns an assistant entry of model carrying the given counts.
func assistantMessage(model string, input, cacheWrite, cacheRead, output int) shuttleengine.TranscriptEntry {
	return shuttleengine.TranscriptEntry{
		Role: shuttleengine.EntryAssistant, Model: model,
		Usage: &shuttleengine.MessageUsage{Input: input, CacheWrite: cacheWrite, CacheRead: cacheRead, Output: output},
	}
}

// TestRecordSessionUsage pins the writer: counts summed per model and priced per message, the models list only for several models, the fallback to the counts shuttle read when a transcript cannot be had, and a write failure that is logged and stops nothing.
// It runs serially because it captures the process-global logger output.
func TestRecordSessionUsage(t *testing.T) {
	tiergate.Require(t, 0)

	int64Ptr := func(n int64) *int64 { return &n }
	floatPtr := func(f float64) *float64 { return &f }
	started := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	end := shuttleengine.SessionEnd{
		SessionID: "sess-1", Engine: "claude", Role: "implementer", Model: "opus", Worktree: "/wt",
		StartedAt: started, EndedAt: started.Add(5 * time.Minute), Outcome: shuttleengine.OutcomeDone,
		Usage: shuttleengine.SessionUsage{Known: true, SplitKnown: true, Input: 7, CacheWrite: 1, CacheRead: 70, Output: 3},
	}
	located := []shuttleengine.LocatedSession{{TranscriptRef: shuttleengine.TranscriptRef{ID: "other.jsonl"}}, {TranscriptRef: shuttleengine.TranscriptRef{ID: "sess-1.jsonl"}}}
	prices := fakePricer{"m1": 1, "m2": 2}
	base := shedrun.SessionUsageRecord{
		SessionID: "sess-1", Row: "Implement", Role: "implementer", Model: "opus", Engine: "claude",
		Started: "2026-11-05T09:00:00Z", Ended: "2026-11-05T09:05:00Z",
	}
	shuttleReading := func(r shedrun.SessionUsageRecord) shedrun.SessionUsageRecord {
		r.InputTokens, r.CacheWriteTokens, r.CacheReadTokens, r.OutputTokens = int64Ptr(7), int64Ptr(1), int64Ptr(70), int64Ptr(3)
		return r
	}
	mainOnly := shuttleengine.SessionRead{Main: shuttleengine.TranscriptRead{Entries: []shuttleengine.TranscriptEntry{
		{Role: shuttleengine.EntryUser},
		assistantMessage("m1", 10, 0, 100, 5),
		assistantMessage("m1", 20, 0, 200, 7),
	}}}
	withFork := shuttleengine.SessionRead{
		Main:  shuttleengine.TranscriptRead{Entries: []shuttleengine.TranscriptEntry{assistantMessage("m1", 10, 0, 100, 5)}},
		Forks: []shuttleengine.TranscriptRead{{Entries: []shuttleengine.TranscriptEntry{assistantMessage("m2", 4, 0, 40, 1)}}},
	}
	unpriced := shuttleengine.SessionRead{Main: shuttleengine.TranscriptRead{Entries: []shuttleengine.TranscriptEntry{assistantMessage("m3", 10, 0, 100, 5)}}}

	priced := base
	priced.Model = "m1"
	priced.InputTokens, priced.CacheWriteTokens, priced.CacheReadTokens, priced.OutputTokens = int64Ptr(30), int64Ptr(0), int64Ptr(300), int64Ptr(12)
	priced.CostUSD = floatPtr(342)

	forked := base
	forked.Model = "m1"
	forked.InputTokens, forked.CacheWriteTokens, forked.CacheReadTokens, forked.OutputTokens = int64Ptr(14), int64Ptr(0), int64Ptr(140), int64Ptr(6)
	forked.CostUSD = floatPtr(205)
	forked.Models = []shedrun.ModelUsage{
		{Model: "m1", InputTokens: 10, CacheReadTokens: 100, OutputTokens: 5, CostUSD: floatPtr(115)},
		{Model: "m2", InputTokens: 4, CacheReadTokens: 40, OutputTokens: 1, CostUSD: floatPtr(90)},
	}

	costUnknown := base
	costUnknown.Model = "m3"
	costUnknown.InputTokens, costUnknown.CacheWriteTokens, costUnknown.CacheReadTokens, costUnknown.OutputTokens = int64Ptr(10), int64Ptr(0), int64Ptr(100), int64Ptr(5)

	tests := []struct {
		name     string
		reads    shuttleengine.RecordReads
		end      shuttleengine.SessionEnd
		want     shedrun.SessionUsageRecord
		wantWarn string
	}{
		{name: "OneModelPricedPerMessage", reads: shuttleengine.RecordReads{Transcripts: fakeTranscripts{located: located, session: mainOnly}, Pricer: prices}, end: end, want: priced},
		{name: "ForkOnAnotherModelPricedAtItsOwn", reads: shuttleengine.RecordReads{Transcripts: fakeTranscripts{located: located, session: withFork}, Pricer: prices}, end: end, want: forked},
		{name: "UnpricedModelLeavesCostUnknown", reads: shuttleengine.RecordReads{Transcripts: fakeTranscripts{located: located, session: unpriced}, Pricer: prices}, end: end, want: costUnknown},
		{name: "NoPricerLeavesCostUnknown", reads: shuttleengine.RecordReads{Transcripts: fakeTranscripts{located: located, session: unpriced}}, end: end, want: costUnknown},
		{name: "NoTranscriptReaderTakesShuttleCounts", end: end, want: shuttleReading(base)},
		{name: "NoTranscriptReaderAndUnsplitUsageLeavesCountsUnknown", end: shuttleengine.SessionEnd{SessionID: "sess-1", Engine: "claude", Role: "implementer", Model: "opus", StartedAt: started, EndedAt: started.Add(5 * time.Minute), Usage: shuttleengine.SessionUsage{Known: true}}, want: base},
		{name: "LocateFailureTakesShuttleCounts", reads: shuttleengine.RecordReads{Transcripts: fakeTranscripts{locateErr: errors.New("projects root unreadable")}, Pricer: prices}, end: end, want: shuttleReading(base), wantWarn: "locate the session transcripts failed"},
		{name: "ForkReadFailureTakesShuttleCounts", reads: shuttleengine.RecordReads{Transcripts: fakeTranscripts{located: located, session: withFork, readErr: errors.New("fork vanished")}, Pricer: prices}, end: end, want: shuttleReading(base), wantWarn: "read the session transcripts failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := logcapture.Capture(t)
			dir := filepath.Join(t.TempDir(), lymdirs.LymDirName, lymdirs.UsageDirName)

			RecordSessionUsage(dir, "Implement", tt.end, tt.reads)

			content, err := os.ReadFile(filepath.Join(dir, "session-sess-1.json"))
			if err != nil {
				t.Fatalf("read the session record: %v", err)
			}
			usage, err := shedrun.ParseRunUsage(map[string][]byte{"session-sess-1.json": content})
			if err != nil {
				t.Fatalf("ParseRunUsage() error: %v", err)
			}
			if len(usage.Sessions) != 1 || !reflect.DeepEqual(usage.Sessions[0], tt.want) {
				t.Errorf("session record = %+v; want %+v", usage.Sessions, tt.want)
			}
			if tt.wantWarn == "" && strings.Contains(logs.String(), "WARN") {
				t.Errorf("log = %q; want no warning", logs.String())
			}
			if !strings.Contains(logs.String(), tt.wantWarn) {
				t.Errorf("log = %q; want it to hold %q", logs.String(), tt.wantWarn)
			}
		})
	}

	t.Run("UnwritableDirectoryWarnsAndStopsNothing", func(t *testing.T) {
		logs := logcapture.Capture(t)
		blocker := filepath.Join(t.TempDir(), "file")
		if err := os.WriteFile(blocker, nil, 0o644); err != nil {
			t.Fatalf("write blocker: %v", err)
		}

		RecordSessionUsage(filepath.Join(blocker, lymdirs.UsageDirName), "Implement", end, shuttleengine.RecordReads{})

		if !strings.Contains(logs.String(), "write the session usage record failed") {
			t.Errorf("log = %q; want the write failure logged", logs.String())
		}
	})
}

// TestUsageRecorder pins the session-end hook of a run: a session end writes the record under the status file's current row, an absent status writes it under an empty row, and an engine with no readers entry is recorded unpriced.
func TestUsageRecorder(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	started := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	located := []shuttleengine.LocatedSession{{TranscriptRef: shuttleengine.TranscriptRef{ID: "sess-1.jsonl"}}}
	session := shuttleengine.SessionRead{Main: shuttleengine.TranscriptRead{Entries: []shuttleengine.TranscriptEntry{assistantMessage("m1", 10, 0, 100, 5)}}}
	readers := map[string]shuttleengine.RecordReads{
		"claude": {Transcripts: fakeTranscripts{located: located, session: session}, Pricer: fakePricer{"m1": 1}},
	}
	tests := []struct {
		name       string
		seedStatus bool
		engine     string
		wantRow    string
		wantCost   *float64
	}{
		{name: "StatusRowNamesTheRecord", seedStatus: true, engine: "claude", wantRow: "Implement", wantCost: floatPointer(115)},
		{name: "AbsentStatusWritesAnEmptyRow", engine: "claude", wantRow: "", wantCost: floatPointer(115)},
		{name: "EngineWithoutReadersIsUnpriced", seedStatus: true, engine: "codex", wantRow: "Implement"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			location := &lymcwd.Location{HubPath: t.TempDir(), WorktreeName: "pair", AnchorRel: "."}
			if tt.seedStatus {
				status := shedengine.Status{CurrentProducer: "Implement"}
				if err := state.WriteJSON(shedrun.StatusFile(location, shedrun.SelfRunID), shedrun.StatusLock(location, shedrun.SelfRunID), status); err != nil {
					t.Fatalf("seed the status file: %v", err)
				}
			}
			end := shuttleengine.SessionEnd{
				SessionID: "sess-1", Engine: tt.engine, Role: "implementer", Model: "m1", Worktree: "/wt",
				StartedAt: started, EndedAt: started.Add(time.Minute), Outcome: shuttleengine.OutcomeDone,
				Usage: shuttleengine.SessionUsage{Known: true, SplitKnown: true, Input: 10, CacheRead: 100, Output: 5},
			}

			UsageRecorder(location, readers)(end)

			content, err := os.ReadFile(filepath.Join(shedrun.UsageDir(location), "session-sess-1.json"))
			if err != nil {
				t.Fatalf("read the session record: %v", err)
			}
			usage, err := shedrun.ParseRunUsage(map[string][]byte{"session-sess-1.json": content})
			if err != nil || len(usage.Sessions) != 1 {
				t.Fatalf("ParseRunUsage() = %+v, %v; want one session", usage, err)
			}
			got := usage.Sessions[0]
			if got.Row != tt.wantRow {
				t.Errorf("record row = %q; want %q", got.Row, tt.wantRow)
			}
			if !reflect.DeepEqual(got.CostUSD, tt.wantCost) {
				t.Errorf("record cost = %v; want %v", got.CostUSD, tt.wantCost)
			}
		})
	}
}

// floatPointer returns a pointer to f.
func floatPointer(f float64) *float64 { return &f }
