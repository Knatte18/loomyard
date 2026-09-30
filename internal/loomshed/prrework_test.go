// prrework_test.go exercises the PR-Rework row's producer at Tier 1: a fake inner session that edits the working-tree plan, fake record seams, and an in-memory ReadCommitted over a map.

package loomshed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const (
	reworkTestHead       = "abc123"
	reworkTestRejectedAt = "2026-09-30T10:00:00Z"
	reworkTestFindings   = "the findings text\n"
	reworkTestCard1      = "# Card 1 — first-card\n\n**Create:**\n- `internal/firstcard/new.go`\n\n**Intent:** placeholder card.\n"
	reworkTestCard2      = "# Card 2 — second-card\n\n**Create:**\n- `internal/secondcard/new.go`\n\n**Intent:** appended card.\n"
)

func reworkOverview(approved bool, framing string, cards ...string) string {
	var index strings.Builder
	for i, c := range cards {
		fmt.Fprintf(&index, "%d — %s — placeholder card %d\n", i+1, c, i+1)
	}
	return fmt.Sprintf("---\nformat: 5\napproved: %t\nlanguage: none\n---\n\n# Plan\n\n%s\n\n## Card Index\n\n%s", approved, framing, index.String())
}

// reworkFixture is one PR-Rework test setup: a committed one-card plan mirrored in the working tree.
type reworkFixture struct {
	t         *testing.T
	anchor    string
	planDir   string
	reworkDir string
	committed map[string][]byte

	pending    *PendingRejection
	pendingErr error
	cleared    int
	commits    int
	rebaseline int
	innerCalls int
	coverage   string

	// onInner runs inside the fake session, before it reports Done.
	onInner func()
}

