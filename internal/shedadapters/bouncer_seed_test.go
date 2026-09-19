// bouncer_seed_test.go covers Bouncer.Call's seed call, seed-side harvest, and the re-bounce mode,
// plus the prompt-shape guarantees shared by both templates: marker completeness against the
// shipped stencils package bytes and the stamp-leak regression.
// The judge, replay, harvest-on-judge, degradation, pointer-discipline, stale-output, and
// cancellation scenarios are deliberately left to batch 4, which adds only test files.

package shedadapters

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencil"
)

// shippedBouncerStencilsFixture builds a stencils fixture directory seeded with the shipped
// bouncer template bytes taken from the contracts/stencils package's own exported vars, plus a
// rubric stencil carrying a realistic leading stamp banner.
func shippedBouncerStencilsFixture(t *testing.T, rubricName, rubricBody string) string {
	t.Helper()
	return newBouncerStencilsFixture(t, map[string]string{
		"bouncer-template-seed":  string(stencils.BouncerTemplateSeed),
		"bouncer-template-judge": string(stencils.BouncerTemplateJudge),
		rubricName:               rubricBody,
	})
}

// testBouncerConfig builds a BouncerConfig over a fresh run dir and the shipped bouncer stencils,
// filling every field except Shuttle exactly as newTestBouncer's own literal always has. It exists
// so a test needing a non-default config field -- the commit seam is the first -- can build one
// without duplicating the fixture.
func testBouncerConfig(t *testing.T) BouncerConfig {
	t.Helper()

	runDir := t.TempDir()
	stencilsDir := shippedBouncerStencilsFixture(t, "bouncer-template-rubric", "# Rubric\n\nBe thorough and cite evidence.\n")
	return BouncerConfig{
		Name:          "gate",
		RunDir:        runDir,
		ArtifactPaths: []string{filepath.Join(runDir, "artifact.md")},
		ReportName:    func(round int) string { return fmt.Sprintf("round-%d-report.md", round) },
		StencilsDir:   stencilsDir,
		RubricStencil: "bouncer-template-rubric",
		Model:         "claude-x",
		Effort:        "high",
		Version:       "v1",
		Now:           fixedClock(time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)),
	}
}

// newTestBouncer builds a *Bouncer over a fresh run dir and the shipped bouncer stencils, ready
// for a seed-mode Call: an empty run dir with no report and no round-1 focus file.
func newTestBouncer(t *testing.T, shuttle Shuttle) (*Bouncer, BouncerConfig) {
	t.Helper()

	cfg := testBouncerConfig(t)
	cfg.Shuttle = shuttle
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}
	return b, cfg
}

// testBouncerConfigWithSpecsMarker builds a BouncerConfig like testBouncerConfig, but seeds the
// rubric with a literal {{.specs_dir}} marker and sets cfg.SpecsDir to specsDir -- the fixture the
// composed-prompt specs_dir assertions need, since testBouncerConfig's own default rubric carries
// no marker at all.
func testBouncerConfigWithSpecsMarker(t *testing.T, specsDir string) BouncerConfig {
	t.Helper()

	cfg := testBouncerConfig(t)
	cfg.StencilsDir = shippedBouncerStencilsFixture(t, "bouncer-template-rubric", "# Rubric\n\nCite {{.specs_dir}}.\n\nBe thorough and cite evidence.\n")
	cfg.SpecsDir = specsDir
	return cfg
}

// TestBouncer_SeedCall_ComposedPromptStatesSpecsDir asserts the seed call's composed prompt --
// where the rubric is interpolated as a marker VALUE, never run through the fill itself -- contains
// the told specs directory and carries no literal "{{.specs_dir}}" marker. A rubric-bytes-only
// assertion could not catch this: the marker lives inside the rubric value, invisible to a check
// that never renders it into the surrounding template.
func TestBouncer_SeedCall_ComposedPromptStatesSpecsDir(t *testing.T) {
	specsDir := t.TempDir()
	if !filepath.IsAbs(specsDir) {
		t.Fatalf("t.TempDir() = %q; want an absolute path", specsDir)
	}

	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	shuttle.duringRun = func() {
		path := shuttle.gotSpec.OutputFiles[0]
		content := "---\nround: 1\nexclude_lenses: []\nfocus: []\n---\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
	}

	cfg := testBouncerConfigWithSpecsMarker(t, specsDir)
	cfg.Shuttle = shuttle
	b, err := NewBouncer(cfg)
	if err != nil {
		t.Fatalf("NewBouncer(...) error = %v; want nil", err)
	}

	if _, _, err := b.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}

	prompt := shuttle.gotSpec.Prompt
	if !strings.Contains(prompt, specsDir) {
		t.Errorf("seed call composed prompt does not contain the told specs directory %q", specsDir)
	}
	if strings.Contains(prompt, "{{.specs_dir}}") {
		t.Error("seed call composed prompt contains a literal \"{{.specs_dir}}\" marker; want it rendered")
	}
}

