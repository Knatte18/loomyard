// logger_test.go verifies how records route to stderr and the durable sink by level and verbosity, the LYX_LOG_LEVEL and LYX_LOG_FILE environment seams, and the SetOutput test seam.
// No test in this file calls t.Parallel: each mutates process-global logger state (verbosity, the output writer, the durable sink, LYX_* environment variables) that the tests share.

package logger

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
)

// withCapturedOutput redirects output to a fresh buffer for the test duration.
func withCapturedOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	SetOutput(&buf)
	t.Cleanup(func() {
		SetOutput(originalOut)
	})
	return &buf
}

var originalOut = out

// TestLogging_RoutesRecordsByLevelAndVerbosity pins the dual-handler fan-out: Warn reaches stderr and the durable sink at every verbosity, Info reaches the durable sink always and stderr from -v, Debug reaches stderr only at -vv and never the durable sink.
// Every record a half receives carries the current trace ID, and a Warn with no durable sink armed still reaches stderr.
func TestLogging_RoutesRecordsByLevelAndVerbosity(t *testing.T) {
	tests := []struct {
		name        string
		emit        func(msg string, args ...any)
		verbosity   int
		unarmedSink bool
		wantStderr  bool
		wantDurable bool
	}{
		{name: "debug at default", emit: Debug, verbosity: 0, wantStderr: false, wantDurable: false},
		{name: "debug at -v", emit: Debug, verbosity: 1, wantStderr: false, wantDurable: false},
		{name: "debug at -vv", emit: Debug, verbosity: 2, wantStderr: true, wantDurable: false},
		{name: "info at default", emit: Info, verbosity: 0, wantStderr: false, wantDurable: true},
		{name: "info at -v", emit: Info, verbosity: 1, wantStderr: true, wantDurable: true},
		{name: "info at -vv", emit: Info, verbosity: 2, wantStderr: true, wantDurable: true},
		{name: "warn at default", emit: Warn, verbosity: 0, wantStderr: true, wantDurable: true},
		{name: "warn at -v", emit: Warn, verbosity: 1, wantStderr: true, wantDurable: true},
		{name: "warn at -vv", emit: Warn, verbosity: 2, wantStderr: true, wantDurable: true},
		{name: "warn with unarmed durable sink", emit: Warn, verbosity: 0, unarmedSink: true, wantStderr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.unarmedSink {
				SetDurableSinkDir("")
			} else {
				SetDurableSinkDir(dir)
			}
			buf := withCapturedOutput(t)
			SetVerbosity(tt.verbosity)
			t.Cleanup(func() { SetVerbosity(0) })
			wantTrace := "trace=" + TraceID()
			const message = "fan-out check"

			tt.emit(message)

			if tt.wantStderr {
				for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
					if !strings.Contains(line, wantTrace) {
						t.Errorf("stderr line = %q; want it to contain %q", line, wantTrace)
					}
				}
				if !strings.Contains(buf.String(), message) {
					t.Errorf("stderr output = %q; want it to contain the message", buf.String())
				}
			} else if buf.Len() != 0 {
				t.Errorf("stderr output = %q; want 0 bytes", buf.String())
			}

			if tt.unarmedSink {
				return
			}
			if !tt.wantDurable {
				if files := listSinkDirFiles(t, dir); len(files) != 0 {
					t.Errorf("listSinkDirFiles(dir) = %v; want no durable sink file", files)
				}
				return
			}
			for _, line := range strings.Split(strings.TrimRight(readSoleSinkFile(t, dir), "\n"), "\n") {
				if !strings.Contains(line, wantTrace) {
					t.Errorf("durable sink line = %q; want it to contain %q", line, wantTrace)
				}
			}
			if !strings.Contains(readSoleSinkFile(t, dir), message) {
				t.Errorf("durable sink content does not contain the message")
			}
		})
	}
}

func TestConfigureFromEnv_LogLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		wantInfo bool
	}{
		{name: "unset leaves the default untouched", level: "", wantInfo: false},
		{name: "info raises the threshold", level: "info", wantInfo: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := withCapturedOutput(t)
			SetVerbosity(0)
			t.Cleanup(func() { SetVerbosity(0) })
			if tt.level == "" {
				t.Setenv("LYX_LOG_LEVEL", "")
				os.Unsetenv("LYX_LOG_LEVEL")
			} else {
				t.Setenv("LYX_LOG_LEVEL", tt.level)
			}

			configureFromEnv()
			Info("info via LYX_LOG_LEVEL")

			if got := strings.Contains(buf.String(), "info via LYX_LOG_LEVEL"); got != tt.wantInfo {
				t.Errorf("Info emitted = %v with LYX_LOG_LEVEL=%q (output %q); want %v", got, tt.level, buf.String(), tt.wantInfo)
			}
		})
	}
}

func TestConfigureFromEnv_LogFileRedirectsOutput(t *testing.T) {
	SetVerbosity(1)
	t.Cleanup(func() {
		SetVerbosity(0)
		SetOutput(originalOut)
	})

	path := t.TempDir() + "/reed-trace.log"
	t.Setenv("LYX_LOG_FILE", path)

	configureFromEnv()
	Warn("warn routed to LYX_LOG_FILE")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read LYX_LOG_FILE: %v", err)
	}
	if !strings.Contains(string(data), "warn routed to LYX_LOG_FILE") {
		t.Errorf("file content = %q; want it to contain the Warn message", string(data))
	}
}

func TestConfigureFromEnv_UnopenableLogFileFallsBackToStderr(t *testing.T) {
	buf := withCapturedOutput(t)
	SetVerbosity(1)
	t.Cleanup(func() { SetVerbosity(0) })

	// A path under a directory that does not exist can never be opened.
	t.Setenv("LYX_LOG_FILE", t.TempDir()+"/no-such-dir/reed-trace.log")

	configureFromEnv()
	Warn("still goes to the captured sink")

	if !strings.Contains(buf.String(), "still goes to the captured sink") {
		t.Errorf("output = %q; want the pre-existing sink to keep receiving log lines when LYX_LOG_FILE cannot be opened", buf.String())
	}
}

// TestWriteDurable_ConcurrentWarnCallsProduceOneFileAndOneTruncationMarker verifies concurrent writes produce one file and one marker.
//
//testtiming:keep pins one sink file and one truncation marker under concurrent first writers, which the serial size-cap test does not exercise
func TestWriteDurable_ConcurrentWarnCallsProduceOneFileAndOneTruncationMarker(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)
	withCapturedOutput(t)
	SetVerbosity(0)
	t.Cleanup(func() { SetVerbosity(0) })

	const goroutines = 20
	payload := strings.Repeat("x", 512*1024)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 2; j++ {
				Warn("concurrent warn", "goroutine", n, "iteration", j, "payload", payload)
			}
		}(i)
	}
	wg.Wait()

	files := listSinkDirFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("listSinkDirFiles(dir) = %v; want exactly one sink file even under concurrent first-write races", files)
	}

	if got := countMarkerLines([]byte(readSoleSinkFile(t, dir))); got != 1 {
		t.Errorf("countMarkerLines(data) = %d; want exactly 1 truncation marker line even under concurrent writers crossing the cap", got)
	}
}
