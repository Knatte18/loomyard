// policy.go implements ValidateDisplay, the check a strand's display passes before reed stores it.

package render

import "fmt"

// ValidateDisplay refuses a display reed cannot place.
// That is an own-window display with a negative WindowSize; every other display passes.
// The error names the display's anchor.
func ValidateDisplay(d Display) error {
	if d.Anchor == AnchorOwnWindow && d.WindowSize < 0 {
		return fmt.Errorf("anchor %q: windowSize %d must not be negative", d.Anchor, d.WindowSize)
	}
	return nil
}
