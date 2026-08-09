# Follow-up tasks — Scope the Shed producer-model rewrite

This task's Scope section names this doc.
It is task state, not part of the durable doc set — it describes work to be done, and it has no place in `manifest/` or `docs/` once the six tasks land.

`shed.md`, `loom.md`, and `roadmap.md` were already rewritten for the flat producer model in the two commits immediately before this worktree spawned, so the remaining work this scoping task found is residue and contradictions rather than three full rewrites, and those three files are the decided state everything else reconciles to.
This doc records the six follow-up tasks that carry out that reconciliation, why they are ordered the way they are, and the open questions this scoping pass surfaced without resolving.

## The six tasks

| ID | Slug | Kind | `depends_on` |
|----|------|------|--------------|
| A | `builder-retire` | code | — |
| B | `plan-format-drop-v3-suffix` | code (mechanical) | A |
| C | `format-docs-name-producers` | docs | B |
| D | `raddle-finalize-fold-and-link-repair` | docs | A |
| E | `shed-model-contradiction-sweep` | docs | C, F |
| F | `batcher-standalone-split` | code and docs | B |

Chain: A → B → {C, F} → E, with D branching off A in parallel.

Each task's full text is staged at `_mill/followup/<ID>-<slug>.md` and published to the wiki at `proposal-<slug>.md`;
this section is a summary, not a second copy of the body.

**A — `builder-retire`.**
Deletes `internal/builderengine` and `internal/buildercli` outright, sweeps every reference out of the CLI help tree, `configreg`, sandbox coverage, and docs, and re-statuses `docs/reference/builder-contract.md` as a retired-design reference rather than deleting it.
Builder is live-registered today, not dormant, so it costs recurring maintenance until removed.

**B — `plan-format-drop-v3-suffix`.**
Renames `docs/reference/plan-format-v3.md` to `docs/reference/plan-format.md` and sweeps every reference — docs and Go identifiers alike — mechanically, by a scripted find/replace, with completion judged by a zero-hit case-insensitive repo grep rather than by any file count written down beforehand.

**C — `format-docs-name-producers`.**
Rewrites `discussion-format.md` and the renamed `plan-format.md` to name their producers and contracts explicitly in producer-model terms, adds the new `Discussion-Review-Gate` producer covering `discussion-format.md`'s checks 1–2, and scoped-edits `loom.md`'s producer table rows 2–7 to name the artifacts that actually exist and to list the new gate.

**D — `raddle-finalize-fold-and-link-repair`.**
Folds Raddle-regeneration into `finalize.md`'s own producer contract as a first-class part of the merge rather than a Related-section mention, and repairs the dead `fabric.md` links, dead `#the-phase-machine` anchors, and non-existent Weft Git Invariant citation left across `finalize.md`, `raddle.md`, and `self-report.md`.
It is the one follow-up task that stays genuinely parallel to the rest of the set.

**E — `shed-model-contradiction-sweep`.**
Sweeps the remaining producer-model contradictions inside `shed.md`, `loom.md`, and `roadmap.md` as the final owner of all three files, adds the short `CONSTRAINTS.md` pointer-rule invariant, and records the model's surfaced open questions in `shed.md`'s producer-contract section and as a named precondition on `roadmap.md`'s Planned `Shed` item.

**F — `batcher-standalone-split`.**
Extracts `internal/batcher` out of webster as a standalone `configreg`-registered module with its own `batcher.yaml`, moves both live `batcher.Select` call sites onto batcher's own entry point, and has reconcile report — never honour — a leftover `webster.yaml` `batcher` key in an existing worktree.

## Why the ordering is what it is

The set is split by file-cluster, with `depends_on` wired wherever two tasks edit the same file and left parallel only where the file sets are genuinely disjoint.

**Why E is serialized last, not parallel.**
E's original parallel scoping rested on a false disjointness claim: E edits `loom.md:15–17` and `:75` while C scoped-edits the table rows, and `docs/overview.md` is edited by A, B, and E alike.
Doc edits are cheap, so the parallelism E bought is worth less than the conflict risk, and sequencing it last lets its contradiction sweep read C's finished table rather than guess at it.
`loom.md:75` holds both open questions — the Discussion pre-gate owned by C's decision and the thin-Output carve-out owned by E's — so a single owner beats splitting one line between two tasks.

**Why D stays parallel.**
D owns `finalize.md`, `raddle.md`, and `self-report.md`, and no other task touches any of them.

**The `loom.md` three-owner note.**
`loom.md` has three owners, in chain order — B, then C, then E, never concurrently.

- B is a mechanical owner: its zero-hit acceptance criterion rewrites `loom.md:29` and table rows 5–7, which spell `plan-format-v3.md` — paths and names only, never prose.
- C owns the producer table's rows 2–7 — the artifact-name fixes and the `Discussion-Review-Gate` insertion.
- E owns everything else in the file and runs last, after both C and F, so it writes the finished state rather than guessing at it.

## Surfaced open questions

Four open questions surfaced during this scoping pass and are deliberately not resolved here; each is named below with its owner.

1. **The Webster producer-atomicity tension.**
   `loom.md:57` lists `Webster` as `black box (LLM + mechanical internally)`, which is an internal multi-step process contradicting the model's atomicity rule.
   **Owner: E**, as a named precondition on `manifest/roadmap.md`'s Planned `Shed` item — recording it without gating it is how it gets skipped.
2. **The thin-Input case for `Discussion-Write`.**
   The thin-Output carve-out is decided for `Preflight`/`Finalize`; the symmetric thin-Input case for `Discussion-Write`, whose Input is "— (starting point)", has not been.
   **Owner: E**, in `shed.md`'s contract section, beside the thin-Output carve-out — no roadmap gate, since this is a contract-wording decision rather than a precondition that could invalidate `Shed`'s design.
3. **The overloaded `shed` name.**
   `docs/overview.md` records earlier `reed` drafts that split the model and view into separate modules named `shed` and `glance`; "shed" now also names the outer phase-FSM.
   **Owner: E**, as `docs/overview.md`'s last owner in the chain.
4. **The deferred Hardener/Tenter Raddle-into-Finalize fold.**
   Deferred by the landed design and stays deferred — recorded here so a future pass does not read the silence as an oversight.

## Deferred phase-enum realignment

The `phase` enum in `internal/loomengine/coherence.go`'s `validPhases` map, and its twin in `docs/reference/status-schema.md`, land with the `Shed` build task and are deliberately untouched by all six follow-up tasks.
