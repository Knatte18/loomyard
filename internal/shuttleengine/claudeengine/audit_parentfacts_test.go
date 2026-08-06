// audit_parentfacts_test.go covers the parent-session write/bash fact
// extraction AuditForks populates onto ForkAudit: ParentWriteCalls,
// ParentWrites, and ParentBashCommands. Fixture transcripts are written
// directly with os.WriteFile (Tier 1: no fixture files, no subprocess) rather
// than reusing audit_test.go's testdata fixtures, since this file's cases are
// specific to the parent-fact fields those fixtures do not exercise.

package claudeengine

import (
	"os"
	"path/filepath"
	"testing"
)

// writeParentTranscript writes lines (already-JSON transcript lines) as a
// newline-joined parent transcript at projectDir/sessionID.jsonl, creating
// projectDir as needed.
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

// TestAuditForks_ParentWritesAndBashInOrder proves a parent transcript's own
// Write/Edit/NotebookEdit and Bash tool_use blocks populate
// ParentWriteCalls/ParentWrites/ParentBashCommands in transcript order,
// alongside the existing SpawnCalls/NamedSpawns facts — and that the
// missing-subagents-dir zero-fork finding still carries these fields.
func TestAuditForks_ParentWritesAndBashInOrder(t *testing.T) {
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
	// Deliberately no subagents/ directory: this is the missing-subagents-dir
	// zero-fork path, proven elsewhere to still populate SpawnCalls/NamedSpawns —
	// here it must also carry the new parent-fact fields.

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

// TestAuditForks_ParentWriteMissingPathKeyCountedNotPanicked proves a
// Write-family tool_use block whose input carries neither file_path nor
// notebook_path does not panic — it still increments ParentWriteCalls, but
// contributes no entry to ParentWrites.
func TestAuditForks_ParentWriteMissingPathKeyCountedNotPanicked(t *testing.T) {
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

// TestAuditForks_ExistingZeroForkAndForkFactsUnchanged proves this batch's
// parent-fact additions leave the pre-existing zero-fork and fork-report
// behaviour untouched: a parent transcript with no writes/bash still reports
// empty ParentWrites/ParentBashCommands (not nil-vs-empty confusion) and the
// existing SpawnCalls/NamedSpawns/Forks facts are unaffected.
func TestAuditForks_ExistingZeroForkAndForkFactsUnchanged(t *testing.T) {
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
