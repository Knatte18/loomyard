// await.go implements the read-only `await` shuttle verb: the capped wait on one or more files, the keep-cache-warm wait any agent can run.
// It only stats the told paths and ends each call by its cap; it writes nothing and reads no file's content.

package shuttlecli

import (
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/spf13/cobra"
)

// awaitVerb is the cobra name of the await verb.
// Command's PersistentPreRunE skips resolution for it, since the verb stats told paths and needs no hub, repository, config or engine.
const awaitVerb = "await"

// awaitCmd builds `lyx shuttle await <path>... [--cap <duration>]`.
// It prints ok with ready true, the absolute path that was met and every absolute path watched as soon as one exists.
// When --cap elapses first it prints ok with ready false, telling the caller to run the command again.
// A relative path resolves against the seam cwd.
// A stat error other than not-exist and a cancelled context are errors with exit 1.
func (c *shuttleCLI) awaitCmd() *cobra.Command {
	var awaitCap time.Duration
	cmd := &cobra.Command{
		Use:         awaitVerb + " <path>...",
		Short:       "wait, capped, for one of the given files to exist",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `await polls the given paths and returns as soon as one of them exists,
printing ok with ready true, the absolute path that was met and every path it
watched. When --cap elapses first it prints ok with ready false and exits 0,
telling the caller to run the command again. It is read-only and needs no hub
or git repository.

A marker is a path like any other: the condition is "this file appears",
whoever writes it. Run the command with a Bash tool timeout above --cap
(300000 ms) and re-run it in a turn that does nothing else, so the prompt
cache stays warm between calls.

Example:
  lyx shuttle await .lyx/ready/marker
  lyx shuttle await /abs/first.md /abs/second.md --cap 30s`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if clihelp.ShouldAbort(ctx) {
				return nil
			}
			out := cmd.OutOrStdout()

			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			watched := make([]string, len(args))
			for i, path := range args {
				if !filepath.IsAbs(path) {
					path = filepath.Join(cwd, path)
				}
				watched[i] = path
			}

			met, ready, err := shuttleengine.AwaitAny(ctx, watched, awaitCap)
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}
			if !ready {
				output.Ok(out, map[string]any{"ready": false, "watched": watched, "message": "none of the watched paths exists yet; call this command again"})
				return nil
			}
			output.Ok(out, map[string]any{"ready": true, "path": met, "watched": watched})
			return nil
		},
	}
	cmd.Flags().DurationVar(&awaitCap, "cap", shuttleengine.DefaultAwaitCap, "longest time one call waits before answering ready false")
	return cmd
}
