// Command wordswap performs a case-preserving whole-token substitution of one word for another
// across files of any language: identifiers, comments, string literals, shell variables, and
// markdown prose all substitute through the same mechanism.
// Its safety invariant is reversibility over recorded spans -- reverting exactly the recorded
// substitution offsets must reproduce the input byte-for-byte, and a file failing that check is
// left untouched and reported.
// `host` + lowercase at a token start is not guessed at: it is reported as AMBIGUOUS so the
// operator can hand-edit it or name it in `-skip`.
//
//	go run ./tools/wordswap -from host -to warp [-dry-run] [-skip <regexp>]... <path-or-glob> [...]
//	go run ./tools/wordswap -from host -to warp -skip 'pane hosting an idle agent' internal/fabricengine/*.go
//
// Last run: 2026-08-09, host->warp sweep of the fabric-host-to-warp-rename task.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// skipList accumulates repeated `-skip <regexp>` flag values into a compiled regexp slice.
type skipList []*regexp.Regexp

// String implements flag.Value by rendering the accumulated patterns for -help output.
func (s *skipList) String() string {
	if s == nil {
		return ""
	}
	patterns := make([]string, len(*s))
	for i, re := range *s {
		patterns[i] = re.String()
	}
	return fmt.Sprint(patterns)
}

// Set implements flag.Value by compiling and appending one -skip regexp value.
func (s *skipList) Set(value string) error {
	re, err := regexp.Compile(value)
	if err != nil {
		return fmt.Errorf("invalid -skip regexp %q: %w", value, err)
	}
	*s = append(*s, re)
	return nil
}

func main() {
	from := flag.String("from", "", "the word to replace (required)")
	to := flag.String("to", "", "the replacement word (required)")
	dryRun := flag.Bool("dry-run", false, "report what would change without writing files")
	var skips skipList
	flag.Var(&skips, "skip", "regexp matching a line to leave unchanged and report as deliberately skipped (repeatable)")
	flag.Parse()

	patterns := flag.Args()
	if *from == "" || *to == "" || len(patterns) == 0 {
		fmt.Fprintln(os.Stderr, "usage: wordswap -from <word> -to <word> [-dry-run] [-skip <regexp>]... <path-or-glob> [...]")
		os.Exit(2)
	}

	var paths []string
	for _, p := range patterns {
		matches, err := filepath.Glob(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "wordswap:", err)
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
	var ambiguous []pathOccurrence
	var skippedList []pathOccurrence

	for _, p := range paths {
		status, result, err := processFile(p, *from, *to, skips, *dryRun)
		if err != nil {
			fmt.Fprintln(os.Stderr, "wordswap:", err)
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
		for _, occ := range result.Ambiguous {
			ambiguous = append(ambiguous, pathOccurrence{path: p, occurrence: occ})
		}
		for _, occ := range result.Skipped {
			skippedList = append(skippedList, pathOccurrence{path: p, occurrence: occ})
		}
	}

	fmt.Printf("\n%d changed, %d unchanged, %d mismatches\n", changed, unchanged, len(mismatches))

	fmt.Println("\nAMBIGUOUS (unresolved):")
	for _, po := range ambiguous {
		fmt.Printf("  %s:%d: %s\n", po.path, po.occurrence.Line, po.occurrence.Text)
	}
	fmt.Println("\nSKIPPED (deliberate):")
	for _, po := range skippedList {
		fmt.Printf("  %s:%d: %s\n", po.path, po.occurrence.Line, po.occurrence.Text)
	}

	if len(mismatches) > 0 || len(ambiguous) > 0 {
		os.Exit(1)
	}
}

// pathOccurrence pairs an Occurrence with the file path it came from, for the report's
// "<path>:<line>: <line text>" entries.
type pathOccurrence struct {
	path       string
	occurrence Occurrence
}

// processFile reads the file at path, runs swapText over its contents, and -- unless dryRun is
// set or the reversibility check failed -- writes the result back preserving the file's existing
// mode, exactly as tools/mdreflow/main.go's processFile does.
// It returns the status string "mismatch", "changed", or "unchanged", plus the underlying Result
// for the caller's report buckets.
func processFile(path string, from, to string, skips []*regexp.Regexp, dryRun bool) (string, Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", Result{}, err
	}
	original := string(data)

	result, err := swapText(original, from, to, skips)
	if err != nil {
		return "", Result{}, err
	}

	if result.Mismatch {
		return "mismatch", result, nil
	}
	if result.Out == original {
		return "unchanged", result, nil
	}

	if !dryRun {
		mode := os.FileMode(0o644)
		if info, statErr := os.Stat(path); statErr == nil {
			mode = info.Mode()
		}
		if err := os.WriteFile(path, []byte(result.Out), mode); err != nil {
			return "", Result{}, err
		}
	}
	return "changed", result, nil
}
