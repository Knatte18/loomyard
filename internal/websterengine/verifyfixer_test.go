//go:build tier0

// verifyfixer_test.go exercises the cold verify-gate fixer's spawn-or-attach and bounded wait over the recover-batch fixture: a real *shuttleengine.Runner over the shuttlefake Reed and Engine, and a fake Clock.

package websterengine_test

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// noAuditEngines is a websterengine.AuditEngines whose one engine has no fork audit, since embedding the interface hides every capability beyond it.
type noAuditEngines struct {
	engine shuttleengine.Engine
}

func (n noAuditEngines) Lookup(string) (shuttleengine.Engine, error) {
	return struct{ shuttleengine.Engine }{n.engine}, nil
}

func (noAuditEngines) DefaultName() string { return shuttlefake.EngineName }

// newFixVerifyFixture returns the recover-batch fixture with a scratch dir, and the FixVerifyDeps over its seams.
// withReport writes a verify-gate report routed to the cold fixer.
func newFixVerifyFixture(t *testing.T, withReport bool) (*recoverFixture, websterengine.FixVerifyDeps) {
	t.Helper()
	fx := newRecoverFixture(t)
	fx.Deps.Geom.ScratchDir = t.TempDir()
	if withReport {
		report := websterengine.VerifyGateReport{Attempt: 2, Cap: 2, Fixer: websterengine.FixerCold}
		if err := websterengine.WriteVerifyGateReport(websterengine.VerifyGateReportPath(fx.ReportsDir), report); err != nil {
			t.Fatalf("WriteVerifyGateReport() = %v", err)
		}
	}
	return fx, websterengine.FixVerifyDeps{
		Starter:    fx.Deps.Starter,
		Roles:      fx.Deps.Roles,
		Config:     fx.Deps.Config,
		Engines:    fx.Deps.Engines,
		Reed:       fx.Deps.Reed,
		Stopper:    fx.Deps.Stopper,
		ShuttleCfg: fx.Deps.ShuttleCfg,
		Geom:       fx.Deps.Geom,
		Gate:       testGateConfig(),
	}
}

