MILL_REVIEW_BEGIN
# Review: webster: rewrite for flat card list

```yaml
verdict: GAPS_FOUND
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-25
```

## Findings

### [GAP] Per-card `verify:` field unaddressed
**Section:** Scope / fork-return-contract / integration-suite-fork-with-bisect
**Issue:** `plan-format-v3.md` defines an optional per-card `**verify:**` field, but the discussion only names build+unit-tests after each card and the plan-level `## verify:` at the end — it never says whether `planparser` parses/stores the per-card `verify:` and whether the fork runs it.
**Fix:** State explicitly whether the per-card `verify:` is parsed and executed after its card, or deliberately ignored in v0 (and why).

### [GAP] Plan-level sections not surfaced to the fork prompt
**Section:** Technical context (render.go) / Scope
**Issue:** The render.go rewrite scope lists "card list + per-batch cards + new return contract" but does not say the fork prompt surfaces `## Shared Decisions` or the CANONICAL `## Rename mechanic` — a fork implementing a `Moves:` card needs that mechanic text and shared decisions to act correctly.
**Fix:** Specify that RenderForkPrompt injects the relevant plan-level `## Shared Decisions` and (when the batch has a `Moves:`) `## Rename mechanic` into the fork prompt.

### [NOTE] Fork report on-disk format left unspecified
**Section:** fork-return-contract / recordbatch report parser
**Issue:** The new minimal report parser replaces `builderengine.ParseReport`, but the on-disk grammar the fork writes (OK/FAILED + head SHA + deviation file list) — its file location and exact shape — is not pinned in the discussion.
**Fix:** Note the report file's on-disk format/location is a plan-phase determination, or sketch the minimal grammar.

### [NOTE] Batcher config key name unspecified
**Section:** batcher-library-config-selected
**Issue:** The active batcher is "chosen via a key in `webster.yaml`" but the key name and its default value are not given.
**Fix:** Name the config key (and that it defaults to `identity`), or explicitly defer to plan phase.

### [NOTE] On-disk existence checks vs. hermetic fixtures
**Section:** Testing (planparser) / Scope
**Issue:** `move-source-missing`, `move-target-collision`, and `path-missing` require checking real worktree files, but the parser's filesystem root for those checks and how `testdata/` fixtures supply those on-disk paths hermetically is not stated.
**Fix:** State how the parser receives the worktree root for existence checks and how fixtures represent on-disk files under `testdata/`.

### [NOTE] Human-escalation mechanism undefined
**Section:** integration-suite-fork-with-bisect / Out (auto-retry)
**Issue:** "Escalate to a human" is the terminal action on fork/integration failure, but the mechanism (pause state, summary path, operator signal) is not described beyond writing the summary doc.
**Fix:** Note how escalation surfaces (e.g. reuse `pause`/state + summary) or defer explicitly.

## Verdict

GAPS_FOUND
Two behavior gaps: per-card `verify:` handling and fork-prompt plan-section surfacing must be resolved.
MILL_REVIEW_END
