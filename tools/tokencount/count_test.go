package main

import (
	"bytes"
	"os"
	"path/filepath"
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
	writeLines(t, filepath.Join(dir, "b.jsonl"),
		`{"type":"custom-title","customTitle":"ly:my-task:webster"}`,
		assistant("m2", "sonnet", 1, 1),
	)
	writeLines(t, filepath.Join(dir, "b", "subagents", "agent-1.jsonl"),
		assistant("m2", "sonnet", 1, 1), // the parent's context repeated in the fork
		assistant("m3", "opus", 4, 5),
	)
	writeLines(t, filepath.Join(dir, "c.jsonl"), assistant("m4", "haiku", 1, 0))

	run, err := CountRun(dir, "my-task")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Usage{
		"burler":      {Input: 1, Output: 2, CacheRead: 3},
		"webster":     {Input: 1, Output: 1, CacheRead: 1},
		"webster+sub": {Input: 1, Output: 4, CacheRead: 5},
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

	var buf bytes.Buffer
	if err := (Report{Runs: []RunTally{run}}).WriteMarkdown(&buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## my-task", "## All runs", "| webster+sub | 1 | 4 | 0 | 5 | 1 | 0.0M | 47.9% |", "opus: 1"} {
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
	touch("/hub/x-weft", 0)
	touch("/other/newest", 0)
	if err := os.MkdirAll(projectDir(projects, "/hub/empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := recentRuns(projects, "/hub", "prime", 2)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "new,mid" {
		t.Errorf("recentRuns = %v, want [new mid]", got)
	}
}
