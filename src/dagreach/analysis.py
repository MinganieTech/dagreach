"""The analysis core: what a graph looks like, and what a change reaches.

Every metric here is defined in docs/metrics.md and computed in pure Python.
No dependency was taken: the algorithms needed at this stage — traversal,
topological order, strongly connected components, longest path, articulation
points — are a few dozen lines each, and a graph library would only start to
pay for itself on the heavier mathematics (exact maximum antichains, spectral
bottlenecks) that dagreach deliberately does not promise yet.

Determinism is a property of the output, not an accident: every traversal walks
nodes in the order the source declared them, so two runs on the same file
produce the same lists, and a diff of two reports is meaningful.
"""

from __future__ import annotations

from dataclasses import dataclass, field

from dagreach.model import Graph
from dagreach.profile import duration_of, group_of

DIRECTIONS = ("down", "up", "both")


# --------------------------------------------------------------------------
# adjacency
# --------------------------------------------------------------------------


@dataclass(slots=True)
class Adjacency:
    """Successor and predecessor lists, deduplicated, in declaration order."""

    successors: dict[str, list[str]]
    predecessors: dict[str, list[str]]

    @property
    def nodes(self) -> list[str]:
        return list(self.successors)

    def neighbours(self, node: str, direction: str) -> list[str]:
        if direction == "down":
            return self.successors[node]
        return self.predecessors[node]


def build_adjacency(graph: Graph) -> Adjacency:
    successors: dict[str, list[str]] = {node_id: [] for node_id in graph.nodes}
    predecessors: dict[str, list[str]] = {node_id: [] for node_id in graph.nodes}
    seen: set[tuple[str, str]] = set()
    for edge in graph.edges:
        if edge.key in seen:
            continue
        seen.add(edge.key)
        successors[edge.source].append(edge.target)
        predecessors[edge.target].append(edge.source)
    return Adjacency(successors, predecessors)


# --------------------------------------------------------------------------
# structure
# --------------------------------------------------------------------------


def reachable(
    graph: Graph, seeds: list[str], direction: str, *, adjacency: Adjacency | None = None
) -> list[str]:
    """Every node reachable from `seeds`, excluding the seeds themselves.

    `direction` is "down" (what depends on the seeds) or "up" (what they depend on).
    """
    adjacency = adjacency or build_adjacency(graph)
    seed_set = set(seeds)
    found: dict[str, None] = {}
    stack = [node for node in reversed(seeds)]
    while stack:
        node = stack.pop()
        for neighbour in adjacency.neighbours(node, direction):
            if neighbour in found or neighbour in seed_set:
                continue
            found[neighbour] = None
            stack.append(neighbour)
    return [node for node in graph.nodes if node in found]


def strongly_connected_cycles(
    graph: Graph, *, adjacency: Adjacency | None = None
) -> list[list[str]]:
    """Cycles, as strongly connected components of more than one node, plus self-loops.

    Tarjan's algorithm, iterative: a deep dependency graph must not blow the
    Python stack.
    """
    adjacency = adjacency or build_adjacency(graph)
    index_of: dict[str, int] = {}
    low_of: dict[str, int] = {}
    on_stack: set[str] = set()
    component_stack: list[str] = []
    counter = 0
    cycles: list[list[str]] = []

    for root in graph.nodes:
        if root in index_of:
            continue
        work: list[tuple[str, int]] = [(root, 0)]
        while work:
            node, child_index = work[-1]
            if child_index == 0:
                index_of[node] = low_of[node] = counter
                counter += 1
                component_stack.append(node)
                on_stack.add(node)

            successors = adjacency.successors[node]
            if child_index < len(successors):
                work[-1] = (node, child_index + 1)
                child = successors[child_index]
                if child not in index_of:
                    work.append((child, 0))
                elif child in on_stack:
                    low_of[node] = min(low_of[node], index_of[child])
                continue

            work.pop()
            if work:
                parent = work[-1][0]
                low_of[parent] = min(low_of[parent], low_of[node])

            if low_of[node] == index_of[node]:
                component: list[str] = []
                while True:
                    member = component_stack.pop()
                    on_stack.discard(member)
                    component.append(member)
                    if member == node:
                        break
                if len(component) > 1:
                    cycles.append(sorted(component))

    for edge in graph.edges:
        if edge.source == edge.target:
            cycles.append([edge.source])

    return cycles


def topological_levels(
    graph: Graph, *, adjacency: Adjacency | None = None
) -> list[list[str]] | None:
    """Nodes grouped by earliest possible position, or None when the graph has a cycle.

    Level 0 holds the nodes with no dependency; a node sits one level below its
    deepest predecessor. The size of the largest level is the parallelism the
    dependencies allow at any single moment.
    """
    adjacency = adjacency or build_adjacency(graph)
    remaining = {node: len(adjacency.predecessors[node]) for node in graph.nodes}
    level_of: dict[str, int] = {}
    frontier = [node for node in graph.nodes if remaining[node] == 0]
    settled = 0

    while frontier:
        next_frontier: list[str] = []
        for node in frontier:
            settled += 1
            level = level_of.setdefault(node, 0)
            for successor in adjacency.successors[node]:
                level_of[successor] = max(level_of.get(successor, 0), level + 1)
                remaining[successor] -= 1
                if remaining[successor] == 0:
                    next_frontier.append(successor)
        frontier = next_frontier

    if settled != len(remaining):
        return None

    depth = max(level_of.values(), default=-1) + 1
    levels: list[list[str]] = [[] for _ in range(depth)]
    for node in graph.nodes:  # declaration order inside each level
        levels[level_of[node]].append(node)
    return levels


