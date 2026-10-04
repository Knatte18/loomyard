// vocabscan_test.go runs fabricVocabularyFailures over t.TempDir() fixtures,
// so the extended bare weft/warp scan is proven to fail and pass where it should independently of the real tree.

package lyxcwd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeVocabFixture writes content at rel under a fresh temp root and returns the root.
func writeVocabFixture(t *testing.T, rel, content string) string {
	t.Helper()
	root := t.TempDir()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(abs), err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", abs, err)
	}
	return root
}

func TestFabricVocabularyFailures_Fixtures(t *testing.T) {
	tests := []struct {
		name     string
		rel      string
		content  string
		wantFail bool
	}{
		{"doc naming the token fails", "docs/x.md", "# x\n\nThe weft side.\n", true},
		{"test file outside the owner set fails", "internal/foo/foo_test.go", "package foo\n\n// weft\n", true},
		{"same content at an allowlisted path passes", "docs/benchmarks/x.md", "# x\n\nThe weft side.\n", false},
		{"exempt identifier alone passes", "internal/foo/foo_test.go", "package foo\n\nvar _ = h.PairWarpWorktree\n", false},
		{"non-exempt identifier fails", "internal/foo/foo_test.go", "package foo\n\nvar FooWeftBar int\n", true},
		{"owner directory test file passes", "internal/fabricengine/x_test.go", "package fabricengine\n\nvar weftPath string\n", false},
		{"skipped root passes", "sandbox/x.md", "weft\n", false},
		{"yaml under a doc root fails", "contracts/x.yaml", "name: weft\n", true},
		{"README fails", "README.md", "weft\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeVocabFixture(t, tt.rel, tt.content)
			failures, scanned := fabricVocabularyFailures(t, root)
			if scanned != 1 {
				t.Fatalf("scanned = %d; want 1", scanned)
			}
			if gotFail := len(failures) > 0; gotFail != tt.wantFail {
				t.Errorf("failures = %v; want fail = %v", failures, tt.wantFail)
			}
			if tt.wantFail && !strings.HasPrefix(failures[0], tt.rel+": ") {
				t.Errorf("failure %q does not name the file %s", failures[0], tt.rel)
			}
		})
	}
}
