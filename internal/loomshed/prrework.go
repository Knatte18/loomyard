// prrework.go declares the rework round layout that a run's earlier PR-Rework rounds leave on disk;
// it also reads the plan committed at HEAD for those rounds.
//
// The round layout is declared once here, by the constants below:
// round-<N>/ holds record.json and prior-generation/,
// and prior-generation/ holds plan/, webster/ and reviews/<run_subdir>/.

package loomshed

import (
	"fmt"
	"io/fs"
	"path"

	"github.com/Knatte18/loomyard/internal/planparser"
)

const (
	reworkRoundPrefix = "round-"
	reworkRecordFile  = "record.json"

	// reworkPriorDir holds a round's archived generation: its plan, webster's run record and its reviews.
	reworkPriorDir     = "prior-generation"
	reworkPriorWebster = "webster"

	// ReworkClassExempt and ReworkClassRequired are the two values of a round record's class field.
	ReworkClassExempt   = "exempt"
	ReworkClassRequired = "required"
)

// PRReworkDeps carries the paths and seams a PR-Rework round needs to read its earlier rounds.
// It also carries the seam that archives webster's run record.
type PRReworkDeps struct {
	// PlanDir is the absolute plan directory.
	PlanDir string
	// ReworkDir is the absolute directory holding the round-<N> directories.
	ReworkDir string
	// ReworkDirRel is ReworkDir relative to the anchor, the form a committed-file read takes.
	ReworkDirRel string
	// ReviewsDir is the absolute root every review segment's run directory lives under.
	ReviewsDir string
	// AmendmentPath is the absolute plan amendment record; empty means the run has none to read.
	AmendmentPath string
	// ReadCommitted returns an anchor-relative file as committed at HEAD; found is false when HEAD has no such file.
	ReadCommitted func(anchorRel string) ([]byte, bool, error)
	// ArchiveWebster moves webster's durable run record into dest, so the next webster run starts fresh.
	ArchiveWebster func(dest string) error
}

// roundRecord is the on-disk form of a round's record.json.
// A record without Class is an archive's completion marker, which a round never committed.
type roundRecord struct {
	PRNumber   int    `json:"pr_number"`
	HeadSHA    string `json:"head_sha"`
	RejectedAt string `json:"rejected_at"`
	FirstCard  int    `json:"first_card"`
	Class      string `json:"class,omitempty"`
}

// ParseCommittedPlan parses the plan at planDir as committed at HEAD;
// it reads every plan file through readCommitted, which takes an anchor-relative path.
// The plan it returns is the generation a rework round archives:
// every card it carries was planned, built and committed before that round began.
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

// NextReworkCardNumber returns the number a rework round's first new card takes:
// one past the highest card of the plan committed at HEAD.
// It reads planDir as committed at HEAD, through readCommitted;
// a file absent at HEAD is an error wrapping fs.ErrNotExist.
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
