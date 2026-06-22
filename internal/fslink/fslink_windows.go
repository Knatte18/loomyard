//go:build windows

package fslink

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// UTF16Ptr converts a string to a null-terminated UTF-16 pointer for Windows syscalls.
func UTF16Ptr(s string) *uint16 {
	return syscall.StringToUTF16Ptr(s)
}

// Create establishes a junction (mount-point reparse point) that links to target.
// It calls prepareLink to refuse clobbering and create parent directories, then
// creates an empty directory, opens it with reparse-point semantics, and issues
// a DeviceIoControl call to set the mount-point reparse data. The target is
// absolutized before being embedded in the reparse data.
func Create(link, target string) error {
	if err := prepareLink(link); err != nil {
		return err
	}

	// Make target absolute
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("filepath.Abs(%s): %w", target, err)
	}

	// Create the empty link directory
	if err := os.Mkdir(link, 0o755); err != nil {
		return fmt.Errorf("mkdir link %s: %w", link, err)
	}

	// Open the directory with reparse-point semantics
	handle, err := windows.CreateFile(
		UTF16Ptr(link),
		windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		os.Remove(link) // Clean up the directory
		return fmt.Errorf("CreateFile(%s): %w", link, err)
	}
	defer windows.CloseHandle(handle)

	// Build the mount-point reparse data.
	// Format: the substitute name has the \??\ prefix and null terminator,
	// the print name is the readable target without prefix and null terminator.
	substName := `\??\` + absTarget
	printName := absTarget

	// Encode to UTF-16
	substNameUTF16 := syscall.StringToUTF16(substName)
	printNameUTF16 := syscall.StringToUTF16(printName)

	// Remove null terminators so we can calculate lengths
	substNameBytes := substNameUTF16[:len(substNameUTF16)-1]
	printNameBytes := printNameUTF16[:len(printNameUTF16)-1]

	substNameLen := len(substNameBytes) * 2
	printNameLen := len(printNameBytes) * 2

	// Build the reparse data buffer
	// Structure: ReparseTag (4) + ReparseDataLength (2) + Reserved (2) +
	//            SubstituteNameOffset (2) + SubstituteNameLength (2) +
	//            PrintNameOffset (2) + PrintNameLength (2) +
	//            MountPointReparseBuffer (substitute + print names)
	headerSize := 8 + 12
	bufSize := headerSize + substNameLen + 2 + printNameLen + 2
	buf := make([]byte, bufSize)

	// ReparseTag (MOUNT_POINT)
	*(*uint32)(unsafe.Pointer(&buf[0])) = windows.IO_REPARSE_TAG_MOUNT_POINT

	// ReparseDataLength (excludes the 8-byte tag and length header)
	reparseDataLen := bufSize - 8
	*(*uint16)(unsafe.Pointer(&buf[4])) = uint16(reparseDataLen)

	// Reserved
	*(*uint16)(unsafe.Pointer(&buf[6])) = 0

	// SubstituteNameOffset and SubstituteNameLength
	*(*uint16)(unsafe.Pointer(&buf[8])) = 0
	*(*uint16)(unsafe.Pointer(&buf[10])) = uint16(substNameLen)

	// PrintNameOffset and PrintNameLength
	printNameOffset := substNameLen + 2
	*(*uint16)(unsafe.Pointer(&buf[12])) = uint16(printNameOffset)
	*(*uint16)(unsafe.Pointer(&buf[14])) = uint16(printNameLen)

	// Copy the substitute name and print name
	copy(buf[16:16+substNameLen], (*[1 << 20]byte)(unsafe.Pointer(&substNameBytes[0]))[:substNameLen])
	copy(buf[16+printNameOffset:16+printNameOffset+printNameLen], (*[1 << 20]byte)(unsafe.Pointer(&printNameBytes[0]))[:printNameLen])

	// Issue FSCTL_SET_REPARSE_POINT
	var bytesReturned uint32
	err = windows.DeviceIoControl(handle, windows.FSCTL_SET_REPARSE_POINT, &buf[0], uint32(len(buf)), nil, 0, &bytesReturned, nil)
	if err != nil {
		os.Remove(link) // Clean up the directory
		return fmt.Errorf("FSCTL_SET_REPARSE_POINT: %w", err)
	}

	return nil
}

// IsLink reports whether path is a link (junction or symlink). It returns
// (false, nil) if path does not exist, (false, err) on stat errors, and
// (true/false, nil) when the path exists and can be checked. A link is true
// when the file has the FILE_ATTRIBUTE_REPARSE_POINT bit set AND its reparse
// tag is IO_REPARSE_TAG_MOUNT_POINT (junction) or IO_REPARSE_TAG_SYMLINK.
func IsLink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	// Check for reparse-point attribute
	sysInfo := info.Sys().(*syscall.Win32FileAttributeData)
	if sysInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
		return false, nil
	}

	// Get the reparse tag via FindFirstFile's Reserved0 field or via FSCTL_GET_REPARSE_POINT
	// Using FindFirstFile is simpler
	pathPtr := UTF16Ptr(path)
	findData := windows.Win32FileAttributeData{}

	// Use FindFirstFile to read the reparse tag
	handle, err := windows.FindFirstFile(pathPtr, &findData)
	if err != nil {
		return false, fmt.Errorf("FindFirstFile(%s): %w", path, err)
	}
	windows.FindClose(handle)

	// The reparse tag is in Reserved0 for reparse points
	tag := findData.Reserved0

	return (tag == windows.IO_REPARSE_TAG_MOUNT_POINT || tag == windows.IO_REPARSE_TAG_SYMLINK), nil
}

// PointsTo returns the resolved absolute target of a link via filepath.EvalSymlinks.
// The result has no \??\ prefix. Returns an error if link is not a link or if the
// target does not exist.
func PointsTo(link string) (string, error) {
	target, err := filepath.EvalSymlinks(link)
	if err != nil {
		return "", fmt.Errorf("filepath.EvalSymlinks(%s): %w", link, err)
	}
	return target, nil
}
