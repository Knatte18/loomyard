// config_test.go covers LoadConfig over a bare temp directory that holds a hardener.yaml and nothing else.

package hardenshed

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/darnengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// seedHardenerConfig writes contents as the hub file of a fresh board directory and returns the directory.
func seedHardenerConfig(t *testing.T, contents string) string {
	t.Helper()
	board := t.TempDir()
	configDir := filepath.Join(board, "_lym", "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, ConfigModule+".yaml"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return board
}

func TestLoadConfig(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	defaults := Config{
		MaxBounces: 7, HardMaxBounces: 10, Reviewer: []string{"opus", "fable", "sonnet"}, Fixer: []string{"opus", "fable", "sonnet"},
		Concurrency: 3, LiveAttempts: 2, VerifyAttempts: 3,
	}
	tests := []struct {
		name     string
		contents string
		want     Config
		wantErr  []string
		wantIs   error
	}{
		{name: "TemplateLoadsEveryDefault", contents: ConfigTemplate(), want: defaults},
		{
			name: "HubFileSetsEveryKey",
			contents: `packages: [internal/foo, internal/bar]
llm_driving: true
max_bounces: 2
hard_max_bounces: 4
reviewer: ["sonnet[high]"]
fixer: ["opus[low]", sonnet]
judge: "opus[medium]"
seed: sonnet
reviewer_fan: standard
fixer_fan: fix-code
auto_accept: true
concurrency: 5
live_attempts: 3
verify_attempts: 4
tree_scope:
  tools/: ./tools/...
`,
			want: Config{
				Packages: []string{"internal/foo", "internal/bar"}, LLMDriving: true, MaxBounces: 2, HardMaxBounces: 4,
				Reviewer: []string{"sonnet[high]"}, Fixer: []string{"opus[low]", "sonnet"}, Judge: "opus[medium]", Seed: "sonnet",
				ReviewerFan: "standard", FixerFan: "fix-code", AutoAccept: true, Concurrency: 5, LiveAttempts: 3, VerifyAttempts: 4,
				TreeScope: map[string]string{"tools": "./tools/..."},
			},
		},
		{name: "ConcurrencyBelowOne", contents: "concurrency: 0\n", wantErr: []string{`"concurrency"`, `lym config hardener --set concurrency`, `lym loom resume`}},
		{name: "LiveAttemptsBelowOne", contents: "live_attempts: 0\n", wantErr: []string{`"live_attempts"`, `lym config hardener --set live_attempts`, `lym loom resume`}},
		{name: "VerifyAttemptsBelowOne", contents: "verify_attempts: 0\n", wantErr: []string{`"verify_attempts"`, `lym config hardener --set verify_attempts`, `lym loom resume`}},
		{name: "HardMaxBouncesBelowOne", contents: "hard_max_bounces: 0\n", wantErr: []string{`"hard_max_bounces"`, `lym config hardener --set hard_max_bounces`, `lym loom resume`}},
		{name: "NegativeMaxBounces", contents: "max_bounces: -1\n", wantErr: []string{`"max_bounces"`, `lym config hardener --set max_bounces`, `lym loom resume`}},
		{name: "ReviewerEntryNotAModelSpec", contents: "reviewer: [\"opus[medium\"]\n", wantErr: []string{`"reviewer"`, `lym config hardener --set reviewer`, `lym loom resume`}},
		{name: "FixerEntryNotAModelSpec", contents: "fixer: [opus, \"a b\"]\n", wantErr: []string{`"fixer"`, `lym config hardener --set fixer`}},
		{name: "JudgeNotAModelSpec", contents: "judge: \"opus[medium\"\n", wantErr: []string{`"judge"`, `lym config hardener --set judge`}},
		{name: "SeedNotAModelSpec", contents: "seed: \"opus[medium\"\n", wantErr: []string{`"seed"`, `lym config hardener --set seed`}},
		{
			name: "MalformedTreeKey", contents: "tree_scope:\n  \"/abs\": x\n",
			wantErr: []string{`"tree_scope"`, `"/abs"`, `lym loom resume`}, wantIs: darnengine.ErrTreeScopeKey,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := LoadConfig(configengine.Dirs{Board: seedHardenerConfig(t, tt.contents)}, ConfigModule)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("LoadConfig() = _, nil; want an error holding %q", tt.wantErr)
				}
				for _, want := range tt.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("LoadConfig() error = %q; want it to hold %q", err, want)
					}
				}
				if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
					t.Errorf("LoadConfig() error = %v; want it to wrap %v", err, tt.wantIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadConfig() = _, %v; want nil", err)
			}
			if diff := cmp.Diff(tt.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("LoadConfig() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
