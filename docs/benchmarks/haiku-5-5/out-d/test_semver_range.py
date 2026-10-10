import unittest

from semver_range import max_satisfying


class SpecExamples(unittest.TestCase):
    def test_caret_major(self):
        self.assertEqual(max_satisfying(["1.2.3", "1.9.0", "2.0.0"], "^1.2.3"), "1.9.0")

    def test_caret_zero_major(self):
        self.assertEqual(max_satisfying(["0.2.3", "0.2.9", "0.3.0"], "^0.2.3"), "0.2.9")

    def test_prerelease_excluded_without_prerelease_comparator(self):
        self.assertEqual(max_satisfying(["1.2.3", "1.2.4-rc.1"], ">=1.2.3"), "1.2.3")

    def test_prerelease_included_with_same_triple(self):
        self.assertEqual(max_satisfying(["1.2.4-rc.1", "1.2.4-rc.2"], ">=1.2.4-rc.1"), "1.2.4-rc.2")

    def test_or_sets(self):
        self.assertEqual(max_satisfying(["1.0.0", "2.5.0", "3.1.0"], "<2.0.0 || >=3.0.0"), "3.1.0")

    def test_hyphen_partial_low(self):
        self.assertEqual(max_satisfying(["1.2.5", "1.3.0", "2.0.0"], "1.2 - 1.2.9"), "1.2.5")

    def test_invalid_range_raises(self):
        with self.assertRaises(ValueError):
            max_satisfying(["1.0.0"], ">=a.b")


class Ordering(unittest.TestCase):
    def test_numeric_not_lexical(self):
        self.assertEqual(max_satisfying(["1.2.9", "1.10.0", "1.9.0"], "*"), "1.10.0")

    def test_release_above_prerelease(self):
        self.assertEqual(max_satisfying(["1.0.0-rc.1", "1.0.0"], ">=1.0.0-rc.1"), "1.0.0")

    def test_numeric_identifiers_compare_numerically(self):
        self.assertEqual(max_satisfying(["1.0.0-rc.10", "1.0.0-rc.9"], ">=1.0.0-rc.1"), "1.0.0-rc.10")

    def test_numeric_identifier_below_alphanumeric(self):
        self.assertEqual(max_satisfying(["1.0.0-alpha", "1.0.0-1"], ">=1.0.0-0"), "1.0.0-alpha")

    def test_alphanumeric_ascii_order(self):
        self.assertEqual(max_satisfying(["1.0.0-beta", "1.0.0-Beta"], ">=1.0.0-0"), "1.0.0-beta")

    def test_more_identifiers_higher(self):
        self.assertEqual(max_satisfying(["1.0.0-alpha.1", "1.0.0-alpha"], ">=1.0.0-alpha"), "1.0.0-alpha.1")

    def test_build_ignored_first_maximum_wins(self):
        self.assertEqual(max_satisfying(["1.0.0+b", "1.0.0+a", "0.9.0"], "*"), "1.0.0+b")

    def test_build_equal_for_equals_operator(self):
        self.assertEqual(max_satisfying(["1.0.0+build.5"], "=1.0.0"), "1.0.0+build.5")


class VersionValidity(unittest.TestCase):
    def test_invalid_versions_skipped(self):
        versions = ["v1.0.0", "1.0", "01.0.0", "1.0.0-01", "1.0.0-", "x", "1.0.0"]
        self.assertEqual(max_satisfying(versions, "*"), "1.0.0")

    def test_returns_original_string(self):
        self.assertEqual(max_satisfying(["2.0.0+meta"], "2"), "2.0.0+meta")

    def test_no_match_returns_none(self):
        self.assertIsNone(max_satisfying(["1.0.0"], ">=2"))

    def test_empty_list_returns_none(self):
        self.assertIsNone(max_satisfying([], "*"))


class Wildcards(unittest.TestCase):
    def test_empty_range_is_any(self):
        self.assertEqual(max_satisfying(["1.0.0", "3.0.0"], "   "), "3.0.0")

    def test_empty_set_in_or_is_any(self):
        self.assertEqual(max_satisfying(["1.0.0", "3.0.0"], "<2 || "), "3.0.0")

    def test_star_never_matches_prerelease(self):
        self.assertEqual(max_satisfying(["1.0.0", "2.0.0-rc.1"], "*"), "1.0.0")

    def test_x_minor(self):
        self.assertEqual(max_satisfying(["1.2.0", "1.2.9", "1.3.0"], "1.2.x"), "1.2.9")

    def test_wildcard_propagates(self):
        self.assertEqual(max_satisfying(["1.0.0", "1.5.3", "2.0.0"], "1.X.3"), "1.5.3")

    def test_bare_major(self):
        self.assertEqual(max_satisfying(["1.0.0", "1.9.9", "2.0.0"], "=1"), "1.9.9")


