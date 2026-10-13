// bindings_test.go pins bindingArgvs's output: the five Alt keys and the status click, each a root-table binding, and no other key.

package reedengine

import (
	"slices"
	"strings"
	"testing"
)

func TestBindingArgvs_BindsTheFiveAltKeysAndTheStatusClickInTheRootTableOnly(t *testing.T) {
	t.Parallel()

	const prev, next = "switch --prev", "switch --next"
	tests := []struct {
		name       string
		prev, next string
		wantKeys   []string
	}{
		{"both switch commands", prev, next, []string{"M-z", "M-Up", "M-Down", "M-Left", "M-Right", "MouseDown1Status"}},
		{"no switch commands leaves their keys unbound", "", "", []string{"M-z", "M-Up", "M-Down", "MouseDown1Status"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var keys []string
			for _, argv := range bindingArgvs(tt.prev, tt.next) {
				if len(argv) < 4 || argv[0] != "bind-key" || argv[1] != "-n" {
					t.Fatalf("binding argv %q, want it to start with bind-key -n <key> <command>", argv)
				}
				keys = append(keys, argv[2])
			}
			if !slices.Equal(keys, tt.wantKeys) {
				t.Errorf("bound keys = %v, want %v", keys, tt.wantKeys)
			}
		})
	}

	t.Run("M-Left and M-Right run the told command in the background", func(t *testing.T) {
		t.Parallel()

		got := map[string][]string{}
		for _, argv := range bindingArgvs(prev, next) {
			got[argv[2]] = argv
		}
		if want := []string{"bind-key", "-n", "M-Left", "run-shell", "-b", prev}; !slices.Equal(got["M-Left"], want) {
			t.Errorf("M-Left argv = %q, want %q", got["M-Left"], want)
		}
		if want := []string{"bind-key", "-n", "M-Right", "run-shell", "-b", next}; !slices.Equal(got["M-Right"], want) {
			t.Errorf("M-Right argv = %q, want %q", got["M-Right"], want)
		}
	})

	t.Run("M-z zooms in the strands' window and the steps move across windows past hidden ones", func(t *testing.T) {
		t.Parallel()

		got := map[string][]string{}
		for _, argv := range bindingArgvs(prev, next) {
			got[argv[2]] = argv
		}
		want := map[string][]string{
			"M-z":    {"bind-key", "-n", "M-z", "if-shell", "-F", "#{&&:#{@lyx_strands},#{||:#{@strand},#{||:#{" + consolePaneOption + "},#{window_zoomed_flag}}}}", "resize-pane -Z"},
			"M-Up":   {"bind-key", "-n", "M-Up", "previous-window", `\;`, "if-shell", "-F", "-t", ":", "#{@lyx_hidden}", selectStrandsWindowCommand},
			"M-Down": {"bind-key", "-n", "M-Down", "next-window", `\;`, "if-shell", "-F", "-t", ":", "#{@lyx_hidden}", "select-window -t :^"},
		}
		for key, wantArgv := range want {
			if !slices.Equal(got[key], wantArgv) {
				t.Errorf("%s argv = %q, want %q", key, got[key], wantArgv)
			}
		}
	})

	t.Run("the status click dispatches on every range type", func(t *testing.T) {
		t.Parallel()

		var click []string
		for _, argv := range bindingArgvs(prev, next) {
			if argv[2] == "MouseDown1Status" {
				click = argv
			}
		}
		joined := strings.Join(click, " ")
		for _, rangeType := range []string{"shell", "pane", "window", "session"} {
			if !strings.Contains(joined, "#{==:#{mouse_status_range},"+rangeType+"}") {
				t.Errorf("status click binding %q does not dispatch on range %q", joined, rangeType)
			}
		}
		if got := strings.Count(joined, `\;`); got != len(statusClickBranches)-1 {
			t.Errorf("status click binding joins its branches with %d escaped separators, want %d", got, len(statusClickBranches)-1)
		}
	})
}
