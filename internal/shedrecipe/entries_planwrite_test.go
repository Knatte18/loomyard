// entries_planwrite_test.go covers planWriteEntry: its construction-time validation over Config and the Env fields it reads,
// and the Calls proving the told table is evaluated once, carries the row's gate, reaches the seat runner with the prior-plan amendment on a fresh spawn only, and the commit closure fires.

package shedrecipe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// withPlanTable fills env.PlanTable with a one-lead table writing output and returns the count of times the source was evaluated.
func withPlanTable(env *Env, output string) *int {
	evaluations := 0
	env.PlanTable = func() (seatengine.Table, error) {
		evaluations++
		return seatengine.Table{
			RolePrefix: "plan",
			Segment:    segmentcolor.Plan,
			Seats:      []seatengine.Seat{{Stencil: "loom-template-plan", Outputs: []string{output}}},
		}, nil
	}
	return &evaluations
}

// TestPlanWriteEntry_ConstructionFailures covers the three seams planWriteEntry requires, the
// AnchorPath root it validates, and the Config rejection every entry in this package performs
// first. A bare zero-value assignment on a copy of newTestEnv(t)'s Env follows
// entries_discussionwrite_test.go's precedent: PlanTable and CommitPlan are both concrete named func
// types, so env.PlanTable = nil already boxes as a typed-nil when it reaches requireSeam's any
// parameter, making the plain-nil branch unreachable and a second "typed-nil versus untyped-nil"
// variant unnecessary.
func TestPlanWriteEntry_ConstructionFailures(t *testing.T) {
	t.Run("NilPlanTable", func(t *testing.T) {
		env := newTestEnv(t)
		env.PlanTable = nil
		_, err := planWriteEntry("Row", Config{}, env)
		if err == nil {
			t.Fatalf("planWriteEntry() error = nil; want non-nil when Env.PlanTable is nil")
		}
		if !strings.Contains(err.Error(), "PlanWrite") || !strings.Contains(err.Error(), "PlanTable") {
			t.Errorf("planWriteEntry() error = %v; want it to name entry %q and field %q", err, "PlanWrite", "PlanTable")
		}
	})

	t.Run("NilCommitPlan", func(t *testing.T) {
		env := newTestEnv(t)
		env.CommitPlan = nil
		_, err := planWriteEntry("Row", Config{}, env)
		if err == nil {
			t.Fatalf("planWriteEntry() error = nil; want non-nil when Env.CommitPlan is nil")
		}
		if !strings.Contains(err.Error(), "PlanWrite") || !strings.Contains(err.Error(), "CommitPlan") {
			t.Errorf("planWriteEntry() error = %v; want it to name entry %q and field %q", err, "PlanWrite", "CommitPlan")
		}
	})

	t.Run("NilSeats", func(t *testing.T) {
		env := newTestEnv(t)
		env.Seats = nil
		_, err := planWriteEntry("Row", Config{}, env)
		if err == nil {
			t.Fatalf("planWriteEntry() error = nil; want non-nil when Env.Seats is nil")
		}
		if !strings.Contains(err.Error(), "PlanWrite") || !strings.Contains(err.Error(), "Seats") {
			t.Errorf("planWriteEntry() error = %v; want it to name entry %q and field %q", err, "PlanWrite", "Seats")
		}
	})

	t.Run("EmptyAnchorPath", func(t *testing.T) {
		env := newTestEnv(t)
		env.AnchorPath = ""
		_, err := planWriteEntry("Row", Config{}, env)
		if err == nil {
			t.Fatalf("planWriteEntry() error = nil; want non-nil when Env.AnchorPath is empty")
		}
		if !strings.Contains(err.Error(), "PlanWrite") || !strings.Contains(err.Error(), "AnchorPath") {
			t.Errorf("planWriteEntry() error = %v; want it to name entry %q and field %q", err, "PlanWrite", "AnchorPath")
		}
	})

	t.Run("NilArchiveWebster", func(t *testing.T) {
		env := newTestEnv(t)
		env.Rework.ArchiveWebster = nil
		_, err := planWriteEntry("Row", Config{}, env)
		if err == nil {
			t.Fatalf("planWriteEntry() error = nil; want non-nil when Env.Rework.ArchiveWebster is nil")
		}
		if !strings.Contains(err.Error(), "PlanWrite") || !strings.Contains(err.Error(), "Rework.ArchiveWebster") {
			t.Errorf("planWriteEntry() error = %v; want it to name entry %q and field %q", err, "PlanWrite", "Rework.ArchiveWebster")
		}
	})

	t.Run("EmptyStencilsDir", func(t *testing.T) {
		env := newTestEnv(t)
		env.StencilsDir = ""
		_, err := planWriteEntry("Row", Config{}, env)
		if err == nil {
			t.Fatalf("planWriteEntry() error = nil; want non-nil when Env.StencilsDir is empty")
		}
		if !strings.Contains(err.Error(), "PlanWrite") || !strings.Contains(err.Error(), "StencilsDir") {
			t.Errorf("planWriteEntry() error = %v; want it to name entry %q and field %q", err, "PlanWrite", "StencilsDir")
		}
	})
}

