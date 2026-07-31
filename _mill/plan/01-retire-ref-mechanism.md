# Batch: retire-ref-mechanism

```yaml
task: 'fabric: fold snapshot-tracking into the Warp-SHA trailer'
batch: retire-ref-mechanism
number: 1
cards: 5
verify: go test -tags integration -count=1 ./internal/gitrepo/... ./cmd/lyx/...
depends-on: []
```

## Batch Scope

This batch deletes the `refs/loomyard/snapshot/<key>` ref mechanism outright — `internal/gitrepo/snapshot.go` and every test, comment, guard entry, invariant clause, roadmap line, and review-prompt instruction that names it. It is one batch because the deletion is not separable: `internal/gitrepo`'s test package does not compile the moment `snapshot.go` is gone unless `gogit_test.go`'s linked-worktree parity harness is pruned in the same change, and `cmd/lyx/gitrepoboundary_test.go` asserts its pinned method set by **set equality**, so it fails loudly the moment the methods disappear. That loud failure is the intended forcing function, not an accident.

It is a root batch with no dependencies. Batch 2 (`commit-empty-primitive`) depends on it because both edit `cmd/lyx/gitrepoboundary_test.go`, `CONSTRAINTS.md`, and `internal/gitrepo/doc.go`, and because `CONSTRAINTS.md`'s "Known blind spot" bullet must pick its replacement worked example (`StageAndCommit`) from methods that exist at the time this batch lands — `CommitEmpty` does not exist yet.

**Batch-local decision — the deletion is one commit, not three.** Cards 1's deletion of `snapshot.go` and its pruning of the four test files land together because any split leaves an intermediate commit where `internal/gitrepo` does not build. Every later card in this batch is comment/doc-only and builds cleanly on its own.

**Nothing in this batch is a coverage regression.** The deleted production code has no callers, so the deleted tests witness nothing that survives. Two qualifications are recorded in Card 1's requirements rather than left to be rediscovered: `keyvalidation_test.go` is **kept** (its `TestValidSHA` covers the surviving `validSHA`), and the loss of `TestIsStrictDescendant_MixedBackend_RepackBetweenCommitAndRead` is acceptable because `parity_test.go`'s `TestSHAExists_MixedBackend_RepackBetweenCommitAndRead` pins the same fingerprint-gated-reindex gate on a surviving method.

## Cards

### Card 1: Delete snapshot.go and prune every test that references it

- **Context:**
  - `internal/gitrepo/snapshot.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/gogit.go`
- **Edits:**
  - `internal/gitrepo/gogit_test.go`
  - `internal/gitrepo/keyvalidation_test.go`
  - `internal/gitrepo/oracle_test.go`
  - `internal/gitrepo/parity_test.go`
- **Creates:** none
- **Deletes:**
  - `internal/gitrepo/snapshot.go`
  - `internal/gitrepo/snapshot_test.go`
