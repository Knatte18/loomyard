// audit_test.go covers AuditForks end to end against a fake ~/.claude/projects layout built under t.TempDir().
// It asserts the parent-transcript spawn tally and each fork's ForkReport fields.
// It also covers the zero-fork finding for a missing subagents/ directory and the missing-parent-transcript error.
// The path encoding that claudeProjectDirFor derives is covered too.

package claudeengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// copyFixture copies the named testdata fixture to dest, creating dest's parent directory as needed.
// The tests use it to build the fake project directory under t.TempDir().
func copyFixture(t *testing.T, name, dest string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(dest), err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		t.Fatalf("write %q: %v", dest, err)
	}
}

// TestAuditForks_FullLayout builds a complete fake project dir:
// a parent transcript and three fork transcripts under subagents/.
// It asserts the ForkAudit and ForkReport fields the audit populates.
func TestAuditForks_FullLayout(t *testing.T) {
	tiergate.Require(t, 0)
	home := t.TempDir()
	t.Setenv("HOME", home)

	workdir := "/home/op/work dir" // a space exercises the non-alnum encoding
	sessionID := "sess-full"
	projectDir := filepath.Join(home, ".claude", "projects", encodeForTest(workdir))

	copyFixture(t, "parent-spawns.jsonl", filepath.Join(projectDir, sessionID+".jsonl"))
	// Named so os.ReadDir's sorted order matches this test's forks[N] indexing.
	copyFixture(t, "fork-clean.jsonl", filepath.Join(projectDir, sessionID, "subagents", "01-clean.jsonl"))
	copyFixture(t, "fork-nested-agent.jsonl", filepath.Join(projectDir, sessionID, "subagents", "02-nested-agent.jsonl"))
	copyFixture(t, "fork-mutating.jsonl", filepath.Join(projectDir, sessionID, "subagents", "03-mutating.jsonl"))

	c := New()
	audit, err := c.AuditForks(sessionID, workdir)
	if err != nil {
		t.Fatalf("AuditForks() error: %v", err)
	}

	if audit.SpawnCalls != 2 {
		t.Errorf("SpawnCalls = %d; want 2 (two Agent tool_use entries in parent-spawns.jsonl)", audit.SpawnCalls)
	}
	if audit.NamedSpawns != 1 {
		t.Errorf("NamedSpawns = %d; want 1 (only the second spawn carries a name)", audit.NamedSpawns)
	}
	if len(audit.Forks) != 3 {
		t.Fatalf("len(Forks) = %d; want 3", len(audit.Forks))
	}

	clean, nested, mutating := audit.Forks[0], audit.Forks[1], audit.Forks[2]

	if !strings.HasSuffix(clean.TranscriptPath, "01-clean.jsonl") {
		t.Errorf("Forks[0].TranscriptPath = %q; want it to name 01-clean.jsonl", clean.TranscriptPath)
	}
	if clean.AgentCalls != 0 || clean.WriteCalls != 0 {
		t.Errorf("clean fork AgentCalls/WriteCalls = %d/%d; want 0/0", clean.AgentCalls, clean.WriteCalls)
	}
	if len(clean.BashCommands) != 0 {
		t.Errorf("clean fork BashCommands = %v; want none", clean.BashCommands)
	}
	if clean.ToolCalls["Read"] != 1 || clean.ToolCalls["Grep"] != 1 {
		t.Errorf("clean fork ToolCalls = %v; want Read:1, Grep:1", clean.ToolCalls)
	}
	if !clean.ReportReturned {
		t.Error("clean fork ReportReturned = false; want true (ends with a non-empty final assistant message)")
	}

	if nested.AgentCalls != 1 {
		t.Errorf("nested-agent fork AgentCalls = %d; want 1", nested.AgentCalls)
	}
	if nested.ToolCalls["Agent"] != 1 || nested.ToolCalls["Read"] != 1 {
		t.Errorf("nested-agent fork ToolCalls = %v; want Read:1, Agent:1", nested.ToolCalls)
	}
	if !nested.ReportReturned {
		t.Error("nested-agent fork ReportReturned = false; want true")
	}

	if mutating.WriteCalls != 1 {
		t.Errorf("mutating fork WriteCalls = %d; want 1", mutating.WriteCalls)
	}
	// WritePaths carries the written paths themselves, not only their count.
	// A caller whose forks may write checks which files each fork touched.
	if len(mutating.WritePaths) != 1 || mutating.WritePaths[0] != "/tmp/repo/notes.md" {
		t.Errorf("mutating fork WritePaths = %v; want [\"/tmp/repo/notes.md\"]", mutating.WritePaths)
	}
	if len(mutating.BashCommands) != 1 || mutating.BashCommands[0] != "git commit -am 'oops'" {
		t.Errorf("mutating fork BashCommands = %v; want [\"git commit -am 'oops'\"]", mutating.BashCommands)
	}
	if mutating.ToolCalls["Write"] != 1 || mutating.ToolCalls["Bash"] != 1 {
		t.Errorf("mutating fork ToolCalls = %v; want Write:1, Bash:1", mutating.ToolCalls)
	}
	if mutating.ReportReturned {
		t.Error("mutating fork ReportReturned = true; want false (transcript ends with a tool_result, no final text message)")
	}
}

