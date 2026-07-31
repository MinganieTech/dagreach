"""No profile: DOT or JSON Graph Format, read as they are.

This is what runs when nothing was recognised, and what to use for a graph you
produced yourself. It normalises nothing and assumes nothing beyond the format,
so the edge semantics are yours to declare.
"""

from __future__ import annotations

from dagreach.adapters import Profile
from dagreach.model import Graph
from dagreach.readers import read_text
from dagreach.semantics import DEFAULT_SEMANTICS


def load(text: str, source: str | None = None) -> Graph:
    return read_text(text, source=source)


def detect(text: str) -> bool:
    """Never claims a file: it is the fallback, not a recognition."""
    return False


PROFILE = Profile(
    name="generic",
    summary="DOT or JSON Graph Format, no normalisation, semantics up to you",
    produced_by="anything",
    edge_semantics=DEFAULT_SEMANTICS,
    load=load,
    detect=detect,
)
