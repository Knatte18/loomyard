// sleep.go reads the machine's suspend and hibernate intervals from the systemd journal,
// so the report can leave them out of its time figures.

package main

import (
	"bytes"
	"os/exec"

	"github.com/Knatte18/loomyard/internal/sleepjournal"
)

// sleepLeftOutPrefix starts the report line that says why no machine sleep is left out of the time figures.
const sleepLeftOutPrefix = "No machine sleep is left out of the time figures: "

// journalSleeps returns the machine's sleeps from the systemd journal and a report line saying where they came from.
// It returns no sleeps when journalctl is absent or fails, or when the user may not read the journal.
// A journal with no sleep in it gives no sleeps too, and the report line says why nothing is left out.
func journalSleeps() ([]Interval, string) {
	var stderr bytes.Buffer
	cmd := exec.Command("journalctl", sleepjournal.Args()...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	return sleepsFromJournal(out, stderr.Bytes(), err)
}

// sleepsFromJournal maps the output, standard error and exit error of a journalctl run to sleeps and a report line.
func sleepsFromJournal(out, stderr []byte, runErr error) ([]Interval, string) {
	read, clause, _ := sleepjournal.Read(out, stderr, runErr)
	if clause != "" {
		return nil, sleepLeftOutPrefix + clause + ".\n"
	}
	sleeps := make([]Interval, len(read))
	for i, sleep := range read {
		sleeps[i] = Interval{sleep.From, sleep.To}
	}
	return sleeps, "Machine sleep, read from the systemd journal, is left out of every time figure.\n"
}
