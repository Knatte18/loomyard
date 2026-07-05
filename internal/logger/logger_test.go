// logger_test.go verifies the default-Warn silence, the SetVerbosity
// thresholds driven by the -v/-vv flag, and the SetOutput test seam.

package logger

import (
	"bytes"
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
