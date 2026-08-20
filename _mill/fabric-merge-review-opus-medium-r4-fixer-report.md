# fabric merge surface — fixer report, round 4 (`opus-medium-r4`)

Companion to `_mill/fabric-merge-review-opus-medium-r4.md`.
Every finding recorded in that review is fixed. Nothing is deferred to a follow-up task.

## Disposition

| Finding | Severity | Fixed | Commit | Proof |
| --- | --- | --- | --- | --- |
| R4-F1 — `sideConcludeAlreadyLanded` adopts any post-start commit | BLOCKING | yes | `57881530` | sabotage + live, both directions incl. squash |
| R4-F2 — closed-set AST test parses one file | MEDIUM | yes | `27e99f6d` | sabotage (old test green, new test red) |
| R4-F3 — `Merge`'s pre-merge sync mutates outside the write lock | MEDIUM | yes | `b8bea5e4` | sabotage (pre-fix lock order fails the new test) |
| R4-F4 — `MergeStageResolved`'s unlocked-ness undocumented | LOW | yes | `bc4d1cdd` | doc-only; reasoning stated in godoc + doc.go |
| R4-F5 — `MergeContinue` hardcodes `AlreadyUpToDate` | LOW | yes | `1c849a71` | sabotage (re-hardcoding fails the new test) |
| R4-F6 — "guard stage is strictly read-only" is false | NIT | yes | `bc4d1cdd` | doc-only |
| R4-F7 — `doc.go`'s adoption paragraph documents the unsound predicate | NIT | yes | `57881530` | folded into F1, same commit as the behavior change |

Plus `1c0fa5d0` — `tools/sandbox/SANDBOX-FABRIC-SUITE.md` F20 gains the two adversarial adoption scenarios R4-F1 created.

## R4-F1 in detail

**Change.** Adoption of an already-landed conclude now rests on git's own parentage instead of HEAD movement.

- `mergeState` gains `warp_source`/`weft_source` — the resolved per-side SHA the attempt actually merges, written by `MergeIn`/`Merge` at the same `saveMergeState` that records `Verb`/`Source`/`*Start`.
  `mergeState.Source` (the branch NAME) is not usable evidence: it can be re-pointed between the crash and the resume.
- `gitrepo.CommitParents` is the new go-git read the evidence rests on (`internal/gitrepo/merge.go`). It stays off the Client Boundary Invariant's pinned CLI list, like its `HeadDetached`/`ResolveSHA` neighbours.
- `sideConcludeAlreadyLanded(repo, start, sourceSHA, squash)` adopts only when ALL of: HEAD moved off `start`; no live `MERGE_HEAD`; the record is not a squash; the recorded source SHA is non-empty; HEAD has ≥2 parents; `parents[0] == start`; some `parents[1:]` equals the recorded source SHA.
- A squash record is never adopted, and neither is a record with an empty source SHA (one written by an older binary). Both stay honestly stuck — `*ErrMergeIncomplete`, record retained — which is the pre-adoption-arm behaviour.

**Sabotage proof.** Reverting the arm to `return head, true, nil` (the ambiguous read) makes both new integration tests fail at their intended assertions, quoting `committed true, error <nil>` where they demand `*ErrMergeIncomplete`. Restoring gives an empty diff and both tests green.

**Live proof, freshly deployed binary, fresh hubs under the session scratchpad.**

- Adversarial (hub `h2`): conflicted `merge-in feat`, then `git merge --abort` + one unrelated commit, then `lyx fabric merge --continue` →
  `{"error":"fabricengine: merge conclude did not finish; run MergeContinue again","mutations":[],"ok":false}` with the record RETAINED and `feat` still un-merged.
  Before the fix the identical sequence returned `{"already_up_to_date":false,"committed":true,"mutations":[{"kind":"merge_committed","target":"warp","detail":"0f68d66…"}],"ok":true}` and deleted the record.
  `merge --abort` refuses too (`merge conclude already landed`) — honestly stuck, both verbs consistent.
- Legitimate (hub `h3`): conflicted `merge-in feat`, resolve, `git commit --no-edit` by hand while `MERGE_HEAD` is live, then `merge --continue` →
  `{"already_up_to_date":false,"committed":true,...}`, HEAD unchanged (no second commit), the operator's resolution intact on disk, record cleared, and the sibling verb (`lyx fabric commit`) released and succeeding immediately after.
- Squash (hub `h4`) — **driven live for the first time in this campaign**: `lyx fabric merge feat --squash` with a failing `pre-commit` hook on warp leaves `{squash: true, warp_outcome: staged, warp_committed: ""}`; the operator hand-lands the squash conclude with plain `git commit -m …` (verified one-parent, and that single parent IS `warp_start` — structurally indistinguishable from any operator commit, which is precisely why it must not be adopted); `merge --continue` then refuses with `merge conclude did not finish`, leaves the record on disk and HEAD untouched.

