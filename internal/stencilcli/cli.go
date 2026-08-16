// cli.go exposes the cobra command tree for the stencil module: list, validate, diff, sync, and
// promote.
// Geometry is resolved once in the parent's PersistentPreRunE, following the pattern
// internal/idecli/cli.go establishes, so a bare "lyx stencil" listing never requires a git repo.
// list, validate, and sync are declared here; diff.go and promote.go each add their own subcommand
// via a closure-returning accessor over the same resolved *lyxcwd.Location.
//
// stencilcli is a named deviation from the CLI/Cobra Invariant's package-naming rule: its kernel is
// internal/stencilstore, not stencilengine. See CONSTRAINTS.md.

package stencilcli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// Command returns the cobra command tree for the stencil module.
func Command() *cobra.Command {
	var l *lyxcwd.Location

	cmd := &cobra.Command{
		Use:   "stencil",
		Short: "Inspect and manage board-copy stencil prompts",
		Long: `stencil inspects and manages the board's stencil prompts -- the source-of-truth
prompt files every producer reads from disk at call time.

Examples:
  lyx stencil list
  lyx stencil validate
  lyx stencil diff loom-template-plan
  lyx stencil diff --all --exit-code
  lyx stencil sync
  lyx stencil promote loom-template-plan`,
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "stencil" {
				return nil
			}

			ctx := cmd.Context()

			cwd, err := lyxcwd.CwdFrom(ctx)
			if err != nil {
				output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to get working directory: %v", err))
				clihelp.Abort(ctx, 1)
				return nil
			}

			resolved, err := lyxcwd.Resolve(cwd)
			if err != nil {
				output.Err(cmd.OutOrStdout(), err.Error())
				clihelp.Abort(ctx, 1)
				return nil
			}

			l = resolved
			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List every registered stencil, its board-copy path, and its edit state",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			out := cmd.OutOrStdout()
			stencilsDir := fabricengine.StencilsDir(l.HubPath)
			registry := stencils.Registry()

			type stencilInfo struct {
				Name  string `json:"name"`
				Path  string `json:"path"`
				State string `json:"state"`
			}

			var list []stencilInfo
			for _, name := range registry.Names() {
				shipped, known := registry.Default(name)
				if !known {
					continue
				}

				path := stencilstore.Path(stencilsDir, name)
				onDisk, readErr := os.ReadFile(path)
				exists := readErr == nil

				list = append(list, stencilInfo{
					Name:  name,
					Path:  path,
					State: classifyLabel(stencilstore.Classify(onDisk, exists, shipped)),
				})
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"stencils": list}))
			return nil
		},
	}

	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Report marker mismatches between each board copy and its shipped default",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			out := cmd.OutOrStdout()
			stencilsDir := fabricengine.StencilsDir(l.HubPath)

			findings, err := stencilstore.Validate(stencilsDir, stencils.Registry())
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			hasError := false
			findingMaps := make([]map[string]any, 0, len(findings))
			for _, f := range findings {
				if f.Severity == stencilstore.SeverityError {
					hasError = true
				}
				findingMaps = append(findingMaps, map[string]any{
					"name":     f.Name,
					"marker":   f.Marker,
					"severity": f.Severity,
				})
			}

			code := output.Ok(out, map[string]any{"findings": findingMaps})
			if hasError {
				// A marker present on disk but absent from the shipped default breaks
				// stencil.Fill at the point of use, so the response is still the ok
				// envelope carrying the findings -- but the exit code must fail the
				// invocation, which output.Ok's own return value cannot do since it is
				// hardcoded to 0.
				code = 1
			}
			clihelp.SetExit(cmd.Context(), code)
			return nil
		},
	}

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Force-refresh every stencil against the shipped registry, even from a -dev build",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			out := cmd.OutOrStdout()
			stencilsDir := fabricengine.StencilsDir(l.HubPath)
			sourceDir := resolveSourceDir(l)

			// ForceRefresh's returned written slice is a plain list of stencil names, not a
			// fabricengine.Mutations record -- no rec exists yet at this point (it is constructed
			// below, scoped to the CommitSeededStencils call only), and stencilstore is not a
			// fabricengine verb, so there is no Kind to classify these writes under. Treated as a
			// pre-flight-style failure: a bare output.Err deliberately, not errWithRecord's
			// mutations/partial pair. Any partially-written state is re-detected and completed by
			// the next sync via Classify, so this is a reporting gap, not a correctness bug.
			written, err := stencilstore.ForceRefresh(stencilsDir, stencils.Registry(), sourceDir)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			rec := fabricengine.NewMutations(filepath.Dir(l.HubPath))
			res, commitErr := fabricengine.CommitSeededStencils(l.HubPath, written, "lyx: seed stencils", rec)
			if commitErr != nil {
				clihelp.SetExit(cmd.Context(), errWithRecord(out, rec.Snapshot(), commitErr))
				return nil
			}

			clihelp.SetExit(cmd.Context(), okWithRecord(out, rec.Snapshot(), map[string]any{
				"committed": res.Committed,
				"sha":       res.SHA,
			}))
			return nil
		},
	}

	loc := func() *lyxcwd.Location { return l }

	cmd.AddCommand(listCmd, validateCmd, syncCmd, newDiffCmd(loc), newPromoteCmd(loc))
	return cmd
}

