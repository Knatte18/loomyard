// doc.go carries the package godoc for raddleparser.

// Package raddleparser is the sole parser of the raddle proposal, `_lym/raddle/proposal.yaml`, in the format `contracts/specs/raddle-proposal-spec.md` pins, and the sole home of the checks that judge it.
// It imports the standard library and internal/discussionparser only, and reads the Sketch's decision record through internal/discussionparser alone.
//
// The file is one object in YAML's JSON-compatible flow form, which Parse decodes with encoding/json; it refuses an unknown key by comparing each object's keys with the json tags of the type that receives it.
// Hash is the approval hash, the hex SHA-256 of the file's bytes.
//
// Check is pure: it judges a Proposal against the decision record's bytes and a BoardContext the caller builds, and returns one Finding per broken rule of the spec.
// A part's `pattern_clusters` map a PATTERN change of the Sketch onto the live entry that absorbs it: Check refuses a mapping the filing could not write, and Part.PatternEntryIDs names the ids the part's record declares.
// RecheckBoard is the second look at the board at approval and at filing, which fails a moved or incomplete board and names the weighed entries that have landed.
package raddleparser
