MILL_REVIEW_BEGIN
# Review: fabric: accumulate the result envelope from mutations, not control flow (slice 14)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude (Opus-class model, Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [BLOCKING:design] Omission direction fails on git-side diff changes
**Section:** `permitted-roots-and-the-oracle` — "every change in the unfiltered diff must be covered by some record entry"
**Issue:** The manifest deliberately records `<prime>/.git/worktrees/<name>` (`manifest.go:184-213`; `verbs.go:393,403` declare exactly those paths as *permitted roots* today), and the unfiltered honesty pass unmasks them — yet no record entry's `Target` is at or above them: `worktree_created`/`worktree_removed` name the worktree path, not the prime repo's admin entry. Every `Add`/`Remove` cell would report a false lie of omission on correct behaviour.
**Fix:** State how git-admin bookkeeping paths are covered — an explicit exemption class in the omission direction, or a rule tying a `worktree_*` entry to its `.git/worktrees/<name>` sibling.

### [BLOCKING:design] No Kind for a worktree branch switch; checkout/pull diffs uncovered
**Section:** `mutation-entry-shape` Kind table + omission direction
**Issue:** `Checkout` runs `git switch` in both worktrees (`checkout.go:82-91,131-148`), rewriting tracked working-tree files; `Pull` advances warp likewise. Those produce `ChangeContentChanged`/`ChangeAdded`/`ChangeRemoved` entries across the worktree with no manifest-observable record entry covering them — the thirteen-member enum has no branch-switch or working-tree-rewrite kind, and the git-state split exempts entries from *commission* only, never diff changes from *omission*.
**Fix:** Either add a kind whose `Target` is the switched worktree root, or state the symmetric omission exemption for git-driven working-tree content changes.

### [NIT:scope] Harness cannot see the record: `VerbCase.Run` returns only `error`
**Demoted-from:** BLOCKING
**Section:** `fabrictest-truthfulness-oracle` / Testing
**Issue:** `VerbCase.Run func(tb, h, f) error` (`fabrictest/verbs.go:195-201`) discards the typed result, and the twelve verbs return twelve heterogeneous result types — so "every matrix cell cross-checks the result envelope's record" has no stated plumbing, contradicting the claim that the oracle "costs nothing to apply everywhere".
**Fix:** Decide the seam — e.g. `Run` returning `(fabricengine.Mutations, error)` plus a common accessor (embedded record type or `Mutated() Mutations`) on every result type — and say so.

### [NIT:consistency] Is `mutations`/`partial` always present on success?
**Section:** Testing — `internal/fabriccli` envelope shape
**Issue:** The success bullet allows "`partial` absent or false" while the empty-record failure bullet requires `partial` false present; the key set is elsewhere described as fixed across verbs.
**Fix:** State one rule — always emit `partial`, or emit only when true — and align both assertions.

### [NIT:decision] New `internal/output` function unnamed
**Section:** Scope / `ok-semantics-and-error-path-fields`
**Issue:** Every other artifact is named precisely (`mutation.go`, `type Check string`), but the additive error-path function has no name or signature, leaving `Err`-vs-new-function precedence for field/`error` key collisions unstated.
**Fix:** Name it and give its signature, plus what happens if `fields` carries `ok` or `error`.

## Verdict

REQUEST_CHANGES
Cross-check's omission direction and harness record plumbing are unsound as specified.
MILL_REVIEW_END
