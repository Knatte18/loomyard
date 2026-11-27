// usersetup_test.go covers the user-setup source: the CLAUDE.md files it returns and the skill listing it builds.

package claudeengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// pluginInstall is one install of a plugin that writePluginInstalls records.
type pluginInstall struct {
	// ID is the plugin's `<name>@<marketplace>` id.
	ID string
	// Scope is the install scope, user or project.
	Scope string
	// ProjectPath is the repository a project-scope install belongs to.
	ProjectPath string
	// Root is the plugin's install directory.
	Root string
}

// writePluginInstalls writes plugins/installed_plugins.json under configDir with one entry per install.
// Each install path is the row's Root, a directory under the test's temp dir.
// writeInstalledPlugins cannot give that since it writes a fixed /plugins/<id> path.
func writePluginInstalls(t *testing.T, configDir string, installs []pluginInstall) {
	t.Helper()
	plugins := map[string][]map[string]string{}
	for _, install := range installs {
		entry := map[string]string{"scope": install.Scope, "installPath": install.Root}
		if install.ProjectPath != "" {
			entry["projectPath"] = install.ProjectPath
		}
		plugins[install.ID] = append(plugins[install.ID], entry)
	}
	data, err := json.Marshal(map[string]any{"version": 2, "plugins": plugins})
	if err != nil {
		t.Fatalf("marshal installed plugins: %v", err)
	}
	writeUserSetupFile(t, filepath.Join(configDir, installedPluginsPath), string(data))
}

// writeUserSetupFile writes content to path, making its directory.
func writeUserSetupFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// writeSkillFile writes skills/<name>/SKILL.md under root and returns the file's path.
// A non-empty description goes into the frontmatter; an empty one leaves the file with no frontmatter.
func writeSkillFile(t *testing.T, root, name, description string) string {
	t.Helper()
	path := filepath.Join(root, skillsDirName, name, skillFileName)
	content := "just a body\n"
	if description != "" {
		content = "---\nname: " + name + "\ndescription: " + description + "\n---\nbody\n"
	}
	writeUserSetupFile(t, path, content)
	return path
}

// userSetupFixture is a temp tree holding a repository, a Claude config directory and a source reading them.
type userSetupFixture struct {
	base      string
	configDir string
	repo      string
	source    *UserSetupSource
}

// newUserSetupFixture returns a fixture whose claude.yaml layer is claudeYAML.
func newUserSetupFixture(t *testing.T, claudeYAML string) userSetupFixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	fixture := userSetupFixture{base: base, configDir: filepath.Join(base, "config"), repo: filepath.Join(base, "a", "b", "repo")}
	if err := os.MkdirAll(fixture.repo, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if claudeYAML != "" {
		writeUserSetupFile(t, configengine.ConfigFile(base, "claude"), claudeYAML)
	}
	fixture.source = NewUserSetupSource(configengine.Dirs{Anchor: base})
	fixture.source.claudeDir = fixture.configDir
	return fixture
}

