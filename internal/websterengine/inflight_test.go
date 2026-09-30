// inflight_test.go exercises RunInFlight over t.TempDir() anchors.
// Tier 1: no git.

package websterengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestRunInFlight tables every state.json/outcome.yaml combination RunInFlight distinguishes.
func TestRunInFlight(t *testing.T) {
	tests := []struct {
		name    string
		state   bool
		outcome func(t *testing.T, outcomePath string)
		want    bool
		wantErr bool
	}{
		{name: "no state file", want: false},
		{name: "state without outcome", state: true, want: true},
		{name: "paused", state: true, outcome: writeOutcome("outcome: paused\nstuck_reason: null\nbatches_done: 1\n"), want: true},
		{name: "stuck with reason", state: true, outcome: writeOutcome("outcome: stuck\nstuck_reason: \"batch 2 failed\"\nbatches_done: 1\n"), want: true},
		{name: "done", state: true, outcome: writeOutcome("outcome: done\nstuck_reason: null\nbatches_done: 3\n"), want: false},
		{name: "unknown outcome value", state: true, outcome: writeOutcome("outcome: bogus\nstuck_reason: null\nbatches_done: 0\n"), wantErr: true},
		{name: "unparseable yaml", state: true, outcome: writeOutcome("outcome: [unterminated\n"), wantErr: true},
		{
			name:  "unreadable outcome",
			state: true,
			outcome: func(t *testing.T, outcomePath string) {
				t.Helper()
				if err := os.MkdirAll(outcomePath, 0o755); err != nil {
					t.Fatalf("MkdirAll(%q): %v", outcomePath, err)
				}
			},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			anchor := t.TempDir()
			dir := websterengine.Dir(anchor)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("MkdirAll(%q): %v", dir, err)
			}
			if tc.state {
				if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{}"), 0o644); err != nil {
					t.Fatalf("write state.json: %v", err)
				}
			}
			if tc.outcome != nil {
				tc.outcome(t, websterengine.OutcomePath(dir))
			}

			got, err := websterengine.RunInFlight(anchor)
			if (err != nil) != tc.wantErr {
				t.Fatalf("RunInFlight err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("RunInFlight = %v, want %v", got, tc.want)
			}
		})
	}
}

// writeOutcome returns a seeder that writes content as the outcome file.
func writeOutcome(content string) func(t *testing.T, outcomePath string) {
	return func(t *testing.T, outcomePath string) {
		t.Helper()
		if err := os.WriteFile(outcomePath, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", outcomePath, err)
		}
	}
}
