# PATTERN-comment-line-breaks

Go comments, doc and inline alike, use semantic line breaks.
Enforcement is review discipline, not a test.

- One sentence per line, with a further break at an independent-clause boundary.
- There is no column limit, so a leading tab's width never matters.
- Existing fixed-column-wrapped comments are legacy, not a convention to match.
- A line-width finding is never legitimate, and untouched lines are never rewrapped: the rule governs new and changed comments only.
