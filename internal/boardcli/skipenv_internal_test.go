// skipenv_internal_test.go — white-box unit tests for applySkipEnv.
//
// Tests the env→cfg resolution helper that folds BOARD_SKIP_* environment variables
// into the Config struct at the CLI entry point.

package boardcli

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

func TestApplySkipEnv(t *testing.T) {
	tests := []struct {
		name         string
		skipGitEnv   string
		skipPushEnv  string
		cfgSkipGit   bool
		cfgSkipPush  bool
		wantSkipGit  bool
		wantSkipPush bool
	}{
		{
			name:         "both env unset",
			skipGitEnv:   "",
			skipPushEnv:  "",
			cfgSkipGit:   false,
			cfgSkipPush:  false,
			wantSkipGit:  false,
			wantSkipPush: false,
		},
		{
			name:         "BOARD_SKIP_GIT=1",
			skipGitEnv:   "1",
			skipPushEnv:  "",
			cfgSkipGit:   false,
			cfgSkipPush:  false,
			wantSkipGit:  true,
			wantSkipPush: false,
		},
		{
			name:         "BOARD_SKIP_PUSH=1",
			skipGitEnv:   "",
			skipPushEnv:  "1",
			cfgSkipGit:   false,
			cfgSkipPush:  false,
			wantSkipGit:  false,
			wantSkipPush: true,
		},
		{
			name:         "both env set to 1",
			skipGitEnv:   "1",
			skipPushEnv:  "1",
			cfgSkipGit:   false,
			cfgSkipPush:  false,
			wantSkipGit:  true,
			wantSkipPush: true,
		},
		{
			name:         "cfg.SkipPush=true, env unset",
			skipGitEnv:   "",
			skipPushEnv:  "",
			cfgSkipGit:   false,
			cfgSkipPush:  true,
			wantSkipGit:  false,
			wantSkipPush: true,
		},
		{
			name:         "cfg.SkipPush=true, BOARD_SKIP_PUSH=1",
			skipGitEnv:   "",
			skipPushEnv:  "1",
			cfgSkipGit:   false,
			cfgSkipPush:  true,
			wantSkipGit:  false,
			wantSkipPush: true,
		},
		{
			name:         "cfg.SkipGit=true, env unset",
			skipGitEnv:   "",
			skipPushEnv:  "",
			cfgSkipGit:   true,
			cfgSkipPush:  false,
			wantSkipGit:  true,
			wantSkipPush: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipGitEnv != "" {
				t.Setenv("BOARD_SKIP_GIT", tt.skipGitEnv)
			} else {
				t.Setenv("BOARD_SKIP_GIT", "")
			}
			if tt.skipPushEnv != "" {
				t.Setenv("BOARD_SKIP_PUSH", tt.skipPushEnv)
			} else {
				t.Setenv("BOARD_SKIP_PUSH", "")
			}

			cfg := boardengine.Config{
				SkipGit:  tt.cfgSkipGit,
				SkipPush: tt.cfgSkipPush,
			}
			result := applySkipEnv(cfg)

			if result.SkipGit != tt.wantSkipGit {
				t.Errorf("SkipGit = %v, want %v", result.SkipGit, tt.wantSkipGit)
			}
			if result.SkipPush != tt.wantSkipPush {
				t.Errorf("SkipPush = %v, want %v", result.SkipPush, tt.wantSkipPush)
			}
		})
	}
}
