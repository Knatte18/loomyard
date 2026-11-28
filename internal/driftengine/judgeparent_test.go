// judgeparent_test.go — tier 0 tests for the judge's ask to the run's parent orch:
// the parent agreement gate entry over a real ask record in a temp directory, the ask's line and the rulings summary.

package driftengine

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

const (
	judgeParentSlug   = "my-slug"
	judgeParentStrand = "orch-strand"
	judgeParentWait   = 30 * time.Minute
	// judgeParentJudgement is a judgement.yaml that parses against judgeParentWant.
	judgeParentJudgement = "spawn_id: spawn-1\ncells:\n  - {id: c1, ruling: matches, reason: fits}\n  - {id: c2, ruling: \"does not match\", reason: off by one}\n"
	// judgeParentSummary is judgeParentJudgement's rulings as the ask's line summarizes them.
	judgeParentSummary = "2 cells, 1 matches and 1 does not match (c2)"
	// otherRulings is the digest of rulings other than judgeParentJudgement's.
	otherRulings = "digest-of-other-rulings"
)

var (
	judgeParentWant = JudgementWant{Mode: DiffLines, SpawnID: "spawn-1", CellIDs: []string{"c1", "c2"}}
	// judgeParentNow is the clock every gate evaluation reads.
	judgeParentNow = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
)

// judgeParentPost is one line the fixture's Post received.
type judgeParentPost struct {
	strand string
	line   string
}

