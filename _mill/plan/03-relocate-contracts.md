# Batch: relocate-contracts

```yaml
task: 'loom: pin the spawn/handover status schema + discussion-format.md'
batch: relocate-contracts
number: 3
cards: 2
verify: 'bash -c "test -f docs/reference/plan-format.md && test -f docs/reference/builder-contract.md && test ! -e docs/modules/plan-format.md && test ! -e docs/modules/builder-contract.md"'
depends-on: []
```

## Rename mechanic

For each `Moves:` pair the implementer MUST:

1. Run `git mv <old> <new>` FIRST, before making any other change to the moved file.
2. Make ONLY surgical edits — touch only the relative links inside the moved file that must
   change after the move (per Decision `retarget-mapping` in the overview): a link that was
   `../reference/<x>.md` becomes `<x>.md` (now a sibling in `docs/reference/`); a link to a doc
   that stays in `docs/modules/` (e.g. `loom.md`, `hardener.md`) becomes `../modules/<name>.md`;
   `../overview.md` and the mutual `plan-format.md` ↔ `builder-contract.md` sibling link are
   unchanged. No prose or content edits.
3. Use a full-file `Creates:` entry only for genuinely new files — not applicable here.
4. Never write the relocated file from scratch and delete the original — that breaks git rename
   history and inflates the diff.

## Batch Scope

Relocate the two cross-module contract docs out of the delete-on-landing `docs/modules/` folder
into the durable `docs/reference/` folder (discussion `docs-are-contracts-not-modules`), fixing
each file's own internal relative links in the same surgical move. All *inbound* references from
other files are handled by batches 4 (non-narrative) and 5 (narrative); this batch owns only the
`git mv` and the two moved files' internal links. Batches 4 and 5 depend on this batch so the
files exist at their new path before inbound links are retargeted.

## Cards

### Card 3: git mv plan-format.md to docs/reference/ and fix its internal links

- **Context:**
  - `docs/overview.md`
  - `docs/reference/model-spec.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `docs/modules/plan-format.md` -> `docs/reference/plan-format.md`
- **Requirements:** `git mv docs/modules/plan-format.md docs/reference/plan-format.md` FIRST.
  Then read the moved file and fix only its internal relative links per Decision
  `retarget-mapping`: any `[...](../reference/model-spec.md)` becomes `[...](model-spec.md)` (now
  a sibling); `[...](../overview.md)` links are unchanged (both `docs/modules/` and
  `docs/reference/` are one level under `docs/`); any link to a doc that stays in `docs/modules/`
  (e.g. `loom.md`, `hardener.md`, `README.md`), if present, becomes `../modules/<name>.md`; the
  sibling `builder-contract.md` link (if present), unchanged. Do not touch prose or any
  `#anchor` fragments. Make no other change to the file.
- **Commit:** `docs(plan-format): relocate to docs/reference and fix internal links`

### Card 4: git mv builder-contract.md to docs/reference/ and fix its internal links

- **Context:**
  - `docs/modules/loom.md`
  - `docs/overview.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:**
  - `docs/modules/builder-contract.md` -> `docs/reference/builder-contract.md`
- **Requirements:** `git mv docs/modules/builder-contract.md docs/reference/builder-contract.md`
  FIRST. Then read the moved file and fix only its internal relative links per Decision
  `retarget-mapping`: every `[loom.md](loom.md)` (and any other same-folder link to a doc staying
  in `docs/modules/`, e.g. `hardener.md`) becomes `[loom.md](../modules/loom.md)`; the sibling
  `[plan-format.md](plan-format.md)` link is unchanged (both now in `docs/reference/`);
  `[...](../overview.md)` unchanged; any `[...](../reference/model-spec.md)` becomes
  `[...](model-spec.md)`. Do not touch prose or `#anchor` fragments. Make no other change.
- **Commit:** `docs(builder-contract): relocate to docs/reference and fix internal links`

## Batch Tests

`verify: 'bash -c "test -f docs/reference/plan-format.md && test -f docs/reference/builder-contract.md && test ! -e docs/modules/plan-format.md && test ! -e docs/modules/builder-contract.md"'`
— asserts both files now exist at the new path and are gone from the old path. The internal-link
correctness of the moved files is confirmed by the repo-wide grep in batch 5's verify (which sees
the whole tree after all retargeting) and by the review gate.
