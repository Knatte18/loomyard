// tmuxfloor.go checks tmux's version floor once per resolved tmux path before reed spawns tmux, and keeps whether that tmux has the allow-set-title pane option.

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

// tmuxFloorAnswer is what one tmux path's version probe decided.
type tmuxFloorAnswer struct {
	// floorErr is nil, or the *toolfloor.FloorError for a tmux below the floor.
	floorErr error
	// lacksAllowSetTitle is whether the version line parsed as a release older than toolfloor.TmuxAllowSetTitle.
	lacksAllowSetTitle bool
}

// tmuxFloorCache holds the process's probe answer for each resolved tmux path, so the version probe runs once per path.
// The zero value is ready to use.
type tmuxFloorCache struct {
	mu      sync.Mutex
	answers map[string]tmuxFloorAnswer
}

// check returns the floor answer for tmuxPath, nil or the *toolfloor.FloorError, or the probe error.
func (c *tmuxFloorCache) check(tmuxPath string, probe func(tmuxPath string) (string, error)) error {
	answer, err := c.answer(tmuxPath, probe)
	if err != nil {
		return err
	}
	return answer.floorErr
}

// answer returns the cached answer for tmuxPath once one exists; otherwise it runs probe on tmuxPath.
// tmuxPath is resolved with exec.LookPath, and the raw path is the key when it does not resolve.
// A probe error is returned wrapped and not cached, so the next call probes again.
// A line toolfloor parses is cached with its answer.
// An unparseable line is logged once with Warn and cached as admitted, with allow-set-title not known to be missing.
// A parsed release without allow-set-title is logged once at Info, the one note of the pane-title fallback.
func (c *tmuxFloorCache) answer(tmuxPath string, probe func(tmuxPath string) (string, error)) (tmuxFloorAnswer, error) {
	key := tmuxPath
	if resolved, err := exec.LookPath(tmuxPath); err == nil {
		key = resolved
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if answer, ok := c.answers[key]; ok {
		return answer, nil
	}
	line, err := probe(tmuxPath)
	if err != nil {
		return tmuxFloorAnswer{}, fmt.Errorf("check tmux version: %w", err)
	}
	parsed, floorErr := toolfloor.CheckTmux(line)
	if !parsed {
		logger.Warn("reed: tmux version line not recognised, admitting it", "line", line)
	}
	answer := tmuxFloorAnswer{floorErr: floorErr}
	if parsed, has := toolfloor.TmuxHasAllowSetTitle(line); parsed && !has {
		answer.lacksAllowSetTitle = true
		logger.Info("reed: tmux has no allow-set-title pane option, so strand launches skip it and the agent may overwrite pane titles", "line", line, "since", toolfloor.TmuxAllowSetTitle)
	}
	if c.answers == nil {
		c.answers = map[string]tmuxFloorAnswer{}
	}
	c.answers[key] = answer
	return answer, nil
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

// CheckTmuxFloor reports whether the tmux at tmuxPath is at or above lym's floor, probing once per resolved path per process.
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

// lacksAllowSetTitle reports whether this command's tmux is known to predate the allow-set-title pane option, from the process's version probe.
// A version that did not parse, a probe that could not run, Windows, and a hook-driven command whose floorLine does not parse answer false, so the caller tries the option.
func (p TmuxCmd) lacksAllowSetTitle() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	if p.execHook != nil {
		parsed, has := toolfloor.TmuxHasAllowSetTitle(p.floorLine)
		return parsed && !has
	}
	answer, err := tmuxFloors.answer(p.tmuxPath, tmuxVersionProbe)
	return err == nil && answer.lacksAllowSetTitle
}
