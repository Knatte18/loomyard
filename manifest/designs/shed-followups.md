# Shed producer-model rewrite — follow-up task bodies

The Planned `Shed` roadmap item's flat producer-list model landed in `shed.md` and `loom.md`, but the documentation set that describes it only partly converted, leaving residue and real contradictions rather than merely stale text.
The `shed-producer-model-scoping` task (2026-08-09) surveyed that residue and broke the remaining reconciliation work into six follow-up tasks, each tracked in the mill wiki under its own slug — `builder-retire`, `plan-format-drop-v3-suffix`, `format-docs-name-producers`, `raddle-finalize-fold-and-link-repair`, `shed-model-contradiction-sweep`, `batcher-standalone-split` — but the wiki task bodies now only summarize; this file is the durable, versioned source of truth for what each task must do.
This doc is that scoping task's actual deliverable, extracted from the mill wiki into `manifest/` so the decided work survives independent of the wiki.

Dependency chain: `A → B → {C, F} → E`, with `D` branching off `A` in parallel.
`loom.md` has three owners in strict chain order — `B` (mechanical, paths/names only) → `C` (table rows 2–7) → `E` (everything else, final).
`manifest/roadmap.md` has two owners in chain order — `A` then `E`.
`docs/overview.md` has three owners in chain order — `A`, `B`, then `E` as last owner.

## A — builder-retire

# builder: delete internal/builderengine and internal/buildercli, retire builder-contract.md as a reference

### Why

Builder is not dormant: `cmd/lyx/main.go:107` registers `buildercli.Command()`, and it appears in `cmd/lyx/helptree_test.go`'s module list and `cmd/lyx/notransients_test.go`.
Parking it in-tree therefore costs real, recurring maintenance: it stays in the CLI help tree per the **CLI / Cobra Invariant**, keeps a second plan parser alive against the **Planparser Sole-Parser Invariant**, and every future refactor must carry it.

The genuinely reusable asset is the *design*, not the code — the recovery ladder, chain rollback, the `mutate.lock` state-mutation lease, the three fabric-commit points, and crash/resume semantics — and that already lives in `builder-contract.md`'s 247 lines, rewritable onto the flat card list later.
The implementation itself stays one `git show` away, permanently.

`manifest/roadmap.md:196` already calls `builder` "superseded as an active plan-format consumer" and `:202` says it "becomes obsolete" — nobody had removed it.

**Rejected alternatives:**

- Keeping the code frozen in-tree — pays help-tree, test, and refactor cost indefinitely for a reference binary.
- Moving it to a `sandbox/` or `attic/` directory — invents an excluded-directory convention this repo does not have.

### What needs to happen

1. **Code deletion.**
   - Delete `internal/builderengine` and `internal/buildercli` entirely.
   - `cmd/lyx/main.go` — unregister `buildercli.Command()` (`:107`) and drop `builder` from the module list in the long help text (`:75`).
   - `internal/configreg/configreg.go` — drop the `{Name: "builder", Template: builderengine.ConfigTemplate}` entry (`:44`) and its import (`:10`); update `internal/configreg/configreg_test.go:17`'s expected module list.
   - `internal/configcli/configcli_test.go` — `:311`, `:327–328` assert the config menu prints `builder (default)`, and `:455` notes builder is deliberately unseeded.
     Dropping `builder` from `configreg` fails this test; it is a second, less obvious consequence of the same one-line registry edit.
   - `cmd/lyx/helptree_test.go` — lines 28 and 106–107.
   - `cmd/lyx/notransients_test.go` — the import (`:21`) and the two `builderengine.Dir`/`ReportsDir` cases (`:57–58`).
   - `cmd/lyx/constructoranchoring_test.go` — the import and its builder assertions.
   - `cmd/lyx/rawgitmutation_test.go` — the `internal/builderengine` half of `TestNoRawGitMutation_WebsterBuilderProductionSource`.
   - `internal/scoutcli/cli_test.go`, `internal/webstercli/cli_test.go`, `internal/webstercli/sync_integration_test.go` — builder references.
   - `internal/webstercli/cli.go:11–12` — a doc comment comparing websterCLI to buildercli.
   - `tools/sandbox` — `suite.go`'s `//go:embed SANDBOX-BUILDER-SUITE.md` (`:47`), the `builderSuite` spec (`:123–128`), the `"builder-suite"` case in `main.go:326`, the doc comments in `suite.go:2` and `main.go:6,12`, and the `SANDBOX-BUILDER-SUITE.md` file itself.
   - `CONSTRAINTS.md` — review `:97` and `:106`, which list `builderengine` among feature packages.

**The `**Covers:** builder` trap.**
`tools/sandbox/SANDBOX-CORE-SUITE.md:224–232`'s scenario S9 "Builder plan validate/status", including its `**Covers:** builder` tag at `:229`, must go.
`cmd/lyx/sandbox_coverage_test.go`'s drift guard hard-fails on a `**Covers:**` token naming a module that is no longer registered, so leaving S9 in place breaks the build even after every other builder site is gone.

**The dangling-anchor trap.**
The four deep links into `builder-contract.md`'s "Webster: the fork-based sibling" section — `manifest/designs/finalize.md:36`, `finalize.md:50` (Related), `docs/overview.md:268`, and `docs/reference/plan-format-v3.md:343` — must all be re-pointed by this task **before** task B runs.
B's zero-hit grep for `plan-format-v3` cannot catch a dangling `builder-contract.md#…` anchor, so nothing downstream would find it.

**The inert-builder.yaml trap.**
Removing `builder` from `configreg`'s module list means `lyx config reconcile` stops emitting `builder.yaml`.
Existing `builder.yaml` files in already-created worktrees are left in place — they are inert once no module reads them, and reconcile does not delete files it no longer owns.
This task states this so nobody files it as a leak.

