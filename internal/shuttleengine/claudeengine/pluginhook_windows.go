//go:build windows

// pluginhook_windows.go leaves a plugin hook's command as it is, since Windows has no process group to kill.

package claudeengine

import "os/exec"

// setHookProcessGroup leaves cmd as it is.
// Windows has no process group to kill, so only hookWaitDelay bounds the wait for a forked child's pipes.
func setHookProcessGroup(cmd *exec.Cmd) {}
