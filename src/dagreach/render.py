"""Turning analysis results into the two output shapes: text and JSON.

Text is for a human reading a terminal or a CI log; JSON is the contract other
tools consume. They report the same facts. Long lists are truncated in text
mode only, and a truncation always says how many items it hid — a silent cap
would read as "that is all there is".
"""

from __future__ import annotations

from typing import Any

from dagreach.analysis import CriticalPath, GraphStats, ImpactReport, format_number
from dagreach.model import Graph
from dagreach.profile import ProfileSummary


def listing(items: list[str], limit: int) -> str:
    """Render a list, hiding the tail beyond `limit` but never in silence."""
    if not items:
        return "none"
    if limit <= 0 or len(items) <= limit:
        return ", ".join(items)
    hidden = len(items) - limit
    return ", ".join(items[:limit]) + f" (+{hidden} more)"


def path_line(path: CriticalPath, limit: int) -> str:
    nodes = path.nodes
    if limit > 0 and len(nodes) > limit:
        hidden = len(nodes) - limit
        return " -> ".join(nodes[:limit]) + f" (+{hidden} more)"
    return " -> ".join(nodes)


# --------------------------------------------------------------------------
# parse
# --------------------------------------------------------------------------


def parse_text(graph: Graph, summary: ProfileSummary) -> list[str]:
    orientation = "directed" if graph.directed else "undirected"
    name = f" {graph.name!r}" if graph.name else ""
    lines = [
        f"{graph.source or '<input>'}: {graph.format}{name}, {orientation}, "
        f"{graph.node_count} nodes, {graph.edge_count} edges"
    ]
    lines.append(
        "profile: "
        + ", ".join(
            [
                f"durations on {summary.nodes_with_duration}/{graph.node_count} nodes"
                f" and {summary.edges_with_duration}/{graph.edge_count} edges",
                f"{len(summary.statuses)} status value(s)",
                f"{len(summary.groups)} group(s)",
            ]
        )
    )

    oddities = []
    self_loops = len(graph.self_loops())
    duplicates = len(graph.duplicate_edges())
    if self_loops:
        oddities.append(f"{self_loops} self-loop(s)")
    if duplicates:
        oddities.append(f"{duplicates} duplicated edge(s)")
    if oddities:
        lines.append("structure: " + ", ".join(oddities))

    lines.extend(warning_lines([*graph.warnings, *summary.unreadable]))
    return lines


def parse_json(graph: Graph, summary: ProfileSummary) -> dict[str, Any]:
    return {
        "source": graph.source,
        "format": graph.format,
        "name": graph.name,
        "directed": graph.directed,
        "nodes": graph.node_count,
        "edges": graph.edge_count,
        "self_loops": len(graph.self_loops()),
        "duplicate_edges": len(graph.duplicate_edges()),
        "profile": {
            "nodes_with_duration": summary.nodes_with_duration,
            "edges_with_duration": summary.edges_with_duration,
            "statuses": dict(summary.statuses),
            "groups": dict(summary.groups),
        },
        "warnings": [*graph.warnings, *summary.unreadable],
    }


# --------------------------------------------------------------------------
# stats
# --------------------------------------------------------------------------


def stats_text(graph: Graph, stats: GraphStats, limit: int) -> list[str]:
    shape = "acyclic" if stats.acyclic else f"{len(stats.cycles)} cycle(s)"
    lines = [f"{graph.source or '<input>'}: {stats.nodes} nodes, {stats.edges} edges, {shape}"]

    if stats.acyclic:
        lines.append(
            f"shape: depth {stats.depth} level(s), width {stats.width}, "
            f"{len(stats.roots)} root(s), {len(stats.leaves)} leaf/leaves"
        )
        if stats.critical_path and stats.critical_path.nodes:
            lines.append(f"critical path: {stats.critical_path.describe()}")
            lines.append("  " + path_line(stats.critical_path, limit))
        if stats.widest_level:
            lines.append(f"widest level: {listing(stats.widest_level, limit)}")
    else:
        lines.append(
            "depth, width and the critical path are not computed while cycles remain; "
            "break them first"
        )
        for cycle in stats.cycles[:limit]:
            lines.append(f"  cycle: {listing(cycle, limit)}")
        if limit > 0 and len(stats.cycles) > limit:
            lines.append(f"  (+{len(stats.cycles) - limit} more cycle(s))")

    lines.append(
        f"articulation points ({len(stats.articulation_points)}): "
        f"{listing(stats.articulation_points, limit)}"
    )
    if stats.isolated:
        lines.append(f"isolated ({len(stats.isolated)}): {listing(stats.isolated, limit)}")
    if stats.groups:
        lines.append(
            "groups: " + ", ".join(f"{name} {count}" for name, count in stats.groups.items())
        )

    lines.extend(warning_lines(graph.warnings))
    return lines


