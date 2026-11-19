// cli_integration_test.go drives the sandbox verbs over a hub fixture: it builds a real sandbox hub through the verbs.

package hardencli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hardenshed"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestSandboxVerbs(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	hub := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	deploys := 0
	deps := cliDeps{Deploy: func(string) error { deploys++; return nil }}
	pairOf := func(t *testing.T, slug, recipe string) *lymcwd.Location {
		t.Helper()
		hubforge.AddPair(t, hub, slug)
		location, err := lymcwd.ResolveWorktree(hub.PairCodeWorktree(slug))
		if err != nil {
			t.Fatalf("ResolveWorktree(%s) = %v; want nil", hub.PairCodeWorktree(slug), err)
		}
		if recipe != "" {
			if err := shedrun.WriteSeed(location, shedrun.SelfRunID, shedrun.Seed{Recipe: recipe, Driver: shedrun.DriverGo}); err != nil {
				t.Fatalf("WriteSeed(%s) = %v; want nil", recipe, err)
			}
		}
		return location
	}
	// run executes `lym harden sandbox <verb>` in the worktree and returns the exit code and the last JSON line.
	run := func(t *testing.T, location *lymcwd.Location, verb string) (int, map[string]any) {
		t.Helper()
		var out bytes.Buffer
		code := clihelp.ExecuteIn(newCommand(deps), location.AnchorPath(), &out, []string{"sandbox", verb})
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		var answer map[string]any
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &answer); err != nil {
			t.Fatalf("`harden sandbox %s` printed %q; want a JSON answer: %v", verb, out.String(), err)
		}
		return code, answer
	}
	requireRefused := func(t *testing.T, location *lymcwd.Location, verb string) {
		t.Helper()
		code, answer := run(t, location, verb)
		if message, _ := answer["error"].(string); code == 0 || !strings.Contains(message, notHardenerWayForward) {
			t.Errorf("`harden sandbox %s` = exit %d, %v; want a refusal naming %q", verb, code, answer, notHardenerWayForward)
		}
	}

	hardener := pairOf(t, "hardened", shedrun.RecipeHardener)
	root := hardenshed.SandboxRoot(hardener.WorktreePath())
	beside := filepath.Join(filepath.Dir(root), "beside-the-root")
	if err := os.MkdirAll(beside, 0o755); err != nil {
		t.Fatal(err)
	}

	var hubPath string
	if !t.Run("new builds a wired hub under the root and prints its path and the .dev-bin lym", func(t *testing.T) {
		code, answer := run(t, hardener, "new")
		if code != 0 {
			t.Fatalf("`harden sandbox new` = exit %d, %v; want 0", code, answer)
		}
		hubPath, _ = answer["hub"].(string)
		lym, _ := answer["lym"].(string)
		if !strings.HasPrefix(hubPath, root+string(filepath.Separator)) {
			t.Errorf("hub = %q; want a path under the sandbox root %q", hubPath, root)
		}
		if want := filepath.Join(hardener.WorktreePath(), ".dev-bin"); filepath.Dir(lym) != want {
			t.Errorf("lym = %q; want a binary in %q", lym, want)
		}
		if deploys != 1 {
			t.Errorf("deploys = %d; want 1, since no .dev-bin lym was built yet", deploys)
		}
		overrides, err := filepath.Glob(filepath.Join(hubPath, "*", "_lym", "config", "loom.yaml"))
		// The hub's board directory holds a loom.yaml of its own, which is not the prime's.
		overrides = slices.DeleteFunc(overrides, func(path string) bool {
			return strings.Contains(path, string(filepath.Separator)+lymdirs.BoltDirName+string(filepath.Separator))
		})
		if err != nil || len(overrides) == 0 {
			t.Fatalf("Glob(the hub's prime loom.yaml) = %v, %v; want the committed override", overrides, err)
		}
		holdsOverride := slices.ContainsFunc(overrides, func(path string) bool {
			contents, err := os.ReadFile(path)
			var document struct {
				Friction *string `yaml:"friction"`
			}
			return err == nil && yaml.Unmarshal(contents, &document) == nil && document.Friction != nil && *document.Friction == ""
		})
		if !holdsOverride {
			t.Errorf("none of %v holds the friction key empty; want the committed override", overrides)
		}
	}) {
		return
	}

	if !t.Run("list shows the hub a second command made", func(t *testing.T) {
		code, answer := run(t, hardener, "list")
		hubs, _ := answer["hubs"].([]any)
		if code != 0 || len(hubs) != 1 || hubs[0] != hubPath {
			t.Errorf("`harden sandbox list` = exit %d, %v; want exit 0 listing %q", code, answer, hubPath)
		}
	}) {
		return
	}

	if !t.Run("clear ends a process naming the root, removes the hubs and leaves a directory beside the root", func(t *testing.T) {
		// The process scan reads /proc, so only Linux has an orphan to end.
		var orphan *exec.Cmd
		exited := make(chan struct{})
		if runtime.GOOS == "linux" {
			// The root is the script's $0, so the process names it on its command line.
			// Its working directory need not, since /proc reports the target of a link along the path.
			orphan = exec.Command("sh", "-c", "while :; do sleep 1; done", root)
			if err := orphan.Start(); err != nil {
				t.Fatalf("start the orphan under %q: %v", root, err)
			}
			t.Cleanup(func() { _ = orphan.Process.Kill() })
			go func() {
				_ = orphan.Wait()
				close(exited)
			}()
		}
		code, answer := run(t, hardener, "clear")
		if code != 0 {
			t.Fatalf("`harden sandbox clear` = exit %d, %v; want 0", code, answer)
		}
		if orphan != nil {
			select {
			case <-exited:
			case <-time.After(10 * time.Second):
				t.Errorf("the orphan process %d still runs after `harden sandbox clear`; want it ended", orphan.Process.Pid)
			}
			ended, _ := answer["ended"].([]any)
			reported := slices.ContainsFunc(ended, func(entry any) bool {
				fields, _ := entry.(map[string]any)
				pid, _ := fields["pid"].(float64)
				return int(pid) == orphan.Process.Pid
			})
			if !reported {
				t.Errorf("`harden sandbox clear` ended %v; want it to report process %d", ended, orphan.Process.Pid)
			}
		}
		if _, err := os.Stat(hubPath); !os.IsNotExist(err) {
			t.Errorf("Stat(%q) = %v; want the hub removed", hubPath, err)
		}
		if hubs, err := hardenshed.ListSandbox(root); err != nil || len(hubs) != 0 {
			t.Errorf("ListSandbox() = %v, %v; want no hub", hubs, err)
		}
		if _, err := os.Stat(beside); err != nil {
			t.Errorf("Stat(%q) = %v; want the directory beside the root untouched", beside, err)
		}
	}) {
		return
	}

	t.Run("every verb is refused in a darn worktree and in one with no seed", func(t *testing.T) {
		for _, location := range []*lymcwd.Location{pairOf(t, "darned", shedrun.RecipeDarn), pairOf(t, "unseeded", "")} {
			for _, verb := range []string{"new", "list", "clear"} {
				requireRefused(t, location, verb)
			}
		}
	})

	t.Run("a worktree holding a root but no hardener seed may clear it and may not make or list", func(t *testing.T) {
		location := pairOf(t, "rooted", "")
		if err := os.MkdirAll(hardenshed.SandboxRoot(location.WorktreePath()), 0o755); err != nil {
			t.Fatal(err)
		}
		if code, answer := run(t, location, "clear"); code != 0 {
			t.Errorf("`harden sandbox clear` = exit %d, %v; want 0 over an existing root", code, answer)
		}
		requireRefused(t, location, "new")
		requireRefused(t, location, "list")
	})
}
