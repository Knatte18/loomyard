<!-- This is the kick-start pack section of webster's fork prompt.
     renderPackSection (render.go) reads it through stencilstore, fills its one marker with the Go-rendered card subsections, and strips this banner and trailing newline;
     RenderForkPrompt fills the result into the optional kickstart_pack marker of webster-body-implementer, which ends the prompt, so the section is the prompt's last.
     pack_cards holds one subsection per card of the batch, each rendered from webster-fragment-pack-card. -->

## Kick-start pack

Read all of this in one turn, in parallel, before any edit: every card file the card list above points at, and every span this section lists under every card, each span with the Read tool's `offset` at its start line and a `limit` reaching its end line.
A span listed under several cards is read once.
After that turn and still before any edit, read whole every existing file any card's target list or `Uses:` list names that no listed span covers, such as a plain path or a self glyph's file.
Also before any edit, locate every unresolved member glyph of every card with `lyx quarry resolve` or a grep, then read its file whole.
A `plan:` handle in an unresolved list names a member a card creates, so it is never located.
Then implement the cards.

The member glyphs on each card of this batch, resolved against the tree at the batch's start.
Each card's subsection sits under a heading that is that card's path in the card list above.
Each entry gives the glyph, then one line per declaration with its file, line span and signature, and the card's unresolved refs come after its entries.

{{.pack_cards}}
