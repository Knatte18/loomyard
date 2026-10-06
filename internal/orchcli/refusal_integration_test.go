//go:build integration

// refusal_integration_test.go proves the orch refusals end to end over one hub: the prime-only rule (`status` and `start` from a task worktree or the prime's records sibling) and the config refusal (a non-bypass permission_mode makes `status` and the watcher start fail with the fix named), each landing before any .lyx/orch is created.

package orchcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestOrchIntegration_Refusals runs both refusal families as steps over one hub.
// The prime-only step goes first and leaves the prime's config untouched; the permission-mode step then writes orch.yaml into the prime and relies on no earlier state.
// It calls t.Parallel as a whole and no step does, since the steps share the one hub.
func TestOrchIntegration_Refusals(t *testing.T) {
	t.Parallel()

	h := hubforge.NewHub(t, ".")
	hubforge.AddPair(t, h, "orch-task")
	anchor := h.Location.AnchorPath()

	if !t.Run("prime-only", func(t *testing.T) {
		cases := map[string]string{
			"task worktree": h.PairWarpWorktree("orch-task"),
			"records prime": h.PrimeWeft(),
		}
		for name, cwd := range cases {
			for _, verb := range []string{"status", "start"} {
				t.Run(name+"/"+verb, func(t *testing.T) {
					refusalCase(t, cwd, verb)
				})
			}
		}
		if _, err := os.Stat(filepath.Join(anchor, lyxdirs.DotLyxDirName, orchDirName)); err == nil {
			t.Error(".lyx/orch was created under the prime; the refusal must land first")
		}
	}) {
		return
	}

	t.Run("permission mode", func(t *testing.T) {
		if err := os.MkdirAll(configengine.ConfigDir(anchor), 0o755); err != nil {
			t.Fatalf("mkdir config dir: %v", err)
		}
		if err := os.WriteFile(configengine.ConfigFile(anchor, "orch"), []byte("permission_mode: prompt\n"), 0o644); err != nil {
			t.Fatalf("write orch.yaml: %v", err)
		}

		for _, verb := range []string{"status", "watch"} {
			t.Run(verb, func(t *testing.T) {
				var out bytes.Buffer
				code := RunCLIIn(anchor, &out, []string{verb})

				if code != 1 {
					t.Fatalf("exit code = %d; want 1; output: %s", code, out.String())
				}
				if !strings.Contains(out.String(), "set permission_mode: bypass or remove the key") {
					t.Errorf("refusal = %q; want the fix named", out.String())
				}
			})
		}
		if _, err := os.Stat(filepath.Join(anchor, lyxdirs.DotLyxDirName, orchDirName)); err == nil {
			t.Error(".lyx/orch was created under the prime; the refusal must land first")
		}
	})
}

// refusalCase runs verb from cwd and asserts the prime-only refusal.
func refusalCase(t *testing.T, cwd, verb string) {
	t.Helper()
	var out bytes.Buffer
	code := RunCLIIn(cwd, &out, []string{verb})

	if code != 1 {
		t.Fatalf("exit code = %d; want 1; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "prime worktree only") {
		t.Errorf("refusal = %q; want the prime-only wording", out.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, lyxdirs.DotLyxDirName, orchDirName)); err == nil {
		t.Errorf(".lyx/orch was created under %s; the refusal must land first", cwd)
	}
}
