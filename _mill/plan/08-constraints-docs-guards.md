# Batch: constraints docs guards and comment sweep

```yaml
task: "Rename Cobra modules to `<module>cli`, extract kernels as `<module>engine`"
batch: "constraints docs guards and comment sweep"
number: 8
cards: 5
verify: "go build ./... && go test ./... && go test -tags integration ./..."
depends-on: [7]
```

## Batch Scope

The deliverable that makes the new convention durable and the docs/guards consistent with
the renamed tree. This batch: (1) rewrites the `CONSTRAINTS.md` CLI/Cobra package-naming
section to **invert** the convention and codify the cli/engine split as repo rules; (2)
retargets the `lyxtest` leaf-invariant guard's `bannedImports` to the new feature package
paths so it keeps guarding; (3) updates `docs/overview.md`; (4) corrects factual path /
command / clickable-link references in the other docs; (5) sweeps stale comment-only
references to the renamed packages. No production behaviour changes. By this batch every
`*cli`/`*engine` package exists, so the guard and doc references resolve correctly.

## Cards

### Card 24: Rewrite the CONSTRAINTS.md CLI/Cobra package-naming convention

- **Context:**
  - `internal/configengine/config.go`
  - `internal/clihelp/exec.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Replace the `### Package naming` subsection of the `## CLI / Cobra
  Invariant` (the current text says a command-owning package takes the command's bare name
  and treats `configcli`/`initcli` as the only exceptions). The new, inverted convention:
  **anything registered in `newRoot()` (i.e. anything that lands in Cobra) is named
  `<module>cli`; the domain kernel a non-CLI consumer needs is extracted as
  `<module>engine`** — cite the precedent `internal/yamlengine` and `internal/configengine`.
  Record, as first-class repo rules: (a) the **litmus** — returns `(T, error)` with no
  cobra / output `io.Writer` / exit codes → engine; exists only because of the command line
  → cli; (b) the **cli/engine boundary** — cli owns `Command()`, the `RunCLI` seam, Cobra
  subcommands, flags, `Short`/`Long`, `PersistentPreRunE`, exit-code handling; engine owns
  the domain kernel; (c) the **dependency direction** — cli imports engine, engine → engine
  is allowed (e.g. `ideengine` imports `boardengine`), engine must never import a `cli`
  package or cobra; (d) the **skip clause** — create an engine unless the logic is trivial/
  incidental (`initcli`, `configcli` — thin wrappers, no real kernel) or throwaway
  (`muxpoccli` — a POC slated for replacement); "no external consumer today" is NOT a skip
  reason (loom is the designed future consumer). Also: in the `## lyxtest Leaf Invariant`
  section update the feature-package examples (currently `board`, `worktree`, `weft`) to
  the new split names (`boardengine`/`boardcli`, `warpengine`/`warpcli`,
  `weftengine`/`weftcli`, etc.); and in `## CLI / Cobra Invariant` → `### For New Code`
  retarget the path reference `internal/warp/warp.go` to `internal/warpcli/warp.go` (the
  "warp variant" now lives in `warpcli`) and adjust the "board/weft variant" wording to
  `boardcli`/`weftcli`. The parent-group list (`board`, `warp`, `weft`, `ide`, `muxpoc`)
  uses command names, which are unchanged — leave those names as-is.
- **Commit:** `docs(constraints): invert package-naming convention, codify cli/engine split`

### Card 25: Retarget the lyxtest leaf-invariant bannedImports

- **Context:**
  - `internal/lyxtest/lyxtest.go`
  - `internal/lyxtest/doc.go`
- **Edits:**
  - `internal/lyxtest/leaf_enforcement_test.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `TestLeafInvariant` the `bannedImports` slice hardcodes the
  pre-rename feature import paths `internal/board`, `internal/warp`, `internal/weft`
  (alongside `internal/configreg`). Those paths no longer exist, so the guard silently
  stops protecting the feature packages while still passing. Replace the three stale
  feature entries with the new feature package import paths so the guard keeps catching a
  `lyxtest → configreg → feature` cycle: keep
  `github.com/Knatte18/loomyard/internal/configreg`, and add
  `.../internal/boardengine`, `.../internal/boardcli`, `.../internal/warpengine`,
  `.../internal/warpcli`, `.../internal/weftengine`, `.../internal/weftcli`,
  `.../internal/ideengine`, `.../internal/idecli`, `.../internal/ghissuesengine`,
  `.../internal/ghissuescli`, `.../internal/muxpoccli`. Update the file's top doc comment
  to name the new feature packages.
- **Commit:** `test(lyxtest): retarget leaf-invariant bannedImports to renamed packages`

### Card 26: Update docs/overview.md package map and command references

- **Context:**
  - `cmd/lyx/main.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update `docs/overview.md` to the renamed tree (use `cmd/lyx/main.go`'s
  final import/registration set as the authoritative module list): (1) the source-tree
  listing at lines ≈165–170 — `internal/board/` → `internal/boardcli/` +
  `internal/boardengine/`, `internal/warp/` → `internal/warpcli/` +
  `internal/warpengine/`, `internal/weft/` → `internal/weftcli/` +
  `internal/weftengine/`, `internal/ide/` → `internal/idecli/` + `internal/ideengine/`,
  `internal/muxpoc/` → `internal/muxpoccli/`, `internal/ghissues/` → `internal/ghissuescli/`
  + `internal/ghissuesengine/`, and remove the `internal/update` entry; (2) the module
  bullets at lines ≈205–206 — delete the standalone `**update**` bullet (it is now `lyx
  config reconcile`) and fold its description into the config/update story, and change
  `**board** … (internal/board)` to reference `internal/boardcli`/`internal/boardengine`;
  (3) line ≈111 — `reconciled via lyx update` → `reconciled via lyx config reconcile`
  (the config-module-name list "board, warp, weft" stays — those are config identifiers);
  (4) line ≈285 — `internal/board/boardtest` → `internal/boardengine/boardtest`; (5) line
  ≈33 — the "feature packages keep their own names" sentence now describes the
  `<module>cli`/`<module>engine` split; reword it and point to the `CONSTRAINTS.md`
  CLI/Cobra package-naming rule. Leave the principles/weft-contract/lifecycle prose
  otherwise intact.
