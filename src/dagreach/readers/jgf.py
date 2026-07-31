"""A reader for JSON Graph Format (https://jsongraphformat.info/).

Both published node shapes are accepted — the object keyed by id, and the list
of objects carrying an `id` — as is the bare `{"nodes": [...], "edges": [...]}`
document that tools emit in practice. Anything accepted beyond the published
specification is recorded as a warning rather than absorbed silently.
"""

from __future__ import annotations

import json
from typing import Any

from dagreach.errors import ParseError
from dagreach.model import Graph


def parse_jgf(text: str, *, source: str | None = None) -> Graph:
    """Parse JSON Graph Format text into a :class:`~dagreach.model.Graph`."""
    try:
        document = json.loads(text)
    except json.JSONDecodeError as exc:
        raise ParseError(
            f"invalid JSON: {exc.msg}", source=source, line=exc.lineno, column=exc.colno
        ) from exc

    if not isinstance(document, dict):
        raise ParseError("expected a JSON object at the top level", source=source)

    graph = Graph(source=source)
    body = _select_graph_body(document, graph, source)

    graph.name = _as_text(body.get("label")) or _as_text(body.get("id"))
    directed = body.get("directed", True)
    if not isinstance(directed, bool):
        graph.warn(f"'directed' is not a boolean ({directed!r}); assuming a directed graph")
        directed = True
    graph.directed = directed
    if not graph.directed:
        graph.warn(
            "the input is an undirected graph; dagreach reads every edge as source -> target"
        )

    graph.attrs.update(_flatten(body.get("metadata"), graph, "graph metadata"))

    _read_nodes(body.get("nodes"), graph, source)
    _read_edges(body, graph, source)
    return graph


def _select_graph_body(
    document: dict[str, Any], graph: Graph, source: str | None
) -> dict[str, Any]:
    if "graphs" in document:
        graphs = document["graphs"]
        if not isinstance(graphs, list) or not graphs:
            raise ParseError("'graphs' must be a non-empty array", source=source)
        if len(graphs) > 1:
            graph.warn(f"the document holds {len(graphs)} graphs; only the first one was read")
        body = graphs[0]
    elif "graph" in document:
        body = document["graph"]
    else:
        if "nodes" not in document and "edges" not in document:
            raise ParseError(
                "expected a 'graph', 'graphs', or a top-level 'nodes'/'edges' object", source=source
            )
        graph.warn(
            "the document has no 'graph' envelope; read as a bare nodes/edges object "
            "(outside the JSON Graph Format specification)"
        )
        body = document

    if not isinstance(body, dict):
        raise ParseError("the graph must be a JSON object", source=source)
    return body


def _read_nodes(nodes: Any, graph: Graph, source: str | None) -> None:
    if nodes is None:
        return

    if isinstance(nodes, dict):
        for node_id, payload in nodes.items():
            graph.add_node(str(node_id), _node_attrs(payload, graph, node_id))
        return

    if isinstance(nodes, list):
        for position, payload in enumerate(nodes):
            if not isinstance(payload, dict):
                raise ParseError(
                    f"node #{position} must be an object, found {type(payload).__name__}",
                    source=source,
                )
            if "id" not in payload:
                raise ParseError(f"node #{position} has no 'id'", source=source)
            node_id = str(payload["id"])
            graph.add_node(node_id, _node_attrs(payload, graph, node_id))
        return

    raise ParseError(
        f"'nodes' must be an object or an array, found {type(nodes).__name__}", source=source
    )


def _read_edges(body: dict[str, Any], graph: Graph, source: str | None) -> None:
    edges = body.get("edges")
    if edges is None:
        edges = body.get("links")
        if edges is not None:
            graph.warn("edges were read from 'links' (outside the JSON Graph Format specification)")
    if edges is None:
        return
    if not isinstance(edges, list):
        raise ParseError(f"'edges' must be an array, found {type(edges).__name__}", source=source)

    for position, payload in enumerate(edges):
        if not isinstance(payload, dict):
            raise ParseError(
                f"edge #{position} must be an object, found {type(payload).__name__}", source=source
            )
        source_id = _endpoint(payload, ("source", "from"), position, "source", graph, source)
        target_id = _endpoint(payload, ("target", "to"), position, "target", graph, source)
        attrs = _flatten(payload.get("metadata"), graph, f"edge #{position} metadata")
        for key in ("relation", "label", "directed"):
            value = _as_text(payload.get(key))
            if value is not None:
                attrs.setdefault(key, value)
        for endpoint in (source_id, target_id):
            if not graph.has_node(endpoint):
                graph.warn(f"edge #{position} refers to undeclared node {endpoint!r}")
        graph.add_edge(source_id, target_id, attrs)


def _endpoint(
    payload: dict[str, Any],
    keys: tuple[str, ...],
    position: int,
    what: str,
    graph: Graph,
    source: str | None,
) -> str:
    for index, key in enumerate(keys):
        if key in payload:
            if index > 0:
                graph.warn(
                    f"edge #{position} uses '{key}' instead of '{keys[0]}' "
                    "(outside the JSON Graph Format specification)"
                )
            return str(payload[key])
    raise ParseError(f"edge #{position} has no {what}", source=source)


def _node_attrs(payload: Any, graph: Graph, node_id: Any) -> dict[str, str]:
    if payload is None:
        return {}
    if not isinstance(payload, dict):
        graph.warn(f"node {str(node_id)!r} has a non-object body; its attributes were ignored")
        return {}
    attrs = _flatten(payload.get("metadata"), graph, f"node {str(node_id)!r} metadata")
    for key, value in payload.items():
        if key in {"metadata", "id"}:
            continue
        text = _as_text(value)
        if text is not None:
            attrs.setdefault(key, text)
    return attrs


def _flatten(metadata: Any, graph: Graph, what: str) -> dict[str, str]:
    if metadata is None:
        return {}
    if not isinstance(metadata, dict):
        graph.warn(f"{what} is not an object; it was ignored")
        return {}
    flattened: dict[str, str] = {}
    for key, value in metadata.items():
        text = _as_text(value)
        if text is None:
            flattened[str(key)] = json.dumps(value, sort_keys=True)
        else:
            flattened[str(key)] = text
    return flattened


def _as_text(value: Any) -> str | None:
    """Render a scalar as text; return None for containers and missing values."""
    if value is None:
        return None
    if isinstance(value, str):
        return value
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return repr(value) if isinstance(value, float) else str(value)
    return None
