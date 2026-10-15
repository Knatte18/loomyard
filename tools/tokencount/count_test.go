package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

// fakeEngine is the name of the engine oneEngine and fakeReadSet register.
const fakeEngine = "fake"

// fakeRates are the fake engine's USD per million tokens of every kind, by model.
var fakeRates = fakePricer{"model-a": 20, "model-b": 10}

// fakeTranscripts is an in-memory transcript reader: located sessions keyed by worktree and session reads keyed by session Ref.
// A locateErr fails every locate, and a readErrs entry fails the read of its Ref.
type fakeTranscripts struct {
	located   map[string][]shuttleengine.LocatedSession
	reads     map[string]shuttleengine.SessionRead
	locateErr error
	readErrs  map[string]error
}

func newFakeTranscripts() fakeTranscripts {
	return fakeTranscripts{
		located:  map[string][]shuttleengine.LocatedSession{},
		reads:    map[string]shuttleengine.SessionRead{},
		readErrs: map[string]error{},
	}
}

// add locates read under worktree with the given modification time; its Ref is the worktree and the main read's ID.
func (f fakeTranscripts) add(worktree string, modified time.Time, read shuttleengine.SessionRead) {
	located := shuttleengine.LocatedSession{
		TranscriptRef: shuttleengine.TranscriptRef{ID: read.Main.ID, Ref: worktree + "/" + read.Main.ID},
		Modified:      modified,
	}
	for _, fork := range read.Forks {
		located.Forks = append(located.Forks, shuttleengine.TranscriptRef{ID: fork.ID, Ref: located.Ref + "/" + fork.ID})
	}
	f.located[worktree] = append(f.located[worktree], located)
	f.reads[located.Ref] = read
}

func (f fakeTranscripts) LocateSessions(worktree string) ([]shuttleengine.LocatedSession, error) {
	if f.locateErr != nil {
		return nil, f.locateErr
	}
	return slices.Clone(f.located[worktree]), nil
}

func (f fakeTranscripts) ReadSession(s shuttleengine.LocatedSession) (shuttleengine.SessionRead, error) {
	if err := f.readErrs[s.Ref]; err != nil {
		return shuttleengine.SessionRead{}, err
	}
	read, ok := f.reads[s.Ref]
	if !ok {
		return shuttleengine.SessionRead{}, fs.ErrNotExist
	}
	return read, nil
}

// fakePricer prices a message at its model's USD per million tokens of every kind, and has no price for another model.
type fakePricer map[string]float64

func (p fakePricer) PriceMessage(model string, u shuttleengine.MessageUsage) (float64, bool) {
	rate, ok := p[model]
	if !ok {
		return 0, false
	}
	return rate * float64(u.Input+u.CacheWrite+u.CacheRead+u.Output) / 1e6, true
}

// fakeInstructions is an in-memory instruction file lister.
type fakeInstructions []string

func (f fakeInstructions) InstructionFiles() []string { return f }

// fakeInstructionFiles are the instruction file names the fake engine lists, the second of which no base commit holds.
var fakeInstructionFiles = fakeInstructions{"INSTRUCTIONS.md", "LOCAL.md"}

// fakeReadSet is a read set of one engine named fakeEngine, reading through fake, pricing at fakeRates and listing fakeInstructionFiles.
func fakeReadSet(fake fakeTranscripts) ReadSet {
	return ReadSet{fakeEngine: {Transcripts: fake, Pricer: fakeRates, Instructions: fakeInstructionFiles}}
}

// oneEngine is a one-engine read set whose engine located every given session under worktree.
func oneEngine(worktree string, sessions ...shuttleengine.SessionRead) ReadSet {
	fake := newFakeTranscripts()
	for _, session := range sessions {
		fake.add(worktree, time.Time{}, session)
	}
	return fakeReadSet(fake)
}

// stampOf parses an RFC 3339 test timestamp.
func stampOf(ts string) time.Time {
	at, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		panic(err)
	}
	return at
}

// readOf is a transcript read named name whose Started is its first timestamped entry.
func readOf(id, name string, entries ...shuttleengine.TranscriptEntry) shuttleengine.TranscriptRead {
	read := shuttleengine.TranscriptRead{ID: id, Name: name, Entries: entries}
	for _, e := range entries {
		if !e.At.IsZero() {
			read.Started = e.At
			break
		}
	}
	return read
}

// sessionOf is a session read of a main transcript and its forks.
func sessionOf(main shuttleengine.TranscriptRead, forks ...shuttleengine.TranscriptRead) shuttleengine.SessionRead {
	return shuttleengine.SessionRead{Main: main, Forks: forks}
}

// assistant is an assistant entry of message id with one input token, out output tokens and read cache-read tokens.
func assistant(id, model string, out, read int) shuttleengine.TranscriptEntry {
	return shuttleengine.TranscriptEntry{
		Role: shuttleengine.EntryAssistant, MessageKey: id, Model: model,
		Usage: &shuttleengine.MessageUsage{Input: 1, Output: out, CacheRead: read},
	}
}

// toolResult is a user entry at ts holding one tool result with the given text.
func toolResult(ts, text string) shuttleengine.TranscriptEntry {
	return toolResultFor("t", ts, text)
}

// toolResultFor is toolResult answering the tool call with the given id.
func toolResultFor(useID, ts, text string) shuttleengine.TranscriptEntry {
	return shuttleengine.TranscriptEntry{
		At: stampOf(ts), Role: shuttleengine.EntryUser,
		ToolResults: []shuttleengine.ToolResult{{CallID: useID, Text: text}},
	}
}

