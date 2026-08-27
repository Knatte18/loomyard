# Review: Add a local-only file category to weft

```yaml
verdict: REQUEST_CHANGES
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-08-27
```

## Findings

### [BLOCKING:consistency] CONSTRAINTS.md premise is stale on this branch
**Section:** `### constraints-gains-one-sentence` / "Constraints"
**Issue:** The decision assumes CONSTRAINTS.md "was trimmed on `main` to rules-only, no rationale" and that the new sentence "matches that voice." That trim landed on `main` at `d66cefe5`, but this branch's parent commit predates it — the branch's own `CONSTRAINTS.md` is still the untrimmed, 659-line, rationale-heavy version. Editing it as-is and later merging would produce a large, unrelated conflict against `main`'s already-trimmed file.
**Fix:** Add an explicit step — pull/merge-in `main` (`mill-merge-in`) before touching `CONSTRAINTS.md`, so the one added sentence lands against the file's actual current shape.

## Verdict

REQUEST_CHANGES
Sound design, verified against source; one branch/parent sync gap must be named before plan writing.
