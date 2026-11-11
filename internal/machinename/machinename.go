// machinename.go resolves this machine's name from the machine file, falling back to the hostname rule,
// and holds the name rule a name set in that file must meet.

package machinename

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/standalonestate"
)

// fileName is the machine file's name in the per-user lym config directory.
const fileName = "machine.json"

// lockName is the name of the lock the machine file's writer holds, beside the file.
const lockName = "machine.lock"

// maxNameLength is the longest name the name rule accepts, a DNS label's length.
const maxNameLength = 63

// setNameWayForward is the way forward of a machine file that does not yield a name.
const setNameWayForward = "run `lym machine name <name>`, which rewrites the file, or remove the file"

// File is the machine file's content.
// Unknown keys are ignored when it is decoded.
type File struct {
	// Name is the machine's current name.
	Name string `json:"name"`
	// Names lists every name recorded for this machine, oldest first, the current one last.
	Names []string `json:"names"`
}

// FilePath returns the machine file's path and its writer's lock's path in the per-user lym config directory.
// It returns ok false when no variable the directory resolves from is set,
// and an error naming the variable, with its way forward, when that variable is relative.
func FilePath() (file, lock string, ok bool, err error) {
	dir, ok, err := standalonestate.AbsoluteUserConfigDir()
	if err != nil {
		return "", "", false, fmt.Errorf("the machine file's directory does not resolve: %w; way forward: set that variable to an absolute path", err)
	}
	if !ok {
		return "", "", false, nil
	}
	return filepath.Join(dir, fileName), filepath.Join(dir, lockName), true, nil
}

// ReadFile decodes the machine file at path into its name and names, with no validation.
// It reports false with no error when no file exists there.
func ReadFile(path string) (File, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return File{}, false, nil
	}
	if err != nil {
		return File{}, false, err
	}
	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return File{}, true, err
	}
	return file, true, nil
}

// Name returns this machine's name: the machine file's name when the file exists, else the hostname rule's.
// A file that does not read, does not decode or holds an invalid name is an error naming its path,
// never a fallback to the hostname, since a wrong name changes which entries read as owned here.
// The file is read on every call.
func Name() (string, error) {
	name, _, err := resolveNames(machineFile, os.Hostname)
	return name, err
}

// Names returns this machine's current name, resolved as Name resolves it, and its former names.
// A former name is a non-empty entry of the machine file's names other than the current one,
// kept even when it fails the name rule, since only this machine could have written an owner under it.
// The hostname rule has no former names.
func Names() (current string, former []string, err error) {
	return resolveNames(machineFile, os.Hostname)
}

// CheckName reports whether name meets the name rule a name set in the machine file must meet:
// lower-case letters, digits and hyphens, starting and ending with a letter or digit, at most 63 characters.
// The rule keeps the name from reading as a domain or a hub-qualified name in an owner, an address or a channel name.
func CheckName(name string) error {
	valid := name != "" && len(name) <= maxNameLength && name[0] != '-' && name[len(name)-1] != '-'
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			valid = false
		}
	}
	if !valid {
		return fmt.Errorf("the machine name %q is invalid: a name is lower-case letters, digits and hyphens, starts and ends with a letter or digit, and is at most %d characters; way forward: choose a name of that form", name, maxNameLength)
	}
	return nil
}

// FromHostname reduces host to its short name: the part before the first dot, lowercased and trimmed of space.
// For example, Neve.local reads neve.
// It returns an error when nothing is left, since no owner can be written or compared without a name.
func FromHostname(host string) (string, error) {
	short, _, _ := strings.Cut(host, ".")
	name := strings.ToLower(strings.TrimSpace(short))
	if name == "" {
		return "", fmt.Errorf("this machine's hostname %q is empty, so no owner can be written or compared; give the machine a hostname", host)
	}
	return name, nil
}

// fileLocator returns the machine file's path, whether one resolves, and the error of a relative variable.
type fileLocator func() (path string, ok bool, err error)

// machineFile locates the machine file in the per-user lym config directory.
func machineFile() (string, bool, error) {
	file, _, ok, err := FilePath()
	return file, ok, err
}

// resolveNames resolves the current and former names from the file locate names, falling back to hostname's rule.
// Tests hand in the file's path and the hostname through locate and hostname.
func resolveNames(locate fileLocator, hostname func() (string, error)) (string, []string, error) {
	path, ok, err := locate()
	if err != nil {
		return "", nil, err
	}
	if !ok {
		name, err := nameFromHostname(hostname)
		return name, nil, err
	}
	file, exists, err := ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("the machine file %s does not read: %w; way forward: %s", path, err, setNameWayForward)
	}
	if !exists {
		name, err := nameFromHostname(hostname)
		return name, nil, err
	}
	if err := CheckName(file.Name); err != nil {
		return "", nil, fmt.Errorf("the machine file %s holds no valid name: %w; way forward: %s", path, err, setNameWayForward)
	}
	var former []string
	for _, name := range file.Names {
		if name != "" && name != file.Name {
			former = append(former, name)
		}
	}
	return file.Name, former, nil
}

// nameFromHostname applies the hostname rule to the hostname hostname reads.
func nameFromHostname(hostname func() (string, error)) (string, error) {
	host, err := hostname()
	if err != nil {
		return "", fmt.Errorf("read this machine's hostname: %w; way forward: give the machine a hostname, or name it with `lym machine name <name>`", err)
	}
	return FromHostname(host)
}
