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

## Findings

(provisional entries appended as spotted; finalized before Job 2 begins)

## Deferred-item re-evaluation

(filled after own pass)

## Scope verdict

(filled at end of Job 1)

## Correctness verdict

(filled at end of Job 1)
