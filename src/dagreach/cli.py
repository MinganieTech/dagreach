"""Command-line entry point.

Argument wiring only: reading lives in `readers`, the metrics in `analysis`,
and the two output shapes in `render`.
"""

from __future__ import annotations

import argparse
import difflib
import json
import sys
from collections.abc import Sequence

from dagreach import __version__, render
from dagreach.analysis import analyse, impact
from dagreach.errors import DagreachError
from dagreach.model import Graph
from dagreach.profile import summarize
from dagreach.readers import FORMATS, read

# Exit codes are part of the public contract (CI depends on them).
EXIT_OK = 0
EXIT_USAGE = 2
EXIT_NOT_IMPLEMENTED = 3
EXIT_INPUT_ERROR = 4

DEFAULT_LIMIT = 10

_PLANNED = {
    "diff": "structural diff and metric deltas between two graphs (slice T3)",
}


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="dagreach",
        # Printed text stays ASCII: a legacy Windows console cannot encode the rest.
        description="Impact analysis for any DAG: what a change reaches, offline, in CI.",
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
    _add_input_arguments(parse_command)

    stats_command = subparsers.add_parser(
        "stats",
        help="structural metrics of a graph",
        description=(
            "Report the shape of a graph: depth, width, roots and leaves, cycles, "
            "articulation points, and the critical path."
        ),
    )
    _add_input_arguments(stats_command)
    _add_listing_arguments(stats_command)

    impact_command = subparsers.add_parser(
        "impact",
        help="what a set of changed nodes reaches",
        description=(
            "Report what changing a set of nodes reaches: everything downstream, what it "
            "depends on, the cost of the impacted set, and the critical path within it."
        ),
    )
    _add_input_arguments(impact_command)
    _add_listing_arguments(impact_command)
    impact_command.add_argument(
        "--changed",
        metavar="ID[,ID...]",
        action="append",
        required=True,
        help="node that changed; repeat the flag or separate ids with commas",
    )

    for name, help_text in _PLANNED.items():
        subparsers.add_parser(name, help=f"[not implemented] {help_text}")
    return parser


def _add_input_arguments(command: argparse.ArgumentParser) -> None:
    command.add_argument(
        "file", metavar="FILE", help="path to a DOT or JSON Graph Format file, or - for stdin"
    )
    command.add_argument(
        "--format", choices=FORMATS, help="skip format detection and read the file as this format"
    )
    command.add_argument("--json", action="store_true", help="emit a JSON report on stdout")


def _add_listing_arguments(command: argparse.ArgumentParser) -> None:
    command.add_argument(
        "--limit",
        type=int,
        default=DEFAULT_LIMIT,
        metavar="N",
        help=(
            f"how many items to show per list in text mode (default {DEFAULT_LIMIT}, "
            "0 for all); the JSON report is never truncated"
        ),
    )


def main(argv: Sequence[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if args.command is None:
        parser.print_help()
        return EXIT_OK

    if args.command in {"parse", "stats", "impact"}:
        try:
            graph = read(args.file, format=args.format)
        except DagreachError as exc:
            print(f"dagreach: {exc}", file=sys.stderr)
            return EXIT_INPUT_ERROR
        if args.command == "parse":
            return _run_parse(graph, args)
        if args.command == "stats":
            return _run_stats(graph, args)
        return _run_impact(graph, args)

    print(
        f"dagreach {__version__}: '{args.command}' is not implemented yet "
        f"({_PLANNED[args.command]}).",
        file=sys.stderr,
    )
    return EXIT_NOT_IMPLEMENTED


def _run_parse(graph: Graph, args: argparse.Namespace) -> int:
    summary = summarize(graph)
    return _emit(
        args.json,
        render.parse_json(graph, summary),
        render.parse_text(graph, summary),
    )


def _run_stats(graph: Graph, args: argparse.Namespace) -> int:
    stats = analyse(graph)
    return _emit(
        args.json,
        render.stats_json(graph, stats),
        render.stats_text(graph, stats, args.limit),
    )


def _run_impact(graph: Graph, args: argparse.Namespace) -> int:
    requested = _split_ids(args.changed)
    if not requested:
        print("dagreach: --changed needs at least one node id", file=sys.stderr)
        return EXIT_USAGE

    unknown = [node for node in requested if not graph.has_node(node)]
    if unknown:
        for node in unknown:
            print(
                f"dagreach: no node {node!r} in {graph.source}{_suggest(node, graph)}",
                file=sys.stderr,
            )
        return EXIT_INPUT_ERROR

    report = impact(graph, requested)
    return _emit(
        args.json,
        render.impact_json(graph, report),
        render.impact_text(graph, report, args.limit),
    )


def _emit(as_json: bool, document: dict, lines: list[str]) -> int:
    if as_json:
        print(json.dumps(document, indent=2, sort_keys=True))
    else:
        for line in lines:
            print(line)
    return EXIT_OK


def _split_ids(values: Sequence[str]) -> list[str]:
    ids: list[str] = []
    for value in values:
        for part in value.split(","):
            part = part.strip()
            if part and part not in ids:
                ids.append(part)
    return ids


def _suggest(node: str, graph: Graph) -> str:
    close = difflib.get_close_matches(node, list(graph.nodes), n=3, cutoff=0.6)
    if not close:
        return ""
    return "; did you mean " + ", ".join(repr(match) for match in close) + "?"


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
