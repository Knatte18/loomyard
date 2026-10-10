//go:build integration

// configcli_integration_test.go — e2e integration tests for configcli.
// Tests real fabriccli.RunCLI over a hubforge.NewHub fixture.

package configcli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestConfigOverRealHub drives configuration edits through a real hub with one pair: a per-worktree module edit is synced into the pair's fabric worktree while the code side stays pristine, and a hub-wide module `--set` changes the hub file and commits it in _board with no per-worktree copy written.
// Its steps run in this order on one hub and pair, so the scenario builds the hub once.
// It does not call t.Parallel: it sets FABRIC_SKIP_GIT and FABRIC_SKIP_PUSH below, and t.Setenv panics under t.Parallel.
func TestConfigOverRealHub(t *testing.T) {
	const slug = "config-hub-test"

	// fabriccli.CloneAndWire has already materialized every registered module's config plus the repo-wide fabric.yaml at BoardDir, and the records-side primary already sits on its RecordsBranchName-suffixed branch.
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	// Topology.Add wires the new pair's junctions itself, reading the wired name-set from the real repo-wide fabric.yaml.
	// Without that the worktree has no _lyx, so configengine.Edit→FindBaseDir would error.
	if _, err := h.Topology.Add(h.Location, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
		t.Fatalf("Topology.Add(%q): %v", slug, err)
	}
	codeWorktreePath := fabricengine.WorktreePath(h.Location, slug)
	fabricWorktreePath := fabricengine.RecordsWorktreePath(h.Location, slug)
	boardDir := fabricengine.BoardDir(h.Location.HubPath)

	// Explicitly clear FABRIC_SKIP_GIT and FABRIC_SKIP_PUSH so the commits are not silent no-ops.
	t.Setenv("FABRIC_SKIP_GIT", "")
	t.Setenv("FABRIC_SKIP_PUSH", "")

	if !t.Run("per-worktree edit is synced into the fabric worktree and the code side stays pristine", func(t *testing.T) {
		codeLayout, err := lyxcwd.Resolve(codeWorktreePath)
		if err != nil {
			t.Fatalf("lyxcwd.Resolve(%q): %v", codeWorktreePath, err)
		}

		// loom is per-worktree, so this exercises the fabric-sync path.
		validYAML := "discussion_timeout_min: 60\n"
		fakeEdit := func(path string) error {
			return os.WriteFile(path, []byte(validYAML), 0o644)
		}

		// sync calls a detached spawnPush that cannot run in-process, so the injected sync commits instead.
		injectedSync := func(w io.Writer) int {
			return fabriccli.RunCLIIn(codeWorktreePath, w, []string{"commit"})
		}

		var out bytes.Buffer
		code := dispatch(codeLayout, &out, []string{"loom"}, fakeEdit, injectedSync, nil, false, nil, nil)
		if code != 0 {
			t.Errorf("dispatch() = %d; want 0; output: %s", code, out.String())
		}

		configRelPath := configengine.ConfigFile(".", "loom")
		configPath := filepath.Join(fabricWorktreePath, configRelPath)
		// git always uses forward slashes.
		configRelPathForGit := strings.ReplaceAll(configRelPath, "\\", "/")

		configContent, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("failed to read config file from fabric worktree at %s: %v", configPath, err)
		}
		if string(configContent) != validYAML {
			t.Errorf("loom config content mismatch; got %q, want %q", string(configContent), validYAML)
		}

		cmd := exec.Command("git", "ls-files", configRelPathForGit)
		cmd.Dir = fabricWorktreePath
		lsFilesOut, err := cmd.Output()
		if err != nil {
			t.Fatalf("git ls-files failed: %v", err)
		}
		if !strings.Contains(string(lsFilesOut), configRelPathForGit) {
			t.Errorf("config file not tracked in fabric worktree; git ls-files output: %q", string(lsFilesOut))
		}

		cmd = exec.Command("git", "ls-files")
		cmd.Dir = codeWorktreePath
		allFilesOut, err := cmd.Output()
		if err != nil {
			t.Fatalf("code-side git ls-files failed: %v", err)
		}
		if strings.Contains(string(allFilesOut), "_lyx") {
			t.Errorf("_lyx should be excluded from code-side git tracking; git ls-files output: %q", string(allFilesOut))
		}

		outStr := out.String()
		if !strings.Contains(outStr, "edited and synced") {
			t.Errorf("dispatch output missing success message; got %q", outStr)
		}
		var env map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(outStr)), &env); err != nil {
			t.Fatalf("dispatch output is not valid JSON: %v; got %q", err, outStr)
		}
		if ok, _ := env["ok"].(bool); !ok {
			t.Errorf("dispatch output envelope ok = %v; want true; got %q", env["ok"], outStr)
		}
		if module, _ := env["module"].(string); module != "loom" {
			t.Errorf("dispatch output envelope module = %q; want \"loom\"; got %q", module, outStr)
		}
	}) {
		return
	}

	rows := []struct {
		module string
		set    string
		want   string
	}{
		{module: "board", set: "labels.x=desc", want: "x: desc"},
		{module: "fabric", set: "branch_prefix=x", want: "branch_prefix: x"},
	}
	// A hub-wide write is refused from a pair and from a slug-bearing session, so the writes below run from the prime with no strand name.
	t.Setenv(agentname.StrandNameEnv, "")
	primeWorktreePath := h.PrimeWorktree()

	guardRows := []struct {
		name     string
		module   string
		set      string
		cwd      string
		strand   string
		wantCode int
		wantErr  string
		// wantWritten is the text the hub file gains on an allowed write.
		wantWritten string
	}{
		{name: "task pair is refused naming the prime", module: "board", set: "labels.guard=desc", cwd: codeWorktreePath, wantCode: 1, wantErr: "prime worktree"},
		{name: "task pair is refused weakening landing's publish_verify", module: "landing", set: "publish_verify=", cwd: codeWorktreePath, wantCode: 1, wantErr: "prime worktree"},
		{name: "prime under a slug-bearing strand name is refused", module: "board", set: "labels.guard=desc", cwd: primeWorktreePath, strand: "ly:" + slug + ":webster", wantCode: 1, wantErr: "task session"},
		{name: "prime under the hub orch's slug-free name is allowed", module: "board", set: "labels.guard=desc", cwd: primeWorktreePath, strand: "ly:orch", wantWritten: "guard: desc"},
	}
	for _, row := range guardRows {
		t.Run("hub-wide write guard: "+row.name, func(t *testing.T) {
			t.Setenv(agentname.StrandNameEnv, row.strand)
			hubFile := configengine.ConfigFile(boardDir, row.module)
			before, err := os.ReadFile(hubFile)
			if err != nil {
				t.Fatalf("read hub %s.yaml: %v", row.module, err)
			}

			var out bytes.Buffer
			if code := RunCLIIn(row.cwd, &out, []string{row.module, "--set", row.set}); code != row.wantCode {
				t.Fatalf("lyx config %s --set %s = %d; want %d; output: %s", row.module, row.set, code, row.wantCode, out.String())
			}
			after, err := os.ReadFile(hubFile)
			if err != nil {
				t.Fatalf("read hub %s.yaml: %v", row.module, err)
			}
			if row.wantCode != 0 {
				assertJSONErrContains(t, out.String(), row.wantErr)
				if !bytes.Equal(before, after) {
					t.Errorf("hub %s.yaml changed on a refused write; got %q", row.module, after)
				}
				return
			}
			if !strings.Contains(string(after), row.wantWritten) {
				t.Errorf("hub %s.yaml lacks the allowed write %q; got %q", row.module, row.wantWritten, after)
			}
		})
	}

	t.Run("hub-wide write guard: the menu from a task pair is refused naming the prime", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, "")
		// An editor that exits at once keeps a missing guard from opening a real one; the refusal is what the test pins.
		t.Setenv("VISUAL", "true")
		boardChoice := slices.IndexFunc(configreg.Modules(), func(m configreg.Module) bool { return m.Name == "board" }) + 1
		hubFile := configengine.ConfigFile(boardDir, "board")
		before, err := os.ReadFile(hubFile)
		if err != nil {
			t.Fatalf("read hub board.yaml: %v", err)
		}

		cmd := Command()
		cmd.SetIn(strings.NewReader(strconv.Itoa(boardChoice) + "\n"))
		var out bytes.Buffer
		if code := clihelp.ExecuteIn(cmd, codeWorktreePath, &out, []string{"menu"}); code != 1 {
			t.Fatalf("lyx config menu = %d; want 1; output: %s", code, out.String())
		}
		menuListing, envelope, _ := strings.Cut(out.String(), "{")
		if !strings.Contains(menuListing, "board") {
			t.Errorf("menu listing = %q; want it to list board", menuListing)
		}
		assertJSONErrContains(t, "{"+envelope, "prime worktree")
		if after, err := os.ReadFile(hubFile); err != nil || !bytes.Equal(before, after) {
			t.Errorf("hub board.yaml = %q, %v after a refused menu edit; want it unchanged", after, err)
		}
	})

	for _, row := range rows {
		t.Run("hub-wide "+row.module+" --set commits in _board", func(t *testing.T) {
			var out bytes.Buffer
			if code := RunCLIIn(primeWorktreePath, &out, []string{row.module, "--set", row.set}); code != 0 {
				t.Fatalf("lyx config %s --set %s = %d; output: %s", row.module, row.set, code, out.String())
			}

			hubFile := configengine.ConfigFile(boardDir, row.module)
			data, err := os.ReadFile(hubFile)
			if err != nil {
				t.Fatalf("read hub %s.yaml: %v", row.module, err)
			}
			if !strings.Contains(string(data), row.want) {
				t.Errorf("hub %s.yaml lacks %q; got %q", row.module, row.want, data)
			}

			status := exec.Command("git", "status", "--porcelain")
			status.Dir = boardDir
			statusOut, err := status.Output()
			if err != nil {
				t.Fatalf("git status in _board: %v", err)
			}
			if strings.TrimSpace(string(statusOut)) != "" {
				t.Errorf("_board is dirty after the edit: %q", statusOut)
			}

			show := exec.Command("git", "show", "--name-only", "--pretty=format:", "HEAD")
			show.Dir = boardDir
			showOut, err := show.Output()
			if err != nil {
				t.Fatalf("git show in _board: %v", err)
			}
			wantRel := strings.ReplaceAll(configengine.ConfigFileRel(row.module), "\\", "/")
			if strings.TrimSpace(string(showOut)) != wantRel {
				t.Errorf("HEAD in _board touches %q; want only %q", showOut, wantRel)
			}

			if _, err := os.Stat(configengine.ConfigFile(fabricWorktreePath, row.module)); !os.IsNotExist(err) {
				t.Errorf("a per-worktree %s.yaml exists; stat err = %v", row.module, err)
			}
		})
	}
}
