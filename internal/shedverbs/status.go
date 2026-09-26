// status.go implements the generic `status` verb body: a one-shot JSON envelope of the current
// phase, and (from the status --watch tail, added onto this same file) a --watch mode that tails
// the same status file, printing a line only when the composed activity actually changes.

package shedverbs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
)

// ensureStatusLockDir creates the parent directory of the status file's advisory lock, the
// ephemeral tree the status body and pause must both ensure before touching internal/state at all.
//
// The two paths live in different trees: the status file is durable, under `_lyx`, while its lock is
// ephemeral, under `.lyx`. internal/lock opens a lock file with O_CREATE and never creates a parent,
// which is why shedengine.preflight MkdirAlls both lock parents on every Run and Step. Nothing
// creates the ephemeral directory before a bootstrap has run, so without this both verbs' carefully-
// worded "no status file ... run ..." remedy messages were unreachable on the one path they exist
// for -- a never-bootstrapped pair instead failed inside lock acquisition with a raw "no such file or
// directory" and reported none of that remedy.
//
// prefix reuses spec.DecodeErrPrefix rather than a second told field: reusing it reproduces
// loomcli's existing "loom: create the status lock's directory %s: %w" text byte-for-byte for
// loom's spec, rather than inventing a second prefix field for one string.
//
// This creates a directory and reads nothing, so it cannot resurrect a deleted status file or mask a
// genuine absence -- found still answers that question, and now actually gets asked.
func ensureStatusLockDir(prefix, statusLockPath string) error {
	dir := filepath.Dir(statusLockPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("%s create the status lock's directory %s: %w", prefix, dir, err)
	}
	return nil
}

// RenderStatusLine composes st into exactly one line: the told label, then the state, then " | now
// " and the activity's now field, then " | last " and the last field only when it is non-empty,
// then " | wait " and the wait field only when it is non-empty.
//
// For label "loom" this renders byte-identically to internal/loomcli's own existing unexported
// renderStatusLine, whose format is "loom %s | now %s" plus the two optional tails -- loomcli's own
// step_test.go and status_test.go pin that exact rendering, and batch 4 retargets those tables onto
// this function rather than deleting them.
func RenderStatusLine(label string, st shedengine.Status) string {
	line := fmt.Sprintf("%s %s | now %s", label, st.State, st.Activity.Now)
	if st.Activity.Last != "" {
		line += " | last " + st.Activity.Last
	}
	if st.Activity.Wait != "" {
		line += " | wait " + st.Activity.Wait
	}
	return line
}

// UnavailableLine composes the line the watch tail prints when a poll cannot read the status file,
// for the told label. It must be computed once outside the poll loop rather than per poll: the tail
// dedupes on printed text, so a line recomposed per poll that ever differed by a byte would turn a
// transient fault into its own flood, which is the whole thing the dedupe exists to prevent.
func UnavailableLine(label string) string {
	return label + " status unavailable (status file transiently unreadable)"
}

// PrintStatusLinesOnChange runs the watch tail: it polls, prints the polled line into out only when
// that line differs from the one it last printed, and sleeps between polls.
//
// Suppressing an unchanged line is the whole point. A producer call lasts minutes while the tail
// polls every second, so printing unconditionally fills tmux's scrollback and buries the one line an
// operator actually needs -- the moment the activity changes -- among the identical ones around it.
//
// polls bounds the loop so a test can drive a finite sequence with no wall-clock wait; a
// non-positive polls means poll forever, which is what the production call passes.
func PrintStatusLinesOnChange(out io.Writer, poll func() string, sleep func(), polls int) {
	lastPrinted := ""
	printedAny := false
	for i := 0; polls <= 0 || i < polls; i++ {
		line := poll()
		if !printedAny || line != lastPrinted {
			fmt.Fprintln(out, line)
			lastPrinted = line
			printedAny = true
		}
		sleep()
	}
}

