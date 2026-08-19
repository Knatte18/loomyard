# orch-review: `_mill/discussion.md` — one-repo-illusion pass

Scope: not a general implementability review — `discussion-review-fable-r0.md` and its
fix round already did that (13 BLOCKING + 6 NIT, all fixed, one owner decision
flagged). This pass re-reads the current (post-fix, still-uncommitted) `discussion.md`
through exactly one lens: **does the design still let "warp"/"weft" become visible
from outside Fabric, anywhere the fable round didn't check.**

Both findings below are new — neither appears in the fable r0 review or fix report.

## BLOCKING: weft-side conflict markers write `<source>-weft` into tracked file content

The design's own bar (`discussion.md:17-21`): "the existence of those two sides must
not be visible from the outside, ever." B6's fix (`conflicts-are-reported-as-unified-worktree-relative-paths`,
lines 207-223) closes the *path*-naming leak carefully — but the conflict *marker
content* git itself writes into the file was never addressed, and it leaks the same
information through a different channel.

**Mechanism, verified against real git** (not just discussion.md's prose):

```
$ git merge --no-commit feature-weft   # branch-name ref
<<<<<<< HEAD
main change
=======
feature change
>>>>>>> feature-weft                   # <-- branch name, verbatim, in the marker

$ git merge --no-commit <sha-of-feature-weft>   # same merge, SHA ref
>>>>>>> 91a46f84d2b2a94c7120e358258c323f20255017   # <-- no branch name
```

`git merge --squash` behaves identically (verified — same `feature-weft` label appears).

**Why this is guaranteed, not an edge case.** `MergeStart(ref string, squash bool)`
(`public-surface-shapes`, line 327) is called on the weft side with
`ref = WeftBranchName(source)` = `"<source>-weft"` — a branch name, per
`weft-source-is-derived-and-must-exist` (line 290). Any weft-side conflict therefore
writes `>>>>>>> <source>-weft` into the conflicted file.

And per B6's own fix, **every conflict under a wired path is guaranteed to be
weft-origin**: wired names are excluded from the warp checkout's index via
`.git/info/exclude` (confirmed live — `internal/fabricengine/junction.go:660`
`seedGitExclude`, `internal/fabricengine/gitexclude.go`), so warp-side merges cannot
conflict there at all (this is exactly B6's own no-collision argument, line 218). So
this isn't a rare corner case — it is the label on **every conflict marker the
resolving agent will ever open**, since wired-path conflicts are the only kind the
document says Finalize resolves as "an ordinary git conflict" (`finalize.md:18`).
Concretely: Finalize (or the LLM it spawns) opens a conflicted file through the
junction and reads `>>>>>>> <slug>-weft` — the internal branch-naming scheme, verbatim,
inside content that crossed the Vocabulary Invariant's boundary.

**Fix, minimal and consistent with the design's own methodology.** Resolve the merged
ref to its commit SHA before calling `MergeStart`, and merge the SHA instead of the
branch name. `gitrepo.Repo` already has `CurrentSHA` (cited in Technical context,
line 509) so the primitive to do this exists. Do it uniformly on **both** sides, not
just weft: if only weft resolves to a SHA-label and warp keeps its branch-name label
(`<source>`, already known to the caller so not itself a leak), the *style* of the
marker becomes an observable tell distinguishing "warp conflicted" from "weft
conflicted" — exactly the kind of asymmetry `no-new-commit-until-both-sides-are-clean`'s
settling test (line 23) and `mutation-recording-stays-scenario-symmetric` (line 421)
already exist to rule out elsewhere in this same document. SHA-resolving both sides
keeps the marker style identical regardless of which side conflicted, and costs one
extra `CurrentSHA` call per side before `MergeStart`.

This needs a decision entry of its own (or a addendum to
`conflicts-are-reported-as-unified-worktree-relative-paths`) and a test row: assert a
weft-only conflict's marker content contains no `-weft`-suffixed name.

## NIT: guard-failure reason strings are a "closed set" that is never actually closed

`safety-guards-are-aggregated-and-side-free` (line 243) and `MergeGuardError`
(line 372-374) both promise a "closed set" of "fixed reason strings" that are
"never per-side, never path-bearing, never order-revealing." Unlike the error
*types* — which `public-surface-shapes` enumerates verbatim in Go (lines 314-392) —
the actual reason-string literals are never listed anywhere as a closed enum. Two
are quoted inline (`cannot merge %q: not a fabric-managed branch`, line 291;
"unresolved conflicts remain", line 280) but the rest ("no merge in progress",
"target worktree dirty", "target not synced to upstream", etc.) are described only
by what precondition they cover, not by literal text. Nothing stops a plan-time or
implementation-time author from phrasing one of the unlisted reasons in a way that
leaks a side ("weft worktree dirty" instead of a side-free "worktree dirty") — the
exact failure mode `safety-guards-are-aggregated-and-side-free` exists to prevent,
just pushed one level down from decided-here to decided-later. Recommend the same
treatment `public-surface-shapes` gave the error types: pin the full reason-string
set verbatim in this document (or explicitly defer it to plan-time with a note that
every string must pass the vocabulary enforcement test), not left implicit.

## Verified accurate (spot-checked, not just trusted)

- `WiredNames`/`RepoWiredNames` (`internal/fabricengine/junctionnames.go:271,287`) —
  confirmed config-driven, hub-wide, not per-pair — stable across a merge, no
  time-of-check/time-of-use risk for B6's mapping rule.
- Warp-side `.git/info/exclude` seeding for wired names —
  `internal/fabricengine/junction.go:660` (`seedGitExclude`),
  `internal/fabricengine/gitexclude.go` — confirmed live, backs B6's no-collision
  argument.
- `internal/gitexec.Run` captures stdout/stderr into buffers
  (`internal/gitexec/gitexec.go:92-93`), never streams to the terminal — confirmed
  this closes off a *different* potential leak vector (raw git CLI output reaching
  the user), which is not the one this review flags. The conflict-marker leak above
  is a content-in-file issue, orthogonal to output-capture and unaffected by it.
