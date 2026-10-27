// overview.go implements the hidden `overview` batten verb, the pane process of the runs strand that draws a beam's window, and the cells the window draws for each run a dobby drives:
// the dobby's lines (overviewdobby.go) above the orch lines of the beam and of every beam below it (overvieworch.go).

package battencli

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/durfmt"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/statuspane"
)

const (
	overviewStarting     = "starting"
	overviewStatusBroken = "status unreadable"
	overviewUnknown      = "-"
	overviewColumnGap    = "  "
	// overviewPausedFormat is the state cell of a run a live pause covers, filled with the minutes since the pause began.
	overviewPausedFormat = "paused %s"
)

// overviewRun is what one poll learned about one batten run.
type overviewRun struct {
	// Slug is the run's id, which is the task worktree's slug.
	Slug string
	// StartedAt is the seed's start time; zero when the seed is unreadable or carries none.
	StartedAt time.Time
	// Batten is the poll of batten's own status file.
	Batten statuspane.StatusPoll
	// Child is the poll of the task worktree's status file; the zero poll unless batten is in the row that runs the child.
	Child statuspane.StatusPoll
	// PausedAt is when the live pause covering the run began; zero when none covers it.
	PausedAt time.Time
	// Recipe is the recipe the run's own seed names; empty while the seed is unreadable.
	Recipe string
	// ChildRecipe is the recipe the child's seed names; empty when unknown.
	ChildRecipe string
}

// RunCells is what a dobby's lines draw of one run between its slug and its pass cell.
type RunCells struct {
	// State is batten's state, `paused <minutes>` while a live pause covers a running, paused or starting run.
	State string
	// Stage is the row the run is in, numbered from its recipe, the child's row while batten runs the child.
	Stage string
	// InStage is the time since the stage began, in whole minutes.
	InStage string
	// Unreadable reports that the run's status, or the status of the child it runs, failed to read on two polls in a row; the other fields are then empty.
	Unreadable bool
}

// overviewRunCells builds the cells of run at now.
// A run with no status shows as `starting` with no stage; the stage is the task worktree's current producer while batten is in Run-Shed and that status is found, and batten's own producer otherwise.
func overviewRunCells(run overviewRun, now time.Time) RunCells {
	inRunShed := run.Batten.Found && run.Batten.Status.CurrentProducer == battenrecipe.NameRunShed
	if run.Batten.Unreadable || (inRunShed && run.Child.Unreadable) {
		return RunCells{Unreadable: true}
	}

	state, stage := overviewStarting, overviewUnknown
	var stageStarted time.Time
	if run.Batten.Found {
		status := run.Batten.Status
		state, stage = string(status.State), overviewStage(run.Recipe, status.CurrentProducer)
		stageStarted, _ = lastEntryEnd(status)
		if inRunShed && run.Child.Found {
			stage = overviewChildStage(run)
			if childStarted, ok := lastEntryEnd(run.Child.Status); ok {
				stageStarted = childStarted
			}
		}
	}
	if stageStarted.IsZero() {
		stageStarted = run.StartedAt
	}

	if !run.PausedAt.IsZero() && (state == string(shedengine.StateRunning) || state == string(shedengine.StatePaused) || state == overviewStarting) {
		state = fmt.Sprintf(overviewPausedFormat, overviewSince(run.PausedAt, now))
	}
	return RunCells{State: state, Stage: stage, InStage: overviewSince(stageStarted, now)}
}

// overviewEntryNumber returns the entry number row has in the named recipe; false when the recipe is empty or unreadable or does not hold the row.
func overviewEntryNumber(recipe, row string) (int, bool) {
	if recipe == "" {
		return 0, false
	}
	rows, err := loomrecipe.Rows(recipe)
	if err != nil {
		return 0, false
	}
	number, ok := loomrecipe.EntryNumbers(rows)[row]
	return number, ok
}

// overviewStage draws row as `<n>. <row>`, n being its entry number in the named recipe; the row alone when its number cannot be read.
func overviewStage(recipe, row string) string {
	number, ok := overviewEntryNumber(recipe, row)
	if !ok {
		return row
	}
	return fmt.Sprintf("%d. %s", number, row)
}

