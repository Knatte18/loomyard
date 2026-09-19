// pause.go implements the generic `pause` verb body: it sets the shared pause-requested flag the
// running phase machine consumes at its next producer boundary, and nothing else.

package shedverbs

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
)

// pauseCmd builds the generic `pause` subcommand. pause never calls spec.BuildShed -- a nil one is
// legal here, exactly as on status.
func pauseCmd(texts VerbTexts, spec *Spec) *cobra.Command {
	return &cobra.Command{
		Use:   texts.Pause.Use,
		Short: texts.Pause.Short,
		Long:  texts.Pause.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			if spec.EnsureStatusLockDir {
				if err := ensureStatusLockDir(spec.DecodeErrPrefix, spec.StatusLockPath); err != nil {
					clihelp.SetExit(ctx, output.Err(out, err.Error()))
					return nil
				}
			}

			err := state.UpdateJSON(spec.StatusPath, spec.StatusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
				if !found {
					return shedengine.Status{}, errors.New(spec.PauseAbsentMessage)
				}
				cur.PauseRequested = true
				return cur, nil
			})
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			clihelp.SetExit(ctx, output.Ok(out, map[string]any{
				"status_file": spec.StatusPath,
			}))
			return nil
		},
	}
}
