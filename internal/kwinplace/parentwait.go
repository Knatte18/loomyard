// parentwait.go waits for the parent window's placement to finish.

package kwinplace

import (
	"bytes"
	"context"
	"os"
	"time"
)

// The wait re-reads the places file from the floor, doubling to the ceiling while the file is unchanged.
const (
	parentPollFloor   = time.Second
	parentPollCeiling = 8 * time.Second
)

// awaitParent waits until the entry of parent in the places file at placesPath is settled, and reports whether it is.
// It gives up at deadline or when ctx ends.
func (f Flow) awaitParent(ctx context.Context, placesPath, parent string, deadline time.Time) bool {
	// An unreadable file compares as empty; ReadPlaces below logs why it cannot be read.
	contents := func() []byte {
		data, err := os.ReadFile(placesPath)
		if err != nil {
			return nil
		}
		return data
	}
	wait := parentPollFloor
	previous := contents()
	for {
		if entry, found := ReadPlaces(placesPath)[parent]; found && entry.Settled() {
			return true
		}
		remaining := deadline.Sub(f.now())
		if remaining <= 0 || ctx.Err() != nil {
			return false
		}
		f.sleep(ctx, min(wait, remaining))
		current := contents()
		if bytes.Equal(current, previous) {
			wait = min(2*wait, parentPollCeiling)
		} else {
			wait = parentPollFloor
		}
		previous = current
	}
}
