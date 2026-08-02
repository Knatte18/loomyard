// trace_test.go covers trace.go's mint/adopt/export precedence: LYX_TRACE_ID
// adoption, minted-value shape, empty/whitespace-only treated as unset, the
// lazy TraceID() path, and the env-propagation contract MintOrAdoptAndExport
// promises spawned children.

package logger

import (
	"os"
	"regexp"
	"sync"
	"testing"
)

var traceIDHexPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// resetTraceState clears the package-level traceOnce/traceID state before a test.
func resetTraceState(t *testing.T) {
	t.Helper()
	savedID := traceID
	traceOnce = sync.Once{}
	traceID = ""
	t.Cleanup(func() {
		traceID = savedID
	})
}

func TestMintOrAdoptAndExport_AdoptsSetEnvVarVerbatim(t *testing.T) {
	resetTraceState(t)
	t.Setenv("LYX_TRACE_ID", "deadbeefcafef00d")

	got := MintOrAdoptAndExport()

	if got != "deadbeefcafef00d" {
		t.Errorf("MintOrAdoptAndExport() = %q; want %q (adopted verbatim)", got, "deadbeefcafef00d")
	}
}

func TestMintOrAdoptAndExport_UnsetEnvVarMints(t *testing.T) {
	resetTraceState(t)
	t.Setenv("LYX_TRACE_ID", "")
	os.Unsetenv("LYX_TRACE_ID")

	got := MintOrAdoptAndExport()

	if !traceIDHexPattern.MatchString(got) {
		t.Errorf("MintOrAdoptAndExport() = %q; want 16 lowercase hex characters", got)
	}
}

func TestMintOrAdoptAndExport_EmptyAndWhitespaceOnlyTreatedAsUnset(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"Empty", ""},
		{"WhitespaceOnly", "   \t  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetTraceState(t)
			t.Setenv("LYX_TRACE_ID", tt.value)

			got := MintOrAdoptAndExport()

			if !traceIDHexPattern.MatchString(got) {
				t.Errorf("MintOrAdoptAndExport() with LYX_TRACE_ID=%q = %q; want a minted 16-lowercase-hex value, not the raw env value", tt.value, got)
			}
		})
	}
}

func TestTraceID_LazyPathMintsWithNoPriorCall(t *testing.T) {
	resetTraceState(t)
	t.Setenv("LYX_TRACE_ID", "")
	os.Unsetenv("LYX_TRACE_ID")

	got := TraceID()

	if !traceIDHexPattern.MatchString(got) {
		t.Errorf("TraceID() with no prior MintOrAdoptAndExport call = %q; want 16 lowercase hex characters", got)
	}
}

func TestMintOrAdoptAndExport_PropagatesExportedValueToEnv(t *testing.T) {
	resetTraceState(t)
	t.Setenv("LYX_TRACE_ID", "")
	os.Unsetenv("LYX_TRACE_ID")

	got := MintOrAdoptAndExport()

	if envVal := os.Getenv("LYX_TRACE_ID"); envVal != got {
		t.Errorf("os.Getenv(LYX_TRACE_ID) = %q after MintOrAdoptAndExport() = %q; want them equal", envVal, got)
	}
}