// TestUserSetup_InstructionFiles pins the CLAUDE.md files a session gets.
// They are the global file, then the files in the directories above the repository root, outermost first.
// Each import is expanded.
// A file inside the root is the agent's own to read and is not listed.
// An absent file adds nothing, and an unreadable global file is an error.
// An unreadable file above the root is left out with a warning.
// Both unreadable rows stand a directory at the file's path, which fails to read for every user, root included.
func TestUserSetup_InstructionFiles(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// global, outer, inner and inRoot are the text of the file at each place; empty writes none.
		global, outer, inner, inRoot string
		// globalUnreadable and outerUnreadable stand a directory at the file's path instead of a file.
		globalUnreadable, outerUnreadable bool
		// wantTexts are the texts of the items under the fixture, in order.
		wantTexts []string
		// wantErrGlobal expects an error naming the global file.
		wantErrGlobal bool
		// wantWarnOuter expects a warning naming the outer file.
		wantWarnOuter bool
	}{
		{
			name: "GlobalOuterAndInnerExpanded", global: "global @SHARED", outer: "outer @SHARED", inner: "inner @SHARED",
			wantTexts: []string{"global shared text", "outer shared text", "inner shared text"},
		},
		{name: "AbsentGlobalAddsNothing", outer: "outer", inner: "inner", wantTexts: []string{"outer", "inner"}},
		{name: "FileInsideTheRootIsNotListed", inner: "inner", inRoot: "root", wantTexts: []string{"inner"}},
		{name: "UnreadableGlobalIsAnError", globalUnreadable: true, inner: "inner", wantErrGlobal: true},
		{name: "UnreadableAboveTheRootWarns", outerUnreadable: true, inner: "inner", wantTexts: []string{"inner"}, wantWarnOuter: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newUserSetupFixture(t, "")
			shared := filepath.Join(f.base, "shared.md")
			writeUserSetupFile(t, shared, "shared text")
			globalPath := filepath.Join(f.configDir, instructionFileName)
			outerPath := filepath.Join(f.base, "a", instructionFileName)
			files := map[string]string{
				globalPath: tt.global,
				outerPath:  tt.outer,
				filepath.Join(f.base, "a", "b", instructionFileName): tt.inner,
				filepath.Join(f.repo, instructionFileName):           tt.inRoot,
			}
			for path, content := range files {
				if content != "" {
					writeUserSetupFile(t, path, strings.ReplaceAll(content, "@SHARED", "@"+shared))
				}
			}
			for path, unreadable := range map[string]bool{globalPath: tt.globalUnreadable, outerPath: tt.outerUnreadable} {
				if unreadable {
					if err := os.MkdirAll(path, 0o755); err != nil {
						t.Fatalf("MkdirAll: %v", err)
					}
				}
			}

			setup, err := f.source.UserSetup(shuttleengine.UserSetupRequest{WorkDir: f.repo, RepoRoot: f.repo})
			if tt.wantErrGlobal {
				if err == nil || !strings.Contains(err.Error(), globalPath) {
					t.Fatalf("UserSetup() error = %v; want one naming %s", err, globalPath)
				}
				return
			}
			if err != nil {
				t.Fatalf("UserSetup() error: %v", err)
			}
			var gotTexts []string
			for _, item := range setup.InstructionFiles {
				if strings.HasPrefix(item.Name, f.base+string(filepath.Separator)) {
					gotTexts = append(gotTexts, item.Text)
				}
			}
			if fmt.Sprint(gotTexts) != fmt.Sprint(tt.wantTexts) {
				t.Errorf("InstructionFiles under the fixture = %q; want %q", gotTexts, tt.wantTexts)
			}
			if got := strings.Contains(strings.Join(setup.Warnings, "\n"), outerPath); got != tt.wantWarnOuter {
				t.Errorf("Warnings = %q; naming %s = %v, want %v", setup.Warnings, outerPath, got, tt.wantWarnOuter)
			}
		})
	}
}

// TestUserSetup_UnresolvableConfigDir pins a Claude config directory that cannot be resolved.
// The parts that need it are left out: the global file and every plugin skill and context.
// The project's skills and a CLAUDE.md above the repository root are kept.
// It returns no error and one warning naming the environment variable.
// It clears CLAUDE_CONFIG_DIR, HOME and USERPROFILE, so it does not run in parallel.
func TestUserSetup_UnresolvableConfigDir(t *testing.T) {
	tiergate.Require(t, 0)

	t.Setenv(claudeConfigDirEnv, "")
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	f := newUserSetupFixture(t, "")
	f.source.claudeDir = ""
	writeUserSetupFile(t, filepath.Join(f.base, "a", instructionFileName), "outer")
	local := writeSkillFile(t, filepath.Join(f.repo, projectClaudeDirName), "local", "Repository skill.")

	setup, err := f.source.UserSetup(shuttleengine.UserSetupRequest{WorkDir: f.repo, RepoRoot: f.repo})
	if err != nil {
		t.Fatalf("UserSetup() error: %v", err)
	}
	if len(setup.Warnings) != 1 || !strings.Contains(setup.Warnings[0], claudeConfigDirEnv) {
		t.Errorf("Warnings = %q; want one naming %s", setup.Warnings, claudeConfigDirEnv)
	}
	var gotTexts []string
	for _, item := range setup.InstructionFiles {
		if strings.HasPrefix(item.Name, f.base+string(filepath.Separator)) {
			gotTexts = append(gotTexts, item.Text)
		}
	}
	if fmt.Sprint(gotTexts) != fmt.Sprint([]string{"outer"}) {
		t.Errorf("InstructionFiles under the fixture = %q; want the outer file alone", gotTexts)
	}
	if want := skillListingRule + "\n- local: Repository skill. (" + local + ")"; setup.SkillListing != want {
		t.Errorf("SkillListing = %q; want %q", setup.SkillListing, want)
	}
	if len(setup.PluginContexts) != 0 || len(setup.Omitted) != 0 {
		t.Errorf("PluginContexts = %+v, Omitted = %+v; want none", setup.PluginContexts, setup.Omitted)
	}
}

