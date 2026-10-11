<!-- This is the quarry locate fragment for webster's fork and recovery prompts.
     RenderForkPrompt (with no pack) and RenderRecoveryPrompt (always) read it through stencilstore, strip this banner and trailing newline,
     and fill the text into the required locate marker of webster-body-implementer, at the end of step 2 of the per-card loop.
     The text is the locate step as it stood before the kick-start pack, indentation included, so a fork without a pack reads the prompt it always did.
     It carries no marker. -->

   Locate them with one `lyx quarry resolve <glyph>...` call over the card's target and `Uses:` glyphs: each answer gives the member's `file` and its `start` and `end` lines, so read those spans instead of grepping for them or reading whole files.
   A `plan:` handle names a member the card creates and does not resolve yet.
