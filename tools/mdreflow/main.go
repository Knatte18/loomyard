// Command mdreflow reflows markdown prose and list-item paragraphs to one sentence per line.
// It adds breaks at internal clause boundaries:
// a semicolon, or a comma before a conjunction that opens a clause with its own subject.
// It changes line breaks only and never rewords, reorders, or drops content.
// Before writing a file, it checks the transformed text against the original.
// Both must match once all whitespace is collapsed to single spaces.
// A file that fails the check is left untouched and reported.
//
//	go run ./tools/mdreflow [-dry-run] <path-or-glob> [...]
//	go run ./tools/mdreflow $(find . -name '*.md' -not -path './.git/*')
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "report what would change without writing files")
	flag.Parse()

	patterns := flag.Args()
	if len(patterns) == 0 {
		fmt.Fprintln(os.Stderr, "usage: mdreflow [-dry-run] <path-or-glob> [...]")
		os.Exit(2)
	}

	var paths []string
	for _, p := range patterns {
		matches, err := filepath.Glob(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mdreflow:", err)
			os.Exit(1)
		}
		if len(matches) == 0 {
			paths = append(paths, p)
		} else {
			paths = append(paths, matches...)
		}
	}

	changed, unchanged := 0, 0
	var mismatches []string

	for _, p := range paths {
		status, err := processFile(p, *dryRun)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mdreflow:", err)
			os.Exit(1)
		}
		switch status {
		case "mismatch":
			mismatches = append(mismatches, p)
			fmt.Printf("MISMATCH (left untouched): %s\n", p)
		case "changed":
			changed++
			verb := "changed"
			if *dryRun {
				verb = "would change"
			}
			fmt.Printf("%s: %s\n", verb, p)
		default:
			unchanged++
		}
	}

	fmt.Printf("\n%d changed, %d unchanged, %d mismatches\n", changed, unchanged, len(mismatches))
	if len(mismatches) > 0 {
		fmt.Println("Files with content mismatches (NOT written):")
		for _, m := range mismatches {
			fmt.Printf("  %s\n", m)
		}
		os.Exit(1)
	}
}

func processFile(path string, dryRun bool) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	original := string(data)
	transformed := reflowText(original)

	if collapsed(transformed) != collapsed(original) {
		return "mismatch", nil
	}
	if transformed == original {
		return "unchanged", nil
	}

	if !dryRun {
		mode := os.FileMode(0o644)
		if info, statErr := os.Stat(path); statErr == nil {
			mode = info.Mode()
		}
		if err := os.WriteFile(path, []byte(transformed), mode); err != nil {
			return "", err
		}
	}
	return "changed", nil
}
