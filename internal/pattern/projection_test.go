//go:build tier0

// projection_test.go covers ProjectOverview and EntryBullet, one row per replace, append and delete outcome.

package pattern

import "testing"

func TestProjectOverview(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		overview   string
		prescribed []string
		removes    []string
		want       string
	}{
		{
			name:       "replaces a one-line bullet",
			overview:   "# PATTERN\n\n- `PATTERN-a` — old.\n- `PATTERN-b` — b.\n",
			prescribed: []string{"- `PATTERN-a` — new."},
			want:       "# PATTERN\n\n- `PATTERN-a` — new.\n- `PATTERN-b` — b.\n",
		},
		{
			name:       "replaces a bullet with its continuation line whole",
			overview:   "- `PATTERN-a` — old,\n  continued.\n- `PATTERN-b` — b.\n",
			prescribed: []string{"- `PATTERN-a` — new."},
			want:       "- `PATTERN-a` — new.\n- `PATTERN-b` — b.\n",
		},
		{
			name:       "appends an absent ID",
			overview:   "- `PATTERN-a` — a.\n",
			prescribed: []string{"- `PATTERN-c` — c."},
			want:       "- `PATTERN-a` — a.\n- `PATTERN-c` — c.\n",
		},
		{
			name:       "appends after an overview without a trailing newline",
			overview:   "- `PATTERN-a` — a.",
			prescribed: []string{"- `PATTERN-c` — c."},
			want:       "- `PATTERN-a` — a.\n- `PATTERN-c` — c.\n",
		},
		{
			name:       "keeps the last of two lines for one ID",
			overview:   "- `PATTERN-a` — old.\n",
			prescribed: []string{"- `PATTERN-a` — first.", "- `PATTERN-a` — second."},
			want:       "- `PATTERN-a` — second.\n",
		},
		{
			name:     "deletes a removed ID with its continuation line",
			overview: "- `PATTERN-a` — a,\n  continued.\n- `PATTERN-b` — b.\n",
			removes:  []string{"PATTERN-a"},
			want:     "- `PATTERN-b` — b.\n",
		},
		{
			name:     "an absent removes ID changes nothing",
			overview: "- `PATTERN-a` — a.\n",
			removes:  []string{"PATTERN-z"},
			want:     "- `PATTERN-a` — a.\n",
		},
		{
			name:       "skips a line with no entry head",
			overview:   "- `PATTERN-a` — a.\n",
			prescribed: []string{"not an entry", "- plain bullet"},
			want:       "- `PATTERN-a` — a.\n",
		},
		{
			name:       "a nil overview projects to the prescribed lines alone",
			prescribed: []string{"- `PATTERN-a` — a.", "- `PATTERN-b` — b."},
			want:       "- `PATTERN-a` — a.\n- `PATTERN-b` — b.\n",
		},
		{
			name:       "an overview already holding the line projects to equal bytes",
			overview:   "# PATTERN\n\n- `PATTERN-a` — a.\n\n## More\n\n- `PATTERN-b` — b.\n",
			prescribed: []string{"- `PATTERN-a` — a.", "- `PATTERN-b` — b."},
			want:       "# PATTERN\n\n- `PATTERN-a` — a.\n\n## More\n\n- `PATTERN-b` — b.\n",
		},
		{
			name:       "reads CRLF as LF",
			overview:   "- `PATTERN-a` — old.\r\n- `PATTERN-b` — b.\r\n",
			prescribed: []string{"- `PATTERN-a` — new."},
			want:       "- `PATTERN-a` — new.\n- `PATTERN-b` — b.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			overview := []byte(tt.overview)
			if tt.overview == "" {
				overview = nil
			}
			if got := string(ProjectOverview(overview, tt.prescribed, tt.removes)); got != tt.want {
				t.Errorf("ProjectOverview() = %q; want %q", got, tt.want)
			}
			if string(overview) != tt.overview && tt.overview != "" {
				t.Errorf("ProjectOverview changed its overview argument to %q", overview)
			}
		})
	}
}

func TestEntryBullet(t *testing.T) {
	t.Parallel()
	overview := []byte("- `PATTERN-a` — a.\n- `PATTERN-b` — b,\n  continued.\n\n- `PATTERN-c` — c.\n")
	tests := []struct {
		name   string
		id     string
		want   string
		wantOK bool
	}{
		{"one-line bullet", "PATTERN-a", "- `PATTERN-a` — a.", true},
		{"bullet with a continuation line", "PATTERN-b", "- `PATTERN-b` — b,\n  continued.", true},
		{"absent ID", "PATTERN-z", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := EntryBullet(overview, tt.id)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("EntryBullet(%q) = %q, %v; want %q, %v", tt.id, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
