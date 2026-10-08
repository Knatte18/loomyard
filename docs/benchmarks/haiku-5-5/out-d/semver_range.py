"""Resolve the highest version satisfying a semver range expression."""

import re
from typing import NamedTuple

_NUM = r"0|[1-9][0-9]*"
_PRE_ID = r"0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*"
_PRE = rf"(?:{_PRE_ID})(?:\.(?:{_PRE_ID}))*"
_BUILD = r"[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*"
_VERSION_RE = re.compile(rf"({_NUM})\.({_NUM})\.({_NUM})(?:-({_PRE}))?(?:\+{_BUILD})?")

_PART = rf"{_NUM}|[xX*]"
_PARTIAL_RE = re.compile(
    rf"({_PART})(?:\.({_PART})(?:\.({_PART})(?:-({_PRE}))?(?:\+{_BUILD})?)?)?"
)

_OPERATORS = (">=", "<=", ">", "<", "=")


class Version(NamedTuple):
    major: int
    minor: int
    patch: int
    prerelease: tuple

    def key(self):
        """Ordering key: build is already dropped; a release sorts above its prereleases."""
        if not self.prerelease:
            pre_key = (1,)
        else:
            pre_key = (0, tuple(_identifier_key(i) for i in self.prerelease))
        return (self.major, self.minor, self.patch, pre_key)

    def triple(self):
        return (self.major, self.minor, self.patch)


def _identifier_key(identifier):
    if identifier.isdigit():
        return (0, int(identifier), "")
    return (1, 0, identifier)


def parse_version(text):
    """Parse a full version, or return None if it is not one."""
    if not isinstance(text, str):
        return None
    match = _VERSION_RE.fullmatch(text)
    if not match:
        return None
    major, minor, patch, pre = match.groups()
    return Version(int(major), int(minor), int(patch), tuple(pre.split(".")) if pre else ())


class Partial(NamedTuple):
    """A version whose missing or wildcard components are None."""

    major: object
    minor: object
    patch: object
    prerelease: tuple

    def zero_filled(self):
        return Version(self.major or 0, self.minor or 0, self.patch or 0, self.prerelease)


def parse_partial(text):
    match = _PARTIAL_RE.fullmatch(text)
    if not match:
        raise ValueError(f"invalid version in range: {text!r}")
    parts = []
    wildcard = False
    for part in match.groups()[:3]:
        wildcard = wildcard or part is None or part in ("x", "X", "*")
        parts.append(None if wildcard else int(part))
    pre = match.group(4)
    if pre and wildcard:
        raise ValueError(f"prerelease on a partial version: {text!r}")
    return Partial(*parts, tuple(pre.split(".")) if pre else ())


# Satisfied by nothing: no release is below 0.0.0, and a 0.0.0 prerelease fails the
# prerelease rule because this comparator has no prerelease.
_NEVER = [("<", Version(0, 0, 0, ()))]
_ANY = []


def _next_major(p):
    return Version(p.major + 1, 0, 0, ())


def _next_minor(p):
    return Version(p.major, p.minor + 1, 0, ())


def _expand_operator(op, p):
    """Expand an operator comparator on a partial version into plain comparators."""
    if p.major is None:
        return _NEVER if op in (">", "<") else _ANY
    if p.patch is not None:
        return [(op, p.zero_filled())]
    upper = _next_major(p) if p.minor is None else _next_minor(p)
    if op == "=":
        return [(">=", p.zero_filled()), ("<", upper)]
    if op == ">":
        return [(">=", upper)]
    if op == ">=":
        return [(">=", p.zero_filled())]
    if op == "<":
        return [("<", p.zero_filled())]
    return [("<", upper)]  # "<="


def _expand_caret(p):
    if p.major is None:
        return _ANY
    if p.major > 0 or p.minor is None:
        upper = _next_major(p)
    elif p.minor > 0 or p.patch is None:
        upper = _next_minor(p)
    else:
        upper = Version(0, 0, p.patch + 1, ())
    return [(">=", p.zero_filled()), ("<", upper)]


def _expand_tilde(p):
    if p.major is None:
        return _ANY
    upper = _next_major(p) if p.minor is None else _next_minor(p)
    return [(">=", p.zero_filled()), ("<", upper)]


def _expand_hyphen(low, high):
    comparators = [(">=", low.zero_filled())]
    if high.major is None:
        return comparators
    if high.minor is None:
        return comparators + [("<", _next_major(high))]
    if high.patch is None:
        return comparators + [("<", _next_minor(high))]
    return comparators + [("<=", high.zero_filled())]


def _expand_term(term):
    if term.startswith("^"):
        return _expand_caret(parse_partial(term[1:]))
    if term.startswith("~"):
        return _expand_tilde(parse_partial(term[1:]))
    for op in _OPERATORS:
        if term.startswith(op):
            return _expand_operator(op, parse_partial(term[len(op):]))
    return _expand_operator("=", parse_partial(term))


def parse_set(text):
    """Parse one comparator set into a list of (operator, Version) comparators."""
    tokens = text.split()
    comparators = []
    i = 0
    while i < len(tokens):
        token = tokens[i]
        if token in _OPERATORS:
            if i + 1 >= len(tokens):
                raise ValueError(f"operator without version: {token!r}")
            token += tokens[i + 1]
            i += 1
        if i + 1 < len(tokens) and tokens[i + 1] == "-":
            if i + 2 >= len(tokens):
                raise ValueError("hyphen range without upper bound")
            comparators += _expand_hyphen(parse_partial(token), parse_partial(tokens[i + 2]))
            i += 3
            continue
        comparators += _expand_term(token)
        i += 1
    return comparators


def parse_range(text):
    """Parse a range into a list of comparator sets."""
    if not isinstance(text, str):
        raise ValueError("range must be a string")
    return [parse_set(part) for part in text.split("||")]


def _compare(version, op, bound):
    a, b = version.key(), bound.key()
    return {
        ">=": a >= b,
        ">": a > b,
        "<=": a <= b,
        "<": a < b,
        "=": a == b,
    }[op]


def satisfies_set(version, comparators):
    if not all(_compare(version, op, bound) for op, bound in comparators):
        return False
    if not version.prerelease:
        return True
    return any(
        bound.prerelease and bound.triple() == version.triple() for _, bound in comparators
    )


def max_satisfying(versions, range_expr):
    """Return the highest entry of versions satisfying range_expr, or None.

    Invalid versions are skipped; an invalid range raises ValueError.
    Among versions equal except for build, the first in list order wins.
    """
    sets = parse_range(range_expr)
    best, best_version = None, None
    for text in versions:
        version = parse_version(text)
        if version is None:
            continue
        if not any(satisfies_set(version, s) for s in sets):
            continue
        if best_version is None or version.key() > best_version.key():
            best, best_version = text, version
    return best
