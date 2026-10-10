// export_test.go covers the exported StripLeadingComment, TopLevelMarkers and IncludeNames helpers.

package stencil

import (
	"strings"
	"testing"
)

func TestStripLeadingComment(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			got := StripLeadingComment(tt.text)
			if got != tt.want {
				t.Errorf("StripLeadingComment(%q) = %q; want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestTopLevelMarkers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		template    string
		want        []string
		wantErrText string
	}{
		{
			name:     "DedupedInFirstSeenOrder",
			template: "{{.a}} and {{.b}} and {{.a}} again",
			want:     []string{"a", "b"},
		},
		{
			name:     "IgnoresLeadingBannerComment",
			template: "<!-- ignored {{.hidden}} -->\n{{.visible}}",
			want:     []string{"visible"},
		},
		{
			name:        "UnparseableTemplate",
			template:    "{{.a",
			wantErrText: "parse template:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := TopLevelMarkers([]byte(tt.template))
			if tt.wantErrText != "" {
				if err == nil {
					t.Fatalf("TopLevelMarkers(%q) returned nil error; want a parse error", tt.template)
				}
				if !strings.Contains(err.Error(), tt.wantErrText) {
					t.Errorf("TopLevelMarkers(%q) error = %q; want it to contain %q", tt.template, err.Error(), tt.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("TopLevelMarkers(%q) returned error: %v", tt.template, err)
			}
			if !equalStrings(got, tt.want) {
				t.Errorf("TopLevelMarkers(%q) = %v; want %v", tt.template, got, tt.want)
			}
		})
	}
}

func TestIncludeNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		template    string
		want        []string
		wantErrText string
	}{
		{
			name:     "NoIncludes",
			template: "{{.a}} and plain text",
			want:     nil,
		},
		{
			name:     "LeadingCommentSpellingAnIncludeReportsNone",
			template: "<!-- header {{template \"hidden\"}} -->\nbody",
			want:     nil,
		},
		{
			name:     "IncludesInDifferentBranchesSortedAndDeduped",
			template: "{{if .a}}{{template \"zeta\"}}{{else}}{{template \"alpha\"}}{{end}}{{with .b}}{{template \"zeta\"}}{{end}}",
			want:     []string{"alpha", "zeta"},
		},
		{
			name:        "UnparseableTemplate",
			template:    "{{template \"a\"",
			wantErrText: "parse template:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := IncludeNames([]byte(tt.template))
			if tt.wantErrText != "" {
				if err == nil {
					t.Fatalf("IncludeNames(%q) returned nil error; want a parse error", tt.template)
				}
				if !strings.Contains(err.Error(), tt.wantErrText) {
					t.Errorf("IncludeNames(%q) error = %q; want it to contain %q", tt.template, err.Error(), tt.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("IncludeNames(%q) returned error: %v", tt.template, err)
			}
			if !equalStrings(got, tt.want) {
				t.Errorf("IncludeNames(%q) = %v; want %v", tt.template, got, tt.want)
			}
		})
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
