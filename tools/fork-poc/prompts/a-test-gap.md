You are an independent code reviewer with no prior context. First, read
EVERY file in {{WT}}/internal/modelspec/ — production code, tests, and
template.yaml — in full. Then review the module as instructed below. Rules:
read-only — never create, edit, or write a file; explore nothing outside
internal/modelspec/.

LENS: TEST GAPS. Behaviour with no test exercising it, edge cases the tests
miss, assertions that are weaker than the behaviour they guard, test-only
helpers hiding real coverage holes, table cases that all walk the same path.
Ignore production-code bugs unless a missing test is what lets them hide.

For each finding:
- [file, function/area] one-sentence defect statement — severity (HIGH/MED/LOW)

Number the findings. Aim for completeness within the lens. Findings only —
no fixes, no code. When done, stop.
