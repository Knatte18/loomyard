// await.go implements the read-only `await` shuttle verb: the capped wait on one or more files, the keep-cache-warm wait any agent can run.
// It stats the told paths and ends each call by its cap, which defaults to the caller role's keep-warm cap; it writes nothing and reads no file's content.
// Inside a git worktree it reads the keep-warm config for that cap.

package shuttlecli

import (
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/spf13/cobra"
)

// awaitVerb is the cobra name of the await verb.
// Command's PersistentPreRunE still skips resolution for it, since the verb resolves only its own config dirs when its directory lies in a git worktree and needs no hub.
const awaitVerb = "await"

// awaitCap returns the cap one await call waits.
// A given --cap (flagGiven) wins and neither function is called.
// Otherwise a cwd inside a git worktree answers the caller role's keep-warm cap from keepWarm, and one outside any repository answers shuttleengine.DefaultAwaitCap so the verb needs no hub or repository.
// A fallback answer and an unresolved worktree are logged at Debug.
func awaitCap(cwd, strand string, flagCap time.Duration, flagGiven bool, keepWarm func(configengine.Dirs, string) shuttleengines.KeepWarmAnswer, resolveWorktree func(string) (*lymcwd.Location, error)) time.Duration {
	if flagGiven {
		return flagCap
	}
	location, err := resolveWorktree(cwd)
	if err != nil {
		logger.Debug("shuttle await: no git worktree, waiting the default cap", "cwd", cwd, "error", err)
		return shuttleengine.DefaultAwaitCap
	}
	answer := keepWarm(fabricengine.ConfigDirs(location), strand)
	if answer.Fallback != nil {
		logger.Debug("shuttle await: the keep-warm resolver fell back", "strand", strand, "error", answer.Fallback)
	}
	return answer.Cap
}

// awaitCmd builds `lym shuttle await <path>... [--cap <duration>]`.
// It prints ok with ready true, the absolute path that was met and every absolute path watched as soon as one exists.
// When --cap elapses first it prints ok with ready false, telling the caller to run the command again.
// A relative path resolves against the seam cwd.
// A stat error other than not-exist and a cancelled context are errors with exit 1.
func (c *shuttleCLI) awaitCmd() *cobra.Command {
	var flagCap time.Duration
	cmd := &cobra.Command{
		Use:         awaitVerb + " <path>...",
		Short:       "wait, capped, for one of the given files to exist",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `await polls the given paths and returns as soon as one of them exists,
printing ok with ready true, the absolute path that was met and every path it
watched. When --cap elapses first it prints ok with ready false and exits 0,
telling the caller to run the command again. It is read-only and needs no hub
or git repository. Without --cap the call waits the keep-warm cap of the caller's
role from claude.yaml inside a git worktree, and 250s elsewhere.

A marker is a path like any other: the condition is "this file appears",
whoever writes it. Run the command with a Bash tool timeout of 3600000 ms
and re-run it in a turn that does nothing else, so the prompt
cache stays warm between calls.

Example:
  lym shuttle await .lym/ready/marker
  lym shuttle await /abs/first.md /abs/second.md --cap 30s`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if clihelp.ShouldAbort(ctx) {
				return nil
			}
			out := cmd.OutOrStdout()

			cwd, err := lymcwd.CwdFrom(ctx)
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

			met, ready, err := shuttleengine.AwaitAny(ctx, watched, awaitCap(cwd, os.Getenv(agentname.StrandNameEnv), flagCap, cmd.Flags().Changed("cap"), shuttleengines.KeepWarm, lymcwd.ResolveWorktree))
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
	cmd.Flags().DurationVar(&flagCap, "cap", shuttleengine.DefaultAwaitCap, "longest time one call waits before answering ready false; defaults to the caller role's keep-warm cap")
	return cmd
}
