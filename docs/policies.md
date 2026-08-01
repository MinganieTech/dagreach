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
| `3` | ran, and at least one policy could not be settled by this graph |
| `4` | the input could not be read (missing file, syntax error, unknown node) |

These are a public contract: a pipeline depends on them, so changing one is a breaking change.
Note that `2` and `4` are *not* policy failures — a broken selector must never look like a clean
gate, and a missing file must never look like an approval. Neither is `3`: see
[the third verdict](#the-third-verdict-unknown) below.

A proven violation outranks an unsettled one, so a run with both exits `1`.

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
| `attr:NAME=VALUE` | nodes whose `NAME` attribute equals `VALUE`, for any `NAME` |

`group=` and `status=` are shorthands for `attr:group=` and `attr:status=`; they exist because
[the attribute profile](attribute-profile.md) gives those two a documented meaning. Everything else
your producer emits — `risk`, `team`, `tier`, `environment` — is reachable through `attr:`, with no
re-export and no mapping of your vocabulary onto ours.

Values are matched exactly: no globs, no regular expressions, no substring matching.

The prefix is required rather than inferred, and the reason is the failure it prevents. If any
unknown key were read as an attribute name, `--fail-if-reaches grup=production` would look for an
attribute called `grup`, match nothing, and pass — a typo turning a gate into a rubber stamp. So a
bare key that is not a shorthand is a usage error (exit `2`), and it names the `attr:` form you
probably meant:

```console
$ dagreach impact services.dot --changed auth --fail-if-reaches risk=high
dagreach: unknown selector key 'risk'; expected one of group, status, node, or attr:risk=high to read it as an attribute
$ echo $?
2
```

`node=ID` gets the same treatment as `--changed`: an id no node carries is a typo, so it exits `4`
with a suggestion rather than passing quietly.

## The third verdict: UNKNOWN

`attr:` can name an attribute this graph knows nothing about. Reporting "nothing matched" would
then be a statement about the *file*, not about the change — and it would read as approval. So a
selector whose attribute **no node declares** is undeterminable:

```console
$ dagreach impact services.dot --changed auth --fail-if-reaches attr:environment=production
policies:
  UNKNOWN fail-if-reaches attr:environment=production: no node in this graph declares 'environment', so nothing can match attr:environment=production and the policy cannot be settled by this file
$ echo $?
3
```

The distinction is between the attribute being absent and the value being absent. `attr:risk=none`
on a graph where every node declares a `risk` is a real pass: the question was asked and answered.
`attr:environment=production` on a graph with no `environment` anywhere was never asked.

A pipeline can treat `3` as it sees fit — block it like `1` while a producer is being fixed, or let
it through with a warning — but it will never be confused with a clean gate.

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
      "verdict": "fail",
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

The three are exhaustive: a target reached now that was not exposed before either did not exist,
or was reached over an edge that did not exist, or was already reachable and changed
classification. A path made only of pre-existing edges, from a seed that already existed, cannot
open a route.

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
