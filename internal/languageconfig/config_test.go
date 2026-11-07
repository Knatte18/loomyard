// config_test.go verifies Load's degrade and content-failure paths for language.yaml.
// It seeds its fixtures with os.MkdirAll and os.WriteFile under t.TempDir, so the tests spawn no process.

package languageconfig_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/languageconfig"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLoad_ResolvesSkills(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// seed writes files under board and anchor; both dirs are told to Load unless noBoard is set.
		seed            func(t *testing.T, board, anchor string)
		noBoard         bool
		want            []string
		wantRenameTools []string
	}{
		{
			name:    "no board dir ignores a worktree file",
			noBoard: true,
			seed: func(t *testing.T, _, anchor string) {
				writeFile(t, configengine.ConfigFile(anchor, "language"), "skills:\n  - scribe:a\n")
			},
			want:            []string{},
			wantRenameTools: []string{},
		},
		{
			name:            "board dir without _lym",
			want:            []string{},
			wantRenameTools: []string{},
		},
		{
			name: "board file holding the template",
			seed: func(t *testing.T, board, _ string) {
				writeFile(t, configengine.ConfigFile(board, "language"), languageconfig.ConfigTemplate())
			},
			want:            []string{},
			wantRenameTools: []string{},
		},
		{
			name: "board file listing skills keeps file order and repeats",
			seed: func(t *testing.T, board, _ string) {
				writeFile(t, configengine.ConfigFile(board, "language"), "skills:\n  - scribe:a\n  - scribe:b\n  - scribe:a\n")
			},
			want:            []string{"scribe:a", "scribe:b", "scribe:a"},
			wantRenameTools: []string{},
		},
		{
			name: "board file listing both keys keeps each in file order",
			seed: func(t *testing.T, board, _ string) {
				writeFile(t, configengine.ConfigFile(board, "language"), "skills:\n  - scribe:a\nrename_tools:\n  - rename b\n  - rename a\n")
			},
			want:            []string{"scribe:a"},
			wantRenameTools: []string{"rename b", "rename a"},
		},
		{
			name: "board file still setting the retired verify_tiers loads without it",
			seed: func(t *testing.T, board, _ string) {
				writeFile(t, configengine.ConfigFile(board, "language"), "skills:\n  - scribe:a\nverify_tiers:\n  - build first\n  - [not, a, string]\nrename_tools:\n  - rename b\n")
			},
			want:            []string{"scribe:a"},
			wantRenameTools: []string{"rename b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board, anchor := t.TempDir(), t.TempDir()
			if tt.seed != nil {
				tt.seed(t, board, anchor)
			}
			dirs := configengine.Dirs{Board: board, Anchor: anchor}
			if tt.noBoard {
				dirs.Board = ""
			}

			got, err := languageconfig.Load(dirs)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got.Skills == nil || !slices.Equal(got.Skills, tt.want) {
				t.Errorf("Load skills = %#v, want non-nil %q", got.Skills, tt.want)
			}
			if got.RenameTools == nil || !slices.Equal(got.RenameTools, tt.wantRenameTools) {
				t.Errorf("Load rename tools = %#v, want non-nil %q", got.RenameTools, tt.wantRenameTools)
			}
		})
	}
}

