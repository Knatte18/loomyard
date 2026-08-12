# Discussion: fabric: close the corrindex two-phase read-modify-write race (slice 15)

```yaml
task: 'fabric: close the corrindex two-phase read-modify-write race (slice 15)'
slug: fabric-corrindex-record-race
status: discussing
parent: main
```

## Problem

`internal/fabricengine/corrindex.go`'s `record()` composes the array it is about to write from `ix.recs` — an in-memory snapshot that `loadCorrIndex` read earlier under a read lock **that has already been released** — and then writes that whole array via `state.WriteJSON`.
The write itself is atomic and flock'd, so the file is never torn;
the defect is the window between the load and the write.
Any other process that wrote the same file inside that window has its entry clobbered, because `next` was composed from a base that no longer reflects what is on disk.
`RebuildIndex` (`internal/fabricengine/index.go:416`) writes the same file under the same file lock but **not** under the weft write lock, so it does not serialise against that window either.
A `commit` racing a `diff`/`revert` on one pair can therefore transiently drop an index entry.

Severity is LOW and self-healing, and this is filed for the locking decision it implies rather than for urgency.
The weft commit trailers are the sole source of truth and the correspondence index is an explicitly rebuildable cache over them, so a dropped entry is reconstructed by the next stale-hit rebuild;
the worst observable effect is one spurious `no_weft_correspondence` from `lyx fabric diff` that a re-run clears.
Verification status is stated plainly: **CONFIRMED by code inspection by two independent parties (crucible round R6 and the campaign orchestrator), NOT reproduced as a runtime failure** — neither party could make it fail on demand.

**Why now:** this is slice 15, the last of the four fabric crucible follow-ups, and slice 14 landed in `d56b57f7`.
It was sequenced at the tail purely because slices 12 and 14 rewrote `internal/fabricengine` package-wide and two agents in that package at once is a merge conflict, not a schedule win.
That blocker is now gone.
Because it is the last of the four, landing it also triggers `manifest/designs/fabric-crucible-followups.md`'s own stated documentation lifecycle: the file is deleted once all four have landed, with its durable rationale folded into `internal/fabricengine`'s package doc.

## Scope

**In:**

- Add `state.UpdateJSON` to `internal/state` — a read-modify-write primitive that holds one exclusive lock across read, mutate, and atomic write.
  This includes extracting lock-free cores out of `ReadJSON` and `WriteJSON` for all three to sit on, so those two existing exported functions' bodies change even though their behaviour does not.
- Rewrite `corrIndex.record` (`internal/fabricengine/corrindex.go:48`) to go through `state.UpdateJSON`, so its upsert applies to the freshly-read on-disk base rather than to a stale `ix.recs` snapshot.
- A deterministic Tier-1 test in `internal/fabricengine/corrindex_test.go` reproducing the `record()`-versus-external-write clobber that production can actually hit.
- Unit tests for `state.UpdateJSON` in `internal/state`, including its own concurrency property.
- Document the read-modify-write rule in `internal/state`'s package header.
- Fold the durable rationale of `manifest/designs/fabric-crucible-followups.md` into `internal/fabricengine/doc.go`, delete that design file, and resolve all **nine** inbound references — per-reference, with an explicit verb each (repoint / rewrite / delete), per the table in Technical context.
- Move `manifest/roadmap.md`'s slice-15 entry from Planned to Done, worded to say the `record()` side is closed rather than that the race is closed.

**Out:**

- `RebuildIndex`'s locking is unchanged — it keeps writing under the file's own lock and does **not** acquire the weft write lock.
- **The `RebuildIndex` scan-then-write residual window stays open, by name and by choice.**
  `RebuildIndex` is itself two-phase — `scanWarpSHATrailers` reads git, then `state.WriteJSON` writes (`index.go:333` then `:416`) — and the scan is not under the index file's lock.
  So the interleaving *scan → `record()` writes → rebuild writes* still loses the recorded entry, because the rebuild's payload was computed before that entry's trailer existed.
  This task closes the opposite direction only (*rebuild writes → `record()` writes from a pre-rebuild base*), which `UpdateJSON` fixes by re-reading under the lock.
  Same shape, same LOW severity, same self-healing property — accepted, not overlooked.
