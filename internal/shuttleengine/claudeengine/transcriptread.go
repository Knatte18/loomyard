// transcriptread.go implements shuttleengine.TranscriptReader for Claude.
// It locates the sessions Claude Code recorded under a worktree in every known config directory.
// It reads them into neutral transcript entries.
// The project directory layout lives here; the line format is parsed in usage.go.

package claudeengine

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/envconfig"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var _ shuttleengine.TranscriptReader = (*Claude)(nil)

// projectsDirName is the directory of a Claude config directory that holds one project directory per worktree.
const projectsDirName = "projects"

// subagentsDirName is the name of the directory that holds a session's subagent and fork transcripts.
// It sits inside the directory named after the session, which lies beside the session's transcript file.
const subagentsDirName = "subagents"

// subagentTranscriptPrefix starts the file name of a subagent's transcript, followed by its agent id and ".jsonl".
const subagentTranscriptPrefix = "agent-"

// knownConfigDirs returns every Claude config directory a session of this machine may have been recorded under.
// Each appears once.
// They are every listed account's directory, retired accounts included.
// They also include the env module's CLAUDE_CONFIG_DIR when set, and ~/.claude.
// An engine told no hub board directory reads the process's own CLAUDE_CONFIG_DIR in place of the first two.
// A config that fails to load is logged at Warn and the remaining directories are still returned.
func (c *Claude) knownConfigDirs() []string {
	var dirs []string
	seen := map[string]bool{}
	add := func(dir string) {
		if dir == "" {
			return
		}
		canonical := canonicalConfigDir(dir)
		if seen[canonical] {
			return
		}
		seen[canonical] = true
		dirs = append(dirs, dir)
	}

	if c.dirs.Board == "" {
		add(os.Getenv(claudeConfigDirEnv))
	} else {
		if claude, err := LoadConfig(c.dirs); err != nil {
			logger.Warn("claudeengine: the listed accounts' config directories are not read", "err", err)
		} else {
			for _, account := range claude.Accounts {
				add(account.ConfigDir)
			}
		}
		if env, err := envconfig.LoadConfig(c.dirs); err != nil {
			logger.Warn("claudeengine: the env module's Claude config directory is not read", "err", err)
		} else {
			add(env.Vars[claudeConfigDirEnv])
		}
	}
	if home, err := claudeHomeDir(); err != nil {
		logger.Warn("claudeengine: the default Claude config directory is not read", "err", err)
	} else {
		add(filepath.Join(home, claudeConfigDirName))
	}
	return dirs
}

// resolvedConfigDir returns the config directory a launch in the told worktree runs under.
// It reports false for an engine told no hub board directory and when the resolution fails, which it logs at Warn.
func (c *Claude) resolvedConfigDir() (string, bool) {
	if c.dirs.Board == "" {
		return "", false
	}
	claude, err := LoadConfig(c.dirs)
	if err != nil {
		return "", false
	}
	_, launched, err := c.resolveLaunchAccount(claude)
	if err != nil {
		logger.Warn("claudeengine: the worktree's Claude config directory is not resolved", "err", err)
		return "", false
	}
	return launched.ConfigDir, true
}

// sessionProjectDir returns the project directory of workdir under the config directory the session was launched with.
// A running session has a live entry in the sessions registry of that directory alone, so the entry names it.
// A session with no live entry is read under the worktree's resolved directory when that holds its transcript.
// Otherwise it is read under the known directory holding its newest copy.
// When none holds a transcript the directory is the worktree's resolved one.
// A read then reports the transcript missing there.
func (c *Claude) sessionProjectDir(sessionID, workdir string) (string, error) {
	known := c.knownConfigDirs()
	for _, dir := range known {
		if registryHoldsLiveEntry(filepath.Join(dir, sessionsDirName), sessionID) {
			return projectDirUnder(filepath.Join(dir, projectsDirName), workdir), nil
		}
	}

	resolved, hasResolved := c.resolvedConfigDir()
	if hasResolved {
		if path, found := newestTranscript([]string{resolved}, workdir, sessionID); found {
			return filepath.Dir(path), nil
		}
	}
	if path, found := newestTranscript(known, workdir, sessionID); found {
		return filepath.Dir(path), nil
	}
	switch {
	case hasResolved:
		return projectDirUnder(filepath.Join(resolved, projectsDirName), workdir), nil
	case len(known) > 0:
		return projectDirUnder(filepath.Join(known[0], projectsDirName), workdir), nil
	}
	return "", errors.New("claudeengine: no Claude config directory to read the session's transcript from; way forward: set HOME")
}

// newestTranscript returns the path of the most recently written transcript of sessionID for workdir under configDirs.
// It reports false when no directory holds one.
// Of two copies written at the same time the earlier directory's wins.
func newestTranscript(configDirs []string, workdir, sessionID string) (string, bool) {
	var newest string
	var newestAt time.Time
	for _, dir := range configDirs {
		path := filepath.Join(projectDirUnder(filepath.Join(dir, projectsDirName), workdir), sessionID+".jsonl")
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if newest == "" || info.ModTime().After(newestAt) {
			newest, newestAt = path, info.ModTime()
		}
	}
	return newest, newest != ""
}

