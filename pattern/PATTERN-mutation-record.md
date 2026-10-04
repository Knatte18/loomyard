# PATTERN-mutation-record

Every mutating fabric verb accumulates a `*Mutations` record, and every mutating result type exposes it under a fixed envelope key set.

- An executor appends its primitive only after it observably changed state.
- Every mutating result type embeds `MutationRecord`; a read-only one must not.
- The envelope carries `mutations`, always an array, and `partial`, always a bool.
- Pre-flight failures emit a bare `output.Err` with neither key.
