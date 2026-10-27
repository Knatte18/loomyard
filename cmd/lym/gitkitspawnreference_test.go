//go:build tier0

// gitkitspawnreference_test.go pins the gitkit-spawn definition gitkitspawn_test.go holds.
// This file names no gitkit export, so it never trips the scans that use the definition.

package main

import "testing"

//testtiming:keep pins the gitkit-spawn definition the TestTierPurity_UntaggedTestsSpawnNothing and TestHermeticGitEnv_GitSpawningPackagesHaveTestMain guards share: every export but HermeticGitEnv spawns, file-name mentions do not
func TestGitkitSpawnReference(t *testing.T) {
	// The sample references are assembled at run time so this file's own source carries none.
	pkg := "git" + "kit."
	tests := []struct {
		name    string
		content string
		want    string
		found   bool
	}{
		{"spawning export", "x := " + pkg + "RevParse(t, dir)", pkg + "RevParse", true},
		{"hermetic helper only", pkg + "HermeticGitEnv()", "", false},
		{"helper then spawn", pkg + "HermeticGitEnv()\n" + pkg + "CopyRepo(t)", pkg + "CopyRepo", true},
		{"file name mention", "see " + pkg + "go for details", "", false},
		{"no mention", "package p", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := gitkitSpawnReference(tt.content)
			if got != tt.want || found != tt.found {
				t.Errorf("gitkitSpawnReference(%q) = %q, %v; want %q, %v", tt.content, got, found, tt.want, tt.found)
			}
		})
	}
}
