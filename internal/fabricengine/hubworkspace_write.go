package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// writeHubWorkspace writes content to the hub workspace file in place and reports whether it wrote.
// An unchanged file is not rewritten at all.
// A read error other than not-exist is returned rather than overwriting bytes it could not compare.
// Every read and write goes through an os.Root at the hub, so a symlink planted at the workspace path that escapes the hub is refused.
// It creates no directory and records no Mutations: the caller records the write.
func writeHubWorkspace(l *lyxcwd.Location, content []byte) (bool, error) {
	root, err := os.OpenRoot(l.HubPath)
	if err != nil {
		return false, fmt.Errorf("open hub root %s: %w", l.HubPath, err)
	}
	defer root.Close()

	fileRel, err := hubRel(l.HubPath, HubWorkspacePath(l))
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
