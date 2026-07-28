MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-28
```

## Findings

### [GAP] Reindex policy undecided; trigger set under-enumerated
**Section:** `linked-worktree-and-interop-evidence` / `mixed-backend-call-sites`
**Issue:** The discussion narrows `Reindex()` to "after `Push`'s `pull --rebase` retry path", but the probe report's recommendation 5 names `StageAndCommit`'s commit and `SetSnapshotSHA`'s push/fetch as well, and observes `gc --auto` fires after ordinary commits and fetches — leaving `SnapshotSHA`'s and `adoptSnapshotRef`'s fetches uncovered; it then offers "or treat `object not found` as a signal to reindex once and retry" without choosing, and never addresses the probe's two open unknowns (Reindex cost on a many-worktree hub, and whether it is safe to call concurrently on a handle this design deliberately shares).
**Fix:** Choose one policy, enumerate every CLI call in `gitrepo` that can write a pack, and state the concurrency guard for a Reindex racing another reader.

### [GAP] CurrentBranch has no poc source and no parity case
**Section:** Technical context ("The go-git implementations already exist") / Testing
**Issue:** The claim that `gitnativepoc` carries spike-verified versions of "every method in scope" is false for `CurrentBranch` — there is no such func in `read.go`/`write.go` (it was one of the five never-examined methods) — and none of the enumerated parity cases covers it, even though the worktree probe warns it passes on a completely broken handle and the first probe warns `Repository.Head()` must not be used and detached HEAD must map to a wrapped error.
**Fix:** Name `CurrentBranch`'s implementation source and error mapping, and enumerate its parity cases (unborn, on-branch, detached, orphan).

### [GAP] The `gh` guard's CommandContext token cannot ever match
**Section:** `constraints-github-auth-invariant` (banned tokens)
**Issue:** `exec.CommandContext` takes the context first, so the literal `exec.CommandContext("gh"` never appears in compilable Go — and that is precisely the form the timeout-bounded `gh auth token` shell-out must use, so the guard would miss the one call shape the design itself mandates (the copied `pathresolve_guard_test.go:30` precedent carries the same latent hole).
**Fix:** Ban a spelling that matches the real form (e.g. `, "gh",` / `, "gh")` anchored tokens or a regex) rather than copying the unmatchable literal.

### [GAP] 401-invalidation has no owner under the thin client
**Section:** `token-resolution-and-cache` / `githubclient-thin-auth-only`
**Issue:** Consumers call go-github directly, so `githubclient` never observes a response; yet "a `401` invalidates the cache and triggers exactly one re-resolution" is stated as client behaviour and has a named test, with no mechanism given (auth `RoundTripper` inside the client? each consumer's duty?). The shell-out's context timeout likewise has no stated duration or seam, though a test must assert it.
**Fix:** Name the mechanism that sees the 401 and the timeout constant/injection point.

### [GAP] Leaf allowlist drops `internal/proc`, losing HideWindow
**Section:** `constraints-github-auth-invariant` (import allowlist)
**Issue:** Today's shell-out calls `proc.HideWindow(cmd)` (`selfreportengine/selfreport.go:57`) to suppress the Windows console window; the stated allowlist — stdlib, go-github, `golang.org/x/sys` — forbids importing `internal/proc`, so the `gh auth token` fallback either regresses to a visible console flash or trips the new leaf-enforcement test.
**Fix:** Allowlist `internal/proc` (it is stdlib-only) or state the `x/sys` reimplementation explicitly.

### [GAP] Probe-2 findings have no durable home
**Section:** `docs-lifecycle`
**Issue:** `.scratch/` is gitignored (`.gitignore:15`), so both probe reports vanish with the worktree, yet the required-godoc-content list names only probe-1 evidence (autocrlf, untracked deletion, non-atomicity, credential helper) and omits `EnableDotGitCommonDir`, the `DetectDotGit` ban, `KeepDescriptors`, `Reindex`, and the `worktreeconfig` incompatibility — the findings the discussion itself calls the single most important implementation detail.
**Fix:** Add the worktree-probe findings to the godoc content the landing commit must write.

### [NOTE] Zero-issue-number convention left conditional
**Section:** Technical context (`selfreportengine` is one file) / Scope
**Issue:** "the zero convention should be kept only if a real code path can still produce it" is undecided, while Scope promises the output envelope does not change; `selfreportcli/cli.go:132`'s `if number != 0` gate depends on the answer.
**Fix:** Decide now whether `number` can still be omitted, and say so.

### [NOTE] gitrepo package doc's locale paragraph goes stale
**Section:** `docs-lifecycle`
**Issue:** `gitrepo/doc.go:18-27` names `CurrentSHA`'s unborn-HEAD detection as English-locale-dependent string matching; after migration that case is typed (`plumbing.ErrReferenceNotFound`), so the paragraph becomes wrong and it is not on the doc-update list.
**Fix:** Include that paragraph in the godoc rewrite the task already performs.

## Verdict

GAPS_FOUND
Six must-resolve gaps: reindex policy, CurrentBranch coverage, guard token, 401 owner, allowlist, evidence home.
MILL_REVIEW_END
