"""What an edge means, which decides which way "reaches" runs.

Exports disagree, and the disagreement is invisible in the file:

    terraform graph   "aws_instance.web" -> "aws_subnet.main"    the source DEPENDS ON the target
    an Airflow export  extract -> transform                       the source FEEDS the target

Read the first one as if it were the second and every impact answer comes out
exactly backwards, which is worse than no answer at all. dagreach therefore
never guesses in silence: the orientation applied is stated in every report,
and a file whose shape contradicts the orientation in force raises a warning.
"""

from __future__ import annotations

from dagreach.model import Edge, Graph

#: `feeds` = source feeds target (follow edges forward to find what is affected).
#: `depends-on` = source depends on target (follow them backward).
EDGE_SEMANTICS = ("feeds", "depends-on")
DEFAULT_SEMANTICS = "feeds"


def orient(graph: Graph, semantics: str) -> Graph:
    """Return the graph in dagreach's internal orientation: source feeds target.

    `depends-on` inputs are reversed once, here, so that every later step reads
    one direction only.
    """
    if semantics not in EDGE_SEMANTICS:
        raise ValueError(f"unknown edge semantics {semantics!r}")

    graph.edge_semantics = semantics
    if semantics == "feeds":
        return graph

    graph.edges = [Edge(edge.target, edge.source, edge.attrs) for edge in graph.edges]
    return graph


def looks_like_dependency_export(graph: Graph) -> str | None:
    """Name the producer when the file carries a recognisable dependency-first signature.

    Only signatures specific enough to be worth acting on are listed; a guess
    that is often wrong would be worse than none.
    """
    if any(node.startswith("[root] ") for node in graph.nodes) and {
        "compound",
        "newrank",
    } <= set(graph.attrs):
        return "terraform graph"
    return None


def warn_if_orientation_is_suspect(graph: Graph, declared: str | None) -> None:
    """Warn when the file looks like a dependency export but nobody said so."""
    if declared == "depends-on":
        return
    producer = looks_like_dependency_export(graph)
    if producer is None:
        return
    graph.warn(
        f"this file looks like {producer} output, where an edge means "
        f"'source depends on target', but it was read as '{graph.edge_semantics}'; "
        "pass --edge-semantics depends-on if impact comes out backwards"
    )
