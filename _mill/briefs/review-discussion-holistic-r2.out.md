MILL_REVIEW_BEGIN
# Review: fabric: collapse external API surface onto Commit — stop leaking warp/weft

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewer_self_id: Claude Opus 4.8
reviewed_file: _mill/discussion.md
date: 2026-08-02
```

## Findings

### [GAP] RelPath-scoped pathspec breaks Commit's relPath="." classify
**Section:** Scope (In, CLI migration) / Technical context (`Fabric.Commit`)
**Issue:** The three CLIs pass `ScopedPathspec(layout.RelPath, [_lyx])` = `<RelPath>/_lyx` (buildercli/weft.go:80, perchcli/run.go:423, webstercli), but `Commit` classifies via `classifyPaths(".", …)` (commit.go:136, relpath-is-dot-for-slice-2) — so at a nested worktree (RelPath ≥ 1 segment, a real case: perchcli/cli_integration_test.go:98, and the whole CONSTRAINTS "Anchored exclusions" bullet is about `RelPath` ≥ 2 for exactly these callers) the weft entry's first segment is `<RelPath>`, not `_lyx`, and misroutes to the warp side.
**Fix:** Decide how migrated callers spell weft paths for `Commit` at non-root RelPath (pass root-relative `_lyx/…`, or have `Commit` consume `l.RelPath` it already resolves), and mandate nested-RelPath integration coverage — current commit_integration_test.go only exercises RelPath=".".

### [NOTE] Perch exclude-magic resolution left as two unchosen options
**Section:** Technical context (KEY IMPLEMENTATION RISK) / Q&A
**Issue:** `perchcli/run.go:424`'s `:(exclude)*.lock` magic entry has no path prefix for `Commit`'s classifier; the discussion offers candidates (a) tolerate pathspec magic and (b) deepen the git-exclude backstop to `**/_lyx/*/**/*.lock`, but picks neither — and this interacts with the RelPath GAP above (option b's base and pattern depth shift with any re-spelling).
**Fix:** Choose (a) or (b) in the discussion; if (b), confirm the deepened pattern does not over-broaden the cross-module exclude invariant (CONSTRAINTS "Cross-module exclusions").

### [NOTE] `lyx fabric status` output-shape change vs sandbox F3
**Section:** Decisions (dead-methods-diff-status-kept) / Testing
**Issue:** Replacing `StatusWeft` (branch/dirty/ahead/behind `map`) with `Fabric.Status` (`[]ChangeEntry`) changes observable CLI output; sandbox F3 (SANDBOX-FABRIC-SUITE.md:115) drives `fabric status` and the Testing section names only "output-envelope shape," not the scenario or any operator-facing doc.
**Fix:** Note that F3's qualitative check still holds and confirm no scripted consumer relies on the dropped branch/ahead/behind fields; update the suite watch-text if needed in-commit.

## Verdict

GAPS_FOUND
One migration GAP: RelPath-scoped weft pathspecs misclassify under Commit's fixed relPath=".".
MILL_REVIEW_END
