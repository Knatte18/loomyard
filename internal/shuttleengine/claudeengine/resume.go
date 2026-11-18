// resume.go implements Claude's resume capabilities.
// It carries a session's transcript into the account a resume runs under.
// It checks that a new run may take over a session.
// It reads the live process that holds one.
// The transcript is read under the project directory of the resolved account's config directory.
// The session registries of every known config directory are read as one set.
// Every fact about either layout stays in this file, per the Shuttle Provider-Seam Invariant.

package claudeengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var (
	_ shuttleengine.TranscriptCarrier   = (*Claude)(nil)
	_ shuttleengine.SessionResumer      = (*Claude)(nil)
	_ shuttleengine.SessionHolderFinder = (*Claude)(nil)
)

// registryEntry is the subset of one Claude session-registry file that the resume check reads.
type registryEntry struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	ProcStart string `json:"procStart"`
}

// resumeConfigDir returns the config directory a resume in the told worktree runs under.
// An engine told no hub board directory takes the first known directory.
// A claude.yaml or env module that does not load, or an account that does not resolve, is an error.
func (c *Claude) resumeConfigDir() (string, error) {
	if c.dirs.Board == "" {
		known := c.knownConfigDirs()
		if len(known) == 0 {
			return "", errors.New("claudeengine: no Claude config directory to resume a session under; way forward: set HOME")
		}
		return known[0], nil
	}
	claude, err := LoadConfig(c.dirs)
	if err != nil {
		return "", fmt.Errorf("claudeengine: load claude.yaml to resolve the resumed session's account: %w", err)
	}
	_, launched, err := c.resolveLaunchAccount(claude)
	if err != nil {
		return "", fmt.Errorf("claudeengine: resolve the account the resumed session runs under: %w", err)
	}
	return launched.ConfigDir, nil
}

// CarryTranscript implements shuttleengine.TranscriptCarrier.
// It finds the newest transcript of sessionID for workdir in every known config directory.
// When the directory the resume runs under already holds it, nothing is copied and it reports false.
// Otherwise it copies the transcript there with its session directory, leaves the source in place, and reports true.
// A session no known directory holds is copied nowhere and reports false.
// A malformed session id is refused before any file is touched.
func (c *Claude) CarryTranscript(sessionID, workdir string) (bool, error) {
	if err := validateSessionID(sessionID); err != nil {
		return false, fmt.Errorf("%w; Claude's /status shows the session id", err)
	}
	resolved, err := c.resumeConfigDir()
	if err != nil {
		return false, err
	}
	// The resolved directory leads, so a copy written as late as the newest one stays where it is.
	source, found := newestTranscript(append([]string{resolved}, c.knownConfigDirs()...), workdir, sessionID)
	if !found {
		return false, nil
	}
	target := projectDirUnder(filepath.Join(resolved, projectsDirName), workdir)
	if filepath.Dir(source) == target {
		return false, nil
	}
	if err := copyTranscript(filepath.Dir(source), target, sessionID); err != nil {
		return false, err
	}
	return true, nil
}

// copyTranscript copies sessionID's transcript and its session directory between project directories.
// It reads from fromProject and creates toProject when it is absent.
// A file already in toProject is replaced whole.
// The transcript is written last, so a copy that failed part way leaves no newer transcript than the source's.
func copyTranscript(fromProject, toProject, sessionID string) error {
	sessionDir := filepath.Join(fromProject, sessionID)
	if info, err := os.Stat(sessionDir); err == nil && info.IsDir() {
		walk := func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(fromProject, path)
			if err != nil {
				return err
			}
			target := filepath.Join(toProject, relative)
			if entry.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			return copyFile(path, target)
		}
		if err := filepath.WalkDir(sessionDir, walk); err != nil {
			return fmt.Errorf("claudeengine: copy the session directory %s into %s: %w", sessionDir, toProject, err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("claudeengine: stat the session directory %s: %w", sessionDir, err)
	}

	name := sessionID + ".jsonl"
	if err := copyFile(filepath.Join(fromProject, name), filepath.Join(toProject, name)); err != nil {
		return fmt.Errorf("claudeengine: copy the transcript of session %s into %s: %w", sessionID, toProject, err)
	}
	return nil
}

// copyFile writes the content of source to target atomically, creating target's directory.
func copyFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return fsx.AtomicWriteBytes(target, data)
}

