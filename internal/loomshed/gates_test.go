// gates_test.go covers NewDiscussionGate and NewPlanGate's own outcome mapping, the ParsePlan error
// split that is the subtlest rule in the task, the fail-closed severity predicate both gates key
// their pass/fail split on, and NewReworkPlanGate's whole-plan check against the told first_card.
//
// It reuses three fixture helpers -- validDecisionRecord, writeDiscussionFixture, and
// seedPlanFormatFixture -- that used to live alongside the two removed validate producers'
// own test files and now live in fixture_test.go, since all three already build exactly the
// on-disk shapes these cases need.
//
// All of it is untagged and offline, per the Test Tier Purity Invariant; the quarry-unavailable case
// asserts on the error path rather than requiring a resolvable fixture, which is what keeps the
// Quarry CGO Requirement Invariant from making this tier depend on a resolvable repository.

package loomshed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/logcapture"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// TestGateRows_StopsNameAWayForward pins the trailing way-forward clause on the gate rows' stops and takes each one:
// the commit-seam failures and the batchifier fault are transient or operator-fixable, so the same row proceeds once the named fix is made.
func TestGateRows_StopsNameAWayForward(t *testing.T) {
	commitFault := errors.New("index.lock exists")
	for _, tt := range []struct {
		name string
		make func(commit func() error) shedengine.ShedProducer
	}{
		{"DiscussionWrite", func(c func() error) shedengine.ShedProducer {
			return NewDiscussionWrite("Discussion-Write", &fakeInnerProducer{outcome: shedengine.Done, pointer: shedengine.OutputPointer{Path: "decision-record.md"}}, c)
		}},
		{"PlanWrite", func(c func() error) shedengine.ShedProducer {
			return NewPlanWrite("Plan-Write", &fakeInnerProducer{outcome: shedengine.Done, pointer: shedengine.OutputPointer{Path: "plan"}}, c)
		}},
		{"Webster", func(c func() error) shedengine.ShedProducer {
			fake := &fakeWebsterRun{}
			anchorPath := t.TempDir()
			writeBatcherConfig(t, anchorPath, `active: "identity"`+"\n")
			return NewWebsterProducer("Webster", anchorPath, fake.run, websterengine.RunDeps{}, c)
		}},
	} {
		t.Run("CommitSeam"+tt.name, func(t *testing.T) {
			fail := true
			p := tt.make(func() error {
				if fail {
					return commitFault
				}
				return nil
			})
			_, _, err := p.Call(context.Background())
			if err == nil || !strings.Contains(err.Error(), "way forward: ") || !strings.Contains(err.Error(), "re-step") {
				t.Fatalf("Call() error = %v; want a trailing way forward naming a re-step", err)
			}
			fail = false
			outcome, _, err := p.Call(context.Background())
			if err != nil || outcome != shedengine.Done {
				t.Errorf("re-step Call() = (%q, %v); want Done after the fault cleared", outcome, err)
			}
		})
	}

	for _, tt := range []struct {
		name string
		make func(anchorPath string) shedengine.ShedProducer
	}{
		{"Batchifier", func(a string) shedengine.ShedProducer { return NewBatchifier("Batchifier", a) }},
		{"Webster", func(a string) shedengine.ShedProducer {
			return NewWebsterProducer("Webster", a, (&fakeWebsterRun{}).run, websterengine.RunDeps{}, func() error { return nil })
		}},
	} {
		t.Run("BrokenBatcherConfig"+tt.name, func(t *testing.T) {
			anchorPath := t.TempDir()
			writeBatcherConfig(t, anchorPath, `active: "no-such-batcher"`+"\n")
			p := tt.make(anchorPath)

			outcome, pointer, err := p.Call(context.Background())
			if err != nil || outcome != shedengine.Stuck {
				t.Fatalf("Call() = (%q, %v); want Stuck with no error", outcome, err)
			}
			if !strings.Contains(pointer.Reason, "way forward: fix batcher.yaml (its active: key or the profile it names)") {
				t.Errorf("Reason = %q; want the batcher.yaml way forward", pointer.Reason)
			}

			writeBatcherConfig(t, anchorPath, `active: "identity"`+"\n")
			outcome, _, err = p.Call(context.Background())
			if err != nil || outcome != shedengine.Done {
				t.Errorf("re-step Call() = (%q, %v); want Done after fixing active:", outcome, err)
			}
		})
	}
}

