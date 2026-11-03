// assist.go builds the review half's assistant seat: an extra reviewer on the engine its model choice names, whose report the reviewer chair consolidates as one more report.
// It derives the assistant's report and end marker from the review path and the attempt token, composes the assistant's own instruction files, and copies a done assistant's report beside the review once the chair has ended.

package burlerengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// AssistSeatName is the name of the review table's assistant seat, so its strand role is the review role followed by -assist.
const AssistSeatName = "assist"

// The stencils an assisted round reads: the chair's stencil, the assistant's own, the chair's consolidation part and the assistant's fanned cluster rules.
const (
	assistedReviewerStencil  = "burler-template-reviewer-assisted"
	assistReviewerStencil    = "burler-template-assist-reviewer"
	assistConsolidateStencil = "burler-assist-consolidation"
	assistClusterFanStencil  = "burler-assist-cluster-fan"
)

// assistRulesMarker is the review step's optional marker an assisted round fills with the chair's consolidation part.
const assistRulesMarker = "assist_rules"

// AssistOutcome is what became of one assistant seat of a review round.
// Ending is the text of its end marker, empty when none was written; Report is its own report and Copy the copy beside the review, set only when Ending is done.
type AssistOutcome struct {
	Seat   string
	Ending string
	Report string
	Copy   string
}

// assisted reports whether opts names an assistant: a model choice with a model.
func (opts RunOpts) assisted() bool {
	return opts.Assist.Model != ""
}

// AssistCopyPath returns the path a done assistant's report is copied to once the chair has ended: `<review stem>.assist-<seat>.md` beside the review file.
func AssistCopyPath(reviewPath, seat string) string {
	stem := strings.TrimSuffix(filepath.Base(reviewPath), filepath.Ext(reviewPath))
	return filepath.Join(filepath.Dir(reviewPath), fmt.Sprintf("%s.assist-%s.md", stem, seat))
}

// assistPaths returns the assistant seat's report and end marker for the review at reviewPath and the attempt token, never tracked:
// under the anchor's .lym at the review path's subpath when the review sits under the anchor's _lym, else under the anchor's .lym burler directory.
// Each name carries the review's stem, the token and the seat, so a retry's files are never the first attempt's.
func assistPaths(anchorPath, reviewPath, token, seat string) (report, marker string) {
	dir := filepath.Join(anchorPath, lymdirs.DotLymDirName, burlerDirName, "assist")
	if rel, err := filepath.Rel(filepath.Join(anchorPath, lymdirs.LymDirName), filepath.Dir(reviewPath)); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		dir = filepath.Join(anchorPath, lymdirs.DotLymDirName, rel)
	}
	stem := strings.TrimSuffix(filepath.Base(reviewPath), filepath.Ext(reviewPath))
	base := filepath.Join(dir, fmt.Sprintf("%s.attempt-%s.assist-%s", stem, token, seat))
	return base + ".md", base + ".end"
}

// assistProfile returns the copy of p the assistant reviews under: its own report path and fan, with p's target, fasit, rubric, focus and prior rounds.
func assistProfile(p *Profile, report, fan string) Profile {
	assist := *p
	assist.ReviewPath = report
	assist.ClusterFan = fan
	assist.ClusterExclude, assist.ClusterExcludeHeld, assist.clusterLenses = nil, nil, nil
	assist.assistant = true
	return assist
}

// assistSeat returns the assistant seat of p's review under opts, or nil when opts names none.
// files are the assistant's instruction file paths, and values its stencil values; the caller composes both, or passes a probe's.
// skipReason, when set, keeps the seat from starting, its end marker naming the reason.
func (e *Engine) assistSeat(p *Profile, opts RunOpts, files reviewFilePaths, skills []string, skipReason string) *seatengine.Seat {
	if !opts.assisted() {
		return nil
	}
	report, marker := assistPaths(e.geom.AnchorPath, p.ReviewPath, opts.Round, AssistSeatName)
	assist := assistProfile(p, report, opts.AssistFan)
	if opts.AssistSkip != "" && skipReason == "" {
		skipReason = opts.AssistSkip
	}
	return &seatengine.Seat{
		Name:        AssistSeatName,
		Stencil:     assistReviewerStencil,
		ModelChoice: opts.Assist,
		ModelKey:    opts.AssistKey,
		Outputs:     []string{report},
		Skills:      skills,
		Values:      reviewSeatValues(&assist, files),
		Gate:        shuttleengine.GateSpec{ReviewGateEntry(report)},
		Assistant:   true,
		EndMarker:   marker,
		SkipReason:  skipReason,
		Timeout:     opts.AssistTimeout,
	}
}

