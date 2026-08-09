# Batch: module-phase-docs

```yaml
task: 'builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference'
batch: module-phase-docs
number: 4
cards: 7
verify: null
depends-on: [3]
```

## Batch Scope

This batch aligns the remaining prose with the tree batches 1–3 produced: it removes the builder **module** from every catalogue that lists it, renames the loom **phase** `Builder` → `Webster` everywhere it is written out, and rewrites every sentence whose claim this task falsified — the v2-coexistence prose class, the "webster is builder's sibling" definitions, and the sandbox-suite inventories.

One batch because every card is the same kind of edit against a different document, they share the phase-rename and module-word vocabulary, and none of them has a runnable surface.
It depends on batch 3 because two of its files (`docs/reference/status-schema.md`, `docs/reference/plan-format-v3.md`) are also edited there for link repair — sequencing the two batches keeps a single owner per file at a time.

Batch-local decision: the phase rename is a **true rename** and may be done mechanically, gated by the zero-hit grep;
the falsified-claim prose is a **rewrite** and is done by hand, sentence at a time.
Where another task owns a line's eventual rewrite, the edit here is the minimum that stops the sentence being false — it must not pre-empt that task's own pass.

## Cards

### Card 10: Retire builder from docs/overview.md

- **Context:**
  - `docs/reference/webster-contract.md`
  - `internal/websterengine/doc.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the Documentation Lifecycle's durable-doc list, drop `plan-format.md` and `builder-contract.md` and add `webster-contract.md`, leaving `status-schema.md`, `discussion-format.md`, `plan-format-v3.md` and `model-spec.md` in place — the lifecycle convention must not name files that no longer exist.
  Delete the whole **builder** module-table entry.
  Rewrite the **webster** entry so it defines webster on its own terms — the implementer module: one long-lived Master session that reads the plan once and forks one implementer per execution batch in-session — instead of as a "fork-based sibling of builder", and re-point its deep link from `builder-contract.md` to `webster-contract.md`.
  In the **loom** entry, change the phase sequence `Preflight → Discussion → Plan → Builder → Raddle → Finalize` to name `Webster` in place of `Builder`.
  In the `internal/pattern` tree comment, change "consumed by builder/webster/burler/loom" to drop builder.
  In the shared-infrastructure paragraph, change the prompt-consumer list "(builder implementer, webster fork/Master, burler review+fix, loom plan)" to drop the builder implementer.
  In the See-also list, replace the `builder-contract.md` bullet with a `webster-contract.md` bullet describing webster's cross-module contract (the `_lyx/webster/` boundary, `outcome.yaml`, and the `summary.md` artifact Finalize consumes).
- **Commit:** `docs(overview): retire the builder module entry and rename the Builder phase`

### Card 11: Retire builder from README.md

- **Context:**
  - `docs/overview.md`
- **Edits:**
  - `README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Remove `lyx builder` from the subcommand-tree example in the `lyx` binary bullet.
  Delete the **builder** module bullet outright.
  Rewrite the **webster** bullet so it stands alone — the implementer module, one long-lived Master session reading the flat card-list plan via `internal/planparser` and forking one implementer per batch in-session — dropping both "a fork-based sibling of `builder`" and the trailing "`builder` stays frozen in-tree as the plan-format-v2 consumer" sentence.
  In the **loom** bullet, rename the phase `Builder` to `Webster` in the phase sequence.
  In the module-topology sentence, change "`builder` and `webster` branch off `shuttle` directly" to name `webster` alone, keeping the parenthetical about driving fat Go verbs rather than a `perch`/`burler` gate loop.
- **Commit:** `docs(readme): retire the builder module and rename the Builder phase`

### Card 12: Rename the phase and drop builder prose in status-schema.md

- **Context:**
  - `internal/loomengine/coherence.go`
  - `internal/websterengine/state.go`