def stats_json(graph: Graph, stats: GraphStats) -> dict[str, Any]:
    return {
        "source": graph.source,
        "nodes": stats.nodes,
        "edges": stats.edges,
        "acyclic": stats.acyclic,
        "cycles": stats.cycles,
        "depth": stats.depth,
        "width": stats.width,
        "widest_level": stats.widest_level,
        "roots": stats.roots,
        "leaves": stats.leaves,
        "isolated": stats.isolated,
        "articulation_points": stats.articulation_points,
        "critical_path": critical_path_json(stats.critical_path),
        "groups": stats.groups,
        "warnings": graph.warnings,
    }


# --------------------------------------------------------------------------
# impact
# --------------------------------------------------------------------------


def impact_text(graph: Graph, report: ImpactReport, limit: int) -> list[str]:
    impacted = report.impacted
    percent = round(report.share * 100)
    lines = [
        f"{graph.source or '<input>'}: {listing(report.seeds, limit)} "
        f"reaches {len(impacted)} of {report.total_nodes} nodes ({percent}%)"
    ]
    lines.append(f"downstream ({len(report.downstream)}): {listing(report.downstream, limit)}")
    if report.upstream:
        lines.append(
            f"upstream ({len(report.upstream)}), what the change depends on: "
            f"{listing(report.upstream, limit)}"
        )
    if report.impacted_leaves:
        lines.append(
            f"impacted leaves ({len(report.impacted_leaves)}): "
            f"{listing(report.impacted_leaves, limit)}"
        )
    if report.cost is not None:
        lines.append(f"cost of the impacted set: {format_number(report.cost)} of declared duration")
    if report.critical_path and report.critical_path.nodes:
        lines.append(f"critical path within the impacted set: {report.critical_path.describe()}")
        lines.append("  " + path_line(report.critical_path, limit))
    if report.groups:
        lines.append(
            "groups touched: "
            + ", ".join(f"{name} {count}" for name, count in report.groups.items())
        )
    if report.impacted_articulation_points:
        lines.append(
            "note: "
            + listing(report.impacted_articulation_points, limit)
            + " is an articulation point: everything behind it depends on it alone"
        )

    lines.extend(warning_lines(graph.warnings))
    return lines


def impact_json(graph: Graph, report: ImpactReport) -> dict[str, Any]:
    return {
        "source": graph.source,
        "changed": report.seeds,
        "impacted": report.impacted,
        "impacted_count": len(report.impacted),
        "total_nodes": report.total_nodes,
        "share": round(report.share, 4),
        "downstream": report.downstream,
        "upstream": report.upstream,
        "impacted_leaves": report.impacted_leaves,
        "impacted_articulation_points": report.impacted_articulation_points,
        "groups": report.groups,
        "cost": report.cost,
        "critical_path": critical_path_json(report.critical_path),
        "warnings": graph.warnings,
    }


def critical_path_json(path: CriticalPath | None) -> dict[str, Any] | None:
    if path is None:
        return None
    return {
        "nodes": path.nodes,
        "edges": path.edges,
        "cost": path.cost,
        "weighted": path.weighted,
        "unit": path.unit,
    }


def warning_lines(warnings: list[str]) -> list[str]:
    if not warnings:
        return []
    return [f"warnings ({len(warnings)}):", *(f"  - {warning}" for warning in warnings)]
