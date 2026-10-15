<!-- This is the per-card pack locate fragment for webster's fork prompt.
     RenderForkPrompt reads it through stencilstore when begin-batch built a kick-start pack and kickstart_pack_per_card is true in a hub run, fills its one marker, strips this banner and trailing newline,
     and fills the text into the required locate marker of webster-body-implementer, in place of the quarry locate fragment.
     The one marker is specs_dir, the deployed specs directory the card grammar lives in. -->

   The card pack, taken at this card's start, lists where each member glyph on the card is, and those spans are complete for a start, so read further code, callers or whole files only when the card needs it.
   Do not call `lyx quarry resolve`, and do not grep, for a listed entry.
   The card pack's line numbers hold until this card's first edit only, after which code is located by its text and never read by those lines.
   A `plan:` entry names a member a card creates, so there is nothing to locate.
   An unresolved member glyph is located with `lyx quarry resolve` or a grep, since a member glyph names a package, not a file.
   When the `lyx loom card-pack` call fails, every glyph on the card is located that same way.
   Card grammar is in `{{.specs_dir}}/loom/loom-plan-spec.md`.
