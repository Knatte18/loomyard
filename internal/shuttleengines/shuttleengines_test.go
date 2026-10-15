// shuttleengines_test.go verifies the registry builds a set from a shuttle config, refuses an unregistered engine from any layer, and stays equal to modelspec's closed engine names.

package shuttleengines

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/modelspec"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine/claudeengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/tmuxkit"
)

// forksWithoutAuditEngine declares RequirementForks through Supports but has no ForkAuditor.
type forksWithoutAuditEngine struct{ shuttleengine.Engine }

func (forksWithoutAuditEngine) Supports(req shuttleengine.Requirement) bool {
	return req == shuttleengine.RequirementForks
}

// writeLocalLayer writes content as module's local layer file under anchor.
func writeLocalLayer(t *testing.T, anchor, module, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(anchor, lyxdirs.LyxDirName), 0o755); err != nil {
		t.Fatalf("mkdir _lyx: %v", err)
	}
	localFile := configengine.LocalFile(anchor, module)
	if err := os.MkdirAll(filepath.Dir(localFile), 0o755); err != nil {
		t.Fatalf("mkdir local dir: %v", err)
	}
	if err := os.WriteFile(localFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write local file: %v", err)
	}
}

// loadWithLocalEngine loads a shuttle config over a temp anchor whose local layer file sets engine: bogus.
func loadWithLocalEngine(t *testing.T) shuttleengine.Config {
	t.Helper()
	anchor := t.TempDir()
	writeLocalLayer(t, anchor, "shuttle", "engine: bogus\n")
	cfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: anchor}, "shuttle")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg
}

// invalidClaudeConfigFixture builds a temp anchor whose claude.local.yaml holds a key the local layer refuses.
// The anchor's shuttle.local.yaml holds two prompt-cache roles, and the fixture returns the shuttle config loaded over it.
func invalidClaudeConfigFixture(t *testing.T) (shuttleengine.Config, configengine.Dirs) {
	t.Helper()
	anchor := t.TempDir()
	writeLocalLayer(t, anchor, "claude", "binary: /x\n")
	writeLocalLayer(t, anchor, "shuttle", "claude_prompt_cache_ttl_roles:\n  alpha: 5m\n  beta: 1h\n")
	dirs := configengine.Dirs{Anchor: anchor}
	cfg, err := shuttleengine.LoadConfig(dirs, "shuttle")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg, dirs
}

// assertBadClaudeConfigRefusesOnlyARunStart pins that a claude.yaml the local layer refuses stops a run start.
// Stopping a strand and reading activity, waits and session states still work.
func assertBadClaudeConfigRefusesOnlyARunStart(t *testing.T, set shuttleengine.EngineSet, cfg shuttleengine.Config, dirs configengine.Dirs) {
	t.Helper()
	reed := &shuttlefake.Reed{Strands: []reedengine.StrandStatus{{GUID: "live-guid", Name: "alpha", Live: true}}}
	runner := shuttleengine.NewRunner(reed, set, dirs.Anchor, dirs.Anchor, cfg)

	if err := runner.StopStrand("live-guid"); err != nil {
		t.Fatalf("StopStrand: %v", err)
	}
	if got, want := reed.RemovedGUIDs, []string{"live-guid"}; !reflect.DeepEqual(got, want) {
		t.Errorf("RemovedGUIDs = %v; want %v", got, want)
	}
	if _, err := shuttleengine.ReadAgentActivity(cfg, dirs.Anchor, set); err != nil {
		t.Errorf("ReadAgentActivity: %v", err)
	}
	if _, err := shuttleengine.ReadAgentWaits(cfg, dirs.Anchor, set, time.Now()); err != nil {
		t.Errorf("ReadAgentWaits: %v", err)
	}
	if _, err := shuttleengine.ReadSessionStates(cfg, dirs.Anchor, set, time.Now()); err != nil {
		t.Errorf("ReadSessionStates: %v", err)
	}

	spec := shuttleengine.Spec{Prompt: "p", OutputFiles: []string{filepath.Join(dirs.Anchor, "out.md")}}
	_, err := runner.Start(spec)
	if err == nil {
		t.Fatal("Start accepted a run over an invalid claude.yaml")
	}
	for _, want := range []string{"claude.local.yaml", "binary"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Start error %q does not contain %q", err, want)
		}
	}
}

