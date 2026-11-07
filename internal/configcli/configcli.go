// configcli.go — configuration CLI command.
//
// Implements the lym config command, which edits module configurations.
// A hub-wide module is committed in the Bolt checkout; any other module syncs fabric.

package configcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shellguard"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// syncFunc runs the post-edit sync, writing its output to the given writer,
// and returns an exit code.
type syncFunc func(w io.Writer) int

// printModule reads and writes the on-disk YAML for a single config module to out.
//
// It validates the module name against the registry before touching the filesystem,
// returning an output.Err JSON envelope when the module is unknown or the file cannot be read.
// On os.IsNotExist it returns a descriptive "not configured" message rather than a raw OS error,
// so the caller gets actionable output.
// On success the raw file bytes are written verbatim and exit 0 is returned.
// The file is the selected layer's:
// without --layer, a hub-wide module's at the board dir and any other at the worktree.
func printModule(dirs configDirs, out io.Writer, module string) int {
	// Validate the module name against the registry before touching the filesystem.
	mod, ok := configreg.Lookup(module)
	if !ok {
		return output.Err(out, fmt.Sprintf("unknown config module: %s (known: %v)", module, configreg.Names()))
	}

	path, _, err := dirs.layerFile(mod)
	if err != nil {
		return output.Err(out, err.Error())
	}
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

// printResolved writes module's resolved configuration to out, each unit annotated with the layer it came from.
// It resolves through the same stack the module's loader does,
// so an absent base layer or an invalid layer file is an error envelope naming the failure.
func printResolved(dirs configDirs, out io.Writer, module string) int {
	mod, ok := configreg.Lookup(module)
	if !ok {
		return output.Err(out, fmt.Sprintf("unknown config module: %s (known: %v)", module, configreg.Names()))
	}
	resolved, err := configengine.Provenance(dirs.engineDirs(), mod.Spec())
	if err != nil {
		return output.Err(out, err.Error())
	}
	_, _ = out.Write(resolved)
	return 0
}

// printAll writes a header-delimited YAML dump of all config modules to out.
//
// Each module section starts with a "# <name>" delimiter line.
// If the config file exists, its YAML content follows;
// if the file is absent, a "# (not configured)" comment is written instead.
// Read errors other than not-found are fatal;
// they return an output.Err envelope immediately rather than emitting partial output.
//
// The aggregate form never errors on absence: it exits 0 whatever modules are configured.
// It lets callers inspect config without a fully seeded workspace.
// Each section is the module's own default file: a hub-wide module's at the board dir, any other at the worktree.
func printAll(dirs configDirs, out io.Writer) int {
	dirs.layer = ""
	for _, mod := range configreg.Modules() {
		name := mod.Name
		// Write a section delimiter so the reader can separate module blocks.
		fmt.Fprintf(out, "# %s\n", name)

		path, _, err := dirs.layerFile(mod)
		if err != nil {
			return output.Err(out, err.Error())
		}
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

// editOne edits a single config module's selected layer.
// On success, a default-layer file is committed in the Bolt checkout;
// a local override is left uncommitted, and a worktree file syncs fabric.
func editOne(dirs configDirs, out io.Writer, module string, edit configengine.EditorFunc, sync syncFunc, commit hubCommitFunc) int {
	mod, ok := configreg.Lookup(module)
	if !ok {
		return output.Err(out, fmt.Sprintf("unknown config module: %s (known: %v)", module, configreg.Names()))
	}
	path, layer, err := dirs.layerFile(mod)
	if err != nil {
		return output.Err(out, err.Error())
	}
	switch layer {
	case configengine.LayerDefault:
		if exitCode, refused := dirs.refuseHubWrite(out); refused {
			return exitCode
		}
		return editHubWide(dirs, out, mod, edit, commit)
	case configengine.LayerMachine:
		if exitCode, refused := dirs.refuseHubWrite(out); refused {
			return exitCode
		}
		return editMachine(dirs, out, mod, path, edit)
	case configengine.LayerLocal:
		return editLocal(dirs, out, mod, path, edit)
	}

	err = configengine.Edit(dirs.engineDirs(), mod.Spec(), configengine.LayerWorktree, edit)
	if err != nil {
		// Check if this is an abort (user saved without fixing YAML).
		if errors.Is(err, configengine.ErrAborted) {
			return output.Err(out, fmt.Sprintf("aborted: _lym/config/%s.yaml unchanged", module))
		}
		// Any other error (I/O, parse loop termination).
		return output.Err(out, err.Error())
	}

	// A worktree with no board is no hub's, so there is nothing to sync.
	if dirs.board == "" {
		return output.Ok(out, map[string]any{
			"module":  module,
			"message": fmt.Sprintf("edited _lym/config/%s.yaml", module),
		})
	}

	var buf bytes.Buffer
	exitCode := sync(&buf)
	if exitCode == 0 {
		return output.Ok(out, map[string]any{
			"module":  module,
			"message": fmt.Sprintf("edited and synced _lym/config/%s.yaml", module),
		})
	}

	// Sync failed; include its output in the failure message for diagnosis.
	return output.Err(out, fmt.Sprintf("edited _lym/config/%s.yaml but fabric sync failed: %s", module, buf.String()))
}

// parseSetFlags parses raw "key=value" strings, as collected from repeated --set flags, into yamlengine.KV pairs.
//
// Each entry is split on the first '=' only, so a value may itself contain '=' characters without being truncated.
// An entry with no '=' at all is malformed;
// it returns a descriptive error rather than silently treating the whole string as a key with an empty value.
func parseSetFlags(raw []string) ([]yamlengine.KV, error) {
	pairs := make([]yamlengine.KV, 0, len(raw))
	for _, entry := range raw {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid --set value %q: expected key=value", entry)
		}
		pairs = append(pairs, yamlengine.KV{Key: parts[0], Value: parts[1]})
	}
	return pairs, nil
}

// setModule writes pairs into a single config module's file and optionally syncs on success.
// It applies them with no editor: configengine.Set performs the write non-interactively in one call.
// A default-layer file is written at the board dir and committed in the Bolt checkout;
// a local override is written uncommitted, and a worktree file syncs fabric.
// A value the module's loader refuses leaves the file as it was.
func setModule(dirs configDirs, out io.Writer, module string, pairs []yamlengine.KV, sync syncFunc, commit hubCommitFunc) int {
	// Look up the module; its declared open maps let --set add or rewrite one entry.
	mod, ok := configreg.Lookup(module)
	if !ok {
		return output.Err(out, fmt.Sprintf("unknown config module: %s (known: %v)", module, configreg.Names()))
	}
	path, layer, err := dirs.layerFile(mod)
	if err != nil {
		return output.Err(out, err.Error())
	}
	switch layer {
	case configengine.LayerDefault:
		if exitCode, refused := dirs.refuseHubWrite(out); refused {
			return exitCode
		}
		return setHubWide(dirs, out, mod, pairs, commit)
	case configengine.LayerMachine:
		if exitCode, refused := dirs.refuseHubWrite(out); refused {
			return exitCode
		}
		return setMachine(dirs, out, mod, path, pairs)
	case configengine.LayerLocal:
		return setLocal(dirs, out, mod, path, pairs)
	}

	// Apply pairs directly, with no editor invocation;
	// a worktree file under a board is sparse and holds only the keys set.
	preserved, err := setChecked(dirs.engineDirs(), mod, configengine.LayerWorktree, path, pairs)
	if err != nil {
		return output.Err(out, err.Error())
	}

	// A worktree with no board is no hub's, so there is nothing to sync.
	var buf bytes.Buffer
	exitCode := 0
	message := fmt.Sprintf("edited and synced _lym/config/%s.yaml", module)
	if dirs.board == "" {
		message = fmt.Sprintf("edited _lym/config/%s.yaml", module)
	} else {
		exitCode = sync(&buf)
	}
	if exitCode == 0 {
		// "preserved" is only included when non-empty,
		// so a clean write's envelope carries no such field.
		fields := map[string]any{
			"module":  module,
			"message": message,
		}
		if len(preserved) > 0 {
			fields["preserved"] = preserved
		}
		return output.Ok(out, fields)
	}

	// Sync failed; include its output in the failure message for diagnosis.
	return output.Err(out, fmt.Sprintf("edited _lym/config/%s.yaml but fabric sync failed: %s", module, buf.String()))
}

// setChecked writes pairs into the layer's file of module at path and checks the result with the module's own loader.
// A write that leaves the stack failing to load with a new error is undone, and path is put back as it was.
// The error names the module and carries the loader's, which names the key.
// A stack that already failed to load lets a write through that leaves that same failure in place.
func setChecked(dirs configengine.Dirs, module configreg.Module, layer configengine.Layer, path string, pairs []yamlengine.KV) ([]string, error) {
	snap, err := snapshotFile(path)
	if err != nil {
		return nil, err
	}
	before := module.CheckLoad(dirs)
	preserved, err := configengine.Set(dirs, module.Spec(), layer, pairs)
	if err != nil {
		return nil, err
	}
	if failure := newLoadFailure(module, before, module.CheckLoad(dirs)); failure != nil {
		return nil, rollBack(path, snap, fmt.Errorf("%w; %s unchanged", failure, path))
	}
	return preserved, nil
}

// newLoadFailure returns a refusal when the module's check, run after a write, fails with a new error.
// The error is new when it differs from the one the check gave before the write.
func newLoadFailure(module configreg.Module, before, after error) error {
	if after == nil || (before != nil && before.Error() == after.Error()) {
		return nil
	}
	return fmt.Errorf("the %s config would not load with this value: %w; way forward: --set the key to a value it takes", module.Name, after)
}

// setLocal writes pairs into a module's local override file at path.
// A local override is never committed and runs no fabric sync.
// A key the local layer refuses, or a value the module's loader refuses, leaves the file as it was.
func setLocal(dirs configDirs, out io.Writer, mod configreg.Module, path string, pairs []yamlengine.KV) int {
	preserved, err := setChecked(dirs.engineDirs(), mod, configengine.LayerLocal, path, pairs)
	if err != nil {
		return output.Err(out, err.Error())
	}
	fields := map[string]any{
		"module":  mod.Name,
		"message": fmt.Sprintf("edited local override %s; it is not committed", path),
	}
	if len(preserved) > 0 {
		fields["preserved"] = preserved
	}
	return output.Ok(out, fields)
}

// setMachine writes pairs into a hub-wide module's machine override file at path.
// A machine override is never committed and runs no fabric sync.
// A key the machine layer refuses leaves the file as it was.
// So does a value the module's loader refuses over the prime's worktree stack.
func setMachine(dirs configDirs, out io.Writer, mod configreg.Module, path string, pairs []yamlengine.KV) int {
	preserved, err := setChecked(dirs.engineDirs(), mod, configengine.LayerMachine, path, pairs)
	if err != nil {
		return output.Err(out, err.Error())
	}
	fields := map[string]any{
		"module":  mod.Name,
		"message": fmt.Sprintf("edited machine override %s; it is not committed", path),
	}
	if len(preserved) > 0 {
		fields["preserved"] = preserved
	}
	return output.Ok(out, fields)
}

// editMachine edits a hub-wide module's machine override file at path in the editor.
// A machine override is never committed and runs no fabric sync.
// A saved key the machine layer refuses puts the file back to its previous bytes.
// So does a save that leaves the stack newly failing to load.
// Where the edit created the file, it is removed rather than restored.
func editMachine(dirs configDirs, out io.Writer, mod configreg.Module, path string, edit configengine.EditorFunc) int {
	snap, err := snapshotFile(path)
	if err != nil {
		return output.Err(out, err.Error())
	}
	before := mod.CheckLoad(dirs.engineDirs())
	if err := configengine.Edit(dirs.engineDirs(), mod.Spec(), configengine.LayerMachine, edit); err != nil {
		if errors.Is(err, configengine.ErrAborted) {
			return output.Err(out, fmt.Sprintf("aborted: %s unchanged", path))
		}
		return output.Err(out, err.Error())
	}
	if failure := newLoadFailure(mod, before, mod.CheckLoad(dirs.engineDirs())); failure != nil {
		return output.Err(out, rollBack(path, snap, fmt.Errorf("%w; %s unchanged", failure, path)).Error())
	}
	return output.Ok(out, map[string]any{
		"module":  mod.Name,
		"message": fmt.Sprintf("edited machine override %s; it is not committed", path),
	})
}

// editLocal edits a module's local override file at path in the editor.
// A local override is never committed and runs no fabric sync.
// A saved key the local layer refuses leaves the file as it was.
func editLocal(dirs configDirs, out io.Writer, mod configreg.Module, path string, edit configengine.EditorFunc) int {
	if err := configengine.Edit(dirs.engineDirs(), mod.Spec(), configengine.LayerLocal, edit); err != nil {
		if errors.Is(err, configengine.ErrAborted) {
			return output.Err(out, fmt.Sprintf("aborted: %s unchanged", path))
		}
		return output.Err(out, err.Error())
	}
	return output.Ok(out, map[string]any{
		"module":  mod.Name,
		"message": fmt.Sprintf("edited local override %s; it is not committed", path),
	})
}

// dispatch routes the config command to one of three paths.
// It takes the --set path when setFlags is non-empty.
// It takes the print path when printOnly is true, and otherwise editOne when a module is given.
//
// The print path is read-only: it writes the on-disk YAML to out and never opens an editor.
// The --set path writes without calling edit and is mutually exclusive with --print.
// layer is the --layer value, empty or one of the four layer names.
// Any other value is refused before anything is read or written.
// hubWriteGuard is called before a default-layer write and refuses it with its message; a nil guard allows every write.
// terminal reports whether the command's input is an interactive terminal.
// Without one, an editor path is refused before the layer is dispatched,
// so no guard is consulted, no file is scaffolded and no editor runs.
func dispatch(l *lymcwd.Location, out io.Writer, args []string, edit configengine.EditorFunc, sync syncFunc, commit hubCommitFunc, printOnly bool, setFlags []string, layer string, hubWriteGuard func() (string, error), terminal bool) int {
	if !validLayer(layer) {
		return output.Err(out, fmt.Sprintf("unknown --layer %q; use default, worktree, machine or local", layer))
	}
	dirs := dirsOf(l)
	dirs.layer = layer
	dirs.hubWriteGuard = hubWriteGuard

	// The --set path never opens the editor, so its validation runs before the --print and edit branches can act:
	// mutual exclusivity with --print, and a module being required.
	if len(setFlags) > 0 && printOnly {
		return output.Err(out, "--print and --set are mutually exclusive")
	}
	if len(setFlags) > 0 && len(args) < 1 {
		return output.Err(out, "module required with --set")
	}
	if len(setFlags) > 0 {
		pairs, err := parseSetFlags(setFlags)
		if err != nil {
			return output.Err(out, err.Error())
		}
		return setModule(dirs, out, args[0], pairs, sync, commit)
	}

	// The --print path is read-only and never opens the editor, so it runs before any edit dispatch.
	if printOnly {
		if len(args) >= 1 {
			return printModule(dirs, out, args[0])
		}
		return printAll(dirs, out)
	}

	// Command lists modules for a bare invocation before dispatch runs,
	// so a module is always present here.
	if len(args) < 1 {
		return output.Err(out, `module required; run "lym config" to list modules and verbs`)
	}
	// The editor paths need a terminal; --set and --print above need none.
	if !terminal {
		return output.Err(out, noTerminalRefusal(args[0], layer))
	}
	return editOne(dirs, out, args[0], edit, sync, commit)
}

// stdinIsTerminal reports whether in is an *os.File whose descriptor is a terminal; any other reader is not one.
func stdinIsTerminal(in io.Reader) bool {
	file, ok := in.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

// noTerminalRefusal words the refusal of an editor path run without a terminal.
// The way forward names module, or a placeholder when module is empty.
// A non-empty layer is carried into both suggested commands, so the retry addresses the same layer.
func noTerminalRefusal(module, layer string) string {
	if module == "" {
		module = "<module>"
	}
	target := "lym config " + module
	if layer != "" {
		target += " --layer " + layer
	}
	return fmt.Sprintf(`lym config opens an editor only from an interactive terminal, and stdin is not one; way forward: "%s --print" to read, "%s --set key=value" to write`, target, target)
}

// baseDirOf returns the enclosing _lym parent: the worktree root joined with the relative path.
func baseDirOf(l *lymcwd.Location) string {
	return filepath.Join(l.WorktreePath(), l.AnchorRel)
}

// buildConfigLong builds the long help text of the config command.
// It embeds the hub-wide modules from the registry, so the text stays in sync when modules are added or removed.
func buildConfigLong() string {
	var hubWide []string
	for _, mod := range configreg.Modules() {
		if mod.HubWide {
			hubWide = append(hubWide, mod.Name)
		}
	}
	return "config edits a module's configuration in _lym/config/ and syncs fabric on\n" +
		"success. A hub-wide module (" + strings.Join(hubWide, ", ") + ") is edited instead at the\n" +
		"hub's board dir and committed in " + fabricengine.BoltDirName + ". With no argument it lists the known\n" +
		"modules and verbs; with a module\n" +
		"name it edits that module directly; `lym config menu` is the interactive\n" +
		"picker. The editor is $VISUAL, then $EDITOR, then `code --wait` when code is\n" +
		"on PATH, then notepad on Windows or nano and vi elsewhere.\n\n" +
		"--print prints the on-disk YAML, with no editor.\n\n" +
		"Use --set key=value (repeatable) to write config values with no editor, e.g.\n" +
		"  lym config board --set design_prefix=foo- --set readme=Home.md\n" +
		"A list key takes a YAML flow list, replaced whole, e.g.\n" +
		"  lym config landing --set 'deploy_branches=[]'\n" +
		"An open map's keys are the repo's own; --set adds or rewrites one entry\n" +
		"or a key inside it, e.g.\n" +
		"  lym config gate --set tiers.tier0.env.LYM_TEST_TIER=0\n" +
		"A value the loader refuses is refused, naming the key.\n" +
		"Keys the template lacks are preserved (JSON \"preserved\" field);\n" +
		"\"lym config reconcile --layer default --apply\" from the prime removes them from the\n" +
		"board dir's files, and \"lym config reconcile --apply\" from a worktree file.\n\n" +
		"--layer default|worktree|machine|local picks the file to print or write: default is the board's\n" +
		"repo default, committed from the prime; local is an untracked override.\n\n" +
		"Python guard off for a hub (hook and notice), from the prime:\n" +
		"  lym config shuttle --layer default --set " + string(shellguard.SwitchDenyPython) + "=false\n\n" +
		"--resolved <module> prints the merged result, each unit annotated with its\n" +
		"layer: template, default, worktree, machine or local.\n\n" +
		"Known modules: " + strings.Join(configreg.Names(), ", ") + "."
}

// resolvedRefusal returns why --resolved cannot run beside the other flags and arguments, or "" when it can.
func resolvedRefusal(args []string, printOnly bool, setFlags []string, layer string) string {
	switch {
	case printOnly:
		return "--resolved and --print are mutually exclusive"
	case len(setFlags) > 0:
		return "--resolved and --set are mutually exclusive"
	case layer != "":
		return "--resolved and --layer are mutually exclusive"
	case len(args) < 1:
		return "module required with --resolved"
	}
	return ""
}

// runResolved is the package-private handler for lym config <module> --resolved.
// It resolves the location from the working directory in the context.
// It prints the module's resolved configuration, each unit annotated with its layer.
func runResolved(ctx context.Context, out io.Writer, module string) int {
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}
	l, err := lymcwd.Resolve(cwd)
	if err != nil {
		return output.Err(out, err.Error())
	}
	return printResolved(dirsOf(l), out, module)
}

// runReconcile is the package-private handler for the lym config reconcile subcommand.
//
// With no layer it reconciles the worktree's own and local config files against their templates.
// With layer default it reconciles every file under the Bolt checkout instead, guarded and committed there on apply.
// Any other layer is refused.
// A run whose worktree is the Bolt checkout itself is refused too.
// It would reconcile the hub's files as a worktree's.
// It emits a JSON envelope.
// When apply is false (dry-run) no files are written.
// Returns exit code 0 on success, 1 on any error.
func runReconcile(ctx context.Context, out io.Writer, apply bool, layer string) int {
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return output.Err(out, fmt.Sprintf("getwd: %v", err))
	}

	l, err := lymcwd.Resolve(cwd)
	if err != nil {
		// The resolve error is already self-describing, so it is passed through without a prefix.
		return output.Err(out, err.Error())
	}

	boltDir := fabricengine.BoltDir(l.HubPath)
	if filepath.Clean(l.WorktreePath()) == filepath.Clean(boltDir) {
		return output.Err(out, fmt.Sprintf("reconcile is refused in the board dir %s, whose files are the hub's, not a worktree's; way forward: run \"lym config reconcile --layer default\" from the hub's prime worktree", boltDir))
	}
	if layer != "" && layer != string(configengine.LayerDefault) {
		return output.Err(out, fmt.Sprintf("reconcile takes --layer %s alone, for the files under the hub's board dir; way forward: run it with no --layer to reconcile this worktree's own and local files", configengine.LayerDefault))
	}
	if layer != "" {
		return reconcileDefaultLayer(l, out, apply, realHubWriteGuard(l))
	}

	baseDir := baseDirOf(l)

	results, err := configsync.ReconcileAll(baseDir, boltDir, configsync.Options{Apply: apply, PruneUnknown: true})
	if err != nil {
		return output.Err(out, fmt.Sprintf("reconcile: %v", err))
	}
	return output.Ok(out, reconcileEnvelope(apply, results))
}

// reconcileEnvelope builds the reconcile verb's success fields: the aggregate applied flag and one object per result.
func reconcileEnvelope(apply bool, results []configsync.Result) map[string]any {
	modules := make([]map[string]any, len(results))
	baseStripPending := false
	for i, result := range results {
		m := map[string]any{
			"module":  result.Module,
			"added":   result.Added,
			"removed": result.Removed,
			"applied": result.Applied,
			"layer":   string(result.Layer),
		}
		// migratedFrom is set only when a pre-cutover legacy config file's values were folded in during this run.
		// Ordinary modules omit it rather than carry an empty array.
		if len(result.MigratedFrom) > 0 {
			m["migratedFrom"] = result.MigratedFrom
		}
		// migrated is set only by a module with a Migrate hook that rewrote its present file.
		if len(result.Migrated) > 0 {
			m["migrated"] = result.Migrated
		}
		// retired and divergent are set only by a hub-wide module's leftover per-worktree copy.
		if result.Retired {
			m["retired"] = true
		}
		if len(result.Divergent) > 0 {
			m["divergent"] = result.Divergent
		}
		// stripped and inherited are set only on a worktree without the sparse marker;
		// reconcile reports them there as pending without stripping.
		// A default-layer result also carries stripped for an unmigrated board.
		// It lists the base units that the hub reconcile of a start verb strips.
		if len(result.Stripped) > 0 {
			m["stripped"] = result.Stripped
			if result.Layer == configengine.LayerDefault {
				baseStripPending = true
			}
		}
		if len(result.Inherited) > 0 {
			m["inherited"] = result.Inherited
		}
		// moved, keptTarget and overwritten are set only by a declared key move.
		if len(result.Moved) > 0 {
			m["moved"] = result.Moved
		}
		if len(result.KeptTarget) > 0 {
			m["keptTarget"] = result.KeptTarget
		}
		if len(result.Overwritten) > 0 {
			m["overwritten"] = result.Overwritten
		}
		// copied is set only by a declared key copy.
		if len(result.Copied) > 0 {
			m["copied"] = result.Copied
		}
		// keptExtra is set only by a module with per-name maps whose file holds a name the template lacks.
		if len(result.KeptExtra) > 0 {
			m["keptExtra"] = result.KeptExtra
		}
		// dropped is set only by a module with per-name maps whose base file holds a name with the template's value.
		if len(result.Dropped) > 0 {
			m["dropped"] = result.Dropped
		}
		// updated and differs are set only by a seed-only module's board file that follows its template.
		if len(result.Updated) > 0 {
			m["updated"] = result.Updated
		}
		if len(result.Differs) > 0 {
			m["differs"] = result.Differs
		}
		// unsettled is set only by a seed-only module whose file holds a pair the module leaves for the operator to settle.
		if len(result.Unsettled) > 0 {
			m["unsettled"] = result.Unsettled
		}
		// seed is set only by a reconcile of the board dir's files.
		if result.Seed != "" {
			m["seed"] = result.Seed
		}
		modules[i] = m
	}
	fields := map[string]any{
		"applied": apply,
		"modules": modules,
	}
	if baseStripPending {
		fields["base_strip"] = "the board's base files still hold units equal to the template; this command never strips them, and only the hub reconcile of a start verb such as `lym loom resume` or `lym batten run` does"
	}
	return fields
}

// Command returns the cobra command for lym config.
//
// The --print flag makes config read-only: it prints on-disk YAML without launching the editor.
// Extra positional arguments are rejected.
// ValidArgs names the config modules for shell completion only.
// The reconcile subcommand is registered so that lym config reconcile routes there.
// lym config <module> still invokes the edit RunE.
// The RunE decides before resolving any cwd: a bare invocation with neither --print nor --set prints the help,
// and an argument that is not a module is refused as an unknown subcommand.
func Command() *cobra.Command {
	configCmd := &cobra.Command{
		Use:         "config [<module>]",
		Short:       "edit module configuration",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long:        buildConfigLong(),
		Args:        cobra.MaximumNArgs(1),
		ValidArgs:   configreg.Names(),
	}
	configCmd.Flags().Bool("print", false, "print on-disk config as YAML without launching the editor")
	configCmd.Flags().StringArray("set", nil, "set config key=value directly, bypassing the editor (repeatable)")
	configCmd.Flags().Bool("resolved", false, "print the module's resolved config, each unit annotated with the layer it came from")
	configCmd.Flags().String("layer", "", "the layer to print or write: default, worktree, machine or local (default: the module's own file); machine is a hub-wide module's never-tracked override of its machine keys under the board dir's .lym, written from the prime")
	// The RunE closure captures configCmd so the --print and --set flags are readable without consulting os.Args.
	configCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if clihelp.ShouldAbort(cmd.Context()) {
			return nil
		}
		printOnly, _ := cmd.Flags().GetBool("print")
		setFlags, _ := cmd.Flags().GetStringArray("set")
		layer, _ := cmd.Flags().GetString("layer")
		resolved, _ := cmd.Flags().GetBool("resolved")
		if resolved {
			if refusal := resolvedRefusal(args, printOnly, setFlags, layer); refusal != "" {
				clihelp.SetExit(cmd.Context(), output.Err(cmd.OutOrStdout(), refusal))
				return nil
			}
		}
		if len(args) == 0 && !printOnly && len(setFlags) == 0 {
			return cmd.Help()
		}
		if len(args) > 0 {
			if _, ok := configreg.Template(args[0]); !ok {
				return fmt.Errorf("unknown subcommand %q for %q; run \"lym config\" to list modules and verbs", args[0], cmd.CommandPath())
			}
		}
		if resolved {
			clihelp.SetExit(cmd.Context(), runResolved(cmd.Context(), cmd.OutOrStdout(), args[0]))
			return nil
		}
		clihelp.SetExit(cmd.Context(), runConfig(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), args, printOnly, setFlags, layer))
		return nil
	}

	// reconcileCmd is registered on configCmd, so cobra routes "lym config reconcile" to it;
	// "lym config <module>" still runs configCmd's RunE.
	reconcileCmd := &cobra.Command{
		Use:         "reconcile",
		Short:       "reconcile module configs against templates",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `reconcile compares all module configuration files in _lym/config/ against
their live templates, reporting added keys (new in template) and removed keys
(deleted from template). By default it is a dry-run: no files are written.
A per-name map entry holding the template's value is reported under "dropped"
and left out of the written file.
Pass --apply to write the reconciled files to disk atomically;
a file is rewritten when the reconcile adds, removes or drops anything.
A module that retires keys or floors a value first migrates its present file: the dry-run lists
each rewrite under the module's "migrated" key, and --apply writes the migrated
file even when the reconcile adds, removes and drops nothing.
Hub-wide modules (fabric, board) are reported, not reconciled here: a leftover
per-worktree copy is "retired" when the hub file holds everything it holds
(--apply deletes it), or "divergent" when it holds an entry the hub file lacks
(it is left in place).

--layer default reconciles every file under the hub's board dir instead: the
hub-wide modules' files and each per-worktree module's repo default, seeding
an absent one from the prime. Renamed keys are migrated and keys the template
lacks are removed. --apply with it runs only from the hub's prime worktree,
outside a task session, and commits the files in ` + fabricengine.BoltDirName + ` and pushes.
Reconcile is refused in the board dir itself.

This verb is the one way to remove a key the template lacks: the reconcile a
start verb, a clone or fabric reconcile runs keeps such a key, unless the
module retired it, since a newer lym binary may have written it.`,
	}
	apply := reconcileCmd.Flags().Bool("apply", false, "apply changes to disk (default: dry-run)")
	reconcileLayer := reconcileCmd.Flags().String("layer", "", "default reconciles the files under the hub's board dir (default: this worktree's own and local files)")
	reconcileCmd.RunE = clihelp.WrapRunCtx(func(ctx context.Context, out io.Writer, args []string) int {
		return runReconcile(ctx, out, *apply, *reconcileLayer)
	})
	configCmd.AddCommand(reconcileCmd)

	// The menu subcommand reads the choice from the command's input, which clihelp.WrapRunCtx does not carry,
	// so its RunE is written by hand.
	menuCmd := &cobra.Command{
		Use:         "menu",
		Short:       "pick a module to edit interactively",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Args:        cobra.NoArgs,
	}
	menuCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if clihelp.ShouldAbort(cmd.Context()) {
			return nil
		}
		clihelp.SetExit(cmd.Context(), runMenu(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout()))
		return nil
	}
	configCmd.AddCommand(menuCmd)

	return configCmd
}

