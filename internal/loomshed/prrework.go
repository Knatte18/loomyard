// prrework.go implements the PR-Rework row's producer: it wraps the gated rework session and does
// everything Go owns around it -- the append-only check of the plan the session extended, the round
// record committed beside the appended cards, the plan-fingerprint re-baseline, and the removal of
// the pending rejection.

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
	"strings"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

const (
	reworkRoundPrefix   = "round-"
	reworkFindingsFile  = "findings.md"
	reworkRecordFile    = "record.json"
	reworkCoverageFile  = "coverage.md"
	reworkRejectCommand = "lyx loom reject <review-file>"
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
	// ReadCommitted returns an anchor-relative file as committed at HEAD; found is false when HEAD has no such file.
	ReadCommitted func(anchorRel string) ([]byte, bool, error)
	// ReadRejection returns the pending rejection; found is false when none is recorded.
	ReadRejection func() (PendingRejection, bool, error)
	// ClearRejection removes the pending rejection record.
	ClearRejection func() error
	// Commit commits the appended cards and the round directory.
	Commit func() error
	// Rebaseline restamps the webster plan fingerprint over the extended plan.
	Rebaseline func() error
}

// roundRecord is the on-disk form of a round's record.json.
type roundRecord struct {
	PRNumber   int    `json:"pr_number"`
	HeadSHA    string `json:"head_sha"`
	RejectedAt string `json:"rejected_at"`
}

// prRework decorates inner, the gated rework session, with the Go-owned steps around it.
// It is a distinct type so the recipe shape test can tell the row apart.
type prRework struct {
	name  string
	inner shedengine.ShedProducer
	deps  PRReworkDeps
}

var _ shedengine.ShedProducer = (*prRework)(nil)

// NewPRRework returns the PR-Rework producer identified as name, wrapping inner.
func NewPRRework(name string, inner shedengine.ShedProducer, deps PRReworkDeps) shedengine.ShedProducer {
	return &prRework{name: name, inner: inner, deps: deps}
}

// Call implements shedengine.ShedProducer.
// A Commit, Rebaseline or ClearRejection failure is a returned error, never Stuck,
// for the reason planWrite.Call gives: re-running the session cannot fix a git or filesystem fault.
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

	committed, err := p.committedRoundHeads()
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

	if committed[pending.HeadSHA] {
		return p.finish(true)
	}

	outcome, pointer, err := p.inner.Call(ctx)
	if err != nil || outcome != shedengine.Done {
		return outcome, pointer, err
	}

	if reason, err := p.appendOnlyViolation(); err != nil {
		return "", shedengine.OutputPointer{}, err
	} else if reason != "" {
		return stuck(reason)
	}

	if err := p.writeRound(pending, pointer.Path); err != nil {
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
		{"ReadCommitted", p.deps.ReadCommitted == nil},
		{"ReadRejection", p.deps.ReadRejection == nil},
		{"ClearRejection", p.deps.ClearRejection == nil},
		{"Commit", p.deps.Commit == nil},
		{"Rebaseline", p.deps.Rebaseline == nil},
	}
	for _, s := range seams {
		if s.missing {
			return fmt.Errorf("loomshed: %s: no %s seam wired", p.name, s.name)
		}
	}
	return nil
}