// TestFixVerifySpawnOrAttach pins the refusal without a report, the spawn's spec and record, the attach to a running fixer and the fresh spawn after a terminal one.
func TestFixVerifySpawnOrAttach(t *testing.T) {
	t.Parallel()

	t.Run("no verify-gate report refuses and spawns nothing", func(t *testing.T) {
		t.Parallel()
		fx, deps := newFixVerifyFixture(t, false)
		st := &websterengine.State{}
		_, spawned, err := websterengine.FixVerifySpawnOrAttach(deps, st, &recoverFakeClock{now: time.Unix(0, 0)})
		if err == nil || !strings.Contains(err.Error(), "way forward: run `lym webster fix-verify` only when") {
			t.Fatalf("FixVerifySpawnOrAttach() error = %v; want the no-report refusal with its way forward", err)
		}
		if spawned || len(fx.Reed.AddedSpecs) != 0 || len(st.VerifyFixers) != 0 {
			t.Errorf("spawned = %v, AddStrand calls = %d, records = %d; want no spawn and no record", spawned, len(fx.Reed.AddedSpecs), len(st.VerifyFixers))
		}
	})

	t.Run("a refusal before the spawn names its way forward and spawns nothing", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name     string
			arrange  func(deps *websterengine.FixVerifyDeps)
			wantText []string
		}{
			{
				name: "no resolved model-spec for the recovery role",
				arrange: func(deps *websterengine.FixVerifyDeps) {
					deps.Roles = map[websterengine.Role]shuttleengine.ModelChoice{}
				},
				wantText: []string{"no resolved model-spec", "way forward: set recovery in webster.yaml"},
			},
			{
				name: "recovery engine cannot audit forks",
				arrange: func(deps *websterengine.FixVerifyDeps) {
					deps.Engines = noAuditEngines{engine: &shuttlefake.Engine{}}
				},
				wantText: []string{"cannot audit forks", "way forward: set recovery in webster.yaml"},
			},
			{
				name: "gate config with no webster-fix key",
				arrange: func(deps *websterengine.FixVerifyDeps) {
					deps.Gate = gateslot.Config{
						Tiers: map[string]gateslot.Tier{"tier0": {About: "unit"}},
						Gates: map[string]gateslot.GateKey{"card": {Tiers: []int{0}, Scope: "card"}},
					}
				},
				wantText: []string{`gates holds no key "webster-fix"`, `fix it with "lym config gate"`},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				fx, deps := newFixVerifyFixture(t, true)
				tt.arrange(&deps)
				st := &websterengine.State{}
				_, spawned, err := websterengine.FixVerifySpawnOrAttach(deps, st, &recoverFakeClock{now: time.Unix(0, 0)})
				if err == nil {
					t.Fatal("FixVerifySpawnOrAttach() error = nil; want the refusal")
				}
				for _, want := range tt.wantText {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("FixVerifySpawnOrAttach() error = %q; want it to contain %q", err, want)
					}
				}
				if spawned || len(fx.Reed.AddedSpecs) != 0 || len(st.VerifyFixers) != 0 {
					t.Errorf("spawned = %v, AddStrand calls = %d, records = %d; want no spawn and no record", spawned, len(fx.Reed.AddedSpecs), len(st.VerifyFixers))
				}
			})
		}
	})

	t.Run("a spawned strand whose run cannot be resolved is stopped and unrecorded", func(t *testing.T) {
		t.Parallel()
		fx, deps := newFixVerifyFixture(t, true)
		deps.ShuttleCfg.RunDir = t.TempDir()
		st := &websterengine.State{}
		_, spawned, err := websterengine.FixVerifySpawnOrAttach(deps, st, &recoverFakeClock{now: time.Unix(0, 0)})
		if err == nil || !strings.Contains(err.Error(), "way forward: transient, re-run `lym webster fix-verify`") {
			t.Fatalf("FixVerifySpawnOrAttach() error = %v; want the resolve refusal with its way forward", err)
		}
		if spawned || len(st.VerifyFixers) != 0 {
			t.Errorf("spawned = %v, records = %d; want no record", spawned, len(st.VerifyFixers))
		}
		if len(fx.Reed.AddedSpecs) != 1 || len(fx.Reed.RemovedGUIDs) != 1 {
			t.Errorf("AddStrand calls = %d, RemoveStrand calls = %v; want the one spawned strand removed", len(fx.Reed.AddedSpecs), fx.Reed.RemovedGUIDs)
		}
	})

	t.Run("spawns at the recovery role, records the strand and attaches on the next call", func(t *testing.T) {
		t.Parallel()
		fx, deps := newFixVerifyFixture(t, true)
		replyPath := websterengine.VerifyFixReplyPath(deps.Geom.ScratchDir)
		if err := os.WriteFile(replyPath, []byte("stale"), 0o644); err != nil {
			t.Fatalf("seed stale reply: %v", err)
		}
		st := &websterengine.State{}
		clk := &recoverFakeClock{now: time.Unix(0, 0)}

		rec, spawned, err := websterengine.FixVerifySpawnOrAttach(deps, st, clk)
		if err != nil || !spawned {
			t.Fatalf("FixVerifySpawnOrAttach() = _, %v, %v; want a spawn", spawned, err)
		}
		if _, err := os.Stat(replyPath); !os.IsNotExist(err) {
			t.Errorf("stat stale reply = %v; want it removed before the spawn", err)
		}
		spec := fx.Engine.LastSpec
		if spec.Role != "verify-fixer" || spec.ModelChoice.Model != "recovery-model" || !slices.Equal(spec.OutputFiles, []string{replyPath}) {
			t.Errorf("spec role %q, model %q, output files %v; want verify-fixer, recovery-model, [%s]", spec.Role, spec.ModelChoice.Model, spec.OutputFiles, replyPath)
		}
		if !slices.Contains(spec.Skills, "scribe:prose") {
			t.Errorf("spec skills = %v; want the role skills", spec.Skills)
		}
		for _, want := range []string{websterengine.VerifyGateReportPath(fx.ReportsDir), replyPath, "lym gate test --wait --gate webster-fix"} {
			if !strings.Contains(fx.Engine.LastPrompt, want) {
				t.Errorf("prompt lacks %q", want)
			}
		}
		if len(st.VerifyFixers) != 1 || st.VerifyFixers[0] != rec {
			t.Fatalf("records = %+v; want the one spawned record", st.VerifyFixers)
		}
		if rec.StrandGUID == "" || rec.ShuttleRunDir == "" || rec.EventsPath == "" || rec.SpawnedAt != "1970-01-01T00:00:00Z" || rec.Status != "" {
			t.Errorf("record = %+v; want the strand, run dir, events, the clock's spawn time and no status", rec)
		}

		again, spawned, err := websterengine.FixVerifySpawnOrAttach(deps, st, clk)
		if err != nil || spawned || again != rec || len(fx.Reed.AddedSpecs) != 1 {
			t.Errorf("second call = %p, %v, %v with %d AddStrand calls; want an attach to %p and no second spawn", again, spawned, err, len(fx.Reed.AddedSpecs), rec)
		}
	})

	t.Run("a terminal fixer is followed by a fresh spawn", func(t *testing.T) {
		t.Parallel()
		fx, deps := newFixVerifyFixture(t, true)
		st := &websterengine.State{VerifyFixers: []*websterengine.VerifyFixerState{{StrandGUID: "old", Status: websterengine.DigestStatusDead}}}
		rec, spawned, err := websterengine.FixVerifySpawnOrAttach(deps, st, &recoverFakeClock{now: time.Unix(0, 0)})
		if err != nil || !spawned || rec.StrandGUID == "old" || len(st.VerifyFixers) != 2 || len(fx.Reed.AddedSpecs) != 1 {
			t.Errorf("FixVerifySpawnOrAttach() = %+v, %v, %v with records %d; want a fresh second record", rec, spawned, err, len(st.VerifyFixers))
		}
	})
}

