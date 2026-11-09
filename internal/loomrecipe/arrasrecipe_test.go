// arrasrecipe_test.go pins the arras recipe's rows, routing and split perch keys.
// It also drives an arras run through Discussion, the split perch and Raddle-File over the sequence fixture.
// It drives gotos into Raddle-Write too.

package loomrecipe

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/recipes"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shedbuild"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// splitPerchSegment is the arras recipe's split perch segment.
const splitPerchSegment = "Raddle-Review"

// TestArrasRecipe pins the arras recipe's Motif skip, routing, split perch segment, absent seams and fixer gate.
func TestArrasRecipe(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	env, paths := testEnv(t)
	if _, err := New(shedrun.RecipeArras, env, paths); err != nil {
		t.Fatalf("New(arras) error = %v; want nil, which holds the loader's record-only form for the recipe", err)
	}

	rows, err := Rows(shedrun.RecipeArras)
	if err != nil {
		t.Fatalf("Rows(arras) error = %v; want nil", err)
	}
	byName := map[string]RecipeRow{}
	for _, row := range rows {
		byName[row.Name] = row
	}

	if got := byName[loomshed.NameMotif].SkipTo; got != loomshed.NameRaddleWrite {
		t.Errorf("Motif SkipTo = %q; want %q, so a discussed entry resumes at Raddle-Write", got, loomshed.NameRaddleWrite)
	}
	routes := []struct {
		row, onDone, onStuck string
	}{
		{loomshed.NameDiscussionBouncer, loomshed.NameRaddleWrite, loomshed.NameDiscussionBurlerReview},
		{loomshed.NameRaddleWrite, loomshed.NameRaddleBouncer, ""},
		{loomshed.NameRaddleBouncer, loomshed.NameRaddleFile, loomshed.NameRaddleBurlerReview},
		{loomshed.NameRaddleBurlerReview, loomshed.NameRaddleBurlerFix, loomshed.NameRaddleBurlerFix},
		{loomshed.NameRaddleBurlerFix, loomshed.NameRaddleBouncer, loomshed.NameRaddleBouncer},
		{loomshed.NameRaddleFile, "", loomshed.NameRaddleWrite},
	}
	for _, route := range routes {
		if got := byName[route.row]; got.OnDone != route.onDone || got.OnStuck != route.onStuck {
			t.Errorf("%s on_done = %q, on_stuck = %q; want %q and %q", route.row, got.OnDone, got.OnStuck, route.onDone, route.onStuck)
		}
	}
	for _, row := range []string{loomshed.NameRaddleBouncer, loomshed.NameRaddleBurlerReview, loomshed.NameRaddleBurlerFix} {
		if got := byName[row].Segment; got != splitPerchSegment {
			t.Errorf("%s segment = %q; want %q", row, got, splitPerchSegment)
		}
	}

	parsed, err := shedbuild.Parse(recipes.ArrasRecipe)
	if err != nil {
		t.Fatalf("shedbuild.Parse(arras recipe) = %v", err)
	}
	configs := map[string]map[string]any{}
	for _, row := range parsed.Producers {
		configs[row.Name] = row.Config
	}
	for _, key := range []string{"carry_over", "approve_seam", "commit_seam", "subject"} {
		if value, present := configs[loomshed.NameRaddleBouncer][key]; present {
			t.Errorf("Raddle-Bouncer config[%q] = %v; want it absent", key, value)
		}
	}
	wantGate := []any{map[string]any{"name": "raddle", "attempts": 3}}
	if got := configs[loomshed.NameRaddleBurlerFix]["gates"]; !reflect.DeepEqual(got, wantGate) {
		t.Errorf("Raddle-Burler-Fix gates = %v; want %v, so a fixed proposal passes every proposal check", got, wantGate)
	}
}

// arrasScriptedRow is a fake row that runs before on each call and returns its outcomes in turn, repeating the last.
type arrasScriptedRow struct {
	outcomes []shedengine.Outcome
	before   func()
	calls    int
}

