# Writing a profile

A profile teaches dagreach to read one producer's export. This page is how to write one, in the
order the decisions actually come up. For what the shipped profiles do, see
[profiles.md](profiles.md).

## First: do you need one?

Often not. The `generic` reader already takes DOT and JSON Graph Format, and `--edge-semantics`
already covers the one thing a file cannot tell you:

```bash
dagreach impact plan.dot --edge-semantics depends-on --changed auth
```

A profile earns its place when a producer's export needs knowledge that is **not in the file** and
that a user should not have to supply every time:

| Signal | Example |
|---|---|
| The direction is a property of the tool, not of the file | `terraform graph` writes A → B meaning A depends on B |
| Identifiers need normalising to be typeable | `"[root] aws_vpc.main (expand)"` → `aws_vpc.main` |
| The format is not a graph format at all | a dbt manifest, a CycloneDX SBOM |
| Structure lives somewhere a generic reader will not look | dbt's `child_map`, CycloneDX's `dependencies` |

One signal is usually enough. Zero means `generic` plus a flag, and that is not a lesser answer.

## What a profile answers

Four questions, and every one of them produces a confidently wrong report when guessed:

1. **Which way does an edge point?** `feeds` or `depends-on`.
2. **What is a node's identifier?** The thing a user will type after `--changed`.
3. **What is worth calling a group?** The rollup that makes `--fail-if-reaches group=…` mean
   something for this producer.
4. **What else travels?** Every other attribute the export carries.

## The five fields

A profile is one entry in `internal/dagreach/adapters.go`. The `gomod` profile below is not shipped
— it is the example this page writes, so you can see where each piece lands:

```go
"gomod": {
    Name:          "gomod",
    Summary:       "reads a module graph, groups by module path",
    ProducedBy:    "go mod graph",
    EdgeSemantics: "depends-on",
    Load:          loadGoMod,
    Detect:        detectGoMod,
},
```

`Summary` and `ProducedBy` are not decoration: they are what `dagreach profiles` prints, and a test
fails when either is empty. Write `Summary` as what the profile *decided*, since that is what a
reader needs in order to trust or distrust the answer.

## `Load`: build the graph the producer meant

```go
func loadGoMod(text, source string) (*Graph, error)
```

You get the whole file as text and the name it came from. You return a `*Graph`, or a `*ParseError`
carrying `Line` and `Column` when you can — an unreadable input exits `4` and must never look like
a policy result.

The graph API is small:

```go
graph := NewGraph(source)
graph.Format = "gomod"                    // shown in `parse`; set it
graph.Name = "example.com/app"            // optional
graph.Attrs["go_version"] = "1.26"        // whole-graph facts
graph.AddNode(id, attrs, true)            // true: these attributes win
graph.AddEdge(source, target, nil)        // declares both endpoints if new
graph.Warn("...")                         // a sentence the report will carry
```

`AddNode` with `override = false` fills in only what is missing, which is what you want when a node
is mentioned before it is declared. Node order is declaration order, and that is contractual: every
list dagreach prints follows it, so read the producer's sections in the order they appear rather
than iterating a Go map.

If the producer emits DOT or JSON, do not write a parser:

```go
raw, err := ParseDOT(text, source)         // then rewrite ids, add groups
document, err := DecodeOrderedJSON(text)   // *Object, key order preserved
```

`*Object` has `Keys()`, `Value(key)`, `Get(key)`, `Len()`. Key order is preserved because JSON
object order is the declaration order of the graph. `asText(value)` renders a scalar and reports
whether it was one; `asString` renders anything.

### Three rules

**Return the producer's own orientation.** Never reverse edges inside `Load`, even when the
producer is a dependency tool. `EdgeSemantics` declares the direction and `Orient` applies it once,
at the door. Two reversals cancel out silently, and the report will then say the opposite of what
it did.

**Leave a trace of every decision.** An identifier you rewrote keeps the original as an attribute
(`terraform_id`); a fallback you took is a `Warn`; a section you skipped is a `Warn` with a count.
The rule is that a reader must be able to reconstruct what you did from the report alone:

```go
graph.Warn("no 'child_map' in this manifest; edges were read from depends_on.nodes")
```

