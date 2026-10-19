// toolfloor.go declares the git and tmux version floor, parses a tool's version line and returns the floor error.

package toolfloor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	// GitFloor is the oldest git lyx runs on.
	GitFloor = "2.43.0"
	// TmuxFloor is the oldest tmux lyx runs on.
	TmuxFloor = "3.4"
)

var (
	gitVersionLine  = regexp.MustCompile(`^git version (\d+(?:\.\d+)*)`)
	tmuxVersionLine = regexp.MustCompile(`^tmux (\d+\.\d+)[a-z]?$`)
)

// FloorError reports a tool whose version is below lyx's floor.
// It is its own type so no caller reads it as an answer git gave.
type FloorError struct {
	// Tool is the tool's name, "git" or "tmux".
	Tool string
	// Found is the dotted version the tool reported.
	Found string
	// Floor is the oldest version lyx runs on.
	Floor string
}

// Error names the tool, the version found, the floor and the way forward.
func (e *FloorError) Error() string {
	return fmt.Sprintf("%s %s is below lyx's minimum %s: install %s %s or newer", e.Tool, e.Found, e.Floor, e.Tool, e.Floor)
}

// CheckGit checks the output of `git --version` against GitFloor.
// A parsed version at or above the floor returns (true, nil) and one below it (true, *FloorError);
// a line that does not parse returns (false, nil), so the caller admits it and warns.
func CheckGit(versionLine string) (parsed bool, err error) {
	return check("git", gitVersionLine, GitFloor, versionLine)
}

// CheckTmux checks the output of `tmux -V` against TmuxFloor, with the same results as CheckGit.
// A trailing release letter, as in `tmux 3.4a`, is ignored.
func CheckTmux(versionLine string) (parsed bool, err error) {
	return check("tmux", tmuxVersionLine, TmuxFloor, versionLine)
}

func check(tool string, pattern *regexp.Regexp, floor, versionLine string) (bool, error) {
	match := pattern.FindStringSubmatch(strings.TrimSpace(versionLine))
	if match == nil {
		return false, nil
	}
	found := match[1]
	if compareVersions(found, floor) < 0 {
		return true, &FloorError{Tool: tool, Found: found, Floor: floor}
	}
	return true, nil
}

// compareVersions compares two dotted numeric versions component by component, a missing component counting as 0.
// Both arguments hold digits and dots only, so every component parses.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		if c := componentAt(as, i) - componentAt(bs, i); c != 0 {
			return c
		}
	}
	return 0
}

func componentAt(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, _ := strconv.Atoi(parts[i])
	return n
}
