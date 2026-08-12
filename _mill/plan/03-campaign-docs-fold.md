# Batch: campaign-docs-fold

```yaml
task: 'fabric: close the corrindex two-phase read-modify-write race (slice 15)'
batch: 'campaign-docs-fold'
number: 3
cards: 3
verify: go test ./internal/lyxcwd/...
depends-on: [2]
```

## Batch Scope

Slice 15 is the fourth and last of the fabric crucible follow-ups, which triggers `manifest/designs/fabric-crucible-followups.md`'s own stated documentation lifecycle: the file is deleted once all four have landed, with its durable rationale folded into `internal/fabricengine`'s package doc.
This batch is that lifecycle, and it is the largest deliverable by volume in the task — a 488-line design doc to distil, nine inbound references to resolve individually, and the roadmap's campaign disposition.
Per `CLAUDE.md` it cannot be deferred to a follow-up commit: docs land with the change that requires them.

It is one batch because the three cards share one context — the design file being retired, its fold target, and its reference graph — and must be ordered: card 5 folds while the source still exists, card 6 repoints the six non-roadmap references away from it, and card 7 resolves the roadmap and only then deletes the file, so the last dangling reference and the delete land in the same commit.

Batch-local decision beyond `## Shared Decisions`: **the fold is rationale-only, not forensics.**
Slice 12's existing fold (`doc.go`'s "The destruction chokepoint" section) is the house style — no evidence table, no round numbers, no campaign process history; it compresses all eight defects into one sentence.
Deliberately left behind: the per-round eight-defect evidence table, the gates-were-green-throughout observation, and the wrong-then-corrected harness-versus-chokepoint ordering argument.
The first two are forensics git history keeps; the third already lives in `manifest/roadmap.md`'s Planned item in substance, so folding it would duplicate a live doc rather than rescue an orphaned one.
Volume is the practical reason as well as the stylistic one: `doc.go` is already 644 lines against the source file's 488, and an untrimmed fold roughly doubles a package doc already at the edge of readable.

## Cards

### Card 5: fold slice 15's durable rationale into `internal/fabricengine`'s package doc

- **Context:**
  - `manifest/designs/fabric-crucible-followups.md`
  - `internal/fabricengine/corrindex.go`
  - `internal/fabricengine/index.go`
  - `internal/state/doc.go`
- **Edits:**
  - `internal/fabricengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add one new godoc section to `internal/fabricengine/doc.go`, placed immediately after the existing `# The destruction chokepoint` section and before the closing `package fabricengine` clause, titled `# The correspondence index's write path`.
  Match that section's voice exactly: bolded `**Why …**` paragraph leads, rationale a reader cannot recover from the code, no process history.

  Cover, and nothing more:
  why `record()` is single-phase — the two-phase load-then-write window it used to have, and that `state.UpdateJSON` closes it by re-reading the on-disk base under the same exclusive lock it writes under;
  why the preferred "re-read under the write lock `record()` already takes" shape was not directly implementable — `record()` takes no write lock in its own frame, `state.WriteJSON` acquires and releases the lock internally, and `lock.AcquireWriteLock` opens a fresh `flock` handle per call, so a nested acquire from `corrindex.go` self-deadlocks;
  why `RebuildIndex` and `refreshCorrIndexAfterSwitch` were left alone — giving them the weft write lock is a claim about every call path that every future caller must preserve, versus a local fact, and it would still leave `record()` two-phase against any writer that does not take the weft lock;
  why `refreshCorrIndexAfterSwitch`'s unlocked `os.Remove`-then-rebuild window is *designed* rather than defective — the discard is intended to drop cross-branch entries that would otherwise keep passing `SHAExists`, so a concurrent `record()` losing its entry there is the intended behaviour;
  and the residual window, by name: `RebuildIndex` is itself two-phase (`scanWarpSHATrailers` reads git, then `state.WriteJSON` writes) with the scan outside the file's lock, so the interleaving *scan → `record()` writes → rebuild writes* still loses the recorded entry.
  State that residual as accepted, not overlooked, and give the reason: LOW severity and self-healing, because the weft commit trailers are the sole source of truth and the index is an explicitly rebuildable cache, so the worst observable effect is one spurious `no_weft_correspondence` from `lyx fabric diff` that a re-run clears.
  Do not write anything implying the race is closed in both directions.

  Read `internal/fabricengine/doc.go` for what campaign framing it already carries.
  Add a framing sentence naming the crucible campaign's four follow-up slices as complete **only if** the file does not already carry that framing;
  if it does, do not restate it.

  Do **not** fold: the eight-defect evidence table by round and verb, the gates-were-green-throughout observation, or the harness-versus-chokepoint ordering argument.
  Keep the whole addition proportionate to the "The destruction chokepoint" section's per-topic density — this is a distillation, not a move.