- `refreshCorrIndexAfterSwitch`'s unlocked `os.Remove` + rebuild (`internal/fabricengine/index.go:315-318`) is left as-is, deliberately (see Decisions).
- The other locked-JSON read-modify-write sites are **not** migrated to `state.UpdateJSON` — **five sites across four files**: `internal/treadleengine/state.go:110`→`117` and `:151`→`172`, `internal/boardengine/store.go:55`→`76`, `internal/reedengine/state.go:57`→`71`, `internal/websterengine/state.go:216`→`236`.
- No new `CONSTRAINTS.md` invariant.
- No change to the correspondence index's on-disk JSON format, to `corrEntry`, or to `exact`/`nearestAtOrBefore`/`entries`.
- No change to any call site of `record()`, `RecordCorrespondence`, `WeftSHAForWarpSHA`, or `resolveRevertTarget`.
- `docs/shared-libs/README.md:37`'s one-line description of `internal/state` ("generic locked typed JSON I/O") stays accurate and needs no edit.

## Decisions

### fix-shape-is-a-state-level-update-primitive

- Decision: add `state.UpdateJSON` and make `record()` call it, rather than making `RebuildIndex` and `refreshCorrIndexAfterSwitch` take the weft write lock.
- Rationale: the task brief's preferred shape — "make `record()` single-phase by re-reading under the write lock it already takes" — is **not directly implementable as written**, and this is the single most important finding of exploration.
  `record()` takes no write lock in its own frame.
  `state.WriteJSON` (`internal/state/state.go:34`) acquires and releases `path+".lock"` internally, and `lock.AcquireWriteLock` calls `flock.New(lockPath)` fresh on every call (`internal/lock/lock.go:21`), so a nested acquire from `corrindex.go` opens a second file descriptor on the same file and self-deadlocks.
  There is no read-modify-write-under-one-lock primitive in `internal/state` today.
  So the preferred shape requires **adding** that primitive;
  the change stays small and additive, but it is not confined to `corrindex.go`.
  With `UpdateJSON`, `record()` holds the exclusive lock across read + upsert + atomic write, with no cross-path reasoning required.
  State the resulting guarantee precisely, because the loose version of it is wrong: `record()` is serialised against every other *write* to that file, so it can no longer compose its payload from a base another writer has already superseded.
  It is **not** serialised against `RebuildIndex`'s scan-to-write span, which is where the other direction's loss happens — see the residual window named in `Out`.
  This task closes `record()`'s own two-phase window;
  it does not close the race in both directions.
- Same-decision consequence for `internal/state`: `UpdateJSON` **cannot** be composed from `ReadJSON` + `WriteJSON`.
  Both acquire `lockPath` internally, so an implementation that acquires the lock and then calls them self-deadlocks by the exact mechanism this decision rests on — and it hangs rather than failing, which is the worst way to discover it from inside a test that also holds the lock.
  The real change is therefore a small refactor: extract lock-free cores (`readJSONUnlocked` / `writeJSONUnlocked`, or equivalent) and re-express `ReadJSON`, `WriteJSON` and `UpdateJSON` on top of them.
  Behaviour of the two existing exported functions is unchanged, but their bodies are not.
- Rejected: giving `RebuildIndex` and `refreshCorrIndexAfterSwitch` the weft write lock.
  It is deadlock-free today — exploration confirmed R6's ordering claim still holds in current code, with `pull.go:303` calling `RebuildIndex` before taking the weft write lock at `pull.go:351`, and the only weft-write-lock acquisitions in the package being `commit.go:182`, `pull.go:351` and `weftgit.go:259` (`coalesce.go:24` and `gitexclude.go:74` are unrelated locks), while `Diff`, `Revert` and `Checkout` — the three paths reaching `RebuildIndex`/`refreshCorrIndexAfterSwitch` — hold no weft lock.
  But that is a claim about *every* call path that every future caller must preserve, versus a local fact;
  and it would still leave `record()` two-phase against any writer that does not take the weft lock.
  Also rejected: doing both, which buys nothing once `record()` is single-phase and pays the cross-path claim's ongoing cost.

