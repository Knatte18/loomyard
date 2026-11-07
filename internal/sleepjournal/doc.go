// Package sleepjournal reads the machine's suspends and hibernates from systemd-sleep's journal entries.
// Args is the journalctl argument list that prints them, and Parse turns the output into sleep intervals.
// Read maps one journalctl run, its output, standard error and exit error, to the sleeps.
// When none were read, it gives a clause saying why, and it says whether the journal was read at all.
// A journal the user may not read, which journalctl signals on standard error with exit status 0, is unread.
// The package spawns no process: each caller runs journalctl itself and wraps the clause in its own note.
// It imports the standard library only.
package sleepjournal
