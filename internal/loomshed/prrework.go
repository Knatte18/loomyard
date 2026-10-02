// prrework.go implements the PR-Rework row's producer: it wraps the gated rework session and does everything Go owns around it.
// That is the archive of the live plan generation into the round's own directory before the session runs,
// the round record that classifies the new generation as exempt from or subject to Plan-Review, the round commit, and the removal of the pending rejection.
//
// The round layout is declared once here, by the constants below:
// round-<N>/ holds findings.md, record.json, coverage.md and prior-generation/,
// and prior-generation/ holds plan/, webster/ and reviews/<run_subdir>/.

package loomshed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const (
	reworkRoundPrefix   = "round-"
	reworkFindingsFile  = "findings.md"
	reworkRecordFile    = "record.json"
	reworkCoverageFile  = "coverage.md"
	reworkRejectCommand = "lyx loom reject <review-file>"

	// reworkPriorDir holds a round's archived generation; its three children below are that generation's plan, webster run record and review run directories.
	reworkPriorDir     = "prior-generation"
	reworkPriorPlan    = "plan"
	reworkPriorWebster = "webster"
	reworkPriorReviews = "reviews"

	// ReworkClassExempt and ReworkClassRequired are the two values of a round record's class field.
	ReworkClassExempt   = "exempt"
	ReworkClassRequired = "required"
)

// PendingRejection is this package's own view of the operator's pending rejection record.
// The caller fills it from landingshed.ReadRejection, the way battenshed.ChildDecision is filled,
// so internal/landingshed stays off this package's import allowlist.
type PendingRejection struct {
	PRNumber   int
	HeadSHA    string
	RejectedAt string
	Findings   string
}

// PRReworkDeps carries the told values and seams NewPRRework needs.
type PRReworkDeps struct {
	// PlanDir is the absolute plan directory.
	PlanDir string
	// ReworkDir is the absolute directory holding the round-<N> directories.
	ReworkDir string
	// ReworkDirRel is ReworkDir relative to the anchor, the form ReadCommitted takes.
	ReworkDirRel string
	// ReviewsDir is the absolute root every review segment's run directory lives under.
	ReviewsDir string
	// ReviewRunSubdirs names the run directories under ReviewsDir that belong to one generation, each archived with the round.
	ReviewRunSubdirs []string
	// ReadCommitted returns an anchor-relative file as committed at HEAD; found is false when HEAD has no such file.
	ReadCommitted func(anchorRel string) ([]byte, bool, error)
	// ReadRejection returns the pending rejection; found is false when none is recorded.
	ReadRejection func() (PendingRejection, bool, error)
	// ClearRejection removes the pending rejection record.
	ClearRejection func() error
	// ArchiveWebster moves webster's durable run record into dest, so the next webster run starts fresh.
	ArchiveWebster func(dest string) error
	// Commit commits the round: the plan directory, the rework directory, the reviews root and webster's directory.
	Commit func() error
}

// roundRecord is the on-disk form of a round's record.json.
// A record without Class is the archive's completion marker; the class is added once the session's plan is in.
type roundRecord struct {
	PRNumber   int    `json:"pr_number"`
	HeadSHA    string `json:"head_sha"`
	RejectedAt string `json:"rejected_at"`
	FirstCard  int    `json:"first_card"`
	Class      string `json:"class,omitempty"`
}

// rejectionIdentity names one rejection by the head it rejected and the time it was recorded.
// The head alone is not enough: a round that lands no code leaves the head unchanged,
// so the operator's next rejection shares it and must still reach a session of its own.
func rejectionIdentity(headSHA, rejectedAt string) string {
	return headSHA + "@" + rejectedAt
}

// ReworkTold carries the values PR-Rework tells its session, decided by the producer rather than by the wiring.
type ReworkTold struct {
	// FirstCard is the number the session's first new card takes: one past the highest card of the retired generation.
	FirstCard int
	// PriorPlanDir is the archived plan directory the session reads for context.
	PriorPlanDir string
}

// prRework decorates the gated rework session, built per Call by session, with the Go-owned steps around it.
// It is a distinct type so the recipe shape test can tell the row apart.
type prRework struct {
	name    string
	session func(ReworkTold) shedengine.ShedProducer
	deps    PRReworkDeps
}

var _ shedengine.ShedProducer = (*prRework)(nil)

// NewPRRework returns the PR-Rework producer identified as name.
// Each Call builds its session from session, handing it the values the producer decides to tell it.
func NewPRRework(name string, session func(ReworkTold) shedengine.ShedProducer, deps PRReworkDeps) shedengine.ShedProducer {
	return &prRework{name: name, session: session, deps: deps}
}