// TestParentAgreementGate table-drives the parent agreement entry over the ask the spawn holds before the evaluation.
// Each row reads whether the entry passed, what its findings name, the asks recorded and what was posted.
func TestParentAgreementGate(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	findings := []burlerengine.ParentFinding{{Change: "c2 matches: the expectation names the shift"}}
	tests := []struct {
		name string
		// prior is the spawn's ask recorded before the evaluation, nil for none; a done ask is answered with findings.
		// It presents the rulings of judgeParentJudgement unless it names a digest of its own.
		prior    *burlerengine.ParentAsk
		findings []burlerengine.ParentFinding
		// judgementText is the judgement file's text, judgeParentJudgement when unset.
		judgementText string
		// limited sets the entry's limit to limit past judgeParentNow; unset, the entry has no limit.
		limited bool
		limit   time.Duration
		// answerOnNow answers the held ask with findings during the entry's first read of the clock.
		answerOnNow bool
		unrecorded  bool
		resolveErr  error
		postErr     error
		wantPassed  bool
		// wantFindings and wantReason are contained in the result's findings and the latest ask's reason.
		wantFindings string
		wantReason   string
		// wantState is the latest ask's state, and wantFirst the first ask's state when set.
		wantState burlerengine.ParentAskState
		wantFirst burlerengine.ParentAskState
		// wantAsks is the number of recorded asks, one when unset.
		wantAsks  int
		wantPosts int
		// wantRound is the latest ask's round, unchecked when unset.
		wantRound int
		// wantDeadline is the posted ask's deadline past judgeParentNow, judgeParentWait when unset.
		wantDeadline time.Duration
	}{
		{
			name: "a spawn with no ask mails the rulings and holds the judge", wantFindings: "lym loom parent-review await my-slug",
			wantState: burlerengine.ParentAskOpen, wantPosts: 1,
		},
		{
			name:  "an open ask before its deadline holds the judge without a new post",
			prior: &burlerengine.ParentAsk{State: burlerengine.ParentAskOpen, Deadline: judgeParentNow.Add(time.Minute)}, wantFindings: "lym loom parent-review await my-slug",
			wantState: burlerengine.ParentAskOpen,
		},
		{
			name:  "an open ask at its deadline is set timeout and passes",
			prior: &burlerengine.ParentAsk{State: burlerengine.ParentAskOpen, Deadline: judgeParentNow}, wantPassed: true,
			wantState: burlerengine.ParentAskTimeout,
		},
		{
			name:  "a satisfied answer to the current rulings passes",
			prior: &burlerengine.ParentAsk{State: burlerengine.ParentAskDone, Deadline: judgeParentNow.Add(time.Minute)}, wantPassed: true,
			wantState: burlerengine.ParentAskDone,
		},
		{
			name:  "a judgement rewritten without changing a ruling keeps a satisfied answer",
			prior: &burlerengine.ParentAsk{State: burlerengine.ParentAskDone, Deadline: judgeParentNow.Add(time.Minute)}, judgementText: "# rewritten\n" + judgeParentJudgement, wantPassed: true,
			wantState: burlerengine.ParentAskDone,
		},
		{
			name:  "an answer with findings holds a judgement that still holds the rulings it answered",
			prior: &burlerengine.ParentAsk{State: burlerengine.ParentAskDone, Deadline: judgeParentNow.Add(time.Minute)}, findings: findings,
			wantFindings: "still holds the rulings it answered", wantState: burlerengine.ParentAskDone,
		},
		{
			name:         "a satisfied answer to other rulings sends a new ask naming the current ones",
			prior:        &burlerengine.ParentAsk{State: burlerengine.ParentAskDone, Digest: otherRulings, Deadline: judgeParentNow.Add(time.Minute)},
			wantFindings: "lym loom parent-review await my-slug", wantState: burlerengine.ParentAskOpen, wantFirst: burlerengine.ParentAskDone, wantAsks: 2, wantPosts: 1,
		},
		{
			name:  "a judgement rewritten after an answer with findings sends a new ask instead of passing",
			prior: &burlerengine.ParentAsk{State: burlerengine.ParentAskDone, Digest: otherRulings, Deadline: judgeParentNow.Add(time.Minute)}, findings: findings,
			wantFindings: "lym loom parent-review await my-slug", wantState: burlerengine.ParentAskOpen, wantFirst: burlerengine.ParentAskDone, wantAsks: 2, wantPosts: 1,
		},
		{
			name:         "an open ask on other rulings is set timeout and a new ask goes out",
			prior:        &burlerengine.ParentAsk{State: burlerengine.ParentAskOpen, Digest: otherRulings, Deadline: judgeParentNow.Add(time.Minute)},
			wantFindings: "lym loom parent-review await my-slug", wantState: burlerengine.ParentAskOpen, wantFirst: burlerengine.ParentAskTimeout, wantAsks: 2, wantPosts: 1,
		},
		{
			name:  "an ask still pending-post is posted once to its recorded strand and left open",
			prior: &burlerengine.ParentAsk{State: burlerengine.ParentAskPendingPost, Deadline: judgeParentNow.Add(time.Minute)}, wantFindings: "lym loom parent-review await my-slug",
			wantState: burlerengine.ParentAskOpen, wantPosts: 1, wantDeadline: time.Minute,
		},
		{
			name:  "an answer landing before the timeout is read as the answer",
			prior: &burlerengine.ParentAsk{State: burlerengine.ParentAskOpen, Deadline: judgeParentNow}, answerOnNow: true,
			wantFindings: "still holds the rulings it answered", wantState: burlerengine.ParentAskDone,
		},
		{
			name: "a wait past the judge's own limit is capped to it", limited: true, limit: 10 * time.Minute,
			wantFindings: "lym loom parent-review await my-slug", wantState: burlerengine.ParentAskOpen, wantPosts: 1, wantDeadline: 10 * time.Minute,
		},
		{
			name: "a judge with no time left records the ask not-sent and passes", limited: true, limit: -time.Minute,
			wantPassed: true, wantState: burlerengine.ParentAskNotSent, wantReason: "leaves the parent no time",
		},
		{name: "no recorded orch strand records no-parent and passes", unrecorded: true, wantPassed: true, wantState: burlerengine.ParentAskNoParent},
		{name: "a resolve error records no-parent, posts nothing and passes", resolveErr: errors.New("boom"), wantPassed: true, wantState: burlerengine.ParentAskNoParent, wantReason: "could not be resolved: boom"},
		{name: "a failed post records undelivered and passes", postErr: errors.New("boom"), wantPassed: true, wantState: "undelivered", wantReason: "the request was not delivered: boom", wantPosts: 1},
		{
			name:         "a held ask ended timeout and cleared is no ask, so a new one of the next round goes out",
			prior:        &burlerengine.ParentAsk{State: burlerengine.ParentAskTimeout, Cleared: true, Deadline: judgeParentNow.Add(-time.Minute)},
			wantFindings: "lym loom parent-review await my-slug", wantState: burlerengine.ParentAskOpen, wantFirst: burlerengine.ParentAskTimeout, wantAsks: 2, wantPosts: 1, wantRound: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files := burlerengine.ParentReviewFilesOf(t.TempDir())
			judgementPath := filepath.Join(t.TempDir(), "judgement.yaml")
			if err := os.WriteFile(judgementPath, []byte(cmp.Or(tt.judgementText, judgeParentJudgement)), 0o644); err != nil {
				t.Fatalf("write the judgement: %v", err)
			}
			if tt.prior != nil {
				recordJudgeAsk(t, files, *tt.prior, tt.findings)
			}
			var posts []judgeParentPost
			seat := &JudgeParentSeat{
				Files: files,
				Slug:  judgeParentSlug,
				Resolve: func() (string, bool, error) {
					return judgeParentStrand, !tt.unrecorded, tt.resolveErr
				},
				Post: func(strand, line string) error {
					posts = append(posts, judgeParentPost{strand: strand, line: line})
					return tt.postErr
				},
			}
			clockReads := 0
			now := func() time.Time {
				clockReads++
				if tt.answerOnNow && clockReads == 1 {
					if _, err := burlerengine.AnswerParentAsk(files, 1, judgeParentNow.Add(-time.Minute), findings); err != nil {
						t.Fatalf("AnswerParentAsk() error = %v; want nil", err)
					}
				}
				return judgeParentNow
			}
			var limit time.Time
			if tt.limited {
				limit = judgeParentNow.Add(tt.limit)
			}
			entry := ParentAgreementGateEntry(seat, judgeParentWant, judgementPath, judgeParentWait, limit, now)

			result, err := entry.Gate()

			if err != nil {
				t.Fatalf("Gate() error = %v; want nil", err)
			}
			if result.Passed != tt.wantPassed || !strings.Contains(result.Findings, tt.wantFindings) || (tt.wantPassed && result.Findings != "") {
				t.Errorf("Gate() = %+v; want passed %v with findings naming %q", result, tt.wantPassed, tt.wantFindings)
			}
			record, _, err := burlerengine.ReadParentAsks(files)
			if err != nil {
				t.Fatalf("ReadParentAsks() error = %v; want nil", err)
			}
			ask, found := record.LatestOfSeed(judgeAskSeed("spawn-1"))
			wantAsks := cmp.Or(tt.wantAsks, 1)
			if !found || ask.State != tt.wantState || len(record.Asks) != wantAsks || !strings.Contains(ask.Reason, tt.wantReason) {
				t.Errorf("latest ask = %+v, found %v, %d asks; want the latest in state %s, reason naming %q, among %d asks", ask, found, len(record.Asks), tt.wantState, tt.wantReason, wantAsks)
			}
			if tt.wantFirst != "" && record.Asks[0].State != tt.wantFirst {
				t.Errorf("first ask state = %s; want %s", record.Asks[0].State, tt.wantFirst)
			}
			if tt.wantRound != 0 && ask.Round != tt.wantRound {
				t.Errorf("latest ask round = %d; want %d", ask.Round, tt.wantRound)
			}
			if len(posts) != tt.wantPosts {
				t.Fatalf("posts = %+v; want %d", posts, tt.wantPosts)
			}
			if tt.wantPosts == 1 {
				wantAsk := burlerengine.ParentAsk{Number: ask.Number, Deadline: judgeParentNow.Add(cmp.Or(tt.wantDeadline, judgeParentWait))}
				want := []judgeParentPost{{strand: judgeParentStrand, line: JudgeAskLine(judgeParentSlug, judgementPath, judgeParentSummary, wantAsk)}}
				if !reflect.DeepEqual(posts, want) {
					t.Errorf("posts = %+v; want %+v", posts, want)
				}
			}
		})
	}
}

