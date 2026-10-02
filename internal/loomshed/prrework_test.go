// prrework_test.go exercises the PR-Rework row's producer at Tier 1: a fake inner session that writes a new plan generation, fake record seams, and an in-memory ReadCommitted over a map.

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
)

// reworkCard renders a card numbered number whose single target group has label and target.
func reworkCard(number int, slug, label, target string) string {
	return fmt.Sprintf("# Card %d — %s\n\n**%s:**\n- `%s`\n\n**Intent:** card %d.\n", number, slug, label, target, number)
}

// reworkGenCard is one card of a generation the fake session writes.
type reworkGenCard struct {
	slug, label, target string
}

func reworkOverview(approved bool, framing string, first int, slugs ...string) string {
	var index strings.Builder
	for i, c := range slugs {
		fmt.Fprintf(&index, "%d — %s — placeholder card %d\n", first+i, c, first+i)
	}
	return fmt.Sprintf("---\nformat: 5\napproved: %t\nlanguage: none\nfirst_card: %d\n---\n\n# Plan\n\n%s\n\n## Card Index\n\n%s", approved, first, framing, index.String())
}

// reworkFixture is one PR-Rework test setup: a committed one-card plan mirrored in the working tree.
type reworkFixture struct {
	t          *testing.T
	anchor     string
	planDir    string
	reworkDir  string
	reviewsDir string
	committed  map[string][]byte

	pending      *PendingRejection
	pendingErr   error
	cleared      int
	clearErr     error
	commits      int
	commitErr    error
	innerCalls   int
	innerErr     error
	archiveCalls []string
	archiveErr   error
	coverage     string
	told         []ReworkTold

	// onInner runs inside the fake session, before it reports Done.
	onInner func()
}

