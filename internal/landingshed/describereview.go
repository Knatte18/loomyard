// describereview.go builds the Describe session's reviewer, an assistant seat that reviews the change description.
// It also builds the writer's gate over the review.
// That gate releases the reviewer, holds the writer's turn end until the review has ended and hands over its findings.
// A second gate covers the reviewer's own report.

package landingshed

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// describeReviewStencilName is the registered name of the Describe reviewer's prompt.
const describeReviewStencilName = "landing-template-describe-review"

// DescribeReviewSeatName is the name of the Describe table's reviewer seat; its strand role is describe-review.
const DescribeReviewSeatName = "review"

// The file names inside the told review directory:
// the reviewer's report and end marker, and the records of its release and of the delivery of its findings.
const (
	describeReviewReportName = "describe-review.md"
	describeReviewMarkerName = "describe-review.end"
	describeReviewReadyName  = "describe-review.ready"
	describeReviewSentName   = "describe-review.sent"
)

// The first line of the reviewer's report: its verdict on the description.
const (
	DescribeReviewApproved = "# Describe review: approved"
	DescribeReviewFindings = "# Describe review: findings"
)

// describeReviewGateName labels the writer's gate entry that waits on the reviewer and hands it its findings.
const describeReviewGateName = "Describe-Review"

// describeReviewGateAttempts is the re-prompt budget of the writer's review entry.
// The entry fails once at most.
// The budget is two because a failure that spends the whole budget re-prompts nothing.
const describeReviewGateAttempts = 2

// describeReviewCheckGap is the longest time between two checks of a pending review that still counts as one wait.
// A longer gap is a pause, a machine sleep or a turn of the writer, and the review's time starts over after it.
const describeReviewCheckGap = 2 * time.Minute

// describeReportGateAttempts is how many times a report with no verdict line re-prompts the reviewer.
const describeReportGateAttempts = 2

// describeReviewFiles are the paths of one review inside its review directory.
type describeReviewFiles struct {
	report string
	marker string
	// ready exists once the description has passed the writer's earlier gates.
	ready string
	// sent exists once the findings reached the writer.
	sent string
}

// describeReviewFilesIn returns the review's paths inside reviewDir.
func describeReviewFilesIn(reviewDir string) describeReviewFiles {
	return describeReviewFiles{
		report: filepath.Join(reviewDir, describeReviewReportName),
		marker: filepath.Join(reviewDir, describeReviewMarkerName),
		ready:  filepath.Join(reviewDir, describeReviewReadyName),
		sent:   filepath.Join(reviewDir, describeReviewSentName),
	}
}

// ResetDescribeReview removes what an earlier Describe session recorded about its review,
// so a table started fresh releases its reviewer again and may send findings again.
// A table with no reviewer seat, and a record already absent, are no error.
func ResetDescribeReview(table seatengine.Table) error {
	for _, seat := range table.Seats {
		if seat.Name != DescribeReviewSeatName {
			continue
		}
		files := describeReviewFilesIn(filepath.Dir(seat.EndMarker))
		for _, path := range []string{files.ready, files.sent} {
			if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("landingshed: remove the earlier describe review record %s: %w", path, err)
			}
		}
	}
	return nil
}

// describeReviewSeat returns the reviewer seat of the Describe table: an assistant that writes its report at report.
// It runs on landing.describe_review's model.
// Its budget is the sum of landing.describe_timeout_min and landing.describe_review_timeout_min.
// The first is there because its run covers its wait for the description.
// A writer that finishes within the first therefore leaves the review at least the second.
// A describe_timeout_min below 1 is the budget as it stands, with nothing added.
// An empty describe_review keeps it from starting, on the writer's model, with its end marker naming the key;
// writer is the writer's model choice.
// Its gate refuses a report whose first line is no verdict line.
func describeReviewSeat(cfg Config, reg modelspec.Registry, writer shuttleengine.ModelChoice, files describeReviewFiles) (seatengine.Seat, error) {
	// Zero asks for the shuttle's default and a negative value is refused at the start, so neither is added to.
	budgetMin := cfg.DescribeTimeoutMin
	if budgetMin > 0 {
		budgetMin += cfg.DescribeReviewTimeoutMin
	}
	seat := seatengine.Seat{
		Name:        DescribeReviewSeatName,
		Stencil:     describeReviewStencilName,
		ModelChoice: writer,
		Outputs:     []string{files.report},
		ConfigRole:  agentconfig.RoleDescriber,
		Values:      map[string]string{"ready_path": files.ready},
		Gate:        shuttleengine.GateSpec{{Name: "Describe-Review-Report", Gate: describeReportGate(files.report), Attempts: describeReportGateAttempts}},
		Assistant:   true,
		EndMarker:   files.marker,
		Timeout:     time.Duration(budgetMin) * time.Minute,
	}
	if cfg.DescribeReview == "" {
		seat.SkipReason = keyName(describeReviewKey) + " is empty, so no reviewer runs"
		return seat, nil
	}
	choice, err := resolveModelChoice(cfg.DescribeReview, reg)
	if err != nil {
		return seatengine.Seat{}, fmt.Errorf("%s model-spec: %w", keyName(describeReviewKey), err)
	}
	seat.ModelChoice = choice
	return seat, nil
}

