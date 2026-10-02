package loomshed

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// roundNumbers lists the N of every round-<N> directory under reworkDir; an absent directory lists none.
func roundNumbers(reworkDir string) ([]int, error) {
	entries, err := os.ReadDir(reworkDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("list rework rounds: %w", err)
	}
	var nums []int
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), reworkRoundPrefix) {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(e.Name(), reworkRoundPrefix))
		if err != nil || n < 1 {
			continue
		}
		nums = append(nums, n)
	}
	return nums, nil
}

// committedRound is one round whose record.json is committed at HEAD and carries a class.
type committedRound struct {
	number int
	record roundRecord
}

// committedRounds returns the rounds committed at HEAD, in the order roundNumbers lists them.
// A round counts as committed only when its record carries a class: a classless record is the archive's completion marker and can reach HEAD outside the round commit,
// and counting it would skip the session.
func committedRounds(deps PRReworkDeps) ([]committedRound, error) {
	nums, err := roundNumbers(deps.ReworkDir)
	if err != nil {
		return nil, err
	}
	var rounds []committedRound
	for _, n := range nums {
		rel := path.Join(deps.ReworkDirRel, reworkRoundPrefix+strconv.Itoa(n), reworkRecordFile)
		data, ok, err := deps.ReadCommitted(rel)
		if err != nil {
			return nil, fmt.Errorf("read committed %s: %w", rel, err)
		}
		if !ok {
			continue
		}
		var rec roundRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return nil, fmt.Errorf("decode committed %s: %w", rel, err)
		}
		if rec.Class == "" {
			continue
		}
		rounds = append(rounds, committedRound{number: n, record: rec})
	}
	return rounds, nil
}

// PlanReviewSkippable reports whether the live plan generation is exempt from Plan-Review, decided from committed state.
// The live generation's round is the highest committed round whose record carries a class and whose first_card equals the live overview's first_card (absent means 1);
// with none, the live plan is generation 0 and never skips.
// It reports true only when that round's record is classed exempt and ReviewExempt agrees over both the live committed cards and the working-tree cards.
// The working tree counts because a Plan-Burler round commits nothing: its edits land only through the approve-and-commit a skip would run, unreviewed.
// An error means the committed plan, a committed record or the working-tree plan could not be read or decoded; the Bouncer then reviews for real.
//
// The inputs are not Go-only: the rework session writes the overview's first_card, and status commits sweep the rework directory,
// so an agent-written record.json can reach HEAD.
// Re-classifying the live cards bounds that: a stale or forged record can at most skip review of a generation whose live cards are themselves exempt.
// A plan rewritten by an operator goto Plan-Write has a first_card that matches no round, and never skips.
func PlanReviewSkippable(deps PRReworkDeps) (bool, error) {
	plan, err := ParseCommittedPlan(deps.PlanDir, deps.ReadCommitted)
	if err != nil {
		return false, fmt.Errorf("loomshed: plan-review skip: parse committed plan: %w", err)
	}
	rounds, err := committedRounds(deps)
	if err != nil {
		return false, fmt.Errorf("loomshed: plan-review skip: %w", err)
	}
	first := max(plan.FirstCard, 1)
	live, found := 0, false
	var rec roundRecord
	for _, r := range rounds {
		if r.record.FirstCard == first && r.number > live {
			live, rec, found = r.number, r.record, true
		}
	}
	if !found || rec.Class != ReworkClassExempt || !planparser.ReviewExempt(plan) {
		return false, nil
	}
	working, err := planparser.ParsePlan(deps.PlanDir)
	if err != nil {
		return false, fmt.Errorf("loomshed: plan-review skip: parse working-tree plan: %w", err)
	}
	return planparser.ReviewExempt(working), nil
}

// ArchivedWebsterDirs lists each round's archived webster directory under reworkDir, ascending by round number.
// A round whose prior-generation holds no webster directory is skipped, and an absent reworkDir lists none.
// Any other read or stat failure is an error, so a prior generation's record is never dropped silently.
func ArchivedWebsterDirs(reworkDir string) ([]string, error) {
	nums, err := roundNumbers(reworkDir)
	if err != nil {
		return nil, fmt.Errorf("loomshed: list archived webster run records: %w", err)
	}
	sort.Ints(nums)
	var dirs []string
	for _, n := range nums {
		dir := filepath.Join(reworkDir, reworkRoundPrefix+strconv.Itoa(n), reworkPriorDir, reworkPriorWebster)
		info, err := os.Stat(dir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			// This round archived no webster run record.
		case err != nil:
			return nil, fmt.Errorf("loomshed: stat archived webster run record %s: %w", dir, err)
		case info.IsDir():
			dirs = append(dirs, dir)
		}
	}
	return dirs, nil
}
