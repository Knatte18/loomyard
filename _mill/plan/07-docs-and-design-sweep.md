# Batch: docs-and-design-sweep

```yaml
task: "Collapse _pattern into _lyx, and un-reserve _raddle as a hub-level name"
batch: "docs-and-design-sweep"
number: 7
cards: 11
verify: go test -tags integration ./cmd/lyx/ ./internal/lyxcwd/ ./internal/fabriccli/ ./internal/fabricengine/
depends-on: [6]
```

## Batch Scope

This batch closes the Documentation Lifecycle half of the task: every doc, code comment, cobra help string, design doc, and sandbox scenario step that describes either the `_pattern` junction or `_raddle` as hub-level/junction-reached geometry is corrected to the shipped truth.
It runs last, after batch 6's grep sweep has produced the authoritative worklist, so no doc is corrected against a code state that later changed.

Five design docs are in scope and they split into **two kinds of work**.
`manifest/designs/raddle.md` contains no `_raddle` token and no junction claim at all — its only geometry statement is that raddle content lives in the weft — so it **gains a new geometry section**.
That is authoring, not correcting.
The other four (`finalize.md`, `shed.md`, `loom.md`, `fabric-unified-view.md`) each describe `_raddle` as junction-reached or hub-structural and are corrected in the same pass.
`manifest/designs/pattern.md` is deleted outright.

Batch-local decisions.
No migration paragraph is written anywhere: no operator-facing upgrade steps, no CLI verb, no reconcile branch.
`manifest/roadmap.md` is corrected for factual path accuracy only — no item is marked complete, added, or moved, because this is consolidation rather than a planned item landing.
Every `.md` file edited here uses semantic line breaks: one sentence per line, extra breaks at internal independent-clause boundaries, no fixed-column hard-wrap, no trailing double-space or backslash.

## Cards

### Card 31: Correct `CONSTRAINTS.md`'s remaining geometry claims

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/unwire.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Three lines outside the Pattern Leaf Invariant (already widened in card 1) still name the removed geometry.
  Line 50, in the Durable-vs-Ephemeral State Invariant, cites `_pattern` as its example of an optional pathspec dir — with `pathspec` now empty by default there is no live example, so rephrase it to state the rule without an example, or name a hypothetical optional dir explicitly marked as such.
  Line 185, in the Fabric Git Invariant, says agents "write overlay files into `_lyx`/`_raddle` via the junction" — `_raddle` is no longer junction-reached, so this becomes `_lyx` alone; note that raddle content will live at `_lyx/raddle/` and therefore arrives through the same `_lyx` junction.
  Line 199 says unwire preserves weft-side `_lyx`/`.lyx`/`_pattern` content — drop `_pattern`, leaving `_lyx`/`.lyx`, and do not add a replacement third name.
  Do not add a new invariant section: this task introduces no new cross-cutting invariant, only narrows two existing ones.
  Do not touch the Lyxdirs Single-Declarer Invariant.
- **Commit:** `docs(constraints): correct the pathspec, agent-write, and unwire geometry claims`

### Card 32: Correct `docs/overview.md`

