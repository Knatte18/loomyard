// logger_test.go verifies the default-Warn silence, the SetVerbosity
// thresholds driven by the -v/-vv flag, and the SetOutput test seam.

package logger

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// withCapturedOutput redirects the package sink to a fresh buffer for the
// duration of the test and restores the real os.Stderr sink at test end, so
// state does not leak between tests that run in the same process.
func withCapturedOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	SetOutput(&buf)
	t.Cleanup(func() {
		SetOutput(originalOut)
	})
	return &buf
}

// originalOut is the real stderr sink, captured once so withCapturedOutput's
// cleanup can restore it without hardcoding os.Stderr in every test.
var originalOut = out

func TestDefaultLevel_WarnIsSilentForInfoAndDebug(t *testing.T) {
	buf := withCapturedOutput(t)
	SetVerbosity(0)

	Info("info at default level")
	Debug("debug at default level")

	if buf.Len() != 0 {
		t.Errorf("Info/Debug at default level wrote %d bytes; want 0 (got %q)", buf.Len(), buf.String())
	}
}

func TestSetVerbosity_OneEnablesInfoNotDebug(t *testing.T) {
	buf := withCapturedOutput(t)
	SetVerbosity(1)

	Info("info at verbosity 1")
	if buf.Len() == 0 {
		t.Error("Info at verbosity 1 wrote 0 bytes; want a log line")
	}
	if !strings.Contains(buf.String(), "info at verbosity 1") {
		t.Errorf("Info output = %q; want it to contain the message", buf.String())
	}

	buf.Reset()
	Debug("debug at verbosity 1")
	if buf.Len() != 0 {
		t.Errorf("Debug at verbosity 1 wrote %d bytes; want 0 (got %q)", buf.Len(), buf.String())
	}
}

func TestSetVerbosity_TwoEnablesDebug(t *testing.T) {
	buf := withCapturedOutput(t)
	SetVerbosity(2)

	Debug("debug at verbosity 2")
	if buf.Len() == 0 {
		t.Error("Debug at verbosity 2 wrote 0 bytes; want a log line")
	}
	if !strings.Contains(buf.String(), "debug at verbosity 2") {
		t.Errorf("Debug output = %q; want it to contain the message", buf.String())
	}
}

func TestSetOutput_CapturesIntoCallerBuffer(t *testing.T) {
	buf := withCapturedOutput(t)
	SetVerbosity(1)

	Warn("warn goes to the injected buffer")

	if !strings.Contains(buf.String(), "warn goes to the injected buffer") {
		t.Errorf("SetOutput buffer = %q; want it to contain the Warn message", buf.String())
	}
}

func TestConfigureFromEnv_LogLevelRaisesThreshold(t *testing.T) {
	buf := withCapturedOutput(t)
	SetVerbosity(0)
	t.Setenv("LYX_LOG_LEVEL", "info")
	t.Cleanup(func() { SetVerbosity(0) })

	configureFromEnv()
	Info("info via LYX_LOG_LEVEL")

	if !strings.Contains(buf.String(), "info via LYX_LOG_LEVEL") {
		t.Errorf("output = %q; want it to contain the Info message once LYX_LOG_LEVEL=info is applied", buf.String())
	}
}

func TestConfigureFromEnv_UnsetLogLevelLeavesDefaultUntouched(t *testing.T) {
	buf := withCapturedOutput(t)
	SetVerbosity(0)
	t.Cleanup(func() { SetVerbosity(0) })

	configureFromEnv()
	Info("should stay silent")

	if buf.Len() != 0 {
		t.Errorf("output = %q; want 0 bytes when LYX_LOG_LEVEL is unset", buf.String())
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

// TestDualHandler_DebugReachesStderrOnlyNotDurableSink covers the
// dual-handler-fan-out decision's Debug exclusion: a Debug record reaches
// the stderr half once -vv is set, but durableHandler.Enabled never accepts
// below Info, so no sink file is created at all for a Debug-only sequence.
func TestDualHandler_DebugReachesStderrOnlyNotDurableSink(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)
	buf := withCapturedOutput(t)
	SetVerbosity(2)
	t.Cleanup(func() { SetVerbosity(0) })

	Debug("debug fan-out check")

	if !strings.Contains(buf.String(), "debug fan-out check") {
		t.Errorf("stderr output = %q; want it to contain the Debug message at -vv", buf.String())
	}
	if got := listSinkDirFiles(t, dir); len(got) != 0 {
		t.Errorf("listSinkDirFiles(dir) = %v; want no durable sink file from a Debug-only sequence", got)
	}
}

// TestDualHandler_InfoReachesDurableSinkAtEveryVerbosityStderrOnlyAtDashV
// covers the composite's OR-gate: an Info record reaches the durable sink
// even at the default Warn threshold (verbosity 0), where the stderr half
// alone would reject it, and reaches stderr only once -v (verbosity 1) or
// above is set -- the exact property the motivating incident depends on.
func TestDualHandler_InfoReachesDurableSinkAtEveryVerbosityStderrOnlyAtDashV(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)
	buf := withCapturedOutput(t)
	SetVerbosity(0)
	t.Cleanup(func() { SetVerbosity(0) })

	Info("info fan-out at default verbosity")

	if buf.Len() != 0 {
		t.Errorf("stderr output = %q; want 0 bytes for an Info record at the default Warn threshold", buf.String())
	}
	files := listSinkDirFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("listSinkDirFiles(dir) = %v; want exactly one durable sink file", files)
	}
	data, err := os.ReadFile(filepath.Join(dir, files[0]))
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = _, %v; want nil error", files[0], err)
	}
	if !strings.Contains(string(data), "info fan-out at default verbosity") {
		t.Errorf("durable sink content = %q; want it to contain the Info message even at the default verbosity", string(data))
	}

	buf.Reset()
	SetVerbosity(1)
	Info("info fan-out at -v")
	if !strings.Contains(buf.String(), "info fan-out at -v") {
		t.Errorf("stderr output = %q; want it to contain the Info message once -v is set", buf.String())
	}
}

