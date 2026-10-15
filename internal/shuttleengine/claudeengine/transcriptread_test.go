package claudeengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
)

// writtenSessions returns a setup that writes a session named main.jsonl with the given lines and fork transcripts into a temporary projects root, and locates it.
func writtenSessions(mainLines []string, forks map[string][]string) func(t *testing.T) []shuttleengine.LocatedSession {
	return func(t *testing.T) []shuttleengine.LocatedSession {
		t.Helper()
		const worktree = "/hub/task"
		projects := t.TempDir()
		projectDir := projectDirUnder(projects, worktree)
		writeParentTranscript(t, projectDir, "main", mainLines)
		for name, lines := range forks {
			writeForkTranscript(t, filepath.Join(projectDir, "main", "subagents"), name, lines)
		}
		sessions, err := locateSessions(projects, worktree)
		if err != nil {
			t.Fatalf("locateSessions: %v", err)
		}
		return sessions
	}
}

// onlyRead adapts a check of the one session a row reads.
func onlyRead(check func(t *testing.T, read shuttleengine.SessionRead)) func(t *testing.T, sessions []shuttleengine.LocatedSession, reads []shuttleengine.SessionRead, err error) {
	return func(t *testing.T, sessions []shuttleengine.LocatedSession, reads []shuttleengine.SessionRead, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("ReadSession: %v", err)
		}
		if len(reads) != 1 {
			t.Fatalf("read %d sessions; want 1", len(reads))
		}
		check(t, reads[0])
	}
}

// usageOf returns the usage the entry carries, failing the test when it carries none.
func usageOf(t *testing.T, entry shuttleengine.TranscriptEntry) shuttleengine.MessageUsage {
	t.Helper()
	if entry.Usage == nil {
		t.Fatalf("entry %q carries no usage", entry.MessageKey)
	}
	return *entry.Usage
}