func TestNewDiscussionGate(t *testing.T) {
	t.Run("Pass", func(t *testing.T) {
		dir := t.TempDir()
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, validDecisionRecord, "support log")

		gate := NewDiscussionGate(decisionRecordPath, supportLogPath)
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if !result.Passed {
			t.Errorf("gate() Passed = %v; want true", result.Passed)
		}
		if result.Findings != "" {
			t.Errorf("gate() Findings = %q; want empty on a pass", result.Findings)
		}
	})

	t.Run("FindingsSurfaceAsAFailedGateAndAWarnLine", func(t *testing.T) {
		dir := t.TempDir()
		withoutGoal := strings.Replace(validDecisionRecord, "## Goal\n\nGoal text.\n\n", "", 1)
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, withoutGoal, "support log")

		buf := logcapture.Capture(t)
		gate := NewDiscussionGate(decisionRecordPath, supportLogPath)
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if result.Passed {
			t.Fatalf("gate() Passed = true; want false for a decision record missing a required heading")
		}
		if !strings.Contains(result.Findings, "discussion-section-missing") {
			t.Errorf("gate() Findings = %q; want it to name the check that fired", result.Findings)
		}

		// The warn line is the only durable record of the refusal: the findings file itself lives in
		// the ephemeral run directory finalize deletes on the Done cleanup, so this assertion is what
		// keeps that promise honest against the closure, not just against the producer that is about to
		// be deleted.
		logged := buf.String()
		if !strings.Contains(logged, result.Findings) {
			t.Errorf("log = %q; want it to carry the same formatted findings as GateResult.Findings", logged)
		}
		if !strings.Contains(logged, "discussion gate failed validation") {
			t.Errorf("log = %q; want it to report the gate refusal", logged)
		}
	})

	// ErrorReturnsRatherThanAFailedGate is the case a validator error is returned rather than reported
	// as a failed gate: a decision record that is a directory is the error fixture the existing
	// validate tests already use (discussionvalidate_test.go's
	// DecisionRecordUnreadableReturnsErrorNotStuck).
	t.Run("ErrorReturnsRatherThanAFailedGate", func(t *testing.T) {
		dir := t.TempDir()
		_, supportLogPath := writeDiscussionFixture(t, dir, "", "support log")
		decisionRecordPath := filepath.Join(dir, "decision-record.md")
		if err := os.MkdirAll(decisionRecordPath, 0o755); err != nil {
			t.Fatalf("mkdir decision record: %v", err)
		}

		gate := NewDiscussionGate(decisionRecordPath, supportLogPath)
		result, err := gate()
		if err == nil {
			t.Fatalf("gate() error = nil; want a non-nil error (decision record path is a directory)")
		}
		if result.Passed || result.Findings != "" {
			t.Errorf("gate() result = %+v; want the zero GateResult alongside a returned error", result)
		}
	})
}

