// logger_test.go verifies the default-Warn silence, the SetVerbosity
// thresholds driven by the -v/-vv flag, and the SetOutput test seam.

package logger

import (
	"bytes"
	"os"
	"strings"
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
