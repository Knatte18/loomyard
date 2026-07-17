// audit_test.go covers AuditForks end to end against a fake ~/.claude/projects
// layout built under t.TempDir(): the parent-transcript spawn tally, one ForkReport
// per fork transcript with every field asserted, the missing-subagents-dir
// zero-fork finding, the missing-parent-transcript error, and the path-encoding
// derivation claudeProjectDirFor implements.

package claudeengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// copyFixture copies the named testdata fixture file to dest, creating dest's
// parent directory as needed — the "build a fake project dir under t.TempDir()"
// step every subtest below uses to populate a derived ~/.claude/projects layout.
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

// TestAuditForks_FullLayout builds a complete fake project dir — a parent
// transcript plus three fork transcripts under subagents/ — and asserts every
// ForkAudit/ForkReport field the audit populates.
func TestAuditForks_FullLayout(t *testing.T) {
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
	// WritePaths must carry the written path itself, not just the count — a
	// caller whose forks may write (webster's implementers) polices WHICH
	// files were touched, per-fork, exactly as ParentWrites does for Master.
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

// TestAuditForks_MissingSubagentsDirYieldsZeroForks proves a fork-authorized
// session that never actually spawned a fork (no subagents/ directory at all)
// is a legitimate zero-fork finding, not an error — the parent transcript's own
// SpawnCalls/NamedSpawns are still populated from what it attempted.
func TestAuditForks_MissingSubagentsDirYieldsZeroForks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	workdir := "/home/op/no-forks-here"
	sessionID := "sess-no-forks"
	projectDir := filepath.Join(home, ".claude", "projects", encodeForTest(workdir))
	copyFixture(t, "parent-spawns.jsonl", filepath.Join(projectDir, sessionID+".jsonl"))
	// Deliberately no subagents/ directory created at all.

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

// TestAuditForks_MissingParentTranscriptErrors proves a missing parent
// transcript IS an error — unlike the missing-subagents-dir case, there is no
// way to know what the session spawned without it.
func TestAuditForks_MissingParentTranscriptErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	c := New()
	_, err := c.AuditForks("sess-absent", "/home/op/never-ran")
	if err == nil {
		t.Fatal("AuditForks() with no parent transcript on disk = nil error; want an error")
	}
}

// TestClaudeProjectDirFor_EncodesNonAlnumBytes pins the exact cwd-encoding
// derivation: every non-alphanumeric byte becomes '-', mirroring
// claudeProjectDir in internal/muxcli/smoke_test.go.
func TestClaudeProjectDirFor_EncodesNonAlnumBytes(t *testing.T) {
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

// encodeForTest reproduces claudeProjectDirFor's encoding step directly (rather
// than calling the unexported function under a different HOME) so this test file
// can compute the expected project directory for a given workdir independently
// of the function under test.
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
