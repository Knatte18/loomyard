// bouncer_seed_test.go covers Bouncer.Call's seed call, seed-side harvest, and the re-bounce mode,
// plus the prompt-shape guarantees shared by both templates: marker completeness against the
// shipped stencils package bytes and the stamp-leak regression.
// The judge, replay, harvest-on-judge, degradation, pointer-discipline, stale-output, and
// cancellation scenarios are deliberately left to batch 4, which adds only test files.

package shedadapters

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/parentdirective"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// TestBouncer_SeedCall_HappyPath also pins the spec the seed passes the shuttle: its role and
// round, the configured model, effort and version, and absolute output paths.
//
//testtiming:keep pins the seed call's spec, pointer and focus file, and that it writes no verdict or ledger
func TestBouncer_SeedCall_HappyPath(t *testing.T) {
	shuttle := &shedfake.Shuttle{
		Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	shuttle.DuringRun = func() {
		path := shuttle.GotSpec.OutputFiles[0]
		content := "---\nround: 1\nexclude_lenses: []\nfocus: [\"check the thing\"]\n---\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
	}
	b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()

	ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if !shuttle.Called {
		t.Error("Call() did not invoke the shuttle seam")
	}
	if shuttle.GotSpec.Role != "bouncer-seed" {
		t.Errorf("recorded spec.Role = %q; want %q", shuttle.GotSpec.Role, "bouncer-seed")
	}
	if shuttle.GotSpec.Round != "1" {
		t.Errorf("seed call spec.Round = %q; want %q", shuttle.GotSpec.Round, "1")
	}
	if shuttle.GotSpec.Model != cfg.Model {
		t.Errorf("recorded spec.Model = %q; want %q", shuttle.GotSpec.Model, cfg.Model)
	}
	if shuttle.GotSpec.Effort != cfg.Effort {
		t.Errorf("recorded spec.Effort = %q; want %q", shuttle.GotSpec.Effort, cfg.Effort)
	}
	if shuttle.GotSpec.Version != cfg.Version {
		t.Errorf("recorded spec.Version = %q; want %q", shuttle.GotSpec.Version, cfg.Version)
	}
	if len(shuttle.GotSpec.OutputFiles) == 0 {
		t.Fatal("recorded spec.OutputFiles is empty")
	}
	for _, f := range shuttle.GotSpec.OutputFiles {
		if !filepath.IsAbs(f) {
			t.Errorf("recorded spec.OutputFiles entry %q is not absolute", f)
		}
	}

	focusRaw, err := os.ReadFile(focusPath(cfg.RunDir, 1))
	if err != nil {
		t.Fatalf("ReadFile(round-1-focus.md) = %v; want nil", err)
	}
	parsed, err := parseFocus(focusRaw)
	if err != nil {
		t.Fatalf("parseFocus(...) error = %v; want nil", err)
	}
	if parsed.Round != 1 {
		t.Errorf("parsed focus Round = %d; want 1", parsed.Round)
	}

	if _, err := os.Stat(verdictPath(cfg.RunDir, 1)); !os.IsNotExist(err) {
		t.Errorf("verdict file exists after a seed call: %v", err)
	}
	if _, err := os.Stat(ledgerPath(cfg.RunDir, 1)); !os.IsNotExist(err) {
		t.Errorf("ledger file exists after a seed call: %v", err)
	}
}

//testtiming:keep pins that a present but unparseable focus file is archived and re-seeded rather than treated as a re-bounce
func TestBouncer_SeedDiscriminator_ParsesRatherThanStats(t *testing.T) {
	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()

	// round-1-focus.md present but unparseable, with no report on disk.
	if err := os.WriteFile(focusPath(cfg.RunDir, 1), []byte("not frontmatter at all"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) = %v; want nil", err)
	}

	shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if !shuttle.Called {
		t.Error("Call() treated an unparseable-but-present focus file as a re-bounce (fake not invoked); want a seed call")
	}

	// The malformed file was archived, not left in place.
	entries, err := os.ReadDir(cfg.RunDir)
	if err != nil {
		t.Fatalf("ReadDir(...) = %v; want nil", err)
	}
	archived := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "round-1-focus-") {
			archived = true
		}
	}
	if !archived {
		t.Error("malformed round-1-focus.md was not archived")
	}
}