func (r *arrasScriptedRow) Call(context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if r.before != nil {
		r.before()
	}
	outcome := r.outcomes[min(r.calls, len(r.outcomes)-1)]
	r.calls++
	return outcome, shedengine.OutputPointer{Reason: "scripted " + string(outcome)}, nil
}

// arrasEscalationHardCap is the split perch's hard cap in a run whose perch escalates, well above its budget of 1.
const arrasEscalationHardCap = 4

// arrasRunOptions shapes the split perch and Raddle-File of a run newArrasRun builds.
type arrasRunOptions struct {
	// fileHaltsOnStuck clears Raddle-File's on_stuck, so a Stuck outcome halts the run there.
	fileHaltsOnStuck bool
	// splitEscalates makes the split perch's judge rule CONTINUE over one open design-wrong entry every round,
	// so the perch escalates with cause circling, below its hard cap, and awaits its parent.
	splitEscalates bool
}

// newArrasRun builds an arras run over the sequence fixture with the split perch's budget at 1.
// Raddle-Write is a fake that writes a proposal and reports Done.
// Raddle-File is a fake that reports fileOutcomes in turn.
// The Discussion rows and the split perch's rows are the real ones.
func newArrasRun(t *testing.T, options arrasRunOptions, fileOutcomes ...shedengine.Outcome) (*shedengine.Shed, string, *arrasScriptedRow) {
	t.Helper()
	anchor, env, paths := buildSequenceFixture(t)
	split := env.Segments[splitPerchSegment]
	split.MaxBounces = 1
	if options.splitEscalates {
		split.HardMaxBounces = arrasEscalationHardCap
		shuttle := env.Shuttle.(*shedfake.Shuttle)
		dispatch := shuttle.RunFn
		designWrongBouncerJudge(env)
		designWrong := shuttle.RunFn
		splitRunDir := string(filepath.Separator) + "split" + string(filepath.Separator)
		shuttle.RunFn = func(spec shuttleengine.Spec) (shuttleengine.Result, error) {
			if strings.HasSuffix(spec.Role, "bouncer-judge") && len(spec.OutputFiles) > 0 && strings.Contains(spec.OutputFiles[0], splitRunDir) {
				return designWrong(spec)
			}
			return dispatch(spec)
		}
		env.SegmentBounces = func(row string) (int, int, bool, error) {
			st, found, err := state.ReadJSON[shedengine.Status](env.StatusPath, env.StatusLockPath)
			if err != nil || !found {
				return 0, 0, false, err
			}
			routing, err := Routing(shedrun.RecipeArras, budgetsOf(env))
			if err != nil {
				return 0, 0, false, err
			}
			count, budget, inSegment := routing.Bounces(row, st.History)
			return count, budget, inSegment, nil
		}
	}
	env.Segments[splitPerchSegment] = split

	shed, err := New(shedrun.RecipeArras, env, paths)
	if err != nil {
		t.Fatalf("New(arras) error = %v; want nil", err)
	}
	proposal := filepath.Join(anchor, loomengine.RaddleProposalRel())
	write := &arrasScriptedRow{outcomes: []shedengine.Outcome{shedengine.Done}, before: func() {
		if err := os.MkdirAll(filepath.Dir(proposal), 0o755); err != nil {
			t.Errorf("mkdir the proposal's directory: %v", err)
		}
		if err := os.WriteFile(proposal, []byte("shape: tapestry\n"), 0o644); err != nil {
			t.Errorf("write the proposal: %v", err)
		}
	}}
	file := &arrasScriptedRow{outcomes: fileOutcomes}
	for i := range shed.Producers {
		switch shed.Producers[i].Name {
		case loomshed.NameRaddleWrite:
			shed.Producers[i].Producer = write
		case loomshed.NameRaddleFile:
			shed.Producers[i].Producer = file
			if options.fileHaltsOnStuck {
				shed.Producers[i].OnStuck = ""
			}
		}
	}
	return shed, filepath.Join(env.RunRoot, "split"), file
}

