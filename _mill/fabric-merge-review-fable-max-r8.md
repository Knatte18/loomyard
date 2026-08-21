# fabric merge surface — independent review, round 8 (`fable-max-r8`)

```yaml
round: 8
tag: fable-max-r8
model: fable
effort: max
date: 2026-08-21
worktree: /home/knatte/Code/loomyard/wts/fabric-merge-crucible-round4
branch: fabric-merge-crucible-round4
clean_room: true
```

Clean-room pass: no prior-round review/fixer material read before the findings below were complete.
Sources read first: SPEC (`git show 3b800bc8:_mill/discussion.md`, plan `00-overview.md`), `crucible/README.md`, the merge-surface production and test code, `internal/fabricengine/doc.go`'s merge section, CLI surface, `docs/overview.md`, `CONSTRAINTS.md`, `tools/sandbox/SANDBOX-FABRIC-SUITE.md` (scenario ideas only).

## What was tested

(appended in real time as each command/scenario returns)

### Phase 1 — source reading (complete before any test ran)

- SPEC: `git show 3b800bc8:_mill/discussion.md` (full read), `_mill/plan/00-overview.md` (Shared Decisions).
- Production: `internal/fabricengine/merge.go`, `mergelifecycle.go`, `mergeguards.go`, `mergestate.go`, `mergestage.go`, `mergepaths.go`, `mergeerrors.go`, `destroy.go` (resetHardTo/ResetHard/resetMergeSides region), `internal/gitrepo/merge.go`, `internal/fabriccli/merge_verbs.go`, `envelope.go`, `weft_verbs.go`.
- Docs: `internal/fabricengine/doc.go` "# The merge surface" (846–1126).
- Tests read in full: `mergestage_integration_test.go`, `merge_cli_integration_test.go`, `merge_target_integration_test.go` lines 720–892 (round 7's four Diverged/Behind tests + helpers); test-name inventory of all 10 other merge test files (6842 lines total across the merge test surface).
- Support plumbing verified: `gitexec.GitError.Error()` (renders args+exit+stderr, no Dir), `weftname.Suffix` (`-weft` sibling dirs), `gitrepo.Fetch/IsAncestor/CurrentSHA/ResetHard`, `weftGitDir`, `RecordCorrespondence`, sibling-guard call sites (checkout.go:48, commit.go:112–137, pull.go:221, remove.go:65–81), merge mutation kinds in `mutation.go`.

### Phase 2 — hermetic gates (baseline, pre-fix)

- `go build ./...` → rc 0.
- `go vet ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` → rc 0.
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` → all ok.
- `go test -tags integration -count=1 -timeout 30m ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/...` → all ok (fabricengine 32.7s, fabriccli 3.1s, gitrepo 1.7s).

### Phase 3 — live driving (dev binary `.dev-bin/lyx` @ be99498d, scratch hub under the session scratchpad, `GIT_CONFIG_GLOBAL` seeded with `defaultBranch=main`)

Hub A: bare warp+weft, seeded warp `main`, `lyx fabric clone` → `warp-HUB` (prime pair `warp`/`warp-weft`), `lyx fabric add feature|task2|task4`.

1. Warp-side conflict lifecycle: diverge `src/app.txt` on `feature` vs `main`; `merge-in feature` → exit 1, `conflicts:["src/app.txt"]`, `partial:false`, two `merge_staged` mutations. Edit file → `merge --continue` → REFUSED `unresolved conflicts remain` (see Finding F1). `merge-stage src/app.txt` → ok; `merge --continue` → ok, `committed:true`, conclude message SHA-labelled (`Merge commit '7105d…'`), weft side fast-forwarded with no fabricated commit.
2. Weft-side conflict lifecycle: `_lyx/notes.md` diverged via `lyx fabric commit` on prime and task2; `merge-in task2` → conflict at unified path `_lyx/notes.md`; markers through the junction show `>>>>>>> <SHA>` (no `-weft` leak); `git add -- _lyx/notes.md` from visible worktree → `fatal: pathspec … beyond a symbolic link` (proves merge-stage is the only route); `merge-stage _lyx/notes.md` → ok (`merge_resolved_staged` on weft); `merge --continue` → concluded on weft only.
3. merge-stage path-shape probes mid-conflict: `./_lyx/notes.md`, absolute path, trailing slash, `../task2/_lyx/notes.md` → all loud errors ("… is not conflicted on either side"), nothing staged; duplicate path in one call → staged fine. Re-stage of an already-staged path → same loud error. See Findings F1/F2.
4. Abort: `merge --abort` mid-merge (after staging one side) → both `worktree_reset` entries, content restored exactly; `--continue`/`--abort`/`merge-stage` with no merge → `no merge in progress` / not-conflicted errors.
5. Foreign state (plain `git merge` conflicted in warp checkout): `merge-in`, `merge-stage`, `commit`, `merge --abort` all refuse with the fixed foreign-state message, state untouched; plain `git merge --abort` afterwards cleans it.
6. Detached HEAD: `git checkout --detach` on warp → `merge-in` refuses `checkout is not on a branch`.
7. Untracked-overwrite self-abort (sequential probe of the genuine-MergeStart-error path): untracked `src/new.txt` in prime + `merge-in task4` (whose branch adds it) → git refuses, selfAbort resets both sides (`partial:true`, two `worktree_reset`), record deleted (only the benign `fabric-merge.json.lock` remains), UNTRACKED FILE SURVIVES (reset --hard leaves it), HEADs unchanged. Wrapped raw git cause in the envelope error (plan-adjudicated shape).
8. Full documented cross-pair workflow: task2 `merge-in main` → conflict → resolve → `merge-stage` → `--continue`; prime `merge task2 --squash -m` → clean squash (weft conclude one-parent, `-m` honored, warp AUD side untouched), `committed:true`.
9. Behind-target sync arm live: outsider push to `main` (prime tracking stale-equal) → `merge task2` fetched, `repo_advanced` fast-forward, merged clean.
10. Round-7 F5 re-drive (unfetched diverged target): outsider push + prime local-only commit, tracking ref still ancestor of HEAD (guard sees "ahead" — asserted precondition) → `merge task2` → REFUSED `branch not synced to upstream`, both HEADs unchanged, empty mutations. The post-fetch sync arm holds.
11. Sibling dispositions during a live record: `commit`/`pull`/`checkout`/`remove <source pair>` (via mergeSourceInFlight)/`push` (its commit half) all refuse with the one typed message; `status`/`pairs` succeed; `remove <unrelated pair>` succeeds (guard correctly narrow).

### Phase 4 — CLI arity/exclusivity probes (live)

- `merge-in` (no arg) → "accepts 1 arg(s), received 0"; `merge a b` → usage; `merge --continue extra` → "takes no positional arguments"; `merge --continue --abort` → cobra mutual-exclusion error; `merge-in no-such-branch` → aggregated, sorted two-reason guard error. All loud, none side-naming.

### Phase 5 — sabotage proofs of round 7's own new mechanisms (scratch clone of HEAD, never this worktree; each sabotage reverted before the next)

| # | Sabotage | Expected red test | Result |
|---|----------|-------------------|--------|
| S1 | `syncSideBeforeMerge` diverged arm → `return nil` | TestMerge_UnfetchedDivergedTargetRefuses + WeftRefuses | BOTH RED (committed true, nil error) |
| S2 | `syncedToUpstreamReason` weft half forced false | TestMerge_FetchedDivergedWeftRefuses | RED — and via the layer-discriminating asserts (warp got synced first) |
| S3 | `sideNotSyncedToUpstream` behind arm → refuse | TestMerge_FetchedBehindTargetIsSyncedNotRefused | RED |
| S4 | `MergeStageResolved` weft staging call dropped (record append kept) | TestRunCLI_MergeStageIsTheOnlyRouteForAWeftSideConflict | RED (post-stage --continue refuses) |
| S5 | partition unknown-path arm → silent skip | TestRunCLI_MergeStageRejectsAPathThatIsNotConflicted + engine TestMergeStageResolved_PathNotConflictedOnEitherSide | BOTH RED |
| S6 | `merge-stage` Args → `cobra.ArbitraryArgs` | TestRunCLI_MergeStageRequiresAtLeastOnePath | RED (exit 0, `staged:[]`) |
| S7 | `MergeStageResolved` foreign-state arm dropped | TestMergeStageResolved_ForeignMergeStateRefusesWithoutStaging | RED |

Every round-7 mechanism named in this round's re-examination target is sabotage-proven load-bearing. Clone restored clean after each; sabotage clone discarded at teardown.

### Phase 6 — docs accuracy walk

- `internal/fabricengine/doc.go` "# The merge surface" cross-checked against the implementation while reading — every claim I checked (record-vs-derived, adoption evidence, lock ordering, sync-decided-twice, detached-HEAD, conflict reporting, sibling dispositions, `MergeStageResolved`'s no-lock argument incl. its foreign-state carve-out) matches the code as shipped.
- `docs/overview.md` fabric row lists all 19 verbs incl. the merge trio and describes merge-stage's junction role accurately.
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md` F18/F19/F20 cover the merge lifecycle, preconditions/hostile-state, and the two bookkeeping-divergence shapes; nothing in this round's findings needs a NEW scenario (F1's fix changes an envelope message's text, which F18 does not pin).
- Message-pin inventory for the fixes: conflict-envelope text pinned at `internal/fabriccli/envelope_test.go:280` (must move with F1); `not conflicted on either side` pinned nowhere (both tests assert only that the error names the path); vocab tests forbid warp/weft tokens and host-phrases, not the word "side" (which is why F2 passes enforcement).

## Findings

Final list. Severity per the campaign scale; CONFIRMED = reproduced/traced, PLAUSIBLE = code-read only.

### F1 (MEDIUM, CONFIRMED live) — the two runtime messages an operator actually follows never name the mandatory `merge-stage` step
- `internal/fabriccli/envelope.go:87` — conflict envelope error: `merge produced conflicts; resolve them, then run "lyx fabric merge --continue"`.
- An operator (or Finalize-style agent) following exactly that: edits the file, runs `merge --continue`, gets `fabricengine: merge preconditions failed: unresolved conflicts remain` — which is confusing (they DID resolve the content; what remains is the un-staged index entry) and also never names `merge-stage`. Loop forever unless they read `--help`. Reproduced live (Phase 3 step 1). For a weft-side conflict `git add` cannot even substitute, so the omission is a hard dead end in the machine-followable text.
- Round 7's F1 closed the *verb* gap and fixed the help text; the runtime guidance strings still describe the two-step lifecycle.
- Fix: make the conflict-envelope error name the three-step sequence (resolve → `lyx fabric merge-stage <paths>` → `lyx fabric merge --continue`). Keep the closed guard-reason set untouched (SPEC-pinned); the CLI owns its envelope error text.

### F2 (NIT, CONFIRMED live) — `MergeStageResolved`'s bad-path error says "on either side", disclosing that two subjects were checked
- `internal/fabricengine/mergestage.go:103` — `"fabricengine: %s is not conflicted on either side"`.
- The guard machinery in this surface is deliberately worded so no message "discloses that two subjects were checked" (pairDirtyReason et al.); this error does exactly that ("either side"), and it crosses the CLI boundary verbatim (driven live in Phase 3 step 3). Not caught by mergevocab tests, which pin warp/weft tokens, not "side".
- Also a discoverability miss: for `./`-prefixed/absolute spellings of a genuinely conflicted path the message gives no hint that the required spelling is the conflicts-array one.
- Fix: reword to name the real contract without the two-sided tell, e.g. `"fabricengine: %s is not a conflicted path in this merge; pass paths exactly as the conflicts array reported them"`.

### F3 (MEDIUM, PLAUSIBLE — race-gated, consequences traced, not raced live) — MergeIn/Merge re-verify only the RECORD under the write lock; the foreign-state and clean-pair guards stay pre-lock knowledge
- `internal/fabricengine/merge.go:170–189` (MergeIn) and `:388–398` (Merge): under the lock, only `mergeRecordExists` is re-checked (plus SHA re-reads). `foreignMergeStatePresent` and `pairDirtyReason` were evaluated before the guard stage's two network fetches and before any lock wait — a window of real seconds.
- Fabric-internal writers are covered (any fabric merge leaves a record → the record re-check refuses), but the CONSTRAINTS-sanctioned human running plain git in the warp checkout can create git-level merge state (or tracked dirt) inside that window. Traced consequences:
  - (a) Foreign conflicted state appearing mid-window: the failing `MergeStart` sees GitError + non-empty `ConflictedFiles` and classifies `MergeConflicted` — fabric writes a record over, and reports conflicts from, a merge it did not start (MergeIn), the exact adoption the foreign-state design exists to refuse; in `Merge` the conflict path immediately `resetMergeSides` (force: true) — DESTROYING the human's in-progress plain-git merge, resolutions included.
  - (b) Tracked dirt appearing mid-window: `MergeStart` fails genuinely and `selfAbortMergeAttempt` resets it away with `force: true`. (The sequential untracked variant is safe — Phase 3 step 7 proved reset --hard leaves untracked files.)
- This is the campaign's stale-guard family (round 7 F5's shape, one ring further out: the information becomes false rather than knowable-later, so the window cannot close fully against an external actor — but its widest parts, the fetches + the lock wait, are cheap to cover).
- Fix: re-evaluate `foreignMergeStatePresent` and pair dirtiness under the lock next to the existing record re-check, in both verbs; add lock-wait integration tests (mirroring `mergelock_integration_test.go`'s hold-the-lock-mutate-release pattern) for the foreign and dirty shapes.

### F4 (NIT, CONFIRMED by code read) — merge-stage success envelope echoes the caller's `staged` array verbatim, duplicates included
- `internal/fabriccli/merge_verbs.go:222–225`: `"staged": args`. Driving `merge-stage _lyx/notes.md _lyx/notes.md` reports `staged:["_lyx/notes.md","_lyx/notes.md"]` (Phase 3 step 3). Harmless but the envelope claims two stagings for one path.
- Fix: deduplicate (order-preserving) before echoing.

### Non-findings worth recording (checked, judged correct or already adjudicated)

- `selfAbortMergeAttempt` wrapping the raw git cause (side-ish path included) into the returned error: explicitly adjudicated by the plan's "genuine MergeStart error self-aborts symmetrically" Shared Decision ("return the wrapped MergeStart error"); mutation-record targets already expose the same paths by invariant. Not re-litigated.
- `Merge` refusing from the sync step AFTER the other side was legitimately fast-forwarded (mutations non-empty, `partial: true` on the refusal envelope): mechanically correct per the partial derivation rule; the advance is real upstream catch-up that stays by design (round 7's test asserts the layer split deliberately).
- CLI `push` refusing mid-merge via its commit half while engine `PushWeft` stays unguarded: consistent with the lock decision's disposition table (the CLI verb is commit-then-push).
- Re-running `merge-stage` on an already-staged path erroring: deliberate per godoc ("error rather than silent skip") and harmless to the flow; covered by F2's message reword rather than a behavior change.
- `mergeStateOrForeignErr` ordering, `MergeStart`'s MERGE_HEAD-means-staged arm, adoption's exact two-parent arity, `mergeSourceInFlight` prime+linked globs, `ConflictedFiles -z` raw paths, `--ff`/`--no-edit`/`-A` pins: all verified as shipped, each with its own regression test from earlier rounds.

## Deferred-item re-evaluation

1. **Windows path behaviour** (`weftPathVisible`/`unifyConflictPaths`): nothing more is executable on this Linux host than round 6 already did. The separator-parameterised seam (`weftPathVisibleWithSeparator`) lets the hermetic suite drive the Windows separator spelling, and it runs green here; the one atom beyond runtime reach (the `os.PathSeparator` argument at the entry point) remains pinned by source inspection, as doc.go states plainly. Still a named, never-executed-on-real-Windows gap for the campaign record.
2. **Round 7's own new mechanisms** (this round's re-examination target): all seven sabotages in Phase 5 went red on exactly the expected tests — the four Diverged/Behind tests, the three merge-stage CLI tests, and the merge-stage foreign-state arm (round 6's F7 mechanism) are load-bearing and sabotage-proof. No proof-quality defect found in them.
3. **Four states where `MergeContinue` gets stuck** (first instalment round 2 rows 27/28/30/31): unchanged; no adjacent code touched by this round's fixes (F1 is CLI message text, F3 adds pre-`MergeStart` re-checks, neither reshapes the conclude path).
4. **Post-record error-return class per-site adjudication**: not required this round; nothing observed contradicting the standing disposition.

## Scope verdict

The as-built merge surface delivers the SPEC. Every discussion decision traced to shipped code during Phase 1: two verbs with the millhouse split, the recorded (never derived) merge with foreign-state refusal, the lifecycle quartet driven off the record, no-new-commit-until-both-clean with recorded reversibility, unified sorted worktree-relative conflicts with the unmappable self-abort, SHA-only merge arguments and markers, aggregated side-free guards with the pinned closed reason set, the combined-lock scope (mutating steps only, never the resolution window), conclude-per-side-never-rollback with idempotent `MergeContinue`, correspondence recording on every conclude path, and the git-shaped CLI with mode-dependent arity. Post-SPEC additions (detached-HEAD guard, the adoption arm with exact-parentage evidence, `merge-stage` and its foreign-state refusal, the twice-decided not-synced predicate) are each documented in doc.go and carry their own tests. The plan's four adjudicated supersessions are honored. No silently-dropped requirement found; no over-reach found.

## Correctness verdict

Hermetic gates (build/vet/test -count=5) and the full `-tags integration` suite green before any change. Live driving across two conflict sides, the full documented operator lifecycle, abort/recovery, foreign state, detached HEAD, sibling dispositions, the untracked-overwrite self-abort, the behind-target sync arm, and the round-7 diverged-target fix all behave as documented. Round 7's new mechanisms are sabotage-proven. Four findings: F1 (MEDIUM, operator-facing guidance strings omit the mandatory `merge-stage` step — the naive operator/agent dead-ends), F3 (MEDIUM, stale pre-lock foreign/dirty guard knowledge in a racing-human window, with a destructive consequence in two arms), F2 and F4 (NITs). No BLOCKING findings; no data-loss path reachable without a concurrently-acting human. Merge-readiness: the surface is sound in the normal single-instance flow; the four findings are fixable within this round.
