// raddlestore.go holds the raddle store on the Bolt: one directory per Sketch a raddle run filed.
// Its store.json records what the run filed, and the reads here answer which slugs the store holds.

package boardengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const (
	// raddleStoreDirName is the directory beside board.json that holds one store directory per Sketch a raddle run filed.
	raddleStoreDirName = "raddle"
	// raddleStoreFileName is the file of a store directory that records what the run filed.
	raddleStoreFileName = "store.json"
)

// RaddleStoreEntry is the record of one Sketch a raddle run filed: store.json of its store directory.
type RaddleStoreEntry struct {
	// Sketch is the slug of the Sketch and names its store directory; store.json has no such key.
	Sketch string `json:"-"`
	// Created is the Sketch's created stamp, which tells the live run's own Sketch from a later entry under the same slug.
	Created time.Time `json:"created,omitzero"`
	// Hash is the hash of the approved proposal.
	Hash string `json:"hash"`
	// Filed lists every slug the run filed, the project's included.
	Filed []string `json:"filed"`
	// Project is the slug of the project the run filed, set only for the project and beam shapes; it is also in Filed.
	Project string `json:"project,omitempty"`
}

// RaddleStore is every store.json under the raddle store root, in Sketch slug order.
type RaddleStore struct {
	entries []RaddleStoreEntry
}

// Holds returns the entry of the Sketch whose slug is slug, or of the Sketch whose run filed slug.
// The second result reports whether the store holds either.
func (s RaddleStore) Holds(slug string) (RaddleStoreEntry, bool) {
	for _, entry := range s.entries {
		if entry.Sketch == slug || slices.Contains(entry.Filed, slug) {
			return entry, true
		}
	}
	return RaddleStoreEntry{}, false
}

// Slugs returns the slug of every Sketch the store holds and every slug those Sketches filed.
func (s RaddleStore) Slugs() []string {
	var slugs []string
	for _, entry := range s.entries {
		slugs = append(slugs, entry.Sketch)
		slugs = append(slugs, entry.Filed...)
	}
	return slugs
}

// RaddleProposal returns the approved proposal.yaml of the store directory that holds slug, reading without the lock.
// slug is the Sketch's own slug or any slug the Sketch's run filed, and found is false when the store holds neither.
// The store outlives the prune, so the answer holds after the Sketch is pruned.
func (b *Board) RaddleProposal(slug string) ([]byte, bool, error) {
	store, err := ReadRaddleStore(b.boardPath)
	if err != nil {
		return nil, false, err
	}
	entry, held := store.Holds(slug)
	if !held {
		return nil, false, nil
	}
	proposal, err := os.ReadFile(filepath.Join(RaddleStoreDir(b.boardPath, entry.Sketch), raddleProposalFileName))
	if err != nil {
		return nil, false, fmt.Errorf("read the approved proposal of Sketch %q: %w", entry.Sketch, err)
	}
	return proposal, true, nil
}

// RaddleStoreDir returns the store directory of the Sketch sketch under boardDir.
func RaddleStoreDir(boardDir, sketch string) string {
	return filepath.Join(boardDir, raddleStoreDirName, sketch)
}

// RaddleStoreRootRel returns the slash-separated path of the raddle store root relative to the board checkout's root, for a read of a board commit's tree.
func RaddleStoreRootRel() string {
	return raddleStoreDirName
}

// RaddleStoreRecordRel returns the slash-separated path of the Sketch's store.json relative to the board checkout's root.
func RaddleStoreRecordRel(sketch string) string {
	return raddleStoreFileRel(sketch, raddleStoreFileName)
}

// RaddleProposalRel returns the slash-separated path of the Sketch's approved proposal relative to the board checkout's root.
func RaddleProposalRel(sketch string) string {
	return raddleStoreFileRel(sketch, raddleProposalFileName)
}

// RaddleColdProposalRel returns the slash-separated path of the Sketch's cold proposal relative to the board checkout's root.
func RaddleColdProposalRel(sketch string) string {
	return raddleStoreFileRel(sketch, raddleColdProposalFileName)
}

// raddleStoreFileRel returns the slash-separated path of the file name in the Sketch's store directory relative to the board checkout's root.
func raddleStoreFileRel(sketch, name string) string {
	return filepath.ToSlash(filepath.Join(raddleStoreDirName, sketch, name))
}

// ParseRaddleStoreEntry decodes the bytes of one store.json.
// The returned entry's Sketch is empty, since store.json holds no such key.
func ParseRaddleStoreEntry(data []byte) (RaddleStoreEntry, error) {
	var entry RaddleStoreEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return RaddleStoreEntry{}, fmt.Errorf("decode the raddle store record: %w", err)
	}
	return entry, nil
}

// ReadRaddleStore reads every store.json under boardDir's raddle store root; an absent root holds none.
// A store directory without a store.json holds nothing.
// A store.json that cannot be read or decoded is an error that names the file and the git checkout that restores it from the Bolt's history.
func ReadRaddleStore(boardDir string) (RaddleStore, error) {
	root := filepath.Join(boardDir, raddleStoreDirName)
	directories, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return RaddleStore{}, nil
	}
	if err != nil {
		return RaddleStore{}, fmt.Errorf("read the raddle store %s: %w", root, err)
	}
	var store RaddleStore
	for _, directory := range directories {
		if !directory.IsDir() {
			continue
		}
		path := filepath.Join(root, directory.Name(), raddleStoreFileName)
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		var entry RaddleStoreEntry
		if err == nil {
			entry, err = ParseRaddleStoreEntry(data)
		}
		if err != nil {
			relative := filepath.ToSlash(filepath.Join(raddleStoreDirName, directory.Name(), raddleStoreFileName))
			return RaddleStore{}, fmt.Errorf("the raddle store file %s cannot be read: %w: restore it from the Bolt's history with `git -C %s checkout -- %s`", path, err, boardDir, relative)
		}
		entry.Sketch = directory.Name()
		store.entries = append(store.entries, entry)
	}
	return store, nil
}
