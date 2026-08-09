# Batch: raddle-fold-and-link-guard

```yaml
task: 'finalize: fold Raddle into its own contract and repair the dead links in raddle.md, finalize.md and self-report.md'
batch: raddle-fold-and-link-guard
number: 1
cards: 11
verify: go test ./internal/lyxcwd/
depends-on: []
```

## Batch Scope

This batch delivers the whole task: a new permanent markdown link/anchor enforcement test (`internal/lyxcwd/docslink_test.go`), the 11 dead-link repairs plus one citation-to-link conversion across six documentation files, the Raddle-into-Finalize fold section in `manifest/designs/finalize.md`, the producer-model residue rewrites in `finalize.md`/`raddle.md`/`self-report.md`/`README.md`, and the `## Markdown Link Integrity` invariant in `CONSTRAINTS.md`.

It is one batch because the guard and the repairs are mutually dependent — see the overview's "one batch, because the guard and the repairs are one unit" Shared Decision.
Cards 1–4 build the checker, cards 5–8 apply the repairs and prose rewrites, card 9 records the invariant, card 10 removes the orphaned phase-chain residue in `README.md`, and card 11 is a zero-diff probe proving the checker resolves rather than trivially passing.

**Cards 2 and 3 commit a deliberately-failing test.**
Do not "fix" that by reordering the cards or by pre-emptively repairing links early — the red step is the point, and the batch `verify:` at the end is the green boundary.
See the overview's "mid-batch commits are deliberately red" Shared Decision.

There is no external interface for a next batch to consume — this is the only batch.

## Cards

### Card 1: pure checker helpers and their table tests

