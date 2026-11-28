// usage.go declares the usage records a run keeps under its worktree's _lym/usage: one file per agent session, the run's identity and its landed diff size, with the writer of each and the one reader of the directory.

package shedrun

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// Names of the files in the usage directory.
const (
	runUsageFileName       = "run.json"
	landedSizeFileName     = "landed.json"
	sessionUsageFilePrefix = "session-"
	usageFileSuffix        = ".json"
	// usageLockSuffix ends the name of each record's lock, which lock.TrackedFileLockPath places outside the tracked usage directory.
	usageLockSuffix = ".lock"
)

// UsageDirRel returns the worktree-anchor-relative path of the usage directory, the join of lymdirs.LymDirName and lymdirs.UsageDirName.
func UsageDirRel() string {
	return filepath.Join(lymdirs.LymDirName, lymdirs.UsageDirName)
}

// UsageDir returns the absolute path of the durable usage directory: the given *lymcwd.Location's AnchorPath() joined with UsageDirRel().
func UsageDir(l *lymcwd.Location) string {
	return filepath.Join(l.AnchorPath(), UsageDirRel())
}

// ModelUsage is the usage of one model within a session: its four token counts and its cost in USD.
// CostUSD is nil when any message of the model had no known price.
type ModelUsage struct {
	Model            string   `json:"model"`
	InputTokens      int64    `json:"input_tokens"`
	CacheWriteTokens int64    `json:"cache_write_tokens"`
	CacheReadTokens  int64    `json:"cache_read_tokens"`
	OutputTokens     int64    `json:"output_tokens"`
	CostUSD          *float64 `json:"cost_usd"`
}

// SessionUsageRecord is the usage of one agent session, written as session-<session id>.json.
// Row is the run row that spawned the session.
// Each token count is nil when the reading is unknown, and CostUSD is nil when any message had no known price.
// Started and Ended are RFC 3339 UTC and absent when unknown.
// Models is set only when the session's messages, a fork's included, ran on more than one model.
type SessionUsageRecord struct {
	SessionID        string       `json:"session_id"`
	Row              string       `json:"row"`
	Role             string       `json:"role"`
	Model            string       `json:"model"`
	Engine           string       `json:"engine"`
	InputTokens      *int64       `json:"input_tokens"`
	CacheWriteTokens *int64       `json:"cache_write_tokens"`
	CacheReadTokens  *int64       `json:"cache_read_tokens"`
	OutputTokens     *int64       `json:"output_tokens"`
	CostUSD          *float64     `json:"cost_usd"`
	Started          string       `json:"started,omitempty"`
	Ended            string       `json:"ended,omitempty"`
	Models           []ModelUsage `json:"models,omitempty"`
}

// RunUsageRecord is the identity of a run for its cost report, written as run.json when the run is seeded.
type RunUsageRecord struct {
	Recipe       string `json:"recipe"`
	Profile      string `json:"profile"`
	StoredRecord bool   `json:"stored_record"`
}

// LandedSizeRecord is the size of the diff a run landed, written as landed.json at its Finalize.
type LandedSizeRecord struct {
	LinesAdded   int `json:"lines_added"`
	LinesDeleted int `json:"lines_deleted"`
	FilesChanged int `json:"files_changed"`
}

// RunUsage is the content of a usage directory: the run record and the landed-size record, each with a flag saying it was found, and the session records sorted by file name.
type RunUsage struct {
	Run         RunUsageRecord
	RunFound    bool
	Landed      LandedSizeRecord
	LandedFound bool
	Sessions    []SessionUsageRecord
}

// WriteSessionUsage writes rec as session-<rec.SessionID>.json in dir, replacing the file of the same session id and keeping any key of it that SessionUsageRecord does not declare.
// It refuses a session id that is empty or not a single path segment.
func WriteSessionUsage(dir string, rec SessionUsageRecord) error {
	if rec.SessionID == "" || rec.SessionID == "." || rec.SessionID == ".." || rec.SessionID != filepath.Base(rec.SessionID) {
		return fmt.Errorf("shedrun: usage record session id %q is not a single path segment", rec.SessionID)
	}
	return writeUsageRecord(filepath.Join(dir, sessionUsageFilePrefix+rec.SessionID+usageFileSuffix), rec)
}

// WriteRunUsage writes rec as run.json in dir.
func WriteRunUsage(dir string, rec RunUsageRecord) error {
	return writeUsageRecord(filepath.Join(dir, runUsageFileName), rec)
}

// WriteLandedSize writes rec as landed.json in dir.
func WriteLandedSize(dir string, rec LandedSizeRecord) error {
	return writeUsageRecord(filepath.Join(dir, landedSizeFileName), rec)
}

// writeUsageRecord writes rec to path atomically through internal/state under the lock of a tracked file.
func writeUsageRecord[T any](path string, rec T) error {
	if err := state.WriteJSON(path, lock.TrackedFileLockPath(path, usageLockSuffix), rec); err != nil {
		return fmt.Errorf("shedrun: write usage record %s: %w", path, err)
	}
	return nil
}

// ParseRunUsage reads a usage directory's files, given as a map from file name to bytes the way a git tree read hands them.
// A file of another name is ignored, and a key a record does not declare is accepted.
// A file that does not decode is an error naming it.
func ParseRunUsage(files map[string][]byte) (RunUsage, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var usage RunUsage
	for _, name := range names {
		var err error
		switch {
		case name == runUsageFileName:
			err = json.Unmarshal(files[name], &usage.Run)
			usage.RunFound = err == nil
		case name == landedSizeFileName:
			err = json.Unmarshal(files[name], &usage.Landed)
			usage.LandedFound = err == nil
		case strings.HasPrefix(name, sessionUsageFilePrefix) && strings.HasSuffix(name, usageFileSuffix):
			var record SessionUsageRecord
			if err = json.Unmarshal(files[name], &record); err == nil {
				usage.Sessions = append(usage.Sessions, record)
			}
		}
		if err != nil {
			return RunUsage{}, fmt.Errorf("shedrun: usage record %s does not decode: %w", name, err)
		}
	}
	return usage, nil
}