// wantWriteEvents is what testdata/parent-writes-results.jsonl must yield:
// a succeeded Write, an Edit whose result is an error, and a Write that never got a result.
func wantWriteEvents() []shuttleengine.WriteEvent {
	return []shuttleengine.WriteEvent{
		{Path: "/work/a.txt", At: time.Date(2026, 1, 2, 10, 0, 1, 500_000_000, time.UTC), Succeeded: true},
		{Path: "/work/b.txt", At: time.Date(2026, 1, 2, 10, 0, 6, 0, time.UTC), Succeeded: false},
		{Path: "/work/c.txt", At: time.Date(2026, 1, 2, 10, 0, 10, 0, time.UTC), Succeeded: false},
	}
}

func assertWriteEvents(t *testing.T, label string, got []shuttleengine.WriteEvent) {
	t.Helper()
	want := wantWriteEvents()
	if len(got) != len(want) {
		t.Fatalf("%s: len = %d; want %d (%v)", label, len(got), len(want), got)
	}
	for i := range want {
		if got[i].Path != want[i].Path || !got[i].At.Equal(want[i].At) || got[i].Succeeded != want[i].Succeeded {
			t.Errorf("%s[%d] = %+v; want %+v", label, i, got[i], want[i])
		}
	}
}

// TestAuditForks_WriteEventsCarryTimeAndResult proves the write events carry time and result.
// The parent transcript and a fork transcript each yield their write calls in order.
// Each call is resolved against the tool_result that names its id, and the path lists keep their meaning.
func TestAuditForks_WriteEventsCarryTimeAndResult(t *testing.T) {
	tiergate.Require(t, 0)
	home := t.TempDir()
	t.Setenv("HOME", home)

	workdir := "/home/op/write-events"
	sessionID := "sess-writes"
	projectDir := filepath.Join(home, ".claude", "projects", encodeForTest(workdir))
	copyFixture(t, "parent-writes-results.jsonl", filepath.Join(projectDir, sessionID+".jsonl"))
	copyFixture(t, "parent-writes-results.jsonl", filepath.Join(projectDir, sessionID, "subagents", "01-writer.jsonl"))

	audit, err := New().AuditForks(sessionID, workdir)
	if err != nil {
		t.Fatalf("AuditForks() error: %v", err)
	}

	assertWriteEvents(t, "ParentWriteEvents", audit.ParentWriteEvents)
	if len(audit.Forks) != 1 {
		t.Fatalf("len(Forks) = %d; want 1", len(audit.Forks))
	}
	assertWriteEvents(t, "Forks[0].WriteEvents", audit.Forks[0].WriteEvents)

	wantPaths := []string{"/work/a.txt", "/work/b.txt", "/work/c.txt"}
	if audit.ParentWriteCalls != 3 || strings.Join(audit.ParentWrites, ",") != strings.Join(wantPaths, ",") {
		t.Errorf("ParentWriteCalls/ParentWrites = %d/%v; want 3/%v", audit.ParentWriteCalls, audit.ParentWrites, wantPaths)
	}
	fork := audit.Forks[0]
	if fork.WriteCalls != 3 || strings.Join(fork.WritePaths, ",") != strings.Join(wantPaths, ",") {
		t.Errorf("fork WriteCalls/WritePaths = %d/%v; want 3/%v", fork.WriteCalls, fork.WritePaths, wantPaths)
	}
}

