import unittest

from semver_range import max_satisfying


class SpecExamplesTest(unittest.TestCase):
    def test_caret_picks_highest_version_in_same_major(self):
        self.assertEqual(max_satisfying(["1.2.3", "1.9.0", "2.0.0"], "^1.2.3"), "1.9.0")

    def test_caret_with_zero_minor_pins_the_minor(self):
        self.assertEqual(max_satisfying(["0.2.3", "0.2.9", "0.3.0"], "^0.2.3"), "0.2.9")

    def test_release_beats_prerelease_of_same_core(self):
        self.assertEqual(max_satisfying(["1.2.3", "1.2.4-rc.1"], ">=1.2.3"), "1.2.3")

    def test_later_prerelease_beats_earlier_one(self):
        self.assertEqual(max_satisfying(["1.2.4-rc.1", "1.2.4-rc.2"], ">=1.2.4-rc.1"), "1.2.4-rc.2")

    def test_or_of_sets_accepts_either_side(self):
        self.assertEqual(max_satisfying(["1.0.0", "2.5.0", "3.1.0"], "<2.0.0 || >=3.0.0"), "3.1.0")

    def test_hyphen_range_with_partial_upper_bound_excludes_next_minor(self):
        self.assertEqual(max_satisfying(["1.2.5", "1.3.0", "2.0.0"], "1.2 - 1.2.9"), "1.2.5")

    def test_invalid_range_raises_value_error(self):
        with self.assertRaises(ValueError):
            max_satisfying(["1.0.0"], ">=a.b")


class VersionEntriesTest(unittest.TestCase):
    def test_no_match_returns_none(self):
        self.assertIsNone(max_satisfying(["1.0.0"], "^2"))

    def test_empty_version_list_returns_none(self):
        self.assertIsNone(max_satisfying([], "*"))

    def test_invalid_entries_are_skipped(self):
        versions = ["v9.0.0", "01.0.0", "1.2", "1.2.3-01", "1.2.3-", "1.2.3.4", " 2.0.0", "1.2.3"]
        self.assertEqual(max_satisfying(versions, "*"), "1.2.3")

    def test_non_string_entries_are_skipped(self):
        self.assertEqual(max_satisfying([None, 5, "1.0.0"], "*"), "1.0.0")

    def test_returned_string_is_the_entry_exactly_as_given(self):
        self.assertEqual(max_satisfying(["1.2.3+meta"], "1.2.3"), "1.2.3+meta")

    def test_build_metadata_is_ignored_and_first_equal_entry_wins(self):
        self.assertEqual(max_satisfying(["1.0.0+b", "1.0.0+a"], "*"), "1.0.0+b")

    def test_build_metadata_on_prerelease_does_not_change_precedence(self):
        self.assertEqual(max_satisfying(["1.0.0-rc.1+build.5", "1.0.0-rc.1"], ">=1.0.0-rc.1"), "1.0.0-rc.1+build.5")

    def test_build_metadata_is_accepted_in_a_range(self):
        self.assertEqual(max_satisfying(["1.9.0", "2.0.0"], "^1.2.3+build.1"), "1.9.0")


class OrderingTest(unittest.TestCase):
    def test_prerelease_identifiers_follow_semver_precedence(self):
        ordered = [
            "1.0.0-2",
            "1.0.0-alpha",
            "1.0.0-alpha.1",
            "1.0.0-alpha.beta",
            "1.0.0-beta",
            "1.0.0-beta.2",
            "1.0.0-beta.11",
            "1.0.0-rc.1",
            "1.0.0",
        ]
        for lower, higher in zip(ordered, ordered[1:]):
            with self.subTest(lower=lower, higher=higher):
                self.assertEqual(max_satisfying([higher, lower], f">={lower}"), higher)

    def test_maximum_of_shuffled_list_is_the_release(self):
        versions = ["1.0.0-beta.11", "1.0.0", "1.0.0-alpha", "1.0.0-rc.1", "1.0.0-beta.2"]
        self.assertEqual(max_satisfying(versions, ">=1.0.0-alpha"), "1.0.0")

    def test_numeric_identifiers_compare_numerically_not_as_text(self):
        self.assertEqual(max_satisfying(["1.0.0-beta.9", "1.0.0-beta.10"], ">=1.0.0-beta.1"), "1.0.0-beta.10")