// describeReportGate returns the reviewer's own gate over its report:
// it passes a report whose first non-blank line is the approved or the findings verdict line, and fails any other.
func describeReportGate(report string) shuttleengine.Gate {
	return func() (shuttleengine.GateResult, error) {
		content, err := os.ReadFile(report)
		if err != nil {
			return shuttleengine.GateResult{}, fmt.Errorf("landingshed: read the describe review %s: %w", report, err)
		}
		if verdict := reviewVerdict(content); verdict == DescribeReviewApproved || verdict == DescribeReviewFindings {
			return shuttleengine.GateResult{Passed: true}, nil
		}
		return shuttleengine.GateResult{Findings: fmt.Sprintf("the report %s does not open with %q or %q; way forward: make its first line one of them, the findings line followed by one item per finding", report, DescribeReviewApproved, DescribeReviewFindings)}, nil
	}
}

// reviewVerdict returns the first non-blank line of a review report, trimmed.
func reviewVerdict(content []byte) string {
	for _, line := range bytes.Split(content, []byte("\n")) {
		if trimmed := strings.TrimSpace(string(line)); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// describeReview is the writer's gate entry over the reviewer.
// It releases the reviewer and holds the writer's turn end until the reviewer's end marker exists.
// It then hands the writer the findings once.
// It is safe for concurrent use.
type describeReview struct {
	files   describeReviewFiles
	timeout time.Duration
	now     func() time.Time
	mu      sync.Mutex
	// waitingSince is when the current wait on the end marker began, and checkedAt when it was last checked.
	waitingSince time.Time
	checkedAt    time.Time
}

// describeReviewGateEntry returns the writer's gate entry over the reviewer whose files are given.
// The entry's first call writes the ready record, which the reviewer waits for before it reads the description.
// Placed behind the description's own check, it therefore releases the reviewer only on a text that passed.
// The entry pends while the end marker is missing, so the writer's turn end waits for the review.
// Past timeout of unbroken waiting, as now reads the time, the step goes on unreviewed;
// a re-attached run, and a check long after the one before it, start that time over.
// A marker that reads other than done passes: the step goes on without a review that did not finish.
// A done review that approves passes.
// One with findings fails for the writer alone, with the report's text as its findings, until they reach the writer.
// It passes from then on, also when the entry is built again over the same files.
// At the run's final verdict the entry passes, so a review still running never fails the step.
func describeReviewGateEntry(files describeReviewFiles, timeout time.Duration, now func() time.Time) shuttleengine.GateEntry {
	review := &describeReview{files: files, timeout: timeout, now: now}
	return shuttleengine.GateEntry{
		Name:      describeReviewGateName,
		Gate:      review.check,
		Final:     func() (shuttleengine.GateResult, error) { return shuttleengine.GateResult{Passed: true}, nil },
		Attempts:  describeReviewGateAttempts,
		PassOnCap: true,
	}
}

// check runs the entry once; describeReviewGateEntry states its verdicts.
func (r *describeReview) check() (shuttleengine.GateResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.releaseReviewer(); err != nil {
		return shuttleengine.GateResult{}, err
	}
	ending, found, err := seatengine.ReadEnding(r.files.marker)
	if err != nil {
		return shuttleengine.GateResult{}, err
	}
	if !found {
		now := r.now()
		if r.waitingSince.IsZero() || now.Sub(r.checkedAt) > describeReviewCheckGap {
			r.waitingSince = now
		}
		r.checkedAt = now
		if now.Sub(r.waitingSince) < r.timeout {
			return shuttleengine.GateResult{Pending: true}, nil
		}
		logger.Warn("landingshed: the describe review's time ran out, so the description goes on unreviewed", "marker", r.files.marker, "key", keyName(describeReviewTimeoutMinKey), "limit", r.timeout)
		return shuttleengine.GateResult{Passed: true}, nil
	}
	if ending != seatengine.EndingDone {
		logger.Info("landingshed: the describe review did not finish, so the description goes on unreviewed", "marker", r.files.marker, "ending", ending)
		return shuttleengine.GateResult{Passed: true}, nil
	}

	if _, err := os.Stat(r.files.sent); err == nil {
		return shuttleengine.GateResult{Passed: true}, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return shuttleengine.GateResult{}, fmt.Errorf("landingshed: stat the describe review's sent record %s: %w", r.files.sent, err)
	}
	content, err := os.ReadFile(r.files.report)
	if errors.Is(err, fs.ErrNotExist) {
		return shuttleengine.GateResult{Passed: true}, nil
	}
	if err != nil {
		return shuttleengine.GateResult{}, fmt.Errorf("landingshed: read the describe review %s: %w", r.files.report, err)
	}
	if reviewVerdict(content) == DescribeReviewApproved {
		return shuttleengine.GateResult{Passed: true}, nil
	}
	findings := "A reviewer read your change description against the diff and the scope and reports the findings below. " +
		"Weigh each against the diff, fix the description where a finding holds, run lym loom validate-description again, and end your turn.\n\n" + string(content)
	return shuttleengine.GateResult{Findings: findings, WriterOnly: true, Reprompted: r}, nil
}

// RecordReprompt implements shuttleengine.RepromptRecorder.
// It writes the sent record once the findings reached the writer.
// A failed write is logged, and the findings may then reach the writer a second time.
func (r *describeReview) RecordReprompt() {
	if err := writeBytesAtomic(r.files.sent, "describe-review-sent", []byte("sent\n")); err != nil {
		logger.Warn("landingshed: the describe review's sent record could not be written", "path", r.files.sent, "error", err)
	}
}

// releaseReviewer writes the ready record the reviewer waits for, unless it exists.
func (r *describeReview) releaseReviewer() error {
	_, err := os.Stat(r.files.ready)
	if errors.Is(err, fs.ErrNotExist) {
		return writeBytesAtomic(r.files.ready, "describe-review-ready", []byte("ready\n"))
	}
	if err != nil {
		return fmt.Errorf("landingshed: stat the describe review's ready record %s: %w", r.files.ready, err)
	}
	return nil
}