// TestNew pins the set the template config builds and the refusal of an engine name no layer may smuggle in.
func TestNew(t *testing.T) {
	t.Parallel()

	templateCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: t.TempDir()}, "shuttle")
	if err != nil {
		t.Fatalf("LoadConfig over the template: %v", err)
	}

	forksWithoutAudit := map[string]constructor{
		"nofork": func(shuttleengine.Config, configengine.Dirs) shuttleengine.Engine { return forksWithoutAuditEngine{} },
	}
	unregistered := []string{"bogus", "registered engines: claude", "engine: in shuttle.yaml"}
	invalidClaudeCfg, invalidClaudeDirs := invalidClaudeConfigFixture(t)
	tests := []struct {
		name         string
		cfg          shuttleengine.Config
		dirs         configengine.Dirs
		constructors map[string]constructor
		wantErr      []string
		// after, when set, runs against the built set.
		after func(t *testing.T, set shuttleengine.EngineSet)
	}{
		{name: "TemplateConfig", cfg: templateCfg},
		{name: "InvalidEngineConfig", cfg: invalidClaudeCfg, dirs: invalidClaudeDirs, after: func(t *testing.T, set shuttleengine.EngineSet) {
			assertBadClaudeConfigRefusesOnlyARunStart(t, set, invalidClaudeCfg, invalidClaudeDirs)
		}},
		{name: "UnregisteredEngine", cfg: shuttleengine.Config{Engine: "bogus"}, wantErr: unregistered},
		{name: "UnregisteredEngineFromLocalLayer", cfg: loadWithLocalEngine(t), wantErr: unregistered},
		{name: "EngineDeclaringForksWithoutAudit", cfg: shuttleengine.Config{Engine: "nofork"}, constructors: forksWithoutAudit, wantErr: []string{`engine "nofork" declares forks but cannot audit them`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var set shuttleengine.EngineSet
			var err error
			if tt.constructors != nil {
				set, err = build(tt.cfg, tt.dirs, tt.constructors)
			} else {
				set, err = New(tt.cfg, tt.dirs)
			}
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("New(%q) accepted a set it must refuse", tt.cfg.Engine)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("New error %q does not contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			engine, err := set.Lookup("")
			if err != nil {
				t.Fatalf("Lookup of the default: %v", err)
			}
			if _, ok := engine.(*claudeengine.Claude); !ok {
				t.Errorf("default engine = %T; want *claudeengine.Claude", engine)
			}
			if got, want := set.Names(), []string{"claude"}; !reflect.DeepEqual(got, want) {
				t.Errorf("Names() = %v; want %v", got, want)
			}
			if tt.after != nil {
				tt.after(t, set)
			}
		})
	}
}

// TestNames_EqualModelspecEngineNames pins that the registry and modelspec's closed set name the same engines, and that modelspec hands out a copy.
func TestNames_EqualModelspecEngineNames(t *testing.T) {
	t.Parallel()

	if got, want := Names(), modelspec.EngineNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %v; want modelspec.EngineNames() = %v", got, want)
	}

	// A caller overwriting the slice it was handed must not change the closed set.
	modelspec.EngineNames()[0] = "overwritten"
	if got, want := Names(), modelspec.EngineNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("after overwriting a returned slice, Names() = %v; want modelspec.EngineNames() = %v", got, want)
	}
}

// TestEnvScrub_MatchesTmuxkitServerEnviron pins the registered engines' scrub against the kit's server-environment filter, and the union over several entries as sorted and deduplicated.
func TestEnvScrub_MatchesTmuxkitServerEnviron(t *testing.T) {
	t.Parallel()

	environ := []string{
		"HOME=/home/x", "CLAUDECODE=1", "CLAUDE_CODE_ENTRYPOINT=cli", "CLAUDE_CODEX=keep",
		"LYX_TRACE_ID=abc", "LYX_STRAND_NAME=ab:orch", "LYX_PARENT=ab:driver", "LYX_OTHER=keep", "PATH=/bin",
	}

	clean, _ := EnvScrub().Apply(environ)
	var want []string
	for _, entry := range clean {
		switch strings.SplitN(entry, "=", 2)[0] {
		case "LYX_TRACE_ID", "LYX_STRAND_NAME", "LYX_PARENT":
		default:
			want = append(want, entry)
		}
	}
	if got := tmuxkit.ServerEnviron(environ); !reflect.DeepEqual(got, want) {
		t.Errorf("tmuxkit.ServerEnviron = %q; want %q", got, want)
	}

	union := scrubOf(map[string]entry{
		"one": {scrub: reedengine.EnvScrub{Names: []string{"B", "A"}, Prefixes: []string{"Y_", "X_"}}},
		"two": {scrub: reedengine.EnvScrub{Names: []string{"B", "C"}, Prefixes: []string{"X_"}}},
	})
	wantUnion := reedengine.EnvScrub{Names: []string{"A", "B", "C"}, Prefixes: []string{"X_", "Y_"}}
	if !reflect.DeepEqual(union, wantUnion) {
		t.Errorf("scrubOf = %+v; want %+v", union, wantUnion)
	}
}
