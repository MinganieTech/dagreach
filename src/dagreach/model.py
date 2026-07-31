"""The internal graph model every reader produces and every command consumes.

Deliberately small: identifiers, attributes, and the order things were seen in.
No adjacency index and no algorithms live here — analysis arrives in T2 and
builds its own structures on top of this.
"""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass(slots=True)
class Node:
    """A vertex. `attrs` holds the raw string attributes as the source spelled them."""

    id: str
    attrs: dict[str, str] = field(default_factory=dict)


@dataclass(slots=True)
class Edge:
    """A directed dependency, read as `source -> target`."""

    source: str
    target: str
    attrs: dict[str, str] = field(default_factory=dict)

    @property
    def key(self) -> tuple[str, str]:
        return (self.source, self.target)


@dataclass(slots=True)
class Graph:
    """A parsed graph, plus what the reader wants the user to know about it.

    `warnings` is part of the contract: readers never fail on a recoverable
    oddity, they record it and carry on, and commands surface it.
    """

    name: str | None = None
    directed: bool = True
    source: str | None = None
    format: str | None = None
    profile: str | None = None
    edge_semantics: str = "feeds"
    nodes: dict[str, Node] = field(default_factory=dict)
    edges: list[Edge] = field(default_factory=list)
    attrs: dict[str, str] = field(default_factory=dict)
    warnings: list[str] = field(default_factory=list)

    # -- construction ----------------------------------------------------

    def add_node(
        self, node_id: str, attrs: dict[str, str] | None = None, *, override: bool = True
    ) -> Node:
        """Declare a node, merging attributes when it is declared more than once.

        `override=False` is how inherited defaults are applied: they fill in what
        is missing and never overwrite a value the source stated explicitly.
        """
        node = self.nodes.get(node_id)
        if node is None:
            node = Node(node_id, dict(attrs or {}))
            self.nodes[node_id] = node
        elif attrs:
            if override:
                node.attrs.update(attrs)
            else:
                for key, value in attrs.items():
                    node.attrs.setdefault(key, value)
        return node

    def add_edge(self, source: str, target: str, attrs: dict[str, str] | None = None) -> Edge:
        """Declare an edge, implicitly declaring its endpoints as DOT does."""
        self.add_node(source)
        self.add_node(target)
        edge = Edge(source, target, dict(attrs or {}))
        self.edges.append(edge)
        return edge

    def warn(self, message: str) -> None:
        self.warnings.append(message)

    # -- inspection ------------------------------------------------------

    @property
    def node_count(self) -> int:
        return len(self.nodes)

    @property
    def edge_count(self) -> int:
        return len(self.edges)

    def has_node(self, node_id: str) -> bool:
        return node_id in self.nodes

    def self_loops(self) -> list[Edge]:
        return [edge for edge in self.edges if edge.source == edge.target]

    def duplicate_edges(self) -> list[tuple[str, str]]:
        """Edge endpoints seen more than once, in first-seen order."""
        seen: set[tuple[str, str]] = set()
        duplicated: dict[tuple[str, str], None] = {}
        for edge in self.edges:
            if edge.key in seen:
                duplicated[edge.key] = None
            seen.add(edge.key)
        return list(duplicated)
