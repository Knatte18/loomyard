// shipped_test.go drives the shipped layer's decoding checks.

package heddleengine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// recoverPanic returns the message decodeShipped panicked with on data, or "" when it did not panic.
func recoverPanic(data string) (message string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			message = fmt.Sprint(recovered)
		}
	}()
	decodeShipped([]byte(data))
	return ""
}

func TestDecodeShipped(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name string
		data string
		want string
	}{
		{"MalformedJSON", `{"entries": [`, "does not decode"},
		{"InvalidEntry", `{"entries": [{"slug": "a", "text": "t", "order": 1, "targets": []}]}`, "invalid entry"},
		{"DuplicateSlug", `{"entries": [
			{"slug": "a", "text": "t", "order": 1, "targets": ["claude-file"]},
			{"slug": "a", "text": "u", "order": 2, "targets": ["claude-file"]}]}`, `"a" twice`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := recoverPanic(tt.data); !strings.Contains(got, tt.want) {
				t.Errorf("decodeShipped panicked with %q; want a panic naming %q", got, tt.want)
			}
		})
	}
}
