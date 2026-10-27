//go:build tier0

// tidy_test.go pins dobby's board-tidy step over a fake shuttle, a fake notice and a temp state record: the landing count, the spawn every few landings, the report and the timeout, and the agent's prompt and launch spec.

package dobbyengine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// tidyShuttleFake records the agents started and the strands stopped.
type tidyShuttleFake struct {
	started []shuttleengine.Spec
	stopped []string
}

func (f *tidyShuttleFake) Start(spec shuttleengine.Spec) (string, error) {
	f.started = append(f.started, spec)
	return "strand-" + string(rune('0'+len(f.started))), nil
}

func (f *tidyShuttleFake) StopStrand(guid string) error {
	f.stopped = append(f.stopped, guid)
	return nil
}

// tidyHarness drives a daemon's landing count and tidy step over fakes.
type tidyHarness struct {
	t       *testing.T
	d       *daemon
	shuttle *tidyShuttleFake
	notices []string
	landed  []string
	every   int
	now     time.Time
}

func newTidyHarness(t *testing.T, every int) *tidyHarness {
	t.Helper()
	stencils := filepath.Join(t.TempDir(), "stencils")
	stencilkit.SeedInto(t, stencils)
	root := t.TempDir()
	h := &tidyHarness{t: t, shuttle: &tidyShuttleFake{}, every: every, now: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)}
	h.d = h.daemonOver(filepath.Join(root, StateFileName), filepath.Join(root, TidyDirName), stencils)
	return h
}

// daemonOver builds a daemon over a state path, as a restarted dobby would build one over the same record.
func (h *tidyHarness) daemonOver(statePath, tidyDir, stencils string) *daemon {
	return &daemon{
		geom: Geometry{WorktreeName: "prime", StatePath: statePath, TidyDir: tidyDir},
		deps: Deps{
			LoadConfig:  func() (Config, error) { return Config{BoardTidyEvery: h.every, BoardTidyTimeoutMin: 30}, nil },
			Landings:    func() ([]string, error) { return slices.Clone(h.landed), nil },
			TidyShuttle: h.shuttle,
			TidyModel:   shuttleengine.ModelChoice{Engine: "b", Model: "sonnet"},
			StencilsDir: stencils,
			NearestOrch: func() (string, error) { return "ly:orch", nil },
			Notice: func(_ context.Context, line string) error {
				h.notices = append(h.notices, line)
				return nil
			},
		},
	}
}

// restart replaces the daemon with a fresh one over the same state record.
func (h *tidyHarness) restart() {
	h.d = h.daemonOver(h.d.geom.StatePath, h.d.geom.TidyDir, h.d.deps.StencilsDir)
}

// step runs the landing count and the tidy step at the harness's clock.
func (h *tidyHarness) step() {
	h.t.Helper()
	if err := h.d.countLandings(h.now); err != nil {
		h.t.Fatalf("countLandings() = %v; want nil", err)
	}
	if err := h.d.tidyStep(context.Background(), h.now); err != nil {
		h.t.Fatalf("tidyStep() = %v; want nil", err)
	}
}

func (h *tidyHarness) state() State {
	h.t.Helper()
	current, _, err := ReadState(h.d.geom.StatePath)
	if err != nil {
		h.t.Fatalf("ReadState() = %v; want nil", err)
	}
	return current
}

// writeReport writes the in-flight agent's report file.
func (h *tidyHarness) writeReport() {
	h.t.Helper()
	path := h.state().Tidy.ReportPath
	if err := os.WriteFile(path, []byte("changed nothing\n"), 0o644); err != nil {
		h.t.Fatalf("WriteFile(%s) = %v; want nil", path, err)
	}
}

