package orchengine

import (
	"strings"
	"testing"
)

func TestDecideStart(t *testing.T) {
	cases := []struct {
		strand, watcher bool
		want            StartAction
	}{
		{true, true, StartAttachOnly},
		{true, false, StartSpawnWatcher},
		{false, true, StartRelaunch},
		{false, false, StartRelaunch},
	}
	for _, c := range cases {
		if got := DecideStart(c.strand, c.watcher); got != c.want {
			t.Errorf("DecideStart(%v,%v) = %v, want %v", c.strand, c.watcher, got, c.want)
		}
	}
}

func existsOnly(paths ...string) func(string) bool {
	return func(p string) bool {
		for _, q := range paths {
			if p == q {
				return true
			}
		}
		return false
	}
}

func TestChooseStartPrompt_FlagWinsOverLastHandoff(t *testing.T) {
	dir := seedStencils(t)
	s := State{LastHandoff: "/h/last.md"}
	got, src, err := ChooseStartPrompt(dir, testRolePath, "/h/flag.md", s, existsOnly("/h/flag.md", "/h/last.md"))
	if err != nil {
		t.Fatal(err)
	}
	if src != SourceFlag || !strings.Contains(got, "/h/flag.md") || !strings.Contains(got, testRolePath) || strings.Contains(got, "/h/last.md") {
		t.Errorf("source=%q prompt=%q", src, got)
	}
}

func TestChooseStartPrompt_MissingFlagFileErrors(t *testing.T) {
	_, _, err := ChooseStartPrompt(seedStencils(t), testRolePath, "/h/gone.md", State{LastHandoff: "/h/last.md"}, existsOnly("/h/last.md"))
	if err == nil {
		t.Fatal("want error for missing flag file")
	}
}

func TestChooseStartPrompt_LastHandoffWhenPresent(t *testing.T) {
	got, src, err := ChooseStartPrompt(seedStencils(t), testRolePath, "", State{LastHandoff: "/h/last.md"}, existsOnly("/h/last.md"))
	if err != nil {
		t.Fatal(err)
	}
	if src != SourceLastHandoff || !strings.Contains(got, "/h/last.md") || !strings.Contains(got, testRolePath) {
		t.Errorf("source=%q prompt=%q", src, got)
	}
}

func TestChooseStartPrompt_LastHandoffGoneFallsToFresh(t *testing.T) {
	_, src, err := ChooseStartPrompt(seedStencils(t), testRolePath, "", State{LastHandoff: "/h/last.md"}, existsOnly())
	if err != nil {
		t.Fatal(err)
	}
	if src != SourceFresh {
		t.Errorf("source=%q, want fresh", src)
	}
}

func TestChooseStartPrompt_PendingHandoffIgnored(t *testing.T) {
	dir := seedStencils(t)
	s := State{PendingHandoff: "/h/partial.md"}
	got, src, err := ChooseStartPrompt(dir, testRolePath, "", s, existsOnly("/h/partial.md"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := RenderStartPrompt(dir, testRolePath)
	if err != nil {
		t.Fatal(err)
	}
	if src != SourceFresh || got != want {
		t.Errorf("source=%q; pending handoff must fall through to the start stencil", src)
	}
}