func TestLoad_InvalidFailsNamingKey(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		// seed writes files under board and anchor.
		seed func(t *testing.T, board, anchor string)
		// key is the key the error must name; empty for a failure with no single key.
		key string
		// wantUnusable is false for a failure that is not a content failure.
		wantUnusable bool
	}{
		{"empty entry", boardFile(`skills: [""]` + "\n"), "skills", true},
		{"leading whitespace", boardFile(`skills: [" scribe:a"]` + "\n"), "skills", true},
		{"trailing whitespace", boardFile(`skills: ["scribe:a "]` + "\n"), "skills", true},
		{"scalar value", boardFile("skills: scribe:a\n"), "skills", true},
		{"unparseable yaml", boardFile("skills: [\n"), "", true},
		{"rename_tools scalar value", boardFile("rename_tools: gopls\n"), "rename_tools", true},
		{"rename_tools empty entry", boardFile(`rename_tools: [""]` + "\n"), "rename_tools", true},
		{"rename_tools entry with a line break", boardFile(`rename_tools: ["a\nb"]` + "\n"), "rename_tools", true},
		{
			name: "local override holding skills",
			seed: func(t *testing.T, board, anchor string) {
				writeFile(t, configengine.ConfigFile(board, "language"), languageconfig.ConfigTemplate())
				if err := os.MkdirAll(configengine.ConfigDir(anchor), 0o755); err != nil {
					t.Fatal(err)
				}
				writeFile(t, configengine.LocalFile(anchor, "language"), "skills:\n  - scribe:a\n")
			},
			key:          "skills",
			wantUnusable: true,
		},
		{
			name: "local override holding verify_tiers",
			seed: func(t *testing.T, board, anchor string) {
				writeFile(t, configengine.ConfigFile(board, "language"), languageconfig.ConfigTemplate())
				if err := os.MkdirAll(configengine.ConfigDir(anchor), 0o755); err != nil {
					t.Fatal(err)
				}
				writeFile(t, configengine.LocalFile(anchor, "language"), "verify_tiers:\n  - build\n")
			},
			key:          "verify_tiers",
			wantUnusable: true,
		},
		{
			name: "local override holding rename_tools",
			seed: func(t *testing.T, board, anchor string) {
				writeFile(t, configengine.ConfigFile(board, "language"), languageconfig.ConfigTemplate())
				if err := os.MkdirAll(configengine.ConfigDir(anchor), 0o755); err != nil {
					t.Fatal(err)
				}
				writeFile(t, configengine.LocalFile(anchor, "language"), "rename_tools:\n  - gopls\n")
			},
			key:          "rename_tools",
			wantUnusable: true,
		},
		{
			name: "directory where the board file belongs",
			seed: func(t *testing.T, board, _ string) {
				if err := os.MkdirAll(configengine.ConfigFile(board, "language"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board, anchor := t.TempDir(), t.TempDir()
			tt.seed(t, board, anchor)

			_, err := languageconfig.Load(configengine.Dirs{Board: board, Anchor: anchor})
			if err == nil {
				t.Fatal("Load succeeded, want an error")
			}
			if got := errors.Is(err, configengine.ErrInvalid); got != tt.wantUnusable {
				t.Errorf("errors.Is(err, ErrInvalid) = %v, want %v: %v", got, tt.wantUnusable, err)
			}
			if got := errors.Is(err, languageconfig.ErrUnusable); got != tt.wantUnusable {
				t.Errorf("errors.Is(err, ErrUnusable) = %v, want %v: %v", got, tt.wantUnusable, err)
			}
			if !tt.wantUnusable {
				return
			}
			if !strings.Contains(err.Error(), `lym config language`) {
				t.Errorf("error %q lacks the way forward", err)
			}
			if tt.key != "" && !strings.Contains(err.Error(), tt.key) {
				t.Errorf("error %q lacks the key %q", err, tt.key)
			}
			if tt.key == "" {
				for _, key := range []string{"skills", "rename_tools"} {
					if strings.Contains(err.Error(), key) {
						t.Errorf("error %q names %q, want no key", err, key)
					}
				}
			}
		})
	}
}

func TestBulletListValue(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name    string
		entries []string
		want    string
	}{
		{"nil list", nil, "none"},
		{"empty list", []string{}, "none"},
		{"two entries as ordered bullets", []string{"build first", "then vet"}, "- build first\n- then vet"},
		{"entry holding template syntax is verbatim", []string{"{{.x}}"}, "- {{.x}}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := languageconfig.BulletListValue(tt.entries); got != tt.want {
				t.Errorf("BulletListValue(%q) = %q, want %q", tt.entries, got, tt.want)
			}
		})
	}
}

// boardFile returns a seed that writes content as the board's language.yaml.
func boardFile(content string) func(t *testing.T, board, anchor string) {
	return func(t *testing.T, board, _ string) {
		writeFile(t, configengine.ConfigFile(board, "language"), content)
	}
}
