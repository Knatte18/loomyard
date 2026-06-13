package muxpoc

import (
	"strings"
	"testing"
)

func TestLayoutChecksumMatchesPsmux(t *testing.T) {
	// Both values are the exact checksums psmux produced/accepted for these
	// layout bodies during live testing — they pin the algorithm to the real
	// implementation, not just to itself.
	tests := []struct {
		body string
		want string
	}{
		{"220x50,0,0[220x15,0,0,1,220x15,0,16,4,220x18,0,32,3]", "acd7"},
		{"220x50,0,0[220x10,0,0,1,220x10,0,11,4,220x28,0,22,3]", "6954"},
	}
	for _, tt := range tests {
		if got := layoutChecksum(tt.body); got != tt.want {
			t.Errorf("layoutChecksum(%q) = %q, want %q", tt.body, got, tt.want)
		}
	}
}

func TestLayoutChecksumIsFourHexDigits(t *testing.T) {
	got := layoutChecksum("anything")
	if len(got) != 4 {
		t.Fatalf("checksum %q is not 4 chars", got)
	}
	for _, c := range got {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("checksum %q has non-hex char %c", got, c)
		}
	}
}

func TestExpandTpl(t *testing.T) {
	tests := []struct {
		name string
		tpl  string
		sid  string
		task string
		want string
	}{
		{"sid and task", "%CLAUDE% --session-id %SID% %TASK%", "abc", "do x", "%CLAUDE% --session-id abc do x"},
		{"empty task", "%CLAUDE% --resume %SID% %TASK%", "abc", "", "%CLAUDE% --resume abc "},
		{"no placeholders", "claude --help", "abc", "x", "claude --help"},
		{"sid appears twice", "%SID%-%SID%", "z", "", "z-z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandTpl(tt.tpl, tt.sid, tt.task); got != tt.want {
				t.Errorf("expandTpl(%q, %q, %q) = %q, want %q", tt.tpl, tt.sid, tt.task, got, tt.want)
			}
		})
	}
}

func TestParseWindowSize(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		w, h, err := parseWindowSize(" 220x50\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if w != 220 || h != 50 {
			t.Errorf("got (%d, %d), want (220, 50)", w, h)
		}
	})

	for _, bad := range []string{"", "220", "220x", "axb", "220x50x3"} {
		t.Run("invalid_"+bad, func(t *testing.T) {
			if _, _, err := parseWindowSize(bad); err == nil {
				t.Errorf("parseWindowSize(%q): expected error, got nil", bad)
			}
		})
	}
}

func TestParsePaneList(t *testing.T) {
	t.Run("two panes, second dead", func(t *testing.T) {
		panes, err := parsePaneList("%1 0 220 15\n%3 1 220 18\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []LivePane{
			{ID: "%1", Dead: false, Width: 220, Height: 15},
			{ID: "%3", Dead: true, Width: 220, Height: 18},
		}
		if len(panes) != len(want) {
			t.Fatalf("got %d panes, want %d", len(panes), len(want))
		}
		for i := range want {
			if panes[i] != want[i] {
				t.Errorf("pane[%d] = %+v, want %+v", i, panes[i], want[i])
			}
		}
	})

	t.Run("empty input is no panes", func(t *testing.T) {
		panes, err := parsePaneList("   \n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if panes != nil {
			t.Errorf("expected nil, got %v", panes)
		}
	})

	for _, bad := range []string{"%1 0 220", "%1 0 abc 15", "%1 0 220 xyz"} {
		t.Run("invalid_"+bad, func(t *testing.T) {
			if _, err := parsePaneList(bad); err == nil {
				t.Errorf("parsePaneList(%q): expected error, got nil", bad)
			}
		})
	}
}