func newReworkFixture(t *testing.T) *reworkFixture {
	t.Helper()
	anchor := t.TempDir()
	f := &reworkFixture{
		t:          t,
		anchor:     anchor,
		planDir:    planparser.PlanDir(anchor),
		reworkDir:  filepath.Join(anchor, "rework"),
		reviewsDir: filepath.Join(anchor, "reviews"),
		committed:  map[string][]byte{},
		pending:    &PendingRejection{PRNumber: 7, HeadSHA: reworkTestHead, RejectedAt: reworkTestRejectedAt, Findings: reworkTestFindings},
		coverage:   filepath.Join(anchor, "coverage-out.md"),
	}
	f.writeGeneration(1, reworkGenCard{"first-card", "Create", "internal/firstcard/new.go"})
	f.commitWorkingPlan()
	if err := os.WriteFile(f.coverage, []byte("coverage map\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// writeGeneration writes a plan whose cards are numbered from first, into the plan directory.
func (f *reworkFixture) writeGeneration(first int, cards ...reworkGenCard) {
	f.t.Helper()
	if err := os.MkdirAll(f.planDir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	slugs := make([]string, len(cards))
	for i, c := range cards {
		slugs[i] = c.slug
		name := fmt.Sprintf("%02d-%s.md", first+i, c.slug)
		if err := os.WriteFile(filepath.Join(f.planDir, name), []byte(reworkCard(first+i, c.slug, c.label, c.target)), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(f.planDir, "00-overview.md"), []byte(reworkOverview(true, "Framing.", first, slugs...)), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// commitWorkingPlan mirrors the working-tree plan files into the committed map, dropping whatever plan files were committed before.
func (f *reworkFixture) commitWorkingPlan() {
	f.t.Helper()
	prefix := planparser.PlanDirRel() + "/"
	for rel := range f.committed {
		if strings.HasPrefix(rel, prefix) {
			delete(f.committed, rel)
		}
	}
	entries, err := os.ReadDir(f.planDir)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || strings.HasPrefix(e.Name(), "amendments") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(f.planDir, e.Name()))
		if err != nil {
			f.t.Fatal(err)
		}
		f.committed[path.Join(planparser.PlanDirRel(), e.Name())] = data
	}
}

// commitRoundRecords mirrors every round record in the working tree into the committed map, as the round commit does.
func (f *reworkFixture) commitRoundRecords() {
	f.t.Helper()
	matches, _ := filepath.Glob(filepath.Join(f.reworkDir, "round-*", "record.json"))
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			f.t.Fatal(err)
		}
		rel, _ := filepath.Rel(f.reworkDir, m)
		f.committed[path.Join("rework", filepath.ToSlash(rel))] = data
	}
}

// commitRound records round n as committed at HEAD, with class, for the rejection of head at rejectedAt.
func (f *reworkFixture) commitRound(n int, head, rejectedAt, class string) {
	f.t.Helper()
	dir := filepath.Join(f.reworkDir, fmt.Sprintf("round-%d", n))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	data, _ := json.Marshal(roundRecord{PRNumber: 7, HeadSHA: head, RejectedAt: rejectedAt, FirstCard: 2, Class: class})
	if err := os.WriteFile(filepath.Join(dir, "record.json"), data, 0o644); err != nil {
		f.t.Fatal(err)
	}
	f.committed[path.Join("rework", fmt.Sprintf("round-%d", n), "record.json")] = data
}

// writeFile writes body at rel under root, creating parent directories.
func (f *reworkFixture) writeFile(root, rel, body string) {
	f.t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *reworkFixture) producer() shedengine.ShedProducer {
	return NewPRRework("PR-Rework", f.session, f.deps())
}

// deps returns the told values and fake seams the fixture hands the producer.
func (f *reworkFixture) deps() PRReworkDeps {
	return PRReworkDeps{
		PlanDir:          f.planDir,
		ReworkDir:        f.reworkDir,
		ReworkDirRel:     "rework",
		ReviewsDir:       f.reviewsDir,
		ReviewRunSubdirs: []string{"plan", "webster"},
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
		ClearRejection: func() error {
			if f.clearErr != nil {
				err := f.clearErr
				f.clearErr = nil
				return err
			}
			f.cleared++
			f.pending = nil
			return nil
		},
		ArchiveWebster: func(dest string) error {
			if f.archiveErr != nil {
				err := f.archiveErr
				f.archiveErr = nil
				return err
			}
			f.archiveCalls = append(f.archiveCalls, dest)
			f.writeFile(dest, "state.json", "{}")
			return nil
		},
		Commit: func() error {
			if f.commitErr != nil {
				err := f.commitErr
				f.commitErr = nil
				return err
			}
			f.commits++
			f.commitRoundRecords()
			f.commitWorkingPlan()
			return nil
		},
	}
}

// session is the fixture's session factory; it records what the producer told it.
func (f *reworkFixture) session(told ReworkTold) shedengine.ShedProducer {
	f.told = append(f.told, told)
	return reworkInner{f: f}
}

type reworkInner struct{ f *reworkFixture }

func (r reworkInner) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	r.f.innerCalls++
	if r.f.onInner != nil {
		r.f.onInner()
	}
	if r.f.innerErr != nil {
		return "", shedengine.OutputPointer{}, r.f.innerErr
	}
	return shedengine.Done, shedengine.OutputPointer{Path: r.f.coverage}, nil
}

// newGeneration is the session body that writes a one-card generation numbered from first.
func (f *reworkFixture) newGeneration(first int, card reworkGenCard) func() {
	return func() { f.writeGeneration(first, card) }
}

func (f *reworkFixture) call() (shedengine.Outcome, error) {
	f.t.Helper()
	outcome, _, err := f.producer().Call(context.Background())
	return outcome, err
}

func (f *reworkFixture) mustDone() {
	f.t.Helper()
	if outcome, err := f.call(); err != nil || outcome != shedengine.Done {
		f.t.Fatalf("Call = %v, %v; want Done", outcome, err)
	}
}

// dirNames lists the entry names of dir.
func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	return names
}

func (f *reworkFixture) readRecord(round int) roundRecord {
	f.t.Helper()
	rec, err := readRoundRecord(filepath.Join(f.reworkDir, fmt.Sprintf("round-%d", round), "record.json"))
	if err != nil {
		f.t.Fatal(err)
	}
	return rec
}

func (f *reworkFixture) roundDirs() int {
	f.t.Helper()
	nums, err := roundNumbers(f.reworkDir)
	if err != nil {
		f.t.Fatal(err)
	}
	return len(nums)
}

var reworkNewCard = reworkGenCard{"second-card", "Create", "internal/secondcard/new.go"}

func TestPRRework_FirstRoundMovesGenerationZero(t *testing.T) {
	f := newReworkFixture(t)
	f.writeFile(f.planDir, "amendments.md", "amendment\n")
	f.writeFile(f.planDir, "archive-20260101T000000Z/old.md", "old\n")
	f.writeFile(f.reviewsDir, "plan/report.md", "plan review\n")
	f.writeFile(f.reviewsDir, "webster/report.md", "webster review\n")
	f.writeFile(f.reviewsDir, "discussion/report.md", "discussion review\n")
	f.onInner = f.newGeneration(2, reworkNewCard)
	f.mustDone()

	prior := filepath.Join(f.reworkDir, "round-1", "prior-generation")
	for _, rel := range []string{"plan/00-overview.md", "plan/01-first-card.md", "plan/amendments.md", "plan/archive-20260101T000000Z/old.md", "reviews/plan/report.md", "reviews/webster/report.md", "webster/state.json"} {
		if _, err := os.Stat(filepath.Join(prior, filepath.FromSlash(rel))); err != nil {
			t.Errorf("archived %s missing: %v", rel, err)
		}
	}
	if len(f.archiveCalls) != 1 || f.archiveCalls[0] != filepath.Join(prior, "webster") {
		t.Errorf("ArchiveWebster calls = %v; want one call to %s", f.archiveCalls, filepath.Join(prior, "webster"))
	}
	if got := dirNames(t, f.planDir); strings.Join(got, ",") != "00-overview.md,02-second-card.md" {
		t.Errorf("live plan = %v; want only the new generation", got)
	}
	if _, err := os.Stat(filepath.Join(f.reviewsDir, "discussion", "report.md")); err != nil {
		t.Errorf("discussion review was moved: %v", err)
	}
	if f.commits != 1 || f.cleared != 1 || f.pending != nil {
		t.Errorf("commits=%d cleared=%d pending=%v; want 1, 1, nil", f.commits, f.cleared, f.pending)
	}
	rec := f.readRecord(1)
	if rec.PRNumber != 7 || rec.HeadSHA != reworkTestHead || rec.RejectedAt != reworkTestRejectedAt || rec.FirstCard != 2 || rec.Class != ReworkClassRequired {
		t.Errorf("record.json = %+v; want identity, first_card 2 and class required", rec)
	}
	findings, err := os.ReadFile(filepath.Join(f.reworkDir, "round-1", "findings.md"))
	if err != nil || string(findings) != reworkTestFindings {
		t.Errorf("findings.md = %q, %v", findings, err)
	}
	cov, err := os.ReadFile(filepath.Join(f.reworkDir, "round-1", "coverage.md"))
	if err != nil || string(cov) != "coverage map\n" {
		t.Errorf("coverage.md = %q, %v", cov, err)
	}
	if len(f.told) != 1 || f.told[0].FirstCard != 2 || f.told[0].PriorPlanDir != filepath.Join(prior, "plan") {
		t.Errorf("session told %+v; want FirstCard 2 and PriorPlanDir %s", f.told, filepath.Join(prior, "plan"))
	}
}

func TestPRRework_TwoRoundsKeepOneGenerationEach(t *testing.T) {
	f := newReworkFixture(t)
	f.onInner = f.newGeneration(2, reworkNewCard)
	f.mustDone()

	f.pending = &PendingRejection{PRNumber: 7, HeadSHA: "def456", RejectedAt: "2026-09-30T12:00:00Z", Findings: "round two\n"}
	f.onInner = f.newGeneration(3, reworkGenCard{"third-card", "Create", "internal/thirdcard/new.go"})
	f.mustDone()

	if got := dirNames(t, filepath.Join(f.reworkDir, "round-1", "prior-generation", "plan")); strings.Join(got, ",") != "00-overview.md,01-first-card.md" {
		t.Errorf("round-1 plan = %v; want generation 0", got)
	}
	if got := dirNames(t, filepath.Join(f.reworkDir, "round-2", "prior-generation", "plan")); strings.Join(got, ",") != "00-overview.md,02-second-card.md" {
		t.Errorf("round-2 plan = %v; want generation 1", got)
	}
	if got := dirNames(t, f.planDir); strings.Join(got, ",") != "00-overview.md,03-third-card.md" {
		t.Errorf("live plan = %v; want generation 2 only", got)
	}
	if rec := f.readRecord(2); rec.FirstCard != 3 {
		t.Errorf("round-2 first_card = %d; want 3", rec.FirstCard)
	}
}

func TestPRRework_CrashResumeConverges(t *testing.T) {
	t.Run("mid-archive", func(t *testing.T) {
		f := newReworkFixture(t)
		f.onInner = f.newGeneration(2, reworkNewCard)
		f.archiveErr = errors.New("webster is busy")
		if _, err := f.call(); err == nil || !strings.Contains(err.Error(), "webster is busy") {
			t.Fatalf("first Call err = %v; want the archive failure", err)
		}
		if f.innerCalls != 0 {
			t.Fatalf("session ran after a failed archive")
		}
		if _, err := os.Stat(filepath.Join(f.reworkDir, "round-1", "record.json")); err == nil {
			t.Fatal("record.json written before the archive completed")
		}
		f.mustDone()
		if f.roundDirs() != 1 || f.commits != 1 || len(f.archiveCalls) != 1 {
			t.Errorf("rounds=%d commits=%d archives=%d; want 1 each", f.roundDirs(), f.commits, len(f.archiveCalls))
		}
		if got := dirNames(t, filepath.Join(f.reworkDir, "round-1", "prior-generation", "plan")); strings.Join(got, ",") != "00-overview.md,01-first-card.md" {
			t.Errorf("archived plan = %v; want generation 0", got)
		}
	})

	t.Run("after archive before session", func(t *testing.T) {
		f := newReworkFixture(t)
		f.onInner = func() {
			f.writeGeneration(2, reworkNewCard)
			f.innerErr = errors.New("session crashed")
		}
		if _, err := f.call(); err == nil {
			t.Fatal("first Call err = nil; want the session failure")
		}
		f.innerErr = nil
		f.onInner = nil
		f.mustDone()
		if f.roundDirs() != 1 || f.commits != 1 || len(f.archiveCalls) != 1 {
			t.Errorf("rounds=%d commits=%d archives=%d; want 1 each", f.roundDirs(), f.commits, len(f.archiveCalls))
		}
		prior := filepath.Join(f.reworkDir, "round-1", "prior-generation", "plan")
		if _, err := os.Stat(filepath.Join(prior, "02-second-card.md")); err == nil {
			t.Error("a file the session wrote was moved into the archive")
		}
		if _, err := os.Stat(filepath.Join(f.planDir, "02-second-card.md")); err != nil {
			t.Errorf("the session's file left the live plan: %v", err)
		}
	})

	t.Run("after session before commit", func(t *testing.T) {
		f := newReworkFixture(t)
		f.onInner = f.newGeneration(2, reworkNewCard)
		f.commitErr = errors.New("git is busy")
		if _, err := f.call(); err == nil || !strings.Contains(err.Error(), "git is busy") {
			t.Fatalf("first Call err = %v; want the commit failure", err)
		}
		f.mustDone()
		if f.roundDirs() != 1 || f.commits != 1 || len(f.archiveCalls) != 1 {
			t.Errorf("rounds=%d commits=%d archives=%d; want 1 each", f.roundDirs(), f.commits, len(f.archiveCalls))
		}
		if rec := f.readRecord(1); rec.Class != ReworkClassRequired {
			t.Errorf("class = %q; want required", rec.Class)
		}
	})

	t.Run("after commit before rejection cleared", func(t *testing.T) {
		f := newReworkFixture(t)
		f.onInner = f.newGeneration(2, reworkNewCard)
		f.clearErr = errors.New("disk is full")
		if _, err := f.call(); err == nil || !strings.Contains(err.Error(), "disk is full") {
			t.Fatalf("first Call err = %v; want the clear failure", err)
		}
		f.mustDone()
		if f.roundDirs() != 1 || f.commits != 1 || f.innerCalls != 1 || f.cleared != 1 {
			t.Errorf("rounds=%d commits=%d session=%d cleared=%d; want 1 each", f.roundDirs(), f.commits, f.innerCalls, f.cleared)
		}
	})
}

func TestPRRework_ClasslessRecordAtHeadIsNotCommitted(t *testing.T) {
	f := newReworkFixture(t)
	f.commitRound(1, reworkTestHead, reworkTestRejectedAt, "")
	f.onInner = f.newGeneration(2, reworkNewCard)
	f.mustDone()
	if f.innerCalls != 1 || f.commits != 1 {
		t.Errorf("session=%d commits=%d; want 1 each: a classless record is not a committed round", f.innerCalls, f.commits)
	}
	if rec := f.readRecord(1); rec.Class == "" {
		t.Error("the round's record was never given a class")
	}
}

func TestPRRework_ClassRecorded(t *testing.T) {
	tests := []struct {
		name  string
		cards []reworkGenCard
		want  string
	}{
		{"all prosa", []reworkGenCard{{"docs-card", "Prosa", "docs/one.md"}, {"more-docs", "Prosa", "docs/two.md"}}, ReworkClassExempt},
		{"mixed", []reworkGenCard{{"docs-card", "Prosa", "docs/one.md"}, {"code-card", "Create", "internal/x/new.go"}}, ReworkClassRequired},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newReworkFixture(t)
			f.onInner = func() { f.writeGeneration(2, tc.cards...) }
			f.mustDone()
			if rec := f.readRecord(1); rec.Class != tc.want {
				t.Errorf("class = %q; want %q", rec.Class, tc.want)
			}
		})
	}
}

