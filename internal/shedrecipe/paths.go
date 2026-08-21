// paths.go implements resolveUnderRoot, the shared relative-Config-path resolver every entry joins
// a recipe-relative value against a told Env root through.

package shedrecipe

import (
	"fmt"
	"path/filepath"
	"strings"
)

// resolveUnderRoot joins value onto root and returns the joined absolute path, naming entry and
// key in every error message.
//
// It errors when value is empty, when value is absolute per filepath.IsAbs, and when the cleaned
// value escapes root -- computed as joined := filepath.Join(root, value) and rejected unless
// joined equals root or is under root with a separator, so a ".." segment cannot climb out. On
// success it returns joined.
//
// Absolute values are rejected rather than passed through because manifest/designs/shed-recipe.md
// bars absolute paths from Config outright, and accepting one would make a non-portable recipe
// silently work on its author's machine.
//
// The one documented exception that never calls this helper is BurlerRound's profile.target.paths
// and profile.fasit.paths, passed through relative and unjoined because burlerengine.Profile.validate
// resolves them against its own told worktree root.
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
