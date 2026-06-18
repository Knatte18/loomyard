# Discussion: Weft repo — companion-repo overlay for lyx

```yaml
task: Weft repo — companion-repo overlay for lyx
slug: weft-repo
status: discussing
parent: main
```

## Problem

lyx currently assumes its overlay artifacts — `_lyx/` config and task-state, codeguide docs, and the board — live committed inside the host repo. This pollutes repos we don't own and was a constant source of trouble in millpy. The fix is the **weft repo**: a companion git repo that carries all lyx-specific files so the host stays pristine.

Separately, the existing codebase and documentation use the word "hub" to mean two different things: (a) the container directory at the top of the layout, and (b) the main/primary worktree checkout. This is a latent source of confusion and needs to be corrected before the weft architecture, which depends on precise layout terminology, is built on top of it.

This task does two things: establishes the weft model as the canonical architecture for lyx (in docs and roadmap), and executes the foundational code changes that downstream tasks (006–008) depend on — the terminology rename and the config path migration.

## Scope

**In:**
- Rename `Layout.Container → Hub`, `Layout.MainWorktree → Prime`, `HubName() → PrimeName()` throughout all Go source, tests, and docs.
- Migrate config path: `internal/config.Load` reads from `_lyx/config/<module>.yaml` instead of `_lyx/<module>.yaml`; `lyx init` scaffolds the `_lyx/config/` subdirectory and writes configs there.
- Update all doc files (`docs/overview.md`, `docs/roadmap.md`, `docs/modules/worktree.md`, `docs/modules/board.md`, `CONSTRAINTS.md`) to use Hub/Prime terminology and document the weft model as the new architecture.
- Mark portals (`_portals/`) as deprecated in docs; removal lands in task 006 alongside the new weft junction model.
- Add roadmap milestones for tasks 006–008 (weft engine, hub-creator, weft producers).

**Out:**
- `internal/paths` weft geometry methods (`WeftWorktree()`, etc.) — task 006.
- Paired host+weft worktree spawn and teardown — task 006.
- `lyx weft` command (`sync`, `status`) — task 006.
- Hub-creator / `lyx-clone` skill — task 007.
- `_codeguide` junction, `lyx config` TUI, `_lyx/config/` schema definitions — task 008.
- `internal/state` worktree registry — task 006 (registry grows to `{host_path, weft_path, branch}`).

## Decisions

### Hub/Prime terminology

- **Decision:** `Hub` = the container directory (top-level folder that is NOT a git repo). `Prime` = the main/primary worktree checkout (on `main` branch). These terms are mutually exclusive and unambiguous.
- **Rationale:** The existing code used "hub" for the main worktree (in Layout field `MainWorktree`, method `HubName()`) while docs sometimes used "hub" for the container. The weft architecture makes the distinction critical: the hub contains both the prime and the weft prime as siblings.
- **Rejected:** Keeping "MainWorktree" as-is — it describes role (main) but not topology; confusable with "hub" in docs. Renaming to "Root" or "Primary" alone without also fixing "Container" → "Hub" would leave half the confusion in place.

### Weft model — canonical architecture

- **Decision:** All lyx overlay artifacts (`_lyx/`, `_codeguide/`) live in a **weft repo** — a separate companion git repo in the same hub. Each host worktree gets a sibling weft worktree at `<hub>/<slug>-weft/`. The host worktree holds junctions (`_lyx`, `_codeguide`) that route writes into the weft worktree. The host repo stays pristine — junctions are listed in `.git/info/exclude`, never in a committed `.gitignore`.
- **Rationale:** File writes stay open (skills write through junctions, unaware of weft); git on the weft is lyx-owned and geometry-scoped. Generalizes the existing board model (`_board/` is already a separate repo whose commits lyx performs via `RunGit(args, cwd)`).
- **Weft suffix:** Fixed `-weft`. For prime: `<prime-name>-weft/`; for worktrees: `<slug>-weft/`. Not configurable — deterministic geometry needs no config lookup.
- **No registry for weft paths:** The weft path of any worktree is always `<hub>/<dir-name>-weft`. Computed on demand; no `local-state.json` needed.
- **This task documents the model.** The Go implementation (paths geometry, paired spawn, `lyx weft` command) is task 006.

### Config path: `_lyx/config/`

- **Decision:** `internal/config.Load` reads from `_lyx/config/<module>.yaml`. `lyx init` creates `_lyx/config/` and writes configs there. Hard cut — no fallback to the old flat `_lyx/<module>.yaml` path.
- **Rationale:** The config subfolder separates human-relevant config surface (module YAML files) from Claude-written task artifacts (`discussion.md`, `plan.md`, `reviews/`). Cheap to migrate now while the config layer is young; expensive later once tasks 006–008 build on top of it. Single-user project — no migration script needed.
- **Rejected:** Keep flat `_lyx/<module>.yaml` and defer — the cost grows with every downstream task.

### Portals deprecated

