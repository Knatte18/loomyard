// Package discussionparser is the SOLE reader and SOLE writer of `_lyx/discussion/`'s on-disk format:
// the decision record's required H2 sections, the support log's existence and the relevant list's format, and the two write paths into the record.
// CheckRelevant checks a present relevant list, the Discussion-Write lead's list of the code it read, apart from the decision record's check.
// AppendDecision adds a post-Discussion design call to the record's `## Decisions` section.
// WriteCarryOver keeps one review segment's carry-over entry, the findings still open when the segment closed, in its `## Open risks` section.
// CheckAddedDecisionsKept reports the added-decision entries a record dropped that any earlier version of it held, read from told bytes.
// It takes told absolute paths and declares no on-disk location of its own.
// loomengine's DiscussionDecisionRecord, DiscussionSupportLog and DiscussionRelevant accessors remain the sole declarers of where `_lyx/discussion/` is.
// Those accessors take a *lyxcwd.Location, which this stdlib-only leaf may not import.
// Staying stdlib-only is deliberate: it keeps this package a Tier 1 leaf any consumer can reuse for section parsing without dragging geometry resolution in along with it.
package discussionparser