func TestReadSession(t *testing.T) {
	t.Parallel()

	const (
		userGo         = `{"type":"user","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"go"}}`
		toolResultLine = `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":"ok"}]}}`
		interruptLine  = `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user]"}]}}`
		localCommand   = `{"type":"user","message":{"role":"user","content":"<local-command-stdout>ok</local-command-stdout>"}}`
		compactSummary = `{"type":"user","isCompactSummary":true,"message":{"role":"user","content":"summary"}}`
	)
	assistantStop := func(stop string) string {
		return `{"type":"assistant","message":{"stop_reason":` + stop + `}}`
	}
	launchString := fmt.Sprintf(`{"type":"user","message":{"role":"user","content":%q}}`, launchPointer("/p/prompt.md"))
	launchItem := fmt.Sprintf(`{"type":"user","message":{"role":"user","content":[{"type":"text","text":%q}]}}`, launchPointer("/p/prompt.md"))

	tests := []struct {
		name  string
		setup func(t *testing.T) []shuttleengine.LocatedSession
		check func(t *testing.T, sessions []shuttleengine.LocatedSession, reads []shuttleengine.SessionRead, err error)
	}{
		{
			name: "a message streamed over three lines carries its usage once, on its last line",
			setup: writtenSessions([]string{
				assistantLine("s", false, 1, 0, 0, 1),
				assistantLine("s", false, 1, 0, 0, 2),
				assistantLine("s", false, 1, 0, 0, 5),
			}, nil),
			check: onlyRead(func(t *testing.T, read shuttleengine.SessionRead) {
				entries := read.Main.Entries
				if len(entries) != 3 || entries[0].Usage != nil || entries[1].Usage != nil {
					t.Fatalf("entries = %+v; want three, usage on the last only", entries)
				}
				if got, want := usageOf(t, entries[2]), (shuttleengine.MessageUsage{Input: 1, Output: 5}); got != want {
					t.Errorf("last line usage = %+v; want %+v", got, want)
				}
				if read.Main.Repeats != 2 {
					t.Errorf("Repeats = %d; want 2", read.Main.Repeats)
				}
			}),
		},
		{
			name: "id-less messages with uuids each carry usage, keyed by uuid",
			setup: writtenSessions([]string{
				`{"type":"assistant","uuid":"u1","message":{"usage":{"input_tokens":1,"output_tokens":1}}}`,
				`{"type":"assistant","uuid":"u2","message":{"usage":{"input_tokens":2,"output_tokens":2}}}`,
			}, nil),
			check: onlyRead(func(t *testing.T, read shuttleengine.SessionRead) {
				entries := read.Main.Entries
				if len(entries) != 2 || entries[0].MessageKey != "u1" || entries[1].MessageKey != "u2" {
					t.Fatalf("entries = %+v; want keys u1, u2", entries)
				}
				usageOf(t, entries[0])
				usageOf(t, entries[1])
				if read.Main.Repeats != 0 {
					t.Errorf("Repeats = %d; want 0", read.Main.Repeats)
				}
			}),
		},
		{
			name: "an id-less, uuid-less line is keyed by its transcript and line number",
			setup: writtenSessions(
				[]string{`not json`, `{"type":"assistant","message":{"usage":{"input_tokens":1}}}`},
				map[string][]string{"f.jsonl": {`not json`, `{"type":"assistant","isSidechain":true,"message":{"usage":{"input_tokens":1}}}`}},
			),
			check: onlyRead(func(t *testing.T, read shuttleengine.SessionRead) {
				if len(read.Forks) != 1 || len(read.Main.Entries) != 1 || len(read.Forks[0].Entries) != 1 {
					t.Fatalf("read = %+v; want one entry in the main and in its one fork", read)
				}
				if got, want := read.Main.Entries[0].MessageKey, "line:main.jsonl:2"; got != want {
					t.Errorf("main key = %q; want %q", got, want)
				}
				if got, want := read.Forks[0].Entries[0].MessageKey, "line:main.jsonl/f.jsonl:2"; got != want {
					t.Errorf("fork key = %q; want %q", got, want)
				}
				usageOf(t, read.Main.Entries[0])
				usageOf(t, read.Forks[0].Entries[0])
			}),
		},
		{
			name: "a fork replaying its parent's message carries no usage for it and counts each line as a repeat",
			setup: writtenSessions(
				[]string{assistantLine("p", false, 1, 0, 0, 1)},
				map[string][]string{"f.jsonl": {assistantLine("p", true, 1, 0, 0, 1), assistantLine("p", true, 1, 0, 0, 1), assistantLine("own", true, 2, 0, 0, 2)}},
			),
			check: onlyRead(func(t *testing.T, read shuttleengine.SessionRead) {
				fork := read.Forks[0]
				if len(fork.Entries) != 3 || fork.Entries[0].Usage != nil || fork.Entries[1].Usage != nil {
					t.Fatalf("fork entries = %+v; want three, the two replayed ones without usage", fork.Entries)
				}
				if got, want := usageOf(t, fork.Entries[2]), (shuttleengine.MessageUsage{Input: 2, Output: 2}); got != want {
					t.Errorf("own message usage = %+v; want %+v", got, want)
				}
				if fork.Repeats != 2 || read.Main.Repeats != 0 {
					t.Errorf("Repeats = fork %d, main %d; want 2, 0", fork.Repeats, read.Main.Repeats)
				}
				if fork.ID != "f.jsonl" || read.Main.ID != "main.jsonl" {
					t.Errorf("IDs = %q, %q; want f.jsonl, main.jsonl", fork.ID, read.Main.ID)
				}
			}),
		},
		{
			name: "the latest non-empty title names the session",
			setup: writtenSessions([]string{
				`{"type":"custom-title","customTitle":"ly:t:first"}`,
				`{"type":"custom-title","customTitle":"ly:t:second"}`,
				`{"type":"custom-title","customTitle":""}`,
			}, nil),
			check: onlyRead(func(t *testing.T, read shuttleengine.SessionRead) {
				if read.Main.Name != "ly:t:second" {
					t.Errorf("Name = %q; want ly:t:second", read.Main.Name)
				}
			}),
		},
		{
			name: "turn markers set the start and end flags",
			setup: writtenSessions([]string{
				userGo, toolResultLine, interruptLine, localCommand, compactSummary,
				assistantStop(`"end_turn"`), assistantStop(`"tool_use"`), assistantStop(`null`),
			}, nil),
			check: onlyRead(func(t *testing.T, read shuttleengine.SessionRead) {
				// Each pair is {StartsTurn, EndsTurn}, one per line in order:
				// A user text starts a turn, and a tool result neither starts nor ends one.
				// The interrupt marker and a local command's output end one, and a compaction summary starts none.
				// An end_turn assistant line ends one, while tool_use and no stop reason do not.
				want := [][2]bool{{true, false}, {false, false}, {false, true}, {false, true}, {false, false}, {false, true}, {false, false}, {false, false}}
				var got [][2]bool
				for _, entry := range read.Main.Entries {
					got = append(got, [2]bool{entry.StartsTurn, entry.EndsTurn})
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("turn flags = %v; want %v", got, want)
				}
			}),
		},
		{
			name: "a line without a usable timestamp has a zero time, and the start is the first timestamped line of any kind",
			setup: writtenSessions([]string{
				`{"type":"custom-title","customTitle":"ly:t:x"}`,
				`{"type":"system","timestamp":"2026-01-01T00:00:00Z"}`,
				`{"type":"user","message":{"role":"user","content":"no timestamp"}}`,
				`{"type":"user","timestamp":"not a time","message":{"role":"user","content":"bad timestamp"}}`,
				`{"type":"user","timestamp":"2026-01-01T01:00:00.5Z","message":{"role":"user","content":"ok"}}`,
			}, nil),
			check: onlyRead(func(t *testing.T, read shuttleengine.SessionRead) {
				entries := read.Main.Entries
				if len(entries) != 3 || !entries[0].At.IsZero() || !entries[1].At.IsZero() {
					t.Fatalf("entries = %+v; want three, the first two with a zero At", entries)
				}
				if want := time.Date(2026, 1, 1, 1, 0, 0, 500_000_000, time.UTC); !entries[2].At.Equal(want) {
					t.Errorf("At = %v; want %v", entries[2].At, want)
				}
				if want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC); !read.Main.Started.Equal(want) {
					t.Errorf("Started = %v; want %v", read.Main.Started, want)
				}
			}),
		},
		{
			name:  "the launch line sets the prompt path, as string content and as a text item",
			setup: writtenSessions([]string{launchString, launchItem, userGo}, nil),
			check: onlyRead(func(t *testing.T, read shuttleengine.SessionRead) {
				var got []string
				for _, entry := range read.Main.Entries {
					got = append(got, entry.PromptPath)
				}
				if want := []string{"/p/prompt.md", "/p/prompt.md", ""}; !reflect.DeepEqual(got, want) {
					t.Errorf("PromptPaths = %q; want %q", got, want)
				}
			}),
		},
		{
			name: "a malformed line and a half-written last line are skipped",
			setup: writtenSessions([]string{
				userGo,
				`not json`,
				assistantLine("a", false, 1, 0, 0, 1),
				`{"type":"assistant","message":{"id":"half","usage":{"input_tokens":1`,
			}, nil),
			check: onlyRead(func(t *testing.T, read shuttleengine.SessionRead) {
				if len(read.Main.Entries) != 2 || read.Main.Repeats != 0 {
					t.Errorf("read = %+v; want the user and the assistant entry, no repeats", read.Main)
				}
			}),
		},
		{
			name: "tool calls are classed and tool results carry their call id and text",
			setup: writtenSessions([]string{
				`{"type":"assistant","message":{"id":"a","content":[{"type":"tool_use","id":"b1","name":"Bash","input":{"command":"ls"}},{"type":"tool_use","id":"r1","name":"Read","input":{"file_path":"/f"}},{"type":"tool_use","id":"g1","name":"Grep","input":{"pattern":"x"}},{"type":"text","text":"hi"}]}}`,
				`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"b1","content":"out"},{"type":"tool_result","tool_use_id":"r1","content":[{"type":"text","text":"a"},{"type":"text","text":"b"}]}]}}`,
			}, nil),
			check: onlyRead(func(t *testing.T, read shuttleengine.SessionRead) {
				wantCalls := []shuttleengine.ToolCall{
					{ID: "b1", Kind: shuttleengine.ToolShell, Command: "ls"},
					{ID: "r1", Kind: shuttleengine.ToolFileRead, Path: "/f"},
					{ID: "g1", Kind: shuttleengine.ToolOther},
				}
				wantResults := []shuttleengine.ToolResult{{CallID: "b1", Text: "out"}, {CallID: "r1", Text: "a\nb"}}
				if got := read.Main.Entries[0].ToolCalls; !reflect.DeepEqual(got, wantCalls) {
					t.Errorf("ToolCalls = %+v; want %+v", got, wantCalls)
				}
				if got := read.Main.Entries[1].ToolResults; !reflect.DeepEqual(got, wantResults) {
					t.Errorf("ToolResults = %+v; want %+v", got, wantResults)
				}
			}),
		},
		{
			name: "a cache write with a one-hour part carries both figures",
			setup: writtenSessions([]string{
				`{"type":"assistant","message":{"id":"c","model":"opus","usage":{"input_tokens":1,"output_tokens":2,"cache_creation_input_tokens":30,"cache_read_input_tokens":4,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":20}}}}`,
			}, nil),
			check: onlyRead(func(t *testing.T, read shuttleengine.SessionRead) {
				want := shuttleengine.MessageUsage{Input: 1, CacheWrite: 30, CacheWriteOneHour: 20, CacheRead: 4, Output: 2}
				if got := usageOf(t, read.Main.Entries[0]); got != want {
					t.Errorf("usage = %+v; want %+v", got, want)
				}
				if read.Main.Entries[0].Model != "opus" {
					t.Errorf("Model = %q; want opus", read.Main.Entries[0].Model)
				}
			}),
		},
		{
			name:  "a sidechain line of the main transcript yields an entry with its usage",
			setup: writtenSessions([]string{assistantLine("sc", true, 7, 0, 0, 3)}, nil),
			check: onlyRead(func(t *testing.T, read shuttleengine.SessionRead) {
				if len(read.Main.Entries) != 1 {
					t.Fatalf("entries = %+v; want one", read.Main.Entries)
				}
				if got, want := usageOf(t, read.Main.Entries[0]), (shuttleengine.MessageUsage{Input: 7, Output: 3}); got != want {
					t.Errorf("usage = %+v; want %+v", got, want)
				}
			}),
		},
		{
			name: "a located session whose main file is removed errors wrapping not-exist",
			setup: func(t *testing.T) []shuttleengine.LocatedSession {
				sessions := writtenSessions([]string{userGo}, nil)(t)
				if err := os.Remove(sessions[0].Ref); err != nil {
					t.Fatal(err)
				}
				return sessions
			},
			check: func(t *testing.T, sessions []shuttleengine.LocatedSession, reads []shuttleengine.SessionRead, err error) {
				if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), sessions[0].Ref) {
					t.Errorf("err = %v; want not-exist naming %s", err, sessions[0].Ref)
				}
			},
		},
		{
			name: "a located session whose fork file is removed errors naming the fork's path",
			setup: func(t *testing.T) []shuttleengine.LocatedSession {
				sessions := writtenSessions([]string{userGo}, map[string][]string{"f.jsonl": {userGo}})(t)
				if err := os.Remove(sessions[0].Forks[0].Ref); err != nil {
					t.Fatal(err)
				}
				return sessions
			},
			check: func(t *testing.T, sessions []shuttleengine.LocatedSession, reads []shuttleengine.SessionRead, err error) {
				if err == nil || !strings.Contains(err.Error(), sessions[0].Forks[0].Ref) {
					t.Errorf("err = %v; want an error naming %s", err, sessions[0].Forks[0].Ref)
				}
			},
		},
		{
			name: "the tokencount-run transcripts read as the shuttlefake golden",
			setup: func(t *testing.T) []shuttleengine.LocatedSession {
				projects, err := filepath.Abs("testdata")
				if err != nil {
					t.Fatal(err)
				}
				sessions, err := locateSessions(projects, "tokencount-run")
				if err != nil {
					t.Fatal(err)
				}
				return sessions
			},
			check: func(t *testing.T, sessions []shuttleengine.LocatedSession, reads []shuttleengine.SessionRead, err error) {
				if err != nil {
					t.Fatalf("ReadSession: %v", err)
				}
				if want := shuttlefake.TokencountRunReads(); !reflect.DeepEqual(reads, want) {
					t.Errorf("reads = %+v; want %+v", reads, want)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sessions := tt.setup(t)
			var reads []shuttleengine.SessionRead
			var err error
			for _, session := range sessions {
				var read shuttleengine.SessionRead
				if read, err = New().ReadSession(session); err != nil {
					break
				}
				reads = append(reads, read)
			}
			tt.check(t, sessions, reads, err)
		})
	}
}