### updatejson-signature-mirrors-readjson

- Decision: `func UpdateJSON[T any](path, lockPath string, mutate func(cur T, found bool) (T, error)) error`.
  It acquires one exclusive lock on `lockPath`, reads `path` (missing file yields the zero `T` and `found=false`), calls `mutate`, writes the returned value atomically, and releases.
  A `mutate` error aborts with no write.
- Rationale: the flag carries information the callback cannot otherwise recover.
  For a slice type like `[]corrEntry`, a missing file and an on-disk empty array both arrive as an empty `cur`, so without `found` the mutate function is structurally unable to tell them apart.
  That argument is local and survives "we never migrate the other five sites."
  Mirroring `ReadJSON`'s existing `(T, bool, error)` shape is a secondary benefit — it keeps the un-migrated read-modify-write sites migratable later without a signature change (`internal/treadleengine/state.go:110` branches on exactly that flag) — but it is not the load-bearing reason, since speculative generality for a flag the only current consumer discards would not justify it on its own.
  `corrindex` does discard it, as it already does at `corrindex.go:36`.
- Rejected: dropping the `found` parameter (one param leaner for the only current consumer, but forecloses the sites that need the distinction);
  an in-place `mutate func(*T) error` form (awkward for a slice type, and it hides the contract that the written value is the returned one).

### record-keeps-its-method-shape

- Decision: `record` keeps its signature `func (ix *corrIndex) record(e corrEntry) error`.
  Internally it calls `state.UpdateJSON` over `[]corrEntry`, applying its existing upsert-by-`WarpSHA` plus stable sort by `WarpSeq` to the **freshly-read on-disk base** instead of to `ix.recs`, then assigns the written result back to `ix.recs`.
- Rationale: no call site and no existing test changes;
  `exact`, `nearestAtOrBefore` and `entries` are untouched;
  and the persist-before-in-memory-update ordering the current doc comment promises ("write failure leaves index unchanged") is preserved, since `ix.recs` is only assigned after `UpdateJSON` returns nil.
  One behavioural consequence must be documented rather than left implicit: after a successful `record()`, `ix.recs` now reflects on-disk truth and may therefore contain entries another process recorded, where previously it could only contain the loading snapshot plus this call's own entry.
  That is a strict improvement — the handle converges on the file rather than drifting from it — and no caller depends on the narrower behaviour.
- Rejected: replacing the method with a free `recordCorrEntry(path, e)` function.
  It is more honest that `ix.recs` is no longer a write base, but it churns every call site and test for no behavioural gain, and the handle is still needed for `exact`/`nearestAtOrBefore`/`entries`.

### refresh-after-switch-window-stays-open

- Decision: leave `refreshCorrIndexAfterSwitch`'s unlocked `os.Remove(path)`-then-`RebuildIndex` sequence (`internal/fabricengine/index.go:315-318`) unchanged, and document why.
- Rationale: this is a second, distinct unlocked window on the same file, but the discard is *intended* to drop entries — its whole purpose is to make the index fail-safe after a coordinated branch switch by deleting cross-branch entries that would otherwise keep passing `SHAExists` and serve answers the current branch's trailer history would never produce.
  A concurrent `record()` losing its entry there is the designed behaviour, not this bug;
  and the index is rebuildable regardless.
- Rejected: holding `path+".lock"` across the remove and the rebuild.
  It needs a lock-scoped delete primitive that does not exist, plus care against `RebuildIndex`'s own `WriteJSON` re-acquiring the same lock (which would self-deadlock for the reason recorded under the fix-shape decision) — real work for no observable gain.