// overviewChildStage draws the child's current row of a run in Run-Shed as `<Run-Shed's number>.<child number> <row>`; the row alone when either number cannot be read.
func overviewChildStage(run overviewRun) string {
	row := run.Child.Status.CurrentProducer
	parent, parentOK := overviewEntryNumber(run.Recipe, battenrecipe.NameRunShed)
	child, childOK := overviewEntryNumber(run.ChildRecipe, row)
	if !parentOK || !childOK {
		return row
	}
	return fmt.Sprintf("%d.%d %s", parent, child, row)
}

// lastEntryEnd returns the end of status's last history entry: its last_at when set, else its at; false when there is no entry or the time does not parse.
func lastEntryEnd(status shedengine.Status) (time.Time, bool) {
	if len(status.History) == 0 {
		return time.Time{}, false
	}
	last := status.History[len(status.History)-1]
	for _, value := range []string{last.LastAt, last.At} {
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, value)
		return parsed, err == nil
	}
	return time.Time{}, false
}

// overviewSince renders the time from start to now in whole minutes, or overviewUnknown when start is unknown.
func overviewSince(start, now time.Time) string {
	if start.IsZero() {
		return overviewUnknown
	}
	return durfmt.FormatMinutes(now.Sub(start))
}

// padRight pads s with spaces to width runes.
func padRight(s string, width int) string {
	if missing := width - len([]rune(s)); missing > 0 {
		return s + strings.Repeat(" ", missing)
	}
	return s
}

// DobbyRunCells reads the RunCells of the batten runs of a hub's prime, keeping across reads what a later read can reuse.
// It reads the seeds, the status files and the pause record without their locks, and writes nothing.
// It is not safe for concurrent use.
type DobbyRunCells struct {
	prime *lymcwd.Location
	// pauseRecord is the path of the hub's pause record.
	pauseRecord string
	// childStatus returns the path of the status file of the task worktree slug names; a test replaces it.
	childStatus func(slug string) (string, error)
	// childPair returns the task worktree location of slug; a test replaces it.
	childPair func(slug string) (*lymcwd.Location, error)

	battenCaches map[string]*statuspane.StatusCache
	childCaches  map[string]*statuspane.StatusCache
	// seeds holds the decoded seed by slug; a seed is write-once, so a decoded one is never read again.
	seeds map[string]shedrun.Seed
	// pairs holds the resolved task worktree location by slug, for as long as its directory exists and the run stays in Run-Shed;
	// the pair's anchor can move between two runs, so a later Run-Shed spell resolves it again.
	pairs map[string]*lymcwd.Location
	// childRecipes holds the recipe a child's seed names by slug; a seed is write-once, so a decoded one is never read again while the run stays in Run-Shed.
	// The entry goes with the pair's, since a later run in the pair has a seed of its own.
	childRecipes map[string]string
}

// NewDobbyRunCells returns the reader of the runs of prime, with the pause coverage read from the record at pauseRecord.
func NewDobbyRunCells(prime *lymcwd.Location, pauseRecord string) *DobbyRunCells {
	r := &DobbyRunCells{
		prime:        prime,
		pauseRecord:  pauseRecord,
		battenCaches: map[string]*statuspane.StatusCache{},
		childCaches:  map[string]*statuspane.StatusCache{},
		seeds:        map[string]shedrun.Seed{},
		pairs:        map[string]*lymcwd.Location{},
		childRecipes: map[string]string{},
	}
	r.childPair = r.resolveChildPair
	r.childStatus = r.resolveChildStatus
	return r
}

// Of returns the RunCells of each of runs at now, by slug.
// A status that fails to read once keeps the cells of its previous read; a pause record that does not read covers nothing.
func (r *DobbyRunCells) Of(runs []dobbyengine.ViewRun, now time.Time) map[string]RunCells {
	record, err := pauserecord.Read(r.pauseRecord)
	if err != nil {
		logger.Debug("batten: reading the pause record for the overview failed", "err", err)
		record = pauserecord.Record{}
	}
	cells := make(map[string]RunCells, len(runs))
	for _, viewRun := range runs {
		run := r.poll(viewRun.Slug)
		if entry, covered := record.Covering(viewRun.Slug); covered {
			run.PausedAt = entry.PausedAt
		}
		cells[viewRun.Slug] = overviewRunCells(run, now)
	}
	return cells
}

