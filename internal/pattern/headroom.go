// headroom.go holds the pure growth check of the overview against the headroom line.

package pattern

import "fmt"

// Headroom reports a finding when the overview grew past HeadroomBytes.
// base and head are the overview's content at the base and at the head; a nil or empty base means the overview was absent there and counts as zero bytes.
// It answers true exactly when the head is longer than the base and longer than HeadroomBytes.
// The finding names the bytes to cut, the head size minus the larger of the base size and HeadroomBytes: the smallest cut that passes.
func Headroom(base, head []byte) (Finding, bool) {
	if len(head) <= len(base) || len(head) <= HeadroomBytes {
		return Finding{}, false
	}
	floor := max(len(base), HeadroomBytes)
	return Finding{
		Kind:    KindOverviewHeadroom,
		Subject: overviewFileName,
		Message: fmt.Sprintf("overview grew to %d bytes from %d, past the %d-byte headroom line; cut %d bytes", len(head), len(base), HeadroomBytes, len(head)-floor),
	}, true
}
