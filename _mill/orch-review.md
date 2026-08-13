# Orchestrator review: gitexec-checked-entry-point discussion.md

Reviewed against the task description (`status.md`: "gitexec: add the checked entry point and migrate the call sites") and against the current `main` codebase.
Phase is `discussing`, no plan yet.

## Verdict

Not ready to move to `mill-plan` without closing one gap. Everything else checked — including a large set of exact numeric re-derivations (61 production sites, 57 in `fabricengine`, 21 `r.run` sites, 4 full-discards, 34 bare-exit-code messages, 7 `wrapProbeError` call paths) — matched the live tree precisely. The one finding below is concrete and would surface as a test failure during `/mill-go` rather than a silent regression, but it's cheap to fix now and expensive to discover mid-migration on an ~80-site branch.

## Finding: a fourth content-sniff site is missing from the inventory, and it guards a tested, production-consumed sentinel

The `content-sniff-sites-take-the-checked-form` decision lists three sites, explicitly noting the design doc's grep found only two and this discussion re-derived a third (`gitrepo/push.go`'s `containsAny(stderr, rebaseRetryTriggers)` inside `pushWithRebaseRetry`, missed because it goes through the `containsAny` helper rather than `strings.Contains` directly).

That same `containsAny(stderr, rebaseRetryTriggers)` pattern also appears at **`internal/gitrepo/push.go:92`, inside `PushRebaseFree`** — a separate function, not the one already named:

```go
// PushRebaseFree, push.go:84-96
_, stderr, code, err := r.run("-c", "push.autoSetupRemote=true", "push")
if err != nil { return err }
if code == 0 { return nil }
if containsAny(stderr, rebaseRetryTriggers) {
    return ErrPushRejected
}
return fmt.Errorf("gitrepo: git push: %s", stderr)
```

This is load-bearing, not incidental:

- `ErrPushRejected` is a real sentinel consumed in production at `internal/fabricengine/coalesce.go:65` (`errors.Is(err, gitrepo.ErrPushRejected)`), and pinned by a dedicated test, `internal/gitrepo/push_test.go:272` `TestPushRebaseFree_DivergedRemote_ReturnsErrPushRejected`, which asserts `errors.Is(err, gitrepo.ErrPushRejected)` on a diverged remote.
- The 21-site `gitrepo` disposition table lists `PushRebaseFree (push.go) | 1 | checked` with no sniff or `errors.As` annotation — the same shorthand used for a plain two-message merge site. Read at face value, that row tells `/mill-go` to apply the `default-merge-rule` boilerplate (checked call, `if err != nil { return fmt.Errorf(...) }`), which drops the `containsAny` sniff entirely and returns a generic wrapped error on every divergence — breaking the `errors.Is` check at `coalesce.go:65` and failing `TestPushRebaseFree_DivergedRemote_ReturnsErrPushRejected`.
- It's also a hybrid the existing decisions don't quite cover as written: it needs the **content-sniff** treatment (`errors.As(err, &gitErr)`, sniff `gitErr.Stderr`) *and* the **sentinel-sites-keep-their-sentinel** treatment (return bare `ErrPushRejected`, no `%w`/`%v` wrapping — note the current code doesn't even embed stderr in the sentinel path today, unlike the `%w: %v` shape that decision documents elsewhere). Neither decision, read alone, tells an implementer this combination is required.

**Suggested fix to the doc, not the code:** add this as the fourth content-sniff site in `content-sniff-sites-take-the-checked-form`, and annotate the `PushRebaseFree` row in the 21-site table (or add a one-line cross-reference) so the disposition isn't read as a plain checked call. The migration itself is small — `errors.As` + sniff `gitErr.Stderr`, return bare `ErrPushRejected` on a match, generic checked-form error otherwise — but it needs to be named, the same way the doc already went out of its way to name the sibling site in `pushWithRebaseRetry`.

## Verified against `main` (no other discrepancies found)

- Re-derived inventory table: 61 total production `gitexec.RunGit(` sites (57 in `fabricengine`, 1 each in `gitrepo`/`lyxcwd`/`fabriccli`/`websterengine`), 21 `r.run` sites in `gitrepo`, 4 full-discard sites (`prune.go` ×2 with `//nolint:errcheck`, `reconcile.go`, `remove.go`), 34 messages embedding a bare exit code — all exact matches to a fresh grep against `main`.
- `wrapProbeError`'s 7 call paths (4 exec-path with empty stderr, 3 exit-path with stderr) — exact match at `warpprobe.go:69,72,79,93,96,134,137`.
- `destroy.go`'s three gate executors (`removeGitWorktree`, `deleteBranch`, `createGitWorktree`) — signatures match the doc's description exactly, including `createGitWorktree`'s extra `createdToken` first return.
- `readBranch` (`reconcile.go:543`) — the prior-exit-code claim is exact, and slightly more nuanced than the doc states: *two* downstream messages (lines 563 and 567) cite the earlier `rev-parse` exit code, not one — doesn't change the decision, just confirms it's grounded in the actual code.
- All three `internal/gitrepo/doc.go` prose corrections are warranted: line 12 does say "one function", line 28 does say "roughly eighty call-sites", and line 198 does state the "no-stderr-leak style" claim the `resethard-is-not-suppression` decision says has nothing enforcing it — confirmed `reset_test.go` doesn't assert on `err.Error()`, only on the resulting SHA/file content/`ErrInvalidSHA`.
- The four sites outside `fabricengine` (`lyxcwd.go` rev-parse, `fabriccli/fabric.go` branch --show-current, `websterengine/gitwrap.go` status --porcelain) — dispositions and rationale hold up against the actual code shape at each site.
- `Pull`/`Fetch` raw-suppression claim: `pull_test.go` and `fetch_integration_test.go` do assert on `"fatal:"` substrings in `err.Error()`, consistent with "a considered decision with a test enforcing it."

## Not re-verified

- GitHub issue #145's existence/closed state — no network check performed.
- The design doc's own original 2026-08-10/11 figures (74 sites, etc.) — took the discussion's re-derived numbers as the ones to check against `main`, since the doc itself states the design doc's figures are stale and shouldn't be trusted.

## Scope check against the task description

Task description is "gitexec: add the checked entry point and migrate the call sites." The Out-of-scope list (no `RunGit` renaming, no re-litigating already-fixed message wording, no credential redaction, no locale pinning, test files exempt) stays disciplined and doesn't creep into adjacent cleanup. The three-way split between pure predicates (raw), mixed/tri-state/content-sniff (checked + `errors.As`), and two-message merges (checked, default rule) is coherent and, aside from the one missing site above, appears exhaustively re-derived from the code rather than trusted from the stale design doc — which is itself named as an explicit operating principle in the Q&A log ("do not trust doc files; trust the code"). Worth noting the doc followed its own principle rigorously enough to catch the design doc's stale `rev-parse`-probe classification and the `ResetHard` suppression myth; the `PushRebaseFree` gap above is the one place that same rigor didn't quite reach a sibling of an already-found site.
