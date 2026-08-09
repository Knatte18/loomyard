```yaml
slug: shed-model-contradiction-sweep
title: "shed: sweep the remaining producer-model contradictions and add the pointer-rule invariant"
depends_on: ["format-docs-name-producers", "batcher-standalone-split"]
brief: |
  Sweep the remaining contradictions inside shed.md, loom.md, and roadmap.md as the final owner of all three files, add the short CONSTRAINTS.md pointer-rule invariant, and record the model's surfaced open questions in shed.md's producer-contract section and as a named precondition on roadmap.md's Planned Shed item.
```

# shed: sweep the remaining producer-model contradictions and add the pointer-rule invariant

## Why

**`deliverable-is-reconcile-the-residue`.**
`shed.md`, `loom.md`, and `roadmap.md` are the landed, authoritative statement of the model.
They were rewritten immediately before this scoping worktree spawned, on the same model this whole follow-up set applies — everything else reconciles *to* them, and never the reverse.

This task exists because the partial conversion left contradictions inside those same three files, plus a set of claims that only become false once tasks A through F have landed.
This task runs last and writes the finished state.

## What needs to happen

### Part one — `shed.md`'s own contradictions

- `:7` and `:19` say "superseding ... **below**" and "the pre-revision text **below**", but that text was deleted in commit `256b8262`, so both references dangle.
- `:18` says Finalize is shared "by value"; it becomes "by reference" per the `finalize-shared-by-reference` decision — two of three sources already say by reference, and it is the phrasing that carries the actual meaning.
- `:13` enumerates `loom`'s producer list verbatim, and `:41` lists the mechanical Go-function producers.
  Both must gain `Discussion-Review-Gate` once task C inserts it into `loom.md`'s table, or the two docs silently disagree about what `loom`'s list contains.

### Part two — the stale "this task is still pending" claims

This scoping task itself falsifies these claims, so this task retires them:

- `shed.md:63`'s claim that wiki task `shed-producer-model-scoping` is the dedicated pass that reconciles any remaining detail mismatch between the two docs.
- `loom.md:76`'s version of the same claim about the producer table.
- `loom.md:91–94`'s version of it.

### Part three — `loom.md`'s remaining residue

This task is `loom.md`'s final owner, and owns everything in the file except the table rows task C already fixed:

- `:15–17`'s naming note, which still says "`loom` = `Shed` + loom's own Preflight + the Discussion/Plan/Webster producer" — old slot framing, contradicting the table 25 lines below it, and whose "This doc has not been rewritten to extract `Shed` explicitly" claim is now false.
- `:29`, which links the v2 `plan-format.md` that task A deletes and frames v3 as "the target format is changing" — task B's mechanical sweep deliberately leaves this self-contradicting for this task to repair.
- `:91–94`, the naming note calling `internal/builderengine` and `internal/buildercli` "a real, separate, already-shipped sibling implementer loop", plus its `builder-contract.md` link.
- `:187`, the module-decomposition row repeating the same already-shipped-sibling claim and `builder-contract.md` link.
- `:56`, row 8's `Batchifier` entry, rewritten to match whatever task F landed.

### Part four — the other files

- `hardener.md:17`'s "producer-slot".
- `docs/overview.md:272`'s stale chain "Preflight → Discussion → Plan → Builder → Raddle → Finalize".
- `manifest/roadmap.md`, where this task is the last owner and therefore carries:
  - `:68`'s "deferred phase slot between Builder and Finalize", moved off task D.
  - the retirement of `:31`'s "**A dedicated scoping task should run first** ... this item is not yet broken down into buildable units" — stale the moment this scoping task lands.
    This task is the right place to declare the breakdown done and name the six follow-up tasks.

### Part five — the two additions

- Resolve `loom.md:75`'s thin-Output question per `preflight-finalize-thin-output-is-permitted`, and record the resolution in `shed.md`'s producer-contract section: the Output contract permits a pass/fail gate signal with no artifact, because `Preflight` and `Finalize` genuinely have no output artifact, and the resume-on-output-files rule degrades gracefully — a producer with no artifact simply re-runs on resume, which is correct for both.
- Add the new short `CONSTRAINTS.md` invariant naming the pointer rule as a review obligation.
  See the dedicated subsection below.

