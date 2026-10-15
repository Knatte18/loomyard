<!-- This is the inline kick-start pack section of webster's fork prompt.
     renderPackSection (render.go) reads it through stencilstore, fills its one marker with the Go-rendered card subsections, and strips this banner and trailing newline;
     RenderForkPrompt fills the result into the optional kickstart_pack_inline marker of webster-body-implementer when begin-batch built a kick-start pack and kickstart_pack_file is false.
     It is the measurement baseline: its text is the 1e39afc10^ text and changes only when the layout comparison itself is meant to change, never to mirror an edit to the file-layout fragments.
     pack_cards holds one subsection per card of the batch, each rendered from webster-fragment-pack-card. -->

## Kick-start pack

The member glyphs on each card of this batch, resolved against the tree at the batch's start.
Each card's subsection sits under a heading that is that card's path in the card list above.
Each entry gives the glyph, then one line per declaration with its file, line span and signature, and the card's unresolved refs come after its entries.

{{.pack_cards}}
