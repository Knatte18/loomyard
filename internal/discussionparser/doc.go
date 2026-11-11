// Package discussionparser is the sole reader and sole writer of the on-disk format of `_lym/discussion/`:
// the decision record's required H2 sections, the support log's existence and the relevant list's format.
// It also holds the two write paths into the record.
//
// CheckRecordSections runs Validate's record checks on a record's bytes alone, with no support log:
// the required H2 sections and the `## PATTERN changes` bullets.
// The board's `upsert --record-file` runs it, with CheckDecisionIDs, on a record it stores apart from any discussion.
//
// CheckRelevant checks a present relevant list, the Discussion-Write lead's list of the code it read.
// It runs apart from the decision record's check.
//
// AppendDecision adds a post-Discussion design call to the record's `## Decisions` section.
// When the call carries a PATTERN change, it also writes that change to the `## PATTERN changes` section:
// the change replaces the bullet for the same entry ID when there is one, and is appended otherwise.
// A call can instead withdraw a declared change:
// it strikes the entry's bullet and closes its added entry with a `Withdraws:` line.
// CheckAddedDecisionsKept reads that line as a strike rather than a drop.
// AppendDecisions adds several such calls in one write and one discussion check.
//
// CheckDecisionIDs checks the record's decision headings and their acceptance coverage.
// Each H3 under `## Decisions` reads `### D<n> — <title>` with a unique id,
// and each id appears as a whole word in an `## Acceptance criteria` bullet.
// It runs beside Validate in the discussion gates and `validate-discussion`, never inside Validate or AppendDecision,
// so a record with older decision headings can still be appended to.
// DecisionIDs reads the record's decision ids for other packages.
//
// Citations returns every inline code span of the record, with its line and any following ` (new)` or ` (removed)` mark.
// It judges none of them: judging a citation against the tree belongs to internal/loomshed.
// It skips fenced blocks and carry-over blocks.
// It also skips `### Added after Discussion` entries and appended `## PATTERN changes` bullets.
// No writer authors those parts.
//
// WriteCarryOver keeps one review segment's carry-over entry in the `## Open risks` section.
// The entry holds the findings still open when the segment closed.
// IdentityText returns the record without the carry-over blocks and added-decision entries that the write paths add,
// so a review segment's re-entry can tell a changed record from one that only gained them.
// SectionBullets returns the top-level bullets of one H2 section, each with its continuation lines.
// It reads outside fenced blocks and carry-over blocks.
//
// SplicePart cuts the record of one part out of the Sketch's record.
// It keeps the decisions the part builds with, every added entry, and the constraints and assumptions whole.
// It also keeps the risks, criteria and PATTERN entries the part carries, and the goal and scope the caller renders.
// MapPatternChanges rewrites declared PATTERN changes as `changes` bullets of the live entries that absorb them.
// It serves a record filed from the Sketch's.
// CheckAddedDecisionsKept reports the added-decision entries and appended PATTERN change bullets a record dropped.
// It compares against any earlier version of the record, read from told bytes.
//
// CheckCarryOversKept reports the carry-over blocks a replacement record drops, changes or adds.
// CheckAddedEntriesSubset reports the added entries it inserts or rewrites.
// Together they let a whole-record replacement drop added entries and appended PATTERN bullets.
// The replacement must keep every carry-over block and insert or rewrite no added entry.
//
// The record may carry an optional `## PATTERN changes` section.
// It lists the PATTERN entries the task adds, changes or removes.
// Validate checks its bullets through ParsePatternChanges.
// It refuses an `adds` or `changes` bullet whose rule text would run past the PATTERN format checker's rule-text cap.
// Rule text is the entry text without a trailing `(test)` or background link.
// An overrun thus fails at Discussion and at `lym loom decision add` rather than first at the plan gate.
// DeclaredPatternChanges returns the declared changes rendered for the directives that show them to an agent.
//
// The package takes told absolute paths and declares no on-disk location of its own.
// Only loomengine's accessors declare where `_lym/discussion/` is:
// DiscussionDecisionRecord, DiscussionSupportLog and DiscussionRelevant.
// Those accessors take a *lymcwd.Location, which this stdlib-only leaf may not import.
// Staying stdlib-only keeps this a Tier 1 leaf any consumer can reuse for section parsing without geometry resolution.
package discussionparser
