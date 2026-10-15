<!-- This is the per-card first step of each card in webster's implementer prompt.
     RenderForkPrompt reads it through stencilstore when begin-batch built a kick-start pack and kickstart_pack_per_card is true in a hub run;
     the text, with this banner and the trailing newline stripped, fills the required card_read marker of webster-body-implementer, after the step's number.
     It carries no marker. -->

Before this card's first edit, run `lyx loom card-pack <N>` with this card's number, then read in one parallel turn every span it lists, each with the Read tool's `offset` at its start line and a `limit` reaching its end line.
   Then read whole every existing file the card's target list or `Uses:` list names that no listed span covers, and locate every unresolved member glyph that is not a `plan:` handle, then read its file whole.
   The card file, which your first turn read, is your whole instruction for that card.
   If its `**Intent:**` field is empty, fall back to that card's one-line intent from the Card Index in `_lyx/plan/00-overview.md`, matched by the same NN/slug.