- **Context:**
  - `internal/fabricengine/junctionnames.go`
  - `internal/fabricengine/status.go`
  - `internal/pattern/pattern.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Line 75's per-module geometry-token list names `_pattern` — remove it, since the token no longer exists.
  Line 125's directory table row `| _raddle/ | Weft worktree | Weft | Raddle documentation ... |` describes a top-level weft directory reached by junction; rewrite it as `_lyx/raddle/`, anchor-level, reached through the existing `_lyx` junction.
  Line 154's parenthetical listing the hub-structural tokens drops `_raddle`, leaving `_board`, `_portals`, `_launchers`.
  Line 161's junction-mapping bullet for `<host>/_pattern` is deleted outright — that junction no longer exists — and the surrounding text describing the optional `pathspec` default must say the default is now empty.
  Line 166's paragraph explaining that no `_raddle` junction is wired "in this release", citing `status.go`'s pollution scan, is now doubly stale: the scan no longer mentions `_raddle` and no `_raddle` junction will ever be wired.
  Replace it with a statement that raddle is anchor-level by design.
  Line 174 names `_lyx` and `_pattern` as keeping the hard refusal — drop `_pattern`.
  Line 195's parenthetical about `_raddle` junction activation being raddle nav-doc work must be corrected: there is no junction to activate.
  Line 292's description of `internal/pattern` as computing whether `_pattern/PATTERN.md` is present becomes `_lyx/PATTERN.md`.
  Do not change the Documentation Lifecycle section itself; card 37 relies on it as-written.
- **Commit:** `docs(overview): correct the _pattern junction and _raddle geometry throughout`

### Card 33: Correct the remaining prose docs

- **Context:**
  - `internal/lyxcwd/enforcement_test.go`
  - `internal/lyxdirs/dirs.go`
- **Edits:**
  - `docs/shared-libs/lyxcwd.md`
  - `docs/research/linux-portability-survey.md`
  - `README.md`
  - `CLAUDE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/shared-libs/lyxcwd.md`, lines 14 and 80 list `_pattern` among the per-module durable-storage subdirectories — remove it from both, since PATTERN now lives inside `_lyx`.
  Line 110 documents the policed geometry token list and **must stay in sync with the code**: drop `_raddle` and `_pattern`, leaving `_board`, `-weft`, `-HUB`, `_portals`, `_launchers`, `_lyx`, and `.lyx`, matching card 29's `geometryToken` switch exactly.
  Re-read `internal/lyxcwd/enforcement_test.go` and copy the surviving set from the code rather than from this plan.
  In `docs/research/linux-portability-survey.md`, lines 82 and 87 pair `_lyx`/`_raddle` as the two junctioned directories — `_raddle` was never junctioned and never will be, so pair `_lyx` with `.lyx` instead, which is the actual second junction.
  In `README.md` line 61, the junction description routes `_lyx/config/` and `_raddle/` into the weft sibling — drop `_raddle/`, since raddle content will arrive through the `_lyx` junction at `_lyx/raddle/`.
  In `CLAUDE.md` line 16, "Put durable notes in this file, `_raddle/`, or code comments instead" becomes `_lyx/raddle/`.
  Every edited line follows the semantic-line-break convention.
- **Commit:** `docs: correct the _pattern and _raddle geometry in the prose docs`

### Card 34: Correct `manifest/roadmap.md`'s PATTERN paths

- **Context:**
  - `internal/pattern/pattern.go`
  - `docs/overview.md`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Line 37 describes "a weft-backed `_pattern/` folder whose invariants are injected as a pointer into every code-touching agent prompt" — correct the path to `_lyx/PATTERN.md` plus `_lyx/pattern/`, keeping the rest of the sentence intact.
  Line 39 says the content migration out of `CONSTRAINTS.md` into `_pattern/PATTERN.md` + detail docs remains outstanding and still happens only at loomyard-init-via-lyx — correct the paths and keep the outstanding-status claim exactly as it is.
  That migration is explicitly **out of scope** for this task: this task moves the location, never the content, and `CONSTRAINTS.md` stays the single live invariants doc.
  Line 41 reads `See [designs/pattern.md](designs/pattern.md).` — a real relative link to the file card 37 deletes.
  Retarget it to `internal/pattern`'s package godoc (`../internal/pattern/doc.go`) so the link does not dangle after that deletion.
  Do **not** mark any roadmap item complete, do not add an item, and do not reorder anything.
  Per the repo's task-completion rule the roadmap moves only on completing or adding a planned item, and this task is consolidation.
- **Commit:** `docs(roadmap): correct the PATTERN paths without moving any item`

### Card 35: Author the settled anchor-level raddle geometry

- **Context:**
  - `manifest/designs/loom.md`
  - `internal/fabricengine/doc.go`
  - `docs/overview.md`
- **Edits:**
  - `manifest/designs/raddle.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** This file contains no `_raddle` token and makes no junction claim, so nothing here is a correction — it **gains** a new geometry section, and that authoring is the point of the card.
  Add a section (e.g. "## Geometry — where raddle content lives") recording the settled design: every raddle file lives under `_lyx/raddle/` inside each worktree, mirroring that worktree's own code tree; it is resolved by plain path lookup joined onto the anchor path, with no junction of its own and no hub-level presence at all; it reaches the weft through the already-wired `_lyx` junction like every other `_lyx` subtree.
  State explicitly that `_raddle` is **not** a reserved hub name and never was junction-reached, and that this supersedes the earlier hub-level framing that survived in `finalize.md`, `shed.md`, `loom.md`, and `fabric-unified-view.md` until this task.
  State that raddle content is tracked `_lyx` content and therefore needs no `.lyx` mirror, per the Durable-vs-Ephemeral State Invariant.
  Do **not** design or specify the shadow tree's implementation, its path-lookup code, or any accessor — raddle's actual implementation is explicitly out of scope.
  This section records the geometry so the raddle implementation task does not start from the superseded design; it does not build it.
  Keep the file's existing "Status: Design partially exists, not scheduled" framing intact.
