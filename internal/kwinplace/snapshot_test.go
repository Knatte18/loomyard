// snapshot_test.go pins which window a process id and title name.

package kwinplace

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestWindow_Matches(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	window := func(class, caption string) Window {
		return Window{PID: 7, ResourceClass: class, Caption: caption}
	}
	tests := []struct {
		name   string
		window Window
		pid    int
		want   bool
	}{
		{"TheWindow", window("org.kde.konsole", "T1 slug — Konsole"), 7, true},
		{"OtherProcess", window("org.kde.konsole", "T1 slug — Konsole"), 8, false},
		{"DesktopFileClass", window("konsole", "T1 slug — Konsole"), 7, false},
		{"OtherClass", window("org.kde.dolphin", "T1 slug — Konsole"), 7, false},
		{"OtherCaption", window("org.kde.konsole", "~ : bash — Konsole"), 7, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.window.Matches(tt.pid, "T1 slug"); got != tt.want {
				t.Errorf("Matches() = %v; want %v", got, tt.want)
			}
		})
	}
}
