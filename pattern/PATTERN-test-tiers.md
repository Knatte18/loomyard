# PATTERN-test-tiers

Running a hub's tests goes through `lyx gate test` over the tiers `gate.yaml` declares, so every run takes its slots, its timeout and its log.

- Tests run only through `lyx gate test` over `gate.yaml`'s tiers, apart from a card `**Verify:**` line and `cmd/testtiming`'s runs in every mode.
- A card `**Verify:**` line runs through the shared `internal/cardverify` runner, and `cmd/testtiming` measures outside the gate because its modes measure what a tier run hides.
- A site or stencil names a gate's tier list and scope form, never a test command or target list.
- The shell guard refuses a command with a prefix in `gate.yaml`'s `refuse_raw`, so an agent's raw test run is steered to the gate.
- The plan gate's `verify-raw-command` finding flags a card `**Verify:**` line that starts with a `refuse_raw` prefix.
