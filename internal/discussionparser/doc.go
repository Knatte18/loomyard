// Package discussionparser is the SOLE reader of `_lyx/discussion/`'s on-disk format: the decision
// record's required H2 sections and the support log's existence.
// It takes told absolute paths and declares no on-disk location of its own — loomengine's
// DiscussionDecisionRecord/DiscussionSupportLog accessors remain the sole declarers of where
// `_lyx/discussion/` is, because those accessors take a *lyxcwd.Location, which this stdlib-only
// leaf may not import.
// Staying stdlib-only is deliberate: a future Plan-Sweep consumer can reuse this package's section
// parsing without dragging any geometry resolution in along with it.
package discussionparser
