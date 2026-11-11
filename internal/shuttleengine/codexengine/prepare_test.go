// prepare_test.go covers Prepare and the lym Codex home.
// It covers the launch Prepare returns, the trust record and config.toml it renders, and the links it makes.
// It also covers the refusals that write nothing.
// It covers the warnings about a Codex folder in the worktree and about a home entry that is not lym's link.
// It covers the check after startup.

package codexengine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func readRecord(t *testing.T, home string) trustRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, trustRecordFileName))
	if err != nil {
		t.Fatalf("read trust record: %v", err)
	}
	var record trustRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("decode trust record: %v", err)
	}
	return record
}

// TestPrepare_Launch pins the launch a valid spec gets.
// The line is built from the lym home, the model and effort, and the writable roots.
// The writable roots are the told records checkout (none when the engine is told none).
// After it come the git dirs of a linked worktree or a plain clone, then the configured roots that exist.
// The hooks of the run are carried, with a guard per row that the python switch and the publish guard install.
// The line runs the workspace-write sandbox, which the fixture's probe reports as starting.
// The line also carries the prompt pointer.
// It also pins the resume line, which exits and records the session start.
// An engine told no board gets no creel hooks, and the resume line has no session id.
// The prompt file holds the deny notice, the creel trust sentence and the spec's prompt.
func TestPrepare_Launch(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// toldRecords reports whether the engine is told the records checkout.
		toldRecords bool
		// toldThroughLink reports whether the told records checkout is a symlink to the real one.
		toldThroughLink bool
		// plainClone reports whether the launch starts in a plain clone instead of a linked worktree.
		plainClone bool
		// configuredRoots reports whether codex.yaml names an existing writable root and a missing one.
		configuredRoots bool
		// gitDirsThroughLink reports whether the linked worktree reaches its git dir and common dir through a symlink.
		gitDirsThroughLink bool
	}{
		{name: "RecordsTold", toldRecords: true},
		{name: "RecordsToldThroughLink", toldRecords: true, toldThroughLink: true},
		{name: "RecordsNotTold"},
		{name: "PlainClone", plainClone: true},
		{name: "ConfiguredRoots", toldRecords: true, configuredRoots: true},
		{name: "GitDirsThroughLink", gitDirsThroughLink: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newRepoFixture(t)
			var anchor, records string
			if tt.plainClone {
				anchor = repo.addClone(t, "clone")
			} else {
				anchor, records = repo.addWorktree(t, "wt")
			}
			if tt.gitDirsThroughLink {
				repo.routeGitDirsThroughLink(t, anchor)
			}
			engine, warnings := newEngine(anchor)
			var wantRoots []string
			if tt.toldRecords {
				engine.dirs.Records = records
				wantRoots = []string{records}
			}
			if tt.toldThroughLink {
				link := filepath.Join(t.TempDir(), "records-link")
				if err := os.Symlink(records, link); err != nil {
					t.Fatalf("symlink: %v", err)
				}
				engine.dirs.Records = link
			}
			wantRoots = append(wantRoots, repo.wantGitDirs(anchor)...)
			if tt.configuredRoots {
				cache := filepath.Join(repo.base, "cache")
				mkdirAll(t, cache)
				writeWritableRoots(t, anchor, cache+", "+filepath.Join(repo.base, "missing"))
				wantRoots = append(wantRoots, cache)
			}
			runDir := t.TempDir()

			launch, err := engine.Prepare(runDir, shuttleengine.Spec{Prompt: "review the plan", DenyPublish: true, ModelChoice: shuttleengine.ModelChoice{Engine: "codex", Model: "gpt-6-astra", Effort: "medium"}}, shuttleengine.Config{DenyPython: true})
			if err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}

			promptPath := filepath.Join(runDir, "prompt.md")
			guards := installedGuards(nil, guardInputs{denyPython: true, denyPublish: true})
			sh := shell.ForGOOS()
			want := shuttleengine.Launch{
				Cmd: buildLaunchCmd(sh, launchLine{
					bin:           "codex",
					home:          repo.lymHome,
					sandboxMode:   sandboxWorkspaceWrite,
					writableRoots: wantRoots,
					model:         "gpt-6-astra",
					effort:        "medium",
					hooks:         hookSettings(filepath.Join(runDir, "events.jsonl"), guards, creelDelivery{}),
					firstTurn:     launchPointer(promptPath),
					eventsPath:    filepath.Join(runDir, "events.jsonl"),
				}),
				ResumeCmd:            sh.Exit(),
				RecordsSessionStarts: true,
			}
			if !reflect.DeepEqual(launch, want) {
				t.Errorf("Prepare() launch = %+v; want %+v", launch, want)
			}
			if got, err := os.ReadFile(promptPath); err != nil || string(got) != denyNotice(guards)+" "+shuttleengine.CreelTrustSentence+"\n\nreview the plan" {
				t.Errorf("prompt.md = %q, %v; want the deny notice, the creel trust sentence and the spec's prompt", got, err)
			}
			if warnings.String() != "" {
				t.Errorf("warnings = %q; want none", warnings)
			}
		})
	}
}

