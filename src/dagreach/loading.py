"""One door for every input: pick a profile, read, orient.

The order matters and is the same for every command. The profile decides how to
read the file and which way its edges point; an explicit flag always wins over
the profile; and whatever ends up applied is stated in the report rather than
assumed by the reader.
"""

from __future__ import annotations

from dagreach.adapters import PROFILES, Profile, detect_profile, get_profile
from dagreach.model import Graph
from dagreach.readers import read_source
from dagreach.semantics import DEFAULT_SEMANTICS, orient, warn_if_orientation_is_suspect


def load_graph(
    path: str,
    *,
    profile: str | None = None,
    format: str | None = None,
    edge_semantics: str | None = None,
) -> Graph:
    """Read `path` through a profile — named, detected, or generic — and orient it."""
    text, source = read_source(path)

    chosen, detected = _resolve_profile(text, profile)
    graph = (
        chosen.load(text, source)
        if chosen.name != "generic"
        else _generic(text, source, format, path)
    )
    graph.profile = chosen.name

    if format and chosen.name != "generic":
        graph.warn(
            f"--format {format} was ignored: the {chosen.name} profile knows the format it reads"
        )
    if detected:
        graph.warn(
            f"read with the {chosen.name} profile, recognised from the file itself; "
            "pass --profile to choose explicitly"
        )

    semantics = edge_semantics or chosen.edge_semantics or DEFAULT_SEMANTICS
    graph = orient(graph, semantics)
    if edge_semantics and chosen.name != "generic" and edge_semantics != chosen.edge_semantics:
        graph.warn(
            f"--edge-semantics {edge_semantics} overrides the {chosen.name} profile, "
            f"which declares {chosen.edge_semantics}"
        )
    if chosen.name == "generic":
        warn_if_orientation_is_suspect(graph, edge_semantics)
    return graph


def _resolve_profile(text: str, requested: str | None) -> tuple[Profile, bool]:
    if requested:
        return get_profile(requested), False
    found = detect_profile(text)
    if found is not None:
        return found, True
    return PROFILES["generic"], False


def _generic(text: str, source: str, format: str | None, hint: str) -> Graph:
    from dagreach.readers import read_text

    return read_text(text, source=source, format=format, hint=hint)
