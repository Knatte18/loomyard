//go:build tier0

// planfixer_test.go exercises the plan fixer's bounded spawn-or-attach and its verdict-reading wait over the recover-batch fixture: a real *shuttleengine.Runner over the shuttlefake Reed and Engine, a fake git and a fake Clock.

package websterengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// testConflictFindings are the findings the fixture's plan-conflict record names.
var testConflictFindings = []string{"glyph-not-found/03: base.go#OldName", "glyph-not-found/04: base.go#OldTest"}

// newFixPlanFixture returns the recover-batch fixture with a scratch dir and a parent, and the FixPlanDeps over its seams.
// findings, when non-empty, are written as begin-batch 02's tree_moved conflict record.
func newFixPlanFixture(t *testing.T, findings []string) (*recoverFixture, websterengine.FixPlanDeps) {
	t.Helper()
	fx := newRecoverFixture(t)
	fx.Deps.Geom.ScratchDir = t.TempDir()
	fx.Deps.Geom.ParentName = "ly:orch"
	fx.Git.head = "head-at-spawn"
	fx.Git.dirtyPaths = []string{"dirty-before.txt"}
	if len(findings) > 0 {
		conflict := &websterengine.PlanConflictError{Batch: 2, Cause: websterengine.ConflictCauseTreeMoved, Findings: findings}
		if err := websterengine.WriteConflictRecord(fx.Deps.Geom.ScratchDir, conflict); err != nil {
			t.Fatalf("WriteConflictRecord() = %v", err)
		}
	}
	return fx, websterengine.FixPlanDeps{
		Starter:    fx.Deps.Starter,
		Roles:      fx.Deps.Roles,
		Config:     fx.Deps.Config,
		Engines:    fx.Deps.Engines,
		Reed:       fx.Deps.Reed,
		Stopper:    fx.Deps.Stopper,
		ShuttleCfg: fx.Deps.ShuttleCfg,
		Geom:       fx.Deps.Geom,
	}
}

