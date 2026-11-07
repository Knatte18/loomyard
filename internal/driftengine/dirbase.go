// dirbase.go rebases the relative capture and compare dirs of a hub's drift.yaml.
// It moves their base from the anchored directory onto the worktree root.

package driftengine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
)

// DirBaseReinterpretation names the move of a relative capture or compare dir's base from the worktree's anchored directory to its root.
const DirBaseReinterpretation = "drift-dir-root-base"

// RebaseDirsOntoRoot rebases the relative capture and compare dirs of a hub drift.yaml.
// It rewrites each one so that, joined onto the worktree root, it names the directory it named before.
// worktreeRoot and anchorDir are the prime's, and a hub anchored at its root changes nothing.
// A dir naming an existing directory under the anchored directory is rewritten, even when the root holds one too.
// Such a file was written with the anchored directory as its base.
// A dir naming one under the root alone is kept, since it is already written for the root.
// A dir naming neither is flagged with the write that fixes it, and an absolute or unset dir is kept.
// It returns the document, rewritten only when a dir changed, one line per rewritten dir and one per flagged dir.
// A document that does not parse returns an error marked configengine.ErrInvalid.
func RebaseDirsOntoRoot(existing []byte, worktreeRoot, anchorDir string) (rewritten []byte, rewrote, flagged []string, err error) {
	if filepath.Clean(worktreeRoot) == filepath.Clean(anchorDir) {
		return existing, nil, nil, nil
	}
	var document yaml.Node
	if err := yaml.Unmarshal(existing, &document); err != nil {
		return nil, nil, nil, configengine.MarkInvalid(fmt.Errorf("rebase drift dirs: unmarshal yaml: %w", err))
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return existing, nil, nil, nil
	}
	for _, step := range []string{stepCapture, stepCompare} {
		dir := scalarAt(document.Content[0], step, "dir")
		if dir == nil || dir.Tag == "!!null" || dir.Value == "" || filepath.IsAbs(dir.Value) {
			continue
		}
		key := step + ".dir"
		underAnchor := filepath.Join(anchorDir, dir.Value)
		switch {
		case isDir(underAnchor):
			moved, err := filepath.Rel(worktreeRoot, underAnchor)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("rebase drift dirs: %s: %w", key, err)
			}
			moved = filepath.ToSlash(moved)
			rewrote = append(rewrote, fmt.Sprintf("%s %q rewritten to %q, since a relative dir now joins the worktree root, not the anchored directory; lym config drift --set %s=<dir> from the prime sets another", key, dir.Value, moved, key))
			dir.Value = moved
		case isDir(filepath.Join(worktreeRoot, dir.Value)):
		default:
			flagged = append(flagged, fmt.Sprintf("%s %q names no directory under the worktree root %s or the anchored directory %s; way forward: lym config drift --set %s=<dir> from the prime, with <dir> relative to the worktree root", key, dir.Value, worktreeRoot, anchorDir, key))
		}
	}
	if len(rewrote) == 0 {
		return existing, nil, flagged, nil
	}

	encoded, err := encodeDocument(&document, "rebase drift dirs")
	if err != nil {
		return nil, nil, nil, err
	}
	return encoded, rewrote, flagged, nil
}

// encodeDocument renders a rewritten drift.yaml document.
// An encoder failure is returned as an error marked configengine.ErrInvalid, its message opening with action.
func encodeDocument(document *yaml.Node, action string) ([]byte, error) {
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, configengine.MarkInvalid(fmt.Errorf("%s: encode yaml: %w", action, err))
	}
	if err := encoder.Close(); err != nil {
		return nil, configengine.MarkInvalid(fmt.Errorf("%s: encode yaml: %w", action, err))
	}
	return out.Bytes(), nil
}

// scalarAt returns the scalar node at the key path under mapping.
// It returns nil when a key is absent, when a node on the path is no mapping, or when the leaf is no scalar.
func scalarAt(mapping *yaml.Node, keys ...string) *yaml.Node {
	node := mapping
	for _, key := range keys {
		if node.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				next = node.Content[i+1]
				break
			}
		}
		if next == nil {
			return nil
		}
		node = next
	}
	if node.Kind != yaml.ScalarNode {
		return nil
	}
	return node
}

// isDir reports whether path names an existing directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
