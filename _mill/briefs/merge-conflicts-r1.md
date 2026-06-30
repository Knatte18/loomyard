# Conflict Resolution Brief

Your sole job is to resolve git conflict markers in the listed files, stage each resolved file, and report success. Do NOT commit. Do NOT run `git merge --continue` — the SKILL does that after receiving `{"status":"success"}`.

## Task intent

These excerpts describe what THIS branch is trying to accomplish. When the merge introduces a parent-side change that conflicts with this branch's intent, the resolution preserves THIS branch's intent. In particular: if a file appears under a batch's `Deletes:` list and the merge introduces a modified version of that file from the parent, the resolution is to delete the file (your branch's intent overrides). Stage the deletion with `git -C C:\Code\loomyard\wts\rename-paths-to-hubgeometry rm <file>`.

### From discussion.md

# Discussion: Rename internal/paths → internal/hubgeometry

```yaml
task: Rename internal/paths to internal/hubgeometry
slug: rename-paths-to-hubgeometry
status: discussing
parent: main
```

## Problem

`internal/paths` is a generic-bucket name — the `utils`/`helpers`/`common`
antipattern. It names the *data type* (filesystem paths) instead of the
*responsibility*. The module actually owns the **geometry** of the hub topology:
the spatial relationships between hub, worktrees, weft siblings, portals, and
launchers. Two tells that the name lags the concept:

- Everyone — docs, reviewers, the model — already calls it "geometry". CONSTRAINTS.md
  literally reads "All worktree and hub **geometry** must be resolved through
  `internal/paths`", and the package doc comment opens "Package paths is the single
  owner of Loomyard worktree and container **geometry**."
- `paths` both *undersells* (it computes layout geometry, not mere strings) and
  *overclaims* (it does **not** own every path — filenames like `Home.md`, URLs, and
  user-supplied paths legitimately bypass it; the just-landed `harden-path-invariant`
  task drew that line precisely).

Renaming the package to `hubgeometry` makes the responsibility explicit and matches
the noun the whole codebase already uses informally. **Why now:** the dependency
`harden-path-invariant` has landed (commit `bac0e54`, "Harden the Path Invariant"),
so the behaviour + AST-enforcement churn on `paths.go` / `enforcement_test.go` /
the invariant section is settled. This rename now sits cleanly on top as a purely
mechanical, behaviour-preserving sweep — doing it before the hardening would have
forced rewriting every `paths`/"Path Invariant" reference twice.

## Scope

**Behaviour-preserving rename. No API surface change beyond the package qualifier.**

**In:**

- Rename the directory `internal/paths/` → `internal/hubgeometry/` (via `git mv` to
  preserve history).
- Rename the intra-package source files whose names carry the old word, via `git mv`:
  - `paths.go` → `hubgeometry.go`
  - `paths_test.go` → `hubgeometry_test.go`
  - `paths_unit_test.go` → `hubgeometry_unit_test.go`
  - The already-topic-named files **stay**: `geometry_test.go`, `weft_test.go`,
    `worktreelist.go`, `worktreelist_test.go`, `codeguide_guard_test.go`,
    `enforcement_test.go`.
- Change the package clause in **every** file in the directory. Note the package is
  **split across two package names** (verify the exact split before editing):
  - Production + same-package (white-box) tests declare `package paths` → become
    `package hubgeometry`: `paths.go` (→`hubgeometry.go`), `worktreelist.go`,
    `enforcement_test.go`, `codeguide_guard_test.go`.
  - External (black-box) test files declare `package paths_test` → become
    `package hubgeometry_test`: `paths_test.go` (→`hubgeometry_test.go`),
    `paths_unit_test.go` (→`hubgeometry_unit_test.go`), `geometry_test.go`,
    `weft_test.go`, `worktreelist_test.go`.