// TestPlanWriteEntry_HappyPath asserts a fully-filled Env constructs a non-nil producer with a nil
// error.
func TestPlanWriteEntry_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	producer, err := planWriteEntry("Row", Config{}, env)
	if err != nil {
		t.Fatalf("planWriteEntry() error = %v; want nil", err)
	}
	if producer == nil {
		t.Fatalf("planWriteEntry() = nil producer; want non-nil")
	}
}

// TestPlanWriteEntry_CallDone drives the happy-path producer's Call once against a done lead.
// It asserts the told table was evaluated once and reached the seat runner with the row's gate, the returned OutputPointer.Path is the lead's first output, and the injected commit closure fired exactly once.
// env.AnchorPath from newTestEnv is a real directory that contains no plan directory, so the rotation is a legitimate absent-directory no-op here.
func TestPlanWriteEntry_CallDone(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	output := filepath.Join(env.AnchorPath, "00-overview.md")
	evaluations := withPlanTable(&env, output)
	commitCalls := 0
	env.CommitPlan = func() error {
		commitCalls++
		return nil
	}
	fake := &shedfake.SeatRunner{Results: []seatengine.Result{leadDoneResult(output)}}
	env.Seats = fake

	producer, err := planWriteEntry("Row", gatesCfg("plan", 2), env)
	if err != nil {
		t.Fatalf("planWriteEntry() error = %v; want nil", err)
	}
	pointer := shedfake.RequireOutcome(t, producer, shedengine.Done)

	if *evaluations != 1 {
		t.Errorf("table source evaluated %d times; want 1", *evaluations)
	}
	if len(fake.GotTables) != 1 {
		t.Fatalf("seat runner saw %d tables; want 1", len(fake.GotTables))
	}
	gate := fake.GotTables[0].Gate
	if len(gate) != 1 || gate[0].Name != "plan" || gate[0].Attempts != 2 || gate[0].Forked == nil {
		t.Errorf("table gate = %+v; want one \"plan\" entry with 2 attempts and the lens-review closure", gate)
	}
	if pointer.Path != output {
		t.Errorf("Call() OutputPointer.Path = %q; want the lead's first output %q", pointer.Path, output)
	}
	if commitCalls != 1 {
		t.Errorf("commit closure invoked %d times; want exactly 1", commitCalls)
	}
}

