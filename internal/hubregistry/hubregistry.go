// hubregistry.go keeps the machine record of hubs: Record writes a hub into it, and Lookup and Live read it.

package hubregistry

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// RecordFileName is the name of the machine record of hubs, inside the directory the caller passes.
const RecordFileName = "hubs.json"

// lockFileName is the name of the record's lock file, beside the record, since the record belongs to no hub.
const lockFileName = "hubs.lock"

// ErrUnresolved is wrapped by the refusal of a shortname that names no hub or more than one hub on this machine.
// A record that cannot be read does not match it.
var ErrUnresolved = errors.New("creel: shortname does not name exactly one hub")

// unresolvedError carries a refusal's own text and matches ErrUnresolved.
type unresolvedError string

func (e unresolvedError) Error() string { return string(e) }

func (e unresolvedError) Is(target error) bool { return target == ErrUnresolved }

// Entry is one hub of the machine record.
type Entry struct {
	// HubPath is the absolute path of the hub.
	HubPath string `json:"hub_path"`
	// Shortname is the shortname the hub recorded itself under.
	Shortname string `json:"shortname"`
	// RecordedAt is the time the hub last recorded itself.
	RecordedAt time.Time `json:"recorded_at"`
}

// recordFile is the JSON object the record holds.
type recordFile struct {
	Hubs []Entry `json:"hubs"`
}

// Record upserts hubPath with shortname and now into the record in dir.
// It drops every other entry whose hub is certainly gone.
// An entry whose Bolt record is absent, and one whose path cannot be told gone, is kept.
// The read and the write happen under one lock, so concurrent daemons lose no entry.
func Record(dir, hubPath, shortname string, now time.Time) error {
	path, lockPath := recordPaths(dir)
	err := state.UpdateJSON(path, lockPath, func(cur recordFile, _ bool) (recordFile, error) {
		next := recordFile{Hubs: []Entry{{HubPath: hubPath, Shortname: shortname, RecordedAt: now}}}
		for _, entry := range cur.Hubs {
			if entry.HubPath == hubPath || entryGone(entry) {
				continue
			}
			next.Hubs = append(next.Hubs, entry)
		}
		return next, nil
	})
	return refuseUnreadable(path, err)
}

// Lookup answers the path of the one hub in dir's record that recorded shortname and whose Bolt record reads it.
// It refuses, wrapping ErrUnresolved, when no hub or several hubs read it, and the refusal names the way forward.
func Lookup(dir, shortname string) (string, error) {
	entries, err := readEntries(dir)
	if err != nil {
		return "", err
	}
	var paths []string
	for _, entry := range entries {
		if entry.Shortname != shortname {
			continue
		}
		if got, found := fabricengine.ReadShortname(fabricengine.BoltDir(entry.HubPath)); found && got == shortname {
			paths = append(paths, entry.HubPath)
		}
	}
	sort.Strings(paths)
	switch len(paths) {
	case 1:
		return paths[0], nil
	case 0:
		return "", unresolvedError(fmt.Sprintf("creel: no hub with the shortname %q is recorded on this machine; way forward: run lym creel inbox in that hub once", shortname))
	default:
		return "", unresolvedError(fmt.Sprintf("creel: mail across hubs needs distinct shortnames, but %d hubs on this machine read the shortname %q; way forward: remove the extra clone, since a recorded shortname never changes; the hubs are %s", len(paths), shortname, strings.Join(paths, ", ")))
	}
}

// Live answers the entries of dir's record whose Bolt record reads the entry's own shortname, in shortname order.
func Live(dir string) ([]Entry, error) {
	entries, err := readEntries(dir)
	if err != nil {
		return nil, err
	}
	var live []Entry
	for _, entry := range entries {
		if got, found := fabricengine.ReadShortname(fabricengine.BoltDir(entry.HubPath)); found && got == entry.Shortname {
			live = append(live, entry)
		}
	}
	sort.Slice(live, func(i, j int) bool {
		if live[i].Shortname != live[j].Shortname {
			return live[i].Shortname < live[j].Shortname
		}
		return live[i].HubPath < live[j].HubPath
	})
	return live, nil
}

// readEntries reads the record in dir; an absent file reads as no entries.
func readEntries(dir string) ([]Entry, error) {
	path, lockPath := recordPaths(dir)
	file, _, err := state.ReadJSON[recordFile](path, lockPath)
	if err != nil {
		return nil, refuseUnreadable(path, err)
	}
	return file.Hubs, nil
}

// recordPaths returns the record file and its lock file inside dir.
func recordPaths(dir string) (path, lockPath string) {
	return filepath.Join(dir, RecordFileName), filepath.Join(dir, lockFileName)
}

// entryGone reports whether the entry's hub is certainly gone.
// The hub is gone when its path does not exist, or when its Bolt record reads another shortname.
// A path a stat cannot settle and an absent Bolt record keep the entry.
func entryGone(entry Entry) bool {
	if _, err := os.Stat(entry.HubPath); errors.Is(err, fs.ErrNotExist) {
		return true
	}
	got, found := fabricengine.ReadShortname(fabricengine.BoltDir(entry.HubPath))
	return found && got != entry.Shortname
}

// refuseUnreadable turns a record that cannot be decoded into a refusal naming the file and the way forward.
// Any other error passes through unchanged.
func refuseUnreadable(path string, err error) error {
	if errors.Is(err, state.ErrDecode) {
		return fmt.Errorf("creel: the machine record of hubs %s cannot be decoded: %w; way forward: remove %s, after which lym creel inbox in each hub, a minute on, records it again", path, err, path)
	}
	return err
}