- **Commit:** `docs(overview): update package map and command refs for the cli/engine split`

### Card 27: Correct path/command/link references in modules, sandbox, roadmap, benchmarks

- **Context:**
  - `cmd/lyx/main.go`
  - `docs/benchmarks/test-suite-timing.md`
- **Edits:**
  - `docs/modules/README.md`
  - `docs/modules/mux.md`
  - `docs/sandbox-hub.md`
  - `docs/roadmap.md`
  - `docs/benchmarks/board-performance.md`
  - `docs/benchmarks/running-tests.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Fix factual references to renamed packages (paths, runnable commands,
  clickable links) — do NOT rewrite historical measurement records. Specifically:
  `docs/modules/README.md` line ≈25 — the warp durable design now lives in the
  `internal/warpengine` package header (was `internal/warp`).
  `docs/modules/mux.md` line ≈9 — `internal/muxpoc` → `internal/muxpoccli`; line ≈188 —
  the illustrative `internal/board` naming-precedent reference is stale under the new
  convention; update it to remain accurate (reference the split, e.g. `internal/boardcli`
  /`internal/boardengine`, or note the convention is now `CONSTRAINTS.md`'s cli/engine
  rule).
  `docs/sandbox-hub.md` line ≈15 — `internal/warp/clone.go`'s `deriveHostName()` →
  `internal/warpengine/clone.go`'s exported `DeriveHostName()`; line ≈157 — the clickable
  link `[internal/warp/clone.go](../internal/warp/clone.go)` →
  `internal/warpengine/clone.go` (with matching link target).
  `docs/roadmap.md` line ≈189 — `internal/warp` package → `internal/warpengine`; line
  ≈202 — `internal/ghissues` package header → `internal/ghissuesengine` (path correction
  only, not a milestone change).
  `docs/benchmarks/board-performance.md` line ≈4 — the link
  `[internal/board/boardtest](../../internal/board/boardtest)` →
  `internal/boardengine/boardtest` (and link target); line ≈15 — the runnable
  `go test … ./internal/board/boardtest` → `./internal/boardengine/boardtest`.
  `docs/benchmarks/running-tests.md` line ≈41 and ≈94 — the runnable examples
  `go test ./internal/weft …` → `./internal/weftengine` and
  `go test ./internal/board/boardtest` → `./internal/boardengine/boardtest`. For
  `docs/benchmarks/test-suite-timing.md` (read-only Context): confirm it contains only
  historical timing-table cells and narrative (point-in-time records) with no runnable
  command or clickable link to a renamed package, and therefore needs no edit; leave the
  historical package-name cells as-is.
- **Commit:** `docs: correct renamed-package paths, commands, and links`

### Card 28: Comment-accuracy sweep for renamed packages

- **Context:**
  - `cmd/lyx/main.go`
- **Edits:**
  - `internal/lyxtest/doc.go`
  - `tools/sandbox/main.go`
  - `cmd/lyx/main_test.go`
  - `internal/paths/paths.go`
  - `cmd/testtiming/main.go`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Update comment-only references to the renamed packages for accuracy
  (non-functional, but they drift on rename): `internal/lyxtest/doc.go` line ≈2 —
  `internal/warp, internal/weft` → the new package names (`internal/warpengine`/
  `internal/warpcli`, `internal/weftengine`/`internal/weftcli`); `tools/sandbox/main.go`
  line ≈40 — `internal/warp/clone.go` → `internal/warpengine/clone.go`;
  `cmd/lyx/main_test.go` line ≈21 — the comment "behaviour … lives in internal/board" →
  `internal/boardcli`/`internal/boardengine`; `internal/paths/paths.go` line ≈344 — the
  comment "seeders in internal/warp" → `internal/warpengine`; `cmd/testtiming/main.go`
  lines ≈36 and ≈180 — the illustrative `internal/board` example in the `Package` field
  comment and the `shortPkg` doc comment → `internal/boardengine` (these are purely
  illustrative; `shortPkg`'s trimming logic is generic and has no functional package list,
  so only the comment text changes). Touch only comment text — no code, no behaviour.
- **Commit:** `docs: sweep stale comment references to renamed packages`

## Batch Tests

`verify` is repo-wide (Tier 1 + Tier 2). The only compiled change is
`internal/lyxtest/leaf_enforcement_test.go` (Tier 1): after the `bannedImports` retarget it
must still pass (lyxtest imports only stdlib + `internal/paths`, so no banned import is
present) while now guarding the renamed feature packages. The comment-sweep edits are
comment-only and are covered by `go build ./...`. CONSTRAINTS.md and the docs do not
compile; their correctness is review-enforced (the plan-reviewer checks the inverted
convention and the factual path/command/link fixes against the renamed tree).
