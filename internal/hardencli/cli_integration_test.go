// cli_integration_test.go drives the sandbox verbs over a hub fixture.
// It builds a real sandbox hub of each layout through the verbs.

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
	"github.com/Knatte18/loomyard/internal/gitkit"
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
	// run executes `lym harden sandbox <verb> <flags>` in the worktree and returns the exit code and the last JSON line.
	run := func(t *testing.T, location *lymcwd.Location, verb string, flags ...string) (int, map[string]any) {
		t.Helper()
		var out bytes.Buffer
		code := clihelp.ExecuteIn(newCommand(deps), location.AnchorPath(), &out, append([]string{"sandbox", verb}, flags...))
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
		if answer["single_repo"] != false {
			t.Errorf("single_repo = %v; want false for a hub made without --single-repo", answer["single_repo"])
		}
		bares, err := filepath.Glob(filepath.Join(filepath.Dir(hubPath), "*-bare"))
		if err != nil || len(bares) != 2 {
			t.Errorf("Glob(the container's bares) = %v, %v; want exactly two, the code bare and the records bare", bares, err)
		}
	}) {
		return
	}

	var singleRepoHub string
	if !t.Run("new --single-repo builds a one-repository hub whose one bare holds the records under refs/lym/", func(t *testing.T) {
		code, answer := run(t, hardener, "new", "--"+singleRepoFlag)
		if code != 0 {
			t.Fatalf("`harden sandbox new --%s` = exit %d, %v; want 0", singleRepoFlag, code, answer)
		}
		singleRepoHub, _ = answer["hub"].(string)
		if answer["single_repo"] != true {
			t.Errorf("single_repo = %v; want true", answer["single_repo"])
		}
		container := filepath.Dir(singleRepoHub)
		if filepath.Dir(container) != root {
			t.Fatalf("hub = %q; want a hub in a container directly under the sandbox root %q", singleRepoHub, root)
		}
		bares, err := filepath.Glob(filepath.Join(container, "*-bare"))
		if err != nil || len(bares) != 1 {
			t.Fatalf("Glob(the container's bares) = %v, %v; want exactly one bare", bares, err)
		}
		if refs := gitkit.Git(t, bares[0], "for-each-ref", "--format=%(refname)", "refs/lym/"); strings.TrimSpace(refs) == "" {
			t.Errorf("git for-each-ref refs/lym/ in %s = %q; want the hub's records under refs/lym/", bares[0], refs)
		}
	}) {
		return
	}

	if !t.Run("list shows the hubs later commands made, in the order they were made", func(t *testing.T) {
		code, answer := run(t, hardener, "list")
		hubs, _ := answer["hubs"].([]any)
		if code != 0 || len(hubs) != 2 || hubs[0] != hubPath || hubs[1] != singleRepoHub {
			t.Errorf("`harden sandbox list` = exit %d, %v; want exit 0 listing %q then %q", code, answer, hubPath, singleRepoHub)
		}
	}) {
		return
	}

	if !t.Run("clear ends a process naming the root, removes the hubs and leaves a directory beside the root", func(t *testing.T) {
		// The process scan reads /proc, so only Linux has an orphan to end.
		var orphan *exec.Cmd
		exited := make(chan struct{})
		if runtime.GOOS == "linux" {
			// The process names the root only by its working directory, which /proc reports through the worktree's `.lym` link.
			orphan = exec.Command("sh", "-c", "while :; do sleep 1; done")
			orphan.Dir = root
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
		for _, hub := range []string{hubPath, singleRepoHub} {
			if _, err := os.Stat(hub); !os.IsNotExist(err) {
				t.Errorf("Stat(%q) = %v; want the hub removed", hub, err)
			}
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