def articulation_points(graph: Graph, *, adjacency: Adjacency | None = None) -> list[str]:
    """Nodes whose removal disconnects the graph, read as an undirected one.

    These are the single points of passage: the ones where a failure, a rewrite
    or a lock hurts the most. Hopcroft and Tarjan's algorithm, iterative.
    """
    adjacency = adjacency or build_adjacency(graph)
    neighbours: dict[str, list[str]] = {}
    for node in graph.nodes:
        merged = dict.fromkeys(adjacency.successors[node])
        merged.update(dict.fromkeys(adjacency.predecessors[node]))
        merged.pop(node, None)  # a self-loop disconnects nothing
        neighbours[node] = list(merged)

    discovery: dict[str, int] = {}
    low: dict[str, int] = {}
    parent: dict[str, str | None] = {}
    counter = 0
    found: set[str] = set()

    for root in graph.nodes:
        if root in discovery:
            continue
        root_children = 0
        work: list[tuple[str, int]] = [(root, 0)]
        parent[root] = None
        while work:
            node, child_index = work[-1]
            if child_index == 0:
                discovery[node] = low[node] = counter
                counter += 1

            if child_index < len(neighbours[node]):
                work[-1] = (node, child_index + 1)
                child = neighbours[node][child_index]
                if child == parent.get(node):
                    continue
                if child in discovery:
                    low[node] = min(low[node], discovery[child])
                else:
                    parent[child] = node
                    if node == root:
                        root_children += 1
                    work.append((child, 0))
                continue

            work.pop()
            if work:
                ancestor = work[-1][0]
                low[ancestor] = min(low[ancestor], low[node])
                if ancestor != root and low[node] >= discovery[ancestor]:
                    found.add(ancestor)

        if root_children > 1:
            found.add(root)

    return [node for node in graph.nodes if node in found]


# --------------------------------------------------------------------------
# paths
# --------------------------------------------------------------------------


@dataclass(slots=True)
class CriticalPath:
    """The longest path through the graph, and how it was measured."""

    nodes: list[str] = field(default_factory=list)
    cost: float = 0.0
    weighted: bool = False

    @property
    def edges(self) -> int:
        return max(len(self.nodes) - 1, 0)

    @property
    def unit(self) -> str:
        return "duration" if self.weighted else "edges"

    def describe(self) -> str:
        if not self.nodes:
            return "no path"
        if self.weighted:
            return f"{format_number(self.cost)} of duration over {self.edges} edge(s)"
        return f"{self.edges} edge(s), unweighted"


def uses_durations(graph: Graph) -> bool:
    """Whether any duration is declared; it decides how a path is measured."""
    for node in graph.nodes.values():
        if duration_of(node) is not None:
            return True
    for edge in graph.edges:
        if duration_of(edge) is not None:
            return True
    return False


def critical_path(
    graph: Graph,
    *,
    adjacency: Adjacency | None = None,
    within: set[str] | None = None,
) -> CriticalPath | None:
    """The longest path, weighted by declared durations when there are any.

    Returns None when the graph (or the restriction) still holds a cycle, since
    "longest" then has no meaning. `within` restricts the search to a subset,
    which is how the path through an impacted set is computed.
    """
    adjacency = adjacency or build_adjacency(graph)
    weighted = uses_durations(graph)
    nodes = [node for node in graph.nodes if within is None or node in within]
    included = set(nodes)

    edge_cost: dict[tuple[str, str], float] = {}
    if weighted:
        for edge in graph.edges:
            if edge.source in included and edge.target in included:
                cost = duration_of(edge) or 0.0
                key = edge.key
                edge_cost[key] = max(edge_cost.get(key, 0.0), cost)

    best: dict[str, float] = {}
    came_from: dict[str, str | None] = {}
    order = _topological_order(nodes, adjacency, included)
    if order is None:
        return None

    for node in order:
        node_cost = (duration_of(graph.nodes[node]) or 0.0) if weighted else 0.0
        incoming = [p for p in adjacency.predecessors[node] if p in included]
        if not incoming:
            best[node] = node_cost
            came_from[node] = None
            continue
        chosen = None
        chosen_score = float("-inf")
        for predecessor in incoming:
            step = edge_cost.get((predecessor, node), 0.0) if weighted else 1.0
            score = best[predecessor] + step
            if score > chosen_score:
                chosen_score = score
                chosen = predecessor
        best[node] = chosen_score + node_cost
        came_from[node] = chosen

    if not best:
        return CriticalPath([], 0.0, weighted)

    position = {node: index for index, node in enumerate(order)}
    end = max(best, key=lambda node: (best[node], -position[node]))
    path = [end]
    while came_from[path[-1]] is not None:
        path.append(came_from[path[-1]])  # type: ignore[arg-type]
    path.reverse()
    return CriticalPath(path, best[end], weighted)


