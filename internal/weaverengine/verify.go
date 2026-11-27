// verify.go holds the plan-level verify Weaver's Done rests on, the verify fixer's spawn and the run's summary record.
// RunDeps, the seams the row's producer is told, is declared here for its first user.

package weaverengine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/batcher"
	"github.com/Knatte18/loomyard/internal/discussionparser"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/gatefixer"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/summaryparser"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// verifySite labels the plan-level verify's runs in the running marker.
const verifySite = "weaver verify"

// verifyFixerWayForward opens the findings of a failed plan-level verify.
const verifyFixerWayForward = "Weaver hands this failure to a verify fixer, which fixes it and commits; Weaver then runs the verify again."

// fixCommitPrefix opens the subject of every commit a verify fixer makes.
const fixCommitPrefix = "fix:"

// VerifyFunc runs command through the verified-tree record for site and returns its result.
type VerifyFunc func(site verifytree.Site, command string) (verifytree.Result, error)

// RunDeps is everything the row's producer is told.
// A nil Verify or Clock takes the production value, and a nil Deliver or ParentBranch is legal.
// Run refuses any other nil seam as a wiring error naming the field.
type RunDeps struct {
	// Geom is the told geometry.
	Geom Geometry
	// Config is the loaded weaver.yaml.
	Config Config
	// Ladder is the resolved rung choices, one per rung of Config.Ladder.
	Ladder []shuttleengine.ModelChoice
	// Batcher forms the batches from the remaining cards.
	Batcher batcher.Batcher
	// Gate is the row's own gate entries, which every batch attempt's gate spec ends with.
	Gate shuttleengine.GateSpec
	// Runner starts, probes and stops the batch strands.
	Runner StrandRunner
	// Fixer spawns, waits on and stops the plan-level verify fixers.
	Fixer gatefixer.Fixer
	// Reed answers strand liveness for a resume.
	Reed shuttleengine.ReedOps
	// Verify runs the plan-level verify; nil takes verifytree.Verify over the geometry's paths.
	Verify VerifyFunc
	// CommitRecords commits Weaver's run directory and the plan directory.
	CommitRecords func(msg string) error
	// RecordsHead returns the records repository's head, recorded at the run's first begin.
	RecordsHead func() (string, error)
	// ParentBranch names the parent branch a clean merge-in comes from.
	ParentBranch websterengine.ParentBranchFunc
	// Deliver sends a strand the decision entries recorded since it began whose headings delivered lacks.
	// It returns the headings it sent; nil sends nothing.
	Deliver func(strandName string, delivered []string) ([]string, error)
	// LanguageSkills are the hub's language skills, loaded after the implementer role's own.
	LanguageSkills []string
	// RenameTools are the hub's structure-aware rename tools the batch prompt lists.
	RenameTools []string
	// FrictionDir is the friction directory, empty when friction capture is off.
	FrictionDir string
	// ReentryStep is the step that re-enters the run, and an empty one means defaultReentryStep.
	ReentryStep string
	// Clock stamps attempts; nil takes the wall clock.
	Clock runplan.Clock
}

// verify returns the told verify seam, or the production one.
func (d RunDeps) verify() VerifyFunc {
	if d.Verify != nil {
		return d.Verify
	}
	paths := verifytree.NewPaths(d.Geom.PaneCwd, d.Geom.VerifyDir)
	return func(site verifytree.Site, command string) (verifytree.Result, error) {
		return verifytree.Verify(context.Background(), paths, site, command)
	}
}

// clock returns the told clock, or the wall clock.
func (d RunDeps) clock() runplan.Clock {
	if d.Clock != nil {
		return d.Clock
	}
	return wallClock{}
}

// wallClock is the production clock.
type wallClock struct{}

func (wallClock) Now() time.Time        { return time.Now() }
func (wallClock) Sleep(d time.Duration) { time.Sleep(d) }

