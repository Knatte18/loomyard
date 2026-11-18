// accountenv_test.go tests the account a launch resolves.
// It covers the CLAUDE_CONFIG_DIR export or unset and the account record.

package claudeengine

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// engineWithAccounts returns an engine told worktree, and the hub's board directory.
// The hub is a temp one whose machine-layer claude.yaml is machineDocument.
func engineWithAccounts(t testing.TB, machineDocument string, worktree shuttleengine.Worktree) (*Claude, string) {
	t.Helper()
	shuttleCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: t.TempDir()}, "shuttle")
	if err != nil {
		t.Fatalf("load the shipped shuttle template: %v", err)
	}
	board, anchor := t.TempDir(), t.TempDir()
	writeClaudeLayer(t, configengine.ConfigFile(board, "claude"), ConfigTemplate())
	writeClaudeLayer(t, configengine.MachineFile(board, "claude"), machineDocument)
	engine := NewForWorktree(shuttleCfg, configengine.Dirs{Board: board, Anchor: anchor}, worktree)
	engine.claudeDir = t.TempDir()
	return engine, board
}

// configDirAssignment returns the pane shell's rendering of CLAUDE_CONFIG_DIR assigned to dir.
func configDirAssignment(dir string) string {
	const marker = "|"
	return strings.TrimSuffix(shell.ForGOOS().WithEnv(claudeConfigDirEnv, dir, marker), marker)
}

