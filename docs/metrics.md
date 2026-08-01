# What every metric means

A number is only useful if you know exactly what was counted. This page is the normative
definition of everything `dagreach stats` and `dagreach impact` report.

Three rules hold everywhere:

- **Nothing is guessed.** A metric that cannot be computed as defined is reported as not computed,
  never approximated into something plausible.
- **The orientation is stated.** Every report says which way edges were read (see
  [edge semantics](#edge-semantics)), because a graph read backwards produces confident nonsense.
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

When the graph has cycles, both are measured on the **condensation**: each cycle is collapsed into
one node named `scc(first-member+N)`, and the report says it was measured that way. Since the
condensation is acyclic by construction, depth and width are always reported - never null. Refusing to
answer would be a caprice — the shape of the rest of the graph is still a fact — but pretending the
cycle is a straight line would be a lie.

## Longest path, and when it is a critical path

The longest path through the graph — the sequence no amount of parallelism can shorten.

- **Without durations**, it is a **structural longest path**, measured in edges. dagreach calls it
  `longest path` and never `critical path`: without durations, nothing says the longest chain of
  edges is the slowest thing in the system.
- **With durations** (see [the attribute profile](attribute-profile.md)), it is the maximum of
  `sum(node durations) + sum(edge durations)` along a path, and it is reported as the
  `critical path`. Nodes and edges without a duration count as zero. A collapsed cycle costs the
  sum of its members, since all of them have to happen.

When several paths tie, the one whose end node was declared first wins, so the result is stable.

## Articulation points

Nodes whose removal would split the graph into more pieces, computed on the **undirected**
projection (Hopcroft–Tarjan).

Read them as single points of passage: everything behind one depends on it alone, so a failure, a
rewrite, or a long lock there has no way around it. Self-loops are ignored, since a node cannot
disconnect itself.

**The undirected reading is a limitation, not a subtlety**, and it is why this metric sits in
`stats` rather than in the impact answer. The directed question — "does every path from A to B go
through X?" — is answered by dominators, which dagreach does not compute yet. Until it does, treat
articulation points as a hint about fragility, not as a statement about reachability.

## The ranking: most reaching

Every node, ordered by how many other nodes it reaches — the question the rest of `stats` circles
without answering. Articulation points name the nodes a graph cannot route around but not how much
sits behind them; roots name where work starts, not what waits on it.

| | |
|---|---|
| Definition | the number of nodes reachable from it, **itself excluded** |
| Order | largest first, ties in declaration order |
| Omitted | nodes reaching nothing — a ranking of zeroes is not a ranking |
| Under `depends-on` | what breaks when this breaks |
| Under `feeds` | what waits when this is late |

Inside a cycle every member reaches every other **and itself**, because a cycle really does hold its
own members up. That is the one case where a node counts itself, and it is why a node in a cycle can
show a share of 100%.

**Read the top of the list with the shape of the graph in mind.** On a graph with one long spine the
first entries are the head of that spine, each reaching one fewer than the last, and they say little
beyond "this is a chain". The ranking is most informative where branches compete: which of five
sources carries the most, which library has the widest blast radius.

**It is not a dominator ranking.** "Reaches 286" is not "286 become unreachable if it goes away" —
another route may exist. That sharper question needs dominators, which are deliberately deferred;
see below.

**It is measured only up to `RankingCeiling` (25 000 nodes).** The computation holds one reachable
set per node, so its memory grows with the square of the graph: 157 MB at 20 000 nodes, 2.7 GB at
100 000. Past the ceiling `stats` says the ranking was not measured, and the JSON reports
`most_reaching: null` — never an empty list, which would claim the opposite.

## Edge semantics

An edge carries a direction but not a meaning, and exports disagree about it:

| Producer | `A -> B` means |
|---|---|
| `terraform graph`, `bazel query deps`, `cargo tree` | A **depends on** B |
| dbt, Airflow, most pipeline exports | A **feeds** B |

Read the first family as if it were the second, and every impact answer is exactly inverted.
dagreach therefore takes `--edge-semantics {feeds,depends-on}`, defaults to `feeds`, **states the
orientation it applied in every report**, and warns when a file carries a recognisable
dependency-export signature while being read as `feeds`. `depends-on` inputs are reversed once, at
the door, so everything downstream reads a single direction.

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
| **why** (`--explain`) | for each reached node, its distance from the changed set and a **shortest witness path**. Breadth-first, so the witness is the shortest one; neighbours are walked in declaration order, so the witness never changes between runs. A witness is always a real path in the graph you passed in, never a path through a condensation. |

An articulation point among the changed nodes is called out explicitly, because it changes how the
number should be read: everything behind it has no alternative route.

## What is deliberately not reported

- **Dominators** — the directed answer to "is X the only way through?", and the sharper form of the
  ranking above. Planned; see the articulation-point note.
- **Bridges** (critical edges) — coming with the structural lint, after 1.0.
- **Exact maximum antichain** — see the width note above.
- **Spectral measures** (algebraic connectivity, bottleneck scores) — they would require a linear
  algebra dependency for a number few people can act on.
- **Anything probabilistic.** Every number here is a fact about the graph you passed in.
