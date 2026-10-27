// logfile.go appends the dobby's own log lines to dobby.log by a per-line open and rotates dobby.log and dobby.out to one previous file at a size bound.

package dobbyengine

import (
	"fmt"
	"os"
)

// logRotateBytes is the size at which a dobby log file is rotated to its previous file.
const logRotateBytes = 1 << 20

// AppendLog appends line and a newline to the file at path, opening and closing it for this one line.
// It never creates a missing directory, so a removed worktree gets nothing back and the open error is returned.
// With rotate set, a write that would take the file past 1 MiB first renames it to path + ".1", replacing an older one, and writes to a fresh file;
// without it the file grows past the bound.
// Only the holder of the file's dobby lock passes rotate, so two renames never race.
func AppendLog(path, line string, rotate bool) error {
	text := line + "\n"
	if rotate {
		if info, err := os.Stat(path); err == nil && info.Size()+int64(len(text)) > logRotateBytes {
			if err := os.Rename(path, path+".1"); err != nil {
				return fmt.Errorf("rotate %s: %w", path, err)
			}
		}
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(text); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// RotatePastBound renames the file at path to path + ".1", replacing an older one, when the file is at or past 1 MiB, and reports whether it did.
// An absent file is no rotation and no error.
func RotatePastBound(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Size() < logRotateBytes {
		return false, nil
	}
	if err := os.Rename(path, path+".1"); err != nil {
		return false, fmt.Errorf("rotate %s: %w", path, err)
	}
	return true, nil
}
