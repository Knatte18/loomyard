// claudeimports_test.go pins Expand's import rule: what expands, what stays as written and what it reports.

package claudeimports_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Knatte18/loomyard/internal/claudeimports"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestExpand does not call t.Parallel: its `~/` row sets the home directory with t.Setenv.
// t.Setenv panics under a parallel ancestor, so only the other rows run as parallel subtests.
func TestExpand(t *testing.T) {
	tiergate.Require(t, 0)

	tests := []struct {
		name string
		// files maps a slash-separated path under the temp directory to its content.
		files map[string]string
		// dirs lists directories created under the temp directory.
		dirs []string
		// entry is the file Expand reads, relative to the temp directory.
		entry string
		// homeIsTempDir points the user's home directory at the temp directory.
		homeIsTempDir bool
		wantText      string
		// wantFiles are the expected Files, relative to the temp directory.
		wantFiles []string
		// wantReadError expects Expand to fail with a read error other than fs.ErrNotExist.
		wantReadError bool
	}{
		{
			name:      "RelativeImportExpands",
			files:     map[string]string{"CLAUDE.md": "top @docs/rules.md end", "docs/rules.md": "RULES"},
			entry:     "CLAUDE.md",
			wantText:  "top RULES end",
			wantFiles: []string{"docs/rules.md"},
		},
		{
			name: "NestedImportResolvesAgainstItsOwnDirectory",
			files: map[string]string{
				"CLAUDE.md": "@sub/b.md",
				"sub/b.md":  "b @c.md",
				"sub/c.md":  "C",
				"c.md":      "WRONG",
			},
			entry:     "CLAUDE.md",
			wantText:  "b C",
			wantFiles: []string{"sub/b.md", "sub/c.md"},
		},
		{
			name:      "ImportInCodeSpanStaysAsWritten",
			files:     map[string]string{"CLAUDE.md": "use `@x.md` and ``@x.md`` here", "x.md": "X"},
			entry:     "CLAUDE.md",
			wantText:  "use `@x.md` and ``@x.md`` here",
			wantFiles: nil,
		},
		{
			name:      "ImportInFencedBlockStaysAsWritten",
			files:     map[string]string{"CLAUDE.md": "```\n@x.md\n```\n@x.md\n", "x.md": "X"},
			entry:     "CLAUDE.md",
			wantText:  "```\n@x.md\n```\nX\n",
			wantFiles: []string{"x.md"},
		},
		{
			name:      "MidWordAtSignIsNoImport",
			files:     map[string]string{"CLAUDE.md": "mail me@x.md", "x.md": "X"},
			entry:     "CLAUDE.md",
			wantText:  "mail me@x.md",
			wantFiles: nil,
		},
		{
			name:      "ImportOfMissingFileStaysAsWritten",
			files:     map[string]string{"CLAUDE.md": "see @missing.md"},
			entry:     "CLAUDE.md",
			wantText:  "see @missing.md",
			wantFiles: nil,
		},
		{
			name:      "ImportOfDirectoryStaysAsWritten",
			files:     map[string]string{"CLAUDE.md": "see @docs"},
			dirs:      []string{"docs"},
			entry:     "CLAUDE.md",
			wantText:  "see @docs",
			wantFiles: nil,
		},
		{
			name: "ChainStopsAfterTheFifthHop",
			files: map[string]string{
				"f0.md": "0 @f1.md",
				"f1.md": "1 @f2.md",
				"f2.md": "2 @f3.md",
				"f3.md": "3 @f4.md",
				"f4.md": "4 @f5.md",
				"f5.md": "5 @f6.md",
				"f6.md": "6",
			},
			entry:     "f0.md",
			wantText:  "0 1 2 3 4 5 @f6.md",
			wantFiles: []string{"f1.md", "f2.md", "f3.md", "f4.md", "f5.md"},
		},
		{
			name:      "CycleStopsAtTheRepeatedFile",
			files:     map[string]string{"a.md": "a @b.md", "b.md": "b @a.md"},
			entry:     "a.md",
			wantText:  "a b @a.md",
			wantFiles: []string{"b.md"},
		},
		{
			name:      "FileImportedTwiceIsListedOnce",
			files:     map[string]string{"a.md": "@x.md @x.md", "x.md": "X"},
			entry:     "a.md",
			wantText:  "X X",
			wantFiles: []string{"x.md"},
		},
		{
			name:          "HomeImportResolvesUnderTheHomeDirectory",
			files:         map[string]string{"CLAUDE.md": "@~/notes.md", "notes.md": "NOTES"},
			entry:         "CLAUDE.md",
			homeIsTempDir: true,
			wantText:      "NOTES",
			wantFiles:     []string{"notes.md"},
		},
		{
			// A directory at the path fails os.ReadFile for every user, root included, so the row never skips.
			name:          "UnreadableEntryReturnsAnError",
			dirs:          []string{"CLAUDE.md"},
			entry:         "CLAUDE.md",
			wantReadError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.homeIsTempDir {
				t.Setenv("HOME", dir)
				t.Setenv("USERPROFILE", dir)
			} else {
				t.Parallel()
			}
			for _, subdir := range tt.dirs {
				if err := os.MkdirAll(filepath.Join(dir, subdir), 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v", subdir, err)
				}
			}
			for name, content := range tt.files {
				path := filepath.Join(dir, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v", filepath.Dir(path), err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v", path, err)
				}
			}

			got, err := claudeimports.Expand(filepath.Join(dir, filepath.FromSlash(tt.entry)))
			if tt.wantReadError {
				if err == nil || errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("Expand(%q) error = %v; want a read error other than fs.ErrNotExist", tt.entry, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Expand(%q) error = %v; want nil", tt.entry, err)
			}
			if got.Text != tt.wantText {
				t.Errorf("Expand(%q).Text = %q; want %q", tt.entry, got.Text, tt.wantText)
			}
			var wantFiles []string
			for _, name := range tt.wantFiles {
				wantFiles = append(wantFiles, filepath.Join(dir, filepath.FromSlash(name)))
			}
			if !reflect.DeepEqual(got.Files, wantFiles) {
				t.Errorf("Expand(%q).Files = %v; want %v", tt.entry, got.Files, wantFiles)
			}
		})
	}
}
