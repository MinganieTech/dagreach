"""Comparing what a change could reach before and after.

The interesting question about two versions of a graph is not "which edges
moved" — a review already shows that — but **which reach relationships became
possible**. One added edge can open thousands of routes; that amplification is
what a diff should surface.

The default computation is deliberately cheap: two multi-source traversals, one
per graph.

    R_before = reachable(before, changed)
    R_after  = reachable(after, changed)
    added    = R_after - R_before
    removed  = R_before - R_after

No transitive closure is materialised. The global question — which pairs became
reachable anywhere in the graph — is a separate, explicitly requested analysis,
because its *result* can hold one entry per pair of nodes.
"""

from __future__ import annotations

from dataclasses import dataclass, field

from dagreach.analysis import Adjacency, WitnessIndex, build_adjacency, witnesses
from dagreach.model import Graph
from dagreach.selectors import Selector

#: Why a node is newly reached, most specific cause first. `newly-reachable` is a
#: defensive last resort: a path made only of pre-existing edges, from a seed that
#: existed, cannot open a new route, so it is not expected to appear.
REASONS = ("new-node", "new-edge", "reclassified", "newly-reachable")


@dataclass(slots=True)
class Exposure:
    """A target that the change reaches now and did not reach before."""

    target: str
    reason: str
    detail: str
    path: list[str] = field(default_factory=list)

    def as_json(self) -> dict:
        return {
            "target": self.target,
            "reason": self.reason,
            "detail": self.detail,
            "path": self.path,
        }


@dataclass(slots=True)
class ReachDiff:
    """What changed between two graphs, from the point of view of the change."""

    seeds: list[str]
    seeds_missing_before: list[str]
    seeds_missing_after: list[str]
    reached_before: list[str]
    reached_after: list[str]
    added: list[str]
    removed: list[str]
    nodes_added: list[str]
    nodes_removed: list[str]
    edges_added: list[tuple[str, str]]
    edges_removed: list[tuple[str, str]]
    witnesses_after: WitnessIndex

    @property
    def unchanged(self) -> bool:
        return not (self.added or self.removed)


def _reach(graph: Graph, seeds: list[str], adjacency: Adjacency) -> tuple[set[str], WitnessIndex]:
    present = [seed for seed in seeds if graph.has_node(seed)]
    index = witnesses(graph, present, "down", adjacency=adjacency)
    return set(index.came_from), index


def reach_diff(before: Graph, after: Graph, seeds: list[str]) -> ReachDiff:
    """The reach delta from `seeds`, plus the structural change that explains it."""
    before_adjacency = build_adjacency(before)
    after_adjacency = build_adjacency(after)

    reached_before, _ = _reach(before, seeds, before_adjacency)
    reached_after, after_index = _reach(after, seeds, after_adjacency)

    before_edges = {edge.key for edge in before.edges}
    after_edges = {edge.key for edge in after.edges}
    # Computed once: inside the comprehensions below these would be rebuilt per node,
    # which turns a linear pass into a quadratic one.
    gained = reached_after - reached_before
    lost = reached_before - reached_after

    return ReachDiff(
        seeds=seeds,
        seeds_missing_before=[seed for seed in seeds if not before.has_node(seed)],
        seeds_missing_after=[seed for seed in seeds if not after.has_node(seed)],
        reached_before=[node for node in before.nodes if node in reached_before],
        reached_after=[node for node in after.nodes if node in reached_after],
        added=[node for node in after.nodes if node in gained],
        removed=[node for node in before.nodes if node in lost],
        nodes_added=[node for node in after.nodes if node not in before.nodes],
        nodes_removed=[node for node in before.nodes if node not in after.nodes],
        edges_added=[key for key in _ordered_edges(after) if key not in before_edges],
        edges_removed=[key for key in _ordered_edges(before) if key not in after_edges],
        witnesses_after=after_index,
    )


def _ordered_edges(graph: Graph) -> list[tuple[str, str]]:
    seen: dict[tuple[str, str], None] = {}
    for edge in graph.edges:
        seen[edge.key] = None
    return list(seen)


def newly_exposed(
    before: Graph,
    after: Graph,
    diff: ReachDiff,
    selector: Selector,
) -> list[Exposure]:
    """Targets the change reaches now and did not reach before.

    A target counts as exposed when it **matches the selector and is reached**
    in `after`, while it did not both match and get reached in `before`. Stated
    on the pair rather than on reachability alone, this also catches the target
    that was always reachable and has just been reclassified — no new edge
    required, and the reason says which of the two happened.
    """
    reached_before = set(diff.reached_before)
    reached_after = set(diff.reached_after)
    before_edges = {edge.key for edge in before.edges}

    exposed_before = {node for node in selector.select(before) if node in reached_before}

    exposures: list[Exposure] = []
    for target in selector.select(after):
        if target not in reached_after or target in exposed_before:
            continue
        path = diff.witnesses_after.path(target)
        exposures.append(
            _explain(before, after, target, path, before_edges, reached_before, selector)
        )
    return exposures