// historyOf returns the producers of history whose name is among names, in order.
func historyOf(history []shedengine.HistoryEntry, names ...string) []string {
	var got []string
	for _, entry := range history {
		if slices.Contains(names, entry.Producer) {
			got = append(got, entry.Producer)
		}
	}
	return got
}

// gotoRaddleWrite moves a halted arras run onto Raddle-Write, as `lym loom goto --to Raddle-Write` does.
func gotoRaddleWrite(t *testing.T, shed *shedengine.Shed) {
	t.Helper()
	if _, err := shedengine.Goto(shedengine.GotoRequest{
		StatusPath:     shed.StatusPath,
		LockPath:       shed.LockPath,
		StatusLockPath: shed.StatusLockPath,
		Producers:      shed.Producers,
		Target:         loomshed.NameRaddleWrite,
	}); err != nil {
		t.Fatalf("Goto(%s) error = %v; want nil", loomshed.NameRaddleWrite, err)
	}
}

// TestArrasRun_GotoIntoRaddleWrite drives the two gotos into Raddle-Write that the split perch meets on re-entry.
// A goto from Raddle-File over a settled perch clears the perch, so the new proposal gets a second review round.
// A goto while the perch awaits its parent on an escalation below the hard cap clears nothing:
// the Bouncer settles its recorded round again and raises the escalation again, with no further review round.
func TestArrasRun_GotoIntoRaddleWrite(t *testing.T) {
	tiergate.Require(t, 0)

	t.Run("from Raddle-File over a settled perch re-arms the perch", func(t *testing.T) {
		shed, splitRunDir, _ := newArrasRun(t, arrasRunOptions{fileHaltsOnStuck: true}, shedengine.Stuck, shedengine.Done)

		first, err := shed.Run(context.Background())
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if first.Outcome != shedengine.RunBlocked || first.HaltedProducer != loomshed.NameRaddleFile {
			t.Fatalf("Run() = %q at %q (reason %q); want %q at %q", first.Outcome, first.HaltedProducer, first.Reason, shedengine.RunBlocked, loomshed.NameRaddleFile)
		}

		gotoRaddleWrite(t, shed)
		second, err := shed.Run(context.Background())
		if err != nil {
			t.Fatalf("Run() after the goto error = %v; want nil", err)
		}
		if second.Outcome != shedengine.RunDone {
			t.Fatalf("Run() after the goto = %q at %q (reason %q); want %q", second.Outcome, second.HaltedProducer, second.Reason, shedengine.RunDone)
		}
		// The goto records its own Raddle-Write entry between the two cycles.
		want := []string{
			loomshed.NameRaddleWrite, loomshed.NameRaddleBurlerReview, loomshed.NameRaddleFile,
			loomshed.NameRaddleWrite,
			loomshed.NameRaddleWrite, loomshed.NameRaddleBurlerReview, loomshed.NameRaddleFile,
		}
		got := historyOf(second.History, loomshed.NameRaddleWrite, loomshed.NameRaddleBurlerReview, loomshed.NameRaddleFile)
		if !slices.Equal(got, want) {
			t.Errorf("history of the Raddle rows = %v; want %v", got, want)
		}
		if _, err := os.Stat(filepath.Join(splitRunDir, "round-1-review.md")); err != nil {
			t.Errorf("stat the split perch's round 1 review: %v; want the fresh cycle's review in the perch's run directory", err)
		}
	})

	t.Run("while the perch awaits its parent below the hard cap raises the escalation again", func(t *testing.T) {
		shed, splitRunDir, file := newArrasRun(t, arrasRunOptions{splitEscalates: true}, shedengine.Done)

		first, err := shed.Run(context.Background())
		if err != nil {
			t.Fatalf("Run() error = %v; want nil", err)
		}
		if first.Outcome != shedengine.RunAwaiting || first.HaltedProducer != loomshed.NameRaddleBouncer || !strings.Contains(first.Reason, "(cause: circling)") {
			t.Fatalf("Run() = %q at %q (reason %q); want %q at %q with cause circling", first.Outcome, first.HaltedProducer, first.Reason, shedengine.RunAwaiting, loomshed.NameRaddleBouncer)
		}
		reviews := len(historyOf(first.History, loomshed.NameRaddleBurlerReview))

		gotoRaddleWrite(t, shed)
		second, err := shed.Run(context.Background())
		if err != nil {
			t.Fatalf("Run() after the goto error = %v; want nil", err)
		}
		if second.Outcome != shedengine.RunAwaiting || second.HaltedProducer != loomshed.NameRaddleBouncer || !strings.Contains(second.Reason, "(cause: circling)") {
			t.Fatalf("Run() after the goto = %q at %q (reason %q); want %q at %q with cause circling again", second.Outcome, second.HaltedProducer, second.Reason, shedengine.RunAwaiting, loomshed.NameRaddleBouncer)
		}
		if got := len(historyOf(second.History, loomshed.NameRaddleBurlerReview)); got != reviews {
			t.Errorf("Raddle-Burler-Review entries after the goto = %d; want %d, no fresh cycle", got, reviews)
		}
		if got := len(historyOf(second.History, loomshed.NameRaddleWrite)); got != 3 {
			t.Errorf("Raddle-Write entries = %d; want 3, the first run, the goto's own entry and its re-run", got)
		}
		if file.calls != 0 {
			t.Errorf("Raddle-File calls = %d; want 0, the unsettled perch filing nothing", file.calls)
		}
		if _, err := os.Stat(filepath.Join(splitRunDir, "round-1-review.md")); err != nil {
			t.Errorf("stat the split perch's round 1 review: %v; want it kept, since nothing was cleared", err)
		}
	})
}