func TestBouncer_SeedCall_SpawnProducedNothingUsable(t *testing.T) {
	tests := []struct {
		name         string
		buildBouncer func(t *testing.T) (*Bouncer, BouncerConfig)
	}{
		{
			name: "SeedTemplateUnreadable",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				return newBouncerFixture(t,
					withBareConfig(),
					withoutStencils("bouncer-template-seed"),
					withShuttle(&shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}),
					withClock(fixedClock(time.Now())),
				).Build()
			},
		},
		{
			name: "RubricUnreadable",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				// The constructor needs a readable rubric;
				// deleting it before Call makes the seed spawn, not construction, fail.
				b, cfg := newBouncerFixture(t,
					withBareConfig(),
					withShuttle(&shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}),
					withClock(fixedClock(time.Now())),
				).Build()
				if err := os.Remove(filepath.Join(cfg.StencilsDir, "bouncer", "bouncer-template-rubric.md")); err != nil {
					t.Fatalf("Remove(rubric) = %v; want nil", err)
				}
				return b, cfg
			},
		},
		{
			name: "FillFailure",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				return newBouncerFixture(t,
					withBareConfig(),
					withStencilOverrides(map[string]string{
						// Declares a marker the Go side does not supply.
						"bouncer-template-seed": "# Seed\n\n{{.rubric}} {{.artifacts}} {{.round}} {{.focus_path}} {{.unknown_marker}}\n",
					}),
					withShuttle(&shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}),
					withClock(fixedClock(time.Now())),
				).Build()
			},
		},
		{
			name: "RunError",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				return newBouncerFixture(t, withShuttle(&shedfake.Shuttle{Err: errors.New("run exploded")})).Build()
			},
		},
		{
			name: "NonOutcomeDone",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				return newBouncerFixture(t, withShuttle(&shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}})).Build()
			},
		},
		{
			name: "AgentWroteNothing",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				return newBouncerFixture(t, withShuttle(&shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}})).Build()
			},
		},
		{
			name: "AgentWroteUnparseableFocus",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
				shuttle.DuringRun = func() {
					path := shuttle.GotSpec.OutputFiles[0]
					if err := os.WriteFile(path, []byte("garbage, not frontmatter"), 0o644); err != nil {
						t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
					}
				}
				return newBouncerFixture(t, withShuttle(shuttle)).Build()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, cfg := tt.buildBouncer(t)

			ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
			if ptr != (shedengine.OutputPointer{}) {
				t.Errorf("Call() pointer = %+v; want empty", ptr)
			}

			focusRaw, err := os.ReadFile(focusPath(cfg.RunDir, 1))
			if err != nil {
				t.Fatalf("ReadFile(round-1-focus.md) = %v; want nil (ensureFocus must synthesize a fallback)", err)
			}
			parsed, err := parseFocus(focusRaw)
			if err != nil {
				t.Fatalf("parseFocus(...) error = %v; want nil", err)
			}
			if parsed.Round != 1 {
				t.Errorf("parsed focus Round = %d; want 1", parsed.Round)
			}
			if len(parsed.ExcludeLenses) != 0 {
				t.Errorf("parsed focus ExcludeLenses = %v; want empty", parsed.ExcludeLenses)
			}
			if len(parsed.Focus) != 0 {
				t.Errorf("parsed focus Focus = %v; want empty", parsed.Focus)
			}
		})
	}
}

// TestBouncer_SeedSideHarvest_SurvivesALateFailure pins that a focus file the seed agent wrote
// survives, byte-identical, a run that then errors or reports a non-Done outcome.
//
//testtiming:keep pins that a focus file the seed agent wrote survives a late run error or a non-Done outcome byte-identical
func TestBouncer_SeedSideHarvest_SurvivesALateFailure(t *testing.T) {
	tests := []struct {
		name   string
		result shuttleengine.Result
		err    error
	}{
		{"a late run error", shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, errors.New("run failed after write")},
		{"a non-Done outcome", shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			written := "---\nround: 1\nexclude_lenses: []\nfocus: [\"real targeting\"]\n---\nrationale\n"
			shuttle := &shedfake.Shuttle{Result: tt.result, Err: tt.err}
			shuttle.DuringRun = func() {
				path := shuttle.GotSpec.OutputFiles[0]
				if err := os.WriteFile(path, []byte(written), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
				}
			}
			b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()

			shedfake.RequireOutcome(t, b, shedengine.Stuck)

			got, err := os.ReadFile(focusPath(cfg.RunDir, 1))
			if err != nil {
				t.Fatalf("ReadFile(round-1-focus.md) = %v; want nil", err)
			}
			if string(got) != written {
				t.Errorf("round-1-focus.md = %q; want it byte-identical to what the agent wrote (%q)", got, written)
			}
		})
	}
}

