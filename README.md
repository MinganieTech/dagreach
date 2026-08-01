# dagreach

**Portable change-impact analysis for dependency graphs.**
**See what a change can reach, why it reaches it, and whether CI should allow it.**

`nx affected` and `bazel query rdeps` answer "what does this change touch?" — but only inside their
own build graph. `dagreach` answers it for a graph produced by anything: a `terraform graph` dump,
a dbt manifest, a pipeline export, a service map, an SBOM, a plan you generated yourself.

Give it a graph and what changed; get the affected surface, the reasons, and a CI decision. No
database, no daemon, no runtime dependency — a file in, an answer out.

DOT carries structure, not meaning: which way an edge points is a convention. **Profiles** carry
that meaning for the producers dagreach knows — Terraform, dbt, CycloneDX — and it warns rather
than guessing for the ones it does not. See [docs/profiles.md](docs/profiles.md).

> **Status: pre-alpha (0.0.1).** Reading, analysis, policies, the reach diff and the first profiles
> work; the CI action and the HTML report do not exist yet. Written in Go and shipped as a single
> static binary: nothing to install alongside it.

## What works today

### `impact` — what a change reaches, why, and whether CI should allow it

```console
$ dagreach impact services.dot --changed auth --explain --fail-if-reaches group=production
services.dot: auth reaches 3 of 3 nodes (100%)
edges: source feeds target, so impact follows edges forward
downstream (2): token, payments
impacted leaves (1): payments
groups touched: core 2, production 1
why (2 of 2 shown):
  token (distance 1): auth -> token
  payments (distance 2): auth -> token -> payments
policies:
  FAIL fail-if-reaches group=production: 1 node(s) matching group=production are reached
    payments: auth -> token -> payments
$ echo $?
1
```

Policies are four flags, not a language — `--fail-if-reaches`, `--max-impacted`, `--fail-on cycle`,
and the reach diff to come. Each one reports its verdict, what matched, and the path that proves
it; the exit code is `1` when a policy fails, and never when the input was simply unreadable. See
[docs/policies.md](docs/policies.md).

The full report, without policies:

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
$ terraform graph | dagreach stats - --edge-semantics depends-on
<stdin>: 42 nodes, 61 edges, acyclic
edges: source depends on target, so impact follows edges backward
shape: depth 7 level(s), width 12 (largest earliest-start generation), 5 root(s), 9 leaf/leaves
longest path: 6 edge(s), structural (no durations declared)
  aws_vpc.main -> aws_subnet.main -> aws_security_group.web (+3 more)
articulation points (3, undirected reading): aws_vpc.main, aws_subnet.main, aws_iam_role.app
```

Forget the `--edge-semantics` on a file dagreach recognises and it says so rather than answering
backwards:

```text
warnings (1):
  - this file looks like terraform graph output, where an edge means 'source depends on target',
    but it was read as 'feeds'; pass --edge-semantics depends-on if impact comes out backwards
```

Width is the most work the dependencies ever allow to run at once; depth is the shortest schedule
with unlimited parallelism. Without durations there is no "critical" path, only a longest one, and
the output says so. A cyclic graph is not an error: the cycles are listed and collapsed before the
metrics that need acyclicity are measured, and the report says it did that. Every definition is in
[docs/metrics.md](docs/metrics.md).

### Profiles — the producer's conventions, so you do not have to know them

```bash
terraform graph > infra.dot
dagreach impact infra.dot --changed aws_vpc.main          # runs backwards, because Terraform does

dbt parse
dagreach impact target/manifest.json --changed source.shop.orders --explain

syft dir:. -o cyclonedx-json > sbom.json
dagreach impact sbom.json --changed 'pkg:npm/qs@6.11.0' --fail-if-reaches group=root
```

Each profile knows the format, the edge direction, and the conventions worth normalising —
Terraform's `[root] aws_vpc.main (expand)` becomes `aws_vpc.main`, dbt resource types and
CycloneDX component types become groups a policy can select on. The profile is detected from the
file, the report says which one was applied, and `--profile` or `--edge-semantics` overrides it.
`dagreach profiles` lists them. Adding one is a few dozen lines: see
[docs/profiles.md](docs/profiles.md).

### `parse` — does my export even load?

```console
$ dagreach parse pipeline.json
pipeline.json: jgf 'etl_daily', directed, 5 nodes, 4 edges
edges: source feeds target, so impact follows edges forward
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
| T3 | Edge semantics, `--explain`, policies and CI exit codes | done |
| T4 | Reach diff between two graphs, `--fail-on-new-reach` | done |
| T5 | Profiles: Terraform, dbt, CycloneDX, generic | done |
| T6 | GitHub Action and pull-request comment | next |
| T6.1 | PASS / FAIL / UNKNOWN verdict qualification | |
| T6.2 | Experimental GitHub Actions profile (see [the decisions](docs/decisions-ci-yaml.md)) | |
| T7 | Self-contained HTML report | |

There is no generic YAML reader and there will not be one: YAML carries no graph semantics, so
reading it generically would mean guessing. A producer profile that happens to read YAML is a
different thing — see [the GitHub Actions decisions](docs/decisions-ci-yaml.md).

Dominators, structural linting and an HTML explorer are deliberately out of scope for 1.0.

## Install

Download the binary for your platform and put it on your PATH. One file, no runtime, no
dependencies. From source:

```bash
go install github.com/MinganieTech/dagreach/cmd/dagreach@latest
```

## Development

```bash
go test ./...
go vet ./... && gofmt -l .
go build ./cmd/dagreach
```

## License

MIT
