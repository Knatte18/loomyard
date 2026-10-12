// build.go identifies the running lyx build and records which strand a bootstrap launched from it, so a bootstrap keeps a live status strand of the current build and replaces one of an older build.

package statuspane

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// BuildIdentity names one lyx build by the running executable's symlink-resolved path, size and modification time.
// A rebuilt binary at the same path changes size or mtime, which is the signal a live status strand launched from the older build needs to be replaced.
type BuildIdentity struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time_unix_nano"`
}

// Sidecar records which strand a bootstrap last added or replaced as a status strand and the build identity of the lyx that launched it.
type Sidecar struct {
	GUID  string        `json:"guid"`
	Build BuildIdentity `json:"build"`
}

// IdentityOf computes the build identity of the executable at path.
func IdentityOf(path string) (BuildIdentity, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return BuildIdentity{}, err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return BuildIdentity{}, err
	}
	return BuildIdentity{Path: resolved, Size: info.Size(), ModTime: info.ModTime().UnixNano()}, nil
}

// CurrentBuild is the build identity of the running executable.
func CurrentBuild() (BuildIdentity, error) {
	exe, err := os.Executable()
	if err != nil {
		return BuildIdentity{}, err
	}
	return IdentityOf(exe)
}

// ReadSidecar reads the sidecar at path.
// A missing or unreadable file yields nil, which StrandCurrent treats as not proven current.
func ReadSidecar(path string) *Sidecar {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var sidecar Sidecar
	if err := json.Unmarshal(data, &sidecar); err != nil {
		return nil
	}
	return &sidecar
}

// WriteSidecar records guid beside build at path, creating the parent directory if needed.
// It refuses an empty guid.
func WriteSidecar(path, guid string, build BuildIdentity) error {
	if guid == "" {
		return errors.New("statuspane: a sidecar needs a strand guid")
	}
	data, err := json.Marshal(Sidecar{GUID: guid, Build: build})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// StrandCurrent reports whether a status strand may be kept: it is live, and the sidecar names its guid and records build.
// A missing or unreadable sidecar means the strand may run a different lyx build, so it is not current.
func StrandCurrent(live bool, guid string, sidecar *Sidecar, build BuildIdentity) bool {
	return live && sidecar != nil && sidecar.GUID == guid && sidecar.Build == build
}
