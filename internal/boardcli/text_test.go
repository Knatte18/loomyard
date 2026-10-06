// text_test.go holds pure tests of the compact listing; nothing here touches disk or spawns a process.

package boardcli_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/boardcli"
	"github.com/Knatte18/loomyard/internal/boardengine"
)

func strPtr(s string) *string { return &s }

// TestRenderCompact asserts the compact listing aligns columns by display width, shows the status bracket only when a status is set,
// leaves no trailing gap without labels, keeps the input order and prints nothing for no input.
//
//testtiming:keep pins the column alignment, multi-byte widths, status bracket, label gap and ordering of the compact listing, which its covering test asserts for one listing only
func TestRenderCompact(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   []boardengine.BriefTask
		want string
	}{
		{
			name: "columns align",
			in: []boardengine.BriefTask{
				{Kind: "task", Labels: []string{"bug"}, Slug: "a", Title: "Short", Status: strPtr("active")},
				{Kind: "note", Labels: []string{"enhancement", "area"}, Slug: "long-slug", Title: "Longer title", Status: strPtr("done")},
			},
			want: "task  a          Short         bug               [active]\n" +
				"note  long-slug  Longer title  enhancement,area  [done]\n",
		},
		{
			name: "multi-byte titles align",
			in: []boardengine.BriefTask{
				{Kind: "task", Labels: []string{"bug"}, Slug: "a", Title: "Fix — now", Status: strPtr("active")},
				{Kind: "task", Labels: []string{"bug"}, Slug: "b", Title: "Plain one", Status: strPtr("done")},
			},
			want: "task  a  Fix — now  bug  [active]\n" +
				"task  b  Plain one  bug  [done]\n",
		},
		{
			name: "status bracket only when set",
			in: []boardengine.BriefTask{
				{Kind: "task", Labels: []string{"bug"}, Slug: "a", Title: "One"},
				{Kind: "task", Labels: []string{"bug"}, Slug: "b", Title: "Two", Status: strPtr("")},
			},
			want: "task  a  One  bug\ntask  b  Two  bug\n",
		},
		{
			name: "no labels leaves no trailing gap",
			in:   []boardengine.BriefTask{{Kind: "note", Slug: "a", Title: "One"}},
			want: "note  a  One\n",
		},
		{
			name: "input order is preserved",
			in: []boardengine.BriefTask{
				{Kind: "note", Labels: []string{"bug"}, Slug: "z", Title: "Z"},
				{Kind: "task", Labels: []string{"bug"}, Slug: "a", Title: "A"},
			},
			want: "note  z  Z  bug\ntask  a  A  bug\n",
		},
		{
			name: "empty input prints nothing",
			in:   nil,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := boardcli.RenderCompact(tt.in); got != tt.want {
				t.Fatalf("got:\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}
