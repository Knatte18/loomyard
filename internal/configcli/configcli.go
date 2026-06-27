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

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/config"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/paths"
	"github.com/Knatte18/loomyard/internal/weft"
)

// syncFunc runs the post-edit sync, writing its output to the given writer,
// and returns an exit code.
type syncFunc func(w io.Writer) int

// editOne edits a single config module and optionally syncs on success.
//
// Flow:
// 1. Look up the template for the given module name via templateFor.
// 2. If unknown, print an error message listing known modules and return 1.
// 3. Call config.Edit to open the file in the editor and validate YAML.
// 4. If config.Edit returns config.ErrAborted, print the abort message and return 1.
// 5. If config.Edit returns any other error, print it and return 1.
// 6. On success, call sync with a buffered writer to capture its output.
// 7. If sync returns 0, discard the buffer and print the success message.
// 8. If sync returns non-zero, print a failure message with the sync output and return 1.
func editOne(baseDir string, out io.Writer, module string, edit config.EditorFunc, sync syncFunc) int {
	// Look up the template for this module.
	template, ok := configreg.Template(module)
	if !ok {
		fmt.Fprintf(out, "unknown config module: %s (known: %v)\n", module, configreg.Names())
		return 1
	}

	// Call config.Edit to open the file in the editor.
	err := config.Edit(baseDir, module, template(), edit)
	if err != nil {
		// Check if this is an abort (user saved without fixing YAML).
		if errors.Is(err, config.ErrAborted) {
			fmt.Fprintf(out, "aborted: _lyx/config/%s.yaml unchanged\n", module)
			return 1
		}
		// Any other error (I/O, parse loop termination, etc.).
		fmt.Fprintf(out, "%v\n", err)
		return 1
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
	fmt.Fprintf(out, "edited _lyx/config/%s.yaml but weft sync failed: %s\n", module, buf.String())
	return 1
}

// dispatch routes the config command to either editOne (if a module is specified)
// or menu (for the interactive numbered menu).
//
// The baseDir is computed from the layout as filepath.Join(WorktreeRoot, RelPath),
// which is the host _lyx parent.
func dispatch(l *paths.Layout, in io.Reader, out io.Writer, args []string, edit config.EditorFunc, sync syncFunc) int {
	baseDir := filepath.Join(l.WorktreeRoot, l.RelPath)
	if len(args) >= 1 {
		return editOne(baseDir, out, args[0], edit, sync)
	}
	return menu(l, baseDir, in, out, edit, sync)
}

// Command returns the cobra command for lyx config.
//
// The returned command is a leaf with Use "config [module]". It accepts an
// optional module-name positional: no positional opens the interactive menu;
// one positional edits that module directly. ValidArgs is set to the known
// config module names for shell completion only — validation of an unknown
// module is left to the handler, which prints the existing plain-text error.
// The public RunCLI seam delegates here via clihelp.Execute so all in-process
// callers continue to work unchanged.
func Command() *cobra.Command {
	return &cobra.Command{
		Use:   "config [module]",
		Short: "edit module configuration",
		Long: `config edits a module's configuration in _lyx/config/ and syncs weft on
success. With no argument it opens an interactive numbered menu of the known
modules; with a module name it edits that module directly.`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: configreg.Names(),
		RunE:      clihelp.WrapRun(runConfig),
	}
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
// Behaviour is unchanged from the pre-cobra RunCLI: no positional → interactive
// menu; one positional → editOne for that module; unknown module → the existing
// plain-text error from editOne.
func runConfig(out io.Writer, args []string) int {
	// Resolve the current working directory.
	cwd, err := paths.Getwd()
	if err != nil {
		fmt.Fprintf(out, "%v\n", err)
		return 1
	}

	// Resolve the layout.
	l, err := paths.Resolve(cwd)
	if err != nil {
		fmt.Fprintf(out, "%v\n", err)
		return 1
	}

	// Build the real editor and sync functions.
	realSync := func(w io.Writer) int {
		return weft.RunCLI(w, []string{"sync"})
	}

	// Dispatch to the interactive menu or specific module.
	return dispatch(l, os.Stdin, out, args, config.DefaultEditor, realSync)
}
