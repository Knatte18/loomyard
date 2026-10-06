// gitkitspawn_test.go holds the one definition of a gitkit spawn shared by the Test Tier Purity and Hermetic Git Test Environment scans:
// every exported gitkit identifier except the hermetic-environment helper spawns git,
// so a reference to any of them marks a file as spawning.
// Matching on an exported identifier keeps a file-name mention such as gitkit.go out of the rule.
// This file names no export besides the helper's own, so it never trips either scan.

package main

import (
	"regexp"
	"testing"
)

// gitkitReference matches a gitkit-qualified exported identifier and captures the identifier.
var gitkitReference = regexp.MustCompile(`gitkit\.([A-Z][A-Za-z0-9_]*)`)

// nonSpawningGitkitExport is the one gitkit export that spawns nothing.
const nonSpawningGitkitExport = "HermeticGitEnv"

// gitkitSpawnReference reports whether content references a gitkit export other than nonSpawningGitkitExport, and returns the first such reference as written.
func gitkitSpawnReference(content string) (string, bool) {
	for _, m := range gitkitReference.FindAllStringSubmatch(content, -1) {
		if m[1] != nonSpawningGitkitExport {
			return m[0], true
		}
	}
	return "", false
}

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
