// cli.go exposes the cobra command tree for the stencil module: list, validate, diff, sync, and promote.
// Geometry is resolved once in the parent's PersistentPreRunE;
// so a bare "lym stencil" listing never requires a git repo.
// list, validate, and sync are declared here; diff.go and promote.go each add their own subcommand
// through a closure-returning accessor over the same resolved *lymcwd.Location.

package stencilcli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/contracts/specs"
	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/buildinfo"
	"github.com/Knatte18/loomyard/internal/buildvcs"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// Command returns the cobra command tree for the stencil module.
func Command() *cobra.Command {
	var l *lymcwd.Location

	cmd := &cobra.Command{
		Use:   "stencil",
		Short: "inspect and manage board-copy stencil prompts",
		Long: `stencil inspects and manages the board's stencil prompts -- the source-of-truth
prompt files every producer reads from disk at call time.

Examples:
  lym stencil list
  lym stencil validate
  lym stencil diff loom-template-plan
  lym stencil diff --all --exit-code
  lym stencil sync
  lym stencil promote loom-template-plan`,
		RunE: clihelp.GroupRunE,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "stencil" {
				return nil
			}

			ctx := cmd.Context()

			cwd, err := lymcwd.CwdFrom(ctx)
			if err != nil {
				output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to get working directory: %v", err))
				clihelp.Abort(ctx, 1)
				return nil
			}

			resolved, err := lymcwd.Resolve(cwd)
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
		Use:         "list",
		Short:       "list every registered stencil and deployed spec, its board-copy path, and its edit state",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			out := cmd.OutOrStdout()

			list := listRegistryEntries(fabricengine.StencilsDir(l.HubPath), stencils.Registry(), kindStencil)
			// list is the only verb that surfaces a deployed spec's edit state;
			// the reconcile policy depends on an operator seeing it.
			list = append(list, listRegistryEntries(fabricengine.SpecsDir(l.HubPath), specs.Registry(), kindSpec)...)

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"stencils": list}))
			return nil
		},
	}

	validateCmd := &cobra.Command{
		Use:         "validate",
		Short:       "report marker mismatches between each board copy and its shipped default",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
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
				// A marker present on disk but absent from the shipped default breaks stencil.Fill at the point of use.
				// The response stays the ok envelope carrying the findings,
				// but the exit code must fail the invocation, because output.Ok's return value is hardcoded to 0.
				code = 1
			}
			clihelp.SetExit(cmd.Context(), code)
			return nil
		},
	}

	syncCmd := &cobra.Command{
		Use:         "sync",
		Short:       "force-refresh every stencil and deployed spec against the shipped registry, even from a -dev or unstamped build",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			out := cmd.OutOrStdout()
			stencilsDir := fabricengine.StencilsDir(l.HubPath)
			sourceDir := resolveSourceDir(l)

			// The written slice from ForceRefresh is a plain list of stencil names, not a fabricengine.Mutations record.
			// No record exists yet at this point, and stencilstore is not a fabricengine verb,
			// so there is no Kind to classify these writes under.
			// The failure is a pre-flight failure: it returns a bare output.Err, not errWithRecord's mutations and partial pair.
			// The next sync re-detects and completes any partially written state through Classify,
			// so this is a reporting gap, not a correctness bug.
			running := buildvcs.Running()
			binaryLabel := fabricengine.BinaryLabel()
			mode := stencilstore.ModeFor(buildinfo.IsDev(), buildinfo.IsProduction(), running.Clean())
			written, err := stencilstore.ForceRefresh(stencilsDir, stencils.Registry(), mode, fabricengine.StencilSource(l.WorktreePath(), sourceDir, running))
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			rec := fabricengine.NewMutations(filepath.Dir(l.HubPath))
			res, commitErr := fabricengine.CommitSeededStencils(l.HubPath, fabricengine.StencilsSubtreeRel(), stencilsDir, written, fabricengine.SyncCommitMessage("stencils", binaryLabel), rec)
			if commitErr != nil {
				clihelp.SetExit(cmd.Context(), errWithRecord(out, rec.Snapshot(), commitErr))
				return nil
			}

			// The specs half repeats the stencils half's force-refresh and commit pair against the deployed-specs subtree.
			// It accumulates into the same rec, so one envelope reports both passes' mutations.
			// Its sourceDir is empty because specs have no worktree source tree;
			// see the specs-reconcile-passes-no-source-dir Shared Decision.
			//
			// A failure here returns the same shapes as the stencils half:
			// a force-refresh failure is a bare output.Err,
			// and a commit failure is errWithRecord carrying the snapshot so far.
			// That snapshot already includes the stencils half's mutations.
			// Both return early, leaving the stencils commit landed and reported as partial.
			specsDir := fabricengine.SpecsDir(l.HubPath)
			specsWritten, specsErr := stencilstore.ForceRefresh(specsDir, specs.Registry(), mode, fabricengine.StencilSource(l.WorktreePath(), "", running))
			if specsErr != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, specsErr.Error()))
				return nil
			}
			specsRes, specsCommitErr := fabricengine.CommitSeededStencils(l.HubPath, fabricengine.SpecsSubtreeRel(), specsDir, specsWritten, fabricengine.SyncCommitMessage("specs", binaryLabel), rec)
			if specsCommitErr != nil {
				clihelp.SetExit(cmd.Context(), errWithRecord(out, rec.Snapshot(), specsCommitErr))
				return nil
			}

			clihelp.SetExit(cmd.Context(), okWithRecord(out, rec.Snapshot(), map[string]any{
				"committed":       res.Committed,
				"sha":             res.SHA,
				"specs_committed": specsRes.Committed,
				"specs_sha":       specsRes.SHA,
			}))
			return nil
		},
	}

	loc := func() *lymcwd.Location { return l }

	cmd.AddCommand(listCmd, validateCmd, syncCmd, newDiffCmd(loc), newPromoteCmd(loc))
	return cmd
}