// Call implements shedengine.ShedProducer.
// An archive, Commit or ClearRejection failure is a returned error, never Stuck, for the reason planWrite.Call gives: re-running the session cannot fix a git or filesystem fault.
func (p *prRework) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := entryErr(ctx, p.name); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.checkSeams(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	pending, found, err := p.deps.ReadRejection()
	if err != nil {
		return stuck(fmt.Sprintf("read the pending rejection: %v; fix it or run %s again", err, reworkRejectCommand))
	}

	committed, err := p.committedRejections()
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	if !found {
		if len(committed) == 0 {
			return stuck("no pending rejection and no committed rework round; run " + reworkRejectCommand)
		}
		// A crash after the record's removal but before Shed persisted Done.
		return p.finish(false)
	}

	identity := rejectionIdentity(pending.HeadSHA, pending.RejectedAt)
	if committed[identity] {
		return p.finish(true)
	}

	round, archived, err := p.roundFor(identity)
	if err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	roundDir := filepath.Join(p.deps.ReworkDir, reworkRoundPrefix+strconv.Itoa(round))

	var rec roundRecord
	if archived {
		rec, err = readRoundRecord(filepath.Join(roundDir, reworkRecordFile))
		if err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: %w", p.name, err)
		}
	} else {
		rec, err = p.archive(pending, roundDir)
		if err != nil {
			return "", shedengine.OutputPointer{}, err
		}
	}
	if err := p.recreateReviewRunDirs(); err != nil {
		return "", shedengine.OutputPointer{}, err
	}

	told := ReworkTold{FirstCard: rec.FirstCard, PriorPlanDir: filepath.Join(roundDir, reworkPriorDir, reworkPriorPlan)}
	outcome, pointer, err := p.session(told).Call(ctx)
	if err != nil || outcome != shedengine.Done {
		return outcome, pointer, err
	}

	plan, err := planparser.ParsePlan(p.deps.PlanDir)
	if err != nil {
		return stuck("rework plan does not parse: " + err.Error())
	}
	rec.Class = ReworkClassRequired
	if planparser.ReviewExempt(plan) {
		rec.Class = ReworkClassExempt
	}
	if err := p.finishRound(roundDir, rec, pointer.Path); err != nil {
		return "", shedengine.OutputPointer{}, err
	}
	if err := p.deps.Commit(); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: commit rework round: %w", p.name, err)
	}
	return p.finish(true)
}

// checkSeams names the first unwired seam; the constructor returns the bare seam interface and so has no error to refuse a nil through.
func (p *prRework) checkSeams() error {
	seams := []struct {
		name    string
		missing bool
	}{
		{"session factory", p.session == nil},
		{"ReadCommitted", p.deps.ReadCommitted == nil},
		{"ReadRejection", p.deps.ReadRejection == nil},
		{"ClearRejection", p.deps.ClearRejection == nil},
		{"ArchiveWebster", p.deps.ArchiveWebster == nil},
		{"Commit", p.deps.Commit == nil},
		{"ReviewsDir", p.deps.ReviewsDir == ""},
	}
	for _, s := range seams {
		if s.missing {
			return fmt.Errorf("loomshed: %s: no %s seam wired", p.name, s.name)
		}
	}
	return nil
}

// finish runs the last step: when clear, remove the pending record, then Done.
// The absent-record crash path calls it with clear false, since that record is already gone.
func (p *prRework) finish(clear bool) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if clear {
		if err := p.deps.ClearRejection(); err != nil {
			return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: clear rejection: %w", p.name, err)
		}
	}
	return shedengine.Done, shedengine.OutputPointer{}, nil
}

// stuck returns a Stuck verdict carrying reason.
func stuck(reason string) (shedengine.Outcome, shedengine.OutputPointer, error) {
	return shedengine.Stuck, shedengine.OutputPointer{Reason: reason}, nil
}

// committedRejections returns the identities of the rejections recorded by rounds committed at HEAD.
func (p *prRework) committedRejections() (map[string]bool, error) {
	rounds, err := committedRounds(p.deps)
	if err != nil {
		return nil, fmt.Errorf("loomshed: %s: %w", p.name, err)
	}
	identities := make(map[string]bool)
	for _, r := range rounds {
		identities[rejectionIdentity(r.record.HeadSHA, r.record.RejectedAt)] = true
	}
	return identities, nil
}

