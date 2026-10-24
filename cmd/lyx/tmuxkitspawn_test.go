// tmuxkitspawn_test.go holds the one definition of a tmuxkit spawn used by the Test Tier Purity scan:
// every exported tmuxkit identifier except Main starts or kills tmux servers,
// so a reference to any of them marks a file as spawning.
// Main is exempt because it runs in TestMain and only the tagged tests it isolates start a server.
// Matching on an exported identifier keeps a file-name mention such as tmuxkit.go out of the rule.
// This file names no export besides Main, so it never trips the scan.
// It carries no build tag, so the guards of every tier that read it compile it.

package main

import "regexp"

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
