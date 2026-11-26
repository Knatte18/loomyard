// newbinary_test.go covers the linked probe program: each link yields a binary never seen before.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestLinkProbeProgram_EachLinkDiffers(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()
	sourceDir := t.TempDir()
	for name, content := range map[string]string{"main.go": probeProgramSource, "go.mod": probeProgramModule} {
		if err := os.WriteFile(filepath.Join(sourceDir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	tempDir, binaryDir := t.TempDir(), t.TempDir()

	var contents [2][]byte
	for i := range contents {
		binary, err := linkProbeProgram(t.Context(), tempDir, sourceDir, binaryDir)
		if err != nil {
			t.Fatalf("linkProbeProgram() error: %v", err)
		}
		contents[i], err = os.ReadFile(binary)
		if err != nil {
			t.Fatalf("read %s: %v", binary, err)
		}
	}
	if bytes.Equal(contents[0], contents[1]) {
		t.Error("linkProbeProgram() linked two identical binaries; want each its own build ID")
	}
}
