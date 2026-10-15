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
			for _, argv := range bindingArgvs(tt.prev, tt.next, false) {
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
		for _, argv := range bindingArgvs(prev, next, true) {
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
		for _, argv := range bindingArgvs(prev, next, true) {
			got[argv[2]] = argv
		}
		want := map[string][]string{
			"M-z":    {"bind-key", "-n", "M-z", "if-shell", "-F", "#{&&:#{@lyx_strands},#{||:#{@strand},#{||:#{" + consolePaneOption + "},#{window_zoomed_flag}}}}", "resize-pane -Z"},
			"M-Up":   {"bind-key", "-n", "M-Up", "set-option", "@lyx_follow", "off", `\;`, "previous-window", `\;`, "if-shell", "-F", "-t", ":", "#{@lyx_hidden}", selectStrandsWindowCommand},
			"M-Down": {"bind-key", "-n", "M-Down", "set-option", "@lyx_follow", "off", `\;`, "next-window", `\;`, "if-shell", "-F", "-t", ":", "#{@lyx_hidden}", "select-window -t :^"},
		}
		for key, wantArgv := range want {
			if !slices.Equal(got[key], wantArgv) {
				t.Errorf("%s argv = %q, want %q", key, got[key], wantArgv)
			}
		}
	})

	t.Run("the status click dispatches on every range type", func(t *testing.T) {
		t.Parallel()

		click := clickArgv(t, bindingArgvs(prev, next, true))
		joined := strings.Join(click, " ")
		for _, rangeType := range []string{"shell", "pane", "window", "session", "follow"} {
			if !strings.Contains(joined, "#{==:#{mouse_status_range},"+rangeType+"}") {
				t.Errorf("status click binding %q does not dispatch on range %q", joined, rangeType)
			}
		}
		// The four listed branches and the follow branch, joined by one separator fewer than the branch count.
		if got := strings.Count(joined, `\;`); got != len(statusClickBranches) {
			t.Errorf("status click binding joins its branches with %d escaped separators, want %d", got, len(statusClickBranches))
		}
	})

	t.Run("the shell, pane and window clicks write follow off first and the session click does not", func(t *testing.T) {
		t.Parallel()

		click := clickArgv(t, bindingArgvs(prev, next, true))
		for _, rangeType := range []string{"shell", "pane", "window"} {
			if got := clickBranchCommand(t, click, rangeType); !strings.HasPrefix(got, "set-option @lyx_follow off ; ") {
				t.Errorf("%s click command = %q, want it to start with the follow-off write", rangeType, got)
			}
		}
		if got := clickBranchCommand(t, click, "session"); strings.Contains(got, "@lyx_follow") {
			t.Errorf("session click command = %q, want it to leave the follow state alone", got)
		}
	})

	t.Run("the follow click toggles the state and runs the jump line only when it is pinned", func(t *testing.T) {
		t.Parallel()

		withJump := clickBranchCommand(t, clickArgv(t, bindingArgvs(prev, next, true)), "follow")
		wantWithJump := `if-shell -F '#{==:#{@lyx_follow},off}' 'set-option @lyx_follow on ; run-shell -b "#{E:@lyx_follow_command}"' 'set-option @lyx_follow off'`
		if withJump != wantWithJump {
			t.Errorf("follow click command with the jump pinned = %q, want %q", withJump, wantWithJump)
		}

		withoutJump := clickBranchCommand(t, clickArgv(t, bindingArgvs(prev, next, false)), "follow")
		wantWithoutJump := `if-shell -F '#{==:#{@lyx_follow},off}' 'set-option @lyx_follow on' 'set-option @lyx_follow off'`
		if withoutJump != wantWithoutJump {
			t.Errorf("follow click command without the jump = %q, want %q", withoutJump, wantWithoutJump)
		}
		if strings.Contains(withoutJump, "run-shell") {
			t.Errorf("follow click command without the jump = %q, want no run-shell", withoutJump)
		}
	})
}

// clickArgv returns the MouseDown1Status binding of argvs.
func clickArgv(t *testing.T, argvs [][]string) []string {
	t.Helper()
	for _, argv := range argvs {
		if argv[2] == "MouseDown1Status" {
			return argv
		}
	}
	t.Fatal("no MouseDown1Status binding")
	return nil
}

// clickBranchCommand returns the command the click binding runs for rangeType, the element after that branch's range condition.
func clickBranchCommand(t *testing.T, click []string, rangeType string) string {
	t.Helper()
	condition := "#{==:#{mouse_status_range}," + rangeType + "}"
	at := slices.Index(click, condition)
	if at == -1 || at+1 >= len(click) {
		t.Fatalf("click binding %q has no branch for range %q", click, rangeType)
	}
	return click[at+1]
}
