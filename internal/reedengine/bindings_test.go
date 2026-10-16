// bindings_test.go pins bindingArgvs's output: the five Alt keys and the status click, each a root-table binding, and no other key.
// It also pins the bindings' build stamp and the op lock's re-pin of a server an older build pinned.

package reedengine

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/buildvcs"
)

func TestBindingArgvs_BindsTheFiveAltKeysAndTheStatusClickInTheRootTableOnly(t *testing.T) {
	t.Parallel()

	const prev, next = "switch --prev", "switch --next"
	tests := []struct {
		name       string
		prev, next string
		wantKeys   []string
	}{
		{"both switch commands", prev, next, []string{"M-Up", "M-Down", "M-Left", "M-Right", "MouseDown1Status"}},
		{"no switch commands leaves their keys unbound", "", "", []string{"M-Up", "M-Down", "MouseDown1Status"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			argvs := bindingArgvs(tt.prev, tt.next, false)
			if want := []string{"unbind-key", "-n", "M-z"}; !slices.Equal(argvs[0], want) {
				t.Fatalf("first binding argv = %q, want %q ahead of every bind-key", argvs[0], want)
			}
			var keys []string
			for _, argv := range argvs[1:] {
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

	t.Run("the steps move across windows past hidden ones", func(t *testing.T) {
		t.Parallel()

		got := map[string][]string{}
		for _, argv := range bindingArgvs(prev, next, true) {
			got[argv[2]] = argv
		}
		want := map[string][]string{
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
		for _, rangeType := range []string{"shell", "window", "session", "follow"} {
			if !strings.Contains(joined, "#{==:#{mouse_status_range},"+rangeType+"}") {
				t.Errorf("status click binding %q does not dispatch on range %q", joined, rangeType)
			}
		}
		if strings.Contains(joined, "mouse_status_range},pane}") {
			t.Errorf("status click binding %q dispatches on the retired pane range", joined)
		}
		// The three listed branches and the follow branch, joined by one separator fewer than the branch count.
		if got := strings.Count(joined, `\;`); got != len(statusClickBranches) {
			t.Errorf("status click binding joins its branches with %d escaped separators, want %d", got, len(statusClickBranches))
		}
	})

	t.Run("the shell and window clicks write follow off first and the session click does not", func(t *testing.T) {
		t.Parallel()

		click := clickArgv(t, bindingArgvs(prev, next, true))
		for _, rangeType := range []string{"shell", "window"} {
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

// TestBindingsBuildStamp pins the bindings' build stamp: when a stamp is stale, what the pin writes, and the op lock re-pinning a server an older build pinned.
func TestBindingsBuildStamp(t *testing.T) {
	t.Parallel()

	commitTime := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	running := buildvcs.Identity{Revision: "474fb6680aaaabbbbccccddddeeeeffff0000111", Time: commitTime}
	older := buildvcs.Identity{Revision: "675728800aaaabbbbccccddddeeeeffff0000111", Time: commitTime.Add(-time.Hour)}
	newer := buildvcs.Identity{Revision: "999999999aaaabbbbccccddddeeeeffff0000111", Time: commitTime.Add(time.Hour)}
	dirty := buildvcs.Identity{Revision: running.Revision, Modified: true, Time: commitTime}

	t.Run("StaleRule", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			stamp   string
			running buildvcs.Identity
			want    bool
		}{
			{"Empty", "", running, true},
			{"Garbled", "not-a-stamp", running, true},
			{"UnparseableTime", "yesterday 474fb6680aaa", running, true},
			{"Own", bindingsBuildStamp(running), running, false},
			{"OwnWithTrailingNewline", bindingsBuildStamp(running) + "\n", running, false},
			{"Older", bindingsBuildStamp(older), running, true},
			{"Newer", bindingsBuildStamp(newer), running, false},
			{"SameCommitTimeOtherBuild", bindingsBuildStamp(dirty), running, true},
			{"UnstampedRunning_Empty", "", buildvcs.Identity{}, false},
			{"UnstampedRunning_Older", bindingsBuildStamp(older), buildvcs.Identity{}, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				if got := bindingsStale(tt.stamp, tt.running); got != tt.want {
					t.Errorf("bindingsStale(%q, %v) = %v; want %v", tt.stamp, tt.running, got, tt.want)
				}
			})
		}
	})

	t.Run("PinWritesStamp", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			build buildvcs.Identity
			want  []string
		}{
			{"Stamped", running, []string{"set-option", "-g", bindingsBuildOption, "2026-10-15T12:00:00Z 474fb6680aaa"}},
			{"Unstamped", buildvcs.Identity{}, []string{"set-option", "-gu", bindingsBuildOption}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				e := newTestEngine(t)
				e.build = tt.build
				fake := installFakeTmux(t, e)

				e.pinBindingsLocked()

				var got [][]string
				for _, call := range fake.ArgvFor("set-option") {
					if slices.Contains(call, bindingsBuildOption) {
						got = append(got, call)
					}
				}
				if want := [][]string{tt.want}; !slices.EqualFunc(got, want, slices.Equal[[]string]) {
					t.Errorf("pinBindingsLocked stamp writes = %q; want %q", got, want)
				}
			})
		}
	})

	t.Run("OpLockRepinsStaleBindings", func(t *testing.T) {
		t.Parallel()
		readFailed := errors.New("no server running")
		tests := []struct {
			name       string
			build      buildvcs.Identity
			stamp      string
			readErr    error
			wantRepin  bool
			wantStamps int
		}{
			{"OlderBuildPinned", running, bindingsBuildStamp(older), nil, true, 1},
			{"NoStamp", running, "", nil, true, 1},
			{"OwnBuildPinned", running, bindingsBuildStamp(running), nil, false, 0},
			{"NewerBuildPinned", running, bindingsBuildStamp(newer), nil, false, 0},
			{"ReadFails", running, "", readFailed, false, 0},
			{"UnstampedRunning", buildvcs.Identity{}, bindingsBuildStamp(older), nil, false, 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				e := newTestEngine(t)
				e.build = tt.build
				fake := installFakeTmux(t, e)
				stampFormat := "#{" + bindingsBuildOption + "}"
				fake.answerFormat(stampFormat, tt.stamp, tt.readErr)

				bindKeysBeforeStep := -1
				err := e.withOpLock(func() error {
					bindKeysBeforeStep = fake.Count("bind-key")
					return nil
				})
				if err != nil {
					t.Fatalf("withOpLock() = %v; want nil", err)
				}

				if got := bindKeysBeforeStep > 0; got != tt.wantRepin {
					t.Errorf("withOpLock issued %d bind-key calls before its step; want a re-pin %v", bindKeysBeforeStep, tt.wantRepin)
				}
				if got := fake.Count("bind-key"); got != bindKeysBeforeStep {
					t.Errorf("withOpLock issued %d bind-key calls in all, %d before its step; want none after it", got, bindKeysBeforeStep)
				}
				stamps := 0
				for _, call := range fake.ArgvFor("set-option") {
					if slices.Contains(call, bindingsBuildOption) {
						stamps++
					}
				}
				if stamps != tt.wantStamps {
					t.Errorf("withOpLock wrote the build stamp %d times; want %d", stamps, tt.wantStamps)
				}
				reads := 0
				for _, call := range fake.ArgvFor("display-message") {
					if call[len(call)-1] == stampFormat {
						reads++
					}
				}
				wantReads := 0
				if tt.build.Revision != "" {
					wantReads = 1
				}
				if reads != wantReads {
					t.Errorf("withOpLock read the build stamp %d times; want %d", reads, wantReads)
				}
			})
		}
	})
}