func TestBouncer_SeedCall_HappyPath(t *testing.T) {
	shuttle := &fakeShuttle{
		result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	shuttle.duringRun = func() {
		path := shuttle.gotSpec.OutputFiles[0]
		content := "---\nround: 1\nexclude_lenses: []\nfocus: [\"check the thing\"]\n---\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
	}
	b, cfg := newTestBouncer(t, shuttle)

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if !shuttle.called {
		t.Error("Call() did not invoke the shuttle seam")
	}
	if shuttle.gotSpec.Role != "bouncer-seed" {
		t.Errorf("recorded spec.Role = %q; want %q", shuttle.gotSpec.Role, "bouncer-seed")
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

func TestBouncer_SeedDiscriminator_ParsesRatherThanStats(t *testing.T) {
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newTestBouncer(t, shuttle)

	// round-1-focus.md present but unparseable, with no report on disk.
	if err := os.WriteFile(focusPath(cfg.RunDir, 1), []byte("not frontmatter at all"), 0o644); err != nil {
		t.Fatalf("WriteFile(...) = %v; want nil", err)
	}

	outcome, _, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if !shuttle.called {
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
				runDir := t.TempDir()
				stencilsDir := newBouncerStencilsFixture(t, map[string]string{
					// bouncer-template-seed deliberately absent.
					"bouncer-template-judge":  string(stencils.BouncerTemplateJudge),
					"bouncer-template-rubric": "# Rubric\n\nBe thorough.\n",
				})
				cfg := BouncerConfig{
					Name:          "gate",
					RunDir:        runDir,
					ArtifactPaths: []string{filepath.Join(runDir, "artifact.md")},
					ReportName:    func(round int) string { return fmt.Sprintf("round-%d-report.md", round) },
					StencilsDir:   stencilsDir,
					RubricStencil: "bouncer-template-rubric",
					Shuttle:       &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}},
					Now:           fixedClock(time.Now()),
				}
				b, err := NewBouncer(cfg)
				if err != nil {
					t.Fatalf("NewBouncer(...) error = %v; want nil", err)
				}
				return b, cfg
			},
		},
		{
			name: "RubricUnreadable",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				runDir := t.TempDir()
				stencilsDir := newBouncerStencilsFixture(t, map[string]string{
					"bouncer-template-seed":  string(stencils.BouncerTemplateSeed),
					"bouncer-template-judge": string(stencils.BouncerTemplateJudge),
					// The registered rubric name below is never seeded, so the probe would fail
					// at construction; give the constructor a readable placeholder rubric and
					// then delete it before Call so the seed spawn (not construction) fails.
					"bouncer-template-rubric": "# Rubric\n\nBe thorough.\n",
				})
				cfg := BouncerConfig{
					Name:          "gate",
					RunDir:        runDir,
					ArtifactPaths: []string{filepath.Join(runDir, "artifact.md")},
					ReportName:    func(round int) string { return fmt.Sprintf("round-%d-report.md", round) },
					StencilsDir:   stencilsDir,
					RubricStencil: "bouncer-template-rubric",
					Shuttle:       &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}},
					Now:           fixedClock(time.Now()),
				}
				b, err := NewBouncer(cfg)
				if err != nil {
					t.Fatalf("NewBouncer(...) error = %v; want nil", err)
				}
				if err := os.Remove(filepath.Join(stencilsDir, "bouncer", "bouncer-template-rubric.md")); err != nil {
					t.Fatalf("Remove(rubric) = %v; want nil", err)
				}
				return b, cfg
			},
		},
		{
			name: "FillFailure",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				runDir := t.TempDir()
				stencilsDir := newBouncerStencilsFixture(t, map[string]string{
					// Declares a marker the Go side does not supply.
					"bouncer-template-seed":   "# Seed\n\n{{.rubric}} {{.artifacts}} {{.round}} {{.focus_path}} {{.unknown_marker}}\n",
					"bouncer-template-judge":  string(stencils.BouncerTemplateJudge),
					"bouncer-template-rubric": "# Rubric\n\nBe thorough.\n",
				})
				cfg := BouncerConfig{
					Name:          "gate",
					RunDir:        runDir,
					ArtifactPaths: []string{filepath.Join(runDir, "artifact.md")},
					ReportName:    func(round int) string { return fmt.Sprintf("round-%d-report.md", round) },
					StencilsDir:   stencilsDir,
					RubricStencil: "bouncer-template-rubric",
					Shuttle:       &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}},
					Now:           fixedClock(time.Now()),
				}
				b, err := NewBouncer(cfg)
				if err != nil {
					t.Fatalf("NewBouncer(...) error = %v; want nil", err)
				}
				return b, cfg
			},
		},
		{
			name: "RunError",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				return newTestBouncer(t, &fakeShuttle{err: errors.New("run exploded")})
			},
		},
		{
			name: "NonOutcomeDone",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				return newTestBouncer(t, &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking}})
			},
		},
		{
			name: "AgentWroteNothing",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				return newTestBouncer(t, &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}})
			},
		},
		{
			name: "AgentWroteUnparseableFocus",
			buildBouncer: func(t *testing.T) (*Bouncer, BouncerConfig) {
				shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
				shuttle.duringRun = func() {
					path := shuttle.gotSpec.OutputFiles[0]
					if err := os.WriteFile(path, []byte("garbage, not frontmatter"), 0o644); err != nil {
						t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
					}
				}
				return newTestBouncer(t, shuttle)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, cfg := tt.buildBouncer(t)

			outcome, ptr, err := b.Call(context.Background())
			if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}
			if outcome != shedengine.Stuck {
				t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
			}
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

func TestBouncer_SeedSideHarvest_SurvivesLateRunError(t *testing.T) {
	written := "---\nround: 1\nexclude_lenses: []\nfocus: [\"real targeting\"]\n---\nrationale\n"
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}, err: errors.New("run failed after write")}
	shuttle.duringRun = func() {
		path := shuttle.gotSpec.OutputFiles[0]
		if err := os.WriteFile(path, []byte(written), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
	}
	b, cfg := newTestBouncer(t, shuttle)

	outcome, _, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}

	got, err := os.ReadFile(focusPath(cfg.RunDir, 1))
	if err != nil {
		t.Fatalf("ReadFile(round-1-focus.md) = %v; want nil", err)
	}
	if string(got) != written {
		t.Errorf("round-1-focus.md = %q; want it byte-identical to what the agent wrote (%q)", got, written)
	}
}

