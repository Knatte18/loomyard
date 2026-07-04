// parse_test.go table-tests the pure pane-list parser in parse.go against
// the exact psmux output shape it is expected to handle, including the
// pane_dead=1 row that remain-on-exit produces.

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
			name: "two panes, second dead, out of vertical order",
			out:  "%3 1 32 220 18\n%1 0 0 220 15\n",
			want: []LivePane{
				{ID: "%3", Dead: true, Top: 32, Width: 220, Height: 18},
				{ID: "%1", Dead: false, Top: 0, Width: 220, Height: 15},
			},
		},
		{
			name: "empty input is no panes",
			out:  "   \n",
			want: nil,
		},
		{
			name:    "missing fields",
			out:     "%1 0 0 220",
			wantErr: true,
		},
		{
			name:    "non-numeric top",
			out:     "%1 0 abc 220 15",
			wantErr: true,
		},
		{
			name:    "non-numeric width",
			out:     "%1 0 0 abc 15",
			wantErr: true,
		},
		{
			name:    "non-numeric height",
			out:     "%1 0 0 220 xyz",
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
