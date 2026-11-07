// trace_test.go covers trace.go's mint, adopt and export precedence.
// It covers LYM_TRACE_ID adoption, the minted value's shape, and empty or whitespace-only values, which count as unset.
// It also covers the lazy TraceID() path and the environment that MintOrAdoptAndExport gives spawned children.
// No test in this file calls t.Parallel:
// each resets package-level trace state and sets LYM_TRACE_ID in the process environment.

package logger

import (
	"os"
	"regexp"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
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

// TestTraceID_AdoptsOnlyMintedAlphabetValuesElseMints pins the adoption rule:
// LYM_TRACE_ID is kept only when it is 16 lowercase hex digits, and any other value is replaced by a mint.
// An adopted value is interpolated into a trace filename that filepath.Join cleans.
// Separators or ".." in it would write outside the logs directory.
// A value that stays inside the directory but is not hex is never matched by `traceFilePattern` in retention.go.
// Sweep can then neither rank nor remove its file, so the logs directory grows without bound.
//
// Both entry points are covered per row, because MintOrAdoptAndExport re-exports what it resolved:
// validating only one would leave the other passing a malformed value on to every spawned child.
// A valid value is adopted verbatim.
// An unset, empty or whitespace-only variable mints, and so does a rejected value.
// TraceID's lazy path mints without a prior MintOrAdoptAndExport call.
func TestTraceID_AdoptsOnlyMintedAlphabetValuesElseMints(t *testing.T) {
	tiergate.Require(t, 0)
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
			t.Setenv("LYM_TRACE_ID", tt.value)
			if tt.unset {
				os.Unsetenv("LYM_TRACE_ID")
			}

			got := TraceID()

			assertTraceIDAdoption(t, "TraceID", tt.value, got, tt.wantAdopt)
		})
		t.Run(tt.name+"/MintOrAdoptAndExport", func(t *testing.T) {
			resetTraceState(t)
			t.Setenv("LYM_TRACE_ID", tt.value)
			if tt.unset {
				os.Unsetenv("LYM_TRACE_ID")
			}

			got := MintOrAdoptAndExport()

			assertTraceIDAdoption(t, "MintOrAdoptAndExport", tt.value, got, tt.wantAdopt)
			// The re-export is the propagation vector R4-09 turns on: a rejected value must
			// not survive in the environment a spawned child inherits.
			if envVal := os.Getenv("LYM_TRACE_ID"); envVal != got {
				t.Errorf("os.Getenv(LYM_TRACE_ID) = %q after MintOrAdoptAndExport() = %q; want them equal", envVal, got)
			}
		})
	}

	t.Run("NewTraceID mints without touching the process identity", func(t *testing.T) {
		resetTraceState(t)
		t.Setenv("LYM_TRACE_ID", "deadbeefcafef00d")
		own := TraceID()

		first, second := NewTraceID(), NewTraceID()

		for _, got := range []string{first, second} {
			if !traceIDPattern.MatchString(got) {
				t.Errorf("NewTraceID() = %q; want 16 lowercase hex characters", got)
			}
		}
		if first == second {
			t.Errorf("two NewTraceID() calls both returned %q; want distinct values", first)
		}
		if got := TraceID(); got != own {
			t.Errorf("TraceID() = %q after NewTraceID(); want it unchanged at %q", got, own)
		}
		if got := os.Getenv("LYM_TRACE_ID"); got != "deadbeefcafef00d" {
			t.Errorf("LYM_TRACE_ID = %q after NewTraceID(); want it unchanged", got)
		}
	})
}

// assertTraceIDAdoption checks got against value: when adoption was expected, got must equal value verbatim;
// otherwise got must be a freshly minted 16-lowercase-hex identity that is not value.
func assertTraceIDAdoption(t *testing.T, entryPoint, value, got string, wantAdopt bool) {
	t.Helper()
	if wantAdopt {
		if got != value {
			t.Errorf("%s() with LYM_TRACE_ID=%q = %q; want the value adopted verbatim", entryPoint, value, got)
		}
		return
	}
	if got == value {
		t.Errorf("%s() with LYM_TRACE_ID=%q = %q; want a freshly minted value, not the rejected one", entryPoint, value, got)
	}
	if !traceIDHexPattern.MatchString(got) {
		t.Errorf("%s() with LYM_TRACE_ID=%q = %q; want 16 lowercase hex characters", entryPoint, value, got)
	}
}
