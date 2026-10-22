// edit.go — interactive config editing with scaffold, validate, and abort contract.
//
// Provides the Edit function, and EditPath for a file at a told path, to load a config file, open it in an injected editor, validate the
// YAML syntax, and loop on validation failure.
// Scaffolds a missing hub base with the sparse header, and any other missing base from the template, and removes a scaffolded file on abort to leave the filesystem in its
// pre-call state.
// An overlay module's base scaffold holds empty per-name maps, and a hub worktree save holding the overlay selector with a value other than the base's is refused.
// Injected EditorFunc allows tests to drive the editor without a real process.

package configengine

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"gopkg.in/yaml.v3"
)

// EditorFunc opens an editor on the given path.
// It signals editor failure or user abort by returning a non-nil error.
type EditorFunc func(path string) error

// ErrAborted is the sentinel error returned when an edit is aborted.
// On abort, Edit removes a scaffolded file and leaves a pre-existing file with what the editor left.
// On an empty or whitespace-only result a pre-existing file is put back to its previous bytes instead.
var ErrAborted = errors.New("config edit aborted")

// resolveEditor picks the editor command as a binary plus the arguments that precede the path.
// The order is $VISUAL, then $EDITOR, then `code --wait` when code is on PATH, on every OS.
// With none of those, Windows gets notepad, and elsewhere nano when it is on PATH, else vi.
// An environment value is taken as one binary name.
func resolveEditor(getenv func(string) string, lookPath func(string) (string, error), goos string) (string, []string) {
	if visual := getenv("VISUAL"); visual != "" {
		return visual, nil
	}
	if editor := getenv("EDITOR"); editor != "" {
		return editor, nil
	}
	if _, err := lookPath("code"); err == nil {
		return "code", []string{"--wait"}
	}
	if goos == "windows" {
		return "notepad", nil
	}
	if _, err := lookPath("nano"); err == nil {
		return "nano", nil
	}
	return "vi", nil
}

// DefaultEditor resolves the editor from $VISUAL, then $EDITOR, then `code --wait` when code is on
// PATH, then notepad on Windows, and elsewhere nano when it is on PATH, else vi.
// It runs the editor via os/exec with Stdin/Stdout/Stderr wired to the process std streams.
// Returns a non-nil error if the editor exits non-zero.
func DefaultEditor(path string) error {
	editorCmd, editorArgs := resolveEditor(os.Getenv, exec.LookPath, runtime.GOOS)

	// Run the editor with the path as the last argument.
	cmd := exec.Command(editorCmd, append(editorArgs, path)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	logger.Info("configengine: spawning editor", "editor", editorCmd, "path", path)
	if err := cmd.Run(); err != nil {
		logger.Warn("configengine: editor spawn failed", "editor", editorCmd, "path", path, "cause", err)
		return err
	}
	logger.Info("configengine: editor exited", "editor", editorCmd, "path", path)
	return nil
}

// scaffoldIfMissing writes template to path (creating configDir first) when
// path does not yet exist, and reports whether it did so.
//
// It is shared by Edit and Set so both entry points scaffold a missing config
// file identically: a caller that scaffolds must know so it can remove the
// fresh file on any later abort, restoring the pre-call filesystem state.
func scaffoldIfMissing(path, configDir, template string) (scaffolded bool, err error) {
	// Check if the file already exists.
	_, statErr := os.Stat(path)
	scaffolded = os.IsNotExist(statErr)
	if !scaffolded {
		return false, nil
	}

	// Create _lyx/config/ directory if needed.
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return false, fmt.Errorf("create config directory: %w", err)
	}

	// Write the template to the new file.
	if err := os.WriteFile(path, []byte(template), 0o644); err != nil {
		return false, fmt.Errorf("scaffold config file: %w", err)
	}

	return true, nil
}

// BaseScaffold returns the bytes an absent base layer is scaffolded with:
// [SparseBaseHeader] when the base is sparse over the template.
// For a spec with an overlay selector it is the template with every per-name map emptied, else the spec's template.
// An overlay module's scaffold holds no per-name entry, so no entry of the base template pins itself over the layer the base picks.
func BaseScaffold(dirs Dirs, spec Spec) string {
	if sparseBase(dirs, spec) {
		return SparseBaseHeader
	}
	if spec.Overlay.Selector != "" {
		return scaffoldWithoutPerNameEntries(spec)
	}
	return string(spec.Template)
}

// scaffoldWithoutPerNameEntries returns the spec's template with every per-name map emptied to {}, re-marshalled.
// A template that fails to parse panics: an embedded template every loader parses cannot fail, so a failure is a wiring defect.
func scaffoldWithoutPerNameEntries(spec Spec) string {
	var template yaml.Node
	if err := yaml.Unmarshal(spec.Template, &template); err != nil {
		panic(fmt.Sprintf("%s config template does not parse: %v", spec.Name, err))
	}
	for _, path := range spec.PerNameMaps {
		node := nodeAt(&template, path)
		if node == nil || node.Kind != yaml.MappingNode {
			continue
		}
		node.Content = nil
		node.Style = yaml.FlowStyle
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&template); err != nil {
		panic(fmt.Sprintf("%s config template does not marshal: %v", spec.Name, err))
	}
	if err := encoder.Close(); err != nil {
		panic(fmt.Sprintf("%s config template does not marshal: %v", spec.Name, err))
	}
	return out.String()
}

