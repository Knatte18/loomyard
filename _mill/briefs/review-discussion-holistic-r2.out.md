MILL_REVIEW_BEGIN
# Review: Prefer raw fetch, scope large tree listings

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Opus-class), Anthropic
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [BLOCKING:design] Raw-attempt failure detection is undefined
**Section:** Decisions → "Raw-first file reads ship as a script" / Testing → `github-read-selftest.sh`
**Issue:** The discussion says raw is tried first and `gh api` used "only on failure", but never defines what failure is — curl exit status, HTTP status capture, or `-f`; with a plain `curl URL`, raw's 404 returns exit 0 and a `404: Not Found` body that would be emitted as file content, and body-emptiness cannot serve as the signal because the "Empty file: exit 0, empty stdout" scenario requires empty-and-successful to be a valid outcome.
**Fix:** State the curl invocation contract explicitly (flag set, how HTTP status is obtained, which statuses trigger fallback), since the harness is asked to assert the exact curl call vector.

### [BLOCKING:design] github-read.sh stdout buffering mechanism unchosen
**Section:** Constraints ("Strict stdout discipline"), Scope → Out ("writes whatever bytes it receives"), Testing → "stdout cleanliness"
**Issue:** Three stated properties collide and no mechanism is picked: streaming curl straight to stdout violates "a failure never leaves a partial prefix on stdout" when raw dies mid-body and the fallback then appends; command-substitution buffering strips trailing newlines and drops NUL bytes, contradicting both "byte-identical to the fixture with no added trailing newline" and the no-binary-detection promise.
**Fix:** Decide and record the mechanism (temp file under a stated location, `curl -o`, or an explicit relaxation of the no-partial-prefix rule for `github-read.sh`) before plan writing.

### [NIT:consistency] `--children` truncation abort not actually inherited
**Section:** Technical context → `github-tree.sh` bullets
**Issue:** "`--children` bypasses the queue entirely" and "the non-recursive path already has its own truncation abort, which `--children` inherits for free" contradict each other — that abort lives inside the walk loop's `else` branch (github-tree.sh:224-226), so a queue-bypassing `--children` must restate or hoist it.
**Fix:** Say the truncation check is duplicated or extracted for the `--children` path rather than inherited.

### [NIT:consistency] "every existing invocation preserved verbatim" is overstated
**Section:** Decisions → "Flag parsing accepts flags before positionals only"
**Issue:** `-` is inside today's accepted path set, so `github-tree.sh acme/x --foo` currently reaches path validation and the API; the new rule ("flags after the positionals are a usage error", `--` needed for a leading-`-` path) changes that to exit 2, and the rule's scope (`--`-prefixed only, or any `-`-prefixed token?) is left ambiguous.
**Fix:** Pin whether the flag test is `--`-prefixed only, and note the leading-dash path edge case as an accepted deviation from byte-identical behaviour.

### [NIT:scope] No curl timeout decided
**Section:** Constraints → "No retries, no backoff"
**Issue:** No-retry is settled, but nothing states whether the raw attempt carries a connect/max time bound; without one a hung raw request stalls the read with no fallback ever firing, which is a distinct concern from retrying.
**Fix:** State a timeout value (or an explicit "no timeout") for the raw attempt.

## Verdict

REQUEST_CHANGES
Two undecided github-read.sh mechanics — raw-failure detection and stdout buffering — block planning.
MILL_REVIEW_END