class Operators(unittest.TestCase):
    def test_greater_than_partial_minor(self):
        self.assertIsNone(max_satisfying(["1.2.9"], ">1.2"))
        self.assertEqual(max_satisfying(["1.2.9", "1.3.0"], ">1.2"), "1.3.0")

    def test_greater_than_partial_major(self):
        self.assertIsNone(max_satisfying(["1.9.9"], ">1"))
        self.assertEqual(max_satisfying(["1.9.9", "2.0.0"], ">1"), "2.0.0")

    def test_less_equal_partial(self):
        self.assertEqual(max_satisfying(["1.2.9", "1.3.0"], "<=1.2"), "1.2.9")
        self.assertEqual(max_satisfying(["1.9.0", "2.0.0"], "<=1"), "1.9.0")

    def test_less_than_partial(self):
        self.assertEqual(max_satisfying(["1.1.9", "1.2.0"], "<1.2"), "1.1.9")

    def test_less_than_excludes_lower_bound_prerelease(self):
        self.assertEqual(max_satisfying(["1.1.0", "1.2.0-rc.1"], "<1.2"), "1.1.0")

    def test_greater_equal_partial(self):
        self.assertIsNone(max_satisfying(["1.1.9"], ">=1.2"))

    def test_strict_full(self):
        self.assertIsNone(max_satisfying(["1.2.3"], ">1.2.3"))
        self.assertEqual(max_satisfying(["1.2.3"], "<=1.2.3"), "1.2.3")

    def test_whitespace_after_operator(self):
        self.assertEqual(max_satisfying(["1.2.3", "1.5.0", "2.0.0"], ">= 1.2.3 < 2"), "1.5.0")

    def test_and_within_set(self):
        self.assertEqual(max_satisfying(["1.0.0", "1.4.0", "1.6.0"], ">1.0.0 <1.5.0"), "1.4.0")


class CaretTilde(unittest.TestCase):
    def test_caret_zero_zero_patch(self):
        self.assertEqual(max_satisfying(["0.0.3", "0.0.4"], "^0.0.3"), "0.0.3")

    def test_caret_partials(self):
        self.assertEqual(max_satisfying(["1.1.0", "1.9.0", "2.0.0"], "^1.2"), "1.9.0")
        self.assertEqual(max_satisfying(["0.0.9", "0.1.0"], "^0.0"), "0.0.9")
        self.assertEqual(max_satisfying(["0.9.9", "1.0.0"], "^0"), "0.9.9")

    def test_caret_with_prerelease(self):
        versions = ["1.2.3-beta.1", "1.2.3-beta.3", "1.5.0-rc.1"]
        self.assertEqual(max_satisfying(versions, "^1.2.3-beta.2"), "1.2.3-beta.3")

    def test_prerelease_other_triple_excluded(self):
        self.assertIsNone(max_satisfying(["1.3.0-alpha"], ">=1.2.3-beta.1 <2.0.0"))

    def test_tilde(self):
        self.assertEqual(max_satisfying(["1.2.3", "1.2.9", "1.3.0"], "~1.2.3"), "1.2.9")
        self.assertEqual(max_satisfying(["1.2.0", "1.3.0"], "~1.2"), "1.2.0")
        self.assertEqual(max_satisfying(["1.9.0", "2.0.0"], "~1"), "1.9.0")


class Hyphen(unittest.TestCase):
    def test_full_upper_inclusive(self):
        self.assertEqual(max_satisfying(["1.2.0", "2.3.4", "2.3.5"], "1.2 - 2.3.4"), "2.3.4")

    def test_partial_upper_exclusive(self):
        self.assertEqual(max_satisfying(["2.3.9", "2.4.0"], "1.2.3 - 2.3"), "2.3.9")
        self.assertEqual(max_satisfying(["2.9.9", "3.0.0"], "1 - 2"), "2.9.9")

    def test_lower_bound(self):
        self.assertIsNone(max_satisfying(["1.2.2"], "1.2.3 - 2"))


class PrereleasePerSet(unittest.TestCase):
    def test_sets_do_not_share_comparators(self):
        self.assertIsNone(max_satisfying(["1.2.4-rc.1"], ">=1.2.4-rc.0 <1.0.0 || >=1.0.0"))

    def test_prerelease_matched_by_own_set(self):
        self.assertEqual(max_satisfying(["1.2.4-rc.1"], "<1.0.0 || >=1.2.4-rc.0"), "1.2.4-rc.1")


class InvalidRanges(unittest.TestCase):
    def test_invalid(self):
        for expr in [">=a.b", "^", ">=", "1.2.3 -", "- 1.2.3", "1.2.3.4", "01.2",
                     "1.x-beta", ">>1", "v1.2.3", "1.2 - 2 - 3", "^~1.2"]:
            with self.subTest(expr=expr), self.assertRaises(ValueError):
                max_satisfying(["1.0.0"], expr)

    def test_invalid_range_raises_with_empty_versions(self):
        with self.assertRaises(ValueError):
            max_satisfying([], ">=a.b")


if __name__ == "__main__":
    unittest.main()
