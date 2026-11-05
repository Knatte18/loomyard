// usage_test.go round-trips the usage records through their writers and the directory reader.

package shedrun

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// readUsageDir returns the files of dir as the name-to-bytes map ParseRunUsage takes.
func readUsageDir(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	files := map[string][]byte{}
	for _, entry := range entries {
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		files[entry.Name()] = content
	}
	return files
}

// TestUsageRecords pins the usage directory: every record round-trips through its writer and ParseRunUsage, a later write of a session id replaces the file and keeps an unknown key, a session id that is no single path segment is refused, and a file that does not decode is named.
func TestUsageRecords(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	int64Ptr := func(n int64) *int64 { return &n }
	floatPtr := func(f float64) *float64 { return &f }
	dir := filepath.Join(t.TempDir(), lymdirs.LymDirName, lymdirs.UsageDirName)

	first := SessionUsageRecord{
		SessionID: "sess-a", Row: "Implement", Role: "implementer", Model: "opus", Engine: "claude",
		InputTokens: int64Ptr(10), CacheWriteTokens: int64Ptr(20), CacheReadTokens: int64Ptr(30), OutputTokens: int64Ptr(40), CostUSD: floatPtr(1.5),
		Started: "2026-11-05T09:00:00Z", Ended: "2026-11-05T09:05:00Z",
		Models: []ModelUsage{{Model: "opus", InputTokens: 6, CacheReadTokens: 30, OutputTokens: 30, CostUSD: floatPtr(1)}, {Model: "haiku", InputTokens: 4, CacheWriteTokens: 20, OutputTokens: 10}},
	}
	unknown := SessionUsageRecord{SessionID: "sess-b", Row: "Review", Engine: "codex"}
	run := RunUsageRecord{Recipe: "darn", Profile: "docs", StoredRecord: true}
	landed := LandedSizeRecord{LinesAdded: 12, LinesDeleted: 3, FilesChanged: 2}
	for name, err := range map[string]error{
		"session a": WriteSessionUsage(dir, first),
		"session b": WriteSessionUsage(dir, unknown),
		"run":       WriteRunUsage(dir, run),
		"landed":    WriteLandedSize(dir, landed),
	} {
		if err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	got, err := ParseRunUsage(readUsageDir(t, dir))
	if err != nil {
		t.Fatalf("ParseRunUsage() error: %v", err)
	}
	want := RunUsage{Run: run, RunFound: true, Landed: landed, LandedFound: true, Sessions: []SessionUsageRecord{first, unknown}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseRunUsage() = %+v; want %+v", got, want)
	}

	// A resumed session's later end replaces its file, and a key an older or newer binary wrote stays.
	path := filepath.Join(dir, "session-sess-a.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	withExtra := strings.Replace(string(content), `"row"`, `"future_key": "kept",`+"\n  "+`"row"`, 1)
	if err := os.WriteFile(path, []byte(withExtra), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	later := first
	later.OutputTokens = int64Ptr(99)
	if err := WriteSessionUsage(dir, later); err != nil {
		t.Fatalf("rewrite session: %v", err)
	}
	rewritten, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(rewritten), `"future_key": "kept"`) {
		t.Errorf("rewritten record = %s; want the unknown key kept", rewritten)
	}
	got, err = ParseRunUsage(readUsageDir(t, dir))
	if err != nil {
		t.Fatalf("ParseRunUsage() after the rewrite: %v", err)
	}
	if len(got.Sessions) != 2 || *got.Sessions[0].OutputTokens != 99 {
		t.Errorf("sessions after the rewrite = %+v; want the two files, sess-a replaced", got.Sessions)
	}

	for _, id := range []string{"", "..", "a/b"} {
		if err := WriteSessionUsage(dir, SessionUsageRecord{SessionID: id}); err == nil {
			t.Errorf("WriteSessionUsage(session id %q) error = nil; want a refusal", id)
		}
	}

	_, err = ParseRunUsage(map[string][]byte{"run.json": []byte(`{"recipe":"darn"}`), "session-bad.json": []byte("{")})
	if err == nil || !strings.Contains(err.Error(), "session-bad.json") {
		t.Errorf("ParseRunUsage() error = %v; want one naming session-bad.json", err)
	}
}
