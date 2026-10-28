// fixreview.go implements the fix review: the one solo session Finalize spawns to judge a merge fixer's change that is over the caps, reading only the fixer's commits, and ParseFixReviewReport, the one reader of the report format contracts/specs/landing-fix-review-spec.md defines.

package landingshed

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// fixReviewStencilName is the registered name of the fix review's prompt.
const fixReviewStencilName = "landing-template-fix-review"

// fixReviewRole is the agent-name role the fix review's spawn carries.
const fixReviewRole = "fix-reviewer"

// fixReviewGateName labels the fix review session's one gate entry, which re-prompts a session whose report does not parse.
const fixReviewGateName = "fix-review-report"

// fixReviewReprompts is how many times the session is re-prompted over a missing or unparseable report.
const fixReviewReprompts = 1

// fixReviewSpecName is the spec every fix review report error points the reader to.
const fixReviewSpecName = "landing-fix-review-spec.md"

// FixReviewVerdict is the verdict on a fix review report's first line.
type FixReviewVerdict string

const (
	// FixReviewApproved is the verdict of a review that found the fixer's change fixes the failure and keeps every test's meaning.
	FixReviewApproved FixReviewVerdict = "approved"
	// FixReviewRejected is the verdict of a review that found the change weakens or dodges a test, or is wrong in itself.
	FixReviewRejected FixReviewVerdict = "rejected"
)

// FixReviewReport is the parsed form of a fix review's report.
// Findings holds one entry per `## Findings` bullet with the bullet marker removed.
type FixReviewReport struct {
	Verdict  FixReviewVerdict
	Findings []string
}

// ParseFixReviewReport reads the fix review report at path.
// A missing file is an error wrapping fs.ErrNotExist.
// A first line that is no known verdict, and a rejected report with no finding, are errors naming the line and the spec.
func ParseFixReviewReport(path string) (FixReviewReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FixReviewReport{}, fmt.Errorf("landingshed: read fix review report %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")
	first := strings.TrimSpace(lines[0])
	verdict := FixReviewVerdict(strings.TrimPrefix(first, fixerVerdictPrefix))
	if !strings.HasPrefix(first, fixerVerdictPrefix) || (verdict != FixReviewApproved && verdict != FixReviewRejected) {
		return FixReviewReport{}, fmt.Errorf("landingshed: fix review report %s: line 1 %q is no known verdict; the first line is %q or %q, per %s",
			path, first, fixerVerdictPrefix+string(FixReviewApproved), fixerVerdictPrefix+string(FixReviewRejected), fixReviewSpecName)
	}

	report := FixReviewReport{Verdict: verdict}
	inFindings := false
	for _, line := range lines[1:] {
		if heading, ok := strings.CutPrefix(line, "## "); ok {
			inFindings = strings.TrimSpace(heading) == "Findings"
			continue
		}
		if bullet, ok := strings.CutPrefix(strings.TrimSpace(line), "- "); ok && inFindings {
			report.Findings = append(report.Findings, bullet)
		}
	}
	if verdict == FixReviewRejected && len(report.Findings) == 0 {
		return FixReviewReport{}, fmt.Errorf("landingshed: fix review report %s: a rejected report needs at least one \"## Findings\" bullet, per %s", path, fixReviewSpecName)
	}
	return report, nil
}

// fixReviewInputs carries the told values one fix review session needs.
// Every field is required except FixerReportPath and LanguageSkills.
type fixReviewInputs struct {
	// StencilsDir is the absolute directory the landing stencils are read from.
	StencilsDir string
	// DecisionRecordPath is the run's decision record, which the agent reads.
	DecisionRecordPath string
	// FixerReportPath is the merge fixer's last report; empty when none is on disk.
	FixerReportPath string
	// Baseline and Head bound the fixer's commits, the only range the agent reviews.
	Baseline string
	Head     string
	// CapReason is why the change is over the caps.
	CapReason string
	// ReportPath is the file the agent writes: its report.
	ReportPath string
	// TaskBranch is the branch under review.
	TaskBranch string
	// ParentBranch is the branch the task lands on.
	ParentBranch string
	// Slug is the task's slug.
	Slug string
	// LanguageSkills are the run's language skills the session loads before its prompt.
	LanguageSkills []string
}

