// menu.go implements the interactive `ide menu` picker over active worktrees,
// resolving each worktree's title via the board facade and hard-erroring when
// the board fails its health check.

package ide

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/board"
	"github.com/Knatte18/loomyard/internal/paths"
)

// Menu presents an interactive picker of active worktrees, allowing the user to open one via Spawn.
//
// It discovers active worktrees from paths.List(l.Cwd), excluding the main worktree (Main==true)
// and only including those whose <path>/<l.RelPath>/_lyx directory exists.
//
// Titles are resolved ONLY through the board facade (b.GetTask(slug) → Task.Title).
// If the board config cannot be loaded, it returns a HARD error. Similarly, if the board
// is absent or unhealthy, it returns a HARD error after b.HealthCheck() fails.
//
// It prints a numbered picker to out, reads from in, and opens the selected worktree via Spawn.
// A zero-worktree list prints a message and returns success.
//
// Arguments:
//   - l: the Layout
//   - in: input reader (for reading user selection)
//   - out: output writer (for printing the picker menu)
//
// Returns an error on failure (HARD error if config load or HealthCheck fails), or nil on success.
func Menu(l *paths.Layout, in io.Reader, out io.Writer) error {
	// Load board config and create board facade
	cfg, err := board.LoadConfig(l.Cwd, "board")
	if err != nil {
		return fmt.Errorf("load board config: %w", err)
	}

	b := board.New(cfg)

	// HARD error if board is absent/unhealthy
	if err := b.HealthCheck(); err != nil {
		return fmt.Errorf("board health check failed: %w", err)
	}

	// Discover active worktrees via paths.List
	entries, err := paths.List(l.Cwd)
	if err != nil {
		return fmt.Errorf("list worktrees: %w", err)
	}

	// Filter: exclude main, and only keep those with _lyx at <path>/<l.RelPath>/_lyx
	var activeWorktrees []string
	var slugs []string

	for _, entry := range entries {
		// Skip main worktree
		if entry.Main {
			continue
		}

		// Extract slug from worktree path (basename)
		slug := filepath.Base(entry.Path)

		// Check if _lyx exists at <path>/<l.RelPath>/_lyx
		lyxPath := filepath.Join(entry.Path, l.RelPath, paths.LyxDirName)
		stat, err := os.Stat(lyxPath)
		if err != nil || !stat.IsDir() {
			// _lyx doesn't exist or is not a directory; skip
			continue
		}

		activeWorktrees = append(activeWorktrees, entry.Path)
		slugs = append(slugs, slug)
	}

	// If zero active worktrees, print message and return success
	if len(activeWorktrees) == 0 {
		fmt.Fprintln(out, "no active worktrees")
		return nil
	}

	// Print numbered picker with slug and title
	for i, slug := range slugs {
		// Resolve title via board facade
		task, found, err := b.GetTask(slug)
		title := ""
		if found && err == nil {
			title = task.Title
		}

		// Format: N) <slug> — <title>
		if title == "" {
			fmt.Fprintf(out, "%d) %s\n", i+1, slug)
		} else {
			fmt.Fprintf(out, "%d) %s — %s\n", i+1, slug, title)
		}
	}

	// Read user selection from input
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("read input: %w", err)
	}

	line = strings.TrimSpace(line)

	// Handle 'q' to quit
	if line == "q" {
		return nil
	}

	// Try to parse as number
	num, err := strconv.Atoi(line)
	if err != nil {
		return fmt.Errorf("invalid input: must be a number or 'q'")
	}

	// Validate range (1-indexed)
	if num < 1 || num > len(activeWorktrees) {
		return fmt.Errorf("invalid selection: %d (must be 1-%d or 'q')", num, len(activeWorktrees))
	}

	// Get the chosen slug
	chosenSlug := slugs[num-1]

	// Spawn the chosen worktree
	if err := Spawn(l, chosenSlug); err != nil {
		return fmt.Errorf("spawn %s: %w", chosenSlug, err)
	}

	return nil
}