// judgeParentDigest returns the digest of the rulings of judgeParentJudgement.
func judgeParentDigest(t *testing.T) string {
	t.Helper()
	judgement, err := ParseJudgement([]byte(judgeParentJudgement), judgeParentWant)
	if err != nil {
		t.Fatalf("ParseJudgement() error = %v; want nil", err)
	}
	digest, err := rulingsDigest(judgement)
	if err != nil {
		t.Fatalf("rulingsDigest() error = %v; want nil", err)
	}
	return digest
}

// recordJudgeAsk records prior as spawn-1's ask with a strand and an asked-at time, answering it when its state is done.
// An ask that names no digest presents the rulings of judgeParentJudgement.
func recordJudgeAsk(t *testing.T, files burlerengine.ParentReviewFiles, prior burlerengine.ParentAsk, findings []burlerengine.ParentFinding) {
	t.Helper()
	answered := prior.State == burlerengine.ParentAskDone
	prior.Seed, prior.Round, prior.Tag, prior.AskedAt = judgeAskSeed("spawn-1"), judgeAskRound, JudgeAskLineTag, judgeParentNow.Add(-20*time.Minute)
	if prior.Strand == "" {
		prior.Strand = judgeParentStrand
	}
	if prior.Digest == "" {
		prior.Digest = judgeParentDigest(t)
	}
	if answered {
		prior.State = burlerengine.ParentAskOpen
	}
	recorded, err := burlerengine.RecordParentAsk(files, prior)
	if err != nil {
		t.Fatalf("RecordParentAsk() error = %v; want nil", err)
	}
	if answered {
		if _, err := burlerengine.AnswerParentAsk(files, recorded.Number, judgeParentNow.Add(-time.Minute), findings); err != nil {
			t.Fatalf("AnswerParentAsk() error = %v; want nil", err)
		}
	}
}

