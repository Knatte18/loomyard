Twill Discussion bench, setup deviations per round (2026-10-11); the full record is `twill-discussion-bench.md`.

13:1x twill writer (twb-tw-om-1) found pair at 50cf6a425 where 1e79825ff already closed the issues; messaged ly:orch; orch replied with the base-800a0e892 instruction mid-run. Solo arms get the same text up front in the note body (input-solo-arms.md).
- twb-so-om-1 forked from main at 32a657839 (main moved); code branch reset to 50cf6a425. Prod lyx binary is now 32a657839 (twill run used 50cf6a425); diff touches reed, darn/plan stencils only, not the Discussion-Write path.
- Round 4 (combo, twp, twc): stencil leak to Bolt in 553aa524, restored in 2be8608b; combo chairs forked through a claude wrapper setting CLAUDE_CODE_FORK_SUBAGENT=1 and claude_deny_agent_tool: false (pair-local); see twill-discussion-bench.md "Deviations, round 4".
- Round 5 (twh): no setup deviation; twb-twh-2's local code branch survived `lyx fabric remove --remote` and was deleted by hand, as in round 4.
