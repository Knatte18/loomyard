// accountstore.go reads and rewrites the machine-local Claude account file and the per-account usage records.
// Both are caches under the hub's `.lym`, so an absent or undecodable file reads as the zero record.

package claudeengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// accountFile is the account file: which account each worktree runs under, and the counters a pick reads.
type accountFile struct {
	// Worktrees maps a worktree name to the name of its account.
	Worktrees map[string]string `json:"worktrees"`
	// Picked maps a worktree name to the time its entry in Worktrees was written.
	Picked map[string]time.Time `json:"picked"`
	// Counts maps the name of a positive-weight account to the number of worktrees picked onto it, as a float.
	Counts map[string]float64 `json:"counts"`
	// Weights maps an account name to the weight its count was taken under.
	Weights map[string]int `json:"weights"`
	// Listed holds the account names of the list the counters were taken under, in list order.
	Listed []string `json:"listed"`
}

// usageWindow is one usage window of an account as the status line reported it.
type usageWindow struct {
	// UsedPercentage is the share of the window's allowance already used.
	UsedPercentage float64 `json:"used_percentage"`
	// ResetsAt is the Unix time in seconds at which the window resets.
	ResetsAt int64 `json:"resets_at"`
}

// usageRecord is the usage record of one account.
type usageRecord struct {
	// Windows maps a window name, `five_hour` or `seven_day`, to its latest reading.
	Windows map[string]usageWindow `json:"windows"`
	// BlockedUntil is the time before which a usage limit blocks the account; the zero time means no block.
	BlockedUntil time.Time `json:"blocked_until,omitzero"`
}

// accountFilePath returns the path of the account file in dir.
func accountFilePath(dir string) string {
	return filepath.Join(dir, lymdirs.ClaudeAccountsFileName)
}

// usageFilePath returns the path of account's usage record in dir.
func usageFilePath(dir, account string) string {
	return filepath.Join(dir, lymdirs.ClaudeUsageDirName, account+".json")
}

// lockPathOf returns the lock path beside the cache file at path.
func lockPathOf(path string) string {
	return path + ".lock"
}

// readCacheRecord reads the JSON record at path under its shared lock.
// A missing file, an empty file and an undecodable one read as the zero record,
// and the undecodable one is logged at Warn, since a cache file is safe to remove.
// Any other read error is returned.
func readCacheRecord[T any](path string) (T, error) {
	record, _, err := state.ReadJSON[T](path, lockPathOf(path))
	if err == nil {
		return record, nil
	}
	var zero T
	if errors.Is(err, state.ErrDecode) {
		logger.Warn("claudeengine: a Claude account cache file does not decode, so it reads as empty; removing it is safe", "file", path, "err", err)
		return zero, nil
	}
	return zero, err
}

// updateCacheRecord replaces the JSON record at path by what mutate returns for it, under one hold of the file's lock.
// A missing, empty or undecodable file hands mutate the zero record, and the undecodable one is logged at Warn.
func updateCacheRecord[T any](path string, mutate func(current T) (T, error)) error {
	err := state.UpdateJSON(path, lockPathOf(path), func(current T, _ bool) (T, error) {
		return mutate(current)
	})
	if !errors.Is(err, state.ErrDecode) {
		return err
	}
	logger.Warn("claudeengine: a Claude account cache file does not decode, so it is replaced; removing it is safe", "file", path, "err", err)
	var zero T
	next, err := mutate(zero)
	if err != nil {
		return err
	}
	return state.WriteJSON(path, lockPathOf(path), next)
}

// readAccountFile reads the account file in dir.
// A missing or undecodable file reads as the zero accountFile.
func readAccountFile(dir string) (accountFile, error) {
	return readCacheRecord[accountFile](accountFilePath(dir))
}

// updateAccountFile rewrites the account file in dir with what mutate returns, under one hold of its lock.
// A key the file holds that accountFile does not declare survives the rewrite.
func updateAccountFile(dir string, mutate func(accountFile) (accountFile, error)) error {
	return updateCacheRecord(accountFilePath(dir), mutate)
}

// readUsage reads the usage record of each account of accounts from dir, keyed by account name.
// An account with no record, or an undecodable one, maps to the zero usageRecord.
func readUsage(dir string, accounts []AccountConfig) (map[string]usageRecord, error) {
	usage := make(map[string]usageRecord, len(accounts))
	for _, account := range accounts {
		record, err := readCacheRecord[usageRecord](usageFilePath(dir, account.Name))
		if err != nil {
			return nil, fmt.Errorf("read the usage record of account %q: %w", account.Name, err)
		}
		usage[account.Name] = record
	}
	return usage, nil
}

// mergeUsage merges the reading read into the stored record and returns the result.
// Per window a later reset replaces the stored window, the same reset replaces it only with a higher percentage,
// and an earlier reset is dropped.
// A window only one side holds is kept, and the stored BlockedUntil is always kept.
func mergeUsage(stored, read usageRecord) usageRecord {
	merged := usageRecord{Windows: make(map[string]usageWindow, len(stored.Windows)+len(read.Windows)), BlockedUntil: stored.BlockedUntil}
	for name, window := range stored.Windows {
		merged.Windows[name] = window
	}
	for name, window := range read.Windows {
		held, ok := merged.Windows[name]
		switch {
		case !ok, window.ResetsAt > held.ResetsAt:
			merged.Windows[name] = window
		case window.ResetsAt == held.ResetsAt && window.UsedPercentage > held.UsedPercentage:
			merged.Windows[name] = window
		}
	}
	return merged
}

// writeUsage merges the reading read into account's stored usage record in dir, under the record's lock.
func writeUsage(dir, account string, read usageRecord) error {
	return updateCacheRecord(usageFilePath(dir, account), func(stored usageRecord) (usageRecord, error) {
		return mergeUsage(stored, read), nil
	})
}

// recordBlocked sets account's BlockedUntil in dir to the later of until and the stored value, keeping its windows.
func recordBlocked(dir, account string, until time.Time) error {
	return updateCacheRecord(usageFilePath(dir, account), func(stored usageRecord) (usageRecord, error) {
		if until.After(stored.BlockedUntil) {
			stored.BlockedUntil = until
		}
		return stored, nil
	})
}
