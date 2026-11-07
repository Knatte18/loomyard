// spawn_test.go covers SpawnOrAttach.
// It pins the attach to a live record and the spawn's spec and record.
// It also pins each failure that must leave no unrecorded strand.

package gatefixer

import (
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSpawnOrAttach pins the attach, the spawn's spec, record and registration, and the failures a spawn must not outlive.
func TestSpawnOrAttach(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("LiveRecordAttachesAndStartsNothing", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t)
		live := &Record{StrandGUID: "live-guid"}
		fx.store.records = []*Record{{StrandGUID: "old", Status: StatusDead}, live}

		rec, spawned, err := SpawnOrAttach(fx.deps, fx.store, fx.spec)
		if err != nil || spawned || rec != live {
			t.Fatalf("SpawnOrAttach() = %p, %v, %v; want an attach to %p", rec, spawned, err, live)
		}
		if len(fx.starter.specs) != 0 || len(fx.registered) != 0 {
			t.Errorf("starts = %d, registrations = %v; want none", len(fx.starter.specs), fx.registered)
		}
	})

	t.Run("SpawnStartsRegistersAndSavesTheRecord", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t)
		if err := os.WriteFile(fx.replyPath, []byte("stale"), 0o644); err != nil {
			t.Fatalf("seed stale reply: %v", err)
		}

		rec := fx.spawn(t)
		if _, err := os.Stat(fx.replyPath); !os.IsNotExist(err) {
			t.Errorf("stat stale reply = %v; want it removed before the spawn", err)
		}
		spec := fx.engine.LastSpec
		wantSkills, err := shuttlefake.Agents(t).Skills(agentconfig.RoleGateFixer, []string{"scribe:prose"})
		if err != nil {
			t.Fatalf("Skills(gate-fixer) error = %v", err)
		}
		if spec.Role != RoleGateFixer || spec.ModelChoice.Model != "fixer-model" || !slices.Equal(spec.OutputFiles, []string{fx.replyPath}) || spec.ConfigRole != agentconfig.RoleGateFixer || !slices.Equal(spec.Skills, wantSkills) {
			t.Errorf("started spec = role %q, model %q, output files %v, config role %q, skills %v; want the fixer's role, model, the reply as only output file, the gate-fixer config role and its skills", spec.Role, spec.ModelChoice.Model, spec.OutputFiles, spec.ConfigRole, spec.Skills)
		}
		if fx.engine.LastPrompt != "fix the failures" {
			t.Errorf("started prompt = %q; want the spec's prompt", fx.engine.LastPrompt)
		}
		if !slices.Equal(fx.registered, []string{rec.StrandGUID}) {
			t.Errorf("registered = %v; want the spawned strand %q", fx.registered, rec.StrandGUID)
		}
		if len(fx.store.records) != 1 || fx.store.records[0] != rec {
			t.Fatalf("store = %+v; want the one spawned record", fx.store.records)
		}
		if rec.StrandGUID == "" || rec.ShuttleRunDir == "" || rec.EventsPath == "" || rec.SpawnedAt != "1970-01-01T00:00:00Z" || rec.ReportPath != "/lym/report.md" || rec.ReplyPath != fx.replyPath || rec.Status != "" {
			t.Errorf("record = %+v; want the strand, run dir, events, the clock's spawn time, both paths and no status", rec)
		}
		if len(fx.stopper.guids) != 0 {
			t.Errorf("stopped = %v; want nothing stopped", fx.stopper.guids)
		}

		again, spawned, err := SpawnOrAttach(fx.deps, fx.store, fx.spec)
		if err != nil || spawned || again != rec || len(fx.starter.specs) != 1 {
			t.Errorf("second call = %p, %v, %v with %d starts; want an attach to %p and no second spawn", again, spawned, err, len(fx.starter.specs), rec)
		}
	})

	t.Run("NilRegisterRegistersNothing", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t)
		fx.deps.Register = nil
		fx.spawn(t)
		if len(fx.registered) != 0 || len(fx.store.records) != 1 {
			t.Errorf("registrations = %v, records = %d; want none registered and the record saved", fx.registered, len(fx.store.records))
		}
	})

	failures := []struct {
		name        string
		arrange     func(t *testing.T, fx *fixture)
		wantStopped bool
		wantText    string
	}{
		{
			name:     "StartFailureSavesAndStopsNothing",
			arrange:  func(_ *testing.T, fx *fixture) { fx.starter.err = errors.New("provider never ready") },
			wantText: "provider never ready",
		},
		{
			name:        "UnresolvedRunStopsTheStrand",
			arrange:     func(t *testing.T, fx *fixture) { fx.deps.ShuttleCfg.RunDir = t.TempDir() },
			wantStopped: true,
			wantText:    "resolve the spawned",
		},
		{
			name: "FailedRegistrationStopsTheStrand",
			arrange: func(_ *testing.T, fx *fixture) {
				fx.deps.Register = func(string) error { return errors.New("no pane") }
			},
			wantStopped: true,
			wantText:    "no pane",
		},
		{
			name:        "FailedSaveStopsTheStrand",
			arrange:     func(_ *testing.T, fx *fixture) { fx.store.saveErr = errors.New("disk full") },
			wantStopped: true,
			wantText:    "disk full",
		},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			tt.arrange(t, fx)

			rec, spawned, err := SpawnOrAttach(fx.deps, fx.store, fx.spec)
			if err == nil || rec != nil || spawned {
				t.Fatalf("SpawnOrAttach() = %v, %v, %v; want the failure", rec, spawned, err)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Errorf("SpawnOrAttach() error = %q; want it to contain %q", err, tt.wantText)
			}
			if class := shedengine.TransientOf(err); class != shedengine.TransientAgentStart {
				t.Errorf("transient class = %q; want %q", class, shedengine.TransientAgentStart)
			}
			wantStops := 0
			if tt.wantStopped {
				wantStops = 1
			}
			if len(fx.stopper.guids) != wantStops {
				t.Errorf("stopped = %v; want %d stop", fx.stopper.guids, wantStops)
			}
			if len(fx.store.records) != 0 {
				t.Errorf("store = %+v; want no record of a failed spawn", fx.store.records)
			}
		})
	}

	t.Run("RoleOutsideTheSetIsRefused", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t)
		fx.spec.Role = "implementer"
		_, _, err := SpawnOrAttach(fx.deps, fx.store, fx.spec)
		if err == nil || !strings.Contains(err.Error(), `"implementer"`) || !strings.Contains(err.Error(), RoleGateFixer) || !strings.Contains(err.Error(), RoleVerifyFixer) {
			t.Fatalf("SpawnOrAttach() error = %v; want a refusal naming the role and the fixer roles", err)
		}
		if len(fx.starter.specs) != 0 {
			t.Errorf("starts = %d; want none", len(fx.starter.specs))
		}
	})
}
