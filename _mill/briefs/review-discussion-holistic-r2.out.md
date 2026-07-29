MILL_REVIEW_BEGIN
# Review: burler: split the round prompt into an orchestrator + three instruction files

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewer_self_id: claude-opus-4-8
reviewed_file: _mill/discussion.md
date: 2026-07-29
```

## Findings

### [NOTE] Prose calls DotLyxDir the "_lyx" tree; it is ".lyx"
**Section:** Decisions §"Who materializes" (l.50); Technical context (l.90); Constraints (l.102)
**Issue:** `DotLyxDir()` returns `.lyx` (Cwd-anchored, machine-local, never-committed), but the prose repeatedly labels the instruction dir as under `_lyx` and even calls it "the ... never-committed `_lyx` tree" — yet `_lyx` (LyxDir) is the weft-*synced/committed* tree; the never-committed one is `.lyx`. The operational instruction (use `layout.DotLyxDir()`) is named correctly throughout, so the plan can proceed, but doc.go is a same-commit deliverable and could propagate the wrong name.
**Fix:** In the discussion prose and the eventual doc.go text, spell the tree `.lyx`/`DotLyxDir()` (not `_lyx`) so the durable module doc does not assert burler writes scaffolding into the weft-synced `_lyx`.

### [NOTE] Existing TestTemplate_HasClusterRulesSection relocation unstated
**Section:** Testing (l.112–113)
**Issue:** `template_test.go`'s `TestTemplate_HasClusterRulesSection` pins the `## Cluster rules` heading + `{{.cluster_rules}}` marker in the raw monolith bytes; after the split that content lives in instruction 2, but the Testing section enumerates relocations for the other four tests and omits this one.
**Fix:** Name it in the relocation list (re-home to instruction-2 bytes) so no plan writer leaves it asserting against the deleted monolith.

## Verdict

APPROVE
Thorough, decided, constraint-aware; only naming/test-relocation notes remain — none block plan writing.
MILL_REVIEW_END
