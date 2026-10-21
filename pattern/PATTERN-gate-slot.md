# PATTERN-gate-slot

Running a gate build or test holds `internal/gateslot` slots, so the hub never runs more at once than its configured count.

- A gate build or test runs only holding `internal/gateslot` slots, in one of two places.
- Inside a `lyx gate test` tier run, holding the slots its tier asks for, with the tier's config setting its parallelism and environment.
- In one of the non-tier holders `internal/gateslot`'s doc lists, holding one slot and setting its own `-trimpath` Go flag, plus `-p` from `tier0`'s parallel when the hub declares `tier0`.
- Every gate build keeps `-trimpath`, so a fresh worktree reuses the build and test cache and no code a gate run builds locates the repo root from a `runtime.Caller` path alone (`PATTERN-trimpath-safe-root`).