// TestFixPlanSpawnOrAttach pins the refusals and their bounds, the spawn's spec, prompt and record, and the attach to a running fixer.
func TestFixPlanSpawnOrAttach(t *testing.T) {
	t.Parallel()

	t.Run("a refusal names its way forward and spawns nothing", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name     string
			findings []string
			earlier  []*websterengine.PlanFixerState
			arrange  func(fx *recoverFixture, deps *websterengine.FixPlanDeps)
			// wantAdded is the number of AddStrand calls reed saw before the refusal.
			wantAdded int
			wantText  []string
		}{
			{
				name:     "no plan-conflict record",
				wantText: []string{"no plan-conflict record names a finding", "way forward: run `lym webster fix-plan` only right after begin-batch refuses with plan_conflict"},
			},
			{
				name:     "an earlier fixer was given the same findings",
				findings: testConflictFindings,
				earlier:  []*websterengine.PlanFixerState{{StrandGUID: "old", Batch: 2, Findings: []string{testConflictFindings[1], testConflictFindings[0]}, Status: websterengine.DigestStatusDone}},
				wantText: []string{"already given these plan-conflict findings", "way forward: write outcome: stuck", "goes to the plan writer"},
			},
			{
				name:     "the batch already had two fixers",
				findings: testConflictFindings,
				earlier: []*websterengine.PlanFixerState{
					{StrandGUID: "a", Batch: 2, Findings: []string{"path-missing/03: a.go"}, Status: websterengine.DigestStatusDone},
					{StrandGUID: "b", Batch: 2, Findings: []string{"path-missing/03: b.go"}, Status: websterengine.DigestStatusDead},
				},
				wantText: []string{"batch 02 already had 2 plan fixers", "way forward: write outcome: stuck"},
			},
			{
				name:     "no resolved model-spec for the master role",
				findings: testConflictFindings,
				arrange: func(_ *recoverFixture, deps *websterengine.FixPlanDeps) {
					deps.Roles = map[websterengine.Role]shuttleengine.ModelChoice{}
				},
				wantText: []string{"no resolved model-spec", "way forward: set master in webster.yaml"},
			},
			{
				name:     "master engine cannot audit forks",
				findings: testConflictFindings,
				arrange: func(_ *recoverFixture, deps *websterengine.FixPlanDeps) {
					deps.Engines = noAuditEngines{engine: &shuttlefake.Engine{}}
				},
				wantText: []string{"cannot audit forks", "way forward: set master in webster.yaml"},
			},
			{
				name:     "HEAD cannot be read",
				findings: testConflictFindings,
				arrange:  func(fx *recoverFixture, _ *websterengine.FixPlanDeps) { fx.Git.headErr = errors.New("head boom") },
				wantText: []string{"head boom", "way forward: transient, re-run `lym webster fix-plan`"},
			},
			{
				name:     "the worktree status cannot be read",
				findings: testConflictFindings,
				arrange: func(fx *recoverFixture, _ *websterengine.FixPlanDeps) {
					fx.Git.dirtyPathsErr = errors.New("status boom")
				},
				wantText: []string{"status boom", "way forward: transient, re-run `lym webster fix-plan`"},
			},
			{
				name:      "the strand cannot be started",
				findings:  testConflictFindings,
				arrange:   func(fx *recoverFixture, _ *websterengine.FixPlanDeps) { fx.Reed.AddErr = errors.New("add boom") },
				wantText:  []string{"add boom", "way forward: transient, re-run `lym webster fix-plan`"},
				wantAdded: 1,
			},
			{
				name:     "the spawned run cannot be found",
				findings: testConflictFindings,
				arrange: func(_ *recoverFixture, deps *websterengine.FixPlanDeps) {
					deps.ShuttleCfg.RunDir = filepath.Join(deps.Geom.ScratchDir, "elsewhere")
				},
				wantText:  []string{"resolve spawned plan fixer run", "way forward: transient, re-run `lym webster fix-plan`"},
				wantAdded: 1,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				fx, deps := newFixPlanFixture(t, tt.findings)
				if tt.arrange != nil {
					tt.arrange(fx, &deps)
				}
				st := &websterengine.State{PlanFixers: tt.earlier}
				_, spawned, err := websterengine.FixPlanSpawnOrAttach(deps, st, &recoverFakeClock{now: time.Unix(0, 0)})
				if err == nil {
					t.Fatal("FixPlanSpawnOrAttach() error = nil; want the refusal")
				}
				for _, want := range tt.wantText {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("FixPlanSpawnOrAttach() error = %q; want it to contain %q", err, want)
					}
				}
				if spawned || len(fx.Reed.AddedSpecs) != tt.wantAdded || len(st.PlanFixers) != len(tt.earlier) {
					t.Errorf("spawned = %v, AddStrand calls = %d, records = %d; want no spawn, %d AddStrand calls and no new record", spawned, len(fx.Reed.AddedSpecs), len(st.PlanFixers), tt.wantAdded)
				}
			})
		}
	})

	t.Run("spawns at the master role, records the conflict and attaches on the next call", func(t *testing.T) {
		t.Parallel()
		fx, deps := newFixPlanFixture(t, testConflictFindings)
		replyPath := websterengine.PlanFixReplyPath(deps.Geom.ScratchDir)
		if err := os.WriteFile(replyPath, []byte("stale"), 0o644); err != nil {
			t.Fatalf("seed stale reply: %v", err)
		}
		st := &websterengine.State{PlanFixers: []*websterengine.PlanFixerState{{StrandGUID: "other-batch", Batch: 1, Findings: []string{"path-missing/01: x.go"}, Status: websterengine.DigestStatusDone}}}
		clk := &recoverFakeClock{now: time.Unix(0, 0)}

		rec, spawned, err := websterengine.FixPlanSpawnOrAttach(deps, st, clk)
		if err != nil || !spawned {
			t.Fatalf("FixPlanSpawnOrAttach() = _, %v, %v; want a spawn", spawned, err)
		}
		if _, err := os.Stat(replyPath); !os.IsNotExist(err) {
			t.Errorf("stat stale reply = %v; want it removed before the spawn", err)
		}
		spec := fx.Engine.LastSpec
		if spec.Role != "plan-fixer" || spec.ModelChoice.Model != "master-model" || !slices.Equal(spec.OutputFiles, []string{replyPath}) {
			t.Errorf("spec role %q, model %q, output files %v; want plan-fixer, master-model, [%s]", spec.Role, spec.ModelChoice.Model, spec.OutputFiles, replyPath)
		}
		if !slices.Contains(spec.Skills, "scribe:prose") {
			t.Errorf("spec skills = %v; want the role skills", spec.Skills)
		}
		for _, want := range []string{websterengine.ConflictRecordPath(deps.Geom.ScratchDir), replyPath, deps.Geom.SpecsDir + "/loom/loom-plan-spec.md", "ly:orch"} {
			if !strings.Contains(fx.Engine.LastPrompt, want) {
				t.Errorf("prompt lacks %q", want)
			}
		}
		if len(st.PlanFixers) != 2 || st.PlanFixers[1] != rec {
			t.Fatalf("records = %+v; want the spawned record appended", st.PlanFixers)
		}
		if rec.StrandGUID == "" || rec.ShuttleRunDir == "" || rec.EventsPath == "" || rec.SpawnedAt != "1970-01-01T00:00:00Z" || rec.Status != "" {
			t.Errorf("record = %+v; want the strand, run dir, events, the clock's spawn time and no status", rec)
		}
		if rec.Batch != 2 || !slices.Equal(rec.Findings, testConflictFindings) || rec.HeadAtSpawn != "head-at-spawn" || !slices.Equal(rec.DirtyAtSpawn, []string{"dirty-before.txt"}) {
			t.Errorf("record batch %d, findings %v, head %q, dirty %v; want 2, %v, head-at-spawn, [dirty-before.txt]", rec.Batch, rec.Findings, rec.HeadAtSpawn, rec.DirtyAtSpawn, testConflictFindings)
		}

		again, spawned, err := websterengine.FixPlanSpawnOrAttach(deps, st, clk)
		if err != nil || spawned || again != rec || len(fx.Reed.AddedSpecs) != 1 {
			t.Errorf("second call = %p, %v, %v with %d AddStrand calls; want an attach to %p and no second spawn", again, spawned, err, len(fx.Reed.AddedSpecs), rec)
		}
	})
}

