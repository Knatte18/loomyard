// run.go implements the `run` burler verb: the profile-YAML-and-flags-to-halves mapper that turns a "lyx burler run" invocation into a blocking burlerengine.Engine.Review call.
// A review that reached done is followed by a burlerengine.Engine.Fix call, and both results print as a single JSON envelope.
// It also owns decodeProfile, the strict YAML decode that maps a profile file 1:1 onto
// burlerengine.Profile.

package burlercli

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/cliwire"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// fileSetYAML mirrors burlerengine.FileSet's YAML shape.
type fileSetYAML struct {
	Paths        []string `yaml:"paths"`
	Instructions string   `yaml:"instructions"`
}

// profileYAML mirrors a profile file's top-level shape, using kebab-case keys.
type profileYAML struct {
	Target            fileSetYAML `yaml:"target"`
	Fasit             fileSetYAML `yaml:"fasit"`
	Rubric            string      `yaml:"rubric"`
	FixScope          string      `yaml:"fix-scope"`
	ToolUse           bool        `yaml:"tool-use"`
	ClusterFan        string      `yaml:"cluster-fan"`
	ReviewPath        string      `yaml:"review-path"`
	FixerReportPath   string      `yaml:"fixer-report-path"`
	PriorReviews      []string    `yaml:"prior-reviews"`
	PriorFixerReports []string    `yaml:"prior-fixer-reports"`
}

// decodeProfile strictly decodes a profile file into burlerengine.Profile
// using Decoder.KnownFields(true). Content validation is Profile.validate's job.
func decodeProfile(data []byte) (burlerengine.Profile, error) {
	var parsed profileYAML

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&parsed); err != nil {
		return burlerengine.Profile{}, fmt.Errorf("burler: profile YAML: %w", err)
	}

	return burlerengine.Profile{
		Target: burlerengine.FileSet{
			Paths:        parsed.Target.Paths,
			Instructions: parsed.Target.Instructions,
		},
		Fasit: burlerengine.FileSet{
			Paths:        parsed.Fasit.Paths,
			Instructions: parsed.Fasit.Instructions,
		},
		Rubric:            parsed.Rubric,
		FixScope:          burlerengine.FixScope(parsed.FixScope),
		ToolUse:           parsed.ToolUse,
		ClusterFan:        parsed.ClusterFan,
		ReviewPath:        parsed.ReviewPath,
		FixerReportPath:   parsed.FixerReportPath,
		PriorReviews:      parsed.PriorReviews,
		PriorFixerReports: parsed.PriorFixerReports,
	}, nil
}

