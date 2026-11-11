// status.go implements the generic `status` verb body: a one-shot JSON envelope of the current
// phase, and (from the status --watch tail, added onto this same file) a --watch mode that tails
// the same status file, printing a line only when the composed activity changes, a growing elapsed time aside.

package shedverbs

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/buildvcs"
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
// The two paths live in different trees: the status file is durable, under `_lym`, while its lock is
// ephemeral, under `.lym`. internal/lock opens a lock file with O_CREATE and never creates a parent,
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

// UnavailableLine composes the line the watch tail prints for the told label when a poll cannot read the status file.
// The line is also its own change key.
// It must be computed once outside the poll loop rather than per poll, since the tail dedupes on that key.
// A line recomposed per poll that ever differed by a byte would turn a transient fault into its own flood, which is the whole thing the dedupe exists to prevent.
func UnavailableLine(label string) string {
	return label + " status unavailable (status file transiently unreadable)"
}

// PrintStatusLinesOnChange runs the watch tail:
// it polls a line and its change key, prints the line into out only when the key differs from the one of the line it last printed, and sleeps between polls.
//
// Suppressing an unchanged line is the whole point.
// A producer call lasts minutes while the tail polls every second.
// Printing every poll fills tmux's scrollback and buries the one line an operator needs, the moment the activity changes, among identical ones.
// The key leaves out what changes on every poll, such as a wait's elapsed time, which the line still shows.
//
// polls bounds the loop so a test can drive a finite sequence with no wall-clock wait;
// a non-positive polls means poll forever, which is what the production call passes.
func PrintStatusLinesOnChange(out io.Writer, poll func() (line, key string), sleep func(), polls int) {
	lastKey := ""
	printedAny := false
	for i := 0; polls <= 0 || i < polls; i++ {
		line, key := poll()
		if !printedAny || key != lastKey {
			fmt.Fprintln(out, line)
			lastKey = key
			printedAny = true
		}
		sleep()
	}
}

// idleGapFloor is the shortest stretch between two history entries that status reports as idle.
// A driver starts the next step within seconds of the last, so a longer stretch is a halt, a wait on a person,
// or a step that ended without recording a verdict.
const idleGapFloor = 5 * time.Minute

// IdleGap is one stretch of a run's history in which no step ran.
// It runs from the end of one entry to the start of the producer call that wrote the next.
// When the next entry is a goto or a skip, which no call writes, it runs to that entry's own time.
// Calls folded into one stuck entry are one stretch of the history, so the time between them is not a gap.
type IdleGap struct {
	// After and AfterOutcome name the entry the gap follows.
	After        string `json:"after"`
	AfterOutcome string `json:"after_outcome"`
	// Before names the row of the entry that ends the gap.
	Before string `json:"before"`
	// From and To are the gap's RFC 3339 bounds, and Idle is its length.
	From string `json:"from"`
	To   string `json:"to"`
	Idle string `json:"idle"`
}

// idleGapsOf returns the gaps of at least idleGapFloor in history, oldest first, and an empty list when there is none.
// An entry ends at its LastAt when a fold set one, and at its At otherwise.
// An entry a producer call wrote with no StartedAt, as one an older binary wrote, ends no gap.
// Neither does an entry whose times do not parse.
func idleGapsOf(history []shedengine.HistoryEntry) []IdleGap {
	gaps := []IdleGap{}
	for i := 1; i < len(history); i++ {
		prev, next := history[i-1], history[i]
		from := cmp.Or(prev.LastAt, prev.At)
		to := next.StartedAt
		if next.Outcome == shedengine.OutcomeGoto || next.Outcome == shedengine.OutcomeSkipped {
			to = next.At
		}
		end, endErr := time.Parse(time.RFC3339, from)
		start, startErr := time.Parse(time.RFC3339, to)
		if endErr != nil || startErr != nil {
			continue
		}
		if idle := start.Sub(end); idle >= idleGapFloor {
			gaps = append(gaps, IdleGap{After: prev.Producer, AfterOutcome: string(prev.Outcome), Before: next.Producer, From: from, To: to, Idle: idle.String()})
		}
	}
	return gaps
}

