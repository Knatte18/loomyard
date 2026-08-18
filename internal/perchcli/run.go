// run.go implements the `run` perch verb: the profile-YAML-and-flags-to-Run mapper that turns a
// "lyx perch run" invocation into a blocking perchengine.Engine.Run call, commits+pushes the
// resulting block artifacts through the fabric repo once at block exit, and prints the Result as a
// single JSON envelope.
// It also owns decodeProfile, the strict YAML decode that maps a profile file 1:1 onto
// perchengine.Profile, resolving its judge-model/model keys' model-spec strings against the
// invocation's shared registry.

package perchcli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// fileSetYAML mirrors burlerengine.FileSet's YAML shape.
type fileSetYAML struct {
	Paths        []string `yaml:"paths"`
	Instructions string   `yaml:"instructions"`
}

// gateYAML mirrors a profile file's "gate" key.
type gateYAML struct {
	Mode    string   `yaml:"mode"`
	Command []string `yaml:"command"`
	Timeout string   `yaml:"timeout"`
}

// profileYAML mirrors a profile file's top-level shape 1:1 onto perchengine.Profile's fields.
type profileYAML struct {
	Target     fileSetYAML `yaml:"target"`
	Fasit      fileSetYAML `yaml:"fasit"`
	Rubric     string      `yaml:"rubric"`
	FixScope   string      `yaml:"fix-scope"`
	ToolUse    bool        `yaml:"tool-use"`
	ClusterFan string      `yaml:"cluster-fan"`
	Gate       gateYAML    `yaml:"gate"`
	RoundCaps  []int       `yaml:"round-caps"`
	JudgeModel string      `yaml:"judge-model"`
	Model      string      `yaml:"model"`
	Timeout    string      `yaml:"timeout"`
}

// decodeProfile strictly decodes a profile file into a perchengine.Profile, resolving model-spec strings against reg.
func decodeProfile(data []byte, reg modelspec.Registry) (perchengine.Profile, error) {
	var parsed profileYAML

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&parsed); err != nil {
		return perchengine.Profile{}, fmt.Errorf("perch: profile YAML: %w", err)
	}

	var gateTimeout time.Duration
	if parsed.Gate.Timeout != "" {
		d, err := time.ParseDuration(parsed.Gate.Timeout)
		if err != nil {
			return perchengine.Profile{}, fmt.Errorf("perch: profile gate.timeout: %w", err)
		}
		gateTimeout = d
	}

	var timeout time.Duration
	if parsed.Timeout != "" {
		d, err := time.ParseDuration(parsed.Timeout)
		if err != nil {
			return perchengine.Profile{}, fmt.Errorf("perch: profile timeout: %w", err)
		}
		timeout = d
	}

	var judgeModel, judgeEffort string
	if parsed.JudgeModel != "" {
		model, effort, err := perchengine.ResolveModelSpec(parsed.JudgeModel, reg)
		if err != nil {
			return perchengine.Profile{}, fmt.Errorf("perch: profile judge-model: %w", err)
		}
		judgeModel, judgeEffort = model, effort
	}

	var runModel, runEffort string
	if parsed.Model != "" {
		model, effort, err := perchengine.ResolveModelSpec(parsed.Model, reg)
		if err != nil {
			return perchengine.Profile{}, fmt.Errorf("perch: profile model: %w", err)
		}
		runModel, runEffort = model, effort
	}

	return perchengine.Profile{
		Target: burlerengine.FileSet{
			Paths:        parsed.Target.Paths,
			Instructions: parsed.Target.Instructions,
		},
		Fasit: burlerengine.FileSet{
			Paths:        parsed.Fasit.Paths,
			Instructions: parsed.Fasit.Instructions,
		},
		Rubric:     parsed.Rubric,
		FixScope:   burlerengine.FixScope(parsed.FixScope),
		ToolUse:    parsed.ToolUse,
		ClusterFan: parsed.ClusterFan,
		Gate: perchengine.Gate{
			Mode:    perchengine.GateMode(parsed.Gate.Mode),
			Command: parsed.Gate.Command,
			Timeout: gateTimeout,
		},
		RoundCaps:   parsed.RoundCaps,
		JudgeModel:  judgeModel,
		JudgeEffort: judgeEffort,
		Model:       runModel,
		Effort:      runEffort,
		Timeout:     timeout,
	}, nil
}

// deriveBlockRunID returns the run identity: the explicit --run-id if supplied, otherwise the stable id derived from the profile path and content.
func deriveBlockRunID(profilePath string, fileProfile perchengine.Profile, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	hash, err := perchengine.ProfileHash(fileProfile)
	if err != nil {
		return "", err
	}
	return perchengine.DeriveRunID(profilePath, hash), nil
}

