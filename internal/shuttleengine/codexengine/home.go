// home.go prepares the shared lym Codex home a launch runs under and reads the checkout the launch starts in.
// The home holds links to the operator home's login and sessions, lym's record of the roots it trusts, and the config.toml rendered whole from that record.
// The checkout gives the roots Codex keys folder trust on and the `.codex` folders in it, tracked by git or not, which a launch warns about or refuses.

package codexengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/fslink"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/state"
)

// Names of the entries in a Codex home and a repository's Codex folder.
const (
	authFileName        = "auth.json"
	sessionsDirName     = "sessions"
	configFileName      = "config.toml"
	trustRecordFileName = "lym-trust.json"
	homeLockFileName    = "lym-home.lock"
	codexFolderName     = ".codex"
	rulesDirName        = "rules"
)

// trustRecord is lym's record of the roots its Codex home trusts, from which config.toml is rendered.
type trustRecord struct {
	// Roots are the absolute, symlink-resolved folders the home trusts, sorted.
	Roots []string `json:"roots"`
}

// checkout is the git checkout a launch starts in.
type checkout struct {
	// root is the worktree root, symlinks resolved as Codex resolves them.
	root string
	// mainRoot is the repository's main checkout root for a linked worktree, symlinks resolved; empty for a plain clone.
	mainRoot string
}

// trustRoots returns the roots Codex keys folder trust on for a session in c: the worktree root, then the main checkout root of a linked worktree.
func (c checkout) trustRoots() []string {
	if c.mainRoot == "" {
		return []string{c.root}
	}
	return []string{c.root, c.mainRoot}
}

// readCheckout reads the checkout the directory anchor sits in from its `.git` entries, without running git.
// A linked worktree's main checkout root is the directory holding its common git dir.
func readCheckout(anchor string) (checkout, error) {
	root, geometry, err := dotgit.FindRoot(anchor)
	if err != nil {
		return checkout{}, fmt.Errorf("codexengine: Codex keys folder trust on a git repository root, and %s is in none: %w", anchor, err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return checkout{}, fmt.Errorf("codexengine: resolve the worktree root %s: %w", root, err)
	}
	found := checkout{root: resolvedRoot}
	if geometry.GitDir != geometry.CommonDir && filepath.Base(geometry.CommonDir) == ".git" {
		mainRoot, err := filepath.EvalSymlinks(filepath.Dir(geometry.CommonDir))
		if err != nil {
			return checkout{}, fmt.Errorf("codexengine: resolve the main checkout root of %s: %w", root, err)
		}
		if mainRoot != resolvedRoot {
			found.mainRoot = mainRoot
		}
	}
	return found, nil
}

// checkCodexFolders warns about the Codex settings a launch in found and anchor runs, and refuses a launch under Codex rules.
// It reads the folders on disk, whether git tracks them or not, since Codex reads them either way.
// A `.codex/config.toml` in the worktree is named in a Warn.
// For a linked worktree whose root holds a `.codex` folder, the main checkout's `.codex/config.toml` is named too, since Codex then takes its hooks from that file.
// A `.codex/rules` directory in the worktree root or the anchor refuses the launch, since an allow rule there lifts the sandbox for its command, and the refusal is also logged.
func (c *Codex) checkCodexFolders(found checkout, anchor string) error {
	folders := []string{filepath.Join(found.root, codexFolderName)}
	if resolvedAnchor, err := filepath.EvalSymlinks(anchor); err == nil && resolvedAnchor != found.root {
		folders = append(folders, filepath.Join(resolvedAnchor, codexFolderName))
	}
	for _, folder := range folders {
		if rules := filepath.Join(folder, rulesDirName); isDir(rules) {
			refusal := fmt.Errorf("codexengine: %s holds Codex rules, and an allow rule there lifts the sandbox for its command, whether git tracks the directory or not; way forward: remove the directory from the worktree, in a commit when git tracks it, then launch again, or launch on an engine other than codex", rules)
			c.warn("codexengine: refused a Codex launch under Codex rules in the worktree", "path", rules)
			return refusal
		}
		if config := filepath.Join(folder, configFileName); isFile(config) {
			c.warn("codexengine: the worktree holds a Codex config, whose hooks and settings the session runs without review", "path", config)
		}
	}
	if found.mainRoot != "" && isDir(filepath.Join(found.root, codexFolderName)) {
		if config := filepath.Join(found.mainRoot, codexFolderName, configFileName); isFile(config) {
			c.warn("codexengine: the main checkout holds a Codex config, whose hooks Codex runs in place of the linked worktree's", "path", config)
		}
	}
	return nil
}

// prepareHome readies the lym Codex home for a launch that trusts roots.
// It first links the home's auth.json and sessions to the operator home's, so a platform that cannot link a file refuses before anything is written.
// Then, under the home's file lock, it adds roots to the trust record, drops recorded roots whose folder no longer exists, and renders config.toml from the result before the record is written.
func (c *Codex) prepareHome(homes homeDirs, roots []string) error {
	if err := c.ensureLink(filepath.Join(homes.lym, authFileName), filepath.Join(homes.operator, authFileName), fslink.CreateFileLink); err != nil {
		return err
	}
	if err := c.ensureLink(filepath.Join(homes.lym, sessionsDirName), filepath.Join(homes.operator, sessionsDirName), fslink.CreateDirLink); err != nil {
		return err
	}
	recordPath := filepath.Join(homes.lym, trustRecordFileName)
	err := state.UpdateJSON(recordPath, filepath.Join(homes.lym, homeLockFileName), func(current trustRecord, _ bool) (trustRecord, error) {
		next := trustRecord{Roots: mergeRoots(current.Roots, roots)}
		if err := fsx.AtomicWriteBytes(filepath.Join(homes.lym, configFileName), []byte(renderConfigTOML(next.Roots))); err != nil {
			return trustRecord{}, fmt.Errorf("write %s: %w", configFileName, err)
		}
		return next, nil
	})
	if err != nil {
		return fmt.Errorf("codexengine: prepare the lym Codex home %s: %w", homes.lym, err)
	}
	return nil
}

// ensureLink makes link a link to target with create when nothing sits at link.
// A link that already points at target is kept.
// A regular file, or a link elsewhere, is kept and named in a Warn with the way forward, since it may hold a login made in the lym home.
// A create that fails because a concurrent launch made the same link meanwhile counts as made.
func (c *Codex) ensureLink(link, target string, create func(link, target string) error) error {
	info, err := os.Lstat(link)
	switch {
	case err == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			if raw, rawErr := fslink.RawTarget(link); rawErr == nil && raw == target {
				return nil
			}
		}
		c.warn("codexengine: the lym Codex home's entry is not a link to the operator home's; way forward: remove it, and the next Codex launch links it again", "path", link, "want", target)
		return nil
	case !errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("codexengine: read %s: %w", link, err)
	}
	if err := create(link, target); err != nil {
		if raw, rawErr := fslink.RawTarget(link); rawErr == nil && raw == target {
			return nil
		}
		return fmt.Errorf("codexengine: link %s to %s: %w", link, target, err)
	}
	return nil
}