2. **Doc retirement.**
   - Extract `builder-contract.md`'s `## Webster: the fork-based sibling` section (`:222`) into `internal/websterengine`'s package doc.
   - Re-point all four deep links into that section, per the dangling-anchor trap above.
   - Re-status `builder-contract.md` as a retired-design reference.
   - Delete `docs/reference/plan-format.md` (v2).

     **Override recorded 2026-08-09 (task A, as landed).**
     The "re-status as a retired-design reference" instruction above did not hold either: task A deleted `docs/reference/builder-contract.md` outright rather than keeping it in place with a retired-status banner, and created `docs/reference/webster-contract.md` as webster's own consumer-facing contract in its place — a new file, not a rename.
     `webster-contract.md` carries webster's live cross-module contract prose; it does not attempt to restate `builder-contract.md`'s recovery ladder, chain-rollback design, or the `mutate.lock` state-mutation lease material — none of that transferred anywhere in-tree.
     That material is recoverable only from git history, one `git show`/`git log -- docs/reference/builder-contract.md` away, exactly as the "genuinely reusable asset is the design, not the code" framing above anticipated for the *implementation*, now extended to this doc as well.
     Any downstream task that expects a retired-but-present `builder-contract.md` on disk will not find one.
   - `discussion-format.md:30` — its justification for `plan-format`'s `approved:` field reads "because `lyx builder run` can be invoked standalone, outside loom", which is false once this task lands; `discussion-format.md:3` links `plan-format.md`.
     Both belong to this task, since this task is what falsifies them.
   - `docs/overview.md` — all builder and plan-format references, not the module table alone: `:92` (lists both `plan-format.md` and `plan-format-v3.md` as kept reference docs — only one survives), `:227` (the `internal/pattern` tree comment naming builder as a consumer), `:264` (the `builder` module-table entry), `:265` (the webster entry, defined as "fork-based sibling of builder"), `:268` (the deep link, above), `:292` (names "builder implementer" among `internal/pattern`'s prompt consumers — the same phrase this task also owns at `roadmap.md:42`), and `:375` (the `builder-contract.md` see-also).
   - `README.md` — `:25` lists `lyx builder` in the subcommand tree, `:86` is the `builder` module bullet, `:87` defines webster as "a fork-based sibling of `builder`", `:94` asserts builder "stays frozen in-tree as the plan-format-v2 consumer" (directly falsified by this task), and `:115` describes builder's place in the module topology.
   - `docs/sandbox-howto.md` — `:8`'s launcher list, `:141–147`'s "Run the builder suite" section, and `:190`'s `SANDBOX-BUILDER-SUITE.md` see-also.
   - `sandbox/builder-suite.cmd` — delete it.
     It invokes the `"builder-suite"` case this task removes from `tools/sandbox/main.go:326`, so it is an orphan by construction, not an independent decision.
   - `.gitattributes:7–9` — the three `internal/builderengine/*` `text eol=lf` pins (`implementer-template.md`, `orchestrator-template.md`, `template.yaml`), all pointing at files this task deletes.
   - **Comment-only residue, swept opportunistically** (none of it breaks the build, all of it reads as stale once builder is gone): `internal/perchengine/doc.go:13` ("builder-review"), `internal/modelspec/modelspec.go:7,35` ("builder's roles", "builder, perch/burler/loom configs"), `internal/loomengine/configtemplate.go:4` ("mirroring builderengine's ... embed-and-accessor").
   - `manifest/roadmap.md` — the Done `builder` item (`:196`, `:202`) and `:42`'s "builder implementer" template mention.
   - `docs/reference/status-schema.md` — its builder-specific prose (`:16`, `:53`, `:69`, `:73`, `:81`) and the `builder-contract.md` link at `:3`.
     **The `phase` enum itself is deliberately NOT this task's** — see the standalone note below on the deferred phase enum.

3. **Comment-only residue and v2-coexistence prose class.**
   Sweep the comment-only residue listed above opportunistically.

**The v2-coexistence prose class.**
Once this task deletes v2 and task B reuses the filename, every surviving "plan-format v2" link silently re-targets v3 content — a worse failure than a dangling link, because nothing breaks.
This task owns every site whose claim it itself falsifies: `docs/reference/builder-contract.md:3`, `:7`, `:224` ("until then it stays frozen and fully functional in-tree"), `:243`; `docs/reference/model-spec.md:3` ("Pinned alongside [plan-format v2] and the emerging [v3]"); `docs/reference/status-schema.md:3`; `manifest/roadmap.md:207` ("Coexists with the still-live plan-format v2 — still used by the frozen `builder`"); and `manifest/designs/review-finding-classification.md:7`, `:47` — where task B's sweep would otherwise turn a v2/v3 pair into "plan-format.md / plan-format.md".

**Instruction repaired 2026-08-09 (task A, as landed).**
This list's first four sites — `docs/reference/builder-contract.md:3`, `:7`, `:224`, `:243` — named lines inside a file this task deletes outright rather than retiring in place (see task A's own override above); those four lines no longer exist anywhere to edit, and no successor file inherited their v2-coexistence claims.
They are struck from this ownership list as satisfied-by-deletion, not left open.
The remaining members of the class — `docs/reference/model-spec.md:3`, `docs/reference/status-schema.md:3`, `manifest/roadmap.md:207`, and `manifest/designs/review-finding-classification.md:7`/`:47` — are unaffected by this repair and remain this task's under the same ownership rule.

### What this task does not own

The `phase` enum in `internal/loomengine/coherence.go:14–22`'s `validPhases` map and its twin in `docs/reference/status-schema.md` — both currently `preflight | discussion | plan | builder | raddle | finalize | done` — are deliberately left alone by this task and the rest of the follow-up set.
Realigning them lands with the `Shed` build task.
The flat producer list replaces the phase enum rather than editing it; rewriting the enum now would mean inventing an interim phase set that `Shed` would immediately discard — churn on a pinned contract and live validation code, to no end.
The enum is not wrong today: it describes the machine that exists.

**What is not deferred:** `status-schema.md`'s builder-specific prose and its `builder-contract.md` link go stale the moment this task lands, so those are this task's, per the "Doc retirement" list above.
Only the enum itself waits.

