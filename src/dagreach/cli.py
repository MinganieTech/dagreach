"""Command-line entry point.

Argument wiring only: reading lives in `readers`, edge orientation in
`semantics`, the metrics in `analysis`, the CI decision in `policy`, and the
two output shapes in `render`.
"""

from __future__ import annotations

import argparse
import difflib
import json
import sys
from collections.abc import Sequence

from dagreach import __version__, render
from dagreach.adapters import PROFILES
from dagreach.analysis import analyse, impact
from dagreach.diff import all_pairs_delta, newly_exposed, reach_diff
from dagreach.errors import DagreachError
from dagreach.loading import load_graph
from dagreach.model import Graph
from dagreach.policy import (
    PolicyResult,
    any_failed,
    fail_if_reaches,
    fail_on_cycle,
    fail_on_new_reach,
    max_impacted,
)
from dagreach.profile import summarize
from dagreach.readers import FORMATS
from dagreach.selectors import SELECTOR_KEYS, parse_selector
from dagreach.semantics import DEFAULT_SEMANTICS, EDGE_SEMANTICS

# Exit codes are part of the public contract (CI depends on them).
EXIT_OK = 0
EXIT_POLICY_FAILED = 1
EXIT_USAGE = 2
EXIT_NOT_IMPLEMENTED = 3
EXIT_INPUT_ERROR = 4

DEFAULT_LIMIT = 10