- **Commit:** `docs(fabric): fold slice 15's locking rationale into the package doc`

### Card 6: resolve the six non-roadmap inbound references

- **Context:**
  - `internal/fabricengine/doc.go`
  - `internal/lyxcwd/docslink_test.go`
  - `manifest/designs/fabric-crucible-followups.md`
  - `manifest/roadmap.md`
- **Edits:**
  - `manifest/designs/fabric-unified-view.md`
  - `manifest/designs/fabric-windows-verification.md`
  - `manifest/designs/gitexec-error-shape.md`
  - `manifest/designs/lyxtest-real-hubs.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Resolve each of the six inbound references to `fabric-crucible-followups.md` outside `manifest/roadmap.md`, with the verb named per reference.
  The nine references are not a uniform swap — four repoint cleanly, two need rewriting because they name things `doc.go` does not contain or are stale on landing whatever they point at.

  **Repoint** — retarget at `internal/fabricengine`'s package doc, naming the target section in prose rather than by anchor, per `## Shared Decisions`:
  `manifest/designs/lyxtest-real-hubs.md`'s "see … 's slice 13, where the hermetic suite stayed green" sentence in the `## Why` section;
  `manifest/designs/fabric-windows-verification.md`'s "the hermetic suite was green throughout … see … 's slice 13" sentence in the `## Why this is worse than a normal coverage gap` section;
  `manifest/designs/fabric-unified-view.md`'s `## Related` bullet reading "now scoped as slices 12-15 in …", whose tense must also be corrected to landed;
  and `manifest/designs/gitexec-error-shape.md`'s `## Related` bullet reading "the four fabric-local classes from the same campaign, scoped as slices 12-15".

  **Rewrite** — these two cannot be repointed, because the fact they cite does not live at the new target:
  `manifest/designs/lyxtest-real-hubs.md`'s status-blockquote line "See … 's build order." — `doc.go` has no build order, and the chain is complete on landing.
  Either drop the pointer or restate the sequencing fact inline; if you restate it, the surrounding blockquote's claim that this doc is sequenced behind the whole fabric chain must read as satisfied rather than pending.
  `manifest/designs/fabric-windows-verification.md`'s `## Related` bullet "the four Planned slices from the same campaign" — stale on landing whatever it points at, since all four are Done.
  Rewrite it to name the landed campaign and, if a pointer is still useful, aim it at `internal/fabricengine`'s package doc; slice 13's harness still inherits the Windows gap honestly, so that half of the bullet survives.

  Every repointed link target is `../../internal/fabricengine/doc.go` from `manifest/designs/`.
  Never append a `#anchor` fragment to it.
  Do not edit `manifest/roadmap.md` in this card and do not delete `manifest/designs/fabric-crucible-followups.md` yet — both are card 7's, so the file still exists while this card runs.
  Keep semantic line breaks in every edited paragraph.
- **Commit:** `docs(manifest): repoint the fabric campaign design-doc references at the package doc`

### Card 7: resolve the roadmap's campaign disposition and delete the design file

