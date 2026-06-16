# Discussion: Rename mhgo to Loomyard (lyx)

```yaml
task: Rename mhgo to Loomyard (lyx)
slug: rename-to-loomyard
status: discussing
parent: main
```

## Problem

The project is being rebranded from **mhgo** to **Loomyard**, with **`lyx`** as
the CLI command. The GitHub repository has already been renamed out-of-band — the
git remote is now `https://github.com/Knatte18/loomyard.git` — but the source tree
still says `mhgo` everywhere: `go.mod` declares `module github.com/Knatte18/mhgo`,
all 80 import statements use that stale path, the binary lives at `cmd/mhgo`, and
the on-disk state dirs, env-var names, gitignore markers, Go identifiers, and docs
all carry the old name. **Why now:** the repo rename already happened, so the
module path no longer matches its canonical location; finishing the rename across
the tree removes the inconsistency in one pass before more code is built on top of
the old names.

This is a mechanical, repo-wide rename. There is no behavioral change to any
module — `go build ./...` and `go test ./...` must pass identically before and
after, with every reference moved to the new brand.

## Scope

**In:**

- **Module path:** `github.com/Knatte18/mhgo` → `github.com/Knatte18/loomyard` in
  `go.mod` and all 80 Go import sites (10 internal packages: `board`, `config`,
  `git`, `gitignore`, `ide`, `lock`, `muxpoc`, `output`, `paths`, `worktree`).
- **CLI binary + directory:** `cmd/mhgo/` → `cmd/lyx/` (git-mv the directory;
  rename `main.go` package doc and usage strings `usage: mhgo <module>` →
  `usage: lyx <module>`, `Command mhgo` → `Command lyx`). The git-mv moves
  `cmd/mhgo/main_test.go` but NOT its content: that file hardcodes `_mhgo` /
  `mhgoDir` / `_mhgo/board.yaml` literals (lines ~43–50, ~73–80) which must be
  renamed to `_lyx` — otherwise `TestRunDispatchesToBoard` /
  `TestRunBoardErrorPropagatesExitCode` create the wrong dir (config now looks
  for `_lyx/`) and fail.
- **Managed-state directory:** `_mhgo/` → `_lyx/` everywhere (the dir holding
  `board.yaml`, `worktree.yaml`, `<module>.yaml`). Source sites: `paths.go`
  `MhgoDir()`, `config.go` `FindBaseDir`/`Load`, `board/init.go`, `ide/menu.go`,
  plus all tests.
- **Local-state directory:** `.mhgo/` → `.lyx/` (gitignored runtime state:
  `muxpoc-state.json`, `muxpoc-state.lock` in `muxpoc/state.go`; the
  `gitignore.Ensure(cwd, ".mhgo/")` call in `board/init.go`; all tests).
- **gitignore block markers:** `# === mhgo-managed ===` / `# === end mhgo-managed ===`
  → `lyx-managed` (in `internal/gitignore/gitignore.go` constants + package doc,
  and the assertions in `gitignore_test.go` / `board/init_test.go`).
- **Root tracked `.gitignore`:** the hand-written binary-ignore patterns `/mhgo`
  and `mhgo.exe` (lines 4–5) → `/lyx` and `lyx.exe`, and the local-state entry
  `.mhgo/` (line 8) → `.lyx/`. These go stale once the binary becomes `lyx`/`lyx.exe`
  and the local-state dir becomes `.lyx/`. **Do NOT touch** the
  `# === mill-managed ===` block lower in the same file — that is the millhouse
  harness block, out of scope (see Out).
- **Exported Go identifier:** `Layout.MhgoDir()` → `Layout.LyxDir()` (1 definition
  in `paths.go` + all call sites). Local/unexported identifiers (`mhgoDir`,
  `mhgoPath`, `mhgoFile`, `mhgoIdx`, `mhgo_dir`, `dotMhgoDir`, and test names like
  `_DotMhgoIgnored`) renamed to the `lyx`/`Lyx` equivalent.
