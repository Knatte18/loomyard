// diff.go declares the diff subcommand, its --all and --exit-code flags, and the line-level unified
// diff renderer both diff modes share.
//
// The two modes compare different base texts and must never be conflated: `diff <name>` renders the
// default this file was forked from (recovered by stamp hash from board history) against the
// currently shipped default, showing upstream changes the operator has not yet taken; `diff --all`
// renders the worktree's own contracts/stencils/ source body against the live board copy's body, catching an
// edit made in the board copy that was never ported back. Comparing --all against the shipped
// default instead would leave the port-back warning firing right through the fix, because promote
// does not change the embedded default until the next deploy.

package stencilcli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// newDiffCmd builds the diff subcommand. loc resolves the *lyxcwd.Location the parent's
// PersistentPreRunE stored, the same closure-over-a-shared-variable pattern Command() uses for list,
// validate, and sync.
func newDiffCmd(loc func() *lyxcwd.Location) *cobra.Command {
	var all bool
	var exitCode bool

	cmd := &cobra.Command{
		Use:   "diff [name]",
		Short: "Show upstream changes not yet taken, or (--all) board edits not yet ported back",
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}

			l := loc()
			out := cmd.OutOrStdout()

			if all {
				runDiffAll(cmd, out, l, exitCode)
				return nil
			}

			if len(args) != 1 {
				clihelp.SetExit(cmd.Context(), output.Err(out, "usage: lyx stencil diff <name> (or lyx stencil diff --all)"))
				return nil
			}
			runDiffOne(cmd, out, l, args[0], exitCode)
			return nil
		},
	}

	cmd.Flags().BoolVar(&all, "all", false, "compare every registry stencil's worktree contracts/stencils/ source against its live board copy")
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "exit non-zero when any compared pair differs, like git diff --exit-code")

	return cmd
}

// runDiffOne implements `diff <name>`: base-versus-currently-shipped-default, where base is
// recovered from board history by stamp hash. found=false is reported explicitly in the envelope
// rather than as an empty diff, and the response falls back to shipped-default-versus-on-disk so the
// operator still sees something actionable.
func runDiffOne(cmd *cobra.Command, out io.Writer, l *lyxcwd.Location, name string, exitCode bool) {
	registry := stencils.Registry()
	shipped, known := registry.Default(name)
	if !known {
		clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("stencil: unknown stencil %q", name)))
		return
	}

	stencilsDir := fabricengine.StencilsDir(l.HubPath)
	boardPath := stencilstore.Path(stencilsDir, name)
	onDisk, err := os.ReadFile(boardPath)
	if err != nil {
		clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("stencil: read board copy %s: %v", boardPath, err)))
		return
	}

	stamp, stamped := stencilstore.ParseStamp(onDisk)

	var (
		base   []byte
		rev    string
		found  bool
		reason string
	)
	switch {
	case !stamped:
		reason = "on-disk stencil carries no parseable stamp"
	default:
		recovered, foundRev, ok, err := fabricengine.StencilBaseByStamp(l.HubPath, name, stamp)
		if err != nil {
			clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf("stencil: recover base for %q: %v", name, err)))
			return
		}
		if ok {
			base, rev, found = recovered, foundRev, true
		} else {
			reason = "no board history revision matches the on-disk stamp"
		}
	}

	fields := map[string]any{"name": name, "found": found}

	var diffText string
	var differs bool
	if found {
		fields["base_revision"] = rev
		diffText, differs = renderUnifiedDiff(base, shipped, "base", "shipped default")
	} else {
		fields["reason"] = reason
		diffText, differs = renderUnifiedDiff(shipped, onDisk, "shipped default", "on-disk")
	}
	fields["diff"] = diffText

	clihelp.SetExit(cmd.Context(), output.Ok(out, fields))
	if exitCode && differs {
		clihelp.SetExit(cmd.Context(), 1)
	}
}