**Override recorded 2026-08-09 (task A, as landed).**
The deferral above no longer holds.
Task A renamed the `phase` enum in both `internal/loomengine/coherence.go`'s `validPhases` map and `docs/reference/status-schema.md`'s twin from `builder` to `webster`, in place, rather than leaving either untouched for `Shed` to realign.
An enum entry naming a module task A itself deletes is not a neutral interim state the way an unedited-but-still-correct enum would be — leaving `"builder"` in live phase validation after `internal/builderengine` no longer exists means the validator accepts a phase word for a module gone from the tree, which is strictly worse than the churn the original deferral was trying to avoid.
That `Shed` will later replace the whole phase enum with a flat producer list is not a reason to ship a wrong value in the meantime; the deferral's premise (rewriting now means inventing an interim phase set `Shed` would discard) does not apply to a same-name rename, only to a genuinely new interim vocabulary.
Both files now read `webster` wherever this section previously said `builder`.
`Shed`'s own realignment work is otherwise unaffected — it still replaces the enum with the flat producer list; it simply does not do so starting from a `builder` value.

### Scope

This task is one task producing one compiling commit, because a package deletion is atomic by nature and splitting it guarantees an intermediate state that does not build.

This task's ownership rule for the v2-coexistence prose class is: it owns every site whose claim it itself falsifies.
Two exclusions from that rule: `plan-format-v3.md:5`'s own "Coexistence, not replacement" section belongs to task C, and `loom.md:29` belongs to task E.

`manifest/roadmap.md` has two owners, this task then task E, in chain order rather than concurrently.

### Sequencing

`depends_on:` nothing — nothing blocks this task.

Two tasks depend on this task:

- Task B, because the `plan-format.md` filename is not free until this task deletes v2's doc.
- Task D, because `finalize.md:36` and `:50`'s link targets move in this task.

### Acceptance

The existing suite is the test.
`go build ./...` and `go test ./...` must pass with `builderengine` and `buildercli` gone.
No new tests are written.

Four guards fail loudly on a half-removal, and this task should expect to be driven by them:

- `cmd/lyx/helptree_test.go`
- `cmd/lyx/notransients_test.go`
- `internal/configreg/configreg_test.go:17`'s module-list assertion
- `cmd/lyx/sandbox_coverage_test.go`'s `TestSandboxCoverage_AllModulesCoveredOrExcluded`

This task must satisfy four `CONSTRAINTS.md` invariants:

- **CLI / Cobra Invariant** — remove `builder` from the help tree cleanly, not orphan it.
- **Planparser Sole-Parser Invariant** — this task's deletion of `builderengine` is what finally makes this literally true.
- **Sandbox Suite Coverage** — this task trips it by removing a registered module, and must delete both `SANDBOX-BUILDER-SUITE.md` and `SANDBOX-CORE-SUITE.md`'s S9 `**Covers:** builder` tag.
- **Fabric Git Invariant (warp + weft)** — its Enforced-by block at `CONSTRAINTS.md:205` machine-checks module ownership for `internal/websterengine`/`internal/builderengine` via `cmd/lyx/rawgitmutation_test.go`'s `TestNoRawGitMutation_WebsterBuilderProductionSource`; narrow that clause to webster alone.
  `:205` sits inside this invariant, which begins at `:173` — not inside the Review Round Invariant, which begins at `:209`.

This task must also satisfy the **Documentation Lifecycle**, which governs the extraction of the Webster section into `internal/websterengine`'s package doc before `builder-contract.md` is retired.

## B — plan-format-drop-v3-suffix

# plan-format: drop the v3 suffix and sweep every reference by script

### Why

`v3` is the only live format once builder is gone, and a version suffix on the sole format is exactly the kind of stale guard `discussion-format.md` already argues against, via its `no-schema-version` reference to `status-schema.md`.

A half-done rename is worse than either end state, because `planparser` and `websterengine` identifiers and template prose must move with the filename.

**Rejected alternatives:**

- A docs-only rename with Go identifiers deferred — leaves the codebase mid-rename.
- Renaming the file but keeping in-text "v3" as a historical label — the suffix is exactly what is being retired.

### What needs to happen

1. This task's first action re-derives the affected file list by grep rather than trusting any count written down beforehand.
   Do not trust a file count written anywhere else — this step is what bounds the list.

2. The affected clusters, as a starting inventory only, are:
   - `internal/planparser`
   - `internal/websterengine`
   - `internal/webstercli`
   - `internal/loomengine`, including `plan-template.md`
   - `internal/batcher/doc.go`
   - `docs/overview.md`
   - `docs/reference/model-spec.md`
   - `docs/reference/webster-contract.md`
   - `manifest/roadmap.md`
   - several `manifest/designs/*.md` files
   - `tools/sandbox/SANDBOX-WEBSTER-SUITE.md`
   - `CONSTRAINTS.md`'s Planparser Sole-Parser Invariant, whose wording changes for the renamed format.

