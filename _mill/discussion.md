# Discussion: Harden the Path Invariant: close enforcement hole + fix geometry leaks

```yaml
task: 'Harden the Path Invariant: close enforcement hole + fix geometry leaks'
slug: harden-path-invariant
status: discussing
parent: main
```

## Problem

The repo has a **Path Invariant** (`CONSTRAINTS.md`): `internal/paths` is the single
owner of all worktree/container geometry, and no other package may recompute it. The
guard that enforces this — `internal/paths/enforcement_test.go` — only bans **two
tokens**: `os.Getwd` and `--show-toplevel`. It catches the *entry points* into geometry
(cwd, git root) but not the *construction* of geometry from string literals. CONSTRAINTS.md
admits the gap in prose (the `_lyx`/config-path rule "is **not** caught by
enforcement_test.go … it is a code-review and planning-discipline rule").

Because construction was never machine-enforced, geometry has leaked past the guard:

- **warp reimplements geometry paths already owns.** `paths.WeftWorktreePath(slug)` is
  exactly `filepath.Join(l.Hub, slug+"-weft")`, yet `warpengine` rebuilds that same join
  by hand in `prune.go`, `reconcile.go`, and `status.go`, plus hardcoded `weftSuffix` /
  `boardDirName` / `HubSuffix` constants in `clone.go`. `prune.go` also does the *inverse*
  (strips `-weft` with index math to recover a host slug).
- **board geometry has no paths helper at all.** `<hub>/_board` exists only as the
  relative config default `path: ${env:LYX_BOARD_PATH:-../_board}` in board's template;
  `paths` has no board accessor, so board *cannot* go through paths today even though
  `<hub>/_board` is hub geometry.

**Why now:** the invariant is load-bearing for loom (the future consumer of every engine)
and for cross-OS junction geometry. Each un-enforced leak is a latent migration-breaker —
exactly the class of bug PR #20 hit when a hardcoded test path drifted from the loader.
This task closes the enforcement hole (machine-enforce geometry *construction*, not just
*entry*) and converts the known leaks.

## Scope

**In:**

- **`internal/paths` — become the sole owner of the geometry vocabulary, in three layers:**
  - Exported constants (single source of the literals): `WeftSuffix = "-weft"`,
    `BoardDirName = "_board"`, `HubSuffix = "-HUB"`.
  - Pure package functions for bootstrap callers that have no `Layout` yet:
    `WeftSiblingPath(hub, slug)`, `BoardDir(hub)`, `HubPath(parent, name)`.
  - Reverse parser: `WeftHostSlug(name) (slug string, ok bool)` — recovers the host slug
    from a `-weft` sibling name; owns the suffix for *matching*, not just construction.
  - Refactor the three existing `Layout` weft methods to delegate to the new consts/funcs
    (thin wrappers; no inline `"-weft"` left).
- **`internal/warpengine` — route every geometry site through `paths` (zero local literals):**
  - `prune.go:79`, `reconcile.go:102`, `status.go:91` (construction) → `Layout` weft methods.
  - `prune.go` pass-2 suffix strip/match → `paths.WeftHostSlug(name)`.
  - `clone.go` — delete the local `weftSuffix` / `boardDirName` / `HubSuffix` consts; build
    paths via `paths.WeftSiblingPath` / `paths.BoardDir` / `paths.HubPath`. (clone then needs
    no `Layout` — pure funcs + consts suffice during bootstrap.)
- **`internal/boardengine` + `internal/boardcli` — board data dir becomes paths-owned:**
  - Remove the `path:` key from the board config template (`template.yaml`). Geometry is
    paths-owned and must **not** be config/env-overridable.
  - Board data dir resolves: `--board-path` flag (explicit, transient) **>**
    `paths.BoardDir(l.Hub)`. `LYX_BOARD_PATH` falls away as a consequence (geometry has no
    env override) — **not** as part of any env-removal initiative.
  - Reword `boardcli` `Long` so it stops conflating the config *file* (cwd-resolved) with
    the board *data dir* (paths-derived).
- **`internal/paths/enforcement_test.go` — add an AST-based geometry-literal scan** (keep the
  existing `os.Getwd` / `--show-toplevel` substring ban). Production-only; flags geometry-token
  literals only in path-construction context. Details under Decisions.
- **`CONSTRAINTS.md`** — record the now-machine-enforced geometry-construction ban and the new
  `paths` API (same commit, per doc-lifecycle rule).
- **`docs/shared-libs/paths.md`** — document the new consts/functions and the enforcement.

**Out:**

- **Any env-var removal or template flattening beyond the board `path:` key.** The
  `${env:NAME:-default}` pattern is desired design and stays. Board template keeps
  `home: ${env:LYX_HOME:-Home.md}`, `sidebar: ${env:LYX_SIDEBAR:-_Sidebar.md}`,
  `proposal_prefix: ${env:LYX_PROPOSAL_PREFIX:-proposal-}` **untouched** — these are
  non-geometry filenames with optional overrides. Do **not** touch the `${env:}` engine
  (`yamlengine`/`envsource`/`.env`), warp's `LYX_BRANCH_PREFIX`, the `*_SKIP_*` toggles, or
  `VISUAL`/`EDITOR`. "loomyard should never *need* env-vars" is a separate future direction,
  not this task.
- **`-HUB` as a paths *function* beyond `HubPath`** — `HubSuffix` const + `HubPath` is the
  full extent. No new Hub `Layout` method (the hub doesn't exist as a git repo at clone time).
- **status.go git-pathspec literals** (`"_lyx"` / `"_codeguide"` passed to `git ls-files` and
  used in `strings.HasPrefix` on git output) — left as-is; they are pathspec/parse usage, not
  path construction. Documented as allowed (see Decisions).
- **Test-file geometry** — the enforcement scan is production-only; test fixtures that build
  `_board`/`-weft` paths are not flagged (review-discipline, as today).
- **`_portals` / `_launchers` / `_codeguide` constants** — these tokens stay as inline literals
  inside `internal/paths` (already paths-owned/allowlisted). No new constants for them; only the
  three suffix/dir names warp needs (`WeftSuffix`, `BoardDirName`, `HubSuffix`) are extracted.

## Decisions

### paths owns the geometry vocabulary in three layers

- Decision: Add to `internal/paths`:
  - Consts: `WeftSuffix = "-weft"`, `BoardDirName = "_board"`, `HubSuffix = "-HUB"` (exported).
  - Pure funcs: `WeftSiblingPath(hub, slug string) string = filepath.Join(hub, slug+WeftSuffix)`;
    `BoardDir(hub string) string = filepath.Join(hub, BoardDirName)`;
    `HubPath(parent, name string) string = filepath.Join(parent, name+HubSuffix)`.
  - Reverse parser: `WeftHostSlug(name string) (slug string, ok bool)` — returns
    `(strings.TrimSuffix(name, WeftSuffix), true)` when `name` ends with `WeftSuffix` **and**
    the stripped slug is non-empty; otherwise `("", false)`. (Matches `prune.go`'s current
    guard `len(name) <= len("-weft")` → skip.)
  - The existing `Layout` methods become thin wrappers: `WeftWorktreePath(slug)` →
    `WeftSiblingPath(l.Hub, slug)`; `WeftRepoRoot()` → `WeftSiblingPath(l.Hub, l.PrimeName())`;
    `WeftWorktree()` → `WeftSiblingPath(l.Hub, filepath.Base(l.WorktreeRoot))`. No inline
    `"-weft"` remains in `paths.go`.
- Rationale: One source for every geometry literal and its inverse. Bootstrap callers (clone,
  before any git repo exists) use the pure funcs; `Layout` holders use the methods; the reverse
  parser keeps suffix *matching* paths-owned too. This lets the enforcement test ban geometry
  literals in warp with **zero allowlist exceptions** — the whole point.
- Rejected: Constants-only (no reverse helper) — leaves `prune.go`'s `strings.TrimSuffix(name,
  paths.WeftSuffix)` inline, re-encoding the match logic at the call site. Synthetic `Layout`
  for clone — hacky; clone has no resolvable git root yet.

### warp routes every geometry site through paths

- Decision: Concrete mapping:
  - `prune.go:79` `Join(l.Hub, slug+"-weft")` → `l.WeftWorktreePath(slug)`.
  - `prune.go` pass-2 (`len(name) <= len("-weft")` / slice strip) → `paths.WeftHostSlug(name)`
    (skip when `ok == false`).
  - `reconcile.go:102` → `l.WeftWorktreePath(slug)` (slug = `filepath.Base(hostPath)`).
  - `status.go:91` → `l.WeftWorktreePath(filepath.Base(hostPath))`.
  - `clone.go` — delete consts; hub build `<cwd>/<name>-HUB` → `paths.HubPath(cwd, name)`;
    `<hub>/<name>-weft` → `paths.WeftSiblingPath(hub, name)`; `<hub>/_board` →
    `paths.BoardDir(hub)`.
- Rationale: All three construction sites are equivalent joins paths already owns; clone's
  consts move to their rightful owner. Behaviour is byte-identical.
- Rejected: Leaving clone/prune-parse as accepted bypasses — would force an allowlist entry and
  defeat the zero-exception goal.

### board data dir is paths-owned, not config/env-overridable

- Decision: Remove `path:` from `board/template.yaml`. `boardcli` resolves the data dir as
  `--board-path` flag (absolute, transient, injected by the detached sync child) **>**
  `paths.BoardDir(l.Hub)`. `boardengine` stays oblivious — it still receives a fully-resolved
  `Config.Path`; only the *source* of that path changes (cli-populated, not yaml-populated).
- Rationale: `<hub>/_board` is hub geometry; geometry must not be config- or env-overridable.
  The flag remains the single explicit transient override. Side benefit: fixes a latent sub-path
  bug — today `filepath.Join(cwd, "../_board")` is wrong when invoked from a sub-directory;
  `paths.BoardDir(l.Hub)` is correct from anywhere.
- Rejected: Keep `LYX_BOARD_PATH` as an env tier (geometry shouldn't have env overrides);
  require `LYX_BOARD_PATH` absolute (drops a working relative form for no benefit and keeps an
  env geometry override we're removing).

### board template: touch only the `path:` key

- Decision: Delete exactly one line (`path: ${env:LYX_BOARD_PATH:-../_board}`). Leave `home`,
  `sidebar`, `proposal_prefix` with their `${env:...:-default}` form **unchanged**.
- Rationale: Those three are non-geometry filenames with optional overrides — precisely the
  desired `${env:NAME:-default}` design. They are not geometry and not in this task's scope.
  A half-env/half-plain template would be an ugly in-between, but the correct resolution is to
  leave the env form intact, not to flatten it.
- Rejected: Flatten the remaining keys to plain defaults (out of scope; the env-override form is
  intentional); broad `${env:}` removal (explicitly a separate future direction).

### enforcement test: AST scan for geometry construction, production-only

- Decision: Rewrite `internal/paths/enforcement_test.go` to keep the existing substring ban
  (`os.Getwd`, `--show-toplevel`; allowlist `internal/paths` + `cmd/lyx/main.go`; skips
  `_test.go`) **and add** an AST scan that:
  - Parses each non-test `.go` file outside `internal/paths` with `go/parser` (mirroring
    `cmd/lyx/registration_test.go`).
  - Token set: `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_codeguide`, `_lyx`.
  - Flags a geometry-token **string literal** only in path-construction context:
    (a) an argument to a `filepath.Join(...)` call, or (b) an operand of a binary `+` expression.
  - Also flags a string **const declaration** whose value is exactly a geometry token (catches
    `const weftSuffix = "-weft"` at its source).
  - Allowlist for the geometry scan: `internal/paths` only (zero warp exceptions). Verify
    `cmd/lyx/main.go` is clean of geometry construction (its module-list names `board`, not
    `_board`).
  - Keeps a predicate/AST-fixture sub-test in sync: synthetic positives
    (`filepath.Join(x, "_board")`, `slug + "-weft"`, `const s = "-weft"`) must flag; synthetic
    negatives (a doc comment, `Long: "...-weft..."` struct-field literal, a plain non-token
    string) must pass.
- Rationale: A substring extension would false-positive on doc comments, cobra help prose (warp's
  `Long` describes the `<name>-weft` / `_board` layout for users), and template strings. AST
  scoping to Join/`+`/const-decl catches *construction* and never *description*. Production-only
  mirrors the existing `_test.go` skip and `registration_test.go`; several test fixtures
  legitimately build these paths.
- Rejected: Substring scan of geometry tokens (false positives on prose); scanning test files
  (would flag legitimate fixtures — `lyxtest.go`, `ideengine/menu_test.go`, `clone_integration_test.go`,
  board config tests; test geometry stays a review rule).

### status.go git-pathspecs are allowed bypasses

- Decision: Leave `status.go:235` (`[]string{"ls-files", "--", "_lyx", "_codeguide"}`),
  `status.go:260`/`:271` (`strings.HasPrefix(tracked, "_lyx")` / `"_codeguide"`) as-is. Record
  them in CONSTRAINTS.md's "legitimately allowed to bypass paths" list.
- Rationale: These are git pathspec arguments and string comparisons on git output — not
  filesystem path construction. The AST detector (Join/`+`/const-decl) correctly ignores them.
  Documenting them prevents a future reviewer reading them as missed leaks.
- Rejected: Constant-izing them (would require a new `_codeguide` constant for non-construction
  use, expanding scope past geometry ownership for no enforcement benefit).

## Technical context

Modules and files mill-plan needs:

- **`internal/paths/paths.go`** — the owner. Existing weft methods (`WeftRepoRoot`,
  `WeftWorktreePath`, `WeftWorktree`, `WeftLyxDir*`, `HostLyxLink*`, `HostJunctions`) already
  use inline `"-weft"` / `"_lyx"` / `"_codeguide"`. Add the consts/pure-funcs/reverse-parser;
  refactor the three weft methods to delegate. `LyxDirName = "_lyx"` already exists as a const.
- **`internal/paths/enforcement_test.go`** (133 lines) — current substring guard. Rewrite as
  above. `cmd/lyx/registration_test.go` is the AST template to copy: `runtime.Caller(0)` →
  repoRoot, `filepath.WalkDir`, `parser.ParseFile(..., parser.SkipObjectResolution)`,
  `ast.Inspect`. Reuse its `discovered_non_empty`-style sanity sub-test so a silently-broken
  walk can't all-pass.
- **`internal/warpengine/clone.go`** (consts at lines 16-25) — `HubSuffix` is currently
  **exported** from warp and used by warpcli/tests; moving it to `paths.HubSuffix` means updating
  those references. `weftSuffix`/`boardDirName` are unexported. `clone_integration_test.go`
  references `boardDirName` (lines 95, 181) — those become `paths.BoardDir(hubPath)` /
  `paths.BoardDirName` (test won't be scanned, but the deleted const breaks compilation, so the
  reference must be updated).
- **`internal/warpengine/{prune,reconcile,status}.go`** — leak sites enumerated above. `prune.go`
  has the only *reverse* parse (pass-2, ~lines 121-128).
- **`internal/boardcli/cli.go`** — `PersistentPreRunE` (lines 60-96): currently
  `boardengine.LoadConfig(cwd, "board")` supplies `Config.Path` from the `path:` key. Rewire so
  the normal branch resolves `layout, _ := paths.Resolve(cwd)` and sets
  `cfg.Path = paths.BoardDir(layout.Hub)` after `LoadConfig` returns the non-geometry keys. The
  `--board-path` branch (`cfg = boardengine.Config{Path: *boardPathFlag}`) is unchanged. The bare
  `lyx board` group path (guard `if cmd.Name() == "board"`) skips PreRunE → no `Resolve` needed
  without a git repo. `Long` (lines 39-43) is the help-prose reword; the file-header comment
  (lines 1-8) describes the old resolution and should be updated too.
- **`internal/boardengine/config.go`** — `Config.Path` has `yaml:"path"` (line 21) and `LoadConfig`
  resolves it relative to `baseDir` (lines 74-77). With `path:` gone from the template, drop the
  `yaml:"path"` tag and the relative-resolution block; `Path` becomes a cli-populated field like
  `SkipGit`/`SkipPush`. `boardengine.LoadConfig` strict validation uses
  `yamlengine.MissingKeys(template, file)` (checks template keys missing from file) — removing
  `path:` from the template means it's no longer required; an old committed `board.yaml` that
  still has `path:` is harmless (extra key, ignored).
- **`internal/boardengine/template.yaml`** — delete line 1 only.
- **`docs/shared-libs/paths.md`** — the paths shared-lib doc; update for the new API. There is no
  per-module doc for warp/board under `docs/modules/`, and `docs/overview.md` needs no change
  (no module added/removed). `docs/roadmap.md` is **not** touched (this is hardening, not a
  milestone).
- **`CONSTRAINTS.md`** — Path Invariant section is the authoritative invariant doc; update in the
  same commit.

Gotchas:

- `paths.Resolve` shells `git rev-parse --show-toplevel`; it works only inside a git repo. board's
  subcommands always run in a worktree, so the new `Resolve` call in `boardcli` is safe; the bare
  group listing never reaches it.
- Behaviour parity: every warp conversion must produce byte-identical paths (the pure funcs are
  the same joins). prune/reconcile/status output and clone layout must be unchanged.
- The reverse parser's empty-slug guard must match prune's current `len(name) <= len("-weft")`
  semantics (a bare `-weft` name yields `ok == false`).

## Constraints

From `CONSTRAINTS.md` (authoritative):

- **Path Invariant** — the invariant being hardened. All geometry through `internal/paths`;
  `os.Getwd` / `--show-toplevel` banned outside `internal/paths` + `cmd/lyx/main.go`. This task
  *extends* the machine-enforcement to geometry-dir literal construction and adds `paths.BoardDir`,
  `WeftSiblingPath`, `HubPath`, `WeftHostSlug`, and the `WeftSuffix`/`BoardDirName`/`HubSuffix`
  consts to the helper inventory. Record that geometry-dir literals outside paths are now banned
  (in construction context) by the extended enforcement test.
- **lyxtest Leaf Invariant** — unaffected (no `lyxtest` import changes), but don't introduce a
  `lyxtest → configreg`/feature edge while touching tests.
- **CLI / Cobra Invariant** — board's `Short`/`Long` are review-checked for accuracy against the
  changed behaviour: the reworded board `Long` MUST match the new resolution (config file from
  cwd; data dir from `paths.BoardDir`/flag). `cmd/lyx/helptree_test.go` pins module/subcommand
  *names* (not `Long` prose) — no help-tree change expected, but verify no test asserts the old
  `Long` string. Keep the JSON envelope / error-as-JSON behaviour intact.
- **Documentation Lifecycle** — docs updated in the same commit (`paths.md`, `CONSTRAINTS.md`).
- **fslink** (from `CLAUDE.md`) — not touched, but note geometry that feeds junctions
  (`HostJunctions`) stays paths-owned.

## Testing

- **`internal/paths` (TDD candidate — the enforcement test is the deliverable):**
  - New unit tests for `WeftSiblingPath`, `BoardDir`, `HubPath`, `WeftHostSlug` (incl. the
    empty-slug `ok == false` edge and a non-`-weft` name).
  - Assert the refactored `Layout` methods (`WeftWorktreePath`, `WeftRepoRoot`, `WeftWorktree`)
    still return identical paths (existing `paths_test.go` / `weft_test.go` should stay green).
  - Enforcement AST scan: predicate/fixture sub-test with synthetic positives
    (`filepath.Join(x, "_board")`, `slug + "-weft"`, `const s = "-HUB"`) and negatives (comment,
    `Long:` struct-field literal, plain string). A `discovered_non_empty`-style sanity sub-test so
    a misconfigured walk can't pass vacuously. The real tree-scan must pass once warp is converted.
- **`internal/warpengine`:** existing prune/reconcile/status/clone tests must stay green
  (behaviour unchanged). Update `clone_integration_test.go` references to the deleted consts. A
  focused test for `paths.WeftHostSlug` parity with the old prune pass-2 logic (same host slugs
  recovered, same skips).
- **`internal/boardengine` / `internal/boardcli`:** the `path:`-resolution tests in
  `config_test.go` (relative/absolute/env/`../` cases) become obsolete with the key removed —
  delete or repurpose. `template_test.go` expects `path` among required keys and asserts
  `{"path", "../_board"}` resolution — update to drop `path`. Add a `boardcli` test that the data
  dir resolves to `paths.BoardDir(l.Hub)` by default and that `--board-path` overrides it. Verify
  `boardtest/concurrency_test.go` and `bench_test.go` (which seed `path: board`) still pass — their
  board-dir expectation moves from `<cwd>/board` to `paths.BoardDir(hub)`; adjust seeds/expectations
  as needed.
- **Repo-wide:** `go build ./...` and `go test ./...` green. The enforcement test is the gate —
  it must fail on a reintroduced `filepath.Join(x, "_board")` / `slug + "-weft"` /
  `const s = "-weft"` outside paths, and pass on the fully-converted tree.

## Q&A log

- **Q:** What exactly is the "enforcement hole"? **A:** The guard bans only the *entry* tokens
  (`os.Getwd`, `--show-toplevel`); it never caught geometry *construction* from string literals.
  That's the hole — close it with an AST construction scan.
- **Q:** How far should warp's `-weft`/`_board` literals be pushed through paths? **A:** All the
  way — three layers (consts, pure bootstrap funcs, reverse parser), so warp has zero geometry
  literals and the enforcement test needs zero warp allowlist exceptions. Include `-HUB` in the
  ban set and move `HubSuffix` into paths too.
- **Q:** Does `prune.go`'s reverse `-weft` strip (pass-2) get converted? **A:** Yes — via the new
  `paths.WeftHostSlug(name)`; the suffix must be paths-owned for matching, not only construction.
- **Q:** Does the enforcement scan cover `_test.go` files? **A:** No — production-only (mirrors
  `registration_test.go` and the existing `os.Getwd` skip). Test-file geometry stays a review rule;
  too many legitimate fixtures build these paths.
- **Q:** What about `status.go`'s `_lyx`/`_codeguide` used with `git ls-files` / `strings.HasPrefix`?
  **A:** Leave them — pathspec args and parse comparisons, not path construction. The detector
  ignores them; record them in the "legitimately allowed" list.
- **Q:** How is the board data dir resolved after removing `path:`? **A:** `--board-path` flag
  (explicit transient) > `paths.BoardDir(l.Hub)`. No env tier — geometry is not env-overridable.
  `boardengine` stays oblivious (still gets a resolved `Config.Path`).
- **Q:** Does removing the board template `path:` mean stripping all its env-vars / flattening it?
  **A:** No. Touch **only** the `path:` key. `home`/`sidebar`/`proposal_prefix` keep their
  `${env:NAME:-default}` form — that pattern is the desired design (optional override of
  non-geometry filenames). The geometry vs non-geometry line is the whole distinction: geometry
  (`path:`) is never overridable; non-geometry filenames may be.
- **Q:** Is "loomyard should never need env-vars" part of this task? **A:** No — separate future
  direction. This task is Path-Invariant hardening, scope = geometry only. No env-engine removal,
  no `LYX_BRANCH_PREFIX` / `*_SKIP_*` / editor-var changes, no backlog task filed.
