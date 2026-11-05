// raddle.go is the `report raddle` verb: it validates its flag, resolves the prime and hands its snapshot to the report engine.

package reportcli

import (
	"fmt"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reportengine"
	"github.com/spf13/cobra"
)

// raddleCommand returns the `report raddle` verb.
func raddleCommand() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:         "raddle",
		Short:       "report each filed Sketch's predicted cost against what its filed parts cost",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `raddle reads the raddle store on the board branch and reports, per Sketch a
raddle run filed, the cost its approved proposal predicted, the summed session
cost of its filed parts' runs, the parts not yet landed and whether the approved
proposal differs from the cold one. An actual cost that leaves out a session of
unknown cost is marked partial.

  --format <form>     json (default) or markdown

It runs from the hub's prime worktree only, reads git history and writes nothing.

Example:
  lym report raddle --format markdown`,
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

			if format != formatJSON && format != formatMarkdown {
				return fail(fmt.Errorf("report: --format %q is not a report form: expected %s or %s", format, formatJSON, formatMarkdown))
			}
			location, err := resolvePrime(ctx, "raddle")
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
			report, err := reportengine.ComputeRaddle(sources)
			if err != nil {
				return fail(err)
			}

			if format == formatMarkdown {
				fmt.Fprint(out, reportengine.RenderRaddleMarkdown(report))
				return nil
			}
			clihelp.SetExit(ctx, output.Ok(out, map[string]any{
				"selection": report.Selection,
				"rows":      report.Rows,
				"findings":  report.Findings,
			}))
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", formatJSON, "output form: json or markdown")
	return cmd
}
