<!-- This is the inline-layout FRESH-READ rule for webster's implementer prompt.
     RenderForkPrompt reads it through stencilstore when begin-batch built a kick-start pack and kickstart_pack_file is false;
     the text, with this banner and the trailing newline stripped, fills the required fresh_read marker of webster-body-implementer.
     It is the measurement baseline: its text is the 1e39afc10^ text and changes only when the layout comparison itself is meant to change, never to mirror an edit to the file-layout fragments.
     It carries no marker. -->

Inherited context can be stale.
A file you looked at during orientation — your own, or one Master read before forking you — is not necessarily the version on disk right now: a prior batch's own card commits may have changed it since.
Only your own reads, taken now, are current;
content you merely inherited is not.
Your first turn, before any edit, reads in parallel every card file the card list below points at and every span the kick-start pack lists under every card, each span with the Read tool's `offset` at its start line and a `limit` reaching its end line.
A span listed under several cards is read once.
After that turn and still before any edit, read whole every existing file any card's target list or `Uses:` list names that no listed span covers, such as a plain path or a self glyph's file.
Also before any edit, locate every unresolved member glyph of every card with `lyx quarry resolve` or a grep, then read its file whole.
A `plan:` handle in an unresolved list names a member a card creates, so it is never located.
Change a file you have already edited in this batch only with the Edit tool, never by rewriting it whole with the Write tool from content read before that edit.
