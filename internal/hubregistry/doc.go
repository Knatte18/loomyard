// doc.go documents the hubregistry package.

// Package hubregistry keeps the machine record of hubs: which hub path holds which shortname on this machine.
//
// A hub records itself when its creel daemon starts.
// A sender's daemon resolves a receiving hub's shortname to its path through the record.
// Every call is told the directory holding the record, so a test's record lives in the test's own directory.
// A hub's Bolt shortname record decides what an entry still means;
// the record is a cache of paths that a lookup always checks against it.
//
// A hub's identity across machines is its repository name, owner/repo, read from its prime's origin remote;
// creel's trusted_hubs lists hubs by it, since a shortname resolves per machine and may be taken again.
// A hub with no origin remote, or a non-GitHub one, has no repository name.
package hubregistry