// RunCLI is the public seam for the stencil module CLI.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the stencil CLI in cwd, or in the process working directory when cwd is empty.
// The empty case goes through clihelp.Execute, because lymcwd.WithCwd panics on an empty directory.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}

// Kind labels distinguish list's two row classes.
// kindStencil marks a row read from fabricengine.StencilsDir and stencils.Registry.
// kindSpec marks a row read from fabricengine.SpecsDir and specs.Registry.
//
// validate, diff, and promote do not run a specs pass.
// validate compares top-level marker sets through stencil.TopLevelMarkers; a spec is not a template,
// so both sides are empty and the pass would be a guaranteed no-op that falsely implies a check ran.
// diff and promote both need a worktree sourceDir, which specs do not have;
// see resolveSourceDir and the specs-reconcile-passes-no-source-dir Shared Decision.
const (
	kindStencil = "stencil"
	kindSpec    = "spec"
)

// stencilInfo is one list row: a registered name's board-copy path and edit state, tagged with which
// registry class (kindStencil or kindSpec) it came from.
type stencilInfo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	State string `json:"state"`
	Kind  string `json:"kind"`
}

// listRegistryEntries returns one stencilInfo per known name in registry, tagged with kind.
// Each row's board copy is read from baseDir.
// The stencils and specs passes of list share this loop.
func listRegistryEntries(baseDir string, registry stencilstore.Registry, kind string) []stencilInfo {
	var list []stencilInfo
	for _, name := range registry.Names() {
		shipped, known := registry.Default(name)
		if !known {
			continue
		}

		path := stencilstore.Path(baseDir, name)
		onDisk, readErr := os.ReadFile(path)
		exists := readErr == nil

		list = append(list, stencilInfo{
			Name:  name,
			Path:  path,
			State: classifyLabel(stencilstore.Classify(onDisk, exists, shipped)),
			Kind:  kind,
		})
	}
	return list
}

// classifyLabel maps a stencilstore.State to the three-value vocabulary that lym stencil list reports.
// StateReconciled folds into "untouched" with StateUntouched:
// both describe a board copy that already matches what a refresh would write.
// StateReconciled differs only in carrying a stamp for a superseded default.
// The next seed or refresh corrects that stamp invisibly.
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

// resolveSourceDir returns the worktree's contracts/stencils tree, or the empty string when that tree is absent.
// The empty string keeps the port-back drift warning silent in a consumer repo.
// A consumer's worktree has no such tree, so the warning would otherwise fire on every run.
func resolveSourceDir(l *lymcwd.Location) string {
	sourceDir := filepath.Join(l.WorktreePath(), "contracts", "stencils")
	if _, err := os.Stat(sourceDir); err != nil {
		return ""
	}
	return sourceDir
}

// okWithRecord and errWithRecord write the mutations and partial keys of the envelope.
// The Mutation Record Invariant requires that every mutating verb outcome carry them.
func okWithRecord(w io.Writer, rec fabricengine.Mutations, fields map[string]any) int {
	fields["mutations"] = rec.Entries()
	fields["partial"] = false
	return output.Ok(w, fields)
}

// errWithRecord reports err with the record's mutations, and sets partial when the record holds any entry.
func errWithRecord(w io.Writer, rec fabricengine.Mutations, err error) int {
	fields := map[string]any{
		"mutations": rec.Entries(),
		"partial":   rec.Len() > 0,
	}
	return output.ErrFields(w, err.Error(), fields)
}