- **Moves:** none
- **Requirements:** Delete `internal/gitrepo/snapshot.go` in full — `SnapshotSHA`, `SetSnapshotSHA`, `snapshotPushMaxAttempts`, `advanceAndPushSnapshotRef`, `adoptSnapshotRef`, `isStrictDescendant`, `remoteName`, `validSnapshotKey`, and `snapshotRef` all go with it. Delete `internal/gitrepo/snapshot_test.go` in full. Before deleting, confirm by grep that `remoteName` and `isStrictDescendant` have no caller outside `snapshot.go` and its tests, rather than assuming it. In `gogit_test.go` (`package gitrepo`, internal) remove `TestRemoteName_Parity`, `TestIsStrictDescendant_Parity`, `TestIsStrictDescendant_MixedBackend_RepackBetweenCommitAndRead`, the `freezePackIndex` helper used only by that last test, and this file's own copies of `oracleRemoteName`, `oracleIsStrictDescendant`, and `oracleSnapshotSHA`; remove from `runLinkedWorktreeParityChecks` the three subtests that call `remoteName()`, `SnapshotSHA(gogitParitySnapshotKey)`, and `isStrictDescendant(...)` inline, remove the `gogitParitySnapshotKey` constant together with its own doc comment, and remove the `git update-ref refs/loomyard/snapshot/...` seeding inside `newLinkedParityFixture`. This harness is the compile-breaking one: `TestLinkedWorktree_Parity` drives it twice (direct and via junction) and it calls all three doomed identifiers directly. In `oracle_test.go` (`package gitrepo_test`, external) remove `oracleSnapshotSHA`, `oracleRemoteName`, and `oracleIsStrictDescendant` — verified: after this card's `parity_test.go` removals none of the three has a caller, and `oracleRemoteName`/`oracleIsStrictDescendant` already have none today. In `parity_test.go` (`package gitrepo_test`, external) remove `newSnapshotRefFixture`, `writeSnapshotRef`, `TestSnapshotSHA_Parity_SetRef`, `TestSnapshotSHA_Parity_AbsentRef`, `TestSnapshotSHA_Parity_InvalidKey`, `TestSnapshotSHA_Parity_UnreadableStore`, `TestSnapshotSHA_MixedBackend_ReadsRefCLISideAdvanceWrote`, and `TestSetSnapshotSHA_MixedBackend_RepackBetweenCommitAndCanonicalization`; **keep** `forcePackIndexFreeze` and its **two** surviving call sites, and **keep** `TestSHAExists_MixedBackend_RepackBetweenCommitAndRead`, which pins the same fingerprint-gated-reindex gate on a surviving method and is why losing the `isStrictDescendant` variant is not a coverage hole. In `keyvalidation_test.go` remove `TestValidSnapshotKey` but **keep the file and keep `TestValidSHA`** — `validSHA` survives and is used by `ResetHard`, `ChangedFilesSince`, and `CheckoutDetached`, so deleting the file wholesale would silently drop that table. Update `keyvalidation_test.go`'s header comment, which currently says the file covers `validSnapshotKey` and `validSHA`. Also sweep the stale fixture and rationale comments this cut leaves behind: `parity_test.go`'s doc comment on the **surviving** `TestStageAndCommit_MixedBackend_PreWarmedHandleSeesCLICommit`, which motivates the test via a wrong SHA reaching a caller's `SetSnapshotSHA` (rephrase against a surviving consumer, keep the test); `gogit_test.go`'s `linkedParityFixture` type doc, which names `remoteName`, `hasUnpushed`, and `isStrictDescendant` (only `hasUnpushed` survives); its `sharedSHA` field comment, which describes the field as the value `refs/loomyard/snapshot/<key>` is set to from the main worktree (the field survives, that clause does not); the doc comment above `runLinkedWorktreeParityChecks` enumerating the checks it runs; and the **surviving** `gogitLinkedFixture` (which compiles fine after the cut because it uses raw git and a generic `handle.Reference()` call) — rename its `commonSnapshotRef` constant and rewrite its comments so a retired namespace is not presented as live. The ref name is arbitrary to what that fixture proves: any ref living in the common dir demonstrates the shared-common-dir read.
- **Commit:** `fabric: delete gitrepo's refs/loomyard/snapshot mechanism and its tests`

### Card 2: Sweep snapshot references out of internal/gitrepo's production comments

- **Context:**
  - `internal/gitrepo/pull.go`
  - `internal/gitrepo/reset.go`
  - `internal/gitrepo/worktree.go`
- **Edits:**
  - `internal/gitrepo/doc.go`
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/gogit.go`
  - `internal/gitrepo/push.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Drive this card from `grep -rin snapshot internal/gitrepo/`, then from separate greps for `remoteName` and `isStrictDescendant` — not from the list below, which is a starting point plus worked examples of the cases that are easy to miss. Resolve every hit: delete it, or rewrite it against a surviving method. Nothing may be left naming `SnapshotSHA`, `SetSnapshotSHA`, `refs/loomyard/snapshot/`, `validSnapshotKey`, `remoteName`, or `isStrictDescendant` as live. In `doc.go` delete the whole `# Snapshot remote model` section and the whole `# The self-correcting snapshot pattern` section including its own heading line; also fix the package summary line that lists snapshot tracking as a capability, the operations-covered paragraph that names snapshot/correspondence tracking, and the `PlainOpen` worked example that says the failure mode reports every existing `refs/loomyard/snapshot/*` key as absent — that example needs a surviving-method restatement, not deletion, because it is illustrating go-git's open behaviour, not the snapshot API. In `gitrepo.go` fix `ErrInvalidSHA`'s doc, which today names only `ChangedFilesSince` and `SetSnapshotSHA`. Do not merely drop the dead name: the comment is **already** undercounting by two before this task touches it, because `CheckoutDetached` (`gitrepo.go`) and `ResetHard` (`reset.go`) both return `ErrInvalidSHA` from the same `validSHA` gate and neither is listed. Since this card is rewriting the comment anyway, enumerate all three surviving returners — `ChangedFilesSince`, `CheckoutDetached`, `ResetHard` — rather than leaving a doc that is accurate about the deletion and wrong about everything else. Then fix `ChangedFilesSince`'s own godoc, which motivates its untracked/staged-blindness by "matching the snapshot model's SHA-to-SHA determinism". That second one is the reason the grep is mandatory rather than optional: `ChangedFilesSince` is otherwise untouched by this entire task, so the "re-read the godoc of every function you touch" obligation never reaches it, yet its comment dangles the moment `doc.go`'s snapshot section is deleted. In `gogit.go` fix the locking-discipline paragraph's plain-ref-read bullet (which names `remoteName` and "both snapshot ref reads" — drop both, keep `CurrentSHA` and `CurrentBranch`), its object-lookup bullet (which names `isStrictDescendant` and `SetSnapshotSHA`'s `^{commit}` canonicalization — drop both, keep `SHAExists`, `ChangedFilesSince`, and `hasUnpushed`, which all survive), the accessor's own "called from every migrated read" enumeration, the `lookupObjectRetrying` caller enumeration, and the `refs/loomyard/snapshot/*` mention inside the `# Why EnableDotGitCommonDir, and why not PlainOpen or DetectDotGit` section. That last one is the **same** worked example as the `doc.go` `PlainOpen` case above — both describe a handle that reports every existing snapshot ref as absent — so give both the same treatment: restate the example against a surviving ref, because what it illustrates is go-git's open behaviour, not the snapshot API. In `push.go` fix the comment that motivates a re-read by "SetSnapshotSHA would push an off-history snapshot". `pull.go`, `reset.go`, and `worktree.go` are in `Context:` only so the sweep can confirm by reading that they hold no hit — measured today they hold none; if the grep disagrees, that is a plan defect worth reporting rather than silently editing an unlisted file.
