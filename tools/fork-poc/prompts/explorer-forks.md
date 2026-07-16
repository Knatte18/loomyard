You are the shared explorer for a review cluster, and you will fan the review
out to fork subagents that inherit your full conversation context.

PHASE 1 — explore. Read EVERY file in {{WT}}/internal/modelspec/ — production
code, tests, and template.yaml — in full. Do not skim. Also remember this
nonce: NONCE={{NONCE}}

PHASE 2 — fork. When exploration is complete, launch THREE PARALLEL subagents
in a single message, all via the Agent tool. IMPORTANT: OMIT the
subagent_type field entirely (that makes them forks inheriting your full
context) and do NOT give them names. Each gets one prompt:

Subagent 1 prompt:
"You inherit the full conversation, including every internal/modelspec file
already read. Do NOT re-read files, do NOT use any tools. First line of your
reply: NONCE=<the nonce from the conversation>. Then review the module
through ONE lens only — CORRECTNESS: logic errors, wrong results on valid
input, broken invariants, boundary mistakes. Numbered findings, each:
[file, area] one-sentence defect — severity (HIGH/MED/LOW). Findings only."

Subagent 2 prompt: same text, but the lens is ERROR HANDLING & ROBUSTNESS:
swallowed errors, missing error paths, panics on hostile input, behaviour on
malformed/empty/weird yaml or spec strings.

Subagent 3 prompt: same text, but the lens is TEST GAPS: untested behaviour,
edge cases the tests miss, assertions weaker than what they guard.

PHASE 3 — report. When all three return, output their three reports
VERBATIM under headers "## correctness", "## error-handling", "## test-gap".
Then stop.

Rules: read-only — never create, edit, or write any file.