func TestNewPlanGate(t *testing.T) {
	t.Run("Pass", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		seedPlanFormatFixture(t, anchorPath, false)

		gate := NewPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()))
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if !result.Passed {
			t.Errorf("gate() Passed = %v; want true", result.Passed)
		}
	})

	t.Run("FindingsSurfaceAsAFailedGateAndAWarnLine", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		seedFormatInvalidPlanFixture(t, anchorPath)

		buf := logcapture.Capture(t)
		gate := NewPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()))
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if result.Passed {
			t.Fatalf("gate() Passed = true; want false for an unrecognized plan format")
		}
		if !strings.Contains(result.Findings, "format-unrecognized") {
			t.Errorf("gate() Findings = %q; want it to name the check that fired", result.Findings)
		}
		logged := buf.String()
		if !strings.Contains(logged, result.Findings) {
			t.Errorf("log = %q; want it to carry the same formatted findings as GateResult.Findings", logged)
		}
		if !strings.Contains(logged, "plan gate failed validation") {
			t.Errorf("log = %q; want it to report the gate refusal", logged)
		}
	})

	t.Run("PlanglyphErrorReturnsRatherThanAFailedGate", func(t *testing.T) {
		anchorPath := t.TempDir()
		seedGlyphPlanFixture(t, anchorPath, true, "sub#Foo", "")
		// A language: go plan pointed at a worktreeRoot that does not exist: openRepo cannot open it,
		// so the gate must return an error rather than reporting GateResult{Passed: false}.
		worktreeRoot := filepath.Join(t.TempDir(), "does-not-exist")

		gate := NewPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()))
		result, err := gate()
		if err == nil {
			t.Fatalf("gate() error = nil; want a non-nil error for a quarry-unavailable worktreeRoot")
		}
		if !errors.Is(err, planglyph.ErrQuarryUnavailable) {
			t.Errorf("gate() error = %v; want it to wrap planglyph.ErrQuarryUnavailable", err)
		}
		if result.Passed || result.Findings != "" {
			t.Errorf("gate() result = %+v; want the zero GateResult alongside a returned error", result)
		}
	})

	t.Run("InformationalOnlyFindingsPassWithAWarnLine", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := plankit.Repo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
		// "newpkg#Bar" resolves not_found with unit: not_found, which createFindings reports as the
		// informational create-new-unit finding -- no blocking finding in this plan.
		seedGlyphPlanFixture(t, anchorPath, true, "newpkg#Bar", "")

		buf := logcapture.Capture(t)
		gate := NewPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()))
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if !result.Passed {
			t.Fatalf("gate() Passed = false; want true for an informational-only findings set")
		}
		logged := buf.String()
		if !strings.Contains(logged, "create-new-unit") {
			t.Errorf("log = %q; want it to surface the informational finding for visibility on the pass path", logged)
		}
	})

	t.Run("MixedBlockingAndInformationalFailsTheGate", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := plankit.Repo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
		// "newpkg#Bar" is informational (create-new-unit); "sub#Missing" resolves not_found with
		// unit: found, which statusFindings reports as the blocking glyph-not-found finding. One
		// blocking finding is enough to fail the gate.
		seedGlyphPlanFixture(t, anchorPath, true, "newpkg#Bar", "sub#Missing")

		gate := NewPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()))
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if result.Passed {
			t.Fatalf("gate() Passed = true; want false for a set carrying one blocking finding")
		}
	})

	// The cards of a batch webster's run record holds done are history:
	// a run moved back to the plan review after Webster built card 1 must not wedge on card 1's own landed Create.
	builtRepo := map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"}
	doneBatchOne := &websterengine.State{Batches: map[int]*websterengine.BatchState{
		1: {Slug: "first-card", Cards: []string{"01-first-card"}, Terminal: true, Status: websterengine.DigestStatusDone},
	}}
	saveState := func(t *testing.T, anchorPath string, st *websterengine.State) {
		t.Helper()
		if err := websterengine.SaveState(websterengine.Dir(anchorPath), websterengine.ScratchDir(anchorPath), st); err != nil {
			t.Fatalf("SaveState: %v", err)
		}
	}

	t.Run("DoneCardCreateThatExistsPasses", func(t *testing.T) {
		t.Parallel()
		anchorPath := t.TempDir()
		worktreeRoot := plankit.Repo(t, builtRepo)
		seedGlyphPlanFixture(t, anchorPath, true, "sub#Foo", "")
		saveState(t, anchorPath, doneBatchOne)

		result, err := NewPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()))()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if !result.Passed {
			t.Errorf("gate() = %+v; want a pass, card 1's batch is done", result)
		}
	})

	t.Run("NoRunRecordFailsCreateAlreadyExists", func(t *testing.T) {
		t.Parallel()
		anchorPath := t.TempDir()
		worktreeRoot := plankit.Repo(t, builtRepo)
		seedGlyphPlanFixture(t, anchorPath, true, "sub#Foo", "")

		result, err := NewPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()))()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if result.Passed || !strings.Contains(result.Findings, "create-already-exists") {
			t.Errorf("gate() = %+v; want a create-already-exists failure", result)
		}
	})

	t.Run("PendingCardIsStillChecked", func(t *testing.T) {
		t.Parallel()
		anchorPath := t.TempDir()
		worktreeRoot := plankit.Repo(t, builtRepo)
		plan := firstCardPlan(true, "go", "sub#Foo", "")
		plan.Cards = append(plan.Cards, plankit.Card{
			Number: 2,
			Slug:   "second-card",
			Groups: []plankit.Group{{Label: "Edit", Targets: []string{"sub#Missing"}}},
		})
		plankit.Write(t, planparser.PlanDir(anchorPath), plan)
		saveState(t, anchorPath, doneBatchOne)

		result, err := NewPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()))()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if result.Passed || !strings.Contains(result.Findings, "glyph-not-found") {
			t.Errorf("gate() = %+v; want card 2's glyph-not-found failure", result)
		}
		if strings.Contains(result.Findings, "create-already-exists") {
			t.Errorf("gate() Findings = %q; want no finding on done card 1", result.Findings)
		}
	})

	// A done card is frozen: its batch recorded the card file's hash at begin, and an edit since fails the gate.
	cardPath := func(anchorPath string) string {
		return filepath.Join(planparser.PlanDir(anchorPath), "01-first-card.md")
	}
	cardHash := func(t *testing.T, anchorPath string) string {
		t.Helper()
		data, err := os.ReadFile(cardPath(anchorPath))
		if err != nil {
			t.Fatalf("read card file: %v", err)
		}
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}
	appendToCard := func(t *testing.T, anchorPath string) {
		t.Helper()
		f, err := os.OpenFile(cardPath(anchorPath), os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("open card file: %v", err)
		}
		defer f.Close()
		if _, err := f.WriteString("\nan edit after the batch began\n"); err != nil {
			t.Fatalf("append to card file: %v", err)
		}
	}
	batchOne := func(terminal bool, status string, hashes map[string]string) *websterengine.State {
		return &websterengine.State{Batches: map[int]*websterengine.BatchState{
			1: {Slug: "first-card", Cards: []string{"01-first-card"}, Terminal: terminal, Status: status, CardHashes: hashes},
		}}
	}

	for _, tt := range []struct {
		name string
		// record builds the run record from the card file's hash before any edit; nil saves no record.
		record      func(hash string) *websterengine.State
		edit        bool
		wantEdited  bool
		wantPassing bool
	}{
		{"EditedDoneCardFails", func(h string) *websterengine.State {
			return batchOne(true, websterengine.DigestStatusDone, map[string]string{"01-first-card": h})
		}, true, true, false},
		{"UneditedDoneCardReportsNothing", func(h string) *websterengine.State {
			return batchOne(true, websterengine.DigestStatusDone, map[string]string{"01-first-card": h})
		}, false, false, true},
		{"EditedInFlightCardReportsNothing", func(h string) *websterengine.State {
			return batchOne(false, "", map[string]string{"01-first-card": h})
		}, true, false, true},
		{"NoRunRecordReportsNothing", nil, true, false, true},
		{"RecordWithoutHashesReportsNothing", func(string) *websterengine.State {
			return batchOne(true, websterengine.DigestStatusDone, nil)
		}, true, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchorPath := t.TempDir()
			seedPlanFormatFixture(t, anchorPath, true)
			if tt.record != nil {
				saveState(t, anchorPath, tt.record(cardHash(t, anchorPath)))
			}
			if tt.edit {
				appendToCard(t, anchorPath)
			}

			result, err := NewPlanGate(anchorPath, t.TempDir(), planglyph.NewIndex(fabricengine.NewReferenceRule()))()
			if err != nil {
				t.Fatalf("gate() error = %v; want nil", err)
			}
			if result.Passed != tt.wantPassing {
				t.Errorf("gate() = %+v; want Passed %v", result, tt.wantPassing)
			}
			if got := strings.Contains(result.Findings, "done-card-edited/01-first-card"); got != tt.wantEdited {
				t.Errorf("gate() Findings = %q; want done-card-edited on card 1: %v", result.Findings, tt.wantEdited)
			}
		})
	}

	t.Run("UnreadableDoneCardIsAReturnedError", func(t *testing.T) {
		t.Parallel()
		anchorPath := t.TempDir()
		seedPlanFormatFixture(t, anchorPath, true)
		saveState(t, anchorPath, batchOne(true, websterengine.DigestStatusDone, map[string]string{"01-first-card": cardHash(t, anchorPath)}))
		plan, err := planparser.ParsePlan(planparser.PlanDir(anchorPath))
		if err != nil {
			t.Fatalf("ParsePlan: %v", err)
		}
		if err := os.Remove(cardPath(anchorPath)); err != nil {
			t.Fatalf("remove card file: %v", err)
		}

		findings, err := ValidatePlan(plan, anchorPath, t.TempDir(), planglyph.NewIndex(fabricengine.NewReferenceRule()))
		if err == nil || findings != nil {
			t.Errorf("ValidatePlan() = %v, %v; want a returned error and no findings", findings, err)
		}
	})
}

