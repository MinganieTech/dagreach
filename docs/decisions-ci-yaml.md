# A GitHub Actions structural dependency profile

**Status: measured against 228 real workflows, and the answer is no-go as specified.** Nothing is
in `go.mod`, no profile exists, and the measurement below is the deliverable. The design decisions
are kept because they remain right; the reason to build them is what the data removed.

The feature is **not** "YAML support". It is one producer's conventions, read from a file that
happens to be YAML.

## Why there is no generic YAML reader, and never will be

DOT and JSON Graph Format *represent a graph*. YAML has no graph semantics at all:

```yaml
build:
  dependencies: [compile]
```

Nothing in that document says whether `build` depends on `compile` or feeds it, whether keys are
nodes, whether `dependencies` means an execution order, an artifact, or a package version, or
whether a name that appears only there should become a node.

A generic YAML reader would have to guess all four, which contradicts the one doctrine dagreach
holds everywhere else: **warn rather than guess**. And it would be redundant — anyone with a
home-made YAML graph can emit JGF or DOT in twenty lines and get every guarantee this tool makes.

| Feature | Decision |
|---|---|
| YAML as a generic format | **No** |
| GitHub Actions profile | **Re-scoped after measurement — see the verdict below** |
| Raw `.gitlab-ci.yml` profile | Not now |
| Profile for a *resolved* GitLab export | To prototype |
| Network resolution of includes | **No** |
| Local multi-file resolution | Possibly, over files explicitly passed |

## What the profile actually answers

It does **not** answer "which jobs must run because `src/auth.go` changed" — dagreach does not
relate source files to jobs, and pretending otherwise would be the fiction this whole page exists
to avoid.

It answers:

- *If the job `build`, or its definition, changes — which jobs behind it can be affected?*
- *If `build` is compromised or modified, can it reach a job that deploys to production?*
- and with `diff`: *did this workflow change just open a new route to production?*

That is CI/CD blast radius and supply-chain reasoning, which is worth having, as long as it is
described as what it is. **The measurement below shows the second question is the one real files
cannot answer** — deployment environments are rarely declared and usually dynamic — and points at
what they can.

## The graph contract

| Rule | Choice |
|---|---|
| Node | one per `jobs.<job_id>` — never a step, which is not independently schedulable and which nothing depends on |
| Matrix | one aggregated node, `matrix=true`; the file declares one job, the run creates N, and dagreach reads files |
| Edge | `needs:` produces **depends-on** edges |
| `if:` | kept as a node attribute, never evaluated |
| Expressions | never evaluated, anywhere |
| Reusable workflow | the calling job stays the node `jobs.<id>`, with `calls=<reference>`; no external content is loaded |

```yaml
jobs:
  build:
  test:
    needs: build
  deploy:
    needs: test
    environment: production
```

becomes `test depends-on build` and `deploy depends-on test`. Because dagreach knows the
orientation, a change to `build` follows the edges backwards and reaches `test`, then `deploy`.

## Attributes

```text
group=production
environment=production
workflow=CI
if=<expression>
runs_on=ubuntu-latest
matrix=true
calls=owner/repo/.github/workflows/deploy.yml@ref
```

`group` takes the job's environment when it declares one, falling back to `workflow:<name>` so the
two namespaces cannot collide. That makes the question everyone asks a one-liner:
`--fail-if-reaches group=production`.

**A dynamic environment is never flattened into a group.** For

```yaml
environment: ${{ inputs.environment }}
```

the profile must not produce `group=${{ inputs.environment }}`. It produces:

```text
environment=${{ inputs.environment }}
environment_resolved=false
group=workflow:CI
```

with a warning that the environment could not be identified. An expression is not a value, and a
group nobody can select on is worse than an honest absence.

## Complete graph, partial attributes

A reusable workflow does not invalidate the graph at the calling level:

```yaml
jobs:
  deploy:
    uses: ./.github/workflows/deploy.yml
```

