// bouncer_parent_test.go covers the Bouncer's ask to the run's parent orch before it judges a round.
// It covers the ask's line, the part of the judge's prompt that tells the judge to wait, and the ask's settle.

package shedadapters

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// bouncerParentSeedFocus is a round-1 focus file, whose write time is the review generation's seed.
const bouncerParentSeedFocus = "---\nround: 1\nexclude_lenses: []\nfocus: []\n---\n"

// TestBouncer_JudgeCall_AsksTheParentBeforeJudging table-drives the seats a Bouncer's judge call asks through.
// A seat that asks before the judge, with its key on, mails the parent the round's own review and fixer report.
// It tells the judge to wait for the answer, and settles the ask once the judge has ruled.
// A key off, or a seat that asks at round start, asks nothing and leaves the judge's prompt as it was.
func TestBouncer_JudgeCall_AsksTheParentBeforeJudging(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name           string
		keyOn          bool
		askBeforeJudge bool
		wantAsk        bool
		// answerFindings has the parent answer with findings while the judge runs, which the judge's CONVERGED ignores.
		answerFindings bool
	}{
		{name: "a seat asking before the judge with the key on asks and tells the judge to wait", keyOn: true, askBeforeJudge: true, wantAsk: true},
		{name: "the key off asks nothing", askBeforeJudge: true},
		{name: "a seat asking at round start asks nothing here", keyOn: true},
		{name: "a parent report with findings refuses a CONVERGED verdict", keyOn: true, askBeforeJudge: true, wantAsk: true, answerFindings: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			shuttle := judgeFakeShuttle(1, bouncerVerdictContent("CONVERGED"), bouncerLedgerContent(1), true)
			fixture := newBouncerFixture(t, withShuttle(shuttle))
			files := burlerengine.ParentReviewFilesOf(t.TempDir())
			var posts []parentPost
			fixture.Config.Parent = &ParentReviewSeat{
				Files:          files,
				Slug:           parentSlug,
				Tag:            CodeParentAskLineTag,
				Subject:        "the task's code change",
				AskBeforeJudge: tt.askBeforeJudge,
				AskEnabled:     func() bool { return tt.keyOn },
				Wait:           func() (time.Duration, string) { return parentWait, "" },
				Resolve:        func() (string, bool, error) { return resolvedStrand, true, nil },
				Post: func(strand, line string) error {
					posts = append(posts, parentPost{strand: strand, line: line})
					return nil
				},
			}
			b, cfg := fixture.Build()
			layoutBouncerRun(t, cfg, []bouncerJudgeFixture{{round: 1, report: bouncerReport(1), focus: bouncerParentSeedFocus}})

			wantOutcome, wantState := shedengine.Done, burlerengine.ParentAskTimeout
			if tt.answerFindings {
				wantOutcome, wantState = shedengine.Stuck, burlerengine.ParentAskDone
				inner := shuttle.DuringRun
				shuttle.DuringRun = func() {
					findings := []burlerengine.ParentFinding{{Change: "tighten the seam"}}
					if _, err := burlerengine.AnswerParentAsk(files, 1, bouncerJudgeTestClock, findings); err != nil {
						t.Errorf("AnswerParentAsk() error = %v; want nil", err)
					}
					inner()
				}
			}

			shedfake.RequireOutcome(t, b, wantOutcome)

			record, _, err := burlerengine.ReadParentAsks(files)
			if err != nil {
				t.Fatalf("ReadParentAsks() error = %v; want nil", err)
			}
			gotBlock := strings.Contains(shuttle.GotSpec.Prompt, "## The parent orch's review")
			if !tt.wantAsk {
				if gotBlock || len(posts) != 0 || len(record.Asks) != 0 {
					t.Errorf("prompt holds the parent block %v, posts %+v, asks %+v; want none of them", gotBlock, posts, record.Asks)
				}
				return
			}
			seed, err := roundSeed(cfg.RunDir)
			if err != nil {
				t.Fatalf("roundSeed() error = %v; want nil", err)
			}
			ask, found := record.RoundAsk(seed, 1)
			if !found || ask.Tag != CodeParentAskLineTag || ask.State != wantState {
				t.Fatalf("round 1 ask = %+v, found %v; want a code-review ask in state %s once the judge ruled", ask, found, wantState)
			}
			if _, err := os.Stat(verdictPath(cfg.RunDir, 1)); tt.answerFindings == (err == nil) {
				t.Errorf("verdict file stat error = %v; want the verdict kept unless the parent's findings refused it", err)
			}
			subject := fmt.Sprintf("the task's code change, with round 1's review %s and fixer report %s", roundReviewPath(cfg.RunDir, 1), roundFixerReportPath(cfg.RunDir, 1))
			want := []parentPost{{strand: resolvedStrand, line: ParentAskLine(CodeParentAskLineTag, parentSlug, subject, ask)}}
			if len(posts) != 1 || posts[0] != want[0] {
				t.Errorf("posts = %+v; want %+v", posts, want)
			}
			if !gotBlock || !strings.Contains(shuttle.GotSpec.Prompt, "lym loom parent-review await") {
				t.Errorf("judge prompt lacks the parent block telling the judge to await the answer")
			}
		})
	}
}

