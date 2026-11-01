// tiergate_test.go pins how LYM_TEST_TIER parses into a selection and how Require skips a test outside it.
// The tests set LYM_TEST_TIER, a process-global variable, so none of them runs in parallel.

package tiergate_test

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recordingTB stands in for a *testing.T to observe whether Require skips or fails it.
// Fatal and Skipf end the calling goroutine, as the real ones do.
type recordingTB struct {
	testing.TB
	failed  bool
	skipped bool
	message string
}

func (r *recordingTB) Helper() {}

func (r *recordingTB) Fatal(args ...any) {
	r.failed = true
	r.message = fmt.Sprint(args...)
	runtime.Goexit()
}

func (r *recordingTB) Skipf(format string, args ...any) {
	r.skipped = true
	runtime.Goexit()
}

func TestSelected_ParsesTheVariable(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name    string
		value   string
		want    tiergate.Selection
		wantErr bool
	}{
		{"Unset", "", tiergate.Selection{Low: 0, High: 0}, false},
		{"OneTier", "2", tiergate.Selection{Low: 2, High: 2}, false},
		{"Range", "0-2", tiergate.Selection{Low: 0, High: 2}, false},
		{"Spaces", " 1 ", tiergate.Selection{Low: 1, High: 1}, false},
		{"Word", "tier1", tiergate.Selection{}, true},
		{"DescendingRange", "2-0", tiergate.Selection{}, true},
		{"Negative", "-1", tiergate.Selection{}, true},
		{"OpenRange", "1-", tiergate.Selection{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tiergate.EnvVar, tt.value)
			got, err := tiergate.Selected()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Selected() with %q error = %v; want error %v", tt.value, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Selected() with %q = %+v; want %+v", tt.value, got, tt.want)
			}
		})
	}
}

func TestRequire_FailsOnAMalformedSelection(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name  string
		value string
	}{
		{"Word", "tier1"},
		{"DescendingRange", "2-0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tiergate.EnvVar, tt.value)
			recorder := &recordingTB{}
			done := make(chan struct{})
			go func() {
				defer close(done)
				tiergate.Require(recorder, 1)
			}()
			<-done
			if !recorder.failed || recorder.skipped {
				t.Errorf("Require with %s=%q failed = %v, skipped = %v; want failed and not skipped", tiergate.EnvVar, tt.value, recorder.failed, recorder.skipped)
			}
			if !strings.Contains(recorder.message, tiergate.EnvVar) {
				t.Errorf("Require with %s=%q failure message = %q; want it to name %s", tiergate.EnvVar, tt.value, recorder.message, tiergate.EnvVar)
			}
		})
	}
}

func TestRequire_RunsOnlySelectedTiers(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name    string
		value   string
		tier    int
		wantRun bool
	}{
		{"SelectedTier", "1", 1, true},
		{"OtherTier", "1", 2, false},
		{"InsideRange", "0-2", 2, true},
		{"AboveRange", "0-2", 3, false},
		{"UnsetRunsTier0", "", 0, true},
		{"UnsetSkipsTier1", "", 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tiergate.EnvVar, tt.value)
			ran := false
			t.Run("Inner", func(t *testing.T) {
				tiergate.Require(t, tt.tier)
				ran = true
			})
			if ran != tt.wantRun {
				t.Errorf("Require(t, %d) with %s=%q ran = %v; want %v", tt.tier, tiergate.EnvVar, tt.value, ran, tt.wantRun)
			}
		})
	}
}
