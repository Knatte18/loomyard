# PATTERN-comment-line-breaks

Go comments, doc and inline alike, use semantic line breaks.
The rule applies `scribe:prose`'s line-break rule with the clarifications below;
inside lyx's writers, fixers and lint, this reading governs where `scribe:prose` reads as requiring the clause break.
`lyx loom lint-comments` alone enforces it, run by every card gate and by the Webster-Burler round gate.
A comment's line break or wrap is never a review finding: no reviewer or judge raises it, and a fixer rewraps only the lines the lint flags.

The writing rules, guidance for whoever writes or changes a comment:

- One sentence per line.
- A break inside a sentence is allowed only at an independent-clause boundary: after a semicolon, or after a comma before a coordinating conjunction whose clause has its own subject and verb.
- The clause-boundary break is permitted, never required.
- A sentence end includes `.`, `!`, `?` or `:` followed by a closing `)`, quote or backtick, so a line ending in a colon is an allowed break.
- A break at any other comma (a list item, a compound predicate, a relative or introductory clause) breaks the rule.
- There is no column limit, so a leading tab's width never matters.
- Existing fixed-column-wrapped comments are legacy, not a convention to match.
- The rule governs new and changed comments only; a break between two untouched lines, or one an edit carried over unchanged, is left as it is.

## Lint bound

The lint flags a break the diff creates that ends neither a sentence, nor at a semicolon, nor at a comma before a coordinating conjunction that opens a clause.
A comma-plus-conjunction break passes only when the words after the conjunction, up to the next `,`, `;`, `:` or sentence end, start with a subject and hold a finite-verb candidate after the subject's head word.
A subject is a subject or demonstrative pronoun, a determiner followed by a word, or a backticked identifier; a finite-verb candidate is an auxiliary or modal from a closed set, or a word ending in `s` or `ed`.
Any other comma-plus-conjunction break, such as one that joins a list item or a compound predicate, is a finding, and an uncertain case errs toward flagging, since joining the two lines always passes.
The check is a word-list heuristic, so a plural noun read as a verb (`, and the gate tests`) passes, and a clause whose subject the opener set misses (`, and nothing runs`) is flagged and joined.
Only a break the diff creates is checked, and it is writer guidance, never a review finding.
