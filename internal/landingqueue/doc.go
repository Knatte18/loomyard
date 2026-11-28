// doc.go carries the package documentation for internal/landingqueue.

// Package landingqueue keeps the landing queue: the runs waiting to land on one parent branch.
// The run that takes the parent branch's lock reads the queue and lands the waiting runs together.
//
// The queue lives in one directory under the hub's board-level never-tracked directory, as Dir returns it.
// Each parent branch has a subdirectory named by the hex of its name.
// It holds one JSON entry per queued run, named by the hex of the run's slug, and at most one move mark.
// The move mark records a move of the parent branch that is under way,
// so a landing that the mover died before recording is recorded by the next backstop.
//
// An entry belongs to the process that wrote it, judged by its pid and start time,
// so a recycled pid is not taken for its dead predecessor.
// An entry whose owner is dead and that has no outcome is pruned.
//
// The directory is a cache that is never tracked: an absent directory, entry or mark reads as none.
// Every write keeps the keys of the file it replaces that this version does not know.
package landingqueue
