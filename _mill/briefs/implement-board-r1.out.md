Excellent! All work is complete. Let me now verify the test count summary:

**Summary of Test Consolidation (Board Package):**

All tests pass with 62.5% coverage (meets the 62.5% floor requirement).

**Test count reduction:**
- Started with 62 top-level test functions in the baseline
- Now have 37 top-level test functions
- Reduction: 25 functions folded into subtests (40% reduction)

**Key folds completed:**

1. **Card 1 (render_test.go):** 16 → 6 functions
   - Folded 8 single-shape render tests into 2 tables: TestRenderProposalAndShapesHomepage (7 cases) and TestRenderStatusVariants (5 cases)
   - Folded single-task tests into TestRenderSingleTask and TestRenderOrphanDetection
   - Folded sidebar tests into TestRenderSidebarExtendedTitle
   - Dropped TestRenderTaskStatus (subset of TestRenderStatusVariants)

2. **Card 2 (cli_test.go):** 8 → 2 functions
   - Folded 5 happy-path verbs into TestCLIContract (with 5 subtests)
   - Folded 3 error/edge cases into TestCLIErrorAndEdgeCases (with 3 subtests)

3. **Card 3 (board_test.go):** 5 → 5 functions (simplified)
   - Simplified TestUpsertTask to facade-unique assertions only
   - Dropped TestRemoveTask (error owned by store layer)
   - Kept TestRerender and all 4 HealthCheck tests

4. **Card 4 (config_test.go):** 8 → 2 functions
   - Folded 6 LoadConfig variants into TestLoadConfig (6 subtests)
   - Folded 2 Outputs functions into TestOutputs (2 subtests)

5. **Card 5 (init_test.go):** 4 → 2 functions
   - Folded 3 first-run checks into TestInitFirstRun (3 subtests)
   - Kept TestInitIdempotent separate

7636e2869f0770cf3195c6476778ab37db146e03