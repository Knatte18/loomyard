# Discussion: finalize: fold Raddle into its own contract and repair the dead links in raddle.md, finalize.md and self-report.md

```yaml
task: 'finalize: fold Raddle into its own contract and repair the dead links in raddle.md, finalize.md and self-report.md'
slug: raddle-finalize-fold-and-link-repair
status: discussing
parent: main
```

## Problem

The Planned `Shed` roadmap item landed a flat producer-list model in `shed.md` and `loom.md`, and part of that model is a decision: **Raddle-regeneration is folded into `Finalize`'s own contract**, not a separate producer and not a reserved phase slot of its own.
`manifest/designs/finalize.md` and `manifest/designs/raddle.md` were never updated to match — they still describe a machine with a reserved Raddle slot between Webster and Finalize, which no longer exists.
`raddle.md` goes further and carries an explicitly-open question asking whether the fold should happen, when `shed.md:19` and `loom.md:65-67` already decided it.

The same three files also carry dead references, which is what makes this a repair task rather than a prose pass: two links to a `fabric.md` that does not exist in `manifest/designs/`, three uses of a `loom.md#the-phase-machine` anchor that was renamed to `#the-phase-machine--a-flat-producer-list-no-predefined-slots`, and a citation to a "Weft Git Invariant" in `CONSTRAINTS.md` that was never named that.

**Why now:** this is task D of the six-task `shed-producer-model-scoping` follow-up set (see `manifest/designs/shed-followups.md#d--raddle-finalize-fold-and-link-repair`, which is the authoritative task spec).
Its dependency, `builder-retire` (task A), landed as commit `0149776a` and did not touch any of this task's three files, so ownership is clean.
Task D owns `finalize.md`, `raddle.md`, and `self-report.md`, and no other task in the set touches any of them — that is what makes it the one genuinely parallel task, running alongside the `B -> {C, F} -> E` chain rather than inside it.

## Scope

**In:**

- Fold Raddle-regeneration into `finalize.md` as a first-class part of Finalize's own contract — a dedicated section, not a `Related`-section mention.
- Re-read **all three owned files** end to end and remove all producer-model residue found — `finalize.md`, `raddle.md`, and `self-report.md`.
  Every line list in this document is a starting inventory, never a bound.
- Rewrite `raddle.md`'s surviving phase-slot and always-run-step framing, including closing its open question at `:54`.
- Repair the 5 dead links/anchors in the three owned files.
- Repair 6 additional dead links in 3 unowned files that no task in the set claims: `manifest/designs/semantic-index.md` (x3), `manifest/designs/webster-parallel-execution.md` (x2), `docs/shared-libs/README.md` (x1).
- Convert `finalize.md:26`'s prose invariant citation into a real markdown link, so the checker covers that break class too.
- Add a permanent markdown link/anchor enforcement test covering `manifest/` and `docs/`, with 7 allowlist entries covering the 8 remaining broken link instances that live in files owned by other tasks.
- Record the new invariant in `CONSTRAINTS.md` in the same commit.

**Out:**

- **`manifest/roadmap.md`.** `roadmap.md:68`'s "deferred phase slot between Builder and Finalize" is real residue, but `roadmap.md` is edited by task A and task E as well.
  Scoping it here would recreate exactly the shared-file collision that forced task E to be serialized.
  It moves to task E, `roadmap.md`'s last owner.
  This includes `roadmap.md:98`'s prose reference to `scout-redesign.md`, which is not a markdown link and therefore invisible to the link checker anyway.
- **`shed.md`.** `shed.md:18`'s "by *value*" wording contradicts the shared-by-reference framing, but it is task E's to fix so the two tasks do not both edit `shed.md`.
- **`loom.md`, `docs/overview.md`, `docs/reference/plan-format-v3.md`, `docs/reference/status-schema.md`, `docs/reference/discussion-format.md`.**
  Each is inside a multi-owner chain (`loom.md`: B -> C -> E; `docs/overview.md`: A -> B -> E; the three `docs/reference/` files: task B).
  Their dead links are allowlisted, not fixed.
- **`Hardener` and `Tenter`'s equivalent Raddle-into-Finalize fold.** Deferred by the landed design at `shed.md:20` and `loom.md:67`, and stays deferred.
  This task does not design it.
- **Raddle's actual implementation.** `raddle.md:20` already scopes this out and that stays true.
- **Any Go production code.** The only Go this task writes is a test file.

## Decisions

### fold-shape-dedicated-section

- Decision: `finalize.md` gets a dedicated section — heading text along the lines of `## Raddle regeneration — part of the merge, not a step before it` — placed after the existing "Only Raddle forwards from child weft to parent weft" section.
  It states the fold in `shed.md:19`/`loom.md:65-67`'s own terms (Raddle-regeneration is scoped as part of the merge itself, because updating Raddle before the Finalize merge is impractical given merge-conflict risk), states that the merge lock Finalize takes must span the whole regeneration critical section, and **points at** `raddle.md` for the regeneration mechanics rather than restating them.
  The existing `Related` bullet for `raddle.md` (`finalize.md:49`) demotes from decision-carrier to mechanics-pointer.
- Rationale: the fold is a property of Finalize's contract, so it belongs in Finalize's doc at section level, where a reader scanning headings sees it.
  A `Related` bullet is exactly the placement the task spec calls out as wrong.
  Pointing rather than restating respects `shed.md:25`'s pointer rule — `raddle.md` stays the single place the parallel-fork structure, the `Overview.md` sequencing, and the `SyncWeft` commit shape are described.
