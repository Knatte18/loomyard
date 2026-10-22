// procgroup_linux.go puts a command in its own process group and kills the whole group on cancel or expiry.

package proc

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// groupPollInterval is how often awaitGone asks whether the terminated group is empty.
const groupPollInterval = 50 * time.Millisecond

// configurePlatformGroupKill starts cmd in its own process group and makes a cancel SIGKILL the whole group, killing the command alone when the group kill fails.
func configurePlatformGroupKill(cmd *exec.Cmd, report func(pid int, groupErr error)) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		pid := cmd.Process.Pid
		groupErr := syscall.Kill(-pid, syscall.SIGKILL)
		report(pid, groupErr)
		if groupErr != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}

// configurePlatformGroupTerminate starts cmd in its own process group and makes a cancel SIGTERM the whole group and arm a SIGKILL of it after killAfter.
// The SIGKILL goes out whether or not the shell has exited, since a descendant may still hold the group.
// A SIGTERM that fails for a reason other than the group being gone kills the command alone and arms no timer.
func configurePlatformGroupTerminate(cmd *exec.Cmd, killAfter time.Duration, report func(pid int, sig os.Signal, groupErr error)) (awaitGone func()) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var (
		mutex      sync.Mutex
		pid        int
		terminated bool
		killSent   bool
		killTimer  *time.Timer
	)
	cmd.Cancel = func() error {
		groupPid := cmd.Process.Pid
		termErr := syscall.Kill(-groupPid, syscall.SIGTERM)
		report(groupPid, syscall.SIGTERM, termErr)
		if errors.Is(termErr, syscall.ESRCH) {
			return termErr
		}
		if termErr != nil {
			return cmd.Process.Kill()
		}
		mutex.Lock()
		defer mutex.Unlock()
		pid = groupPid
		terminated = true
		killTimer = time.AfterFunc(killAfter, func() {
			killErr := syscall.Kill(-groupPid, syscall.SIGKILL)
			report(groupPid, syscall.SIGKILL, killErr)
			mutex.Lock()
			killSent = true
			mutex.Unlock()
		})
		return nil
	}

	return func() {
		mutex.Lock()
		groupPid, ran := pid, terminated
		timer := killTimer
		mutex.Unlock()
		if !ran {
			return
		}
		for {
			if errors.Is(syscall.Kill(-groupPid, 0), syscall.ESRCH) {
				timer.Stop()
				return
			}
			mutex.Lock()
			sent := killSent
			mutex.Unlock()
			if sent {
				return
			}
			time.Sleep(groupPollInterval)
		}
	}
}
