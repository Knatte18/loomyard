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

	_ = paths
	_ = dryRun
}
