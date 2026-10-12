<!-- This is the pack-on first step of each card in webster's implementer prompt.
     RenderForkPrompt reads it through stencilstore when begin-batch built a kick-start pack;
     the text, with this banner and the trailing newline stripped, fills the required card_read marker of webster-body-implementer, after the step's number.
     It carries no marker. -->

At the start of each card, read the card file and every span the kick-start pack below lists under that card's heading, in parallel, in one turn, before doing anything else.
   Read each span with the Read tool's `offset` at its start line and a `limit` reaching its end line.
   A span in a file an earlier card of this batch edited is not read by offset: read that file whole in the same turn, and find the member in it by the member name its glyph names, with the listed signature only a cross-check, since an earlier card may have re-signed it.
   The card file is the first of these reads.
   It is your whole instruction for that card.
   If its `**Intent:**` field is empty, fall back to that card's one-line intent from the Card Index in `_lyx/plan/00-overview.md`, matched by the same NN/slug.