func TestBouncer_SeedSideHarvest_SurvivesNonOutcomeDone(t *testing.T) {
	written := "---\nround: 1\nexclude_lenses: []\nfocus: [\"real targeting\"]\n---\nrationale\n"
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking}}
	shuttle.duringRun = func() {
		path := shuttle.gotSpec.OutputFiles[0]
		if err := os.WriteFile(path, []byte(written), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
	}
	b, cfg := newTestBouncer(t, shuttle)

	outcome, _, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}

	got, err := os.ReadFile(focusPath(cfg.RunDir, 1))
	if err != nil {
		t.Fatalf("ReadFile(round-1-focus.md) = %v; want nil", err)
	}
	if string(got) != written {
		t.Errorf("round-1-focus.md = %q; want it byte-identical to what the agent wrote (%q)", got, written)
	}
}

func TestBouncer_ReBounce(t *testing.T) {
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	b, cfg := newTestBouncer(t, shuttle)

	seeded := "---\nround: 1\nexclude_lenses: []\nfocus: [\"already seeded\"]\n---\n"
	if err := os.WriteFile(focusPath(cfg.RunDir, 1), []byte(seeded), 0o644); err != nil {
		t.Fatalf("WriteFile(...) = %v; want nil", err)
	}

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if shuttle.called {
		t.Error("Call() invoked the shuttle seam on a re-bounce; want it never called")
	}

	got, err := os.ReadFile(focusPath(cfg.RunDir, 1))
	if err != nil {
		t.Fatalf("ReadFile(round-1-focus.md) = %v; want nil", err)
	}
	if string(got) != seeded {
		t.Errorf("round-1-focus.md = %q; want it left byte-identical (%q)", got, seeded)
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
// paid-for agent running behind it, which the ly-drive skill tells operators cannot happen
// ("there is no orphan: the next step attaches to the agent rather than abandoning it").
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
			shuttle := &fakeShuttle{
				result:       shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
				attachFound:  tt.attachFound,
				attachResult: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
			}
			b, cfg := newTestBouncer(t, shuttle)

			if err := os.WriteFile(focusPath(cfg.RunDir, 1), []byte(seeded), 0o644); err != nil {
				t.Fatalf("WriteFile(...) = %v; want nil", err)
			}

			outcome, ptr, err := b.Call(context.Background())
			if err != nil {
				t.Fatalf("Call() error = %v; want nil", err)
			}

			if !shuttle.attachCalled {
				t.Fatal("Call() returned from the re-bounce without probing for a live seed; a live seed agent would be abandoned in its pane")
			}
			wantOutputs := []string{focusPath(cfg.RunDir, 1)}
			if !slices.Equal(shuttle.gotAttachSpec.OutputFiles, wantOutputs) {
				t.Errorf("re-bounce probe OutputFiles = %v; want %v -- Attach matches on this set alone, so it must equal the seed spawn's own", shuttle.gotAttachSpec.OutputFiles, wantOutputs)
			}
			if shuttle.gotAttachSpec.Role != bouncerSeedRole {
				t.Errorf("re-bounce probe Role = %q; want %q -- it must describe the same run the seed spawn started", shuttle.gotAttachSpec.Role, bouncerSeedRole)
			}
			if shuttle.gotAttachSpec.Round != "1" {
				t.Errorf("re-bounce probe Round = %q; want \"1\"", shuttle.gotAttachSpec.Round)
			}

			// The branch's own behaviour is unchanged either way: probing is not respawning.
			if shuttle.called {
				t.Error("Call() spawned through the shuttle seam on a re-bounce; want the probe only, never a spawn")
			}
			if outcome != shedengine.Stuck {
				t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
			}
			if ptr != (shedengine.OutputPointer{}) {
				t.Errorf("Call() pointer = %+v; want empty", ptr)
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
	shuttle := &fakeShuttle{
		result:    shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
		attachErr: errors.New("reed state unreadable"),
	}
	b, cfg := newTestBouncer(t, shuttle)

	seeded := "---\nround: 1\nexclude_lenses: []\nfocus: [\"already seeded\"]\n---\n"
	if err := os.WriteFile(focusPath(cfg.RunDir, 1), []byte(seeded), 0o644); err != nil {
		t.Fatalf("WriteFile(...) = %v; want nil", err)
	}

	outcome, ptr, err := b.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil -- a degraded probe is a warned Stuck, never an engine error", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %q; want %q", outcome, shedengine.Stuck)
	}
	if ptr != (shedengine.OutputPointer{}) {
		t.Errorf("Call() pointer = %+v; want empty", ptr)
	}
	if shuttle.called {
		t.Error("Call() spawned through the shuttle seam after a failed probe; want no spawn")
	}
}

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
	})

	t.Run("Judge_FirstRound", func(t *testing.T) {
		values := map[string]string{
			"rubric":          strippedRubric,
			"artifacts":       "/abs/artifact.md",
			"round":           "1",
			"next_round":      "2",
			"report_path":     "/abs/round-1-report.md",
			"previous_ledger": "(none)",
			"verdict_path":    "/abs/round-1-bouncer-verdict.md",
			"ledger_path":     "/abs/round-1-bouncer-ledger.md",
			"focus_path":      "/abs/round-2-focus.md",
		}
		if values["previous_ledger"] != "(none)" {
			t.Fatalf("test setup error: previous_ledger must be the literal (none) for round 1")
		}
		prompt, err := stencil.Fill(stencils.BouncerTemplateJudge, values)
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
	})
}