// resolveChildPair resolves the task worktree of slug once per Run-Shed spell while its directory exists.
func (r *DobbyRunCells) resolveChildPair(slug string) (*lymcwd.Location, error) {
	pair, ok := r.pairs[slug]
	if !ok {
		resolved, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(r.prime, slug))
		if err != nil {
			return nil, err
		}
		pair = resolved
		r.pairs[slug] = pair
	}
	return pair, nil
}

// resolveChildStatus returns the status file of the task worktree of slug.
func (r *DobbyRunCells) resolveChildStatus(slug string) (string, error) {
	pair, err := r.childPair(slug)
	if err != nil {
		return "", err
	}
	return shedrun.StatusFile(pair, shedrun.SelfRunID), nil
}

// resolveChildRecipe returns the recipe the seed of the task worktree of slug names, empty when the seed is absent.
// A decoded seed's recipe is cached by slug.
func (r *DobbyRunCells) resolveChildRecipe(slug string) (string, error) {
	if recipe, ok := r.childRecipes[slug]; ok {
		return recipe, nil
	}
	pair, err := r.childPair(slug)
	if err != nil {
		return "", err
	}
	seed, found, err := shedrun.ReadSeed(pair, shedrun.SelfRunID)
	if err != nil || !found {
		return "", err
	}
	r.childRecipes[slug] = seed.Recipe
	return seed.Recipe, nil
}

// poll builds the overviewRun of slug from its seed, batten's status and, while batten is in Run-Shed, the child's status and seed.
func (r *DobbyRunCells) poll(slug string) overviewRun {
	run := overviewRun{Slug: slug}

	seed, cached := r.seeds[slug]
	if !cached {
		read, found, err := shedrun.ReadSeed(r.prime, slug)
		switch {
		case err != nil:
			logger.Debug("batten: reading a seed for the overview failed", "slug", slug, "err", err)
		case found:
			seed, cached = read, true
			r.seeds[slug] = seed
		}
	}
	if cached {
		run.Recipe = seed.Recipe
		if startedAt, err := time.Parse(time.RFC3339, seed.StartedAt); err == nil {
			run.StartedAt = startedAt
		}
	}

	if info, err := os.Stat(fabricengine.WorktreePath(r.prime, slug)); err != nil || !info.IsDir() {
		delete(r.pairs, slug)
	}

	run.Batten = cacheFor(r.battenCaches, slug).Poll(shedrun.StatusFile(r.prime, slug))
	if run.Batten.Found && run.Batten.Status.CurrentProducer == battenrecipe.NameRunShed {
		path, err := r.childStatus(slug)
		if err != nil {
			logger.Debug("batten: resolving a task worktree for the overview failed", "slug", slug, "err", err)
		} else {
			run.Child = cacheFor(r.childCaches, slug).Poll(path)
		}
		recipe, err := r.resolveChildRecipe(slug)
		if err != nil {
			logger.Debug("batten: reading a child seed for the overview failed", "slug", slug, "err", err)
		}
		run.ChildRecipe = recipe
	} else {
		delete(r.pairs, slug)
		delete(r.childRecipes, slug)
	}
	return run
}

// cacheFor returns the cache of slug in caches, creating it on first use.
func cacheFor(caches map[string]*statuspane.StatusCache, slug string) *statuspane.StatusCache {
	cache, ok := caches[slug]
	if !ok {
		cache = statuspane.NewStatusCache()
		caches[slug] = cache
	}
	return cache
}

