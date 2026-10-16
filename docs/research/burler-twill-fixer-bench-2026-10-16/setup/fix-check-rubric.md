# Fix-check rubric

This rubric is for checking a fix you just made to a reviewed artifact, never for reviewing the artifact afresh.
The subject is the change set: every sentence, decision, card, row or code line this round's fixer added, removed or rewrote, and every place that change reaches.
Text the fix did not touch and does not reach is out of scope, however wrong it looks.

## Severity

- `BLOCKING`: the fix made the artifact wrong where it was right, or left a stated premise of a finding unresolved in a way that ships a wrong behaviour.
- `MEDIUM`: the fix contradicts a sibling, states a bound or behaviour the code does not hold, or names a step that cannot be carried out.
- `LOW`: the fix is right but leaves a sibling, a criterion or a cross-reference stale in a way a reader would notice.

## What counts as a finding

- A finding names the changed text it concerns (file and section, decision, card or line), what is wrong, and the evidence: a quote from the artifact, a `path:line` or glyph in the code, a `PATTERN` entry, or a test.
- A finding that only repeats a review finding the fixer already fixed is not a finding.
- A finding that proposes a further improvement the review never asked for, and that the fix did not make worse, is not a finding.