def _explain(
    before: Graph,
    after: Graph,
    target: str,
    path: list[str],
    before_edges: set[tuple[str, str]],
    reached_before: set[str],
    selector: Selector,
) -> Exposure:
    if not before.has_node(target):
        return Exposure(target, "new-node", f"{target!r} did not exist before", path)

    new_edge = next(
        ((a, b) for a, b in zip(path, path[1:], strict=False) if (a, b) not in before_edges),
        None,
    )
    if new_edge is not None:
        return Exposure(target, "new-edge", f"new edge {new_edge[0]} -> {new_edge[1]}", path)

    if target in reached_before and not selector.matches(before, target):
        was = before.nodes[target].attrs.get(selector.key, "unset")
        return Exposure(
            target,
            "reclassified",
            f"already reachable, and {selector.key} changed from {was!r} to {selector.value!r}",
            path,
        )

    return Exposure(
        target,
        "newly-reachable",
        "reachable through edges that already existed; an upstream change opened the route",
        path,
    )


# --------------------------------------------------------------------------
# the opt-in global analysis
# --------------------------------------------------------------------------


@dataclass(slots=True)
class AllPairsDelta:
    """How many ordered pairs became — or stopped being — reachable, per source."""

    added_total: int
    removed_total: int
    added_by_source: dict[str, int]
    removed_by_source: dict[str, int]
    sources: int


def all_pairs_delta(before: Graph, after: Graph) -> AllPairsDelta:
    """Reachability delta over every ordered pair of nodes.

    Aggregated by source on purpose: the full answer holds one entry per pair,
    so a graph of any size produces a result no one reads. Reachable sets are
    built as bitsets in reverse topological order, which keeps the work
    proportional to the edges rather than to the pairs — but the size of the
    answer is the reason this analysis is opt-in, not its speed.
    """
    universe = [node for node in after.nodes] + [
        node for node in before.nodes if node not in after.nodes
    ]
    bit_of = {node: index for index, node in enumerate(universe)}

    before_sets = _reachable_bitsets(before, bit_of)
    after_sets = _reachable_bitsets(after, bit_of)

    added_by_source: dict[str, int] = {}
    removed_by_source: dict[str, int] = {}
    added_total = removed_total = 0

    for node in universe:
        before_bits = before_sets.get(node, 0)
        after_bits = after_sets.get(node, 0)
        added = (after_bits & ~before_bits).bit_count()
        removed = (before_bits & ~after_bits).bit_count()
        if added:
            added_by_source[node] = added
            added_total += added
        if removed:
            removed_by_source[node] = removed
            removed_total += removed

    return AllPairsDelta(
        added_total=added_total,
        removed_total=removed_total,
        added_by_source=added_by_source,
        removed_by_source=removed_by_source,
        sources=len(universe),
    )


def _reachable_bitsets(graph: Graph, bit_of: dict[str, int]) -> dict[str, int]:
    """One bitset per node: everything it reaches, itself excluded."""
    from dagreach.analysis import condense, strongly_connected_cycles

    working, member_of = graph, {node: node for node in graph.nodes}
    if strongly_connected_cycles(graph):
        working, member_of = condense(graph)

    adjacency = build_adjacency(working)
    order = _reverse_topological_order(working, adjacency)

    members: dict[str, list[str]] = {}
    for member, component in member_of.items():
        members.setdefault(component, []).append(member)

    reach_of: dict[str, int] = {}
    for node in order:
        bits = 0
        for successor in adjacency.successors[node]:
            bits |= reach_of[successor]
            for member in members[successor]:
                bits |= 1 << bit_of[member]
        reach_of[node] = bits

    expanded: dict[str, int] = {}
    for component, component_members in members.items():
        bits = reach_of[component]
        if len(component_members) > 1:
            # Inside a cycle everything reaches everything, itself included.
            for member in component_members:
                bits |= 1 << bit_of[member]
        for member in component_members:
            expanded[member] = bits
    return expanded


def _reverse_topological_order(graph: Graph, adjacency: Adjacency) -> list[str]:
    remaining = {node: len(adjacency.successors[node]) for node in graph.nodes}
    order: list[str] = []
    frontier = [node for node in graph.nodes if remaining[node] == 0]
    while frontier:
        next_frontier: list[str] = []
        for node in frontier:
            order.append(node)
            for predecessor in adjacency.predecessors[node]:
                remaining[predecessor] -= 1
                if remaining[predecessor] == 0:
                    next_frontier.append(predecessor)
        frontier = next_frontier
    return order
