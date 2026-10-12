// overview.go implements the hidden `overview` batten verb, the pane process of the prime's runs strand, and the pure rendering of the runs in flight it draws:
// one line per batten run with its state, stage, time in stage, age and reed session.

package battencli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/statuspane"
)

const (
	overviewNoRuns         = "no runs in flight"
	overviewStarting       = "starting"
	overviewStatusBroken   = "status unreadable"
	overviewRunsBroken     = "runs unreadable"
	overviewUnknown        = "-"
	overviewColumnGap      = "  "
	overviewMoreRunsFormat = "… %d more runs not shown"
	// overviewCellCount is the cells after the slug: state, stage, time in stage, age and session.
	overviewCellCount = 5
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
	// PairExists reports that the task worktree's directory exists.
	PairExists bool
	// Session is the name of the pair's reed session.
	Session string
}

// overviewLine is one run's cells before they are padded into columns.
type overviewLine struct {
	slug       string
	cells      [overviewCellCount]string
	unreadable bool
}

// renderOverview renders the runs in flight, one line each, with its columns padded to the widest value, ordered by start time and then slug with the runs of no known start last.
// A run shows while batten's state is not done and its pair exists or batten is creating it; a run with no status shows as `starting`; a status unreadable on two polls shows the slug and `status unreadable`.
// No run to show renders the one line overviewNoRuns.
func renderOverview(runs []overviewRun, now time.Time) []string {
	shown := make([]overviewRun, 0, len(runs))
	for _, run := range runs {
		if overviewShows(run) {
			shown = append(shown, run)
		}
	}
	if len(shown) == 0 {
		return []string{overviewNoRuns}
	}
	sort.SliceStable(shown, func(a, b int) bool {
		sa, sb := shown[a].StartedAt, shown[b].StartedAt
		switch {
		case sa.IsZero() != sb.IsZero():
			return !sa.IsZero()
		case !sa.Equal(sb):
			return sa.Before(sb)
		default:
			return shown[a].Slug < shown[b].Slug
		}
	})

	lines := make([]overviewLine, len(shown))
	slugWidth := 0
	widths := [overviewCellCount]int{}
	for i, run := range shown {
		lines[i] = overviewLineOf(run, now)
		slugWidth = max(slugWidth, len([]rune(run.Slug)))
		if lines[i].unreadable {
			continue
		}
		for c, cell := range lines[i].cells {
			widths[c] = max(widths[c], len([]rune(cell)))
		}
	}

	rendered := make([]string, len(lines))
	for i, line := range lines {
		cells := []string{padRight(line.slug, slugWidth)}
		if line.unreadable {
			cells = append(cells, overviewStatusBroken)
		} else {
			for c, cell := range line.cells {
				cells = append(cells, padRight(cell, widths[c]))
			}
		}
		rendered[i] = strings.TrimRight(strings.Join(cells, overviewColumnGap), " ")
	}
	return rendered
}

// overviewShows reports whether run belongs on the overview.
func overviewShows(run overviewRun) bool {
	if run.Batten.Unreadable {
		return true
	}
	if !run.Batten.Found {
		return true
	}
	status := run.Batten.Status
	if status.State == shedengine.StateDone {
		return false
	}
	return run.PairExists || status.CurrentProducer == battenrecipe.NameWorktreeCreate
}

// overviewLineOf builds the cells of one shown run: batten's state, the stage, the time in it, the run's age and the reed session.
func overviewLineOf(run overviewRun, now time.Time) overviewLine {
	line := overviewLine{slug: run.Slug}
	inRunShed := run.Batten.Found && run.Batten.Status.CurrentProducer == battenrecipe.NameRunShed
	if run.Batten.Unreadable || (inRunShed && run.Child.Unreadable) {
		line.unreadable = true
		return line
	}

	state, stage := overviewStarting, overviewUnknown
	var stageStarted time.Time
	if run.Batten.Found {
		status := run.Batten.Status
		state, stage = string(status.State), status.CurrentProducer
		stageStarted, _ = lastEntryEnd(status)
		if inRunShed && run.Child.Found {
			stage = run.Child.Status.CurrentProducer
			if childStarted, ok := lastEntryEnd(run.Child.Status); ok {
				stageStarted = childStarted
			}
		}
	}
	if stageStarted.IsZero() {
		stageStarted = run.StartedAt
	}

	line.cells = [overviewCellCount]string{
		state,
		stage,
		overviewSince(stageStarted, now),
		overviewSince(run.StartedAt, now),
		run.Session,
	}
	return line
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
	return statuspane.FormatMinutes(now.Sub(start))
}

// padRight pads s with spaces to width runes.
func padRight(s string, width int) string {
	if missing := width - len([]rune(s)); missing > 0 {
		return s + strings.Repeat(" ", missing)
	}
	return s
}

// fitOverview returns at most rows lines: the oldest runs' lines that leave room for a closing line counting the runs left out.
// Lines that already fit come back unchanged.
func fitOverview(lines []string, rows int) []string {
	if len(lines) <= rows {
		return lines
	}
	kept := max(rows-1, 0)
	fitted := append([]string(nil), lines[:kept]...)
	return append(fitted, fmt.Sprintf(overviewMoreRunsFormat, len(lines)-kept))
}