- **Commit:** `docs(raddle): record the settled anchor-level _lyx/raddle geometry`

### Card 36: Correct the four design docs that describe `_raddle` as junction-reached

- **Context:**
  - `manifest/designs/raddle.md`
  - `internal/fabricengine/junctionnames.go`
- **Edits:**
  - `manifest/designs/finalize.md`
  - `manifest/designs/shed.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/fabric-unified-view.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `finalize.md`, lines 16, 21, and 29 build the whole two-conflict-mechanics argument on `_raddle`/`_pattern` content being reached through a filesystem junction that `git diff` cannot see across.
  The mechanic is still correct — it is a junction boundary — but the junction is now `_lyx`, so retarget the paths: `_lyx/raddle/` in place of `_raddle`, and drop `_pattern` entirely rather than replacing it.
  Line 29's `fabric.CommitWeft` pathspec example `["_raddle"]` (and its parenthetical "eventually `["_pattern"]` for `PATTERN.md`") becomes `["_lyx"]` with a note that raddle and PATTERN content are both inside it now, which is precisely why the earlier per-directory scoping is obsolete.
  The sentence introducing that call begins on line 28 and attributes it to `fabric.CommitWeft`'s pathspec parameter, not to `ScopedPathspec` — do not rename the mechanism while editing the literal.
  In `shed.md` line 17, the reference to "the weft-side document-driven (non-git) conflict path for `_raddle`/`_pattern` content" retargets the same way.
  In `loom.md`, line 66's diff-exclusion set `_lyx`/`_raddle` collapses to `_lyx` alone, and line 67's "`_raddle` merge-back at Finalize" becomes the `_lyx/raddle/` merge-back.
  In `fabric-unified-view.md`, line 21 states that hub-structural entries `_board`, `_portals`, `_launchers`, `_raddle` stay hardcoded via `HubReservedNames()` — that claim is now false and must be corrected to the three surviving tokens.
  Line 194 reads `- [pattern.md](pattern.md) — hand-authored weft content;` — a second real relative link to the file card 37 deletes.
  Retarget it the same way, to `internal/pattern`'s package godoc, rather than removing the bullet.
  Lines 19, 131, 133, 134, and 195 are **historical change-narrative** describing what past slices did, and rewriting them would falsify the record; leave their wording intact and add one short superseded-note near the top of each affected section stating that `_pattern` no longer exists as a junction and that raddle is anchor-level, with a pointer to `manifest/designs/raddle.md`.
  Do not delete any historical entry.
- **Commit:** `docs(designs): retarget the raddle and PATTERN geometry claims onto _lyx`

### Card 37: Delete `manifest/designs/pattern.md`

- **Context:**
  - `docs/overview.md`
  - `manifest/designs/raddle.md`
  - `manifest/roadmap.md`
  - `manifest/designs/fabric-unified-view.md`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `manifest/designs/pattern.md`
- **Moves:** none
- **Requirements:** Delete the file.
  The Documentation Lifecycle in `docs/overview.md` says module-design docs are mechanical drafts for planned, not-yet-built modules, deleted when the module lands, with the implementation and its tests becoming the source of truth.
  `internal/pattern` has landed, so this deletion is already overdue; this task is what makes the file's content actively wrong, since it describes `_pattern` as "`_lyx`'s first sibling junction" and "the *second* junction the pathspec wires".
  Do not rewrite its `_pattern`-geometry passages instead — that would preserve a file the lifecycle says should not exist.
  This card declares no `Edits:` and therefore fixes no link itself.
  Both known inbound links are owned by earlier cards in this batch and must already be retargeted before this card runs: `manifest/roadmap.md` line 41 (card 34) and `manifest/designs/fabric-unified-view.md` line 194 (card 36).
  Before deleting, run `grep -rn 'designs/pattern\.md\|](pattern\.md)' --include='*.md' .` to confirm both are gone and that no third inbound link has appeared.
  If the grep reports a link in a file no card in this batch edits, stop and report it rather than deleting the target — a dangling link is a worse end state than a late deletion.
  Record in the commit message that this is an overdue Documentation Lifecycle deletion, not a change this task's scope required.
- **Commit:** `docs(designs): delete the landed pattern module design doc`

### Card 38: Rewrite `internal/fabricengine/doc.go`'s pathspec narrative

- **Context:**
  - `internal/fabricengine/template.yaml`
  - `internal/fabricengine/junctionnames.go`
  - `internal/yamlengine/reconcile.go`
  - `internal/fabricengine/reconcile.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Lines 53-92 are the package's authoritative narrative about the `pathspec` default and are a **rewrite**, not a token substitution.
  Lines 53-55 state the default is `_lyx _pattern` and that `PATTERN.md` is staged alongside `_lyx` by the same `commitWeft` call — both clauses go; replace them with a statement that the default is empty and that `_lyx` arrives from `structuralCommittedDirs`, so PATTERN content is committed as ordinary `_lyx` content.
  Lines 57-73 are the "narrow-pathspec asymmetry" gap, currently saying an already-initialised worktree "stays on `pathspec: _lyx` forever and never persists `_pattern` content".
  This passage **inverts** rather than disappearing: because `configsync.ReconcileAll` -> `yamlengine.Reconcile` never rewrites an existing `pathspec:` key, a deployed worktree now stays on `pathspec: _pattern` — *wider* than the template, not narrower — keeping a junction the template no longer wires, until the repo is re-cloned.
  Write that inverse explicitly, and make this passage the doc home for the fresh-clone-only fact: `applyStaleRemoval` tears down a junction absent from `RepoWiredNames`, but only once that repo's recorded `pathspec` is actually empty, which it will not be, so changing `template.yaml` governs newly cloned repos only.
  State that this is accepted rather than a defect, because the sole deployed repo is the throwaway SANDBOX which is re-cloned rather than migrated.
  Do not describe an upgrade path — none exists.
  Lines 75-79's deliberate-asymmetry framing survives; only its example changes.
  The "four hub-structural tokens (`_board`, `_portals`, `_launchers`, `_raddle`)" phrase spans lines 84-85 — `_raddle` closes on line 85 — and becomes three; line 87's "today just `_pattern`" becomes "empty today".
  Lines 86-92's worked example — append a name to `template.yaml`'s `pathspec:` default to wire a future optional module — stays, because the mechanism is still true and worth documenting; it simply no longer has `_pattern` as its live instance and becomes a purely hypothetical example.
  Also correct line 28's PATTERN-residue description, and lines 115, 118, 364, and 388, which name `_pattern` as a wired junction.
  The `_pattern` references at 364 and 388 enumerate what `clone` creates and what `unwire` preserves — drop `_pattern` from both lists rather than retargeting it.
