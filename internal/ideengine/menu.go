// menu.go implements the interactive `ide menu` picker over active worktrees.
// It shows each worktree's task title from the board and returns an error when the board fails its health check.

package ideengine

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/vscode"
)

// Menu lists the active worktrees and reads the user's choice from in.
// A worktree is active when it is not the main worktree and its anchor holds a `_lym` directory.
// Each entry shows its task title from the board when the board has one.
// Menu returns an error when the board config cannot be loaded or the board fails its health check.
// An input of q returns nil without starting a worktree; otherwise Menu starts the chosen one.
func Menu(l *lymcwd.Location, in io.Reader, out io.Writer) error {
	boardDir := fabricengine.BoardDir(l.HubPath)
	cfg, err := boardengine.LoadConfig(fabricengine.ConfigDirs(l), "board")
	if err != nil {
		return fmt.Errorf("load board config: %w", err)
	}

	cfg.Path = boardDir

	b := boardengine.New(cfg)

	if err := b.HealthCheck(); err != nil {
		return fmt.Errorf("board health check failed: %w", err)
	}

	entries, err := fabricengine.List(l.AnchorPath())
	if err != nil {
		return fmt.Errorf("list worktrees: %w", err)
	}

	var activeWorktrees []string
	var slugs []string

	for _, entry := range entries {
		if entry.Main {
			continue
		}

		slug := filepath.Base(entry.Path)

		own, err := lymcwd.NamedWorktree(l.HubPath, slug)
		if err != nil {
			logger.Warn("worktree left out of the menu: its anchor cannot be read", "slug", slug, "error", err)
			continue
		}
		lymPath := filepath.Join(entry.Path, own.AnchorRel, lymdirs.LymDirName)
		stat, err := os.Stat(lymPath)
		if err != nil || !stat.IsDir() {
			continue
		}

		activeWorktrees = append(activeWorktrees, entry.Path)
		slugs = append(slugs, slug)
	}

	if len(activeWorktrees) == 0 {
		fmt.Fprintln(out, "no active worktrees")
		return nil
	}

	for i, slug := range slugs {
		task, found, err := b.GetTask(slug)
		title := ""
		if found && err == nil {
			title = task.Title
		}

		if title == "" {
			fmt.Fprintf(out, "%d) %s\n", i+1, slug)
		} else {
			fmt.Fprintf(out, "%d) %s — %s\n", i+1, slug, title)
		}
	}

	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("read input: %w", err)
	}

	line = strings.TrimSpace(line)

	if line == "q" {
		return nil
	}

	num, err := strconv.Atoi(line)
	if err != nil {
		return fmt.Errorf("invalid input: must be a number or 'q'")
	}

	if num < 1 || num > len(activeWorktrees) {
		return fmt.Errorf("invalid selection: %d (must be 1-%d or 'q')", num, len(activeWorktrees))
	}

	chosenSlug := slugs[num-1]

	keybindings, err := Spawn(l, chosenSlug)
	if keybindings.Outcome == vscode.KeybindingsSkipped {
		logger.Warn("keybindings block not seeded", "slug", chosenSlug, "reason", keybindings.Reason)
	}
	if err != nil {
		return fmt.Errorf("spawn %s: %w", chosenSlug, err)
	}

	return nil
}