func TestLocateSessions(t *testing.T) {
	const worktree = "/hub/task"

	home := t.TempDir()
	t.Setenv("HOME", home)
	writeParentTranscript(t, projectDirUnder(filepath.Join(home, ".claude", "projects"), worktree), "only", []string{`{"type":"user"}`})
	found, err := New().LocateSessions(worktree)
	if err != nil || len(found) != 1 || found[0].ID != "only.jsonl" {
		t.Errorf("LocateSessions under HOME = %+v, %v; want the one session only.jsonl", found, err)
	}

	earlier := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	touch := func(t *testing.T, path string, at time.Time) {
		t.Helper()
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	ids := func(refs []shuttleengine.TranscriptRef) []string {
		var got []string
		for _, ref := range refs {
			got = append(got, ref.ID)
		}
		return got
	}

	tests := []struct {
		name string
		// setup prepares the projects root and returns the project directory's path under it.
		setup func(t *testing.T, projects, projectDir string)
		check func(t *testing.T, projects, projectDir string, got []shuttleengine.LocatedSession, err error)
	}{
		{
			name:  "a missing projects root is no sessions and no error",
			setup: func(t *testing.T, projects, projectDir string) { os.RemoveAll(projects) },
			check: func(t *testing.T, projects, projectDir string, got []shuttleengine.LocatedSession, err error) {
				if err != nil || len(got) != 0 {
					t.Errorf("got %+v, %v; want none", got, err)
				}
			},
		},
		{
			name:  "a missing project directory is no sessions and no error",
			setup: func(t *testing.T, projects, projectDir string) {},
			check: func(t *testing.T, projects, projectDir string, got []shuttleengine.LocatedSession, err error) {
				if err != nil || len(got) != 0 {
					t.Errorf("got %+v, %v; want none", got, err)
				}
			},
		},
		{
			name: "sessions come back sorted with their file times and sorted forks",
			setup: func(t *testing.T, projects, projectDir string) {
				writeParentTranscript(t, projectDir, "b", []string{`{}`})
				writeParentTranscript(t, projectDir, "a", []string{`{}`})
				touch(t, filepath.Join(projectDir, "a.jsonl"), earlier)
				touch(t, filepath.Join(projectDir, "b.jsonl"), later)
				writeForkTranscript(t, filepath.Join(projectDir, "a", "subagents"), "z.jsonl", []string{`{}`})
				writeForkTranscript(t, filepath.Join(projectDir, "a", "subagents"), "y.jsonl", []string{`{}`})
				touch(t, filepath.Join(projectDir, "a", "subagents", "y.jsonl"), later)
			},
			check: func(t *testing.T, projects, projectDir string, got []shuttleengine.LocatedSession, err error) {
				if err != nil || len(got) != 2 {
					t.Fatalf("got %+v, %v; want two sessions", got, err)
				}
				if got[0].ID != "a.jsonl" || got[1].ID != "b.jsonl" {
					t.Errorf("IDs = %q, %q; want a.jsonl, b.jsonl", got[0].ID, got[1].ID)
				}
				if want := filepath.Join(projectDir, "a.jsonl"); got[0].Ref != want {
					t.Errorf("Ref = %q; want %q", got[0].Ref, want)
				}
				if !got[0].Modified.Equal(earlier) || !got[1].Modified.Equal(later) {
					t.Errorf("Modified = %v, %v; want %v, %v", got[0].Modified, got[1].Modified, earlier, later)
				}
				if want := []string{"y.jsonl", "z.jsonl"}; !reflect.DeepEqual(ids(got[0].Forks), want) {
					t.Errorf("forks = %q; want %q", ids(got[0].Forks), want)
				}
				if len(got[1].Forks) != 0 {
					t.Errorf("a session without subagents has forks %+v; want none", got[1].Forks)
				}
			},
		},
		{
			name: "files that are not session transcripts are ignored",
			setup: func(t *testing.T, projects, projectDir string) {
				writeParentTranscript(t, projectDir, "s", []string{`{}`})
				if err := os.WriteFile(filepath.Join(projectDir, "notes.txt"), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				writeForkTranscript(t, filepath.Join(projectDir, "s", "subagents"), "f.jsonl", []string{`{}`})
				if err := os.WriteFile(filepath.Join(projectDir, "s", "subagents", "notes.txt"), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, projects, projectDir string, got []shuttleengine.LocatedSession, err error) {
				if err != nil || len(got) != 1 || got[0].ID != "s.jsonl" || !reflect.DeepEqual(ids(got[0].Forks), []string{"f.jsonl"}) {
					t.Errorf("got %+v, %v; want only s.jsonl with fork f.jsonl", got, err)
				}
			},
		},
		{
			name: "a project directory that is a file errors naming it",
			setup: func(t *testing.T, projects, projectDir string) {
				if err := os.WriteFile(projectDir, []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, projects, projectDir string, got []shuttleengine.LocatedSession, err error) {
				if err == nil || !strings.Contains(err.Error(), projectDir) {
					t.Errorf("err = %v; want an error naming %s", err, projectDir)
				}
			},
		},
		{
			name: "a subagents path that is a file errors naming it",
			setup: func(t *testing.T, projects, projectDir string) {
				writeParentTranscript(t, projectDir, "s", []string{`{}`})
				if err := os.Mkdir(filepath.Join(projectDir, "s"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(projectDir, "s", "subagents"), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, projects, projectDir string, got []shuttleengine.LocatedSession, err error) {
				want := filepath.Join(projectDir, "s", "subagents")
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("err = %v; want an error naming %s", err, want)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			projects := t.TempDir()
			projectDir := projectDirUnder(projects, worktree)
			if err := os.MkdirAll(projects, 0o755); err != nil {
				t.Fatal(err)
			}
			tt.setup(t, projects, projectDir)
			got, err := locateSessions(projects, worktree)
			tt.check(t, projects, projectDir, got, err)
		})
	}
}
