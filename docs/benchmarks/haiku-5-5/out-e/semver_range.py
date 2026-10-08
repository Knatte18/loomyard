"""Pick the highest semver version that satisfies a range expression."""

import operator
import re
from typing import NamedTuple, Optional

_NUMERIC = r"0|[1-9][0-9]*"
_PRE_ID = r"0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*"
_PRE = rf"(?:{_PRE_ID})(?:\.(?:{_PRE_ID}))*"
_BUILD = r"[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*"
_FULL = re.compile(rf"({_NUMERIC})\.({_NUMERIC})\.({_NUMERIC})(?:-({_PRE}))?(?:\+({_BUILD}))?")
_PART = r"0|[1-9][0-9]*|[xX*]"
_PARTIAL = re.compile(rf"({_PART})(?:\.({_PART})(?:\.({_PART}))?)?")
_WILDCARDS = ("x", "X", "*")
_TERM = re.compile(r"(>=|<=|>|<|=|\^|~)?(.*)", re.DOTALL)
_OPERATORS = (">=", "<=", ">", "<", "=", "^", "~")
_COMPARE = {
    ">=": operator.ge,
    ">": operator.gt,
    "<=": operator.le,
    "<": operator.lt,
}


class _Version(NamedTuple):
    core: tuple  # (major, minor, patch)
    pre: tuple  # prerelease identifier keys; empty for a release

    def sort_key(self):
        return (*self.core, (0, self.pre) if self.pre else (1,))


class _Comparator(NamedTuple):
    op: str
    bound: _Version


# Nothing sorts below 0.0.0, so this comparator matches no version.
_NOTHING = _Comparator("<", _Version((0, 0, 0), ()))


def max_satisfying(versions: list[str], range_expr: str) -> str | None:
    """Return the highest entry of `versions` that satisfies `range_expr`, or None."""
    sets = _parse_range(range_expr)
    best_text = None
    best = None
    for text in versions:
        version = _parse_version(text)
        if version is None or not any(_set_matches(s, version) for s in sets):
            continue
        # Strict comparison keeps the first of several equal maxima.
        if best is None or version.sort_key() > best.sort_key():
            best_text, best = text, version
    return best_text


def _parse_version(text: str) -> Optional[_Version]:
    match = _FULL.fullmatch(text)
    if match is None:
        return None
    major, minor, patch, pre = match.group(1, 2, 3, 4)
    return _Version((int(major), int(minor), int(patch)), _pre_keys(pre))


def _pre_keys(pre: Optional[str]) -> tuple:
    if pre is None:
        return ()
    return tuple(_identifier_key(ident) for ident in pre.split("."))


def _identifier_key(ident: str) -> tuple:
    # Numeric identifiers sort below alphanumeric ones, as semver requires.
    if ident.isdigit():
        return (0, int(ident))
    return (1, ident)


def _parse_range(range_expr: str) -> list[list[_Comparator]]:
    # An empty set yields no comparators, which matches like `*`.
    return [_parse_set(text) for text in range_expr.split("||")]


def _parse_set(text: str) -> list[_Comparator]:
    tokens = text.split()
    comparators = []
    i = 0
    while i < len(tokens):
        if i + 2 < len(tokens) and tokens[i + 1] == "-":
            comparators += _hyphen_terms(tokens[i], tokens[i + 2])
            i += 3
        elif tokens[i] in _OPERATORS and i + 1 < len(tokens):
            comparators += _term(tokens[i], tokens[i + 1])
            i += 2
        else:
            op, body = _TERM.fullmatch(tokens[i]).groups()
            comparators += _term(op or "", body)
            i += 1
    return comparators


def _term(op: str, body: str) -> list[_Comparator]:
    core, pre = _parse_operand(body)
    if op == "^":
        return _caret_terms(core, pre)
    if op == "~":
        return _tilde_terms(core, pre)
    return _comparator_terms(op or "=", core, pre)


