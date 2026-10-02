package claudeengine

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
)

// writeSessionTranscript writes lines as session sid's transcript under a temp HOME and returns the workdir it belongs to.
func writeSessionTranscript(t *testing.T, sid string, lines ...string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	workdir := filepath.Join(t.TempDir(), "work")
	dir, err := claudeProjectDirFor(workdir)
	if err != nil {
		t.Fatalf("project dir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, sid+".jsonl"), []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return workdir
}

func titleLine(title string) string {
	return `{"type":"custom-title","customTitle":"` + title + `"}`
}

const otherLine = `{"type":"assistant","message":{"content":[]}}`

func TestSessionNameDrift(t *testing.T) {
	const want = "tst:slug:worker"
	tests := []struct {
		name  string
		lines []string
		drift bool
	}{
		{"latest title differs", []string{titleLine(want), otherLine, titleLine("other")}, true},
		{"earlier match superseded", []string{titleLine(want), titleLine("renamed")}, true},
		{"latest title matches", []string{titleLine("old"), titleLine(want), otherLine}, false},
		{"no title line", []string{otherLine}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workdir := writeSessionTranscript(t, "sid", tt.lines...)
			if got := New().SessionNameDrift("sid", workdir, want); got != tt.drift {
				t.Errorf("SessionNameDrift = %v, want %v", got, tt.drift)
			}
		})
	}
}

func TestSessionNameDrift_MissingTranscriptReportsNoneAndLogs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	if New().SessionNameDrift("absent", filepath.Join(t.TempDir(), "work"), "tst:worker") {
		t.Error("a missing transcript reported drift")
	}
	if !strings.Contains(buf.String(), "claudeengine: could not read the session name") {
		t.Errorf("log %q lacks the could-not-read line", buf.String())
	}
}

func TestSessionNameDrift_UnreadableTranscriptReportsNoneAndLogs(t *testing.T) {
	workdir := writeSessionTranscript(t, "sid", titleLine("x"))
	dir, _ := claudeProjectDirFor(workdir)
	path := filepath.Join(dir, "sid.jsonl")
	// A directory in place of the file makes the read fail with something other than not-exist.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	if New().SessionNameDrift("sid", workdir, "tst:worker") {
		t.Error("an unreadable transcript reported drift")
	}
	if !strings.Contains(buf.String(), "claudeengine: could not read the session name") {
		t.Errorf("log %q lacks the could-not-read line", buf.String())
	}
}

func TestRenameText(t *testing.T) {
	if got := New().RenameText("tst:slug:worker"); got != "/rename tst:slug:worker" {
		t.Errorf("RenameText = %q", got)
	}
}