// RunCLI is the public seam for the stencil module CLI.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn is RunCLI's seam-cwd-carrying sibling: an empty cwd means "read the process cwd" and
// delegates to clihelp.Execute exactly as RunCLI always has, while any other value seeds cwd into
// the execution context via clihelp.ExecuteIn.
// The branch exists because lyxcwd.WithCwd panics on an empty directory, so a uniform delegation to
// ExecuteIn would panic on every existing RunCLI call.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}

// classifyLabel maps a stencilstore.State to the three-value vocabulary lyx stencil list reports.
// StateReconciled is folded into "untouched" alongside StateUntouched, since both describe a board
// copy that already matches what a future refresh would write -- StateReconciled differs only in
// carrying a stamp for the wrong (superseded) default, which reconciliation corrects invisibly on
// the next seed/refresh pass.
func classifyLabel(state stencilstore.State) string {
	switch state {
	case stencilstore.StateAbsent:
		return "absent"
	case stencilstore.StateEdited:
		return "edited"
	default:
		return "untouched"
	}
}

// resolveSourceDir computes the worktree-relative contracts/stencils/ source tree path the same way
// cmd/lyx/stencilseed.go does: filepath.Join(l.WorktreePath(), "contracts", "stencils") when it exists,
// and the empty string otherwise. The empty string is what keeps the port-back drift warning
// silent in a consumer repo, whose worktree carries no such tree, rather than firing on every run
// forever.
func resolveSourceDir(l *lyxcwd.Location) string {
	sourceDir := filepath.Join(l.WorktreePath(), "contracts", "stencils")
	if _, err := os.Stat(sourceDir); err != nil {
		return ""
	}
	return sourceDir
}

// okWithRecord and errWithRecord mirror internal/fabriccli's envelope helpers of the same name: they
// lay down the fixed mutations/partial key pair the Mutation Record Invariant requires of every
// mutating verb outcome. sync is stencilcli's only mutating verb, so these live inline here rather
// than in a dedicated envelope.go.
func okWithRecord(w io.Writer, rec fabricengine.Mutations, fields map[string]any) int {
	fields["mutations"] = rec.Entries()
	fields["partial"] = false
	return output.Ok(w, fields)
}

// errWithRecord mirrors internal/fabriccli's helper of the same name; see okWithRecord's comment.
func errWithRecord(w io.Writer, rec fabricengine.Mutations, err error) int {
	fields := map[string]any{
		"mutations": rec.Entries(),
		"partial":   rec.Len() > 0,
	}
	return output.ErrFields(w, err.Error(), fields)
}
