"""Turning an analysis into a CI decision.

Four flags, not a language. Each one states its verdict, what matched, and the
path that proves it — a gate that says "no" without saying why is a gate teams
disable.

A policy that fails sets the exit code to 1. Everything a policy reports is in
the JSON output too, so a pipeline can act on the detail rather than on the
exit code alone.
"""

from __future__ import annotations

from dataclasses import dataclass, field

from dagreach.analysis import ImpactReport, WitnessIndex
from dagreach.model import Graph
from dagreach.selectors import Selector


@dataclass(slots=True)
class PolicyResult:
    """The verdict of one policy, and the evidence behind it."""

    policy: str
    subject: str
    failed: bool
    detail: str
    matched: list[str] = field(default_factory=list)
    witnesses: dict[str, list[str]] = field(default_factory=dict)

    def as_json(self) -> dict:
        return {
            "policy": self.policy,
            "subject": self.subject,
            "failed": self.failed,
            "detail": self.detail,
            "matched": self.matched,
            "witnesses": self.witnesses,
        }


def fail_if_reaches(
    graph: Graph, report: ImpactReport, selector: Selector, *, witness_limit: int = 3
) -> PolicyResult:
    """Fail when the change reaches anything the selector matches."""
    impacted = set(report.impacted)
    matched = [node for node in selector.select(graph) if node in impacted]
    witnesses = _witnesses_for(matched[:witness_limit], report.witnesses)

    if not matched:
        return PolicyResult(
            "fail-if-reaches",
            str(selector),
            False,
            f"nothing matching {selector} is reached",
        )

    seeds = set(report.seeds)
    only_seeds = all(node in seeds for node in matched)
    detail = f"{len(matched)} node(s) matching {selector} are reached"
    if only_seeds:
        detail += " (they are the changed nodes themselves)"
    return PolicyResult("fail-if-reaches", str(selector), True, detail, matched, witnesses)


def max_impacted(report: ImpactReport, ceiling: int) -> PolicyResult:
    """Fail when a change touches more of the graph than the team accepts."""
    size = len(report.impacted)
    failed = size > ceiling
    detail = (
        f"{size} node(s) impacted, ceiling is {ceiling}"
        if failed
        else f"{size} node(s) impacted, within the ceiling of {ceiling}"
    )
    return PolicyResult("max-impacted", str(ceiling), failed, detail)


def fail_on_cycle(cycles: list[list[str]], *, limit: int = 3) -> PolicyResult:
    """Fail when the graph is not the acyclic thing it claims to be."""
    if not cycles:
        return PolicyResult("fail-on-cycle", "cycle", False, "the graph is acyclic")
    matched = [", ".join(cycle) for cycle in cycles[:limit]]
    return PolicyResult(
        "fail-on-cycle",
        "cycle",
        True,
        f"{len(cycles)} cycle(s) found",
        matched,
    )


def _witnesses_for(nodes: list[str], index: WitnessIndex) -> dict[str, list[str]]:
    return {node: index.path(node) for node in nodes if index.path(node)}


def any_failed(results: list[PolicyResult]) -> bool:
    return any(result.failed for result in results)


def fail_on_new_reach(exposures: list, selector: Selector) -> PolicyResult:
    """Fail when the change reaches a matching target it did not reach before.

    "Did not reach before" is judged on the pair (matches the selector, is
    reached), so a target that was always reachable and has just been
    reclassified counts too. See dagreach.diff.newly_exposed.
    """
    if not exposures:
        return PolicyResult(
            "fail-on-new-reach",
            str(selector),
            False,
            f"nothing matching {selector} became reachable",
        )
    return PolicyResult(
        "fail-on-new-reach",
        str(selector),
        True,
        f"{len(exposures)} target(s) matching {selector} became reachable",
        [exposure.target for exposure in exposures],
        {exposure.target: exposure.path for exposure in exposures if exposure.path},
    )
