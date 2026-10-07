// verifygatereport.go implements the per-run report of the webster verify gate's failed evaluations,
// the touch-based card hint that names which cards may have broken a failing package,
// and the findings text the gate returns to Merriam.

package websterengine

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// verifyGateReportFileName is the verify-gate report's file name inside the reports dir.
const verifyGateReportFileName = "verify-gate.yaml"

// VerifyGateReportPath returns the verify-gate report's path inside reportsDir.
func VerifyGateReportPath(reportsDir string) string {
	return filepath.Join(reportsDir, verifyGateReportFileName)
}

// VerifyGateReport is the gate's record of its latest failed evaluation.
type VerifyGateReport struct {
	// Attempt is the failed evaluation's number, counted from one.
	Attempt int `yaml:"attempt"`
	// Cap is the gate's attempt cap.
	Cap int `yaml:"cap"`
	// Dirty lists the paths of a clean-tree failure, which runs no verify and so has no Failures or LogPath.
	Dirty []string `yaml:"dirty,omitempty"`
	// TimedOut is the verify timeout as a duration string when the verify outlived it and was killed, and empty otherwise.
	// Such a failure has no Failures and carries LogTail instead.
	TimedOut string `yaml:"timed_out,omitempty"`
	// LogTail is the bounded tail of the verify log of a timed-out run.
	LogTail string `yaml:"log_tail,omitempty"`
	// Failures lists the failing identities the verify output parsed to.
	Failures []VerifyFailure `yaml:"failures,omitempty"`
	// LogPath is the full verify log.
	LogPath string `yaml:"log_path,omitempty"`
	// Hint lists the NN-slug labels of the cards whose commits touched a failing package, in trail order.
	Hint []string `yaml:"hint,omitempty"`
	// FixCommits lists the fixer fork's commits, oldest first.
	FixCommits []string `yaml:"fix_commits,omitempty"`
}

// WriteVerifyGateReport serializes r to path, replacing any earlier file atomically.
func WriteVerifyGateReport(path string, r VerifyGateReport) error {
	data, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("websterengine: marshal verify-gate report for %s: %w", path, err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("websterengine: create verify-gate report dir %s: %w", dir, err)
	}
	if err := writeFileAtomic(dir, path, data); err != nil {
		return fmt.Errorf("websterengine: write verify-gate report %s: %w", path, err)
	}
	return nil
}

// cardHint returns the labels, in trail order, whose commit changed a file directly in the directory of a failing package.
// shas and labels are the parallel trail accumulatedCardSHAs returns.
// changedPaths lists the repository-relative paths one commit changed.
// A failing package maps to a worktree directory through modulePath, go.mod's module path.
// An opaque identity, or a package outside the module, contributes no directory, so it contributes no hint.
// The hint claims only that a card touched a failing package;
// Merriam judges it with the plan in hand.
func cardHint(modulePath string, failures []VerifyFailure, shas, labels []string, changedPaths func(sha string) ([]string, error)) ([]string, error) {
	dirs := map[string]bool{}
	for _, f := range failures {
		if dir, ok := moduleRelDir(modulePath, f.Package); ok {
			dirs[dir] = true
		}
	}
	if len(dirs) == 0 {
		return nil, nil
	}

	var hint []string
	seen := map[string]bool{}
	for i, sha := range shas {
		if seen[labels[i]] {
			continue
		}
		paths, err := changedPaths(sha)
		if err != nil {
			return nil, fmt.Errorf("websterengine: changed paths of %s: %w", sha, err)
		}
		for _, p := range paths {
			if dirs[path.Dir(p)] {
				seen[labels[i]] = true
				hint = append(hint, labels[i])
				break
			}
		}
	}
	return hint, nil
}

// moduleRelDir maps the import path pkg to its slash-separated directory relative to the module root.
// It reports false for an empty pkg or one outside the module.
func moduleRelDir(modulePath, pkg string) (string, bool) {
	if modulePath == "" || pkg == "" {
		return "", false
	}
	if pkg == modulePath {
		return ".", true
	}
	rel, ok := strings.CutPrefix(pkg, modulePath+"/")
	if !ok || rel == "" {
		return "", false
	}
	return rel, true
}

// verifyGateLogTailBytes bounds the log tail a timed-out run's report carries.
const verifyGateLogTailBytes = 4096

// logTail returns the last verifyGateLogTailBytes of output, trimmed.
func logTail(output string) string {
	if len(output) > verifyGateLogTailBytes {
		output = output[len(output)-verifyGateLogTailBytes:]
	}
	return strings.TrimSpace(output)
}

// renderVerifyGateFindings returns the findings text the gate returns to Merriam:
// the attempt and the cap, then either the dirty paths, the timeout sentence with the log path and tail, or each failing identity with its output tail, the full log path, and the hint.
func renderVerifyGateFindings(r VerifyGateReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Verify gate failed (attempt %d of %d).\n", r.Attempt, r.Cap)

	if len(r.Dirty) > 0 {
		b.WriteString("The worktree is not clean, so no verify ran. Commit or remove these paths:\n")
		for _, p := range r.Dirty {
			fmt.Fprintf(&b, "- %s\n", p)
		}
		return b.String()
	}

	if r.TimedOut != "" {
		fmt.Fprintf(&b, "The verify command did not finish within %s and was killed.\n", r.TimedOut)
		if r.LogPath != "" {
			fmt.Fprintf(&b, "Full log: %s\n", r.LogPath)
		}
		if r.LogTail != "" {
			fmt.Fprintf(&b, "Log tail:\n%s\n", r.LogTail)
		}
		return b.String()
	}

	b.WriteString("Failing identities:\n")
	for _, f := range r.Failures {
		fmt.Fprintf(&b, "- %s (%s)\n", f.ID, f.Kind)
		if f.Tail != "" {
			for _, line := range strings.Split(f.Tail, "\n") {
				fmt.Fprintf(&b, "    %s\n", line)
			}
		}
	}
	if r.LogPath != "" {
		fmt.Fprintf(&b, "Full log: %s\n", r.LogPath)
	}
	if len(r.Hint) > 0 {
		fmt.Fprintf(&b, "Cards whose commits touched a failing package: %s\n", strings.Join(r.Hint, ", "))
	} else {
		b.WriteString("No recorded card touched a failing package.\n")
	}
	return b.String()
}
