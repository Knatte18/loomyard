# Report: semver range resolver

Files: `semver_range.py` (`max_satisfying`), `test_semver_range.py` (unittest suite).

## Status

All tests in `test_semver_range.py` pass.
Nothing in the spec was left unimplemented.

## Ambiguities and choices

- **Build identifiers.**
  The spec gives no grammar for `BUILD`.
  I accept any non-empty dot-separated run of `[0-9A-Za-z-]`, including leading zeros, as semver 2.0.0 does.
- **Build metadata in a range.**
  `>=1.2.3+build` is accepted; the build part is ignored, as it is for ordering.
- **Whitespace after `^` and `~`.**
  The spec allows whitespace only after operator comparators.
  I also allow it after `^` and `~`, so `^ 1.2.3` means `^1.2.3`.
- **Wildcard followed by a concrete part.**
  The spec says everything after a wildcard is a wildcard, so `1.x.3` is read as `1.x.x`.
  The trailing `3` is ignored rather than rejected.
- **Wildcard and missing parts in `^` and `~`.**
  I treat them alike, so `^0.x` equals `^0` (`<1.0.0`) and `~1.x` equals `~1` (`<2.0.0`).
- **Strict operators on a bare wildcard.**
  `>*` and `<*` are not specified.
  I make them match nothing, as node-semver does.
  `=*`, `>=*` and `<=*` match any release.
- **Hyphen range edges.**
  A wildcard or missing low part is completed with zeros, so `* - 1.2.3` means `>=0.0.0 <=1.2.3`.
  A wildcard or partial high part sets only an exclusive upper bound, and `*` as the high part sets none.
  A full low part with a prerelease is used as written.
- **Hyphen range inside a set.**
  The spec lists the hyphen range among the terms, so `1.0.0 - 2.0.0 <1.5.0` is one set of two terms.
  A `-` token that is not in the middle of `A - B` raises `ValueError`.
- **Partial versions with a prerelease.**
  The spec defines partial versions without a prerelease, so `>=1.2-beta` raises `ValueError`.
- **Full version without spaces.**
  `1.2.3-2.0.0` parses as one full version with prerelease `2.0.0`, not as a hyphen range, since the spec requires spaces around the hyphen.
- **Empty sets.**
  An empty set in `||` matches like `*`, so `1.0.0 ||` accepts every release.
- **Prerelease rule with expansion.**
  "At least one comparator after expansion" is read literally.
  `=1.2.3-beta` expands to `>=1.2.3-beta` and `<=1.2.3-beta`, and both carry the prerelease.
- **Equal maxima.**
  Ties are broken by list order: the first matching entry wins.
- **Invalid range checked first.**
  `max_satisfying` parses the range before reading `versions`, so an invalid range raises even when the list is empty.

## Known gaps

- Non-string entries in `versions` are not handled.
  They raise a `TypeError` from the regex match and are not skipped, although the spec says invalid entries are skipped.
- The tests are hand-picked cases, not generated or property-based.
  No ordering was checked against an independent semver implementation.
- Parsing is regex-based and re-runs for each call, with no caching across calls.

## Test output

