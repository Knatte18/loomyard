// root_other.go — the root's name and ownership check off Windows.

//go:build !windows

package lyxtemp

import (
	"fmt"
	"strconv"
	"syscall"
)

// rootName is the root's directory name under the system temp directory, which off Windows is shared by every user.
func rootName(uid int) string {
	return "lyx-" + strconv.Itoa(uid)
}

// checkRoot requires path to be a real directory with permission bits exactly 0700 and owned by uid.
func checkRoot(path string, uid int) error {
	info, err := checkRealDirectory(path)
	if err != nil {
		return err
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		return fmt.Errorf("lyx temp root %q has mode %04o, want 0700", path, perm)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("lyx temp root %q: owner unreadable", path)
	}
	if int(stat.Uid) != uid {
		return fmt.Errorf("lyx temp root %q is owned by uid %d, want %d", path, stat.Uid, uid)
	}
	return nil
}
