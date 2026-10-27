//go:build tier0

// gates_test.go covers NewDiscussionGate and NewPlanGate's own outcome mapping, the ParsePlan error
// split that is the subtlest rule in the task, the fail-closed severity predicate both gates key
// their pass/fail split on.
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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/pattern"
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
			return NewWebsterProducer("Webster", configengine.Dirs{Anchor: anchorPath}, fake.run, websterengine.RunDeps{}, c)
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
		{"Webster", func(a string) shedengine.ShedProducer {
			return NewWebsterProducer("Webster", configengine.Dirs{Anchor: a}, (&fakeWebsterRun{}).run, websterengine.RunDeps{}, func() error { return nil })
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

		gate := NewDiscussionGate(decisionRecordPath, supportLogPath, filepath.Join(dir, "relevant.md"))
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

	// The gate reads the decision record, the support log and the relevant list, so an advisor's notes file beside them is never judged.
	t.Run("AdvisorNotesAreNotRead", func(t *testing.T) {
		dir := t.TempDir()
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, validDecisionRecord, "support log")
		if err := os.WriteFile(filepath.Join(dir, "advisor-1.md"), []byte("not a decision record, no required heading\n"), 0o644); err != nil {
			t.Fatalf("write advisor notes: %v", err)
		}

		result, err := NewDiscussionGate(decisionRecordPath, supportLogPath, filepath.Join(dir, "relevant.md"))()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if !result.Passed || result.Findings != "" {
			t.Errorf("gate() = %+v; want a pass with no findings", result)
		}
	})

	t.Run("FindingsSurfaceAsAFailedGateAndAWarnLine", func(t *testing.T) {
		dir := t.TempDir()
		withoutGoal := strings.Replace(validDecisionRecord, "## Goal\n\nGoal text.\n\n", "", 1)
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, withoutGoal, "support log")

		buf := logcapture.Capture(t)
		gate := NewDiscussionGate(decisionRecordPath, supportLogPath, filepath.Join(dir, "relevant.md"))
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

	// A malformed relevant list fails the gate alongside the decision record's own findings, which come first.
	t.Run("RelevantFindingsFollowTheRecordFindings", func(t *testing.T) {
		dir := t.TempDir()
		withoutGoal := strings.Replace(validDecisionRecord, "## Goal\n\nGoal text.\n\n", "", 1)
		decisionRecordPath, supportLogPath := writeDiscussionFixture(t, dir, withoutGoal, "support log")
		relevantPath := filepath.Join(dir, "relevant.md")
		if err := os.WriteFile(relevantPath, []byte("a stray paragraph line\n"), 0o644); err != nil {
			t.Fatalf("write relevant list: %v", err)
		}

		result, err := NewDiscussionGate(decisionRecordPath, supportLogPath, relevantPath)()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		sectionAt := strings.Index(result.Findings, "discussion-section-missing")
		strayAt := strings.Index(result.Findings, "relevant-stray-line")
		if result.Passed || sectionAt < 0 || strayAt < sectionAt {
			t.Errorf("gate() = %+v; want a failure naming discussion-section-missing before relevant-stray-line", result)
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

		gate := NewDiscussionGate(decisionRecordPath, supportLogPath, filepath.Join(dir, "relevant.md"))
		result, err := gate()
		if err == nil {
			t.Fatalf("gate() error = nil; want a non-nil error (decision record path is a directory)")
		}
		if result.Passed || result.Findings != "" {
			t.Errorf("gate() result = %+v; want the zero GateResult alongside a returned error", result)
		}
	})

	// The darn gate adds the history check and the commit to the section check, so each case drives it over a told history and a counting commit.
	const addedEntry = "\n### Added after Discussion (parent, 2026-03-04): Keep the cache\n\nDecision: Keep the cache per run.\n\nRationale: A shared cache races.\n"
	const addedHeading = "### Added after Discussion (parent, 2026-03-04): Keep the cache"
	historyError := errors.New("history unavailable")
	commitError := errors.New("commit failed")

	darnTests := []struct {
		name          string
		record        string
		history       [][]byte
		historyErr    error
		commitErr     error
		wantPassed    bool
		wantFindings  string
		wantErr       error
		wantCommits   int
		wantHistoryOn bool
	}{
		{name: "a record whose history holds no added entry passes and commits once", record: validDecisionRecord, history: [][]byte{[]byte(validDecisionRecord)}, wantPassed: true, wantCommits: 1, wantHistoryOn: true},
		{name: "a record that keeps its added entry passes and commits once", record: validDecisionRecord + addedEntry, history: [][]byte{[]byte(validDecisionRecord + addedEntry)}, wantPassed: true, wantCommits: 1, wantHistoryOn: true},
		{name: "a dropped added entry fails naming its heading and commits nothing", record: validDecisionRecord, history: [][]byte{[]byte(validDecisionRecord + addedEntry)}, wantFindings: addedHeading, wantHistoryOn: true},
		// A `lym fabric commit` of the overwrite leaves the dropping copy as the newest version, so the check must read the whole history.
		{name: "a dropped entry fails when the newest version is the dropping copy", record: validDecisionRecord, history: [][]byte{[]byte(validDecisionRecord), []byte(validDecisionRecord + addedEntry)}, wantFindings: addedHeading, wantHistoryOn: true},
		{name: "a record missing a section never reads the history", record: strings.Replace(validDecisionRecord, "## Goal\n\nGoal text.\n\n", "", 1), history: [][]byte{[]byte(validDecisionRecord + addedEntry)}, wantFindings: "discussion-section-missing"},
		{name: "a history error comes back as the gate's error", record: validDecisionRecord, historyErr: historyError, wantErr: historyError, wantHistoryOn: true},
		{name: "a commit error comes back as the gate's error", record: validDecisionRecord, history: [][]byte{[]byte(validDecisionRecord)}, commitErr: commitError, wantErr: commitError, wantCommits: 1, wantHistoryOn: true},
	}
	for _, tt := range darnTests {
		t.Run("Darn/"+tt.name, func(t *testing.T) {
			decisionRecordPath, supportLogPath := writeDiscussionFixture(t, t.TempDir(), tt.record, "support log")
			historyCalls, commits := 0, 0
			history := func() ([][]byte, error) {
				historyCalls++
				return tt.history, tt.historyErr
			}
			commit := func() error {
				commits++
				return tt.commitErr
			}

			result, err := NewDarnDiscussionGate(decisionRecordPath, supportLogPath, history, commit)()
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("gate() error = %v; want it to wrap %v", err, tt.wantErr)
				}
				if tt.commitErr != nil && !strings.Contains(err.Error(), decisionRecordPath) {
					t.Errorf("gate() error = %q; want it to name the decision record path %q", err, decisionRecordPath)
				}
			} else if err != nil {
				t.Fatalf("gate() error = %v; want nil", err)
			}
			if result.Passed != tt.wantPassed {
				t.Errorf("gate() Passed = %v; want %v", result.Passed, tt.wantPassed)
			}
			if tt.wantFindings != "" && !strings.Contains(result.Findings, tt.wantFindings) {
				t.Errorf("gate() Findings = %q; want it to contain %q", result.Findings, tt.wantFindings)
			}
			if commits != tt.wantCommits {
				t.Errorf("commit calls = %d; want %d", commits, tt.wantCommits)
			}
			if (historyCalls > 0) != tt.wantHistoryOn {
				t.Errorf("history calls = %d; want history read = %v", historyCalls, tt.wantHistoryOn)
			}
		})
	}
}

