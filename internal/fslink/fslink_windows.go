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

// utf16Ptr converts a string to a UTF-16 pointer for Windows syscalls, panicking
// on NUL bytes.
func utf16Ptr(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		panic(err)
	}
	return p
}

// CreateDirLink establishes a junction from link to target, creating parent directories and
// refusing to clobber existing paths.
func CreateDirLink(link, target string) error {
	if err := prepareLink(link); err != nil {
		return err
	}

	absTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("filepath.Abs(%s): %w", target, err)
	}

	if err := os.Mkdir(link, 0o755); err != nil {
		return fmt.Errorf("mkdir link %s: %w", link, err)
	}

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

	substName := `\??\` + absTarget
	printName := absTarget

	substNameUTF16 := syscall.StringToUTF16(substName)
	printNameUTF16 := syscall.StringToUTF16(printName)

	substNameBytes := substNameUTF16[:len(substNameUTF16)-1]
	printNameBytes := printNameUTF16[:len(printNameUTF16)-1]

	substNameLen := len(substNameBytes) * 2
	printNameLen := len(printNameBytes) * 2

	headerSize := 16
	bufSize := headerSize + substNameLen + 2 + printNameLen + 2
	buf := make([]byte, bufSize)

	*(*uint32)(unsafe.Pointer(&buf[0])) = windows.IO_REPARSE_TAG_MOUNT_POINT

	reparseDataLen := bufSize - 8
	*(*uint16)(unsafe.Pointer(&buf[4])) = uint16(reparseDataLen)

	*(*uint16)(unsafe.Pointer(&buf[6])) = 0

	*(*uint16)(unsafe.Pointer(&buf[8])) = 0
	*(*uint16)(unsafe.Pointer(&buf[10])) = uint16(substNameLen)

	printNameOffset := substNameLen + 2
	*(*uint16)(unsafe.Pointer(&buf[12])) = uint16(printNameOffset)
	*(*uint16)(unsafe.Pointer(&buf[14])) = uint16(printNameLen)

	copy(buf[16:16+substNameLen], (*[1 << 20]byte)(unsafe.Pointer(&substNameBytes[0]))[:substNameLen])
	copy(buf[16+printNameOffset:16+printNameOffset+printNameLen], (*[1 << 20]byte)(unsafe.Pointer(&printNameBytes[0]))[:printNameLen])

	var bytesReturned uint32
	err = windows.DeviceIoControl(handle, windows.FSCTL_SET_REPARSE_POINT, &buf[0], uint32(len(buf)), nil, 0, &bytesReturned, nil)
	if err != nil {
		os.Remove(link) // Clean up the directory
		return fmt.Errorf("FSCTL_SET_REPARSE_POINT: %w", err)
	}

	return nil
}

// readReparseData opens path with reparse-point semantics and returns the
// reparse data buffer truncated to the bytes Windows wrote.
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

// reparseSubstituteName extracts the substitute name from a junction or
// symlink reparse data buffer, stripping the \??\ prefix if present.
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

// IsLink reports whether path is a link (junction or symlink), returning (false, nil) if absent.
func IsLink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	sysInfo := info.Sys().(*syscall.Win32FileAttributeData)
	if sysInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
		return false, nil
	}

	data, err := readReparseData(path)
	if err != nil {
		return false, err
	}

	tag := *(*uint32)(unsafe.Pointer(&data[0]))

	return (tag == windows.IO_REPARSE_TAG_MOUNT_POINT || tag == windows.IO_REPARSE_TAG_SYMLINK), nil
}

// PointsTo returns the resolved absolute target of a link, with no \??\ prefix.
// Returns an error if link is not a link or if the target does not exist.
func PointsTo(link string) (string, error) {
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

// RawTarget returns the literal, one-hop target recorded in link's own reparse point data — the
// substitute name CreateDirLink wrote — without resolving it further, and without requiring that
// target (or anything past it) to exist.
// This is the ownership-check primitive PointsTo cannot serve: PointsTo fully resolves the chain, so
// it fails outright when a later segment is gone (the legitimate case of a stale-pair prune, where
// one side of a wired junction's chain no longer exists), and it silently walks past a
// target that is itself a further symlink (a Fabric junction wired at a path whose own immediate
// target is another Fabric junction, e.g. a portal pointing at a further Fabric junction that
// itself points at the paired repository) — comparing that fully-resolved end state against the
// ONE HOP a wiring call actually recorded is a mismatch by construction, not a drift.
// Returns an error if link is not a link.
func RawTarget(link string) (string, error) {
	isLink, err := IsLink(link)
	if err != nil {
		return "", err
	}
	if !isLink {
		return "", fmt.Errorf("RawTarget: %s is not a link", link)
	}

	data, err := readReparseData(link)
	if err != nil {
		return "", err
	}
	rawTarget, err := reparseSubstituteName(data)
	if err != nil {
		return "", fmt.Errorf("RawTarget(%s): %w", link, err)
	}
	return rawTarget, nil
}
