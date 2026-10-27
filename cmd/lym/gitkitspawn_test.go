// gitkitspawn_test.go holds the one definition of a gitkit spawn shared by the Test Tier Purity and Hermetic Git Test Environment scans:
// every exported gitkit identifier except the hermetic-environment helper spawns git,
// so a reference to any of them marks a file as spawning.
// Matching on an exported identifier keeps a file-name mention such as gitkit.go out of the rule.
// This file names no export besides the helper's own, so it never trips either scan.
// It carries no build tag, so the guards of every tier that read it compile it.

package main

import "regexp"

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