// overviewSource draws a beam's window: the lines of its dobby, then the orch lines of the beam and of every beam below it.
// It is not safe for concurrent use.
type overviewSource struct {
	now func() time.Time
	// orchPaths locates the state file of the window's own orch, labelled orchLabel; the frame reads it alone when orchs is nil or fails.
	orchPaths orchengine.Paths
	orchLabel string
	// orchs returns the paths of the orchs the window shows, with the slug label of each, empty for the prime; read on each frame.
	orchs func() ([]orchengine.Paths, []string, error)
	// orchCaches holds one state cache per orch, by state file path.
	orchCaches map[string]*orchStateCache
	thresholds overviewThresholds
	// dobby renders the lines of the window's dobby at now; nil draws no dobby section.
	dobby func(now time.Time) []string
}

// newOverviewSource returns the source of the window of worktree, in the hub of prime, drawing the orchs against thresholds and reading the time from now.
func newOverviewSource(prime, worktree *lymcwd.Location, thresholds overviewThresholds, now func() time.Time) *overviewSource {
	label := worktree.WorktreeName
	if label == prime.WorktreeName {
		label = ""
	}
	parents := NewDobbyParents(prime)
	return &overviewSource{
		now:       now,
		orchPaths: orchcli.PrimePaths(worktree),
		orchLabel: label,
		orchs: func() ([]orchengine.Paths, []string, error) {
			return windowOrchPaths(prime, worktree.WorktreeName, parents.Of)
		},
		orchCaches: map[string]*orchStateCache{},
		thresholds: thresholds,
	}
}

// windowOrchPaths returns the orch paths of the orchs the window of the worktree named worktree shows, in the hub of prime, with the slug of each beam's worktree as its label and an empty label for the prime.
// parentOf answers the worktree a beam was forked from, "" for the prime.
func windowOrchPaths(prime *lymcwd.Location, worktree string, parentOf func(slug string) (string, error)) ([]orchengine.Paths, []string, error) {
	locations, err := orchcli.HubOrchs(prime)
	if err != nil {
		return nil, nil, err
	}
	shown := orchsAtOrBelow(prime.WorktreeName, worktree, locations, parentOf)
	paths := make([]orchengine.Paths, len(shown))
	labels := make([]string, len(shown))
	for i, location := range shown {
		paths[i] = orchcli.PrimePaths(location)
		if location.WorktreeName != prime.WorktreeName {
			labels[i] = location.WorktreeName
		}
	}
	return paths, labels, nil
}

// orchsAtOrBelow keeps, in order, the orch locations the window of the worktree named worktree shows.
// The prime's window, the prime being named primeName, shows every one; a beam's window shows the beam's own and each beam whose parent chain, read through parentOf, reaches the beam.
// A location whose parent chain does not read is left out.
func orchsAtOrBelow(primeName, worktree string, locations []*lymcwd.Location, parentOf func(slug string) (string, error)) []*lymcwd.Location {
	if worktree == primeName {
		return locations
	}
	var shown []*lymcwd.Location
	for _, location := range locations {
		below, err := atOrBelow(location.WorktreeName, worktree, primeName, parentOf)
		if err != nil {
			logger.Debug("batten: reading a beam's parent chain for the overview failed", "beam", location.WorktreeName, "err", err)
			continue
		}
		if below {
			shown = append(shown, location)
		}
	}
	return shown
}

// atOrBelow reports whether the worktree named slug is the one named worktree or descends from it, following parentOf up to the prime, named primeName.
// A parent chain that revisits a slug returns an error.
func atOrBelow(slug, worktree, primeName string, parentOf func(slug string) (string, error)) (bool, error) {
	visited := map[string]bool{}
	for slug != "" && slug != primeName {
		if slug == worktree {
			return true, nil
		}
		if visited[slug] {
			return false, fmt.Errorf("parent chain revisits %q", slug)
		}
		visited[slug] = true
		parent, err := parentOf(slug)
		if err != nil {
			return false, err
		}
		slug = parent
	}
	return false, nil
}