// resolveRunTarget maps a file-decoded profile and tuning flags onto the concrete block's identity,
// run dir, scratch dir, and overlaid profile.
// runDir and scratchDir are both derived from the SAME id, joined onto c.runDirBase and
// c.scratchDirBase respectively, so the two directories can never disagree about which block they
// belong to.
func (c *perchCLI) resolveRunTarget(profilePath, explicitRunID string, fileProfile perchengine.Profile, model, effort string, timeout time.Duration) (id, runDir, scratchDir string, profile perchengine.Profile, err error) {
	id, err = deriveBlockRunID(profilePath, fileProfile, explicitRunID)
	if err != nil {
		return "", "", "", perchengine.Profile{}, err
	}
	runDir = filepath.Join(c.runDirBase, id)
	scratchDir = filepath.Join(c.scratchDirBase, id)

	// Overlay the tuning flags AFTER deriving the id above: they are part of
	// what the block actually ran (and of its persisted identity hash) but must
	// not mint a new id. Each flag overrides only when supplied (non-empty /
	// non-zero), mirroring burlercli's flag semantics.
	profile = fileProfile
	if model != "" {
		profile.Model = model
	}
	if effort != "" {
		profile.Effort = effort
	}
	if timeout != 0 {
		profile.Timeout = timeout
	}
	return id, runDir, scratchDir, profile, nil
}

