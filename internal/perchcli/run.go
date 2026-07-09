// run.go implements the `run` perch verb: the profile-YAML-and-flags-to-Run
// mapper that turns a "lyx perch run" invocation into a blocking
// perchengine.Engine.Run call, commits+pushes the resulting block artifacts
// through weft once at block exit, and prints the Result as a single JSON
// envelope. It also owns decodeProfile, the strict YAML decode that maps a
// profile file 1:1 onto perchengine.Profile.

package perchcli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/Knatte18/loomyard/internal/weftengine"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// fileSetYAML mirrors burlerengine.FileSet's YAML shape (the target/fasit
// key of a profile file): a list of paths and/or free-form instructions.
// It is identical to burlercli's own fileSetYAML, kept as a separate type
// here (rather than a shared export) so perchcli and burlercli stay
// decoupled — the same rationale that keeps their Profile seams package-local.
type fileSetYAML struct {
	Paths        []string `yaml:"paths"`
	Instructions string   `yaml:"instructions"`
}

// gateYAML mirrors a profile file's "gate" key: which signal(s) decide
// convergence (Mode), the argv a command-mode gate runs (Command), and how
// long that command may run (Timeout, a Go duration string such as "10m";
// empty defers to perchengine's built-in default).
type gateYAML struct {
	Mode    string   `yaml:"mode"`
	Command []string `yaml:"command"`
	Timeout string   `yaml:"timeout"`
}

// profileYAML mirrors a profile file's top-level shape 1:1 onto
// perchengine.Profile's fields: the embedded burler content keys (Target,
// Fasit, Rubric, FixScope, ToolUse, ClusterN — burler's own kebab-case
// vocabulary) plus the perch-owned loop keys (Gate, RoundCaps, JudgeModel,
// JudgeEffort, Model, Effort, Timeout). It exists as a separate type (rather
// than decoding straight into Profile) so the YAML key vocabulary stays
// decoupled from Profile's Go field names, exactly like burlercli's
// profileYAML.
type profileYAML struct {
	Target      fileSetYAML `yaml:"target"`
	Fasit       fileSetYAML `yaml:"fasit"`
	Rubric      string      `yaml:"rubric"`
	FixScope    string      `yaml:"fix-scope"`
	ToolUse     bool        `yaml:"tool-use"`
	ClusterN    int         `yaml:"cluster-n"`
	Gate        gateYAML    `yaml:"gate"`
	RoundCaps   []int       `yaml:"round-caps"`
	JudgeModel  string      `yaml:"judge-model"`
	JudgeEffort string      `yaml:"judge-effort"`
	Model       string      `yaml:"model"`
	Effort      string      `yaml:"effort"`
	Timeout     string      `yaml:"timeout"`
}

// decodeProfile strictly decodes a profile file's raw bytes into a
// perchengine.Profile. Decoding uses yaml.v3's Decoder.KnownFields(true) per
// the yaml-strictness-split decision: an operator typo in a profile key
// (e.g. "fixscope:" for "fix-scope:") must fail loudly here rather than
// silently zeroing a safety-critical field. The two Go-duration-string
// fields (gate.timeout, timeout) are parsed via time.ParseDuration, also
// fail-loud on a malformed value. decodeProfile performs no further content
// validation itself (RoundCaps shape, Gate.Mode legality, and so on) — that
// stays perchengine.Profile's job via its own validate step inside
// Engine.Run, so this function's only responsibility is the YAML-to-struct
// mapping.
func decodeProfile(data []byte) (perchengine.Profile, error) {
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

	return perchengine.Profile{
		Target: burlerengine.FileSet{
			Paths:        parsed.Target.Paths,
			Instructions: parsed.Target.Instructions,
		},
		Fasit: burlerengine.FileSet{
			Paths:        parsed.Fasit.Paths,
			Instructions: parsed.Fasit.Instructions,
		},
		Rubric:   parsed.Rubric,
		FixScope: burlerengine.FixScope(parsed.FixScope),
		ToolUse:  parsed.ToolUse,
		ClusterN: parsed.ClusterN,
		Gate: perchengine.Gate{
			Mode:    perchengine.GateMode(parsed.Gate.Mode),
			Command: parsed.Gate.Command,
			Timeout: gateTimeout,
		},
		RoundCaps:   parsed.RoundCaps,
		JudgeModel:  parsed.JudgeModel,
		JudgeEffort: parsed.JudgeEffort,
		Model:       parsed.Model,
		Effort:      parsed.Effort,
		Timeout:     timeout,
	}, nil
}

// deriveBlockRunID returns the run identity for one `perch run` invocation:
// the explicit --run-id when supplied, otherwise the stable id derived from
// the profile path and fileProfile — the profile exactly as decoded from the
// file, BEFORE any --model/--effort/--timeout overlay. Deriving from the
// pre-overlay profile is load-bearing: it keeps the id stable across
// invocations of the same file, so re-running with different tuning flags
// resolves to the SAME run dir and is refused loud by the engine's identity
// check (which covers the overlaid values) instead of silently forking a
// fresh block in a new dir.
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

