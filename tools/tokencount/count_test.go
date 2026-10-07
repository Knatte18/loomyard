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
	writeLines(t, filepath.Join(dir, "c.jsonl"), assistant("m4", "haiku", 1, 0))

	run, err := CountRun(dir, "my-task")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Usage{
		"burler":      {Input: 1, Output: 2, CacheRead: 3},
		"webster":     {Input: 1, Output: 1, CacheRead: 1},
		"webster+sub": {Input: 3, Output: 9, CacheRead: 16},
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
	bare, err := CountRun(noBase, "no-base")
	if err != nil {
		t.Fatal(err)
	}
	if bare.BaseSHA != "" {
		t.Errorf("BaseSHA = %q, want empty for a run with no successful begin-batch pair", bare.BaseSHA)
	}

	wantForks := []ForkTally{
		{
			Slug: "my-task", File: "agent-1.jsonl", Cards: []string{"01-alpha", "02-beta"},
			Messages: 2, PeakContext: 10, Usage: Usage{Input: 2, Output: 6, CacheRead: 14},
			Started: time.Date(2026, 1, 2, 3, 4, 5, 500_000_000, time.UTC),
		},
		{
			Slug: "my-task", File: "agent-2.jsonl",
			Messages: 1, PeakContext: 3, Usage: Usage{Input: 1, Output: 3, CacheRead: 2},
			Started: time.Date(2026, 1, 2, 3, 10, 0, 0, time.UTC),
		},
	}
	if len(run.Forks) != len(wantForks) {
		t.Fatalf("forks = %+v, want %d forks", run.Forks, len(wantForks))
	}
	for i, want := range wantForks {
		got := run.Forks[i]
		if got.Slug != want.Slug || got.File != want.File || !slices.Equal(got.Cards, want.Cards) ||
			got.Messages != want.Messages || got.PeakContext != want.PeakContext ||
			got.Usage != want.Usage || !got.Started.Equal(want.Started) {
			t.Errorf("fork %d = %+v, want %+v", i, got, want)
		}
	}

	var buf bytes.Buffer
	if err := (Report{Runs: []RunTally{run}}).WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## my-task", "## All runs", "| webster+sub | 2 | 9 | 0 | 16 | 3 | 0.0M | 67.9% |", "opus: 2",
		"## Webster forks\n\n| run | cards | messages | peak context | weight |\n|---|---|---|---|---|\n" +
			"| my-task | 01-alpha, 02-beta | 2 | 10 | 0.0M |\n| my-task | unattributed | 1 | 3 | 0.0M |\n",
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
	if _, err := CountRun(projectDir(t.TempDir(), "/hub/none"), "none"); err == nil {
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
	touch("/hub/old", 3*time.Hour)
	touch("/hub/new", time.Hour)
	touch("/hub/mid", 2*time.Hour)
	touch("/hub/prime", 0)
	touch("/hub/parked", 0)
	touch("/other/newest", 0)
	if err := os.MkdirAll(projectDir(projects, "/hub/empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	finished := map[string]bool{"old": true, "new": true, "mid": true, "prime": true, "empty": true, "newest": true}

	got, err := recentRuns(projects, "/hub", "prime", finished, 2)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "new,mid" {
		t.Errorf("recentRuns = %v, want [new mid]", got)
	}
}

func TestArchivedSlugs(t *testing.T) {
	t.Parallel()
	refs := "refs/tags/archive/run-pushes/b783fd2\nrefs/tags/archive/run-pushes/0a1b2c3\nrefs/tags/archive/test-tiers/9f8e7d6\nrefs/tags/archive/no-sha\nrefs/tags/other/x/y\n"
	got := archivedSlugs(refs)
	if len(got) != 2 || !got["run-pushes"] || !got["test-tiers"] {
		t.Errorf("archivedSlugs = %v, want run-pushes and test-tiers", got)
	}
}