// TestPrepare_SandboxMode pins the sandbox a launch runs in for each sandbox_mode and probe result.
// auto and workspace-write probe the sandbox once, with the Codex executable, under the lym home, in the anchor;
// a sandbox that starts runs workspace-write.
// One that cannot start runs danger-full-access under auto, with a warning naming the cause.
// Under workspace-write it refuses the launch before the prompt file is written, naming the cause and the way forward.
// A probe that did not run to a verdict refuses the launch under both modes.
// danger-full-access runs no probe.
func TestPrepare_SandboxMode(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const cause = "bwrap: setting up uid map: Permission denied"
	tests := []struct {
		name string
		mode string
		// probeErr is the probe's answer; nil means the sandbox starts.
		probeErr error
		// wantProbe reports whether the launch probes the sandbox.
		wantProbe bool
		// wantMode is the sandbox the launch line carries.
		wantMode string
		// wantWarn is a text the warnings must hold; empty means none.
		wantWarn string
		// wantErr holds the texts a refusal must carry; empty means the launch goes ahead.
		wantErr []string
	}{
		{name: "AutoSandboxStarts", mode: sandboxAuto, wantProbe: true, wantMode: sandboxWorkspaceWrite},
		{name: "AutoSandboxCannotStart", mode: sandboxAuto, probeErr: errors.New(cause), wantProbe: true, wantMode: sandboxFullAccess, wantWarn: cause},
		{name: "WorkspaceWriteSandboxStarts", mode: sandboxWorkspaceWrite, wantProbe: true, wantMode: sandboxWorkspaceWrite},
		{
			name: "WorkspaceWriteSandboxCannotStart", mode: sandboxWorkspaceWrite, probeErr: errors.New(cause), wantProbe: true,
			wantErr: []string{"codexengine:", cause, "way forward", "lym config codex --layer machine --set sandbox_mode=" + sandboxAuto},
		},
		{
			name: "AutoProbeDidNotRun", mode: sandboxAuto, probeErr: fmt.Errorf("%w: %s", errSandboxProbeNotRun, cause), wantProbe: true,
			wantErr: []string{"codexengine:", cause, "way forward"},
		},
		{
			name: "WorkspaceWriteProbeDidNotRun", mode: sandboxWorkspaceWrite, probeErr: fmt.Errorf("%w: %s", errSandboxProbeNotRun, cause), wantProbe: true,
			wantErr: []string{"codexengine:", cause, "way forward"},
		},
		{name: "FullAccess", mode: sandboxFullAccess, probeErr: errors.New(cause), wantMode: sandboxFullAccess},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newRepoFixture(t)
			anchor, _ := repo.addWorktree(t, "wt")
			writeFile(t, configengine.ConfigFile(anchor, "codex"), "writable_roots: []\nsandbox_mode: "+tt.mode+"\n")
			engine, warnings := newEngine(anchor)
			var probed []string
			engine.sandboxProbe = func(bin, home, dir string) error {
				probed = append(probed, bin, home, dir)
				return tt.probeErr
			}
			runDir := t.TempDir()
			promptPath := filepath.Join(runDir, "prompt.md")

			launch, err := engine.Prepare(runDir, shuttleengine.Spec{Prompt: "p"}, shuttleengine.Config{})

			var wantProbed []string
			if tt.wantProbe {
				wantProbed = []string{"codex", repo.lymHome, anchor}
			}
			if !reflect.DeepEqual(probed, wantProbed) {
				t.Errorf("sandbox probe called with %q; want %q", probed, wantProbed)
			}
			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatal("Prepare() error = nil; want a refusal")
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("Prepare() error %q does not contain %q", err, want)
					}
				}
				if _, statErr := os.Stat(promptPath); !errors.Is(statErr, os.ErrNotExist) {
					t.Errorf("prompt.md stat error = %v; want no prompt file", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}
			eventsPath := filepath.Join(runDir, "events.jsonl")
			want := buildLaunchCmd(shell.ForGOOS(), launchLine{
				bin:           "codex",
				home:          repo.lymHome,
				sandboxMode:   tt.wantMode,
				writableRoots: repo.wantGitDirs(anchor),
				hooks:         hookSettings(eventsPath, installedGuards(nil, guardInputs{}), creelDelivery{}),
				firstTurn:     launchPointer(promptPath),
				eventsPath:    eventsPath,
			})
			if launch.Cmd != want {
				t.Errorf("Prepare() launch line = %q; want %q", launch.Cmd, want)
			}
			if got := warnings.String(); tt.wantWarn == "" && got != "" || !strings.Contains(got, tt.wantWarn) {
				t.Errorf("warnings = %q; want %q", got, tt.wantWarn)
			}
		})
	}
}

