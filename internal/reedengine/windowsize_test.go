// windowsize_test.go covers windowsize.go's pure parsers/predicates and its four *Locked tmux
// round trips, every one driven through TmuxCmd's execHook seam (no live server, no external process
// spawn, no sleep) — the shape generation_test.go and strand_test.go already use.

package reedengine

import (
	"errors"
	"testing"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
)

func TestParseWindowSize(t *testing.T) {
	tests := []struct {
		name   string
		out    string
		wantW  int
		wantH  int
		wantOK bool
	}{
		{"WellFormed", "220 50", 220, 50, true},
		{"TrailingNewline", "220 50\n", 220, 50, true},
		{"ExtraWhitespace", "  220   50  ", 220, 50, true},
		{"Empty", "", 0, 0, false},
		{"OneField", "220", 0, 0, false},
		{"ThreeFields", "220 50 7", 0, 0, false},
		{"NonNumeric", "abc def", 0, 0, false},
		{"ZeroWidth", "0 50", 0, 0, false},
		{"ZeroHeight", "220 0", 0, 0, false},
		{"NegativeWidth", "-1 50", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h, ok := parseWindowSize(tt.out)
			if w != tt.wantW || h != tt.wantH || ok != tt.wantOK {
				t.Errorf("parseWindowSize(%q) = (%d, %d, %v), want (%d, %d, %v)", tt.out, w, h, ok, tt.wantW, tt.wantH, tt.wantOK)
			}
		})
	}
}

func TestLiveBoxLocked(t *testing.T) {
	tests := []struct {
		name   string
		answer string
		err    error
		wantW  int
		wantH  int
		wantOK bool
	}{
		{"WellFormedLivePair", "220 50", nil, 220, 50, true},
		{"Garbage", "abc def", nil, 999, 111, false},
		{"Empty", "", nil, 999, 111, false},
		{"NonPositiveDimension", "220 0", nil, 999, 111, false},
		{"RoundTripError", "", errors.New("boom"), 999, 111, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			// Distinct from the scripted live pair (220x50) so a fallback
			// cannot pass by coincidence.
			e.cfg.Width, e.cfg.Height = 999, 111
			e.tmux.execHook = func(capture bool, args ...string) (string, error) {
				if args[0] == "display-message" {
					return tt.answer, tt.err
				}
				return "", nil
			}

			got, ok := e.liveBoxLocked()
			want := render.Box{X: 0, Y: 0, W: tt.wantW, H: tt.wantH}
			if got != want || ok != tt.wantOK {
				t.Errorf("liveBoxLocked() = (%+v, %v), want (%+v, %v)", got, ok, want, tt.wantOK)
			}
		})
	}
}

func TestReservedRowsFromStatus(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantRows int
		wantOK   bool
	}{
		{"Off", "off", 0, true},
		{"On", "on", 1, true},
		{"NumericTwo", "2", 2, true},
		{"UppercaseOff", "OFF", 0, true},
		{"PaddedOn", " on ", 1, true},
		{"Empty", "", 0, false},
		{"Garbage", "garbage", 0, false},
		{"Negative", "-1", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, ok := reservedRowsFromStatus(tt.raw)
			if rows != tt.wantRows || ok != tt.wantOK {
				t.Errorf("reservedRowsFromStatus(%q) = (%d, %v), want (%d, %v)", tt.raw, rows, ok, tt.wantRows, tt.wantOK)
			}
		})
	}
}

func TestWindowSizeAllowsChain(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"Latest", "latest", true},
		{"UppercaseLatest", "LATEST", true},
		{"PaddedLatest", " latest ", true},
		{"Manual", "manual", false},
		{"Largest", "largest", false},
		{"Smallest", "smallest", false},
		{"Empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := windowSizeAllowsChain(tt.raw); got != tt.want {
				t.Errorf("windowSizeAllowsChain(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestReadStatusRowsLocked(t *testing.T) {
	t.Run("ScriptedAnswer", func(t *testing.T) {
		e := newTestEngine(t)
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			if args[0] == "display-message" {
				return "on", nil
			}
			return "", nil
		}
		rows, ok := e.readStatusRowsLocked()
		if !ok || rows != 1 {
			t.Errorf("readStatusRowsLocked() = (%d, %v), want (1, true)", rows, ok)
		}
	})

	t.Run("RoundTripError", func(t *testing.T) {
		e := newTestEngine(t)
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			return "", errors.New("boom")
		}
		rows, ok := e.readStatusRowsLocked()
		if ok || rows != 0 {
			t.Errorf("readStatusRowsLocked() = (%d, %v), want (0, false)", rows, ok)
		}
	})
}

func TestReadWindowSizeLatestLocked(t *testing.T) {
	t.Run("ScriptedAnswer", func(t *testing.T) {
		e := newTestEngine(t)
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			if args[0] == "display-message" {
				return "latest", nil
			}
			return "", nil
		}
		if got := e.readWindowSizeLatestLocked(); !got {
			t.Errorf("readWindowSizeLatestLocked() = %v, want true", got)
		}
	})

	t.Run("RoundTripError", func(t *testing.T) {
		e := newTestEngine(t)
		e.tmux.execHook = func(capture bool, args ...string) (string, error) {
			return "", errors.New("boom")
		}
		if got := e.readWindowSizeLatestLocked(); got {
			t.Errorf("readWindowSizeLatestLocked() = %v, want false", got)
		}
	})
}

// TestPinGeometryOptionsLocked records every set-option argv the hook receives, asserting both pins
// are issued session/window-targeted (never -g), and that a first-pin error does not stop the second
// pin from being issued.
func TestPinGeometryOptionsLocked(t *testing.T) {
	tests := []struct {
		name        string
		firstErrors bool
	}{
		{"BothSucceed", false},
		{"FirstPinErrors", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEngine(t)
			var calls [][]string
			e.tmux.execHook = func(capture bool, args ...string) (string, error) {
				if args[0] != "set-option" {
					return "", nil
				}
				calls = append(calls, append([]string{}, args...))
				if tt.firstErrors && len(calls) == 1 {
					return "", errors.New("boom")
				}
				return "", nil
			}

			e.pinGeometryOptionsLocked()

			if len(calls) != 2 {
				t.Fatalf("pinGeometryOptionsLocked issued %d set-option calls, want 2: %v", len(calls), calls)
			}

			wantTarget := exactSessionWindowTarget(e.SessionName())

			first := calls[0]
			if !containsArg(first, "-t") || !containsArg(first, wantTarget) {
				t.Errorf("first pin args = %v, want it to carry -t %q", first, wantTarget)
			}
			if containsArg(first, "-g") {
				t.Errorf("first pin args = %v, want no -g", first)
			}
			if !containsArg(first, "status") || !containsArg(first, "off") {
				t.Errorf("first pin args = %v, want the status off pair", first)
			}

			second := calls[1]
			if !containsArg(second, "-w") {
				t.Errorf("second pin args = %v, want -w", second)
			}
			if containsArg(second, "-g") {
				t.Errorf("second pin args = %v, want no -g", second)
			}
			if !containsArg(second, "window-size") || !containsArg(second, "latest") {
				t.Errorf("second pin args = %v, want the window-size latest pair", second)
			}
		})
	}
}

// containsArg reports whether want appears verbatim anywhere in args.
func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
