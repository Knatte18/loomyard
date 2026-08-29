MILL_REVIEW_BEGIN
# Review: prowler: collapse github-repo-explorer's truncation-fallback tree-walk into one script call

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude Opus 5 (opus-class, 2026)
reviewed_file: _mill/discussion.md
date: 2026-08-29
```

## Findings

### [BLOCKING:consistency] Streaming emit contradicts "empty stdout on failure"
**Section:** "Truncated fallback" step 2 + "Strict stdout discipline" + "Traversal-order output, no post-hoc sort"
**Issue:** The fallback emits the root's blobs "immediately" and the no-sort rationale explicitly rejects "buffering the whole listing", yet the error contract requires a failed run to emit "nothing on stdout" and the Testing section asserts "empty stdout" for the mid-walk `gh api` failure — after root blobs and earlier subtrees have already been printed, stdout cannot be empty.
**Fix:** Decide one way — either buffer all paths until the walk completes successfully (and drop the anti-buffering rationale from the sort decision), or relax the error contract and the mid-walk test to allow already-emitted lines plus a non-zero exit (and say how a caller distinguishes that from a complete list).

### [NIT:consistency] Root has no `{sha}` for the non-recursive re-fetch
**Section:** "Truncated fallback" step 1
**Issue:** The rule is stated as `git/trees/{sha}` uniformly "at every depth, root included", but the root's own sha is never captured — the `--jq` stream emits only `#trunc` plus entry lines, not the response's top-level `.sha`.
**Fix:** State that the root's re-fetch uses the ref it was originally addressed by (`HEAD` or `HEAD:<path>`) and only descendant nodes use `{sha}`.

### [NIT:scope] URL-encoding reject set narrower than the stated support envelope
**Section:** "`[path]` is a tree-ish suffix" → Normalization
**Issue:** The rule's parenthetical enumerates only `space`, `#`, `?`, `%`, while the surrounding text says support is "verified only against slash-separated ASCII paths" — a non-ASCII path falls outside the verified envelope but inside the accepted set.
**Fix:** Say whether the parenthetical is exhaustive or illustrative, and state the disposition of non-ASCII path bytes (reject up front, or pass through and rely on the API error).

## Verdict

REQUEST_CHANGES
Streaming output and the empty-stdout-on-failure contract are mutually exclusive as written.
MILL_REVIEW_END
