"""A CycloneDX SBOM.

The supply-chain question is a reach question: a library is found vulnerable,
or changes licence — which products, images and deployments does it reach?
An SBOM already holds that graph, and `dependencies[].dependsOn` states it
explicitly, so the file needs no interpretation beyond its own direction: a ref
depends on what it lists.

Components keep their name, version, type and licences as attributes, so a
policy can select on them.
"""

from __future__ import annotations

import json
from typing import Any

from dagreach.adapters import Profile
from dagreach.errors import ParseError
from dagreach.model import Graph


def load(text: str, source: str | None = None) -> Graph:
    try:
        document = json.loads(text)
    except json.JSONDecodeError as exc:
        raise ParseError(
            f"invalid JSON: {exc.msg}", source=source, line=exc.lineno, column=exc.colno
        ) from exc
    if not isinstance(document, dict):
        raise ParseError("a CycloneDX document must be a JSON object", source=source)

    graph = Graph(source=source, format="cyclonedx")
    for key in ("bomFormat", "specVersion", "serialNumber", "version"):
        value = document.get(key)
        if value is not None:
            graph.attrs[key] = str(value)

    metadata = document.get("metadata")
    if isinstance(metadata, dict):
        root = metadata.get("component")
        if isinstance(root, dict):
            graph.name = root.get("name")
            graph.add_node(_reference(root), _attrs(root, is_root=True))

    components = document.get("components")
    if isinstance(components, list):
        for component in components:
            if isinstance(component, dict):
                graph.add_node(_reference(component), _attrs(component))

    dependencies = document.get("dependencies")
    if not isinstance(dependencies, list):
        graph.warn("no 'dependencies' array: the SBOM lists components but no relationships")
        return graph

    declared = set(graph.nodes)
    for entry in dependencies:
        if not isinstance(entry, dict):
            continue
        ref = entry.get("ref")
        if not isinstance(ref, str):
            continue
        for target in entry.get("dependsOn") or []:
            graph.add_edge(ref, str(target))

    undeclared = [node for node in graph.nodes if node not in declared]
    if undeclared:
        graph.warn(
            f"{len(undeclared)} reference(s) appear in 'dependencies' but in no component entry"
        )
    return graph


def _reference(component: dict[str, Any]) -> str:
    for key in ("bom-ref", "purl"):
        value = component.get(key)
        if isinstance(value, str) and value:
            return value
    name = component.get("name") or "unnamed"
    version = component.get("version")
    return f"{name}@{version}" if version else str(name)


def _attrs(component: dict[str, Any], *, is_root: bool = False) -> dict[str, str]:
    attrs: dict[str, str] = {
        "group": "root" if is_root else str(component.get("type") or "library")
    }
    for key in ("name", "version", "purl", "publisher"):
        value = component.get(key)
        if isinstance(value, str):
            attrs[key] = value
    licences = _licences(component.get("licenses"))
    if licences:
        attrs["licenses"] = licences
    return attrs


def _licences(licenses: Any) -> str:
    if not isinstance(licenses, list):
        return ""
    names: list[str] = []
    for entry in licenses:
        if not isinstance(entry, dict):
            continue
        licence = entry.get("license")
        if isinstance(licence, dict):
            value = licence.get("id") or licence.get("name")
            if isinstance(value, str):
                names.append(value)
        expression = entry.get("expression")
        if isinstance(expression, str):
            names.append(expression)
    return ",".join(names)


def detect(text: str) -> bool:
    head = text[:2000]
    return '"bomFormat"' in head and "CycloneDX" in head


PROFILE = Profile(
    name="cyclonedx",
    summary="reads an SBOM, groups by component type, keeps versions and licences",
    produced_by="CycloneDX (syft, cdxgen, trivy, ...)",
    edge_semantics="depends-on",
    load=load,
    detect=detect,
)