func TestPRRework_CollisionNamesBothAndWayForward(t *testing.T) {
	f := newReworkFixture(t)
	f.writeFile(filepath.Join(f.reworkDir, "round-1", "prior-generation", "plan"), "00-overview.md", "stale\n")
	f.onInner = f.newGeneration(2, reworkNewCard)
	_, err := f.call()
	if err == nil {
		t.Fatal("Call err = nil; want a collision error")
	}
	for _, want := range []string{"exists at both", filepath.Join(f.planDir, "00-overview.md"), filepath.Join(f.reworkDir, "round-1", "prior-generation", "plan", "00-overview.md"), "way forward"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if f.innerCalls != 0 || len(f.archiveCalls) != 0 {
		t.Errorf("session=%d archives=%d; want nothing run after a collision", f.innerCalls, len(f.archiveCalls))
	}
	if got := dirNames(t, f.planDir); strings.Join(got, ",") != "00-overview.md,01-first-card.md" {
		t.Errorf("live plan = %v; want it untouched", got)
	}
}

// TestPRRework_BusyWebsterRefusalReachesTheOperator covers the way forward a webster run holding its run lock gives: it comes through the archive error unchanged.
func TestPRRework_BusyWebsterRefusalReachesTheOperator(t *testing.T) {
	f := newReworkFixture(t)
	f.archiveErr = errors.New("webster: a run holds run.lock; way forward: wait for the run to finish, then retry")
	_, err := f.call()
	if err == nil || !strings.Contains(err.Error(), "wait for the run to finish, then retry") {
		t.Fatalf("Call err = %v; want the way forward in the error", err)
	}
	if f.innerCalls != 0 || f.commits != 0 {
		t.Errorf("session=%d commits=%d; want nothing run", f.innerCalls, f.commits)
	}
}

func TestPRRework_ReentryAfterCommit(t *testing.T) {
	f := newReworkFixture(t)
	f.commitRound(1, reworkTestHead, reworkTestRejectedAt, ReworkClassRequired)
	f.mustDone()
	if f.innerCalls != 0 || f.commits != 0 || f.cleared != 1 {
		t.Errorf("inner=%d commits=%d cleared=%d", f.innerCalls, f.commits, f.cleared)
	}
	if f.roundDirs() != 1 {
		t.Errorf("round directories = %d; want 1", f.roundDirs())
	}
}

// TestPRRework_SecondRejectionAtSameHeadRuns covers a round that landed no code: the operator's next rejection shares its head, and it still gets a session and a round of its own.
func TestPRRework_SecondRejectionAtSameHeadRuns(t *testing.T) {
	const secondRejectedAt = "2026-09-30T11:00:00Z"
	f := newReworkFixture(t)
	f.commitRound(1, reworkTestHead, reworkTestRejectedAt, ReworkClassRequired)
	f.pending.RejectedAt = secondRejectedAt
	f.onInner = f.newGeneration(2, reworkNewCard)
	f.mustDone()
	if f.innerCalls != 1 || f.commits != 1 || f.cleared != 1 {
		t.Errorf("inner=%d commits=%d cleared=%d; want 1 each", f.innerCalls, f.commits, f.cleared)
	}
	rec := f.readRecord(2)
	if rec.HeadSHA != reworkTestHead || rec.RejectedAt != secondRejectedAt {
		t.Errorf("round-2 record.json = %+v; want head %s rejected at %s", rec, reworkTestHead, secondRejectedAt)
	}
	findings, err := os.ReadFile(filepath.Join(f.reworkDir, "round-2", "findings.md"))
	if err != nil || string(findings) != reworkTestFindings {
		t.Errorf("round-2 findings.md = %q, %v; want %q", findings, err, reworkTestFindings)
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
	f.commitRound(1, reworkTestHead, reworkTestRejectedAt, ReworkClassRequired)
	f.mustDone()
	if f.innerCalls != 0 || f.commits != 0 {
		t.Errorf("inner=%d commits=%d", f.innerCalls, f.commits)
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
	f.writeGeneration(1, reworkGenCard{"first-card", "Create", "internal/firstcard/new.go"}, reworkNewCard)
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
	f := newReworkFixture(t)
	p := NewPRRework("PR-Rework", f.session, PRReworkDeps{})
	_, _, err := p.Call(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ReadCommitted") {
		t.Fatalf("err = %v; want a named missing-seam error", err)
	}
}
