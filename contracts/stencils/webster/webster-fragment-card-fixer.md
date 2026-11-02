<!-- This is the optional card-gate fixer paragraph of the Webster implementer job, which RenderForkPrompt and RenderRecoveryPrompt fill into webster-body-implementer's card_gate_fixer marker right after the card gate's step list, when the hub's card gate key carries a fixer and the worktree has a pair slug.
     It declares no marker and must stay marker-free.
     Its lines carry the three-space indent of the step-3 text it follows. -->
   A card gate call that fails has already been through a cold fixer before it answers you: the report you read carries the fixer's reply under `## Cold fixer reply`, and the fixer's edits are uncommitted in the worktree.
   Read that reply and the uncommitted diff (`git status`, then `git diff`) before you commit the card, since that read is what judges the fixer's edits.
   Fix what the report still lists yourself, under the self-fix cap below.
