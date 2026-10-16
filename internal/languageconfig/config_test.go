// config_test.go verifies language.yaml's Load degrade and content-failure paths and AppendSkills' ordering, seeded via plain os.MkdirAll/os.WriteFile against a t.TempDir() so the test stays untagged and spawn-free.

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
	t.Parallel()
	tests := []struct {
		name string
		// seed writes files under board and anchor; both dirs are told to Load unless noBoard is set.
		seed      func(t *testing.T, board, anchor string)
		noBoard   bool
		want      []string
		wantTiers []string
	}{
		{
			name:    "no board dir ignores a worktree file",
			noBoard: true,
			seed: func(t *testing.T, _, anchor string) {
				writeFile(t, configengine.ConfigFile(anchor, "language"), "skills:\n  - scribe:a\n")
			},
			want:      []string{},
			wantTiers: []string{},
		},
		{
			name:      "board dir without _lyx",
			want:      []string{},
			wantTiers: []string{},
		},
		{
			name: "board file holding the template",
			seed: func(t *testing.T, board, _ string) {
				writeFile(t, configengine.ConfigFile(board, "language"), languageconfig.ConfigTemplate())
			},
			want:      []string{},
			wantTiers: []string{},
		},
		{
			name: "board file listing skills keeps file order and repeats",
			seed: func(t *testing.T, board, _ string) {
				writeFile(t, configengine.ConfigFile(board, "language"), "skills:\n  - scribe:a\n  - scribe:b\n  - scribe:a\n")
			},
			want:      []string{"scribe:a", "scribe:b", "scribe:a"},
			wantTiers: []string{},
		},
		{
			name: "board file listing both keys keeps each in file order",
			seed: func(t *testing.T, board, _ string) {
				writeFile(t, configengine.ConfigFile(board, "language"), "skills:\n  - scribe:a\nverify_tiers:\n  - build first\n  - vet second\n")
			},
			want:      []string{"scribe:a"},
			wantTiers: []string{"build first", "vet second"},
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
			if got.VerifyTiers == nil || !slices.Equal(got.VerifyTiers, tt.wantTiers) {
				t.Errorf("Load verify tiers = %#v, want non-nil %q", got.VerifyTiers, tt.wantTiers)
			}
		})
	}
}

func TestLoad_InvalidFailsNamingKey(t *testing.T) {
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
		{"verify_tiers scalar value", boardFile("verify_tiers: build\n"), "verify_tiers", true},
		{"verify_tiers non-string entry", boardFile("verify_tiers: [1]\n"), "verify_tiers", true},
		{"verify_tiers empty entry", boardFile(`verify_tiers: [""]` + "\n"), "verify_tiers", true},
		{"verify_tiers leading whitespace", boardFile(`verify_tiers: [" build"]` + "\n"), "verify_tiers", true},
		{"verify_tiers trailing whitespace", boardFile(`verify_tiers: ["build "]` + "\n"), "verify_tiers", true},
		{"verify_tiers entry holding a newline", boardFile(`verify_tiers: ["a\nb"]` + "\n"), "verify_tiers", true},
		{"verify_tiers entry holding a carriage return", boardFile(`verify_tiers: ["a\rb"]` + "\n"), "verify_tiers", true},
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
			if !strings.Contains(err.Error(), `lyx config language`) {
				t.Errorf("error %q lacks the way forward", err)
			}
			if tt.key != "" && !strings.Contains(err.Error(), tt.key) {
				t.Errorf("error %q lacks the key %q", err, tt.key)
			}
			if tt.key == "" {
				for _, key := range []string{"skills", "verify_tiers"} {
					if strings.Contains(err.Error(), key) {
						t.Errorf("error %q names %q, want no key", err, key)
					}
				}
			}
		})
	}
}

func TestVerifyTiersValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		tiers []string
		want  string
	}{
		{"nil list", nil, "none"},
		{"empty list", []string{}, "none"},
		{"two entries as ordered bullets", []string{"build first", "then vet"}, "- build first\n- then vet"},
		{"entry holding template syntax is verbatim", []string{"{{.x}}"}, "- {{.x}}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := languageconfig.VerifyTiersValue(tt.tiers); got != tt.want {
				t.Errorf("VerifyTiersValue(%q) = %q, want %q", tt.tiers, got, tt.want)
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

func TestAppendSkills(t *testing.T) {
	t.Parallel()
	own := []string{"scribe:prose", "scribe:code-quality"}
	tests := []struct {
		name     string
		language []string
		want     []string
	}{
		{"empty language list", nil, []string{"scribe:prose", "scribe:code-quality"}},
		{"language appended in order", []string{"scribe:x", "scribe:y"}, []string{"scribe:prose", "scribe:code-quality", "scribe:x", "scribe:y"}},
		{"repeat appears once at its first position", []string{"scribe:x", "scribe:y", "scribe:x"}, []string{"scribe:prose", "scribe:code-quality", "scribe:x", "scribe:y"}},
		{"own skill is skipped", []string{"scribe:code-quality", "scribe:x"}, []string{"scribe:prose", "scribe:code-quality", "scribe:x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := languageconfig.AppendSkills(own, tt.language); !slices.Equal(got, tt.want) {
				t.Errorf("AppendSkills = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("spare capacity of own is untouched", func(t *testing.T) {
		t.Parallel()
		backing := []string{"scribe:prose", "sentinel"}
		short := backing[:1]

		got := languageconfig.AppendSkills(short, []string{"scribe:x"})

		if want := []string{"scribe:prose", "scribe:x"}; !slices.Equal(got, want) {
			t.Errorf("AppendSkills = %q, want %q", got, want)
		}
		if backing[1] != "sentinel" {
			t.Errorf("own's backing array was written: %q", backing)
		}
	})
}
