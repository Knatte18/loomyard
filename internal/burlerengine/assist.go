// assist.go builds the review's assistant seat: an extra reviewer on the engine its model choice names.
// Its report is one more report the reviewer chair consolidates.
// It derives the assistant's report and end marker from the review path and the attempt token.
// It composes the instruction files; once the chair has ended, it copies a done report beside the review.

package burlerengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/lenses"
	"github.com/Knatte18/loomyard/internal/lensreport"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// AssistSeatName is the name of the review table's assistant seat.
// Its strand role is the review role followed by -assist.
const AssistSeatName = "assist"

// The stencils an assisted round reads:
// the chair's stencil and consolidation part, the assistant's solo and fanned stencils, and its cluster-fan rules.
const (
	assistedReviewerStencil     = "burler-template-reviewer-assisted"
	assistReviewerStencil       = "burler-template-assist-reviewer"
	assistFannedReviewerStencil = "burler-template-assist-reviewer-fanned"
	assistConsolidateStencil    = "burler-assist-consolidation"
	assistClusterFanStencil     = "burler-assist-cluster-fan"
)

// assistRulesMarker is the review step's optional marker an assisted round fills with the chair's consolidation part.
const assistRulesMarker = "assist_rules"

// AssistOutcome is what became of one assistant seat of a review round.
// Ending is the text of its end marker, empty when none was written;
// Report is its own report and Copy the copy beside the review, set only when Ending is done.
// MissingLenses names, in fan order, each lens of a done fanned assistant that returned no lens report,
// so a solo run of a fanned seat cannot pass as a fanned one.
type AssistOutcome struct {
	Seat          string
	Ending        string
	Report        string
	Copy          string
	MissingLenses []string
}

// assisted reports whether opts names an assistant: a model choice with a model.
func (opts RunOpts) assisted() bool {
	return opts.Assist.Model != ""
}

// AssistCopyPath returns the path a done assistant's report is copied to once the chair has ended:
// `<review stem>.assist-<seat>.md` beside the review file.
func AssistCopyPath(reviewPath, seat string) string {
	stem := strings.TrimSuffix(filepath.Base(reviewPath), filepath.Ext(reviewPath))
	return filepath.Join(filepath.Dir(reviewPath), fmt.Sprintf("%s.assist-%s.md", stem, seat))
}

// assistPaths returns the assistant seat's report and end marker for the review at reviewPath and the attempt token.
// They are never tracked.
// When the review sits under the anchor's _lym, they sit under the anchor's .lym at the review path's subpath;
// otherwise they sit under the anchor's .lym burler directory.
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

// assistProfile returns the copy of p the assistant reviews under:
// its own report path and fan, with p's target, fasit, rubric, focus and prior rounds.
func assistProfile(p *Profile, report, fan string) Profile {
	assist := *p
	assist.ReviewPath = report
	assist.ClusterFan = fan
	assist.ClusterExclude, assist.ClusterExcludeHeld, assist.clusterLenses = nil, nil, nil
	assist.assistant = true
	return assist
}

// assistLensNames returns the lens names of the assistant's fan in fan order.
// A solo assistant has none, and so has a fan the lens library does not define, which keeps the assistant from starting.
func (e *Engine) assistLensNames(fan string) []string {
	if fan == "" {
		return nil
	}
	resolved, err := lenses.ResolveFan(e.cfg, fan)
	if err != nil {
		return nil
	}
	names := make([]string, len(resolved))
	for i, lens := range resolved {
		names[i] = lens.Name
	}
	return names
}

// assistSeat returns the assistant seat of p's review under opts, or nil when opts names none.
// files are the assistant's instruction file paths, and values its stencil values;
// the caller composes both, or passes a probe's.
// skipReason, when set, keeps the seat from starting, its end marker naming the reason.
// A fanned assistant runs the fanned stencil.
// Its lens reports are its subagent reports, so its directive and write surface name them.
func (e *Engine) assistSeat(p *Profile, opts RunOpts, files reviewFilePaths, skills []string, skipReason string) *seatengine.Seat {
	if !opts.assisted() {
		return nil
	}
	report, marker := assistPaths(e.geom.AnchorPath, p.ReviewPath, opts.Round, AssistSeatName)
	assist := assistProfile(p, report, opts.AssistFan)
	if opts.AssistSkip != "" && skipReason == "" {
		skipReason = opts.AssistSkip
	}
	seat := &seatengine.Seat{
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
	for _, lens := range e.assistLensNames(opts.AssistFan) {
		seat.SubagentReports = append(seat.SubagentReports, lensreport.Path(report, lens))
	}
	if len(seat.SubagentReports) > 0 {
		seat.Stencil = assistFannedReviewerStencil
		seat.Values["lens_report_path"] = lensreport.PathPattern(report)
	}
	return seat
}

// withAssistant returns table with seat added as its assistant:
// the chair runs the assisted stencil and reads the assistant's report as an input.
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

// composeAssist composes the assistant's instruction files for p's review under opts:
// it writes them into a fresh instruction directory, rendered for the assistant's engine.
// The assistant reads what the chair reads:
// the explore and review steps, fasit, rubric, PATTERN directive, focus and prior rounds.
// It uses its own report path and fan.
// It returns the files' paths and an empty reason.
// A missing engine block or an unknown fan gives placeholder paths and the reason the assistant cannot start;
// the step then goes on without it.
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

// unstartedAssist returns the reason the assistant cannot start, and placeholder file paths in the probe directory.
// The seat never starts, but the table validation rejects a blank instruction path.
// Erroring on that would fail the round instead of letting it go on without the assistant.
// The paths are placeholders: no file is written there, and nothing may read them.
func (e *Engine) unstartedAssist(err error) (reviewFilePaths, string) {
	return reviewFilesIn(e.probeDirOf()), notStartedReason(err)
}

// notStartedReason logs why the assistant cannot start and returns it as its end marker's reason.
func notStartedReason(err error) string {
	logger.Warn("burler: the assistant reviewer cannot start; the round goes on without it", "error", err)
	return err.Error()
}

// assistRulesBlock renders the chair's consolidation part of an assisted round for engine:
// the value of the review step's optional assist_rules marker.
// A round with no assistant returns "" without reading anything.
// The value leads with the blank line that separates it from the cluster rules;
// the stencil loader would drop that blank line from the stencil itself.
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

// keepAssistReports reads every assistant seat's end marker in seats and returns what became of each.
// It copies the report of a seat whose marker reads done beside the review file.
// Any other ending, gate failed included, copies nothing.
// A failed read or copy is logged and leaves that assistant's Copy empty, since the assistant never gates the round.
// lensNames are the lenses the assistant's fan asked of it;
// a done assistant that returned no report for one of them has it named on its outcome and logged,
// and the round never fails.
func keepAssistReports(reviewPath string, seats []seatengine.SeatResult, lensNames []string) []AssistOutcome {
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
			outcome.MissingLenses = missingLensReports(outcome.Report, lensNames)
		}
		if len(outcome.MissingLenses) > 0 {
			logger.Warn("burler: the assistant was asked to fan and returned no report for these lenses", "seat", seat.Name, "lenses", outcome.MissingLenses)
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes
}

// missingLensReports returns the names in lensNames that have no returned lens report beside report.
func missingLensReports(report string, lensNames []string) []string {
	var missing []string
	for _, lens := range lensNames {
		if !lensreport.Returned(lensreport.Path(report, lens)) {
			missing = append(missing, lens)
		}
	}
	return missing
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
