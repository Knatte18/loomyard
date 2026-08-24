---
name: prose
description: Precision and terseness rules for every piece of text an agent writes — chat replies, markdown files, code comments, docstrings. Always active.
---

# Prose Skill

Rules for how text is written, independent of where it ends up — including markdown formatting, folded in below rather than split into its own skill since markdown is most of what gets written.
`code-quality`'s Comments section builds on the general rules here rather than restating them;
it owns comment-specific content on top.

---

## Get to the point

State the point first.
No throat-clearing, no "let me explain," no restating what's about to be said before saying it.

## Eliminate empty intensifiers

Words that add emphasis without meaning: "actually", "really", "genuinely", "truly", "completely", "totally", "fully", "definitely", "certainly", "absolutely", "just", "simply", "merely".
Test: remove the word.
If the sentence means the same thing, delete it.

## No padding

Don't restate a point already made.
Don't narrate what's already visible in context — a heading, an example, the surrounding code.
Don't catalogue edge cases or history the reader doesn't need for the point at hand.
If a sentence adds no new information, cut it.

## Say it once

State a rule, fact, or conclusion in the one place it belongs.
Don't repeat it in a summary, a "why this matters" aside, or a closing recap — reference the original instead of restating it.

## Line breaks

Applies to any multi-line prose written into a file — markdown paragraphs and list items, and multi-line code comments/docstrings alike.
Not chat replies, which aren't diffed.

**Semantic line breaks, never fixed-column hard-wrap.**
Fixed-column wrapping breaks mid-phrase,
so a single-word edit touches every wrapped line in the paragraph instead of just the changed words.
Write one sentence per line;
inside a long sentence, also break at an internal independent-clause boundary — a comma before a coordinating conjunction ("but"/"and"/"or"), or a semicolon, where what follows has its own subject and verb.
Use a plain newline.
Never trailing double-spaces or a backslash — those force a real `<br>`.

Table cells and blockquotes stay on one line — this rule doesn't apply inside them.

## Markdown

Applies whenever the output is a `.md` file or a markdown-formatted reply.

- **Headings structure the document, not decorate it.**
  Use them to separate genuinely distinct sections.
  Don't skip levels (no `#` straight to `###`).
  Don't build heading scaffolding — `## Summary`, `## Details`, `## Conclusion` — around content short enough to just say.

## Applies everywhere text is produced

Not a chat-reply rule.
Governs every piece of text an agent writes: conversational responses, markdown files, code comments, docstrings.
Load this skill first, before any skill that writes user-facing text.
