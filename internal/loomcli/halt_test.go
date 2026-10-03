// halt_test.go covers loom's halt friction: the halt note, loomAfterStep under step and loomPostRun under run.
// Every test is untagged Tier 1: the reflection agent is a shedfake.Shuttle, which also stands in for its `lyx selfreport create` filing, so nothing spawns an agent or files an issue.

package loomcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/frictionengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedverbs"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// haltFixture is a receiver with Tier 2 on, seeded stencils, the selfreport knob off and a shuttle answering done.
type haltFixture struct {
	c           *loomCLI
	shuttle     *shedfake.Shuttle
	frictionDir string
}

func newHaltFixture(t *testing.T) *haltFixture {
	t.Helper()

	root := t.TempDir()
	stencilsDir := filepath.Join(root, "stencils")
	stencilkit.SeedInto(t, stencilsDir)
	frictionDir := filepath.Join(root, "friction")
	if err := os.MkdirAll(frictionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", frictionDir, err)
	}

	loc := locationkit.Location(root, "warp", ".")
	archiveParent := filepath.Dir(loomengine.LoomFrictionArchivePrefix(loc))
	if err := os.MkdirAll(archiveParent, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) = %v; want nil", archiveParent, err)
	}

	shuttle := &shedfake.Shuttle{Result: shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}}
	c := &loomCLI{
		location:    loc,
		frictionDir: frictionDir,
		cfg:         loomengine.Config{Friction: "claude:sonnet[effort=high]", FrictionTimeoutMin: 1, Selfreport: false},
		runDeps:     websterengine.RunDeps{Geom: websterengine.Geometry{StencilsDir: stencilsDir}},
		shedPaths: shedbuild.ShedPaths{
			LockPath:       filepath.Join(root, "run.lock"),
			StatusPath:     filepath.Join(root, "status.json"),
			StatusLockPath: filepath.Join(root, "status.json.lock"),
		},
		reflectionShuttle: shuttle,
	}
	return &haltFixture{c: c, shuttle: shuttle, frictionDir: frictionDir}
}

// seedStatus seeds the status file and sets its producer, state and error.
func (f *haltFixture) seedStatus(t *testing.T, producer string, st shedengine.State, errText string) {
	t.Helper()

	p := f.c.shedPaths
	if err := loomshed.Seed(p.StatusPath, p.StatusLockPath, "warp", "main"); err != nil {
		t.Fatalf("Seed() = %v; want nil", err)
	}
	err := state.UpdateJSON[shedengine.Status](p.StatusPath, p.StatusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
		cur.CurrentProducer = producer
		cur.State = st
		cur.Error = errText
		return cur, nil
	})
	if err != nil {
		t.Fatalf("UpdateJSON() = %v; want nil", err)
	}
}

// notes returns the markdown files in the friction directory.
func (f *haltFixture) notes(t *testing.T) []string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(f.frictionDir, "*.md"))
	if err != nil {
		t.Fatalf("Glob() = %v; want nil", err)
	}
	return matches
}

func (f *haltFixture) requireNoNotesNoSpawn(t *testing.T) {
	t.Helper()

	if got := f.notes(t); len(got) != 0 {
		t.Errorf("friction directory holds %v; want no notes", got)
	}
	if len(f.shuttle.Specs) != 0 {
		t.Errorf("reflection shuttle ran %d times; want 0", len(f.shuttle.Specs))
	}
}

func readNote(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) = %v; want nil", path, err)
	}
	return string(b)
}

func TestLoomAfterStep_Blocked_WritesHaltNoteAndReflects(t *testing.T) {
	t.Parallel()

	f := newHaltFixture(t)
	res := shedengine.StepResult{
		Producer: loomshed.NameWebster,
		State:    shedengine.StateBlocked,
		Reason:   "webster stuck on batch 3",
		History:  make([]shedengine.HistoryEntry, 4),
	}

	if got := f.c.loomAfterStep(context.Background(), res, nil); got != frictionengine.StatusReflected {
		t.Fatalf("loomAfterStep() = %q; want %q", got, frictionengine.StatusReflected)
	}
	if len(f.shuttle.Specs) != 1 {
		t.Fatalf("reflection shuttle ran %d times; want 1", len(f.shuttle.Specs))
	}
	if !strings.Contains(f.shuttle.Specs[0].Prompt, "loom-halt.md") {
		t.Errorf("reflection prompt does not name the halt note: %q", f.shuttle.Specs[0].Prompt)
	}
}

func TestWriteHaltNote_NamesProducerAndReason(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	err := writeHaltNote(dir, haltNote{Producer: loomshed.NameWebster, State: shedengine.StateBlocked, Reason: "webster stuck on batch 3", HistoryLen: 4})
	if err != nil {
		t.Fatalf("writeHaltNote() = %v; want nil", err)
	}

	got := readNote(t, filepath.Join(dir, "loom-halt.md"))
	for _, want := range []string{loomshed.NameWebster, "webster stuck on batch 3", "blocked", "history_entries: 4"} {
		if !strings.Contains(got, want) {
			t.Errorf("halt note %q does not contain %q", got, want)
		}
	}
}

