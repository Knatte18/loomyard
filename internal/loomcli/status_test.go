// status_test.go drives loom's status verb in-process and pins the envelope key set it emits and the interrupt policy it adds.
// The generic status line rendering is tested in internal/shedverbs.

package loomcli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/state"
)

// TestStatusCmd_EnvelopeKeySet drives statusCmd()'s RunE in-process against a hand-populated
// receiver whose shedPaths point at a seeded status file under t.TempDir(), and asserts the emitted
// envelope's top-level key set equals exactly the nine payload keys plus "ok" -- in both directions,
// so the interrupt_policy addition can neither quietly rename an existing key nor let a later
// removal go unnoticed. It also tables interrupt_policy's own value across the three shapes
// InterruptPolicyFor returns: "handback" for NameWebster, "reinvoke" for another named row, and the
// empty string for a current_producer naming no row at all.
func TestStatusCmd_EnvelopeKeySet(t *testing.T) {
	tests := []struct {
		name            string
		currentProducer string
		wantPolicy      string
		covered         bool
	}{
		{"HandbackForWebster", loomshed.NameWebster, shedverbs.InterruptPolicyHandback, false},
		{"ReinvokeForAnotherRow", loomshed.NameDiscussionWrite, shedverbs.InterruptPolicyReinvoke, false},
		{"EmptyForUnknownRow", "not-a-real-row", "", false},
		{"PausedSinceForACoveredRun", loomshed.NameWebster, shedverbs.InterruptPolicyHandback, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			statusPath := filepath.Join(dir, "status.json")
			statusLockPath := filepath.Join(dir, "status.json.lock")

			seeded := shedengine.Status{
				CurrentProducer: tt.currentProducer,
				State:           shedengine.StateRunning,
				Activity:        shedengine.Activity{Now: "Preflight"},
			}
			if err := state.WriteJSON(statusPath, statusLockPath, seeded); err != nil {
				t.Fatalf("state.WriteJSON(%q) = %v; want nil", statusPath, err)
			}

			c := &loomCLI{
				shedPaths: shedbuild.ShedPaths{
					StatusPath:     statusPath,
					StatusLockPath: statusLockPath,
				},
			}
			var wantPausedSince string
			if tt.covered {
				c.location = &lyxcwd.Location{RepoName: "example", HubPath: filepath.Join(dir, "hub"), WorktreeName: "some-slug", AnchorRel: "."}
				if err := os.MkdirAll(c.location.WorktreePath(), 0o755); err != nil {
					t.Fatal(err)
				}
				recordPath := coverRun(t, c)
				record, err := pauserecord.Read(recordPath)
				if err != nil || len(record.Live) != 1 {
					t.Fatalf("pauserecord.Read(%q) = %v, %v; want one live entry", recordPath, record, err)
				}
				wantPausedSince = record.Live[0].PausedAt.Format(time.RFC3339)
			}

			var out bytes.Buffer
			exitCode := clihelp.Execute(loomVerbCommand(c, "status"), &out, nil)
			if exitCode != 0 {
				t.Fatalf("statusCmd() exit code = %d; want 0; output: %q", exitCode, out.String())
			}

			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("json.Unmarshal(%q) = %v; want nil", out.String(), err)
			}

			wantKeys := map[string]bool{
				"ok": true, "current_producer": true, "state": true, "error": true,
				"pause_requested": true, "activity": true, "history_length": true,
				"slug": true, "parent": true, "interrupt_policy": true, "trace_dir": true,
				"run_id": true, "progress": true, "last_step": true,
			}
			if tt.covered {
				wantKeys["paused_since"] = true
				wantKeys["paused_by"] = true
				if got, _ := envelope["paused_since"].(string); got != wantPausedSince {
					t.Errorf("statusCmd() envelope[\"paused_since\"] = %q; want %q", got, wantPausedSince)
				}
				if got, _ := envelope["paused_by"].(string); got != pauserecord.ScopeHub {
					t.Errorf("statusCmd() envelope[\"paused_by\"] = %q; want %q", got, pauserecord.ScopeHub)
				}
			}
			for k := range wantKeys {
				if _, ok := envelope[k]; !ok {
					t.Errorf("statusCmd() envelope missing key %q; got keys %v", k, envelope)
				}
			}
			for k := range envelope {
				if !wantKeys[k] {
					t.Errorf("statusCmd() envelope has unexpected key %q; got keys %v", k, envelope)
				}
			}

			if got, _ := envelope["interrupt_policy"].(string); got != tt.wantPolicy {
				t.Errorf("statusCmd() envelope[\"interrupt_policy\"] = %q; want %q", got, tt.wantPolicy)
			}
		})
	}
}
