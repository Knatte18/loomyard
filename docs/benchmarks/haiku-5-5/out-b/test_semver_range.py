import unittest

from semver_range import max_satisfying


class MaxSatisfyingTests(unittest.TestCase):
    def test_caret_keeps_major(self):
        self.assertEqual(max_satisfying(["1.2.3", "1.9.0", "2.0.0"], "^1.2.3"), "1.9.0")

    def test_caret_zero_minor_keeps_minor(self):
        self.assertEqual(max_satisfying(["0.2.3", "0.2.9", "0.3.0"], "^0.2.3"), "0.2.9")

    def test_caret_zero_patch_is_exact(self):
        self.assertEqual(max_satisfying(["0.0.3", "0.0.4"], "^0.0.3"), "0.0.3")

    def test_caret_partial_forms(self):
        cases = [
            (["1.9.9", "2.0.0"], "^1.2", "1.9.9"),
            (["0.0.9", "0.1.0"], "^0.0", "0.0.9"),
            (["0.9.9", "1.0.0"], "^0", "0.9.9"),
        ]
        for versions, expr, expected in cases:
            with self.subTest(expr=expr):
                self.assertEqual(max_satisfying(versions, expr), expected)

    def test_caret_prerelease_lower_bound(self):
        versions = ["1.2.3-beta.1", "1.2.3-beta.2", "1.9.0"]
        self.assertEqual(max_satisfying(versions, "^1.2.3-beta.2"), "1.9.0")

    def test_tilde_forms(self):
        cases = [
            (["1.2.3", "1.2.9", "1.3.0"], "~1.2.3", "1.2.9"),
            (["1.2.0", "1.3.0"], "~1.2", "1.2.0"),
            (["1.9.0", "2.0.0"], "~1", "1.9.0"),
        ]
        for versions, expr, expected in cases:
            with self.subTest(expr=expr):
                self.assertEqual(max_satisfying(versions, expr), expected)

    def test_bare_partial_and_equals_match_x_range(self):
        versions = ["1.2.0", "1.2.9", "1.3.0"]
        for expr in ["1.2", "=1.2", "1.2.x", "=1.2.*"]:
            with self.subTest(expr=expr):
                self.assertEqual(max_satisfying(versions, expr), "1.2.9")

    def test_equals_major_only(self):
        self.assertEqual(max_satisfying(["1.0.0", "1.9.9", "2.0.0"], "=1"), "1.9.9")

    def test_greater_than_partial_starts_after_range(self):
        self.assertIsNone(max_satisfying(["1.2.9"], ">1.2"))
        self.assertIsNone(max_satisfying(["1.9.9"], ">1"))
        self.assertEqual(max_satisfying(["1.9.9", "2.0.0"], ">1"), "2.0.0")

    def test_less_than_or_equal_partial_stops_before_next(self):
        self.assertEqual(max_satisfying(["1.2.9", "1.3.0"], "<=1.2"), "1.2.9")
        self.assertEqual(max_satisfying(["1.9.9", "2.0.0"], "<=1"), "1.9.9")

    def test_less_than_partial_is_exclusive_of_its_start(self):
        self.assertEqual(max_satisfying(["1.1.9", "1.2.0"], "<1.2"), "1.1.9")

    def test_greater_or_equal_partial(self):
        self.assertEqual(max_satisfying(["1.1.9", "1.2.0"], ">=1.2"), "1.2.0")

    def test_whitespace_after_operator(self):
        self.assertEqual(max_satisfying(["1.2.2", "1.2.3"], ">= 1.2.3"), "1.2.3")
        self.assertEqual(max_satisfying(["1.2.3", "1.2.4"], "> 1.2.3"), "1.2.4")

    def test_hyphen_range_with_partial_upper(self):
        self.assertEqual(
            max_satisfying(["1.2.3", "2.3.9", "2.4.0"], "1.2.3 - 2.3"), "2.3.9"
        )

    def test_hyphen_range_with_full_upper(self):
        self.assertEqual(max_satisfying(["1.1.9", "2.3.4", "2.3.5"], "1.2 - 2.3.4"), "2.3.4")

    def test_hyphen_range_with_partial_lower(self):
        self.assertEqual(
            max_satisfying(["1.2.5", "1.3.0", "2.0.0"], "1.2 - 1.2.9"), "1.2.5"
        )

    def test_or_sets(self):
        self.assertEqual(
            max_satisfying(["1.0.0", "2.5.0", "3.1.0"], "<2.0.0 || >=3.0.0"), "3.1.0"
        )

    def test_empty_range_and_star_match_releases(self):
        for expr in ["", "   ", "*"]:
            with self.subTest(expr=expr):
                self.assertEqual(max_satisfying(["0.9.0", "1.0.0-rc.1"], expr), "0.9.0")

    def test_star_never_matches_prerelease(self):
        self.assertIsNone(max_satisfying(["2.0.0-beta"], "*"))

    def test_prerelease_needs_comparator_with_same_core(self):
        expr = ">=1.2.3-beta.1 <2.0.0"
        self.assertIsNone(max_satisfying(["1.3.0-alpha"], expr))
        self.assertEqual(max_satisfying(["1.2.3-beta.2"], expr), "1.2.3-beta.2")

    def test_prerelease_comparators_do_not_share_across_sets(self):
        # The prerelease comparator sits in the first set, which rejects 1.3.0-beta by ordering.
        self.assertIsNone(max_satisfying(["1.3.0-beta"], "<1.3.0-alpha || >=1.0.0 <2.0.0"))

    def test_prerelease_ordering_chain(self):
        chain = [
            "1.0.0-alpha",
            "1.0.0-alpha.1",
            "1.0.0-alpha.beta",
            "1.0.0-beta",
            "1.0.0-beta.2",
            "1.0.0-beta.11",
            "1.0.0-rc.1",
            "1.0.0",
        ]
        for lower, higher in zip(chain, chain[1:]):
            with self.subTest(lower=lower, higher=higher):
                self.assertEqual(max_satisfying([lower, higher], f">={lower}"), higher)

    def test_numeric_identifier_below_alphanumeric(self):
        self.assertEqual(max_satisfying(["1.0.0-1", "1.0.0-a"], ">=1.0.0-1"), "1.0.0-a")

    def test_build_metadata_ignored_first_maximum_wins(self):
        self.assertEqual(max_satisfying(["1.0.0+a", "1.0.0+b"], "1.0.0"), "1.0.0+a")
        self.assertEqual(max_satisfying(["1.0.0+a", "1.0.0+b"], "=1.0.0+zzz"), "1.0.0+a")

    def test_returns_entry_as_given(self):
        self.assertEqual(max_satisfying(["1.0.0+build.5"], "*"), "1.0.0+build.5")

    def test_invalid_entries_are_skipped(self):
        versions = ["v1.2.3", "1.2", "01.2.3", "1.2.3-01", "1.2.3-", "1.2.3.4", "1.x.3", "*", "1.2.3"]
        self.assertEqual(max_satisfying(versions, "*"), "1.2.3")
        self.assertIsNone(max_satisfying(["v1.2.3", "1.2"], "*"))

    def test_invalid_range_raises(self):
        bad_ranges = [
            ">=a.b",
            "1.2.3.4",
            "^",
            "~",
            "^ 1.2.3",
            "1.2-2.3.4",
            "1.2 -",
            "- 1.2",
            "1.2 - 2 - 3",
            ">=1.2-beta",
            ">*",
            "^*",
            "1.x.abc",
            "1..2",
        ]
        for expr in bad_ranges:
            with self.subTest(expr=expr):
                with self.assertRaises(ValueError):
                    max_satisfying(["1.0.0"], expr)

    def test_invalid_range_raises_with_no_versions(self):
        with self.assertRaises(ValueError):
            max_satisfying([], ">=a.b")

    def test_wildcard_forms(self):
        self.assertEqual(max_satisfying(["1.5.0", "2.0.0"], "1.X"), "1.5.0")
        self.assertEqual(max_satisfying(["9.9.9"], "x"), "9.9.9")
        self.assertEqual(max_satisfying(["9.9.9"], "*"), "9.9.9")

    def test_empty_set_in_or_matches_everything(self):
        self.assertEqual(max_satisfying(["5.0.0"], "1.0.0 ||"), "5.0.0")

    def test_exact_prerelease_equality(self):
        self.assertEqual(
            max_satisfying(["1.2.3-beta", "1.2.3-beta.1"], "=1.2.3-beta"), "1.2.3-beta"
        )

    def test_component_after_wildcard_is_ignored(self):
        self.assertEqual(max_satisfying(["1.5.0", "2.0.0"], "1.x.3"), "1.5.0")


if __name__ == "__main__":
    unittest.main()
