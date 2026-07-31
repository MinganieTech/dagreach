# dagreach — working conventions

## Language

**Everything committed to this repository is written in English** — code comments, docstrings,
README and docs, CLI output and error messages, commit messages, issues and pull requests.
Conversation with the owner happens in French; the artifacts never do.

## Scope discipline

`dagreach` answers one question: **what does a change reach in a DAG, and what does that cost?**

- Inputs are files (DOT, JSON Graph Format). No database, no daemon, no runtime dependency on any
  orchestrator. Everything must work offline.
- The visualiser is an output format, never the product.
- Structural linting is deliberately out of scope until 1.0 ships.
- This project has no relationship with any other codebase. Do not import concepts, vocabulary or
  code from elsewhere: no gates, no evidence, no packets, no admission, no governance.

## Engineering rules

- **Runtime dependencies are a deliberate decision, not a reflex.** The only one planned is
  NetworkX (analysis core, T2). Anything else needs a reason written down.
- **Exit codes are a public contract** (`0` ok, `2` usage error, `3` not implemented, `4` the input
  could not be read). CI pipelines depend on them; changing one is a breaking change.
- **A recoverable oddity is a warning, never a failure and never a silence.** Readers record what
  they had to work around and the commands surface it. Anything accepted beyond a specification
  says so in the warning.
- The version lives in `src/dagreach/__init__.py` and nowhere else.
- Metric definitions must be documented and defensible. State the assumption in the output when one
  is made — e.g. an unweighted critical path is measured in edges, and must say so.
- Every slice keeps `ruff check`, `ruff format --check` and `pytest` green on Linux and Windows.
