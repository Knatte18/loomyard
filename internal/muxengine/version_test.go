// version_test.go drives the pure `-V` parsers and the versionAtLeast
// comparator (version.go) with fixtures covering both multiplexers' output
// shapes, plus malformed input and every relevant versionAtLeast boundary.

package muxengine

import "testing"

func TestParseTmuxVersion(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		want    [3]int
		wantErr bool
	}{
		{"valid tmux -V line", "tmux 3.3\n", [3]int{3, 3, 0}, false},
		{"lettered tmux -V line", "tmux 3.3a\n", [3]int{3, 3, 0}, false},
		{"next- tmux -V line", "tmux next-3.4\n", [3]int{3, 4, 0}, false},
		{"malformed input", "not a version string\n", [3]int{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTmuxVersion(tt.out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseTmuxVersion(%q) error = %v, wantErr %v", tt.out, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("parseTmuxVersion(%q) = %v, want %v", tt.out, got, tt.want)
			}
		})
	}
}

func TestParseTmuxVersionWindows(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		want    [3]int
		wantErr bool
	}{
		{"valid Windows tmux-port (psmux) -V line", "psmux 3.3.4\n", [3]int{3, 3, 4}, false},
		{"malformed input", "psmux dev-build\n", [3]int{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTmuxVersionWindows(tt.out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseTmuxVersionWindows(%q) error = %v, wantErr %v", tt.out, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("parseTmuxVersionWindows(%q) = %v, want %v", tt.out, got, tt.want)
			}
		})
	}
}

func TestVersionAtLeast(t *testing.T) {
	pin := [3]int{3, 3, 3}
	tests := []struct {
		name string
		got  [3]int
		min  [3]int
		want bool
	}{
		{"above pin", [3]int{3, 3, 4}, pin, true},
		{"at pin", [3]int{3, 3, 3}, pin, true},
		{"below pin", [3]int{3, 3, 2}, pin, false},
		{"below pin on minor", [3]int{3, 2, 9}, pin, false},
		{"above pin on major", [3]int{4, 0, 0}, pin, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionAtLeast(tt.got, tt.min); got != tt.want {
				t.Errorf("versionAtLeast(%v, %v) = %v, want %v", tt.got, tt.min, got, tt.want)
			}
		})
	}
}