// projectDirUnder returns the project directory of workdir under projects.
// The directory name is workdir with every non-alphanumeric byte replaced by '-'.
func projectDirUnder(projects, workdir string) string {
	encoded := []byte(workdir)
	for i, b := range encoded {
		isAlnum := (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
		if !isAlnum {
			encoded[i] = '-'
		}
	}
	return filepath.Join(projects, string(encoded))
}

// LocateSessions returns the sessions Claude Code recorded under the worktree path in every known config directory.
// They are sorted by session ID.
// A session id found in several directories is returned once, as its most recently written copy, which holds every turn.
// A missing projects root or project directory is no sessions and no error.
func (c *Claude) LocateSessions(worktree string) ([]shuttleengine.LocatedSession, error) {
	newest := map[string]shuttleengine.LocatedSession{}
	for _, dir := range c.knownConfigDirs() {
		found, err := locateSessions(filepath.Join(dir, projectsDirName), worktree)
		if err != nil {
			return nil, err
		}
		for _, session := range found {
			if kept, seen := newest[session.ID]; !seen || session.Modified.After(kept.Modified) {
				newest[session.ID] = session
			}
		}
	}
	return slices.SortedFunc(maps.Values(newest), func(a, b shuttleengine.LocatedSession) int {
		return strings.Compare(a.ID, b.ID)
	}), nil
}

// locateSessions lists the sessions of worktree's project directory under projects:
// every .jsonl file directly in it is a session, and the .jsonl files in its <name>/subagents/ directory are its forks.
// os.ReadDir sorts by file name, which is the ID, so sessions and forks come back sorted.
func locateSessions(projects, worktree string) ([]shuttleengine.LocatedSession, error) {
	projectDir := projectDirUnder(projects, worktree)
	entries, err := os.ReadDir(projectDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claudeengine: read project directory %s: %w", projectDir, err)
	}
	var sessions []shuttleengine.LocatedSession
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("claudeengine: stat session %s: %w", filepath.Join(projectDir, entry.Name()), err)
		}
		subagentsDir := filepath.Join(projectDir, strings.TrimSuffix(entry.Name(), ".jsonl"), subagentsDirName)
		forks, err := locateForks(subagentsDir)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, shuttleengine.LocatedSession{
			TranscriptRef: shuttleengine.TranscriptRef{ID: entry.Name(), Ref: filepath.Join(projectDir, entry.Name())},
			Modified:      info.ModTime(),
			Forks:         forks,
		})
	}
	return sessions, nil
}

// locateForks lists the .jsonl files in subagentsDir as fork transcripts; a missing directory is no forks.
func locateForks(subagentsDir string) ([]shuttleengine.TranscriptRef, error) {
	entries, err := os.ReadDir(subagentsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claudeengine: read subagents directory %s: %w", subagentsDir, err)
	}
	var forks []shuttleengine.TranscriptRef
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		forks = append(forks, shuttleengine.TranscriptRef{ID: entry.Name(), Ref: filepath.Join(subagentsDir, entry.Name())})
	}
	return forks, nil
}

// ReadSession reads a session this engine located.
// It reads the session's own transcript with sidechain lines included, then each fork transcript.
// A fork skips the message ids the session's own transcript counted, since it replays its parent's spawning message.
// A transcript that cannot be read is an error naming its path, including one that vanished after it was located.
func (c *Claude) ReadSession(s shuttleengine.LocatedSession) (shuttleengine.SessionRead, error) {
	main, err := readTranscript(s.TranscriptRef, s.ID, nil)
	if err != nil {
		return shuttleengine.SessionRead{}, err
	}
	read := shuttleengine.SessionRead{Main: main.read}
	for _, fork := range s.Forks {
		forkRead, err := readTranscript(fork, s.ID+"/"+fork.ID, main.countedIDs)
		if err != nil {
			return shuttleengine.SessionRead{}, err
		}
		read.Forks = append(read.Forks, forkRead.read)
	}
	return read, nil
}

// readTranscriptResult is a transcript read with the message ids it counted.
type readTranscriptResult struct {
	read       shuttleengine.TranscriptRead
	countedIDs map[string]struct{}
}

// readTranscript parses the transcript ref names, sidechain lines included, into its neutral read.
func readTranscript(ref shuttleengine.TranscriptRef, transcriptKey string, skipIDs map[string]struct{}) (readTranscriptResult, error) {
	parsed, err := parseTranscript(ref.Ref, transcriptKey, true, skipIDs)
	if err != nil {
		return readTranscriptResult{}, fmt.Errorf("claudeengine: read transcript %s: %w", ref.Ref, err)
	}
	return readTranscriptResult{
		read: shuttleengine.TranscriptRead{
			ID:      ref.ID,
			Name:    parsed.name,
			Started: parsed.started,
			Repeats: parsed.repeats,
			Entries: parsed.entries,
		},
		countedIDs: parsed.countedIDs,
	}, nil
}
