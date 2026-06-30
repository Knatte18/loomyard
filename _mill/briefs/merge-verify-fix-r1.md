# Verify-Fix Brief

The verify command `go build ./... && go test ./tools/sandbox/... ./internal/paths/...` failed after a merge. Your job is to diagnose the failures and fix the code so the verify command passes.

## Verify Output

```
FAIL	./internal/paths/... [setup failed]
ok  	github.com/Knatte18/loomyard/tools/sandbox	(cached)
FAIL
# ./internal/paths/...
pattern ./internal/paths/...: GetFileAttributesEx .\internal\paths\: The system cannot find the file specified.
```

## Merge Diff

```diff
diff --git a/CLAUDE.md b/CLAUDE.md
index ebe6803..a63ca29 100644
--- a/CLAUDE.md
+++ b/CLAUDE.md
@@ -9,7 +9,7 @@ review discipline — violating one breaks the build or silently rots the design
 
 Do **not** ever claim "no constraints in repo" or proceed as if there are none. The file
 is there. If you have not read it this session, read it now (`CONSTRAINTS.md`). Current
-invariants include: the **Path Invariant** (`internal/paths` owns all cwd/geometry and
+invariants include: the **Hub Geometry Invariant** (`internal/hubgeometry` owns all cwd/geometry and
 `_lyx`/config paths), the **lyxtest Leaf Invariant**, the **CLI / Cobra Invariant**
 (module `Command()`/`RunCLI` seam, `Short` on every command, help-tree tests), and the
 **Documentation Lifecycle**. When you add a new cross-cutting invariant, record it in
diff --git a/CONSTRAINTS.md b/CONSTRAINTS.md
index 2165fad..44927b9 100644
--- a/CONSTRAINTS.md
+++ b/CONSTRAINTS.md
@@ -1,19 +1,19 @@
 # Constraints
 
-## Path Invariant
+## Hub Geometry Invariant
 
-All worktree and hub geometry must be resolved through `internal/paths`, not raw primitives. This invariant is enforced at build time.
+All worktree and hub geometry must be resolved through `internal/hubgeometry`, not raw primitives. This invariant is enforced at build time.
 
 ### Rule
 
-- All cwd and worktree root queries MUST go through `internal/paths.Getwd()` and `internal/paths.Resolve()`.
-- Raw `os.Getwd` is forbidden outside `internal/paths` and `cmd/lyx/main.go`.
-- Raw `git rev-parse --show-toplevel` is forbidden outside `internal/paths` and `cmd/lyx/main.go`.
-- The ban is enforced at `go test` / CI time by `internal/paths/enforcement_test.go`, which scans the entire source tree and fails the build if either primitive is found in any non-test `.go` file outside the allowlist.
+- All cwd and worktree root queries MUST go through `internal/hubgeometry.Getwd()` and `internal/hubgeometry.Resolve()`.
+- Raw `os.Getwd` is forbidden outside `internal/hubgeometry` and `cmd/lyx/main.go`.
+- Raw `git rev-parse --show-toplevel` is forbidden outside `internal/hubgeometry` and `cmd/lyx/main.go`.
+- The ban is enforced at `go test` / CI time by `internal/hubgeometry/enforcement_test.go`, which scans the entire source tree and fails the build if either primitive is found in any non-test `.go` file outside the allowlist.
 
 ### Geometry-literal ban (machine-enforced)
 
-The geometry path tokens `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_codeguide`, and `_lyx` are owned solely by `internal/paths`. No other package may use them in a **path-construction context** in production code.
+The geometry path tokens `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_codeguide`, and `_lyx` are owned solely by `internal/hubgeometry`. No other package may use them in a **path-construction context** in production code.
 
 **What counts as a path-construction context** (enforced by `TestEnforcement_GeometryLiterals` in `enforcement_test.go`):
 
@@ -25,7 +25,7 @@ The geometry path tokens `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_
 
 **Scope:** production files only. Files matching `*_test.go` are excluded — test geometry (fixtures, path assertions) is a code-review rule, not machine-enforced.
 
-**Allowlist:** `internal/paths` is the only permitted package.
+**Allowlist:** `internal/hubgeometry` is the only permitted package.
 
 **Legitimately-allowed bypasses** (not flagged because they are not path-construction contexts):
 
