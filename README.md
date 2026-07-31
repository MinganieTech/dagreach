# dagreach

**Impact analysis for any DAG — what a change reaches, offline, in CI.**

`nx affected` and `bazel query rdeps` answer "what does this change touch?" — but only inside
their own build graph. `dagreach` answers the same question for *any* directed acyclic graph you
can hand it: a `terraform graph` dump, a dbt manifest, a build graph, a task plan, a dependency
tree, anything that speaks DOT.

No database, no daemon, no runtime dependency. A file in, an answer out.

> **Status: pre-alpha (0.0.1).** Reading and analysis work; the diff, the CI integration and the
> HTML report do not exist yet.

## What works today

### `impact` — what a change reaches

```console
$ dagreach impact pipeline.json --changed transform_orders
pipeline.json: transform_orders reaches 3 of 5 nodes (60%)
downstream (2): load_warehouse, notify
upstream (2), what the change depends on: extract_orders, extract_customers
impacted leaves (1): notify
cost of the impacted set: 480 of declared duration
critical path within the impacted set: 480 of duration over 1 edge(s)
  transform_orders -> load_warehouse
groups touched: transform 1, load 2
note: transform_orders is an articulation point: everything behind it depends on it alone
```

### `stats` — the shape of the graph

```console
$ terraform graph | dagreach stats -
<stdin>: 42 nodes, 61 edges, acyclic
shape: depth 7 level(s), width 12, 5 root(s), 9 leaf/leaves
critical path: 6 edge(s), unweighted
  aws_vpc.main -> aws_subnet.main -> aws_security_group.web -> ...
articulation points (3): aws_vpc.main, aws_subnet.main, aws_iam_role.app
```

Width is the most work the dependencies ever allow to run at once; depth is the shortest schedule
with unlimited parallelism. A cyclic graph is not an error: the cycles are listed, and the metrics
that need acyclicity are reported as not computed rather than approximated. Every definition is in
[docs/metrics.md](docs/metrics.md).

### `parse` — does my export even load?

```console
$ dagreach parse pipeline.json
pipeline.json: jgf 'etl_daily', directed, 5 nodes, 4 edges
profile: durations on 4/5 nodes and 0/4 edges, 3 status value(s), 3 group(s)
```

Every command takes `--json` for a machine-readable report, reads a file or stdin, and detects DOT
(`.dot`, `.gv`) from JSON Graph Format (`.json`) on its own.

Four optional attributes — `duration`, `weight`, `status`, `group` — carry meaning; see
[docs/attribute-profile.md](docs/attribute-profile.md). Everything else is kept verbatim.

Recoverable oddities are reported, never silently absorbed: parallel edges collapsed by `strict`,
endpoints that were never declared, durations that are not numbers, spellings outside the
specification. Syntax errors point at a line and a column. Text output truncates long lists and
says how many it hid; the JSON report never truncates.

## What it will do

```console
$ dagreach diff before.dot after.dot --fail-on cycle
# → nodes and edges added/removed, and what that did to the metrics above
```

## Roadmap

| Slice | Content | State |
|---|---|---|
| T0 | Repository, license, CI, command skeleton | done |
| T1 | DOT and JSON Graph Format readers, attribute profile, `parse` | done |
| T2 | Analysis core: reachability, critical path, articulation points, width | done |
| T3 | Structural diff and metric deltas, CI exit codes | next |
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
