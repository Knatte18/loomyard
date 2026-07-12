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
	"log"
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

// statReportPath is the seam gather's two report-existence checks (the
// primary check and the pre-dead-classification re-check) both call instead
// of os.Stat directly, mirroring pollRealClock's own test-injection pattern:
// production always uses os.Stat, but a test can override this package
// variable to script a distinct result per call -- in particular, a
// non-ENOENT error on exactly the second (re-check) call, which a real
// filesystem race cannot be scripted to reproduce deterministically.
var statReportPath = os.Stat

// digestFields converts a Digest into the map output.Ok expects: Digest's
// own json tags already spell the pinned snake_case field names, so a
// marshal/unmarshal round trip through map[string]any reuses them exactly
// rather than re-listing every field by hand here. It then enforces the
// digest contract's presence rules, which struct tags alone cannot express:
// files_changed and dirty are "terminal, report-backed" fields — a running
// or dead snapshot never measured them, so emitting a zero there would be a
// false statement, while omitempty would wrongly drop a legitimate terminal
// zero — and a running snapshot always carries elapsed_s, including the
// omitempty-hostile 0 of its first second.
func digestFields(d builderengine.Digest) map[string]any {
	data, _ := json.Marshal(d)
	var fields map[string]any
	_ = json.Unmarshal(data, &fields)

	if d.Status != builderengine.DigestStatusDone && d.Status != builderengine.DigestStatusStuck {
		delete(fields, "files_changed")
		delete(fields, "dirty")
	}
	if d.Status == builderengine.DigestStatusRunning {
		fields["elapsed_s"] = d.ElapsedS
	}
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

			// gatherReport fills ins's report-present branch: parse the report
			// and compute the digest's git-backed inputs. The gitquery helpers
			// (ChangedFiles/Dirty) run ONLY here -- a running tick must never
			// shell out to git.
			gatherReport := func(ins *builderengine.ClassifyInputs) error {
				report, err := builderengine.ParseReport(reportPath)
				if err != nil {
					return err
				}
				// plan-format.md pins the report's batch: field to the
				// NN-<batch-slug> stem of its own filename, and Distill
				// passes it verbatim into the digest's batch field — the one
				// field the orchestrator navigates by. A mismatch (a typo'd
				// or copy-pasted stem) is a malformed report and fails loud
				// here, the same discipline ParseReport applies to every
				// other field, never a silently mislabeled digest.
				expectedBatch := fmt.Sprintf("%02d-%s", batchNumber, bs.Slug)
				if report.Batch != expectedBatch {
					return fmt.Errorf("builder: batch report %s: batch field %q does not match this batch's own identifier %q", reportPath, report.Batch, expectedBatch)
				}
				changed, err := builderengine.ChangedFiles(c.layout.Cwd, bs.StartSHA)
				if err != nil {
					return err
				}
				dirty, err := builderengine.Dirty(c.layout.Cwd)
				if err != nil {
					return err
				}
				ins.Report = report
				ins.Changed = changed
				ins.Scope = scope
				ins.Dirty = dirty
				return nil
			}

			// gather is Classify's per-tick input assembler: it always checks
			// for the report FIRST, and re-checks it before ever returning a
			// dead classification -- a report written between the first stat
			// and the (slower) events/mux gathers must win over a
			// simultaneously-true Stop/timeout/died condition, or the
			// orchestrator's next respawn is refused on the very report this
			// tick ignored.
			gather := func() (builderengine.Digest, bool, error) {
				ins := builderengine.ClassifyInputs{
					BatchNumber:  batchNumber,
					BatchSlug:    bs.Slug,
					ReportPath:   reportPath,
					BatchTimeout: batchTimeout,
					Elapsed:      time.Since(spawnedAt),
				}

				if _, statErr := statReportPath(reportPath); statErr == nil {
					if err := gatherReport(&ins); err != nil {
						return builderengine.Digest{}, false, err
					}
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

				// Report-present must win for real, not just in decision
				// order: the implementer writes its report BEFORE its turn
				// ends, so a Stop event observed above can postdate a report
				// that landed after the first stat. Re-stat before returning
				// any dead classification and let the report re-classify. A
				// non-ENOENT stat error here gets the same fail-loud treatment
				// as the primary stat above: silently falling through to the
				// dead classification on a transient stat failure could mask a
				// report that actually landed, the exact false positive this
				// re-check exists to prevent.
				if ins.Report == nil && digest.Status == builderengine.DigestStatusDead {
					if _, statErr := statReportPath(reportPath); statErr == nil {
						if err := gatherReport(&ins); err != nil {
							return builderengine.Digest{}, false, err
						}
						digest, terminal = builderengine.Classify(ins)
					} else if !os.IsNotExist(statErr) {
						return builderengine.Digest{}, false, statErr
					}
				}

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

			// Persist the terminal classification onto a FRESH state loaded
			// under the state-mutation lease, never onto the copy loaded at
			// poll entry: the long-poll can block for minutes, and a
			// spawn-batch that landed inside that window (its own SaveState
			// recording a new batch) would be silently erased by saving the
			// stale entry-time copy — the lost-update this lease exists to
			// prevent. CurrentBatch clears to 0 only if it still points at
			// the batch this poll classified; a concurrently-spawned batch's
			// cursor is not this poll's to reset.
			mutateLock, err := builderengine.AcquireStateMutation(c.builderDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			freshState, err := builderengine.LoadState(c.builderDir)
			if err != nil || freshState == nil {
				_ = mutateLock.Release()
				if err == nil {
					err = fmt.Errorf("builder: state.json vanished while poll was classifying batch %02d-%s", batchNumber, bs.Slug)
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			if freshBatch, ok := freshState.Batches[batchNumber]; ok {
				freshBatch.Terminal = true
				freshBatch.Status = digest.Status
			}
			if freshState.CurrentBatch == batchNumber {
				freshState.CurrentBatch = 0
			}
			if err := builderengine.SaveState(c.builderDir, freshState); err != nil {
				_ = mutateLock.Release()
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			_ = mutateLock.Release()

			// Cleanup on the report-backed terminals, mirroring shuttle's own
			// finalize (wait.go): nobody holds the shuttle Run handle across a
			// batch (spawn-batch exits right after Start), so poll is the only
			// place the strand can ever be released — without this every
			// done/stuck batch leaks a live pane hosting an idle agent process
			// forever (found live in round fable-r2). done also removes the
			// run dir (shuttle parity); stuck keeps it, since its raw session
			// output is the one diagnostic trail a human may still want. Every
			// dead classification keeps BOTH pane and run dir — the doc-pinned
			// diagnosis discipline. Cleanup failures are logged, never fatal:
			// the classification itself already stands (shuttle's precedent).
			if digest.Status == builderengine.DigestStatusDone || digest.Status == builderengine.DigestStatusStuck {
				if _, err := c.mux.RemoveStrand(bs.StrandGUID, false); err != nil {
					log.Printf("builder: poll cleanup: remove strand %s (non-fatal): %v", bs.StrandGUID, err)
				}
				if digest.Status == builderengine.DigestStatusDone {
					if err := os.RemoveAll(bs.ShuttleRunDir); err != nil {
						log.Printf("builder: poll cleanup: remove run dir %s (non-fatal): %v", bs.ShuttleRunDir, err)
					}
				}
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
