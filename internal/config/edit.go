// edit.go — interactive config editing with scaffold, validate, and abort contract.
//
// Provides the Edit function to load a config file, open it in an injected editor,
// validate the YAML syntax, and loop on validation failure. Scaffolds missing files
// from a template and removes them on abort to leave the filesystem in its pre-call
// state. Injected EditorFunc allows tests to drive the editor without a real process.

package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// EditorFunc opens an editor on the given path.
// It signals editor failure or user abort by returning a non-nil error.
type EditorFunc func(path string) error

// ErrAborted is the sentinel error returned when an edit is aborted.
// On abort, Edit leaves the filesystem in its pre-call state (removes scaffolded
// files, leaves pre-existing files unchanged).
var ErrAborted = errors.New("config edit aborted")

// DefaultEditor resolves the editor from $VISUAL, then $EDITOR, falling back to
// notepad on Windows and vi elsewhere. It runs the editor via os/exec with
// Stdin/Stdout/Stderr wired to the process std streams.
// Returns a non-nil error if the editor exits non-zero.
func DefaultEditor(path string) error {
	// Resolve editor command from environment or fallback.
	var editorCmd string
	if visual := os.Getenv("VISUAL"); visual != "" {
		editorCmd = visual
	} else if editor := os.Getenv("EDITOR"); editor != "" {
		editorCmd = editor
	} else if runtime.GOOS == "windows" {
		editorCmd = "notepad"
	} else {
		editorCmd = "vi"
	}

	// Run the editor with the path as an argument.
	cmd := exec.Command(editorCmd, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// Edit opens a config file in an editor, validates the YAML syntax, and loops
// on validation failure.
//
// Flow:
// 1. Call FindBaseDir(baseDir) to ensure initialization; propagate error if not.
// 2. Compute path = filepath.Join(baseDir, "_lyx", "config", module+".yaml").
// 3. If path does not exist, write template to it (scaffold; 0o644) and track
//    that this call created the file (scaffolded := true).
// 4. Loop:
//    a. Record the file bytes.
//    b. Call edit(path); if it returns an error, abort.
//    c. Re-read the bytes and yaml.Unmarshal into map[string]any to validate.
//    d. On parse success, return nil.
//    e. On parse failure, if bytes unchanged from pre-edit snapshot, abort
//       (operator saved without fixing); otherwise print the parse error to
//       os.Stderr and loop to re-open the editor.
// 5. Abort means: if scaffolded, os.Remove the file so the filesystem returns to
//    its pre-call state; then return ErrAborted (wrapping the editor error when
//    applicable). When the file pre-existed, abort leaves it as-is.
//
// Validation is syntactic only (the file must parse as YAML); known keys are
// not enforced.
func Edit(baseDir, module, template string, edit EditorFunc) error {
	// Check that baseDir is initialized.
	_, err := FindBaseDir(baseDir)
	if err != nil {
		return err
	}

	// Compute the config file path.
	path := filepath.Join(baseDir, "_lyx", "config", module+".yaml")

	// Check if the file already exists.
	_, err = os.Stat(path)
	scaffolded := os.IsNotExist(err)

	// If the file does not exist, scaffold it from the template.
	if scaffolded {
		// Create _lyx/config/ directory if needed.
		configDir := filepath.Join(baseDir, "_lyx", "config")
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return fmt.Errorf("create config directory: %w", err)
		}

		// Write the template to the new file.
		if err := os.WriteFile(path, []byte(template), 0o644); err != nil {
			return fmt.Errorf("scaffold config file: %w", err)
		}
	}

	// Loop until valid YAML is saved or edit is aborted.
	for {
		// Record the current file bytes before editing.
		preEditBytes, err := os.ReadFile(path)
		if err != nil {
			// If we just scaffolded and now can't read it, something is very wrong.
			if scaffolded {
				_ = os.Remove(path)
			}
			return fmt.Errorf("read config file: %w", err)
		}

		// Call the editor.
		if err := edit(path); err != nil {
			// Editor failed or user aborted.
			if scaffolded {
				_ = os.Remove(path)
			}
			return fmt.Errorf("%w: %w", ErrAborted, err)
		}

		// Re-read the file bytes after editing.
		postEditBytes, err := os.ReadFile(path)
		if err != nil {
			// If we just scaffolded, clean up before returning.
			if scaffolded {
				_ = os.Remove(path)
			}
			return fmt.Errorf("read config file after edit: %w", err)
		}

		// Validate YAML syntax.
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

		// Validation succeeded; exit the loop.
		return nil
	}
}
