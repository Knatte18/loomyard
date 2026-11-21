// verifyreport.go holds the verify-failure record, the plan-level verify's record of its latest failed evaluation.
// It also holds the findings text it renders.
// A card hint names which cards may have broken a failing package.

package runplan

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Knatte18/loomyard/internal/gateslot"
)

// verifyGateReportFileName is the verify-failure record's file name inside the reports dir.
const verifyGateReportFileName = "verify-gate.yaml"

// VerifyGateReportPath returns the verify-failure record's path inside reportsDir.
func VerifyGateReportPath(reportsDir string) string {
	return filepath.Join(reportsDir, verifyGateReportFileName)
}

// VerifyGateReport is the plan-level verify's record of its latest failed evaluation.
type VerifyGateReport struct {
	// Attempt is the failed evaluation's number, counted from one.
	Attempt int `yaml:"attempt"`
	// Cap is the gate entry's re-prompt budget, so a failed evaluation's number can run one past it.
	Cap int `yaml:"cap"`
	// Command is the verify command the failed evaluation ran.
	// It is empty for a run-level entry's failure and for a dirty tree found before the command was rendered.
	Command string `yaml:"command,omitempty"`
	// ExitCode is the failed verify command's exit code, zero for a failure that ran no command.
	ExitCode int `yaml:"exit_code,omitempty"`
	// Dirty lists the paths of a clean-tree failure, which runs no verify and so has no Failures or LogPath.
	Dirty []string `yaml:"dirty,omitempty"`
	// TimedOut names the gate tier and the timeout it outlived, and is empty when no tier timed out.
	// Such a failure has no Failures and carries LogTail instead.
	TimedOut string `yaml:"timed_out,omitempty"`
	// LogTail is the bounded tail of the verify log of a timed-out or hung run.
	// It is also the tail for a failed run whose log named no failing identity.
	LogTail string `yaml:"log_tail,omitempty"`
	// Hang is the marker, log line or `exit code <n>` that read the failed run as a hang, and empty for any other failure.
	// Such a failure carries the Failures its log parses to and LogTail.
	Hang string `yaml:"hang,omitempty"`
	// HangStacks is set when the hung run's log holds goroutine stacks.
	HangStacks bool `yaml:"hang_stacks,omitempty"`
	// WatchdogWindow is the CPU watchdog's busy window when it quit a test binary, as a duration string.
	WatchdogWindow string `yaml:"watchdog_window,omitempty"`
	// Failures lists the failing identities the verify output parsed to.
	Failures []VerifyFailure `yaml:"failures,omitempty"`
	// LogPath is the full verify log.
	LogPath string `yaml:"log_path,omitempty"`
	// Hint lists the NN-slug labels of the cards whose commits touched a failing package, in trail order.
	Hint []string `yaml:"hint,omitempty"`
	// FixCommits lists the fixers' commits above the pre-fix head, oldest first.
	FixCommits []string `yaml:"fix_commits,omitempty"`
	// Fixer names the fixer this failure goes to, in the implementer's own vocabulary.
	Fixer string `yaml:"fixer,omitempty"`
	// Entry names the run-level gate entry, other than the verify entry, whose failure this report records.
	// It is empty for a verify failure.
	Entry string `yaml:"entry,omitempty"`
	// Findings is the failed Entry's own findings text, and is empty for a verify failure.
	Findings string `yaml:"findings,omitempty"`
}

// WriteVerifyGateReport serializes r to path, replacing any earlier file atomically.
func WriteVerifyGateReport(path string, r VerifyGateReport) error {
	data, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("runplan: marshal verify-gate report for %s: %w", path, err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("runplan: create verify-gate report dir %s: %w", dir, err)
	}
	if err := WriteFileAtomic(dir, path, data); err != nil {
		return fmt.Errorf("runplan: write verify-gate report %s: %w", path, err)
	}
	return nil
}

