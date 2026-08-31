MILL_REVIEW_BEGIN
# Review: Add cross-repo code search to prowler

```yaml
duration_s: 120.0
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Opus-class, Anthropic) — exact build not self-observable
reviewed_file: _mill/discussion.md
date: 2026-08-31
```

## Findings

### [NIT:consistency] Search stub shape omits `--jq`
**Section:** "The new stub keys fixtures on a request key" / Testing **Issue:** The enumerated search invocation shape is "`-X GET`, repeated `-f`, `-H Accept:`" with no `--jq`, yet "Runtime dependency is `gh` alone" requires all extraction via `gh api --jq`, and the stub must apply that expression to the fixture body (as `testdata/github-tree/bin/gh` line 80 does). **Fix:** State `--jq <expr>` as part of the accepted search shape and assert its presence alongside `-X GET`.

### [NIT:design] Preflight `--jq` rationale rests on an unverified premise
**Section:** "Preflight each repo against the core rate-limit bucket" **Issue:** The claim that dropping `--jq` "would leave the script with an exit code and no way to tell 404 from 403" is asserted, not in the verified-live table; the tree stub writes the error body to stdout independently of `--jq`, so the premise for "there is no bare two-argument `api <endpoint>` shape to support" is unconfirmed. **Fix:** Either mark the claim as unverified or drop it — the decision itself (keep `--jq`) is unaffected either way.

### [NIT:decision] Empty `text_matches` behaviour stated only in Testing
**Section:** "Single output format: repo, path, snippet — always" **Issue:** The decision defines the snippet as "the first `text_matches` fragment" and never says what happens when the array is absent or empty; the empty-third-field behaviour appears only in the scenario list. **Fix:** Fold the empty-fragment case into the output-format decision so the contract is stated where it is defined.

## Verdict

APPROVE
Scope, decisions, exit codes, and harness contract verified against source; only cosmetic gaps remain.
MILL_REVIEW_END
