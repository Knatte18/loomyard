//go:build tier0

// snapshot_test.go covers BoardEntrySnapshot's rendering of a board entry.

package darnengine

import "testing"

func TestBoardEntrySnapshot(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                     string
		slug, title, brief, body string
		want                     string
	}{
		{
			name: "AllParts", slug: "one-perch", title: " One perch ", brief: "Merge the perches.", body: "## Idea\n\nOne perch.\n",
			want: "# Board entry `one-perch`: One perch\n\nMerge the perches.\n\n## Idea\n\nOne perch.\n",
		},
		{
			name: "BlankBriefLeftOut", slug: "one-perch", title: "One perch", brief: "  ", body: "Body.",
			want: "# Board entry `one-perch`: One perch\n\nBody.\n",
		},
		{
			name: "TitleOnly", slug: "one-perch", title: "One perch",
			want: "# Board entry `one-perch`: One perch\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := string(BoardEntrySnapshot(tt.slug, tt.title, tt.brief, tt.body)); got != tt.want {
				t.Errorf("BoardEntrySnapshot(%q, %q, %q, %q) = %q; want %q", tt.slug, tt.title, tt.brief, tt.body, got, tt.want)
			}
		})
	}
}