**Asymmetry audit (the round's high-yield lesson), completed.** Every reader of the "HEAD moved, no live `MERGE_HEAD`" signal was classified by which way it resolves the ambiguity:

- `sideConcludeMayHaveLanded` (`mergeguards.go`) — refusal-shaped. Correct, unchanged.
- `foreignMergeStatePresent`, `mergeAttemptIncompleteReason` — refusal-shaped. Correct, unchanged.
- `gitrepo.MergeStart`'s `headAfter != headBefore` — a positive claim, but inside one call that captured `headBefore` itself moments earlier, with no crash window. Not the same shape. Unchanged.
- `mergeState.bothSidesAlreadyUpToDate` / `landedConcludeCommit` — positive claims read off recorded fields written by the code that observed them, not off an ambiguous re-read. Unchanged.

`sideConcludeAlreadyLanded` was the only claim-shaped reader, and it is now evidence-backed.

## Tests added or strengthened

- `internal/fabricengine/mergein_recovery_integration_test.go`
  - `TestMergeContinue_UnrelatedCommitWhileRecordLive_IsNeverAdopted` (R4-F1, adversarial direction)
  - `TestMergeContinue_SquashConcludeLandedByHand_IsNeverAdopted` (R4-F1, squash shape — the prior campaign's carried-forward "reasoned about, never driven" item)
  - `TestMergeContinue_BothSidesAlreadyUpToDate_DerivesAlreadyUpToDate` (R4-F5)
  - helpers `resolveSHAForTest`, `isAncestorForTest`, `abortMergeAndLandUnrelatedCommit`
- `internal/fabricengine/merge_target_integration_test.go`
  - `TestMerge_PreMergeSyncRunsInsideTheWriteLock` (R4-F3) — behavioural, not structural: with the weft write lock externally held, `Merge` must not have advanced either checkout, and must complete once released.
- `internal/fabricengine/mergevocab_test.go`
  - `mergeReasonConstsFromSource` now scans every non-test `.go` file in the package and records each member's declaring file; duplicate declarations `t.Fatal`.
  - `TestMergeVocabulary_GuardReasonSetIsDeclaredInOneFile` (R4-F2) — new.
- `internal/fabricengine/mergestate_integration_test.go`
  - `assertNoZeroFields` — the roundtrip test named `PreservesEveryField` could previously pass with a newly added field roundtripping zero-to-zero. It now fails if `want` leaves any field at its zero value, which is what makes the name true as the record grows.

Every scenario asserts its own precondition with `t.Fatal` before the assertion that matters (record still live, conclude SHAs still empty, HEAD really moved, `Squash` really recorded, fixture really stale) — the silent-degradation guard the round prompt requires.

## Docs updated in the same change as the behavior

- `internal/fabricengine/doc.go`, "# The merge surface": the adoption paragraph rewritten around the claim-vs-refusal asymmetry and the parentage evidence; the squash non-adoption stated; the plain-git last-resort instructions corrected (a side whose `MERGE_HEAD` is gone must be `reset --hard` to its recorded start and re-merged against the recorded source, because adoption checks the first parent too; a squash has no hand-landed route at all); the write-lock scope now names `Merge`'s pre-merge sync; the result-flag paragraph now covers `MergeContinue`'s own return.
- `internal/fabricengine/mergeerrors.go`, `mergelifecycle.go`, `mergeguards.go`, `mergestate.go`, `mergestage.go`, `merge.go` godoc updated alongside each fix.
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md` F20 extended; the coverage guard stays green.
- `CONSTRAINTS.md` — **not** touched. No cross-cutting invariant moved: every change is inside the merge surface's own contract, which `doc.go` owns.
- `manifest/roadmap.md` — **not** touched, per the round prompt and CLAUDE.md.

## Verification, final state (commit `1c0fa5d0`)

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — all ok.
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — fabricengine 47.3 s, fabriccli 4.3 s, gitrepo 2.5 s, all ok.
- `./deploy-dev` re-run after every source change; all live scenarios re-driven against the deployed binary.

## Deferred — with reasons

Nothing is deferred as unfixed work. Two items are carried-forward COVERAGE gaps rather than findings:

- **Windows path behaviour in `weftPathVisible`/`unifyConflictPaths`.** NOT executed, NOT simulated. This round ran entirely on Linux, as every prior round did. I am stating this plainly rather than letting silence imply coverage. It stays a genuine environment gap and needs a Windows host, not another round on this one.
- **Round 2's 45-row per-site adjudication of the post-record error-return class.** Not re-walked row by row; not required this round. R4-F3 moves `Merge`'s lock acquisition earlier but adds no new post-record error return, and R4-F1 adds none either (its new refusal path returns through the existing `MergeConclude` failure arm).

Re-evaluated and unchanged: the **four stuck states** where the conclude lands but `CurrentSHA`/`saveMergeState` fails. R4-F1 does not change that calculus. Those states are ones where the conclude genuinely landed as a real two-parent merge of the recorded source, so the strengthened arm still adopts them — what it removes is adoption of commits that are not that. If anything, the recovery for those rows now rests on evidence rather than on an ambiguous read.

## Teardown

All scratch hubs (`h1`–`h4`) live under the session scratchpad, outside the repo.
`git status` in the worktree is clean of everything but this round's own commits.

## Round self-assessment

The BLOCKING finding was reproduced live before the fix and re-driven live after, in both directions plus the squash shape.
Both MEDIUM findings are sabotage-proven — for R4-F2 the proof includes showing the OLD test stayed green under the same sabotage, which is the point of the finding.
The residual risk I would hand to round 5: the `parents[0] != start` clause makes adoption strict enough that a legitimate recovery where the operator did anything else on that branch first is refused. That is the safe direction and it is documented, but it is a judgement call about operator ergonomics that a fresh reviewer should second-guess rather than inherit.
