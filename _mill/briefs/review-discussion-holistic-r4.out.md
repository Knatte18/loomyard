MILL_REVIEW_BEGIN
# Review: fabric: unify warp + weft into one git-coordination module

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] RevertWithWeft needs a reset primitive with no home named
**Section:** Decisions/RevertWithWeft; Decisions/Most git mechanics grow into gitrepo
**Issue:** RevertWithWeft "resets both repos" but `internal/gitrepo` has no reset method (only CurrentSHA/StageAndCommit/SHAExists/ChangedFilesSince/Push/PushCoalesced/Snapshot*), and the gitrepo-growth list (pull, pathspec staging, lock commit) omits reset — so where the git reset lives is unspecified.
**Fix:** Name the reset primitive and its layer (a new `gitrepo.Repo` reset method vs a fabric-level reset on `gitexec`), consistent with the "generic ops go to gitrepo" rule.

### [GAP] Sandbox clone parity needs a board/wiki repo that isn't provisioned
**Section:** Decisions/Clone: full parity, board repo included; Testing/Sandbox
**Issue:** `clone` replicates CloneHub, which clones a third board repo derived as `<weftURL>.wiki.git` (deriveBoardURL, verified), but Testing provisions only host `lyx-fabric-test` and weft `lyx-fabric-test-weft` — a `lyx fabric clone` sandbox scenario would abort cloning a nonexistent `lyx-fabric-test-weft.wiki.git`.
**Fix:** State how board is supplied for the fabric sandbox clone scenario (initialize the wiki for the weft test repo, or pass an explicit board URL).

### [NOTE] "Pathspec-scoped staging" already exists in gitrepo
**Section:** Scope (In); Decisions/Most git mechanics grow into gitrepo
**Issue:** The scope lists pathspec-scoped staging as a gitrepo gap, but `gitrepo.StageAndCommit` already stages an explicit pathspec-scoped file list; the genuinely-new additions are fast-forward `Pull` and a write-lock-serialized commit.
**Fix:** Reword so the plan writer builds the write-lock wrapper + Pull rather than re-implementing staging that already ships.

## Verdict

GAPS_FOUND
Reset primitive location and sandbox board/wiki provisioning are unresolved.
MILL_REVIEW_END
