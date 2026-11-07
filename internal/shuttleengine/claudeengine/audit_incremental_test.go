// audit_incremental_test.go covers the seen-set filtering of AuditForksIncremental.
// Only fork transcripts whose path is not a key of seenTranscripts are reported in Forks.
// A nil map makes AuditForksIncremental equivalent to AuditForks.
// The parent facts are unaffected by the seen set and always read in full.
// Fixture transcripts are written directly with os.WriteFile: no fixture files, no subprocess.

package claudeengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeForkTranscript writes lines as a newline-joined fork transcript at subagentsDir/name.
// It creates subagentsDir as needed and returns the full path written.
func writeForkTranscript(t *testing.T, subagentsDir, name string, lines []string) string {
	t.Helper()
	if err := os.MkdirAll(subagentsDir, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", subagentsDir, err)
	}
	data := ""
	for _, line := range lines {
		data += line + "\n"
	}
	dest := filepath.Join(subagentsDir, name)
	if err := os.WriteFile(dest, []byte(data), 0o644); err != nil {
		t.Fatalf("write %q: %v", dest, err)
	}
	return dest
}

// TestAuditForksIncremental_SeenSetFiltering proves that Forks reports only the fork transcripts not yet seen.
func TestAuditForksIncremental_SeenSetFiltering(t *testing.T) {
	tiergate.Require(t, 0)
	home := t.TempDir()
	t.Setenv("HOME", home)

	workdir := "/home/op/incremental"
	sessionID := "sess-incremental"
	projectDir := filepath.Join(home, ".claude", "projects", encodeForTest(workdir))

	writeParentTranscript(t, projectDir, sessionID, []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Agent","input":{"subagent_type":"fork"}}]}}`,
	})
	subagentsDir := filepath.Join(projectDir, sessionID, "subagents")
	fork1 := writeForkTranscript(t, subagentsDir, "01-fork.jsonl", []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/repo/x.go"}}]}}`,
	})
	fork2 := writeForkTranscript(t, subagentsDir, "02-fork.jsonl", []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/repo/y.go"}}]}}`,
	})

	c := New()
	seen := map[string]bool{fork1: true}
	audit, err := c.AuditForksIncremental(sessionID, workdir, seen)
	if err != nil {
		t.Fatalf("AuditForksIncremental() error: %v", err)
	}

	if len(audit.Forks) != 1 {
		t.Fatalf("len(Forks) = %d; want 1 (fork1 already seen)", len(audit.Forks))
	}
	if audit.Forks[0].TranscriptPath != fork2 {
		t.Errorf("Forks[0].TranscriptPath = %q; want %q (the unseen fork)", audit.Forks[0].TranscriptPath, fork2)
	}
}

// TestAuditForksIncremental_NilMapEquivalentToAuditForks proves a nil seenTranscripts map reports all fork transcripts.
// The result matches AuditForks.
func TestAuditForksIncremental_NilMapEquivalentToAuditForks(t *testing.T) {
	tiergate.Require(t, 0)
	home := t.TempDir()
	t.Setenv("HOME", home)

	workdir := "/home/op/incremental-nil"
	sessionID := "sess-incremental-nil"
	projectDir := filepath.Join(home, ".claude", "projects", encodeForTest(workdir))

	writeParentTranscript(t, projectDir, sessionID, []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Agent","input":{"subagent_type":"fork"}}]}}`,
	})
	subagentsDir := filepath.Join(projectDir, sessionID, "subagents")
	writeForkTranscript(t, subagentsDir, "01-fork.jsonl", []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/repo/x.go"}}]}}`,
	})
	writeForkTranscript(t, subagentsDir, "02-fork.jsonl", []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/repo/y.go"}}]}}`,
	})

	c := New()
	incremental, err := c.AuditForksIncremental(sessionID, workdir, nil)
	if err != nil {
		t.Fatalf("AuditForksIncremental(nil) error: %v", err)
	}
	full, err := c.AuditForks(sessionID, workdir)
	if err != nil {
		t.Fatalf("AuditForks() error: %v", err)
	}

	if len(incremental.Forks) != len(full.Forks) || len(incremental.Forks) != 2 {
		t.Fatalf("AuditForksIncremental(nil).Forks = %d entries, AuditForks().Forks = %d entries; want 2 each", len(incremental.Forks), len(full.Forks))
	}
	for i := range full.Forks {
		if incremental.Forks[i].TranscriptPath != full.Forks[i].TranscriptPath {
			t.Errorf("Forks[%d].TranscriptPath = %q; want %q (AuditForksIncremental(nil) must match AuditForks)", i, incremental.Forks[i].TranscriptPath, full.Forks[i].TranscriptPath)
		}
	}
}

// TestAuditForksIncremental_ParentFactsUnaffectedBySeenSet proves that the seen set filters only Forks.
// The parent facts (SpawnCalls, NamedSpawns, ParentWriteCalls, ParentWrites, ParentBashCommands) are read in full.
func TestAuditForksIncremental_ParentFactsUnaffectedBySeenSet(t *testing.T) {
	tiergate.Require(t, 0)
	home := t.TempDir()
	t.Setenv("HOME", home)

	workdir := "/home/op/incremental-parent-facts"
	sessionID := "sess-incremental-parent-facts"
	projectDir := filepath.Join(home, ".claude", "projects", encodeForTest(workdir))

	writeParentTranscript(t, projectDir, sessionID, []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Agent","input":{"subagent_type":"fork"}}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Write","input":{"file_path":"/repo/z.go","content":"z"}}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"go build ./..."}}]}}`,
	})
	subagentsDir := filepath.Join(projectDir, sessionID, "subagents")
	fork1 := writeForkTranscript(t, subagentsDir, "01-fork.jsonl", []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/repo/x.go"}}]}}`,
	})

	c := New()
	// Everything is seen: Forks empties out, but the parent facts must not.
	seen := map[string]bool{fork1: true}
	audit, err := c.AuditForksIncremental(sessionID, workdir, seen)
	if err != nil {
		t.Fatalf("AuditForksIncremental() error: %v", err)
	}

	if len(audit.Forks) != 0 {
		t.Errorf("len(Forks) = %d; want 0 (the only fork transcript is seen)", len(audit.Forks))
	}
	if audit.SpawnCalls != 1 {
		t.Errorf("SpawnCalls = %d; want 1", audit.SpawnCalls)
	}
	if audit.ParentWriteCalls != 1 || len(audit.ParentWrites) != 1 || audit.ParentWrites[0] != "/repo/z.go" {
		t.Errorf("ParentWriteCalls/ParentWrites = %d/%v; want 1/[\"/repo/z.go\"]", audit.ParentWriteCalls, audit.ParentWrites)
	}
	if len(audit.ParentBashCommands) != 1 || audit.ParentBashCommands[0] != "go build ./..." {
		t.Errorf("ParentBashCommands = %v; want [\"go build ./...\"]", audit.ParentBashCommands)
	}
}