// verifyOutcome is the result of one plan-level verify evaluation.
type verifyOutcome struct {
	// Passed reports that the verify passed, on its first run or its rerun.
	Passed bool
	// Flaky lists the identities that failed on the first run of a verify whose rerun passed.
	Flaky []string
	// Findings is the failure rendered for a fixer; empty when the verify passed.
	Findings string
	// Unjudged reports that the verify judged no code, by a runner crash or a memory pressure stop,
	// so no fixer can mend it.
	Unjudged bool
}

// runVerify runs the hub's verify gate call through the verify seam under the geometry's pause hold.
// A failure is rerun once, and a pass on the rerun is a pass whose failing identities are flaky.
// A timeout, a dirty tree and an unjudged run are not rerun, and an unjudged run starts no fixer.
// A failure writes the verify-failure record and returns its findings.
func runVerify(deps RunDeps, st *State, evaluation int) (verifyOutcome, error) {
	if deps.Geom.GateConfig == nil {
		return verifyOutcome{}, errors.New("weaver: the geometry carries no gate config reader; way forward: the CLI layer must set Geometry.GateConfig")
	}
	cfg, err := deps.Geom.GateConfig()
	if err != nil {
		return verifyOutcome{}, fmt.Errorf("weaver: read the gate config for the plan-level verify: %w", err)
	}
	call, err := cfg.GateCall(deps.Config.VerifyGate)
	if err != nil {
		return verifyOutcome{}, err
	}

	verify := deps.verify()
	site := verifytree.Site{Label: verifySite, Attempt: evaluation, Hold: deps.Geom.VerifyHold}
	res, err := verify(site, call)
	if err != nil {
		return verifyOutcome{}, fmt.Errorf("weaver: plan-level verify: %w", err)
	}
	if res.Status == verifytree.StatusPassed || res.Status == verifytree.StatusSkipped {
		return verifyOutcome{Passed: true}, nil
	}

	var flaky []string
	if res.Status == verifytree.StatusFailed && !res.TimedOut && !res.Unjudged() {
		firstOutput, err := readVerifyLog(res.Log)
		if err != nil {
			return verifyOutcome{}, err
		}
		for _, failure := range runplan.ParseVerifyRun(firstOutput, false) {
			flaky = append(flaky, failure.ID)
		}
		res, err = verify(site, call)
		if err != nil {
			return verifyOutcome{}, fmt.Errorf("weaver: plan-level verify rerun: %w", err)
		}
		if res.Status == verifytree.StatusPassed || res.Status == verifytree.StatusSkipped {
			return verifyOutcome{Passed: true, Flaky: flaky}, nil
		}
	}

	if res.Unjudged() {
		return verifyOutcome{Unjudged: true, Findings: unjudgedFindings(res)}, nil
	}
	report := runplan.VerifyGateReport{Attempt: evaluation, Cap: len(deps.Ladder), Command: call, Fixer: "verify fixer"}
	if res.Status == verifytree.StatusDirty {
		report.Dirty = res.Dirty
	} else {
		output, err := readVerifyLog(res.Log)
		if err != nil {
			return verifyOutcome{}, err
		}
		report = failureReport(call, evaluation, len(deps.Ladder), res, output)
		report.Fixer = "verify fixer"
		report.Hint, err = cardHint(deps, st, report.Failures)
		if err != nil {
			return verifyOutcome{}, err
		}
	}
	if err := runplan.WriteVerifyGateReport(runplan.VerifyGateReportPath(deps.Geom.ReportsDir), report); err != nil {
		return verifyOutcome{}, err
	}
	return verifyOutcome{Findings: runplan.RenderVerifyGateFindings(report, verifyFixerWayForward)}, nil
}

