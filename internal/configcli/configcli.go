// configcli.go — configuration CLI command.
//
// Implements the lyx config command, which edits module configurations and triggers weft sync.

package configcli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/weftcli"
)

// syncFunc runs the post-edit sync, writing its output to the given writer,
// and returns an exit code.
type syncFunc func(w io.Writer) int

// printModule reads and writes the on-disk YAML for a single config module to out.
//
// It validates the module name against the registry before touching the filesystem,
// returning an output.Err JSON envelope when the module is unknown or the file
// cannot be read. On os.IsNotExist it returns a descriptive "not configured" message
// rather than a raw OS error so the caller gets actionable output. On success the raw
// file bytes are written verbatim and exit 0 is returned.
func printModule(baseDir string, out io.Writer, module string) int {
	// Validate the module name against the registry before touching the filesystem.
	if _, ok := configreg.Template(module); !ok {
		return output.Err(out, fmt.Sprintf("unknown config module: %s (known: %v)", module, configreg.Names()))
	}

	path := hubgeometry.ConfigFile(baseDir, module)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return output.Err(out, fmt.Sprintf("config module %s not configured (path: %s)", module, path))
		}
		return output.Err(out, fmt.Sprintf("read config file: %v", err))
	}

	// Write the raw YAML bytes verbatim; on success exit 0.
	_, _ = out.Write(data)
	return 0
}

// printAll writes a header-delimited YAML dump of all config modules to out.
//
// Each module section starts with a "# <name>" delimiter line. If the config file
// exists its YAML content follows; if the file is absent a "# (not configured)"
// comment is written instead. Read errors other than not-found are fatal and return
// an output.Err envelope immediately rather than emitting partial output.
//
// The aggregate form never errors on absence — it exits 0 regardless of how many
// modules are configured so callers can use it for inspection without a fully-seeded
// workspace.
func printAll(baseDir string, out io.Writer) int {
	for _, name := range configreg.Names() {
		// Write a section delimiter so the reader can separate module blocks.
		fmt.Fprintf(out, "# %s\n", name)

		path := hubgeometry.ConfigFile(baseDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(out, "# (not configured)\n")
				continue
			}
			// Unexpected I/O failure; abort rather than emit partial output.
			return output.Err(out, fmt.Sprintf("read config file: %v", err))
		}

		// Write the file's YAML content; ensure a trailing newline for clean section separation.
		_, _ = out.Write(data)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			fmt.Fprintf(out, "\n")
		}
	}
	return 0
}

// editOne edits a single config module and optionally syncs on success.
//
// Flow:
// 1. Look up the template for the given module name via templateFor.
// 2. If unknown, print an error message listing known modules and return 1.
// 3. Call configengine.Edit to open the file in the editor and validate YAML.
// 4. If configengine.Edit returns configengine.ErrAborted, print the abort message and return 1.
// 5. If configengine.Edit returns any other error, print it and return 1.
// 6. On success, call sync with a buffered writer to capture its output.
// 7. If sync returns 0, discard the buffer and print the success message.
// 8. If sync returns non-zero, print a failure message with the sync output and return 1.
func editOne(baseDir string, out io.Writer, module string, edit configengine.EditorFunc, sync syncFunc) int {
	// Look up the template for this module.
	template, ok := configreg.Template(module)
	if !ok {
		return output.Err(out, fmt.Sprintf("unknown config module: %s (known: %v)", module, configreg.Names()))
	}

	// Call configengine.Edit to open the file in the editor.
	err := configengine.Edit(baseDir, module, template(), edit)
	if err != nil {
		// Check if this is an abort (user saved without fixing YAML).
		if errors.Is(err, configengine.ErrAborted) {
			return output.Err(out, fmt.Sprintf("aborted: _lyx/config/%s.yaml unchanged", module))
		}
		// Any other error (I/O, parse loop termination, etc.).
		return output.Err(out, err.Error())
	}

	// Edit succeeded; now call sync and capture its output.
	var buf bytes.Buffer
	exitCode := sync(&buf)
	if exitCode == 0 {
		// Sync succeeded; discard output to keep the stream clean.
		fmt.Fprintf(out, "edited and synced _lyx/config/%s.yaml\n", module)
		return 0
	}

	// Sync failed; include its output in the failure message for diagnosis.
	return output.Err(out, fmt.Sprintf("edited _lyx/config/%s.yaml but weft sync failed: %s", module, buf.String()))
}

// dispatch routes the config command to the print path (when printOnly is true),
// editOne (if a module is specified), or menu (for the interactive numbered menu).
//
// When printOnly is true the command is read-only: it writes on-disk YAML to out
// without opening an editor. The print path is evaluated before any edit/menu logic.
// The baseDir is computed from the layout as filepath.Join(WorktreeRoot, RelPath).
func dispatch(l *hubgeometry.Layout, in io.Reader, out io.Writer, args []string, edit configengine.EditorFunc, sync syncFunc, printOnly bool) int {
	baseDir := filepath.Join(l.WorktreeRoot, l.RelPath)

	// Handle --print before any edit/menu dispatch; the print path is read-only
	// and never opens the editor.
	if printOnly {
		if len(args) >= 1 {
			return printModule(baseDir, out, args[0])
		}
		return printAll(baseDir, out)
	}

	if len(args) >= 1 {
		return editOne(baseDir, out, args[0], edit, sync)
	}
	return menu(l, baseDir, in, out, edit, sync)
}

