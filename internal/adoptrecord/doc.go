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
// `Covers` reports whether the record holds a subject under adopt:
// the record is unfinished, lists the subject, and records no owner write of it that stood.
// `TakesUp` reports whether an unfinished record whose check step is not done takes up a worktree.
// It does when the record lists the subject the worktree reads its owner from;
// `lym adopt --here` then runs the record's request.
// Unknown keys of the file are tolerated on read and kept on write.
//
// `Watch` is the read-only under-adopt accessor the orch watcher and the dobby are handed.
// It answers whether an adopt in flight on this machine covers a worktree.
// Neither the watcher nor the dobby stops or hands over a worktree another machine owns while the takeover runs here.
// A worktree's subject is the entry whose owner it reads, resolved up the parent chain to `root`.
// A beam with no owner of its own is therefore covered by the adopt that lists it.
// Failing that, the adopt of the subject above it covers it.
// It reads the record without a lock.
// A record or board that does not read counts as no adopt, logged once per episode.
package adoptrecord
