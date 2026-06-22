## Summary

I have successfully implemented both cards in the weft batch according to the plan:

### Card 8: Fold weft config_test LoadConfig variants
- File: internal/weft/config_test.go
- Folded three separate test functions into one table-driven test TestLoadConfig with three named cases
- Preserved TestDefaultConfig and TestConfigDirs unchanged
- Commit: c808602

### Card 9: Fold push-integration duplicates, drop redundant FF pull
- File: internal/weft/weft_integration_test.go
- Folded three push/sync integration tests into TestPushIntegration table-driven test
- Dropped TestPullIntegration_FastForward (redundant)
- Commit: b5da6b0

### Results
- Test count reduced from 21 to 15
- Coverage: 64.6% (meets floor)
- All tests pass

{"status":"success","commit_sha":"b5da6b0174d20f69c5492030b2dbec4b7e921daf","session_id":"da58073c-f27e-4fad-a6c7-567708a1b78d"}