# `fabric merge` — crucible round fable-medium-r3 review report

Round 3, scoped per the re-seeded round prompt: residuals A–C plus adversarial work in the regions they cross.
The CLOSED-AND-VERIFIED list (r1 F1–F8, r2 R1/R2/R3/R5, residual 1) is off-limits and was not re-litigated.

## Executive summary

(written last)

## Scope assessment

This round does not redo the broad plan-vs-shipped sweep; rounds 1 and 2 covered it and the orchestrator verified.
Scope here: residual A (closure test cannot detect an added member), residual B (four invisible-conclude states leave MergeContinue stuck), residual C (spot-check of round 2's 45-row adjudication).

## Findings

(appended as formed; severity ordering finalized at the end)

## What was tested

(appended as each command/scenario returns)

### Log — baseline gates (hermetic tag: none/default)

- `go build ./...` + `go vet` on fabricengine/fabriccli/gitrepo: OK.
- `go test -count=1 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` (hermetic, no tag): all ok.

### Log — residual A reproduction (independent, before reading any prior report)

Added a tenth constant `mergeReasonSabotageTenth = "sabotage tenth member"` to the closed set in `mergeerrors.go`,
ran `go test -count=1 ./internal/fabricengine/` (hermetic): **green** (`ok ... 0.096s`).
`TestMergeVocabulary_GuardReasonSetIsClosedAndSideFree` compares a `want` literal list against a `got` list of constant references, both hand-maintained in the test; nothing reads the const block, so an added member is invisible.
Restored `mergeerrors.go`; `git status --porcelain` empty.
Corroborating drift already present: `mergeerrors_test.go`'s `TestMergeErrors_NoVocabularyLeakInReasons` hand-list carries only 7 of the 9 members (missing `mergeReasonDetachedHead`, `mergeReasonAttemptIncomplete`).

### Log — residual B live drive (deployed dev binary, real hub)

Deployed via `./deploy-dev` (lyx @ 7ef1b63c); NOTE the PATH `lyx` at `~/.local/bin/lyx` is a stale July binary — every live command below used the absolute `.dev-bin/lyx` path.
Fixture: local bare warp `proj` (content on `main`), empty bare weft `proj-weft` (HEAD re-pointed to `main` — an empty bare's default `master` HEAD otherwise yields a `master-weft` weft prime that bricks `fabric add` on a `main` warp), `lyx fabric clone`, then `lyx fabric add task`.
Scenario (rows 27/28 shape — warp conclude landed, record never learned it):
1. Conflicting README edits on `main` and `task` warp branches; `lyx fabric merge-in task` → `conflicts:["README"]`, record retained (`warp_outcome: conflicted`, `weft_outcome: up_to_date`).
2. Resolved README, `git add`; then plain `git commit --no-edit` in the warp checkout — byte-identical on-disk state to a crash between conclude's `git commit` and the record save: warp HEAD on the merge commit, no MERGE_HEAD, `warp_committed` still `""`.
3. `lyx fabric merge --continue` (twice): both return `fabricengine: merge conclude did not finish; run MergeContinue again` — the retry re-runs `git commit --no-edit` on a clean tree, which fails forever. The printed instruction is false in exactly this state.
4. `lyx fabric merge --abort`: refuses with `merge conclude already landed` (R2's guard, correct).
5. `lyx fabric pull`: refuses with `a merge is in progress; run MergeContinue or MergeAbort first` — but neither verb can succeed.
6. The record lives at `proj-weft/.git/fabric-merge.json`. Plain git in the two checkouts cannot remove it, so doc.go's "plain git in the two checkouts is the last resort" does not actually clear this state — the only real escape is hand-deleting a fabric-internal state file no doc names.
Conclusion: the pair is permanently wedged for every fabric verb; round 2's "stuck is strictly better than destruction" holds, but "operator has plain git as a documented last resort" does not — the escape hatch neither works as documented nor is discoverable from what the CLI prints.