// RunCLI runs the lym config command with args and returns its exit code.
// Both stdout and stderr go to out, so a caller can capture them in one buffer.
// The working directory is the process's.
func RunCLI(out io.Writer, args []string) int {
	return RunCLIIn("", out, args)
}

// RunCLIIn runs the lym config command with args, using cwd as the working directory.
// An empty cwd means the process's working directory and goes to clihelp.Execute instead.
// lymcwd.WithCwd panics on an empty directory, so an empty cwd must not reach clihelp.ExecuteIn.
func RunCLIIn(cwd string, out io.Writer, args []string) int {
	if cwd == "" {
		return clihelp.Execute(Command(), out, args)
	}
	return clihelp.ExecuteIn(Command(), cwd, out, args)
}

// runConfig runs the lym config command for args and returns its exit code.
// It resolves the layout from the cwd in ctx and builds the real editor, sync, and hub commit.
// The real sync runs fabriccli.RunCLIIn with "sync" in that same cwd, rather than re-deriving it from process state.
// When printOnly is true the command is read-only: it prints on-disk YAML without opening an editor or running sync.
// setFlags carries the raw "key=value" strings collected from repeated --set flags.
func runConfig(ctx context.Context, in io.Reader, out io.Writer, args []string, printOnly bool, setFlags []string, layer string) int {
	l, realSync, realCommit, err := resolveReal(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}

	return dispatch(l, out, args, configengine.DefaultEditor, realSync, realCommit, printOnly, setFlags, layer, realHubWriteGuard(l), stdinIsTerminal(in))
}

