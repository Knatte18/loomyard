// warpexclude.go seeds the warp exclude list into the repository's `info/exclude`.
// The list holds the paths under a warp worktree's anchor that fabric keeps out of git in every warp worktree.
// It is the `warp_exclude` key of fabric's config, separate from the junction name-set and never wired as a junction.

package fabricengine

import (
	"bytes"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// seedWarpExcludeLines appends one `/<anchorRel>/<path>` line per path to `info/exclude` of the repository at worktree.
// When anchorRel is ".", the line is `/<path>`.
// The lines are anchored at the repository root, since a slash-free pattern matches at every depth,
// and they name the anchor subpath because the listed files sit at a worktree's anchor.
// A line already present is skipped, and no line is ever removed:
// `info/exclude` is one file for every worktree of the repository.
// rec records KindFileWritten on the exclude file only when it changed.
func seedWarpExcludeLines(rec *Mutations, worktree, anchorRel string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	anchorPrefix := filepath.ToSlash(anchorRel)
	excludePath, changed, err := mutateGitExclude(worktree, func(content string) (string, error) {
		present := make(map[string]bool)
		for _, line := range strings.Split(content, "\n") {
			present[strings.TrimSpace(line)] = true
		}
		for _, listed := range paths {
			line := "/" + path.Join(anchorPrefix, listed)
			if present[line] {
				continue
			}
			if content != "" && !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += line + "\n"
			present[line] = true
		}
		return content, nil
	})
	if err != nil {
		return err
	}
	if changed {
		rec.Append(KindFileWritten, excludePath, "")
	}
	return nil
}

// SeedWarpExclude loads fabric's config and seeds its warp exclude list under l's anchor.
// The list goes into the exclude file of the repository that owns l's warp worktree.
// It returns the listed paths, relative to the anchor, and records no mutation: it is no fabric verb.
// Nothing on the list is written in the weft, so the weft is never seeded.
func SeedWarpExclude(l *lymcwd.Location) ([]string, error) {
	cfg, err := LoadConfig(ConfigDirs(l))
	if err != nil {
		return nil, err
	}
	paths := cfg.WarpExcludePaths()
	if err := seedWarpExcludeLines(nil, l.WorktreePath(), l.AnchorRel, paths); err != nil {
		return nil, err
	}
	return paths, nil
}

// AppendExcludePaths returns a hub config reinterpretation's rewrite that adds paths to a hub's own warp exclude list.
// A fabric.yaml that sets warp_exclude gains each path its list lacks, appended in order, and one line naming them.
// A file that leaves the key unset takes the template's list, so it comes back unchanged, as does one listing every path.
// The rewrite ignores the worktree root and anchored directory it is handed.
// A document that does not parse returns an error marked configengine.ErrInvalid.
func AppendExcludePaths(paths ...string) func(existing []byte, worktreeRoot, anchorDir string) (rewritten []byte, rewrote, flagged []string, err error) {
	return func(existing []byte, _, _ string) ([]byte, []string, []string, error) {
		var document yaml.Node
		if err := yaml.Unmarshal(existing, &document); err != nil {
			return nil, nil, nil, configengine.MarkInvalid(fmt.Errorf("extend %s: unmarshal yaml: %w", ExcludeListKey, err))
		}
		if document.Kind != yaml.DocumentNode || len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
			return existing, nil, nil, nil
		}
		mapping := document.Content[0]
		var value *yaml.Node
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == ExcludeListKey && mapping.Content[i+1].Kind == yaml.ScalarNode {
				value = mapping.Content[i+1]
			}
		}
		if value == nil {
			return existing, nil, nil, nil
		}
		listed := strings.Fields(value.Value)
		var added []string
		for _, missing := range paths {
			if !slices.Contains(listed, missing) {
				listed = append(listed, missing)
				added = append(added, missing)
			}
		}
		if len(added) == 0 {
			return existing, nil, nil, nil
		}
		value.Value = strings.Join(listed, " ")
		value.Tag = "!!str"

		var out bytes.Buffer
		encoder := yaml.NewEncoder(&out)
		encoder.SetIndent(2)
		if err := encoder.Encode(&document); err != nil {
			return nil, nil, nil, configengine.MarkInvalid(fmt.Errorf("extend %s: encode yaml: %w", ExcludeListKey, err))
		}
		if err := encoder.Close(); err != nil {
			return nil, nil, nil, configengine.MarkInvalid(fmt.Errorf("extend %s: encode yaml: %w", ExcludeListKey, err))
		}
		line := fmt.Sprintf("%s gained %s, a file lym now renders into every worktree; lym config fabric --set %s=<paths> from the prime sets another list", ExcludeListKey, strings.Join(added, " "), ExcludeListKey)
		return out.Bytes(), []string{line}, nil, nil
	}
}

// ExcludeWayForward returns the refusal text for a path the warp exclude list does not name:
// the path cannot be written because git would show it as untracked,
// and it joins the list through the `warp_exclude` key in `lym config fabric`.
func ExcludeWayForward(path string) string {
	return fmt.Sprintf("%s is not on fabric's warp exclude list, so it would show as untracked; way forward: add it to the warp_exclude key in \"lym config fabric\" and run again", path)
}