- **Commit:** `docs(fabricengine): rewrite the pathspec narrative for the empty default`

### Card 39: Correct the remaining `internal/fabricengine` code comments

- **Context:**
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/junctionnames.go`
  - `manifest/designs/raddle.md`
- **Edits:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/weftwiring.go`
  - `internal/fabricengine/cleanup.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** These are comment-only edits; no behaviour changes in this card.
  `junction.go` lines 172 and 175 describe `_lyx` and `_pattern` as the two junctions that may hold user content and cite "make `_pattern/` in the repo and start writing" as the natural operator flow — rewrite for `_lyx` alone, since the hard refusal now protects `_lyx` including `_lyx/PATTERN.md`.
  `unwire.go` line 30's comment about weft `_pattern` content being preserved by design retargets to `_lyx`.
  `reconcile.go` line 348's hub-structural parenthetical `(_board/_portals/_launchers/_raddle)` drops `_raddle`.
  `weftwiring.go` line 12's "preserving history for future `_raddle` squash-merge-back" retargets to `_lyx/raddle/`.
  `cleanup.go` lines 81 and 88 document `raddleFoldedBack` and the flag matrix; `raddleFoldedBack` is a stub (`func raddleFoldedBack(_ string) bool { return false }`) with no path logic at all, so its `_raddle` references need a **wording update only** — retarget them to `_lyx/raddle/` and do not change the function's signature or body.
  After this card, no comment in `internal/fabricengine` may describe `_pattern` as a wired junction or `_raddle` as hub-level.
- **Commit:** `docs(fabricengine): retarget the remaining junction and raddle comments`

### Card 40: Correct `internal/fabriccli`'s cobra help text

- **Context:**
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/doc.go`
  - `internal/fabricengine/pull.go`
