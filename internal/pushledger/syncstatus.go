// syncstatus.go renders the one-line sync field of a status bar from the ledger.

package pushledger

import (
	"fmt"
	"time"
)

// SyncStatus returns the hub's sync field, `sync: <minutes>m · <seconds>s`, at now.
// The minutes are those since the hub last had no owed entry, zero while none is owed.
// The seconds are the longest push interval in force across the owed entries, or base while none is owed.
// An entry that is held and not owed counts towards neither.
// An unreadable ledger is returned as its error.
func SyncStatus(hubPath string, now time.Time, base time.Duration) (string, error) {
	entries, err := List(hubPath)
	if err != nil {
		return "", err
	}
	owed := false
	var longest time.Duration
	for _, entry := range entries {
		if !entry.Owed() {
			continue
		}
		owed = true
		longest = max(longest, time.Duration(entry.IntervalS)*time.Second)
	}
	minutes := 0
	if owed {
		cleared, err := LastClear(hubPath)
		if err != nil {
			return "", err
		}
		if !cleared.IsZero() {
			minutes = int(now.Sub(cleared) / time.Minute)
		}
	}
	if longest == 0 {
		longest = base
	}
	return fmt.Sprintf("sync: %dm · %ds", minutes, int(longest/time.Second)), nil
}