// ParseCommittedPlan parses the plan at planDir as committed at HEAD, reading every plan file through readCommitted, which takes an anchor-relative path.
// It is the retired generation a rework round archives: every card it carries was planned, built and committed before the round began.
// A plan file absent at HEAD is an error wrapping fs.ErrNotExist.
func ParseCommittedPlan(planDir string, readCommitted func(anchorRel string) ([]byte, bool, error)) (*planparser.Plan, error) {
	return planparser.ParsePlanFrom(planDir, func(name string) ([]byte, error) {
		rel := path.Join(planparser.PlanDirRel(), name)
		data, ok, err := readCommitted(rel)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%s not committed at HEAD: %w", rel, fs.ErrNotExist)
		}
		return data, nil
	})
}

// NextReworkCardNumber returns the number a rework round's first new card takes: one past the highest card of the plan committed at HEAD.
// Its arguments are ParseCommittedPlan's.
func NextReworkCardNumber(planDir string, readCommitted func(anchorRel string) ([]byte, bool, error)) (int, error) {
	committed, err := ParseCommittedPlan(planDir, readCommitted)
	if err != nil {
		return 0, err
	}
	highest := 0
	for _, c := range committed.Cards {
		highest = max(highest, c.Number)
	}
	return highest + 1, nil
}

// roundFor picks the round for the rejection named by identity.
// A working-tree round whose record.json records the identity means the archive is done (archived is true) and the session may have begun, so nothing is moved again.
// Otherwise the highest round directory with no record.json is this rejection's interrupted archive and is resumed.
// Otherwise it is a new round, the highest plus one.
func (p *prRework) roundFor(identity string) (round int, archived bool, err error) {
	nums, err := roundNumbers(p.deps.ReworkDir)
	if err != nil {
		return 0, false, fmt.Errorf("loomshed: %s: %w", p.name, err)
	}
	highest, unfinished := 0, 0
	for _, n := range nums {
		highest = max(highest, n)
		data, err := os.ReadFile(filepath.Join(p.deps.ReworkDir, reworkRoundPrefix+strconv.Itoa(n), reworkRecordFile))
		if err != nil {
			unfinished = max(unfinished, n)
			continue
		}
		var rec roundRecord
		if json.Unmarshal(data, &rec) == nil && rejectionIdentity(rec.HeadSHA, rec.RejectedAt) == identity {
			return n, true, nil
		}
	}
	if unfinished > 0 {
		return unfinished, false, nil
	}
	return highest + 1, false, nil
}

// LatestArchivedReviewsDir returns the reviews directory of the highest round's prior-generation under reworkDir, or "" when no round exists.
// The round commit uses it to tell whether the round moved tracked review files out of the reviews root.
func LatestArchivedReviewsDir(reworkDir string) string {
	return latestPriorChild(reworkDir, reworkPriorReviews)
}

// LatestArchivedWebsterDir returns the webster directory of the highest round's prior-generation under reworkDir, or "" when no round exists.
func LatestArchivedWebsterDir(reworkDir string) string {
	return latestPriorChild(reworkDir, reworkPriorWebster)
}

// latestPriorChild joins child onto the highest round's prior-generation directory under reworkDir.
func latestPriorChild(reworkDir, child string) string {
	nums, err := roundNumbers(reworkDir)
	if err != nil || len(nums) == 0 {
		return ""
	}
	highest := 0
	for _, n := range nums {
		highest = max(highest, n)
	}
	return filepath.Join(reworkDir, reworkRoundPrefix+strconv.Itoa(highest), reworkPriorDir, child)
}

// readRoundRecord decodes the record.json at recordPath.
func readRoundRecord(recordPath string) (roundRecord, error) {
	data, err := os.ReadFile(recordPath)
	if err != nil {
		return roundRecord{}, fmt.Errorf("read round record %s: %w", recordPath, err)
	}
	var rec roundRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return roundRecord{}, fmt.Errorf("decode round record %s: %w", recordPath, err)
	}
	return rec, nil
}

