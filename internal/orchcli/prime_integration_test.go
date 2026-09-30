//go:build integration

// prime_integration_test.go proves the prime-only rule end to end: `status` run from a task
// worktree and from the prime's weft sibling refuses on the envelope and creates no .lyx/orch.

package orchcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestOrchIntegration_PrimeOnlyRefusal drives status from each non-prime vantage point.
func TestOrchIntegration_PrimeOnlyRefusal(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "orch-task")

	cases := map[string]string{
		"task worktree": h.PairWarpWorktree("orch-task"),
		"weft prime":    h.PrimeWeft(),
	}
	for name, cwd := range cases {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			code := RunCLIIn(cwd, &out, []string{"status"})

			if code != 1 {
				t.Fatalf("exit code = %d; want 1; output: %s", code, out.String())
			}
			if !strings.Contains(out.String(), "prime worktree only") {
				t.Errorf("refusal = %q; want the prime-only wording", out.String())
			}
			if _, err := os.Stat(filepath.Join(cwd, lyxdirs.DotLyxDirName, orchDirName)); err == nil {
				t.Errorf(".lyx/orch was created under %s; the refusal must land first", cwd)
			}
		})
	}
	if _, err := os.Stat(filepath.Join(h.Location.AnchorPath(), lyxdirs.DotLyxDirName, orchDirName)); err == nil {
		t.Error(".lyx/orch was created under the prime; the refusal must land first")
	}
}
