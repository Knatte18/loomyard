// engineblock_test.go covers ReadSet and ReadForEngine over a stencils directory.
// The directory holds plain includes and per-engine copies of an engine block.

package stencilstore

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeNamed writes body as the stencil name under dir, creating its family directory.
func writeNamed(t *testing.T, dir, name, body string) {
	t.Helper()
	path := Path(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadSet(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir := t.TempDir()
	writeNamed(t, dir, "seat-plain", "Plain {{.x}}.\n{{template \"seat-block\"}}\n")
	writeNamed(t, dir, "seat-block", "Block.\n")
	writeNamed(t, dir, "seat-engined", "Lead.\n{{template \"seat-block\"}}\n{{template \"engine-tool-words\"}}\n")
	writeNamed(t, dir, EngineBlockName("claude", "tool-words"), "Use Edit.\n")
	writeNamed(t, dir, EngineBlockName("codex", "tool-words"), "Use apply_patch.\n")
	writeNamed(t, dir, "seat-nested", "Lead.\n{{template \"seat-worded\"}}\n")
	writeNamed(t, dir, "seat-worded", "Worded: {{template \"engine-tool-words\"}}\n")

	tests := []struct {
		name       string
		engine     string
		stencil    string
		wantBlocks map[string]string
		wantErr    string
		wantMissed *MissingEngineBlockError
	}{
		{name: "PlainIncludesReadTheSameWithNoEngine", stencil: "seat-plain", wantBlocks: map[string]string{"seat-block": "Block.\n"}},
		{name: "EngineBlockReadsTheClaudeCopy", engine: "claude", stencil: "seat-engined", wantBlocks: map[string]string{"seat-block": "Block.\n", "engine-tool-words": "Use Edit."}},
		{name: "EngineBlockReadsTheCodexCopy", engine: "codex", stencil: "seat-engined", wantBlocks: map[string]string{"seat-block": "Block.\n", "engine-tool-words": "Use apply_patch."}},
		{name: "PlainIncludeCarriesItsEngineBlocksInPlace", engine: "codex", stencil: "seat-nested", wantBlocks: map[string]string{"seat-worded": "Worded: Use apply_patch.\n"}},
		{name: "EngineWithNoCopyIsAMissingBlock", engine: "gemini", stencil: "seat-engined", wantMissed: &MissingEngineBlockError{Block: "tool-words", Engine: "gemini", Path: filepath.Join(dir, "engine", "gemini", "tool-words.md")}},
		{name: "EngineBlockWithNoEngineIsRefused", stencil: "seat-engined", wantErr: `stencilstore: stencil "seat-engined" includes engine block "tool-words", but no engine was named to read it for; way forward:`},
		{name: "UnreadableStencil", engine: "claude", stencil: "seat-absent", wantErr: `stencilstore: read stencil "seat-absent"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body, blocks, err := ReadSet(dir, tt.engine, tt.stencil)
			if tt.wantMissed != nil {
				var missed *MissingEngineBlockError
				if !errors.As(err, &missed) || !reflect.DeepEqual(missed, tt.wantMissed) {
					t.Fatalf("ReadSet(%q, %q) error = %v; want %+v", tt.engine, tt.stencil, err, tt.wantMissed)
				}
				for _, part := range []string{tt.wantMissed.Block, tt.wantMissed.Engine, tt.wantMissed.Path, "way forward"} {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("ReadSet() error = %q; want it to name %q", err, part)
					}
				}
				return
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ReadSet(%q, %q) error = %v; want one containing %q", tt.engine, tt.stencil, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadSet(%q, %q) = %v; want nil error", tt.engine, tt.stencil, err)
			}
			want, _ := os.ReadFile(Path(dir, tt.stencil))
			if string(body) != string(want) {
				t.Errorf("ReadSet() body = %q; want %q", body, want)
			}
			if got := slices.Sorted(maps.Keys(blocks)); !slices.Equal(got, slices.Sorted(maps.Keys(tt.wantBlocks))) {
				t.Fatalf("ReadSet() blocks = %v; want %v", got, slices.Sorted(maps.Keys(tt.wantBlocks)))
			}
			for name, text := range tt.wantBlocks {
				if string(blocks[name]) != text {
					t.Errorf("ReadSet() block %q = %q; want %q", name, blocks[name], text)
				}
			}
		})
	}
}

func TestReadForEngine(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	dir := t.TempDir()
	writeNamed(t, dir, EngineBlockName("claude", "tool"), "<!-- Header. -->\nthe Edit tool\n")
	writeNamed(t, dir, EngineBlockName("codex", "tool"), "<!-- Header. -->\n`apply_patch`\n")
	writeNamed(t, dir, EngineBlockName("claude", "nesting"), "{{template \"engine-tool\"}}\n")
	writeNamed(t, dir, "inline", "<!-- Pulls in {{template \"engine-absent\"}}. -->\nChange it with {{template \"engine-tool\"}}.\n")
	writeNamed(t, dir, "fragment", "<!-- Fragment header. -->\nFragment: {{template \"engine-tool\"}}.\n")
	writeNamed(t, dir, "withfragment", "Lead.\n{{template \"fragment\"}}Tail.\n")
	writeNamed(t, dir, "withmissing", "Lead.\n{{template \"absent-fragment\"}}\n")
	writeNamed(t, dir, "trimmed", "Use {{- template \"engine-tool\"}}.\n")
	writeNamed(t, dir, "nested", "Use {{template \"engine-nesting\"}}.\n")

	tests := []struct {
		name       string
		engine     string
		stencil    string
		want       string
		wantErr    string
		wantMissed bool
	}{
		{name: "ClaudeCopyStandsInsideTheLine", engine: "claude", stencil: "inline", want: "<!-- Pulls in {{template \"engine-absent\"}}. -->\nChange it with the Edit tool.\n"},
		{name: "PlainFragmentStandsInPlaceWithItsEngineBlock", engine: "claude", stencil: "withfragment", want: "Lead.\nFragment: the Edit tool.\nTail.\n"},
		{name: "PlainFragmentTakesTheLaunchEnginesBlock", engine: "codex", stencil: "withfragment", want: "Lead.\nFragment: `apply_patch`.\nTail.\n"},
		{name: "MissingPlainFragmentNamesBothStencilsAndTheWayForward", engine: "claude", stencil: "withmissing", wantErr: `stencilstore: stencil "withmissing" includes stencil "absent-fragment", which does not exist; way forward: run lym stencil diff, then lym stencil sync to seed the missing stencil`},
		{name: "CodexCopyStandsInsideTheLine", engine: "codex", stencil: "inline", want: "<!-- Pulls in {{template \"engine-absent\"}}. -->\nChange it with `apply_patch`.\n"},
		{name: "EngineWithNoCopyIsAMissingBlock", engine: "gemini", stencil: "inline", wantMissed: true},
		{name: "NoEngineIsRefused", stencil: "inline", wantErr: `but no engine was named to read it for; way forward:`},
		{name: "TrimmedActionIsRefused", engine: "claude", stencil: "trimmed", wantErr: `stencilstore: stencil "trimmed" leaves engine block "tool" unresolved; way forward:`},
		{name: "EngineBlockIncludingAnotherIsRefused", engine: "claude", stencil: "nested", wantErr: `stencilstore: stencil "nested" leaves engine block "tool" unresolved; way forward:`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ReadForEngine(dir, tt.engine, tt.stencil)
			if tt.wantMissed {
				var missed *MissingEngineBlockError
				if !errors.As(err, &missed) {
					t.Fatalf("ReadForEngine(%q, %q) error = %v; want a *MissingEngineBlockError", tt.engine, tt.stencil, err)
				}
				return
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ReadForEngine(%q, %q) error = %v; want one containing %q", tt.engine, tt.stencil, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadForEngine(%q, %q) = %v; want nil error", tt.engine, tt.stencil, err)
			}
			if string(got) != tt.want {
				t.Errorf("ReadForEngine(%q, %q) = %q; want %q", tt.engine, tt.stencil, got, tt.want)
			}
		})
	}
}