- **Env-var prefix:** `MHGO_*` → `LYX_*` (`MHGO_HOME`, `MHGO_BOARD`,
  `MHGO_BOARD_PATH`, `MHGO_SIDEBAR`, `MHGO_PROPOSAL_PREFIX`, `MHGO_BRANCH_PREFIX`,
  `MHGO_CODE_REVIEWER`). These appear as `$env:NAME` examples in `board/init.go`'s
  generated `board.yaml` comment block and in `docs/shared-libs/config.md`. The
  test placeholder `NONEXISTENT_MHGO_TEST_VAR_XYZ` → `NONEXISTENT_LYX_TEST_VAR_XYZ`.
- **Integration-test repo URL:** `https://github.com/Knatte18/mhgo-wiki-test.git`
  → `https://github.com/Knatte18/loomyard-test.git` (note the new name is
  `loomyard-test`, **not** `loomyard-wiki-test`). Sites: `testRepoURL` const in
  `internal/board/boardtest/integration_test.go`, and the references in
  `docs/benchmarks/board-performance.md`.
- **Brand prose & comments:** all docs under `docs/` (overview, roadmap,
  modules/*, shared-libs/*, benchmarks/*, psmux-tui-behavior), package-level Go doc
  comments, and `CONSTRAINTS.md`. Apply the prose voice rule (see Decisions →
  prose-voice).
- **Test-fixture strings:** `@mhgo.dev` git-config emails → `@loomyard.dev`;
  illustrative paths `C:\Code\mhgo\…` → `C:\Code\loomyard\…`; example slug
  `mhgo-mux-design` → `loomyard-mux-design` (input **and** expected output move
  together in `muxpoc/state_test.go`); psmux probe labels `mhgoprobe` /
  `mhgohookprobe` in mux exploration docs → `lyxprobe` / `lyxhookprobe`.
- **Harness config (tracked):** `mill-config.yaml` `repo.short_name: "MHGO"` →
  `"LYX"`.
- **Path-invariant references:** the `cmd/mhgo` allowlist entry in
  `internal/paths/enforcement_test.go` → `cmd/lyx`, plus the matching comments in
  `enforcement_test.go`, `paths.go`, and `CONSTRAINTS.md` (which name
  `cmd/mhgo/main.go`).

**Out:**

- **No behavioral changes.** Logic, control flow, and public behavior of every
  module stay identical. This is rename-only.
- **No backward-compatibility shim.** `lyx` does **not** read old `_mhgo/` /
  `.mhgo/` directories. There are no existing mhgo-managed repos outside this one
  (confirmed by the operator), so a clean break is safe.
- **The millhouse task-harness files** under `_mill/`, `.millhouse/`, the `.wiki`
  junction, the worktree's `.portals` junction, and `.vscode/settings.json` are NOT
  part of the rename. `.vscode/settings.json` is gitignored (only its
  `mill-config.yaml` `short_name` source is tracked, and that one key IS in scope).
  Do not edit anything in the parent worktree. **Note:** this is the *millhouse*
  `.portals` junction — distinct from mhgo's **own** `internal/worktree`
  portals/launchers feature, which IS part of the codebase being renamed (see the
  `PortalTarget` clarification under Technical context).
- **The actual GitHub repo rename** (mhgo→loomyard) and the `mhgo-wiki-test`→
  `loomyard-test` rename are operator actions done outside this task; this task only
  updates the strings that point at them.
- **`docs/vendor/psmux_scripting.md`** — external vendored doc, contains no `mhgo`,
  left untouched.

## Decisions

### naming-map

- Decision: the canonical mapping is:

  | Anchor | From | To |
  |---|---|---|
  | Module path | `github.com/Knatte18/mhgo` | `github.com/Knatte18/loomyard` |
  | CLI command / `cmd/` dir | `mhgo` / `cmd/mhgo` | `lyx` / `cmd/lyx` |
  | Managed-state dir | `_mhgo/` | `_lyx/` |
  | Local-state dir | `.mhgo/` | `.lyx/` |
  | gitignore markers | `mhgo-managed` | `lyx-managed` |
  | Exported ident | `MhgoDir()` | `LyxDir()` |
  | Env-var prefix | `MHGO_` | `LYX_` |
  | `short_name` | `MHGO` | `LYX` |
  | Product name (prose) | mhgo | Loomyard |
  | Test email domain | `@mhgo.dev` | `@loomyard.dev` |
  | Integration test repo | `mhgo-wiki-test` | `loomyard-test` |

- Rationale: deliberate split mirroring millhouse precedent (`mill` → `_mill` /
  `MILL_`). The **repo/module** takes the product name `loomyard` (matches the
  already-renamed remote); everything **operational** (command, on-disk dirs,
  env-vars, identifiers, short_name) takes the short command name `lyx`, which is
  terse on disk and on the CLI.
- Rejected: (a) `loomyard` for the on-disk dirs/identifiers — operator rejected as
  too verbose; (b) `loom` — considered, operator chose `lyx`; (c) a blind global
  `mhgo`→`lyx` replace — would corrupt prose (e.g. "mhgo is a Go toolkit" must
  become "Loomyard is a Go toolkit", not "lyx is a Go toolkit") and would wrongly
  rewrite the external repo URL.

### prose-voice

- Decision: in documentation and comments, the **project/product** is written
  **"Loomyard"** (titles, "X is a Go toolkit", "X will replace mill/millhouse");
  the **CLI invocation** is written **`lyx`** in code font (`lyx board`,
  `usage: lyx <module>`, "concurrent `lyx` processes on a machine"). Module path
  references become `github.com/Knatte18/loomyard`.
- Rationale: the task is "Rename mhgo to Loomyard (lyx)" — Loomyard is the brand,
  `lyx` is how you invoke it. Prose must distinguish the two; a mechanical
  single-token replace cannot.
- Rejected: using `lyx` as the product name in prose (loses the brand); keeping
  `mhgo` anywhere in shipped docs.

### clean-break-no-compat

- Decision: no compatibility code for old `_mhgo/` / `.mhgo/` directory names.
- Rationale: operator confirmed there are no mhgo-managed repos outside this one,
  so nothing on disk needs migrating. A shim would add untested fallback code paths
  for zero benefit (YAGNI).
- Rejected: dual-read fallback (`_lyx/` then `_mhgo/`).

### external-test-repo

- Decision: rename `testRepoURL` to `https://github.com/Knatte18/loomyard-test.git`
  (drop the `-wiki` segment). The operator is renaming that GitHub repo themselves.
- Rationale: the integration test pushes to a real remote; the string must match
  the repo's new actual name. Operator confirmed the new name is `loomyard-test`.
- Rejected: leaving `mhgo-wiki-test` (would not match the renamed remote);
  `loomyard-wiki-test` (not the chosen name).

### harness-config-in-scope

- Decision: update `mill-config.yaml` `repo.short_name` `"MHGO"` → `"LYX"`.
- Rationale: it's the tracked source of the project's short label (drives the VS
  Code window-title prefix on future spawns); keeping `"MHGO"` leaves a stale brand
  in a tracked file.
- Rejected: leaving harness config untouched. Note `.vscode/settings.json` is
  gitignored and therefore not committed — out of scope for the commit even though
  Q4 answered "include harness config"; only the tracked `short_name` matters.

## Technical context

- **Language/build:** Go 1.26 module. Gate is `go build ./...` and `go test ./...`
  from the worktree root. Deps: `gofrs/flock`, `yaml.v3`.
- **Geometry owner — `internal/paths/paths.go`:** single source of worktree/
  container geometry. `Layout.MhgoDir()` returns `filepath.Join(Cwd, "_mhgo")` —
  rename to `LyxDir()` returning `"_lyx"`. `PortalTarget` also joins the literal
  `"_mhgo"` (line ~141) — this is the managed-state-dir name and IS in scope
  (`"_mhgo"`→`"_lyx"`); it is distinct from the portal *directory* names
  `_portals`/`_launchers` (`PortalsDir`/`LaunchersDir`), which contain no `mhgo`
  and are therefore unaffected. Package doc references `cmd/mhgo/main.go` as the
  os.Getwd allowlist exception.
- **Config — `internal/config/config.go`:** `FindBaseDir` and `Load` hardcode the
  literal `"_mhgo"` (lines ~32, ~70) and error text `_mhgo/ directory not found`.
- **Board init — `internal/board/init.go`:** `mhgoDir := filepath.Join(cwd, "_mhgo")`
  (~37); `gitignore.Ensure(cwd, ".mhgo/")` (~103); and the generated `board.yaml`
  comment block (~128–140) embeds the `$env:MHGO_*` example var names.
- **gitignore — `internal/gitignore/gitignore.go`:** `startMarker`/`endMarker`
  constants (`# === mhgo-managed ===`) + package doc. Many assertions in
  `gitignore_test.go` and one in `board/init_test.go` check the literal markers and
  `.mhgo/` entry.
- **IDE menu — `internal/ide/menu.go`:** joins literal `"_mhgo"` (~68).
- **muxpoc — `internal/muxpoc/state.go`:** `stateRelPath = ".mhgo/muxpoc-state.json"`,
  `lockRelPath = ".mhgo/muxpoc-state.lock"` (~25–26); a doc comment with the
  `C:\Code\mhgo\wts\mhgo-mux-design → muxpoc-mhgo-mux-design` example (~176). The
  derivation logic does NOT hardcode `mhgo` — only the example strings do.
- **Path-invariant enforcement — `internal/paths/enforcement_test.go`:** scans the
  whole tree and fails the build if raw `os.Getwd` or `git rev-parse
  --show-toplevel` appear outside `internal/paths` and the allowlisted
  `cmd/mhgo/main.go`. The allowlist literal `cmd/mhgo` (lines ~17, ~64, ~66) must
  become `cmd/lyx` in the same change that git-mv's the directory — for correctness
  and consistency. (Note: today `cmd/mhgo/main.go` contains no raw `os.Getwd` /
  `--show-toplevel`, so a stale allowlist entry would not by itself fail
  `enforcement_test`; the update is still required so the allowlist names the real
  path, and to stay safe if the CLI's `main.go` ever introduces such a primitive.)
  `CONSTRAINTS.md` documents this same invariant and names `cmd/mhgo/main.go`.
- **Paired test input/expected — `internal/muxpoc/state_test.go`:** cwd inputs like
  `C:\Code\mhgo\wts\mhgo-mux-design` feed a basename-derivation; if the example slug
  is renamed, the expected derived output in the same case must move with it.
- **Import-path count:** 80 `github.com/Knatte18/mhgo` import lines across the 10
  internal packages + `cmd`. A module-path replace of `github.com/Knatte18/mhgo` →
  `github.com/Knatte18/loomyard` covers `go.mod` line 1 and every import. The
  external `github.com/Knatte18/mhgo-wiki-test` must NOT be caught by that same
  replace (it maps to `loomyard-test`, not `loomyard-wiki-test`) — order/scope the
  replacements so the longer/external string is handled distinctly.

## Constraints

- **Path Invariant (CONSTRAINTS.md):** all cwd/worktree-root access goes through
  `internal/paths.Getwd()` / `Resolve()`. Raw `os.Getwd` and
  `git rev-parse --show-toplevel` are banned outside `internal/paths` and the CLI
  `main.go`. The ban is build-enforced by `enforcement_test.go`. The rename must
  keep this green: the allowlisted path becomes `cmd/lyx/main.go`. Do not introduce
  new raw primitives.
- **No behavioral drift:** the full test suite must pass with the same results as
  before the rename. Any test failure after the rename indicates a missed or
  inconsistent replacement, not a logic change to fix.
- **Worktree isolation:** work only inside this worktree
  (`C:\Code\loomyard\wts\rename-to-loomyard`); never edit the parent/hub worktree.

## Testing

- **Primary gate:** `go build ./...` then `go test ./...` from the worktree root —
  both must pass. This is the definitive verification that the rename is complete
  and consistent (stale imports fail to build; literal-string drift fails tests).
- **`internal/gitignore` (literal-assertion tests):** `gitignore_test.go` asserts
  the `mhgo-managed` markers and the `.mhgo/` entry — update expectations to
  `lyx-managed` and `.lyx/`. Same for the marker assertion in `board/init_test.go`.
- **`internal/config`:** `config_test.go` references `.mhgo` dir and the
  `NONEXISTENT_MHGO_TEST_VAR_XYZ` placeholder — update to `.lyx` / `_LYX_`.
- **`internal/board` + `boardtest`:** `init_test.go` checks the generated
  `board.yaml`/`.gitignore` content (markers, `_mhgo`/`.mhgo`); the `boardtest`
  integration/bench tests use `@mhgo.dev` emails and the `mhgo-wiki-test` URL —
  update to `@loomyard.dev` and `loomyard-test`.
- **`internal/muxpoc`:** `state_test.go` — update `.mhgo`→`.lyx` paths, and move
  example-slug input + expected derived output together; `muxpoc_smoke_test.go`
  references follow the same dir rename.
- **`internal/paths`:** `enforcement_test.go` allowlist must read `cmd/lyx`;
  `paths_test.go`/`worktreelist_test.go` `_mhgo`→`_lyx` and any `MhgoDir`→`LyxDir`
  call sites.
- No new test scenarios are required — this is rename-only. Do not add tests beyond
  updating the literals the rename invalidates.

## Q&A log

- **Q:** On-disk dir naming for `_mhgo/` and `.mhgo/`? **A:** `_lyx/` and `.lyx/`.
  Operator rejected `loomyard`; chose `lyx` over `loom`.
- **Q:** Env-var prefix for `MHGO_*`? **A:** `LYX_*`.
- **Q:** Backward compatibility with existing `_mhgo/`/`.mhgo/` dirs? **A:** None —
  clean break; no mhgo-managed repos exist outside this one.
- **Q:** Include the millhouse harness config (`mill-config.yaml short_name`,
  `.vscode` window title)? **A:** Yes — but `.vscode` is gitignored, so only the
  tracked `short_name` ("MHGO"→"LYX") is committed.
- **Q:** Rename `@mhgo.dev` test emails? **A:** Yes → `@loomyard.dev`.
- **Q:** The real external integration-test repo `mhgo-wiki-test`? **A:** Rename to
  `https://github.com/Knatte18/loomyard-test.git` — operator is renaming that repo
  on GitHub themselves (new name is `loomyard-test`, not `loomyard-wiki-test`).
- **Q:** `repo.short_name` value? **A:** `"LYX"` — used only where a short label is
  needed.
- **Q:** Module path target? **A:** `github.com/Knatte18/loomyard` — matches the
  already-renamed remote (not a question the operator had to decide; confirmed by
  the remote URL).
- **Q:** Prose voice? **A:** Product = "Loomyard" in prose; command = `lyx` in code
  font; module path = `github.com/Knatte18/loomyard`.
- **Q:** (review r1 gap) `cmd/mhgo/main_test.go` hardcodes `_mhgo`/`mhgoDir` — in
  scope? **A:** Yes — explicitly a rename site; git-mv moves the file but not its
  content, and the literals must become `_lyx` or the board-dispatch tests fail.
- **Q:** (review r1 gap) Root tracked `.gitignore` binary patterns `/mhgo`,
  `mhgo.exe`, `.mhgo/` — in scope? **A:** Yes → `/lyx`, `lyx.exe`, `.lyx/`. The
  `# === mill-managed ===` block in the same file stays untouched (harness).