- **Context:**
  - `internal/lyxcwd/enforcement_test.go`
  - `internal/lyxcwd/leaf_enforcement_test.go`
  - `.scratch/linkcheck.py`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `internal/lyxcwd/docslink_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/lyxcwd/docslink_test.go` in package `lyxcwd`, importing stdlib only.
  Add a file-level comment explaining that this file guards markdown link and anchor integrity under `manifest/` and `docs/`, and that its placement in `internal/lyxcwd` is a file-layout convenience reusing `repoRootForEnforcement` and `walkEnforcementRoots` from `enforcement_test.go`, not an ownership claim on markdown links by that package.
  Define three pure helpers and one type in this card:
  `docsLinkSlug(heading string) string` — implements GitHub's heading-slug rules: strip a leading run of `#` characters and the single following space; delete every backtick; lowercase; delete every rune that is not a Unicode letter, a Unicode digit, `_`, `-`, or a space; replace each remaining space with `-`.
  Note the deletion is a deletion, not a replacement — an em-dash surrounded by spaces leaves the two spaces behind, which become a double hyphen.
  `type docsLink struct` with fields `Line int` and `Target string`.
  `docsLinkExtract(data []byte) []docsLink` — returns every **inline** markdown link `[text](target)` in document order with its 1-based line number.
  It must track fenced code blocks (both ``` and `~~~` fences, opened and closed at line start allowing up to three leading spaces) and skip every link-shaped match inside a fence.
  Reference-style links (`[text][ref]`) and angle-bracket autolinks (`<https://...>`) are out of grammar and must be ignored, per `_mill/discussion.md`'s "Link grammar — decided, not left to the implementer".
  The target is the parenthesized text and must not contain whitespace or a closing paren.
  `docsLinkHeadingAnchors(data []byte) map[string]bool` — returns the set of anchors for a file's ATX headings (`#` through `######` followed by a space) in document order, skipping headings inside fenced code blocks, applying `docsLinkSlug` to each heading's text, and appending GitHub's duplicate-disambiguation suffixes: the first occurrence of a slug is bare, the second gets `-1`, the third `-2`, and so on.
  Add `TestDocsLinkSlug`, `TestDocsLinkExtract`, and `TestDocsLinkHeadingAnchors` as table tests over literal data — never a filesystem tree.
  `TestDocsLinkSlug` must include the three worked examples from `_mill/discussion.md`'s "Link-checker implementation notes": the `loom.md` phase-machine heading slugging to `the-phase-machine--a-flat-producer-list-no-predefined-slots`, the webster-contract summary-artifact heading slugging to `the-summary-artifact--_lyxwebstersummarymd`, and `raddle.md`'s when-it-runs heading slugging to `when-it-runs-deferred-to-merge-time-not-mid-task`.
  Add a fourth case for `## Fabric Git Invariant (warp + weft)` slugging to `fabric-git-invariant-warp--weft`, since card 5 creates a link depending on exactly that slug.
  `TestDocsLinkExtract` must cover: a plain inline link, a link whose text contains backticks, two links on one line, a link inside a ``` fence being skipped, a link inside a `~~~` fence being skipped, and a reference-style link plus an autolink both being ignored.
  `TestDocsLinkHeadingAnchors` must cover: distinct headings, two identically-slugging headings producing `foo` and `foo-1`, three producing `foo`/`foo-1`/`foo-2`, and a `#`-prefixed line inside a fence not counting as a heading.
  All three tests must pass when this card is complete.
- **Commit:** `test(lyxcwd): add pure markdown link/anchor checker helpers`

### Card 2: repo scan driver and the red step

- **Context:**
  - `internal/lyxcwd/enforcement_test.go`
  - `.scratch/linkcheck.py`
- **Edits:**
  - `internal/lyxcwd/docslink_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the scan driver and the real-repo subtest to `docslink_test.go`.
  Define `type docsLinkKey struct` with fields `File string` and `Target string`, where `File` is the repoRoot-relative slash-normalized path of the file the link was found in and `Target` is the raw target string exactly as written in the source.
  Define `type docsLinkBreak struct` with fields `File string`, `Line int`, `Target string`, and `Reason string`, where `Reason` is one of the two literals `missing file` or `missing anchor`.
  Define `docsLinkScan(t *testing.T, repoRoot string, roots []string, allow map[docsLinkKey]string) (breaks []docsLinkBreak, unmatched []docsLinkKey)`.
  It calls `walkEnforcementRoots(t, repoRoot, roots, []string{".md"}, ...)`, runs `docsLinkExtract` over each file, and for each link: skips targets beginning `http://`, `https://`, or `mailto:` without resolving them; splits the target on its first `#` into a file part and a fragment; when the file part is empty resolves the fragment against the containing file's own `docsLinkHeadingAnchors`; otherwise resolves the file part relative to the containing file's directory, checks the resolved path exists on disk, and — only when it exists, ends in `.md`, and the fragment is non-empty — resolves the fragment against that target file's `docsLinkHeadingAnchors`.
  A target that exists but does not end in `.md` gets its file existence checked and no anchor check attempted.
  Every unresolved link becomes a `docsLinkBreak`.
  `breaks` is the list of breaks whose `docsLinkKey` is **not** present in `allow`; `unmatched` is the list of `allow` keys that no break in this run — allowlisted or not — matched.
  The root restriction is source-side only: `roots` names which files are scanned for outgoing links and never restricts where a target may point.
  Resolve every target wherever it lands in the repo, and resolve the `#anchor` of any `.md` target whether or not that target is itself inside `roots`.
  Add subtest `TestEnforcement_MarkdownLinks` with a `t.Run("repo", ...)` case calling `docsLinkScan(t, repoRootForEnforcement(t), []string{"manifest", "docs"}, docsLinkAllowlist)` and failing on any element of `breaks` or `unmatched`, with a message that lists each break as file, line, reason, and target, and each unmatched allowlist entry with an instruction to delete it.
  Declare `var docsLinkAllowlist = map[docsLinkKey]string{}` — **empty in this card**.
  Run `go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` and confirm the repo subtest fails reporting **exactly 19 breaks**, matching `.scratch/linkcheck.py`'s current output file-for-file, line-for-line, and reason-for-reason.
  Record the confirmed count of 19 in the commit body.
  This card commits a failing test on purpose.
- **Commit:** `test(lyxcwd): scan manifest/ and docs/ for broken markdown links (red: 19)`

### Card 3: the self-expiring allowlist

- **Context:**
  - `.scratch/linkcheck.py`
  - `_mill/discussion.md`
  - `docs/reference/plan-format-v3.md`
- **Edits:**
  - `internal/lyxcwd/docslink_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Populate `docsLinkAllowlist` with exactly seven entries, each mapping a `docsLinkKey` to a one-line reason naming the owning task.
  The seven keys, taken verbatim from `_mill/discussion.md`'s `allowlist-is-keyed-and-self-expiring` table, are:
  `{File: "docs/reference/discussion-format.md", Target: "plan-format.md"}` — task B, resolves when `plan-format-v3.md` is renamed to `plan-format.md`.
  `{File: "docs/reference/plan-format-v3.md", Target: "plan-format.md"}` — task B, same.
  `{File: "docs/reference/status-schema.md", Target: "plan-format.md"}` — task B, same.
  `{File: "manifest/designs/loom.md", Target: "../../docs/reference/plan-format.md"}` — task B, same.
  `{File: "docs/reference/plan-format-v3.md", Target: "../../manifest/designs/scout-redesign.md"}` — task B owns the file; the target fix is the one this task applies elsewhere.
  `{File: "docs/overview.md", Target: "../CONSTRAINTS.md#package-naming"}` — chain A to B to E; E is last owner.
  `{File: "manifest/designs/loom.md", Target: "../../docs/overview.md#hub-geometry-invariants"}` — chain B to C to E; E is last owner.
  Add a comment above the map stating that it is keyed by `(file, target)` and never by line number, that every entry names its owning task, and that an entry whose key is not matched by any break in a scan is reported as deletable.
  Use the phrasing "7 entries covering 8 link instances" wherever the count is stated — `docs/reference/plan-format-v3.md` carries the `scout-redesign.md` target twice and the key collapses both into one entry, which is intended.
  Do not add an entry for any of the 11 links this task repairs.
  Run `go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` and confirm the repo subtest now fails reporting **exactly 11 breaks** and **zero unmatched** allowlist entries.
  Record the confirmed count of 11 in the commit body.
  This card commits a failing test on purpose.
- **Commit:** `test(lyxcwd): add the 7-entry self-expiring link allowlist (red: 11)`

### Card 4: synthetic-tree and allowlist-behaviour subtests

- **Context:**
  - `internal/lyxcwd/enforcement_test.go`
- **Edits:**
  - `internal/lyxcwd/docslink_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add the remaining subtests of `TestEnforcement_MarkdownLinks`, each building its own tree under `t.TempDir()` and calling `docsLinkScan(t, tmpRoot, []string{"."}, allow)` with a locally-constructed allowlist.
  Do **not** place any fixture under a directory whose name contains `testdata` — `walkEnforcementRoots` skips those (`internal/lyxcwd/enforcement_test.go:128`), so such a fixture would walk to zero files and every subtest built on it would pass vacuously.
  Cover, one subtest or table case each: a relative link to an existing file with no fragment resolves; a relative link to a missing file produces a `missing file` break; a `#fragment` matching a heading in the target file resolves; a `#fragment` with no matching heading produces a `missing anchor` break; a same-file fragment link resolves against the containing file's own headings; `http://`, `https://`, and `mailto:` targets are skipped and never produce a break; an allowlisted `(file, target)` pair produces no break and leaves `unmatched` empty; a stale allowlist entry whose link now resolves is reported in `unmatched`; a stale allowlist entry whose keyed file no longer exists in the tree is reported in `unmatched`; link-shaped text inside a ``` fence and inside a `~~~` fence is ignored end-to-end through `docsLinkScan`; two identically-slugging headings in one file make `#foo` and `#foo-1` both resolve; and a link to a non-`.md` target that exists has its existence checked with no anchor check attempted, while a link to a missing non-`.md` target still produces a `missing file` break.
  The renamed-away stale case is the one a naive implementation silently passes — build it explicitly by allowlisting a `(file, target)` pair for a file the temp tree does not contain, and assert the key comes back in `unmatched`.
  The `repo` subtest from card 2 stays the only case using `repoRootForEnforcement(t)`.
  Every subtest added by this card must pass; the `repo` subtest remains red until card 8.
- **Commit:** `test(lyxcwd): cover link resolution, fences, dup anchors and stale allowlist entries`

### Card 5: finalize.md — the Raddle fold, the residue rewrite, and the link repairs

- **Context:**
  - `manifest/designs/shed.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/raddle.md`
  - `CONSTRAINTS.md`
  - `internal/fabricengine/doc.go`
  - `docs/overview.md`
  - `manifest/designs/shed-followups.md`
  - `internal/fabriccli/fabric.go`
  - `_mill/discussion.md`
  - `CLAUDE.md`
- **Edits:**
  - `manifest/designs/finalize.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Re-read `manifest/designs/finalize.md` end to end and remove all producer-model residue.
  The sites below are a starting inventory, not a bound — anything else asserting a reserved Raddle slot, a `Shed`-special-cased Finalize, or swappable Preflight/producer slots must go too.
  In the status blockquote at `:3`, replace "Finalize is **`Shed`'s** literally-shared code (identical for `loom` and `Hardener`, not a swappable per-instance slot the way Preflight and the producer are — see [shed.md](shed.md))" with the `finalize-shared-by-reference` framing: `Finalize` is an ordinary producer that both `loom`'s and `Hardener`'s producer lists name — one definition, named twice, never copied, and never something `Shed` special-cases.
  Keep the blockquote on one line per `CLAUDE.md`'s markdown rule.
  At `:11`, replace the `[fabric.md](fabric.md)` link with `` [`internal/fabricengine`](../../internal/fabricengine/doc.go) `` and drop the stale "absorbed into `fabric` once that lands" framing — the warp/fabric rename landed in `3a748e50` and fabric slice 10 landed in `fcb606f7`.
  At `:12`, rewrite "that's `warp cleanup`'s (future: `fabric`'s) existing, separate job" to name the shipped spelling `lyx fabric cleanup` with no "future" hedge; `internal/fabriccli/fabric.go` registers `cleanup` under the `fabric` command.
  Treat `:11` and `:12` as one sentence pair and rewrite them together.
  At `:26`, replace the prose citation "CONSTRAINTS.md's Weft Git Invariant" with the real markdown link `` [Fabric Git Invariant (warp + weft)](../../CONSTRAINTS.md#fabric-git-invariant-warp--weft) ``.
  No "Weft Git Invariant" exists; the real section is `## Fabric Git Invariant (warp + weft)`, and its "Orchestration, not agent" bullet carries the claim this line is making.
  Cite by section name via the anchor, never by line number.
  Add a new section after the existing `## Only Raddle forwards from child weft to parent weft — not `_lyx`` section, headed along the lines of `## Raddle regeneration — part of the merge, not a step before it`.
  It must state the fold in `shed.md:19` and `loom.md:65-67`'s own terms — Raddle-regeneration is scoped as part of the merge itself, because updating Raddle before the Finalize merge is impractical given merge-conflict risk — state that the merge lock Finalize takes must span the whole regeneration critical section (read parent's current HEAD, run the leaf-fork and `Overview.md` regeneration against it, commit via `SyncWeft`, as one atomic unit), and **point at** `raddle.md` for the regeneration mechanics rather than restating them.
  Do not restate the parallel-fork structure, the `Overview.md` sequencing, or the `SyncWeft` commit shape — `shed.md:25`'s pointer rule makes `raddle.md` the single place those live.
  Do not write the section in full producer-contract Input/Output form; `loom.md`'s table row 11 already carries Finalize's Input/Output.
  Write the section as the landed decision, not as the only conceivable arrangement, so it does not foreclose the deferred alternative recorded in `_mill/discussion.md`'s `raddle-as-own-producer-deferred`.
  At `:45-46`, rewrite "`Shed`'s literally-shared code ... both share this exact code" to the same shared-by-reference framing used at `:3`.
  At `:48`, replace "`Shed` hasn't been extracted from it yet (see that doc's own naming note)" with a description of the `shed.md`/`loom.md` split of authority — `shed.md` owns the generic mechanism, `loom.md` owns `loom`'s concrete producer list, per `shed.md:3` — phrased so it stays correct whether or not task E has landed its `loom.md:15-17` fix.
  At `:49`, demote the existing `raddle.md` `Related` bullet from decision-carrier to mechanics-pointer, since the new section now carries the decision.
  At `:52`, replace the second `[fabric.md](fabric.md)` link with `` [`internal/fabricengine`](../../internal/fabricengine/doc.go) ``.
  Do not touch `:9` — its `internal/websterengine` citation is already correct and the spec's claim that it references Builder is stale; `grep -in builder manifest/designs/finalize.md` returns zero hits.
  Do not rename `raddle.md`'s `## When it runs: deferred to merge-time, not mid-task` heading or change the `:28` link that targets its anchor.
  Keep the `Two merge targets` section's `warp`/`weft` vocabulary — `CONSTRAINTS.md`'s Fabric Vocabulary Invariant prose-doc split keeps it in a doc explaining fabric's own mechanism.
  Apply semantic line breaks to every line touched.
