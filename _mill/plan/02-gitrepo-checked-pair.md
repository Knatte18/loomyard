# Batch: gitrepo-checked-pair

```yaml
task: 'gitexec: add the checked entry point and migrate the call sites'
batch: gitrepo-checked-pair
number: 2
cards: 8
verify: go build ./... && go test ./internal/gitrepo/... ./cmd/lyx/... && go test -tags integration ./internal/gitrepo/...
depends-on: [1]
```

## Batch Scope

This batch adds `runChecked` beside `run` in `internal/gitrepo` and migrates 19 of the 21 `r.run(` call sites to it, leaving `Pull` and `Fetch` raw with markers.
It is one batch because the Client Boundary guard breaks the moment `runChecked` exists — `TestGitrepoBoundary_PinnedRunCallSites` asserts the `r.run(`-containing method set and counts `gitexec.` occurrences — so the guard repair must land with the migration or the batch cannot pass its own `verify:`.
The package's prose corrections land here too, for the same reason: `internal/gitrepo/doc.go` states "one function" and documents a `ResetHard` stderr-suppression contract this batch removes.

Batch-local decisions beyond `## Shared Decisions`:

- `runChecked` is a **second chokepoint beside `run`**, not a wrapper around it — implementing it on top of `run` would force this package to construct `*gitexec.GitError` itself.
- `ResetHard` migrates and `doc.go`'s "no-stderr-leak style" clause for it is deleted in the same commit: `reset_test.go` asserts on the resulting SHA, the restored file content, and `ErrInvalidSHA`, and never touches `err.Error()`, so nothing pins that suppression.
  `Pull` and `Fetch` are the opposite case and stay raw.
- `TestPush_CrossCloneRebaseRetry` already drives the non-fast-forward recovery path, so no new push test is added here;
  the discussion's "add one if none exists" clause is satisfied by the existing test.

## Cards

### Card 5: the runChecked sibling and the raw marker on run's own body

- **Context:**
  - `internal/gitexec/gitexec.go`
- **Edits:**
  - `internal/gitrepo/gitrepo.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `func (r *Repo) runChecked(args ...string) (string, error)` directly below the existing `run` method, with the single-statement body `return gitexec.Run(args, r.path)`.
  Do not implement it in terms of `run`.
  Add an adjacent marker comment on the `gitexec.RunGit(args, r.path)` line inside `run`'s own body reading, in substance, `//gitexec:raw — the raw half of the gitrepo checked/raw pair, by design; each of its callers carries its own justification`.
  Update the file's own top-of-file comment, which currently names "the shared run helper over gitexec.RunGit", to describe the pair.
  Do not change `run`'s signature or body beyond adding that marker comment.
- **Commit:** `feat(gitrepo): add the runChecked chokepoint beside run`

### Card 6: migrate gitrepo.go's eleven call sites

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/gitrepo/commitempty_integration_test.go`
- **Edits:**
  - `internal/gitrepo/gitrepo.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate every `r.run(` call in `StageAndCommit`, `CommitEmpty`, `StageAllAndCommit`, `CheckoutDetached`, and `RestoreBranch` to `r.runChecked(`, applying `default-merge-rule` at the plain sites (`add --`, `commit -m`, `ls-files --cached`, `commit --allow-empty`, `add -A`, `checkout --detach`, `checkout`), and `errors.As` recovery at the three `diff --cached --quiet` tri-states.
  Transcribe each tri-state per site, because the answer codes are inverted between two of them: in `CommitEmpty`, exit 0 falls through and proceeds to commit while exit 1 returns `ErrIndexNotEmpty`;
  in `StageAllAndCommit`, exit 0 returns `("", false, nil)` for nothing-to-commit while exit 1 falls through and proceeds to commit;
  `StageAndCommit`'s own gate is a third variant and is transcribed the same way, from its current `switch code` block.
  The shape at each tri-state is: nil error means exit 0;
  `var gitErr *gitexec.GitError` plus `errors.As(err, &gitErr) && gitErr.ExitCode == 1` means exit 1;
  anything else is the default branch, which now wraps the error with `%w` instead of formatting the discarded stderr.
  `ErrIndexNotEmpty`, `ErrNoCommits`, and `ErrInvalidSHA` keep their existing identities and are returned bare exactly as today.
  Do not introduce the literal `"-f"` anywhere in this package — the Never Force-Add Invariant bans it in non-test source.
- **Commit:** `refactor(gitrepo): migrate gitrepo.go's call sites to runChecked`

