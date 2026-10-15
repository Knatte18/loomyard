// cli.go builds the cobra command tree for the report module and the RunCLI/RunCLIIn seams that wire it into the standard io.Writer-based call contract.
// The convergence verb validates its flags, resolves the prime's geometry once, and hands it to the report engine.
// The prime resolution both verbs share lives here too.

package reportcli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reportengine"
	"github.com/spf13/cobra"
)

const (
	formatJSON     = "json"
	formatMarkdown = "markdown"

	// sinceLayout is the date form --since takes.
	sinceLayout = "2006-01-02"

	// defaultSinceWeeks is how many weeks before now the report opens when --since is not given.
	defaultSinceWeeks = 12

	defaultFreshDays = 3
)

// Command returns the cobra command for the report module: the bare `report` group and its two verbs, convergence and timing.
func Command() *cobra.Command {
	report := &cobra.Command{
		Use:   "report",
		Short: "read-only reports over the hub's records and history",
		Long: `report holds the read-only reports lyx computes from the records it already
writes: convergence, which asks whether the orch's work converges, and timing,
which asks where run time went. Every verb runs from the hub's prime worktree
only and changes nothing.`,
		RunE: clihelp.GroupRunE,
	}
	report.AddCommand(convergenceCommand())
	report.AddCommand(timingCommand())
	return report
}

// resolvePrime resolves the cwd the context carries to the hub's prime worktree for the verb named verb.
// Each refusal names `lyx report <verb>`: a cwd that is no worktree, a worktree that is not drivable, and a worktree that is not the prime.
func resolvePrime(ctx context.Context, verb string) (*lyxcwd.Location, error) {
	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		return nil, err
	}
	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		return nil, err
	}
	if err := fabricengine.RequireDrivableWorktree(location); err != nil {
		return nil, fmt.Errorf("report: lyx report %s runs from the hub's prime worktree only: %w", verb, err)
	}
	primeName, primeNameErr := fabricengine.PrimeName(location)
	if err := refuseNonPrime(verb, location.WorktreeName, primeName, primeNameErr); err != nil {
		return nil, err
	}
	return location, nil
}

// convergenceCommand returns the `report convergence` verb.
func convergenceCommand() *cobra.Command {
	var since, format string
	var freshDays int
	cmd := &cobra.Command{
		Use:         "convergence",
		Short:       "report what woke the orch, what stopped runs and how landings fared, per run and per week",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `convergence reads the hub's finished runs and the parent branch's landings and
reports, per run and per ISO week, the answers to four board questions:
do orch wakes per run fall, do the same stop and failure classes recur, does the
share of fixes that repair freshly landed code fall, and do review rounds per
segment fall. A record a run lacks is reported absent, never as zero.

  --since <date>      UTC date (YYYY-MM-DD) before which a run is left out
                      (default: twelve weeks ago)
  --fresh-days <n>    days a fix may trail the landing it repairs and still
                      count as repairing fresh code; at least 1 (default 3)
  --format <form>     json (default) or markdown

It runs from the hub's prime worktree only, reads git history and writes nothing.

Example:
  lyx report convergence --format markdown`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			fail := func(err error) error {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			now := time.Now().UTC()
			options, err := parseOptions(since, freshDays, format, now)
			if err != nil {
				return fail(err)
			}

			location, err := resolvePrime(ctx, "convergence")
			if err != nil {
				return fail(err)
			}

			geometry, err := primeGeometry(location)
			if err != nil {
				return fail(err)
			}
			sources, err := reportengine.Open(geometry)
			if err != nil {
				return fail(err)
			}
			report, err := reportengine.Compute(sources, options)
			if err != nil {
				return fail(err)
			}

			if format == formatMarkdown {
				fmt.Fprint(out, reportengine.RenderMarkdown(report))
				return nil
			}
			clihelp.SetExit(ctx, output.Ok(out, map[string]any{
				"selection": report.Selection,
				"runs":      report.Runs,
				"partial":   report.Partial,
				"landings":  report.Landings,
				"weeks":     report.Weeks,
				"absent":    report.Absent,
				"findings":  report.Findings,
			}))
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "UTC date (YYYY-MM-DD) before which a run is left out (default: twelve weeks ago)")
	cmd.Flags().IntVar(&freshDays, "fresh-days", defaultFreshDays, "days a fix may trail the landing it repairs and still count as repairing fresh code; at least 1")
	cmd.Flags().StringVar(&format, "format", formatJSON, "output form: json or markdown")
	return cmd
}

// parseOptions validates the flags before anything is read.
// Each refusal names the flag, the value given and the form accepted.
func parseOptions(since string, freshDays int, format string, now time.Time) (reportengine.Options, error) {
	options := reportengine.Options{FreshDays: freshDays}
	if since == "" {
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		options.Since = today.AddDate(0, 0, -7*defaultSinceWeeks)
	} else {
		parsed, err := time.ParseInLocation(sinceLayout, since, time.UTC)
		if err != nil {
			return reportengine.Options{}, fmt.Errorf("report: --since %q is not a date: expected YYYY-MM-DD in UTC", since)
		}
		options.Since = parsed
	}
	if freshDays < 1 {
		return reportengine.Options{}, fmt.Errorf("report: --fresh-days %d is too small: expected an integer of at least 1", freshDays)
	}
	if format != formatJSON && format != formatMarkdown {
		return reportengine.Options{}, fmt.Errorf("report: --format %q is not a report form: expected %s or %s", format, formatJSON, formatMarkdown)
	}
	return options, nil
}

// primeGeometry resolves the checkouts and branches the report reads from the prime worktree's location.
func primeGeometry(location *lyxcwd.Location) (reportengine.Geometry, error) {
	codeRepo := location.WorktreePath()
	parentBranch, err := gitrepo.New(codeRepo).CurrentBranch()
	if err != nil {
		return reportengine.Geometry{}, fmt.Errorf("report: read the prime's branch: %w", err)
	}
	recordsRepo, err := fabricengine.RecordsRepoRoot(location)
	if err != nil {
		return reportengine.Geometry{}, err
	}
	boardRepo := fabricengine.BoardDir(location.HubPath)
	boardBranch, err := gitrepo.New(boardRepo).CurrentBranch()
	if err != nil {
		return reportengine.Geometry{}, fmt.Errorf("report: read the board's branch: %w", err)
	}
	return reportengine.Geometry{
		CodeRepo:      codeRepo,
		RecordsRepo:   recordsRepo,
		BoardRepo:     boardRepo,
		ParentBranch:  parentBranch,
		RecordsBranch: fabricengine.RecordsBranchName(parentBranch),
		BoardBranch:   boardBranch,
	}, nil
}

// RunCLI is the public seam for the report module CLI, delegating to clihelp.Execute for output capture.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's cwd-carrying sibling: an empty cwd reads the process cwd, any other value seeds cwd into the execution context, because lyxcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}
