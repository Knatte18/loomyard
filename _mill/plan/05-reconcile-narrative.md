# Batch: reconcile-narrative

```yaml
task: 'loom: pin the spawn/handover status schema + discussion-format.md'
batch: reconcile-narrative
number: 5
cards: 4
verify: 'bash -c "! grep -rIn --exclude-dir=_mill --exclude-dir=.git --exclude-dir=.lyx -e modules/plan-format.md -e modules/builder-contract.md . && ! ( grep -n -e plan-format.md -e builder-contract.md docs/modules/loom.md | grep -v reference )"'
depends-on: [1, 2, 3, 4]
```

## Batch Scope

Reconcile the four narrative docs with the pinned contracts, each in one card so its content
reconciliation and its inbound-link retargeting land together (avoiding two batches touching the
same file). Runs last (depends on 1–4): batches 1–2 create the two new docs it links to, batch 3
moves the contract docs, batch 4 clears the non-narrative inbound refs — so this batch's
repo-wide grep is the final link-integrity gate over the whole tree. Per the Documentation
Lifecycle constraint, this reconciliation ships in the same task as the contracts it matches.

## Cards

### Card 9: Reconcile loom.md (status contract + Setup→Preflight + inbound links)

- **Context:**
  - `_mill/discussion.md`
  - `docs/reference/status-schema.md`
- **Edits:**
  - `docs/modules/loom.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Reconcile `docs/modules/loom.md` (per discussion `loom-md-reconciliation`):
  1. **"State & contracts" section:** point the status-file description at the new
     `../reference/status-schema.md`; correct the verdict-history wording to a **per-phase
     outcome** trail (loom's status records phase-level outcomes; per-round verdicts live in
     perch's block files) — remove any wording implying loom's status carries per-round verdict
     history; state the status file is **JSON via `internal/state`**; keep the note that loom's
     `pause_requested` flag lives **in-status**.
  2. **Setup→Preflight rename:** in the phase-machine diagram (near line 54, the `Setup` node),
     the "Setup validates geometry and preconditions…" prose (near line 63), and the
     module-decomposition table row (near line 239, `| Setup | uses existing modules | …`),
     rename `Setup` → `Preflight`. Match the pinned `phase` vocabulary
     (`preflight|discussion|plan|builder|raddle|finalize|done`).
  3. **Inbound sibling links** (loom.md stays in `docs/modules/`, so links to the moved docs
     become `../reference/…` per Decision `retarget-mapping`): near line 36
     `([plan format](plan-format.md))` → `([plan format](../reference/plan-format.md))`; near
     line 111 `[plan-format.md](plan-format.md)` → `[plan-format.md](../reference/plan-format.md)`;
     near line 117 `[modules/builder-contract.md](builder-contract.md)` →
     `[builder-contract.md](../reference/builder-contract.md)` (retext the display too, so no
     `modules/builder-contract.md` token survives); near line 235 (module table row) both
     `[plan-format.md](plan-format.md)` → `[plan-format.md](../reference/plan-format.md)` and
     `[builder-contract.md](builder-contract.md)` → `[builder-contract.md](../reference/builder-contract.md)`.
     Every `plan-format.md`/`builder-contract.md` mention in loom.md must resolve via
     `../reference/…` after this card. Leave links to `loom.md`/`hardener.md`/`../overview.md`
     untouched.
- **Commit:** `docs(loom): reconcile status contract, Setup→Preflight, relocated links`

### Card 10: Reconcile overview.md (doc-lifecycle + Setup→Preflight + Raddle + inbound links)

- **Context:**
  - `_mill/discussion.md`
  - `docs/reference/status-schema.md`
  - `docs/reference/discussion-format.md`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Reconcile `docs/overview.md`:
  1. **Documentation-lifecycle section:** amend it to distinguish *module-design docs*
     (`docs/modules/`, deleted when the module lands) from *durable contract/reference docs*
     (`docs/reference/`, kept) — naming `status-schema.md`, `discussion-format.md`,
     `plan-format.md`, `builder-contract.md`, `model-spec.md` as the durable-contract examples
     that live in `docs/reference/`.
  2. **loom phase blurb (near line 273):** rename `Setup` → `Preflight` and add the missing
     `Raddle` step, so the phase list reads
     `Preflight → Discussion → Plan → Builder → Raddle → Finalize`.
  3. **Inbound relative links** (overview.md is at the `docs/` root, so `modules/…` →
     `reference/…`): near line 269 `[plan-format.md](modules/plan-format.md)` →
     `[plan-format.md](reference/plan-format.md)`; near line 272
     `[modules/builder-contract.md](modules/builder-contract.md)` →
     `[builder-contract.md](reference/builder-contract.md)`; near line 375
     `[modules/builder-contract.md](modules/builder-contract.md)` →
     `[builder-contract.md](reference/builder-contract.md)`. Leave `modules/loom.md`,
     `modules/hardener.md`, and other non-moving links untouched.
- **Commit:** `docs(overview): split doc-lifecycle, Setup→Preflight+Raddle, relocated links`

### Card 11: Reconcile roadmap.md (milestone 12.1 done + inbound links)

- **Context:**
  - `_mill/discussion.md`
  - `docs/reference/status-schema.md`
  - `docs/reference/discussion-format.md`
- **Edits:**
  - `docs/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Reconcile `docs/roadmap.md`:
  1. **Mark milestone 12 build-order sub-item 1 ("Contracts first") ✅ Done**, linking the two
     new docs `[status-schema.md](reference/status-schema.md)` and
     `[discussion-format.md](reference/discussion-format.md)`. Keep the sub-item's existing prose;
     add the done-marker + links (analogous to how other milestones mark ✅ Done with a doc link).
  2. **Inbound relative links** (`modules/…` → `reference/…`) at the sites that reference the
     moved docs: near line 57 `[modules/builder-contract.md](modules/builder-contract.md)`;
     near line 60 `[plan format](modules/plan-format.md)`; near line 74
     `[modules/builder-contract.md](modules/builder-contract.md)`; near line 189
     `[modules/builder-contract.md](modules/builder-contract.md)`; near line 195 the
     `[modules/plan-format.md](modules/plan-format.md)` link ONLY (leave the adjacent
     `[modules/loom.md](modules/loom.md)` untouched — loom.md does not move); near line 333
     `[modules/builder-contract.md](modules/builder-contract.md)`. Retarget each to
     `reference/…` and drop `modules/` from any display text so no `modules/plan-format.md` or
     `modules/builder-contract.md` token survives.
