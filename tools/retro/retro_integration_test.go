//go:build !windows

// retro_integration_test.go runs retro.sh over a fixture records repository and transcript root.
// It checks the tables the script writes and a row of the figures it renders.

package retro

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// session is one row of sessions.jsonl.
type session struct {
	Slug        string  `json:"slug"`
	Session     string  `json:"session"`
	Kind        string  `json:"kind"`
	Title       string  `json:"title"`
	Row         string  `json:"row"`
	Model       string  `json:"model"`
	Cost        float64 `json:"cost"`
	Turns       int     `json:"turns"`
	Calls       int     `json:"calls"`
	LookupTurns int     `json:"lookup_turns"`
	SerialSaved int     `json:"serial_saved"`
	Refusals    int     `json:"refusals"`
	GateRuns    int     `json:"gate_runs"`
	GateFails   int     `json:"gate_fails"`
}

// writeFile writes content to path, creating its directory.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readLines returns the lines of path, failing the test when it cannot be read.
func readLines(t *testing.T, path string) []string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

// buildRecords makes a records repository with an archive tag for run-a.
// The tag holds three usage records of one row, a shed status and two review facts files.
func buildRecords(t *testing.T, dir string) {
	t.Helper()
	gitkit.Git(t, dir, "init", "-b", "main")
	// s1 is an outlier on turns alone and s3 on cost alone, so the rule must be turns or cost.
	writeFile(t, filepath.Join(dir, lymdirs.LymDirName, "usage", "session-s1.json"),
		`{"session_id":"s1","row":"Darn-Write","role":"darn","model":"claude-opus-5-5","cost_usd":0.6}`)
	writeFile(t, filepath.Join(dir, lymdirs.LymDirName, "usage", "session-s2.json"),
		`{"session_id":"s2","row":"Darn-Write","role":"darn","model":"claude-opus-5-5","cost_usd":0.5}`)
	writeFile(t, filepath.Join(dir, lymdirs.LymDirName, "usage", "session-s3.json"),
		`{"session_id":"s3","row":"Darn-Write","role":"darn","model":"claude-opus-5-5","cost_usd":1.2}`)
	writeFile(t, filepath.Join(dir, lymdirs.LymDirName, "shed", "run-a", "status.json"), `{"history":[
		{"producer":"Darn-Write","outcome":"done","at":"2026-11-10T11:00:00Z","gate_attempts":2,"gate_spans":[
			{"gate":"verify","kind":"gate","start":"2026-11-10T10:00:00Z","end":"2026-11-10T10:05:00Z"},
			{"gate":"verify","kind":"rework","start":"2026-11-10T10:05:00Z","end":"2026-11-10T10:20:00Z"},
			{"gate":"verify","kind":"gate","start":"2026-11-10T10:20:00Z","end":"2026-11-10T10:25:00Z"}]},
		{"producer":"Finalize","outcome":"done","at":"2026-11-10T12:00:00Z"}]}`)
	writeFile(t, filepath.Join(dir, lymdirs.LymDirName, "reviews", "code", "round-1-facts.md"), "# Round 1 facts\n\n"+
		"| Round | Findings | BLOCKING | design | Gating |\n| --- | --- | --- | --- | --- |\n| 1 | 9 | 9 | 9 | 9 |\n")
	writeFile(t, filepath.Join(dir, lymdirs.LymDirName, "reviews", "code", "round-2-facts.md"), "# Round 2 facts\n\n"+
		"| Round | Findings | BLOCKING | design | Gating |\n| --- | --- | --- | --- | --- |\n"+
		"| 1 | 3 | 1 | 2 | 1 |\n| 2 | 1 | 0 | 1 | 0 |\n\n"+
		"## Recurring keys (ledger rounds each key was open in)\n\n- `stale-doc`: rounds 1, 2\n\n"+
		"## Keys open in an earlier round (ledger rounds each key was open in)\n\n- `other-key`: rounds 1\n")
	gitkit.Git(t, dir, "add", "-A")
	gitkit.Git(t, dir, "commit", "-m", "run-a records")
	gitkit.Git(t, dir, "tag", "archive/run-a/abc123")
}