// cardHint names the cards whose commits touched a failing package, from the batches' card trails.
// A card's changed paths are the paths that differ between its batch's start and HEAD but not between its end and HEAD,
// so a path a later batch changed again is not attributed to it.
func cardHint(deps RunDeps, st *State, failures []runplan.VerifyFailure) ([]string, error) {
	if len(failures) == 0 {
		return nil, nil
	}
	var shas, labels []string
	starts := map[string]string{}
	numbers := make([]int, 0, len(st.Batches))
	for number := range st.Batches {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	for _, number := range numbers {
		bs := st.Batches[number]
		if bs == nil {
			continue
		}
		for _, sha := range bs.CardSHAs {
			shas = append(shas, sha)
			labels = append(labels, fmt.Sprintf("%02d-%s", number, bs.Slug))
			starts[sha] = bs.StartSHA
		}
	}
	repo := gitrepo.New(deps.Geom.WorktreeRoot)
	changedPaths := func(sha string) ([]string, error) {
		since, err := repo.ChangedFilesSince(starts[sha])
		if err != nil {
			return nil, err
		}
		after, err := repo.ChangedFilesSince(sha)
		if err != nil {
			return nil, err
		}
		return slices.DeleteFunc(since, func(path string) bool { return slices.Contains(after, path) }), nil
	}
	return runplan.CardHint(readModulePath(deps.Geom.WorktreeRoot), failures, shas, labels, changedPaths)
}

// readModulePath returns the module path go.mod in worktree declares, empty when there is none.
func readModulePath(worktree string) string {
	f, err := os.Open(filepath.Join(worktree, "go.mod"))
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}
	return ""
}

// fixerStore adapts State.Fixers to gatefixer.Store for the fixer number it holds.
type fixerStore struct {
	st     *State
	number int
}

// Live returns the newest fixer record that is set and not terminal, and nil when there is none.
func (s *fixerStore) Live() (*gatefixer.Record, error) {
	for i := len(s.st.Fixers) - 1; i >= 0; i-- {
		if rec := s.st.Fixers[i].Record; rec != nil && rec.Status == "" {
			return rec, nil
		}
	}
	return nil, nil
}

// Save sets rec as the record of the fixer state the store's number names.
func (s *fixerStore) Save(rec *gatefixer.Record) error {
	for _, fixer := range s.st.Fixers {
		if fixer.Number == s.number {
			fixer.Record = rec
			return nil
		}
	}
	return fmt.Errorf("weaver: no fixer %d is recorded to hold the strand %s", s.number, rec.StrandGUID)
}

// fixerSpec renders the launch of one plan-level verify fixer.
// Its prompt is the verify fixer body behind gatefixer's cold frame, told the findings and the journal path.
// It runs on its rung's choice under fixer_timeout_min and carries no shuttle gates:
// Weaver's re-run of the verify judges it.
func fixerSpec(deps RunDeps, st *State, fixer *FixerState, findings string) (gatefixer.Spec, error) {
	if fixer.Rung < 0 || fixer.Rung >= len(deps.Ladder) {
		return gatefixer.Spec{}, fmt.Errorf("weaver: fixer %d names rung %d, but the ladder holds %d rung(s)", fixer.Number, fixer.Rung+1, len(deps.Ladder))
	}
	reportPath, err := filepath.Abs(runplan.VerifyGateReportPath(deps.Geom.ReportsDir))
	if err != nil {
		return gatefixer.Spec{}, fmt.Errorf("weaver: resolve the verify-failure report path: %w", err)
	}
	replyPath, err := filepath.Abs(fixer.ReplyPath)
	if err != nil {
		return gatefixer.Spec{}, fmt.Errorf("weaver: resolve fixer %d's reply path: %w", fixer.Number, err)
	}
	patternChanges, err := discussionparser.DeclaredPatternChanges(deps.Geom.DecisionRecordPath)
	if err != nil {
		return gatefixer.Spec{}, fmt.Errorf("weaver: read the declared PATTERN changes for fixer %d: %w", fixer.Number, err)
	}
	engine := deps.Runner.EngineName(deps.Ladder[fixer.Rung])
	prompt, err := gatefixer.RenderPrompt(gatefixer.PromptInputs{
		StencilsDir: deps.Geom.StencilsDir,
		Engine:      engine,
		BodyStencil: verifyFixBodyStencil,
		BodyValues: map[string]string{
			"findings":      findings,
			"journal_path":  JournalPath(deps.Geom.RunDir),
			"plan_dir":      deps.Geom.PlanDir,
			"worktree_root": deps.Geom.WorktreeRoot,
		},
		ReportPath:     reportPath,
		ReplyPath:      replyPath,
		RepoRoot:       deps.Geom.WorktreeRoot,
		PatternChanges: patternChanges,
		ParentName:     deps.Geom.ParentName,
		NotePath:       friction.NotePath(deps.FrictionDir, fmt.Sprintf("weaver-verify-fix-%d", fixer.Number)),
	})
	if err != nil {
		return gatefixer.Spec{}, err
	}
	return gatefixer.Spec{
		Prompt:       prompt,
		PromptEngine: engine,
		ReportPath:   reportPath,
		ReplyPath:    replyPath,
		Role:         gatefixer.RoleVerifyFixer,
		ModelChoice:  deps.Ladder[fixer.Rung],
		Skills:       deps.LanguageSkills,
		Segment:      segmentcolor.Webster,
		Timeout:      time.Duration(deps.Config.FixerTimeoutMin) * time.Minute,
	}, nil
}

