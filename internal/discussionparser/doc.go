// Package discussionparser is the SOLE reader of `_lyx/discussion/`'s on-disk format: the decision
// record's required H2 sections and the support log's existence.
// It takes told absolute paths and declares no on-disk location of its own — loomengine's
// DiscussionDecisionRecord/DiscussionSupportLog accessors remain the sole declarers of where
// `_lyx/discussion/` is, because those accessors take a *lyxcwd.Location, which this stdlib-only
// leaf may not import.
// Staying stdlib-only is deliberate: it keeps this package a Tier 1 leaf any consumer can reuse for
// section parsing without dragging geometry resolution in along with it.
package discussionparser
