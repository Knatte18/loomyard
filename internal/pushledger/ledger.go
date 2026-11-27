// ledger.go reads and writes the ledger's entries and the hub's clear time.

package pushledger

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

const (
	ledgerLockName = "ledger.lock"
	clearFileName  = "clear.json"
	entrySuffix    = ".json"
	lockSuffix     = ".lock"
)

// Key names one push target of a hub.
type Key string

const (
	// BoardKey is the target of the board's branch.
	BoardKey Key = "board"
	// LandingKey is the landing target of the prime's code branch.
	LandingKey Key = "landing"
)

// RecordsKey returns the key of the prime's or a pair's target: the base name of its records worktree.
func RecordsKey(recordsWorktree string) Key {
	return Key(filepath.Base(recordsWorktree))
}

// Entry is what the ledger holds of one target.
type Entry struct {
	// Worktree is the target's code worktree path, empty for the board.
	Worktree string `json:"worktree,omitempty"`
	// Slug names the run the target belongs to.
	Slug string `json:"slug,omitempty"`
	// FirstOwed is when the target's push first became owed, zero while no push is.
	FirstOwed time.Time `json:"first_owed,omitzero"`
	// LastAttempt is when the push was last tried.
	LastAttempt time.Time `json:"last_attempt,omitzero"`
	// IntervalS is the push interval in force, in seconds.
	IntervalS int `json:"interval_s,omitempty"`
	// PushNow asks the target's pusher to push without waiting out the interval.
	PushNow bool `json:"push_now,omitempty"`
	// Landings are the landings on the target's branch that are not yet pushed, in landing order.
	Landings []Landing `json:"landings,omitempty"`
	// HeldRecords is the records commit a dropped landing leaves held on the run's own pair target until the pair goes.
	HeldRecords string `json:"held_records,omitempty"`
	// Dropped maps each run whose landing was dropped from a parent target to its landing commit.
	// A run stays listed while that commit stands on the parent past origin, so no read turns it back into a landing.
	Dropped map[string]string `json:"dropped,omitempty"`
	// HandOvers maps each run still to be handed over, its landing gone from a parent target, to its owner machine.
	HandOvers map[string]string `json:"hand_overs,omitempty"`
}

// Owed reports whether the entry has a push still to make: it holds landings or a first-owed time.
func (e Entry) Owed() bool {
	return len(e.Landings) > 0 || !e.FirstOwed.IsZero()
}

// Landing is one run's landing on a parent branch whose push is still owed.
type Landing struct {
	// Run is the landed run's slug.
	Run string `json:"run"`
	// Tip is the parent tip the landing made.
	Tip string `json:"tip"`
	// RecordsCommit is the run's records commit that its pair target holds back while the landing stands.
	RecordsCommit string `json:"records_commit,omitempty"`
	// ParentBranch is the branch landed on.
	ParentBranch string `json:"parent_branch"`
	// At is when the run landed.
	At time.Time `json:"at"`
}

// clearRecord is the hub file holding the time the hub last had no owed target.
type clearRecord struct {
	At time.Time `json:"at"`
}

func ledgerDir(hubPath string) string {
	return filepath.Join(fabriclayout.HubScratchDir(hubPath), lymdirs.PushLedgerDirName)
}

// EntryPath returns the file of the target key in the hub's ledger.
func EntryPath(hubPath string, key Key) string {
	return filepath.Join(ledgerDir(hubPath), url.PathEscape(string(key))+entrySuffix)
}

// Read returns the entry of key, reporting false when the ledger holds none.
func Read(hubPath string, key Key) (Entry, bool, error) {
	path := EntryPath(hubPath, key)
	entry, found, err := state.ReadJSON[Entry](path, path+lockSuffix)
	if err != nil {
		return Entry{}, false, fmt.Errorf("read the push ledger entry %s: %w", key, err)
	}
	return entry, found, nil
}

