//go:build windows

package fslink

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// utf16Ptr converts a string to a null-terminated UTF-16 pointer for Windows syscalls.
// It panics if s contains a NUL byte, matching the contract of the call sites that
// pass filesystem paths (which never legitimately contain NUL).
func utf16Ptr(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		panic(err)
	}
	return p
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
		utf16Ptr(link),
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
	// Header before PathBuffer is 16 bytes: the 8-byte generic header
	// (ReparseTag + ReparseDataLength + Reserved) plus the four USHORT
	// MountPointReparseBuffer fields (SubstituteNameOffset/Length,
	// PrintNameOffset/Length). PathBuffer then holds both null-terminated names.
	headerSize := 16
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

// readReparseData opens path with reparse-point semantics and returns the raw
// reparse data buffer, truncated to the number of bytes Windows actually wrote.
// The output buffer is sized to the maximum reparse data buffer — the tag alone
// is not enough; Windows rejects an undersized buffer with "data area too small".
func readReparseData(path string) ([]byte, error) {
	handle, err := windows.CreateFile(
		utf16Ptr(path),
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateFile(%s): %w", path, err)
	}
	defer windows.CloseHandle(handle)

	const maxReparseSize = 16 * 1024 // MAXIMUM_REPARSE_DATA_BUFFER_SIZE
	buf := make([]byte, maxReparseSize)
	var bytesReturned uint32
	err = windows.DeviceIoControl(
		handle,
		windows.FSCTL_GET_REPARSE_POINT,
		nil, 0,
		&buf[0], uint32(len(buf)),
		&bytesReturned, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("FSCTL_GET_REPARSE_POINT: %w", err)
	}
	return buf[:bytesReturned], nil
}

// reparseSubstituteName extracts the substitute name (with the \??\ prefix
// stripped) from a junction or symlink reparse data buffer. The PathBuffer is
// preceded by a 16-byte header for mount points and a 20-byte header for
// symlinks (the latter has an extra 4-byte Flags field); the name offset and
// length fields live at the same offsets (8 and 10) for both.
func reparseSubstituteName(data []byte) (string, error) {
	if len(data) < 16 {
		return "", fmt.Errorf("reparse buffer too small (%d bytes)", len(data))
	}
	tag := *(*uint32)(unsafe.Pointer(&data[0]))
	var pathBufOffset int
	switch tag {
	case windows.IO_REPARSE_TAG_MOUNT_POINT:
		pathBufOffset = 16
	case windows.IO_REPARSE_TAG_SYMLINK:
		pathBufOffset = 20
	default:
		return "", fmt.Errorf("unsupported reparse tag %#x", tag)
	}

	substNameOffset := int(*(*uint16)(unsafe.Pointer(&data[8])))
	substNameLen := int(*(*uint16)(unsafe.Pointer(&data[10])))
	start := pathBufOffset + substNameOffset
	end := start + substNameLen
	if end > len(data) {
		return "", fmt.Errorf("reparse buffer truncated: need %d bytes, have %d", end, len(data))
	}

	u16 := unsafe.Slice((*uint16)(unsafe.Pointer(&data[start])), substNameLen/2)
	name := syscall.UTF16ToString(u16)
	return strings.TrimPrefix(name, `\??\`), nil
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

	data, err := readReparseData(path)
	if err != nil {
		return false, err
	}

	// The reparse tag is in the first 4 bytes of the reparse point data.
	tag := *(*uint32)(unsafe.Pointer(&data[0]))

	return (tag == windows.IO_REPARSE_TAG_MOUNT_POINT || tag == windows.IO_REPARSE_TAG_SYMLINK), nil
}

// PointsTo returns the resolved absolute target of a link. The target is read
// directly from the reparse data (Go's filepath.EvalSymlinks does not resolve
// junctions on current Windows builds, where they report as ModeIrregular), then
// canonicalized via filepath.EvalSymlinks so the result matches how callers
// resolve the other end of a comparison. The result has no \??\ prefix. Returns
// an error if link is not a link or if the target does not exist.
func PointsTo(link string) (string, error) {
	// Verify it's actually a link
	isLink, err := IsLink(link)
	if err != nil {
		return "", err
	}
	if !isLink {
		return "", fmt.Errorf("PointsTo: %s is not a link", link)
	}

	data, err := readReparseData(link)
	if err != nil {
		return "", err
	}
	rawTarget, err := reparseSubstituteName(data)
	if err != nil {
		return "", fmt.Errorf("PointsTo(%s): %w", link, err)
	}

	target, err := filepath.EvalSymlinks(rawTarget)
	if err != nil {
		return "", fmt.Errorf("filepath.EvalSymlinks(%s): %w", rawTarget, err)
	}
	return target, nil
}
