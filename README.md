# dagreach

**Impact analysis for any DAG — what a change reaches, offline, in CI.**

`nx affected` and `bazel query rdeps` answer "what does this change touch?" — but only inside
their own build graph. `dagreach` answers the same question for *any* directed acyclic graph you
can hand it: a `terraform graph` dump, a dbt manifest, a build graph, a task plan, a dependency
tree, anything that speaks DOT.

No database, no daemon, no runtime to install alongside. A file in, an answer out.

> **Status: pre-alpha (0.0.1).** This release is a skeleton — the command surface is declared, the
> analysis is not implemented yet. Watch the repository if you want to be told when it does
> something useful.

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
| T1 | DOT and JSON Graph Format readers, attribute profile | next |
| T2 | Analysis core: reachability, critical path, articulation points, width | |
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
