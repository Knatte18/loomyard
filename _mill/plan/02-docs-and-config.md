# Batch: docs-and-config

```yaml
task: "Rename mhgo to Loomyard (lyx)"
batch: docs-and-config
number: 2
cards: 2
verify: null
depends-on: [1]
```

## Batch Scope

This batch sweeps the remaining non-code `mhgo` references: all `.md`
documentation under `docs/`, the root `CONSTRAINTS.md`, and the tracked
`mill-config.yaml` `short_name` field. It is separated from batch 1 because none
of it affects `go build`/`go test`, and combining it would push batch 1 over the
context budget. It depends on batch 1 so the documented module path, command name,
and `cmd/lyx` directory already exist when the prose is updated.

Apply the `prose-voice` decision throughout: product/project → **Loomyard**,
CLI invocation → **`lyx`** (code font), module path → `github.com/Knatte18/loomyard`.

**Validator note (`wiki-config-mutation`):** Card 10 edits `mill-config.yaml`. This
is safe mid-flight because `repo.short_name` is a **display-only field** — it feeds
the VS Code window-title prefix used by `mill-spawn`/`mill-color` on *future*
spawns and is never read by `mill-go`, `mill-plan`, or any code executing the
current task. No consuming code is added or changed by this plan. The plan reviewer
should clear the `wiki-config-mutation` check via this justification
(condition (a): documented-safe-mid-flight bootstrap rationale).

## Cards

### Card 9: Documentation brand sweep

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/overview.md`
  - `docs/roadmap.md`
  - `docs/psmux-tui-behavior.md`
  - `docs/benchmarks/board-performance.md`
  - `docs/benchmarks/test-suite-timing.md`
  - `docs/modules/board.md`
  - `docs/modules/ide.md`
  - `docs/modules/mux.md`
  - `docs/modules/mux-exploration.md`
  - `docs/modules/mux-hooks-exploration.md`
  - `docs/modules/mux-proposal.md`
  - `docs/modules/muxpoc.md`
  - `docs/modules/worktree.md`
  - `docs/shared-libs/README.md`
  - `docs/shared-libs/config.md`
  - `docs/shared-libs/gitignore.md`
  - `docs/shared-libs/lock.md`
  - `docs/shared-libs/paths.md`
  - `docs/shared-libs/state.md`
- **Creates:** none
- **Deletes:** none
- **Requirements:** Rewrite every `mhgo` reference per the `prose-voice` rule. Specifically: the project name in prose → **Loomyard** (e.g. `# Overview: mhgo` → `# Overview: Loomyard`, "`mhgo` is a Go toolkit" → "Loomyard is a Go toolkit", "mhgo is intended to replace mill/millhouse" → "Loomyard is intended to replace mill/millhouse", "Roadmap: mhgo" → "Roadmap: Loomyard"); the CLI command in code font → `lyx` (e.g. "concurrent `mhgo` processes" → "concurrent `lyx` processes"); the module path `github.com/Knatte18/mhgo` → `github.com/Knatte18/loomyard`. Rename on-disk dir references in docs `_mhgo`→`_lyx` and `.mhgo`→`.lyx`; gitignore marker mentions `mhgo-managed`→`lyx-managed`; env-var examples `MHGO_*`→`LYX_*`; command-path references `cmd/mhgo`→`cmd/lyx`. In `docs/shared-libs/config.md`, the `$env:MHGO_HOME`/`$env:MHGO_BOARD`/`$env:MHGO_CODE_REVIEWER` examples → `LYX_*`. In `docs/benchmarks/board-performance.md`, change `github.com/Knatte18/mhgo-wiki-test` → `github.com/Knatte18/loomyard-test`. In the mux exploration docs, rename illustrative psmux probe labels `mhgoprobe`→`lyxprobe` and `mhgohookprobe`→`lyxhookprobe`, and example slugs/paths `mhgo-mux-design`→`loomyard-mux-design`, `C:\Code\mhgo\`→`C:\Code\loomyard\`, `mhgo-instantiated`→`lyx-instantiated`. In `docs/benchmarks/test-suite-timing.md`, update the referenced Go test-function name `TestMenuRequiresMhgoDir` → `TestMenuRequiresLyxDir` to match the rename in batch 1 card 6.
- **Commit:** `docs: rebrand mhgo to Loomyard/lyx across documentation`

### Card 10: Constraints doc and mill-config short_name

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `CONSTRAINTS.md`
  - `mill-config.yaml`
- **Creates:** none
- **Deletes:** none
- **Requirements:** In `CONSTRAINTS.md`, update the path-invariant references `cmd/mhgo/main.go` → `cmd/lyx/main.go` (the two bullets naming the allowlisted `os.Getwd` / `git rev-parse --show-toplevel` exception). In `mill-config.yaml`, change `repo.short_name: "MHGO"` → `"LYX"`; do not change any other key. Apply prose-voice to any other `mhgo` mention in `CONSTRAINTS.md` (none expected beyond the cmd path).
- **Commit:** `docs: update CONSTRAINTS cmd path and mill-config short_name to lyx`

## Batch Tests

`verify: null` — this batch touches only Markdown documentation and a
display-only config field (`mill-config.yaml repo.short_name`). There is no
runnable code surface: no `.go` file changes, so `go build`/`go test` results are
unchanged from batch 1. Verification is by reviewer inspection. Reviewer check:
after this batch, `grep -rI mhgo .` (excluding `_mill/`, `.git/`, `.millhouse/`,
`docs/vendor/`, and `.vscode/`) should return nothing — every tracked `mhgo`
reference is renamed by batches 1 and 2.