// runCmd builds the `run` subcommand. It validates --profile manually (rather
// than via MarkFlagRequired) to route the flag error through SetExit.
func (c *burlerCLI) runCmd() *cobra.Command {
	var (
		profilePath  string
		reviewModel  string
		reviewEffort string
		fixModel     string
		fixEffort    string
		round        string
		timeout      time.Duration
	)

	cmd := &cobra.Command{
		Use:         "run",
		Short:       "run one review+fix round from a profile YAML file",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `run reads a profile YAML file describing one review+fix round — what to
review, what to judge it against, and how the round is allowed to write its
fixes — drives the round through the real shuttle substrate, and prints its
result as a single JSON envelope.

The round runs two sessions one after the other. A reviewer (role
burler-review) writes the review file. Once lyx has accepted the review,
a fixer (role burler-fix) fixes what the reviewer found. A review with no
findings spawns no fixer: lyx writes the fixer report itself, and the envelope
reports fixSkipped true. A reviewer that did not finish runs no fixer. The
envelope reports each half's identity under review and fix.

Example profile YAML:
  target:
    paths: ["docs/overview.md"]
    instructions: ""
  fasit:
    paths: ["_mill/discussion.md"]
    instructions: ""
  rubric: |
    BLOCKING: the doc contradicts the discussion's pinned decisions.
    MEDIUM: a decision is described but its rationale is missing.
    LOW: wording is unclear but not misleading.
    NIT: minor formatting.
  fix-scope: source
  tool-use: false
  cluster-fan: ""  # naming a fan from burler.yaml activates cluster review, one fork per fan entry
  review-path: _lyx/burler/review.md
  fixer-report-path: _lyx/burler/fixer-report.md
  prior-reviews: []
  prior-fixer-reports: []

Example invocation:
  lyx burler run --profile profile.yaml

--review-model/--review-effort and --fix-model/--fix-effort override each
half's provider model and reasoning effort; empty defers to the provider
default. --timeout overrides the shuttle config's run-timeout; zero defers to
the config default.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			// ShouldAbort first, as PATTERN-cli-cobra requires of every RunE and
			// as every sibling verb already does. A failing PersistentPreRunE has already written its
			// error envelope and recorded the exit code, so the flag check that used to run ahead of
			// this emitted a SECOND envelope — and the second one ("--profile is required") was the
			// misleading one, naming a flag while the real failure was the wiring refusal above it
			// (crucible round opus-medium-r6, R6-16). A missing --profile on an aborted pre-run is
			// not information the operator needs; the refusal is.
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			// Validate flag shape before ever touching c.engine.
			if profilePath == "" {
				clihelp.SetExit(cmd.Context(), output.Err(out, "burler: --profile is required"))
				return nil
			}

			// Resolved against the SEAM cwd, the same base every other relative flag in this
			// invocation uses (wire resolves --target-dir and --stencils-dir through it). A bare
			// os.ReadFile resolved against the PROCESS cwd instead, so under an in-process driver one
			// relative flag named a different directory than the rest (crucible round opus-medium-r6,
			// R6-17).
			data, err := os.ReadFile(cliwire.ResolveToldDir(c.cwd, profilePath))
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("burler: read --profile: %v", err)))
				return nil
			}

			profile, err := decodeProfile(data)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			opts := burlerengine.RunOpts{
				Review:  burlerengine.ModelChoice{Model: reviewModel, Effort: reviewEffort},
				Fix:     burlerengine.ModelChoice{Model: fixModel, Effort: fixEffort},
				Timeout: timeout,
				Round:   round,
			}

			// Standalone mode boots its own reed session here, idempotently, because nothing else
			// can: `lyx reed up` is hub-only, and pre-fix the round's spawn died on "no reed
			// session" with an impossible recourse (crucible round fable5-high-r3, F-A1). Nil in
			// hub mode, where the session is the operator's or loom's own to manage.
			if c.reedUp != nil {
				if err := c.reedUp(cmd.Context(), true); err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("burler: bring up the standalone reed session: %v", err)))
					return nil
				}
			}

			review, err := c.engine.Review(profile, opts)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// A reviewer that did not finish leaves no accepted review for a fixer to act on.
			var fix burlerengine.FixResult
			if review.Outcome == shuttleengine.OutcomeDone {
				fix, err = c.engine.Fix(profile, opts, review.Digest)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, resultEnvelope(review, fix, c.mode, c.stateDir, c.stencilsDir)))
			return nil
		},
	}

	cmd.Flags().StringVar(&profilePath, "profile", "", "path to the profile YAML file describing this round (required)")
	cmd.Flags().StringVar(&reviewModel, "review-model", "", "the reviewer's provider model override; empty defers to the engine/provider default")
	cmd.Flags().StringVar(&reviewEffort, "review-effort", "", "the reviewer's reasoning-effort override; empty defers to the provider default")
	cmd.Flags().StringVar(&fixModel, "fix-model", "", "the fixer's provider model override; empty defers to the engine/provider default")
	cmd.Flags().StringVar(&fixEffort, "fix-effort", "", "the fixer's reasoning-effort override; empty defers to the provider default")
	cmd.Flags().StringVar(&round, "round", "", "round token used to fill the strand-name template")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "wall-clock deadline before an in-progress run is classified as timed out (0 = config default)")

	return cmd
}

// resultEnvelope maps the two halves' results onto the JSON envelope run's RunE prints via
// output.Ok. It is a separate function (rather than an inline map literal) so its shape
// is directly unit-testable without needing a live engine call.
// fix is the zero FixResult when the fixer was not reached because the review did not finish.
// The outcome is the fix half's when it ran and the review's otherwise.
// The verdict, fork count and cluster warnings come from the review, and fixSkipped reports a fix that spawned no fixer.
// review.ForkAudit is nil for a non-cluster review (or one that never reached done), so forkCount guards that nil rather than dereferencing Forks on it.
//
// mode, stateDir, and stencilsDir are CLI-level facts about how the stack was wired, taken as
// parameters rather than read off a receiver so this function stays directly unit-testable without a
// live engine call. They are emitted in BOTH modes deliberately: a mode field that existed only
// in standalone could not be used to tell the two modes apart, which is its whole purpose, and
// stencilsDir is equally worth reporting in a hub run pointed at an experimental stencil set via the
// flag. stateDir is the empty string in hub mode, where no derived state directory exists.
func resultEnvelope(review burlerengine.ReviewResult, fix burlerengine.FixResult, mode, stateDir, stencilsDir string) map[string]any {
	forkCount := 0
	if review.ForkAudit != nil {
		forkCount = len(review.ForkAudit.Forks)
	}
	outcome := review.Outcome
	if fix.Outcome != "" {
		outcome = fix.Outcome
	}

	return map[string]any{
		"outcome":         string(outcome),
		"verdict":         string(review.Verdict),
		"reviewPath":      review.ReviewPath,
		"fixerReportPath": fix.FixerReportPath,
		"review":          halfEnvelope(review.Review),
		"fix":             halfEnvelope(fix.Fix),
		"fixSkipped":      fix.Skipped,
		"clusterWarnings": review.ClusterWarnings,
		"forkCount":       forkCount,
		"mode":            mode,
		"stateDir":        stateDir,
		"stencilsDir":     stencilsDir,
	}
}

// halfEnvelope maps one half of a round onto its JSON object.
// startError is empty for a half that started.
func halfEnvelope(half burlerengine.Half) map[string]any {
	return map[string]any{
		"sessionId":            half.SessionID,
		"strandGuid":           half.StrandGUID,
		"lastAssistantMessage": half.LastAssistantMessage,
		"runDir":               half.RunDir,
		"startError":           half.StartError,
	}
}
