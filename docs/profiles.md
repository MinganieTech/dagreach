
# Profiles: reading a producer's export

DOT and JSON carry structure. They do not carry which way an edge points, what an identifier is,
or what counts as a group — and getting those wrong produces confident, wrong answers.

A profile is that missing knowledge for one producer. It is detected from the file when you do not
name one, the report always says which profile was applied, and `--edge-semantics` overrides it.

```console
$ dagreach profiles
profiles:
  terraform  reads terraform graph
             edges: depends-on; strips the [root] decoration, groups by resource kind
  dbt        reads dbt (target/manifest.json)
             edges: feeds; reads a manifest offline, groups by resource type, keeps tags and materialisation
  cyclonedx  reads CycloneDX (syft, cdxgen, trivy, ...)
             edges: depends-on; reads an SBOM, groups by component type, keeps versions and licences
  generic    reads anything
             edges: feeds; DOT or JSON Graph Format, no normalisation, semantics up to you
```

`dagreach profiles --json` is the same listing for a program: `{name, produced_by, edge_semantics,
summary, detected}` per profile, so a tool can decide whether to hand dagreach a file — and which
way its edges will be read — without parsing the text above.

## terraform

```bash
terraform graph > infra.dot
dagreach impact infra.dot --changed aws_vpc.main --fail-if-reaches group=aws_db_instance
```

Terraform writes an edge from a resource **to what it depends on**, so impact runs backwards
through the file; the profile knows this, and you do not pass a flag. It also strips the
renderer's decoration so identifiers are typeable:

```text
"[root] aws_vpc.main (expand)"  ->  aws_vpc.main
```

The original is kept as the `terraform_id` attribute. If stripping would make two nodes collide,
those nodes keep their full identifiers and the report says so. `group` becomes the resource kind
(`aws_vpc`, `data`, `provider`), which is what makes `--fail-if-reaches group=aws_db_instance`
meaningful.

## dbt

```bash
dbt parse                       # writes target/manifest.json
dagreach impact target/manifest.json --changed source.shop.orders --explain
```

dbt answers "what is downstream of this model" through its own selectors. What it cannot do is
answer it **offline**, against a manifest you did not just produce, or **compare two manifests** —
which is exactly what a change gate needs:

```bash
dagreach diff main/manifest.json pr/manifest.json \
  --changed model.acme.stg_orders --fail-on-new-reach group=exposure
```

A manifest is recognised by its `metadata`, or failing that by carrying both `child_map` and
`parent_map` — the shape that survives when a manifest is committed for a documentation site and
loses its version markers.

Models, sources, tests, exposures, metrics and semantic models all become nodes, keyed by their
dbt `unique_id`. `group` is the resource type; `tags`, `materialized`, `schema` and `package_name`
travel as attributes. Edges come from `child_map`, and fall back to `depends_on.nodes` on older
manifests — with a warning, so you know which was used.

## cyclonedx

```bash
syft dir:. -o cyclonedx-json > sbom.json
dagreach impact sbom.json --changed 'pkg:npm/qs@6.11.0' --explain
```

The supply-chain question is a reach question: a library is found vulnerable, or changes licence —
what depends on it?

```text
sbom.json: pkg:npm/qs@6.11.0 reaches 4 of 4 nodes (100%)
edges: cyclonedx profile, source depends on target, so impact follows edges backward
downstream (3): pkg:npm/checkout-service@2.4.0, pkg:npm/express@4.19.2, pkg:npm/body-parser@1.20.2
why (3 of 3 shown):
  pkg:npm/checkout-service@2.4.0 (distance 3): pkg:npm/qs@6.11.0 -> pkg:npm/body-parser@1.20.2 -> ...
```

Nodes are keyed by `bom-ref` (or `purl`), `group` is the component type — the root component from
`metadata.component` gets `group=root`, so `--fail-if-reaches group=root` asks "does this package
reach the product itself?". `name`, `version`, `purl` and `licenses` travel as attributes.

## generic

Everything else: DOT or JSON Graph Format, read as they are, nothing normalised. **You declare the
direction**, because nobody else can:

```bash
dagreach impact plan.dot --edge-semantics depends-on --changed auth
```

If the file looks like a known dependency export and you did not say so, dagreach warns rather
than answering backwards.

## Adding a profile

A profile is an entry in `internal/dagreach/adapters.go` with five things: the name, what produces
it, its edge semantics, a `Load(text, source) (*Graph, error)`, and a `Detect` that recognises the
format from its first few thousand characters — or returns `false` rather than guessing.

Three rules make a profile trustworthy rather than magic:

- **`Load` returns the graph in the producer's own orientation.** The single reversal happens later,
  in one place, driven by the declared semantics.
- **Anything the profile decided is visible**: an identifier it rewrote keeps the original as an
  attribute, a fallback it took is a warning, and detection itself is announced.
- **Attributes keep the producer's own names.** `group`, `status` and `duration` are the only three
  dagreach reads; everything else stays as it was written and is selectable with `attr:NAME=VALUE`.

**[writing-a-profile.md](writing-a-profile.md) is the full guide** — when a profile is worth it,
the graph API, what to test, and a checklist.