// writeOverviewOnlyPlanDir creates the plan directory under anchorPath and writes only overview
// (00-overview.md's content), leaving every card file absent -- the shape the ParsePlan
// per-card-read-fault fixtures below build on top of.
func writeOverviewOnlyPlanDir(t *testing.T, anchorPath, overview string) (planDir string) {
	t.Helper()
	planDir = filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview file: %v", err)
	}
	return planDir
}

// oneCardIndexLine is the Card Index line firstCardOverview renders for the sole card.
const oneCardIndexLine = "1 — first-card — placeholder card 1"

// firstCardOverview returns the overview plankit renders for firstCardPlan's one-card plan.
func firstCardOverview() string {
	return string(plankit.Render(firstCardPlan(true, "none", "internal/firstcard/new.go", ""))["00-overview.md"])
}

// TestNewPlanGate_ParsePlanSplit covers the ParsePlan error split -- the subtlest rule in the task:
// a malformed overview, an unparseable card index, and an absent 00-overview.md each produce
// Passed false with the error's own text as findings, while both of ParsePlan's read faults produce
// a returned error -- the overview read and the per-card read reached through parseCardFile.
func TestNewPlanGate_ParsePlanSplit(t *testing.T) {
	t.Run("MalformedOverviewFrontmatterIsFindings", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		// Unparseable YAML frontmatter: yaml.Decode fails, which ParsePlan wraps as a plain
		// fmt.Errorf, never a *fs.PathError.
		writeOverviewOnlyPlanDir(t, anchorPath, "---\nformat: [not, valid\n---\n\n## Card Index\n\n1 — c — c\n")

		gate := NewPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()))
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil (a malformed overview is findings, not a returned error)", err)
		}
		if result.Passed {
			t.Fatalf("gate() Passed = true; want false for an unparseable overview")
		}
		if result.Findings == "" {
			t.Errorf("gate() Findings = %q; want the ParsePlan error's own text", result.Findings)
		}
	})

	t.Run("UnparseableCardIndexLineIsFindings", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		// A Card Index line that matches none of cardIndexLineRe's accepted separators.
		overview := strings.Replace(firstCardOverview(), oneCardIndexLine, "not a valid card index line", 1)
		if !strings.Contains(overview, "not a valid card index line") {
			t.Fatalf("overview does not carry the index line %q", oneCardIndexLine)
		}
		writeOverviewOnlyPlanDir(t, anchorPath, overview)

		gate := NewPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()))
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil (an unparseable card index line is findings, not a returned error)", err)
		}
		if result.Passed {
			t.Fatalf("gate() Passed = true; want false for an unparseable card index line")
		}
		if result.Findings == "" {
			t.Errorf("gate() Findings = %q; want the ParsePlan error's own text", result.Findings)
		}
	})

	t.Run("AbsentOverviewIsFindings", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		// No _lyx/plan/00-overview.md at all: ParsePlan's os.IsNotExist branch returns a plain
		// fmt.Errorf, never wrapping the underlying *fs.PathError with %w -- so it is findings, the same
		// disposition discussionparser.Validate already gives a missing file.

		gate := NewPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()))
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil (an absent overview is findings, not a returned error)", err)
		}
		if result.Passed {
			t.Fatalf("gate() Passed = true; want false for an absent plan overview")
		}
		if result.Findings == "" {
			t.Errorf("gate() Findings = %q; want the ParsePlan error's own text", result.Findings)
		}
	})

	t.Run("OverviewReadFaultIsAReturnedError", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		planDir := filepath.Join(anchorPath, lyxdirs.LyxDirName, "plan")
		if err := os.MkdirAll(planDir, 0o755); err != nil {
			t.Fatalf("mkdir plan dir: %v", err)
		}
		// A directory at 00-overview.md's own path forces os.ReadFile to fail with a non-not-exist
		// *fs.PathError, which ParsePlan wraps with %w -- the overview read fault.
		if err := os.MkdirAll(filepath.Join(planDir, "00-overview.md"), 0o755); err != nil {
			t.Fatalf("mkdir overview path: %v", err)
		}

		gate := NewPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()))
		result, err := gate()
		if err == nil {
			t.Fatalf("gate() error = nil; want a non-nil error for an unreadable overview file")
		}
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			t.Errorf("gate() error = %v; want it to wrap a *fs.PathError", err)
		}
		if result.Passed || result.Findings != "" {
			t.Errorf("gate() result = %+v; want the zero GateResult alongside a returned error", result)
		}
	})

	t.Run("PerCardReadFaultIsAReturnedError", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		planDir := writeOverviewOnlyPlanDir(t, anchorPath, firstCardOverview())
		// A directory at the card file's own path, reached through parseCardFile, forces its
		// os.ReadFile to fail the same non-not-exist way the overview read fault above does.
		if err := os.MkdirAll(filepath.Join(planDir, "01-first-card.md"), 0o755); err != nil {
			t.Fatalf("mkdir card file path: %v", err)
		}

		gate := NewPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()))
		result, err := gate()
		if err == nil {
			t.Fatalf("gate() error = nil; want a non-nil error for an unreadable card file")
		}
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			t.Errorf("gate() error = %v; want it to wrap a *fs.PathError", err)
		}
		if result.Passed || result.Findings != "" {
			t.Errorf("gate() result = %+v; want the zero GateResult alongside a returned error", result)
		}
	})
}

