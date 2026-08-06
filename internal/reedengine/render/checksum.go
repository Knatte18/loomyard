// checksum.go computes tmux's window_layout checksum: a 16-bit rotate-right-1 accumulator over the
// layout body bytes.
// This is the psmux-verified half of layout mechanics;
// it must stay byte-for-byte identical to the tmux/psmux checksum algorithm so a rendered layout
// continues to be accepted by tmux's select-layout.

package render

import "fmt"

// layoutChecksum computes the tmux window-layout checksum (16-bit rotate-right).
// Returns four lowercase hex digits.
func layoutChecksum(s string) string {
	var csum uint16
	for i := 0; i < len(s); i++ {
		csum = (csum >> 1) | ((csum & 1) << 15)
		csum += uint16(s[i])
	}
	return fmt.Sprintf("%04x", csum)
}