- **Edits:**
  - `internal/fabriccli/fabric.go`
  - `internal/fabriccli/weft_verbs.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** This is user-visible help text, so help accuracy is a review obligation under the CLI/Cobra Invariant — every affected `Short`/`Long` must be re-read, not just the lines that mention the removed names.
  In `fabric.go`, line 145's "(`_lyx` and `_pattern`)" junction pair becomes `_lyx` and `.lyx`.
  Lines 188-190 are the `junction_healthy`/`junction_reason` sentence naming the junction pair `(_lyx and _pattern)` — retarget that pair to `_lyx` and `.lyx`.
  The pollution-scan sentence is a separate statement beginning at line 191 and running to line 194; it covers `_lyx`, `_pattern`, and `_raddle` with `_raddle` matches being report-only — rewrite it for `_lyx` alone, with every match carrying the automated `git rm --cached` remedy, and drop the report-only clause entirely since no report-only class remains.
  Lines 207 and 211 describe junction repair covering both host junctions and a worktree "wired before the `_pattern` junction existed" — retarget the first to the actual junction pair and delete the pre-`_pattern` upgrade narrative rather than rewording it.
  Line 262's "(`_lyx`, `.lyx`, and `_pattern`)" drops `_pattern`.
  In `weft_verbs.go`, line 93's comment about a repo's pathspec naming only `_pattern` becomes a generic optional-name statement; line 148's help text "(default: `_pattern`)" becomes "(default: none)"; line 213's "post-anchor weft commits touch `_pattern/`" retargets to the `_lyx` PATTERN paths.
  Do not add or remove any command, flag, or `Short`, and do not change the `Command()`/`RunCLI` seam — the CLI surface itself is unchanged by this task.
- **Commit:** `docs(fabriccli): correct the junction, pollution, and pathspec help text`

### Card 41: Correct the sandbox fabric suite's executable steps

- **Context:**
  - `internal/fabricengine/unwire.go`
  - `internal/fabricengine/pull.go`
  - `CONSTRAINTS.md`
  - `cmd/lyx/sandbox_coverage_test.go`
- **Edits:**
  - `tools/sandbox/SANDBOX-FABRIC-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** These are manual scenario steps an operator runs, not prose, so they cannot be token-substituted.
  Three lines change, and **no `_extra` pathspec seeding is added to the suite**: with `_lyx` and `.lyx` both junctioned, scenario F5 still has two junctions on disk to prove "removes every fabric junction present", which was the assertion's real content.
  Line 185's parenthetical example `(e.g. _lyx, _pattern)` becomes `(e.g. _lyx, .lyx)`.
  Line 186 **contradicts the code today and must be rewritten, not merely re-pointed**: it instructs the operator to confirm unwire "clears the weft-side `_lyx` content ... while leaving `_pattern` content on the weft side untouched", but `unwire.go`'s package doc states the opposite — weft-side `_lyx` and `.lyx` are preserved — and `CONSTRAINTS.md`'s unwire line agrees with the code.
  Rewrite it to confirm weft-side `_lyx` content is **preserved**, explicitly including `_lyx/PATTERN.md`, and drop the second-junction clause entirely.
  Line 205's F6 residue assertion, "which `_pattern/`-touching weft commits need review", becomes the new PATTERN paths.
  Flag the line-186 correction explicitly in the commit message: it is a pre-existing defect this task happens to expose, not a change this task causes.
  Do not add or remove a scenario and do not change any `**Covers:**` line — `cmd/lyx/sandbox_coverage_test.go` checks module coverage against the live cobra root and this task adds no module.
- **Commit:** `docs(sandbox): correct the F5 unwire steps and the F6 residue assertion`

## Batch Tests

`verify:` runs the integration-tagged suites for `cmd/lyx`, `internal/lyxcwd`, `internal/fabriccli`, and `internal/fabricengine`.
Although most of this batch is markdown, four cards edit Go files whose comments and help strings are machine-checked: `TestEnforcement_FabricVocabulary` (`internal/lyxcwd/enforcement_test.go`) scans comments and string literals in production `.go` files under `internal/` and `cmd/`; `cmd/lyx`'s `helptree_test.go`, `drift_test.go`, and `longlist_test.go` pin cobra help output; and `cmd/lyx/sandbox_coverage_test.go` reads `tools/sandbox/*SUITE.md`.
That is why the verify scope is not empty for a batch that looks docs-only.

No new test is written in this batch.
The markdown-only cards (31-37, 41) have no runnable surface of their own; their correctness is a review obligation, checked against the code the earlier batches shipped.

Two review obligations this batch cannot machine-check, called out so the reviewer looks for them:
the prose-doc split under the Fabric Vocabulary Invariant — a doc explaining fabric's own mechanism keeps the `weft`/`warp` vocabulary while a doc describing a consumer module rewords — and the semantic-line-break convention on every edited `.md` line.

After this batch, `grep -rn '_pattern\|_raddle' internal/ cmd/ docs/ manifest/ tools/ README.md CLAUDE.md CONSTRAINTS.md` must report only the two deliberate survivors named in card 30: `internal/lyxcwd/raddle_guard_test.go` and `internal/fabricengine/structuraldirs_test.go`.
Anything else is a genuine miss.
