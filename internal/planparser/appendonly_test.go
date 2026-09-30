// appendonly_test.go covers CheckAppendOnly over parsed fixture plans written to a temp directory.

package planparser_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
)

// appendOnlyOverview renders a plan overview with the given frontmatter approval flag, framing paragraph, verify command and card count;
// each card i is `i — cN — card N`.
func appendOnlyOverview(approved bool, framing, verify string, cards int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\nformat: 5\napproved: %t\n---\n\n# Plan: rework\n\n%s\n\n## Card Index\n\n", approved, framing)
	for i := 1; i <= cards; i++ {
		fmt.Fprintf(&b, "%d — c%d — card %d\n", i, i, i)
	}
	if verify != "" {
		fmt.Fprintf(&b, "\n## verify:\n\n%s\n", verify)
	}
	return b.String()
}

// appendOnlyPlan writes and parses a plan whose card intents are given by intents.
func appendOnlyPlan(t *testing.T, approved bool, framing, verify string, intents []string) *planparser.Plan {
	t.Helper()

	files := map[string]string{"00-overview.md": appendOnlyOverview(approved, framing, verify, len(intents))}
	for i, intent := range intents {
		n := i + 1
		card := fmt.Sprintf("# Card %d — c%d\n\n**Edit:**\n- `a%d.go`\n**Intent:** %s\n", n, n, n, intent)
		files[fmt.Sprintf("%02d-c%d.md", n, n)] = card
	}
	dir := writePlanFiles(t, files)

	plan, err := planparser.ParsePlan(dir)
	if err != nil {
		t.Fatalf("ParsePlan(%q) error = %v; want nil", dir, err)
	}
	return plan
}

func TestCheckAppendOnly(t *testing.T) {
	t.Parallel()

	const framing = "Framing paragraph."
	const verify = "go test ./..."
	baseIntents := []string{"first intent.", "second intent."}

	tests := []struct {
		name     string
		extended func(t *testing.T) *planparser.Plan
		want     []string
	}{
		{
			name: "one appended card",
			extended: func(t *testing.T) *planparser.Plan {
				return appendOnlyPlan(t, true, framing, verify, append(append([]string{}, baseIntents...), "third."))
			},
			want: nil,
		},
		{
			name: "two appended cards",
			extended: func(t *testing.T) *planparser.Plan {
				return appendOnlyPlan(t, true, framing, verify, append(append([]string{}, baseIntents...), "third.", "fourth."))
			},
			want: nil,
		},
		{
			name: "edited existing card intent",
			extended: func(t *testing.T) *planparser.Plan {
				return appendOnlyPlan(t, true, framing, verify, []string{"first intent.", "edited intent.", "third."})
			},
			want: []string{"card 2 (c2) differs from its base form"},
		},
		{
			name: "flipped approved",
			extended: func(t *testing.T) *planparser.Plan {
				return appendOnlyPlan(t, false, framing, verify, append(append([]string{}, baseIntents...), "third."))
			},
			want: []string{"frontmatter approved changed from true to false"},
		},
		{
			name: "edited verify section",
			extended: func(t *testing.T) *planparser.Plan {
				return appendOnlyPlan(t, true, framing, "go vet ./...", append(append([]string{}, baseIntents...), "third."))
			},
			want: []string{"verify section changed"},
		},
		{
			name: "edited framing",
			extended: func(t *testing.T) *planparser.Plan {
				return appendOnlyPlan(t, true, "Changed framing.", verify, append(append([]string{}, baseIntents...), "third."))
			},
			want: []string{"framing section changed"},
		},
		{
			name: "no new card",
			extended: func(t *testing.T) *planparser.Plan {
				return appendOnlyPlan(t, true, framing, verify, baseIntents)
			},
			want: []string{"no card was appended"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			base := appendOnlyPlan(t, true, framing, verify, baseIntents)
			got := planparser.CheckAppendOnly(base, tt.extended(t))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CheckAppendOnly = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestCheckAppendOnly_MissingBaseCard(t *testing.T) {
	t.Parallel()

	base := appendOnlyPlan(t, true, "Framing paragraph.", "", []string{"one.", "two."})
	extended := appendOnlyPlan(t, true, "Framing paragraph.", "", []string{"one."})

	got := planparser.CheckAppendOnly(base, extended)
	want := []string{"card 2 (c2) is missing from the extended plan", "no card was appended"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CheckAppendOnly = %v; want %v", got, want)
	}
}
