# Verify-Fix Brief

The verify command `go test -tags integration -race -count=1 ./internal/gitrepo/...` failed after a merge. Your job is to diagnose the failures and fix the code so the verify command passes.

## Verify Output

```
FAIL	github.com/Knatte18/loomyard/internal/gitrepo [build failed]
FAIL
# github.com/Knatte18/loomyard/internal/gitrepo [github.com/Knatte18/loomyard/internal/gitrepo.test]
internal/gitrepo/gitrepo.go:237:6: undefined: strings
```

## Merge Diff

```diff
diff --git a/CONSTRAINTS.md b/CONSTRAINTS.md
index ca20ab2b..f8e20769 100644
--- a/CONSTRAINTS.md
+++ b/CONSTRAINTS.md
@@ -7,7 +7,7 @@ Short, authoritative list of the repo's structural invariants. Each is partly ma
 `internal/hubgeometry` owns all cwd, worktree-root, and geometry resolution.
 
 - All cwd / worktree-root queries go through `hubgeometry.Getwd()` / `Resolve()`. Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/hubgeometry` and `cmd/lyx/main.go`.
-- Geometry tokens — `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_raddle`, `_lyx` — are owned solely by `internal/hubgeometry`. No other package may use them in a path-construction context (a `filepath.Join` arg, a `+` operand, or a string `const`). Whole-token match; production files only; comparisons and git-pathspec slice literals are not path construction and stay allowed.
+- Geometry tokens — `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_raddle`, `_lyx`, `_pattern` — are owned solely by `internal/hubgeometry`. No other package may use them in a path-construction context (a `filepath.Join` arg, a `+` operand, or a string `const`). Whole-token match; production files only; comparisons and git-pathspec slice literals are not path construction and stay allowed.
 - `_lyx`, its `config/` subdir, and any `<module>.yaml` resolve through `hubgeometry.LyxDirName` / `ConfigDir(base)` / `ConfigFile(base, module)` — **in test code too** (a config-layout migration once broke a hardcoded test fixture).
 - Geometry is structural, never config/env-overridable (the board dir is `--board-path` flag > `hubgeometry.BoardDir(l.Hub)`, not a config key).
 - **Enforced by** `internal/hubgeometry/enforcement_test.go` (`TestEnforcement_GeometryLiterals`) on every `go test`. API and helpers: godoc for `internal/hubgeometry`.
@@ -48,6 +48,12 @@ Short, authoritative list of the repo's structural invariants. Each is partly ma
 - `codeintelcli` → `codeintelengine` is the only allowed direction; the reverse import (`codeintelengine` → `codeintelcli`, or `codeintelengine` → any other feature package) is never allowed.
 - **Enforced by** `internal/codeintelengine/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`) on every `go test`.
 
+## Pattern Leaf Invariant
+
+`internal/pattern` production code imports only stdlib and `internal/hubgeometry` — never `builderengine`, `websterengine`, `burlerengine`, `loomengine`, or any other feature package — so every one of those four consumers can import it without cycles; the reverse import (`pattern` → any feature package) is never allowed.
+
+- **Enforced by** `internal/pattern/leaf_enforcement_test.go` (`TestLeafInvariant_AllowlistOnly`) on every `go test`.
+
 ## CLI / Cobra Invariant
 
 Every lyx CLI module is a cobra subtree assembled under one root in `cmd/lyx/main.go`.
diff --git a/docs/overview.md b/docs/overview.md
index bc4bce55..566197da 100644
--- a/docs/overview.md
+++ b/docs/overview.md
@@ -94,7 +94,9 @@ The test: **would this state mean anything on a different machine?** Orchestrati
 
 Each host worktree has a sibling weft worktree. Host worktrees use **junctions** (Windows) or symlinks to route writes into the sibling weft worktree:
 - `<host>/_lyx` → `<hub>/<slug>-weft/_lyx` (config junction)
-- `<host>/_raddle` → `<hub>/<slug>-weft/_raddle` (raddle junction)
+- `<host>/_pattern` → `<hub>/<slug>-weft/_pattern` (PATTERN constraint-injection junction)
+
+No `_raddle` junction is wired in this release — `internal/fabricengine/status.go`'s host-pollution scan is explicit that no junction exists for `_raddle` yet, and `hubgeometry.HostJunctions` has never returned one; a prior version of this list incorrectly claimed one, which this entry corrects.
 
 Junctions are listed in `.git/info/exclude` per worktree and are never committed to `.gitignore`. From the CLI's perspective, reads and writes happen transparently — code that writes to `_lyx/config/board.yaml` writes through the junction into the weft repo without awareness of the indirection.
 
@@ -143,6 +145,7 @@ github.com/Knatte18/loomyard/
 ├── internal/output/              shared JSON output
 ├── internal/modelspec/           model-spec parser + models.yaml registry leaf
 ├── internal/tokenvocab/          shared token vocabulary (repo, hub) + Render compose over stencil, a leaf
+├── internal/pattern/             PATTERN active check + role directive leaf, consumed by builder/webster/burler/loom
 └── internal/shell/               provider-invariant pane-shell mechanics leaf (pwsh + posix)
 ```
 
@@ -182,7 +185,7 @@ The cross-OS spawn primitive **proc** is the one remaining internal (non-CLI) la
 
 **init** is not a module but a cross-cutting setup command (`lyx init`) that scaffolds the shared `_lyx/` config dir for every module.
 
-The user-facing modules sit on a thin layer of shared infrastructure (`internal/configengine`, `internal/gitexec`, `internal/gitrepo`, `internal/lock`, `internal/output`, `internal/hubgeometry`, `internal/state`, `internal/shell`, `internal/modelspec`, `internal/tokenvocab`) — defined in [shared-libs/README.md](shared-libs/README.md).
+The user-facing modules sit on a thin layer of shared infrastructure (`internal/configengine`, `internal/gitexec`, `internal/gitrepo`, `internal/lock`, `internal/output`, `internal/hubgeometry`, `internal/state`, `internal/shell`, `internal/modelspec`, `internal/tokenvocab`, `internal/pattern`) — defined in [shared-libs/README.md](shared-libs/README.md). `internal/pattern` is the leaf that computes whether `_pattern/PATTERN.md` is present and returns the role-appropriate constraints directive injected into every code-touching agent prompt (builder implementer, webster fork/Master, burler review+fix, loom plan).
 
 ## Execution stack (orchestration layers)
 
diff --git a/docs/shared-libs/hubgeometry.md b/docs/shared-libs/hubgeometry.md
index 664ee9ad..0e7459e9 100644
--- a/docs/shared-libs/hubgeometry.md
+++ b/docs/shared-libs/hubgeometry.md
@@ -25,6 +25,7 @@ These three constants are the single source of the geometry tokens for the whole
 - **`WeftSuffix`** (`"-weft"`) — suffix appended to a host-worktree slug to form its weft sibling directory name (e.g. `"feat"` → `"feat-weft"`). Use `WeftSiblingPath` / `WeftRepoRoot` / `WeftWorktreePath` rather than constructing the path from this constant directly.
 - **`BoardDirName`** (`"_board"`) — name of the board data directory inside the hub (i.e. `<hub>/_board`). Use `BoardDir(hub)` to obtain the full path.
 - **`HubSuffix`** (`"-HUB"`) — suffix appended to a repo name to form its hub container directory (e.g. `"loomyard"` → `"loomyard-HUB"`). Use `HubPath(parent, name)` to obtain the full path.
+- **`PatternDirName`** (`"_pattern"`) — directory name for the PATTERN constraint-injection surface within a worktree (i.e. `<worktree>/_pattern`). Use `PatternDir(baseDir)` / `PatternFile(baseDir)` to obtain the full paths; the filename `"PATTERN.md"` is deliberately not a geometry constant, since only those two accessors ever need it.
 
 ### Functions
 
@@ -75,6 +76,13 @@ These functions resolve configuration file paths. They take a `baseDir` (the dir
 - **`ConfigFile(baseDir, module string) string`** — Returns `filepath.Join(ConfigDir(baseDir), module+".yaml")`. The path to a specific module's configuration file (e.g., `_lyx/config/board.yaml`).
 - **`DotEnv(baseDir string) string`** — Returns `filepath.Join(baseDir, ".env")`. The path to the environment variable overrides file.
 
+### Pattern path helpers
+
+These functions resolve the PATTERN constraint-injection surface's paths. Like the config path helpers above, they take a `baseDir` parameter, not a `Layout`, and are the only place `internal/pattern` (and every agent) may obtain a `_pattern` path — per the Hub Geometry Invariant, no consumer joins `PatternDirName` and `"PATTERN.md"` itself.
+
+- **`PatternDir(baseDir string) string`** — Returns `filepath.Join(baseDir, PatternDirName)`. The `_pattern` directory holding `PATTERN.md`.
+- **`PatternFile(baseDir string) string`** — Returns `filepath.Join(PatternDir(baseDir), "PATTERN.md")`. The path to the PATTERN surface file itself.
+
 ### Geometry bootstrap functions
 
 These pure functions construct geometry paths without requiring a resolved `Layout`. They are the correct way for early-stage callers (pre-init, pre-layout, bootstrap code) to form geometry paths. They consume the geometry constants above — no caller needs to repeat the raw suffix strings.
@@ -101,6 +109,21 @@ These pure functions construct geometry paths without requiring a resolved `Layo
 - **`MenuLauncherRel() string`** — `filepath.Rel(filepath.Dir(MenuLauncherPath()), filepath.Join(Prime, RelPath))`. The relative path from the menu launcher directory to the Prime worktree's subpath for menu spawning.
 - **`PrimeName() string`** — `filepath.Base(Prime)`. The Prime worktree's directory name (stable, used in paths like `ide-menu.cmd`).
 
+### Pattern junction methods (Layout)
+
+These four methods mirror their `_lyx` counterparts exactly (`WeftLyxDir`/`WeftLyxDirFor`/`HostLyxLink`/`HostLyxLinkHere`), substituting `PatternDirName` for `LyxDirName`. They are the junction endpoints for the `_pattern` weft/host pair.
+
+- **`WeftPatternDir() string`** — `filepath.Join(WeftWorktree(), RelPath, PatternDirName)`. The `_pattern` directory in the current worktree's weft sibling; the junction target for `WeftPatternDir`/`HostPatternLinkHere`.
+- **`WeftPatternDirFor(slug string) string`** — `filepath.Join(WeftWorktreePath(slug), RelPath, PatternDirName)`. The `_pattern` directory within a named slug's weft worktree; pairs with `HostPatternLink(slug)`.
+- **`HostPatternLink(slug string) string`** — `filepath.Join(WorktreePath(slug), RelPath, PatternDirName)`. The `_pattern` junction link in a named slug's host worktree.
+- **`HostPatternLinkHere() string`** — `filepath.Join(WorktreeRoot, RelPath, PatternDirName)`. The `_pattern` junction link in the current host worktree, derived from `WorktreeRoot`+`RelPath` (not `Cwd`).
+- **`PatternFileHere() string`** — `PatternFile(filepath.Join(WorktreeRoot, RelPath))`. The path to `PATTERN.md` for the current worktree; every agent's active check. Anchored at `WorktreeRoot`+`RelPath` rather than `WorktreeRoot` alone (would miss the file in nested-hub geometry) or `Cwd` (would drift from the junction endpoints above). On a `Resolve`-built `Layout` this equals `PatternFile(Cwd)`, since `Resolve` sets `RelPath = filepath.Rel(WorktreeRoot, Cwd)`.
+
+### Junction detection methods (Layout)
+
+- **`HostJunctions(slug string) []HostJunction`** — the list of host junctions for a named slug: two entries, `_lyx` first (`{Name: LyxDirName, Link: HostLyxLink(slug), Target: WeftLyxDirFor(slug)}`) then `_pattern` (`{Name: PatternDirName, Link: HostPatternLink(slug), Target: WeftPatternDirFor(slug)}`). `Hub`/slug-anchored: used by wiring, unwiring, and `remove`.
+- **`HostJunctionsHere() []HostJunction`** — the same two junction records, resolved against the current worktree instead of a slug: `_lyx`'s `Link` comes from `HostLyxLinkHere()` and `Target` from `WeftLyxDir()`; `_pattern`'s `Link` comes from `HostPatternLinkHere()` and `Target` from `WeftPatternDir()`. Used by the three junction health-check sites (`internal/fabricengine/reconcile.go`, `status.go`, `drift.go`), which have no slug available — `PairInSync` in particular takes no slug parameter at all and is documented stateless.
+
 ## Design principles
 
 **Geometry-only.** `hubgeometry` computes *where* things are, never *mutates* them. Worktree creation/removal, junction setup, and config scaffolding stay in the domain modules. `hubgeometry` is the dumb geometry resolver so they can be smart about state transitions.
@@ -119,7 +142,7 @@ These pure functions construct geometry paths without requiring a resolved `Layo
 
 **`TestEnforcement` (cwd/root primitives ban):** Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/hubgeometry` and `cmd/lyx/main.go`. The scan uses a substring check on the raw file bytes and fails the build if either token appears in any non-test `.go` file outside the allowlist.
 
-**`TestEnforcement_GeometryLiterals` (geometry-literal construction ban):** The geometry path tokens `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_raddle`, and `_lyx` may not appear as string literals in a **path-construction context** in any production file outside `internal/hubgeometry`. Path-construction contexts are:
+**`TestEnforcement_GeometryLiterals` (geometry-literal construction ban):** The geometry path tokens `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_raddle`, `_lyx`, and `_pattern` may not appear as string literals in a **path-construction context** in any production file outside `internal/hubgeometry`. Path-construction contexts are:
 
 - An argument to a `filepath.Join(...)` call.
 - An operand of a binary `+` (`token.ADD`) expression.
diff --git a/docs/shared-libs/stencil.md b/docs/shared-libs/stencil.md
index 362d0e4e..ada7af5d 100644
--- a/docs/shared-libs/stencil.md
+++ b/docs/shared-libs/stencil.md
@@ -22,11 +22,16 @@ A **stencil** is a template with cut-out fields you fill to reproduce a pattern
 // It returns an error if any marker in the template has no value — an unfilled
 // marker is never silently left blank.
 func Fill(template []byte, values map[string]string) ([]byte, error)
+
+// FillOptional renders a markdown template exactly like Fill, except every name
+// listed in optional is exempt from Fill's unfilled-top-level-marker guarantee.
+func FillOptional(template []byte, values map[string]string, optional []string) ([]byte, error)
 ```
 
-- **Input:** a markdown template (bytes / an asset file's contents) and a set of named values.
+- **Input:** a markdown template (bytes / an asset file's contents), a set of named values, and (for `FillOptional`) a list of marker names exempt from the unfilled-marker guarantee.
 - **Output:** the filled markdown, ready to hand to `shuttle.Run` as a prompt.
 - **Marker syntax:** the pinned grammar is Go stdlib `text/template` (`text/`, never `html/template` — output must not be HTML-escaped): `{{.X}}` substitution, plus `{{if eq .Type "…"}}` equality conditionals for bulk-vs-tool-use / cluster-present / seeded-context-vs-safety-pass sections. Variadic `eq`, and the `and`/`or`/`not`/comparison operators, come free with `text/template`. A leading `<!-- … -->` comment on the template asset is stripped before parsing.
+- **`Fill` is defined as `FillOptional(template, values, nil)`.** There is one code path, not two parallel implementations that could drift apart; `Fill` simply supplies no optional names.
 
 ## The one load-bearing guarantee — fail on an unfilled marker
 
@@ -34,6 +39,12 @@ This is the reason the leaf exists beyond DRY. **An unfilled marker is a hard er
 
 The built scoping: every **top-level** absent-or-empty marker is collected and reported together, sorted, in one error, and the template is never executed. A **branch-internal** reached-but-absent marker is instead caught incrementally, one per call, via `missingkey=error` — this is not "every hole in one error" for branch-internal markers, only for top-level ones. A caller-required marker (like `fasit`/`target`) must therefore live at the template's top level, never inside a conditional branch.
 
+## The optional-marker exemption — `FillOptional`
+
+A marker name passed in `FillOptional`'s `optional` argument is exempt from **both** of the guards above: the top-level batch check no longer reports it as unfilled, and `missingkey=error` no longer fires on it at execution time even when it is reached only via a branch. Concretely, a listed name that is absent from `values`, present as `""`, or present as a whitespace-only string all render as nothing rather than tripping either guard or leaking literal whitespace into the output — one shared `strings.TrimSpace` definition of "empty" governs both guards and both call paths. A listed name that is present with a genuine non-empty value renders that value exactly as before; the exemption only changes behaviour for the absent-or-empty case. A listed name that never appears in the template at all is a harmless no-op.
+
+Optionality is a property of **the caller's argument list**, not of the template text: a marker is optional because a specific Go call site passed its name in `optional`, which is a testable fact about that call site, unlike wrapping the same field in a `{{if}}` conditional inside the markdown asset. The same template can therefore be filled once with a marker required (via `Fill`, or `FillOptional` with that name omitted from `optional`) and once with it optional (via `FillOptional` with that name listed), depending entirely on which caller is filling it.
+
 ## Consumers
 
 - **`burler`** — the handler prompt and each cluster-reviewer prompt (the pre-assembled bulk blob is passed *as a value*, not read via tools — see the `internal/burlerengine` package documentation).
diff --git a/internal/builderengine/implementer-template.md b/internal/builderengine/implementer-template.md
index 81547b91..c8a7c7d4 100644
--- a/internal/builderengine/implementer-template.md
+++ b/internal/builderengine/implementer-template.md
@@ -6,12 +6,15 @@
      five non-empty and there are no {{if}}/{{range}} conditionals anywhere
      in this file (a required marker inside a conditional branch would
      render silently blank when present-but-empty — see
-     internal/stencil/stencil.go). -->
+     internal/stencil/stencil.go). pattern_directive is the sixth marker,
+     and the one optional one: it is filled via stencil.FillOptional and
+     renders as nothing when PATTERN is inactive. -->
 
 # Builder implementer — one batch, start to finish
 
 You are the implementer for exactly one batch of a pinned plan-format v2 plan. Your job is to read your batch file and the plan overview, implement every card it lists, in order, run the batch's `verify:` command, and — as your FINAL action — write the batch-report file the orchestrator reads to learn what happened. You never drive the batch loop itself; that is the orchestrator's job, one level up.
 
+{{.pattern_directive}}
 ## Your batch and the overview — read both, never another batch's file
 
 Read `{{.batch_file}}` (batch `{{.batch_name}}`) now, in full, and also read `00-overview.md` from the same plan directory: its task framing, Batch Index, and any `## Shared Decisions` section orient you before you touch a single card — a decision made in an earlier batch is not yours to re-derive from scratch. Never read another batch's own file: your batch file plus the overview is the whole of your plan material.
diff --git a/internal/builderengine/spawn.go b/internal/builderengine/spawn.go
index e6a85a39..b2f9492c 100644
--- a/internal/builderengine/spawn.go
+++ b/internal/builderengine/spawn.go
@@ -33,6 +33,7 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/modelspec"
+	"github.com/Knatte18/loomyard/internal/pattern"
 	"github.com/Knatte18/loomyard/internal/shuttleengine"
 	"github.com/Knatte18/loomyard/internal/stencil"
 )
@@ -469,13 +470,14 @@ func SpawnBatch(deps SpawnDeps, opts SpawnBatchOptions) (*SpawnResult, error) {
 	}
 
 	values := map[string]string{
-		"batch_file":    batchFilePath,
-		"batch_name":    batchName,
-		"report_path":   reportPath,
-		"self_fix_cap":  fmt.Sprintf("%d", deps.Config.SelfFixCap),
-		"worktree_root": deps.WorktreeRoot,
-	}
-	prompt, err := stencil.Fill(ImplementerTemplate(), values)
+		"batch_file":        batchFilePath,
+		"batch_name":        batchName,
+		"report_path":       reportPath,
+		"self_fix_cap":      fmt.Sprintf("%d", deps.Config.SelfFixCap),
+		"worktree_root":     deps.WorktreeRoot,
+		"pattern_directive": pattern.Directive(deps.Layout, pattern.RoleImplementer),
+	}
+	prompt, err := stencil.FillOptional(ImplementerTemplate(), values, []string{"pattern_directive"})
 	if err != nil {
 		return nil, fmt.Errorf("builder: fill implementer template: %w", err)
 	}
diff --git a/internal/builderengine/template_test.go b/internal/builderengine/template_test.go
index cc8a62d2..8b99b093 100644
--- a/internal/builderengine/template_test.go
+++ b/internal/builderengine/template_test.go
@@ -102,15 +102,18 @@ func containsKey(text, key string) bool {
 
 // implementerTemplateMarkerValues returns a values map with every one of
 // ImplementerTemplate's five required top-level markers set to a
-// non-empty placeholder, so a test can fill the template cleanly or delete
-// one key at a time to prove stencil.Fill's per-marker error.
+// non-empty placeholder, plus pattern_directive — the one optional marker,
+// filled via stencil.FillOptional — set to a placeholder too, so a test can
+// fill the template cleanly or delete one key at a time to prove
+// stencil.Fill's per-marker error.
 func implementerTemplateMarkerValues() map[string]string {
 	return map[string]string{
-		"batch_file":    "/plan/02-list-tests.md",
-		"batch_name":    "02-list-tests",
-		"report_path":   "/builder/reports/02-list-tests.yaml",
-		"self_fix_cap":  "2",
-		"worktree_root": "/worktree",
+		"batch_file":        "/plan/02-list-tests.md",
+		"batch_name":        "02-list-tests",
+		"report_path":       "/builder/reports/02-list-tests.yaml",
+		"self_fix_cap":      "2",
+		"worktree_root":     "/worktree",
+		"pattern_directive": "## Constraints — do this before you write any code\n\n- Read _pattern/PATTERN.md.",
 	}
 }
 
@@ -152,8 +155,8 @@ func TestImplementerTemplate_StatesBatchDiscipline(t *testing.T) {
 	// overview (framing, Batch Index, Shared Decisions), but still never
 	// another batch's own file.
 	requireContains(t, text, "plan-format v2")
-	requireContains(t, text, "also read\n`00-overview.md`")
-	requireContains(t, text, "Never read another\nbatch's own file")
+	requireContains(t, text, "also read `00-overview.md`")
+	requireContains(t, text, "Never read another batch's own file")
 
 	// The five typed file-op field names a card carries.
 	requireContains(t, text, "**Edits:**")
@@ -164,8 +167,8 @@ func TestImplementerTemplate_StatesBatchDiscipline(t *testing.T) {
 
 	// Rename-mechanic compliance: git mv FIRST, then only surgical edits,
 	// never rewrite-and-recreate.
-	requireContains(t, text, "run\n`git mv <old> <new>` FIRST")
-	requireContains(t, text, "never rewrite\nthe relocated file from scratch and delete the original")
+	requireContains(t, text, "run `git mv <old> <new>` FIRST")
+	requireContains(t, text, "never rewrite the relocated file from scratch and delete the original")
 
 	// Commit-subject rule: the card's own **Commit:** value wins verbatim
 	// when present; otherwise the NN.C fallback is derived.
@@ -359,13 +362,17 @@ func TestOrchestratorTemplate_StatesBatchOrderAndRecoveryLadder(t *testing.T) {
 	requireContains(t, text, "already in flight")
 }
 
-// TestImplementerTemplate_FillsWithAllMarkers asserts stencil.Fill succeeds
-// when every one of ImplementerTemplate's five required markers is
-// supplied, and fails — naming the marker — when any single one is absent.
+// TestImplementerTemplate_FillsWithAllMarkers asserts stencil.FillOptional
+// succeeds when every one of ImplementerTemplate's five required markers
+// plus the optional pattern_directive marker is supplied, and fails —
+// naming the marker — when any single REQUIRED one is absent.
+// pattern_directive is deliberately excluded from this deletion sweep: it
+// is the one optional marker (see the template's own banner comment), so
+// deleting it must not error.
 func TestImplementerTemplate_FillsWithAllMarkers(t *testing.T) {
 	t.Run("all markers supplied", func(t *testing.T) {
-		if _, err := stencil.Fill(builderengine.ImplementerTemplate(), implementerTemplateMarkerValues()); err != nil {
-			t.Fatalf("stencil.Fill() = %v; want nil", err)
+		if _, err := stencil.FillOptional(builderengine.ImplementerTemplate(), implementerTemplateMarkerValues(), []string{"pattern_directive"}); err != nil {
+			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
 		}
 	})
 
@@ -373,13 +380,54 @@ func TestImplementerTemplate_FillsWithAllMarkers(t *testing.T) {
 		t.Run("missing "+marker, func(t *testing.T) {
 			values := implementerTemplateMarkerValues()
 			delete(values, marker)
-			_, err := stencil.Fill(builderengine.ImplementerTemplate(), values)
+			_, err := stencil.FillOptional(builderengine.ImplementerTemplate(), values, []string{"pattern_directive"})
 			if err == nil {
-				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
+				t.Fatalf("stencil.FillOptional() with %q missing = nil error; want error naming the marker", marker)
 			}
 			if !strings.Contains(err.Error(), marker) {
-				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
+				t.Errorf("stencil.FillOptional() error = %q; want it to name marker %q", err.Error(), marker)
 			}
 		})
 	}
 }
+
+// TestImplementerTemplate_PatternDirectiveOptional asserts pattern_directive
+// behaves as an optional marker: an empty value renders cleanly with no
+// leftover `{{`, no orphan `## Constraints` heading, and no stray
+// blank-line block where the directive would have sat, and a non-empty
+// value places the directive block ahead of the first work instruction
+// ("## Your batch and the overview").
+func TestImplementerTemplate_PatternDirectiveOptional(t *testing.T) {
+	t.Run("empty pattern_directive renders cleanly", func(t *testing.T) {
+		values := implementerTemplateMarkerValues()
+		values["pattern_directive"] = ""
+		got, err := stencil.FillOptional(builderengine.ImplementerTemplate(), values, []string{"pattern_directive"})
+		if err != nil {
+			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
+		}
+		text := string(got)
+		if strings.Contains(text, "{{") {
+			t.Errorf("rendered output contains leftover {{: %q", text)
+		}
+		if strings.Contains(text, "## Constraints") {
+			t.Errorf("rendered output contains an orphan ## Constraints heading: %q", text)
+		}
+		if strings.Contains(text, "\n\n\n\n") {
+			t.Errorf("rendered output contains a stray blank-line block: %q", text)
+		}
+	})
+
+	t.Run("non-empty pattern_directive precedes the first work instruction", func(t *testing.T) {
+		values := implementerTemplateMarkerValues()
+		got, err := stencil.FillOptional(builderengine.ImplementerTemplate(), values, []string{"pattern_directive"})
+		if err != nil {
+			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
+		}
+		text := string(got)
+		directiveIdx := strings.Index(text, values["pattern_directive"])
+		workIdx := strings.Index(text, "## Your batch and the overview")
+		if directiveIdx == -1 || workIdx == -1 || directiveIdx >= workIdx {
+			t.Errorf("pattern_directive (idx %d) does not precede the first work instruction (idx %d)", directiveIdx, workIdx)
+		}
+	})
+}
diff --git a/internal/burlerengine/engine.go b/internal/burlerengine/engine.go
index 43fa2e4d..6dcfd06d 100644
--- a/internal/burlerengine/engine.go
+++ b/internal/burlerengine/engine.go
@@ -11,6 +11,7 @@ import (
 	"os"
 
 	"github.com/Knatte18/loomyard/internal/hubgeometry"
+	"github.com/Knatte18/loomyard/internal/pattern"
 	"github.com/Knatte18/loomyard/internal/shuttleengine"
 )
 
@@ -99,7 +100,11 @@ func (e *Engine) Run(p Profile, opts RunOpts) (Result, error) {
 		return Result{}, err
 	}
 
-	prompt, err := composePrompt(&p)
+	// RoleReviewFix, not a reviewer-only role: the template states in as
+	// many words that the agent has two jobs in order in one session, and
+	// part B is fixing what part A found.
+	directive := pattern.Directive(e.layout, pattern.RoleReviewFix)
+	prompt, err := composePrompt(&p, directive)
 	if err != nil {
 		return Result{}, err
 	}
diff --git a/internal/burlerengine/prompt.go b/internal/burlerengine/prompt.go
index 43c57c18..734ab26c 100644
--- a/internal/burlerengine/prompt.go
+++ b/internal/burlerengine/prompt.go
@@ -1,8 +1,14 @@
-// prompt.go composes the burler round prompt: it builds the nine marker
-// values the embedded template (template.go) requires and fills it via
-// internal/stencil. composePrompt is called only after (*Profile).validate
-// has run, so every path field it reads is already a cleaned absolute path
-// and p.clusterLenses (when ClusterFan was set) is already resolved.
+// prompt.go composes the burler round prompt: it builds the nine required
+// marker values the embedded template (template.go) requires, plus the
+// optional pattern_directive marker, and fills it via internal/stencil.
+// composePrompt is called only after (*Profile).validate has run, so every
+// path field it reads is already a cleaned absolute path and
+// p.clusterLenses (when ClusterFan was set) is already resolved.
+// composePrompt itself does no filesystem access beyond the directory
+// check formatFileSet already performs on each Target/Fasit path — it
+// takes patternDirective as a plain string parameter rather than a
+// *hubgeometry.Layout, so it never gains geometry awareness of its own;
+// the caller (Engine.Run) computes the directive.
 
 package burlerengine
 
@@ -15,10 +21,15 @@ import (
 )
 
 // composePrompt builds the burler round prompt for p by composing each of
-// the template's nine top-level marker values (path lists, fix-scope rules,
-// tool-use rules, the prior-rounds block, the cluster-rules block) and
-// filling reviewPromptTemplate with them via stencil.Fill.
-func composePrompt(p *Profile) (string, error) {
+// the template's nine required top-level marker values (path lists,
+// fix-scope rules, tool-use rules, the prior-rounds block, the
+// cluster-rules block), plus patternDirective under the optional
+// pattern_directive marker, and filling reviewPromptTemplate with them via
+// stencil.FillOptional. patternDirective is not gated on whether the
+// round's target is code or prose: loomyard has no target-type
+// classification, and a file-extension heuristic would be new fragile
+// logic whose misclassification would silently drop the constraints.
+func composePrompt(p *Profile, patternDirective string) (string, error) {
 	values := map[string]string{
 		"target":            formatFileSet(p.Target),
 		"fasit":             formatFileSet(p.Fasit),
@@ -29,9 +40,10 @@ func composePrompt(p *Profile) (string, error) {
 		"cluster_rules":     clusterRulesBlock(p),
 		"review_path":       p.ReviewPath,
 		"fixer_report_path": p.FixerReportPath,
+		"pattern_directive": patternDirective,
 	}
 
-	rendered, err := stencil.Fill(reviewPromptTemplate, values)
+	rendered, err := stencil.FillOptional(reviewPromptTemplate, values, []string{"pattern_directive"})
 	if err != nil {
 		return "", fmt.Errorf("burler: compose prompt: %w", err)
 	}
diff --git a/internal/burlerengine/prompt_test.go b/internal/burlerengine/prompt_test.go
index 334dd5f5..e84aeaf2 100644
--- a/internal/burlerengine/prompt_test.go
+++ b/internal/burlerengine/prompt_test.go
@@ -51,7 +51,7 @@ func newComposableProfile(t *testing.T) Profile {
 func TestComposePrompt_FillsAllMarkers(t *testing.T) {
 	p := newComposableProfile(t)
 
-	got, err := composePrompt(&p)
+	got, err := composePrompt(&p, "")
 	if err != nil {
 		t.Fatalf("composePrompt() = %v; want nil error", err)
 	}
@@ -70,7 +70,7 @@ func TestComposePrompt_FixScope(t *testing.T) {
 		p := newComposableProfile(t)
 		p.FixScope = FixScopeSource
 
-		got, err := composePrompt(&p)
+		got, err := composePrompt(&p, "")
 		if err != nil {
 			t.Fatalf("composePrompt() = %v; want nil error", err)
 		}
@@ -82,7 +82,7 @@ func TestComposePrompt_FixScope(t *testing.T) {
 		p := newComposableProfile(t)
 		p.FixScope = FixScopeOverlay
 
-		got, err := composePrompt(&p)
+		got, err := composePrompt(&p, "")
 		if err != nil {
 			t.Fatalf("composePrompt() = %v; want nil error", err)
 		}
@@ -98,7 +98,7 @@ func TestComposePrompt_ToolUse(t *testing.T) {
 		p := newComposableProfile(t)
 		p.ToolUse = true
 
-		got, err := composePrompt(&p)
+		got, err := composePrompt(&p, "")
 		if err != nil {
 			t.Fatalf("composePrompt() = %v; want nil error", err)
 		}
@@ -110,7 +110,7 @@ func TestComposePrompt_ToolUse(t *testing.T) {
 		p := newComposableProfile(t)
 		p.ToolUse = false
 
-		got, err := composePrompt(&p)
+		got, err := composePrompt(&p, "")
 		if err != nil {
 			t.Fatalf("composePrompt() = %v; want nil error", err)
 		}
@@ -126,7 +126,7 @@ func TestComposePrompt_PriorRounds(t *testing.T) {
 	t.Run("first round", func(t *testing.T) {
 		p := newComposableProfile(t)
 
-		got, err := composePrompt(&p)
+		got, err := composePrompt(&p, "")
 		if err != nil {
 			t.Fatalf("composePrompt() = %v; want nil error", err)
 		}
@@ -138,7 +138,7 @@ func TestComposePrompt_PriorRounds(t *testing.T) {
 		p.PriorReviews = []string{filepath.Join(t.TempDir(), "prior-review.md")}
 		p.PriorFixerReports = []string{filepath.Join(t.TempDir(), "prior-fixer-report.md")}
 
-		got, err := composePrompt(&p)
+		got, err := composePrompt(&p, "")
 		if err != nil {
 			t.Fatalf("composePrompt() = %v; want nil error", err)
 		}
@@ -154,7 +154,7 @@ func TestComposePrompt_PriorRounds(t *testing.T) {
 func TestComposePrompt_DirectoryAnnotation(t *testing.T) {
 	p := newComposableProfile(t)
 
-	got, err := composePrompt(&p)
+	got, err := composePrompt(&p, "")
 	if err != nil {
 		t.Fatalf("composePrompt() = %v; want nil error", err)
 	}
@@ -183,7 +183,7 @@ func TestComposePrompt_ClusterRules(t *testing.T) {
 	t.Run("non-cluster", func(t *testing.T) {
 		p := newComposableProfile(t)
 
-		got, err := composePrompt(&p)
+		got, err := composePrompt(&p, "")
 		if err != nil {
 			t.Fatalf("composePrompt() = %v; want nil error", err)
 		}
@@ -199,7 +199,7 @@ func TestComposePrompt_ClusterRules(t *testing.T) {
 			{Name: "security", Text: "pay extra attention to security"},
 		}
 
-		got, err := composePrompt(&p)
+		got, err := composePrompt(&p, "")
 		if err != nil {
 			t.Fatalf("composePrompt() = %v; want nil error", err)
 		}
diff --git a/internal/burlerengine/review-prompt-template.md b/internal/burlerengine/review-prompt-template.md
index ec9ad949..a93af8e0 100644
--- a/internal/burlerengine/review-prompt-template.md
+++ b/internal/burlerengine/review-prompt-template.md
@@ -5,7 +5,10 @@
      top-level {{.X}} substitution; stencil.Fill requires all nine non-empty
      and there are no {{if}}/{{range}} conditionals anywhere in this file
      (a required marker inside a conditional branch would render silently
-     blank when present-but-empty — see internal/stencil/stencil.go). -->
+     blank when present-but-empty — see internal/stencil/stencil.go).
+     pattern_directive is the tenth marker, and the one optional one: it is
+     filled via stencil.FillOptional and renders as nothing when PATTERN is
+     inactive. -->
 
 # Burler round — review, then fix
 
@@ -16,6 +19,7 @@ You are a burler: a single agent doing ONE review+fix round over an artifact. Yo
 
 Do the two jobs in that order, in full, without skipping ahead.
 
+{{.pattern_directive}}
 ## What to review (the target)
 
 {{.target}}
diff --git a/internal/burlerengine/template_test.go b/internal/burlerengine/template_test.go
index e348934d..3552f539 100644
--- a/internal/burlerengine/template_test.go
+++ b/internal/burlerengine/template_test.go
@@ -57,7 +57,7 @@ func TestTemplate_StatesClusterForkDiscipline(t *testing.T) {
 		{Name: "style", Text: "pay extra attention to style"},
 	}
 
-	got, err := composePrompt(&p)
+	got, err := composePrompt(&p, "")
 	if err != nil {
 		t.Fatalf("composePrompt() = %v; want nil error", err)
 	}
@@ -85,8 +85,10 @@ func requireContains(t *testing.T, text, needle string) {
 }
 
 // allMarkerValues returns a values map with every one of the template's
-// nine required top-level markers set to a non-empty placeholder, so tests
-// can delete one key at a time to prove stencil.Fill's per-marker error.
+// nine required top-level markers set to a non-empty placeholder, plus
+// pattern_directive — the one optional marker, filled via
+// stencil.FillOptional — set to a placeholder too, so tests can delete one
+// key at a time to prove stencil.FillOptional's per-marker error.
 func allMarkerValues() map[string]string {
 	return map[string]string{
 		"target":            "target placeholder",
@@ -98,16 +100,21 @@ func allMarkerValues() map[string]string {
 		"cluster_rules":     "cluster-rules placeholder",
 		"review_path":       "/tmp/review.md",
 		"fixer_report_path": "/tmp/fixer-report.md",
+		"pattern_directive": "## Constraints — do this before you judge or change anything\n\n- Read _pattern/PATTERN.md.",
 	}
 }
 
-// TestTemplate_FillsWithAllMarkers asserts stencil.Fill succeeds when every
-// one of the nine required markers is supplied, and fails — naming the
-// marker — when any single one is absent.
+// TestTemplate_FillsWithAllMarkers asserts stencil.FillOptional succeeds
+// when every one of the nine required markers plus the optional
+// pattern_directive marker is supplied, and fails — naming the marker —
+// when any single REQUIRED one is absent. pattern_directive is
+// deliberately excluded from this deletion sweep: it is the one optional
+// marker (see the template's own banner comment), so deleting it must not
+// error.
 func TestTemplate_FillsWithAllMarkers(t *testing.T) {
 	t.Run("all markers supplied", func(t *testing.T) {
-		if _, err := stencil.Fill(reviewPromptTemplate, allMarkerValues()); err != nil {
-			t.Fatalf("stencil.Fill() = %v; want nil", err)
+		if _, err := stencil.FillOptional(reviewPromptTemplate, allMarkerValues(), []string{"pattern_directive"}); err != nil {
+			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
 		}
 	})
 
@@ -118,13 +125,54 @@ func TestTemplate_FillsWithAllMarkers(t *testing.T) {
 		t.Run("missing "+marker, func(t *testing.T) {
 			values := allMarkerValues()
 			delete(values, marker)
-			_, err := stencil.Fill(reviewPromptTemplate, values)
+			_, err := stencil.FillOptional(reviewPromptTemplate, values, []string{"pattern_directive"})
 			if err == nil {
-				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
+				t.Fatalf("stencil.FillOptional() with %q missing = nil error; want error naming the marker", marker)
 			}
 			if !strings.Contains(err.Error(), marker) {
-				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
+				t.Errorf("stencil.FillOptional() error = %q; want it to name marker %q", err.Error(), marker)
 			}
 		})
 	}
 }
+
+// TestTemplate_PatternDirectiveOptional asserts pattern_directive behaves
+// as an optional marker: an empty value renders cleanly with no leftover
+// `{{`, no orphan `## Constraints` heading, and no stray blank-line block
+// where the directive would have sat, and a non-empty value places the
+// directive block ahead of the first work instruction ("## What to review
+// (the target)").
+func TestTemplate_PatternDirectiveOptional(t *testing.T) {
+	t.Run("empty pattern_directive renders cleanly", func(t *testing.T) {
+		values := allMarkerValues()
+		values["pattern_directive"] = ""
+		got, err := stencil.FillOptional(reviewPromptTemplate, values, []string{"pattern_directive"})
+		if err != nil {
+			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
+		}
+		text := string(got)
+		if strings.Contains(text, "{{") {
+			t.Errorf("rendered output contains leftover {{: %q", text)
+		}
+		if strings.Contains(text, "## Constraints") {
+			t.Errorf("rendered output contains an orphan ## Constraints heading: %q", text)
+		}
+		if strings.Contains(text, "\n\n\n\n") {
+			t.Errorf("rendered output contains a stray blank-line block: %q", text)
+		}
+	})
+
+	t.Run("non-empty pattern_directive precedes the first work instruction", func(t *testing.T) {
+		values := allMarkerValues()
+		got, err := stencil.FillOptional(reviewPromptTemplate, values, []string{"pattern_directive"})
+		if err != nil {
+			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
+		}
+		text := string(got)
+		directiveIdx := strings.Index(text, values["pattern_directive"])
+		workIdx := strings.Index(text, "## What to review (the target)")
+		if directiveIdx == -1 || workIdx == -1 || directiveIdx >= workIdx {
+			t.Errorf("pattern_directive (idx %d) does not precede the first work instruction (idx %d)", directiveIdx, workIdx)
+		}
+	})
+}
diff --git a/internal/fabriccli/fabric.go b/internal/fabriccli/fabric.go
index f09ee641..8eff4833 100644
--- a/internal/fabriccli/fabric.go
+++ b/internal/fabriccli/fabric.go
@@ -130,8 +130,8 @@ use "lyx fabric pairs".`,
 	removeCmd = &cobra.Command{
 		Use:   "remove [--force] <slug>",
 		Short: "destroy a dual host+weft worktree pair",
-		Long: `Remove a paired host and weft git worktree, plus all associated portal
-junctions and launchers.
+		Long: `Remove a paired host and weft git worktree, plus every host junction
+(_lyx and _pattern), portal junctions, and launchers.
 
 By default the command refuses to remove a worktree with uncommitted changes
 on either the host or weft side. Use --force to remove anyway.
@@ -172,14 +172,35 @@ Example:
 	cmd.AddCommand(&cobra.Command{
 		Use:   "pairs",
 		Short: "show full host↔weft pair geometry with drift and junction-health fields",
-		RunE:  clihelp.WrapRun(func(out io.Writer, args []string) int { return runPairs(out, args) }),
+		Long: `Show every host↔weft pair's branch, in-sync verdict, junction health, and
+host-pollution scan.
+
+junction_healthy and junction_reason cover BOTH host junctions (_lyx and
+_pattern): a pair is only healthy when every junction resolves to its own
+weft directory, and junction_reason names the first unhealthy one by name
+when it is not. The pollution scan likewise covers _lyx, _pattern, and
+_raddle paths accidentally tracked in the host index; _lyx and _pattern
+matches carry an automated git rm --cached remedy, _raddle matches are
+report-only.`,
+		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int { return runPairs(out, args) }),
 	})
 
 	// reconcile
 	cmd.AddCommand(&cobra.Command{
 		Use:   "reconcile",
 		Short: "repair a managed pair whose weft side drifted or broke",
-		RunE:  clihelp.WrapRun(func(out io.Writer, args []string) int { return runReconcile(out, args) }),
+		Long: `Reconcile walks every host worktree and applies the minimal corrective
+action needed to restore a valid paired topology: recreate a missing weft
+worktree, re-point a broken junction, adopt a raw (non-lyx) host worktree, or
+report an unmanaged branch untouched.
+
+Junction repair covers BOTH host junctions (_lyx and _pattern): if either is
+missing, not a link, or points elsewhere, this re-wires every junction for
+that pair in one call — a pair with only one junction broken is repaired,
+not reported already-healthy. This is also the upgrade path for a worktree
+wired before the _pattern junction existed: one reconcile call adds the
+missing junction and materialises its weft-side target.`,
+		RunE: clihelp.WrapRun(func(out io.Writer, args []string) int { return runReconcile(out, args) }),
 	})
 
 	// prune [--apply]
diff --git a/internal/fabriccli/weft_verbs.go b/internal/fabriccli/weft_verbs.go
index c29628d2..74437b82 100644
--- a/internal/fabriccli/weft_verbs.go
+++ b/internal/fabriccli/weft_verbs.go
@@ -164,7 +164,7 @@ Every fabric weft commit carries a trailing "Warp-SHA: <sha>" trailer naming the
 paired warp repo's current HEAD, recorded into the correspondence index immediately
 after the commit lands.
 
-Staging is scoped to the directories listed in the fabric config (default: _lyx).
+Staging is scoped to the directories listed in the fabric config (default: _lyx _pattern).
 
 Related commands:
   lyx fabric push   — commit then push in the same process
diff --git a/internal/fabricengine/doc.go b/internal/fabricengine/doc.go
index c8bcd11e..1afcc12a 100644
--- a/internal/fabricengine/doc.go
+++ b/internal/fabricengine/doc.go
@@ -23,4 +23,16 @@
 // fabric never calls gitrepo's `StageAllAndCommit` (board's opt-in wildcard-stage
 // exception, per gitrepo's doc.go) — all staging is explicit-list
 // `StageAndCommit`, scoped to a configured pathspec.
+//
+// The default weft-staging pathspec (template.yaml's `pathspec:` key) is `_lyx _pattern`, so a `PATTERN.md` written through the `_pattern` junction is staged and committed alongside `_lyx` by the same `CommitWeft` call, rather than being inert content nothing ever pushes.
+//
+// Two consequences of that default living only in the config template, never enforced or reconciled onto an existing worktree, are worth stating plainly rather than leaving an operator to discover them by surprise.
+//
+// First, existing worktrees never pick this up: `configsync.ReconcileAll` -> `yamlengine.Reconcile` keeps a `pathspec:` key that is already present in a worktree's `fabric.yaml` and adds no key when one already exists, so every already-initialised worktree stays on `pathspec: _lyx` forever and never persists `_pattern` content, no matter how many times `lyx init` or `lyx config` reconcile is re-run — an operator must widen an existing worktree's `fabric.yaml` by hand.
+//
+// Second, no detection or warning surface is in scope: nothing, neither `lyx fabric status` nor `lyx init`, reports a narrow pathspec, so an existing worktree stays silently inert until an operator notices and edits the file themselves. That gap is accepted here rather than papered over — a "your pathspec predates PATTERN" warning would be a new diagnostic class in `fabric status`, and PATTERN has no content to persist in this repo yet — so this comment is what puts the gap in writing instead.
+//
+// This is a deliberate asymmetry with the junction side (see internal/hubgeometry and fabricengine's own junction wiring): a junction self-heals on the next `lyx init`/reconcile and reports loudly until it does, because `WireJunctions` owns junction state outright, whereas `pathspec` is an operator-editable config value that `configsync` must never silently overwrite.
+//
+// The junction side of that asymmetry has a concrete blast radius worth naming rather than leaving an operator to meet as a surprise: every worktree wired before `hubgeometry.HostJunctions` gained its `_pattern` entry lacks the `_pattern` junction, full stop — including this repo's own live worktrees, including whichever one lands this change. Until re-run, `lyx fabric status` reports that pair not in sync, with `JunctionReason` naming `_pattern`; `lyx fabric reconcile` reports `ReconcileActionJunctionRepointed` rather than `ReconcileActionAlreadyHealthy` for it — and repairs it, so reconcile *is* the remedy, not merely a report; and `loom`'s preflight fails `CheckJunction`, sets its `check3BlocksSeed` flag, and blocks the run. The remedy is one `lyx init` (idempotent: wires the missing junction and materialises the weft-side directory) or one `lyx fabric reconcile`; either clears every one of those three symptoms in a single call. This is not suppressed because it should not be: the junction genuinely is missing, and a health check that lied about a missing junction is the exact fault the junction-health generalisation (`checkJunctionHealth`, `PairInSync`, `Status`'s inline verdict) exists to remove.
 package fabricengine
diff --git a/internal/fabricengine/drift.go b/internal/fabricengine/drift.go
index d3ebb932..4bd91c7b 100644
--- a/internal/fabricengine/drift.go
+++ b/internal/fabricengine/drift.go
@@ -1,10 +1,10 @@
 // drift.go implements the stateless pair-in-sync check for fabric topology.
 //
 // PairInSync derives the weft sibling deterministically and checks that the weft
-// worktree is on WeftBranchName(hostBranch), and that the host _lyx junction is
-// valid and points to the weft _lyx directory. It is stateless, consulting no
-// registry: fabric's correspondence check compares the weft branch against
-// WeftBranchName(hostBranch).
+// worktree is on WeftBranchName(hostBranch), and that every host junction
+// (l.HostJunctionsHere()) is valid and points to its own weft directory. It is
+// stateless, consulting no registry: fabric's correspondence check compares the
+// weft branch against WeftBranchName(hostBranch).
 //
 // PairInSync and HostClean (hostclean.go) are wired into the loom preflight
 // via internal/loomengine.
@@ -27,7 +27,8 @@ import (
 // A pair is considered in sync when:
 //   - The weft worktree is on WeftBranchName(hostBranch) (via rev-parse --abbrev-ref HEAD
 //     on both worktrees)
-//   - The host _lyx junction exists and points to the correct weft _lyx directory
+//   - Every host junction in l.HostJunctionsHere() exists and points to its own
+//     weft directory
 //
 // The weft sibling is derived deterministically as <worktree-base>-weft (via paths geometry).
 // No registry or status.md is consulted; PairInSync is stateless.
@@ -70,44 +71,48 @@ func PairInSync(l *hubgeometry.Layout) (ok bool, reason string, err error) {
 		return false, fmt.Sprintf("host on %s, weft on %s (want %s)", hostBranch, weftBranch, expectedWeftBranch), nil
 	}
 
-	// Verify the host _lyx junction is valid and points to the correct weft target.
-	hostLink := l.HostLyxLinkHere()
-	weftTarget := l.WeftLyxDir()
-
-	// Distinguish a missing _lyx entry from an existing one that is not a
-	// link: fslink.IsLink reports (false, nil) for both shapes, and the loom
-	// preflight consumes these reason strings — a real directory sitting
-	// where the junction belongs must not masquerade as merely missing.
-	if _, lstatErr := os.Lstat(hostLink); lstatErr != nil {
-		if os.IsNotExist(lstatErr) {
-			return false, "junction missing", nil
+	// Verify every host junction is valid and points to its correct weft
+	// target — l.HostJunctionsHere(), the same Here-anchored, slug-free
+	// accessor checkJunctionHealth loops in reconcile.go. PairInSync's
+	// signature is unchanged and it stays stateless and slug-free, which is
+	// exactly why it loops HostJunctionsHere() rather than HostJunctions(slug).
+	for _, j := range l.HostJunctionsHere() {
+		// Distinguish a missing junction entry from an existing one that is not
+		// a link: fslink.IsLink reports (false, nil) for both shapes, and the
+		// loom preflight consumes these reason strings — a real directory
+		// sitting where the junction belongs must not masquerade as merely
+		// missing.
+		if _, lstatErr := os.Lstat(j.Link); lstatErr != nil {
+			if os.IsNotExist(lstatErr) {
+				return false, fmt.Sprintf("host %s junction missing", j.Name), nil
+			}
+			return false, "", fmt.Errorf("check host junction: %w", lstatErr)
+		}
+		isLink, err := fslink.IsLink(j.Link)
+		if err != nil {
+			return false, "", fmt.Errorf("check host junction: %w", err)
+		}
+		if !isLink {
+			// Same wording as checkJunctionHealth for this drift shape, so
+			// status/reconcile and PairInSync describe it identically.
+			return false, fmt.Sprintf("host %s is not a junction", j.Name), nil
 		}
-		return false, "", fmt.Errorf("check host junction: %w", lstatErr)
-	}
-	isLink, err := fslink.IsLink(hostLink)
-	if err != nil {
-		return false, "", fmt.Errorf("check host junction: %w", err)
-	}
-	if !isLink {
-		// Same wording as checkJunctionHealth for this drift shape, so
-		// status/reconcile and PairInSync describe it identically.
-		return false, "host _lyx is not a junction", nil
-	}
 
-	// Resolve the junction and verify it points to the correct target.
-	linkTarget, err := fslink.PointsTo(hostLink)
-	if err != nil {
-		return false, "", fmt.Errorf("resolve host junction: %w", err)
-	}
+		// Resolve the junction and verify it points to the correct target.
+		linkTarget, err := fslink.PointsTo(j.Link)
+		if err != nil {
+			return false, "", fmt.Errorf("resolve host junction: %w", err)
+		}
 
-	// Resolve weft target for comparison.
-	weftTargetResolved, err := filepath.EvalSymlinks(weftTarget)
-	if err != nil {
-		return false, "", fmt.Errorf("resolve weft target: %w", err)
-	}
+		// Resolve weft target for comparison.
+		weftTargetResolved, err := filepath.EvalSymlinks(j.Target)
+		if err != nil {
+			return false, "", fmt.Errorf("resolve weft target: %w", err)
+		}
 
-	if linkTarget != weftTargetResolved {
-		return false, "junction points elsewhere", nil
+		if linkTarget != weftTargetResolved {
+			return false, fmt.Sprintf("host %s junction points elsewhere", j.Name), nil
+		}
 	}
 
 	return true, "", nil
diff --git a/internal/fabricengine/junction.go b/internal/fabricengine/junction.go
index 9d6022f7..6a7fabab 100644
--- a/internal/fabricengine/junction.go
+++ b/internal/fabricengine/junction.go
@@ -22,9 +22,18 @@ import (
 // current worktree, keyed by slug.
 //
 // For each junction in l.HostJunctions(slug), WireJunctions:
+//   - Materialises the junction's weft-side target via os.MkdirAll
 //   - Creates the directory junction via fslink.CreateDirLink (idempotent via fslink.IsLink/PointsTo)
 //   - Appends the junction Name to the host worktree's .git/info/exclude (line-exact idempotent)
 //
+// Materialising the weft-side target is what lets every WireJunctions caller leave
+// a resolvable junction behind: initengine/init.go, fabricengine/checkout.go, and
+// fabricengine/reconcile.go all call WireJunctions, but of the three only Init
+// materialises the weft directory itself. Before this, fslink.CreateDirLink would
+// happily create a link to a nonexistent target (a raw reparse point on Windows, a
+// dangling symlink elsewhere), leaving checkout's and reconcile's junctions
+// dangling until some other path created the target.
+//
 // The two operations are sequenced such that if either fails, the junction may be
 // left partially wired; the caller is responsible for rollback if needed. The
 // operations themselves are individually idempotent (re-running is safe).
@@ -57,6 +66,16 @@ func WireJunctions(l *hubgeometry.Layout, slug string) error {
 // It iterates over the junctions returned by l.HostJunctions(slug), applying the same
 // create-or-verify-or-re-point logic per junction using each record's Link and Target.
 //
+// Before any check runs, each iteration materialises its junction's weft-side
+// target via os.MkdirAll — deliberately the first statement of the loop, ahead
+// of the os.Lstat(link) call below and both fslink.CreateDirLink call sites.
+// fslink.CreateDirLink happily creates a link to a nonexistent target (a raw
+// reparse point on Windows, a dangling symlink elsewhere), so a junction a prior
+// checkout or reconcile left dangling would otherwise hit the link-exists
+// branch's filepath.EvalSymlinks(target) failure below — the hard "weft
+// directory does not exist" error — on every subsequent WireJunctions call.
+// Materialising the target first gives that worktree a self-repair path instead.
+//
 // For each junction, if the path already exists:
 //   - A link resolving to the correct target is left alone (idempotent).
 //   - A link that dangles or resolves to the wrong target is re-pointed —
@@ -75,6 +94,12 @@ func seedLyxJunction(l *hubgeometry.Layout, slug string) error {
 		link := j.Link
 		target := j.Target
 
+		// Materialise the weft-side target first, before any of the checks
+		// below run — see the godoc above for why placement matters.
+		if err := os.MkdirAll(target, 0o755); err != nil {
+			return fmt.Errorf("materialise weft target %s: %w", target, err)
+		}
+
 		_, err := os.Lstat(link)
 		if err == nil {
 			// Link exists. Resolve the target first; if target doesn't exist, report distinctly.
@@ -109,11 +134,18 @@ func seedLyxJunction(l *hubgeometry.Layout, slug string) error {
 			}
 
 			// A real (non-link) directory predating weft; refuse to touch it —
-			// it may hold user content, which fabric never deletes.
+			// it may hold user content, which fabric never deletes. The remedy
+			// clause names what the operator can actually do, since it must
+			// serve both _lyx (this batch's baseline) and _pattern (a later
+			// batch's second junction) alike: PATTERN content is described
+			// throughout as the host repo's hand-authored invariants, which
+			// makes "create _pattern/ in the repo and start writing" the
+			// natural operator mistake this guard exists to catch.
 			return fmt.Errorf(
-				"host repo already contains a real %s at %s; it predates weft — migrate via the hub-creator",
+				"host repo already contains a real %s at %s; it predates weft — move its content into the paired weft worktree's own %s, or remove this directory, then re-run `lyx init` to create the junction",
 				filepath.Base(link),
 				link,
+				filepath.Base(link),
 			)
 		}
 
@@ -134,110 +166,141 @@ func seedLyxJunction(l *hubgeometry.Layout, slug string) error {
 // distinguishing a real reversal from a no-op on an already-clean (or
 // never-wired) worktree.
 type UnwireResult struct {
-	// JunctionRemoved reports whether the host _lyx junction was present and removed.
-	JunctionRemoved bool
+	// JunctionsRemoved lists the Name of each junction that was actually present
+	// and removed, in l.HostJunctions(slug) order. A name slice, not a count or
+	// a bool: which junction(s) were removed is CLI-observable, and "1 of 2
+	// removed" tells an operator nothing about which one is still wired.
+	JunctionsRemoved []string
 	// ExcludeChanged reports whether a junction-name line was removed from
 	// .git/info/exclude.
 	ExcludeChanged bool
 }
 
 // UnwireJunctions reverses WireJunctions for the current worktree, keyed by slug:
-// it removes the host _lyx junction and its .git/info/exclude entry, undoing
-// exactly what WireJunctions seeded — nothing more (the worktree pairing and weft
-// content are untouched; see Remove for the larger paired-teardown operation).
+// it removes every host junction in l.HostJunctions(slug) and their shared
+// .git/info/exclude entries, undoing exactly what WireJunctions seeded — nothing
+// more (the worktree pairing and weft content are untouched; see Remove for the
+// larger paired-teardown operation).
 //
-// The junction is unwired before the exclude entry, mirroring WireJunctions'
+// The junctions are unwired before the exclude entries, mirroring WireJunctions'
 // creation order in reverse. Per the "any junction inconsistency is a hard error"
 // invariant, if unseedLyxJunction reports an error the exclude file is never
 // touched: an unexpected junction state (a real directory, or a link pointing
 // somewhere unexpected) aborts the whole operation so a corrupted or
 // externally-modified junction is never silently worked around.
 //
-// Returns an empty UnwireResult and nil error when the junction was never wired
-// (the legitimate no-op case). Returns an error, with JunctionRemoved reflecting
-// what already happened, if the exclude-file update fails after a successful
-// junction removal.
+// Returns an empty UnwireResult and nil error when no junction was wired (the
+// legitimate no-op case). Returns an error, with JunctionsRemoved reflecting
+// whatever was already removed before the failure, if unseedLyxJunction aborts
+// partway through its loop, or if the exclude-file update fails after junction
+// removal completed. A zero UnwireResult on a mid-loop failure would misreport a
+// partial removal as untouched — with two or more junctions, the first may
+// already be gone before the second fails.
 func UnwireJunctions(l *hubgeometry.Layout, slug string) (UnwireResult, error) {
 	removed, err := unseedLyxJunction(l, slug)
 	if err != nil {
-		return UnwireResult{}, err
+		return UnwireResult{JunctionsRemoved: removed}, err
 	}
 
 	changed, err := unseedGitExclude(l, slug)
 	if err != nil {
-		return UnwireResult{JunctionRemoved: removed}, err
+		return UnwireResult{JunctionsRemoved: removed}, err
 	}
 
-	return UnwireResult{JunctionRemoved: removed, ExcludeChanged: changed}, nil
+	return UnwireResult{JunctionsRemoved: removed, ExcludeChanged: changed}, nil
 }
 
-// unseedLyxJunction removes the host _lyx junction for slug, mirroring
-// seedLyxJunction's validation in the same order (target resolution before the
-// link-type check) so the two functions stay in lockstep as the junction model
-// evolves.
+// unseedLyxJunction removes every host junction in l.HostJunctions(slug). It is
+// a thin wrapper over unseedJunctionRecords, which owns the actual per-junction
+// loop; the split exists purely so the loop's abort-and-accumulate contract is
+// directly testable against a synthetic junction slice, since l.HostJunctions
+// always returns exactly one entry today and cannot itself produce the
+// multi-junction scenario the contract is about.
 //
-// It is deliberately scoped to the single _lyx junction (HostLyxLink/WeftLyxDirFor)
-// rather than iterating l.HostJunctions(slug) the way unseedGitExclude does:
-// HostJunctions returns exactly one entry today, and UnwireResult.JunctionRemoved
-// is a single bool by design to match. If HostJunctions ever grows a second entry,
-// this function and UnwireResult should be revisited together.
+// Returns (nil, nil) if no junction exists — none were ever wired, or all were
+// already unwired; this is the legitimate no-op case, not an error. See
+// unseedJunctionRecords for the error cases.
+func unseedLyxJunction(l *hubgeometry.Layout, slug string) (removed []string, err error) {
+	return unseedJunctionRecords(l.HostJunctions(slug))
+}
+
+// unseedJunctionRecords removes each junction in junctions in order, mirroring
+// seedLyxJunction's per-junction validation in the same order (target
+// resolution before the link-type check) so the two functions stay in lockstep
+// as the junction model evolves.
 //
-// Returns (false, nil) if the junction does not exist — it was never wired, or was
-// already unwired; this is the legitimate no-op case, not an error. Returns an
-// error, without touching the link, if the weft-side target is missing or
-// unreachable, if the host path is a real directory rather than a junction, or if
-// the junction resolves to an unexpected target — all of these indicate corruption
-// or external modification rather than a normal unwire.
-func unseedLyxJunction(l *hubgeometry.Layout, slug string) (removed bool, err error) {
-	link := l.HostLyxLink(slug)
-
-	if _, err := os.Lstat(link); err != nil {
-		if os.IsNotExist(err) {
-			return false, nil
+// It aborts on the first junction error rather than continuing best-effort —
+// deliberately the opposite of removeHostJunction's rule in weftwiring.go, which
+// continues past a per-junction failure because its caller discards the return
+// value. Here, a junction inconsistency is a hard error the operator must see,
+// and UnwireJunctions gates the exclude-file update on this function succeeding;
+// continuing past a corrupted junction would silently work around exactly the
+// state this guard exists to surface.
+//
+// Returns (nil, nil) if junctions is empty or none of its entries exist on
+// disk — none were ever wired, or all were already unwired; this is the
+// legitimate no-op case, not an error. Returns (removed, err), where removed
+// holds every junction Name successfully removed before the failing one, if
+// the weft-side target for some junction is missing or unreachable, if that
+// junction's host path is a real directory rather than a junction, or if it
+// resolves to an unexpected target — all of these indicate corruption or
+// external modification rather than a normal unwire.
+func unseedJunctionRecords(junctions []hubgeometry.HostJunction) (removed []string, err error) {
+	for _, j := range junctions {
+		link := j.Link
+		target := j.Target
+
+		if _, err := os.Lstat(link); err != nil {
+			if os.IsNotExist(err) {
+				// This junction was never wired, or was already unwired; move on
+				// to the next one rather than treating it as an error.
+				continue
+			}
+			return removed, fmt.Errorf("lstat %s: %w", link, err)
 		}
-		return false, fmt.Errorf("lstat %s: %w", link, err)
-	}
 
-	// The link exists. Resolve the canonical weft-side target first, exactly as
-	// seedLyxJunction does, so a missing/unreachable target is reported distinctly
-	// from a wrong-target junction.
-	target := l.WeftLyxDirFor(slug)
-	targetResolved, errTarget := filepath.EvalSymlinks(target)
-	if errTarget != nil {
-		return false, fmt.Errorf("weft directory does not exist at %s; cannot validate junction target", target)
-	}
+		// The link exists. Resolve the canonical weft-side target first, exactly
+		// as seedLyxJunction does, so a missing/unreachable target is reported
+		// distinctly from a wrong-target junction.
+		targetResolved, errTarget := filepath.EvalSymlinks(target)
+		if errTarget != nil {
+			return removed, fmt.Errorf("weft directory does not exist at %s; cannot validate junction target", target)
+		}
 
-	isLink, err := fslink.IsLink(link)
-	if err != nil {
-		return false, fmt.Errorf("islink %s: %w", link, err)
-	}
-	if !isLink {
-		// A real directory predating weft (or otherwise not a junction); refuse to
-		// touch it rather than risk deleting user content.
-		return false, fmt.Errorf(
-			"host repo already contains a real %s at %s; it is not a junction — refusing to remove it",
-			filepath.Base(link),
-			link,
-		)
-	}
+		isLink, err := fslink.IsLink(link)
+		if err != nil {
+			return removed, fmt.Errorf("islink %s: %w", link, err)
+		}
+		if !isLink {
+			// A real directory predating weft (or otherwise not a junction); refuse to
+			// touch it rather than risk deleting user content.
+			return removed, fmt.Errorf(
+				"host repo already contains a real %s at %s; it is not a junction — refusing to remove it",
+				filepath.Base(link),
+				link,
+			)
+		}
 
-	linkResolved, err := fslink.PointsTo(link)
-	if err != nil {
-		return false, fmt.Errorf("resolve link target %s: %w", link, err)
-	}
-	if linkResolved != targetResolved {
-		// The junction points somewhere other than the expected weft directory —
-		// corruption or external modification, not a normal unwire target.
-		return false, fmt.Errorf(
-			"host junction %s points to unexpected target %s (want %s); refusing to remove it",
-			link, linkResolved, targetResolved,
-		)
-	}
+		linkResolved, err := fslink.PointsTo(link)
+		if err != nil {
+			return removed, fmt.Errorf("resolve link target %s: %w", link, err)
+		}
+		if linkResolved != targetResolved {
+			// The junction points somewhere other than the expected weft directory —
+			// corruption or external modification, not a normal unwire target.
+			return removed, fmt.Errorf(
+				"host junction %s points to unexpected target %s (want %s); refusing to remove it",
+				link, linkResolved, targetResolved,
+			)
+		}
 
-	if err := fslink.Remove(link); err != nil {
-		return false, fmt.Errorf("remove host junction %s: %w", link, err)
+		if err := fslink.Remove(link); err != nil {
+			return removed, fmt.Errorf("remove host junction %s: %w", link, err)
+		}
+		removed = append(removed, j.Name)
 	}
-	return true, nil
+
+	return removed, nil
 }
 
 // unseedGitExclude removes junction-name lines previously added by seedGitExclude
diff --git a/internal/fabricengine/junction_pattern_integration_test.go b/internal/fabricengine/junction_pattern_integration_test.go
new file mode 100644
index 00000000..9ffc5fbc
--- /dev/null
+++ b/internal/fabricengine/junction_pattern_integration_test.go
@@ -0,0 +1,610 @@
+//go:build integration
+
+// junction_pattern_integration_test.go covers the per-junction generalisation
+// this batch makes to seedLyxJunction, unseedLyxJunction, checkJunctionHealth,
+// and PairInSync's inline junction check. From card 15 onward, HostJunctions
+// returns two entries (_lyx and _pattern), so this file's cases now exercise
+// a genuinely two-junction world: every generalisation from batch 3 — health
+// check (per-site: reconcile, status, drift), per-junction refusal/repoint,
+// unwire/remove looping, and wiring idempotency (including the legacy
+// single-junction-to-two upgrade path) — is proven against a real second,
+// non-_lyx junction, not merely a loop of length one.
+//
+// Package fabricengine_test to reuse the external-test-package fixture idiom
+// of lifecycle_differential_test.go; shares the single TestMain in
+// testmain_test.go.
+
+package fabricengine_test
+
+import (
+	"fmt"
+	"os"
+	"path/filepath"
+	"slices"
+	"strings"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/fabricengine"
+	"github.com/Knatte18/loomyard/internal/fslink"
+	"github.com/Knatte18/loomyard/internal/gitexec"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
+	"github.com/Knatte18/loomyard/internal/lyxtest"
+)
+
+// readExcludeLines resolves and reads the host worktree's .git/info/exclude
+// file, mirroring the resolution logic seedGitExclude/unseedGitExclude use
+// (git rev-parse --git-path info/exclude, joined with the worktree path if
+// relative) so this test observes the same path the production code writes.
+func readExcludeLines(t *testing.T, l *hubgeometry.Layout, slug string) []string {
+	t.Helper()
+
+	worktreePath := l.WorktreePath(slug)
+	stdout, _, exitCode, err := gitexec.RunGit([]string{"rev-parse", "--git-path", "info/exclude"}, worktreePath)
+	if err != nil || exitCode != 0 {
+		t.Fatalf("git rev-parse --git-path info/exclude failed: %v (exit %d)", err, exitCode)
+	}
+
+	excludePath := strings.TrimSpace(stdout)
+	if !filepath.IsAbs(excludePath) {
+		excludePath = filepath.Join(worktreePath, excludePath)
+	}
+
+	content, err := os.ReadFile(excludePath)
+	if err != nil {
+		if os.IsNotExist(err) {
+			return nil
+		}
+		t.Fatalf("read exclude file: %v", err)
+	}
+	return strings.Split(string(content), "\n")
+}
+
+// TestWireJunctions_MaterialisesMissingWeftTarget is card 6's regression
+// guard: seedLyxJunction must create the weft-side target directory when it
+// is missing (the checkout/reconcile-left-dangling shape), leaving a junction
+// that resolves immediately, and a second WireJunctions call on the same
+// worktree must succeed rather than hard-erroring — the bug this card fixes.
+func TestWireJunctions_MaterialisesMissingWeftTarget(t *testing.T) {
+	t.Parallel()
+
+	fixture := lyxtest.CopyPairedLocal(t)
+	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
+		"fabric": fabricengine.ConfigTemplate(),
+	})
+
+	l := fixture.Layout
+	slug := filepath.Base(fixture.Hub)
+	target := l.WeftLyxDirFor(slug)
+
+	// The weft-prime template pre-seeds _lyx/config/placeholder; remove the
+	// whole target directory so it genuinely does not exist, matching the
+	// state a checkout/reconcile-created junction points at today.
+	if err := os.RemoveAll(target); err != nil {
+		t.Fatalf("remove weft target %s: %v", target, err)
+	}
+
+	if err := fabricengine.WireJunctions(l, slug); err != nil {
+		t.Fatalf("WireJunctions with missing weft target: %v", err)
+	}
+
+	if info, err := os.Stat(target); err != nil || !info.IsDir() {
+		t.Fatalf("weft target %s not materialised: stat err=%v", target, err)
+	}
+
+	link := l.HostLyxLink(slug)
+	isLink, err := fslink.IsLink(link)
+	if err != nil || !isLink {
+		t.Fatalf("junction at %s is not a link after WireJunctions: isLink=%v err=%v", link, isLink, err)
+	}
+	if _, err := fslink.PointsTo(link); err != nil {
+		t.Errorf("junction at %s does not resolve immediately after WireJunctions: %v", link, err)
+	}
+
+	// A second WireJunctions on the same worktree must succeed: this is the
+	// checkout/reconcile path that hard-errored before this card, because the
+	// link-exists branch could not resolve a still-missing target.
+	if err := fabricengine.WireJunctions(l, slug); err != nil {
+		t.Fatalf("second WireJunctions = %v; want nil (self-repair path)", err)
+	}
+}
+
+// TestWireJunctions_RefusesRealHostDirectory is card 7's regression guard: a
+// real, non-link directory sitting at the host junction path is still
+// refused — fabric never moves or deletes user content — and the returned
+// error names both the offending path and the re-run-`lyx init` remedy this
+// card's reworded message introduces, replacing the old "migrate via the
+// hub-creator" clause that pointed at a tool that does not address this case.
+func TestWireJunctions_RefusesRealHostDirectory(t *testing.T) {
+	t.Parallel()
+
+	fixture := lyxtest.CopyPairedLocal(t)
+	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
+		"fabric": fabricengine.ConfigTemplate(),
+	})
+
+	l := fixture.Layout
+	slug := filepath.Base(fixture.Hub)
+	link := l.HostLyxLink(slug)
+
+	// Seed a real, non-link directory at the host junction path — the
+	// "created _lyx by hand" mistake this card's message must guide an
+	// operator away from (and, per the batch scope, the same mistake an
+	// operator makes hand-authoring _pattern content).
+	if err := os.MkdirAll(link, 0o755); err != nil {
+		t.Fatalf("mkdir real host dir %s: %v", link, err)
+	}
+	marker := filepath.Join(link, "marker.txt")
+	if err := os.WriteFile(marker, []byte("real content"), 0o644); err != nil {
+		t.Fatalf("write marker file: %v", err)
+	}
+
+	err := fabricengine.WireJunctions(l, slug)
+	if err == nil {
+		t.Fatal("WireJunctions = nil; want error refusing a real host directory")
+	}
+
+	msg := err.Error()
+	if !strings.Contains(msg, link) {
+		t.Errorf("error %q does not name the offending path %q", msg, link)
+	}
+	if !strings.Contains(msg, "lyx init") {
+		t.Errorf("error %q does not name the re-run-`lyx init` remedy", msg)
+	}
+
+	// The real directory and its content must be untouched: fabric never
+	// deletes or moves user content on this guard's account.
+	content, readErr := os.ReadFile(marker)
+	if readErr != nil {
+		t.Fatalf("read marker after refused WireJunctions: %v", readErr)
+	}
+	if string(content) != "real content" {
+		t.Errorf("marker content changed: %q", string(content))
+	}
+}
+
+// TestUnwireJunctions_ReportsAndClearsEveryJunction is card 8's base-case
+// regression guard: wiring then unwiring reports every junction Name in
+// UnwireResult.JunctionsRemoved and removes every corresponding line from
+// .git/info/exclude. From card 15 onward HostJunctions returns two entries
+// (_lyx and _pattern), so this now runs against a genuinely two-junction
+// world — the precondition batch 5's second junction depends on this
+// machinery already handling correctly.
+func TestUnwireJunctions_ReportsAndClearsEveryJunction(t *testing.T) {
+	t.Parallel()
+
+	fixture := lyxtest.CopyPairedLocal(t)
+	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
+		"fabric": fabricengine.ConfigTemplate(),
+	})
+
+	l := fixture.Layout
+	slug := filepath.Base(fixture.Hub)
+
+	if err := fabricengine.WireJunctions(l, slug); err != nil {
+		t.Fatalf("WireJunctions: %v", err)
+	}
+	if lines := readExcludeLines(t, l, slug); !containsLine(lines, hubgeometry.LyxDirName) {
+		t.Fatalf(".git/info/exclude does not contain %q after WireJunctions: %v", hubgeometry.LyxDirName, lines)
+	}
+	if lines := readExcludeLines(t, l, slug); !containsLine(lines, hubgeometry.PatternDirName) {
+		t.Fatalf(".git/info/exclude does not contain %q after WireJunctions: %v", hubgeometry.PatternDirName, lines)
+	}
+
+	result, err := fabricengine.UnwireJunctions(l, slug)
+	if err != nil {
+		t.Fatalf("UnwireJunctions: %v", err)
+	}
+
+	if want := []string{hubgeometry.LyxDirName, hubgeometry.PatternDirName}; !slices.Equal(result.JunctionsRemoved, want) {
+		t.Errorf("JunctionsRemoved = %v; want %v", result.JunctionsRemoved, want)
+	}
+	if !result.ExcludeChanged {
+		t.Error("ExcludeChanged = false; want true")
+	}
+
+	lyxLink := l.HostLyxLink(slug)
+	if _, statErr := os.Lstat(lyxLink); !os.IsNotExist(statErr) {
+		t.Errorf("junction %s still exists after UnwireJunctions", lyxLink)
+	}
+	patternLink := l.HostPatternLink(slug)
+	if _, statErr := os.Lstat(patternLink); !os.IsNotExist(statErr) {
+		t.Errorf("junction %s still exists after UnwireJunctions", patternLink)
+	}
+	if lines := readExcludeLines(t, l, slug); containsLine(lines, hubgeometry.LyxDirName) {
+		t.Errorf(".git/info/exclude still contains %q after UnwireJunctions: %v", hubgeometry.LyxDirName, lines)
+	}
+	if lines := readExcludeLines(t, l, slug); containsLine(lines, hubgeometry.PatternDirName) {
+		t.Errorf(".git/info/exclude still contains %q after UnwireJunctions: %v", hubgeometry.PatternDirName, lines)
+	}
+}
+
+// TestUnwireJunctions_AlreadyUnwiredIsNoOp asserts that unwiring a worktree
+// whose junctions were never wired (or already unwired) is a legitimate
+// no-op: an empty JunctionsRemoved and a nil error, never an error.
+func TestUnwireJunctions_AlreadyUnwiredIsNoOp(t *testing.T) {
+	t.Parallel()
+
+	fixture := lyxtest.CopyPairedLocal(t)
+	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
+		"fabric": fabricengine.ConfigTemplate(),
+	})
+
+	l := fixture.Layout
+	slug := filepath.Base(fixture.Hub)
+
+	result, err := fabricengine.UnwireJunctions(l, slug)
+	if err != nil {
+		t.Fatalf("UnwireJunctions on never-wired worktree = %v; want nil", err)
+	}
+	if len(result.JunctionsRemoved) != 0 {
+		t.Errorf("JunctionsRemoved = %v; want empty", result.JunctionsRemoved)
+	}
+	if result.ExcludeChanged {
+		t.Error("ExcludeChanged = true; want false")
+	}
+}
+
+// containsLine reports whether lines contains name as a trimmed, line-exact
+// match, mirroring the comparison seedGitExclude/unseedGitExclude use.
+func containsLine(lines []string, name string) bool {
+	for _, line := range lines {
+		if strings.TrimSpace(line) == name {
+			return true
+		}
+	}
+	return false
+}
+
+// TestDetectHostPollution_PatternTrackedAsRestorable is card 18's regression
+// guard: a tracked path under _pattern in the host index must be reported as
+// pollution with the same automated restore remedy _lyx pollution gets (git
+// rm --cached plus a reminder to restore the junction/exclude entry) — never
+// report-only like _raddle, since _pattern has a junction from card 15
+// onward.
+func TestDetectHostPollution_PatternTrackedAsRestorable(t *testing.T) {
+	t.Parallel()
+
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+
+	// Track a file under _pattern directly in the host worktree's index —
+	// the "hand-authored _pattern content accidentally committed to host"
+	// mistake this scan exists to catch.
+	hostPatternDir := filepath.Join(l.WorktreeRoot, hubgeometry.PatternDirName)
+	if err := os.MkdirAll(hostPatternDir, 0o755); err != nil {
+		t.Fatalf("mkdir host _pattern dir: %v", err)
+	}
+	trackedFile := filepath.Join(hostPatternDir, "PATTERN.md")
+	if err := os.WriteFile(trackedFile, []byte("# constraints\n"), 0o644); err != nil {
+		t.Fatalf("write tracked file: %v", err)
+	}
+	lyxtest.MustRun(t, l.WorktreeRoot, "git", "add", "--", hubgeometry.PatternDirName)
+	lyxtest.MustRun(t, l.WorktreeRoot, "git", "commit", "-m", "accidentally track _pattern")
+
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	result, err := topology.Status(l)
+	if err != nil {
+		t.Fatalf("Status: %v", err)
+	}
+	if len(result.Pairs) == 0 {
+		t.Fatal("Status returned no pairs")
+	}
+
+	const wantPath = "_pattern/PATTERN.md"
+	var found *fabricengine.PollutionEntry
+	for i, entry := range result.Pairs[0].Pollution {
+		if entry.Path == wantPath {
+			found = &result.Pairs[0].Pollution[i]
+			break
+		}
+	}
+	if found == nil {
+		t.Fatalf("no pollution entry for %q found in %+v", wantPath, result.Pairs[0].Pollution)
+	}
+	if found.ReportOnly {
+		t.Errorf("PollutionEntry for %q is ReportOnly; want a restorable (automated-remedy) entry, matching _lyx", wantPath)
+	}
+	if found.Remedy == "" {
+		t.Errorf("PollutionEntry for %q has empty Remedy; want the same git rm --cached remedy _lyx gets", wantPath)
+	}
+	if !strings.Contains(found.Remedy, "rm --cached") {
+		t.Errorf("PollutionEntry for %q remedy = %q; want it to contain \"rm --cached\"", wantPath, found.Remedy)
+	}
+}
+
+// TestPairInSync_JunctionDriftShapes is card 11's regression guard: each of
+// PairInSync's three junction-drift shapes — missing, not-a-link, and
+// points-elsewhere — produces reason wording naming the junction, aligned
+// with checkJunctionHealth's wording (card 10) for the same shape. From card
+// 15 onward, each drift shape is exercised against BOTH junctions (_lyx and
+// _pattern), proving PairInSync's HostJunctionsHere() loop — drift.go does
+// not share checkJunctionHealth's code path, so this is not redundant with
+// reconcile.go's or status.go's own coverage — reports the correct wording
+// for the second, non-_lyx junction too.
+func TestPairInSync_JunctionDriftShapes(t *testing.T) {
+	shapes := []struct {
+		name       string
+		corrupt    func(t *testing.T, link, target string)
+		wantReason func(dirName string) string
+	}{
+		{
+			name: "Missing",
+			corrupt: func(t *testing.T, link, target string) {
+				if err := fslink.Remove(link); err != nil {
+					t.Fatalf("remove junction: %v", err)
+				}
+			},
+			wantReason: func(dirName string) string {
+				return fmt.Sprintf("host %s junction missing", dirName)
+			},
+		},
+		{
+			name: "NotALink",
+			corrupt: func(t *testing.T, link, target string) {
+				if err := fslink.Remove(link); err != nil {
+					t.Fatalf("remove junction: %v", err)
+				}
+				if err := os.Mkdir(link, 0o755); err != nil {
+					t.Fatalf("mkdir real dir in junction's place: %v", err)
+				}
+			},
+			wantReason: func(dirName string) string {
+				return fmt.Sprintf("host %s is not a junction", dirName)
+			},
+		},
+		{
+			name: "PointsElsewhere",
+			corrupt: func(t *testing.T, link, target string) {
+				if err := fslink.Remove(link); err != nil {
+					t.Fatalf("remove junction: %v", err)
+				}
+				wrongTarget := filepath.Join(filepath.Dir(target), "not-the-weft-target")
+				if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
+					t.Fatalf("mkdir wrong target: %v", err)
+				}
+				if err := fslink.CreateDirLink(link, wrongTarget); err != nil {
+					t.Fatalf("seed wrong-target junction: %v", err)
+				}
+			},
+			wantReason: func(dirName string) string {
+				return fmt.Sprintf("host %s junction points elsewhere", dirName)
+			},
+		},
+	}
+
+	junctions := []struct {
+		name      string
+		dirName   string
+		linkFor   func(l *hubgeometry.Layout) string
+		targetFor func(l *hubgeometry.Layout) string
+	}{
+		{
+			name:      "Lyx",
+			dirName:   hubgeometry.LyxDirName,
+			linkFor:   func(l *hubgeometry.Layout) string { return l.HostLyxLinkHere() },
+			targetFor: func(l *hubgeometry.Layout) string { return l.WeftLyxDir() },
+		},
+		{
+			name:      "Pattern",
+			dirName:   hubgeometry.PatternDirName,
+			linkFor:   func(l *hubgeometry.Layout) string { return l.HostPatternLinkHere() },
+			targetFor: func(l *hubgeometry.Layout) string { return l.WeftPatternDir() },
+		},
+	}
+
+	for _, j := range junctions {
+		for _, tt := range shapes {
+			t.Run(j.name+"_"+tt.name, func(t *testing.T) {
+				t.Parallel()
+
+				fixture := lyxtest.CopyPairedLocal(t)
+				lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
+					"fabric": fabricengine.ConfigTemplate(),
+				})
+				lyxtest.MustRun(t, fixture.WeftPrime, "git", "checkout", "-b", fabricengine.WeftBranchName("main"))
+
+				l := fixture.Layout
+				slug := filepath.Base(fixture.Hub)
+				if err := fabricengine.WireJunctions(l, slug); err != nil {
+					t.Fatalf("WireJunctions: %v", err)
+				}
+
+				link := j.linkFor(l)
+				target := j.targetFor(l)
+				tt.corrupt(t, link, target)
+
+				ok, reason, err := fabricengine.PairInSync(l)
+				if err != nil {
+					t.Fatalf("PairInSync: %v", err)
+				}
+				if ok {
+					t.Errorf("PairInSync = true; want false (%s)", tt.name)
+				}
+				wantReason := tt.wantReason(j.dirName)
+				if reason != wantReason {
+					t.Errorf("PairInSync reason = %q; want %q", reason, wantReason)
+				}
+			})
+		}
+	}
+}
+
+// TestReconcile_RepairsPatternOnlyDrift is the reconcile.go site of card 19's
+// per-site health-check coverage: with _lyx healthy and _pattern missing,
+// Reconcile must repair (ReconcileActionJunctionRepointed) rather than report
+// ReconcileActionAlreadyHealthy. reconcile.go does not share drift.go's
+// PairInSync code path (see checkJunctionHealth), so this is not redundant
+// with TestPairInSync_JunctionDriftShapes above.
+func TestReconcile_RepairsPatternOnlyDrift(t *testing.T) {
+	t.Parallel()
+
+	const slug = "reconcile-pattern-only-drift"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
+		t.Fatalf("setup Add: %v", err)
+	}
+	if err := fabricengine.WireJunctions(l, slug); err != nil {
+		t.Fatalf("WireJunctions: %v", err)
+	}
+
+	hostLayout, err := hubgeometry.Resolve(l.WorktreePath(slug))
+	if err != nil {
+		t.Fatalf("hubgeometry.Resolve(host): %v", err)
+	}
+
+	// _lyx stays healthy; only _pattern goes missing.
+	patternLink := hostLayout.HostPatternLinkHere()
+	if err := fslink.Remove(patternLink); err != nil {
+		t.Fatalf("remove _pattern junction: %v", err)
+	}
+
+	result, err := topology.Reconcile(l)
+	if err != nil {
+		t.Fatalf("Reconcile: %v", err)
+	}
+
+	weftPath := l.WeftWorktreePath(slug)
+	var found bool
+	for _, pair := range result.Pairs {
+		if pair.WeftWorktree != filepath.ToSlash(weftPath) {
+			continue
+		}
+		found = true
+		if pair.Action != fabricengine.ReconcileActionJunctionRepointed {
+			t.Errorf("Action = %q; want %q (not %q)", pair.Action, fabricengine.ReconcileActionJunctionRepointed, fabricengine.ReconcileActionAlreadyHealthy)
+		}
+		if pair.Error != "" {
+			t.Errorf("Error = %q; want empty", pair.Error)
+		}
+	}
+	if !found {
+		t.Fatalf("Reconcile result has no pair for weft path %s: %+v", weftPath, result.Pairs)
+	}
+
+	if isLink, err := fslink.IsLink(patternLink); err != nil || !isLink {
+		t.Errorf("_pattern junction %s not repaired by Reconcile: isLink=%v err=%v", patternLink, isLink, err)
+	}
+}
+
+// TestStatus_ReportsPatternJunctionUnhealthy is the status.go site of card
+// 19's per-site health-check coverage: with _lyx healthy and _pattern
+// mis-pointed, Status must report JunctionHealthy false, a JunctionReason
+// naming _pattern, and the pair not in sync. status.go computes its verdict
+// inline (see status.go's own doc comment) rather than sharing drift.go's
+// PairInSync, so this is not redundant with TestPairInSync_JunctionDriftShapes.
+func TestStatus_ReportsPatternJunctionUnhealthy(t *testing.T) {
+	t.Parallel()
+
+	const slug = "status-pattern-unhealthy"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
+		t.Fatalf("setup Add: %v", err)
+	}
+	if err := fabricengine.WireJunctions(l, slug); err != nil {
+		t.Fatalf("WireJunctions: %v", err)
+	}
+
+	hostLayout, err := hubgeometry.Resolve(l.WorktreePath(slug))
+	if err != nil {
+		t.Fatalf("hubgeometry.Resolve(host): %v", err)
+	}
+
+	// _lyx stays healthy; only _pattern is re-pointed at an unrelated directory.
+	patternLink := hostLayout.HostPatternLinkHere()
+	if err := fslink.Remove(patternLink); err != nil {
+		t.Fatalf("remove _pattern junction: %v", err)
+	}
+	wrongTarget := filepath.Join(fixture.Hub, "not-the-weft-pattern-dir")
+	if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
+		t.Fatalf("mkdir wrong target: %v", err)
+	}
+	if err := fslink.CreateDirLink(patternLink, wrongTarget); err != nil {
+		t.Fatalf("seed wrong-target junction: %v", err)
+	}
+
+	result, err := topology.Status(l)
+	if err != nil {
+		t.Fatalf("Status: %v", err)
+	}
+
+	hostPath := l.WorktreePath(slug)
+	var found bool
+	for _, pair := range result.Pairs {
+		if pair.HostWorktree != filepath.ToSlash(hostPath) {
+			continue
+		}
+		found = true
+		if pair.JunctionHealthy {
+			t.Error("JunctionHealthy = true; want false")
+		}
+		if !strings.Contains(pair.JunctionReason, hubgeometry.PatternDirName) {
+			t.Errorf("JunctionReason = %q; want it to name %q", pair.JunctionReason, hubgeometry.PatternDirName)
+		}
+		if pair.InSync {
+			t.Error("InSync = true; want false")
+		}
+	}
+	if !found {
+		t.Fatalf("Status result has no pair for host path %s: %+v", hostPath, result.Pairs)
+	}
+}
+
+// TestWireJunctions_UpgradesLyxOnlyWorktreeToBoth is the wiring-idempotency
+// upgrade path: a worktree with _lyx already wired and _pattern not yet wired
+// at all (the shape every pre-card-15 worktree is in) completes a
+// WireJunctions call without error and adds only the missing _pattern
+// junction — it must not error, and it must not need to touch the
+// already-healthy _lyx junction to succeed.
+func TestWireJunctions_UpgradesLyxOnlyWorktreeToBoth(t *testing.T) {
+	t.Parallel()
+
+	fixture := lyxtest.CopyPairedLocal(t)
+	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
+		"fabric": fabricengine.ConfigTemplate(),
+	})
+
+	l := fixture.Layout
+	slug := filepath.Base(fixture.Hub)
+
+	// Wire both junctions once, then simulate the legacy pre-upgrade state by
+	// removing _pattern only — _lyx is fully healthy, _pattern was never wired.
+	if err := fabricengine.WireJunctions(l, slug); err != nil {
+		t.Fatalf("WireJunctions (initial): %v", err)
+	}
+	patternLink := l.HostPatternLink(slug)
+	if err := fslink.Remove(patternLink); err != nil {
+		t.Fatalf("remove _pattern junction to simulate legacy worktree: %v", err)
+	}
+
+	lyxLink := l.HostLyxLink(slug)
+	lyxResolvedBefore, err := fslink.PointsTo(lyxLink)
+	if err != nil {
+		t.Fatalf("PointsTo(%s) before upgrade: %v", lyxLink, err)
+	}
+
+	// The upgrade call: must complete without error and wire only the missing
+	// _pattern junction.
+	if err := fabricengine.WireJunctions(l, slug); err != nil {
+		t.Fatalf("WireJunctions (upgrade): %v", err)
+	}
+
+	if isLink, err := fslink.IsLink(patternLink); err != nil || !isLink {
+		t.Fatalf("_pattern junction %s not wired by upgrade call: isLink=%v err=%v", patternLink, isLink, err)
+	}
+	lyxResolvedAfter, err := fslink.PointsTo(lyxLink)
+	if err != nil {
+		t.Fatalf("PointsTo(%s) after upgrade: %v", lyxLink, err)
+	}
+	if lyxResolvedBefore != lyxResolvedAfter {
+		t.Errorf("_lyx junction target changed across upgrade call: before=%s after=%s", lyxResolvedBefore, lyxResolvedAfter)
+	}
+
+	// Wiring twice (the already-upgraded worktree) is a no-op.
+	if err := fabricengine.WireJunctions(l, slug); err != nil {
+		t.Fatalf("WireJunctions (idempotent re-run): %v", err)
+	}
+}
diff --git a/internal/fabricengine/junction_repoint_test.go b/internal/fabricengine/junction_repoint_test.go
index fce8371d..8b5a3c73 100644
--- a/internal/fabricengine/junction_repoint_test.go
+++ b/internal/fabricengine/junction_repoint_test.go
@@ -12,6 +12,12 @@
 // seedLyxJunction has the same unfixed gap, so a differential harness would
 // diverge here.
 //
+// From card 15 onward, seedLyxJunction's per-junction repair loop runs over
+// two junctions (_lyx and _pattern), so this file also carries the _pattern
+// counterparts of both repoint cases: the per-junction refusal/repair
+// behaviour must hold identically for the second, non-_lyx junction, which
+// has no _lyx-shaped shortcut to fall back on.
+//
 // Package fabricengine_test to reuse the external-test-package fixture idiom
 // of lifecycle_differential_test.go; shares the single TestMain in
 // testmain_test.go.
@@ -78,6 +84,57 @@ func TestWireJunctions_RepointsWrongTargetJunction(t *testing.T) {
 	}
 }
 
+// TestWireJunctions_RepointsWrongTargetJunction_Pattern is the _pattern
+// counterpart of TestWireJunctions_RepointsWrongTargetJunction: the host
+// _pattern junction, not _lyx, is pointed at an unrelated real directory, and
+// WireJunctions must re-point it at the correct weft _pattern target — the
+// same per-junction repair behaviour, exercised against the second junction.
+func TestWireJunctions_RepointsWrongTargetJunction_Pattern(t *testing.T) {
+	t.Parallel()
+
+	fixture := lyxtest.CopyPairedLocal(t)
+	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
+		"fabric": fabricengine.ConfigTemplate(),
+	})
+
+	l := fixture.Layout
+	slug := filepath.Base(fixture.Hub)
+	link := l.HostPatternLink(slug)
+	correctTarget := l.WeftPatternDirFor(slug)
+
+	// Point the junction at an unrelated real directory instead.
+	wrongTarget := filepath.Join(t.TempDir(), "not-the-weft-pattern-dir")
+	if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
+		t.Fatalf("mkdir wrong target: %v", err)
+	}
+	if err := os.RemoveAll(link); err != nil {
+		t.Fatalf("remove existing junction: %v", err)
+	}
+	if err := fslink.CreateDirLink(link, wrongTarget); err != nil {
+		t.Fatalf("seed wrong-target junction: %v", err)
+	}
+
+	if err := fabricengine.WireJunctions(l, slug); err != nil {
+		t.Fatalf("WireJunctions: %v", err)
+	}
+
+	isLink, err := fslink.IsLink(link)
+	if err != nil || !isLink {
+		t.Fatalf("junction at %s is not a link after WireJunctions: isLink=%v err=%v", link, isLink, err)
+	}
+	resolved, err := fslink.PointsTo(link)
+	if err != nil {
+		t.Fatalf("PointsTo(%s): %v", link, err)
+	}
+	wantResolved, err := filepath.EvalSymlinks(correctTarget)
+	if err != nil {
+		t.Fatalf("EvalSymlinks(%s): %v", correctTarget, err)
+	}
+	if resolved != wantResolved {
+		t.Errorf("junction resolves to %s; want %s", resolved, wantResolved)
+	}
+}
+
 // TestWireJunctions_RepointsDanglingJunction points the host _lyx junction at
 // a target that does not exist, then asserts WireJunctions removes and
 // recreates it at the correct target instead of refusing it.
@@ -122,3 +179,51 @@ func TestWireJunctions_RepointsDanglingJunction(t *testing.T) {
 		t.Errorf("junction resolves to %s; want %s", resolved, wantResolved)
 	}
 }
+
+// TestWireJunctions_RepointsDanglingJunction_Pattern is the _pattern
+// counterpart of TestWireJunctions_RepointsDanglingJunction: the host
+// _pattern junction, not _lyx, dangles (points at a nonexistent target), and
+// WireJunctions must re-point it at the correct weft _pattern target rather
+// than refusing it — the same per-junction repair behaviour, exercised
+// against the second junction.
+func TestWireJunctions_RepointsDanglingJunction_Pattern(t *testing.T) {
+	t.Parallel()
+
+	fixture := lyxtest.CopyPairedLocal(t)
+	lyxtest.SeedConfig(t, fixture.WeftPrime, map[string]string{
+		"fabric": fabricengine.ConfigTemplate(),
+	})
+
+	l := fixture.Layout
+	slug := filepath.Base(fixture.Hub)
+	link := l.HostPatternLink(slug)
+	correctTarget := l.WeftPatternDirFor(slug)
+
+	danglingTarget := filepath.Join(t.TempDir(), "does-not-exist-pattern")
+	if err := os.RemoveAll(link); err != nil {
+		t.Fatalf("remove existing junction: %v", err)
+	}
+	if err := fslink.CreateDirLink(link, danglingTarget); err != nil {
+		t.Fatalf("seed dangling junction: %v", err)
+	}
+
+	if err := fabricengine.WireJunctions(l, slug); err != nil {
+		t.Fatalf("WireJunctions: %v", err)
+	}
+
+	isLink, err := fslink.IsLink(link)
+	if err != nil || !isLink {
+		t.Fatalf("junction at %s is not a link after WireJunctions: isLink=%v err=%v", link, isLink, err)
+	}
+	resolved, err := fslink.PointsTo(link)
+	if err != nil {
+		t.Fatalf("PointsTo(%s): %v", link, err)
+	}
+	wantResolved, err := filepath.EvalSymlinks(correctTarget)
+	if err != nil {
+		t.Fatalf("EvalSymlinks(%s): %v", correctTarget, err)
+	}
+	if resolved != wantResolved {
+		t.Errorf("junction resolves to %s; want %s", resolved, wantResolved)
+	}
+}
diff --git a/internal/fabricengine/junction_test.go b/internal/fabricengine/junction_test.go
new file mode 100644
index 00000000..af514538
--- /dev/null
+++ b/internal/fabricengine/junction_test.go
@@ -0,0 +1,129 @@
+// junction_test.go unit-tests unseedJunctionRecords directly against synthetic
+// hubgeometry.HostJunction slices — no build tag, since it touches only plain
+// directories and fslink, never git. It exists because l.HostJunctions(slug)
+// still returns exactly one entry in this batch (a second entry is batch 5's
+// job), so the abort-and-accumulate contract this card gives unseedLyxJunction
+// cannot be driven through the exported (l, slug) surface with more than one
+// junction; this file drives the extracted loop directly instead.
+
+package fabricengine
+
+import (
+	"os"
+	"path/filepath"
+	"slices"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/fslink"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
+)
+
+// wireTestJunction creates a real link at link pointing to target (creating
+// target first, since fslink.CreateDirLink does not require it to pre-exist
+// but this helper wants a healthy, resolvable junction for the "removable"
+// cases below).
+func wireTestJunction(t *testing.T, link, target string) {
+	t.Helper()
+
+	if err := os.MkdirAll(target, 0o755); err != nil {
+		t.Fatalf("mkdir target %s: %v", target, err)
+	}
+	if err := fslink.CreateDirLink(link, target); err != nil {
+		t.Fatalf("CreateDirLink(%s, %s): %v", link, target, err)
+	}
+}
+
+// TestUnseedJunctionRecords_AccumulatesBeforeAbort is card 8's regression
+// guard for the bug it fixes: when a later junction in the slice fails to
+// unwire after an earlier one succeeded, the returned removed slice names the
+// earlier one and is not the zero value. This is only exercisable against a
+// synthetic multi-record slice — see the file doc comment for why.
+func TestUnseedJunctionRecords_AccumulatesBeforeAbort(t *testing.T) {
+	t.Parallel()
+
+	root := t.TempDir()
+
+	firstLink := filepath.Join(root, "first-link")
+	firstTarget := filepath.Join(root, "first-target")
+	wireTestJunction(t, firstLink, firstTarget)
+
+	// The second junction's host path is a real, non-link directory —
+	// exactly the "not a junction" refusal unseedJunctionRecords must abort
+	// on, without having touched anything past the first junction.
+	secondLink := filepath.Join(root, "second-link")
+	if err := os.MkdirAll(secondLink, 0o755); err != nil {
+		t.Fatalf("mkdir real second-link dir: %v", err)
+	}
+
+	junctions := []hubgeometry.HostJunction{
+		{Name: "first", Link: firstLink, Target: firstTarget},
+		{Name: "second", Link: secondLink, Target: filepath.Join(root, "second-target")},
+	}
+
+	removed, err := unseedJunctionRecords(junctions)
+	if err == nil {
+		t.Fatal("unseedJunctionRecords = nil error; want error from the second (real-directory) junction")
+	}
+	if want := []string{"first"}; !slices.Equal(removed, want) {
+		t.Errorf("removed = %v; want %v (the earlier junction, accumulated before the abort)", removed, want)
+	}
+
+	// The first junction really is gone; the second (never a junction) is untouched.
+	if _, statErr := os.Lstat(firstLink); !os.IsNotExist(statErr) {
+		t.Errorf("first junction %s still exists after removal", firstLink)
+	}
+	if info, statErr := os.Stat(secondLink); statErr != nil || !info.IsDir() {
+		t.Errorf("second host dir %s not left untouched: stat err=%v", secondLink, statErr)
+	}
+}
+
+// TestUnseedJunctionRecords_EmptyIsNoOp asserts that an empty junctions slice
+// (matching l.HostJunctions(slug) before any junction has ever been wired) is
+// a legitimate no-op: (nil, nil), not an error.
+func TestUnseedJunctionRecords_EmptyIsNoOp(t *testing.T) {
+	t.Parallel()
+
+	removed, err := unseedJunctionRecords(nil)
+	if err != nil {
+		t.Fatalf("unseedJunctionRecords(nil) = %v; want nil", err)
+	}
+	if len(removed) != 0 {
+		t.Errorf("removed = %v; want empty", removed)
+	}
+}
+
+// TestUnseedJunctionRecords_RemovesEveryHealthyJunction asserts the base case
+// this card's generalisation must not regress: every junction in the slice
+// that is present and healthy is removed, and every removed Name is reported.
+func TestUnseedJunctionRecords_RemovesEveryHealthyJunction(t *testing.T) {
+	t.Parallel()
+
+	root := t.TempDir()
+
+	firstLink := filepath.Join(root, "first-link")
+	firstTarget := filepath.Join(root, "first-target")
+	wireTestJunction(t, firstLink, firstTarget)
+
+	secondLink := filepath.Join(root, "second-link")
+	secondTarget := filepath.Join(root, "second-target")
+	wireTestJunction(t, secondLink, secondTarget)
+
+	junctions := []hubgeometry.HostJunction{
+		{Name: "first", Link: firstLink, Target: firstTarget},
+		{Name: "second", Link: secondLink, Target: secondTarget},
+	}
+
+	removed, err := unseedJunctionRecords(junctions)
+	if err != nil {
+		t.Fatalf("unseedJunctionRecords = %v; want nil", err)
+	}
+	if want := []string{"first", "second"}; !slices.Equal(removed, want) {
+		t.Errorf("removed = %v; want %v", removed, want)
+	}
+
+	for _, link := range []string{firstLink, secondLink} {
+		if _, statErr := os.Lstat(link); !os.IsNotExist(statErr) {
+			t.Errorf("junction %s still exists after removal", link)
+		}
+	}
+}
diff --git a/internal/fabricengine/reconcile.go b/internal/fabricengine/reconcile.go
index a9fe876d..dcd144a3 100644
--- a/internal/fabricengine/reconcile.go
+++ b/internal/fabricengine/reconcile.go
@@ -31,8 +31,11 @@ const (
 	// its existing branch.
 	ReconcileActionWeftRecreated ReconcileAction = "weft_recreated"
 
-	// ReconcileActionJunctionRepointed means a broken or dangling host _lyx junction
-	// was re-pointed to the correct weft _lyx directory.
+	// ReconcileActionJunctionRepointed means at least one broken or dangling host
+	// junction was re-pointed to its correct weft directory. WireJunctions repairs
+	// every junction in one call, so the outcome's Detail (via
+	// junctionRepointedDetail) names all of them, not just the one that failed
+	// checkJunctionHealth.
 	ReconcileActionJunctionRepointed ReconcileAction = "junction_repointed"
 
 	// ReconcileActionRawAdopted means a host worktree created outside lyx had its weft
@@ -142,20 +145,19 @@ func (t *Topology) Reconcile(l *hubgeometry.Layout) (ReconcileResult, error) {
 			pairedAction := t.reconcileMissingWeft(hostLayout, hostPath, weftPath, slug, hostBranch, &pr)
 			pr.Action = pairedAction
 		} else {
-			// The weft worktree exists. Check whether the junction is healthy; if not, re-point it.
-			hostLink := hostLayout.HostLyxLinkHere()
-			weftLyxDir := hostLayout.WeftLyxDir()
-			junctionHealthy, _ := checkJunctionHealth(hostLink, weftLyxDir)
+			// The weft worktree exists. Check whether every junction is healthy; if not, re-point them.
+			junctionHealthy, _ := checkJunctionHealth(hostLayout)
 
 			if !junctionHealthy {
-				// Re-point the junction by running WireJunctions. WireJunctions is idempotent
-				// and handles both the missing-junction and the wrong-target cases.
+				// Re-point the junction(s) by running WireJunctions. WireJunctions is idempotent
+				// and handles both the missing-junction and the wrong-target cases, for every
+				// junction in one call — not just the one checkJunctionHealth found unhealthy.
 				if wireErr := WireJunctions(hostLayout, slug); wireErr != nil {
 					pr.Error = fmt.Sprintf("re-point junction: %v", wireErr)
 					pr.Action = ReconcileActionJunctionRepointed
 				} else {
 					pr.Action = ReconcileActionJunctionRepointed
-					pr.Detail = fmt.Sprintf("junction re-pointed: %s → %s", hostLink, weftLyxDir)
+					pr.Detail = junctionRepointedDetail(hostLayout)
 				}
 			} else {
 				pr.Action = ReconcileActionAlreadyHealthy
@@ -307,39 +309,63 @@ func readBranch(dir string) (string, error) {
 	return strings.TrimSpace(out), nil
 }
 
-// checkJunctionHealth verifies that hostLink is a junction/symlink pointing to weftLyxDir.
+// checkJunctionHealth verifies that every junction in hostLayout.HostJunctionsHere()
+// is a link resolving to its own Target, reporting the first unhealthy one found
+// (first-unhealthy-wins).
 //
-// Returns (ok, reason) where ok is true only if the junction is correctly configured.
-func checkJunctionHealth(hostLink, weftLyxDir string) (bool, string) {
-	// Check whether the host link exists at all.
-	_, err := os.Lstat(hostLink)
-	if err != nil {
-		if os.IsNotExist(err) {
-			return false, "host _lyx junction missing"
+// A junction is unhealthy if its Link is missing, is not a link, or resolves
+// somewhere other than its Target. Every reason string names the junction (by
+// Name) it describes, since with more than one junction a bare "junction
+// missing" no longer tells an operator which one is broken.
+//
+// Returns (ok, reason) where ok is true only if every junction is correctly
+// configured; reason is empty in that case.
+func checkJunctionHealth(hostLayout *hubgeometry.Layout) (bool, string) {
+	for _, j := range hostLayout.HostJunctionsHere() {
+		// Check whether the host link exists at all.
+		_, err := os.Lstat(j.Link)
+		if err != nil {
+			if os.IsNotExist(err) {
+				return false, fmt.Sprintf("host %s junction missing", j.Name)
+			}
+			return false, fmt.Sprintf("lstat error: %v", err)
 		}
-		return false, fmt.Sprintf("lstat error: %v", err)
-	}
 
-	// Verify the path is a link (junction or symlink), not a plain directory.
-	isLink, err := fslink.IsLink(hostLink)
-	if err != nil || !isLink {
-		return false, "host _lyx is not a junction"
-	}
+		// Verify the path is a link (junction or symlink), not a plain directory.
+		isLink, err := fslink.IsLink(j.Link)
+		if err != nil || !isLink {
+			return false, fmt.Sprintf("host %s is not a junction", j.Name)
+		}
 
-	// Resolve both ends and compare canonicalized paths.
-	hostResolved, err := fslink.PointsTo(hostLink)
-	if err != nil {
-		return false, fmt.Sprintf("resolve host link: %v", err)
-	}
+		// Resolve both ends and compare canonicalized paths.
+		hostResolved, err := fslink.PointsTo(j.Link)
+		if err != nil {
+			return false, fmt.Sprintf("resolve host link: %v", err)
+		}
 
-	weftResolved, err := filepath.EvalSymlinks(filepath.Clean(weftLyxDir))
-	if err != nil {
-		return false, fmt.Sprintf("resolve weft target: %v", err)
-	}
+		weftResolved, err := filepath.EvalSymlinks(filepath.Clean(j.Target))
+		if err != nil {
+			return false, fmt.Sprintf("resolve weft target: %v", err)
+		}
 
-	if filepath.Clean(hostResolved) != filepath.Clean(weftResolved) {
-		return false, "host _lyx junction points elsewhere"
+		if filepath.Clean(hostResolved) != filepath.Clean(weftResolved) {
+			return false, fmt.Sprintf("host %s junction points elsewhere", j.Name)
+		}
 	}
 
 	return true, ""
 }
+
+// junctionRepointedDetail formats ReconcileActionJunctionRepointed's Detail
+// string, naming every junction in hostLayout.HostJunctionsHere() as
+// "Link → Target" — not just the one checkJunctionHealth found unhealthy,
+// since WireJunctions repairs (or verifies) all of them in the single call
+// that produced this outcome.
+func junctionRepointedDetail(hostLayout *hubgeometry.Layout) string {
+	junctions := hostLayout.HostJunctionsHere()
+	parts := make([]string, len(junctions))
+	for i, j := range junctions {
+		parts[i] = fmt.Sprintf("%s → %s", j.Link, j.Target)
+	}
+	return "junction re-pointed: " + strings.Join(parts, "; ")
+}
diff --git a/internal/fabricengine/remove_junctions_integration_test.go b/internal/fabricengine/remove_junctions_integration_test.go
new file mode 100644
index 00000000..43ff294d
--- /dev/null
+++ b/internal/fabricengine/remove_junctions_integration_test.go
@@ -0,0 +1,84 @@
+//go:build integration
+
+// remove_junctions_integration_test.go proves card 9's fix: Remove tears down
+// every host junction — not just the worktree-root case
+// fslink.RemoveLinksIn's safety net already covers — including one nested
+// under a non-"." RelPath, where that safety net (which scans only the
+// worktree root's immediate children) cannot see it. At RelPath == "." the
+// safety net masks the bug entirely, which is why this file drives the
+// nested case specifically. From card 15 onward HostJunctions returns two
+// entries (_lyx and _pattern), so this is now a true discriminator against
+// the old _lyx-hardcoded form: _pattern's nested removal has no _lyx-shaped
+// shortcut to fall back on.
+//
+// Package fabricengine_test to reuse newFabricFixture from
+// reconcile_stale_registration_test.go; shares the single TestMain in
+// testmain_test.go.
+
+package fabricengine_test
+
+import (
+	"os"
+	"path/filepath"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/fabricengine"
+	"github.com/Knatte18/loomyard/internal/fslink"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
+)
+
+// TestRemove_TearsDownNestedJunction wires a junction nested one level below
+// the worktree root (RelPath "sub") and asserts Remove leaves no junction
+// behind at that nested path.
+func TestRemove_TearsDownNestedJunction(t *testing.T) {
+	t.Parallel()
+
+	const slug = "remove-nested-junction"
+	fixture := newFabricFixture(t)
+	l := fixture.Layout
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+
+	if _, err := topology.Add(l, slug, fabricengine.AddOptions{SkipPush: true}); err != nil {
+		t.Fatalf("setup Add: %v", err)
+	}
+
+	// Resolve a nested layout: same worktree (l.WorktreeRoot), but Cwd one
+	// level deeper — RelPath becomes "sub", matching the hub-wide nesting
+	// convention HostLyxLink/WeftLyxDirFor assume (every sibling worktree
+	// nests at the same RelPath offset as the caller's own).
+	subDir := filepath.Join(l.WorktreeRoot, "sub")
+	if err := os.MkdirAll(subDir, 0o755); err != nil {
+		t.Fatalf("mkdir %s: %v", subDir, err)
+	}
+	nestedLayout, err := hubgeometry.Resolve(subDir)
+	if err != nil {
+		t.Fatalf("hubgeometry.Resolve(%s): %v", subDir, err)
+	}
+	if nestedLayout.RelPath != "sub" {
+		t.Fatalf("nestedLayout.RelPath = %q; want %q", nestedLayout.RelPath, "sub")
+	}
+
+	if err := fabricengine.WireJunctions(nestedLayout, slug); err != nil {
+		t.Fatalf("WireJunctions(nested): %v", err)
+	}
+
+	nestedLyxLink := nestedLayout.HostLyxLink(slug)
+	if isLink, err := fslink.IsLink(nestedLyxLink); err != nil || !isLink {
+		t.Fatalf("setup: nested _lyx junction %s not wired: isLink=%v err=%v", nestedLyxLink, isLink, err)
+	}
+	nestedPatternLink := nestedLayout.HostPatternLink(slug)
+	if isLink, err := fslink.IsLink(nestedPatternLink); err != nil || !isLink {
+		t.Fatalf("setup: nested _pattern junction %s not wired: isLink=%v err=%v", nestedPatternLink, isLink, err)
+	}
+
+	if _, err := topology.Remove(nestedLayout, slug, true); err != nil {
+		t.Fatalf("Remove: %v", err)
+	}
+
+	if _, statErr := os.Lstat(nestedLyxLink); !os.IsNotExist(statErr) {
+		t.Errorf("nested _lyx junction %s still exists after Remove", nestedLyxLink)
+	}
+	if _, statErr := os.Lstat(nestedPatternLink); !os.IsNotExist(statErr) {
+		t.Errorf("nested _pattern junction %s still exists after Remove", nestedPatternLink)
+	}
+}
diff --git a/internal/fabricengine/status.go b/internal/fabricengine/status.go
index 2eb05aac..430b8a52 100644
--- a/internal/fabricengine/status.go
+++ b/internal/fabricengine/status.go
@@ -3,9 +3,9 @@
 //
 // Status enumerates all host worktrees via hubgeometry.List, pairs each with its weft
 // sibling, reports branch, in-sync verdict, junction health, and scans the host index
-// for any _lyx or _raddle paths that have been accidentally git-tracked (host pollution).
-// A pair is InSync when weftBranch == WeftBranchName(hostBranch), and DriftReason
-// states the expected suffixed branch rather than a bare mismatch.
+// for any _lyx, _pattern, or _raddle paths that have been accidentally git-tracked
+// (host pollution). A pair is InSync when weftBranch == WeftBranchName(hostBranch),
+// and DriftReason states the expected suffixed branch rather than a bare mismatch.
 //
 // Status computes its in-sync verdict inline (branch correspondence via WeftBranchName,
 // then junction health via checkJunctionHealth, both already defined in reconcile.go)
@@ -76,9 +76,10 @@ type StatusResult struct {
 //   - Reports in-sync status: weftBranch == WeftBranchName(hostBranch) and the host
 //     _lyx junction is valid
 //   - Reports junction health (separate from the drift check) using checkJunctionHealth
-//   - Scans the host index for any _lyx or _raddle paths via git ls-files; marks
-//     _lyx entries as remediable (git rm --cached + restore junction/exclude) and
-//     _raddle entries as report-only (no junction to restore in this task)
+//   - Scans the host index for any _lyx, _pattern, or _raddle paths via git ls-files;
+//     marks _lyx and _pattern entries as remediable (git rm --cached + restore
+//     junction/exclude — both have a junction from card 15 onward) and _raddle
+//     entries as report-only (no junction to restore in this task)
 //
 // Layout l is the resolved layout for the current working directory; it provides Hub
 // and Prime fields for deriving the weft repo root and weft worktree names.
@@ -145,9 +146,7 @@ func (t *Topology) Status(l *hubgeometry.Layout) (StatusResult, error) {
 
 		// Determine junction health independently of the drift verdict so callers
 		// can distinguish "branches match but junction is broken" from full in-sync.
-		hostLink := hostLayout.HostLyxLinkHere()
-		weftLyxDir := hostLayout.WeftLyxDir()
-		junctionHealthy, junctionReason := checkJunctionHealth(hostLink, weftLyxDir)
+		junctionHealthy, junctionReason := checkJunctionHealth(hostLayout)
 		pair.JunctionHealthy = junctionHealthy
 		pair.JunctionReason = junctionReason
 
@@ -167,7 +166,8 @@ func (t *Topology) Status(l *hubgeometry.Layout) (StatusResult, error) {
 			pair.InSync = true
 		}
 
-		// Scan the host index for _lyx and _raddle paths that must never be tracked there.
+		// Scan the host index for _lyx, _pattern, and _raddle paths that must never
+		// be tracked there.
 		pollution, pollErr := detectHostPollution(hostPath)
 		if pollErr != nil {
 			// Non-fatal: record the error inline and continue.
@@ -185,18 +185,26 @@ func (t *Topology) Status(l *hubgeometry.Layout) (StatusResult, error) {
 	return result, nil
 }
 
-// detectHostPollution scans the host worktree index for _lyx and _raddle paths
-// that should never be tracked in the host repo.
+// detectHostPollution scans the host worktree index for _lyx, _pattern, and _raddle
+// paths that should never be tracked in the host repo.
 //
-// For each match under _lyx, the remedy is the git rm --cached command that removes
-// the file from the index without deleting it from disk, plus a reminder to restore
-// the junction/exclude entry. _raddle matches are report-only: no junction is wired
-// for _raddle in this release so no automated restore step is offered.
+// For each match under _lyx or _pattern, the remedy is the git rm --cached command
+// that removes the file from the index without deleting it from disk, plus a
+// reminder to restore the junction/exclude entry — both have a junction to restore
+// (from card 15 onward), so the same automated remedy applies to both. _raddle
+// matches are report-only: no junction is wired for _raddle in this release so no
+// automated restore step is offered.
+//
+// The two new "_pattern" uses below (the ls-files pathspec entry and the
+// strings.HasPrefix comparison) are legal under the Hub Geometry Invariant despite
+// "_pattern" being an enforced token: the invariant's own carve-out excludes
+// comparisons and git-pathspec slice literals from "path construction," which is
+// what a filepath.Join argument, a "+" operand, or a string const value are.
 func detectHostPollution(hostPath string) ([]PollutionEntry, error) {
 	// git ls-files lists only tracked (index) files matching the given pathspecs.
 	// Using -- prevents ambiguity when the pathspec looks like a branch name.
 	out, _, exitCode, err := gitexec.RunGit(
-		[]string{"ls-files", "--", "_lyx", "_raddle"},
+		[]string{"ls-files", "--", "_lyx", "_pattern", "_raddle"},
 		hostPath,
 	)
 	if err != nil {
@@ -220,8 +228,10 @@ func detectHostPollution(hostPath string) ([]PollutionEntry, error) {
 			continue
 		}
 
-		// Determine whether the path is under _lyx or _raddle.
-		if strings.HasPrefix(tracked, "_lyx") || tracked == "_lyx" {
+		// Determine whether the path is under _lyx, _pattern, or _raddle.
+		switch {
+		case strings.HasPrefix(tracked, "_lyx") || tracked == "_lyx",
+			strings.HasPrefix(tracked, "_pattern") || tracked == "_pattern":
 			// Offer git rm --cached as the remedy, plus a reminder to restore the
 			// junction and exclude entry so lyx topology is intact afterwards.
 			remedy := fmt.Sprintf(
@@ -232,7 +242,7 @@ func detectHostPollution(hostPath string) ([]PollutionEntry, error) {
 				Path:   tracked,
 				Remedy: remedy,
 			})
-		} else if strings.HasPrefix(tracked, "_raddle") || tracked == "_raddle" {
+		case strings.HasPrefix(tracked, "_raddle") || tracked == "_raddle":
 			// _raddle pollution is report-only: no junction is wired for _raddle yet.
 			entries = append(entries, PollutionEntry{
 				Path:       tracked,
diff --git a/internal/fabricengine/template.yaml b/internal/fabricengine/template.yaml
index 15caedb5..e29e88d9 100644
--- a/internal/fabricengine/template.yaml
+++ b/internal/fabricengine/template.yaml
@@ -1,2 +1,2 @@
 branch_prefix: ${env:LYX_BRANCH_PREFIX:-}  # prefix prepended to the slug to form the branch name (e.g. "hanf/"); empty = branch == slug
-pathspec: _lyx  # directory path(s) relative to worktree root, whitespace-separated; _lyx is the default
+pathspec: _lyx _pattern  # directory path(s) relative to worktree root, whitespace-separated; _lyx _pattern is the default
diff --git a/internal/fabricengine/template_test.go b/internal/fabricengine/template_test.go
index f5f2b5fe..6dde5f7b 100644
--- a/internal/fabricengine/template_test.go
+++ b/internal/fabricengine/template_test.go
@@ -6,6 +6,7 @@
 package fabricengine
 
 import (
+	"strings"
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/yamlengine"
@@ -63,9 +64,14 @@ func TestConfigTemplate_ResolvesToEmptyBranchPrefix(t *testing.T) {
 	}
 }
 
-// TestConfigTemplate_PathspecResolvesToLyx asserts that the template's
-// pathspec default resolves to "_lyx" regardless of environment.
-func TestConfigTemplate_PathspecResolvesToLyx(t *testing.T) {
+// TestConfigTemplate_PathspecResolvesToLyxAndPattern asserts that the
+// template's pathspec default resolves to "_lyx" and "_pattern", in that
+// order, regardless of environment. The resolved value is whitespace-split
+// (mirroring Config.Dirs, the consumer that actually splits it) rather than
+// compared as one whole string, since the value is whitespace-split at the
+// consumer -- a splitting bug there would otherwise be silent and would
+// simply drop "_pattern".
+func TestConfigTemplate_PathspecResolvesToLyxAndPattern(t *testing.T) {
 	got := ConfigTemplate()
 	resolved, err := yamlengine.Resolve([]byte(got), nil)
 	if err != nil {
@@ -81,7 +87,19 @@ func TestConfigTemplate_PathspecResolvesToLyx(t *testing.T) {
 	if !ok {
 		t.Fatalf("resolved template missing key pathspec")
 	}
-	if pathspec != "_lyx" {
-		t.Errorf("resolved[pathspec] = %q; want %q", pathspec, "_lyx")
+	pathspecStr, ok := pathspec.(string)
+	if !ok {
+		t.Fatalf("resolved[pathspec] = %#v; want a string", pathspec)
+	}
+	got2 := strings.Fields(pathspecStr)
+	want := []string{"_lyx", "_pattern"}
+	if len(got2) != len(want) {
+		t.Fatalf("resolved[pathspec] whitespace-split = %v; want %v", got2, want)
+	}
+	for i := range want {
+		if got2[i] != want[i] {
+			t.Errorf("resolved[pathspec] whitespace-split = %v; want %v", got2, want)
+			break
+		}
 	}
 }
diff --git a/internal/fabricengine/weftgit.go b/internal/fabricengine/weftgit.go
index 45fd8fd0..44113dc6 100644
--- a/internal/fabricengine/weftgit.go
+++ b/internal/fabricengine/weftgit.go
@@ -231,23 +231,96 @@ func (f *Fabric) warpHeadSHA() (sha string, unborn bool, err error) {
 	return "", false, err
 }
 
+// weftPathspecFilter filters pathspec entries before staging, so a caller's
+// stale positive entry (e.g. "_pattern" in a worktree where nothing has ever
+// been written there) never reaches `git add`, which fails its ENTIRE
+// invocation — including every other, genuinely-matching entry — the moment
+// one entry matches nothing at all.
+//
+// An entry is kept if either:
+//   - it begins with ":" — git pathspec magic (an ":(exclude)..." entry from
+//     internal/buildercli/weft.go, internal/webstercli/weft.go, or
+//     internal/perchcli/run.go's cross-module exclusions). Magic entries are
+//     always passed through untouched and NEVER evaluated for a match: they
+//     do not name a path to check, and treating one as a plain path would
+//     both mis-evaluate it and defeat its own purpose.
+//   - it is a plain path that matches at least one path in the weft
+//     worktree OR the index (see entryMatchesWeft). Untracked-in-worktree
+//     must count: a brand-new "_pattern/PATTERN.md" is untracked at the
+//     moment of its first commit, so a tracked-only check would drop the
+//     very first PATTERN commit. Index-only must count too:
+//     internal/initengine/undo.go commits a "_lyx" path that os.RemoveAll
+//     has just deleted from the worktree, surviving only in the index, so a
+//     worktree-existence-only check would silently break `lyx init --undo`.
+//
+// Returns the filtered entries and whether at least one non-magic (plain)
+// entry survived the filter. When positive is false, CommitWeft must not
+// call StageAndCommit at all, even with a non-empty filtered slice: handing
+// git a pathspec made up of only ":(exclude)" entries and no positive entry
+// is read by git as "everything except those," staging the entire weft
+// worktree — the opposite of the no-op CommitWeft already promises for
+// "nothing of ours to stage."
+func weftPathspecFilter(weftPath string, pathspec []string) (filtered []string, positive bool, err error) {
+	for _, entry := range pathspec {
+		if strings.HasPrefix(entry, ":") {
+			filtered = append(filtered, entry)
+			continue
+		}
+		matches, err := entryMatchesWeft(weftPath, entry)
+		if err != nil {
+			return nil, false, err
+		}
+		if matches {
+			filtered = append(filtered, entry)
+			positive = true
+		}
+	}
+	return filtered, positive, nil
+}
+
+// entryMatchesWeft reports whether pathspec entry matches at least one path
+// tracked in the weft repo's index or present untracked in its worktree,
+// via `git ls-files --cached --others -- <entry>` run with cwd at weftPath
+// — the same anchor StageAndCommit's own `git add` uses, so this check can
+// never disagree with the command it is filtering for. --cached covers an
+// index-only path (already deleted from the worktree); --others covers a
+// brand-new untracked file. Either alone would miss one of the two real
+// callers this filter exists for — see weftPathspecFilter's doc comment.
+func entryMatchesWeft(weftPath, entry string) (bool, error) {
+	stdout, stderr, code, err := gitexec.RunGit([]string{"ls-files", "--cached", "--others", "--", entry}, weftPath)
+	if err != nil {
+		return false, fmt.Errorf("fabricengine: git ls-files --cached --others -- %s: %w", entry, err)
+	}
+	if code != 0 {
+		return false, fmt.Errorf("fabricengine: git ls-files --cached --others -- %s in %s: %s", entry, weftPath, stderr)
+	}
+	return strings.TrimSpace(stdout) != "", nil
+}
+
 // CommitWeft stages pathspec-scoped changes in the weft worktree and commits
 // them, under the fabric-layer write lock. Staging always goes through
 // f.Weft.StageAndCommit's explicit pathspec list — CommitWeft never calls
-// StageAllAndCommit, per gitrepo's doc.go consumer rules. When the warp repo
-// already has a HEAD, the commit carries a Warp-SHA trailer naming it, and
-// RecordCorrespondence is called immediately with the (pre-push) new weft
-// SHA: this is the detached CLI push path's pre-push record, which
-// self-corrects at lookup time if a later rebase-recovered push rewrites the
-// SHA out from under it. When the warp repo has no commits yet (see
-// warpHeadSHA), the commit lands with no trailer and no correspondence
-// record — there is no warp SHA yet to name — and normal trailer/record
-// behavior resumes on the first CommitWeft call after warp's first commit.
-// Returns ("", false, nil) when opts.SkipGit is true, nothing was staged, or
-// pathspec has already been fully removed from both the working tree and
-// the index by a prior commit — CommitWeft tolerates git's "did not match
-// any files" pathspec failure, which the shared gitrepo.StageAndCommit
-// primitive does not special-case on its own.
+// StageAllAndCommit, per gitrepo's doc.go consumer rules. Immediately before
+// staging, pathspec is run through weftPathspecFilter (still inside the
+// write lock): non-magic entries that match nothing in the worktree or
+// index are dropped, and if no positive entry survives at all, CommitWeft
+// returns ("", false, nil) without calling StageAndCommit — see
+// weftPathspecFilter's doc comment for why that early return is not
+// optional. When the warp repo already has a HEAD, the commit carries a
+// Warp-SHA trailer naming it, and RecordCorrespondence is called immediately
+// with the (pre-push) new weft SHA: this is the detached CLI push path's
+// pre-push record, which self-corrects at lookup time if a later
+// rebase-recovered push rewrites the SHA out from under it. When the warp
+// repo has no commits yet (see warpHeadSHA), the commit lands with no
+// trailer and no correspondence record — there is no warp SHA yet to name —
+// and normal trailer/record behavior resumes on the first CommitWeft call
+// after warp's first commit. Returns ("", false, nil) when opts.SkipGit is
+// true, nothing was staged, or pathspec has already been fully removed from
+// both the working tree and the index by a prior commit — CommitWeft
+// tolerates git's "did not match any files" pathspec failure, which the
+// shared gitrepo.StageAndCommit primitive does not special-case on its own
+// (retained as a defense-in-depth fallback; weftPathspecFilter's own
+// pre-check is what keeps this path from being reached in practice).
 func (f *Fabric) CommitWeft(pathspec []string, message string, opts SyncOptions) (sha string, committed bool, err error) {
 	if opts.SkipGit {
 		return "", false, nil
@@ -273,7 +346,15 @@ func (f *Fabric) CommitWeft(pathspec []string, message string, opts SyncOptions)
 		commitMessage = appendWarpSHATrailer(message, warpSHA)
 	}
 
-	sha, committed, err = f.Weft.StageAndCommit(commitMessage, pathspec)
+	filteredPathspec, positive, err := weftPathspecFilter(f.weftPath, pathspec)
+	if err != nil {
+		return "", false, err
+	}
+	if !positive {
+		return "", false, nil
+	}
+
+	sha, committed, err = f.Weft.StageAndCommit(commitMessage, filteredPathspec)
 	if err != nil {
 		// gitrepo.StageAndCommit's `git add --` does not tolerate a pathspec
 		// that no longer matches anything at all, on disk or in the index.
diff --git a/internal/fabricengine/weftgit_pathspec_integration_test.go b/internal/fabricengine/weftgit_pathspec_integration_test.go
new file mode 100644
index 00000000..abe8f028
--- /dev/null
+++ b/internal/fabricengine/weftgit_pathspec_integration_test.go
@@ -0,0 +1,323 @@
+//go:build integration
+
+// weftgit_pathspec_integration_test.go — integration coverage for
+// weftPathspecFilter, the pre-stage filter CommitWeft runs immediately
+// before f.Weft.StageAndCommit: one test per predicate clause, against real
+// git, plus (added by card 14) the batch's own regression assertion that the
+// widened default pathspec and this filter belong together. Package
+// fabricengine (internal), reusing index_integration_test.go's
+// newPlainWarpRepo/newFabric fixture helpers and syncweft_integration_test.go's
+// writeWeftConfigContent, since both files share this package.
+
+package fabricengine
+
+import (
+	"os"
+	"os/exec"
+	"path/filepath"
+	"strings"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/lyxtest"
+	"github.com/Knatte18/loomyard/internal/yamlengine"
+	"gopkg.in/yaml.v3"
+)
+
+// lsFilesWeft returns `git ls-files`'s raw output for weftPath — the
+// currently-tracked (index) path set, read fresh after whatever the test
+// just did.
+func lsFilesWeft(t *testing.T, weftPath string) string {
+	t.Helper()
+
+	cmd := exec.Command("git", "ls-files")
+	cmd.Dir = weftPath
+	out, err := cmd.Output()
+	if err != nil {
+		t.Fatalf("git ls-files in %s: %v", weftPath, err)
+	}
+	return string(out)
+}
+
+// diffCachedQuietWeft reports whether `git diff --cached --quiet` exits 0 in
+// weftPath — true means nothing at all is staged.
+func diffCachedQuietWeft(t *testing.T, weftPath string) bool {
+	t.Helper()
+
+	cmd := exec.Command("git", "diff", "--cached", "--quiet")
+	cmd.Dir = weftPath
+	err := cmd.Run()
+	if err == nil {
+		return true
+	}
+	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
+		return false
+	}
+	t.Fatalf("git diff --cached --quiet in %s: %v", weftPath, err)
+	return false
+}
+
+// mustWriteFileWeft writes content to path, creating parent directories as
+// needed, failing the test on any error.
+func mustWriteFileWeft(t *testing.T, path, content string) {
+	t.Helper()
+
+	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
+		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), err)
+	}
+	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
+		t.Fatalf("WriteFile %s: %v", path, err)
+	}
+}
+
+// TestCommitWeft_UntrackedNewFileCountsAsMatch covers the "untracked must
+// count" predicate clause: a pathspec's first entry ("doesnotexist") matches
+// nothing at all and must be silently dropped rather than failing the whole
+// `git add`, while the second entry ("newmodule") names a brand-new,
+// never-staged directory — untracked in the worktree, absent from the index
+// — that must still count as a match and get committed. This is the exact
+// shape a first-ever "_pattern/PATTERN.md" commit needs: a
+// tracked-only-in-the-index predicate would filter it out and drop the very
+// first PATTERN commit.
+func TestCommitWeft_UntrackedNewFileCountsAsMatch(t *testing.T) {
+	t.Parallel()
+
+	warpPath := newPlainWarpRepo(t)
+	weftFixture := lyxtest.CopyWeft(t)
+	f := newFabric(t, warpPath, weftFixture.WeftPath)
+
+	mustWriteFileWeft(t, filepath.Join(weftFixture.WeftPath, "newmodule", "newfile.txt"), "brand new, never staged")
+
+	sha, committed, err := f.CommitWeft([]string{"doesnotexist", "newmodule"}, DefaultCommitMessage, SyncOptions{})
+	if err != nil {
+		t.Fatalf("CommitWeft() error = %v; want nil", err)
+	}
+	if !committed {
+		t.Fatalf("CommitWeft() committed = false; want true")
+	}
+	if sha == "" {
+		t.Errorf("CommitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
+	}
+
+	tracked := lsFilesWeft(t, weftFixture.WeftPath)
+	if !strings.Contains(tracked, "newmodule/newfile.txt") {
+		t.Errorf("git ls-files = %q; want it to track newmodule/newfile.txt", tracked)
+	}
+}
+
+// TestCommitWeft_IndexOnlyDeletionCountsAsMatch covers the "index-only must
+// count" predicate clause: internal/initengine/undo.go's `lyx init --undo`
+// commits a "_lyx" path that os.RemoveAll has just deleted from the
+// worktree, surviving only in the index at that point. A
+// worktree-existence-only predicate would silently break that deletion
+// commit — this test seeds a tracked file, deletes it from disk only (never
+// staged), then asserts CommitWeft still commits the deletion.
+func TestCommitWeft_IndexOnlyDeletionCountsAsMatch(t *testing.T) {
+	t.Parallel()
+
+	warpPath := newPlainWarpRepo(t)
+	weftFixture := lyxtest.CopyWeft(t)
+	f := newFabric(t, warpPath, weftFixture.WeftPath)
+
+	trackedPath := filepath.Join(weftFixture.WeftPath, "_lyx", "trackedfile.txt")
+	mustWriteFileWeft(t, trackedPath, "tracked content")
+	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "add", "_lyx/trackedfile.txt")
+	lyxtest.MustRun(t, weftFixture.WeftPath, "git", "commit", "-q", "-m", "seed tracked file")
+
+	// Delete from disk only — the file survives in the index until something
+	// stages the deletion. This is the exact state undo.go's os.RemoveAll
+	// leaves the weft worktree in immediately before its own CommitWeft call.
+	if err := os.Remove(trackedPath); err != nil {
+		t.Fatalf("os.Remove(%q): %v", trackedPath, err)
+	}
+
+	sha, committed, err := f.CommitWeft([]string{"_lyx"}, DefaultCommitMessage, SyncOptions{})
+	if err != nil {
+		t.Fatalf("CommitWeft() error = %v; want nil", err)
+	}
+	if !committed {
+		t.Fatalf("CommitWeft() committed = false; want true (the index-only deletion should count as a match)")
+	}
+	if sha == "" {
+		t.Errorf("CommitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
+	}
+
+	tracked := lsFilesWeft(t, weftFixture.WeftPath)
+	if strings.Contains(tracked, "_lyx/trackedfile.txt") {
+		t.Errorf("git ls-files = %q; want _lyx/trackedfile.txt no longer tracked after the deletion commit", tracked)
+	}
+}
+
+// TestCommitWeft_ExcludeMagicPassesThroughUntouched is the mandatory guard
+// case: a pathspec carrying a ":(exclude)" entry must pass it through
+// untouched (never evaluated as a plain path to match) while a genuine
+// positive entry alongside it still commits — and the excluded artifact must
+// stay unstaged. Without this test, a filter that behaves correctly on plain
+// paths could still silently re-stage machine-local artifacts by
+// mis-evaluating exclusion magic as an ordinary non-matching entry.
+func TestCommitWeft_ExcludeMagicPassesThroughUntouched(t *testing.T) {
+	t.Parallel()
+
+	warpPath := newPlainWarpRepo(t)
+	weftFixture := lyxtest.CopyWeft(t)
+	f := newFabric(t, warpPath, weftFixture.WeftPath)
+
+	mustWriteFileWeft(t, filepath.Join(weftFixture.WeftPath, "_lyx", "durable.txt"), "durable state")
+	mustWriteFileWeft(t, filepath.Join(weftFixture.WeftPath, "_lyx", "run.lock"), "machine-local lock")
+
+	sha, committed, err := f.CommitWeft([]string{"_lyx", ":(exclude)_lyx/*.lock"}, DefaultCommitMessage, SyncOptions{})
+	if err != nil {
+		t.Fatalf("CommitWeft() error = %v; want nil", err)
+	}
+	if !committed {
+		t.Fatalf("CommitWeft() committed = false; want true")
+	}
+	if sha == "" {
+		t.Errorf("CommitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
+	}
+
+	tracked := lsFilesWeft(t, weftFixture.WeftPath)
+	if !strings.Contains(tracked, "_lyx/durable.txt") {
+		t.Errorf("git ls-files = %q; want it to track _lyx/durable.txt", tracked)
+	}
+	if strings.Contains(tracked, "_lyx/run.lock") {
+		t.Errorf("git ls-files = %q; want _lyx/run.lock excluded, never tracked", tracked)
+	}
+}
+
+// TestCommitWeft_OnlyPositiveEntryMatchingNothing_StagesNothing covers the
+// early-return case: when the only non-magic entry in the pathspec matches
+// nothing at all, CommitWeft must return ("", false, nil) WITHOUT ever
+// calling StageAndCommit — leaving a genuinely dirty tracked file
+// (modified, unstaged) and an untracked lock file completely untouched.
+// Asserting the pre-existing HEAD SHA is unchanged and nothing is staged is
+// what actually catches the regression this guards: handing git a pathspec
+// of only ":(exclude)" magic with no positive entry is read as "everything
+// except those," which would otherwise stage the entire weft worktree
+// (including the dirty config.yaml) rather than nothing.
+func TestCommitWeft_OnlyPositiveEntryMatchingNothing_StagesNothing(t *testing.T) {
+	t.Parallel()
+
+	warpPath := newPlainWarpRepo(t)
+	weftFixture := lyxtest.CopyWeft(t)
+	f := newFabric(t, warpPath, weftFixture.WeftPath)
+
+	preSHA := currentSHA(t, weftFixture.WeftPath)
+
+	// A genuinely dirty tracked file and an untracked lock file: if the
+	// filter mishandled the all-negative pathspec by staging everything,
+	// both would show up staged below.
+	writeWeftConfigContent(t, weftFixture.WeftPath, "dirtied but must stay unstaged")
+	mustWriteFileWeft(t, filepath.Join(weftFixture.WeftPath, "_lyx", "run.lock"), "machine-local lock")
+
+	sha, committed, err := f.CommitWeft([]string{"doesnotexist", ":(exclude)_lyx/*.lock"}, DefaultCommitMessage, SyncOptions{})
+	if err != nil {
+		t.Fatalf("CommitWeft() error = %v; want nil", err)
+	}
+	if committed {
+		t.Fatalf("CommitWeft() committed = true; want false (the only positive entry matches nothing)")
+	}
+	if sha != "" {
+		t.Errorf("CommitWeft() sha = %q; want empty", sha)
+	}
+
+	if !diffCachedQuietWeft(t, weftFixture.WeftPath) {
+		t.Errorf("git diff --cached --quiet reports staged changes; want nothing staged at all")
+	}
+	postSHA := currentSHA(t, weftFixture.WeftPath)
+	if postSHA != preSHA {
+		t.Errorf("weft HEAD changed from %q to %q; want unchanged (no commit should have been made)", preSHA, postSHA)
+	}
+}
+
+// resolvedDefaultPathspecDirs resolves the fabric config template's REAL
+// default pathspec and splits it via Config.Dirs -- not a hand-written
+// literal — so the regression test below exercises whatever the template
+// actually declares today, catching a future default change too.
+func resolvedDefaultPathspecDirs(t *testing.T) []string {
+	t.Helper()
+
+	resolved, err := yamlengine.Resolve([]byte(ConfigTemplate()), nil)
+	if err != nil {
+		t.Fatalf("yamlengine.Resolve(ConfigTemplate()): %v", err)
+	}
+	var cfg Config
+	if err := yaml.Unmarshal(resolved, &cfg); err != nil {
+		t.Fatalf("yaml.Unmarshal resolved config template: %v", err)
+	}
+	return cfg.Dirs()
+}
+
+// TestCommitWeft_WidenedDefaultPathspec_LyxChangeStillCommitsWithNoPattern is
+// this batch's single most important regression assertion, proving card 13
+// (the pathspec-tolerance filter) and card 14 (the widened default
+// pathspec) belong together in one batch: with the real, resolved default
+// pathspec — "_lyx _pattern" — and NO files under "_pattern" at all, a
+// genuine "_lyx" change still commits. Without weftPathspecFilter, this is
+// exactly the silent regression the batch scope describes: `git add --
+// _lyx _pattern` fails in its entirety the moment `_pattern` matches
+// nothing, and CommitWeft's own pre-existing "did not match any files"
+// tolerance swallows that into ("", false, nil) with no error — so the
+// only way to catch it is asserting the commit actually happened, not
+// checking for an error. Covers both shapes an empty "_pattern" can take —
+// wholly absent and present-but-empty — since git tracks files, not
+// directories, and a materialised-but-empty "_pattern/" is the normal,
+// expected state for this whole task while content migration stays out of
+// scope.
+func TestCommitWeft_WidenedDefaultPathspec_LyxChangeStillCommitsWithNoPattern(t *testing.T) {
+	dirs := resolvedDefaultPathspecDirs(t)
+	if len(dirs) != 2 || dirs[0] != "_lyx" || dirs[1] != "_pattern" {
+		t.Fatalf("resolved default pathspec dirs = %v; want [_lyx _pattern]", dirs)
+	}
+
+	t.Run("PatternDirWhollyAbsent", func(t *testing.T) {
+		t.Parallel()
+
+		warpPath := newPlainWarpRepo(t)
+		weftFixture := lyxtest.CopyWeft(t)
+		f := newFabric(t, warpPath, weftFixture.WeftPath)
+
+		if _, err := os.Stat(filepath.Join(weftFixture.WeftPath, "_pattern")); !os.IsNotExist(err) {
+			t.Fatalf("precondition: _pattern must not exist in this fixture; Stat err = %v", err)
+		}
+
+		writeWeftConfigContent(t, weftFixture.WeftPath, "lyx change, _pattern wholly absent")
+
+		sha, committed, err := f.CommitWeft(dirs, DefaultCommitMessage, SyncOptions{})
+		if err != nil {
+			t.Fatalf("CommitWeft() error = %v; want nil", err)
+		}
+		if !committed {
+			t.Fatalf("CommitWeft() committed = false; want true (the _lyx change must still commit)")
+		}
+		if sha == "" {
+			t.Errorf("CommitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
+		}
+	})
+
+	t.Run("PatternDirExistsButEmpty", func(t *testing.T) {
+		t.Parallel()
+
+		warpPath := newPlainWarpRepo(t)
+		weftFixture := lyxtest.CopyWeft(t)
+		f := newFabric(t, warpPath, weftFixture.WeftPath)
+
+		// git tracks files, not directories: a materialised-but-empty
+		// "_pattern/" still has nothing for a pathspec to match.
+		if err := os.MkdirAll(filepath.Join(weftFixture.WeftPath, "_pattern"), 0o755); err != nil {
+			t.Fatalf("MkdirAll _pattern: %v", err)
+		}
+
+		writeWeftConfigContent(t, weftFixture.WeftPath, "lyx change, _pattern present but empty")
+
+		sha, committed, err := f.CommitWeft(dirs, DefaultCommitMessage, SyncOptions{})
+		if err != nil {
+			t.Fatalf("CommitWeft() error = %v; want nil", err)
+		}
+		if !committed {
+			t.Fatalf("CommitWeft() committed = false; want true (the _lyx change must still commit)")
+		}
+		if sha == "" {
+			t.Errorf("CommitWeft() sha = %q; want a non-empty new HEAD SHA", sha)
+		}
+	})
+}
diff --git a/internal/fabricengine/weftwiring.go b/internal/fabricengine/weftwiring.go
index e4f40e58..9acfc0a3 100644
--- a/internal/fabricengine/weftwiring.go
+++ b/internal/fabricengine/weftwiring.go
@@ -21,6 +21,7 @@
 package fabricengine
 
 import (
+	"errors"
 	"fmt"
 	"os"
 
@@ -116,17 +117,52 @@ func pushWeftBranch(l *hubgeometry.Layout, slug, branch string, opts SyncOptions
 	return nil
 }
 
-// removeHostJunction removes the host _lyx junction at the given link path.
+// removeHostJunction removes every host junction for slug — every entry in
+// l.HostJunctions(slug) — via fslink.Remove. It is a thin wrapper over
+// removeJunctionRecords, which owns the actual best-effort loop; the split
+// exists purely so the loop's continue-past-failure contract is directly
+// testable against a synthetic junction slice, since l.HostJunctions always
+// returns exactly one entry today and cannot itself produce the
+// multi-junction scenario the contract is about (mirroring
+// unseedLyxJunction/unseedJunctionRecords in junction.go).
 //
-// Uses fslink.Remove to delete the junction/symlink only (idempotent).
-// Returns nil if the junction does not exist (idempotent).
-// Returns an error if removal fails for reasons other than not-exist.
+// Returns nil if every junction is already absent (idempotent). See
+// removeJunctionRecords for the error case.
 func removeHostJunction(l *hubgeometry.Layout, slug string) error {
-	link := l.HostLyxLink(slug)
-	if err := fslink.Remove(link); err != nil {
-		return fmt.Errorf("remove host junction %s: %w", link, err)
+	return removeJunctionRecords(l.HostJunctions(slug))
+}
+
+// removeJunctionRecords removes each junction in junctions via fslink.Remove.
+//
+// It is best-effort and deliberately continues past a per-junction failure,
+// accumulating every error via errors.Join rather than aborting on the first —
+// the opposite of unseedJunctionRecords' abort-on-first-junction-error rule in
+// junction.go, and deliberately so. A future reader must not "fix" one loop to
+// match the other; they intentionally disagree. Remove's call site is
+// `_ = removeHostJunction(l, slug)`, discarding the return value exactly as the
+// adjacent removePortal and removeLaunchers calls in the same teardown do, so
+// aborting on the first junction's failure would silently leave every later
+// junction in place — defeating the whole point of this step.
+//
+// This step exists because Remove's later link-cleanup step,
+// fslink.RemoveLinksIn(target), scans only the immediate children of the
+// worktree root and misses a nested junction whenever RelPath != "." — the
+// reason step (5) of Remove removes junctions explicitly, before that safety
+// net runs. Leaving this _lyx-only would reintroduce exactly that documented
+// bug for every junction after the first.
+//
+// Returns nil if junctions is empty or every entry is already absent
+// (idempotent). Returns a joined error naming every junction whose removal
+// failed; a non-nil error does NOT mean no junction was removed — the loop
+// still attempted (and may have succeeded for) every other junction.
+func removeJunctionRecords(junctions []hubgeometry.HostJunction) error {
+	var errs []error
+	for _, j := range junctions {
+		if err := fslink.Remove(j.Link); err != nil {
+			errs = append(errs, fmt.Errorf("remove host junction %s: %w", j.Link, err))
+		}
 	}
-	return nil
+	return errors.Join(errs...)
 }
 
 // removeWeftWorktree tears down the weft worktree, optionally its branch (an
diff --git a/internal/fabricengine/weftwiring_test.go b/internal/fabricengine/weftwiring_test.go
new file mode 100644
index 00000000..85784947
--- /dev/null
+++ b/internal/fabricengine/weftwiring_test.go
@@ -0,0 +1,87 @@
+// weftwiring_test.go unit-tests removeJunctionRecords directly against
+// synthetic hubgeometry.HostJunction slices — no build tag, since it touches
+// only plain directories and fslink, never git. It exists because
+// l.HostJunctions(slug) still returns exactly one entry in this batch (a
+// second entry is batch 5's job), so removeHostJunction's best-effort,
+// continue-past-failure contract cannot be driven through the exported
+// (l, slug) surface with more than one junction; this file drives the
+// extracted loop directly instead.
+
+package fabricengine
+
+import (
+	"os"
+	"path/filepath"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
+)
+
+// TestRemoveJunctionRecords_ContinuesPastFailure is card 9's regression
+// guard: with one junction in a state that makes its removal fail (a real,
+// non-empty directory, which fslink.Remove cannot delete), the others —
+// before and after it in the slice — are still removed. This is the opposite
+// contract from unseedJunctionRecords (card 8), which aborts on the first
+// failure; both are exercised the same way for the same reason: neither is
+// drivable with more than one junction through l.HostJunctions(slug) yet.
+func TestRemoveJunctionRecords_ContinuesPastFailure(t *testing.T) {
+	t.Parallel()
+
+	root := t.TempDir()
+
+	firstLink := filepath.Join(root, "first-link")
+	firstTarget := filepath.Join(root, "first-target")
+	wireTestJunction(t, firstLink, firstTarget)
+
+	// The middle junction's host path is a real, non-empty directory —
+	// fslink.Remove (a bare os.Remove) cannot delete a non-empty directory,
+	// so this is guaranteed to fail regardless of platform.
+	middleLink := filepath.Join(root, "middle-link")
+	if err := os.MkdirAll(middleLink, 0o755); err != nil {
+		t.Fatalf("mkdir real middle-link dir: %v", err)
+	}
+	if err := os.WriteFile(filepath.Join(middleLink, "marker.txt"), []byte("content"), 0o644); err != nil {
+		t.Fatalf("write marker file: %v", err)
+	}
+
+	lastLink := filepath.Join(root, "last-link")
+	lastTarget := filepath.Join(root, "last-target")
+	wireTestJunction(t, lastLink, lastTarget)
+
+	junctions := []hubgeometry.HostJunction{
+		{Name: "first", Link: firstLink, Target: firstTarget},
+		{Name: "middle", Link: middleLink, Target: filepath.Join(root, "middle-target")},
+		{Name: "last", Link: lastLink, Target: lastTarget},
+	}
+
+	err := removeJunctionRecords(junctions)
+	if err == nil {
+		t.Fatal("removeJunctionRecords = nil error; want a joined error from the middle junction")
+	}
+
+	// Both the junction before AND after the failing one are removed — proving
+	// the loop continued rather than aborting at the first failure.
+	if _, statErr := os.Lstat(firstLink); !os.IsNotExist(statErr) {
+		t.Errorf("first junction %s still exists; want removed despite middle's failure", firstLink)
+	}
+	if _, statErr := os.Lstat(lastLink); !os.IsNotExist(statErr) {
+		t.Errorf("last junction %s still exists; want removed despite middle's failure", lastLink)
+	}
+
+	// The failing directory itself is untouched (fslink.Remove never partially
+	// deletes a real, non-empty directory).
+	if info, statErr := os.Stat(middleLink); statErr != nil || !info.IsDir() {
+		t.Errorf("middle host dir %s not left in place: stat err=%v", middleLink, statErr)
+	}
+}
+
+// TestRemoveJunctionRecords_EmptyIsNoOp asserts that an empty junctions slice
+// (matching l.HostJunctions(slug) before any junction has ever been wired) is
+// a legitimate no-op.
+func TestRemoveJunctionRecords_EmptyIsNoOp(t *testing.T) {
+	t.Parallel()
+
+	if err := removeJunctionRecords(nil); err != nil {
+		t.Errorf("removeJunctionRecords(nil) = %v; want nil", err)
+	}
+}
diff --git a/internal/gitrepo/gitrepo.go b/internal/gitrepo/gitrepo.go
index 039dc70c..ce81d27d 100644
--- a/internal/gitrepo/gitrepo.go
+++ b/internal/gitrepo/gitrepo.go
@@ -146,7 +146,10 @@ func (r *Repo) CurrentSHA() (string, error) {
 // `--` (in the add, the staged-change check, and the commit alike), so an
 // entry starting with ':' is treated as a magic signature (and a file
 // literally named that way cannot be staged as-is) — callers pass plain
-// relative paths and must not rely on magic.
+// relative paths and must not rely on magic. When files contains any magic
+// entry, the `add` step passes `-f`: see hasPathspecMagic's call site for
+// why a plain add otherwise refuses outright, and why -f here does not
+// weaken the exclusion the magic entry itself provides.
 func (r *Repo) StageAndCommit(msg string, files []string) (sha string, committed bool, err error) {
 	// An empty list stages nothing, so there is never anything of the
 	// caller's to commit; return the documented no-op signal before any git
@@ -156,7 +159,29 @@ func (r *Repo) StageAndCommit(msg string, files []string) (sha string, committed
 		return "", false, nil
 	}
 
-	addArgs := append([]string{"add", "--"}, files...)
+	addArgs := []string{"add", "--"}
+	if hasPathspecMagic(files) {
+		// -f: recent git refuses `git add` outright — "The following paths
+		// are ignored by one of your .gitignore files" — the moment ANY
+		// listed pathspec entry names a path matched by .gitignore/exclude,
+		// even when that entry is a ":(exclude)..." entry whose entire
+		// purpose is to keep the path OUT of what gets staged, never to add
+		// it. This fires for real here: fabricengine.seedWeftArtifactExcludes
+		// seeds the weft repo's .git/info/exclude with the same machine-local
+		// artifact patterns (pause flags, *.lock, prompts/) that callers like
+		// buildercli's and webstercli's weftCommit also name in ":(exclude)"
+		// pathspec entries as defense-in-depth (see CONSTRAINTS.md's Weft Git
+		// Invariant, "Cross-module exclusions") — so a plain `git add`
+		// refuses outright before ever reaching the positive entries. -f
+		// suppresses only that pre-check; exclude-magic pathspec entries
+		// still keep the excluded path out of the index exactly as before
+		// (confirmed: `git add -f -- x ":(exclude)x/ignored-path"` stages x
+		// but never ignored-path) — -f does not defeat exclude magic, it only
+		// stops git from refusing to look past an ignored path named inside
+		// pathspec magic.
+		addArgs = []string{"add", "-f", "--"}
+	}
+	addArgs = append(addArgs, files...)
 	_, stderr, code, err := r.run(addArgs...)
 	if err != nil {
 		return "", false, err
@@ -203,6 +228,19 @@ func (r *Repo) StageAndCommit(msg string, files []string) (sha string, committed
 	return sha, true, nil
 }
 
+// hasPathspecMagic reports whether files contains at least one git pathspec
+// magic entry — a leading ':' (long form ":(exclude)..." or shorthand
+// "!"/"^"), as opposed to a plain relative path. StageAndCommit uses this to
+// decide whether the `git add` call needs `-f`; see its call site.
+func hasPathspecMagic(files []string) bool {
+	for _, f := range files {
+		if strings.HasPrefix(f, ":") {
+			return true
+		}
+	}
+	return false
+}
+
 // StageAllAndCommit stages every working-tree change via `git add -A` and
 // commits whatever lands in the index with msg — the wildcard sibling of
 // StageAndCommit, which never wildcard-stages. It exists as board's
diff --git a/internal/hubgeometry/enforcement_test.go b/internal/hubgeometry/enforcement_test.go
index 98bb03fb..46c31313 100644
--- a/internal/hubgeometry/enforcement_test.go
+++ b/internal/hubgeometry/enforcement_test.go
@@ -221,7 +221,7 @@ func TestEnforcement_GeometryLiterals(t *testing.T) {
 	// tokens. Only internal/hubgeometry is permitted to use these in path-construction context.
 	geometryToken := func(s string) bool {
 		switch s {
-		case "_board", "-weft", "-HUB", "_portals", "_launchers", "_raddle", "_lyx":
+		case "_board", "-weft", "-HUB", "_portals", "_launchers", "_raddle", "_lyx", "_pattern":
 			return true
 		}
 		return false
diff --git a/internal/hubgeometry/hubgeometry.go b/internal/hubgeometry/hubgeometry.go
index 2ead8cf1..1c042242 100644
--- a/internal/hubgeometry/hubgeometry.go
+++ b/internal/hubgeometry/hubgeometry.go
@@ -49,6 +49,12 @@ const (
 	// (e.g. "loomyard" → "loomyard-HUB"). It is the single source of this literal;
 	// use HubPath(parent, name) to obtain the full path.
 	HubSuffix = "-HUB"
+
+	// PatternDirName is the directory name for the PATTERN constraint-injection surface
+	// within a worktree (i.e. <worktree>/_pattern), the durable file every agent consults
+	// for conditional constraint injection. It is the single source of this literal; use
+	// PatternDir(baseDir)/PatternFile(baseDir) to obtain the full paths.
+	PatternDirName = "_pattern"
 )
 
 // ErrNotAGitRepo is returned when a directory is not within a git repository.
@@ -332,6 +338,30 @@ func DotEnv(baseDir string) string {
 	return filepath.Join(baseDir, dotEnvName)
 }
 
+// PatternDir returns the path to the _pattern directory within a baseDir.
+//
+// It is a pure bootstrap helper for callers that have no resolved Layout, parallel to
+// ConfigDir. The _pattern directory holds PATTERN.md, the constraint-injection surface
+// every agent consults. Per the Hub Geometry Invariant, no other package may construct
+// this path.
+//
+// Returns filepath.Join(baseDir, PatternDirName).
+func PatternDir(baseDir string) string {
+	return filepath.Join(baseDir, PatternDirName)
+}
+
+// PatternFile returns the path to the PATTERN.md file within a baseDir.
+//
+// It is a pure bootstrap helper for callers that have no resolved Layout, parallel to
+// ConfigFile. Keeping the full path — directory and filename — in this one accessor is
+// what lets internal/pattern stay a leaf that never joins the two halves itself; "PATTERN.md"
+// is deliberately not a geometry constant since only this accessor ever needs it.
+//
+// Returns filepath.Join(PatternDir(baseDir), "PATTERN.md").
+func PatternFile(baseDir string) string {
+	return filepath.Join(PatternDir(baseDir), "PATTERN.md")
+}
+
 // WeftSiblingPath returns the absolute path to the weft sibling worktree for the
 // given slug inside hub.
 //
@@ -384,15 +414,16 @@ func WeftHostSlug(name string) (slug string, ok bool) {
 
 // IsReservedHubName reports whether name is one of the hub-level entry names
 // lyx geometry itself owns: the per-worktree lyx dir (_lyx), the raddle dir
-// (_raddle), the board passenger (_board), and the portal/launcher mirrors
-// (_portals, _launchers). A worktree slug must never claim one of these — a
-// host worktree directory named after a geometry token collides with the very
-// paths lyx composes at the hub level (e.g. a worktree named "_portals" would
-// have portal junctions created inside it). Slug validation (fabric's Add)
-// calls this so the rejection lives with the single owner of the literals.
+// (_raddle), the board passenger (_board), the portal/launcher mirrors
+// (_portals, _launchers), and the PATTERN constraint-injection surface
+// (_pattern). A worktree slug must never claim one of these — a host worktree
+// directory named after a geometry token collides with the very paths lyx
+// composes at the hub level (e.g. a worktree named "_portals" would have
+// portal junctions created inside it). Slug validation (fabric's Add) calls
+// this so the rejection lives with the single owner of the literals.
 func IsReservedHubName(name string) bool {
 	switch name {
-	case LyxDirName, "_raddle", BoardDirName, "_portals", "_launchers":
+	case LyxDirName, "_raddle", BoardDirName, "_portals", "_launchers", PatternDirName:
 		return true
 	}
 	return false
@@ -652,6 +683,30 @@ func (l *Layout) WeftLyxDirFor(slug string) string {
 	return filepath.Join(l.WeftWorktreePath(slug), l.RelPath, LyxDirName)
 }
 
+// WeftPatternDir returns the path to the _pattern directory in the current worktree's weft
+// sibling.
+//
+// The path is: <hub>/<current-worktree>-weft/<RelPath>/_pattern. This mirrors WeftLyxDir
+// exactly and is the junction target for pattern weft, with RelPath-mirroring like
+// WeftLyxDir (collapses to <weft>/_pattern at RelPath ".").
+//
+// Returns filepath.Join(WeftWorktree(), RelPath, PatternDirName).
+func (l *Layout) WeftPatternDir() string {
+	return filepath.Join(l.WeftWorktree(), l.RelPath, PatternDirName)
+}
+
+// WeftPatternDirFor returns the path to the _pattern directory within a named slug's weft
+// worktree.
+//
+// The path is: <hub>/<slug>-weft/<RelPath>/_pattern. This mirrors WeftLyxDirFor exactly
+// and pairs with HostPatternLink(slug) as the junction endpoints, matching the
+// HostLyxLink(slug)/WeftLyxDirFor(slug) precedent.
+//
+// Returns filepath.Join(WeftWorktreePath(slug), RelPath, PatternDirName).
+func (l *Layout) WeftPatternDirFor(slug string) string {
+	return filepath.Join(l.WeftWorktreePath(slug), l.RelPath, PatternDirName)
+}
+
 // WeftRaddleDir returns the path to the _raddle directory in the current worktree's weft sibling.
 //
 // Returns filepath.Join(WeftWorktree(), RelPath, "_raddle").
@@ -680,6 +735,45 @@ func (l *Layout) HostLyxLinkHere() string {
 	return filepath.Join(l.WorktreeRoot, l.RelPath, LyxDirName)
 }
 
+// HostPatternLink returns the path to the _pattern junction link in a named slug's host
+// worktree.
+//
+// The path is: <hub>/<slug>/<RelPath>/_pattern. This mirrors HostLyxLink exactly: the
+// host-side junction endpoint that points into the paired weft worktree via
+// WeftPatternDirFor(slug).
+//
+// Returns filepath.Join(WorktreePath(slug), RelPath, PatternDirName).
+func (l *Layout) HostPatternLink(slug string) string {
+	return filepath.Join(l.WorktreePath(slug), l.RelPath, PatternDirName)
+}
+
+// HostPatternLinkHere returns the path to the _pattern junction link in the current host
+// worktree.
+//
+// The path is: <hub>/<current-worktree>/<RelPath>/_pattern, derived from WorktreeRoot+RelPath,
+// not from Cwd. This mirrors HostLyxLinkHere exactly and serves as the host-side junction
+// endpoint paired with WeftPatternDir().
+//
+// Returns filepath.Join(WorktreeRoot, RelPath, PatternDirName).
+func (l *Layout) HostPatternLinkHere() string {
+	return filepath.Join(l.WorktreeRoot, l.RelPath, PatternDirName)
+}
+
+// PatternFileHere returns the path to the PATTERN.md file for the current worktree.
+//
+// It is anchored at WorktreeRoot+RelPath — not WorktreeRoot alone, and not Cwd — because
+// this is the accessor every agent calls to check whether PATTERN is active (batch 6's
+// active check): a WorktreeRoot-only anchor would miss the file entirely in any nested-hub
+// geometry (Cwd inside a subpath), silently rendering PATTERN inactive in all five agents,
+// while a bare Cwd anchor would drift from the junction endpoints above, which are all
+// WorktreeRoot+RelPath-anchored. On a Resolve-built Layout this is byte-for-byte equal to
+// PatternFile(l.Cwd), since Resolve sets RelPath = filepath.Rel(WorktreeRoot, Cwd).
+//
+// Returns PatternFile(filepath.Join(WorktreeRoot, RelPath)).
+func (l *Layout) PatternFileHere() string {
+	return PatternFile(filepath.Join(l.WorktreeRoot, l.RelPath))
+}
+
 // HostJunction represents a directory junction in the host worktree that links to a weft directory.
 //
 // It carries three fields because the two seeding operations (junction creation and
@@ -695,11 +789,16 @@ type HostJunction struct {
 
 // HostJunctions returns the list of host junctions for a given slug.
 //
-// Currently, this returns a single-element slice containing the _lyx junction.
-// The junction record carries Name, Link, and Target fields for use by the
-// seeders in internal/fabricengine.
+// Returns two entries, _lyx first: {Name: LyxDirName, Link: HostLyxLink(slug), Target:
+// WeftLyxDirFor(slug)} followed by {Name: PatternDirName, Link: HostPatternLink(slug),
+// Target: WeftPatternDirFor(slug)}. The junction record carries Name, Link, and Target
+// fields for use by the seeders in internal/fabricengine. _lyx stays first deliberately:
+// UnwireResult.JunctionsRemoved is documented as being in this slice's order, and the
+// health check is first-unhealthy-wins, so the order is observable by callers.
 //
-// Returns a slice with exactly one entry: {Name: LyxDirName, Link: HostLyxLink(slug), Target: WeftLyxDirFor(slug)}.
+// HostJunctions is Hub/slug-anchored: wiring, unwiring, and remove (which all act on a
+// named slug, not necessarily the current worktree) call this. See HostJunctionsHere
+// below for the Here-anchored, slug-free counterpart the health-check sites use instead.
 func (l *Layout) HostJunctions(slug string) []HostJunction {
 	return []HostJunction{
 		{
@@ -707,5 +806,42 @@ func (l *Layout) HostJunctions(slug string) []HostJunction {
 			Link:   l.HostLyxLink(slug),
 			Target: l.WeftLyxDirFor(slug),
 		},
+		{
+			Name:   PatternDirName,
+			Link:   l.HostPatternLink(slug),
+			Target: l.WeftPatternDirFor(slug),
+		},
+	}
+}
+
+// HostJunctionsHere returns the same HostJunction records as HostJunctions(slug), but
+// resolved against the current worktree rather than a named slug: each entry's Link comes
+// from the corresponding "…Here()" accessor (HostLyxLinkHere(), HostPatternLinkHere()) and
+// each Target from the un-slugged weft accessor (WeftLyxDir(), WeftPatternDir()), mirroring
+// the existing HostLyxLinkHere()/HostLyxLink(slug) and WeftLyxDir()/WeftLyxDirFor(slug)
+// pairs this precedent already establishes.
+//
+// It exists because HostJunctions(slug) is Hub/slug-anchored — the right shape for wiring,
+// unwiring, and remove, which always act on a named slug — while all three junction
+// health-check sites (internal/fabricengine/reconcile.go, status.go, and drift.go) have no
+// slug available and are Here-anchored instead. PairInSync(l *hubgeometry.Layout) in
+// particular takes no slug parameter at all and is documented as stateless; threading a
+// slug into it would break that contract.
+//
+// Returns two entries, _lyx first, mirroring HostJunctions's order: {Name: LyxDirName,
+// Link: HostLyxLinkHere(), Target: WeftLyxDir()} followed by {Name: PatternDirName, Link:
+// HostPatternLinkHere(), Target: WeftPatternDir()}.
+func (l *Layout) HostJunctionsHere() []HostJunction {
+	return []HostJunction{
+		{
+			Name:   LyxDirName,
+			Link:   l.HostLyxLinkHere(),
+			Target: l.WeftLyxDir(),
+		},
+		{
+			Name:   PatternDirName,
+			Link:   l.HostPatternLinkHere(),
+			Target: l.WeftPatternDir(),
+		},
 	}
 }
diff --git a/internal/hubgeometry/hubgeometry_test.go b/internal/hubgeometry/hubgeometry_test.go
index e34692e3..412e0611 100644
--- a/internal/hubgeometry/hubgeometry_test.go
+++ b/internal/hubgeometry/hubgeometry_test.go
@@ -593,13 +593,158 @@ func TestRefactoredMethods(t *testing.T) {
 		slug := "test-slug"
 		junctions := layout.HostJunctions(slug)
 
-		if len(junctions) != 1 {
-			t.Fatalf("HostJunctions() returned %d junctions; want 1", len(junctions))
+		if len(junctions) != 2 {
+			t.Fatalf("HostJunctions() returned %d junctions; want 2", len(junctions))
 		}
 
-		junction := junctions[0]
-		if junction.Name != "_lyx" {
-			t.Errorf("HostJunctions()[0].Name = %q; want %q", junction.Name, "_lyx")
+		lyxJunction := junctions[0]
+		if lyxJunction.Name != "_lyx" {
+			t.Errorf("HostJunctions()[0].Name = %q; want %q", lyxJunction.Name, "_lyx")
+		}
+		if lyxJunction.Link != layout.HostLyxLink(slug) {
+			t.Errorf("HostJunctions()[0].Link = %q; want %q", lyxJunction.Link, layout.HostLyxLink(slug))
+		}
+		if lyxJunction.Target != layout.WeftLyxDirFor(slug) {
+			t.Errorf("HostJunctions()[0].Target = %q; want %q", lyxJunction.Target, layout.WeftLyxDirFor(slug))
+		}
+
+		patternJunction := junctions[1]
+		if patternJunction.Name != "_pattern" {
+			t.Errorf("HostJunctions()[1].Name = %q; want %q", patternJunction.Name, "_pattern")
+		}
+		if patternJunction.Link != layout.HostPatternLink(slug) {
+			t.Errorf("HostJunctions()[1].Link = %q; want %q", patternJunction.Link, layout.HostPatternLink(slug))
+		}
+		if patternJunction.Target != layout.WeftPatternDirFor(slug) {
+			t.Errorf("HostJunctions()[1].Target = %q; want %q", patternJunction.Target, layout.WeftPatternDirFor(slug))
+		}
+	})
+}
+
+// TestHostJunctionsHere verifies the Here-anchored, slug-free junction-detection
+// accessor: it must return the expected Name/Link/Target for both RelPath == "." and a
+// nested RelPath, and it must agree entry-for-entry with HostJunctions(slug) when the
+// layout's slug and current worktree coincide — the precondition every one of
+// fabricengine's health-check call sites relies on.
+func TestHostJunctionsHere(t *testing.T) {
+	t.Parallel()
+
+	fix := lyxtest.CopyHostHub(t)
+	hub := fix.Hub
+
+	t.Run("at root", func(t *testing.T) {
+		t.Parallel()
+
+		layout, err := hubgeometry.Resolve(hub)
+		if err != nil {
+			t.Fatalf("Resolve() error = %v; want nil", err)
+		}
+
+		junctions := layout.HostJunctionsHere()
+		if len(junctions) != 2 {
+			t.Fatalf("HostJunctionsHere() returned %d junctions; want 2", len(junctions))
+		}
+
+		lyxJunction := junctions[0]
+		wantLyxLink := layout.HostLyxLinkHere()
+		wantLyxTarget := layout.WeftLyxDir()
+		if lyxJunction.Name != "_lyx" {
+			t.Errorf("HostJunctionsHere()[0].Name = %q; want %q", lyxJunction.Name, "_lyx")
+		}
+		if lyxJunction.Link != wantLyxLink {
+			t.Errorf("HostJunctionsHere()[0].Link = %q; want %q", lyxJunction.Link, wantLyxLink)
+		}
+		if lyxJunction.Target != wantLyxTarget {
+			t.Errorf("HostJunctionsHere()[0].Target = %q; want %q", lyxJunction.Target, wantLyxTarget)
+		}
+
+		patternJunction := junctions[1]
+		wantPatternLink := layout.HostPatternLinkHere()
+		wantPatternTarget := layout.WeftPatternDir()
+		if patternJunction.Name != "_pattern" {
+			t.Errorf("HostJunctionsHere()[1].Name = %q; want %q", patternJunction.Name, "_pattern")
+		}
+		if patternJunction.Link != wantPatternLink {
+			t.Errorf("HostJunctionsHere()[1].Link = %q; want %q", patternJunction.Link, wantPatternLink)
+		}
+		if patternJunction.Target != wantPatternTarget {
+			t.Errorf("HostJunctionsHere()[1].Target = %q; want %q", patternJunction.Target, wantPatternTarget)
+		}
+	})
+
+	t.Run("at nested subpath", func(t *testing.T) {
+		t.Parallel()
+
+		subDir := filepath.Join(hub, "services", "api")
+		if err := os.MkdirAll(subDir, 0755); err != nil {
+			t.Fatalf("failed to create subdir: %v", err)
+		}
+
+		layout, err := hubgeometry.Resolve(subDir)
+		if err != nil {
+			t.Fatalf("Resolve() error = %v; want nil", err)
+		}
+
+		junctions := layout.HostJunctionsHere()
+		if len(junctions) != 2 {
+			t.Fatalf("HostJunctionsHere() returned %d junctions; want 2", len(junctions))
+		}
+
+		lyxJunction := junctions[0]
+		wantLyxLink := layout.HostLyxLinkHere()
+		wantLyxTarget := layout.WeftLyxDir()
+		if lyxJunction.Link != wantLyxLink {
+			t.Errorf("HostJunctionsHere()[0].Link = %q; want %q", lyxJunction.Link, wantLyxLink)
+		}
+		if lyxJunction.Target != wantLyxTarget {
+			t.Errorf("HostJunctionsHere()[0].Target = %q; want %q", lyxJunction.Target, wantLyxTarget)
+		}
+
+		patternJunction := junctions[1]
+		wantPatternLink := layout.HostPatternLinkHere()
+		wantPatternTarget := layout.WeftPatternDir()
+		if patternJunction.Link != wantPatternLink {
+			t.Errorf("HostJunctionsHere()[1].Link = %q; want %q", patternJunction.Link, wantPatternLink)
+		}
+		if patternJunction.Target != wantPatternTarget {
+			t.Errorf("HostJunctionsHere()[1].Target = %q; want %q", patternJunction.Target, wantPatternTarget)
 		}
 	})
+
+	t.Run("agrees with HostJunctions when slug matches current worktree", func(t *testing.T) {
+		t.Parallel()
+
+		layout, err := hubgeometry.Resolve(hub)
+		if err != nil {
+			t.Fatalf("Resolve() error = %v; want nil", err)
+		}
+
+		// The current worktree's own base name is the slug that makes HostJunctions(slug)
+		// resolve to the same host worktree HostJunctionsHere() is already anchored at.
+		slug := filepath.Base(layout.WorktreeRoot)
+
+		here := layout.HostJunctionsHere()
+		bySlug := layout.HostJunctions(slug)
+
+		if len(here) != len(bySlug) {
+			t.Fatalf("HostJunctionsHere() returned %d junctions; HostJunctions(%q) returned %d", len(here), slug, len(bySlug))
+		}
+		for i := range here {
+			if here[i] != bySlug[i] {
+				t.Errorf("HostJunctionsHere()[%d] = %+v; HostJunctions(%q)[%d] = %+v", i, here[i], slug, i, bySlug[i])
+			}
+		}
+	})
+}
+
+// TestIsReservedHubName_Pattern pins _pattern into the reserved-name set alongside
+// _lyx, _raddle, _board, _portals, and _launchers (see geometry_test.go's
+// TestIsReservedHubName for the full table): a worktree slug must never claim the
+// PATTERN constraint-injection surface's directory name.
+func TestIsReservedHubName_Pattern(t *testing.T) {
+	t.Parallel()
+
+	if got := hubgeometry.IsReservedHubName("_pattern"); !got {
+		t.Errorf("IsReservedHubName(%q) = %v; want true", "_pattern", got)
+	}
 }
diff --git a/internal/hubgeometry/pattern_test.go b/internal/hubgeometry/pattern_test.go
new file mode 100644
index 00000000..54a5e088
--- /dev/null
+++ b/internal/hubgeometry/pattern_test.go
@@ -0,0 +1,156 @@
+// pattern_test.go covers the _pattern geometry surface: the PatternDirName constant,
+// the free PatternDir/PatternFile helpers, and the six Layout accessors that mirror
+// their existing _lyx counterparts. Every case here is pure filepath.Join arithmetic —
+// no subprocess is spawned and no fixture tree is copied — so this file stays untagged.
+
+package hubgeometry_test
+
+import (
+	"path/filepath"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
+)
+
+// newTestLayout builds a Layout by hand for join-arithmetic assertions, mirroring the
+// field derivation Resolve performs (RelPath = filepath.Rel(WorktreeRoot, Cwd)) without
+// spawning git, since this file is deliberately untagged.
+func newTestLayout(hub, worktreeRoot, relPath string) *hubgeometry.Layout {
+	return &hubgeometry.Layout{
+		Cwd:          filepath.Join(worktreeRoot, relPath),
+		WorktreeRoot: worktreeRoot,
+		Hub:          hub,
+		RelPath:      relPath,
+		Prime:        worktreeRoot,
+		Repo:         filepath.Base(worktreeRoot),
+	}
+}
+
+// TestPatternDir_Free asserts PatternDir(baseDir)'s join.
+func TestPatternDir_Free(t *testing.T) {
+	tests := []struct {
+		name    string
+		baseDir string
+	}{
+		{"root base", filepath.Join("C:", "hub", "wt")},
+		{"nested base", filepath.Join("C:", "hub", "wt", "services", "api")},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := hubgeometry.PatternDir(tt.baseDir)
+			want := filepath.Join(tt.baseDir, "_pattern")
+			if got != want {
+				t.Errorf("PatternDir(%q) = %q; want %q", tt.baseDir, got, want)
+			}
+		})
+	}
+}
+
+// TestPatternFile_Free asserts PatternFile(baseDir)'s join.
+func TestPatternFile_Free(t *testing.T) {
+	tests := []struct {
+		name    string
+		baseDir string
+	}{
+		{"root base", filepath.Join("C:", "hub", "wt")},
+		{"nested base", filepath.Join("C:", "hub", "wt", "services", "api")},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := hubgeometry.PatternFile(tt.baseDir)
+			want := filepath.Join(tt.baseDir, "_pattern", "PATTERN.md")
+			if got != want {
+				t.Errorf("PatternFile(%q) = %q; want %q", tt.baseDir, got, want)
+			}
+		})
+	}
+}
+
+// TestLayout_PatternAccessors asserts each _pattern Layout accessor's join for both
+// RelPath == "." and a nested RelPath of at least two segments.
+func TestLayout_PatternAccessors(t *testing.T) {
+	hub := filepath.Join("C:", "hub")
+	worktreeRoot := filepath.Join(hub, "wt")
+	slug := "test-slug"
+
+	relPaths := []struct {
+		name    string
+		relPath string
+	}{
+		{"at root", "."},
+		{"nested two segments", filepath.Join("services", "api")},
+	}
+
+	for _, rp := range relPaths {
+		t.Run(rp.name, func(t *testing.T) {
+			l := newTestLayout(hub, worktreeRoot, rp.relPath)
+
+			t.Run("WeftPatternDir", func(t *testing.T) {
+				got := l.WeftPatternDir()
+				want := filepath.Join(l.WeftWorktree(), l.RelPath, "_pattern")
+				if got != want {
+					t.Errorf("WeftPatternDir() = %q; want %q", got, want)
+				}
+			})
+
+			t.Run("WeftPatternDirFor", func(t *testing.T) {
+				got := l.WeftPatternDirFor(slug)
+				want := filepath.Join(l.WeftWorktreePath(slug), l.RelPath, "_pattern")
+				if got != want {
+					t.Errorf("WeftPatternDirFor(%q) = %q; want %q", slug, got, want)
+				}
+			})
+
+			t.Run("HostPatternLink", func(t *testing.T) {
+				got := l.HostPatternLink(slug)
+				want := filepath.Join(l.WorktreePath(slug), l.RelPath, "_pattern")
+				if got != want {
+					t.Errorf("HostPatternLink(%q) = %q; want %q", slug, got, want)
+				}
+			})
+
+			t.Run("HostPatternLinkHere", func(t *testing.T) {
+				got := l.HostPatternLinkHere()
+				want := filepath.Join(l.WorktreeRoot, l.RelPath, "_pattern")
+				if got != want {
+					t.Errorf("HostPatternLinkHere() = %q; want %q", got, want)
+				}
+			})
+
+			t.Run("PatternFileHere", func(t *testing.T) {
+				got := l.PatternFileHere()
+				want := hubgeometry.PatternFile(filepath.Join(l.WorktreeRoot, l.RelPath))
+				if got != want {
+					t.Errorf("PatternFileHere() = %q; want %q", got, want)
+				}
+			})
+		})
+	}
+}
+
+// TestPatternFileHere_EqualsPatternFileOfCwd pins the equality PatternFileHere() relies
+// on: for any Layout whose RelPath was derived as filepath.Rel(WorktreeRoot, Cwd) — the
+// way Resolve derives it — PatternFileHere() equals PatternFile(l.Cwd) exactly, since
+// filepath.Join(WorktreeRoot, RelPath) collapses to Cwd itself.
+func TestPatternFileHere_EqualsPatternFileOfCwd(t *testing.T) {
+	tests := []struct {
+		name    string
+		relPath string
+	}{
+		{"at root", "."},
+		{"nested two segments", filepath.Join("services", "api")},
+	}
+
+	worktreeRoot := filepath.Join("C:", "hub", "wt")
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			l := newTestLayout(filepath.Join("C:", "hub"), worktreeRoot, tt.relPath)
+
+			got := l.PatternFileHere()
+			want := hubgeometry.PatternFile(l.Cwd)
+			if got != want {
+				t.Errorf("PatternFileHere() = %q; want PatternFile(l.Cwd) = %q", got, want)
+			}
+		})
+	}
+}
diff --git a/internal/hubgeometry/weft_test.go b/internal/hubgeometry/weft_test.go
index f82c75ec..da3f7649 100644
--- a/internal/hubgeometry/weft_test.go
+++ b/internal/hubgeometry/weft_test.go
@@ -38,11 +38,11 @@ func TestWeftGeometryMethods(t *testing.T) {
 			wantWeftRepoRoot:     filepath.Join("/h", "main-weft"),
 			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
 			wantWeftWorktreePath: filepath.Join("/h", "x-weft"),
-			wantWeftLyxDir:    filepath.Join("/h", "feat-weft", "_lyx"),
-			wantWeftLyxDirFor: filepath.Join("/h", "x-weft", "_lyx"),
-			wantWeftRaddleDir: filepath.Join("/h", "feat-weft", "_raddle"),
-			wantHostLyxLink:   filepath.Join("/h", "x", "_lyx"),
-			wantHostLyxLinkHere: filepath.Join("/h", "feat", "_lyx"),
+			wantWeftLyxDir:       filepath.Join("/h", "feat-weft", "_lyx"),
+			wantWeftLyxDirFor:    filepath.Join("/h", "x-weft", "_lyx"),
+			wantWeftRaddleDir:    filepath.Join("/h", "feat-weft", "_raddle"),
+			wantHostLyxLink:      filepath.Join("/h", "x", "_lyx"),
+			wantHostLyxLinkHere:  filepath.Join("/h", "feat", "_lyx"),
 		},
 		{
 			name:                 "/h /h/main feat sub case",
@@ -53,11 +53,11 @@ func TestWeftGeometryMethods(t *testing.T) {
 			wantWeftRepoRoot:     filepath.Join("/h", "main-weft"),
 			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
 			wantWeftWorktreePath: filepath.Join("/h", "x-weft"),
-			wantWeftLyxDir:    filepath.Join("/h", "feat-weft", "sub", "_lyx"),
-			wantWeftLyxDirFor: filepath.Join("/h", "x-weft", "sub", "_lyx"),
-			wantWeftRaddleDir: filepath.Join("/h", "feat-weft", "sub", "_raddle"),
-			wantHostLyxLink:   filepath.Join("/h", "x", "sub", "_lyx"),
-			wantHostLyxLinkHere: filepath.Join("/h", "feat", "sub", "_lyx"),
+			wantWeftLyxDir:       filepath.Join("/h", "feat-weft", "sub", "_lyx"),
+			wantWeftLyxDirFor:    filepath.Join("/h", "x-weft", "sub", "_lyx"),
+			wantWeftRaddleDir:    filepath.Join("/h", "feat-weft", "sub", "_raddle"),
+			wantHostLyxLink:      filepath.Join("/h", "x", "sub", "_lyx"),
+			wantHostLyxLinkHere:  filepath.Join("/h", "feat", "sub", "_lyx"),
 		},
 		{
 			name:                 "/h /h/main feat sub/dir case",
@@ -68,11 +68,11 @@ func TestWeftGeometryMethods(t *testing.T) {
 			wantWeftRepoRoot:     filepath.Join("/h", "main-weft"),
 			wantWeftWorktree:     filepath.Join("/h", "feat-weft"),
 			wantWeftWorktreePath: filepath.Join("/h", "y-weft"),
-			wantWeftLyxDir:    filepath.Join("/h", "feat-weft", "sub/dir", "_lyx"),
-			wantWeftLyxDirFor: filepath.Join("/h", "y-weft", "sub/dir", "_lyx"),
-			wantWeftRaddleDir: filepath.Join("/h", "feat-weft", "sub/dir", "_raddle"),
-			wantHostLyxLink:   filepath.Join("/h", "y", "sub/dir", "_lyx"),
-			wantHostLyxLinkHere: filepath.Join("/h", "feat", "sub/dir", "_lyx"),
+			wantWeftLyxDir:       filepath.Join("/h", "feat-weft", "sub/dir", "_lyx"),
+			wantWeftLyxDirFor:    filepath.Join("/h", "y-weft", "sub/dir", "_lyx"),
+			wantWeftRaddleDir:    filepath.Join("/h", "feat-weft", "sub/dir", "_raddle"),
+			wantHostLyxLink:      filepath.Join("/h", "y", "sub/dir", "_lyx"),
+			wantHostLyxLinkHere:  filepath.Join("/h", "feat", "sub/dir", "_lyx"),
 		},
 	}
 
@@ -207,8 +207,9 @@ func TestWeftGeometryAtMainWorktree(t *testing.T) {
 	}
 }
 
-// TestHostJunctions verifies that HostJunctions(slug) returns exactly one entry with
-// the correct Name, Link, and Target fields, and that no entry's Name equals _raddle.
+// TestHostJunctions verifies that HostJunctions(slug) returns exactly two entries, _lyx
+// first then _pattern, with the correct Name, Link, and Target fields for each, at
+// RelPath == "." and at a nested RelPath, and that no entry's Name equals _raddle.
 func TestHostJunctions(t *testing.T) {
 	tests := []struct {
 		name    string
@@ -218,7 +219,7 @@ func TestHostJunctions(t *testing.T) {
 		relPath string
 		// Expected junction values
 		wantJunctionCount int
-		wantName          string
+		wantNames         []string
 	}{
 		{
 			name:              "prime-derived layout, root case",
@@ -226,8 +227,8 @@ func TestHostJunctions(t *testing.T) {
 			prime:             "/h/main",
 			slug:              "feat",
 			relPath:           ".",
-			wantJunctionCount: 1,
-			wantName:          "_lyx",
+			wantJunctionCount: 2,
+			wantNames:         []string{"_lyx", "_pattern"},
 		},
 		{
 			name:              "non-prime worktree layout, root case",
@@ -235,8 +236,8 @@ func TestHostJunctions(t *testing.T) {
 			prime:             "/h/main",
 			slug:              "other",
 			relPath:           ".",
-			wantJunctionCount: 1,
-			wantName:          "_lyx",
+			wantJunctionCount: 2,
+			wantNames:         []string{"_lyx", "_pattern"},
 		},
 		{
 			name:              "subpath case",
@@ -244,8 +245,8 @@ func TestHostJunctions(t *testing.T) {
 			prime:             "/h/main",
 			slug:              "feat",
 			relPath:           "sub",
-			wantJunctionCount: 1,
-			wantName:          "_lyx",
+			wantJunctionCount: 2,
+			wantNames:         []string{"_lyx", "_pattern"},
 		},
 	}
 
@@ -263,28 +264,35 @@ func TestHostJunctions(t *testing.T) {
 
 			// Verify count
 			if len(junctions) != tt.wantJunctionCount {
-				t.Errorf("HostJunctions(%q) returned %d entries; want %d", tt.slug, len(junctions), tt.wantJunctionCount)
+				t.Fatalf("HostJunctions(%q) returned %d entries; want %d", tt.slug, len(junctions), tt.wantJunctionCount)
 			}
 
-			// Verify the single entry
-			if len(junctions) > 0 {
-				j := junctions[0]
-
-				if j.Name != tt.wantName {
-					t.Errorf("HostJunctions(%q)[0].Name = %q; want %q", tt.slug, j.Name, tt.wantName)
-				}
-
-				// Verify Link matches HostLyxLink(slug)
-				wantLink := layout.HostLyxLink(tt.slug)
-				if j.Link != wantLink {
-					t.Errorf("HostJunctions(%q)[0].Link = %q; want %q", tt.slug, j.Link, wantLink)
-				}
+			// Verify the _lyx entry (index 0)
+			lyxJunction := junctions[0]
+			if lyxJunction.Name != tt.wantNames[0] {
+				t.Errorf("HostJunctions(%q)[0].Name = %q; want %q", tt.slug, lyxJunction.Name, tt.wantNames[0])
+			}
+			wantLyxLink := layout.HostLyxLink(tt.slug)
+			if lyxJunction.Link != wantLyxLink {
+				t.Errorf("HostJunctions(%q)[0].Link = %q; want %q", tt.slug, lyxJunction.Link, wantLyxLink)
+			}
+			wantLyxTarget := layout.WeftLyxDirFor(tt.slug)
+			if lyxJunction.Target != wantLyxTarget {
+				t.Errorf("HostJunctions(%q)[0].Target = %q; want %q", tt.slug, lyxJunction.Target, wantLyxTarget)
+			}
 
-				// Verify Target matches WeftLyxDirFor(slug)
-				wantTarget := layout.WeftLyxDirFor(tt.slug)
-				if j.Target != wantTarget {
-					t.Errorf("HostJunctions(%q)[0].Target = %q; want %q", tt.slug, j.Target, wantTarget)
-				}
+			// Verify the _pattern entry (index 1)
+			patternJunction := junctions[1]
+			if patternJunction.Name != tt.wantNames[1] {
+				t.Errorf("HostJunctions(%q)[1].Name = %q; want %q", tt.slug, patternJunction.Name, tt.wantNames[1])
+			}
+			wantPatternLink := layout.HostPatternLink(tt.slug)
+			if patternJunction.Link != wantPatternLink {
+				t.Errorf("HostJunctions(%q)[1].Link = %q; want %q", tt.slug, patternJunction.Link, wantPatternLink)
+			}
+			wantPatternTarget := layout.WeftPatternDirFor(tt.slug)
+			if patternJunction.Target != wantPatternTarget {
+				t.Errorf("HostJunctions(%q)[1].Target = %q; want %q", tt.slug, patternJunction.Target, wantPatternTarget)
 			}
 		})
 	}
diff --git a/internal/initcli/initcli.go b/internal/initcli/initcli.go
index 4811a36c..9d8ea159 100644
--- a/internal/initcli/initcli.go
+++ b/internal/initcli/initcli.go
@@ -33,24 +33,30 @@ import (
 func Command() *cobra.Command {
 	initCmd := &cobra.Command{
 		Use:   "init",
-		Short: "scaffold _lyx/config/ in the current directory (or reverse it with --undo)",
+		Short: "wire the _lyx and _pattern junctions and scaffold _lyx/config/ (or reverse it with --undo)",
 		Long: `init activates the lyx topology for the current worktree.
 
-It wires cwd-keyed fabric junctions, creates _lyx/ and _lyx/config/ directories,
-maintains the managed .gitignore block for .lyx/, and reconciles all module
-config files against their templates (idempotent: existing user edits are
-preserved). A weft pairing must already exist (run 'lyx fabric add' or
-'lyx fabric clone' first).
+It wires both cwd-keyed fabric junctions (_lyx and _pattern), creates the
+_lyx/, _lyx/config/, and _pattern/ directories, maintains the managed
+.gitignore block for .lyx/, and reconciles all module config files against
+their templates (idempotent: existing user edits are preserved). A weft
+pairing must already exist (run 'lyx fabric add' or 'lyx fabric clone' first).
 
-Pass --undo to reverse a previous init: this removes the host _lyx junction,
-clears the weft-side _lyx content (committing and pushing the deletion),
-and reverts the managed .gitignore block and the .git/info/exclude entry
-that init added. --undo is safe to run on a directory that was never
-initialized (a clean no-op) and is mainly useful for test/sandbox cleanup.
+Pass --undo to reverse a previous init: this removes both host junctions and
+clears the weft-side _lyx content, committing and pushing that deletion.
+Weft _pattern content is deliberately preserved — it is the host repo's own
+hand-authored invariants, not lyx's runtime state — and is never cleared,
+committed, or pushed by --undo. --undo also reverts the managed .gitignore
+block and the .git/info/exclude entries that init added. --undo is safe to
+run on a directory that was never initialized (a clean no-op) and is mainly
+useful for test/sandbox cleanup.
+
+Breaking change: --undo's JSON output now reports "junctions_removed" (a
+list of junction names) in place of the old singular "lyx_junction" key.
 
   lyx init --undo`,
 	}
-	initCmd.Flags().Bool("undo", false, "reverse a previous init: remove the _lyx junction, weft-side content, and the .gitignore/.git-exclude entries it added")
+	initCmd.Flags().Bool("undo", false, "reverse a previous init: remove every host junction, weft-side content, and the .gitignore/.git-exclude entries it added")
 	initCmd.RunE = clihelp.WrapRun(func(out io.Writer, args []string) int {
 		undo, _ := initCmd.Flags().GetBool("undo")
 		if undo {
@@ -95,16 +101,20 @@ func runInit(out io.Writer, args []string) int {
 	}
 
 	return output.Ok(out, map[string]any{
-		"lyx_dir":   result.LyxDir,
-		"gitignore": result.Gitignore,
-		"modules":   modules,
+		"lyx_dir":     result.LyxDir,
+		"pattern_dir": result.PatternDir,
+		"gitignore":   result.Gitignore,
+		"modules":     modules,
 	})
 }
 
 // runUndo is the package-private handler for `lyx init --undo`.
 //
 // It resolves cwd and delegates the actual reversal to initengine.Undo, then
-// formats the result as the JSON output envelope.
+// formats the result as the JSON output envelope. The emitted "junctions_removed"
+// key carries the JunctionsRemoved slice — a breaking change from the prior
+// singular "lyx_junction" key, since a slice is the only shape that scales to
+// more than one junction.
 func runUndo(out io.Writer, args []string) int {
 	cwd, err := hubgeometry.Getwd()
 	if err != nil {
@@ -117,9 +127,9 @@ func runUndo(out io.Writer, args []string) int {
 	}
 
 	return output.Ok(out, map[string]any{
-		"lyx_junction": result.LyxJunction,
-		"weft_content": result.WeftContent,
-		"git_exclude":  result.GitExclude,
-		"gitignore":    result.Gitignore,
+		"junctions_removed": result.JunctionsRemoved,
+		"weft_content":      result.WeftContent,
+		"git_exclude":       result.GitExclude,
+		"gitignore":         result.Gitignore,
 	})
 }
diff --git a/internal/initcli/initcli_test.go b/internal/initcli/initcli_test.go
index ca20778e..edccf363 100644
--- a/internal/initcli/initcli_test.go
+++ b/internal/initcli/initcli_test.go
@@ -36,7 +36,7 @@ func TestRunInit_Smoke(t *testing.T) {
 	if ok, _ := result["ok"].(bool); !ok {
 		t.Error("ok flag is not true")
 	}
-	for _, key := range []string{"lyx_dir", "gitignore", "modules"} {
+	for _, key := range []string{"lyx_dir", "pattern_dir", "gitignore", "modules"} {
 		if _, present := result[key]; !present {
 			t.Errorf("result missing %q key; output: %s", key, buf.String())
 		}
@@ -71,7 +71,7 @@ func TestRunInit_UndoFlagDispatch(t *testing.T) {
 	if ok, _ := result["ok"].(bool); !ok {
 		t.Errorf("ok flag is not true; output: %s", buf2.String())
 	}
-	for _, key := range []string{"lyx_junction", "weft_content", "git_exclude", "gitignore"} {
+	for _, key := range []string{"junctions_removed", "weft_content", "git_exclude", "gitignore"} {
 		if _, present := result[key]; !present {
 			t.Errorf("result missing %q key; output: %s", key, buf2.String())
 		}
diff --git a/internal/initengine/init.go b/internal/initengine/init.go
index 086ecf67..42f23487 100644
--- a/internal/initengine/init.go
+++ b/internal/initengine/init.go
@@ -29,19 +29,21 @@ type ModuleResult struct {
 
 // InitResult summarizes what Init changed.
 type InitResult struct {
-	LyxDir    string // "created" or "exists"
-	Gitignore string // "updated" or "unchanged"
-	Modules   []ModuleResult
+	LyxDir     string // "created" or "exists"
+	PatternDir string // "created" or "exists"
+	Gitignore  string // "updated" or "unchanged"
+	Modules    []ModuleResult
 }
 
 // Init activates the fabric topology by wiring cwd-keyed junctions, then
 // reconciles the config layer in cwd by:
 //  1. Resolving the layout from cwd
 //  2. Checking for a weft pairing; if absent, returning an error early
-//  3. Wiring the host _lyx junction via fabricengine.WireJunctions
-//  4. Creating _lyx and _lyx/config directories
-//  5. Maintaining the managed .gitignore block for .lyx/
-//  6. Reconciling all module config files against their templates via ReconcileAll
+//  3. Observing whether each HOST junction path already exists, BEFORE wiring
+//  4. Wiring the host junctions via fabricengine.WireJunctions
+//  5. Creating _lyx and _pattern directories (and _lyx/config)
+//  6. Maintaining the managed .gitignore block for .lyx/
+//  7. Reconciling all module config files against their templates via ReconcileAll
 //
 // Idempotent: junction wiring is idempotent (via fslink.IsLink/PointsTo); a second
 // run does not clobber existing config files (Reconcile preserves user values) and
@@ -62,33 +64,47 @@ func Init(cwd string) (InitResult, error) {
 		return InitResult{}, fmt.Errorf("no weft pairing — run `lyx fabric add` or `lyx fabric clone` first")
 	}
 
-	// Wire junctions for the current worktree (keyed by its slug: filepath.Base(WorktreeRoot)).
 	slug := filepath.Base(l.WorktreeRoot)
+	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
+	patternDir := filepath.Join(cwd, hubgeometry.PatternDirName)
+
+	// Observe both HOST junction paths BEFORE WireJunctions runs. Since batch 3,
+	// WireJunctions' seeder unconditionally materialises each junction's weft-side
+	// target (os.MkdirAll), so a POST-wiring stat of the host junction path always
+	// succeeds through the freshly-created junction — silently reporting "exists"
+	// on a first-ever init. The host path itself is also the only reliable signal:
+	// the weft-side target cannot be used instead, because a weft branch forks from
+	// its parent's weft branch (see weftwiring.go) and so may already carry _lyx/
+	// content inherited from that history even though THIS host worktree's Init has
+	// never run. The host junction path, by contrast, is genuinely local to this
+	// worktree and does not exist until some Init call creates it.
+	lyxDirStatus, err := preWiringHostDirStatus(lyxDir)
+	if err != nil {
+		return InitResult{}, err
+	}
+	patternDirStatus, err := preWiringHostDirStatus(patternDir)
+	if err != nil {
+		return InitResult{}, err
+	}
+
+	// Wire junctions for the current worktree (keyed by its slug).
 	if err := fabricengine.WireJunctions(l, slug); err != nil {
 		return InitResult{}, fmt.Errorf("failed to wire junctions: %w", err)
 	}
 
 	var result InitResult
-
-	// Create _lyx directory (activation completed above).
-	lyxDir := filepath.Join(cwd, hubgeometry.LyxDirName)
-	info, err := os.Stat(lyxDir)
-	if err != nil && !os.IsNotExist(err) {
-		return InitResult{}, fmt.Errorf("failed to stat _lyx: %w", err)
+	result.LyxDir = lyxDirStatus
+	result.PatternDir = patternDirStatus
+
+	// Ensure the host _lyx and _pattern directories exist. Both are redundant with
+	// WireJunctions having just created a junction that resolves to a real
+	// directory, but keep Init self-contained even if a future caller's contract
+	// with WireJunctions ever changes.
+	if err := os.MkdirAll(lyxDir, 0o755); err != nil {
+		return InitResult{}, fmt.Errorf("failed to create _lyx directory: %w", err)
 	}
-
-	if os.IsNotExist(err) {
-		// Directory doesn't exist, create it.
-		if err := os.MkdirAll(lyxDir, 0o755); err != nil {
-			return InitResult{}, fmt.Errorf("failed to create _lyx directory: %w", err)
-		}
-		result.LyxDir = "created"
-	} else if info.IsDir() {
-		// Directory already exists.
-		result.LyxDir = "exists"
-	} else {
-		// Exists but is not a directory.
-		return InitResult{}, fmt.Errorf("_lyx exists but is not a directory")
+	if err := os.MkdirAll(patternDir, 0o755); err != nil {
+		return InitResult{}, fmt.Errorf("failed to create _pattern directory: %w", err)
 	}
 
 	// Create _lyx/config/ subdirectory to hold configuration files.
@@ -131,3 +147,28 @@ func Init(cwd string) (InitResult, error) {
 
 	return result, nil
 }
+
+// preWiringHostDirStatus reports whether the host junction path dir (e.g. cwd/_lyx
+// or cwd/_pattern) already exists, observed BEFORE WireJunctions runs. This is the
+// pre-wiring observation Init's InitResult vocabulary is built on: see Init's godoc
+// for why the host path — not the weft-side target — is the only reliable signal.
+//
+// Returns "created" if dir does not yet exist (this Init invocation is the one that
+// will bring it into being via WireJunctions), "exists" if it is already a
+// directory (a prior Init already wired it), or an error if dir exists but is not a
+// directory — a real, non-directory file occupying a path Init expects to be (or
+// become) a junction, which fabric must never silently paper over — or if the stat
+// itself fails for a reason other than not-exist.
+func preWiringHostDirStatus(dir string) (string, error) {
+	info, err := os.Stat(dir)
+	if err != nil {
+		if os.IsNotExist(err) {
+			return "created", nil
+		}
+		return "", fmt.Errorf("failed to stat %s: %w", filepath.Base(dir), err)
+	}
+	if !info.IsDir() {
+		return "", fmt.Errorf("%s exists but is not a directory", filepath.Base(dir))
+	}
+	return "exists", nil
+}
diff --git a/internal/initengine/init_test.go b/internal/initengine/init_test.go
index e192e7c8..01477eb8 100644
--- a/internal/initengine/init_test.go
+++ b/internal/initengine/init_test.go
@@ -31,12 +31,39 @@ func TestInit_FirstRun(t *testing.T) {
 		t.Fatalf("Init() = %v; want nil", err)
 	}
 
+	// This is the regression guard for the ordering fix (card 16's whole reason for
+	// existing): both LyxDir and PatternDir must report "created" on a genuine first
+	// run. Before the fix, WireJunctions' seeder had already materialised the
+	// weft-side target by the time the host path was stated, so LyxDir would
+	// silently (and incorrectly) report "exists" here.
+	if result.LyxDir != "created" {
+		t.Errorf("result.LyxDir = %q; want %q on first run", result.LyxDir, "created")
+	}
+	if result.PatternDir != "created" {
+		t.Errorf("result.PatternDir = %q; want %q on first run", result.PatternDir, "created")
+	}
+
 	// Verify _lyx/config/ directories exist
 	configDir := hubgeometry.ConfigDir(f.Layout.WorktreeRoot)
 	if _, err := os.Stat(configDir); err != nil {
 		t.Fatalf("_lyx/config not created: %v", err)
 	}
 
+	// Verify both junctions resolve and both weft directories exist.
+	slug := filepath.Base(f.Layout.WorktreeRoot)
+	if _, err := os.Stat(f.Layout.HostLyxLink(slug)); err != nil {
+		t.Errorf("host _lyx junction does not resolve: %v", err)
+	}
+	if _, err := os.Stat(f.Layout.HostPatternLink(slug)); err != nil {
+		t.Errorf("host _pattern junction does not resolve: %v", err)
+	}
+	if _, err := os.Stat(f.Layout.WeftLyxDirFor(slug)); err != nil {
+		t.Errorf("weft _lyx directory does not exist: %v", err)
+	}
+	if _, err := os.Stat(f.Layout.WeftPatternDirFor(slug)); err != nil {
+		t.Errorf("weft _pattern directory does not exist: %v", err)
+	}
+
 	// Verify both config files exist
 	for _, module := range []string{"board", "fabric"} {
 		cfgPath := hubgeometry.ConfigFile(f.Layout.WorktreeRoot, module)
@@ -132,6 +159,9 @@ func TestInit_Idempotent(t *testing.T) {
 	if result2.LyxDir != "exists" {
 		t.Errorf("result2.LyxDir = %q; want %q", result2.LyxDir, "exists")
 	}
+	if result2.PatternDir != "exists" {
+		t.Errorf("result2.PatternDir = %q; want %q", result2.PatternDir, "exists")
+	}
 	if result2.Gitignore != "unchanged" {
 		t.Errorf("result2.Gitignore = %q; want %q", result2.Gitignore, "unchanged")
 	}
diff --git a/internal/initengine/undo.go b/internal/initengine/undo.go
index 4b87f848..bd0f30b3 100644
--- a/internal/initengine/undo.go
+++ b/internal/initengine/undo.go
@@ -1,10 +1,13 @@
 // undo.go implements the core logic for lyx init --undo — the reversal of Init.
 //
-// Undo reverses exactly what Init wires: the host _lyx junction, the
-// weft-side _lyx content, the managed .gitignore block, and the
-// .git/info/exclude entry. Each step independently no-ops if its own target
-// is already absent, and a junction inconsistency aborts the whole run before
-// any weft-content or .gitignore step runs (see fabricengine.UnwireJunctions).
+// Undo reverses exactly what Init wires: every host junction, the weft-side
+// _lyx content, the managed .gitignore block, and the .git/info/exclude
+// entries. Each step independently no-ops if its own target is already absent,
+// and a junction inconsistency aborts the whole run before any weft-content or
+// .gitignore step runs (see fabricengine.UnwireJunctions).
+//
+// Weft _pattern content is deliberately NEVER touched by Undo — see Undo's
+// godoc for why.
 
 package initengine
 
@@ -19,8 +22,14 @@ import (
 
 // UndoResult summarizes what Undo changed.
 type UndoResult struct {
-	LyxJunction string // "removed" or "not_present"
-	WeftContent string // "cleared" or "not_present"
+	// JunctionsRemoved lists the Name of each host junction that was actually
+	// present and removed, carrying fabricengine.UnwireResult.JunctionsRemoved
+	// through unchanged. Empty when no junction was wired.
+	JunctionsRemoved []string
+	// WeftContent describes _lyx only — "cleared" or "not_present" — and NEVER
+	// _pattern: weft _pattern content is preserved by design (see Undo's
+	// godoc), so its presence or absence never contributes to this field.
+	WeftContent string
 	GitExclude  string // "reverted" or "unchanged"
 	Gitignore   string // "reverted" or "unchanged"
 }
@@ -30,12 +39,30 @@ type UndoResult struct {
 //     Init there is no "no weft pairing" pre-gate — each step below
 //     independently no-ops when its own target is absent).
 //  2. Derive slug from the worktree root (identical to Init).
-//  3. Unwire the host junction and its .git/info/exclude entry via
-//     fabricengine.UnwireJunctions. Any error here aborts immediately: no
-//     weft-content clearing or .gitignore revert runs.
-//  4. Clear weft-side _lyx content, if any weft worktree exists at all, then
-//     unconditionally commit and push that deletion through fabricengine.
+//  3. Unwire every host junction (both _lyx and _pattern) and their shared
+//     .git/info/exclude entries via fabricengine.UnwireJunctions. Any error
+//     here aborts immediately: no weft-content clearing or .gitignore
+//     revert runs.
+//  4. Clear weft-side _lyx content ONLY, if any weft worktree exists at all,
+//     then unconditionally commit and push that deletion through
+//     fabricengine. Weft _pattern content is deliberately NEVER cleared,
+//     committed, or pushed by this step, or by any other step of Undo.
 //  5. Revert the managed .gitignore block's ".lyx/" entry.
+//
+// The _lyx/_pattern asymmetry in step 4 is deliberate, not an oversight: step
+// 4 does os.RemoveAll, commits, and PUSHES the deletion, which is correct for
+// _lyx — lyx's own runtime state, owned entirely by fabric — and would be
+// badly wrong for _pattern, which holds the host repo's own hand-authored
+// constraint-injection content (PATTERN.md). Deactivating lyx must not
+// destroy the repo's own invariants and push that destruction to the remote,
+// where it cannot be casually undone. So while step 3 unwires BOTH junctions
+// (a junction is fabric-owned wiring metadata, never user content — removing
+// it is always safe, for either directory), step 4's RemoveAll/commit/push
+// sequence names only hubgeometry.LyxDirName: the os.RemoveAll target stays
+// l.WeftLyxDirFor(slug), the commit pathspec stays
+// fabricengine.ScopedPathspec(l.RelPath, []string{hubgeometry.LyxDirName}),
+// and the commit message stays _lyx-scoped. No _pattern equivalent exists,
+// and none should be added.
 func Undo(cwd string) (UndoResult, error) {
 	// Resolve layout from cwd (needed for weft sibling derivation and slug).
 	l, err := hubgeometry.Resolve(cwd)
@@ -47,9 +74,10 @@ func Undo(cwd string) (UndoResult, error) {
 
 	slug := filepath.Base(l.WorktreeRoot)
 
-	// Step 3: unwire the host junction and its exclude entry. Per the "any
-	// junction inconsistency is a hard error" Shared Decision, any error here
-	// aborts the whole run: no weft-content or .gitignore step runs.
+	// Step 3: unwire every host junction (both _lyx and _pattern) and their
+	// exclude entries. Per the "any junction inconsistency is a hard error"
+	// Shared Decision, any error here aborts the whole run: no weft-content
+	// or .gitignore step runs.
 	junctionResult, err := fabricengine.UnwireJunctions(l, slug)
 	if err != nil {
 		return UndoResult{}, err
@@ -57,12 +85,14 @@ func Undo(cwd string) (UndoResult, error) {
 
 	var result UndoResult
 
-	// Step 4: weft-side content. First check whether a weft worktree exists
-	// at all; if it doesn't, this is a truly-unpaired host (the same
-	// condition Init hard-gates on) and every remaining part of this step
-	// is skipped — in particular, fabricengine's CommitWeft must never be
-	// called against a nonexistent weft worktree, since its ensureLockDir
-	// would unconditionally create a stray <slug>-weft/.weft/ directory tree.
+	// Step 4: weft-side _lyx content ONLY — weft _pattern content is
+	// deliberately never touched here or anywhere else in Undo; see Undo's
+	// godoc for why. First check whether a weft worktree exists at all; if
+	// it doesn't, this is a truly-unpaired host (the same condition Init
+	// hard-gates on) and every remaining part of this step is skipped — in
+	// particular, fabricengine's CommitWeft must never be called against a
+	// nonexistent weft worktree, since its ensureLockDir would
+	// unconditionally create a stray <slug>-weft/.weft/ directory tree.
 	weftWorktree := l.WeftWorktree()
 	if _, statErr := os.Stat(weftWorktree); statErr != nil && !os.IsNotExist(statErr) {
 		return UndoResult{}, statErr
@@ -113,10 +143,7 @@ func Undo(cwd string) (UndoResult, error) {
 		result.Gitignore = "unchanged"
 	}
 
-	result.LyxJunction = "not_present"
-	if junctionResult.JunctionRemoved {
-		result.LyxJunction = "removed"
-	}
+	result.JunctionsRemoved = junctionResult.JunctionsRemoved
 	result.GitExclude = "unchanged"
 	if junctionResult.ExcludeChanged {
 		result.GitExclude = "reverted"
diff --git a/internal/initengine/undo_test.go b/internal/initengine/undo_test.go
index 12d24df4..d1dfa254 100644
--- a/internal/initengine/undo_test.go
+++ b/internal/initengine/undo_test.go
@@ -13,6 +13,7 @@ package initengine
 import (
 	"os"
 	"path/filepath"
+	"slices"
 	"sort"
 	"strings"
 	"testing"
@@ -112,8 +113,8 @@ func TestUndo_HappyPath(t *testing.T) {
 		t.Fatalf("Undo() = %v; want nil", err)
 	}
 
-	if result.LyxJunction != "removed" {
-		t.Errorf("result.LyxJunction = %q; want %q", result.LyxJunction, "removed")
+	if want := []string{hubgeometry.LyxDirName, hubgeometry.PatternDirName}; !slices.Equal(result.JunctionsRemoved, want) {
+		t.Errorf("result.JunctionsRemoved = %v; want %v", result.JunctionsRemoved, want)
 	}
 	if result.WeftContent != "cleared" {
 		t.Errorf("result.WeftContent = %q; want %q", result.WeftContent, "cleared")
@@ -125,10 +126,14 @@ func TestUndo_HappyPath(t *testing.T) {
 		t.Errorf("result.Gitignore = %q; want %q", result.Gitignore, "reverted")
 	}
 
-	// Host junction is gone.
-	hostLink := f.Layout.HostLyxLinkHere()
-	if _, statErr := os.Lstat(hostLink); !os.IsNotExist(statErr) {
-		t.Errorf("host junction %s still exists after Undo", hostLink)
+	// Both host junctions are gone.
+	hostLyxLink := f.Layout.HostLyxLinkHere()
+	if _, statErr := os.Lstat(hostLyxLink); !os.IsNotExist(statErr) {
+		t.Errorf("host _lyx junction %s still exists after Undo", hostLyxLink)
+	}
+	hostPatternLink := f.Layout.HostPatternLinkHere()
+	if _, statErr := os.Lstat(hostPatternLink); !os.IsNotExist(statErr) {
+		t.Errorf("host _pattern junction %s still exists after Undo", hostPatternLink)
 	}
 
 	// Weft-side _lyx directory is gone.
@@ -158,11 +163,63 @@ func TestUndo_HappyPath(t *testing.T) {
 		t.Errorf(".gitignore still contains the .lyx/ entry: %q", gitignoreContent)
 	}
 
-	// The .git/info/exclude line is gone.
+	// The .git/info/exclude lines for both junctions are gone.
 	excludeContent := readExcludeContent(t, f.Layout, filepath.Base(f.Layout.WorktreeRoot))
 	if excludeContainsLine(excludeContent, hubgeometry.LyxDirName) {
 		t.Errorf(".git/info/exclude still contains %q line after Undo", hubgeometry.LyxDirName)
 	}
+	if excludeContainsLine(excludeContent, hubgeometry.PatternDirName) {
+		t.Errorf(".git/info/exclude still contains %q line after Undo", hubgeometry.PatternDirName)
+	}
+}
+
+// TestUndo_PreservesPatternContent verifies the deliberate asymmetry between
+// _lyx and _pattern that Undo's godoc documents: weft _lyx content is cleared
+// (committed and pushed), but weft _pattern content — the host repo's own
+// hand-authored invariants — is left untouched on disk, and no deletion of it
+// is ever committed.
+func TestUndo_PreservesPatternContent(t *testing.T) {
+	f := lyxtest.CopyPairedLocal(t)
+	t.Setenv("WEFT_SKIP_PUSH", "1")
+
+	if _, err := Init(f.Layout.WorktreeRoot); err != nil {
+		t.Fatalf("Init() = %v; want nil", err)
+	}
+
+	// Seed a PATTERN.md under the weft _pattern directory, mirroring the
+	// host repo's own hand-authored constraint content.
+	weftPatternDir := f.Layout.WeftPatternDir()
+	patternFile := filepath.Join(weftPatternDir, "PATTERN.md")
+	if err := os.WriteFile(patternFile, []byte("# constraints\n"), 0o644); err != nil {
+		t.Fatalf("seed PATTERN.md: %v", err)
+	}
+	lyxtest.MustRun(t, f.Layout.WeftWorktree(), "git", "add", "--", hubgeometry.PatternDirName)
+	lyxtest.MustRun(t, f.Layout.WeftWorktree(), "git", "commit", "-m", "seed PATTERN.md")
+
+	result, err := Undo(f.Layout.WorktreeRoot)
+	if err != nil {
+		t.Fatalf("Undo() = %v; want nil", err)
+	}
+
+	if want := []string{hubgeometry.LyxDirName, hubgeometry.PatternDirName}; !slices.Equal(result.JunctionsRemoved, want) {
+		t.Errorf("result.JunctionsRemoved = %v; want %v", result.JunctionsRemoved, want)
+	}
+
+	// PATTERN.md survives on disk, untouched.
+	content := mustReadFile(t, patternFile)
+	if content != "# constraints\n" {
+		t.Errorf("PATTERN.md content changed after Undo: %q", content)
+	}
+
+	// No deletion of it was committed: the _pattern pathspec is clean in the
+	// weft worktree (nothing staged or committed against it by Undo).
+	stdout, _, exitCode, err := gitexec.RunGit([]string{"status", "--porcelain", "--", hubgeometry.PatternDirName}, f.Layout.WeftWorktree())
+	if err != nil || exitCode != 0 {
+		t.Fatalf("git status in weft worktree failed: %v (exit %d)", err, exitCode)
+	}
+	if strings.TrimSpace(stdout) != "" {
+		t.Errorf("weft worktree _pattern pathspec not clean after Undo: %q", stdout)
+	}
 }
 
 // TestUndo_NeverInitialized verifies that Undo is a clean no-op on a
@@ -187,8 +244,8 @@ func TestUndo_NeverInitialized(t *testing.T) {
 		t.Fatalf("Undo() = %v; want nil", err)
 	}
 
-	if result.LyxJunction != "not_present" {
-		t.Errorf("result.LyxJunction = %q; want %q", result.LyxJunction, "not_present")
+	if len(result.JunctionsRemoved) != 0 {
+		t.Errorf("result.JunctionsRemoved = %v; want empty", result.JunctionsRemoved)
 	}
 	if result.WeftContent != "not_present" {
 		t.Errorf("result.WeftContent = %q; want %q", result.WeftContent, "not_present")
@@ -256,8 +313,8 @@ func TestUndo_Idempotent(t *testing.T) {
 		t.Fatalf("second Undo() = %v; want nil", err)
 	}
 
-	if result.LyxJunction != "not_present" {
-		t.Errorf("result.LyxJunction = %q; want %q", result.LyxJunction, "not_present")
+	if len(result.JunctionsRemoved) != 0 {
+		t.Errorf("result.JunctionsRemoved = %v; want empty", result.JunctionsRemoved)
 	}
 	if result.WeftContent != "not_present" {
 		t.Errorf("result.WeftContent = %q; want %q", result.WeftContent, "not_present")
@@ -409,8 +466,9 @@ func TestUndo_PartialRecovery(t *testing.T) {
 			t.Fatalf("Init() = %v; want nil", err)
 		}
 
-		// Simulate a crash between removing the junction and clearing weft
-		// content: remove only the host junction, leaving weft content in place.
+		// Simulate a crash between removing the _lyx junction and clearing weft
+		// content: remove only the host _lyx junction, leaving the _pattern
+		// junction and weft content in place.
 		hostLink := f.Layout.HostLyxLinkHere()
 		if err := fslink.Remove(hostLink); err != nil {
 			t.Fatalf("remove host junction: %v", err)
@@ -421,8 +479,10 @@ func TestUndo_PartialRecovery(t *testing.T) {
 			t.Fatalf("recovery Undo() = %v; want nil", err)
 		}
 
-		if result.LyxJunction != "not_present" {
-			t.Errorf("result.LyxJunction = %q; want %q", result.LyxJunction, "not_present")
+		// _lyx was already removed by the simulated crash, so only _pattern is
+		// actually removed by this recovery Undo call.
+		if want := []string{hubgeometry.PatternDirName}; !slices.Equal(result.JunctionsRemoved, want) {
+			t.Errorf("result.JunctionsRemoved = %v; want %v", result.JunctionsRemoved, want)
 		}
 		if result.WeftContent != "cleared" {
 			t.Errorf("result.WeftContent = %q; want %q", result.WeftContent, "cleared")
@@ -454,14 +514,18 @@ func TestUndo_PartialRecovery(t *testing.T) {
 		// step 4 of Undo would do) but do not push, simulating a prior Undo
 		// run that committed locally but failed to push. Undo's step 3
 		// (junction removal) always runs before step 4, so a run that reached
-		// step 4 necessarily already removed the host junction too; mirror
+		// step 4 necessarily already removed BOTH host junctions too; mirror
 		// that here so the full Undo call below sees an already-clean
 		// junction step (no-op) rather than a corrupted one (the weft-side
 		// unwiring guard validates the weft-side target still exists before
 		// touching the link, which the deletion below removes).
-		hostLink := f.Layout.HostLyxLinkHere()
-		if err := fslink.Remove(hostLink); err != nil {
-			t.Fatalf("remove host junction: %v", err)
+		hostLyxLink := f.Layout.HostLyxLinkHere()
+		if err := fslink.Remove(hostLyxLink); err != nil {
+			t.Fatalf("remove host _lyx junction: %v", err)
+		}
+		hostPatternLink := f.Layout.HostPatternLinkHere()
+		if err := fslink.Remove(hostPatternLink); err != nil {
+			t.Fatalf("remove host _pattern junction: %v", err)
 		}
 		weftLyxDir := f.Layout.WeftLyxDir()
 		if err := os.RemoveAll(weftLyxDir); err != nil {
diff --git a/internal/loomengine/plan-template.md b/internal/loomengine/plan-template.md
index 8e5c8dea..78a36751 100644
--- a/internal/loomengine/plan-template.md
+++ b/internal/loomengine/plan-template.md
@@ -1,15 +1,19 @@
 <!-- This is the loom Plan producer's autonomous prompt. It is filled by
      composePlanPrompt (plan.go) via internal/stencil and handed to shuttle as
      the plan agent's entire instruction set. Every marker below is a
-     top-level {{.X}} substitution; stencil.Fill requires all three non-empty
-     and there are no {{if}}/{{range}} conditionals anywhere in this file (a
-     required marker inside a conditional branch would render silently blank
-     when present-but-empty — see internal/stencil/stencil.go). -->
+     top-level {{.X}} substitution; stencil.Fill requires the three original
+     ones non-empty and there are no {{if}}/{{range}} conditionals anywhere
+     in this file (a required marker inside a conditional branch would
+     render silently blank when present-but-empty — see
+     internal/stencil/stencil.go). pattern_directive is the fourth marker,
+     and the one optional one: it is filled via stencil.FillOptional and
+     renders as nothing when PATTERN is inactive. -->
 
 # Plan — read the decision record, write a plan-format-v3 flat-card plan
 
 You are the Plan producer: a single autonomous agent that reads the decision record and writes a plan-format-v3 flat-card plan. You never interview, never ask, and have no review logic of your own.
 
+{{.pattern_directive}}
 ## Step 1 — Read the decision record
 
 Read `{{.decision_record_path}}`. This is your **sole** input — never read the support log or the board. If the file is missing or empty, STOP and report that rather than inventing scope.
diff --git a/internal/loomengine/plan.go b/internal/loomengine/plan.go
index 13f4dc94..2b7b3705 100644
--- a/internal/loomengine/plan.go
+++ b/internal/loomengine/plan.go
@@ -27,25 +27,31 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/modelspec"
+	"github.com/Knatte18/loomyard/internal/pattern"
 	"github.com/Knatte18/loomyard/internal/shuttleengine"
 	"github.com/Knatte18/loomyard/internal/stencil"
 )
 
 // composePlanPrompt builds the Plan producer's prompt by composing the
-// template's three top-level marker values (the decision record path, the
-// plan directory the agent writes into, and the overview file path it must
-// write last) and filling planTemplate with them via stencil.Fill. Unlike
-// composePrompt, there is no mode-specific branch to compose: the Plan
-// producer is autonomous-only, so plan-template.md carries a single,
-// unconditional instruction set.
-func composePlanPrompt(decisionRecordPath, planDir, overviewPath string) ([]byte, error) {
+// template's three required top-level marker values (the decision record
+// path, the plan directory the agent writes into, and the overview file
+// path it must write last), plus patternDirective under the optional
+// pattern_directive marker, and filling planTemplate with them via
+// stencil.FillOptional. Unlike composePrompt, there is no mode-specific
+// branch to compose: the Plan producer is autonomous-only, so
+// plan-template.md carries a single, unconditional instruction set.
+// composePlanPrompt stays a pure string function — patternDirective is
+// computed one level up, in PlanSpec, which already holds the Layout this
+// function has no need for.
+func composePlanPrompt(decisionRecordPath, planDir, overviewPath, patternDirective string) ([]byte, error) {
 	values := map[string]string{
 		"decision_record_path": decisionRecordPath,
 		"plan_dir":             planDir,
 		"overview_path":        overviewPath,
+		"pattern_directive":    patternDirective,
 	}
 
-	rendered, err := stencil.Fill(planTemplate, values)
+	rendered, err := stencil.FillOptional(planTemplate, values, []string{"pattern_directive"})
 	if err != nil {
 		return nil, fmt.Errorf("loom: compose plan prompt: %w", err)
 	}
@@ -84,7 +90,14 @@ func PlanSpec(layout *hubgeometry.Layout, cfg Config, reg modelspec.Registry) (s
 	planDir := layout.PlanDir()
 	overviewPath := layout.PlanOverview()
 
-	prompt, err := composePlanPrompt(decisionRecordPath, planDir, overviewPath)
+	// RoleImplementer: the Plan producer authors the typed file-op
+	// instructions (Edits:/Creates:/Moves:/Requirements:) a later
+	// code-writing agent executes near-verbatim, so it is the last
+	// authoring point before code — an invariant missed here is carried
+	// into every card that inherits it, unlike the Discussion producer,
+	// which this task deliberately excludes.
+	directive := pattern.Directive(layout, pattern.RoleImplementer)
+	prompt, err := composePlanPrompt(decisionRecordPath, planDir, overviewPath, directive)
 	if err != nil {
 		return shuttleengine.Spec{}, fmt.Errorf("loom: PlanSpec: %w", err)
 	}
diff --git a/internal/loomengine/plan_test.go b/internal/loomengine/plan_test.go
index ea2432b8..92377dab 100644
--- a/internal/loomengine/plan_test.go
+++ b/internal/loomengine/plan_test.go
@@ -5,6 +5,7 @@
 package loomengine
 
 import (
+	"os"
 	"path/filepath"
 	"strings"
 	"testing"
@@ -97,6 +98,86 @@ func TestPlanSpec_PromptFilled(t *testing.T) {
 	}
 }
 
+// TestPlanSpec_PatternDirectiveOptional proves pattern_directive behaves as
+// an optional marker driven all the way through PlanSpec: an empty
+// directive (the common case — PATTERN inactive) renders with no leftover
+// "{{", no orphan "## Constraints" heading, and no stray blank-line block,
+// while a non-empty directive (PATTERN active) appears ahead of "## Step
+// 1". The two cases deliberately use DIFFERENT Layout fixtures. Every other
+// test in this file builds its Layout from a path that never exists on
+// disk (filepath.Join("home", "user", "repo")), which is fine for pure
+// string-shape assertions — but pattern.Directive performs a real
+// os.Stat on _pattern/PATTERN.md, so reusing that fake Layout here would
+// always render the directive empty and the non-empty case's placement
+// assertion would pass vacuously, proving nothing. The non-empty case
+// instead builds its Layout on a t.TempDir() with a real _pattern/PATTERN.md
+// seeded on disk. This is the one test in this file that touches the
+// filesystem — cards 24 through 27 inject pattern_directive directly as a
+// stencil value and never stat anything, since their templates are
+// exercised through stencil.FillOptional rather than through a
+// Layout-taking entry point; PlanSpec is Layout-taking, so this test is
+// the one place in the whole batch that must actually exercise
+// pattern.Directive's own os.Stat. t.TempDir() is not a banned token under
+// the Test Tier Purity Invariant, so this file stays untagged.
+func TestPlanSpec_PatternDirectiveOptional(t *testing.T) {
+	cfg := Config{Plan: "opus[effort=high]", PlanTimeoutMin: 120}
+
+	t.Run("empty pattern_directive (PATTERN inactive) renders cleanly", func(t *testing.T) {
+		// filepath.Join("home", "user", "repo") never exists on disk, so
+		// pattern.Directive's os.Stat always resolves "not exist" here —
+		// PATTERN is inactive by construction.
+		layout := &hubgeometry.Layout{WorktreeRoot: filepath.Join("home", "user", "repo")}
+
+		reg, err := modelspec.LoadRegistry(t.TempDir())
+		if err != nil {
+			t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
+		}
+		spec, err := PlanSpec(layout, cfg, reg)
+		if err != nil {
+			t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
+		}
+
+		prompt := spec.Prompt
+		if strings.Contains(prompt, "{{") {
+			t.Errorf("PlanSpec(...).Prompt contains a leftover {{: %q", prompt)
+		}
+		if strings.Contains(prompt, "## Constraints") {
+			t.Errorf("PlanSpec(...).Prompt contains an orphan ## Constraints heading: %q", prompt)
+		}
+		if strings.Contains(prompt, "\n\n\n\n") {
+			t.Errorf("PlanSpec(...).Prompt contains a stray blank-line block: %q", prompt)
+		}
+	})
+
+	t.Run("non-empty pattern_directive (PATTERN active) precedes Step 1", func(t *testing.T) {
+		worktreeRoot := t.TempDir()
+		patternDir := filepath.Join(worktreeRoot, "_pattern")
+		if err := os.MkdirAll(patternDir, 0o755); err != nil {
+			t.Fatalf("MkdirAll(%q) = %v; want nil", patternDir, err)
+		}
+		if err := os.WriteFile(filepath.Join(patternDir, "PATTERN.md"), []byte("# PATTERN\n"), 0o644); err != nil {
+			t.Fatalf("WriteFile(PATTERN.md) = %v; want nil", err)
+		}
+		layout := &hubgeometry.Layout{WorktreeRoot: worktreeRoot}
+
+		reg, err := modelspec.LoadRegistry(t.TempDir())
+		if err != nil {
+			t.Fatalf("modelspec.LoadRegistry(t.TempDir()) = _, %v; want nil error", err)
+		}
+		spec, err := PlanSpec(layout, cfg, reg)
+		if err != nil {
+			t.Fatalf("PlanSpec(...) = _, %v; want nil error", err)
+		}
+
+		prompt := spec.Prompt
+		directiveIdx := strings.Index(prompt, "## Constraints")
+		stepIdx := strings.Index(prompt, "## Step 1")
+		if directiveIdx == -1 || stepIdx == -1 || directiveIdx >= stepIdx {
+			t.Errorf("pattern_directive (idx %d) does not precede ## Step 1 (idx %d) in prompt: %q", directiveIdx, stepIdx, prompt)
+		}
+	})
+}
+
 // TestPlanSpec_PromptStatesCardCriteria verifies the rendered prompt
 // carries plan-format-v3's card-granularity contract ("What a card is"),
 // not just the field format: a live run against a template without these
diff --git a/internal/loomengine/preflight.go b/internal/loomengine/preflight.go
index dec1201f..6ca9277b 100644
--- a/internal/loomengine/preflight.go
+++ b/internal/loomengine/preflight.go
@@ -122,11 +122,25 @@ func checkResolved(l *hubgeometry.Layout) (Report, error) {
 			return Report{}, err
 		}
 		if !ok {
+			// PairInSync's junction reasons are a consumed string format: all
+			// three now read "host <name> junction …" or "host <name> is not a
+			// junction" (fabricengine's junction-name parameterisation), so a
+			// prefix match on "junction" no longer catches any of them — only
+			// a substring match does. Any future reword of those reasons must
+			// keep the substring "junction" in them, or this classification
+			// silently reverts to CheckWeftSync.
+			//
+			// Order matters: the "host on " case is checked first so the
+			// branch-mismatch reason ("host on <a>, weft on <b> (want <c>)")
+			// is classified before the broader Contains check runs — relying
+			// on that ordering is the safer arrangement, even though the
+			// branch-mismatch reason's content alone never contains
+			// "junction" either way.
 			var check CheckID
 			switch {
 			case strings.HasPrefix(reason, "host on "):
 				check = CheckWeftSync
-			case strings.HasPrefix(reason, "junction"):
+			case strings.Contains(reason, "junction"):
 				check = CheckJunction
 				check3BlocksSeed = true
 			default:
diff --git a/internal/loomengine/preflight_integration_test.go b/internal/loomengine/preflight_integration_test.go
index 470ed1f0..54bcc705 100644
--- a/internal/loomengine/preflight_integration_test.go
+++ b/internal/loomengine/preflight_integration_test.go
@@ -72,6 +72,13 @@ func seedValidStatus(t *testing.T, l *hubgeometry.Layout) {
 // hubgeometry.Getwd() — rather than checkResolved's injected-Layout form.
 // Because os.Chdir is process-global, callers of restoreCwd must never run
 // under t.Parallel().
+//
+// Callers MUST invoke restoreCwd AFTER creating any t.TempDir()/fixture the
+// test will os.Chdir into, never before: t.Cleanup runs LIFO, and on Windows
+// a directory cannot be removed while it is the process's current working
+// directory. Registering restoreCwd's chdir-back last is what makes it run
+// before that TempDir's own removal, so cleanup lands back outside the
+// directory before Go tries to delete it.
 func restoreCwd(t *testing.T) {
 	t.Helper()
 
@@ -147,9 +154,12 @@ func TestPreflight_HealthyPairAndSeed(t *testing.T) {
 // the public Preflight() (not checkResolved) because it needs
 // hubgeometry.Getwd() to observe a non-repo cwd.
 func TestPreflight_NotAGitRepo(t *testing.T) {
+	// t.TempDir() must be created before restoreCwd registers its cleanup —
+	// see restoreCwd's doc comment: on Windows, cleanup must chdir back out of
+	// dir before Go tries to remove it, and t.Cleanup runs LIFO.
+	dir := t.TempDir()
 	restoreCwd(t)
 
-	dir := t.TempDir()
 	if err := os.Chdir(dir); err != nil {
 		t.Fatalf("Chdir(%s): %v", dir, err)
 	}
@@ -166,9 +176,12 @@ func TestPreflight_NotAGitRepo(t *testing.T) {
 // single worktree-root failure. Exercises the public Preflight() for the
 // same reason as TestPreflight_NotAGitRepo.
 func TestPreflight_SubdirectoryInvocation(t *testing.T) {
-	restoreCwd(t)
-
+	// setupPreflightFixture's t.TempDir()-backed fixture must be created before
+	// restoreCwd registers its cleanup — see restoreCwd's doc comment: on
+	// Windows, cleanup must chdir back out of the fixture before Go tries to
+	// remove it, and t.Cleanup runs LIFO.
 	f, _ := setupPreflightFixture(t)
+	restoreCwd(t)
 
 	sub := filepath.Join(f.Hub, "sub")
 	if err := os.Mkdir(sub, 0o755); err != nil {
@@ -295,25 +308,169 @@ func TestPreflight_HostWeftDifferentBranches(t *testing.T) {
 	assertCheckSet(t, report, CheckWeftSync)
 }
 
-// TestPreflight_JunctionBroken asserts that removing the wired host _lyx
-// junction reports junction, and that the seed stat — which now resolves
-// through a missing _lyx entirely — is classified seed-unreadable (never
-// seed-missing) because check 3 already failed.
+// TestPreflight_JunctionBroken asserts that all three of PairInSync's
+// junction-drift shapes — missing, not-a-link, and points-elsewhere —
+// classify as junction (card 12's substring-match fix: a prefix match only
+// ever caught the missing shape). Each drift shape is exercised against BOTH
+// junctions (_lyx and _pattern, from card 15 onward) so the classification is
+// proven to hold for the second, non-_lyx junction too — not just the one
+// PairInSync's underlying loop was originally written and tested against.
+//
+// The seed-check expectation differs by junction, and deliberately so:
+// status.json lives under _lyx (l.LoomStatusFile() is _lyx-anchored), so a
+// broken _lyx junction also makes the seed stat fail — classified
+// seed-unreadable (never seed-missing) because check 3 already failed. A
+// broken _pattern junction, by contrast, leaves the seed fully readable
+// through the still-healthy _lyx junction: check 3 still fails and still
+// classifies as CheckJunction (never CheckWeftSync), but no seed failure is
+// added at all, since check 4's stat of l.LoomStatusFile() succeeds either
+// way. This asymmetry is exactly what "check3BlocksSeed" is named for: it
+// only changes check 4's classification of a stat failure that already
+// happened, it does not itself cause one.
 func TestPreflight_JunctionBroken(t *testing.T) {
+	shapes := []struct {
+		name    string
+		corrupt func(t *testing.T, hostLink string)
+	}{
+		{
+			name: "Missing",
+			corrupt: func(t *testing.T, hostLink string) {
+				if err := fslink.Remove(hostLink); err != nil {
+					t.Fatalf("remove host junction %s: %v", hostLink, err)
+				}
+			},
+		},
+		{
+			name: "NotALink",
+			corrupt: func(t *testing.T, hostLink string) {
+				if err := fslink.Remove(hostLink); err != nil {
+					t.Fatalf("remove host junction %s: %v", hostLink, err)
+				}
+				if err := os.Mkdir(hostLink, 0o755); err != nil {
+					t.Fatalf("mkdir real dir in junction's place %s: %v", hostLink, err)
+				}
+			},
+		},
+		{
+			name: "PointsElsewhere",
+			corrupt: func(t *testing.T, hostLink string) {
+				if err := fslink.Remove(hostLink); err != nil {
+					t.Fatalf("remove host junction %s: %v", hostLink, err)
+				}
+				wrongTarget := filepath.Join(filepath.Dir(hostLink), "not-the-weft-junction-dir")
+				if err := os.MkdirAll(wrongTarget, 0o755); err != nil {
+					t.Fatalf("mkdir wrong target %s: %v", wrongTarget, err)
+				}
+				if err := fslink.CreateDirLink(hostLink, wrongTarget); err != nil {
+					t.Fatalf("CreateDirLink(%s, %s): %v", hostLink, wrongTarget, err)
+				}
+			},
+		},
+	}
+
+	junctions := []struct {
+		name       string
+		linkFor    func(f lyxtest.PairedFixture, slug string) string
+		wantChecks []CheckID // in addition to CheckJunction, which every case wants
+	}{
+		{
+			name:       "Lyx",
+			linkFor:    func(f lyxtest.PairedFixture, slug string) string { return f.Layout.HostLyxLink(slug) },
+			wantChecks: []CheckID{CheckSeedUnreadable},
+		},
+		{
+			name:       "Pattern",
+			linkFor:    func(f lyxtest.PairedFixture, slug string) string { return f.Layout.HostPatternLink(slug) },
+			wantChecks: nil,
+		},
+	}
+
+	for _, j := range junctions {
+		for _, tt := range shapes {
+			t.Run(j.name+"_"+tt.name, func(t *testing.T) {
+				t.Parallel()
+
+				f, slug := setupPreflightFixture(t)
+				hostLink := j.linkFor(f, slug)
+				tt.corrupt(t, hostLink)
+
+				report, err := checkResolved(f.Layout)
+				if err != nil {
+					t.Fatalf("checkResolved: %v", err)
+				}
+				want := append([]CheckID{CheckJunction}, j.wantChecks...)
+				assertCheckSet(t, report, want...)
+			})
+		}
+	}
+}
+
+// TestPreflight_LegacyWorktreeUpgrade covers the upgrade consequence every
+// worktree wired before card 15 meets: _lyx is fully healthy, but _pattern was
+// never wired at all (simulated here by removing it from an otherwise-healthy
+// fixture, rather than corrupting it — the fixture never had it, full stop).
+// Preflight must classify this as CheckJunction, never CheckWeftSync, and
+// blocks the run (report.OK == false) — but does NOT also fail the seed
+// check, since status.json lives under the still-healthy _lyx junction (see
+// TestPreflight_JunctionBroken's doc comment for the same asymmetry). A
+// single Reconcile repairs it (adds the missing junction and materialises its
+// weft-side target) rather than reporting already-healthy; and a fresh
+// Preflight afterward reports OK — the "one lyx init or one lyx fabric
+// reconcile" remedy this batch documents.
+func TestPreflight_LegacyWorktreeUpgrade(t *testing.T) {
 	t.Parallel()
 
 	f, slug := setupPreflightFixture(t)
 
-	hostLink := f.Layout.HostLyxLink(slug)
-	if err := fslink.Remove(hostLink); err != nil {
-		t.Fatalf("remove host junction %s: %v", hostLink, err)
+	// Simulate the pre-upgrade state: this worktree's _pattern junction was
+	// never wired, even though _lyx is fully healthy — the "existing worktree,
+	// new lyx binary" shape, not a corruption.
+	patternLink := f.Layout.HostPatternLink(slug)
+	if err := fslink.Remove(patternLink); err != nil {
+		t.Fatalf("remove _pattern junction to simulate a legacy (pre-upgrade) worktree: %v", err)
 	}
 
 	report, err := checkResolved(f.Layout)
 	if err != nil {
 		t.Fatalf("checkResolved: %v", err)
 	}
-	assertCheckSet(t, report, CheckJunction, CheckSeedUnreadable)
+	assertCheckSet(t, report, CheckJunction)
+
+	// One Reconcile call repairs the missing junction: it must report
+	// JunctionRepointed (the repair happened), never AlreadyHealthy.
+	topology := fabricengine.NewTopology(fabricengine.Config{})
+	result, err := topology.Reconcile(f.Layout)
+	if err != nil {
+		t.Fatalf("Reconcile: %v", err)
+	}
+	var found bool
+	for _, pair := range result.Pairs {
+		if pair.HostWorktree != filepath.ToSlash(f.Layout.WorktreeRoot) {
+			continue
+		}
+		found = true
+		if pair.Action != fabricengine.ReconcileActionJunctionRepointed {
+			t.Errorf("Reconcile Action = %q; want %q", pair.Action, fabricengine.ReconcileActionJunctionRepointed)
+		}
+		if pair.Error != "" {
+			t.Errorf("Reconcile Error = %q; want empty", pair.Error)
+		}
+	}
+	if !found {
+		t.Fatalf("Reconcile result has no pair for host worktree %s: %+v", f.Layout.WorktreeRoot, result.Pairs)
+	}
+
+	// The junction now resolves.
+	if isLink, err := fslink.IsLink(patternLink); err != nil || !isLink {
+		t.Fatalf("_pattern junction %s not restored by Reconcile: isLink=%v err=%v", patternLink, isLink, err)
+	}
+
+	// A fresh Preflight now reports OK: the remedy this batch documents.
+	report, err = checkResolved(f.Layout)
+	if err != nil {
+		t.Fatalf("checkResolved after Reconcile: %v", err)
+	}
+	assertCheckSet(t, report)
 }
 
 // TestPreflight_SeedMissing asserts that a genuinely absent seed — junction
diff --git a/internal/pattern/doc.go b/internal/pattern/doc.go
new file mode 100644
index 00000000..81131b4e
--- /dev/null
+++ b/internal/pattern/doc.go
@@ -0,0 +1,54 @@
+// doc.go carries the package godoc for pattern: the active check, why it is
+// pure existence, why the three roles are what they are, and why the
+// injected pointer stays a relative path.
+
+// Package pattern answers one question for every code-touching lyx agent —
+// is PATTERN active in this worktree, and what should the agent be told? —
+// and returns the role-appropriate directive text to inject into that
+// agent's prompt.
+//
+// # The active check is pure existence
+//
+// PATTERN is active iff `_pattern/PATTERN.md` exists, resolved via
+// hubgeometry.Layout.PatternFileHere() and nothing else: this package never
+// constructs the path itself (the Hub Geometry Invariant's enforcement test
+// makes that impossible from batch 2 onward). Existence alone is the check —
+// never a content inspection — because the `_pattern/` directory itself may
+// exist without PATTERN.md (the normal inactive state; `lyx init` always
+// creates the directory), and a content-inspecting check would turn a
+// benign empty file into a runtime error in every one of the five agent
+// paths that call Directive. Three edge cases follow from that same
+// existence-only design and are each pinned by a test: an empty PATTERN.md
+// is active (degenerate but harmless); PATTERN.md present as a directory is
+// inactive (it is not a readable index); and a stat error that is not
+// "not exist" — a permission or I/O failure — is treated as active, since
+// resolving that ambiguity by silently disabling five agents' constraints is
+// worse than resolving it by handing the agent the directive anyway, so it
+// reads the file itself and reports a real, visible failure if it genuinely
+// cannot.
+//
+// # Why three roles, not one
+//
+// Directive's Role parameter selects one of three directive-text variants,
+// one per agent shape in the stack, because each shape needs its constraint
+// text worded for what that agent actually does: RoleImplementer for any
+// agent that edits code (builder implementer, webster fork, loom plan) is
+// worded as a pre-edit checklist; RoleReviewFix for the one review+fix round
+// (burler) covers both of that round's phases in the round's own order,
+// since a pure reviewer variant would have no user — burler is the only
+// reviewing template in the set and it also fixes; RoleOrchestrator for the
+// one role that never edits code itself (webster Master) is worded for
+// forking rather than editing, because an implementer-worded instruction
+// would ask Master to do something its own prompt says it never does.
+//
+// # Why the pointer stays relative
+//
+// Each directive injects a pointer to `_pattern/PATTERN.md`, never the
+// constraints inline, so prompt size stays constant however large PATTERN
+// grows. The pointer is a literal relative string baked into the directive
+// constant, never an interpolated absolute path built from a Layout field:
+// an absolute path would vary per worktree, which would make the fixed
+// directive strings unable to be compared for equality (or matched by
+// substring) across worktrees the way this package's own tests, and any
+// consumer's tests, need to.
+package pattern
diff --git a/internal/pattern/leaf_enforcement_test.go b/internal/pattern/leaf_enforcement_test.go
new file mode 100644
index 00000000..e0f1bf0a
--- /dev/null
+++ b/internal/pattern/leaf_enforcement_test.go
@@ -0,0 +1,88 @@
+// leaf_enforcement_test.go enforces the Pattern Leaf Invariant: production
+// code in internal/pattern imports ONLY the standard library and
+// internal/hubgeometry — never a feature package (builderengine,
+// websterengine, burlerengine, loomengine, or any other). Like modelspec's
+// and tokenvocab's leaf_enforcement_test.go, this check is an ALLOWLIST: any
+// import outside the allowed set fails the test, so a future stray
+// dependency is caught with no list maintenance required.
+
+package pattern
+
+import (
+	"go/parser"
+	"go/token"
+	"io/fs"
+	"path/filepath"
+	"runtime"
+	"strings"
+	"testing"
+)
+
+// allowedImports are the only non-stdlib import paths production code in
+// this package may use.
+var allowedImports = map[string]bool{
+	"github.com/Knatte18/loomyard/internal/hubgeometry": true,
+}
+
+// TestLeafInvariant_AllowlistOnly verifies that every non-test .go file in
+// this package directory imports only stdlib (no '.' in the first path
+// segment) or an entry in allowedImports. It uses go/parser with
+// ImportsOnly so only real import declarations are inspected, never string
+// literals in doc comments.
+func TestLeafInvariant_AllowlistOnly(t *testing.T) {
+	_, file, _, ok := runtime.Caller(0)
+	if !ok {
+		t.Fatal("could not determine pattern source directory location")
+	}
+	pkgDir := filepath.Dir(file)
+
+	var failures []string
+
+	err := filepath.WalkDir(pkgDir, func(path string, d fs.DirEntry, err error) error {
+		if err != nil {
+			return err
+		}
+		if d.IsDir() {
+			return nil
+		}
+		if strings.HasSuffix(d.Name(), "_test.go") || !strings.HasSuffix(d.Name(), ".go") {
+			return nil
+		}
+
+		fset := token.NewFileSet()
+		astFile, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
+		if err != nil {
+			t.Logf("warning: failed to parse %s: %v", path, err)
+			return nil
+		}
+
+		for _, imp := range astFile.Imports {
+			importPath := strings.Trim(imp.Path.Value, `"`)
+
+			// A stdlib import path has no '.' in its first path segment
+			// (e.g. "fmt", "os", "go/parser") — a domain that would need a
+			// registered TLD (e.g. "github.com/...") always contains one.
+			firstSegment := importPath
+			if idx := strings.IndexByte(importPath, '/'); idx >= 0 {
+				firstSegment = importPath[:idx]
+			}
+			isStdlib := !strings.Contains(firstSegment, ".")
+
+			if isStdlib || allowedImports[importPath] {
+				continue
+			}
+
+			relPath, _ := filepath.Rel(pkgDir, path)
+			failures = append(failures, relPath+": "+importPath)
+		}
+
+		return nil
+	})
+	if err != nil {
+		t.Fatalf("failed to walk pattern directory: %v", err)
+	}
+
+	if len(failures) > 0 {
+		t.Errorf("Pattern Leaf Invariant violated; imports outside the allowlist (stdlib + hubgeometry) found: %v", failures)
+	}
+}
diff --git a/internal/pattern/pattern.go b/internal/pattern/pattern.go
new file mode 100644
index 00000000..77f016f5
--- /dev/null
+++ b/internal/pattern/pattern.go
@@ -0,0 +1,136 @@
+// pattern.go implements the PATTERN active check and the three role-specific
+// directive constants Directive selects between. See doc.go for the
+// package-level rationale.
+
+package pattern
+
+import (
+	"os"
+
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
+)
+
+// Role identifies which agent-facing directive variant Directive should
+// render. The zero Role is deliberately not one of the three named
+// constants below; Directive documents its behaviour for that case (the
+// empty string) rather than leaving it to fall through undocumented.
+type Role int
+
+// The three directive variants Directive knows how to render, one per agent
+// shape in the stack. See doc.go's "Why three roles, not one" section for
+// why each variant is worded the way it is.
+const (
+	// RoleImplementer selects the pre-edit checklist used by any agent that
+	// edits code: builder's implementer, webster's fork, and loom's plan
+	// producer.
+	RoleImplementer Role = iota + 1
+	// RoleReviewFix selects the combined review+fix variant used by the
+	// burler round, the one reviewing template in the set — it also fixes,
+	// so a pure reviewer-only variant would have no user.
+	RoleReviewFix
+	// RoleOrchestrator selects the forking-only variant used by webster's
+	// Master session, which never edits code itself.
+	RoleOrchestrator
+)
+
+// implementerDirective is RoleImplementer's directive text. Every directive
+// constant in this file carries its own "##" heading inline so an inactive
+// render leaves no orphan heading behind, is phrased as an imperative
+// checklist rather than a single sentence, and names the literal relative
+// pointer "_pattern/PATTERN.md" rather than an interpolated absolute path.
+const implementerDirective = `## Constraints — do this before you write any code
+
+- **STOP.** Read _pattern/PATTERN.md in full before editing a single file.
+- Read every detail doc under _pattern/ that PATTERN.md points to and that touches what you are about to change.
+- These constraints are BINDING: a change that violates one is wrong even if the verify command passes.
+- If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your report instead of silently picking one.
+`
+
+// reviewFixDirective is RoleReviewFix's directive text, covering both of the
+// burler round's phases (A-review, B-fix) in the round's own order.
+const reviewFixDirective = `## Constraints — do this before you judge or change anything
+
+- Read _pattern/PATTERN.md in full before forming any judgment.
+- Read every detail doc under _pattern/ that PATTERN.md points to and that touches what you are about to judge or change.
+- In part A, every violation of a listed constraint is a BLOCKING finding: record it no matter how small it looks, and never wave it through because the code works or the tests pass.
+- In part B, the fix must not introduce a violation of its own: a fix that trades one finding for a constraint breach is not a fix.
+- If a constraint conflicts with anything else in this prompt, the constraint wins — say so in your report instead of silently picking one.
+`
+
+// orchestratorDirective is RoleOrchestrator's directive text, worded for
+// forking rather than editing since webster's Master never edits code
+// itself.
+const orchestratorDirective = `## Constraints — do this before you fork anything
+
+- Read _pattern/PATTERN.md in full before forking a single implementer.
+- Read every detail doc under _pattern/ that PATTERN.md points to and that touches what the forks you are about to spawn will do.
+- Every fork inherits its context, so reading this once here is what puts the constraints in front of all of them; it must not be skipped on the grounds of not editing code.
+- The constraints are BINDING on the forks it spawns: a batch report trading a constraint for a passing verify is a failed batch, not a success.
+`
+
+// statFile is the stat implementation isActive calls. It is a package-level
+// variable — rather than a hardcoded os.Stat call — purely so this
+// package's own test suite can simulate a non-"not exist" stat error (a
+// permission or I/O failure) portably across platforms, without depending
+// on process privilege or a POSIX-only permission trick. Production code
+// never reassigns it.
+var statFile = os.Stat
+
+// Directive reports whether PATTERN is active for the worktree l resolves
+// to and, if so, returns the role's directive text to inject into that
+// agent's prompt.
+//
+// It returns the empty string in three documented cases, each a deliberate
+// no-op rather than a panic or an error — because a slip in any one of them
+// would otherwise take down prompt assembly in every one of the five agent
+// paths that call Directive:
+//
+//   - l is nil (several Deps structs are assembled field-by-field by CLI
+//     callers that could leave Layout unset).
+//   - PATTERN is inactive for l (see isActive).
+//   - role is not one of RoleImplementer, RoleReviewFix, or
+//     RoleOrchestrator — including the zero Role.
+//
+// See doc.go for the full active-check rule and the rationale behind the
+// three role variants.
+func Directive(l *hubgeometry.Layout, role Role) string {
+	if l == nil {
+		return ""
+	}
+	if !isActive(l) {
+		return ""
+	}
+	switch role {
+	case RoleImplementer:
+		return implementerDirective
+	case RoleReviewFix:
+		return reviewFixDirective
+	case RoleOrchestrator:
+		return orchestratorDirective
+	default:
+		// An unknown or zero Role renders no directive; this default case
+		// is what makes that behaviour defined and documented rather than
+		// an unhandled fall-through.
+		return ""
+	}
+}
+
+// isActive reports whether PATTERN is active for the worktree l resolves
+// to. The rule is: an absent PatternFileHere() means inactive, and every
+// other stat outcome means active — with one exception, a directory in
+// PATTERN.md's place, which is also inactive because it is not a readable
+// index.
+func isActive(l *hubgeometry.Layout) bool {
+	info, err := statFile(l.PatternFileHere())
+	if err != nil {
+		// os.IsNotExist is the normal, common inactive case: PATTERN.md was
+		// never created. Any other error — permission denied, I/O failure —
+		// is treated as active per Directive's doc comment: the ambiguity
+		// resolves loud, in the agent's own read of the file, rather than
+		// silently disabling the constraints.
+		return !os.IsNotExist(err)
+	}
+	// A directory named PATTERN.md is not a readable index; treat it the
+	// same as absent rather than reading something that isn't the file.
+	return !info.IsDir()
+}
diff --git a/internal/pattern/pattern_test.go b/internal/pattern/pattern_test.go
new file mode 100644
index 00000000..240cb6e0
--- /dev/null
+++ b/internal/pattern/pattern_test.go
@@ -0,0 +1,272 @@
+// pattern_test.go exercises Directive's active check and its three
+// directive variants. Every test here is untagged Tier 1: it uses only
+// os.Stat (via the package's statFile seam) and t.TempDir, and spawns
+// nothing.
+
+package pattern
+
+import (
+	"errors"
+	"os"
+	"path/filepath"
+	"strings"
+	"testing"
+
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
+)
+
+// layoutAt builds a minimal *hubgeometry.Layout rooted at worktreeRoot, with
+// the given RelPath, sufficient for PatternFileHere() to resolve — the only
+// accessor this package calls.
+func layoutAt(worktreeRoot, relPath string) *hubgeometry.Layout {
+	return &hubgeometry.Layout{
+		WorktreeRoot: worktreeRoot,
+		RelPath:      relPath,
+	}
+}
+
+// writePatternFile creates root/_pattern/PATTERN.md (and the _pattern
+// directory) with the given content, failing the test on any error.
+func writePatternFile(t *testing.T, root, content string) {
+	t.Helper()
+	dir := filepath.Join(root, "_pattern")
+	if err := os.MkdirAll(dir, 0o755); err != nil {
+		t.Fatalf("MkdirAll(%q) = %v", dir, err)
+	}
+	if err := os.WriteFile(filepath.Join(dir, "PATTERN.md"), []byte(content), 0o644); err != nil {
+		t.Fatalf("WriteFile(PATTERN.md) = %v", err)
+	}
+}
+
+// TestDirective_ActiveWithFile covers the common active case — PATTERN.md
+// present as a regular file — for all three roles.
+func TestDirective_ActiveWithFile(t *testing.T) {
+	root := t.TempDir()
+	writePatternFile(t, root, "# PATTERN\n\nsome constraints\n")
+	l := layoutAt(root, ".")
+
+	tests := []struct {
+		name string
+		role Role
+	}{
+		{"Implementer", RoleImplementer},
+		{"ReviewFix", RoleReviewFix},
+		{"Orchestrator", RoleOrchestrator},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := Directive(l, tt.role)
+			if got == "" {
+				t.Errorf("Directive(active, %v) = \"\"; want non-empty", tt.role)
+			}
+		})
+	}
+}
+
+// TestDirective_InactiveWithoutFile covers the two ordinary inactive cases:
+// the _pattern directory present without PATTERN.md (the normal state
+// lyx init leaves behind), and neither present at all.
+func TestDirective_InactiveWithoutFile(t *testing.T) {
+	tests := []struct {
+		name  string
+		setup func(t *testing.T, root string)
+	}{
+		{
+			name: "DirPresentFileAbsent",
+			setup: func(t *testing.T, root string) {
+				t.Helper()
+				if err := os.MkdirAll(filepath.Join(root, "_pattern"), 0o755); err != nil {
+					t.Fatalf("MkdirAll = %v", err)
+				}
+			},
+		},
+		{
+			name:  "NeitherPresent",
+			setup: func(t *testing.T, root string) {},
+		},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			root := t.TempDir()
+			tt.setup(t, root)
+			l := layoutAt(root, ".")
+			if got := Directive(l, RoleImplementer); got != "" {
+				t.Errorf("Directive(%s, RoleImplementer) = %q; want \"\"", tt.name, got)
+			}
+		})
+	}
+}
+
+// TestDirective_EmptyPatternFileIsActive pins the "empty file still counts
+// as active" edge rule: a degenerate but harmless state, preferable to a
+// content-inspecting check that would turn a benign empty file into a
+// runtime error.
+func TestDirective_EmptyPatternFileIsActive(t *testing.T) {
+	root := t.TempDir()
+	writePatternFile(t, root, "")
+	l := layoutAt(root, ".")
+
+	if got := Directive(l, RoleImplementer); got == "" {
+		t.Errorf("Directive(empty PATTERN.md) = \"\"; want non-empty")
+	}
+}
+
+// TestDirective_PatternFileAsDirectoryIsInactive pins the "PATTERN.md as a
+// directory counts as inactive" edge rule: a directory in that place is not
+// a readable index.
+func TestDirective_PatternFileAsDirectoryIsInactive(t *testing.T) {
+	root := t.TempDir()
+	patternFileAsDir := filepath.Join(root, "_pattern", "PATTERN.md")
+	if err := os.MkdirAll(patternFileAsDir, 0o755); err != nil {
+		t.Fatalf("MkdirAll(%q) = %v", patternFileAsDir, err)
+	}
+	l := layoutAt(root, ".")
+
+	if got := Directive(l, RoleImplementer); got != "" {
+		t.Errorf("Directive(PATTERN.md as directory) = %q; want \"\"", got)
+	}
+}
+
+// TestDirective_NilLayout pins the nil-Layout guard: several Deps structs
+// are assembled field-by-field by CLI callers that could leave Layout
+// unset, and a nil dereference here would take down all five agent paths
+// for a slip unrelated to PATTERN.
+func TestDirective_NilLayout(t *testing.T) {
+	if got := Directive(nil, RoleImplementer); got != "" {
+		t.Errorf("Directive(nil, RoleImplementer) = %q; want \"\"", got)
+	}
+}
+
+// TestDirective_UnknownRole pins the documented unknown/zero Role
+// behaviour: no directive text, even when PATTERN is active.
+func TestDirective_UnknownRole(t *testing.T) {
+	root := t.TempDir()
+	writePatternFile(t, root, "content")
+	l := layoutAt(root, ".")
+
+	tests := []struct {
+		name string
+		role Role
+	}{
+		{"ZeroRole", Role(0)},
+		{"OutOfRangeRole", Role(99)},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			if got := Directive(l, tt.role); got != "" {
+				t.Errorf("Directive(active, %v) = %q; want \"\"", tt.role, got)
+			}
+		})
+	}
+}
+
+// TestDirective_VariantsArePairwiseDistinct pins that the three role
+// variants never collapse into the same text, and that each carries the
+// literal relative pointer "_pattern/PATTERN.md" — never an interpolated
+// absolute path, which would make the value vary per worktree.
+func TestDirective_VariantsArePairwiseDistinct(t *testing.T) {
+	root := t.TempDir()
+	writePatternFile(t, root, "content")
+	l := layoutAt(root, ".")
+
+	variants := map[Role]string{
+		RoleImplementer:  Directive(l, RoleImplementer),
+		RoleReviewFix:    Directive(l, RoleReviewFix),
+		RoleOrchestrator: Directive(l, RoleOrchestrator),
+	}
+	for role, text := range variants {
+		if !strings.Contains(text, "_pattern/PATTERN.md") {
+			t.Errorf("Directive(%v) does not contain the literal pointer _pattern/PATTERN.md: %q", role, text)
+		}
+	}
+
+	if variants[RoleImplementer] == variants[RoleReviewFix] {
+		t.Error("RoleImplementer and RoleReviewFix render identical directive text")
+	}
+	if variants[RoleImplementer] == variants[RoleOrchestrator] {
+		t.Error("RoleImplementer and RoleOrchestrator render identical directive text")
+	}
+	if variants[RoleReviewFix] == variants[RoleOrchestrator] {
+		t.Error("RoleReviewFix and RoleOrchestrator render identical directive text")
+	}
+}
+
+// TestDirective_VariantsBeginWithOwnHeading pins that each variant carries
+// its own "##" heading inline, so an inactive render leaves no orphan
+// heading behind in the surrounding prompt template.
+func TestDirective_VariantsBeginWithOwnHeading(t *testing.T) {
+	root := t.TempDir()
+	writePatternFile(t, root, "content")
+	l := layoutAt(root, ".")
+
+	tests := []struct {
+		name string
+		role Role
+	}{
+		{"Implementer", RoleImplementer},
+		{"ReviewFix", RoleReviewFix},
+		{"Orchestrator", RoleOrchestrator},
+	}
+	for _, tt := range tests {
+		t.Run(tt.name, func(t *testing.T) {
+			got := Directive(l, tt.role)
+			if !strings.HasPrefix(got, "## ") {
+				t.Errorf("Directive(%v) does not begin with its own \"##\" heading: %q", tt.role, got)
+			}
+		})
+	}
+}
+
+// TestDirective_RelPathNestedSubdirectory is the regression guard for the
+// worst failure mode in this task: a Layout whose RelPath is a nested
+// subdirectory must resolve PATTERN.md at
+// <WorktreeRoot>/<RelPath>/_pattern/PATTERN.md and must NOT be satisfied by
+// one planted at the worktree root instead. Without this guard, a
+// root-anchored resolution would render PATTERN silently inactive in every
+// agent invoked from a subdirectory, with no error anywhere.
+func TestDirective_RelPathNestedSubdirectory(t *testing.T) {
+	root := t.TempDir()
+	relPath := filepath.Join("sub", "dir")
+
+	// Plant PATTERN.md only at the (wrong) worktree root; the RelPath-aware
+	// resolution must still see this worktree as inactive.
+	writePatternFile(t, root, "content")
+	l := layoutAt(root, relPath)
+	if got := Directive(l, RoleImplementer); got != "" {
+		t.Errorf("Directive() found the root-planted PATTERN.md via a nested RelPath; got %q, want \"\"", got)
+	}
+
+	// Now plant PATTERN.md at the correct nested location; the resolution
+	// must find it there.
+	nestedRoot := filepath.Join(root, relPath)
+	writePatternFile(t, nestedRoot, "content")
+	if got := Directive(l, RoleImplementer); got == "" {
+		t.Error("Directive() did not find PATTERN.md planted at <WorktreeRoot>/<RelPath>/_pattern/PATTERN.md")
+	}
+}
+
+// TestDirective_NonNotExistStatErrorIsActive pins the third edge rule: a
+// stat error that is not os.IsNotExist (a permission or I/O failure) is
+// treated as active, not inactive. This is simulated through the
+// package-level statFile seam rather than a real unreadable-directory
+// trick, because an actual permission-denied stat error is not portable —
+// it depends on the OS and on whether the test process runs elevated (e.g.
+// as root in a container, where POSIX permission bits are not enforced),
+// and Windows has no equivalent lever at all.
+func TestDirective_NonNotExistStatErrorIsActive(t *testing.T) {
+	root := t.TempDir()
+	l := layoutAt(root, ".")
+
+	// PATTERN.md is absent on disk; without the seam this would resolve
+	// inactive via os.IsNotExist. Force a distinct, non-not-exist error to
+	// confirm the "any other stat error is active" rule.
+	original := statFile
+	statFile = func(name string) (os.FileInfo, error) {
+		return nil, &os.PathError{Op: "stat", Path: name, Err: errors.New("permission denied")}
+	}
+	t.Cleanup(func() { statFile = original })
+
+	if got := Directive(l, RoleImplementer); got == "" {
+		t.Error("Directive() with a non-IsNotExist stat error = \"\"; want the directive text (active)")
+	}
+}
diff --git a/internal/reedengine/proctree.go b/internal/reedengine/proctree.go
index c8b91cff..c4479cc2 100644
--- a/internal/reedengine/proctree.go
+++ b/internal/reedengine/proctree.go
@@ -118,7 +118,16 @@ func matchSocketCmdlines(procs []ProcCmdline, binary, socket string) []int {
 // "__warm__" helper alive, holding the hub's .lyx/logs directory forever
 // (found live in the fabric-cutover review, round fable-r1).
 func tmuxProcessName(binary string) string {
-	name := filepath.Base(binary)
+	// binary names a Windows path (this function derives a Windows
+	// process-table Name), so the base-name split must recognize '\' even
+	// when this code runs on a non-Windows test host: path/filepath.Base is
+	// GOOS-native and would leave a "C:\...\tmux.exe"-shaped input untouched
+	// on Linux, which is exactly what proctree.go's own package doc promises
+	// never happens ("None of these functions touch the OS").
+	name := binary
+	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
+		name = name[i+1:]
+	}
 	if !strings.HasSuffix(strings.ToLower(name), ".exe") {
 		name += ".exe"
 	}
diff --git a/internal/stencil/stencil.go b/internal/stencil/stencil.go
index 7e21a0fb..7220f693 100644
--- a/internal/stencil/stencil.go
+++ b/internal/stencil/stencil.go
@@ -17,6 +17,8 @@ import (
 // Fill renders a markdown template by substituting {{.X}} marker fields from values
 // and returns the rendered bytes. It never HTML-escapes output (it uses text/template,
 // not html/template), so values containing markdown or code fences pass through verbatim.
+// Fill is defined as FillOptional(template, values, nil): it carries no optional-marker
+// exemption, so every top-level marker follows Fill's original unfilled-marker guarantee.
 //
 // A leading `<!-- ... -->` comment is stripped before parsing (a documentation banner on
 // the template asset); comments elsewhere in the template are left untouched as ordinary
@@ -34,6 +36,28 @@ import (
 // template's top level, never inside a conditional branch, or an empty value for it will
 // pass through unnoticed.
 func Fill(template []byte, values map[string]string) ([]byte, error) {
+	return FillOptional(template, values, nil)
+}
+
+// FillOptional renders a markdown template exactly like Fill, except every name listed in
+// optional is exempt from Fill's unfilled-top-level-marker guarantee: an optional marker
+// absent from values, or present as an empty or whitespace-only string, renders as nothing
+// instead of tripping either of Fill's two guards. This is the mechanism a caller uses to
+// mark a specific `{{.X}}` field as allowed to render blank — optionality is a property of
+// the call site's argument list, not of the template text, so the same template can be
+// filled once with a marker required and once with it optional depending on the caller.
+//
+// The exemption reaches both guards, which are separate mechanisms: the top-level batch
+// check (unfilledTopLevelMarkers) skips a listed name entirely, and the branch-internal
+// missingkey=error check never fires on a listed name because FillOptional seeds a copy of
+// values with an explicit "" for every optional name that is absent or whitespace-only
+// before executing. The caller's values map is never mutated. A name listed in optional but
+// absent from the template entirely is a harmless no-op.
+//
+// An optional name listed in optional is a first-class part of Fill's contract, not a
+// backdoor around it: Fill(t, v) and FillOptional(t, v, nil) are byte-identical on the same
+// input, including on the error path, because Fill is defined in terms of FillOptional.
+func FillOptional(template []byte, values map[string]string, optional []string) ([]byte, error) {
 	// Strip a leading banner comment before parsing; mid-template comments are ordinary
 	// template syntax and must reach the parser untouched.
 	stripped := stripLeadingComment(string(template))
@@ -45,16 +69,43 @@ func Fill(template []byte, values map[string]string) ([]byte, error) {
 		return nil, fmt.Errorf("parse template: %w", err)
 	}
 
+	// A set lookup is what lets both guards below share one definition of "this name is
+	// exempt", rather than each re-deriving it from the optional slice independently.
+	optionalNames := make(map[string]bool, len(optional))
+	for _, name := range optional {
+		optionalNames[name] = true
+	}
+
 	// Batch-check every top-level marker before executing anything: this is what lets us
 	// report every unfilled top-level marker in one error instead of failing on the first.
-	offenders := unfilledTopLevelMarkers(t, values)
+	// A listed optional name is skipped by this check regardless of its value.
+	offenders := unfilledTopLevelMarkers(t, values, optionalNames)
 	if len(offenders) > 0 {
 		sort.Strings(offenders)
 		return nil, fmt.Errorf("stencil: unfilled top-level marker(s): %s", strings.Join(offenders, ", "))
 	}
 
+	// missingkey=error fires at execution time on a key wholly absent from the map, so an
+	// optional name absent from values would otherwise still error the moment execution
+	// reaches it. Seed a copy — never the caller's own map — with an explicit "" for every
+	// optional name that is absent or, per the same TrimSpace test unfilledTopLevelMarkers
+	// uses, whitespace-only: a "   " optional value must render as nothing, not as its
+	// three spaces verbatim.
+	execValues := values
+	if len(optionalNames) > 0 {
+		execValues = make(map[string]string, len(values))
+		for k, v := range values {
+			execValues[k] = v
+		}
+		for name := range optionalNames {
+			if strings.TrimSpace(execValues[name]) == "" {
+				execValues[name] = ""
+			}
+		}
+	}
+
 	var buf bytes.Buffer
-	if err := t.Execute(&buf, values); err != nil {
+	if err := t.Execute(&buf, execValues); err != nil {
 		// A branch-internal reached-but-absent marker surfaces here, one at a time,
 		// because missingkey=error halts execution at the first miss.
 		return nil, fmt.Errorf("execute template: %w", err)
@@ -83,8 +134,9 @@ func stripLeadingComment(text string) string {
 // unfilledTopLevelMarkers walks the parsed template's top-level (depth-0) nodes only —
 // it does not descend into if/with/range bodies, since those are checked incrementally
 // at execution time instead — and returns the deduplicated names of every bare `{{.X}}`
-// substitution whose value in values is absent or empty-or-whitespace-only.
-func unfilledTopLevelMarkers(t *tmpl.Template, values map[string]string) []string {
+// substitution whose value in values is absent or empty-or-whitespace-only. A name present
+// in optional is exempt from this check entirely, regardless of its value in values.
+func unfilledTopLevelMarkers(t *tmpl.Template, values map[string]string, optional map[string]bool) []string {
 	// A comment-only or empty template parses to a tree with no root, or a root with
 	// no nodes; either way there is nothing to check.
 	if t.Tree == nil || t.Tree.Root == nil {
@@ -114,6 +166,10 @@ func unfilledTopLevelMarkers(t *tmpl.Template, values map[string]string) []strin
 		}
 
 		name := fieldNode.Ident[0]
+		// A listed optional name is exempt from this guard outright, whatever its value.
+		if optional[name] {
+			continue
+		}
 		// An absent key reads as the zero value "" from a map[string]string, so this
 		// single TrimSpace check covers both the absent-key and empty/whitespace cases.
 		if strings.TrimSpace(values[name]) != "" {
diff --git a/internal/stencil/stencil_test.go b/internal/stencil/stencil_test.go
index 0cc4925c..e82f505c 100644
--- a/internal/stencil/stencil_test.go
+++ b/internal/stencil/stencil_test.go
@@ -1,7 +1,8 @@
-// stencil_test.go is the black-box, table-driven contract test for stencil.Fill: the
-// happy path, the unfilled-top-level-marker guard (including sorting/dedup), the
-// incremental branch-internal guard, conditional sections, the leading-comment strip,
-// and the no-HTML-escaping / idempotence guarantees.
+// stencil_test.go is the black-box, table-driven contract test for stencil.Fill and
+// stencil.FillOptional: the happy path, the unfilled-top-level-marker guard (including
+// sorting/dedup), the incremental branch-internal guard, conditional sections, the
+// leading-comment strip, the no-HTML-escaping / idempotence guarantees, and
+// FillOptional's optional-marker exemption from both guards.
 
 package stencil_test
 
@@ -385,3 +386,224 @@ func TestFill_NoHTMLEscaping(t *testing.T) {
 		t.Errorf("Fill() = %q; want %q (no HTML escaping)", string(got), want)
 	}
 }
+
+// TestFillOptional_AbsentRendersNothing covers a marker listed as optional and absent
+// from values rendering as nothing with no error, instead of tripping the
+// unfilled-top-level-marker guard.
+func TestFillOptional_AbsentRendersNothing(t *testing.T) {
+	got, err := stencil.FillOptional(
+		[]byte("Head: {{.Head}}\nExtra: {{.Extra}}"),
+		map[string]string{"Head": "present"},
+		[]string{"Extra"},
+	)
+	if err != nil {
+		t.Fatalf("FillOptional() unexpected error: %v", err)
+	}
+	want := "Head: present\nExtra: "
+	if string(got) != want {
+		t.Errorf("FillOptional() = %q; want %q", string(got), want)
+	}
+}
+
+// TestFillOptional_PresentButEmptyRendersNothing covers a marker listed as optional and
+// present in values as "" rendering as nothing with no error.
+func TestFillOptional_PresentButEmptyRendersNothing(t *testing.T) {
+	got, err := stencil.FillOptional(
+		[]byte("Extra: {{.Extra}}"),
+		map[string]string{"Extra": ""},
+		[]string{"Extra"},
+	)
+	if err != nil {
+		t.Fatalf("FillOptional() unexpected error: %v", err)
+	}
+	want := "Extra: "
+	if string(got) != want {
+		t.Errorf("FillOptional() = %q; want %q", string(got), want)
+	}
+}
+
+// TestFillOptional_WhitespaceOnlyNormalisesToEmpty covers a marker listed as optional
+// and present in values as whitespace-only rendering as nothing, not as its whitespace
+// verbatim — the same TrimSpace-based "empty" definition unfilledTopLevelMarkers uses
+// must also govern what FillOptional seeds before execution.
+func TestFillOptional_WhitespaceOnlyNormalisesToEmpty(t *testing.T) {
+	got, err := stencil.FillOptional(
+		[]byte("Extra: [{{.Extra}}]"),
+		map[string]string{"Extra": "   "},
+		[]string{"Extra"},
+	)
+	if err != nil {
+		t.Fatalf("FillOptional() unexpected error: %v", err)
+	}
+	want := "Extra: []"
+	if string(got) != want {
+		t.Errorf("FillOptional() = %q; want %q (whitespace-only optional value must normalise to empty)", string(got), want)
+	}
+}
+
+// TestFillOptional_PresentAndNonEmptyRendersValue covers a marker listed as optional but
+// present and non-empty rendering its actual value, confirming the optional exemption
+// only changes behaviour for absent/empty values, not for a value that is genuinely set.
+func TestFillOptional_PresentAndNonEmptyRendersValue(t *testing.T) {
+	got, err := stencil.FillOptional(
+		[]byte("Extra: {{.Extra}}"),
+		map[string]string{"Extra": "filled-in"},
+		[]string{"Extra"},
+	)
+	if err != nil {
+		t.Fatalf("FillOptional() unexpected error: %v", err)
+	}
+	want := "Extra: filled-in"
+	if string(got) != want {
+		t.Errorf("FillOptional() = %q; want %q", string(got), want)
+	}
+}
+
+// TestFillOptional_NonOptionalEmptyMarkerStillErrors covers a template with only a
+// non-optional empty marker: the existing unfilled-top-level-marker error must still
+// fire, confirming FillOptional(t, v, nil-equivalent-for-that-name) behaves exactly like
+// Fill for names not listed as optional.
+func TestFillOptional_NonOptionalEmptyMarkerStillErrors(t *testing.T) {
+	_, err := stencil.FillOptional(
+		[]byte("Fasit: {{.Fasit}}"),
+		map[string]string{"Fasit": ""},
+		[]string{"SomeOtherName"},
+	)
+	if err == nil {
+		t.Fatal("FillOptional() got nil error; want the unfilled top-level marker error for the non-optional Fasit")
+	}
+	if !strings.Contains(err.Error(), "Fasit") {
+		t.Errorf("FillOptional() error = %q; want it to name Fasit", err.Error())
+	}
+}
+
+// TestFillOptional_MixOfOptionalAndRequiredEmptyReportsOnlyRequired covers a template
+// with one optional-and-empty marker plus one required-and-empty marker: the error must
+// name only the required one, confirming the optional exemption removes a name from the
+// offenders list rather than merely suppressing the whole error.
+func TestFillOptional_MixOfOptionalAndRequiredEmptyReportsOnlyRequired(t *testing.T) {
+	_, err := stencil.FillOptional(
+		[]byte("Fasit: {{.Fasit}}\nExtra: {{.Extra}}"),
+		map[string]string{"Fasit": "", "Extra": ""},
+		[]string{"Extra"},
+	)
+	if err == nil {
+		t.Fatal("FillOptional() got nil error; want the unfilled top-level marker error for the non-optional Fasit")
+	}
+	wantMsg := "stencil: unfilled top-level marker(s): Fasit"
+	if err.Error() != wantMsg {
+		t.Errorf("FillOptional() error = %q; want %q (Extra must not appear, it is optional)", err.Error(), wantMsg)
+	}
+}
+
+// TestFillOptional_ByteIdenticalToFillOnSameInput covers Fill(t, v) and
+// FillOptional(t, v, nil) producing byte-identical output on the happy path and
+// byte-identical error text on the error path, confirming Fill is genuinely defined as
+// FillOptional(t, v, nil) rather than a parallel implementation that could drift.
+func TestFillOptional_ByteIdenticalToFillOnSameInput(t *testing.T) {
+	t.Run("happy_path", func(t *testing.T) {
+		template := []byte("Fasit: {{.Fasit}}\nTarget: {{.Target}}\n")
+		values := map[string]string{"Fasit": "foo", "Target": "bar"}
+
+		fillGot, fillErr := stencil.Fill(template, values)
+		optGot, optErr := stencil.FillOptional(template, values, nil)
+		if fillErr != nil || optErr != nil {
+			t.Fatalf("unexpected errors: Fill() = %v, FillOptional() = %v", fillErr, optErr)
+		}
+		if string(fillGot) != string(optGot) {
+			t.Errorf("Fill() = %q; FillOptional(t, v, nil) = %q; want byte-identical", string(fillGot), string(optGot))
+		}
+	})
+
+	t.Run("error_path", func(t *testing.T) {
+		template := []byte("Fasit: {{.Fasit}}\n")
+		values := map[string]string{}
+
+		_, fillErr := stencil.Fill(template, values)
+		_, optErr := stencil.FillOptional(template, values, nil)
+		if fillErr == nil || optErr == nil {
+			t.Fatalf("Fill() and FillOptional() must both error; got Fill() = %v, FillOptional() = %v", fillErr, optErr)
+		}
+		if fillErr.Error() != optErr.Error() {
+			t.Errorf("Fill() error = %q; FillOptional(t, v, nil) error = %q; want byte-identical", fillErr.Error(), optErr.Error())
+		}
+	})
+}
+
+// TestFillOptional_OptionalNameAbsentFromTemplateIsNoOp covers an optional name listed
+// but never referenced anywhere in the template: rendering succeeds unaffected, since
+// there is no marker for the exemption to apply to.
+func TestFillOptional_OptionalNameAbsentFromTemplateIsNoOp(t *testing.T) {
+	got, err := stencil.FillOptional(
+		[]byte("Fasit: {{.Fasit}}"),
+		map[string]string{"Fasit": "value"},
+		[]string{"NeverMentioned"},
+	)
+	if err != nil {
+		t.Fatalf("FillOptional() unexpected error: %v", err)
+	}
+	want := "Fasit: value"
+	if string(got) != want {
+		t.Errorf("FillOptional() = %q; want %q", string(got), want)
+	}
+}
+
+// TestFillOptional_CallerValuesMapNotMutated covers the caller's values map being left
+// untouched after a call whose optional-seeding step would otherwise need to add or
+// overwrite an entry, confirming FillOptional operates on a private copy.
+func TestFillOptional_CallerValuesMapNotMutated(t *testing.T) {
+	values := map[string]string{"Fasit": "value"}
+
+	_, err := stencil.FillOptional(
+		[]byte("Fasit: {{.Fasit}}\nExtra: {{.Extra}}"),
+		values,
+		[]string{"Extra"},
+	)
+	if err != nil {
+		t.Fatalf("FillOptional() unexpected error: %v", err)
+	}
+	if _, exists := values["Extra"]; exists {
+		t.Errorf("FillOptional() mutated the caller's values map by adding %q", "Extra")
+	}
+	if len(values) != 1 {
+		t.Errorf("FillOptional() mutated the caller's values map; got %d entries, want 1", len(values))
+	}
+}
+
+// TestFillOptional_RepeatedCallsProduceIdenticalOutput covers repeated FillOptional
+// calls with the same inputs producing byte-identical output and identical error text,
+// mirroring Fill's own idempotence guarantee.
+func TestFillOptional_RepeatedCallsProduceIdenticalOutput(t *testing.T) {
+	t.Run("output_stable_across_calls", func(t *testing.T) {
+		template := []byte("Head: {{.Head}}\nExtra: {{.Extra}}")
+		values := map[string]string{"Head": "value"}
+		optional := []string{"Extra"}
+
+		first, err := stencil.FillOptional(template, values, optional)
+		if err != nil {
+			t.Fatalf("FillOptional() unexpected error on first call: %v", err)
+		}
+		second, err := stencil.FillOptional(template, values, optional)
+		if err != nil {
+			t.Fatalf("FillOptional() unexpected error on second call: %v", err)
+		}
+		if string(first) != string(second) {
+			t.Errorf("FillOptional() not idempotent: first = %q, second = %q", string(first), string(second))
+		}
+	})
+
+	t.Run("error_text_stable_across_calls", func(t *testing.T) {
+		template := []byte("Fasit: {{.Fasit}}\nExtra: {{.Extra}}")
+		values := map[string]string{}
+		optional := []string{"Extra"}
+
+		_, firstErr := stencil.FillOptional(template, values, optional)
+		_, secondErr := stencil.FillOptional(template, values, optional)
+		if firstErr == nil || secondErr == nil {
+			t.Fatalf("FillOptional() got nil error(s); want unfilled-marker errors on both calls")
+		}
+		if firstErr.Error() != secondErr.Error() {
+			t.Errorf("FillOptional() error message not stable: first = %q, second = %q", firstErr.Error(), secondErr.Error())
+		}
+	})
+}
diff --git a/internal/webstercli/beginbatch.go b/internal/webstercli/beginbatch.go
index 5657123a..0cc994b0 100644
--- a/internal/webstercli/beginbatch.go
+++ b/internal/webstercli/beginbatch.go
@@ -101,6 +101,7 @@ Example:
 				Injector:     c.injector,
 				Reed:         c.reed,
 				WorktreeRoot: c.layout.Cwd,
+				Layout:       c.layout,
 				WebsterDir:   c.websterDir,
 				ReportsDir:   c.reportsDir,
 				PromptsDir:   c.promptsDir,
diff --git a/internal/websterengine/beginbatch.go b/internal/websterengine/beginbatch.go
index 6d072c3d..b5b5e5b6 100644
--- a/internal/websterengine/beginbatch.go
+++ b/internal/websterengine/beginbatch.go
@@ -24,6 +24,7 @@ import (
 	"time"
 
 	"github.com/Knatte18/loomyard/internal/batcher"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/modelspec"
 	"github.com/Knatte18/loomyard/internal/planparser"
 	"github.com/Knatte18/loomyard/internal/shuttleengine"
@@ -62,9 +63,13 @@ type Injector interface {
 // choreography into Master's pane; Reed is the live reed query surface the
 // prior-recovery-strand reclaim consults (a dead-but-live recovery record a
 // fork batch is about to overwrite); WorktreeRoot is the host repo checkout
-// BeginBatch captures HeadSHA from; WebsterDir, ReportsDir, and PromptsDir
-// are the hubgeometry-resolved _lyx/webster, _lyx/webster/reports, and
-// _lyx/webster/prompts directories.
+// BeginBatch captures HeadSHA from; Layout is the resolved Layout
+// RenderForkPrompt uses for both {{.worktree_root}} (filled from
+// Layout.Cwd) and the PATTERN active check, so the two anchors are always
+// derived from the one Layout the caller resolved rather than from two
+// independently-passed values that could disagree; WebsterDir, ReportsDir,
+// and PromptsDir are the hubgeometry-resolved _lyx/webster,
+// _lyx/webster/reports, and _lyx/webster/prompts directories.
 type BeginDeps struct {
 	Plan         *planparser.Plan
 	Batches      []batcher.Batch
@@ -75,6 +80,7 @@ type BeginDeps struct {
 	Injector     Injector
 	Reed         shuttleengine.ReedOps
 	WorktreeRoot string
+	Layout       *hubgeometry.Layout
 	WebsterDir   string
 	ReportsDir   string
 	PromptsDir   string
@@ -209,7 +215,7 @@ func BeginBatch(deps BeginDeps, batchNumber int) (*BeginResult, error) {
 		return nil, fmt.Errorf("webster: resolve report path: %w", err)
 	}
 
-	prompt, err := RenderForkPrompt(deps.Plan, batch, prevDigest, reportPath, deps.WorktreeRoot, deps.Config.SelfFixCap)
+	prompt, err := RenderForkPrompt(deps.Plan, batch, prevDigest, reportPath, deps.Layout, deps.Config.SelfFixCap)
 	if err != nil {
 		return nil, err
 	}
diff --git a/internal/websterengine/beginbatch_test.go b/internal/websterengine/beginbatch_test.go
index 13947d57..59f88836 100644
--- a/internal/websterengine/beginbatch_test.go
+++ b/internal/websterengine/beginbatch_test.go
@@ -30,6 +30,7 @@ import (
 
 	"github.com/Knatte18/loomyard/internal/batcher"
 	"github.com/Knatte18/loomyard/internal/gitexec"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/modelspec"
 	"github.com/Knatte18/loomyard/internal/planparser"
 	"github.com/Knatte18/loomyard/internal/reedengine"
@@ -270,6 +271,7 @@ func newBeginFixture(t *testing.T) *beginFixture {
 		Injector:     injector,
 		Reed:         reed,
 		WorktreeRoot: worktree,
+		Layout:       &hubgeometry.Layout{Cwd: worktree, WorktreeRoot: worktree, RelPath: "."},
 		WebsterDir:   t.TempDir(),
 		ReportsDir:   t.TempDir(),
 		PromptsDir:   promptsDir,
diff --git a/internal/websterengine/fork-template.md b/internal/websterengine/fork-template.md
index 89453b47..e55f9b35 100644
--- a/internal/websterengine/fork-template.md
+++ b/internal/websterengine/fork-template.md
@@ -5,18 +5,23 @@
      "Read this file and follow it exactly: <this file's own path>" — the
      prompt text itself never sits in Master's own context, so there is no
      paraphrase surface between what Go rendered and what the fork reads.
-     Six markers below are top-level {{.X}} substitutions; stencil.Fill
-     requires all six non-empty. {{.rename_mechanic}} is the one
-     branch-internal marker, reached only inside the {{if .rename_mechanic}}
-     block below — it renders as nothing when the batch has no Moves-bearing
-     card, per the fork-prompt-plan-level-context Shared Decision (see
+     Seven markers below are top-level {{.X}} substitutions; stencil.Fill
+     requires the six original ones non-empty. Two markers are exceptions,
+     via two different mechanisms: {{.rename_mechanic}} is branch-internal,
+     reached only inside the {{if .rename_mechanic}} block below — it
+     renders as nothing when the batch has no Moves-bearing card, per the
+     fork-prompt-plan-level-context Shared Decision (see
      internal/stencil/stencil.go for why only THIS marker may sit inside a
-     conditional). -->
+     conditional); {{.pattern_directive}} is top-level and optional via
+     stencil.FillOptional instead — it renders as nothing when PATTERN is
+     inactive. FillOptional does not retroactively change rename_mechanic's
+     own branch-internal mechanism; the two stay distinct. -->
 
 # Webster fork implementer — one batch of cards, inheriting Master's context
 
 You are an implementer fork for one execution batch, forked in-session from the Master session that is already driving this plan. You never start cold: you inherit Master's whole context — the codebase orientation, the plan's framing, and every constraint Master already read up front — so this prompt is deliberately thin. Your only job is to implement every card below, in order, and write your batch-report as your final action.
 
+{{.pattern_directive}}
 ## You are the IMPLEMENTER, not the driver — never run `lyx webster`
 
 You inherit Master's context, which includes Master's own loop instructions (`begin-batch` / `await-batch` / `record-batch` / `recover-batch`). Those are MASTER's verbs, NOT yours. **NEVER run any `lyx webster` command** — not `await-batch`, not anything. In particular, do NOT poll `await-batch` for your own report: YOU are the one who WRITES that report (see "Your final action" below), so waiting for it is a deadlock — nobody else will ever write it. From this fork's turn, your actions are only: implement your cards (below) on the HOST repo, and write your batch-report file. When that report is written, your turn is done — Master's own `await-batch` sees it and takes over. Ignore any inherited instinct to drive the webster loop.
diff --git a/internal/websterengine/master-template.md b/internal/websterengine/master-template.md
index aef96c05..368064aa 100644
--- a/internal/websterengine/master-template.md
+++ b/internal/websterengine/master-template.md
@@ -4,10 +4,13 @@
      one whole plan run: the long-lived session that reads the codebase and
      the plan once, then forks one implementer per execution batch in-session
      (Claude Code's Agent tool, subagent_type "fork"). Every marker below is
-     a top-level {{.X}} substitution; stencil.Fill requires all seven
-     non-empty and there are no {{if}}/{{range}} conditionals anywhere in
-     this file (a required marker inside a conditional branch would render
-     silently blank when present-but-empty — see internal/stencil/stencil.go). -->
+     a top-level {{.X}} substitution; stencil.Fill requires the seven
+     original ones non-empty and there are no {{if}}/{{range}} conditionals
+     anywhere in this file (a required marker inside a conditional branch
+     would render silently blank when present-but-empty — see
+     internal/stencil/stencil.go). pattern_directive is the eighth marker,
+     and the one optional one: it is filled via stencil.FillOptional and
+     renders as nothing when PATTERN is inactive. -->
 
 # Webster Master — read once, fork per batch, judge only the minimal report
 
@@ -20,6 +23,7 @@
 
 You are the long-lived Master session for one webster plan run. Unlike a fresh process per batch, you stay alive for the WHOLE plan: you read the codebase and the plan once, up front, and every implementer you spawn is an in-session fork that inherits everything you have already read — no cold orientation, no codebase tour, per batch. You never edit code yourself, you never run git against the weft, and you never use a `/model` switch.
 
+{{.pattern_directive}}
 ## Orientation — read this ONCE, up front
 
 Before forking anything, read the codebase's structure and conventions, read `CONSTRAINTS.md` in full, and read the whole plan — every card file, not just the overview — once. This is the stable context every fork you spawn inherits instead of re-deriving it cold each time.
diff --git a/internal/websterengine/recoverbatch.go b/internal/websterengine/recoverbatch.go
index 6ac9f948..1ebb0432 100644
--- a/internal/websterengine/recoverbatch.go
+++ b/internal/websterengine/recoverbatch.go
@@ -213,7 +213,7 @@ func recoverSpawn(deps RecoverDeps, batch batcher.Batch, prior *BatchState, prev
 		return nil, fmt.Errorf("webster: resolve report path: %w", err)
 	}
 
-	prompt, err := RenderForkPrompt(deps.Plan, batch, prevDigest, reportPath, deps.WorktreeRoot, deps.Config.SelfFixCap)
+	prompt, err := RenderForkPrompt(deps.Plan, batch, prevDigest, reportPath, deps.Layout, deps.Config.SelfFixCap)
 	if err != nil {
 		return nil, err
 	}
diff --git a/internal/websterengine/render.go b/internal/websterengine/render.go
index 580e4d69..eead52e2 100644
--- a/internal/websterengine/render.go
+++ b/internal/websterengine/render.go
@@ -27,6 +27,8 @@ import (
 	"strings"
 
 	"github.com/Knatte18/loomyard/internal/batcher"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
+	"github.com/Knatte18/loomyard/internal/pattern"
 	"github.com/Knatte18/loomyard/internal/planparser"
 	"github.com/Knatte18/loomyard/internal/stencil"
 )
@@ -91,10 +93,21 @@ const noSharedDecisions = "none"
 // digest, ALREADY rendered by the caller as a one-line summary — read from
 // state.json's BatchState.Digest, never re-distilled here against a HEAD
 // that may have since moved; an empty prevDigest renders the literal
-// sentinel "none (first batch)" instead of a blank field. reportPath and
-// worktreeRoot are the fork's own OutputFiles target and host checkout, and
-// selfFixCap is the config knob bounding the fork's in-session self-fix
-// attempts.
+// sentinel "none (first batch)" instead of a blank field. reportPath is the
+// fork's own OutputFiles target; l is the resolved Layout this batch's
+// worktree runs in — the single source of both {{.worktree_root}} and the
+// PATTERN active check, so the two anchors can never disagree the way two
+// independently-passed strings could. selfFixCap is the config knob
+// bounding the fork's in-session self-fix attempts.
+//
+// {{.worktree_root}} is filled from l.Cwd, NOT l.WorktreeRoot: every caller
+// of this function assigns l.Cwd to the Layout field this parameter
+// replaced, so filling from WorktreeRoot instead would silently change what
+// the fork prompt calls its worktree root at any RelPath != "." — the exact
+// geometry this plumbing exists for. On a Resolve-built Layout the two are
+// byte-identical at RelPath == ".", but that holds because both are
+// Cwd-equivalent there, not because the fields are interchangeable in
+// general.
 //
 // Per the fork-prompt-plan-level-context Shared Decision, this function
 // ALWAYS injects plan's plan-level "## Shared Decisions" body
@@ -103,8 +116,10 @@ const noSharedDecisions = "none"
 // batch contains at least one card with a non-empty Moves field — every
 // other batch's rendered value for that marker is the empty string, which
 // the fork template's own conditional section (card 28) is responsible for
-// rendering as nothing.
-func RenderForkPrompt(plan *planparser.Plan, batch batcher.Batch, prevDigest string, reportPath, worktreeRoot string, selfFixCap int) ([]byte, error) {
+// rendering as nothing. pattern_directive is injected via
+// pattern.Directive(l, pattern.RoleImplementer) and filled through
+// stencil.FillOptional, so it renders as nothing when PATTERN is inactive.
+func RenderForkPrompt(plan *planparser.Plan, batch batcher.Batch, prevDigest string, reportPath string, l *hubgeometry.Layout, selfFixCap int) ([]byte, error) {
 	digestLine := prevDigest
 	if strings.TrimSpace(digestLine) == "" {
 		digestLine = noPrecedingBatchDigest
@@ -121,15 +136,16 @@ func RenderForkPrompt(plan *planparser.Plan, batch batcher.Batch, prevDigest str
 	}
 
 	values := map[string]string{
-		"cards":            renderBatchCards(batch.Cards),
-		"report_path":      reportPath,
-		"self_fix_cap":     fmt.Sprintf("%d", selfFixCap),
-		"worktree_root":    worktreeRoot,
-		"prev_digest":      digestLine,
-		"shared_decisions": sharedDecisions,
-		"rename_mechanic":  renameMechanic,
+		"cards":             renderBatchCards(batch.Cards),
+		"report_path":       reportPath,
+		"self_fix_cap":      fmt.Sprintf("%d", selfFixCap),
+		"worktree_root":     l.Cwd,
+		"prev_digest":       digestLine,
+		"shared_decisions":  sharedDecisions,
+		"rename_mechanic":   renameMechanic,
+		"pattern_directive": pattern.Directive(l, pattern.RoleImplementer),
 	}
-	prompt, err := stencil.Fill(ForkTemplate(), values)
+	prompt, err := stencil.FillOptional(ForkTemplate(), values, []string{"pattern_directive"})
 	if err != nil {
 		return nil, fmt.Errorf("webster: fill fork template: %w", err)
 	}
@@ -287,8 +303,19 @@ const noIntegrationPromptPath = "none (this plan has no \"## verify:\" section)"
 // write beyond its two contract files is a parent-write audit violation);
 // selfFixCap and pollWaitS are the config knobs Master's prompt states as
 // tuning knobs for its forks and its recover-batch re-polling,
-// respectively.
-func RenderMasterPrompt(plan *planparser.Plan, st *State, outcomePath, summaryPath, integrationPromptPath string, selfFixCap, pollWaitS int) ([]byte, error) {
+// respectively. l is the resolved Layout the caller's own PATTERN active
+// check runs against — RenderMasterPrompt had neither a root nor a Layout
+// parameter before this.
+//
+// pattern_directive is injected via pattern.Directive(l,
+// pattern.RoleOrchestrator) — RoleOrchestrator, not RoleImplementer: this
+// template states in as many words that Master never edits code, so an
+// implementer-worded directive would be one Master cannot carry out; Master
+// qualifies on the context-inheritance clause instead, since its forks are
+// in-session and thin precisely because they inherit everything Master has
+// read. It is filled through stencil.FillOptional, so it renders as nothing
+// when PATTERN is inactive.
+func RenderMasterPrompt(plan *planparser.Plan, st *State, outcomePath, summaryPath, integrationPromptPath string, selfFixCap, pollWaitS int, l *hubgeometry.Layout) ([]byte, error) {
 	integrationPrompt := strings.TrimSpace(integrationPromptPath)
 	if integrationPrompt == "" {
 		integrationPrompt = noIntegrationPromptPath
@@ -302,8 +329,9 @@ func RenderMasterPrompt(plan *planparser.Plan, st *State, outcomePath, summaryPa
 		"integration_prompt_path": integrationPrompt,
 		"self_fix_cap":            fmt.Sprintf("%d", selfFixCap),
 		"poll_wait_s":             fmt.Sprintf("%d", pollWaitS),
+		"pattern_directive":       pattern.Directive(l, pattern.RoleOrchestrator),
 	}
-	prompt, err := stencil.Fill(MasterTemplate(), values)
+	prompt, err := stencil.FillOptional(MasterTemplate(), values, []string{"pattern_directive"})
 	if err != nil {
 		return nil, fmt.Errorf("webster: fill master template: %w", err)
 	}
diff --git a/internal/websterengine/runlevel.go b/internal/websterengine/runlevel.go
index acd09f37..ffaf5e9a 100644
--- a/internal/websterengine/runlevel.go
+++ b/internal/websterengine/runlevel.go
@@ -499,7 +499,7 @@ func Run(deps RunDeps, opts RunOptions) (RunResult, error) {
 		}
 	}
 
-	prompt, err := RenderMasterPrompt(plan, st, outcomePath, summaryPath, integrationPromptPath, deps.Config.SelfFixCap, deps.Config.PollWaitS)
+	prompt, err := RenderMasterPrompt(plan, st, outcomePath, summaryPath, integrationPromptPath, deps.Config.SelfFixCap, deps.Config.PollWaitS, deps.Layout)
 	if err != nil {
 		return RunResult{}, err
 	}
diff --git a/internal/websterengine/template_test.go b/internal/websterengine/template_test.go
index 20f9f977..83cf29f6 100644
--- a/internal/websterengine/template_test.go
+++ b/internal/websterengine/template_test.go
@@ -23,11 +23,22 @@ import (
 	"testing"
 
 	"github.com/Knatte18/loomyard/internal/batcher"
+	"github.com/Knatte18/loomyard/internal/hubgeometry"
 	"github.com/Knatte18/loomyard/internal/planparser"
 	"github.com/Knatte18/loomyard/internal/stencil"
 	"github.com/Knatte18/loomyard/internal/websterengine"
 )
 
+// testLayout returns a *hubgeometry.Layout anchored at a fixed, non-existent
+// "/worktree" path — every RenderForkPrompt test in this file that does not
+// itself exercise pattern_directive uses this fixture, since
+// pattern.Directive's os.Stat on a path that never exists on disk always
+// resolves PATTERN inactive, matching every one of these tests' pre-existing
+// expectation of an empty pattern_directive.
+func testLayout() *hubgeometry.Layout {
+	return &hubgeometry.Layout{Cwd: "/worktree", WorktreeRoot: "/worktree", RelPath: "."}
+}
+
 // requireContains fails the test, naming the missing needle, if text does
 // not contain it. Mirrors builderengine/template_test.go's helper of the
 // same name (package-local — the two packages deliberately do not share a
@@ -123,8 +134,10 @@ const (
 
 // masterTemplateMarkerValues returns a values map with every one of
 // MasterTemplate's seven required top-level markers set to a non-empty
-// placeholder, so a test can fill the template cleanly or delete one key at
-// a time to prove stencil.Fill's per-marker error.
+// placeholder, plus pattern_directive — the one optional marker, filled via
+// stencil.FillOptional — set to a placeholder too, so a test can fill the
+// template cleanly or delete one key at a time to prove
+// stencil.FillOptional's per-marker error.
 func masterTemplateMarkerValues() map[string]string {
 	return map[string]string{
 		"batch_index":             "01 — json-flag — add the --json flag",
@@ -134,6 +147,7 @@ func masterTemplateMarkerValues() map[string]string {
 		"integration_prompt_path": "/lyx/webster/prompts/integration.md",
 		"self_fix_cap":            "2",
 		"poll_wait_s":             "480",
+		"pattern_directive":       "## Constraints — do this before you fork anything\n\n- Read _pattern/PATTERN.md.",
 	}
 }
 
@@ -144,16 +158,19 @@ func masterTemplateMarkerValues() map[string]string {
 // inside the template's own {{if .rename_mechanic}} block — present (so the
 // condition itself evaluates cleanly under stencil's missingkey=error) but
 // empty (so the branch is not taken), exactly RenderForkPrompt's own
-// non-Moves-batch behavior.
+// non-Moves-batch behavior. pattern_directive is the one top-level OPTIONAL
+// marker (filled via stencil.FillOptional, distinct from rename_mechanic's
+// branch-internal mechanism) and is set to a non-empty placeholder here too.
 func forkTemplateMarkerValues() map[string]string {
 	return map[string]string{
-		"cards":            "### Card 2 — list-tests\n\n**What:** add tests\n**Context:** none\n**Edits:**\n- `list_test.go`\n**Creates:** none\n**Deletes:** none\n**Moves:** none",
-		"report_path":      "/webster/reports/02-list-tests.yaml",
-		"self_fix_cap":     "2",
-		"worktree_root":    "/worktree",
-		"prev_digest":      "01-json-flag: done head_sha=abc123",
-		"shared_decisions": "none",
-		"rename_mechanic":  "",
+		"cards":             "### Card 2 — list-tests\n\n**What:** add tests\n**Context:** none\n**Edits:**\n- `list_test.go`\n**Creates:** none\n**Deletes:** none\n**Moves:** none",
+		"report_path":       "/webster/reports/02-list-tests.yaml",
+		"self_fix_cap":      "2",
+		"worktree_root":     "/worktree",
+		"prev_digest":       "01-json-flag: done head_sha=abc123",
+		"shared_decisions":  "none",
+		"rename_mechanic":   "",
+		"pattern_directive": "## Constraints — do this before you write any code\n\n- Read _pattern/PATTERN.md.",
 	}
 }
 
@@ -357,13 +374,17 @@ func TestMasterTemplate_StatesBracketSequenceAndRecoveryLadder(t *testing.T) {
 	requireContains(t, text, "never end your turn")
 }
 
-// TestMasterTemplate_FillsWithAllMarkers asserts stencil.Fill succeeds when
-// every one of MasterTemplate's six required markers is supplied, and fails
-// — naming the marker — when any single one is absent.
+// TestMasterTemplate_FillsWithAllMarkers asserts stencil.FillOptional
+// succeeds when every one of MasterTemplate's seven required markers plus
+// the optional pattern_directive marker is supplied, and fails — naming the
+// marker — when any single REQUIRED one is absent. pattern_directive is
+// deliberately excluded from this deletion sweep: it is the one optional
+// marker (see the template's own banner comment), so deleting it must not
+// error.
 func TestMasterTemplate_FillsWithAllMarkers(t *testing.T) {
 	t.Run("all markers supplied", func(t *testing.T) {
-		if _, err := stencil.Fill(websterengine.MasterTemplate(), masterTemplateMarkerValues()); err != nil {
-			t.Fatalf("stencil.Fill() = %v; want nil", err)
+		if _, err := stencil.FillOptional(websterengine.MasterTemplate(), masterTemplateMarkerValues(), []string{"pattern_directive"}); err != nil {
+			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
 		}
 	})
 
@@ -371,17 +392,58 @@ func TestMasterTemplate_FillsWithAllMarkers(t *testing.T) {
 		t.Run("missing "+marker, func(t *testing.T) {
 			values := masterTemplateMarkerValues()
 			delete(values, marker)
-			_, err := stencil.Fill(websterengine.MasterTemplate(), values)
+			_, err := stencil.FillOptional(websterengine.MasterTemplate(), values, []string{"pattern_directive"})
 			if err == nil {
-				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
+				t.Fatalf("stencil.FillOptional() with %q missing = nil error; want error naming the marker", marker)
 			}
 			if !strings.Contains(err.Error(), marker) {
-				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
+				t.Errorf("stencil.FillOptional() error = %q; want it to name marker %q", err.Error(), marker)
 			}
 		})
 	}
 }
 
+// TestMasterTemplate_PatternDirectiveOptional asserts pattern_directive
+// behaves as an optional marker: an empty value renders cleanly with no
+// leftover `{{`, no orphan `## Constraints` heading, and no stray
+// blank-line block where the directive would have sat, and a non-empty
+// value places the directive block ahead of the first work instruction
+// ("## Orientation").
+func TestMasterTemplate_PatternDirectiveOptional(t *testing.T) {
+	t.Run("empty pattern_directive renders cleanly", func(t *testing.T) {
+		values := masterTemplateMarkerValues()
+		values["pattern_directive"] = ""
+		got, err := stencil.FillOptional(websterengine.MasterTemplate(), values, []string{"pattern_directive"})
+		if err != nil {
+			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
+		}
+		text := string(got)
+		if strings.Contains(text, "{{") {
+			t.Errorf("rendered output contains leftover {{: %q", text)
+		}
+		if strings.Contains(text, "## Constraints") {
+			t.Errorf("rendered output contains an orphan ## Constraints heading: %q", text)
+		}
+		if strings.Contains(text, "\n\n\n\n") {
+			t.Errorf("rendered output contains a stray blank-line block: %q", text)
+		}
+	})
+
+	t.Run("non-empty pattern_directive precedes the first work instruction", func(t *testing.T) {
+		values := masterTemplateMarkerValues()
+		got, err := stencil.FillOptional(websterengine.MasterTemplate(), values, []string{"pattern_directive"})
+		if err != nil {
+			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
+		}
+		text := string(got)
+		directiveIdx := strings.Index(text, values["pattern_directive"])
+		workIdx := strings.Index(text, "## Orientation")
+		if directiveIdx == -1 || workIdx == -1 || directiveIdx >= workIdx {
+			t.Errorf("pattern_directive (idx %d) does not precede the first work instruction (idx %d)", directiveIdx, workIdx)
+		}
+	})
+}
+
 // TestForkTemplate_PinsReportSchemaKeys asserts the embedded fork template's
 // bytes carry the minimal fork-return contract's field names verbatim
 // (status, head_sha, deviations — never the v2 report's tests/stuck_reason/
@@ -412,15 +474,21 @@ func TestForkTemplate_PinsReportSchemaKeys(t *testing.T) {
 	requireNotContains(t, text, "tests: green")
 }
 
-// TestForkTemplate_FillsWithAllMarkers asserts stencil.Fill succeeds when
-// every one of ForkTemplate's six required markers is supplied, and fails —
-// naming the marker — when any single one is absent. rename_mechanic is
-// deliberately excluded: it is a branch-internal marker, never required at
-// the top level (see TestRenderForkPrompt_InjectsRenameMechanicOnlyForMovesBearingBatch).
+// TestForkTemplate_FillsWithAllMarkers asserts stencil.FillOptional succeeds
+// when every one of ForkTemplate's six required markers plus the optional
+// pattern_directive marker is supplied, and fails — naming the marker —
+// when any single REQUIRED one is absent. rename_mechanic and
+// pattern_directive are both excluded from this deletion sweep, via two
+// different mechanisms: rename_mechanic is a branch-internal marker, never
+// required at the top level (see
+// TestRenderForkPrompt_InjectsRenameMechanicOnlyForMovesBearingBatch);
+// pattern_directive is the one top-level OPTIONAL marker, exempted via
+// stencil.FillOptional's optional list, so deleting it must not error
+// either.
 func TestForkTemplate_FillsWithAllMarkers(t *testing.T) {
 	t.Run("all markers supplied", func(t *testing.T) {
-		if _, err := stencil.Fill(websterengine.ForkTemplate(), forkTemplateMarkerValues()); err != nil {
-			t.Fatalf("stencil.Fill() = %v; want nil", err)
+		if _, err := stencil.FillOptional(websterengine.ForkTemplate(), forkTemplateMarkerValues(), []string{"pattern_directive"}); err != nil {
+			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
 		}
 	})
 
@@ -428,17 +496,85 @@ func TestForkTemplate_FillsWithAllMarkers(t *testing.T) {
 		t.Run("missing "+marker, func(t *testing.T) {
 			values := forkTemplateMarkerValues()
 			delete(values, marker)
-			_, err := stencil.Fill(websterengine.ForkTemplate(), values)
+			_, err := stencil.FillOptional(websterengine.ForkTemplate(), values, []string{"pattern_directive"})
 			if err == nil {
-				t.Fatalf("stencil.Fill() with %q missing = nil error; want error naming the marker", marker)
+				t.Fatalf("stencil.FillOptional() with %q missing = nil error; want error naming the marker", marker)
 			}
 			if !strings.Contains(err.Error(), marker) {
-				t.Errorf("stencil.Fill() error = %q; want it to name marker %q", err.Error(), marker)
+				t.Errorf("stencil.FillOptional() error = %q; want it to name marker %q", err.Error(), marker)
 			}
 		})
 	}
 }
 
+// TestForkTemplate_PatternDirectiveOptional asserts pattern_directive
+// behaves as an optional marker: an empty value renders cleanly with no
+// leftover `{{`, no orphan `## Constraints` heading, and no stray
+// blank-line block where the directive would have sat, and a non-empty
+// value places the directive block ahead of the first work instruction
+// ("## You are the IMPLEMENTER"). A third case pins this file's own extra
+// requirement: an empty pattern_directive together with an empty
+// rename_mechanic renders with neither an orphan `## Constraints` heading
+// nor an orphan `## Rename mechanic` heading.
+func TestForkTemplate_PatternDirectiveOptional(t *testing.T) {
+	t.Run("empty pattern_directive renders cleanly", func(t *testing.T) {
+		values := forkTemplateMarkerValues()
+		values["pattern_directive"] = ""
+		got, err := stencil.FillOptional(websterengine.ForkTemplate(), values, []string{"pattern_directive"})
+		if err != nil {
+			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
+		}
+		text := string(got)
+		if strings.Contains(text, "{{") {
+			t.Errorf("rendered output contains leftover {{: %q", text)
+		}
+		if strings.Contains(text, "## Constraints") {
+			t.Errorf("rendered output contains an orphan ## Constraints heading: %q", text)
+		}
+		// Scope the stray-blank-line check to the region immediately
+		// preceding the first work heading — the pattern_directive
+		// insertion point — rather than the whole document: this file's
+		// own pre-existing {{if .rename_mechanic}} block (unrelated to
+		// pattern_directive) collapses to a wider gap of its own when
+		// rename_mechanic is empty, which is not this test's concern.
+		beforeHeading := text[:strings.Index(text, "## You are the IMPLEMENTER")]
+		if strings.HasSuffix(beforeHeading, "\n\n\n\n") {
+			t.Errorf("rendered output contains a stray blank-line block ahead of the first work heading: %q", beforeHeading)
+		}
+	})
+
+	t.Run("non-empty pattern_directive precedes the first work instruction", func(t *testing.T) {
+		values := forkTemplateMarkerValues()
+		got, err := stencil.FillOptional(websterengine.ForkTemplate(), values, []string{"pattern_directive"})
+		if err != nil {
+			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
+		}
+		text := string(got)
+		directiveIdx := strings.Index(text, values["pattern_directive"])
+		workIdx := strings.Index(text, "## You are the IMPLEMENTER")
+		if directiveIdx == -1 || workIdx == -1 || directiveIdx >= workIdx {
+			t.Errorf("pattern_directive (idx %d) does not precede the first work instruction (idx %d)", directiveIdx, workIdx)
+		}
+	})
+
+	t.Run("empty pattern_directive and empty rename_mechanic together leave neither heading orphaned", func(t *testing.T) {
+		values := forkTemplateMarkerValues()
+		values["pattern_directive"] = ""
+		values["rename_mechanic"] = ""
+		got, err := stencil.FillOptional(websterengine.ForkTemplate(), values, []string{"pattern_directive"})
+		if err != nil {
+			t.Fatalf("stencil.FillOptional() = %v; want nil", err)
+		}
+		text := string(got)
+		if strings.Contains(text, "## Constraints") {
+			t.Errorf("rendered output contains an orphan ## Constraints heading: %q", text)
+		}
+		if strings.Contains(text, "## Rename mechanic") {
+			t.Errorf("rendered output contains an orphan ## Rename mechanic heading: %q", text)
+		}
+	})
+}
+
 // TestTemplates_NoV2TokensRemain asserts neither embedded template carries
 // any of the three dropped v2 concepts — oversized batches, deferred-verify
 // chains, and the per-batch "## Scope" section — anywhere in its bytes.
@@ -481,7 +617,7 @@ func TestRenderForkPrompt_InjectsPrevDigestSentinelOnlyWhenEmpty(t *testing.T) {
 	}}
 
 	t.Run("empty prevDigest renders the first-batch sentinel", func(t *testing.T) {
-		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-seam-extensions.yaml", "/worktree", 2)
+		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-seam-extensions.yaml", testLayout(), 2)
 		if err != nil {
 			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
 		}
@@ -495,7 +631,7 @@ func TestRenderForkPrompt_InjectsPrevDigestSentinelOnlyWhenEmpty(t *testing.T) {
 		// prevDigest; what this case actually proves is that the supplied
 		// digest line itself reaches the rendered prompt verbatim.
 		digest := "01-seam-extensions: done head_sha=abc123"
-		got, err := websterengine.RenderForkPrompt(plan, batch, digest, "/reports/02-webster-foundation.yaml", "/worktree", 2)
+		got, err := websterengine.RenderForkPrompt(plan, batch, digest, "/reports/02-webster-foundation.yaml", testLayout(), 2)
 		if err != nil {
 			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
 		}
@@ -519,7 +655,7 @@ func TestRenderForkPrompt_RendersWhatProseOverIntent(t *testing.T) {
 			Intent: "create r3a.md marker",
 			What:   "Create `r3a.md` containing the single line `OK-A` and commit it.",
 		}}}
-		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-alpha.yaml", "/worktree", 2)
+		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-alpha.yaml", testLayout(), 2)
 		if err != nil {
 			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
 		}
@@ -533,7 +669,7 @@ func TestRenderForkPrompt_RendersWhatProseOverIntent(t *testing.T) {
 		batch := batcher.Batch{Cards: []planparser.Card{{
 			Number: 1, Slug: "alpha", Title: "alpha", Intent: "create r3a.md marker",
 		}}}
-		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-alpha.yaml", "/worktree", 2)
+		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-alpha.yaml", testLayout(), 2)
 		if err != nil {
 			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
 		}
@@ -556,7 +692,7 @@ func TestRenderForkPrompt_RendersPinnedCommitSubject(t *testing.T) {
 			Number: 1, Slug: "alpha", Title: "alpha", Intent: "add the flag",
 			Commit: "1: json-flag",
 		}}}
-		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-alpha.yaml", "/worktree", 2)
+		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-alpha.yaml", testLayout(), 2)
 		if err != nil {
 			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
 		}
@@ -575,7 +711,7 @@ func TestRenderForkPrompt_RendersPinnedCommitSubject(t *testing.T) {
 		batch := batcher.Batch{Cards: []planparser.Card{{
 			Number: 1, Slug: "alpha", Title: "alpha", Intent: "add the flag",
 		}}}
-		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-alpha.yaml", "/worktree", 2)
+		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-alpha.yaml", testLayout(), 2)
 		if err != nil {
 			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
 		}
@@ -600,7 +736,7 @@ func TestRenderForkPrompt_InjectsSharedDecisionsAlways(t *testing.T) {
 
 	t.Run("non-empty SharedDecisions passes through verbatim", func(t *testing.T) {
 		plan := testPlan("### Decision: json-envelope-reuse\n\n- **Decision:** reuse output.Ok.", "")
-		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-json-flag.yaml", "/worktree", 2)
+		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-json-flag.yaml", testLayout(), 2)
 		if err != nil {
 			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
 		}
@@ -609,7 +745,7 @@ func TestRenderForkPrompt_InjectsSharedDecisionsAlways(t *testing.T) {
 
 	t.Run("empty SharedDecisions renders the none sentinel", func(t *testing.T) {
 		plan := testPlan("", "")
-		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-json-flag.yaml", "/worktree", 2)
+		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-json-flag.yaml", testLayout(), 2)
 		if err != nil {
 			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
 		}
@@ -634,7 +770,7 @@ func TestRenderForkPrompt_InjectsRenameMechanicOnlyForMovesBearingBatch(t *testi
 			{Number: 4, Slug: "helptree-rename", Title: "helptree-rename", Intent: "rename the row mapper",
 				Moves: []planparser.MovePair{{Old: "internal/boardengine/rows.go", New: "internal/boardengine/rowsjson.go"}}},
 		}}
-		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/04-helptree-rename.yaml", "/worktree", 2)
+		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/04-helptree-rename.yaml", testLayout(), 2)
 		if err != nil {
 			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
 		}
@@ -645,7 +781,7 @@ func TestRenderForkPrompt_InjectsRenameMechanicOnlyForMovesBearingBatch(t *testi
 		batch := batcher.Batch{Cards: []planparser.Card{
 			{Number: 1, Slug: "json-flag", Title: "json-flag", Intent: "add the --json flag"},
 		}}
-		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-json-flag.yaml", "/worktree", 2)
+		got, err := websterengine.RenderForkPrompt(plan, batch, "", "/reports/01-json-flag.yaml", testLayout(), 2)
 		if err != nil {
 			t.Fatalf("RenderForkPrompt() = _, %v; want nil error", err)
 		}
diff --git a/manifest/designs/codeintel-redesign.md b/manifest/designs/codeintel-redesign.md
index 29ed24a6..f9a5a67b 100644
--- a/manifest/designs/codeintel-redesign.md
+++ b/manifest/designs/codeintel-redesign.md
@@ -1,86 +1,109 @@
-# codeintel — multi-language redesign (Someday, deprioritized)
+# codeintel — multi-language code-intelligence (Planned)
 
-> **Status: Design exists, not scheduled.** Full four-layer architecture designed during the vacation-time discussion. **Deprioritized because it isn't required for a first working `loom run`** — not abandoned. This is a *different, larger* design than what's actually shipped today: the current `internal/codeintelengine` (see its package doc) is a single-language (Go-only), daemon-free, no-toolchain-manager implementation. This doc describes the future redesign; it does not describe what exists.
+> **Status: Planned — V1 is Go-only, architected for multi-language.** Promoted from Someday (2026-07) after a design pass that settled three things the earlier sketch left open: the **consumer surface** (a `lyx codeintel` CLI plus an in-process Go API — *no MCP*), the **daemon contract** (a single `EnsureServer` seam with two swappable spawn strategies behind it), and the **V1 scope** (build and prove *both* strategies against `gopls`, populate the registry for Go only, lock its format so the other languages drop in without a breaking change). This doc supersedes the earlier four-layer/MCP write-up. It is **independent of the rest of the Planned queue** — it reads code, drives a language server, returns locations, with no dependency on board / native-clients / fabric-v2 / loom — so it can be built now, in parallel. It is a *different, larger* design than what ships today: the current `internal/codeintelengine` (see its package doc) is a single-language (Go), daemon-free, no-toolchain-manager implementation; V1 **extends** it, it does not replace it wholesale.
 
-## Motivation (unchanged from the original proposal)
+## Motivation
 
-Webster forks (and the planner) currently discover "what does this symbol touch" via text search (Grep/Glob) plus manual reading — imprecise (false positives from name collisions, silent misses) and expensive (every false-positive hit costs a full LLM round-trip). A working codeintel replaces this with fast, deterministic, compiler-derived lookups, and is what makes [plan-format v3](../../docs/reference/plan-format-v3.md)'s symbol fields (`creates-symbols`/`edits-symbols`/`reads-symbols`) trustworthy enough to write into a card at all — without it, they degrade to guesses (see `internal/websterengine`'s package documentation for the resolution of this exact machine-mismatch problem).
+Webster forks (and the planner, and reviewers) currently discover "what does this symbol touch" via text search (Grep/Glob) plus manual reading — imprecise (false positives from name collisions, silent misses) and expensive (every false-positive hit costs a full LLM round-trip). A working codeintel replaces this with fast, deterministic, compiler-derived lookups, and is what makes [plan-format v3](../../docs/reference/plan-format-v3.md)'s symbol fields (`creates-symbols`/`edits-symbols`/`reads-symbols`) trustworthy enough to write into a card at all — without it, they degrade to guesses (see `internal/websterengine`'s package documentation for the resolution of this exact machine-mismatch problem).
 
-**What codeintel is not:** not a semantic/conceptual index ("what have we written that's thematically similar" — see [semantic-index.md](semantic-index.md), a separate, further-out idea, not part of this proposal); not a replacement for raddle (raddle answers "where does this belong and why," codeintel answers "what exactly is affected"); not a DAG builder itself (it provides raw reference/definition facts; mechanical DAG-derivation is webster's own logic — see `internal/websterengine`'s package documentation).
+The payoff is not lyx-runtime CPU speed. It is (a) planner/implementer/reviewer stop gissing "where is this defined / used" and stop paying an LLM round per false grep hit — fewer wasted agent rounds, less token spend, shorter wall-clock; and (b) the plan-format-v3 symbol fields become trustworthy enough to exist.
 
-## Four-layer architecture
+**What codeintel is not:** not a semantic/conceptual index ("what have we written that's thematically similar" — see [semantic-index.md](semantic-index.md), a separate, further-out idea); not a replacement for raddle (raddle answers "where does this belong and why," codeintel answers "what exactly is affected"); not a DAG builder itself (it provides raw reference/definition facts; mechanical DAG-derivation is webster's own logic — see `internal/websterengine`'s package documentation); and — settled in this design pass — **not an LSP server.** lyx never *serves* LSP. It *consumes* published language-server binaries (gopls first) through an embedded LSP client. "codeintel" = lyx gains code-intelligence powered by LSP, not lyx becomes a language server.
 
-### 1. Toolchain manager
+## The two consumer entry points — one engine, no MCP
 
-Owns installation and pinning of the underlying language-server binaries.
+Everything sits on one `codeintelengine`; only the entry differs:
 
-- Checks whether the correct **pinned version** exists in a codeintel-owned cache directory (e.g. `~/.cache/lyx/tools/<lang>/<version>/`); installs deterministically if missing (`go install ...@<pinned-version>`, or a direct prebuilt-release download) — never relies on the host already having the language's own toolchain.
-- **Hard constraint: prefer self-contained, runtime-free binaries** over anything needing an external runtime on the host. This ruled out the official `roslyn-language-server` and `csharp-ls` (both require the .NET SDK) in favor of OmniSharp-Roslyn's self-contained builds.
-- Pins an **exact** version, not "latest" — unlike editor extensions optimizing for one interactive user tolerating drift, codeintel needs the same input to produce the same output across machines and over time.
+- **Go-orchestrated code** (webster's own DAG-derivation) calls the engine **in-process** — `codeintelengine.References(...)` / `Definition(...)`, direct function calls, no subprocess, no protocol. Webster does *not* shell out to its own CLI.
+- **LLM agents** (planner, webster forks, implementer, reviewer) call **`lyx codeintel references|definition|symbol <query>`** — a thin Cobra command over the same engine.
 
-### 2. Daemon/supervisor
+**Why a CLI and not an MCP server.** The query surface is 2–3 fixed calls, so MCP's value (dynamic tool discovery + typed schemas) is near zero, while its cost (server registration, per-worktree config, connection lifecycle, a Claude-Code-specific mechanism to maintain) is not. A bash CLI is one code path (fits the CLI/Cobra invariant — `Command()`/`RunCLI`, `Short`, help-tree tests), engine-neutral (works for any provider, unlike MCP — see `CLAUDE.md`'s provider-agnostic-engines rule), and needs no new agent skill (agents already parse `file:line:col` from grep; swapping the grep call for a `lyx codeintel` call is zero new learning). On the deployment target (Linux) the per-call latency delta between a subprocess and an MCP message is a few ms of framework overhead — invisible against multi-second LLM turns and against the LSP query itself, which is identical for both paths. MCP would not even remove the daemon (a shared warm server is needed across ephemeral agents regardless), so it buys no throughput and no architectural saving. MCP can be revisited later *only if* dynamic discovery ever becomes worth it; it is not in V1.
 
-Owns process lifecycle for each running language-server instance. Modeled directly on the existing wiki-daemon pattern (`millhouse/plugins/mill/scripts/wiki/_client.py`), ported to Go, generalized to be **language-parameterized**:
+**Hard constraint on the CLI:** it is a *thin client over the warm daemon* — it must never boot the language server itself. A cold server per invocation loses to grep; the whole "faster than grep" claim rests on the server being warm when the agent asks (see `EnsureServer` below). The CLI is just another client of the daemon, side by side with the in-process Go API.
 
-- **State file per `(language, worktree-root)`** — not a single global file, since multiple language servers may be live at once.
-- **Auto-spawn on demand** — a health check *is* the "start if not running" path, no separate check step.
-- **Two-part staleness check**: a recorded process is trusted only if (a) the PID is alive *and* (b) it actually answers (a cheap real LSP call, e.g. `workspace/symbol` with an empty query) — not PID-liveness alone, since an LSP server can hang without the process dying.
-- **Detached spawn** — survives the spawning process exiting (`start_new_session=True` on Unix, `CREATE_BREAKAWAY_FROM_JOB` on Windows), no `systemd`/OS-service dependency.
-- **Version-forced restart** — if the client's compiled-in protocol/tool version doesn't match what's recorded in the state file, kill and respawn.
-- **Not reused from the wiki-daemon:** its bespoke line-delimited JSON-over-TCP wire protocol — codeintel's daemon speaks real LSP (JSON-RPC 2.0, `Content-Length` framing) to the underlying server; only the lifecycle/supervision layer is shared.
+**Batch mode:** the CLI accepts several symbols in one call (`lyx codeintel references Foo Bar Baz`) to amortize process startup across lookups — cheap insurance against Windows' expensive process creation, and unnecessary but harmless on Linux.
 
-### 3. LSP client
+**Agent wiring** is prompt-injection near the decision point, not a static CLAUDE.md line — put the "you may call `lyx codeintel`" instruction next to the relevant field (webster: beside `edits-symbols`, only when the language's daemon is confirmed reachable). The reviewer/implementer anchor (a treadle/burler round has no `edits-symbols` field to hang it on) is an **open integration point**, resolved in the later consumer-wiring slice, not V1.
 
-`initialize`/`initialized` handshake, `textDocument/references`, `textDocument/definition`, `callHierarchy/*`. Standard for the great majority of servers (gopls, ty, rust-analyzer, OmniSharp-Roslyn), with per-adapter escape hatches for known non-standard behavior (e.g. Roslyn's own official server needs a `solution/open` call after `initialize`; OmniSharp doesn't have this requirement, another reason it's preferred).
+## `EnsureServer` — the one layer-2 contract
 
-### 4. Language registry
+A single function is the whole lifecycle layer to everything above it:
 
-Maps `language → {binary, pinned version, CLI flags, protocol quirks, install method, has_native_daemon}`. **v1 scope: Go only (`gopls`)** — covers loomyard's own codebase; no other language is a proven, immediate need. Documented future candidates:
+```go
+EnsureServer(lang, worktreeRoot) -> LSPConn
+```
 
-- **Go → `gopls`.** Pure Go binary, no external runtime. **Has a native shared-daemon mode** (`gopls -remote=auto`, confirmed in production use by Anthropic's own official `gopls-lsp` Claude Code plugin) — for Go specifically, codeintel's daemon layer can likely be a thin wrapper delegating to gopls's own remote mode rather than reimplementing full supervision.
-- **Python → `ty`** (Astral, Rust-based, self-contained), preferred over Pyright (Node-dependent, fails the runtime-free filter). No known native shared-daemon mode — codeintel's own daemon wrapper carries the full weight here; `ty` markets itself as fast even cold, so measure whether a full daemon is even necessary before building one for this language.
-- **C# → OmniSharp-Roslyn** (self-contained platform builds), preferred over the official `roslyn-language-server` and `csharp-ls` (both require the .NET SDK).
+CLI, Go API, and the layer-3 protocol calls all go through it and never reason about lifecycle. Its body is four steps:
 
-### Public interface
+1. **Toolchain** (layer 1): is the binary present at the pinned version? Install deterministically if missing.
+2. **Spawn strategy** — selected by the registry's `has_native_daemon` flag (see below).
+3. **Probe (shared by both strategies):** does the server *answer*? A cheap `workspace/symbol` with an empty query. This runs regardless of strategy — even `gopls -remote=auto` can hand back a connection to a hung shared instance, so PID-liveness alone is never trusted.
+4. Return a warm `LSPConn`.
 
-```go
-type Location struct {
-    File   string
-    Line   int
-    Column int
-}
-
-func References(symbol string) ([]Location, error)
-func Definition(symbol string) (Location, error)
-```
+Steps 2+3 *are* the doc's old "a health check is the start-if-not-running path": every CLI call invokes `EnsureServer` blindly; on the warm path that is only the probe (cheap), on the cold path it spawns. That is exactly what keeps the thin-CLI-over-warm-daemon model true. **The two strategies collapse under one contract — they differ only in who spawns:**
 
-Consumers never need to know whether they're talking to `gopls` or OmniSharp.
+- **`native`** (Go / `gopls`): run `gopls -remote=auto`; gopls itself dedups and spawns the shared instance. Confirmed in production use by Anthropic's own official `gopls-lsp` Claude Code plugin. We build no supervisor for Go in production.
+- **`supervised`** (Python / `ty`, C# / OmniSharp — no native shared-daemon mode): **we** own it — a **state file per `(language, worktree-root)`** (not one global file, since multiple language servers may be live), auto-spawn on the health check, **two-part staleness** (PID alive *and* answers), **detached spawn** that survives the spawning process (`start_new_session=True` on Unix, `CREATE_BREAKAWAY_FROM_JOB` on Windows — no `systemd`/OS-service dependency), and **version-forced restart** when the client's compiled-in protocol version doesn't match the state file.
 
-## The name-resolution path: `workspace/symbol`
+This layer is modeled on the existing wiki-daemon pattern (`millhouse/plugins/mill/scripts/wiki/_client.py`), ported to Go and generalized to be language-parameterized. **Not reused from it:** the wiki-daemon's bespoke line-delimited JSON-over-TCP wire protocol — codeintel speaks real LSP (see layer 3); only the lifecycle/supervision shape is shared.
 
-Given a bare symbol name (no explicit position), the engine issues `workspace/symbol` and requires exactly one candidate — zero is "not found," more than one is "ambiguous" (every candidate returned as a formatted `file:line:col` string so the caller can disambiguate without a second broader search). A server that doesn't advertise `workspaceSymbolProvider` fails this path immediately rather than attempting the call and getting an undefined result. An explicit `file:line:col` position bypasses this resolver entirely.
+## Proving layer 2 in a Go-only V1 — supervise plain `gopls`
 
-## Feedback from external review (folded in)
+The risk in a Go-only V1 is that the `supervised` strategy — the one we actually own — is never exercised, because production Go takes the `native` path. A daemon interface designed but never run is exactly the kind that turns out mis-shaped the day OmniSharp is built against it. Mitigation, at near-zero cost, since gopls is already installed: **V1 builds `supervised` and tests it against a plain `gopls`** (a bare `gopls` process *we* spawn, state-file, probe, and restart — not `-remote=auto`). Production Go still uses `native`; but layer 2 and the strategy-selection seam are proven working against a real LSP server before any non-Go dependency exists. Adding `ty`/OmniSharp later is then a registry entry + adapter quirks against machinery already known to run.
 
-- **Design-lock now, implementation later, and that's fine as a deliberate split:** the four-layer architecture looked heavy to support gopls alone (which has its own `-remote=auto`), but given codeintel is explicitly multi-language (Go, Python, C#), the daemon/supervisor layer is necessary infrastructure, not premature generalization — Python and C# servers don't share gopls's native daemon behavior. **Lock now:** the registry format and the `References`/`Definition` public interface, across all three planned languages, so the registry never needs a breaking shape change later. **Defer:** build and test the Go (`gopls`) adapter first; let Python/C# adapters wait until there's a concrete second consumer, as long as the registry format already has room for them.
-- **Per-language snapshot keys, not one shared `codeintel` key** (see [`internal/fabricengine`](../../internal/fabricengine/doc.go)) — use `codeintel-go`/`codeintel-py`/`codeintel-cs` so one language's daemon downtime can't block or falsely-advance tracking for the others.
-- **Tag decisions as "ported from Millhouse" vs. "new for this rewrite"** when consolidating — several assumptions here (grep-based search miss rate, LSP cold-start cost) may already be observed facts from months of Millhouse production use, not open questions for the Go rewrite. This distinction changes how much scrutiny each decision still needs before being treated as settled.
+## The LSP client (layer 3)
+
+The shared protocol layer, identical across all languages: `initialize`/`initialized`, `textDocument/references`, `textDocument/definition`, `callHierarchy/*`, `workspace/symbol`. JSON-RPC 2.0 with `Content-Length` framing. Per-adapter escape hatches for known non-standard behavior (e.g. Roslyn's official server needs a `solution/open` after `initialize`; OmniSharp doesn't — one reason it's preferred). Because layer 3 is shared and the `EnsureServer` seam hides the strategy, **the CLI surface is identical across languages**: `lyx codeintel references Foo` looks the same whether the backend is gopls or OmniSharp — the engine routes by the worktree's registered language. (V1 is implicitly Go; cross-language routing from a bare name is a concern only once a second language has a real consumer.)
+
+## Toolchain manager (layer 1)
+
+Owns installation and pinning of the underlying language-server binaries.
+
+- Checks whether the correct **pinned version** exists in a codeintel-owned cache directory (e.g. `~/.cache/lyx/tools/<lang>/<version>/`); installs deterministically if missing (`go install ...@<pinned-version>`, or a direct prebuilt-release download) — never relies on the host already having the language's own toolchain.
+- **Hard constraint: prefer self-contained, runtime-free binaries.** This ruled out the official `roslyn-language-server` and `csharp-ls` (both require the .NET SDK) in favor of OmniSharp-Roslyn's self-contained builds, and Pyright (Node-dependent) in favor of `ty` (Astral, Rust, self-contained).
+- Pins an **exact** version, not "latest" — unlike editor extensions optimizing for one interactive user tolerating drift, codeintel needs the same input to produce the same output across machines and over time.
+
+## Language registry (layer 4)
+
+Maps `language → {binary, pinned version, CLI flags, protocol quirks, install method, has_native_daemon}`. **The format is locked now, all fields, across the three planned languages** (Go / `gopls`, Python / `ty`, C# / OmniSharp-Roslyn) so it never needs a breaking shape change later — even though **V1 populates Go only**. This is the "easy to add a language" contract made concrete: adding a language = **one registry entry + an optional protocol-quirk hook + a `has_native_daemon` choice** (which strategy `EnsureServer` picks). No change to the client, the CLI, or the engine API. That contract — not the gopls adapter — is the deliverable.
+
+## Name resolution and exit codes
+
+Given a bare symbol name (no explicit position), the engine issues `workspace/symbol` and interprets the result as a small, deterministic contract, surfaced through the CLI's exit code:
+
+- **exactly one candidate → found.** Exit `0`; the location is printed as `file:line:col`.
+- **zero candidates → not found.** Exit `1`; empty stdout.
+- **more than one → ambiguous.** Exit `2`; *every* candidate printed as `file:line:col` on stdout, so the caller disambiguates without a second broader search (still one precise answer set vs. N grep hits).
+
+A server that doesn't advertise `workspaceSymbolProvider` fails this path immediately rather than attempting the call and getting an undefined result. An explicit `file:line:col` position bypasses the resolver entirely. The in-process Go API returns the same three cases as typed values rather than exit codes.
+
+## V1 scope — what lands
+
+- The `EnsureServer` contract, with **both** spawn strategies coded.
+- `native` (`gopls -remote=auto`) as the production Go path.
+- `supervised` built and **tested against a plain `gopls`** (state-file + probe + restart), proving layer 2 and the strategy seam.
+- Layer 3 LSP client (shared).
+- The registry format locked (all fields), Go populated.
+- `lyx codeintel references|definition|symbol` CLI + the in-process Go API, both over `EnsureServer`.
+- The exit-code contract above.
+- **Windows works** (detached spawn on both OSes) but is **not optimized for** — subprocess-spawn cost is a dev-only concern; Linux is the deployment target and process spawn there is cheap.
+
+**Deferred, explicitly:** the `ty` (Python) and OmniSharp (C#) adapters — registry entries + adapter quirks against the proven machinery, when a concrete second consumer exists. And the **consumer wiring** (planner/webster/reviewer prompt-injection, the reviewer anchor) is its own later integration slice — V1 delivers the engine + CLI + Go API and is tested against loomyard's own Go codebase, not against a live producer.
+
+## Fabric / snapshot integration
+
+Per-language snapshot keys, not one shared `codeintel` key (see [`internal/fabricengine`](../../internal/fabricengine/doc.go)) — `codeintel-go`/`codeintel-py`/`codeintel-cs` — so one language's daemon downtime can't block or falsely-advance tracking for the others. This concerns raddle/tracking, not the live-query CLI path (which reflects the daemon's current view of the worktree); V1's interactive queries need no snapshot machinery.
 
 ## Known limitations
 
-- **Cannot resolve symbols that don't exist yet** — a structural limit, not a bug. Mitigated at the webster/plan-format level by plan-internal name matching for not-yet-existing symbols (see `internal/websterengine`'s package documentation, the dead-DAG-seam section), not by codeintel itself.
+- **Cannot resolve symbols that don't exist yet** — a structural limit, not a bug. Mitigated at the webster/plan-format level by plan-internal name matching for not-yet-existing symbols (see `internal/websterengine`'s package documentation, the dead-DAG-seam section), not by codeintel itself. This bites the planner consumer most, the reviewer/implementer least.
 - Reduced precision around generics, reflection, and heavy dynamic-dispatch patterns (DI containers, `dynamic` in C#) — worth explicit measurement per language before trusting codeintel as a hard collision judge, especially for C#.
 - No cross-worktree cache sharing — each active worktree needs its own loaded/type-checked view.
-- Cold-start cost is real and version/repo-size-dependent — should be measured empirically (`codeintel-spike` wiki task reportedly already has this data for the Go-only in-process arm).
-
-## Consumers and usage pattern
-
-- **Planner:** verifies symbol names against the real codebase before writing `edits-symbols`/`reads-symbols` into a card (see [plan-format-v3.md](../../docs/reference/plan-format-v3.md)'s deferred symbol fields).
-- **Webster forks:** conditional prompt injection — only when a card has declared `edits-symbols` *and* the relevant language's codeintel daemon is confirmed reachable. Put the instruction in the card/task prompt itself, near the relevant field, not in a static CLAUDE.md — context proximity to the decision point matters more than instruction placement in general system-level files.
-- Two consumer interfaces on one daemon: a **Go API** for webster's own orchestration (direct function calls, no protocol overhead), and an **MCP server** for Claude agents that need dynamic tool discovery. Prefer this self-built/self-pinned path over Claude Code's built-in `ENABLE_LSP_TOOL` flag for production use — that path is explicitly documented upstream as "raw," undocumented, and subject to change, conflicting with this project's determinism requirements; fine for quick interactive experimentation only.
+- Cold-start cost is real and version/repo-size-dependent — should be measured empirically; the warm-daemon design exists precisely so ephemeral agents don't pay it.
 
 ## Related
 
 - [plan-format-v3.md](../../docs/reference/plan-format-v3.md) — the symbol fields this module makes trustworthy.
 - [`internal/fabricengine`](../../internal/fabricengine/doc.go) — per-language snapshot-key notification.
-- `internal/codeintelengine` package doc — the current, simpler, shipped implementation this design eventually redesigns (not superseded yet — no work has started on this redesign).
+- [semantic-index.md](semantic-index.md) — the separate, further-out conceptual-search idea codeintel is explicitly *not*.
+- `internal/codeintelengine` package doc — the current, simpler, shipped implementation V1 extends (Go-only, daemon-free).
diff --git a/manifest/designs/fabric-unified-view.md b/manifest/designs/fabric-unified-view.md
index c05ac59e..02768d34 100644
--- a/manifest/designs/fabric-unified-view.md
+++ b/manifest/designs/fabric-unified-view.md
@@ -1,74 +1,129 @@
-# fabric: unified-repo view — one illusion of a single repo, over warp + weft
+# fabric: unified-repo view — the single entry-portal that makes warp+weft look like one repo
 
-> **Status: Someday, exploratory.** Raised during a design discussion about `fabric`'s warp/weft split; not committed to, not scoped as a task yet. Several sub-questions below are explicitly open. Does not modify [fabric.md](fabric.md) — that file is deleted at cutover once `internal/fabricengine`'s package doc is the sole remaining source of `fabric`'s rationale; this doc's content belongs in that package doc (or a successor doc) once actually picked up, not in the soon-to-be-deleted file.
+> **Status: Planned — not built.** Promoted from Someday and substantially expanded (2026-07): what began as "extend the illusion through a `Fabric.Commit` and a unified diff" grew, through design discussion, into a fundamental clone/init/topology reshaping of `fabric`. Sequenced **after** the Planned `board` item (which removes `board-url` from clone) and **after** `native clients` (build fabric's git logic against the final go-git `gitrepo` from the start, the same reasoning that sequences `loom` after it); **`Shed` follows this**. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), the durable parts fold into `internal/fabricengine`'s package doc when this lands and this file is deleted.
 
-## The illusion: one normal repo, from the outside
+## The redefined scope: fabric is the one-repo illusion portal, nothing more
 
-Junctions (`_lyx`, `_raddle`, later `_pattern`) exist so that, from the host worktree's perspective, weft-backed folders look like ordinary parts of the same repo — even though they're a second git history underneath. The idea explored here: extend that illusion all the way through `fabric`'s own API surface, not just the filesystem layer. Writing to any file in the worktree, whether it physically lands in warp or in weft, should look from the outside — to a human or an LLM — like it just landed in one repo. `fabric` is the only module that knows both repos exist; it should be the only place the distinction is handled, never something a caller has to track.
+`fabric` operates two git repos (warp and weft) but its whole job is to make it look, from the outside, like there is **one flat repo**. That is the entire scope — and stating it this crisply resolves a long-standing confusion about "what is `lyx init` vs what is `fabric`":
 
-This is a broader, sibling concern to [host-visibility.md](host-visibility.md) (which hides `CONSTRAINTS.md`/`CLAUDE.local.md` specifically from the host's own git history) — same junction-based mechanism, applied here to `fabric`'s API rather than to one specific file pair.
+- **`fabric` owns topology + wiring.** Clone, worktree-add, branch pairing, junction wiring — all of it. There is no separate "init phase" that wires junctions after the fact (see "Clone does everything" below); that concept **dissolves into `fabric`**.
+- **Session bootstrap is `loom`'s, not fabric's.** Seeding `_lyx` *content* when a session starts is a different concern that stays with `loom`. Clean line: **topology + wiring = fabric; session start = loom.**
 
-## `Fabric.Commit` — the idea, and why it may not be needed
+`fabric`'s git-API is **deliberately simplified and is not for humans** — it is for `lyx` and other LoomYard code. We expose only what is needed, never a general-purpose git wrapper. A human always keeps plain `git` in their own warp worktree; fabric's surface is not a replacement for that (see "Warp stays ordinary git" below). This answers the old doc's open "humans or only agents?" question: the API is tooling-facing, warp-raw-git is human-facing.
 
-The idea as originally raised: a single `Fabric.Commit(paths, msg)` entry point classifies each path against known weft junction mountpoints and dispatches to the warp or weft repo accordingly, instead of requiring a caller to pick `Warp.StageAndCommit`/`SyncWeft` explicitly. Classification itself is a trivial path-prefix check against already-known junction mount points — no new low-level mechanism needed there.
+## `Fabric.Commit` — the centerpiece (this reverses the old conclusion)
 
-Whether this is worth building at all turns out to be genuinely open — see "Weft-commit stays Go-orchestrator-only" below, which walks through why no confirmed caller has been identified once the warp-side and weft-side questions are both worked through.
+The original exploration tentatively concluded `Fabric.Commit` had **no confirmed caller** and might be dropped. That conclusion was reached under the premise "warp stays raw git, weft is Go-only, so nobody needs a unified classify-and-dispatch commit." The illusion-first framing **changes that premise and gives `Fabric.Commit` its caller: everything in LYX that writes files.**
 
-## Warp stays ordinary, unrestricted git — for humans and agents alike
+`Fabric.Commit([files], msg, [snapshot-tags])` classifies each path against the known weft junction mountpoints (a trivial path-prefix check) and dispatches to the warp or weft repo accordingly. The value is **not** correspondence — the correspondence link is recorded weft-side regardless (see below). The value is **API uniformity / the illusion itself**: a caller (LLM or Go) never has to know which of the two repos a file physically lives in. That is the point of the design, and it is real.
 
-**Resolved: plain `git add`/`git commit` (and anything else — rebase, amend, force-push) stays the norm for warp, for both humans and agents. `Fabric.Commit` is not a replacement for warp's git usage.** Reason: warp is a normal project repo other people and tools touch outside any LYX-based workflow — a collaborator who has never heard of `fabric` will just run ordinary git commands, including operations `fabric` cannot see coming (rebase, history rewrite). Any design that assumed *every* warp mutation goes through `Fabric.Commit` would break the moment a real collaborator did the obvious thing. This isn't a new requirement to design for — `fabric.md`'s existing "History-rewrite safety" section already commits to exactly this posture: `fabric` relies on `SHAExists` before trusting any stored SHA reference, doesn't try to be "rebase-aware" any more than `gitrepo` does, and (per fabric.md's resolved open question) surfaces a typed staleness error rather than attempting automatic recovery when an external rewrite invalidates a stored reference.
+Two things still hold and must be designed for:
 
-Consequence for this doc: the earlier open question ("should an agent's own warp commits go through `lyx fabric commit` instead of raw `git commit`, to make classification/safety uniform") is **answered — no.** Forcing it wouldn't achieve uniformity anyway, since nothing can force *other* collaborators through the same path; `fabric`'s job is to stay correct in the face of arbitrary external warp git activity (already designed for), not to become the only door in. This also narrows `Fabric.Commit`'s real value: it matters for giving weft a single, safely-gated entry point, not as a general warp-commit replacement.
+- **Not atomic.** A `Fabric.Commit` spanning both warp-side and weft-side paths is still two underlying git commits, not a cross-repo transaction. The illusion is "one repo" for the *interface*, but crash-safety across the two commits needs a defined story (commit weft-first or warp-first, and what happens if the second fails). This is the honest cost of the illusion — an open question below.
+- **The "LLM never decides weft-commit timing" invariant survives — but decouple mechanism from policy.** Its original enforcement was *accidental*: an LLM's `git add <weft-file>` simply failed (git add operates on warp; the weft file isn't tracked there). `Fabric.Commit` makes weft writes clean, so that accidental guardrail is gone. Keep the invariant anyway, as **deliberate policy** — weft-commit timing is orchestration's call (Finalize, raddle-regen at phase boundaries) — and rewrite its justification from "git add would fail" to "orchestration controls weft-commit timing." Cleaner than before: a conscious rule, not a side-effect. (This does not decide that LLMs *should* commit weft — only that the mechanism no longer forbids it by accident.)
 
-## Weft-commit stays Go-orchestrator-only — but does `Fabric.Commit` have a real caller at all?
+## Clone does everything — the `lyx init` phase is gone
 
-`CONSTRAINTS.md`'s "Orchestration, not agent" invariant is **not up for revision** here (explicit decision, not left open): every weft commit is Go calling the engine in-process at a round/phase boundary the orchestrator itself controls; an LLM agent never decides *when* a weft commit happens, regardless of how the commit is dispatched internally. That part stands regardless of what happens below.
+Today setup is two steps: `fabric clone` clones the repos, then a human `cd`s into the right subpath and runs `lyx init` there to wire junctions. The reason init was separate: **`RelPath` (the lyx-anchor subpath) is positional today** — `hubgeometry.Resolve(cwd)` computes `RelPath = filepath.Rel(WorktreeRoot, cwd)`, so you had to physically stand in the subpath for the right anchor to be captured. Clone couldn't know it.
 
-**Open, and honestly unresolved by this doc's own reasoning: does anything actually call `Fabric.Commit`?** Walking the other conclusions here through to the end: warp-side commits stay on plain git for both humans and agents (see above) — no caller there. Weft-side commits are performed exclusively by Go orchestration code (Finalize, Raddle-regen) that already knows explicitly what it's writing, because it's the code that authored the content in the first place — auto-classification doesn't save that caller anything either, the same counter-argument this idea faced early on (a caller already has to know where a file belongs to decide where to *write* it, so guessing again at commit time buys nothing). **No confirmed caller has been identified.** Whoever picks this up should find a real one before building it, or drop `Fabric.Commit` from scope entirely and keep only the diff/status and enforcement work below.
+The fix: **store the subpath↔weft binding in the `weft` repo itself.** One weft repo is 1:1 with one host repo and one anchor subpath (multi-subpath is explicitly out of scope — see below), so that binding is intrinsic to weft. `lyx fabric clone` then does the whole job in one shot — prime worktree, prime-weft worktree, and all junction wiring at the right subpath — with **no init step**. It runs in create-or-adopt mode, exactly the pattern `suffixWeftPrimaryBranch` already uses for the primary weft branch:
 
-**If a caller does turn up and this gets built, two things still hold:**
+- **Weft remote already carries the binding** (a re-clone on a new machine): read the subpath from weft, wire accordingly. Any `--subpath` flag is ignored (or validated against the record and errors on mismatch, so you never accidentally re-anchor).
+- **Weft remote has no binding** (the genuine first-ever setup — first clone *is* create): this is where the human supplies the anchor. Because the prime worktree does not exist yet at clone time, **cwd cannot specify a subpath inside it** — you are standing where the *hub* will be created, not inside a worktree. So the subpath is an **explicit flag**, `--subpath <rel>` (default `"."` = root, the common case), recorded into the new weft config as the permanent source of truth. `--weft-url` is the one genuinely underivable input (where to push the new weft repo). Post-`board`, no `board-url` is needed (board lives in `weft:main`).
 
-- **Not atomic either way** — a call spanning both warp-side and weft-side paths in one invocation is still two separate underlying git commits; there is no cross-repo transaction. Partial-failure semantics (report both results vs. attempt to roll back whichever side succeeded) remain open.
-- **Weft paths must hard-block** (explicit error, not a silent no-op) for any caller that isn't the sanctioned orchestrator context — consistent with the invariant already being an actively *audited* hard rule elsewhere (`internal/websterengine/audit.go` flags a weft-referencing Bash command as a review violation today). An explicit refusal matches that "hard rule, not soft suggestion" posture; since a legitimate caller should never hit this path, an error is the correct signal, not a mystery no-op. **Open question:** the exact mechanism for identifying "the sanctioned orchestrator context" — a caller-identity check, or a separate, more privileged API surface that only orchestration code imports and that never appears on any agent-facing path.
-- Restricting an agent-facing surface to only `Fabric.Commit` (never raw `.Warp`/`.Weft` field access, which `fabric.md`'s existing API exposes directly today) would **not** be a reprise of `fabric.md`'s already-rejected "forwarding method per operation" alternative — that alternative was rejected for duplicating the *entire* `gitrepo.Repo` method set (`StageAndCommit`, `ChangedFilesSince`, `CurrentSHA`, `Push`, `Pull`, `SHAExists`, ...) as pass-through methods. This would be *one* method for the one operation needing a safety boundary, not a wholesale duplication. (Checked the shipped CLI: today's `lyx fabric commit` verb is already weft-only — `internal/fabriccli/weft_verbs.go` — there is no existing warp-commit CLI verb, so this would fill a genuine gap rather than duplicate one, *if* a caller exists.) Internal Go orchestration code keeps using `.Warp`/`.Weft` directly regardless — it already knows the distinction.
+Because this is a once-ever, source-of-truth decision, create-mode **echoes and confirms** the resolved anchor before writing:
 
-## `fabric` needs its own unified diff/status — a different, simpler case than Finalize's merge-diff
+```
+Anchoring lyx at "<RelPath>"  (relative to repo root <WorktreeRoot>)
+Weft repo → <url>
+Junctions to wire: _lyx _raddle _pattern …   (from template)
+Proceed? [y/N]
+```
 
-Two distinct kinds of "diff spanning warp and weft" come up in this project, and only one of them is genuinely special:
+The one truly bad failure here — cd'd/flagged the wrong anchor, now locked forever — is caught by that echo, without forcing extra ceremony on the common root case.
 
-- **"What changed in my own worktree since some earlier point"** — the case this section is actually about. **Not special.** An ordinary per-repo diff (`gitrepo.Repo.ChangedFilesSince`), using the correspondence index only to resolve the right starting weft SHA from a warp-SHA reference point, merged into one report. The real gap is narrower than it first looks: raw `git diff`/`git status`, run in the host worktree, is still blind to weft's separate history, so nobody — human or LLM — should need to reach for raw git to get the full local picture. Filling that gap doesn't need anything beyond primitives that already exist.
-- **"How my weft content compares to a *different* weft (parent's, at merge-back)"** — genuinely special, and already solved, for a different purpose: [finalize.md](finalize.md)'s document-driven, non-git-conflict mechanism (Go precomputes the diff directly against the real weft worktree via the correspondence index, hands the agent a plain document, never git conflict markers). This doc doesn't need to (and shouldn't) reinvent that — it already exists for the case that actually needs it. Don't conflate the two.
+**Multi-subpath is not supported.** One prime worktree, one anchor, one weft. Two independent lyx roots in one host repo would require two weft repos and two separate clones — low probability, deliberately out of scope. This is what keeps the binding a clean 1:1 and "store the subpath in weft" unambiguous.
 
-`Topology.Status`/`Fabric.StatusWeft` already exist today, but separately (one per side), and only cover the first case. This proposes an actual **merged** view — a `Fabric.Diff` (and/or an extended `Status`) presenting warp and weft changes together as one report, still only for the "since an earlier point in my own worktree" case above.
+## Consequence: `RelPath` moves from positional to recorded
 
-**Open question:** whether this becomes a new CLI verb (`lyx fabric diff`) or stays a Go-internal API not exposed as a standalone command — depends on who ends up needing it (a human debugging, an LLM instructed to inspect its own worktree state, or only internal callers like Finalize).
+This is the real work of the clone change. Today `Resolve(cwd)` trusts cwd. Once the subpath is a recorded binding in weft, **runtime `Resolve` must consult that binding** (or a marker at the anchor), not blindly re-derive from cwd — otherwise a command run from the wrong subdir (e.g. repo root, above the anchor) resolves `RelPath = "."` ≠ the recorded anchor and the geometry is wrong. The recorded binding becomes source of truth; cwd-derivation is demoted from "the resolver" to, at most, a consistency check (it happens to agree when you stand at or below the anchor: `Rel(prime, prime/X) = X`). Reconciling the two — record wins, cwd validates — is the concrete change this item must make in `hubgeometry`. Subpath support itself is already comprehensive: `RelPath` is threaded through every junction/portal/launcher path and every relative-link climb, handling both `"."` and arbitrary segments — it is not vestigial.
 
-## SHA-bookkeeping — reuse, not a new mechanism
+## Config-driven junction list — fabric stops enumerating modules
 
-Confirmed feasible without inventing anything: `gitrepo.Repo.SnapshotSHA`/`SetSnapshotSHA` (persisted per-key SHA tracking) and `fabricengine`'s Warp-SHA correspondence index (`RecordCorrespondence`/`WeftSHAForWarpSHA`/`RebuildIndex`) already do exactly this class of bookkeeping today, built for raddle staleness tracking and the weft-sync trailer respectively. A unified status/diff view would generalize reuse of this existing infrastructure, not add a new primitive underneath it.
+Today the wired-junction set is hardcoded (`hubgeometry.IsReservedHubName`: `_lyx`, `_raddle`, `_board`, `_portals`, `_launchers`) — adding `_pattern` or `_codeintel` means a code change. Instead, the set of weft-backed folder names lives in a **config file with a template** that any new weft-backed module appends its junction name to. `fabric` never has to know about every module that might want a folder.
 
-**What triggers a snapshot?** Nothing new needs inventing here either. `fabric` already leaves cadence to the caller (see `fabric.md`'s resolved "Push timing policy" open question: `fabric` stays unopinionated about cadence, a caller-level policy decision). A unified diff doesn't need `fabric` to autonomously decide "since when" — the caller captures its own reference point (e.g. `CurrentSHA()` at phase-start) and later passes it to `ChangedFilesSince`/`Diff`; this is a pure function of caller-supplied input, not an event `fabric` needs to trigger on its own. The one piece that *is* already auto-triggered today is the correspondence index (`RecordCorrespondence` fires alongside every real `CommitWeft`) — unchanged by anything proposed in this doc.
+This lives naturally in the **same weft config as the subpath binding** — both are per-repo setup facts, and `lyx fabric clone` reads both from one place. It grows out of today's `fabric.yaml` (`pathspec` key already there).
 
-## How far does "route git through Fabric" go? — tension with an already-deliberate scope boundary
+Ownership boundary to set deliberately (CONSTRAINTS.md's **Hub Geometry Invariant** says `hubgeometry` owns all geometry/paths): keep `hubgeometry` the owner of the *paths* (it computes `<Hub>/<slug>/<RelPath>/<name>`), but inject the *name set* from fabric config. The invariant holds (geometry owns paths) while fabric stops hardcoding modules.
 
-A broader version of this idea was raised and is **not settled**: route *every* git operation through `Fabric`, unconditionally, for full control over warp/weft correctness. This runs directly into `fabric.md`'s own, already-deliberate "Scope boundaries — deliberately not a general-purpose git wrapper" section: `gitrepo`'s scope already excludes rebase, interactive staging, cherry-pick, and conflict resolution, and explicitly preserves "a human always has plain git available in either working tree." That was a conscious choice, not an oversight — and the "warp stays ordinary git" resolution above is a direct instance of it holding.
+## Eager wiring at worktree-add
 
-Two questions need answering before this goes anywhere, not just one:
+Every `lyx fabric worktree add` wires all junctions immediately, under the hood — no dormant `_lyx` waiting for a later activation. `WireJunctions` already lives in `fabric` and wires the whole configured set in one pass; folding it into `worktree add` (after the weft worktree exists, so the junction targets resolve and the host-pristine guard is satisfied) is what makes fabric the sole, end-to-end wirer. The old "junction wired ⇔ active session" distinction is dropped as finer than it is worth; an empty `_lyx` on an idle worktree harms nothing.
 
-- **Humans, or only agents?** Preventing a human from running plain `git` in their own worktree is neither technically enforceable (it's their own shell) nor obviously desirable, given warp is meant to be "an ordinary project repo" — reinforced by the collaborator argument above.
-- **Does "full control over correspondence" actually require *all* git operations, or only the mutating ones?** The correspondence index only needs to know about commits/pushes/pulls/merges — operations that advance history. Read-only operations (status, diff, log) don't affect correctness at all; routing them through `Fabric` buys a consistent *view* (the unified-diff point above), not control. Wrapping every git verb (rebase, blame, stash, branch, ...) reprises the "forwarding method per operation" pattern `fabric.md` already rejected — at a much larger scale than the single `Commit` method proposed above.
+## Snapshot-tracking folds into the `Warp-SHA` trailer mechanism
 
-## Open questions (unresolved, for whoever picks this up)
+Today there are **two** SHA-bookkeeping mechanisms: `gitrepo`'s `refs/loomyard/snapshot/<key>` refs (`SetSnapshotSHA`/`SnapshotSHA`) and `fabricengine`'s `Warp-SHA` trailers + correspondence index. Unify them: snapshot-recording becomes an **optional trailer on the weft commit** — `Fabric.Commit([files], msg, snapshotTags=["raddle"])` writes a `Snapshot: raddle` trailer alongside the `Warp-SHA` trailer, and a snapshot's baseline is derived from the latest weft commit carrying that tag (its `Warp-SHA` trailer *is* the warp SHA the snapshot describes — exactly what raddle needs). Same architecture the correspondence index already uses: trailer is truth, an index on top is a rebuildable cache.
 
-- **Whether `Fabric.Commit` has any real caller at all** — walked through above and came up empty; find one or drop it from scope before building anything.
-- Partial-failure semantics for a `Fabric.Commit` call spanning both warp and weft paths in one invocation (only matters if a caller is found).
-- The exact mechanism for restricting weft-side dispatch to the sanctioned orchestrator context.
-- Whether `Fabric.Diff` becomes a CLI verb or stays Go-internal only.
-- Whether "route git through Fabric" should extend beyond commits at all, and if so, to agents only or to humans too — see the section above; leans toward "commits/pushes only, agents only" but this isn't decided.
+Fold it into `Commit` rather than a standalone `Fabric.Snapshot("tag")`: a snapshot is meaningless except in relation to a specific committed state, so coupling it to the commit that produced that state is more correct (and matches raddle's "advance only on confirmed success"). A standalone no-commit snapshot call is only warranted if a consumer must record a baseline without producing weft content — which raddle/codeintel (both commit their output) never do; leave it out until a real caller appears. This retires the separate `refs/loomyard/snapshot/` mechanism.
+
+## Warp-rebase and remote-reconcile — the hardest part, but bounded
+
+`fabric` must handle warp history that moves underneath it: a non-LYX collaborator (or the same operator on another machine) pushes to warp remote, and later a LYX+fabric session on this machine must **sync those commits down** — resolving conflicts (the intent is to spawn an LLM), including the extreme case where the remote was **rebased**.
+
+The naïve fear is "replay all of weft onto the rebased warp." That fear shrinks once decomposed, because most of weft is a **pure function of the code that regenerates**, not something merged:
+
+- **Detection is already honest and shipped.** After a warp rewrite, weft's `Warp-SHA` trailers point at warp SHAs that no longer exist; `SHAExists` catches this rather than trusting a dead reference (the "staleness survives rebuild" tests already guard it).
+- **raddle / codeintel self-heal.** Both regenerate at merge-time (raddle.md) — pure functions of code. Post-rebase: stale → regenerate → new weft commit with a fresh `Warp-SHA` trailer → correspondence re-established.
+- **`_lyx` never propagates to parent** (finalize.md) — no re-alignment needed against a rebased parent.
+- **The residue is small:** genuinely hand/LLM-authored weft content (`PATTERN`). Rare, small. This is the only thing that needs real re-alignment.
+
+So rebase-recovery = re-anchor the correspondence (the `RevertWithWeft` "nearest-older" logic is the building block) + regenerate derived content + a small hand/LLM re-alignment for `PATTERN`. The layering keeps the shipped invariant intact: **`fabric` core detects and precomputes the diff; an orchestrator above it spawns the LLM** for genuine content conflicts, using finalize.md's document-driven mechanism (Go hands the agent a plain document, never git conflict markers across a junction). "Rebase is part of fabric" means the mechanics + detection are fabric's; the LLM resolution sits just above, in orchestration — never an LLM deciding weft-commit timing.
+
+## Warp stays ordinary git — preserved, and it is why all this is feasible
+
+Plain `git add`/`git commit` (and rebase, amend, force-push) stays the norm for warp, for humans and agents alike. `Fabric.Commit` is not a mandatory door on warp — nothing can force *other* collaborators through it, so fabric's job is to stay correct under arbitrary external warp git activity, not to be the only entrance. This is exactly what makes the whole illusion feasible: **the correspondence link is one-directional (weft records `Warp-SHA` pointing at warp), recorded at weft-commit time from warp's current HEAD.** Warp therefore never needs to route through fabric for correspondence to work — a raw `git commit` in warp is fine, and the next weft commit picks up warp's new HEAD in its trailer. Only weft holds the linking info; warp behaves as if lyx never piggybacked it. A post-checkout hook already fires drift warnings so an out-of-band warp branch (no weft yet) is *detected*, not silently mishandled.
+
+Consequence: a warp-only `Fabric.Commit` is legitimate (it completes the illusion — the two-repo split stays invisible even for warp-only writes) even though it buys nothing for correspondence. That is fine; the uniformity *is* the reason.
+
+## Scope boundary — still not a general-purpose git wrapper
+
+`gitrepo`'s scope deliberately excludes rebase, interactive staging, cherry-pick, conflict resolution, and preserves "a human always has plain git in either working tree." That conscious boundary stands. Routing *every* git verb through `Fabric` (blame, stash, branch, log …) reprises the already-rejected "forwarding method per operation" pattern at large scale and is not the goal. What fabric wraps is the small set that affects the two-repo illusion and correspondence: commit/push/pull/sync, clone/worktree/branch topology, and the unified diff/status. Read-only verbs the caller can run directly; where a *unified view across both repos* is wanted (a single `Fabric.Diff`/`Status` for "what changed in my worktree since a point"), that is an ordinary per-repo diff merged via the correspondence index, not a new primitive.
+
+## Dependencies and sequencing
+
+- **After `board: move storage to weft:main`.** board-weft-storage removes `board-url` and the board-clone step from `CloneHub` (board moves into `weft:main`), so this item inherits a 2-repo clone (host+weft) to restructure, not 3-repo. It also introduces prime's second weft checkout (`weft:main` for board, alongside `weft:main-weft`) and the "everything lyx-related lives in weft" pattern the subpath/junction-config binding slots into. The two share the weft-branch adopt-or-create primitive (`suffixWeftPrimaryBranch`), which board hardens for the `weft:main` case first.
+- **After `native clients`.** Build fabric's clone/commit/snapshot git logic against the final go-git-based `gitrepo`, so it isn't re-validated if the CLI→library swap surfaces any subtle behavioral difference — the same reasoning that sequences `loom` after `native clients`.
+- **`Shed` follows this.**
+
+## Build order — slices, not one task, and extend-in-place
+
+`fabric` V2 is not a from-scratch rewrite and not a parallel `FabricV2` package. The Warp+Weft→V1 merger justified a parallel reference because it was a genuine architectural *union of two modules*; V2 is ~six changes layered onto `fabricengine`, whose core (two `gitrepo` instances, weft pairing, `Warp-SHA` trailer correspondence, weft-git plumbing, junction primitives, branch scheme) is reused wholesale. A parallel package would be massive duplication for no gain. **Extend `fabricengine` in place, one landable slice at a time**, with git history + green tests between slices as the reference — not a second copy of the module.
+
+Suggested slice order (none individually "enormous" — the size was always the sum):
+
+1. **Config-driven junction list** — replace the hardcoded reserved-name set (`hubgeometry.IsReservedHubName`) with a config-read list + a template new weft-backed modules append to. Small, near-mechanical, independent. `hubgeometry` keeps owning the paths; only the name set comes from config. Note the handoff with the Planned `PATTERN.md` item: `pattern-wiring` ships against today's *hardcoded* list (it adds `_pattern` there, which is correct — it is not blocked on this slice), so by the time this slice runs `_pattern` is already a hardcoded entry. This slice is therefore where PATTERN's (and `_lyx`/`_raddle`/`_board`'s) hardcoded junction wiring is migrated into config — it cleans up the existing entries, not just hypothetical future modules.
+2. **`Fabric.Commit` (classify+dispatch) + unified `Fabric.Diff`/`Status`** — pure API additions over the existing `Warp`/`Weft` handles, `CommitWeft`, and `ChangedFilesSince`. Independent; the atomicity / partial-failure story lands here.
+3. **Snapshot-as-trailer** — fold `refs/loomyard/snapshot/` into the `Warp-SHA` trailer mechanism; coordinate with `native clients` (which ports the ref-based snapshot to go-git first — this slice supersedes that, a minor, harmless overlap).
+4. **Clone-does-everything + subpath-in-weft + `init` dissolution** — the structural heart, after `board`. This is where **`lyx init` dissolves.** `initengine.Init` today does six things: cwd→`RelPath` anchor resolution, weft-pairing check, `WireJunctions`, `_lyx`/`_lyx/config` creation, the `.lyx/` `.gitignore` block, and `configsync.ReconcileAll`. Under V2 the topology parts (wiring, `.gitignore`, `_lyx` dirs) fold into `fabric`'s clone/worktree-add (eager wiring); the cwd-anchor mechanism is *replaced* by the weft-recorded subpath; config reconciliation stays in `configsync`/`configcli` (called once by clone, or a separate post-clone `lyx config` step). Net: `initengine`/`initcli` shrink toward deletion, with `--undo`'s teardown moving onto `fabric`'s existing `UnwireJunctions` + config revert. Optionally split 4a (subpath binding + `RelPath` positional→recorded) from 4b (fold wiring into clone/add). Develop the risky new clone path as a **coexisting entry point inside `fabricengine`** — the old clone stays until the new one is proven, then swap and delete — a local safety valve, not a module-wide fork.
+5. **Warp-rebase / remote-reconcile** — last. Detection (`SHAExists`) + correspondence re-anchor (`RevertWithWeft` nearest-older) + `PATTERN` document-driven resolution land with fabric; the full self-heal leans on raddle regeneration (Someday), so reconcile's complete form trails there. The LLM conflict-resolver sits in orchestration above fabric (finalize.md's document-driven path), never deciding weft-commit timing.
+
+The roadmap item is therefore a small campaign — 4–5 board tasks when picked up — not one atomic task. Slices 1–3 don't touch clone and could technically precede `board`, but keeping them after `board` avoids reopening the sequence set above.
+
+## Open questions (for whoever builds this)
+
+- **Partial-failure semantics for a two-sided `Fabric.Commit`** — commit weft-first or warp-first, and the recovery/report story when the second commit fails (no cross-repo transaction exists).
+- **The RelPath record-vs-cwd reconciliation mechanism** — a marker file at the anchor, a value read from weft config, or a climb — and how `Resolve` consults it while keeping cwd as a consistency check.
+- **Home of the junction-name config** — `fabric.yaml` read by `hubgeometry`, vs `hubgeometry` owning it and pulling values from config; keep geometry the owner of paths either way.
+- **Weft remote provisioning at first clone** — the first-ever setup needs a weft remote to push to (empty is fine); either require it pre-created (the GitHub-wiki wrinkle) or have clone provision it.
+- **Exact rebase/remote-reconcile orchestration** — which layer drives pull → conflict-resolve → raddle-regen, and how `PATTERN` re-alignment is presented to the resolving agent (reusing finalize.md's document-driven path).
+- **Whether `Fabric.Diff` is a CLI verb or Go-internal only** — depends on who needs it (a human debugging, an instructed agent, or only internal callers).
 
 ## Related
 
-- [fabric.md](fabric.md) — the base design this generalizes over. Not itself edited by this doc; deleted at cutover once `internal/fabricengine`'s package doc absorbs the rationale.
-- [finalize.md](finalize.md) — the related but distinct, genuinely special cross-worktree diff problem (my weft vs. a different weft), already solved there for the merge-conflict path.
-- [host-visibility.md](host-visibility.md) — a related, narrower illusion (hiding `CONSTRAINTS.md`/`CLAUDE.local.md` from host git history) via the same junction mechanism.
-- `CONSTRAINTS.md`'s "Orchestration, not agent" section — the invariant this design must not violate, only enforce more consistently.
+- [board-weft-storage.md](board-weft-storage.md) — the Planned item this sequences after; removes `board-url` from clone, establishes weft-as-home and prime's two-weft-checkout shape.
+- [native-clients-migration.md](native-clients-migration.md) — the `gitrepo` go-git migration this builds its git logic on top of.
+- [finalize.md](finalize.md) — the document-driven, non-git-marker weft-conflict mechanism the rebase/reconcile path reuses; also the weft-side merge-back this shares primitives with.
+- [raddle.md](raddle.md) — the regenerate-don't-merge property that bounds rebase recovery; the snapshot-staleness consumer the trailer-fold serves.
+- [host-visibility.md](host-visibility.md) — the narrower sibling illusion (`CLAUDE.local.md`), same junction mechanism.
+- [pattern.md](pattern.md) — the hand-authored weft content that is the real residue of rebase re-alignment; also a `_pattern` junction consumer of the config-driven list.
+- `internal/fabricengine` (doc.go) — the shipped base this generalizes; the durable parts fold here on landing. `CONSTRAINTS.md`'s "Orchestration, not agent" section is the invariant this enforces more consistently, never violates.
diff --git a/manifest/designs/pattern.md b/manifest/designs/pattern.md
new file mode 100644
index 00000000..eea7913f
--- /dev/null
+++ b/manifest/designs/pattern.md
@@ -0,0 +1,90 @@
+# PATTERN — loomyard's own invariants doc, wired into every agent
+
+> **Status: Design — not built. Planned.** Two clearly separated pieces with different timing: (1) the **wiring** — how an active PATTERN doc reaches every code-touching agent — is buildable **now** (it depends only on already-shipped `fabric` junctions and `internal/stencil`, not on `loom`); (2) the **content migration** — moving loomyard's own invariants out of the mill-owned `CONSTRAINTS.md` into PATTERN — happens **only when loomyard is initialized via lyx itself** (dogfooding), never in this repo now. Per the [documentation lifecycle](../../docs/overview.md#documentation-lifecycle), durable parts fold into the owning package doc when this lands and this file is deleted.
+
+## What PATTERN is (and is not)
+
+PATTERN is **loomyard's own invariants mechanism** — the equivalent of Millhouse's `CONSTRAINTS.md`, but owned by loomyard, from scratch, not a port.
+
+Loomyard does **not** have one today. The `CONSTRAINTS.md` at this repo's root is **Millhouse's** artifact — it exists here only because Millhouse is the tool currently developing loomyard, and mill tooling + `CLAUDE.md` read it every session. It is not loomyard's own mechanism; PATTERN is the missing piece.
+
+Why loomyard needs its own: **lyx initialized in any repo must be able to carry føringer (guidance/invariants) to the agents it spawns there.** PATTERN is that carrier. It matters for two cases:
+
+- **lyx-in-a-client-repo** — a consulting host repo where lyx drives development; the invariants for that repo travel with it, invisibly to the host's own git history (weft-resident — see below).
+- **loomyard-via-loomyard (dogfooding)** — when loomyard develops itself onto `loom` instead of onto mill, `CONSTRAINTS.md` becomes worthless (mill is no longer the developer), and loomyard's invariants must by then live in PATTERN.
+
+## Shape: a weft-backed `_pattern/` folder, not a single file
+
+PATTERN is a **directory**, reached from the warp worktree through a `_pattern` junction into `weft` — already anticipated in [fabric-unified-view.md](fabric-unified-view.md) and [finalize.md](finalize.md). It is `_lyx`'s first sibling junction, not a third peer alongside an already-junctioned `_raddle`: `_raddle` carries no junction of its own today, so `_pattern` is the *second* junction `hubgeometry` declares, not one of three. The directory holds:
+
+- **`_pattern/PATTERN.md`** — the index: short two-line entries, one per invariant (the constraint stated in a line, plus a pointer to its detail doc). Never long-form prose inline.
+- **`_pattern/<topic>/…`** — a detail submap: one per-topic doc per invariant carrying the full rule / rationale / enforcement. This is the same short-index-plus-linked-detail structure already proven for raddle's `Overview.md` → module docs, and named as the shared pattern in [board-weft-storage.md](board-weft-storage.md).
+
+All PATTERN content lives in `weft`, so it is invisible to the host repo's own git history by construction (this is what supersedes the `CONSTRAINTS.md`-equivalent half of the `host-visibility` item — see [host-visibility.md](host-visibility.md)).
+
+## The wiring: how constraints reach every agent
+
+The problem the wiring solves: an agent lyx spawns (implementer, reviewer, planner, a webster fork, a burler round) must be told to follow the active invariants — the same way `CLAUDE.md` tells a session "Read `CONSTRAINTS.md` before writing or reviewing any code" today.
+
+The mechanism, reusing what already exists:
+
+- **A conditional `stencil` marker** — `{{.pattern_directive}}` — placed in every **code-touching** template (implementer, reviewer, planner, webster fork, burler round). Not in discussion / judge templates, which do not write or review code. (Note: the `<NN>`-style angle-bracket placeholders already in templates are literal text, not stencil syntax; stencil substitutes `{{.X}}` markers.)
+- **Go computes the marker value at prompt-assembly time.** A cheap active-check — does `_pattern/PATTERN.md` exist? — decides:
+  - **active** → the value is a short directive, e.g. *"Before writing or reviewing any code, read `_pattern/PATTERN.md` and follow every constraint listed there."*
+  - **inactive** → empty.
+- **A pointer is injected, never the constraints inline.** The directive names the file; the agent reads it itself. This keeps every prompt lean regardless of how large PATTERN grows, and mirrors today's `CLAUDE.md` → `CONSTRAINTS.md` discipline exactly.
+
+A single shared helper in the prompt-assembly path returns the directive-or-empty, called wherever `stencil.Fill` runs on a code-touching template — so the active-check lives in one place, not copied per engine.
+
+## Consequence for `stencil`: an optional (allowed-empty) marker
+
+`stencil.Fill` carries one load-bearing guarantee: **every top-level `{{.X}}` marker must resolve to a non-empty value** — it treats an absent or whitespace-only value as an unfilled marker and fails (`unfilledTopLevelMarkers` in `internal/stencil/stencil.go`). A conditional PATTERN directive that is **empty when inactive** violates that invariant head-on.
+
+So the wiring requires a small, real `stencil` extension: a notion of an **optional marker** — one explicitly allowed to be empty, exempt from the non-empty guarantee. PATTERN is **not** the first conditional token in the system: `websterengine`'s `rename_mechanic` predates it, sitting inside a `{{if .rename_mechanic}}` block in `fork-template.md`, in production before this wiring landed. The optional-marker extension is a deliberate design choice, not a forced one — it puts optionality in Go, where it is testable per call site, and it keeps the "no conditionals in templates" banner rule uniform across every other prompt template, rather than existing because `{{if}}` could not have worked. (The alternative — giving the inactive marker a benign non-empty value like a lone space or a hidden comment — is a hack that pollutes the rendered prompt; prefer the explicit optional-marker concept.)
+
+## Junction wiring and activation
+
+The split is narrower than "`fabric`'s responsibility": `internal/hubgeometry` declares the `_pattern` junction record and owns every `_pattern` path literal (the Hub Geometry Invariant); `internal/fabricengine`'s `seedLyxJunction` (called from `WireJunctions`) materialises the weft-side target and creates the junction itself; `internal/initengine`'s `Init` is the caller that actually wires it for a fresh worktree. `fabric add` explicitly does **not** wire the host junction — its own code states the junction is wired by `lyx init` via `WireJunctions`, not by `add` (junction creation is core, already-shipped `fabricengine` — not the Someday `fabric-unified-view` work, which only *mentions* `_pattern`).
+
+Activation is by **file existence, not junction presence**:
+
+- `initengine.Init`, via `fabricengine.WireJunctions` → `seedLyxJunction`, materialises the `_pattern/` directory in `weft` (possibly empty) and the junction on every fresh `lyx init` — simplest, and a junction needs its target to exist to resolve. No other `WireJunctions` caller (`fabricengine/checkout.go`, `fabricengine/reconcile.go`) materialises a weft directory itself; they only repair or verify an existing junction.
+- PATTERN is **active** iff `_pattern/PATTERN.md` is present. A repo with no invariants yet (no `PATTERN.md`) simply has an empty `_pattern/`, the Go active-check returns false, and the directive marker renders empty everywhere. No special "PATTERN not configured" branch anywhere — the file's presence is the whole switch.
+- Activation also requires `_pattern` to be listed in `fabric`'s own weft pathspec, or PATTERN content written under `_pattern/` never leaves the machine (weft never commits it). A worktree initialised before this pathspec widened keeps its narrower pathspec and must be widened by hand — the wiring does not retroactively repair an already-initialised worktree's weft commit scope.
+
+## Scope boundary — wiring now, content migration only at init
+
+**In scope now (buildable, `loom`-independent):**
+
+- the `{{.pattern_directive}}` marker in the code-touching templates,
+- the `stencil` optional-marker extension,
+- the Go active-check + shared directive helper,
+- `hubgeometry` declaring the `_pattern` junction record, `fabricengine`'s `seedLyxJunction` materialising the weft dir + junction, and `initengine.Init` calling it.
+
+**Explicitly NOT now — deferred to loomyard-init-via-lyx (the dogfooding transition):**
+
+- moving loomyard's own invariants out of `CONSTRAINTS.md` into `_pattern/PATTERN.md` + detail docs, and retiring `CONSTRAINTS.md`. While mill still develops loomyard, `CONSTRAINTS.md` stays the single live invariants doc (`CLAUDE.md` and mill tooling read it). Authoring PATTERN content in this repo now would just create a redundant second doc under dual maintenance for no benefit until the cutover.
+
+The wiring built now is inert in this repo until an actual `_pattern/PATTERN.md` exists — which is correct: it lets the mechanism ship and be tested (against a fixture `PATTERN.md`) long before any real content migration.
+
+## Open questions
+
+Four of the five questions this section originally posed are settled by the wiring task; one is not, and stays open rather than being silently dropped:
+
+- **Exact template set — settled.** Five templates carry the marker: builder's implementer prompt, burler's round prompt, webster's fork prompt, webster's Master prompt, and loom's plan prompt — the last two admitted by explicit clauses (a context-inheritance root whose in-session forks write code; the author of typed file-op instructions a later code-writing agent executes near-verbatim), not by the naive "reviewer/planner" guess this section originally listed. Discussion is excluded: it emits a decision record the Plan producer re-derives from, so a constraint miss there still has a gate after it.
+- **Home of the active-check helper — settled.** `internal/pattern`, a new leaf package — not `stencil` (which stays generic, no PATTERN-specific knowledge) and not `fabric` (which owns the junction, not the prompt-assembly-time check).
+- **Directive wording — settled.** Three role variants, as literal Go constants (`RoleImplementer`, `RoleReviewFix`, `RoleOrchestrator`), each an imperative checklist under its own `##` heading rather than a single sentence — the role varies the wording precisely because a reviewer's job (judge, then fix) differs from an implementer's (write) and from an orchestrator's (fork only, never edit).
+- **Optional-marker surface in `stencil` — settled.** An explicit allow-list passed as `FillOptional`'s third parameter (`optional []string`), not a naming convention: the caller declares which markers are optional for that specific fill, so the same template can be filled once with a marker required and once with it optional depending on the call site.
+- **Detail-submap layout — still open.** Whether `_pattern/<topic>/` has a fixed structure or is free-form per invariant. This is a question about PATTERN's own *content*, not its wiring, and belongs to the content migration this task explicitly defers to loomyard-init-via-lyx (see Scope boundary above) — nothing in the wiring batches settles it.
+
+## Related
+
+- `manifest/roadmap.md` — the Planned `PATTERN.md` item this doc details.
+- [board-weft-storage.md](board-weft-storage.md) — establishes that `PATTERN.md` (and all non-warp content) lives in `weft`; names the short-index-plus-linked-detail structure PATTERN reuses.
+- [host-visibility.md](host-visibility.md) — its `CONSTRAINTS.md`-equivalent half is superseded by PATTERN-in-weft.
+- [finalize.md](finalize.md) — merge-back forwards `_pattern` (like `_raddle`) via a narrowed weft pathspec; PATTERN content is genuinely hand/LLM-authored, so it is the weft-side document-driven conflict path's main real case.
+- [fabric-unified-view.md](fabric-unified-view.md) — where `_pattern` is listed among the weft junctions; note this is a *Someday* API-unification item, **not** a dependency — PATTERN needs only base `fabric` junction creation.
+- `internal/stencil` — the template-fill leaf the `{{.pattern_directive}}` marker rides on; the optional-marker extension lands here.
+- `internal/fabricengine` — materialises the `_pattern` junction (via `seedLyxJunction`), alongside `_lyx`; `_raddle` carries no junction today.
+- [raddle.md](raddle.md) — `_pattern`'s neighbor in `weft`; its `Overview.md` → module-docs shape is PATTERN's index-plus-detail precedent.
+- Root `CONSTRAINTS.md` + `CLAUDE.md` — the Millhouse mechanism PATTERN mirrors and (only at dogfooding) replaces.
diff --git a/manifest/roadmap.md b/manifest/roadmap.md
index f455d43a..d52ebd77 100644
--- a/manifest/roadmap.md
+++ b/manifest/roadmap.md
@@ -8,11 +8,14 @@ Committed to, in this order, next.
 
 1. **board: move storage to `weft:main`** — replaces board's own separate remote repo with a reserved `weft:main` branch (README.md rendering, JSON-backed Proposals/Manifest/Tasks/Done). Depends on `fabric`'s branch-naming enforcement (`<slug>-weft` uniformly), which is now live (`fabric` shipped Done below, old warp/weft modules deleted). See [designs/board-weft-storage.md](designs/board-weft-storage.md).
 
+1. **fabric: unified-repo view — the single entry-portal that makes warp+weft look like one repo** — a major expansion of the former Someday item (promoted): `fabric` becomes the sole, deliberately-simplified git portal for everything LoomYard does, giving callers the illusion of one flat repo over the two underlying histories. Folds the separate `lyx init` phase into `lyx fabric clone` (create-or-adopt: the first clone records the lyx-anchor subpath into `weft` and wires all junctions; every later clone reads it — no cd-and-init step), makes the junction set config-driven (a template new weft-backed modules append to, replacing today's hardcoded list), unifies snapshot-tracking into the `Warp-SHA` trailer mechanism (`Fabric.Commit([files], msg, [snapshot-tags])` as the centerpiece), and takes on warp-rebase / remote-reconcile recovery (fabric detects + precomputes; an orchestrator spawns the LLM for genuine conflicts — the hardest part, but bounded because most of `weft` regenerates). Keeps warp ordinary git for humans and the "an LLM never decides weft-commit timing" invariant intact — now a deliberate policy rather than an accident of `git add`. Depends on the Planned `board` item (which removes `board-url` from clone); see `fabric` in Done below for its current status. See [designs/fabric-unified-view.md](designs/fabric-unified-view.md).
 1. **Shed: shared outer phase-FSM, combined with the Finalize step** — generalizes the phase-sequencing engine `loom.md` already specifies (sequencing, resume, crash-recovery, pause, status-file contract) into a shared skeleton with two swappable slots (Preflight, producer), reused by the Someday `Hardener` module, **built together with Finalize** (see [designs/finalize.md](designs/finalize.md) — merge-back, incl. the warp/weft split and the Raddle-only-forward pathspec) since Finalize is Shed's own literally-shared code, not a per-instance slot — one task, not two, same reasoning as the combined `Treadle`+`perch` item. **Testable cheaply:** plug a quick, throwaway producer into the producer-slot to exercise the skeleton + Finalize end-to-end before any real producer (Discussion/Plan/Webster, or the Someday `Tenter`) needs to exist — the same "fake phases before real producers" approach `loom.md` already specifies for its own skeleton. Does not rewrite `loom.md`'s existing design — records the shared-engine name and scope only. Independent of the landed `Treadle` engine (see the `internal/treadleengine` package documentation) — a different engine, was never blocked on it. See [designs/shed.md](designs/shed.md).
 
 1. **loom: phase-machine skeleton + session bootstrap** — the status-file-driven engine (sequencing, resume, crash-recovery, pause), testable against fake phases before real producers are wired in, plus the `lyx loom run` entry point. Builds on `Shed` above. See [designs/loom.md](designs/loom.md).
 
-1. **`PATTERN.md` — loomyard's own machine-and-review-enforced invariants doc, gating dogfooding** — a from-scratch (not a port) equivalent of Millhouse's `CONSTRAINTS.md`. This is the prerequisite for switching loomyard's *own* development onto `loom` (dogfooding lyx with lyx): you cannot self-host development until lyx has its own enforceable invariants doc. Sequenced last in Planned — it depends on `loom` existing (the dogfooding target) and on `board`/weft being live (where it physically lives — see [designs/board-weft-storage.md](designs/board-weft-storage.md), which already lists `PATTERN.md` as weft content). Format: short two-line entries (constraint + pointer), full rule/rationale/enforcement detail in a linked per-topic doc. Millhouse's own `CONSTRAINTS.md` stays untouched for as long as Millhouse develops loomyard. Also subsumes the constraints-hiding half of Someday's `host-visibility` item (PATTERN-in-weft is already invisible to the host repo).
+1. **`PATTERN.md` — loomyard's own invariants mechanism, wired into every agent** — a from-scratch equivalent of Millhouse's `CONSTRAINTS.md`, owned by loomyard (which has no such mechanism today; the root `CONSTRAINTS.md` is Millhouse's, present only because mill develops loomyard). A weft-backed `_pattern/` folder whose invariants are injected as a pointer into every code-touching agent prompt. **The wiring has landed**: the `hubgeometry`/`fabricengine`/`initengine` junction plumbing, the `internal/pattern` active-check leaf, the `stencil` optional-marker extension, and the `{{.pattern_directive}}` marker in all five code-touching templates (builder implementer, burler round, webster fork, webster Master, loom plan) are all built and merged. **The content migration** out of `CONSTRAINTS.md` into `_pattern/PATTERN.md` + detail docs remains outstanding and still happens only at loomyard-init-via-lyx — `CONSTRAINTS.md` stays the single live invariants doc until that cutover. Also supersedes the constraints-hiding half of Someday's `host-visibility`. See [designs/pattern.md](designs/pattern.md).
+
+1. **codeintel: LSP-backed code intelligence — V1 Go-only, built for multi-language** (promoted from Someday) — gives planner/implementer/reviewer fast, deterministic "where is this defined / used" lookups so they stop grepping blindly and stop paying an LLM round per false-positive hit; also what makes plan-format-v3's symbol fields trustworthy. lyx is an LSP **client**, never a server — it drives published language-server binaries (`gopls` first). Two consumer entry points on one engine: an in-process **Go API** (webster's DAG-derivation) and a **`lyx codeintel references|definition|symbol` CLI** for agents (**no MCP** — the fixed 2–3 query surface doesn't justify it, and a CLI is one code path + engine-neutral + fits the CLI/Cobra invariant). The lifecycle is one `EnsureServer(lang, worktree)` seam with two swappable spawn strategies behind it — `native` (`gopls -remote=auto`, gopls owns supervision) and `supervised` (our own state-file/auto-spawn/staleness/detached-spawn daemon, for `ty`/OmniSharp which have no native shared-daemon). **Independent of the rest of the Planned queue** (no dependency on board / native-clients / fabric / loom) — buildable now, in parallel. V1 populates the registry for Go only but locks its format for all three planned languages, and proves the `supervised` strategy by running it against a plain `gopls` so layer 2 is validated before any C#/Python dependency exists. See [designs/codeintel-redesign.md](designs/codeintel-redesign.md).
 
 ## Someday
 
@@ -32,12 +35,8 @@ Committed to eventually — will be done — but not scheduled next. No build or
 
 1. **Real-Linux validation** — run the sandbox suite and validate every tmux/`/proc` assumption on a real Linux box (built and cross-compiled so far, never executed there).
 
-1. **codeintel** — full four-layer design (toolchain manager, daemon/supervisor, LSP client, language registry) exists; deprioritized until loom's first end-to-end run lands. See [designs/codeintel-redesign.md](designs/codeintel-redesign.md).
-
 1. **raddle** — codeguide's woven-in successor; parallel-regeneration design exists; deferred phase slot between Builder and Finalize. See [designs/raddle.md](designs/raddle.md).
 
-1. **fabric: unified-repo view** — extend the "junctions make weft look like part of the host repo" illusion all the way through `fabric`'s own API: a single auto-routing `Fabric.Commit`, a unified diff/status spanning both repos, and SHA-bookkeeping reuse — all while keeping the existing "an LLM never decides weft-commit timing" invariant intact, only enforced more consistently (a hard block, not a silent no-op). Several sub-questions still open. See [designs/fabric-unified-view.md](designs/fabric-unified-view.md).
-
 1. **webster: parallel card execution** — worktree-per-card concurrent forking with a DAG; explored twice (pre- and during vacation discussion), rejected both times for git-index-race and mid-flight-visibility hazards. See [designs/webster-parallel-execution.md](designs/webster-parallel-execution.md).
 
 1. **Tenter + Hardener** — behavior-based hardening of a live-substrate module (the archetype: `reed` driving real tmux) in a sandbox repo, on-demand and post-loom, off the `shuttle → burler → perch → loom` spine. Concept still being figured out. `Tenter` is the review-loop (`Treadle` configured for behavior-review, `perch`'s direct sibling); `Hardener` is the full campaign (`Shed` + `Tenter`, worktree-spawn via `fabric` + safe-merge-back, the same lifecycle `loom` uses). Both stay Someday — neither is needed to get `loom` running, unlike the Planned `Treadle`/`Shed`/perch-rewrite work they build on once scheduled. See [designs/hardener.md](designs/hardener.md) (a DRAFT doc, do not implement from it yet).

```

## Instructions

1. Read the failing tests and the source files they exercise.
2. Fix the root cause of the failures. Do not modify tests unless they are genuinely wrong due to the merge (e.g. a test asserted against a value that the merge legitimately changed).
3. Re-run `go test -tags integration -race -count=1 ./internal/gitrepo/...` after each fix attempt using `git -C /home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients` for git commands.
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

Available: Read, Edit, Write, Bash, Grep, Glob. Use `git -C /home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients` for git commands; do not `cd`. Worktree cwd is `/home/knatte/Code/loomyard/wts/loomyard.worktrees/native-clients`.