func TestNewPlanGate(t *testing.T) {
	t.Run("Pass", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		seedPlanFormatFixture(t, anchorPath, false)

		gate := NewPlanGate(anchorPath, worktreeRoot, "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})
		result, err := gate()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if !result.Passed {
			t.Errorf("gate() Passed = %v; want true", result.Passed)
		}
	})

	t.Run("LensReviewGateRequiresARecordedLensReview", func(t *testing.T) {
		for _, tt := range []struct {
			name       string
			write      bool
			content    string
			wantPassed bool
		}{
			{name: "no lens review file"},
			{name: "empty file", write: true},
			{name: "whitespace-only file", write: true, content: " \n\t\n"},
			{name: "one line of text", write: true, content: "The lens review.\n", wantPassed: true},
		} {
			t.Run(tt.name, func(t *testing.T) {
				anchorPath := t.TempDir()
				seedPlanFormatFixture(t, anchorPath, false)
				lensReviewPath := filepath.Join(planparser.PlanDir(anchorPath), planparser.LensReviewFileName)
				if tt.write {
					if err := os.WriteFile(lensReviewPath, []byte(tt.content), 0o644); err != nil {
						t.Fatalf("write lens review: %v", err)
					}
				}

				gate := NewLensReviewPlanGate(anchorPath, t.TempDir(), "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})
				result, err := gate()
				if err != nil {
					t.Fatalf("gate() error = %v; want nil", err)
				}
				if result.Passed != tt.wantPassed {
					t.Fatalf("gate() Passed = %v; want %v (findings %q)", result.Passed, tt.wantPassed, result.Findings)
				}
				if tt.wantPassed {
					return
				}
				for _, want := range []string{"lens-review-missing", lensReviewPath} {
					if !strings.Contains(result.Findings, want) {
						t.Errorf("gate() Findings = %q; want it to contain %q", result.Findings, want)
					}
				}
			})
		}
	})

	t.Run("FindingsSurfaceAsAFailedGateAndAWarnLine", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := t.TempDir()
		seedFormatInvalidPlanFixture(t, anchorPath)

		buf := logcapture.Capture(t)
		gate := NewPlanGate(anchorPath, worktreeRoot, "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})
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

		gate := NewPlanGate(anchorPath, worktreeRoot, "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})
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

		told := t.TempDir()
		// A scratch directory that cannot be created: its parent is a regular file.
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
			t.Fatalf("write the blocking file: %v", err)
		}
		cases := []struct {
			name       string
			scratchDir string
			// wantFile is the path the log must carry and the file that must hold the finding; empty means the finding stays inline.
			wantFile string
		}{
			{name: "told directory records a file and logs its count and path", scratchDir: told, wantFile: filepath.Join(told, "Plan-Gate-informational-findings.txt")},
			{name: "empty directory keeps the findings inline", scratchDir: ""},
			{name: "unwritable directory falls back to the findings inline", scratchDir: filepath.Join(blocker, "scratch")},
		}
		for _, tc := range cases {
			buf := logcapture.Capture(t)
			gate := NewPlanGate(anchorPath, worktreeRoot, tc.scratchDir, planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})
			result, err := gate()
			if err != nil {
				t.Fatalf("%s: gate() error = %v; want nil", tc.name, err)
			}
			if !result.Passed {
				t.Fatalf("%s: gate() Passed = false; want true for an informational-only findings set", tc.name)
			}
			logged := buf.String()
			if tc.wantFile == "" {
				if !strings.Contains(logged, "create-new-unit") {
					t.Errorf("%s: log = %q; want it to carry the informational finding inline", tc.name, logged)
				}
				continue
			}
			if strings.Contains(logged, "create-new-unit") || !strings.Contains(logged, "count=1") || !strings.Contains(logged, tc.wantFile) {
				t.Errorf("%s: log = %q; want the count and the file path, and not the finding text", tc.name, logged)
			}
			recorded, err := os.ReadFile(tc.wantFile)
			if err != nil || !strings.Contains(string(recorded), "create-new-unit") {
				t.Errorf("%s: file %s = %q, %v; want it to hold the finding", tc.name, tc.wantFile, recorded, err)
			}
		}
	})

	t.Run("MixedBlockingAndInformationalFailsTheGate", func(t *testing.T) {
		anchorPath := t.TempDir()
		worktreeRoot := plankit.Repo(t, map[string]string{"sub/a.go": "package sub\n\nfunc Foo() {}\n"})
		// "newpkg#Bar" is informational (create-new-unit); "sub#Missing" resolves not_found with
		// unit: found, which statusFindings reports as the blocking glyph-not-found finding. One
		// blocking finding is enough to fail the gate.
		seedGlyphPlanFixture(t, anchorPath, true, "newpkg#Bar", "sub#Missing")

		gate := NewPlanGate(anchorPath, worktreeRoot, "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})
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

		result, err := NewPlanGate(anchorPath, worktreeRoot, "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})()
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

		result, err := NewPlanGate(anchorPath, worktreeRoot, "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})()
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

		result, err := NewPlanGate(anchorPath, worktreeRoot, "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})()
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

			result, err := NewPlanGate(anchorPath, t.TempDir(), "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})()
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

		findings, err := ValidatePlan(plan, anchorPath, t.TempDir(), planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})
		if err == nil || findings != nil {
			t.Errorf("ValidatePlan() = %v, %v; want a returned error and no findings", findings, err)
		}
	})

	// The context checks count their own check IDs, so the other findings of the fixture plan do not matter.
	const (
		symbolPair = "`sub#Foo` -> `plan:sub#Bar`"
		filePair   = "`old/a.go` -> `new/a.go`"
		bullet     = "- `PATTERN-cwd-resolution` — adds: the new text"
	)
	countCheck := func(findings []planindex.Finding, check string) int {
		n := 0
		for _, f := range findings {
			if f.Check == check {
				n++
			}
		}
		return n
	}
	// The PATTERN rows build overviews around the headroom line with overviewOf.
	const (
		entryB      = "- `PATTERN-b` — b."
		shortEntryA = "- `PATTERN-a` — short."
		sameSizeA   = "- `PATTERN-a` — other."
		removesA    = "- `PATTERN-a` — removes"
		removesB    = "- `PATTERN-b` — removes"
	)
	longEntryA := "- `PATTERN-a` — " + strings.Repeat("l", 200)
	bigEntryB := "- `PATTERN-b` — " + strings.Repeat("b", 150)
	underLine := overviewOf(pattern.HeadroomBytes-100, shortEntryA, entryB)
	// removableUnderLine grows past the line when longEntryA replaces shortEntryA, and falls back under it once bigEntryB is removed too.
	removableUnderLine := overviewOf(pattern.HeadroomBytes-100, shortEntryA, bigEntryB)
	const (
		namesB  = "Delete the `PATTERN-b` bullet from PATTERN.md."
		cardOne = "1-first-card"
		cardTwo = "2-first-card"
	)
	smallOverview := "# PATTERN\n\n- `PATTERN-other` — o.\n"
	const (
		headroomWayForward     = "lym loom decision add"
		removesWayForward      = "drop the fenced line"
		unprescribedWayForward = "lym loom insert-card"
	)

	for _, tt := range []struct {
		name                       string
		renamePair                 string
		planSection, recordSection string
		tools                      []string
		doneCard                   bool
		wantRename, wantMismatch   int
		// verify is the card's Verify line and overview the overview's retired verify section; both are set on the parsed plan.
		verify, overview string
		refuseRaw        []string
		wantRaw          int
		wantNoWait       int
		wantRetired      int
		// patternOverview is the worktree's PATTERN.md, absent when empty, and patternBase the overview the fixed PatternBase closure returns, or patternBaseErr the error it returns instead.
		// A row setting neither leaves PatternBase nil.
		patternOverview string
		patternBase     string
		patternBaseErr  bool
		// cardFence is fenced text on the sole card, secondFence the Text of a second card added to the parsed plan, and noRecordPath clears the context's decision record path.
		cardFence, secondFence string
		noRecordPath           bool
		// wantDetail is a fragment every finding of the PATTERN checks must hold, the way forward its message names.
		wantDetail                                  string
		wantHeadroom, wantRemoves, wantUnprescribed int
		// wantCardHeadroom counts the pattern-card-headroom findings, each of which must name the card wantCardHeadroomOn.
		wantCardHeadroom   int
		wantCardHeadroomOn string
		wantErr            bool
	}{
		{name: "PatternLinesGrowingPastTheLineFromABaseUnderIt", recordSection: "absent", patternOverview: underLine, patternBase: underLine, cardFence: fenceOf(longEntryA), wantHeadroom: 1, wantCardHeadroom: 1, wantCardHeadroomOn: cardOne, wantDetail: headroomWayForward},
		{name: "PatternLinesReplacingBulletsAtEqualSizePass", recordSection: "absent", patternOverview: underLine, patternBase: underLine, cardFence: fenceOf(sameSizeA)},
		{name: "PatternOverviewPastTheLineWithNoRemovesIsFlagged", recordSection: "absent", patternOverview: overviewOf(pattern.HeadroomBytes+20, shortEntryA, bigEntryB), patternBase: underLine, wantHeadroom: 1, wantDetail: headroomWayForward},
		{name: "PatternRemovesWithNoFencedLineShrinksTheProjection", planSection: removesB, recordSection: "absent", patternOverview: overviewOf(pattern.HeadroomBytes+20, shortEntryA, bigEntryB), patternBase: underLine},
		{name: "PatternRemovesFencedLineIsFlagged", planSection: removesA, recordSection: "absent", patternOverview: underLine, patternBase: underLine, cardFence: fenceOf(sameSizeA), wantRemoves: 1, wantDetail: removesWayForward},
		{name: "PatternRemovesFencedByADoneCardPasses", planSection: removesA, recordSection: "absent", doneCard: true, patternOverview: underLine, patternBase: underLine, cardFence: fenceOf(sameSizeA)},
		{name: "TwoCardsFencingOneIDWhereTheLaterFitsFlagsOnlyTheEarlierCard", recordSection: "absent", patternOverview: underLine, patternBase: underLine, cardFence: fenceOf(longEntryA), secondFence: fenceOf(sameSizeA), wantCardHeadroom: 1, wantCardHeadroomOn: cardOne, wantDetail: headroomWayForward},
		{name: "TwoCardsFencingOneIDWhereTheLaterOverflowsIsFlagged", recordSection: "absent", patternOverview: underLine, patternBase: underLine, cardFence: fenceOf(sameSizeA), secondFence: fenceOf(longEntryA), wantHeadroom: 1, wantCardHeadroom: 1, wantCardHeadroomOn: cardTwo, wantDetail: headroomWayForward},
		{name: "RemovalNamedByTheGrowingCardOffsetsItsGrowth", planSection: removesB, recordSection: "absent", patternOverview: removableUnderLine, patternBase: removableUnderLine, cardFence: namesB + "\n\n" + fenceOf(longEntryA)},
		{name: "RemovalNamedByTwoCardsIsCreditedToTheFirstNotTheLast", planSection: removesB, recordSection: "absent", patternOverview: removableUnderLine, patternBase: removableUnderLine, cardFence: namesB + "\n\n" + fenceOf(longEntryA), secondFence: namesB},
		{name: "RemovalNamedByALaterCardFlagsTheEarlierGrowingCard", planSection: removesB, recordSection: "absent", patternOverview: removableUnderLine, patternBase: removableUnderLine, cardFence: fenceOf(longEntryA), secondFence: namesB, wantCardHeadroom: 1, wantCardHeadroomOn: cardOne, wantDetail: headroomWayForward},
		{name: "RemovalNoCardNamesShrinksTheProjectionBeforeTheFirstCard", planSection: removesB, recordSection: "absent", patternOverview: removableUnderLine, patternBase: removableUnderLine, cardFence: fenceOf(longEntryA)},
		{name: "DoneCardIsNeverProjected", recordSection: "absent", doneCard: true, patternOverview: underLine, patternBase: overviewOf(pattern.HeadroomBytes+200, shortEntryA, entryB), cardFence: fenceOf(longEntryA)},
		{name: "TwoLinesOfOneCardWhereTheLaterFitsPass", recordSection: "absent", patternOverview: underLine, patternBase: underLine, cardFence: fenceOf(longEntryA, sameSizeA)},
		{name: "TwoLinesOfOneCardWhereTheLaterOverflowsIsFlagged", recordSection: "absent", patternOverview: underLine, patternBase: underLine, cardFence: fenceOf(sameSizeA, longEntryA), wantHeadroom: 1, wantCardHeadroom: 1, wantCardHeadroomOn: cardOne, wantDetail: headroomWayForward},
		{name: "OverviewAlreadyHoldingTheLinesProjectsTheBaseSize", recordSection: "absent", patternOverview: overviewOf(pattern.HeadroomBytes+50, longEntryA, entryB), patternBase: overviewOf(pattern.HeadroomBytes+50, longEntryA, entryB), cardFence: fenceOf(longEntryA)},
		{name: "DeclaredEntryWithNoFencedLineAndAnUnchangedBulletIsFlagged", planSection: bullet, recordSection: bullet, patternOverview: smallOverview, patternBase: smallOverview, wantUnprescribed: 1, wantDetail: unprescribedWayForward},
		{name: "DeclaredEntryWithAFencedLinePasses", planSection: bullet, recordSection: bullet, patternOverview: smallOverview, patternBase: smallOverview, cardFence: fenceOf("- `PATTERN-cwd-resolution` — the new text")},
		{name: "DeclaredEntryAnEarlierGenerationBuiltPasses", planSection: bullet, recordSection: bullet, patternOverview: smallOverview + "- `PATTERN-cwd-resolution` — built.\n", patternBase: smallOverview},
		{name: "DeclaredEntryWithNoDecisionRecordPathPasses", planSection: bullet, noRecordPath: true, patternOverview: smallOverview, patternBase: smallOverview},
		{name: "NoWorktreePatternSkipsTheBaseRead", recordSection: "absent", patternBaseErr: true, cardFence: fenceOf(longEntryA)},
		{name: "PatternBaseErrorIsReturned", recordSection: "absent", patternOverview: underLine, patternBaseErr: true, wantErr: true},
		{name: "RawTestVerifyIsRefused", recordSection: "absent", verify: "go test ./internal/x", refuseRaw: []string{"go test"}, wantRaw: 1},
		{name: "RawTestAfterAChainIsRefused", recordSection: "absent", verify: "cd x && go test ./...", refuseRaw: []string{"go test"}, wantRaw: 1},
		{name: "RawTestPastALauncherIsRefused", recordSection: "absent", verify: "env go test ./x", refuseRaw: []string{"go test"}, wantRaw: 1},
		{name: "RawTestPastAnEnvironmentAssignmentIsRefused", recordSection: "absent", verify: "GOFLAGS=-count=1 go test ./x", refuseRaw: []string{"go test"}, wantRaw: 1},
		{name: "RawTestWithNoRefuseListPasses", recordSection: "absent", verify: "go test ./internal/x"},
		{name: "RawTestInAnArgumentPasses", recordSection: "absent", verify: "echo go test", refuseRaw: []string{"go test"}},
		{name: "RawTestOfADoneCardIsNotReported", recordSection: "absent", doneCard: true, verify: "go test ./internal/x", refuseRaw: []string{"go test"}},
		{name: "GateCallWithNoWaitIsRefused", recordSection: "absent", verify: "lym gate test ./internal/x", refuseRaw: []string{"go test"}, wantNoWait: 1},
		{name: "GateCallWithWaitPasses", recordSection: "absent", verify: "lym gate test --wait --tier 0 --card 3", refuseRaw: []string{"go test"}},
		{name: "GateCallWithNoWaitOfADoneCardIsNotReported", recordSection: "absent", doneCard: true, verify: "lym gate test ./internal/x"},
		{name: "OverviewVerifySectionIsInformational", recordSection: "absent", overview: "go build ./...", refuseRaw: []string{"go test"}, wantRetired: 1},
		{name: "SymbolPairWithNoRenameToolIsFlagged", renamePair: symbolPair, recordSection: "absent", wantRename: 1},
		{name: "SymbolPairWithARenameToolPasses", renamePair: symbolPair, recordSection: "absent", tools: []string{"gopls rename"}},
		{name: "FilePairWithNoRenameToolPasses", renamePair: filePair, recordSection: "absent"},
		{name: "SymbolPairOfADoneCardPasses", renamePair: symbolPair, recordSection: "absent", doneCard: true},
		{name: "RecordSectionThePlanLacksIsFlagged", recordSection: bullet, wantMismatch: 1},
		{name: "PlanSectionTheRecordLacksIsFlagged", planSection: bullet, wantMismatch: 1},
		{name: "DifferingTextIsFlaggedOnBothSides", planSection: bullet, recordSection: "- `PATTERN-cwd-resolution` — adds: other text", wantMismatch: 2},
		{name: "DifferingActionIsFlaggedOnBothSides", planSection: "- `PATTERN-cwd-resolution` — changes: the new text", recordSection: bullet, wantMismatch: 2},
		{name: "PlanLineFittingNoBulletShapeIsFlagged", planSection: "not a bullet", wantMismatch: 1},
		{name: "MatchingSectionsPass", planSection: bullet, recordSection: bullet},
		{name: "TwoAbsentSectionsPass", recordSection: ""},
		{name: "RecordPathWithNoFilePasses", planSection: bullet, recordSection: "absent"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			anchorPath := t.TempDir()
			pc := seedPlanContextFixture(t, anchorPath, tt.renamePair, tt.planSection, tt.recordSection, tt.cardFence)
			pc.RenameTools = tt.tools
			pc.RefuseRaw = tt.refuseRaw
			if tt.noRecordPath {
				pc.DecisionRecordPath = ""
			}
			worktreeRoot := t.TempDir()
			if tt.patternOverview != "" {
				if err := os.WriteFile(pattern.File(worktreeRoot), []byte(tt.patternOverview), 0o644); err != nil {
					t.Fatalf("write PATTERN.md: %v", err)
				}
			}
			if tt.patternOverview != "" || tt.patternBaseErr {
				var base []byte
				if tt.patternBase != "" {
					base = []byte(tt.patternBase)
				}
				var baseErr error
				if tt.patternBaseErr {
					baseErr = errors.New("base unavailable")
				}
				pc.PatternBase = func() ([]byte, error) { return base, baseErr }
			}
			if tt.doneCard {
				saveState(t, anchorPath, doneBatchOne)
			}
			plan, err := planparser.ParsePlan(planparser.PlanDir(anchorPath))
			if err != nil {
				t.Fatalf("ParsePlan: %v", err)
			}
			plan.Cards[0].Verify = tt.verify
			plan.Verify = tt.overview
			if tt.secondFence != "" {
				second := plan.Cards[0]
				second.Number = 2
				second.Text = tt.secondFence
				plan.Cards = append(plan.Cards, second)
			}

			findings, err := ValidatePlan(plan, anchorPath, worktreeRoot, planglyph.NewIndex(fabricengine.NewReferenceRule()), pc)
			if tt.wantErr {
				if err == nil || findings != nil {
					t.Errorf("ValidatePlan() = %v, %v; want a returned error and no findings", findings, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidatePlan() error = %v; want nil", err)
			}
			for check, want := range map[string]int{patternHeadroomCheck: tt.wantHeadroom, patternCardHeadroomCheck: tt.wantCardHeadroom, patternRemovesPrescribedCheck: tt.wantRemoves, patternLineUnprescribedCheck: tt.wantUnprescribed} {
				if got := countCheck(findings, check); got != want {
					t.Errorf("ValidatePlan() %s findings = %d; want %d (%v)", check, got, want, findings)
				}
			}
			for _, f := range findings {
				isPatternCheck := f.Check == patternHeadroomCheck || f.Check == patternCardHeadroomCheck || f.Check == patternRemovesPrescribedCheck || f.Check == patternLineUnprescribedCheck
				if isPatternCheck && (f.Severity != planindex.SeverityBlocking || !strings.Contains(f.Detail, tt.wantDetail)) {
					t.Errorf("ValidatePlan() %s finding = %+v; want a blocking finding whose detail holds %q", f.Check, f, tt.wantDetail)
				}
				if f.Check == patternCardHeadroomCheck && f.Card != tt.wantCardHeadroomOn {
					t.Errorf("ValidatePlan() %s finding Card = %q; want %q", f.Check, f.Card, tt.wantCardHeadroomOn)
				}
			}
			if got := countCheck(findings, renameToolMissingCheck); got != tt.wantRename {
				t.Errorf("ValidatePlan() %s findings = %d; want %d (%v)", renameToolMissingCheck, got, tt.wantRename, findings)
			}
			if got := countCheck(findings, patternChangesMismatchCheck); got != tt.wantMismatch {
				t.Errorf("ValidatePlan() %s findings = %d; want %d (%v)", patternChangesMismatchCheck, got, tt.wantMismatch, findings)
			}
			for check, want := range map[string]int{verifyRawCommandCheck: tt.wantRaw, verifyGateNoWaitCheck: tt.wantNoWait, planVerifyRetiredCheck: tt.wantRetired} {
				if got := countCheck(findings, check); got != want {
					t.Errorf("ValidatePlan() %s findings = %d; want %d (%v)", check, got, want, findings)
				}
			}
			for _, f := range findings {
				if f.Check == planVerifyRetiredCheck && f.Severity != planindex.SeverityInformational {
					t.Errorf("ValidatePlan() %s severity = %v; want informational", f.Check, f.Severity)
				}
				if (f.Check == verifyRawCommandCheck || f.Check == verifyGateNoWaitCheck) && f.Severity != planindex.SeverityBlocking {
					t.Errorf("ValidatePlan() %s severity = %v; want blocking", f.Check, f.Severity)
				}
			}
		})
	}

	t.Run("MalformedRecordSectionIsAReturnedError", func(t *testing.T) {
		t.Parallel()
		anchorPath := t.TempDir()
		pc := seedPlanContextFixture(t, anchorPath, "", "", "not a bullet", "")
		plan, err := planparser.ParsePlan(planparser.PlanDir(anchorPath))
		if err != nil {
			t.Fatalf("ParsePlan: %v", err)
		}

		findings, err := ValidatePlan(plan, anchorPath, t.TempDir(), planglyph.NewIndex(fabricengine.NewReferenceRule()), pc)
		if err == nil || !strings.Contains(err.Error(), "lym loom validate-discussion") || findings != nil {
			t.Errorf("ValidatePlan() = %v, %v; want a returned error naming lym loom validate-discussion and no findings", findings, err)
		}
	})

	t.Run("LensReviewGateReportsRenameToolMissing", func(t *testing.T) {
		t.Parallel()
		anchorPath := t.TempDir()
		pc := seedPlanContextFixture(t, anchorPath, symbolPair, "", "absent", "")

		result, err := NewLensReviewPlanGate(anchorPath, t.TempDir(), "", planglyph.NewIndex(fabricengine.NewReferenceRule()), pc)()
		if err != nil {
			t.Fatalf("gate() error = %v; want nil", err)
		}
		if result.Passed || !strings.Contains(result.Findings, renameToolMissingCheck) {
			t.Errorf("gate() = %+v; want a failure naming %s", result, renameToolMissingCheck)
		}
	})
}

// writeOverviewOnlyPlanDir creates the plan directory under anchorPath and writes only overview
// (00-overview.md's content), leaving every card file absent -- the shape the ParsePlan
// per-card-read-fault fixtures below build on top of.
func writeOverviewOnlyPlanDir(t *testing.T, anchorPath, overview string) (planDir string) {
	t.Helper()
	planDir = filepath.Join(anchorPath, lymdirs.LymDirName, "plan")
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

		gate := NewPlanGate(anchorPath, worktreeRoot, "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})
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

		gate := NewPlanGate(anchorPath, worktreeRoot, "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})
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
		// No _lym/plan/00-overview.md at all: ParsePlan's os.IsNotExist branch returns a plain
		// fmt.Errorf, never wrapping the underlying *fs.PathError with %w -- so it is findings, the same
		// disposition discussionparser.Validate already gives a missing file.

		gate := NewPlanGate(anchorPath, worktreeRoot, "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})
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
		planDir := filepath.Join(anchorPath, lymdirs.LymDirName, "plan")
		if err := os.MkdirAll(planDir, 0o755); err != nil {
			t.Fatalf("mkdir plan dir: %v", err)
		}
		// A directory at 00-overview.md's own path forces os.ReadFile to fail with a non-not-exist
		// *fs.PathError, which ParsePlan wraps with %w -- the overview read fault.
		if err := os.MkdirAll(filepath.Join(planDir, "00-overview.md"), 0o755); err != nil {
			t.Fatalf("mkdir overview path: %v", err)
		}

		gate := NewPlanGate(anchorPath, worktreeRoot, "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})
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

		gate := NewPlanGate(anchorPath, worktreeRoot, "", planglyph.NewIndex(fabricengine.NewReferenceRule()), PlanContext{})
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