- Rejected: weaving the fold into the existing "Only Raddle forwards" section without a new heading (less prominent than the spec requires);
  writing it in full producer-contract Input/Output form (over-specifies — `finalize.md` is a design doc, not the producer-contract file, and `loom.md`'s table row 11 already carries Finalize's Input/Output).

### raddle-as-own-producer-deferred

- Decision: **Not designed in this task, but recorded so it is not lost.**
  The user surfaced an alternative model during discussion: let `Raddle` be its own producer in `Shed` after all, and lift merge-in and locking into `Shed` as well, so the lock is a `Shed`-level concern rather than something each producer hand-rolls.
  This task writes the fold section against the currently-landed model (`shed.md:19`, `loom.md:65-67`).
- Rationale: the alternative is a substantially different design touching `shed.md`, `loom.md`, and the producer-contract model — all of which are other tasks' files, and none of which are in this task's scope.
  Adopting it here would mean re-opening a decision that has already landed, from inside the one task in the set that must stay parallel.
- Rejected: designing it now (out of scope, wrong owner, breaks parallelism);
  saying nothing about it (it would read as an oversight when someone next reads the fold section).
- Follow-up: worth a separate wiki task if pursued.
  The fold section should not be written in a way that forecloses it — describe the fold as the landed decision, not as the only conceivable arrangement.

### fabric-md-link-target

- Decision: both `finalize.md:11` and `finalize.md:52` repoint at `internal/fabricengine`'s package documentation (`../../internal/fabricengine/doc.go`).
  `:11`'s surrounding prose additionally drops its stale "absorbed into `fabric` once that lands" framing.
- Rationale: the mechanics actually named at those two sites — `CommitWeft`'s pathspec parameter and `Warp-SHA` correspondence tracking — are shipped and documented in that package doc, and `raddle.md:84` and `raddle.md:60` already cite it in exactly this form for exactly these mechanics.
  The "once that lands" framing is itself residue: the warp/fabric rename landed in `3a748e50` and fabric slice 10 landed in `fcb606f7`.
