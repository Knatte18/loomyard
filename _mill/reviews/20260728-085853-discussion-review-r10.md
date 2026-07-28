MILL_REVIEW_BEGIN
# Review: native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github

```yaml
verdict: GAPS_FOUND
reviewer_model: sonnet
reviewed_file: _mill/discussion.md
date: 2026-07-28
```

## Findings

### [GAP] Reindex epoch tracks one *Repo instance, not the shared physical checkout
**Section:** linked-worktree-and-interop-evidence (reactive reindex/epoch policy)
**Issue:** `runEpoch` bumps only on `r.run` calls made through *this* `*Repo` value, but the same physical checkout is routinely addressed by multiple independent `Repo` instances in this codebase — `Fabric.Warp`/`Weft` cached for the `Repo`'s lifetime (`fabric.go:66-67`) alongside fresh, separate instances from `PushWeftAt` (`weftgit.go:328`, explicitly documented as usable with "no Fabric instance ... involved"), `websterengine` (`gitwrap.go:27`, `runlevel.go:845`), and `fabriccli.spawnPush`'s wholly separate detached OS process (`spawn.go:21-37`). A packfile-invalidating write made through any of those other instances/processes (a fetch, or `gc --auto` after an ordinary commit — the discussion's own cited evidence) never bumps a long-lived cached handle's own epoch, so that handle's `object not found` is "returned as truth" with no reindex. For `isStrictDescendant` this silently reproduces the exact `SetSnapshotSHA` adopt-loop silent-drop bug this same decision names as "the precise bug the adopt-on-conflict loop exists to prevent" — just triggered by a different instance's write rather than this one's own `r.run`.
**Fix:** State explicitly whether one physical checkout can be addressed by more than one live `Repo`/process concurrently (the cited call sites say yes), and if so either extend the reindex trigger beyond this instance's own call history (e.g. a cheap on-disk staleness check) or record the residual risk and why the swallowing-method fallback is acceptable despite it.

### [NOTE] Two source citations are off by one line
**Section:** Scope (selfreportcli rewrite list); constraints-github-auth-invariant
**Issue:** `buildCreateArgs` is named in the comment on `cli.go:95`, not the cited line 96. Separately, `pathresolve_guard_test.go:30` is cited as banning `exec.Command("lyx"`, but that token is actually on line 29 — line 30 is the already-dead `exec.CommandContext("lyx"` entry the same sentence goes on to describe as unmatchable.
**Fix:** Correct both line numbers before this text is carried into the plan or PR description.

## Verdict

GAPS_FOUND
One unresolved cross-instance cache-staleness gap in the reindex design; two trivial line-citation fixes.
MILL_REVIEW_END
