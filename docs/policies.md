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

## `--fail-on-new-reach`: the policy that compares two versions

The other policies judge one graph. This one compares two, and it is the one that catches what a
review misses — an edge that looks harmless but opens a route that did not exist:

```bash
dagreach diff before.dot after.dot --changed auth --explain --fail-on-new-reach group=production
```

```text
before.dot -> after.dot: auth reaches 3 nodes, was 2 (+1, -0)
edges: source feeds target, so impact follows edges forward
new reach (1): payments
structure: 0 node(s) added, 0 removed, 1 edge(s) added, 0 removed
why (1 of 1 shown):
  payments is now reached
    reason: new edge token -> payments
    path:   auth -> token -> payments
policies:
  FAIL fail-on-new-reach group=production: 1 target(s) matching group=production became reachable
    payments: auth -> token -> payments
```

### The definition it implements

**A target is newly exposed when it matches the selector and is reached in `after`, while it did
not both match and get reached in `before`.**

The predicate is on the *pair* — matching and reached — not on reachability alone. That is
deliberately one step broader than "reachable now, unreachable before", and it is what catches the
target that was always reachable and has simply been **reclassified** into `group=production`. No
new edge is required for a policy to fail, and the report always says which of the two happened:

| Reason | What it means |
|---|---|
| `new-node` | the target did not exist in `before` |
| `new-edge` | an edge on the witness path did not exist in `before`; the edge is named |
| `reclassified` | already reachable, and the selector attribute changed; both values are named |
| `newly-reachable` | defensive last resort, not expected to occur |

### The cost

Two multi-source traversals, one per graph, then set differences:

```text
R_before = reachable(before, changed)
R_after  = reachable(after, changed)
added    = R_after - R_before
removed  = R_before - R_after
```

No transitive closure is materialised. Measured on 20 000 nodes and 100 000 edges: **0.3 s**.

## `--all-pairs-reachability-delta`: opt in, and know what you are asking

```bash
dagreach diff before.dot after.dot --all-pairs-reachability-delta [--count-only] [--limit N]
```

This answers the global question — which ordered pairs became reachable anywhere in the graph —
and it is a separate flag for one reason: **the answer itself can hold one entry per pair of
nodes**, so on any real graph it is a result nobody reads. That is a property of the question, not
a promise about the algorithm; dagreach computes it with bitsets over the condensation, and
measured 2.6 s on the same 20 000-node graph.

Consequently:

- it is **not meant for ordinary CI**;
- the result is **aggregated by source** by default, largest first;
- `--count-only` reports the totals alone;
- `--limit` shortens the ranking, and says how many sources it hid.

It never fails a build on its own: no policy is attached to it.