// TestDriftVerify_ParentAgreementCarriedWhileAsking pins when the judge's table carries the parent agreement entry.
// It does under drift.judge_ask_parent, and with the key off once the spawn holds an ask;
// it does not with the key off and no ask, nor with no parent seat told.
// The agreement entry follows the judgement entry in the table.
func TestDriftVerify_ParentAgreementCarriedWhileAsking(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name   string
		noSeat bool
		keyOn  bool
		held   bool
		want   bool
	}{
		{name: "the key on carries it", keyOn: true, want: true},
		{name: "the key off with a held ask carries it", held: true, want: true},
		{name: "the key off with no ask carries none"},
		{name: "no parent seat carries none", noSeat: true, keyOn: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			files := burlerengine.ParentReviewFilesOf(t.TempDir())
			if tt.held {
				recordJudgeAsk(t, files, burlerengine.ParentAsk{State: burlerengine.ParentAskOpen, Deadline: judgeParentNow.Add(time.Minute)}, nil)
			}
			deps := Deps{
				Config:     func() (Config, error) { return Config{JudgeAskParent: tt.keyOn, JudgeParentTimeoutMin: 30}, nil },
				Now:        func() time.Time { return judgeParentNow },
				JudgeTable: func(string) (seatengine.Table, error) { return seatengine.Table{}, nil },
			}
			if !tt.noSeat {
				deps.JudgeParent = &JudgeParentSeat{Files: files, Slug: judgeParentSlug}
			}
			verify := NewDriftVerify("Drift-Verify", Row{}, shuttleengine.GateEntry{Name: "judgement"}, deps)
			request := JudgeRequest{Mode: DiffLines, SpawnID: "spawn-1"}

			entry, carried, err := verify.parentAgreement(request)

			if err != nil {
				t.Fatalf("parentAgreement() error = %v; want nil", err)
			}
			if carried != tt.want || (entry.Name == ParentAgreementEntryName) != tt.want {
				t.Errorf("parentAgreement() = entry %q, carried %v; want carried %v", entry.Name, carried, tt.want)
			}
			table, err := verify.tableWithParent(request)
			if err != nil {
				t.Fatalf("tableWithParent() error = %v; want nil", err)
			}
			var gotNames []string
			for _, gateEntry := range table.Gate {
				gotNames = append(gotNames, gateEntry.Name)
			}
			wantNames := []string{"judgement"}
			if tt.want {
				wantNames = append(wantNames, ParentAgreementEntryName)
			}
			if !slices.Equal(gotNames, wantNames) {
				t.Errorf("tableWithParent() gate entries = %v; want %v", gotNames, wantNames)
			}
		})
	}
}