// CheckResume implements shuttleengine.SessionResumer.
// It checks that the session has a transcript for workdir under the directory the resume runs under,
// and that no live process holds it according to the registries of every known config directory.
func (c *Claude) CheckResume(sessionID, workdir string) (string, error) {
	resolved, err := c.resumeConfigDir()
	if err != nil {
		return "", err
	}
	projectDir := projectDirUnder(filepath.Join(resolved, projectsDirName), workdir)
	return checkResumable(sessionID, projectDir, c.registryDirs(), proc.IsAlive, proc.StartTime)
}

// SessionHolder implements shuttleengine.SessionHolderFinder.
// It reads the session registries of every known config directory as one set.
func (c *Claude) SessionHolder(sessionID string) (int, bool, error) {
	return sessionHolder(sessionID, c.registryDirs(), proc.IsAlive, proc.StartTime)
}

// sessionsDirName is the directory of a Claude config directory that holds the session registry.
const sessionsDirName = "sessions"

// registryDirs returns the session registry directory of every known config directory.
func (c *Claude) registryDirs() []string {
	known := c.knownConfigDirs()
	registries := make([]string, len(known))
	for i, dir := range known {
		registries[i] = filepath.Join(dir, sessionsDirName)
	}
	return registries
}

// registryHoldsLiveEntry reports whether the registry in registryDir holds an entry for sessionID whose process is alive.
// An unreadable registry or entry holds none.
func registryHoldsLiveEntry(registryDir, sessionID string) bool {
	records, err := readRegistry(registryDir)
	if err != nil {
		return false
	}
	for _, record := range records {
		if record.ReadErr != nil || record.DecodeErr != nil || record.Entry.SessionID != sessionID {
			continue
		}
		if held, _ := liveHolder(record.Entry, proc.IsAlive, proc.StartTime); held {
			return true
		}
	}
	return false
}

// checkResumable refuses a resume unless sessionID is well-formed and has a transcript under projectDir.
// A missing transcript refuses with an error wrapping shuttleengine.ErrNoTranscript, naming projectDir.
// It also refuses while a live process in any of registryDirs holds the session, with a *shuttleengine.SessionHeldError.
// A live pid refuses unless its start time differs from the entry's procStart, which proves the pid was reused.
// When either start time cannot be read, the pid still refuses, so two processes never drive one session.
// An unreadable registry returns a warning and the check proceeds.
// It kills, signals and edits nothing.
func checkResumable(sessionID, projectDir string, registryDirs []string, alive func(pid int) bool, startTime func(pid int) (string, bool)) (warning string, err error) {
	if err := validateSessionID(sessionID); err != nil {
		return "", fmt.Errorf("%w; Claude's /status shows the session id", err)
	}

	transcript := filepath.Join(projectDir, sessionID+".jsonl")
	if _, err := os.Stat(transcript); errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("claudeengine: no transcript for session %s in %s; only a session run from this directory can be resumed: %w: %w", sessionID, projectDir, shuttleengine.ErrNoTranscript, err)
	} else if err != nil {
		return "", fmt.Errorf("claudeengine: cannot stat the transcript of session %s: %w", sessionID, err)
	}

	for _, registryDir := range registryDirs {
		records, err := readRegistry(registryDir)
		if err != nil {
			warning = fmt.Sprintf("claudeengine: could not read session registry %s (%v); a live holder of session %s could not be ruled out", registryDir, err, sessionID)
			continue
		}

		for _, record := range records {
			if record.ReadErr != nil {
				warning = fmt.Sprintf("claudeengine: could not read session registry entry %s (%v); a live holder of session %s could not be ruled out", record.Path, record.ReadErr, sessionID)
				continue
			}
			if record.DecodeErr != nil {
				warning = fmt.Sprintf("claudeengine: could not decode session registry entry %s (%v); a live holder of session %s could not be ruled out", record.Path, record.DecodeErr, sessionID)
				continue
			}
			entry := record.Entry
			if entry.SessionID != sessionID {
				continue
			}
			held, proven := liveHolder(entry, alive, startTime)
			if !held {
				continue
			}
			unproven := ""
			if !proven {
				unproven = ", and the pid could not be proven reused"
			}
			return "", &shuttleengine.SessionHeldError{
				SessionID: sessionID,
				PID:       entry.PID,
				Err:       fmt.Errorf("claudeengine: session %s is held by live process %d (%s)%s; exit that session first, then resume", sessionID, entry.PID, record.Path, unproven),
			}
		}
	}
	return warning, nil
}

