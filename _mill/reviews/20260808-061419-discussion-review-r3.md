MILL_REVIEW_BEGIN
# Review: Audit the remaining leaf and seam import invariants

```yaml
verdict: GAPS_FOUND
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-08
```

## Findings

### [GAP] Audit population criterion unstated; lyxcwd cap excluded
**Section:** Audit results / Scope → Out
**Issue:** The seven audited rules are asserted without a selection criterion, and at least one same-shape rule sits outside them unexamined — `CONSTRAINTS.md:24` "`internal/lyxcwd`'s own imports are capped at stdlib plus `internal/gitexec`", which I verified is true today (only `lyxcwd.go:17` imports `gitexec`) but is enforced by **no** allowlist test: `internal/lyxcwd/enforcement_test.go` contains only `TestEnforcement`, `TestEnforcement_GeometryLiterals`, `TestEnforcement_FabricVocabulary`, none of which check lyxcwd's own import set.
That is precisely the lyxtest finding's shape (stated import set, unenforced), so a plan writer cannot tell whether omitting it is a decision or an oversight.
**Fix:** State the criterion that bounds the seven (e.g. "rules whose CONSTRAINTS entry names a dedicated enforcement test") and record lyxcwd's cap explicitly as audited-and-deferred with a reason, rather than leaving it under the blanket "every other invariant".

### [GAP] "Go enforces it" claim on lyxcwd's cap not swept
**Section:** Audit results → Sweep for other now-invalid claims
**Issue:** `docs/shared-libs/lyxcwd.md:6` reads "**Dependency direction (Go enforces it):** `internal/lyxcwd`'s own imports are capped at stdlib plus `internal/gitexec` — nothing else, ever."
Go enforces only acyclicity, not the cap — a non-cyclic stray import would pass silently.
The sweep's pass 3 pattern (`never imports|does not import|imports only`) does not match this phrasing, so the sweep is structurally blind to it.
**Fix:** Decide in-scope or out-of-scope explicitly, and record that the sweep's pattern misses "capped at"/"Go enforces" phrasings so a re-run is not mistaken for exhaustive.

### [GAP] Fate of the test-build-cycle rationale unspecified
**Section:** Scope → In (`internal/lyxtest/doc.go` lines 7-13)
**Issue:** The scope deletes both the denylist framing *and* the "would close a test-build cycle" sentences (verified at `doc.go:7-13`), while the conversion also removes the `bannedImports` block and its cycle comment (`leaf_enforcement_test.go:32-46`) — leaving `CONSTRAINTS.md:37` as the sole remaining statement of *why* the allowlist must not be widened.
Sibling docs (`internal/tokenvocab/doc.go:10-12`) do carry their reason locally.
**Fix:** State whether the rewritten `doc.go` and test header retain the cycle rationale in one clause, or deliberately delegate it to `CONSTRAINTS.md`.

### [NOTE] modelspec:4 fix left as two options
**Section:** Technical context → `internal/modelspec/leaf_enforcement_test.go:4`
**Issue:** "the contrast is simply deleted or reworded" leaves two outcomes open for a file whose sibling convention is fixed (`pattern`/`tokenvocab`/`githubclient` all use "Like modelspec's … this check is an ALLOWLIST").
**Fix:** Name the sibling wording as the target so mill-plan does not re-decide.

### [NOTE] shuttleengine:22 cross-reference target undecided
**Section:** Decisions → "lyxtest's test is renamed"
**Issue:** Only the function name is decided; `seam_enforcement_test.go:22` cites lyxtest as the *style origin* of the `ImportsOnly` idiom, which becomes circular once lyxtest is itself a copy of `internal/pattern`'s shape.
**Fix:** Decide whether the citation retargets or merely updates the name.

## Verdict

GAPS_FOUND
Audit boundary and one doc-rationale outcome need closing before plan writing.
MILL_REVIEW_END
