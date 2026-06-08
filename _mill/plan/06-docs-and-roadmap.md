# Batch: docs-and-roadmap

```yaml
task: "board-modul (rename fra wiki) + _mhgo-konfigurasjon"
batch: "docs-and-roadmap"
number: 6
cards: 3
verify: null
depends-on: [4, 5]
```

## Batch Scope

This batch brings the documentation in line with the final code: it renames
`docs/wiki.md` → `docs/board.md` and rewrites it to describe the `board` module
including the new config layer and `mhgo init`; updates `docs/overview.md` and
`docs/benchmarks.md` for the rename, the cwd config model, and the renamed env
vars; and creates `docs/roadmap.md` capturing the deferred work (the
discussion's "Out" list). It depends on batches 4 and 5 so the docs describe the
shipped behavior (cwd activation, `--board-path` spawn, `init`). Pure docs:
`verify: null` (no runnable surface). See discussion decision `roadmap-split`.

## Cards

### Card 21: rename and rewrite the module doc (wiki.md → board.md)

- **Context:**
  - `internal/board/cli.go`
  - `internal/board/board.go`
  - `internal/board/config.go`
  - `internal/board/init.go`
  - `internal/board/render.go`
  - `internal/board/store.go`
  - `internal/board/sync.go`
  - `internal/board/spawn_windows.go`
  - `internal/board/layer.go`
  - `internal/board/task.go`
- **Edits:** none
- **Creates:**
  - `docs/board.md`
- **Deletes:**
  - `docs/wiki.md`
- **Requirements:** Run `git mv docs/wiki.md docs/board.md`, then rewrite the
  content to describe the `board` module. Change the title to `# Module: board`,
  all `internal/wiki`→`internal/board`, `wiki.go`→`board.go`, `Wiki`→`Board`,
  `mhgo wiki ...`→`mhgo board ...`, `WIKI_SKIP_*`→`BOARD_SKIP_*`, the commit
  message `"wiki sync"`→`"board sync"`, and the internal-dependency-graph block.
  Update the `### cli.go` section to describe the cwd model (`os.Getwd()` +
  `LoadConfig(cwd, "board")`, error when `_mhgo/` is absent, the internal
  `--board-path` bypass for the detached sync child) and drop the old `--wiki-path
  → MHGO_WIKI_PATH → ../gowiki` precedence text. Update `### render.go` to note
  the configurable home/sidebar filenames and proposal prefix. Update `###
  wiki.go`(→ board.go) `writeOp` to mention the up-front `MkdirAll` before the
  write lock, and the read-method short-circuit on a missing board dir. Add a new
  `## Configuration` section documenting the layered model (built-in defaults <
  `_mhgo/board.yaml` < `.mhgo/board.yaml`, deep-merged per key, `$env:NAME`
  interpolation with hard error on unset, cwd-authoritative resolution, the
  `path`/`home`/`sidebar`/`proposal_prefix` keys and their defaults) and a `##
  init` section documenting `mhgo init` (creates `_mhgo/`, commented
  `board.yaml`, the `mhgo-managed` `.gitignore` block, JSON summary, idempotent;
  does not create the board dir). Update the `## Environment variables` table to
  `BOARD_SKIP_GIT`/`BOARD_SKIP_PUSH` and remove the `MHGO_WIKI_PATH` row. Update
  the `## Tests` section file list and the `[overview.md]`/`[benchmarks.md]`
  cross-links.
- **Commit:** `docs(board): rewrite module doc for board rename + config + init`

### Card 22: update overview.md and benchmarks.md

- **Context:**
  - `cmd/mhgo/main.go`
  - `internal/board/cli.go`
  - `internal/board/boardtest/bench_test.go`
  - `internal/board/boardtest/doc.go`
- **Edits:**
  - `docs/overview.md`
  - `docs/benchmarks.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `docs/overview.md`: update the `## Structure` tree
  (`internal/wiki/`→`internal/board/`, `wikitest/`→`boardtest/`,
  `cli.go wiki.go`→`cli.go board.go`), the "everything else is `package wiki`"
  line → `package board`, the `## Module dispatch` switch example
  (`case "wiki"`→`case "board"`, mention the new top-level `case "init"`), the
  `## Modules` list (`wiki`→`board`, add a line for `init` if appropriate), the
  `## Tests` paths, and the `## Other docs` links (`wiki.md`→`board.md`, add
  `roadmap.md`). In `docs/benchmarks.md`: retitle to the board module, change all
  `internal/wiki/wikitest`→`internal/board/boardtest` and `wiki`→`board` prose,
  update the `## How to run` commands to `BOARD_SKIP_GIT=1` and the
  `./internal/board/boardtest` path, and add a short note under the hot-path
  results that the CLI-driven benchmarks (`Upsert`/`Get`/`List`) were
  re-architected to the cwd model and now include the `os.Getwd()` +
  `LoadConfig` cost (do not invent new measured numbers — keep the historical
  result blocks, annotating that the CLI-bench harness changed). Keep the
  external test-repo URL `github.com/Knatte18/mhgo-wiki-test` unchanged.
- **Commit:** `docs: update overview and benchmarks for board rename + config`

### Card 23: create docs/roadmap.md

- **Context:**
  - `_mill/discussion.md`
  - `docs/overview.md`
- **Edits:** none
- **Creates:**
  - `docs/roadmap.md`
- **Deletes:** none
- **Requirements:** Create `docs/roadmap.md` as the living long-term-direction
  doc. Capture the deferred work from the discussion's "Out" scope as roadmap
  items (do NOT create task entries, and do not duplicate per-task discussion
  content): creating/cloning the board repo itself (the future growth of `init`,
  analogous to mill-setup Phases 1–3; today the board dir auto-creates on first
  write and the user makes it a git repo for push); seeding a `.mhgo/board.yaml`
  override stub / persisting machine-local overrides; a `verify`/doctor
  subcommand; future modules beyond `board` (e.g. a `mill` orchestrator) and
  Claude-Code-plugin packaging; and the explicitly out-of-scope millpy plumbing
  (junctions, hardlinks, portals, PYTHONPATH/venv, the wiki daemon, VS Code
  colours). Add a one-line note that later tasks maintain this file. Keep it
  concise and forward-looking.
- **Commit:** `docs: add roadmap for deferred board/init work`

## Batch Tests

`verify: null` — this batch only edits Markdown documentation and has no runnable
surface. Correctness is verified by review against the shipped code from batches
1–5.
