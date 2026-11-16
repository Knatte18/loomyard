// pausehold_test.go pins that `lym batten run` asks the act rule before it seeds a status file.

package battencli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/runfsm"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestBattenRun_RefusesWhileAPauseCoversTheSlug runs the run verb's driveRun with the act rule's verdict set.
// A live pause entry that covers the slug refuses the run through the rule, naming `lym tender resume <slug>`.
// The refusal leaves nothing seeded and exits with the covered code.
// An entry for another slug seeds as before.
// A fake verdict covers the other answers:
// act goes on, and a hand-over exits with the owned-elsewhere code.
// A terminal wait refuses plain.
// A landed run with a done record gets the done-slug refusal, and a failed read names the re-run.
func TestBattenRun_RefusesWhileAPauseCoversTheSlug(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	verdictOf := func(in runfsm.Inputs) *runfsm.Verdict {
		verdict := runfsm.Act(in)
		return &verdict
	}
	tests := []struct {
		name string
		// scope is the scope of the pause entry the prime's record holds; empty writes none.
		scope string
		// verdict and verdictErr replace the verdict read over the pause record when either is set.
		verdict    *runfsm.Verdict
		verdictErr error
		// doneRecord writes a done status file before the run.
		doneRecord bool
		wantExit   int
		wantOutput []string
		// wantStatusFile is whether the status file stands after the run.
		wantStatusFile bool
	}{
		{name: "an entry for the slug refuses", scope: "some-slug", wantExit: dobbyengine.PassExitCovered, wantOutput: []string{"lym tender resume"}},
		{name: "a hub-wide entry refuses", scope: pauserecord.ScopeHub, wantExit: dobbyengine.PassExitCovered, wantOutput: []string{"lym tender resume"}},
		{name: "an entry for another slug seeds as today", scope: "other-slug", wantStatusFile: true},
		{name: "an act verdict goes on to the status check", verdict: verdictOf(runfsm.Inputs{State: runfsm.Running}), wantStatusFile: true},
		{
			name:       "a hand-over verdict exits with the owned-elsewhere code naming the owner",
			verdict:    verdictOf(runfsm.Inputs{OwnedElsewhere: true, Owner: "there", AdoptSubject: "some-beam"}),
			wantExit:   dobbyengine.PassExitOwnedElsewhere,
			wantOutput: []string{"machine there owns the run", "lym adopt some-beam"},
		},
		{
			name:       "a terminal cancelled wait names the removal",
			verdict:    verdictOf(runfsm.Inputs{State: runfsm.Cancelled}),
			wantExit:   1,
			wantOutput: []string{"was cancelled", "--remove"},
		},
		{
			name:           "a terminal landed wait over a done record gives the done-slug refusal",
			verdict:        verdictOf(runfsm.Inputs{State: runfsm.Landed}),
			doneRecord:     true,
			wantExit:       1,
			wantOutput:     []string{"has already completed", "lym batten cleanup --apply"},
			wantStatusFile: true,
		},
		{
			name:       "a failed read refuses naming the re-run",
			verdictErr: errors.New("board is gone"),
			wantExit:   1,
			wantOutput: []string{"board is gone", "lym batten run some-slug"},
		},
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
			if tt.scope != "" {
				now := time.Now()
				if _, err := pauserecord.Update(geometry.RecordPath, now, func(record *pauserecord.Record) error {
					record.Live = append(record.Live, pauserecord.Entry{Scope: tt.scope, PausedAt: now, Caller: "EXTERNAL"})
					return nil
				}); err != nil {
					t.Fatalf("pauserecord.Update: %v", err)
				}
			}
			if tt.doneRecord {
				writeStatus(t, c, shedengine.Status{CurrentProducer: battenrecipe.NameWorktreeTeardown, State: shedengine.StateDone})
			}
			c.runVerdict = func() (runfsm.Verdict, error) {
				if tt.verdictErr != nil {
					return runfsm.Verdict{}, tt.verdictErr
				}
				if tt.verdict != nil {
					return *tt.verdict, nil
				}
				coverage, err := pauserecord.ReadCoverage(geometry.RecordPath, c.slug, time.Now())
				if err != nil {
					return runfsm.Verdict{}, err
				}
				_, verdict := runfsm.Judge(runfsm.Records{}, runfsm.Inputs{PauseCovers: coverage.Covered}, runfsm.Aside{ShedPause: true})
				return verdict, nil
			}
			// A run past the pre-run yields at once.
			c.untilStop = func(context.Context, string) ([]byte, error) {
				return stopEnvelope(t, map[string]any{"ok": true, "outcome": string(shedengine.Yield)}, string(shedengine.StateRunning), battenrecipe.NameRunShed, 0), nil
			}
			var driveOut bytes.Buffer
			driveExit := clihelp.Execute(runCommand(c), &driveOut, []string{c.slug})

			if driveExit != tt.wantExit {
				t.Errorf("run exit = %d, output %q; want exit %d", driveExit, driveOut.String(), tt.wantExit)
			}
			for _, want := range tt.wantOutput {
				if !strings.Contains(driveOut.String(), want) {
					t.Errorf("run output %q; want it to name %q", driveOut.String(), want)
				}
			}
			if _, statErr := os.Stat(c.shedPaths.StatusPath); (statErr == nil) != tt.wantStatusFile {
				t.Errorf("status file stat = %v; want it present: %v", statErr, tt.wantStatusFile)
			}
		})
	}
}

// TestBattenRun_LostWorktreeCreateClaimExitsWithTheDriversCode asserts the driver's own exit for a lost claim.
// A Worktree-Create row that fails with an OwnedElsewhereError ends in the driver's own error envelope and exit.
// The owned-elsewhere code is set only for a refusal before the step loop.
func TestBattenRun_LostWorktreeCreateClaimExitsWithTheDriversCode(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	c := newFakeReceiver(t, nil)
	c.env.CreateWorktree = func(context.Context, battenshed.HoldPrimeLock) error {
		return ownedElsewhereFor(c.slug, "there", "some-beam")
	}

	var out bytes.Buffer
	exitCode := clihelp.Execute(runCommand(c), &out, []string{c.slug})

	if exitCode != 0 || !strings.Contains(out.String(), `"outcome":"blocked"`) || !strings.Contains(out.String(), "lym adopt some-beam") {
		t.Errorf("run exit = %d, output %q; want exit 0 and a blocked envelope carrying the refusal's way forward", exitCode, out.String())
	}
}

// TestBattenStatus_CarriesPausedSinceForACoveredSlug runs batten's status verb over a prime with a live entry.
// The envelope carries paused_since and paused_by for the entry's slug and neither for an entry covering another slug.
func TestBattenStatus_CarriesPausedSinceForACoveredSlug(t *testing.T) {
	tiergate.Require(t, 0)
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
