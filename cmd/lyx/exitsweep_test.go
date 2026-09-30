// exitsweep_test.go covers notifyExitAndSweep's arm-gated sweep and exitSweepBounds' bounds choice.
// Every case uses t.TempDir() fixtures and a redirected sink, so nothing arms a cwd-anchored sink.

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loggerconfig"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

const exitSweepDeadPID = 999999999

// redirectSink points the sink at a fresh temp dir, captures the stderr half, and restores both on cleanup.
func redirectSink(t *testing.T) (dir string, warnings *bytes.Buffer) {
	t.Helper()
	dir = t.TempDir()
	warnings = &bytes.Buffer{}
	logger.SetOutput(warnings)
	logger.SetDurableSinkDir(dir)
	t.Cleanup(func() {
		logger.SetDurableSinkDir("")
		logger.SetOutput(os.Stderr)
	})
	return dir, warnings
}

// seedAgedDeadTrace writes a dead-pid trace last active 15 days ago and returns its path.
func seedAgedDeadTrace(t *testing.T, dir string) string {
	t.Helper()
	ts := time.Now().Add(-15 * 24 * time.Hour)
	path := filepath.Join(dir, fmt.Sprintf("trace-%s-%016x-%d.log", ts.UTC().Format("20060102T150405Z"), 1, exitSweepDeadPID))
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if err := os.Chtimes(path, ts, ts); err != nil {
		t.Fatalf("Chtimes(%s) = %v", path, err)
	}
	return path
}

func writeLoggerYAML(t *testing.T, anchor, content string) {
	t.Helper()
	path := loggerconfig.ConfigPath(anchor)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func dirEntryCount(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s) = %v", dir, err)
	}
	return len(entries)
}

const invalidLoggerYAML = "trace_retention_count: 0\ntrace_retention_days: 14\n"

func TestNotifyExitAndSweep_ZeroCodeWithoutRecordDoesNotSweep(t *testing.T) {
	dir, _ := redirectSink(t)
	aged := seedAgedDeadTrace(t, dir)

	notifyExitAndSweep(0)

	if _, err := os.Stat(aged); err != nil {
		t.Errorf("aged trace = %v; want it to survive a process whose sink never armed", err)
	}
	if got := dirEntryCount(t, dir); got != 1 {
		t.Errorf("dir holds %d files; want 1 (no new trace file)", got)
	}
}

func TestNotifyExitAndSweep_NonZeroCodeArmsAndSweeps(t *testing.T) {
	dir, _ := redirectSink(t)
	aged := seedAgedDeadTrace(t, dir)

	notifyExitAndSweep(1)

	if _, err := os.Stat(aged); !os.IsNotExist(err) {
		t.Errorf("aged trace stat err = %v; want it deleted", err)
	}
	own := logger.CurrentSinkArmState()
	if !own.Armed {
		t.Fatalf("sink not armed after a non-zero exit")
	}
	if got := dirEntryCount(t, dir); got != 1 {
		t.Errorf("dir holds %d files; want only this process's own trace", got)
	}
}

func TestNotifyExitAndSweep_ZeroCodeAfterInfoRecordSweeps(t *testing.T) {
	dir, _ := redirectSink(t)
	aged := seedAgedDeadTrace(t, dir)
	logger.Info("exit sweep test record")

	notifyExitAndSweep(0)

	if _, err := os.Stat(aged); !os.IsNotExist(err) {
		t.Errorf("aged trace stat err = %v; want it deleted", err)
	}
}

func TestExitSweepBounds_RedirectedIgnoresConfig(t *testing.T) {
	_, warnings := redirectSink(t)
	anchor := t.TempDir()
	writeLoggerYAML(t, anchor, invalidLoggerYAML)

	got := exitSweepBounds(logger.SinkArmState{Armed: true, Redirected: true, Dir: t.TempDir(), AnchorPath: anchor})

	if got != logger.DefaultRetentionBounds() {
		t.Errorf("bounds = %+v; want defaults", got)
	}
	if warnings.Len() != 0 {
		t.Errorf("stderr = %q; want no Warn", warnings.String())
	}
}

func TestExitSweepBounds_ValidConfigWins(t *testing.T) {
	_, warnings := redirectSink(t)
	anchor := t.TempDir()
	writeLoggerYAML(t, anchor, "trace_retention_count: 7\ntrace_retention_days: 3\n")

	got := exitSweepBounds(logger.SinkArmState{Armed: true, Dir: t.TempDir(), AnchorPath: anchor})

	want := logger.RetentionBounds{Count: 7, MaxAge: 3 * 24 * time.Hour}
	if got != want {
		t.Errorf("bounds = %+v; want %+v", got, want)
	}
	if warnings.Len() != 0 {
		t.Errorf("stderr = %q; want no Warn", warnings.String())
	}
}

func TestExitSweepBounds_InvalidConfigWarnsOnceAndDefaults(t *testing.T) {
	_, warnings := redirectSink(t)
	anchor := t.TempDir()
	writeLoggerYAML(t, anchor, invalidLoggerYAML)

	got := exitSweepBounds(logger.SinkArmState{Armed: true, Dir: t.TempDir(), AnchorPath: anchor})

	if got != logger.DefaultRetentionBounds() {
		t.Errorf("bounds = %+v; want defaults", got)
	}
	if n := strings.Count(warnings.String(), "level=WARN"); n != 1 {
		t.Errorf("Warn records = %d; want 1; stderr = %q", n, warnings.String())
	}
	if !strings.Contains(warnings.String(), loggerconfig.ConfigPath(anchor)) {
		t.Errorf("stderr = %q; want it to name %q", warnings.String(), loggerconfig.ConfigPath(anchor))
	}
}

func TestExitSweepBounds_AnchorWithoutLyxDefaultsSilently(t *testing.T) {
	_, warnings := redirectSink(t)
	anchor := t.TempDir()
	if _, err := os.Stat(filepath.Join(anchor, lyxdirs.LyxDirName)); err == nil {
		t.Fatalf("fixture anchor unexpectedly holds %s", lyxdirs.LyxDirName)
	}

	got := exitSweepBounds(logger.SinkArmState{Armed: true, Dir: t.TempDir(), AnchorPath: anchor})

	if got != logger.DefaultRetentionBounds() {
		t.Errorf("bounds = %+v; want defaults", got)
	}
	if n := strings.Count(warnings.String(), "level=WARN"); n != 0 {
		t.Errorf("Warn records = %d; want 0; stderr = %q", n, warnings.String())
	}
}