// TestPlanWriteEntry_CallAppendsPriorPlanBlock seeds a stale plan file under env.AnchorPath and a minimal prior-plan stencil into env.StencilsDir.
// A fresh spawn archives the stale plan and hands the seat runner a table whose Amendment is the rendered block;
// a live lead resumes with the stale plan left in place, no table run, and a table that carries no amendment.
func TestPlanWriteEntry_CallAppendsPriorPlanBlock(t *testing.T) {
	t.Parallel()
	seedStalePlan := func(t *testing.T) (env Env, planDir string, archived *[]string) {
		t.Helper()
		env = newTestEnv(t)
		writeStencil(t, env.StencilsDir, "loom-template-prior-plan", "PRIOR {{.archive_dir}}\n{{.moved_files}}\n")
		planDir = planparser.PlanDir(env.AnchorPath)
		if err := os.MkdirAll(planDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", planDir, err)
		}
		if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte("stale"), 0o644); err != nil {
			t.Fatalf("write stale plan: %v", err)
		}
		withPlanTable(&env, filepath.Join(planDir, "00-overview.md"))
		env.CommitPlan = func() error { return nil }
		archived = new([]string)
		env.Rework.ArchiveWebster = func(dest string) error {
			*archived = append(*archived, dest)
			return nil
		}
		return env, planDir, archived
	}

	t.Run("FreshSpawnCarriesTheBlockAsAmendment", func(t *testing.T) {
		t.Parallel()
		env, planDir, archived := seedStalePlan(t)
		fake := &shedfake.SeatRunner{Results: []seatengine.Result{leadDoneResult(filepath.Join(planDir, "00-overview.md"))}}
		env.Seats = fake

		producer, err := planWriteEntry("Row", Config{}, env)
		if err != nil {
			t.Fatalf("planWriteEntry() error = %v; want nil", err)
		}
		if _, _, err := producer.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}

		if len(fake.GotTables) != 1 {
			t.Fatalf("seat runner saw %d tables; want 1", len(fake.GotTables))
		}
		if len(*archived) != 1 || filepath.Base((*archived)[0]) != "webster" || filepath.Dir(filepath.Dir((*archived)[0])) != planDir {
			t.Errorf("ArchiveWebster calls = %v; want one call into an archive directory's webster subdirectory under %s", *archived, planDir)
		}
		amendment := fake.GotTables[0].Amendment
		if !strings.HasPrefix(amendment, "\n") {
			t.Errorf("amendment = %q; want it to start with a blank line", amendment)
		}
		for _, want := range []string{"PRIOR " + planDir, "`00-overview.md`"} {
			if !strings.Contains(amendment, want) {
				t.Errorf("amendment = %q; want it to contain %q", amendment, want)
			}
		}
	})

	t.Run("LiveLeadResumesWithoutRotatingOrAmending", func(t *testing.T) {
		t.Parallel()
		env, planDir, archived := seedStalePlan(t)
		output := filepath.Join(planDir, "00-overview.md")
		fake := &shedfake.SeatRunner{
			LiveTables:    []seatengine.LiveTable{{Chair: liveSeatLead}},
			ResumeResults: []seatengine.Result{leadDoneResult(output)},
		}
		env.Seats = fake

		producer, err := planWriteEntry("Row", Config{}, env)
		if err != nil {
			t.Fatalf("planWriteEntry() error = %v; want nil", err)
		}
		if _, _, err := producer.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v; want nil", err)
		}

		if got, err := os.ReadFile(output); err != nil || string(got) != "stale" {
			t.Errorf("plan overview = %q, %v; want the stale file left in place", got, err)
		}
		if len(*archived) != 0 {
			t.Errorf("ArchiveWebster calls = %v; want none for a resume", *archived)
		}
		if len(fake.GotTables) != 0 {
			t.Errorf("seat runner ran %d tables; want none for a resume", len(fake.GotTables))
		}
		if len(fake.GotResumeTables) != 1 || fake.GotResumeTables[0].Amendment != "" {
			t.Errorf("resumed tables = %+v; want one carrying no amendment", fake.GotResumeTables)
		}
	})
}