- **Context:**
  - `internal/fabricengine/doc.go`
  - `manifest/designs/fabric-crucible-followups.md`
  - `CONSTRAINTS.md`
  - `_mill/discussion.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/fabric-crucible-followups.md`
- **Moves:** none
- **Requirements:** Resolve `manifest/roadmap.md`'s campaign disposition and delete the design file in this one commit, so the last dangling reference and the delete land together.

  **Delete the whole Planned item** titled "fabric: crucible follow-ups — slices 14-15", not just its stale sentences.
  With all four slices landed there is no planned work left to describe, and the per-slice Done entries already carry the as-built summaries.
  Do **not** leave a residual Planned entry for the still-open `RebuildIndex` scan-then-write window: it is an accepted residual of a LOW self-healing defect, documented by card 5's fold, not a scheduled item.
  Deleting the item also removes the first of the three "Full task bodies live at …" sentences.

  **Add a Done entry for slice 15**, following the Done section's existing per-slice shape (a bold item name, then an as-built summary).
  Word it to say the `record()` side is closed, never that the race is closed: name `state.UpdateJSON` as the new read-modify-write primitive holding one exclusive lock across read, mutate and atomic write; say `corrIndex.record` now applies its upsert to the freshly-read on-disk base rather than a stale in-memory snapshot; and name the residual explicitly — `RebuildIndex`'s own scan-then-write span is still two-phase, so the reverse interleaving can still lose an entry, accepted as a LOW self-healing residual documented in `internal/fabricengine`'s package doc.
  Record that no `CONSTRAINTS.md` invariant was added and why (adoption is deliberately one consumer, so a universal-use invariant would be false on landing), and that the rule lives in `internal/state`'s package doc instead.
  Add no `designs/` link — the doc is deleted, and per the Maintenance section Done entries do not link to deleted design docs.

  **Fix slice 12's Done entry**, which ends "Slices 14-15 remain — see Planned above" — false once the Planned item is gone.
  Replace it with a statement that all four slices have landed, or drop the sentence.

  **Delete the two remaining "Full task body lives at …" sentences**, one in slice 13's Done entry and one in slice 14's Done entry.
  After the delete those bodies live nowhere, and the Done entries already carry full as-built summaries, so nothing is lost.

  **Delete `manifest/designs/fabric-crucible-followups.md`** with `git rm`.

  Finally, confirm the delete left nothing dangling: `grep -rn "fabric-crucible-followups" --include=*.md --include=*.go . | grep -v '^./_mill/'` must return zero hits.
  The `_mill/` exclusion is required, not cosmetic — this task's own `_mill/discussion.md` is committed on the branch and names the file repeatedly, so an unscoped grep can never reach zero.
  Keep semantic line breaks in every edited paragraph.
- **Commit:** `docs(manifest): retire the fabric crucible follow-ups design doc and close slice 15`

## Batch Tests

`verify: go test ./internal/lyxcwd/...` is the batch's gate because `internal/lyxcwd/docslink_test.go` is what actually enforces the Markdown Link Integrity Invariant over every repointed reference and over the deleted file — a dangling relative link or a missing anchor fails there, and nothing else in the suite would catch it.
It also covers the anchor trap this batch's shared decision guards against only partially: that test skips the anchor check for non-`.md` targets, so a `doc.go#some-section` fragment would pass it while being dead on GitHub, which is why the plan bans the fragment outright rather than relying on the gate.
The scope is `internal/lyxcwd` rather than the full suite because this batch edits one `.go` file (a package comment in `doc.go`, no code) and four `.md` files.
The overview's module-wide `verify: go build ./...` covers `doc.go` still compiling.
Card 7's closing grep is a manual completeness check with no test home;
it is stated in the card as an explicit step because the delete's correctness is not otherwise observable.
The repo-wide done gate (`go test ./... && go test -tags integration ./...`) picks up anything outside these scopes.