- **Commit:** `docs(roadmap): mark contracts milestone done, retarget relocated links`

### Card 12: Retarget long-term-ideas.md inline path

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `docs/long-term-ideas.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/long-term-ideas.md` (line 17), the inline code reference
  `` `modules/plan-format.md` `` becomes `` `reference/plan-format.md` `` (this file is at the
  `docs/` root). Change only that token; leave surrounding prose unchanged.
- **Commit:** `docs(long-term-ideas): retarget plan-format path after relocation`

## Batch Tests

`verify: 'bash -c "! grep -rIn … -e modules/plan-format.md -e modules/builder-contract.md . && ! ( grep -n -e plan-format.md -e builder-contract.md docs/modules/loom.md | grep -v reference )"'`
— two gates. The first is the **repo-wide** link-integrity check: after every batch, no
`modules/plan-format.md` or `modules/builder-contract.md` path token may survive anywhere in the
tree (excluding `_mill/`, which legitimately quotes the old paths in the discussion and plan).
The second catches loom.md's bare-sibling links specifically: every `plan-format.md` /
`builder-contract.md` mention in `docs/modules/loom.md` must appear on a line that also contains
`reference` (i.e. resolves via `../reference/…`) — a bare `(plan-format.md)` that the first
grep's `modules/…` pattern would miss fails here instead. Pure-docs batch; no code surface.