// overviewSource reads the prime's batten runs for the overview pane, keeping what the polls can reuse.
// It is not safe for concurrent use.
type overviewSource struct {
	prime *lyxcwd.Location
	now   func() time.Time
	// childStatus returns the path of the status file of the task worktree slug names; a test replaces it.
	childStatus func(slug string) (string, error)

	battenCaches map[string]*statuspane.StatusCache
	childCaches  map[string]*statuspane.StatusCache
	// seeds holds the decoded seed by slug; a seed is write-once, so a decoded one is never read again.
	seeds map[string]shedrun.Seed
	// pairs holds the resolved task worktree location by slug, for as long as its directory exists.
	pairs map[string]*lyxcwd.Location
}

// newOverviewSource returns a source over the prime's runs that reads the time from now.
func newOverviewSource(prime *lyxcwd.Location, now func() time.Time) *overviewSource {
	s := &overviewSource{
		prime:        prime,
		now:          now,
		battenCaches: map[string]*statuspane.StatusCache{},
		childCaches:  map[string]*statuspane.StatusCache{},
		seeds:        map[string]shedrun.Seed{},
		pairs:        map[string]*lyxcwd.Location{},
	}
	s.childStatus = s.resolveChildStatus
	return s
}

// resolveChildStatus resolves the task worktree of slug once while its directory exists and returns its status file.
func (s *overviewSource) resolveChildStatus(slug string) (string, error) {
	pair, ok := s.pairs[slug]
	if !ok {
		resolved, err := lyxcwd.ResolveWorktree(fabricengine.WorktreePath(s.prime, slug))
		if err != nil {
			return "", err
		}
		pair = resolved
		s.pairs[slug] = pair
	}
	return shedrun.StatusFile(pair, shedrun.SelfRunID), nil
}

// frame polls the prime's runs and returns the overview frame.
// It reads the run records and the status files without their locks, and writes nothing.
func (s *overviewSource) frame() statuspane.Frame {
	slugs, err := shedrun.List(s.prime)
	if err != nil {
		logger.Debug("batten: listing the runs for the overview failed", "err", err)
		return statuspane.Frame{Lines: []string{overviewRunsBroken}}
	}

	var runs []overviewRun
	for _, slug := range slugs {
		if run, ok := s.poll(slug); ok {
			runs = append(runs, run)
		}
	}
	lines := renderOverview(runs, s.now())
	return statuspane.Frame{
		Lines: lines,
		Fit:   func(rows int) []string { return fitOverview(lines, rows) },
	}
}

// poll builds the overviewRun of slug; false when its seed names another recipe.
func (s *overviewSource) poll(slug string) (overviewRun, bool) {
	run := overviewRun{Slug: slug}

	seed, cached := s.seeds[slug]
	if !cached {
		read, found, err := shedrun.ReadSeed(s.prime, slug)
		switch {
		case err != nil:
			logger.Debug("batten: reading a seed for the overview failed", "slug", slug, "err", err)
		case !found:
			return run, false
		default:
			seed = read
			s.seeds[slug] = seed
			cached = true
		}
	}
	if cached {
		if seed.Recipe != shedrun.RecipeBatten {
			return run, false
		}
		if startedAt, err := time.Parse(time.RFC3339, seed.StartedAt); err == nil {
			run.StartedAt = startedAt
		}
	}

	pair := fabricengine.WorktreePath(s.prime, slug)
	run.Session = reedengine.SessionName(pair)
	info, err := os.Stat(pair)
	run.PairExists = err == nil && info.IsDir()
	if !run.PairExists {
		delete(s.pairs, slug)
	}

	run.Batten = cacheFor(s.battenCaches, slug).Poll(shedrun.StatusFile(s.prime, slug))
	if run.Batten.Found && run.Batten.Status.CurrentProducer == battenrecipe.NameRunShed {
		path, err := s.childStatus(slug)
		if err != nil {
			logger.Debug("batten: resolving a task worktree for the overview failed", "slug", slug, "err", err)
		} else {
			run.Child = cacheFor(s.childCaches, slug).Poll(path)
		}
	}
	return run, true
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

// overviewCmd builds the hidden `overview` subcommand: the pane process of the prime's runs strand.
// It reads the run records and status files only; its one write is the content-rows report, which reed confines to the strand that runs it.
func (c *battenCLI) overviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "overview",
		Short:       "draw the runs in flight in a pane (run by the prime's runs strand)",
		Hidden:      true,
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceInternal},
		Long: `overview draws one line per batten run in flight -- its state, stage, time in
the stage, age and reed session -- redrawing in place until it is signalled.
The prime's runs strand runs it, and it reports its line count to reed so the
strand is sized to it. It reads files only, and runs from the prime only.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			location, err := lyxcwd.Resolve(cwd)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			if err := RefuseUnlessPrime(location); err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			reedCfg, err := reedengine.LoadConfig(location.AnchorPath(), "reed")
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			reedGeom, err := hubgeom.ReedGeometry(location)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			engine := reedengine.New(reedCfg, reedGeom)

			source := newOverviewSource(location, time.Now)
			pane := statuspane.Pane{
				Out:    out,
				Size:   statuspane.TerminalSize(os.Stdout),
				Report: statuspane.ReportFromEnv(os.Getenv, engine.SetContentRows),
				Sleep:  time.Sleep,
			}
			pane.Run(ctx, source.frame)
			return nil
		},
	}
}
