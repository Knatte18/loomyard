//go:build tier0

// tmuxkitspawnreference_test.go pins the tmuxkit-spawn definition tmuxkitspawn_test.go holds.
// This file names no tmuxkit export, so it never trips the scan that uses the definition.

package main

import "testing"

//testtiming:keep pins the tmuxkit-spawn definition the TestTierPurity_UntaggedTestsSpawnNothing guard uses: every export but Main spawns, file-name mentions do not
func TestTmuxkitSpawnReference(t *testing.T) {
	// The sample references are assembled at run time so this file's own source carries none.
	pkg := "tmux" + "kit."
	tests := []struct {
		name    string
		content string
		want    string
		found   bool
	}{
		{"spawning export", "k := " + pkg + "Socket(t, tmux)", pkg + "Socket", true},
		{"main only", "os.Exit(" + pkg + "Main(m))", "", false},
		{"main then spawn", pkg + "Main(m)\n" + pkg + "Socket(t, tmux)", pkg + "Socket", true},
		{"file name mention", "see " + pkg + "go for details", "", false},
		{"no mention", "package p", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := tmuxkitSpawnReference(tt.content)
			if got != tt.want || found != tt.found {
				t.Errorf("tmuxkitSpawnReference(%q) = %q, %v; want %q, %v", tt.content, got, found, tt.want, tt.found)
			}
		})
	}
}
