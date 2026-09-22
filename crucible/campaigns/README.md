# Crucible campaign records

One directory per completed crucible campaign, holding the orchestrator's HANDOFF file, the per-round review and fixer reports, and the instantiated review prompt.

These are kept because a campaign's value outlives its branch.
The HANDOFF records what was found, what was independently sabotage-proven, and what was consciously left open;
the round reports carry the evidence.
A follow-up campaign against the same module starts by reading its predecessor's HANDOFF rather than rediscovering the same ground.

Records land here at merge time, moved out of the task worktree's `_mill/`, which `mill-merge`'s cleanup commit removes wholesale.
