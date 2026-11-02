package output_test

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSummaryWriter pins that an envelope line reaches the writer as its summary, however the writes split it, and that every other byte passes through as written.
func TestSummaryWriter(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name        string
		writes      []string
		wantBefore  string
		wantFlushed string
	}{
		{
			name:        "EnvelopeSplitAcrossWrites_TextAround",
			writes:      []string{"prompt: ", "answer\n{\"ok\":", "true,\"slug\":\"x\"}\n", "tail\n"},
			wantBefore:  "prompt: answer\nok\nslug: x\ntail\n",
			wantFlushed: "prompt: answer\nok\nslug: x\ntail\n",
		},
		{
			name:        "BraceLineNoEnvelope_AsWritten",
			writes:      []string{"{not json\n", "a {\"ok\":true}\n"},
			wantBefore:  "{not json\na {\"ok\":true}\n",
			wantFlushed: "{not json\na {\"ok\":true}\n",
		},
		{
			name:        "UnterminatedEnvelope_HeldUntilFlush",
			writes:      []string{`{"ok":true}`},
			wantBefore:  "",
			wantFlushed: "ok\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			writer := output.NewSummaryWriter(&out)
			for _, write := range tt.writes {
				if n, err := writer.Write([]byte(write)); err != nil || n != len(write) {
					t.Fatalf("Write(%q) = %d, %v; want %d, nil", write, n, err, len(write))
				}
			}
			if got := out.String(); got != tt.wantBefore {
				t.Errorf("before Flush = %q; want %q", got, tt.wantBefore)
			}
			if err := writer.Flush(); err != nil {
				t.Fatalf("Flush() = %v; want nil", err)
			}
			if got := out.String(); got != tt.wantFlushed {
				t.Errorf("after Flush = %q; want %q", got, tt.wantFlushed)
			}
		})
	}
}
