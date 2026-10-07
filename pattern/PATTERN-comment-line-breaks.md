# PATTERN-comment-line-breaks

Go comments, doc and inline alike, use semantic line breaks.
The rule applies `scribe:prose`'s line-break rule with the clarifications below;
inside lyx's reviewers, fixers and lint, this reading governs where `scribe:prose` reads as requiring the clause break.
`lyx loom lint-comments` enforces part of it, run by every card gate and by the Webster-Burler round gate;
the rest is review discipline.

- One sentence per line.
- A break inside a sentence is allowed only at an independent-clause boundary: after a semicolon, or after a comma before a coordinating conjunction whose clause has its own subject and verb.
- The clause-boundary break is permitted, never required, so its absence is no finding.
- A sentence end includes `.`, `!`, `?` or `:` followed by a closing `)`, quote or backtick, so a line ending in a colon is an allowed break.
- A break at any other comma (a list item, a compound predicate, a relative or introductory clause) is a violation.
- There is no column limit, so a leading tab's width never matters.
- Existing fixed-column-wrapped comments are legacy, not a convention to match.
- A line-width finding is never legitimate, and the rule governs new and changed comments only.
- A break between two untouched lines, or one an edit carried over unchanged, is never a finding, and a fix rewraps only the sentence the edit touches.

## Lint bound

The lint flags a break the diff creates that ends neither a sentence, nor at a semicolon, nor at a comma before a coordinating conjunction.
It lets every comma-plus-conjunction break through, compound predicates included, because telling those from independent clauses needs a parser; they stay review findings.