// runDiffAll implements `diff --all`: the port-back guard, comparing every registry stencil's
// worktree contracts/stencils/ source body against its live board copy body, both normalised via
// stencilstore.NormalizeLF and stencil.StripLeadingComment. A registry entry whose board copy or
// source file cannot be read is skipped, mirroring stencilstore's own drift-warning behaviour.
func runDiffAll(cmd *cobra.Command, out io.Writer, l *lyxcwd.Location, exitCode bool) {
	sourceDir := resolveSourceDir(l)
	if sourceDir == "" {
		clihelp.SetExit(cmd.Context(), output.Err(out, fmt.Sprintf(
			"stencil: no contracts/stencils/ source tree present at %s", filepath.Join(l.WorktreePath(), "contracts", "stencils"))))
		return
	}

	stencilsDir := fabricengine.StencilsDir(l.HubPath)
	registry := stencils.Registry()

	var results []map[string]any
	anyDiffer := false

	for _, name := range registry.Names() {
		boardPath := stencilstore.Path(stencilsDir, name)
		boardContent, err := os.ReadFile(boardPath)
		if err != nil {
			continue
		}

		sourcePath := filepath.Join(sourceDir, filepath.FromSlash(stencilstore.RelPath(name)))
		sourceContent, err := os.ReadFile(sourcePath)
		if err != nil {
			continue
		}

		boardBody := stencilstore.NormalizeLF([]byte(stencil.StripLeadingComment(string(boardContent))))
		sourceBody := stencilstore.NormalizeLF([]byte(stencil.StripLeadingComment(string(sourceContent))))

		diffText, differs := renderUnifiedDiff(sourceBody, boardBody, "worktree source", "board copy")
		if differs {
			anyDiffer = true
		}
		results = append(results, map[string]any{
			"name":    name,
			"differs": differs,
			"diff":    diffText,
		})
	}

	clihelp.SetExit(cmd.Context(), output.Ok(out, map[string]any{"stencils": results}))
	if exitCode && anyDiffer {
		clihelp.SetExit(cmd.Context(), 1)
	}
}

// diffOpKind labels one line-level diff operation.
type diffOpKind int

// The three line-level diff operations diffLines can emit.
const (
	diffEqual diffOpKind = iota
	diffDelete
	diffInsert
)

// diffOp is one line-level diff operation: a line kept as context, removed from a, or added in b.
type diffOp struct {
	kind diffOpKind
	line string
}

// diffLines computes a line-level diff between a and b via a straightforward longest-common-
// subsequence dynamic program. This is quadratic in the line counts, which is fine for prompt-sized
// files (at most a few hundred lines) and not a general-purpose diff engine.
func diffLines(a, b []string) []diffOp {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				lcs[i][j] = lcs[i+1][j+1] + 1
			case lcs[i+1][j] >= lcs[i][j+1]:
				lcs[i][j] = lcs[i+1][j]
			default:
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{kind: diffEqual, line: a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, diffOp{kind: diffDelete, line: a[i]})
			i++
		default:
			ops = append(ops, diffOp{kind: diffInsert, line: b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{kind: diffDelete, line: a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{kind: diffInsert, line: b[j]})
	}
	return ops
}

// diffContext is the number of unchanged lines of context renderUnifiedDiff carries on each side of
// the changed range it hunks.
const diffContext = 3

// renderUnifiedDiff renders a single-hunk unified diff between aBody and bBody, labelled fromLabel
// and toLabel in the "--- "/"+++ " header lines, and reports whether the two bodies differ at all.
// A caller must gate an "empty diff" reading on differs, never on text == "": this function returns
// ("", false) precisely when the bodies are identical, and never returns an empty string when
// differs is true.
func renderUnifiedDiff(aBody, bBody []byte, fromLabel, toLabel string) (text string, differs bool) {
	aLines := strings.Split(string(aBody), "\n")
	bLines := strings.Split(string(bBody), "\n")
	ops := diffLines(aLines, bLines)

	firstChange, lastChange := -1, -1
	for idx, op := range ops {
		if op.kind == diffEqual {
			continue
		}
		if firstChange == -1 {
			firstChange = idx
		}
		lastChange = idx
	}
	if firstChange == -1 {
		return "", false
	}

	hunkStart := firstChange
	for hunkStart > 0 && firstChange-hunkStart < diffContext {
		hunkStart--
	}
	hunkEnd := lastChange
	for hunkEnd < len(ops)-1 && hunkEnd-lastChange < diffContext {
		hunkEnd++
	}

	aStart, bStart := 1, 1
	for _, op := range ops[:hunkStart] {
		switch op.kind {
		case diffEqual:
			aStart++
			bStart++
		case diffDelete:
			aStart++
		case diffInsert:
			bStart++
		}
	}

	aCount, bCount := 0, 0
	for _, op := range ops[hunkStart : hunkEnd+1] {
		switch op.kind {
		case diffEqual:
			aCount++
			bCount++
		case diffDelete:
			aCount++
		case diffInsert:
			bCount++
		}
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "--- %s\n", fromLabel)
	fmt.Fprintf(&buf, "+++ %s\n", toLabel)
	fmt.Fprintf(&buf, "@@ -%d,%d +%d,%d @@\n", aStart, aCount, bStart, bCount)
	for _, op := range ops[hunkStart : hunkEnd+1] {
		switch op.kind {
		case diffEqual:
			fmt.Fprintf(&buf, " %s\n", op.line)
		case diffDelete:
			fmt.Fprintf(&buf, "-%s\n", op.line)
		case diffInsert:
			fmt.Fprintf(&buf, "+%s\n", op.line)
		}
	}

	return buf.String(), true
}
