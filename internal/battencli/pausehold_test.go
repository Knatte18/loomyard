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

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
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

// TestBattenStatus_CarriesPausedSinceForACoveredSlug runs batten's status verb over a prime whose record holds a live entry:
// the envelope carries paused_since and paused_by for the entry's slug and neither for an entry covering another slug.
func TestBattenStatus_CarriesPausedSinceForACoveredSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		scope     string
		wantCover bool
	}{
		{"a covered slug carries both keys", "some-slug", true},
		{"an uncovered slug carries neither", "other-slug", false},
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
			record, err := pauserecord.Read(geometry.RecordPath)
			if err != nil || len(record.Live) != 1 {
				t.Fatalf("pauserecord.Read = %v, %v; want one live entry", record, err)
			}
			writeStatus(t, c, shedengine.Status{CurrentProducer: battenrecipe.NameRunShed, State: shedengine.StateRunning})

			var out bytes.Buffer
			if exitCode := clihelp.Execute(battenVerbCommand(c, "status"), &out, []string{c.slug}); exitCode != 0 {
				t.Fatalf("status exit code = %d; want 0; output: %s", exitCode, out.String())
			}
			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("envelope %q is not one JSON object: %v", out.String(), err)
			}

			since, hasSince := envelope["paused_since"]
			by, hasBy := envelope["paused_by"]
			if hasSince != tt.wantCover || hasBy != tt.wantCover {
				t.Fatalf("envelope keys paused_since = %v, paused_by = %v; want both present = %v; envelope %v", hasSince, hasBy, tt.wantCover, envelope)
			}
			if tt.wantCover && (since != record.Live[0].PausedAt.Format(time.RFC3339) || by != tt.scope) {
				t.Errorf("paused_since = %v, paused_by = %v; want %q, %q", since, by, record.Live[0].PausedAt.Format(time.RFC3339), tt.scope)
			}
		})
	}
}