func TestWriteHaltNote_EmptyDirectoryWritesNothing(t *testing.T) {
	t.Parallel()

	if err := writeHaltNote("", haltNote{Producer: "p", State: shedengine.StateBlocked}); err != nil {
		t.Errorf("writeHaltNote(\"\") = %v; want nil", err)
	}
}

func TestLoomAfterStep_ProducerError_FailedStatusReflects(t *testing.T) {
	t.Parallel()

	f := newHaltFixture(t)
	f.seedStatus(t, loomshed.NameWebster, shedengine.StateFailed, "producer exploded")

	got := f.c.loomAfterStep(context.Background(), shedengine.StepResult{}, errors.New("producer exploded"))
	if got != frictionengine.StatusReflected {
		t.Fatalf("loomAfterStep() = %q; want %q", got, frictionengine.StatusReflected)
	}
	if len(f.shuttle.Specs) != 1 {
		t.Errorf("reflection shuttle ran %d times; want 1", len(f.shuttle.Specs))
	}
}

func TestLoomAfterStep_BusyAndStaleFailed_WriteNothing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{"Busy", fmt.Errorf("%w: lock", shedengine.ErrShedBusy)},
		{"StaleFailedWithADifferentError", errors.New("a newer failure")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newHaltFixture(t)
			f.seedStatus(t, loomshed.NameWebster, shedengine.StateFailed, "an older failure")

			if got := f.c.loomAfterStep(context.Background(), shedengine.StepResult{}, tt.err); got != frictionengine.StatusSkipped {
				t.Errorf("loomAfterStep() = %q; want %q", got, frictionengine.StatusSkipped)
			}
			f.requireNoNotesNoSpawn(t)
		})
	}
}

func TestLoomAfterStep_Done_ReportsTheRowStatus(t *testing.T) {
	t.Parallel()

	f := newHaltFixture(t)
	done := shedengine.StepResult{State: shedengine.StateDone}

	f.c.rowFrictionStatus = frictionengine.StatusFailed
	if got := f.c.loomAfterStep(context.Background(), done, nil); got != frictionengine.StatusFailed {
		t.Errorf("loomAfterStep() = %q; want %q", got, frictionengine.StatusFailed)
	}

	f.c.rowFrictionStatus = ""
	if got := f.c.loomAfterStep(context.Background(), done, nil); got != frictionengine.StatusSkipped {
		t.Errorf("loomAfterStep() with no row run = %q; want %q", got, frictionengine.StatusSkipped)
	}
	f.requireNoNotesNoSpawn(t)
}

func TestLoomAfterStep_AwaitingAndPaused_WriteNothing(t *testing.T) {
	t.Parallel()

	for _, st := range []shedengine.State{shedengine.StateAwaiting, shedengine.StatePaused} {
		t.Run(string(st), func(t *testing.T) {
			t.Parallel()

			f := newHaltFixture(t)
			res := shedengine.StepResult{State: st, Reason: "hand-off"}

			if got := f.c.loomAfterStep(context.Background(), res, nil); got != frictionengine.StatusSkipped {
				t.Errorf("loomAfterStep() = %q; want %q", got, frictionengine.StatusSkipped)
			}
			f.requireNoNotesNoSpawn(t)
		})
	}
}

func TestLoomAfterStep_UnwritableFrictionDirectory_StillReturnsAStatus(t *testing.T) {
	t.Parallel()

	f := newHaltFixture(t)
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("a file, not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) = %v; want nil", blocker, err)
	}
	f.c.frictionDir = filepath.Join(blocker, "friction")

	res := shedengine.StepResult{Producer: loomshed.NameWebster, State: shedengine.StateBlocked, Reason: "stuck"}
	if got := f.c.loomAfterStep(context.Background(), res, nil); got == "" {
		t.Error("loomAfterStep() = \"\"; want a status")
	}
}