class ComparatorTest(unittest.TestCase):
    def test_bare_partial_means_its_whole_minor_range(self):
        self.assertEqual(max_satisfying(["1.2.0", "1.2.9", "1.3.0"], "1.2"), "1.2.9")

    def test_equals_major_only_means_its_whole_major_range(self):
        self.assertEqual(max_satisfying(["1.0.0", "1.9.9", "2.0.0"], "=1"), "1.9.9")

    def test_greater_than_partial_excludes_its_whole_range(self):
        self.assertIsNone(max_satisfying(["1.2.0", "1.2.9"], ">1.2"))

    def test_greater_than_major_only_starts_at_next_major(self):
        self.assertEqual(max_satisfying(["1.9.9", "2.0.0"], ">1"), "2.0.0")
        self.assertIsNone(max_satisfying(["1.9.9"], ">1"))

    def test_less_than_partial_excludes_its_whole_range(self):
        self.assertIsNone(max_satisfying(["1.2.0", "1.2.9"], "<1.2"))

    def test_less_or_equal_partial_includes_its_whole_range(self):
        self.assertEqual(max_satisfying(["1.2.9", "1.3.0"], "<=1.2"), "1.2.9")

    def test_less_or_equal_major_only_includes_its_whole_range(self):
        self.assertEqual(max_satisfying(["1.9.9", "2.0.0"], "<=1"), "1.9.9")

    def test_whitespace_between_operator_and_version_is_allowed(self):
        self.assertEqual(max_satisfying(["1.2.2", "1.2.3"], ">= 1.2.3"), "1.2.3")
        self.assertIsNone(max_satisfying(["1.2.2"], ">= 1.2.3"))

    def test_full_version_equals_is_exact(self):
        self.assertEqual(max_satisfying(["1.2.3", "1.2.4"], "=1.2.3"), "1.2.3")

    def test_full_version_less_than_is_plain(self):
        self.assertEqual(max_satisfying(["1.2.3", "1.2.4"], "<1.2.4"), "1.2.3")

    def test_star_matches_every_release(self):
        self.assertEqual(max_satisfying(["0.1.0", "9.9.9"], "*"), "9.9.9")

    def test_x_wildcard_in_minor_position_matches_its_major(self):
        self.assertEqual(max_satisfying(["1.5.0", "2.0.0"], "1.x"), "1.5.0")
        self.assertEqual(max_satisfying(["1.5.0", "2.0.0"], "X"), "2.0.0")

    def test_component_after_wildcard_that_is_not_wildcard_is_invalid(self):
        with self.assertRaises(ValueError):
            max_satisfying(["1.5.0"], "1.x.3")

    def test_greater_than_star_matches_nothing(self):
        self.assertIsNone(max_satisfying(["1.0.0", "2.0.0"], ">*"))

    def test_empty_range_matches_any_release(self):
        self.assertEqual(max_satisfying(["1.0.0", "2.0.0-rc.1"], ""), "1.0.0")

    def test_whitespace_only_range_matches_any_release(self):
        self.assertEqual(max_satisfying(["1.0.0", "3.0.0"], "   "), "3.0.0")

    def test_empty_set_in_union_matches_any_release(self):
        self.assertEqual(max_satisfying(["1.0.0", "3.0.0"], "9.0.0 ||"), "3.0.0")