// sessionHolder returns the pid of the live process a registry entry in registryDirs names as holding sessionID.
// It reads every registry as one set and applies the resume check's rule,
// so a pid whose start time differs from the entry's procStart holds nothing.
// An unreadable registry is an error.
// So is an unreadable or undecodable entry when no live holder is found.
// A holder behind such a registry or entry cannot be ruled out.
// A live holder found in another entry or registry is still returned.
func sessionHolder(sessionID string, registryDirs []string, alive func(pid int) bool, startTime func(pid int) (string, bool)) (int, bool, error) {
	var unreadable error
	for _, registryDir := range registryDirs {
		records, err := readRegistry(registryDir)
		if err != nil {
			unreadable = fmt.Errorf("claudeengine: read session registry %s: %w", registryDir, err)
			continue
		}
		for _, record := range records {
			if record.ReadErr != nil {
				unreadable = fmt.Errorf("claudeengine: read session registry entry %s: %w", record.Path, record.ReadErr)
				continue
			}
			if record.DecodeErr != nil {
				unreadable = fmt.Errorf("claudeengine: decode session registry entry %s: %w", record.Path, record.DecodeErr)
				continue
			}
			if record.Entry.SessionID != sessionID {
				continue
			}
			if held, _ := liveHolder(record.Entry, alive, startTime); held {
				return record.Entry.PID, true, nil
			}
		}
	}
	return 0, false, unreadable
}

// liveHolder reports whether entry's process is alive and may still be the one that registered.
// It also reports whether the start time proved it.
// A start time that differs from the entry's procStart proves the pid reused, which holds nothing.
// An unreadable start time, or an entry with no procStart, holds unproven, so two processes never drive one session.
func liveHolder(entry registryEntry, alive func(pid int) bool, startTime func(pid int) (string, bool)) (held, proven bool) {
	if !alive(entry.PID) {
		return false, false
	}
	live, readable := startTime(entry.PID)
	if readable && entry.ProcStart != "" {
		return live == entry.ProcStart, true
	}
	return true, false
}

// registryRecord is one session-registry file as read.
// It holds the decoded entry, or the error that stopped it being read or decoded.
type registryRecord struct {
	Path      string
	Entry     registryEntry
	ReadErr   error
	DecodeErr error
}

// readRegistry reads every .json file in registryDir, in directory order.
// A directory that does not exist is an empty registry, since a known config directory may never have run a session.
// It returns an error only when the directory exists and cannot be read.
func readRegistry(registryDir string) ([]registryRecord, error) {
	dirEntries, err := os.ReadDir(registryDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []registryRecord
	for _, e := range dirEntries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		record := registryRecord{Path: filepath.Join(registryDir, e.Name())}
		data, err := os.ReadFile(record.Path)
		if err != nil {
			record.ReadErr = err
		} else {
			record.DecodeErr = json.Unmarshal(data, &record.Entry)
		}
		records = append(records, record)
	}
	return records, nil
}
