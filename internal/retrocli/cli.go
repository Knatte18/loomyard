// cli.go builds `lym retro`, which files and releases the darn task that audits landed runs.

// Package retrocli provides `lym retro`.
// The verb finds the last retro and picks the landed runs the new one covers through `retroengine`.
// It renders the task body from the retro stencil and files the task on the board.
// It then releases the task, so the hub's dobby starts it as a darn run.
// It runs from the hub's prime worktree only, in a repository that holds the retro's scripts.
// The board write carries the caller's strand name, so a run's agent is refused a new task as on `lym board`.
package retrocli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reportcli"
	"github.com/Knatte18/loomyard/internal/reportengine"
	"github.com/Knatte18/loomyard/internal/retroengine"
)

// dateLayout is the form --since and --until take.
const dateLayout = "2006-01-02"

// Command returns the `lym retro` verb.
func Command() *cobra.Command {
	var since, until string
	var maxRuns int
	cmd := &cobra.Command{
		Use:         "retro [<slug>...]",
		Short:       "file and release a darn task that audits landed runs from their transcripts and proposes fixes",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: fmt.Sprintf(`retro files one darn task that audits landed runs, and releases it.
With no slug it covers the runs landed since the last retro: the newest stamp
among the report directories under %[1]s and the retro tasks on
the board. With no earlier retro it covers the last %[2]d days. Named slugs
cover exactly those runs, each landed within the last %[3]d days.

The task's writer runs the scripts under %[4]s over the runs' records
and transcripts, reads what the figures point at, and writes a stamped report
of proposals, most severe first, under %[1]s. It files nothing
on the board.

  --since <date>     UTC date (YYYY-MM-DD) after which a run is covered, in
                     place of the last retro; not with slugs
  --until <date>     UTC date (YYYY-MM-DD) up to which a run is covered
                     (default: now); not with slugs
  --max-runs <n>     the most runs one retro covers (default %[5]d); past it the
                     oldest are kept and the next retro starts after them

It runs from the hub's prime worktree only.

Example:
  lym retro`,
			retroengine.ReportDirRel, int(retroengine.DefaultWindow.Hours()/24), int(retroengine.NamedLookback.Hours()/24),
			retroengine.ToolsDirRel, retroengine.DefaultMaxRuns),
		Args: cobra.ArbitraryArgs,
		RunE: clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int {
			window, err := parseWindow(args, since, until, maxRuns, time.Now().UTC())
			if err != nil {
				return output.Err(out, err.Error())
			}
			return fileRetro(ctx, out, window)
		}),
	}
	cmd.Flags().StringVar(&since, "since", "", "UTC date (YYYY-MM-DD) after which a run is covered, in place of the last retro")
	cmd.Flags().StringVar(&until, "until", "", "UTC date (YYYY-MM-DD) up to which a run is covered (default: now)")
	cmd.Flags().IntVar(&maxRuns, "max-runs", retroengine.DefaultMaxRuns, "the most runs one retro covers; at least 1")
	return cmd
}

// parseWindow validates the arguments and flags before anything is read.
// Each refusal names the flag, the value given and the form accepted.
func parseWindow(slugs []string, since, until string, maxRuns int, now time.Time) (retroengine.Window, error) {
	window := retroengine.Window{Slugs: slugs, Now: now, MaxRuns: maxRuns}
	if len(slugs) > 0 && (since != "" || until != "") {
		return retroengine.Window{}, errors.New("retro: named runs take no --since or --until: name the runs, or give the window, not both")
	}
	if maxRuns < 1 {
		return retroengine.Window{}, fmt.Errorf("retro: --max-runs %d is too small: expected an integer of at least 1", maxRuns)
	}
	for _, flag := range []struct {
		name  string
		value string
		into  *time.Time
	}{{"--since", since, &window.Since}, {"--until", until, &window.Until}} {
		if flag.value == "" {
			continue
		}
		parsed, err := time.ParseInLocation(dateLayout, flag.value, time.UTC)
		if err != nil {
			return retroengine.Window{}, fmt.Errorf("retro: %s %q is not a date: expected YYYY-MM-DD in UTC", flag.name, flag.value)
		}
		*flag.into = parsed
	}
	// A retro stamped after now would hide the runs that land between the filing and that stamp from the next retro.
	if window.Until.After(now) {
		return retroengine.Window{}, fmt.Errorf("retro: --until %q is later than now: omit --until, or give a date no later than today", until)
	}
	return window, nil
}

