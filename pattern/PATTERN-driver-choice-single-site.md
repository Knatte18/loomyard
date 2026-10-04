# PATTERN-driver-choice-single-site

A *recorded* seed driver value is read in exactly one place per recipe, that recipe's own bootstrap verb.
The branch on it selects the run's driving surface — which driver spawns and whether the session carries the loom status strand — and nothing else.

- No producer, no generic verb and no engine reads the recorded value, and no code path gates *behaviour* on it.
- One carve-out, and only this shape: a CLI verb may compare a driver value the operator **just typed** against the addressed run's recorded one and refuse on the envelope when they disagree (`internal/battencli`'s `refuseAdoptedSeed`).
  It selects no spawn and changes no behaviour, and it is the loud command-line failure this entry prefers over silently discarding the typed flag.
  A refusal decided by the recorded value alone, with nothing typed to compare it against, is not this shape and stays barred.
- The entry governs the **read, not the vocabulary**.
  The driver constants are legitimately named at the seeding sites, which validate a flag before a seed exists; those sites validate an argument rather than reading a written seed.

## Permitted constant consumers

- The shed CLI's `seed` command.
- Batten's own flag validation in `internal/battencli/arm.go`.
- Batten's child-driver param reader in `internal/battencli/wire.go`.
- Loom's own seed writer in `internal/loomcli/sharedbootstrap.go`.

The last two are constant consumers, not readers: batten's reads a `child_driver` *param* and loom's *writes* the field.
`internal/battencli/arm.go` is on the list twice over: its flag validation is a constant consumer, and its `refuseAdoptedSeed` is the one carve-out above, which does read the field.

## Rationale

The recorded value is a startup choice.
A second reader introduces silent divergence between what a run was seeded as and what it is doing, unobservable from either the status file or the envelope.
A flag validator fails loudly at the command line, where a mistake is visible immediately.

## Enforcement

A **tripwire, not a completeness proof**.
`internal/loomcli/bootstrap_test.go` asserts that the only production readers of the seed's driver **field** outside the `shedrun` package are `internal/loomcli` and the functions named in its own `driverFieldReadCarveOuts`.
The carve-outs are keyed by file and enclosing function, so a second reader in a carved-out file still fails.
The scan covers the field selector rather than the driver constants.
Adding a reader fails it and forces a human to confirm, and each carve-out carries its justification beside the entry.