// TestHasBlockingFinding_AgainstTheGate exercises the same fail-closed severity table planvalidate_test.go's TestHasBlockingFinding_UnrecognizedSeverityFailsClosed pins, asserted here against the exact predicate NewPlanGate's own pass/fail split calls: an unrecognized or zero-value Severity fails the gate closed rather than silently passing, because planindex.Severity is an open string type and neither shape can occur through a real resolve-backed findings set -- only a hand-built Finding, or a future producer that forgets to stamp one, can carry either.
//
//testtiming:keep pins that an unrecognized or zero severity fails the plan gate closed, which TestNewPlanGate never asserts
func TestHasBlockingFinding_AgainstTheGate(t *testing.T) {
	for _, tt := range []struct {
		name     string
		severity planindex.Severity
		want     bool
	}{
		{"blocking blocks the gate", planindex.SeverityBlocking, true},
		{"informational passes the gate", planindex.SeverityInformational, false},
		{"the zero value blocks the gate", planindex.Severity(""), true},
		{"an unrecognized severity blocks the gate", planindex.Severity("advisory"), true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := hasBlockingFinding([]planindex.Finding{{Check: "some-check", Severity: tt.severity}})
			if got != tt.want {
				t.Errorf("hasBlockingFinding(severity %q) = %v; want %v", tt.severity, got, tt.want)
			}
		})
	}
}

