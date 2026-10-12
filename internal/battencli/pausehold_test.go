// pausehold_test.go pins that `lyx batten run` refuses a slug a live pause entry covers, with and without --window, before it seeds a status file or opens a window.

package battencli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/spf13/cobra"
)

// TestBattenRun_RefusesWhileAPauseCoversTheSlug runs battenPreRun and runInWindow over a prime whose record holds a live entry:
// an entry covering the slug refuses both naming `lyx resume <slug>` with nothing seeded and no window opened, and an entry for another slug seeds as before.
func TestBattenRun_RefusesWhileAPauseCoversTheSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		scope     string
		wantCover bool
	}{
		{"an entry for the slug refuses", "some-slug", true},
		{"a hub-wide entry refuses", pauserecord.ScopeHub, true},
		{"an entry for another slug seeds as today", "other-slug", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := newFakeReceiver(t, nil)
			if err := os.MkdirAll(filepath.Join(c.location.WorktreePath(), ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			geometry, err := hubgeom.PauseGeometry(c.location)
			if err != nil {
				t.Fatalf("hubgeom.PauseGeometry: %v", err)
			}
			now := time.Now()
			if _, err := pauserecord.Update(geometry.RecordPath, now, func(record *pauserecord.Record) error {
				record.Live = append(record.Live, pauserecord.Entry{Scope: tt.scope, PausedAt: now, Caller: "EXTERNAL"})
				return nil
			}); err != nil {
				t.Fatalf("pauserecord.Update: %v", err)
			}
			windowOpened := false
			c.windowOpener = func(string, []string) (reedengine.WindowResult, error) {
				windowOpened = true
				return reedengine.WindowResult{}, nil
			}
			c.terminalLauncher = func() (string, error) { return "konsole", nil }

			preRunErr := c.battenPreRun(context.Background())

			cmd := &cobra.Command{Use: "run", RunE: func(cmd *cobra.Command, args []string) error { return c.runInWindow(cmd) }}
			var out bytes.Buffer
			clihelp.Execute(cmd, &out, []string{})
			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("envelope %q is not one JSON object: %v", out.String(), err)
			}

			_, statErr := os.Stat(c.shedPaths.StatusPath)
			if !tt.wantCover {
				if preRunErr != nil || envelope["ok"] != true || !windowOpened || statErr != nil {
					t.Fatalf("battenPreRun = %v, envelope %v, window opened = %v, status file stat = %v; want a normal run", preRunErr, envelope, windowOpened, statErr)
				}
				return
			}
			if preRunErr == nil || !strings.Contains(preRunErr.Error(), "lyx resume") {
				t.Errorf("battenPreRun = %v; want a refusal naming %q", preRunErr, "lyx resume")
			}
			if msg, _ := envelope["error"].(string); envelope["ok"] != false || !strings.Contains(msg, "lyx resume") {
				t.Errorf("runInWindow envelope = %v; want a refusal naming %q", envelope, "lyx resume")
			}
			if windowOpened {
				t.Error("a window was opened for a covered slug")
			}
			if !os.IsNotExist(statErr) {
				t.Errorf("status file stat = %v; want it absent, nothing seeded", statErr)
			}
		})
	}
}
