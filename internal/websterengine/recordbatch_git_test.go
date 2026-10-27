//go:build tier1

// recordbatch_git_test.go exercises RecordBatch's post-batch checks over a real scratch git repository:
// the delta of the batch's own commits, which quarry computes from git's blobs, drives handle binding, the scope guard and drift detection,
// so these tests need real commits.
// Every other RecordBatch behavior is tested over a fakeGit in recordbatch_test.go.

package websterengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// recordScenario is the one scratch repository the post-batch steps share: a base commit, then a work commit adding internal/foo/impl.go.
type recordScenario struct {
	repo     string
	startSHA string
	headSHA  string
}

func newRecordScenario(t *testing.T) *recordScenario {
	t.Helper()
	repo := newScratchRepo(t)
	startSHA := gitkit.CommitFile(t, repo, "base.txt", "base", "base commit")
	headSHA := gitkit.CommitFile(t, repo, "internal/foo/impl.go", "package foo\n", "01.1: add impl")
	return &recordScenario{repo: repo, startSHA: startSHA, headSHA: headSHA}
}

// restart puts the repository back on its work commit with a clean tree, and returns a record fixture over it.
// The fixture's Git is nil, so webster asks the repository itself.
func (s *recordScenario) restart(t *testing.T, scripted []shuttleengine.ForkAudit) *recordFixture {
	t.Helper()
	gitkit.Git(t, s.repo, "reset", "--hard", s.headSHA)
	gitkit.Git(t, s.repo, "clean", "-fdx")
	return newRecordFixtureOver(t, s.repo, nil, s.startSHA, s.headSHA, scripted)
}

// newRealRecordFixture builds a record fixture over a scratch repository of its own.
func newRealRecordFixture(t *testing.T, scripted []shuttleengine.ForkAudit) *recordFixture {
	t.Helper()
	return newRecordScenario(t).restart(t, scripted)
}

// forkReturned is the fork audit every step scripts: one fork that returned its report.
func forkReturned() []shuttleengine.ForkAudit {
	return []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", ReportReturned: true}}},
	}
}

// forkRewroteReport scripts the next audits as the handed-back fork resumed: its bracket transcript wrote the batch's report again.
func forkRewroteReport(fx *recordFixture) {
	report := filepath.Join(fx.ReportsDir, websterengine.ReportFileName(1, "json-flag"))
	fx.Audit.scripted = []shuttleengine.ForkAudit{
		{Forks: []shuttleengine.ForkReport{{TranscriptPath: "subagents/f1.jsonl", WritePaths: []string{report}, ReportReturned: true}}},
	}
}

// seedDriftPlanDir writes a real, parseable two-card plan directory whose second card's Uses field
// names every ref in uses. RecordBatch's own drift repair calls planparser.RewriteRefs against this
// directory, which re-parses it from disk rather than reading RecordDeps.Plan, so a repair scenario
// needs genuine card files here and not only the in-memory plan newRecordFixture builds.
func seedDriftPlanDir(t *testing.T, uses []string) string {
	t.Helper()
	dir := t.TempDir()
	base := []plankit.Group{{Label: "Prosa", Targets: []string{"//base.txt"}}}
	plankit.Write(t, dir, plankit.Plan{
		Approved: true,
		Language: "go",
		Framing:  "A two-card fixture whose second card is still pending.",
		Cards: []plankit.Card{
			{Number: 1, Slug: "json-flag", Summary: "the recorded batch's own card", Groups: base, Intent: "The recorded batch's own card."},
			{
				Number:  2,
				Slug:    "pending",
				Summary: "the not-yet-built card",
				Groups:  base,
				Uses:    uses,
				Intent:  "The not-yet-built card that still references what batch 1 moved out from under it.",
			},
		},
	})
	return dir
}