// TestJudgeAskLimit pins a judge ask's latest deadline: the timeout less the margin, and none before a judge run.
func TestJudgeAskLimit(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name   string
		opened time.Time
		want   time.Time
	}{
		{name: "a judge run that has not started has no limit"},
		{name: "a started judge run ends its asks before its timeout", opened: judgeParentNow, want: judgeParentNow.Add(judgeTimeout - judgeParentMargin)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := judgeAskLimit(tt.opened); !got.Equal(tt.want) {
				t.Errorf("judgeAskLimit(%v) = %v; want %v", tt.opened, got, tt.want)
			}
		})
	}
}

// errJudgeRunEnded is the error the capturing seat runner ends a fresh run with.
var errJudgeRunEnded = errors.New("the judge run ended")

// tableCapturingSeats is a seat runner that finds no live chair and hands the table of a fresh run to onRun.
// Its run then ends with errJudgeRunEnded.
type tableCapturingSeats struct {
	onRun func(table seatengine.Table)
}

func (s tableCapturingSeats) Probe(seatengine.Table) (seatengine.LiveTable, error) {
	return seatengine.LiveTable{}, nil
}

func (s tableCapturingSeats) Resume(seatengine.Table, seatengine.LiveTable) (seatengine.Result, error) {
	return seatengine.Result{}, errors.New("no chair is live, so none is resumed")
}

func (s tableCapturingSeats) Run(table seatengine.Table) (seatengine.Result, error) {
	s.onRun(table)
	return seatengine.Result{}, errJudgeRunEnded
}

// TestDriftVerify_RunAgentCapsTheParentAskByTheRunsStart runs the judge through runAgent, which both diff modes take.
// The parent ask that the table's agreement entry sends is capped by the run's start.
// Its deadline is that start plus the judge's timeout less the margin.
// drift.judge_parent_timeout_min alone would run past it.
func TestDriftVerify_RunAgentCapsTheParentAskByTheRunsStart(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	files := burlerengine.ParentReviewFilesOf(t.TempDir())
	judgementPath := filepath.Join(t.TempDir(), "judgement.yaml")
	if err := os.WriteFile(judgementPath, []byte(judgeParentJudgement), 0o644); err != nil {
		t.Fatalf("write the judgement: %v", err)
	}
	seats := tableCapturingSeats{onRun: func(table seatengine.Table) {
		for _, entry := range table.Gate {
			if entry.Name != ParentAgreementEntryName {
				continue
			}
			if _, err := entry.Gate(); err != nil {
				t.Errorf("Gate() error = %v; want nil", err)
			}
		}
	}}
	deps := Deps{
		// A wait of two judge timeouts, so the cap is what ends the ask.
		Config: func() (Config, error) {
			return Config{JudgeAskParent: true, JudgeParentTimeoutMin: int(2 * judgeTimeout / time.Minute)}, nil
		},
		Now:        func() time.Time { return judgeParentNow },
		Seats:      seats,
		JudgeTable: func(string) (seatengine.Table, error) { return seatengine.Table{}, nil },
		Paths:      Paths{Judgement: judgementPath},
		JudgeParent: &JudgeParentSeat{
			Files:   files,
			Slug:    judgeParentSlug,
			Resolve: func() (string, bool, error) { return judgeParentStrand, true, nil },
			Post:    func(string, string) error { return nil },
		},
	}
	verify := NewDriftVerify("Drift-Verify", Row{}, shuttleengine.GateEntry{Name: "judgement"}, deps)
	request := JudgeRequest{Mode: DiffLines, SpawnID: "spawn-1", Cells: []JudgeCell{{ID: "c1"}, {ID: "c2"}}}

	_, _, _, err := verify.runAgent(context.Background(), request)

	if !errors.Is(err, errJudgeRunEnded) {
		t.Fatalf("runAgent() error = %v; want the run's end", err)
	}
	record, _, err := burlerengine.ReadParentAsks(files)
	if err != nil {
		t.Fatalf("ReadParentAsks() error = %v; want nil", err)
	}
	ask, found := record.LatestOfSeed(judgeAskSeed("spawn-1"))
	want := judgeParentNow.Add(judgeTimeout - judgeParentMargin)
	if !found || !ask.Deadline.Equal(want) {
		t.Errorf("the spawn's ask = %+v, found %v; want its deadline at %v", ask, found, want)
	}
}

