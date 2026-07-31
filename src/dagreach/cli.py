"""Command-line entry point.

`parse` is implemented (T1): it answers "does my file load, and what did you
understand from it?". The analysis commands are declared but still empty — they
arrive with the analysis core.
"""

from __future__ import annotations

import argparse
import json
import sys
from collections.abc import Sequence

from dagreach import __version__
from dagreach.errors import DagreachError
from dagreach.model import Graph
from dagreach.profile import ProfileSummary, summarize
from dagreach.readers import FORMATS, read

# Exit codes are part of the public contract (CI depends on them).
EXIT_OK = 0
EXIT_USAGE = 2
EXIT_NOT_IMPLEMENTED = 3
EXIT_INPUT_ERROR = 4

_PLANNED = {
    "stats": "structural metrics of a graph (slice T2)",
    "impact": "what a set of changed nodes reaches (slice T2)",
    "diff": "structural diff and metric deltas between two graphs (slice T3)",
}


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="dagreach",
        description="Impact analysis for any DAG — what a change reaches, offline, in CI.",
    )
    parser.add_argument("--version", action="version", version=f"dagreach {__version__}")

    subparsers = parser.add_subparsers(dest="command", metavar="COMMAND")

    parse_command = subparsers.add_parser(
        "parse",
        help="read a graph and report what was understood",
        description=(
            "Read a graph and report what was understood: format, size, profile attributes, "
            "and anything the reader had to work around."
        ),
    )
    parse_command.add_argument(
        "file", metavar="FILE", help="path to a DOT or JSON Graph Format file, or - for stdin"
    )
    parse_command.add_argument(
        "--format", choices=FORMATS, help="skip format detection and read the file as this format"
    )
    parse_command.add_argument("--json", action="store_true", help="emit a JSON report on stdout")

    for name, help_text in _PLANNED.items():
        subparsers.add_parser(name, help=f"[not implemented] {help_text}")
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if args.command is None:
        parser.print_help()
        return EXIT_OK

    if args.command == "parse":
        return _run_parse(args)

    print(
        f"dagreach {__version__}: '{args.command}' is not implemented yet "
        f"({_PLANNED[args.command]}).",
        file=sys.stderr,
    )
    return EXIT_NOT_IMPLEMENTED


def _run_parse(args: argparse.Namespace) -> int:
    try:
        graph = read(args.file, format=args.format)
    except DagreachError as exc:
        print(f"dagreach: {exc}", file=sys.stderr)
        return EXIT_INPUT_ERROR

    summary = summarize(graph)
    if args.json:
        print(json.dumps(_as_report(graph, summary), indent=2, sort_keys=True))
    else:
        for line in _as_text(graph, summary):
            print(line)
    return EXIT_OK


def _as_report(graph: Graph, summary: ProfileSummary) -> dict:
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


def _as_text(graph: Graph, summary: ProfileSummary) -> list[str]:
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

    structure_parts = []
    self_loops = len(graph.self_loops())
    duplicates = len(graph.duplicate_edges())
    if self_loops:
        structure_parts.append(f"{self_loops} self-loop(s)")
    if duplicates:
        structure_parts.append(f"{duplicates} duplicated edge(s)")
    if structure_parts:
        lines.append("structure: " + ", ".join(structure_parts))

    warnings = [*graph.warnings, *summary.unreadable]
    if warnings:
        lines.append(f"warnings ({len(warnings)}):")
        lines.extend(f"  - {warning}" for warning in warnings)
    return lines


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
