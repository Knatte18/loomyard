MILL_REVIEW_BEGIN
# Review: prowler: collapse github-repo-explorer's truncation-fallback tree-walk into one script call

```yaml
verdict: APPROVE
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-29
```

## Findings

### [NIT:design] 404-vs-422 discrimination mechanism unstated
**Section:** `### [path]` is a tree-ish suffix / Strict stdout discipline **Issue:** The design commits to reporting `path` 404 and 422 as distinct stderr messages, but the only signal available from `gh api` is its own stderr text (`gh: Not Found (HTTP 404)`), so the script must string-match a message format coupled to gh's version and locale — and the stub `gh` decision confirms the only thing a failure produces is a canned stderr line plus a non-zero exit. **Fix:** Name the discrimination mechanism (stderr capture plus an `HTTP <code>` match, or an explicit status probe) so the plan writer does not invent one and the fixtures' canned stderr lines match it.

### [NIT:scope] `[path]` normalization/encoding undefined
**Section:** `### [path]` is a tree-ish suffix **Issue:** The path is concatenated onto the endpoint as `git/trees/HEAD:<path>`, but the discussion never says whether leading/trailing slashes are stripped or whether a path containing a space or other URL-significant character is encoded; verified only against slash-separated ASCII paths (`drivers/net/ethernet`). **Fix:** State the normalization rule (strip surrounding slashes; whether such a path is rejected up front or left to surface as a loud `gh` error) so the harness can pin it.

### [NIT:consistency] "one call" claim omits the `gh auth status` preflight
**Section:** `HEAD` as the ref / Testing (fast-path assertion) **Issue:** The unscoped-untruncated case is stated as "exactly **one** call" and the harness asserts one `gh api` call, but the design also runs a `gh auth status` preflight (the stub answers it), and `gh auth status` itself validates the token against the API — so the rate-limit-budget framing is one round trip short. **Fix:** Qualify the claim as one `gh api` tree call and note the preflight round trip where the one-call property is documented in `README.md`.

### [NIT:design] Empty/commitless repo maps onto the repo-404 message
**Section:** Strict stdout discipline and fail-fast errors **Issue:** `404 on the unscoped fetch` is bound to a single "repo not found" diagnostic, but a repository that exists with no commits also 404s on `git/trees/HEAD`, so a valid empty repo would be reported as nonexistent; the success-with-zero-blobs case (exit status, empty stdout) is likewise unstated. **Fix:** Say whether the 404 message is worded to cover both causes, and state that a legitimately blob-free listing exits 0 with empty stdout.

## Verdict

APPROVE
Scope, decisions, failure modes and test scenarios are all pinned; only edge-case wording gaps remain.
MILL_REVIEW_END
