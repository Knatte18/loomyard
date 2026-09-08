// trace_test.go covers trace.go's mint/adopt/export precedence: LYX_TRACE_ID adoption, minted-value
// shape, empty/whitespace-only treated as unset, the lazy TraceID() path, and the env-propagation
// contract MintOrAdoptAndExport promises spawned children.

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

// TestTraceIDAdoption_RejectsValuesOutsideTheMintedAlphabet is the regression guard for the R4
// review's R4-09: LYX_TRACE_ID was adopted verbatim, and sink.go interpolates the adopted value into
// a trace filename that filepath.Join then CLEANS -- so a value carrying separators and ".." walked
// the trace write out of the logs directory (LYX_TRACE_ID='ci-run/../../pwned' landed a file one
// level above .lyx/logs, and a longer chain escapes the state directory outright). A non-hex value
// that stays inside the directory is no better: retention.go's traceFilePattern never matches it, so
// Sweep can neither rank nor remove the file and the logs directory grows without bound.
//
// Both entry points are covered per row, because MintOrAdoptAndExport re-exports what it resolved:
// validating only one of the two would leave the other laundering a malformed value into every
// spawned child.
func TestTraceIDAdoption_RejectsValuesOutsideTheMintedAlphabet(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantAdopt bool
	}{
		{"PathTraversal", "ci-run/../../pwned", false},
		{"BareSeparator", "ci/run", false},
		{"WindowsSeparator", `ci\run`, false},
		{"ParentDirectory", "..", false},
		{"NonHexCharacters", "not-a-hex-trace!", false},
		{"UppercaseHex", "DEADBEEFCAFEF00D", false},
		{"TooShort", "deadbeef", false},
		{"TooLong", "deadbeefcafef00d00", false},
		{"ValidAdoption", "deadbeefcafef00d", true},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/TraceID", func(t *testing.T) {
			resetTraceState(t)
			t.Setenv("LYX_TRACE_ID", tt.value)

			got := TraceID()

			assertTraceIDAdoption(t, "TraceID", tt.value, got, tt.wantAdopt)
		})
		t.Run(tt.name+"/MintOrAdoptAndExport", func(t *testing.T) {
			resetTraceState(t)
			t.Setenv("LYX_TRACE_ID", tt.value)

			got := MintOrAdoptAndExport()

			assertTraceIDAdoption(t, "MintOrAdoptAndExport", tt.value, got, tt.wantAdopt)
			// The re-export is the propagation vector R4-09 turns on: a rejected value must
			// not survive in the environment a spawned child inherits.
			if envVal := os.Getenv("LYX_TRACE_ID"); envVal != got {
				t.Errorf("os.Getenv(LYX_TRACE_ID) = %q after MintOrAdoptAndExport() = %q; want them equal", envVal, got)
			}
		})
	}
}

// assertTraceIDAdoption checks that got is exactly the adopted value when adoption was expected, and
// a freshly minted 16-lowercase-hex identity that is NOT the rejected value otherwise.
func assertTraceIDAdoption(t *testing.T, entryPoint, value, got string, wantAdopt bool) {
	t.Helper()
	if wantAdopt {
		if got != value {
			t.Errorf("%s() with LYX_TRACE_ID=%q = %q; want the value adopted verbatim", entryPoint, value, got)
		}
		return
	}
	if got == value {
		t.Errorf("%s() with LYX_TRACE_ID=%q = %q; want a freshly minted value, not the rejected one", entryPoint, value, got)
	}
	if !traceIDHexPattern.MatchString(got) {
		t.Errorf("%s() with LYX_TRACE_ID=%q = %q; want 16 lowercase hex characters", entryPoint, value, got)
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
