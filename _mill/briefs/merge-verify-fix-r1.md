# Verify-Fix Brief

The verify command `go test -tags integration ./internal/weftengine/... ./internal/weftcli/... -count=1` failed after a merge. Your job is to diagnose the failures and fix the code so the verify command passes.

## Verify Output

```
FAIL	github.com/Knatte18/loomyard/internal/weftengine [build failed]
ok  	github.com/Knatte18/loomyard/internal/weftcli	4.714s
FAIL
# github.com/Knatte18/loomyard/internal/weftengine [github.com/Knatte18/loomyard/internal/weftengine.test]
internal\weftengine\sync_test.go:251:55: not enough arguments in call to Commit
	have (string, []string, SyncOptions)
	want (string, []string, string, SyncOptions)
```

## Merge Diff

```diff
diff --git a/README.md b/README.md
new file mode 100644
index 0000000..2d07c9a
--- /dev/null
+++ b/README.md
@@ -0,0 +1,114 @@
+# LoomYard
+
+LoomYard (LY) is a task-orchestration system for [Claude Code](https://claude.ai/code). It manages the lifecycle of coding tasks — from triaging issues to merging finished code — using AI subagents for design, planning, implementation, and review, with each task isolated in its own git worktree.
+
+At its center is **`lyx`** — a single Go binary (LoomYard eXecutable) that owns the task board, the git topology, and (in progress) the orchestrator. Everything else in LY is built around `lyx`. The repo is under active development: several modules ship today, and the orchestration layers are being built out.
+
+> **A re-implementation of Millhouse in Go.** LoomYard is a ground-up rebuild of [Millhouse](https://github.com/Knatte18/millhouse) — same goal (task orchestration for Claude Code with isolated worktrees and AI subagents), rebuilt in Go instead of Python: one compiled binary, deep internal tests, and a cleaner geometry/overlay model.
+
+## Inspiration
+
+Through Millhouse, LoomYard builds on ideas from three projects:
+
+- **[claude-code-plugins](https://github.com/motlin/claude-code-plugins)** by Craig Motlin — task tracking and skill plugins for Claude Code
+- **[autoboard](https://github.com/willietran/autoboard)** by Willie Tran — autonomous agent orchestration patterns
+- **[skills](https://github.com/mattpocock/skills)** by Matt Pocock — Claude Code skill conventions
+
+## Naming: `lyx` · `loom` · `ly`
+
+Three names for three layers, deliberately non-overlapping:
+
+- **`lyx`** — the binary/CLI (**L**oom**Y**ard e**X**ecutable): one binary with a namespaced subcommand tree (`lyx board`, `lyx weft`, `lyx warp`, …).
+- **`loom`** — the orchestrator *module* (`lyx loom run`), a domain like `board` or `weft` that drives a phased run.
+- **`ly`** — the skill / orchestration plugin; skills are `/ly-*`.
+
+Convenience alias: **`lyx run` → `lyx loom run`** (the everyday autonomous call).
+
+## Design principles
+
+1. **Toolkit-first.** Build small, composable primitives (board, warp, weft, mux) before the orchestrator that ties them together.
+2. **One-shot, daemonless, file-coordinated.** A command does its work, writes JSON to stdout, and exits. Concurrent processes cooperate through files and locks, not a server.
+3. **cwd-authoritative.** Config and state resolve from the current working directory, which need not equal the git-repo root.
+4. **Correctness by tool design, not by recall.** A `lyx` command makes the correct path the path of least resistance and makes drift *detectable*, rather than relying on an operator or agent to remember a rule.
+
+## Weft overlay model
+
+LoomYard keeps the host repo pristine by routing all its own artifacts into a companion **weft repo** — a separate git repository that `lyx` controls.
+
+```
+<hub>/                              (top-level Hub, NOT a git repo)
+  ├── <prime>/                      (host worktree, main branch)
+  ├── <prime>-weft/                 (weft Prime worktree)
+  ├── <slug>/                       (additional host worktree)
+  ├── <slug>-weft/                  (weft worktree for <slug>)
+  └── _board/                       (board repo; the task store)
+```
+
+Each host worktree uses a **junction** (Windows) or symlink to route writes (`_lyx/config/`, `_codeguide/`) into its sibling weft worktree — transparently, so code that writes `_lyx/config/board.yaml` never sees the indirection. Two state roots with opposite lifecycles: **`_lyx/`** is durable and weft-synced (config, board, orchestration status — resume works across machines); **`.lyx/`** is ephemeral and machine-bound (live psmux runtime state, never synced).
+
+All worktree and Hub geometry resolves through a single package, `internal/hubgeometry` — the sole owner of cwd and worktree-root math. See [CONSTRAINTS.md](CONSTRAINTS.md).
+
+## Modules
+
+Every user-facing module is a `lyx <module>` namespace, assembled into one cobra root. All commands print JSON: `{"ok":true, ...}` on success, `{"ok":false,"error":"..."}` on failure.
+
+**Shipped:**
+
+- **init** — scaffolds `_lyx/` and reconciles every module's config against its template (idempotent; never clobbers existing values).
+- **board** — the task-tracker board.
+- **config** — view/edit module configs; `lyx config reconcile` reconciles all configs against their templates; `lyx config <module> --set key=value` writes values non-interactively.
+- **weft** — owns all git into the paired weft repo (`status|commit|push|pull|sync`).
+- **warp** — the host↔weft git topology owner: clone, dual-worktree add/remove, coordinated checkout, reconcile, status, prune, cleanup.
+- **ide** — one-shot IDE launcher for worktrees, with an interactive menu.
+- **muxpoc** — a shipped proof-of-concept psmux orchestrator.
+- **selfreport** — file bugs/enhancements against the repo via `gh`.
+
+**In progress (design):**
+
+- **mux** — the psmux overlay + strand bookkeeping + render.
+- **loom** — the phased orchestrator (Setup → Discussion → Plan → Builder → Finalize), each phase gated by a review.
+- **review** — a generic profile-driven gate engine, used by `loom` and standalone.
+
+The internal libraries **proc** (cross-OS process spawn) and **shuttle** (drive one LLM agent via a swappable engine) sit under these; see [docs/modules/](docs/modules/).
+
+## Orchestration stack
+
+The orchestrator is a layered stack, each layer knowing only the one below. It has this shape because agents run as **interactive psmux sessions, never headless `claude -p`** — so spawning an agent is "place a pane, launch a provider, drive it, detect completion," not a plain `exec`.
+
+```
+internal/proc     spawn any OS process, cross-OS                    [OS primitive]
+internal/mux      psmux overlay + strand bookkeeping + render       [builds on proc]
+internal/shuttle  run ONE LLM agent via a swappable engine          [builds on mux]
+review            generic gate engine: handler/fixer + judge        [builds on shuttle]
+loom              phase machine: drive each phase through a gate     [builds on review]
+```
+
+The whole stack runs headless (auto mode): strands exist, agents run, output files are read, nobody need watch.
+
+## Building
+
+```bash
+go build ./cmd/lyx        # build the lyx binary
+go test ./...             # run the full suite (structural invariants included)
+```
+
+`deploy.cmd` builds and installs `lyx` onto PATH. Once deployed, run `lyx init` from a worktree to scaffold its `_lyx/` config.
+
+## Sandbox Hub
+
+The **sandbox Hub** is a dedicated bench for dogfooding `lyx` against itself, exercising the real deployed binary end to end. Build it with `sandbox-build.cmd`, run the agent suite with `sandbox-suite.cmd`, and collect its findings with `sandbox-fetch.cmd`. See [docs/sandbox-howto.md](docs/sandbox-howto.md) for the runbook.
+
+## Requirements
+
+- [Claude Code](https://claude.ai/code)
+- Go 1.26+
+- `gh` CLI authenticated (`gh auth login`)
+- Git 2.35+ (for `git worktree`)
+- psmux (for the orchestration layers)
+
+## Documentation
+
+- [CONSTRAINTS.md](CONSTRAINTS.md) — the repo's structural invariants (authoritative).
+- [docs/overview.md](docs/overview.md) — architecture, naming, module and shared-lib map.
+- [docs/roadmap.md](docs/roadmap.md) — numbered milestones and long-term direction.
+- [docs/modules/](docs/modules/) — the module map and per-module design docs.
diff --git a/docs/overview.md b/docs/overview.md
index 84f8e79..091a245 100644
--- a/docs/overview.md
+++ b/docs/overview.md
@@ -211,7 +211,7 @@ User-facing modules each get one `lyx <module>` namespace:
 
 - **init** — scaffolds the `_lyx/` directory structure and creates all module config files via reconciliation against templates (`internal/initcli`). Idempotent: does not clobber existing config files. `lyx init --undo` reverses that scaffolding (junction, weft-side content, `.gitignore` block, `.git/info/exclude` entry) for test/sandbox cleanup. ✅ Implemented.
 - **board** — the task-tracker board (`internal/boardcli` + `internal/boardengine`). ✅ Implemented.
-- **config** — interactive menu for viewing and editing module configs; `lyx config reconcile` reconciles all module config files against their live templates (dry-run by default, `--apply` writes atomically). ✅ Implemented.
+- **config** — interactive menu for viewing and editing module configs; `lyx config reconcile` reconciles all module config files against their live templates (dry-run by default, `--apply` writes atomically); `lyx config <module> --set key=value` (repeatable) writes one or more config values directly with no editor invocation, for scripts/agents that need a non-interactive path. ✅ Implemented.
 - **weft** — owns all git into the paired weft repo (`lyx weft status|commit|push|pull|sync`). ✅ Implemented.
 - **warp** — **host↔weft-coordinated git topology**: clone (hub-creator), dual-worktree add/remove, coordinated checkout (switches host+weft together + re-points junctions), reconcile, status, prune, cleanup. The single owner of the mirror invariant — consolidates the former `worktree` / `git-clone` modules and `internal/git`; its CLI surface is `lyx warp clone|add|list|remove|checkout|status|reconcile|prune|cleanup`. ✅ Implemented.
 - **ide** — one-shot VS Code launcher with interactive menu. ✅ Implemented.
diff --git a/internal/configcli/configcli.go b/internal/configcli/configcli.go
index b7f8de0..35afa47 100644
--- a/internal/configcli/configcli.go
+++ b/internal/configcli/configcli.go
@@ -22,6 +22,7 @@ import (
 	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/output"
 	"github.com/Knatte18/loomyard/internal/weftcli"
+	"github.com/Knatte18/loomyard/internal/yamlengine"
 )
 
 // syncFunc runs the post-edit sync, writing its output to the given writer,
@@ -132,15 +133,94 @@ func editOne(baseDir string, out io.Writer, module string, edit configengine.Edi
 	return output.Err(out, fmt.Sprintf("edited _lyx/config/%s.yaml but weft sync failed: %s", module, buf.String()))
 }
 
+// parseSetFlags parses a list of raw "key=value" strings (as collected from
+// repeated --set flags) into yamlengine.KV pairs.
+//
+// Each entry is split on the first '=' only, so a value may itself contain
+// '=' characters without truncating. An entry with no '=' at all is
+// malformed and returns a descriptive error rather than silently treating
+// the whole string as a key with an empty value.
+func parseSetFlags(raw []string) ([]yamlengine.KV, error) {
+	pairs := make([]yamlengine.KV, 0, len(raw))
+	for _, entry := range raw {
+		parts := strings.SplitN(entry, "=", 2)
+		if len(parts) < 2 {
+			return nil, fmt.Errorf("invalid --set value %q: expected key=value", entry)
+		}
+		pairs = append(pairs, yamlengine.KV{Key: parts[0], Value: parts[1]})
+	}
+	return pairs, nil
+}
+
+// setModule writes pairs into a single config module's file and optionally
+// syncs on success, mirroring editOne's structure but with no editor
+// invocation: configengine.Set performs the whole non-interactive write in
+// one call.
+//
+// Flow:
+// 1. Look up the template for the given module name via configreg.Template.
+// 2. If unknown, print an error message listing known modules and return 1.
+// 3. Call configengine.Set to scaffold-if-missing and apply pairs.
+// 4. If Set returns an error (e.g. an unknown config key), print it and return 1.
+// 5. On success, call sync with a buffered writer to capture its output.
+// 6. If sync returns 0, discard the buffer and print the success message.
+// 7. If sync returns non-zero, print a failure message with the sync output and return 1.
+func setModule(baseDir string, out io.Writer, module string, pairs []yamlengine.KV, sync syncFunc) int {
+	// Look up the template for this module.
+	template, ok := configreg.Template(module)
+	if !ok {
+		return output.Err(out, fmt.Sprintf("unknown config module: %s (known: %v)", module, configreg.Names()))
+	}
+
+	// Call configengine.Set to scaffold-if-missing and apply pairs directly,
+	// with no editor invocation.
+	if err := configengine.Set(baseDir, module, template(), pairs); err != nil {
+		return output.Err(out, err.Error())
+	}
+
+	// Set succeeded; now call sync and capture its output, exactly as editOne does.
+	var buf bytes.Buffer
+	exitCode := sync(&buf)
+	if exitCode == 0 {
+		// Sync succeeded; discard output to keep the stream clean.
+		fmt.Fprintf(out, "edited and synced _lyx/config/%s.yaml\n", module)
+		return 0
+	}
+
+	// Sync failed; include its output in the failure message for diagnosis.
+	return output.Err(out, fmt.Sprintf("edited _lyx/config/%s.yaml but weft sync failed: %s", module, buf.String()))
+}
+
 // dispatch routes the config command to the print path (when printOnly is true),
-// editOne (if a module is specified), or menu (for the interactive numbered menu).
+// the --set path (when setFlags is non-empty), editOne (if a module is
+// specified), or menu (for the interactive numbered menu).
 //
 // When printOnly is true the command is read-only: it writes on-disk YAML to out
 // without opening an editor. The print path is evaluated before any edit/menu logic.
-// The baseDir is computed from the layout as filepath.Join(WorktreeRoot, RelPath).
-func dispatch(l *hubgeometry.Layout, in io.Reader, out io.Writer, args []string, edit configengine.EditorFunc, sync syncFunc, printOnly bool) int {
+// The --set path is a fully non-interactive write: it never calls edit and is
+// mutually exclusive with --print. The baseDir is computed from the layout as
+// filepath.Join(WorktreeRoot, RelPath).
+func dispatch(l *hubgeometry.Layout, in io.Reader, out io.Writer, args []string, edit configengine.EditorFunc, sync syncFunc, printOnly bool, setFlags []string) int {
 	baseDir := filepath.Join(l.WorktreeRoot, l.RelPath)
 
+	// Handle --set before any --print/edit/menu dispatch: it is a fully
+	// non-interactive write path that never opens the editor, so its
+	// validation (mutual exclusivity with --print, module-required) must run
+	// before either of those branches gets a chance to act.
+	if len(setFlags) > 0 && printOnly {
+		return output.Err(out, "--print and --set are mutually exclusive")
+	}
+	if len(setFlags) > 0 && len(args) < 1 {
+		return output.Err(out, "module required with --set")
+	}
+	if len(setFlags) > 0 {
+		pairs, err := parseSetFlags(setFlags)
+		if err != nil {
+			return output.Err(out, err.Error())
+		}
+		return setModule(baseDir, out, args[0], pairs, sync)
+	}
+
 	// Handle --print before any edit/menu dispatch; the print path is read-only
 	// and never opens the editor.
 	if printOnly {
@@ -162,8 +242,13 @@ func dispatch(l *hubgeometry.Layout, in io.Reader, out io.Writer, args []string,
 func buildConfigLong() string {
 	return "config edits a module's configuration in _lyx/config/ and syncs weft on\n" +
 		"success. With no argument it opens an interactive numbered menu of the known\n" +
-		"modules; with a module name it edits that module directly.\n\n" +
+		"modules; with a module name it edits that module directly. The editor is\n" +
+		"resolved from $VISUAL or $EDITOR; with neither set it falls back to notepad\n" +
+		"on Windows or vi elsewhere.\n\n" +
 		"Use --print to print the on-disk YAML without launching the editor.\n\n" +
+		"Use --set key=value (repeatable) to write one or more config values directly,\n" +
+		"bypassing the editor entirely, e.g.\n" +
+		"  lyx config board --set proposal_prefix=foo- --set home=Home.md\n\n" +
 		"Known modules: " + strings.Join(configreg.Names(), ", ") + "."
 }
 
@@ -232,11 +317,13 @@ func Command() *cobra.Command {
 		ValidArgs: configreg.Names(),
 	}
 	configCmd.Flags().Bool("print", false, "print on-disk config as YAML without launching the editor")
-	// The RunE closure captures configCmd so the --print flag is readable without
-	// consulting os.Args directly.
+	configCmd.Flags().StringArray("set", nil, "set config key=value directly, bypassing the editor (repeatable)")
+	// The RunE closure captures configCmd so the --print/--set flags are
+	// readable without consulting os.Args directly.
 	configCmd.RunE = clihelp.WrapRun(func(out io.Writer, args []string) int {
 		printOnly, _ := configCmd.Flags().GetBool("print")
-		return runConfig(out, args, printOnly)
+		setFlags, _ := configCmd.Flags().GetStringArray("set")
+		return runConfig(out, args, printOnly, setFlags)
 	})
 
 	// Build the reconcile subcommand and register it so cobra routes
@@ -274,8 +361,9 @@ func RunCLI(out io.Writer, args []string) int {
 // editor (DefaultEditor) and the real sync function (weft.RunCLI with "sync"),
 // and dispatches to dispatch with os.Stdin as the interactive input reader.
 // When printOnly is true the command is read-only: it prints on-disk YAML
-// without opening an editor or running sync.
-func runConfig(out io.Writer, args []string, printOnly bool) int {
+// without opening an editor or running sync. setFlags carries the raw
+// "key=value" strings collected from repeated --set flags.
+func runConfig(out io.Writer, args []string, printOnly bool, setFlags []string) int {
 	// Resolve the current working directory.
 	cwd, err := hubgeometry.Getwd()
 	if err != nil {
@@ -293,6 +381,6 @@ func runConfig(out io.Writer, args []string, printOnly bool) int {
 		return weftcli.RunCLI(w, []string{"sync"})
 	}
 
-	// Dispatch to the print path, interactive menu, or specific module.
-	return dispatch(l, os.Stdin, out, args, configengine.DefaultEditor, realSync, printOnly)
+	// Dispatch to the print path, --set path, interactive menu, or specific module.
+	return dispatch(l, os.Stdin, out, args, configengine.DefaultEditor, realSync, printOnly, setFlags)
 }
diff --git a/internal/configcli/configcli_integration_test.go b/internal/configcli/configcli_integration_test.go
index 31de118..c4b8889 100644
--- a/internal/configcli/configcli_integration_test.go
+++ b/internal/configcli/configcli_integration_test.go
@@ -80,7 +80,7 @@ func TestE2ESyncIntegration(t *testing.T) {
 
 	// Run dispatch with the fake editor and injected sync.
 	var out bytes.Buffer
-	code := dispatch(hostLayout, os.Stdin, &out, []string{"warp"}, fakeEdit, injectedSync, false)
+	code := dispatch(hostLayout, os.Stdin, &out, []string{"warp"}, fakeEdit, injectedSync, false, nil)
 
 	// Assert dispatch succeeded.
 	if code != 0 {
diff --git a/internal/configcli/configcli_test.go b/internal/configcli/configcli_test.go
index fe5bfaa..cec578c 100644
--- a/internal/configcli/configcli_test.go
+++ b/internal/configcli/configcli_test.go
@@ -396,7 +396,7 @@ func TestPrintModule_Seeded(t *testing.T) {
 
 	l := makeLayoutAt(baseDir)
 	var out bytes.Buffer
-	code := dispatch(l, nil, &out, []string{"warp"}, makeNeverCalledEditor(t), nil, true)
+	code := dispatch(l, nil, &out, []string{"warp"}, makeNeverCalledEditor(t), nil, true, nil)
 
 	if code != 0 {
 		t.Errorf("dispatch(print=true, seeded) = %d; want 0; output: %q", code, out.String())
@@ -417,7 +417,7 @@ func TestPrintModule_KnownButUnseeded(t *testing.T) {
 
 	l := makeLayoutAt(baseDir)
 	var out bytes.Buffer
-	code := dispatch(l, nil, &out, []string{"warp"}, makeNeverCalledEditor(t), nil, true)
+	code := dispatch(l, nil, &out, []string{"warp"}, makeNeverCalledEditor(t), nil, true, nil)
 
 	if code != 1 {
 		t.Errorf("dispatch(print=true, unseeded) = %d; want 1", code)
@@ -436,7 +436,7 @@ func TestPrintAggregate_PartialSeed(t *testing.T) {
 
 	l := makeLayoutAt(baseDir)
 	var out bytes.Buffer
-	code := dispatch(l, nil, &out, nil, makeNeverCalledEditor(t), nil, true)
+	code := dispatch(l, nil, &out, nil, makeNeverCalledEditor(t), nil, true, nil)
 
 	if code != 0 {
 		t.Errorf("dispatch(print=true, aggregate) = %d; want 0; output: %q", code, out.String())
@@ -465,7 +465,7 @@ func TestPrintUnknownModule(t *testing.T) {
 	baseDir := t.TempDir()
 	l := makeLayoutAt(baseDir)
 	var out bytes.Buffer
-	code := dispatch(l, nil, &out, []string{"bogus"}, makeNeverCalledEditor(t), nil, true)
+	code := dispatch(l, nil, &out, []string{"bogus"}, makeNeverCalledEditor(t), nil, true, nil)
 
 	if code != 1 {
 		t.Errorf("dispatch(print=true, unknown) = %d; want 1", code)
@@ -484,3 +484,145 @@ func TestConfigLong_ContainsModuleNames(t *testing.T) {
 		}
 	}
 }
+
+// countingEditor returns a configengine.EditorFunc that increments *calls
+// every time it is invoked, so tests can assert the --set path never opens
+// the editor by asserting the counter stays at 0.
+func countingEditor(calls *int) configengine.EditorFunc {
+	return func(path string) error {
+		*calls++
+		return nil
+	}
+}
+
+// TestDispatchSet_NeverInvokesEditor verifies that a successful --set
+// invocation never calls the injected EditorFunc.
+func TestDispatchSet_NeverInvokesEditor(t *testing.T) {
+	baseDir := t.TempDir()
+	seedModuleConfig(t, baseDir, "warp", "branch_prefix: old-\n")
+
+	l := makeLayoutAt(baseDir)
+	var out bytes.Buffer
+	editorCalls := 0
+	tracker := &fakeSyncTracker{exitCode: 0}
+	code := dispatch(l, nil, &out, []string{"warp"}, countingEditor(&editorCalls), tracker.syncFunc(), false, []string{"branch_prefix=new-"})
+
+	if code != 0 {
+		t.Errorf("dispatch(--set) = %d; want 0; output: %q", code, out.String())
+	}
+	if editorCalls != 0 {
+		t.Errorf("dispatch(--set) invoked the editor %d times; want 0", editorCalls)
+	}
+}
+
+// TestDispatchSet_UnknownKeyNeverSyncs verifies that an unknown key passed to
+// --set returns an error and the injected sync function is never invoked.
+func TestDispatchSet_UnknownKeyNeverSyncs(t *testing.T) {
+	baseDir := t.TempDir()
+	seedModuleConfig(t, baseDir, "warp", "branch_prefix: old-\n")
+
+	l := makeLayoutAt(baseDir)
+	var out bytes.Buffer
+	editorCalls := 0
+	tracker := &fakeSyncTracker{exitCode: 0}
+	code := dispatch(l, nil, &out, []string{"warp"}, countingEditor(&editorCalls), tracker.syncFunc(), false, []string{"bogus_key=x"})
+
+	if code != 1 {
+		t.Errorf("dispatch(--set unknown key) = %d; want 1", code)
+	}
+	if tracker.called {
+		t.Error("sync should not be called when --set names an unknown key")
+	}
+	assertJSONErrContains(t, out.String(), "unknown config key")
+}
+
+// TestDispatchSet_PrintMutuallyExclusive verifies that passing both --print
+// and --set returns the mutual-exclusivity error, with neither the editor
+// nor sync invoked.
+func TestDispatchSet_PrintMutuallyExclusive(t *testing.T) {
+	baseDir := t.TempDir()
+	l := makeLayoutAt(baseDir)
+	var out bytes.Buffer
+	editorCalls := 0
+	tracker := &fakeSyncTracker{exitCode: 0}
+	code := dispatch(l, nil, &out, []string{"warp"}, countingEditor(&editorCalls), tracker.syncFunc(), true, []string{"branch_prefix=new-"})
+
+	if code != 1 {
+		t.Errorf("dispatch(--print, --set) = %d; want 1", code)
+	}
+	if editorCalls != 0 {
+		t.Errorf("dispatch(--print, --set) invoked the editor %d times; want 0", editorCalls)
+	}
+	if tracker.called {
+		t.Error("sync should not be called when --print and --set are both set")
+	}
+	assertJSONErrContains(t, out.String(), "mutually exclusive")
+}
+
+// TestDispatchSet_NoModuleRequiresOne verifies that --set with no module
+// positional returns the module-required error.
+func TestDispatchSet_NoModuleRequiresOne(t *testing.T) {
+	baseDir := t.TempDir()
+	l := makeLayoutAt(baseDir)
+	var out bytes.Buffer
+	tracker := &fakeSyncTracker{exitCode: 0}
+	code := dispatch(l, nil, &out, nil, makeNeverCalledEditor(t), tracker.syncFunc(), false, []string{"branch_prefix=new-"})
+
+	if code != 1 {
+		t.Errorf("dispatch(--set, no module) = %d; want 1", code)
+	}
+	assertJSONErrContains(t, out.String(), "module required with --set")
+}
+
+// TestDispatchSet_MultipleValuesOneSync verifies that multiple --set values
+// in one dispatch() call all land in a single sync invocation.
+func TestDispatchSet_MultipleValuesOneSync(t *testing.T) {
+	baseDir := t.TempDir()
+	seedModuleConfig(t, baseDir, "warp", "branch_prefix: old-\n")
+
+	l := makeLayoutAt(baseDir)
+	var out bytes.Buffer
+	syncCalls := 0
+	sync := func(w io.Writer) int {
+		syncCalls++
+		return 0
+	}
+	code := dispatch(l, nil, &out, []string{"warp"}, makeNeverCalledEditor(t), sync, false, []string{"branch_prefix=new-"})
+
+	if code != 0 {
+		t.Errorf("dispatch(--set multiple) = %d; want 0; output: %q", code, out.String())
+	}
+	if syncCalls != 1 {
+		t.Errorf("dispatch(--set multiple) called sync %d times; want 1", syncCalls)
+	}
+}
+
+// TestDispatchSet_MalformedValue verifies that a malformed --set value with
+// no '=' returns the parseSetFlags error.
+func TestDispatchSet_MalformedValue(t *testing.T) {
+	baseDir := t.TempDir()
+	l := makeLayoutAt(baseDir)
+	var out bytes.Buffer
+	tracker := &fakeSyncTracker{exitCode: 0}
+	code := dispatch(l, nil, &out, []string{"warp"}, makeNeverCalledEditor(t), tracker.syncFunc(), false, []string{"no-equals-sign"})
+
+	if code != 1 {
+		t.Errorf("dispatch(--set malformed) = %d; want 1", code)
+	}
+	if tracker.called {
+		t.Error("sync should not be called for a malformed --set value")
+	}
+	assertJSONErrContains(t, out.String(), "expected key=value")
+}
+
+// TestConfigLong_MentionsEditorFallbackAndSet verifies that buildConfigLong's
+// output documents both the EDITOR/VISUAL editor fallback and the --set flag.
+func TestConfigLong_MentionsEditorFallbackAndSet(t *testing.T) {
+	longText := buildConfigLong()
+	if !strings.Contains(longText, "EDITOR") || !strings.Contains(longText, "VISUAL") {
+		t.Errorf("config Long missing EDITOR/VISUAL fallback documentation; Long = %q", longText)
+	}
+	if !strings.Contains(longText, "--set") {
+		t.Errorf("config Long missing --set documentation; Long = %q", longText)
+	}
+}
diff --git a/internal/configengine/edit.go b/internal/configengine/edit.go
index 7867502..f81fdfb 100644
--- a/internal/configengine/edit.go
+++ b/internal/configengine/edit.go
@@ -53,6 +53,33 @@ func DefaultEditor(path string) error {
 	return cmd.Run()
 }
 
+// scaffoldIfMissing writes template to path (creating configDir first) when
+// path does not yet exist, and reports whether it did so.
+//
+// It is shared by Edit and Set so both entry points scaffold a missing config
+// file identically: a caller that scaffolds must know so it can remove the
+// fresh file on any later abort, restoring the pre-call filesystem state.
+func scaffoldIfMissing(path, configDir, template string) (scaffolded bool, err error) {
+	// Check if the file already exists.
+	_, statErr := os.Stat(path)
+	scaffolded = os.IsNotExist(statErr)
+	if !scaffolded {
+		return false, nil
+	}
+
+	// Create _lyx/config/ directory if needed.
+	if err := os.MkdirAll(configDir, 0755); err != nil {
+		return false, fmt.Errorf("create config directory: %w", err)
+	}
+
+	// Write the template to the new file.
+	if err := os.WriteFile(path, []byte(template), 0o644); err != nil {
+		return false, fmt.Errorf("scaffold config file: %w", err)
+	}
+
+	return true, nil
+}
+
 // Edit opens a config file in an editor, validates the YAML syntax, and loops
 // on validation failure.
 //
@@ -85,22 +112,11 @@ func Edit(baseDir, module, template string, edit EditorFunc) error {
 	// Compute the config file path via paths helper.
 	path := hubgeometry.ConfigFile(baseDir, module)
 
-	// Check if the file already exists.
-	_, err = os.Stat(path)
-	scaffolded := os.IsNotExist(err)
-
-	// If the file does not exist, scaffold it from the template.
-	if scaffolded {
-		// Create _lyx/config/ directory if needed.
-		configDir := hubgeometry.ConfigDir(baseDir)
-		if err := os.MkdirAll(configDir, 0755); err != nil {
-			return fmt.Errorf("create config directory: %w", err)
-		}
-
-		// Write the template to the new file.
-		if err := os.WriteFile(path, []byte(template), 0o644); err != nil {
-			return fmt.Errorf("scaffold config file: %w", err)
-		}
+	// Scaffold the file from the template if it does not already exist.
+	configDir := hubgeometry.ConfigDir(baseDir)
+	scaffolded, err := scaffoldIfMissing(path, configDir, template)
+	if err != nil {
+		return err
 	}
 
 	// Loop until valid YAML is saved or edit is aborted.
diff --git a/internal/configengine/set.go b/internal/configengine/set.go
new file mode 100644
index 0000000..83184e6
--- /dev/null
+++ b/internal/configengine/set.go
@@ -0,0 +1,72 @@
+// set.go implements the non-interactive `lyx config <module> --set key=value`
+// write path: scaffold-if-missing plus a single yamlengine.SetValues mutation,
+// with no editor invocation and no validation loop. It shares scaffoldIfMissing
+// with Edit so both entry points create and roll back a fresh default-valued
+// file identically.
+
+package configengine
+
+import (
+	"fmt"
+	"os"
+	"strings"
+
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
+	"github.com/Knatte18/loomyard/internal/yamlengine"
+)
+
+// Set writes pairs into module's config file under baseDir, scaffolding the
+// file from template first if it does not yet exist.
+//
+// Unlike Edit, Set never opens an editor and never loops on validation
+// failure: it is the fully non-interactive counterpart used by the --set CLI
+// flag. Every error path removes a freshly-scaffolded file before returning,
+// mirroring Edit's abort-removes-scaffold contract, so a failed --set never
+// leaves a fresh default-valued file behind on disk.
+func Set(baseDir, module, template string, pairs []yamlengine.KV) error {
+	// Check that baseDir is initialized.
+	if _, err := FindBaseDir(baseDir); err != nil {
+		return err
+	}
+
+	path := hubgeometry.ConfigFile(baseDir, module)
+	configDir := hubgeometry.ConfigDir(baseDir)
+
+	scaffolded, err := scaffoldIfMissing(path, configDir, template)
+	if err != nil {
+		return err
+	}
+
+	// removeIfScaffolded restores the pre-call filesystem state on any later
+	// failure, exactly as Edit's abort path does: a failed --set must never
+	// leave a fresh default-valued file behind.
+	removeIfScaffolded := func() {
+		if scaffolded {
+			_ = os.Remove(path)
+		}
+	}
+
+	existingBytes, err := os.ReadFile(path)
+	if err != nil {
+		removeIfScaffolded()
+		return err
+	}
+
+	result, err := yamlengine.SetValues([]byte(template), existingBytes, pairs)
+	if err != nil {
+		removeIfScaffolded()
+		return err
+	}
+
+	if len(result.Unknown) > 0 {
+		removeIfScaffolded()
+		return fmt.Errorf("unknown config key(s): %s (known: %s)", strings.Join(result.Unknown, ", "), strings.Join(result.Known, ", "))
+	}
+
+	if err := os.WriteFile(path, result.Merged, 0o644); err != nil {
+		removeIfScaffolded()
+		return err
+	}
+
+	return nil
+}
diff --git a/internal/configengine/set_test.go b/internal/configengine/set_test.go
new file mode 100644
index 0000000..185e320
--- /dev/null
+++ b/internal/configengine/set_test.go
@@ -0,0 +1,149 @@
+// set_test.go — unit tests for the non-interactive Set entry point (set.go).
+//
+// Tests cover: scaffold-then-set when the config file is missing, rollback
+// of a freshly-scaffolded file on an unknown key, byte-for-byte preservation
+// of a pre-existing file on an unknown key, and preservation of untouched
+// keys when setting one key on an existing multi-key file.
+
+package configengine_test
+
+import (
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/configengine"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
+	"github.com/Knatte18/loomyard/internal/yamlengine"
+)
+
+// TestSet_ScaffoldWhenMissingThenSet mirrors TestEdit_ScaffoldWhenMissing's
+// fixture setup: calling Set against a baseDir with no existing config file
+// creates it from template and applies the requested pairs in one call.
+func TestSet_ScaffoldWhenMissingThenSet(t *testing.T) {
+	tmpDir := t.TempDir()
+
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
+	if err := os.Mkdir(lyxDir, 0755); err != nil {
+		t.Fatalf("failed to create _lyx: %v", err)
+	}
+
+	template := "key1: value1\nkey2: value2\n"
+	err := configengine.Set(tmpDir, "testmod", template, []yamlengine.KV{{Key: "key1", Value: "set1"}})
+	if err != nil {
+		t.Fatalf("Set() = %v; want nil", err)
+	}
+
+	path := hubgeometry.ConfigFile(tmpDir, "testmod")
+	data, err := os.ReadFile(path)
+	if err != nil {
+		t.Fatalf("config file not found at %s: %v", path, err)
+	}
+	if !strings.Contains(string(data), "key1: set1") {
+		t.Errorf("Set() file = %q; want key1: set1", string(data))
+	}
+	if !strings.Contains(string(data), "key2: value2") {
+		t.Errorf("Set() file = %q; want key2: value2 (untouched template default)", string(data))
+	}
+}
+
+// TestSet_UnknownKeyRemovesScaffoldedFile verifies that an unknown key against
+// a freshly-missing file removes the just-scaffolded file and returns a
+// non-nil error mentioning the unknown key.
+func TestSet_UnknownKeyRemovesScaffoldedFile(t *testing.T) {
+	tmpDir := t.TempDir()
+
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
+	if err := os.Mkdir(lyxDir, 0755); err != nil {
+		t.Fatalf("failed to create _lyx: %v", err)
+	}
+
+	template := "key1: value1\n"
+	err := configengine.Set(tmpDir, "testmod", template, []yamlengine.KV{{Key: "bogus", Value: "x"}})
+	if err == nil {
+		t.Fatalf("Set() = nil; want error for unknown key")
+	}
+	if !strings.Contains(err.Error(), "bogus") {
+		t.Errorf("Set() error = %v; want it to mention the unknown key", err)
+	}
+
+	path := hubgeometry.ConfigFile(tmpDir, "testmod")
+	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
+		t.Errorf("config file still exists after unknown-key rejection; should have been removed")
+	}
+}
+
+// TestSet_UnknownKeyLeavesExistingFileUnchanged verifies that an unknown key
+// against a pre-existing file leaves that file byte-for-byte unchanged and
+// returns a non-nil error.
+func TestSet_UnknownKeyLeavesExistingFileUnchanged(t *testing.T) {
+	tmpDir := t.TempDir()
+
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
+	if err := os.Mkdir(lyxDir, 0755); err != nil {
+		t.Fatalf("failed to create _lyx: %v", err)
+	}
+	configDir := hubgeometry.ConfigDir(tmpDir)
+	if err := os.Mkdir(configDir, 0755); err != nil {
+		t.Fatalf("failed to create _lyx/config: %v", err)
+	}
+
+	path := hubgeometry.ConfigFile(tmpDir, "testmod")
+	originalContent := "key1: original_value\n"
+	if err := os.WriteFile(path, []byte(originalContent), 0644); err != nil {
+		t.Fatalf("failed to write config file: %v", err)
+	}
+
+	template := "key1: default1\n"
+	err := configengine.Set(tmpDir, "testmod", template, []yamlengine.KV{{Key: "bogus", Value: "x"}})
+	if err == nil {
+		t.Fatalf("Set() = nil; want error for unknown key")
+	}
+
+	finalBytes, readErr := os.ReadFile(path)
+	if readErr != nil {
+		t.Fatalf("failed to read config file: %v", readErr)
+	}
+	if string(finalBytes) != originalContent {
+		t.Errorf("Set() left file = %q; want unchanged %q", string(finalBytes), originalContent)
+	}
+}
+
+// TestSet_PreservesOtherKeysOnExistingFile verifies that setting one key on
+// an existing multi-key file preserves the other keys' values.
+func TestSet_PreservesOtherKeysOnExistingFile(t *testing.T) {
+	tmpDir := t.TempDir()
+
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
+	if err := os.Mkdir(lyxDir, 0755); err != nil {
+		t.Fatalf("failed to create _lyx: %v", err)
+	}
+	configDir := hubgeometry.ConfigDir(tmpDir)
+	if err := os.Mkdir(configDir, 0755); err != nil {
+		t.Fatalf("failed to create _lyx/config: %v", err)
+	}
+
+	path := hubgeometry.ConfigFile(tmpDir, "testmod")
+	originalContent := "key1: original_value1\nkey2: original_value2\n"
+	if err := os.WriteFile(path, []byte(originalContent), 0644); err != nil {
+		t.Fatalf("failed to write config file: %v", err)
+	}
+
+	template := "key1: default1\nkey2: default2\n"
+	err := configengine.Set(tmpDir, "testmod", template, []yamlengine.KV{{Key: "key1", Value: "new_value1"}})
+	if err != nil {
+		t.Fatalf("Set() = %v; want nil", err)
+	}
+
+	finalBytes, readErr := os.ReadFile(path)
+	if readErr != nil {
+		t.Fatalf("failed to read config file: %v", readErr)
+	}
+	if !strings.Contains(string(finalBytes), "key1: new_value1") {
+		t.Errorf("Set() file = %q; want key1: new_value1", string(finalBytes))
+	}
+	if !strings.Contains(string(finalBytes), "key2: original_value2") {
+		t.Errorf("Set() file = %q; want key2: original_value2 (untouched)", string(finalBytes))
+	}
+}
diff --git a/internal/hubgeometry/worktreelist.go b/internal/hubgeometry/worktreelist.go
index 8943f4a..53677eb 100644
--- a/internal/hubgeometry/worktreelist.go
+++ b/internal/hubgeometry/worktreelist.go
@@ -32,12 +32,12 @@ type WorktreeEntry struct {
 //
 // Returns WorktreeEntry slice or an error if parsing or git execution fails.
 func List(sourceDir string) ([]WorktreeEntry, error) {
-	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"worktree", "list", "--porcelain"}, sourceDir)
+	stdout, _, exitCode, err := gitexec.RunGit([]string{"worktree", "list", "--porcelain"}, sourceDir)
 	if err != nil {
 		return nil, err
 	}
 	if exitCode != 0 {
-		return nil, fmt.Errorf("git worktree list failed: %s", stderr)
+		return nil, fmt.Errorf("list git worktrees in %q failed (git exit %d)", sourceDir, exitCode)
 	}
 
 	return parseWorktreePorcelain(stdout)
diff --git a/internal/hubgeometry/worktreelist_test.go b/internal/hubgeometry/worktreelist_test.go
index a0ba637..4240e49 100644
--- a/internal/hubgeometry/worktreelist_test.go
+++ b/internal/hubgeometry/worktreelist_test.go
@@ -116,3 +116,27 @@ func TestList(t *testing.T) {
 		})
 	}
 }
+
+// TestList_NotAGitRepo asserts that calling List against a directory that is not
+// inside any git repository fails with an error composed from local context (the
+// source directory and git's exit code), not git's raw stderr text.
+func TestList_NotAGitRepo(t *testing.T) {
+	t.Parallel()
+
+	notARepo := t.TempDir()
+
+	entries, err := hubgeometry.List(notARepo)
+	if err == nil {
+		t.Fatalf("List(%q) error = nil; want error (not a git repository)", notARepo)
+	}
+	wantSubstr := fmt.Sprintf("%q", notARepo)
+	if !strings.Contains(err.Error(), wantSubstr) {
+		t.Errorf("List(%q) error = %q; want substring %q (source dir)", notARepo, err.Error(), wantSubstr)
+	}
+	if strings.Contains(err.Error(), "fatal:") {
+		t.Errorf("List(%q) error = %q; want no %q substring (raw git stderr leak)", notARepo, err.Error(), "fatal:")
+	}
+	if entries != nil {
+		t.Errorf("List(%q) entries = %v; want nil", notARepo, entries)
+	}
+}
diff --git a/internal/warpengine/add.go b/internal/warpengine/add.go
index 1bd7391..74fa8f7 100644
--- a/internal/warpengine/add.go
+++ b/internal/warpengine/add.go
@@ -74,7 +74,7 @@ type AddResult struct {
 // Returns AddResult on success or an error if any step fails.
 func (w *Worktree) Add(l *hubgeometry.Layout, slug string, opts AddOptions) (AddResult, error) {
 	// (1) Clean check
-	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain", "--untracked-files=no"}, l.WorktreeRoot)
+	stdout, _, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain", "--untracked-files=no"}, l.WorktreeRoot)
 	if err != nil {
 		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
 	}
@@ -139,12 +139,12 @@ func (w *Worktree) Add(l *hubgeometry.Layout, slug string, opts AddOptions) (Add
 	parentBranch := strings.TrimSpace(stdout)
 
 	// (7) Create host worktree
-	_, stderr, exitCode, err = gitexec.RunGit([]string{"worktree", "add", "-b", branch, target}, l.WorktreeRoot)
+	_, _, exitCode, err = gitexec.RunGit([]string{"worktree", "add", "-b", branch, target}, l.WorktreeRoot)
 	if err != nil {
 		return AddResult{}, fmt.Errorf("cwd is not a valid git worktree")
 	}
 	if exitCode != 0 {
-		return AddResult{}, fmt.Errorf("worktree add failed: %s", stderr)
+		return AddResult{}, fmt.Errorf("create worktree %q for branch %q failed (git exit %d)", target, branch, exitCode)
 	}
 
 	// Install the post-checkout hook now that the host worktree exists.
@@ -159,7 +159,7 @@ func (w *Worktree) Add(l *hubgeometry.Layout, slug string, opts AddOptions) (Add
 	weftPath := l.WeftWorktreePath(slug)
 	if weftBranchAlreadyExists {
 		// Adopt: git worktree add <path> <branch> (no -b, branch exists)
-		_, stderr, exitCode, err := gitexec.RunGit(
+		_, _, exitCode, err := gitexec.RunGit(
 			[]string{"worktree", "add", weftPath, branch},
 			l.WeftRepoRoot(),
 		)
@@ -169,7 +169,7 @@ func (w *Worktree) Add(l *hubgeometry.Layout, slug string, opts AddOptions) (Add
 		}
 		if exitCode != 0 {
 			w.rollbackAdd(l, slug, branch, target)
-			return AddResult{}, fmt.Errorf("weft worktree add (adopt) failed: %s", stderr)
+			return AddResult{}, fmt.Errorf("adopt weft worktree for branch %q failed (git exit %d)", branch, exitCode)
 		}
 	} else {
 		// Create: git worktree add -b <branch> <path> <parentBranch> (fork from parent)
@@ -192,14 +192,14 @@ func (w *Worktree) Add(l *hubgeometry.Layout, slug string, opts AddOptions) (Add
 	}
 
 	// (11) Push host branch (LAST step for host)
-	_, stderr, exitCode, err = gitexec.RunGit([]string{"push", "-u", "origin", branch}, l.WorktreeRoot)
+	_, _, exitCode, err = gitexec.RunGit([]string{"push", "-u", "origin", branch}, l.WorktreeRoot)
 	if err != nil {
 		w.rollbackAdd(l, slug, branch, target)
 		return AddResult{}, fmt.Errorf("push: %w", err)
 	}
 	if exitCode != 0 {
 		w.rollbackAdd(l, slug, branch, target)
-		return AddResult{}, fmt.Errorf("push failed: %s", stderr)
+		return AddResult{}, fmt.Errorf("push branch %q failed (git exit %d)", branch, exitCode)
 	}
 
 	// (12) Push weft branch
diff --git a/internal/warpengine/add_test.go b/internal/warpengine/add_test.go
index 3cb060d..f501c0a 100644
--- a/internal/warpengine/add_test.go
+++ b/internal/warpengine/add_test.go
@@ -323,6 +323,50 @@ func TestAddRollback(t *testing.T) {
 	}
 }
 
+// TestAddAdoptWeftBranchLockedFails asserts that when Add attempts to adopt an
+// existing weft branch that is already checked out in another weft worktree, the
+// resulting error is composed from local context (the branch name and git exit
+// code) rather than git's own stderr text.
+func TestAddAdoptWeftBranchLockedFails(t *testing.T) {
+	t.Parallel()
+
+	const slug = "adopt-lock-test"
+	f := lyxtest.CopyPairedLocal(t)
+
+	// Create a weft branch ahead of time (outside Add) so Add routes to the adopt path.
+	weftBranch := slug
+	parentBranch := "main"
+	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "branch", weftBranch, parentBranch)
+
+	// Lock the weft branch by checking it out in a separate weft worktree. This causes
+	// the adopt-path `git worktree add <path> <branch>` inside Add to fail with
+	// "already checked out".
+	lockPath := filepath.Join(f.Layout.Hub, "lock-weft-adopt")
+	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "worktree", "add", lockPath, weftBranch)
+	t.Cleanup(func() {
+		_, _, _, _ = gitexec.RunGit([]string{"worktree", "remove", "--force", lockPath}, f.Layout.WeftRepoRoot())
+	})
+
+	w := New(Config{})
+	result, err := w.Add(f.Layout, slug, AddOptions{SkipPush: true})
+
+	if err == nil {
+		t.Fatalf("Add(%q) with locked weft branch error = nil; want adopt failure", slug)
+	}
+	if !strings.Contains(err.Error(), weftBranch) {
+		t.Errorf("Add(%q) error = %q; want substring %q (branch name)", slug, err.Error(), weftBranch)
+	}
+	if strings.Contains(err.Error(), "fatal:") {
+		t.Errorf("Add(%q) error = %q; want no %q substring (raw git stderr leak)", slug, err.Error(), "fatal:")
+	}
+	if strings.Contains(err.Error(), "already checked out") {
+		t.Errorf("Add(%q) error = %q; want no %q substring (raw git stderr leak)", slug, err.Error(), "already checked out")
+	}
+	if result.Slug != "" {
+		t.Errorf("Add(%q) result should be zero on error; got non-empty result", slug)
+	}
+}
+
 // TestAddAdoptExistingWeftBranch asserts that Add adopts an existing weft branch
 // instead of aborting with an error.
 func TestAddAdoptExistingWeftBranch(t *testing.T) {
diff --git a/internal/warpengine/checkout.go b/internal/warpengine/checkout.go
index befdc20..18a6368 100644
--- a/internal/warpengine/checkout.go
+++ b/internal/warpengine/checkout.go
@@ -77,7 +77,7 @@ func (w *Worktree) Checkout(l *hubgeometry.Layout, branch string) (CheckoutResul
 
 	// (3) Switch the host worktree to the target branch. Git propagates its own refusal
 	// (e.g., conflicting local changes) unchanged; we do not suppress it.
-	_, hostSwitchStderr, exitCode, err := gitexec.RunGit(
+	_, _, exitCode, err = gitexec.RunGit(
 		[]string{"switch", branch},
 		l.WorktreeRoot,
 	)
@@ -85,7 +85,7 @@ func (w *Worktree) Checkout(l *hubgeometry.Layout, branch string) (CheckoutResul
 		return CheckoutResult{}, fmt.Errorf("host switch: %w", err)
 	}
 	if exitCode != 0 {
-		return CheckoutResult{}, fmt.Errorf("host switch failed: %s", hostSwitchStderr)
+		return CheckoutResult{}, fmt.Errorf("host switch to branch %q failed (git exit %d)", branch, exitCode)
 	}
 
 	// (4) Resolve the weft sibling branch. On any failure, roll back the host switch.
@@ -123,7 +123,7 @@ func (w *Worktree) switchOrForkWeft(l *hubgeometry.Layout, branch string) error
 
 	if weftBranchExists(l, branch) {
 		// Branch already exists in the weft repo: switch the weft worktree to it.
-		_, stderr, exitCode, err := gitexec.RunGit(
+		_, _, exitCode, err := gitexec.RunGit(
 			[]string{"switch", branch},
 			weftWorktree,
 		)
@@ -131,7 +131,7 @@ func (w *Worktree) switchOrForkWeft(l *hubgeometry.Layout, branch string) error
 			return fmt.Errorf("weft switch: %w", err)
 		}
 		if exitCode != 0 {
-			return fmt.Errorf("weft switch failed: %s", stderr)
+			return fmt.Errorf("weft switch to branch %q failed (git exit %d)", branch, exitCode)
 		}
 		return nil
 	}
@@ -154,7 +154,7 @@ func (w *Worktree) switchOrForkWeft(l *hubgeometry.Layout, branch string) error
 
 	// Create the new weft branch forked from the parent weft branch and switch
 	// the weft worktree to it immediately via git switch -c.
-	_, stderr, exitCode, err := gitexec.RunGit(
+	_, _, exitCode, err = gitexec.RunGit(
 		[]string{"switch", "-c", branch, parentWeftBranch},
 		weftWorktree,
 	)
@@ -162,7 +162,7 @@ func (w *Worktree) switchOrForkWeft(l *hubgeometry.Layout, branch string) error
 		return fmt.Errorf("fork weft branch: %w", err)
 	}
 	if exitCode != 0 {
-		return fmt.Errorf("fork weft branch failed: %s", stderr)
+		return fmt.Errorf("fork weft branch %q from %q failed (git exit %d)", branch, parentWeftBranch, exitCode)
 	}
 
 	return nil
diff --git a/internal/warpengine/checkout_test.go b/internal/warpengine/checkout_test.go
index 6a24b3f..d86417c 100644
--- a/internal/warpengine/checkout_test.go
+++ b/internal/warpengine/checkout_test.go
@@ -210,6 +210,19 @@ func TestCheckout_HostRollback(t *testing.T) {
 		t.Fatalf("Checkout(%q) error = nil; want weft-side failure triggering rollback", targetBranch)
 	}
 
+	// The error message must be composed from local context (the branch name), not from
+	// git's own stderr text. Pin the absence of git-authored wording so a future edit
+	// cannot silently reintroduce a raw stderr leak.
+	if !strings.Contains(err.Error(), targetBranch) {
+		t.Errorf("Checkout(%q) error = %q; want substring %q (branch name)", targetBranch, err.Error(), targetBranch)
+	}
+	if strings.Contains(err.Error(), "fatal:") {
+		t.Errorf("Checkout(%q) error = %q; want no %q substring (raw git stderr leak)", targetBranch, err.Error(), "fatal:")
+	}
+	if strings.Contains(err.Error(), "already checked out") {
+		t.Errorf("Checkout(%q) error = %q; want no %q substring (raw git stderr leak)", targetBranch, err.Error(), "already checked out")
+	}
+
 	// The host must be rolled back to main — it must NOT be on target.
 	hostBranchOut, _, exitCode, err2 := gitexec.RunGit(
 		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
@@ -235,6 +248,30 @@ func TestCheckout_HostRollback(t *testing.T) {
 	}
 }
 
+// TestCheckout_HostSwitchNonexistentBranch asserts that Checkout on a branch name that
+// does not exist anywhere (host or weft) fails with an error composed from local context
+// (the branch name) rather than git's own stderr text.
+func TestCheckout_HostSwitchNonexistentBranch(t *testing.T) {
+	t.Parallel()
+
+	f := setupCheckoutFixture(t)
+
+	const targetBranch = "nonexistent-branch-xyz"
+
+	w := New(Config{})
+	_, err := w.Checkout(f.Layout, targetBranch)
+
+	if err == nil {
+		t.Fatalf("Checkout(%q) error = nil; want host-switch failure (branch does not exist)", targetBranch)
+	}
+	if !strings.Contains(err.Error(), targetBranch) {
+		t.Errorf("Checkout(%q) error = %q; want substring %q (branch name)", targetBranch, err.Error(), targetBranch)
+	}
+	if strings.Contains(err.Error(), "fatal:") {
+		t.Errorf("Checkout(%q) error = %q; want no %q substring (raw git stderr leak)", targetBranch, err.Error(), "fatal:")
+	}
+}
+
 // TestCheckout_UnmanagedBranch asserts that when the host switches to a branch that
 // has no corresponding weft branch, Checkout forks a new weft branch from the current
 // parent weft branch (matching the adopt-or-create fork-point of warp add).
diff --git a/internal/warpengine/cleanup.go b/internal/warpengine/cleanup.go
index 4201a1c..f3df1a1 100644
--- a/internal/warpengine/cleanup.go
+++ b/internal/warpengine/cleanup.go
@@ -149,7 +149,7 @@ func (w *Worktree) Cleanup(l *hubgeometry.Layout, apply, force bool) (CleanupRes
 // clean, newline-delimited list of branch names with no decoration. Returns an
 // error if the git command fails to spawn or exits non-zero.
 func listWeftBranches(l *hubgeometry.Layout) ([]string, error) {
-	out, stderr, exitCode, err := gitexec.RunGit(
+	out, _, exitCode, err := gitexec.RunGit(
 		[]string{"branch", "--format=%(refname:short)"},
 		l.WeftRepoRoot(),
 	)
@@ -157,7 +157,7 @@ func listWeftBranches(l *hubgeometry.Layout) ([]string, error) {
 		return nil, fmt.Errorf("git branch: %w", err)
 	}
 	if exitCode != 0 {
-		return nil, fmt.Errorf("git branch exited %d: %s", exitCode, stderr)
+		return nil, fmt.Errorf("list weft branches failed (git exit %d)", exitCode)
 	}
 
 	raw := strings.TrimSpace(out)
@@ -171,7 +171,7 @@ func listWeftBranches(l *hubgeometry.Layout) ([]string, error) {
 // deleteWeftBranch deletes a single weft branch via git branch -D and records
 // any error in entry.Error. Returns true only when the deletion succeeded.
 func deleteWeftBranch(l *hubgeometry.Layout, branch string, entry *CleanupBranchEntry) bool {
-	_, stderr, exitCode, err := gitexec.RunGit(
+	_, _, exitCode, err := gitexec.RunGit(
 		[]string{"branch", "-D", branch},
 		l.WeftRepoRoot(),
 	)
@@ -180,7 +180,7 @@ func deleteWeftBranch(l *hubgeometry.Layout, branch string, entry *CleanupBranch
 		return false
 	}
 	if exitCode != 0 {
-		entry.Error = fmt.Sprintf("git branch -D %s failed: %s", branch, stderr)
+		entry.Error = fmt.Sprintf("delete weft branch %q failed (git exit %d)", branch, exitCode)
 		return false
 	}
 	return true
diff --git a/internal/warpengine/cleanup_test.go b/internal/warpengine/cleanup_test.go
index b99820a..94bfb0d 100644
--- a/internal/warpengine/cleanup_test.go
+++ b/internal/warpengine/cleanup_test.go
@@ -9,6 +9,7 @@ package warpengine
 
 import (
 	"path/filepath"
+	"strings"
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/gitexec"
@@ -216,6 +217,60 @@ func TestCleanup_ApplyForceDeletesTaskBranch(t *testing.T) {
 	}
 }
 
+// TestCleanup_DeleteFailureNoStderrLeak asserts that when deleteWeftBranch fails
+// (the orphaned branch is locked by being checked out in another weft worktree),
+// entry.Error is composed from local context (the branch name and git exit code)
+// rather than git's own stderr text.
+func TestCleanup_DeleteFailureNoStderrLeak(t *testing.T) {
+	t.Parallel()
+
+	f := setupCleanupFixture(t)
+	orphanBranch := createOrphanWeftBranch(t, f, "orphan-delete-fails")
+
+	// Lock the orphan branch by checking it out in a separate weft worktree so that
+	// `git branch -D` on it fails (branch checked out elsewhere).
+	lockPath := filepath.Join(f.Layout.Hub, "lock-cleanup-orphan")
+	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "worktree", "add", lockPath, orphanBranch)
+	t.Cleanup(func() {
+		_, _, _, _ = gitexec.RunGit([]string{"worktree", "remove", "--force", lockPath}, f.Layout.WeftRepoRoot())
+	})
+
+	w := New(Config{})
+	r, err := w.Cleanup(f.Layout, true, true)
+	if err != nil {
+		t.Fatalf("Cleanup(apply=true, force=true) error = %v; want nil", err)
+	}
+
+	var found *CleanupBranchEntry
+	for i := range r.Entries {
+		if r.Entries[i].Branch == orphanBranch {
+			found = &r.Entries[i]
+			break
+		}
+	}
+	if found == nil {
+		t.Fatalf("Cleanup(apply=true, force=true): locked orphan branch %q not found in entries %+v", orphanBranch, r.Entries)
+	}
+
+	if found.Deleted {
+		t.Errorf("CleanupBranchEntry.Deleted = true for a locked branch; want false (delete should fail)")
+	}
+	if found.Error == "" {
+		t.Fatalf("CleanupBranchEntry.Error = \"\"; want non-empty (delete should fail)")
+	}
+	if !strings.Contains(found.Error, orphanBranch) {
+		t.Errorf("CleanupBranchEntry.Error = %q; want substring %q (branch name)", found.Error, orphanBranch)
+	}
+	if strings.Contains(found.Error, "fatal:") {
+		t.Errorf("CleanupBranchEntry.Error = %q; want no %q substring (raw git stderr leak)", found.Error, "fatal:")
+	}
+
+	// Branch must still exist — the deletion failed.
+	if !weftBranchExists(f.Layout, orphanBranch) {
+		t.Errorf("orphan branch %q was deleted despite lock; want intact", orphanBranch)
+	}
+}
+
 // TestCleanup_LiveBranchNeverDeleted asserts that weft branches with corresponding
 // live host worktrees are never reported or deleted by Cleanup. The test runs sequentially
 // on a shared fixture with both no-prefix and prefixed branch cases to preserve regression
diff --git a/internal/warpengine/clone.go b/internal/warpengine/clone.go
index ce99037..b50526b 100644
--- a/internal/warpengine/clone.go
+++ b/internal/warpengine/clone.go
@@ -124,13 +124,13 @@ func cloneRepo(url, dest string) error {
 	gitURL := filepath.ToSlash(url)
 	gitDest := filepath.ToSlash(destName)
 
-	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"clone", gitURL, gitDest}, parentDir)
+	stdout, _, exitCode, err := gitexec.RunGit([]string{"clone", gitURL, gitDest}, parentDir)
 	if err != nil {
 		return fmt.Errorf("clone failed: %w", err)
 	}
 
 	if exitCode != 0 {
-		return fmt.Errorf("clone failed: %s", stderr)
+		return fmt.Errorf("clone %q to %q failed (git exit %d)", url, dest, exitCode)
 	}
 
 	_ = stdout // stdout is not used; we only check for errors
diff --git a/internal/warpengine/clone_test.go b/internal/warpengine/clone_test.go
index dd8b301..36b33f0 100644
--- a/internal/warpengine/clone_test.go
+++ b/internal/warpengine/clone_test.go
@@ -1,8 +1,10 @@
-// clone_test.go — unit tests for URL-derivation helpers.
+// clone_test.go — unit tests for URL-derivation helpers and cloneRepo's error path.
 
 package warpengine
 
 import (
+	"path/filepath"
+	"strings"
 	"testing"
 )
 
@@ -54,6 +56,32 @@ func TestDeriveHostName(t *testing.T) {
 	}
 }
 
+// TestCloneRepo_InvalidURLFails asserts that cloneRepo's error on a bogus/nonexistent
+// source URL is composed from local context (the attempted URL and destination, plus
+// the git exit code) rather than git's own stderr text. No real git fixture is needed:
+// a nonexistent source path is enough to make `git clone` fail immediately.
+func TestCloneRepo_InvalidURLFails(t *testing.T) {
+	dest := filepath.Join(t.TempDir(), "cloned-repo")
+	const url = "/does/not/exist/nonexistent-repo.git"
+
+	err := cloneRepo(url, dest)
+	if err == nil {
+		t.Fatalf("cloneRepo(%q, %q) error = nil; want failure for a nonexistent source", url, dest)
+	}
+	if !strings.Contains(err.Error(), url) {
+		t.Errorf("cloneRepo(%q, %q) error = %q; want substring %q (attempted URL)", url, dest, err.Error(), url)
+	}
+	// Compare against filepath.Base(dest) rather than the raw dest string: %q escapes
+	// backslashes on Windows, so the literal OS-native dest path would never appear
+	// unescaped in err.Error() even though the destination is faithfully reported.
+	if destName := filepath.Base(dest); !strings.Contains(err.Error(), destName) {
+		t.Errorf("cloneRepo(%q, %q) error = %q; want substring %q (destination)", url, dest, err.Error(), destName)
+	}
+	if strings.Contains(err.Error(), "fatal:") {
+		t.Errorf("cloneRepo(%q, %q) error = %q; want no %q substring (raw git stderr leak)", url, dest, err.Error(), "fatal:")
+	}
+}
+
 func TestDeriveBoardURL(t *testing.T) {
 	tests := []struct {
 		name    string
diff --git a/internal/warpengine/junction.go b/internal/warpengine/junction.go
index a84333e..24c990d 100644
--- a/internal/warpengine/junction.go
+++ b/internal/warpengine/junction.go
@@ -302,7 +302,7 @@ func seedGitExclude(l *hubgeometry.Layout, slug string) error {
 	worktreePath := l.WorktreePath(slug)
 
 	// Get the exclude path via git rev-parse --git-path
-	stdout, stderr, exitCode, err := gitexec.RunGit(
+	stdout, _, exitCode, err := gitexec.RunGit(
 		[]string{"rev-parse", "--git-path", "info/exclude"},
 		worktreePath,
 	)
@@ -310,7 +310,7 @@ func seedGitExclude(l *hubgeometry.Layout, slug string) error {
 		return fmt.Errorf("failed to get git-path for info/exclude: %w", err)
 	}
 	if exitCode != 0 {
-		return fmt.Errorf("git rev-parse --git-path failed: %s", stderr)
+		return fmt.Errorf("resolve git exclude path for %q failed (git exit %d)", worktreePath, exitCode)
 	}
 
 	excludePath := strings.TrimSpace(stdout)
diff --git a/internal/warpengine/prune.go b/internal/warpengine/prune.go
index a042dae..85dac74 100644
--- a/internal/warpengine/prune.go
+++ b/internal/warpengine/prune.go
@@ -165,7 +165,7 @@ func (w *Worktree) Prune(l *hubgeometry.Layout, apply bool) (PruneResult, error)
 func removeStalePair(l *hubgeometry.Layout, weftPath string, pe *PruneEntry) bool {
 	// Attempt to remove via git worktree remove --force. We use --force because
 	// the host is already gone so the weft may have been left in a dirty state.
-	_, stderr, exitCode, err := gitexec.RunGit(
+	_, _, exitCode, err := gitexec.RunGit(
 		[]string{"worktree", "remove", "--force", weftPath},
 		l.WeftRepoRoot(),
 	)
@@ -176,7 +176,7 @@ func removeStalePair(l *hubgeometry.Layout, weftPath string, pe *PruneEntry) boo
 	if exitCode != 0 {
 		// git worktree remove --force failed; fall back to os.RemoveAll.
 		if removeErr := os.RemoveAll(weftPath); removeErr != nil {
-			pe.Error = fmt.Sprintf("git worktree remove failed (%s); fallback os.RemoveAll also failed: %v", stderr, removeErr)
+			pe.Error = fmt.Sprintf("remove weft worktree %q failed (git exit %d); fallback cleanup also failed: %v", weftPath, exitCode, removeErr)
 			return false
 		}
 	}
diff --git a/internal/warpengine/prune_test.go b/internal/warpengine/prune_test.go
index f8a9f86..3aa61f5 100644
--- a/internal/warpengine/prune_test.go
+++ b/internal/warpengine/prune_test.go
@@ -11,6 +11,7 @@ import (
 	"strings"
 	"testing"
 
+	"github.com/Knatte18/loomyard/internal/gitexec"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
 )
 
@@ -150,6 +151,82 @@ func TestPrune_StaleWeft(t *testing.T) {
 	}
 }
 
+// TestPrune_DoubleRemovalFailureNoStderrLeak asserts that when both the git-level
+// removal (`git worktree remove --force`) AND the os.RemoveAll fallback fail,
+// pe.Error is composed from local context (the weft path and git exit code) rather
+// than git's own stderr text. The git-level failure is forced by locking the weft
+// worktree (`git worktree lock`, which even --force cannot override); the fallback
+// failure is forced by holding an open file handle inside the weft worktree, which
+// blocks deletion on Windows.
+func TestPrune_DoubleRemovalFailureNoStderrLeak(t *testing.T) {
+	t.Parallel()
+
+	f := setupPruneFixture(t)
+
+	const testSlug = "feature-prune-double-fail"
+	w := New(Config{BranchPrefix: ""})
+	_, err := w.Add(f.Layout, testSlug, AddOptions{SkipGit: true})
+	if err != nil {
+		t.Fatalf("Add(%q): %v", testSlug, err)
+	}
+
+	weftPath := f.Layout.WeftWorktreePath(testSlug)
+	hostPath := f.Layout.WorktreePath(testSlug)
+
+	// Remove only the host worktree so the pair becomes stale/orphaned.
+	lyxtest.MustRun(t, f.Hub, "git", "worktree", "remove", "--force", hostPath)
+	lyxtest.MustRun(t, f.Hub, "git", "branch", "-D", testSlug)
+
+	// Force the git-level removal to fail: a locked worktree is refused even by
+	// `git worktree remove --force` (double -f or an explicit unlock is required).
+	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "worktree", "lock", weftPath, "--reason", "test-lock")
+
+	// Force the os.RemoveAll fallback to also fail: hold an open file handle inside
+	// the weft worktree so deletion is blocked (Windows refuses to unlink an
+	// open-for-read file out from under an active handle).
+	blockerPath := filepath.Join(weftPath, "prune-double-fail-blocker")
+	if err := os.WriteFile(blockerPath, []byte("blocker"), 0o644); err != nil {
+		t.Fatalf("write blocker file: %v", err)
+	}
+	blocker, err := os.Open(blockerPath)
+	if err != nil {
+		t.Fatalf("open blocker file: %v", err)
+	}
+	defer blocker.Close()
+
+	r, err := w.Prune(f.Layout, true)
+	if err != nil {
+		t.Fatalf("Prune(apply=true) error = %v; want nil", err)
+	}
+
+	var found *PruneEntry
+	for i := range r.Entries {
+		if filepath.Clean(r.Entries[i].WeftWorktree) == filepath.Clean(weftPath) {
+			found = &r.Entries[i]
+			break
+		}
+	}
+	if found == nil {
+		t.Fatalf("Prune(apply=true): no entry for weft path %s; entries = %+v", weftPath, r.Entries)
+	}
+
+	if found.Removed {
+		t.Errorf("PruneEntry.Removed = true despite lock + open handle; want false (both removal paths should fail)")
+	}
+	if found.Error == "" {
+		t.Fatalf("PruneEntry.Error = \"\"; want non-empty (both removal paths should fail)")
+	}
+	if strings.Contains(found.Error, "fatal:") {
+		t.Errorf("PruneEntry.Error = %q; want no %q substring (raw git stderr leak)", found.Error, "fatal:")
+	}
+
+	// Release the handle and unlock/remove the worktree so t.Cleanup (TempDir removal)
+	// does not fail on Windows, and so the fixture teardown succeeds cleanly.
+	blocker.Close()
+	_, _, _, _ = gitexec.RunGit([]string{"worktree", "unlock", weftPath}, f.Layout.WeftRepoRoot())
+	_, _, _, _ = gitexec.RunGit([]string{"worktree", "remove", "--force", weftPath}, f.Layout.WeftRepoRoot())
+}
+
 // TestPrune_LivePairNeverTouched asserts that Prune, whether in dry-run or apply mode,
 // does not include or remove a healthy live pair in its output.
 func TestPrune_LivePairNeverTouched(t *testing.T) {
diff --git a/internal/warpengine/reconcile.go b/internal/warpengine/reconcile.go
index 3dcda97..7d09dc4 100644
--- a/internal/warpengine/reconcile.go
+++ b/internal/warpengine/reconcile.go
@@ -214,7 +214,7 @@ func (w *Worktree) reconcileMissingWeft(
 // weft repo. This is the "adopt" path: the branch already exists so no -b flag is used.
 func adoptWeftWorktree(hostLayout *hubgeometry.Layout, weftPath, branch string) error {
 	// git worktree add <path> <branch> — no -b because the branch already exists.
-	_, stderr, exitCode, err := gitexec.RunGit(
+	_, _, exitCode, err := gitexec.RunGit(
 		[]string{"worktree", "add", weftPath, branch},
 		hostLayout.WeftRepoRoot(),
 	)
@@ -222,7 +222,7 @@ func adoptWeftWorktree(hostLayout *hubgeometry.Layout, weftPath, branch string)
 		return fmt.Errorf("git worktree add: %w", err)
 	}
 	if exitCode != 0 {
-		return fmt.Errorf("git worktree add failed: %s", stderr)
+		return fmt.Errorf("adopt weft worktree %q for branch %q failed (git exit %d)", weftPath, branch, exitCode)
 	}
 	return nil
 }
diff --git a/internal/warpengine/reconcile_test.go b/internal/warpengine/reconcile_test.go
index 5a46d21..a66b01c 100644
--- a/internal/warpengine/reconcile_test.go
+++ b/internal/warpengine/reconcile_test.go
@@ -110,6 +110,73 @@ func TestReconcile_MissingWeftWorktreeRecreated(t *testing.T) {
 	_ = weftPath
 }
 
+// TestReconcile_MissingWeftRecreateFailsNoStderrLeak asserts that when the weft-worktree
+// recreate path (adoptWeftWorktree) fails, ReconcilePairResult.Error is composed from
+// local context (the weft path, branch, and git exit code) rather than git's own stderr
+// text. The failure is forced by locking the weft branch in a separate weft worktree so
+// `git worktree add <path> <branch>` fails with "already checked out", mirroring
+// TestCheckout_HostRollback's lock technique.
+func TestReconcile_MissingWeftRecreateFailsNoStderrLeak(t *testing.T) {
+	t.Parallel()
+
+	f := setupReconcileFixture(t)
+
+	const testSlug = "feature-recreate-fail"
+	w := New(Config{BranchPrefix: ""})
+	_, err := w.Add(f.Layout, testSlug, AddOptions{SkipGit: true})
+	if err != nil {
+		t.Fatalf("Add(%q): %v", testSlug, err)
+	}
+
+	featureWeftPath := f.Layout.WeftWorktreePath(testSlug)
+
+	// Remove the weft worktree directory but keep the branch, so Reconcile takes the
+	// recreate path (rule 1).
+	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "worktree", "remove", "--force", featureWeftPath)
+	if !weftBranchExists(f.Layout, testSlug) {
+		t.Fatalf("pre-condition: weft branch %q must exist for recreate path", testSlug)
+	}
+
+	// Lock the weft branch by checking it out in a separate weft worktree, so the
+	// recreate's `git worktree add <path> <branch>` fails with "already checked out".
+	lockPath := filepath.Join(f.Layout.Hub, "lock-reconcile-recreate")
+	lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "worktree", "add", lockPath, testSlug)
+	t.Cleanup(func() {
+		lyxtest.MustRun(t, f.Layout.WeftRepoRoot(), "git", "worktree", "remove", "--force", lockPath)
+	})
+
+	r, err := w.Reconcile(f.Layout)
+	if err != nil {
+		t.Fatalf("Reconcile() error = %v; want nil", err)
+	}
+
+	var found *ReconcilePairResult
+	for i := range r.Pairs {
+		if filepath.Clean(r.Pairs[i].WeftWorktree) == filepath.Clean(featureWeftPath) {
+			found = &r.Pairs[i]
+			break
+		}
+	}
+	if found == nil {
+		t.Fatalf("Reconcile(): no pair result for weft path %s", featureWeftPath)
+	}
+	if found.Action != ReconcileActionWeftRecreated {
+		t.Errorf("Action = %q; want %q", found.Action, ReconcileActionWeftRecreated)
+	}
+	if found.Error == "" {
+		t.Fatalf("Error = \"\"; want non-empty (recreate should fail while the branch is locked)")
+	}
+	if !strings.Contains(found.Error, testSlug) {
+		t.Errorf("Error = %q; want substring %q (branch name)", found.Error, testSlug)
+	}
+	if strings.Contains(found.Error, "fatal:") {
+		t.Errorf("Error = %q; want no %q substring (raw git stderr leak)", found.Error, "fatal:")
+	}
+	if strings.Contains(found.Error, "already checked out") {
+		t.Errorf("Error = %q; want no %q substring (raw git stderr leak)", found.Error, "already checked out")
+	}
+}
+
 // TestReconcile_BrokenJunctionRepointed asserts that Reconcile re-points a host _lyx junction
 // that was removed (broken/dangling) while the weft worktree is still present.
 func TestReconcile_BrokenJunctionRepointed(t *testing.T) {
diff --git a/internal/warpengine/weftwiring.go b/internal/warpengine/weftwiring.go
index 021ef9f..c4d0b88 100644
--- a/internal/warpengine/weftwiring.go
+++ b/internal/warpengine/weftwiring.go
@@ -64,7 +64,7 @@ func weftBranchExists(l *hubgeometry.Layout, branch string) bool {
 // Returns an error if the command fails or exits with non-zero code.
 func createWeftWorktree(l *hubgeometry.Layout, slug, branch, startPoint string) error {
 	weftPath := l.WeftWorktreePath(slug)
-	_, stderr, exitCode, err := gitexec.RunGit(
+	_, _, exitCode, err := gitexec.RunGit(
 		[]string{"worktree", "add", "-b", branch, weftPath, startPoint},
 		l.WeftRepoRoot(),
 	)
@@ -72,7 +72,7 @@ func createWeftWorktree(l *hubgeometry.Layout, slug, branch, startPoint string)
 		return fmt.Errorf("failed to run git worktree add for weft: %w", err)
 	}
 	if exitCode != 0 {
-		return fmt.Errorf("weft worktree add failed: %s", stderr)
+		return fmt.Errorf("create weft worktree %q for branch %q failed (git exit %d)", weftPath, branch, exitCode)
 	}
 	return nil
 }
@@ -91,7 +91,7 @@ func pushWeftBranch(l *hubgeometry.Layout, slug, branch string, opts AddOptions)
 	}
 
 	weftPath := l.WeftWorktreePath(slug)
-	_, stderr, exitCode, err := gitexec.RunGit(
+	_, _, exitCode, err := gitexec.RunGit(
 		[]string{"push", "-u", "origin", branch},
 		weftPath,
 	)
@@ -99,7 +99,7 @@ func pushWeftBranch(l *hubgeometry.Layout, slug, branch string, opts AddOptions)
 		return fmt.Errorf("failed to run git push for weft: %w", err)
 	}
 	if exitCode != 0 {
-		return fmt.Errorf("weft push failed: %s", stderr)
+		return fmt.Errorf("push weft branch %q failed (git exit %d)", branch, exitCode)
 	}
 
 	return nil
diff --git a/internal/warpengine/weftwiring_test.go b/internal/warpengine/weftwiring_test.go
index 9f33558..322ab17 100644
--- a/internal/warpengine/weftwiring_test.go
+++ b/internal/warpengine/weftwiring_test.go
@@ -196,6 +196,70 @@ func TestWeftSpawnPushesWeftBranch(t *testing.T) {
 	}
 }
 
+// TestCreateWeftWorktree_InvalidStartPointFails asserts that createWeftWorktree's error
+// on an invalid start point is composed from local context (the weft path and branch,
+// plus the git exit code) rather than git's own stderr text.
+func TestCreateWeftWorktree_InvalidStartPointFails(t *testing.T) {
+	t.Parallel()
+
+	const slug = "create-weft-invalid-start"
+	const branch = "create-weft-invalid-start"
+
+	f := lyxtest.CopyPairedLocal(t)
+
+	err := createWeftWorktree(f.Layout, slug, branch, "nonexistent-start-point-xyz")
+	if err == nil {
+		t.Fatalf("createWeftWorktree(...) error = nil; want failure for a nonexistent start point")
+	}
+
+	// Compare against filepath.Base(weftPath) rather than the raw path: %q escapes
+	// backslashes on Windows, so the literal OS-native path never appears unescaped
+	// in err.Error() even though the weft path is faithfully reported.
+	weftPath := f.Layout.WeftWorktreePath(slug)
+	if weftName := filepath.Base(weftPath); !strings.Contains(err.Error(), weftName) {
+		t.Errorf("createWeftWorktree(...) error = %q; want substring %q (weft path)", err.Error(), weftName)
+	}
+	if !strings.Contains(err.Error(), branch) {
+		t.Errorf("createWeftWorktree(...) error = %q; want substring %q (branch)", err.Error(), branch)
+	}
+	if strings.Contains(err.Error(), "fatal:") {
+		t.Errorf("createWeftWorktree(...) error = %q; want no %q substring (raw git stderr leak)", err.Error(), "fatal:")
+	}
+}
+
+// TestPushWeftBranch_NoRemoteFails asserts that pushWeftBranch's error when no remote is
+// configured is composed from local context (the branch and git exit code) rather than
+// git's own stderr text.
+func TestPushWeftBranch_NoRemoteFails(t *testing.T) {
+	t.Parallel()
+
+	const slug = "push-weft-no-remote"
+	const branch = "push-weft-no-remote"
+
+	f := lyxtest.CopyPairedLocal(t)
+
+	w := New(Config{})
+	// SkipGit suppresses the push inside Add itself; we call pushWeftBranch directly
+	// below to exercise its error path without touching the shared template weft-bare.
+	if _, err := w.Add(f.Layout, slug, AddOptions{SkipGit: true}); err != nil {
+		t.Fatalf("Add(%q): %v", slug, err)
+	}
+
+	weftPath := f.Layout.WeftWorktreePath(slug)
+	lyxtest.MustRun(t, weftPath, "git", "remote", "remove", "origin")
+
+	err := pushWeftBranch(f.Layout, slug, branch, AddOptions{})
+	if err == nil {
+		t.Fatalf("pushWeftBranch(...) error = nil; want failure with no remote configured")
+	}
+	if !strings.Contains(err.Error(), branch) {
+		t.Errorf("pushWeftBranch(...) error = %q; want substring %q (branch)", err.Error(), branch)
+	}
+	if strings.Contains(err.Error(), "fatal:") {
+		t.Errorf("pushWeftBranch(...) error = %q; want no %q substring (raw git stderr leak)", err.Error(), "fatal:")
+	}
+}
+
 // TestWeftRollbackOnPostHostCreateFailure simulates a post-host-create failure
 // and asserts both host and weft state is rolled back completely.
 // Note: since Add is dormant (does not create junctions), rollback does not need
diff --git a/internal/weftengine/sync.go b/internal/weftengine/sync.go
index 067761a..325d07c 100644
--- a/internal/weftengine/sync.go
+++ b/internal/weftengine/sync.go
@@ -179,7 +179,7 @@ func pushUnpushed(weftPath string) error {
 			}
 			continue
 		}
-		return fmt.Errorf("push failed: %s", stderr)
+		return fmt.Errorf("push from %q failed (git exit %d) after rebase retry", weftPath, code)
 	}
 	return fmt.Errorf("push still failing after rebase retry")
 }
diff --git a/internal/weftengine/sync_test.go b/internal/weftengine/sync_test.go
index 1498752..be57d40 100644
--- a/internal/weftengine/sync_test.go
+++ b/internal/weftengine/sync_test.go
@@ -5,6 +5,7 @@
 package weftengine
 
 import (
+	"fmt"
 	"os"
 	"os/exec"
 	"path/filepath"
@@ -232,6 +233,50 @@ func TestPush(t *testing.T) {
 	}
 }
 
+// TestPush_BrokenRemoteFailsWithoutStderrLeak asserts that when a push fails for a
+// reason the rebase-retry loop cannot address (here, a remote URL that does not
+// point at any git repository), the returned error is composed from local context
+// (the weft path and git's exit code) rather than git's own stderr text.
+func TestPush_BrokenRemoteFailsWithoutStderrLeak(t *testing.T) {
+	t.Parallel()
+
+	fixture := lyxtest.CopyWeft(t)
+	weftRepo := fixture.WeftPath
+
+	// Modify and commit so there is something unpushed.
+	lyxFile := filepath.Join(weftRepo, "_lyx", "config.yaml")
+	if err := os.WriteFile(lyxFile, []byte("modified"), 0o644); err != nil {
+		t.Fatalf("WriteFile: %v", err)
+	}
+	committed, err := Commit(weftRepo, []string{"_lyx"}, SyncOptions{})
+	if err != nil {
+		t.Fatalf("Commit: %v", err)
+	}
+	if !committed {
+		t.Fatalf("Commit should have succeeded")
+	}
+
+	// Point origin at a path that is not a git repository at all. The local
+	// remote-tracking ref (used by hasUnpushed's @{u} lookup) is unaffected, but
+	// the push itself fails for a reason that does not match any of the
+	// retry-triggering substrings ("non-fast-forward", "rejected", "fetch first"),
+	// so it survives the rebase-retry loop and reaches the final error path.
+	badRemote := filepath.Join(t.TempDir(), "does-not-exist")
+	lyxtest.MustRun(t, weftRepo, "git", "remote", "set-url", "origin", badRemote)
+
+	err = Push(weftRepo, SyncOptions{})
+	if err == nil {
+		t.Fatalf("Push() error = nil; want error (broken remote)")
+	}
+	wantSubstr := fmt.Sprintf("%q", weftRepo)
+	if !strings.Contains(err.Error(), wantSubstr) {
+		t.Errorf("Push() error = %q; want substring %q (weft path)", err.Error(), wantSubstr)
+	}
+	if strings.Contains(err.Error(), "fatal:") {
+		t.Errorf("Push() error = %q; want no %q substring (raw git stderr leak)", err.Error(), "fatal:")
+	}
+}
+
 func TestPull_FastForward(t *testing.T) {
 	t.Parallel()
 	fixture := lyxtest.CopyWeft(t)
diff --git a/internal/yamlengine/reconcile.go b/internal/yamlengine/reconcile.go
index 01de03b..21af881 100644
--- a/internal/yamlengine/reconcile.go
+++ b/internal/yamlengine/reconcile.go
@@ -73,14 +73,7 @@ func Reconcile(template, existing []byte) (merged []byte, added, removed []strin
 	sort.Strings(removed)
 
 	// Reconcile: overwrite template leaf values with existing values
-	for path, existingLeaf := range existingLeaves {
-		if templateLeaf, ok := templateLeaves[path]; ok {
-			// Preserve the user's value in the template leaf
-			templateLeaf.Value = existingLeaf.Value
-			templateLeaf.Tag = existingLeaf.Tag
-			templateLeaf.Style = existingLeaf.Style
-		}
-	}
+	applyExistingOverrides(templateLeaves, existingLeaves)
 
 	// Marshal the mutated template back to bytes
 	merged, err = yaml.Marshal(&templateNode)
@@ -133,6 +126,22 @@ func MissingKeys(template, existing []byte) ([]string, error) {
 	return missing, nil
 }
 
+// applyExistingOverrides copies each existing leaf's value, tag, and style
+// onto the matching template leaf in place, leaving template leaves with no
+// counterpart in existing untouched. Both Reconcile and SetValues share this
+// merge step: it is the single definition of what "layer existing onto
+// template" means, so the two call sites cannot drift out of sync.
+func applyExistingOverrides(templateLeaves, existingLeaves map[string]*yaml.Node) {
+	for path, existingLeaf := range existingLeaves {
+		if templateLeaf, ok := templateLeaves[path]; ok {
+			// Preserve the user's value in the template leaf.
+			templateLeaf.Value = existingLeaf.Value
+			templateLeaf.Tag = existingLeaf.Tag
+			templateLeaf.Style = existingLeaf.Style
+		}
+	}
+}
+
 // collectLeafPaths walks a YAML node tree and collects all leaf key-paths
 // (scalars accessible via mappings and sequences).
 //
diff --git a/internal/yamlengine/set.go b/internal/yamlengine/set.go
new file mode 100644
index 0000000..0e71f96
--- /dev/null
+++ b/internal/yamlengine/set.go
@@ -0,0 +1,117 @@
+// set.go implements value-preserving single/multi-key YAML mutation for the
+// non-interactive `lyx config <module> --set key=value` path. Unlike Reconcile
+// (which merges an entire existing file into a template), SetValues applies a
+// small, explicit list of key=value pairs while still routing every write
+// through the template-shaped working tree so partial/stale existing files
+// never hide a valid key behind a missing node.
+
+package yamlengine
+
+import (
+	"sort"
+
+	"gopkg.in/yaml.v3"
+)
+
+// KV is a single key=value pair to apply via SetValues. Key is a dotted
+// leaf key-path (the same shape collectLeafPaths produces, e.g.
+// "level1.level2.key"); Value is the raw string to store as the leaf's
+// scalar value.
+type KV struct {
+	Key   string
+	Value string
+}
+
+// SetResult is the outcome of a SetValues call.
+//
+// Merged holds the new file bytes and is only valid when Unknown is empty;
+// callers must not write Merged to disk otherwise. Unknown is the sorted,
+// deduplicated list of requested keys absent from the template's leaf-key
+// set. Known is the template's full sorted leaf-key set, included so callers
+// can build a helpful "known keys are..." error message without recomputing it.
+type SetResult struct {
+	Merged  []byte
+	Unknown []string
+	Known   []string
+}
+
+// SetValues applies pairs to a template-shaped YAML document, preserving
+// comments, key order, and any values from existing that already agree with
+// the template's structure.
+//
+// The working tree mutated and marshalled is always templateNode, never a
+// bare parse of existing: this guarantees every template leaf has a real,
+// settable node even when existing is a stale or partial file missing some of
+// the template's keys. When existing is non-empty its leaf values are first
+// copied onto the matching templateNode leaves (mirroring Reconcile's merge
+// step), so a --set call layers on top of whatever the user already
+// customized rather than clobbering it back to the template defaults.
+//
+// If any pairs[i].Key is not present in the template's leaf-key set, no
+// mutation is performed at all: SetResult.Unknown is returned non-empty and
+// Merged is nil. Otherwise every pair is applied to the working tree in the
+// given order (a later pair for a repeated key wins) and the mutated tree is
+// marshalled into SetResult.Merged.
+func SetValues(template, existing []byte, pairs []KV) (SetResult, error) {
+	// Parse the template into the tree we will mutate and ultimately marshal.
+	var templateNode yaml.Node
+	if err := yaml.Unmarshal(template, &templateNode); err != nil {
+		return SetResult{}, err
+	}
+
+	templateLeaves := make(map[string]*yaml.Node)
+	collectLeafPaths(&templateNode, templateLeaves)
+
+	known := make([]string, 0, len(templateLeaves))
+	for path := range templateLeaves {
+		known = append(known, path)
+	}
+	sort.Strings(known)
+
+	// Layer existing's values onto the template working tree via the same
+	// applyExistingOverrides helper Reconcile uses, so a --set call preserves
+	// whatever the user already customized rather than resetting untouched
+	// keys back to defaults.
+	if len(existing) > 0 {
+		var existingNode yaml.Node
+		if err := yaml.Unmarshal(existing, &existingNode); err != nil {
+			return SetResult{}, err
+		}
+		existingLeaves := make(map[string]*yaml.Node)
+		collectLeafPaths(&existingNode, existingLeaves)
+
+		applyExistingOverrides(templateLeaves, existingLeaves)
+	}
+
+	// Validate every requested key against the template's leaf set before
+	// mutating anything, so a single unknown key rejects the whole call
+	// rather than silently applying a partial write.
+	unknownSet := make(map[string]bool)
+	for _, pair := range pairs {
+		if _, ok := templateLeaves[pair.Key]; !ok {
+			unknownSet[pair.Key] = true
+		}
+	}
+	if len(unknownSet) > 0 {
+		unknown := make([]string, 0, len(unknownSet))
+		for key := range unknownSet {
+			unknown = append(unknown, key)
+		}
+		sort.Strings(unknown)
+		return SetResult{Unknown: unknown, Known: known}, nil
+	}
+
+	// Every key is now guaranteed to have a real node in templateNode, since
+	// the working tree always contains every template leaf. Apply pairs in
+	// order so a repeated key's later value wins.
+	for _, pair := range pairs {
+		templateLeaves[pair.Key].Value = pair.Value
+	}
+
+	merged, err := yaml.Marshal(&templateNode)
+	if err != nil {
+		return SetResult{}, err
+	}
+
+	return SetResult{Merged: merged, Known: known}, nil
+}
diff --git a/internal/yamlengine/set_test.go b/internal/yamlengine/set_test.go
new file mode 100644
index 0000000..3c71ed6
--- /dev/null
+++ b/internal/yamlengine/set_test.go
@@ -0,0 +1,175 @@
+// set_test.go contains table-driven and individual tests for SetValues, covering
+// unknown-key rejection, byte-for-byte round-tripping of tricky values, comment/order
+// preservation, and the partial-existing regression case that motivated Card 1's
+// always-mutate-the-template-tree design.
+
+package yamlengine
+
+import (
+	"strings"
+	"testing"
+)
+
+// TestSetValues_UnknownKeyRejectsWholeCall verifies that when any key among
+// multiple pairs is unknown, SetValues returns a non-empty Unknown and a nil
+// Merged — no partial mutation is observable, even though the other keys in
+// the same call are valid.
+func TestSetValues_UnknownKeyRejectsWholeCall(t *testing.T) {
+	template := []byte("key1: default1\nkey2: default2\n")
+
+	result, err := SetValues(template, nil, []KV{
+		{Key: "key1", Value: "new1"},
+		{Key: "bogus", Value: "irrelevant"},
+	})
+	if err != nil {
+		t.Fatalf("SetValues() unexpected error: %v", err)
+	}
+
+	if len(result.Unknown) != 1 || result.Unknown[0] != "bogus" {
+		t.Errorf("SetValues() Unknown = %v; want [\"bogus\"]", result.Unknown)
+	}
+	if result.Merged != nil {
+		t.Errorf("SetValues() Merged = %q; want nil (no partial mutation)", result.Merged)
+	}
+}
+
+// TestSetValues_ValueWithEqualsRoundTrips verifies that a value containing an
+// '=' character is preserved byte-for-byte in Merged.
+func TestSetValues_ValueWithEqualsRoundTrips(t *testing.T) {
+	template := []byte("key1: default\n")
+	const want = "a=b=c"
+
+	result, err := SetValues(template, nil, []KV{{Key: "key1", Value: want}})
+	if err != nil {
+		t.Fatalf("SetValues() unexpected error: %v", err)
+	}
+	assertMergedKeyValue(t, result, "key1", want)
+}
+
+// TestSetValues_ValueWithSpacesRoundTrips verifies that a value containing
+// spaces is preserved byte-for-byte in Merged.
+func TestSetValues_ValueWithSpacesRoundTrips(t *testing.T) {
+	template := []byte("key1: default\n")
+	const want = "hello there world"
+
+	result, err := SetValues(template, nil, []KV{{Key: "key1", Value: want}})
+	if err != nil {
+		t.Fatalf("SetValues() unexpected error: %v", err)
+	}
+	assertMergedKeyValue(t, result, "key1", want)
+}
+
+// TestSetValues_MultiplePairsAllApplied verifies that multiple valid pairs in
+// one call are all reflected in Merged.
+func TestSetValues_MultiplePairsAllApplied(t *testing.T) {
+	template := []byte("key1: default1\nkey2: default2\nkey3: default3\n")
+
+	result, err := SetValues(template, nil, []KV{
+		{Key: "key1", Value: "set1"},
+		{Key: "key3", Value: "set3"},
+	})
+	if err != nil {
+		t.Fatalf("SetValues() unexpected error: %v", err)
+	}
+	assertMergedKeyValue(t, result, "key1", "set1")
+	assertMergedKeyValue(t, result, "key3", "set3")
+	assertMergedKeyValue(t, result, "key2", "default2")
+}
+
+// TestSetValues_CommentsAndOrderPreserved mirrors the idempotency-style
+// assertions in TestReconcile_TemplateCommentsAndOrder: template comments and
+// key order survive in Merged, and only the requested key's value changes.
+func TestSetValues_CommentsAndOrderPreserved(t *testing.T) {
+	template := []byte("# Key 1 comment\nkey1: template_val1\n# Key 2 comment\nkey2: template_val2\n")
+	existing := []byte("key2: user_val2\nkey1: user_val1\n")
+
+	result, err := SetValues(template, existing, []KV{{Key: "key1", Value: "new_val1"}})
+	if err != nil {
+		t.Fatalf("SetValues() unexpected error: %v", err)
+	}
+	if result.Unknown != nil {
+		t.Fatalf("SetValues() Unknown = %v; want none", result.Unknown)
+	}
+
+	merged := string(result.Merged)
+	if !strings.Contains(merged, "# Key 1 comment") || !strings.Contains(merged, "# Key 2 comment") {
+		t.Errorf("SetValues() merged does not preserve template comments; got %q", merged)
+	}
+	// key1 was explicitly set to new_val1, overriding existing's user_val1.
+	assertMergedKeyValue(t, result, "key1", "new_val1")
+	// key2 was untouched by pairs, so existing's user_val2 must survive.
+	assertMergedKeyValue(t, result, "key2", "user_val2")
+
+	idx1 := strings.Index(merged, "key1")
+	idx2 := strings.Index(merged, "key2")
+	if idx1 > idx2 {
+		t.Errorf("SetValues() merged does not preserve template key order")
+	}
+}
+
+// TestSetValues_EmptyExistingBehavesLikeTemplate verifies that an empty
+// existing behaves like Reconcile's empty-existing case: Merged is equivalent
+// to the template with the requested keys set.
+func TestSetValues_EmptyExistingBehavesLikeTemplate(t *testing.T) {
+	template := []byte("key1: default1\nkey2: default2\n")
+
+	result, err := SetValues(template, nil, []KV{{Key: "key1", Value: "set1"}})
+	if err != nil {
+		t.Fatalf("SetValues() unexpected error: %v", err)
+	}
+	assertMergedKeyValue(t, result, "key1", "set1")
+	assertMergedKeyValue(t, result, "key2", "default2")
+}
+
+// TestSetValues_PartialExistingDoesNotSuppressSet is the plan-review round-1
+// regression case: a pairs[i].Key present in template (so it passes Known
+// validation) but absent from a non-empty, partial existing (which only has
+// one of the template's three keys) must still be applied in Merged rather
+// than silently dropped, because the working tree is always templateNode —
+// never a bare parse of existing — so every template leaf has a real node
+// regardless of what existing does or doesn't contain.
+func TestSetValues_PartialExistingDoesNotSuppressSet(t *testing.T) {
+	template := []byte("key1: default1\nkey2: default2\nkey3: default3\n")
+	// existing has only key1; key2 and key3 have no corresponding node here.
+	existing := []byte("key1: user_val1\n")
+
+	result, err := SetValues(template, existing, []KV{{Key: "key2", Value: "newly_set"}})
+	if err != nil {
+		t.Fatalf("SetValues() unexpected error: %v", err)
+	}
+	if result.Unknown != nil {
+		t.Fatalf("SetValues() Unknown = %v; want none", result.Unknown)
+	}
+	// key1's existing override must survive.
+	assertMergedKeyValue(t, result, "key1", "user_val1")
+	// key2 must be set, not silently dropped because it had no node in existing.
+	assertMergedKeyValue(t, result, "key2", "newly_set")
+	// key3 must remain the template default (untouched by both existing and pairs).
+	assertMergedKeyValue(t, result, "key3", "default3")
+}
+
+// assertMergedKeyValue is a test helper that unmarshals result.Merged into a
+// map and asserts the given top-level key holds want.
+func assertMergedKeyValue(t *testing.T, result SetResult, key, want string) {
+	t.Helper()
+	got := extractYAMLValue(t, result.Merged, key)
+	if got != want {
+		t.Errorf("SetValues() merged[%q] = %q; want %q", key, got, want)
+	}
+}
+
+// extractYAMLValue is a minimal top-level-key extractor for asserting a
+// single scalar value out of merged YAML bytes without pulling in a full
+// YAML-to-map round-trip (which would normalize quoting and defeat the
+// byte-for-byte assertions this test file makes).
+func extractYAMLValue(t *testing.T, data []byte, key string) string {
+	t.Helper()
+	prefix := key + ": "
+	for _, line := range strings.Split(string(data), "\n") {
+		if strings.HasPrefix(line, prefix) {
+			return strings.TrimPrefix(line, prefix)
+		}
+	}
+	t.Fatalf("key %q not found in merged YAML: %q", key, string(data))
+	return ""
+}

```

## Instructions

1. Read the failing tests and the source files they exercise.
2. Fix the root cause of the failures. Do not modify tests unless they are genuinely wrong due to the merge (e.g. a test asserted against a value that the merge legitimately changed).
3. Re-run `go test -tags integration ./internal/weftengine/... ./internal/weftcli/... -count=1` after each fix attempt using `git -C C:\Code\loomyard\wts\lyx-deinit` for git commands.
4. Commit each fix attempt with a clear commit message.
5. Self-fix up to `3` times. If the verify command still fails after `3` attempts, stop and report stuck.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success:

{"status":"success","commit_sha":"<last-HEAD-sha>"}

After exhausting fix rounds:

{"status":"stuck","stuck_type":"verify","reason":"<one-line description of what still fails>","commit_sha":"<last-HEAD-sha>"}

Anything other than this JSON object on the last line is a protocol violation; the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost. Do not wrap the JSON in a code fence; do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob. Use `git -C C:\Code\loomyard\wts\lyx-deinit` for git commands; do not `cd`. Worktree cwd is `C:\Code\loomyard\wts\lyx-deinit`.