// TestPrepare_ExpandsHomeInWritableRoots pins that a leading ~/ in writable_roots expands to the user's home.
// It sets the home environment variables, so it does not run in parallel.
func TestPrepare_ExpandsHomeInWritableRoots(t *testing.T) {
	tiergate.Require(t, 0)

	repo := newRepoFixture(t)
	anchor, _ := repo.addWorktree(t, "wt")
	userHome := filepath.Join(repo.base, "user-home")
	cache := filepath.Join(userHome, "cache")
	mkdirAll(t, cache)
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	writeWritableRoots(t, anchor, "~/cache")
	engine, _ := newEngine(anchor)
	runDir := t.TempDir()

	launch, err := engine.Prepare(runDir, shuttleengine.Spec{Prompt: "p"}, shuttleengine.Config{})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}

	promptPath := filepath.Join(runDir, "prompt.md")
	want := buildLaunchCmd(shell.ForGOOS(), launchLine{
		bin:           "codex",
		home:          repo.lymHome,
		sandboxMode:   sandboxWorkspaceWrite,
		writableRoots: append(repo.wantGitDirs(anchor), cache),
		hooks:         hookSettings(filepath.Join(runDir, "events.jsonl"), installedGuards(nil, guardInputs{}), creelDelivery{}),
		firstTurn:     launchPointer(promptPath),
		eventsPath:    filepath.Join(runDir, "events.jsonl"),
	})
	if launch.Cmd != want {
		t.Errorf("Prepare() launch line = %q; want %q", launch.Cmd, want)
	}
}

