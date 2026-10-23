// parse_test.go table-tests the pure pane-list parser in parse.go against the exact tmux output
// shape it is expected to handle, including the pane_dead=1 row that remain-on-exit produces.

package reedengine

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
			out:  "%3 1 32 220 18 4242\n%1 0 0 220 15 1717\n",
			want: []LivePane{
				{ID: "%3", Dead: true, Top: 32, Width: 220, Height: 18, PID: 4242},
				{ID: "%1", Dead: false, Top: 0, Width: 220, Height: 15, PID: 1717},
			},
		},
		{
			name: "title with spaces survives",
			out:  "%1 0 0 220 15 1717 tst:slug:my worker\n",
			want: []LivePane{
				{ID: "%1", Top: 0, Width: 220, Height: 15, PID: 1717, Title: "tst:slug:my worker"},
			},
		},
		{
			name: "empty title",
			out:  "%1 0 0 220 15 1717 \n",
			want: []LivePane{
				{ID: "%1", Top: 0, Width: 220, Height: 15, PID: 1717},
			},
		},
		{
			name: "tail with a spaced title, current command and spaced start command",
			out:  "%1 0 0 220 15 1717 tst:slug:my worker\tnode\t/bin/bash -l\n",
			want: []LivePane{
				{ID: "%1", Width: 220, Height: 15, PID: 1717, Title: "tst:slug:my worker", CurrentCommand: "node", StartCommand: "/bin/bash -l"},
			},
		},
		{
			name: "tail with an empty title and both commands",
			out:  "%1 0 0 220 15 1717 \tbash\tbash\n",
			want: []LivePane{
				{ID: "%1", Width: 220, Height: 15, PID: 1717, CurrentCommand: "bash", StartCommand: "bash"},
			},
		},
		{
			name: "tail ending in an empty start command",
			out:  "%1 0 0 220 15 1717 t\tbash\t\n",
			want: []LivePane{
				{ID: "%1", Width: 220, Height: 15, PID: 1717, Title: "t", CurrentCommand: "bash"},
			},
		},
		{
			name: "line with no separator is a title alone",
			out:  "%1 0 0 220 15 1717 just a title\n",
			want: []LivePane{
				{ID: "%1", Width: 220, Height: 15, PID: 1717, Title: "just a title"},
			},
		},
		{
			name: "empty input is no panes",
			out:  "   \n",
			want: nil,
		},
		{
			name:    "missing fields",
			out:     "%1 0 0 220 15",
			wantErr: true,
		},
		{
			name:    "non-numeric top",
			out:     "%1 0 abc 220 15 4242",
			wantErr: true,
		},
		{
			name:    "non-numeric width",
			out:     "%1 0 0 abc 15 4242",
			wantErr: true,
		},
		{
			name:    "non-numeric height",
			out:     "%1 0 0 220 xyz 4242",
			wantErr: true,
		},
		{
			name:    "non-numeric pid",
			out:     "%1 0 0 220 15 pid",
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

func TestParseWindowPanes(t *testing.T) {
	out := "@2 1 grp %5 0 4242 a title\tnode\t/bin/bash -l\n" +
		"@3 0 - %6 1 99 only a title\n" +
		"@4 0 - %7\n"
	want := []windowPane{
		{WindowID: "@2", Own: true, Group: "grp", PaneID: "%5", PID: 4242, Title: "a title", CurrentCommand: "node", StartCommand: "/bin/bash -l"},
		{WindowID: "@3", PaneID: "%6", Dead: true, PID: 99, Title: "only a title"},
	}
	got := parseWindowPanes(out)
	if len(got) != len(want) {
		t.Fatalf("parseWindowPanes(%q) = %+v, want %+v", out, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pane[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
