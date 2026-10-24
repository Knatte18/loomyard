//go:build tier0

// syncdeferral_test.go covers the board sync's deferrable flag under the process's network gate, against a board path that does not exist.

package boardcli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// TestSync_DeferrableExitsUnderACoveringPause runs sync against a board path that does not exist, so a sync that reaches the board errors:
// a deferrable sync under a Defer gate a hub entry covers succeeds without touching the board, a plain sync reports the board error,
// and a deferrable sync with nothing covering proceeds to the board and reports the same error.
// Serial: it installs the process's network gate.
func TestSync_DeferrableExitsUnderACoveringPause(t *testing.T) {
	missingBoard := filepath.Join(t.TempDir(), "no-board")

	tests := []struct {
		name      string
		args      []string
		covered   bool
		wantOK    bool
		wantError bool
	}{
		{"DeferrableCoveredSucceedsUntouched", []string{"--board-path", missingBoard, "sync", "--deferrable"}, true, true, false},
		{"PlainCoveredReachesTheBoard", []string{"--board-path", missingBoard, "sync"}, true, false, true},
		{"DeferrableUncoveredReachesTheBoard", []string{"--board-path", missingBoard, "sync", "--deferrable"}, false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recordPath := filepath.Join(t.TempDir(), pauserecord.RecordFileName)
			now := time.Now()
			if _, err := pauserecord.Update(recordPath, now, func(r *pauserecord.Record) error {
				if tt.covered {
					r.Live = []pauserecord.Entry{{Scope: pauserecord.ScopeHub, PausedAt: now}}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(pauserecord.InstallGate(pauserecord.Gate{Geometry: pauserecord.Geometry{RecordPath: recordPath}, Defer: true}))

			var stdout bytes.Buffer
			code := RunCLI(&stdout, tt.args)
			var envelope struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("parse %q: %v", stdout.String(), err)
			}
			if envelope.OK != tt.wantOK || (envelope.Error != "") != tt.wantError || (code == 0) != tt.wantOK {
				t.Errorf("sync %q = exit %d, %q; want ok %v and board error %v", tt.args, code, stdout.String(), tt.wantOK, tt.wantError)
			}
		})
	}
}
