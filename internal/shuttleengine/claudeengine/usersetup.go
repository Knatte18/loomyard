// usersetup.go resolves what a Claude session in a worktree loads from the user's Claude setup:
// the CLAUDE.md files, and the listing of the skills the session's scope turns on.
// It hands that out through shuttleengine's user-setup seam.

package claudeengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/claudeimports"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

const (
	// instructionFileName is the instruction file Claude Code reads in the config directory.
	// It reads the same file in each directory above the working directory.
	instructionFileName = "CLAUDE.md"
	// skillsDirName is the directory of a skill root that holds one directory per skill.
	skillsDirName = "skills"
	// skillFileName is the file that defines a skill.
	skillFileName = "SKILL.md"
	// projectClaudeDirName is the directory of a repository that holds its Claude Code settings and skills.
	projectClaudeDirName = ".claude"
	// pluginScopeUser and pluginScopeProject are the install scopes installed_plugins.json records.
	pluginScopeUser    = "user"
	pluginScopeProject = "project"
)

// skillListingRule opens a non-empty skill listing.
// Skills stay typed on the launch spec, so the rule asks for no load, only how a listed skill is used.
const skillListingRule = "A skill is used by reading its SKILL.md in full before following it. The skills of this session:"

// UserSetupSource implements shuttleengine.UserSetupSource over the user's Claude Code setup.
// It reads claude.yaml through the dirs fixed at construction, and is safe to share across concurrent sessions.
type UserSetupSource struct {
	// dirs locate the layered claude.yaml, whose session_scope decides which plugins and skills a session lists.
	dirs configengine.Dirs
	// claudeDir is Claude Code's config directory; empty resolves it from the environment for each session.
	claudeDir string
}

var _ shuttleengine.UserSetupSource = (*UserSetupSource)(nil)

// NewUserSetupSource returns a source that reads claude.yaml through dirs.
func NewUserSetupSource(dirs configengine.Dirs) *UserSetupSource {
	return &UserSetupSource{dirs: dirs}
}