`deploy` is an opaque job, but the dependencies between `deploy` and the other jobs are perfectly
analysable. What is unknown is what is *inside* — the called workflow may itself declare
`environment: production`.

The report therefore separates two different kinds of incompleteness, rather than collapsing them
into one `complete=false`:

```text
graph_scope=declared-jobs
graph_coverage=complete
attribute_coverage=partial
unresolved_attributes=environment
```

## The verdict needs a third value

Consider:

```bash
dagreach impact ci.yml --changed build --fail-if-reaches group=production
```

If `build` reaches a job whose reusable workflow is unresolved, dagreach must neither return PASS
nor claim a proven FAIL:

```text
policies:
  UNKNOWN fail-if-reaches group=production:
    no declared job matching group=production was reached
    but reachable job deploy calls an unresolved reusable workflow
    whose environment is unknown
```

| Situation | Verdict | Exit code |
|---|---|---|
| Policy satisfied and the analysis was sufficient | PASS | `0` |
| Violation proven | FAIL | `1` |
| Invalid usage | ERROR | `2` |
| Policy undeterminable | **UNKNOWN** | `3` |
| Input could not be read | ERROR | `4` |

In CI, UNKNOWN blocks by default, without being presented as a proven violation. This is not a new
policy and not a policy language: it is a qualification the verdict always needed.

Two properties matter as much as the value itself:

- **Indeterminacy is contextual.** An unresolved reusable workflow that the changed node cannot
  reach does not make the policy undeterminable. Only what lies on a reachable path counts.
- **Code `3` was retired, and comes back for this.** It once meant "declared but not implemented";
  nothing is, so the number is free. Reusing it is a deliberate choice recorded here, not an
  accident. Code `4` stays distinct from `2`: a missing file and a bad flag are both errors, but a
  pipeline that wants to distinguish "you called me wrong" from "your input is unreadable" should
  keep being able to.

## Conditions: a deliberate over-approximation

dagreach already says what a change *can* reach, so not evaluating conditions is consistent with
the product. It must still be written down:

```text
reachability=structural-potential
conditions=evaluated:false
```

and never implied that the path will execute. Given

```yaml
build:
  if: github.ref == 'refs/heads/main'
deploy:
  needs: build
  if: github.ref != 'refs/heads/main'
```

the edge `build -> deploy` exists structurally, while no single run satisfies both conditions. The
result is a cautious over-approximation. **For a security gate that false positive is acceptable; a
false PASS is not.**

## What the measurement found

Run on **228 workflows from 23 public repositories** — Grafana, Prometheus, Terraform, dbt, the
GitHub CLI, Next.js, pandas, Home Assistant, Compose, Argo CD, Flux, Cilium, cosign, GoReleaser,
syft, Trivy, Dagger, the OpenTelemetry collector, Kustomize, Elasticsearch, Airflow, CPython,
Rust — holding **583 jobs**. A mixed sample chosen before the numbers were known, not one picked
to suit the answer.

| Question | Measured |
|---|---|
| jobs that call a reusable workflow | **21%** (122/583), in 16 of 23 repositories |
| jobs declaring `needs` | **37%** (218/583), 481 declared edges |
| workflows with any `needs` at all | **21%** (48/228) |
| **workflows that are a single job** | **66%** (150/228) |
| jobs carrying an `if:` condition | **55%** (320/583) |
| **jobs declaring an `environment`** | **3%** (20/583) |
| of those, a dynamic expression | **55%** (11/20) |

### Criterion 1 and 2: survived

Reusable workflows are common enough to matter — one job in five is opaque — but they do not make
the answer routinely partial. They do make **UNKNOWN mandatory rather than optional**: a policy
that ignores a fifth of the jobs would be guessing, which is the failure this profile exists to
avoid.

### Criterion 3: the headline use case does not survive

The flagship query this whole design was built around —

```bash
dagreach impact ci.yml --changed build --fail-if-reaches group=production
```

