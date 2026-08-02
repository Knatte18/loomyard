// menu.go implements the interactive `ide menu` picker over active worktrees,
// resolving each worktree's title via the board facade and hard-erroring when
// the board fails its health check.

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
	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// Menu presents an interactive picker of active worktrees, allowing the user to open one via Spawn.
// It discovers active worktrees from hubgeometry.List, excluding the main worktree and those
// without _lyx. Titles are resolved through the board facade. Returns an error on board config
// load or health check failure, or nil on success.
func Menu(l *hubgeometry.Layout, in io.Reader, out io.Writer) error {
	cfg, err := boardengine.LoadConfig(l.Cwd, "board")
	if err != nil {
		return fmt.Errorf("load board config: %w", err)
	}

	cfg.Path = hubgeometry.BoardDir(l.Hub)

	b := boardengine.New(cfg)

	if err := b.HealthCheck(); err != nil {
		return fmt.Errorf("board health check failed: %w", err)
	}

	entries, err := hubgeometry.List(l.Cwd)
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

		lyxPath := filepath.Join(entry.Path, l.RelPath, hubgeometry.LyxDirName)
		stat, err := os.Stat(lyxPath)
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

	if err := Spawn(l, chosenSlug); err != nil {
		return fmt.Errorf("spawn %s: %w", chosenSlug, err)
	}

	return nil
}
