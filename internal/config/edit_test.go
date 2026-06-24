// edit_test.go — unit tests for the interactive config editing machinery (edit.go).
//
// Tests cover: scaffold-when-missing, edit of existing file, re-edit loop on
// validation failure, abort on unchanged-after-failure (both scaffolded and
// pre-existing), abort on editor error, and not-initialized propagation.

package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/config"
)

// TestEdit_ScaffoldWhenMissing tests that Edit writes the template to
// _lyx/config/<module>.yaml before the editor runs, and that the fake editor
// sees the template bytes.
func TestEdit_ScaffoldWhenMissing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/ directory (the file itself will be scaffolded).
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}

	template := "key1: value1\nkey2: value2\n"
	editorSawBytes := []byte(nil)

	// Fake editor that records what it saw.
	fakeEditor := func(path string) error {
		var err error
		editorSawBytes, err = os.ReadFile(path)
		return err
	}

	err := config.Edit(tmpDir, "testmod", template, fakeEditor)
	if err != nil {
		t.Fatalf("Edit() = %v; want nil", err)
	}

	// Verify the file was scaffolded and editor saw the template.
	if string(editorSawBytes) != template {
		t.Errorf("editor saw %q; want %q", string(editorSawBytes), template)
	}

	// Verify the file exists in the right place.
	expectedPath := filepath.Join(tmpDir, "_lyx", "config", "testmod.yaml")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("config file not found at %s: %v", expectedPath, err)
	}
}

// TestEdit_EditExistingFile tests that Edit opens an existing file in the editor,
// and the editor can rewrite it with valid YAML. Edit returns nil and the file
// holds the new bytes.
func TestEdit_EditExistingFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/ and _lyx/config/ with a pre-existing config file.
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	existingPath := filepath.Join(configDir, "testmod.yaml")
	originalContent := "original: value\n"
	if err := os.WriteFile(existingPath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Fake editor that rewrites the file with new valid YAML.
	fakeEditor := func(path string) error {
		newContent := "modified: true\n"
		return os.WriteFile(path, []byte(newContent), 0644)
	}

	err := config.Edit(tmpDir, "testmod", "unused-template", fakeEditor)
	if err != nil {
		t.Fatalf("Edit() = %v; want nil", err)
	}

	// Verify the file was updated.
	finalBytes, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}
	if string(finalBytes) != "modified: true\n" {
		t.Errorf("file content = %q; want %q", string(finalBytes), "modified: true\n")
	}
}

// TestEdit_ReEditLoop tests that when the fake editor writes invalid YAML on
// the first pass and valid YAML on the second pass, Edit loops and invokes the
// editor twice, returning nil on success.
func TestEdit_ReEditLoop(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/ directory.
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}

	template := "key: value\n"
	editorCallCount := 0

	// Fake editor that writes invalid YAML on first call, valid on second.
	fakeEditor := func(path string) error {
		editorCallCount++
		if editorCallCount == 1 {
			// First call: write invalid YAML.
			return os.WriteFile(path, []byte("invalid: {\n"), 0644)
		}
		// Second call: write valid YAML.
		return os.WriteFile(path, []byte("valid: true\n"), 0644)
	}

	err := config.Edit(tmpDir, "testmod", template, fakeEditor)
	if err != nil {
		t.Fatalf("Edit() = %v; want nil", err)
	}

	if editorCallCount != 2 {
		t.Errorf("editor called %d times; want 2", editorCallCount)
	}
}

// TestEdit_AbortOnUnchangedAfterFailure_Scaffolded tests that when the fake
// editor writes invalid YAML and leaves it unchanged, Edit returns ErrAborted
// and removes the scaffolded file (restoring the pre-call filesystem state).
func TestEdit_AbortOnUnchangedAfterFailure_Scaffolded(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/ directory.
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}

	template := "key: value\n"

	// Fake editor that writes invalid YAML but does not change it on re-open.
	fakeEditor := func(path string) error {
		// Overwrite with invalid YAML; leave it unchanged on subsequent calls.
		current, _ := os.ReadFile(path)
		if string(current) != "invalid: {\n" {
			// First call: write invalid YAML.
			return os.WriteFile(path, []byte("invalid: {\n"), 0644)
		}
		// Leave it unchanged (do nothing).
		return nil
	}

	err := config.Edit(tmpDir, "testmod", template, fakeEditor)
	if !errors.Is(err, config.ErrAborted) {
		t.Fatalf("Edit() = %v; want ErrAborted", err)
	}

	// Verify the scaffolded file was removed.
	configPath := filepath.Join(tmpDir, "_lyx", "config", "testmod.yaml")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Errorf("config file still exists after abort; should have been removed")
	}
}

