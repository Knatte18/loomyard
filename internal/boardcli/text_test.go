// text_test.go holds pure tests of the compact listing; nothing here touches disk or spawns a process.

package boardcli_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/boardcli"
	"github.com/Knatte18/loomyard/internal/boardengine"
)

func strPtr(s string) *string { return &s }

func TestRenderCompact_AlignsColumns(t *testing.T) {
	got := boardcli.RenderCompact([]boardengine.BriefTask{
		{Tier: 1, Type: "bug", Slug: "a", Title: "Short", Status: strPtr("active")},
		{Tier: 2, Type: "feature", Slug: "long-slug", Title: "Longer title", Status: strPtr("done")},
	})
	want := "1  bug      a          Short         [active]\n" +
		"2  feature  long-slug  Longer title  [done]\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderCompact_AlignsMultiByteTitles(t *testing.T) {
	got := boardcli.RenderCompact([]boardengine.BriefTask{
		{Tier: 1, Type: "bug", Slug: "a", Title: "Fix — now", Status: strPtr("active")},
		{Tier: 1, Type: "bug", Slug: "b", Title: "Plain one", Status: strPtr("done")},
	})
	want := "1  bug  a  Fix — now  [active]\n" +
		"1  bug  b  Plain one  [done]\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderCompact_StatusBracketOnlyWhenSet(t *testing.T) {
	got := boardcli.RenderCompact([]boardengine.BriefTask{
		{Tier: 1, Type: "bug", Slug: "a", Title: "One"},
		{Tier: 1, Type: "bug", Slug: "b", Title: "Two", Status: strPtr("")},
	})
	want := "1  bug  a  One\n1  bug  b  Two\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderCompact_PreservesInputOrder(t *testing.T) {
	got := boardcli.RenderCompact([]boardengine.BriefTask{
		{Tier: 3, Type: "bug", Slug: "z", Title: "Z"},
		{Tier: 1, Type: "bug", Slug: "a", Title: "A"},
	})
	want := "3  bug  z  Z\n1  bug  a  A\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderCompact_EmptyInputPrintsNothing(t *testing.T) {
	if got := boardcli.RenderCompact(nil); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