// bashUse is an assistant entry holding one shell call of command.
func bashUse(id, command string) shuttleengine.TranscriptEntry {
	return shuttleengine.TranscriptEntry{
		Role: shuttleengine.EntryAssistant, MessageKey: "use-" + id,
		ToolCalls: []shuttleengine.ToolCall{{ID: id, Kind: shuttleengine.ToolShell, Command: command}},
	}
}

// stamped is an assistant entry at ts, ending its turn when endsTurn is set.
func stamped(ts string, endsTurn bool) shuttleengine.TranscriptEntry {
	return shuttleengine.TranscriptEntry{
		At: stampOf(ts), Role: shuttleengine.EntryAssistant, MessageKey: ts, EndsTurn: endsTurn,
		Usage: &shuttleengine.MessageUsage{Input: 1, Output: 1},
	}
}

// userAt is a user entry at ts holding text, which starts a turn.
func userAt(ts, text string) shuttleengine.TranscriptEntry {
	return shuttleengine.TranscriptEntry{At: stampOf(ts), Role: shuttleengine.EntryUser, StartsTurn: true, UserTexts: []string{text}}
}

func TestRunTimeByRole(t *testing.T) {
	t.Parallel()
	build := func(t *testing.T, sessions ...shuttleengine.SessionRead) RunTally {
		t.Helper()
		run, err := CountRun(oneEngine("/hub/r", sessions...), "/hub/r", "r", time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		return run
	}
	burler := func(id string, entries ...shuttleengine.TranscriptEntry) shuttleengine.TranscriptRead {
		return readOf(id, "ly:r:burler", entries...)
	}
	// ending is a user entry that ends its turn, such as an interrupt or a local command's output.
	ending := func(ts, text string) shuttleengine.TranscriptEntry {
		e := userAt(ts, text)
		e.StartsTurn, e.EndsTurn = false, true
		return e
	}

	// burler: two turns of 1m and 2m with a 10m gap, then an interrupted turn of 1m followed by 5m of idle and a typed local command.
	// tool: a tool call turn whose result arrives 3m later counts the gap.
	// An overlapping second burler session adds only its uncovered tail, and a fork counts under burler+sub.
	run := build(t,
		sessionOf(
			burler("a.jsonl",
				userAt("2026-01-01T00:00:00Z", "go"),
				stamped("2026-01-01T00:01:00Z", true),
				userAt("2026-01-01T00:11:00Z", "again"),
				stamped("2026-01-01T00:13:00Z", true),
				userAt("2026-01-01T00:14:00Z", "third"),
				ending("2026-01-01T00:15:00Z", "interrupted"),
				ending("2026-01-01T00:20:00Z", "local command output"),
				userAt("2026-01-01T00:30:00Z", "late"),
				stamped("2026-01-01T00:31:00Z", false),
				toolResult("2026-01-01T00:34:00Z", "done"),
				stamped("2026-01-01T00:35:00Z", true),
			),
			readOf("s.jsonl", "",
				userAt("2026-01-01T00:40:00Z", "sub"),
				stamped("2026-01-01T00:42:00Z", true),
			),
		),
		sessionOf(burler("b.jsonl",
			userAt("2026-01-01T00:32:00Z", "overlap"),
			stamped("2026-01-01T00:37:00Z", true),
		)),
	)
	min := time.Minute
	// burler turns: 0-1, 11-13, 14-15, 30-35 and 32-37 unioned to 30-37.
	if got, want := run.RoleTime("burler"), (1+2+1+7)*min; got != want {
		t.Errorf("RoleTime(burler) = %v; want %v", got, want)
	}
	if got, want := run.RoleTime("burler+sub"), 2*min; got != want {
		t.Errorf("RoleTime(burler+sub) = %v; want %v", got, want)
	}
	if got := run.RoleTime("absent"); got != 0 {
		t.Errorf("RoleTime(absent) = %v; want 0", got)
	}
	// The span is 0..42m and the union of working time is 11m + 2m.
	if got, want := run.Idle(), 42*min-13*min; got != want {
		t.Errorf("Idle() = %v; want %v", got, want)
	}

	var buf bytes.Buffer
	orch := OrchTally{Charges: map[string]*OrchCharge{"r": {Weight: 5e6}}}
	if err := (Report{Runs: []RunTally{run}, Orch: &orch}).WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| orch | | | | | | 5.0M | ",
		"| role | sessions | output | cache write | cache read | input | weight | share | time | time share | est. cost | models (messages) |",
		"| idle | | | | | | | | 29m0s | 69.0% | | |",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, buf.String())
		}
	}

	// An interval into a turn-start line, or longer than the stall cap, is idle whole; the rest of a session still counts.
	const done = "done"
	for _, c := range []struct {
		name    string
		entries []shuttleengine.TranscriptEntry
		work    time.Duration
		idle    time.Duration
	}{
		{"prompt an hour after a tool call", []shuttleengine.TranscriptEntry{stamped("2026-01-01T00:00:00Z", false), userAt("2026-01-01T01:00:00Z", "p"), stamped("2026-01-01T01:01:00Z", true)}, min, 60 * min},
		{"prompt an hour after a tool result", []shuttleengine.TranscriptEntry{toolResult("2026-01-01T00:00:00Z", done), userAt("2026-01-01T01:00:00Z", "p"), stamped("2026-01-01T01:01:00Z", true)}, min, 60 * min},
		{"tool result 32 hours after its tool call", []shuttleengine.TranscriptEntry{stamped("2026-01-01T00:00:00Z", false), toolResult("2026-01-02T08:00:00Z", done), stamped("2026-01-02T08:01:00Z", true)}, min, 32 * 60 * min},
		{"assistant entry an hour after a tool result", []shuttleengine.TranscriptEntry{toolResult("2026-01-01T00:00:00Z", done), stamped("2026-01-01T01:00:00Z", true)}, 0, 60 * min},
		{"tool result 20 minutes after its tool call", []shuttleengine.TranscriptEntry{stamped("2026-01-01T00:00:00Z", false), toolResult("2026-01-01T00:20:00Z", done), stamped("2026-01-01T00:21:00Z", true)}, 21 * min, 0},
		{"user text a second after a tool result", []shuttleengine.TranscriptEntry{toolResult("2026-01-01T00:00:00Z", done), userAt("2026-01-01T00:00:01Z", "queued"), stamped("2026-01-01T00:00:30Z", true)}, 29 * time.Second, time.Second},
	} {
		got := build(t, sessionOf(burler("a.jsonl", c.entries...)))
		if w := got.RoleTime("burler"); w != c.work {
			t.Errorf("%s: RoleTime(burler) = %v; want %v", c.name, w, c.work)
		}
		if i := got.Idle(); i != c.idle {
			t.Errorf("%s: Idle() = %v; want %v", c.name, i, c.idle)
		}
	}

	// Two runs whose working intervals overlap in wall-clock time sum per role and in idle, and share the summed span.
	one := build(t, sessionOf(burler("a.jsonl", userAt("2026-01-01T00:00:00Z", "x"), stamped("2026-01-01T00:02:00Z", true), userAt("2026-01-01T00:04:00Z", "y"))))
	two := build(t, sessionOf(burler("a.jsonl", userAt("2026-01-01T00:01:00Z", "x"), stamped("2026-01-01T00:03:00Z", true), userAt("2026-01-01T00:05:00Z", "y"))))
	buf.Reset()
	if err := (Report{Runs: []RunTally{one, two}}).WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	all, _, _ := strings.Cut(buf.String(), "## r\n")
	for _, want := range []string{
		"| burler | 2 | 2 | 0 | 0 | 2 | 0.0M | 100.0% | 4m0s | 50.0% |",
		"| idle | | | | | | | | 4m0s | 50.0% | | |",
		"Time is turn time:",
		"An interval into a line that starts a turn, or longer than the stall cap of 30m0s, is idle.",
		"The union of working time plus idle is the span.",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("All runs section lacks %q:\n%s", want, all)
		}
	}
}

