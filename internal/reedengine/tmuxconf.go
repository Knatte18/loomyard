// tmuxconf.go — the operator tmux config feature: the `tmux_conf` key's validation.

package reedengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// tmuxConfEnv is the environment variable the `tmux_conf` template line reads.
const tmuxConfEnv = "LYX_REED_TMUX_CONF"

// tmuxConfForbiddenChars are the non-whitespace characters a tmux config path may not hold:
// source-file treats its argument as a glob, and the boot nests the path in a command string tmux parses a second time.
const tmuxConfForbiddenChars = `*?[\'";#{}$`

// tmuxConfPath validates the `tmux_conf` value and returns the path to source.
// An empty raw returns the empty string.
// A leading `~/` expands to home; the result must be an absolute path to an existing regular file holding no whitespace and none of the characters tmux would reinterpret.
// Every error names the key `tmux_conf`, the variable tmuxConfEnv and the path.
func tmuxConfPath(raw, home string) (string, error) {
	if raw == "" {
		return "", nil
	}
	path := raw
	if strings.HasPrefix(path, "~/") {
		path = home + path[1:]
	}
	refuse := func(reason string) (string, error) {
		return "", fmt.Errorf("reed: the configured tmux config %q (key tmux_conf, variable %s) %s; set tmux_conf in reed.yaml to an absolute path to an existing regular file, or leave it empty", path, tmuxConfEnv, reason)
	}
	if !filepath.IsAbs(path) {
		return refuse("is not an absolute path")
	}
	if strings.ContainsFunc(path, func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune(tmuxConfForbiddenChars, r) }) {
		return refuse("holds whitespace or one of the characters " + tmuxConfForbiddenChars)
	}
	info, err := os.Stat(path)
	if err != nil {
		return refuse("cannot be found: " + err.Error())
	}
	if !info.Mode().IsRegular() {
		return refuse("is not a regular file")
	}
	return path, nil
}
