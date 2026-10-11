<!-- This is the pack locate fragment for webster's fork prompt.
     RenderForkPrompt reads it through stencilstore when begin-batch built a kick-start pack, fills its one marker, strips this banner and trailing newline,
     and fills the text into the required locate marker of webster-body-implementer, in place of the quarry locate fragment.
     The one marker is specs_dir, the deployed specs directory the card grammar lives in. -->

   The pack below lists where each member glyph on the cards is.
   Open each listed file and read the listed span before editing.
   Do not call `lyx quarry resolve`, and do not grep, for a resolved entry.
   Spans are as of the batch's start, so in a file already edited in this batch, find the member by its listed signature.
   A `plan:` entry names a member a card creates and does not resolve yet, so there is nothing to locate.
   An unresolved member glyph is located with `lyx quarry resolve` or a grep, since a member glyph names a package, not a file.
   Card grammar is in `{{.specs_dir}}/loom/loom-plan-spec.md`.