// TestArrasRun_SplitPerchReviewsEveryProposalBeforeFiling drives an arras run to its end.
// The run passes Discussion, then Raddle-Write, then the split perch's review of the proposal, then Raddle-File.
// After a Raddle-File return, the new proposal is reviewed again by the perch with its full budget of 1,
// so the second review round runs, and the run ends, only when the settled perch re-arms on the fresh entry.
func TestArrasRun_SplitPerchReviewsEveryProposalBeforeFiling(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name         string
		fileOutcomes []shedengine.Outcome
		want         []string
	}{
		{
			name:         "a filing on the first proposal",
			fileOutcomes: []shedengine.Outcome{shedengine.Done},
			want:         []string{loomshed.NameDiscussionWrite, loomshed.NameRaddleWrite, loomshed.NameRaddleBurlerReview, loomshed.NameRaddleFile},
		},
		{
			name:         "a Raddle-File return re-arms the perch",
			fileOutcomes: []shedengine.Outcome{shedengine.Stuck, shedengine.Done},
			want: []string{
				loomshed.NameDiscussionWrite,
				loomshed.NameRaddleWrite, loomshed.NameRaddleBurlerReview, loomshed.NameRaddleFile,
				loomshed.NameRaddleWrite, loomshed.NameRaddleBurlerReview, loomshed.NameRaddleFile,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shed, splitRunDir, file := newArrasRun(t, arrasRunOptions{}, tt.fileOutcomes...)

			result, err := shed.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() error = %v; want nil", err)
			}
			if result.Outcome != shedengine.RunDone {
				t.Fatalf("Run() = %q at %q (reason %q); want %q", result.Outcome, result.HaltedProducer, result.Reason, shedengine.RunDone)
			}
			got := historyOf(result.History, loomshed.NameDiscussionWrite, loomshed.NameRaddleWrite, loomshed.NameRaddleBurlerReview, loomshed.NameRaddleFile)
			if !slices.Equal(got, tt.want) {
				t.Errorf("history of the arras rows = %v; want %v", got, tt.want)
			}
			if file.calls != len(tt.fileOutcomes) {
				t.Errorf("Raddle-File calls = %d; want %d", file.calls, len(tt.fileOutcomes))
			}
			if _, err := os.Stat(filepath.Join(splitRunDir, "round-1-review.md")); err != nil {
				t.Errorf("stat the split perch's round 1 review: %v; want the round's review in the perch's run directory", err)
			}
		})
	}
}