// TestFixVerifyAwait pins the wait's classification and what each outcome stops, reads and removes.
func TestFixVerifyAwait(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// arrange acts on the spawned fixer before the wait.
		arrange    func(t *testing.T, fx *recoverFixture, deps websterengine.FixVerifyDeps, rec *websterengine.VerifyFixerState)
		wait       func(cfg websterengine.Config) time.Duration
		wantStatus string
		wantReason string
		wantReply  string
		wantStop   bool
	}{
		{
			name: "a reply file is done",
			arrange: func(t *testing.T, fx *recoverFixture, deps websterengine.FixVerifyDeps, _ *websterengine.VerifyFixerState) {
				if err := os.WriteFile(websterengine.VerifyFixReplyPath(deps.Geom.ScratchDir), []byte("fixed a"), 0o644); err != nil {
					t.Fatalf("write reply: %v", err)
				}
			},
			wait:       websterengine.RecoveryWaitBudget,
			wantStatus: websterengine.DigestStatusDone,
			wantReply:  "fixed a",
			wantStop:   true,
		},
		{
			name: "a turn that ended without a reply is dead asking",
			arrange: func(t *testing.T, fx *recoverFixture, _ websterengine.FixVerifyDeps, rec *websterengine.VerifyFixerState) {
				fx.Engine.Events = []shuttleengine.Event{{Kind: shuttleengine.EventStop}}
				f, err := os.OpenFile(rec.EventsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if err != nil {
					t.Fatalf("open events file: %v", err)
				}
				defer f.Close()
				if _, err := f.WriteString("{\"type\":\"stop\"}\n"); err != nil {
					t.Fatalf("append turn end: %v", err)
				}
			},
			wait:       websterengine.RecoveryWaitBudget,
			wantStatus: websterengine.DigestStatusDead,
			wantReason: websterengine.DeadReasonAsking,
			wantStop:   true,
		},
		{
			name:       "a live fixer without a reply is dead on the timeout",
			arrange:    func(*testing.T, *recoverFixture, websterengine.FixVerifyDeps, *websterengine.VerifyFixerState) {},
			wait:       websterengine.RecoveryWaitBudget,
			wantStatus: websterengine.DigestStatusDead,
			wantReason: websterengine.DeadReasonTimeout,
			wantStop:   true,
		},
		{
			name: "a gone strand is dead",
			arrange: func(_ *testing.T, fx *recoverFixture, _ websterengine.FixVerifyDeps, _ *websterengine.VerifyFixerState) {
				fx.Reed.Strands = nil
			},
			wait:       websterengine.RecoveryWaitBudget,
			wantStatus: websterengine.DigestStatusDead,
			wantReason: websterengine.DeadReasonDied,
		},
		{
			name:       "a shorter wait returns running and stops nothing",
			arrange:    func(*testing.T, *recoverFixture, websterengine.FixVerifyDeps, *websterengine.VerifyFixerState) {},
			wait:       func(websterengine.Config) time.Duration { return 2 * time.Second },
			wantStatus: websterengine.DigestStatusRunning,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx, deps := newFixVerifyFixture(t, true)
			st := &websterengine.State{}
			clk := &recoverFakeClock{now: time.Unix(0, 0)}
			rec, _, err := websterengine.FixVerifySpawnOrAttach(deps, st, clk)
			if err != nil {
				t.Fatalf("FixVerifySpawnOrAttach() = %v", err)
			}
			tt.arrange(t, fx, deps, rec)

			result, err := websterengine.FixVerifyAwait(deps, rec, tt.wait(deps.Config), clk)
			if err != nil {
				t.Fatalf("FixVerifyAwait() = %v", err)
			}
			if result.Status != tt.wantStatus || result.DeadReason != tt.wantReason || result.Reply != tt.wantReply {
				t.Errorf("FixVerifyAwait() = %+v; want status %q, reason %q, reply %q", result, tt.wantStatus, tt.wantReason, tt.wantReply)
			}
			if stopped := slices.Contains(fx.Reed.RemovedGUIDs, rec.StrandGUID); stopped != tt.wantStop {
				t.Errorf("strand stopped = %v; want %v", stopped, tt.wantStop)
			}
			_, statErr := os.Stat(rec.ShuttleRunDir)
			if removed := os.IsNotExist(statErr); removed != (tt.wantStatus == websterengine.DigestStatusDone) {
				t.Errorf("run dir removed = %v; want removed only on done", removed)
			}
			if tt.wantStatus == websterengine.DigestStatusRunning {
				return
			}
			if !websterengine.RecordVerifyFixerTerminal(st, rec.StrandGUID, result) || rec.Status != tt.wantStatus || rec.DeadReason != tt.wantReason {
				t.Errorf("record after RecordVerifyFixerTerminal = %+v; want status %q, reason %q", rec, tt.wantStatus, tt.wantReason)
			}
		})
	}

	if websterengine.RecordVerifyFixerTerminal(&websterengine.State{}, "absent", &websterengine.FixVerifyResult{Status: websterengine.DigestStatusDone}) {
		t.Error("RecordVerifyFixerTerminal() over a state without the fixer = true; want false")
	}
}