// fileRetro plans the retro over window from the prime the context's cwd resolves to, then files and releases its task.
func fileRetro(ctx context.Context, out io.Writer, window retroengine.Window) int {
	location, err := resolvePrime(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}
	worktree := location.WorktreePath()
	if _, err := os.Stat(filepath.Join(worktree, retroengine.ToolsDirRel)); err != nil {
		return output.Err(out, fmt.Sprintf("retro: this repository holds no %s, the retro's scripts: run lym retro on the hub of a repository that has them", retroengine.ToolsDirRel))
	}

	tasks, err := boardengine.ReadTasks(fabriclayout.BoardDir(location.HubPath))
	if err != nil {
		return output.Err(out, err.Error())
	}
	names, err := reportDirNames(filepath.Join(worktree, retroengine.ReportDirRel))
	if err != nil {
		return output.Err(out, err.Error())
	}
	for _, task := range tasks {
		names = append(names, task.Slug)
	}
	last, hasLast := retroengine.LastRetro(names)

	landed, err := landedRuns(location, window.Start(last, hasLast))
	if err != nil {
		return output.Err(out, err.Error())
	}
	plan, err := retroengine.Select(landed, last, hasLast, window)
	if err != nil {
		return output.Err(out, err.Error())
	}
	if slices.ContainsFunc(tasks, func(task boardengine.Task) bool { return task.Slug == plan.Slug }) {
		return output.Err(out, fmt.Sprintf("retro: the board already holds an entry %s: "+
			"a retro ending now can be filed again a second later; a retro ending at a landing needs a different set of runs, or --since and --until",
			plan.Slug))
	}

	body, err := plan.Body(fabriclayout.StencilsDir(location.HubPath))
	if err != nil {
		return output.Err(out, err.Error())
	}
	board, err := openBoard(location)
	if err != nil {
		return output.Err(out, err.Error())
	}
	if _, err := board.UpsertTask(plan.TaskFields(body)); err != nil {
		return output.Err(out, fmt.Sprintf("retro: file the task %s: %v", plan.Slug, err))
	}
	// The board gives a new task the draft status, so the filing releases it with a second write.
	if err := board.SetStatus(plan.Slug, nil); err != nil {
		return output.Err(out, fmt.Sprintf("retro: the task %s was filed but not released and stands as a draft with no run: release it with `lym board set-status` and a null status: %v", plan.Slug, err))
	}
	return output.Ok(out, map[string]any{
		"slug":       plan.Slug,
		"runs":       plan.Slugs(),
		"from":       plan.From.UTC().Format(time.RFC3339),
		"until":      plan.Until.UTC().Format(time.RFC3339),
		"report_dir": plan.ReportDir(),
	})
}

// resolvePrime resolves the context's cwd to the hub's prime worktree, refusing any other worktree.
func resolvePrime(ctx context.Context) (*lymcwd.Location, error) {
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return nil, err
	}
	location, err := lymcwd.Resolve(cwd)
	if err != nil {
		return nil, err
	}
	if err := fabricengine.RequireDrivableWorktree(location); err != nil {
		return nil, fmt.Errorf("retro: lym retro runs from the hub's prime worktree only: %w", err)
	}
	primeName, err := fabricengine.PrimeName(location)
	if err != nil {
		return nil, fmt.Errorf("retro: cannot verify this is the hub's prime worktree: %w", err)
	}
	if location.WorktreeName != primeName {
		return nil, fmt.Errorf("retro: lym retro runs from the hub's prime worktree only; %q is not the prime worktree (%q is): re-run it from there",
			location.WorktreeName, primeName)
	}
	return location, nil
}

// reportDirNames returns the names of the entries under dir, none when dir does not exist.
func reportDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("retro: list the retro reports: %w", err)
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names, nil
}

// landedRuns returns the runs landed on the prime's parent branch from since on, each with its landing commit's time.
// A landing no finished run claims is left out.
func landedRuns(location *lymcwd.Location, since time.Time) ([]retroengine.Run, error) {
	geometry, err := reportcli.PrimeGeometry(location)
	if err != nil {
		return nil, err
	}
	sources, err := reportengine.Open(geometry)
	if err != nil {
		return nil, err
	}
	report, err := reportengine.Compute(sources, reportengine.Options{Since: since, FreshDays: 1})
	if err != nil {
		return nil, err
	}
	var runs []retroengine.Run
	for _, landing := range report.Landings {
		if landing.Slug != "" {
			runs = append(runs, retroengine.Run{Slug: landing.Slug, Landed: landing.Committed})
		}
	}
	return runs, nil
}

// openBoard opens the hub's board with the caller's strand name,
// so the board's guard judges a new task as it does on `lym board`.
func openBoard(location *lymcwd.Location) (*boardengine.Board, error) {
	cfg, err := boardengine.LoadConfig(fabricengine.ConfigDirs(location), "board")
	if err != nil {
		return nil, err
	}
	cfg.Path = fabriclayout.BoardDir(location.HubPath)
	cfg = boardengine.ApplySkipEnv(cfg)
	cfg.StrandName = os.Getenv(agentname.StrandNameEnv)
	return boardengine.New(cfg), nil
}
