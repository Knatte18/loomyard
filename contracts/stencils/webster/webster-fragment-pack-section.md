<!-- This is the kick-start pack section of webster's fork prompt.
     renderPackSection (render.go) reads it through stencilstore, fills its one marker with the Go-rendered card subsections, and strips this banner and trailing newline;
     RenderForkPrompt fills the result into the optional kickstart_pack marker of webster-body-implementer.
     pack_cards holds one subsection per card of the batch, each rendered from webster-fragment-pack-card. -->

## Kick-start pack

The member glyphs on each card of this batch, resolved against the tree at the batch's start.
Each card's subsection sits under a heading that is that card's path in the card list above.
Each entry gives the glyph, then one line per declaration with its file, line span and signature, and the card's unresolved refs come after its entries.

{{.pack_cards}}