### updatejson-adoption-stays-at-one-consumer

- Decision: add the primitive and use it in `corrindex.go` only;
  do not migrate the other five read-modify-write sites in this task — five sites across four files, since `internal/treadleengine/state.go` carries two independent pairs.
- Rationale: each of the five has its own concurrency story to establish, and this slice is explicitly scoped LOW.
  Widening it to four further packages inverts the risk/payoff of a self-healing race fix.
- Rejected: migrating every read-modify-write site in one pass.

### no-new-constraints-invariant

- Decision: add no invariant to `CONSTRAINTS.md`.
  State the rule — a locked-JSON read-modify-write must hold one lock across read and write, which is what `UpdateJSON` is for — in `internal/state`'s package header comment instead.
- Rationale: because adoption stays at one consumer, an invariant of the form "every locked-JSON read-modify-write goes through `UpdateJSON`" would be false on the day it lands, and a false invariant is worse than none.
  The package header is where the next author writing a read-modify-write over `internal/state` actually meets the rule.
- Rejected: adding the invariant, which would force migrating the other five sites and reopen the adoption decision.

### fold-and-delete-the-design-file

- Decision: fold a **trimmed** rationale from `manifest/designs/fabric-crucible-followups.md` into `internal/fabricengine/doc.go`, delete the design file, resolve each of the nine inbound references individually (repoint / rewrite / delete — see the table in Technical context), and move `manifest/roadmap.md`'s slice-15 entry from Planned to Done — all in this task's commits.
  Two sub-decisions matter more than the headline.
  **The fold is rationale-only, not forensics.**
  Slice 12's existing fold (`doc.go:564-644`) is the house style, and it contains no evidence table, no round numbers and no campaign process history — it compresses all eight defects into one sentence at `doc.go:573`.
  So the fold takes slice 15's own locking rationale, the residual window named in `Out`, and whatever campaign framing `doc.go` does not already carry.
  It does **not** take the per-round evidence table, the gates-were-green-throughout observation, or the wrong-then-corrected harness-versus-chokepoint ordering argument — that last one is planning history and already lives in `manifest/roadmap.md:20-23` in substance, so folding it would duplicate a live doc rather than rescue an orphaned one.
  Git history and the roadmap keep the forensics.
  Volume is the practical reason as well as the stylistic one: `doc.go` is already 644 lines against the source file's 488, and an untrimmed fold roughly doubles a package doc already at the edge of readable.
  **The nine references are not a uniform swap.**
  Only four are genuine "see the rationale" links that repoint cleanly;
  the rest name things `doc.go` does not contain (task bodies, a build order) or are stale on landing regardless of target.
  Note this is a different question from the rejected alternative below: "repoint the references at roadmap's Done entries" and "should this reference survive at all" are not the same decision.
- Rationale: the file's own line 5 states this lifecycle explicitly ("deleted once all four have landed, with their durable rationale folded into `internal/fabricengine`'s package doc"), and slice 15 is the fourth.
  It matches `docs/overview.md#documentation-lifecycle`, which makes module-design docs deletable on landing with rationale moving to the Go package header.
  The file is also already stale — its line 3 status header reads "slices 12-13 landed (2026-08-11); slices 14-15 not yet built" while slice 14 landed in `d56b57f7` — and nothing else will pick this up.
  Slice 12 already folded its own share of the rationale into `doc.go`'s "The destruction chokepoint" section, so the fold target and its house style already exist.
- Rejected: repointing every inbound reference at `manifest/roadmap.md`'s Done entries instead (roadmap carries per-slice as-built summaries, but it is a planning doc, not the durable home the lifecycle names — and this is orthogonal to deleting the three roadmap pointer sentences, which is a survival question, not a target question);
  keeping the file and folding only slice 15 (contradicts its own stated lifecycle);
  folding the file wholesale (contradicts the house style it cites, and duplicates `roadmap.md:20-23`).

## Technical context