### Card 7: migrate push.go, including the sniff and the sentinel hybrid

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/gitrepo/push_test.go`
  - `internal/fabricengine/coalesce.go`
- **Edits:**
  - `internal/gitrepo/push.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate all four `r.run(` sites in `pushWithRebaseRetry`, the one in `PushRebaseFree`, and the one in `HasUnpushed` to `r.runChecked(`.
  In `pushWithRebaseRetry`, move the `containsAny(stderr, rebaseRetryTriggers)` trigger sniff onto the recovered error as `errors.As(err, &gitErr) && containsAny(gitErr.Stderr, rebaseRetryTriggers)`, and move the `"no rebase in progress"` abort check onto the abort call's own recovered `gitErr.Stderr` the same way.
  Keep the `pull --rebase` call's `*GitError` bound: its exit branch is control flow, not a message — `errors.As` on that error gates the `rebase --abort` spawn, and anything that is not a `*GitError` returns immediately without attempting an abort, because an abort after a failure in which git never ran is wrong.
  The two abort-outcome messages keep rendering the prior `pull --rebase` call's stderr from that bound `gitErr.Stderr`, exactly as today;
  this is the prior-call-diagnostic carve-out and the fragment is not dropped.
  In `PushRebaseFree` the migrated shape is: nil error returns nil;
  `errors.As(err, &gitErr) && containsAny(gitErr.Stderr, rebaseRetryTriggers)` returns `ErrPushRejected` **bare**, with no `%w` and no `%v` wrapper;
  anything else returns `fmt.Errorf("gitrepo: git push: %w", err)`.
  Applying the default merge rule there instead would break `errors.Is(err, gitrepo.ErrPushRejected)` at `internal/fabricengine/coalesce.go` and fail `TestPushRebaseFree_DivergedRemote_ReturnsErrPushRejected`.
  In `HasUnpushed` the migrated shape is: nil error compares the trimmed stdout against `"0"`;
  `errors.As(err, &gitErr)` returns `(true, nil)`, preserving the no-upstream-configured answer;
  anything else returns `(false, err)`.
  Update `HasUnpushed`'s godoc so it describes the recovery in terms of the error type rather than the exit code.
  Do not modify `internal/fabricengine/coalesce.go` or `internal/gitrepo/push_test.go` — both are read to confirm the sentinel surface survives unchanged.
- **Commit:** `refactor(gitrepo): migrate push.go, preserving the rebase sniff and ErrPushRejected`

### Card 8: migrate IsAncestor's tri-state

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/gitrepo/ancestry_integration_test.go`
- **Edits:**
  - `internal/gitrepo/ancestry.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate `IsAncestor`'s single `r.run("merge-base", "--is-ancestor", sha, ref)` site to `r.runChecked`.
  The migrated shape is: nil error returns `(true, nil)`;
  `errors.As(err, &gitErr) && gitErr.ExitCode == 1` returns `(false, nil)`;
  anything else returns `(false, fmt.Errorf("gitrepo: merge-base --is-ancestor %s %s in %s: %w", sha, ref, r.path, err))`.
  The `git exited %d` fragment in the current default branch is deleted along with its `code` argument.
  The two `ErrInvalidSHA` pre-git guards are untouched.
- **Commit:** `refactor(gitrepo): migrate IsAncestor to runChecked with errors.As recovery`

### Card 9: migrate ResetHard and delete the suppression clause it never had

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/gitrepo/reset_test.go`
- **Edits:**
  - `internal/gitrepo/reset.go`
  - `internal/gitrepo/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Migrate `ResetHard`'s `r.run("reset", "--hard", sha)` site to `r.runChecked`, collapsing its two guards into `if err != nil { return fmt.Errorf("gitrepo: reset --hard %s in %s: %w", sha, r.path, err) }` and deleting the `git exited %d` fragment with its `code` argument.
  The `ErrInvalidSHA` guard returns before any git spawn and is unchanged.
  In `internal/gitrepo/doc.go`, delete the sentence in the `# ResetHard surface` section reading "Non-zero-exit errors follow Pull's no-stderr-leak style: the repo path and git's exit code, never raw stderr", and replace the `reset.go` file header's description of the same behaviour if it repeats the claim.
  Do not touch the `# Pull surface` section's equivalent clause — `Pull` genuinely keeps that contract.
- **Commit:** `refactor(gitrepo): migrate ResetHard and drop its unpinned suppression claim`

### Card 10: mark Pull and Fetch raw

- **Context:**
  - `internal/gitrepo/pull_test.go`
  - `internal/gitrepo/fetch_integration_test.go`
