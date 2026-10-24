// Package discussionparser is the SOLE reader and SOLE writer of `_lyx/discussion/`'s on-disk format:
// the decision record's required H2 sections, the support log's existence and the relevant list's format, and the two write paths into the record.
// CheckRelevant checks a present relevant list, the Discussion-Write lead's list of the code it read, apart from the decision record's check.
// AppendDecision adds a post-Discussion design call to the record's `## Decisions` section, and also writes a PATTERN change to the `## PATTERN changes` section when the call carries one.
// That change replaces the section's bullet for the same entry ID when there is one and is appended otherwise.
// WriteCarryOver keeps one review segment's carry-over entry, the findings still open when the segment closed, in its `## Open risks` section.
// IdentityText returns the record without the carry-over blocks and added-decision entries those two paths write, so a review segment's re-entry tells a changed record from one that only gained them.
// CheckAddedDecisionsKept reports the added-decision entries and appended PATTERN change bullets a record dropped that any earlier version of it held, read from told bytes.
// The record may carry an optional `## PATTERN changes` section listing the PATTERN entries the task adds, changes or removes.
// Validate checks its bullets through ParsePatternChanges and refuses an `adds` or `changes` bullet whose PATTERN.md entry line would run past the PATTERN format checker's line cap, so the overrun fails at Discussion and at `lyx loom decision add` rather than first at the plan gate.
// DeclaredPatternChanges returns them rendered for the directives that show them to an agent.
// It takes told absolute paths and declares no on-disk location of its own.
// loomengine's DiscussionDecisionRecord, DiscussionSupportLog and DiscussionRelevant accessors remain the sole declarers of where `_lyx/discussion/` is.
// Those accessors take a *lyxcwd.Location, which this stdlib-only leaf may not import.
// Staying stdlib-only is deliberate: it keeps this package a Tier 1 leaf any consumer can reuse for section parsing without dragging geometry resolution in along with it.
package discussionparser
