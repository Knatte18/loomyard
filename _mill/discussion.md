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
- Rewrite `corrIndex.record` (`internal/fabricengine/corrindex.go:48`) to go through `state.UpdateJSON`, so its upsert applies to the freshly-read on-disk base rather than to a stale `ix.recs` snapshot.
- A Tier-1 concurrency test in `internal/fabricengine/corrindex_test.go` that reproduces the lost update before the fix and passes after.
- Unit tests for `state.UpdateJSON` in `internal/state`.
- Document the read-modify-write rule in `internal/state`'s package header.
- Fold the durable rationale of `manifest/designs/fabric-crucible-followups.md` into `internal/fabricengine/doc.go`, delete that design file, and repoint all eight inbound references at the package doc.
- Move `manifest/roadmap.md`'s slice-15 entry from Planned to Done.

**Out:**

- `RebuildIndex`'s locking is unchanged — it keeps writing under the file's own lock and does **not** acquire the weft write lock.
- `refreshCorrIndexAfterSwitch`'s unlocked `os.Remove` + rebuild (`internal/fabricengine/index.go:315-318`) is left as-is, deliberately (see Decisions).
- The other four locked-JSON read-modify-write sites are **not** migrated to `state.UpdateJSON`: `internal/treadleengine/state.go:110,151`, `internal/boardengine/store.go:55,76`, `internal/reedengine/state.go:57,71`, `internal/websterengine/state.go:216,236`.
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
  With `UpdateJSON`, `record()` holds the exclusive lock across read + upsert + atomic write, which serialises it against every other writer on that file — another `record()` and `RebuildIndex`'s own `state.WriteJSON` alike — with no cross-path reasoning required.
- Rejected: giving `RebuildIndex` and `refreshCorrIndexAfterSwitch` the weft write lock.
  It is deadlock-free today — exploration confirmed R6's ordering claim still holds in current code, with `pull.go:303` calling `RebuildIndex` before taking the weft write lock at `pull.go:351`, and the only weft-write-lock acquisitions in the package being `commit.go:182`, `pull.go:351` and `weftgit.go:259` (`coalesce.go:24` and `gitexclude.go:74` are unrelated locks), while `Diff`, `Revert` and `Checkout` — the three paths reaching `RebuildIndex`/`refreshCorrIndexAfterSwitch` — hold no weft lock.
  But that is a claim about *every* call path that every future caller must preserve, versus a local fact;
  and it would still leave `record()` two-phase against any writer that does not take the weft lock.
  Also rejected: doing both, which buys nothing once `record()` is single-phase and pays the cross-path claim's ongoing cost.

### updatejson-signature-mirrors-readjson

- Decision: `func UpdateJSON[T any](path, lockPath string, mutate func(cur T, found bool) (T, error)) error`.
  It acquires one exclusive lock on `lockPath`, reads `path` (missing file yields the zero `T` and `found=false`), calls `mutate`, writes the returned value atomically, and releases.
  A `mutate` error aborts with no write.
- Rationale: mirroring `ReadJSON`'s existing `(T, bool, error)` shape keeps the missing-vs-empty distinction that `ReadJSON` deliberately surfaces, so the four un-migrated read-modify-write sites stay migratable later without a signature change — `internal/treadleengine/state.go:110` branches on exactly that flag.
  `corrindex` discards the flag, as it already does at `corrindex.go:36`.
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
  do not migrate the other four read-modify-write sites in this task.
- Rationale: each of the four has its own concurrency story to establish, and this slice is explicitly scoped LOW.
  Widening it to five packages inverts the risk/payoff of a self-healing race fix.
- Rejected: migrating all five sites in one pass.

### no-new-constraints-invariant

- Decision: add no invariant to `CONSTRAINTS.md`.
  State the rule — a locked-JSON read-modify-write must hold one lock across read and write, which is what `UpdateJSON` is for — in `internal/state`'s package header comment instead.
- Rationale: because adoption stays at one consumer, an invariant of the form "every locked-JSON read-modify-write goes through `UpdateJSON`" would be false on the day it lands, and a false invariant is worse than none.
  The package header is where the next author writing a read-modify-write over `internal/state` actually meets the rule.
- Rejected: adding the invariant, which would force migrating the other four sites and reopen the adoption decision.

### fold-and-delete-the-design-file

- Decision: fold the durable rationale of `manifest/designs/fabric-crucible-followups.md` into `internal/fabricengine/doc.go`, delete the design file, repoint all eight inbound references at the package doc, and move `manifest/roadmap.md`'s slice-15 entry from Planned to Done — all in this task's commits.
- Rationale: the file's own line 5 states this lifecycle explicitly ("deleted once all four have landed, with their durable rationale folded into `internal/fabricengine`'s package doc"), and slice 15 is the fourth.
  It matches `docs/overview.md#documentation-lifecycle`, which makes module-design docs deletable on landing with rationale moving to the Go package header.
  The file is also already stale — its line 3 status header reads "slices 12-13 landed (2026-08-11); slices 14-15 not yet built" while slice 14 landed in `d56b57f7` — and nothing else will pick this up.
  Slice 12 already folded its own share of the rationale into `doc.go`'s "destruction chokepoint" section, so the fold target and its house style already exist.
- Rejected: repointing the inbound references at `manifest/roadmap.md`'s Done entries instead (roadmap carries per-slice as-built summaries, but it is a planning doc, not the durable home the lifecycle names);
  keeping the file and folding only slice 15 (contradicts its own stated lifecycle).

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

