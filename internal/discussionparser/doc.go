// Package discussionparser is the SOLE reader and SOLE writer of `_lym/discussion/`'s on-disk format:
// the decision record's required H2 sections, the support log's existence and the relevant list's format, and the two write paths into the record.
// CheckRelevant checks a present relevant list, the Discussion-Write lead's list of the code it read, apart from the decision record's check.
// AppendDecision adds a post-Discussion design call to the record's `## Decisions` section, and also writes a PATTERN change to the `## PATTERN changes` section when the call carries one.
// That change replaces the section's bullet for the same entry ID when there is one and is appended otherwise.
// AppendDecisions adds several such calls to the record in one write and one discussion check.
// CheckDecisionIDs checks the record's decision headings and their acceptance coverage: each H3 under `## Decisions` reads `### D<n> — <title>` with a unique id, and each id appears as a whole word in an `## Acceptance criteria` bullet.
// It runs beside Validate in the discussion gates and `validate-discussion`, never inside it, and not from AppendDecision, so a record whose decision headings predate it is still appended to.
// Citations returns every inline code span of the record, with its line and whether a ` (new)` or ` (removed)` mark follows it, and judges none of them: judging a citation against the tree is internal/loomshed's.
// It skips fenced blocks, `### Added after Discussion` entries, appended `## PATTERN changes` bullets and carry-over blocks, the parts no writer authors.
// WriteCarryOver keeps one review segment's carry-over entry, the findings still open when the segment closed, in its `## Open risks` section.
// IdentityText returns the record without the carry-over blocks and added-decision entries those two paths write, so a review segment's re-entry tells a changed record from one that only gained them.
// CheckAddedDecisionsKept reports the added-decision entries and appended PATTERN change bullets a record dropped that any earlier version of it held, read from told bytes.
// The record may carry an optional `## PATTERN changes` section listing the PATTERN entries the task adds, changes or removes.
// Validate checks its bullets through ParsePatternChanges and refuses an `adds` or `changes` bullet whose rule text, the entry text without a trailing `(test)` or background link, would run past the PATTERN format checker's rule-text cap,
// so the overrun fails at Discussion and at `lym loom decision add` rather than first at the plan gate.
// DeclaredPatternChanges returns them rendered for the directives that show them to an agent.
// It takes told absolute paths and declares no on-disk location of its own.
// loomengine's DiscussionDecisionRecord, DiscussionSupportLog and DiscussionRelevant accessors remain the sole declarers of where `_lym/discussion/` is.
// Those accessors take a *lymcwd.Location, which this stdlib-only leaf may not import.
// Staying stdlib-only is deliberate: it keeps this package a Tier 1 leaf any consumer can reuse for section parsing without dragging geometry resolution in along with it.
package discussionparser
