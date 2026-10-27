// arbiter.go implements the body of `lym loom arbiter`.
// At a stop the run's driver cannot settle, it hands the stop to the shed-layer arbiter call and prints the validated verdict.
// The verb works over injected seams, so every branch is testable with a fake shuttle and clock; the wiring that adapts the real ones sits beside it.

package loomcli

import (
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/arbiterstop"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedarbiter"
	"github.com/Knatte18/loomyard/internal/sheddriver"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// arbiterVerb runs `lym loom arbiter` over deps and prints its envelope to out, returning the exit code.
// It spawns at most Settings.MaxSpawns agents per run, counted from the attempt records on disk so no restart resets the count, and moves no run itself.
// A re-run for the stop an attempt already arbitrated spawns nothing: it returns that attempt's verdict, or waits on its live arbiter.
func arbiterVerb(out io.Writer, deps shedarbiter.Deps, in shedarbiter.Input) int {
	call, err := shedarbiter.NewCall(deps, in)
	if err != nil {
		return output.Err(out, err.Error())
	}
	envelope, err := call.Settle()
	if err != nil {
		return output.Err(out, err.Error())
	}
	return output.Ok(out, arbiterEnvelopeFields(envelope))
}

// arbiterEnvelopeFields renders envelope as the verb's output keys; relaunch appears for a resume only.
func arbiterEnvelopeFields(e loomengine.ArbiterEnvelope) map[string]any {
	verbs := e.Verbs
	if verbs == nil {
		verbs = []string{}
	}
	fields := map[string]any{
		"verdict":      e.Verdict,
		"reason":       e.Reason,
		"question":     e.Question,
		"goto":         e.Goto,
		"verbs":        verbs,
		"cause":        e.Cause,
		"attempt":      e.Attempt,
		"attempt_dir":  e.AttemptDir,
		"verdict_path": e.VerdictPath,
	}
	if e.Verdict == loomengine.ArbiterVerdictResume {
		fields["relaunch"] = e.Relaunch
	}
	return fields
}

// arbiterCmd builds the `arbiter` subcommand.
func (c *loomCLI) arbiterCmd() *cobra.Command {
	var envelopePath, reportPath string
	cmd := &cobra.Command{
		Use:         "arbiter <run-id>",
		Short:       "settle a stop the run's driver cannot, through a cold arbiter agent",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `arbiter is run only by the run's driver, at a stop it cannot settle. It spawns
a cold arbiter agent, waits for its verdict file and prints the validated verdict:
verdict (resume or hand-on), reason, question, goto, verbs, cause (empty for the
agent's own verdict), attempt, attempt_dir, verdict_path and, on a resume only,
the relaunch line the driver runs next.

A re-run for a stop an arbiter already judged spawns nothing: it returns that
verdict, or waits on the arbiter still running. Each refusal names its way
forward, the hand-back to the run's parent.

Example:
  lym loom arbiter <run-id> --envelope <stop-envelope> --report <stop-report>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()
			deps, err := c.arbiterDepsFor()
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}
			clihelp.SetExit(cmd.Context(), arbiterVerb(out, deps, shedarbiter.Input{RunID: args[0], EnvelopePath: envelopePath, ReportPath: reportPath}))
			return nil
		},
	}
	cmd.Flags().StringVar(&envelopePath, "envelope", "", "the stop envelope the driver saved")
	cmd.Flags().StringVar(&reportPath, "report", "", "the driver's stop or fork report, when there is one")
	_ = cmd.MarkFlagRequired("envelope")
	return cmd
}

// arbiterDepsFor assembles the verb's seams for the run the pre-run armed.
// The arbiter's settings are shed.yaml's under the run's recipe override, so a darn run takes darn.yaml's keys and a loom or beam run loom.yaml's.
// Every closure reads on demand and opens nothing at build time.
func (c *loomCLI) arbiterDepsFor() (shedarbiter.Deps, error) {
	dirs := fabricengine.ConfigDirs(c.location)
	shedConfig, err := sheddriver.LoadConfig(dirs)
	if err != nil {
		return shedarbiter.Deps{}, err
	}
	override, err := loomengine.RecipeOverride(dirs, c.recipe)
	if err != nil {
		return shedarbiter.Deps{}, err
	}
	settings, err := sheddriver.ResolveArbiter(shedConfig, override, c.registry)
	if err != nil {
		return shedarbiter.Deps{}, err
	}
	recipeTarget, err := loomrecipe.AmendmentTarget(c.recipe)
	if err != nil {
		return shedarbiter.Deps{}, err
	}
	location := c.location
	runID := shedrun.ResolveRunID(location, c.runID)
	planApproved := planApprovedReader(location.AnchorPath())
	reviewsDir := loomengine.LoomReviewsDir(location)
	bouncerRunDir := func(producer string) (string, bool, error) {
		subdir, isBouncer, err := c.bouncerSubdir(producer)
		if err != nil || !isBouncer {
			return "", false, err
		}
		return filepath.Join(reviewsDir, subdir), true, nil
	}
	return shedarbiter.Deps{
		ReadStatus:  circlingStatusReader(c.shedPaths.StatusPath, c.shedPaths.StatusLockPath),
		Settings:    settings,
		ArbiterDir:  shedrun.ArbiterDir(location, c.runID),
		AttemptDir:  func(number int) string { return shedrun.ArbiterAttemptDir(location, c.runID, number) },
		MarkerPath:  shedrun.ArbiterMarker(location, c.runID),
		Producers:   c.routing.Producers,
		GotoGuard:   c.settledGuard(),
		StencilsDir: c.runDeps.Geom.StencilsDir,
		Stop:        arbiterstop.NewReaders(location),
		PendingCirclingDecision: func(producer string) (bool, error) {
			runDir, isBouncer, err := bouncerRunDir(producer)
			if err != nil || !isBouncer {
				return false, err
			}
			return shedadapters.PendingCirclingDecision(runDir)
		},
		Prompt: shedarbiter.PromptInput{
			Recipe:             arbiterRecipeDescription(c.recipe),
			Slug:               seedSlug(location.WorktreeName),
			Worktree:           location.WorktreePath(),
			DecisionRecordPath: loomengine.DiscussionDecisionRecord(location),
			SupportLogPath:     loomengine.DiscussionSupportLog(location),
		},
		AmendmentTarget:    func() string { return arbiterAmendmentTarget(recipeTarget, planApproved) },
		Shuttle:            sheddriver.RunnerShuttle{Runner: c.runner},
		Now:                time.Now,
		RelaunchRunID:      runID,
		HandBackWayForward: arbiterHandBackWayForward,
	}, nil
}

// arbiterAmendmentTarget returns recipeTarget when it is non-empty and approved answers true, and the literal "none" otherwise.
// An approved error answers "none", since a target the arbiter cannot rely on is no target.
func arbiterAmendmentTarget(recipeTarget string, approved func() (bool, error)) string {
	if recipeTarget == "" {
		return arbiterAbsentPath
	}
	ok, err := approved()
	if err != nil || !ok {
		return arbiterAbsentPath
	}
	return recipeTarget
}
