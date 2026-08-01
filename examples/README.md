# Examples, by what you are trying to prevent

Each section is a real question, a real graph in this directory, and the command that answers it.
Every one of them runs in CI: `examples.json` is a manifest, and a test checks each exit code and
each quoted line, so nothing here can rot into a lie.

Run them from this directory with `dagreach` on your PATH.

---

## Stop an infrastructure change from reaching a production database

```bash
terraform graph > infra.dot
dagreach impact infra.dot --changed aws_vpc.main \
  --fail-if-reaches group=aws_db_instance --explain
```

```text
infra.dot: aws_vpc.main reaches 7 of 9 nodes (78%)
edges: terraform profile, source depends on target, so impact follows edges backward
downstream (6): aws_subnet.public, aws_subnet.private, aws_security_group.web, ... (+2 more)
note: aws_vpc.main is an articulation point: everything behind it depends on it alone
policies:
  FAIL fail-if-reaches group=aws_db_instance: 1 node(s) matching group=aws_db_instance are reached
    aws_db_instance.orders: aws_vpc.main -> aws_subnet.private -> aws_db_instance.orders
```

Exit `1`, and the reviewer sees the route rather than a number. Note the second line: Terraform
writes an edge from a resource **to what it depends on**, so impact runs backwards through the
file — the profile knows that, and says so.

## Catch a pull request that opens a new route to production

The most valuable one, because a review will not catch it: one added edge, `token -> payments`,
looks harmless on its own.

```bash
dagreach diff services-before.dot services-after.dot --changed auth \
  --explain --fail-on-new-reach group=production
```

```text
services-before.dot -> services-after.dot: auth reaches 4 nodes, was 3 (+1, -0)
new reach (1): payments
structure: 0 node(s) added, 0 removed, 1 edge(s) added, 0 removed
why (1 of 1 shown):
  payments is now reached
    reason: new edge token -> payments
    path:   auth -> token -> payments
policies:
  FAIL fail-on-new-reach group=production: 1 target(s) matching group=production became reachable
```

Not "which edges changed" — *which reach relationships became possible*. A target that was always
reachable and has just been reclassified into `group=production` fails the same policy, with
`reason: reclassified` instead of a named edge.

## Find what a vulnerable package reaches, across ecosystems

```bash
syft dir:. -o cyclonedx-json > service-sbom.cdx.json
dagreach impact service-sbom.cdx.json --changed 'pkg:npm/qs@6.11.0' \
  --fail-if-reaches group=application
```

```text
service-sbom.cdx.json: pkg:npm/qs@6.11.0 reaches 5 of 7 nodes (71%)
edges: cyclonedx profile, source depends on target, so impact follows edges backward
impacted leaves (2): pkg:npm/orders-api@3.1.0, pkg:npm/reporting-cli@0.4.2
policies:
  FAIL fail-if-reaches group=application: 1 node(s) matching group=application are reached
    pkg:npm/reporting-cli@0.4.2: pkg:npm/qs@6.11.0 -> pkg:npm/reporting-cli@0.4.2
```

`npm why` answers this inside one ecosystem. An SBOM crosses them, and the answer comes with an
exit code.

## Know which models, tests and dashboards a changed source reaches

```bash
dbt parse    # writes target/manifest.json
dagreach impact warehouse-manifest.json --changed source.warehouse.shop.customers --explain
```

```text
warehouse-manifest.json: source.warehouse.shop.customers reaches 8 of 10 nodes (80%)
edges: dbt profile, source feeds target, so impact follows edges forward
impacted leaves (4): test.warehouse.not_null_fct_orders_id, ... exposure.warehouse.crm_sync
groups touched: model 3, test 2, source 1, exposure 2
```

dbt's own selectors answer this too — but not **offline**, not against a manifest you did not just
produce, and not between two manifests:

```bash
dagreach diff main/manifest.json pr/manifest.json \
  --changed model.warehouse.fct_orders --fail-on-new-reach group=exposure
```

## Refuse a change that touches more of the graph than the team accepts

```bash
dagreach impact warehouse-manifest.json \
  --changed source.warehouse.shop.customers --max-impacted 5
```

```text
policies:
  FAIL max-impacted 5: 8 node(s) impacted, ceiling is 5
```

A blunt instrument, and useful exactly where blunt is right: a pull request that quietly doubles
its own blast radius.

## Gate on an attribute the producer emits and dagreach never named

`group=` and `status=` are shorthands, not the vocabulary. dbt records how each model is
materialized; nobody at dagreach decided that was interesting, and it does not have to:

```bash
dagreach impact warehouse-manifest.json \
  --changed source.warehouse.shop.customers --fail-if-reaches attr:materialized=table
```

```text
policies:
  FAIL fail-if-reaches attr:materialized=table: 2 node(s) matching attr:materialized=table are reached
    model.warehouse.fct_orders: source.warehouse.shop.customers -> model.warehouse.stg_customers -> model.warehouse.fct_orders
```

`attr:NAME=VALUE` reads any attribute a profile or a file carries — `risk`, `team`, `tier`,
`licenses`. The prefix is required so that a typo is a usage error rather than a policy that
matches nothing and waves the change through.

## Refuse to call a question answered when the graph cannot answer it

The same SBOM as above, asked a question it has no data for — this one carries licences, not
vulnerability severities:

```bash
dagreach impact service-sbom.cdx.json --changed 'pkg:npm/qs@6.11.0' \
  --fail-if-reaches attr:severity=critical
```

```text
policies:
  UNKNOWN fail-if-reaches attr:severity=critical: no node in this graph declares 'severity', so nothing can match attr:severity=critical and the policy cannot be settled by this file
```

Exit `3`, not `0`. Nothing matched, but nothing *could* have matched: reporting that as a pass would
be a statement about the file dressed up as a statement about the change. This is the failure mode
that quietly turns a gate into decoration, and it is worth its own exit code.

## See what the shape of a plan allows before committing to a date

```bash
dagreach stats release-plan.jgf.json
```

```text
release-plan.jgf.json: 8 nodes, 10 edges, acyclic
shape: depth 5 level(s), width 2 (largest earliest-start generation), 1 root(s), 1 leaf/leaves
critical path: 60 of duration over 4 edge(s)
  spec -> schema -> api -> web (+1 more)
```

Width is the most work the dependencies ever allow at one moment: two people, not five. The
critical path is 60 hours no amount of parallelism removes.

## Know what a slipped task delays, and by how much

```bash
dagreach impact release-plan.jgf.json --changed api
```

```text
downstream (3): web, load-test, release
cost of the impacted set: 58 of declared duration
critical path within the impacted set: 46 of duration over 2 edge(s)
  api -> web -> release
```

`api` slipping does not delay the release by its own duration; it delays it by the 46 hours of
chain behind it.

---

## The corpus

| File | What it is | Profile |
|---|---|---|
| `infra.dot` | `terraform graph` output for a small stack | terraform |
| `services-before.dot`, `services-after.dot` | a service map, before and after one added edge | generic |
| `service-sbom.cdx.json` | a CycloneDX SBOM with a shared transitive dependency | cyclonedx |
| `warehouse-manifest.json` | a dbt manifest: sources, models, tests, exposures | dbt |
| `release-plan.jgf.json` | a task plan in JSON Graph Format, with durations and groups | generic |

Every file is used by at least one example, and a test enforces that too: a graph nobody asks a
question about is decoration.
