"""
Resolves the highest version that satisfies a semver range expression.
Versions follow semver 2.0.0 precedence and ignore build metadata.
Ranges support comparators, partial versions with wildcards, caret, tilde, hyphen ranges, and `||` unions.
"""

import operator
import re
from typing import NamedTuple

_NUMERIC = re.compile(r"0|[1-9][0-9]*")
_IDENTIFIER = re.compile(r"[0-9A-Za-z-]+")
_WILDCARDS = ("x", "X", "*")
# Two-character symbols come first so that `<=` is not read as `<`.
_SYMBOLS = (">=", "<=", ">", "<", "=")
_COMPARISONS = {
    ">=": operator.ge,
    ">": operator.gt,
    "<=": operator.le,
    "<": operator.lt,
    "=": operator.eq,
    "never": lambda version_key, bound_key: False,
}


class _Version(NamedTuple):
    """
    A full version without build metadata.

    Instance variables:
        core: (major, minor, patch) integers.
        prerelease: Prerelease identifiers, empty for a release.
    """

    core: tuple[int, int, int]
    prerelease: tuple[str, ...]


class _Comparator(NamedTuple):
    """
    One bound on versions, such as `>=1.2.3`.

    Instance variables:
        symbol: Key into _COMPARISONS.
        version: The bound.
    """

    symbol: str
    version: _Version


# Matches no version; used for `>*` and `<*`.
_NEVER = _Comparator("never", _Version((0, 0, 0), ()))


def max_satisfying(versions: list[str], range_expr: str) -> str | None:
    """
    Returns the highest entry of versions that satisfies range_expr.

    The returned string is the entry exactly as given.
    Entries that are not valid versions are skipped.
    Among entries of equal precedence, the first in list order wins.

    Raises:
        ValueError: If range_expr is not a valid range.
    """
    comparator_sets = _parse_range(range_expr)
    best_text = None
    best_key = None
    for text in versions:
        version = _parse_version(text)
        if version is None or not _range_matches(comparator_sets, version):
            continue
        key = _precedence_key(version)
        if best_key is None or key > best_key:
            best_text, best_key = text, key
    return best_text


def _parse_range(range_expr):
    """
    Parses a range into one list of comparators per `||`-separated set.

    An empty set has no comparators, so it matches any release.
    """
    if not isinstance(range_expr, str):
        raise ValueError(f"range must be a string, not {type(range_expr).__name__}")
    return [_parse_set(text) for text in range_expr.split("||")]


def _parse_set(text):
    """Expands the whitespace-separated terms of one set, all of which must hold."""
    tokens = text.split()
    comparators = []
    index = 0
    while index < len(tokens):
        term_comparators, index = _parse_term(tokens, index)
        comparators += term_comparators
    return comparators


def _parse_term(tokens, index):
    """
    Expands the term at tokens[index].

    Returns its comparators and the index of the token after the term.
    """
    token = tokens[index]
    if token[0] in "^~":
        parts, prerelease = _parse_operand(token[1:])
        expand = _caret if token[0] == "^" else _tilde
        return expand(parts, prerelease), index + 1
    symbol = _leading_symbol(token)
    if symbol is not None:
        return _parse_comparator(tokens, index, symbol)
    if index + 1 < len(tokens) and tokens[index + 1] == "-":
        return _parse_hyphen(tokens, index)
    parts, prerelease = _parse_operand(token)
    return _operator_comparators("=", parts, prerelease), index + 1


def _leading_symbol(token):
    """Returns the comparator symbol that token starts with, or None."""
    return next((symbol for symbol in _SYMBOLS if token.startswith(symbol)), None)


def _parse_comparator(tokens, index, symbol):
    """
    Expands a comparator whose version is attached (`>=1.2`) or is the next token (`>= 1.2`).

    Returns its comparators and the index of the token after the term.
    """
    operand, consumed = tokens[index][len(symbol):], 1
    if not operand:
        if index + 1 == len(tokens):
            raise ValueError(f"comparator {symbol!r} has no version")
        operand, consumed = tokens[index + 1], 2
    parts, prerelease = _parse_operand(operand)
    return _operator_comparators(symbol, parts, prerelease), index + consumed


def _parse_hyphen(tokens, index):
    """
    Expands `A - B`, a lower bound A and an upper bound B with spaces around the hyphen.

    Returns its comparators and the index of the token after the term.
    """
    if index + 2 == len(tokens):
        raise ValueError("hyphen range has no upper bound")
    low_parts, low_prerelease = _parse_operand(tokens[index])
    high_parts, high_prerelease = _parse_operand(tokens[index + 2])
    low = _operator_comparators(">=", low_parts, low_prerelease)
    high = _operator_comparators("<=", high_parts, high_prerelease)
    return low + high, index + 3


def _operator_comparators(symbol, parts, prerelease):
    """
    Expands a comparator symbol applied to a version or partial version.

    A full version keeps its plain meaning.
    A partial version stands for the range it covers, so `=1.2` means `>=1.2.0 <1.3.0`.
    """
    if None not in parts:
        return [_Comparator(symbol, _Version(parts, prerelease))]
    low = _Version(_zero_filled(parts), ())
    high = _upper_bound(parts)
    if symbol == ">":
        return [_Comparator(">=", high)] if high is not None else [_NEVER]
    if symbol == "<":
        return [_Comparator("<", low)] if high is not None else [_NEVER]
    if symbol == ">=":
        return [_Comparator(">=", low)]
    if symbol == "<=":
        return [_Comparator("<", high)] if high is not None else []
    comparators = [_Comparator(">=", low)]
    if high is not None:
        comparators.append(_Comparator("<", high))
    return comparators


