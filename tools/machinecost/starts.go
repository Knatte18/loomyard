// starts.go times process starts from Go: the wall per start and the user and system time of the started children.

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// measureStarts starts argv count times, one after another, without the caller's GIT_* variables.
// It adds the median wall per start and the children's mean user and system time per start under start/<name>/.
func measureStarts(ctx context.Context, name string, argv []string, count int, measurements *collector) error {
	walls := make([]time.Duration, 0, count)
	var user, system time.Duration
	env := environWithoutGitVariables()
	for range count {
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Env = env
		started := time.Now()
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("start %s: %w", strings.Join(argv, " "), err)
		}
		walls = append(walls, time.Since(started))
		user += cmd.ProcessState.UserTime()
		system += cmd.ProcessState.SystemTime()
	}
	prefix := groupStart + "/" + name + "/"
	measurements.add(prefix+"median_us", unitMicroseconds, microseconds(median(walls)))
	measurements.add(prefix+"children_user_us", unitMicroseconds, microseconds(user)/float64(count))
	measurements.add(prefix+"children_sys_us", unitMicroseconds, microseconds(system)/float64(count))
	return nil
}
