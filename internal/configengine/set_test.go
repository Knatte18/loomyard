// set_test.go — unit tests for the non-interactive Set entry point (set.go).
//
// Tests cover: scaffold-then-set when the config file is missing, rollback
// of a freshly-scaffolded file on an unknown key, byte-for-byte preservation
// of a pre-existing file on an unknown key, preservation of untouched
// keys when setting one key on an existing multi-key file, and end-to-end
// reporting of Set's returned preserved-keys list for an orphaned key.

package configengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/yamlengine"
)

// TestSet_ScaffoldWhenMissingThenSet mirrors TestEdit_ScaffoldWhenMissing's
// fixture setup: calling Set against a baseDir with no existing config file
// creates it from template and applies the requested pairs in one call.
func TestSet_ScaffoldWhenMissingThenSet(t *testing.T) {
	tmpDir := t.TempDir()

	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}

	template := "key1: value1\nkey2: value2\n"
	_, err := configengine.Set(tmpDir, "testmod", template, []yamlengine.KV{{Key: "key1", Value: "set1"}})
	if err != nil {
		t.Fatalf("Set() = %v; want nil", err)
	}

	path := hubgeometry.ConfigFile(tmpDir, "testmod")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("config file not found at %s: %v", path, err)
	}
	if !strings.Contains(string(data), "key1: set1") {
		t.Errorf("Set() file = %q; want key1: set1", string(data))
	}
	if !strings.Contains(string(data), "key2: value2") {
		t.Errorf("Set() file = %q; want key2: value2 (untouched template default)", string(data))
	}
}

// TestSet_UnknownKeyRemovesScaffoldedFile verifies that an unknown key against
// a freshly-missing file removes the just-scaffolded file and returns a
// non-nil error mentioning the unknown key.
func TestSet_UnknownKeyRemovesScaffoldedFile(t *testing.T) {
	tmpDir := t.TempDir()

	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}

	template := "key1: value1\n"
	_, err := configengine.Set(tmpDir, "testmod", template, []yamlengine.KV{{Key: "bogus", Value: "x"}})
	if err == nil {
		t.Fatalf("Set() = nil; want error for unknown key")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("Set() error = %v; want it to mention the unknown key", err)
	}

	path := hubgeometry.ConfigFile(tmpDir, "testmod")
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("config file still exists after unknown-key rejection; should have been removed")
	}
}

// TestSet_UnknownKeyLeavesExistingFileUnchanged verifies that an unknown key
// against a pre-existing file leaves that file byte-for-byte unchanged and
// returns a non-nil error.
func TestSet_UnknownKeyLeavesExistingFileUnchanged(t *testing.T) {
	tmpDir := t.TempDir()

	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	path := hubgeometry.ConfigFile(tmpDir, "testmod")
	originalContent := "key1: original_value\n"
	if err := os.WriteFile(path, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	template := "key1: default1\n"
	_, err := configengine.Set(tmpDir, "testmod", template, []yamlengine.KV{{Key: "bogus", Value: "x"}})
	if err == nil {
		t.Fatalf("Set() = nil; want error for unknown key")
	}

	finalBytes, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("failed to read config file: %v", readErr)
	}
	if string(finalBytes) != originalContent {
		t.Errorf("Set() left file = %q; want unchanged %q", string(finalBytes), originalContent)
	}
}

// TestSet_PreservesOtherKeysOnExistingFile verifies that setting one key on
// an existing multi-key file preserves the other keys' values.
func TestSet_PreservesOtherKeysOnExistingFile(t *testing.T) {
	tmpDir := t.TempDir()

	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	path := hubgeometry.ConfigFile(tmpDir, "testmod")
	originalContent := "key1: original_value1\nkey2: original_value2\n"
	if err := os.WriteFile(path, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	template := "key1: default1\nkey2: default2\n"
	_, err := configengine.Set(tmpDir, "testmod", template, []yamlengine.KV{{Key: "key1", Value: "new_value1"}})
	if err != nil {
		t.Fatalf("Set() = %v; want nil", err)
	}

	finalBytes, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("failed to read config file: %v", readErr)
	}
	if !strings.Contains(string(finalBytes), "key1: new_value1") {
		t.Errorf("Set() file = %q; want key1: new_value1", string(finalBytes))
	}
	if !strings.Contains(string(finalBytes), "key2: original_value2") {
		t.Errorf("Set() file = %q; want key2: original_value2 (untouched)", string(finalBytes))
	}
}

// TestSet_PreservesUnrecognizedExistingKeyEndToEnd verifies that a real
// on-disk config file carrying a top-level key absent from the template
// survives a Set call untouched, and that Set reports the preserved key
// name in its returned []string.
func TestSet_PreservesUnrecognizedExistingKeyEndToEnd(t *testing.T) {
	tmpDir := t.TempDir()

	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
	if err := os.Mkdir(lyxDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx: %v", err)
	}
	configDir := hubgeometry.ConfigDir(tmpDir)
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("failed to create _lyx/config: %v", err)
	}

	path := hubgeometry.ConfigFile(tmpDir, "testmod")
	originalContent := "key1: original_value1\nlegacy: keepme\n"
	if err := os.WriteFile(path, []byte(originalContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	template := "key1: default1\n"
	preserved, err := configengine.Set(tmpDir, "testmod", template, []yamlengine.KV{{Key: "key1", Value: "new_value1"}})
	if err != nil {
		t.Fatalf("Set() = %v; want nil", err)
	}
	if len(preserved) != 1 || preserved[0] != "legacy" {
		t.Errorf("Set() preserved = %v; want [\"legacy\"]", preserved)
	}

	finalBytes, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("failed to read config file: %v", readErr)
	}
	if !strings.Contains(string(finalBytes), "legacy: keepme") {
		t.Errorf("Set() file = %q; want legacy: keepme preserved verbatim", string(finalBytes))
	}
}
