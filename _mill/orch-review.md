# Orchestrator review — `fabric-corrindex-record-race` discussion

Reviewed against the code on branch `fabric-corrindex-record-race` at `b516ecc3`.
Every line-number and call-graph claim in the discussion was checked against the tree, not taken on trust.

**Verdict: four blocking, seven non-blocking.**
The fix shape is right and the exploration behind it is genuinely good — the "the brief's preferred shape is not implementable as written" finding is correct, load-bearing, and the kind of thing a weaker discussion would have papered over.
What is wrong is narrower and sharper: the discussion never notices that its own call-graph section disproves the race its test is designed to reproduce.

## What checks out

Verified correct, so the plan can build on these without re-deriving them:

- **The self-deadlock claim.** `state.WriteJSON` (`internal/state/state.go:34`) and `ReadJSON` (`:58`) each acquire and release internally, and `lock.AcquireWriteLock` (`internal/lock/lock.go:21-22`) calls `flock.New(lockPath)` fresh per call.
  Two open file descriptions on one file mutually exclude under Linux `flock(2)` and Windows `LockFileEx` alike, in-process included.
  So `corrindex.go` cannot wrap `WriteJSON` in its own acquire. Correct, and correctly load-bearing.
- **The weft-write-lock inventory.** Non-test acquisitions in `internal/fabricengine` are exactly `commit.go:182`, `pull.go:351`, `weftgit.go:259`, with `coalesce.go:24` and `gitexclude.go:74` on unrelated lock paths. As stated.
- **The ordering claim in the rejected alternative.** `pull.go:303` (`RebuildIndex`) does precede `pull.go:351` (lock). As stated.
- **`RebuildIndex` never reads the index file.** Confirmed: `corrIndexPath` → git scan → `state.WriteJSON(path, path+".lock", entries)` at `index.go:416`. Its base is the trailer history, not the file.
- **"No existing test changes."** Verified rather than assumed: every `record()` call site in the suite (`corrindex_test.go:23,26,68,71,94,124,159,162,189`, `revert_test.go:25`) obtains its handle through `loadCorrIndex(path)` first, and none constructs a `corrIndex{recs: ...}` literal.
  Re-basing the upsert on the on-disk file is therefore invisible to all of them, including the multi-`record()` loops at `:124` and `:189`. The claim holds.
- **The docslink checker tolerates a `.go` target.** `internal/lyxcwd/docslink_test.go:333` existence-checks any non-`.md` target and skips the anchor check, and `:210` pins `../../internal/fabricengine/doc.go` as a first-class case. The repoint will not trip the Markdown Link Integrity Invariant.
- **`docs/shared-libs/README.md:37`** ("generic locked typed JSON I/O") does stay accurate with `UpdateJSON` added. Agreed, no edit needed.

---

## BLOCKING

### 1. The reproducing test targets a race the production call graph already excludes

This is the finding that matters, and the discussion assembles every fact needed to see it without drawing the conclusion.

Technical context line 154 records that `RecordCorrespondence`'s only two production callers are `commitEmptySnapshot` (`weftgit.go:180`) and `commitWeftLocked` (`weftgit.go:240`), "both of which run under the weft write lock."
I verified this and it is stronger than stated: `commitWeft` (`weftgit.go:255-262`) takes the lock and delegates; `commitBothSides` (`commit.go:182`) takes it before any commit; and `pull.go`'s re-anchor calls `commitEmptySnapshot` at `:358`, under the lock taken at `:351`.
There is no third caller.
The weft write lock and the correspondence index are both per-pair, so **two `record()` calls against one index file cannot overlap in production.**

The proposed Tier-1 test is N goroutines each loading a handle and calling `record()` — i.e. exactly `record()`-versus-`record()`.
It will fail before the fix and pass after, and it will prove nothing about production, because the weft write lock already forbids that interleaving.

The race that *is* real is `record()` versus `RebuildIndex`, and the discussion says so in its own Problem section ("a `commit` racing a `diff`/`revert`").
`RebuildIndex`'s four callers — `pull.go:303`, `revert.go:65`, `index.go:156`, `refreshCorrIndexAfterSwitch` via `checkout.go:125` — hold no weft lock, which is precisely why they can interleave with a locked `commit`.

The fix is right; the test is aimed at the wrong axis.
And it is not a hard test to aim correctly — it does not even need goroutines:

```go
ix, _ := loadCorrIndex(path)                                  // base: empty
state.WriteJSON(path, path+".lock", []corrEntry{other})       // stands in for RebuildIndex's write
ix.record(mine)                                               // today: clobbers `other`
// reload must observe BOTH entries
```

