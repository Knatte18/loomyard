MILL_REVIEW_BEGIN
# Review: Add cross-repo code search to prowler

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [BLOCKING:design] Stub fixture dispatch key is unspecified
**Section:** Decisions → "Separate stub `gh` and fixture tree"; Testing
**Issue:** The harness is specified to mirror `github-tree-selftest.sh` "exactly", but that stub keys `map.tsv` on the endpoint string alone (`field1 = endpoint`, `testdata/github-tree/bin/gh` lines 44–62); every search call in a multi-repo sweep hits the identical endpoint `search/code`, so scenarios needing per-repo bodies (multi-repo ordering, `incomplete_results` on repo 2, 403 mid-sweep on repo 2 of 3, per-repo `total_count` note) cannot be expressed by an endpoint-keyed map.
**Fix:** State the new stub's dispatch key — e.g. endpoint plus the `q=` parameter value, or call-sequence index — so the plan writer does not silently inherit a keying scheme that cannot distinguish the scenarios listed.

### [BLOCKING:consistency] Exit-2 claim contradicts `github-tree.sh`
**Section:** Testing → "Argument rejection, all before any network call"
**Issue:** The discussion lists an invalid `<owner>/<repo>` ref among the cases exiting 2 "matching `github-tree.sh`'s convention", but `github-tree.sh` exits 2 only for the usage-shape error (arg count, line 41–44) and exits 1 via `die` for an invalid ref (line 49–51); the sibling convention is therefore shape→2, semantic-invalidity→1.
**Fix:** Say explicitly which exit code each of the five rejection cases uses, and drop or correct the "matching `github-tree.sh`'s convention" premise.

### [BLOCKING:decision] SKILL.md frontmatter and skills/INDEX.md have no disposition
**Section:** Scope → In; Constraints → docs in same commit
**Issue:** `skills/github-repo-explorer/SKILL.md`'s frontmatter (`description: Browse a GitHub repo's file tree and read files via the gh CLI…`, `argument-hint: "<owner/repo> [path] [question]"`) is the skill's dispatch surface and names only tree+read; `plugins/prowler/skills/INDEX.md` line 6 duplicates that description verbatim. Neither is named in the In list or the Out list.
**Fix:** State whether the frontmatter description/argument-hint and `INDEX.md`'s row are updated for the search capability, or deliberately left alone with a reason.

### [NIT:scope] Duplicate repo refs unaddressed
**Section:** Decisions → "Hard cap of 10 repos"; Testing → argument rejection
**Issue:** Nothing says what happens when the same `<owner>/<repo>` appears twice in the ref list — it would burn two of the ten `code_search` calls and emit duplicate records.
**Fix:** State the disposition (dedupe silently, reject as usage error, or accept as-is) so the harness matrix is determinate.

## Verdict

REQUEST_CHANGES
Three blocking gaps: stub fixture keying, exit-code contract, skill-frontmatter disposition.
MILL_REVIEW_END
