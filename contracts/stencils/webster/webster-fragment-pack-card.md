<!-- This is one card's subsection of the kick-start pack of a webster fork, which goes into the kick-start pack file or the stdout of `lyx loom card-pack`.
     renderCardPack (render.go) reads it through stencilstore, fills its markers, and strips this banner and trailing newline;
     renderPackSection joins one such subsection per card and fills the result into the pack_cards marker of the pack section fragment of either layout.
     card_path is the card's path in the card list, pack_entries lists the card's member glyphs each with its declarations,
     and pack_unresolved is optional: the card's unresolved list when it has unresolved refs, and nothing otherwise. -->

### `{{.card_path}}`

{{.pack_entries}}{{.pack_unresolved}}