func TestCountRun(t *testing.T) {
	t.Parallel()
	// The main run is the Claude reader's own reading of its tokencount-run transcripts, served by the fake.
	const worktree = "/hub/my-task"
	claude := newFakeTranscripts()
	for _, read := range shuttlefake.TokencountRunReads() {
		claude.add(worktree, time.Time{}, read)
	}

	run, err := CountRun(fakeReadSet(claude), worktree, "my-task", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Usage{
		"burler":        {Input: 1, Output: 2, CacheRead: 3},
		"burler-review": {Input: 2, Output: 3},
		"webster":       {Input: 1, Output: 1, CacheRead: 1},
		"webster+sub":   {Input: 4, Output: 10, CacheRead: 18},
		"untitled":      {Input: 1, Output: 1},
	}
	if len(run.Roles) != len(want) {
		t.Fatalf("roles = %v, want %d roles", run.Roles, len(want))
	}
	for role, u := range want {
		got := run.Roles[role]
		if got == nil || got.Usage != u {
			t.Errorf("%s usage = %+v, want %+v", role, got, u)
		}
	}
	if run.Duplicates != 3 {
		t.Errorf("duplicates = %d, want 3", run.Duplicates)
	}
	if run.BaseSHA != "early00" {
		t.Errorf("BaseSHA = %q, want the earliest successful begin-batch envelope's early00", run.BaseSHA)
	}
	if run.WebsterEngine != fakeEngine {
		t.Errorf("WebsterEngine = %q, want %q, the engine that located the webster session", run.WebsterEngine, fakeEngine)
	}

	// A key repeated across two sessions of one engine counts once and adds one to Duplicates.
	const dedupeTree = "/hub/dedupe"
	shared := func() shuttleengine.TranscriptEntry {
		return timedAssistant("shared", "model-a", "2026-01-02T10:00:00Z", 5)
	}
	dedupe := oneEngine(dedupeTree,
		sessionOf(readOf("p.jsonl", "ly:dedupe:burler", shared())),
		sessionOf(readOf("q.jsonl", "ly:dedupe:burler", shared(), timedAssistant("own", "model-a", "2026-01-02T10:01:00Z", 7))),
	)
	deduped, err := CountRun(dedupe, dedupeTree, "dedupe", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if got := deduped.Roles["burler"]; got.Usage.Output != 12 || got.Sessions != 2 || deduped.Duplicates != 1 {
		t.Errorf("same-engine repeat: burler output = %d over %d sessions, Duplicates = %d; want 12 over 2 sessions, 1", got.Usage.Output, got.Sessions, deduped.Duplicates)
	}
	if deduped.WebsterEngine != "" {
		t.Errorf("WebsterEngine = %q for a run with no webster session; want empty", deduped.WebsterEngine)
	}
	// A second engine on the same worktree counts beside the first, and the same key under each engine counts twice.
	second := newFakeTranscripts()
	second.add(dedupeTree, time.Time{}, sessionOf(readOf("r.jsonl", "ly:dedupe:burler", shared())))
	dedupe["second"] = shuttleengine.RecordReads{Transcripts: second, Pricer: fakeRates}
	// An engine with no session there beside engines with sessions counts the others.
	dedupe["empty"] = shuttleengine.RecordReads{Transcripts: newFakeTranscripts()}
	twoEngines, err := CountRun(dedupe, dedupeTree, "dedupe", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if got := twoEngines.Roles["burler"]; got.Usage.Output != 17 || got.Sessions != 3 || twoEngines.Duplicates != 1 {
		t.Errorf("two engines: burler output = %d over %d sessions, Duplicates = %d; want 17 over 3 sessions, 1", got.Usage.Output, got.Sessions, twoEngines.Duplicates)
	}

	bare, err := CountRun(oneEngine("/hub/no-base", sessionOf(readOf("w.jsonl", "ly:no-base:webster",
		bashUse("refused", "lyx webster begin-batch 1"),
		toolResultFor("refused", "2026-01-02T02:00:00Z", `{"ok":false,"error":"paused"}`),
		// A begin-batch result without a timestamp does not set the base.
		bashUse("untimed", "lyx webster begin-batch 2"),
		shuttleengine.TranscriptEntry{Role: shuttleengine.EntryUser, ToolResults: []shuttleengine.ToolResult{{CallID: "untimed", Text: `{"start_sha":"untimed0"}`}}},
	))), "/hub/no-base", "no-base", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if bare.BaseSHA != "" {
		t.Errorf("BaseSHA = %q, want empty for a run with no successful timestamped begin-batch pair", bare.BaseSHA)
	}

	// Two Merriam sessions, the earlier-started one in the read that sorts last.
	// The first holds the skill-load turn and its assistant messages before the launch line, then the prompt read.
	launch := func(ts, path string) shuttleengine.TranscriptEntry {
		e := userAt(ts, "Read "+path+" in full first")
		e.PromptPath = path
		return e
	}
	readUse := func(id, useID, path string, cacheRead int) shuttleengine.TranscriptEntry {
		e := assistant(id, "model-a", 1, cacheRead)
		e.ToolCalls = []shuttleengine.ToolCall{{ID: useID, Kind: shuttleengine.ToolFileRead, Path: path}}
		return e
	}
	const merriamName = "ly:merriams:webster"
	merriams := oneEngine("/hub/merriams",
		sessionOf(readOf("z-first.jsonl", merriamName,
			userAt("2026-01-02T01:00:00Z", "load the skills"),
			assistant("s1", "model-a", 1, 100),
			launch("2026-01-02T01:00:05Z", "/p/prompt.md"),
			readUse("s2", "other", "/p/other.md", 150),
			toolResultFor("other", "2026-01-02T01:00:06Z", "other"),
			readUse("s3", "prompt", "/p/prompt.md", 200),
			toolResultFor("prompt", "2026-01-02T01:00:07Z", "prompt"),
			assistant("s4", "model-a", 1, 300),
			assistant("s5", "model-a", 1, 400),
		)),
		sessionOf(readOf("a-second.jsonl", merriamName,
			launch("2026-01-02T02:00:00Z", "/p/prompt.md"),
			assistant("t1", "model-a", 1, 100),
		)),
		sessionOf(readOf("m-list.jsonl", merriamName,
			launch("2026-01-02T03:00:00Z", "/p/list.md"),
			readUse("u1", "list", "/p/list.md", 100),
			toolResultFor("list", "2026-01-02T03:00:01Z", "prompt"),
			assistant("u2", "model-a", 1, 500),
		)),
		// A read that ends at the prompt read's result has nothing to measure.
		sessionOf(readOf("b-ends.jsonl", merriamName,
			launch("2026-01-02T04:00:00Z", "/p/prompt.md"),
			readUse("v1", "ends", "/p/prompt.md", 100),
			toolResultFor("ends", "2026-01-02T04:00:01Z", "prompt"),
		)),
	)
	withMerriams, err := CountRun(merriams, "/hub/merriams", "merriams", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	wantMerriams := []MerriamStart{
		{Session: "z-first.jsonl", Started: time.Date(2026, 1, 2, 1, 0, 0, 0, time.UTC), StartContext: 301},
		{Session: "a-second.jsonl", Started: time.Date(2026, 1, 2, 2, 0, 0, 0, time.UTC), NoStart: "no Read of the prompt file /p/prompt.md"},
		{Session: "m-list.jsonl", Started: time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC), StartContext: 501},
		{Session: "b-ends.jsonl", Started: time.Date(2026, 1, 2, 4, 0, 0, 0, time.UTC), NoStart: "no assistant message after the prompt Read's result"},
	}
	if !slices.EqualFunc(withMerriams.Merriams, wantMerriams, func(got, want MerriamStart) bool {
		return got.Session == want.Session && got.Started.Equal(want.Started) && got.StartContext == want.StartContext && got.NoStart == want.NoStart
	}) {
		t.Errorf("Merriams = %+v, want %+v", withMerriams.Merriams, wantMerriams)
	}

	// A bound leaves out a webster session that starts before it, with its fork, and a session with no timestamped line.
	bound := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	bounded := oneEngine("/hub/bounded",
		sessionOf(readOf("e-webster.jsonl", "ly:bounded:webster",
			launch("2026-01-02T10:00:00Z", "/p/prompt.md"),
			bashUse("old", "lyx webster begin-batch 1"),
			toolResultFor("old", "2026-01-02T10:01:00Z", `{"batch":"01-alpha","start_sha":"old0000"}`),
			timedAssistant("old1", "model-a", "2026-01-02T10:02:00Z", 50),
		), readOf("agent-old.jsonl", "",
			toolResult("2026-01-02T10:03:00Z", "- `_lyx/plan/01-alpha.md`"),
			timedAssistant("oldfork", "model-a", "2026-01-02T10:04:00Z", 7),
		)),
		sessionOf(readOf("n-none.jsonl", "ly:bounded:burler", assistant("none1", "model-a", 9, 0))),
		sessionOf(readOf("l-later.jsonl", "ly:bounded:burler",
			userAt("2026-01-02T13:00:00Z", "go"),
			timedAssistant("late1", "model-a", "2026-01-02T13:05:00Z", 4),
		)),
	)
	afterBound, err := CountRun(bounded, "/hub/bounded", "bounded", bound)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterBound.Roles) != 1 || afterBound.Roles["burler"] == nil || afterBound.Roles["burler"].Usage != (Usage{Output: 4}) || afterBound.Roles["burler"].Sessions != 1 {
		t.Errorf("bounded roles = %+v; want only the later session's burler usage", afterBound.Roles)
	}
	if len(afterBound.Forks) != 0 || len(afterBound.Merriams) != 0 || afterBound.BaseSHA != "" {
		t.Errorf("bounded run holds forks %+v, Merriams %+v, BaseSHA %q; want none from the left-out session", afterBound.Forks, afterBound.Merriams, afterBound.BaseSHA)
	}
	if want := time.Date(2026, 1, 2, 13, 0, 0, 0, time.UTC); !afterBound.First.Equal(want) || afterBound.LeftOut != 2 || !afterBound.Bound.Equal(bound) {
		t.Errorf("bounded First = %v, LeftOut = %d, Bound = %v; want %v, 2, %v", afterBound.First, afterBound.LeftOut, afterBound.Bound, want, bound)
	}
	var boundedReport bytes.Buffer
	if err := (Report{Runs: []RunTally{afterBound}}).WriteMarkdown(&boundedReport); err != nil {
		t.Fatal(err)
	}
	if want := "Session transcripts left out: 2; they started before the slug's previous run ended, at 2026-01-02T12:00:00Z.\n"; !strings.Contains(boundedReport.String(), want) {
		t.Errorf("bounded report lacks %q:\n%s", want, boundedReport.String())
	}

	wantForks := []ForkTally{
		{
			Slug: "my-task", File: "agent-1.jsonl", Cards: []string{"01-alpha", "02-beta"},
			Session: "b.jsonl", StartContext: 6, Messages: 2, PeakContext: 10, Usage: Usage{Input: 2, Output: 6, CacheRead: 14},
			Started: time.Date(2026, 1, 2, 3, 4, 5, 500_000_000, time.UTC),
		},
		{
			Slug: "my-task", File: "agent-2.jsonl",
			Session: "b.jsonl", StartContext: 3, Messages: 1, PeakContext: 3, Usage: Usage{Input: 1, Output: 3, CacheRead: 2},
			Started: time.Date(2026, 1, 2, 3, 10, 0, 0, time.UTC),
		},
		{
			Slug: "my-task", File: "agent-3.jsonl",
			Session: "a-webster.jsonl", StartContext: 3, Messages: 1, PeakContext: 3, Usage: Usage{Input: 1, Output: 1, CacheRead: 2},
			Started: time.Date(2026, 1, 2, 3, 15, 0, 0, time.UTC),
		},
	}
	if len(run.Forks) != len(wantForks) {
		t.Fatalf("forks = %+v, want %d forks", run.Forks, len(wantForks))
	}
	for i, want := range wantForks {
		got := run.Forks[i]
		if got.Slug != want.Slug || got.File != want.File || !slices.Equal(got.Cards, want.Cards) ||
			got.Messages != want.Messages || got.PeakContext != want.PeakContext ||
			got.StartContext != want.StartContext || got.Session != want.Session ||
			got.Usage != want.Usage || !got.Started.Equal(want.Started) {
			t.Errorf("fork %d = %+v, want %+v", i, got, want)
		}
	}

	var buf bytes.Buffer
	if err := (Report{Runs: []RunTally{run}}).WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## my-task", "## All runs", "| webster+sub | 3 | 10 | 0 | 18 | 4 | 0.0M | 58.0% |", "opus: 3, sonnet: 1",
		"## Webster forks\n\n| run | cards | start context | messages | peak context | weight |\n|---|---|---|---|---|---|\n" +
			"| my-task | 01-alpha, 02-beta | 6 | 2 | 10 | 0.0M |\n| my-task | unattributed | 3 | 1 | 3 | 0.0M |\n| my-task | unattributed | 3 | 1 | 3 | 0.0M |\n",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, buf.String())
		}
	}
	if strings.Index(buf.String(), "## All runs") > strings.Index(buf.String(), "## my-task") {
		t.Errorf("overview is not first:\n%s", buf.String())
	}
}

