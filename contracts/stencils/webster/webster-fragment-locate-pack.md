<!-- This is the pack locate fragment for webster's fork prompt.
     RenderForkPrompt reads it through stencilstore when begin-batch built a kick-start pack, fills its one marker, strips this banner and trailing newline,
     and fills the text into the required locate marker of webster-body-implementer, in place of the quarry locate fragment.
     The one marker is specs_dir, the deployed specs directory the card grammar lives in. -->

   The kick-start pack below lists, under each card's path, where each member glyph on that card is, and step 1 read the card's listed spans.
   Do not call `lyx quarry resolve`, and do not grep, for a resolved entry.
   Spans are as of the batch's start, so in a file an earlier card of this batch edited, find the member by its glyph's member name, with the listed signature as a cross-check.
   The pack is a kick-start, not a complete list, so read further code, callers or whole files whenever the card needs it.
   A `plan:` entry names a member a card creates and does not resolve yet, so there is nothing to locate.
   An unresolved member glyph is located with `lyx quarry resolve` or a grep, since a member glyph names a package, not a file.
   Card grammar is in `{{.specs_dir}}/loom/loom-plan-spec.md`.