func TestLoomPostRun_HaltNotes(t *testing.T) {
	t.Parallel()

	t.Run("Blocked", func(t *testing.T) {
		t.Parallel()

		f := newHaltFixture(t)
		res := shedengine.Result{Outcome: shedengine.RunBlocked, HaltedProducer: loomshed.NameWebster, Reason: "stuck", History: make([]shedengine.HistoryEntry, 2)}

		if got := f.c.loomPostRun(context.Background(), res, nil)["friction"]; got != frictionengine.StatusReflected {
			t.Fatalf("friction = %v; want %q", got, frictionengine.StatusReflected)
		}
		if len(f.shuttle.Specs) != 1 {
			t.Errorf("reflection shuttle ran %d times; want 1", len(f.shuttle.Specs))
		}
	})

	t.Run("FailedRunError", func(t *testing.T) {
		t.Parallel()

		f := newHaltFixture(t)
		f.seedStatus(t, loomshed.NameWebster, shedengine.StateFailed, "producer exploded")

		if got := f.c.loomPostRun(context.Background(), shedengine.Result{}, errors.New("producer exploded"))["friction"]; got != frictionengine.StatusReflected {
			t.Fatalf("friction = %v; want %q", got, frictionengine.StatusReflected)
		}
		if len(f.shuttle.Specs) != 1 {
			t.Errorf("reflection shuttle ran %d times; want 1", len(f.shuttle.Specs))
		}
	})

	t.Run("OtherRunErrorSkips", func(t *testing.T) {
		t.Parallel()

		f := newHaltFixture(t)
		f.seedStatus(t, loomshed.NameWebster, shedengine.StateFailed, "an older failure")

		if got := f.c.loomPostRun(context.Background(), shedengine.Result{}, errors.New("a newer failure"))["friction"]; got != frictionengine.StatusSkipped {
			t.Errorf("friction = %v; want %q", got, frictionengine.StatusSkipped)
		}
		f.requireNoNotesNoSpawn(t)
	})

	t.Run("Awaiting", func(t *testing.T) {
		t.Parallel()

		f := newHaltFixture(t)
		res := shedengine.Result{Outcome: shedengine.RunAwaiting, HaltedProducer: loomshed.NamePRGate, Reason: "approve"}

		if got := f.c.loomPostRun(context.Background(), res, nil)["friction"]; got != frictionengine.StatusSkipped {
			t.Errorf("friction = %v; want %q", got, frictionengine.StatusSkipped)
		}
		f.requireNoNotesNoSpawn(t)
	})
}

// funcProducer adapts a func to shedengine.ShedProducer.
type funcProducer func(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)

func (fn funcProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return fn(ctx)
}

// stepEnvelopeOver drives the generic step body over f.c.specFor("step") with PreStep cleared and BuildShed replaced by a one-row shed whose Webster producer is p.
func (f *haltFixture) stepEnvelopeOver(t *testing.T, p shedengine.ShedProducer) (map[string]any, int) {
	t.Helper()

	f.seedStatus(t, loomshed.NameWebster, shedengine.StateRunning, "")
	spec := f.c.specFor("step")
	spec.Hooks.PreStep = nil
	spec.BuildShed = func() (*shedengine.Shed, error) {
		return &shedengine.Shed{
			Producers:      []shedengine.ProducerDef{{Name: loomshed.NameWebster, Producer: p}},
			StatusPath:     f.c.shedPaths.StatusPath,
			LockPath:       f.c.shedPaths.LockPath,
			StatusLockPath: f.c.shedPaths.StatusLockPath,
		}, nil
	}
	f.c.spec = &spec

	var cmd *cobra.Command
	for _, v := range shedverbs.Verbs(loomVerbTexts, &spec) {
		if v.Name() == "step" {
			cmd = v
		}
	}
	var out bytes.Buffer
	code := clihelp.Execute(cmd, &out, nil)

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var envelope map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &envelope); err != nil {
		t.Fatalf("decode envelope %q = %v; want nil", out.String(), err)
	}
	return envelope, code
}

func TestStep_BlockedHaltReflectsOverRefusalAndHaltNotes(t *testing.T) {
	t.Parallel()

	f := newHaltFixture(t)
	refuse := funcProducer(func(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
		err := websterengine.WriteRefusalNote(f.frictionDir, websterengine.RefusalNote{
			Verb:    "begin-batch",
			Args:    "03",
			Message: "the plan drifted",
			Fields:  map[string]any{"plan_drifted": true},
		})
		if err != nil {
			t.Errorf("WriteRefusalNote() = %v; want nil", err)
		}
		return shedengine.Stuck, shedengine.OutputPointer{}, nil
	})

	envelope, _ := f.stepEnvelopeOver(t, refuse)

	if envelope["state"] != string(shedengine.StateBlocked) {
		t.Fatalf("envelope state = %v; want %q; envelope = %v", envelope["state"], shedengine.StateBlocked, envelope)
	}
	if envelope["friction"] != frictionengine.StatusReflected {
		t.Errorf("envelope friction = %v; want %q", envelope["friction"], frictionengine.StatusReflected)
	}
	if len(f.shuttle.Specs) != 1 {
		t.Fatalf("reflection shuttle ran %d times; want 1", len(f.shuttle.Specs))
	}
	prompt := f.shuttle.Specs[0].Prompt
	for _, want := range []string{"webster-refusal-begin-batch.md", "loom-halt.md"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("reflection prompt does not name %q: %q", want, prompt)
		}
	}
}

func TestStep_ProducerErrorReflectsOnTheErrorEnvelope(t *testing.T) {
	t.Parallel()

	f := newHaltFixture(t)
	explode := funcProducer(func(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
		return "", shedengine.OutputPointer{}, errors.New("producer exploded")
	})

	envelope, code := f.stepEnvelopeOver(t, explode)

	if code == 0 || envelope["ok"] != false {
		t.Fatalf("exit = %d, envelope = %v; want an error envelope", code, envelope)
	}
	if envelope["friction"] != frictionengine.StatusReflected {
		t.Errorf("envelope friction = %v; want %q", envelope["friction"], frictionengine.StatusReflected)
	}
	if len(f.shuttle.Specs) != 1 {
		t.Errorf("reflection shuttle ran %d times; want 1", len(f.shuttle.Specs))
	}
}