// finish is steps 6: re-baseline, then (when clear) remove the pending record, then Done.
func (p *prRework) finish(clear bool) (shedengine.Outcome, shedengine.OutputPointer, error) {
	if err := p.deps.Rebaseline(); err != nil {
		return "", shedengine.OutputPointer{}, fmt.Errorf("loomshed: %s: re-baseline plan fingerprint: %w", p.name, err)
	}
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

// roundNumbers lists the N of every round-<N> directory under ReworkDir; an absent directory lists none.
func (p *prRework) roundNumbers() ([]int, error) {
	entries, err := os.ReadDir(p.deps.ReworkDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("loomshed: %s: list rework rounds: %w", p.name, err)
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

// committedRoundHeads returns the set of head_sha values recorded by rounds committed at HEAD.
func (p *prRework) committedRoundHeads() (map[string]bool, error) {
	nums, err := p.roundNumbers()
	if err != nil {
		return nil, err
	}
	heads := make(map[string]bool)
	for _, n := range nums {
		rel := path.Join(p.deps.ReworkDirRel, reworkRoundPrefix+strconv.Itoa(n), reworkRecordFile)
		data, ok, err := p.deps.ReadCommitted(rel)
		if err != nil {
			return nil, fmt.Errorf("loomshed: %s: read committed %s: %w", p.name, rel, err)
		}
		if !ok {
			continue
		}
		var rec roundRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return nil, fmt.Errorf("loomshed: %s: decode committed %s: %w", p.name, rel, err)
		}
		heads[rec.HeadSHA] = true
	}
	return heads, nil
}

// appendOnlyViolation compares the plan committed at HEAD with the working tree and returns the
// joined violations, or "" when the working tree is the base plus appended cards.
func (p *prRework) appendOnlyViolation() (string, error) {
	base, err := planparser.ParsePlanFrom(p.deps.PlanDir, func(name string) ([]byte, error) {
		rel := path.Join(planparser.PlanDirRel(), name)
		data, ok, err := p.deps.ReadCommitted(rel)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("%s not committed at HEAD: %w", rel, fs.ErrNotExist)
		}
		return data, nil
	})
	if err != nil {
		return "", fmt.Errorf("loomshed: %s: parse the committed plan: %w", p.name, err)
	}
	extended, err := planparser.ParsePlan(p.deps.PlanDir)
	if err != nil {
		return stuckReasonParse(err), nil
	}
	violations := planparser.CheckAppendOnly(base, extended)
	if len(violations) == 0 {
		return "", nil
	}
	return "rework plan is not append-only: " + strings.Join(violations, "; "), nil
}

// stuckReasonParse words a working-tree parse failure as the Stuck reason: the session left an unparseable plan, which a retry can fix.
func stuckReasonParse(err error) string {
	return "rework plan does not parse: " + err.Error()
}

// writeRound writes the round directory for pending: findings.md, record.json and coverage.md copied from coveragePath.
func (p *prRework) writeRound(pending PendingRejection, coveragePath string) error {
	n, err := p.roundFor(pending.HeadSHA)
	if err != nil {
		return err
	}
	dir := filepath.Join(p.deps.ReworkDir, reworkRoundPrefix+strconv.Itoa(n))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("loomshed: %s: create round directory: %w", p.name, err)
	}

	coverage, err := os.ReadFile(coveragePath)
	if err != nil {
		return fmt.Errorf("loomshed: %s: read coverage file: %w", p.name, err)
	}
	record, err := json.MarshalIndent(roundRecord{PRNumber: pending.PRNumber, HeadSHA: pending.HeadSHA, RejectedAt: pending.RejectedAt}, "", "  ")
	if err != nil {
		return fmt.Errorf("loomshed: %s: encode round record: %w", p.name, err)
	}
	files := []struct {
		name string
		data []byte
	}{
		{reworkFindingsFile, []byte(pending.Findings)},
		{reworkRecordFile, append(record, '\n')},
		{reworkCoverageFile, coverage},
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.name), f.data, 0o644); err != nil {
			return fmt.Errorf("loomshed: %s: write round %s: %w", p.name, f.name, err)
		}
	}
	return nil
}

// roundFor returns the working-tree round whose record.json already carries headSHA (a crash before
// the commit), else the highest round plus one.
func (p *prRework) roundFor(headSHA string) (int, error) {
	nums, err := p.roundNumbers()
	if err != nil {
		return 0, err
	}
	highest := 0
	for _, n := range nums {
		if n > highest {
			highest = n
		}
		data, err := os.ReadFile(filepath.Join(p.deps.ReworkDir, reworkRoundPrefix+strconv.Itoa(n), reworkRecordFile))
		if err != nil {
			continue
		}
		var rec roundRecord
		if json.Unmarshal(data, &rec) == nil && rec.HeadSHA == headSHA {
			return n, nil
		}
	}
	return highest + 1, nil
}
