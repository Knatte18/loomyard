// promote.go declares the promote subcommand: it copies a live board-copy edit back into the
// current worktree's contracts/stencils/<family>/ source tree, stripping the stamp line on the way in because
// the source tree is the seed and carries no stamp of its own.
// promote writes only into the worktree source tree; it never writes to the board copy, never
// commits, and never pushes -- porting an edit back and landing it are deliberately separate acts.

package stencilcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// newPromoteCmd builds the promote subcommand. loc resolves the *lyxcwd.Location the parent's
// PersistentPreRunE stored, the same closure-over-a-shared-variable pattern Command() uses for list,
// validate, and sync.
func newPromoteCmd(loc func() *lyxcwd.Location) *cobra.Command {
	return &cobra.Command{
		Use:   "promote <name>",
		Short: "Copy a board-copy edit back into this worktree's contracts/stencils/ source tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			l := loc()
			out := cmd.OutOrStdout()

			if len(args) != 1 {
				clihelp.SetExit(cmd.Context(), output.Err(out, "usage: lyx stencil promote <name>"))
				return nil
			}
			name := args[0]

			if _, known := stencils.Registry().Default(name); !known {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("stencil: unknown stencil %q", name)))
				return nil
			}

			sourceDir := resolveSourceDir(l)
			if sourceDir == "" {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf(
					"stencil: no contracts/stencils/ source tree present at %s", filepath.Join(l.WorktreePath(), "contracts", "stencils"))))
				return nil
			}

			stencilsDir := fabricengine.StencilsDir(l.HubPath)
			boardPath := stencilstore.Path(stencilsDir, name)
			boardContent, err := os.ReadFile(boardPath)
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("stencil: read board copy %s: %v", boardPath, err)))
				return nil
			}

			targetPath := filepath.Join(sourceDir, filepath.FromSlash(stencilstore.RelPath(name)))
			if _, err := os.Stat(targetPath); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf(
					"stencil: no source file %s for %q in this worktree's contracts/stencils/ tree", targetPath, name)))
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("stencil: create parent directory for %s: %v", targetPath, err)))
				return nil
			}
			if err := os.WriteFile(targetPath, stripStampLine(boardContent), 0o644); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("stencil: write %s: %v", targetPath, err)))
				return nil
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"name": name,
				"path": targetPath,
			}))
			return nil
		},
	}
}

// stripStampLine removes only the "lyx-stencil: sha256=..." line from content's leading
// `<!-- ... -->` banner, leaving every other line of that banner and the entire body
// byte-identical. A stencil.StripLeadingComment of the whole banner would discard human-facing
// documentation the source file is supposed to keep -- this strips exactly the stamp and nothing
// else.
// content with no leading banner, or a banner carrying no stamp line, is returned unchanged.
// When the stamp was the banner's only content, the whole banner (and the one blank line
// immediately following it) is dropped rather than left as an empty `<!--  -->` husk.
func stripStampLine(content []byte) []byte {
	s := string(content)
	trimmed := strings.TrimLeft(s, " \t\r\n")
	if !strings.HasPrefix(trimmed, "<!--") {
		return content
	}
	leadWS := len(s) - len(trimmed)
	closeIdx := strings.Index(trimmed, "-->")
	if closeIdx == -1 {
		return content
	}
	end := leadWS + closeIdx + len("-->")

	stampKeyText := strings.TrimPrefix(stencilstore.StampPrefix, "<!-- ")
	inner := s[leadWS+len("<!--") : leadWS+closeIdx]
	if !strings.Contains(inner, stampKeyText) {
		return content
	}

	lines := strings.Split(inner, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.Contains(line, stampKeyText) {
			continue
		}
		kept = append(kept, line)
	}

	if strings.TrimSpace(strings.Join(kept, "\n")) == "" {
		afterBanner := s[end:]
		afterBanner = strings.TrimPrefix(afterBanner, "\r\n")
		afterBanner = strings.TrimPrefix(afterBanner, "\n")
		return []byte(s[:leadWS] + afterBanner)
	}

	newBanner := "<!--" + strings.TrimRight(strings.Join(kept, "\n"), " \t") + " -->"
	return []byte(s[:leadWS] + newBanner + s[end:])
}
