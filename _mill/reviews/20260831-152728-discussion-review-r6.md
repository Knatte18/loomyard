MILL_REVIEW_BEGIN
# Review: Prefer raw fetch, scope large tree listings

```yaml
duration_s: 114.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [NIT:design] Fallback probe premise unstated for >1MB files
**Section:** Decisions → "A path naming a directory is a hard error" **Issue:** The type probe is a default-media-type `contents` call whose response "carries base64 content for a file"; that shape does not hold for files above the contents API's ~1MB JSON limit, so the probe — not the raw-Accept fetch — would fail and be diagnosed as the error, on a file the fallback could otherwise read. **Fix:** Fold this into the live-capture implementer note already required for the fallback-status Decision and record the outcome as a limitation (parity with today's `--jq .content | base64 -d`, which has the same limit) or narrow the probe accordingly.

### [NIT:consistency] Arg-count rule collides with the `--` terminator
**Section:** Testing → `github-read-selftest.sh` ("Wrong argument count (zero, one, three)") vs Decision "takes no ref argument" **Issue:** `github-read.sh` accepts a `--` terminator, so a `--`-prefixed path is a legitimate three-token invocation, yet the testing bullet declares three arguments a usage error. **Fix:** State that the count is checked after the terminator is consumed, so the two bullets do not assert opposite outcomes for the same argv.

### [NIT:scope] Guard message "exact wording" is asserted but never specified
**Section:** Decision "Guard abort is a normal `die`" **Issue:** The decision says the harness asserts the exact wording of both guard-abort variants, but no wording is given anywhere in the discussion — only content requirements (names the ceiling, names the remedies, names `--max-entries`). **Fix:** Either state the two message strings, or say the assertions are substring/content-level, so the plan writer is not pinned to text that does not exist.

## Verdict

APPROVE
Decisions are complete and source-grounded; the three nits are wording and premise-pinning only.
MILL_REVIEW_END
