MILL_REVIEW_BEGIN
# Review: fabric: unify warp + weft into one git-coordination module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] SyncWeft: detached push vs post-push SHA re-read conflict
**Section:** Decisions § SyncWeft; Q&A "SyncWeft semantics"
**Issue:** The decision requires both "the CLI-level detached push spawn (commit + detached, un-waited push, as weftcli spawn.go does today)" and recording correspondence from a weft SHA "re-read via CurrentSHA after a successful push" — but a fire-and-forget detached push (verified: `weftcli/spawn.go` starts `lyx weft push` without `Wait`) means the committing process never sees push success or the rebase-rewritten SHA, and the detached push process has no warp SHA to record. The design's `SyncWeft` (line 62) instead pushes in-process; the two are contradictory.
**Fix:** State explicitly whether `SyncWeft` pushes in-process (enabling post-push `CurrentSHA` re-read + `RecordCorrespondence`) or the CLI spawns a detached push, and name which process/layer performs `RecordCorrespondence` and the post-push re-read.

### [NOTE] BranchPrefix composition with the `-weft` suffix unspecified
**Section:** Decisions § Branch naming / § Config; Testing (branch-name derivation)
**Issue:** fabric.yaml retains a `branch_prefix` (warp's default is empty → branch==slug, verified), but the derivation spec (`<slug>` / `<slug>-weft`, `main` / `main-weft`) never says how a non-empty prefix composes with `-weft` (e.g. `hanf/<slug>-weft` vs suffix-on-slug) or whether the primary branch is ever prefixed.
**Fix:** Specify the prefix+slug+suffix composition and primary-branch handling so the derivation TDD tests are unambiguous for a non-empty prefix.

### [NOTE] Correspondence-index location: "gitignored" vs "git-metadata area"
**Section:** Decisions § Correspondence index
**Issue:** The index is described as both a "gitignored local cache" and in "the weft clone's git-metadata area"; a file inside the gitdir needs no gitignore, while a worktree file needs an entry (and who writes it) — the two phrasings pick different locations.
**Fix:** Pin one location (gitdir path vs worktree + gitignore entry, following `internal/state`'s path/lock pattern).

## Verdict

GAPS_FOUND
One sequencing gap in SyncWeft push/correspondence recording must be resolved before planning.
MILL_REVIEW_END
