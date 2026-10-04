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
		{Kind: "task", Labels: []string{"bug"}, Slug: "a", Title: "Short", Status: strPtr("active")},
		{Kind: "note", Labels: []string{"enhancement", "area"}, Slug: "long-slug", Title: "Longer title", Status: strPtr("done")},
	})
	want := "task  a          Short         bug               [active]\n" +
		"note  long-slug  Longer title  enhancement,area  [done]\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderCompact_AlignsMultiByteTitles(t *testing.T) {
	got := boardcli.RenderCompact([]boardengine.BriefTask{
		{Kind: "task", Labels: []string{"bug"}, Slug: "a", Title: "Fix — now", Status: strPtr("active")},
		{Kind: "task", Labels: []string{"bug"}, Slug: "b", Title: "Plain one", Status: strPtr("done")},
	})
	want := "task  a  Fix — now  bug  [active]\n" +
		"task  b  Plain one  bug  [done]\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderCompact_StatusBracketOnlyWhenSet(t *testing.T) {
	got := boardcli.RenderCompact([]boardengine.BriefTask{
		{Kind: "task", Labels: []string{"bug"}, Slug: "a", Title: "One"},
		{Kind: "task", Labels: []string{"bug"}, Slug: "b", Title: "Two", Status: strPtr("")},
	})
	want := "task  a  One  bug\ntask  b  Two  bug\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderCompact_NoLabelsLeavesNoTrailingGap(t *testing.T) {
	got := boardcli.RenderCompact([]boardengine.BriefTask{
		{Kind: "note", Slug: "a", Title: "One"},
	})
	if want := "note  a  One\n"; got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderCompact_PreservesInputOrder(t *testing.T) {
	got := boardcli.RenderCompact([]boardengine.BriefTask{
		{Kind: "note", Labels: []string{"bug"}, Slug: "z", Title: "Z"},
		{Kind: "task", Labels: []string{"bug"}, Slug: "a", Title: "A"},
	})
	want := "note  z  Z  bug\ntask  a  A  bug\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderCompact_EmptyInputPrintsNothing(t *testing.T) {
	if got := boardcli.RenderCompact(nil); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
