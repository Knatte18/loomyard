// gitkitspawn_test.go holds the one definition of a gitkit spawn.
// The Test Tier Purity and Hermetic Git Test Environment scans share it.
// Every exported gitkit identifier except the hermetic-environment helper spawns git,
// so a reference to one marks a file as spawning.
// A file-name mention such as gitkit.go does not.
// This file names no export besides the helper's own, so it never trips either scan,
// and it carries no build tag, so every tier's guards compile it.

package main

import "regexp"

// gitkitReference matches a gitkit-qualified exported identifier and captures the identifier.
var gitkitReference = regexp.MustCompile(`gitkit\.([A-Z][A-Za-z0-9_]*)`)

// nonSpawningGitkitExport is the one gitkit export that spawns nothing.
const nonSpawningGitkitExport = "HermeticGitEnv"

// gitkitSpawnReference reports whether content references a gitkit export that spawns git.
// It returns the first such reference as written.
func gitkitSpawnReference(content string) (string, bool) {
	for _, m := range gitkitReference.FindAllStringSubmatch(content, -1) {
		if m[1] != nonSpawningGitkitExport {
			return m[0], true
		}
	}
	return "", false
}
