// cost.go is the `report cost` verb: it validates its flags, reads the report module's minimum run count, resolves the prime and hands both to the report engine.

package reportcli

import (
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reportconfig"
	"github.com/Knatte18/loomyard/internal/reportengine"
	"github.com/spf13/cobra"
)

// costCommand returns the `report cost` verb.
func costCommand() *cobra.Command {
	var since, format string
	cmd := &cobra.Command{
		Use:         "cost",
		Short:       "report what landed runs cost in dollars and hours, per recipe, profile and stored record, with a fixed cost and a cost per 100 changed lines",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `cost reads the usage records every run keeps and reports, per recipe, profile
and stored-record flag, what the landed runs cost: the mean dollars and hours,
and, over the runs that landed code, a least-squares fit of a fixed cost plus a
cost per 100 changed lines, in dollars and in hours, with each fit's correlation.
A cell with fewer runs than report.yaml's cost_min_runs reads "too few runs" and
carries no figure. A run with an unknown session cost enters no dollar figure.

  --since <date>      UTC date (YYYY-MM-DD) the window opens at
                      (default: seven days ago)
  --format <form>     json (default) or markdown

It runs from the hub's prime worktree only, reads git history and writes nothing.

Example:
  lym report cost --since 2026-10-01 --format markdown`,
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

			options, err := parseCostOptions(since, format, time.Now().UTC())
			if err != nil {
				return fail(err)
			}
			location, err := resolvePrime(ctx, "cost")
			if err != nil {
				return fail(err)
			}
			config, err := reportconfig.LoadConfig(fabricengine.ConfigDirs(location))
			if err != nil {
				return fail(err)
			}
			options.MinRuns = config.CostMinRuns
			geometry, err := primeGeometry(location)
			if err != nil {
				return fail(err)
			}
			sources, err := reportengine.Open(geometry)
			if err != nil {
				return fail(err)
			}
			report, err := reportengine.ComputeCost(sources, options)
			if err != nil {
				return fail(err)
			}

			if format == formatMarkdown {
				fmt.Fprint(out, reportengine.RenderCostMarkdown(report))
				return nil
			}
			clihelp.SetExit(ctx, output.Ok(out, map[string]any{
				"selection": report.Selection,
				"cells":     report.Cells,
				"findings":  report.Findings,
			}))
			return nil
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "UTC date (YYYY-MM-DD) the window opens at (default: seven days ago)")
	cmd.Flags().StringVar(&format, "format", formatJSON, "output form: json or markdown")
	return cmd
}

// parseCostOptions validates the flags before anything is read.
// Each refusal names the flag, the value given and the form accepted, and the default window is the timing report's.
func parseCostOptions(since, format string, now time.Time) (reportengine.CostOptions, error) {
	var options reportengine.CostOptions
	if since == "" {
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		options.Since = today.AddDate(0, 0, -defaultTimingDays)
	} else {
		parsed, err := time.ParseInLocation(sinceLayout, since, time.UTC)
		if err != nil {
			return reportengine.CostOptions{}, fmt.Errorf("report: --since %q is not a date: expected YYYY-MM-DD in UTC", since)
		}
		options.Since = parsed
	}
	if format != formatJSON && format != formatMarkdown {
		return reportengine.CostOptions{}, fmt.Errorf("report: --format %q is not a report form: expected %s or %s", format, formatJSON, formatMarkdown)
	}
	return options, nil
}