**This task re-reads `shed.md` and `loom.md` end to end**, rather than working the line lists above — exactly as task D does for `finalize.md`.
`shed.md:63` sits inside a whole "Why this doc doesn't rewrite loom.md's full detail" section whose premise changes once tasks C and E have run.

## The pointer-rule invariant

**`pointer-rule-becomes-a-short-constraints-invariant`.**
Add a new, short `CONSTRAINTS.md` invariant naming the pointer rule as a review obligation: an instruction file must never duplicate or paraphrase another producer's format-contract content, only point at it.
It matches the file's existing seam-invariant precedent — Treadle Runner-Seam, Scout Engine-Seam, Shuttle Provider-Seam, Batcher Registry+Config.
It is enforced by review, not machine-checked.
It must be **short** — one invariant statement plus an "Enforced by: review obligation" line, in the same shape as the existing entries, not a treatise.

## Open questions

The following three questions are surfaced deliberately, per the discussion, and are **not** resolved by this task.

**Question 1 — `Webster` violates the producer-atomicity rule.**
The landed model states a producer is always atomic: one mechanical action, or one LLM session, never an internal multi-step process of its own.
But `loom.md:57` lists `Webster` as a black box with its own per-batch fork loop, opaque to `loom`'s flat list — precisely an internal multi-step process.
Either atomicity admits a carve-out for black-box producers that own their own loop, or `Webster` decomposes into flat producers the way `Plan` did.
This is the single largest unresolved tension in the model, to be decided before `Shed` is built rather than during.
**This task's obligation:** record it as a named precondition on `manifest/roadmap.md`'s Planned `Shed` item, not merely as prose in a design doc — recording it without gating it is how it gets skipped.

**Question 2 — `Discussion-Write` has no Input.**
`loom.md:50` records its Input as "— (starting point)".
The thin-Output carve-out is now decided for `Preflight` and `Finalize`, but the symmetric thin-*Input* case has not been.
The task body itself is arguably the Input, which would make the pointer target the wiki task record rather than a format-contract file — a different kind of pointer than every other row in the table.
This task records it in `shed.md`'s producer-contract section, immediately beside the thin-Output carve-out it mirrors.
It does **not** get a roadmap gate the way question 1 does, because it is a contract-wording decision rather than a precondition that could invalidate `Shed`'s design.

**Question 3 — `shed` is an overloaded name in this repo.**
`docs/overview.md:289` and `:318` record that earlier `reed` drafts split the model and view into separate modules named `shed` and `glance`.
A reader hitting `:289` first will mis-resolve it, now that "shed" names the outer phase-FSM.
An explicit disambiguating note is worth more than leaving two unrelated meanings in one doc set.
This task owns it as `docs/overview.md`'s last owner in the chain.

**The deferred-phase-enum record.**
Per `phase-enum-realignment-is-deferred-to-the-shed-build`: `internal/loomengine/coherence.go:14–22`'s `validPhases` map and `docs/reference/status-schema.md`'s matching phase enum are deliberately left alone by tasks A through F.
Realigning them lands with the `Shed` build task, because the flat producer list replaces the phase enum rather than editing it, and rewriting it now would invent an interim phase set that `Shed` would immediately discard.
This task records this deferral explicitly alongside its roadmap edits, so a later reader finds a decision rather than an oversight.

## Scope

This task holds three ownership positions:

- `loom.md`'s final owner, after tasks B and C.
- `roadmap.md`'s last owner, after task A.
- `docs/overview.md`'s last owner, after tasks A and B.

This task writes the finished state rather than guessing, which is the whole reason it is serialized last.

## Sequencing

`depends_on: format-docs-name-producers, batcher-standalone-split` — this task must see task C's finished table, and it cannot write `loom.md` row 8 before task F has decided what row 8 says.

## Acceptance

Docs-only, the same criterion as task D: every relative markdown link and anchor introduced or touched resolves, via a link-check pass over `manifest/` and `docs/`.
</content>