— rests on jobs declaring `environment:`. **Three percent do, and more than half of those are
expressions** (`${{ inputs.environment }}`, `${{ needs.init.outputs.channel }}`). A statically
identifiable production environment exists on roughly **one job in seventy**. The question the
profile was supposed to answer is, in practice, barely answerable.

Two further numbers resize the rest of the promise: **two thirds of workflows are a single job**,
so there is no graph to analyse at all, and **55% of jobs carry an `if:`**, so the structural
over-approximation is not a corner case but the norm.

### What is actually there

The same sample says the value sits elsewhere:

| Signal | Measured |
|---|---|
| jobs declaring a **write permission** | **26%** (149/583) |
| jobs passing **`secrets: inherit`** | **7%** (43/583) — every one of them an opaque reusable call |
| jobs named deploy/release/publish/push | 10% (58/583) |
| the workflows that *do* have a graph | 48 workflows, 302 jobs |

That is a supply-chain question, not a deployment one: *can a change to this job reach a job that
holds write permissions, or that hands its secrets to a workflow we cannot see?* Forty-three jobs
in this sample inherit secrets into an opaque call — and that is exactly where an UNKNOWN verdict
earns its place.

## Verdict

**No-go for the profile as specified.** `group=production` was the reason to build it, and the data
does not support it.

**Conditional go for a re-scoped one**, if it is built at all:

- gate on `permissions` and `secrets: inherit` rather than on `environment`, since those are
  declared, static, and present on 26% and 7% of jobs;
- keep `environment` as an attribute, never as a group when it is an expression;
- state plainly that two thirds of workflows have no graph, so the profile answers "nothing
  downstream" for most files and earns its keep on release pipelines;
- ship UNKNOWN with it, because a fifth of jobs are opaque.

It also stays where the priority list put it: **after** the profile-authoring guide, which lets
someone else write this profile — and after the HTML report. The measurement is the deliverable
here; the code is not owed.

## Why raw GitLab is deferred

GitHub Actions has a simple structural model: without `needs`, jobs are independent. GitLab adds
implicit stage barriers, `needs: []` which removes them, `rules:needs` which swaps dependencies by
context, `include` and conditional includes, `extends` and `!reference`, matrices with dependencies
on specific instances, child and multi-project pipelines, and hidden jobs used as templates.

That is no longer "a few dozen lines for a profile" — it starts to look like a partial
reimplementation of GitLab's own compiler, which would be wrong by construction and quietly so.

The better path is to let GitLab resolve its own configuration through its **CI Lint API**
(`dry_run` with `include_jobs`), and hand the resolved export to dagreach. The model is preserved:
one file in, one answer out, with no authentication, no network and no GitLab client in the core.

## The dependency this costs

**`go.yaml.in/yaml/v4`** — not v3. The YAML organisation's fork now carries the library, v1 to v3
are frozen outside security fixes, and v4 is the version recommended for new work. v4 also allows
limiting nesting depth and alias expansion, which matters for a tool that may read adversarial
files in CI. (Confirm the exact released version at implementation time; this page was written
from secondary sources.)

In Go the dependency is compiled into the binary: users still download one file. The cost is
supply-chain surface, not friction.

## Order of work

| Slice | Content |
|---|---|
| T6 | GitHub Action and pull-request comment |
| T6.1 | PASS / FAIL / UNKNOWN verdict qualification |
| T6.2 | Experimental GitHub Actions profile |
| — | GitLab profile from a resolved export |
| — | Reading `.gitlab-ci.yml` directly — still undecided |

T6 first: it makes the profiles that already exist reachable by everyone, which is worth more than
one more producer. The GitHub Actions profile then becomes an obvious example *inside* that Action.

And it ships under its real name — **a GitHub Actions structural dependency profile**, never "YAML
support" — which keeps the promise: dagreach understands one producer's conventions, exposes what
it deduced, and never turns an ignorance into a silent success.
