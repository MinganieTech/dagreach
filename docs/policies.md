# Policies and exit codes

A policy turns an analysis into a decision a pipeline can act on. Four flags, deliberately — a
policy language is something a project earns once the primitives have proven themselves, not
something it starts with.

Every policy states its verdict, what matched, and the **path that proves it**. A gate that says
"no" without saying why is a gate teams disable.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | ran, and every policy passed |
| `1` | ran, and at least one policy failed |
| `2` | the command line was wrong (unknown flag, unreadable selector) |
| `3` | the command exists but is not implemented yet |
| `4` | the input could not be read (missing file, syntax error, unknown node) |

These are a public contract: a pipeline depends on them, so changing one is a breaking change.
Note that `2` and `4` are *not* policy failures — a broken selector must never look like a clean
gate, and a missing file must never look like an approval.

## The flags

```bash
dagreach impact graph.dot --changed auth-service --fail-if-reaches group=production
dagreach impact graph.dot --changed auth-service --max-impacted 100
dagreach stats  graph.dot --fail-on cycle
```

| Flag | Fails when |
|---|---|
| `--fail-if-reaches SELECTOR` | the impacted set contains a node matching the selector |
| `--max-impacted N` | more than `N` nodes are impacted |
| `--fail-on cycle` | the graph contains a cycle |

Flags can be repeated and combined; the command exits `1` if any of them fails, and the report
lists every policy that was evaluated, whether it failed or not.

When the only matching node is one of the changed nodes themselves, the policy still fails — a
change *to* production is a change that reaches production — but the report says so explicitly, so
the reader is not left guessing.

## Selectors

A small grammar, not an expression language:

| Selector | Matches |
|---|---|
| `group=VALUE` | nodes whose `group` attribute equals `VALUE` |
| `status=VALUE` | nodes whose `status` attribute equals `VALUE` |
| `node=ID` | one node, by exact id |

Anything else is a usage error (exit `2`) with the accepted keys listed. Values are matched
exactly: no globs, no regular expressions, no substring matching. See
[the attribute profile](attribute-profile.md) for where `group` and `status` come from.

## What it looks like

```console
$ dagreach impact services.dot --changed auth --fail-if-reaches group=production
services.dot: auth reaches 3 of 4 nodes (75%)
edges: source feeds target, so impact follows edges forward
downstream (2): token, payments
policies:
  FAIL fail-if-reaches group=production: 1 node(s) matching group=production are reached
    payments: auth -> token -> payments
$ echo $?
1
```

In JSON, the same thing, under `policies`:

```json
{
  "policies": [
    {
      "policy": "fail-if-reaches",
      "subject": "group=production",
      "failed": true,
      "detail": "1 node(s) matching group=production are reached",
      "matched": ["payments"],
      "witnesses": { "payments": ["auth", "token", "payments"] }
    }
  ]
}
```

## Planned: `--fail-on-new-reach` (slice T4)

The next policy compares two graphs rather than judging one, and it is the one that catches what a
review misses — an edge that looks harmless but opens a route that did not exist:

```bash
dagreach diff before.dot after.dot --changed auth-service --fail-on-new-reach group=production
```

**The definition it will implement**, fixed here so it cannot drift: *fail when a node matching the
selector in `after` is reachable from the changed set in `after`, but was not reachable from it in
`before`.*

Stated that way it also catches the case with no new edge at all — a node that was already
reachable and has just been **reclassified** into `group=production`. The report will separate the
cause from the consequence:

```text
FAIL: auth-service now reaches payments-db
reason: new edge auth-service -> token-service
path: auth-service -> token-service -> payments-db
target matched: group=production
```

The computation is two multi-source traversals, one per graph, `added = R_after - R_before` — no
transitive closure is materialised.

A separate, explicitly requested `--all-pairs-reachability-delta` will answer the global question
("which pairs became reachable anywhere in the graph"). Its help text will state that the **result
itself** can hold Θ(V²) pairs, that it is not meant for ordinary CI, that it aggregates by default,
and it will offer `--count-only` and a limit. The cost is in the size of the answer, not in a
promise about the algorithm.
