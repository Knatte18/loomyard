// doc.go carries the package-level doc comment for internal/hubregistry.

// Package hubregistry keeps the machine record of hubs: which hub path holds which shortname on this machine.
//
// A hub records itself when its creel daemon starts, and a sender's daemon resolves a receiving hub's shortname to its path through the record.
// Every call is told the directory holding the record, so a test's record lives in the test's own directory.
// A hub's Bolt shortname record decides what an entry still means; the record is a cache of paths that a lookup always checks against it.
package hubregistry