func TestCountRunWithoutSessions(t *testing.T) {
	t.Parallel()
	const worktree = "/hub/x"
	tests := []struct {
		name      string
		noSession bool
		locateErr error
		readErr   error
		// orch is set when CountOrch fails the same way as CountRun; absence is no failure for it.
		orch      bool
		wantParts []string
		wantIs    error
	}{
		{"no session from any engine", true, nil, nil, false, []string{"no sessions for x at /hub/x from engines fake"}, nil},
		{"locator error", false, errors.New("locator down"), nil, true, []string{"locating sessions of /hub/x through engine fake", "locator down"}, nil},
		{"read error", false, nil, errors.New("unreadable"), true, []string{"reading session s.jsonl of engine fake", "unreadable"}, nil},
		{"session vanished after locating", false, nil, fs.ErrNotExist, true, []string{"reading session s.jsonl of engine fake"}, fs.ErrNotExist},
	}
	runs := []RunTally{{Slug: "x", First: stampOf("2026-01-02T09:00:00Z"), Last: stampOf("2026-01-02T11:00:00Z")}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := newFakeTranscripts()
			if !tt.noSession {
				fake.add(worktree, time.Time{}, sessionOf(readOf("s.jsonl", "ly:x:burler", timedAssistant("m", "model-a", "2026-01-02T10:00:00Z", 1))))
			}
			fake.locateErr = tt.locateErr
			if tt.readErr != nil {
				fake.readErrs[worktree+"/s.jsonl"] = tt.readErr
			}
			reads := fakeReadSet(fake)
			calls := map[string]func() error{
				"CountRun": func() error { _, err := CountRun(reads, worktree, "x", time.Time{}); return err },
			}
			if tt.orch {
				calls["CountOrch"] = func() error { _, err := CountOrch(reads, worktree, runs); return err }
			}
			for name, call := range calls {
				err := call()
				if err == nil {
					t.Fatalf("%s = nil error; want one", name)
				}
				for _, part := range tt.wantParts {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("%s error = %q; want it to contain %q", name, err, part)
					}
				}
				if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
					t.Errorf("%s error = %v; want errors.Is %v", name, err, tt.wantIs)
				}
			}
		})
	}
}

