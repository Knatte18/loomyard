// campaign_rows.go holds Campaign, the bundle of one run's config and seams that the Harden rows are built from.
// It also renders the module brief of the Harden-Bouncer's seed and the facts its judge reads about a round.
// It answers whether a round's live gate is red.

package hardenshed

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// fixerReportFileFormat names a round's fixer report in the run directory.
const fixerReportFileFormat = "round-%d-fixer-report.md"

// Campaign is one run's loaded hardener config and the seams its Harden rows act through.
// Every row of the run holds the same Campaign, and each method takes the run directory its row works in.
// A caller builds it with every field set, bar the seams a method it never reaches uses.
type Campaign struct {
	// Config is the run's loaded hardener config.
	Config Config
	// WorktreeRoot is the absolute root of the run's worktree, which the live gate reads the campaign's packages under.
	WorktreeRoot string
	// AnchorPath is the absolute anchor path of the run's worktree, which the round archive lands under.
	AnchorPath string
	// DecisionRecordPath is the absolute path of the run's decision record, which holds the operator's ask.
	DecisionRecordPath string
	// RunDir is the run directory the Harden rows share, which a halt writes its strays into.
	RunDir string
	// RunLive makes one `lym gate test` call of the live gate key, with the copies and packages, in its own slots.
	RunLive func(key string, copies int, packages []string) (LiveRun, error)
	// Reap reaps the sandbox at point for row and returns the strays it found.
	Reap func(row string, point ReapPoint) ([]StrayEntry, error)
	// Head returns the commit the worktree is at.
	Head func() (string, error)
	// Commits lists the commits made past from.
	Commits func(from string) ([]string, error)
	// Commit commits the anchor-relative paths of a round archive.
	Commit func(relPaths []string, message string) error
	// Entry reads the title and labels of the run's board entry, which name the module when the config names no package.
	Entry func() (title string, labels []string, err error)
}

// SeedBrief renders the module brief of the Harden-Bouncer's seed: which modules the campaign hardens.
// A config naming packages lists them as module directories under the worktree root.
// A config naming none gives the board entry's title and labels instead, for the seed to find the modules from.
// An error reading the board entry is returned.
func (c *Campaign) SeedBrief() (string, error) {
	if len(c.Config.Packages) > 0 {
		return fmt.Sprintf("The hardener config names the module directories the campaign hardens, relative to the worktree root `%s`: %s.",
			c.WorktreeRoot, backtickList(c.Config.Packages)), nil
	}
	title, labels, err := c.Entry()
	if err != nil {
		return "", fmt.Errorf("read the run's board entry: %w", err)
	}
	labelText := "none"
	if len(labels) > 0 {
		labelText = backtickList(labels)
	}
	return fmt.Sprintf("The hardener config names no module directory. The task's title is %q and its labels are %s: find the module or modules they name under the worktree root `%s`.",
		title, labelText, c.WorktreeRoot), nil
}

// backtickList returns items as a comma-separated list, each in backticks.
func backtickList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = "`" + item + "`"
	}
	return strings.Join(quoted, ", ")
}

// LiveGate returns the live gate of the fix row that works in runDir.
// Its round is the round the review row last wrote there.
func (c *Campaign) LiveGate(runDir string) shuttleengine.Gate {
	return NewLiveGate(LiveDeps{
		RunDir:       runDir,
		WorktreeRoot: c.WorktreeRoot,
		Round:        func() (int, error) { return shedadapters.ReviewRound(runDir) },
		Packages:     c.Config.Packages,
		LLMDriving:   c.Config.LLMDriving,
		Concurrency:  c.Config.Concurrency,
		Run:          c.RunLive,
	})
}

// WrapRow decorates the Harden round row inner, named row and of the given kind, with the campaign's reaps and files.
func (c *Campaign) WrapRow(inner shedengine.ShedProducer, row string, kind RowKind, runDir string) shedengine.ShedProducer {
	return WrapRow(inner, RowDeps{
		Row:     row,
		Kind:    kind,
		RunDir:  runDir,
		Reap:    c.Reap,
		Models:  c.models,
		Head:    c.Head,
		Commits: c.Commits,
		Archive: c.archiveDeps(runDir),
	})
}