// fixReviewTable builds the seatengine.Table of the fix review session: one lead that reads the fixer's commits and writes the report at in.ReportPath.
// It runs on the merge fixer's model-spec and turn timeout.
// An empty required input field is an error naming the field, and so is a stencil missing from in.StencilsDir.
func fixReviewTable(in fixReviewInputs, cfg Config, reg modelspec.Registry) (seatengine.Table, error) {
	required := []struct{ name, value string }{
		{"StencilsDir", in.StencilsDir},
		{"DecisionRecordPath", in.DecisionRecordPath},
		{"Baseline", in.Baseline},
		{"Head", in.Head},
		{"CapReason", in.CapReason},
		{"ReportPath", in.ReportPath},
		{"TaskBranch", in.TaskBranch},
		{"ParentBranch", in.ParentBranch},
		{"Slug", in.Slug},
	}
	for _, r := range required {
		if r.value == "" {
			return seatengine.Table{}, fmt.Errorf("landingshed: fixReviewTable: %s is empty", r.name)
		}
	}
	if _, err := stencilstore.Read(in.StencilsDir, fixReviewStencilName); err != nil {
		return seatengine.Table{}, fmt.Errorf("landingshed: fixReviewTable: %w", err)
	}
	parsed, err := modelspec.Parse(cfg.Fixer)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("landingshed: fixReviewTable: fixer model-spec: %w", err)
	}
	resolved, err := reg.Resolve(parsed)
	if err != nil {
		return seatengine.Table{}, fmt.Errorf("landingshed: fixReviewTable: fixer model-spec: %w", err)
	}

	return seatengine.Table{
		RolePrefix: fixReviewRole,
		Segment:    segmentcolor.Landing,
		Timeout:    time.Duration(cfg.FixerTimeoutMin) * time.Minute,
		Values: map[string]string{
			"slug":                 in.Slug,
			"decision_record_path": in.DecisionRecordPath,
			"fixer_report_path":    in.FixerReportPath,
			"baseline":             in.Baseline,
			"head":                 in.Head,
			"cap_reason":           in.CapReason,
			"report_path":          in.ReportPath,
			"task_branch":          in.TaskBranch,
			"parent_branch":        in.ParentBranch,
		},
		Optional: []string{"fixer_report_path"},
		Seats: []seatengine.Seat{{
			Stencil:     fixReviewStencilName,
			ModelChoice: shuttleengine.NewModelChoice(resolved),
			Outputs:     []string{in.ReportPath},
			Skills:      in.LanguageSkills,
		}},
	}, nil
}

// fixReviewEnd names how one fix review session ended.
type fixReviewEnd string

const (
	// fixReviewEndApproved is a report that approves the change.
	fixReviewEndApproved fixReviewEnd = "approved"
	// fixReviewEndRejected is a report that rejects the change.
	fixReviewEndRejected fixReviewEnd = "rejected"
	// fixReviewEndNoVerdict is a session that timed out or left no report that parses.
	fixReviewEndNoVerdict fixReviewEnd = "no-verdict"
	// fixReviewEndChanged is a session that left the tree dirty or moved the task head.
	fixReviewEndChanged fixReviewEnd = "changed"
)

// fixReviewOutcome is how a fix review session ended and what the reroute or halt that follows names.
type fixReviewOutcome struct {
	end fixReviewEnd
	// reportPath is the report the session writes.
	reportPath string
	// sessionLog is the session's run directory, which holds its log.
	sessionLog string
	// findings are the rejected report's findings.
	findings []string
	// cause is why a no-verdict or changed session ended so.
	cause string
}

