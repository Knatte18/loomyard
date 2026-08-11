MILL_REVIEW_BEGIN
# Review: fabric: accumulate the result envelope from mutations, not control flow (slice 14) — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude (Opus-class, Anthropic); exact build string not verifiable from inside the sandbox
reviewed_file: plan/
date: 2026-08-11
```

## Findings

### [BLOCKING:scope] `mutateGitExclude` widening misses a 4th caller file
**Location:** batch 5 / card 17
**Issue:** The card widens `mutateGitExclude` to `(excludePath string, changed bool, err error)` and names "its three in-package callers (`seedGitExclude`, `unseedGitExclude`, `seedWeftArtifactExcludes`)" as the complete set, but `internal/fabricengine/gitexclude_integration_test.go` calls it at four sites (`:58`, `:62`, `:92`, `:100`), two of which bind the current second return (`changed, err := mutateGitExclude(...)`). That file is in neither the card's `Edits:` nor `## All Files Touched`.
**Fix:** Add `internal/fabricengine/gitexclude_integration_test.go` to card 17's `Edits:` and to `## All Files Touched`, and state that the three-caller list is production-only.

### [BLOCKING:scope] Deleting fabrictest's `Check` breaks `matrix_test.go`, not listed
**Location:** batch 7 / card 28
**Issue:** The card deletes `fabrictest`'s `type Check string` and its three constants and says "repoint every consumer", but its `Edits:` names only `refusal.go`, `refusal_test.go`, `verbs.go`, `doc.go`. `internal/fabricengine/fabrictest/matrix_test.go:108` carries `for _, other := range []Check{CheckContainment, CheckOwnership, CheckDirtiness}` and will not compile after the deletion; that file is Context-only here and is first edited by card 31.
**Fix:** Add `internal/fabricengine/fabrictest/matrix_test.go` to card 28's `Edits:` with the `fabricengine.Check`/`fabricengine.Check*` repoint of line 108, so card 28's own commit compiles.

### [BLOCKING:design] Admin-entry coverage derivation is wrong for weft targets
**Location:** batch 7 / card 29 (Coverage rule)
**Issue:** The rule says the `<prime>/.git/worktrees/<slug>` admin entry is derived "exactly as the harness's own `primeWorktreeAdminPermittedRoot` / `primeWeftAdminPermittedRoot` helpers already derive it", but those take a **warp** slug: `primeWeftAdminPermittedRoot` applies `weftname.SiblingPath("", slug)` (`verbs.go:403`), which turns a weft `worktree_created` target of `<slug>-weft` into `warp-bare-weft/.git/worktrees/<slug>-weft-weft`. Neither helper, fed a weft target, yields the real key, so every `Add`/`Remove` cell's weft admin change stays uncovered and fires a false lie of omission.
**Fix:** State the warp-vs-weft discrimination explicitly — strip `weftname.Suffix` from a target before calling `primeWeftAdminPermittedRoot`, or name both admin roots directly, and add a weft-side row to card 32's admin-entry test case.

### [NIT:consistency] `runReconcile` has four `output.Err` sites, not three
**Location:** batch 6 / card 25
**Issue:** "All three of this handler's `output.Err` sites become `errWithRecord`" is a miscount — `internal/fabriccli/fabric.go` has four (`:561` location resolution, `:569` `ReconcileFabricAt`, `:574` `LoadConfig`, `:581` `Reconcile`); only the last three qualify, since `rec` is built once `l` resolves.
**Fix:** Say "the three post-`l` `output.Err` sites", and state that `:561` stays a bare `output.Err` under card 24's pre-mutation carve-out.

### [NIT:consistency] `createWeftWorktree`'s branch-created condition has no source
**Location:** batch 5 / card 18
**Issue:** The card requires `AppendRef(KindBranchCreated, branch, "")` "when the call created the branch rather than checking out an existing one", but `createWeftWorktree` (`weftwiring.go:100-118`) always runs `worktree add -b <branch>`; the adopt-vs-create decision lives in the caller (`add.go`'s `weftBranchAlreadyExists`). The stated predicate is unimplementable inside the function.
**Fix:** State the entry as unconditional on `exitCode == 0 && err == nil` inside `createWeftWorktree`, since `-b` always creates.

### [NIT:consistency] `CommitWeftAt`/`PushWeftAt` claim overstated
**Location:** batch 8 / card 36 requirement 1
**Issue:** "Neither function exists any more" is true of the exported spellings only — `commitWeftAt`/`pushWeftAt` still exist unexported and are exactly what `Bolt.Commit`/`Bolt.Push` delegate to (`internal/fabricengine/bolt.go:23-30`).
**Fix:** Word the correction as "no exported `CommitWeftAt`/`PushWeftAt` remains; `boardengine` reaches the unexported pair through `fabricengine.Bolt`".

## Verdict

REQUEST_CHANGES
Two unlisted repoint files and a wrong weft admin-entry derivation must be fixed.
MILL_REVIEW_END