// TestPrepare_InstallsGuardsFromItsInputs pins which inputs reach the installed guard rows.
// The python switch and the publish guard each install their own row alone;
// a seeded refuse_raw list of the hub's gate config reaches the hook and the prompt file's deny notice;
// and a gate config that is present but does not load refuses the launch before the prompt file is written.
func TestPrepare_InstallsGuardsFromItsInputs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// gate is the gate.yaml text written for the worktree; empty writes no file.
		gate        string
		denyPython  bool
		denyPublish bool
		wantRefuse  []string
		wantInputs  guardInputs
		wantErr     bool
	}{
		{name: "PythonSwitchAlone", denyPython: true, wantInputs: guardInputs{denyPython: true}},
		{name: "PublishGuardAlone", denyPublish: true, wantInputs: guardInputs{denyPublish: true}},
		{name: "SeededRefuseRaw", gate: "slots: 2\nrefuse_raw: [\"go test\", \"dotnet test\"]\n", wantRefuse: []string{"go test", "dotnet test"}},
		{name: "GateConfigThatDoesNotLoad", gate: "slots: [\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newRepoFixture(t)
			anchor, _ := repo.addWorktree(t, "wt")
			if tt.gate != "" {
				gateFile := configengine.ConfigFile(anchor, "gate")
				mkdirAll(t, filepath.Dir(gateFile))
				writeFile(t, gateFile, tt.gate)
			}
			engine, _ := newEngine(anchor)
			runDir := t.TempDir()

			launch, err := engine.Prepare(runDir, shuttleengine.Spec{Prompt: "p", DenyPublish: tt.denyPublish}, shuttleengine.Config{DenyPython: tt.denyPython})
			promptPath := filepath.Join(runDir, "prompt.md")
			if tt.wantErr {
				if err == nil || !strings.HasPrefix(err.Error(), "codexengine:") {
					t.Errorf("Prepare() error = %v; want a refusal beginning codexengine:", err)
				}
				if _, statErr := os.Stat(promptPath); !errors.Is(statErr, os.ErrNotExist) {
					t.Errorf("prompt.md stat error = %v; want no prompt file", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}
			guards := installedGuards(tt.wantRefuse, tt.wantInputs)
			wantCmd := buildLaunchCmd(shell.ForGOOS(), launchLine{
				bin:           "codex",
				home:          repo.lymHome,
				sandboxMode:   sandboxWorkspaceWrite,
				writableRoots: repo.wantGitDirs(anchor),
				hooks:         hookSettings(filepath.Join(runDir, "events.jsonl"), guards, creelDelivery{}),
				firstTurn:     launchPointer(promptPath),
				eventsPath:    filepath.Join(runDir, "events.jsonl"),
			})
			if launch.Cmd != wantCmd {
				t.Errorf("Prepare() launch line = %q; want %q", launch.Cmd, wantCmd)
			}
			if got, err := os.ReadFile(promptPath); err != nil || string(got) != denyNotice(guards)+" "+shuttleengine.CreelTrustSentence+"\n\np" {
				t.Errorf("prompt.md = %q, %v; want the deny notice of the installed rows and the prompt", got, err)
			}
		})
	}
}

// TestPrepare_AppendsStandingPassages pins that the prompt file opens with the deny notice;
// the passages the reader finds for engine codex and the spec's role follow, each before a blank line.
// A told board hides the local layer of a worktree with no `_lym` folder;
// its base layer names the fixture's homes.
// The test also checks that the launch readied the fixture's lym home, so it never reaches the operator's.
// It installs the process-wide passage reader, so it does not run in parallel.
func TestPrepare_AppendsStandingPassages(t *testing.T) {
	tiergate.Require(t, 0)

	repo := newRepoFixture(t)
	anchor, _ := repo.addWorktree(t, "wt")
	engine, _ := newEngine(anchor)
	board := t.TempDir()
	repo.writeCodexConfig(t, configengine.ConfigFile(board, "codex"), "")
	engine.dirs.Board = board
	var asked []string
	restore := shuttleengine.InstallPassageReader(func(boltDir, engine, role string) ([]string, error) {
		asked = append(asked, engine, role)
		return []string{"first passage", "second passage"}, nil
	})
	t.Cleanup(restore)

	runDir := t.TempDir()
	if _, err := engine.Prepare(runDir, shuttleengine.Spec{Prompt: "the prompt", Role: "code-review-assist"}, shuttleengine.Config{}); err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(runDir, "prompt.md"))
	if want := denyNotice(installedGuards(nil, guardInputs{})) + " " + shuttleengine.CreelTrustSentence + "\n\nfirst passage\n\nsecond passage\n\nthe prompt"; err != nil || string(got) != want {
		t.Errorf("prompt.md = %q, %v; want %q", got, err, want)
	}
	if want := []string{"codex", "code-review-assist"}; !reflect.DeepEqual(asked, want) {
		t.Errorf("passage reader asked for %v; want %v", asked, want)
	}
	if got, want := readRecord(t, repo.lymHome).Roots, []string{repo.mainRoot, anchor}; !reflect.DeepEqual(got, want) {
		t.Errorf("fixture lym home trust roots = %v; want %v", got, want)
	}
}

