import unittest

from semver_range import max_satisfying


class TestVersionHandling(unittest.TestCase):
    def test_highest_release_wins(self):
        self.assertEqual(max_satisfying(["1.0.0", "2.0.0", "1.5.0"], "*"), "2.0.0")

    def test_returns_element_exactly_as_given(self):
        self.assertEqual(max_satisfying(["1.2.3+build.5"], "*"), "1.2.3+build.5")

    def test_build_metadata_ignored_first_maximum_wins(self):
        self.assertEqual(max_satisfying(["1.0.0+a", "1.0.0+b"], "*"), "1.0.0+a")
        self.assertEqual(max_satisfying(["1.0.0+b", "1.0.0+a"], "*"), "1.0.0+b")

    def test_invalid_versions_skipped_silently(self):
        versions = ["v1.0.0", "1.0", "1.0.0.0", "01.0.0", "1.0.0-01", "1.x.0", "1.2.3"]
        self.assertEqual(max_satisfying(versions, ">=0.0.0"), "1.2.3")

    def test_no_match_returns_none(self):
        self.assertIsNone(max_satisfying(["1.0.0"], ">=2.0.0"))

    def test_empty_version_list_returns_none(self):
        self.assertIsNone(max_satisfying([], "*"))

    def test_prerelease_ordering_follows_semver(self):
        ordered = [
            "1.0.0-1",
            "1.0.0-2",
            "1.0.0-10",
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
                self.assertEqual(max_satisfying([lower, higher], ">=1.0.0-0"), higher)

    def test_release_is_above_its_prerelease(self):
        self.assertEqual(max_satisfying(["1.0.0", "1.0.0-rc.1"], ">=1.0.0-0"), "1.0.0")


class TestCaret(unittest.TestCase):
    def test_caret_major(self):
        self.assertEqual(max_satisfying(["1.2.3", "1.9.0", "2.0.0"], "^1.2.3"), "1.9.0")

    def test_caret_zero_major_nonzero_minor(self):
        self.assertEqual(max_satisfying(["0.2.3", "0.2.9", "0.3.0"], "^0.2.3"), "0.2.9")

    def test_caret_zero_zero_patch(self):
        self.assertEqual(max_satisfying(["0.0.3", "0.0.4"], "^0.0.3"), "0.0.3")

    def test_caret_partial_major_minor(self):
        self.assertEqual(max_satisfying(["1.1.9", "1.9.9", "2.0.0"], "^1.2"), "1.9.9")

    def test_caret_partial_zero_zero(self):
        self.assertEqual(max_satisfying(["0.0.9", "0.1.0"], "^0.0"), "0.0.9")

    def test_caret_partial_major_only(self):
        self.assertEqual(max_satisfying(["0.9.0", "1.0.0"], "^0"), "0.9.0")

    def test_caret_prerelease_lower_bound(self):
        versions = ["1.2.3-beta.1", "1.2.3-beta.2", "1.9.0"]
        self.assertEqual(max_satisfying(versions, "^1.2.3-beta.2"), "1.9.0")
        self.assertEqual(max_satisfying(["1.2.3-beta.1", "1.2.3-beta.3"], "^1.2.3-beta.2"), "1.2.3-beta.3")


class TestTilde(unittest.TestCase):
    def test_tilde_full(self):
        self.assertEqual(max_satisfying(["1.2.9", "1.3.0"], "~1.2.3"), "1.2.9")

    def test_tilde_minor_only(self):
        self.assertEqual(max_satisfying(["1.2.9", "1.3.0"], "~1.2"), "1.2.9")

    def test_tilde_major_only(self):
        self.assertEqual(max_satisfying(["1.9.0", "2.0.0"], "~1"), "1.9.0")


class TestPartialAndEquals(unittest.TestCase):
    def test_bare_partial_means_equals(self):
        self.assertEqual(max_satisfying(["1.2.9", "1.3.0"], "1.2"), "1.2.9")

    def test_equals_major_only(self):
        self.assertEqual(max_satisfying(["1.9.9", "2.0.0"], "=1"), "1.9.9")

    def test_x_wildcard_patch(self):
        self.assertEqual(max_satisfying(["1.2.9", "1.3.0"], "1.2.x"), "1.2.9")

    def test_wildcard_after_wildcard_ignores_trailing_part(self):
        self.assertEqual(max_satisfying(["1.5.0", "2.0.0"], "1.x.3"), "1.5.0")

    def test_bare_wildcards_match_any_release(self):
        for expr in ["*", "x", "X", "", "   ", "=*"]:
            with self.subTest(expr=expr):
                self.assertEqual(max_satisfying(["0.1.0", "9.9.9", "1.0.0-rc.1"], expr), "9.9.9")

    def test_full_equals_matches_only_that_version(self):
        self.assertEqual(max_satisfying(["1.2.3", "1.2.4"], "=1.2.3"), "1.2.3")


class TestOperators(unittest.TestCase):
    def test_strict_greater_than_partial(self):
        self.assertEqual(max_satisfying(["1.2.9", "1.3.0"], ">1.2"), "1.3.0")

    def test_strict_greater_than_major(self):
        self.assertEqual(max_satisfying(["1.9.9", "2.0.0"], ">1"), "2.0.0")

    def test_strict_greater_than_full_excludes_equal(self):
        self.assertIsNone(max_satisfying(["1.2.3"], ">1.2.3"))

    def test_greater_or_equal_partial(self):
        self.assertEqual(max_satisfying(["1.2.0", "1.1.9"], ">=1.2"), "1.2.0")

    def test_less_than_partial(self):
        self.assertEqual(max_satisfying(["1.1.9", "1.2.0"], "<1.2"), "1.1.9")

    def test_less_or_equal_partial(self):
        self.assertEqual(max_satisfying(["1.2.9", "1.3.0"], "<=1.2"), "1.2.9")

    def test_less_or_equal_major(self):
        self.assertEqual(max_satisfying(["1.9.9", "2.0.0"], "<=1"), "1.9.9")

    def test_whitespace_between_operator_and_version(self):
        self.assertEqual(max_satisfying(["1.2.2", "1.2.3"], ">= 1.2.3"), "1.2.3")

    def test_strict_wildcard_matches_nothing(self):
        self.assertIsNone(max_satisfying(["1.0.0"], ">*"))


class TestSetsAndHyphen(unittest.TestCase):
    def test_or_sets(self):
        self.assertEqual(max_satisfying(["1.0.0", "2.5.0", "3.1.0"], "<2.0.0 || >=3.0.0"), "3.1.0")

    def test_empty_set_means_any(self):
        self.assertEqual(max_satisfying(["0.1.0", "5.0.0"], "1.0.0 ||"), "5.0.0")

    def test_hyphen_partial_low_partial_high(self):
        self.assertEqual(max_satisfying(["1.2.5", "1.3.0", "2.0.0"], "1.2 - 1.2.9"), "1.2.5")

    def test_hyphen_full_low_partial_high(self):
        self.assertEqual(max_satisfying(["1.2.3", "2.3.9", "2.4.0"], "1.2.3 - 2.3"), "2.3.9")

    def test_hyphen_with_additional_term(self):
        self.assertEqual(max_satisfying(["1.2.5", "1.3.0"], "1.2 - 1.3.0 <1.3.0"), "1.2.5")


class TestPrereleaseRule(unittest.TestCase):
    def test_prerelease_matches_with_same_core_prerelease_comparator(self):
        self.assertEqual(max_satisfying(["1.2.3-beta.2"], ">=1.2.3-beta.1 <2.0.0"), "1.2.3-beta.2")

    def test_prerelease_of_other_core_rejected(self):
        self.assertIsNone(max_satisfying(["1.3.0-alpha"], ">=1.2.3-beta.1 <2.0.0"))

    def test_prerelease_rejected_without_prerelease_comparator(self):
        self.assertEqual(max_satisfying(["1.2.3", "1.2.4-rc.1"], ">=1.2.3"), "1.2.3")

    def test_star_never_matches_prerelease(self):
        self.assertEqual(max_satisfying(["1.0.0-alpha", "0.9.0"], "*"), "0.9.0")

    def test_sets_do_not_share_comparators(self):
        self.assertIsNone(max_satisfying(["1.2.3-beta.2"], ">=1.2.3-beta.5 <1.2.3-beta.9 || <2.0.0"))


class TestInvalidRange(unittest.TestCase):
    def test_invalid_ranges_raise_value_error(self):
        for expr in [
            ">=a.b",
            "1.2.3.4",
            "v1.2.3",
            "1.2.3 -",
            "- 1.2.3",
            ">=",
            "1.2.3-",
            "01.2.3",
            "1.0.0 || >=1.a",
        ]:
            with self.subTest(expr=expr):
                with self.assertRaises(ValueError):
                    max_satisfying(["1.0.0"], expr)

    def test_invalid_range_raises_even_without_versions(self):
        with self.assertRaises(ValueError):
            max_satisfying([], ">=a.b")


if __name__ == "__main__":
    unittest.main()
