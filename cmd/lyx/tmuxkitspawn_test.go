// tmuxkitspawn_test.go holds the one definition of a tmuxkit spawn used by the Test Tier Purity scan:
// every exported tmuxkit identifier except Main starts or kills tmux servers,
// so a reference to any of them marks a file as spawning.
// Main is exempt because it runs in TestMain and only the tagged tests it isolates start a server.
// Matching on an exported identifier keeps a file-name mention such as tmuxkit.go out of the rule.
// This file names no export besides Main, so it never trips the scan.

package main

import (
	"regexp"
	"testing"
)

// tmuxkitReference matches a tmuxkit-qualified exported identifier and captures the identifier.
var tmuxkitReference = regexp.MustCompile(`tmuxkit\.([A-Z][A-Za-z0-9_]*)`)

// nonSpawningTmuxkitExport is the one tmuxkit export that is no spawn.
const nonSpawningTmuxkitExport = "Main"

// tmuxkitSpawnReference reports whether content references a tmuxkit export other than nonSpawningTmuxkitExport, and returns the first such reference as written.
func tmuxkitSpawnReference(content string) (string, bool) {
	for _, m := range tmuxkitReference.FindAllStringSubmatch(content, -1) {
		if m[1] != nonSpawningTmuxkitExport {
			return m[0], true
		}
	}
	return "", false
}

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
