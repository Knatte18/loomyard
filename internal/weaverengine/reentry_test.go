// reentry_test.go pins how a call of Run re-enters a stop:
// the stopped batch recorded done with no spawn, and the attempt or fixer on the last rung.
// It also pins the needs-parent step, the verify and a plan that no longer matches the recorded partition.

package weaverengine_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/planindex"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// stopFirstBatch runs fx until its first batch stops on the last rung, three failed attempts scripted.
func stopFirstBatch(t *testing.T, fx *runFixture) {
	t.Helper()
	failed := attemptScript{report: runplan.Report{Status: runplan.ReportStatusFailed, Journal: "tried"}}
	fx.runner.scripts = []attemptScript{failed, failed, failed}
	requireStuck(t, mustRun(t, fx), "batch 01-one")
}

// rewritePlan writes the fixture's plan again with the given card slugs, numbered from one.
func rewritePlan(t *testing.T, fx *runFixture, slugs ...string) {
	t.Helper()
	cards := make([]plankit.Card, len(slugs))
	for i, slug := range slugs {
		cards[i] = plankit.Card{Number: i + 1, Slug: slug, Groups: []plankit.Group{{Label: "Create", Targets: []string{"internal/x/" + slug + ".go"}}}}
	}
	plankit.Write(t, fx.deps.Geom.PlanDir, plankit.Plan{Approved: true, Language: "none", Framing: "A test plan.", Cards: cards})
}

