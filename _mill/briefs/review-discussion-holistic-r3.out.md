MILL_REVIEW_BEGIN
# Review: Prefer raw fetch, scope large tree listings

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [BLOCKING:design] Fallback status extraction is unspecified
**Section:** Decision "reports the `gh api` failure, not the raw failure" **Issue:** The discussion promises a 401/403/404/other diagnosis "in the same style as `github-tree.sh`", but `github-tree.sh:124-151` derives that status by regexing the *stdout* of `gh api ... --jq <expr>` (`"status": "404"`), and the fallback here has no `--jq` and sends its stdout into the temp file that must never be emitted on failure — so the technique does not transfer and no replacement (parse the temp file, capture stderr, or `gh api -i`) is chosen. **Fix:** Decide and state where the fallback's HTTP status comes from, and note that the harness stub can only reproduce whatever shape is chosen, so the choice must be pinned against real `gh` behaviour under `Accept: application/vnd.github.raw`.

### [NIT:consistency] Guard message suggests `--children` while in `--children`
**Section:** Decisions "Guard abort is a normal `die`" + "The guard applies to every mode" **Issue:** The single stderr line is specified to name "both remedies (scope to a subdirectory, or use `--children`)", but in `--children` mode one remedy is already in effect, and the testing item "Guard fires in `--children` mode" asserts nothing about the wording. **Fix:** State whether the message is mode-invariant or mode-aware, and assert it in the `--children` guard scenario.

### [NIT:decision] `<owner/repo>` validation has no stated disposition
**Section:** Decision "Path validation is duplicated in `github-read.sh`" **Issue:** The Decision covers only path normalisation/character validation, yet the testing section asserts "Malformed `<owner/repo>`: rejected locally"; `github-tree.sh:49` implements that check with the `[[ =~ ^[A-Za-z0-9._-]+/… ]]` bracket-range form the adjacent comment condemns for path validation. **Fix:** Say explicitly that the repo-slug check is copied too, and whether it is copied verbatim (accepting the known collation looseness) or tightened.

### [NIT:scope] Trailing-slash convention not named as a SKILL.md deliverable
**Section:** Scope, SKILL.md bullet **Issue:** The SKILL.md change is scoped to read-order plus "guidance on choosing between `--children`, scoped-recursive, and whole-repo listing"; the trailing-`/` directory marker — which a calling agent must know to interpret `--children` output — is not listed as documented anywhere the caller reads. **Fix:** Add the output-convention sentence to the SKILL.md deliverable alongside the README section.

## Verdict

REQUEST_CHANGES
Fallback HTTP-status extraction for `github-read.sh` is unspecified and not portable from `github-tree.sh`.
MILL_REVIEW_END