- **Commit:** `fabric: sweep snapshot references out of gitrepo's package docs`

### Card 3: Update the gitrepo Client Boundary guard

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/pull.go`
  - `internal/gitrepo/reset.go`
- **Edits:**
  - `cmd/lyx/gitrepoboundary_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Remove `"SnapshotSHA"`, `"advanceAndPushSnapshotRef"`, and `"adoptSnapshotRef"` from the `gitrepoPinnedRunBoundMethods` map — the guard asserts this set by equality, so leaving any of the three in place fails `TestGitrepoBoundary_PinnedRunCallSites` with "pinned but no longer r.run-bound". Rewrite **two** comments in this file, not one: the file header's `# The one blind spot this guard cannot see` section, which uses `SnapshotSHA` as its worked example of a method that legitimately hosts a migrated go-git read and a CLI-bound call side by side; and the explanatory comment above the map, whose second bullet names `SetSnapshotSHA` alongside `Push` and `PushCoalesced` as CLI-bound-by-contract methods absent from the list, and whose first bullet is entirely about `SnapshotSHA`. Use `StageAndCommit` as the replacement worked example in both: it is a surviving mixed method — three `r.run` calls (add, `diff --cached`, commit) followed by a migrated go-git `CurrentSHA` read — so the blind spot it illustrates is unchanged in kind. Correct `gitrepoBoundaryMinScannedFiles`'s doc comment, which enumerates the package's non-test files: it currently names seven (`doc.go`, `gitrepo.go`, `gogit.go`, `pull.go`, `push.go`, `reset.go`, `snapshot.go`) but omits `worktree.go`, so the enumeration is already stale by one file today. After `snapshot.go` is deleted the count is still seven, but the correct membership is `doc.go`, `gitrepo.go`, `gogit.go`, `pull.go`, `push.go`, `reset.go`, `worktree.go`. The floor value of 5 stays as-is.
- **Commit:** `fabric: drop retired snapshot methods from the gitrepo boundary guard`

### Card 4: Update CONSTRAINTS.md's gitrepo Client Boundary Invariant

- **Context:**
  - `cmd/lyx/gitrepoboundary_test.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `## gitrepo Client Boundary Invariant` section's **Statement** bullet, remove `SetSnapshotSHA`'s push and `SnapshotSHA`'s fetch from the exhaustively-named CLI-bound set, leaving `StageAndCommit`, `StageAllAndCommit`, `Push`, `PushCoalesced`, `PushRebaseFree`, `Pull`, `ResetHard`, `CheckoutDetached`, `RestoreBranch`, and `hasUnpushed`. Rewrite the **Known blind spot** bullet, whose only worked example is `SnapshotSHA` legitimately mixing a migrated read with a CLI-bound fetch; replace it with `StageAndCommit`, matching Card 3's replacement in the guard file so the two read consistently. Do not add `CommitEmpty` here — it does not exist yet and is Batch 2's job. Add no new invariant: this task introduces none.
- **Commit:** `fabric: retire snapshot methods from the gitrepo boundary invariant`

### Card 5: Correct the live instructions that still name the retired API

