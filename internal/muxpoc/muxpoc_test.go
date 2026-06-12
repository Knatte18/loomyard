package muxpoc

import (
	"os"
	"strings"
	"testing"
)

// TestSanitizeEnv_DropsClaudeCodeVars is the load-bearing test: the inherited Claude Code
// parent-session markers must be removed (they make a pane's claude a nested child and
// suppress transcript persistence), while unrelated vars survive.
func TestSanitizeEnv_DropsClaudeCodeVars(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDE_CODE_CHILD_SESSION", "1")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "abc")
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", "cli")
	t.Setenv("MUXPOC_KEEPME", "yes")

	got := sanitizeEnv()
	for _, kv := range got {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if key == "CLAUDECODE" || strings.HasPrefix(key, "CLAUDE_CODE_") {
			t.Errorf("sanitizeEnv leaked %q", key)
		}
	}
	if !containsKV(got, "MUXPOC_KEEPME=yes") {
		t.Errorf("sanitizeEnv dropped an unrelated var MUXPOC_KEEPME")
	}

	stripped := strippedEnvKeys()
	for _, want := range []string{"CLAUDECODE", "CLAUDE_CODE_CHILD_SESSION", "CLAUDE_CODE_SESSION_ID", "CLAUDE_CODE_ENTRYPOINT"} {
		if !contains(stripped, want) {
			t.Errorf("strippedEnvKeys missing %q (got %v)", want, stripped)
		}
	}
}

func TestSubst_LaunchAndResume(t *testing.T) {
	c := DefaultConfig()
	c.Claude = `C:\bin\claude.exe`
	if got := c.launchCmd("SID123"); got != `& 'C:\bin\claude.exe' --session-id SID123` {
		t.Errorf("launchCmd = %q", got)
	}
	if got := c.resumeCmd("SID123"); got != `& 'C:\bin\claude.exe' --resume SID123` {
		t.Errorf("resumeCmd = %q", got)
	}
}

func TestStateRoundtrip(t *testing.T) {
	dir := t.TempDir()
	in := State{Socket: "muxpoc-x", Session: "muxpoc", Width: 200, Height: 50,
		Panes: []PaneState{
			{Role: "main", SessionID: "sid-main", CWD: dir, PaneID: "%1"},
			{Role: "reviewer", SessionID: "sid-rev", CWD: dir, PaneID: "%3"},
		}}
	if err := saveState(dir, in); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	out, have, err := loadState(dir)
	if err != nil || !have {
		t.Fatalf("loadState: have=%v err=%v", have, err)
	}
	if len(out.Panes) != 2 || out.Panes[1].SessionID != "sid-rev" || out.Session != "muxpoc" {
		t.Errorf("roundtrip mismatch: %+v", out)
	}

	// Missing file → have=false, no error.
	if _, have, err := loadState(t.TempDir()); have || err != nil {
		t.Errorf("expected no state in empty dir, got have=%v err=%v", have, err)
	}

	if err := clearState(dir); err != nil {
		t.Errorf("clearState: %v", err)
	}
	if _, err := os.Stat(statePath(dir)); !os.IsNotExist(err) {
		t.Errorf("state file should be gone after clearState")
	}
}

func TestSocketFor_StableAndSanitized(t *testing.T) {
	a := socketFor(`C:\Code\mhgo\wts\my-task`)
	b := socketFor(`C:\Code\mhgo\wts\my-task`)
	if a != b {
		t.Errorf("socketFor not stable: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "muxpoc-") {
		t.Errorf("socketFor missing prefix: %q", a)
	}
	if strings.ContainsAny(a, `\/: .`) {
		t.Errorf("socketFor not sanitized: %q", a)
	}
}

func TestNewUUID_Format(t *testing.T) {
	u, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(u, "-")
	if len(parts) != 5 || len(u) != 36 {
		t.Errorf("uuid wrong shape: %q", u)
	}
	if parts[2][0] != '4' {
		t.Errorf("uuid not v4: %q", u)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func containsKV(ss []string, want string) bool { return contains(ss, want) }
