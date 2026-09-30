// skipenv.go — the BOARD_SKIP_* environment fold.
//
// ApplySkipEnv is the one place BOARD_SKIP_GIT and BOARD_SKIP_PUSH are read;
// every CLI entry that builds a Config calls it, so no caller carries a second copy.

package boardengine

import "os"

// ApplySkipEnv folds BOARD_SKIP_* environment variables into cfg.
// BOARD_SKIP_GIT=1 sets SkipGit, BOARD_SKIP_PUSH=1 sets SkipPush,
// and anything else leaves the field as cfg had it.
func ApplySkipEnv(cfg Config) Config {
	if os.Getenv("BOARD_SKIP_GIT") == "1" {
		cfg.SkipGit = true
	}
	if os.Getenv("BOARD_SKIP_PUSH") == "1" {
		cfg.SkipPush = true
	}
	return cfg
}
