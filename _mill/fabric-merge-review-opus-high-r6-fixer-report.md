# fabric merge surface — fixer report (round 6, tag `opus-high-r6`)

Companion to `_mill/fabric-merge-review-opus-high-r6.md`.
Every finding in that report is fixed. Nothing is deferred as unfixed, and nothing is marked NOT-FIXED-THIS-ROUND.

## Summary

| # | Severity | Finding | Commit |
|---|---|---|---|
| F1 | MEDIUM | adoption adopts an octopus merge fabric could never produce | `07197a8a` |
| F2 | MEDIUM | `MergeIn`'s under-lock **weft** start re-read had zero coverage | `f9aa18d1` |
| F3 | MEDIUM | the squash-refusal test passed for the wrong reason | `dd0a72ea` |
| F4 | LOW | the live-`MERGE_HEAD` adoption clause had zero coverage | `c35b3da9` |
| F5 | LOW | the abort guard's recorded-conclude-SHA clause had zero coverage | `8ff9e999` |
| F6 | MEDIUM | anchor/git separator mismatch breaks weft conflict mapping on Windows | `c3be0f4c` |
| F7 | LOW | `MergeStageResolved` had no foreign-merge-state refusal | `4a6901f1` |
| F8 | NIT | `Conflicts` was nil on every error return | `16fefa50` |
| F9 | NIT | sandbox suite's session-log template stopped at F13 | `958ffdfa` |