_PLANNED: dict[str, str] = {}


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="dagreach",
        # Printed text stays ASCII: a legacy Windows console cannot encode the rest.
        description=(
            "Portable change-impact analysis for dependency graphs. See what a change can "
            "reach, why it reaches it, and whether CI should allow it."
        ),
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
            "articulation points, and the longest path."
        ),
    )
    _add_input_arguments(stats_command)
    _add_listing_arguments(stats_command)
    stats_command.add_argument(
        "--fail-on",
        choices=["cycle"],
        action="append",
        default=[],
        help="exit 1 when the condition holds",
    )

    impact_command = subparsers.add_parser(
        "impact",
        help="what a set of changed nodes reaches",
        description=(
            "Report what changing a set of nodes reaches: everything downstream, why each "
            "node is reached, what the change depends on, and whether CI should allow it."
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
    impact_command.add_argument(
        "--explain",
        action="store_true",
        help="show a shortest witness path for every reached node",
    )
    impact_command.add_argument(
        "--fail-if-reaches",
        metavar="SELECTOR",
        action="append",
        default=[],
        help=f"exit 1 when the change reaches a matching node ({'=, '.join(SELECTOR_KEYS)}=VALUE)",
    )
    impact_command.add_argument(
        "--max-impacted",
        metavar="N",
        type=int,
        help="exit 1 when more than N nodes are impacted",
    )
    impact_command.add_argument(
        "--fail-on",
        choices=["cycle"],
        action="append",
        default=[],
        help="exit 1 when the condition holds",
    )

    diff_command = subparsers.add_parser(
        "diff",
        help="what a change reaches now that it did not reach before",
        description=(
            "Compare two versions of a graph from the point of view of a change: what it "
            "reaches now, what it stopped reaching, why, and whether CI should allow it."
        ),
    )
    diff_command.add_argument("before", metavar="BEFORE", help="the graph as it was")
    diff_command.add_argument("after", metavar="AFTER", help="the graph as it is now")
    diff_command.add_argument(
        "--profile",
        choices=list(PROFILES),
        help="read both files with a producer's conventions (see `dagreach profiles`)",
    )
    diff_command.add_argument(
        "--format", choices=FORMATS, help="read both files as this format instead of detecting it"
    )
    diff_command.add_argument(
        "--edge-semantics",
        choices=EDGE_SEMANTICS,
        help=f"what an edge means in both files (default {DEFAULT_SEMANTICS})",
    )
    diff_command.add_argument("--json", action="store_true", help="emit a JSON report on stdout")
    _add_listing_arguments(diff_command)
    diff_command.add_argument(
        "--changed",
        metavar="ID[,ID...]",
        action="append",
        default=[],
        help="node that changed; repeat the flag or separate ids with commas",
    )
    diff_command.add_argument(
        "--explain",
        action="store_true",
        help="show why each target became reachable, with the path that proves it",
    )
    diff_command.add_argument(
        "--fail-on-new-reach",
        metavar="SELECTOR",
        action="append",
        default=[],
        help=(
            "exit 1 when a matching target is reached in AFTER but was not reached in BEFORE "
            "(a target reclassified into the selector counts too)"
        ),
    )
    diff_command.add_argument(
        "--all-pairs-reachability-delta",
        action="store_true",
        help=(
            "also compare reachability over every ordered pair of nodes. Potentially quadratic: "
            "the ANSWER can hold one entry per pair, so this is not meant for ordinary CI. "
            "Aggregated by source; add --count-only for totals alone and --limit to shorten"
        ),
    )
    diff_command.add_argument(
        "--count-only",
        action="store_true",
        help="with --all-pairs-reachability-delta, report totals without the per-source ranking",
    )

    subparsers.add_parser(
        "profiles",
        help="list the producer profiles dagreach knows",
        description="List the producer profiles: what each one reads, and how its edges point.",
    )

    for name, help_text in _PLANNED.items():
        subparsers.add_parser(name, help=f"[not implemented] {help_text}")
    return parser


def _add_input_arguments(command: argparse.ArgumentParser) -> None:
    command.add_argument(
        "file", metavar="FILE", help="path to a DOT or JSON Graph Format file, or - for stdin"
    )
    command.add_argument(
        "--profile",
        choices=list(PROFILES),
        help=(
            "read the file with a producer's conventions; detected from the content when "
            "omitted (see `dagreach profiles`)"
        ),
    )
    command.add_argument(
        "--format", choices=FORMATS, help="skip format detection and read the file as this format"
    )
    command.add_argument(
        "--edge-semantics",
        choices=EDGE_SEMANTICS,
        help=(
            f"what an edge means (default {DEFAULT_SEMANTICS}): 'feeds' = source feeds target, "
            "'depends-on' = source depends on target, as terraform, bazel and cargo export it"
        ),
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

    if args.command == "profiles":
        for line in render.profiles_text(PROFILES):
            print(line)
        return EXIT_OK

    if args.command in {"parse", "stats", "impact"}:
        try:
            graph = load_graph(
                args.file,
                profile=args.profile,
                format=args.format,
                edge_semantics=args.edge_semantics,
            )
        except DagreachError as exc:
            print(f"dagreach: {exc}", file=sys.stderr)
            return EXIT_INPUT_ERROR
        if args.command == "parse":
            return _run_parse(graph, args)
        if args.command == "stats":
            return _run_stats(graph, args)
        return _run_impact(graph, args)

    if args.command == "diff":
        return _run_diff(args)

    print(
        f"dagreach {__version__}: '{args.command}' is not implemented yet "
        f"({_PLANNED[args.command]}).",
        file=sys.stderr,
    )
    return EXIT_NOT_IMPLEMENTED


def _run_parse(graph: Graph, args: argparse.Namespace) -> int:
    summary = summarize(graph)
    _emit(args.json, render.parse_json(graph, summary), render.parse_text(graph, summary))
    return EXIT_OK


def _run_stats(graph: Graph, args: argparse.Namespace) -> int:
    stats = analyse(graph)
    policies: list[PolicyResult] = []
    if "cycle" in args.fail_on:
        policies.append(fail_on_cycle(stats.cycles))
    _emit(
        args.json,
        render.stats_json(graph, stats, policies),
        render.stats_text(graph, stats, args.limit, policies),
    )
    return EXIT_POLICY_FAILED if any_failed(policies) else EXIT_OK


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

    try:
        selectors = [parse_selector(text) for text in args.fail_if_reaches]
    except DagreachError as exc:
        print(f"dagreach: {exc}", file=sys.stderr)
        return EXIT_USAGE

    report = impact(graph, requested)

    policies = [fail_if_reaches(graph, report, selector) for selector in selectors]
    if args.max_impacted is not None:
        policies.append(max_impacted(report, args.max_impacted))
    if "cycle" in args.fail_on:
        policies.append(fail_on_cycle(analyse(graph).cycles))

    _emit(
        args.json,
        render.impact_json(graph, report, policies, explain=args.explain),
        render.impact_text(graph, report, args.limit, policies, explain=args.explain),
    )
    return EXIT_POLICY_FAILED if any_failed(policies) else EXIT_OK


def _run_diff(args: argparse.Namespace) -> int:
    try:
        before = _load(args.before, args)
        after = _load(args.after, args)
    except DagreachError as exc:
        print(f"dagreach: {exc}", file=sys.stderr)
        return EXIT_INPUT_ERROR

    requested = _split_ids(args.changed)
    if not requested and not args.all_pairs_reachability_delta:
        print(
            "dagreach: diff needs --changed, or --all-pairs-reachability-delta for the "
            "global comparison",
            file=sys.stderr,
        )
        return EXIT_USAGE

    unknown = [node for node in requested if not before.has_node(node) and not after.has_node(node)]
    if unknown:
        for node in unknown:
            print(
                f"dagreach: no node {node!r} in either graph{_suggest(node, after)}",
                file=sys.stderr,
            )
        return EXIT_INPUT_ERROR

    try:
        selectors = [parse_selector(text) for text in args.fail_on_new_reach]
    except DagreachError as exc:
        print(f"dagreach: {exc}", file=sys.stderr)
        return EXIT_USAGE

    diff = reach_diff(before, after, requested)
    exposures = [
        exposure
        for selector in selectors
        for exposure in newly_exposed(before, after, diff, selector)
    ]
    policies = [
        fail_on_new_reach(newly_exposed(before, after, diff, selector), selector)
        for selector in selectors
    ]
    delta = all_pairs_delta(before, after) if args.all_pairs_reachability_delta else None

    _emit(
        args.json,
        render.diff_json(before, after, diff, exposures, policies, all_pairs=delta),
        render.diff_text(
            before,
            after,
            diff,
            exposures,
            policies,
            args.limit,
            explain=args.explain,
            all_pairs=delta,
            count_only=args.count_only,
        ),
    )
    return EXIT_POLICY_FAILED if any_failed(policies) else EXIT_OK


def _load(path: str, args: argparse.Namespace) -> Graph:
    return load_graph(
        path,
        profile=args.profile,
        format=args.format,
        edge_semantics=args.edge_semantics,
    )


def _emit(as_json: bool, document: dict, lines: list[str]) -> None:
    if as_json:
        print(json.dumps(document, indent=2, sort_keys=True))
    else:
        for line in lines:
            print(line)


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