func newReworkFixture(t *testing.T) *reworkFixture {
	t.Helper()
	anchor := t.TempDir()
	f := &reworkFixture{
		t:         t,
		anchor:    anchor,
		planDir:   planparser.PlanDir(anchor),
		reworkDir: filepath.Join(anchor, "rework"),
		committed: map[string][]byte{},
		pending:   &PendingRejection{PRNumber: 7, HeadSHA: reworkTestHead, RejectedAt: reworkTestRejectedAt, Findings: reworkTestFindings},
		coverage:  filepath.Join(anchor, "coverage-out.md"),
	}
	f.writePlan(true, "Framing.", map[string]string{"01-first-card.md": reworkTestCard1}, "first-card")
	for _, name := range []string{"00-overview.md", "01-first-card.md"} {
		data, err := os.ReadFile(filepath.Join(f.planDir, name))
		if err != nil {
			t.Fatal(err)
		}
		f.committed[path.Join(planparser.PlanDirRel(), name)] = data
	}
	if err := os.WriteFile(f.coverage, []byte("coverage map\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *reworkFixture) writePlan(approved bool, framing string, cards map[string]string, slugs ...string) {
	f.t.Helper()
	if err := os.MkdirAll(f.planDir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	for name, body := range cards {
		if err := os.WriteFile(filepath.Join(f.planDir, name), []byte(body), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(f.planDir, "00-overview.md"), []byte(reworkOverview(approved, framing, slugs...)), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *reworkFixture) appendCard() {
	f.writePlan(true, "Framing.", map[string]string{"01-first-card.md": reworkTestCard1, "02-second-card.md": reworkTestCard2}, "first-card", "second-card")
}

// commitRound records round n as committed at HEAD for the rejection of head at rejectedAt.
func (f *reworkFixture) commitRound(n int, head, rejectedAt string) {
	f.t.Helper()
	dir := filepath.Join(f.reworkDir, fmt.Sprintf("round-%d", n))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	data, _ := json.Marshal(roundRecord{PRNumber: 7, HeadSHA: head, RejectedAt: rejectedAt})
	if err := os.WriteFile(filepath.Join(dir, "record.json"), data, 0o644); err != nil {
		f.t.Fatal(err)
	}
	f.committed[path.Join("rework", fmt.Sprintf("round-%d", n), "record.json")] = data
}

func (f *reworkFixture) producer() shedengine.ShedProducer {
	inner := reworkInner{f: f}
	return NewPRRework("PR-Rework", inner, PRReworkDeps{
		PlanDir:      f.planDir,
		ReworkDir:    f.reworkDir,
		ReworkDirRel: "rework",
		ReadCommitted: func(rel string) ([]byte, bool, error) {
			data, ok := f.committed[rel]
			return data, ok, nil
		},
		ReadRejection: func() (PendingRejection, bool, error) {
			if f.pendingErr != nil {
				return PendingRejection{}, false, f.pendingErr
			}
			if f.pending == nil {
				return PendingRejection{}, false, nil
			}
			return *f.pending, true, nil
		},
		ClearRejection: func() error { f.cleared++; f.pending = nil; return nil },
		Commit:         func() error { f.commits++; return nil },
		Rebaseline:     func() error { f.rebaseline++; return nil },
	})
}

type reworkInner struct{ f *reworkFixture }

func (r reworkInner) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	r.f.innerCalls++
	if r.f.onInner != nil {
		r.f.onInner()
	}
	return shedengine.Done, shedengine.OutputPointer{Path: r.f.coverage}, nil
}

func TestPRRework_StuckWithoutCommit(t *testing.T) {
	cases := []struct {
		name string
		edit func(f *reworkFixture)
		want string
	}{
		{"edited card", func(f *reworkFixture) {
			f.writePlan(true, "Framing.", map[string]string{"01-first-card.md": strings.Replace(reworkTestCard1, "placeholder", "rewritten", 1), "02-second-card.md": reworkTestCard2}, "first-card", "second-card")
		}, "card 1 (first-card)"},
		{"flipped approved", func(f *reworkFixture) {
			f.writePlan(false, "Framing.", map[string]string{"01-first-card.md": reworkTestCard1, "02-second-card.md": reworkTestCard2}, "first-card", "second-card")
		}, "approved"},
		{"edited framing", func(f *reworkFixture) {
			f.writePlan(true, "Other framing.", map[string]string{"01-first-card.md": reworkTestCard1, "02-second-card.md": reworkTestCard2}, "first-card", "second-card")
		}, "framing"},
		{"no new card", func(f *reworkFixture) {}, "no card was appended"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newReworkFixture(t)
			f.onInner = func() { tc.edit(f) }
			outcome, ptr, err := f.producer().Call(context.Background())
			if err != nil || outcome != shedengine.Stuck {
				t.Fatalf("got %v, %v; want Stuck", outcome, err)
			}
			if !strings.Contains(ptr.Reason, tc.want) {
				t.Errorf("reason %q lacks %q", ptr.Reason, tc.want)
			}
			if f.commits != 0 || f.rebaseline != 0 || f.cleared != 0 {
				t.Errorf("commits=%d rebaseline=%d cleared=%d; want none", f.commits, f.rebaseline, f.cleared)
			}
		})
	}
}

func TestPRRework_AppendedCardDone(t *testing.T) {
	f := newReworkFixture(t)
	f.onInner = f.appendCard
	outcome, _, err := f.producer().Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("got %v, %v; want Done", outcome, err)
	}
	if f.commits != 1 || f.rebaseline != 1 || f.cleared != 1 || f.pending != nil {
		t.Errorf("commits=%d rebaseline=%d cleared=%d pending=%v", f.commits, f.rebaseline, f.cleared, f.pending)
	}
	dir := filepath.Join(f.reworkDir, "round-1")
	findings, err := os.ReadFile(filepath.Join(dir, "findings.md"))
	if err != nil || string(findings) != reworkTestFindings {
		t.Errorf("findings.md = %q, %v", findings, err)
	}
	cov, err := os.ReadFile(filepath.Join(dir, "coverage.md"))
	if err != nil || string(cov) != "coverage map\n" {
		t.Errorf("coverage.md = %q, %v", cov, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "record.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rec roundRecord
	if err := json.Unmarshal(data, &rec); err != nil || rec.PRNumber != 7 || rec.HeadSHA != reworkTestHead || rec.RejectedAt == "" {
		t.Errorf("record.json = %s, %v", data, err)
	}
}

func TestPRRework_ReentryAfterCommit(t *testing.T) {
	f := newReworkFixture(t)
	f.commitRound(1, reworkTestHead, reworkTestRejectedAt)
	outcome, _, err := f.producer().Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("got %v, %v; want Done", outcome, err)
	}
	if f.innerCalls != 0 || f.commits != 0 || f.rebaseline != 1 || f.cleared != 1 {
		t.Errorf("inner=%d commits=%d rebaseline=%d cleared=%d", f.innerCalls, f.commits, f.rebaseline, f.cleared)
	}
	entries, _ := os.ReadDir(f.reworkDir)
	if len(entries) != 1 {
		t.Errorf("round directories = %d; want 1", len(entries))
	}
}

// TestPRRework_SecondRejectionAtSameHeadRuns covers a round that landed no code: the operator's next rejection shares its head, and it still gets a session and a round of its own.
func TestPRRework_SecondRejectionAtSameHeadRuns(t *testing.T) {
	const secondRejectedAt = "2026-09-30T11:00:00Z"
	f := newReworkFixture(t)
	f.commitRound(1, reworkTestHead, reworkTestRejectedAt)
	f.pending.RejectedAt = secondRejectedAt
	f.onInner = f.appendCard

	outcome, _, err := f.producer().Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("got %v, %v; want Done", outcome, err)
	}
	if f.innerCalls != 1 || f.commits != 1 || f.cleared != 1 {
		t.Errorf("inner=%d commits=%d cleared=%d; want 1 each", f.innerCalls, f.commits, f.cleared)
	}
	data, err := os.ReadFile(filepath.Join(f.reworkDir, "round-2", "record.json"))
	if err != nil {
		t.Fatalf("round-2 record: %v", err)
	}
	var rec roundRecord
	if err := json.Unmarshal(data, &rec); err != nil || rec.HeadSHA != reworkTestHead || rec.RejectedAt != secondRejectedAt {
		t.Errorf("round-2 record.json = %s, %v; want head %s rejected at %s", data, err, reworkTestHead, secondRejectedAt)
	}
	findings, err := os.ReadFile(filepath.Join(f.reworkDir, "round-2", "findings.md"))
	if err != nil || string(findings) != reworkTestFindings {
		t.Errorf("round-2 findings.md = %q, %v; want %q", findings, err, reworkTestFindings)
	}
}

func TestPRRework_RetryAfterStuckStaysStuck(t *testing.T) {
	f := newReworkFixture(t)
	f.onInner = func() {
		f.writePlan(true, "Framing.", map[string]string{"01-first-card.md": strings.Replace(reworkTestCard1, "placeholder", "rewritten", 1), "02-second-card.md": reworkTestCard2}, "first-card", "second-card")
	}
	p := f.producer()
	for attempt := 1; attempt <= 2; attempt++ {
		if outcome, _, err := p.Call(context.Background()); err != nil || outcome != shedengine.Stuck {
			t.Fatalf("attempt %d: got %v, %v; want Stuck", attempt, outcome, err)
		}
	}
	// The second attempt's session leaves the edit in place without touching it again.
	f.onInner = nil
	if outcome, _, err := p.Call(context.Background()); err != nil || outcome != shedengine.Stuck {
		t.Fatalf("third attempt: got %v, %v; want Stuck", outcome, err)
	}
	if f.commits != 0 {
		t.Errorf("commits = %d; want 0", f.commits)
	}
}

func TestPRRework_AbsentRecordNoRound(t *testing.T) {
	f := newReworkFixture(t)
	f.pending = nil
	outcome, ptr, err := f.producer().Call(context.Background())
	if err != nil || outcome != shedengine.Stuck {
		t.Fatalf("got %v, %v; want Stuck", outcome, err)
	}
	if !strings.Contains(ptr.Reason, "lyx loom reject") {
		t.Errorf("reason %q does not name lyx loom reject", ptr.Reason)
	}
}

func TestPRRework_AbsentRecordWithCommittedRound(t *testing.T) {
	f := newReworkFixture(t)
	f.pending = nil
	f.commitRound(1, reworkTestHead, reworkTestRejectedAt)
	outcome, _, err := f.producer().Call(context.Background())
	if err != nil || outcome != shedengine.Done {
		t.Fatalf("got %v, %v; want Done", outcome, err)
	}
	if f.innerCalls != 0 || f.commits != 0 || f.rebaseline != 1 {
		t.Errorf("inner=%d commits=%d rebaseline=%d", f.innerCalls, f.commits, f.rebaseline)
	}
}

func TestPRRework_MalformedRecordStuck(t *testing.T) {
	f := newReworkFixture(t)
	f.pendingErr = errors.New("bad json")
	outcome, ptr, err := f.producer().Call(context.Background())
	if err != nil || outcome != shedengine.Stuck || !strings.Contains(ptr.Reason, "bad json") {
		t.Fatalf("got %v, %q, %v", outcome, ptr.Reason, err)
	}
}

func TestNextReworkCardNumber(t *testing.T) {
	f := newReworkFixture(t)
	// The working tree's leftover second card is not committed, so it does not move the number.
	f.appendCard()
	reader := func(rel string) ([]byte, bool, error) {
		data, ok := f.committed[rel]
		return data, ok, nil
	}
	got, err := NextReworkCardNumber(f.planDir, reader)
	if err != nil || got != 2 {
		t.Errorf("NextReworkCardNumber() = %d, %v; want 2, nil", got, err)
	}
}

func TestPRRework_NilSeamIsNamedError(t *testing.T) {
	p := NewPRRework("PR-Rework", reworkInner{f: newReworkFixture(t)}, PRReworkDeps{})
	_, _, err := p.Call(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ReadCommitted") {
		t.Fatalf("err = %v; want a named missing-seam error", err)
	}
}
