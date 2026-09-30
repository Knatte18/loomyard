package loomcli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// statusSidecarFile is the ephemeral sidecar's filename, joined onto loomengine.LoomScratchDir so the
// file lives under .lyx and never in tracked content.
const statusSidecarFile = "status-strand.json"

// buildIdentity names one lyx build by the running executable's symlink-resolved path, size and
// modification time. A rebuilt binary at the same path changes size or mtime, which is exactly the
// signal a live status strand launched from the older build needs to be replaced.
type buildIdentity struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time_unix_nano"`
}

// statusSidecar records which strand the bootstrap last added or replaced as the status strand and
// the build identity of the lyx that launched it.
type statusSidecar struct {
	GUID  string        `json:"guid"`
	Build buildIdentity `json:"build"`
}

// identityOf computes the build identity of the executable at path.
func identityOf(path string) (buildIdentity, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return buildIdentity{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return buildIdentity{}, err
	}
	return buildIdentity{Path: resolved, Size: info.Size(), ModTime: info.ModTime().UnixNano()}, nil
}

// currentBuildIdentity is the build identity of the running executable.
func currentBuildIdentity() (buildIdentity, error) {
	exe, err := os.Executable()
	if err != nil {
		return buildIdentity{}, err
	}
	return identityOf(exe)
}

// statusSidecarPath is the sidecar's path under the worktree's loom scratch directory.
func statusSidecarPath(l *lyxcwd.Location) string {
	return filepath.Join(loomengine.LoomScratchDir(l), statusSidecarFile)
}

// readStatusSidecar reads the sidecar at path. A missing or unreadable file yields nil, which
// resolveStatusStrandAction treats as "not proven current".
func readStatusSidecar(path string) *statusSidecar {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var sc statusSidecar
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil
	}
	return &sc
}

// writeStatusSidecar records guid beside build at path, creating the scratch directory if needed.
func writeStatusSidecar(path, guid string, build buildIdentity) error {
	if guid == "" {
		return errors.New("loom: status sidecar needs a strand guid")
	}
	data, err := json.Marshal(statusSidecar{GUID: guid, Build: build})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
