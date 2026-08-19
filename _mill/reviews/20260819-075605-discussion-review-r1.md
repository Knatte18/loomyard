MILL_REVIEW_BEGIN
# Review: fabric: merge-conflict primitive

```yaml
duration_s: 169.0
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus-class model (environment reports claude-opus-5)
reviewed_file: _mill/discussion.md
date: 2026-08-19
```

## Findings

### [BLOCKING:design] merge-tree/commit-tree/update-ref have no declared home
**Section:** §merge-needs-no-target-worktree vs §public-surface-shapes vs §Constraints (gitrepo Client Boundary)
**Issue:** `Merge`'s three object-database commands are the only git plumbing in the design with no owning API: the "verbatim" gitrepo surface declares only `MergeStart`/`MergeConclude`/`ConflictedFiles`/`MergeHeadPresent`, while `git-primitive-in-gitrepo-coordination-in-fabricengine` puts git-level primitives in `gitrepo` and the Constraints section pins only those three onto `cmd/lyx/gitrepoboundary_test.go`'s method list — verified: that list is set-equality per CONSTRAINTS.md, so an undeclared new `runChecked` method fails the guard.
**Fix:** State normatively whether `MergeTree`/`CommitTree`/`UpdateRef` land as `gitrepo.Repo` methods (and therefore on the pinned list, in this commit) or as `fabricengine` `gitexec` call sites, and add them to the verbatim surface.

### [NIT:decision] target-side weft branch is never derived or guarded
**Demoted-from:** BLOCKING
**Section:** §weft-source-is-derived-and-must-exist, §safety-guards-are-aggregated-and-side-free
**Issue:** Only `<source>-weft` derivation and its existence refusal are decided; `Merge(target, source, …)` must also resolve `WeftBranchName(target)` and publish a weft ref, yet no decision states that derivation, and no member of the closed reason set covers an absent or non-fabric-managed target (`"source branch not found"`/`"source branch is not fabric-managed"` name the source only).
**Fix:** Add the target-side derivation and its existence guard as a named decision, with the reason string(s) added verbatim to the closed set in the same section.

### [NIT:consistency] two incompatible spellings of the not-fabric-managed reason
**Demoted-from:** BLOCKING
**Section:** §weft-source-is-derived-and-must-exist vs §safety-guards-are-aggregated-and-side-free
**Issue:** The guard section pins the closed set as *fixed* strings including `"source branch is not fabric-managed"`, but the weft-source decision gives the same refusal as `cannot merge %q: not a fabric-managed branch` — a format verb that interpolates the branch name, contradicting "fixed reason strings drawn from a closed set, never path-bearing".
**Fix:** Pick one spelling; if the branch name must appear, say where it appears (a separate error field, not the reason string) so the closed set stays literal.

### [NIT:design] `partial` semantics on the conflict path unstated
**Section:** §public-surface-shapes, §Constraints (Mutation Record Invariant)
**Issue:** Conflicts return `(MergeResult{Conflicts…}, nil)` yet the CLI emits a failure envelope with exit 1; with a fast-forwarded or staged side the record is non-empty while `error == nil`, so `partial` is `false` on an envelope that reports failure — a plan writer could reasonably set it `true`.
**Fix:** State explicitly that `partial` stays `false` on the conflict path because the invariant's rule is `error ≠ nil ∧ record non-empty`.

### [NIT:consistency] `merge --continue`/`--abort` arity contradicts "every branch argument is required"
**Section:** §cli-mirrors-git, §Testing (CLI)
**Issue:** `merge` is specified with two required positional branch args, but `--continue`/`--abort` take none, and the testing plan asks for an arity test asserting "branch argument required on both verbs" without naming the mode split.
**Fix:** State the mode-dependent arity rule (zero positionals with `--continue`/`--abort`, exactly two otherwise) so the cobra `Args` validator is decided here.

### [NIT:design] the asserted-missing weft ownership kind may already exist
**Section:** §weft-side-gated-reset-in-destroy-dot-go
**Issue:** `destroy.go`'s `resetHardTo(rec, req, repo, sha)` is already repo-generic (only `Fabric.ResetHard` hardcodes `f.warp`, `ownedWarpCheckout`, `force:false`), and `ownedRegisteredLinkedWorktree(repoDir)` covers linked worktrees — so "a new unexported executor" plus a new ownership kind is asserted rather than established.
**Fix:** Reduce the decision to what is actually new (an abort-specific `pathRequest` with `force:true`, and a weft ownership kind only if the existing linked-worktree kind is shown insufficient).

### [NIT:decision] no stated behaviour below the git 2.38 floor
**Section:** §Technical context (git version floor)
**Issue:** `git merge-tree --write-tree` carries a hard floor and the repo documents no minimum git version anywhere else (checked: only `internal/gitrepo/doc.go`, which pins go-git, not git); the design says the dev box runs 2.53 but never says what an older git produces — today a raw, unowned git usage error.
**Fix:** Decide whether `Merge` probes the version and returns a typed refusal, or the floor is declared unchecked and recorded in `internal/gitrepo/doc.go`.

### [NIT:design] foreign merge state still reaches `Fabric.Commit` raw
**Section:** §combined-lock-around-mutating-steps-only
**Issue:** The new `Commit` guard fires only when the fabric record exists, but the design elsewhere accepts that a human's plain-git merge can leave `MERGE_HEAD` on the warp side — that case still yields git's raw "cannot do a partial commit during a merge", the exact outcome the guard exists to prevent.
**Fix:** Say whether `Commit` also refuses on foreign git merge state, or state deliberately that it does not and why.

## Verdict

REQUEST_CHANGES
Three decision gaps in the Merge surface must be closed before plan writing.
_Note: 2 finding(s) demoted from BLOCKING to NIT by the stage's blocking-class ceiling; current blocking_count is 1._
MILL_REVIEW_END
