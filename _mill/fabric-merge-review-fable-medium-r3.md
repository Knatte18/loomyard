# `fabric merge` — crucible round fable-medium-r3 review report

Round 3, scoped per the re-seeded round prompt: residuals A–C plus adversarial work in the regions they cross.
The CLOSED-AND-VERIFIED list (r1 F1–F8, r2 R1/R2/R3/R5, residual 1) is off-limits and was not re-litigated.

## Executive summary

Four findings: 0 BLOCKING, 2 MEDIUM, 1 LOW, 1 NIT. No CLOSED-AND-VERIFIED item was found wrong or incomplete.

- **B1 (MEDIUM, live-confirmed)** — the four invisible-conclude states (r2 rows 27/28/30/31) do not merely leave `MergeContinue` "stuck": they permanently wedge the pair. Continue loops on a false instruction, abort correctly refuses, every guarded sibling verb refuses, and the doc's plain-git escape hatch cannot remove the record. A cheap, schema-free adoption arm in `concludeMergeSides` (mirroring `sideConcludeMayHaveLanded`'s predicate) makes continue idempotent across a landed-but-unrecorded conclude and closes all four states.
- **A1 (MEDIUM, sabotage-confirmed)** — the closed-set closure test asserts closure by comparing two hand-maintained lists; an added member is invisible to the entire hermetic tier. An AST-backed membership test (repo precedent: `cmd/lyx/registration_test.go`) makes the same-commit rule real.
- **A2 (LOW)** — the drift A1 permits has already happened once: the reasons leak-test covers 7 of 9 members.
- **C1 (NIT)** — r2's 45-row adjudication survives spot-checking (method reproduced at 94, arithmetic consistent, ~20 verdicts re-checked); one unreachable "else" arm in row 24, safe direction, recorded here since prior reports are off-limits to edit.

Merge-readiness opinion (pre-fix): NOT yet — B1 leaves a real crash shape with no working recovery path, and A1 leaves the vocabulary invariant unenforced. Both are cheap, scoped fixes; with them landed this surface is merge-ready.

## Scope assessment