// buildConfigLong constructs the Long description for the config command,
// embedding the live list of known modules from the registry so the help text
// stays in sync without requiring manual updates when modules are added or removed.
func buildConfigLong() string {
	return "config edits a module's configuration in _lyx/config/ and syncs weft on\n" +
		"success. With no argument it opens an interactive numbered menu of the known\n" +
		"modules; with a module name it edits that module directly.\n\n" +
		"Use --print to print the on-disk YAML without launching the editor.\n\n" +
		"Known modules: " + strings.Join(configreg.Names(), ", ") + "."
}

// runReconcile is the package-private handler for the lyx config reconcile subcommand.
//
// It reconciles all module config files against their templates via configsync.ReconcileAll
// and emits a JSON envelope. When apply is false (dry-run) no files are written; when
// apply is true, changes are committed to disk atomically. Returns exit code 0 on success,
// 1 on any error.
func runReconcile(out io.Writer, apply bool) int {
	// Resolve the current working directory and layout.
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, fmt.Sprintf("getwd: %v", err))
	}

	l, err := hubgeometry.Resolve(cwd)
	if err != nil {
		return output.Err(out, fmt.Sprintf("resolve layout: %v", err))
	}

	// Compute baseDir as the host _lyx parent: the worktree root joined with the relative path.
	baseDir := filepath.Join(l.WorktreeRoot, l.RelPath)

	// Reconcile all modules; apply controls whether changes are written to disk.
	results, err := configsync.ReconcileAll(baseDir, apply)
	if err != nil {
		return output.Err(out, fmt.Sprintf("reconcile: %v", err))
	}

	// Build module result objects for JSON output.
	modules := make([]map[string]any, len(results))
	for i, result := range results {
		modules[i] = map[string]any{
			"module":  result.Module,
			"added":   result.Added,
			"removed": result.Removed,
			"applied": result.Applied,
		}
	}

	// Emit the JSON envelope with the aggregate applied flag and per-module details.
	return output.Ok(out, map[string]any{
		"applied": apply,
		"modules": modules,
	})
}

// Command returns the cobra command for lyx config.
//
// The returned command uses a configCmd variable (closure pattern) so that the
// --print flag value is readable in the RunE handler. The --print flag makes
// config read-only: it prints on-disk YAML without launching the editor.
// Args is cobra.MaximumNArgs(1) so extra positionals are rejected. ValidArgs is
// set to the known config module names for shell completion only. A reconcile
// subcommand is registered so that "lyx config reconcile" is routed there while
// "lyx config <module>" continues to invoke the edit/menu RunE.
func Command() *cobra.Command {
	configCmd := &cobra.Command{
		Use:       "config [module]",
		Short:     "edit module configuration",
		Long:      buildConfigLong(),
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: configreg.Names(),
	}
	configCmd.Flags().Bool("print", false, "print on-disk config as YAML without launching the editor")
	// The RunE closure captures configCmd so the --print flag is readable without
	// consulting os.Args directly.
	configCmd.RunE = clihelp.WrapRun(func(out io.Writer, args []string) int {
		printOnly, _ := configCmd.Flags().GetBool("print")
		return runConfig(out, args, printOnly)
	})

	// Build the reconcile subcommand and register it so cobra routes
	// "lyx config reconcile" here while "lyx config <module>" continues
	// to invoke the edit/menu RunE above.
	reconcileCmd := &cobra.Command{
		Use:   "reconcile",
		Short: "reconcile module configs against templates",
		Long: `reconcile compares all module configuration files in _lyx/config/ against
their live templates, reporting added keys (new in template) and removed keys
(deleted from template). By default it is a dry-run: no files are written.
Pass --apply to write the reconciled files to disk atomically.`,
	}
	apply := reconcileCmd.Flags().Bool("apply", false, "apply changes to disk (default: dry-run)")
	reconcileCmd.RunE = clihelp.WrapRun(func(out io.Writer, args []string) int {
		return runReconcile(out, *apply)
	})
	configCmd.AddCommand(reconcileCmd)

	return configCmd
}

// RunCLI is the public seam for the lyx config command.
//
// It delegates to clihelp.Execute(Command(), out, args) so that all existing
// in-process callers and tests compile and pass unchanged. The cobra command
// carries both stdout and stderr into out for single-buffer test capture.
func RunCLI(out io.Writer, args []string) int {
	return clihelp.Execute(Command(), out, args)
}

// runConfig is the package-private handler for the lyx config command.
//
// It resolves the layout from the current working directory, builds the real
// editor (DefaultEditor) and the real sync function (weft.RunCLI with "sync"),
// and dispatches to dispatch with os.Stdin as the interactive input reader.
// When printOnly is true the command is read-only: it prints on-disk YAML
// without opening an editor or running sync.
func runConfig(out io.Writer, args []string, printOnly bool) int {
	// Resolve the current working directory.
	cwd, err := hubgeometry.Getwd()
	if err != nil {
		return output.Err(out, err.Error())
	}

	// Resolve the layout.
	l, err := hubgeometry.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// Build the real editor and sync functions.
	realSync := func(w io.Writer) int {
		return weftcli.RunCLI(w, []string{"sync"})
	}

	// Dispatch to the print path, interactive menu, or specific module.
	return dispatch(l, os.Stdin, out, args, configengine.DefaultEditor, realSync, printOnly)
}
