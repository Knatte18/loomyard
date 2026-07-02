// checksum.go ports layoutChecksum verbatim from internal/muxpoccli/cmd.go:
// tmux's window-layout checksum, a 16-bit rotate-right-1 accumulator over the
// layout body bytes. This is the psmux-verified half of layout mechanics; it
// must stay byte-for-byte identical to muxpoc so a rendered layout continues
// to be accepted by tmux's select-layout.

package render

import "fmt"

// layoutChecksum computes the tmux window-layout checksum for s (the layout
// string following the leading "csum," field), returned as four lowercase
// hex digits. Matches tmux's layout_checksum: a 16-bit rotate-right
// accumulator.
func layoutChecksum(s string) string {
	var csum uint16
	for i := 0; i < len(s); i++ {
		csum = (csum >> 1) | ((csum & 1) << 15)
		csum += uint16(s[i])
	}
	return fmt.Sprintf("%04x", csum)
}
