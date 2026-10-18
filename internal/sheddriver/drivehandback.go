// drivehandback.go holds what every shed-driver hand-back leaves behind:
// a Go-authored stop report, the park the recipe's Park seam performs, and the parent messages the LLM driver's two notify rules send.

package sheddriver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// driveNoticeTail ends every message the driver sends its parent.
const driveNoticeTail = "Answer briefly, then continue your task."

// driveAttemptNote is one tackler or arbiter attempt as the stop report and the hand-back record it.
type driveAttemptNote struct {
	// Role is the agent that judged the stop: TacklerRole or the arbiter seam's Role.
	Role        string
	Number      int
	Verdict     string
	Cause       string
	VerdictPath string
}

// driveHandBack describes one hand-back.
// Question is the parent question, and Relay the parent notice to relay verbatim.
// DoneFrictionFailed is set when a done run reported friction: failed, which notifies without parking.
type driveHandBack struct {
	RunID              string
	ReportPath         string
	Stop               Stop
	Rules              []string
	Attempts           []driveAttemptNote
	Question           string
	Relay              string
	DoneFrictionFailed bool
}

// driveNotices returns the messages a hand-back sends its parent, each ending in driveNoticeTail.
// Watched, a batten announces the run and retries a matching verify halt itself, so only the parent notice, a question and a friction-failed done are sent.
// Unwatched, the same are sent, plus one line naming the run-id and the stop report at every other hand-back; a done stop sends nothing else.
func driveNotices(h driveHandBack, watched bool) []string {
	var notices []string
	if h.Relay != "" {
		notices = append(notices, h.Relay+" "+driveNoticeTail)
	}
	if h.Question != "" {
		notices = append(notices, fmt.Sprintf("Run %s stopped with a question; stop report %s. Question: %s %s", h.RunID, h.ReportPath, h.Question, driveNoticeTail))
	}
	if h.DoneFrictionFailed {
		notices = append(notices, fmt.Sprintf("Run %s is done but reported friction: failed; stop report %s. %s", h.RunID, h.ReportPath, driveNoticeTail))
	}
	done := h.Stop.Key().State == string(shedengine.StateDone)
	if !watched && len(notices) == 0 && !done {
		notices = append(notices, fmt.Sprintf("Run %s stopped; stop report %s. %s", h.RunID, h.ReportPath, driveNoticeTail))
	}
	return notices
}

// driveReport is the content of a stop report.
// Progress is the step's progress as the envelope reported it, and Sends holds one line per send, declined send, failed send or unqueued send.
type driveReport struct {
	RunID         string
	At            time.Time
	HistoryLength int
	Progress      string
	Reason        string
	Rules         []string
	Attempts      []driveAttemptNote
	Question      string
	FrictionDir   string
	RepairDir     string
	Sends         []string
}

// render writes the report as markdown, one fact per line; an empty friction directory reads none.
func (r driveReport) render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Drive report for run `%s`\n\n", r.RunID)
	fmt.Fprintf(&b, "- Written: %s\n", r.At.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Position: history length %d, progress %q\n", r.HistoryLength, r.Progress)
	fmt.Fprintf(&b, "- Reason: %s\n", r.Reason)
	fmt.Fprintf(&b, "- Friction directory: %s\n", pathOrNone(r.FrictionDir))
	fmt.Fprintf(&b, "- Repair directory: %s\n", r.RepairDir)
	b.WriteString("\n## Rules applied\n\n")
	for _, rule := range r.Rules {
		fmt.Fprintf(&b, "- %s\n", rule)
	}
	b.WriteString("\n## Attempts\n\n")
	for _, attempt := range r.Attempts {
		fmt.Fprintf(&b, "- %s attempt %d: verdict %s", attempt.Role, attempt.Number, attempt.Verdict)
		if attempt.Cause != "" {
			fmt.Fprintf(&b, ", cause %s", attempt.Cause)
		}
		fmt.Fprintf(&b, ", verdict file %s\n", attempt.VerdictPath)
	}
	if r.Question != "" {
		fmt.Fprintf(&b, "\n## Question for the run's parent\n\n%s\n", r.Question)
	}
	b.WriteString("\n## Messages to the parent\n\n")
	for _, send := range r.Sends {
		fmt.Fprintf(&b, "- %s\n", send)
	}
	return b.String()
}

// HandBackDeps is every seam and fixed value a hand-back reads.
type HandBackDeps struct {
	// ReportPath is where the stop report is written; the driver replaces it with a fresh path for each hand-back.
	ReportPath string
	// Notify queues one line for the run's parent at stop; sent is false, with a nil error, when the recipe declines to send a notice at this stop.
	Notify func(stop Stop, line string) (sent bool, err error)
	// OrchRecorded reports whether an orch strand is recorded to receive a notice.
	OrchRecorded func() (bool, error)
	// Park records the stop report at reportPath as the run's park; an error means the park could not be recorded.
	Park func(reportPath string) error
	// Now is the clock.
	Now func() time.Time
}

// handBackRun sends the hand-back's notices, writes the stop report and parks the run.
// Each notice goes through Notify when an orch strand is recorded; otherwise the line is logged and posted nowhere, and the report alone carries it.
// Every send, declined send, failed send and unqueued line is one line of the report.
// The report is written at ReportPath, creating its directory, and a write failure is returned.
// A hand-back describing a friction-failed done stops there, so a finished run is never parked.
// Otherwise Park is called after the report is written, and its error is returned.
// Releasing the driver lock and exiting are the caller's.
func handBackRun(d HandBackDeps, h driveHandBack, report driveReport, watched bool) error {
	report.At = d.Now()
	if notices := driveNotices(h, watched); len(notices) > 0 {
		orchRecorded, err := d.OrchRecorded()
		if err != nil {
			logger.Warn("sheddriver: drive: orch strand lookup failed; queueing no notice", "run", h.RunID, "error", err)
		}
		for _, notice := range notices {
			if err != nil || !orchRecorded {
				logger.Info("sheddriver: drive: no orch strand recorded; notice posted nowhere", "run", h.RunID, "notice", notice)
				report.Sends = append(report.Sends, "not queued, no orch strand recorded: "+notice)
				continue
			}
			sent, sendErr := d.Notify(h.Stop, notice)
			switch {
			case sendErr != nil:
				logger.Warn("sheddriver: drive: notice send failed", "run", h.RunID, "error", sendErr)
				report.Sends = append(report.Sends, fmt.Sprintf("send failed (%v): %s", sendErr, notice))
			case !sent:
				report.Sends = append(report.Sends, "not sent, the recipe sends none at this stop: "+notice)
			default:
				report.Sends = append(report.Sends, "sent: "+notice)
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(d.ReportPath), 0o755); err != nil {
		return fmt.Errorf("sheddriver: drive: create the stop report directory: %w", err)
	}
	if err := os.WriteFile(d.ReportPath, []byte(report.render()), 0o644); err != nil {
		return fmt.Errorf("sheddriver: drive: write the stop report %s: %w", d.ReportPath, err)
	}
	if h.DoneFrictionFailed {
		return nil
	}
	return d.Park(d.ReportPath)
}