// TestBouncer_Call_RefusesARecoveredConvergedOverParentFindings pins the refusal of a recovered verdict.
// The verdict is CONVERGED and already on disk at Call entry, with the subject unchanged since it was judged.
// It is refused when the round's parent ask holds findings.
// The refusal archives the verdict, spawns no judge, and approves nothing.
//
//testtiming:keep pins the recovered CONVERGED verdict's refusal over parent findings: its reason, the verdict's archive, and that no judge or commit runs
func TestBouncer_Call_RefusesARecoveredConvergedOverParentFindings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	files := burlerengine.ParentReviewFilesOf(t.TempDir())
	h := newReentry(t, convergingShuttle(nil), func(_ *reentry, cfg *BouncerConfig) {
		cfg.Parent = &ParentReviewSeat{
			Files:          files,
			Slug:           parentSlug,
			Tag:            CodeParentAskLineTag,
			Subject:        "the task's code change",
			AskBeforeJudge: true,
			AskEnabled:     func() bool { return true },
			Wait:           func() (time.Duration, string) { return parentWait, "" },
			Resolve:        func() (string, bool, error) { return resolvedStrand, true, nil },
			Post:           func(string, string) error { return nil },
		}
	})
	layoutBouncerRun(t, h.cfg, []bouncerJudgeFixture{{
		round: 1, report: bouncerReport(1), verdict: bouncerVerdictContent("CONVERGED"), ledger: bouncerLedgerContent(1), focus: bouncerParentSeedFocus,
	}})
	if err := writeGenerationRecord(h.cfg.RunDir, generationRecord{Number: 1, JudgeSubject: h.subject}); err != nil {
		t.Fatalf("writeGenerationRecord() = %v; want nil", err)
	}
	seed, err := roundSeed(h.cfg.RunDir)
	if err != nil {
		t.Fatalf("roundSeed() error = %v; want nil", err)
	}
	asked, err := burlerengine.RecordParentAsk(files, burlerengine.ParentAsk{
		Round: 1, Seed: seed, Tag: CodeParentAskLineTag, AskedAt: bouncerJudgeTestClock,
		State: burlerengine.ParentAskOpen, Strand: resolvedStrand, Deadline: bouncerJudgeTestClock.Add(parentWait),
	})
	if err != nil {
		t.Fatalf("RecordParentAsk() error = %v; want nil", err)
	}
	findings := []burlerengine.ParentFinding{{Change: "tighten the seam"}}
	if _, err := burlerengine.AnswerParentAsk(files, asked.Number, bouncerJudgeTestClock, findings); err != nil {
		t.Fatalf("AnswerParentAsk() error = %v; want nil", err)
	}

	ptr := shedfake.RequireOutcome(t, h.b, shedengine.Stuck)

	if !strings.Contains(ptr.Reason, "holds findings, which rule out CONVERGED") {
		t.Errorf("Call() reason = %q; want the parent-findings refusal", ptr.Reason)
	}
	if _, err := os.Stat(verdictPath(h.cfg.RunDir, 1)); !os.IsNotExist(err) {
		t.Errorf("verdict file stat error = %v; want the refused verdict archived", err)
	}
	if got := countCalls(h.log, "commit"); got != 0 {
		t.Errorf("Commit ran %d times; want none over the parent's findings", got)
	}
	if got := len(h.shuttle.Specs); got != 0 {
		t.Errorf("shuttle runs = %d; want none, the refusal only archives", got)
	}
}
