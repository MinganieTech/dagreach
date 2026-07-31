"""Command-line entry point.

This is the T0 skeleton: the command surface is declared so that later slices
have a stable place to land, but no analysis is implemented yet.
"""

from __future__ import annotations

import argparse
import sys
from collections.abc import Sequence

from dagreach import __version__

# Exit codes are part of the public contract (CI depends on them).
EXIT_OK = 0
EXIT_USAGE = 2
EXIT_NOT_IMPLEMENTED = 3

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
    for name, help_text in _PLANNED.items():
        subparsers.add_parser(name, help=f"[not implemented] {help_text}")
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if args.command is None:
        parser.print_help()
        return EXIT_OK

    print(
        f"dagreach {__version__}: '{args.command}' is not implemented yet "
        f"({_PLANNED[args.command]}).",
        file=sys.stderr,
    )
    return EXIT_NOT_IMPLEMENTED


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