**Instruction repaired 2026-08-09 (task A, as landed).**
This inventory originally listed `docs/reference/builder-contract.md`; that path no longer exists, since task A deleted the file outright rather than leaving it as a retired reference (see task A's own override above).
The entry above has been replaced with `docs/reference/webster-contract.md`, task A's new consumer-facing contract doc, which is in this task's sweep scope: it links `plan-format-v3.md` twice (`:13`, `:48`) and therefore needs the same `v3`-suffix rename this task performs everywhere else.

#### Hard exclusion — `gopkg.in/yaml.v3`

The sweep must never touch that import token.
It appears in ten Go files, including `internal/planparser/parse.go:21` — the sole plan parser, i.e. the file this task is most certain to be editing.
A broad `v3` replace corrupts the import and breaks the build in the least obvious place.
This task's script names the exclusion explicitly rather than relying on the pattern set being narrow enough.

#### Execution discipline

A scripted find/replace followed by a full `go test ./...`, never a hand-edit pass.
Per this repo's own tooling rules the script must not use `sed`.

3. Record the deliberate window between task A and this task where `docs/reference/plan-format.md` does not exist at all: task A deletes v2 to free the name, this task re-creates it from v3.
   Links to `plan-format.md` dangle in between, by design and briefly.

4. Record what this task deliberately leaves broken: this task's rewrite of `loom.md:29` knowingly leaves that sentence self-contradicting, because a pure find/replace cannot repair an argument about two formats when only one survives.
   This is accepted, not overlooked — this task's grep criterion passes while the sentence reads wrong, and task E repairs the prose as `loom.md`'s final owner.

### Scope

This task's position in `loom.md`'s three-owner chain is B → C → E: this task is the mechanical owner, because its zero-hit criterion necessarily rewrites `loom.md:29` and table rows 5–7 at `:53–55`, which spell `plan-format-v3.md`.

This task changes paths and names only, never prose.

**Override recorded 2026-08-09 (task B, as landed).**
Record every one of this task's own instructions it departed from, since tasks C and E read this file rather than task B's now-torn-down state.

1. The stated five-pattern set became six.
   The five missed the doc title's space variant `# Plan format v3`, so the stated zero-hit criterion would have passed with the renamed doc still titled v3.
2. The unqualified "repo grep" became a grep with exactly one file-level exclusion — this file — because its `### Acceptance` sentence naming the pattern set is itself a pattern-bearing line, and a blind sweep destroys the criterion it defines.
   Both halves of what that means, since they are easy to conflate:
   - The sweeper additionally skipped line 18 of `manifest/roadmap.md`, whose "`plan-format-v3.md` → `plan-format.md`" would have collapsed to a self-referential no-op.
     That skip was temporary: task B rewrote the line by hand so it names no version, and the final acceptance grep carries no roadmap exclusion at all.
   - This file is the sole permanent exemption.
     Its citations of the doc's pre-rename path, and its other references to the format by the old name, survive on purpose.
     A verified count, not a remembered one: `grep -c 'plan-format-v3\.md' manifest/designs/shed-followups.md` returns **six** once this whole block is written — five citations predating this override plus the one two sentences above (the `manifest/roadmap.md` mention) — do not carry forward the "four" that appears in this task's discussion, whose tally silently omitted one occurrence, because this note is a durable record tasks C and E will read.
     The file is a historical record of what each task was told at scoping time, and those citations were accurate at that moment; rewriting them would make the record claim the scoping task knew the post-rename name.
     A reader who follows one of them will not find the file — this note is where they learn it moved to `docs/reference/plan-format.md`.
3. "This task changes paths and names only, never prose" is superseded.
   The repo-wide v2 erasure rewrote prose in `docs/reference/plan-format.md`, `manifest/designs/loom.md`, `manifest/roadmap.md`, and Go comments across three packages.
4. The `### Why` subsection's rejected alternative — "renaming the file but keeping in-text `v3` as a historical label" — was honoured rather than overridden, and extended: the four bare-`v3` labels in `internal/planparser` comments were rewritten too.
   This is recorded explicitly because it is a rejection rather than an instruction, and a reader could otherwise conclude task B left the class alone.
5. The `### Sequencing` claim that this task "deliberately leaves `loom.md:29` self-contradicting" no longer holds — task B rewrote that line in full.
6. The starting inventory's claim that `CONSTRAINTS.md`'s Planparser Sole-Parser Invariant needs rewording is stale.
   The invariant carries no version reference and no link to the doc, and the file's only `v3` occurrences are its two `gopkg.in/yaml.v3` import-allowlist entries, which are the hard exclusion.
   Task B edited nothing there.
7. The `#### Hard exclusion` subsection's claim that the `gopkg.in/yaml.v3` import "appears in ten Go files" is wrong — the verified count is **32**.
   The figure is corrected here so the next reader is not misled about the blast radius of a broad `v3` replace.
8. The same subsection's claim that "This task's script names the exclusion explicitly" is wrong — the script names no exclusion.
   All six patterns require a `plan` prefix, so the import string is unmatchable by construction; the exclusion is verified by a post-sweep count rather than implemented.

Because this task runs before both C and E, this is chain order rather than concurrency — no two owners hold the file at once.

### Sequencing

`depends_on: builder-retire` — the filename is not free until v2's doc is deleted.

Two tasks depend on this task: task C and task F, because both edit the renamed file.

### Acceptance

The completion criterion is the discussion's own case-insensitive repo grep returning zero hits for the full pattern set, plus a passing `go test ./...`.
The pattern set is: `plan-format-v3`, `plan_format_v3`, `plan-format v3`, `plan-v3`, and `Plan-format v3`.

The narrower three-pattern set was rejected because it would leave `loom.md:58`'s "plan-v3's card contract", `loom.md:94`'s "Webster/plan-v3 equivalent", and `internal/planparser/doc.go:32`'s "Plan-format v3" all passing, contradicting the decision's own intent.

`internal/planparser`'s existing tests and `internal/webstercli/cli_test.go` cover behaviour preservation.
The meaningful failure mode is incompleteness, checked by grep rather than by an assertion in a test file.

## C — format-docs-name-producers

# format docs: name their producers and contracts in producer-model terms, add Discussion-Review-Gate

### Why

`discussion-format.md` and the renamed `plan-format.md` are the two pinned contracts the flat producer model points at — every Input/Output cell in `loom.md`'s producer table is supposed to be a pointer into one of these two files, never a restated copy.
Both files still describe themselves in pre-producer terms, so the pointer rule they are meant to anchor currently has nothing coherent to point at.

**`loom-table-names-real-artifacts`.**
`loom.md`'s producer table currently names two artifacts that exist nowhere in the pinned contracts: `discussion.md` and `plan.md`.
The real artifacts are `_lyx/discussion/decision-record.md` (not `discussion.md`) and the `_lyx/plan/` directory (not `plan.md`), and the two-file access boundary becomes part of the `Plan-Write` Input pointer.
A producer table whose Input/Output pointers name nonexistent files defeats the pointer rule it is meant to demonstrate.
This was not left open for this task to re-derive — the real paths are already pinned by `discussion-format.md` and by `loom.md:188`'s own statement that the Planner writes `_lyx/plan/NN-<card>.md` per card plus `00-overview.md` as the done-sentinel.

### What needs to happen

1. Rewrite `discussion-format.md` and the renamed `plan-format.md` to name their producers and contracts explicitly in producer-model terms.
2. Add the `Discussion-Review-Gate` producer, covering checks 1–2 of `discussion-format.md:80–82`.
   See the dedicated subsection below for its full rationale.
3. Scoped-edit `loom.md`'s table rows 2–7 to name the real artifacts — `_lyx/discussion/decision-record.md` and `_lyx/plan/` — and insert `Discussion-Review-Gate` into the producer list.
4. Fix `discussion-format.md:1`'s own title, which still reads "the `discussion.md` ↔ Plan contract" — the same nonexistent artifact the `loom.md` table named.
5. Restate `discussion-format.md:14` in producer-model terms.
   It currently grounds the two-file split in "Builder's 'distilled digest, never raw prose' rule (see `builder-contract.md`'s digest contract)" — a live contract justified by a retired design doc.
   The rule itself is sound and stays; only its attribution is rewritten.

   **Instruction repaired 2026-08-09 (task A, as landed).**
   `builder-contract.md`'s "digest contract" no longer exists to ground the citation in — task A deleted the file outright, and no digest-contract section survived into `docs/reference/webster-contract.md`.
   The rule is grounded elsewhere in the live tree instead: `internal/websterengine`'s own package documentation (`doc.go`) states the distilled-`Digest`-persisted-at-terminal contract directly (see also `recordbatch.go`'s `RecordResult.Digest` handling), and `docs/overview.md:60` independently states the same "Go-distilled digests, never raw prose" rule at the architecture level.
   Task C should re-ground `discussion-format.md:14`'s citation in one or both of those live sources, not in the deleted file.