This round does not redo the broad plan-vs-shipped sweep; rounds 1 and 2 covered it and the orchestrator verified.
Scope here: residual A (closure test cannot detect an added member), residual B (four invisible-conclude states leave MergeContinue stuck), residual C (spot-check of round 2's 45-row adjudication).

## Findings

Severity-ranked. All CONFIRMED.

### B1 — MEDIUM — four invisible-conclude states permanently wedge the pair; `MergeContinue` is not idempotent across a landed-but-unrecorded conclude (residual B) — CONFIRMED live

`internal/fabricengine/mergelifecycle.go:41-48` and `:58-65` (the `CurrentSHA`/`saveMergeState` returns after a side's `git commit` landed).

Scenario (driven live, deployed binary, real hub — see the log): conclude's `git commit` lands on a side, then the process dies (or `CurrentSHA`/`saveMergeState` fails) before the record learns the SHA. The record now says `*_committed: ""` with the side's HEAD on the merge commit and no MERGE_HEAD.
- `merge --continue` re-runs `MergeConclude("")` → `git commit --no-edit` on a clean tree → fails → `ErrMergeIncomplete` **forever**, printing "run MergeContinue again" — an instruction that is false in exactly this state.
- `merge --abort` refuses (`merge conclude already landed`, R2's guard — correct).
- Every guarded sibling verb (`pull` driven live) refuses with "run MergeContinue or MergeAbort first" — neither can succeed.
- The record lives in the weft gitdir (`fabric-merge.json`); plain git in the two checkouts cannot remove it, so `doc.go`'s "plain git in the two checkouts is the last resort" does not clear this state. The only actual escape is hand-deleting a fabric-internal state file no doc names.

Round 2's reasoning ("stuck is strictly better than destruction; plain git is the documented last resort") half-holds: stuck genuinely beats destruction, but the documented last resort does not work here, so "stuck" is in fact "permanently wedged".

Suggested fix (cheap, no schema change, no new failure mode): give `concludeMergeSides` an adoption arm mirroring `sideConcludeMayHaveLanded` — before re-running a side's conclude, when the side's recorded committed SHA is empty, its outcome is `staged`/`conflicted`, its HEAD has moved off its recorded pre-merge SHA, and no MERGE_HEAD is present, adopt the current HEAD as that side's conclude SHA instead of re-committing. This keeps the abort/continue mirror total: every state `MergeAbort` refuses via `concludeLandedReason` becomes a state `MergeContinue` can actually finish, and the `ErrMergeIncomplete` message becomes true again. Update `doc.go`'s recovery prose in the same change.

### A1 — MEDIUM — the closed-set "closure" test cannot detect an added member (residual A) — CONFIRMED by sabotage

`internal/fabricengine/mergevocab_test.go:127`, `TestMergeVocabulary_GuardReasonSetIsClosedAndSideFree`.

Its doc comment claims "adding a member without updating this test fails loudly". It does not: `want` (literals) and `got` (constant references) are both hand-maintained inside the test, and nothing reads the const block in `mergeerrors.go`. Reproduced independently: added `mergeReasonSabotageTenth` to the closed set, whole hermetic `fabricengine` tier stays green, restored to an empty diff.

Suggested fix, following the repo's `go/ast` invariant-test precedent (`cmd/lyx/registration_test.go` et al.): parse `mergeerrors.go`, extract every `mergeReason*` constant name and value from the const block, and assert exact equality against one pinned name→value map — making the same-commit rule real. Drive the other hand-lists (side-free assertion, leak lists) from that single pinned source so they can no longer drift. Prove detection by sabotage (add a member, watch the new test fail at its intended assertion, revert to empty diff).

### A2 — LOW — hand-list drift already happened: the leak test covers 7 of 9 members — CONFIRMED

`internal/fabricengine/mergeerrors_test.go`, `TestMergeErrors_NoVocabularyLeakInReasons`: its `reasons` list is missing `mergeReasonDetachedHead` and `mergeReasonAttemptIncomplete` — the two members added after the list was written. Neither missing member currently leaks vocabulary, so no assertion is wrong today, but this is A1's failure mode already realized. Fixed by A1's single-source refactor.

### C1 — NIT — r2 adjudication row 24 contains an unreachable arm; no code change possible or needed (residual C)

`_mill/fabric-merge-review-opus-medium-r2.md` row 24 says `MergeContinue` "refuses (F1) if an outcome is empty, else concludes a half-reset pair". The else arm is unreachable: `selfAbortMergeAttempt`'s four call sites are all `MergeStart` error returns, where the failing side's outcome is by construction still empty, so F1 always refuses. Safe-direction inaccuracy in a prior round's report; the guard is unaffected, prior reports are off-limits to edit, and the correction is recorded here instead. Everything else checked in the table — the method (94 reproduced), the arithmetic (17 = 13 + 4), and ~20 row verdicts — holds.

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

### Log — residual C spot-check (after own findings were complete)

Reproduced r2's enumeration sweep verbatim (the `ext()` awk extent resolver + error-return awk over the eight functions): per-function 30/30/5/2/6/12/8/1, **TOTAL 94 — matches r2's raw count exactly**.
Destructive arithmetic re-derived from the table's own rows: MergeIn 10–13 (4) + Merge 20–23 (4) + concludeMergeSides 27–31 (5) + MergeContinue 38–41 (4) = **17**, of which 27/28/30/31 invisible (**4**) → 13 visible. Consistent with the summary block.
Rows spot-checked against the current tree (rows 1, 7, 10–13, 20–23, 24, 26, 27/28/30/31, 34, 38–41, 44, 45): every `next continue`/`next abort` verdict is right, and every DESTRUCTIVE abort arm is now covered by R2's `concludeLandedReason` guard (verified live for the invisible arm in the residual-B drive above).
One inaccuracy found, safe direction: row 24 claims MergeContinue "refuses (F1) if an outcome is empty, **else concludes a half-reset pair**" — the else arm is unreachable, because `selfAbortMergeAttempt` is only ever invoked from a `MergeStart` error return, and the failing side's outcome is by construction still empty at every one of its four call sites, so F1 always refuses. Report-accuracy NIT in a prior round's orchestrator-owned report; no code is wrong and the guard is unaffected. Not escalated.

## Post-fix verdict

All four findings closed in-round (B1, A1, A2 by code; C1 by record — see the fixer report).
Final gates green from the finished tree: hermetic `-count=5` across fabricengine/fabriccli/gitrepo/cmd/lyx, full `-tags integration` across the three packages (42.1s/4.1s/2.2s), golangci-lint clean.
The B1 wedge was re-driven live end to end on the deployed binary and now recovers by adoption; A1's detection was proven by the exact sabotage that previously went unseen.
**Merge-readiness: READY.** No open residuals from this round; the one deliberate open item inherited from round 2 (residual B) is now closed rather than re-deferred.
