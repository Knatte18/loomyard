//go:build llm

// smoke_adopt_large_test.go measures what adopting a large real session costs: whether the resume launch reaches a live strand within `startup_timeout_s` and leaves no dialog on the pane.
// It is opt-in: LYX_SMOKE_ADOPT_SESSION names the id of an existing Claude Code session, and the test skips without it.
// It reads only a copy of that session's transcript, placed under a fresh session id in the sandbox's own Claude project directory, and never touches the original transcript or the session's registry entry.

package orchcli

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// adoptLargeSessionEnv names the environment variable carrying the id of the session to adopt a copy of.
const adoptLargeSessionEnv = "LYX_SMOKE_ADOPT_SESSION"

// resumeDialogMarkers are the texts a Claude Code resume or permission dialog shows, lower-cased.
var resumeDialogMarkers = []string{"do you want to", "resume from summary", "resume full session", "how would you like to continue"}

// TestSmokeOrch_AdoptLargeSession adopts a copy of the session named by LYX_SMOKE_ADOPT_SESSION and asserts startup within startup_timeout_s and no dialog left on the pane.
func TestSmokeOrch_AdoptLargeSession(t *testing.T) {
	sessionID := os.Getenv(adoptLargeSessionEnv)
	if sessionID == "" {
		t.Skipf("%s is unset; set it to the id of an existing session to adopt a copy of", adoptLargeSessionEnv)
	}
	f := newLiveFixture(t, smokeOrchConfig("compact", "bypass", 200000000, 100000000, 300), nil)

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("resolve the home directory: %v", err)
	}
	if h := os.Getenv("HOME"); h != "" {
		home = h
	}
	matches, _ := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", sessionID+".jsonl"))
	if len(matches) == 0 {
		t.Fatalf("no transcript for session %q under %s", sessionID, filepath.Join(home, ".claude", "projects"))
	}
	original, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read the transcript of session %q: %v", sessionID, err)
	}

	// The copy gets a fresh session id, so adopting it never meets the original session's registry entry.
	copyID := newSessionID(t)
	projectDir := filepath.Join(home, ".claude", "projects", encodeProjectDir(f.prime))
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("create %s: %v", projectDir, err)
	}
	copyPath := filepath.Join(projectDir, copyID+".jsonl")
	if err := os.WriteFile(copyPath, original, 0o600); err != nil {
		t.Fatalf("write the transcript copy %s: %v", copyPath, err)
	}
	t.Cleanup(func() { _ = os.Remove(copyPath) })

	started := time.Now()
	out, code := smokeRun(t, f.exe, f.prime, time.Duration(f.shuttleCfg.StartupTimeoutS+60)*time.Second, "orch", "start", "--adopt", copyID, "--no-attach")
	startup := time.Since(started)
	t.Logf("transcript size: %d bytes; adopt startup: %s; startup_timeout_s: %d", len(original), startup.Round(time.Millisecond), f.shuttleCfg.StartupTimeoutS)
	if code != 0 {
		t.Fatalf("orch start --adopt exited %d: %s", code, out)
	}
	if limit := time.Duration(f.shuttleCfg.StartupTimeoutS) * time.Second; startup > limit {
		t.Errorf("adopt startup took %s, over startup_timeout_s (%s)", startup, limit)
	}

	guid, _ := f.orchRun(t)
	pane, err := f.reed.CapturePane(guid)
	if err != nil {
		t.Fatalf("capture the orch pane: %v", err)
	}
	lower := strings.ToLower(pane)
	for _, marker := range resumeDialogMarkers {
		if strings.Contains(lower, marker) {
			t.Errorf("a dialog is left on the pane (%q):\n%s", marker, pane)
		}
	}

	f.stopOrch(t)
}

// newSessionID returns a random version-4 UUID in Claude Code's session id shape.
func newSessionID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("generate a session id: %v", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// encodeProjectDir encodes workdir the way Claude Code names its project directory: every non-alphanumeric byte becomes '-'.
func encodeProjectDir(workdir string) string {
	encoded := []byte(workdir)
	for i, b := range encoded {
		isAlnum := (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
		if !isAlnum {
			encoded[i] = '-'
		}
	}
	return string(encoded)
}