// seedReworkGlyphPlan writes a one-card language: go plan under anchorPath: a new generation whose card is numbered cardNumber, declares first_card: cardNumber when that is above 1, creates the glyph creates and, when uses is non-empty, also uses it.
// It returns the plan committed at HEAD before the rework round, keyed by anchor-relative path: a one-card generation whose card 1 creates sub#Foo, already built in the worktree.
// The told number is therefore 2.
func seedReworkGlyphPlan(t *testing.T, anchorPath string, cardNumber int, creates, uses string) map[string][]byte {
	t.Helper()
	var usesTargets []string
	if uses != "" {
		usesTargets = []string{uses}
	}
	firstCard := 0
	if cardNumber > 1 {
		firstCard = cardNumber
	}
	plankit.Write(t, planparser.PlanDir(anchorPath), plankit.Plan{
		Approved:  true,
		Language:  "go",
		FirstCard: firstCard,
		Framing:   "Framing.",
		Cards: []plankit.Card{{
			Number:  cardNumber,
			Slug:    "new-card",
			Summary: fmt.Sprintf("placeholder card %d", cardNumber),
			Groups:  []plankit.Group{{Label: "Create", Targets: []string{creates}}},
			Uses:    usesTargets,
			Intent:  "new generation card.",
		}},
	})
	committed := map[string][]byte{}
	for name, data := range plankit.Render(firstCardPlan(true, "go", "sub#Foo", "")) {
		committed[path.Join(planparser.PlanDirRel(), name)] = data
	}
	return committed
}

