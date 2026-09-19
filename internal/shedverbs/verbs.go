// verbs.go assembles the four generic verb bodies into the one constructor batches 4 and 5 consume.

package shedverbs

import "github.com/spf13/cobra"

// Verbs returns the four generic subcommands -- run, step, status, pause, in that order -- built
// from texts and armed against spec.
//
// The two arguments carry deliberately different lifetimes, per the overview's
// build-time-texts-run-time-spec Shared Decision: cobra builds the whole command tree before any
// PersistentPreRunE runs, so help text must exist at construction time (texts, passed by value)
// while resolved paths and hooks cannot (spec, filled in place by the arming module's own
// PersistentPreRunE, after Verbs has already returned).
//
// A module needing more than the two flags the generic bodies read (status's --watch and
// --interval) decorates the returned command itself before adding it to its own subtree --
// loomcli registers --parent on the step command it got back, and lifecyclecli sets
// Args: cobra.ExactArgs(1) on the commands it needs it on. Hooks reach the module's own values by
// closure over its own receiver, never through a parameter this constructor exposes.
//
// Verbs registers no flag beyond status's --watch and --interval, and declares no Args constraint
// of its own.
//
// This package exposes no Command()/RunCLI seam: it is not a CLI module and is not counted in the
// CLI/Cobra Invariant's module tally.
func Verbs(texts VerbTexts, spec *Spec) []*cobra.Command {
	return []*cobra.Command{
		runCmd(texts, spec),
		stepCmd(texts, spec),
		statusCmd(texts, spec),
		pauseCmd(texts, spec),
	}
}