// TestPlanWriteEntry_GateConfig covers the "gates" Config key planWriteEntry resolves through resolveGateSpec:
// a "plan" entry resolves to the plan validator.
func TestPlanWriteEntry_GateConfig(t *testing.T) {
	t.Run("GatePlanResolvesToPlanValidator", func(t *testing.T) {
		env := newTestEnv(t)
		gateSpec, err := resolveGateSpec("PlanWrite", gatesCfg("plan", 3), env)
		if err != nil {
			t.Fatalf("resolveGateSpec() error = %v; want nil", err)
		}
		if len(gateSpec) != 1 || gateSpec[0].Gate == nil {
			t.Fatalf("resolveGateSpec() = %+v; want one entry carrying the plan closure", gateSpec)
		}

		// Drive the constructed gate rather than comparing func values, which Go cannot compare.
		// newTestEnv's AnchorPath is a real, empty directory carrying no _lyx/plan/00-overview.md,
		// so the plan validator reports the overview not found -- a finding text no discussion-gate
		// failure could ever produce, which is what proves the plan closure and not the discussion
		// one was resolved.
		result, err := gateSpec[0].Gate()
		if err != nil {
			t.Fatalf("gateSpec[0].Gate() error = %v; want nil", err)
		}
		if result.Passed {
			t.Fatal("gateSpec[0].Gate() Passed = true; want false for a missing plan overview")
		}
		if !strings.Contains(result.Findings, "plan overview not found") {
			t.Errorf("gateSpec[0].Gate() Findings = %q; want it to name the missing plan overview, proving the plan validator ran", result.Findings)
		}
	})

	t.Run("PlanWriteForkedGateChecksSymbolRenamesAgainstTheEnvsRenameTools", func(t *testing.T) {
		for _, tt := range []struct {
			name  string
			tools []string
			want  bool
		}{
			{"no rename tool", nil, true},
			{"a rename tool", []string{"gopls rename"}, false},
		} {
			t.Run(tt.name, func(t *testing.T) {
				env := newTestEnv(t)
				env.PlanIndex = noFindingsPlanIndex{}
				env.RenameTools = tt.tools
				plankit.Write(t, planparser.PlanDir(env.AnchorPath), symbolRenamePlan())

				gateSpec, err := planWriteGateSpec(gatesCfg("plan", 3), env)
				if err != nil {
					t.Fatalf("planWriteGateSpec() error = %v; want nil", err)
				}
				result, err := gateSpec[0].Forked()
				if err != nil {
					t.Fatalf("Forked() error = %v; want nil", err)
				}
				if got := strings.Contains(result.Findings, "rename-tool-missing"); got != tt.want {
					t.Errorf("Forked() = %+v; want rename-tool-missing: %v", result, tt.want)
				}
			})
		}
	})

	t.Run("PlanWriteSetsForkedOnItsPlanEntry", func(t *testing.T) {
		env := newTestEnv(t)
		env.PlanIndex = noFindingsPlanIndex{}
		plankit.Write(t, planparser.PlanDir(env.AnchorPath), plankit.Plan{
			Approved: true,
			Language: "none",
			Framing:  "Framing.",
			Cards: []plankit.Card{{
				Number:  1,
				Slug:    "first-card",
				Summary: "placeholder",
				Groups:  []plankit.Group{{Label: "Create", Targets: []string{"internal/x/new.go"}}},
				Intent:  "placeholder.",
			}},
		})

		gateSpec, err := planWriteGateSpec(gatesCfg("plan", 3), env)
		if err != nil {
			t.Fatalf("planWriteGateSpec() error = %v; want nil", err)
		}
		if len(gateSpec) != 1 || gateSpec[0].Gate == nil || gateSpec[0].Forked == nil {
			t.Fatalf("planWriteGateSpec() = %+v; want one entry carrying Gate and Forked", gateSpec)
		}

		result, err := gateSpec[0].Gate()
		if err != nil || !result.Passed {
			t.Fatalf("Gate() = %+v, %v; want a pass for a plan the plain plan gate accepts", result, err)
		}
		result, err = gateSpec[0].Forked()
		if err != nil {
			t.Fatalf("Forked() error = %v; want nil", err)
		}
		if result.Passed || !strings.Contains(result.Findings, "lens-review-missing") {
			t.Errorf("Forked() = %+v; want a failure naming lens-review-missing, proving the lens-review closure", result)
		}
	})
}
