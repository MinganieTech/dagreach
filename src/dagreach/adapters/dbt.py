"""dbt's `target/manifest.json`.

dbt already answers "what is downstream of this model" through its own
selectors. What it cannot do is answer it **offline**, against a manifest you
did not just produce, or compare two manifests — which is exactly what a change
gate needs.

Edges are read parent to child, so they already run in the direction of impact.
`child_map` is used when the manifest carries it, and `depends_on.nodes` is the
fallback for older manifests.
"""

from __future__ import annotations

import json
from typing import Any

from dagreach.adapters import Profile
from dagreach.errors import ParseError
from dagreach.model import Graph

_SECTIONS = ("nodes", "sources", "exposures", "metrics", "semantic_models")


def load(text: str, source: str | None = None) -> Graph:
    try:
        manifest = json.loads(text)
    except json.JSONDecodeError as exc:
        raise ParseError(
            f"invalid JSON: {exc.msg}", source=source, line=exc.lineno, column=exc.colno
        ) from exc
    if not isinstance(manifest, dict):
        raise ParseError("a dbt manifest must be a JSON object", source=source)

    graph = Graph(source=source, format="dbt")
    metadata = manifest.get("metadata")
    if isinstance(metadata, dict):
        graph.name = metadata.get("project_name")
        for key in ("dbt_version", "dbt_schema_version", "adapter_type"):
            value = metadata.get(key)
            if isinstance(value, str):
                graph.attrs[key] = value

    declared: dict[str, dict[str, Any]] = {}
    for section in _SECTIONS:
        entries = manifest.get(section)
        if not isinstance(entries, dict):
            continue
        for unique_id, entry in entries.items():
            if isinstance(entry, dict):
                declared[unique_id] = entry
                graph.add_node(unique_id, _attrs(unique_id, entry))

    child_map = manifest.get("child_map")
    if isinstance(child_map, dict) and child_map:
        for parent, children in child_map.items():
            for child in children or []:
                graph.add_edge(str(parent), str(child))
    else:
        graph.warn("no 'child_map' in this manifest; edges were read from depends_on.nodes")
        for unique_id, entry in declared.items():
            depends_on = entry.get("depends_on")
            parents = depends_on.get("nodes", []) if isinstance(depends_on, dict) else []
            for parent in parents or []:
                graph.add_edge(str(parent), unique_id)

    undeclared = [node for node in graph.nodes if node not in declared]
    if undeclared:
        graph.warn(
            f"{len(undeclared)} node(s) appear in the dependency maps but in no section of the "
            "manifest (macros and tests of other packages usually explain this)"
        )
    return graph


def _attrs(unique_id: str, entry: dict[str, Any]) -> dict[str, str]:
    attrs: dict[str, str] = {}
    resource_type = entry.get("resource_type") or unique_id.split(".", 1)[0]
    attrs["group"] = str(resource_type)
    for key in ("name", "package_name", "schema", "database", "path"):
        value = entry.get(key)
        if isinstance(value, str):
            attrs[key] = value
    config = entry.get("config")
    if isinstance(config, dict):
        materialized = config.get("materialized")
        if isinstance(materialized, str):
            attrs["materialized"] = materialized
        tags = config.get("tags")
        if isinstance(tags, list) and tags:
            attrs["tags"] = ",".join(str(tag) for tag in tags)
    return attrs


def detect(text: str) -> bool:
    head = text[:4000]
    return '"dbt_schema_version"' in head or '"dbt_version"' in head and '"nodes"' in head


PROFILE = Profile(
    name="dbt",
    summary="reads a manifest offline, groups by resource type, keeps tags and materialisation",
    produced_by="dbt (target/manifest.json)",
    edge_semantics="feeds",
    load=load,
    detect=detect,
)
