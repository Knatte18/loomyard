// export_test.go covers the exported StripLeadingComment and TopLevelMarkers helpers, and pins the
// existing Fill unfilled-marker error as a regression against the unfilledTopLevelMarkers refactor
// that shares its AST-walking logic with TopLevelMarkers.

package stencil

import (
	"strings"
	"testing"
)

func TestStripLeadingComment(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "DropsLeadingBanner",
			text: "<!-- banner -->\nbody text",
			want: "body text",
		},
		{
			name: "NoLeadingComment",
			text: "body text with no banner",
			want: "body text with no banner",
		},
		{
			name: "UnterminatedBlock",
			text: "<!-- banner never closes\nbody text",
			want: "<!-- banner never closes\nbody text",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripLeadingComment(tt.text)
			if got != tt.want {
				t.Errorf("StripLeadingComment(%q) = %q; want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestTopLevelMarkers(t *testing.T) {
	template := []byte("{{.a}} and {{.b}} and {{.a}} again")

	got, err := TopLevelMarkers(template)
	if err != nil {
		t.Fatalf("TopLevelMarkers(%q) returned error: %v", template, err)
	}

	want := []string{"a", "b"}
	if !equalStrings(got, want) {
		t.Errorf("TopLevelMarkers(%q) = %v; want %v", template, got, want)
	}
}

func TestTopLevelMarkers_IgnoresLeadingBannerComment(t *testing.T) {
	template := []byte("<!-- ignored {{.hidden}} -->\n{{.visible}}")

	got, err := TopLevelMarkers(template)
	if err != nil {
		t.Fatalf("TopLevelMarkers(%q) returned error: %v", template, err)
	}

	want := []string{"visible"}
	if !equalStrings(got, want) {
		t.Errorf("TopLevelMarkers(%q) = %v; want %v", template, got, want)
	}
}

func TestTopLevelMarkers_UnparseableTemplate(t *testing.T) {
	template := []byte("{{.a")

	_, err := TopLevelMarkers(template)
	if err == nil {
		t.Fatalf("TopLevelMarkers(%q) returned nil error; want a parse error", template)
	}
	if !strings.Contains(err.Error(), "parse template:") {
		t.Errorf("TopLevelMarkers(%q) error = %q; want it to contain %q", template, err.Error(), "parse template:")
	}
}

func TestFill_RegressionUnfilledMarker(t *testing.T) {
	template := []byte("{{.missing}}")

	_, err := Fill(template, nil)
	if err == nil {
		t.Fatalf("Fill(%q, nil) returned nil error; want the unfilled-marker error", template)
	}
	if !strings.Contains(err.Error(), "unfilled top-level marker(s): missing") {
		t.Errorf("Fill(%q, nil) error = %q; want it to contain %q", template, err.Error(), "unfilled top-level marker(s): missing")
	}
}

// equalStrings reports whether a and b hold the same strings in the same order.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