// TestPrepare_CreelHooksInAHub pins that an engine told a board directory marks the launch CreelHooks.
// The launch carries the creel Stop and PostToolUse settings over that hub's flag directory.
// The idle waiter is set to queue under the launch's lym home and the Codex executable codex.yaml names.
func TestPrepare_CreelHooksInAHub(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	repo := newRepoFixture(t)
	anchor, _ := repo.addWorktree(t, "wt")
	engine, _ := newEngine(anchor)
	board := t.TempDir()
	repo.writeCodexConfig(t, configengine.ConfigFile(board, "codex"), "binary: /opt/codex/bin/codex-custom\n")
	engine.dirs.Board = board
	runDir := t.TempDir()

	launch, err := engine.Prepare(runDir, shuttleengine.Spec{Prompt: "p"}, shuttleengine.Config{})
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}
	if !launch.CreelHooks {
		t.Error("Prepare() CreelHooks = false; want true for an engine told a board")
	}
	creel := creelDelivery{flagDir: shuttleengine.CreelFlagDir(board), home: repo.lymHome, bin: "/opt/codex/bin/codex-custom"}
	settings := hookSettings(filepath.Join(runDir, "events.jsonl"), installedGuards(nil, guardInputs{}), creel)
	for _, setting := range settings {
		if strings.HasPrefix(setting, "hooks."+hookEventStop+"=") || strings.HasPrefix(setting, "hooks."+hookEventPostToolUse+"=") {
			if quoted := shell.ForGOOS().Quote(setting); !strings.Contains(launch.Cmd, quoted) {
				t.Errorf("Prepare() launch line = %q; want it to hold the creel setting %s", launch.Cmd, quoted)
			}
		}
	}
}

// TestPrepare_ReadiesHome pins the lym home a launch leaves.
// It checks that auth.json and sessions link to the operator home's, and that the worktree root is trusted.
// A linked worktree also trusts the main checkout root, and a plain clone trusts only its own root.
// It drops a vanished root, and it renders config.toml from the record with both [tui] settings.
func TestPrepare_ReadiesHome(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// build adds the checkout to the fixture and returns its root and the roots the home must trust.
		build func(t *testing.T, repo repoFixture) (anchor string, wantRoots []string)
	}{
		{
			name: "LinkedWorktree",
			build: func(t *testing.T, repo repoFixture) (string, []string) {
				anchor, _ := repo.addWorktree(t, "wt")
				return anchor, []string{repo.mainRoot, anchor}
			},
		},
		{
			name: "PlainClone",
			build: func(t *testing.T, repo repoFixture) (string, []string) {
				anchor := repo.addClone(t, "clone")
				return anchor, []string{anchor}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newRepoFixture(t)
			anchor, wantRoots := tt.build(t, repo)
			vanished := filepath.Join(repo.base, "gone")
			mkdirAll(t, repo.lymHome)
			writeFile(t, filepath.Join(repo.lymHome, trustRecordFileName), `{"roots":["`+vanished+`"],"kept_key":1}`)
			engine, _ := newEngine(anchor)

			if _, err := engine.Prepare(t.TempDir(), shuttleengine.Spec{Prompt: "p"}, shuttleengine.Config{}); err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}

			for name, target := range map[string]string{authFileName: filepath.Join(repo.operatorHome, authFileName), sessionsDirName: filepath.Join(repo.operatorHome, sessionsDirName)} {
				if got, err := os.Readlink(filepath.Join(repo.lymHome, name)); err != nil || got != target {
					t.Errorf("Readlink(%s) = %q, %v; want %q", name, got, err, target)
				}
			}
			if got := readRecord(t, repo.lymHome).Roots; !reflect.DeepEqual(got, wantRoots) {
				t.Errorf("trust record roots = %v; want %v", got, wantRoots)
			}
			if raw, _ := os.ReadFile(filepath.Join(repo.lymHome, trustRecordFileName)); !strings.Contains(string(raw), "kept_key") {
				t.Errorf("trust record = %s; want the unknown key kept", raw)
			}
			got, err := os.ReadFile(filepath.Join(repo.lymHome, configFileName))
			if err != nil {
				t.Fatalf("read config.toml: %v", err)
			}
			if want := renderConfigTOML(wantRoots); string(got) != want {
				t.Errorf("config.toml = %q; want %q", got, want)
			}
		})
	}
}

// TestRenderConfigTOML pins the rendered config.toml:
// the header, both [tui] settings, then one trusted table per root with the path TOML-quoted.
func TestRenderConfigTOML(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	got := renderConfigTOML([]string{`/a/b`, `/c/"d"`})
	want := configHeader + "\n[tui]\nscreen_reader_detection_done = true\n\n[tui.model_availability_nux]\n" +
		"\n[projects.\"/a/b\"]\ntrust_level = \"trusted\"\n" +
		"\n[projects.\"/c/\\\"d\\\"\"]\ntrust_level = \"trusted\"\n"
	if got != want {
		t.Errorf("renderConfigTOML() = %q; want %q", got, want)
	}
}

