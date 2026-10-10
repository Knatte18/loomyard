//go:build !windows

// namedjob_other.go stubs the named job object, which only Windows has.

package proc

// holdNamedJobPlatform does nothing and returns a no-op release.
func holdNamedJobPlatform(name string) (func() error, error) {
	return func() error { return nil }, nil
}

// terminateNamedJobPlatform reports that no job exists.
func terminateNamedJobPlatform(name string) (bool, error) {
	return false, nil
}