// TestEdit_AbortOnUnchangedAfterFailure_PreExisting tests that when the file
// pre-existed and the editor writes invalid YAML then leaves it unchanged,
// Edit returns ErrAborted but leaves the file in place.
func TestEdit_AbortOnUnchangedAfterFailure_PreExisting(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/ and _lyx/config/ with a pre-existing config file.
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := filepath.Join(lyxDir, "config")
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	existingPath := filepath.Join(configDir, "testmod.yaml")
	originalContent := "original: value\n"
	if err := os.WriteFile(existingPath, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	// Fake editor that writes invalid YAML but leaves it unchanged on re-open.
	fakeEditor := func(path string) error {
		current, _ := os.ReadFile(path)
		if string(current) == originalContent || string(current) == "invalid: {\n" {
			if string(current) != "invalid: {\n" {
				// First call: overwrite with invalid YAML.
				return os.WriteFile(path, []byte("invalid: {\n"), 0644)
			}
			// Leave it unchanged.
			return nil
		}
		return nil
	}

	err := config.Edit(tmpDir, "testmod", "unused-template", fakeEditor)
	if !errors.Is(err, config.ErrAborted) {
		t.Fatalf("Edit() = %v; want ErrAborted", err)
	}

	// Verify the pre-existing file still exists (was not deleted).
	if _, err := os.Stat(existingPath); err != nil {
		t.Errorf("config file was deleted after abort; should remain: %v", err)
	}
}

// TestEdit_AbortOnEditorError tests that when the fake editor returns an error,
// Edit returns ErrAborted (wrapping the editor error). If the file was scaffolded,
// it is removed.
func TestEdit_AbortOnEditorError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create _lyx/ directory.
	lyxDir := filepath.Join(tmpDir, "_lyx")
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}

	template := "key: value\n"
	editorErr := errors.New("simulated editor failure")

	// Fake editor that always fails.
	fakeEditor := func(path string) error {
		return editorErr
	}

	err := config.Edit(tmpDir, "testmod", template, fakeEditor)
	if !errors.Is(err, config.ErrAborted) {
		t.Fatalf("Edit() = %v; want ErrAborted", err)
	}

	// Verify that the editor error is wrapped in the result.
	if !errors.Is(err, editorErr) {
		t.Errorf("Edit() error does not contain editor error; got %v", err)
	}

	// Verify the scaffolded file was removed.
	configPath := filepath.Join(tmpDir, "_lyx", "config", "testmod.yaml")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Errorf("config file still exists after abort; should have been removed")
	}
}

// TestEdit_NotInitialized tests that calling Edit with a baseDir lacking _lyx/
// returns the FindBaseDir not-initialized error (not ErrAborted).
func TestEdit_NotInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	// Do NOT create _lyx/ directory.

	template := "key: value\n"

	// Fake editor that should never be called.
	fakeEditor := func(path string) error {
		t.Fatal("editor should not be called for uninitialized dir")
		return nil
	}

	err := config.Edit(tmpDir, "testmod", template, fakeEditor)
	if err == nil {
		t.Fatalf("Edit() = nil; want error")
	}

	// Verify the error is about not being initialized, not ErrAborted.
	if errors.Is(err, config.ErrAborted) {
		t.Errorf("Edit() returned ErrAborted; want FindBaseDir not-initialized error")
	}

	if !stringContains(err.Error(), "not initialized") {
		t.Errorf("Edit() error = %v; want error containing 'not initialized'", err)
	}
}
