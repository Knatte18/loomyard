//go:build tier1

// refusal_integration_test.go proves the orch refusals end to end over one hub: the prime-or-beam rule (`status`, `resume-context`, `rules` and `BeamUp` from a task worktree that is no beam or the prime's records sibling) and the config refusal (a non-bypass permission_mode makes `status` and the watcher start fail with the fix named), each landing before any .lyx/orch is created.

package orchcli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubreconcile"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// TestOrchIntegration_Refusals runs both refusal families as steps over one hub.
// The prime-or-beam step goes first and leaves the prime's config untouched; the permission-mode step then writes orch.yaml into the prime and relies on no earlier state.
// It calls t.Parallel as a whole and no step does, since the steps share the one hub.
func TestOrchIntegration_Refusals(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	hubforge.AddPair(t, h, "orch-task")
	anchor := h.Location.AnchorPath()

	if !t.Run("prime-or-beam", func(t *testing.T) {
		cases := map[string]string{
			"task worktree": h.PairCodeWorktree("orch-task"),
			"records prime": h.PrimeRecords(),
		}
		for name, cwd := range cases {
			for _, verb := range []string{"status", resumeContextVerb, "rules"} {
				t.Run(name+"/"+verb, func(t *testing.T) {
					refusalCase(t, cwd, verb)
				})
			}
			t.Run(name+"/BeamUp", func(t *testing.T) {
				location, err := lyxcwd.Resolve(cwd)
				if err != nil {
					t.Fatalf("resolve %s: %v", cwd, err)
				}
				_, err = BeamUp(location, BeamUpOptions{Prepare: true})
				if err == nil || !strings.Contains(err.Error(), "lyx orch runs from the hub's prime worktree or a beam") {
					t.Errorf("BeamUp from %s = %v; want the prime-or-beam refusal", cwd, err)
				}
				if _, err := os.Stat(filepath.Join(cwd, lyxdirs.DotLyxDirName, orchDirName)); err == nil {
					t.Errorf(".lyx/orch was created under %s; the refusal must land first", cwd)
				}
			})
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

	// No orch verb reconciles the hub's config: `lyx beam start` does, before it calls BeamUp.
	t.Run("no verb reconciles the hub config", func(t *testing.T) {
		stampPath := hubreconcile.Geometry{BoardDir: h.BoardDir()}.StampPath()
		if err := os.Remove(stampPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove stamp: %v", err)
		}

		var out bytes.Buffer
		RunCLIIn(anchor, &out, []string{"status"})

		if _, err := os.Stat(stampPath); err == nil {
			t.Errorf("status wrote the build stamp; no orch verb reconciles")
		}
	})
}

// refusalCase runs verb from cwd and asserts the prime-or-beam refusal both a task worktree and the prime's records sibling meet.
// The beam verbs the task worktree's refusal names are asserted by TestRefuseNonPrime, since the records sibling's refusal names none.
func refusalCase(t *testing.T, cwd, verb string) {
	t.Helper()
	var out bytes.Buffer
	code := RunCLIIn(cwd, &out, []string{verb})

	if code != 1 {
		t.Fatalf("exit code = %d; want 1; output: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "lyx orch runs from the hub's prime worktree or a beam") {
		t.Errorf("refusal = %q; want the prime-or-beam wording", out.String())
	}
	if _, err := os.Stat(filepath.Join(cwd, lyxdirs.DotLyxDirName, orchDirName)); err == nil {
		t.Errorf(".lyx/orch was created under %s; the refusal must land first", cwd)
	}
}
