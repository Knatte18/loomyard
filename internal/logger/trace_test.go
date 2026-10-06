// trace_test.go covers trace.go's mint/adopt/export precedence: LYX_TRACE_ID adoption, minted-value shape, empty/whitespace-only treated as unset, the lazy TraceID() path, and the env-propagation contract MintOrAdoptAndExport promises spawned children.
// No test in this file calls t.Parallel: each resets package-level trace state and sets LYX_TRACE_ID in the process environment.

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

// TestTraceID_AdoptsOnlyMintedAlphabetValuesElseMints is the regression guard for the R4 review's R4-09: LYX_TRACE_ID was adopted verbatim, and sink.go interpolates the adopted value into a trace filename that filepath.Join then CLEANS -- so a value carrying separators and ".." walked the trace write out of the logs directory (LYX_TRACE_ID='ci-run/../../pwned' landed a file one level above .lyx/logs, and a longer chain escapes the state directory outright).
// A non-hex value that stays inside the directory is no better: retention.go's traceFilePattern never matches it, so Sweep can neither rank nor remove the file and the logs directory grows without bound.
//
// Both entry points are covered per row, because MintOrAdoptAndExport re-exports what it resolved:
// validating only one of the two would leave the other laundering a malformed value into every spawned child.
// A valid value is adopted verbatim; an unset, empty or whitespace-only variable mints, as does a rejected value, and TraceID's lazy path mints with no prior MintOrAdoptAndExport call.
func TestTraceID_AdoptsOnlyMintedAlphabetValuesElseMints(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantAdopt bool
		unset     bool
	}{
		{name: "Unset", unset: true},
		{name: "Empty", value: ""},
		{name: "WhitespaceOnly", value: "   \t  "},
		{name: "PathTraversal", value: "ci-run/../../pwned"},
		{name: "BareSeparator", value: "ci/run"},
		{name: "WindowsSeparator", value: `ci\run`},
		{name: "ParentDirectory", value: ".."},
		{name: "NonHexCharacters", value: "not-a-hex-trace!"},
		{name: "UppercaseHex", value: "DEADBEEFCAFEF00D"},
		{name: "TooShort", value: "deadbeef"},
		{name: "TooLong", value: "deadbeefcafef00d00"},
		{name: "ValidAdoption", value: "deadbeefcafef00d", wantAdopt: true},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/TraceID", func(t *testing.T) {
			resetTraceState(t)
			t.Setenv("LYX_TRACE_ID", tt.value)
			if tt.unset {
				os.Unsetenv("LYX_TRACE_ID")
			}

			got := TraceID()

			assertTraceIDAdoption(t, "TraceID", tt.value, got, tt.wantAdopt)
		})
		t.Run(tt.name+"/MintOrAdoptAndExport", func(t *testing.T) {
			resetTraceState(t)
			t.Setenv("LYX_TRACE_ID", tt.value)
			if tt.unset {
				os.Unsetenv("LYX_TRACE_ID")
			}

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