class CaretTest(unittest.TestCase):
    def test_caret_zero_zero_patch_pins_the_patch(self):
        self.assertEqual(max_satisfying(["0.0.3", "0.0.4"], "^0.0.3"), "0.0.3")

    def test_caret_major_only_pins_the_major(self):
        self.assertEqual(max_satisfying(["1.9.0", "2.0.0"], "^1"), "1.9.0")

    def test_caret_zero_major_only_pins_the_major_zero(self):
        self.assertEqual(max_satisfying(["0.9.9", "1.0.0"], "^0"), "0.9.9")

    def test_caret_zero_minor_partial_pins_the_minor(self):
        self.assertEqual(max_satisfying(["0.0.9", "0.1.0"], "^0.0"), "0.0.9")

    def test_caret_with_prerelease_keeps_prerelease_lower_bound(self):
        self.assertEqual(
            max_satisfying(["1.2.3-beta.1", "1.2.3-beta.2", "2.0.0"], "^1.2.3-beta.2"),
            "1.2.3-beta.2",
        )

    def test_caret_with_prerelease_excludes_lower_prerelease(self):
        self.assertIsNone(max_satisfying(["1.2.3-beta.1"], "^1.2.3-beta.2"))


class TildeTest(unittest.TestCase):
    def test_tilde_full_version_keeps_the_minor(self):
        self.assertEqual(max_satisfying(["1.2.9", "1.3.0"], "~1.2.3"), "1.2.9")

    def test_tilde_partial_minor_keeps_the_minor(self):
        self.assertEqual(max_satisfying(["1.2.9", "1.3.0"], "~1.2"), "1.2.9")

    def test_tilde_major_only_keeps_the_major(self):
        self.assertEqual(max_satisfying(["1.9.9", "2.0.0"], "~1"), "1.9.9")


class HyphenTest(unittest.TestCase):
    def test_full_upper_bound_is_inclusive(self):
        self.assertEqual(max_satisfying(["1.2.3", "2.3.4", "2.3.5"], "1.2 - 2.3.4"), "2.3.4")

    def test_partial_upper_bound_excludes_the_next_minor(self):
        self.assertEqual(max_satisfying(["2.3.9", "2.4.0"], "1.2.3 - 2.3"), "2.3.9")

    def test_partial_lower_bound_is_completed_with_zeros(self):
        self.assertIsNone(max_satisfying(["1.1.9"], "1.2 - 2.0.0"))

    def test_hyphen_without_spaces_is_invalid(self):
        with self.assertRaises(ValueError):
            max_satisfying(["1.2.3"], "1.2.3 -2.0.0")


class PrereleaseRuleTest(unittest.TestCase):
    def test_prerelease_on_other_core_is_rejected(self):
        self.assertIsNone(max_satisfying(["1.3.0-alpha"], ">=1.2.3-beta.1 <2.0.0"))

    def test_prerelease_on_same_core_as_a_prerelease_comparator_is_accepted(self):
        self.assertEqual(max_satisfying(["1.2.3-beta.2", "1.3.0-alpha"], ">=1.2.3-beta.1 <2.0.0"), "1.2.3-beta.2")

    def test_star_never_matches_a_prerelease(self):
        self.assertIsNone(max_satisfying(["1.0.0-rc.1"], "*"))

    def test_prerelease_rule_does_not_share_comparators_across_sets(self):
        self.assertIsNone(
            max_satisfying(["1.3.0-alpha.2"], ">=1.0.0 <2.0.0 || >=1.3.0-alpha.5 <=1.3.0-alpha.9"),
        )


class InvalidRangeTest(unittest.TestCase):
    def test_malformed_ranges_raise_value_error(self):
        invalid_ranges = [
            "1.2.3 &&",
            "^",
            "~",
            ">=",
            "1.2.3 -",
            "1.2.3 - 2.0.0 - 3.0.0",
            ">= 1.2.3 - 2.0.0",
            "1.2.3-01",
            "1.2-beta",
            "1.2.3+",
            "=1.2.3.4",
            "1..2",
            "v1.2.3",
            "1.2.3 | 2.0.0",
            "|||",
        ]
        for range_expr in invalid_ranges:
            with self.subTest(range_expr=range_expr):
                with self.assertRaises(ValueError):
                    max_satisfying(["1.0.0"], range_expr)

    def test_invalid_range_raises_even_with_no_versions(self):
        with self.assertRaises(ValueError):
            max_satisfying([], ">=a.b")

    def test_non_string_range_raises_value_error(self):
        with self.assertRaises(ValueError):
            max_satisfying(["1.0.0"], None)


if __name__ == "__main__":
    unittest.main()
