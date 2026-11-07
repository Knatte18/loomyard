// Command godocreflow reflows the text of Go doc comments to semantic line breaks.
// It puts each sentence on its own line, and it breaks again at an internal independent-clause boundary.
// It reflows file-level header comments, package doc comments, and comments before an exported declaration.
//
// It is a pure mechanical transform.
// It only re-breaks existing comment text onto different lines; it never adds, drops, or reorders a word.
// It finds comments with go/parser and go/ast, not with a regex over raw text;
// so a "//" inside a string literal is never taken for a comment.
//
// A semantic line still wider than -max-width after splitting gets one last-resort greedy word wrap.
// The wrap is a narrow exception for the rare over-wide outlier, which reads badly in a side-by-side diff;
// it does not return to fixed-column wrapping.
// -max-width is a flag rather than a constant so the column budget can be tuned per run without a rebuild.
//
// Single-line comments that already fit, end-of-line comments, and directive comments stay unchanged.
// Comment blocks with indented example code or a Go 1.19 heading or list stay unchanged too.
// So do struct-field and interface-method doc comments, and generated files.
//
//	go run ./tools/godocreflow [-check] [-max-width N] <file_or_dir> ...
//
//	-check		print before/after blocks instead of writing files (dry run)
//	-max-width	last-resort word-wrap column budget, 0 disables it (default 100)
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