// waitingNote asks spec.Hooks.Waiting what a running run waits on.
// It returns the zero note when the hook is nil or the state is not running, so the hook never runs against a settled run.
func waitingNote(spec *Spec, st shedengine.Status) (WaitNote, error) {
	if spec.Hooks.Waiting == nil || st.State != shedengine.StateRunning {
		return WaitNote{}, nil
	}
	return spec.Hooks.Waiting(st)
}

// runStatusWatch drives the --watch tail against spec's own status file, using spec.StatusLabel as the rendered line's prefix.
// It is the narrow, explicitly-taken interactive-handoff exception -- everything fallible has already run on the one-shot envelope above.
// A read failure or a !found inside the poll renders the unavailable line, never terminates the tail, and never writes an envelope:
// the pane is expected to survive the driver rewriting the file underneath it.
func runStatusWatch(out io.Writer, spec *Spec, interval time.Duration) {
	poll := func() (string, string) { return statusWatchLine(spec) }
	PrintStatusLinesOnChange(out, poll, func() { time.Sleep(interval) }, 0)
}

// statusWatchLine renders one poll of the --watch tail as a line and its change key.
// The line is the status line with " | waiting <note>" appended while the Waiting hook reports a note.
// The key carries the note's steady form in place of its text.
// A hook error drops the note rather than ending the tail.
func statusWatchLine(spec *Spec) (line, key string) {
	polled, found, err := state.ReadJSON[shedengine.Status](spec.StatusPath, spec.StatusLockPath)
	if err != nil || !found {
		unavailable := UnavailableLine(spec.StatusLabel)
		return unavailable, unavailable
	}
	line = RenderStatusLine(spec.StatusLabel, polled)
	key = line
	if note, noteErr := waitingNote(spec, polled); noteErr == nil && note.Text != "" {
		line += " | waiting " + note.Text
		key += " | waiting " + note.Steady
	}
	return line, key
}

// statusCmd builds the generic `status` subcommand, registering the two flags -- --watch and
// --interval -- the generic body itself reads and no others.
func statusCmd(texts VerbTexts, spec *Spec) *cobra.Command {
	var watch, asJSON bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:         texts.Status.Use,
		Short:       texts.Status.Short,
		Long:        texts.Status.Long,
		Annotations: map[string]string{clihelp.AudienceAnnotation: texts.Status.Audience},
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			out := cmd.OutOrStdout()
			traceDir := logger.TraceDir()

			if watch && asJSON {
				clihelp.SetExit(ctx, output.ErrFields(out, "--watch renders a live line, not JSON; drop --json, or drop --watch for the JSON envelope", map[string]any{"trace_dir": traceDir}))
				return nil
			}

			st, found, err := state.ReadJSON[shedengine.Status](spec.StatusPath, spec.StatusLockPath)
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
				"hand_back":        spec.handsBack(st),
				"restep_hold":      spec.restepHolds(st),
				"pause_reason":     st.PauseReason,
				"activity":         st.Activity,
				"history_length":   len(st.History),
				"idle_gaps":        idleGapsOf(st.History),
				"interrupt_policy": interruptPolicy,
				"trace_dir":        traceDir,
				"run_id":           spec.RunID,
				"progress":         progressOf(spec.Routing, st.CurrentProducer),
				"last_step":        lastStepOf(spec.StepsDir, buildvcs.Running()),
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
			waiting, err := waitingNote(spec, st)
			if err != nil {
				// Reported verbatim, like StatusExtras: the hook owns its whole string.
				clihelp.SetExit(ctx, output.ErrFields(out, err.Error(), map[string]any{"trace_dir": traceDir}))
				return nil
			}
			if waiting.Text != "" {
				core["waiting"] = waiting.Text
			}
			if !asJSON && writerIsTerminal(out) {
				fmt.Fprint(out, RenderStatusHuman(spec.StatusLabel, spec.RunID, st, spec.Routing, waiting.Text))
				return nil
			}
			clihelp.SetExit(ctx, output.Ok(out, core))
			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "print the JSON envelope even on a terminal, where the one-shot status otherwise prints a human view; refused with --watch")
	cmd.Flags().BoolVar(&watch, "watch", false, "tail the status file, printing a line only when the activity changes, a growing elapsed time aside, instead of emitting a single JSON envelope")
	cmd.Flags().DurationVar(&interval, "interval", time.Second, "poll interval for --watch; exists so a test can drive the poll fast without a real wall-clock wait")

	return cmd
}
