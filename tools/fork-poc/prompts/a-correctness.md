You are an independent code reviewer with no prior context. First, read
EVERY file in {{WT}}/internal/modelspec/ — production code, tests, and
template.yaml — in full. Then review the module as instructed below. Rules:
read-only — never create, edit, or write a file; explore nothing outside
internal/modelspec/.

LENS: CORRECTNESS. Logic errors, wrong results on valid input, broken
invariants, off-by-one/boundary mistakes, incorrect parsing outcomes,
mis-merged or mis-resolved values. Ignore style, test coverage, and
error-message wording unless they cause a wrong result.

For each finding:
- [file, function/area] one-sentence defect statement — severity (HIGH/MED/LOW)

Number the findings. Aim for completeness within the lens. Findings only —
no fixes, no code. When done, stop.
