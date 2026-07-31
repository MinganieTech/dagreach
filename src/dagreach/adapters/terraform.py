"""`terraform graph` output.

Terraform writes an edge from a resource to what it depends on, so impact runs
backwards through this file. It also decorates identifiers for its own renderer:

    "[root] aws_instance.web (expand)" -> "[root] aws_subnet.main (expand)"

The decoration is noise at the command line — nobody wants to type
`--changed '[root] aws_instance.web (expand)'` — so the profile strips it and
keeps the original under `terraform_id`. Two nodes that would collide once
stripped keep their full identifiers, and the graph says so.
"""

from __future__ import annotations

import re

from dagreach.adapters import Profile
from dagreach.model import Graph
from dagreach.readers.dot import parse_dot

_PREFIX = re.compile(r"^\[root\]\s+")
_SUFFIX = re.compile(r"\s+\((expand|close|destroy)\)$")


def normalise(node_id: str) -> str:
    """Drop the renderer's decoration: `[root] aws_vpc.main (expand)` -> `aws_vpc.main`."""
    return _SUFFIX.sub("", _PREFIX.sub("", node_id)).strip()


def kind_of(node_id: str) -> str:
    """The resource kind, used as the group: `aws_vpc.main` -> `aws_vpc`."""
    if node_id.startswith("provider["):
        return "provider"
    head = node_id.split(".", 1)[0]
    return head or "unknown"


def load(text: str, source: str | None = None) -> Graph:
    raw = parse_dot(text, source=source)

    mapping = {node_id: normalise(node_id) for node_id in raw.nodes}
    collisions = _collisions(mapping)
    if collisions:
        raw.warn(
            f"{len(collisions)} identifier(s) would collide once the [root] decoration is "
            "stripped, so those nodes keep their full terraform identifier"
        )
        for node_id in list(mapping):
            if mapping[node_id] in collisions:
                mapping[node_id] = node_id

    # Edges stay in Terraform's own orientation; the single reversal happens once,
    # in dagreach.semantics.orient, driven by the profile's declared semantics.
    graph = Graph(
        name=raw.name,
        directed=True,
        source=source,
        format="dot",
        attrs=dict(raw.attrs),
        warnings=list(raw.warnings),
    )
    for node_id, node in raw.nodes.items():
        attrs = dict(node.attrs)
        attrs["group"] = kind_of(mapping[node_id])
        if mapping[node_id] != node_id:
            attrs["terraform_id"] = node_id
        graph.add_node(mapping[node_id], attrs)
    for edge in raw.edges:
        graph.add_edge(mapping[edge.source], mapping[edge.target], dict(edge.attrs))
    return graph


def _collisions(mapping: dict[str, str]) -> set[str]:
    seen: dict[str, str] = {}
    collisions: set[str] = set()
    for original, stripped in mapping.items():
        if stripped in seen and seen[stripped] != original:
            collisions.add(stripped)
        seen[stripped] = original
    return collisions


def detect(text: str) -> bool:
    head = text[:4000]
    return "[root] " in head and ('compound = "true"' in head or 'newrank = "true"' in head)


PROFILE = Profile(
    name="terraform",
    summary="strips the [root] decoration, groups by resource kind",
    produced_by="terraform graph",
    edge_semantics="depends-on",
    load=load,
    detect=detect,
)
