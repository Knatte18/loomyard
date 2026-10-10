// glyphs.go declares the "glyphs" subcommand: the flat glyph index the planner reads its
// spellings out of.

package quarrycli

import (
	"github.com/spf13/cobra"

	"github.com/Knatte18/quarry/quarry"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/planglyph"
)

// newGlyphsCmd returns the "glyphs" subcommand, reading the resolved repository root through root
// at call time.
func newGlyphsCmd(root func() string) *cobra.Command {
	var text bool
	cmd := &cobra.Command{
		Use:         "glyphs <dir>",
		Short:       "list every glyph under a repository-relative directory, flat and depth-first",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `glyphs answers a glyphs query for dir -- a repository-relative directory path, with
"" and "." both meaning the repository root -- under quarry's own frozen preset
(depth all, symbols on), and emits quarry's own JSON rendering unchanged. This is
the flat index a plan's Create/Rename/Delete targets copy their glyph spellings
out of, verbatim.

With --text, glyphs emits quarry's own text view instead, one line per symbol ("<file>:<start>-<end> <kind> <id>", followed by "[incomplete] <path>" lines when present).
Use it before any line filter such as grep: a line carries one whole symbol,
so the filter drops symbols rather than single JSON fields.
The glyph to copy from a line is its last field, the id.

Examples:
  lyx quarry glyphs internal/planglyph
  lyx quarry glyphs --text internal/planglyph | grep -v _test.go`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			out := cmd.OutOrStdout()

			answer, err := planglyph.Glyphs(root(), args[0])
			if err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
				return nil
			}

			var data []byte
			if text {
				data = []byte(quarry.RenderGlyphsText(answer))
			} else {
				data, err = quarry.RenderGlyphsJSON(answer)
				if err != nil {
					clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
					return nil
				}
			}
			if _, err := out.Write(data); err != nil {
				clihelp.SetExit(cmd.Context(), output.Err(out, err.Error()))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&text, "text", false, "emit quarry's one-line-per-symbol text view instead of JSON")
	return cmd
}
