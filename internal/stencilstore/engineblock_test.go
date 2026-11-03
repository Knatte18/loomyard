// engineblock_test.go covers ReadSet over a stencils directory holding plain includes and per-engine copies of an engine block.

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

// writeNamed writes body as the stencil name under dir, at the path Path gives it.
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

	tests := []struct {
		name       string
		engine     string
		stencil    string
		wantBlocks map[string]string
		wantErr    string
		wantMissed *MissingEngineBlockError
	}{
		{name: "PlainIncludesReadTheSameWithNoEngine", stencil: "seat-plain", wantBlocks: map[string]string{"seat-block": "Block.\n"}},
		{name: "EngineBlockReadsTheClaudeCopy", engine: "claude", stencil: "seat-engined", wantBlocks: map[string]string{"seat-block": "Block.\n", "engine-tool-words": "Use Edit.\n"}},
		{name: "EngineBlockReadsTheCodexCopy", engine: "codex", stencil: "seat-engined", wantBlocks: map[string]string{"seat-block": "Block.\n", "engine-tool-words": "Use apply_patch.\n"}},
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
