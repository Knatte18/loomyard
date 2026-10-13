// archive_test.go drives the pair-log archive over temporary directories with an injected clock.

package pairteardown

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/verifytree"
)

var archiveNow = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// archiveStamp returns the entry name of an archive made age before archiveNow.
func archiveStamp(age time.Duration) string {
	return archiveNow.Add(-age).Format(archiveStampLayout)
}

// writeArchiveFile writes content to path, creating its directory.
func writeArchiveFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

// requireArchiveEntries asserts the names under dir are exactly want, in order.
func requireArchiveEntries(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadDir(%s) error = %v", dir, err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("entries of %s = %v; want %v", dir, got, want)
	}
}

// requireArchiveFile asserts path holds content.
func requireArchiveFile(t *testing.T, path, content string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != content {
		t.Errorf("ReadFile(%s) = %q, %v; want %q", path, got, err, content)
	}
}

// countArchiveLogLines counts the lines of log at level that contain text.
func countArchiveLogLines(log, level, text string) int {
	count := 0
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, "level="+level) && strings.Contains(line, text) {
			count++
		}
	}
	return count
}

// pairWithLogs returns a Teardown over a temp hub whose pair slug holds a trace log, a verify log and a verify file that is no log.
func pairWithLogs(t *testing.T, slug string) (td *Teardown, traceLog string) {
	t.Helper()
	td = &Teardown{prime: &lyxcwd.Location{HubPath: t.TempDir(), AnchorRel: "."}}
	anchor := td.taskAnchor(slug)
	traceLog = filepath.Join(logger.LogsDirAt(anchor), "trace.log")
	writeArchiveFile(t, traceLog, "trace")
	writeArchiveFile(t, filepath.Join(verifytree.Dir(anchor), "verify-1.log"), "verify")
	writeArchiveFile(t, filepath.Join(verifytree.Dir(anchor), "record.json"), "{}")
	return td, traceLog
}

// canMakeUnreadable reports whether chmod 000 makes a file unreadable to this process.
func canMakeUnreadable() bool {
	return runtime.GOOS != "windows" && os.Geteuid() != 0
}

