MILL_REVIEW_BEGIN
# Review: Rename mhgo to Loomyard (lyx) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-06-16
```

## Findings

### [BLOCKING] Stale `mhgo` in ~25 `.go` files — batch 1 incomplete

**Location:** See full list below — every file is in batch 1's scope.
**Issue:** The plan's own batch-1 verifier check states "after this batch, `grep -rI mhgo --include='*.go'` should return nothing." Twenty-five occurrences remain across board, muxpoc, and worktree packages; many are runtime-observable strings, not just comments.
**Fix:** Apply the naming-map and prose-voice rules to every occurrence listed below.

Key instances by file and nature:

`internal/board/config.go:78` — runtime error string still says `"mhgo init"` (plan: Card 5 + analogue of worktree/config.go which was correctly updated to `"lyx init"`).

`internal/board/cli.go:89` — usage string `"usage: mhgo board <subcommand> [json-payload]"` — emitted to stderr on bad input.

`internal/board/board.go:37,168` — doc comments reference `` `mhgo board sync` `` (the detached process the code actually spawns; comment misidentifies the binary name).

`internal/board/sync.go:8` — package doc says `` `mhgo board sync` ``.

`internal/board/spawn_other.go:3`, `spawn_windows.go:5,21` — comments still say `mhgo board sync` / `mhgo and git are console apps`.

`internal/board/boardtest/doc.go:1` — package doc says `mhgo's cross-cutting`.

`internal/board/boardtest/bench_test.go:28`, `concurrency_test.go:36,112,153` — comments refer to `_mhgo/board.yaml`. These are the `seedWiki` helper comments.

`internal/board/boardtest/bench_git_test.go:38` — git email `bench@mhgo.dev` (integration-gated; plan Card 5 requires `@loomyard.dev`).

`internal/board/boardtest/integration_test.go:41` — git email `test@mhgo.dev`.

`internal/muxpoc/cli.go:37` — doc comment `mhgo muxpoc <subcommand> [args...]`.

`internal/muxpoc/up.go:19` — comment says `'mhgo muxpoc up' subcommand`.

`internal/muxpoc/review.go:26,35` — **runtime error strings** returned to callers: `"run 'mhgo muxpoc up' first"`. These are user-visible JSON error messages.

`internal/muxpoc/state.go:94` — **runtime error string** `"mkdir .mhgo: %w"` — the code creates `.lyx/` but the error message names `.mhgo/`.

`internal/muxpoc/state.go:176` — doc comment example path `C:\Code\mhgo\wts\mhgo-mux-design`.

`internal/muxpoc/state_test.go:112,116` — test input CWD strings still `mhgo-mux-design` (plan Card 7 requires renaming to `loomyard-mux-design`).

`internal/worktree/worktree.go:1,4` — package doc still says `mhgo container` and `the mhgo`.

### [BLOCKING] Batch 2 docs not renamed — board.md, ide.md, mux.md

**Location:** `docs/modules/board.md:151-290`, `docs/modules/ide.md:94,97-98,124,139,141`, `docs/modules/mux.md` (throughout)
**Issue:** Batch 2's Card 9 requires all `mhgo` references in docs to be renamed. `board.md` still contains `_mhgo/`, `mhgo init`, `mhgo board sync`, `mhgo-managed`, `mhgo_dir` JSON key, and `cmd/mhgo`; `ide.md` still references `cmd/mhgo/main.go`, `_mhgo/`, `mhgo ide`, `mhgo-instantiated`, and `mhgo shell`; `mux.md` has extensive `mhgo mux`, `mhgo`, `\\.\pipe\mhgo`, and probe label `mhgoprobe` occurrences.
**Fix:** Apply Card 9's prose-voice rule to each file; `_mhgo/` → `_lyx/`, `mhgo init` → `lyx init`, `mhgo-managed` → `lyx-managed`, `cmd/mhgo` → `cmd/lyx`, product-prose → Loomyard, CLI invocations → `lyx`.

### [BLOCKING] `docs/benchmarks/board-performance.md:143` — wrong integration-test repo URL

**Location:** `docs/benchmarks/board-performance.md:143`
**Issue:** Still references `github.com/Knatte18/mhgo-wiki-test`; plan Card 9 explicitly requires this to become `github.com/Knatte18/loomyard-test`.
**Fix:** Change `mhgo-wiki-test` → `loomyard-test` on that line.

### [NIT] `internal/board/board.go` — stale doc cross-ref to `mhgo board sync`

**Location:** `internal/board/board.go:37,168`
**Issue:** Two comments describe the spawned detached process as `` `mhgo board sync` ``; the binary is now `lyx`. Not a test failure but misidentifies the process launched by `spawnSync`.
**Fix:** Change both occurrences to `` `lyx board sync` `` (covered by the BLOCKING fix above but called out as a runtime-observable descriptor for operators).

## Verdict

REQUEST_CHANGES
Batch 1 is incomplete: 25+ `mhgo` occurrences remain in `.go` files (including runtime error strings); Batch 2 docs are largely unedited.
MILL_REVIEW_END
