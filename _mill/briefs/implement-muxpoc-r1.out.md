## Implementation Summary

Implemented all three cards in the muxpoc batch:

### Card 14: Fold layoutChecksum tests
- Folded into table-driven TestLayoutChecksum with pinned-value rows and a format row; original names preserved as subtests.

### Card 15: Fold socketName and env-filtering tests
- Folded TestSocketName/TestSocketNameStability into TestSocketName; folded TestSanitizeEnv/TestStrippedEnvKeys into TestEnvFiltering.

### Card 16: Fold muxpoc CLI error tests
- Folded three CLI error tests into table-driven TestRunCLIErrors.

### Verification Results
- All tests pass; coverage 33.0% (meets floor).

{"status":"success","commit_sha":"831a16cb10666761acce1b66f2f100a92edf68f0","session_id":"6cb05201-e7b0-4f33-8e1a-ef962694c05e"}