// orchLines polls the state of every orch the window shows and returns one orch line per orch that has a state file to show.
// An orch list that cannot be read falls back to the window's own orch alone.
func (s *overviewSource) orchLines() []string {
	paths, labels := []orchengine.Paths{s.orchPaths}, []string{s.orchLabel}
	if s.orchs != nil {
		listed, listedLabels, err := s.orchs()
		if err != nil {
			logger.Debug("batten: listing the hub's orchs for the overview failed", "err", err)
		} else {
			paths, labels = listed, listedLabels
		}
	}
	var lines []string
	for i, orchPaths := range paths {
		cache, ok := s.orchCaches[orchPaths.StatePath]
		if !ok {
			cache = &orchStateCache{}
			s.orchCaches[orchPaths.StatePath] = cache
		}
		if line, ok := overviewLabeledOrchLine(labels[i], cache.Poll(orchPaths), s.thresholds); ok {
			lines = append(lines, line)
		}
	}
	return lines
}

// frame polls the window's dobby and orchs and returns the frame: the dobby's lines, then a blank line and the orch lines.
// A missing section leaves out the blank line with it.
// It reads the orch states, the run records, the status files and the dobby's record without their locks, and writes nothing.
func (s *overviewSource) frame() statuspane.Frame {
	var lines []string
	if s.dobby != nil {
		lines = s.dobby(s.now())
	}
	if orch := s.orchLines(); len(orch) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, orch...)
	}
	return statuspane.Frame{Lines: lines}
}

// worktreeHasDobby reports whether the worktree named worktree runs a dobby: the prime, or a beam among those beams lists.
// A task pair has none.
func worktreeHasDobby(prime *lymcwd.Location, worktree string, beams func() ([]dobbyengine.BeamPair, error)) (bool, error) {
	if worktree == prime.WorktreeName {
		return true, nil
	}
	pairs, err := beams()
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(pairs, func(pair dobbyengine.BeamPair) bool { return pair.Slug == worktree }), nil
}

// overviewCmd builds the hidden `overview` subcommand: the pane process of the runs strand that draws the window of the prime or of a beam.
// It reads the orch states, the orch config once at start, the run records and status files, and the dobby's records.
func (c *battenCLI) overviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "overview",
		Short:       "draw a beam's window: its dobby and its orchs (run by the runs strand of the prime and of each beam)",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `overview draws the window of the worktree it runs in, the prime or a beam.
First comes the worktree's dobby: whether it is alive and its max_runs, each run
it drives with its state, stage, time in the stage and last pass, and each
released task it has not started with what the task waits on. Below it comes one
orch line per orch -- the session's context reading against the soft and hard
thresholds, its cycle phase and cycle count -- for the worktree's own orch and
the orch of every beam below it; the prime's window shows every orch. It redraws
in place until it is signalled.
The runs strand runs it, and every line is drawn uncut. It reads files and
probes locks for an instant, and runs from the prime and from any beam.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			cwd, err := lymcwd.CwdFrom(ctx)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			acting, err := lymcwd.Resolve(cwd)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			prime, err := ResolveBattenPrime(acting)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			pauseGeom, err := hubgeom.PauseGeometry(prime)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			source := newOverviewSource(prime, acting, loadOverviewThresholds(prime), time.Now)
			worktree := acting.WorktreeName
			hasDobby, err := worktreeHasDobby(prime, worktree, hubgeom.DobbyStart(prime).Beams)
			if err != nil {
				logger.Warn("batten: the overview cannot tell whether its worktree has a dobby and draws no dobby section", "worktree", worktree, "err", err)
			} else if !hasDobby {
				logger.Debug("batten: the overview's worktree has no dobby and draws no dobby section", "worktree", worktree)
			} else if reader, err := NewDobbyViewReader(prime, worktree); err != nil {
				logger.Warn("batten: the overview cannot place its worktree's dobby and draws no dobby section", "worktree", worktree, "err", err)
			} else {
				cells := NewDobbyRunCells(prime, pauseGeom.RecordPath)
				source.dobby = func(now time.Time) []string { return overviewDobbyLines(reader, cells, now) }
			}
			pane := statuspane.Pane{
				Out:     out,
				Size:    statuspane.TerminalSize(os.Stdout),
				Resized: statuspane.NotifyResize(ctx),
			}
			pane.Run(ctx, source.frame)
			return nil
		},
	}
}
