# fabric merge surface — fixer report, round 8 (`fable-max-r8`)

```yaml
round: 8
tag: fable-max-r8
review_report: _mill/fabric-merge-review-fable-max-r8.md
findings_total: 5
fixed: 5
deferred: 0
```

Every finding from the round 8 review is fixed, committed individually, and verified. No finding was deferred and none was NOT-FIXED-THIS-ROUND.

## Fixes, one commit each

### r8-F1 (MEDIUM) — conflict envelope names the mandatory merge-stage step — commit `beb74807`
- `internal/fabriccli/envelope.go`: `errConflictsWithRecord`'s fixed error text now reads `merge produced conflicts; resolve each listed path, mark it resolved with "lyx fabric merge-stage <path>...", then run "lyx fabric merge --continue"` — the full three-step sequence instead of the two-step one an operator/agent could not complete (weft-side conflicts have no `git add` substitute). Godoc updated with the rationale.
- `internal/fabriccli/envelope_test.go`: the verbatim message pin moved with the message.
- Verified: build/vet/hermetic green; `TestRunCLI_MergeIn*`/`TestRunCLI_MergeStage*` integration green; new text observed live in both a warp-side and a weft-side conflict envelope (post-fix re-drive, hub B).

### r8-F2 (NIT) — merge-stage bad-path error drops the "either side" tell — commit `301011fc`
- `internal/fabricengine/mergestage.go`: partition error reworded to `%s is not a conflicted path in this merge; pass paths exactly as the conflicts list reported them` — side-free (no "two subjects were checked" disclosure) and naming the exact-spelling contract, which also fixes discoverability for `./`-prefixed/absolute spellings of genuinely conflicted paths. In-body comment explains both properties.
- Verified: engine + CLI not-conflicted tests green (they assert the path is named, which still holds); new message observed live on `merge-stage ./app.txt` mid-conflict.

### r8-F4 (NIT) — merge-stage staged echo deduplicates — commit `73911d55` (amended)
- `internal/fabriccli/merge_verbs.go`: success envelope echoes `uniquePreservingOrder(args)` instead of `args` verbatim; helper added with godoc.
- `internal/fabriccli/merge_cli_integration_test.go`: new `TestRunCLI_MergeStageEchoesEachPathOnce` — duplicate path stages fine, `staged` carries exactly one entry. Sabotage-proven: reverting the echo to `args` fails it.
- Process disclosure for the orchestrator: the first commit of this fix accidentally contained only the test — the sabotage-proof's `git checkout --` restored the not-yet-staged production file to HEAD, and a piped `tail` masked the resulting red test. Caught immediately (the very next targeted run), production change re-applied, commit AMENDED in place (local, unpushed) so the finding maps to one complete commit. Lesson recorded: never gate a commit on a piped test invocation.
- Verified: all four merge-stage CLI tests green post-amend.

### r8-F3 (MEDIUM) — foreign state and pair dirtiness re-verified under the write lock — commit `8598bf25`
- `internal/fabricengine/merge.go`: new `recheckMergePreconditionsUnderLock` (record → foreign → dirty, in that order — a foreign conflicted index is also tracked-dirty, and the foreign refusal names the actual remedy) replaces both verbs' record-only re-check. MergeIn and Merge now behave, for a mid-wait racing human, the way a strictly sequential call over the same state behaves.
- `internal/fabricengine/mergelock_integration_test.go`: four new lock-race tests in the file's established hold-launch-mutate-release pattern:
  - `TestMergeIn_ForeignStateAppearingWhileWaitingForLock_Refuses` / `TestMerge_…` — a plain-git conflicted merge landing mid-wait refuses as `*ErrForeignMergeState`, no record written, operator's state byte-identically untouched.
  - `TestMergeIn_PairTurningDirtyWhileWaitingForLock_RefusesPreservingDirt` / `TestMerge_…` — a mid-wait tracked edit on a file the source also touches refuses with exactly `["worktree dirty"]`, the edit survives, and (Merge) the mutation record is empty, pinning that the re-check fires before the pre-merge sync step.
- Sabotage-proven against the exact pre-fix state (re-check reduced to record-only): all four tests fail 3/3 runs — no timing flake — and the captured pre-fix failure shows the traced destruction chain live (`git merge` refusing on the operator's dirt → `selfAbortMergeAttempt` WARN → force reset → raw wrapped git error to the caller).
- `internal/fabricengine/doc.go`: the write-lock paragraph now documents the foreign/dirty re-probes and states the residual TOCTOU honestly (the re-checks cover the seconds-wide fetch + lock-wait window; no re-check closes the final instants against an external actor).
- Verified: hermetic `-count=2` green; full lock-test family `-count=2` green.

### r8-F5 (NIT, late lint-gate finding) — vestigial var/assign split — commit `a5a9f03a`
- `internal/fabriccli/merge_verbs.go:101`: `var mergeCmd *cobra.Command` + assignment merged to `mergeCmd := &cobra.Command{…}` (gosimple S1021; nothing in the literal referenced the variable). Found by the golang-build skill's `golangci-lint run` step during Job 2 verification, recorded in the review report with that provenance.
- Verified: build/vet/hermetic green; `golangci-lint run` clean on `internal/fabricengine/... internal/fabriccli/...`.

## Final verification (whole surface, post-all-fixes)

- `go build ./...` → 0. `go vet` (three packages) → 0.
- `go test -count=5 ./internal/fabricengine/... ./internal/fabriccli/... ./internal/gitrepo/... ./cmd/lyx/...` → all ok.
- `go test -tags integration -count=1 -timeout 30m` (three packages) → all ok (fabricengine 34.0s, fabriccli 3.4s, gitrepo 1.7s).
- `golangci-lint run ./internal/fabricengine/... ./internal/fabriccli/...` → clean.
- Re-deployed (`./deploy-dev` @ a5a9f03a) and re-drove live on a fresh hub: warp-side conflict lifecycle with the new F1 message → naive `--continue` refusal → F2's new bad-path message on `./app.txt` → deduped `staged` echo on a duplicate path → `--continue` `committed:true`; weft-side conflict → new envelope message → sibling `commit` refusal mid-merge → `merge-stage` → `--abort` restoring content exactly.
- Teardown: `git status` clean of everything but this round's commits; zero stray `lyx` processes; all scratch hubs under the session scratchpad, outside the repo.

## Sandbox suite

No new `SANDBOX-FABRIC-SUITE.md` scenario is required: F1/F2/F4 change message/envelope content already exercised by scenario F18 (which pins behavior, not those strings), and F3's lock-race scenario needs hold-the-lock orchestration that only the integration harness can stage — it is recorded as the four `*WhileWaitingForLock*` tests rather than a hand-drivable sandbox scenario. Noted here per the round prompt's instruction.

## Deferred / NOT-FIXED-THIS-ROUND

None.
