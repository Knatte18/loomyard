// Command godocreflow reflows the text of Go doc-comment blocks -- file-level header comments, package doc
// comments, and comments immediately preceding an exported declaration -- to semantic line breaks (one
// sentence per line, plus a break at an internal independent-clause boundary), per the golang-comments
// skill's "Line-wrap style" section.
//
// It is a pure mechanical transform: only re-breaks existing comment text onto different lines, never
// adds, drops, or reorders a word. Comment discovery uses go/parser and go/ast (not regex over raw text),
// so a "//" inside a string literal can never be mistaken for a comment. The sentence/clause-splitting
// heuristics (abbreviations, backtick/paren-span guards, the Oxford-comma "first eligible comma" rule) are
// ported from millhouse's plugins/mill/scripts/tools/pydocreflow/pydocreflow.py; see split.go.
//
// A semantic line that still exceeds -max-width after sentence/clause splitting -- and has no further
// semicolon/conjunction boundary left to break at -- gets one last-resort greedy word-wrap. This is a
// targeted exception for the rare over-wide outlier (uncomfortable in side-by-side diff view), not a
// return to general fixed-column wrapping; see wrapLongLine in split.go. -max-width is a flag rather than
// a compiled-in constant specifically so the column budget can be tuned per run with no rebuild.
//
// Left untouched: single-line comments that already fit on one line, end-of-line inline comments,
// build/tool directive comments (//go:generate, //go:build, //nolint, struct-tag-adjacent comments a
// linter parses), any comment block containing indented example code or a Go 1.19 doc-comment
// heading/list, struct-field and interface-method doc comments, and generated files ("Code generated ...
// DO NOT EDIT" header).
//
//	go run ./tools/godocreflow [-check] [-max-width N] <file_or_dir> ...
//
//	-check		print before/after blocks instead of writing files (dry run)
//	-max-width	last-resort word-wrap column budget, 0 disables it (default 100)
//
// Last run: 2026-08-06, repo-wide sweep (golang-comments-linebreak-sweep task).
package main

import (
	"flag"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sort"
)

func main() {
	check := flag.Bool("check", false, "print before/after blocks instead of writing files (dry run)")
	maxWidth := flag.Int("max-width", 100, "last-resort word-wrap column budget (indent + \"// \" + content); 0 disables it")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: godocreflow [-check] [-max-width N] <file_or_dir> ...")
		os.Exit(2)
	}

	files, err := collectGoFiles(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "godocreflow:", err)
		os.Exit(1)
	}

	changed := 0
	wrapped := 0
	for _, path := range files {
		srcBytes, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "godocreflow:", err)
			os.Exit(1)
		}
		src := string(srcBytes)

		fset := token.NewFileSet()
		edits, err := collectEdits(fset, path, src, *maxWidth)
		if err != nil {
			fmt.Fprintf(os.Stderr, "SKIP (parse error) %s: %v\n", path, err)
			continue
		}
		if len(edits) == 0 {
			continue
		}
		changed++
		for _, e := range edits {
			wrapped += e.wrapped
		}
		if *check {
			printEdits(path, edits)
			continue
		}
		newSrc := applyEdits(src, edits)
		if err := os.WriteFile(path, []byte(newSrc), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "godocreflow:", err)
			os.Exit(1)
		}
		fmt.Println("reflowed", path)
	}
	verb := "changed"
	if *check {
		verb = "would change"
	}
	fmt.Fprintf(os.Stderr, "%d file(s) %s\n", changed, verb)
	if wrapped > 0 {
		fmt.Fprintf(os.Stderr, "%d line(s) needed the %d-char last-resort width fallback\n", wrapped, *maxWidth)
	}
}

func collectGoFiles(args []string) ([]string, error) {
	var files []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			files = append(files, arg)
			continue
		}
		err = filepath.WalkDir(arg, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) == ".go" {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

// printEdits prints each edit as a labeled before/after block, in source order, for hand-checking.
func printEdits(path string, edits []edit) {
	sorted := make([]edit, len(edits))
	copy(sorted, edits)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].start < sorted[j].start })
	for _, e := range sorted {
		fmt.Printf("=== %s:%d ===\n--- before\n%s\n--- after\n%s\n\n", path, e.line, e.old, e.text)
	}
}
