You have already explored {{WT}}/internal/modelspec/ in this conversation —
its code is in your context. Review it now through ONE lens only:

LENS: TEST GAPS. Behaviour with no test exercising it, edge cases the tests
miss, assertions that are weaker than the behaviour they guard, test-only
helpers hiding real coverage holes, table cases that all walk the same path.
Ignore production-code bugs unless a missing test is what lets them hide.

Do NOT re-read the files; rely on inherited context. Do not use any tools.

For each finding:
- [file, function/area] one-sentence defect statement — severity (HIGH/MED/LOW)

Number the findings. Aim for completeness within the lens. Findings only —
no fixes, no code. When done, stop.