// TestBouncer_ReBounceProbesForALiveSeed is the regression guard for the one spawning mode that had
// no live-agent probe.
//
// doc.go's "Every spawning adapter probes for a live agent first" enumerates the Bouncer's probe
// sites as the seed pass, the judge pass, and the entry-time judge probe -- and the entry-time probe
// is guarded by `n > 0 && judged(n)`, so it never covers the re-bounce. A parsing round-1 focus file
// proves the seed agent wrote its one declared output, never that it exited, and shuttle's Wait
// polls for bare existence at that path. So a driver killed in the window between the write and the
// agent's own exit lands on this branch with a live seed still holding round-1-focus.md, and
// returning Stuck without waiting abandons it while the segment's round producer starts reading that
// very file.
//
// Reproduced live in crucible round 1: `lyx loom step` SIGKILLed mid-Discussion-Bouncer seed with
// exactly one agent alive, then re-invoked. It correctly did not double-spawn -- and left the
// paid-for agent running behind it. That reproduction contradicted what the pre-rewrite driver
// instructions told operators; the rewritten driver stencil makes no such claim.
func TestBouncer_ReBounceProbesForALiveSeed(t *testing.T) {
	seeded := "---\nround: 1\nexclude_lenses: []\nfocus: [\"already seeded\"]\n---\n"

	tests := []struct {
		name        string
		attachFound bool
	}{
		{"LiveSeedIsWaitedOn", true},
		{"NothingLiveLeavesTheBranchUnchanged", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shuttle := &shedfake.Shuttle{
				Result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
				AttachFound:  tt.attachFound,
				AttachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
			}
			b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()

			if err := os.WriteFile(focusPath(cfg.RunDir, 1), []byte(seeded), 0o644); err != nil {
				t.Fatalf("WriteFile(...) = %v; want nil", err)
			}

			ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)

			if !shuttle.AttachCalled {
				t.Fatal("Call() returned from the re-bounce without probing for a live seed; a live seed agent would be abandoned in its pane")
			}
			wantOutputs := []string{focusPath(cfg.RunDir, 1)}
			if !slices.Equal(shuttle.GotAttachSpec.OutputFiles, wantOutputs) {
				t.Errorf("re-bounce probe OutputFiles = %v; want %v -- Attach matches on this set alone, so it must equal the seed spawn's own", shuttle.GotAttachSpec.OutputFiles, wantOutputs)
			}
			if shuttle.GotAttachSpec.Role != bouncerSeedRole {
				t.Errorf("re-bounce probe Role = %q; want %q -- it must describe the same run the seed spawn started", shuttle.GotAttachSpec.Role, bouncerSeedRole)
			}
			if shuttle.GotAttachSpec.Round != "1" {
				t.Errorf("re-bounce probe Round = %q; want \"1\"", shuttle.GotAttachSpec.Round)
			}

			// The branch's own behaviour is unchanged either way: probing is not respawning.
			if shuttle.Called {
				t.Error("Call() spawned through the shuttle seam on a re-bounce; want the probe only, never a spawn")
			}
			if ptr.Path != "" || ptr.GateAttempts != nil {
				t.Errorf("Call() pointer = %+v; want empty Path and no GateAttempts", ptr)
			}
			if want := "bouncer segment already seeded; round producer returned no report"; ptr.Reason != want {
				t.Errorf("Call() Reason = %q; want %q", ptr.Reason, want)
			}
			got, err := os.ReadFile(focusPath(cfg.RunDir, 1))
			if err != nil {
				t.Fatalf("ReadFile(round-1-focus.md) = %v; want nil", err)
			}
			if string(got) != seeded {
				t.Errorf("round-1-focus.md = %q; want it left byte-identical (%q)", got, seeded)
			}
		})
	}
}