// TestDualHandler_WarnReachesBothHalvesAtEveryVerbosity covers that Warn,
// the default threshold, always reaches both the stderr half and the
// durable sink regardless of -v/-vv.
func TestDualHandler_WarnReachesBothHalvesAtEveryVerbosity(t *testing.T) {
	for _, verbosity := range []int{0, 1, 2} {
		t.Run(fmt.Sprintf("verbosity=%d", verbosity), func(t *testing.T) {
			dir := t.TempDir()
			SetDurableSinkDir(dir)
			buf := withCapturedOutput(t)
			SetVerbosity(verbosity)
			t.Cleanup(func() { SetVerbosity(0) })

			Warn("warn fan-out check")

			if !strings.Contains(buf.String(), "warn fan-out check") {
				t.Errorf("stderr output = %q; want it to contain the Warn message at verbosity %d", buf.String(), verbosity)
			}
			files := listSinkDirFiles(t, dir)
			if len(files) != 1 {
				t.Fatalf("listSinkDirFiles(dir) = %v; want exactly one durable sink file", files)
			}
			data, err := os.ReadFile(filepath.Join(dir, files[0]))
			if err != nil {
				t.Fatalf("os.ReadFile(%q) = _, %v; want nil error", files[0], err)
			}
			if !strings.Contains(string(data), "warn fan-out check") {
				t.Errorf("durable sink content = %q; want it to contain the Warn message", string(data))
			}
		})
	}
}

// TestDualHandler_WarnWithUnarmedDurableSinkReachesStderrOnlyNoPanic covers
// the case where the durable sink never activates (SetDurableSinkDir("")
// falls through to the testing.Testing()/LYX_TRACE gate, keeping sinkOK
// false): a Warn call must still reach stderr, produce no error, and not
// panic -- durableWriter.Write silently discards the record rather than
// surfacing the unarmed sink as a failure.
func TestDualHandler_WarnWithUnarmedDurableSinkReachesStderrOnlyNoPanic(t *testing.T) {
	SetDurableSinkDir("")
	buf := withCapturedOutput(t)
	SetVerbosity(0)
	t.Cleanup(func() { SetVerbosity(0) })

	Warn("warn with unarmed durable sink")

	if !strings.Contains(buf.String(), "warn with unarmed durable sink") {
		t.Errorf("stderr output = %q; want it to contain the Warn message even with the durable sink unarmed", buf.String())
	}
}

// TestDualHandler_EveryLevelStampsCurrentTraceID covers Card 18's
// trace-stamping requirement end-to-end through the dual-handler wiring:
// every emitted line at every level -- stderr and durable sink alike --
// carries trace= matching TraceID()'s current value.
func TestDualHandler_EveryLevelStampsCurrentTraceID(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)
	buf := withCapturedOutput(t)
	SetVerbosity(2)
	t.Cleanup(func() { SetVerbosity(0) })

	wantTrace := "trace=" + TraceID()

	Debug("debug trace stamp check")
	Info("info trace stamp check")
	Warn("warn trace stamp check")

	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if !strings.Contains(line, wantTrace) {
			t.Errorf("stderr line = %q; want it to contain %q", line, wantTrace)
		}
	}

	files := listSinkDirFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("listSinkDirFiles(dir) = %v; want exactly one durable sink file", files)
	}
	data, err := os.ReadFile(filepath.Join(dir, files[0]))
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = _, %v; want nil error", files[0], err)
	}
	// The header line and every subsequent Info/Warn record (Debug never
	// reaches the durable sink) all carry trace=.
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if !strings.Contains(line, wantTrace) {
			t.Errorf("durable sink line = %q; want it to contain %q", line, wantTrace)
		}
	}
}

// TestWriteDurable_ConcurrentWarnCallsProduceOneFileAndOneTruncationMarker
// pins the concurrency-contract decision: concurrent Warn calls across many
// goroutines, run under go test -race, must still open exactly one sink
// file (pins sinkOnce) and, when their combined bytes cross the size cap,
// emit exactly one truncation-marker line (pins writeDurable's
// mutex-guarded counter+cap-check+marker+write critical section).
func TestWriteDurable_ConcurrentWarnCallsProduceOneFileAndOneTruncationMarker(t *testing.T) {
	dir := t.TempDir()
	SetDurableSinkDir(dir)
	withCapturedOutput(t)
	SetVerbosity(0)
	t.Cleanup(func() { SetVerbosity(0) })

	const goroutines = 20
	// 512 KiB per call, two calls per goroutine: 20 MiB combined, well past
	// the 8 MiB cap, so the crossing is guaranteed to happen under real
	// concurrent contention rather than a single sequential writer.
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

	data, err := os.ReadFile(filepath.Join(dir, files[0]))
	if err != nil {
		t.Fatalf("os.ReadFile(%q) = _, %v; want nil error", files[0], err)
	}
	if got := countMarkerLines(data); got != 1 {
		t.Errorf("countMarkerLines(data) = %d; want exactly 1 truncation marker line even under concurrent writers crossing the cap", got)
	}
}