- **Context:**
  - `internal/gitrepo/doc.go`
  - `cmd/lyx/gitrepoboundary_test.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `manifest/roadmap.md`
  - `crucible/gitrepo-review-prompt.md`
  - `crucible/fabric-review-prompt.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `manifest/roadmap.md`, the `## Done` section's `**gitrepo**` entry is a present-tense module inventory naming `StageAndCommit`, `Push`, `PushCoalesced`, `CurrentSHA`, `ChangedFilesSince`, `SHAExists`, and `SnapshotSHA`/`SetSnapshotSHA`; drop the two snapshot names and leave the rest. This is **not** a roadmap status movement — CLAUDE.md's "roadmap moves only on completing or adding a planned item" rule governs status, which is a different question from correcting a stale API enumeration inside an existing entry. Two other lines in the same file name the ref API as historical descriptions of what the `git-native-library` spike and the `native clients` migration covered at the time; those are accurate history and **stay written as they are**. In `crucible/gitrepo-review-prompt.md`, four separate places carry **live, forward-looking instructions** to a future reviewer or fixer and all four break on this commit. The "What to read" list names `snapshot.go` among the source files a re-run of that module review must read — remove it. The "High-yield focus" section then instructs a future reviewer to actively drive-test the deleted mechanism in three bullets: an argument/flag-injection bullet that names `SetSnapshotSHA` alongside the surviving `SHAExists` and `ChangedFilesSince` and uses `update-ref <ref> <sha>` as one of its three worked interpolation examples (edit surgically — drop the `SetSnapshotSHA` clause and that example, keep the bullet and its two surviving methods); a bullet devoted entirely to `SetSnapshotSHA`'s adopt-on-conflict race with real concurrent writers (delete it, the machinery is gone); and a bullet devoted entirely to `remoteName()`'s "origin" fallback under a multi-remote repo (delete it, same reason). The round-framing paragraph after those bullets singles out "the snapshot-ref concurrency/adopt-on-conflict machinery" as one of two areas deserving a particularly skeptical look — rewrite so the remaining named area stands alone. Finally, the "Fixing" section tells a future fixer to match the file-level doc-comment convention "already used across `gitrepo.go`/`push.go`/`snapshot.go`" — drop the third name. In `crucible/fabric-review-prompt.md`, the "What to read" list describes `internal/gitrepo/**` as fabric's git operator and enumerates `Repo`, `StageAndCommit`, `Push`, `CurrentSHA`, `ChangedFilesSince`, `SHAExists`, `SnapshotSHA`/`SetSnapshotSHA`; drop the two snapshot names. In **both** crucible files the findings history further down (F1, F3, F4, R1, F-R3-2, F-R3-5 and their equivalents, all referencing the retired methods) is a **frozen round record and stays stale by design** — it documents what past rounds found, not what the code is. The distinction to apply is instruction-versus-record, not keyword presence: anything telling a future reviewer or fixer what to *do* is live and must be corrected; anything reporting what a past round *found* is frozen and must be left alone. Finally, as this batch's closing gate, re-run `grep -rin snapshot internal/gitrepo/ cmd/lyx/` plus separate greps for `remoteName` and `isStrictDescendant`, and confirm every surviving hit reads correctly against surviving API. Run the same three greps a second time against the crucible directory — it sits outside the mandated internal sweep, which is exactly how the live "High-yield focus" instructions above were nearly missed — and triage each hit as instruction or record. A residual hit naming a deleted identifier should be impossible — the greps were run when this plan was written and every file carrying one is in Card 1's or Card 2's `Edits:` — so if one turns up in a file outside this batch's edited set, stop and report it as a plan defect rather than editing an unlisted file.
- **Commit:** `fabric: correct roadmap and crucible prompts for the retired snapshot API`

## Batch Tests

`verify: go test -tags integration -count=1 ./internal/gitrepo/... ./cmd/lyx/...` — scoped to exactly the two package trees this batch touches. The `-tags integration` build runs the untagged Tier-1 tests too (untagged files compile under every tag set), so this single command covers both tiers for these packages; there is no separate untagged invocation to add. `./cmd/lyx/...` is not optional scope creep: `TestGitrepoBoundary_PinnedRunCallSites` is the set-equality guard this batch deliberately provokes, and `TestTierPurity_UntaggedTestsSpawnNothing` and `TestHermeticGitEnv_GitSpawningPackagesHaveTestMain` both walk the whole module and can fire on a test-file edit made in `internal/gitrepo`. Measured runtime on the untouched tree: about 116s for `internal/gitrepo` and 27s for `cmd/lyx`.

No new test is written in this batch. The verification that matters is negative — the package still compiles with `snapshot.go` gone, the surviving parity and linked-worktree coverage still passes, and the boundary guard's set equality holds against the shortened pinned list. `internal/gitrepo`'s existing `TestMain` already calls `lyxtest.HermeticGitEnv()`, so the Hermetic Git Test Environment Invariant needs nothing from this batch.
