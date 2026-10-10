// namedjob_windows.go holds the Windows job objects: the unnamed per-child job and the named job a loop holds so a later process can end its tree.

package proc

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobObjectTerminate is the access right TerminateJobObject needs on a job opened by name.
const jobObjectTerminate uint32 = 0x0008

var procOpenJobObject = windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenJobObjectW")

// createKillOnCloseJob creates a job object, named when name is non-nil, whose processes all end when its last handle closes.
func createKillOnCloseJob(name *uint16) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, name)
	// A named job that already exists comes back with its handle and ERROR_ALREADY_EXISTS.
	if err != nil && !(job != 0 && errors.Is(err, windows.ERROR_ALREADY_EXISTS)) {
		return 0, fmt.Errorf("create job object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("set job object kill-on-close: %w", err)
	}
	return job, nil
}

// assignProcessToJob puts the process pid into job.
func assignProcessToJob(job windows.Handle, pid int) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return fmt.Errorf("assign process %d to its job object: %w", pid, err)
	}
	return nil
}

// holdNamedJobPlatform creates the named job, assigns the calling process to it and returns the release of the handle.
func holdNamedJobPlatform(name string) (func() error, error) {
	namePointer, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("job object name %q: %w", name, err)
	}
	job, err := createKillOnCloseJob(namePointer)
	if err != nil {
		return nil, err
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("assign the calling process to job object %q: %w", name, err)
	}
	return func() error { return windows.CloseHandle(job) }, nil
}

// terminateNamedJobPlatform opens the job by name and terminates it, reporting false when no job of that name exists.
func terminateNamedJobPlatform(name string) (bool, error) {
	namePointer, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false, fmt.Errorf("job object name %q: %w", name, err)
	}
	handle, _, callErr := procOpenJobObject.Call(uintptr(jobObjectTerminate), 0, uintptr(unsafe.Pointer(namePointer)))
	if handle == 0 {
		if errors.Is(callErr, windows.ERROR_FILE_NOT_FOUND) {
			return false, nil
		}
		return false, fmt.Errorf("open job object %q: %w", name, callErr)
	}
	job := windows.Handle(handle)
	defer windows.CloseHandle(job)
	if err := windows.TerminateJobObject(job, 1); err != nil {
		return false, fmt.Errorf("terminate job object %q: %w", name, err)
	}
	return true, nil
}