// Update replaces the entry of key with what mutate makes of it, or removes it when mutate answers keep false.
// The ledger lock is held across the read, mutate and the write, so mutate must not write the ledger itself.
// An error from mutate writes nothing.
// The hub's clear time becomes now when the write leaves no entry owed.
// It also becomes now when the write takes the hub from no owed entry to one.
func Update(hubPath string, key Key, now time.Time, mutate func(cur Entry, found bool) (next Entry, keep bool, err error)) error {
	dir := ledgerDir(hubPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create the push ledger: %w", err)
	}
	held, err := lock.AcquireWriteLock(filepath.Join(dir, ledgerLockName))
	if err != nil {
		return fmt.Errorf("lock the push ledger: %w", err)
	}
	defer held.Release()

	entries, err := readEntries(hubPath)
	if err != nil {
		return err
	}
	cur, found := entries[key]
	owedBefore := anyOwed(entries)
	next, keep, err := mutate(cur, found)
	if err != nil {
		return err
	}

	path := EntryPath(hubPath, key)
	if keep {
		if err := state.WriteJSON(path, path+lockSuffix, next); err != nil {
			return fmt.Errorf("write the push ledger entry %s: %w", key, err)
		}
		entries[key] = next
	} else {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove the push ledger entry %s: %w", key, err)
		}
		delete(entries, key)
	}

	if owedBefore && anyOwed(entries) {
		return nil
	}
	clearPath := filepath.Join(dir, clearFileName)
	if err := state.WriteJSON(clearPath, clearPath+lockSuffix, clearRecord{At: now}); err != nil {
		return fmt.Errorf("write the push ledger's clear time: %w", err)
	}
	return nil
}

func anyOwed(entries map[Key]Entry) bool {
	for _, entry := range entries {
		if entry.Owed() {
			return true
		}
	}
	return false
}

// List returns every entry of the hub's ledger by key, read as one snapshot.
func List(hubPath string) (map[Key]Entry, error) {
	dir := ledgerDir(hubPath)
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return map[Key]Entry{}, nil
	}
	held, err := lock.AcquireReadLock(filepath.Join(dir, ledgerLockName))
	if err != nil {
		return nil, fmt.Errorf("lock the push ledger: %w", err)
	}
	defer held.Release()
	return readEntries(hubPath)
}

// readEntries reads every entry file of an existing ledger directory.
// The caller holds the ledger lock.
func readEntries(hubPath string) (map[Key]Entry, error) {
	files, err := os.ReadDir(ledgerDir(hubPath))
	if err != nil {
		return nil, fmt.Errorf("list the push ledger: %w", err)
	}
	entries := map[Key]Entry{}
	for _, file := range files {
		name, isEntry := strings.CutSuffix(file.Name(), entrySuffix)
		if !isEntry || file.Name() == clearFileName {
			continue
		}
		unescaped, err := url.PathUnescape(name)
		if err != nil {
			return nil, fmt.Errorf("read the push ledger file name %s: %w", file.Name(), err)
		}
		entry, found, err := Read(hubPath, Key(unescaped))
		if err != nil {
			return nil, err
		}
		if found {
			entries[Key(unescaped)] = entry
		}
	}
	return entries, nil
}

// LastClear returns when the hub last had no owed target, zero when no time was written.
func LastClear(hubPath string) (time.Time, error) {
	path := filepath.Join(ledgerDir(hubPath), clearFileName)
	record, _, err := state.ReadJSON[clearRecord](path, path+lockSuffix)
	if err != nil {
		return time.Time{}, fmt.Errorf("read the push ledger's clear time: %w", err)
	}
	return record.At, nil
}

// OwedTips returns the parent tip of each landing standing on key, in landing order.
func OwedTips(hubPath string, key Key) ([]string, error) {
	entry, _, err := Read(hubPath, key)
	if err != nil {
		return nil, err
	}
	var tips []string
	for _, landing := range entry.Landings {
		tips = append(tips, landing.Tip)
	}
	return tips, nil
}

// HeldRecordsCommit returns the records commit of the run slug that no push may yet carry.
// It is the commit of the run's oldest standing landing on any target.
// With no standing landing, it is the one a dropped landing left held on the run's own entry.
// held is false when the run has none.
func HeldRecordsCommit(hubPath, slug string) (commit string, held bool, err error) {
	entries, err := List(hubPath)
	if err != nil {
		return "", false, err
	}
	var oldest *Landing
	for _, entry := range entries {
		for _, landing := range entry.Landings {
			if landing.Run == slug && (oldest == nil || landing.At.Before(oldest.At)) {
				oldest = &landing
			}
		}
	}
	if oldest != nil {
		return oldest.RecordsCommit, oldest.RecordsCommit != "", nil
	}
	for _, entry := range entries {
		if entry.Slug == slug && entry.HeldRecords != "" {
			return entry.HeldRecords, true, nil
		}
	}
	return "", false, nil
}