The command below was run from the repository root, and its output was captured as written.
```
$ python3 -m unittest discover -s .scratch/haikubench/out-e -p 'test_semver_range.py' -v
test_caret_major (test_semver_range.TestCaret.test_caret_major) ... ok
test_caret_partial_major_minor (test_semver_range.TestCaret.test_caret_partial_major_minor) ... ok
test_caret_partial_major_only (test_semver_range.TestCaret.test_caret_partial_major_only) ... ok
test_caret_partial_zero_zero (test_semver_range.TestCaret.test_caret_partial_zero_zero) ... ok
test_caret_prerelease_lower_bound (test_semver_range.TestCaret.test_caret_prerelease_lower_bound) ... ok
test_caret_zero_major_nonzero_minor (test_semver_range.TestCaret.test_caret_zero_major_nonzero_minor) ... ok
test_caret_zero_zero_patch (test_semver_range.TestCaret.test_caret_zero_zero_patch) ... ok
test_invalid_range_raises_even_without_versions (test_semver_range.TestInvalidRange.test_invalid_range_raises_even_without_versions) ... ok
test_invalid_ranges_raise_value_error (test_semver_range.TestInvalidRange.test_invalid_ranges_raise_value_error) ... ok
test_greater_or_equal_partial (test_semver_range.TestOperators.test_greater_or_equal_partial) ... ok
test_less_or_equal_major (test_semver_range.TestOperators.test_less_or_equal_major) ... ok
test_less_or_equal_partial (test_semver_range.TestOperators.test_less_or_equal_partial) ... ok
test_less_than_partial (test_semver_range.TestOperators.test_less_than_partial) ... ok
test_strict_greater_than_full_excludes_equal (test_semver_range.TestOperators.test_strict_greater_than_full_excludes_equal) ... ok
test_strict_greater_than_major (test_semver_range.TestOperators.test_strict_greater_than_major) ... ok
test_strict_greater_than_partial (test_semver_range.TestOperators.test_strict_greater_than_partial) ... ok
test_strict_wildcard_matches_nothing (test_semver_range.TestOperators.test_strict_wildcard_matches_nothing) ... ok
test_whitespace_between_operator_and_version (test_semver_range.TestOperators.test_whitespace_between_operator_and_version) ... ok
test_bare_partial_means_equals (test_semver_range.TestPartialAndEquals.test_bare_partial_means_equals) ... ok
test_bare_wildcards_match_any_release (test_semver_range.TestPartialAndEquals.test_bare_wildcards_match_any_release) ... ok
test_equals_major_only (test_semver_range.TestPartialAndEquals.test_equals_major_only) ... ok
test_full_equals_matches_only_that_version (test_semver_range.TestPartialAndEquals.test_full_equals_matches_only_that_version) ... ok
test_wildcard_after_wildcard_ignores_trailing_part (test_semver_range.TestPartialAndEquals.test_wildcard_after_wildcard_ignores_trailing_part) ... ok
test_x_wildcard_patch (test_semver_range.TestPartialAndEquals.test_x_wildcard_patch) ... ok
test_prerelease_matches_with_same_core_prerelease_comparator (test_semver_range.TestPrereleaseRule.test_prerelease_matches_with_same_core_prerelease_comparator) ... ok
test_prerelease_of_other_core_rejected (test_semver_range.TestPrereleaseRule.test_prerelease_of_other_core_rejected) ... ok
test_prerelease_rejected_without_prerelease_comparator (test_semver_range.TestPrereleaseRule.test_prerelease_rejected_without_prerelease_comparator) ... ok
test_sets_do_not_share_comparators (test_semver_range.TestPrereleaseRule.test_sets_do_not_share_comparators) ... ok
test_star_never_matches_prerelease (test_semver_range.TestPrereleaseRule.test_star_never_matches_prerelease) ... ok
test_empty_set_means_any (test_semver_range.TestSetsAndHyphen.test_empty_set_means_any) ... ok
test_hyphen_full_low_partial_high (test_semver_range.TestSetsAndHyphen.test_hyphen_full_low_partial_high) ... ok
test_hyphen_partial_low_partial_high (test_semver_range.TestSetsAndHyphen.test_hyphen_partial_low_partial_high) ... ok
test_hyphen_with_additional_term (test_semver_range.TestSetsAndHyphen.test_hyphen_with_additional_term) ... ok
test_or_sets (test_semver_range.TestSetsAndHyphen.test_or_sets) ... ok
test_tilde_full (test_semver_range.TestTilde.test_tilde_full) ... ok
test_tilde_major_only (test_semver_range.TestTilde.test_tilde_major_only) ... ok
test_tilde_minor_only (test_semver_range.TestTilde.test_tilde_minor_only) ... ok
test_build_metadata_ignored_first_maximum_wins (test_semver_range.TestVersionHandling.test_build_metadata_ignored_first_maximum_wins) ... ok
test_empty_version_list_returns_none (test_semver_range.TestVersionHandling.test_empty_version_list_returns_none) ... ok
test_highest_release_wins (test_semver_range.TestVersionHandling.test_highest_release_wins) ... ok
test_invalid_versions_skipped_silently (test_semver_range.TestVersionHandling.test_invalid_versions_skipped_silently) ... ok
test_no_match_returns_none (test_semver_range.TestVersionHandling.test_no_match_returns_none) ... ok
test_prerelease_ordering_follows_semver (test_semver_range.TestVersionHandling.test_prerelease_ordering_follows_semver) ... ok
test_release_is_above_its_prerelease (test_semver_range.TestVersionHandling.test_release_is_above_its_prerelease) ... ok
test_returns_element_exactly_as_given (test_semver_range.TestVersionHandling.test_returns_element_exactly_as_given) ... ok

----------------------------------------------------------------------
Ran 45 tests in 0.001s

OK
```

## Timing

start: 1791470075.331877366
stop: 1791470161.037618595
duration: 85.7