// runStatusWatch drives the --watch tail against spec's own status file, using spec.StatusLabel as
// the rendered line's prefix. It is the narrow, explicitly-taken interactive-handoff exception --
// everything fallible has already run on the one-shot envelope above. A read failure or a !found
// inside the poll closure returns the precomputed unavailable line, never terminates the tail, and
// never writes an envelope: the pane is expected to survive the driver rewriting the file underneath
// it.
func runStatusWatch(out io.Writer, spec *Spec, interval time.Duration) {
	unavailable := UnavailableLine(spec.StatusLabel)
	poll := func() string {
		polled, polledFound, pollErr := state.ReadJSONStrict[shedengine.Status](spec.StatusPath, spec.StatusLockPath)
		if pollErr != nil || !polledFound {
			return unavailable
		}
		return RenderStatusLine(spec.StatusLabel, polled)
	}
	PrintStatusLinesOnChange(out, poll, func() { time.Sleep(interval) }, 0)
}

// statusCmd builds the generic `status` subcommand, registering the two flags -- --watch and
// --interval -- the generic body itself reads and no others.
func statusCmd(texts VerbTexts, spec *Spec) *cobra.Command {
	var watch bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   texts.Status.Use,
		Short: texts.Status.Short,
		Long:  texts.Status.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			traceDir := logger.TraceDir()

			if spec.EnsureStatusLockDir {
				if err := ensureStatusLockDir(spec.DecodeErrPrefix, spec.StatusLockPath); err != nil {
					clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"trace_dir": traceDir}))
					return nil
				}
			}

			st, found, err := state.ReadJSONStrict[shedengine.Status](spec.StatusPath, spec.StatusLockPath)
			if err != nil {
				clihelp.SetExit(ctx, output.ErrFields(out, spec.DecodeErrPrefix+" decode status file "+spec.StatusPath+": "+err.Error(), map[string]any{"trace_dir": traceDir}))
				return nil
			}
			if !found {
				// trace_dir is the one generic key this branch carries: StatusExtras still never
				// runs against a zero shedengine.Status, and found: false is the one discriminator
				// both absent dispositions share.
				if spec.AbsentStatus.Refuse {
					clihelp.SetExit(ctx, output.ErrFields(out, spec.AbsentStatus.RefuseMessage, map[string]any{"found": false, "trace_dir": traceDir}))
					return nil
				}
				clihelp.SetExit(ctx, output.Ok(out, map[string]any{
					"found":       false,
					"status_path": spec.StatusPath,
					"trace_dir":   traceDir,
				}))
				return nil
			}

			if watch {
				runStatusWatch(out, spec, interval)
			}

			interruptPolicy := ""
			if spec.Hooks.InterruptPolicyFor != nil {
				interruptPolicy = spec.Hooks.InterruptPolicyFor(st.CurrentProducer)
			}
			core := map[string]any{
				"current_producer": st.CurrentProducer,
				"state":            string(st.State),
				"error":            st.Error,
				"activity":         st.Activity,
				"history_length":   len(st.History),
				"interrupt_policy": interruptPolicy,
				"trace_dir":        traceDir,
			}
			if spec.Hooks.StatusExtras != nil {
				extras, err := spec.Hooks.StatusExtras(st)
				if err != nil {
					// Reported verbatim, with no re-prefixing: the hook owns its whole string.
					clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"trace_dir": traceDir}))
					return nil
				}
				for k, v := range extras {
					core[k] = v
				}
			}
			clihelp.SetExit(ctx, output.Ok(out, core))
			return nil
		},
	}

	cmd.Flags().BoolVar(&watch, "watch", false, "tail the status file, printing a line only when the activity changes, instead of emitting a single JSON envelope")
	cmd.Flags().DurationVar(&interval, "interval", time.Second, "poll interval for --watch; exists so a test can drive the poll fast without a real wall-clock wait")

	return cmd
}