// TestLogArchive asserts the archive's write, prune and pair-teardown step: what lands in an entry, what is left alone, and what is logged.
// Rows that capture the process-global logger run serially; the others run in parallel.
func TestLogArchive(t *testing.T) {
	tests := []struct {
		name string
		// serial marks a row that captures the process-global logger.
		serial bool
		run    func(t *testing.T)
	}{
		{"a pair with trace and verify logs gets one entry with both subdirectories", false, func(t *testing.T) {
			dir := t.TempDir()
			traceLog := filepath.Join(dir, "src", "trace.log")
			verifyLog := filepath.Join(dir, "src", "verify-1.log")
			writeArchiveFile(t, traceLog, "trace")
			writeArchiveFile(t, verifyLog, "verify")
			archive := logArchive{root: filepath.Join(dir, "archive"), now: func() time.Time { return archiveNow }, removeAll: os.RemoveAll}

			entry, files, err := archive.write("slug", []archiveSource{{"trace", []string{traceLog}}, {"verify", []string{verifyLog}}})

			if err != nil || files != 2 || entry != filepath.Join(archive.root, "slug", archiveStamp(0)) {
				t.Fatalf("write() = %q, %d, %v; want the %s entry, 2 files and no error", entry, files, err, archiveStamp(0))
			}
			requireArchiveFile(t, filepath.Join(entry, "trace", "trace.log"), "trace")
			requireArchiveFile(t, filepath.Join(entry, "verify", "verify-1.log"), "verify")
			requireArchiveFile(t, traceLog, "trace")
			requireArchiveFile(t, verifyLog, "verify")
		}},
		{"a pair with neither source creates no entry and no slug directory", false, func(t *testing.T) {
			archive := logArchive{root: filepath.Join(t.TempDir(), "archive"), now: func() time.Time { return archiveNow }, removeAll: os.RemoveAll}

			entry, files, err := archive.write("slug", []archiveSource{{"trace", nil}, {"verify", nil}})

			if entry != "" || files != 0 || err != nil {
				t.Errorf("write() = %q, %d, %v; want no entry, no files and no error", entry, files, err)
			}
			requireArchiveEntries(t, archive.root)
		}},
		{"a copy failure leaves no entry and returns the error", false, func(t *testing.T) {
			dir := t.TempDir()
			present := filepath.Join(dir, "trace.log")
			writeArchiveFile(t, present, "trace")
			archive := logArchive{root: filepath.Join(dir, "archive"), now: func() time.Time { return archiveNow }, removeAll: os.RemoveAll}

			_, _, err := archive.write("slug", []archiveSource{{"trace", []string{present}}, {"verify", []string{filepath.Join(dir, "missing.log")}}})

			if err == nil {
				t.Error("write() error = nil; want the copy failure")
			}
			requireArchiveEntries(t, filepath.Join(archive.root, "slug"))
		}},
		{"two writes of one slug at the same time land in two entries", false, func(t *testing.T) {
			dir := t.TempDir()
			traceLog := filepath.Join(dir, "trace.log")
			writeArchiveFile(t, traceLog, "trace")
			archive := logArchive{root: filepath.Join(dir, "archive"), now: func() time.Time { return archiveNow }, removeAll: os.RemoveAll}

			first, _, firstErr := archive.write("slug", []archiveSource{{"trace", []string{traceLog}}})
			second, _, secondErr := archive.write("slug", []archiveSource{{"trace", []string{traceLog}}})

			if firstErr != nil || secondErr != nil || first == second {
				t.Fatalf("write() twice = %q, %v and %q, %v; want two distinct entries", first, firstErr, second, secondErr)
			}
			requireArchiveEntries(t, filepath.Join(archive.root, "slug"), archiveStamp(0), archiveStamp(0)+"-2")
		}},
		{"a prune removes entries past the retention and what they empty, and leaves the rest", false, func(t *testing.T) {
			root := t.TempDir()
			const day = 24 * time.Hour
			writeArchiveFile(t, filepath.Join(root, "a", archiveStamp(31*day), "trace", "old.log"), "old")
			writeArchiveFile(t, filepath.Join(root, "a", archiveStamp(29*day), "trace", "new.log"), "new")
			writeArchiveFile(t, filepath.Join(root, "a", "not-a-stamp", "keep.log"), "keep")
			writeArchiveFile(t, filepath.Join(root, "a", "plain-file"), "keep")
			writeArchiveFile(t, filepath.Join(root, "b", archiveStamp(31*day)+"-2", "trace", "old.log"), "old")
			archive := logArchive{root: root, now: func() time.Time { return archiveNow }, removeAll: os.RemoveAll}

			archive.prune()

			requireArchiveEntries(t, root, "a")
			requireArchiveEntries(t, filepath.Join(root, "a"), archiveStamp(29*day), "not-a-stamp", "plain-file")
		}},
		{"a prune logs a Warn for a removal failure, and none for a path already gone", true, func(t *testing.T) {
			buf := logcapture.CaptureVerbose(t)
			root := t.TempDir()
			writeArchiveFile(t, filepath.Join(root, "a", archiveStamp(31*24*time.Hour), "trace", "old.log"), "old")
			gone := fmt.Errorf("another teardown removed it: %w", fs.ErrNotExist)
			archive := logArchive{root: root, now: func() time.Time { return archiveNow }, removeAll: func(string) error { return gone }}

			archive.prune()

			if got := countArchiveLogLines(buf.String(), "WARN", ""); got != 0 {
				t.Errorf("WARN lines for a path already gone = %d; want 0\n%s", got, buf.String())
			}

			archive.removeAll = func(string) error { return errors.New("permission denied") }
			archive.prune()

			if got := countArchiveLogLines(buf.String(), "WARN", "could not remove an expired log archive path"); got != 1 {
				t.Errorf("WARN lines for a failed removal = %d; want 1\n%s", got, buf.String())
			}
		}},
		{"archiving a pair with logs leaves one entry and logs one Info line", true, func(t *testing.T) {
			buf := logcapture.CaptureVerbose(t)
			td, _ := pairWithLogs(t, "slug")

			td.archivePairLogs("slug")

			slugDir := filepath.Join(fabricengine.LogArchiveDir(td.prime.HubPath), "slug")
			entries, err := os.ReadDir(slugDir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("ReadDir(%s) = %v, %v; want one entry", slugDir, entries, err)
			}
			entry := filepath.Join(slugDir, entries[0].Name())
			requireArchiveFile(t, filepath.Join(entry, "trace", "trace.log"), "trace")
			requireArchiveFile(t, filepath.Join(entry, "verify", "verify-1.log"), "verify")
			requireArchiveEntries(t, filepath.Join(entry, "verify"), "verify-1.log")
			var line string
			for _, candidate := range strings.Split(buf.String(), "\n") {
				if strings.Contains(candidate, "level=INFO") && strings.Contains(candidate, "archived the pair's logs") {
					line = candidate
				}
			}
			if got := countArchiveLogLines(buf.String(), "INFO", "archived the pair's logs"); got != 1 || !strings.Contains(line, entry) || !strings.Contains(line, "files=2") {
				t.Errorf("INFO lines = %d, line %q; want one naming %s and files=2\n%s", got, line, entry, buf.String())
			}
		}},
		{"archiving a pair whose trace file is unreadable leaves no entry and logs one Warn", true, func(t *testing.T) {
			if !canMakeUnreadable() {
				t.Skip("an unreadable file stays readable here")
			}
			buf := logcapture.CaptureVerbose(t)
			td, traceLog := pairWithLogs(t, "slug")
			if err := os.Chmod(traceLog, 0); err != nil {
				t.Fatalf("Chmod() error = %v", err)
			}

			td.archivePairLogs("slug")

			requireArchiveEntries(t, filepath.Join(fabricengine.LogArchiveDir(td.prime.HubPath), "slug"))
			if got := countArchiveLogLines(buf.String(), "WARN", ""); got != 1 {
				t.Errorf("WARN lines = %d; want 1\n%s", got, buf.String())
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.serial {
				t.Parallel()
			}
			tt.run(t)
		})
	}
}