def _parse_operand(text: str) -> tuple[tuple, tuple]:
    """Return ((major, minor, patch), prerelease keys), with None for missing or wildcard parts."""
    full = _FULL.fullmatch(text)
    if full is not None:
        major, minor, patch, pre = full.group(1, 2, 3, 4)
        return (int(major), int(minor), int(patch)), _pre_keys(pre)
    partial = _PARTIAL.fullmatch(text)
    if partial is None:
        raise ValueError(f"invalid version in range: {text!r}")
    parts = [None if part in _WILDCARDS else int(part) for part in partial.groups() if part is not None]
    return _truncate_at_wildcard(parts), ()


def _truncate_at_wildcard(parts: list) -> tuple:
    # Everything after a wildcard, or a missing part, is also a wildcard.
    parts = parts + [None] * (3 - len(parts))
    first_open = next((i for i, part in enumerate(parts) if part is None), 3)
    return tuple(parts[:first_open]) + (None,) * (3 - first_open)


def _lower(core: tuple, pre: tuple) -> _Version:
    return _Version(tuple(0 if part is None else part for part in core), pre)


def _partial_upper(core: tuple) -> _Version:
    major, minor, _ = core
    if minor is None:
        return _Version((major + 1, 0, 0), ())
    return _Version((major, minor + 1, 0), ())


def _comparator_terms(op: str, core: tuple, pre: tuple) -> list[_Comparator]:
    if None not in core:
        bound = _Version(core, pre)
        if op == "=":
            return [_Comparator(">=", bound), _Comparator("<=", bound)]
        return [_Comparator(op, bound)]
    if core[0] is None:
        # A bare wildcard admits every version; strict operators admit none.
        return [] if op in ("=", ">=", "<=") else [_NOTHING]
    lower = _lower(core, ())
    upper = _partial_upper(core)
    if op == "=":
        return [_Comparator(">=", lower), _Comparator("<", upper)]
    if op == ">=":
        return [_Comparator(">=", lower)]
    if op == "<=":
        return [_Comparator("<", upper)]
    if op == ">":
        return [_Comparator(">=", upper)]
    return [_Comparator("<", lower)]


def _caret_terms(core: tuple, pre: tuple) -> list[_Comparator]:
    major, minor, patch = core
    if major is None:
        return []
    if major > 0:
        upper = (major + 1, 0, 0)
    elif minor is None:
        upper = (1, 0, 0)
    elif patch is None or minor > 0:
        upper = (0, minor + 1, 0)
    else:
        upper = (0, 0, patch + 1)
    return [_Comparator(">=", _lower(core, pre)), _Comparator("<", _Version(upper, ()))]


def _tilde_terms(core: tuple, pre: tuple) -> list[_Comparator]:
    major, minor, _ = core
    if major is None:
        return []
    upper = (major + 1, 0, 0) if minor is None else (major, minor + 1, 0)
    return [_Comparator(">=", _lower(core, pre)), _Comparator("<", _Version(upper, ()))]


def _hyphen_terms(low_text: str, high_text: str) -> list[_Comparator]:
    low_core, low_pre = _parse_operand(low_text)
    high_core, high_pre = _parse_operand(high_text)
    terms = [_Comparator(">=", _lower(low_core, low_pre))]
    if None not in high_core:
        terms.append(_Comparator("<=", _Version(high_core, high_pre)))
    elif high_core[0] is not None:
        terms.append(_Comparator("<", _partial_upper(high_core)))
    return terms


def _set_matches(comparators: list[_Comparator], version: _Version) -> bool:
    key = version.sort_key()
    if not all(_COMPARE[c.op](key, c.bound.sort_key()) for c in comparators):
        return False
    if not version.pre:
        return True
    # A prerelease needs a comparator with a prerelease on the same MAJOR.MINOR.PATCH.
    return any(c.bound.pre and c.bound.core == version.core for c in comparators)