// TestUserSetup_SkillListing pins the skill listing of a session.
// A scope that preloads one plugin skill lists that plugin's skills and no other plugin's.
// A user skill the scope turns off is left out, and a project skill under the repository root is listed.
// A disabled scope lists every installed plugin's skills.
// A project-scope install for another repository is not listed.
// A skill with no description is listed with an empty one.
// A malformed or unreadable plugins file lists no plugin skill, returns no error and warns naming the file.
// A plugin with no install path and an unreadable SKILL.md are left out with a warning naming them.
func TestUserSetup_SkillListing(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const enabledScope = "session_scope:\n  enabled: true\n  skills_off: [notes]\n"
	line := func(name, description, path string) string {
		return fmt.Sprintf("- %s: %s (%s)", name, description, path)
	}
	tests := []struct {
		name   string
		claude string
		// setup writes the fixture's skills and plugins.
		// It returns the session's preloaded skills, the listing lines wanted in order and a text the warnings must hold.
		setup func(t *testing.T, f userSetupFixture) (skills, wantLines []string, wantWarn string)
	}{
		{
			name: "ScopePreloadsOnePluginSkill", claude: enabledScope,
			setup: func(t *testing.T, f userSetupFixture) ([]string, []string, string) {
				scribe, other := filepath.Join(f.base, "plugins", "scribe"), filepath.Join(f.base, "plugins", "other")
				writePluginInstalls(t, f.configDir, []pluginInstall{
					{ID: "scribe@market", Scope: "user", Root: scribe},
					{ID: "other@market", Scope: "user", Root: other},
				})
				prose := writeSkillFile(t, scribe, "prose", "Write tersely.")
				testingSkill := writeSkillFile(t, scribe, "testing", "Test behavior.")
				writeSkillFile(t, other, "unrelated", "Not for this session.")
				return []string{"scribe:prose"}, []string{line("scribe:prose", "Write tersely.", prose), line("scribe:testing", "Test behavior.", testingSkill)}, ""
			},
		},
		{
			name: "UserSkillTurnedOffIsLeftOut", claude: enabledScope,
			setup: func(t *testing.T, f userSetupFixture) ([]string, []string, string) {
				writeSkillFile(t, f.configDir, "notes", "Turned off.")
				kept := writeSkillFile(t, f.configDir, "kept", "Stays.")
				return nil, []string{line("kept", "Stays.", kept)}, ""
			},
		},
		{
			name: "ProjectSkillIsListed", claude: enabledScope,
			setup: func(t *testing.T, f userSetupFixture) ([]string, []string, string) {
				local := writeSkillFile(t, filepath.Join(f.repo, projectClaudeDirName), "local", "Repository skill.")
				return nil, []string{line("local", "Repository skill.", local)}, ""
			},
		},
		{
			name: "DisabledScopeListsEveryPlugin", claude: "session_scope:\n  enabled: false\n",
			setup: func(t *testing.T, f userSetupFixture) ([]string, []string, string) {
				scribe, other := filepath.Join(f.base, "plugins", "scribe"), filepath.Join(f.base, "plugins", "other")
				writePluginInstalls(t, f.configDir, []pluginInstall{
					{ID: "scribe@market", Scope: "user", Root: scribe},
					{ID: "other@market", Scope: "user", Root: other},
				})
				unrelated := writeSkillFile(t, other, "unrelated", "Listed anyway.")
				prose := writeSkillFile(t, scribe, "prose", "Write tersely.")
				return nil, []string{line("other:unrelated", "Listed anyway.", unrelated), line("scribe:prose", "Write tersely.", prose)}, ""
			},
		},
		{
			name: "ProjectInstallForAnotherRepositoryIsNotListed", claude: "session_scope:\n  enabled: false\n",
			setup: func(t *testing.T, f userSetupFixture) ([]string, []string, string) {
				mine, theirs := filepath.Join(f.base, "plugins", "mine"), filepath.Join(f.base, "plugins", "theirs")
				writePluginInstalls(t, f.configDir, []pluginInstall{
					{ID: "mine@market", Scope: "project", ProjectPath: f.repo, Root: mine},
					{ID: "theirs@market", Scope: "project", ProjectPath: filepath.Join(f.base, "elsewhere"), Root: theirs},
				})
				own := writeSkillFile(t, mine, "own", "This repository's.")
				writeSkillFile(t, theirs, "foreign", "Another repository's.")
				return nil, []string{line("mine:own", "This repository's.", own)}, ""
			},
		},
		{
			name: "SkillWithNoDescriptionIsListedEmpty", claude: enabledScope,
			setup: func(t *testing.T, f userSetupFixture) ([]string, []string, string) {
				bare := writeSkillFile(t, f.configDir, "bare", "")
				return nil, []string{line("bare", "", bare)}, ""
			},
		},
		{
			name: "MalformedPluginsFileListsNoPluginSkill", claude: enabledScope,
			setup: func(t *testing.T, f userSetupFixture) ([]string, []string, string) {
				plugins := filepath.Join(f.configDir, installedPluginsPath)
				writeUserSetupFile(t, plugins, "{")
				kept := writeSkillFile(t, f.configDir, "kept", "Stays.")
				return nil, []string{line("kept", "Stays.", kept)}, plugins
			},
		},
		{
			name: "UnreadablePluginsFileListsNoPluginSkill", claude: enabledScope,
			setup: func(t *testing.T, f userSetupFixture) ([]string, []string, string) {
				plugins := filepath.Join(f.configDir, installedPluginsPath)
				if err := os.MkdirAll(plugins, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				kept := writeSkillFile(t, f.configDir, "kept", "Stays.")
				return nil, []string{line("kept", "Stays.", kept)}, plugins
			},
		},
		{
			name: "PluginWithNoInstallPathIsLeftOut", claude: "session_scope:\n  enabled: false\n",
			setup: func(t *testing.T, f userSetupFixture) ([]string, []string, string) {
				writePluginInstalls(t, f.configDir, []pluginInstall{{ID: "bare@market", Scope: "user"}})
				kept := writeSkillFile(t, f.configDir, "kept", "Stays.")
				return nil, []string{line("kept", "Stays.", kept)}, "bare@market"
			},
		},
		{
			name: "UnreadableSkillFileIsLeftOut", claude: "session_scope:\n  enabled: false\n",
			setup: func(t *testing.T, f userSetupFixture) ([]string, []string, string) {
				scribe := filepath.Join(f.base, "plugins", "scribe")
				writePluginInstalls(t, f.configDir, []pluginInstall{{ID: "scribe@market", Scope: "user", Root: scribe}})
				broken := filepath.Join(scribe, skillsDirName, "broken", skillFileName)
				if err := os.MkdirAll(broken, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				prose := writeSkillFile(t, scribe, "prose", "Write tersely.")
				return nil, []string{line("scribe:prose", "Write tersely.", prose)}, broken
			},
		},
		{
			name: "NoSkillsGiveAnEmptyListing", claude: enabledScope,
			setup: func(t *testing.T, f userSetupFixture) ([]string, []string, string) { return nil, nil, "" },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newUserSetupFixture(t, tt.claude)
			skills, wantLines, wantWarn := tt.setup(t, f)

			setup, err := f.source.UserSetup(shuttleengine.UserSetupRequest{WorkDir: f.repo, RepoRoot: f.repo, Skills: skills})
			if err != nil {
				t.Fatalf("UserSetup() error: %v", err)
			}
			if len(wantLines) == 0 {
				if setup.SkillListing != "" {
					t.Errorf("SkillListing = %q; want it empty", setup.SkillListing)
				}
			} else {
				var gotLines []string
				for _, listed := range strings.Split(setup.SkillListing, "\n") {
					if strings.HasPrefix(listed, "- ") {
						gotLines = append(gotLines, listed)
					}
				}
				if fmt.Sprint(gotLines) != fmt.Sprint(wantLines) {
					t.Errorf("SkillListing lines = %q; want %q", gotLines, wantLines)
				}
				if !strings.HasPrefix(setup.SkillListing, skillListingRule+"\n") {
					t.Errorf("SkillListing = %q; want it to open with the rule for using a skill", setup.SkillListing)
				}
			}
			var wantNames []string
			for _, listed := range wantLines {
				name, _, _ := strings.Cut(strings.TrimPrefix(listed, "- "), ": ")
				wantNames = append(wantNames, name)
			}
			if !slices.Equal(setup.ListedSkills, wantNames) {
				t.Errorf("ListedSkills = %q; want %q", setup.ListedSkills, wantNames)
			}
			if warnings := strings.Join(setup.Warnings, "\n"); wantWarn == "" && warnings != "" || !strings.Contains(warnings, wantWarn) {
				t.Errorf("Warnings = %q; want one naming %q", setup.Warnings, wantWarn)
			}
		})
	}
}

// sessionStartHooksJSON returns a hooks file with one SessionStart entry of the given matcher holding one command hook.
// A zero timeout leaves the hook's own timeout out.
func sessionStartHooksJSON(matcher string, timeoutSeconds int, command string) string {
	timeout := ""
	if timeoutSeconds > 0 {
		timeout = fmt.Sprintf(`,"timeout":%d`, timeoutSeconds)
	}
	return fmt.Sprintf(`{"hooks":{"SessionStart":[{"matcher":%q,"hooks":[{"type":"command","command":%q%s}]}]}}`, matcher, command, timeout)
}

// TestUserSetup_PluginSessionStartContexts pins the context a session gets from its plugins' SessionStart hooks.
// A plugin the scope turns off runs no hook, and a hook whose matcher misses startup is not run.
// A hook that fails, or that no runner exists for, is left out and listed as omitted.
// So is a hooks file holding a matcher that is no pattern, and a hook whose type is not command is not run.
// The recorded run carries Claude Code's stdin, the two plugin variables and the hook's timeout.
func TestUserSetup_PluginSessionStartContexts(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const (
		enabledScope   = "session_scope:\n  enabled: true\n"
		requestTimeout = 10 * time.Second
		hookName       = "plugin scribe@market SessionStart hook"
	)
	preloaded := []string{"scribe:prose"}
	tests := []struct {
		name string
		// skills are the session's preloaded skills; none leaves the plugin off in the scope.
		skills []string
		// hooks is the plugin's hooks file text; empty writes no file.
		hooks string
		// stdout and runErr are what the fake runner answers.
		stdout string
		runErr error
		// nilRunner leaves the request's RunHook nil.
		nilRunner bool
		// wantRuns is the number of hook runs the fake runner records.
		wantRuns int
		// wantTexts are the contexts of the items, in order.
		wantTexts []string
		// wantOmittedName and wantOmittedReason are texts the one omitted hook's name and reason hold; empty wants none omitted.
		wantOmittedName, wantOmittedReason string
		// wantTimeout is the timeout of the recorded run.
		wantTimeout time.Duration
	}{
		{
			name: "PlainTextIsTheContext", skills: preloaded, hooks: sessionStartHooksJSON("", 0, "emit"),
			stdout: "  plain context \n", wantRuns: 1, wantTexts: []string{"plain context"}, wantTimeout: requestTimeout,
		},
		{
			name: "JSONGivesItsAdditionalContext", skills: preloaded, hooks: sessionStartHooksJSON("*", 7, "emit"),
			stdout:   `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"from json"}}`,
			wantRuns: 1, wantTexts: []string{"from json"}, wantTimeout: 7 * time.Second,
		},
		{
			name: "MatcherListingStartupRuns", skills: preloaded, hooks: sessionStartHooksJSON("startup|resume", 0, "emit"),
			stdout: "text", wantRuns: 1, wantTexts: []string{"text"}, wantTimeout: requestTimeout,
		},
		{name: "NoOutputAddsNoItem", skills: preloaded, hooks: sessionStartHooksJSON("", 0, "emit"), wantRuns: 1, wantTimeout: requestTimeout},
		{
			name: "JSONWithoutAdditionalContextAddsNoItem", skills: preloaded, hooks: sessionStartHooksJSON("", 0, "emit"),
			stdout: `{"continue":true}`, wantRuns: 1, wantTimeout: requestTimeout,
		},
		{name: "ResumeMatcherIsNotRun", skills: preloaded, hooks: sessionStartHooksJSON("resume", 0, "emit"), stdout: "text"},
		{name: "OffPluginIsNotRun", hooks: sessionStartHooksJSON("", 0, "emit"), stdout: "text"},
		{
			name: "MalformedHooksFileRunsNothingAndIsOmitted", skills: preloaded, hooks: "{",
			wantOmittedName: filepath.Join("hooks", "hooks.json"), wantOmittedReason: "parse the hooks file",
		},
		{
			name: "InvalidMatcherRunsNothingAndIsOmitted", skills: preloaded, hooks: sessionStartHooksJSON("[", 0, "emit"),
			wantOmittedName: filepath.Join("hooks", "hooks.json"), wantOmittedReason: "no pattern",
		},
		{
			name: "NonCommandHookTypeIsNotRun", skills: preloaded,
			hooks: `{"hooks":{"SessionStart":[{"hooks":[{"type":"http","command":"emit"}]}]}}`,
		},
		{
			name: "FailingHookIsOmittedWithItsCause", skills: preloaded, hooks: sessionStartHooksJSON("", 0, "emit"),
			runErr: fmt.Errorf("exit status 3"), wantRuns: 1, wantOmittedName: hookName, wantOmittedReason: "exit status 3", wantTimeout: requestTimeout,
		},
		{
			name: "NilRunnerOmitsEveryHookAsNotRun", skills: preloaded, hooks: sessionStartHooksJSON("", 0, "emit"), nilRunner: true,
			wantOmittedName: hookName, wantOmittedReason: "not run",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newUserSetupFixture(t, enabledScope)
			root := filepath.Join(f.base, "plugins", "scribe")
			writePluginInstalls(t, f.configDir, []pluginInstall{{ID: "scribe@market", Scope: "user", Root: root}})
			if tt.hooks != "" {
				writeUserSetupFile(t, filepath.Join(root, "hooks", "hooks.json"), tt.hooks)
			}
			var runs []shuttleengine.HookRun
			req := shuttleengine.UserSetupRequest{
				WorkDir: filepath.Join(f.repo, "sub"), RepoRoot: f.repo, Skills: tt.skills, SessionID: "run-1", HookTimeout: requestTimeout,
			}
			if !tt.nilRunner {
				req.RunHook = func(run shuttleengine.HookRun) ([]byte, error) {
					runs = append(runs, run)
					return []byte(tt.stdout), tt.runErr
				}
			}

			setup, err := f.source.UserSetup(req)
			if err != nil {
				t.Fatalf("UserSetup() error: %v", err)
			}
			if len(runs) != tt.wantRuns {
				t.Fatalf("hook runs = %d; want %d", len(runs), tt.wantRuns)
			}
			var gotTexts []string
			for _, item := range setup.PluginContexts {
				gotTexts = append(gotTexts, item.Text)
				if item.Name != hookName || !strings.Contains(item.WayForward, "session scope") || item.Plugin != "scribe@market" || item.Hook != "emit" {
					t.Errorf("plugin context item = %+v; want name %q, its plugin and hook, and a way forward naming the session scope", item, hookName)
				}
			}
			if !reflect.DeepEqual(gotTexts, tt.wantTexts) {
				t.Errorf("PluginContexts texts = %q; want %q", gotTexts, tt.wantTexts)
			}
			if tt.wantOmittedName == "" {
				if len(setup.Omitted) != 0 {
					t.Errorf("Omitted = %+v; want none", setup.Omitted)
				}
			} else if len(setup.Omitted) != 1 || !strings.Contains(setup.Omitted[0].Name, tt.wantOmittedName) ||
				!strings.Contains(setup.Omitted[0].Reason, tt.wantOmittedReason) {
				t.Errorf("Omitted = %+v; want one with name holding %q and reason holding %q", setup.Omitted, tt.wantOmittedName, tt.wantOmittedReason)
			}
			for _, omitted := range setup.Omitted {
				// A hooks file that cannot be read names no one hook.
				wantPlugin, wantHook := "", ""
				if tt.wantOmittedName == hookName {
					wantPlugin, wantHook = "scribe@market", "emit"
				}
				if omitted.Plugin != wantPlugin || omitted.Hook != wantHook {
					t.Errorf("Omitted plugin, hook = %q, %q; want %q, %q", omitted.Plugin, omitted.Hook, wantPlugin, wantHook)
				}
			}
			if len(runs) == 0 {
				return
			}
			run := runs[0]
			if run.Command != "emit" || run.Dir != req.WorkDir || run.Timeout != tt.wantTimeout {
				t.Errorf("hook run = %+v; want command emit in %s under %s", run, req.WorkDir, tt.wantTimeout)
			}
			for _, want := range []string{"CLAUDE_PLUGIN_ROOT=" + root, "CLAUDE_PROJECT_DIR=" + req.WorkDir} {
				if !slices.Contains(run.Env, want) {
					t.Errorf("hook run Env = %q; want it to hold %q", run.Env, want)
				}
			}
			var gotStdin map[string]string
			if err := json.Unmarshal(run.Stdin, &gotStdin); err != nil {
				t.Fatalf("hook run Stdin %q is not JSON: %v", run.Stdin, err)
			}
			wantStdin := map[string]string{
				"session_id": "run-1", "transcript_path": "", "cwd": req.WorkDir, "hook_event_name": "SessionStart", "source": "startup",
			}
			if !reflect.DeepEqual(gotStdin, wantStdin) {
				t.Errorf("hook run Stdin = %v; want %v", gotStdin, wantStdin)
			}
		})
	}
}
