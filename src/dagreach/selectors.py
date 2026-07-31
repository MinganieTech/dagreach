"""Selecting nodes for a policy.

A deliberately small grammar — `key=value`, three keys — rather than an
expression language. A policy engine is something a project grows into once the
primitives have proven themselves, not something it starts with.
"""

from __future__ import annotations

from dataclasses import dataclass

from dagreach.errors import DagreachError
from dagreach.model import Graph
from dagreach.profile import group_of, status_of

SELECTOR_KEYS = ("group", "status", "node")


class SelectorError(DagreachError):
    """The selector could not be understood."""


@dataclass(frozen=True, slots=True)
class Selector:
    key: str
    value: str

    def __str__(self) -> str:
        return f"{self.key}={self.value}"

    def matches(self, graph: Graph, node_id: str) -> bool:
        if self.key == "node":
            return node_id == self.value
        node = graph.nodes[node_id]
        if self.key == "group":
            return group_of(node) == self.value
        return status_of(node) == self.value

    def select(self, graph: Graph) -> list[str]:
        """Every matching node, in declaration order."""
        return [node_id for node_id in graph.nodes if self.matches(graph, node_id)]


def parse_selector(text: str) -> Selector:
    key, separator, value = text.partition("=")
    key, value = key.strip(), value.strip()
    if not separator or not key or not value:
        raise SelectorError(
            f"cannot read the selector {text!r}; expected KEY=VALUE "
            f"with KEY one of {', '.join(SELECTOR_KEYS)}"
        )
    if key not in SELECTOR_KEYS:
        raise SelectorError(
            f"unknown selector key {key!r}; expected one of {', '.join(SELECTOR_KEYS)}"
        )
    return Selector(key, value)
