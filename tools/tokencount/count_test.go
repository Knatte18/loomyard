package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

func writeLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assistant(id, model string, out, read int) string {
	return `{"type":"assistant","message":{"id":"` + id + `","model":"` + model +
		`","usage":{"input_tokens":1,"output_tokens":` + strconv.Itoa(out) +
		`,"cache_creation_input_tokens":0,"cache_read_input_tokens":` + strconv.Itoa(read) + `}}}`
}

// toolResult is a user transcript line at ts holding one tool result whose content is contentJSON, a JSON string or list.
func toolResult(ts, contentJSON string) string {
	return toolResultFor("t", ts, contentJSON)
}

// toolResultFor is toolResult answering the tool use with the given id.
func toolResultFor(useID, ts, contentJSON string) string {
	return `{"type":"user","timestamp":"` + ts + `","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"` + useID + `","content":` + contentJSON + `}]}}`
}

// bashUse is an assistant transcript line holding one Bash tool use of command.
func bashUse(t *testing.T, id, command string) string {
	t.Helper()
	return `{"type":"assistant","message":{"id":"use-` + id + `","content":[{"type":"tool_use","id":"` + id + `","name":"Bash","input":{"command":` + jsonString(t, command) + `}}]}}`
}

func jsonString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// stamped is an assistant transcript line at ts with the given stop reason.
func stamped(ts, stopReason string) string {
	return `{"type":"assistant","timestamp":"` + ts + `","message":{"id":"` + ts + `","stop_reason":` + stopReason + `,"usage":{"input_tokens":1,"output_tokens":1}}}`
}

// userAt is a user transcript line at ts holding text.
func userAt(t *testing.T, ts, text string) string {
	t.Helper()
	return `{"type":"user","timestamp":"` + ts + `","message":{"role":"user","content":` + jsonString(t, text) + `}}`
}

