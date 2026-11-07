//go:build !windows

// namedjob_other.go stubs the named job object, which only Windows has.

package proc

// holdNamedJobPlatform holds no job off Windows, where there are no job objects to hold.
func holdNamedJobPlatform(name string) (func() error, error) {
	return func() error { return nil }, nil
}

// terminateNamedJobPlatform reports that no job exists.
func terminateNamedJobPlatform(name string) (bool, error) {
	return false, nil
}
