// doc.go carries the package godoc for raddleparser.

// Package raddleparser is the sole parser of the raddle proposal, `_lym/raddle/proposal.yaml`.
// The format is the one `contracts/specs/raddle-proposal-spec.md` pins;
// the package is also the sole home of the checks that judge the proposal.
// It imports the standard library and internal/discussionparser only;
// the Sketch's decision record is read through internal/discussionparser alone.
//
// Parse refuses an unknown key.
// Hash is the approval hash, the hex SHA-256 of the file's bytes.
//
// Check is pure.
// It judges a Proposal against the decision record's bytes and a BoardContext the caller builds.
// It returns one Finding per broken rule of the spec.
// The board's slug rules reach it as a function in the BoardContext,
// so a slug the board would refuse fails before approval.
// A BoardContext with Perch set is a perch-mode run's, whose split perch approves the proposal:
// Check then allows one task or one project whose slug is the run's own entry, which the slug rules exempt.
// Check refuses a part's `pattern_clusters` mapping the filing could not write.
// RecheckBoard is the second look at the board at approval and at filing.
package raddleparser
