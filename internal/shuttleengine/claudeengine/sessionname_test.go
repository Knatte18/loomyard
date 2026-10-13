package claudeengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
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

// countNameReadLines counts the captured could-not-read lines at level.
func countNameReadLines(output, level string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "level="+level) && strings.Contains(line, "claudeengine: could not read the session name") {
			count++
		}
	}
	return count
}

// replaceTranscriptWithDirectory makes session sid's transcript read fail with something other than not-exist.
func replaceTranscriptWithDirectory(t *testing.T, workdir, sid string) {
	t.Helper()
	dir, _ := claudeProjectDirFor(workdir)
	path := filepath.Join(dir, sid+".jsonl")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// The tests below capture the process-global logger at Debug, so neither is parallel.
func TestSessionNameDrift_MissingTranscriptReportsNoneAndLogs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	buf := logcapture.Capture(t)
	logger.SetVerbosity(2)

	if New().SessionNameDrift("absent", filepath.Join(t.TempDir(), "work"), "tst:worker") {
		t.Error("a missing transcript reported drift")
	}
	if got := countNameReadLines(buf.String(), "DEBUG"); got != 1 {
		t.Errorf("could-not-read lines at level=DEBUG = %d, want 1; log:\n%s", got, buf.String())
	}
	if got := countNameReadLines(buf.String(), "WARN"); got != 0 {
		t.Errorf("could-not-read lines at level=WARN = %d, want none for a transcript that does not exist yet; log:\n%s", got, buf.String())
	}
}

func TestSessionNameDrift_UnreadableTranscriptReportsNoneAndLogs(t *testing.T) {
	workdir := writeSessionTranscript(t, "sid", titleLine("x"))
	replaceTranscriptWithDirectory(t, workdir, "sid")
	buf := logcapture.Capture(t)
	logger.SetVerbosity(2)

	claude := New()
	for range 2 {
		if claude.SessionNameDrift("sid", workdir, "tst:worker") {
			t.Error("an unreadable transcript reported drift")
		}
	}
	if got := countNameReadLines(buf.String(), "WARN"); got != 1 {
		t.Errorf("could-not-read lines at level=WARN after two reads of one session = %d, want 1; log:\n%s", got, buf.String())
	}
	if got := countNameReadLines(buf.String(), "DEBUG"); got != 1 {
		t.Errorf("could-not-read lines at level=DEBUG after two reads of one session = %d, want the repeat at 1; log:\n%s", got, buf.String())
	}

	workdir2 := writeSessionTranscript(t, "sid2", titleLine("x"))
	replaceTranscriptWithDirectory(t, workdir2, "sid2")
	if claude.SessionNameDrift("sid2", workdir2, "tst:worker") {
		t.Error("a second unreadable transcript reported drift")
	}
	if got := countNameReadLines(buf.String(), "WARN"); got != 2 {
		t.Errorf("could-not-read lines at level=WARN after a second session = %d, want 2; log:\n%s", got, buf.String())
	}
}

func TestRenameText(t *testing.T) {
	if got := New().RenameText("tst:slug:worker"); got != "/rename tst:slug:worker" {
		t.Errorf("RenameText = %q", got)
	}
}