Counts: **3 MEDIUM, 3 LOW, 3 NIT.** Two carried a behavioural defect (F1, F6); one a behavioural inconsistency plus a false doc premise (F7); three were proof-quality gaps proven by sabotage (F2–F5, four clauses across three findings); three were contract/doc accuracy (F7's doc half, F8, F9).

One commit per finding, on `fabric-merge-crucible-round4`. **Nothing pushed.**

## What changed, per finding

### F1 — `internal/fabricengine/mergelifecycle.go`, `doc.go`, `mergein_recovery_integration_test.go`

`sideConcludeAlreadyLanded`'s parent test went from `len(parents) < 2 || parents[0] != start` plus a membership scan over `parents[1:]`, to exact `len(parents) != 2 || parents[0] != start || parents[1] != sourceSHA`.

This corrects a rule set at round 4 and carried unchallenged through round 5 and two orchestrator verifications — round 4's own report states it as "`HEAD` has ≥2 parents; `parents[0] == start`; and some `parents[1:]` equals the recorded source SHA", while `doc.go` simultaneously claimed "exactly two parents". The code followed the loose reading.

Verification:
- **Live, before the fix** (lab hub `h2`): a 3-parent `git merge <warp_source> <unrelated>` over the recorded start came back `committed: true`, record deleted, correspondence recorded, unrelated content on the branch.
- **Live, after the fix** (lab hubs `h8`, `h9`, rebuilt from scratch by `octopus.sh` against the redeployed binary): `merge conclude did not finish; run MergeContinue again`, record retained, HEAD untouched.
- New test `TestMergeContinue_OctopusMergeCarryingTheSource_IsNeverAdopted`, sabotage-proven: restoring the loose predicate turns it red.
- Every pre-existing adoption test (round 4's and round 5's) stays green, which is the check that the tightening rejects nothing fabric or its documented recovery route produces.

### F2 — `internal/fabricengine/mergelock_integration_test.go`

`TestMergeIn_StartsAreReReadUnderLock` now lands a concurrent commit on **both** sides during the lock wait, asserts with `t.Fatal` that each side genuinely moved, and checks `WarpStart` and `WeftStart` independently.

Sabotage-proven: deleting the weft re-read from `MergeIn` (sabotage S1, which left the whole suite green before this change) now fails this test.

### F3 — `internal/fabricengine/mergein_recovery_integration_test.go`

New `TestMergeContinue_SquashRecordCarryingATwoParentMerge_IsNeverAdopted`. It builds the only shape the `squash` clause can refuse — a squash record whose side carries a genuine two-parent merge of the recorded source on the recorded start — reached by `doc.go`'s own recovery route (`git reset --hard <start>` then a plain merge, since a squash leaves no `MERGE_HEAD` for `git merge --abort`). It asserts with `t.Fatal` that every *other* adoption clause is satisfied first, so nothing but `squash` can be what refuses.

Sabotage-proven: dropping `squash ||` (sabotage S7) now fails this test. No production change — the clause was already correct, only unproven.

### F4 — `internal/fabricengine/mergein_recovery_integration_test.go`

New `TestMergeContinue_SecondMergeStartedOverALandedConclude_LeavesNoLiveMergeHead`: hand-land this merge's conclude, then start a second clean `git merge --no-commit` before running `MergeContinue`.

The assertion is the discriminating one rather than the incidental one — a merge verb that returns without error leaves git-level merge state on neither side. Sabotage-proven: replacing the `MERGE_HEAD` arm with a no-op (sabotage S6) now fails it. No production change.

### F5 — `internal/fabricengine/mergecrucible_integration_test.go`

New `TestMergeCrucible_AbortRefusesOnTheRecordedConcludeSHAAlone`: after a half-concluded attempt whose warp conclude landed and was recorded, the operator resets that side back to the recorded start, so neither side's HEAD looks moved and only the record's own memory can refuse.

Sabotage-proven: deleting `if committed != "" { return true }` (sabotage S12) now fails it. No production change.

### F6 — `internal/fabricengine/mergepaths.go`, `mergepaths_test.go`

`weftPathVisible` now converts `anchorRel` with `filepath.ToSlash` before joining. `unifyConflictPaths`' godoc, which claimed `anchorRel` was already normalised "never filepath's OS-dependent one", now describes what the code does.

`filepath.ToSlash` specifically, not `strings.ReplaceAll`: it is identity when the OS separator is already `/`, so a Linux directory whose name legitimately contains a backslash keeps its name. Two table rows added — a multi-segment slash anchor (guards the mapping) and a single anchor segment containing a backslash (pins the conversion choice).

Both rows sabotage-proven: layering a blanket `strings.ReplaceAll` on top turns the table red, and ignoring the anchor when building the prefix turns it red.

**Honest scope:** the Windows half is still not executed — Linux host, as on all five prior rounds. What is different is that the defect is now traced to two exact lines (`mergepaths.go:43` against `lyxcwd/anchor.go:84`) with a stated mechanism, rather than carried as "Windows untested". The fix is provably inert on Linux by construction, which is both why it cannot regress anything here and why no Linux test can prove the Windows half. Live re-drive on the multi-segment anchor hub (`h7`) confirms the Linux behaviour is unchanged: `conflicts:["apps/backend/_lyx/base.txt"]`.

### F7 — `internal/fabricengine/mergestage.go`, `doc.go`, `mergestage_integration_test.go`

`MergeStageResolved` gains the record-then-foreign-probe arm `MergeIn` and `Merge` already use, refusing with `*ErrForeignMergeState`. Both godocs corrected: `doc.go`'s "every mutating merge verb refuses rather than touch it" is now true, and `MergeStageResolved`'s own no-lock argument no longer rests on the false premise that "no merge in progress" implies "nothing conflicted".

New `TestMergeStageResolved_ForeignMergeStateRefusesWithoutStaging`, sabotage-proven by removing the arm. `internal/mergeresolve` (the only shipped caller) is unaffected — it gates on `MergeInProgress()` first — and its suite is green.

### F8 — `internal/fabricengine/merge.go`, `mergelifecycle.go`, `mergecrucible_integration_test.go`

New `finalizeMergeResult(res, rec)` replaces the four verbs' `defer func() { res.Mutations = rec.Snapshot() }()`, stamping the mutation record and normalising `Conflicts` to the `mergeNoConflicts` sentinel in the one place that sees every return site.

Spelling the sentinel out per return site is exactly what left the property half-true: a prior round fixed the two success paths and pinned those two, while roughly a dozen error returns per verb still handed back a zero `MergeResult` marshalling as `"conflicts": null`. `TestMergeCrucible_ConflictsIsEmptyNeverNil` gains four error-return subtests, one per verb; sabotage-proven by removing the normalisation.

### F9 — `tools/sandbox/SANDBOX-FABRIC-SUITE.md`

Session-log template extended F13 → F20, so the three merge scenarios stop being dropped from every report. F20 gains the octopus arm, including the fixture detail that decides whether the scenario proves anything (root the decoy outside the merge's own history, or git discards the pre-merge tip as a redundant parent and you get two parents, not three) and the instruction to assert the parentage before drawing a conclusion.

The sandbox coverage guard (`TestSandboxCoverage_AllModulesCoveredOrExcluded`) and the markdown-link guard are green.

F7's scenario is deliberately **not** added to the suite: `MergeStageResolved` has no CLI verb, so it is unreachable from a black-box sandbox session. It is covered by the integration test instead.

## Gates

All run on the final tree.

- `go build ./...` — clean.
- `go vet ./...` (repo-wide, not just the three packages) — clean.
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` — all ok.
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` — all ok (31.1s / 2.8s / 1.7s).
- `go test -tags integration ./internal/mergeresolve/...` and `go test ./internal/mergeresolve/...` — ok (the only consumer of the verb F7 changed).
- `./deploy-dev` re-run after the last source change; every live scenario below re-driven against that binary.

## Live re-drive after the fixes (real hubs, no launcher)

Each hub built from scratch: `GIT_CONFIG_GLOBAL` with `init.defaultBranch = main`, bare warp + bare weft, seeded warp `main`, `lyx fabric clone`.

- **F1 (`h9`)** — octopus refused, record retained, HEAD untouched. The same script returned `committed: true` before the fix.
- **F6 (`h7`)** — multi-segment `--subpath apps/backend` hub: conflict maps to `apps/backend/_lyx/base.txt`, unchanged.
- **Full lifecycle (`r1`)** — guard refusal envelope; conflicted `merge-in` (two-side unified list, `partial:false`); SHA-labelled markers on both sides with no `-weft` leak; `merge --continue -m` concluding both sides with one `merge_committed` each; both checkouts clean with no `MERGE_HEAD`; a second `merge-in` reporting `already_up_to_date:true, committed:false`; `merge --abort` with nothing in progress reporting `no merge in progress`; and a fresh conflicted `merge-in` + `--abort` restoring both sides exactly and leaving both worktrees clean.
- **Non-ASCII (`r2`)** — `conflicts:["_lyx/ä-nöte.md","ä-warp.md"]`, raw bytes, never C-quoted. Round 5's F1 has not regressed.

## Prior-round cross-check (read only after this round's findings were written and committed)

No previously-fixed behaviour regressed; the full suites plus the live re-drives above cover round 4's R4-F1/F3/F5, round 5's F1/F2/F7, and the first instalment's R1 (empty-result merge) and R2 (conclude-landed guard).

Two of this round's findings sit directly on top of prior-round work, and the relationship is worth stating plainly:

- **F1 corrects a rule round 4 wrote and round 5 plus two orchestrator verifications accepted.** Round 4's report specifies the loose form verbatim (`≥2 parents`, source "somewhere among" the rest) while `doc.go` claimed "exactly two". Nobody compared the two statements.
- **F7 sits on round 5's F3.** Round 5 fixed `doc.go`'s enumeration to *name* `MergeStageResolved` as deliberately unguarded; this round found that the justification for it being unguarded had a false premise.

Both are the campaign's signature shape — a claim whose evidence does not discriminate — found by re-testing an already-verified fix rather than by reviewing new code.

## Deferred items, re-evaluated

- **Windows path behaviour** — still not executed. See F6 above for the honest statement: the mechanism is now traced and fixed, the Windows execution is not available on this host and no round has ever had it.
- **The four stuck-`MergeContinue` states** — re-confirmed unchanged. Round 5's lock-ordering change does not interact with them, and F1/F3/F4 tighten the adoption arm without narrowing what it recovers: every legitimate recovery route `doc.go` documents produces exactly two parents in exactly the recorded order.
- **The post-record error-return 45-row table** — not re-walked; not required this round. F8 touches those return sites but changes no error value and no site's disposition, only the `Conflicts` field's nil-ness.

## Teardown

Every scratch hub lives under this session's scratchpad, outside the repo. `git status` in the worktree is clean of everything but this round's own commits.
