# A GitHub Actions structural dependency profile

**Status: decided in principle, not implemented.** Nothing is in `go.mod`, no profile exists, and
this lands after T6. The page exists so the decisions can be judged before any code.

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
| GitHub Actions profile | **Yes, experimental** |
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
described as what it is.

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