// writeSummary writes summary.md in Weaver's run directory in the shared final-summary format.
// It opens with a title naming the task, then lists the batches with their outcomes.
// When flaky is non-empty it adds `## Integration suite triage`.
// When a verify fixer ran it adds `## Verify gate fixes` with each fixer's findings and its `fix:` commits.
func writeSummary(deps RunDeps, st *State, flaky []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n## Batches\n\n", readPlanTitle(deps.Geom.PlanDir))
	numbers := make([]int, 0, len(st.Batches))
	for number := range st.Batches {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	for _, number := range numbers {
		bs := st.Batches[number]
		if bs == nil {
			continue
		}
		attempts := 0
		for _, attempt := range st.Attempts {
			if attempt.Batch == number {
				attempts++
			}
		}
		fmt.Fprintf(&b, "- `%02d-%s`: %s after %d attempt(s)\n", number, bs.Slug, bs.Status, attempts)
	}

	if len(flaky) > 0 {
		b.WriteString("\n## Integration suite triage\n\nThe plan-level verify failed once and passed on its rerun, so these identities are flaky:\n\n")
		for _, id := range flaky {
			fmt.Fprintf(&b, "- `%s`\n", id)
		}
	}

	if len(st.Fixers) > 0 {
		b.WriteString("\n## Verify gate fixes\n\n")
		for _, fixer := range st.Fixers {
			fmt.Fprintf(&b, "- Fixer %d on rung %d, ending %s, was given:\n\n```\n%s\n```\n\n", fixer.Number, fixer.Rung+1, fixer.Outcome, strings.TrimSpace(fixer.Findings))
		}
		commits, err := fixCommitLines(deps, st)
		if err != nil {
			return err
		}
		for _, line := range commits {
			fmt.Fprintf(&b, "- %s\n", line)
		}
	}

	path := summaryparser.Path(deps.Geom.RunDir)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("weaver: write %s: %w", path, err)
	}
	return nil
}

// fixCommitLines returns one line per `fix:` commit above the last batch's end commit, and each departure line.
func fixCommitLines(deps RunDeps, st *State) ([]string, error) {
	if st.LastEnd == "" {
		return nil, nil
	}
	git := deps.Geom.git()
	head, err := git.HeadSHA(deps.Geom.WorktreeRoot)
	if err != nil {
		return nil, err
	}
	entries, err := git.CommitLog(deps.Geom.WorktreeRoot, st.LastEnd, head)
	if err != nil {
		return nil, fmt.Errorf("weaver: read the fixers' commits for the summary: %w", err)
	}
	var lines []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Subject, fixCommitPrefix) {
			lines = append(lines, fmt.Sprintf("`%s` %s", entry.SHA, entry.Subject))
		}
		lines = append(lines, entry.DepartureLines()...)
	}
	return lines, nil
}

// readPlanTitle returns the title heading of the plan's overview, or a fixed title when it has none.
func readPlanTitle(planDir string) string {
	data, err := os.ReadFile(filepath.Join(planDir, runplan.PlanOverviewFile))
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if title, ok := strings.CutPrefix(line, "# "); ok && strings.TrimSpace(title) != "" {
				return strings.TrimSpace(title)
			}
		}
	}
	return "Weaver run"
}