// committedReader returns a ReadCommitted seam over committed.
func committedReader(committed map[string][]byte) func(string) ([]byte, bool, error) {
	return func(rel string) ([]byte, bool, error) {
		data, ok := committed[rel]
		return data, ok, nil
	}
}

func TestNewReworkPlanGate(t *testing.T) {
	builtRepo := map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"}

	t.Run("WholeNewPlanNumberedFromToldCardPasses", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := plankit.Repo(t, builtRepo)
		committed := seedReworkGlyphPlan(t, anchorPath, 2, "newpkg#Bar", "")

		result, err := NewReworkPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()), committedReader(committed))()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if !result.Passed {
			t.Errorf("gate() = %+v; want a pass", result)
		}
	})

	t.Run("FirstCardDifferingFromToldNumberFails", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := plankit.Repo(t, builtRepo)
		committed := seedReworkGlyphPlan(t, anchorPath, 1, "newpkg#Bar", "")

		result, err := NewReworkPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()), committedReader(committed))()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if result.Passed || !strings.Contains(result.Findings, "rework-first-card") {
			t.Errorf("gate() = %+v; want a rework-first-card failure", result)
		}
	})

	t.Run("EveryCardIsResolved", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := plankit.Repo(t, builtRepo)
		committed := seedReworkGlyphPlan(t, anchorPath, 2, "newpkg#Bar", "sub#Missing")

		result, err := NewReworkPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()), committedReader(committed))()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if result.Passed || !strings.Contains(result.Findings, "glyph-not-found") {
			t.Errorf("gate() = %+v; want a glyph-not-found failure", result)
		}
	})

	t.Run("NoCardIsExemptedAsBuilt", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := plankit.Repo(t, builtRepo)
		committed := seedReworkGlyphPlan(t, anchorPath, 2, "sub#Foo", "")

		result, err := NewReworkPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()), committedReader(committed))()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if result.Passed || !strings.Contains(result.Findings, "create-already-exists") {
			t.Errorf("gate() = %+v; want a create-already-exists failure, since a new-generation card that creates a built symbol is a defect", result)
		}
	})

	t.Run("NoCommittedPlanReturnsAnError", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := plankit.Repo(t, builtRepo)
		seedReworkGlyphPlan(t, anchorPath, 2, "newpkg#Bar", "")

		result, err := NewReworkPlanGate(anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()), committedReader(nil))()
		if err == nil {
			t.Fatalf("gate() = %+v, nil; want an error when HEAD carries no plan", result)
		}
	})
}
