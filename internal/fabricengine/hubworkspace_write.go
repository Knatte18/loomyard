package fabricengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// writeHubWorkspace writes content to the workspace file at path, directly in the hub dir, in place and reports whether it wrote.
// An unchanged file is not rewritten at all.
// A read error other than not-exist is returned rather than overwriting bytes it could not compare.
// Every read and write goes through an os.Root at the hub, so a symlink planted at the workspace path that escapes the hub is refused.
// It creates no directory and records no Mutations: the caller records the write.
func writeHubWorkspace(l *lyxcwd.Location, path string, content []byte) (bool, error) {
	root, err := os.OpenRoot(l.HubPath)
	if err != nil {
		return false, fmt.Errorf("open hub root %s: %w", l.HubPath, err)
	}
	defer root.Close()

	fileRel, err := hubRel(l.HubPath, path)
	if err != nil {
		return false, err
	}
	existing, err := root.ReadFile(fileRel)
	switch {
	case err == nil && string(existing) == string(content):
		return false, nil
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return false, fmt.Errorf("read workspace file %s: %w", fileRel, err)
	}
	if err := root.WriteFile(fileRel, content, 0o644); err != nil {
		return false, fmt.Errorf("write workspace file %s: %w", fileRel, err)
	}
	return true, nil
}

// removeStaleBeamWorkspaces removes each <hub>/<RepoName>-*.code-workspace file that carries beamWorkspaceMark and whose slug is not in live.
// A file without the mark, one that does not parse as JSON, and a directory are left alone, so an operator's own file of such a name stays.
// Every read and removal goes through an os.Root at the hub; a removal is recorded in rec.
func removeStaleBeamWorkspaces(rec *Mutations, l *lyxcwd.Location, live map[string]bool) error {
	root, err := os.OpenRoot(l.HubPath)
	if err != nil {
		return fmt.Errorf("open hub root %s: %w", l.HubPath, err)
	}
	defer root.Close()

	hub, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("open hub dir %s: %w", l.HubPath, err)
	}
	defer hub.Close()
	entries, err := hub.ReadDir(-1)
	if err != nil {
		return fmt.Errorf("list hub dir %s: %w", l.HubPath, err)
	}
	prefix := l.RepoName + "-"
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, hubWorkspaceExt) {
			continue
		}
		slug := strings.TrimSuffix(strings.TrimPrefix(name, prefix), hubWorkspaceExt)
		if slug == "" || live[slug] {
			continue
		}
		data, err := root.ReadFile(name)
		if err != nil {
			logger.Debug("fabricengine: a hub workspace file that cannot be read is left in place", "name", name, "error", err)
			continue
		}
		if !carriesBeamMark(data) {
			continue
		}
		if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove workspace file %s: %w", name, err)
		}
		rec.Append(KindPathRemoved, BeamWorkspacePath(l, slug), "single")
	}
	return nil
}

// carriesBeamMark reports whether the workspace file bytes data open with beamWorkspaceMark set to true as their first key.
// Only the file's head is parsed, since the settings spliced behind it may hold comments a JSON decoder refuses.
func carriesBeamMark(data []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return false
	}
	key, err := decoder.Token()
	if err != nil || key != beamWorkspaceMark {
		return false
	}
	var marked bool
	return decoder.Decode(&marked) == nil && marked
}