// mergeRoots returns the union of recorded and added, sorted, without the recorded roots whose folder no longer exists.
func mergeRoots(recorded, added []string) []string {
	var merged []string
	for _, root := range recorded {
		if isDir(root) && !slices.Contains(merged, root) {
			merged = append(merged, root)
		}
	}
	for _, root := range added {
		if !slices.Contains(merged, root) {
			merged = append(merged, root)
		}
	}
	slices.Sort(merged)
	return merged
}

// configHeader opens the rendered config.toml, telling a reader whose file it is.
const configHeader = "# Rendered by lym from " + trustRecordFileName + " at every Codex launch; an edit here is lost at the next launch.\n" +
	"# Put your own Codex settings in the operator home's config.toml.\n"

// renderConfigTOML renders the lym home's config.toml: the `[tui]` settings Codex otherwise writes on its first start, then one trusted project table per root.
func renderConfigTOML(roots []string) string {
	var b strings.Builder
	b.WriteString(configHeader)
	b.WriteString("\n[tui]\nscreen_reader_detection_done = true\n\n[tui.model_availability_nux]\n")
	for _, root := range roots {
		b.WriteString("\n" + projectTableHeader(root) + "\ntrust_level = \"trusted\"\n")
	}
	return b.String()
}

// projectTableHeader returns the config.toml table header of root's project settings.
func projectTableHeader(root string) string {
	return "[projects." + tomlString(root) + "]"
}

// CheckAfterStartup reads the lym home's config.toml back once a launch is past its startup gates and logs at Warn each trust root it no longer holds, since Codex writes the file itself and ignores lym's lock.
// It checks the roots of the checkout this engine's anchor sits in, and reads at Warn whatever stops it from checking.
func (c *Codex) CheckAfterStartup() {
	codex, err := loadConfigOrRefuse(c.dirs)
	if err != nil {
		c.warn("codexengine: could not check the lym Codex home's trust roots after startup", "error", err)
		return
	}
	homes, err := resolveHomes(codex)
	if err != nil {
		c.warn("codexengine: could not check the lym Codex home's trust roots after startup", "error", err)
		return
	}
	found, err := readCheckout(c.dirs.Anchor)
	if err != nil {
		c.warn("codexengine: could not check the lym Codex home's trust roots after startup", "error", err)
		return
	}
	configPath := filepath.Join(homes.lym, configFileName)
	content, err := os.ReadFile(configPath)
	if err != nil {
		c.warn("codexengine: could not read the lym Codex home's config after startup", "path", configPath, "error", err)
		return
	}
	for _, root := range found.trustRoots() {
		if !strings.Contains(string(content), projectTableHeader(root)) {
			c.warn("codexengine: the lym Codex home's config lost a trusted root after startup, so a later launch there shows Codex's trust prompt, which the startup classifier accepts", "path", configPath, "root", root)
		}
	}
}

// isDir reports whether path is a directory, following links.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isFile reports whether path is a regular file, following links.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
