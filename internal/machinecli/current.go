// current.go reads this machine's current name state from the machine file and the hostname rule, and shows it.

package machinecli

import (
	"fmt"
	"io"

	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/output"
)

// Where a current name comes from.
const (
	sourceFile     = "file"
	sourceHostname = "hostname"
)

// current is what readCurrent reads: the machine file's state and the name it yields.
type current struct {
	// file and lock are the machine file's and its lock's paths; hasDirectory is false when no directory resolved.
	file, lock   string
	hasDirectory bool
	// locateErr is the error of a relative variable, which names the variable.
	locateErr error
	// exists reports whether the machine file exists; decoded is its content when it decodes.
	exists  bool
	decoded machinename.File
	// readErr is a file that does not read; decodeErr is a file that exists and does not decode.
	readErr, decodeErr error
	// nameErr is why a decoded file's name is invalid, nil when it is valid.
	nameErr error
	// name is the valid current name, "" when there is none.
	// It is the file's name, else the hostname rule's name when no file or no directory exists.
	name string
	// source is sourceFile or sourceHostname when name is set.
	source string
	// former holds the decoded file's non-empty names other than the current name; empty for a hostname name.
	former []string
	// hostname is the hostname rule's name of this host, "" when hostName errors, which hostErr carries.
	hostname string
	hostErr  error
}

// readCurrent reads the machine file through machinename.ReadFile, never machinename.Name, so a bad file stays readable.
func readCurrent(d deps) current {
	cur := current{former: []string{}}
	cur.file, cur.lock, cur.hasDirectory, cur.locateErr = d.locate()
	cur.hostname, cur.hostErr = d.hostName()
	if cur.locateErr != nil {
		return cur
	}
	if cur.hasDirectory {
		file, exists, err := machinename.ReadFile(cur.file)
		cur.exists = exists
		switch {
		case err != nil && exists:
			cur.decodeErr = err
		case err != nil:
			cur.readErr = err
		default:
			cur.decoded = file
		}
	}
	switch {
	case cur.hasDirectory && cur.exists && cur.decodeErr == nil:
		if cur.nameErr = machinename.CheckName(cur.decoded.Name); cur.nameErr != nil {
			break
		}
		cur.name, cur.source = cur.decoded.Name, sourceFile
		for _, name := range cur.decoded.Names {
			if name != "" && name != cur.name {
				cur.former = append(cur.former, name)
			}
		}
	case cur.hasDirectory && (cur.exists || cur.readErr != nil):
		// A file that exists but yields no name never falls back to the hostname, as machinename.Name never does.
	case cur.hostErr == nil:
		cur.name, cur.source = cur.hostname, sourceHostname
	}
	return cur
}

// showName prints the machine name state under `machine` and returns the exit code.
func showName(out io.Writer, d deps) int {
	cur := readCurrent(d)
	fields := map[string]any{"file": cur.file}
	switch {
	case cur.locateErr != nil:
		return output.Err(out, cur.locateErr.Error())
	case cur.readErr != nil:
		return output.ErrFields(out, fmt.Sprintf("the machine file %s does not read: %v; way forward: remove the file or fix its permissions, then run `lym machine name <name>`", cur.file, cur.readErr), fields)
	case cur.decodeErr != nil:
		return output.ErrFields(out, fmt.Sprintf("the machine file %s does not decode: %v; way forward: run `lym machine name <name>`, which rewrites the file, or remove the file", cur.file, cur.decodeErr), fields)
	case cur.nameErr != nil:
		return output.ErrFields(out, fmt.Sprintf("the machine file %s holds no valid name: %v; way forward: run `lym machine name <name>`, which rewrites the file, or remove the file", cur.file, cur.nameErr), fields)
	case cur.name == "":
		return output.Err(out, cur.hostErr.Error())
	}
	return output.Ok(out, map[string]any{"machine": map[string]any{
		"name":   cur.name,
		"source": cur.source,
		"file":   cur.file,
		"former": cur.former,
	}})
}
