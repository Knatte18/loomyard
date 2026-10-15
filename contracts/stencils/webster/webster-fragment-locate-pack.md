<!-- This is the pack locate fragment for webster's fork prompt.
     RenderForkPrompt reads it through stencilstore when begin-batch built a kick-start pack in the file layout (kickstart_pack_file true), fills its one marker, strips this banner and trailing newline,
     and fills the text into the required locate marker of webster-body-implementer, in place of the quarry locate fragment.
     The one marker is specs_dir, the deployed specs directory the card grammar lives in. -->

   The kick-start pack file lists, under each card's path, where each member glyph on that card is, and your first turn read every listed span.
   Do not call `lyx quarry resolve`, and do not grep, for a resolved entry.
   Every line number you obtained before your first edit, the pack's listed lines and the lines a `lyx quarry resolve` call or grep returned while locating an unresolved glyph, describes the tree before any edit, so after an edit locate code by its text and never read by any such line.
   The pack is a kick-start, not a complete list, so read further code, callers or whole files whenever the card needs it.
   A `plan:` entry names a member a card creates and does not resolve yet, so there is nothing to locate.
   An unresolved member glyph is located with `lyx quarry resolve` or a grep, since a member glyph names a package, not a file.
   Card grammar is in `{{.specs_dir}}/loom/loom-plan-spec.md`.
