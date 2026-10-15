// Package sleepjournal reads the machine's suspends and hibernates from systemd-sleep's journal entries.
// Args is the journalctl argument list that prints them, Parse turns the output into sleep intervals, and Read maps one journalctl run to the sleeps or to a clause saying why none were read.
// It spawns no process: each caller runs journalctl itself and wraps the clause in its own note.
// It imports the standard library only.
package sleepjournal