func TestRunTimeByRole(t *testing.T) {
	t.Parallel()
	const endTurn, toolUse = `"end_turn"`, `"tool_use"`
	build := func(t *testing.T, sessions map[string][]string) RunTally {
		t.Helper()
		dir := t.TempDir()
		for name, lines := range sessions {
			writeLines(t, filepath.Join(dir, name), lines...)
		}
		run, err := CountRun(dir, "r", time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		return run
	}
	title := func(role string) string { return `{"type":"custom-title","customTitle":"ly:r:` + role + `"}` }

	// burler: two turns of 1m and 2m with a 10m gap, then an interrupted turn of 1m followed by 5m of idle and a typed local command.
	// tool: a tool_use turn whose result arrives 3m later counts the gap.
	// An overlapping second burler session adds only its uncovered tail, and a sub-agent counts under burler+sub.
	run := build(t, map[string][]string{
		"a.jsonl": {
			title("burler"),
			userAt(t, "2026-01-01T00:00:00Z", "go"),
			stamped("2026-01-01T00:01:00Z", endTurn),
			userAt(t, "2026-01-01T00:11:00Z", "again"),
			stamped("2026-01-01T00:13:00Z", endTurn),
			userAt(t, "2026-01-01T00:14:00Z", "third"),
			userAt(t, "2026-01-01T00:15:00Z", "[Request interrupted by user]"),
			userAt(t, "2026-01-01T00:20:00Z", "<local-command-stdout>ok</local-command-stdout>"),
			userAt(t, "2026-01-01T00:30:00Z", "late"),
			stamped("2026-01-01T00:31:00Z", toolUse),
			toolResult("2026-01-01T00:34:00Z", jsonString(t, "done")),
			stamped("2026-01-01T00:35:00Z", endTurn),
		},
		"b.jsonl": {
			title("burler"),
			userAt(t, "2026-01-01T00:32:00Z", "overlap"),
			stamped("2026-01-01T00:37:00Z", endTurn),
		},
		"a/subagents/s.jsonl": {
			userAt(t, "2026-01-01T00:40:00Z", "sub"),
			stamped("2026-01-01T00:42:00Z", endTurn),
		},
	})
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
	done := jsonString(t, "done")
	compactSummary := `{"type":"user","isCompactSummary":true,"timestamp":"2026-01-01T00:05:00Z","message":{"role":"user","content":` + jsonString(t, "summary") + `}}`
	for _, c := range []struct {
		name  string
		lines []string
		work  time.Duration
		idle  time.Duration
	}{
		{"prompt an hour after a tool_use", []string{stamped("2026-01-01T00:00:00Z", toolUse), userAt(t, "2026-01-01T01:00:00Z", "p"), stamped("2026-01-01T01:01:00Z", endTurn)}, min, 60 * min},
		{"prompt an hour after a tool_result", []string{toolResult("2026-01-01T00:00:00Z", done), userAt(t, "2026-01-01T01:00:00Z", "p"), stamped("2026-01-01T01:01:00Z", endTurn)}, min, 60 * min},
		{"tool_result 32 hours after its tool_use", []string{stamped("2026-01-01T00:00:00Z", toolUse), toolResult("2026-01-02T08:00:00Z", done), stamped("2026-01-02T08:01:00Z", endTurn)}, min, 32 * 60 * min},
		{"assistant line an hour after a tool_result", []string{toolResult("2026-01-01T00:00:00Z", done), stamped("2026-01-01T01:00:00Z", endTurn)}, 0, 60 * min},
		{"tool_result 20 minutes after its tool_use", []string{stamped("2026-01-01T00:00:00Z", toolUse), toolResult("2026-01-01T00:20:00Z", done), stamped("2026-01-01T00:21:00Z", endTurn)}, 21 * min, 0},
		{"interrupt marker under the cap", []string{stamped("2026-01-01T00:00:00Z", toolUse), userAt(t, "2026-01-01T00:10:00Z", "[Request interrupted by user]")}, 10 * min, 0},
		{"user text a second after a tool_result", []string{toolResult("2026-01-01T00:00:00Z", done), userAt(t, "2026-01-01T00:00:01Z", "queued"), stamped("2026-01-01T00:00:30Z", endTurn)}, 29 * time.Second, time.Second},
		{"compact summary after its command", []string{userAt(t, "2026-01-01T00:00:00Z", "<command-name>/compact</command-name>"), compactSummary, stamped("2026-01-01T00:06:00Z", endTurn)}, 6 * min, 0},
	} {
		got := build(t, map[string][]string{"a.jsonl": append([]string{title("burler")}, c.lines...)})
		if w := got.RoleTime("burler"); w != c.work {
			t.Errorf("%s: RoleTime(burler) = %v; want %v", c.name, w, c.work)
		}
		if i := got.Idle(); i != c.idle {
			t.Errorf("%s: Idle() = %v; want %v", c.name, i, c.idle)
		}
	}

	// Two runs whose working intervals overlap in wall-clock time sum per role and in idle, and share the summed span.
	one := build(t, map[string][]string{"a.jsonl": {title("burler"), userAt(t, "2026-01-01T00:00:00Z", "x"), stamped("2026-01-01T00:02:00Z", endTurn), userAt(t, "2026-01-01T00:04:00Z", "y")}})
	two := build(t, map[string][]string{"a.jsonl": {title("burler"), userAt(t, "2026-01-01T00:01:00Z", "x"), stamped("2026-01-01T00:03:00Z", endTurn), userAt(t, "2026-01-01T00:05:00Z", "y")}})
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
	projects := t.TempDir()
	worktree := "/hub/my-task"
	dir := projectDir(projects, worktree)
	if filepath.Base(dir) != "-hub-my-task" {
		t.Fatalf("projectDir = %s, want -hub-my-task", filepath.Base(dir))
	}

	writeLines(t, filepath.Join(dir, "a.jsonl"),
		`{"type":"custom-title","customTitle":"ly:my-task:burler"}`,
		assistant("m1", "sonnet", 2, 3),
		assistant("m1", "sonnet", 2, 3), // the same message on a second line
		`not json`,
		`{"type":"custom-title","customTitle":"ly:my-task:burler-2"}`,
	)
	// Two successful begin-batch pairs, the later-timestamped one in the file that sorts first.
	// The refusal envelope and the unrelated result mentioning start_sha precede the earliest pair.
	writeLines(t, filepath.Join(dir, "a-webster.jsonl"),
		`{"type":"custom-title","customTitle":"ly:my-task:webster"}`,
		bashUse(t, "late", "lyx webster begin-batch 2"),
		toolResultFor("late", "2026-01-02T03:20:00Z", jsonString(t, `{"batch":"02-beta","start_sha":"late000"}`)),
	)
	writeLines(t, filepath.Join(dir, "b.jsonl"),
		`{"type":"custom-title","customTitle":"ly:my-task:webster"}`,
		bashUse(t, "refused", "lyx webster begin-batch 1"),
		toolResultFor("refused", "2026-01-02T02:00:00Z", jsonString(t, `{"ok":false,"error":"paused"}`)),
		bashUse(t, "other", "git log"),
		toolResultFor("other", "2026-01-02T02:30:00Z", jsonString(t, `{"start_sha":"unrelated"}`)),
		bashUse(t, "early", "lyx webster begin-batch 1"),
		toolResultFor("early", "2026-01-02T03:00:00Z", `[{"type":"text","text":`+jsonString(t, `{"batch":"01-alpha","start_sha":"early00"}`)+`}]`),
		assistant("m2", "sonnet", 1, 1),
	)
	prompt := "1\t# Webster fork\n" +
		"58\t- `_lyx/plan/01-alpha.md`\n" +
		"59\t- `_lyx/plan/02-beta.md`\n" +
		"60\t- `_lyx/plan/00-overview.md`\n" +
		"70\t- `_lyx/plan/03-gamma.md`: go test ./...\n"
	writeLines(t, filepath.Join(dir, "b", "subagents", "agent-1.jsonl"),
		toolResult("2026-01-02T03:04:05.5Z", jsonString(t, "no pointer here")),
		assistant("m2", "sonnet", 1, 1), // the parent's context repeated in the fork
		assistant("m3", "opus", 4, 5),
		toolResult("2026-01-02T03:04:06Z", `[{"type":"text","text":`+jsonString(t, prompt)+`}]`),
		assistant("m5", "opus", 2, 9),
		toolResult("2026-01-02T03:04:07Z", jsonString(t, "- `_lyx/plan/04-delta.md`")),
	)
	writeLines(t, filepath.Join(dir, "b", "subagents", "agent-2.jsonl"),
		toolResult("2026-01-02T03:10:00Z", jsonString(t, "- `docs/plan.md`")),
		assistant("m6", "sonnet", 3, 2),
	)
	// A fork under a second webster session.
	writeLines(t, filepath.Join(dir, "a-webster", "subagents", "agent-3.jsonl"),
		toolResult("2026-01-02T03:15:00Z", jsonString(t, "no pointer here")),
		assistant("m7", "opus", 1, 2),
	)
	writeLines(t, filepath.Join(dir, "c.jsonl"), assistant("m4", "haiku", 1, 0))

	run, err := CountRun(dir, "my-task", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Usage{
		"burler":      {Input: 1, Output: 2, CacheRead: 3},
		"webster":     {Input: 1, Output: 1, CacheRead: 1},
		"webster+sub": {Input: 4, Output: 10, CacheRead: 18},
		"untitled":    {Input: 1, Output: 1},
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
	if run.Duplicates != 2 {
		t.Errorf("duplicates = %d, want 2", run.Duplicates)
	}
	if run.BaseSHA != "early00" {
		t.Errorf("BaseSHA = %q, want the earliest successful begin-batch envelope's early00", run.BaseSHA)
	}

	noBase := projectDir(projects, "/hub/no-base")
	writeLines(t, filepath.Join(noBase, "w.jsonl"),
		`{"type":"custom-title","customTitle":"ly:no-base:webster"}`,
		bashUse(t, "refused", "lyx webster begin-batch 1"),
		toolResultFor("refused", "2026-01-02T02:00:00Z", jsonString(t, `{"ok":false,"error":"paused"}`)),
	)
	bare, err := CountRun(noBase, "no-base", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if bare.BaseSHA != "" {
		t.Errorf("BaseSHA = %q, want empty for a run with no successful begin-batch pair", bare.BaseSHA)
	}

	// Two Merriam sessions, the earlier-started one in the file that sorts last.
	// The first holds the skill-load turn and its assistant messages before the launch line, then the prompt Read.
	merriams := projectDir(projects, "/hub/merriams")
	launch := func(ts, path string) string {
		return `{"type":"user","timestamp":"` + ts + `","message":{"role":"user","content":"Read ` + path + ` in full first; it is your complete, authoritative instructions."}}`
	}
	readUse := func(id, useID, path string, cacheRead int) string {
		return `{"type":"assistant","message":{"id":"` + id + `","content":[{"type":"tool_use","id":"` + useID + `","name":"Read","input":{"file_path":"` + path + `"}}],` +
			`"usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":` + strconv.Itoa(cacheRead) + `}}}`
	}
	writeLines(t, filepath.Join(merriams, "z-first.jsonl"),
		`{"type":"custom-title","customTitle":"ly:merriams:webster"}`,
		`{"type":"user","timestamp":"2026-01-02T01:00:00Z","message":{"role":"user","content":[{"type":"text","text":"load the skills"}]}}`,
		assistant("s1", "opus", 1, 100),
		launch("2026-01-02T01:00:05Z", "/p/prompt.md"),
		readUse("s2", "other", "/p/other.md", 150),
		toolResultFor("other", "2026-01-02T01:00:06Z", jsonString(t, "other")),
		readUse("s3", "prompt", "/p/prompt.md", 200),
		toolResultFor("prompt", "2026-01-02T01:00:07Z", jsonString(t, "prompt")),
		assistant("s4", "opus", 1, 300),
		assistant("s5", "opus", 1, 400),
	)
	writeLines(t, filepath.Join(merriams, "a-second.jsonl"),
		`{"type":"custom-title","customTitle":"ly:merriams:webster"}`,
		launch("2026-01-02T02:00:00Z", "/p/prompt.md"),
		assistant("t1", "opus", 1, 100),
	)
	// A launch line typed as a text item of list content measures as string content does.
	writeLines(t, filepath.Join(merriams, "m-list.jsonl"),
		`{"type":"custom-title","customTitle":"ly:merriams:webster"}`,
		`{"type":"user","timestamp":"2026-01-02T03:00:00Z","message":{"role":"user","content":[{"type":"text","text":"Read /p/list.md in full first; it is your complete, authoritative instructions."}]}}`,
		readUse("u1", "list", "/p/list.md", 100),
		toolResultFor("list", "2026-01-02T03:00:01Z", jsonString(t, "prompt")),
		assistant("u2", "opus", 1, 500),
	)
	// A transcript that ends at the prompt Read's result has nothing to measure.
	writeLines(t, filepath.Join(merriams, "b-ends.jsonl"),
		`{"type":"custom-title","customTitle":"ly:merriams:webster"}`,
		launch("2026-01-02T04:00:00Z", "/p/prompt.md"),
		readUse("v1", "ends", "/p/prompt.md", 100),
		toolResultFor("ends", "2026-01-02T04:00:01Z", jsonString(t, "prompt")),
	)
	withMerriams, err := CountRun(merriams, "merriams", time.Time{})
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
	boundedDir := projectDir(projects, "/hub/bounded")
	bound := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	writeLines(t, filepath.Join(boundedDir, "e-webster.jsonl"),
		`{"type":"custom-title","customTitle":"ly:bounded:webster"}`,
		launch("2026-01-02T10:00:00Z", "/p/prompt.md"),
		bashUse(t, "old", "lyx webster begin-batch 1"),
		toolResultFor("old", "2026-01-02T10:01:00Z", jsonString(t, `{"batch":"01-alpha","start_sha":"old0000"}`)),
		timedAssistant("old1", "opus", "2026-01-02T10:02:00Z", 50),
	)
	writeLines(t, filepath.Join(boundedDir, "e-webster", "subagents", "agent-old.jsonl"),
		toolResult("2026-01-02T10:03:00Z", jsonString(t, "- `_lyx/plan/01-alpha.md`")),
		timedAssistant("oldfork", "opus", "2026-01-02T10:04:00Z", 7),
	)
	writeLines(t, filepath.Join(boundedDir, "n-none.jsonl"),
		`{"type":"custom-title","customTitle":"ly:bounded:burler"}`,
		assistant("none1", "sonnet", 9, 0),
	)
	writeLines(t, filepath.Join(boundedDir, "l-later.jsonl"),
		`{"type":"custom-title","customTitle":"ly:bounded:burler"}`,
		userAt(t, "2026-01-02T13:00:00Z", "go"),
		timedAssistant("late1", "sonnet", "2026-01-02T13:05:00Z", 4),
	)
	afterBound, err := CountRun(boundedDir, "bounded", bound)
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
		"## my-task", "## All runs", "| webster+sub | 3 | 10 | 0 | 18 | 4 | 0.0M | 70.5% |", "opus: 3, sonnet: 1",
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
	if _, err := CountRun(projectDir(t.TempDir(), "/hub/none"), "none", time.Time{}); err == nil {
		t.Fatal("CountRun found sessions in an empty projects directory")
	}
}

func TestRecentRuns(t *testing.T) {
	t.Parallel()
	projects := t.TempDir()
	touch := func(worktree string, age time.Duration) {
		path := filepath.Join(projectDir(projects, worktree), "s.jsonl")
		writeLines(t, path, assistant("m", "sonnet", 1, 0))
		when := time.Now().Add(-age)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
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
	for slug, age := range map[string]time.Duration{"old": 5 * time.Hour, "mid": 4 * time.Hour, "new": 3 * time.Hour, "inflight": 2 * time.Hour, "prime": 0, "empty": 0} {
		touch(filepath.Join(hub, slug), age)
		archive(slug, shedrun.RecipeLoom, full...)
	}
	touch(filepath.Join(hub, "darn"), 90*time.Minute)
	archive("darn", "darn", "Darn-Write", "Finalize")
	touch(filepath.Join(hub, "bench"), time.Hour)
	archive("bench", shedrun.RecipeLoom, "Preflight", "Discussion-Write")
	touch(filepath.Join(hub, "parked"), 0)
	touch("/other/newest", 0)
	archive("newest", shedrun.RecipeLoom, full...)
	if err := os.RemoveAll(filepath.Join(projectDir(projects, filepath.Join(hub, "empty")), "s.jsonl")); err != nil {
		t.Fatal(err)
	}

	counted, skipped, err := recentRuns(projects, hub, "prime", finished, trees, 2)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(counted, ",") != "new,mid" {
		t.Errorf("recentRuns counted = %v, want [new mid]", counted)
	}
	if strings.Join(skipped, ",") != "bench,darn,inflight" {
		t.Errorf("recentRuns skipped = %v, want [bench darn inflight]", skipped)
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
				{"Publish", 1, 6 * time.Minute, 4 * time.Minute},
				{"Plan", 2, 5 * time.Minute, 3 * time.Minute},
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
		dir := t.TempDir()
		writeLines(t, filepath.Join(dir, "a.jsonl"),
			`{"type":"custom-title","customTitle":"ly:r:burler"}`,
			userAt(t, clock("10:00:00"), "go"),
			stamped(clock("10:02:00"), `"end_turn"`),
			userAt(t, clock("10:08:00"), "again"),
			stamped(clock("10:10:00"), `"end_turn"`),
		)
		run, err := CountRun(dir, "r", time.Time{})
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
	row := "| Publish | 1 | 6m0s | 4m0s | 42.9% |"
	for name, section := range map[string]string{"All runs": all, "full": perFull} {
		if !strings.Contains(section, "### Producers") || !strings.Contains(section, row) {
			t.Errorf("%s section lacks the Producers table row %q:\n%s", name, row, section)
		}
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
}