- **Edits:**
  - `internal/gitrepo/pull.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Leave both `r.run(` call sites in `Pull` and `Fetch` on the raw form, with their two-guard shape and their exact current error strings unchanged, and add an adjacent `//gitexec:raw — <why>` marker comment at each.
  The justification must name the pinned contract in substance: a non-zero exit here IS a failure, but folding git's stderr into the message would break the test-enforced no-`fatal:`-leak surface, so the reproduction pointer stands in for the stderr.
  Both markers must use the same `//gitexec:raw` token that `run`'s body carries — one invariant, one searchable token.
  Do not edit `internal/gitrepo/pull_test.go` or `internal/gitrepo/fetch_integration_test.go`;
  they are read as the evidence for the marker's claim and must keep passing unchanged.
- **Commit:** `docs(gitrepo): mark Pull and Fetch as deliberate raw sites`

### Card 11: correct doc.go's falsified prose

- **Context:**
  - `internal/gitexec/gitexec.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/gitrepo.go`
- **Edits:**
  - `internal/gitrepo/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `# Relationship to internal/gitexec — the two-backend boundary` section, replace the claim that gitexec is "deliberately minimal: one function, RunGit(...)" with a description of the two-function pair, stating which form each CLI-bound `gitrepo` method sits on: `Pull` and `Fetch` on the raw `run`, everything else on `runChecked`.
  Replace the "roughly eighty call-sites across packages" figure with a figure re-derived at implementation time by grepping the non-test Go sources for the `gitexec.Run` prefix — do not transcribe the stale number, and do not restate the count as an exact value the next edit will falsify;
  a rounded figure with the packages named is enough.
  Update the CLI-bound method list in the same section if the migration changed which methods reach the CLI — it did not, so confirm rather than assume.
  Leave the locale-matching paragraph in place: `pushWithRebaseRetry`'s stderr sniffs still match git's untranslated text, they just read it off `gitErr.Stderr` now, and pinning the subprocess locale stays a deliberately-untaken gitexec-level decision.
- **Commit:** `docs(gitrepo): correct the two-backend boundary prose for the checked pair`

### Card 12: repair the Client Boundary guard

- **Context:**
  - `internal/gitrepo/gitrepo.go`
  - `internal/gitrepo/push.go`
  - `internal/gitrepo/pull.go`
  - `internal/gitrepo/ancestry.go`
  - `internal/gitrepo/reset.go`
- **Edits:**
  - `cmd/lyx/gitrepoboundary_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Change the guard's method-set key from `r.run` alone to the union of `r.run` and `r.runChecked`: `bodyCallsMethodOnReceiver` matches on the exact AST method name, so call it twice per method and add the name when either matches.
  `gitrepoPinnedRunBoundMethods` keeps all twelve current entries unchanged — after migration only `Pull`, `Fetch`, and `HasUnpushed` still call `r.run`, so keying on `r.run` alone would go blind to the other nine.
  Replace the `gitexecTotal += strings.Count(rendered.String(), "gitexec.")` whole-file substring count and its `!= 1` assertion with an AST call-expression count: walk each parsed file with `ast.Inspect`, count every `*ast.CallExpr` whose `Fun` is an `*ast.SelectorExpr` whose `X` is the package ident `gitexec` and whose `Sel` is `Run` or `RunGit`, and assert the total is exactly two.
  A substring count is no longer usable because roughly six methods now declare `var gitErr *gitexec.GitError` for their `errors.As` recovery, so the count lands near eight and `!= 2` would be as false as `!= 1`.
  Keep the existing "the call site must live inside `run`'s own body" assertion but widen it to the pair: one call expression inside `run`'s body and one inside `runChecked`'s.
  Update the file's header comment, which currently states "exactly one `gitexec.RunGit` call site exists in the package's non-test source", and its `gitrepoPinnedRunBoundMethods` godoc, which names methods by which helper they call.
  Add a one-line cross-reference to the gitexec Checked-Call Invariant, noting that this guard is keyed by **method name** and answers which methods may reach the CLI at all, while that one is keyed by **call site** and answers which sites may use the raw form.
  Keep the `gitrepoBoundaryMinScannedFiles` vacuous-scan floor.
  Do not edit `CONSTRAINTS.md` here — its matching amendment lands in batch 8, alongside the new invariant it cross-references.
- **Commit:** `test(cmd/lyx): key the Client Boundary guard on both gitrepo helpers`

## Batch Tests

`verify:` runs `go build ./...`, then the Tier 1 suites of `internal/gitrepo` and `cmd/lyx` (the latter is what proves card 12's repaired `cmd/lyx/gitrepoboundary_test.go` passes against the migrated package, and that `cmd/lyx/tierpurity_test.go`, `hermeticenv_test.go`, and `rawgitmutation_test.go` still pass with their current tokens), then `go test -tags integration ./internal/gitrepo/...`.
That Tier 2 run is the regression net for all 19 migrated sites and carries four specifically load-bearing files: `internal/gitrepo/pull_test.go` and `internal/gitrepo/fetch_integration_test.go` must pass **unchanged**, which is what proves the two raw suppression sites stayed raw;
`internal/gitrepo/push_test.go` covers `HasUnpushed`'s no-upstream answer through the new `errors.As` path, `TestPush_CrossCloneRebaseRetry` covers the migrated trigger sniff, and `TestPushRebaseFree_DivergedRemote_ReturnsErrPushRejected` catches the hybrid being flattened;
`internal/gitrepo/commitempty_integration_test.go` and `internal/gitrepo/ancestry_integration_test.go` exercise the tri-state answer branches, where a mis-transcribed exit-code comparison silently converts an answer into a failure.
`internal/gitrepo/reset_test.go` must keep passing;
it does not assert on error text, which is why `ResetHard` could migrate at all.