// UserSetup resolves what a Claude session in req.WorkDir loads from the user's Claude setup.
// InstructionFiles holds the user's global CLAUDE.md, then each CLAUDE.md in a directory above req.RepoRoot.
// The files above the root come outermost first, each with its `@` imports expanded.
// SkillListing lists the skills the session's scope turns on.
// Those are the scope's plugins' skills, the user's skills and the project's.
// PluginContexts holds the context of each `SessionStart` command hook of a plugin the scope turns on.
// The hooks run through req.RunHook.
// A hook that errors, times out or is not run because req.RunHook is nil is listed in Omitted.
// So is a hooks file that cannot be read.
// A config directory that cannot be resolved leaves out the parts that need it and adds a warning naming the cause.
// An unreadable plugins file, plugin or SKILL.md is left out with a warning.
// So is an unreadable CLAUDE.md above the repository root, since that walk reaches directories the user may not own.
// The one error is an unreadable global file, and a claude.yaml that does not load.
func (s *UserSetupSource) UserSetup(req shuttleengine.UserSetupRequest) (shuttleengine.UserSetup, error) {
	claude, err := loadConfigOrRefuse(s.dirs)
	if err != nil {
		return shuttleengine.UserSetup{}, err
	}
	var setup shuttleengine.UserSetup
	configDir, err := s.resolveConfigDir()
	if err != nil {
		setup.Warnings = append(setup.Warnings, fmt.Sprintf(
			"claudeengine: the Claude config directory cannot be resolved, so the user's global file, user skills and plugin skills are left out: %v; way forward: set %s or HOME",
			err, claudeConfigDirEnv))
		configDir = ""
	}

	files, warnings, err := instructionFiles(configDir, req.RepoRoot)
	if err != nil {
		return shuttleengine.UserSetup{}, err
	}
	setup.InstructionFiles = files
	setup.Warnings = append(setup.Warnings, warnings...)

	var plugins map[string]string
	if configDir != "" {
		plugins, warnings = installedPluginRoots(configDir, req.RepoRoot)
		setup.Warnings = append(setup.Warnings, warnings...)
	}
	ids := make([]string, 0, len(plugins))
	for id := range plugins {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	scope := resolveSessionScope(claude.SessionScope, ids, req.Skills, agentname.MatchesRole(req.Role, agentname.RoleOrch))
	setup.SkillListing, setup.ListedSkills, warnings = skillListing(configDir, req.RepoRoot, req.WorkDir, plugins, scope)
	setup.Warnings = append(setup.Warnings, warnings...)
	setup.PluginContexts, setup.Omitted = pluginSessionStartContexts(req, ids, plugins, scope)
	return setup, nil
}

// resolveConfigDir returns the source's Claude Code config directory, resolving it from the environment when none is set.
func (s *UserSetupSource) resolveConfigDir() (string, error) {
	if s.claudeDir != "" {
		return s.claudeDir, nil
	}
	return claudeConfigDir()
}

// instructionFiles returns the CLAUDE.md files a Claude session loads that Codex does not read itself:
// the global file in configDir, then each CLAUDE.md in a directory above repoRoot, outermost first.
// An empty configDir leaves out the global file.
// Each file is expanded through claudeimports.Expand, and its name is its absolute path.
// An absent file adds nothing.
// An unreadable global file is the error; an unreadable file above repoRoot is left out with a warning.
func instructionFiles(configDir, repoRoot string) (items []shuttleengine.InstructionItem, warnings []string, err error) {
	if configDir != "" {
		item, found, err := readInstructionFile(filepath.Join(configDir, instructionFileName))
		if err != nil {
			return nil, nil, fmt.Errorf("claudeengine: read the user's global instruction file %s: %w; way forward: fix its permissions or remove it, then launch again", item.Name, err)
		}
		if found {
			items = append(items, item)
		}
	}
	var above []string
	for dir := filepath.Dir(filepath.Clean(repoRoot)); ; {
		above = append(above, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	slices.Reverse(above)
	for _, dir := range above {
		item, found, err := readInstructionFile(filepath.Join(dir, instructionFileName))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("claudeengine: the instruction file %s is unreadable and left out: %v; way forward: fix its permissions or remove it", item.Name, err))
			continue
		}
		if found {
			items = append(items, item)
		}
	}
	return items, warnings, nil
}

// readInstructionFile reads the instruction file at path with its imports expanded.
// The item always carries the file's absolute name; found is false for an absent file.
func readInstructionFile(path string) (item shuttleengine.InstructionItem, found bool, err error) {
	if absolute, absErr := filepath.Abs(path); absErr == nil {
		path = absolute
	}
	item.Name = path
	expanded, err := claudeimports.Expand(path)
	if errors.Is(err, fs.ErrNotExist) {
		return item, false, nil
	}
	if err != nil {
		return item, false, err
	}
	item.Text = expanded.Text
	item.WayForward = "shorten " + path
	return item, true, nil
}

// installEntry is one install of a plugin in installed_plugins.json.
type installEntry struct {
	Scope       string `json:"scope"`
	InstallPath string `json:"installPath"`
	ProjectPath string `json:"projectPath"`
}

// installedPluginRoots returns, for each plugin id configDir names as installed, the directory it is installed in.
// A plugin counts for the repository when it has a user-scope install.
// It also counts with a project-scope install whose project path is repoRoot.
// An absent plugins file names none.
// An unreadable or malformed plugins file names none, and a plugin with no install path is left out.
// Each of those two cases adds a warning naming the file.
func installedPluginRoots(configDir, repoRoot string) (map[string]string, []string) {
	path := filepath.Join(configDir, installedPluginsPath)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []string{fmt.Sprintf("claudeengine: the plugins file %s is unreadable, so no plugin skill is listed: %v; way forward: fix its permissions", path, err)}
	}
	var file struct {
		Plugins map[string][]installEntry `json:"plugins"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, []string{fmt.Sprintf("claudeengine: the plugins file %s is malformed, so no plugin skill is listed: %v; way forward: repair or remove it", path, err)}
	}
	roots := map[string]string{}
	var warnings []string
	for id, installs := range file.Plugins {
		for _, install := range installs {
			if install.Scope != pluginScopeUser && (install.Scope != pluginScopeProject || install.ProjectPath != repoRoot) {
				continue
			}
			if install.InstallPath == "" {
				warnings = append(warnings, fmt.Sprintf("claudeengine: plugin %s has no installPath in %s, so its skills are left out; way forward: reinstall the plugin", id, path))
				break
			}
			roots[id] = install.InstallPath
			break
		}
	}
	slices.Sort(warnings)
	return roots, warnings
}

// listedSkill is one line of the skill listing.
type listedSkill struct {
	name        string
	description string
	path        string
}

// skillListing returns the rule for using a skill and one line per skill the session's scope turns on.
// A line is `- <name>: <description> (<absolute SKILL.md path>)`, and the lines are sorted by name.
// It returns "" when no skill is listed.
// It also returns the listed skills' names in listing order.
// The skills are those under `skills/*/SKILL.md` of each plugin the scope turns on, named `<plugin>:<skill>`.
// They also come from configDir (the user's), and from `.claude` in workDir and each directory above it up to repoRoot.
// A scope with no plugin entries counts every plugin in plugins as on.
// A user or project skill the scope's overrides turn off is left out.
// An empty configDir leaves out the user's skills.
// An unreadable SKILL.md is left out with a warning naming it.
func skillListing(configDir, repoRoot, workDir string, plugins map[string]string, scope sessionScope) (string, []string, []string) {
	var skills []listedSkill
	var warnings []string
	scan := func(skillsDir, namePrefix string, honorOverrides bool) {
		entries, err := os.ReadDir(skillsDir)
		if errors.Is(err, fs.ErrNotExist) {
			return
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("claudeengine: the skills directory %s is unreadable and left out: %v; way forward: fix its permissions", skillsDir, err))
			return
		}
		for _, entry := range entries {
			path := filepath.Join(skillsDir, entry.Name(), skillFileName)
			data, err := os.ReadFile(path)
			if errors.Is(err, fs.ErrNotExist) || err != nil && !isDirectory(filepath.Dir(path)) {
				continue
			}
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("claudeengine: the skill file %s is unreadable and left out: %v; way forward: fix its permissions", path, err))
				continue
			}
			name := namePrefix + entry.Name()
			if honorOverrides && scope.SkillOverrides[name] == skillOff {
				continue
			}
			skills = append(skills, listedSkill{name: name, description: skillDescription(data), path: path})
		}
	}

	pluginIDs := make([]string, 0, len(plugins))
	for id := range plugins {
		pluginIDs = append(pluginIDs, id)
	}
	slices.Sort(pluginIDs)
	for _, id := range pluginIDs {
		if on, listed := scope.EnabledPlugins[id]; listed && !on {
			continue
		}
		name, _, _ := strings.Cut(id, "@")
		scan(filepath.Join(plugins[id], skillsDirName), name+":", false)
	}
	if configDir != "" {
		scan(filepath.Join(configDir, skillsDirName), "", true)
	}
	for dir := workDir; ; {
		scan(filepath.Join(dir, projectClaudeDirName, skillsDirName), "", true)
		parent := filepath.Dir(dir)
		if dir == repoRoot || parent == dir {
			break
		}
		dir = parent
	}

	if len(skills) == 0 {
		return "", nil, warnings
	}
	slices.SortFunc(skills, func(a, b listedSkill) int {
		return strings.Compare(a.name+"\x00"+a.path, b.name+"\x00"+b.path)
	})
	lines := []string{skillListingRule}
	names := make([]string, len(skills))
	for i, skill := range skills {
		names[i] = skill.name
		lines = append(lines, fmt.Sprintf("- %s: %s (%s)", skill.name, skill.description, skill.path))
	}
	return strings.Join(lines, "\n"), names, warnings
}

// isDirectory reports whether path names a directory.
func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// skillDescription returns the `description:` value of a SKILL.md's frontmatter on one line, or "" when it has none.
func skillDescription(data []byte) string {
	rest, found := strings.CutPrefix(string(data), "---\n")
	if !found {
		return ""
	}
	block, _, found := strings.Cut(rest, "\n---")
	if !found {
		return ""
	}
	var frontmatter struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(block), &frontmatter); err != nil {
		return ""
	}
	return strings.Join(strings.Fields(frontmatter.Description), " ")
}