**The defect, precisely.**
`corrindex.go:35` `loadCorrIndex` reads via `state.ReadJSON[[]corrEntry](path, path+".lock")` and the read lock is released before it returns.
`corrindex.go:48` `record` builds `next` from `ix.recs` (skipping any entry whose `WarpSHA` matches, appending `e`, stable-sorting by `WarpSeq`) and writes it with `state.WriteJSON(ix.path, ix.path+".lock", next)`.
Between those two calls the file is unprotected.

**Why the brief's "re-read under the write lock it already takes" cannot be done in place.**
`internal/state/state.go:28-46` — `WriteJSON` calls `lock.AcquireWriteLock(lockPath)` and `defer l.Release()` internally.
`internal/lock/lock.go:20-27` — `AcquireWriteLock` does `flock.New(lockPath)` then `fl.Lock()`, a fresh handle (and therefore a fresh open file description) per call.
Two such handles on the same file mutually exclude even within one process, on both Linux `flock(2)` and Windows `LockFileEx`.
So `corrindex.go` cannot wrap a `WriteJSON` call in its own `AcquireWriteLock` on the same lock path.
That mutual exclusion is also what makes the reproducing test in Testing below work in-process.

**Files to change.**

- `internal/state/state.go` — extract lock-free cores from `ReadJSON`/`WriteJSON` (their bodies change, their behaviour does not), add `UpdateJSON` on top of those cores, and extend the package header comment with the read-modify-write rule.
  Do **not** implement `UpdateJSON` as acquire-then-call-`ReadJSON`/`WriteJSON`: both acquire `lockPath` internally, so that composition hangs rather than failing.
- `internal/state/state_test.go` (or a new sibling) — unit cover for `UpdateJSON`.
- `internal/fabricengine/corrindex.go` — rewrite `record`'s body; update its doc comment, which currently says "Persists before in-memory update so write failure leaves index unchanged" and must now also state that the upsert base is the on-disk file and that `ix.recs` converges on disk truth.
- `internal/fabricengine/corrindex_test.go` — add the concurrency test.
- `internal/fabricengine/doc.go` — fold in the durable campaign rationale.
- `manifest/designs/fabric-crucible-followups.md` — delete.
- `manifest/roadmap.md` — slice 15 Planned → Done, plus the three pointer sentences at lines 33, 208, 215.
- `manifest/designs/fabric-windows-verification.md` (lines 34, 73), `manifest/designs/gitexec-error-shape.md` (line 510), `manifest/designs/fabric-unified-view.md` (line 228), `manifest/designs/lyxtest-real-hubs.md` (lines 7, 20) — per the reference table below.

**The nine inbound references, with the verb for each.**
There are **nine**, not eight — the count is easy to get wrong because two files carry two each and the roadmap carries three.

| reference | what it says | verb |
|---|---|---|
| `manifest/roadmap.md:33` | "Full task bodies live at …" | **delete the sentence** — after the delete the task bodies live nowhere, and `doc.go` carries rationale, not task bodies. The Planned/Done entries already carry full as-built summaries, so nothing is lost. |
| `manifest/roadmap.md:208` | same sentence, slice 13 Done entry | **delete the sentence** — same reason. |
| `manifest/roadmap.md:215` | same sentence, slice 14 Done entry | **delete the sentence** — same reason. |
| `manifest/designs/lyxtest-real-hubs.md:7` | "See … 's build order." | **rewrite** — `doc.go` has no build order, and the chain is complete on landing. Drop the pointer or restate the sequencing fact inline. |
| `manifest/designs/fabric-windows-verification.md:73` | "the four Planned slices from the same campaign" | **rewrite** — stale on landing whatever it points at; all four are Done. |
| `manifest/designs/lyxtest-real-hubs.md:20` | "see … 's slice 13, where the hermetic suite stayed green" | **repoint** at `internal/fabricengine`'s package doc. |
| `manifest/designs/fabric-windows-verification.md:34` | "the hermetic suite was green throughout … see … 's slice 13" | **repoint**. |
| `manifest/designs/fabric-unified-view.md:228` | "now scoped as slices 12-15 in …" | **repoint**, with tense corrected to landed. |
| `manifest/designs/gitexec-error-shape.md:510` | "the four fabric-local classes from the same campaign" | **repoint**. |