// TestAuditForks_MissingSubagentsDirYieldsZeroForks proves a missing subagents/ directory is not an error.
// A fork-authorized session that spawned no forks is a legitimate zero-fork finding.
// The parent transcript is still read, so SpawnCalls and NamedSpawns are populated from the spawns it attempted.
func TestAuditForks_MissingSubagentsDirYieldsZeroForks(t *testing.T) {
	tiergate.Require(t, 0)
	home := t.TempDir()
	t.Setenv("HOME", home)

	workdir := "/home/op/no-forks-here"
	sessionID := "sess-no-forks"
	projectDir := filepath.Join(home, ".claude", "projects", encodeForTest(workdir))
	copyFixture(t, "parent-spawns.jsonl", filepath.Join(projectDir, sessionID+".jsonl"))
	// No subagents/ directory is created: the session spawned no forks.

	c := New()
	audit, err := c.AuditForks(sessionID, workdir)
	if err != nil {
		t.Fatalf("AuditForks() error: %v; want nil (missing subagents/ is not an error)", err)
	}
	if audit.Forks == nil || len(audit.Forks) != 0 {
		t.Errorf("Forks = %#v; want an empty, non-nil slice", audit.Forks)
	}
	if audit.SpawnCalls != 2 || audit.NamedSpawns != 1 {
		t.Errorf("SpawnCalls/NamedSpawns = %d/%d; want 2/1 (parent transcript is still read)", audit.SpawnCalls, audit.NamedSpawns)
	}
}

// TestAuditForks_MissingParentTranscriptErrors proves a missing parent transcript is an error.
// Without the parent transcript there is no way to know what the session spawned.
func TestAuditForks_MissingParentTranscriptErrors(t *testing.T) {
	tiergate.Require(t, 0)
	home := t.TempDir()
	t.Setenv("HOME", home)

	c := New()
	_, err := c.AuditForks("sess-absent", "/home/op/never-ran")
	if err == nil {
		t.Fatal("AuditForks() with no parent transcript on disk = nil error; want an error")
	}
}

// TestClaudeProjectDirFor_EncodesNonAlnumBytes pins the exact cwd-encoding derivation:
// every non-alphanumeric byte becomes '-'.
//
//testtiming:keep pins the literal encoding of a backslash-and-space path, which its covering test computes through its own reimplementation of the encoding
func TestClaudeProjectDirFor_EncodesNonAlnumBytes(t *testing.T) {
	tiergate.Require(t, 0)
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := claudeProjectDirFor(`C:\a b\c`)
	if err != nil {
		t.Fatalf("claudeProjectDirFor() error: %v", err)
	}
	want := filepath.Join(home, ".claude", "projects", "C--a-b-c")
	if got != want {
		t.Errorf("claudeProjectDirFor(%q) = %q; want %q", `C:\a b\c`, got, want)
	}
}

// encodeForTest returns workdir with each non-alphanumeric byte replaced by '-'.
// It repeats the encoding here, so that a test's expected directory does not depend on the code under test.
func encodeForTest(workdir string) string {
	encoded := []byte(workdir)
	for i, b := range encoded {
		isAlnum := (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
		if !isAlnum {
			encoded[i] = '-'
		}
	}
	return string(encoded)
}
