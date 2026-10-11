<!-- This is the kick-start pack section of webster's fork prompt.
     renderPackSection (render.go) reads it through stencilstore, fills its two markers with Go-rendered lists, and strips this banner and trailing newline;
     RenderForkPrompt fills the result into the optional kickstart_pack marker of webster-body-implementer.
     pack_entries lists the member glyphs the batch's cards name, each with its declarations;
     pack_unresolved lists the refs that did not resolve, each with its reason. -->

## Kick-start pack

The member glyphs on this batch's cards, resolved against the tree at the batch's start.
Each entry gives the glyph, then one line per declaration with its file, line span and signature.

{{.pack_entries}}

### Unresolved

These refs on this batch's cards did not resolve to a declaration, each with its reason.

{{.pack_unresolved}}