6. Rewrite `plan-format.md:5`'s "Coexistence, not replacement" section, which asserts the format does not retire v2.
   That claim is false once task A deletes v2, so the renamed file would otherwise carry the claim forward about itself.

   **Override recorded 2026-08-09 (task B, as landed).**
   This section is already gone: task A rewrote it away, and task B then deleted the surviving retired-v2 blockquote outright rather than leaving prose behind for this task to rewrite.
   This task's obligation here is discharged;
   task C's remaining work on `plan-format.md` is the producer-model rewrite only.

#### The `Discussion-Review-Gate` producer

**`discussion-review-gate-exists`.**
The `Discussion` side is not inherently asymmetric — scope a `Discussion-Review-Gate` mechanical producer, mirroring `Plan-Review-Gate`.
It runs checks 1–2 of `discussion-format.md:80–82`: both files exist under `_lyx/discussion/`, and `decision-record.md` has all seven required sections present (Goal, Scope, Decisions, Constraints, Auto-mode assumptions, Open risks, Acceptance criteria).
Both are per-run, artifact-observable properties of exactly the kind `Plan-Review-Gate` already hard-fails on, and both are already written down — this task names them as a producer, it does not design anything new.

**Check 3 is not a gate check.**
`discussion-format.md:83`'s claim — that the Plan producer's declared input set never names `support-log.md` — is a property of the producer *definition*, not of any run's artifacts.
There is nothing per-run for a gate to evaluate.
It becomes a build-time test assertion over the producer definition instead: a static property caught once and forever by a compile- or test-time guard, rather than re-evaluated on every run.
This task's body states this explicitly, in so many words, so nobody re-files it later as a missing gate check.

