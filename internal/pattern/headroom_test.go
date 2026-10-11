// headroom_test.go covers Headroom, one row per growth outcome against the headroom line.

package pattern

import (
	"bytes"
	"fmt"
	"testing"
)

func TestHeadroom(t *testing.T) {
	t.Parallel()
	size := func(n int) []byte { return bytes.Repeat([]byte("x"), n) }
	finding := func(head, base, cut int) string {
		return fmt.Sprintf("overview grew to %d bytes from %d, past the %d-byte headroom line; cut %d bytes", head, base, HeadroomBytes, cut)
	}

	tests := []struct {
		name string
		base []byte
		head []byte
		// wantMessage is the finding's exact message; empty means the row passes.
		wantMessage string
	}{
		{"grew from under the line to above it", size(HeadroomBytes - 100), size(HeadroomBytes + 50), finding(HeadroomBytes+50, HeadroomBytes-100, 50)},
		{"grew from a base above the line", size(HeadroomBytes + 40), size(HeadroomBytes + 90), finding(HeadroomBytes+90, HeadroomBytes+40, 50)},
		{"grew to exactly the line", size(HeadroomBytes - 100), size(HeadroomBytes), ""},
		{"shrank while above the line", size(HeadroomBytes + 90), size(HeadroomBytes + 40), ""},
		{"unchanged above the line", size(HeadroomBytes + 40), size(HeadroomBytes + 40), ""},
		{"absent at the base and present above the line", nil, size(HeadroomBytes + 50), finding(HeadroomBytes+50, 0, 50)},
		{"absent at the head", size(HeadroomBytes + 40), nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, found := Headroom(tt.base, tt.head)
			if found != (tt.wantMessage != "") {
				t.Fatalf("Headroom found = %v (%+v), want %v", found, got, tt.wantMessage != "")
			}
			if !found {
				return
			}
			want := Finding{Kind: KindOverviewHeadroom, Subject: "PATTERN.md", Message: tt.wantMessage}
			if got != want {
				t.Errorf("Headroom = %+v, want %+v", got, want)
			}
		})
	}
}