- Rejected: pointing at `manifest/designs/fabric-unified-view.md` — that doc's own status line says it is deleted once slice 10 and the open half of slice 6 are both done, and slice 10 has landed, so the link would re-break;
  splitting the two targets (no benefit, and `:11`'s prose is being rewritten anyway).

### weft-git-invariant-citation

- Decision: `finalize.md:26` cites the **Fabric Git Invariant (warp + weft)** as a real markdown link with an anchor — `[Fabric Git Invariant (warp + weft)](../../CONSTRAINTS.md#fabric-git-invariant-warp--weft)` — not by line number, and not as an unlinked prose mention.
- Rationale: no "Weft Git Invariant" exists.
  The real section is `## Fabric Git Invariant (warp + weft)`, and it does carry the claim `finalize.md:26` is making — its "Orchestration, not agent" bullet states that agents write overlay files into `_lyx` via the junction and that raddle content lives at `_lyx/raddle/` and arrives through that same junction.
  The task spec cites `CONSTRAINTS.md:173`, which is **stale** — the heading is at `CONSTRAINTS.md:187` as of this branch.
  Citing by section name rather than line number is what stops this from rotting again.

  **Why a link and not just a name.**
  A by-name prose citation is invisible to the new enforcement test — the checker only sees markdown links, so a prose mention of a renamed or deleted section would rot exactly the way the original "Weft Git Invariant" citation did, and the test would never say a word.
  That matters because this break class is one of the three the test's own rationale is built on (see `link-check-is-a-permanent-go-test`);
  leaving it as prose would make one third of that rationale untrue on the day the test lands.
  `finalize.md` is inside the walk and `CONSTRAINTS.md` is reachable at `../../CONSTRAINTS.md`, so as a link the citation is machine-verified from the first run.
  Anchor slug verified against the GitHub rules the checker implements: `## Fabric Git Invariant (warp + weft)` -> `fabric-git-invariant-warp--weft` (the parens and the `+` are deleted rather than replaced, leaving the double hyphen from the two spaces that surrounded the `+`).
- Rejected: citing by line number (already demonstrated to rot within one task cycle);
  an unlinked prose mention of the section name (rots silently — the whole point of the test is to catch this class, and prose is outside its grammar);
  linking the file without the anchor (`[...](../../CONSTRAINTS.md)`) — file existence would be checked, but a renamed section would still slip through, which is the actual failure mode here.

### raddle-md-three-slot-references

- Decision: **`raddle.md` gets the same end-to-end re-read `finalize.md` gets** — the list below is a starting inventory, not a bound, and the same applies to `self-report.md`.
  The four confirmed sites:
  - **`:7`** — "living in `weft`: an always-run step after Webster".
    Retired framing, and it already contradicts this file's own `## When it runs: deferred to merge-time, not mid-task` section three sections later.
    Raddle is not a step after Webster;
    regeneration happens inside the Finalize merge.
    Reword to describe what Raddle *is* (codeguide's weaving-vocabulary successor, living in `weft`, generating docs over the diff a plan produced) without asserting when it runs — the dedicated section below it owns that.
  - **`:3`**, **`:54`**, **`:85`** — as detailed below.
  - `:3` (status blockquote) — drop "Already has a reserved-but-unbuilt phase slot between Webster and Finalize"; state instead that Raddle-regeneration is folded into `Finalize`'s contract, and repoint the anchor to `loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots`.
  - `:54` — convert "**Open, not yet decided:** whether this removes raddle's reserved phase slot..." into the recorded decision, pointing at `finalize.md`'s new fold section and at `shed.md:19`/`loom.md:65-67` as where it landed.
    Repoint the same dead anchor.
  - `:85` (`Related` bullet) — "where raddle's phase slot sits in the phase machine" rewords to point at the flat producer list and the fold, since no slot exists.
    The link itself resolves (no anchor); only the prose is wrong.
- Rationale: the spec named `:3` and `:85`, but `:54` is where the third dead anchor and the open question both live, and the fold is decided — leaving the question open would directly contradict `shed.md:19`.
  `:7` is the case that proves the enumeration method was wrong rather than merely incomplete: it carries the retired framing without using the word "slot", so no slot-focused line list would ever have caught it.
  An open re-read rule catches that class;
  a closed list cannot.
- Rejected: leaving `:85` (its prose asserts a structure that no longer exists, even though the link resolves);
  keeping `:54`'s question and noting it as decided elsewhere (contradicts the landed decision, and leaves the dead anchor).

### repair-scope-owned-plus-safe-unowned

- Decision: repair the 5 dead links in owned files **plus** 6 in three files no task in the set claims.
  Full repair list:
  1. `manifest/designs/finalize.md:11` — `fabric.md` -> `../../internal/fabricengine/doc.go`
  2. `manifest/designs/finalize.md:52` — `fabric.md` -> `../../internal/fabricengine/doc.go`
  3. `manifest/designs/raddle.md:3` — `loom.md#the-phase-machine` -> `loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots`
  4. `manifest/designs/raddle.md:54` — same anchor repair
  5. `manifest/designs/self-report.md:30` — same anchor repair
  6. `manifest/designs/semantic-index.md:3` — `scout-redesign.md` -> `[`internal/scoutengine`](../../internal/scoutengine/doc.go)`
  7. `manifest/designs/semantic-index.md:8` — same
  8. `manifest/designs/semantic-index.md:54` — same
  9. `manifest/designs/webster-parallel-execution.md:54` — same
  10. `manifest/designs/webster-parallel-execution.md:60` — same
  11. `docs/shared-libs/README.md:12` — `../roadmap.md` -> `../../manifest/roadmap.md`

  Plus one link that is **added**, not repaired — `finalize.md:26`'s prose invariant citation becomes `[Fabric Git Invariant (warp + weft)](../../CONSTRAINTS.md#fabric-git-invariant-warp--weft)`, per the `weft-git-invariant-citation` decision.
  It is not in the 11 because it is not currently a broken link;
  it is currently not a link at all.
  After this task the checker verifies 12 touched links, of which 11 were breaks.
- Rationale: every allowlist entry is a debt marker, and 6 of the 14 breaks outside this task's files had **no owner at all** — nobody was scheduled to ever remove them.
  The three added files (`semantic-index.md`, `webster-parallel-execution.md`, `docs/shared-libs/README.md`) appear in no other task's scope, so taking them costs nothing in collision risk and cuts the allowlist from 14 entries to 8, all attributable to a named owner.
- Rejected: owned-files-only (leaves 6 permanently unowned entries);
  fixing all 19 (requires editing `loom.md`, `docs/overview.md`, and three `docs/reference/` files, all multi-owner — this is precisely the collision that serialized task E, and it would destroy this task's parallelism).

### scout-redesign-target-is-the-package-doc

- Decision: the 5 `scout-redesign.md` references in scope repoint at `internal/scoutengine`'s package documentation, in **linked** form — `[`internal/scoutengine`](../../internal/scoutengine/doc.go)` — not as unlinked prose mentions.
- Rationale: verified — `scout-redesign.md` **never existed in git history** (`git log --diff-filter=A -- '*scout-redesign.md'` returns nothing), so this was never a rename or a deletion, just a link to a doc that was planned and never written.
  Meanwhile scout shipped as `internal/scoutengine` + `internal/scoutcli`, and `manifest/roadmap.md:219` already ends its scout entry with "See the `internal/scoutengine` package documentation" — that is the established citation target for scout's design in this repo.

  **Linked, not prose.**
  `roadmap.md:219`'s mention is unlinked prose, but `roadmap.md` is out of scope and its convention is not the one to copy here.
  The five sites being repaired are all currently markdown links, and `raddle.md:29`, `:60`, and `:84` already use the linked ``[`internal/fabricengine`](../../internal/fabricengine/doc.go)`` form for exactly this kind of package-doc citation — as do repair items 1 and 2 in this same task.
  Keeping them linked also keeps them inside the new checker's coverage;
  downgrading them to prose would quietly remove five sites from the guard this task is building.
- Rejected: pointing at `manifest/designs/semantic-index.md` (it is scout's *sibling*, not its successor — `semantic-index.md:3` and `:8` both describe `scout-redesign.md` as a separate thing they defer to, so pointing them at themselves is circular);
  pointing at `manifest/designs/scout-plan-symbol-fields.md` (a speculative downstream idea, not scout's design);
  leaving them allowlisted.
- Note for the implementer: these five sites have differing surrounding prose (`semantic-index.md:3` calls it "the 'deferred idea' scout-redesign.md refers to"; `:8` calls it a parenthetical for `scout`; `webster-parallel-execution.md:54` calls it "the direct ancestor of the scout proposal").
  Reword each in place so the sentence still reads correctly against a package doc rather than a design doc — this is not a find-and-replace.

### link-check-is-a-permanent-go-test

- Decision: add `TestEnforcement_MarkdownLinks` as a **new file** `internal/lyxcwd/docslink_test.go`, in package `lyxcwd`.
  It walks `manifest/` and `docs/` for `.md` files, extracts every inline markdown link, skips `http://`/`https://`/`mailto:`, resolves the file part relative to the containing file, and — for `.md` targets carrying a `#fragment` — resolves the anchor against the target's own headings using GitHub's slug rules.
- Rationale: the task spec names a link-check pass as the acceptance criterion and says it "is exactly what would have caught the dead `fabric.md` links, the dead phase-machine anchors, and the non-existent Weft Git Invariant citation before they shipped" — "before they shipped" means a permanent guard, not a one-shot script.
  All three of those break classes are genuinely covered by the test, but the third one only because the invariant citation is being converted from prose into a real markdown link (see `weft-git-invariant-citation`);
  left as prose it would sit outside the checker's grammar, making a third of this rationale untrue on the day the test lands.
  The repo has a strong precedent for repo-wide enforcement tests, and `internal/lyxcwd/enforcement_test.go` already provides both helpers this needs: `repoRootForEnforcement(t)` and `walkEnforcementRoots(t, repoRoot, roots, suffixes, fn)`.
  That file already does a `.md` walk (`walkEnforcementRoots(t, repoRoot, []string{"internal"}, []string{".md"}, ...)` at line 925), so walking `[]string{"manifest", "docs"}` is the same call with different roots.
- Rejected: appending to `enforcement_test.go` (already ~950 lines and thematically about cwd/geometry/vocabulary — a separate file in the same package gets the helper reuse without the bloat, and keeps blame clean);
  a one-shot script in `.scratch/` (no regression guard — the next dead link ships the same way these did);
  a new `internal/docslink` leaf package with the checker as production code (heavier, and nothing today needs it callable from a CLI — YAGNI).
- Note: `internal/lyxcwd`'s Leaf Invariant caps **production** imports to stdlib + `internal/gitexec` (`leaf_enforcement_test.go`).
  A test file is not production code, so this does not touch that invariant — but the new test must import stdlib only anyway, which it naturally does.
- Note: placing the test in `internal/lyxcwd` is a file-layout convenience, **not** an ownership claim on markdown links by that package — the Cwd Resolution Invariant scopes `internal/lyxcwd` to cwd resolution and nothing else.
  `CONSTRAINTS.md` must say so in the new section (see `constraints-entry`, point 3), exactly as `CONSTRAINTS.md:177` already does for `TestEnforcement_FabricVocabulary`.

### allowlist-is-keyed-and-self-expiring

- Decision: the allowlist is a map keyed by `(file, target)` — never by line number — with a one-line reason naming the owning task.
  The test additionally **fails on a stale entry**, where staleness is defined as: **the allowlist key was not matched by any broken link found in this run's scan.**
- Rationale: line numbers rot on the first edit to the file.
  Naming the owner turns each entry into a tracked debt rather than anonymous wallpaper, and the stale-entry check is what makes the list shrink to zero on its own as tasks B and E land, instead of persisting after the underlying break is gone.

  **Why "not matched by the scan" and not "the link now resolves".**
  The narrower "now resolves" phrasing has a hole that hits 2 of the 7 entries.
  Task B renames `docs/reference/plan-format-v3.md` to `plan-format.md` (`shed-followups.md:206`), so the two entries keyed on the old filename stop being visited by the walk at all once that lands — the walk never opens a file that no longer exists, the "does it resolve now?" question is never asked, and both entries linger silently forever.
  That is precisely the rot this decision exists to prevent, so the check has to fire on the file-renamed-away case as well as the link-now-resolves case.
  Defining staleness as *unmatched key* covers both with one condition and needs no second mechanism: after each run, every allowlist key that no scanned break matched is reported as deletable, whether that is because the link was fixed, because the containing file was renamed, or because the file was deleted outright.
- Rejected: the same map without any stale check (entries linger silently forever after B lands);
  "the link now resolves" as the staleness condition (the renamed-away hole above — it is the weaker of the two rules with no compensating benefit);
  keeping "now resolves" and bolting on a separate per-entry file-existence check (two rules where one strictly more general rule does the job);
  a coarse list of ignored target filenames (would mask a genuinely new break to the same target, e.g. a fresh dead `plan-format.md` link added in an unrelated file).
- The 7 entries, covering 8 broken link instances:

  | File | Dead target | Kind | Owner |
  |---|---|---|---|
  | `docs/reference/discussion-format.md` | `plan-format.md` | missing file | task B — resolves when `plan-format-v3.md` is renamed to `plan-format.md` |
  | `docs/reference/plan-format-v3.md` | `plan-format.md` | missing file | task B — same |
  | `docs/reference/status-schema.md` | `plan-format.md` | missing file | task B — same |
  | `manifest/designs/loom.md` | `../../docs/reference/plan-format.md` | missing file | task B — same |
  | `docs/reference/plan-format-v3.md` | `../../manifest/designs/scout-redesign.md` | missing file | task B owns this file; the target fix is the same one this task applies elsewhere |
  | `docs/overview.md` | `../CONSTRAINTS.md#package-naming` | missing anchor | chain A -> B -> E; E is last owner |
  | `manifest/designs/loom.md` | `../../docs/overview.md#hub-geometry-invariants` | missing anchor | chain B -> C -> E; E is last owner |

  Note this is **7 entries covering 8 link instances** — `docs/reference/plan-format-v3.md` appears twice for `scout-redesign.md` (lines 178 and 345), and a `(file, target)` key collapses those two into one entry, which is the intended behaviour.
  Use the "7 entries covering 8 link instances" phrasing wherever the count is stated, so the two numbers never read as a contradiction.

### constraints-entry

- Decision: add a `## Markdown Link Integrity` section to `CONSTRAINTS.md`, in the same commit as the test.
- Rationale: `CLAUDE.md` requires any new cross-cutting invariant to be recorded in `CONSTRAINTS.md` in the same commit, and this one binds every `.md` file under `manifest/` and `docs/`.
  The section must state:
  1. **The rule** — every inline markdown link in a `.md` file under `manifest/` or `docs/` resolves, both its file part and its `#anchor`.
  2. **The enforcing test** — `internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks`.
  3. **A placement caveat**, in the same words the Fabric Vocabulary Invariant already uses at `CONSTRAINTS.md:177`: the test living in `internal/lyxcwd` is a **file-layout convenience** — it reuses that package's `repoRootForEnforcement` and `walkEnforcementRoots` helpers — and is **not an ownership claim**.
     This matters because the Cwd Resolution Invariant says `internal/lyxcwd` owns cwd resolution *and nothing else*, so without the caveat the new test reads as a second ownership claim on that package.
     The Fabric Vocabulary Invariant hit exactly this tension and resolved it with that sentence;
     copy the resolution rather than re-litigating it.
  4. **What the check does and does not reach, stated honestly** — mirroring the "What the machine check does and does not reach — stated honestly, not implying full coverage" framing at `CONSTRAINTS.md:178`.
     Not reached: external `http`/`https`/`mailto` URLs (never fetched);
     reference-style links and `<...>` autolinks (out of grammar by decision, not by oversight);
     link-shaped text inside fenced code blocks (deliberately skipped);
     prose mentions of a filename that are not markdown links (`roadmap.md:98`'s `scout-redesign.md` reference is a live example that this task leaves standing);
     and `.md` files outside `manifest/` and `docs/` as **scan sources** — `README.md`, `CLAUDE.md`, and `internal/**/*.md` have their own outgoing links checked by nobody.

     **The root restriction is source-side only.**
     `manifest/` and `docs/` name which files are *scanned* for outgoing links.
     They do not restrict where those links may *point*: every link target is resolved wherever it lands in the repo, and any `.md` target gets its `#anchor` resolved too, inside those roots or not.
     State this explicitly in `CONSTRAINTS.md`, because the two facts sit one sentence apart and an implementer could otherwise read "outside `manifest/` and `docs/` is not reached" as licence to skip anchor resolution for out-of-root targets.
     That reading would silently un-guard `finalize.md:26`'s `../../CONSTRAINTS.md#fabric-git-invariant-warp--weft` — the exact break class round 1 added the link to cover — and also the ten `../../internal/*/doc.go` targets this task creates or touches.
  5. **The allowlist's self-expiring contract** — keyed by `(file, target)`, each entry naming its owning task, and an entry whose key goes unmatched by a scan is reported as deletable.

## Technical context

**Authoritative task spec:** `manifest/designs/shed-followups.md#d--raddle-finalize-fold-and-link-repair` (lines 314-369).
Read it — the wiki task body is a pointer to it, not a spec.
Note that two of its line citations are already stale (see Gotchas).

**The landed model this task writes against:**

- `manifest/designs/shed.md:7-20` — `Shed` has no predefined slots; everything is a producer in one flat list.
  `:18` says `Finalize` is an ordinary producer both `loom` and `Hardener` reference (its "by *value*" wording is task E's to fix).
  `:19` is the fold decision.
  `:20` defers `Tenter`'s equivalent.
- `manifest/designs/loom.md:41-76` — the producer-list table.
  Row 11 is `Finalize`: "mechanical (mostly) | approved diff | merge-back, PR; shared by reference with `Hardener`'s own producer list, never by `Shed` special-casing it".
  `:65-67` restates the fold with the merge-conflict-risk rationale and the `lyx fabric sync` commit path.
- **`finalize-shared-by-reference`** — `Finalize` is shared **by reference**: both `loom`'s and `Hardener`'s lists name the same producer definition.
  One definition, named twice, never copied.
  This is the framing the fold is written against, and it is what makes `finalize.md:45-46`'s "literally-shared code ... both share this exact code" wrong.

**Files this task edits:**

| File | Why |
|---|---|
| `manifest/designs/finalize.md` | fold section + full residue re-read + 2 link repairs + invariant citation converted to a checked link |
| `manifest/designs/raddle.md` | end-to-end re-read; 4 confirmed residue sites (`:3`, `:7`, `:54`, `:85`), 2 of which carry dead anchors |
| `manifest/designs/self-report.md` | end-to-end re-read; 1 confirmed dead anchor at `:30` |
| `manifest/designs/semantic-index.md` | 3 dead `scout-redesign.md` links |
| `manifest/designs/webster-parallel-execution.md` | 2 dead `scout-redesign.md` links |
| `docs/shared-libs/README.md` | 1 wrong relative path at `:12` |
| `internal/lyxcwd/docslink_test.go` | new — the enforcement test |
| `CONSTRAINTS.md` | new invariant section |

**`finalize.md` residue inventory (the end-to-end re-read).**
The spec is explicit that its line list is a starting inventory, not a bound.
Confirmed residue as of this branch:

- `:3` — "Finalize is **`Shed`'s** literally-shared code (identical for `loom` and `Hardener`, not a swappable per-instance slot the way Preflight and the producer are)".
  Both halves are retired: there is no shared-Finalize special case, and there are no Preflight/producer slots to contrast against.
  Rewrite against `finalize-shared-by-reference`.
- `:11` — `fabric.md` link + the stale "absorbed into `fabric` once that lands" framing.
- `:12` — "that's `warp cleanup`'s (future: `fabric`'s) existing, separate job".
  Same rename residue as `:11`, one line later.
  The shipped spelling is `lyx fabric cleanup` (verified: `internal/fabriccli/fabric.go:249` registers `Use: "cleanup ..."` under the `fabric` command), so the doc names a command that does not exist and hedges "future" about something already shipped.
  Rewrite alongside `:11` — they are one sentence pair.
- `:26` — the non-existent "Weft Git Invariant" citation, which also becomes a real markdown link (see `weft-git-invariant-citation`).
- `:45-46` — "`Shed`'s literally-shared code ... both share this exact code", the same retired shared-code framing as `:3`.
- `:48` — "`Shed` hasn't been extracted from it yet (see that doc's own naming note)".
  This is scheduled to become false when task E fixes `loom.md:15-17`.
  Since task E may land after this task, phrase the replacement so it is correct either way — describe the `shed.md`/`loom.md` split of authority (`shed.md` owns the generic mechanism, `loom.md` owns `loom`'s concrete producer list, per `shed.md:3`) rather than asserting anything about extraction status.
- `:52` — the second `fabric.md` link.

**Spec item already resolved — do not "fix" it.**
The spec lists "`:9` references Builder's escalation behavior, which task A retires."
Verified false as of this branch: `grep -in builder` across all three owned files returns **zero** hits.
`finalize.md:9` cites `internal/websterengine`'s package documentation, which exists (`internal/websterengine/doc.go`).
The builder->webster rename landed before task A.
Report this as already-satisfied rather than inventing a change.

**Link-checker implementation notes:**

- `internal/lyxcwd/enforcement_test.go:92` defines `repoRootForEnforcement(t *testing.T) string`, resolving the repo root relative to the test file's own location via `runtime.Caller`.
- `internal/lyxcwd/enforcement_test.go:110` defines `walkEnforcementRoots(t, repoRoot string, roots, suffixes []string, fn func(relPath string, data []byte))`.
  Roots are repoRoot-relative; `relPath` comes back slash-normalized; `.git` and `testdata` directories are skipped.
  Line 925 is a working `.md`-suffix precedent.
- Anchor slugging must match GitHub's rules, because that is what the existing anchors were written against.
  The rule that matters here: lowercase, strip backticks, strip all non-word/non-space/non-hyphen characters (which deletes em-dashes and slashes entirely rather than replacing them), then replace spaces with hyphens.
  Worked examples that must pass:
  - `## The phase machine — a flat producer list, no predefined slots` -> `the-phase-machine--a-flat-producer-list-no-predefined-slots` (the em-dash vanishes, leaving the double hyphen from the two surrounding spaces).
  - `## The summary artifact — `_lyx/webster/summary.md`` -> `the-summary-artifact--_lyxwebstersummarymd` (backticks and slashes deleted, underscore kept as a word character).
  - `## When it runs: deferred to merge-time, not mid-task` -> `when-it-runs-deferred-to-merge-time-not-mid-task`.
- **Link grammar — decided, not left to the implementer.**
  The checker recognises **inline links only**: `[text](target)`.
  Reference-style links (`[text][ref]` plus a `[ref]: target` definition) and angle-bracket autolinks (`<https://...>`) are **out of grammar** and silently ignored, because neither form appears anywhere in `manifest/` or `docs/` today and adding them would be speculative.
  `CONSTRAINTS.md`'s honesty paragraph must say so explicitly, so a future author who introduces a reference-style link knows it is unguarded rather than assuming it is covered.
- **Fenced code blocks are skipped.**
  Track ``` and `~~~` fences while scanning and ignore any link-shaped text inside one — a fenced example of a broken link is documentation, not a broken link, and failing on it would make the test impossible to document around.
  Indented (4-space) code blocks are not tracked;
  they do not currently occur with link-shaped content in either tree.
- **Duplicate headings get GitHub's `-1`, `-2` disambiguation suffixes.**
  When two headings in one file slug identically, GitHub appends `-1` to the second occurrence, `-2` to the third, and so on.
  The anchor resolver must build the target file's anchor set with those suffixes applied in document order, or a legitimate link to the second of two identically-named headings would be reported as broken.
  This does not currently bite any link in either tree, but it is cheap to get right at build time and expensive to debug later.
- A throwaway reference implementation of this checker lives at `.scratch/linkcheck.py` on this branch.
  It is gitignored scratch, not the deliverable — use it to cross-check the Go test's findings, then let it be.
  Its current output on this branch is 19 problems across 45 files; after this task the Go test must report 0 with 7 allowlist entries active (covering 8 link instances).
  Note the reference implementation handles **neither** fenced-code-block skipping **nor** duplicate-heading `-1` suffixes — the Go test must, per the two bullets above, so a straight port is not sufficient.

**Gotchas:**

- **Do not rename `raddle.md`'s `## When it runs: deferred to merge-time, not mid-task` heading.**
  `finalize.md:28` links `raddle.md#when-it-runs-deferred-to-merge-time-not-mid-task`, and that anchor currently resolves.
  Rewriting `:54` sits inside that section — change the body, keep the heading text.
  If the heading must change, the `finalize.md:28` link changes in the same edit.
  The new enforcement test will catch this, which is the point.
- **`CONSTRAINTS.md:173` is not the Fabric Git Invariant.**
  The spec says it is.
  On this branch, line 173 is inside the *Fabric Vocabulary* Invariant; `## Fabric Git Invariant (warp + weft)` is at line 187.
  Cite by name.
- **`raddle.md:3` says "between Webster and Finalize"; the spec's prose says "between Builder and Finalize".**
  The file is what matters; the spec's phrasing is imprecise, not a second thing to find.
- **`raddle.md:85` has no anchor**, so the link resolves and the checker will never flag it.
  It still needs the prose rewrite.
  This one is review-caught, not machine-caught.
- The 5 `scout-redesign.md` sites need individually-reworded prose, not a find-and-replace (see the `scout-redesign-target-is-the-package-doc` decision's implementer note).
- `docs/shared-libs/README.md:12` resolves `../roadmap.md` to `docs/roadmap.md`.
  The real file is `manifest/roadmap.md`, so the correct relative target from `docs/shared-libs/` is `../../manifest/roadmap.md`.
  Editing this file is *not* the same as editing `manifest/roadmap.md`, which stays out of scope.

## Constraints

From `CONSTRAINTS.md`:

- **Documentation Lifecycle** (`CONSTRAINTS.md:339`, delegating to `docs/overview.md#documentation-lifecycle`) — governs which docs are durable design docs versus mechanical per-module docs that fold and get deleted.
  Directly relevant: `finalize.md`'s own status line commits it to being deleted once `Shed`+Finalize land, and `fabric-unified-view.md`'s commits it to deletion once slice 10 and slice 6's open half are both done.
  This is the reasoning behind pointing `finalize.md` at `internal/fabricengine`'s package doc rather than at another soon-to-be-deleted design doc.
- **lyxcwd Leaf Invariant** (`CONSTRAINTS.md:8`, enforced by `internal/lyxcwd/leaf_enforcement_test.go`) — production code in `internal/lyxcwd` imports only stdlib and `internal/gitexec`.
  The new test file is test code and therefore outside that cap, but it should import stdlib only regardless.
- **Fabric Vocabulary Invariant** (`CONSTRAINTS.md:157`) — its `internal/**/*.md` walk does **not** reach `manifest/` or `docs/`, so the prose this task writes is a review obligation for warp/weft/host vocabulary, not machine-checked.
  Use `warp`/`weft`/`fabric` correctly by hand; `CONSTRAINTS.md:173-175`'s prose-doc split applies (a doc explaining fabric's own mechanism keeps the vocabulary).
  `finalize.md`'s "Two merge targets" section is squarely a fabric-mechanism doc and keeps it.
- **Fabric Git Invariant (warp + weft)** (`CONSTRAINTS.md:187`) — the invariant `finalize.md:26` should have been citing.

From `CLAUDE.md`:

- **Docs land in the same commit.** A change introducing cross-cutting infrastructure updates `CONSTRAINTS.md` in the same commit — this task's enforcement test is exactly that.
- **`manifest/roadmap.md` moves only on completing or adding a planned item.** Independently reinforces keeping it out of scope.
- **Markdown: semantic line breaks, no fixed-column hard-wrap.** One sentence per line; break inside long sentences at internal independent-clause boundaries.
  Applies to every `.md` edited here, including the prose being rewritten, not just newly-added lines.
  Table cells and blockquotes stay on one line.
- **Worktree isolation.** All work stays in `wts/raddle-finalize-fold-and-link-repair`.

Task-set constraint:

- **Do not touch a file owned by another task**, even to fix an obvious break.
  The allowlist exists precisely so that obvious-but-foreign breaks stay unfixed here.

## Testing

Docs-only apart from one test file, so there is no production code to test.
The test *is* the deliverable.

**`TestEnforcement_MarkdownLinks` — the one new test, and a genuine TDD candidate.**
Write it before making any of the 11 link repairs.
The natural sequence:

1. Write the test with an **empty** allowlist and run it.
   It must fail, reporting 19 broken links across `manifest/` and `docs/`.
   This is the red step, and it also validates the checker against a known-good expected set (the `.scratch/linkcheck.py` output).
2. Populate the allowlist with the 7 entries (covering 8 link instances).
   The test must now fail with exactly 11 breaks — the ones this task repairs.
3. Apply the 11 repairs, plus the `finalize.md:26` citation-to-link conversion.
   The test goes green over 12 touched links.
4. Add a deliberately-broken link to a scratch fixture or temporarily to a real file, confirm the test catches it, then revert.
   This proves the checker is actually resolving rather than trivially passing.

**Fixture seam — decided, because the obvious choice silently does nothing.**
Most scenarios below (missing file, missing anchor, fenced-code skip, duplicate-heading `-1`, both stale cases) need a synthetic tree rather than the real repo.
**Do not put that tree under `testdata/`.**
`walkEnforcementRoots` skips any directory whose name *contains* `"testdata"` (`internal/lyxcwd/enforcement_test.go:128`), so a `testdata/` fixture walks to zero files and every subtest built on it passes **vacuously** — green, and testing nothing.

The seam instead:

- Factor the checker into **pure helpers** that take data rather than reading the filesystem: a slug function (`string -> string`), a heading-set extractor (`[]byte -> map[string]bool`, applying the duplicate `-1` suffixes in document order), and an inline-link extractor (`[]byte -> []link`, fence-aware).
  Every grammar and slug scenario is a plain table test against these — no tree needed at all.
- For the scenarios that genuinely need a tree (link resolution across files, the two stale-allowlist cases), build it under **`t.TempDir()`** and call `walkEnforcementRoots(t, tmpRoot, []string{"."}, []string{".md"}, ...)`.
  Its `repoRoot` parameter is an ordinary path, not hardwired to the real repo root, so a temp root composes with it directly and the `testdata` skip never fires.
- The single real-repo case — the actual `manifest/` + `docs/` walk with the live allowlist — stays as its own subtest using `repoRootForEnforcement(t)`.
  That is the one that must report zero.

**Scenarios the test must cover** (as subtests or table cases, shape otherwise left to mill-plan):

- A relative link to an existing file with no fragment resolves.
- A relative link to a missing file fails.
- A `#fragment` matching a heading in the target file resolves.
- A `#fragment` with no matching heading fails.
- A same-file fragment (`[x](#some-heading)`) resolves against the containing file's own headings.
- `http://`, `https://`, and `mailto:` targets are skipped, never fetched.
- The three worked slug examples in Technical context each resolve — em-dash deletion, backtick/slash deletion, and colon deletion are the three rules most likely to be got wrong.
- An allowlisted `(file, target)` pair does not fail the test.
- A **stale** allowlist entry fails the test with a message saying to delete it, in **both** stale cases: (a) the link now resolves, and (b) the keyed file no longer exists or was renamed away, so the scan never visits it.
  Case (b) is the one that motivated the definition — cover it explicitly, since it is the case a naive implementation silently passes.
- Link-shaped text inside a fenced code block is ignored, for both ``` and `~~~` fences.
- Two identically-slugging headings in one file produce anchors `foo` and `foo-1`, and a link to `#foo-1` resolves.
- Links to non-`.md` targets (e.g. `../../internal/fabricengine/doc.go`) have their file existence checked but no anchor check attempted.
  This case matters: after this task, `finalize.md` has two of them.

**Manual verification (review obligation, not machine-checked):**

- `finalize.md` reads coherently end to end after the residue rewrite — the fold section does not contradict the "Only Raddle forwards" section immediately above it, and no sentence still implies a reserved Raddle slot or a `Shed`-special-cased Finalize.
- `raddle.md:85`'s prose no longer asserts a phase slot (invisible to the checker — no anchor).
- The 5 `scout-redesign.md` sites read correctly as references to a package doc rather than a design doc.
- Every edited `.md` follows semantic line breaks.
- `go build ./... && go test ./...` passes — the new test is the only addition, but the full suite confirms nothing else regressed.

## Q&A log

- **Q:** How does the link-check acceptance criterion get implemented — permanent Go enforcement test with an allowlist, a one-shot script, or a permanent test with no allowlist? **A:** Permanent Go enforcement test plus allowlist, with the invariant recorded in `CONSTRAINTS.md` in the same commit.
- **Q:** Which of the 19 repo-wide broken links does this task repair? **A:** Started as the 5 in owned files; after the user asked what the problem was with taking all of them, the set was split into 6 safe (no other task claims those files) and 8 unsafe (multi-owner chain files, where editing would recreate exactly the collision that serialized task E). Final answer: repair 11, allowlist 7 entries covering the remaining 8 link instances.
- **Q:** What replaces the dead `fabric.md` links? **A:** `internal/fabricengine`'s package doc for both, and `:11`'s stale "absorbed into `fabric` once that lands" framing goes with it — `fabric-unified-view.md` was rejected because it is itself scheduled for deletion.
- **Q:** What shape does the Raddle fold take in `finalize.md`? **A:** A dedicated section (not a `Related` bullet, not a full producer-contract restatement). The user noted that the more logical arrangement might be Raddle as its own `Shed` producer with merge-in and locking lifted into `Shed` too, but judged that too large a change to decide now — recorded under `raddle-as-own-producer-deferred`, and the section is written so as not to foreclose it.
- **Q:** Which three `raddle.md` references carry the slot framing? **A:** `:3` (status blockquote, dead anchor), `:54` (the open question, dead anchor), `:85` (Related bullet, link resolves but prose is wrong). All three are rewritten; the spec named only `:3` and `:85`, but `:54` carries both the third dead anchor and the question the fold answers.
- **Q:** What do the `scout-redesign.md` references point at? **A:** `internal/scoutengine`'s package documentation. Verification changed the answer here — `scout-redesign.md` never existed in git at all, and scout has since shipped, with `roadmap.md:219` already using that exact citation form. The initial recommendation of `semantic-index.md` was wrong: that doc is scout's sibling, and pointing it at itself would be circular.
- **Q:** [review r1, BLOCKING] The allowlist's stale-entry check is defined as "the link now resolves", but task B renames `plan-format-v3.md` away, so the two entries keyed on that filename are never scanned again and linger forever — 2 of 7 entries. How is staleness defined? **A:** Redefine it as "allowlist key not matched by any scanned broken link", which covers both the now-resolves case and the file-renamed-or-deleted case with one condition. Rejected keeping "now resolves" plus a bolted-on file-existence check (two rules where one strictly more general rule suffices).
- **Q:** [review r1, BLOCKING] The permanent test is justified partly by "it would have caught the non-existent Weft Git Invariant citation", but the decided repair is a by-name *prose* citation, which the checker cannot see — so a third of the test's rationale is uncovered. **A:** Convert it to a real markdown link, `[Fabric Git Invariant (warp + weft)](../../CONSTRAINTS.md#fabric-git-invariant-warp--weft)`. Anchor slug verified against the checker's own rules. `finalize.md` is inside the walk, so the break class becomes genuinely machine-guarded and the rationale becomes true. Rejected dropping the clause from the rationale (honest but leaves the class unguarded) and linking the file without an anchor (a renamed section would still slip through).
- **Q:** Where does the enforcement test live, and what shape is the allowlist? **A:** Delegated to the assistant. New file `internal/lyxcwd/docslink_test.go` in package `lyxcwd`, reusing `repoRootForEnforcement` and `walkEnforcementRoots` from `enforcement_test.go` without bloating that ~950-line file. Allowlist is keyed by `(file, target)` — never line number — with an owning-task reason per entry, and the test fails on stale entries so the list self-expires as tasks B and E land.
