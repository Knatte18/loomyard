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

func TestChooseStartPrompt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		flag       string
		state      State
		existing   []string
		wantErr    bool
		wantSource string
		wantIn     []string
		wantNotIn  []string
		// wantFresh asserts the prompt is exactly the start stencil's.
		wantFresh bool
	}{
		{
			name:       "flag wins over the last handoff",
			flag:       "/h/flag.md",
			state:      State{LastHandoff: "/h/last.md"},
			existing:   []string{"/h/flag.md", "/h/last.md"},
			wantSource: SourceFlag,
			wantIn:     []string{"/h/flag.md", testRolePath},
			wantNotIn:  []string{"/h/last.md"},
		},
		{
			name:     "missing flag file errors",
			flag:     "/h/gone.md",
			state:    State{LastHandoff: "/h/last.md"},
			existing: []string{"/h/last.md"},
			wantErr:  true,
		},
		{
			name:       "last handoff when present",
			state:      State{LastHandoff: "/h/last.md"},
			existing:   []string{"/h/last.md"},
			wantSource: SourceLastHandoff,
			wantIn:     []string{"/h/last.md", testRolePath},
		},
		{
			name:       "last handoff gone falls to fresh",
			state:      State{LastHandoff: "/h/last.md"},
			wantSource: SourceFresh,
		},
		{
			name:       "pending handoff is ignored",
			state:      State{PendingHandoff: "/h/partial.md"},
			existing:   []string{"/h/partial.md"},
			wantSource: SourceFresh,
			wantFresh:  true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := seedStencils(t)
			got, src, err := ChooseStartPrompt(dir, testRolePath, c.flag, c.state, existsOnly(c.existing...))
			if c.wantErr {
				if err == nil {
					t.Fatal("want an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if src != c.wantSource {
				t.Errorf("source = %q, want %q", src, c.wantSource)
			}
			for _, want := range c.wantIn {
				if !strings.Contains(got, want) {
					t.Errorf("prompt %q missing %q", got, want)
				}
			}
			for _, unwanted := range c.wantNotIn {
				if strings.Contains(got, unwanted) {
					t.Errorf("prompt %q must not contain %q", got, unwanted)
				}
			}
			if c.wantFresh {
				want, err := RenderStartPrompt(dir, testRolePath)
				if err != nil {
					t.Fatal(err)
				}
				if got != want {
					t.Errorf("prompt = %q, want the start stencil %q", got, want)
				}
			}
		})
	}
}
