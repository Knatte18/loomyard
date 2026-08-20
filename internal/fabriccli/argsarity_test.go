// argsarity_test.go pins every fabric verb's declared positional-argument arity. It is a pure
// cobra-tree inspection with no git spawn and no hub fixture, per the Test Tier Purity Invariant —
// the end-to-end per-verb assertions that need a real hub live in the integration-tagged
// cli_test.go instead.

package fabriccli_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
)

// TestCommand_EveryVerbRejectsExtraPositionalArgs proves each fabric verb declares an Args validator
// that refuses arguments it does not consume.
//
// This is fabric's R8 finding L3. Only `diff` set an Args validator; every other command left Args
// nil, which cobra defaults to ArbitraryArgs for a leaf command, so extra positional arguments were
// accepted and silently dropped. Observed live: `lyx fabric unwire bogus-typo` performed the full
// unwire and exited 0, and `lyx fabric commit <some-file>` committed the whole configured pathspec
// while ignoring the named file, reporting success either way. The usage strings advertise an arity
// the binary did not enforce.
//
// The check is on the declared validator rather than on a driven invocation, deliberately: most of
// these verbs need a real hub to reach their RunE at all, and the defect was the absence of the
// declaration itself.
func TestCommand_EveryVerbRejectsExtraPositionalArgs(t *testing.T) {
	t.Parallel()

	// tooMany is one more argument than each verb legitimately takes, so a verb with a correct
	// validator refuses it. Keyed by command name; every subcommand of `lyx fabric` must appear,
	// which is what stops a newly added verb from quietly inheriting ArbitraryArgs.
	tooMany := map[string][]string{
		"add":       {"a", "b"},
		"remove":    {"a", "b"},
		"checkout":  {"a", "b"},
		"diff":      {"a", "b"},
		"list":      {"a"},
		"pairs":     {"a"},
		"reconcile": {"a"},
		"prune":     {"a"},
		"cleanup":   {"a"},
		"unwire":    {"a"},
		"status":    {"a"},
		"commit":    {"a"},
		"push":      {"a"},
		"pull":      {"a"},
		"sync":      {"a"},
		"merge":     {"a", "b"},
		"merge-in":  {"a", "b"},
	}

	// handRolledArity names the verbs that refuse a wrong argument COUNT in their own handler rather
	// than through a cobra Args validator, with the reason. clone's own check rejects both too-few and
	// too-many and carries a fuller usage message than cobra's generic "accepts at most 2 arg(s)", so
	// adding a validator there would replace a better error with a worse one.
	handRolledArity := map[string]string{
		"clone": "clone.go's own len(args) != 1 && != 2 check refuses both directions with the full usage line",
	}

	root := fabriccli.Command()
	var seen int
	for _, sub := range root.Commands() {
		name := sub.Name()
		if name == "help" || name == "completion" {
			continue
		}
		if reason, exempt := handRolledArity[name]; exempt {
			seen++
			t.Logf("verb %q is exempt from the Args-validator requirement: %s", name, reason)
			continue
		}
		args, ok := tooMany[name]
		if !ok {
			t.Errorf("verb %q has no entry in tooMany or handRolledArity; add one so its argument arity is pinned", name)
			continue
		}
		seen++
		t.Run(name, func(t *testing.T) {
			if sub.Args == nil {
				t.Fatalf("verb %q declares no Args validator; extra positional arguments would be accepted and silently ignored", name)
			}
			if err := sub.Args(sub, args); err == nil {
				t.Errorf("verb %q accepted %d positional arguments; want a refusal", name, len(args))
			}
		})
	}
	if want := len(tooMany) + len(handRolledArity); seen != want {
		t.Errorf("walked %d verbs; the two tables name %d — they and the cobra tree have drifted", seen, want)
	}
}