func TestRecentRuns(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	fake := newFakeTranscripts()
	touch := func(worktree string, age time.Duration) {
		fake.add(worktree, now.Add(-age), sessionOf(readOf("s.jsonl", "ly:s:burler", assistant("m", "model-a", 1, 0))))
	}
	hub := t.TempDir()
	if err := os.Mkdir(filepath.Join(hub, "inflight"), 0o755); err != nil {
		t.Fatal(err)
	}
	finished := map[string][]ArchiveTag{}
	trees := fakeArchiveTrees{}
	// archive gives slug one archive tag whose seed names recipe and whose status holds the producers.
	archive := func(slug, recipe string, producers ...string) {
		finished[slug] = []ArchiveTag{{Name: "archive/" + slug + "/1", Commit: slug}}
		seedRel, statusRel, err := shedrun.TreeRunFileRels(slug)
		if err != nil {
			t.Fatal(err)
		}
		var entries []string
		for _, producer := range producers {
			entries = append(entries, `{"producer":"`+producer+`","at":"2026-01-01T10:00:00Z"}`)
		}
		trees[[2]string{slug, seedRel}] = `{"recipe":"` + recipe + `","driver":"go"}`
		trees[[2]string{slug, statusRel}] = `{"history":[` + strings.Join(entries, ",") + `]}`
	}
	full := []string{"Plan-Write", "Webster", "Finalize", "Friction-Reflect"}
	for slug, age := range map[string]time.Duration{"old": 5 * time.Hour, "mid": 4 * time.Hour, "new": 3 * time.Hour, "inflight": 2 * time.Hour, "prime": 0} {
		touch(filepath.Join(hub, slug), age)
		archive(slug, shedrun.RecipeLoom, full...)
	}
	touch(filepath.Join(hub, "darn"), 90*time.Minute)
	archive("darn", "darn", "Darn-Write", "Finalize")
	touch(filepath.Join(hub, "bench"), time.Hour)
	archive("bench", shedrun.RecipeLoom, "Preflight", "Discussion-Write")
	touch(filepath.Join(hub, "parked"), 0)
	// A finished slug with no session located under the hub, and one whose session sits in another hub, are never candidates.
	archive("empty", shedrun.RecipeLoom, full...)
	touch("/other/newest", 0)
	archive("newest", shedrun.RecipeLoom, full...)

	counted, skipped, err := recentRuns(fakeReadSet(fake), hub, "prime", finished, trees, 2)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(counted, ",") != "new,mid" {
		t.Errorf("recentRuns counted = %v, want [new mid]", counted)
	}
	if strings.Join(skipped, ",") != "bench,darn,inflight" {
		t.Errorf("recentRuns skipped = %v, want [bench darn inflight]", skipped)
	}

	fake.locateErr = errors.New("locator down")
	if _, _, err := recentRuns(fakeReadSet(fake), hub, "prime", finished, trees, 2); err == nil || !strings.Contains(err.Error(), "locator down") {
		t.Errorf("recentRuns with a failing locator = %v; want an error naming it", err)
	}
}

