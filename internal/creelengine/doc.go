// Package creelengine is the engine of creel, the per-hub message daemon: a pool of mail and queued injects that agents post to and read from, delivered into agent panes when they are idle.
// The engine is told its Geometry and a delivery seam and derives no path of its own.
//
// # Storage
//
// The pool is never stored as state; it is folded from an append-only journal under Geometry.JournalDir.
// The journal holds one JSON-lines file per UTC day, named YYYY-MM-DD.jsonl.
// An append writes one line and syncs it before it returns, and a new day file opens with a high-water record of the largest id issued so far, so an id is never reused, also after retention deleted the files holding the largest ones.
//
// A reader ignores keys it does not know and a line whose event name it does not know, so a newer binary's records never stop an older one.
// A torn last line left by a crash is skipped with one warning; a malformed line anywhere else fails the load naming the file and line, since skipping it could drop a message silently.
// An inject with a sending event and no outcome folds as unconfirmed, so one in flight at a crash is never typed again.
//
// At a UTC day change the writer carries what is still live out of files older than seven days: it appends a carried record for each unread message, queued inject and live hold to the new day file, syncs, and only then deletes the old files.
// The fold is idempotent by id, so a crash between the carry and the deletion leaves both and folds to one entry.
package creelengine
