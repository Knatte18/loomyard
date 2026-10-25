//go:build tier0

// pausehold_test.go pins that `lyx batten run` refuses a slug a live pause entry covers before it seeds a status file.

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
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// TestBattenRun_RefusesWhileAPauseCoversTheSlug runs the run verb's driveRun over a prime whose record holds a live entry:
// an entry covering the slug refuses naming `lyx resume <slug>` with nothing seeded, the run verb exiting with the covered code, and an entry for another slug seeds as before.
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
			// A run past the pre-run yields at once.
			c.untilStop = func(context.Context, string) ([]byte, error) {
				return stopEnvelope(t, map[string]any{"ok": true, "outcome": string(shedengine.Yield)}, string(shedengine.StateRunning), battenrecipe.NameRunShed, 0), nil
			}
			var driveOut bytes.Buffer
			driveExit := clihelp.Execute(runCommand(c), &driveOut, []string{c.slug})

			_, statErr := os.Stat(c.shedPaths.StatusPath)
			if !tt.wantCover {
				if driveExit != 0 || statErr != nil {
					t.Fatalf("run exit = %d, output %q, status file stat = %v; want a normal run", driveExit, driveOut.String(), statErr)
				}
				return
			}
			if driveExit != dobbyengine.PassExitCovered || !strings.Contains(driveOut.String(), "lyx resume") {
				t.Errorf("run exit = %d, output %q; want exit %d and a refusal naming %q", driveExit, driveOut.String(), dobbyengine.PassExitCovered, "lyx resume")
			}
			if !os.IsNotExist(statErr) {
				t.Errorf("status file stat = %v; want it absent, nothing seeded", statErr)
			}
		})
	}
}

// TestBattenRun_LostWorktreeCreateClaimExitsWithTheDriversCode asserts a Worktree-Create row that fails with an OwnedElsewhereError ends in the driver's own error envelope and exit, since the owned-elsewhere code is set only for a refusal before the step loop.
func TestBattenRun_LostWorktreeCreateClaimExitsWithTheDriversCode(t *testing.T) {
	t.Parallel()

	c := newFakeReceiver(t, nil)
	c.env.CreateWorktree = func(context.Context) error { return ownedElsewhereRefusal(c.slug, "there") }

	var out bytes.Buffer
	exitCode := clihelp.Execute(runCommand(c), &out, []string{c.slug})

	if exitCode != 0 || !strings.Contains(out.String(), `"outcome":"blocked"`) || !strings.Contains(out.String(), "lyx adopt "+c.slug) {
		t.Errorf("run exit = %d, output %q; want exit 0 and a blocked envelope carrying the refusal's way forward", exitCode, out.String())
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