// TestPrepare_ConcurrentLaunchesLoseNoRoot pins that concurrent launches into one lym home keep every root.
// Every root must end up in both the record and config.toml.
func TestPrepare_ConcurrentLaunchesLoseNoRoot(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	repo := newRepoFixture(t)
	var anchors []string
	for i := range 6 {
		anchor, _ := repo.addWorktree(t, fmt.Sprintf("wt%d", i))
		anchors = append(anchors, anchor)
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(anchors))
	for _, anchor := range anchors {
		wg.Add(1)
		go func() {
			defer wg.Done()
			engine, _ := newEngine(anchor)
			_, err := engine.Prepare(t.TempDir(), shuttleengine.Spec{Prompt: "p"}, shuttleengine.Config{})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Prepare() error: %v", err)
		}
	}
	record := readRecord(t, repo.lymHome)
	config, err := os.ReadFile(filepath.Join(repo.lymHome, configFileName))
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	for _, root := range append(anchors, repo.mainRoot) {
		if !strings.Contains(strings.Join(record.Roots, "\n"), root) || !strings.Contains(string(config), projectTableHeader(root)) {
			t.Errorf("root %s missing: record %v, config %q", root, record.Roots, config)
		}
	}
}

// TestPrepare_RefusesBeforeWriting pins each refusal;
// a refused spec writes neither the prompt file nor anything in the lym home.
func TestPrepare_RefusesBeforeWriting(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		spec shuttleengine.Spec
		// anchorRel is the folder of the worktree the engine is anchored at; empty means the worktree root.
		anchorRel string
		// setup changes the worktree before Prepare.
		setup   func(t *testing.T, anchor string)
		wantErr []string
		// wantWarn is a text the warnings must hold; empty means none.
		wantWarn string
	}{
		{name: "VersionPin", spec: shuttleengine.Spec{ModelChoice: shuttleengine.ModelChoice{Model: "astra", Version: "6"}}, wantErr: []string{`version "6"`, "way forward"}},
		{name: "PromptPermissionMode", spec: shuttleengine.Spec{PermissionMode: "prompt"}, wantErr: []string{`permission mode "prompt"`, "way forward"}},
		{name: "ResumedSession", spec: shuttleengine.Spec{ResumeSessionID: "abc"}, wantErr: []string{"cannot be resumed", "way forward"}},
		{name: "EffortOutsideEfforts", spec: shuttleengine.Spec{ModelChoice: shuttleengine.ModelChoice{Effort: "xhigh"}}, wantErr: []string{`effort "xhigh"`, "codex.yaml's efforts", "way forward"}},
		{
			name: "RelativeWritableRoot",
			setup: func(t *testing.T, anchor string) {
				writeWritableRoots(t, anchor, "cache")
			},
			wantErr: []string{`codex.yaml's writable_roots "cache"`, "way forward"},
		},
		{
			name: "UnknownSandboxMode",
			setup: func(t *testing.T, anchor string) {
				writeFile(t, configengine.ConfigFile(anchor, "codex"), "writable_roots: []\nsandbox_mode: read-only\n")
			},
			wantErr: []string{`codex.yaml's sandbox_mode "read-only"`, "way forward", "--layer <default|worktree|machine|local>"},
		},
		{
			name: "RulesInWorktree",
			setup: func(t *testing.T, anchor string) {
				mkdirAll(t, filepath.Join(anchor, codexFolderName, rulesDirName))
			},
			wantErr:  []string{filepath.Join(codexFolderName, rulesDirName), "way forward", "remove the directory", "launch on an engine other than codex"},
			wantWarn: filepath.Join(codexFolderName, rulesDirName),
		},
		{
			name:      "RulesUnderSubfolderAnchor",
			anchorRel: "sub",
			setup: func(t *testing.T, anchor string) {
				mkdirAll(t, filepath.Join(anchor, "sub", codexFolderName, rulesDirName))
			},
			wantErr:  []string{filepath.Join("sub", codexFolderName, rulesDirName), "way forward", "remove the directory"},
			wantWarn: filepath.Join("sub", codexFolderName, rulesDirName),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newRepoFixture(t)
			anchor, _ := repo.addWorktree(t, "wt")
			if tt.setup != nil {
				tt.setup(t, anchor)
			}
			engineAnchor := filepath.Join(anchor, tt.anchorRel)
			mkdirAll(t, engineAnchor)
			repo.writeCodexLayer(t, engineAnchor, "")
			engine, warnings := newEngine(engineAnchor)
			runDir := t.TempDir()
			spec := tt.spec
			spec.Prompt = "p"

			_, err := engine.Prepare(runDir, spec, shuttleengine.Config{})
			if err == nil {
				t.Fatal("Prepare() error = nil; want a refusal")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Prepare() error %q does not contain %q", err, want)
				}
			}
			if _, statErr := os.Stat(filepath.Join(runDir, "prompt.md")); !errors.Is(statErr, os.ErrNotExist) {
				t.Errorf("prompt.md stat error = %v; want no prompt file", statErr)
			}
			if _, statErr := os.Lstat(repo.lymHome); !errors.Is(statErr, os.ErrNotExist) {
				t.Errorf("lym home stat error = %v; want the home unmade", statErr)
			}
			if got := warnings.String(); tt.wantWarn == "" && got != "" || !strings.Contains(got, tt.wantWarn) {
				t.Errorf("warnings = %q; want %q", got, tt.wantWarn)
			}
		})
	}
}