- Update the package doc comment in `hubgeometry.go` ("Package paths is the single
  owner…" → "Package hubgeometry is the single owner…").
- Update every importer: the import path
  `github.com/Knatte18/loomyard/internal/paths` → `…/internal/hubgeometry` and every
  `paths.` qualifier → `hubgeometry.` (71 `.go` files import it today — board, warp,
  weft, ide, config family, lyxtest, cmd/lyx, vscode, envsource, etc.; it is a leaf
  nearly everything depends on, so the importer set is large).
- Update the enforcement test (`enforcement_test.go`) hardcoded allowlist string
  literals `"internal/paths"` → `"internal/hubgeometry"` (two occurrences: lines ~69
  and ~347) and every `internal/paths` mention in its comments.
- Update any other string literals / comments in `.go` files that name `internal/paths`
  or `package paths`.
- **Update the codeguide-guard filename literal (load-bearing, easy to miss).**
  `internal/paths/codeguide_guard_test.go:48` skips the geometry file by a *filename*
  literal: `if d.Name() == "paths.go" { return nil }`. This skip is **filename-based,
  not package-name-based** — a plain `internal/paths`/`package paths` string sweep will
  not catch it. Because `paths.go` legitimately contains the `_codeguide` literal
  (lines 365/369, `WeftCodeguideDir`), after `git mv paths.go → hubgeometry.go` the
  skip stops matching, the guard's tree-scan reads `hubgeometry.go`, finds `_codeguide`,
  and `TestCodeguideGuard/tree-scan` fails. The literal **must** become `"hubgeometry.go"`
  (and the surrounding comment at lines 45-47 that names `paths.go` should be updated
  to `hubgeometry.go` for accuracy).
- Rename the **invariant** in `CONSTRAINTS.md`: "Path Invariant" → "Hub Geometry
  Invariant" (the `## Path Invariant` heading and every reference to it, including the
  lyxtest Leaf Invariant's "importing only the standard library and `internal/paths`"),
  and update its prose / helper references (`paths.Getwd()`, `paths.Resolve()`,
  `paths.BoardDir(...)`, `paths.LyxDirName`, `paths.ConfigDir(...)`,
  `paths.ConfigFile(...)`, `paths.WeftSiblingPath(...)`, `paths.HubPath(...)`, etc.)
  to the `hubgeometry.` qualifier, and `internal/paths` → `internal/hubgeometry`.
- Rename the doc `docs/shared-libs/paths.md` → `docs/shared-libs/hubgeometry.md` (via
  `git mv`) and update its contents (package name, qualifiers, title).
- **Comprehensive doc + project-instruction sweep** (operator decision): replace every
  stale `internal/paths` reference and every "Path Invariant" invariant-name reference
  across the repo with `internal/hubgeometry` / "Hub Geometry Invariant":
  - `CLAUDE.md` (project instructions — names "the **Path Invariant** (`internal/paths`
    owns all cwd/geometry and `_lyx`/config paths)").
  - `docs/overview.md`, `docs/shared-libs/README.md`, `docs/shared-libs/envsource.md`
    (line 5, dependency-direction line "`internal/envsource` imports `internal/paths`"),
    `docs/modules/loom.md`, `docs/modules/mux.md`, `docs/benchmarks/test-suite-timing.md`,
    `docs/roadmap.md`.
  - **Heading-anchor rename + dependent cross-doc link (load-bearing, easy to miss).**
    `docs/overview.md:64` is the section heading `## Path Invariants` (plural). Renaming
    it to `## Hub Geometry Invariants` changes its GitHub auto-anchor slug from
    `#path-invariants` to `#hub-geometry-invariants`. `docs/modules/loom.md:256` links to
    `[launcher geometry](../overview.md#path-invariants)`, so that link **must** be
    updated to `../overview.md#hub-geometry-invariants` in the same commit or it silently
    404s. Note the slug is lowercase/hyphenated, so the `"Path Invariant"` / `"internal/paths"`
    text greps will NOT catch it — it needs its own check (see Testing).

**Out:**

- Any change to geometry logic, the `Layout` type's fields/methods, resolution
  behaviour, or exported symbol names other than the package qualifier. Pure rename.
- The Path-Invariant *hardening* itself (BoardDir, leak fixes, AST enforcement) — that
  was the separate `harden-path-invariant` task, already landed.
- `_mill/status.md` and any `_mill/` mill-managed state, and the wiki task body — those
  are tooling state / board records, not source or docs; leave untouched.
- The proposal does **not** introduce a shorter `geometry` alias — see the naming
  decision below.

## Decisions

### package-name: hubgeometry (not geometry)

- Decision: rename to `hubgeometry`, not the shorter `geometry`.
- Rationale: `hubgeometry` is the more precise scope name and the model is hub-centric
  throughout (hub, worktrees, weft siblings, portals, launchers). It matches the noun
  the codebase already uses while being unambiguous about what geometry.
- Rejected: `geometry` — lighter qualifier at every call site, and `Layout` does resolve
  a plain repo too (not strictly hub-only), but the precision of `hubgeometry` wins.
  Settled in the proposal; not reopened.

### file-renames: rename the paths*.go files too

- Decision: `git mv` `paths.go`/`paths_test.go`/`paths_unit_test.go` to their
  `hubgeometry*` names; leave the already-topic-named files alone.
- Rationale: full consistency — no source file inside `internal/hubgeometry/` should
  carry the old bucket word. `git mv` preserves blame/history.
- Rejected: keep file names, rename only the dir + package clause — smaller diff, but
  leaves `paths.go` lagging the package name, which is exactly the smell being removed.

### scope: comprehensive repo-wide reference sweep

- Decision: update **every** file that names `internal/paths` or the "Path Invariant"
  invariant, including `CLAUDE.md` and all docs beyond the proposal's explicit list,
  plus code comments and string literals — all in this one commit.
- Rationale: a rename that leaves dangling old-name references in project instructions
  and module docs defeats the point (the name is the deliverable). One atomic commit
  keeps docs and code consistent, per the Documentation Lifecycle / Task-completion
  discipline in `CLAUDE.md`.
- Rejected: update only the proposal's listed files — would leave `CLAUDE.md`,
  `modules/loom.md`, `modules/mux.md`, `benchmarks/test-suite-timing.md`, and
  `roadmap.md` pointing at a directory that no longer exists.

### history-preserving moves: git mv, surgical edits

- Decision: use `git mv` for the directory and file renames; make surgical
  search-and-replace edits for qualifiers/imports — never full-file rewrites.
- Rationale: preserves history and keeps the diff reviewable as a pure rename.

## Technical context

What mill-plan needs to know:

- **Module path:** `github.com/Knatte18/loomyard` (from `go.mod`). The full import path
  to update is `github.com/Knatte18/loomyard/internal/paths` → `…/internal/hubgeometry`.
- **Importer set:** 71 `.go` files import `internal/paths`. They span: `cmd/lyx/*`,
  `internal/boardcli`, `internal/boardengine`, `internal/configcli`,
  `internal/configengine`, `internal/configsync`, `internal/envsource`,
  `internal/idecli`, `internal/ideengine`, `internal/initcli`, `internal/lyxtest`,
  `internal/muxpoccli`, `internal/vscode`, `internal/warpcli`, `internal/warpengine`,
  `internal/weft*`, and the package's own test files. Discover the exact, current set
  with: `grep -rl "internal/paths" --include="*.go" .`
- **Mechanical edit shape per importer:** (1) the import line `…/internal/paths` →
  `…/internal/hubgeometry`; (2) every `paths.` identifier qualifier → `hubgeometry.`.
  Because `paths` is also a plausible local variable name, prefer replacing the
  qualifier only where it is the package selector. In practice a token-aware
  replace of `paths.` (followed by an exported identifier) → `hubgeometry.` is safe;
  a `gofmt`/`goimports` pass after editing tidies import grouping. Verify with
  `go vet ./...` that no stray `paths.` selector remains.
- **Hardcoded allowlist strings (critical):** `internal/paths/enforcement_test.go`
  compares `pkgDir == "internal/paths"` (the raw-primitive allowlist, ~line 69) and
  `filepath.ToSlash(filepath.Dir(relPath)) == "internal/paths"` (the geometry-literal
  allowlist, ~line 347). Both string literals **must** become `"internal/hubgeometry"`
  or the enforcement tests will either stop allowing the package its legitimate
  primitives/literals (false failures) or scan the renamed dir as a violator. Also the
  enforcement-test comments mention `internal/paths` ("Two levels up from
  internal/paths/enforcement_test.go", "Allowlist: internal/paths is the sole permitted
  owner", etc.) — update those for accuracy.
- **`cmd/lyx/main.go` allowlist:** the raw-`os.Getwd`/`git rev-parse` allowlist also
  permits `cmd/lyx/main.go`; that entry is unaffected by the rename (no change needed).
- **Doc/instruction references to find:** `internal/paths` appears in `CLAUDE.md`,
  `CONSTRAINTS.md`, `docs/overview.md`, `docs/shared-libs/paths.md`,
  `docs/shared-libs/README.md`, `docs/shared-libs/envsource.md`, `docs/modules/loom.md`,
  `docs/modules/mux.md`, `docs/benchmarks/test-suite-timing.md`, `docs/roadmap.md`.
  (`docs/shared-libs/envsource.md:5` names the dependency-direction line
  "`internal/envsource` imports `internal/paths`".) The invariant name
  "Path Invariant" appears in `CLAUDE.md`, `CONSTRAINTS.md`, `docs/overview.md`.
  Re-discover with `grep -rln "internal/paths" .` and `grep -rln "Path Invariant" .`
  before finishing; exclude `_mill/` from edits.
- **Doc rename:** `docs/shared-libs/paths.md` → `docs/shared-libs/hubgeometry.md` (the
  README and overview link to it, so update those links too). `roadmap.md` per
  `CLAUDE.md` is milestone-only, but a stale module-path reference is a correctness fix,
  not a new note — update the reference in place, don't append roadmap commentary.
- **Package clause is split across two names:** `paths.go`/`worktreelist.go` and the
  white-box tests `enforcement_test.go`/`codeguide_guard_test.go` declare `package paths`
  (→ `package hubgeometry`); the black-box test files `paths_test.go`,
  `paths_unit_test.go`, `geometry_test.go`, `weft_test.go`, `worktreelist_test.go`
  declare `package paths_test` (→ `package hubgeometry_test`). Do not assume a single
  package name — confirm each file's clause.

## Constraints

From `CONSTRAINTS.md` (this task edits the constraint text itself, so treat carefully):

- **Hub Geometry Invariant** (currently "Path Invariant"): `internal/paths` (→
  `internal/hubgeometry`) remains the sole owner of cwd/geometry and `_lyx`/config-path
  resolution. The rename must not move any logic out of the package or change the
  enforced rules — only the package name and the invariant's *name*. After the rename
  the geometry-literal ban's allowlist and the raw-primitive allowlist must still point
  at the (renamed) sole package, so `enforcement_test.go` must pass with the updated
  string literals.
- **lyxtest Leaf Invariant:** `internal/lyxtest` must keep importing only the standard
  library and `internal/paths` (→ `internal/hubgeometry`). The rename updates the
  import; the leaf property is preserved. `internal/lyxtest/leaf_enforcement_test.go`
  must still pass.
- **CLI / Cobra Invariant:** untouched by this rename — no command, `Short`/`Long`, or
  registration changes. The help-tree / drift / registration / longlist guards should
  stay green unchanged.
- **Documentation Lifecycle / Task completion** (`CLAUDE.md`): docs are updated in the
  **same commit** — that is the whole comprehensive-sweep decision above. Record the
  invariant rename in `CONSTRAINTS.md` in this commit.
- **`git mv` for renames** (project convention): directory and file moves use `git mv`;
  edits are surgical, never full-file rewrites.

## Testing

This is mechanical and behaviour-preserving; the existing suite is the guardrail. No
new tests are required.

- **Build:** `go build ./...` must pass — catches any missed import path or qualifier.
- **Vet:** `go vet ./...` — catches stray/dangling `paths.` selectors and import issues.
- **Full suite:** `go test ./...` must pass. The load-bearing guards to watch:
  - `internal/hubgeometry/enforcement_test.go` — `TestEnforcement_*` (raw-primitive ban)
    and `TestEnforcement_GeometryLiterals` (geometry-literal ban). These fail loudly if
    the allowlist string literals weren't updated to `"internal/hubgeometry"`, or if any
    importer accidentally acquired a banned primitive/literal. This is the single most
    likely thing to break and the primary signal the rename is correct.
  - `internal/hubgeometry/codeguide_guard_test.go` — `TestCodeguideGuard/tree-scan` skips
    the geometry file by the **filename** literal `"paths.go"` (line 48), which must be
    changed to `"hubgeometry.go"` (see the Scope bullet above). If left unchanged this
    test fails after the file rename — it is the second most likely break after the
    enforcement allowlist.
  - `internal/lyxtest/leaf_enforcement_test.go` — confirms lyxtest's import set is still
    {stdlib, internal/hubgeometry}.
  - `cmd/lyx/*` guards (`drift_test.go`, `helptree_test.go`, `registration_test.go`,
    `longlist_test.go`) — expected to stay green untouched; if any references the old
    package name in a string, update it.
- **Formatting:** run `gofmt`/`goimports` so import blocks are correctly grouped after
  the path change.
- **No-dangling-reference check:** after edits, each of these (excluding `_mill/`) must
  return nothing:
  - `grep -rn "internal/paths" .`
  - `grep -rn "Path Invariant" .`
  - `grep -rn "package paths(_test)?\b" .` — matches both the white-box (`package paths`)
    and black-box (`package paths_test`) clauses; a plain `package paths\b` misses
    `package paths_test` because `_` is a word character.
  - `grep -rn "#path-invariant" .` — the lowercase heading-anchor slug; catches the
    `docs/modules/loom.md` link if its `#path-invariants` fragment was missed (the
    text greps above do not match the hyphenated lowercase fragment).
  - `grep -rn '"paths.go"' .` — the codeguide-guard filename literal; the package-name
    greps above do not match it.

## Q&A log

- **Q:** Does the `harden-path-invariant` dependency need to land first? **A:** It already
  has — commit `bac0e54` ("Harden the Path Invariant"); CONSTRAINTS.md still reads "Path
  Invariant"/`internal/paths`, so this rename is a clean base on top.
- **Q:** Rename the intra-package source files (`paths.go`, `paths_test.go`,
  `paths_unit_test.go`) too, or just the directory + package clause? **A:** Rename them
  too, via `git mv`; the already-topic-named files (`geometry_test.go`, `weft_test.go`,
  `worktreelist*.go`, `codeguide_guard_test.go`, `enforcement_test.go`) stay.
- **Q:** Update `CLAUDE.md` even though the proposal's scope list didn't name it? **A:**
  Yes — update `CLAUDE.md` and all other files that refer to `internal/paths` / "Path
  Invariant" the way `CLAUDE.md` does (comprehensive sweep).
- **Q:** Update only the proposal's listed docs, or every stale `internal/paths` doc
  reference? **A:** Update all doc references repo-wide (excludes `_mill/status.md`,
  which is mill-managed state).
- **Q:** Introduce a shorter `geometry` alias? **A:** No — `hubgeometry` only; settled in
  the proposal for precision/hub-centricity.


### From _mill/plan/00-overview.md


```yaml
task: "Rename internal/paths to internal/hubgeometry"
slug: rename-paths-to-hubgeometry
approved: true
started: 20260630-161302
parent: main
root: ""
verify: null
```

### From _mill/plan/01-code-rename.md


```yaml
task: "Rename internal/paths to internal/hubgeometry"
batch: "code-rename"
number: 1
cards: 8
verify: go build ./... && go test ./... && go vet -tags integration ./...
depends-on: []
```



- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `cmd/lyx/exitcode_test.go`
  - `cmd/lyx/main_test.go`
  - `cmd/lyx/registration_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/boardcli/cli.go`
  - `internal/boardcli/cli_test.go`
  - `internal/boardengine/boardtest/bench_test.go`
  - `internal/boardengine/config_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/configcli/configcli.go`
  - `internal/configcli/configcli_integration_test.go`
  - `internal/configcli/configcli_test.go`
  - `internal/configcli/menu.go`
  - `internal/configcli/reconcile_test.go`
  - `internal/configengine/config.go`
  - `internal/configengine/config_test.go`
  - `internal/configengine/edit.go`
  - `internal/configengine/edit_test.go`
  - `internal/configsync/configsync.go`
  - `internal/configsync/configsync_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/idecli/cli.go`
  - `internal/ideengine/menu.go`
  - `internal/ideengine/menu_test.go`
  - `internal/ideengine/spawn.go`
  - `internal/ideengine/spawn_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/warpcli/clone.go`
  - `internal/warpcli/warp.go`
  - `internal/warpcli/warp_test.go`
  - `internal/warpengine/add.go`
  - `internal/warpengine/checkout.go`
  - `internal/warpengine/cleanup.go`
  - `internal/warpengine/clone.go`
  - `internal/warpengine/clone_integration_test.go`
  - `internal/warpengine/config_test.go`
  - `internal/warpengine/drift.go`
  - `internal/warpengine/drift_test.go`
  - `internal/warpengine/hook.go`
  - `internal/warpengine/hook_test.go`
  - `internal/warpengine/junction.go`
  - `internal/warpengine/launchers.go`
  - `internal/warpengine/launchers_test.go`
  - `internal/warpengine/list.go`
  - `internal/warpengine/portals.go`
  - `internal/warpengine/portals_test.go`
  - `internal/warpengine/prune.go`
  - `internal/warpengine/reconcile.go`
  - `internal/warpengine/reconcile_test.go`
  - `internal/warpengine/remove.go`
  - `internal/warpengine/remove_test.go`
  - `internal/warpengine/status.go`
  - `internal/warpengine/status_test.go`
  - `internal/warpengine/weftwiring.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/weftcli/cli.go`
  - `internal/weftcli/cli_test.go`
  - `internal/weftengine/config_test.go`
  - `internal/weftengine/weft_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `internal/lyxtest/doc.go`
  - `internal/lyxtest/leaf_enforcement_test.go`
  - `internal/lyxtest/lyxtest.go`
  - `internal/lyxtest/lyxtest_test.go`
  - `internal/vscode/color.go`
  - `internal/vscode/color_test.go`
  - `internal/envsource/envsource.go`
  - `internal/initcli/initcli.go`
  - `internal/initcli/initcli_test.go`
  - `internal/muxpoccli/cli.go`
- **Edits:**
  - `internal/boardengine/config.go`
  - `internal/boardengine/template_test.go`
  - `internal/warpcli/clone_cli_test.go`
  - `internal/idecli/cli_test.go`
  - `cmd/lyx/unknown_subcommand_test.go`
  - `internal/muxpoccli/muxpoc_smoke_test.go`
- **Creates:** none
- **Deletes:** none

### From _mill/plan/02-docs.md


```yaml
task: "Rename internal/paths to internal/hubgeometry"
batch: "docs"
number: 2
cards: 3
verify: null
depends-on: [1]
```



- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `docs/overview.md`
  - `docs/shared-libs/README.md`
  - `docs/shared-libs/envsource.md`
  - `docs/shared-libs/configengine.md`
  - `docs/modules/loom.md`
  - `docs/modules/mux.md`
  - `docs/benchmarks/test-suite-timing.md`
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Edits:**
  - `CLAUDE.md`
- **Creates:** none
- **Deletes:** none

## Conflicting files

- `docs/overview.md`

## Instructions

For each file listed above:

1. Read the file and locate every conflict block (`<<<<<<<`, `=======`, `>>>>>>>`).
2. Understand both sides of the conflict — what each branch intended.
3. Write a resolution that preserves the intent of both sides. When both sides modify **different, non-overlapping parts** of the same conflict region — for example, different columns of one table row, different keys of one object, or disjoint lines of a prose block — **combine both edits** into a single resolved structure. Do NOT pick one side wholesale just because the region overlaps syntactically; picking one side wholesale is correct only when the two changes are genuinely mutually exclusive (e.g. the same key is renamed to two different values). Worked example: if `ours` changes column A and `theirs` changes column B of the same table row, the resolution keeps both column changes in a single row — it does not discard either.
4. Run `git -C C:\Code\loomyard\wts\rename-paths-to-hubgeometry add <file>` to stage the resolved file.
5. For modify/delete (DU) conflicts: if Task intent above lists this file under a batch's `Deletes:`, run `git -C C:\Code\loomyard\wts\rename-paths-to-hubgeometry rm <file>` instead of editing; that stages the intentional deletion.
6. For UD conflicts — files this branch **modified** that the parent branch **deleted**: do not silently keep the modification. Instead:
   a. Run `git log --diff-filter=D --oneline MERGE_HEAD -- <file>` to find the deletion commit on the parent.
   b. Run `git show <deletion-commit>` to inspect context.
   c. If the deletion commit message mentions a replacement file (e.g. "replaced by", "moved to", "consolidated into"), or the commit also adds a file in the same directory with overlapping content: stage the deletion — `git -C C:\Code\loomyard\wts\rename-paths-to-hubgeometry rm <file>`.
   d. If detection is inconclusive: report `{"status":"stuck","stuck_type":"logic","reason":"modify/delete conflict on <file>: cannot determine if parent deletion is a replacement -- operator must decide"}` and halt. Do NOT silently keep the modification.

Never use `git checkout --ours` or `git checkout --theirs` — they silently discard one side of the conflict.

## Report

Your last output line MUST be a bare JSON object (no code fence, no backticks):

On success (nothing discarded):

{"status":"success"}

On success with discarded content — if you had to drop content from one side (e.g. two sides made mutually exclusive changes and only one could survive), list each dropped item:

{"status":"success","discarded":["<short description of what was dropped from which side>"]}

An empty or absent `discarded` field means nothing was lost. If anything was discarded, you MUST list it; an empty list when content was actually dropped is a protocol violation. The `mill-merge-in` frontend reads this field and surfaces any losses to the operator before continuing, rather than silently running `git merge --continue`.

If you cannot resolve one or more conflicts:

{"status":"stuck","stuck_type":"logic","reason":"<one-line description of what you could not resolve>"}

Anything other than this JSON object on the last line is a protocol violation; the merge-in dispatcher treats that as stuck_type: logic with reason "no structured report" — your work is lost. Do not wrap the JSON in a code fence; do not add commentary after it.

## Tools

Available: Read, Edit, Write, Bash, Grep, Glob. Use `git -C C:\Code\loomyard\wts\rename-paths-to-hubgeometry` for any git commands; do not `cd`. Worktree cwd is `C:\Code\loomyard\wts\rename-paths-to-hubgeometry`.
