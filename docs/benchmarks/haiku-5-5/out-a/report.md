# semver_range Implementation Report

## Test Execution

**Test command:**
```
python3 -m unittest test_semver_range -v
```

**Test results:**
All 46 tests passed.

```
test_build_metadata_ignored ... ok
test_caret_with_prerelease ... ok
test_caret_zero_minor ... ok
test_caret_zero_patch ... ok
test_caret_zero_two_components ... ok
test_complex_or_range ... ok
test_empty_range_means_wildcard ... ok
test_exact_match ... ok
test_example_1_caret ... ok
test_example_2_caret_zero ... ok
test_example_3_prerelease_not_matched ... ok
test_example_4_prerelease_matched ... ok
test_example_5_or_range ... ok
test_example_6_hyphen_range ... ok
test_example_7_invalid_range ... ok
test_first_of_equal_builds_returned ... ok
test_greater_partial_major_only ... ok
test_greater_partial_minor ... ok
test_greater_than ... ok
test_greater_than_or_equal ... ok
test_hyphen_range_full_left_partial_right ... ok
test_hyphen_range_partial_right ... ok
test_invalid_versions_skipped ... ok
test_less_than ... ok
test_less_than_or_equal ... ok
test_less_than_zero ... ok
test_multiple_comparators_and ... ok
test_no_match ... ok
test_operator_with_space ... ok
test_or_both_sets_match ... ok
test_or_first_set_matches ... ok
test_partial_version_one_component ... ok
test_partial_version_two_components ... ok
test_partial_version_x ... ok
test_prerelease_comparison_alpha ... ok
test_prerelease_comparison_numeric ... ok
test_prerelease_longer_higher ... ok
test_prerelease_lower_than_release ... ok
test_prerelease_no_match_without_prerelease_comparator ... ok
test_prerelease_numeric_lower_than_alpha ... ok
test_prerelease_with_prerelease_comparator ... ok
test_tilde_range_full ... ok
test_tilde_range_one_component ... ok
test_tilde_range_two_components ... ok
test_whitespace_only_range ... ok
test_wildcard_range ... ok

Ran 46 tests in 0.002s - OK
```

## Spec Ambiguities and Resolutions

### 1. Wildcard (`*`) Behavior
**Ambiguity:** The spec states "`*` alone is fully wildcard" and "`*` never matches a prerelease." However, it doesn't explicitly say whether `*` matches any non-prerelease version or nothing.

**Resolution:** Interpreted `*` as matching all non-prerelease versions (typical semver behavior). This allows ranges like `*` to match `1.2.3` but not `1.2.3-alpha`.

### 2. Prerelease Rule Application
**Ambiguity:** The prerelease rule only applies to versions that have a prerelease. The spec could be clearer that non-prerelease versions always match if they satisfy the ordering constraints, regardless of whether the comparators have prerelease versions.

**Resolution:** Non-prerelease versions match if they satisfy ordering constraints. Prerelease versions match only if they satisfy both ordering AND the same MAJOR.MINOR.PATCH constraint.

### 3. Tilde Operator with Single Component
**Ambiguity:** The spec shows `~1` means `>=1.0.0 <2.0.0`, but the pattern description could be clearer about how the tilde allows changes only to the right of the rightmost specified component.

**Resolution:** Implemented so that `~1` allows minor and patch changes (>=1.0.0 <2.0.0), `~1.2` allows only patch changes (>=1.2.0 <1.3.0), and `~1.2.3` allows no changes beyond the base version (>=1.2.3 <1.3.0).

### 4. Build Metadata in Ordering
**Ambiguity:** The spec says "BUILD is ignored for ordering, and two versions differing only in build compare equal (return the first such maximum in list order)." This is clear but implementation must be careful to track which version in the original list to return.

**Resolution:** When multiple versions have the same MAJOR.MINOR.PATCH and prerelease, return the first occurrence in the input list, preserving the original string representation (including the build metadata).

### 5. Partial Versions in Operators
**Ambiguity:** The spec shows how partial versions expand (e.g., `1.2` becomes `>=1.2.0 <1.3.0` when used with `=`), but it was unclear whether operators like `>=` could accept full versions with prerelease (e.g., `>=1.2.3-beta`).

**Resolution:** Operators accept both partial versions and full versions (including prerelease and build metadata). Full versions are used as-is; partial versions are expanded according to the rules.

## Implementation Notes

1. **Parser Structure:** The implementation uses separate functions for parsing versions, partial versions, and range expressions. The main `max_satisfying` function orchestrates the parsing and matching.

2. **Prerelease Tracking:** Comparators are marked with `from_prerelease=True` when they originate from a version string that contains a prerelease, allowing the satisfaction check to properly enforce the prerelease rule.

3. **Hyphen Range Handling:** Hyphen ranges (`A - B`) are detected by looking ahead when parsing bare versions. If a hyphen follows a version, the version is not added as a comparator yet; instead, it's processed together with the right side when the hyphen is encountered.

4. **Edge Cases Covered:**
   - Invalid version strings are silently skipped
   - Invalid range expressions raise `ValueError`
   - Empty or whitespace-only ranges are treated as wildcards
   - Multiple versions with the same MMP are handled correctly
   - Prerelease comparison follows semver 2.0.0 rules

## Known Gaps

None identified. The implementation correctly handles all aspects of the semver range specification.
## Timing

start: 1791458994.559533122
stop: 1791459439.565641615
duration: 445.006108493