func TestBouncer_StampLeakRegression_BothTemplates(t *testing.T) {
	rubric := "<!-- lyx-stencil: sha256=deadbeef000000000000000000000000000000000000000000000000000000 -->\n# Rubric\n\nBe thorough.\n"
	strippedRubric := stencil.StripLeadingComment(rubric)
	if strings.Contains(strippedRubric, "<!-- lyx-stencil:") {
		t.Fatalf("test setup error: StripLeadingComment did not strip the fixture's own banner")
	}

	t.Run("Seed", func(t *testing.T) {
		values := map[string]string{
			"rubric":     strippedRubric,
			"artifacts":  "/abs/artifact.md",
			"round":      "1",
			"focus_path": "/abs/round-1-focus.md",
		}
		prompt, err := stencil.Fill(stencils.BouncerTemplateSeed, values)
		if err != nil {
			t.Fatalf("stencil.Fill(seed template, ...) error = %v; want nil", err)
		}
		if strings.Contains(string(prompt), "<!-- lyx-stencil:") {
			t.Error("filled seed prompt contains a leaked stamp banner")
		}
	})

	t.Run("Judge", func(t *testing.T) {
		values := map[string]string{
			"rubric":          strippedRubric,
			"artifacts":       "/abs/artifact.md",
			"round":           "1",
			"next_round":      "2",
			"report_path":     "/abs/round-1-report.md",
			"previous_ledger": "(none)",
			"verdict_path":    "/abs/round-1-bouncer-verdict.md",
			"ledger_path":     "/abs/round-1-bouncer-ledger.md",
			"focus_path":      "/abs/round-2-focus.md",
		}
		prompt, err := stencil.Fill(stencils.BouncerTemplateJudge, values)
		if err != nil {
			t.Fatalf("stencil.Fill(judge template, ...) error = %v; want nil", err)
		}
		if strings.Contains(string(prompt), "<!-- lyx-stencil:") {
			t.Error("filled judge prompt contains a leaked stamp banner")
		}
	})
}

