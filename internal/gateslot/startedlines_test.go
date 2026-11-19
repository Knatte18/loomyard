// startedlines_test.go covers the started line's round trip through its sole parser.

package gateslot_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/gateslot"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestStartedLines(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   []gateslot.StartedRun
	}{
		{
			name:   "FormattedLineParsesBack",
			output: gateslot.FormatStartedLine("tier2", 4),
			want:   []gateslot.StartedRun{{Run: "tier2", Started: 4}},
		},
		{
			name:   "ZeroCountParsesAsZero",
			output: gateslot.FormatStartedLine("tier0 copy 2", 0),
			want:   []gateslot.StartedRun{{Run: "tier0 copy 2", Started: 0}},
		},
		{
			name: "OtherLinesAreSkipped",
			output: strings.Join([]string{
				"gate test: tier0 diff discovered 1 passed 1 failed 0 skipped 0 wall 0.1s exit 0 log x",
				gateslot.FormatStartedLine("tier1 copy 1", 3),
				"gate test: started many test binaries in tier1",
				gateslot.FormatStartedLine("tier1 copy 2", 2),
			}, "\n"),
			want: []gateslot.StartedRun{{Run: "tier1 copy 1", Started: 3}, {Run: "tier1 copy 2", Started: 2}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := gateslot.ParseStartedLines(tt.output); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseStartedLines(%q) = %v; want %v", tt.output, got, tt.want)
			}
		})
	}
}
