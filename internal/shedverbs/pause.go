// pause.go implements the generic `pause` verb body: with no flag it sets the shared pause-requested flag the running phase machine consumes at its next producer boundary, and with --before, --after or --clear it records or removes a stop condition the engine fires at a named producer.

package shedverbs

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
)

// pauseFlags are the values of the pause verb's three flags.
type pauseFlags struct {
	// before and after name the producer a condition is recorded for, empty when the flag is absent.
	before string
	after  string
	// clear removes both recorded conditions.
	clear bool
}

// conditional reports whether any flag is present.
func (f pauseFlags) conditional() bool {
	return f.before != "" || f.after != "" || f.clear
}

// applyPauseConditions returns cur with the flags applied, and the envelope keys the answer adds.
// --before and --after record the condition in their slot, replacing a recorded one, and leave pause_requested alone.
// --clear removes both conditions and leaves pause_requested alone.
// A contradictory flag set, or a target naming no producer, is an error and nothing is applied.
func applyPauseConditions(cur shedengine.Status, flags pauseFlags, producers []shedengine.ProducerDef) (shedengine.Status, map[string]any, error) {
	if flags.clear && (flags.before != "" || flags.after != "") {
		return cur, nil, errors.New("shedverbs: pause --clear cannot be combined with --before or --after; way forward: run pause --clear alone, then pause --before or --after")
	}
	names := make([]string, len(producers))
	for i, def := range producers {
		names[i] = def.Name
	}
	for _, target := range []string{flags.before, flags.after} {
		if target == "" {
			continue
		}
		if !slices.Contains(names, target) {
			return cur, nil, fmt.Errorf("shedverbs: pause target %s names no producer; way forward: re-run pause with --before or --after naming one of: %s", target, strings.Join(names, ", "))
		}
	}

	extras := map[string]any{}
	if flags.clear {
		if cur.PauseBefore != "" {
			extras["removed_before"] = cur.PauseBefore
		}
		if cur.PauseAfter != "" {
			extras["removed_after"] = cur.PauseAfter
		}
		cur.PauseBefore = ""
		cur.PauseAfter = ""
	}
	if flags.before != "" {
		if cur.PauseBefore != "" {
			extras["replaced_before"] = cur.PauseBefore
		}
		cur.PauseBefore = flags.before
	}
	if flags.after != "" {
		if cur.PauseAfter != "" {
			extras["replaced_after"] = cur.PauseAfter
		}
		cur.PauseAfter = flags.after
	}
	extras["before"] = cur.PauseBefore
	extras["after"] = cur.PauseAfter
	return cur, extras, nil
}

// pauseCmd builds the generic `pause` subcommand. pause never calls spec.BuildShed -- a nil one is
// legal here, exactly as on status.
func pauseCmd(texts VerbTexts, spec *Spec) *cobra.Command {
	var flags pauseFlags
	cmd := &cobra.Command{
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

			var extras map[string]any
			err := state.UpdateJSON(spec.StatusPath, spec.StatusLockPath, func(cur shedengine.Status, found bool) (shedengine.Status, error) {
				if !found {
					return shedengine.Status{}, errors.New(spec.PauseAbsentMessage)
				}
				if !flags.conditional() {
					cur.PauseRequested = true
					return cur, nil
				}
				applied, added, err := applyPauseConditions(cur, flags, spec.Routing.Producers)
				extras = added
				return applied, err
			})
			if err != nil {
				clihelp.SetExit(ctx, output.Err(out, err.Error()))
				return nil
			}

			envelope := map[string]any{"status_file": spec.StatusPath}
			for key, value := range extras {
				envelope[key] = value
			}
			clihelp.SetExit(ctx, output.Ok(out, envelope))
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.before, "before", "", "pause before this producer runs")
	cmd.Flags().StringVar(&flags.after, "after", "", "pause after this producer returns a routed running outcome")
	cmd.Flags().BoolVar(&flags.clear, "clear", false, "remove both recorded pause conditions")
	return cmd
}