// resolveRunTarget maps a file-decoded profile and the run-tuning flags onto
// the concrete block one `perch run` invocation drives: the stable run
// identity id, its run dir under c.runDirBase, and the profile with the
// tuning flags overlaid. The load-bearing ordering lives HERE, in one tested
// function, rather than inlined in runCmd's RunE: the id is derived from
// fileProfile — the profile exactly as the file decoded it, BEFORE the
// --model/--effort/--timeout overlay — so the id is stable across tuning-flag
// changes and a re-run with different flags resolves to the SAME run dir,
// where the engine's identity check (which covers the overlaid values) refuses
// it loud instead of silently forking a fresh block. Overlaying before
// deriving would fold the flags into the id and defeat that; keeping the whole
// sequence in one function lets a test pin the ordering so a later reorder
// cannot slip through unnoticed (which an isolated deriveBlockRunID test
// cannot catch, since it never exercises RunE's call ordering).
func (c *perchCLI) resolveRunTarget(profilePath, explicitRunID string, fileProfile perchengine.Profile, model, effort string, timeout time.Duration) (id, runDir string, profile perchengine.Profile, err error) {
	id, err = deriveBlockRunID(profilePath, fileProfile, explicitRunID)
	if err != nil {
		return "", "", perchengine.Profile{}, err
	}
	runDir = filepath.Join(c.runDirBase, id)

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
	return id, runDir, profile, nil
}

// runCmd builds the `run` subcommand: validates that --profile was supplied
// before ever touching c's PersistentPreRunE-populated state (matching
// burlercli's run.go flag-shape pattern, so the flag error surfaces in its
// own JSON line rather than racing a failing PersistentPreRunE's already-
// recorded exit code), reads and strictly decodes the --profile file,
// derives the block's run identity from the decoded file content (before
// any flag overlay — see the identity comment in the body), overlays the
// three run-tuning flags, constructs a fresh *perchengine.Engine per
// invocation (its pause seam closes over the concrete runDir this call
// resolves), and blocks on Engine.Run until the block reaches a terminal
// outcome OR a hard engine error. Either way, the run dir's artifacts are
// committed and pushed through weft exactly once, per the Weft Git
// Invariant, before the JSON envelope is printed — an engine error can
// still follow a completed round or two whose artifacts are already on
// disk, and those must not be stranded uncommitted.
//
// --profile is validated manually here rather than via cobra's
// MarkFlagRequired, for the same reason burlercli's run verb does: cobra's
// own flag-required error bypasses SetExit/ShouldAbort and is wrapped by
// clihelp.RunRoot's generic cobra-error path instead.
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
  cluster-n: 0
  gate:
    mode: llm-verdict
  round-caps: [5, 8, 10]
  judge-model: haiku
  judge-effort: ""
  model: ""
  effort: ""
  timeout: 0s

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

			fileProfile, err := decodeProfile(data)
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
			id, runDir, profile, err := c.resolveRunTarget(profilePath, runID, fileProfile, model, effort, timeout)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			// The engine is constructed per-invocation, never in
			// PersistentPreRunE: its pause seam closes over this call's
			// concrete runDir, which is only known once --profile/--run-id
			// have been resolved above.
			engine := perchengine.New(c.burlerEngine, c.runner, c.perchCfg, c.layout, perchengine.Options{
				PauseRequested: func() bool {
					_, err := os.Stat(perchengine.PauseFlagPath(runDir))
					return err == nil
				},
			})

			result, runErr := engine.Run(profile, runDir)

			// The weft sync runs once at block exit regardless of outcome —
			// including a hard engine error — per the Weft Git Invariant:
			// perchcli is the loop owner, perchengine itself is weft-blind.
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
			weftWorktree := c.layout.WeftWorktree()
			opts := weftengine.EnvSyncOptions()
			committed, weftErr := weftengine.Commit(
				weftWorktree,
				weftengine.ScopedPathspec(c.layout.RelPath, []string{hubgeometry.LyxDirName}),
				fmt.Sprintf("perch: %s %s", id, outcomeLabel),
				opts,
			)
			if weftErr == nil {
				weftErr = weftengine.Push(weftWorktree, opts)
			}

			if runErr != nil {
				msg := runErr.Error()
				if weftErr != nil {
					msg = fmt.Sprintf("%s (additionally, the weft sync failed: %v)", msg, weftErr)
				}
				clihelp.SetExit(cmd.Context(), output.Err(out, msg))
				return nil
			}
			if weftErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf(
					"perch: block %s finished (%s) but the weft sync failed: %v", id, result.Outcome, weftErr,
				)))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"outcome":       string(result.Outcome),
				"stuckReason":   string(result.StuckReason),
				"roundsRun":     result.RoundsRun,
				"runId":         id,
				"runDir":        runDir,
				"weftCommitted": committed,
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
