// usersetup_test.go covers how Prepare carries the user's Claude setup into a launch.
// A fake source stands in for the setup, so no test reads a Claude config directory.

package codexengine

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shell"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// fakeUserSetupSource answers a fixed setup or error and records the request it was asked.
type fakeUserSetupSource struct {
	setup shuttleengine.UserSetup
	err   error
	got   shuttleengine.UserSetupRequest
}

// UserSetup records req and returns the fixed answer.
func (f *fakeUserSetupSource) UserSetup(req shuttleengine.UserSetupRequest) (shuttleengine.UserSetup, error) {
	f.got = req
	return f.setup, f.err
}

// TestPrepare_UserSetup pins how the user's Claude setup reaches a launch.
// The request carries the launch's facts, and the items follow the creel head in the source's order.
// A source warning and an omitted hook reach the warnings, and a source error refuses the launch.
// An item that takes the setting over the argument cap refuses it, naming the item and its way forward.
func TestPrepare_UserSetup(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const global = "/home/user/.claude/CLAUDE.md"
	tests := []struct {
		name  string
		setup shuttleengine.UserSetup
		err   error
		// wantErr holds the texts a refusal carries; empty means the launch goes ahead.
		wantErr []string
		// wantWarn holds the texts the warnings carry.
		wantWarn []string
		// wantSilent requires that nothing is warned.
		wantSilent bool
		// throughLink reaches the worktree through a symlink to it, so the engine's anchor is not its resolved path.
		throughLink bool
	}{
		{name: "AnchorThroughALinkGivesTheResolvedWorkDir", throughLink: true},
		{
			name: "ItemsFollowTheCreelHeadInOrder",
			setup: shuttleengine.UserSetup{
				InstructionFiles: []shuttleengine.InstructionItem{{Name: global, Text: "global rules", WayForward: "shorten " + global}},
				SkillListing:     "listing of skills",
				PluginContexts:   []shuttleengine.InstructionItem{{Name: "plugin p@m SessionStart hook", Text: "plugin context", WayForward: "turn the plugin off"}},
			},
		},
		{
			name: "OmittedHookAndWarningReachTheWarnings",
			setup: shuttleengine.UserSetup{
				Warnings: []string{"a source warning"},
				Omitted:  []shuttleengine.OmittedHook{{Name: "plugin p@m SessionStart hook run.sh", Reason: "exit status 3"}},
			},
			wantWarn: []string{"a source warning", "plugin p@m SessionStart hook run.sh", "exit status 3", "session scope"},
		},
		{
			name: "UnrunHookWarnsOfNothing",
			setup: shuttleengine.UserSetup{
				Omitted: []shuttleengine.OmittedHook{{Name: "plugin p@m SessionStart hook run.sh", Reason: shuttleengine.ErrHookUnrun.Error()}},
			},
			wantSilent: true,
		},
		{
			name:    "SourceErrorRefuses",
			err:     errors.New("claudeengine: the global file is unreadable; way forward: fix its permissions"),
			wantErr: []string{"codexengine:", "the global file is unreadable", "way forward: fix its permissions"},
		},
		{
			name: "GlobalFileOverTheCapRefuses",
			setup: shuttleengine.UserSetup{
				InstructionFiles: []shuttleengine.InstructionItem{{Name: global, Text: strings.Repeat("a", maxArgumentBytes), WayForward: "shorten " + global}},
			},
			wantErr: []string{global, "way forward: shorten " + global},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newRepoFixture(t)
			anchor, _ := repo.addWorktree(t, "wt")
			engineAnchor := anchor
			if tt.throughLink {
				engineAnchor = filepath.Join(t.TempDir(), "anchor-link")
				if err := os.Symlink(anchor, engineAnchor); err != nil {
					t.Fatalf("symlink: %v", err)
				}
			}
			engine, warnings := newEngine(engineAnchor)
			source := &fakeUserSetupSource{setup: tt.setup, err: tt.err}
			engine.userSetup = source
			engine.runHook = func(shuttleengine.HookRun) ([]byte, error) { return nil, nil }
			runDir := t.TempDir()
			spec := shuttleengine.Spec{Prompt: "p", PromptEngine: engineName, Role: "darn", Skills: []string{"scribe:prose"}}

			launch, err := engine.Prepare(runDir, spec, shuttleengine.Config{})

			got := source.got
			if got.RunHook == nil {
				t.Error("request RunHook = nil; want the engine's runner")
			}
			got.RunHook = nil
			wantRequest := shuttleengine.UserSetupRequest{
				WorkDir: anchor, RepoRoot: anchor, Skills: spec.Skills, Role: spec.Role,
				SessionID: filepath.Base(runDir), HookTimeout: 10 * time.Second,
			}
			if !reflect.DeepEqual(got, wantRequest) {
				t.Errorf("source request = %+v; want %+v", got, wantRequest)
			}
			for _, want := range tt.wantWarn {
				if !strings.Contains(warnings.String(), want) {
					t.Errorf("warnings = %q; want them to hold %q", warnings, want)
				}
			}
			if tt.wantSilent && warnings.String() != "" {
				t.Errorf("warnings = %q; want none", warnings)
			}
			promptPath := filepath.Join(runDir, "prompt.md")
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
			texts := []string{wantCreelHead(installedGuards(nil, guardInputs{}))}
			for _, item := range append(tt.setup.InstructionFiles, tt.setup.PluginContexts...) {
				texts = append(texts, item.Text)
			}
			if tt.setup.SkillListing != "" {
				// The listing sits between the instruction files and the plugin contexts.
				texts = append(texts[:len(tt.setup.InstructionFiles)+1], append([]string{tt.setup.SkillListing}, texts[len(tt.setup.InstructionFiles)+1:]...)...)
			}
			setting := developerInstructionsSetting(strings.Join(texts, "\n\n"))
			if want := shell.ForGOOS().Quote(setting); !strings.Contains(launch.Cmd, want) {
				t.Errorf("Prepare() launch line = %q; want it to hold the setting %s", launch.Cmd, want)
			}
		})
	}
}