func TestArchivedSlugs(t *testing.T) {
	t.Parallel()
	at := func(s string) time.Time {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	tests := []struct {
		name string
		refs string
		want map[string][]ArchiveTag
	}{
		{
			"two tags of a slug newest first",
			"refs/tags/archive/s/old\tc1\t2026-01-01T00:00:00+00:00\nrefs/tags/archive/s/new\tc2\t2026-01-02T00:00:00+00:00\n",
			map[string][]ArchiveTag{"s": {
				{"archive/s/new", "c2", at("2026-01-02T00:00:00Z")},
				{"archive/s/old", "c1", at("2026-01-01T00:00:00Z")},
			}},
		},
		{
			"equal dates ordered by name",
			"refs/tags/archive/s/b\tc2\t2026-01-01T00:00:00+00:00\nrefs/tags/archive/s/a\tc1\t2026-01-01T00:00:00+00:00\n",
			map[string][]ArchiveTag{"s": {
				{"archive/s/a", "c1", at("2026-01-01T00:00:00Z")},
				{"archive/s/b", "c2", at("2026-01-01T00:00:00Z")},
			}},
		},
		{
			"malformed lines are skipped",
			"refs/tags/archive/no-sha\tc\t2026-01-01T00:00:00+00:00\n" +
				"refs/tags/other/x/y\tc\t2026-01-01T00:00:00+00:00\n" +
				"refs/tags/archive/s/empty\t\t2026-01-01T00:00:00+00:00\n" +
				"refs/tags/archive/s/nodate\tc\t\n" +
				"refs/tags/archive/s/ok\tc\t2026-01-01T00:00:00+00:00\n",
			map[string][]ArchiveTag{"s": {{"archive/s/ok", "c", at("2026-01-01T00:00:00Z")}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := archivedSlugs(tt.refs)
			if len(got) != len(tt.want) {
				t.Fatalf("archivedSlugs = %v, want %v", got, tt.want)
			}
			for slug, tags := range tt.want {
				if len(got[slug]) != len(tags) {
					t.Fatalf("archivedSlugs[%s] = %v, want %v", slug, got[slug], tags)
				}
				for i := range tags {
					g := got[slug][i]
					if g.Name != tags[i].Name || g.Commit != tags[i].Commit || !g.Committed.Equal(tags[i].Committed) {
						t.Errorf("archivedSlugs[%s][%d] = %v, want %v", slug, i, g, tags[i])
					}
				}
			}
		})
	}
}

// fakeArchiveTrees is an in-memory ArchiveTrees keyed by commit and tree-relative path.
type fakeArchiveTrees map[[2]string]string

func (f fakeArchiveTrees) FileAtRevision(rev, relPath string) ([]byte, error) {
	if content, ok := f[[2]string{rev, relPath}]; ok {
		return []byte(content), nil
	}
	return nil, gitrepo.ErrPathNotAtRevision
}

func TestShedRecordSpanAndProducers(t *testing.T) {
	t.Parallel()
	clock := func(hms string) string { return "2026-01-01T" + hms + "Z" }
	at := func(hms string) time.Time {
		parsed, err := time.Parse(time.RFC3339, clock(hms))
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	entry := func(producer, at string) string {
		return `{"producer":"` + producer + `","outcome":"done","output":"","at":"` + clock(at) + `"}`
	}
	folded := func(producer, at, lastAt string) string {
		return `{"producer":"` + producer + `","outcome":"done","output":"","at":"` + clock(at) + `","repeats":1,"last_at":"` + clock(lastAt) + `"}`
	}
	status := func(entries ...string) string { return `{"history":[` + strings.Join(entries, ",") + `]}` }
	seedRel, statusRel, err := shedrun.TreeRunFileRels("r")
	if err != nil {
		t.Fatal(err)
	}
	const seed = `{"recipe":"loom","driver":"go"}`
	seedStarted := func(startedAt string) string {
		return `{"recipe":"loom","driver":"go","started_at":"` + startedAt + `"}`
	}
	newest := ArchiveTag{Name: "archive/r/new", Commit: "c2", Committed: at("11:00:00")}
	older := func(committed string) ArchiveTag {
		return ArchiveTag{Name: "archive/r/old", Commit: "c1", Committed: at(committed)}
	}
	tree := func(commit, seedJSON, statusJSON string) fakeArchiveTrees {
		files := fakeArchiveTrees{}
		if seedJSON != "" {
			files[[2]string{commit, seedRel}] = seedJSON
		}
		if statusJSON != "" {
			files[[2]string{commit, statusRel}] = statusJSON
		}
		return files
	}
	plain := status(entry("Plan", "10:03:00"), entry("Publish", "10:12:00"))

	tests := []struct {
		name     string
		tags     []ArchiveTag
		files    fakeArchiveTrees
		wantNil  bool
		wantErr  string
		wantFrom time.Time
		wantTo   time.Time
		wantRows []ProducerRow
		wantIdle time.Duration
	}{
		{
			name:     "started_at, folded and out-of-order entries",
			tags:     []ArchiveTag{newest},
			files:    tree("c2", seedStarted(clock("09:58:00")), status(entry("Plan", "10:03:00"), folded("Bouncer", "10:04:00", "10:06:00"), entry("Plan", "10:05:00"), entry("Publish", "10:12:00"))),
			wantFrom: at("09:58:00"),
			wantTo:   at("10:12:00"),
			wantRows: []ProducerRow{
				{"Plan", 2, 5 * time.Minute, 3 * time.Minute},
				{"Publish", 1, 6 * time.Minute, 4 * time.Minute},
				{"Bouncer", 1, 3 * time.Minute, 3 * time.Minute},
			},
			wantIdle: 10 * time.Minute,
		},
		{name: "no started_at starts at the first transcript line", tags: []ArchiveTag{newest}, files: tree("c2", seed, plain), wantFrom: at("10:00:00"), wantTo: at("10:12:00")},
		{name: "older tag after the first line", tags: []ArchiveTag{newest, older("10:01:00")}, files: tree("c2", seed, plain), wantFrom: at("10:01:00"), wantTo: at("10:12:00")},
		{name: "first entry before the first line", tags: []ArchiveTag{newest}, files: tree("c2", seed, status(entry("Plan", "09:59:00"), entry("Publish", "10:12:00"))), wantFrom: at("09:59:00"), wantTo: at("10:12:00")},
		{name: "status without a seed", tags: []ArchiveTag{newest}, files: tree("c2", "", plain), wantFrom: at("10:00:00"), wantTo: at("10:12:00")},
		{
			name:     "newest tag is read",
			tags:     []ArchiveTag{newest, older("08:00:00")},
			files:    fakeArchiveTrees{[2]string{"c2", statusRel}: plain, [2]string{"c1", statusRel}: status(entry("Plan", "10:20:00"))},
			wantFrom: at("10:00:00"),
			wantTo:   at("10:12:00"),
		},
		{name: "status that does not decode", tags: []ArchiveTag{newest}, files: tree("c2", seed, "{"), wantErr: "status.json"},
		{name: "seed that does not decode", tags: []ArchiveTag{newest}, files: tree("c2", "{", plain), wantErr: "seed.json"},
		{name: "started_at that does not parse", tags: []ArchiveTag{newest}, files: tree("c2", seedStarted("yesterday"), plain), wantErr: "seed.json"},
		{name: "entry at that does not parse", tags: []ArchiveTag{newest}, files: tree("c2", seed, status(`{"producer":"Plan","at":"soon"}`)), wantErr: "status.json"},
		{name: "folded last_at that does not parse", tags: []ArchiveTag{newest}, files: tree("c2", seed, status(`{"producer":"Plan","at":"`+clock("10:03:00")+`","repeats":1,"last_at":"later"}`)), wantErr: "status.json"},
		{name: "no tags", files: tree("c2", seed, plain), wantNil: true},
		{name: "tag without status", tags: []ArchiveTag{newest}, files: tree("c2", seed, ""), wantNil: true},
		{name: "empty history", tags: []ArchiveTag{newest}, files: tree("c2", seed, status()), wantNil: true},
	}
	// A run whose burler works 10:00-10:02 and 10:08-10:10.
	counted := func(t *testing.T) RunTally {
		t.Helper()
		reads := oneEngine("/hub/r", sessionOf(readOf("a.jsonl", "ly:r:burler",
			userAt(clock("10:00:00"), "go"),
			stamped(clock("10:02:00"), true),
			userAt(clock("10:08:00"), "again"),
			stamped(clock("10:10:00"), true),
		)))
		run, err := CountRun(reads, "/hub/r", "r", time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		return run
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec, err := readShedRecord(tt.files, "r", tt.tags)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("readShedRecord = nil error; want one naming the file %s", tt.wantErr)
				}
				for _, part := range []string{"r", newest.Name, tt.wantErr} {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("readShedRecord error = %q; want it to name %q", err, part)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantNil {
				if rec != nil {
					t.Fatalf("readShedRecord = %+v; want no record", rec)
				}
				return
			}
			run := counted(t)
			run.useShedRecord(*rec)
			if !run.Span.From.Equal(tt.wantFrom) || !run.Span.To.Equal(tt.wantTo) {
				t.Errorf("span = %v..%v; want %v..%v", run.Span.From, run.Span.To, tt.wantFrom, tt.wantTo)
			}
			if tt.wantRows == nil {
				return
			}
			if got := run.Producers(); !slices.Equal(got, tt.wantRows) {
				t.Errorf("Producers() = %v; want %v", got, tt.wantRows)
			}
			if got := run.Idle(); got != tt.wantIdle {
				t.Errorf("Idle() = %v; want %v", got, tt.wantIdle)
			}
		})
	}

	// The report prints the Producers table for a run with a record, the no-status line for one without, and the summed table in All runs.
	rec, err := readShedRecord(tests[0].files, "r", tests[0].tags)
	if err != nil {
		t.Fatal(err)
	}
	withRecord := counted(t)
	withRecord.useShedRecord(*rec)
	without := counted(t)
	without.Slug = "bare"
	withRecord.Slug = "full"
	var buf bytes.Buffer
	if err := (Report{Runs: []RunTally{withRecord, without}}).WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	report := buf.String()
	all, rest, _ := strings.Cut(report, "## full\n")
	perFull, perBare, _ := strings.Cut(rest, "## bare\n")
	row := "| Publish | 1 | 6m0s | 4m0s | 2m0s | 50.0% |"
	for name, section := range map[string]string{"All runs": all, "full": perFull} {
		if !strings.Contains(section, "### Producers") || !strings.Contains(section, row) {
			t.Errorf("%s section lacks the Producers table row %q:\n%s", name, row, section)
		}
	}
	if strings.Contains(report, "leaves out machine sleep") {
		t.Errorf("report with no sleep subtracted claims a sleep subtraction:\n%s", report)
	}
	noStatus := "No shed status was read, so the span is the transcript window and no Producers table is printed."
	if !strings.Contains(perBare, noStatus) || strings.Contains(perBare, "### Producers") {
		t.Errorf("bare section = %q; want the no-status line and no table", perBare)
	}
	buf.Reset()
	if err := (Report{Runs: []RunTally{without}}).WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.SplitN(buf.String(), "## bare\n", 2)[0], "### Producers") {
		t.Errorf("All runs section prints a Producers table with no record:\n%s", buf.String())
	}

	// A sleep 10:01-10:09 is no time at all: it leaves the span, both working intervals it straddles and every Producers row.
	slept := counted(t)
	slept.useShedRecord(*rec)
	slept.subtractSleeps([]Interval{{at("10:01:00"), at("10:09:00")}})
	wantRows := []ProducerRow{
		{"Plan", 2, 3 * time.Minute, 2 * time.Minute},
		{"Publish", 1, 3 * time.Minute, 2 * time.Minute},
		{"Bouncer", 1, 0, 0},
	}
	if got := slept.Producers(); !slices.Equal(got, wantRows) {
		t.Errorf("Producers() with a sleep = %v; want %v", got, wantRows)
	}
	for name, c := range map[string]struct{ got, want time.Duration }{
		"AwakeSpan()":      {slept.AwakeSpan(), 6 * time.Minute},
		"Slept()":          {slept.Slept(), 8 * time.Minute},
		"Idle()":           {slept.Idle(), 4 * time.Minute},
		"RoleTime(burler)": {slept.RoleTime("burler"), 2 * time.Minute},
	} {
		if c.got != c.want {
			t.Errorf("%s with a sleep = %v; want %v", name, c.got, c.want)
		}
	}
	buf.Reset()
	if err := (Report{Runs: []RunTally{slept}}).WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Every time figure leaves out machine sleep, 8m0s over these runs.\n",
		"Machine sleep left out of the span: 8m0s.\n",
		"| idle | | | | | | | | 4m0s | 66.7% | | |",
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("report with a sleep lacks %q:\n%s", want, buf.String())
		}
	}
}