def _topological_order(
    nodes: list[str], adjacency: Adjacency, included: set[str]
) -> list[str] | None:
    remaining = {
        node: sum(1 for p in adjacency.predecessors[node] if p in included) for node in nodes
    }
    order: list[str] = []
    frontier = [node for node in nodes if remaining[node] == 0]
    while frontier:
        next_frontier: list[str] = []
        for node in frontier:
            order.append(node)
            for successor in adjacency.successors[node]:
                if successor not in included:
                    continue
                remaining[successor] -= 1
                if remaining[successor] == 0:
                    next_frontier.append(successor)
        frontier = next_frontier
    return order if len(order) == len(nodes) else None


# --------------------------------------------------------------------------
# reports
# --------------------------------------------------------------------------


@dataclass(slots=True)
class GraphStats:
    """Everything `dagreach stats` reports."""

    nodes: int
    edges: int
    roots: list[str]
    leaves: list[str]
    isolated: list[str]
    cycles: list[list[str]]
    depth: int | None
    width: int | None
    widest_level: list[str]
    articulation_points: list[str]
    critical_path: CriticalPath | None
    groups: dict[str, int]

    @property
    def acyclic(self) -> bool:
        return not self.cycles


def analyse(graph: Graph) -> GraphStats:
    adjacency = build_adjacency(graph)
    cycles = strongly_connected_cycles(graph, adjacency=adjacency)
    levels = topological_levels(graph, adjacency=adjacency)

    roots = [n for n in graph.nodes if not adjacency.predecessors[n]]
    leaves = [n for n in graph.nodes if not adjacency.successors[n]]
    isolated = [n for n in roots if not adjacency.successors[n]]

    widest: list[str] = []
    if levels:
        widest = max(levels, key=len)

    groups: dict[str, int] = {}
    for node in graph.nodes.values():
        group = group_of(node)
        if group:
            groups[group] = groups.get(group, 0) + 1

    return GraphStats(
        nodes=graph.node_count,
        edges=graph.edge_count,
        roots=roots,
        leaves=leaves,
        isolated=isolated,
        cycles=cycles,
        depth=len(levels) if levels is not None else None,
        width=len(widest) if levels else None,
        widest_level=widest,
        articulation_points=articulation_points(graph, adjacency=adjacency),
        critical_path=critical_path(graph, adjacency=adjacency) if not cycles else None,
        groups=groups,
    )


@dataclass(slots=True)
class ImpactReport:
    """Everything `dagreach impact` reports."""

    seeds: list[str]
    downstream: list[str]
    upstream: list[str]
    impacted_leaves: list[str]
    impacted_articulation_points: list[str]
    groups: dict[str, int]
    cost: float | None
    critical_path: CriticalPath | None
    total_nodes: int

    @property
    def impacted(self) -> list[str]:
        """The seeds and everything below them — what a change actually touches."""
        return [*self.seeds, *self.downstream]

    @property
    def share(self) -> float:
        return len(self.impacted) / self.total_nodes if self.total_nodes else 0.0


def impact(graph: Graph, seeds: list[str]) -> ImpactReport:
    adjacency = build_adjacency(graph)
    ordered_seeds = [node for node in graph.nodes if node in set(seeds)]
    downstream = reachable(graph, ordered_seeds, "down", adjacency=adjacency)
    upstream = reachable(graph, ordered_seeds, "up", adjacency=adjacency)

    impacted = set(ordered_seeds) | set(downstream)
    weighted = uses_durations(graph)
    cost = None
    if weighted:
        cost = sum(duration_of(graph.nodes[node]) or 0.0 for node in impacted)

    groups: dict[str, int] = {}
    for node_id in graph.nodes:
        if node_id not in impacted:
            continue
        group = group_of(graph.nodes[node_id])
        if group:
            groups[group] = groups.get(group, 0) + 1

    cut_vertices = set(articulation_points(graph, adjacency=adjacency))

    return ImpactReport(
        seeds=ordered_seeds,
        downstream=downstream,
        upstream=upstream,
        impacted_leaves=[n for n in graph.nodes if n in impacted and not adjacency.successors[n]],
        impacted_articulation_points=[n for n in ordered_seeds if n in cut_vertices],
        groups=groups,
        cost=cost,
        critical_path=critical_path(graph, adjacency=adjacency, within=impacted),
        total_nodes=graph.node_count,
    )


def format_number(value: float) -> str:
    """Render a measurement without trailing noise: 12 rather than 12.0."""
    if value == int(value):
        return str(int(value))
    return f"{value:g}"
