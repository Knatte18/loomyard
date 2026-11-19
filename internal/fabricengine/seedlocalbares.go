// seedlocalbares.go builds the local bare repositories a throwaway hub is cloned from.

package fabricengine

import (
	"fmt"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/gitexec"
)

// The directory names of the two bares SeedLocalBares creates, and the one file the warp bare's commit holds.
const (
	seededWarpDirName  = "warp-bare"
	seededWeftDirName  = "weft-bare"
	seededReadmeName   = "README"
	seededReadmeText   = "a seeded local warp\n"
	seededCommitAuthor = "lym"
	seededCommitEmail  = "lym@localhost"
	seededBranch       = "main"
)

// SeedLocalBares creates the bare repositories a hub is cloned from under dir, which must exist, and returns their paths.
// The warp bare holds one commit with a root README on main, and its HEAD names main.
// The weft bare is empty and holds no ref.
// It is the shape CloneHub clones a hub from, built with no remote and no work tree.
func SeedLocalBares(dir string) (warp, weft string, err error) {
	warp = filepath.Join(dir, seededWarpDirName)
	weft = filepath.Join(dir, seededWeftDirName)
	for _, bare := range []string{warp, weft} {
		if err := initQuietBare(dir, bare); err != nil {
			return "", "", err
		}
	}
	if err := commitReadmeOnMain(warp); err != nil {
		return "", "", err
	}
	return warp, weft, nil
}

// initQuietBare creates a bare repository at bare on main, with background maintenance off.
func initQuietBare(cwd, bare string) error {
	if _, err := gitexec.Run([]string{"init", "--bare", "--initial-branch=" + seededBranch, bare}, cwd); err != nil {
		return fmt.Errorf("init bare repository %s: %w", bare, err)
	}
	for _, setting := range [][]string{{"gc.auto", "0"}, {"maintenance.auto", "false"}} {
		if _, err := gitexec.Run(append([]string{"config"}, setting...), bare); err != nil {
			return fmt.Errorf("configure bare repository %s: %w", bare, err)
		}
	}
	return nil
}

// commitReadmeOnMain writes a root commit holding one README onto main of the bare repository at bare.
func commitReadmeOnMain(bare string) error {
	blob, err := gitexec.RunStdin([]string{"hash-object", "-w", "--stdin"}, bare, seededReadmeText)
	if err != nil {
		return fmt.Errorf("write the seeded README in %s: %w", bare, err)
	}
	tree, err := gitexec.RunStdin([]string{"mktree"}, bare, fmt.Sprintf("100644 blob %s\t%s\n", trimNewline(blob), seededReadmeName))
	if err != nil {
		return fmt.Errorf("write the seeded tree in %s: %w", bare, err)
	}
	identity := []string{
		"GIT_AUTHOR_NAME=" + seededCommitAuthor, "GIT_AUTHOR_EMAIL=" + seededCommitEmail,
		"GIT_COMMITTER_NAME=" + seededCommitAuthor, "GIT_COMMITTER_EMAIL=" + seededCommitEmail,
	}
	commit, err := gitexec.RunWithEnv([]string{"commit-tree", trimNewline(tree), "-m", "Seed the local warp"}, bare, identity)
	if err != nil {
		return fmt.Errorf("commit the seeded tree in %s: %w", bare, err)
	}
	if _, err := gitexec.Run([]string{"update-ref", "refs/heads/" + seededBranch, trimNewline(commit)}, bare); err != nil {
		return fmt.Errorf("point %s at the seeded commit in %s: %w", seededBranch, bare, err)
	}
	return nil
}

// trimNewline returns git's one-line output without its trailing newline.
func trimNewline(output string) string {
	for len(output) > 0 && (output[len(output)-1] == '\n' || output[len(output)-1] == '\r') {
		output = output[:len(output)-1]
	}
	return output
}