def _caret(parts, prerelease):
    """
    Expands `^V`, which allows changes that keep the leftmost non-zero component.

    A wildcard major matches any version.
    """
    if parts[0] is None:
        return []
    given = [index for index, part in enumerate(parts) if part is not None]
    # The leftmost non-zero component is fixed; with none, the last one given is.
    pinned = next((index for index in given if parts[index] > 0), given[-1])
    high = tuple(parts[index] + 1 if index == pinned else 0 for index in range(3))
    return [
        _Comparator(">=", _Version(_zero_filled(parts), prerelease)),
        _Comparator("<", _Version(high, ())),
    ]


def _tilde(parts, prerelease):
    """
    Expands `~V`, which keeps the major fixed and the minor too when V names one.

    A wildcard major matches any version.
    """
    if parts[0] is None:
        return []
    return [
        _Comparator(">=", _Version(_zero_filled(parts), prerelease)),
        _Comparator("<", _upper_bound(parts)),
    ]


def _parse_operand(text):
    """
    Parses a partial version, or a full version with optional prerelease and build.

    Returns (parts, prerelease); parts has None for each missing or wildcard component.
    """
    head, plus, build = text.partition("+")
    core, dash, prerelease = head.partition("-")
    parts = _parse_partial(core)
    if not dash and not plus:
        return parts, ()
    if None in parts:
        raise ValueError(f"prerelease or build needs a full version: {text!r}")
    if dash and not _identifiers_valid(prerelease, no_leading_zeros=True):
        raise ValueError(f"invalid prerelease: {text!r}")
    if plus and not _identifiers_valid(build, no_leading_zeros=False):
        raise ValueError(f"invalid build metadata: {text!r}")
    return parts, (tuple(prerelease.split(".")) if dash else ())


def _parse_partial(text):
    """
    Parses MAJOR, MAJOR.MINOR or MAJOR.MINOR.PATCH, where x, X and * are wildcards.

    Returns a 3-tuple of ints, with None for each missing or wildcard component.
    Every component after a wildcard must also be a wildcard.
    """
    names = text.split(".")
    if len(names) > 3:
        raise ValueError(f"too many version components: {text!r}")
    parts = [None, None, None]
    for index, name in enumerate(names):
        if name in _WILDCARDS:
            if any(later not in _WILDCARDS for later in names[index + 1:]):
                raise ValueError(f"component after a wildcard is not a wildcard: {text!r}")
            break
        if not _NUMERIC.fullmatch(name):
            raise ValueError(f"invalid version component: {text!r}")
        parts[index] = int(name)
    return tuple(parts)


def _identifiers_valid(text, no_leading_zeros):
    for identifier in text.split("."):
        if not _IDENTIFIER.fullmatch(identifier):
            return False
        if no_leading_zeros and identifier.isdigit() and not _NUMERIC.fullmatch(identifier):
            return False
    return True


def _parse_version(text):
    """
    Returns the full version named by text, or None if text is not one.

    Non-string entries are not versions.
    """
    if not isinstance(text, str):
        return None
    try:
        parts, prerelease = _parse_operand(text)
    except ValueError:
        return None
    if None in parts:
        return None
    return _Version(parts, prerelease)


def _zero_filled(parts):
    return tuple(0 if part is None else part for part in parts)


def _upper_bound(parts):
    """
    Returns the first version past the minor level of parts, or past the major when no minor is given.

    Returns None for a wildcard major, which covers every version.
    """
    major, minor, _ = parts
    if major is None:
        return None
    if minor is None:
        return _Version((major + 1, 0, 0), ())
    return _Version((major, minor + 1, 0), ())


def _precedence_key(version):
    """
    Returns a sort key that orders versions by semver 2.0.0 precedence.

    A release sorts above every prerelease with the same core.
    """
    if not version.prerelease:
        return (version.core, 1, ())
    identifiers = tuple(_identifier_key(identifier) for identifier in version.prerelease)
    return (version.core, 0, identifiers)


def _identifier_key(identifier):
    """
    Returns a sort key for one prerelease identifier.

    Numeric identifiers sort by value and below alphanumeric ones, which sort in ASCII order.
    """
    if identifier.isdigit():
        return (0, int(identifier), "")
    return (1, 0, identifier)


def _range_matches(comparator_sets, version):
    return any(_set_matches(comparators, version) for comparators in comparator_sets)


def _set_matches(comparators, version):
    """
    Returns True if version satisfies every comparator in one set, under the prerelease rule.

    A prerelease also needs a comparator with a prerelease on the same core.
    """
    if not all(_holds(comparator, version) for comparator in comparators):
        return False
    if not version.prerelease:
        return True
    return any(
        comparator.version.prerelease and comparator.version.core == version.core
        for comparator in comparators
    )


def _holds(comparator, version):
    compare = _COMPARISONS[comparator.symbol]
    return compare(_precedence_key(version), _precedence_key(comparator.version))
