// doc.go carries the package-level doc comment for internal/pushledger.

// Package pushledger keeps a hub's machine-local ledger of owed pushes: one entry per push target.
//
// The ledger sits under the Bolt checkout's never-tracked directory, one JSON file per target,
// and every write holds one ledger lock.
// An entry is owed while it holds landings or a first-owed time.
// An entry holding only held records, dropped landings or owed hand-overs is held, not owed.
// No push ends such an entry: it waits for its pair to go, its commit to leave the parent or its hand-over to succeed.
// A landing stands until the push of its parent branch settles or drops it.
// The standing entry is therefore the pending record of that landing.
//
// Beside the entries the ledger keeps the time the hub last had no owed target,
// so an owed period counts from its first owed entry.
// A missing ledger directory reads as an empty ledger.
//
// The package is a leaf: it imports the state, lock and geometry-name packages and the standard library.
package pushledger
