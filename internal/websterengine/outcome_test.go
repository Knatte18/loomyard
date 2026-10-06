// outcome_test.go exercises parseOutcome's accept/reject table.
// Tier 1: no git, only t.TempDir() plus an injected now.

package websterengine

import (
	"os"
	"path/filepath"
	"testing"
)

// outcomeWriteFile writes raw content to path, creating its parent
// directory first, failing the test on any error.
func outcomeWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

// TestParseOutcome_AcceptReject tables every outcome.yaml accept/reject case parseOutcome's
// fail-loud discipline pins: a well-formed done/stuck/paused file parses;
// an unrecognized outcome value, a stuck file missing stuck_reason, an unknown extra key and a
// missing file are all rejected loudly rather than guessed at.
func TestParseOutcome_AcceptReject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// content is the file body; missing leaves the file absent instead.
		content string
		missing bool
		wantErr bool
		want    outcome
	}{
		{
			name:    "done accepted",
			content: "outcome: done\nstuck_reason: null\nbatches_done: 3\n",
			want:    outcome{Outcome: "done", StuckReason: "", BatchesDone: 3},
		},
		{
			name:    "stuck with reason accepted",
			content: "outcome: stuck\nstuck_reason: \"batch 03 red after 2 self-fix attempts\"\nbatches_done: 2\n",
			want:    outcome{Outcome: "stuck", StuckReason: "batch 03 red after 2 self-fix attempts", BatchesDone: 2},
		},
		{
			name:    "paused accepted",
			content: "outcome: paused\nstuck_reason: null\nbatches_done: 1\n",
			want:    outcome{Outcome: "paused", StuckReason: "", BatchesDone: 1},
		},
		{
			name:    "unrecognized outcome value rejected",
			content: "outcome: bogus\nstuck_reason: null\nbatches_done: 0\n",
			wantErr: true,
		},
		{
			name:    "stuck without stuck_reason rejected",
			content: "outcome: stuck\nstuck_reason: null\nbatches_done: 0\n",
			wantErr: true,
		},
		{
			name:    "stuck with blank stuck_reason rejected",
			content: "outcome: stuck\nstuck_reason: \"   \"\nbatches_done: 0\n",
			wantErr: true,
		},
		{
			name:    "unknown key rejected",
			content: "outcome: done\nstuck_reason: null\nbatches_done: 1\nbogus_extra_key: true\n",
			wantErr: true,
		},
		{
			name:    "unparseable yaml rejected",
			content: "outcome: [this is not a mapping\n",
			wantErr: true,
		},
		{
			name:    "missing file is a wrapped error, not a guessed nil result",
			missing: true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "outcome.yaml")
			if !tt.missing {
				outcomeWriteFile(t, path, tt.content)
			}

			got, err := parseOutcome(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseOutcome() error = nil; want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOutcome() error = %v; want nil", err)
			}
			if *got != tt.want {
				t.Errorf("parseOutcome() = %+v; want %+v", *got, tt.want)
			}
		})
	}
}