// transcriptLines returns the lines of a transcript whose session made the given calls.
// Each call takes one turn and is answered by the matching result.
func transcriptLines(title string, calls []map[string]any, results []string) string {
	var lines []string
	if title != "" {
		line, _ := json.Marshal(map[string]any{"type": "custom-title", "customTitle": title})
		lines = append(lines, string(line))
	}
	for i, call := range calls {
		id := "t" + string(rune('a'+i))
		call["type"], call["id"] = "tool_use", id
		stamp := "2026-11-10T10:0" + string(rune('0'+i)) + ":00Z"
		turn, _ := json.Marshal(map[string]any{"type": "assistant", "timestamp": stamp,
			"message": map[string]any{"id": "m" + id, "content": []any{call}}})
		result, _ := json.Marshal(map[string]any{"type": "user", "timestamp": stamp,
			"message": map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": id, "content": results[i]}}}})
		lines = append(lines, string(turn), string(result))
	}
	return strings.Join(lines, "\n") + "\n"
}

func TestRetroScript_WritesTheFiguresOfEachRun(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	for _, tool := range []string{"jq", "awk", "bash"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("retro.sh tests need %s on PATH: %v", tool, err)
		}
	}

	root := t.TempDir()
	records, projects, hub, out := filepath.Join(root, "records"), filepath.Join(root, "projects"), filepath.Join(root, "hub"), filepath.Join(root, "out")
	if err := os.MkdirAll(records, 0o755); err != nil {
		t.Fatal(err)
	}
	buildRecords(t, records)
	runDir := filepath.Join(projects, strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, filepath.Join(hub, "run-a")))
	// The sixth call, a gate run moved to the background, fails only in the task notification that follows it.
	notification, _ := json.Marshal(map[string]any{"type": "queue-operation",
		"content": "<task-notification><tool-use-id>tf</tool-use-id><summary>Background command failed (exit code 1)</summary></task-notification>"})
	writeFile(t, filepath.Join(runDir, "s1.jsonl"), transcriptLines("ly:run-a:darn",
		[]map[string]any{
			{"name": "Read", "input": map[string]any{"file_path": "/w/a.go"}},
			{"name": "Bash", "input": map[string]any{"command": "grep -rn Foo internal/"}},
			{"name": "Bash", "input": map[string]any{"command": "rm -rf $DIR"}},
			{"name": "Bash", "input": map[string]any{"command": "lym gate test ./internal/x"}},
			{"name": "Edit", "input": map[string]any{"file_path": "/w/a.go"}},
			{"name": "Bash", "input": map[string]any{"command": "lym gate test ./internal/y"}},
		},
		[]string{"package a", "a.go:1:Foo", "PreToolUse:Bash hook error: rm on a computed path is refused here", "Exit code 1\nFAIL", "ok",
			"Command was moved to the background with ID: b1"})+string(notification)+"\n")
	edit := []map[string]any{{"name": "Edit", "input": map[string]any{"file_path": "/w/a.go"}}}
	writeFile(t, filepath.Join(runDir, "s2.jsonl"), transcriptLines("ly:run-a:darn", append(edit, edit...), []string{"ok", "ok"}))
	writeFile(t, filepath.Join(runDir, "s3.jsonl"), transcriptLines("ly:run-a:darn", edit, []string{"ok"}))
	writeFile(t, filepath.Join(runDir, "s4.jsonl"), transcriptLines("ly:run-a:gate-fixer", edit, []string{"ok"}))
	writeFile(t, filepath.Join(runDir, "s1/subagents/agent-f1.jsonl"), transcriptLines("",
		[]map[string]any{{"name": "Glob", "input": map[string]any{"pattern": "*.go"}}}, []string{"a.go"}))

	script, err := filepath.Abs("retro.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", script, out, "run-a", "run-gone")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PROJECTS="+projects, "HUB="+hub, "RECORDS="+records)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("retro.sh = %v; want success:\n%s", err, output)
	}

	var got []session
	for _, line := range readLines(t, filepath.Join(out, "sessions.jsonl")) {
		var row session
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("sessions.jsonl line %q: %v", line, err)
		}
		got = append(got, row)
	}
	want := []session{
		{Slug: "run-a", Session: "s1", Kind: "main", Title: "ly:run-a:darn", Row: "Darn-Write", Model: "claude-opus-5-5", Cost: 0.6,
			Turns: 6, Calls: 6, LookupTurns: 2, SerialSaved: 1, Refusals: 1, GateRuns: 2, GateFails: 2},
		{Slug: "run-a", Session: "agent-f1", Kind: "fork", Turns: 1, Calls: 1, LookupTurns: 1},
		{Slug: "run-a", Session: "s2", Kind: "main", Title: "ly:run-a:darn", Row: "Darn-Write", Model: "claude-opus-5-5", Cost: 0.5, Turns: 2, Calls: 2},
		{Slug: "run-a", Session: "s3", Kind: "main", Title: "ly:run-a:darn", Row: "Darn-Write", Model: "claude-opus-5-5", Cost: 1.2, Turns: 1, Calls: 1},
		{Slug: "run-a", Session: "s4", Kind: "main", Title: "ly:run-a:gate-fixer", Turns: 1, Calls: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("sessions.jsonl = %+v; want %+v", got, want)
	}

	tables := []struct {
		file string
		want []string
	}{
		{"refusals.jsonl", []string{`{"slug":"run-a","session":"s1","kind":"main","turn":3,"tool":"Bash","refusal":"rm on a computed path is refused here"}`}},
		{"rows.jsonl", []string{
			`{"slug":"run-a","idx":0,"producer":"Darn-Write","outcome":"done","gate_attempts":2,"verify_runs":2,"verify_reworks":1,"routed_to":""}`,
			`{"slug":"run-a","idx":1,"producer":"Finalize","outcome":"done","gate_attempts":0,"verify_runs":0,"verify_reworks":0,"routed_to":""}`,
		}},
		{"facts.jsonl", []string{
			`{"slug":"run-a","perch":"code","round":1,"column":"Findings","value":"3"}`,
			`{"slug":"run-a","perch":"code","round":1,"column":"BLOCKING","value":"1"}`,
			`{"slug":"run-a","perch":"code","round":1,"column":"design","value":"2"}`,
			`{"slug":"run-a","perch":"code","round":1,"column":"Gating","value":"1"}`,
			`{"slug":"run-a","perch":"code","round":2,"column":"Findings","value":"1"}`,
			`{"slug":"run-a","perch":"code","round":2,"column":"BLOCKING","value":"0"}`,
			`{"slug":"run-a","perch":"code","round":2,"column":"design","value":"1"}`,
			`{"slug":"run-a","perch":"code","round":2,"column":"Gating","value":"0"}`,
		}},
		{"usage.jsonl", []string{
			`{"slug":"run-a","session":"s1","row":"Darn-Write","role":"darn","model":"claude-opus-5-5","cost":0.6}`,
			`{"slug":"run-a","session":"s2","row":"Darn-Write","role":"darn","model":"claude-opus-5-5","cost":0.5}`,
			`{"slug":"run-a","session":"s3","row":"Darn-Write","role":"darn","model":"claude-opus-5-5","cost":1.2}`,
		}},
		{"keys.jsonl", []string{`{"slug":"run-a","perch":"code","key":"stale-doc","rounds":"1, 2"}`}},
		{"missing.jsonl", []string{`{"slug":"run-gone","what":"archive tag"}`, `{"slug":"run-gone","what":"transcript directory (Claude Code layout)"}`}},
	}
	for _, table := range tables {
		if got := readLines(t, filepath.Join(out, table.file)); !reflect.DeepEqual(got, table.want) {
			t.Errorf("%s = %q; want %q", table.file, got, table.want)
		}
	}

	figures := readLines(t, filepath.Join(out, "figures.md"))
	for _, want := range []string{
		"| run-a | 4 | 1 | $2.3 | 2 | 1 | 1 | code 2 |",
		"| Darn-Write | 3 | 2 | $0.6 | run-a s1 (6 turns, $0.6); run-a s3 (1 turns, $1.2) |",
		"| code | Findings | 4 |",
		"| code | design | 3 |",
		"- run-a code: `stale-doc`, rounds 1, 2",
	} {
		found := false
		for _, line := range figures {
			found = found || line == want
		}
		if !found {
			t.Errorf("figures.md has no line %q:\n%s", want, strings.Join(figures, "\n"))
		}
	}
}
