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

## Findings

(provisional entries appended as spotted; finalized before Job 2 begins)

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

### F3 (LOW, PLAUSIBLE — race-only, not reproduced) — MergeIn/Merge re-verify only the RECORD under the write lock; the foreign-state and clean-pair guards stay pre-lock knowledge
- `internal/fabricengine/merge.go:170–189` (MergeIn) and `:388–398` (Merge): under the lock, only `mergeRecordExists` is re-checked (plus SHA re-reads). `foreignMergeStatePresent` and `pairDirtyReason` were evaluated before the guard stage's fetches and before any lock wait.
- Fabric-internal writers are covered (any fabric merge leaves a record → record re-check refuses), but the CONSTRAINTS-sanctioned human running plain git in the warp checkout can create git-level merge state (or tracked dirt) during the fetch window or a lock wait. Consequences: (a) foreign conflicted entries appearing mid-window make the failing `MergeStart` classify as `MergeConflicted` and fabric records/reports a merge over the human's foreign state — the exact adoption the foreign-state design refuses; (b) tracked dirt appearing mid-window makes `MergeStart` fail genuinely and `selfAbortMergeAttempt` reset it away with `force: true` (the sequential untracked variant is safe — Phase 3 step 7 — but tracked dirt would be destroyed).
- This is the campaign's stale-guard family (round 7 F5's shape, one ring further out). The window cannot be closed against an external actor, but the widest parts (two network fetches + lock wait) are cheap to cover: re-evaluate `foreignMergeStatePresent` (and pair dirtiness) under the lock next to the record re-check.
- Fix: add the two re-checks under the lock in both verbs, with an integration test driving the lock-wait shape for the foreign case (mirroring `mergelock_integration_test.go`'s existing pattern).

### F4 (NIT, CONFIRMED by code read) — merge-stage success envelope echoes the caller's `staged` array verbatim, duplicates included
- `internal/fabriccli/merge_verbs.go:222–225`: `"staged": args`. Driving `merge-stage _lyx/notes.md _lyx/notes.md` reports `staged:["_lyx/notes.md","_lyx/notes.md"]` (Phase 3 step 3). Harmless but the envelope claims two stagings for one path.
- Fix: deduplicate (order-preserving) before echoing.

## Deferred-item re-evaluation

(filled after own pass)

## Scope verdict

(filled at end of Job 1)

## Correctness verdict

(filled at end of Job 1)