- **Decision:** The `_portals/` mechanism (junctions from `<hub>/_portals/<slug>` into each worktree's `_lyx/`) is deprecated. The weft model replaces its use case: the weft worktrees are direct siblings and browsable without portals.
- **Rationale:** Portals were created to let the prime's VS Code browse each worktree's `_lyx/` without navigating away. In the weft model, `<slug>-weft/_lyx/` is a plain sibling directory — no indirection needed.
- **Removal:** Task 006 removes portal creation/teardown from `worktree add`/`remove` and the `createPortal`/`removePortal` functions once weft junctions replace them.

## Technical context

**Rename touchpoints** — every file that references the fields or method being renamed:

| File | What changes |
|---|---|
| `internal/paths/paths.go` | `Layout.Container → Hub`, `Layout.MainWorktree → Prime`, `HubName() → PrimeName()`, internal body of all methods |
| `internal/paths/paths_test.go` | All `.Container`, `.MainWorktree`, `HubName()` references (∼15 sites) |
| `internal/ide/color.go` | `l.Container` (line 49, 70), `l.MainWorktree` (line 55) |
| `internal/ide/color_test.go` | Field/method name references |
| `internal/worktree/portals_test.go` | `l.Container` (lines 37, 99, 139, 189) |
| `internal/worktree/remove_test.go` | `l.Container` (lines 105, 128) |
| `docs/overview.md` | "hub" used for container and main worktree — replace throughout |
| `docs/roadmap.md` | terminology + weft milestones |
| `docs/modules/worktree.md` | "hub" in container layout diagrams, container definition |
| `docs/modules/board.md` | references to hub naming |
| `CONSTRAINTS.md` | Layout method list (currently lists `HubName()`) |

`internal/paths/codeguide_guard_test.go` scans for the literal `_codeguide` substring in production files — unaffected by the rename.

**Config path migration touchpoints:**

| File | What changes |
|---|---|
| `internal/config/config.go` | `loadYAMLLayer(filepath.Join(baseDir, "_lyx", module+".yaml"))` → `.../_lyx/config/<module>.yaml` |
| `internal/board/init.go` | Create `_lyx/config/` dir; write `board.yaml` and `worktree.yaml` there |
| `internal/config/config_test.go` | All `_lyx/board.yaml` fixture paths → `_lyx/config/board.yaml`; add test that `_lyx/config/` dir is created |
| `internal/board/init_test.go` | Assert `_lyx/config/board.yaml` output, not `_lyx/board.yaml` |

**`FindBaseDir`** in `internal/config/config.go` checks for `_lyx/` existence — unchanged; the `_lyx/` directory is still the init marker.

## Constraints

From `CONSTRAINTS.md` (enforced at build time):
- All cwd and worktree root queries go through `paths.Getwd()` and `paths.Resolve()`. Raw `os.Getwd` and `git rev-parse --show-toplevel` are banned outside `internal/paths` and `cmd/lyx/main.go`.
- Enforced by `internal/paths/enforcement_test.go`.

For this task:
- The rename is strictly behavior-preserving. No logic changes — only identifier names. All existing tests must pass after rename.
- Config path migration is a hard cut. No conditional fallback. There are no committed `_lyx/board.yaml` or `_lyx/worktree.yaml` files in the repo to migrate (the files live in the user's local worktrees, not committed).

## Testing

**Terminology rename (behavior-preserving):**
- `internal/paths/paths_test.go`: update all field references (`layout.Container → layout.Hub`, `layout.MainWorktree → layout.Prime`) and `HubName()` → `PrimeName()` calls. No new test cases — existing coverage is sufficient; these are pure name changes.
- `internal/ide/color_test.go`, `internal/worktree/portals_test.go`, `internal/worktree/remove_test.go`: same — field name updates only.

**Config path migration:**
- `internal/config/config_test.go`: update all fixture code that creates `_lyx/board.yaml` to instead create `_lyx/config/board.yaml`. Add one test verifying that a YAML file at the old path `_lyx/board.yaml` is NOT picked up (regression guard for the hard cut).
- `internal/board/init_test.go`: assert that `lyx init` produces `_lyx/config/board.yaml` and `_lyx/config/worktree.yaml`, not the old flat paths. Assert `_lyx/config/` directory is created.

## Q&A log

- **Q:** Does the weft worktree live in a `_weft/<slug>/` subdirectory (as per the original wiki design)? **A:** No. Weft worktrees are plain siblings in the hub: `<hub>/<slug>-weft/`. No `_weft/` subdirectory.
- **Q:** Is the `-weft` suffix configurable? **A:** No. Fixed suffix; weft path is always computable from geometry.
- **Q:** Does task 005 include the Go weft implementation (paths geometry, paired spawn, `lyx weft`)? **A:** No. That is task 006. Task 005 is the design pivot + foundational renames that 006 builds on.
- **Q:** Is `_codeguide` junctioned alongside `_lyx`? **A:** Not in task 005 or 006. Codeguide activation is a separate step (task 008).
- **Q:** Do portals remain in the weft model? **A:** Portals are obsolete; deprecated in this task's docs, removed in task 006.
- **Q:** Is the config path migration a hard cut or does it fall back to the old path? **A:** Hard cut. No fallback.
- **Q:** Does `lyx worktree add` need to bootstrap the weft repo? **A:** No. The weft repo is a prerequisite of using lyx; `worktree add` (task 006) assumes it exists and fails fast if not.
- **Q:** Is a worktree registry (`local-state.json`) needed for weft paths? **A:** No. Weft path is always `<hub>/<slug>-weft`; computable on demand.
