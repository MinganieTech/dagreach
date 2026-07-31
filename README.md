# dagreach

**Impact analysis for any DAG — what a change reaches, offline, in CI.**

`nx affected` and `bazel query rdeps` answer "what does this change touch?" — but only inside
their own build graph. `dagreach` answers the same question for *any* directed acyclic graph you
can hand it: a `terraform graph` dump, a dbt manifest, a build graph, a task plan, a dependency
tree, anything that speaks DOT.

No database, no daemon, no runtime to install alongside. A file in, an answer out.

> **Status: pre-alpha (0.0.1).** Reading works; the analysis does not exist yet. Watch the
> repository if you want to be told when it does something useful.

## What works today

`dagreach parse` reads a graph and tells you what it understood — the cheapest way to find out
whether your export is usable before any analysis exists.

```console
$ terraform graph | dagreach parse -
<stdin>: dot, directed, 42 nodes, 61 edges
profile: durations on 0/42 nodes, 0 status value(s), 0 group(s)

$ dagreach parse pipeline.json --json
{ "format": "jgf", "nodes": 5, "edges": 4, "profile": { ... }, "warnings": [] }
```

It reads DOT (`.dot`, `.gv`) and JSON Graph Format (`.json`), from a file or from stdin, and
guesses which one you gave it. Four optional attributes — `duration`, `weight`, `status`, `group` —
carry meaning; see [docs/attribute-profile.md](docs/attribute-profile.md). Everything else is kept
verbatim.

Recoverable oddities are reported, never silently absorbed: parallel edges collapsed by `strict`,
endpoints that were never declared, durations that are not numbers, spellings outside the
specification. Syntax errors point at a line and a column.

## What it will do

```console
$ dagreach impact --changed auth-service infra.dot
# → everything downstream of auth-service, the critical path through it,
#   whether it is an articulation point, how wide the graph still is without it

$ dagreach diff before.dot after.dot --fail-on cycle
# → nodes and edges added/removed, and what that did to the metrics above

$ dagreach stats infra.dot --json
# → depth, width, critical path, articulation points, cycles
```

## Roadmap

| Slice | Content | State |
|---|---|---|
| T0 | Repository, license, CI, command skeleton | done |
| T1 | DOT and JSON Graph Format readers, attribute profile, `parse` | done |
| T2 | Analysis core: reachability, critical path, articulation points, width | next |
| T3 | Structural diff and metric deltas, CI exit codes | |
| T4 | GitHub Action and pull-request comment | |
| T5 | Self-contained HTML report | |
| T6 | Docs, recipes (Terraform, dbt, Airflow), launch | |

Structural linting is deliberately out of scope for 1.0.

## Install

```bash
pipx install dagreach     # or: uvx dagreach
```

## Development

```bash
pip install -e ".[dev]"
ruff check . && pytest
```

## License

MIT
