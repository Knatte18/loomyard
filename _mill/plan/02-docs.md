# Batch: docs

```yaml
task: Rename internal/ghissues → selfreport
batch: docs
number: 2
cards: 2
verify: null
depends-on: [1]
```

## Batch Scope

This batch updates the prose documentation to match the renamed module: the module table
and bullet in `docs/overview.md`, the historical milestone entry in `docs/roadmap.md`, and
the sandbox docs (`docs/sandbox-howto.md`, `docs/sandbox-hub.md`, `tools/sandbox/test-scheme.md`).
It also carries the operator's `drop-mill-pipeline-refs` decision: every
`mill-ghissues-to-tasks` mention is removed (not renamed) and the surrounding prose is
reworded to stand without naming that millhouse pipeline. There is no runnable surface, so
`verify: null`. It depends on batch 1 only for ordering — the docs describe the
already-renamed `lyx selfreport create` command.

## Cards

### Card 5: Update overview.md module table and roadmap milestone

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/overview.md`
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - `docs/overview.md`: in the source-tree block change `internal/ghissuescli/         the
    ghissues CLI command` → `internal/selfreportcli/        the selfreport CLI command`
    and `internal/ghissuesengine/      the ghissues domain kernel` →
    `internal/selfreportengine/     the selfreport domain kernel` (keep the column
    alignment consistent with surrounding rows). In the module-list section, rename the
    `**ghissues**` bullet heading to `**selfreport**` and change the
    `(lyx ghissues create <title>)` example to `(lyx selfreport create <title>)`; keep the
    rest of the bullet (hardcoded target, `--body`/`-`/`--label` description) intact.
  - `docs/roadmap.md`: in the "✅ Done" ghissues milestone entry, change the
    `lyx ghissues create <title>` example to `lyx selfreport create <title>` and the
    `internal/ghissuesengine` package reference to `internal/selfreportengine`. Keep the
    historical milestone framing (it stays a Done entry; do not add or restructure
    milestones — per the roadmap discipline in `CLAUDE.md`). The milestone's leading
    `**`ghissues` — …**` label may be updated to `**`selfreport` — …**` for consistency.
- **Commit:** `docs(selfreport): update overview module table and roadmap milestone`

### Card 6: Rename command refs and drop mill-ghissues-to-tasks in sandbox docs

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/sandbox-howto.md`
  - `docs/sandbox-hub.md`
  - `tools/sandbox/test-scheme.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In all three files, change every `lyx ghissues create` → `lyx selfreport create`
    (includes `tools/sandbox/test-scheme.md` lines for the command, its `--help` discovery,
    and the per-finding filing step; and `docs/sandbox-hub.md` the gh-prerequisite line).
  - Remove every `mill-ghissues-to-tasks` reference and reword for grammar so each sentence
    stands without naming the mill pipeline (do NOT invent `mill-selfreport-to-tasks`):
    - `docs/sandbox-howto.md` (~line 17-18): drop the "which feed the `GitHub issue →
      mill-ghissues-to-tasks` pipeline" clause — reword to end the sentence cleanly at the
      `lyx selfreport create` filing action.
    - `docs/sandbox-howto.md` (~line 103): drop the "with the mill pipeline
      (`/mill-ghissues-to-tasks`)" clause — reword so the sentence about feeding the backlog
      stands without naming the mill skill.
    - `docs/sandbox-hub.md` (~line 111-112): drop the "which feeds the `GitHub issue ->
      mill-ghissues-to-tasks` pipeline" clause — reword to end cleanly at the
      `lyx selfreport create` filing action.
  - Read each affected passage in full and reword for coherent grammar, not just delete the
    token. After this card, a tree-wide grep for `ghissues` (case-insensitive) must return
    zero hits outside `_mill/`.
- **Commit:** `docs(selfreport): rename command refs and drop mill pipeline mentions`

## Batch Tests

`verify: null` — this is a pure-documentation batch with no runnable surface. Correctness is
confirmed by review plus the tree-wide `ghissues` grep called out in Card 6 (zero hits
outside `_mill/` after the rename).