// hubWorktreeGuardsSelector reports whether layer is the worktree layer of a hub for a spec with an overlay selector:
// the one layer above the base a selector value must not differ in.
func hubWorktreeGuardsSelector(dirs Dirs, spec Spec, layer Layer) bool {
	return spec.Overlay.Selector != "" && layer == LayerWorktree && BaseLayer(dirs, spec) == LayerDefault
}

// worktreeSelectorRefusal returns the refusal of a hub worktree document holding the selector with a value other than the default file's, or nil when it holds none.
// heldBefore says the file held the selector before the write or edit being judged.
func worktreeSelectorRefusal(dirs Dirs, spec Spec, path string, document []byte, heldBefore bool) error {
	defaultPath, _ := LayerFile(dirs, spec, LayerDefault)
	defaultBytes, err := os.ReadFile(defaultPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read config file %s: %w", defaultPath, err)
	}
	other, err := worktreeSelectorOther(spec, document, defaultBytes)
	if err != nil {
		return err
	}
	if other {
		return selectorRefusal(dirs, spec, path, heldBefore)
	}
	return nil
}

// Edit opens the layer's file of spec under dirs in an editor, validates the YAML syntax, and loops on validation
// failure.
// An absent base layer is scaffolded through [BaseScaffold], the header for a hub's sparse base and the template for a standalone or seed-only one; a layer above it is sparse, so an absent file is scaffolded empty.
// On abort, removes any scaffolded file and returns ErrAborted.
// Validation is syntactic only;
// known keys are not enforced, except that a saved local override must pass [ValidateLocal] and a saved machine override [ValidateMachine],
// and a saved hub worktree file may not hold a spec's overlay selector with a value other than the default file's.
// A refused save is put back to its previous bytes, or removed when the call created it.
func Edit(dirs Dirs, spec Spec, layer Layer, edit EditorFunc) error {
	path, owner, err := layerPath(dirs, spec, layer)
	if err != nil {
		return err
	}
	if _, err := FindBaseDir(owner); err != nil {
		return err
	}

	if layer == BaseLayer(dirs, spec) {
		return EditPath(path, BaseScaffold(dirs, spec), edit)
	}
	selectorGuarded := hubWorktreeGuardsSelector(dirs, spec, layer)
	if layer != LayerLocal && layer != LayerMachine && !selectorGuarded {
		return EditPath(path, "", edit)
	}

	previous, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("read config file: %w", readErr)
	}
	if err := EditPath(path, "", edit); err != nil {
		return err
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file after edit: %w", err)
	}
	refusal := validateOverride(spec, layer, saved)
	if refusal == nil && selectorGuarded {
		_, previousHeld, _ := selectorValue(spec, previous)
		refusal = worktreeSelectorRefusal(dirs, spec, path, saved, previousHeld)
	}
	if refusal != nil {
		if readErr != nil {
			_ = os.Remove(path)
		} else {
			_ = os.WriteFile(path, previous, 0o644)
		}
		return refusal
	}
	return nil
}

// EditPath is Edit's body for a file at a told path, with no _lyx/ check:
// it scaffolds path from template when absent (creating its parent directory), runs the editor,
// and loops on a YAML parse error.
// An unchanged unparseable save returns ErrAborted, and any abort removes a file EditPath scaffolded.
// An empty or whitespace-only result is an abort too, checked before the YAML parse: a pre-existing file is written back to the bytes it held on entry.
// Validation is syntactic only.
func EditPath(path, template string, edit EditorFunc) error {
	scaffolded, err := scaffoldIfMissing(path, filepath.Dir(path), template)
	if err != nil {
		return err
	}

	// The bytes a pre-existing file held on entry: an empty result restores these, never a later pass's.
	entryBytes, err := os.ReadFile(path)
	if err != nil {
		if scaffolded {
			_ = os.Remove(path)
		}
		return fmt.Errorf("read config file: %w", err)
	}

	// Loop until valid YAML is saved or edit is aborted.
	for {
		preEditBytes, err := os.ReadFile(path)
		if err != nil {
			// If we just scaffolded and now can't read it, something is very wrong.
			if scaffolded {
				_ = os.Remove(path)
			}
			return fmt.Errorf("read config file: %w", err)
		}

		if err := edit(path); err != nil {
			// Editor failed or user aborted.
			if scaffolded {
				_ = os.Remove(path)
			}
			return fmt.Errorf("%w: %w", ErrAborted, err)
		}

		postEditBytes, err := os.ReadFile(path)
		if err != nil {
			// If we just scaffolded, clean up before returning.
			if scaffolded {
				_ = os.Remove(path)
			}
			return fmt.Errorf("read config file after edit: %w", err)
		}

		// An empty or whitespace-only result is never saved over a layer; a comment-only one is not empty.
		if strings.TrimSpace(string(postEditBytes)) == "" {
			if scaffolded {
				_ = os.Remove(path)
			} else {
				_ = os.WriteFile(path, entryBytes, 0o644)
			}
			return ErrAborted
		}

		var config map[string]any
		if err := yaml.Unmarshal(postEditBytes, &config); err != nil {
			// Parse failed. Check if the user left the bytes unchanged.
			if string(preEditBytes) == string(postEditBytes) {
				// User saved without fixing; abort.
				if scaffolded {
					_ = os.Remove(path)
				}
				return ErrAborted
			}

			// Bytes changed; print the error and loop to re-edit.
			fmt.Fprintf(os.Stderr, "config parse error: %v\n", err)
			continue
		}

		return nil
	}
}
