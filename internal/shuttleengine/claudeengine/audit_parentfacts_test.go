// audit_parentfacts_test.go covers how AuditForks fills the parent-session facts of ForkAudit.
// The write and bash facts are ParentWriteCalls, ParentWrites, and ParentBashCommands.
// Each test writes its transcript with os.WriteFile, so it reads no fixture file and runs no subprocess.

package claudeengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeParentTranscript writes the JSON transcript lines, newline-joined, to projectDir/sessionID.jsonl.
// It creates projectDir when it does not exist.
func writeParentTranscript(t *testing.T, projectDir, sessionID string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", projectDir, err)
	}
	data := ""
	for _, line := range lines {
		data += line + "\n"
	}
	dest := filepath.Join(projectDir, sessionID+".jsonl")
	if err := os.WriteFile(dest, []byte(data), 0o644); err != nil {
		t.Fatalf("write %q: %v", dest, err)
	}
}

// TestAuditForks_ParentWritesAndBashInOrder checks the parent's own tool calls, in transcript order:
// Write, Edit and NotebookEdit fill ParentWriteCalls and ParentWrites, and Bash fills ParentBashCommands.
// It also checks that a session with no subagents directory, which yields zero forks, still reports these fields.
func TestAuditForks_ParentWritesAndBashInOrder(t *testing.T) {
	tiergate.Require(t, 0)
	home := t.TempDir()
	t.Setenv("HOME", home)

	workdir := "/home/op/parent-facts"
	sessionID := "sess-parent-facts"
	projectDir := filepath.Join(home, ".claude", "projects", encodeForTest(workdir))

	writeParentTranscript(t, projectDir, sessionID, []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Write","input":{"file_path":"/repo/a.go","content":"x"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"go test ./..."}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_02"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/repo/b.go","old_string":"a","new_string":"b"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_03"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"NotebookEdit","input":{"notebook_path":"/repo/nb.ipynb","new_source":"1+1"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_04"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"git status"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_05"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Made three edits and ran two commands."}]}}`,
	})
	// The test creates no subagents/ directory, so the audit takes the zero-fork path.
	// That path must still populate the parent-fact fields.

	c := New()
	audit, err := c.AuditForks(sessionID, workdir)
	if err != nil {
		t.Fatalf("AuditForks() error: %v", err)
	}

	if audit.ParentWriteCalls != 3 {
		t.Errorf("ParentWriteCalls = %d; want 3 (Write, Edit, NotebookEdit)", audit.ParentWriteCalls)
	}
	wantWrites := []string{"/repo/a.go", "/repo/b.go", "/repo/nb.ipynb"}
	if len(audit.ParentWrites) != len(wantWrites) {
		t.Fatalf("ParentWrites = %v; want %v", audit.ParentWrites, wantWrites)
	}
	for i, want := range wantWrites {
		if audit.ParentWrites[i] != want {
			t.Errorf("ParentWrites[%d] = %q; want %q", i, audit.ParentWrites[i], want)
		}
	}

	wantBash := []string{"go test ./...", "git status"}
	if len(audit.ParentBashCommands) != len(wantBash) {
		t.Fatalf("ParentBashCommands = %v; want %v", audit.ParentBashCommands, wantBash)
	}
	for i, want := range wantBash {
		if audit.ParentBashCommands[i] != want {
			t.Errorf("ParentBashCommands[%d] = %q; want %q", i, audit.ParentBashCommands[i], want)
		}
	}

	if audit.SpawnCalls != 0 || audit.NamedSpawns != 0 {
		t.Errorf("SpawnCalls/NamedSpawns = %d/%d; want 0/0 (no Agent tool_use in this transcript)", audit.SpawnCalls, audit.NamedSpawns)
	}
}

// TestAuditForks_ParentWriteMissingPathKeyCountedNotPanicked checks a Write-family tool_use block with no path key.
// The block must not panic, must count in ParentWriteCalls, and must add no ParentWrites entry.
func TestAuditForks_ParentWriteMissingPathKeyCountedNotPanicked(t *testing.T) {
	tiergate.Require(t, 0)
	home := t.TempDir()
	t.Setenv("HOME", home)

	workdir := "/home/op/missing-path-key"
	sessionID := "sess-missing-path"
	projectDir := filepath.Join(home, ".claude", "projects", encodeForTest(workdir))

	writeParentTranscript(t, projectDir, sessionID, []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Write","input":{"content":"no path key here"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Write","input":{"file_path":"/repo/c.go","content":"y"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_02"}]}}`,
	})

	c := New()
	audit, err := c.AuditForks(sessionID, workdir)
	if err != nil {
		t.Fatalf("AuditForks() error: %v", err)
	}

	if audit.ParentWriteCalls != 2 {
		t.Errorf("ParentWriteCalls = %d; want 2 (both Write blocks counted, even the one missing a path key)", audit.ParentWriteCalls)
	}
	if len(audit.ParentWrites) != 1 || audit.ParentWrites[0] != "/repo/c.go" {
		t.Errorf("ParentWrites = %v; want [\"/repo/c.go\"] (the path-less block contributes no entry)", audit.ParentWrites)
	}
}

// TestAuditForks_ExistingZeroForkAndForkFactsUnchanged checks a parent transcript that has no writes or Bash calls:
// ParentWrites and ParentBashCommands must come back empty,
// and the existing SpawnCalls, NamedSpawns and Forks facts must keep their values.
func TestAuditForks_ExistingZeroForkAndForkFactsUnchanged(t *testing.T) {
	tiergate.Require(t, 0)
	home := t.TempDir()
	t.Setenv("HOME", home)

	workdir := "/home/op/no-parent-facts"
	sessionID := "sess-no-parent-facts"
	projectDir := filepath.Join(home, ".claude", "projects", encodeForTest(workdir))

	writeParentTranscript(t, projectDir, sessionID, []string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Agent","input":{"subagent_type":"fork"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_01"}]}}`,
	})

	c := New()
	audit, err := c.AuditForks(sessionID, workdir)
	if err != nil {
		t.Fatalf("AuditForks() error: %v", err)
	}

	if audit.SpawnCalls != 1 || audit.NamedSpawns != 0 {
		t.Errorf("SpawnCalls/NamedSpawns = %d/%d; want 1/0", audit.SpawnCalls, audit.NamedSpawns)
	}
	if len(audit.Forks) != 0 {
		t.Errorf("len(Forks) = %d; want 0 (no subagents/ dir)", len(audit.Forks))
	}
	if audit.ParentWriteCalls != 0 {
		t.Errorf("ParentWriteCalls = %d; want 0", audit.ParentWriteCalls)
	}
	if len(audit.ParentWrites) != 0 {
		t.Errorf("ParentWrites = %v; want empty", audit.ParentWrites)
	}
	if len(audit.ParentBashCommands) != 0 {
		t.Errorf("ParentBashCommands = %v; want empty", audit.ParentBashCommands)
	}
}