// models returns the reviewer and fixer model-specs round runs, each list taken in turn and cycled.
func (c *Campaign) models(round int) (review, fix string) {
	return cycled(c.Config.Reviewer, round), cycled(c.Config.Fixer, round)
}

// cycled returns the entry of list that round takes, "" for an empty list.
func cycled(list []string, round int) string {
	if len(list) == 0 {
		return ""
	}
	return Rotate(list, round)[round-1]
}

// archiveDeps returns the round archive's seams over runDir.
func (c *Campaign) archiveDeps(runDir string) ArchiveDeps {
	return ArchiveDeps{RunDir: runDir, AnchorRoot: c.AnchorPath, Commit: c.Commit}
}

// LiveRed reports whether the live gate of round is red.
// A round with no live report is red once its fix row ran, which the round's commits file shows:
// a fix row whose earlier gate ran out returns before its live gate writes a report.
func (c *Campaign) LiveRed(runDir string, round int) (bool, error) {
	report, found, err := ReadLiveReport(runDir, round)
	if err != nil {
		return false, fmt.Errorf("read the live report of round %d: %w", round, err)
	}
	if found {
		return report.Mark == MarkRed, nil
	}
	_, fixRan, err := readRoundRecord[RoundCommits](runDir, "commits", round)
	if err != nil {
		return false, err
	}
	return fixRan, nil
}