// runFixReview runs the fix review over the fixer's commits, baseline..head, inside row, and returns how it ended.
// capReason is why the change is over the caps.
// A session that fails to start is an error marked transient.
func runFixReview(deps Deps, gate verifyGate, row, baseline, head, capReason string) (fixReviewOutcome, error) {
	reportPath := filepath.Join(deps.ScratchDir, row+"-fix-review.md")
	fixerReport := filepath.Join(deps.ScratchDir, row+"-fixer-report.md")
	if _, err := os.Stat(fixerReport); err != nil {
		fixerReport = ""
	}
	table, err := fixReviewTable(fixReviewInputs{
		StencilsDir:        deps.StencilsDir,
		DecisionRecordPath: deps.DecisionRecordPath,
		FixerReportPath:    fixerReport,
		Baseline:           baseline,
		Head:               head,
		CapReason:          capReason,
		ReportPath:         reportPath,
		TaskBranch:         deps.TaskBranch,
		ParentBranch:       deps.ParentBranch,
		Slug:               deps.Slug,
		LanguageSkills:     deps.LanguageSkills,
	}, deps.Config, deps.Registry)
	if err != nil {
		return fixReviewOutcome{}, fmt.Errorf("landingshed: %s: %w", row, err)
	}
	if err := archiveStaleReport(reportPath, gate.now(), row, "fix review"); err != nil {
		return fixReviewOutcome{}, err
	}
	table.Gate = shuttleengine.GateSpec{{Name: fixReviewGateName, Attempts: fixReviewReprompts, Gate: func() (shuttleengine.GateResult, error) {
		if _, err := ParseFixReviewReport(reportPath); err != nil {
			return shuttleengine.GateResult{Findings: err.Error()}, nil
		}
		return shuttleengine.GateResult{Passed: true}, nil
	}}}

	result, runErr := deps.FixerSeats.Run(table)
	if (runErr != nil && errors.Is(runErr, shuttleengine.ErrNotStarted)) || (runErr == nil && result.Chair.NotStarted) {
		cause := runErr
		if cause == nil {
			cause = errors.New(result.Chair.StartError)
		}
		return fixReviewOutcome{}, shedengine.MarkTransient(shedengine.TransientAgentStart, fmt.Errorf("landingshed: %s: the fix review session did not start: %w", row, cause))
	}
	if runErr != nil {
		return fixReviewOutcome{}, fmt.Errorf("landingshed: %s: run the fix review session: %w", row, runErr)
	}

	outcome := fixReviewOutcome{reportPath: reportPath, sessionLog: result.Chair.RunDir}
	defer func() {
		logger.Info("landingshed: fix review ended", "row", row, "end", string(outcome.end), "range", baseline+".."+head)
	}()
	dirty, err := gate.clean(row, "after the fix review")
	if err != nil {
		return fixReviewOutcome{}, err
	}
	after, err := deps.TaskHead()
	if err != nil {
		return fixReviewOutcome{}, fmt.Errorf("landingshed: %s: read the task head: %w", row, err)
	}
	switch {
	case dirty != "":
		// The halt names its own way forward: a commit would move the head past the range the review judged.
		outcome.end, outcome.cause = fixReviewEndChanged, strings.TrimSuffix(dirty, dirtyWayForward)
		return outcome, nil
	case after != head:
		outcome.end, outcome.cause = fixReviewEndChanged, fmt.Sprintf("the task head moved from %s to %s", head, after)
		return outcome, nil
	case result.Chair.Outcome == shuttleengine.OutcomeTimeout:
		outcome.end, outcome.cause = fixReviewEndNoVerdict, "the session ran past fixer_timeout_min"
		return outcome, nil
	}
	report, err := ParseFixReviewReport(reportPath)
	switch {
	case err != nil:
		outcome.end, outcome.cause = fixReviewEndNoVerdict, singleLine(err.Error())
	case report.Verdict == FixReviewApproved:
		outcome.end = fixReviewEndApproved
	default:
		outcome.end, outcome.findings = fixReviewEndRejected, report.Findings
	}
	return outcome, nil
}

// rerouteCause words what a rejected or verdict-less review adds to the reroute's cause.
func (o fixReviewOutcome) rerouteCause() string {
	if o.end == fixReviewEndRejected {
		return fmt.Sprintf("the fix review rejected it: %s; review: %s", singleLine(strings.Join(o.findings, "; ")), o.reportPath)
	}
	return fmt.Sprintf("the fix review gave no verdict (%s); session log: %s", o.cause, o.sessionLog)
}