Deterministic, git-free, untagged, no barrier to get wrong, no goroutine-count tuning — and it reproduces the interleaving that can actually occur.
It sabotage-proves cleanly too: revert `record()` to composing from `ix.recs` and it fails on demand.

Consequence for the Q&A log: "a Tier-1 goroutine test … is the only way the fix is provable at all" is false.
The sequential form is both provable and stricter.

Keep a goroutine test as well if wanted — as cover for `state.UpdateJSON`'s own concurrency property, where it belongs and where the discussion already scopes it (Testing, bullet 4 of the `internal/state` list).

### 2. `UpdateJSON` cannot be composed from `ReadJSON` + `WriteJSON`, and the spec does not say so

The discussion documents the self-deadlock mechanism precisely (Technical context, "Why the brief's … cannot be done in place") and then specifies `UpdateJSON` as "acquires one exclusive lock on `lockPath`, reads `path` …, writes the returned value atomically" without noting that the obvious implementation — acquire, then call `ReadJSON`/`WriteJSON` — self-deadlocks by that same mechanism.

Both existing functions acquire on `lockPath` internally.
An implementer who writes the composition gets a **hang, not a failure** — the worst possible failure mode to hit inside a test that also holds the lock.

`Files to change` compounds this: it lists `internal/state/state.go` as "add `UpdateJSON`; extend the package header comment."
The real change is a refactor — extract lock-free cores (`readJSONUnlocked` / `writeJSONUnlocked`, or equivalent) that `ReadJSON`, `WriteJSON` and `UpdateJSON` all sit on top of.
That is still small and still additive in behaviour, but it touches the two existing exported functions' bodies, which the discussion currently implies it does not.

State it explicitly, in the decision and in `Files to change`.

### 3. The residual window is unnamed, and the task claims to close a race it half-closes

`UpdateJSON` fixes one direction of the real race and leaves the other open:

| interleaving | today | after the fix |
|---|---|---|
| `RebuildIndex` writes, then `record()` writes from a pre-rebuild base | rebuild's entries lost | **fixed** — `record()` re-reads under the lock |
| `RebuildIndex` scans, `record()` writes, `RebuildIndex` writes | record's entry lost | **still lost** |

The second row survives because `RebuildIndex` is itself two-phase — scan git, then `state.WriteJSON` — and its scan is not under the index lock.
If the scan predates the weft commit whose trailer carries the new entry, the rebuild's write clobbers a `record()` that landed in between.
Same shape, same severity, same self-healing.

The `Out` section says only that "`RebuildIndex`'s locking is unchanged", which reads as "no work needed here", not "a structurally identical window stays open."
Decision `fix-shape-is-a-state-level-update-primitive` goes further and overclaims outright: `UpdateJSON` "serialises it against every other writer on that file — another `record()` and `RebuildIndex`'s own `state.WriteJSON` alike."
It serialises against the *write*; it does not serialise against the scan-to-write span, which is where the loss happens.

Fix the claim, add the residual to `Out` by name, and — since slice 15 is the entry that moves to Done — make sure the roadmap Done text says the `record()` side is closed rather than that the race is closed.
Leaving `RebuildIndex` alone is a fine call for a LOW self-healing defect. Silently implying it is fixed is not.

### 4. The fold-and-repoint is not the mechanical step the discussion presents

Two distinct problems.

**a. The fold list contradicts the house style it cites.**
Decision `fold-and-delete-the-design-file` says slice 12 "already folded its own share … fold alongside it, in the same voice."
Read what slice 12 actually folded (`doc.go:564-644`): eight sections of *rationale* — why a chokepoint, why the gate executes rather than approves, why ownership is a closed enum, why scope is caller-declared, why the probe lives outside `destroy.go`.
It contains no evidence table, no round numbers, no campaign process history. It compresses all eight defects into one sentence: "Eight data-loss defects across five review rounds were one shape, not eight mistakes."

`Durable content to fold into doc.go` proposes the opposite: the eight-defect evidence table by round and verb, the fact that the gates were green throughout, and "why the chokepoint led over the harness (the recorded wrong-then-corrected ordering argument)."
That last item is planning history and it is **already in `manifest/roadmap.md:20-23`**, verbatim in substance — "An earlier draft put the harness first … **that was wrong**".
Folding it into `doc.go` duplicates a live doc rather than rescuing an orphaned one.

Trim the fold to what a future `fabricengine` reader needs at the code: slice 15's own locking rationale, the residual window from finding 3, and whatever campaign framing is not already in `doc.go`.
Let git history and the roadmap keep the forensics. Right now `doc.go` is 644 lines and the source file is 488; an untrimmed fold roughly doubles a package doc that is already at the edge of readable.

