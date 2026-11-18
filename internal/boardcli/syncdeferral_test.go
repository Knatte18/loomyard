// syncdeferral_test.go covers the board sync's deferrable flag under the process's network gate;
// it runs against a board path that does not exist.

package boardcli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSync_DeferrableExitsUnderACoveringPause runs sync against a board path that does not exist,
// so a sync that reaches the board errors:
// a deferrable sync under a Defer gate a hub entry covers succeeds without touching the board;
// a plain sync waits once at the gate, before the board's push lock, and reports the board error once the pause lifts;
// and a deferrable sync with nothing covering proceeds to the board and reports the same error.
// Serial: it installs the process's network gate.
func TestSync_DeferrableExitsUnderACoveringPause(t *testing.T) {
	tiergate.Require(t, 0)
	missingBoard := filepath.Join(t.TempDir(), "no-board")

	tests := []struct {
		name      string
		args      []string
		covered   bool
		wantOK    bool
		wantError bool
		wantWaits int
	}{
		{"DeferrableCoveredSucceedsUntouched", []string{"--board-path", missingBoard, "sync", "--deferrable"}, true, true, false, 0},
		{"PlainCoveredWaitsThenReachesTheBoard", []string{"--board-path", missingBoard, "sync"}, true, false, true, 1},
		{"DeferrableUncoveredReachesTheBoard", []string{"--board-path", missingBoard, "sync", "--deferrable"}, false, false, true, 0},
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
			waits := 0
			// A wait lifts the pause, so a sync the gate holds goes on to the board.
			lift := func(context.Context, time.Duration) error {
				waits++
				_, err := pauserecord.Update(recordPath, now, func(r *pauserecord.Record) error {
					r.Live = nil
					return nil
				})
				return err
			}
			t.Cleanup(pauserecord.InstallGate(pauserecord.Gate{Geometry: pauserecord.Geometry{RecordPath: recordPath}, Defer: true, Notify: func(string) {}, Sleep: lift}))

			var stdout bytes.Buffer
			code := clihelp.Execute(Command(), &stdout, tt.args)
			if waits != tt.wantWaits {
				t.Errorf("sync %q waited at the gate %d times; want %d", tt.args, waits, tt.wantWaits)
			}
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