// withAssistant returns table with seat added as its assistant: the chair runs the assisted stencil and reads the assistant's report as an input.
// A nil seat returns table unchanged.
func withAssistant(table seatengine.Table, seat *seatengine.Seat) seatengine.Table {
	if seat == nil {
		return table
	}
	chair := table.Seats[0]
	chair.Stencil = assistedReviewerStencil
	chair.Inputs = append(append([]string{}, chair.Inputs...), seat.Outputs...)
	table.Seats = []seatengine.Seat{chair, *seat}
	return table
}

// composeAssist composes the assistant's instruction files for p's review under opts into a fresh instruction directory, rendered for the assistant's engine, and writes them.
// The assistant reads the same explore and review steps, fasit, rubric, PATTERN directive, focus and prior rounds the chair reads, with its own report path and fan.
// It returns the files' paths and an empty reason, or placeholder paths and the reason the assistant cannot start, which a missing engine block or an unknown fan gives; the step then goes on without it.
func (e *Engine) composeAssist(p *Profile, opts RunOpts, directive string) (reviewFilePaths, string) {
	report, _ := assistPaths(e.geom.AnchorPath, p.ReviewPath, opts.Round, AssistSeatName)
	assist := assistProfile(p, report, opts.AssistFan)
	if assist.ClusterFan != "" {
		resolved, err := resolveRecordedFan(e.cfg, assist.ClusterFan)
		if err != nil {
			return e.unstartedAssist(err)
		}
		assist.clusterLenses = resolved
	}
	dir, err := e.instructionDir("assist")
	if err != nil {
		return e.unstartedAssist(err)
	}
	files := reviewFilesIn(dir)
	prompt, err := composeReviewPrompt(e.stencilsDir, e.geom.SpecsDir, e.seatEngine(opts.Assist), &assist, directive, "", files)
	if err != nil {
		return e.unstartedAssist(err)
	}
	if err := writeInstructionFiles(prompt.Files); err != nil {
		return e.unstartedAssist(err)
	}
	return files, ""
}

// unstartedAssist returns what composeAssist answers when the assistant cannot start: the reason, and the probe directory's file paths.
// The seat never starts, but a table whose seat holds a blank instruction path fails its validation, which would error the round instead of letting it go on without the assistant.
// The paths are placeholders: no file is written there, and nothing may read them.
func (e *Engine) unstartedAssist(err error) (reviewFilePaths, string) {
	return reviewFilesIn(e.probeDirOf()), notStartedReason(err)
}

// notStartedReason logs why the assistant cannot start and returns it as its end marker's reason.
func notStartedReason(err error) string {
	logger.Warn("burler: the assistant reviewer cannot start; the round goes on without it", "error", err)
	return err.Error()
}

// assistRulesBlock renders the chair's consolidation part of an assisted round for engine, for the review step's optional assist_rules marker.
// A round with no assistant returns "" without reading anything.
// The value is led by the blank line that separates it from the cluster rules, which the stencil loader would drop from the stencil itself.
func assistRulesBlock(stencilsDir, engine string, opts RunOpts) (string, error) {
	if !opts.assisted() {
		return "", nil
	}
	rules, err := fillInstruction(stencilsDir, engine, assistConsolidateStencil, map[string]string{"assist_origin": "assist:" + AssistSeatName}, nil)
	if err != nil {
		return "", err
	}
	return "\n\n" + strings.TrimRight(string(rules), "\n"), nil
}

// keepAssistReports reads every assistant seat's end marker in seats and copies the report of one whose marker reads done beside the review file, returning what became of each.
// Any other ending, gate failed included, copies nothing.
// A failed read or copy is logged and leaves that assistant's Copy empty, since the assistant never gates the round.
func keepAssistReports(reviewPath string, seats []seatengine.SeatResult) []AssistOutcome {
	var outcomes []AssistOutcome
	for _, seat := range seats {
		if !seat.Assistant {
			continue
		}
		outcome := AssistOutcome{Seat: seat.Name}
		if len(seat.Outputs) > 0 {
			outcome.Report = seat.Outputs[0]
		}
		ending, _, err := seatengine.ReadEnding(seat.EndMarker)
		if err != nil {
			logger.Warn("burler: the assistant's end marker is unreadable; its report is not kept", "seat", seat.Name, "error", err)
		}
		outcome.Ending = ending
		if ending == seatengine.EndingDone && outcome.Report != "" {
			outcome.Copy = copyAssistReport(outcome.Report, AssistCopyPath(reviewPath, seat.Name))
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes
}

// copyAssistReport copies the report at from to to and returns to, or "" when the copy failed, which it logs.
func copyAssistReport(from, to string) string {
	content, err := os.ReadFile(from)
	if err == nil {
		err = fsx.AtomicWriteBytes(to, content)
	}
	if err != nil {
		logger.Warn("burler: the assistant's report was not copied beside the review", "report", from, "copy", to, "error", err)
		return ""
	}
	return to
}
