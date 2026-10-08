# Report: semver range resolver

## Tests

Command, run from `.scratch/haikubench/out-c/`:

```
python3 -m unittest -v test_semver_range
```

Real output (tail; every test line was `ok`):

```
test_whitespace_after_operator (test_semver_range.MaxSatisfyingTest.test_whitespace_after_operator) ... ok

----------------------------------------------------------------------
Ran 25 tests in 0.002s

OK
```

All 25 tests pass.
Every example from the spec table is covered by a test.

## Ambiguities and choices

- `>*` and `<*` match nothing; `=*`, `>=*` and `<=*` match everything.
  The spec only defines `*` for `=`.
- A wildcard component makes all later components wildcards (`x.2` is `*`, `1.x.3` is `1.x`).
- A prerelease on a partial with a wildcard patch (`1.2.x-beta`) is invalid.
- Numeric range components with leading zeros (`01.2.3`) are invalid, as in versions.
- Build metadata on a range version is accepted and ignored.
- Whitespace between operator and version is accepted only for `>=`, `>`, `<=`, `<`, `=`; `^ 1.2` and `~ 1.2` raise `ValueError`, since the spec grants it only to operator comparators.
- A hyphen range needs bare partial versions on both sides; `>=1 - 2`, `1 - 2 - 3`, a leading or trailing `-` raise `ValueError`.
- Hyphen `A` with a prerelease keeps it in the lower bound and counts for the prerelease rule; `B` with a prerelease is plain `<=B`.
- Caret and tilde with a prerelease (`~1.2.3-beta`) keep the prerelease in the lower bound.
- The prerelease rule compares the version's `MAJOR.MINOR.PATCH` to every comparator with a prerelease, including the lower bound of caret, tilde and hyphen expansions.
- A `||` with an empty side (`>=3.0.0 ||`) has an empty set, which is `*`.
- An empty `versions` list still validates the range.
- Non-string entries in `versions` are skipped as invalid.
- Ties on equal precedence (differing only in build) return the first in list order.

## Known gaps

- `range_expr` that is not a `str` raises `AttributeError`, not `ValueError`.
- Whitespace is whatever `str.split()` treats as whitespace, including Unicode spaces.
- No tests beyond the cases listed; there is no property or fuzz test against a reference implementation.

## Timing

start: 1791459898.731009
stop: 1791459973.001699686
duration: 74.3
