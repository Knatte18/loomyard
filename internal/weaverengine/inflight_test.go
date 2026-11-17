// inflight_test.go covers RunInFlight over temp anchors with planted state and outcome files.

package weaverengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
)

func TestRunInFlight(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// state plants state.json.
		state bool
		// outcome plants outcome.yaml when non-empty.
		outcome string
		want    bool
		wantErr bool
	}{
		{name: "NoState"},
		{name: "StateWithNoOutcome", state: true, want: true},
		{name: "StuckOutcome", state: true, outcome: "outcome: stuck\nstuck_reason: verify failed\nbatches_done: 1\n", want: true},
		{name: "DoneOutcome", state: true, outcome: "outcome: done\nstuck_reason: null\nbatches_done: 3\n"},
		{name: "UndecodableOutcome", state: true, outcome: "outcome: [unterminated\n", wantErr: true},
		{name: "UnknownOutcomeValue", state: true, outcome: "outcome: paused\n", wantErr: true},
		{name: "UnknownOutcomeKey", state: true, outcome: "outcome: done\nextra: 1\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchor := t.TempDir()
			runDir := weaverengine.Dir(anchor)
			if err := os.MkdirAll(runDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.state {
				if err := os.WriteFile(filepath.Join(runDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tt.outcome != "" {
				if err := os.WriteFile(weaverengine.OutcomePath(runDir), []byte(tt.outcome), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			got, err := weaverengine.RunInFlight(anchor)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RunInFlight() error = %v; want error %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("RunInFlight() = %v; want %v", got, tt.want)
			}
		})
	}
}
