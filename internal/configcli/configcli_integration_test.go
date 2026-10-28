//go:build tier1

// configcli_integration_test.go — e2e integration tests for configcli.
// Tests real fabriccli.RunCLI over a hubforge.CopyHub fixture.

package configcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/runprofile"
)

// TestConfigOverRealHub drives configuration edits through a real hub with one pair: a per-worktree module edit is synced into the pair's fabric worktree while the code side stays pristine, and a hub-wide module `--set` changes the hub file and commits it in _board with no per-worktree copy written.
// Its steps run in this order on one hub and pair, so the scenario builds the hub once.
// It does not call t.Parallel: it sets FABRIC_SKIP_GIT and FABRIC_SKIP_PUSH below, and t.Setenv panics under t.Parallel.
func TestConfigOverRealHub(t *testing.T) {
	const slug = "config-hub-test"

	// fabriccli.CloneAndWire has already materialized every registered module's config plus the repo-wide fabric.yaml at BoardDir, and the records-side primary already sits on its RecordsBranchName-suffixed branch.
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})

	// Topology.Add wires the new pair's junctions itself, reading the wired name-set from the real repo-wide fabric.yaml.
	// Without that the worktree has no _lym, so configengine.Edit→FindBaseDir would error.
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
		codeLayout, err := lymcwd.Resolve(codeWorktreePath)
		if err != nil {
			t.Fatalf("lymcwd.Resolve(%q): %v", codeWorktreePath, err)
		}

		// A task session's own edit syncs through fabriccli.RunCLIIn, which runs without a parent and so passes the role guard.
		t.Setenv(agentname.StrandNameEnv, "ly:"+slug+":webster")

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
		code := dispatch(codeLayout, &out, []string{"loom"}, fakeEdit, injectedSync, nil, false, nil, "", nil, true)
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
		if strings.Contains(string(allFilesOut), "_lym") {
			t.Errorf("_lym should be excluded from code-side git tracking; git ls-files output: %q", string(allFilesOut))
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
		name   string
		module string
		// layer is the --layer value; empty writes the module's own file.
		layer    string
		set      string
		cwd      string
		strand   string
		wantCode int
		wantErr  string
		// wantWritten is the text the checked file gains on an allowed write.
		wantWritten string
	}{
		{name: "task pair is refused naming the prime", module: "board", set: "labels.guard=desc", cwd: codeWorktreePath, wantCode: 1, wantErr: "prime worktree"},
		{name: "task pair is refused setting landing's deploy", module: "landing", set: "deploy=./update-plugins.sh", cwd: codeWorktreePath, wantCode: 1, wantErr: "prime worktree"},
		{name: "prime under a slug-bearing strand name is refused", module: "board", set: "labels.guard=desc", cwd: primeWorktreePath, strand: "ly:" + slug + ":webster", wantCode: 1, wantErr: "task session"},
		{name: "prime under the hub orch's slug-free name is allowed", module: "board", set: "labels.guard=desc", cwd: primeWorktreePath, strand: "ly:orch", wantWritten: "guard: desc"},
		{name: "task pair is refused writing a machine file naming the prime", module: "gate", layer: "machine", set: "tier_parallel.tier0=8", cwd: codeWorktreePath, wantCode: 1, wantErr: "prime worktree"},
	}
	for _, row := range guardRows {
		t.Run("hub-wide write guard: "+row.name, func(t *testing.T) {
			t.Setenv(agentname.StrandNameEnv, row.strand)
			checkedFile := configengine.ConfigFile(boardDir, row.module)
			args := []string{row.module, "--set", row.set}
			if row.layer != "" {
				args = append(args, "--layer", row.layer)
			}
			if row.layer == string(configengine.LayerMachine) {
				checkedFile = configengine.MachineFile(boardDir, row.module)
			}
			// A machine file is absent until a write makes it, and reads as empty bytes until then.
			readChecked := func() []byte {
				data, err := os.ReadFile(checkedFile)
				if err != nil && !(row.layer != "" && os.IsNotExist(err)) {
					t.Fatalf("read %s: %v", checkedFile, err)
				}
				return data
			}
			before := readChecked()

			var out bytes.Buffer
			if code := RunCLIIn(row.cwd, &out, args); code != row.wantCode {
				t.Fatalf("lym config %s = %d; want %d; output: %s", strings.Join(args, " "), code, row.wantCode, out.String())
			}
			after := readChecked()
			if row.wantCode != 0 {
				assertJSONErrContains(t, out.String(), row.wantErr)
				if !bytes.Equal(before, after) {
					t.Errorf("%s changed on a refused write; got %q", checkedFile, after)
				}
				return
			}
			if !strings.Contains(string(after), row.wantWritten) {
				t.Errorf("%s lacks the allowed write %q; got %q", checkedFile, row.wantWritten, after)
			}
		})
	}

	t.Run("machine layer write from the prime writes the machine file and leaves _board untouched", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, "")
		headBefore := boardHead(t, boardDir)

		var out bytes.Buffer
		if code := RunCLIIn(primeWorktreePath, &out, []string{"gate", "--layer", "machine", "--set", "tier_parallel.tier0=8"}); code != 0 {
			t.Fatalf("lym config gate --layer machine --set tier_parallel.tier0=8 = %d; want 0; output: %s", code, out.String())
		}
		if data, err := os.ReadFile(configengine.MachineFile(boardDir, "gate")); err != nil || string(data) != "tier_parallel:\n    tier0: \"8\"\n" {
			t.Errorf("machine gate file = %q, %v; want tier_parallel.tier0: 8", data, err)
		}
		if head := boardHead(t, boardDir); head != headBefore {
			t.Errorf("_board HEAD = %s after a machine write; want %s", head, headBefore)
		}
		status := exec.Command("git", "status", "--porcelain")
		status.Dir = boardDir
		statusOut, err := status.Output()
		if err != nil {
			t.Fatalf("git status in _board: %v", err)
		}
		if strings.TrimSpace(string(statusOut)) != "" {
			t.Errorf("_board status = %q after a machine write; want it empty", statusOut)
		}
	})

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

		// The real command meets the no-terminal refusal before any listing, so this drives menu with a terminal answer of true to reach its hub-write guard.
		codeLayout, err := lymcwd.Resolve(codeWorktreePath)
		if err != nil {
			t.Fatalf("lymcwd.Resolve(%q): %v", codeWorktreePath, err)
		}
		dirs := dirsOf(codeLayout)
		dirs.hubWriteGuard = realHubWriteGuard(codeLayout)
		var out bytes.Buffer
		if code := menu(dirs, strings.NewReader(strconv.Itoa(boardChoice)+"\n"), &out, makeNeverCalledEditor(t), nil, nil, true); code != 1 {
			t.Fatalf("lym config menu = %d; want 1; output: %s", code, out.String())
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

	// A file that is no terminal and a reader that is no file are the two non-terminal inputs of stdinIsTerminal; its terminal branch needs a real terminal, which no test has.
	t.Run("the real command refuses its editor paths when its input is not a terminal", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, "")
		loomFile := configengine.ConfigFile(codeWorktreePath, "loom")
		before, err := os.ReadFile(loomFile)
		if err != nil {
			t.Fatalf("read the worktree loom.yaml: %v", err)
		}
		regularFile, err := os.Create(filepath.Join(t.TempDir(), "stdin"))
		if err != nil {
			t.Fatalf("create the stdin stand-in: %v", err)
		}
		defer regularFile.Close()

		for _, tt := range []struct {
			name string
			in   io.Reader
			args []string
		}{
			{name: "a regular file on a module", in: regularFile, args: []string{"loom"}},
			{name: "a reader on the menu", in: strings.NewReader("1\n"), args: []string{"menu"}},
		} {
			cmd := Command()
			cmd.SetIn(tt.in)
			var out bytes.Buffer
			if code := clihelp.ExecuteIn(cmd, codeWorktreePath, &out, tt.args); code != 1 {
				t.Fatalf("lym config %s (%s) = %d; want 1; output: %s", tt.args[0], tt.name, code, out.String())
			}
			assertJSONErrContains(t, out.String(), "--print")
		}
		if after, err := os.ReadFile(loomFile); err != nil || !bytes.Equal(before, after) {
			t.Errorf("worktree loom.yaml = %q, %v after refused editor paths; want it unchanged", after, err)
		}
	})

	for _, row := range rows {
		t.Run("hub-wide "+row.module+" --set commits in _board", func(t *testing.T) {
			var out bytes.Buffer
			if code := RunCLIIn(primeWorktreePath, &out, []string{row.module, "--set", row.set}); code != 0 {
				t.Fatalf("lym config %s --set %s = %d; output: %s", row.module, row.set, code, out.String())
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

	t.Run("reconcile --layer default migrates renamed keys in _board and commits them", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, "")
		loomFile := configengine.ConfigFile(boardDir, "loom")
		current, err := os.ReadFile(loomFile)
		if err != nil {
			t.Fatalf("read the board loom.yaml: %v", err)
		}
		// webster_fan holds a value under the old name alone, and plan_fan sits beside the new name it was renamed to.
		// The hub base is sparse, so the new name is planted for plan_fan to sit beside.
		var planted []string
		for _, line := range strings.Split(strings.TrimRight(string(current), "\n"), "\n") {
			if !strings.HasPrefix(line, "code_reviewer_fan:") && !strings.HasPrefix(line, "plan_reviewer_fan:") {
				planted = append(planted, line)
			}
		}
		planted = append(planted, "webster_fan: review-code", "plan_reviewer_fan: review-plan", "plan_fan: stale-fan")
		before := []byte(strings.Join(planted, "\n") + "\n")
		if err := os.WriteFile(loomFile, before, 0o644); err != nil {
			t.Fatalf("plant old keys in the board loom.yaml: %v", err)
		}
		const templateEqualLenses = "lenses: {}\nfans:\n  standard: [generic, generic, correctness, error-handling, test-gaps]\n"
		if err := os.WriteFile(configengine.ConfigFile(boardDir, "lenses"), []byte(templateEqualLenses), 0o644); err != nil {
			t.Fatalf("plant a template-equal fan in the board lenses.yaml: %v", err)
		}

		// A reconcile never reads or writes a machine file, so its bytes stay as seeded through the apply below.
		const machineBytes = "slots: 1\ntier_parallel:\n  tier0: 8\n"
		machineFile := configengine.MachineFile(boardDir, "gate")
		if err := os.MkdirAll(filepath.Dir(machineFile), 0o755); err != nil {
			t.Fatalf("create the board's .lym config dir: %v", err)
		}
		if err := os.WriteFile(machineFile, []byte(machineBytes), 0o644); err != nil {
			t.Fatalf("seed the machine file: %v", err)
		}

		code, result := runReconcileCLI(t, primeWorktreePath, "--layer", "default")
		if code != 0 {
			t.Fatalf("reconcile --layer default = %d; want 0; result: %v", code, result)
		}
		loom := reconcileModule(t, result, "loom")
		if loom["layer"] != string(configengine.LayerDefault) || loom["seed"] != "hub" || loom["applied"] != false {
			t.Errorf("dry-run loom entry = %v; want layer default, seed hub, applied false", loom)
		}
		migrated := fmt.Sprint(loom["migrated"])
		for _, want := range []string{"webster_fan", "plan_fan"} {
			if !strings.Contains(migrated, want) {
				t.Errorf("dry-run loom migrated = %s; want it to name %s", migrated, want)
			}
		}
		if dropped := fmt.Sprint(reconcileModule(t, result, "lenses")["dropped"]); dropped != "[fans.standard]" {
			t.Errorf("dry-run lenses dropped = %s; want [fans.standard]", dropped)
		}

		refusals := []struct {
			name    string
			cwd     string
			args    []string
			wantErr string
		}{
			{name: "apply from a task pair", cwd: codeWorktreePath, args: []string{"--layer", "default", "--apply"}, wantErr: "prime worktree"},
			{name: "the board dir", cwd: boardDir, args: []string{"--apply"}, wantErr: "lym config reconcile --layer default"},
			{name: "a layer other than default", cwd: primeWorktreePath, args: []string{"--layer", "local", "--apply"}, wantErr: "--layer default alone"},
			{name: "the board dir with a layer other than default", cwd: boardDir, args: []string{"--layer", "local"}, wantErr: "prime worktree"},
		}
		for _, refusal := range refusals {
			var out bytes.Buffer
			if code := RunCLIIn(refusal.cwd, &out, append([]string{"reconcile"}, refusal.args...)); code != 1 {
				t.Errorf("reconcile from %s = %d; want 1; output: %s", refusal.name, code, out.String())
			}
			assertJSONErrContains(t, out.String(), refusal.wantErr)
		}
		if code, _ := runReconcileCLI(t, codeWorktreePath, "--layer", "default"); code != 0 {
			t.Errorf("dry-run reconcile --layer default from a task pair = %d; want 0", code)
		}
		if after, err := os.ReadFile(loomFile); err != nil || !bytes.Equal(before, after) {
			t.Fatalf("board loom.yaml = %q, %v after a dry run and refusals; want it unchanged", after, err)
		}

		if runtime.GOOS != "windows" {
			t.Run("a failed apply leaves the board files as they were", func(t *testing.T) {
				if os.Geteuid() == 0 {
					t.Skip("file mode bits do not bind root")
				}
				configDir := configengine.ConfigDir(boardDir)
				if err := os.Chmod(configDir, 0o555); err != nil {
					t.Fatalf("make the board config dir read-only: %v", err)
				}
				t.Cleanup(func() { _ = os.Chmod(configDir, 0o755) })
				headBefore := boardHead(t, boardDir)

				var out bytes.Buffer
				if code := RunCLIIn(primeWorktreePath, &out, []string{"reconcile", "--layer", "default", "--apply"}); code != 1 {
					t.Fatalf("reconcile --layer default --apply into a read-only dir = %d; want 1; output: %s", code, out.String())
				}
				assertJSONErrContains(t, out.String(), "reconcile:")
				if after, err := os.ReadFile(loomFile); err != nil || !bytes.Equal(before, after) {
					t.Errorf("board loom.yaml = %q, %v after a failed apply; want it unchanged", after, err)
				}
				if head := boardHead(t, boardDir); head != headBefore {
					t.Errorf("_board HEAD = %s after a failed apply; want %s", head, headBefore)
				}
			})
		}

		// A hub-wide orch rules file left uncommitted in Bolt must stay out of the config commit.
		const rulesContent = "an uncommitted rule\n"
		rulesRel := filepath.Join("_lym", "orch", "rules.md")
		rulesFile := filepath.Join(boardDir, rulesRel)
		if err := os.MkdirAll(filepath.Dir(rulesFile), 0o755); err != nil {
			t.Fatalf("create the board orch dir: %v", err)
		}
		if err := os.WriteFile(rulesFile, []byte(rulesContent), 0o644); err != nil {
			t.Fatalf("write the uncommitted rules file: %v", err)
		}

		if code, result = runReconcileCLI(t, primeWorktreePath, "--layer", "default", "--apply"); code != 0 {
			t.Fatalf("reconcile --layer default --apply = %d; want 0; result: %v", code, result)
		}
		after, err := os.ReadFile(loomFile)
		if err != nil {
			t.Fatalf("read the board loom.yaml: %v", err)
		}
		for _, want := range []string{"\ncode_reviewer_fan: review-code", "\nplan_reviewer_fan: review-plan"} {
			if !strings.Contains(string(after), want) {
				t.Errorf("applied board loom.yaml lacks %q; got %q", want, after)
			}
		}
		for _, gone := range []string{"\nwebster_fan:", "\nwebster_reviewer_fan:", "\nplan_fan:", "stale-fan"} {
			if strings.Contains(string(after), gone) {
				t.Errorf("applied board loom.yaml still holds %q; got %q", gone, after)
			}
		}

		show := exec.Command("git", "show", "--name-only", "--pretty=format:%s", "HEAD")
		show.Dir = boardDir
		showOut, err := show.Output()
		if err != nil {
			t.Fatalf("git show in _board: %v", err)
		}
		wantShow := "config: reconcile the board dir's files\n" + strings.ReplaceAll(configengine.ConfigFileRel("lenses"), "\\", "/") + "\n" + strings.ReplaceAll(configengine.ConfigFileRel("loom"), "\\", "/")
		if strings.TrimSpace(string(showOut)) != wantShow {
			t.Errorf("HEAD in _board = %q; want %q", showOut, wantShow)
		}

		status := exec.Command("git", "status", "--porcelain", "--untracked-files=all")
		status.Dir = boardDir
		statusOut, err := status.Output()
		if err != nil {
			t.Fatalf("git status in _board: %v", err)
		}
		wantStatus := "?? " + strings.ReplaceAll(rulesRel, "\\", "/")
		if strings.TrimSpace(string(statusOut)) != wantStatus {
			t.Errorf("_board status = %q after the apply; want only %q untracked", statusOut, wantStatus)
		}
		if kept, err := os.ReadFile(rulesFile); err != nil || string(kept) != rulesContent {
			t.Errorf("rules file = %q, %v after the apply; want it unchanged", kept, err)
		}
		if kept, err := os.ReadFile(machineFile); err != nil || string(kept) != machineBytes {
			t.Errorf("machine gate file = %q, %v after the apply; want it unchanged", kept, err)
		}
	})

	t.Run("reconcile --layer default --apply prunes every retired loom, landing and gate key and leaves the profiles file as written", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, "")
		appendPlanted := func(module, planted string) {
			t.Helper()
			file := configengine.ConfigFile(boardDir, module)
			current, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read the board %s.yaml: %v", module, err)
			}
			body := strings.TrimRight(string(current), "\n")
			if body != "" {
				body += "\n"
			}
			if err := os.WriteFile(file, []byte(body+planted), 0o644); err != nil {
				t.Fatalf("plant retired keys in the board %s.yaml: %v", module, err)
			}
		}
		appendPlanted("loom", "webster_max_bounces: 2\nmerge_max_bounces: 1\nalignment_pr_approve: true\nrework: true\n")
		appendPlanted("landing", "require_pr_to_base: []\n")
		gateFile := configengine.ConfigFile(boardDir, "gate")
		if err := os.WriteFile(gateFile, []byte("gates:\n  round: [0, 1]\n  publish: [0]\n"), 0o644); err != nil {
			t.Fatalf("plant a publish gate in the board gate.yaml: %v", err)
		}
		appendPlanted("profiles", "retired-keys:\n  loom:\n    loom: {webster_max_bounces: 3, merge_max_bounces: 1}\n")
		profilesFile := configengine.ConfigFile(boardDir, "profiles")
		profilesBefore, err := os.ReadFile(profilesFile)
		if err != nil {
			t.Fatalf("read the board profiles.yaml: %v", err)
		}

		if code, result := runReconcileCLI(t, primeWorktreePath, "--layer", "default", "--apply"); code != 0 {
			t.Fatalf("reconcile --layer default --apply = %d; want 0; result: %v", code, result)
		}

		read := func(module string) string {
			t.Helper()
			content, err := os.ReadFile(configengine.ConfigFile(boardDir, module))
			if err != nil {
				t.Fatalf("read the board %s.yaml: %v", module, err)
			}
			return string(content)
		}
		loom := read("loom")
		if !strings.Contains(loom, "\ncode_max_bounces: 2 ") {
			t.Errorf("applied board loom.yaml lacks code_max_bounces: 2; got %q", loom)
		}
		for _, gone := range []string{"webster_max_bounces", "merge_max_bounces", "alignment_pr_approve", "rework"} {
			if strings.Contains(loom, gone) {
				t.Errorf("applied board loom.yaml still holds %q; got %q", gone, loom)
			}
		}
		if landing := read("landing"); !strings.Contains(landing, "\ndeploy_branches: []") || strings.Contains(landing, "require_pr_to_base") {
			t.Errorf("applied board landing.yaml = %q; want an empty deploy_branches and no require_pr_to_base", landing)
		}
		if gate := read("gate"); !strings.Contains(gate, "webster-fix: {tiers: [0, 1], scope: diff}") || strings.Contains(gate, "round") || strings.Contains(gate, "publish") {
			t.Errorf("applied board gate.yaml = %q; want the round list migrated to webster-fix and no publish", gate)
		}
		if profilesAfter := read("profiles"); profilesAfter != string(profilesBefore) {
			t.Errorf("board profiles.yaml = %q after the apply; want %q, byte for byte", profilesAfter, profilesBefore)
		}

		// The profile still holding the retired keys seeds a run: the Webster key reads as its Code successor and the Merge key is dropped.
		profiles, err := runprofile.Load(configengine.Dirs{Board: boardDir})
		if err != nil {
			t.Fatalf("runprofile.Load: %v", err)
		}
		profile, err := profiles.Settings("retired-keys", "loom")
		if err != nil {
			t.Fatalf("Settings(retired-keys, loom): %v", err)
		}
		documents, err := configreg.LayerRunSettings("retired-keys", profile, nil)
		if err != nil {
			t.Fatalf("LayerRunSettings(retired-keys): %v", err)
		}
		if got, want := string(documents["loom"]), "code_max_bounces: 3 "; !strings.HasPrefix(got, want) || strings.Contains(got, "merge_max_bounces") {
			t.Errorf("seeded loom layer = %q; want it to start with %q and hold no merge_max_bounces", got, want)
		}
	})

	t.Run("reconcile --layer default lists an unmigrated base's strip as pending and never performs it", func(t *testing.T) {
		t.Setenv(agentname.StrandNameEnv, "")
		loomTemplate, ok := configreg.Template("loom")
		if !ok {
			t.Fatal("configreg has no loom template")
		}
		loomFile := configengine.ConfigFile(boardDir, "loom")
		full := []byte(loomTemplate())
		if err := os.WriteFile(loomFile, full, 0o644); err != nil {
			t.Fatalf("write the current template whole into the board loom.yaml: %v", err)
		}
		marker := configengine.BaseSparseMarker(boardDir)
		if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove the board marker: %v", err)
		}

		code, result := runReconcileCLI(t, primeWorktreePath, "--layer", "default")
		if code != 0 {
			t.Fatalf("reconcile --layer default = %d; want 0; result: %v", code, result)
		}
		if stripped := reconcileModule(t, result, "loom")["stripped"]; stripped == nil {
			t.Errorf("dry-run loom entry lists no stripped keys; want the template-equal keys; result: %v", result)
		}
		if _, ok := result["base_strip"].(string); !ok {
			t.Errorf("dry-run envelope lacks base_strip; got %v", result)
		}

		if code, result = runReconcileCLI(t, primeWorktreePath, "--layer", "default", "--apply"); code != 0 {
			t.Fatalf("reconcile --layer default --apply = %d; want 0; result: %v", code, result)
		}
		if after, err := os.ReadFile(loomFile); err != nil || !bytes.Equal(after, full) {
			t.Errorf("board loom.yaml = %q, %v after the apply; want the units left in place", after, err)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Errorf("board marker stat err = %v after the apply; want it absent", err)
		}
	})
}

// boardHead returns the commit the board repo at boardDir is on.
func boardHead(t *testing.T, boardDir string) string {
	t.Helper()
	revParse := exec.Command("git", "rev-parse", "HEAD")
	revParse.Dir = boardDir
	out, err := revParse.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD in _board: %v", err)
	}
	return strings.TrimSpace(string(out))
}
