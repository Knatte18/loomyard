// envmarker_windows.go gives Windows no lookup of processes by their environment.

package proc

import "errors"

// PIDsWithEnv reports errors.ErrUnsupported on Windows, where no process's environment is readable by pid.
func PIDsWithEnv(string) ([]int, error) {
	return nil, errors.ErrUnsupported
}