func TestCountLandingsAndTidy(t *testing.T) {
	t.Parallel()
	five := []string{"a", "b", "c", "d", "e"}

	t.Run("five landings across runs with board_tidy_every 5 spawn one agent naming the five slugs", func(t *testing.T) {
		t.Parallel()
		h := newTidyHarness(t, 5)
		h.step()
		h.landed = five
		h.step()
		if len(h.shuttle.started) != 1 {
			t.Fatalf("agents started = %d; want 1", len(h.shuttle.started))
		}
		for _, slug := range five {
			if !strings.Contains(h.shuttle.started[0].Prompt, "`"+slug+"`") {
				t.Errorf("prompt does not name the landed slug %q", slug)
			}
		}
		if tidy := h.state().Tidy; tidy == nil || tidy.StrandGUID == "" || !tidy.Deadline.Equal(h.now.Add(30*time.Minute)) {
			t.Fatalf("state Tidy = %+v; want the strand, its report and a deadline 30 minutes out", tidy)
		}
		wantReport := filepath.Join(h.d.geom.TidyDir, h.now.UTC().Format("20060102T150405Z")+".md")
		if got := h.state().Tidy.ReportPath; got != wantReport {
			t.Errorf("Tidy.ReportPath = %q; want %q", got, wantReport)
		}
	})

	t.Run("a restart with the same records spawns none and counts none twice", func(t *testing.T) {
		t.Parallel()
		h := newTidyHarness(t, 5)
		h.step()
		h.landed = five
		h.step()
		h.restart()
		h.step()
		if len(h.shuttle.started) != 1 {
			t.Errorf("agents started = %d; want still 1", len(h.shuttle.started))
		}
		if got := h.state().LandedSinceTidy; !slices.Equal(got, five) {
			t.Errorf("LandedSinceTidy = %v; want %v", got, five)
		}
	})

	t.Run("a first start over old records counts none", func(t *testing.T) {
		t.Parallel()
		h := newTidyHarness(t, 5)
		h.landed = five
		h.step()
		if len(h.shuttle.started) != 0 || len(h.state().LandedSinceTidy) != 0 {
			t.Errorf("agents started, landed since tidy = %d, %v; want none counted", len(h.shuttle.started), h.state().LandedSinceTidy)
		}
		h.landed = append(slices.Clone(five), "f")
		h.step()
		if got := h.state().LandedSinceTidy; !slices.Equal(got, []string{"f"}) {
			t.Errorf("LandedSinceTidy = %v; want only the landing made after the start", got)
		}
	})

	t.Run("board_tidy_every 0 spawns none", func(t *testing.T) {
		t.Parallel()
		h := newTidyHarness(t, 0)
		h.step()
		h.landed = five
		h.step()
		if len(h.shuttle.started) != 0 {
			t.Errorf("agents started = %d; want none", len(h.shuttle.started))
		}
	})

	t.Run("a report written posts its path once and stops the agent", func(t *testing.T) {
		t.Parallel()
		h := newTidyHarness(t, 5)
		h.step()
		h.landed = five
		h.step()
		h.writeReport()
		reportPath := h.state().Tidy.ReportPath
		h.step()
		h.step()
		want := "board tidy after 5 landings: report " + reportPath + "; put each removal or scope change to the operator"
		if !slices.Equal(h.notices, []string{want}) {
			t.Errorf("notices = %q; want the one %q", h.notices, want)
		}
		if !slices.Equal(h.shuttle.stopped, []string{"strand-1"}) {
			t.Errorf("strands stopped = %v; want [strand-1]", h.shuttle.stopped)
		}
		if after := h.state(); after.Tidy != nil || len(after.LandedSinceTidy) != 0 {
			t.Errorf("state after the report = Tidy %+v, landed %v; want both cleared", after.Tidy, after.LandedSinceTidy)
		}
	})

	t.Run("a deadline passed stops the agent and posts the no-report line, and the next tidy names the earlier slugs with its own", func(t *testing.T) {
		t.Parallel()
		h := newTidyHarness(t, 6)
		h.step()
		h.landed = append(slices.Clone(five), "f")
		h.step()
		h.now = h.now.Add(31 * time.Minute)
		h.step()
		want := "board tidy after 6 landings gave no report; its slugs carry to the next tidy: a, b, c, d, e, f"
		if !slices.Equal(h.notices, []string{want}) {
			t.Fatalf("notices = %q; want the one %q", h.notices, want)
		}
		if !slices.Equal(h.shuttle.stopped, []string{"strand-1"}) {
			t.Errorf("strands stopped = %v; want [strand-1]", h.shuttle.stopped)
		}

		h.every = 7
		h.landed = append(slices.Clone(h.landed), "g")
		h.step()
		if len(h.shuttle.started) != 2 {
			t.Fatalf("agents started = %d; want the second tidy", len(h.shuttle.started))
		}
		for _, slug := range []string{"a", "f", "g"} {
			if !strings.Contains(h.shuttle.started[1].Prompt, "`"+slug+"`") {
				t.Errorf("second prompt does not name %q", slug)
			}
		}
	})
}

func TestTidyPrompt(t *testing.T) {
	t.Parallel()
	stencils := filepath.Join(t.TempDir(), "stencils")
	stencilkit.SeedInto(t, stencils)

	prompt, err := TidyPrompt(stencils, TidyPromptInput{Landed: []string{"alpha", "beta"}, ReportPath: "/hub/prime/.lym/beam/tidy/20261026T151739Z.md", Parent: "ly:orch"})
	if err != nil {
		t.Fatalf("TidyPrompt() = _, %v; want nil", err)
	}
	for _, want := range []string{"`alpha`", "`beta`", "/hub/prime/.lym/beam/tidy/20261026T151739Z.md", "ly:orch"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("TidyPrompt() lacks %q", want)
		}
	}
	if strings.Contains(prompt, "{{") {
		t.Errorf("TidyPrompt() holds an unfilled marker")
	}

	spec := TidySpec(prompt, "/report.md", shuttleengine.ModelChoice{Engine: "b", Model: "sonnet"}, 30*time.Minute)
	if spec.Segment != segmentcolor.Coordinator {
		t.Errorf("TidySpec().Segment = %v; want %v", spec.Segment, segmentcolor.Coordinator)
	}
	if !slices.Equal(spec.Skills, []string{"ly:board"}) {
		t.Errorf("TidySpec().Skills = %v; want [ly:board]", spec.Skills)
	}
	if spec.Role != BoardTidyRole || !slices.Equal(spec.OutputFiles, []string{"/report.md"}) || spec.Timeout != 30*time.Minute {
		t.Errorf("TidySpec() = role %q, outputs %v, timeout %v; want the role, the report and the timeout", spec.Role, spec.OutputFiles, spec.Timeout)
	}
}
