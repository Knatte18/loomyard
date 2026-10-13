// fmtverb.go implements the `fmt` loom verb, which rewrites named Go files in place to their go/format form.
// It gives an agent whose edit directive forbids every rewriting tool a sanctioned way to fix a file's formatting.

package loomcli

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/spf13/cobra"
)

// goRewrite is one named Go file that passed every check, with its go/format form.
type goRewrite struct {
	// resolved is the file's path with symlinks resolved, the path a write goes to.
	resolved string
	// named is the path as the caller wrote it.
	named string
	// formatted is the file's go/format form.
	formatted []byte
	// changed is whether formatted differs from the file's bytes.
	changed bool
}

// planGoFormat checks and formats every path before the caller writes any.
// A relative path resolves against cwd.
// A path passes when its symlink-resolved form lies strictly inside the symlink-resolved root, ends in `.go`, reads and parses;
// it yields a goRewrite.
// Every other path yields one refused entry, `<path as named>: <reason>`.
func planGoFormat(root, cwd string, paths []string) (rewrites []goRewrite, refused []string) {
	resolvedRoot, rootErr := filepath.EvalSymlinks(root)
	for _, named := range paths {
		if rootErr != nil {
			refused = append(refused, fmt.Sprintf("%s: resolve worktree root: %v", named, rootErr))
			continue
		}
		joined := named
		if !filepath.IsAbs(joined) {
			joined = filepath.Join(cwd, joined)
		}
		resolved, err := filepath.EvalSymlinks(joined)
		if errors.Is(err, fs.ErrNotExist) {
			refused = append(refused, named+": no such file")
			continue
		}
		if err != nil {
			refused = append(refused, fmt.Sprintf("%s: %v", named, err))
			continue
		}
		if !strictlyInside(resolvedRoot, resolved) {
			refused = append(refused, named+": outside this worktree")
			continue
		}
		if !strings.HasSuffix(resolved, ".go") {
			refused = append(refused, named+": not a .go file")
			continue
		}
		source, err := os.ReadFile(resolved)
		if err != nil {
			refused = append(refused, fmt.Sprintf("%s: %v", named, err))
			continue
		}
		formatted, err := format.Source(source)
		if err != nil {
			refused = append(refused, fmt.Sprintf("%s: does not parse: %v", named, err))
			continue
		}
		rewrites = append(rewrites, goRewrite{
			resolved:  resolved,
			named:     named,
			formatted: formatted,
			changed:   !bytes.Equal(formatted, source),
		})
	}
	return rewrites, refused
}

// strictlyInside reports whether path lies below root, never equal to it.
func strictlyInside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// fmtCmd builds the `fmt` subcommand.
// It rewrites only `.go` files the caller names inside its own worktree, and only to their go/format form; it runs no binary.
func (c *loomCLI) fmtCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "fmt <file>...",
		Short:       "rewrite named Go files in place to their go/format form",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `fmt rewrites each named .go file in place to its go/format form, in-process,
with no gofmt binary. It is the sanctioned way to fix the formatting
cmd/lyx/gofmt_test.go fails. A relative path resolves against the current
directory. Every file is checked and formatted before any is written: one
path that does not exist, lies outside this worktree, is not a .go file or
does not parse refuses the whole call and writes nothing. The envelope's
"changed" lists the files it rewrote; a file already formatted is left
untouched and unreported.

Example:
  lyx loom fmt internal/x/y.go
  lyx loom fmt a.go b.go`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			if len(args) == 0 {
				clihelp.SetExit(cmd.Context(), output.Err(out, `loom fmt: name at least one .go file; way forward: run "lyx loom fmt <file>..." with the Go files to format`))
				return nil
			}

			rewrites, refused := planGoFormat(c.env.WorktreeRoot, c.cwd, args)
			if len(refused) > 0 {
				clihelp.SetExit(cmd.Context(), output.ErrFields(out, `loom fmt: refused a named file; way forward: name only .go files inside this worktree that parse, then re-run "lyx loom fmt"`, map[string]any{
					"refused": refused,
				}))
				return nil
			}

			changed := []string{}
			for _, rewrite := range rewrites {
				if !rewrite.changed {
					continue
				}
				if err := os.WriteFile(rewrite.resolved, rewrite.formatted, 0o644); err != nil {
					clihelp.SetExit(cmd.Context(), output.ErrFields(out, `loom fmt: write failed; way forward: re-run "lyx loom fmt" with the same files once the write error is fixed; a formatted file is left untouched`, map[string]any{
						"written": changed,
						"failed":  fmt.Sprintf("%s: %v", rewrite.named, err),
					}))
					return nil
				}
				changed = append(changed, rewrite.named)
			}

			clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{
				"changed": changed,
			}))
			return nil
		},
	}
}
