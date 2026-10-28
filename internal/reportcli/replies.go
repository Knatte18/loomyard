// replies.go is the `report replies` verb: it resolves the prime, reads the creel journal as the REPORT program and hands its entries to the report engine to count per receiver role.

package reportcli

import (
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/creelengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reportengine"
	"github.com/spf13/cobra"
)

// reportProgram is the program name the verb signs its creel log call with.
const reportProgram = "REPORT"

// creelLog reads the creel journal for the replies verb; a test replaces it with a fake.
var creelLog = readCreelLog

// readCreelLog reads every entry of the hub's creel journal as the REPORT program, which claims no strand.
func readCreelLog(location *lymcwd.Location) ([]creelengine.LogEntry, error) {
	client, err := creelcli.NewProgramClient(location, reportProgram)
	if err != nil {
		return nil, err
	}
	return client.Log(creelengine.LogFilter{})
}

// repliesCommand returns the `report replies` verb.
func repliesCommand() *cobra.Command {
	return &cobra.Command{
		Use:         "replies",
		Short:       "report, per receiver role, the replies creel was asked for, the replies it got, its chases and the messages left unanswered",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `replies reads the creel journal, which keeps seven days, and counts per
receiver role the posts that expected a reply, the replies the role sent, the
chases creel sent it and the messages it left unanswered, so the stencils of the
roles that often need chasing can be tuned.

It runs from the hub's prime worktree only, as an operator's terminal or an orch
pane, reads the journal and writes nothing.

Example:
  lym report replies`,
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

			location, err := resolvePrime(ctx, "replies")
			if err != nil {
				return fail(err)
			}
			entries, err := creelLog(location)
			if err != nil {
				return fail(err)
			}
			clihelp.SetExit(ctx, output.Ok(out, map[string]any{"roles": reportengine.ComputeReplies(entries)}))
			return nil
		},
	}
}