func TestParsePaneOrderSortsByTop(t *testing.T) {
	// Lines are deliberately out of vertical order.
	ids, err := parsePaneOrder("32 %3\n0 %1\n16 %4\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"%1", "%4", "%3"}
	if len(ids) != len(want) {
		t.Fatalf("got %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("ids[%d] = %q, want %q", i, ids[i], want[i])
		}
	}

	for _, bad := range []string{"abc %1", "%1"} {
		if _, err := parsePaneOrder(bad); err == nil {
			t.Errorf("parsePaneOrder(%q): expected error, got nil", bad)
		}
	}
}

// layoutPane holds one pane parsed back out of a buildColumnLayout string.
type layoutPane struct {
	height int
	top    int
	id     string
}

// parseColumnLayout splits a "csum,WxH,0,0[Wxh,0,y,id,...]" string back into its
// checksum, body (everything after the checksum field), and per-pane geometry so
// tests can assert on observable layout properties.
func parseColumnLayout(t *testing.T, full string) (csum, body string, panes []layoutPane) {
	t.Helper()
	comma := strings.IndexByte(full, ',')
	if comma < 0 {
		t.Fatalf("no checksum field in %q", full)
	}
	csum, body = full[:comma], full[comma+1:]

	open := strings.IndexByte(body, '[')
	closeIdx := strings.LastIndexByte(body, ']')
	if open < 0 || closeIdx < 0 || closeIdx < open {
		t.Fatalf("no pane group brackets in %q", body)
	}
	fields := strings.Split(body[open+1:closeIdx], ",")
	if len(fields)%4 != 0 {
		t.Fatalf("pane fields not a multiple of 4: %v", fields)
	}
	for i := 0; i < len(fields); i += 4 {
		dims := strings.Split(fields[i], "x")
		if len(dims) != 2 {
			t.Fatalf("bad pane dims %q", fields[i])
		}
		panes = append(panes, layoutPane{
			height: atoiOrFail(t, dims[1]),
			top:    atoiOrFail(t, fields[i+2]),
			id:     fields[i+3],
		})
	}
	return csum, body, panes
}

func atoiOrFail(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			t.Fatalf("not a number: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func TestBuildColumnLayoutBottomDominatesAndAncestorsEqual(t *testing.T) {
	cases := []struct {
		name string
		w, h int
		ids  []string
	}{
		{"two panes", 220, 50, []string{"%1", "%3"}},
		{"three panes", 220, 50, []string{"%1", "%4", "%3"}},
		{"three panes tall", 220, 56, []string{"%1", "%4", "%6"}},
		{"three panes short", 200, 29, []string{"%1", "%2", "%3"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			full := buildColumnLayout(tc.w, tc.h, tc.ids)
			csum, body, panes := parseColumnLayout(t, full)
			n := len(tc.ids)

			if len(panes) != n {
				t.Fatalf("got %d panes, want %d", len(panes), n)
			}

			// Checksum prefix must be the checksum of the body.
			if want := layoutChecksum(body); csum != want {
				t.Errorf("checksum prefix = %q, want %q (body=%q)", csum, want, body)
			}

			// Pane ids appear in order, with the leading '%' stripped.
			for i, id := range tc.ids {
				if panes[i].id != strings.TrimPrefix(id, "%") {
					t.Errorf("pane[%d] id = %q, want %q", i, panes[i].id, strings.TrimPrefix(id, "%"))
				}
			}

			// y offsets are cumulative with a one-row divider between panes.
			wantY := 0
			for i, p := range panes {
				if p.top != wantY {
					t.Errorf("pane[%d] top = %d, want %d", i, p.top, wantY)
				}
				wantY += p.height + 1
			}

			// Heights + dividers fill the window exactly.
			sum := n - 1 // dividers
			for _, p := range panes {
				sum += p.height
			}
			if sum != tc.h {
				t.Errorf("heights+dividers = %d, want window height %d", sum, tc.h)
			}

			// The bottom (active) pane is the tallest and takes at least half.
			bottom := panes[n-1].height
			for i := 0; i < n-1; i++ {
				if panes[i].height >= bottom {
					t.Errorf("ancestor pane[%d] height %d >= bottom %d", i, panes[i].height, bottom)
				}
			}
			if bottom*2 < tc.h {
				t.Errorf("bottom pane %d is below 50%% of window height %d", bottom, tc.h)
			}

			// All ancestor panes share the remaining height equally.
			for i := 1; i < n-1; i++ {
				if panes[i].height != panes[0].height {
					t.Errorf("ancestor heights unequal: pane[0]=%d pane[%d]=%d", panes[0].height, i, panes[i].height)
				}
			}
		})
	}
}