**`discussion-stays-two-files-with-current-names`.**
`_lyx/discussion/` stays a two-file directory: `decision-record.md` (the Plan producer's sole input) and `support-log.md` (read only by the Discussion-review gate).
`decision-record.md` is **not** renamed.
`discussion-format.md:16` states the filenames are self-describing on purpose, `decision-record.md` pairs with `support-log.md`, and the file holds seven sections, so naming it after one of its own sections would mislead.
Rejected alternatives: `decisions.md` and `decision.md` — both terser, both lose the sibling parallelism, and both would force a code sweep across `DiscussionDecisionRecord` in `internal/loomengine/config.go`, `discussionpath_test.go`, `discussion_test.go`, and `prompttemplate.go`, for no contract gain.
Note: the sourcing discussion cites this sentence as `discussion-format.md:15`, but the self-describing-filenames sentence is actually at `:16` — `:15` is the preceding sentence about the filesystem boundary.
This task uses `:16`; the off-by-one from the discussion is not propagated into this body.

**The symmetric "what NOT to look for" rule.**
Per `review-finding-classification.md` item 5, a "what NOT to look for" instruction must be written symmetrically into both the producer's own format-contract and the reviewing producer's rubric.
Writing it into only one side recreates the non-convergent review loop that doc exists to prevent.
This task honours that rule when writing the `Discussion-Review-Gate`'s rubric: whatever the gate is told not to look for must also be written into `discussion-format.md` itself, not only into the gate's own instructions.

### Scope

This task owns `loom.md`'s producer table rows 2–7 only — the artifact-name fixes and the `Discussion-Review-Gate` insertion.
Task E owns everything else in `loom.md` and runs after both this task and task F.

### Sequencing

`depends_on: plan-format-drop-v3-suffix` — this task edits the renamed file.

Task E depends on this task, so that E writes `loom.md`'s finished table state rather than guessing at it.

### Acceptance

Docs-only; this task has no test surface of its own.

The `Discussion-Review-Gate`'s checks are specified here, not implemented — implementation lands with `Shed`.
Check 3's build-time assertion is likewise specified rather than written, since the producer definition it would assert over does not exist yet.

Every relative markdown link and anchor introduced or touched by this task must resolve.

## D — raddle-finalize-fold-and-link-repair

# finalize: fold Raddle into its own contract and repair the dead links in raddle.md, finalize.md and self-report.md

### Why

The landed model folds Raddle-regeneration into `Finalize`'s own contract, rather than keeping it as a step of its own.
`finalize.md` and `raddle.md` still describe a machine that no longer exists.
The same files also carry three separate dead references, which is what makes this a repair task rather than a prose pass: a link to a design doc that does not exist, a renamed anchor that no longer resolves in three places, and a citation to a `CONSTRAINTS.md` invariant that was never named that.

### What needs to happen

1. Fold Raddle into `finalize.md`'s own contract as a first-class part of the merge, not a Related-section mention.
2. Remove `raddle.md`'s superseded "reserved phase slot between Builder and Finalize" text at lines 3 and 85, and close its explicitly-open question at line 54 — the fold is decided.
3. Fix `finalize.md:3`'s verbatim two-slot text ("not a swappable per-instance slot the way Preflight and the producer are").
4. Fix `finalize.md:11` and `:52`, which link `fabric.md` — a file that does not exist in `manifest/designs/`.
5. Fix the dead `loom.md#the-phase-machine` anchor, renamed to `#the-phase-machine--a-flat-producer-list-no-predefined-slots`, in `raddle.md:3`, `raddle.md:54`, and `self-report.md:30`.
6. Fix `finalize.md:26`, which cites a "Weft Git Invariant" in `CONSTRAINTS.md` that does not exist — the real entry is the Fabric Git Invariant (warp + weft) at `CONSTRAINTS.md:173`.

**This task re-reads `finalize.md` end to end**, rather than working the fixed line list above — the line numbers are a starting inventory, not a bound.
Known additional residue the discussion already found:

- `:45–46` still calls Finalize "`Shed`'s literally-shared code ... both share this exact code", which is the retired shared-code framing.
- `:48` asserts "`Shed` hasn't been extracted from it yet (see that doc's own naming note)", which is false once task E fixes `loom.md:15–17`.
- `:9` references Builder's escalation behavior, which task A retires.

### Finalize is shared by reference

**`finalize-shared-by-reference`.**
`Finalize` is shared **by reference** — both `loom`'s and `Hardener`'s lists name the same producer definition: one definition, named twice, never copied.
This is the framing the fold is written against.
`shed.md:18`'s "by value" wording is task E's to fix, not this task's, so the two tasks do not both edit `shed.md`.

### Scope

This task deliberately does not touch `manifest/roadmap.md`.
`roadmap.md:68`'s "deferred phase slot between Builder and Finalize" is real residue, but `roadmap.md` is edited by task A and task E too, so scoping it to this task would recreate exactly the shared-file collision that forced task E to be serialized.
It moves to task E, `roadmap.md`'s last owner.

This task owns `finalize.md`, `raddle.md`, and `self-report.md`, and no other task in the set touches any of them — that is what makes this task genuinely parallel rather than parallel-by-assertion.

**Deferred, on record so it does not read as an oversight.**
Per the discussion's surfaced open questions, item 4: `Hardener` and `Tenter`'s equivalent Raddle-into-Finalize fold is deferred by the landed design at `shed.md:20` and `loom.md:67`, and stays deferred.
This task does not design it.

### Sequencing

`depends_on: builder-retire` — `finalize.md:36` and `:50`'s link targets move in task A.

This task branches off task A in parallel with the B → {C, F} → E chain; it does not block, and is not blocked by, any of C, E, or F.

### Acceptance

Docs-only.
The one mechanical check worth running is that every relative markdown link and anchor introduced or touched resolves — a link-check pass over `manifest/` and `docs/` is the acceptance criterion.
It is exactly what would have caught the dead `fabric.md` links, the dead phase-machine anchors, and the non-existent Weft Git Invariant citation before they shipped.

## E — shed-model-contradiction-sweep

# shed: sweep the remaining producer-model contradictions and add the pointer-rule invariant

### Why

**`deliverable-is-reconcile-the-residue`.**
`shed.md`, `loom.md`, and `roadmap.md` are the landed, authoritative statement of the model.
They were rewritten immediately before this scoping worktree spawned, on the same model this whole follow-up set applies — everything else reconciles *to* them, and never the reverse.

This task exists because the partial conversion left contradictions inside those same three files, plus a set of claims that only become false once tasks A through F have landed.
This task runs last and writes the finished state.

### What needs to happen

#### Part one — `shed.md`'s own contradictions

- `:7` and `:19` say "superseding ... **below**" and "the pre-revision text **below**", but that text was deleted in commit `256b8262`, so both references dangle.
- `:18` says Finalize is shared "by value"; it becomes "by reference" per the `finalize-shared-by-reference` decision — two of three sources already say by reference, and it is the phrasing that carries the actual meaning.
- `:13` enumerates `loom`'s producer list verbatim, and `:41` lists the mechanical Go-function producers.
  Both must gain `Discussion-Review-Gate` once task C inserts it into `loom.md`'s table, or the two docs silently disagree about what `loom`'s list contains.

#### Part two — the stale "this task is still pending" claims

This scoping task itself falsifies these claims, so this task retires them:

- `shed.md:63`'s claim that wiki task `shed-producer-model-scoping` is the dedicated pass that reconciles any remaining detail mismatch between the two docs.
- `loom.md:76`'s version of the same claim about the producer table.
- `loom.md:91–94`'s version of it.

#### Part three — `loom.md`'s remaining residue

This task is `loom.md`'s final owner, and owns everything in the file except the table rows task C already fixed:

- `:15–17`'s naming note, which still says "`loom` = `Shed` + loom's own Preflight + the Discussion/Plan/Webster producer" — old slot framing, contradicting the table 25 lines below it, and whose "This doc has not been rewritten to extract `Shed` explicitly" claim is now false.
- `:29`, which links the v2 `plan-format.md` that task A deletes and frames v3 as "the target format is changing" — task B's mechanical sweep deliberately leaves this self-contradicting for this task to repair.

  **Override recorded 2026-08-09 (task B, as landed).**
  Task B rewrote the line in full instead of leaving it self-contradicting — it now states the live plan format directly, with no v2 link and no "target format is changing" framing.
  This task's obligation on `:29` is to verify the rewrite rather than to repair it.
  E remains `loom.md`'s final owner for everything else on this list.
- `:91–94`, the naming note calling `internal/builderengine` and `internal/buildercli` "a real, separate, already-shipped sibling implementer loop", plus its `builder-contract.md` link.
- `:187`, the module-decomposition row repeating the same already-shipped-sibling claim and `builder-contract.md` link.
- `:56`, row 8's `Batchifier` entry, rewritten to match whatever task F landed.

  **Override recorded 2026-08-09 (task A, as landed).**
  The `:91–94` naming note and the `:187` module-decomposition row, both assigned to this task (E), were rewritten in full by task A instead of being left for E.
  Task A did this because its own package-name zero-hit criterion (`builderengine`, `buildercli`) reaches those two sites regardless of which task's ownership list they sit on — a bare-word or package-name grep does not respect chain-order assignment, and leaving those two names in place through E's turn would have failed task A's own acceptance gate.
  Both sites now describe `internal/websterengine`/`internal/webstercli` and link `webster-contract.md` instead of `builder-contract.md`.
  Everything else on this list — `:15–17`, `:29`, `:56`, and the rest of `loom.md` not named above — remains E's, and the B → C → E chain-order ownership for the file is otherwise unchanged: E still runs after B and C and still writes the file's finished state.

#### Part four — the other files

- `hardener.md:17`'s "producer-slot".
- `docs/overview.md:272`'s stale chain "Preflight → Discussion → Plan → Builder → Raddle → Finalize".
- `manifest/roadmap.md`, where this task is the last owner and therefore carries:
  - `:68`'s "deferred phase slot between Builder and Finalize", moved off task D.

    **Override recorded 2026-08-09 (task A, as landed).**
    Task A's `builder` → `webster` phase rename necessarily touched the word `Builder` on this line — it now reads "deferred phase slot between Webster and Finalize" — because the phase-token rename is repo-wide and this line names a phase word, not a task label.
    That is a **word**-level change only.
    The **semantics** this task (E) is assigned — deciding what actually fills that deferred slot, and whether the slot framing itself survives `Shed`'s flat producer list — remain entirely E's remaining roadmap obligation, unaffected by the rename.
    E should not find the line already reading `Webster` and conclude its obligation here has lapsed; the word changed, the open question did not.
  - the retirement of `:31`'s "**A dedicated scoping task should run first** ... this item is not yet broken down into buildable units" — stale the moment this scoping task lands.
    This task is the right place to declare the breakdown done and name the six follow-up tasks.

  **Override recorded 2026-08-09 (task B, as landed).**
  Record both roadmap edits task B made, so E does not go looking for either.
  B deleted the "v3 is the live plan format now that its predecessor is retired." sentence from the plan-format Done item, since B's own sweep of the item's heading is what made it incoherent.
  B also rewrote line 18's six-task breakdown parenthetical, which the sweeper had deliberately skipped, so it describes the rename instead of spelling both filenames.
  The task slug on that line is untouched.
  E's remaining roadmap obligation is unchanged by either edit.

**Note (2026-08-09): this last bullet has already been done by the scoping task itself,** ahead of task E, per an explicit operator override of the original plan (see `manifest/roadmap.md`'s Planned `Shed` item, already showing the six-task breakdown, and this file's own existence).
Task E should treat `roadmap.md:68`'s "deferred phase slot between Builder and Finalize" line as its remaining roadmap obligation and verify the breakdown text still matches reality by the time E runs, rather than re-writing it from scratch.

#### Part five — the two additions

- Resolve `loom.md:75`'s thin-Output question per `preflight-finalize-thin-output-is-permitted`, and record the resolution in `shed.md`'s producer-contract section: the Output contract permits a pass/fail gate signal with no artifact, because `Preflight` and `Finalize` genuinely have no output artifact, and the resume-on-output-files rule degrades gracefully — a producer with no artifact simply re-runs on resume, which is correct for both.
- Add the new short `CONSTRAINTS.md` invariant naming the pointer rule as a review obligation.
  See the dedicated subsection below.

**This task re-reads `shed.md` and `loom.md` end to end**, rather than working the line lists above — exactly as task D does for `finalize.md`.
`shed.md:63` sits inside a whole "Why this doc doesn't rewrite loom.md's full detail" section whose premise changes once tasks C and E have run.

### The pointer-rule invariant

**`pointer-rule-becomes-a-short-constraints-invariant`.**
Add a new, short `CONSTRAINTS.md` invariant naming the pointer rule as a review obligation: an instruction file must never duplicate or paraphrase another producer's format-contract content, only point at it.
It matches the file's existing seam-invariant precedent — Treadle Runner-Seam, Scout Engine-Seam, Shuttle Provider-Seam, Batcher Registry+Config.
It is enforced by review, not machine-checked.
It must be **short** — one invariant statement plus an "Enforced by: review obligation" line, in the same shape as the existing entries, not a treatise.

### Open questions

The following three questions are surfaced deliberately, per the discussion, and are **not** resolved by this task.

**Question 1 — `Webster` violates the producer-atomicity rule.**
The landed model states a producer is always atomic: one mechanical action, or one LLM session, never an internal multi-step process of its own.
But `loom.md:57` lists `Webster` as a black box with its own per-batch fork loop, opaque to `loom`'s flat list — precisely an internal multi-step process.
Either atomicity admits a carve-out for black-box producers that own their own loop, or `Webster` decomposes into flat producers the way `Plan` did.
This is the single largest unresolved tension in the model, to be decided before `Shed` is built rather than during.
**This task's obligation:** record it as a named precondition on `manifest/roadmap.md`'s Planned `Shed` item, not merely as prose in a design doc — recording it without gating it is how it gets skipped.

**Note (2026-08-09): this precondition has already been recorded** on `manifest/roadmap.md`'s Planned `Shed` item by the scoping task itself, ahead of task E (same override noted in Part four above).
Task E should verify the roadmap wording still accurately reflects the open question by the time E runs, rather than re-adding it.

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

### Scope

This task holds three ownership positions:

- `loom.md`'s final owner, after tasks B and C.
- `roadmap.md`'s last owner, after task A.
- `docs/overview.md`'s last owner, after tasks A and B.

This task writes the finished state rather than guessing, which is the whole reason it is serialized last.

### Sequencing

`depends_on: format-docs-name-producers, batcher-standalone-split` — this task must see task C's finished table, and it cannot write `loom.md` row 8 before task F has decided what row 8 says.

### Acceptance

Docs-only, the same criterion as task D: every relative markdown link and anchor introduced or touched resolves, via a link-check pass over `manifest/` and `docs/`.

## F — batcher-standalone-split

# batcher: split out of webster into a standalone configreg module with its own batcher.yaml

### Why

`Batchifier` is a `Shed`-level producer, position 8 in `loom`'s list, between `Plan-Review` and `Webster`, so batching is no longer webster-internal execution policy.

Two-step consequence: extract `batcher` as a standalone module now, and absorb the `Batchifier` producer into `loom` via `Shed` later, when the batchifier choice becomes part of `loom`'s producer-list configuration.

The key cannot go straight to `loom.yaml` today: both live `batcher.Select` call sites are webster's, and `Shed` — the thing that would own a `loom.yaml` batchifier key — does not exist, so making webster read `loom.yaml` would either break standalone `lyx webster run` or couple two modules' configs for no live benefit.

**Rejected alternatives:**

- Dropping `Batchifier` from `loom`'s producer list to preserve the shipped `internal/batcher` framing — would subordinate the newest decision to older doc text.
- Surfacing `Batchifier`'s position as unresolved — the user already resolved it.
- Moving the key straight to `loom.yaml` now — unworkable, since no `loom.yaml` reader exists.
- Leaving it in `webster.yaml` — contradicts the split.
- A `loom.yaml` key with `webster.yaml` fallback — a transition mechanism with no transition to serve.

### What needs to happen

1. **Module shape.**
   "Standalone" means a `configreg`-registered config module, not a `lyx batcher` command, because a batchifier has no user-facing verb.

   Two consequences, since the opposite was briefly assumed:
   - The CLI / Cobra Invariant does not apply, because nothing is registered on the cobra root.
   - Sandbox Suite Coverage does not apply, because `cmd/lyx/sandbox_coverage_test.go:38–47` enumerates `newRoot().Commands()`, i.e. cobra registration rather than `configreg`, so adding a `**Covers:** batcher` tag would actively fail that test's drift assert.

2. **Config wiring.**
   `internal/batcher` gains the loading of `batcher.yaml` and exposes an entry point returning the active `Batcher` — the natural extension of the `Select`-by-name seam it already has.
   `websterengine.Config.Batcher` is therefore removed, not retained.

   The earlier "retained" note is superseded: retaining the field would leave webster holding a yaml key it no longer owns, and populating it from `batcher.yaml` would be exactly the cross-module config coupling the `loom.yaml` option was rejected for.

3. **The inventory.**
   Both call sites move, not one:
   - `internal/websterengine/runlevel.go:332` — `batcher.Select(deps.Config.Batcher)` becomes a call into `batcher`'s own entry point.
   - `internal/webstercli/cli.go:160` — the `PersistentPreRunE` fail-fast gate, whose behaviour is preserved and only whose source changes.
   - `internal/websterengine/template.yaml:3` — where the `batcher: ""` key physically lives, with its explanatory comment.
   - `internal/webstercli/verbs_test.go:221–223` plus the whole gate-test pair at `:696–732` — `TestPersistentPreRunE_UnknownBatcherFailsFast` and `TestPersistentPreRunE_DefaultBatcherResolves`.
     Both string-replace `batcher: ""` out of the template, so both break the moment the key leaves it — the pair is taken whole, not just the `:697` comment.
   - `internal/websterengine/doc.go:12` and `:25–27`.
   - `docs/overview.md:267`.
   - `internal/websterengine/config_test.go:125`'s `cfg.Batcher == "identity"` assertion, which moves into `internal/batcher`'s own tests with the field.

4. **Migration.**
   Reconcile reports a leftover `webster.yaml` `batcher:` value once, as an orphaned key, and otherwise ignores it — never silently dropped, never read.

   Rationale: honouring it would reinstate the cross-module read this split exists to remove, invisibly, so two worktrees with identical `batcher.yaml` files could batch differently.

   **Rejected alternatives:**
   - Honouring the old key as a fallback.
   - Silently ignoring it.

   **Doc amendments** (each its own step):
   - `internal/batcher/doc.go`'s package comment must stop saying batching is "100% webster's own execution-policy decision" and instead say it is a standalone step webster consumes today and `Shed` will drive as producer #8 once built.
   - `CONSTRAINTS.md`'s Batcher Registry+Config Invariant, both the ownership claim and the `webster.yaml` config-key pin.
   - `docs/overview.md:271`'s batcher module-table entry, which pins the key to `webster.yaml`.
   - The renamed `plan-format.md`'s "Batch is gone / the card is the unit" section, where the card stays the plan's unit but the "entirely internal to webster" framing goes.

### Scope

This task does not change the `Batcher` interface, the registry, or `Select` itself — those stay untouched.
What changes is where the name fed to `Select` is configured, plus the module's registration and docs.

This task does not edit `loom.md`; row 8 of `loom.md`'s producer table is task E's, written after this task lands.

### Sequencing

`depends_on: plan-format-drop-v3-suffix` — this task edits the renamed file.

Task E depends on this task in turn, since `loom.md:56`'s row 8 must reflect whatever this task lands.

### Acceptance

The config relocation is the only behavioural change.
TDD candidates:

- A test asserting the active batchifier resolves from `batcher.yaml` through `batcher`'s own entry point.
- A `configreg` test asserting `batcher` appears in the module list, mirroring `internal/configreg/configreg_test.go:17`'s existing shape.
- A migration test covering an existing worktree whose `webster.yaml` still carries a `batcher:` value.

`internal/batcher`'s existing registry and `Select` tests must pass untouched, since that is the evidence that only the configuration source moved and not the batching itself.

Name the Cwd Resolution Invariant as relevant if the config-key move touches path resolution.
