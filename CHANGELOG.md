# Changelog

Notable changes, newest first. Versions follow [semantic versioning](https://semver.org): before
1.0, a minor bump may break things, and the JSON report carries its own `schema_version` so a
consumer can tell.

## v0.1.0-rc.2

Fixes one defect in `rc.1`, found by running the published action on all three systems - the first
time that path had ever run.

- The action passed `--explain` to every command but `parse`, including `stats`, which does not read
  it. Silently dropped until per-command flag validation turned it into a usage error, so the
  strictness that closed a fail-open path opened a different one in the caller. `stats` through the
  action now works; `impact` and `diff` were never affected.

The `rc.1` binary itself is sound: the defect was in `action.yml`, and only for `stats`.

## v0.1.0-rc.1

First release. Everything below has existed for a while in the repository; this is the point at
which somebody other than the author can run it.

### Reading

- DOT and JSON Graph Format, with a hand-written parser and no dependencies.
- Profiles for `terraform graph`, dbt manifests and CycloneDX SBOMs, detected from the file and
  always announced. `generic` for everything else.
- `--edge-semantics feeds|depends-on`, applied once at the door, stated in every report, and warned
  about when a file looks like a dependency export read the wrong way.
- Ambiguity refused rather than resolved: a JSON document is one value and then the end of the file,
  and a key declared twice in the same object is an error.

### Analysis

- Reachability, upstream and downstream, with a shortest witness path per reached node.
- Cycles listed and collapsed before the metrics that need acyclicity, which the report says.
- Depth, width, articulation points, longest path — "critical" only when durations exist.
- `most reaching`: every node ranked by how much of the graph sits behind it, up to 25 000 nodes.
- Reach diff between two graphs, with the reason a target became reachable: `new-node`, `new-edge`
  or `reclassified`.

### Deciding

- `--fail-if-reaches`, `--max-impacted`, `--fail-on cycle`, `--fail-on-new-reach`.
- Selectors: `group=`, `status=`, `node=`, and `attr:NAME=VALUE` for anything else a producer emits.
- Three verdicts. A selector naming an attribute no node declares is `UNKNOWN`, not a pass.
- Exit codes: `0` ok, `1` a policy failed, `2` usage, `3` a policy this graph could not settle,
  `4` unreadable input.
- A flag a command does not read is a usage error, never a silent no-op.
- `diff` refuses two graphs read differently rather than producing a backwards delta.

### Reporting

- Text, `--json` (versioned, pinned by golden files), `--markdown`, `--html`.
- The markdown report cannot be forged by a node identifier; the HTML page escapes everything from
  the graph, fetches nothing and runs no script.

### Shipping

- A GitHub Action that downloads the released binary, checks it against the release's own
  `checksums.txt`, and posts one comment per tag.
- Static binaries for Linux, macOS and Windows on `amd64` and `arm64`.