- **Edits:**
  - `docs/reference/status-schema.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename the phase token in every occurrence so the doc matches `internal/loomengine/coherence.go`'s `validPhases` after card 3: the `"phase": "builder"` example values, all three spellings of the enum `preflight | discussion | plan | builder | raddle | finalize | done`, the `start_sha` prose naming "when Builder begins" and "until Builder starts", the mid-run example's narration line naming a `builder-review` round, and the mid-run intro sentence "Builder now mid-review-gate".
  The JSON examples carry an inline `//` comment repeating one of those phrases, which must be renamed with them: `start_sha`'s "repo HEAD stamped when Builder begins (Raddle diff base)".
  All become `webster` / `Webster` / `webster-review`.
  Separately, rewrite the prose that names the builder **module**, which no longer exists: the `internal/state` sentence citing "the same mechanism `builder` uses for its own `state.json`" (cite webster's own `_lyx/webster/state.json` instead), the `pause_requested` divergence note "diverges from builder, which uses a separate flag file" — which appears twice, once as per-field-notes prose and once as the inline `//` comment on the `pause_requested` line of the first JSON example, and both must be restated against webster's pause flag, the live example — the `format:` paragraph contrasting with "`plan-format.md` (which needs `format:` because `builder` mechanically validates plans across a real v1→v2 bump)" — restate the reason on this file's own terms (single writer, no version-compatibility pressure) without the comparison — and the strict-parsing sentence citing "builder's `ParseOutcome`" (cite webster's own strict outcome decode).
  In the Status banner, the "loom analogue of …" prose now names one live doc plus one that returns under a later task: rewrite it so it no longer asserts that a v2 plan format coexists, and leave the `plan-format.md` link itself dangling — card 9 already re-pointed the `builder-contract.md` half at `webster-contract.md`.
- **Commit:** `docs(status-schema): rename the Builder phase to Webster and drop builder prose`

### Card 13: Rebuild model-spec.md's worked example on webster.yaml

- **Context:**
  - `internal/websterengine/template.yaml`
- **Edits:**
  - `docs/reference/model-spec.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite the Status banner: change "builder's roles" to "webster's roles", drop the "Pinned alongside [plan-format v2](plan-format.md) and the emerging [v3](plan-format-v3.md)" claim — v2 no longer exists — so it pins alongside the single live plan format, and change "The registry loader and spec parser land with the first consumer (`builder`)" to name webster.
  Leave the `plan-format-v3.md` link intact and do not retarget the removed v2 link at it.
  Rebuild the Precedence section entirely on `webster.yaml`.
  Its opening precedence-hierarchy line — `loom's config section > the module's own config (e.g. builder.yaml) > (unset)` — sits outside the worked example and must change too: the parenthetical becomes `(e.g. webster.yaml)`.
  Then the worked example itself: the question becomes which effort webster's `master` role runs at, and the three-line yaml block becomes `models.yaml` (sonnet defaults `effort=medium`), `webster.yaml: master: sonnet[effort=high]`, and `loom config: webster: { master: sonnet }`.
  Update the two paragraphs that follow so they name `webster.yaml`'s discarded `effort=high` rather than `builder.yaml`'s.
  Webster has two roles where builder had four, so the example's shape changes, not just its names — do not invent extra roles to preserve the old structure.
  In the "Roles that use this notation" section, delete the two sentences describing `builder.yaml`'s four roles and the absent builder `evaluator`, keeping the `webster.yaml` sentence (`master` and `recovery`) as the section's opening.
  In "What is *not* a parameter", the `context` bullet cites "builder's `implementer_oversized`" — webster has no analogue, because its in-session forks inherit Master's model.
  Reword it generically ("a role that needs a large window points at a model/variant that *has* one") rather than substituting a webster role.
- **Commit:** `docs(model-spec): rebuild the worked example on webster.yaml`

### Card 14: Move the roadmap past builder

- **Context:**
  - `manifest/designs/shed-followups.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Delete the shipped-list **builder** item ("batch-implementation loop over a pinned plan … stays frozen and functional in-tree, with deletion tracked as a separate later task") in full — the planned deletion it points at is this task, so the item completes by leaving the list.
  In the open CLI-wording question, drop `buildercli` from the `buildercli`/`perchcli`/`webstercli` list.
  In the follow-up-chain summary, rewrite task A's parenthetical so it no longer spells the deleted package names — keep the task slug `builder-retire`, and describe it as retiring the superseded batch-implementation loop and adding `webster-contract.md`;
  update the rest of the summary so it matches the decisions this task actually shipped, including the phase rename.
  In the `PATTERN.md` item, drop the builder implementer from the code-touching template list and change "all five code-touching templates" to four.
  In the raddle item, rename the "deferred phase slot between Builder and Finalize" to name Webster;
  this card changes the phase **word** only — the slot's semantics remain a later task's obligation, and card 17 records that split.
  In the webster-rewrite item, delete the trailing sentence "`builder` becomes obsolete as a plan-format consumer".
  In the plan-format-v3 item, rewrite the coexistence sentence: v2 is gone, so state that v3 is the live plan format and drop the `plan-format.md` link rather than retargeting it.
- **Commit:** `docs(roadmap): retire the builder item and rename the Builder phase`

### Card 15: Sweep builder from the design docs

- **Context:**
  - `docs/reference/webster-contract.md`
- **Edits:**
  - `docs/reference/plan-format-v3.md`
  - `manifest/designs/hardener.md`
  - `manifest/designs/raddle.md`
  - `manifest/designs/review-finding-classification.md`
  - `manifest/designs/self-report.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/designs/raddle.md`, rename the phase word in all four sites — the Status banner's "reserved-but-unbuilt phase slot between Builder and Finalize", "an always-run step after Builder", "Running it right after Builder", and the Open-question sentence's "reserved phase slot between Builder and Finalize" — to name Webster.
  Change the phase word only;
  a later task rewrites this document's contract framing.
  In `manifest/designs/hardener.md`, change the phase list "Discussion/Plan/Builder" to "Discussion/Plan/Webster".
  In `manifest/designs/self-report.md`, change "builder/webster implementer fork" to "webster implementer fork".
  In `manifest/designs/review-finding-classification.md`, collapse the now-duplicated format pair: drop the deleted v2 plan-format doc's path from the parenthesised list of lyx's own formats, and change the numbered item heading `**plan-format.md** / **plan-format-v3.md**` so it names the single surviving plan-format doc.
  In `docs/reference/plan-format-v3.md`, the "Coexistence, not replacement" callout asserts that v2 "stays live and valid — the frozen `builder` still parses it" and retires "only when `builder` is deleted".
  That claim is false the moment this task lands, and it is a v2-coexistence-prose site, so make the minimal edit that stops it being false: state that v3 is the live plan format now that its predecessor is retired.
  Leave the `[plan-format.md v2](plan-format.md)` link dangling — the file returns under a later task — and do not rewrite the section in producer-model terms, which is another task's job.
- **Commit:** `docs(designs): rename the Builder phase and drop builder module references`

### Card 16: Retire the builder sandbox suite from the operator docs

- **Context:**
  - `tools/sandbox/SANDBOX-CORE-SUITE.md`
- **Edits:**
  - `docs/sandbox-howto.md`
  - `docs/sandbox-hub.md`
  - `docs/skills.md`
  - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `docs/sandbox-howto.md`, drop the deleted builder-suite launcher script from the launcher list, delete the whole `### 4e. Run the builder suite` section including its fenced command and its explanatory paragraph, and delete the `SANDBOX-BUILDER-SUITE.md` See-also bullet.
  `4e` is the last lettered step in the series, so no renumbering follows.
  In `docs/sandbox-hub.md`, rename the phase word in "Phased runs (Setup → Discussion → Plan → Builder → Finalize)" to Webster.
  In `docs/skills.md`, rename "loom Builder phase" to "loom Webster phase", and reattribute the three "builder/fixer prompt template" mentions to webster's fork prompt template — the file this task deletes no longer backs them.
  In `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`, rewrite every sentence that defines webster against builder or points at the deleted suite: the opening's "mirroring `SANDBOX-BUILDER-SUITE.md`'s own operating model" and "`webster` is builder's fork-based sibling", "two scenarios, not builder's nine", `lyx builder` in the wired-worktree prerequisite list, "mirroring the reed/shuttle/burler/builder suites'", "the way builder's separate implementer strands do", the scope note "builder's batch-loop scenarios stay in `SANDBOX-BUILDER-SUITE.md`", and "deliberately narrower than builder's own".
  Each becomes a statement about webster on its own terms;
  the scope note simply loses its builder clause, since the suite it names no longer exists.
  Do not touch this file's `plan-format-v3` mentions — a later task owns that rename.
- **Commit:** `docs(sandbox): retire the builder suite from the operator docs`

## Batch Tests

`verify: null` — every card here is documentation with no runnable surface, and nothing in the repo parses these files.
The one Markdown-reading test, `cmd/lyx/sandbox_coverage_test.go`, scans `tools/sandbox/*SUITE.md` for `**Covers:**` tags;
`SANDBOX-WEBSTER-SUITE.md` is edited here but its `**Covers:** webster` tag is untouched, and that guard runs again at batch 5's full suite.
The batch is otherwise verified by the module-wide `go build ./...` at the batch boundary and by batch 5's acceptance greps, which are what actually prove the phase rename and the module-word sweep are complete.
