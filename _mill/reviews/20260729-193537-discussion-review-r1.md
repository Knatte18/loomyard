MILL_REVIEW_BEGIN
# Review: fabric: Fabric.Commit classify+dispatch + unified diff/status

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: claude-opus-4-8 (Claude Opus 4.8)
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [GAP] Diff weft-anchor uses exact, not nearest-older
**Section:** §Decisions / unified-diff-status-warp-anchor
**Issue:** The decision bridges via `WeftSHAForWarpSHA(sinceWarpSHA)`, which is `exact()`-only (index.go:148); but the `warp-only-commit-is-plain-git` decision makes warp-only commits (no correspondence entry) legitimate and common, so any non-anchor warp SHA — most warp commits in a warp-heavy or collaborator workflow — misses and silently drops real weft changes in that range, contradicting the "no-correspondence anchor is rare (older-than-first-weft / pre-lyx)" framing. `corrindex.go`'s `nearestAtOrBefore` and `ErrNoCorrespondence`'s own "exact or nearest-older" doc suggest nearest-older is the intended semantics.
**Fix:** Specify that Diff's weft anchor derives from the nearest-older weft correspondence (not exact), or state explicitly why exact is correct and re-scope the "degrade to warp-only" case to how often it truly fires.

### [NOTE] Auto-pushing warp weakens "warp stays ordinary git"
**Section:** §Decisions / async-push-both-sides-detached vs warp-only-commit-is-plain-git
**Issue:** Firing an async push of the host/warp repo on every `Fabric.Commit` is an action a hand-run `git commit` never takes; for the "everything that writes files" caller this routinely pushes host work to a shared remote non-lyx collaborators use, a tension the decision notes only for the failure (non-ff) case, not for desirability.
**Fix:** Note whether warp auto-push is gated/conditional (e.g. only when an upstream/lyx-owned branch is configured) or accept the consequence explicitly for the collaborator-repo case.

### [NOTE] Snapshot-tag content is unvalidated
**Section:** §Decisions / snapshot-trailer-written-now + classification-input-contract
**Issue:** The classifier "trusts its caller," and snapshot tags are written verbatim as `Snapshot: <tag>` trailers; a tag containing a newline or colon could inject/corrupt trailer lines (including a spoofed `Warp-SHA`) that slice 3's reader parses back.
**Fix:** State the tag charset/format contract (or that tags are trusted Go-internal input in slice 2) so the open-item on trailer format also pins tag-value constraints.

## Verdict

GAPS_FOUND
One gap: the Diff weft-anchor's exact-match bridge drops weft changes for common non-anchor warp SHAs.
MILL_REVIEW_END