// TestPrepare_Warnings pins the warnings a launch goes on despite:
// a worktree config, the main checkout's config behind a worktree's `.codex` folder,
// and a home auth.json or sessions that is a regular file or a link elsewhere, which Prepare keeps.
func TestPrepare_Warnings(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		// setup changes the fixture before Prepare and returns the texts the warnings must hold.
		setup func(t *testing.T, repo repoFixture, anchor string) []string
	}{
		{
			name: "WorktreeConfig",
			setup: func(t *testing.T, _ repoFixture, anchor string) []string {
				path := filepath.Join(anchor, codexFolderName, configFileName)
				mkdirAll(t, filepath.Dir(path))
				writeFile(t, path, "")
				return []string{path}
			},
		},
		{
			name: "MainCheckoutConfigBehindWorktreeFolder",
			setup: func(t *testing.T, repo repoFixture, anchor string) []string {
				mkdirAll(t, filepath.Join(anchor, codexFolderName))
				path := filepath.Join(repo.mainRoot, codexFolderName, configFileName)
				mkdirAll(t, filepath.Dir(path))
				writeFile(t, path, "")
				return []string{path}
			},
		},
		{
			name: "RegularAuthFile",
			setup: func(t *testing.T, repo repoFixture, _ string) []string {
				path := filepath.Join(repo.lymHome, authFileName)
				mkdirAll(t, repo.lymHome)
				writeFile(t, path, "{}")
				return []string{path, "way forward: remove it"}
			},
		},
		{
			name: "SessionsLinkedElsewhere",
			setup: func(t *testing.T, repo repoFixture, _ string) []string {
				path := filepath.Join(repo.lymHome, sessionsDirName)
				mkdirAll(t, repo.lymHome)
				if err := os.Symlink(repo.base, path); err != nil {
					t.Fatalf("symlink: %v", err)
				}
				return []string{path, "way forward: remove it"}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newRepoFixture(t)
			anchor, _ := repo.addWorktree(t, "wt")
			wants := tt.setup(t, repo, anchor)
			engine, warnings := newEngine(anchor)

			if _, err := engine.Prepare(t.TempDir(), shuttleengine.Spec{Prompt: "p"}, shuttleengine.Config{}); err != nil {
				t.Fatalf("Prepare() error: %v", err)
			}
			for _, want := range wants {
				if !strings.Contains(warnings.String(), want) {
					t.Errorf("warnings = %q; want them to hold %q", warnings, want)
				}
			}
		})
	}
}

