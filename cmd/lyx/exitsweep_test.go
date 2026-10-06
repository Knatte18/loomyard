// exitsweep_test.go covers notifyExitAndSweep's arm-gated sweep and exitSweepBounds' bounds choice.
// Every case uses t.TempDir() fixtures and a redirected sink, so nothing arms a cwd-anchored sink.
// The logger's sink directory and output are process-global state, so no test here runs in parallel.

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

// TestNotifyExitAndSweep asserts the sweep runs only for a process whose sink armed: a non-zero exit arms it, a zero exit arms it only after an Info record, and a quiet zero exit leaves aged traces alone.
func TestNotifyExitAndSweep(t *testing.T) {
	tests := []struct {
		name       string
		code       int
		infoRecord bool
		wantSwept  bool
		wantArmed  bool
		// wantEntries is the file count the dir must hold afterwards; negative skips the check.
		wantEntries int
	}{
		{"zero code without a record does not sweep", 0, false, false, false, 1},
		{"non-zero code arms and sweeps", 1, false, true, true, 1},
		{"zero code after an info record sweeps", 0, true, true, false, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, _ := redirectSink(t)
			aged := seedAgedDeadTrace(t, dir)
			if tt.infoRecord {
				logger.Info("exit sweep test record")
			}

			notifyExitAndSweep(tt.code)

			_, err := os.Stat(aged)
			if tt.wantSwept && !os.IsNotExist(err) {
				t.Errorf("aged trace stat err = %v; want it deleted", err)
			}
			if !tt.wantSwept && err != nil {
				t.Errorf("aged trace = %v; want it to survive a process whose sink never armed", err)
			}
			if tt.wantArmed && !logger.CurrentSinkArmState().Armed {
				t.Errorf("sink not armed after a non-zero exit")
			}
			if tt.wantEntries >= 0 {
				if got := dirEntryCount(t, dir); got != tt.wantEntries {
					t.Errorf("dir holds %d files; want %d", got, tt.wantEntries)
				}
			}
		})
	}
}

// TestExitSweepBounds asserts which retention bounds the sweep uses and how many Warn records an unusable logger.yaml produces: a redirected sink ignores the config, a valid config wins, an invalid one warns once and defaults, and an anchor without _lyx defaults silently.
func TestExitSweepBounds(t *testing.T) {
	defaults := logger.DefaultRetentionBounds()
	tests := []struct {
		name string
		// yaml is the logger.yaml content written under the anchor; empty writes none.
		yaml       string
		redirected bool
		want       logger.RetentionBounds
		wantWarns  int
	}{
		{"redirected sink ignores the config", invalidLoggerYAML, true, defaults, 0},
		{"valid config wins", "trace_retention_count: 7\ntrace_retention_days: 3\n", false, logger.RetentionBounds{Count: 7, MaxAge: 3 * 24 * time.Hour}, 0},
		{"invalid config warns once and defaults", invalidLoggerYAML, false, defaults, 1},
		{"anchor without _lyx defaults silently", "", false, defaults, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, warnings := redirectSink(t)
			anchor := t.TempDir()
			if tt.yaml != "" {
				writeLoggerYAML(t, anchor, tt.yaml)
			} else if _, err := os.Stat(filepath.Join(anchor, lyxdirs.LyxDirName)); err == nil {
				t.Fatalf("fixture anchor unexpectedly holds %s", lyxdirs.LyxDirName)
			}

			got := exitSweepBounds(logger.SinkArmState{Armed: true, Redirected: tt.redirected, Dir: t.TempDir(), AnchorPath: anchor})

			if got != tt.want {
				t.Errorf("bounds = %+v; want %+v", got, tt.want)
			}
			if n := strings.Count(warnings.String(), "level=WARN"); n != tt.wantWarns {
				t.Errorf("Warn records = %d; want %d; stderr = %q", n, tt.wantWarns, warnings.String())
			}
			if tt.wantWarns == 0 && warnings.Len() != 0 {
				t.Errorf("stderr = %q; want no Warn", warnings.String())
			}
			if tt.wantWarns > 0 && !strings.Contains(warnings.String(), loggerconfig.ConfigPath(anchor)) {
				t.Errorf("stderr = %q; want it to name %q", warnings.String(), loggerconfig.ConfigPath(anchor))
			}
		})
	}
}