**b. Three of the inbound references cannot be repointed, only rewritten or deleted.**
"Repoint all eight inbound references at the package doc" treats them as a uniform swap. They are not:

- `manifest/roadmap.md:33` — "Full task bodies live at …". After the delete, the task bodies live nowhere. `doc.go` carries rationale, not task bodies.
- `manifest/roadmap.md:208` and `:215` — same sentence on the slice 13 and 14 Done entries. Same problem.
- `manifest/designs/lyxtest-real-hubs.md:7` — "See … 's build order." `doc.go` has no build order.
- `manifest/designs/fabric-windows-verification.md:73` — "the four Planned slices from the same campaign". Stale on landing regardless of where the link points.

Only `lyxtest-real-hubs.md:20`, `fabric-windows-verification.md:34`, `fabric-unified-view.md:228` and `gitexec-error-shape.md:510` are genuine "see the rationale" links that repoint cleanly.

The honest treatment of the roadmap trio is to delete the pointer sentence — the Done entries already carry full as-built summaries, so nothing is lost.
Note this is *not* the alternative the decision rejected: rejecting "repoint the references at roadmap's Done entries" is a different question from "should a reference survive at all."
Make the repoint a per-reference table in the plan, with an explicit verb (repoint / rewrite / delete) for each.

---

## Non-blocking

1. **Nine inbound references, not eight.** Counted in the tree: `roadmap.md:33,208,215`, `lyxtest-real-hubs.md:7,20`, `fabric-windows-verification.md:34,73`, `fabric-unified-view.md:228`, `gitexec-error-shape.md:510`.
   The `Files to change` list enumerates all nine correctly; the prose says "eight" three times (scope, decision, Q&A). An implementer working from the count stops one short.

2. **The completion grep is mis-scoped.** "grep for `fabric-crucible-followups` across `.md` and `.go` after the repoint, expecting zero hits" will return six hits from `_mill/discussion.md`, which is committed on this branch (`b516ecc3`).
   Scope the check to exclude `_mill/`, or the criterion can never pass.

3. **"The other four … sites" is four files but five sites.** `internal/treadleengine/state.go` has two independent read-modify-write pairs (`110`→`117` and `151`→`172`).
   The `Out` list cites both read line numbers under one entry while giving read+write pairs for the other three, which reads as an inconsistency. Cosmetic, but this list is the map a later migration task will work from.

4. **If the goroutine test survives finding 1, it needs a barrier.** As described — each goroutine loads *and* records — the pre-fix failure is probabilistic, not certain: a goroutine that loads after another's write sees that entry and preserves it.
   Only a barrier between the load phase and the record phase makes the pre-fix failure deterministic. Relevant to the `state.UpdateJSON` unit test too, which has the same shape.

5. **Anchors do not work on a `.go` link target.** `docslink_test.go:333` skips the anchor check for non-`.md` targets, so `doc.go#some-section` would pass the checker while being a dead link on GitHub.
   The repointed references must name the section in prose, the way the followups file itself does today ("see doc.go's \"The destruction chokepoint\" section").

6. **The `found` parameter has a better justification than the one given.** The decision defends it as keeping the four un-migrated sites migratable — speculative generality for a flag the only consumer discards.
   The stronger argument is local: for `[]corrEntry`, a missing file and an empty array are indistinguishable in `cur`, so `found` carries information the callback cannot otherwise recover.
   Same conclusion, but it survives "we never migrate the other four."

7. **The fold is the largest deliverable by volume and is listed last.** Scope has one small primitive, one small rewrite, two test files — and a 488-line design doc to distil, nine references to resolve, and a roadmap move.
   Batch accordingly; the docs half is not a tail-end chore, and per `CLAUDE.md` it cannot be split into a follow-up commit either.

---

## Summary of required changes before planning

1. Replace the goroutine reproducer with the deterministic `record()`-versus-external-write test (blocking 1); keep goroutines for `state.UpdateJSON`'s own cover.
2. State in the decision and in `Files to change` that `UpdateJSON` requires lock-free cores extracted from `ReadJSON`/`WriteJSON` (blocking 2).
3. Name the `RebuildIndex` scan-then-write residual in `Out`, correct the "serialises against every other writer" claim, and word the roadmap Done entry to match (blocking 3).
4. Trim the `doc.go` fold to rationale, drop what `roadmap.md:20-23` already carries, and turn the repoint into a per-reference table with an explicit verb each (blocking 4).
5. Fix the count to nine, scope the completion grep past `_mill/` (non-blocking 1-2).