// Halt reaps the sandbox at ReapHalt and adds the strays to the stray report of the run directory's last round.
func (c *Campaign) Halt(row string) error {
	strays, reapErr := c.Reap(row, ReapHalt)
	var failures []error
	if reapErr != nil {
		failures = append(failures, fmt.Errorf("reap at %s for %s: %w", ReapHalt, row, reapErr))
	}
	round, err := shedadapters.ReviewRound(c.RunDir)
	if err == nil {
		err = appendStrays(c.RunDir, round, strays)
	}
	if err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

// ArchiveFinal copies the run directory's latest Bouncer ledger into the harden directory and commits it.
func (c *Campaign) ArchiveFinal(runDir string) error {
	return ArchiveFinalLedger(c.archiveDeps(runDir))
}

// RoundFacts renders the Hardener section of round's facts, one bullet per fact.
// It opens with the path of the round's focus file, which holds the charter.
// It then names the decision record, which holds the operator's ask.
// The judge so carries the charter forward and keeps to the operator's focus.
// A file the round lacks is named as absent.
// A live report that is absent is also named red when LiveRed reads it red.
func (c *Campaign) RoundFacts(runDir string, round int) (string, error) {
	var facts strings.Builder

	fmt.Fprintf(&facts, "- Charter: `%s`, the focus file this round ran under; the next focus file restates its standing sections.\n",
		shedadapters.FocusPath(runDir, round))
	fmt.Fprintf(&facts, "- Operator's ask: `%s`; the focus its `## Goal` states, as its `Added after Discussion` entries amend it, binds the next focus file.\n",
		c.DecisionRecordPath)

	models, found, err := readRoundRecord[RoundModels](runDir, "models", round)
	if err != nil {
		return "", err
	}
	if found {
		fmt.Fprintf(&facts, "- Models: reviewer `%s`, fixer `%s`.\n", models.Review, models.Fix)
	} else {
		facts.WriteString("- Models: absent.\n")
	}

	live, liveFound, err := ReadLiveReport(runDir, round)
	if err != nil {
		return "", fmt.Errorf("read the live report of round %d: %w", round, err)
	}
	red, err := c.LiveRed(runDir, round)
	if err != nil {
		return "", err
	}
	writeLiveFacts(&facts, live, liveFound, red)

	if err := writeStrayFacts(&facts, runDir, round); err != nil {
		return "", err
	}
	commits, found, err := readRoundRecord[RoundCommits](runDir, "commits", round)
	if err != nil {
		return "", err
	}
	switch {
	case !found:
		facts.WriteString("- Fix commits: absent.\n")
	case len(commits.Commits) == 0:
		facts.WriteString("- Fix commits: none.\n")
	default:
		fmt.Fprintf(&facts, "- Fix commits: %s.\n", strings.Join(commits.Commits, ", "))
	}

	if err := writeDeferredFacts(&facts, runDir, round, live); err != nil {
		return "", err
	}
	return facts.String(), nil
}

// writeLiveFacts writes the bullets of the round's live report, or names it absent.
func writeLiveFacts(facts *strings.Builder, live LiveReport, found, red bool) {
	switch {
	case found:
		fmt.Fprintf(facts, "- Live report: %s. The `%s` run %s and started %d test binaries.\n", live.Mark, LiveGateKey, passedWord(live.Live.Passed), live.Live.Started)
	case red:
		facts.WriteString("- Live report: absent, read as red because the round's fix row ran and wrote none.\n")
	default:
		facts.WriteString("- Live report: absent.\n")
	}
	if !found {
		return
	}
	switch {
	case live.Copies == nil:
		facts.WriteString("- Copies run: none.\n")
	case live.Copies.Refused != "":
		fmt.Fprintf(facts, "- Copies run: refused: %s\n", live.Copies.Refused)
	default:
		fmt.Fprintf(facts, "- Copies run: %s and started %d test binaries.\n", passedWord(live.Copies.Passed), live.Copies.Started)
	}
	if len(live.OutsideStrayCover) > 0 {
		fmt.Fprintf(facts, "- Packages outside the stray-state cover: %s.\n", strings.Join(live.OutsideStrayCover, ", "))
	}
}

// passedWord names a call's result as the facts file words it.
func passedWord(passed bool) string {
	if passed {
		return "passed"
	}
	return "failed"
}

// writeStrayFacts writes the bullets of the round's stray report, or names it absent.
func writeStrayFacts(facts *strings.Builder, runDir string, round int) error {
	report, found, err := readRoundRecord[StrayReport](runDir, "strays", round)
	if err != nil {
		return err
	}
	switch {
	case !found:
		facts.WriteString("- Strays: the stray report is absent.\n")
	case len(report.Entries) == 0:
		facts.WriteString("- Strays: none.\n")
	default:
		for _, entry := range report.Entries {
			fmt.Fprintf(facts, "- Stray %s found by row %s at %s%s.\n", describeStray(entry), entry.Row, entry.Point, interruptedNote(entry))
		}
	}
	return nil
}

// describeStray names the server or process a stray entry records.
func describeStray(entry StrayEntry) string {
	if entry.Kind == StrayServer {
		return fmt.Sprintf("server of hub %s", entry.Hub)
	}
	return fmt.Sprintf("process %d (%s)", entry.PID, strings.Join(entry.Argv, " "))
}

// interruptedNote marks a stray an interrupted row left.
func interruptedNote(entry StrayEntry) string {
	if entry.Interrupted {
		return ", left by an interrupted row"
	}
	return ""
}

// writeDeferredFacts writes the findings the round's fixer report defers and those its red live report lists.
// A fixer report that cannot be parsed is named as such and fails nothing.
func writeDeferredFacts(facts *strings.Builder, runDir string, round int, live LiveReport) error {
	content, err := os.ReadFile(filepath.Join(runDir, fmt.Sprintf(fixerReportFileFormat, round)))
	switch {
	case errors.Is(err, os.ErrNotExist):
		facts.WriteString("- Fixer report: absent.\n")
	case err != nil:
		return fmt.Errorf("read the fixer report of round %d: %w", round, err)
	default:
		report, parseErr := burlerengine.ParseFixerReport(content)
		if parseErr != nil {
			fmt.Fprintf(facts, "- Fixer report: unreadable: %s\n", strings.Join(strings.Fields(parseErr.Error()), " "))
			break
		}
		deferred := 0
		for _, disposition := range report.Dispositions {
			if disposition.Disposition == burlerengine.DispositionDeferred {
				fmt.Fprintf(facts, "- Deferred by the fixer: `%s`: %s\n", disposition.ID, disposition.Reason)
				deferred++
			}
		}
		if deferred == 0 {
			facts.WriteString("- Deferred by the fixer: none.\n")
		}
	}
	for _, line := range live.Deferred {
		fmt.Fprintf(facts, "- Deferred by the red live report: %s\n", line)
	}
	if len(live.Deferred) == 0 {
		facts.WriteString("- Deferred by the red live report: none.\n")
	}
	return nil
}
