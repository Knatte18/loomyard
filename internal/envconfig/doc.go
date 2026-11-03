// Package envconfig owns the hub-wide env.yaml: the environment variables lym exports to every process it starts for the hub.
//
// The module's one key, `vars`, maps a variable name to its value.
// It is a per-name map classified machine.
// The hub file in the board dir holds the values every machine shares.
// The never-tracked machine file `<BoltDir>/.lym/config/env.machine.yaml` holds this machine's own.
// The machine value wins name by name.
// The local layer refuses it.
//
// The gate's tiers and shared build, the Drift capture and compare commands, and every agent session's pane get the variables.
// Precedence, most specific first: a gate tier's or build's own `env`, or a Drift step's, then this module, then the environment lym inherits.
// The .NET build-server switches lym forces, `proc.BuildServerOffEnv`, stay lym's own in gate runs and strand panes, and a module variable of such a name is dropped there.
//
// LoadConfig degrades to the template when the hub holds no env.yaml, so a hub not yet reconciled exports nothing.
// It refuses a name that is no environment variable name, `PATH` and a name starting `LYM_`, in any letter case.
// The prefix names lym's own variables, and `PATH` carries the directory a strand pane's prelude puts first so that `lym` resolves to its spawning binary.
// LoadConfig never resolves cwd itself: the caller hands it the directories (see PATTERN-cwd-resolution).
package envconfig
