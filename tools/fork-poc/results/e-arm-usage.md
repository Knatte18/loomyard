# E-arm: 8-lens handler run (burler phase-1/2 shape, sonnet)

One handler strand: Phase 1 exploration -> Phase 2 spawned 8 unnamed fork
subagents in one message (it used subagent_type: "fork" explicitly — works
in 2.1.204) + its own holistic review -> Phase 3 consolidated review with
origins and a rejected-section (results/e-arm-consolidated.md).

- All 8 forks ran IN PARALLEL (no queueing observed) and every fork's first
  request read 73,078 tokens from the parent's live cache and created 73 —
  effectively perfect prefix sharing at N=8.
- All 8 inherited context (nonce recalled); 7 delivered single-lens reports
  (29 findings); the test-gaps fork went rogue: tried to fork further
  (blocked: "Fork is not available inside a forked worker" — the depth limit
  confirmed empirically), then ran all lenses itself. Two forks used tools
  despite the spike's no-tools instruction.
- The handler-as-judge phase worked: it detected the rogue fork, salvaged
  only its novel findings, cross-checked lenses against Phase-1 ground truth,
  and empirically verified one uncertain claim before including it.
- Handler session total: 438k compute over 58 turns (includes exploration,
  fork orchestration, holistic pass, consolidation, an operator interruption
  + resume, and post-hoc Q&A with the operator). Note: the user interrupted
  once mid-run; a clean run would be leaner.

Design notes fed back into the report:
- Lens prompts must live as configurable templates (shipped defaults,
  per-run selection), never hardcoded in lyx.
- Forks SHOULD have tool access in the real design — exploration cannot be
  assumed complete for every lens; steer with "prefer inherited context,
  fetch only what your lens needs" rather than bans.
- Hard rule in fork prompts: no Agent calls (nested forking fails).
- Go-side compliance check per fork from subagents/*.jsonl: no Agent calls,
  output-format adherence; tool-call counts as signal, not violation.