Every repointed link must **name the target section in prose** rather than relying on an anchor — e.g. "see `internal/fabricengine`'s package doc, \"The destruction chokepoint\"", which is how the followups file itself does it today.
`internal/lyxcwd/docslink_test.go:333` skips the anchor check for non-`.md` targets, so a `doc.go#some-section` anchor would pass the checker while being a dead link on GitHub.
The same test at `:210` already pins `../../internal/fabricengine/doc.go` as a first-class case, so a bare `.go` target does not trip the Markdown Link Integrity Invariant.

**Call graph worth knowing.**
`record()` has exactly one production caller: `Fabric.RecordCorrespondence` (`index.go:108`), which loads the index then records.
`RecordCorrespondence` is called from `commitEmptySnapshot` (`weftgit.go:180`) and `commitWeftLocked` (`weftgit.go:240`), both of which run under the weft write lock — `commitWeft` takes it at `weftgit.go:259` and delegates, `commitBothSides` takes it at `commit.go:182` before any commit, and `pull.go`'s re-anchor calls `commitEmptySnapshot` at `:358` under the lock taken at `:351`.
There is no third caller.
`RebuildIndex` is called from `pull.go:303`, `revert.go:65`, `index.go:156` (`WeftSHAForWarpSHA`'s stale-hit self-correction), and `refreshCorrIndexAfterSwitch` (`index.go:318`), itself called from `checkout.go:125`.
None of those four hold the weft write lock at the point of call.

**This call graph determines which race is real, and therefore what the test must assert.**
The weft write lock and the correspondence index are both per-pair, and every `record()` path runs under that lock, so **two `record()` calls against one index file cannot overlap in production**.
A test of `record()` versus `record()` would fail before the fix and pass after while proving nothing about production.
The interleaving that *can* occur is `record()` versus `RebuildIndex` — exactly what the Problem section describes as "a `commit` racing a `diff`/`revert`" — because `RebuildIndex`'s four callers hold no weft lock.
The test in Testing below is aimed at that axis, and it needs no goroutines to hit it.

**Durable content to fold into `doc.go` — and what is deliberately left behind.**

Fold: slice 15's own locking rationale (why `record()` is single-phase, why `RebuildIndex` was left alone, and the residual window named in `Out`);
and any campaign framing `doc.go` does not already carry.

Do **not** fold: the eight-defect evidence table by round and verb;
the gates-were-green-throughout observation;
the wrong-then-corrected "harness first versus chokepoint first" ordering argument.
The first two are forensics that git history keeps, and the third already lives in `manifest/roadmap.md:20-23` in substance — folding it duplicates a live doc rather than rescuing an orphaned one.
`doc.go:573` already compresses the whole eight-defect class into one sentence, which is the level the house style pitches at.

Fold alongside `doc.go`'s existing "The destruction chokepoint" section (`doc.go:564-644`, slice 12's own fold) and in the same voice: rationale, not process history.
Watch the volume — `doc.go` is 644 lines and the source design file is 488.

**Layering gotcha.**
`corrindex.go`'s header comment records that keeping git out of that file is what lets its tests stay untagged Tier-1 under the Test Tier Purity Invariant.
The new test must honour that: explicit `t.TempDir()` paths, no git spawn, no build tag.

## Constraints

From `CONSTRAINTS.md` and the repo's own docs:

- **Test Tier Purity Invariant** — `corrindex_test.go` is untagged Tier-1 and must stay git-free;
  the new concurrency test uses `t.TempDir()` paths only.
- **Documentation Lifecycle** (`CONSTRAINTS.md:410`, pointing at `docs/overview.md#documentation-lifecycle`) — module-design docs are deleted when their module lands, with rationale moving into the Go package header.
  This is what mandates the fold-and-delete above.
