MILL_REVIEW_BEGIN
# Review: Spike: structured Go reference/call-graph lookup (go/packages / gopls)

```yaml
verdict: APPROVE
reviewer_model: opushigh
reviewed_file: _mill/discussion.md
date: 2026-07-17
```

## Findings

### [NOTE] Callgraph root/entry-point selection unspecified
**Section:** Decisions → transitive-impact-in-scope; Testing (CHA/RTA/VTA)
**Issue:** RTA/VTA require seed roots to build the SSA callgraph before incoming edges can be queried, but no root set (e.g. `cmd/lyx` main + inits + tests) is named; results are meaningless without it.
**Fix:** Note in the findings method that the author selects and records the callgraph roots used, since RTA/VTA reachability depends on them.

### [NOTE] No fallback stated if gopls cannot be installed
**Section:** Technical context → Dependencies; Decisions → cc-native-lsp-mismatch
**Issue:** A docs-only fallback exists for the CC-native arm, but the gopls-held-open-subprocess comparison arm assumes `go install gopls` succeeds, with no stated outcome if the environment blocks install/network.
**Fix:** State that the in-process arm is load-bearing and the gopls-subprocess comparison degrades to docs-characterization if gopls cannot be installed here.

## Verdict

APPROVE
Thorough; r1 transitive-measurement gap resolved, only two non-blocking method notes remain.
MILL_REVIEW_END
