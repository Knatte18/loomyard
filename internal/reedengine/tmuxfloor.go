// tmuxfloor.go checks tmux's version floor once per resolved tmux path before reed spawns tmux.

package reedengine

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/toolfloor"
)

// tmuxFloorCache holds the process's floor answer for each resolved tmux path, so the version probe runs once per path.
// The zero value is ready to use.
type tmuxFloorCache struct {
	mu      sync.Mutex
	answers map[string]error
}

// check returns the cached answer for tmuxPath once one exists; otherwise it runs probe on tmuxPath.
// tmuxPath is resolved with exec.LookPath, and the raw path is the key when it does not resolve.
// A probe error is returned wrapped and not cached, so the next call probes again.
// A line toolfloor parses is cached with its answer, nil or the *toolfloor.FloorError.
// An unparseable line is logged once with Warn and cached as admitted.
func (c *tmuxFloorCache) check(tmuxPath string, probe func(tmuxPath string) (string, error)) error {
	key := tmuxPath
	if resolved, err := exec.LookPath(tmuxPath); err == nil {
		key = resolved
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if answer, ok := c.answers[key]; ok {
		return answer
	}
	line, err := probe(tmuxPath)
	if err != nil {
		return fmt.Errorf("check tmux version: %w", err)
	}
	parsed, floorErr := toolfloor.CheckTmux(line)
	if !parsed {
		logger.Warn("reed: tmux version line not recognised, admitting it", "line", line)
	}
	if c.answers == nil {
		c.answers = map[string]error{}
	}
	c.answers[key] = floorErr
	return floorErr
}

// tmuxFloors is the process's tmux floor answers.
var tmuxFloors tmuxFloorCache

// tmuxVersionProbe reports tmux's version line; it is a variable so a test injects one.
var tmuxVersionProbe = probeTmuxVersion

// probeTmuxVersion runs `<tmuxPath> -V` and returns its trimmed stdout, or the exec error when tmux could not run.
func probeTmuxVersion(tmuxPath string) (string, error) {
	logger.Info("reed: spawn tmux version probe", "tmux", tmuxPath)
	out, err := exec.Command(tmuxPath, "-V").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// CheckTmuxFloor reports whether the tmux at tmuxPath is at or above lyx's floor, probing once per resolved path per process.
// It returns a *toolfloor.FloorError for a tmux below the floor, and the wrapped exec error, uncached, when tmux could not be run.
// It admits without probing on Windows, whose psmux has its own version shape and the capability probe's own floor.
// It is exported so a tmux spawn outside the runner checks too.
func CheckTmuxFloor(tmuxPath string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	return tmuxFloors.check(tmuxPath, tmuxVersionProbe)
}

// checkFloor checks the floor for this command's tmux binary before a spawn.
// A command with an execHook never reads or fills the process cache.
// It admits when floorLine is empty, and otherwise returns the refusal toolfloor.CheckTmux gives floorLine, admitting an unparseable line.
func (p TmuxCmd) checkFloor() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if p.execHook != nil {
		if p.floorLine == "" {
			return nil
		}
		_, err := toolfloor.CheckTmux(p.floorLine)
		return err
	}
	return CheckTmuxFloor(p.tmuxPath)
}
