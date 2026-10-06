// edit_test.go — unit tests for the interactive config editing machinery (edit.go).
//
// Tests cover: scaffold-when-missing, edit of existing file, re-edit loop on validation failure,
// abort on unchanged-after-failure (both scaffolded and pre-existing), abort on editor error, and
// not-initialized propagation.

package configengine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// editStep is one editor invocation: it writes content, fails with err, or leaves the file as it found it.
type editStep struct {
	write string
	err   error
	leave bool
}

// TestEdit pins the editing loop: a missing file is scaffolded from the template before the editor runs, invalid YAML re-opens the editor, an editor that leaves invalid YAML unchanged or fails aborts with ErrAborted and removes only a file Edit scaffolded, and a base directory without _lyx/ is refused with the not-initialized error before any editor runs.
func TestEdit(t *testing.T) {
	t.Parallel()
	const template = "key1: value1\nkey2: value2\n"
	const original = "original: value\n"
	const invalid = "invalid: {\n"
	editorFailure := errors.New("simulated editor failure")

	tests := []struct {
		name  string
		state baseState
		steps []editStep
		// wantSaw is what the first editor invocation read from the file; empty skips the check.
		wantSaw     string
		wantAborted bool
		wantErr     string
		// wantFile is the file's final content; nil means the file must not exist.
		wantFile *string
	}{
		{
			name:     "missing file is scaffolded from the template before the editor runs",
			state:    baseLyx,
			steps:    []editStep{{leave: true}},
			wantSaw:  template,
			wantFile: ptr(template),
		},
		{
			name:     "existing file is edited in place",
			state:    baseWithFile,
			steps:    []editStep{{write: "modified: true\n"}},
			wantSaw:  original,
			wantFile: ptr("modified: true\n"),
		},
		{
			name:     "invalid YAML re-opens the editor until it is valid",
			state:    baseLyx,
			steps:    []editStep{{write: invalid}, {write: "valid: true\n"}},
			wantFile: ptr("valid: true\n"),
		},
		{
			name:        "unchanged invalid YAML aborts and removes the scaffolded file",
			state:       baseLyx,
			steps:       []editStep{{write: invalid}, {leave: true}},
			wantAborted: true,
		},
		{
			name:        "unchanged invalid YAML aborts and keeps a pre-existing file",
			state:       baseWithFile,
			steps:       []editStep{{write: invalid}, {leave: true}},
			wantAborted: true,
			wantFile:    ptr(invalid),
		},
		{
			name:        "editor failure aborts, wraps the failure and removes the scaffolded file",
			state:       baseLyx,
			steps:       []editStep{{err: editorFailure}},
			wantAborted: true,
		},
		{
			name:    "base directory without _lyx is refused before the editor runs",
			state:   baseNone,
			wantErr: "not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseDir, path := newBase(t, tt.state, "testmod", original)

			var calls int
			var saw string
			editor := func(editPath string) error {
				if calls >= len(tt.steps) {
					t.Fatalf("editor invoked %d times; want at most %d", calls+1, len(tt.steps))
				}
				step := tt.steps[calls]
				if calls == 0 {
					data, err := os.ReadFile(editPath)
					if err != nil {
						t.Fatalf("editor could not read %s: %v", editPath, err)
					}
					saw = string(data)
				}
				calls++
				if step.err != nil {
					return step.err
				}
				if step.leave {
					return nil
				}
				return os.WriteFile(editPath, []byte(step.write), 0644)
			}

			err := configengine.Edit(baseDir, "testmod", template, editor)

			if aborted := errors.Is(err, configengine.ErrAborted); aborted != tt.wantAborted {
				t.Fatalf("Edit() = %v; ErrAborted = %v, want %v", err, aborted, tt.wantAborted)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Edit() = %v; want an error containing %q", err, tt.wantErr)
				}
			} else if !tt.wantAborted && err != nil {
				t.Fatalf("Edit() = %v; want nil", err)
			}
			if calls != len(tt.steps) {
				t.Errorf("editor invoked %d times; want %d", calls, len(tt.steps))
			}
			if tt.wantSaw != "" && saw != tt.wantSaw {
				t.Errorf("editor saw %q; want %q", saw, tt.wantSaw)
			}
			if len(tt.steps) > 0 && tt.steps[0].err != nil && !errors.Is(err, tt.steps[0].err) {
				t.Errorf("Edit() error does not wrap the editor error; got %v", err)
			}

			if tt.wantFile == nil {
				if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
					t.Errorf("config file still exists; want it absent (stat err = %v)", statErr)
				}
				return
			}
			assertFileUnchanged(t, path, *tt.wantFile)
		})
	}
}

// TestEditPath_ScaffoldsIntoMissingParentDir tests that EditPath on a path whose parent directory does not exist scaffolds the template there, applies the editor's write and returns nil.
//
//testtiming:keep pins that EditPath creates a missing parent directory chain, which the covering Edit tests never exercise
func TestEditPath_ScaffoldsIntoMissingParentDir(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing", "parent", "staged.yaml")
	template := "key1: value1\n"

	fakeEditor := func(editPath string) error {
		return os.WriteFile(editPath, []byte("key1: edited\n"), 0o644)
	}

	if err := configengine.EditPath(path, template, fakeEditor); err != nil {
		t.Fatalf("EditPath() = %v; want nil", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read staged file: %v", err)
	}
	if string(got) != "key1: edited\n" {
		t.Errorf("staged file = %q; want the editor's write", got)
	}
}

// ptr returns a pointer to s, for a table field where nil means "absent".
func ptr(s string) *string { return &s }
