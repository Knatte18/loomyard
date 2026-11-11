// Package adoptrecord owns the adopt record: the JSON file in which a `lym adopt` worker records its work.
// It holds the request the worker runs, the steps it has ended and the inputs the later steps need.
//
// The record is `adopt.json` directly under the hub's adopt scratch directory, and `adopt.lock` sits beside it.
// `Path` and `LockPath` name them.
// `Write` replaces the record atomically under its own state lock,
// and `Read` reads it without any lock, so a status view never waits on the worker.
// `Read` of an absent record creates no directory or lock file.
// The worker and `lym adopt --discard` take `adopt.lock` themselves; nothing here does.
//
// A `Record` is a pure value.
// `ReadState` tells a running record whose worker is gone from a live one,
// and `ResumeStep` names the step a resume restarts at.
// Unknown keys of the file are tolerated on read and kept on write.
package adoptrecord