// CardHint returns the labels, in trail order, of the cards that touched a failing package's directory.
// A card touches it when its commit changes a file directly in that directory.
// shas and labels are parallel: labels[i] is the card whose commit is shas[i].
// changedPaths lists the repository-relative paths one commit changed.
// A failing package maps to a worktree directory through modulePath, the module path of go.mod.
// An opaque identity, or a package outside the module, contributes no directory, so it contributes no hint.
// The hint claims only that a card touched a failing package;
// whoever fixes the failure judges it with the plan in hand.
func CardHint(modulePath string, failures []VerifyFailure, shas, labels []string, changedPaths func(sha string) ([]string, error)) ([]string, error) {
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
			return nil, fmt.Errorf("runplan: changed paths of %s: %w", sha, err)
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

// LogTail returns the last bytes of output a verify-failure record carries, trimmed.
func LogTail(output string) string {
	if len(output) > verifyGateLogTailBytes {
		output = output[len(output)-verifyGateLogTailBytes:]
	}
	return strings.TrimSpace(output)
}

// RenderVerifyGateFindings returns the findings text of r.
// The text opens with wayForward, the caller's way forward to the report's fixer.
// It continues with the evaluation number and the re-prompt limit.
// It then gives the named entry's findings.
// Otherwise it gives the dirty paths, the timeout sentence, a hang or the failing identities.
// A failure that named no test gives the exit code, the log path and the log tail.
// It has no identity list and no card hint.
// A timeout carries the log path and tail.
// A hang carries its matched marker, identities, log path and tail.
// Identities carry their output tail, the full log path and the hint.
// A timeout or hang says the step was not re-run.
// A watchdog quit also names the window and the operator's way forward.
func RenderVerifyGateFindings(r VerifyGateReport, wayForward string) string {
	var b strings.Builder
	b.WriteString(wayForward + "\n")
	fmt.Fprintf(&b, "Verify gate failed (evaluation %d; the session may be re-prompted %d time(s)).\n", r.Attempt, r.Cap)

	if r.Entry != "" {
		fmt.Fprintf(&b, "The run-level %q entry failed:\n%s\n", r.Entry, r.Findings)
		return b.String()
	}

	if len(r.Dirty) > 0 {
		b.WriteString("The worktree is not clean, so no verify ran. Commit or remove these paths:\n")
		for _, p := range r.Dirty {
			fmt.Fprintf(&b, "- %s\n", p)
		}
		return b.String()
	}

	if r.TimedOut != "" {
		fmt.Fprintf(&b, "The verify command timed out: %s; the step was not re-run.\n", r.TimedOut)
		writeLogLocation(&b, r)
		return b.String()
	}

	if r.Hang != "" {
		if r.HangStacks {
			b.WriteString("The verify step hung and was not re-run; its goroutine stacks are in the log.\n")
		} else {
			fmt.Fprintf(&b, "The verify step was killed or exited with %s and was not re-run.\n", r.Hang)
		}
		if r.WatchdogWindow != "" {
			fmt.Fprintf(&b, "The gate test's CPU watchdog quit a test binary that kept a core busy for %s.\n", r.WatchdogWindow)
			fmt.Fprintf(&b, "A package that is CPU-bound but healthy needs the operator to %s; that is not yours to do.\n", gateslot.BusyQuitWayForward)
		}
		fmt.Fprintf(&b, "Matched: %s\n", r.Hang)
		writeFailingIdentities(&b, r.Failures)
		writeLogLocation(&b, r)
		return b.String()
	}

	if len(r.Failures) == 0 {
		fmt.Fprintf(&b, "The verify command exited with code %d and named no failing test.\n", r.ExitCode)
		writeLogLocation(&b, r)
		return b.String()
	}

	writeFailingIdentities(&b, r.Failures)
	writeLogLocation(&b, r)
	if len(r.Hint) > 0 {
		fmt.Fprintf(&b, "Cards whose commits touched a failing package: %s\n", strings.Join(r.Hint, ", "))
	} else {
		b.WriteString("No recorded card touched a failing package.\n")
	}
	return b.String()
}

// writeFailingIdentities writes the failing-identities list, each identity with its output tail indented under it.
func writeFailingIdentities(b *strings.Builder, failures []VerifyFailure) {
	b.WriteString("Failing identities:\n")
	for _, f := range failures {
		fmt.Fprintf(b, "- %s (%s)\n", f.ID, f.Kind)
		if f.Tail != "" {
			for _, line := range strings.Split(f.Tail, "\n") {
				fmt.Fprintf(b, "    %s\n", line)
			}
		}
	}
}

// writeLogLocation writes the full log path and the log tail, each only when the report has it.
func writeLogLocation(b *strings.Builder, r VerifyGateReport) {
	if r.LogPath != "" {
		fmt.Fprintf(b, "Full log: %s\n", r.LogPath)
	}
	if r.LogTail != "" {
		fmt.Fprintf(b, "Log tail:\n%s\n", r.LogTail)
	}
}
