# Reading a CI pipeline: the decisions a profile would have to make

**Status: proposed, not implemented.** No YAML dependency is in `go.mod`, and no CI profile exists.
This page exists so the decisions can be judged *before* any code, because they — not the parser —
are what decides whether these profiles deserve to exist.

YAML is only the envelope. The dependency graph of a pipeline is one field inside it:

```yaml
jobs:
  build:                 # a node
  test:
    needs: build         # an edge: test depends on build
  deploy:
    needs: [test, lint]  # two edges
```

Once the file is parsed, extracting that is a dozen lines. The difficulty is everywhere else: in a
CI file the graph is often **not written**, it has to be **deduced**, and a deduction that is not
visible is exactly the failure mode this tool exists to avoid.

The rule that governs every decision below is the one already in force for edge direction:
**dagreach may deduce, but it must say so.** Every deduction lands in the report as an attribute or
a warning.

---

## What a node is, and what it is not

| Decision | Choice | Why, and what it costs |
|---|---|---|
| **N1 — a node is a job** | jobs, never steps | A step is not independently schedulable and nothing depends on it; a job is what CI retries, gates and bills. Cost: a change that touches one step of a ten-step job impacts the whole job. |
| **N2 — a matrix job is one node** | one node per declared job, with `matrix` as an attribute | The file declares one job; the run creates N. dagreach reads files, so it reports what the file says and records the expansion count. Cost: "which matrix leg is affected" is not answerable. The alternative — expanding the matrix — requires evaluating expressions (see R1) and would make node identifiers unstable between runs. |
| **N3 — identifiers are the job ids** | `jobs.<id>` for GitHub Actions, the job name for GitLab | Typeable, stable, and what the platform's own UI shows. |

## What an edge is

| Decision | Choice | Why |
|---|---|---|
| **E1 — edge semantics are `depends-on`** | `needs:` names what must finish first | Both platforms write the dependency, not the flow. The profile declares it; the report states it, as for every profile. |
| **E2 — GitLab without `needs:` derives the graph from `stages:`** | every job of stage *n* depends on **every** job of stage *n-1* | This is what GitLab actually does. But it is a *derived* graph, not a declared one, and it is dense: 10 jobs over 3 stages produce ~33 edges nobody wrote. **The report must say `graph derived from stages, not from needs`**, and the derived edges carry `derived=stages`. |
| **E3 — a file mixing `needs:` and stage ordering keeps both** | declared `needs` plus derived stage edges for the jobs that declare none | Silently dropping one of the two would misreport the schedule. Both are marked, so a reader can tell them apart. |
| **E4 — a conditional edge is still an edge** | `if:` / `rules:` are recorded as an attribute, never evaluated | dagreach reports the reach a change **can** have, which is the right reading for a gate: a route that exists only on Tuesdays is still a route. The condition travels as `if=<expression>` so a reader can judge it. |

## Where the graph stops

| Decision | Choice | Why |
|---|---|---|
| **B1 — a reusable workflow is a boundary, not a hole** | `uses: owner/repo/.github/workflows/x.yml` becomes a node with `calls=<reference>`, and the profile does **not** follow it | Following would mean reading a file nobody passed on the command line, possibly from another repository, possibly over the network. dagreach reads what you hand it. A warning names every boundary crossed, so the answer is never quietly partial. |
| **B2 — `workflow_run` triggers are reported, not resolved** | a warning naming the referenced workflow | The edge is real but lives between two files; resolving it needs both, which is `diff`-shaped input, not a single file. |
| **B3 — one file is one graph** | several workflows are several graphs | Merging them would invent a shared namespace across files that the platform does not have. |

## What is refused outright

| **R1 — no expression evaluation.** `${{ github.event_name == 'push' }}`, GitLab variables, `!reference` — these are a language with a runtime. Evaluating a subset would be worse than not evaluating: it would be right often enough to be trusted, and wrong silently. |
|---|
| **R2 — no include resolution across files or repositories** (`include:`, `extends:`, remote templates). Same reason as B1, plus network access, which this tool does not do. |
| **R3 — no matrix expansion.** See N2; it depends on R1. |

Each refusal produces a warning when the construct is present, so a user reading a report knows
which parts of their file dagreach did not interpret.

## Attribute mapping

| dagreach attribute | GitHub Actions | GitLab CI |
|---|---|---|
| `group` | the job's `environment` when it declares one, otherwise `workflow:<name>` | the job's `environment`, otherwise `stage:<name>` |
| `status` | not set (a file has no run state) | not set |
| `duration` | not set (see below) | not set |
| kept as attributes | `runs_on`, `if`, `uses`, `matrix`, `environment`, `workflow` | `stage`, `rules`, `image`, `environment`, `derived` |

Mapping `group` to the deployment environment is deliberate: it makes the question everyone
actually asks — *does this change reach anything that deploys to production?* — a one-liner:

```bash
dagreach impact .github/workflows/ci.yml --changed build --fail-if-reaches group=production
```

The `workflow:` and `stage:` prefixes keep the fallback from colliding with a real environment name.

**Durations are not read from the file**, because a file has no timings. A profile could take them
from a run export later; until it does, the critical path of a pipeline is a *longest path measured
in edges*, and the output says so — as it already does everywhere else.

## The dependency this would cost

One module: **`go.yaml.in/yaml/v3`** — the fork maintained by the owners of the YAML specification,
adopted by Kubernetes and Prometheus. It replaces `gopkg.in/yaml.v3`, which is frozen: its
maintainer stepped back after fourteen years and only security fixes land there now.
`goccy/go-yaml` parses more of the YAML test suite but rests on a single maintainer, and dagreach
would read a trivial subset, so parser completeness is not what should decide — governance is.

In Go a dependency is compiled into the binary: users still download one file, nothing to install,
no conflicts. The cost is supply-chain surface, not user friction. That is a real cost and a small
one.

## The kill criteria

This page is worth writing precisely so it can conclude "no". These profiles should **not** exist
if any of the following turns out to be true when it meets a real repository:

1. **E2 makes GitLab unusable.** If deriving from stages produces a graph so dense that impact
   answers "everything reaches everything", the derived graph is not information. In that case the
   GitLab profile should refuse files that declare no `needs:` at all, rather than answer densely.
2. **B1 makes the answer routinely partial.** If most real workflows call reusable workflows, then
   stopping at the boundary means the reach is systematically understated — and understating reach
   in a gate is the one error that matters.
3. **R1 turns out to be load-bearing.** If jobs in practice are gated by expressions rather than by
   `needs`, then the declared graph is not the real one, and the profile would be measuring a
   fiction.

Points 1 and 2 are answerable in an afternoon against a sample of real workflows — and that is the
next step before writing any code, not after.