- **Task completion — docs land in the same commit** (`CLAUDE.md`) — a change touching a module doc, `docs/overview.md`'s module table, or a cross-cutting invariant updates them in the same commit.
  `docs/overview.md`'s module table and `docs/shared-libs/README.md:37` both stay accurate here, so neither needs an edit.
- **`manifest/roadmap.md` moves only on completing or adding a planned item** (`CLAUDE.md`) — slice 15 completes a planned item, so the roadmap does move.
- **Markdown: semantic line breaks, no fixed-column hard-wrap** (`CLAUDE.md`) — applies to every `.md` file touched, including the repointed inbound references.
- **Fabric Git Invariant (warp ↔ weft)** and **Fabric Destruction Chokepoint Invariant** — neither is touched by this change, but the fold into `doc.go` sits next to the second one's rationale.
- No new invariant is added, per the no-new-constraints-invariant decision.

## Testing

**`internal/state` — `UpdateJSON` unit tests (TDD candidate).**
Write these first;
the primitive is new, small, and fully specifiable before `corrindex` consumes it.
Scenarios that must be covered:

- Missing file: `mutate` receives the zero value with `found=false`, and the returned value is created on disk.
- Existing file: `mutate` receives the decoded value with `found=true`, and the returned value replaces it atomically.
- A `mutate` that returns an error aborts with the file unchanged (and, for the missing-file case, still absent).
- Concurrent `UpdateJSON` calls against one path from many goroutines, each appending a distinct element, all survive — this is `UpdateJSON`'s own concurrency property, and this is the right home for a goroutine test.
  Distinct `flock` handles mutually exclude in-process (see Technical context), so the exclusion under test is real.
  If this test is written as "each goroutine reads then updates", put a barrier between the read phase and the update phase — without one the pre-fix failure is only probabilistic, since a goroutine that reads after another's write sees that write and preserves it.
  Driving it through `UpdateJSON` itself avoids the issue entirely, which is the preferred shape.

**`internal/fabricengine/corrindex_test.go` — the reproducing test (the TDD candidate that matters).**
This converts the brief's "reasoned about but never driven" into a driven test, and it is worth writing before the fix so its pre-fix failure is observed rather than assumed.

It must reproduce `record()` versus an **external write**, not `record()` versus `record()` — see the call-graph note in Technical context for why the latter cannot happen in production.
That needs no goroutines and no barrier, which makes it deterministic rather than probabilistic:

```go
ix, _ := loadCorrIndex(path)                              // base: empty
state.WriteJSON(path, path+".lock", []corrEntry{other})   // stands in for RebuildIndex's write
ix.record(mine)                                           // today: clobbers `other`
// a fresh loadCorrIndex must observe BOTH entries
```

Keep it untagged and git-free, with an explicit `t.TempDir()` path.

It sabotage-proves cleanly, in the manner of slice 13's table: revert `record()` to composing from `ix.recs` and it fails on demand.

Note the residual window named in `Out` is **not** testable as a fix here and must not be asserted as one: the reverse interleaving (rebuild scans, `record()` writes, rebuild writes) still loses the entry after this change.

**Existing cover that must keep passing unchanged.**
`corrindex_test.go`'s round-trip, upsert-by-`WarpSHA`, sort-order and `nearestAtOrBefore` tests;
`index_integration_test.go` and the `checkout_index_refresh_test.go` refresh cover;
`pull`, `diff` and `revert` integration suites that exercise `RebuildIndex` and the stale-hit self-correction path.
No behavioural change is intended for any of them.

**Doc changes.**
Verify no dangling links remain after the delete — grep for `fabric-crucible-followups` across `.md` and `.go`, **excluding `_mill/`**, expecting zero hits.
The exclusion is required, not cosmetic: this discussion file is committed on the branch and mentions the name repeatedly, so an unscoped grep can never reach zero.