- **Commit:** `docs(finalize): fold Raddle into Finalize's contract and repair dead references`

### Card 6: raddle.md — close the open question and drop the slot framing

- **Context:**
  - `manifest/designs/shed.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/finalize.md`
- **Edits:**
  - `manifest/designs/raddle.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Re-read `manifest/designs/raddle.md` end to end and remove all reserved-phase-slot residue.
  The four sites below are a starting inventory, not a bound.
  At `:3` (the status blockquote), drop "Already has a reserved-but-unbuilt phase slot between Webster and Finalize" and state instead that Raddle-regeneration is folded into `Finalize`'s own contract, repointing the link from `[loom.md](loom.md#the-phase-machine)` to `[loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots)`.
  Keep the blockquote on one line.
  At `:7`, reword "living in `weft`: an always-run step after Webster" so the sentence describes what Raddle *is* — codeguide's weaving-vocabulary successor, living in `weft`, generating docs over the diff a plan produced, building heavily on Millhouse's `codeguide-update` — without asserting when it runs.
  The `## When it runs: deferred to merge-time, not mid-task` section three sections later owns the timing, and the current wording contradicts it.
  At `:54`, convert "**Open, not yet decided:** whether this removes raddle's reserved phase slot between Webster and Finalize ... Not resolved here." into the recorded decision: the fold happened, regeneration is part of the Finalize merge, and it landed at `shed.md:19` and `loom.md:65-67`.
  Point at `finalize.md`'s new fold section for Finalize's side of the contract, and repoint the same dead `loom.md#the-phase-machine` anchor to `loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots`.
  This edit sits inside the `## When it runs` section — change the body, keep the heading text byte-for-byte, because `finalize.md:28` links its anchor and the new enforcement test will catch a rename.
  At `:85` (the `Related` bullet), reword "[loom.md](loom.md) — where raddle's phase slot sits in the phase machine" to point at the flat producer list and the fold instead, since no slot exists.
  The link itself resolves and has no anchor, so the checker will never flag this one — it is review-caught, not machine-caught, and still must be fixed.
  Leave `:16`'s list of docs where the superseded hub-level framing survived alone unless the end-to-end re-read shows it has become false.
  Apply semantic line breaks to every line touched.
- **Commit:** `docs(raddle): record the Finalize fold and drop the reserved-slot framing`

### Card 7: self-report.md — repair the phase-machine anchor

- **Context:**
  - `manifest/designs/loom.md`
- **Edits:**
  - `manifest/designs/self-report.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Re-read `manifest/designs/self-report.md` end to end and remove any producer-model residue found.
  At `:30`, repoint `[loom.md](loom.md#the-phase-machine)` to `[loom.md](loom.md#the-phase-machine--a-flat-producer-list-no-predefined-slots)`.
  Check the surrounding sentence — "This mirrors the `Raddle` pattern" — still reads correctly against the flat producer list now that Raddle has no slot of its own; the mirrored property is the fresh-context agent reading only accumulated notes, which the fold does not change, so reword only if the end-to-end re-read shows the sentence asserting a slot.
  The `:48-49` `Related` bullet mentions "the `Raddle` pattern Tier 2's reflection step mirrors" — apply the same check there.
  Do not edit `manifest/designs/loom.md`.
  Apply semantic line breaks to every line touched.
- **Commit:** `docs(self-report): repair the dead loom.md phase-machine anchor`

### Card 8: the six unowned-file link repairs

- **Context:**
  - `internal/scoutengine/doc.go`
  - `manifest/roadmap.md`
- **Edits:**
  - `manifest/designs/semantic-index.md`
  - `manifest/designs/webster-parallel-execution.md`
  - `docs/shared-libs/README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Repair the six remaining dead links, in files no other task in the `shed-producer-model-scoping` set claims.
  Repoint all five `scout-redesign.md` references to `internal/scoutengine`'s package documentation in **linked** form — `` [`internal/scoutengine`](../../internal/scoutengine/doc.go) `` — never as an unlinked prose mention, so the sites stay inside the new checker's coverage.
  `scout-redesign.md` never existed in git history; scout shipped as `internal/scoutengine` plus `internal/scoutcli`, and `manifest/roadmap.md:219` already treats that package doc as scout's citation target.
  The five sites are `manifest/designs/semantic-index.md:3`, `:8`, `:54`, and `manifest/designs/webster-parallel-execution.md:54`, `:60`.
  This is **not** a find-and-replace — each site's surrounding prose describes a design doc and must be reworded in place so the sentence reads correctly against a package doc.
  `semantic-index.md:3` calls it "the 'deferred idea' scout-redesign.md refers to"; `:8` uses it as a parenthetical for `scout`; `:54` calls it "the precise, compiler-derived sibling"; `webster-parallel-execution.md:54` calls it "the direct ancestor of the scout proposal"; `:60` is a `Related` bullet labelled "Part B's successor".
  Do not repoint any of them at `manifest/designs/semantic-index.md` — that doc is scout's sibling, and `:3` and `:8` both describe the target as a separate thing they defer to, so it would be circular.
  In `docs/shared-libs/README.md:12`, change `[roadmap.md](../roadmap.md)` to `[roadmap.md](../../manifest/roadmap.md)` — from `docs/shared-libs/`, `../roadmap.md` resolves to the non-existent `docs/roadmap.md`.
  Editing this file is not the same as editing `manifest/roadmap.md`, which stays out of scope and must not be touched.
  Apply semantic line breaks to every line touched.
  After this card, run `go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` and confirm the `repo` subtest reports **zero breaks and zero unmatched** allowlist entries.
- **Commit:** `docs: repoint dead scout-redesign.md and roadmap.md links at live targets`

### Card 9: record the Markdown Link Integrity invariant

- **Context:**
  - `internal/lyxcwd/docslink_test.go`
  - `manifest/designs/finalize.md`
  - `README.md`
  - `CLAUDE.md`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a `## Markdown Link Integrity` section to `CONSTRAINTS.md`, placed after `## Fabric Git Invariant (warp + weft)` and before `## Review Round Invariant`, following the shape the surrounding sections already use (rule, bullets, an `- **Enforced by**` bullet last).
  The section must state five things.
  One: the rule — every inline markdown link in a `.md` file under `manifest/` or `docs/` resolves, both its file part and its `#anchor`.
  Two: the enforcing test — `internal/lyxcwd/docslink_test.go`'s `TestEnforcement_MarkdownLinks`.
  Three: a placement caveat in the same words the Fabric Vocabulary Invariant already uses at `CONSTRAINTS.md:177` — the test living in `internal/lyxcwd` is a **file-layout convenience** reusing that package's `repoRootForEnforcement` and `walkEnforcementRoots` helpers, and is **not an ownership claim**, because the Cwd Resolution Invariant scopes `internal/lyxcwd` to cwd resolution and nothing else.
  Four: what the check does and does not reach, stated honestly, mirroring the "What the machine check does and does not reach — stated honestly, not implying full coverage" framing at `CONSTRAINTS.md:178`.
  Not reached: external `http`/`https`/`mailto` URLs, never fetched; reference-style links and `<...>` autolinks, out of grammar by decision rather than oversight; link-shaped text inside fenced code blocks, deliberately skipped; prose mentions of a filename that are not markdown links, with `manifest/roadmap.md:98`'s `scout-redesign.md` reference named as a live example this task leaves standing; and `.md` files outside `manifest/` and `docs/` as **scan sources**, so `README.md`, `CLAUDE.md`, and `internal/**/*.md` have their own outgoing links checked by nobody.
  State explicitly, in the same paragraph, that the root restriction is **source-side only**: `manifest/` and `docs/` name which files are scanned for outgoing links and do not restrict where those links may point — every target is resolved wherever it lands in the repo, and any `.md` target gets its `#anchor` resolved too, inside those roots or not.
  Without that sentence an implementer could read the root restriction as licence to skip anchor resolution for out-of-root targets, which would silently un-guard `finalize.md`'s `../../CONSTRAINTS.md#fabric-git-invariant-warp--weft` link and the `../../internal/*/doc.go` targets this task creates.
  Five: the allowlist's self-expiring contract — keyed by `(file, target)` and never by line number, each entry naming its owning task, and an entry whose key goes unmatched by a scan reported as deletable.
  Apply semantic line breaks throughout.
- **Commit:** `docs(constraints): record the Markdown Link Integrity invariant`

### Card 10: README.md — drop Raddle from the phase chain

- **Context:**
  - `manifest/designs/shed.md`
  - `manifest/designs/loom.md`
  - `manifest/designs/shed-followups.md`
- **Edits:**
  - `README.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** At `README.md:93`, rewrite "**loom** — the phased orchestrator (Preflight → Discussion → Plan → Webster → Raddle → Finalize), each producing phase gated by a `perch` review." so it no longer lists `Raddle` as a phase of its own between Webster and Finalize.
  Raddle-regeneration is folded into `Finalize`'s contract per `shed.md:19` and `loom.md:65-67`, so the chain is Preflight, Discussion, Plan, Webster, Finalize.
  Keep the rest of the bullet's claim intact — each producing phase is still gated by a `perch` review — and keep the sentence on one line, matching the surrounding bullets' existing style.
  Check `:94-95`'s follow-on sentence in the same bullet ("Preflight is built; Discussion, Plan, the phase-machine skeleton, Finalize, and session bootstrap are still being built out.") still reads correctly after the chain edit and does not itself name a Raddle phase.
  Do not touch any other line of `README.md`.
  In particular, leave `:25`, `:86`, `:87`, `:94`, and `:115` alone — those were task A's (`builder-retire`, landed as `0149776a`) and are already done.
  This file is taken here because it is genuinely unowned: task A's `README.md` line list in `shed-followups.md:75` never included `:93`, task A has landed, and no other task in the six-task set names `README.md` at all, so this residue would otherwise ship unfixed and un-flagged — the checker cannot see it, since it is prose rather than a markdown link.
  Do not edit `manifest/designs/shed-followups.md`.
- **Commit:** `docs(readme): drop Raddle from loom's phase chain`

### Card 11: prove the checker resolves rather than trivially passing

- **Context:**
  - `internal/lyxcwd/docslink_test.go`
  - `manifest/designs/self-report.md`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Zero-diff verification card.
  Temporarily append a deliberately-broken inline link — target `no-such-file.md` — to `manifest/designs/self-report.md`, run `go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks`, and confirm the `repo` subtest fails naming that file, that line, `missing file`, and that target.
  Then repeat with a deliberately-broken anchor — target `loom.md#no-such-heading` — and confirm the failure reports `missing anchor`.
  Revert both edits with `git checkout -- manifest/designs/self-report.md` and re-run the full `go test ./internal/lyxcwd/` to confirm the package is green and `git status --porcelain` is clean.
  This card must leave no diff behind.
  Also run the batch's own `verify:` command, `go test ./internal/lyxcwd/`, and the repo-wide `go build ./... && go test ./...` once, so the batch hands off with the whole suite confirmed rather than only the one package.
  Additionally confirm by review, not by machine: `finalize.md` reads coherently end to end and its new fold section does not contradict the "Only Raddle forwards" section above it; no sentence in the three owned files still implies a reserved Raddle slot or a `Shed`-special-cased Finalize; `raddle.md:85`'s prose no longer asserts a phase slot; the five repointed `scout-redesign.md` sites read as references to a package doc rather than a design doc; and every edited `.md` follows semantic line breaks.
- **Commit:** none

## Batch Tests

`verify: go test ./internal/lyxcwd/` — the whole `lyxcwd` package, which is the new `TestEnforcement_MarkdownLinks` plus the existing `TestEnforcement`, `TestEnforcement_GeometryLiterals`, `TestEnforcement_FabricVocabulary`, and the leaf-invariant test.
This is a Go project, so the `PYTHONPATH= ` prefix rule does not apply.

Scoping the verify to the one package is correct here: `internal/lyxcwd/docslink_test.go` is the only Go file this batch touches, it is a test file with no production dependents, and the other eight edited files are markdown.
Running the package rather than the single test function is deliberate — the new file shares `repoRootForEnforcement` and `walkEnforcementRoots` with `enforcement_test.go`, so a compile-level mistake in the new file breaks the existing enforcement tests too, and the package-scoped run surfaces that immediately.

Card 11 additionally runs `go build ./... && go test ./...` once as a hand-off check, and `pipeline.done_gate` (`go test ./... && go test -tags integration ./...`) covers the repo-wide case at task completion.

The scenarios the test itself must cover are enumerated per card: card 1 covers the slug, extractor, and anchor-set grammar as table tests over literal data; card 4 covers link resolution, URL skipping, fences, duplicate-heading disambiguation, non-`.md` targets, and both stale-allowlist cases over `t.TempDir()` trees; card 2's `repo` subtest is the single real-repo case and is the one that must report zero.
Card 11 proves the `repo` subtest is resolving rather than trivially passing by breaking a real link and confirming the failure.