// TestPrepare_ExportsTheResolvedAccount pins the account a launch resolves.
// It also pins what the launch's two lines and account.json say of it.
func TestPrepare_ExportsTheResolvedAccount(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	home, err := claudeHomeDir()
	if err != nil {
		t.Fatalf("resolve the home directory: %v", err)
	}
	now := time.Unix(3_000_000, 0)
	const (
		two      = "accounts:\n  - {name: a, config_dir: ROOT/a, weight: 1}\n  - {name: b, config_dir: ROOT/b, weight: 1}\n"
		hubPin   = "account_pins:\n  hub: a\n"
		beamsPin = "account_pins:\n  beams: b\n"
	)
	task := shuttleengine.Worktree{Name: "t", Kind: shuttleengine.WorktreeTask}
	prime := shuttleengine.Worktree{Name: "prime", Kind: shuttleengine.WorktreePrime}
	beam := shuttleengine.Worktree{Name: "t", Kind: shuttleengine.WorktreeBeam}
	entry := func(account string) accountFile {
		return accountFile{Worktrees: map[string]string{"t": account}, Picked: map[string]time.Time{"t": now}}
	}
	primeEntry := accountFile{Worktrees: map[string]string{"prime": "a"}}

	tests := []struct {
		name string
		// machine is the machine-layer claude.yaml; ROOT stands for the temp directory of the fake config directories.
		machine string
		// worktree is the told worktree; a nil one makes the engine told no worktree.
		worktree *shuttleengine.Worktree
		file     accountFile
		// blockedB makes account b's usage record block it until after now.
		blockedB bool
		// env is the hub's env.yaml; empty writes none.
		env  string
		role string
		// accountPlugins gives the exported account's directory a plugins file and a global CLAUDE.md.
		// The engine then has no config directory of its own.
		accountPlugins bool
		// wantDir is the directory both lines export, ROOT-relative; empty means no export.
		wantDir   string
		wantUnset bool
		// wantRecord is the account.json the launch writes; its ConfigDir is ROOT-relative unless it is absolute.
		wantRecord launchedAccount
		wantErr    bool
	}{
		{name: "AnEntryOnANonDefaultDirectoryIsExported", machine: two, worktree: &task, file: entry("b"), wantDir: "b", wantRecord: launchedAccount{ConfigDir: "b", Account: "b"}},
		{
			name: "AnAccountOnTheDefaultDirectoryUnsetsIt", machine: "accounts:\n  - {name: home, config_dir: '~/.claude', weight: 1}\n  - {name: b, config_dir: ROOT/b, weight: 1}\n",
			worktree: &task, file: entry("home"), wantUnset: true, wantRecord: launchedAccount{ConfigDir: filepath.Join(home, ".claude"), Account: "home"},
		},
		{name: "NoAccountLeavesBothLinesAlone", machine: two, worktree: &task, wantRecord: launchedAccount{ConfigDir: filepath.Join(home, ".claude")}},
		{name: "AHubPinBeatsAnEntryAndABeamsPin", machine: two + hubPin + "  beams: b\n", worktree: &prime, file: accountFile{Worktrees: map[string]string{"prime": "b"}}, wantDir: "a", wantRecord: launchedAccount{ConfigDir: "a", Account: "a"}},
		{name: "ABeamsPinAppliesToThePrime", machine: two + beamsPin, worktree: &prime, file: primeEntry, wantDir: "b", wantRecord: launchedAccount{ConfigDir: "b", Account: "b"}},
		{name: "ABeamsPinAppliesToABeam", machine: two + beamsPin, worktree: &beam, file: entry("a"), wantDir: "b", wantRecord: launchedAccount{ConfigDir: "b", Account: "b"}},
		{name: "ABeamsPinDoesNotApplyToATask", machine: two + beamsPin, worktree: &task, file: entry("a"), wantDir: "a", wantRecord: launchedAccount{ConfigDir: "a", Account: "a"}},
		{name: "ABlockedBeamsPinYieldsToTheEntry", machine: two + beamsPin, worktree: &beam, file: entry("a"), blockedB: true, wantDir: "a", wantRecord: launchedAccount{ConfigDir: "a", Account: "a"}},
		{name: "ABlockedBeamsPinStandsWithNoEntry", machine: two + beamsPin, worktree: &beam, blockedB: true, wantDir: "b", wantRecord: launchedAccount{ConfigDir: "b", Account: "b"}},
		{name: "ABlockedHubPinStands", machine: two + "account_pins:\n  hub: b\n", worktree: &task, file: entry("a"), blockedB: true, wantDir: "b", wantRecord: launchedAccount{ConfigDir: "b", Account: "b"}},
		{
			name: "PluginsAndUserSetupComeFromTheExportedAccountsDirectory", machine: two, worktree: &task, file: entry("b"), accountPlugins: true,
			wantDir: "b", wantRecord: launchedAccount{ConfigDir: "b", Account: "b"},
		},
		{name: "AnEntryNamingARemovedAccountLaunchesWithNeither", machine: two, worktree: &task, file: entry("ghost"), wantRecord: launchedAccount{ConfigDir: filepath.Join(home, ".claude")}},
		{
			name: "AnEntryNamingARetiredAccountLaunchesWithNeither", machine: "accounts:\n  - {name: a, config_dir: ROOT/a, weight: 1}\n  - {name: old, config_dir: ROOT/old, weight: 0}\n",
			worktree: &task, file: entry("old"), wantRecord: launchedAccount{ConfigDir: filepath.Join(home, ".claude")},
		},
		{name: "AnEngineToldNoWorktreeResolvesOnlyTheHubPin", machine: two + beamsPin, wantRecord: launchedAccount{ConfigDir: filepath.Join(home, ".claude")}},
		{name: "AnEngineToldNoWorktreeStillExportsTheHubPin", machine: two + hubPin, wantDir: "a", wantRecord: launchedAccount{ConfigDir: "a", Account: "a"}},
		{
			name: "TheRecordNamesTheListedAccountOfTheEnvModulesDirectory", machine: two, worktree: &task, env: "vars:\n  CLAUDE_CONFIG_DIR: ROOT/b\n",
			wantRecord: launchedAccount{ConfigDir: "b", Account: "b"},
		},
		{name: "AnInvalidAccountListRefusesBeforeAnyArtifact", machine: "accounts:\n  - {name: a, config_dir: ROOT/a, weight: 1}\n  - {name: a, config_dir: ROOT/b, weight: 1}\n", worktree: &task, wantErr: true},
		{
			name: "AHubPinNamingTheOnlyAccountIsStillExported", machine: "accounts:\n  - {name: a, config_dir: ROOT/a, weight: 1}\n" + hubPin, worktree: &task,
			wantDir: "a", wantRecord: launchedAccount{ConfigDir: "a", Account: "a"},
		},
		{name: "AnOrchSpecToldThePrimeUnderABeamsPinExportsThePin", machine: two + beamsPin, worktree: &prime, role: "orch", wantDir: "b", wantRecord: launchedAccount{ConfigDir: "b", Account: "b"}},
		{name: "ADobbySpecToldThePrimeUnderABeamsPinExportsThePin", machine: two + beamsPin, worktree: &prime, role: "dobby", wantDir: "b", wantRecord: launchedAccount{ConfigDir: "b", Account: "b"}},
		{
			name: "AnAccountsExportWinsOverTheEnvModulesDirectory", machine: two, worktree: &task, file: entry("a"), env: "vars:\n  CLAUDE_CONFIG_DIR: ROOT/c\n",
			wantDir: "a", wantRecord: launchedAccount{ConfigDir: "a", Account: "a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root, accountDir := t.TempDir(), t.TempDir()
			underRoot := func(path string) string {
				if filepath.IsAbs(path) {
					return path
				}
				return filepath.Join(root, path)
			}
			worktree := shuttleengine.Worktree{}
			if tt.worktree != nil {
				worktree = *tt.worktree
				worktree.AccountDir = accountDir
			}
			engine, board := engineWithAccounts(t, strings.ReplaceAll(tt.machine, "ROOT", root), worktree)
			engine.now = func() time.Time { return now }
			if tt.env != "" {
				writeClaudeLayer(t, configengine.ConfigFile(board, "env"), strings.ReplaceAll(tt.env, "ROOT", root))
			}
			if tt.accountPlugins {
				engine.claudeDir = ""
				writeClaudeLayer(t, filepath.Join(root, "b", installedPluginsPath), `{"plugins":{"demo@market":[{"scope":"user"}]}}`)
				writeClaudeLayer(t, filepath.Join(root, "b", instructionFileName), "the account's rules\n")
			}
			if err := updateAccountFile(accountDir, func(accountFile) (accountFile, error) { return tt.file, nil }); err != nil {
				t.Fatalf("seed the account file: %v", err)
			}
			if tt.blockedB {
				if err := recordBlocked(accountDir, "b", now.Add(time.Hour)); err != nil {
					t.Fatalf("record account b as blocked: %v", err)
				}
			}

			runDir := t.TempDir()
			launch, err := engine.Prepare(runDir, shuttleengine.Spec{Prompt: "do the thing", PromptEngine: engineName, Role: tt.role}, shuttleengine.Config{})
			if tt.wantErr {
				if err == nil {
					t.Fatal("Prepare() = nil error; want the invalid account list refused")
				}
				if _, statErr := os.Stat(filepath.Join(runDir, "prompt.md")); !os.IsNotExist(statErr) {
					t.Errorf("prompt.md exists after a refused Prepare (stat err=%v); want no artifacts written", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Prepare() error: %v; want nil", err)
			}

			sh := shell.ForGOOS()
			for label, line := range map[string]string{"Launch.Cmd": launch.Cmd, "Launch.ResumeCmd": launch.ResumeCmd} {
				switch {
				case tt.wantDir != "":
					if want := configDirAssignment(underRoot(tt.wantDir)); !strings.Contains(line, want) {
						t.Errorf("%s = %q; want it to hold %q", label, line, want)
					}
				case tt.wantUnset:
					if want := sh.UnsetEnv(claudeConfigDirEnv); !strings.Contains(line, want) {
						t.Errorf("%s = %q; want it to hold %q", label, line, want)
					}
				default:
					if strings.Contains(line, claudeConfigDirEnv) {
						t.Errorf("%s = %q; want no %s", label, line, claudeConfigDirEnv)
					}
				}
			}
			got, found, err := readLaunchedAccount(runDir)
			want := launchedAccount{ConfigDir: underRoot(tt.wantRecord.ConfigDir), Account: tt.wantRecord.Account}
			if err != nil || !found || got != want {
				t.Errorf("account.json = %+v, found %v, err %v; want %+v", got, found, err, want)
			}
			settings, err := os.ReadFile(filepath.Join(runDir, settingsFileName))
			if err != nil {
				t.Fatalf("read settings.json: %v", err)
			}
			var document struct {
				StatusLine *struct {
					Type    string `json:"type"`
					Command string `json:"command"`
				} `json:"statusLine"`
				Hooks map[string]json.RawMessage `json:"hooks"`
			}
			if err := json.Unmarshal(settings, &document); err != nil {
				t.Fatalf("parse settings.json %q: %v", settings, err)
			}
			switch wantAccount := tt.wantRecord.Account; {
			case wantAccount == "" && document.StatusLine != nil:
				t.Errorf("settings.json statusLine = %+v; want none for a session on an unlisted directory", document.StatusLine)
			case wantAccount != "":
				wantCommand := fmt.Sprintf("'%s' claude statusline '%s'", testLymPath, wantAccount)
				if document.StatusLine == nil || document.StatusLine.Type != "command" || document.StatusLine.Command != wantCommand {
					t.Errorf("settings.json statusLine = %+v; want a command entry running %q", document.StatusLine, wantCommand)
				}
			}
			for _, hook := range []string{"PostToolUse", "Stop"} {
				if _, ok := document.Hooks[hook]; !ok {
					t.Errorf("settings.json hooks = %v; want the %s hooks kept beside the status line", slices.Collect(maps.Keys(document.Hooks)), hook)
				}
			}
			if tt.accountPlugins {
				if !strings.Contains(string(settings), "demo@market") {
					t.Errorf("settings.json = %q; want the plugin the exported account's directory installs", settings)
				}
				manifest, err := os.ReadFile(filepath.Join(runDir, shuttleengine.ManifestFileName))
				if wantFile := filepath.Join(root, "b", instructionFileName); err != nil || !strings.Contains(string(manifest), wantFile) {
					t.Errorf("manifest = %q, err %v; want it to list the exported account's %s", manifest, err, wantFile)
				}
			}
		})
	}
}
