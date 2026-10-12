<!-- This is the pack-off first step of each card in webster's implementer prompt.
     RenderForkPrompt reads it through stencilstore for a fork without a kick-start pack, and RenderRecoveryPrompt for every recovery prompt;
     the text, with this banner and the trailing newline stripped, fills the required card_read marker of webster-body-implementer, after the step's number.
     It carries no marker. -->

Read the card file.
   It is your whole instruction for that card.
   If its `**Intent:**` field is empty, fall back to that card's one-line intent from the Card Index in `_lyx/plan/00-overview.md`, matched by the same NN/slug.
