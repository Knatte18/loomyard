<!-- This is the declared-changes fragment of a PATTERN directive.
     internal/pattern.Directive reads it through stencilstore.Read when the caller passes declared changes,
     strips this banner and fills the file's one marker, the pattern_change_list marker, with the declared bullet lines joined by newlines.
     The result fills the optional pattern_changes marker of the role's directive stencil.
     This file declares that one marker and no other, and the marker is required. -->

### Declared PATTERN changes

This task declares these changes to PATTERN entries, and its own diff makes them:

{{.pattern_change_list}}

- An entry listed here is followed and judged by its listed new text, never by the overview's current text.
- An entry listed as `removes` no longer binds.
- A departure from a listed entry cites the listed text.
- Only the entries listed here are judged by their listed text; every other entry is judged by the overview's current text.
