// transcriptread.go implements shuttleengine.TranscriptReader for Claude: it locates the sessions Claude Code recorded under a worktree and reads them into neutral transcript entries.
// The project directory layout lives here; the line format is parsed in usage.go.

package claudeengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

var _ shuttleengine.TranscriptReader = (*Claude)(nil)

// projectsRoot returns the directory Claude Code keeps its per-worktree project directories in.
func projectsRoot() (string, error) {
	home, err := claudeHomeDir()
	if err != nil {
		return "", fmt.Errorf("claudeengine: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "projects"), nil
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

// LocateSessions returns the sessions Claude Code recorded under the worktree path, sorted by session ID.
// A missing projects root or project directory is no sessions and no error.
func (c *Claude) LocateSessions(worktree string) ([]shuttleengine.LocatedSession, error) {
	projects, err := projectsRoot()
	if err != nil {
		return nil, err
	}
	return locateSessions(projects, worktree)
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
		subagentsDir := filepath.Join(projectDir, strings.TrimSuffix(entry.Name(), ".jsonl"), "subagents")
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

// ReadSession reads a session this engine located: its own transcript with sidechain lines included, then each fork transcript.
// A fork skips the message ids the session's own transcript counted, since it replays its parent's spawning message.
// A transcript that cannot be read, one that vanished after locating included, is an error naming its path.
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
