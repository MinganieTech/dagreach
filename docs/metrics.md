# What every metric means

A number is only useful if you know exactly what was counted. This page is the normative
definition of everything `dagreach stats` and `dagreach impact` report.

Two rules hold everywhere:

- **Nothing is guessed.** When a metric cannot be computed — a critical path in a graph that still
  has a cycle — dagreach says so and reports nothing rather than reporting something plausible.
- **Order is deterministic.** Every list follows the order the source file declared its nodes, so
  two runs on the same input produce byte-identical reports and a diff between two reports means
  something.

## Structure

| Metric | Definition |
|---|---|
| **nodes**, **edges** | as declared, after `strict` collapsing. Parallel edges count once for traversal, but the edge count reports what the file contained. |
| **roots** | nodes with no incoming edge — where work can start. |
| **leaves** | nodes with no outgoing edge — where it ends. |
| **isolated** | nodes with neither, usually a sign of a broken export. |
| **cycles** | strongly connected components of more than one node, plus self-loops. A DAG has none; anything else is a defect in the graph, not in dagreach. |

## Depth and width

Nodes are placed at their **earliest possible position**: a node with no dependency sits at level
0, and every other node sits one level below its deepest predecessor.

- **depth** — the number of levels. It is the shortest schedule achievable with unlimited
  parallelism, counted in steps.
- **width** — the size of the largest level. It is the most work the dependencies ever allow to
  run at the same moment; buying more parallelism than the width is buying idle workers.

This is the width of the earliest-start schedule, not the size of the largest antichain. The two
differ when a node *could* be delayed into a busier level. The scheduling reading is the useful
one, and it is O(nodes + edges); the exact maximum antichain requires bipartite matching and is
not computed.

Both are reported only for an acyclic graph.

## Critical path

The longest path through the graph — the sequence no amount of parallelism can shorten.

- **Without durations**, it is measured in **edges**, and every output says `unweighted`.
- **With durations** (see [the attribute profile](attribute-profile.md)), it is the maximum of
  `sum(node durations) + sum(edge durations)` along a path. Nodes and edges without a duration
  count as zero.

An unweighted path length is never presented as a duration. When several paths tie, the one whose
end node was declared first wins, so the result is stable.

## Articulation points

Nodes whose removal would split the graph into more pieces, computed on the **undirected**
projection (Hopcroft–Tarjan).

Read them as single points of passage: everything behind one depends on it alone, so a failure, a
rewrite, or a long lock there has no way around it. The undirected reading is deliberate — it
answers "does anything else hold this together?", regardless of edge direction. Self-loops are
ignored, since a node cannot disconnect itself.

## Impact

Given a set of **changed** nodes:

| Metric | Definition |
|---|---|
| **downstream** | every node reachable by following edges forward, **excluding** the changed nodes themselves. This is what the change can break. |
| **impacted** | the changed nodes plus everything downstream. |
| **share** | impacted ÷ total nodes. |
| **upstream** | every node reachable backwards — what the change depends on, and therefore what must be healthy for it to work. |
| **impacted leaves** | impacted nodes with no successor; usually the deliverables, the endpoints, the things someone notices. |
| **cost** | the sum of the declared durations over the impacted set, when durations exist. |
| **critical path within the impacted set** | the same critical path definition, restricted to impacted nodes. It is the time the impact costs if everything that can run in parallel does. |
| **groups touched** | impacted nodes counted per `group` attribute. |

An articulation point among the changed nodes is called out explicitly, because it changes how the
number should be read: everything behind it has no alternative route.

## What is deliberately not reported

- **Bridges** (critical edges) — coming with the structural lint, after 1.0.
- **Exact maximum antichain** — see the width note above.
- **Spectral measures** (algebraic connectivity, bottleneck scores) — they would require a linear
  algebra dependency for a number few people can act on.
- **Anything probabilistic.** Every number here is a fact about the graph you passed in.
