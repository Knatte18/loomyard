// parse_test.go table-tests the pure pane/size/order parsers in parse.go
// against the exact psmux output shapes they are expected to handle,
// including the pane_dead=1 row that remain-on-exit produces.

package muxengine

import "testing"

func TestParsePaneList(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		want    []LivePane
		wantErr bool
	}{
		{
			name: "two panes, second dead",
			out:  "%1 0 220 15\n%3 1 220 18\n",
			want: []LivePane{
				{ID: "%1", Dead: false, Width: 220, Height: 15},
				{ID: "%3", Dead: true, Width: 220, Height: 18},
			},
		},
		{
			name: "empty input is no panes",
			out:  "   \n",
			want: nil,
		},
		{
			name:    "missing fields",
			out:     "%1 0 220",
			wantErr: true,
		},
		{
			name:    "non-numeric width",
			out:     "%1 0 abc 15",
			wantErr: true,
		},
		{
			name:    "non-numeric height",
			out:     "%1 0 220 xyz",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePaneList(tt.out)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parsePaneList(%q): expected error, got nil", tt.out)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePaneList(%q): unexpected error: %v", tt.out, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parsePaneList(%q) = %+v, want %+v", tt.out, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("pane[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
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
			t.Errorf("parseWindowSize = (%d, %d), want (220, 50)", w, h)
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

func TestParsePaneOrder(t *testing.T) {
	t.Run("sorts by top ascending", func(t *testing.T) {
		// Lines are deliberately out of vertical order.
		ids, err := parsePaneOrder("32 %3\n0 %1\n16 %4\n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"%1", "%4", "%3"}
		if len(ids) != len(want) {
			t.Fatalf("parsePaneOrder = %v, want %v", ids, want)
		}
		for i := range want {
			if ids[i] != want[i] {
				t.Errorf("ids[%d] = %q, want %q", i, ids[i], want[i])
			}
		}
	})

	t.Run("empty input is no panes", func(t *testing.T) {
		ids, err := parsePaneOrder("   \n")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 0 {
			t.Errorf("parsePaneOrder(empty) = %v, want empty", ids)
		}
	})

	for _, bad := range []string{"abc %1", "%1"} {
		t.Run("invalid_"+bad, func(t *testing.T) {
			if _, err := parsePaneOrder(bad); err == nil {
				t.Errorf("parsePaneOrder(%q): expected error, got nil", bad)
			}
		})
	}
}
