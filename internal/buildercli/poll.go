// poll.go implements the `poll` builder verb: assembles Classify's inputs
// for the current in-flight batch (report parse; whether the implementer's
// turn has ended, via the run dir's events.jsonl and the claude engine
// PersistentPreRunE already constructed; whether its mux strand is still
// live, via the mux engine's own live Status() query; and elapsed time
// since spawn), computing diff/dirty via the gitquery helpers LAZILY --
// only inside the report-present branch, since a running tick must never
// run git -- and blocks on builderengine.PollUntilTerminal. A terminal
// digest marks the batch terminal in state, persists it, and weft-commits
// the report plus state.json (the second of the loop's three weft-commit
// points); a deadline "running" snapshot is returned as-is, with no weft
// commit and no git diff.

package buildercli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// pollRealClock is buildercli's own production clock for
// builderengine.PollUntilTerminal: PollUntilTerminal's clk parameter is an
// unexported interface, but Go's structural interface satisfaction lets any
// type whose method set matches (Now/Sleep here) be passed across the
// package boundary without ever naming the interface.
type pollRealClock struct{}

func (pollRealClock) Now() time.Time        { return time.Now() }
func (pollRealClock) Sleep(d time.Duration) { time.Sleep(d) }

// digestFields converts a Digest into the map output.Ok expects: Digest's
// own json tags already spell the pinned snake_case field names, so a
// marshal/unmarshal round trip through map[string]any reuses them exactly
// rather than re-listing every field by hand here.
func digestFields(d builderengine.Digest) map[string]any {
	data, _ := json.Marshal(d)
	var fields map[string]any
	_ = json.Unmarshal(data, &fields)
	return fields
}

// pollCmd builds the `poll` subcommand.
func (c *builderCLI) pollCmd() *cobra.Command {
	var wait time.Duration

	cmd := &cobra.Command{
		Use:   "poll",
		Short: "long-poll the in-flight batch's implementer for its terminal digest",
		Long: `poll blocks inside Go, watching the in-flight batch's implementer for a
terminal classification -- report present (done/stuck), the implementer's
turn ended without ever writing a report (dead: asking), elapsed since
spawn past batch_timeout_min (dead: timeout), or its mux strand gone
(dead: died) -- returning the instant one is reached. If --wait elapses
first it returns a running snapshot {batch, status, elapsed_s} instead; the
orchestrator's next poll call re-polls from there. A terminal poll weft-
commits the batch report and state.json; a running snapshot never touches
git or weft.

Example:
  lyx builder poll --wait 8m`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			st, err := builderengine.LoadState(c.builderDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if st == nil || st.CurrentBatch == 0 {
				clihelp.SetExit(cmd.Context(), output.Err(out, "builder: no batch in flight"))
				return nil
			}

			batchNumber := st.CurrentBatch
			bs, ok := st.Batches[batchNumber]
			if !ok {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("builder: no BatchState recorded for in-flight batch %d", batchNumber)))
				return nil
			}

			plan, err := builderengine.ParsePlan(c.planDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			var scope []string
			for _, b := range plan.Batches {
				if b.Number == batchNumber {
					scope = b.Scope
					break
				}
			}

			spawnedAt, err := time.Parse(time.RFC3339, bs.SpawnedAt)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("builder: parse spawnedAt %q for batch %d: %v", bs.SpawnedAt, batchNumber, err)))
				return nil
			}

			reportPath := filepath.Join(c.reportsDir, builderengine.BatchReportFileName(batchNumber, bs.Slug))
			batchTimeout := time.Duration(c.cfg.BatchTimeoutMin) * time.Minute

			// gather is Classify's per-tick input assembler: it always
			// checks for the report FIRST and runs the gitquery helpers
			// (ChangedFiles/Dirty) ONLY inside that report-present branch --
			// a running tick must never shell out to git.
			gather := func() (builderengine.Digest, bool, error) {
				ins := builderengine.ClassifyInputs{
					BatchNumber:  batchNumber,
					BatchSlug:    bs.Slug,
					ReportPath:   reportPath,
					BatchTimeout: batchTimeout,
					Elapsed:      time.Since(spawnedAt),
				}

				if _, statErr := os.Stat(reportPath); statErr == nil {
					report, rerr := builderengine.ParseReport(reportPath)
					if rerr != nil {
						return builderengine.Digest{}, false, rerr
					}
					changed, cerr := builderengine.ChangedFiles(c.layout.Cwd, bs.StartSHA)
					if cerr != nil {
						return builderengine.Digest{}, false, cerr
					}
					dirty, derr := builderengine.Dirty(c.layout.Cwd)
					if derr != nil {
						return builderengine.Digest{}, false, derr
					}
					ins.Report = report
					ins.Changed = changed
					ins.Scope = scope
					ins.Dirty = dirty
				} else if !os.IsNotExist(statErr) {
					return builderengine.Digest{}, false, statErr
				} else {
					turnEnded, terr := builderengine.TurnEnded(bs.EventsPath, c.engine)
					if terr != nil {
						return builderengine.Digest{}, false, terr
					}
					strandLive, serr := builderengine.StrandLive(c.mux, bs.StrandGUID)
					if serr != nil {
						return builderengine.Digest{}, false, serr
					}
					ins.TurnEnded = turnEnded
					ins.StrandLive = strandLive
				}

				digest, terminal := builderengine.Classify(ins)
				return digest, terminal, nil
			}

			waitBudget := wait
			if waitBudget == 0 {
				waitBudget = time.Duration(c.cfg.PollWaitS) * time.Second
			}

			digest, err := builderengine.PollUntilTerminal(gather, waitBudget, pollRealClock{})
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			if digest.Status == builderengine.DigestStatusRunning {
				clihelp.SetExit(cmd.Context(), output.Ok(out, digestFields(digest)))
				return nil
			}

			bs.Terminal = true
			bs.Status = digest.Status
			st.Batches[batchNumber] = bs
			if err := builderengine.SaveState(c.builderDir, st); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			if _, weftErr := weftCommit(c.layout, fmt.Sprintf("poll %02d-%s %s", batchNumber, bs.Slug, digest.Status)); weftErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("builder: batch %02d-%s classified %s but the weft sync failed: %v", batchNumber, bs.Slug, digest.Status, weftErr)))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, digestFields(digest)))
			return nil
		},
	}

	cmd.Flags().DurationVar(&wait, "wait", 0, "long-poll wait budget before returning a running snapshot; 0 defers to builder.yaml's poll_wait_s")

	return cmd
}
