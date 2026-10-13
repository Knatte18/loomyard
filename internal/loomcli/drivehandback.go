// drivehandback.go holds what every Go-driver hand-back leaves behind:
// a Go-authored stop report, the run records committed and the park marker written by the same code `lyx loom commit-records --park` runs, and the parent messages the LLM driver's two notify rules send.

package loomcli

import (
	"bytes"
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
	// Role is the agent that judged the stop: loomengine.TacklerRole or loomengine.ArbiterRole.
	Role        string
	Number      int
	Verdict     string
	Cause       string
	VerdictPath string
}

// driveHandBack describes one hand-back.
// Question is the parent question, and Relay the parent notice to relay verbatim.
// DoneFrictionFailed is set when a done run reported friction: failed, which notifies without parking.
// WatchedVerifyLine is set when a watching batten owns a matching verify halt, so the parent still hears of it.
type driveHandBack struct {
	RunID              string
	ReportPath         string
	Stop               driveStop
	Rules              []string
	Attempts           []driveAttemptNote
	Question           string
	Relay              string
	DoneFrictionFailed bool
	WatchedVerifyLine  bool
}

// driveNotices returns the messages a hand-back sends its parent, each ending in driveNoticeTail.
// Watched, a batten announces the run, so only the parent notice, a question, a friction-failed done and a watched verify halt are sent.
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
	done := h.Stop.key().State == string(shedengine.StateDone)
	if (watched && h.WatchedVerifyLine) || (!watched && len(notices) == 0 && !done) {
		notices = append(notices, fmt.Sprintf("Run %s stopped; stop report %s. %s", h.RunID, h.ReportPath, driveNoticeTail))
	}
	return notices
}

// driveReport is the content of a stop report.
// Progress is the step's progress as the envelope reported it, and Sends holds one line per send, failed send or unqueued send.
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

// render writes the report as markdown, one fact per line.
func (r driveReport) render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Drive report for run `%s`\n\n", r.RunID)
	fmt.Fprintf(&b, "- Written: %s\n", r.At.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Position: history length %d, progress %q\n", r.HistoryLength, r.Progress)
	fmt.Fprintf(&b, "- Reason: %s\n", r.Reason)
	fmt.Fprintf(&b, "- Friction directory: %s\n", r.FrictionDir)
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

// driveParkDeps is every seam and fixed value a hand-back reads.
type driveParkDeps struct {
	// ReportPath is where the stop report is written, a fresh driverReportPath.
	ReportPath string
	// Commit commits and pushes the run's records.
	Commit commitStatusDeps
	// MarkerPath is the driver park marker's path.
	MarkerPath string
	// Notify queues one line for the run's parent.
	Notify func(line string) error
	// OrchRecorded reports whether an orch strand is recorded to receive a notice.
	OrchRecorded func() (bool, error)
	// Now is the clock.
	Now func() time.Time
}

// handBackRun sends the hand-back's notices, writes the stop report and parks the run.
// Each notice goes through Notify when an orch strand is recorded; otherwise the line is logged and queued nowhere, and the report alone carries it.
// Every send, failed send and unqueued line is one line of the report.
// The report is written at ReportPath, creating its directory, and a write failure is returned.
// A hand-back describing a friction-failed done stops there, so a finished run is never parked.
// Otherwise commitAndPark commits the records and writes the park marker; a commit failure is logged and the marker is still written, and only a marker that could not be written is returned.
// Releasing the driver lock and exiting are the caller's.
func handBackRun(d driveParkDeps, h driveHandBack, report driveReport, watched bool) error {
	report.At = d.Now()
	if notices := driveNotices(h, watched); len(notices) > 0 {
		orchRecorded, err := d.OrchRecorded()
		if err != nil {
			logger.Warn("loom: drive: orch strand lookup failed; queueing no notice", "run", h.RunID, "error", err)
		}
		for _, notice := range notices {
			switch {
			case err != nil || !orchRecorded:
				logger.Info("loom: drive: no orch strand recorded; notice queued nowhere", "run", h.RunID, "notice", notice)
				report.Sends = append(report.Sends, "not queued, no orch strand recorded: "+notice)
			default:
				if sendErr := d.Notify(notice); sendErr != nil {
					logger.Warn("loom: drive: notice send failed", "run", h.RunID, "error", sendErr)
					report.Sends = append(report.Sends, fmt.Sprintf("send failed (%v): %s", sendErr, notice))
				} else {
					report.Sends = append(report.Sends, "sent: "+notice)
				}
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(d.ReportPath), 0o755); err != nil {
		return fmt.Errorf("loom: drive: create the stop report directory: %w", err)
	}
	if err := os.WriteFile(d.ReportPath, []byte(report.render()), 0o644); err != nil {
		return fmt.Errorf("loom: drive: write the stop report %s: %w", d.ReportPath, err)
	}
	if h.DoneFrictionFailed {
		return nil
	}
	var envelope bytes.Buffer
	if code := commitAndPark(&envelope, d.Commit, d.MarkerPath, d.ReportPath); code != 0 {
		logger.Warn("loom: drive: commit of the run records failed or the park marker was not written", "run", h.RunID, "envelope", strings.TrimSpace(envelope.String()))
		if marker, err := os.ReadFile(d.MarkerPath); err != nil || string(marker) != d.ReportPath {
			return fmt.Errorf("loom: drive: the park marker %s was not written: %s", d.MarkerPath, strings.TrimSpace(envelope.String()))
		}
	}
	return nil
}