func TestRun_Reentry(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("PassingStoppedBatchIsRecordedDoneWithNoSpawnAlsoAfterAPendingCardIsEdited", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one", "two")
		stopFirstBatch(t, fx)
		fx.git.commit("the parent repaired the batch")
		card, err := os.OpenFile(filepath.Join(fx.deps.Geom.PlanDir, "02-two.md"), os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := card.WriteString("\nAn edit the arbiter made.\n"); err != nil {
			t.Fatal(err)
		}
		_ = card.Close()
		fx.runner.scripts = append(fx.runner.scripts, attemptScript{commit: true})

		requireDone(t, mustRun(t, fx), 2)

		if got := len(fx.runner.specs); got != 4 {
			t.Errorf("strands started = %d; want the stopped batch's three and the next batch's one, none to re-enter", got)
		}
		st := fx.loadState()
		first := st.Batches[1]
		head, _ := fx.git.HeadSHA("")
		if !first.Terminal || first.Status != "done" || first.Digest == nil || first.Digest.HeadSHA == "" || st.Stop != nil {
			t.Errorf("batch 1 = %+v, Stop = %+v; want it recorded done with a digest and no stop", first, st.Stop)
		}
		if st.BatchEnds[1] == "" || st.BatchEnds[1] == head {
			t.Errorf("batch 1 end = %q, head = %q; want the end commit of the repaired batch", st.BatchEnds[1], head)
		}
		if got := len(fx.verify.calls); got != 3 {
			t.Errorf("verify calls = %d; want the batch's card and batch gate and the plan-level verify", got)
		}
	})

	t.Run("StoppedBatchWithADroppedCreateIsRecordedDoneWithNoSpawn", func(t *testing.T) {
		t.Parallel()
		const target = "internal/x/one.go"
		fx := newRunFixture(t, 1, "one")
		stopFirstBatch(t, fx)
		fx.git.commit("the parent repaired the batch")
		fx.index.notDone = []planindex.Finding{{Check: "create-not-done", Card: "1-one", Severity: planindex.SeverityBlocking, Detail: target + " does not resolve"}}
		fx.index.notDoneTarget = target
		record := "## Decisions\n\n### Added after Discussion (parent, 2026-11-21): Drop it\n\nDecision: drop it.\nRationale: unneeded.\nDrops Create: `" + target + "`\n"
		if err := os.WriteFile(fx.deps.Geom.DecisionRecordPath, []byte(record), 0o644); err != nil {
			t.Fatal(err)
		}

		requireDone(t, mustRun(t, fx), 1)

		if got := len(fx.runner.specs); got != 3 {
			t.Errorf("strands started = %d; want the stopped batch's three, none to re-enter", got)
		}
	})

	reentered := []struct {
		name string
		// setup shapes the fixture after the stop.
		setup func(fx *runFixture)
	}{
		{"DirtyTree", func(fx *runFixture) { fx.git.dirty = []string{"x.go"} }},
		{"FailingBatchGateAtHead", func(fx *runFixture) { fx.verify.scripts = []verifyScript{{failing: "TestBroken"}} }},
		{"RowWithGateEntries", func(fx *runFixture) {
			fx.deps.Gate = shuttleengine.GateSpec{{Name: "drift", Attempts: 1, Gate: func() (shuttleengine.GateResult, error) {
				return shuttleengine.GateResult{Passed: true}, nil
			}}}
		}},
	}
	for _, tt := range reentered {
		t.Run("NotRecordedDoneAndGetsALastRungAttempt_"+tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newRunFixture(t, 1, "one")
			stopFirstBatch(t, fx)
			tt.setup(fx)
			fx.runner.scripts = append(fx.runner.scripts, attemptScript{commit: true})

			requireDone(t, mustRun(t, fx), 1)

			if len(fx.runner.specs) != 4 || fx.runner.specs[3].Model != "model-c" {
				t.Fatalf("started = %d strand(s); want a fourth attempt on the last rung", len(fx.runner.specs))
			}
			st := fx.loadState()
			if !st.Attempts[3].Reentry || st.Attempts[3].Number != 4 || st.Stop != nil {
				t.Errorf("attempt 4 = reentry %v number %d, Stop = %+v; want the marked re-entry attempt and the stop cleared", st.Attempts[3].Reentry, st.Attempts[3].Number, st.Stop)
			}
		})
	}

	t.Run("EditToTheStoppedBatchsOwnCardRefreshesItsRecordedHash", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		stopFirstBatch(t, fx)
		before := fx.loadState().Batches[1].CardHashes["01-one"]
		fx.git.dirty = []string{"x.go"}
		cardPath := filepath.Join(fx.deps.Geom.PlanDir, "01-one.md")
		card, err := os.OpenFile(cardPath, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := card.WriteString("\nAn edit the parent made to the stopped card.\n"); err != nil {
			t.Fatal(err)
		}
		_ = card.Close()
		fx.runner.scripts = append(fx.runner.scripts, attemptScript{commit: true})

		requireDone(t, mustRun(t, fx), 1)

		sum := sha256.Sum256([]byte(readFile(t, cardPath)))
		after := fx.loadState().Batches[1].CardHashes["01-one"]
		if after == before || after != hex.EncodeToString(sum[:]) {
			t.Errorf("card hash = %q after the edit, %q before; want the hash of the edited card", after, before)
		}
	})

	t.Run("NeedsParentStartsOnTheAskingRungAndNeverRecordsDoneWithNoSpawn", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		fx.runner.scripts = []attemptScript{{report: runplan.Report{Status: runplan.ReportStatusNeedsParent, Need: "run lym fabric merge"}}, {commit: true}}
		requireStuck(t, mustRun(t, fx), "run lym fabric merge")

		requireDone(t, mustRun(t, fx), 1)

		if len(fx.runner.specs) != 2 || fx.runner.specs[1].Model != "model-a" {
			t.Fatalf("started = %d strand(s); want a second attempt on the asking rung", len(fx.runner.specs))
		}
		if !fx.loadState().Attempts[1].Reentry || len(fx.verify.calls) != 1 {
			t.Errorf("verify calls = %d; want the plan-level verify alone, with no re-entry gate run", len(fx.verify.calls))
		}
	})

	planEdits := []struct {
		name  string
		slugs []string
	}{
		{"LostARecordedCard", []string{"one"}},
		{"GainedAnUnrecordedCard", []string{"one", "two", "three"}},
	}
	for _, tt := range planEdits {
		t.Run("PlanThatChangedItsCardSetRefusesNamingBothWaysForward_"+tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newRunFixture(t, 1, "one", "two")
			stopFirstBatch(t, fx)
			rewritePlan(t, fx, tt.slugs...)
			if !slices.Contains(tt.slugs, "two") {
				if err := os.Remove(filepath.Join(fx.deps.Geom.PlanDir, "02-two.md")); err != nil {
					t.Fatal(err)
				}
			}

			requireStuck(t, mustRun(t, fx), "put the plan's card set back as the recorded partition holds it", "lym weaver reset", "re-step the row")

			if len(fx.runner.specs) != 3 {
				t.Errorf("strands started = %d; want no attempt over a plan that does not match", len(fx.runner.specs))
			}
		})
	}

	t.Run("LostLastRungAttemptsThenReentryRetriesOnceOnTheLastRung", func(t *testing.T) {
		t.Parallel()
		for _, secondLost := range []bool{false, true} {
			fx := newRunFixture(t, 1, "one")
			failed := attemptScript{report: runplan.Report{Status: runplan.ReportStatusFailed}}
			lost := attemptScript{dieAfterStart: true}
			fx.runner.scripts = []attemptScript{failed, failed, lost, lost}
			requireStuck(t, mustRun(t, fx), "batch 01-one")
			fx.git.dirty = []string{"x.go"}
			fx.runner.scripts = append(fx.runner.scripts, lost, attemptScript{commit: true})
			if secondLost {
				fx.runner.scripts[5] = lost
			}

			result := mustRun(t, fx)

			var models []string
			for _, spec := range fx.runner.specs {
				models = append(models, spec.Model)
			}
			if want := []string{"model-a", "model-b", "model-c", "model-c", "model-c", "model-c"}; !slices.Equal(models, want) {
				t.Errorf("models = %v; want the re-entry attempt and its one retry on the last rung %v", models, want)
			}
			st := fx.loadState()
			if !st.Attempts[4].Reentry || st.Attempts[5].Reentry || st.Attempts[5].Number != 6 {
				t.Errorf("attempts 5 and 6 = reentry %v and %v, number %d; want only the first marked", st.Attempts[4].Reentry, st.Attempts[5].Reentry, st.Attempts[5].Number)
			}
			if secondLost {
				requireStuck(t, result, "batch 01-one")
				continue
			}
			requireDone(t, result, 1)
		}
	})

	t.Run("VerifyReentryRunsTheVerifyAgainAndAFailureGetsOneLastRungFixer", func(t *testing.T) {
		t.Parallel()
		stopVerify := func(t *testing.T) *runFixture {
			fx := newRunFixture(t, 1, "one")
			for range 8 {
				fx.verify.scripts = append(fx.verify.scripts, verifyScript{failing: "TestBroken"})
			}
			requireStuck(t, mustRun(t, fx), "plan-level verify")
			fx.verify.scripts = fx.verify.scripts[:len(fx.verify.calls)]
			return fx
		}

		t.Run("FailsAgain", func(t *testing.T) {
			fx := stopVerify(t)
			for range 4 {
				fx.verify.scripts = append(fx.verify.scripts, verifyScript{failing: "TestBroken"})
			}

			requireStuck(t, mustRun(t, fx), "plan-level verify")

			if len(fx.fixer.specs) != 4 || fx.fixer.specs[3].ModelChoice.Model != "model-c" {
				t.Fatalf("fixers spawned = %d; want one more, on the last rung", len(fx.fixer.specs))
			}
			st := fx.loadState()
			if !st.Fixers[3].Reentry || st.Stop == nil || st.Stop.Kind != "verify" {
				t.Errorf("fixer 4 reentry = %v, Stop = %+v; want it marked and a new verify stop", st.Fixers[3].Reentry, st.Stop)
			}
		})

		t.Run("Passes", func(t *testing.T) {
			fx := stopVerify(t)

			requireDone(t, mustRun(t, fx), 1)

			if len(fx.fixer.specs) != 3 || fx.loadState().Stop != nil {
				t.Errorf("fixers spawned = %d, Stop = %+v; want no new fixer and the stop cleared", len(fx.fixer.specs), fx.loadState().Stop)
			}
		})
	})

	t.Run("LastBatchWhoseFinalRecordCommitFailsStartsNoSecondReentry", func(t *testing.T) {
		t.Parallel()
		fx := newRunFixture(t, 1, "one")
		stopFirstBatch(t, fx)
		fx.git.dirty = []string{"x.go"}
		fx.runner.scripts = append(fx.runner.scripts, attemptScript{commit: true})
		fx.commitErr = func(msg string) error {
			if msg == "weaver: run record" {
				return os.ErrPermission
			}
			return nil
		}

		requireStuck(t, mustRun(t, fx), "re-step the row")
		if st := fx.loadState(); st.Stop != nil || len(st.Attempts) != 4 {
			t.Fatalf("Stop = %+v, attempts = %d; want the stop cleared by the re-entry attempt's done", st.Stop, len(st.Attempts))
		}

		fx.commitErr = nil
		requireDone(t, mustRun(t, fx), 1)

		if len(fx.runner.specs) != 4 {
			t.Errorf("strands started = %d; want no second re-entry", len(fx.runner.specs))
		}
	})
}
