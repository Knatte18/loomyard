MILL_REVIEW_BEGIN
# Review: shed: land the producer-typology decision (atomicity carve-out) and sweep remaining doc contradictions

```yaml
verdict: APPROVE
reviewer_model: claude-sonnet-5
reviewer_self_id: Claude Sonnet 5 (Anthropic)
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Scope of this pass

Re-derived essentially every checkable citation in the document against the live tree at this worktree's branch point (`c3af3c9c`, confirmed an ancestor of `HEAD`) rather than trusting the doc's own "verified against the tree" claim: all cited `shed.md` lines (`:7,8,18,19,22–29,59–63`), including confirming the "two swappable slots" text really is gone from the file (only the dangling `:7`/`:19` back-references to it survive, exactly as claimed) and that `:18`'s "by *value*" is genuinely italicised (invisible to a plain grep, exactly as flagged); all cited `loom.md` lines (`:3,15–17,43,44,47–60,50,58,70–72,76–83`); `CONSTRAINTS.md`'s `## Batcher Registry+Config Invariant` at `:348–353` with `## GitHub Auth Invariant` immediately following at `:355`; `docs/overview.md`'s three `shed`/`glance` sites (`:283`, `:300–301`, `:329–331`); `hardener.md:17`; `roadmap.md:57–61,110`; and a wide set of `shed-followups.md` citations (`:3–5`, `:227–231`, `:327–330`, `:378–383`, `:409`, `:487`, `:491`, `:497–500`, `:504`, `:525`, `:529–532`, `:536–540`). All six commit hashes (`0149776a`/`80238b3f`/`2186ff53`/`ab3d67b1`/`e179ad0c`/`6b396aa1`) resolve to exactly the commits the doc names them as. Every one of these checked out exactly as cited.

Two trivial findings, both NIT, neither affecting any decision.

## Findings

### [NIT:consistency] Section E's span is off by one line

**Section:** Technical context, "Exact edit sites" (`shed-followups.md`), and Decisions → `shed-followups-supersession-block`
**Issue:** The doc states "section E spans `:409–550`" (used to scope where the supersession block goes and what stays untouched). `## E — shed-model-contradiction-sweep` is at `shed-followups.md:409` and the next section, `## F — batcher-standalone-split`, starts at `:552` — so section E's actual last line is `:551`, not `:550`.
**Fix:** Restate as `:409–551`, or state the boundary as "up to (not including) `## F`'s heading at `:552`" so the range doesn't need updating if F's own content shifts by a line.

### [NIT:consistency] `loom.md`'s open-questions residue is cited as two different ranges in the same document

**Section:** Scope → "In" (`loom.md — ... plus E's loom.md residue (:15–17, :78–83)`) vs. Technical context → "Exact edit sites" (`:76–83 — the open-questions paragraph`)
**Issue:** The same block of text (the open-questions paragraph plus task C's hand-off note) is cited as `:78–83` in Scope and `:76–83` in Technical context — a two-line discrepancy. Reading the actual lines, `:76–77` is the *already-resolved* first open question (`Discussion-Validate` closing it) and `:78` onward is the still-open second question this task inherits, so `:78–83` is arguably the more precise citation for "residue this task owns" and `:76–83` the fuller paragraph for context — this may be intentional differing granularity rather than an error, but nothing in the text says so.
**Fix:** Either align both to the same range, or add one clause noting the two-line difference is deliberate (context paragraph vs. owned residue).

## Verdict

APPROVE
Every line-numbered citation, both italicised and plain-text claims, and all six commit hashes checked and confirmed accurate against the live tree — including several claims (the deleted "two swappable slots" text, the invisible-to-grep `by *value*` wording, the stale `:289`/`:318` self-correction) that required careful verification rather than a quick eyeball. Two trivial NITs found, both citation-range slips with no effect on any decision.
MILL_REVIEW_END
