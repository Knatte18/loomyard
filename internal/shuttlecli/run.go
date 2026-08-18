// run.go implements the `run` shuttle verb: the flag-to-Spec mapper that turns a "lyx shuttle run"
// invocation into a blocking shuttleengine.Runner.Run call and prints its classified outcome as a
// single JSON envelope.

package shuttlecli

import (
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/spf13/cobra"
)

// identityFields returns the error-envelope fields naming the run a failed Run call had already
// started, or nil when it failed before any strand existed (a flag, config, or validation error,
// where every field would be empty and an all-empty-string envelope only adds noise).
func identityFields(result shuttleengine.Result) map[string]any {
	if result.StrandGUID == "" && result.SessionID == "" && result.RunDir == "" {
		return nil
	}
	return map[string]any{
		"guid":      result.StrandGUID,
		"sessionId": result.SessionID,
		"runDir":    result.RunDir,
	}
}

// runCmd builds the `run` subcommand, validating flags and blocking on runner.Run
// until a terminal outcome is reached. All outcomes (done/asking/died/timeout) report
// success; only mechanism failures (flag errors, read errors, engine errors) report errors.
func (c *shuttleCLI) runCmd() *cobra.Command {
	var (
		prompt      string
		promptFile  string
		outputFiles []string
		model       string
		effort      string
		interactive bool
		role        string
		round       string
		parent      string
		anchor      string
		focus       bool
		shrink      bool
		timeout     time.Duration
		keepPane    bool
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "run one agent turn and block until it reaches a classified outcome",
		Long: `run starts one shuttle agent, blocks until it reaches a classified
outcome (done/asking/died/timeout), and prints that outcome as a single JSON
envelope. A run's output files ARE its return value: "done" means every
--output-file entry now exists, "asking" means the agent ended its turn with
a question instead. An --output-file entry may be absolute or relative — a
relative path resolves against the WORKTREE ROOT, not the shell's cwd — and
must not already exist when the run starts: a stale file would satisfy the
contract immediately, so the run is rejected instead.

Example (autonomous, two output files):
  lyx shuttle run --prompt "review this diff" --output-file review.md --output-file findings.json

Example (interactive, agent may ask clarifying questions):
  lyx shuttle run --prompt-file task.md --output-file result.md --interactive

--effort overrides the provider's reasoning-effort level; empty defers to
the provider default.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			// PersistentPreRunE has already emitted its own error envelope and recorded an exit
			// code when it aborted, and clihelp.Abort does not stop cobra from running this RunE.
			// Emitting a second envelope after it produced TWO JSON objects on one invocation,
			// which breaks every caller that unmarshals the output as one object — this package's
			// own smoke tests included — and reported the SECONDARY problem after the primary one
			// with nothing saying which to fix first.
			// This guard costs nothing the flag checks below were written for: when the pre-run
			// succeeded, ShouldAbort is false and a bad flag combination is still reported as its
			// own flag error rather than being swallowed by an already-recorded exit code.
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			// Validate flag shape before ever touching c.runner, so a bad flag combination is
			// reported as its own flag error.
			havePrompt := prompt != ""
			havePromptFile := promptFile != ""
			if havePrompt == havePromptFile {
				msg := "exactly one of --prompt or --prompt-file is required"
				if havePrompt {
					msg = "--prompt and --prompt-file are mutually exclusive"
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, msg))
				return nil
			}
			if len(outputFiles) == 0 {
				clihelp.SetExit(cmd.Context(), output.Err(out, "--output-file must be given at least once"))
				return nil
			}

			promptText := prompt
			if havePromptFile {
				data, err := os.ReadFile(promptFile)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("read --prompt-file: %v", err)))
					return nil
				}
				promptText = string(data)
			}

			spec := shuttleengine.Spec{
				Prompt:      promptText,
				OutputFiles: outputFiles,
				Model:       model,
				Effort:      effort,
				Interactive: interactive,
				Role:        role,
				Round:       round,
				Parent:      parent,
				Display: render.Display{
					Anchor:                   render.Anchor(anchor),
					Focus:                    focus,
					ShrinkWhenWaitingOnChild: shrink,
				},
				Timeout:  timeout,
				KeepPane: keepPane,
			}

			result, err := c.runner.Run(spec)
			if err != nil {
				// A mechanism failure after the strand registered still carries the run's
				// identity (see Run.Wait), and that is exactly when the operator needs it: no
				// cleanup ran, so the run dir is still on disk and the strand may still be
				// live. Emit those three whenever they exist rather than the bare message,
				// which left an operator with nothing to attach to or tear down.
				clihelp.SetExit(cmd.Context(), output.ErrFields(out, err.Error(), identityFields(result)))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"outcome":              string(result.Outcome),
				"sessionId":            result.SessionID,
				"guid":                 result.StrandGUID,
				"lastAssistantMessage": result.LastAssistantMessage,
				"runDir":               result.RunDir,
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&prompt, "prompt", "", "task prompt text (mutually exclusive with --prompt-file)")
	cmd.Flags().StringVar(&promptFile, "prompt-file", "", "path to a file whose contents become the task prompt")
	cmd.Flags().StringArrayVar(&outputFiles, "output-file", nil, "output file the agent must write (repeatable; required at least once; must not already exist; relative paths resolve against the worktree root)")
	cmd.Flags().StringVar(&model, "model", "", "provider model override; empty defers to the engine/provider default")
	cmd.Flags().StringVar(&effort, "effort", "", "reasoning-effort override; empty defers to the provider default")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "run interactively (the agent may ask questions); default is autonomous")
	cmd.Flags().StringVar(&role, "role", "", "role token used to fill the strand-name template")
	cmd.Flags().StringVar(&round, "round", "", "round token used to fill the strand-name template")
	cmd.Flags().StringVar(&parent, "parent", "", "parent strand's guid")
	cmd.Flags().StringVar(&anchor, "anchor", string(render.AnchorBelowParent), "placement: below-parent|hidden")
	cmd.Flags().BoolVar(&focus, "focus", true, "give this strand tmux input focus")
	cmd.Flags().BoolVar(&shrink, "shrink", true, "collapse this strand to a compact strip once a descendant is present")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "wall-clock deadline before an in-progress run is classified as timed out (0 = config default)")
	cmd.Flags().BoolVar(&keepPane, "keep-pane", false, `leave the strand and its pane alive after a "done" outcome`)

	return cmd
}