// archive moves the live generation into roundDir's prior-generation directory and writes the round's findings and record.
// The first card is read from the plan committed at HEAD, which is still the retired generation.
// record.json is written last, as the archive's completion marker, so a crash leaves a round without one and the next Call resumes it.
func (p *prRework) archive(pending PendingRejection, roundDir string) (roundRecord, error) {
	firstCard, err := NextReworkCardNumber(p.deps.PlanDir, p.deps.ReadCommitted)
	if err != nil {
		return roundRecord{}, fmt.Errorf("loomshed: %s: number the first new card: %w", p.name, err)
	}
	priorDir := filepath.Join(roundDir, reworkPriorDir)

	moves, err := p.planMoves(priorDir)
	if err != nil {
		return roundRecord{}, err
	}
	if err := os.MkdirAll(roundDir, 0o755); err != nil {
		return roundRecord{}, fmt.Errorf("loomshed: %s: create round directory: %w", p.name, err)
	}
	if err := os.WriteFile(filepath.Join(roundDir, reworkFindingsFile), []byte(pending.Findings), 0o644); err != nil {
		return roundRecord{}, fmt.Errorf("loomshed: %s: write round %s: %w", p.name, reworkFindingsFile, err)
	}
	for _, m := range moves {
		if err := os.MkdirAll(filepath.Dir(m.to), 0o755); err != nil {
			return roundRecord{}, fmt.Errorf("loomshed: %s: create archive directory: %w", p.name, err)
		}
		if err := os.Rename(m.from, m.to); err != nil {
			return roundRecord{}, fmt.Errorf("loomshed: %s: archive %s into %s: %w", p.name, m.from, m.to, err)
		}
	}
	if err := os.MkdirAll(p.deps.PlanDir, 0o755); err != nil {
		return roundRecord{}, fmt.Errorf("loomshed: %s: recreate plan directory: %w", p.name, err)
	}
	if err := p.deps.ArchiveWebster(filepath.Join(priorDir, reworkPriorWebster)); err != nil {
		return roundRecord{}, fmt.Errorf("loomshed: %s: archive webster run record: %w", p.name, err)
	}

	rec := roundRecord{PRNumber: pending.PRNumber, HeadSHA: pending.HeadSHA, RejectedAt: pending.RejectedAt, FirstCard: firstCard}
	if err := writeRoundRecord(filepath.Join(roundDir, reworkRecordFile), rec); err != nil {
		return roundRecord{}, fmt.Errorf("loomshed: %s: %w", p.name, err)
	}
	return rec, nil
}

// move is one rename of the archive.
type move struct{ from, to string }

// planMoves lists the renames that archive every plan entry and every review run directory into priorDir, skipping what is already moved.
// An entry present at both origin and destination is a returned error naming both and the way forward, and nothing is moved.
// An empty review run directory at the origin holds nothing to move: the recipe build recreates it, so it never collides with an archived copy.
func (p *prRework) planMoves(priorDir string) ([]move, error) {
	var moves []move
	entries, err := os.ReadDir(p.deps.PlanDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("loomshed: %s: read plan directory: %w", p.name, err)
	}
	for _, e := range entries {
		moves = append(moves, move{filepath.Join(p.deps.PlanDir, e.Name()), filepath.Join(priorDir, reworkPriorPlan, e.Name())})
	}
	for _, sub := range p.deps.ReviewRunSubdirs {
		from := filepath.Join(p.deps.ReviewsDir, sub)
		if !holdsEntry(from) {
			continue
		}
		moves = append(moves, move{from, filepath.Join(priorDir, reworkPriorReviews, sub)})
	}
	for _, m := range moves {
		if _, err := os.Lstat(m.to); err == nil {
			return nil, fmt.Errorf("loomshed: %s: archive collision: %s exists at both %s and %s; way forward: remove whichever copy is stale, then re-step", p.name, filepath.Base(m.from), m.from, m.to)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("loomshed: %s: stat archive target %s: %w", p.name, m.to, err)
		}
	}
	return moves, nil
}

// holdsEntry reports whether dir exists and has at least one entry.
func holdsEntry(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

// recreateReviewRunDirs recreates the archived review run directories empty, so a segment's first write finds its directory.
func (p *prRework) recreateReviewRunDirs() error {
	for _, sub := range p.deps.ReviewRunSubdirs {
		if err := os.MkdirAll(filepath.Join(p.deps.ReviewsDir, sub), 0o755); err != nil {
			return fmt.Errorf("loomshed: %s: recreate review run directory %s: %w", p.name, sub, err)
		}
	}
	return nil
}

// finishRound copies the session's coverage file into roundDir and rewrites record.json carrying the class.
func (p *prRework) finishRound(roundDir string, rec roundRecord, coveragePath string) error {
	coverage, err := os.ReadFile(coveragePath)
	if err != nil {
		return fmt.Errorf("loomshed: %s: read coverage file: %w", p.name, err)
	}
	if err := os.WriteFile(filepath.Join(roundDir, reworkCoverageFile), coverage, 0o644); err != nil {
		return fmt.Errorf("loomshed: %s: write round %s: %w", p.name, reworkCoverageFile, err)
	}
	if err := writeRoundRecord(filepath.Join(roundDir, reworkRecordFile), rec); err != nil {
		return fmt.Errorf("loomshed: %s: %w", p.name, err)
	}
	return nil
}

// writeRoundRecord encodes rec into the file at recordPath.
func writeRoundRecord(recordPath string, rec roundRecord) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("encode round record: %w", err)
	}
	if err := os.WriteFile(recordPath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write round %s: %w", reworkRecordFile, err)
	}
	return nil
}
