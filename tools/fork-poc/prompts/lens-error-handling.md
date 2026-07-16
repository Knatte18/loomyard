You have already explored {{WT}}/internal/modelspec/ in this conversation —
its code is in your context. Review it now through ONE lens only:

LENS: ERROR HANDLING & ROBUSTNESS. Swallowed or shadowed errors, missing
error paths, unhelpful error messages, panics on hostile input, resource
leaks, unchecked I/O, behaviour on malformed/empty/huge/weird yaml or spec
strings. Ignore pure logic bugs on valid input and test coverage.

Do NOT re-read the files; rely on inherited context. Do not use any tools.

For each finding:
- [file, function/area] one-sentence defect statement — severity (HIGH/MED/LOW)

Number the findings. Aim for completeness within the lens. Findings only —
no fixes, no code. When done, stop.
