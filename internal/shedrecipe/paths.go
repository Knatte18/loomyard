// paths.go holds resolveUnderRoot, which every entry uses to join a relative Config path onto an Env root.

package shedrecipe

import (
	"fmt"
	"path/filepath"
	"strings"
)

// resolveUnderRoot joins value onto root and returns the joined path, naming entry and key in every error.
//
// It errors when value is empty or absolute per filepath.IsAbs.
// It also rejects a value that escapes root.
// The joined path must equal root or sit under it with a separator, so a ".." segment cannot climb out.
//
// Absolute values are rejected because contracts/specs/shed-recipe-spec.md bars them from Config.
// Passing one through would let a non-portable recipe work only on its author's machine.
//
// The one exception is the profile.target.paths and profile.fasit.paths keys of the BurlerReview and BurlerFix rows.
// They stay relative and unjoined, since their profile validator resolves them against the worktree root itself.
func resolveUnderRoot(entry, key, root, value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("shedrecipe: %s: config key %q must not be empty", entry, key)
	}
	if filepath.IsAbs(value) {
		return "", fmt.Errorf("shedrecipe: %s: config key %q value %q must not be absolute", entry, key, value)
	}

	joined := filepath.Join(root, value)
	if joined != root && !strings.HasPrefix(joined, root+string(filepath.Separator)) {
		return "", fmt.Errorf("shedrecipe: %s: config key %q value %q escapes root %q", entry, key, value, root)
	}
	return joined, nil
}
