// Package discussionparser is the sole reader and sole writer of the on-disk format of `_lym/discussion/`:
// the decision record's required H2 sections, the support log's existence and the relevant list's format.
// It also holds the two write paths into the record.
//
// CheckRecordSections runs Validate's record checks on a record's bytes alone, with no support log.
// The board's `upsert --record-file` runs it, with CheckDecisionIDs, on a record it stores apart from any discussion.
// CheckRelevant checks a present relevant list, the Discussion-Write lead's list of the code it read.
//
// AppendDecisions adds post-Discussion design calls to the record's `## Decisions` section in one write and one check.
// When the call carries a PATTERN change, it also writes that change to the `## PATTERN changes` section:
// the change replaces the bullet for the same entry ID when there is one, and is appended otherwise.
// A call can instead withdraw a declared change:
// it strikes the entry's bullet and closes its added entry with a `Withdraws:` line.
// CheckAddedDecisionsKept reads that line as a strike rather than a drop.
// A call can also drop plan Create targets:
// the entry then carries one `Drops Create: `<target>“ line per target after its Rationale paragraph.
// DroppedCreates returns each dropped target with the title of the entry that drops it.
//
// CheckDecisionIDs checks the record's decision headings and their acceptance coverage.
// Each H3 under `## Decisions` reads `### D<n> — <title>` with a unique id,
// and each id appears as a whole word in an `## Acceptance criteria` bullet.
// It runs beside Validate in the discussion gates and `validate-discussion`, never inside Validate or AppendDecisions,
// so a record with older decision headings can still be appended to.
//
// Citations returns every inline code span of the record, with its line and any following ` (new)` or ` (removed)` mark.
// It judges none of them: judging a citation against the tree belongs to internal/loomshed.
// It skips fenced blocks and carry-over blocks.
// It also skips `### Added after Discussion` entries and appended `## PATTERN changes` bullets.
//
// WriteCarryOver keeps one review segment's carry-over entry in the `## Open risks` section.
// The entry holds the findings still open when the segment closed.
// IdentityText returns the record without the carry-over blocks and added-decision entries that the write paths add,
// so a review segment's re-entry can tell a changed record from one that only gained them.
//
// CheckAddedDecisionsKept reports the added-decision entries and appended PATTERN change bullets a record dropped.
// It compares against any earlier version of the record, read from told bytes.
// CheckCarryOversKept reports the carry-over blocks a replacement record drops, changes or adds.
// CheckAddedEntriesSubset reports the added entries it inserts or rewrites.
// Together they let a whole-record replacement drop added entries and appended PATTERN bullets.
// The replacement must keep every carry-over block and insert or rewrite no added entry.
//
// The record may carry an optional `## PATTERN changes` section.
// It lists the PATTERN entries the task adds, changes or removes.
// Validate checks its bullets through ParsePatternChanges.
// It refuses an `adds` or `changes` bullet whose rule text would run past the PATTERN format checker's rule-text cap,
// so an overrun fails at Discussion and at `lym loom decision add` rather than first at the plan gate.
//
// The package takes told absolute paths and declares no on-disk location of its own.
// Only loomengine's accessors declare where `_lym/discussion/` is.
// Those accessors take a *lymcwd.Location, which this stdlib-only leaf may not import.
// Staying stdlib-only keeps this a Tier 1 leaf any consumer can reuse for section parsing without geometry resolution.
package discussionparser
