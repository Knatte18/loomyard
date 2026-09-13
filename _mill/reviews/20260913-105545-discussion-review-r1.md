# Review: Rename hub container suffix from -HUB to -LYXHUB

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [NIT:consistency] File-inventory total is off by one against its own enumeration
**Section:** Technical context → File inventory
**Issue:** The header states "52 files" but the enumerated production/test/docs lists under it sum to 51 (verified by grep: `grep -rl -- "-HUB" ... internal cmd docs tools manifest contracts` returns exactly 51 matches, all of which appear in the discussion's own lists). `CONSTRAINTS.md` is named separately in Scope → In as an item to update but is not one of the 51 enumerated files (it currently contains no `-HUB` literal — it gets a new addition, not a substitution), which would reconcile the count to 52 if that's what was intended, but the doc never states this explicitly.
**Suggested fix:** Either fold `CONSTRAINTS.md` into the File inventory list explicitly (as "53. new content, no existing literal") or correct the header count to 51 + 1 with a one-line note. Cosmetic only — every actual file is already named, so a plan writer has no ambiguity about what to touch.

## Verdict

APPROVE
Every technical claim (both declarers and their sole call sites, `looksLikeHub`/`DeriveWarpName` name-independence, the full `RepoName` consumer map, the portal/launcher/socket-key rationale for "no physical rename", and the class-(a)/(b)/(c) test-literal sort) was independently re-verified against current source and checked out exactly, the open migration question from the brief is resolved with a well-reasoned decision, and the operator-scope-extension caveat is handled honestly rather than fabricated.
