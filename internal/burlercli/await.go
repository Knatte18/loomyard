// await.go implements the `await-review` burler verb: the capped, read-only wait on the marker Go writes once a round's review is on disk.

package burlercli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// awaitReviewVerb is the cobra name of the await-review verb.
// resolvePersistentPreRun skips hub, mode and engine wiring for it, since the verb polls one told path.
const awaitReviewVerb = "await-review"

// awaitPollInterval is the fixed pause between two stats of the marker.
const awaitPollInterval = 200 * time.Millisecond

// defaultAwaitCap is the --cap default: below an agent's default two-minute Bash tool timeout, so the call returns before the tool kills it.
const defaultAwaitCap = 100 * time.Second

// awaitReviewCmd builds `lyx burler await-review <marker-path>`.
// It prints ok with ready true and the absolute marker path as soon as the marker exists,
// and ok with ready false when --cap elapses first, telling the caller to run the command again.
// A relative path resolves against the seam cwd.
// A stat error other than not-exist is an error with exit 1.
func (c *burlerCLI) awaitReviewCmd() *cobra.Command {
	var awaitCap time.Duration
	cmd := &cobra.Command{
		Use:   awaitReviewVerb + " <marker-path>",
		Short: "wait, capped, for a round's review-ready marker to exist",
		Long: `await-review polls for one marker path and returns as soon as it exists,
printing ok with ready true and the absolute marker path. When --cap elapses
first it prints ok with ready false and exits 0, telling the caller to run the
command again. A fixer agent runs it until the reviewer's review is ready.
It is read-only and needs no hub or git repository.

Example:
  lyx burler await-review .lyx/reviews/webster/round-1-review.md.ready
  lyx burler await-review /abs/path/round-1-review.md.ready --cap 30s`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				output.Err(out, err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			marker := args[0]
			if !filepath.IsAbs(marker) {
				marker = filepath.Join(cwd, marker)
			}

			deadline := time.Now().Add(awaitCap)
			for {
				_, err := os.Stat(marker)
				if err == nil {
					output.Ok(out, map[string]any{"ready": true, "marker": marker})
					return nil
				}
				if !errors.Is(err, fs.ErrNotExist) {
					output.Err(out, err.Error())
					clihelp.Abort(ctx, 1)
					return nil
				}

				remaining := time.Until(deadline)
				if remaining <= 0 {
					output.Ok(out, map[string]any{"ready": false, "marker": marker, "message": "the review is not ready yet; call this command again"})
					return nil
				}
				select {
				case <-ctx.Done():
					output.Err(out, ctx.Err().Error())
					clihelp.Abort(ctx, 1)
					return nil
				case <-time.After(min(awaitPollInterval, remaining)):
				}
			}
		},
	}
	cmd.Flags().DurationVar(&awaitCap, "cap", defaultAwaitCap, "longest time one call waits before answering ready false")
	return cmd
}
