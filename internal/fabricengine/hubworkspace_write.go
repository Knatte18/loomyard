package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// WriteHubWorkspace writes content to the prime's hub workspace file and reports whether it wrote.
// It first creates _portals/<AnchorRel>, because clone never creates it and removing the last pair prunes it,
// so the workspace file's _portals folder always resolves.
// An unchanged file is not rewritten at all, unlike writeLauncherScriptIfChanged, which rewrites unconditionally.
// A read error other than not-exist is returned rather than overwriting bytes it could not compare.
// Every write goes through an os.Root at the hub, so a symlink planted at any component that escapes the hub is refused.
// It records no Mutations: it is not a fabric verb.
func WriteHubWorkspace(l *lyxcwd.Location, primeName string, content []byte) (bool, error) {
	root, err := os.OpenRoot(l.HubPath)
	if err != nil {
		return false, fmt.Errorf("open hub root %s: %w", l.HubPath, err)
	}
	defer root.Close()

	portalsRel, err := hubRel(l.HubPath, filepath.Join(PortalsDir(l), l.AnchorRel))
	if err != nil {
		return false, err
	}
	if err := root.MkdirAll(portalsRel, 0o755); err != nil {
		return false, fmt.Errorf("create portals dir %s: %w", portalsRel, err)
	}

	fileRel, err := hubRel(l.HubPath, HubWorkspacePath(l, primeName))
	if err != nil {
		return false, err
	}
	if err := root.MkdirAll(filepath.Dir(fileRel), 0o755); err != nil {
		return false, fmt.Errorf("create workspace dir %s: %w", filepath.Dir(fileRel), err)
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