// realHubWriteGuard returns the guard over l and the calling session's strand name from the environment.
func realHubWriteGuard(l *lymcwd.Location) func() (string, error) {
	return func() (string, error) {
		return hubWriteRefusal(l, os.Getenv(agentname.StrandNameEnv))
	}
}

// runMenu runs the lym config menu subcommand, which picks a module to edit interactively.
//
// It resolves the layout from the cwd in ctx and builds the real sync and hub commit.
// It then runs the picker reading from in, with the real editor.
func runMenu(ctx context.Context, in io.Reader, out io.Writer) int {
	l, realSync, realCommit, err := resolveReal(ctx)
	if err != nil {
		return output.Err(out, err.Error())
	}
	dirs := dirsOf(l)
	dirs.hubWriteGuard = realHubWriteGuard(l)
	return menu(dirs, in, out, configengine.DefaultEditor, realSync, realCommit, stdinIsTerminal(in))
}

// resolveReal resolves the layout from the cwd in ctx and builds the real sync function and hub commit.
//
// The sync is fabriccli.RunCLIIn with "sync" in that resolved cwd.
// The nested call must not re-derive the cwd from process state.
// The hub commit is newHubCommit over the Bolt checkout.
func resolveReal(ctx context.Context) (*lymcwd.Location, syncFunc, hubCommitFunc, error) {
	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	l, err := lymcwd.Resolve(cwd)
	if err != nil {
		return nil, nil, nil, err
	}
	realSync := func(w io.Writer) int {
		return fabriccli.RunCLIIn(cwd, w, []string{"sync"})
	}
	return l, realSync, newHubCommit(fabricengine.BoltDir(l.HubPath)), nil
}