// TestCheckAfterStartup pins that the check warns about each trust root config.toml lost, naming it;
// it stays silent while every root is there.
func TestCheckAfterStartup(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	repo := newRepoFixture(t)
	anchor, _ := repo.addWorktree(t, "wt")
	engine, warnings := newEngine(anchor)
	if _, err := engine.Prepare(t.TempDir(), shuttleengine.Spec{Prompt: "p"}, shuttleengine.Config{}); err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}

	engine.CheckAfterStartup()
	if got := warnings.String(); got != "" {
		t.Errorf("warnings with every root kept = %q; want none", got)
	}

	writeFile(t, filepath.Join(repo.lymHome, configFileName), renderConfigTOML([]string{repo.mainRoot}))
	engine.CheckAfterStartup()
	if got := warnings.String(); !strings.Contains(got, "root "+anchor) || strings.Contains(got, "root "+repo.mainRoot) {
		t.Errorf("warnings after the worktree root was dropped = %q; want one naming %s alone", got, anchor)
	}
}

// TestLoadConfig_Template pins codex.yaml's template:
// the codex binary on PATH, the homes under ~/, the efforts low to high;
// the Claude plugin cache as the skill root, the auto sandbox mode and the Go build cache as the writable root.
func TestLoadConfig_Template(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	got, err := LoadConfig(configengine.Dirs{Anchor: t.TempDir()})
	if err != nil {
		t.Fatalf("LoadConfig() error: %v", err)
	}
	want := Config{Home: "~/.lym/codex", OperatorHome: "~/.codex", Efforts: []string{"low", "medium", "high"}, SkillRoots: []string{"~/.claude/plugins/cache"}, SandboxMode: sandboxAuto, WritableRoots: []string{"~/.cache/go-build"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LoadConfig() = %+v; want %+v", got, want)
	}
	if bin := codexBinary(got); bin != "codex" {
		t.Errorf("codexBinary() = %q; want codex", bin)
	}
}

// TestLoadConfig_SandboxModeLayers pins which layers set sandbox_mode in a hub.
// The machine override wins over the worktree file.
// So one machine runs unsandboxed while the tracked files keep the sandbox.
// A local override that sets it refuses the load.
func TestLoadConfig_SandboxModeLayers(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name     string
		worktree string
		machine  string
		local    string
		want     string
		// wantErr is a text the load error must hold; empty means the load succeeds.
		wantErr string
	}{
		{name: "MachineOverWorktree", worktree: sandboxWorkspaceWrite, machine: sandboxFullAccess, want: sandboxFullAccess},
		{name: "WorktreeWithNoMachineFile", worktree: sandboxFullAccess, want: sandboxFullAccess},
		{name: "LocalRefused", local: sandboxFullAccess, wantErr: `"sandbox_mode"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dirs := configengine.Dirs{Anchor: t.TempDir(), Board: t.TempDir()}
			base := configengine.ConfigFile(dirs.Board, "codex")
			mkdirAll(t, filepath.Dir(base), filepath.Dir(configengine.ConfigFile(dirs.Anchor, "codex")))
			writeFile(t, base, configengine.SparseBaseHeader)
			layers := []struct{ path, mode string }{
				{configengine.ConfigFile(dirs.Anchor, "codex"), tt.worktree},
				{configengine.MachineFile(dirs.Board, "codex"), tt.machine},
				{configengine.LocalFile(dirs.Anchor, "codex"), tt.local},
			}
			for _, layer := range layers {
				if layer.mode != "" {
					mkdirAll(t, filepath.Dir(layer.path))
					writeFile(t, layer.path, "sandbox_mode: "+layer.mode+"\n")
				}
			}

			got, err := LoadConfig(dirs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("LoadConfig() error = %v; want one naming %s", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() error: %v", err)
			}
			if got.SandboxMode != tt.want {
				t.Errorf("LoadConfig().SandboxMode = %q; want %q", got.SandboxMode, tt.want)
			}
		})
	}
}

// TestExpandHomePath pins the home keys' resolution:
// a leading ~/ under the user's home, an absolute path kept, and an empty or relative value refused by its key.
func TestExpandHomePath(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	absolute := filepath.Join(t.TempDir(), "home")
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "UnderUserHome", value: "~/.lym/codex", want: filepath.Join(userHome, ".lym", "codex")},
		{name: "Absolute", value: absolute, want: absolute},
		{name: "Empty", value: "", wantErr: true},
		{name: "Relative", value: "codex-home", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := expandHomePath("home", tt.value)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "codex.yaml's home") {
					t.Errorf("expandHomePath(%q) error = %v; want a refusal naming the key", tt.value, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("expandHomePath(%q) = %q, %v; want %q", tt.value, got, err, tt.want)
			}
		})
	}
}
