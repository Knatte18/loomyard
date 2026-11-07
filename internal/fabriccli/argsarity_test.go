// argsarity_test.go pins every fabric verb's declared positional-argument arity.
// It is a pure cobra-tree inspection with no git spawn and no hub fixture, per the Test Tier Purity Invariant.
// The end-to-end per-verb assertions that need a real hub live in the tier1 cli_test.go instead.

package fabriccli_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestCommand_EveryVerbRejectsExtraPositionalArgs proves each fabric verb declares an Args validator.
// The validator refuses arguments the verb does not consume.
//
// A command with no Args validator gets cobra's ArbitraryArgs for a leaf command.
// Extra positional arguments are then accepted and silently dropped.
//
// The check reads the declared validator rather than driving an invocation.
// Most verbs need a real hub to reach their RunE at all.
func TestCommand_EveryVerbRejectsExtraPositionalArgs(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	// tooMany is one more argument than each verb legitimately takes,
	// so a verb with a correct validator refuses it.
	// Keyed by command name; every subcommand of `lym fabric` must appear.
	// Otherwise a newly added verb would quietly inherit ArbitraryArgs.
	tooMany := map[string][]string{
		"add":             {"a", "b"},
		"remove":          {"a", "b"},
		"remote-teardown": {"a"},
		"shortname":       {"a", "b"},
		"checkout":        {"a", "b"},
		"diff":            {"a", "b"},
		"list":            {"a"},
		"pairs":           {"a"},
		"reconcile":       {"a"},
		"anchor":          {"a", "b"},
		"prune":           {"a"},
		"cleanup":         {"a"},
		"unwire":          {"a"},
		"status":          {"a"},
		"commit":          {"a"},
		"push":            {"a"},
		"pull":            {"a"},
		"sync":            {"a"},
		"merge":           {"a", "b"},
		"merge-in":        {"a", "b"},
	}

	// variadicArity names the verbs that legitimately take an unbounded number of positional arguments.
	// For them "one more than it consumes" does not exist, so the tooMany table cannot express their arity.
	// Their arity is pinned from the other direction: each must refuse too few arguments.
	// Omitting such a verb from every table would let it inherit ArbitraryArgs unnoticed.
	variadicArity := map[string]string{
		"merge-stage": "takes one or more conflicted paths; refusing zero is the only arity it can pin",
	}

	// handRolledArity names the verbs that refuse a wrong argument count in their own handler.
	// They do not go through a cobra Args validator; the map gives the reason each one does so.
	// clone's own check rejects both too few and too many, with a fuller usage line than cobra's generic one.
	// A validator there would replace a better error with a worse one.
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
		if reason, variadic := variadicArity[name]; variadic {
			seen++
			t.Run(name, func(t *testing.T) {
				if sub.Args == nil {
					t.Fatalf("verb %q declares no Args validator; it would accept zero positional arguments and act on nothing (%s)", name, reason)
				}
				if err := sub.Args(sub, nil); err == nil {
					t.Errorf("verb %q accepted zero positional arguments; want a refusal (%s)", name, reason)
				}
				if err := sub.Args(sub, []string{"a", "b", "c"}); err != nil {
					t.Errorf("verb %q refused three positional arguments (%v); want them accepted (%s)", name, err, reason)
				}
			})
			continue
		}
		args, ok := tooMany[name]
		if !ok {
			t.Errorf("verb %q has no entry in tooMany, handRolledArity or variadicArity; add one so its argument arity is pinned", name)
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
	if want := len(tooMany) + len(handRolledArity) + len(variadicArity); seen != want {
		t.Errorf("walked %d verbs; the three tables name %d — they and the cobra tree have drifted", seen, want)
	}
}