// TestFixPlanAwait pins the verdict a terminal wait reads, the overrule of a fixed verdict that touched code, and the terminal record.
func TestFixPlanAwait(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// reply, when non-empty, is written as the fixer's reply before the wait.
		reply string
		// arrange acts on the fixture after the spawn and before the wait.
		arrange     func(fx *recoverFixture)
		wantStatus  string
		wantReason  string
		wantVerdict string
		wantWarning string
	}{
		{
			name:        "a fixed reply is fixed",
			reply:       "verdict: fixed\n- 03: OldName -> NewName\n",
			wantStatus:  websterengine.DigestStatusDone,
			wantVerdict: websterengine.PlanFixVerdictFixed,
		},
		{
			name:        "a plan_writer reply is plan_writer",
			reply:       "verdict: plan_writer\n- 04: the test was removed\n",
			wantStatus:  websterengine.DigestStatusDone,
			wantVerdict: websterengine.PlanFixVerdictPlanWriter,
		},
		{
			name:        "a reply without a verdict line is plan_writer",
			reply:       "fixed both cards\n",
			wantStatus:  websterengine.DigestStatusDone,
			wantVerdict: websterengine.PlanFixVerdictPlanWriter,
		},
		{
			name:        "a fixed reply after HEAD moved is overruled",
			reply:       "verdict: fixed\n",
			arrange:     func(fx *recoverFixture) { fx.Git.head = "moved-head" },
			wantStatus:  websterengine.DigestStatusDone,
			wantVerdict: websterengine.PlanFixVerdictPlanWriter,
			wantWarning: "HEAD moved from head-at-spawn to moved-head",
		},
		{
			name:        "a fixed reply that left a new uncommitted path is overruled",
			reply:       "verdict: fixed\n",
			arrange:     func(fx *recoverFixture) { fx.Git.dirtyPaths = []string{"dirty-before.txt", "base.go"} },
			wantStatus:  websterengine.DigestStatusDone,
			wantVerdict: websterengine.PlanFixVerdictPlanWriter,
			wantWarning: "it left uncommitted changes in base.go",
		},
		{
			name:       "a gone strand is dead with no verdict",
			arrange:    func(fx *recoverFixture) { fx.Reed.Strands = nil },
			wantStatus: websterengine.DigestStatusDead,
			wantReason: websterengine.DeadReasonDied,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx, deps := newFixPlanFixture(t, testConflictFindings)
			st := &websterengine.State{}
			clk := &recoverFakeClock{now: time.Unix(0, 0)}
			rec, _, err := websterengine.FixPlanSpawnOrAttach(deps, st, clk)
			if err != nil {
				t.Fatalf("FixPlanSpawnOrAttach() = %v", err)
			}
			if tt.reply != "" {
				if err := os.WriteFile(websterengine.PlanFixReplyPath(deps.Geom.ScratchDir), []byte(tt.reply), 0o644); err != nil {
					t.Fatalf("write reply: %v", err)
				}
			}
			if tt.arrange != nil {
				tt.arrange(fx)
			}

			result, err := websterengine.FixPlanAwait(deps, rec, websterengine.RecoveryWaitBudget(deps.Config), clk)
			if err != nil {
				t.Fatalf("FixPlanAwait() = %v", err)
			}
			if result.Status != tt.wantStatus || result.DeadReason != tt.wantReason || result.Verdict != tt.wantVerdict || result.Reply != tt.reply {
				t.Errorf("FixPlanAwait() = %+v; want status %q, reason %q, verdict %q, reply %q", result, tt.wantStatus, tt.wantReason, tt.wantVerdict, tt.reply)
			}
			warned := slices.ContainsFunc(result.Warnings, func(w string) bool { return strings.Contains(w, "overruled") })
			if tt.wantWarning == "" && warned || tt.wantWarning != "" && !slices.ContainsFunc(result.Warnings, func(w string) bool { return strings.Contains(w, tt.wantWarning) }) {
				t.Errorf("warnings = %v; want an overrule warning containing %q only when one is expected", result.Warnings, tt.wantWarning)
			}
			if !slices.Contains(fx.Reed.RemovedGUIDs, rec.StrandGUID) && tt.wantStatus == websterengine.DigestStatusDone {
				t.Error("strand not stopped; want every done fixer stopped")
			}
			if !websterengine.RecordPlanFixerTerminal(st, rec.StrandGUID, result) || rec.Status != tt.wantStatus || rec.DeadReason != tt.wantReason || rec.Verdict != tt.wantVerdict {
				t.Errorf("record after RecordPlanFixerTerminal = %+v; want status %q, reason %q, verdict %q", rec, tt.wantStatus, tt.wantReason, tt.wantVerdict)
			}
		})
	}

	if websterengine.RecordPlanFixerTerminal(&websterengine.State{}, "absent", &websterengine.FixPlanResult{Status: websterengine.DigestStatusDone}) {
		t.Error("RecordPlanFixerTerminal() over a state without the fixer = true; want false")
	}
}