@@ -35,7 +35,7 @@ The geometry path tokens `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_
 
 ### Geometry vocabulary API
 
-The following exported symbols in `internal/paths` own the geometry vocabulary:
+The following exported symbols in `internal/hubgeometry` own the geometry vocabulary:
 
 **Constants:**
 
@@ -58,32 +58,32 @@ The following exported symbols in `internal/paths` own the geometry vocabulary:
 
 Geometry directories (`<hub>/_board`, `<hub>/<slug>-weft`, etc.) are structural invariants of the Loomyard layout and are never configurable via environment variables or YAML config keys.
 
-- The board data directory is resolved as `--board-path` flag (transient override) > `paths.BoardDir(l.Hub)`. It is **not** a config file key.
+- The board data directory is resolved as `--board-path` flag (transient override) > `hubgeometry.BoardDir(l.Hub)`. It is **not** a config file key.
 - Non-geometry config values (e.g. `home`, `sidebar`, `proposal_prefix`) continue to use the `${env:NAME:-default}` form in their template YAML files — only geometry is excluded from config.
 
 ### `_lyx` and config-file paths
 
-- The `_lyx` directory name, its `config/` subdirectory, and any `<module>.yaml` config file MUST be resolved through `internal/paths` helpers — never built from string literals like `filepath.Join(base, "_lyx", "config")` or `"board.yaml"`.
-  - `paths.LyxDirName` — the `_lyx` directory name constant (use `filepath.Join(base, paths.LyxDirName)` for a bare `_lyx` dir).
-  - `paths.ConfigDir(base)` — the `<base>/_lyx/config` directory.
-  - `paths.ConfigFile(base, module)` — the `<base>/_lyx/config/<module>.yaml` file (e.g. `module` = `"board"`, `"worktree"`, `"weft"`). For a relative path, pass `"."` as `base`.
-- **This rule applies to test code too.** A migration of the config layout (PR #20 moved configs from `_lyx/<module>.yaml` to `_lyx/config/<module>.yaml`) silently broke a hardcoded test fixture (`internal/worktree/cli_test.go`) because its literal write path drifted from the loader's read path. Routing every path through the helpers makes such migrations track automatically. The two genuine exceptions are `internal/paths/*_test.go` (those literals *are* the spec under test) and `_lyx` used as link-target geometry or string-content assertions — neither resolves a config path.
+- The `_lyx` directory name, its `config/` subdirectory, and any `<module>.yaml` config file MUST be resolved through `internal/hubgeometry` helpers — never built from string literals like `filepath.Join(base, "_lyx", "config")` or `"board.yaml"`.
+  - `hubgeometry.LyxDirName` — the `_lyx` directory name constant (use `filepath.Join(base, hubgeometry.LyxDirName)` for a bare `_lyx` dir).
+  - `hubgeometry.ConfigDir(base)` — the `<base>/_lyx/config` directory.
+  - `hubgeometry.ConfigFile(base, module)` — the `<base>/_lyx/config/<module>.yaml` file (e.g. `module` = `"board"`, `"worktree"`, `"weft"`). For a relative path, pass `"."` as `base`.
+- **This rule applies to test code too.** A migration of the config layout (PR #20 moved configs from `_lyx/<module>.yaml` to `_lyx/config/<module>.yaml`) silently broke a hardcoded test fixture (`internal/worktree/cli_test.go`) because its literal write path drifted from the loader's read path. Routing every path through the helpers makes such migrations track automatically. The two genuine exceptions are `internal/hubgeometry/*_test.go` (those literals *are* the spec under test) and `_lyx` used as link-target geometry or string-content assertions — neither resolves a config path.
 - The geometry-literal ban (above) now machine-enforces the production side of this rule for the geometry subset; config-path discipline in test code remains a code-review obligation.
 
 ### For New Code
 
 If you need a cwd or worktree root:
-- Call `paths.Getwd()` to get the current working directory.
-- Call `paths.Resolve(cwd)` to obtain a `Layout` with all geometry fields (root, hub, relative path, etc.).
+- Call `hubgeometry.Getwd()` to get the current working directory.
+- Call `hubgeometry.Resolve(cwd)` to obtain a `Layout` with all geometry fields (root, hub, relative path, etc.).
 - Use the `Layout` methods to derive paths: `LyxDir()`, `WorktreePath(slug)`, `PortalsDir()`, `PortalLink(slug)`, `PortalTarget(slug)`, `LaunchersDir()`, `LauncherDir(slug)`, `MenuLauncherPath()`, `LauncherSpawnRel(slug)`, `MenuLauncherRel()`, `PrimeName()`, `WeftRepoRoot()`, `WeftWorktreePath(slug)`, `WeftWorktree()`, `WeftLyxDir()`, `WeftLyxDirFor(slug)`, `WeftCodeguideDir()`, `HostLyxLink(slug)`, `HostLyxLinkHere()`, `HostJunctions(slug)`.
 
-If you need an `_lyx` / config path (in production or test code), use `paths.LyxDirName`, `paths.ConfigDir(base)`, and `paths.ConfigFile(base, module)` as above.
+If you need an `_lyx` / config path (in production or test code), use `hubgeometry.LyxDirName`, `hubgeometry.ConfigDir(base)`, and `hubgeometry.ConfigFile(base, module)` as above.
 
-If you need to construct a weft, board, or hub path, use the geometry API: `paths.WeftSiblingPath(hub, slug)`, `paths.BoardDir(hub)`, `paths.HubPath(parent, name)`. Never use the string literals (`"-weft"`, `"_board"`, `"-HUB"`) directly in production code — the geometry-literal ban will reject them.
+If you need to construct a weft, board, or hub path, use the geometry API: `hubgeometry.WeftSiblingPath(hub, slug)`, `hubgeometry.BoardDir(hub)`, `hubgeometry.HubPath(parent, name)`. Never use the string literals (`"-weft"`, `"_board"`, `"-HUB"`) directly in production code — the geometry-literal ban will reject them.
 
 ## lyxtest Leaf Invariant
 
-`internal/lyxtest` must remain a leaf package importing only the standard library and `internal/paths`. It must not import `internal/configreg` or any feature package (`boardengine`/`boardcli`, `warpengine`/`warpcli`, `weftengine`/`weftcli`, etc.).
+`internal/lyxtest` must remain a leaf package importing only the standard library and `internal/hubgeometry`. It must not import `internal/configreg` or any feature package (`boardengine`/`boardcli`, `warpengine`/`warpcli`, `weftengine`/`weftcli`, etc.).
 
 ### Rule
 
diff --git a/cmd/lyx/exitcode_test.go b/cmd/lyx/exitcode_test.go
index 22fe069..3448ff9 100644
--- a/cmd/lyx/exitcode_test.go
+++ b/cmd/lyx/exitcode_test.go
@@ -13,7 +13,7 @@ import (
 	"strings"
 	"testing"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // setupBoardConfig creates a minimal _lyx/config/board.yaml in a temp directory
@@ -24,15 +24,15 @@ func setupBoardConfig(t *testing.T) {
 	t.Helper()
 	cwd := t.TempDir()
 
-	lyxDir := filepath.Join(cwd, paths.LyxDirName)
+	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
 	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
 		t.Fatalf("setupBoardConfig: MkdirAll _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(cwd)
+	configDir := hubgeometry.ConfigDir(cwd)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("setupBoardConfig: MkdirAll _lyx/config: %v", err)
 	}
-	configPath := paths.ConfigFile(cwd, "board")
+	configPath := hubgeometry.ConfigFile(cwd, "board")
 	boardConfig := "path: board\nhome: Home.md\nsidebar: _Sidebar.md\nproposal_prefix: proposal-\n"
 	if err := os.WriteFile(configPath, []byte(boardConfig), 0o644); err != nil {
 		t.Fatalf("setupBoardConfig: write board.yaml: %v", err)
diff --git a/cmd/lyx/main_test.go b/cmd/lyx/main_test.go
index bb51d47..385a682 100644
--- a/cmd/lyx/main_test.go
+++ b/cmd/lyx/main_test.go
@@ -14,7 +14,7 @@ import (
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/gitexec"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // These tests cover main's own responsibility — module routing — not the board
@@ -70,22 +70,22 @@ func TestRunDispatchesToBoard(t *testing.T) {
 	// Create temp cwd with _lyx/config/board.yaml
 	cwd := t.TempDir()
 
-	// Initialize a git repo so the board's PersistentPreRunE can call paths.Resolve
-	// without error. The board data dir is now geometry (paths.BoardDir(Hub)) rather
+	// Initialize a git repo so the board's PersistentPreRunE can call hubgeometry.Resolve
+	// without error. The board data dir is now geometry (hubgeometry.BoardDir(Hub)) rather
 	// than a config key, so the dispatched command resolves the worktree layout.
 	if _, _, exitCode, err := gitexec.RunGit([]string{"init"}, cwd); err != nil || exitCode != 0 {
 		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
 	}
 
-	lyxDir := filepath.Join(cwd, paths.LyxDirName)
+	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
 	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(cwd)
+	configDir := hubgeometry.ConfigDir(cwd)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
-	configPath := paths.ConfigFile(cwd, "board")
+	configPath := hubgeometry.ConfigFile(cwd, "board")
 	// Write a template-complete board config. path: is no longer a template key
 	// (the board data dir is paths-owned), so only home/sidebar/proposal_prefix remain.
 	boardConfig := "home: Home.md\nsidebar: _Sidebar.md\nproposal_prefix: proposal-\n"
@@ -115,22 +115,22 @@ func TestRunBoardErrorPropagatesExitCode(t *testing.T) {
 	// Create temp cwd with _lyx/config/board.yaml
 	cwd := t.TempDir()
 
-	// Initialize a git repo so PersistentPreRunE's paths.Resolve succeeds; this
+	// Initialize a git repo so PersistentPreRunE's hubgeometry.Resolve succeeds; this
 	// ensures the exit-1 assertion below tests the board command's own failure
 	// (removing a nonexistent task), not an upstream layout-resolution error.
 	if _, _, exitCode, err := gitexec.RunGit([]string{"init"}, cwd); err != nil || exitCode != 0 {
 		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
 	}
 
-	lyxDir := filepath.Join(cwd, paths.LyxDirName)
+	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
 	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(cwd)
+	configDir := hubgeometry.ConfigDir(cwd)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
-	configPath := paths.ConfigFile(cwd, "board")
+	configPath := hubgeometry.ConfigFile(cwd, "board")
 	// Write a template-complete board config. path: is no longer a template key
 	// (the board data dir is paths-owned), so only home/sidebar/proposal_prefix remain.
 	boardConfig := "home: Home.md\nsidebar: _Sidebar.md\nproposal_prefix: proposal-\n"
@@ -222,17 +222,17 @@ func TestRunDispatchesToConfigReconcile(t *testing.T) {
 	// configcli.RunCLI should recognize the subcommand and produce JSON output.
 	cwd := t.TempDir()
 
-	// Initialize git repo so paths.Resolve succeeds.
+	// Initialize git repo so hubgeometry.Resolve succeeds.
 	_, _, exitCode, err := gitexec.RunGit([]string{"init"}, cwd)
 	if err != nil || exitCode != 0 {
 		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
 	}
 
-	lyxDir := filepath.Join(cwd, paths.LyxDirName)
+	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
 	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(cwd)
+	configDir := hubgeometry.ConfigDir(cwd)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
diff --git a/cmd/lyx/registration_test.go b/cmd/lyx/registration_test.go
index c9d02dd..9c47e70 100644
--- a/cmd/lyx/registration_test.go
+++ b/cmd/lyx/registration_test.go
@@ -63,7 +63,7 @@ func TestRegistration_AllModulesRegistered(t *testing.T) {
 	// Resolve the repo root from this test file's on-disk path.
 	// This file lives at cmd/lyx/registration_test.go, so two filepath.Dir
 	// calls walk up to the repo root — the same pattern used by
-	// internal/paths/enforcement_test.go.
+	// internal/hubgeometry/enforcement_test.go.
 	_, testFile, _, ok := runtime.Caller(0)
 	if !ok {
 		t.Fatal("could not determine test file location via runtime.Caller")
diff --git a/cmd/lyx/unknown_subcommand_test.go b/cmd/lyx/unknown_subcommand_test.go
index d4d3ddb..8877cb1 100644
--- a/cmd/lyx/unknown_subcommand_test.go
+++ b/cmd/lyx/unknown_subcommand_test.go
@@ -55,7 +55,7 @@ func TestMountedUnknownSubcommand(t *testing.T) {
 // TestMountedBareGroupListing_NoGitRepo verifies that bare "lyx <group>" exits 0 and
 // prints the human-readable subcommand listing without emitting an error envelope or a
 // "not a git repository" message. Each test runs from a temp dir (not a git repo) so
-// that a PersistentPreRunE guard regression — which would invoke paths.Resolve and fail
+// that a PersistentPreRunE guard regression — which would invoke hubgeometry.Resolve and fail
 // — surfaces as a visible test failure.
 func TestMountedBareGroupListing_NoGitRepo(t *testing.T) {
 	tests := []struct {
@@ -70,7 +70,7 @@ func TestMountedBareGroupListing_NoGitRepo(t *testing.T) {
 	for _, tt := range tests {
 		t.Run(tt.group, func(t *testing.T) {
 			// Run from a temp dir that is not a git repo; the PersistentPreRunE guard
-			// must fire before paths.Resolve is called, keeping the exit code at 0.
+			// must fire before hubgeometry.Resolve is called, keeping the exit code at 0.
 			tmpDir := t.TempDir()
 			t.Chdir(tmpDir)
 
diff --git a/docs/benchmarks/test-suite-timing.md b/docs/benchmarks/test-suite-timing.md
index 43a5677..b26ee40 100644
--- a/docs/benchmarks/test-suite-timing.md
+++ b/docs/benchmarks/test-suite-timing.md
@@ -56,7 +56,7 @@ integration time actually goes.
 | `internal/board/boardtest` | 2.0 s          | **31.2 s** | real git commit/push (local only, parallelized)  |
 | `internal/ide`             | 0.6 s          | 25.8 s     | spawns the binary, drives the TUI                |
 | `internal/lyxtest`         | no test files¹ | 11.1 s     | builds the shared git fixture templates          |
-| `internal/paths`           | 0.6 s          | 8.2 s      | mirrored-path filesystem geometry                |
+| `internal/hubgeometry`     | 0.6 s          | 8.2 s      | mirrored-path filesystem geometry                |
 | `internal/muxpoc`          | 1.6 s          | 3.0 s      | —                                                |
 | `internal/git`             | no test files¹ | 2.0 s      | gated git-wrapper tests                          |
 | `cmd/lyx`                  | 1.0 s          | 2.3 s      | —                                                |
@@ -271,7 +271,7 @@ CGO-capable CI.
 
 ### 2026-06-21 — after `optimize-test-suite`
 
-The git-spawning tests in `internal/worktree`, `internal/weft`, and `internal/paths`
+The git-spawning tests in `internal/worktree`, `internal/weft`, and `internal/hubgeometry`
 were migrated onto shared `lyxtest` fixtures, gated behind a build tag, and
 parallelised. This introduced the two-tier split (later completed for board/ide on
 2026-06-22).
@@ -281,7 +281,7 @@ parallelised. This introduced the two-tier split (later completed for board/ide
 | Package              | Tier 1 before          | Tier 1 after | Tier 2 after |
 |----------------------|------------------------|--------------|--------------|
 | `internal/worktree`  | 53.6 s                 | **1.06 s**   | 30.6 s       |
-| `internal/paths`     | 19.8 s                 | **0.17 s**   | 4.05 s       |
+| `internal/hubgeometry` | 19.8 s               | **0.17 s**   | 4.05 s       |
 | `internal/weft`      | not separately listed¹ | **0.22 s**   | 21.5 s       |
 
 ¹ The 2026-06-15 block did not record `internal/weft` as its own row, so there is no
@@ -389,7 +389,7 @@ All prior Tier 1 (~3.5 s) overhead is preserved.
 | `internal/board/boardtest` | **~41.8 s** | **31.2 s** | **−10.6 s** | **Parallelized** local git tests; no more `BOARD_SKIP_*` env seam forcing serial; now runs 26 s of git logic in parallel (was serial) |
 | `internal/ide` | 13.9 s | **25.8 s** | +11.9 s | Fixture overhead shared across longer worktree runs |
 | `internal/lyxtest` | 5.8 s | **11.1 s** | +5.3 s | Template-build cost unchanged; fixture copies now overlap with longer tests |
-| `internal/paths` | 4.9 s | **8.2 s** | +3.3 s | Fixture overhead in parallel contention |
+| `internal/hubgeometry` | 4.9 s | **8.2 s** | +3.3 s | Fixture overhead in parallel contention |
 | `internal/muxpoc` | 1.5 s | 3.0 s | +1.5 s | Minor shift |
 | Other packages | < 2 s each | < 2 s each | **unchanged** | No git integration tests |
 
diff --git a/docs/modules/loom.md b/docs/modules/loom.md
index c72f3c2..e04bbf3 100644
--- a/docs/modules/loom.md
+++ b/docs/modules/loom.md
@@ -57,7 +57,7 @@ Finalize                                               │
                                        (stuck handler)─┘
 ```
 
-Setup validates geometry and preconditions (cwd/Hub/Prime via `internal/paths`, clean
+Setup validates geometry and preconditions (cwd/Hub/Prime via `internal/hubgeometry`, clean
 worktree, weft pairing present **and in sync** — host branch == weft branch, via
 [`warp`](warp.md#drift-detection--when) — no half-finished prior run). Each producing phase emits
 a draft artifact and is followed by a review gate. `approved` advances to the next
@@ -253,7 +253,7 @@ small `.lyx/lyxrun.cmd` (machine-local, untracked — it embeds an absolute path
 that just does `cd <worktree>` then `lyx loom run`. Because everything is
 [cwd-authoritative](../overview.md#principles), the launcher needs no arguments — geometry resolves
 from cwd, so you cannot run it from the wrong place. It reuses the
-[launcher geometry](../overview.md#path-invariants) already in `internal/paths`.
+[launcher geometry](../overview.md#hub-geometry-invariants) already in `internal/hubgeometry`.
 
 **One terminal per worktree.** Scope for now is exactly that — each worktree its own terminal /
 psmux session. The cross-worktree multi-column view (all worktrees in one window) is a deferred mux
diff --git a/docs/modules/mux.md b/docs/modules/mux.md
index bc2136c..2219ef7 100644
--- a/docs/modules/mux.md
+++ b/docs/modules/mux.md
@@ -163,7 +163,7 @@ columns by slug. No architectural change — a metadata field and a rule.
    requirement: strip the inherited Claude-Code parent-session env (see [Resume](#resume-after-crash--native---resume-with-env-hygiene)).
 4. **One named psmux server per hub — the orphan firewall.** mux boots its server as
    `psmux -L lyx-<hub-basename>-<short-hash>` — a legible hub basename plus a short hash of the
-   hub's **absolute path**, derived deterministically via `internal/paths`. The hash is required
+   hub's **absolute path**, derived deterministically via `internal/hubgeometry`. The hash is required
    for two reasons: the name must be unique per absolute hub path (two hubs sharing a basename on
    different paths must not collide onto one server), and a raw path is not a valid `-L` name
    (`:` / `\` / spaces). The basename keeps it human-legible in `psmux ls` and `lyx mux status`;
diff --git a/docs/overview.md b/docs/overview.md
index 316cf4d..ebb5f54 100644
--- a/docs/overview.md
+++ b/docs/overview.md
@@ -61,11 +61,11 @@ Convenience alias: **`lyx run` → `lyx loom run`** (the everyday autonomous cal
    never *needed* (it would be strictly more work), and `lyx weft status` flags drift — but it is a
    friction asymmetry, not a wall.
 
-## Path Invariants
+## Hub Geometry Invariants
 
-**All worktree and Hub geometry resolves through `internal/paths`.**
+**All worktree and Hub geometry resolves through `internal/hubgeometry`.**
 
-The `internal/paths` package is the sole owner of cwd and worktree-root geometry math. It
+The `internal/hubgeometry` package is the sole owner of cwd and worktree-root geometry math. It
 exposes two entry points:
 
 - `Getwd()` — the only permitted call to `os.Getwd` outside `cmd/lyx/main.go`.
@@ -75,9 +75,9 @@ exposes two entry points:
 The `Layout` type provides geometry methods: `LyxDir()`, `WorktreePath(slug)`,
 `PortalsDir()`, `PortalLink(slug)`, `PortalTarget(slug)`, `LaunchersDir()`, `LauncherDir(slug)`, `MenuLauncherPath()`, `LauncherSpawnRel(slug)`, `MenuLauncherRel()`, `PrimeName()`, `WeftRepoRoot()`, `WeftWorktreePath(slug)`, `WeftWorktree()`, `WeftLyxDir()`, `WeftLyxDirFor(slug)`, `WeftCodeguideDir()`, `HostLyxLink(slug)`, `HostLyxLinkHere()`, `HostJunctions(slug)`.
 
-**Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned** outside `internal/paths`
+**Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned** outside `internal/hubgeometry`
 and `cmd/lyx/main.go`. The ban is enforced at `go test` / CI time by
-`internal/paths/enforcement_test.go`, which walks the entire source tree and fails the build
+`internal/hubgeometry/enforcement_test.go`, which walks the entire source tree and fails the build
 if either literal token is found in any non-test `.go` file outside the allowlist.
 
 See [CONSTRAINTS.md](../CONSTRAINTS.md) for details.
@@ -172,9 +172,11 @@ github.com/Knatte18/loomyard/
 ├── internal/idecli/              the ide CLI command
 ├── internal/ideengine/           the ide domain kernel
 ├── internal/muxpoccli/           the muxpoc POC module
+├── internal/ghissuescli/         the ghissues CLI command
+├── internal/ghissuesengine/      the ghissues domain kernel
 ├── internal/selfreportcli/       the selfreport CLI command
 ├── internal/selfreportengine/    the selfreport domain kernel
-├── internal/paths/               geometry resolver (the sole owner of cwd/root math)
+├── internal/hubgeometry/         geometry resolver (the sole owner of cwd/root math)
 ├── internal/configengine/        shared config resolution
 ├── internal/gitexec/             shared git operations
 ├── internal/lock/                shared file locking
@@ -239,7 +241,7 @@ back into mux — see [modules/mux.md](modules/mux.md#naming).)
 scaffolds the shared `_lyx/` config dir for every module.
 
 The user-facing modules sit on a thin layer of shared infrastructure
-(`internal/configengine`, `internal/gitexec`, `internal/lock`, `internal/output`, `internal/paths`, `internal/state`) — defined in
+(`internal/configengine`, `internal/gitexec`, `internal/lock`, `internal/output`, `internal/hubgeometry`, `internal/state`) — defined in
 [shared-libs/README.md](shared-libs/README.md).
 
 ## Execution stack (orchestration layers)
diff --git a/docs/roadmap.md b/docs/roadmap.md
index 9e4e0bc..235b56b 100644
--- a/docs/roadmap.md
+++ b/docs/roadmap.md
@@ -75,11 +75,11 @@ observable changes until the new module that needs the extracted lib arrives.
 4. **worktree module + portals, launchers, and ide module.** ✅ **Done.** Create / track / tear down
    git worktrees; manage container junctions and spawnable
    launchers; VS Code launcher with interactive menu; centralized path geometry
-   in `internal/paths`. Consumes `internal/configengine` + `internal/git`; owns the **junction-aware teardown**
+   in `internal/hubgeometry`. Consumes `internal/configengine` + `internal/git`; owns the **junction-aware teardown**
    sequence (the Windows locked-worktree hazard). The module is **stateless by design** — `lyx worktree list` is a thin
-   `git worktree list` wrapper; there is no worktree registry. Introduces `internal/paths` as the sole geometry owner, banning
-   raw `os.Getwd` and `git rev-parse --show-toplevel` outside `internal/paths` and `cmd/lyx/main.go`
-   via `internal/paths/enforcement_test.go`. (Portals are present and working — a subdir-mirrored
+   `git worktree list` wrapper; there is no worktree registry. Introduces `internal/hubgeometry` as the sole geometry owner, banning
+   raw `os.Getwd` and `git rev-parse --show-toplevel` outside `internal/hubgeometry` and `cmd/lyx/main.go`
+   via `internal/hubgeometry/enforcement_test.go`. (Portals are present and working — a subdir-mirrored
    Hub view of each worktree's `_lyx/`; kept available, not slated for removal.)
 
 5. **Task 006 — Weft engine.** ✅ **Done.** Path geometry for weft worktrees, paired host+weft spawn and teardown, `lyx weft` command (`status|commit|push|pull|sync`).
diff --git a/docs/shared-libs/README.md b/docs/shared-libs/README.md
index 97fe969..5137663 100644
--- a/docs/shared-libs/README.md
+++ b/docs/shared-libs/README.md
@@ -15,7 +15,7 @@ See [roadmap.md](../roadmap.md) milestones 2–3 for the extraction order.
 
 ## Libraries
 
-- [paths.md](paths.md) — `internal/paths`: canonical geometry resolver, sole owner of cwd/root math
+- [hubgeometry.md](hubgeometry.md) — `internal/hubgeometry`: canonical geometry resolver, sole owner of cwd/root math
 - [yamlengine.md](yamlengine.md) — `internal/yamlengine`: pure YAML engine for env expansion and config reconciliation
 - [envsource.md](envsource.md) — `internal/envsource`: single source of truth for environment variable sourcing (`.env` + OS overlay)
 - [configengine.md](configengine.md) — `internal/configengine`: strict YAML config loading backed by yamlengine and envsource
diff --git a/docs/shared-libs/configengine.md b/docs/shared-libs/configengine.md
index 7c694d5..afe80e3 100644
--- a/docs/shared-libs/configengine.md
+++ b/docs/shared-libs/configengine.md
@@ -34,7 +34,7 @@ The `Load(baseDir, module, template []byte)` function reads the on-disk config f
 **Flow:**
 
 1. Call `FindBaseDir(baseDir)` — check that `_lyx/` exists at baseDir.
-2. Read the config file at `paths.ConfigFile(baseDir, module)` (e.g., `_lyx/config/board.yaml`). If absent, return an error instructing the user to run `lyx config reconcile`.
+2. Read the config file at `hubgeometry.ConfigFile(baseDir, module)` (e.g., `_lyx/config/board.yaml`). If absent, return an error instructing the user to run `lyx config reconcile`.
 3. Check for missing template keys via `yamlengine.MissingKeys(template, fileBytes)`. If any keys are missing, return an error naming the file, the missing key-paths, and instructing the user to run `lyx config reconcile`.
 4. Build the environment via `envsource.Build(baseDir)` (reads `.env`, overlays OS env).
 5. Resolve environment variables via `yamlengine.Resolve(fileBytes, env)` (expands `${env:...}` markers).
@@ -67,7 +67,7 @@ Multiple markers in one value are all expanded. A value with no marker is a lite
 
 ## `.env` loading
 
-Environment variables are sourced by `envsource.Build(baseDir)`, which reads `paths.DotEnv(baseDir)` (typically `<cwd>/.env`) and overlays the OS environment.
+Environment variables are sourced by `envsource.Build(baseDir)`, which reads `hubgeometry.DotEnv(baseDir)` (typically `<cwd>/.env`) and overlays the OS environment.
 
 - **Format**: `KEY=VALUE` lines, blank lines skipped, lines starting with `#` are comments, split on first `=` only.
 - **Precedence: OS env wins.** Any variable set in the process environment overrides the corresponding `.env` value.
diff --git a/docs/shared-libs/envsource.md b/docs/shared-libs/envsource.md
index 1e3cf5a..ff5a0a1 100644
--- a/docs/shared-libs/envsource.md
+++ b/docs/shared-libs/envsource.md
@@ -2,7 +2,7 @@
 
 The **single source of truth** for how environment variables enter the system. It reads the `.env` file and overlays the OS environment into a unified map.
 
-**Dependency direction (Go enforces it):** `internal/envsource` imports `internal/paths` and stdlib only, never domain modules. All modules that need env data call `envsource.Build()`.
+**Dependency direction (Go enforces it):** `internal/envsource` imports `internal/hubgeometry` and stdlib only, never domain modules. All modules that need env data call `envsource.Build()`.
 
 ## Exported function
 
@@ -12,7 +12,7 @@ Reads and merges environment variables from `.env` and the OS environment.
 
 **Behavior:**
 
-1. Calls `paths.DotEnv(baseDir)` to compute the path to the `.env` file.
+1. Calls `hubgeometry.DotEnv(baseDir)` to compute the path to the `.env` file.
 2. Reads the `.env` file line-by-line, parsing `KEY=VALUE` pairs.
 3. Reads the OS environment via `os.Environ()`.
 4. Merges the two: OS values **take precedence** over `.env` values for any duplicate key.
diff --git a/docs/shared-libs/paths.md b/docs/shared-libs/hubgeometry.md
similarity index 90%
rename from docs/shared-libs/paths.md
rename to docs/shared-libs/hubgeometry.md
index b007152..0f7508d 100644
--- a/docs/shared-libs/paths.md
+++ b/docs/shared-libs/hubgeometry.md
@@ -1,12 +1,12 @@
-# `internal/paths`
+# `internal/hubgeometry`
 
 The **canonical geometry resolver** — the single owner of all worktree and Hub
 path math. Centralizes cwd/worktree-root handling so the `cwd ≠ git-repo-path` bug
 class never recurs.
 
-**Dependency direction (Go enforces it):** `internal/paths` imports only
+**Dependency direction (Go enforces it):** `internal/hubgeometry` imports only
 `internal/gitexec` + stdlib and **never** a domain module. All domain modules
-(`warp`, `board`, `ide`, `muxpoc`) import `paths` for geometry.
+(`warp`, `board`, `ide`, `muxpoc`) import `hubgeometry` for geometry.
 
 ## The problem
 
@@ -18,7 +18,7 @@ makes correctness structural, not a matter of discipline.
 
 ### Constants
 
-The following constants centralize every geometry and layout literal so no other package needs to repeat a string value. All production code that constructs paths from these names must import `internal/paths` and use these constants — never inline string literals.
+The following constants centralize every geometry and layout literal so no other package needs to repeat a string value. All production code that constructs paths from these names must import `internal/hubgeometry` and use these constants — never inline string literals.
 
 #### Layout constants
 
@@ -39,7 +39,7 @@ These three constants are the single source of the geometry tokens for the whole
 Returns the current working directory.
 
 **Behavior:** A thin wrapper over `os.Getwd`; the only permitted `os.Getwd` call
-outside `internal/paths` (and `cmd/lyx/main.go`).
+outside `internal/hubgeometry` (and `cmd/lyx/main.go`).
 
 **Returns:** On success, the cleaned absolute path of the cwd. On failure, an error
 (e.g., the cwd no longer exists).
@@ -131,9 +131,9 @@ These pure functions construct geometry paths without requiring a resolved `Layo
 
 ## Design principles
 
-**Geometry-only.** `paths` computes *where* things are, never *mutates* them.
+**Geometry-only.** `hubgeometry` computes *where* things are, never *mutates* them.
 Worktree creation/removal, junction setup, and config scaffolding stay in the
-domain modules. `paths` is the dumb geometry resolver so they can be smart about
+domain modules. `hubgeometry` is the dumb geometry resolver so they can be smart about
 state transitions.
 
 **Single call per invocation.** Most callsites invoke `Resolve(cwd)` once at the
@@ -141,36 +141,36 @@ start of a command and re-use the returned `Layout` throughout. This amortizes a
 git calls and normalization upfront.
 
 **Normalization in one place.** Forward slashes from `git rev-parse --show-toplevel`
-vs backslashes from `os.Getwd()` are reconciled once in `paths` via
+vs backslashes from `os.Getwd()` are reconciled once in `hubgeometry` via
 `filepath.FromSlash` + `filepath.Clean`, so callers never deal with mixed forms.
 
-**Config resolution stays cwd-authoritative.** `paths.Resolve` is geometry-only and
+**Config resolution stays cwd-authoritative.** `hubgeometry.Resolve` is geometry-only and
 does NOT check for `_lyx/`. The cwd-authoritative config invariant (`_lyx/` must
 exist at cwd) remains enforced by `internal/configengine.FindBaseDir`. Board and other
-modules keep passing `cwd` to their `LoadConfig` (obtained via `paths.Getwd`). This
+modules keep passing `cwd` to their `LoadConfig` (obtained via `hubgeometry.Getwd`). This
 lets `board init` (pre-init, no `_lyx/`) and other early-stage commands call into
-`paths` without a spurious "not initialized" failure.
+`hubgeometry` without a spurious "not initialized" failure.
 
-**Mirrored system dirs never enumerate the worktree.** `paths` only derives Loomyard's
+**Mirrored system dirs never enumerate the worktree.** `hubgeometry` only derives Loomyard's
 own system directories (`_lyx`, `_portals`, `_launchers`) from `RelPath` and never
 enumerates or mirrors user content. A nested or git-ignored `_codeguide` sibling
 (or any other sibling repo) is never mirrored as a subpath-specific copy.
 
 ## The enforcement wall
 
-`internal/paths/enforcement_test.go` runs two repo-wide AST scans on every
-`go test ./internal/paths/...` run:
+`internal/hubgeometry/enforcement_test.go` runs two repo-wide AST scans on every
+`go test ./internal/hubgeometry/...` run:
 
 **`TestEnforcement` (cwd/root primitives ban):**
 Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside
-`internal/paths` and `cmd/lyx/main.go`. The scan uses a substring check on the
+`internal/hubgeometry` and `cmd/lyx/main.go`. The scan uses a substring check on the
 raw file bytes and fails the build if either token appears in any non-test `.go`
 file outside the allowlist.
 
 **`TestEnforcement_GeometryLiterals` (geometry-literal construction ban):**
 The geometry path tokens `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`,
 `_codeguide`, and `_lyx` may not appear as string literals in a
-**path-construction context** in any production file outside `internal/paths`.
+**path-construction context** in any production file outside `internal/hubgeometry`.
 Path-construction contexts are:
 
 - An argument to a `filepath.Join(...)` call.
diff --git a/internal/boardcli/cli.go b/internal/boardcli/cli.go
index 33f95a4..565639f 100644
--- a/internal/boardcli/cli.go
+++ b/internal/boardcli/cli.go
@@ -3,7 +3,7 @@
 // Command() returns the root "board" command with 11 subcommands. Configuration
 // resolution happens once in a PersistentPreRunE: the config file (home, sidebar,
 // proposal_prefix) is loaded from _lyx/config/board.yaml, and the board data dir
-// is resolved as paths.BoardDir(layout.Hub) via paths.Resolve. The hidden
+// is resolved as hubgeometry.BoardDir(layout.Hub) via hubgeometry.Resolve. The hidden
 // --board-path persistent flag overrides the data dir for the detached sync child
 // process launched by spawn.go, bypassing both config and path resolution.
 
@@ -17,8 +17,8 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/boardengine"
 	"github.com/Knatte18/loomyard/internal/clihelp"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/output"
-	"github.com/Knatte18/loomyard/internal/paths"
 	"github.com/spf13/cobra"
 )
 
@@ -77,7 +77,7 @@ available subcommands without requiring a git repo.`,
 			cfg = boardengine.Config{Path: *boardPathFlag}
 		} else {
 			// Resolve configuration from the current working directory.
-			cwd, err := paths.Getwd()
+			cwd, err := hubgeometry.Getwd()
 			if err != nil {
 				output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to get working directory: %v", err))
 				clihelp.Abort(ctx, 1)
@@ -94,13 +94,13 @@ available subcommands without requiring a git repo.`,
 			// Resolve the worktree layout to derive the board data dir. The board
 			// data dir is geometry (<hub>/_board) and must come from paths, not from
 			// the config file or an environment variable.
-			layout, rerr := paths.Resolve(cwd)
+			layout, rerr := hubgeometry.Resolve(cwd)
 			if rerr != nil {
 				output.Err(cmd.OutOrStdout(), rerr.Error())
 				clihelp.Abort(ctx, 1)
 				return nil
 			}
-			cfg.Path = paths.BoardDir(layout.Hub)
+			cfg.Path = hubgeometry.BoardDir(layout.Hub)
 		}
 
 		// Fold BOARD_SKIP_* env into cfg at the single production entry point.
diff --git a/internal/boardcli/cli_test.go b/internal/boardcli/cli_test.go
index 1eb7b9b..683d7a0 100644
--- a/internal/boardcli/cli_test.go
+++ b/internal/boardcli/cli_test.go
@@ -5,7 +5,7 @@
 // 1 for error), and each verb's distinctive field (task, tasks[], Home.md written).
 //
 // Board data dir strategy: seedCwd initialises a git repo (git init) in the cwd
-// so that PersistentPreRunE can call paths.Resolve without error. The board data
+// so that PersistentPreRunE can call hubgeometry.Resolve without error. The board data
 // dir is then Hub/_board where Hub = filepath.Dir(cwd). This is the production
 // code path; no --board-path injection is used for operational tests.
 
@@ -21,34 +21,34 @@ import (
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/boardcli"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // seedCwd creates a temp directory with _lyx/config/board.yaml seeded with all
 // template keys (home, sidebar, proposal_prefix; path: is not a template key),
-// initialises a git repo there (so paths.Resolve succeeds), changes to that
+// initialises a git repo there (so hubgeometry.Resolve succeeds), changes to that
 // directory, and returns the cwd path. The board data dir is Hub/_board where
-// Hub = filepath.Dir(cwd); callers can compute it as paths.BoardDir(filepath.Dir(cwd)).
+// Hub = filepath.Dir(cwd); callers can compute it as hubgeometry.BoardDir(filepath.Dir(cwd)).
 func seedCwd(t *testing.T) string {
 	t.Helper()
 
 	cwd := t.TempDir()
 
-	// Initialise a git repo so PersistentPreRunE can call paths.Resolve without error.
+	// Initialise a git repo so PersistentPreRunE can call hubgeometry.Resolve without error.
 	if out, err := exec.Command("git", "-C", cwd, "init").CombinedOutput(); err != nil {
 		t.Fatalf("git init: %v\n%s", err, out)
 	}
 
-	if err := os.MkdirAll(filepath.Join(cwd, paths.LyxDirName), 0o755); err != nil {
+	if err := os.MkdirAll(filepath.Join(cwd, hubgeometry.LyxDirName), 0o755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	if err := os.MkdirAll(paths.ConfigDir(cwd), 0o755); err != nil {
+	if err := os.MkdirAll(hubgeometry.ConfigDir(cwd), 0o755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
 
 	// Write board config with all template keys; path: is no longer a template key.
 	configContent := "home: Home.md\nsidebar: _Sidebar.md\nproposal_prefix: proposal-\n"
-	if err := os.WriteFile(paths.ConfigFile(cwd, "board"), []byte(configContent), 0o644); err != nil {
+	if err := os.WriteFile(hubgeometry.ConfigFile(cwd, "board"), []byte(configContent), 0o644); err != nil {
 		t.Fatalf("failed to write board.yaml: %v", err)
 	}
 
@@ -168,8 +168,8 @@ func TestCLIContract(t *testing.T) {
 			wantFieldExist: "ok",
 			assertFieldExists: func(t *testing.T, result map[string]any, cwd string) {
 				// seedCwd initialised a git repo at cwd; Hub = filepath.Dir(cwd);
-				// paths.Resolve derives Hub from the git root, so board renders at Hub/_board.
-				homePath := filepath.Join(paths.BoardDir(filepath.Dir(cwd)), "Home.md")
+				// hubgeometry.Resolve derives Hub from the git root, so board renders at Hub/_board.
+				homePath := filepath.Join(hubgeometry.BoardDir(filepath.Dir(cwd)), "Home.md")
 				if _, err := os.Stat(homePath); err != nil {
 					t.Fatalf("Home.md not created at %q: %v", homePath, err)
 				}
@@ -815,15 +815,15 @@ func TestCLILookupContract(t *testing.T) {
 }
 
 // TestCLIBoardPathResolution verifies the two board data dir resolution paths in
-// PersistentPreRunE: without --board-path the CLI uses paths.BoardDir(hub) derived
-// from paths.Resolve; with --board-path the supplied path takes precedence.
-// This test initialises a real git repo so that paths.Resolve succeeds.
+// PersistentPreRunE: without --board-path the CLI uses hubgeometry.BoardDir(hub) derived
+// from hubgeometry.Resolve; with --board-path the supplied path takes precedence.
+// This test initialises a real git repo so that hubgeometry.Resolve succeeds.
 func TestCLIBoardPathResolution(t *testing.T) {
 	t.Setenv("BOARD_SKIP_GIT", "1")
 
 	// Build a two-level fixture: topDir is the Hub; worktree is a git repo inside it.
-	// paths.Resolve(worktree) derives Hub = topDir, so
-	// paths.BoardDir(Hub) = filepath.Join(topDir, "_board").
+	// hubgeometry.Resolve(worktree) derives Hub = topDir, so
+	// hubgeometry.BoardDir(Hub) = filepath.Join(topDir, "_board").
 	topDir := t.TempDir()
 	worktree := filepath.Join(topDir, "worktree")
 	if err := os.MkdirAll(worktree, 0o755); err != nil {
@@ -834,21 +834,21 @@ func TestCLIBoardPathResolution(t *testing.T) {
 	}
 
 	// Seed _lyx/config/board.yaml without path: (not a template key).
-	if err := os.MkdirAll(paths.ConfigDir(worktree), 0o755); err != nil {
+	if err := os.MkdirAll(hubgeometry.ConfigDir(worktree), 0o755); err != nil {
 		t.Fatalf("mkdir config: %v", err)
 	}
 	configContent := "home: Home.md\nsidebar: _Sidebar.md\nproposal_prefix: proposal-\n"
-	if err := os.WriteFile(paths.ConfigFile(worktree, "board"), []byte(configContent), 0o644); err != nil {
+	if err := os.WriteFile(hubgeometry.ConfigFile(worktree, "board"), []byte(configContent), 0o644); err != nil {
 		t.Fatalf("write board.yaml: %v", err)
 	}
 
-	// Change to the worktree so paths.Getwd() in the CLI returns it.
+	// Change to the worktree so hubgeometry.Getwd() in the CLI returns it.
 	t.Chdir(worktree)
 
-	expectedBoardDir := paths.BoardDir(topDir)
+	expectedBoardDir := hubgeometry.BoardDir(topDir)
 
 	t.Run("no_board_path_resolves_via_paths", func(t *testing.T) {
-		// PersistentPreRunE calls paths.Resolve and derives cfg.Path = BoardDir(topDir).
+		// PersistentPreRunE calls hubgeometry.Resolve and derives cfg.Path = BoardDir(topDir).
 		// Upsert writes tasks.json inside that derived board dir.
 		exitCode, stdout := runCLI(t, "upsert", `{"slug":"path-test","title":"Path Test"}`)
 		if exitCode != 0 {
@@ -856,12 +856,12 @@ func TestCLIBoardPathResolution(t *testing.T) {
 		}
 		tasksFile := filepath.Join(expectedBoardDir, "tasks.json")
 		if _, err := os.Stat(tasksFile); err != nil {
-			t.Errorf("board not at paths.BoardDir(hub) %q: %v", expectedBoardDir, err)
+			t.Errorf("board not at hubgeometry.BoardDir(hub) %q: %v", expectedBoardDir, err)
 		}
 	})
 
 	t.Run("board_path_flag_overrides_resolution", func(t *testing.T) {
-		// --board-path bypasses paths.Resolve and uses the supplied path directly.
+		// --board-path bypasses hubgeometry.Resolve and uses the supplied path directly.
 		// list is a read-only operation that works under --board-path (no render step),
 		// so we verify redirection by comparing task counts: absOverride is a fresh
 		// empty dir (0 tasks) while expectedBoardDir already has the "path-test" task
diff --git a/internal/boardengine/boardtest/bench_test.go b/internal/boardengine/boardtest/bench_test.go
index 7a5cd51..3479d79 100644
--- a/internal/boardengine/boardtest/bench_test.go
+++ b/internal/boardengine/boardtest/bench_test.go
@@ -18,7 +18,7 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/boardcli"
 	"github.com/Knatte18/loomyard/internal/boardengine"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // benchSizes is the set of board sizes (number of tasks already in tasks.json)
@@ -38,15 +38,15 @@ func seedWiki(tb testing.TB, n int) string {
 	dir := tb.TempDir()
 
 	// Create _lyx and _lyx/config directories with board.yaml config
-	lyxDir := filepath.Join(dir, paths.LyxDirName)
+	lyxDir := filepath.Join(dir, hubgeometry.LyxDirName)
 	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
 		tb.Fatalf("mkdir _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(dir)
+	configDir := hubgeometry.ConfigDir(dir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		tb.Fatalf("mkdir _lyx/config: %v", err)
 	}
-	configPath := paths.ConfigFile(dir, "board")
+	configPath := hubgeometry.ConfigFile(dir, "board")
 	if err := os.WriteFile(configPath, []byte("path: board\nhome: Home.md\nsidebar: _Sidebar.md\nproposal_prefix: proposal-\n"), 0o644); err != nil {
 		tb.Fatalf("write board.yaml: %v", err)
 	}
diff --git a/internal/boardengine/config.go b/internal/boardengine/config.go
index da4420c..df02e56 100644
--- a/internal/boardengine/config.go
+++ b/internal/boardengine/config.go
@@ -18,7 +18,7 @@ import (
 // Config represents the configuration for a board module.
 type Config struct {
 	// Path is the absolute path to the board data directory. It is set by the
-	// caller (boardcli.Command's PersistentPreRunE via paths.BoardDir or the
+	// caller (boardcli.Command's PersistentPreRunE via hubgeometry.BoardDir or the
 	// --board-path flag), never by the config file. yaml:"-" prevents the
 	// yaml.v3 unmarshaller from mapping any leftover path: key onto this field.
 	Path           string `yaml:"-"`
@@ -57,7 +57,7 @@ func (c Config) Outputs() Outputs {
 //
 // LoadConfig no longer resolves a data-dir path. Config.Path is always empty
 // on return; the caller is responsible for setting it (boardcli sets it via
-// paths.BoardDir or the --board-path flag).
+// hubgeometry.BoardDir or the --board-path flag).
 func LoadConfig(baseDir, module string) (Config, error) {
 	// Load and resolve the config file using the template.
 	resolved, err := configengine.Load(baseDir, module, []byte(ConfigTemplate()))
diff --git a/internal/boardengine/config_test.go b/internal/boardengine/config_test.go
index 6da6e66..109ace9 100644
--- a/internal/boardengine/config_test.go
+++ b/internal/boardengine/config_test.go
@@ -13,27 +13,27 @@ import (
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/boardengine"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // TestLoadConfig_HappyPath tests that LoadConfig loads a valid config
 // with all template keys present and resolves environment variables.
-// LoadConfig no longer sets Config.Path; the caller does that via paths.BoardDir.
+// LoadConfig no longer sets Config.Path; the caller does that via hubgeometry.BoardDir.
 func TestLoadConfig_HappyPath(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/config/ directories
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
 
 	// Write a config file with all template keys (path: is not a template key)
-	configFile := paths.ConfigFile(tmpDir, "board")
+	configFile := hubgeometry.ConfigFile(tmpDir, "board")
 	content := `home: Home.md
 sidebar: _Sidebar.md
 proposal_prefix: proposal-
@@ -47,7 +47,7 @@ proposal_prefix: proposal-
 		t.Fatalf("unexpected error: %v", err)
 	}
 
-	// Path is never set by LoadConfig; the caller sets it via paths.BoardDir.
+	// Path is never set by LoadConfig; the caller sets it via hubgeometry.BoardDir.
 	if cfg.Path != "" {
 		t.Errorf("expected Path to be empty after LoadConfig; got %q", cfg.Path)
 	}
@@ -64,23 +64,23 @@ proposal_prefix: proposal-
 
 // TestLoadConfig_AbsolutePathResolution verifies that a path: key in the config
 // file is ignored by LoadConfig because Config.Path has yaml:"-".
-// The board data dir is geometry owned by paths.BoardDir; the config key is a no-op.
+// The board data dir is geometry owned by hubgeometry.BoardDir; the config key is a no-op.
 func TestLoadConfig_AbsolutePathResolution(t *testing.T) {
 	tmpDir := t.TempDir()
 	absBoard := t.TempDir()
 
 	// Create _lyx/config/ directories
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
 
 	// Write config with an absolute path: key that should be ignored.
-	configFile := paths.ConfigFile(tmpDir, "board")
+	configFile := hubgeometry.ConfigFile(tmpDir, "board")
 	content := `path: ` + absBoard + `
 home: Home.md
 sidebar: _Sidebar.md
@@ -104,22 +104,22 @@ proposal_prefix: proposal-
 // TestLoadConfig_RelativePathResolution verifies that a relative path: key in the
 // config file is ignored by LoadConfig because Config.Path has yaml:"-".
 // LoadConfig no longer performs any relative-path resolution; the board data dir
-// is geometry owned by paths.BoardDir.
+// is geometry owned by hubgeometry.BoardDir.
 func TestLoadConfig_RelativePathResolution(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/config/ directories
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
 
 	// Write config with a relative path: key that should be ignored.
-	configFile := paths.ConfigFile(tmpDir, "board")
+	configFile := hubgeometry.ConfigFile(tmpDir, "board")
 	content := `path: ../custom_board
 home: Home.md
 sidebar: _Sidebar.md
@@ -144,24 +144,24 @@ proposal_prefix: proposal-
 // TestLoadConfig_EnvResolution verifies that a path: key using ${env:...} syntax
 // in the config file is ignored by LoadConfig because Config.Path has yaml:"-".
 // The env-override mechanism for the board data dir has been removed; the data
-// dir is now geometry owned by paths.BoardDir and is not env-overridable.
+// dir is now geometry owned by hubgeometry.BoardDir and is not env-overridable.
 func TestLoadConfig_EnvResolution(t *testing.T) {
 	tmpDir := t.TempDir()
 	absBoard := t.TempDir()
 	t.Setenv("TEST_BOARD_PATH", absBoard)
 
 	// Create _lyx/config/ directories
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
 
 	// Write config with an env-variable path: key that should be ignored.
-	configFile := paths.ConfigFile(tmpDir, "board")
+	configFile := hubgeometry.ConfigFile(tmpDir, "board")
 	content := `path: ${env:TEST_BOARD_PATH}
 home: Home.md
 sidebar: _Sidebar.md
diff --git a/internal/boardengine/template_test.go b/internal/boardengine/template_test.go
index a0f6f85..9197127 100644
--- a/internal/boardengine/template_test.go
+++ b/internal/boardengine/template_test.go
@@ -25,7 +25,7 @@ func TestConfigTemplate_ValidYAML(t *testing.T) {
 // TestConfigTemplate_HasRequiredKeys asserts that the template contains
 // all expected configuration keys (home, sidebar, proposal_prefix).
 // The geometry key path is intentionally absent — board data dir is now
-// owned by paths.BoardDir, not the config file.
+// owned by hubgeometry.BoardDir, not the config file.
 func TestConfigTemplate_HasRequiredKeys(t *testing.T) {
 	got := ConfigTemplate()
 	var result map[string]any
diff --git a/internal/configcli/configcli.go b/internal/configcli/configcli.go
index ed3a2a9..83219e8 100644
--- a/internal/configcli/configcli.go
+++ b/internal/configcli/configcli.go
@@ -19,8 +19,8 @@ import (
 	"github.com/Knatte18/loomyard/internal/configengine"
 	"github.com/Knatte18/loomyard/internal/configreg"
 	"github.com/Knatte18/loomyard/internal/configsync"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/output"
-	"github.com/Knatte18/loomyard/internal/paths"
 	"github.com/Knatte18/loomyard/internal/weftcli"
 )
 
@@ -41,7 +41,7 @@ func printModule(baseDir string, out io.Writer, module string) int {
 		return output.Err(out, fmt.Sprintf("unknown config module: %s (known: %v)", module, configreg.Names()))
 	}
 
-	path := paths.ConfigFile(baseDir, module)
+	path := hubgeometry.ConfigFile(baseDir, module)
 	data, err := os.ReadFile(path)
 	if err != nil {
 		if os.IsNotExist(err) {
@@ -70,7 +70,7 @@ func printAll(baseDir string, out io.Writer) int {
 		// Write a section delimiter so the reader can separate module blocks.
 		fmt.Fprintf(out, "# %s\n", name)
 
-		path := paths.ConfigFile(baseDir, name)
+		path := hubgeometry.ConfigFile(baseDir, name)
 		data, err := os.ReadFile(path)
 		if err != nil {
 			if os.IsNotExist(err) {
@@ -138,7 +138,7 @@ func editOne(baseDir string, out io.Writer, module string, edit configengine.Edi
 // When printOnly is true the command is read-only: it writes on-disk YAML to out
 // without opening an editor. The print path is evaluated before any edit/menu logic.
 // The baseDir is computed from the layout as filepath.Join(WorktreeRoot, RelPath).
-func dispatch(l *paths.Layout, in io.Reader, out io.Writer, args []string, edit configengine.EditorFunc, sync syncFunc, printOnly bool) int {
+func dispatch(l *hubgeometry.Layout, in io.Reader, out io.Writer, args []string, edit configengine.EditorFunc, sync syncFunc, printOnly bool) int {
 	baseDir := filepath.Join(l.WorktreeRoot, l.RelPath)
 
 	// Handle --print before any edit/menu dispatch; the print path is read-only
@@ -175,12 +175,12 @@ func buildConfigLong() string {
 // 1 on any error.
 func runReconcile(out io.Writer, apply bool) int {
 	// Resolve the current working directory and layout.
-	cwd, err := paths.Getwd()
+	cwd, err := hubgeometry.Getwd()
 	if err != nil {
 		return output.Err(out, fmt.Sprintf("getwd: %v", err))
 	}
 
-	l, err := paths.Resolve(cwd)
+	l, err := hubgeometry.Resolve(cwd)
 	if err != nil {
 		return output.Err(out, fmt.Sprintf("resolve layout: %v", err))
 	}
@@ -275,13 +275,13 @@ func RunCLI(out io.Writer, args []string) int {
 // without opening an editor or running sync.
 func runConfig(out io.Writer, args []string, printOnly bool) int {
 	// Resolve the current working directory.
-	cwd, err := paths.Getwd()
+	cwd, err := hubgeometry.Getwd()
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
 
 	// Resolve the layout.
-	l, err := paths.Resolve(cwd)
+	l, err := hubgeometry.Resolve(cwd)
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
diff --git a/internal/configcli/configcli_integration_test.go b/internal/configcli/configcli_integration_test.go
index d0c6fe8..31de118 100644
--- a/internal/configcli/configcli_integration_test.go
+++ b/internal/configcli/configcli_integration_test.go
@@ -15,8 +15,8 @@ import (
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/configreg"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 	"github.com/Knatte18/loomyard/internal/warpengine"
 	"github.com/Knatte18/loomyard/internal/weftcli"
 )
@@ -53,9 +53,9 @@ func TestE2ESyncIntegration(t *testing.T) {
 
 	// Resolve layout for the new host worktree.
 	hostWorktreePath := f.Layout.WorktreePath(slug)
-	hostLayout, err := paths.Resolve(hostWorktreePath)
+	hostLayout, err := hubgeometry.Resolve(hostWorktreePath)
 	if err != nil {
-		t.Fatalf("paths.Resolve(%q): %v", hostWorktreePath, err)
+		t.Fatalf("hubgeometry.Resolve(%q): %v", hostWorktreePath, err)
 	}
 
 	// Chdir into the host worktree so weft.RunCLI's cwd resolution lands on the fixture.
@@ -89,7 +89,7 @@ func TestE2ESyncIntegration(t *testing.T) {
 
 	// Assert _lyx/config/warpengine.yaml is tracked/committed in the weft worktree.
 	weftWorktreePath := f.Layout.WeftWorktreePath(slug)
-	configRelPath := paths.ConfigFile(".", "warp")
+	configRelPath := hubgeometry.ConfigFile(".", "warp")
 	configPath := filepath.Join(weftWorktreePath, configRelPath)
 	// For git commands, use forward slashes (git always uses forward slashes).
 	configRelPathForGit := strings.ReplaceAll(configRelPath, "\\", "/")
diff --git a/internal/configcli/configcli_test.go b/internal/configcli/configcli_test.go
index 031f198..fe5bfaa 100644
--- a/internal/configcli/configcli_test.go
+++ b/internal/configcli/configcli_test.go
@@ -18,7 +18,7 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/configengine"
 	"github.com/Knatte18/loomyard/internal/configreg"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // fakeEditor returns a fake EditorFunc that writes the given valid YAML
@@ -51,13 +51,13 @@ func TestEditOneSuccess(t *testing.T) {
 	baseDir := t.TempDir()
 
 	// Create _lyx/config directory
-	configDir := paths.ConfigDir(baseDir)
+	configDir := hubgeometry.ConfigDir(baseDir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("failed to create config dir: %v", err)
 	}
 
 	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
-	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
+	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
 		t.Fatalf("failed to write board.yaml: %v", err)
 	}
 
@@ -82,13 +82,13 @@ func TestEditOneUnknownModule(t *testing.T) {
 	baseDir := t.TempDir()
 
 	// Create _lyx/config directory
-	configDir := paths.ConfigDir(baseDir)
+	configDir := hubgeometry.ConfigDir(baseDir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("failed to create config dir: %v", err)
 	}
 
 	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
-	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
+	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
 		t.Fatalf("failed to write board.yaml: %v", err)
 	}
 
@@ -126,13 +126,13 @@ func TestEditOneAbort(t *testing.T) {
 	baseDir := t.TempDir()
 
 	// Create _lyx/config directory
-	configDir := paths.ConfigDir(baseDir)
+	configDir := hubgeometry.ConfigDir(baseDir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("failed to create config dir: %v", err)
 	}
 
 	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
-	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
+	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
 		t.Fatalf("failed to write board.yaml: %v", err)
 	}
 
@@ -157,13 +157,13 @@ func TestEditOneSyncFails(t *testing.T) {
 	baseDir := t.TempDir()
 
 	// Create _lyx/config directory
-	configDir := paths.ConfigDir(baseDir)
+	configDir := hubgeometry.ConfigDir(baseDir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("failed to create config dir: %v", err)
 	}
 
 	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
-	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
+	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
 		t.Fatalf("failed to write board.yaml: %v", err)
 	}
 
@@ -193,17 +193,17 @@ func TestMenuSelection(t *testing.T) {
 	baseDir := t.TempDir()
 
 	// Create _lyx/config directory
-	configDir := paths.ConfigDir(baseDir)
+	configDir := hubgeometry.ConfigDir(baseDir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("failed to create config dir: %v", err)
 	}
 
 	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
-	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
+	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
 		t.Fatalf("failed to write board.yaml: %v", err)
 	}
 
-	l := &paths.Layout{
+	l := &hubgeometry.Layout{
 		WorktreeRoot: baseDir,
 		RelPath:      ".",
 	}
@@ -231,17 +231,17 @@ func TestMenuQuit(t *testing.T) {
 	baseDir := t.TempDir()
 
 	// Create _lyx/config directory
-	configDir := paths.ConfigDir(baseDir)
+	configDir := hubgeometry.ConfigDir(baseDir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("failed to create config dir: %v", err)
 	}
 
 	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
-	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
+	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
 		t.Fatalf("failed to write board.yaml: %v", err)
 	}
 
-	l := &paths.Layout{
+	l := &hubgeometry.Layout{
 		WorktreeRoot: baseDir,
 		RelPath:      ".",
 	}
@@ -264,17 +264,17 @@ func TestMenuInvalidSelection(t *testing.T) {
 	baseDir := t.TempDir()
 
 	// Create _lyx/config directory
-	configDir := paths.ConfigDir(baseDir)
+	configDir := hubgeometry.ConfigDir(baseDir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("failed to create config dir: %v", err)
 	}
 
 	// Create a fake _lyx/config/board.yaml to satisfy FindBaseDir
-	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
+	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# temp\n"), 0o644); err != nil {
 		t.Fatalf("failed to write board.yaml: %v", err)
 	}
 
-	l := &paths.Layout{
+	l := &hubgeometry.Layout{
 		WorktreeRoot: baseDir,
 		RelPath:      ".",
 	}
@@ -301,21 +301,21 @@ func TestMenuStatus(t *testing.T) {
 	baseDir := t.TempDir()
 
 	// Create _lyx/config directory
-	configDir := paths.ConfigDir(baseDir)
+	configDir := hubgeometry.ConfigDir(baseDir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("failed to create config dir: %v", err)
 	}
 
 	// Create board.yaml and warp.yaml to mark them as (configured)
-	if err := os.WriteFile(paths.ConfigFile(baseDir, "board"), []byte("# board\n"), 0o644); err != nil {
+	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "board"), []byte("# board\n"), 0o644); err != nil {
 		t.Fatalf("failed to write board.yaml: %v", err)
 	}
-	if err := os.WriteFile(paths.ConfigFile(baseDir, "warp"), []byte("# warp\n"), 0o644); err != nil {
+	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, "warp"), []byte("# warp\n"), 0o644); err != nil {
 		t.Fatalf("failed to write warp.yaml: %v", err)
 	}
 	// weft.yaml not created, so it should show (default)
 
-	l := &paths.Layout{
+	l := &hubgeometry.Layout{
 		WorktreeRoot: baseDir,
 		RelPath:      ".",
 	}
@@ -348,9 +348,9 @@ func makeNeverCalledEditor(t *testing.T) configengine.EditorFunc {
 	}
 }
 
-// makeLayoutAt returns a minimal *paths.Layout with WorktreeRoot at baseDir and RelPath ".".
-func makeLayoutAt(baseDir string) *paths.Layout {
-	return &paths.Layout{
+// makeLayoutAt returns a minimal *hubgeometry.Layout with WorktreeRoot at baseDir and RelPath ".".
+func makeLayoutAt(baseDir string) *hubgeometry.Layout {
+	return &hubgeometry.Layout{
 		WorktreeRoot: baseDir,
 		RelPath:      ".",
 	}
@@ -359,11 +359,11 @@ func makeLayoutAt(baseDir string) *paths.Layout {
 // seedModuleConfig writes YAML content to the config file for the named module under baseDir.
 func seedModuleConfig(t *testing.T, baseDir, module, content string) {
 	t.Helper()
-	dir := paths.ConfigDir(baseDir)
+	dir := hubgeometry.ConfigDir(baseDir)
 	if err := os.MkdirAll(dir, 0o755); err != nil {
 		t.Fatalf("failed to create config dir: %v", err)
 	}
-	if err := os.WriteFile(paths.ConfigFile(baseDir, module), []byte(content), 0o644); err != nil {
+	if err := os.WriteFile(hubgeometry.ConfigFile(baseDir, module), []byte(content), 0o644); err != nil {
 		t.Fatalf("failed to seed config for module %s: %v", module, err)
 	}
 }
@@ -411,7 +411,7 @@ func TestPrintModule_Seeded(t *testing.T) {
 func TestPrintModule_KnownButUnseeded(t *testing.T) {
 	baseDir := t.TempDir()
 	// Create the config directory but not the warp.yaml file.
-	if err := os.MkdirAll(paths.ConfigDir(baseDir), 0o755); err != nil {
+	if err := os.MkdirAll(hubgeometry.ConfigDir(baseDir), 0o755); err != nil {
 		t.Fatalf("failed to create config dir: %v", err)
 	}
 
diff --git a/internal/configcli/menu.go b/internal/configcli/menu.go
index e3bf2ed..fa5f473 100644
--- a/internal/configcli/menu.go
+++ b/internal/configcli/menu.go
@@ -14,7 +14,7 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/configengine"
 	"github.com/Knatte18/loomyard/internal/configreg"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // menu presents an interactive picker of available config modules.
@@ -27,14 +27,14 @@ import (
 // Handles 'q' to quit (return 0).
 // Parses selection as 1-indexed number, validates range, routes to editOne on valid choice.
 // Returns the exit code from editOne or an error code (1) on invalid input.
-func menu(l *paths.Layout, baseDir string, in io.Reader, out io.Writer, edit configengine.EditorFunc, sync syncFunc) int {
+func menu(l *hubgeometry.Layout, baseDir string, in io.Reader, out io.Writer, edit configengine.EditorFunc, sync syncFunc) int {
 	// Get the list of available modules.
 	names := configreg.Names()
 
 	// Print numbered picker with configured/default status.
 	for i, name := range names {
 		// Check if config file exists.
-		configPath := paths.ConfigFile(baseDir, name)
+		configPath := hubgeometry.ConfigFile(baseDir, name)
 		_, err := os.Stat(configPath)
 		status := "(default)"
 		if err == nil {
diff --git a/internal/configcli/reconcile_test.go b/internal/configcli/reconcile_test.go
index d7dce88..d40c078 100644
--- a/internal/configcli/reconcile_test.go
+++ b/internal/configcli/reconcile_test.go
@@ -12,7 +12,7 @@ import (
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/gitexec"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // TestReconcile_DryRun verifies that "lyx config reconcile" without --apply writes
@@ -21,25 +21,25 @@ import (
 func TestReconcile_DryRun(t *testing.T) {
 	tmpDir := t.TempDir()
 
-	// Initialize a minimal git repo so paths.Resolve works.
+	// Initialize a minimal git repo so hubgeometry.Resolve works.
 	_, _, exitCode, err := gitexec.RunGit([]string{"init"}, tmpDir)
 	if err != nil || exitCode != 0 {
 		t.Fatalf("git init failed: %v (exit code %d)", err, exitCode)
 	}
 
 	// Create config directory with a sample board file.
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("mkdir config: %v", err)
 	}
 
-	boardPath := paths.ConfigFile(tmpDir, "board")
+	boardPath := hubgeometry.ConfigFile(tmpDir, "board")
 	originalContent := "path: board\nstale_key: old_value\n"
 	if err := os.WriteFile(boardPath, []byte(originalContent), 0o644); err != nil {
 		t.Fatalf("write board.yaml: %v", err)
 	}
 
-	// Chdir into the temp repo so paths.Getwd inside RunCLI resolves to a git repo.
+	// Chdir into the temp repo so hubgeometry.Getwd inside RunCLI resolves to a git repo.
 	oldCwd, err2 := os.Getwd()
 	if err2 != nil {
 		t.Fatalf("getwd: %v", err2)
@@ -118,7 +118,7 @@ func TestReconcile_Apply(t *testing.T) {
 	}
 
 	// Create config directory.
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("mkdir config: %v", err)
 	}
@@ -157,7 +157,7 @@ func TestReconcile_Apply(t *testing.T) {
 	}
 
 	// Verify weft.yaml was created on disk.
-	weftPath := paths.ConfigFile(tmpDir, "weft")
+	weftPath := hubgeometry.ConfigFile(tmpDir, "weft")
 	if _, err := os.Stat(weftPath); err != nil {
 		t.Errorf("weft.yaml not created: %v", err)
 	}
diff --git a/internal/configengine/config.go b/internal/configengine/config.go
index efec183..7acfb46 100644
--- a/internal/configengine/config.go
+++ b/internal/configengine/config.go
@@ -13,7 +13,7 @@ import (
 	"path/filepath"
 
 	"github.com/Knatte18/loomyard/internal/envsource"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/yamlengine"
 )
 
@@ -22,7 +22,7 @@ import (
 // It performs a strict check without walking up to parent directories.
 // Returns the cwd on success, empty string and an error on failure.
 func FindBaseDir(cwd string) (string, error) {
-	lyxDir := filepath.Join(cwd, paths.LyxDirName)
+	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
 	_, err := os.Stat(lyxDir)
 	if os.IsNotExist(err) {
 		return "", fmt.Errorf("not initialized: _lyx/ directory not found")
@@ -35,14 +35,14 @@ func FindBaseDir(cwd string) (string, error) {
 // Load loads and resolves configuration from a YAML file using a template.
 //
 // Flow:
-// 1. Call FindBaseDir(baseDir) and propagate its error.
-// 2. Compute cfgPath := paths.ConfigFile(baseDir, module) and read it.
-//    If the file is absent, return an error naming the path and instructing "lyx config reconcile".
-// 3. Check for missing keys in the file via yamlengine.MissingKeys(template, fileBytes).
-//    If keys are missing, return an error naming cfgPath, the missing key-paths, and "lyx config reconcile".
-// 4. Build the environment via envsource.Build(baseDir).
-// 5. Resolve fileBytes via yamlengine.Resolve(fileBytes, env).
-// 6. Return the resolved bytes.
+//  1. Call FindBaseDir(baseDir) and propagate its error.
+//  2. Compute cfgPath := hubgeometry.ConfigFile(baseDir, module) and read it.
+//     If the file is absent, return an error naming the path and instructing "lyx config reconcile".
+//  3. Check for missing keys in the file via yamlengine.MissingKeys(template, fileBytes).
+//     If keys are missing, return an error naming cfgPath, the missing key-paths, and "lyx config reconcile".
+//  4. Build the environment via envsource.Build(baseDir).
+//  5. Resolve fileBytes via yamlengine.Resolve(fileBytes, env).
+//  6. Return the resolved bytes.
 //
 // Errors from steps 3-5 wrap the underlying error with the config key/file context.
 func Load(baseDir, module string, template []byte) ([]byte, error) {
@@ -53,7 +53,7 @@ func Load(baseDir, module string, template []byte) ([]byte, error) {
 	}
 
 	// Step 2: Read the config file
-	cfgPath := paths.ConfigFile(baseDir, module)
+	cfgPath := hubgeometry.ConfigFile(baseDir, module)
 	fileBytes, err := os.ReadFile(cfgPath)
 	if os.IsNotExist(err) {
 		return nil, fmt.Errorf("config file %s not found; run \"lyx config reconcile\"", cfgPath)
diff --git a/internal/configengine/config_test.go b/internal/configengine/config_test.go
index 4dd747c..af73874 100644
--- a/internal/configengine/config_test.go
+++ b/internal/configengine/config_test.go
@@ -13,7 +13,7 @@ import (
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/configengine"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"gopkg.in/yaml.v3"
 )
 
@@ -22,11 +22,11 @@ func TestLoad_HappyPath(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/config/ directories
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
@@ -35,7 +35,7 @@ func TestLoad_HappyPath(t *testing.T) {
 	template := []byte("path: _board\nhome: Home.md\n")
 
 	// Write config file matching template
-	yamlFile := paths.ConfigFile(tmpDir, "board")
+	yamlFile := hubgeometry.ConfigFile(tmpDir, "board")
 	if err := os.WriteFile(yamlFile, []byte("path: custom_path\nhome: Index.md\n"), 0644); err != nil {
 		t.Fatalf("failed to write board.yaml: %v", err)
 	}
@@ -64,11 +64,11 @@ func TestLoad_MissingKey(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/config/ directories
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
@@ -77,7 +77,7 @@ func TestLoad_MissingKey(t *testing.T) {
 	template := []byte("path: _board\nhome: Home.md\n")
 
 	// Config file missing "home" key
-	yamlFile := paths.ConfigFile(tmpDir, "board")
+	yamlFile := hubgeometry.ConfigFile(tmpDir, "board")
 	if err := os.WriteFile(yamlFile, []byte("path: custom_path\n"), 0644); err != nil {
 		t.Fatalf("failed to write board.yaml: %v", err)
 	}
@@ -104,11 +104,11 @@ func TestLoad_AbsentFile(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/config/ directories but NOT board.yaml
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
@@ -134,11 +134,11 @@ func TestLoad_EnvResolution(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/config/ directories
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
@@ -150,7 +150,7 @@ func TestLoad_EnvResolution(t *testing.T) {
 	template := []byte("path: ${env:TEST_CONFIG_VAR}\n")
 
 	// Config file with the same env marker
-	yamlFile := paths.ConfigFile(tmpDir, "board")
+	yamlFile := hubgeometry.ConfigFile(tmpDir, "board")
 	if err := os.WriteFile(yamlFile, []byte("path: ${env:TEST_CONFIG_VAR}\n"), 0644); err != nil {
 		t.Fatalf("failed to write board.yaml: %v", err)
 	}
@@ -176,11 +176,11 @@ func TestLoad_OptionalEnv(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/config/ directories
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
@@ -191,7 +191,7 @@ func TestLoad_OptionalEnv(t *testing.T) {
 	template := []byte("path: ${env:TEST_OPTIONAL_VAR:-default_path}\n")
 
 	// Config file with optional env
-	yamlFile := paths.ConfigFile(tmpDir, "board")
+	yamlFile := hubgeometry.ConfigFile(tmpDir, "board")
 	if err := os.WriteFile(yamlFile, []byte("path: ${env:TEST_OPTIONAL_VAR:-default_path}\n"), 0644); err != nil {
 		t.Fatalf("failed to write board.yaml: %v", err)
 	}
@@ -217,11 +217,11 @@ func TestLoad_ExtraKeyTolerated(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/config/ directories
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
@@ -230,7 +230,7 @@ func TestLoad_ExtraKeyTolerated(t *testing.T) {
 	template := []byte("path: _board\n")
 
 	// Config file with extra key
-	yamlFile := paths.ConfigFile(tmpDir, "board")
+	yamlFile := hubgeometry.ConfigFile(tmpDir, "board")
 	if err := os.WriteFile(yamlFile, []byte("path: custom_path\nextra_key: extra_value\n"), 0644); err != nil {
 		t.Fatalf("failed to write board.yaml: %v", err)
 	}
@@ -269,11 +269,11 @@ func TestLoad_NestedKeyTemplate(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/config/ directories
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
@@ -282,7 +282,7 @@ func TestLoad_NestedKeyTemplate(t *testing.T) {
 	template := []byte("server:\n  host: localhost\n  port: '8080'\n")
 
 	// Config file with nested values
-	yamlFile := paths.ConfigFile(tmpDir, "test")
+	yamlFile := hubgeometry.ConfigFile(tmpDir, "test")
 	if err := os.WriteFile(yamlFile, []byte("server:\n  host: example.com\n  port: '9090'\n"), 0644); err != nil {
 		t.Fatalf("failed to write test.yaml: %v", err)
 	}
@@ -316,7 +316,7 @@ func TestFindBaseDir_Present(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/ directory
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
diff --git a/internal/configengine/edit.go b/internal/configengine/edit.go
index c0b2648..7867502 100644
--- a/internal/configengine/edit.go
+++ b/internal/configengine/edit.go
@@ -14,7 +14,7 @@ import (
 	"os/exec"
 	"runtime"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"gopkg.in/yaml.v3"
 )
 
@@ -57,21 +57,21 @@ func DefaultEditor(path string) error {
 // on validation failure.
 //
 // Flow:
-// 1. Call FindBaseDir(baseDir) to ensure initialization; propagate error if not.
-// 2. Compute path = paths.ConfigFile(baseDir, module).
-// 3. If path does not exist, write template to it (scaffold; 0o644) and track
-//    that this call created the file (scaffolded := true).
-// 4. Loop:
-//    a. Record the file bytes.
-//    b. Call edit(path); if it returns an error, abort.
-//    c. Re-read the bytes and yaml.Unmarshal into map[string]any to validate.
-//    d. On parse success, return nil.
-//    e. On parse failure, if bytes unchanged from pre-edit snapshot, abort
-//       (operator saved without fixing); otherwise print the parse error to
-//       os.Stderr and loop to re-open the editor.
-// 5. Abort means: if scaffolded, os.Remove the file so the filesystem returns to
-//    its pre-call state; then return ErrAborted (wrapping the editor error when
-//    applicable). When the file pre-existed, abort leaves it as-is.
+//  1. Call FindBaseDir(baseDir) to ensure initialization; propagate error if not.
+//  2. Compute path = hubgeometry.ConfigFile(baseDir, module).
+//  3. If path does not exist, write template to it (scaffold; 0o644) and track
+//     that this call created the file (scaffolded := true).
+//  4. Loop:
+//     a. Record the file bytes.
+//     b. Call edit(path); if it returns an error, abort.
+//     c. Re-read the bytes and yaml.Unmarshal into map[string]any to validate.
+//     d. On parse success, return nil.
+//     e. On parse failure, if bytes unchanged from pre-edit snapshot, abort
+//     (operator saved without fixing); otherwise print the parse error to
+//     os.Stderr and loop to re-open the editor.
+//  5. Abort means: if scaffolded, os.Remove the file so the filesystem returns to
+//     its pre-call state; then return ErrAborted (wrapping the editor error when
+//     applicable). When the file pre-existed, abort leaves it as-is.
 //
 // Validation is syntactic only (the file must parse as YAML); known keys are
 // not enforced.
@@ -83,7 +83,7 @@ func Edit(baseDir, module, template string, edit EditorFunc) error {
 	}
 
 	// Compute the config file path via paths helper.
-	path := paths.ConfigFile(baseDir, module)
+	path := hubgeometry.ConfigFile(baseDir, module)
 
 	// Check if the file already exists.
 	_, err = os.Stat(path)
@@ -92,7 +92,7 @@ func Edit(baseDir, module, template string, edit EditorFunc) error {
 	// If the file does not exist, scaffold it from the template.
 	if scaffolded {
 		// Create _lyx/config/ directory if needed.
-		configDir := paths.ConfigDir(baseDir)
+		configDir := hubgeometry.ConfigDir(baseDir)
 		if err := os.MkdirAll(configDir, 0755); err != nil {
 			return fmt.Errorf("create config directory: %w", err)
 		}
diff --git a/internal/configengine/edit_test.go b/internal/configengine/edit_test.go
index cd488a8..c209919 100644
--- a/internal/configengine/edit_test.go
+++ b/internal/configengine/edit_test.go
@@ -14,7 +14,7 @@ import (
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/configengine"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // TestEdit_ScaffoldWhenMissing tests that Edit writes the template to
@@ -24,7 +24,7 @@ func TestEdit_ScaffoldWhenMissing(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/ directory (the file itself will be scaffolded).
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
@@ -50,7 +50,7 @@ func TestEdit_ScaffoldWhenMissing(t *testing.T) {
 	}
 
 	// Verify the file exists in the right place.
-	expectedPath := paths.ConfigFile(tmpDir, "testmod")
+	expectedPath := hubgeometry.ConfigFile(tmpDir, "testmod")
 	if _, err := os.Stat(expectedPath); err != nil {
 		t.Errorf("config file not found at %s: %v", expectedPath, err)
 	}
@@ -63,16 +63,16 @@ func TestEdit_EditExistingFile(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/ and _lyx/config/ with a pre-existing config file.
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
 
-	existingPath := paths.ConfigFile(tmpDir, "testmod")
+	existingPath := hubgeometry.ConfigFile(tmpDir, "testmod")
 	originalContent := "original: value\n"
 	if err := os.WriteFile(existingPath, []byte(originalContent), 0644); err != nil {
 		t.Fatalf("failed to write config file: %v", err)
@@ -106,7 +106,7 @@ func TestEdit_ReEditLoop(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/ directory.
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
@@ -142,7 +142,7 @@ func TestEdit_AbortOnUnchangedAfterFailure_Scaffolded(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/ directory.
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
@@ -167,7 +167,7 @@ func TestEdit_AbortOnUnchangedAfterFailure_Scaffolded(t *testing.T) {
 	}
 
 	// Verify the scaffolded file was removed.
-	configPath := paths.ConfigFile(tmpDir, "testmod")
+	configPath := hubgeometry.ConfigFile(tmpDir, "testmod")
 	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
 		t.Errorf("config file still exists after abort; should have been removed")
 	}
@@ -180,16 +180,16 @@ func TestEdit_AbortOnUnchangedAfterFailure_PreExisting(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/ and _lyx/config/ with a pre-existing config file.
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
 
-	existingPath := paths.ConfigFile(tmpDir, "testmod")
+	existingPath := hubgeometry.ConfigFile(tmpDir, "testmod")
 	originalContent := "original: value\n"
 	if err := os.WriteFile(existingPath, []byte(originalContent), 0644); err != nil {
 		t.Fatalf("failed to write config file: %v", err)
@@ -227,7 +227,7 @@ func TestEdit_AbortOnEditorError(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/ directory.
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
@@ -251,7 +251,7 @@ func TestEdit_AbortOnEditorError(t *testing.T) {
 	}
 
 	// Verify the scaffolded file was removed.
-	configPath := paths.ConfigFile(tmpDir, "testmod")
+	configPath := hubgeometry.ConfigFile(tmpDir, "testmod")
 	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
 		t.Errorf("config file still exists after abort; should have been removed")
 	}
diff --git a/internal/configsync/configsync.go b/internal/configsync/configsync.go
index c99cdc6..87adad4 100644
--- a/internal/configsync/configsync.go
+++ b/internal/configsync/configsync.go
@@ -11,7 +11,7 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/configreg"
 	"github.com/Knatte18/loomyard/internal/fsx"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/yamlengine"
 )
 
@@ -30,7 +30,7 @@ type Result struct {
 // ReconcileAll reconciles all module config files against their templates.
 //
 // For each module returned by configreg.Modules(), it:
-//   - Computes cfgPath := paths.ConfigFile(baseDir, m.Name)
+//   - Computes cfgPath := hubgeometry.ConfigFile(baseDir, m.Name)
 //   - Reads existing bytes from disk (absent file → empty []byte, not an error)
 //   - Calls yamlengine.Reconcile([]byte(m.Template()), existing)
 //   - When apply && (fileAbsent || len(added)+len(removed) > 0):
@@ -44,7 +44,7 @@ func ReconcileAll(baseDir string, apply bool) ([]Result, error) {
 	var results []Result
 
 	for _, m := range configreg.Modules() {
-		cfgPath := paths.ConfigFile(baseDir, m.Name)
+		cfgPath := hubgeometry.ConfigFile(baseDir, m.Name)
 
 		// Read existing config file (missing file → empty bytes)
 		existing, err := os.ReadFile(cfgPath)
diff --git a/internal/configsync/configsync_test.go b/internal/configsync/configsync_test.go
index 5208afa..a710216 100644
--- a/internal/configsync/configsync_test.go
+++ b/internal/configsync/configsync_test.go
@@ -6,18 +6,18 @@ import (
 	"os"
 	"testing"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 func TestReconcileAll_DryRun(t *testing.T) {
 	tmpDir := t.TempDir()
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("mkdir: %v", err)
 	}
 
 	// Seed board.yaml with a missing key and a stale key
-	boardPath := paths.ConfigFile(tmpDir, "board")
+	boardPath := hubgeometry.ConfigFile(tmpDir, "board")
 	if err := os.WriteFile(boardPath, []byte("path: board\nstale_key: old_value\n"), 0o644); err != nil {
 		t.Fatalf("write board.yaml: %v", err)
 	}
@@ -65,13 +65,13 @@ func TestReconcileAll_DryRun(t *testing.T) {
 
 func TestReconcileAll_ApplyCreatesFiles(t *testing.T) {
 	tmpDir := t.TempDir()
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("mkdir: %v", err)
 	}
 
 	// Seed board.yaml
-	boardPath := paths.ConfigFile(tmpDir, "board")
+	boardPath := hubgeometry.ConfigFile(tmpDir, "board")
 	if err := os.WriteFile(boardPath, []byte("path: board\nstale_key: old_value\n"), 0o644); err != nil {
 		t.Fatalf("write board.yaml: %v", err)
 	}
@@ -111,7 +111,7 @@ func TestReconcileAll_ApplyCreatesFiles(t *testing.T) {
 	}
 
 	// Verify weft.yaml was created
-	weftPath := paths.ConfigFile(tmpDir, "weft")
+	weftPath := hubgeometry.ConfigFile(tmpDir, "weft")
 	if _, err := os.Stat(weftPath); err != nil {
 		t.Errorf("weft.yaml was not created: %v", err)
 	}
@@ -132,7 +132,7 @@ func TestReconcileAll_ApplyCreatesFiles(t *testing.T) {
 
 func TestReconcileAll_Idempotent(t *testing.T) {
 	tmpDir := t.TempDir()
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("mkdir: %v", err)
 	}
diff --git a/internal/envsource/envsource.go b/internal/envsource/envsource.go
index c0e0499..44042dd 100644
--- a/internal/envsource/envsource.go
+++ b/internal/envsource/envsource.go
@@ -9,10 +9,10 @@ import (
 	"os"
 	"strings"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
-// Build reads the .env file at paths.DotEnv(baseDir) and overlays the OS environment,
+// Build reads the .env file at hubgeometry.DotEnv(baseDir) and overlays the OS environment,
 // returning a merged map where OS values take precedence over .env values.
 //
 // The .env file is parsed line-by-line, skipping blank lines and lines beginning with #.
@@ -25,7 +25,7 @@ import (
 // Returns the merged map on success, or an error if the .env file cannot be read.
 func Build(baseDir string) (map[string]string, error) {
 	// Read the .env file
-	dotEnvPath := paths.DotEnv(baseDir)
+	dotEnvPath := hubgeometry.DotEnv(baseDir)
 	dotEnvMap, err := readDotEnv(dotEnvPath)
 	if err != nil {
 		return nil, err
diff --git a/internal/paths/codeguide_guard_test.go b/internal/hubgeometry/codeguide_guard_test.go
similarity index 87%
rename from internal/paths/codeguide_guard_test.go
rename to internal/hubgeometry/codeguide_guard_test.go
index 7a5ade1..26df2bd 100644
--- a/internal/paths/codeguide_guard_test.go
+++ b/internal/hubgeometry/codeguide_guard_test.go
@@ -1,10 +1,10 @@
-// codeguide_guard_test.go is a guard to ensure that internal/paths never
-// discovers or enumerates the _codeguide directory. This documents that paths
+// codeguide_guard_test.go is a guard to ensure that internal/hubgeometry never
+// discovers or enumerates the _codeguide directory. This documents that hubgeometry
 // never scans the worktree to mirror dirs — a future nested/ignored _codeguide
 // can never be treated as a sibling. Geometry methods like WeftCodeguideDir() are
 // exceptions: they compute paths purely via filepath.Join with no discovery logic.
 
-package paths
+package hubgeometry
 
 import (
 	"io/fs"
@@ -15,7 +15,7 @@ import (
 	"testing"
 )
 
-// TestCodeguideGuard verifies that no production source file in internal/paths
+// TestCodeguideGuard verifies that no production source file in internal/hubgeometry
 // contains the literal substring _codeguide.
 func TestCodeguideGuard(t *testing.T) {
 	t.Run("tree-scan", func(t *testing.T) {
@@ -24,7 +24,7 @@ func TestCodeguideGuard(t *testing.T) {
 		if !ok {
 			t.Fatal("could not determine test file location")
 		}
-		// One level up from internal/paths/codeguide_guard_test.go → package dir
+		// One level up from internal/hubgeometry/codeguide_guard_test.go → package dir
 		pkgDir := filepath.Dir(file)
 
 		// Predicate: returns true if the bytes contain _codeguide.
@@ -42,10 +42,10 @@ func TestCodeguideGuard(t *testing.T) {
 
 			// Only check .go files that are not _test.go files.
 			if !d.IsDir() && strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
-				// Skip paths.go: it contains geometry methods like WeftCodeguideDir() that compute
+				// Skip hubgeometry.go: it contains geometry methods like WeftCodeguideDir() that compute
 				// paths purely via filepath.Join, which is allowed. The guard applies only to
 				// discovery/enumeration logic, not to geometry computation.
-				if d.Name() == "paths.go" {
+				if d.Name() == "hubgeometry.go" {
 					return nil
 				}
 				data, err := os.ReadFile(path)
diff --git a/internal/paths/enforcement_test.go b/internal/hubgeometry/enforcement_test.go
similarity index 90%
rename from internal/paths/enforcement_test.go
rename to internal/hubgeometry/enforcement_test.go
index 3d117dc..bfa404c 100644
--- a/internal/paths/enforcement_test.go
+++ b/internal/hubgeometry/enforcement_test.go
@@ -1,8 +1,8 @@
 // enforcement_test.go is a repo-wide guard: it walks every package and fails
-// the build if any file outside internal/paths reaches for raw cwd or top-level
-// git geometry, keeping internal/paths the sole geometry owner.
+// the build if any file outside internal/hubgeometry reaches for raw cwd or top-level
+// git geometry, keeping internal/hubgeometry the sole geometry owner.
 
-package paths
+package hubgeometry
 
 import (
 	"go/ast"
@@ -18,7 +18,7 @@ import (
 )
 
 // TestEnforcement walks the repo source tree and verifies that no source file
-// outside internal/paths and cmd/lyx contains the raw cwd/root primitives
+// outside internal/hubgeometry and cmd/lyx contains the raw cwd/root primitives
 // os.Getwd or git rev-parse --show-toplevel.
 func TestEnforcement(t *testing.T) {
 	t.Run("tree-scan", func(t *testing.T) {
@@ -27,7 +27,7 @@ func TestEnforcement(t *testing.T) {
 		if !ok {
 			t.Fatal("could not determine test file location")
 		}
-		// Two levels up from internal/paths/enforcement_test.go → repo root
+		// Two levels up from internal/hubgeometry/enforcement_test.go → repo root
 		repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(file)))
 
 		// Predicate: returns true if the bytes contain a banned token.
@@ -65,8 +65,8 @@ func TestEnforcement(t *testing.T) {
 				// Normalize path separators to forward slashes for comparison.
 				pkgDir = filepath.ToSlash(pkgDir)
 
-				// Check allowlist: internal/paths, cmd/lyx/main.go
-				isAllowed := pkgDir == "internal/paths" ||
+				// Check allowlist: internal/hubgeometry, cmd/lyx/main.go
+				isAllowed := pkgDir == "internal/hubgeometry" ||
 					(pkgDir == "cmd/lyx" && d.Name() == "main.go")
 
 				// Skip files in the allowlist (they are allowed to contain banned tokens).
@@ -137,7 +137,7 @@ func TestEnforcement(t *testing.T) {
 }
 
 // TestEnforcement_GeometryLiterals walks the repo source tree and verifies that no
-// production file outside internal/paths constructs a geometry path token as a string
+// production file outside internal/hubgeometry constructs a geometry path token as a string
 // literal in a path-construction context: a filepath.Join argument, a binary +
 // operand, or a string const declaration value. Whole-token matching (exact equality,
 // not substring) avoids false positives on compound names such as "_boardroom" or
@@ -145,7 +145,7 @@ func TestEnforcement(t *testing.T) {
 // rule, not a machine-enforced invariant.
 func TestEnforcement_GeometryLiterals(t *testing.T) {
 	// geometryToken reports whether s is exactly one of the forbidden geometry path
-	// tokens. Only internal/paths is permitted to use these in path-construction context.
+	// tokens. Only internal/hubgeometry is permitted to use these in path-construction context.
 	geometryToken := func(s string) bool {
 		switch s {
 		case "_board", "-weft", "-HUB", "_portals", "_launchers", "_codeguide", "_lyx":
@@ -311,14 +311,14 @@ func TestEnforcement_GeometryLiterals(t *testing.T) {
 	})
 
 	// tree-scan sub-test: walks every production Go file in the repo (excluding test
-	// files and the internal/paths allowlist) and fails if any file constructs a
+	// files and the internal/hubgeometry allowlist) and fails if any file constructs a
 	// geometry token in a path context.
 	t.Run("tree-scan", func(t *testing.T) {
 		_, thisFile, _, ok := runtime.Caller(0)
 		if !ok {
 			t.Fatal("could not determine test file location via runtime.Caller")
 		}
-		// Two filepath.Dir calls walk from internal/paths/enforcement_test.go → repo root.
+		// Two filepath.Dir calls walk from internal/hubgeometry/enforcement_test.go → repo root.
 		repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
 
 		var scanned int
@@ -342,9 +342,9 @@ func TestEnforcement_GeometryLiterals(t *testing.T) {
 			if relErr != nil {
 				return relErr
 			}
-			// Allowlist: internal/paths is the sole permitted owner of geometry literals
+			// Allowlist: internal/hubgeometry is the sole permitted owner of geometry literals
 			// in path-construction context.
-			if filepath.ToSlash(filepath.Dir(relPath)) == "internal/paths" {
+			if filepath.ToSlash(filepath.Dir(relPath)) == "internal/hubgeometry" {
 				return nil
 			}
 
@@ -364,17 +364,17 @@ func TestEnforcement_GeometryLiterals(t *testing.T) {
 			t.Fatalf("failed to walk repo tree: %v", err)
 		}
 
-		// Sanity check: at least one production file outside internal/paths must have
+		// Sanity check: at least one production file outside internal/hubgeometry must have
 		// been scanned so a misconfigured walk (wrong root, all files skipped) cannot
 		// silently produce a vacuous all-pass result.
 		t.Run("scanned_non_empty", func(t *testing.T) {
 			if scanned == 0 {
-				t.Error("geometry-literal guard: no production Go files scanned outside internal/paths; the AST walk may be misconfigured")
+				t.Error("geometry-literal guard: no production Go files scanned outside internal/hubgeometry; the AST walk may be misconfigured")
 			}
 		})
 
 		if len(failures) > 0 {
-			t.Errorf("geometry-literal construction found outside internal/paths in:\n%v", failures)
+			t.Errorf("geometry-literal construction found outside internal/hubgeometry in:\n%v", failures)
 		}
 	})
 }
diff --git a/internal/paths/geometry_test.go b/internal/hubgeometry/geometry_test.go
similarity index 81%
rename from internal/paths/geometry_test.go
rename to internal/hubgeometry/geometry_test.go
index 7852cf6..f029b36 100644
--- a/internal/paths/geometry_test.go
+++ b/internal/hubgeometry/geometry_test.go
@@ -2,13 +2,13 @@
 // parser added in the paths-foundation batch. It also asserts parity between the
 // refactored weft Layout methods and their WeftSiblingPath equivalents.
 
-package paths_test
+package hubgeometry_test
 
 import (
 	"path/filepath"
 	"testing"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // TestWeftSiblingPath verifies that WeftSiblingPath joins hub and slug with WeftSuffix.
@@ -25,19 +25,19 @@ func TestWeftSiblingPath(t *testing.T) {
 			name: "simple slug",
 			hub:  "/h",
 			slug: "feat",
-			want: filepath.Join("/h", "feat"+paths.WeftSuffix),
+			want: filepath.Join("/h", "feat"+hubgeometry.WeftSuffix),
 		},
 		{
 			name: "nested hub",
 			hub:  "/repos/loomyard-HUB",
 			slug: "main",
-			want: filepath.Join("/repos/loomyard-HUB", "main"+paths.WeftSuffix),
+			want: filepath.Join("/repos/loomyard-HUB", "main"+hubgeometry.WeftSuffix),
 		},
 	}
 
 	for _, tt := range tests {
 		t.Run(tt.name, func(t *testing.T) {
-			got := paths.WeftSiblingPath(tt.hub, tt.slug)
+			got := hubgeometry.WeftSiblingPath(tt.hub, tt.slug)
 			if got != tt.want {
 				t.Errorf("WeftSiblingPath(%q, %q) = %q; want %q", tt.hub, tt.slug, got, tt.want)
 			}
@@ -57,18 +57,18 @@ func TestBoardDir(t *testing.T) {
 		{
 			name: "simple hub",
 			hub:  "/h",
-			want: filepath.Join("/h", paths.BoardDirName),
+			want: filepath.Join("/h", hubgeometry.BoardDirName),
 		},
 		{
 			name: "nested hub",
 			hub:  "/repos/loomyard-HUB",
-			want: filepath.Join("/repos/loomyard-HUB", paths.BoardDirName),
+			want: filepath.Join("/repos/loomyard-HUB", hubgeometry.BoardDirName),
 		},
 	}
 
 	for _, tt := range tests {
 		t.Run(tt.name, func(t *testing.T) {
-			got := paths.BoardDir(tt.hub)
+			got := hubgeometry.BoardDir(tt.hub)
 			if got != tt.want {
 				t.Errorf("BoardDir(%q) = %q; want %q", tt.hub, got, tt.want)
 			}
@@ -90,19 +90,19 @@ func TestHubPath(t *testing.T) {
 			name:     "simple repo name",
 			parent:   "/repos",
 			repoName: "loomyard",
-			want:     filepath.Join("/repos", "loomyard"+paths.HubSuffix),
+			want:     filepath.Join("/repos", "loomyard"+hubgeometry.HubSuffix),
 		},
 		{
 			name:     "nested parent",
 			parent:   "/home/user/code",
 			repoName: "myproject",
-			want:     filepath.Join("/home/user/code", "myproject"+paths.HubSuffix),
+			want:     filepath.Join("/home/user/code", "myproject"+hubgeometry.HubSuffix),
 		},
 	}
 
 	for _, tt := range tests {
 		t.Run(tt.name, func(t *testing.T) {
-			got := paths.HubPath(tt.parent, tt.repoName)
+			got := hubgeometry.HubPath(tt.parent, tt.repoName)
 			if got != tt.want {
 				t.Errorf("HubPath(%q, %q) = %q; want %q", tt.parent, tt.repoName, got, tt.want)
 			}
@@ -154,7 +154,7 @@ func TestWeftHostSlug(t *testing.T) {
 
 	for _, tt := range tests {
 		t.Run(tt.name, func(t *testing.T) {
-			gotSlug, gotOK := paths.WeftHostSlug(tt.input)
+			gotSlug, gotOK := hubgeometry.WeftHostSlug(tt.input)
 			if gotSlug != tt.wantSlug || gotOK != tt.wantOK {
 				t.Errorf("WeftHostSlug(%q) = (%q, %v); want (%q, %v)",
 					tt.input, gotSlug, gotOK, tt.wantSlug, tt.wantOK)
@@ -172,7 +172,7 @@ func TestWeftLayoutMethodParity(t *testing.T) {
 	prime := filepath.Join(hub, "main")
 	slug := "feat"
 
-	layout := &paths.Layout{
+	layout := &hubgeometry.Layout{
 		Cwd:          filepath.Join(hub, slug),
 		WorktreeRoot: filepath.Join(hub, slug),
 		Hub:          hub,
@@ -182,7 +182,7 @@ func TestWeftLayoutMethodParity(t *testing.T) {
 
 	// WeftWorktreePath(slug) must equal WeftSiblingPath(hub, slug).
 	gotWorktreePath := layout.WeftWorktreePath(slug)
-	wantWorktreePath := paths.WeftSiblingPath(hub, slug)
+	wantWorktreePath := hubgeometry.WeftSiblingPath(hub, slug)
 	if gotWorktreePath != wantWorktreePath {
 		t.Errorf("WeftWorktreePath(%q) = %q; want WeftSiblingPath(%q, %q) = %q",
 			slug, gotWorktreePath, hub, slug, wantWorktreePath)
@@ -190,7 +190,7 @@ func TestWeftLayoutMethodParity(t *testing.T) {
 
 	// WeftRepoRoot() must equal WeftSiblingPath(hub, filepath.Base(prime)).
 	gotRepoRoot := layout.WeftRepoRoot()
-	wantRepoRoot := paths.WeftSiblingPath(hub, filepath.Base(prime))
+	wantRepoRoot := hubgeometry.WeftSiblingPath(hub, filepath.Base(prime))
 	if gotRepoRoot != wantRepoRoot {
 		t.Errorf("WeftRepoRoot() = %q; want WeftSiblingPath(%q, %q) = %q",
 			gotRepoRoot, hub, filepath.Base(prime), wantRepoRoot)
@@ -198,7 +198,7 @@ func TestWeftLayoutMethodParity(t *testing.T) {
 
 	// WeftWorktree() must equal WeftSiblingPath(hub, filepath.Base(WorktreeRoot)).
 	gotWorktree := layout.WeftWorktree()
-	wantWorktree := paths.WeftSiblingPath(hub, filepath.Base(layout.WorktreeRoot))
+	wantWorktree := hubgeometry.WeftSiblingPath(hub, filepath.Base(layout.WorktreeRoot))
 	if gotWorktree != wantWorktree {
 		t.Errorf("WeftWorktree() = %q; want WeftSiblingPath(%q, %q) = %q",
 			gotWorktree, hub, filepath.Base(layout.WorktreeRoot), wantWorktree)
diff --git a/internal/paths/paths.go b/internal/hubgeometry/hubgeometry.go
similarity index 99%
rename from internal/paths/paths.go
rename to internal/hubgeometry/hubgeometry.go
index eecc4c0..c563e22 100644
--- a/internal/paths/paths.go
+++ b/internal/hubgeometry/hubgeometry.go
@@ -1,8 +1,8 @@
-// Package paths is the single owner of Loomyard worktree and container geometry.
+// Package hubgeometry is the single owner of Loomyard worktree and container geometry.
 // It resolves the active Layout from a working directory and exposes typed
 // accessors for every derived path, so no other package recomputes geometry
 // from raw os.Getwd or git --show-toplevel calls.
-package paths
+package hubgeometry
 
 import (
 	"errors"
diff --git a/internal/paths/paths_test.go b/internal/hubgeometry/hubgeometry_test.go
similarity index 92%
rename from internal/paths/paths_test.go
rename to internal/hubgeometry/hubgeometry_test.go
index 0fc3ab0..1522d70 100644
--- a/internal/paths/paths_test.go
+++ b/internal/hubgeometry/hubgeometry_test.go
@@ -1,9 +1,9 @@
 //go:build integration
 
-// paths_test.go covers Layout resolution, the geometry accessors, and the
+// hubgeometry_test.go covers Layout resolution, the geometry accessors, and the
 // ErrNotAGitRepo path for directories outside a git repo.
 
-package paths_test
+package hubgeometry_test
 
 import (
 	"errors"
@@ -11,8 +11,8 @@ import (
 	"path/filepath"
 	"testing"
 
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 )
 
 // TestResolve_FromWorktreeRoot verifies that Resolve from the worktree root
@@ -23,7 +23,7 @@ func TestResolve_FromWorktreeRoot(t *testing.T) {
 	fix := lyxtest.CopyHostHub(t)
 	hub := fix.Hub
 
-	layout, err := paths.Resolve(hub)
+	layout, err := hubgeometry.Resolve(hub)
 	if err != nil {
 		t.Fatalf("Resolve() error = %v; want nil", err)
 	}
@@ -68,7 +68,7 @@ func TestResolve_FromSubdirectory(t *testing.T) {
 		t.Fatalf("failed to create subdirectory: %v", err)
 	}
 
-	layout, err := paths.Resolve(subDir)
+	layout, err := hubgeometry.Resolve(subDir)
 	if err != nil {
 		t.Fatalf("Resolve() error = %v; want nil", err)
 	}
@@ -96,7 +96,7 @@ func TestResolve_GeometryMethods(t *testing.T) {
 	fix := lyxtest.CopyHostHub(t)
 	hub := fix.Hub
 
-	layout, err := paths.Resolve(hub)
+	layout, err := hubgeometry.Resolve(hub)
 	if err != nil {
 		t.Fatalf("Resolve() error = %v; want nil", err)
 	}
@@ -154,7 +154,7 @@ func TestResolve_ForwardSlashNormalization(t *testing.T) {
 	hub := fix.Hub
 
 	// Call Resolve normally; both cwd and --show-toplevel output get normalized
-	layout, err := paths.Resolve(hub)
+	layout, err := hubgeometry.Resolve(hub)
 	if err != nil {
 		t.Fatalf("Resolve() error = %v; want nil", err)
 	}
@@ -176,13 +176,13 @@ func TestResolve_NotAGitRepo(t *testing.T) {
 
 	nonGitDir := t.TempDir()
 
-	layout, err := paths.Resolve(nonGitDir)
+	layout, err := hubgeometry.Resolve(nonGitDir)
 
 	if layout != nil {
 		t.Errorf("Resolve() returned non-nil layout in non-git dir: %v", layout)
 	}
 
-	if !errors.Is(err, paths.ErrNotAGitRepo) {
+	if !errors.Is(err, hubgeometry.ErrNotAGitRepo) {
 		t.Errorf("Resolve() error = %v; want wrapped ErrNotAGitRepo", err)
 	}
 }
@@ -200,7 +200,7 @@ func TestMirroredMethods(t *testing.T) {
 		t.Run("at root", func(t *testing.T) {
 			t.Parallel()
 
-			layout, err := paths.Resolve(hub)
+			layout, err := hubgeometry.Resolve(hub)
 			if err != nil {
 				t.Fatalf("Resolve() error = %v; want nil", err)
 			}
@@ -221,7 +221,7 @@ func TestMirroredMethods(t *testing.T) {
 				t.Fatalf("failed to create subdir: %v", err)
 			}
 
-			layout, err := paths.Resolve(subDir)
+			layout, err := hubgeometry.Resolve(subDir)
 			if err != nil {
 				t.Fatalf("Resolve() error = %v; want nil", err)
 			}
@@ -246,12 +246,12 @@ func TestMirroredMethods(t *testing.T) {
 				t.Fatalf("failed to create subdir2: %v", err)
 			}
 
-			layout1, err := paths.Resolve(subDir1)
+			layout1, err := hubgeometry.Resolve(subDir1)
 			if err != nil {
 				t.Fatalf("Resolve(subDir1) error = %v; want nil", err)
 			}
 
-			layout2, err := paths.Resolve(subDir2)
+			layout2, err := hubgeometry.Resolve(subDir2)
 			if err != nil {
 				t.Fatalf("Resolve(subDir2) error = %v; want nil", err)
 			}
@@ -272,7 +272,7 @@ func TestMirroredMethods(t *testing.T) {
 		t.Run("at root (backward compat)", func(t *testing.T) {
 			t.Parallel()
 
-			layout, err := paths.Resolve(hub)
+			layout, err := hubgeometry.Resolve(hub)
 			if err != nil {
 				t.Fatalf("Resolve() error = %v; want nil", err)
 			}
@@ -294,7 +294,7 @@ func TestMirroredMethods(t *testing.T) {
 				t.Fatalf("failed to create subdir: %v", err)
 			}
 
-			layout, err := paths.Resolve(subDir)
+			layout, err := hubgeometry.Resolve(subDir)
 			if err != nil {
 				t.Fatalf("Resolve() error = %v; want nil", err)
 			}
@@ -319,12 +319,12 @@ func TestMirroredMethods(t *testing.T) {
 				t.Fatalf("failed to create subdir2: %v", err)
 			}
 
-			layout1, err := paths.Resolve(subDir1)
+			layout1, err := hubgeometry.Resolve(subDir1)
 			if err != nil {
 				t.Fatalf("Resolve(subDir1) error = %v; want nil", err)
 			}
 
-			layout2, err := paths.Resolve(subDir2)
+			layout2, err := hubgeometry.Resolve(subDir2)
 			if err != nil {
 				t.Fatalf("Resolve(subDir2) error = %v; want nil", err)
 			}
@@ -345,7 +345,7 @@ func TestMirroredMethods(t *testing.T) {
 		t.Run("at root", func(t *testing.T) {
 			t.Parallel()
 
-			layout, err := paths.Resolve(hub)
+			layout, err := hubgeometry.Resolve(hub)
 			if err != nil {
 				t.Fatalf("Resolve() error = %v; want nil", err)
 			}
@@ -365,7 +365,7 @@ func TestMirroredMethods(t *testing.T) {
 				t.Fatalf("failed to create subdir: %v", err)
 			}
 
-			layout, err := paths.Resolve(subDir)
+			layout, err := hubgeometry.Resolve(subDir)
 			if err != nil {
 				t.Fatalf("Resolve() error = %v; want nil", err)
 			}
@@ -384,7 +384,7 @@ func TestMirroredMethods(t *testing.T) {
 		t.Run("at root", func(t *testing.T) {
 			t.Parallel()
 
-			layout, err := paths.Resolve(hub)
+			layout, err := hubgeometry.Resolve(hub)
 			if err != nil {
 				t.Fatalf("Resolve() error = %v; want nil", err)
 			}
@@ -410,7 +410,7 @@ func TestMirroredMethods(t *testing.T) {
 				t.Fatalf("failed to create subdir: %v", err)
 			}
 
-			layout, err := paths.Resolve(subDir)
+			layout, err := hubgeometry.Resolve(subDir)
 			if err != nil {
 				t.Fatalf("Resolve() error = %v; want nil", err)
 			}
@@ -435,7 +435,7 @@ func TestMirroredMethods(t *testing.T) {
 		t.Run("at root", func(t *testing.T) {
 			t.Parallel()
 
-			layout, err := paths.Resolve(hub)
+			layout, err := hubgeometry.Resolve(hub)
 			if err != nil {
 				t.Fatalf("Resolve() error = %v; want nil", err)
 			}
@@ -460,7 +460,7 @@ func TestMirroredMethods(t *testing.T) {
 				t.Fatalf("failed to create subdir: %v", err)
 			}
 
-			layout, err := paths.Resolve(subDir)
+			layout, err := hubgeometry.Resolve(subDir)
 			if err != nil {
 				t.Fatalf("Resolve() error = %v; want nil", err)
 			}
@@ -487,7 +487,7 @@ func TestRefactoredMethods(t *testing.T) {
 	fix := lyxtest.CopyHostHub(t)
 	hub := fix.Hub
 
-	layout, err := paths.Resolve(hub)
+	layout, err := hubgeometry.Resolve(hub)
 	if err != nil {
 		t.Fatalf("Resolve() error = %v; want nil", err)
 	}
diff --git a/internal/paths/paths_unit_test.go b/internal/hubgeometry/hubgeometry_unit_test.go
similarity index 65%
rename from internal/paths/paths_unit_test.go
rename to internal/hubgeometry/hubgeometry_unit_test.go
index fba2701..406fa82 100644
--- a/internal/paths/paths_unit_test.go
+++ b/internal/hubgeometry/hubgeometry_unit_test.go
@@ -1,13 +1,13 @@
-// paths_unit_test.go — pure path-math unit tests for config helpers and constants.
+// hubgeometry_unit_test.go — pure path-math unit tests for config helpers and constants.
 // These tests do not require a git repository and run under standard unit test verification.
 
-package paths_test
+package hubgeometry_test
 
 import (
 	"path/filepath"
 	"testing"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // TestConfigHelpers tests the free-function config path helpers.
@@ -18,8 +18,8 @@ func TestConfigHelpers(t *testing.T) {
 		t.Parallel()
 
 		baseDir := "/home/user/project"
-		got := paths.ConfigDir(baseDir)
-		want := filepath.Join(baseDir, paths.LyxDirName, "config")
+		got := hubgeometry.ConfigDir(baseDir)
+		want := filepath.Join(baseDir, hubgeometry.LyxDirName, "config")
 
 		if got != want {
 			t.Errorf("ConfigDir(%q) = %q; want %q", baseDir, got, want)
@@ -31,8 +31,8 @@ func TestConfigHelpers(t *testing.T) {
 
 		baseDir := "/home/user/project"
 		module := "myapp"
-		got := paths.ConfigFile(baseDir, module)
-		want := filepath.Join(baseDir, paths.LyxDirName, "config", "myapp.yaml")
+		got := hubgeometry.ConfigFile(baseDir, module)
+		want := filepath.Join(baseDir, hubgeometry.LyxDirName, "config", "myapp.yaml")
 
 		if got != want {
 			t.Errorf("ConfigFile(%q, %q) = %q; want %q", baseDir, module, got, want)
@@ -43,7 +43,7 @@ func TestConfigHelpers(t *testing.T) {
 		t.Parallel()
 
 		baseDir := "/home/user/project"
-		got := paths.DotEnv(baseDir)
+		got := hubgeometry.DotEnv(baseDir)
 		want := filepath.Join(baseDir, ".env")
 
 		if got != want {
@@ -56,7 +56,7 @@ func TestConfigHelpers(t *testing.T) {
 func TestLyxDirNameConstant(t *testing.T) {
 	t.Parallel()
 
-	if paths.LyxDirName != "_lyx" {
-		t.Errorf("LyxDirName = %q; want %q", paths.LyxDirName, "_lyx")
+	if hubgeometry.LyxDirName != "_lyx" {
+		t.Errorf("LyxDirName = %q; want %q", hubgeometry.LyxDirName, "_lyx")
 	}
 }
diff --git a/internal/paths/weft_test.go b/internal/hubgeometry/weft_test.go
similarity index 97%
rename from internal/paths/weft_test.go
rename to internal/hubgeometry/weft_test.go
index 04efc3d..5af7746 100644
--- a/internal/paths/weft_test.go
+++ b/internal/hubgeometry/weft_test.go
@@ -1,13 +1,13 @@
 // weft_test.go covers the weft geometry methods on Layout and verifies the
 // host↔weft junction pairing for the RelPath "." and subpath cases.
 
-package paths_test
+package hubgeometry_test
 
 import (
 	"path/filepath"
 	"testing"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // TestWeftGeometryMethods covers the eight weft geometry methods with both
@@ -78,7 +78,7 @@ func TestWeftGeometryMethods(t *testing.T) {
 
 	for _, tt := range tests {
 		t.Run(tt.name, func(t *testing.T) {
-			layout := &paths.Layout{
+			layout := &hubgeometry.Layout{
 				Cwd:          filepath.Join(tt.hub, "feat", tt.relPath),
 				WorktreeRoot: filepath.Join(tt.hub, "feat"),
 				Hub:          tt.hub,
@@ -158,7 +158,7 @@ func TestWeftGeometryMethods(t *testing.T) {
 func TestHostLyxLinkHereDivergesFromLyxDir(t *testing.T) {
 	// Equal case: Cwd == WorktreeRoot and RelPath == "." -> both resolve to the
 	// same _lyx directory.
-	atRoot := &paths.Layout{
+	atRoot := &hubgeometry.Layout{
 		Cwd:          filepath.Join("/h", "feat"),
 		WorktreeRoot: filepath.Join("/h", "feat"),
 		Hub:          "/h",
@@ -173,7 +173,7 @@ func TestHostLyxLinkHereDivergesFromLyxDir(t *testing.T) {
 	// Divergent case: Cwd points at the worktree root but RelPath is a real
 	// subdir, so LyxDir() (Cwd-anchored) and HostLyxLinkHere() (WorktreeRoot+
 	// RelPath-anchored) must differ.
-	atSub := &paths.Layout{
+	atSub := &hubgeometry.Layout{
 		Cwd:          filepath.Join("/h", "feat"),
 		WorktreeRoot: filepath.Join("/h", "feat"),
 		Hub:          "/h",
@@ -191,7 +191,7 @@ func TestHostLyxLinkHereDivergesFromLyxDir(t *testing.T) {
 func TestWeftGeometryAtMainWorktree(t *testing.T) {
 	hub := "/h"
 	main := "/h/main"
-	layout := &paths.Layout{
+	layout := &hubgeometry.Layout{
 		Cwd:          main,
 		WorktreeRoot: main,
 		Hub:          hub,
@@ -251,7 +251,7 @@ func TestHostJunctions(t *testing.T) {
 
 	for _, tt := range tests {
 		t.Run(tt.name, func(t *testing.T) {
-			layout := &paths.Layout{
+			layout := &hubgeometry.Layout{
 				Cwd:          filepath.Join(tt.hub, tt.slug, tt.relPath),
 				WorktreeRoot: filepath.Join(tt.hub, tt.slug),
 				Hub:          tt.hub,
@@ -291,7 +291,7 @@ func TestHostJunctions(t *testing.T) {
 
 	// Sub-test: scope guard — verify no junction name is _codeguide
 	t.Run("no_codeguide_names", func(t *testing.T) {
-		layout := &paths.Layout{
+		layout := &hubgeometry.Layout{
 			Cwd:          filepath.Join("/h", "main"),
 			WorktreeRoot: filepath.Join("/h", "main"),
 			Hub:          "/h",
diff --git a/internal/paths/worktreelist.go b/internal/hubgeometry/worktreelist.go
similarity index 99%
rename from internal/paths/worktreelist.go
rename to internal/hubgeometry/worktreelist.go
index b607961..8943f4a 100644
--- a/internal/paths/worktreelist.go
+++ b/internal/hubgeometry/worktreelist.go
@@ -1,7 +1,7 @@
 // worktreelist.go parses `git worktree list --porcelain` into structured
 // entries; it is the single porcelain parser shared across the codebase.
 
-package paths
+package hubgeometry
 
 import (
 	"fmt"
diff --git a/internal/paths/worktreelist_test.go b/internal/hubgeometry/worktreelist_test.go
similarity index 85%
rename from internal/paths/worktreelist_test.go
rename to internal/hubgeometry/worktreelist_test.go
index 7db2911..a0ba637 100644
--- a/internal/paths/worktreelist_test.go
+++ b/internal/hubgeometry/worktreelist_test.go
@@ -3,7 +3,7 @@
 // worktreelist_test.go covers the porcelain worktree-list parser, including
 // the bare-repo rejection and Main-on-first-entry behavior.
 
-package paths_test
+package hubgeometry_test
 
 import (
 	"fmt"
@@ -11,8 +11,8 @@ import (
 	"strings"
 	"testing"
 
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 )
 
 // TestList covers the porcelain parser: a fresh repo yields exactly the main
@@ -25,12 +25,12 @@ func TestList(t *testing.T) {
 		// extraWorktrees is the number of additional worktrees created
 		// alongside the main checkout before listing.
 		extraWorktrees int
-		verify         func(t *testing.T, hub string, entries []paths.WorktreeEntry)
+		verify         func(t *testing.T, hub string, entries []hubgeometry.WorktreeEntry)
 	}{
 		{
 			name:           "SingleWorktree",
 			extraWorktrees: 0,
-			verify: func(t *testing.T, hub string, entries []paths.WorktreeEntry) {
+			verify: func(t *testing.T, hub string, entries []hubgeometry.WorktreeEntry) {
 				if len(entries) != 1 {
 					t.Fatalf("List() len = %d; want 1", len(entries))
 				}
@@ -49,7 +49,7 @@ func TestList(t *testing.T) {
 		{
 			name:           "TwoWorktrees",
 			extraWorktrees: 1,
-			verify: func(t *testing.T, hub string, entries []paths.WorktreeEntry) {
+			verify: func(t *testing.T, hub string, entries []hubgeometry.WorktreeEntry) {
 				if len(entries) != 2 {
 					t.Fatalf("List() len = %d; want 2", len(entries))
 				}
@@ -70,7 +70,7 @@ func TestList(t *testing.T) {
 		{
 			name:           "BareRepoRejection",
 			extraWorktrees: 0,
-			verify: func(t *testing.T, hub string, entries []paths.WorktreeEntry) {
+			verify: func(t *testing.T, hub string, entries []hubgeometry.WorktreeEntry) {
 				// This test is not meant to be called; it's handled in the
 				// outer loop with a special case.
 			},
@@ -86,7 +86,7 @@ func TestList(t *testing.T) {
 				bareRepo := filepath.Join(t.TempDir(), "bare.git")
 				lyxtest.MustRun(t, t.TempDir(), "git", "init", "--bare", bareRepo)
 
-				entries, err := paths.List(bareRepo)
+				entries, err := hubgeometry.List(bareRepo)
 				if err == nil {
 					t.Fatalf("List() error = nil; want error containing 'bare'")
 				}
@@ -107,7 +107,7 @@ func TestList(t *testing.T) {
 				lyxtest.MustRun(t, hub, "git", "worktree", "add", wtPath)
 			}
 
-			entries, err := paths.List(hub)
+			entries, err := hubgeometry.List(hub)
 			if err != nil {
 				t.Fatalf("List() error = %v; want nil", err)
 			}
diff --git a/internal/idecli/cli.go b/internal/idecli/cli.go
index 707ec12..e92a7d7 100644
--- a/internal/idecli/cli.go
+++ b/internal/idecli/cli.go
@@ -12,9 +12,9 @@ import (
 	"os"
 
 	"github.com/Knatte18/loomyard/internal/clihelp"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/ideengine"
 	"github.com/Knatte18/loomyard/internal/output"
-	"github.com/Knatte18/loomyard/internal/paths"
 	"github.com/spf13/cobra"
 )
 
@@ -27,7 +27,7 @@ import (
 // cobra lists available subcommands without invoking the PreRunE (no git repo needed).
 func Command() *cobra.Command {
 	// l is populated by PersistentPreRunE and closed over by each subcommand RunE.
-	var l *paths.Layout
+	var l *hubgeometry.Layout
 
 	cmd := &cobra.Command{
 		Use:   "ide",
@@ -46,7 +46,7 @@ func Command() *cobra.Command {
 			ctx := cmd.Context()
 
 			// Resolve current working directory; fail fast if the lookup errors.
-			cwd, err := paths.Getwd()
+			cwd, err := hubgeometry.Getwd()
 			if err != nil {
 				output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to get working directory: %v", err))
 				clihelp.Abort(ctx, 1)
@@ -54,7 +54,7 @@ func Command() *cobra.Command {
 			}
 
 			// Resolve layout from cwd; requires being inside a git repository.
-			resolved, err := paths.Resolve(cwd)
+			resolved, err := hubgeometry.Resolve(cwd)
 			if err != nil {
 				output.Err(cmd.OutOrStdout(), fmt.Sprintf("failed to resolve layout: %v", err))
 				clihelp.Abort(ctx, 1)
diff --git a/internal/idecli/cli_test.go b/internal/idecli/cli_test.go
index d3da9c7..bce14cc 100644
--- a/internal/idecli/cli_test.go
+++ b/internal/idecli/cli_test.go
@@ -17,7 +17,7 @@ import (
 
 // TestRunCLISpawnDispatch tests that spawn subcommand dispatches correctly with stubbed launcher.
 func TestRunCLISpawnDispatch(t *testing.T) {
-	// Create a real git repo so paths.Resolve succeeds inside the PersistentPreRunE.
+	// Create a real git repo so hubgeometry.Resolve succeeds inside the PersistentPreRunE.
 	gitRepo := lyxtest.CopyHostHub(t).Hub
 
 	t.Chdir(gitRepo)
diff --git a/internal/ideengine/menu.go b/internal/ideengine/menu.go
index 1ce5662..6248cd9 100644
--- a/internal/ideengine/menu.go
+++ b/internal/ideengine/menu.go
@@ -14,12 +14,12 @@ import (
 	"strings"
 
 	"github.com/Knatte18/loomyard/internal/boardengine"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // Menu presents an interactive picker of active worktrees, allowing the user to open one via Spawn.
 //
-// It discovers active worktrees from paths.List(l.Cwd), excluding the main worktree (Main==true)
+// It discovers active worktrees from hubgeometry.List(l.Cwd), excluding the main worktree (Main==true)
 // and only including those whose <path>/<l.RelPath>/_lyx directory exists.
 //
 // Titles are resolved ONLY through the board facade (b.GetTask(slug) → Task.Title).
@@ -35,7 +35,7 @@ import (
 //   - out: output writer (for printing the picker menu)
 //
 // Returns an error on failure (HARD error if config load or HealthCheck fails), or nil on success.
-func Menu(l *paths.Layout, in io.Reader, out io.Writer) error {
+func Menu(l *hubgeometry.Layout, in io.Reader, out io.Writer) error {
 	// Load board config and create board facade
 	cfg, err := boardengine.LoadConfig(l.Cwd, "board")
 	if err != nil {
@@ -49,8 +49,8 @@ func Menu(l *paths.Layout, in io.Reader, out io.Writer) error {
 		return fmt.Errorf("board health check failed: %w", err)
 	}
 
-	// Discover active worktrees via paths.List
-	entries, err := paths.List(l.Cwd)
+	// Discover active worktrees via hubgeometry.List
+	entries, err := hubgeometry.List(l.Cwd)
 	if err != nil {
 		return fmt.Errorf("list worktrees: %w", err)
 	}
@@ -69,7 +69,7 @@ func Menu(l *paths.Layout, in io.Reader, out io.Writer) error {
 		slug := filepath.Base(entry.Path)
 
 		// Check if _lyx exists at <path>/<l.RelPath>/_lyx
-		lyxPath := filepath.Join(entry.Path, l.RelPath, paths.LyxDirName)
+		lyxPath := filepath.Join(entry.Path, l.RelPath, hubgeometry.LyxDirName)
 		stat, err := os.Stat(lyxPath)
 		if err != nil || !stat.IsDir() {
 			// _lyx doesn't exist or is not a directory; skip
diff --git a/internal/ideengine/menu_test.go b/internal/ideengine/menu_test.go
index 1cba4fc..6a78552 100644
--- a/internal/ideengine/menu_test.go
+++ b/internal/ideengine/menu_test.go
@@ -14,7 +14,7 @@ import (
 	"strings"
 	"testing"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // mustRunMenu is a test helper that runs a command in a directory.
@@ -55,7 +55,7 @@ func newTestGitRepoWithWorktrees(t *testing.T) (string, string) {
 	mustRunMenu(t, mainWorktreePath, "git", "commit", "-m", "initial")
 
 	// Create main's _lyx directory
-	if err := os.MkdirAll(filepath.Join(mainWorktreePath, paths.LyxDirName), 0o755); err != nil {
+	if err := os.MkdirAll(filepath.Join(mainWorktreePath, hubgeometry.LyxDirName), 0o755); err != nil {
 		t.Fatalf("failed to create main _lyx: %v", err)
 	}
 
@@ -68,7 +68,7 @@ func TestMenuHardErrorOnMissingBoard(t *testing.T) {
 
 	container, mainWorktreePath := newTestGitRepoWithWorktrees(t)
 
-	layout := &paths.Layout{
+	layout := &hubgeometry.Layout{
 		Hub:     container,
 		Prime:   mainWorktreePath,
 		RelPath: ".",
@@ -106,16 +106,16 @@ func TestMenuExcludesMain(t *testing.T) {
 	}()
 
 	// Create _lyx in child
-	if err := os.MkdirAll(filepath.Join(childPath, paths.LyxDirName), 0o755); err != nil {
+	if err := os.MkdirAll(filepath.Join(childPath, hubgeometry.LyxDirName), 0o755); err != nil {
 		t.Fatalf("failed to create child _lyx: %v", err)
 	}
 
 	// Create board config
-	configDir := paths.ConfigDir(mainWorktreePath)
+	configDir := hubgeometry.ConfigDir(mainWorktreePath)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("failed to create config dir: %v", err)
 	}
-	boardConfigPath := paths.ConfigFile(mainWorktreePath, "board")
+	boardConfigPath := hubgeometry.ConfigFile(mainWorktreePath, "board")
 	boardConfig := `path: ../_board
 home: Home.md
 sidebar: _Sidebar.md
@@ -136,7 +136,7 @@ proposal_prefix: proposal-
 		t.Fatalf("failed to write tasks.json: %v", err)
 	}
 
-	layout := &paths.Layout{
+	layout := &hubgeometry.Layout{
 		Hub:     container,
 		Prime:   mainWorktreePath,
 		RelPath: ".",
@@ -179,11 +179,11 @@ func TestMenuRequiresLyxDir(t *testing.T) {
 	// Note: child is created but has no _lyx
 
 	// Create board config
-	configDir := paths.ConfigDir(mainWorktreePath)
+	configDir := hubgeometry.ConfigDir(mainWorktreePath)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("failed to create config dir: %v", err)
 	}
-	boardConfigPath := paths.ConfigFile(mainWorktreePath, "board")
+	boardConfigPath := hubgeometry.ConfigFile(mainWorktreePath, "board")
 	boardConfig := `path: ../_board
 home: Home.md
 sidebar: _Sidebar.md
@@ -204,7 +204,7 @@ proposal_prefix: proposal-
 		t.Fatalf("failed to write tasks.json: %v", err)
 	}
 
-	layout := &paths.Layout{
+	layout := &hubgeometry.Layout{
 		Hub:     container,
 		Prime:   mainWorktreePath,
 		RelPath: ".",
@@ -235,7 +235,7 @@ func TestMenuNumericSelection(t *testing.T) {
 	for _, child := range []string{"child1", "child2"} {
 		childPath := filepath.Join(container, child)
 		mustRunMenu(t, mainWorktreePath, "git", "worktree", "add", "-b", child+"-branch", childPath)
-		if err := os.MkdirAll(filepath.Join(childPath, paths.LyxDirName), 0o755); err != nil {
+		if err := os.MkdirAll(filepath.Join(childPath, hubgeometry.LyxDirName), 0o755); err != nil {
 			t.Fatalf("failed to create %s _lyx: %v", child, err)
 		}
 	}
@@ -249,11 +249,11 @@ func TestMenuNumericSelection(t *testing.T) {
 	}()
 
 	// Create board config
-	configDir := paths.ConfigDir(mainWorktreePath)
+	configDir := hubgeometry.ConfigDir(mainWorktreePath)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		t.Fatalf("failed to create config dir: %v", err)
 	}
-	boardConfigPath := paths.ConfigFile(mainWorktreePath, "board")
+	boardConfigPath := hubgeometry.ConfigFile(mainWorktreePath, "board")
 	boardConfig := `path: ../_board
 home: Home.md
 sidebar: _Sidebar.md
@@ -275,7 +275,7 @@ proposal_prefix: proposal-
 		t.Fatalf("failed to write tasks.json: %v", err)
 	}
 
-	layout := &paths.Layout{
+	layout := &hubgeometry.Layout{
 		Hub:     container,
 		Prime:   mainWorktreePath,
 		RelPath: ".",
diff --git a/internal/ideengine/spawn.go b/internal/ideengine/spawn.go
index 8ea2d83..a7f4f95 100644
--- a/internal/ideengine/spawn.go
+++ b/internal/ideengine/spawn.go
@@ -6,7 +6,7 @@ package ideengine
 import (
 	"path/filepath"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/vscode"
 )
 
@@ -24,7 +24,7 @@ var CodeLauncher = vscode.Launch
 //  4. Open the worktree at its relpath (dir holding _lyx/ and .vscode/) via CodeLauncher
 //
 // Returns an error if any step fails.
-func Spawn(l *paths.Layout, slug string) error {
+func Spawn(l *hubgeometry.Layout, slug string) error {
 	// Compute worktreeDir from slug
 	worktreeDir := l.WorktreePath(slug)
 
diff --git a/internal/ideengine/spawn_test.go b/internal/ideengine/spawn_test.go
index 8895598..6821699 100644
--- a/internal/ideengine/spawn_test.go
+++ b/internal/ideengine/spawn_test.go
@@ -7,7 +7,7 @@ import (
 	"path/filepath"
 	"testing"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // TestSpawn covers end-to-end spawn flow: config generation, launcher invocation,
@@ -53,7 +53,7 @@ func TestSpawn(t *testing.T) {
 				}
 			}
 
-			layout := &paths.Layout{
+			layout := &hubgeometry.Layout{
 				Hub:     container,
 				Prime:   mainWorktreePath,
 				RelPath: tt.relpath,
diff --git a/internal/initcli/initcli.go b/internal/initcli/initcli.go
index 915f9c2..a6fa474 100644
--- a/internal/initcli/initcli.go
+++ b/internal/initcli/initcli.go
@@ -20,8 +20,8 @@ import (
 	"github.com/Knatte18/loomyard/internal/clihelp"
 	"github.com/Knatte18/loomyard/internal/configsync"
 	"github.com/Knatte18/loomyard/internal/gitignore"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/output"
-	"github.com/Knatte18/loomyard/internal/paths"
 	"github.com/Knatte18/loomyard/internal/warpengine"
 )
 
@@ -73,13 +73,13 @@ func RunInit(out io.Writer, args []string) int {
 // Returns exit code 0 on success, 1 on error.
 func runInit(out io.Writer, args []string) int {
 	// Resolve current working directory
-	cwd, err := paths.Getwd()
+	cwd, err := hubgeometry.Getwd()
 	if err != nil {
 		return output.Err(out, fmt.Sprintf("failed to get working directory: %v", err))
 	}
 
 	// Resolve layout from cwd (needed for weft sibling derivation and slug)
-	l, err := paths.Resolve(cwd)
+	l, err := hubgeometry.Resolve(cwd)
 	if err != nil {
 		return output.Err(out, fmt.Sprintf("failed to resolve layout: %v", err))
 	}
@@ -101,7 +101,7 @@ func runInit(out io.Writer, args []string) int {
 	status := map[string]string{}
 
 	// Step 4: Create _lyx directory (activation completed in steps 1-3 above)
-	lyxDir := filepath.Join(cwd, paths.LyxDirName)
+	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
 	info, err := os.Stat(lyxDir)
 	if err != nil && !os.IsNotExist(err) {
 		return output.Err(out, fmt.Sprintf("failed to stat _lyx: %v", err))
@@ -122,7 +122,7 @@ func runInit(out io.Writer, args []string) int {
 	}
 
 	// Create _lyx/config/ subdirectory to hold configuration files
-	configDir := paths.ConfigDir(cwd)
+	configDir := hubgeometry.ConfigDir(cwd)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		return output.Err(out, fmt.Sprintf("failed to create _lyx/config directory: %v", err))
 	}
diff --git a/internal/initcli/initcli_test.go b/internal/initcli/initcli_test.go
index 4719071..0c4bd96 100644
--- a/internal/initcli/initcli_test.go
+++ b/internal/initcli/initcli_test.go
@@ -18,9 +18,9 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/boardengine"
 	"github.com/Knatte18/loomyard/internal/gitexec"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/initcli"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 	"github.com/Knatte18/loomyard/internal/warpengine"
 	"github.com/Knatte18/loomyard/internal/weftengine"
 )
@@ -51,14 +51,14 @@ func TestRunInit_FirstRun(t *testing.T) {
 	}
 
 	// Verify _lyx/config/ directories exist
-	configDir := paths.ConfigDir(f.Layout.WorktreeRoot)
+	configDir := hubgeometry.ConfigDir(f.Layout.WorktreeRoot)
 	if _, err := os.Stat(configDir); err != nil {
 		t.Fatalf("_lyx/config not created: %v", err)
 	}
 
 	// Verify all three config files exist
 	for _, module := range []string{"board", "warp", "weft"} {
-		cfgPath := paths.ConfigFile(f.Layout.WorktreeRoot, module)
+		cfgPath := hubgeometry.ConfigFile(f.Layout.WorktreeRoot, module)
 		if _, err := os.Stat(cfgPath); err != nil {
 			t.Errorf("%s.yaml not created: %v", module, err)
 		}
@@ -112,7 +112,7 @@ func TestRunInit_Idempotent(t *testing.T) {
 	}
 
 	// Capture files and gitignore after first run
-	boardPath := paths.ConfigFile(f.Layout.WorktreeRoot, "board")
+	boardPath := hubgeometry.ConfigFile(f.Layout.WorktreeRoot, "board")
 	content1, err := os.ReadFile(boardPath)
 	if err != nil {
 		t.Fatalf("read board.yaml: %v", err)
@@ -206,7 +206,7 @@ func TestRunInit_NoPairing(t *testing.T) {
 		t.Error(".gitignore should not exist when no pairing")
 	}
 
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if _, err := os.Stat(configDir); err == nil {
 		t.Error("_lyx/config should not exist when no pairing")
 	}
diff --git a/internal/lyxtest/doc.go b/internal/lyxtest/doc.go
index 5c6b248..0992544 100644
--- a/internal/lyxtest/doc.go
+++ b/internal/lyxtest/doc.go
@@ -1,12 +1,12 @@
 // Package lyxtest holds the shared git-fixture support machinery for Loomyard's
 // test suites across internal/warpengine, internal/warpcli, internal/weftengine,
-// internal/weftcli, and internal/paths.
+// internal/weftcli, and internal/hubgeometry.
 // It owns the fixture builders and per-test isolation helpers, following the
 // template-built-once + per-test filesystem copy pattern to minimize setup overhead
 // and maximize parallelism. See MustRun, CopyHostHub, CopyPaired, and CopyWeft.
 //
 // Leaf Invariant: internal/lyxtest must remain a leaf package importing only the
-// standard library and internal/paths. It must not import internal/configreg or any
+// standard library and internal/hubgeometry. It must not import internal/configreg or any
 // feature package (boardengine/boardcli, warpengine/warpcli, weftengine/weftcli,
 // ideengine/idecli, selfreportengine/selfreportcli, muxpoccli). Feature packages'
 // internal tests import lyxtest; a configreg or feature import would close a
diff --git a/internal/lyxtest/leaf_enforcement_test.go b/internal/lyxtest/leaf_enforcement_test.go
index 1dc0d58..a106dc2 100644
--- a/internal/lyxtest/leaf_enforcement_test.go
+++ b/internal/lyxtest/leaf_enforcement_test.go
@@ -16,7 +16,7 @@ import (
 	"testing"
 )
 
-// TestLeafInvariant verifies that lyxtest imports only stdlib and internal/paths,
+// TestLeafInvariant verifies that lyxtest imports only stdlib and internal/hubgeometry,
 // never internal/configreg or any feature package (boardengine/boardcli, warpengine/warpcli,
 // weftengine/weftcli, ideengine/idecli, selfreportengine/selfreportcli, muxpoccli).
 // It uses go/parser to read actual import paths, avoiding false positives from
diff --git a/internal/lyxtest/lyxtest.go b/internal/lyxtest/lyxtest.go
index bbb9b84..cd98df6 100644
--- a/internal/lyxtest/lyxtest.go
+++ b/internal/lyxtest/lyxtest.go
@@ -13,7 +13,7 @@ import (
 	"sync"
 	"testing"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // MustRun runs a command with the given arguments in the specified directory.
@@ -37,19 +37,19 @@ func MustRun(tb testing.TB, dir string, args ...string) {
 // SeedConfig creates the _lyx/config directory if needed, writes each module's
 // YAML file, stages all changes, and commits them so the files are checked out
 // in the worktree. This preserves the leaf invariant: lyxtest imports only stdlib
-// and internal/paths, never configreg or feature packages.
+// and internal/hubgeometry, never configreg or feature packages.
 func SeedConfig(tb testing.TB, repoDir string, configByModule map[string]string) {
 	tb.Helper()
 
 	// Create config directory if it doesn't exist.
-	configDir := paths.ConfigDir(repoDir)
+	configDir := hubgeometry.ConfigDir(repoDir)
 	if err := os.MkdirAll(configDir, 0o755); err != nil {
 		tb.Fatalf("mkdir config dir: %v", err)
 	}
 
 	// Write each module's config file.
 	for module, content := range configByModule {
-		configPath := paths.ConfigFile(repoDir, module)
+		configPath := hubgeometry.ConfigFile(repoDir, module)
 		if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
 			tb.Fatalf("write config file %s: %v", module, err)
 		}
@@ -182,7 +182,7 @@ func buildWeftPrime() (weftPrime, weftBare string) {
 			panic(err)
 		}
 
-		weftPrime := paths.WeftSiblingPath(tmpDir, base)
+		weftPrime := hubgeometry.WeftSiblingPath(tmpDir, base)
 		if err := os.Mkdir(weftPrime, 0o755); err != nil {
 			panic(err)
 		}
@@ -191,7 +191,7 @@ func buildWeftPrime() (weftPrime, weftBare string) {
 
 		// Create _lyx/config with neutral placeholder (no real config files).
 		// Tests needing real config seed it via SeedConfig.
-		lyxConfigDir := paths.ConfigDir(weftPrime)
+		lyxConfigDir := hubgeometry.ConfigDir(weftPrime)
 		if err := os.MkdirAll(lyxConfigDir, 0o755); err != nil {
 			panic(err)
 		}
@@ -240,7 +240,7 @@ func buildWeftOnly() (weftPath, bare string) {
 		// TestPushIntegration can commit the "_lyx" pathspec. This fixture only
 		// needs some tracked file under _lyx, not a real config layout; tests that
 		// need real config call SeedConfig after CopyWeft.
-		lyxDir := filepath.Join(weftPath, paths.LyxDirName)
+		lyxDir := filepath.Join(weftPath, hubgeometry.LyxDirName)
 		if err := os.MkdirAll(lyxDir, 0o755); err != nil {
 			panic(err)
 		}
@@ -279,7 +279,7 @@ type PairedFixture struct {
 	Bare      string
 	WeftPrime string
 	WeftBare  string
-	Layout    *paths.Layout
+	Layout    *hubgeometry.Layout
 }
 
 // WeftFixture represents an isolated copy of the weft-only template
@@ -472,7 +472,7 @@ func CopyPaired(tb testing.TB) PairedFixture {
 
 	// Copy weft-prime (must preserve the -weft suffix)
 	base := filepath.Base(templateHub)
-	copiedWeftPrime := paths.WeftSiblingPath(tempContainer, base)
+	copiedWeftPrime := hubgeometry.WeftSiblingPath(tempContainer, base)
 	if err := copyDirRecursive(templateWeftPrime, copiedWeftPrime); err != nil {
 		tb.Fatalf("copyDirRecursive weftPrime: %v", err)
 	}
@@ -493,9 +493,9 @@ func CopyPaired(tb testing.TB) PairedFixture {
 	}
 
 	// Get layout from copied hub
-	layout, err := paths.Resolve(copiedHub)
+	layout, err := hubgeometry.Resolve(copiedHub)
 	if err != nil {
-		tb.Fatalf("paths.Resolve: %v", err)
+		tb.Fatalf("hubgeometry.Resolve: %v", err)
 	}
 
 	return PairedFixture{
@@ -538,7 +538,7 @@ func CopyPairedLocal(tb testing.TB) PairedFixture {
 
 	// Copy weft-prime (must preserve the -weft suffix); omit weft-bare
 	base := filepath.Base(templateHub)
-	copiedWeftPrime := paths.WeftSiblingPath(tempContainer, base)
+	copiedWeftPrime := hubgeometry.WeftSiblingPath(tempContainer, base)
 	if err := copyDirRecursive(templateWeftPrime, copiedWeftPrime); err != nil {
 		tb.Fatalf("copyDirRecursive weftPrime: %v", err)
 	}
@@ -550,9 +550,9 @@ func CopyPairedLocal(tb testing.TB) PairedFixture {
 	}
 
 	// Get layout from copied hub
-	layout, err := paths.Resolve(copiedHub)
+	layout, err := hubgeometry.Resolve(copiedHub)
 	if err != nil {
-		tb.Fatalf("paths.Resolve: %v", err)
+		tb.Fatalf("hubgeometry.Resolve: %v", err)
 	}
 
 	return PairedFixture{
diff --git a/internal/lyxtest/lyxtest_test.go b/internal/lyxtest/lyxtest_test.go
index a2c2ea6..bae460e 100644
--- a/internal/lyxtest/lyxtest_test.go
+++ b/internal/lyxtest/lyxtest_test.go
@@ -9,7 +9,7 @@ import (
 	"strings"
 	"testing"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // TestCopyHostHub verifies that CopyHostHub returns valid independent git repos.
@@ -263,7 +263,7 @@ func TestSeedConfig(t *testing.T) {
 	})
 
 	// Verify files exist with correct content
-	module1Path := paths.ConfigFile(tmpDir, "module1")
+	module1Path := hubgeometry.ConfigFile(tmpDir, "module1")
 	content1, err := os.ReadFile(module1Path)
 	if err != nil {
 		t.Fatalf("read module1.yaml: %v", err)
@@ -272,7 +272,7 @@ func TestSeedConfig(t *testing.T) {
 		t.Errorf("module1 content = %q; want %q", string(content1), configContent)
 	}
 
-	module2Path := paths.ConfigFile(tmpDir, "module2")
+	module2Path := hubgeometry.ConfigFile(tmpDir, "module2")
 	content2, err := os.ReadFile(module2Path)
 	if err != nil {
 		t.Fatalf("read module2.yaml: %v", err)
@@ -316,7 +316,7 @@ func TestCopyPaired_NeutralFixture(t *testing.T) {
 	fixture := CopyPaired(t)
 
 	// Verify the weft-prime contains _lyx/config/placeholder
-	placeholderPath := filepath.Join(paths.ConfigDir(fixture.WeftPrime), "placeholder")
+	placeholderPath := filepath.Join(hubgeometry.ConfigDir(fixture.WeftPrime), "placeholder")
 	placeholderContent, err := os.ReadFile(placeholderPath)
 	if err != nil {
 		t.Fatalf("read placeholder: %v", err)
@@ -326,7 +326,7 @@ func TestCopyPaired_NeutralFixture(t *testing.T) {
 	}
 
 	// Verify the weft-prime does NOT contain real config files (e.g., weft.yaml)
-	weftConfigPath := paths.ConfigFile(fixture.WeftPrime, "weft")
+	weftConfigPath := hubgeometry.ConfigFile(fixture.WeftPrime, "weft")
 	if _, err := os.Stat(weftConfigPath); !os.IsNotExist(err) {
 		if err == nil {
 			t.Errorf("weft.yaml should not exist in neutral fixture, but it does")
diff --git a/internal/muxpoccli/cli.go b/internal/muxpoccli/cli.go
index 9c89827..f12c3eb 100644
--- a/internal/muxpoccli/cli.go
+++ b/internal/muxpoccli/cli.go
@@ -2,7 +2,7 @@
 // the RunCLI seam that wires it into the legacy io.Writer-based call contract.
 //
 // Command() builds a parent "muxpoc" cobra command with persistent tuning flags
-// and a PersistentPreRunE that resolves the worktree root via paths.Resolve.
+// and a PersistentPreRunE that resolves the worktree root via hubgeometry.Resolve.
 // Each subcommand's RunE closes over the resolved cfg variable that PreRunE populates.
 
 // Package muxpoc is a shipped proof-of-concept psmux orchestrator that proves
@@ -24,8 +24,8 @@ import (
 	"time"
 
 	"github.com/Knatte18/loomyard/internal/clihelp"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/output"
-	"github.com/Knatte18/loomyard/internal/paths"
 	"github.com/spf13/cobra"
 )
 
@@ -46,7 +46,7 @@ type Config struct {
 //
 // The parent command carries persistent tuning flags (--psmux, --pwsh, --claude,
 // --launch, --resume, --width, --height, --interval) so every subcommand inherits
-// them. A PersistentPreRunE resolves the worktree root via paths.Resolve and
+// them. A PersistentPreRunE resolves the worktree root via hubgeometry.Resolve and
 // populates the closure-local cfg variable; on failure it writes an error response
 // and signals abort so that subcommand RunE bodies do not execute against an
 // uninitialised environment. Running "lyx muxpoc" with no arguments lists
@@ -88,14 +88,14 @@ the risky parts — daemon and pane recovery — of the planned mux module.`,
 			return nil
 		}
 
-		cwd, err := paths.Getwd()
+		cwd, err := hubgeometry.Getwd()
 		if err != nil {
 			output.Err(c.OutOrStdout(), fmt.Sprintf("failed to get current working directory: %v", err))
 			clihelp.Abort(c.Context(), 1)
 			return nil
 		}
 
-		layout, err := paths.Resolve(cwd)
+		layout, err := hubgeometry.Resolve(cwd)
 		if err != nil {
 			output.Err(c.OutOrStdout(), fmt.Sprintf("not a git repository: %v", err))
 			clihelp.Abort(c.Context(), 1)
diff --git a/internal/muxpoccli/muxpoc_smoke_test.go b/internal/muxpoccli/muxpoc_smoke_test.go
index bca02f2..ba184cf 100644
--- a/internal/muxpoccli/muxpoc_smoke_test.go
+++ b/internal/muxpoccli/muxpoc_smoke_test.go
@@ -52,7 +52,7 @@ func TestSmokeFullLifecycle(t *testing.T) {
 	}
 	t.Cleanup(func() { _ = os.Chdir(origWd) })
 
-	// Initialize temp dir as a git repo so paths.Resolve succeeds
+	// Initialize temp dir as a git repo so hubgeometry.Resolve succeeds
 	initCmd := exec.Command("git", "init", "-b", "main")
 	initCmd.Dir = cwd
 	if err := initCmd.Run(); err != nil {
diff --git a/internal/vscode/color.go b/internal/vscode/color.go
index 55458ba..d4dd912 100644
--- a/internal/vscode/color.go
+++ b/internal/vscode/color.go
@@ -11,7 +11,7 @@ import (
 	"path/filepath"
 	"strings"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // ErrUnsupported is returned when vscode launch is attempted on an unsupported platform.
@@ -42,7 +42,7 @@ var mainColor = "#2d7d46"
 //   - Return the first palette color that is not mainColor and not in use
 //   - If all non-green colors are used, return the first non-green (palette[1])
 //   - If hub/dirs missing, return first non-green
-func PickColor(l *paths.Layout) string {
+func PickColor(l *hubgeometry.Layout) string {
 	used := make(map[string]bool)
 
 	// Try to read the hub directory
diff --git a/internal/vscode/color_test.go b/internal/vscode/color_test.go
index e3388df..77e11e2 100644
--- a/internal/vscode/color_test.go
+++ b/internal/vscode/color_test.go
@@ -9,7 +9,7 @@ import (
 	"path/filepath"
 	"testing"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // TestPickColor covers the palette picker, ensuring it selects unused non-green colors
@@ -113,7 +113,7 @@ func TestPickColor(t *testing.T) {
 
 			tt.setupFunc(tmpDir, mainPath)
 
-			layout := &paths.Layout{
+			layout := &hubgeometry.Layout{
 				Hub:     tmpDir,
 				Prime:   mainPath,
 				RelPath: tt.RelPath,
diff --git a/internal/warpcli/clone.go b/internal/warpcli/clone.go
index c8e5467..0021985 100644
--- a/internal/warpcli/clone.go
+++ b/internal/warpcli/clone.go
@@ -8,8 +8,8 @@ import (
 	"fmt"
 	"io"
 
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/output"
-	"github.com/Knatte18/loomyard/internal/paths"
 	"github.com/Knatte18/loomyard/internal/warpengine"
 )
 
@@ -25,7 +25,7 @@ func runClone(out io.Writer, args []string) int {
 // cloning, making the operation idempotent. The teardown uses warpengine.RemoveAll
 // so tests can inject errors by swapping that exported var.
 func runCloneWithReset(out io.Writer, args []string, reset bool) int {
-	cwd, err := paths.Getwd()
+	cwd, err := hubgeometry.Getwd()
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
@@ -47,7 +47,7 @@ func runCloneWithReset(out io.Writer, args []string, reset bool) int {
 		if name == "" {
 			return output.Err(out, fmt.Sprintf("could not derive repo name from host URL %s", hostURL))
 		}
-		hubPath := paths.HubPath(cwd, name)
+		hubPath := hubgeometry.HubPath(cwd, name)
 		if err := warpengine.RemoveAll(hubPath); err != nil {
 			return output.Err(out, fmt.Sprintf("reset: remove hub at %s: %v", hubPath, err))
 		}
diff --git a/internal/warpcli/clone_cli_test.go b/internal/warpcli/clone_cli_test.go
index cbde4d2..c136584 100644
--- a/internal/warpcli/clone_cli_test.go
+++ b/internal/warpcli/clone_cli_test.go
@@ -76,10 +76,10 @@ func makeBareRemote(t *testing.T, dir, name string) string {
 // It swaps warpengine.RemoveAll cross-package to inject a teardown error, then calls
 // runCloneWithReset with a non-existent board URL so the board clone triggers teardown.
 // The test stays serial (no t.Parallel) because it changes the global RemoveAll seam and
-// relies on t.Chdir for the cwd that runCloneWithReset reads via paths.Getwd().
+// relies on t.Chdir for the cwd that runCloneWithReset reads via hubgeometry.Getwd().
 func TestCloneHub_TeardownFailure(t *testing.T) {
 	cwd := t.TempDir()
-	t.Chdir(cwd) // runCloneWithReset reads paths.Getwd() so cwd must be set.
+	t.Chdir(cwd) // runCloneWithReset reads hubgeometry.Getwd() so cwd must be set.
 
 	// Swap RemoveAll to inject a teardown failure; restore after test.
 	orig := warpengine.RemoveAll
diff --git a/internal/warpcli/warp.go b/internal/warpcli/warp.go
index ae365f7..7db5cdd 100644
--- a/internal/warpcli/warp.go
+++ b/internal/warpcli/warp.go
@@ -54,8 +54,8 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/clihelp"
 	"github.com/Knatte18/loomyard/internal/gitexec"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/output"
-	"github.com/Knatte18/loomyard/internal/paths"
 	"github.com/Knatte18/loomyard/internal/warpengine"
 	"github.com/spf13/cobra"
 )
@@ -218,12 +218,12 @@ func RunCLI(out io.Writer, args []string) int {
 // runAdd parses and executes the warp add subcommand.
 // Under cobra, args[0] is the slug (cobra has already stripped the "add" token).
 func runAdd(out io.Writer, args []string) int {
-	cwd, err := paths.Getwd()
+	cwd, err := hubgeometry.Getwd()
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
 
-	l, err := paths.Resolve(cwd)
+	l, err := hubgeometry.Resolve(cwd)
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
@@ -255,12 +255,12 @@ func runAdd(out io.Writer, args []string) int {
 
 // runList parses and executes the warp list subcommand.
 func runList(out io.Writer, _ []string) int {
-	cwd, err := paths.Getwd()
+	cwd, err := hubgeometry.Getwd()
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
 
-	_, err = paths.Resolve(cwd)
+	_, err = hubgeometry.Resolve(cwd)
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
@@ -291,12 +291,12 @@ func runList(out io.Writer, _ []string) int {
 // weft side without requiring the user to supply a branch name. On success it
 // emits a JSON object with branch and weft_worktree fields.
 func runCheckout(out io.Writer, args []string) int {
-	cwd, err := paths.Getwd()
+	cwd, err := hubgeometry.Getwd()
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
 
-	l, err := paths.Resolve(cwd)
+	l, err := hubgeometry.Resolve(cwd)
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
@@ -349,12 +349,12 @@ func runCheckout(out io.Writer, args []string) int {
 // calls Status to enumerate all host↔weft pairs with drift and pollution data,
 // and emits the result via output.Ok.
 func runPairs(out io.Writer, _ []string) int {
-	cwd, err := paths.Getwd()
+	cwd, err := hubgeometry.Getwd()
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
 
-	l, err := paths.Resolve(cwd)
+	l, err := hubgeometry.Resolve(cwd)
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
@@ -381,12 +381,12 @@ func runPairs(out io.Writer, _ []string) int {
 // calls Reconcile to walk and repair all host↔weft pairs, and emits the
 // result via output.Ok.
 func runReconcile(out io.Writer, _ []string) int {
-	cwd, err := paths.Getwd()
+	cwd, err := hubgeometry.Getwd()
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
 
-	l, err := paths.Resolve(cwd)
+	l, err := hubgeometry.Resolve(cwd)
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
@@ -410,12 +410,12 @@ func runReconcile(out io.Writer, _ []string) int {
 // runPruneWithFlag executes the prune logic with the resolved apply flag.
 // It is called from the pruneCmd RunE after reading --apply from the cobra flag set.
 func runPruneWithFlag(out io.Writer, apply bool) int {
-	cwd, err := paths.Getwd()
+	cwd, err := hubgeometry.Getwd()
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
 
-	l, err := paths.Resolve(cwd)
+	l, err := hubgeometry.Resolve(cwd)
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
@@ -439,12 +439,12 @@ func runPruneWithFlag(out io.Writer, apply bool) int {
 // runCleanupWithFlags executes the cleanup logic with the resolved apply and force flags.
 // It is called from the cleanupCmd RunE after reading --apply and --force from the cobra flag set.
 func runCleanupWithFlags(out io.Writer, apply, force bool) int {
-	cwd, err := paths.Getwd()
+	cwd, err := hubgeometry.Getwd()
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
 
-	l, err := paths.Resolve(cwd)
+	l, err := hubgeometry.Resolve(cwd)
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
@@ -469,12 +469,12 @@ func runCleanupWithFlags(out io.Writer, apply, force bool) int {
 // It is called from the removeCmd RunE after reading --force from the cobra flag set.
 // Under cobra, args[0] is the slug (cobra has already consumed "remove" from the list).
 func runRemoveWithFlag(out io.Writer, args []string, force bool) int {
-	cwd, err := paths.Getwd()
+	cwd, err := hubgeometry.Getwd()
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
 
-	l, err := paths.Resolve(cwd)
+	l, err := hubgeometry.Resolve(cwd)
 	if err != nil {
 		return output.Err(out, err.Error())
 	}
diff --git a/internal/warpcli/warp_test.go b/internal/warpcli/warp_test.go
index 2bf37b9..052e3cc 100644
--- a/internal/warpcli/warp_test.go
+++ b/internal/warpcli/warp_test.go
@@ -14,8 +14,8 @@ import (
 	"strings"
 	"testing"
 
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 	"github.com/Knatte18/loomyard/internal/warpcli"
 )
 
@@ -28,10 +28,10 @@ func setupCLIRepo(t *testing.T) string {
 	f := lyxtest.CopyHostHub(t)
 	t.Chdir(f.Hub)
 
-	if err := os.MkdirAll(paths.ConfigDir(f.Hub), 0755); err != nil {
+	if err := os.MkdirAll(hubgeometry.ConfigDir(f.Hub), 0755); err != nil {
 		t.Fatalf("create config dir: %v", err)
 	}
-	if err := os.WriteFile(paths.ConfigFile(f.Hub, "warp"), []byte("branch_prefix: wt-\n"), 0644); err != nil {
+	if err := os.WriteFile(hubgeometry.ConfigFile(f.Hub, "warp"), []byte("branch_prefix: wt-\n"), 0644); err != nil {
 		t.Fatalf("write warp.yaml: %v", err)
 	}
 	return f.Hub
diff --git a/internal/warpengine/add.go b/internal/warpengine/add.go
index 5790f47..1bd7391 100644
--- a/internal/warpengine/add.go
+++ b/internal/warpengine/add.go
@@ -11,7 +11,7 @@ import (
 	"strings"
 
 	"github.com/Knatte18/loomyard/internal/gitexec"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // AddOptions controls optional behaviour for Add. Tests pass these directly
@@ -72,7 +72,7 @@ type AddResult struct {
 // The ORIGINAL error is returned; rollback-step failures are not masked.
 //
 // Returns AddResult on success or an error if any step fails.
-func (w *Worktree) Add(l *paths.Layout, slug string, opts AddOptions) (AddResult, error) {
+func (w *Worktree) Add(l *hubgeometry.Layout, slug string, opts AddOptions) (AddResult, error) {
 	// (1) Clean check
 	stdout, stderr, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain", "--untracked-files=no"}, l.WorktreeRoot)
 	if err != nil {
@@ -231,7 +231,7 @@ func (w *Worktree) Add(l *paths.Layout, slug string, opts AddOptions) (AddResult
 // Note: Add does not create the host _lyx junction (it is dormant), so rollback
 // does not remove it. The junction is wired by lyx init via WireJunctions.
 // All errors are collected; the original error passed to the caller is preserved.
-func (w *Worktree) rollbackAdd(l *paths.Layout, slug, branch, target string) error {
+func (w *Worktree) rollbackAdd(l *hubgeometry.Layout, slug, branch, target string) error {
 	var firstErr error
 
 	// (1) Remove weft worktree and branch
diff --git a/internal/warpengine/checkout.go b/internal/warpengine/checkout.go
index e8d381e..befdc20 100644
--- a/internal/warpengine/checkout.go
+++ b/internal/warpengine/checkout.go
@@ -13,7 +13,7 @@ import (
 	"strings"
 
 	"github.com/Knatte18/loomyard/internal/gitexec"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // CheckoutResult contains the fields produced by a successful Checkout.
@@ -41,7 +41,7 @@ type CheckoutResult struct {
 //     return the original error untouched; the pair is never left half-switched.
 //
 // Returns CheckoutResult on success or an error if any step fails.
-func (w *Worktree) Checkout(l *paths.Layout, branch string) (CheckoutResult, error) {
+func (w *Worktree) Checkout(l *hubgeometry.Layout, branch string) (CheckoutResult, error) {
 	weftWorktree := l.WeftWorktree()
 
 	// (1) Precondition: refuse if the weft worktree is dirty. A dirty weft would mean
@@ -118,7 +118,7 @@ func (w *Worktree) Checkout(l *paths.Layout, branch string) (CheckoutResult, err
 // as the fork point and creates the new branch in-place via git switch -c. This
 // preserves the shared merge-base needed for future squash-merge-back operations,
 // matching Add's adopt-or-create fork-point logic.
-func (w *Worktree) switchOrForkWeft(l *paths.Layout, branch string) error {
+func (w *Worktree) switchOrForkWeft(l *hubgeometry.Layout, branch string) error {
 	weftWorktree := l.WeftWorktree()
 
 	if weftBranchExists(l, branch) {
@@ -179,7 +179,7 @@ func (w *Worktree) switchOrForkWeft(l *paths.Layout, branch string) error {
 // before the failure point — the junctions still point to the original branch state
 // and are therefore consistent with the rolled-back host branch. Rewiring would be
 // incorrect here and is not needed.
-func (w *Worktree) rollbackHostSwitch(l *paths.Layout, originalBranch string) {
+func (w *Worktree) rollbackHostSwitch(l *hubgeometry.Layout, originalBranch string) {
 	// Best-effort: silently ignore rollback failure because the caller already holds
 	// the original error that triggered this rollback.
 	_, _, _, _ = gitexec.RunGit([]string{"switch", originalBranch}, l.WorktreeRoot)
diff --git a/internal/warpengine/cleanup.go b/internal/warpengine/cleanup.go
index 680d40d..4201a1c 100644
--- a/internal/warpengine/cleanup.go
+++ b/internal/warpengine/cleanup.go
@@ -19,7 +19,7 @@ import (
 	"strings"
 
 	"github.com/Knatte18/loomyard/internal/gitexec"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // CleanupBranchEntry describes the fate of one orphaned weft branch under Cleanup.
@@ -65,7 +65,7 @@ func codeguideFoldedBack(_ string) bool {
 // worktree sibling and, according to the flag matrix, reports or deletes them.
 //
 // Orphaned weft branches are identified by comparing all weft branch names against
-// the set of host worktree slugs enumerated via paths.List. The board repo branch
+// the set of host worktree slugs enumerated via hubgeometry.List. The board repo branch
 // namespace is excluded — only the weft repo's branches are examined.
 //
 // The flag matrix governs deletion:
@@ -77,11 +77,11 @@ func codeguideFoldedBack(_ string) bool {
 //
 // Returns CleanupResult on success or an error on fatal system failures. Per-branch
 // deletion errors are recorded inline in CleanupBranchEntry.Error.
-func (w *Worktree) Cleanup(l *paths.Layout, apply, force bool) (CleanupResult, error) {
+func (w *Worktree) Cleanup(l *hubgeometry.Layout, apply, force bool) (CleanupResult, error) {
 	// Enumerate host worktrees to build the set of known host slugs.
-	// We use paths.List rather than scanning the hub directory so we only consider
+	// We use hubgeometry.List rather than scanning the hub directory so we only consider
 	// git-registered worktrees, not arbitrary directories.
-	entries, err := paths.List(l.WorktreeRoot)
+	entries, err := hubgeometry.List(l.WorktreeRoot)
 	if err != nil {
 		return CleanupResult{}, fmt.Errorf("list host worktrees: %w", err)
 	}
@@ -148,7 +148,7 @@ func (w *Worktree) Cleanup(l *paths.Layout, apply, force bool) (CleanupResult, e
 // It runs git branch --format=%(refname:short) in the weft repo root to get a
 // clean, newline-delimited list of branch names with no decoration. Returns an
 // error if the git command fails to spawn or exits non-zero.
-func listWeftBranches(l *paths.Layout) ([]string, error) {
+func listWeftBranches(l *hubgeometry.Layout) ([]string, error) {
 	out, stderr, exitCode, err := gitexec.RunGit(
 		[]string{"branch", "--format=%(refname:short)"},
 		l.WeftRepoRoot(),
@@ -170,7 +170,7 @@ func listWeftBranches(l *paths.Layout) ([]string, error) {
 
 // deleteWeftBranch deletes a single weft branch via git branch -D and records
 // any error in entry.Error. Returns true only when the deletion succeeded.
-func deleteWeftBranch(l *paths.Layout, branch string, entry *CleanupBranchEntry) bool {
+func deleteWeftBranch(l *hubgeometry.Layout, branch string, entry *CleanupBranchEntry) bool {
 	_, stderr, exitCode, err := gitexec.RunGit(
 		[]string{"branch", "-D", branch},
 		l.WeftRepoRoot(),
diff --git a/internal/warpengine/clone.go b/internal/warpengine/clone.go
index a5b7288..ce99037 100644
--- a/internal/warpengine/clone.go
+++ b/internal/warpengine/clone.go
@@ -10,7 +10,7 @@ import (
 	"strings"
 
 	"github.com/Knatte18/loomyard/internal/gitexec"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // RemoveAll is an exported testability seam for os.RemoveAll, allowing tests to inject errors.
@@ -47,7 +47,7 @@ func CloneHub(cwd, hostURL, weftURL, boardURL string) (hubPath, resolvedBoardURL
 	}
 
 	// Step 2: Compute Hub path
-	hubPath = paths.HubPath(cwd, name)
+	hubPath = hubgeometry.HubPath(cwd, name)
 
 	// Step 3: Check if Hub already exists
 	if _, err := os.Stat(hubPath); err == nil {
@@ -69,7 +69,7 @@ func CloneHub(cwd, hostURL, weftURL, boardURL string) (hubPath, resolvedBoardURL
 	// warnings fire on every subsequent git checkout within this repo.
 	// Hook installation is non-fatal: a failure is logged but does not abort
 	// the clone (the hook is belt-and-suspenders for usability, not correctness).
-	if hookLayout, err := paths.Resolve(hostWorktreePath); err == nil {
+	if hookLayout, err := hubgeometry.Resolve(hostWorktreePath); err == nil {
 		if hookErr := InstallPostCheckoutHook(hookLayout); hookErr != nil {
 			log.Printf("warp clone: post-checkout hook install (non-fatal): %v", hookErr)
 		}
@@ -78,7 +78,7 @@ func CloneHub(cwd, hostURL, weftURL, boardURL string) (hubPath, resolvedBoardURL
 	}
 
 	// Step 6: Clone weft repo
-	if err := cloneRepo(weftURL, paths.WeftSiblingPath(hubPath, name)); err != nil {
+	if err := cloneRepo(weftURL, hubgeometry.WeftSiblingPath(hubPath, name)); err != nil {
 		return "", "", teardownHub(hubPath, err)
 	}
 
@@ -89,7 +89,7 @@ func CloneHub(cwd, hostURL, weftURL, boardURL string) (hubPath, resolvedBoardURL
 	}
 
 	// Step 8: Clone board repo
-	if err := cloneRepo(board, paths.BoardDir(hubPath)); err != nil {
+	if err := cloneRepo(board, hubgeometry.BoardDir(hubPath)); err != nil {
 		return "", "", teardownHub(hubPath, err)
 	}
 
diff --git a/internal/warpengine/clone_integration_test.go b/internal/warpengine/clone_integration_test.go
index b9f2ea9..278f131 100644
--- a/internal/warpengine/clone_integration_test.go
+++ b/internal/warpengine/clone_integration_test.go
@@ -9,8 +9,8 @@ import (
 	"path/filepath"
 	"testing"
 
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 )
 
 // makeBareRemote creates a bare git repository with a single commit on the main branch.
@@ -92,7 +92,7 @@ func TestCloneHub_HappyPath(t *testing.T) {
 	// Assert hub directory structure
 	hostPath := filepath.Join(hubPath, "myrepo")
 	weftPath := filepath.Join(hubPath, "myrepo-weft")
-	boardPath := paths.BoardDir(hubPath)
+	boardPath := hubgeometry.BoardDir(hubPath)
 
 	// Check that repos exist and are git repos
 	for _, path := range []string{hostPath, weftPath, boardPath} {
@@ -136,9 +136,9 @@ func TestCloneHub_GeometryRoundTrip(t *testing.T) {
 
 	// Resolve geometry from the cloned host Prime
 	hostPath := filepath.Join(hubPath, "myrepo")
-	layout, err := paths.Resolve(hostPath)
+	layout, err := hubgeometry.Resolve(hostPath)
 	if err != nil {
-		t.Fatalf("paths.Resolve: %v", err)
+		t.Fatalf("hubgeometry.Resolve: %v", err)
 	}
 
 	// Assert geometry
@@ -178,7 +178,7 @@ func TestCloneHub_ExplicitBoardURL(t *testing.T) {
 	}
 
 	// Assert board repo exists
-	boardPath := paths.BoardDir(hubPath)
+	boardPath := hubgeometry.BoardDir(hubPath)
 	if _, err := os.Stat(boardPath); err != nil {
 		t.Fatalf("board does not exist: %s", boardPath)
 	}
diff --git a/internal/warpengine/config_test.go b/internal/warpengine/config_test.go
index 1f3651b..239af80 100644
--- a/internal/warpengine/config_test.go
+++ b/internal/warpengine/config_test.go
@@ -11,7 +11,7 @@ import (
 	"strings"
 	"testing"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/warpengine"
 )
 
@@ -21,17 +21,17 @@ func TestLoadConfig_HappyPath(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/config/ directories
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
 
 	// Write a config file with branch_prefix
-	configFile := paths.ConfigFile(tmpDir, "warp")
+	configFile := hubgeometry.ConfigFile(tmpDir, "warp")
 	content := `branch_prefix: hanf/
 `
 	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
@@ -53,17 +53,17 @@ func TestLoadConfig_EmptyBranchPrefix(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/config/ directories
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
 
 	// Write a config file with empty branch_prefix
-	configFile := paths.ConfigFile(tmpDir, "warp")
+	configFile := hubgeometry.ConfigFile(tmpDir, "warp")
 	content := `branch_prefix: ""
 `
 	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
@@ -87,17 +87,17 @@ func TestLoadConfig_EnvResolution(t *testing.T) {
 	t.Setenv("TEST_BRANCH_PREFIX", "feature/")
 
 	// Create _lyx/config/ directories
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
 
 	// Write config with env variable
-	configFile := paths.ConfigFile(tmpDir, "warp")
+	configFile := hubgeometry.ConfigFile(tmpDir, "warp")
 	content := `branch_prefix: ${env:TEST_BRANCH_PREFIX}
 `
 	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
diff --git a/internal/warpengine/drift.go b/internal/warpengine/drift.go
index 15f5d98..1e8def8 100644
--- a/internal/warpengine/drift.go
+++ b/internal/warpengine/drift.go
@@ -14,7 +14,7 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/fslink"
 	"github.com/Knatte18/loomyard/internal/gitexec"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // PairInSync reports whether the host worktree and its paired weft worktree are in sync.
@@ -29,7 +29,7 @@ import (
 // Returns (true, "", nil) if the pair is in sync.
 // Returns (false, reason, nil) if the pair is out of sync; reason describes the divergence.
 // Returns (false, "", err) if the check encounters a system error (e.g., git failure, stat error).
-func PairInSync(l *paths.Layout) (ok bool, reason string, err error) {
+func PairInSync(l *hubgeometry.Layout) (ok bool, reason string, err error) {
 	// Verify the host worktree's current branch via rev-parse --abbrev-ref HEAD.
 	hostOut, _, exitCode, err := gitexec.RunGit(
 		[]string{"rev-parse", "--abbrev-ref", "HEAD"},
diff --git a/internal/warpengine/drift_test.go b/internal/warpengine/drift_test.go
index 807d519..3b311c6 100644
--- a/internal/warpengine/drift_test.go
+++ b/internal/warpengine/drift_test.go
@@ -13,8 +13,8 @@ import (
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/fslink"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 )
 
 // TestPairInSync_BranchDivergence verifies that mismatched host and weft branches
@@ -43,7 +43,7 @@ func TestPairInSync_BranchDivergence(t *testing.T) {
 	lyxtest.MustRun(t, hostWorktreePath, "git", "checkout", "-b", "diverge-test")
 
 	// Resolve layout for the host and check pair sync.
-	hostLayout, err := paths.Resolve(hostWorktreePath)
+	hostLayout, err := hubgeometry.Resolve(hostWorktreePath)
 	if err != nil {
 		t.Fatalf("resolve layout for host: %v", err)
 	}
@@ -82,7 +82,7 @@ func TestPairInSync_BrokenJunction(t *testing.T) {
 	}
 
 	// Resolve layout for the paired host worktree.
-	hostLayout, err := paths.Resolve(f.Layout.WorktreePath(slug))
+	hostLayout, err := hubgeometry.Resolve(f.Layout.WorktreePath(slug))
 	if err != nil {
 		t.Fatalf("resolve layout: %v", err)
 	}
diff --git a/internal/warpengine/hook.go b/internal/warpengine/hook.go
index 7116b6e..3224335 100644
--- a/internal/warpengine/hook.go
+++ b/internal/warpengine/hook.go
@@ -12,7 +12,7 @@ import (
 	"strings"
 
 	"github.com/Knatte18/loomyard/internal/gitexec"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // postCheckoutScript is the embedded POSIX sh post-checkout hook body.
@@ -58,7 +58,7 @@ _user_exit=$?
 // On platforms that support chmod (non-Windows), the file is marked executable.
 // On Windows, git reads and executes the hook via its bundled bash regardless of
 // the file mode, so the chmod is a no-op but harmless.
-func InstallPostCheckoutHook(l *paths.Layout) error {
+func InstallPostCheckoutHook(l *hubgeometry.Layout) error {
 	// Resolve the common git directory so the hook lands in the shared .git
 	// even when called from a linked worktree (where --git-dir differs).
 	commonDirOut, _, exitCode, err := gitexec.RunGit(
diff --git a/internal/warpengine/hook_test.go b/internal/warpengine/hook_test.go
index 24e1339..8bcfd72 100644
--- a/internal/warpengine/hook_test.go
+++ b/internal/warpengine/hook_test.go
@@ -13,8 +13,8 @@ import (
 	"strings"
 	"testing"
 
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 )
 
 // resolveCommonHooksDir returns the common git hooks directory for the repo
@@ -43,9 +43,9 @@ func TestInstallPostCheckoutHook_Idempotent(t *testing.T) {
 	t.Parallel()
 
 	f := lyxtest.CopyHostHub(t)
-	l, err := paths.Resolve(f.Hub)
+	l, err := hubgeometry.Resolve(f.Hub)
 	if err != nil {
-		t.Fatalf("paths.Resolve(%q): %v", f.Hub, err)
+		t.Fatalf("hubgeometry.Resolve(%q): %v", f.Hub, err)
 	}
 
 	if err := InstallPostCheckoutHook(l); err != nil {
@@ -90,9 +90,9 @@ func TestInstallPostCheckoutHook_ChainIdempotent(t *testing.T) {
 	const userHookContent = "#!/bin/sh\necho user\n"
 
 	f := lyxtest.CopyHostHub(t)
-	l, err := paths.Resolve(f.Hub)
+	l, err := hubgeometry.Resolve(f.Hub)
 	if err != nil {
-		t.Fatalf("paths.Resolve(%q): %v", f.Hub, err)
+		t.Fatalf("hubgeometry.Resolve(%q): %v", f.Hub, err)
 	}
 
 	// Plant a user hook.
@@ -154,9 +154,9 @@ func TestInstallPostCheckoutHook_WeftResolution_Prime(t *testing.T) {
 	t.Parallel()
 
 	f := lyxtest.CopyPairedLocal(t)
-	l, err := paths.Resolve(f.Hub)
+	l, err := hubgeometry.Resolve(f.Hub)
 	if err != nil {
-		t.Fatalf("paths.Resolve(%q): %v", f.Hub, err)
+		t.Fatalf("hubgeometry.Resolve(%q): %v", f.Hub, err)
 	}
 
 	// Install the hook in the shared repo.
@@ -198,9 +198,9 @@ func TestInstallPostCheckoutHook_WeftResolution_Child(t *testing.T) {
 	const slug = "hook-child-test"
 
 	f := lyxtest.CopyPairedLocal(t)
-	l, err := paths.Resolve(f.Hub)
+	l, err := hubgeometry.Resolve(f.Hub)
 	if err != nil {
-		t.Fatalf("paths.Resolve(%q): %v", f.Hub, err)
+		t.Fatalf("hubgeometry.Resolve(%q): %v", f.Hub, err)
 	}
 
 	// Create a child worktree pair via Add; the child host is on branch <slug>.
diff --git a/internal/warpengine/junction.go b/internal/warpengine/junction.go
index b67663b..4353e3a 100644
--- a/internal/warpengine/junction.go
+++ b/internal/warpengine/junction.go
@@ -15,7 +15,7 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/fslink"
 	"github.com/Knatte18/loomyard/internal/gitexec"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // WireJunctions creates directory junctions and seeds git-exclude entries for the
@@ -37,7 +37,7 @@ import (
 // Returns nil on success. Returns an error if:
 //   - The host contains a real directory predating weft (violation of pristine invariant)
 //   - Junction or exclude operations fail (wrapped with context)
-func WireJunctions(l *paths.Layout, slug string) error {
+func WireJunctions(l *hubgeometry.Layout, slug string) error {
 	// Create or verify host junctions
 	if err := seedLyxJunction(l, slug); err != nil {
 		return err
@@ -66,7 +66,7 @@ func WireJunctions(l *paths.Layout, slug string) error {
 //
 // Otherwise:
 //   - Returns an error indicating the host repo contains a real directory that predates weft
-func seedLyxJunction(l *paths.Layout, slug string) error {
+func seedLyxJunction(l *hubgeometry.Layout, slug string) error {
 	junctions := l.HostJunctions(slug)
 
 	for _, j := range junctions {
@@ -122,7 +122,7 @@ func seedLyxJunction(l *paths.Layout, slug string) error {
 // path via git rev-parse --git-path info/exclude. If the path is relative, joins it
 // with the worktree path. Preserves line-exact idempotency per name.
 // Idempotent: re-running when all junction names are already present is a no-op.
-func seedGitExclude(l *paths.Layout, slug string) error {
+func seedGitExclude(l *hubgeometry.Layout, slug string) error {
 	worktreePath := l.WorktreePath(slug)
 
 	// Get the exclude path via git rev-parse --git-path
diff --git a/internal/warpengine/launchers.go b/internal/warpengine/launchers.go
index 4424b70..9148d22 100644
--- a/internal/warpengine/launchers.go
+++ b/internal/warpengine/launchers.go
@@ -10,7 +10,7 @@ import (
 	"runtime"
 	"strings"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // writeLaunchers writes per-worktree launchers for the given slug.
@@ -28,7 +28,7 @@ import (
 //	@cd /d "%~dp0<climb-backslash>" && lyx ide menu
 //
 // On non-Windows: returns nil (no-op).
-func writeLaunchers(l *paths.Layout, slug string) error {
+func writeLaunchers(l *hubgeometry.Layout, slug string) error {
 	if runtime.GOOS != "windows" {
 		return nil // No-op on non-Windows
 	}
@@ -91,7 +91,7 @@ func writeLaunchers(l *paths.Layout, slug string) error {
 // in the leaf _launchers/<RelPath>/ dir, the prune stops there in practice,
 // removing only LauncherDir(slug) itself (intended asymmetry).
 // Returns nil if the directory does not exist (os.RemoveAll returns nil for non-existent paths).
-func removeLaunchers(l *paths.Layout, slug string) error {
+func removeLaunchers(l *hubgeometry.Layout, slug string) error {
 	launcherDir := l.LauncherDir(slug)
 	if err := os.RemoveAll(launcherDir); err != nil {
 		return fmt.Errorf("remove launcher dir %s: %w", launcherDir, err)
diff --git a/internal/warpengine/launchers_test.go b/internal/warpengine/launchers_test.go
index f039c9e..3d6f8f6 100644
--- a/internal/warpengine/launchers_test.go
+++ b/internal/warpengine/launchers_test.go
@@ -12,8 +12,8 @@ import (
 	"strings"
 	"testing"
 
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 )
 
 // TestWriteLaunchers covers launcher file creation on Windows.
@@ -107,9 +107,9 @@ func TestWriteLaunchers(t *testing.T) {
 				cwd = f.Hub
 			}
 
-			l, err := paths.Resolve(cwd)
+			l, err := hubgeometry.Resolve(cwd)
 			if err != nil {
-				t.Fatalf("paths.Resolve(%q): %v", cwd, err)
+				t.Fatalf("hubgeometry.Resolve(%q): %v", cwd, err)
 			}
 
 			// Write launchers.
@@ -168,9 +168,9 @@ func TestRemoveLaunchers(t *testing.T) {
 	}
 
 	f := lyxtest.CopyHostHub(t)
-	l, err := paths.Resolve(f.Hub)
+	l, err := hubgeometry.Resolve(f.Hub)
 	if err != nil {
-		t.Fatalf("paths.Resolve(%q): %v", f.Hub, err)
+		t.Fatalf("hubgeometry.Resolve(%q): %v", f.Hub, err)
 	}
 
 	// Write launchers for two slugs.
diff --git a/internal/warpengine/list.go b/internal/warpengine/list.go
index f09f74b..18bed7e 100644
--- a/internal/warpengine/list.go
+++ b/internal/warpengine/list.go
@@ -1,19 +1,19 @@
 // list.go exposes the worktree List operation as a thin wrapper over the shared
-// porcelain parser in internal/paths.
+// porcelain parser in internal/hubgeometry.
 
 package warpengine
 
 import (
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
-// WorktreeEntry is a type alias for paths.WorktreeEntry.
-type WorktreeEntry = paths.WorktreeEntry
+// WorktreeEntry is a type alias for hubgeometry.WorktreeEntry.
+type WorktreeEntry = hubgeometry.WorktreeEntry
 
 // List returns a list of all git worktrees in the repository.
 //
 // The sourceDir is any worktree in the repository (usually the main checkout).
-// Delegates to paths.List for the actual implementation.
+// Delegates to hubgeometry.List for the actual implementation.
 func (w *Worktree) List(sourceDir string) ([]WorktreeEntry, error) {
-	return paths.List(sourceDir)
+	return hubgeometry.List(sourceDir)
 }
diff --git a/internal/warpengine/portals.go b/internal/warpengine/portals.go
index 056d13a..ed78ec2 100644
--- a/internal/warpengine/portals.go
+++ b/internal/warpengine/portals.go
@@ -8,14 +8,14 @@ import (
 	"path/filepath"
 
 	"github.com/Knatte18/loomyard/internal/fslink"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // createPortal creates a portal junction from <container>/_portals/<RelPath>/<slug> to <container>/<slug>/<relpath>/_lyx.
 //
 // Delegates to fslink.CreateDirLink with the computed link and target paths.
 // fslink.CreateDirLink already MkdirAll's filepath.Dir(link), creating the mirrored _portals/<RelPath>/ chain.
-func createPortal(l *paths.Layout, slug string) error {
+func createPortal(l *hubgeometry.Layout, slug string) error {
 	link := l.PortalLink(slug)
 	target := l.PortalTarget(slug)
 	return fslink.CreateDirLink(link, target)
@@ -27,7 +27,7 @@ func createPortal(l *paths.Layout, slug string) error {
 // After successful/idempotent removal, prunes empty mirrored ancestors up to but not
 // including <container>/_portals/. Returns nil if the link does not exist (idempotent).
 // Returns an error if removal fails.
-func removePortal(l *paths.Layout, slug string) error {
+func removePortal(l *hubgeometry.Layout, slug string) error {
 	link := l.PortalLink(slug)
 	if err := fslink.Remove(link); err != nil {
 		return fmt.Errorf("remove portal %s: %w", link, err)
diff --git a/internal/warpengine/portals_test.go b/internal/warpengine/portals_test.go
index 7925c37..1db9253 100644
--- a/internal/warpengine/portals_test.go
+++ b/internal/warpengine/portals_test.go
@@ -10,21 +10,21 @@ import (
 	"path/filepath"
 	"testing"
 
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 )
 
 // setupPortalTarget resolves a layout from the given directory and creates
 // the target _lyx directory structure for the given slug and layout.
 // Returns the resolved layout and the created _lyx target directory path.
 // If portal creation is unsupported on this platform, skips the test.
-func setupPortalTarget(t *testing.T, dir string, slug string) (*paths.Layout, string) {
+func setupPortalTarget(t *testing.T, dir string, slug string) (*hubgeometry.Layout, string) {
 	t.Helper()
 
 	// Resolve layout from directory.
-	l, err := paths.Resolve(dir)
+	l, err := hubgeometry.Resolve(dir)
 	if err != nil {
-		t.Fatalf("paths.Resolve(%q): %v", dir, err)
+		t.Fatalf("hubgeometry.Resolve(%q): %v", dir, err)
 	}
 
 	// Create target _lyx directory structure.
@@ -42,7 +42,7 @@ func setupPortalTarget(t *testing.T, dir string, slug string) (*paths.Layout, st
 }
 
 // TestCreatePortal covers the createPortal and removePortal helpers.
-// It creates a paths.Layout from a test repo subdirectory (non-trivial RelPath),
+// It creates a hubgeometry.Layout from a test repo subdirectory (non-trivial RelPath),
 // creates the target _lyx/ dir, calls createPortal and asserts the junction
 // resolves to the target at the mirrored location l.PortalLink(slug).
 // Then it calls removePortal and asserts the link is gone, empty ancestors are
diff --git a/internal/warpengine/prune.go b/internal/warpengine/prune.go
index 55545e5..4e9db85 100644
--- a/internal/warpengine/prune.go
+++ b/internal/warpengine/prune.go
@@ -11,7 +11,7 @@ import (
 	"path/filepath"
 
 	"github.com/Knatte18/loomyard/internal/gitexec"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // PruneEntry describes one stale or orphaned pair that Prune has identified.
@@ -51,13 +51,13 @@ type PruneResult struct {
 //
 // Live pairs (both host and weft directories exist and are registered) are never
 // touched. The board repo is excluded entirely — Prune only considers host worktrees
-// discovered via paths.List.
+// discovered via hubgeometry.List.
 //
 // Returns PruneResult on success or an error on fatal system failures. Per-entry
 // removal errors are recorded inline in PruneEntry.Error.
-func (w *Worktree) Prune(l *paths.Layout, apply bool) (PruneResult, error) {
+func (w *Worktree) Prune(l *hubgeometry.Layout, apply bool) (PruneResult, error) {
 	// Enumerate all registered host worktrees from the repository.
-	entries, err := paths.List(l.WorktreeRoot)
+	entries, err := hubgeometry.List(l.WorktreeRoot)
 	if err != nil {
 		return PruneResult{}, fmt.Errorf("list worktrees: %w", err)
 	}
@@ -122,7 +122,7 @@ func (w *Worktree) Prune(l *paths.Layout, apply bool) (PruneResult, error) {
 		// Only consider directories that follow the <slug>-weft naming convention.
 		// WeftHostSlug rejects entries that are not valid weft names (wrong suffix or
 		// empty slug after stripping), matching the skip semantics of the old guard.
-		hostSlug, ok := paths.WeftHostSlug(name)
+		hostSlug, ok := hubgeometry.WeftHostSlug(name)
 		if !ok {
 			continue
 		}
@@ -158,7 +158,7 @@ func (w *Worktree) Prune(l *paths.Layout, apply bool) (PruneResult, error) {
 // It writes any removal error into pe.Error and returns true only when
 // the removal completed without error. The caller has already set pe fields
 // other than Removed and Error.
-func removeStalePair(l *paths.Layout, weftPath string, pe *PruneEntry) bool {
+func removeStalePair(l *hubgeometry.Layout, weftPath string, pe *PruneEntry) bool {
 	// Attempt to remove via git worktree remove --force. We use --force because
 	// the host is already gone so the weft may have been left in a dirty state.
 	_, stderr, exitCode, err := gitexec.RunGit(
diff --git a/internal/warpengine/reconcile.go b/internal/warpengine/reconcile.go
index 5c20559..08f5ee5 100644
--- a/internal/warpengine/reconcile.go
+++ b/internal/warpengine/reconcile.go
@@ -15,7 +15,7 @@ import (
 	"strings"
 
 	"github.com/Knatte18/loomyard/internal/gitexec"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // ReconcileAction describes the corrective action applied to one host↔weft pair.
@@ -81,12 +81,12 @@ type ReconcileResult struct {
 //     not apply, or branch is not managed) → report, touch nothing.
 //
 // The layout l provides Hub, Prime, and WeftRepoRoot geometry. Reconcile never walks
-// the raw branch namespace — it only acts on worktrees it finds via paths.List.
+// the raw branch namespace — it only acts on worktrees it finds via hubgeometry.List.
 // Returns an error only on fatal system failures; per-worktree errors are recorded
 // inline in ReconcilePairResult.Error.
-func (w *Worktree) Reconcile(l *paths.Layout) (ReconcileResult, error) {
+func (w *Worktree) Reconcile(l *hubgeometry.Layout) (ReconcileResult, error) {
 	// Enumerate all host worktrees from any worktree in the repository.
-	entries, err := paths.List(l.WorktreeRoot)
+	entries, err := hubgeometry.List(l.WorktreeRoot)
 	if err != nil {
 		return ReconcileResult{}, fmt.Errorf("list worktrees: %w", err)
 	}
@@ -108,7 +108,7 @@ func (w *Worktree) Reconcile(l *paths.Layout) (ReconcileResult, error) {
 
 		// Build a per-host-worktree layout so junction geometry and branch resolution
 		// are rooted at the correct worktree rather than the cwd worktree.
-		hostLayout, layoutErr := paths.Resolve(hostPath)
+		hostLayout, layoutErr := hubgeometry.Resolve(hostPath)
 		if layoutErr != nil {
 			pr.Error = fmt.Sprintf("resolve layout: %v", layoutErr)
 			pr.Action = ReconcileActionUnmanagedReported
@@ -169,8 +169,8 @@ func (w *Worktree) Reconcile(l *paths.Layout) (ReconcileResult, error) {
 //     weft branch and worktree dormant (no junction).
 //  3. Otherwise → report unmanaged and touch nothing.
 func (w *Worktree) reconcileMissingWeft(
-	l *paths.Layout,
-	hostLayout *paths.Layout,
+	l *hubgeometry.Layout,
+	hostLayout *hubgeometry.Layout,
 	hostPath, weftPath, slug, hostBranch string,
 	pr *ReconcilePairResult,
 ) ReconcileAction {
@@ -210,7 +210,7 @@ func (w *Worktree) reconcileMissingWeft(
 
 // adoptWeftWorktree creates a git worktree at weftPath for the existing branch in the
 // weft repo. This is the "adopt" path: the branch already exists so no -b flag is used.
-func adoptWeftWorktree(hostLayout *paths.Layout, weftPath, branch string) error {
+func adoptWeftWorktree(hostLayout *hubgeometry.Layout, weftPath, branch string) error {
 	// git worktree add <path> <branch> — no -b because the branch already exists.
 	_, stderr, exitCode, err := gitexec.RunGit(
 		[]string{"worktree", "add", weftPath, branch},
@@ -233,7 +233,7 @@ func adoptWeftWorktree(hostLayout *paths.Layout, weftPath, branch string) error
 // This is a heuristic: if _lyx exists as a directory or junction the host may already be
 // managed by a different lyx instance, which is beyond raw adoption.
 func isRawHostWorktree(hostPath string) bool {
-	lyxPath := filepath.Join(hostPath, paths.LyxDirName)
+	lyxPath := filepath.Join(hostPath, hubgeometry.LyxDirName)
 	_, err := os.Lstat(lyxPath)
 	// A raw host worktree has no _lyx at all.
 	return os.IsNotExist(err)
@@ -242,7 +242,7 @@ func isRawHostWorktree(hostPath string) bool {
 // createDormantWeftForRawHost creates a weft branch and worktree for a raw host worktree,
 // leaving it dormant (no junction wiring). The weft branch forks from the current weft HEAD
 // (parallel to the add adopt-or-create logic). The caller must run lyx init to wire junctions.
-func createDormantWeftForRawHost(hostLayout *paths.Layout, l *paths.Layout, slug, hostBranch string) error {
+func createDormantWeftForRawHost(hostLayout *hubgeometry.Layout, l *hubgeometry.Layout, slug, hostBranch string) error {
 	weftRoot := hostLayout.WeftRepoRoot()
 
 	// Capture the current weft HEAD branch as the fork point for the new weft branch.
diff --git a/internal/warpengine/reconcile_test.go b/internal/warpengine/reconcile_test.go
index 489921a..19d3662 100644
--- a/internal/warpengine/reconcile_test.go
+++ b/internal/warpengine/reconcile_test.go
@@ -13,8 +13,8 @@ import (
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/fslink"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 )
 
 // setupReconcileFixture prepares a CopyPairedLocal fixture with warp config seeded into the
@@ -181,9 +181,9 @@ func TestReconcile_RawHostWorktreeAdopted(t *testing.T) {
 	})
 
 	// Confirm no _lyx in the raw worktree and no weft branch.
-	rawLayout, err := paths.Resolve(rawHostPath)
+	rawLayout, err := hubgeometry.Resolve(rawHostPath)
 	if err != nil {
-		t.Fatalf("paths.Resolve(rawHostPath): %v", err)
+		t.Fatalf("hubgeometry.Resolve(rawHostPath): %v", err)
 	}
 	if weftBranchExists(rawLayout, rawBranch) {
 		t.Fatalf("pre-condition: weft branch %q must not exist for raw-adopt path", rawBranch)
@@ -227,7 +227,7 @@ func TestReconcile_RawHostWorktreeAdopted(t *testing.T) {
 	}
 
 	// The host _lyx must still be absent (dormant = no junction; lyx init wires it).
-	lyxPath := filepath.Join(rawHostPath, paths.LyxDirName)
+	lyxPath := filepath.Join(rawHostPath, hubgeometry.LyxDirName)
 	if _, statErr := os.Lstat(lyxPath); !os.IsNotExist(statErr) {
 		t.Errorf("host _lyx at %s exists after raw-adopt; want absent (lyx init wires the junction)", lyxPath)
 	}
@@ -257,15 +257,15 @@ func TestReconcile_UnmanagedBranchReportedUntouched(t *testing.T) {
 	})
 
 	// Place a real _lyx directory (not a junction) so the worktree is not "raw".
-	fakeLyx := filepath.Join(unmanagedHostPath, paths.LyxDirName)
+	fakeLyx := filepath.Join(unmanagedHostPath, hubgeometry.LyxDirName)
 	if err := os.MkdirAll(fakeLyx, 0o755); err != nil {
 		t.Fatalf("MkdirAll fake _lyx: %v", err)
 	}
 
 	// Confirm pre-conditions: no weft branch, not raw (has _lyx dir).
-	unmanagedLayout, err := paths.Resolve(unmanagedHostPath)
+	unmanagedLayout, err := hubgeometry.Resolve(unmanagedHostPath)
 	if err != nil {
-		t.Fatalf("paths.Resolve(unmanagedHostPath): %v", err)
+		t.Fatalf("hubgeometry.Resolve(unmanagedHostPath): %v", err)
 	}
 	if weftBranchExists(unmanagedLayout, unmanagedBranch) {
 		t.Fatalf("pre-condition: weft branch %q must not exist", unmanagedBranch)
diff --git a/internal/warpengine/remove.go b/internal/warpengine/remove.go
index 8c956a2..f018239 100644
--- a/internal/warpengine/remove.go
+++ b/internal/warpengine/remove.go
@@ -10,7 +10,7 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/fslink"
 	"github.com/Knatte18/loomyard/internal/gitexec"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // RemoveResult contains the result of successfully removing a worktree.
@@ -44,7 +44,7 @@ type RemoveResult struct {
 // 10. Leave <container>/_launchers/ide-menu.cmd in place.
 //
 // Returns RemoveResult on success or an error if the target doesn't exist or other failures occur.
-func (w *Worktree) Remove(l *paths.Layout, slug string, force bool) (RemoveResult, error) {
+func (w *Worktree) Remove(l *hubgeometry.Layout, slug string, force bool) (RemoveResult, error) {
 	// Compute weft branch name (mirrored)
 	branch := w.cfg.BranchPrefix + slug
 
diff --git a/internal/warpengine/remove_test.go b/internal/warpengine/remove_test.go
index ac69699..07ec154 100644
--- a/internal/warpengine/remove_test.go
+++ b/internal/warpengine/remove_test.go
@@ -11,8 +11,8 @@ import (
 	"path/filepath"
 	"testing"
 
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 )
 
 // TestRemove covers paired teardown: clean removal of both host and weft, the dirty-tree
@@ -210,9 +210,9 @@ func TestRemoveSubpathJunction(t *testing.T) {
 	// Change to subpath to resolve Layout with RelPath set.
 	t.Chdir(subpathDir)
 
-	l, err := paths.Resolve(subpathDir)
+	l, err := hubgeometry.Resolve(subpathDir)
 	if err != nil {
-		t.Fatalf("paths.Resolve: %v", err)
+		t.Fatalf("hubgeometry.Resolve: %v", err)
 	}
 
 	// Verify RelPath is set.
diff --git a/internal/warpengine/status.go b/internal/warpengine/status.go
index aa24efc..c5552ab 100644
--- a/internal/warpengine/status.go
+++ b/internal/warpengine/status.go
@@ -1,6 +1,6 @@
 // status.go implements the paired host↔weft status view and host-pollution detection for warp.
 //
-// Status enumerates all host worktrees via paths.List, pairs each with its weft sibling,
+// Status enumerates all host worktrees via hubgeometry.List, pairs each with its weft sibling,
 // reports branch, in-sync verdict, junction health, and scans the host index for any
 // _lyx or _codeguide paths that have been accidentally git-tracked (host pollution).
 
@@ -14,7 +14,7 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/fslink"
 	"github.com/Knatte18/loomyard/internal/gitexec"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // PollutionEntry describes a single tracked path in the host index that should never
@@ -60,7 +60,7 @@ type StatusResult struct {
 // Status returns the paired host↔weft status view for all worktrees reachable from
 // the given layout, plus host-pollution detection on the host index.
 //
-// For each host worktree discovered via paths.List, Status:
+// For each host worktree discovered via hubgeometry.List, Status:
 //   - Derives the paired weft worktree path via layout geometry
 //   - Reads the host branch and weft branch (if the weft exists)
 //   - Reports in-sync status via PairInSync from the host's layout
@@ -73,9 +73,9 @@ type StatusResult struct {
 // and Prime fields for deriving the weft repo root and weft worktree names.
 // Returns an error only on fatal system failures; per-worktree errors are recorded
 // inline in PairStatus.DriftReason / PairStatus.JunctionReason.
-func (w *Worktree) Status(l *paths.Layout) (StatusResult, error) {
+func (w *Worktree) Status(l *hubgeometry.Layout) (StatusResult, error) {
 	// Enumerate all host worktrees from any worktree in the repository.
-	entries, err := paths.List(l.WorktreeRoot)
+	entries, err := hubgeometry.List(l.WorktreeRoot)
 	if err != nil {
 		return StatusResult{}, fmt.Errorf("list worktrees: %w", err)
 	}
@@ -124,7 +124,7 @@ func (w *Worktree) Status(l *paths.Layout) (StatusResult, error) {
 		// Build a per-host-worktree layout to call PairInSync. PairInSync requires a
 		// Layout whose WorktreeRoot is the host worktree being inspected, so we derive
 		// one from the host path rather than reusing l (which points to the cwd worktree).
-		hostLayout, layoutErr := paths.Resolve(hostPath)
+		hostLayout, layoutErr := hubgeometry.Resolve(hostPath)
 		if layoutErr != nil {
 			pair.DriftReason = fmt.Sprintf("resolve host layout: %v", layoutErr)
 			result.Pairs = append(result.Pairs, pair)
diff --git a/internal/warpengine/status_test.go b/internal/warpengine/status_test.go
index 4b3d2ac..6e92e2b 100644
--- a/internal/warpengine/status_test.go
+++ b/internal/warpengine/status_test.go
@@ -13,8 +13,8 @@ import (
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/fslink"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 )
 
 // setupStatusFixture prepares a CopyPairedLocal fixture with warp config seeded and the
@@ -160,7 +160,7 @@ func TestStatus_JunctionHealth(t *testing.T) {
 	}
 
 	// Rebuild layout since the junction removal may affect resolution.
-	brokenLayout, err := paths.Resolve(f.Hub)
+	brokenLayout, err := hubgeometry.Resolve(f.Hub)
 	if err != nil {
 		t.Fatalf("Resolve after junction removal: %v", err)
 	}
diff --git a/internal/warpengine/weftwiring.go b/internal/warpengine/weftwiring.go
index d028a48..021ef9f 100644
--- a/internal/warpengine/weftwiring.go
+++ b/internal/warpengine/weftwiring.go
@@ -18,13 +18,13 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/fslink"
 	"github.com/Knatte18/loomyard/internal/gitexec"
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // weftRepoExists reports whether a weft repo exists at the expected location.
 //
 // A weft repo must be a directory that passes the git rev-parse --is-inside-work-tree check.
-func weftRepoExists(l *paths.Layout) bool {
+func weftRepoExists(l *hubgeometry.Layout) bool {
 	weftRepoRoot := l.WeftRepoRoot()
 
 	// Check if directory exists
@@ -45,7 +45,7 @@ func weftRepoExists(l *paths.Layout) bool {
 // weftBranchExists reports whether a branch exists in the weft repo.
 //
 // It uses git rev-parse --verify to check for the branch.
-func weftBranchExists(l *paths.Layout, branch string) bool {
+func weftBranchExists(l *hubgeometry.Layout, branch string) bool {
 	_, _, exitCode, err := gitexec.RunGit(
 		[]string{"rev-parse", "--verify", "refs/heads/" + branch},
 		l.WeftRepoRoot(),
@@ -62,7 +62,7 @@ func weftBranchExists(l *paths.Layout, branch string) bool {
 // shared merge-base needed for future squash-merge-back operations. Runs
 // git worktree add -b <branch> <path> <startPoint> in the weft repo root.
 // Returns an error if the command fails or exits with non-zero code.
-func createWeftWorktree(l *paths.Layout, slug, branch, startPoint string) error {
+func createWeftWorktree(l *hubgeometry.Layout, slug, branch, startPoint string) error {
 	weftPath := l.WeftWorktreePath(slug)
 	_, stderr, exitCode, err := gitexec.RunGit(
 		[]string{"worktree", "add", "-b", branch, weftPath, startPoint},
@@ -85,7 +85,7 @@ func createWeftWorktree(l *paths.Layout, slug, branch, startPoint string) error
 //
 // Otherwise, runs git push -u origin <branch> from the weft worktree.
 // Returns an error if the command fails or exits with non-zero code.
-func pushWeftBranch(l *paths.Layout, slug, branch string, opts AddOptions) error {
+func pushWeftBranch(l *hubgeometry.Layout, slug, branch string, opts AddOptions) error {
 	if opts.SkipGit || opts.SkipPush {
 		return nil
 	}
@@ -110,7 +110,7 @@ func pushWeftBranch(l *paths.Layout, slug, branch string, opts AddOptions) error
 // Uses fslink.Remove to delete the junction/symlink only (idempotent).
 // Returns nil if the junction does not exist (idempotent).
 // Returns an error if removal fails for reasons other than not-exist.
-func removeHostJunction(l *paths.Layout, slug string) error {
+func removeHostJunction(l *hubgeometry.Layout, slug string) error {
 	link := l.HostLyxLink(slug)
 	if err := fslink.Remove(link); err != nil {
 		return fmt.Errorf("remove host junction %s: %w", link, err)
@@ -127,7 +127,7 @@ func removeHostJunction(l *paths.Layout, slug string) error {
 //
 // All commands run with cwd = WeftRepoRoot.
 // Returns the first error encountered, or nil if all steps succeed.
-func removeWeftWorktree(l *paths.Layout, slug, branch string, force bool) error {
+func removeWeftWorktree(l *hubgeometry.Layout, slug, branch string, force bool) error {
 	weftPath := l.WeftWorktreePath(slug)
 	weftRoot := l.WeftRepoRoot()
 
diff --git a/internal/weftcli/cli.go b/internal/weftcli/cli.go
index 44dfefd..7f788de 100644
--- a/internal/weftcli/cli.go
+++ b/internal/weftcli/cli.go
@@ -14,8 +14,8 @@ import (
 	"path/filepath"
 
 	"github.com/Knatte18/loomyard/internal/clihelp"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/output"
-	"github.com/Knatte18/loomyard/internal/paths"
 	"github.com/Knatte18/loomyard/internal/weftengine"
 	"github.com/spf13/cobra"
 )
@@ -31,7 +31,7 @@ import (
 func Command() *cobra.Command {
 	// Closure vars populated by PersistentPreRunE and read by subcommand RunEs.
 	var (
-		l        *paths.Layout
+		l        *hubgeometry.Layout
 		cfg      weftengine.Config
 		pathspec []string
 		bypass   bool   // true when --weft-path is set
@@ -77,14 +77,14 @@ func Command() *cobra.Command {
 			}
 
 			// Normal mode: resolve cwd → layout → config → pathspec.
-			cwd, err := paths.Getwd()
+			cwd, err := hubgeometry.Getwd()
 			if err != nil {
 				output.Err(out, err.Error())
 				clihelp.Abort(ctx, 1)
 				return nil
 			}
 
-			resolved, err := paths.Resolve(cwd)
+			resolved, err := hubgeometry.Resolve(cwd)
 			if err != nil {
 				output.Err(out, err.Error())
 				clihelp.Abort(ctx, 1)
diff --git a/internal/weftcli/cli_test.go b/internal/weftcli/cli_test.go
index 21e3d46..b8a1a37 100644
--- a/internal/weftcli/cli_test.go
+++ b/internal/weftcli/cli_test.go
@@ -14,8 +14,8 @@ import (
 	"strings"
 	"testing"
 
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 	"github.com/Knatte18/loomyard/internal/weftengine"
 )
 
@@ -184,12 +184,12 @@ func TestRunCLI_EnvMapToOption(t *testing.T) {
 
 	hubPath := fixture.Hub
 
-	// Change to the hub directory so paths.Resolve can locate the repo from cwd;
+	// Change to the hub directory so hubgeometry.Resolve can locate the repo from cwd;
 	// t.Chdir restores the original cwd automatically after the test.
 	t.Chdir(hubPath)
 
 	// Modify a file in the weft config that would be committed
-	weftConfigFile := filepath.Join(fixture.WeftPrime, paths.LyxDirName, "placeholder")
+	weftConfigFile := filepath.Join(fixture.WeftPrime, hubgeometry.LyxDirName, "placeholder")
 	if err := os.WriteFile(weftConfigFile, []byte("modified"), 0o644); err != nil {
 		t.Fatalf("WriteFile: %v", err)
 	}
diff --git a/internal/weftengine/config_test.go b/internal/weftengine/config_test.go
index 8cb28c5..36ccaad 100644
--- a/internal/weftengine/config_test.go
+++ b/internal/weftengine/config_test.go
@@ -8,7 +8,7 @@ import (
 	"strings"
 	"testing"
 
-	"github.com/Knatte18/loomyard/internal/paths"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 )
 
 // TestConfigDirs tests the Dirs() method on Config.
@@ -47,17 +47,17 @@ func TestLoadConfig_HappyPath(t *testing.T) {
 	tmpDir := t.TempDir()
 
 	// Create _lyx/config/ directories
-	lyxDir := filepath.Join(tmpDir, paths.LyxDirName)
+	lyxDir := filepath.Join(tmpDir, hubgeometry.LyxDirName)
 	if err := os.Mkdir(lyxDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx: %v", err)
 	}
-	configDir := paths.ConfigDir(tmpDir)
+	configDir := hubgeometry.ConfigDir(tmpDir)
 	if err := os.Mkdir(configDir, 0755); err != nil {
 		t.Fatalf("failed to create _lyx/config: %v", err)
 	}
 
 	// Write a config file with pathspec
-	configFile := paths.ConfigFile(tmpDir, "weft")
+	configFile := hubgeometry.ConfigFile(tmpDir, "weft")
 	content := `pathspec: _lyx _codeguide
 `
 	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
diff --git a/internal/weftengine/weft_integration_test.go b/internal/weftengine/weft_integration_test.go
index 07c210f..f1af7e5 100644
--- a/internal/weftengine/weft_integration_test.go
+++ b/internal/weftengine/weft_integration_test.go
@@ -11,8 +11,8 @@ import (
 	"strings"
 	"testing"
 
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/lyxtest"
-	"github.com/Knatte18/loomyard/internal/paths"
 )
 
 func TestPushIntegration(t *testing.T) {
@@ -45,7 +45,7 @@ func TestPushIntegration(t *testing.T) {
 			weftRepo := fixture.WeftPath
 
 			// Commit a change
-			lyxFile := filepath.Join(weftRepo, paths.LyxDirName, "config.yaml")
+			lyxFile := filepath.Join(weftRepo, hubgeometry.LyxDirName, "config.yaml")
 			if err := os.WriteFile(lyxFile, []byte(tt.fileContent), 0o644); err != nil {
 				t.Fatalf("WriteFile: %v", err)
 			}

```

## Instructions

1. Read the failing tests and the source files they exercise.
2. Fix the root cause of the failures. Do not modify tests unless they are genuinely wrong due to the merge (e.g. a test asserted against a value that the merge legitimately changed).
3. Re-run `go build ./... && go test ./tools/sandbox/... ./internal/paths/...` after each fix attempt using `git -C C:\Code\loomyard\wts\sandbox-report-json` for git commands.
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

Available: Read, Edit, Write, Bash, Grep, Glob. Use `git -C C:\Code\loomyard\wts\sandbox-report-json` for git commands; do not `cd`. Worktree cwd is `C:\Code\loomyard\wts\sandbox-report-json`.