Also run `internal/lyxcwd`'s docslink test, which is what actually enforces the Markdown Link Integrity Invariant over the repointed references.

**Batching note.**
The docs half is the largest deliverable by volume, not a tail-end chore: one small primitive plus one small rewrite plus two test files on the code side, against a 488-line design doc to distil, nine references to resolve, and a roadmap move on the other.
Per `CLAUDE.md` it cannot be deferred to a follow-up commit either — docs land with the change that requires them.
Batch the plan accordingly.

## Q&A log

- **Q:** Which fix shape — a `state`-level update primitive, or the weft write lock on `RebuildIndex`/`refreshCorrIndexAfterSwitch`? **A:** The `state`-level primitive. The weft-lock version rests on a whole-package deadlock claim every future caller must preserve, and leaves `record()` two-phase against non-weft-locked writers.
- **Q:** Close `refreshCorrIndexAfterSwitch`'s unlocked delete-then-rebuild window too? **A:** No — leave it and document why. The discard is intended to drop entries, so a concurrent `record()` losing its entry there is designed behaviour, not this bug.
- **Q:** How far does `state.UpdateJSON` adoption go — corrindex only, or every locked-JSON read-modify-write site? **A:** corrindex only. The other five (across four files — `treadleengine` has two pairs) each have their own concurrency story, and this slice is scoped LOW.
- **Q:** Does a race never reproduced at runtime get a test that drives it? **A:** Yes — a Tier-1 test that fails today and passes after. Orchestrator review corrected its shape: the original goroutine `record()`-versus-`record()` design proved nothing, because every `record()` path runs under the weft write lock and cannot overlap in production. The test is `record()` versus an external write, sequential and deterministic; goroutines belong in `state.UpdateJSON`'s own cover.
- **Q:** Does this task close the race outright? **A:** No, and the discussion must not imply it does. It closes `record()`'s two-phase window; `RebuildIndex`'s scan-then-write span leaves a structurally identical window open, named in `Out` and reflected in the roadmap Done wording. Leaving it is a fine call for a LOW self-healing defect; implying it is fixed is not.
- **Q:** Slice 15 is the last of the four — does the fold-and-delete of `manifest/designs/fabric-crucible-followups.md` happen in this task? **A:** Yes, including resolving all **nine** inbound references and moving the roadmap entry to Done. The file's own lifecycle statement mandates it and nothing else will pick it up.
- **Q:** Is the fold a wholesale move of the design file? **A:** No — rationale only, matching slice 12's existing fold. The evidence table, the green-gates observation, and the harness-versus-chokepoint ordering argument stay out; the last already lives in `roadmap.md:20-23`.
- **Q:** Do the nine inbound references all repoint the same way? **A:** No. Four repoint cleanly, two need rewriting (`doc.go` has no build order and the "four Planned slices" phrasing is stale on landing), and the three roadmap "full task bodies live at …" sentences are deleted, since after the delete those bodies live nowhere and the Done entries already carry the summaries.
- **Q:** `UpdateJSON`'s signature — keep `ReadJSON`'s `found` flag? **A:** Yes, `mutate func(cur T, found bool) (T, error)`. The load-bearing reason is local: for a slice type, a missing file and an empty array are indistinguishable in `cur`. Keeping the un-migrated sites migratable is a secondary benefit, not the justification.
- **Q:** Can `UpdateJSON` be built from `ReadJSON` + `WriteJSON`? **A:** No — both acquire the lock internally, so that composition hangs rather than fails. It requires extracting lock-free cores, which changes those two functions' bodies (not their behaviour).
- **Q:** Does `record()` stay a method on `*corrIndex`? **A:** Yes. No call site or existing test changes; the receiver is still needed for `exact`/`nearestAtOrBefore`/`entries`.
- **Q:** New `CONSTRAINTS.md` invariant for the read-modify-write rule? **A:** No — with one consumer, "every locked-JSON RMW goes through `UpdateJSON`" would be false on landing. The rule goes in `internal/state`'s package header.