// TestRecordBatch_PostBatchChecks runs every case whose outcome depends on the real delta of the batch's commits over one scratch repository, one step per case.
// Each step starts by restarting the repository from the work commit, so no step relies on another's state.
func TestRecordBatch_PostBatchChecks(t *testing.T) {
	t.Parallel()
	s := newRecordScenario(t)

	// A Create declaration whose symbol actually landed in the batch's own work commit is bound from the
	// record-batch delta and rewritten on disk to its plain glyph, with the batch still terminating cleanly.
	t.Run("binds a handle from the delta", func(t *testing.T) {
		fx := s.restart(t, forkReturned())

		planDir, plan := writeRecordPlanDir(t, "**Create:**\n- `plan:internal/foo#Bar` -> `func Bar() {}`\n\n**Intent:** add Bar\n")
		fx.Deps.Geom.PlanDir = planDir
		fx.Deps.Plan = plan
		fx.Deps.Batches[0].Cards = plan.Cards
		if err := websterengine.RestampPlanBaseline(fx.Deps.State, planDir, fx.Deps.Geom.WebsterDir); err != nil {
			t.Fatalf("RestampPlanBaseline() error = %v", err)
		}

		headSHA := gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc Bar() {}\n", "01.1: add Bar")
		writeReport(t, fx.ReportsDir, validReport(headSHA))

		result, err := websterengine.RecordBatch(fx.Deps, 1)
		if err != nil {
			t.Fatalf("RecordBatch() error = %v; want nil", err)
		}
		if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
			t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
		}

		data, readErr := os.ReadFile(filepath.Join(planDir, "01-json-flag.md"))
		if readErr != nil {
			t.Fatalf("read card file: %v", readErr)
		}
		if len(result.Digest.Deviations) != 0 {
			t.Errorf("Digest.Deviations = %v; want empty, since the change stays inside the target's package", result.Digest.Deviations)
		}
		if strings.Contains(string(data), "plan:internal/foo#Bar") {
			t.Errorf("card file still carries the unbound handle: %s", data)
		}
		if !strings.Contains(string(data), "internal/foo#Bar") {
			t.Errorf("card file does not carry the bound plain glyph: %s", data)
		}
	})

	// A symbol touched outside the completed batch's own target glyphs lands as its file in the digest's deviations.
	// It never lands in RecordResult.Warnings, and the batch still terminates.
	t.Run("scope guard findings land in the digest's deviations, not the warnings", func(t *testing.T) {
		fx := s.restart(t, forkReturned())
		// A symbol added outside the plan's own declared targets.
		headSHA := gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc Surprise() {}\n", "01.2: add Surprise")
		writeReport(t, fx.ReportsDir, validReport(headSHA))
		fx.Deps.Plan.Cards[0].Targets = []string{"unrelated/thing#Nothing"}

		result, err := websterengine.RecordBatch(fx.Deps, 1)
		if err != nil {
			t.Fatalf("RecordBatch() error = %v; want nil", err)
		}
		if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
			t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
		}
		if want := []string{"internal/foo/impl.go"}; !slices.Equal(result.Digest.Deviations, want) {
			t.Errorf("Digest.Deviations = %v; want %v", result.Digest.Deviations, want)
		}
		for _, w := range result.Warnings {
			if strings.Contains(w, "scope-outside-plan") {
				t.Errorf("RecordResult.Warnings = %v; want no scope-outside-plan warning", result.Warnings)
			}
		}
	})

	// A symbol the delta reports deleted with no corresponding rename, that the plan still references, hands the batch back to its fork once, leaving the batch open and its report in place;
	// the next call warns about the later card and persists a terminal digest.
	t.Run("drift on a deleted still-referenced symbol hands back once, then warns about the later card", func(t *testing.T) {
		fx := s.restart(t, forkReturned())
		// A symbol must exist at the delta's START side to be reported deleted — the fixture's own
		// StartSHA (base.txt only) predates internal/foo entirely, so the batch's own start boundary is
		// moved to a commit that already carries the symbol, and a later commit removes it.
		withSymbol := gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc WillGoAway() {}\n", "01.2: add WillGoAway")
		fx.Deps.State.Batches[1].StartSHA = withSymbol
		if err := os.Remove(filepath.Join(fx.Worktree, "internal/foo/impl.go")); err != nil {
			t.Fatalf("remove impl.go: %v", err)
		}
		gitkit.Git(t, fx.Worktree, "add", "-A")
		gitkit.Git(t, fx.Worktree, "commit", "-m", "01.3: remove WillGoAway")
		headSHA := gitkit.RevParse(t, fx.Worktree, "HEAD")
		writeReport(t, fx.ReportsDir, validReport(headSHA))
		addPendingCard(fx, []string{"internal/foo#WillGoAway"})

		_, err := websterengine.RecordBatch(fx.Deps, 1)
		var handBack *websterengine.LaterTargetRemovedError
		if !errors.As(err, &handBack) {
			t.Fatalf("RecordBatch() error = %v; want a *LaterTargetRemovedError", err)
		}
		if !strings.Contains(err.Error(), "internal/foo#WillGoAway") || !strings.Contains(err.Error(), "way forward:") || !strings.Contains(err.Error(), "lym webster record-batch 01") {
			t.Errorf("RecordBatch() error = %q; want the removed symbol and the record-batch way forward", err)
		}
		if bs := fx.Deps.State.Batches[1]; bs.Terminal || !bs.LaterTargetsReturned {
			t.Errorf("batch record after the hand-back: terminal %v, LaterTargetsReturned %v; want an open batch marked handed back", bs.Terminal, bs.LaterTargetsReturned)
		}
		if _, statErr := os.Stat(filepath.Join(fx.ReportsDir, websterengine.ReportFileName(1, fx.Deps.State.Batches[1].Slug))); statErr != nil {
			t.Errorf("stat the batch report after the hand-back: %v; want the report left in place", statErr)
		}

		forkRewroteReport(fx)
		result, err := websterengine.RecordBatch(fx.Deps, 1)
		if err != nil {
			t.Fatalf("RecordBatch() second call error = %v; want nil — after the one hand-back, drift about a later card warns, it does not fail this batch", err)
		}
		if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
			t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
		}
		var inWarnings int
		for _, w := range result.Warnings {
			if strings.Contains(w, "later card:") && strings.Contains(w, "WillGoAway") {
				inWarnings++
			}
		}
		if inWarnings != 1 {
			t.Errorf("RecordResult.Warnings = %v; want exactly one later card: warning", result.Warnings)
		}
		var recorded int
		for _, w := range fx.Deps.State.Batches[1].AuditWarnings {
			if w.Class == "later-card-drift" {
				recorded++
			}
		}
		if recorded != 1 {
			t.Errorf("BatchState.AuditWarnings = %+v; want exactly one later-card-drift entry", fx.Deps.State.Batches[1].AuditWarnings)
		}
	})

	// An inexact rename quarry classifies as an evidence-tier candidate hands the batch back once, naming the candidate.
	// The next call lets the batch terminate with the informational rename-candidate finding on Warnings:
	// a finding that kills the batch never reaches the reviewer whose decision the tier exists to inform.
	t.Run("an evidence-tier rename candidate hands back once, then warns and does not block", func(t *testing.T) {
		fx := s.restart(t, forkReturned())
		// The symbol must exist at the delta's start side to be reported deleted, so the batch's own
		// start boundary moves to a commit that already carries it.
		withSymbol := gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc WillMove() int { return 1 }\n", "01.2: add WillMove")
		fx.Deps.State.Batches[1].StartSHA = withSymbol
		// Renamed AND rewritten: the token streams differ in length, so quarry's exact tier declines it
		// and offers it as a candidate instead.
		headSHA := gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc Moved() int {\n\ttotal := 1\n\ttotal += 0\n\treturn total\n}\n", "01.3: rename and rewrite")
		writeReport(t, fx.ReportsDir, validReport(headSHA))
		addPendingCard(fx, []string{"internal/foo#WillMove"})

		if _, err := websterengine.RecordBatch(fx.Deps, 1); !errors.Is(err, websterengine.ErrLaterTargetRemoved) || !strings.Contains(err.Error(), "internal/foo#Moved") {
			t.Fatalf("RecordBatch() error = %v; want ErrLaterTargetRemoved naming the candidate internal/foo#Moved", err)
		}

		forkRewroteReport(fx)
		result, err := websterengine.RecordBatch(fx.Deps, 1)
		if err != nil {
			t.Fatalf("RecordBatch() second call error = %v; want nil — an informational rename-candidate must never fail the batch", err)
		}
		if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
			t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
		}
		var surfaced bool
		for _, w := range result.Warnings {
			if strings.Contains(w, "rename-candidate") {
				surfaced = true
			}
		}
		if !surfaced {
			t.Errorf("RecordResult.Warnings = %v; want the informational rename-candidate finding surfaced there", result.Warnings)
		}
	})

	// The same rename-and-rewrite quarry only offers as a candidate is repaired when the fork reports it:
	// the pending card names the new glyph, and neither a candidate nor a later-card warning is left.
	t.Run("a reported rename quarry cannot prove rewrites the later card", func(t *testing.T) {
		fx := s.restart(t, forkReturned())
		planDir := seedDriftPlanDir(t, []string{"internal/foo#WillMove"})
		fx.Deps.Geom.PlanDir = planDir
		fx.Deps.State.PlanFingerprint = mustFingerprint(t, planDir)

		withSymbol := gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc WillMove() int { return 1 }\n", "01.2: add WillMove")
		fx.Deps.State.Batches[1].StartSHA = withSymbol
		headSHA := gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc Moved() int {\n\ttotal := 1\n\ttotal += 0\n\treturn total\n}\n", "01.3: rename and rewrite")
		writeReport(t, fx.ReportsDir, validReport(headSHA)+"renames:\n  - old: internal/foo#WillMove\n    new: internal/foo#Moved\n    reason: WillMove breaks a lint rule\n")
		addPendingCard(fx, []string{"internal/foo#WillMove"})

		result, err := websterengine.RecordBatch(fx.Deps, 1)
		if err != nil {
			t.Fatalf("RecordBatch() error = %v; want nil", err)
		}
		if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
			t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
		}
		for _, w := range result.Warnings {
			if strings.Contains(w, "rename-candidate") || strings.Contains(w, "later card:") {
				t.Errorf("RecordResult.Warnings = %v; want no candidate or later-card warning for a repaired rename", result.Warnings)
			}
		}
		pending, readErr := os.ReadFile(filepath.Join(planDir, "02-pending.md"))
		if readErr != nil {
			t.Fatalf("read 02-pending.md: %v", readErr)
		}
		if !strings.Contains(string(pending), "internal/foo#Moved") || strings.Contains(string(pending), "internal/foo#WillMove") {
			t.Errorf("02-pending.md = %q; want the reported new glyph in place of the old one", pending)
		}
		if want, err := websterengine.Fingerprint(planDir); err != nil || fx.Deps.State.PlanFingerprint != want {
			t.Errorf("State.PlanFingerprint = %q; want the rewritten plan's %q (err %v)", fx.Deps.State.PlanFingerprint, want, err)
		}
	})

	// A FAILED report goes to a recovery strand, so its batch is never handed back to its fork over a removed later target.
	t.Run("a failed report is never handed back over a removed later target", func(t *testing.T) {
		fx := s.restart(t, forkReturned())
		withSymbol := gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc WillGoAway() {}\n", "01.2: add WillGoAway")
		fx.Deps.State.Batches[1].StartSHA = withSymbol
		if err := os.Remove(filepath.Join(fx.Worktree, "internal/foo/impl.go")); err != nil {
			t.Fatalf("remove impl.go: %v", err)
		}
		gitkit.Git(t, fx.Worktree, "add", "-A")
		gitkit.Git(t, fx.Worktree, "commit", "-m", "01.3: remove WillGoAway")
		headSHA := gitkit.RevParse(t, fx.Worktree, "HEAD")
		writeReport(t, fx.ReportsDir, "status: FAILED\nhead_sha: "+headSHA+"\n")
		addPendingCard(fx, []string{"internal/foo#WillGoAway"})

		_, err := websterengine.RecordBatch(fx.Deps, 1)
		if err != nil {
			t.Fatalf("RecordBatch() error = %v; want nil: a FAILED report is recorded, never handed back over a removed later target", err)
		}
		if fx.Deps.State.Batches[1].LaterTargetsReturned {
			t.Error("LaterTargetsReturned is set after a FAILED report; want it unset")
		}
	})

	// The batch being recorded is excluded from drift detection. Its work is exactly what the delta reports, so a Delete card
	// referencing the symbol it just deleted was reporting its own success as
	// plan-references-deleted-symbol — and no Delete card could ever be recorded at all.
	t.Run("a delete card deleting its own target is not drift", func(t *testing.T) {
		fx := s.restart(t, forkReturned())
		withSymbol := gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc WillGoAway() {}\n", "01.2: add WillGoAway")
		fx.Deps.State.Batches[1].StartSHA = withSymbol
		if err := os.Remove(filepath.Join(fx.Worktree, "internal/foo/impl.go")); err != nil {
			t.Fatalf("remove impl.go: %v", err)
		}
		gitkit.Git(t, fx.Worktree, "add", "-A")
		gitkit.Git(t, fx.Worktree, "commit", "-m", "01.3: remove WillGoAway")
		headSHA := gitkit.RevParse(t, fx.Worktree, "HEAD")
		writeReport(t, fx.ReportsDir, validReport(headSHA))

		// The card being recorded IS the Delete card, and it names the symbol its own batch removed.
		fx.Deps.Plan.Cards[0].Targets = []string{"internal/foo#WillGoAway"}
		fx.Deps.Plan.Cards[0].TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeDelete, Refs: []string{"internal/foo#WillGoAway"}},
		}
		fx.Deps.Batches[0].Cards = fx.Deps.Plan.Cards[:1]

		result, err := websterengine.RecordBatch(fx.Deps, 1)
		if err != nil {
			t.Fatalf("RecordBatch() error = %v; want nil — a Delete card's own deletion is its success, never drift", err)
		}
		if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
			t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
		}
	})

	// A Create target whose symbol actually landed in the batch's own work commit passes the done-checks and the batch
	// still terminates cleanly.
	t.Run("the done-checks pass on a landed create", func(t *testing.T) {
		fx := s.restart(t, forkReturned())
		// The fixture's own work commit adds internal/foo/impl.go with no declared symbol; add one
		// the fixture's own Create target can resolve against.
		gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl.go", "package foo\n\nfunc Bar() {}\n", "01.2: add Bar")
		headSHA := gitkit.CommitFile(t, fx.Worktree, "base.txt", "base updated", "01.3: bump base")
		writeReport(t, fx.ReportsDir, validReport(headSHA))
		fx.Deps.Plan.Cards[0].TargetGroups = []planparser.TargetGroup{
			{Type: planparser.CardTypeCreate, Refs: []string{"internal/foo#Bar"}},
		}

		result, err := websterengine.RecordBatch(fx.Deps, 1)
		if err != nil {
			t.Fatalf("RecordBatch() error = %v; want nil", err)
		}
		if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
			t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
		}
	})

	// Regression for the round-4 review's R4-01. BindHandles and DetectDrift both rewrite the plan on disk BEFORE they report a
	// finding, and a single call routinely does both: here the delta carries an exact-tier rename the
	// pending card references (repaired, so planparser.RewriteRefs rewrites 02-pending.md) alongside a
	// deletion the same card references (blocking, so RecordBatch returns ErrCardNotDone).
	//
	// With the re-baseline positioned after the refusal, state.json kept the pre-rewrite fingerprint
	// while the plan on disk carried webster's own sanctioned edit, so every later begin-batch refused
	// it as a foreign edit — and `--fresh`, the advised recourse, then refused the run outright over the
	// cards that had already landed. The run was unrecoverable without hand-editing state.json.
	t.Run("the plan fingerprint is restamped even when drift repairs the plan", func(t *testing.T) {
		fx := s.restart(t, forkReturned())

		planDir := seedDriftPlanDir(t, []string{"internal/foo#WillMove", "internal/foo#WillGoAway"})
		fx.Deps.Geom.PlanDir = planDir
		seeded := mustFingerprint(t, planDir)
		fx.Deps.State.PlanFingerprint = seeded

		// Both symbols must exist at the delta's start side, so the batch's own start boundary moves to
		// a commit that already carries them.
		withSymbols := gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl.go",
			"package foo\n\nfunc WillMove() int { return 1 }\n\nfunc WillGoAway() {}\n", "01.2: add both symbols")
		fx.Deps.State.Batches[1].StartSHA = withSymbols
		// One exact-tier rename (identical body, so quarry asserts the pair) plus one genuine deletion.
		headSHA := gitkit.CommitFile(t, fx.Worktree, "internal/foo/impl.go",
			"package foo\n\nfunc Moved() int { return 1 }\n", "01.3: rename one, delete the other")
		writeReport(t, fx.ReportsDir, validReport(headSHA))
		addPendingCard(fx, []string{"internal/foo#WillMove", "internal/foo#WillGoAway"})

		// The first call hands back the plain deletion only: the exact-tier rename is quarry's own pair, never a removed target.
		_, err := websterengine.RecordBatch(fx.Deps, 1)
		var handBack *websterengine.LaterTargetRemovedError
		if !errors.As(err, &handBack) {
			t.Fatalf("RecordBatch() error = %v; want a *LaterTargetRemovedError for the deletion", err)
		}
		if !strings.Contains(err.Error(), "internal/foo#WillGoAway") || strings.Contains(err.Error(), "internal/foo#WillMove") {
			t.Errorf("RecordBatch() error = %q; want the deleted symbol named and the exact-renamed one absent", err)
		}

		forkRewroteReport(fx)
		if _, err := websterengine.RecordBatch(fx.Deps, 1); err != nil {
			t.Fatalf("RecordBatch() error = %v; want nil — the deleted-and-still-referenced symbol warns about a later card", err)
		}

		repaired, readErr := os.ReadFile(filepath.Join(planDir, "02-pending.md"))
		if readErr != nil {
			t.Fatalf("read 02-pending.md: %v", readErr)
		}
		if !strings.Contains(string(repaired), "internal/foo#Moved") {
			t.Fatalf("02-pending.md = %q; want the exact-tier repair to have rewritten the renamed glyph — the fixture is not exercising a rewrite at all", repaired)
		}

		if fx.Deps.State.PlanFingerprint == seeded {
			t.Error("State.PlanFingerprint still carries its pre-call value after a call that rewrote the plan on disk; every later begin-batch would refuse webster's own sanctioned rewrite as a foreign edit")
		}
		if fx.Deps.State.PlanFingerprint == "" {
			t.Error("State.PlanFingerprint was cleared rather than re-baselined")
		}
	})

	// A C# re-sign whose edit landed is carried into the plan.
	// The pending card that names the old glyph names the minted id, and the plan is re-baselined so the next begin-batch meets no fingerprint mismatch.
	// Neither the scope guard nor drift detection holds the re-signed file against the batch.
	// A re-sign whose edit did not land fails the batch.
	// Each step runs over a repository of its own, whose base commit declares Draw(int count).
	for _, step := range []struct {
		name string
		// workParam is the type Draw's parameter has in the work commit.
		workParam string
		landed    bool
	}{
		{name: "a landed C# re-sign rewrites the pending card to the minted id", workParam: "long", landed: true},
		{name: "a C# re-sign whose edit did not land fails the batch", workParam: "int"},
	} {
		t.Run(step.name, func(t *testing.T) {
			repo := newScratchRepo(t)
			startSHA := gitkit.CommitFile(t, repo, "csharp/Renderer.cs", csharpRendererSource("int"), "base commit")
			headSHA := gitkit.CommitFile(t, repo, "csharp/Renderer.cs", csharpRendererSource(step.workParam)+"\n", "01.1: edit Draw")
			fx := (&recordScenario{repo: repo, startSHA: startSHA, headSHA: headSHA}).restart(t, forkReturned())

			planDir, plan := csharpResignPlanDir(t)
			fx.Deps.Geom.PlanDir = planDir
			fx.Deps.Plan = plan
			fx.Deps.Batches = []batcher.Batch{{Cards: plan.Cards[:1]}, {Cards: plan.Cards[1:]}}
			if err := websterengine.RestampPlanBaseline(fx.Deps.State, planDir, fx.Deps.Geom.WebsterDir); err != nil {
				t.Fatalf("RestampPlanBaseline() error = %v", err)
			}
			writeReport(t, fx.ReportsDir, validReport(headSHA))

			result, err := websterengine.RecordBatch(fx.Deps, 1)
			pending, readErr := os.ReadFile(filepath.Join(planDir, "02-pending.md"))
			if readErr != nil {
				t.Fatalf("read 02-pending.md: %v", readErr)
			}

			if !step.landed {
				if !errors.Is(err, websterengine.ErrBatchFailed) || !strings.Contains(err.Error(), "resign-not-done") {
					t.Fatalf("RecordBatch() error = %v; want a failed batch naming resign-not-done", err)
				}
				if !strings.Contains(string(pending), "Glyphchain.Sharp#Renderer.Draw(int)") {
					t.Errorf("02-pending.md = %q; want it untouched when the re-sign is not done", pending)
				}
				return
			}
			if err != nil {
				t.Fatalf("RecordBatch() error = %v; want nil", err)
			}
			if result.Digest == nil || result.Digest.Status != websterengine.DigestStatusDone {
				t.Fatalf("RecordBatch() digest = %+v; want a terminal done digest", result.Digest)
			}
			if !strings.Contains(string(pending), "Glyphchain.Sharp#Renderer.Draw(long)") || strings.Contains(string(pending), "Renderer.Draw(int)") {
				t.Errorf("02-pending.md = %q; want it to name the minted id and not the old glyph", pending)
			}
			if want, err := websterengine.Fingerprint(planDir); err != nil || fx.Deps.State.PlanFingerprint != want {
				t.Errorf("State.PlanFingerprint = %q; want the plan directory's own %q (err %v), the one begin-batch compares", fx.Deps.State.PlanFingerprint, want, err)
			}
			if slices.Contains(result.Digest.Deviations, "csharp/Renderer.cs") {
				t.Errorf("Digest.Deviations = %v; want no csharp/Renderer.cs entry", result.Digest.Deviations)
			}
			for _, w := range result.Warnings {
				if strings.Contains(w, "later card:") {
					t.Errorf("RecordResult.Warnings = %v; want no later card: warning", result.Warnings)
				}
			}
		})
	}
}
