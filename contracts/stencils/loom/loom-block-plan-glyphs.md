<!-- This is the loom-block-plan-glyphs plan-format block: the `lym quarry` glyph lookup rule.
     loom-template-plan, loom-template-part-writer and loom-template-parts-chair pull it in with the bare {{template "loom-block-plan-glyphs"}}, with no pipeline.
     The fill hands it the including seat's value map, so it declares no markers.
     It names no step of the stencil around it; each including stencil states its own rule for the lines this block leaves out. -->

### Look up glyphs with `lym quarry` — never spell one from memory

`lym quarry` is your only source of glyph spellings,
and it answers against the current worktree only — it takes no repository-path flag, so a spelling it gives you is always from the tree the plan validator resolves against.

- `lym quarry glyphs <dir>` is the flat index: every symbol under `<dir>`'s whole tree, depth-first, each with its own glyph spelling.
  This is what you read a card's target spellings out of.
  `lym quarry glyphs --text <dir>` gives the same index as one symbol per line, the form to use before any line filter such as `grep`.
  The glyph to copy from a `--text` line is its last field, the id — never the whole line.
- `lym quarry resolve <glyph>...` checks that a spelling names something real — pass it one or more glyphs, positionally, in one call.
  It answers each glyph separately, and exits non-zero when any glyph is not found, ambiguous, or rejected by the grammar, but still prints every glyph's answer.
  Read each answer's `status` (or `error`, for a grammar-rejected spelling) rather than treating the non-zero exit as a tool failure.
- `lym quarry toc <path>` and `lym quarry expand <glyph>` are the structure and detail queries: `toc` for a directory's shape, and `expand` for a type's own head plus every member whose owner chain begins with it.

**You never spell a glyph — you copy a line verbatim out of a quarry answer.**