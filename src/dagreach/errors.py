"""Exception types shared by every reader."""

from __future__ import annotations


class DagreachError(Exception):
    """Base class for every error dagreach raises on purpose."""


class ParseError(DagreachError):
    """The input could not be read as the format it claims to be.

    Carries a location so the message can point at the offending line rather
    than at the file as a whole.
    """

    def __init__(
        self,
        message: str,
        *,
        source: str | None = None,
        line: int | None = None,
        column: int | None = None,
    ) -> None:
        self.message = message
        self.source = source
        self.line = line
        self.column = column
        super().__init__(self._format())

    def _format(self) -> str:
        location = self.source or "<input>"
        if self.line is not None:
            location = f"{location}:{self.line}"
            if self.column is not None:
                location = f"{location}:{self.column}"
        return f"{location}: {self.message}"


class UnknownFormatError(DagreachError):
    """The input format could not be determined, and none was given."""
