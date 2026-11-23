// Command godocreflow reflows the text of Go doc comments to semantic line breaks.
// It puts each sentence on its own line, and it breaks again at an internal independent-clause boundary.
// It reflows file-level header comments, package doc comments, and comments before an exported declaration.
//
// It is a pure mechanical transform.
// It only re-breaks existing comment text onto different lines; it never adds, drops, or reorders a word.
// It finds comments with go/parser and go/ast, not with a regex over raw text;
// so a "//" inside a string literal is never taken for a comment.
//
// It lays lines out by the rules `lym loom lint-comments` checks, so it adds no wrapped-break finding there.
// Each break it makes is one those rules accept, or one the comment already had between the same two words.
// A line whose text reaches the rules' length limit is split at such a break.
// A paragraph that no such break brings under the limit stays as written.
// The run names every prose line that still reaches the limit for a hand rewrite.
//
// Single-line comments under the limit, end-of-line comments, and generated files stay unchanged.
// So do comment groups holding a directive, and struct-field and interface-method doc comments.
// Paragraphs of indented example code, headings or lists stay as written;
// the prose paragraphs around them are reflowed.
//
//	go run ./tools/godocreflow [-check] <file_or_dir> ...
//
//	-check	print before/after blocks instead of writing files (dry run)
package main

import (
	"flag"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"sort"

	"github.com/Knatte18/loomyard/internal/commentlint"
)

func main() {
	check := flag.Bool("check", false, "print before/after blocks instead of writing files (dry run)")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: godocreflow [-check] <file_or_dir> ...")
		os.Exit(2)
	}

	files, err := collectGoFiles(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "godocreflow:", err)
		os.Exit(1)
	}

	changed := 0
	longLines := 0
	for _, path := range files {
		srcBytes, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "godocreflow:", err)
			os.Exit(1)
		}
		src := string(srcBytes)

		fset := token.NewFileSet()
		edits, longLineNumbers, err := collectEdits(fset, path, src, commentlint.LineLengthLimit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "SKIP (parse error) %s: %v\n", path, err)
			continue
		}
		reportLongLines(path, longLineNumbers)
		longLines += len(longLineNumbers)
		if len(edits) == 0 {
			continue
		}
		changed++
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
	if longLines > 0 {
		fmt.Fprintf(os.Stderr, "%d prose line(s) reach %d characters; rewrite them by hand\n", longLines, commentlint.LineLengthLimit)
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

// reportLongLines prints each given line number of path as a prose line that reaches the length limit.
func reportLongLines(path string, lineNumbers []int) {
	for _, lineNumber := range lineNumbers {
		fmt.Fprintf(os.Stderr, "%s:%d: line reaches %d characters\n", path, lineNumber, commentlint.LineLengthLimit)
	}
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