// runCmd builds the `run` subcommand: validates --profile, reads and decodes it, derives the block identity, overlays tuning flags, and runs the block through Engine.
func (c *perchCLI) runCmd() *cobra.Command {
	var (
		profilePath string
		runID       string
		model       string
		effort      string
		timeout     time.Duration
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "run a profile-driven gate loop over burler rounds from a profile YAML file",
		Long: `run reads a profile YAML file describing one perch block — what to review,
what to judge it against, the convergence gate, and the round-cap ladder —
drives burler rounds through the real shuttle substrate until the block is
APPROVED or STUCK (or PAUSED, if "lyx perch pause" was called against this
run), and prints its Result as a single JSON envelope. Re-running with the
same profile (and the same derived or supplied --run-id) resumes exactly
where the block left off.

Example profile YAML (llm-verdict gate — the default for text review):
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
  fix-scope: overlay
  tool-use: false
  cluster-fan: ""  # naming a fan from burler.yaml activates cluster review, one fork per fan entry
  gate:
    mode: llm-verdict
  round-caps: [5, 8, 10]
  judge-model: haiku            # judge-model: "sonnet[effort=medium]"  -- bracket overrides the registry default
  model: ""                     # e.g. "sonnet" or "sonnet[effort=high]" -- empty defers to judge-model/perch.yaml/built-in
  timeout: 0s

judge-model and model use the model-spec notation (contracts/specs/llm-model-spec.md):
a registry alias, optionally with an [effort=...] bracket, or the escape form
"<engine>:<model-id>[...]" for a model not (yet) in the registry. A bare alias
picks up its default effort from the operator-owned models.yaml; effort is the
only bracket param perch accepts.

Example command-gate variant (convergence decided by a real command, not
the burler verdict):
  # gate:
  #   mode: command
  #   command: ["go", "test", "./..."]
  #   timeout: 10m

Example invocation:
  lyx perch run --profile profile.yaml

--model/--effort/--timeout override the profile's model/effort/timeout for
every round of this block; empty/zero defers to the profile's own values.
The overrides are part of the block's identity: re-running an existing block
with different tuning flags is refused ("started with a different profile") —
pass a fresh --run-id to run the same profile under different tuning.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			// Validate flag shape before ever touching c's PersistentPreRunE-
			// populated state (unpopulated when config resolution aborted),
			// so a missing --profile is reported as its own flag error
			// rather than being swallowed by, or racing with, the
			// PersistentPreRunE abort's already-recorded exit code.
			if profilePath == "" {
				clihelp.SetExit(cmd.Context(), output.Err(out, "perch: --profile is required"))
				return nil
			}
			// An explicit --run-id is joined directly into a directory path
			// under the perch runs area; reject anything that is not the
			// same safe shape a derived id has, before it can escape that
			// directory (e.g. "../elsewhere") or trip a confusing MkdirAll
			// failure deep inside the run.
			if runID != "" && !perchengine.ValidRunID(runID) {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("perch: --run-id %q must be lowercase alphanumerics and dashes only (no path separators)", runID)))
				return nil
			}

			// A failing PersistentPreRunE has already written an error
			// response and recorded the exit code; short-circuit rather
			// than touch c's fields, which are unpopulated on that path.
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			data, err := os.ReadFile(profilePath)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("perch: read --profile: %v", err)))
				return nil
			}

			fileProfile, err := decodeProfile(data, c.modelReg)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// Map the file-decoded profile and the tuning flags onto this
			// invocation's concrete block. resolveRunTarget derives the run
			// identity from the FILE content BEFORE overlaying the tuning
			// flags — a load-bearing ordering it keeps in one tested place
			// (see its doc) rather than inlined here, where a later reorder
			// could silently fold the flags into the derived id.
			id, runDir, scratchDir, profile, err := c.resolveRunTarget(profilePath, runID, fileProfile, model, effort, timeout)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// The engine is constructed per-invocation, never in
			// PersistentPreRunE: its pause seam closes over this call's
			// concrete scratchDir, which is only known once --profile/--run-id
			// have been resolved above.
			engine := perchengine.New(c.burlerEngine, c.runner, c.perchCfg, c.perchGeom, perchengine.Options{
				PauseRequested: func() bool {
					_, err := os.Stat(perchengine.PauseFlagPath(scratchDir))
					return err == nil
				},
			})

			result, runErr := engine.Run(profile, runDir, scratchDir, fabricengine.StencilsDir(c.layout.HubPath))

			// A busy fail-fast means ANOTHER invocation owns this block and
			// is mid-round right now; this invocation changed nothing on
			// disk. Running the fabric sync below would commit and push the
			// winner's in-flight partial state under a misleading
			// "perch: <id> ERROR" message — skip it; the winner runs the
			// sync at its own block exit.
			if errors.Is(runErr, perchengine.ErrBlockBusy) {
				clihelp.SetExit(cmd.Context(), output.Err(out, runErr.Error()))
				return nil
			}

			// The fabric sync runs once at block exit regardless of outcome —
			// including a hard engine error — per the Fabric Git Invariant:
			// perchcli is the loop owner, perchengine itself is fabric-blind.
			// A hard error can still follow a completed round or two (e.g. a
			// could-not-start gate error, or a second-consecutive non-done
			// attempt) whose artifacts are already on disk; skipping the
			// sync on that path would strand them uncommitted indefinitely
			// if this is the last invocation to touch the worktree before
			// the next resume (which may be a long time, or never).
			outcomeLabel := "ERROR"
			if runErr == nil {
				outcomeLabel = string(result.Outcome)
			}
			opts := fabricengine.EnvSyncOptions()
			// Lock files (run.lock, state.json.lock) and the pause flag now
			// live in the block's .lyx scratch dir, not inside _lyx — see
			// perchengine.ScratchDir and treadleengine.Options.ScratchDir.
			// No exclusion layer is involved at all: the pathspec below
			// names _lyx only, so those never-tracked artifacts are simply
			// outside the tree fabric ever looks at.
			files := fabricengine.ScopedPathspec(c.layout.AnchorRel, []string{lyxdirs.LyxDirName})
			// SkipGit is checked here, before fabricengine.Open's stat-based
			// path validation, mirroring Commit's own top-level
			// short-circuit: the CI/test bypass must never require a real
			// fabric repo to exist on disk, but Open (unlike Commit
			// itself) validates both paths unconditionally.
			var committed bool
			var syncErr error
			if !opts.SkipGit {
				var fab *fabricengine.Fabric
				fab, syncErr = fabricengine.Open(c.layout)
				if syncErr == nil {
					var res fabricengine.CommitResult
					res, syncErr = fab.Commit(
						files,
						fmt.Sprintf("perch: %s %s", id, outcomeLabel),
						nil,
						opts,
					)
					committed = res.Committed()
				}
			}

			if runErr != nil {
				msg := runErr.Error()
				if syncErr != nil {
					msg = fmt.Sprintf("%s (additionally, the fabric sync failed: %v)", msg, syncErr)
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, msg))
				return nil
			}
			if syncErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf(
					"perch: block %s finished (%s) but the fabric sync failed: %v", id, result.Outcome, syncErr,
				)))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"outcome":         string(result.Outcome),
				"stuckReason":     string(result.StuckReason),
				"roundsRun":       result.RoundsRun,
				"runId":           id,
				"runDir":          runDir,
				"scratchDir":      scratchDir,
				"fabricCommitted": committed,
			}))
			return nil
		},
	}

	cmd.Flags().StringVar(&profilePath, "profile", "", "path to the profile YAML file describing this block (required)")
	cmd.Flags().StringVar(&runID, "run-id", "", "run identity override; empty derives a stable id from the profile path and content")
	cmd.Flags().StringVar(&model, "model", "", "provider model override for every round; empty defers to the profile's model")
	cmd.Flags().StringVar(&effort, "effort", "", "reasoning-effort override for every round; empty defers to the profile's effort")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "per-round wall-clock deadline override; 0 defers to the profile's timeout")

	return cmd
}
