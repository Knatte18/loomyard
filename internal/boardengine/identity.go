// identity.go holds the one write of this machine's id into the machine file, beside `lym machine name`.
//
// EnsureIdentity returns this machine's identity and writes the id on the machine's first registration;
// IdentityRefusal is its read-only refusal for a status read that must not write the id.

package boardengine

import (
	"fmt"
	"runtime"

	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/state"
)

// machineFileWayForward is the way forward of a machine file that does not yield a name.
const machineFileWayForward = "run `lym machine name <name>`, which rewrites the file, or remove the file"

// machineFileLocator is machinename.FilePath's shape: the machine file's path, its lock's path and whether they resolve.
type machineFileLocator func() (file, lock string, ok bool, err error)

// EnsureIdentity returns this machine's identity, writing a new id into the machine file when it holds none.
// A missing file is created holding the hostname rule's name.
// The id is written under the file's lock and every other key of the file is kept.
// With no directory to hold the file it returns a *RegistrationError naming the variable to set.
// An unreadable file or one holding an invalid name is an error naming its way forward.
func EnsureIdentity() (machinename.Identity, error) {
	return ensureIdentityWith(machinename.FilePath, machinename.FromThisHost)
}

// IdentityRefusal returns the refusal EnsureIdentity would return for a machine file directory that does not resolve.
// It reads no file and writes none, and returns nil when the directory resolves.
func IdentityRefusal() error {
	return identityRefusal(machinename.FilePath)
}

// identityRefusal builds the refusal of a machine file directory that does not resolve, nil when it does.
// An error of locate, a relative variable, is returned as it is.
func identityRefusal(locate machineFileLocator) error {
	_, _, ok, err := locate()
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	variable := "XDG_CONFIG_HOME or HOME"
	if runtime.GOOS == "windows" {
		variable = "LOCALAPPDATA"
	}
	return &RegistrationError{
		Reason:     fmt.Sprintf("no directory holds this machine's file, since %s is not set", variable),
		WayForward: fmt.Sprintf("set %s to an absolute path, then run `lym beam stop` and `lym beam start`, so the dobbies start with it", variable),
	}
}

// ensureIdentityWith is EnsureIdentity over the machine file locate names and the hostname rule's name hostname returns.
func ensureIdentityWith(locate machineFileLocator, hostname func() (string, error)) (machinename.Identity, error) {
	if refusal := identityRefusal(locate); refusal != nil {
		return machinename.Identity{}, refusal
	}
	path, lockPath, _, _ := locate()

	file, exists, err := machinename.ReadFile(path)
	if err != nil {
		return machinename.Identity{}, fmt.Errorf("the machine file %s does not read: %w; way forward: %s", path, err, machineFileWayForward)
	}
	hostName := ""
	if exists {
		if err := machinename.CheckName(file.Name); err != nil {
			return machinename.Identity{}, fmt.Errorf("the machine file %s holds no valid name: %w; way forward: %s", path, err, machineFileWayForward)
		}
		if file.ID != "" {
			return identityOf(file), nil
		}
	} else {
		if hostName, err = hostname(); err != nil {
			return machinename.Identity{}, err
		}
		// A file holding a name the name rule refuses would break every later read of it.
		if err := machinename.CheckName(hostName); err != nil {
			return machinename.Identity{}, fmt.Errorf("the hostname rule's name cannot start the machine file: %w", err)
		}
	}

	id, err := machinename.NewID()
	if err != nil {
		return machinename.Identity{}, err
	}
	var written machinename.File
	err = state.UpdateJSON(path, lockPath, func(current machinename.File, found bool) (machinename.File, error) {
		// Another writer may have given the file an id since the read above; its id stands.
		if found && current.ID != "" {
			written = current
			return current, nil
		}
		if !found {
			current = machinename.File{Name: hostName, Names: []string{hostName}}
		}
		current.ID = id
		written = current
		return current, nil
	})
	if err != nil {
		return machinename.Identity{}, fmt.Errorf("write the machine id into %s: %w", path, err)
	}
	return identityOf(written), nil
}

// identityOf returns the identity a machine file with a valid name yields.
// Its former names are the file's non-empty names other than the current one.
func identityOf(file machinename.File) machinename.Identity {
	var former []string
	for _, name := range file.Names {
		if name != "" && name != file.Name {
			former = append(former, name)
		}
	}
	return machinename.Identity{ID: file.ID, Name: file.Name, Former: former, Forgotten: file.Forgotten}
}
