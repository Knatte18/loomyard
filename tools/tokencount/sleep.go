// sleep.go reads the machine's suspends and hibernates from the systemd journal, the sleeps every time figure of the report leaves out.

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	// sleepEntered starts the message systemd-sleep logs right before the machine sleeps.
	sleepEntered = "Performing sleep operation "
	// sleepReturned starts the message systemd-sleep logs once the machine is awake again.
	sleepReturned = "System returned from sleep operation "
)

// journalSleeps returns the machine's sleeps from the systemd journal and a report line saying where they came from.
// With journalctl absent or failing, or no sleep in its journal, it returns no sleeps and a line saying why nothing is left out.
func journalSleeps() ([]Interval, string) {
	path, err := exec.LookPath("journalctl")
	if err != nil {
		return nil, "No machine sleep is left out of the time figures: journalctl is not on PATH.\n"
	}
	out, err := exec.Command(path, "-t", "systemd-sleep", "-o", "json", "--output-fields=MESSAGE,_BOOT_ID", "--no-pager").Output()
	if err != nil {
		return nil, fmt.Sprintf("No machine sleep is left out of the time figures: journalctl failed: %v.\n", err)
	}
	sleeps, err := parseSleepJournal(out)
	if err != nil {
		return nil, fmt.Sprintf("No machine sleep is left out of the time figures: %v.\n", err)
	}
	if len(sleeps) == 0 {
		return nil, "No machine sleep is left out of the time figures: the journal records no systemd-sleep operation.\n"
	}
	return sleeps, "Machine sleep, read from the systemd journal, is left out of every time figure.\n"
}

// journalEntry is the part of one `journalctl -o json` line parseSleepJournal reads.
type journalEntry struct {
	// Realtime is the entry's wall time in microseconds since the Unix epoch, as a decimal string.
	Realtime string `json:"__REALTIME_TIMESTAMP"`
	Boot     string `json:"_BOOT_ID"`
	// Message is a JSON string, or a byte array for a message that is not valid UTF-8.
	Message json.RawMessage `json:"MESSAGE"`
}

// parseSleepJournal reads systemd-sleep's journal entries, one JSON object per line in time order, into sleep intervals.
// A sleep runs from an entry that starts the sleep operation to the next entry that returns from the same operation in the same boot;
// a start with no such return, a return with no start and every other message add nothing.
// A line that does not decode, or a timestamp that does not parse, is an error.
func parseSleepJournal(data []byte) ([]Interval, error) {
	type sleepStart struct {
		at              time.Time
		boot, operation string
	}
	var sleeps []Interval
	var pending *sleepStart
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var entry journalEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			return nil, fmt.Errorf("journal line %q does not decode: %w", scanner.Text(), err)
		}
		var message string
		if json.Unmarshal(entry.Message, &message) != nil {
			continue
		}
		micros, err := strconv.ParseInt(entry.Realtime, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("journal timestamp %q does not parse: %w", entry.Realtime, err)
		}
		at := time.UnixMicro(micros)
		switch {
		case strings.HasPrefix(message, sleepEntered):
			pending = &sleepStart{at, entry.Boot, sleepOperation(message, sleepEntered)}
		case strings.HasPrefix(message, sleepReturned):
			if pending != nil && pending.boot == entry.Boot && pending.operation == sleepOperation(message, sleepReturned) {
				sleeps = append(sleeps, Interval{pending.at, at})
			}
			pending = nil
		}
	}
	return sleeps, scanner.Err()
}

// sleepOperation returns the quoted operation name, such as hibernate, that follows prefix in message.
func sleepOperation(message, prefix string) string {
	operation, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(message, prefix), "'"), "'")
	return operation
}