**Degrade, do not refuse.** A missing optional section is a warning and a smaller graph. Return an
error only when the file is not what it claimed to be — a CycloneDX document that is not an object,
JSON that does not parse. An SBOM with no `dependencies` array still loads: it lists components,
says it found no relationships, and answers the structural questions honestly.

## `Detect`: recognise, never guess

```go
func detectGoMod(text string) bool
```

Detection runs when the user did not pass `--profile`, over the file content, in `ProfileOrder`. It
must be cheap — look at the first few thousand characters, not the whole file:

```go
func detectDBT(text string) bool {
    head := text
    if len(head) > 4000 {
        head = head[:4000]
    }
    return strings.Contains(head, `"dbt_schema_version"`) ||
        (strings.Contains(head, `"dbt_version"`) && strings.Contains(head, `"nodes"`))
}
```

Match on a marker the producer emits and nobody else does. A loose `Detect` is worse than no
`Detect`: it silently applies the wrong edge semantics to somebody else's file, and every answer
after that is inverted. When unsure, return `false` — the user can still pass `--profile`.

Detection is always announced in the report, so a wrong guess is visible rather than silent:

```text
warnings (1):
  - read with the dbt profile, recognised from the file itself; pass --profile to choose explicitly
```

## Attributes: keep the producer's vocabulary

dagreach reads exactly three attribute names, described in
[attribute-profile.md](attribute-profile.md): `group`, `status`, and `duration`/`weight`. Set
`group` to the producer's own natural rollup — resource kind, resource type, component type — and
`duration` when the export carries one.

**Everything else keeps the name the producer gave it.** Do not map a producer's vocabulary onto
dagreach's: `attr:NAME=VALUE` selects on any attribute, so `materialized`, `licenses` and `tags`
are usable as they are.

```bash
dagreach impact manifest.json --changed source.shop.orders --fail-if-reaches attr:materialized=table
```

A renamed attribute is a translation the user has to learn and a fact they can no longer look up in
their own producer's documentation. Copy what is useful, name it what it is called, and stop.

## Register it

Two places, and forgetting the second makes the profile invisible:

```go
var ProfileOrder = []string{"terraform", "dbt", "cyclonedx", "gomod", "generic"}
```

Order matters for detection: more specific producers first, `generic` last and never detected. A
test fails when the map and the list disagree.

## Test it

Add a fixture under `internal/dagreach/testdata/` — a real export, trimmed to the smallest thing
that still exercises the shape. Then the checklist, which is what the existing profile tests cover:

| Test | Why it is not optional |
|---|---|
| The fixture is detected as your profile | the whole point of `Detect` |
| No other fixture is detected as your profile | an over-eager `Detect` inverts somebody else's answers |
| Identifiers come out as documented | they are what users type after `--changed` |
| `group` comes out as documented | policies are written against it |
| An impact answer runs the right way **with no flag** | this is the one test that catches a double reversal |
| A degraded input loads with a warning | the fallback path is the one nobody runs by hand |

The last two matter most. A profile that is merely parsed is not tested; a profile whose impact
answer is asserted is:

```go
func TestTerraformImpactRunsTheRightWayWithoutAnyFlag(t *testing.T) { ... }
```

## Document it

- A section in [profiles.md](profiles.md): the command that produces the file, what the profile
  decided, and what the identifiers look like.
- An entry in `examples/examples.json` **if it answers a question the corpus does not already
  answer**. Every example runs in CI, so an example is a promise you keep, not a demonstration you
  wrote once. A profile does not need one to ship.

## The whole thing, as a checklist

- [ ] `generic` plus `--edge-semantics` genuinely does not cover it
- [ ] Five fields filled, `Summary` says what the profile decided
- [ ] `Load` returns the producer's orientation, unreversed
- [ ] `Format` set; identifiers normalised, with the original kept as an attribute
- [ ] Every fallback and every skipped section produces a `Warn`
- [ ] Unreadable input returns a `*ParseError` with a position when one exists
- [ ] `Detect` reads a bounded prefix, matches a marker nobody else emits, returns `false` when unsure
- [ ] Attributes keep the producer's own names, beyond `group`/`status`/`duration`
- [ ] Registered in both `profiles` and `ProfileOrder`
- [ ] Fixture in `testdata/`, and the six tests above
- [ ] A section in `profiles.md`