func TestBouncer_SpecIdentity_RoleAndRound(t *testing.T) {
	shuttle := &fakeShuttle{
		result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone},
	}
	shuttle.duringRun = func() {
		path := shuttle.gotSpec.OutputFiles[0]
		content := "---\nround: 1\nexclude_lenses: []\nfocus: []\n---\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
	}
	b, _ := newTestBouncer(t, shuttle)

	if _, _, err := b.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if shuttle.gotSpec.Role != "bouncer-seed" {
		t.Errorf("seed call spec.Role = %q; want %q", shuttle.gotSpec.Role, "bouncer-seed")
	}
	if shuttle.gotSpec.Round != "1" {
		t.Errorf("seed call spec.Round = %q; want %q", shuttle.gotSpec.Round, "1")
	}
}

func TestBouncer_SpecPassthrough_ModelEffortVersionAndAbsoluteOutputs(t *testing.T) {
	shuttle := &fakeShuttle{result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	shuttle.duringRun = func() {
		path := shuttle.gotSpec.OutputFiles[0]
		content := "---\nround: 1\nexclude_lenses: []\nfocus: []\n---\n"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v; want nil", path, err)
		}
	}
	b, cfg := newTestBouncer(t, shuttle)

	if _, _, err := b.Call(context.Background()); err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if shuttle.gotSpec.Model != cfg.Model {
		t.Errorf("recorded spec.Model = %q; want %q", shuttle.gotSpec.Model, cfg.Model)
	}
	if shuttle.gotSpec.Effort != cfg.Effort {
		t.Errorf("recorded spec.Effort = %q; want %q", shuttle.gotSpec.Effort, cfg.Effort)
	}
	if shuttle.gotSpec.Version != cfg.Version {
		t.Errorf("recorded spec.Version = %q; want %q", shuttle.gotSpec.Version, cfg.Version)
	}
	if len(shuttle.gotSpec.OutputFiles) == 0 {
		t.Fatal("recorded spec.OutputFiles is empty")
	}
	for _, f := range shuttle.gotSpec.OutputFiles {
		if !filepath.IsAbs(f) {
			t.Errorf("recorded spec.OutputFiles entry %q is not absolute", f)
		}
	}
}
