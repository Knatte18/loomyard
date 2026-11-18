// reads_test.go covers locating a worktree's sessions through the engines' read capabilities.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestLocateSessions_ListedAccount pins that a session run on a listed non-default Claude account is counted.
// The session is counted through the readers built with the hub's dirs.
// It sets HOME and CLAUDE_CONFIG_DIR, so it does not run in parallel.
func TestLocateSessions_ListedAccount(t *testing.T) {
	tiergate.Require(t, 0)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	board, anchor, accountDir := t.TempDir(), t.TempDir(), t.TempDir()
	machineFile := configengine.MachineFile(board, "claude")
	if err := os.MkdirAll(filepath.Dir(machineFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(machineFile, []byte("accounts:\n  - {name: work, config_dir: "+accountDir+", weight: 1}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(accountDir, "projects", "-hub-task")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "main.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reads := ReadSet(shuttleengines.RecordReaders(configengine.Dirs{Board: board, Anchor: anchor}))
	located, err := locateSessions(reads, "/hub/task")
	if err != nil {
		t.Fatalf("locateSessions() error: %v", err)
	}
	if len(located) != 1 || located[0].engine != "claude" || located[0].session.ID != "main.jsonl" {
		t.Errorf("locateSessions() = %+v; want the one claude session main.jsonl", located)
	}
}
