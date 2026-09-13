# Review: modelspec: bare-effort shorthand and version-to-v rename

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-09-13
```

## Findings

### [NIT:consistency] Wrong line number cited for the deleted "param with no equals" test case

**Section:** Testing — "`TestParse_Rejects` — deleted case"
**Issue:** The discussion cites `internal/modelspec/parse_test.go`, line 186 for the `"param with no equals"` case (`sonnet[effort]` / `no '=' separator`). In the actual file this test case's `name:`/`in:`/`wantSubstr:` lines are 176–178, not 186 — the `t.Fatalf` cleanup-check line inside the test runner loop is what sits at 186. Every other line citation in the discussion (parse.go:47-51, 60-70, 112-118, 122-155; registry_test.go:66,69,149,161; llm-model-spec.md:64,66,112; docs/overview.md:302) checked out exactly against the current source.
**Suggested fix:** No action needed before mill-plan — the case is identified unambiguously by name and by its `in`/`wantSubstr` values, so the off-by-~10 line number costs nothing during implementation. Worth a mental note only if the plan step also cites this line number verbatim.

## Verdict

APPROVE
The discussion is thoroughly grounded — every cited production call site (7 `Params["version"]` readers), every `version=` occurrence repo-wide, every charset/precedence/validation claim, and the claimed absence of programmatic bracket-spec construction all check out exactly against the current `internal/modelspec` source and its callers, so the four-file blast radius, the `bracketKeys`-vs-`knownParams` vocabulary split, and the deleted `no '=' separator` behavior are all sound and ready for `mill-plan`.
