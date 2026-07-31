"""The dagreach attribute profile.

dagreach does not define a graph format. It reads DOT and JSON Graph Format,
and gives a meaning to a handful of attribute names when they happen to be
there. Everything is optional: a graph with no profile attributes still gets
every structural answer, only the weighted ones change.

See docs/attribute-profile.md for the normative description.
"""

from __future__ import annotations

from collections import Counter
from dataclasses import dataclass, field

from dagreach.model import Edge, Graph, Node

#: Attribute names read for a duration, in order of precedence.
DURATION_KEYS = ("duration", "weight")
#: Attribute name read for a lifecycle state.
STATUS_KEY = "status"
#: Attribute name read for a grouping, used for rollups.
GROUP_KEY = "group"

PROFILE_KEYS = (*DURATION_KEYS, STATUS_KEY, GROUP_KEY)


def duration_of(item: Node | Edge) -> float | None:
    """The declared duration, or None when absent or unreadable."""
    for key in DURATION_KEYS:
        raw = item.attrs.get(key)
        if raw is None:
            continue
        value = _as_float(raw)
        if value is not None:
            return value
    return None


def status_of(item: Node | Edge) -> str | None:
    value = item.attrs.get(STATUS_KEY)
    return value.strip() if isinstance(value, str) and value.strip() else None


def group_of(item: Node | Edge) -> str | None:
    value = item.attrs.get(GROUP_KEY)
    return value.strip() if isinstance(value, str) and value.strip() else None


@dataclass(slots=True)
class ProfileSummary:
    """What the profile found in a graph — the basis of `dagreach parse` output."""

    nodes_with_duration: int = 0
    edges_with_duration: int = 0
    statuses: Counter[str] = field(default_factory=Counter)
    groups: Counter[str] = field(default_factory=Counter)
    unreadable: list[str] = field(default_factory=list)

    @property
    def uses_durations(self) -> bool:
        return bool(self.nodes_with_duration or self.edges_with_duration)


def summarize(graph: Graph) -> ProfileSummary:
    """Report the profile attributes present, and the values that could not be read."""
    summary = ProfileSummary()

    for node in graph.nodes.values():
        if duration_of(node) is not None:
            summary.nodes_with_duration += 1
        else:
            _record_unreadable_duration(node, f"node {node.id!r}", summary)
        status = status_of(node)
        if status:
            summary.statuses[status] += 1
        group = group_of(node)
        if group:
            summary.groups[group] += 1

    for edge in graph.edges:
        if duration_of(edge) is not None:
            summary.edges_with_duration += 1
        else:
            _record_unreadable_duration(edge, f"edge {edge.source!r} -> {edge.target!r}", summary)

    return summary


def _record_unreadable_duration(item: Node | Edge, label: str, summary: ProfileSummary) -> None:
    for key in DURATION_KEYS:
        raw = item.attrs.get(key)
        if raw is not None and _as_float(raw) is None:
            summary.unreadable.append(f"{label}: {key}={raw!r} is not a number, so it was ignored")
            return


def _as_float(raw: str) -> float | None:
    try:
        value = float(raw)
    except (TypeError, ValueError):
        return None
    if value != value or value in (float("inf"), float("-inf")):  # NaN and infinities
        return None
    return value