// TestJudgeAskLine pins the line's exact text: the tag, the slug, the ask number, the judgement file and the summary.
// It also pins the deadline as a local wall-clock time and the answer verb with its --ask flag in both forms.
//
//testtiming:keep pins the drift-judge ask line's literal text, which TestParentAgreementGate builds through JudgeAskLine
func TestJudgeAskLine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	deadline := time.Date(2026, 11, 19, 15, 33, 26, 0, time.UTC)

	got := JudgeAskLine("my-slug", "/wt/_lym/drift/judgement.yaml", judgeParentSummary, burlerengine.ParentAsk{Number: 3, Deadline: deadline})

	answer := "lym loom parent-review answer my-slug --ask 3"
	want := fmt.Sprintf(`[drift-judge request] my-slug ask 3: the drift judge proposes these rulings, each with its reason in /wt/_lym/drift/judgement.yaml: 2 cells, 1 matches and 1 does not match (c2). Answer by %s with "%s --satisfied" to agree, or "%s --findings-file <path>" whose findings each name a cell, case or file id and the ruling it should have`,
		deadline.Local().Format("2006-01-02 15:04:05 MST"), answer, answer)
	if got != want {
		t.Errorf("JudgeAskLine() = %q; want %q", got, want)
	}
}

// TestJudgementSummary covers each kind of ruling the summary renders, and a judgement with none.
func TestJudgementSummary(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name      string
		judgement Judgement
		want      string
	}{
		{name: "no rulings", want: "no rulings"},
		{
			name:      "cells that all match name none",
			judgement: Judgement{Cells: []CellRuling{{ID: "c1", Ruling: RulingMatches}}},
			want:      "1 cells, 1 matches and 0 does not match",
		},
		{
			name:      "every case with its ruling",
			judgement: Judgement{Cases: []CaseRuling{{Case: "a", Ruling: RulingMatches}, {Case: "b", Ruling: RulingUnexpected}}},
			want:      "case a matches, case b unexpected",
		},
		{
			name: "cells and files with the files holding an uncaused check named",
			judgement: Judgement{
				Cells: []CellRuling{{ID: "c1", Ruling: RulingNoMatch}},
				Files: []FileRuling{
					{ID: "f1", Ruling: RulingNone},
					{ID: "f2", Ruling: RulingLoosened, Checks: []LoosenedRuling{{Ruling: RulingCaused}}},
					{ID: "f3", Ruling: RulingLoosened, Checks: []LoosenedRuling{{Ruling: RulingUncaused}, {Ruling: RulingUncaused}}},
				},
			},
			want: "1 cells, 0 matches and 1 does not match (c1); 3 files, 1 none and 2 loosened (an uncaused check in f3)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := judgementSummary(tt.judgement); got != tt.want {
				t.Errorf("judgementSummary() = %q; want %q", got, tt.want)
			}
		})
	}
}
