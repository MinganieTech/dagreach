"""Profiles: what a producer's export means, so you do not have to say it.

DOT and JSON carry structure. They do not carry which way an edge points, what
an identifier is, or what counts as a group — and getting those wrong produces
confident, wrong answers. A profile is the small piece of knowledge that turns
one producer's export into a graph dagreach can reason about:

* the format to read,
* the edge semantics that producer uses,
* identifier and attribute conventions worth normalising.

Profiles are the opposite of a plugin system: three or four exemplary ones,
each a few dozen lines, so that adding a fifth is obvious work rather than an
architectural decision.
"""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass

from dagreach.model import Graph

__all__ = ["PROFILES", "Profile", "detect_profile", "get_profile", "profile_names"]


@dataclass(frozen=True, slots=True)
class Profile:
    """One producer's conventions."""

    name: str
    summary: str
    produced_by: str
    edge_semantics: str
    load: Callable[[str, str | None], Graph]
    detect: Callable[[str], bool]


def _build() -> dict[str, Profile]:
    from dagreach.adapters import cyclonedx, dbt, generic, terraform

    return {
        profile.name: profile
        for profile in (terraform.PROFILE, dbt.PROFILE, cyclonedx.PROFILE, generic.PROFILE)
    }


PROFILES: dict[str, Profile] = _build()


def profile_names() -> list[str]:
    return list(PROFILES)


def get_profile(name: str) -> Profile:
    return PROFILES[name]


def detect_profile(text: str) -> Profile | None:
    """Recognise a producer from the content, or return None rather than guess.

    `generic` never matches here: it is what runs when nothing was recognised,
    and a detection that always succeeds would tell the reader nothing.
    """
    for profile in PROFILES.values():
        if profile.name == "generic":
            continue
        if profile.detect(text):
            return profile
    return None