- `internal/state/state.go` — add `UpdateJSON`; extend the package header comment with the read-modify-write rule.
- `internal/state/state_test.go` (or a new sibling) — unit cover for `UpdateJSON`.
- `internal/fabricengine/corrindex.go` — rewrite `record`'s body; update its doc comment, which currently says "Persists before in-memory update so write failure leaves index unchanged" and must now also state that the upsert base is the on-disk file and that `ix.recs` converges on disk truth.
- `internal/fabricengine/corrindex_test.go` — add the concurrency test.
- `internal/fabricengine/doc.go` — fold in the durable campaign rationale.
- `manifest/designs/fabric-crucible-followups.md` — delete.
- `manifest/roadmap.md` — slice 15 Planned → Done;
  repoint links at lines 33, 208, 215.
- `manifest/designs/fabric-windows-verification.md` (lines 34, 73), `manifest/designs/gitexec-error-shape.md` (line 510), `manifest/designs/fabric-unified-view.md` (line 228), `manifest/designs/lyxtest-real-hubs.md` (lines 7, 20) — repoint inbound references.

**Call graph worth knowing.**
`record()` has exactly one production caller: `Fabric.RecordCorrespondence` (`index.go:108`), which loads the index then records.
`RecordCorrespondence` is called from `commitEmptySnapshot` (`weftgit.go:180`) and `commitWeftLocked` (`weftgit.go:240`), both of which run under the weft write lock.
`RebuildIndex` is called from `pull.go:303`, `revert.go:65`, `index.go:156` (`WeftSHAForWarpSHA`'s stale-hit self-correction), and `refreshCorrIndexAfterSwitch` (`index.go:318`), itself called from `checkout.go:125`.
None of those four hold the weft write lock at the point of call.

**Durable content to fold into `doc.go`.**
The eight-defect evidence table (round, verb, what it destroyed);
the "one shape, not eight mistakes" framing — a destructive operation acting on a path it does not own, or without checking whether there is uncommitted work to lose, found in six files across five rounds;
the fact that `go build`/`go vet`/`go test` were green before, during and after every one of them, and that all eight were found by driving real git against a real filesystem in hostile or dirty state;
why the chokepoint led over the harness (the recorded wrong-then-corrected ordering argument, kept because it is the kind of reasoning that sounds right and delays a safety fix);
and slice 15's own locking rationale, which this task's Decisions section supersedes and extends.
`doc.go` already carries slice 12's share in its "destruction chokepoint" section — fold alongside it, in the same voice.

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
- Concurrent `UpdateJSON` calls against one path from many goroutines, each appending a distinct element, all survive — this is the property `corrindex` is buying.

**`internal/fabricengine/corrindex_test.go` — the reproducing concurrency test (the TDD candidate that matters).**
This is the test that converts the brief's "reasoned about but never driven" into a driven one, and it is worth writing before the fix so its pre-fix failure is observed rather than assumed.
N goroutines each load their own `corrIndex` handle over one shared `t.TempDir()` path and call `record()` with a distinct `WarpSHA`;
after all complete, a fresh `loadCorrIndex` must observe every entry.
Distinct `flock` handles mutually exclude in-process (see Technical context), so this reliably loses updates today and passes after the fix.
Keep it untagged and git-free.

Consider the same shape as a sabotage proof in the manner of slice 13's table: the test should fail on demand if `record()` is reverted to composing from `ix.recs`.

**Existing cover that must keep passing unchanged.**
`corrindex_test.go`'s round-trip, upsert-by-`WarpSHA`, sort-order and `nearestAtOrBefore` tests;
`index_integration_test.go` and the `checkout_index_refresh_test.go` refresh cover;
`pull`, `diff` and `revert` integration suites that exercise `RebuildIndex` and the stale-hit self-correction path.
No behavioural change is intended for any of them.

**Doc changes.**
Verify no dangling links remain after the delete — grep for `fabric-crucible-followups` across `.md` and `.go` after the repoint, expecting zero hits.

## Q&A log

- **Q:** Which fix shape — a `state`-level update primitive, or the weft write lock on `RebuildIndex`/`refreshCorrIndexAfterSwitch`? **A:** The `state`-level primitive. The weft-lock version rests on a whole-package deadlock claim every future caller must preserve, and leaves `record()` two-phase against non-weft-locked writers.
- **Q:** Close `refreshCorrIndexAfterSwitch`'s unlocked delete-then-rebuild window too? **A:** No — leave it and document why. The discard is intended to drop entries, so a concurrent `record()` losing its entry there is designed behaviour, not this bug.
- **Q:** How far does `state.UpdateJSON` adoption go — corrindex only, or all five read-modify-write sites? **A:** corrindex only. The other four each have their own concurrency story, and this slice is scoped LOW.
- **Q:** Does a race never reproduced at runtime get a concurrency test? **A:** Yes — a Tier-1 goroutine test that fails today and passes after. It is the only way the fix is provable at all.
- **Q:** Slice 15 is the last of the four — does the fold-and-delete of `manifest/designs/fabric-crucible-followups.md` happen in this task? **A:** Yes, including repointing all eight inbound references and moving the roadmap entry to Done. The file's own lifecycle statement mandates it and nothing else will pick it up.
- **Q:** `UpdateJSON`'s signature — keep `ReadJSON`'s `found` flag? **A:** Yes, `mutate func(cur T, found bool) (T, error)`. corrindex discards it, but it keeps the four un-migrated sites migratable without a later signature change.
- **Q:** Does `record()` stay a method on `*corrIndex`? **A:** Yes. No call site or existing test changes; the receiver is still needed for `exact`/`nearestAtOrBefore`/`entries`.
- **Q:** New `CONSTRAINTS.md` invariant for the read-modify-write rule? **A:** No — with one consumer, "every locked-JSON RMW goes through `UpdateJSON`" would be false on landing. The rule goes in `internal/state`'s package header.
