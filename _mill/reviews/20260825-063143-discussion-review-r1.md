# Review: loom: Discussion-Review producer

```yaml
verdict: APPROVE
reviewer_model: orchestrator
reviewed_file: _mill/discussion.md
date: 2026-08-25
```

## Findings

### [NIT:consistency] "producer table rows" update left unspecified in shape, unlike every other decision
**Section:** Scope → In: "Update `manifest/designs/loom.md` (producer table rows, the two rubric subsections' 'future `Bouncer` rubric' wording), and move the roadmap item to Done."
**Issue:** Every other change in this document pins an exact name, key, or value — but this bullet doesn't say whether the producer table's existing single `Discussion-Review` row (`Kind: bespoke`, `Type: LLM/review segment`) stays one abstract row (matching how the table already treats `Webster` as one black-box row hiding an internal fork loop) or becomes two literal rows for `Discussion-Bouncer`/`Discussion-Burler`. Nothing elsewhere in Decisions, Technical context, or Gotchas resolves it either.
**Suggested fix:** State the intended shape explicitly (a one-line addendum is enough), e.g. "the table keeps one row, renamed/relabeled to the segment, consistent with the `Webster` row's black-box framing" — or the reverse, if two rows is intended.

## Verdict

APPROVE
Scope, all nine decisions, constraint coverage, and failure modes (round-cap cost, silent-`OnDone` termination, coverage-guard regression, empty-rubric interpolation) are concretely specified and tightly grounded in cited code (`internal/shedadapters`, `internal/shedrecipe`'s existing entries, `internal/loomcli/wiring.go`'s exact TODO comment); the one open item is cosmetic and does not block plan writing.