// TestBouncer_ReBounceDegradesOnAnUndeterminableProbe pins that a probe which could not answer the
// liveness question is never read as "nothing is running": the re-bounce degrades rather than
// silently proceeding, the same rule awaitLiveJudge's caller already follows.
func TestBouncer_ReBounceDegradesOnAnUndeterminableProbe(t *testing.T) {
	shuttle := &shedfake.Shuttle{
		Result:    shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		AttachErr: errors.New("reed state unreadable"),
	}
	b, cfg := newBouncerFixture(t, withShuttle(shuttle)).Build()

	seeded := "---\nround: 1\nexclude_lenses: []\nfocus: [\"already seeded\"]\n---\n"
	if err := os.WriteFile(focusPath(cfg.RunDir, 1), []byte(seeded), 0o644); err != nil {
		t.Fatalf("WriteFile(...) = %v; want nil", err)
	}

	ptr := shedfake.RequireOutcome(t, b, shedengine.Stuck)
	if ptr.Path != "" {
		t.Errorf("Call() pointer.Path = %q; want empty", ptr.Path)
	}
	if want := "shedadapters: bouncer re-bounce seed attach probe failed"; ptr.Reason != want {
		t.Errorf("Call() Reason = %q; want the degrade message %q", ptr.Reason, want)
	}
	if shuttle.Called {
		t.Error("Call() spawned through the shuttle seam after a failed probe; want no spawn")
	}
}

// TestBouncer_MarkerCompleteness_BothTemplates also pins that no stamp banner of the rubric leaks into either filled prompt.
func TestBouncer_MarkerCompleteness_BothTemplates(t *testing.T) {
	rubric := "<!-- lyx-stencil: sha256=deadbeef -->\n# Rubric\n\nBe thorough.\n"
	strippedRubric := stencil.StripLeadingComment(rubric)

	t.Run("Seed", func(t *testing.T) {
		values := map[string]string{
			"rubric":     strippedRubric,
			"artifacts":  "/abs/artifact.md",
			"round":      "1",
			"focus_path": "/abs/round-1-focus.md",
		}
		maps.Copy(values, focusSchemaMarkers(true))
		values[parentdirective.MarkerName] = "PARENT DIRECTIVE"
		prompt, err := stencil.Fill(stencils.BouncerTemplateSeed, values)
		if err != nil {
			t.Fatalf("stencil.Fill(seed template, ...) error = %v; want nil", err)
		}
		markers, err := stencil.TopLevelMarkers(stencils.BouncerTemplateSeed)
		if err != nil {
			t.Fatalf("stencil.TopLevelMarkers(seed template) error = %v; want nil", err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(prompt), values[marker]) {
				t.Errorf("filled seed prompt does not contain value for marker %q (%q)", marker, values[marker])
			}
		}
		if strings.Contains(string(prompt), "<!-- lyx-stencil:") {
			t.Error("filled seed prompt contains a leaked stamp banner")
		}
	})

	t.Run("Judge_FirstRound", func(t *testing.T) {
		values := map[string]string{
			"rubric":          strippedRubric,
			"facts_path":      "/abs/round-1-facts.md",
			"round":           "1",
			"next_round":      "2",
			"decision_rule":   decisionRuleMarker(1, 3),
			"report_path":     "/abs/round-1-report.md",
			"previous_ledger": "(none)",
			"verdict_path":    "/abs/round-1-bouncer-verdict.md",
			"ledger_path":     "/abs/round-1-bouncer-ledger.md",
			"focus_path":      "/abs/round-2-focus.md",
		}
		maps.Copy(values, focusSchemaMarkers(true))
		values[parentdirective.MarkerName] = "PARENT DIRECTIVE"
		if values["previous_ledger"] != "(none)" {
			t.Fatalf("test setup error: previous_ledger must be the literal (none) for round 1")
		}
		prompt, err := stencil.FillOptional(stencils.BouncerTemplateJudge, values, []string{"pattern_directive"})
		if err != nil {
			t.Fatalf("stencil.Fill(judge template, ...) error = %v; want nil", err)
		}
		markers, err := stencil.TopLevelMarkers(stencils.BouncerTemplateJudge)
		if err != nil {
			t.Fatalf("stencil.TopLevelMarkers(judge template) error = %v; want nil", err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(prompt), values[marker]) {
				t.Errorf("filled judge prompt does not contain value for marker %q (%q)", marker, values[marker])
			}
		}
		if !strings.Contains(string(prompt), "(none)") {
			t.Error("filled first-round judge prompt does not render the previous_ledger literal \"(none)\"")
		}
		if strings.Contains(string(prompt), "<!-- lyx-stencil:") {
			t.Error("filled judge prompt contains a leaked stamp banner")
		}
	})
}
