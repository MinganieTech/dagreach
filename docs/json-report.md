# The JSON report

`--json` is the interface other tools are meant to build on. This page is its contract: what each
field means, what may change without warning, and what may not.

Every report carries `schema_version`, currently **1**.

```bash
dagreach impact graph.dot --changed auth --json | jq '.policies[] | select(.verdict != "pass")'
```

## What is promised

| Promise | Detail |
|---|---|
| **Additive changes do not bump the version** | New keys may appear in any object, and enum-like fields (`verdict`, `reason`, `measure`, `policy`, `profile`) may gain values. **Ignore keys you do not know, and tolerate values you do not know.** |
| **A version bump is required** to remove or rename a key, change a value's type, change a unit or a meaning, or weaken an ordering guarantee below |
| **Array order is deterministic** | Every list of node ids follows the order the source file declared them. Paths follow the path. `policies` follows the order the flags were given. Two runs on the same input produce byte-identical JSON. |
| **Object keys are sorted** | so a report diffs cleanly against another |
| **Numbers are numbers** | never strings; `share` is rounded to four decimals; `cost` is expressed in the unit named by `measure` |
| **`null` means absent, not zero** | `name`, `cost` and `longest_path` are nullable; an empty list is `[]` and never `null` |

**Warnings are prose, not a contract.** `warnings` carries human sentences whose wording changes
freely. Never parse them — everything a program should act on is a structured field somewhere else.

Exit codes are part of the same contract and live in [policies.md](policies.md): `0` ok, `1` a
policy failed, `2` usage, `3` a policy this graph cannot settle, `4` unreadable input.

## Shared shapes

**`longest_path`** — `null` when there is no path.

| Field | Type | Meaning |
|---|---|---|
| `nodes` | `[string]` | the path, in order |
| `edges` | `int` | its length in edges |
| `cost` | `number` | in the unit named by `measure` |
| `weighted` | `bool` | whether durations were declared |
| `measure` | `"duration"` \| `"edges"` | `edges` means the path is structural: no durations, so nothing is "critical" |

**`policies[]`** — one entry per policy evaluated, failed or not.

| Field | Type | Meaning |
|---|---|---|
| `policy` | `string` | `fail-if-reaches`, `max-impacted`, `fail-on-cycle`, `fail-on-new-reach` |
| `subject` | `string` | the selector or ceiling it was given |
| `verdict` | `string` | `pass`, `fail`, or `unknown` — the graph could not settle it |
| `failed` | `bool` | true only for `fail`; kept for readers written against schema 1, and equal to `verdict == "fail"` |
| `detail` | `string` | one sentence, for humans |
| `matched` | `[string]` | what triggered it |
| `witnesses` | `{string: [string]}` | node id to a path proving it, for as many as the limit shows |

## `parse`

| Field | Type | Meaning |
|---|---|---|
| `source`, `format`, `profile` | `string` | the path as given, `dot`/`jgf`/a profile's own name, and the profile applied |
| `name` | `string\|null` | the graph's declared name |
| `directed` | `bool` | as declared; an undirected graph is read as directed and warned about |
| `edge_semantics` | `"feeds"` \| `"depends-on"` | the orientation applied |
| `nodes`, `edges` | `int` | counts as declared |
| `self_loops`, `duplicate_edges` | `int` | oddities worth knowing before trusting a metric |
| `attributes` | object | `nodes_with_duration`, `edges_with_duration`, `statuses` and `groups` as `{value: count}` |
| `warnings` | `[string]` | prose |

## `stats`

Adds to the identity fields above:

| Field | Type | Meaning |
|---|---|---|
| `acyclic` | `bool` | |
| `cycles` | `[[string]]` | one entry per cycle, members in declaration order |
| `condensed` | `bool` | whether the metrics below were measured on the condensation |
| `collapsed_cycles` | `{string: [string]}` | condensed node id to its members |
| `depth`, `width` | `int` | levels, and the largest earliest-start generation |
| `widest_generation` | `[string]` | the nodes of that generation |
| `roots`, `leaves`, `isolated` | `[string]` | |
| `articulation_points` | `[string]` | undirected reading — see [metrics.md](metrics.md) |
| `most_reaching` | `[object]\|null` | every node reaching at least one other, largest first; `{node, reaches, share}`. **`null` means the ranking was not measured** — the graph is over the 25 000-node ceiling — while `[]` means it was measured and nothing reaches anything. |
| `longest_path` | object\|null | |
| `groups` | `{string: int}` | |

## `impact`

| Field | Type | Meaning |
|---|---|---|
| `changed` | `[string]` | the seeds, in declaration order |
| `downstream` | `[string]` | what the change can break, seeds excluded |
| `impacted`, `impacted_count` | `[string]`, `int` | seeds plus downstream |
| `total_nodes`, `share` | `int`, `number` | `share` = impacted ÷ total, four decimals |
| `upstream` | `[string]` | what the change depends on |
| `impacted_leaves` | `[string]` | impacted nodes with no successor |
| `impacted_articulation_points` | `[string]` | seeds that are themselves single points of passage |
| `groups` | `{string: int}` | impacted nodes per group |
| `cost` | `number\|null` | sum of declared durations over the impacted set |
| `longest_path` | object\|null | restricted to the impacted set |
| `explain` | `{string: {distance:int, path:[string]}}` | **only with `--explain`**; a shortest witness per reached node |

## `diff`

| Field | Type | Meaning |
|---|---|---|
| `before`, `after` | `string` | the two paths as given |
| `changed` | `[string]` | the seeds |
| `changed_missing_before`, `changed_missing_after` | `[string]` | seeds absent from one version |
| `reached_before`, `reached_after` | `[string]` | reach in each version |
| `added_reach`, `removed_reach` | `[string]` | the delta — the point of the command |
| `nodes_added`, `nodes_removed` | `[string]` | structural change |
| `edges_added`, `edges_removed` | `[[string, string]]` | each edge as a two-element array |
| `exposures` | `[{target, reason, detail, path}]` | targets newly exposed to a `--fail-on-new-reach` selector; `reason` is `new-node`, `new-edge` or `reclassified` |
| `all_pairs_reachability_delta` | object | **only with the flag**: `added_pairs`, `removed_pairs`, `sources`, `added_by_source`, `removed_by_source` |

## How the shape is held

Promises are worth what enforces them. Nine reports — every command, with and without policies —
are pinned as golden files in `internal/dagreach/testdata/golden/`, and CI compares against them
on Linux, Windows and macOS. Any change to the JSON shape fails the build with a diff.

```bash
go test ./internal/dagreach -run Golden -update   # rewrite them, then read the diff
```

Rewriting them is a deliberate act: the test tells you to explain the change in the commit message
and to bump `SchemaVersion` if a consumer could break.
