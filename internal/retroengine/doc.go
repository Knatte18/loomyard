// Package retroengine plans a retro:
// the landed runs it covers, the stamp that names it and the board task that audits them.
//
// A retro is one darn task.
// Its writer runs the retro's scripts over the runs' records and transcripts.
// It reads what their figures point at and writes a report of proposals under the retro report directory.
// The task's slug and the report's directory share one name, the slug prefix and the UTC end of the window covered.
// So the newest such name, among the report directories and the board's entries, is the last retro,
// and a retro still in flight counts as one.
//
// The engine reads nothing itself: a caller hands it the landed runs, the names to search for the last retro,
// and the stencils directory the task body renders from.
package retroengine
