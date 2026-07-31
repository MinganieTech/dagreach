# dagreach — working conventions

## Language

**Everything committed to this repository is written in English** — code comments, docstrings,
README and docs, CLI output and error messages, commit messages, issues and pull requests.
Conversation with the owner happens in French; the artifacts never do.

## Scope discipline

`dagreach` answers one question: **what can this change reach, why, and should CI allow it?**
The metrics exist to support that decision; they are not the product.

- Inputs are files (DOT, JSON Graph Format). No database, no daemon, no runtime dependency on any
  orchestrator. Everything must work offline.
- The visualiser is an output format, never the product.
- Structural linting is deliberately out of scope until 1.0 ships.
- This project has no relationship with any other codebase. Do not import concepts, vocabulary or
  code from elsewhere: no gates, no evidence, no packets, no admission, no governance.

## Engineering rules

- **Runtime dependencies are a deliberate decision, not a reflex.** There are none, and the
  analysis core did not need the graph library that was planned for it: traversal, topological
  order, strongly connected components, longest path and articulation points are a few dozen lines
  each. A dependency becomes justified when the mathematics does — exact maximum antichains
  (bipartite matching), spectral bottlenecks — and that decision gets written down here first.
- **Printed text is ASCII.** A Windows console on a legacy code page renders anything else as `?`.
  Docstrings and documentation are free; CLI output is not, and a test enforces it.
- **Exit codes are a public contract** (`0` ok, `1` a policy failed, `2` usage error, `3` not
  implemented, `4` the input could not be read). CI pipelines depend on them; changing one is a
  breaking change. A broken selector or a missing file must never exit `1`: an unusable input must
  not be indistinguishable from a clean gate.
- **Never guess which way an edge points.** The orientation applied is stated in every report, and
  a file whose shape contradicts it raises a warning. A graph read backwards produces confident,
  fluent nonsense - the worst failure this tool can have.
- **Every decision carries its evidence.** A policy verdict ships the matching nodes and a witness
  path. A gate that cannot explain itself gets disabled by the team that depends on it.
- **A recoverable oddity is a warning, never a failure and never a silence.** Readers record what
  they had to work around and the commands surface it. Anything accepted beyond a specification
  says so in the warning.
- The version lives in `src/dagreach/__init__.py` and nowhere else.
- Metric definitions must be documented and defensible. State the assumption in the output when one
  is made — e.g. an unweighted critical path is measured in edges, and must say so.
- Every slice keeps `ruff check`, `ruff format --check` and `pytest` green on Linux and Windows.
